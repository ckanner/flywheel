// Codex Pro uses the plugin's public machine API through the bundled worker.
// Go owns the outer lifecycle and admits a result only after both layers settle.
package drivers

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/ckanner/flywheel/internal/watchdog"
)

const (
	CodexProWorkerEnv      = "FLYWHEEL_CODEX_PRO_WORKER"
	CodexProBinaryEnv      = "FLYWHEEL_CODEX_BIN"
	CodexProPackageVersion = "0.10.2"
	codexProCleanupMS      = int64(10_000)
	machineRequestBytes    = 1 << 20 // non-Claude worker JSONL frame limit
)

// CodexProCommand is shared with the local installation diagnostics.
func CodexProCommand() string {
	if command := os.Getenv(CodexProWorkerEnv); command != "" {
		return command
	}
	return "flywheel-codex-pro-worker"
}

// CodexProBinary identifies the app-server executable consumed by the plugin.
func CodexProBinary() string {
	if command := os.Getenv(CodexProBinaryEnv); command != "" {
		return command
	}
	return "codex"
}

// CodexProArgs admits one explicitly configured public machine run. Scoped
// calls request persistent history; SessionID resumes only that exact thread.
type CodexProArgs struct {
	Subscription      *config.SubscriptionPolicy
	Native            json.RawMessage
	Managed           *NativeAdmission
	Prompt            string
	Model             string
	ReasoningEffort   string
	PermissionMode    string
	SessionID         string
	SessionTitle      string
	PersistentSession bool
	RequestID         string
	Cwd               string
	Command           string
	APIKeys           map[string]string
	Watchdog          watchdog.Opts
	OnEvent           func(line string)
	RecoveryDirectory string
	OnLifecycle       func(MachineLifecycle) error
}

type codexProStart struct {
	Subscription       *config.SubscriptionPolicy `json:"subscription,omitempty"`
	EvidenceOnly       bool                       `json:"evidenceOnly,omitempty"`
	Images             []imageinput.Image         `json:"images,omitempty"`
	InputImageManifest *imageinput.Manifest       `json:"-"` // receipt verification only; never a recovery prompt
	Native             json.RawMessage            `json:"native,omitempty"`
	Managed            *NativeAdmission           `json:"managed,omitempty"`
	Type               string                     `json:"type"`
	Protocol           int                        `json:"protocol"`
	RequestID          string                     `json:"requestId"`
	Cwd                string                     `json:"cwd"`
	Model              string                     `json:"model"`
	ReasoningEffort    string                     `json:"reasoningEffort"`
	PermissionMode     string                     `json:"permissionMode"`
	Prompt             string                     `json:"prompt"`
	TimeoutMS          int64                      `json:"timeoutMs"`
	IdleTimeoutMS      int64                      `json:"idleTimeoutMs,omitempty"`
	SessionTitle       string                     `json:"sessionTitle,omitempty"`
	ThreadMode         string                     `json:"threadMode,omitempty"`
	ThreadID           string                     `json:"threadId,omitempty"`
	RecoveryDirectory  string                     `json:"recoveryDirectory,omitempty"`
}

type codexProParsed struct {
	NativeReceipt    json.RawMessage
	Receipt          json.RawMessage
	SessionID        string
	Text             string
	Usage            *Usage
	Settled          bool
	CleanupConfirmed bool
	Cancelled        bool
	TimedOut         bool
	Truncated        bool
	StreamFailed     bool // worker protocol/envelope failure, distinct from a provider cancellation receipt
	CleanupError     string
	Error            string
	Session          *SessionOutcome
}

type codexProFailureFacts struct {
	Stage      string `json:"stage"`
	Category   string `json:"category"`
	HTTPStatus int    `json:"httpStatus"`
}

type codexProExecution struct {
	RunID                string                `json:"runId"`
	ThreadID             string                `json:"threadId"`
	TurnID               string                `json:"turnId"`
	Ownership            string                `json:"ownership"`
	Dispatch             string                `json:"dispatch"`
	PID                  *int                  `json:"pid"`
	SupervisorPID        *int                  `json:"supervisorPid"`
	ExitCode             *int                  `json:"exitCode"`
	Signal               *string               `json:"signal"`
	SupervisorExitCode   *int                  `json:"supervisorExitCode"`
	SupervisorSignal     *string               `json:"supervisorSignal"`
	TerminationRequested *bool                 `json:"terminationRequested"`
	TimedOut             *bool                 `json:"timedOut"`
	Cancelled            *bool                 `json:"cancelled"`
	Truncated            *bool                 `json:"truncated"`
	SpawnError           json.RawMessage       `json:"spawnError"`
	Failure              *codexProFailureFacts `json:"failure"`
}

