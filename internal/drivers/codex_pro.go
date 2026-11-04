// Codex Pro uses the plugin's public machine API through the bundled worker.
// Go owns the outer lifecycle and admits a result only after both layers settle.
package drivers

import (
	"bytes"
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

const (
	CodexProWorkerEnv      = "FLYWHEEL_CODEX_PRO_WORKER"
	CodexProBinaryEnv      = "FLYWHEEL_CODEX_BIN"
	CodexProPackageVersion = "0.8.0"
	codexProCleanupMS      = int64(10_000)
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

// CodexProArgs admits one fresh, explicitly configured public machine run.
type CodexProArgs struct {
	Prompt          string
	Model           string
	ReasoningEffort string
	PermissionMode  string
	SessionID       string
	Cwd             string
	Command         string
	APIKeys         map[string]string
	Watchdog        watchdog.Opts
	OnEvent         func(line string)
}

type codexProStart struct {
	Type            string `json:"type"`
	Protocol        int    `json:"protocol"`
	RequestID       string `json:"requestId"`
	Cwd             string `json:"cwd"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort"`
	PermissionMode  string `json:"permissionMode"`
	Prompt          string `json:"prompt"`
	TimeoutMS       int64  `json:"timeoutMs"`
}

type codexProParsed struct {
	Receipt          json.RawMessage
	SessionID        string
	Text             string
	Usage            *Usage
	Settled          bool
	CleanupConfirmed bool
	Cancelled        bool
	TimedOut         bool
	Truncated        bool
	CleanupError     string
	Error            string
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
	if !codexProHasFields(raw, "runId", "startedAt", "finishedAt", "durationMs", "threadId", "turnId", "pid", "supervisorPid",
		"exitCode", "signal", "supervisorExitCode", "supervisorSignal", "terminationRequested", "timedOut", "cancelled", "truncated", "spawnError", "failure", "ownership") {
		return fmt.Errorf("Codex Pro execution omitted a physical fact")
	}
	*e = codexProExecution(value)
	return nil
}

func codexProHasFields(raw json.RawMessage, names ...string) bool {
	var fields map[string]json.RawMessage
	// Both callers already decoded this same value into a struct; its only
	// possible shapes are an object or null, which also decode into this map.
	_ = json.Unmarshal(raw, &fields)
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			return false
		}
	}
	return true
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
	Stage      string             `json:"stage"`
	Category   string             `json:"category"`
	Diagnostic string             `json:"diagnostic"`
	Usage      json.RawMessage    `json:"usage"`
	Cleanup    *codexProCleanup   `json:"cleanup"`
	Execution  *codexProExecution `json:"execution"`
}

type codexProReceipt struct {
	Protocol  int    `json:"protocol"`
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	RunID     string `json:"runId"`
	OK        *bool  `json:"ok"`
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
	var parsed codexProParsed
	terminal := false
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
			parsed.CleanupConfirmed = false
			parsed.Error = "invalid request: Codex Pro worker emitted malformed JSONL"
			break
		}
		if frame.Protocol != 1 || frame.RequestID != request.RequestID || terminal {
			parsed.CleanupConfirmed = false
			parsed.Error = "invalid request: Codex Pro worker protocol/request mismatch or frame after terminal result"
			break
		}
		switch frame.Type {
		case "progress":
			if frame.Text == nil {
				parsed.Error = "invalid request: Codex Pro progress is missing text"
			}
		case "result":
			terminal = true
			parsed = parseCodexProReceipt(frame.Receipt, request)
			if !codexProHasFields(json.RawMessage(line), "receipt", "error") {
				parsed.CleanupConfirmed = false
				parsed.Error = "invalid request: Codex Pro worker omitted a terminal receipt/error field"
			}
			if frame.Adapter.Package != "dsh-subagent-codex-pro" || frame.Adapter.Version != CodexProPackageVersion || frame.Adapter.Protocol != 1 {
				parsed.CleanupConfirmed = false
				parsed.Error = "invalid request: Codex Pro adapter package/version/protocol mismatch"
			}
			if frame.Error != nil {
				parsed.Error = codexProFailureMessage(frame.Error.Code, frame.Error.Message, "", nil)
			}
		default:
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
	parsed.Settled = parsed.Settled && parsed.Error == ""
	return parsed
}

