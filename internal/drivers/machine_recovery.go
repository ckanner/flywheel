package drivers

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// MachineRecoveryArgs describes an existing invocation. There is intentionally
// no prompt, credential, new session option or retry callback on this interface.
type MachineRecoveryArgs struct {
	Harness           string
	RequestID         string
	RunID             string
	RecoveryDirectory string
	Cwd               string
	Model             string
	ReasoningEffort   string
	PermissionMode    string
	PersistentSession bool
	SessionID         string
	Watchdog          watchdog.Opts
}

type machineRecoveryOriginal struct {
	Cwd             string `json:"cwd"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort"`
	PermissionMode  string `json:"permissionMode"`
	ThreadMode      string `json:"threadMode"`
	ThreadID        string `json:"threadId,omitempty"`
}

type machineRecoveryRequest struct {
	Type              string                  `json:"type"`
	Protocol          int                     `json:"protocol"`
	RequestID         string                  `json:"requestId"`
	RunID             string                  `json:"runId"`
	RecoveryDirectory string                  `json:"recoveryDirectory"`
	Original          machineRecoveryOriginal `json:"original"`
}

var encodeMachineRecovery = (*json.Encoder).Encode

func parseMachineRecovery(raw string, request machineRecoveryRequest, harness string) (json.RawMessage, error) {
	var frame struct {
		Type      string          `json:"type"`
		Protocol  int             `json:"protocol"`
		RequestID string          `json:"requestId"`
		Adapter   machineAdapter  `json:"adapter"`
		Error     json.RawMessage `json:"error"`
		Recovery  *struct {
			Protocol  int             `json:"protocol"`
			Type      string          `json:"type"`
			RequestID string          `json:"requestId"`
			RunID     string          `json:"runId"`
			Status    string          `json:"status"`
			Receipt   json.RawMessage `json:"receipt"`
		} `json:"recovery"`
	}
	if err := json.Unmarshal([]byte(raw), &frame); err != nil || frame.Type != "recovery" || frame.Protocol != 3 ||
		frame.RequestID != request.RequestID || !machineAdapterMatches(frame.Adapter, harness) || string(frame.Error) != "null" ||
		frame.Recovery == nil || frame.Recovery.Protocol != 1 || frame.Recovery.Type != "recovery" ||
		frame.Recovery.RequestID != request.RequestID || frame.Recovery.RunID != request.RunID {
		return nil, fmt.Errorf("read-only recovery envelope or invocation identity is invalid")
	}
	if frame.Recovery.Status != "terminal" || len(frame.Recovery.Receipt) == 0 || string(frame.Recovery.Receipt) == "null" {
		return nil, fmt.Errorf("read-only recovery has no durable terminal receipt; invocation remains blocked")
	}
	return frame.Recovery.Receipt, nil
}

func recoveredMachineResult(harness string, raw json.RawMessage, request codexProStart) Result {
	out := Result{Harness: harness, Model: request.Model, Recovered: true}
	message := ""
	if harness == "codex-pro" {
		parsed := parseCodexProReceipt(raw, request)
		out.CodexProReceipt, out.Session, out.SessionID, out.Text, out.Usage = raw, parsed.Session, parsed.SessionID, parsed.Text, parsed.Usage
		out.Settled, out.Cancelled, out.TimedOut, out.Truncated = parsed.Settled, parsed.Cancelled, parsed.TimedOut, parsed.Truncated
		out.Session.CleanupConfirmed, out.CleanupError, message = parsed.CleanupConfirmed, parsed.CleanupError, parsed.Error
	} else {
		parsed := parseClaudeCodeProReceipt(raw, claudeCodeProStart(request))
		out.ClaudeCodeProReceipt, out.Session, out.SessionID, out.Text, out.Usage = raw, parsed.Session, parsed.Session.SessionID, parsed.Text, parsed.Usage
		out.Settled, out.Cancelled, out.TimedOut, out.Truncated = parsed.Settled, parsed.Cancelled, parsed.TimedOut, parsed.Truncated
		out.Session.CleanupConfirmed, out.ErrorCode, message = parsed.CleanupConfirmed, parsed.ErrorCode, parsed.Error
	}
	if message != "" {
		out.ErrorMessage = &message
	}
	if out.Session.Dispatch == "not-dispatched" && out.Session.CleanupConfirmed {
		out.Usage, out.CostUSD, out.Session.UsageStatus = &Usage{}, new(float64), "complete"
	}
	return out
}

