// Claude Code Pro consumes the plugin's public machine API through an owned
// worker. Its native approval modes are not Codex sandbox policies.
package drivers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
)

const (
	ClaudeCodeProWorkerEnv      = "FLYWHEEL_CLAUDE_CODE_PRO_WORKER"
	ClaudeCodeProBinaryEnv      = "FLYWHEEL_CLAUDE_BIN"
	ClaudeCodeProPackageVersion = "0.3.2"
	claudeCodeProCleanupMS      = int64(10_000)
	claudeCodeProRequestBytes   = 1 << 20 // worker LIMITS.requestBytes, including the JSONL newline
)

var claudeCodeProModelPattern = regexp.MustCompile(`^claude-(sonnet|opus|haiku)-[a-z0-9][a-z0-9.-]*(?:\[1m\])?$`)

// Match only the aliases understood by the pinned public API. Exact custom
// model IDs remain valid; no static catalog invents a newer model or fallback.
func claudeCodeProModelMatches(requested, observed string) bool {
	if requested == observed {
		return strings.TrimSpace(observed) != ""
	}
	match := claudeCodeProModelPattern.FindStringSubmatch(observed)
	return len(match) == 2 && match[1] == requested
}

func ClaudeCodeProCommand() string {
	if command := os.Getenv(ClaudeCodeProWorkerEnv); command != "" {
		return command
	}
	return "flywheel-claude-code-pro-worker"
}

func ClaudeCodeProBinary() string {
	if command := os.Getenv(ClaudeCodeProBinaryEnv); command != "" {
		return command
	}
	return "claude"
}

type ClaudeCodeProArgs struct {
	Prompt            string
	Model             string
	ReasoningEffort   string
	PermissionMode    string
	SessionID         string
	PersistentSession bool
	RequestID         string
	Cwd               string
	Command           string
	APIKeys           map[string]string
	Watchdog          watchdog.Opts
	OnEvent           func(line string)
}

type claudeCodeProStart struct {
	Type            string `json:"type"`
	Protocol        int    `json:"protocol"`
	RequestID       string `json:"requestId"`
	Cwd             string `json:"cwd"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort"`
	PermissionMode  string `json:"permissionMode"`
	Prompt          string `json:"prompt"`
	TimeoutMS       int64  `json:"timeoutMs"`
	ThreadMode      string `json:"threadMode,omitempty"`
	ThreadID        string `json:"threadId,omitempty"`
}

type claudeCodeProParsed struct {
	Receipt          json.RawMessage
	Text             string
	Usage            *Usage
	Settled          bool
	CleanupConfirmed bool
	Cancelled        bool
	TimedOut         bool
	Truncated        bool
	Error            string
	ErrorCode        errs.ErrorCode
	Session          *SessionOutcome
}

type claudeCodeProExecution struct {
	PID              *int    `json:"pid"`
	ProcessGroupID   *int    `json:"processGroupId"`
	Ownership        string  `json:"ownership"`
	Dispatch         string  `json:"dispatch"`
	Outcome          string  `json:"outcome"`
	TerminalReceived *bool   `json:"terminalReceived"`
	ExitCode         *int    `json:"exitCode"`
	Signal           *string `json:"signal"`
	Cancelled        *bool   `json:"cancelled"`
	TimedOut         *bool   `json:"timedOut"`
	Truncated        *bool   `json:"truncated"`
}

func (e *claudeCodeProExecution) UnmarshalJSON(raw []byte) error {
	type execution claudeCodeProExecution
	var value execution
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if !receiptHasFields(raw, "pid", "processGroupId", "ownership", "dispatch", "outcome", "terminalReceived",
		"exitCode", "signal", "cancelled", "timedOut", "truncated") {
		return fmt.Errorf("Claude Code Pro execution omitted a physical fact")
	}
	*e = claudeCodeProExecution(value)
	return nil
}

