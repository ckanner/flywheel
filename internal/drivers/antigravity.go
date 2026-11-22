package drivers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
)

const (
	AntigravityPackageVersion = "0.2.1"
	AntigravityPackage        = "@ckanner/dsh-subagent-antigravity"
	AntigravityWorkerEnv      = "FLYWHEEL_ANTIGRAVITY_WORKER"
	AntigravityBinaryEnv      = "FLYWHEEL_AGY_BIN"
)

func adapterPackageFor(harness string) string {
	if harness == "antigravity" {
		return AntigravityPackage
	}
	return "dsh-subagent-" + harness
}

type AntigravityArgs = ClaudeCodeProArgs

func AntigravityCommand() string {
	if command := os.Getenv(AntigravityWorkerEnv); command != "" {
		return command
	}
	return "flywheel-antigravity-worker"
}

func AntigravityBinary() string {
	if command := os.Getenv(AntigravityBinaryEnv); command != "" {
		return command
	}
	return "agy"
}

// The native CLI always retains conversation history. "fresh" means a new
// conversation, never a claim that the native account has no stored history.
func RunAntigravity(a AntigravityArgs) Result {
	request := codexProStart{Type: "start", Protocol: 1, RequestID: a.RequestID, Subscription: a.Subscription, EvidenceOnly: a.EvidenceOnly, Native: a.Native, Managed: a.Managed}
	if request.RequestID == "" {
		request.RequestID = rand.Text()
	}
	if a.PersistentSession {
		request.Protocol = 2
		request.ThreadMode = "persistent"
		request.ThreadID = a.SessionID
	}
	if a.RecoveryDirectory != "" || a.OnLifecycle != nil {
		request.Protocol = 3
		request.RecoveryDirectory = a.RecoveryDirectory
		if request.ThreadMode == "" {
			request.ThreadMode = "ephemeral"
		}
	}
	reject := func(detail string) Result {
		return rejectedHarnessCall(CallArgs{Model: a.Model, RequestID: request.RequestID, SessionID: a.SessionID, PersistentSession: a.PersistentSession, RecoveryDirectory: a.RecoveryDirectory}, "antigravity", detail)
	}
	if request.Protocol == 3 && (a.OnLifecycle == nil || !filepath.IsAbs(a.RecoveryDirectory) || strings.ContainsRune(a.RecoveryDirectory, 0)) {
		return reject("durable lifecycle requires an absolute recovery directory and acknowledgement callback")
	}
	if !machineOpaqueID(request.RequestID) || len(request.RequestID) > 256 || strings.TrimSpace(a.Prompt) == "" ||
		(a.SessionID != "" && (!a.PersistentSession || !machineOpaqueID(a.SessionID))) ||
		!(a.PermissionMode == "danger-full-access" && !a.EvidenceOnly || a.PermissionMode == "read-only" && a.EvidenceOnly) {
		return reject("Antigravity requires an explicit prompt, identity and full-access or verified evidence-only permissions")
	}
	if len(a.Images) != 0 {
		return reject("Antigravity image transport is not part of this machine contract; use the configured Claude multimodal route")
	}
	if a.EvidenceOnly && (len(a.Native) != 0 || a.Managed != nil || config.AntigravityOrchestrator(a.Model)) {
		return reject("Antigravity evidence-only calls require an ordinary model without native children")
	}
	if err := config.ValidateAntigravityNative(a.Native); err != nil {
		return reject(err.Error())
	}
	var native config.AntigravityNative
	_ = json.Unmarshal(a.Native, &native)
	if native.Mode == "swarm" && a.Managed != nil && a.Managed.Role != "execute" {
		return reject("Antigravity Swarm is restricted to approved Execute collaboration")
	}
	if a.Subscription == nil || *a.Subscription != (config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}) {
		return reject("Antigravity requires subscription-only policy version 1")
	}
	for _, value := range a.APIKeys {
		if value != "" {
			return reject("Antigravity subscription execution cannot receive API credentials")
		}
	}
	if err := config.ValidateAntigravityBinding(a.Model, a.ReasoningEffort); err != nil {
		return reject(err.Error())
	}
	cwd, err := filepath.Abs(a.Cwd)
	if err == nil {
		cwd, err = filepath.EvalSymlinks(cwd)
	}
	if err != nil {
		return reject("Antigravity cwd cannot be resolved: " + err.Error())
	}
	wall := a.Watchdog.WallTimeoutMs
	if wall <= 0 {
		wall = watchdog.DefaultWallTimeoutMs
	}
	if wall > 2147483647 {
		return reject("Antigravity timeout exceeds the public machine API limit")
	}
	request.Cwd, request.Model, request.ReasoningEffort = cwd, a.Model, a.ReasoningEffort
	request.Prompt, request.PermissionMode, request.TimeoutMS = a.Prompt, a.PermissionMode, wall
	request.SessionTitle, request.IdleTimeoutMS = a.SessionTitle, max(0, a.Watchdog.IdleTimeoutMs)
	payload, _ := json.Marshal(request)
	if len(payload)+1 > machineRequestBytes {
		return reject("Antigravity start request exceeds the worker's 1 MiB JSONL limit")
	}
	command := a.Command
	if command == "" {
		command = AntigravityCommand()
	}
	env := childenv.BuildChildEnv(childenv.OSLookupEnv, nil)
	if value, ok := os.LookupEnv(AntigravityBinaryEnv); ok {
		env[AntigravityBinaryEnv] = value
	}
	return runOwnedCLIWorker(a, request, command, env, "antigravity", "Antigravity", parseAntigravityReceipt)
}