func parseCodexProReceipt(raw json.RawMessage, request codexProStart) codexProParsed {
	parsed := codexProParsed{Receipt: raw}
	var receipt codexProReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil || !codexProHasFields(raw, "outcome", "failure") || receipt.Protocol != 1 || receipt.Type != "result" ||
		receipt.RequestID != request.RequestID || receipt.RunID == "" || receipt.OK == nil ||
		(receipt.Outcome == nil) == (receipt.Failure == nil) ||
		receipt.Requested.Cwd != request.Cwd || receipt.Requested.Model != request.Model ||
		receipt.Requested.Effort != request.ReasoningEffort || receipt.Requested.PermissionMode != request.PermissionMode ||
		string(receipt.Requested.ThreadID) != "null" || string(receipt.CostUSD) != "null" || receipt.CostStatus != "unknown" {
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
	if execution != nil {
		parsed.TimedOut = execution.TimedOut != nil && *execution.TimedOut
		parsed.Cancelled = execution.Cancelled != nil && *execution.Cancelled
		parsed.Truncated = execution.Truncated != nil && *execution.Truncated
	}
	executionMatches := execution != nil && execution.RunID == receipt.RunID && execution.Ownership == "supervised-group" &&
		(receipt.Outcome == nil || execution.ThreadID == receipt.Outcome.ThreadID)
	// Failed service calls may still attest cleanup and remain safe to retry.
	// Absence of a trustworthy receipt is different from a settled model error.
	parsed.CleanupConfirmed = cleanup != nil && cleanup.Confirmed && executionMatches
	if cleanup != nil && !cleanup.Confirmed {
		parsed.CleanupError = "Codex Pro process cleanup unconfirmed: " + cleanup.Detail
	}
	if len(usage) != 0 && string(usage) != "null" {
		var err error
		parsed.Usage, err = parseCodexProUsage(usage)
		if err != nil {
			parsed.Error = err.Error()
			return parsed
		}
	}
	if !*receipt.OK {
		parsed.Error = codexProFailureMessage(category, diagnostic, stopReason, execution)
		return parsed
	}
	if receipt.Outcome == nil || receipt.Failure != nil || cleanup == nil || !cleanup.Confirmed ||
		stopReason != "completed" || !codexProExecutionValid(execution, receipt.RunID, parsed.SessionID) {
		parsed.Error = "Codex Pro receipt does not prove successful completion and process cleanup"
		return parsed
	}
	if err := validateCodexProConfiguration(receipt.Outcome, request); err != nil {
		parsed.Error = "invalid request: " + err.Error()
		return parsed
	}
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
	return e != nil && e.RunID == runID && threadID != "" && e.ThreadID == threadID && e.TurnID != "" &&
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
	c := o.Configuration
	if c == nil || c.Source != "thread/start" || !c.Matched || c.Missing == nil || len(*c.Missing) != 0 || c.Mismatches == nil || len(*c.Mismatches) != 0 {
		return fmt.Errorf("Codex Pro did not attest a fresh matching configuration")
	}
	for _, settings := range []codexProSettings{c.Requested, c.Effective} {
		if settings.Model != request.Model || settings.ApprovalPolicy != "never" || settings.Sandbox != request.PermissionMode ||
			!codexProSameDirectory(settings.Cwd, request.Cwd) {
			return fmt.Errorf("Codex Pro model/cwd/permission receipt differs from request")
		}
	}
	t := o.TurnConfiguration
	if t == nil || !t.Accepted || t.RequestedEffort != request.ReasoningEffort || t.Evidence != "turn/start-request" || string(t.EffectiveEffort) != "null" {
		return fmt.Errorf("Codex Pro effort acceptance evidence is missing or contradictory")
	}
	return validateCodexProPolicy(c.Effective.SandboxPolicy, request)
}

func codexProSameDirectory(actual, requested string) bool {
	if !filepath.IsAbs(actual) {
		return false
	}
	physical, err := filepath.EvalSymlinks(actual)
	return err == nil && physical == requested
}

// This is a post-run admission check, not a pre-turn sandbox guard: v0.8.0
// exposes the complete policy only in the final receipt. Standard Codex temp
// directories are permitted; extra writable roots must remain inside cwd.
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
	if err := decoder.Decode(&policy); err != nil || policy.NetworkAccess == nil || *policy.NetworkAccess {
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
	permission := a.PermissionMode
	if permission == "" {
		permission = "read-only"
	}
	if strings.TrimSpace(a.Model) == "" || strings.TrimSpace(a.Prompt) == "" || a.SessionID != "" ||
		(permission != "read-only" && permission != "workspace-write") {
		return invalidHarnessResult("codex-pro", a.Model, "Codex Pro requires model, prompt, bounded permissions, and a fresh session")
	}
	cwd, err := filepath.Abs(a.Cwd)
	if err == nil {
		cwd, err = filepath.EvalSymlinks(cwd)
	}
	if err != nil {
		return invalidHarnessResult("codex-pro", a.Model, "Codex Pro cwd cannot be resolved: "+err.Error())
	}
	effort := a.ReasoningEffort
	if effort == "" {
		effort = "high"
	}
	wallMS := a.Watchdog.WallTimeoutMs
	if wallMS <= 0 {
		wallMS = watchdog.DefaultWallTimeoutMs
	}
	request := codexProStart{Type: "start", Protocol: 1, RequestID: rand.Text(), Cwd: cwd,
		Model: a.Model, ReasoningEffort: effort, PermissionMode: permission, Prompt: a.Prompt, TimeoutMS: wallMS}
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
	var onActivity func(chunk, stream string)
	events := jsonLines{onLine: a.OnEvent, limit: watchdog.MaxBuffer}
	if a.OnEvent != nil {
		onActivity = events.feed
	}
	written := make(chan error, 1)
	result := watchdog.Run(command, nil, watchdog.Opts{
		Context: a.Watchdog.Context, Cwd: cwd, Env: childenv.EnvSlice(env),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs, WallTimeoutMs: wallMS,
		KillGraceMs: max(a.Watchdog.KillGraceMs, codexProCleanupMS), OnActivity: onActivity,
		InteractStdin: func(stdin io.WriteCloser, finish func()) {
			err := json.NewEncoder(stdin).Encode(request)
			written <- err
			if err != nil {
				finish()
			}
		},
	})
	if a.OnEvent != nil {
		events.flush()
	}
	parsed := parseCodexProStream(result.Stdout, request)
	select {
	case err := <-written:
		if err != nil {
			parsed.Error = "Codex Pro worker request write failed: " + err.Error()
		}
	default: // Spawn failure or pre-start cancellation has no stdin callback.
	}
	out := fromWatchdog(result, "codex-pro", "", a.Model)
	out.CodexProReceipt, out.SessionID, out.Text, out.Usage = parsed.Receipt, parsed.SessionID, parsed.Text, parsed.Usage
	out.Cancelled = out.Cancelled || parsed.Cancelled
	out.TimedOut = out.TimedOut || parsed.TimedOut
	out.Truncated = out.Truncated || parsed.Truncated
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
	if !out.Settled && !out.Cancelled {
		message := parsed.Error
		switch {
		case out.TimedOut:
			message = "Codex Pro worker timed out"
		case out.SpawnError != "":
			message = out.SpawnError
		case out.CleanupError != "":
			message = strings.TrimPrefix(message+"; "+out.CleanupError, "; ")
		case out.Truncated:
			message = "Codex Pro worker output was truncated"
		case message == "":
			message = "Codex Pro worker exited without successful process completion: " + strings.TrimSpace(out.Stderr)
		}
		out.ErrorMessage = &message
	}
	return out
}