type claudeCodeProReceipt struct {
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
		SessionMode    string          `json:"sessionMode"`
		SessionID      json.RawMessage `json:"sessionId"`
		SettingSources []string        `json:"settingSources"`
	} `json:"requested"`
	Execution *claudeCodeProExecution `json:"execution"`
	Cleanup   *struct {
		Confirmed *bool  `json:"confirmed"`
		Method    string `json:"method"`
	} `json:"cleanup"`
	Session *struct {
		Mode      string          `json:"mode"`
		ID        json.RawMessage `json:"id"`
		Resumed   *bool           `json:"resumed"`
		Resumable *bool           `json:"resumable"`
	} `json:"session"`
	Configuration *struct {
		Source   string `json:"source"`
		Observed *struct {
			Cwd            string `json:"cwd"`
			Model          string `json:"model"`
			PermissionMode string `json:"permissionMode"`
			CLIVersion     string `json:"cliVersion"`
		} `json:"observed"`
		Matched *bool `json:"matched"`
	} `json:"configuration"`
	Output *[]struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"output"`
	Usage           json.RawMessage `json:"usage"`
	ReportedCostUSD *float64        `json:"reportedCostUSD"`
	CostUSD         json.RawMessage `json:"costUSD"`
	CostStatus      string          `json:"costStatus"`
	Failure         *struct {
		Stage    string `json:"stage"`
		Category string `json:"category"`
		Detail   string `json:"detail"`
	} `json:"failure"`
}

func claudeCodeProUnknownSession(request claudeCodeProStart) *SessionOutcome {
	return &SessionOutcome{Protocol: fmt.Sprintf("claude-code-pro/%s/worker-v%d", ClaudeCodeProPackageVersion, request.Protocol),
		RequestID: request.RequestID, Dispatch: "unknown", Outcome: "unknown", UsageStatus: "unknown"}
}

// A display progress frame cannot authorize continuation. An intact final
// envelope is correlated twice: by the worker and by the plugin receipt.
func parseClaudeCodeProStream(stdout string, request claudeCodeProStart) claudeCodeProParsed {
	parsed := claudeCodeProParsed{Session: claudeCodeProUnknownSession(request)}
	terminal, trusted := false, true
	for _, line := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var frame struct {
			Protocol  int     `json:"protocol"`
			Type      string  `json:"type"`
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
			Admission *struct {
				Dispatched       *bool `json:"dispatched"`
				CleanupConfirmed *bool `json:"cleanupConfirmed"`
			} `json:"admission"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil || frame.Protocol != request.Protocol ||
			frame.RequestID != request.RequestID || terminal {
			trusted = false
			parsed.Error = "invalid request: Claude Code Pro worker emitted malformed, mismatched or post-terminal data"
			break
		}
		switch frame.Type {
		case "progress":
			if frame.Text == nil {
				trusted = false
				parsed.Error = "invalid request: Claude Code Pro progress omitted text"
			}
		case "result":
			terminal = true
			parsed = parseClaudeCodeProReceipt(frame.Receipt, request)
			if !receiptHasFields(json.RawMessage(line), "receipt", "error") || frame.Adapter.Package != "dsh-subagent-claude-code-pro" ||
				frame.Adapter.Version != ClaudeCodeProPackageVersion || frame.Adapter.Protocol != 1 {
				trusted = false
				parsed.Error = "invalid request: Claude Code Pro terminal envelope does not match the pinned adapter contract"
			} else if frame.Admission != nil {
				if string(frame.Receipt) != "null" || frame.Error == nil || frame.Admission.Dispatched == nil || *frame.Admission.Dispatched ||
					frame.Admission.CleanupConfirmed == nil || !*frame.Admission.CleanupConfirmed || !claudeCodeProAdmissionError(frame.Error.Code) {
					trusted = false
					parsed.Error = "invalid request: Claude Code Pro admission contradicts its terminal envelope"
				} else {
					parsed.CleanupConfirmed = true
					parsed.Session.Dispatch, parsed.Session.Outcome, parsed.Session.UsageStatus = "not-dispatched", "failed", "complete"
					parsed.Session.SessionID, parsed.Usage = request.ThreadID, &Usage{}
					parsed.Error, parsed.ErrorCode = "Claude Code Pro worker admission failed: "+frame.Error.Message, errs.INVALID_REQUEST
				}
			} else if frame.Error != nil {
				trusted = false
				parsed.Error = "Claude Code Pro worker failed without a trusted invocation result: " + frame.Error.Message
			}
		default:
			trusted = false
			parsed.Error = "invalid request: Claude Code Pro worker emitted an unknown frame"
		}
		if !trusted {
			break
		}
	}
	if !terminal && parsed.Error == "" {
		parsed.Error = "Claude Code Pro worker ended without a terminal receipt"
	}
	if !trusted {
		parsed.CleanupConfirmed, parsed.Settled, parsed.Usage, parsed.ErrorCode = false, false, nil, ""
		parsed.Session = claudeCodeProUnknownSession(request)
	}
	parsed.Session.CleanupConfirmed = parsed.CleanupConfirmed
	return parsed
}

