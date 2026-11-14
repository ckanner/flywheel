package drivers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
)

const KimiNativeWorkerEnv = "FLYWHEEL_KIMI_NATIVE_WORKER"
const KimiNativeProtocol = "kimi-server/v1"

func KimiNativeCommand() string {
	if command := os.Getenv(KimiNativeWorkerEnv); command != "" {
		return command
	}
	return "flywheel-kimi-native-worker"
}

type kimiNativeStart struct {
	Type            string           `json:"type"`
	Protocol        int              `json:"protocol"`
	RequestID       string           `json:"requestId"`
	Cwd             string           `json:"cwd"`
	WorkspaceCwd    string           `json:"workspaceCwd,omitempty"`
	Model           string           `json:"model"`
	ReasoningEffort string           `json:"reasoningEffort"`
	PermissionMode  string           `json:"permissionMode"`
	Prompt          string           `json:"prompt"`
	TimeoutMS       int64            `json:"timeoutMs"`
	SessionID       string           `json:"sessionId,omitempty"`
	Persistent      bool             `json:"persistent,omitempty"`
	Native          json.RawMessage  `json:"native"`
	Managed         *NativeAdmission `json:"managed,omitempty"`
}

// The native server worker owns one authenticated local runtime. Its whole
// stream, capability admission and lifecycle must agree before Go admits it.
func parseKimiNativeStream(stdout string, request kimiNativeStart) (Result, error) {
	out := Result{Session: &SessionOutcome{Protocol: KimiNativeProtocol, RequestID: request.RequestID,
		Dispatch: "unknown", Outcome: "unknown", UsageStatus: "unknown"}}
	ready, terminal, sessionID := false, false, ""
	for _, line := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var frame struct {
			Type      string          `json:"type"`
			Protocol  int             `json:"protocol"`
			RequestID string          `json:"requestId"`
			SessionID string          `json:"sessionId"`
			Status    string          `json:"status"`
			Settled   *bool           `json:"settled"`
			Text      string          `json:"text"`
			Usage     json.RawMessage `json:"usage"`
			Native    json.RawMessage `json:"native"`
			Event     json.RawMessage `json:"event"`
			Lifecycle *SessionOutcome `json:"lifecycle"`
			Error     *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(line), &frame) != nil || frame.Protocol != 1 || frame.RequestID != request.RequestID || terminal {
			return out, fmt.Errorf("Kimi native worker emitted a damaged or uncorrelated stream")
		}
		switch frame.Type {
		case "ready":
			if ready || frame.SessionID == "" || (request.SessionID != "" && frame.SessionID != request.SessionID) {
				return out, fmt.Errorf("Kimi native worker ready identity mismatch")
			}
			ready, sessionID = true, frame.SessionID
		case "event":
			if !ready || frame.SessionID != sessionID || len(frame.Event) == 0 || string(frame.Event) == "null" {
				return out, fmt.Errorf("Kimi native event is outside the admitted session")
			}
		case "result":
			terminal = true
			if frame.Settled == nil || (frame.Status != "ok" && frame.Status != "error" && frame.Status != "cancelled") ||
				(ready && frame.SessionID != sessionID) || (frame.Status == "ok" && (!ready || !*frame.Settled || frame.Error != nil || strings.TrimSpace(frame.Text) == "")) {
				return out, fmt.Errorf("Kimi native terminal result is incomplete or contradictory")
			}
			if err := validateNativeReceipt(request.Native, frame.Native, frame.Status == "ok", *frame.Settled); err != nil {
				return out, err
			}
			lifecycle := frame.Lifecycle
			if lifecycle == nil || lifecycle.Protocol != KimiNativeProtocol || lifecycle.RequestID != request.RequestID || lifecycle.SessionID != frame.SessionID {
				return out, fmt.Errorf("Kimi native lifecycle identity mismatch")
			}
			// The turn semantics match the established scoped-session contract;
			// validate a copy so the original backend protocol stays in evidence.
			check := *lifecycle
			check.Protocol = DSHSessionProtocol
			if err := validateDSHLifecycle(&check, dshStart{RequestID: request.RequestID, SessionID: request.SessionID, Persistent: request.Persistent}, frame.SessionID, *frame.Settled, ready, frame.Status); err != nil {
				return out, fmt.Errorf("Kimi native lifecycle: %w", err)
			}
			usage := parseKimiUsage(frame.Usage)
			if (lifecycle.UsageStatus == "complete") != (usage != nil) {
				return out, fmt.Errorf("Kimi native usage certainty mismatch")
			}
			out.Session, out.NativeReceipt, out.Usage, out.Text, out.SessionID = lifecycle, frame.Native, usage, frame.Text, frame.SessionID
			out.Settled, out.Cancelled = frame.Status == "ok", frame.Status == "cancelled"
			if frame.Error != nil {
				message := frame.Error.Code + ": " + frame.Error.Message
				out.ErrorMessage = &message
			}
		default:
			return out, fmt.Errorf("Kimi native worker emitted an unknown frame")
		}
	}
	if !terminal {
		return out, fmt.Errorf("Kimi native worker ended without a terminal receipt")
	}
	return out, nil
}