// The public receipt distinguishes an explicit unknown null from an omitted
// physical fact. A fixed-version worker cannot silently drop these facts.
func (e *codexProExecution) UnmarshalJSON(raw []byte) error {
	type execution codexProExecution
	var value execution
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if !receiptHasFields(raw, "runId", "startedAt", "finishedAt", "durationMs", "threadId", "turnId", "pid", "supervisorPid",
		"exitCode", "signal", "supervisorExitCode", "supervisorSignal", "terminationRequested", "timedOut", "cancelled", "truncated", "spawnError", "failure", "ownership") {
		return fmt.Errorf("Codex Pro execution omitted a physical fact")
	}
	*e = codexProExecution(value)
	return nil
}

type codexProCleanup struct {
	Confirmed bool   `json:"confirmed"`
	Detail    string `json:"detail"`
}

type codexProSettings struct {
	Model          string          `json:"model"`
	Cwd            string          `json:"cwd"`
	ApprovalPolicy string          `json:"approvalPolicy"`
	Sandbox        string          `json:"sandbox"`
	SandboxPolicy  json.RawMessage `json:"sandboxPolicy"`
}

type codexProConfiguration struct {
	Source     string           `json:"source"`
	Requested  codexProSettings `json:"requested"`
	Effective  codexProSettings `json:"effective"`
	Matched    bool             `json:"matched"`
	Missing    *[]string        `json:"missing"`
	Mismatches *[]string        `json:"mismatches"`
}

type codexProTurnConfiguration struct {
	RequestedEffort string          `json:"requestedEffort"`
	EffectiveEffort json.RawMessage `json:"effectiveEffort"`
	Evidence        string          `json:"evidence"`
	Accepted        bool            `json:"accepted"`
}