// Only errors that the worker can raise before entering runMachine carry its
// explicit admission attestation. A runtime throw is never on this list.
func claudeCodeProAdmissionError(code string) bool {
	switch code {
	case "INVALID_REQUEST", "UNSUPPORTED_RUNTIME", "ADAPTER_UNAVAILABLE", "ADAPTER_MISMATCH", "CANCELLED",
		"INPUT_TIMEOUT", "INPUT_LIMIT", "INPUT_ERROR", "PARENT_DISCONNECTED":
		return true
	default:
		return false
	}
}

func parseClaudeCodeProReceipt(raw json.RawMessage, request claudeCodeProStart) claudeCodeProParsed {
	parsed := claudeCodeProParsed{Receipt: raw, Session: claudeCodeProUnknownSession(request)}
	var receipt claudeCodeProReceipt
	mode := "ephemeral"
	if request.Protocol == 2 {
		mode = "persistent"
	}
	if err := json.Unmarshal(raw, &receipt); err != nil || !receiptHasFields(raw, "failure", "reportedCostUSD") ||
		receipt.Protocol != 1 || receipt.Type != "result" || receipt.RequestID != request.RequestID ||
		strings.TrimSpace(receipt.RunID) == "" || strings.ContainsRune(receipt.RunID, 0) || receipt.OK == nil ||
		(*receipt.OK == (receipt.Failure != nil)) || receipt.Requested.Cwd != request.Cwd || receipt.Requested.Model != request.Model ||
		receipt.Requested.Effort != request.ReasoningEffort || receipt.Requested.PermissionMode != "bypassPermissions" ||
		receipt.Requested.SessionMode != mode || !requestedSessionMatches(receipt.Requested.SessionID, request.ThreadID) ||
		!slices.Equal(receipt.Requested.SettingSources, []string{"user", "project", "local"}) ||
		string(receipt.CostUSD) != "null" || receipt.CostStatus != "unknown" || (receipt.ReportedCostUSD != nil && *receipt.ReportedCostUSD < 0) {
		parsed.Error = "invalid request: Claude Code Pro receipt identity or requested configuration mismatch"
		return parsed
	}
	e, s, c := receipt.Execution, receipt.Session, receipt.Cleanup
	if e == nil || e.TerminalReceived == nil || e.Cancelled == nil || e.TimedOut == nil || e.Truncated == nil ||
		c == nil || c.Confirmed == nil || s == nil || s.Resumed == nil || s.Resumable == nil || s.Mode != mode ||
		receipt.Configuration == nil || receipt.Configuration.Matched == nil || receipt.Output == nil {
		parsed.Error = "invalid request: Claude Code Pro receipt omitted lifecycle or configuration facts"
		return parsed
	}
	var sessionID *string
	if len(s.ID) == 0 || json.Unmarshal(s.ID, &sessionID) != nil ||
		(sessionID != nil && (strings.TrimSpace(*sessionID) == "" || strings.ContainsRune(*sessionID, 0))) ||
		(sessionID != nil && request.ThreadID != "" && *sessionID != request.ThreadID) ||
		(*s.Resumed && (request.ThreadID == "" || sessionID == nil)) || (e.ExitCode != nil && e.Signal != nil) {
		parsed.Error = "invalid request: Claude Code Pro session identity or physical exit is contradictory"
		return parsed
	}
	usage, usageStatus, usageErr := parseClaudeCodeProUsage(receipt.Usage)
	if usageErr != nil {
		parsed.Error = usageErr.Error()
		return parsed
	}
	parsed.Cancelled, parsed.TimedOut, parsed.Truncated = *e.Cancelled, *e.TimedOut, *e.Truncated
	parsed.Session.RunID = receipt.RunID
	if sessionID != nil {
		parsed.Session.SessionID = *sessionID
	}
	if e.Dispatch == "not-dispatched" {
		if *receipt.OK || receipt.Failure == nil || e.Outcome != "not-started" || *e.TerminalReceived ||
			e.PID != nil || e.ProcessGroupID != nil || e.Ownership != "none" || e.ExitCode != nil || e.Signal != nil ||
			!*c.Confirmed || c.Method != "not-started" || sessionID != nil || *s.Resumed || *s.Resumable ||
			len(*receipt.Output) != 0 || usageStatus != "unknown" {
			parsed.Error = "invalid request: Claude Code Pro non-dispatch proof is contradictory"
			return parsed
		}
		parsed.CleanupConfirmed, parsed.Usage = true, &Usage{}
		parsed.Session.Dispatch, parsed.Session.Outcome, parsed.Session.UsageStatus = "not-dispatched", "failed", "complete"
		parsed.Session.SessionID = request.ThreadID
	} else {
		if e.Dispatch != "dispatched" || e.PID == nil || *e.PID <= 0 || e.ProcessGroupID == nil || *e.ProcessGroupID != *e.PID ||
			e.Ownership != "process-group" || c.Method != "process-group" ||
			(e.Outcome != "completed" && e.Outcome != "failed" && e.Outcome != "unknown") {
			parsed.Error = "invalid request: Claude Code Pro process ownership or outcome is contradictory"
			return parsed
		}
		parsed.CleanupConfirmed, parsed.Session.Dispatch = *c.Confirmed, "dispatched"
		configuration := receipt.Configuration
		matched := *configuration.Matched && configuration.Source == "cli-init" && configuration.Observed != nil &&
			samePhysicalDirectory(configuration.Observed.Cwd, request.Cwd) && configuration.Observed.PermissionMode == "bypassPermissions" &&
			claudeCodeProModelMatches(request.Model, configuration.Observed.Model) && sessionID != nil && *s.Resumed == (request.ThreadID != "")
		physical := !*e.Truncated && e.ExitCode != nil && e.Signal == nil &&
			((e.Outcome == "completed" && *e.ExitCode == 0) || (e.Outcome == "failed" && (*e.ExitCode == 0 || *e.ExitCode == 1)))
		terminal := *e.TerminalReceived && matched && physical
		if *s.Resumable && (!*receipt.OK || mode != "persistent" || !terminal || !*c.Confirmed || *e.Cancelled || *e.TimedOut) {
			parsed.Error = "invalid request: Claude Code Pro offered history without a successful persistent checkpoint"
			return parsed
		}
		if *receipt.OK {
			if !terminal || e.Outcome != "completed" || *e.Cancelled || *e.TimedOut || !*c.Confirmed || *s.Resumable != (mode == "persistent") {
				parsed.Error = "Claude Code Pro receipt does not prove successful completion, configuration and cleanup"
				return parsed
			}
			parsed.Session.Outcome = "completed"
			parsed.Usage, parsed.Session.UsageStatus = usage, usageStatus
			parsed.Session.ResumeAccepted, parsed.Session.ResumeCandidate = *s.Resumed, *s.Resumable
			for _, block := range *receipt.Output {
				if block.Type != "text" {
					parsed.Session.Outcome, parsed.Session.ResumeCandidate = "unknown", false
					parsed.Usage, parsed.Session.UsageStatus = nil, "unknown"
					parsed.Error = "invalid request: Claude Code Pro returned an unsupported output block"
					return parsed
				}
				parsed.Text += block.Text
			}
			if strings.TrimSpace(parsed.Text) == "" {
				parsed.Error = "empty response: Claude Code Pro returned an empty answer"
				return parsed
			}
			parsed.Settled = true
			return parsed
		}
		if len(*receipt.Output) != 0 || (receipt.Failure.Category == "cleanup-unconfirmed" && *c.Confirmed) {
			parsed.Error = "invalid request: Claude Code Pro failure contradicts its output or cleanup"
			return parsed
		}
		if terminal && receipt.Failure.Category != "protocol" && receipt.Failure.Category != "configuration-mismatch" {
			parsed.Usage, parsed.Session.UsageStatus = usage, usageStatus
			if !*e.Cancelled && !*e.TimedOut {
				parsed.Session.Outcome = e.Outcome
			}
		}
	}
	parsed.ErrorCode = claudeCodeProFailureCode(receipt.Failure.Category)
	parsed.Error = "Claude Code Pro " + receipt.Failure.Stage + " failure: " + receipt.Failure.Detail
	return parsed
}