type antigravityExecution struct {
	claudeCodeProExecution
	FinishedReason *string `json:"finishedReason"`
}

func (e *antigravityExecution) UnmarshalJSON(raw []byte) error {
	if err := json.Unmarshal(raw, &e.claudeCodeProExecution); err != nil {
		return err
	}
	var reason struct {
		FinishedReason *string `json:"finishedReason"`
	}
	if err := json.Unmarshal(raw, &reason); err != nil {
		return err
	}
	if !receiptHasFields(raw, "finishedReason") {
		return fmt.Errorf("Antigravity execution omitted finishedReason")
	}
	e.FinishedReason = reason.FinishedReason
	return nil
}

type antigravityReceipt struct {
	Protocol  int    `json:"protocol"`
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	RunID     string `json:"runId"`
	OK        *bool  `json:"ok"`
	Requested struct {
		Cwd            string                     `json:"cwd"`
		Model          string                     `json:"model"`
		Effort         string                     `json:"effort"`
		ExecutionMode  string                     `json:"executionMode"`
		PermissionMode string                     `json:"permissionMode"`
		Sandbox        *bool                      `json:"sandbox"`
		ToolPolicy     string                     `json:"toolPolicy"`
		SessionMode    string                     `json:"sessionMode"`
		SessionID      json.RawMessage            `json:"sessionId"`
		Subscription   *config.SubscriptionPolicy `json:"subscriptionPolicy"`
		Native         json.RawMessage            `json:"native"`
	} `json:"requested"`
	Execution *antigravityExecution `json:"execution"`
	Native    json.RawMessage       `json:"native"`
	Cleanup   *struct {
		Confirmed *bool  `json:"confirmed"`
		Method    string `json:"method"`
	} `json:"cleanup"`
	ProviderWork *struct {
		State      string `json:"state"`
		ReplaySafe *bool  `json:"replaySafe"`
	} `json:"providerWork"`
	Session *struct {
		Mode      string  `json:"mode"`
		ID        *string `json:"id"`
		Resumed   *bool   `json:"resumed"`
		Resumable *bool   `json:"resumable"`
	} `json:"session"`
	Configuration *struct {
		Source   string `json:"source"`
		Matched  *bool  `json:"matched"`
		Observed *struct {
			Cwd            string    `json:"cwd"`
			Model          string    `json:"model"`
			PermissionMode string    `json:"permissionMode"`
			Agent          string    `json:"agent"`
			Tools          *[]string `json:"tools"`
		} `json:"observed"`
	} `json:"configuration"`
	Subscription *struct {
		Mode              string `json:"mode"`
		Status            string `json:"status"`
		CreditsFallback   *bool  `json:"creditsFallback"`
		ConfigFingerprint string `json:"configFingerprint"`
	} `json:"subscription"`
	Output *[]struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"output"`
	Usage      json.RawMessage `json:"usage"`
	CostUSD    json.RawMessage `json:"costUSD"`
	CostStatus string          `json:"costStatus"`
	Failure    *struct {
		Stage    string `json:"stage"`
		Category string `json:"category"`
		Detail   string `json:"detail"`
	} `json:"failure"`
}

// Conversation counters are retained in the raw receipt. They are not per-turn
// deltas and must not be added to Flywheel's per-invocation budget accounting.
func validateAntigravityUsage(raw json.RawMessage) error {
	var u struct {
		Status   string `json:"status"`
		Scope    string `json:"scope"`
		Input    *int64 `json:"inputTokens"`
		Output   *int64 `json:"outputTokens"`
		Thinking *int64 `json:"thinkingTokens"`
		Cache    *int64 `json:"cacheReadTokens"`
		Total    *int64 `json:"totalTokens"`
	}
	if json.Unmarshal(raw, &u) != nil || !receiptHasFields(raw, "inputTokens", "outputTokens", "thinkingTokens", "cacheReadTokens", "totalTokens") || !slices.Contains([]string{"unknown", "partial", "complete"}, u.Status) || !slices.Contains([]string{"unknown", "conversation"}, u.Scope) {
		return fmt.Errorf("invalid request: Antigravity usage shape is invalid")
	}
	for _, v := range []*int64{u.Input, u.Output, u.Thinking, u.Cache, u.Total} {
		if v != nil && (*v < 0 || *v > 1<<53-1) || u.Status == "complete" && v == nil {
			return fmt.Errorf("invalid request: Antigravity usage contains unsafe or missing counters")
		}
	}
	return nil
}

func parseAntigravityReceipt(raw json.RawMessage, request codexProStart) (p claudeCodeProParsed) {
	p = claudeCodeProParsed{Receipt: raw, Session: unknownOwnedCLISession("antigravity", request)}
	var r antigravityReceipt
	mode := "new"
	if request.ThreadID != "" {
		mode = "resume"
	}
	execution, permission, policy := "accept-edits", "always-proceed", "native"
	if request.EvidenceOnly {
		execution, permission, policy = "plan", "request-review", "evidence-only"
	}
	if json.Unmarshal(raw, &r) != nil || !receiptHasFields(raw, "failure", "costUSD", "costStatus") || r.Protocol != 1 || r.Type != "result" || r.RequestID != request.RequestID || !machineOpaqueID(r.RunID) || r.OK == nil || *r.OK == (r.Failure != nil) ||
		r.Requested.Cwd != request.Cwd || r.Requested.Model != request.Model || r.Requested.Effort != request.ReasoningEffort || r.Requested.ExecutionMode != execution || r.Requested.PermissionMode != permission || r.Requested.ToolPolicy != policy || r.Requested.Sandbox == nil || *r.Requested.Sandbox || r.Requested.SessionMode != mode ||
		!(len(r.Requested.SessionID) == 0 && request.ThreadID == "" || requestedSessionMatches(r.Requested.SessionID, request.ThreadID)) || r.Requested.Subscription == nil || *r.Requested.Subscription != (config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}) || string(r.CostUSD) != "null" || r.CostStatus != "unknown" ||
		!(len(request.Native) == 0 && len(r.Requested.Native) == 0 || len(request.Native) > 0 && proJSONEqual(request.Native, r.Requested.Native)) {
		p.Error = "invalid request: Antigravity receipt identity or requested configuration mismatch"
		return
	}
	e, c, s, w := r.Execution, r.Cleanup, r.Session, r.ProviderWork
	if e == nil || e.TerminalReceived == nil || e.Cancelled == nil || e.TimedOut == nil || e.Truncated == nil || c == nil || c.Confirmed == nil || s == nil || s.Resumed == nil || s.Resumable == nil || w == nil || w.ReplaySafe == nil || r.Output == nil || r.Configuration == nil || r.Configuration.Matched == nil || r.Subscription == nil || r.Subscription.CreditsFallback == nil {
		p.Error = "invalid request: Antigravity receipt omitted lifecycle or configuration facts"
		return
	}
	if s.Mode != mode || *s.Resumed != (mode == "resume") || s.ID != nil && (!machineOpaqueID(*s.ID) || request.ThreadID != "" && *s.ID != request.ThreadID) || e.ExitCode != nil && e.Signal != nil || !slices.Contains([]string{"not-dispatched", "possible", "accepted"}, e.TurnDispatch) ||
		!slices.Contains([]string{"not-dispatched", "completed", "failed", "unknown"}, w.State) || !slices.Contains([]string{"not-started", "completed", "failed", "unknown"}, e.Outcome) ||
		e.FinishedReason != nil && (*r.OK || strings.TrimSpace(*e.FinishedReason) == "" || len(*e.FinishedReason) > 4096) {
		p.Error = "invalid request: Antigravity physical or session facts contradict the request"
		return
	}
	if err := validateAntigravityUsage(r.Usage); err != nil {
		p.Error = err.Error()
		return
	}
	if err := validateAntigravityNativeReceipt(r.Native, request, *r.OK, e, s.ID); err != nil {
		p.Error = err.Error()
		return
	}
	p.NativeReceipt = r.Native
	p.Session.RunID = r.RunID
	if s.ID != nil {
		p.Session.SessionID = *s.ID
	}
	if e.PID == nil {
		if e.ProcessGroupID != nil || e.Ownership != "none" || e.Dispatch != "not-dispatched" || e.TurnDispatch != "not-dispatched" || e.Outcome != "not-started" || e.ExitCode != nil || e.Signal != nil || *e.TerminalReceived || c.Method != "not-started" || !*c.Confirmed || *r.OK || w.State != "not-dispatched" || !*w.ReplaySafe {
			p.Error = "invalid request: Antigravity non-dispatch proof is contradictory"
			return
		}
	} else if *e.PID <= 0 || e.ProcessGroupID == nil || *e.ProcessGroupID != *e.PID || e.Ownership != "process-group" || c.Method != "process-group" {
		p.Error = "invalid request: Antigravity owned process facts are contradictory"
		return
	}
	p.CleanupConfirmed = *c.Confirmed
	if e.TurnDispatch == "not-dispatched" && e.Dispatch == "not-dispatched" && *c.Confirmed && !*r.OK && w.State == "not-dispatched" && *w.ReplaySafe {
		p.Session.Dispatch, p.Session.Outcome, p.Session.UsageStatus = "not-dispatched", "failed", "complete"
		p.Usage = &Usage{}
		p.Session.SessionID = request.ThreadID
	} else if e.Dispatch == "dispatched" {
		p.Session.Dispatch = "dispatched"
	}
	if *r.OK {
		observed := r.Configuration.Observed
		if e.Dispatch != "dispatched" || e.TurnDispatch != "accepted" || e.Outcome != "completed" || !*e.TerminalReceived || e.ExitCode == nil || *e.ExitCode != 0 || e.Signal != nil || *e.Cancelled || *e.TimedOut || *e.Truncated || !*c.Confirmed || w.State != "completed" || !*w.ReplaySafe || s.ID == nil || !*s.Resumable ||
			!*r.Configuration.Matched || r.Configuration.Source != "cli-init" || observed == nil || !samePhysicalDirectory(observed.Cwd, request.Cwd) || observed.Model != request.Model || observed.PermissionMode != permission ||
			request.EvidenceOnly && (observed.Tools == nil || !validAntigravityEvidenceAgent(r.Native, request.Cwd, r.RunID, observed.Agent)) || r.Subscription.Mode != "subscription-only" || r.Subscription.Status != "verified-local-controls" || *r.Subscription.CreditsFallback || r.Subscription.ConfigFingerprint == "" {
			p.Error = "Antigravity receipt does not prove successful completion, subscription controls, configuration and cleanup"
			return
		}
		for _, block := range *r.Output {
			if block.Type != "text" {
				p.Error = "invalid request: Antigravity output block is unsupported"
				return
			}
			p.Text += block.Text
		}
		if strings.TrimSpace(p.Text) == "" {
			p.Error = "empty response: Antigravity returned an empty answer"
			return
		}
		p.Settled = true
		p.Session.Outcome = "completed"
		p.Session.ResumeAccepted = *s.Resumed
		p.Session.ResumeCandidate = request.ThreadMode == "persistent"
		return
	}
	p.Cancelled, p.TimedOut, p.Truncated = *e.Cancelled, *e.TimedOut, *e.Truncated
	if e.TerminalReceived != nil && *e.TerminalReceived && e.TurnDispatch == "accepted" && *c.Confirmed && w.State == "failed" && !*e.Truncated {
		p.Session.Outcome = "failed"
	}
	p.ErrorCode = antigravityFailureCode(r.Failure.Category)
	p.Error = "Antigravity " + r.Failure.Stage + " failure (" + r.Failure.Category + "): " + r.Failure.Detail
	return
}

// A completed CLI policy violation or native turn cutoff cannot be repaired by
// a transport retry. Classification never supplies missing provider or cleanup
// facts: an observed-agent guard may stop locally while the outcome is unknown.
func antigravityFailureCode(category string) errs.ErrorCode {
	switch category {
	case "collaboration-not-observed", "observed-agent-limit", "native-cutoff":
		return errs.INVALID_REQUEST
	case "quota":
		return errs.QUOTA
	case "rate-limit":
		return errs.RATE_LIMIT
	default:
		return claudeCodeProFailureCode(category)
	}
}