func runKimiNative(a KimiArgs) Result {
	r := kimiNativeStart{Type: "start", Protocol: 1, RequestID: a.RequestID, Cwd: a.Cwd, WorkspaceCwd: a.WorkspaceCwd,
		Model: a.Model, ReasoningEffort: a.Thinking, PermissionMode: a.PermissionMode, Prompt: a.Prompt,
		SessionID: a.SessionID, Persistent: a.PersistentSession, Native: a.Native, Managed: a.Managed, TimeoutMS: a.Watchdog.WallTimeoutMs}
	if r.RequestID == "" {
		r.RequestID = "native-" + rand.Text()
	}
	if r.Model == "" {
		r.Model = KimiDefaultModel
	}
	if r.ReasoningEffort == "" {
		r.ReasoningEffort = "high"
	}
	if r.TimeoutMS <= 0 {
		r.TimeoutMS = watchdog.DefaultWallTimeoutMs
	}
	reject := func(detail string) Result {
		out := invalidHarnessResult("kimi", r.Model, detail)
		out.Session = &SessionOutcome{Protocol: KimiNativeProtocol, RequestID: r.RequestID, Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true, UsageStatus: "unknown"}
		return out
	}
	if r.PermissionMode != "danger-full-access" || strings.TrimSpace(r.Prompt) == "" || strings.TrimSpace(r.RequestID) == "" ||
		strings.ContainsAny(r.RequestID+r.SessionID, "\x00\r\n") || (r.SessionID != "" && !r.Persistent) {
		return reject("Kimi native requires an explicit full-access binding, prompt and valid invocation identity; resume requires persistent history")
	}
	cwd, err := filepath.Abs(r.Cwd)
	if err == nil {
		cwd, err = filepath.EvalSymlinks(cwd)
	}
	if err != nil {
		return reject("Kimi native cwd cannot be resolved: " + err.Error())
	}
	r.Cwd = cwd
	payload, err := json.Marshal(r)
	if err != nil || len(payload) >= claudeCodeProRequestBytes {
		return reject("Kimi native request is invalid or exceeds the 1 MiB limit")
	}
	payload = append(payload, '\n')
	env := childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)
	for _, key := range []string{KimiCommandEnv, "KIMI_CODE_HOME", "FLYWHEEL_RESOURCE_DIR"} {
		if value, exists := os.LookupEnv(key); exists {
			env[key] = value
		}
	}
	if a.Command != "" {
		env[KimiCommandEnv] = a.Command
	}
	result, writeErr := runMachineWorker(KimiNativeCommand(), "kimi", codexProStart{Protocol: 1}, payload, watchdog.Opts{
		Context: a.Watchdog.Context, Cwd: cwd, Env: childenv.EnvSlice(env), IdleTimeoutMs: a.Watchdog.IdleTimeoutMs,
		WallTimeoutMs: r.TimeoutMS, KillGraceMs: max(a.Watchdog.KillGraceMs, 15000),
	}, nil, a.OnEvent)
	return finishKimiNative(r, result, writeErr)
}

func finishKimiNative(request kimiNativeStart, result watchdog.Result, writeErr error) Result {
	parsed, parseErr := parseKimiNativeStream(result.Stdout, request)
	out := fromWatchdog(result, "kimi", "kimi", request.Model)
	out.Session, out.NativeReceipt, out.Text, out.Usage, out.SessionID = parsed.Session, parsed.NativeReceipt, parsed.Text, parsed.Usage, parsed.SessionID
	out.Cancelled = out.Cancelled || parsed.Cancelled
	out.ErrorMessage = parsed.ErrorMessage
	valid := parseErr == nil && writeErr == nil && !out.Truncated
	if !valid {
		out.Usage, out.Session = nil, &SessionOutcome{Protocol: KimiNativeProtocol, RequestID: request.RequestID, Dispatch: "unknown", Outcome: "unknown", UsageStatus: "unknown"}
		message := fmt.Sprintf("Kimi native transport was not trustworthy: parse=%v write=%v truncated=%v", parseErr, writeErr, out.Truncated)
		out.ErrorMessage = &message
	}
	if (out.ExitCode != nil || out.Signal != "") && !out.Session.CleanupConfirmed && out.CleanupError == "" {
		out.CleanupError = "Kimi native process cleanup unconfirmed"
	}
	out.Settled = parsed.Settled && valid && !out.Cancelled && !out.TimedOut && out.Signal == "" && out.CleanupError == "" && out.SpawnError == "" && out.ExitCode != nil && *out.ExitCode == 0
	if out.SpawnError != "" || (out.Cancelled && out.ExitCode == nil && out.Signal == "") {
		out.Session.Dispatch, out.Session.Outcome, out.Session.CleanupConfirmed = "not-dispatched", "failed", true
		out.Usage, out.CostUSD, out.Session.UsageStatus = &Usage{}, new(float64), "complete"
	} else if out.TimedOut || out.Cancelled || out.Signal != "" || out.CleanupError != "" || out.ExitCode == nil || (parsed.Settled && *out.ExitCode != 0) {
		out.Session.Outcome, out.Session.ResumeCandidate = "unknown", false
		out.Session.CleanupConfirmed = out.Session.CleanupConfirmed && out.CleanupError == ""
	}
	if !out.Settled && !out.Cancelled {
		out.ErrorCode = errs.TRANSPORT
		if out.ErrorMessage == nil {
			message := "Kimi native worker did not complete successfully"
			out.ErrorMessage = &message
		}
	}
	return out
}