type codexProOutcome struct {
	Output []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"output"`
	StopReason string          `json:"stopReason"`
	Diagnostic string          `json:"diagnostic"`
	ThreadID   string          `json:"threadId"`
	Usage      json.RawMessage `json:"usage"`
	RateLimits *struct {
		Observations *int64 `json:"observations"`
	} `json:"rateLimits"`
	Configuration     *codexProConfiguration     `json:"configuration"`
	TurnConfiguration *codexProTurnConfiguration `json:"turnConfiguration"`
	Cleanup           *codexProCleanup           `json:"cleanup"`
	Execution         *codexProExecution         `json:"execution"`
}

type codexProFailure struct {
	Stage             string                     `json:"stage"`
	Category          string                     `json:"category"`
	Diagnostic        string                     `json:"diagnostic"`
	Usage             json.RawMessage            `json:"usage"`
	Cleanup           *codexProCleanup           `json:"cleanup"`
	Execution         *codexProExecution         `json:"execution"`
	Configuration     *codexProConfiguration     `json:"configuration"`
	TurnConfiguration *codexProTurnConfiguration `json:"turnConfiguration"`
}

type codexProReceipt struct {
	Native    json.RawMessage `json:"native"`
	Protocol  int             `json:"protocol"`
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	RunID     string          `json:"runId"`
	OK        *bool           `json:"ok"`
	Requested struct {
		Cwd            string          `json:"cwd"`
		Model          string          `json:"model"`
		Effort         string          `json:"effort"`
		PermissionMode string          `json:"permissionMode"`
		ThreadID       json.RawMessage `json:"threadId"`
	} `json:"requested"`
	Outcome    *codexProOutcome `json:"outcome"`
	Failure    *codexProFailure `json:"failure"`
	CostUSD    json.RawMessage  `json:"costUSD"`
	CostStatus string           `json:"costStatus"`
}

// parseCodexProStream keeps the original receipt even when admission fails.
// Progress is activity, never evidence of completion or configuration acceptance.
func parseCodexProStream(stdout string, request codexProStart) codexProParsed {
	parsed := codexProParsed{Session: codexProUnknownSession(request)}
	lifecycle := machineLifecycleFlow{harness: "codex-pro", request: request}
	terminal := false
	trustedStream := true
	var nativeSequence, activitySequence int64
	for _, line := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var frame struct {
			Type      string  `json:"type"`
			Protocol  int     `json:"protocol"`
			RequestID string  `json:"requestId"`
			Text      *string `json:"text"`
			Adapter   struct {
				Package  string `json:"package"`
				Version  string `json:"version"`
				Protocol int    `json:"protocol"`
			} `json:"adapter"`
			Receipt json.RawMessage `json:"receipt"`
			Error   *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			trustedStream = false
			parsed.CleanupConfirmed = false
			parsed.Error = "invalid request: Codex Pro worker emitted malformed JSONL"
			break
		}
		if frame.Protocol != request.Protocol || frame.RequestID != request.RequestID || terminal {
			trustedStream = false
			parsed.CleanupConfirmed = false
			parsed.Error = "invalid request: Codex Pro worker protocol/request mismatch or frame after terminal result"
			break
		}
		switch frame.Type {
		case "activity":
			if err := validateProActivityEvent(json.RawMessage(line), "codex-pro", request, &activitySequence); err != nil {
				trustedStream = false
				parsed.Error = err.Error()
			}
		case "native-event":
			if err := validateProNativeEvent(json.RawMessage(line), "codex-pro", request, &nativeSequence); err != nil {
				trustedStream = false
				parsed.Error = err.Error()
			}
		case "lifecycle":
			if _, err := lifecycle.frame([]byte(line)); err != nil || request.Protocol != 3 {
				trustedStream = false
				parsed.Error = "invalid request: Codex Pro lifecycle evidence is invalid"
			}
		case "progress":
			if frame.Text == nil {
				trustedStream = false
				parsed.Error = "invalid request: Codex Pro progress is missing text"
			}
		case "result":
			terminal = true
			parsed = parseCodexProReceipt(frame.Receipt, request)
			if !receiptHasFields(json.RawMessage(line), "receipt", "error") {
				trustedStream = false
				parsed.CleanupConfirmed = false
				parsed.Error = "invalid request: Codex Pro worker omitted a terminal receipt/error field"
			}
			if frame.Adapter.Package != "dsh-subagent-codex-pro" || frame.Adapter.Version != CodexProPackageVersion || frame.Adapter.Protocol != 1 {
				trustedStream = false
				parsed.CleanupConfirmed = false
				parsed.Error = "invalid request: Codex Pro adapter package/version/protocol mismatch"
			}
			if request.Protocol == 3 && parsed.Settled && !machineLifecycleProvesResult(lifecycle.last, parsed.Session) {
				trustedStream = false
				parsed.Error = "invalid request: Codex Pro result omitted durable lifecycle evidence"
			}
			if frame.Error != nil {
				if !machineWorkerObservationError(frame.Error.Code) {
					trustedStream = false
				}
				// A worker observation failure forbids applying/resuming this
				// result, but does not erase independently validated cleanup.
				parsed.Cancelled, parsed.TimedOut, parsed.Truncated, parsed.Settled = false, false, false, false
				parsed.Session.Outcome, parsed.Session.ResumeCandidate = "unknown", false
				parsed.Error = codexProFailureMessage(frame.Error.Code, frame.Error.Message, "", nil)
			}
		default:
			trustedStream = false
			parsed.CleanupConfirmed = false
			parsed.Error = "invalid request: Codex Pro worker emitted an unknown frame"
		}
		// A failed model result is still a terminal frame. Scan the remaining
		// stream so garbage or a second result cannot retain its cleanup proof.
		if parsed.Error != "" && !terminal {
			break
		}
	}
	if parsed.Error == "" && !terminal {
		parsed.Error = "Codex Pro worker ended without a terminal receipt"
	}
	if !trustedStream {
		parsed.StreamFailed = true
		parsed.CleanupConfirmed = false
		parsed.Cancelled, parsed.TimedOut, parsed.Truncated = false, false, false
		parsed.Session = codexProUnknownSession(request)
		parsed.Usage = nil // retain raw evidence, but do not bill an untrusted stream
	}
	parsed.Session.CleanupConfirmed = parsed.CleanupConfirmed
	parsed.Settled = parsed.Settled && parsed.Error == ""
	return parsed
}

func codexProUnknownSession(request codexProStart) *SessionOutcome {
	return &SessionOutcome{Protocol: fmt.Sprintf("codex-pro/%s/worker-v%d", CodexProPackageVersion, request.Protocol),
		RequestID: request.RequestID, Dispatch: "unknown", Outcome: "unknown", UsageStatus: "unknown"}
}

func parseCodexProReceipt(raw json.RawMessage, request codexProStart) (parsed codexProParsed) {
	parsed = codexProParsed{Receipt: raw, Session: codexProUnknownSession(request)}
	var receipt codexProReceipt
	defer func() {
		parsed.NativeReceipt = receipt.Native
		if receipt.OK == nil {
			return
		}
		fold, err := foldProNative("codex-pro", request.Native, receipt.Native, *receipt.OK, parsed.CleanupConfirmed,
			parsed.Session.Dispatch == "not-dispatched", parsed.Session.SessionID, parsed.Usage, parsed.Session.UsageStatus)
		if err != nil {
			parsed.Error = err.Error()
			parsed.Settled, parsed.CleanupConfirmed, parsed.Usage = false, false, nil
			parsed.Session.Outcome, parsed.Session.UsageStatus, parsed.Session.ResumeCandidate = "unknown", "unknown", false
			return
		}
		parsed.Usage, parsed.Session.UsageStatus, parsed.CleanupConfirmed = fold.Usage, fold.Status, fold.Cleanup
		if !fold.Completion {
			parsed.Settled, parsed.Session.ResumeCandidate, parsed.Session.Outcome = false, false, "unknown"
		}
	}()
	if err := json.Unmarshal(raw, &receipt); err != nil || !receiptHasFields(raw, "outcome", "failure") || receipt.Protocol != 1 || receipt.Type != "result" ||
		receipt.RequestID != request.RequestID || receipt.RunID == "" || receipt.OK == nil ||
		(receipt.Outcome == nil) == (receipt.Failure == nil) ||
		receipt.Requested.Cwd != request.Cwd || receipt.Requested.Model != request.Model ||
		receipt.Requested.Effort != request.ReasoningEffort || receipt.Requested.PermissionMode != codexProMachinePermission(request.PermissionMode) ||
		!requestedSessionMatches(receipt.Requested.ThreadID, request.ThreadID) || string(receipt.CostUSD) != "null" || receipt.CostStatus != "unknown" {
		parsed.Error = "invalid request: Codex Pro receipt identity, requested settings, or cost contract mismatch"
		return parsed
	}
	var execution *codexProExecution
	var cleanup *codexProCleanup
	var usage json.RawMessage
	category, diagnostic, stopReason := "", "", ""
	if receipt.Outcome != nil {
		o := receipt.Outcome
		execution, cleanup, usage = o.Execution, o.Cleanup, o.Usage
		parsed.SessionID = o.ThreadID
		stopReason, diagnostic = o.StopReason, o.Diagnostic
		if len(usage) == 0 || string(usage) == "null" || o.RateLimits == nil || o.RateLimits.Observations == nil || *o.RateLimits.Observations < 0 {
			parsed.Error = "invalid request: Codex Pro outcome omitted usage or rate-limit evidence"
			return parsed
		}
		for _, block := range o.Output {
			if block.Type != "text" {
				parsed.Error = "invalid request: Codex Pro returned an unsupported output block"
				return parsed
			}
			parsed.Text += block.Text
		}
	} else if receipt.Failure != nil {
		f := receipt.Failure
		execution, cleanup, usage = f.Execution, f.Cleanup, f.Usage
		category, diagnostic = f.Category, f.Diagnostic
	}
	executionMatches := execution != nil && execution.RunID == receipt.RunID && execution.Ownership == "supervised-group" &&
		(receipt.Outcome == nil || execution.ThreadID == receipt.Outcome.ThreadID) &&
		(request.ThreadID == "" || execution.ThreadID == request.ThreadID || (receipt.Outcome == nil && execution.ThreadID == ""))
	if execution != nil && !executionMatches {
		parsed.Error = "invalid request: Codex Pro execution identity or ownership mismatch"
		return parsed
	}
	// Failed calls may attest cleanup. This alone never proves a terminal model
	// outcome or permits a scoped or fresh replay.
	parsed.CleanupConfirmed = cleanup != nil && cleanup.Confirmed && executionMatches
	if cleanup != nil && !cleanup.Confirmed {
		parsed.CleanupError = "Codex Pro process cleanup unconfirmed: " + cleanup.Detail
	}
	if len(usage) != 0 && string(usage) != "null" {
		observed, err := parseCodexProUsage(usage)
		if err != nil {
			parsed.Error = err.Error()
			return parsed
		}
		if executionMatches && strings.TrimSpace(execution.RunID) != "" && strings.TrimSpace(execution.ThreadID) != "" &&
			strings.TrimSpace(execution.TurnID) != "" && !strings.ContainsRune(execution.RunID+execution.ThreadID+execution.TurnID, 0) {
			// Completeness alone does not attribute counters to this invocation.
			parsed.Usage = observed
			var report struct {
				Status string `json:"status"`
			}
			_ = json.Unmarshal(usage, &report) // parseCodexProUsage validated this object/status.
			parsed.Session.UsageStatus = report.Status
		}
	}
	if executionMatches {
		parsed.Session.RunID, parsed.Session.TurnID, parsed.Session.SessionID = receipt.RunID, execution.TurnID, execution.ThreadID
	}
	if request.Protocol == 3 && (execution == nil || (execution.Dispatch != "not-dispatched" && execution.Dispatch != "possible" && execution.Dispatch != "accepted") ||
		(*receipt.OK && execution.Dispatch != "accepted")) {
		parsed.Error = "invalid request: Codex Pro durable receipt omitted its typed dispatch fact"
		parsed.Session, parsed.Usage = codexProUnknownSession(request), nil
		return parsed
	}
	if !*receipt.OK {
		// Attribute termination only after the native execution identity and
		// typed dispatch have been checked; foreign receipts are not causes.
		if executionMatches {
			parsed.TimedOut = execution.TimedOut != nil && *execution.TimedOut
			parsed.Cancelled = execution.Cancelled != nil && *execution.Cancelled
			parsed.Truncated = execution.Truncated != nil && *execution.Truncated
		}
		// Only typed pre-turn stages from the pinned API prove non-dispatch.
		// A resume rejection may mean a busy thread; its prose is not evidence
		// that a fresh writer may safely repeat this prompt.
		if f := receipt.Failure; f != nil && parsed.CleanupConfirmed && execution.TurnID == "" &&
			f.TurnConfiguration == nil && execution.Failure != nil && execution.Failure.Stage == f.Stage &&
			(f.Stage == "request" || f.Stage == "initialize" || (f.Stage == "thread-start" && request.ThreadID == "")) {
			parsed.Session.Dispatch, parsed.Session.Outcome = "not-dispatched", "failed"
		}
		if request.Protocol == 3 && parsed.CleanupConfirmed && execution.Dispatch == "not-dispatched" && execution.TurnID == "" {
			parsed.Session.Dispatch, parsed.Session.Outcome, parsed.Session.UsageStatus = "not-dispatched", "failed", "complete"
			parsed.Usage = &Usage{}
		}
		if request.PermissionMode == "read-only" && parsed.CleanupConfirmed && parsed.Truncated &&
			executionMatches && execution.Dispatch == "accepted" &&
			execution.Failure != nil && execution.Failure.Category == "output-limit" &&
			receipt.Outcome != nil && validateCodexProReadyConfiguration(receipt.Outcome.Configuration, request) == nil {
			// Attested read-only sandbox + confirmed process-group cleanup +
			// explicit adapter output-limit: retire the damaged thread and
			// record a concrete failed outcome so the machine layer can verify
			// unchanged candidate/guard and retry cleanly with a fresh session.
			parsed.Session.Dispatch, parsed.Session.Outcome = "dispatched", "failed"
			parsed.Session.ResumeCandidate = false
			parsed.Session.ResumeAccepted = request.ThreadID != ""
		}
		parsed.Error = codexProFailureMessage(category, diagnostic, stopReason, execution)
		return parsed
	}
	if receipt.Outcome == nil || receipt.Failure != nil || cleanup == nil || !cleanup.Confirmed ||
		!executionMatches || stopReason != "completed" || !codexProExecutionValid(execution, receipt.RunID, parsed.SessionID) {
		parsed.Error = "Codex Pro receipt does not prove successful completion and process cleanup"
		return parsed
	}
	if err := validateCodexProConfiguration(receipt.Outcome, request); err != nil {
		parsed.Error = "invalid request: " + err.Error()
		return parsed
	}
	// Terminal execution is independent from an empty/malformed model answer.
	// A retained history handle remains a candidate until an explicit resume accepts it.
	parsed.Session.Dispatch, parsed.Session.Outcome = "dispatched", "completed"
	parsed.Session.ResumeCandidate = request.ThreadMode == "persistent"
	parsed.Session.ResumeAccepted = request.ThreadID != ""
	if strings.TrimSpace(parsed.Text) == "" {
		parsed.Error = "Codex Pro returned an empty response"
		return parsed
	}
	parsed.Settled = true
	return parsed
}

// App-server teardown deliberately sends TERM/KILL after a completed turn.
// That is independent from the caller cancellation and timeout facts.
func codexProExecutionValid(e *codexProExecution, runID, threadID string) bool {
	return e != nil && e.RunID == runID && strings.TrimSpace(threadID) != "" && !strings.ContainsRune(threadID, 0) &&
		e.ThreadID == threadID && strings.TrimSpace(e.TurnID) != "" && !strings.ContainsRune(e.TurnID, 0) &&
		e.Ownership == "supervised-group" && e.PID != nil && *e.PID > 0 && e.SupervisorPID != nil && *e.SupervisorPID > 0 &&
		e.TimedOut != nil && !*e.TimedOut && e.Cancelled != nil && !*e.Cancelled && e.Truncated != nil && !*e.Truncated &&
		e.TerminationRequested != nil && e.Failure == nil && string(e.SpawnError) == "null" &&
		codexProExitValid(e.ExitCode, e.Signal, *e.TerminationRequested) &&
		codexProExitValid(e.SupervisorExitCode, e.SupervisorSignal, *e.TerminationRequested)
}

func codexProExitValid(code *int, signal *string, terminationRequested bool) bool {
	if code != nil && signal != nil {
		return false // Node reports exit status or signal, never both for one exit.
	}
	if code != nil && *code != 0 {
		return false
	}
	if signal != nil {
		return terminationRequested && (*signal == "SIGTERM" || *signal == "SIGKILL")
	}
	return code != nil || terminationRequested
}

func validateCodexProConfiguration(o *codexProOutcome, request codexProStart) error {
	if err := validateCodexProReadyConfiguration(o.Configuration, request); err != nil {
		return err
	}
	t := o.TurnConfiguration
	if t == nil || !t.Accepted || t.RequestedEffort != request.ReasoningEffort || t.Evidence != "turn/start-request" || string(t.EffectiveEffort) != "null" {
		return fmt.Errorf("Codex Pro effort acceptance evidence is missing or contradictory")
	}
	return nil
}

// The lifecycle ready checkpoint runs this check before acknowledging a turn.
// The terminal receipt independently checks the actual accepted turn settings.
func validateCodexProReadyConfiguration(c *codexProConfiguration, request codexProStart) error {
	source := "thread/start"
	if request.ThreadID != "" {
		source = "thread/resume"
	}
	if c == nil || c.Source != source || !c.Matched || c.Missing == nil || len(*c.Missing) != 0 || c.Mismatches == nil || len(*c.Mismatches) != 0 {
		return fmt.Errorf("Codex Pro did not attest the requested start/resume configuration")
	}
	for _, settings := range []codexProSettings{c.Requested, c.Effective} {
		if settings.Model != request.Model || settings.ApprovalPolicy != "never" || settings.Sandbox != request.PermissionMode ||
			!samePhysicalDirectory(settings.Cwd, request.Cwd) {
			return fmt.Errorf("Codex Pro model/cwd/permission receipt differs from request")
		}
	}
	return validateCodexProPolicy(c.Effective.SandboxPolicy, request)
}

// The public machine API names its full-access preset differently from the
// canonical DSH/Codex sandbox mode used in Flywheel requests and configuration.
func codexProMachinePermission(permission string) string {
	if permission == "danger-full-access" {
		return "dangerously-bypass-approvals-and-sandbox"
	}
	return permission
}

// The same complete policy is checked in thread-ready before durable ACK and
// independently in the final receipt. In workspace-write,
// standard temp directories are permitted and extra writable roots must remain
// inside cwd. Explicit full access has its own unconfined policy shape.
func validateCodexProPolicy(raw json.RawMessage, request codexProStart) error {
	var policy struct {
		Type                string    `json:"type"`
		NetworkAccess       *bool     `json:"networkAccess"`
		WritableRoots       *[]string `json:"writableRoots"`
		ExcludeTmpdirEnvVar *bool     `json:"excludeTmpdirEnvVar"`
		ExcludeSlashTmp     *bool     `json:"excludeSlashTmp"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return fmt.Errorf("Codex Pro full sandbox policy is malformed")
	}
	if request.PermissionMode == "danger-full-access" {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields) // the strict object decode above already succeeded
		if policy.Type != "dangerFullAccess" || len(fields) != 1 {
			return fmt.Errorf("Codex Pro full-access policy has a contradictory shape")
		}
		return nil
	}
	if policy.NetworkAccess == nil || *policy.NetworkAccess {
		return fmt.Errorf("Codex Pro full sandbox policy has missing or enabled network access")
	}
	if request.PermissionMode == "read-only" {
		if policy.Type != "readOnly" || policy.WritableRoots != nil || policy.ExcludeTmpdirEnvVar != nil || policy.ExcludeSlashTmp != nil {
			return fmt.Errorf("Codex Pro read-only sandbox policy has a contradictory shape")
		}
		return nil
	}
	if policy.Type != "workspaceWrite" || policy.WritableRoots == nil || policy.ExcludeTmpdirEnvVar == nil || policy.ExcludeSlashTmp == nil {
		return fmt.Errorf("Codex Pro workspace sandbox policy is incomplete")
	}
	for _, root := range *policy.WritableRoots {
		if !filepath.IsAbs(root) {
			return fmt.Errorf("Codex Pro writable root must be absolute")
		}
		physical, err := filepath.EvalSymlinks(root)
		if err != nil || (physical != request.Cwd && !strings.HasPrefix(physical, request.Cwd+string(filepath.Separator))) {
			return fmt.Errorf("Codex Pro writable root escapes the requested workspace")
		}
	}
	return nil
}

