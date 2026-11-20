// Claude Code Pro consumes the plugin's public machine API through an owned
// worker. Its native approval modes are not Codex sandbox policies.
package drivers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/ckanner/flywheel/internal/watchdog"
)

const (
	ClaudeCodeProWorkerEnv      = "FLYWHEEL_CLAUDE_CODE_PRO_WORKER"
	ClaudeCodeProBinaryEnv      = "FLYWHEEL_CLAUDE_BIN"
	ClaudeCodeProPackageVersion = "0.7.0"
	claudeCodeProCleanupMS      = int64(10_000)
	claudeCodeProRequestBytes   = 16 << 20 // worker LIMITS.requestBytes, including the JSONL newline
)

var claudeCodeProModelPattern = regexp.MustCompile(`^claude-(sonnet|opus|haiku|fable)-[a-z0-9][a-z0-9.-]*(?:\[1m\])?$`)

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
	Subscription      *config.SubscriptionPolicy
	EvidenceOnly      bool
	Images            []imageinput.Image
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

type claudeCodeProStart = codexProStart

type claudeCodeProParsed struct {
	NativeReceipt    json.RawMessage
	Receipt          json.RawMessage
	Text             string
	Usage            *Usage
	Settled          bool
	CleanupConfirmed bool
	Cancelled        bool
	TimedOut         bool
	Truncated        bool
	StreamFailed     bool // worker protocol/envelope failure, distinct from a provider cancellation receipt
	Error            string
	ErrorCode        errs.ErrorCode
	Session          *SessionOutcome
}

