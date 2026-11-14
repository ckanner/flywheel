// DSH worker adapter：Go 拥有超时/取消和确定性验收，Node worker 用公开 SDK
// 开 fresh session 或恢复已确认的 scoped history；不隐式降级 provider/model/权限。
package drivers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
)

const DSHWorkerEnv = "FLYWHEEL_DSH_WORKER"

// DSHArgs binds one SDK worker to one provider/model and permission scope.
type DSHArgs struct {
	Native            json.RawMessage
	Managed           *NativeAdmission
	Prewarm           PreparedDSH // one-use SDK initialization, never a shared model session
	PrewarmID         string      // durable owner-chosen random identity before startup
	PrewarmDirectory  string      // existing private physical directory for pre-dispatch cleanup evidence
	Prompt            string
	Doctor            bool // local SDK route/policy/shutdown probe; never submits a model prompt
	Provider          string
	Model             string
	Thinking          string
	PermissionMode    string
	SessionID         string // explicit history handle; only protocol v2 permits resume
	PersistentSession bool
	RequestID         string // engine invocation, not an SDK idempotency key
	Cwd               string
	WorkspaceCwd      string
	SessionTitle      string
	Command           string // executable path; empty => FLYWHEEL_DSH_WORKER or flywheel-dsh-worker
	APIKeys           map[string]string
	Watchdog          watchdog.Opts
	OnEvent           func(line string)
}

// DSHCommand is the configured worker executable, shared with CLI diagnostics.
func DSHCommand() string {
	if command := os.Getenv(DSHWorkerEnv); command != "" {
		return command
	}
	return "flywheel-dsh-worker"
}

type dshStart struct {
	Native          json.RawMessage  `json:"native,omitempty"`
	Managed         *NativeAdmission `json:"managed,omitempty"`
	Type            string           `json:"type"`
	Protocol        int              `json:"protocol"`
	RequestID       string           `json:"requestId"`
	Cwd             string           `json:"cwd"`
	WorkspaceCwd    string           `json:"workspaceCwd,omitempty"`
	SessionTitle    string           `json:"sessionTitle,omitempty"`
	Provider        string           `json:"provider"`
	Model           string           `json:"model"`
	ReasoningEffort string           `json:"reasoningEffort"`
	PermissionMode  string           `json:"permissionMode"`
	Prompt          string           `json:"prompt"`
	TimeoutMS       int64            `json:"timeoutMs,omitempty"`
	SessionID       string           `json:"sessionId,omitempty"`
	Persistent      bool             `json:"persistent,omitempty"`
}

type dshParsed struct {
	NativeReceipt    json.RawMessage
	SessionID        string
	Text             string
	Usage            *Usage
	CostUSD          *float64
	Settled          bool
	Cancelled        bool
	Error            string
	ErrorCode        errs.ErrorCode
	CleanupError     string
	CleanupConfirmed bool
	Session          *SessionOutcome
}

const DSHSessionProtocol = "dsh-sdk/session-resume/worker-v2"
const DSHFreshSessionProtocol = "dsh-sdk/worker-v1"

// validateDSHLifecycle checks independently reported run facts. A completed
// turn may have unusable model output; neither that nor process cleanup makes
// a missing terminal or ambiguous restore a known execution result.
func validateDSHLifecycle(s *SessionOutcome, request dshStart, sessionID string, settled, ready bool, status string) error {
	if s == nil || s.Protocol != DSHSessionProtocol || s.RequestID != request.RequestID || s.SessionID != sessionID ||
		s.CleanupConfirmed != settled || (ready && s.ResumeAccepted != (request.SessionID != "")) ||
		(s.ResumeAccepted && (request.SessionID == "" || s.SessionID != request.SessionID)) {
		return fmt.Errorf("DSH lifecycle identity or cleanup receipt mismatch")
	}
	if (s.Dispatch != "not-dispatched" && s.Dispatch != "dispatched" && s.Dispatch != "unknown") ||
		(s.Outcome != "completed" && s.Outcome != "failed" && s.Outcome != "cancelled" && s.Outcome != "unknown") ||
		(s.UsageStatus != "complete" && s.UsageStatus != "unknown") {
		return fmt.Errorf("DSH lifecycle contains unknown facts")
	}
	if (s.Dispatch == "unknown" && s.Outcome != "unknown") ||
		(s.Dispatch == "not-dispatched" && s.Outcome != "failed" && s.Outcome != "unknown") ||
		(s.Dispatch == "dispatched" && s.Outcome != "unknown" && (!ready || s.TurnID == "")) ||
		(status == "ok" && (s.Dispatch != "dispatched" || s.Outcome != "completed")) ||
		(s.ResumeCandidate && (!request.Persistent || !ready || s.Outcome != "completed" || !settled || s.Dispatch != "dispatched" || status == "cancelled")) {
		return fmt.Errorf("DSH lifecycle contradicts the terminal result")
	}
	return nil
}