// Claude reports uncached input separately from cache tokens. Flywheel's Input
// includes both, so total and cache ratios use the same units as other drivers.
func parseClaudeCodeProUsage(raw json.RawMessage) (*Usage, string, error) {
	var report struct {
		Status        string `json:"status"`
		Input         *int64 `json:"inputTokens"`
		Output        *int64 `json:"outputTokens"`
		CacheRead     *int64 `json:"cacheReadTokens"`
		CacheCreation *int64 `json:"cacheCreationTokens"`
		Total         *int64 `json:"totalTokens"`
	}
	if err := json.Unmarshal(raw, &report); err != nil || !receiptHasFields(raw, "inputTokens", "outputTokens", "cacheReadTokens", "cacheCreationTokens", "totalTokens") ||
		(report.Status != "complete" && report.Status != "partial" && report.Status != "unknown") {
		return nil, "unknown", fmt.Errorf("invalid request: Claude Code Pro usage is malformed or has unknown completeness")
	}
	for _, value := range []*int64{report.Input, report.Output, report.CacheRead, report.CacheCreation, report.Total} {
		if value != nil && (*value < 0 || *value > 1<<53-1) {
			return nil, "unknown", fmt.Errorf("invalid request: Claude Code Pro usage contains an unsafe counter")
		}
	}
	if report.Status != "complete" {
		return nil, report.Status, nil
	}
	if report.Input == nil || report.Output == nil || report.CacheRead == nil || report.CacheCreation == nil || report.Total == nil ||
		*report.Total != *report.Input+*report.Output+*report.CacheRead+*report.CacheCreation {
		return nil, "unknown", fmt.Errorf("invalid request: Claude Code Pro complete usage is missing or contradicts its total")
	}
	return &Usage{Input: *report.Input + *report.CacheRead + *report.CacheCreation, Output: *report.Output,
		CacheRead: *report.CacheRead, CacheWrite: *report.CacheCreation, TotalTokens: *report.Total}, report.Status, nil
}