func parseCodexProUsage(raw json.RawMessage) (*Usage, error) {
	var report struct {
		Status string            `json:"status"`
		Run    map[string]*int64 `json:"run"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, fmt.Errorf("invalid request: Codex Pro usage is malformed")
	}
	switch report.Status {
	case "partial", "unknown":
		return nil, nil // Kept verbatim in the receipt; never a complete zero bill.
	case "complete":
	default:
		return nil, fmt.Errorf("invalid request: Codex Pro usage completeness is unknown")
	}
	var usage Usage
	for _, field := range []struct {
		name  string
		value *int64
	}{
		{"inputTokens", &usage.Input}, {"outputTokens", &usage.Output},
		{"reasoningOutputTokens", &usage.Reasoning}, {"cachedInputTokens", &usage.CacheRead},
		{"cacheWriteInputTokens", &usage.CacheWrite}, {"totalTokens", &usage.TotalTokens},
	} {
		value := report.Run[field.name]
		if value == nil || *value < 0 || *value > 1<<53-1 {
			return nil, fmt.Errorf("invalid request: Codex Pro complete usage is missing a safe counter: %s", field.name)
		}
		*field.value = *value
	}
	if usage.CacheRead > usage.Input || usage.CacheWrite > usage.Input-usage.CacheRead || usage.Reasoning > usage.Output ||
		usage.TotalTokens != usage.Input+usage.Output {
		return nil, fmt.Errorf("invalid request: Codex Pro usage counters contradict their subsets or total")
	}
	return &usage, nil
}

// Preserve the stable Flywheel taxonomy without treating every access-policy
// failure (which includes sandbox/cyber policy) as bad account credentials.
func codexProFailureMessage(category, diagnostic, stopReason string, execution *codexProExecution) string {
	if execution != nil && execution.Failure != nil {
		category = execution.Failure.Category
		if execution.Failure.HTTPStatus != 0 {
			return fmt.Sprintf("Codex Pro HTTP %d: %s", execution.Failure.HTTPStatus, diagnostic)
		}
	}
	if stopReason == "max-tokens" {
		return "context window exceeded: " + diagnostic
	}
	prefix := "Codex Pro failure: "
	switch category {
	case "INVALID_REQUEST", "invalid-request", "protocol", "access-policy", "product-error",
		"ADAPTER_MISMATCH", "ADAPTER_UNAVAILABLE", "UNSUPPORTED_RUNTIME", "INPUT_LIMIT":
		prefix = "invalid request: "
	case "invalid-result", "EMPTY_RESPONSE":
		prefix = "empty response: "
	case "service", "SERVER":
		prefix = "server error: "
	case "transport", "TRANSPORT":
		prefix = "network error: "
	case "limit", "QUOTA":
		prefix = "quota exceeded: "
	case "RATE_LIMIT":
		prefix = "rate limit: "
	case "AUTH":
		prefix = "authentication failed: "
	case "TIMEOUT":
		prefix = "timed out: "
	}
	return prefix + diagnostic
}

// RunCodexPro writes one request and keeps stdin open as the parent's lifetime
// connection. Parent death closes the pipe and makes the worker abort/await the
// plugin, while the plugin's own guardian handles worker death independently.
func RunCodexPro(a CodexProArgs) Result {
	request := codexProStart{Subscription: a.Subscription, Native: a.Native, Managed: a.Managed, Type: "start", Protocol: 1, RequestID: a.RequestID}
	if request.RequestID == "" {
		request.RequestID = rand.Text()
	}
	if a.PersistentSession {
		request.Protocol, request.ThreadMode, request.ThreadID = 2, "persistent", a.SessionID
	}

	if a.RecoveryDirectory != "" || a.OnLifecycle != nil {
		request.Protocol, request.RecoveryDirectory = 3, a.RecoveryDirectory
		if request.ThreadMode == "" {
			request.ThreadMode = "ephemeral"
		}
	}
	reject := func(detail string) Result {
		out := invalidHarnessResult("codex-pro", a.Model, detail)
		out.Session = codexProUnknownSession(request)
		out.Session.Dispatch, out.Session.Outcome, out.Session.CleanupConfirmed = "not-dispatched", "failed", true
		return out
	}
	if request.Protocol == 3 && (a.OnLifecycle == nil || !filepath.IsAbs(a.RecoveryDirectory) || strings.ContainsRune(a.RecoveryDirectory, 0)) {
		return reject("durable lifecycle requires an absolute recovery directory and acknowledgement callback")
	}
	permission := a.PermissionMode
	if permission == "" {
		permission = "read-only"
	}
	if strings.TrimSpace(a.Model) == "" || strings.TrimSpace(a.Prompt) == "" ||
		(a.SessionID != "" && (!a.PersistentSession || strings.TrimSpace(a.SessionID) == "" || strings.ContainsRune(a.SessionID, 0))) ||
		strings.TrimSpace(request.RequestID) == "" || len(request.RequestID) > 256 || strings.ContainsRune(request.RequestID, 0) ||
		(permission != "read-only" && permission != "workspace-write" && permission != "danger-full-access") {
		return reject("Codex Pro requires model, prompt, valid invocation identity and supported permissions; resume requires scoped persistent history")
	}
	cwd, err := filepath.Abs(a.Cwd)
	if err == nil {
		cwd, err = filepath.EvalSymlinks(cwd)
	}
	if err != nil {
		return reject("Codex Pro cwd cannot be resolved: " + err.Error())
	}
	effort := a.ReasoningEffort
	if effort == "" {
		effort = "high"
	}
	wallMS := a.Watchdog.WallTimeoutMs
	if wallMS <= 0 {
		wallMS = watchdog.DefaultWallTimeoutMs
	}
	request.Cwd, request.Model, request.ReasoningEffort = cwd, a.Model, effort
	request.PermissionMode, request.Prompt, request.TimeoutMS = permission, a.Prompt, wallMS
	request.SessionTitle, request.IdleTimeoutMS = a.SessionTitle, max(0, a.Watchdog.IdleTimeoutMs)
	payload, err := json.Marshal(request)
	if err != nil {
		return reject("Codex Pro native request is not valid JSON: " + err.Error())
	}
	if err := validateProNativeRequest("codex-pro", request.Native); err != nil {
		return reject(err.Error())
	}
	payload = append(payload, '\n')
	if len(payload) > machineRequestBytes {
		return reject("Codex Pro start request exceeds the worker's 1 MiB JSONL limit")
	}
	command := a.Command
	if command == "" {
		command = CodexProCommand()
	}
	env := childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)
	for _, key := range []string{CodexProBinaryEnv, "CODEX_HOME"} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	result, interactionErr := runMachineWorker(command, "codex-pro", codexProStart(request), payload, watchdog.Opts{
		Context: a.Watchdog.Context, Cwd: cwd, Env: childenv.EnvSlice(env),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs, WallTimeoutMs: wallMS,
		KillGraceMs: max(a.Watchdog.KillGraceMs, codexProCleanupMS),
	}, a.OnLifecycle, a.OnEvent)
	parsed := parseCodexProStream(result.Stdout, request)
	if interactionErr != nil {
		parsed.StreamFailed = true
		parsed.Error = "Codex Pro worker transport failed: " + interactionErr.Error()
		parsed.Session, parsed.Usage, parsed.CleanupConfirmed = codexProUnknownSession(request), nil, false
	}
	out := fromWatchdog(result, "codex-pro", "", a.Model)
	out.Session = parsed.Session
	out.NativeReceipt = parsed.NativeReceipt
	out.CodexProReceipt, out.SessionID, out.Text, out.Usage = parsed.Receipt, parsed.SessionID, parsed.Text, parsed.Usage
	if result.Truncated {
		// A complete-looking prefix cannot attest what the capture dropped.
		out.Usage, out.Session.UsageStatus = nil, "unknown"
	}
	// A protocol/ACK error can cause the worker to cancel its native task.
	// That cleanup action does not replace the known initiating failure; an
	// actual caller cancellation is already present in the watchdog result.
	providerFactsTrusted := !parsed.StreamFailed && !result.Truncated
	parsed.CleanupConfirmed = parsed.CleanupConfirmed && providerFactsTrusted
	mergeWorkerTermination(&out, parsed.Cancelled && providerFactsTrusted, parsed.TimedOut && providerFactsTrusted, parsed.Truncated && providerFactsTrusted)
	if parsed.CleanupError != "" {
		out.CleanupError = strings.TrimPrefix(out.CleanupError+"; "+parsed.CleanupError, "; ")
	}
	if (result.ExitCode != nil || result.Signal != "") && !parsed.CleanupConfirmed && out.CleanupError == "" {
		// The plugin guardian owns a separate process group. Reaping the worker
		// cannot prove that group is quiescent, even when its eventual cleanup
		// is bounded. The orchestrator must stop instead of starting a retry.
		out.CleanupError = "Codex Pro process cleanup unconfirmed: no trustworthy cleanup receipt; confirm owned processes have stopped before resuming"
	}
	out.Settled = parsed.Settled && parsed.Error == "" && !out.Cancelled && !out.TimedOut && !out.Truncated && out.Signal == "" &&
		out.SpawnError == "" && out.CleanupError == "" && out.ExitCode != nil && *out.ExitCode == 0
	if result.SpawnError != "" || (result.Cancelled && result.ExitCode == nil && result.Signal == "") {
		// Watchdog returned before the worker existed: positive proof that no
		// backend side effect occurred, unlike the absence of a receipt.
		out.Session.Dispatch, out.Session.Outcome, out.Session.CleanupConfirmed = "not-dispatched", "failed", true
	} else {
		out.Session.CleanupConfirmed = parsed.CleanupConfirmed && out.CleanupError == ""
		if result.TimedOut || result.Cancelled || result.Truncated || result.Signal != "" || result.CleanupError != "" ||
			result.ExitCode == nil || (*result.ExitCode != 0 && out.Session.Outcome == "completed") {
			out.Session.Outcome, out.Session.ResumeCandidate = "unknown", false
		}
	}
	if !out.Settled && !out.Cancelled {
		message := parsed.Error
		switch {
		case out.TimedOut:
			message = timeoutMessage("Codex Pro worker", out)
			out.ErrorCode = errs.TIMEOUT
		case out.SpawnError != "":
			message = out.SpawnError
		case out.CleanupError != "":
			message = strings.TrimPrefix(message+"; "+out.CleanupError, "; ")
		case out.Truncated:
			if out.Session.Dispatch == "dispatched" && out.Session.Outcome == "failed" && out.ErrorCode == "" {
				out.ErrorCode = errs.TRANSPORT
			}
		case message == "":
			message = "Codex Pro worker exited without successful process completion: " + strings.TrimSpace(out.Stderr)
		}
		out.ErrorMessage = &message
	}
	if out.Session.Dispatch == "not-dispatched" && out.Session.CleanupConfirmed {
		out.Usage, out.CostUSD, out.Session.UsageStatus = &Usage{}, new(float64), "complete"
	}
	return out
}
