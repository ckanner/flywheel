// DSH worker adapter：Go 拥有超时/取消和确定性验收，Node worker 用公开 SDK
// 开 fresh session。协议 v1 不恢复会话，不接受隐式 provider/model/权限降级。
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
	"github.com/ckanner/flywheel/internal/watchdog"
)

const DSHWorkerEnv = "FLYWHEEL_DSH_WORKER"

// DSHArgs binds one fresh SDK worker to one provider/model and permission scope.
type DSHArgs struct {
	Prompt         string
	Doctor         bool // local SDK route/policy/shutdown probe; never submits a model prompt
	Provider       string
	Model          string
	Thinking       string
	PermissionMode string
	SessionID      string // v1 rejects nonempty values instead of silently cold-starting
	Cwd            string
	Command        string // executable path; empty => FLYWHEEL_DSH_WORKER or flywheel-dsh-worker
	APIKeys        map[string]string
	Watchdog       watchdog.Opts
	OnEvent        func(line string)
}

// DSHCommand is the configured worker executable, shared with CLI diagnostics.
func DSHCommand() string {
	if command := os.Getenv(DSHWorkerEnv); command != "" {
		return command
	}
	return "flywheel-dsh-worker"
}

type dshStart struct {
	Type            string `json:"type"`
	Protocol        int    `json:"protocol"`
	RequestID       string `json:"requestId"`
	Cwd             string `json:"cwd"`
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort"`
	PermissionMode  string `json:"permissionMode"`
	Prompt          string `json:"prompt"`
	TimeoutMS       int64  `json:"timeoutMs,omitempty"`
}

type dshParsed struct {
	SessionID string
	Text      string
	Usage     *Usage
	CostUSD   *float64
	Settled   bool
	Cancelled bool
	Error     string
}

// parseDSHStream validates the worker's attested effective settings and terminal
// state. Unknown frames or a missing receipt cannot be treated as a successful
// call. Raw durable DSH events remain in stdout for audit, not as completion.
func parseDSHStream(stdout string, request dshStart) dshParsed {
	var parsed dshParsed
	ready, terminal := false, false
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var frame struct {
			Type            string          `json:"type"`
			Protocol        int             `json:"protocol"`
			RequestID       string          `json:"requestId"`
			SessionID       string          `json:"sessionId"`
			Cwd             string          `json:"cwd"`
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
			Error           *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
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
			if ready || frame.SessionID == "" || frame.Cwd != request.Cwd || frame.Provider != request.Provider ||
				frame.Model != request.Model || frame.ReasoningEffort != request.ReasoningEffort ||
				frame.PermissionMode != request.PermissionMode || frame.ApprovalPolicy != "never" {
				parsed.Error = "DSH worker effective settings mismatch"
				break
			}
			ready = true
			parsed.SessionID = frame.SessionID
		case "event":
			if !ready || frame.SessionID != parsed.SessionID || len(frame.Event) == 0 || string(frame.Event) == "null" {
				parsed.Error = "DSH worker event arrived without a matching ready session"
			}
		case "result":
			terminal = true
			// Startup failures may precede ready. Success always requires the receipt.
			if frame.Status == "error" && frame.Error != nil {
				parsed.Error = frame.Error.Code + ": " + frame.Error.Message
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
func RunDSH(a DSHArgs) Result {
	permission := a.PermissionMode
	if permission == "" {
		permission = "read-only"
	}
	if strings.TrimSpace(a.Provider) == "" || strings.TrimSpace(a.Model) == "" || a.SessionID != "" ||
		(permission != "read-only" && permission != "workspace-write") {
		return invalidHarnessResult("dsh", a.Model, "DSH requires provider, model, supported permissions, and a fresh session")
	}
	cwd, err := filepath.Abs(a.Cwd)
	if err == nil {
		cwd, err = filepath.EvalSymlinks(cwd)
	}
	if err != nil {
		return invalidHarnessResult("dsh", a.Model, "DSH cwd cannot be resolved: "+err.Error())
	}
	effort := a.Thinking
	if effort == "" {
		effort = "high"
	}
	request := dshStart{Type: "start", Protocol: 1, RequestID: rand.Text(), Cwd: cwd,
		Provider: a.Provider, Model: a.Model, ReasoningEffort: effort,
		PermissionMode: permission, Prompt: a.Prompt, TimeoutMS: a.Watchdog.WallTimeoutMs}
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
	for _, key := range []string{"FLYWHEEL_DSH_BIN", "FLYWHEEL_DSH_PROFILE", "DSH_HOME"} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	var onActivity func(chunk, stream string)
	events := jsonLines{onLine: a.OnEvent, limit: watchdog.MaxBuffer}
	if a.OnEvent != nil {
		onActivity = events.feed
	}
	written := make(chan error, 1)
	result := watchdog.Run(command, argv, watchdog.Opts{
		Context: a.Watchdog.Context, Cwd: cwd, Env: childenv.EnvSlice(env),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs, WallTimeoutMs: a.Watchdog.WallTimeoutMs,
		KillGraceMs: a.Watchdog.KillGraceMs, OnActivity: onActivity,
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
	parsed := parseDSHStream(result.Stdout, request)
	select {
	case err := <-written:
		if err != nil {
			parsed.Error = "DSH worker request write failed: " + err.Error()
		}
	default: // no stdin writer when spawn fails or cancellation precedes Start
	}
	out := fromWatchdog(result, "dsh", a.Provider, a.Model)
	out.SessionID, out.Text, out.Usage, out.CostUSD = parsed.SessionID, parsed.Text, parsed.Usage, parsed.CostUSD
	out.Cancelled = out.Cancelled || parsed.Cancelled
	out.Settled = parsed.Settled && parsed.Error == "" && !out.Cancelled && !out.TimedOut && !out.Truncated &&
		out.SpawnError == "" && out.CleanupError == "" && out.ExitCode != nil && *out.ExitCode == 0
	if !out.Settled && !out.Cancelled {
		message := parsed.Error
		switch {
		case out.TimedOut:
			message = "DSH worker timed out"
		case out.SpawnError != "":
			message = out.SpawnError
		case out.CleanupError != "":
			message = out.CleanupError
		case out.Truncated:
			message = "DSH worker output was truncated"
		case message == "":
			message = "DSH worker exited without successful process completion: " + strings.TrimSpace(out.Stderr)
		}
		out.ErrorMessage = &message
	}
	return out
}