func claudeCodeProFailureCode(category string) errs.ErrorCode {
	switch category {
	case "invalid-request", "unsupported-runtime", "configuration-mismatch":
		return errs.INVALID_REQUEST
	case "auth":
		return errs.AUTH
	case "timeout":
		return errs.TIMEOUT
	case "invalid-result":
		return errs.EMPTY_RESPONSE
	default:
		return errs.TRANSPORT // ambiguous CLI limits/prose do not select a paid fallback
	}
}

func RunClaudeCodePro(a ClaudeCodeProArgs) Result {
	request := claudeCodeProStart{Type: "start", Protocol: 1, RequestID: a.RequestID}
	if request.RequestID == "" {
		request.RequestID = rand.Text()
	}
	if a.PersistentSession {
		request.Protocol, request.ThreadMode, request.ThreadID = 2, "persistent", a.SessionID
	}
	reject := func(detail string) Result {
		return rejectedHarnessCall(CallArgs{Model: a.Model, RequestID: request.RequestID, SessionID: a.SessionID,
			PersistentSession: a.PersistentSession}, "claude-code-pro", detail)
	}
	if strings.TrimSpace(a.Model) == "" || strings.TrimSpace(a.Prompt) == "" || a.PermissionMode != "danger-full-access" ||
		(a.SessionID != "" && (!a.PersistentSession || strings.TrimSpace(a.SessionID) == "" || strings.ContainsRune(a.SessionID, 0))) ||
		strings.TrimSpace(request.RequestID) == "" || len(request.RequestID) > 256 || strings.ContainsRune(request.RequestID, 0) {
		return reject("Claude Code Pro requires explicit model, prompt, invocation identity and full-access; resume requires scoped persistent history")
	}
	for key, value := range a.APIKeys {
		if value != "" && !config.ClaudeCodeProCredentialAllowed(key) {
			return reject("Claude Code Pro received a credential key unsupported by its public machine API")
		}
	}
	effort := a.ReasoningEffort
	if effort == "" {
		effort = "high"
	}
	if !slices.Contains([]string{"low", "medium", "high", "xhigh", "max"}, effort) {
		return reject("Claude Code Pro effort must be low|medium|high|xhigh|max")
	}
	cwd, err := filepath.Abs(a.Cwd)
	if err == nil {
		cwd, err = filepath.EvalSymlinks(cwd)
	}
	if err != nil {
		return reject("Claude Code Pro cwd cannot be resolved: " + err.Error())
	}
	wallMS := a.Watchdog.WallTimeoutMs
	if wallMS <= 0 {
		wallMS = watchdog.DefaultWallTimeoutMs
	}
	if wallMS > 2147483647 {
		return reject("Claude Code Pro timeout exceeds the public machine API limit")
	}
	request.Cwd, request.Model, request.ReasoningEffort = cwd, a.Model, effort
	request.PermissionMode, request.Prompt, request.TimeoutMS = a.PermissionMode, a.Prompt, wallMS
	// The request contains only strings and bounded integers, so JSON encoding
	// cannot fail. Check and send the same bytes: JSON escaping and the newline
	// count toward the worker boundary, not just the unencoded prompt length.
	payload, _ := json.Marshal(request)
	payload = append(payload, '\n')
	if len(payload) > claudeCodeProRequestBytes {
		return reject("Claude Code Pro start request exceeds the worker's 1 MiB JSONL limit")
	}
	command := a.Command
	if command == "" {
		command = ClaudeCodeProCommand()
	}
	env := childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)
	for _, key := range []string{ClaudeCodeProBinaryEnv, "CLAUDE_CONFIG_DIR"} {
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
		KillGraceMs: max(a.Watchdog.KillGraceMs, claudeCodeProCleanupMS), OnActivity: onActivity,
		InteractStdin: func(stdin io.WriteCloser, finish func()) {
			_, err := stdin.Write(payload)
			written <- err
			if err != nil {
				finish()
			}
		},
	})
	if a.OnEvent != nil {
		events.flush()
	}
	parsed := parseClaudeCodeProStream(result.Stdout, request)
	select {
	case err := <-written:
		if err != nil {
			parsed.Error = "Claude Code Pro worker request write failed: " + err.Error()
			parsed.Session, parsed.Usage, parsed.CleanupConfirmed = claudeCodeProUnknownSession(request), nil, false
		}
	default: // watchdog proved a spawn failure or cancellation before starting.
	}
	out := fromWatchdog(result, "claude-code-pro", "", a.Model)
	out.ClaudeCodeProReceipt, out.Session, out.SessionID, out.Text, out.Usage = parsed.Receipt, parsed.Session, parsed.Session.SessionID, parsed.Text, parsed.Usage
	out.Cancelled, out.TimedOut, out.Truncated = out.Cancelled || parsed.Cancelled, out.TimedOut || parsed.TimedOut, out.Truncated || parsed.Truncated
	if result.Truncated {
		out.Usage, out.Session.UsageStatus = nil, "unknown"
	}
	if (result.ExitCode != nil || result.Signal != "") && !parsed.CleanupConfirmed && out.CleanupError == "" {
		out.CleanupError = "Claude Code Pro process cleanup unconfirmed: no trustworthy owned-group receipt"
	}
	out.Settled = parsed.Settled && parsed.Error == "" && !out.Cancelled && !out.TimedOut && !out.Truncated && out.Signal == "" &&
		out.SpawnError == "" && out.CleanupError == "" && out.ExitCode != nil && *out.ExitCode == 0
	if result.SpawnError != "" || (result.Cancelled && result.ExitCode == nil && result.Signal == "") {
		out.Session.Dispatch, out.Session.Outcome, out.Session.CleanupConfirmed, out.Session.UsageStatus = "not-dispatched", "failed", true, "complete"
		out.Usage = &Usage{}
	} else {
		out.Session.CleanupConfirmed = parsed.CleanupConfirmed && out.CleanupError == ""
		if result.Cancelled || result.TimedOut || result.Truncated || result.Signal != "" || result.CleanupError != "" ||
			result.ExitCode == nil || (*result.ExitCode != 0 && out.Session.Outcome == "completed") {
			out.Session.Outcome, out.Session.ResumeCandidate = "unknown", false
		}
	}
	if out.Session.Dispatch == "not-dispatched" && out.Session.CleanupConfirmed {
		out.CostUSD = new(float64) // positive proof no backend process accepted work
	}
	if !out.Settled && !out.Cancelled {
		message := parsed.Error
		switch {
		case out.TimedOut:
			message = "Claude Code Pro worker timed out"
		case out.SpawnError != "":
			message = out.SpawnError
		case out.CleanupError != "":
			message = strings.TrimPrefix(message+"; "+out.CleanupError, "; ")
		case out.Truncated:
			message = "Claude Code Pro worker output was truncated"
		case message == "":
			message = "Claude Code Pro worker exited without successful process completion"
		}
		out.ErrorMessage = &message
		if !out.TimedOut && !out.Truncated && out.SpawnError == "" && out.CleanupError == "" && out.Signal == "" && out.ExitCode != nil && *out.ExitCode == 1 {
			out.ErrorCode = parsed.ErrorCode
		}
	}
	return out
}