// RecoverMachine starts only the worker's public read-only query. It never calls
// runMachine, starts a CLI or sends a turn. Native cleanup must be proven by the
// stored public receipt, not by this short-lived query process exiting.
func RecoverMachine(a MachineRecoveryArgs) (Result, error) {
	if (a.Harness != "codex-pro" && a.Harness != "claude-code-pro") || !machineOpaqueID(a.RequestID) || !machineOpaqueID(a.RunID) ||
		!filepath.IsAbs(a.RecoveryDirectory) || strings.ContainsRune(a.RecoveryDirectory, 0) || !filepath.IsAbs(a.Cwd) ||
		!machineOpaqueID(a.Model) || !machineOpaqueID(a.ReasoningEffort) ||
		(a.PermissionMode != "read-only" && a.PermissionMode != "workspace-write" && a.PermissionMode != "danger-full-access") ||
		(a.SessionID != "" && (!a.PersistentSession || !machineOpaqueID(a.SessionID))) {
		return Result{}, fmt.Errorf("invalid original invocation for read-only recovery")
	}
	mode, command := "ephemeral", CodexProCommand()
	if a.PersistentSession {
		mode = "persistent"
	}
	if a.Harness == "claude-code-pro" {
		command = ClaudeCodeProCommand()
	}
	request := machineRecoveryRequest{Type: "recover", Protocol: 3, RequestID: a.RequestID, RunID: a.RunID, RecoveryDirectory: a.RecoveryDirectory,
		Original: machineRecoveryOriginal{Cwd: a.Cwd, Model: a.Model, ReasoningEffort: a.ReasoningEffort, PermissionMode: a.PermissionMode, ThreadMode: mode, ThreadID: a.SessionID}}
	written := make(chan error, 1)
	result := watchdog.Run(command, nil, watchdog.Opts{
		Context: a.Watchdog.Context, Env: childenv.EnvSlice(childenv.BuildChildEnv(childenv.OSLookupEnv, nil)),
		IdleTimeoutMs: 10_000, WallTimeoutMs: 10_000, KillGraceMs: 1_000,
		InteractStdin: func(stdin io.WriteCloser, finish func()) {
			err := encodeMachineRecovery(json.NewEncoder(stdin), request)
			written <- err
			if err != nil {
				finish()
			}
		},
	})
	select {
	case err := <-written:
		if err != nil {
			return Result{}, fmt.Errorf("read-only recovery request write failed: %w", err)
		}
	default: // no child existed
	}
	if result.ExitCode == nil || *result.ExitCode != 0 || result.Signal != "" || result.TimedOut || result.Cancelled || result.Truncated ||
		result.SpawnError != "" || result.CleanupError != "" {
		return Result{}, fmt.Errorf("read-only recovery worker did not complete cleanly")
	}
	raw, err := parseMachineRecovery(result.Stdout, request, a.Harness)
	if err != nil {
		return Result{}, err
	}
	original := codexProStart{Type: "start", Protocol: 3, RequestID: a.RequestID, Cwd: a.Cwd, Model: a.Model,
		ReasoningEffort: a.ReasoningEffort, PermissionMode: a.PermissionMode, ThreadMode: mode, ThreadID: a.SessionID}
	out := recoveredMachineResult(a.Harness, raw, original)
	if out.Session.RunID != a.RunID {
		return Result{}, fmt.Errorf("read-only recovery receipt does not belong to the recorded run")
	}
	out.ExitCode = result.ExitCode
	return out, nil
}