type claudeCodeProExecution struct {
	PID              *int    `json:"pid"`
	ProcessGroupID   *int    `json:"processGroupId"`
	Ownership        string  `json:"ownership"`
	Dispatch         string  `json:"dispatch"`
	TurnDispatch     string  `json:"turnDispatch"`
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
	Subscription *struct {
		Version           int     `json:"version"`
		Mode              string  `json:"mode"`
		Status            string  `json:"status"`
		Source            string  `json:"source"`
		CheckedAt         *string `json:"checkedAt"`
		APIProvider       string  `json:"apiProvider"`
		SubscriptionType  string  `json:"subscriptionType"`
		ExtraUsageEnabled *bool   `json:"extraUsageEnabled"`
		ResolvedModel     string  `json:"resolvedModel"`
		Scope             string  `json:"scope"`
	} `json:"subscription"`
	Native    json.RawMessage `json:"native"`
	Protocol  int             `json:"protocol"`
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	RunID     string          `json:"runId"`
	OK        *bool           `json:"ok"`
	Requested struct {
		InputImages    *imageinput.Manifest `json:"inputImages,omitempty"`
		Cwd            string               `json:"cwd"`
		Model          string               `json:"model"`
		Effort         string               `json:"effort"`
		PermissionMode string               `json:"permissionMode"`
		SessionMode    string               `json:"sessionMode"`
		SessionID      json.RawMessage      `json:"sessionId"`
		SettingSources []string             `json:"settingSources"`
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
		Source        string `json:"source"`
		ModelEvidence *struct {
			Version     int      `json:"version"`
			RootMatched *bool    `json:"rootMatched"`
			RootModels  []string `json:"rootModels"`
			Source      string   `json:"source"`
		} `json:"modelEvidence"`
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
	return parseOwnedCLIStream(stdout, request, "claude-code-pro", "Claude Code Pro", parseClaudeCodeProReceipt)
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

func parseClaudeCodeProReceipt(raw json.RawMessage, request claudeCodeProStart) (parsed claudeCodeProParsed) {
	parsed = claudeCodeProParsed{Receipt: raw, Session: claudeCodeProUnknownSession(request)}
	var receipt claudeCodeProReceipt
	defer func() {
		parsed.NativeReceipt = receipt.Native
		if receipt.OK == nil {
			return
		}
		fold, err := foldProNative("claude-code-pro", request.Native, receipt.Native, *receipt.OK, parsed.CleanupConfirmed,
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
	mode := "ephemeral"
	if request.ThreadMode == "persistent" {
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
	if err := imageinput.ValidateManifest(receipt.Requested.InputImages); err != nil || !reflect.DeepEqual(receipt.Requested.InputImages, request.InputImageManifest) {
		parsed.Error = "invalid request: Claude Code Pro receipt input image manifest mismatch"
		return parsed
	}
	if request.Subscription != nil && *receipt.OK {
		sub := receipt.Subscription
		model := receipt.Configuration
		if sub == nil || sub.Version != 1 || sub.Mode != "subscription-only" || sub.Status != "verified" || sub.Source != "sdk-usage-0.3.326" || sub.CheckedAt == nil || *sub.CheckedAt == "" || sub.APIProvider != "firstParty" || !slices.Contains([]string{"pro", "max", "team", "enterprise"}, sub.SubscriptionType) || sub.ExtraUsageEnabled == nil || *sub.ExtraUsageEnabled || !claudeCodeProModelMatches(request.Model, sub.ResolvedModel) || sub.Scope != "pre-prompt-snapshot" || model == nil || model.ModelEvidence == nil || model.ModelEvidence.Version != 1 || model.ModelEvidence.RootMatched == nil || !*model.ModelEvidence.RootMatched || model.ModelEvidence.Source != "assistant.message.model" || len(model.ModelEvidence.RootModels) == 0 {
			parsed.Error = "invalid request: Claude Code Pro omitted verified native subscription and root model evidence"
			return parsed
		}
		for _, observed := range model.ModelEvidence.RootModels {
			if !claudeCodeProModelMatches(request.Model, observed) {
				parsed.Error = "invalid request: Claude Code Pro root model changed during subscription execution"
				return parsed
			}
		}
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
	parsed.Session.RunID = receipt.RunID
	if sessionID != nil {
		parsed.Session.SessionID = *sessionID
	}
	if request.Protocol == 3 && (e.TurnDispatch != "not-dispatched" && e.TurnDispatch != "possible" && e.TurnDispatch != "accepted" ||
		(*receipt.OK && e.TurnDispatch != "accepted")) {
		parsed.Error = "invalid request: Claude Code Pro durable receipt omitted its typed turn-dispatch fact"
		parsed.Session = claudeCodeProUnknownSession(request)
		return parsed
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
		if request.Protocol == 3 && e.TurnDispatch != "not-dispatched" {
			// A lost dispatch acknowledgement deliberately remains possible even
			// when no native PID was observed. Absence is not a turn receipt.
			parsed.Session.Dispatch, parsed.Session.Outcome, parsed.Session.UsageStatus = "unknown", "unknown", "unknown"
			parsed.Usage = nil
		}
	} else {
		if e.Dispatch != "dispatched" || e.PID == nil || *e.PID <= 0 || e.ProcessGroupID == nil || *e.ProcessGroupID != *e.PID ||
			e.Ownership != "process-group" || c.Method != "process-group" ||
			(e.Outcome != "completed" && e.Outcome != "failed" && e.Outcome != "unknown") {
			parsed.Error = "invalid request: Claude Code Pro process ownership or outcome is contradictory"
			return parsed
		}
		parsed.CleanupConfirmed, parsed.Session.Dispatch = *c.Confirmed, "dispatched"
		if e.TurnDispatch == "not-dispatched" && !*receipt.OK && *c.Confirmed && !*e.TerminalReceived && len(*receipt.Output) == 0 && usageStatus == "unknown" {
			parsed.Session.Dispatch, parsed.Session.Outcome, parsed.Session.UsageStatus = "not-dispatched", "failed", "complete"
			parsed.Session.SessionID = request.ThreadID
			parsed.Usage = &Usage{}
		}

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
		if len(*receipt.Output) != 0 || (receipt.Failure.Category == "cleanup-unconfirmed" && *c.Confirmed && len(receipt.Native) == 0) {
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
	// Only a structurally valid failure with correlated ownership may supply
	// termination facts. Contradictory success/ownership remains a transport error.
	parsed.Cancelled, parsed.TimedOut, parsed.Truncated = *e.Cancelled, *e.TimedOut, *e.Truncated
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
	request := claudeCodeProStart{Subscription: a.Subscription, Images: a.Images, InputImageManifest: imageinput.Describe(a.Images), Native: a.Native, Managed: a.Managed, Type: "start", Protocol: 1, RequestID: a.RequestID}
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
		return rejectedHarnessCall(CallArgs{Model: a.Model, RequestID: request.RequestID, SessionID: a.SessionID,
			PersistentSession: a.PersistentSession}, "claude-code-pro", detail)
	}
	if request.Protocol == 3 && (a.OnLifecycle == nil || !filepath.IsAbs(a.RecoveryDirectory) || strings.ContainsRune(a.RecoveryDirectory, 0)) {
		return reject("durable lifecycle requires an absolute recovery directory and acknowledgement callback")
	}
	if len(a.Prompt) > machineRequestBytes {
		return reject("Claude Code Pro prompt exceeds the unchanged 1 MiB text limit")
	}
	if err := imageinput.ValidateManifest(request.InputImageManifest); err != nil {
		return reject(err.Error())
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
	request.SessionTitle, request.IdleTimeoutMS = a.SessionTitle, max(0, a.Watchdog.IdleTimeoutMs)
	// Raw native controls must encode successfully before a worker is started.
	// Escaping and the newline count toward the same bounded worker request.
	payload, err := json.Marshal(request)
	if err != nil {
		return reject("Claude Code Pro native request is not valid JSON: " + err.Error())
	}
	if err := validateProNativeRequest("claude-code-pro", request.Native); err != nil {
		return reject(err.Error())
	}
	payload = append(payload, '\n')
	if len(payload) > claudeCodeProRequestBytes {
		return reject("Claude Code Pro start request exceeds the worker's 16 MiB JSONL limit")
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
	return runOwnedCLIWorker(a, request, command, env, "claude-code-pro", "Claude Code Pro", parseClaudeCodeProReceipt)
}