// parseDSHStream validates accepted SDK configuration, attested permission and
// directory receipts, and terminal state. Unknown frames or a missing receipt
// cannot be treated as success. Raw durable DSH events remain in stdout for audit.
func parseDSHStream(stdout string, request dshStart) dshParsed {
	var parsed dshParsed
	ready, terminal, cleanupConfirmed := false, false, false
	providerError := ""
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var frame struct {
			Native          json.RawMessage `json:"native"`
			Type            string          `json:"type"`
			Protocol        int             `json:"protocol"`
			RequestID       string          `json:"requestId"`
			SessionID       string          `json:"sessionId"`
			Cwd             string          `json:"cwd"`
			WorkspaceCwd    string          `json:"workspaceCwd"`
			Provider        string          `json:"provider"`
			Model           string          `json:"model"`
			ReasoningEffort string          `json:"reasoningEffort"`
			PermissionMode  string          `json:"permissionMode"`
			ApprovalPolicy  string          `json:"approvalPolicy"`
			Status          string          `json:"status"`
			Settled         bool            `json:"settled"`
			Text            string          `json:"text"`
			Usage           json.RawMessage `json:"usage"`
			CostUSD         *float64        `json:"costUsd"`
			Event           json.RawMessage `json:"event"`
			Persistent      *bool           `json:"persistent"`
			Resumed         *bool           `json:"resumed"`
			Lifecycle       *SessionOutcome `json:"lifecycle"`
			Error           *struct {
				Code    errs.ErrorCode `json:"code"`
				Message string         `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			parsed.Error = "DSH worker emitted malformed JSONL"
			break
		}
		if frame.Protocol != request.Protocol || frame.RequestID != request.RequestID || terminal {
			parsed.Error = "DSH worker protocol/request identity mismatch or frame after terminal result"
			break
		}
		switch frame.Type {
		case "ready":
			if ready || frame.SessionID == "" || frame.Cwd != request.Cwd || frame.WorkspaceCwd != request.WorkspaceCwd || frame.Provider != request.Provider ||
				frame.Model != request.Model || frame.ReasoningEffort != request.ReasoningEffort ||
				frame.PermissionMode != request.PermissionMode || frame.ApprovalPolicy != "never" {
				parsed.Error = "DSH worker effective settings mismatch"
				break
			}
			if request.Protocol == 2 && (frame.Persistent == nil || !*frame.Persistent || frame.Resumed == nil ||
				*frame.Resumed != (request.SessionID != "") || (request.SessionID != "" && frame.SessionID != request.SessionID)) {
				parsed.Error = "DSH worker history request or restored session identity mismatch"
				break
			}
			ready = true
			parsed.SessionID = frame.SessionID
		case "event":
			if !ready || frame.SessionID != parsed.SessionID || len(frame.Event) == 0 || string(frame.Event) == "null" {
				parsed.Error = "DSH worker event arrived without a matching ready session"
			}
		case "native-event":
			if len(request.Native) == 0 || !ready || frame.SessionID != parsed.SessionID || len(frame.Event) == 0 || string(frame.Event) == "null" {
				parsed.Error = "native event arrived without a matching admitted invocation"
			}
		case "result":
			if err := validateNativeReceipt(request.Native, frame.Native, frame.Status == "ok", frame.Settled); err != nil {
				parsed.Error = err.Error()
				break
			}
			parsed.NativeReceipt = frame.Native
			terminal = true
			cleanupConfirmed = frame.Settled
			if !frame.Settled {
				// This is absence of a cleanup guarantee, not evidence that a
				// particular SDK child is still alive. Do not overlap another call.
				parsed.CleanupError = "DSH worker did not confirm SDK process cleanup"
			}
			if ready && frame.SessionID != parsed.SessionID {
				parsed.Error = "DSH worker returned a terminal result for another session"
				break
			}
			if request.Protocol == 2 {
				if err := validateDSHLifecycle(frame.Lifecycle, request, frame.SessionID, frame.Settled, ready, frame.Status); err != nil {
					parsed.Error = err.Error()
					break
				}
				parsed.Session = frame.Lifecycle
				usage, err := parseDSHUsage(frame.Usage)
				if err != nil || (frame.Lifecycle.UsageStatus == "complete") != (usage != nil) || (frame.CostUSD != nil && *frame.CostUSD < 0) {
					parsed.Error = "DSH lifecycle usage completeness mismatch"
					break
				}
				parsed.Usage, parsed.CostUSD = usage, frame.CostUSD
			}
			// Startup failures may precede ready. Success always requires the receipt.
			if frame.Status == "error" && frame.Error != nil {
				if !errs.IsKnownCode(frame.Error.Code) {
					parsed.Error = "DSH worker returned an invalid structured error receipt"
					break
				}
				parsed.ErrorCode = frame.Error.Code
				providerError = string(frame.Error.Code) + ": " + frame.Error.Message
				// Keep checking the stream: a later frame cannot leave a trusted
				// provider code behind after violating terminal ordering.
				break
			}
			if frame.Status == "cancelled" {
				parsed.Cancelled = true
				break
			}
			if frame.Status != "ok" || !frame.Settled || !ready || frame.SessionID != parsed.SessionID ||
				strings.TrimSpace(frame.Text) == "" || frame.Error != nil {
				parsed.Error = "DSH worker result lacks successful completion of the ready session"
				break
			}
			usage, err := parseDSHUsage(frame.Usage)
			if err != nil {
				parsed.Error = err.Error()
				break
			}
			if frame.CostUSD != nil && *frame.CostUSD < 0 {
				parsed.Error = "DSH worker reported negative cost"
				break
			}
			parsed.Usage, parsed.CostUSD, parsed.Text = usage, frame.CostUSD, frame.Text
			parsed.Settled = true
		default:
			parsed.Error = "DSH worker emitted an unknown protocol frame"
		}
		if parsed.Error != "" {
			break
		}
	}
	if parsed.Error == "" && !terminal {
		parsed.Error = "DSH worker ended without a terminal result"
	}
	// Cleanup is an independent fact, including ordinary provider failures.
	// A malformed stream cannot attest a trustworthy shutdown receipt.
	parsed.CleanupConfirmed = terminal && cleanupConfirmed && parsed.Error == ""
	if parsed.Error != "" {
		parsed.ErrorCode = errs.TRANSPORT
		parsed.Session = nil
		parsed.Usage, parsed.CostUSD = nil, nil
	} else if providerError != "" {
		parsed.Error = providerError
	}
	parsed.Settled = parsed.Settled && parsed.Error == "" && !parsed.Cancelled
	return parsed
}

// The worker reports aggregate input (including cache read/write) and output;
// reasoning is a subset of output, so do not add it to total a second time.
func parseDSHUsage(raw json.RawMessage) (*Usage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var values map[string]*int64
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("DSH worker reported invalid usage: %w", err)
	}
	var usage Usage
	for _, field := range []struct {
		name  string
		value *int64
	}{
		{"input_tokens", &usage.Input}, {"output_tokens", &usage.Output},
		{"reasoning_tokens", &usage.Reasoning}, {"cache_read_tokens", &usage.CacheRead},
		{"cache_write_tokens", &usage.CacheWrite}, {"total_tokens", &usage.TotalTokens},
	} {
		value, exists := values[field.name]
		if !exists || value == nil || *value < 0 {
			return nil, fmt.Errorf("DSH worker reported incomplete or negative usage: %s", field.name)
		}
		*field.value = *value
	}
	return &usage, nil
}

// RunDSH exchanges one versioned JSONL request/result with the bundled SDK worker.
// The process inherits only the standard CLI environment, explicit channel keys,
// and three DSH runtime settings. No user's complete shell environment is copied.
func dshCall(a DSHArgs) (dshStart, string, []string, map[string]string, error) {
	permission := a.PermissionMode
	if permission == "" {
		permission = "read-only"
	}
	if strings.TrimSpace(a.Provider) == "" || strings.TrimSpace(a.Model) == "" || (a.SessionID != "" && !a.PersistentSession) ||
		(a.RequestID != "" && (strings.TrimSpace(a.RequestID) == "" || strings.ContainsAny(a.RequestID, "\x00\r\n"))) ||
		(a.PersistentSession && (strings.TrimSpace(a.RequestID) == "" || a.Doctor || strings.ContainsAny(a.RequestID+a.SessionID, "\x00\r\n"))) ||
		(permission != "read-only" && permission != "workspace-write" && permission != "danger-full-access") {
		return dshStart{}, "", nil, nil, fmt.Errorf("DSH requires provider, model, supported permissions, and an explicit scoped request for history")
	}
	cwd, err := filepath.Abs(a.Cwd)
	if err == nil {
		cwd, err = filepath.EvalSymlinks(cwd)
	}
	if err != nil {
		return dshStart{}, "", nil, nil, fmt.Errorf("DSH cwd cannot be resolved: %w", err)
	}
	project := a.WorkspaceCwd
	if project != "" {
		project, err = filepath.Abs(project)
		if err == nil {
			project, err = filepath.EvalSymlinks(project)
		}
		if err != nil {
			return dshStart{}, "", nil, nil, fmt.Errorf("DSH project workspace cannot be resolved: %w", err)
		}
	}
	effort := a.Thinking
	if effort == "" {
		effort = "high"
	}
	request := dshStart{Native: a.Native, Managed: a.Managed, Type: "start", Protocol: 1, RequestID: rand.Text(), Cwd: cwd,
		Provider: a.Provider, Model: a.Model, ReasoningEffort: effort,
		WorkspaceCwd: project, SessionTitle: a.SessionTitle,
		PermissionMode: permission, Prompt: a.Prompt, TimeoutMS: a.Watchdog.WallTimeoutMs}
	if a.RequestID != "" {
		request.RequestID = a.RequestID
	}
	if a.PersistentSession {
		request.Protocol, request.RequestID, request.Persistent, request.SessionID = 2, a.RequestID, true, a.SessionID
	}
	var argv []string
	if a.Doctor {
		argv = []string{"--doctor"}
		// Protocol v1 requires a nonempty admission prompt. The doctor worker
		// consumes this fixed marker locally and skips session/prompt entirely.
		request.Prompt = "Flywheel SDK readiness probe; no model prompt is authorized."
	}
	command := a.Command
	if command == "" {
		command = DSHCommand()
	}
	env := childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)
	for _, key := range []string{"FLYWHEEL_DSH_BIN", "FLYWHEEL_DSH_PROFILE", "DSH_HOME", "FLYWHEEL_RESOURCE_DIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	return request, command, argv, env, nil
}

func RunDSH(a DSHArgs) Result {
	if a.Prewarm != nil {
		return a.Prewarm.Run(a)
	}
	request, command, argv, env, err := dshCall(a)
	if err != nil {
		return invalidDSHSession(a, err.Error())
	}
	var onActivity func(chunk, stream string)
	events := jsonLines{onLine: a.OnEvent, limit: watchdog.MaxBuffer}
	if a.OnEvent != nil {
		onActivity = events.feed
	}
	written := make(chan error, 1)
	result := watchdog.Run(command, argv, watchdog.Opts{
		Context: a.Watchdog.Context, Cwd: request.Cwd, Env: childenv.EnvSlice(env),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs, WallTimeoutMs: a.Watchdog.WallTimeoutMs,
		KillGraceMs: dshCleanupGrace(a), OnActivity: onActivity,
		InteractStdin: func(stdin io.WriteCloser, finish func()) {
			err := json.NewEncoder(stdin).Encode(request)
			_ = stdin.Close()
			written <- err
			if err != nil {
				finish()
			}
		},
	})
	if a.OnEvent != nil {
		events.flush()
	}
	var writeError error
	select {
	case writeError = <-written:
	default: // no stdin writer when spawn fails or cancellation precedes Start
	}
	return finishDSH(a, request, result, writeError)
}

func finishDSH(a DSHArgs, request dshStart, result watchdog.Result, writeError error) Result {
	parsed := parseDSHStream(result.Stdout, request)
	if writeError != nil {
		parsed.Error = "DSH worker request write failed: " + writeError.Error()
		parsed.ErrorCode = errs.TRANSPORT
		parsed.Session = nil
		parsed.Usage, parsed.CostUSD = nil, nil
	}
	out := fromWatchdog(result, "dsh", a.Provider, a.Model)
	out.NativeReceipt = parsed.NativeReceipt
	out.ErrorCode = parsed.ErrorCode
	if out.CleanupError == "" {
		out.CleanupError = parsed.CleanupError
	}
	if (result.ExitCode != nil || result.Signal != "") && !parsed.CleanupConfirmed && out.CleanupError == "" {
		// Reaping the outer worker does not attest the SDK shutdown contract.
		// Preserve unknown cleanup so the engine stops instead of overlapping calls.
		out.CleanupError = "DSH worker did not confirm SDK process cleanup: no trustworthy terminal receipt"
	}
	out.SessionID, out.Text, out.Usage, out.CostUSD = parsed.SessionID, parsed.Text, parsed.Usage, parsed.CostUSD
	if out.Truncated {
		// A complete-looking prefix cannot attest the omitted tail, even when
		// only stderr exceeded the capture bound. Its charge is unconfirmed.
		out.Usage, out.CostUSD = nil, nil
	}
	out.Cancelled = out.Cancelled || parsed.Cancelled
	if out.ErrorCode != "" && out.ExitCode != nil && *out.ExitCode != 1 && !out.TimedOut && !out.Cancelled &&
		out.Signal == "" && out.SpawnError == "" && out.CleanupError == "" && !out.Truncated {
		parsed.Error = fmt.Sprintf("DSH worker exit code %d did not match the expected failure exit 1: %s", *out.ExitCode, parsed.Error)
		out.ErrorCode = errs.TRANSPORT
	}
	out.Settled = parsed.Settled && parsed.Error == "" && !out.Cancelled && !out.TimedOut && !out.Truncated &&
		out.Signal == "" && out.SpawnError == "" && out.CleanupError == "" && out.ExitCode != nil && *out.ExitCode == 0
	if !out.Settled && !out.Cancelled {
		if out.ErrorCode == "" {
			// A successful protocol receipt cannot sanitize a failing process.
			// Keep logs as diagnostics instead of reclassifying their prose.
			out.ErrorCode = errs.TRANSPORT
		}
		message := parsed.Error
		switch {
		case out.TimedOut:
			message = "DSH worker timed out"
		case out.SpawnError != "":
			message = out.SpawnError
		case out.Truncated:
			message = "DSH worker output was truncated"
		case out.CleanupError != "":
			message = strings.TrimPrefix(message+"; "+out.CleanupError, "; ")
		case message == "":
			message = "DSH worker exited without successful process completion: " + strings.TrimSpace(out.Stderr)
		}
		out.ErrorMessage = &message
	}
	if a.PersistentSession {
		out.Session = parsed.Session
		// A checked worker receipt still requires the expected physical exit.
		// Timeout, cancellation or a damaged outer stream cannot be normalized
		// into a safe replay decision by a successful inner turn.
		if out.Session == nil || out.TimedOut || out.Cancelled || out.Truncated || out.Signal != "" || out.CleanupError != "" ||
			out.ExitCode == nil || (parsed.Settled && *out.ExitCode != 0) || (!parsed.Settled && *out.ExitCode != 1) {
			out.Session = &SessionOutcome{Protocol: DSHSessionProtocol, RequestID: a.RequestID, SessionID: a.SessionID,
				Dispatch: "unknown", Outcome: "unknown", UsageStatus: "unknown"}
			if out.SpawnError != "" || (out.Cancelled && out.ExitCode == nil && out.Signal == "") {
				out.Session.Dispatch, out.Session.Outcome, out.Session.CleanupConfirmed = "not-dispatched", "failed", true
				out.Usage, out.CostUSD = &Usage{}, new(float64)
			}
		}
	} else if !a.Doctor {
		// v1 has no separate dispatch/turn receipt on its error path. Its
		// successful checked result still proves completion of this correlated
		// request; arbitrary error codes/prose never prove non-dispatch.
		out.Session = &SessionOutcome{Protocol: DSHFreshSessionProtocol, RequestID: request.RequestID,
			SessionID: parsed.SessionID, Dispatch: "unknown", Outcome: "unknown",
			CleanupConfirmed: parsed.CleanupConfirmed && out.CleanupError == "", UsageStatus: "unknown"}
		if out.Settled {
			out.Session.Dispatch, out.Session.Outcome = "dispatched", "completed"
		} else if out.SpawnError != "" || (out.Cancelled && out.ExitCode == nil && out.Signal == "") {
			out.Session.Dispatch, out.Session.Outcome, out.Session.CleanupConfirmed = "not-dispatched", "failed", true
			out.Usage, out.CostUSD = &Usage{}, new(float64)
		}
	}
	// An intact correlated native receipt can still measure usage when outer
	// process cleanup/exit is unsafe. Accounting certainty is separate from replay.
	if out.Session != nil && out.Usage != nil {
		out.Session.UsageStatus = "complete"
	}
	return out
}

func invalidDSHSession(a DSHArgs, detail string) Result {
	out := invalidHarnessResult("dsh", a.Model, detail)
	out.Session = &SessionOutcome{Protocol: DSHFreshSessionProtocol, RequestID: a.RequestID,
		Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true, UsageStatus: "unknown"}
	if a.PersistentSession {
		out.Session.Protocol = DSHSessionProtocol
	}
	return out
}
