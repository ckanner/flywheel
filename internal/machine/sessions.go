package machine

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
)

func sessionKey(role, scope string) string { return role + "/" + scope }

func sessionHash(value any) string {
	raw, _ := json.Marshal(value) // all callers pass concrete JSON-safe values
	// Evidence is typed before save and maps after load; canonical key ordering
	// makes a checkpoint's identity independent of that representation.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var canonical any
	_ = decoder.Decode(&canonical)
	raw, _ = json.Marshal(canonical)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func sessionMetrics(ledger *state.SessionLedger, id string) any {
	if ledger != nil && id != "" {
		for _, record := range ledger.Records {
			if record.Last.ID == id {
				return record
			}
		}
	}
	return nil
}

func (c *Context) trackedSessions(role, harness string) bool {
	driverRole := role
	if role == "judge-panel" {
		driverRole = "judge"
	}
	if c.Config.Drivers[driverRole] == "mock" {
		return false
	}
	return config.SupportsScopedSessions(harness)
}

func (c *Context) scopedSessions(role, harness string) bool {
	if sessionQualityMode(role, c.Config.Sessions) != "scoped" || !c.trackedSessions(role, harness) {
		return false
	}
	p := c.Config.Loops["plan"]
	return role != "plan" || !p.ResidentSessionSet || p.ResidentSession
}

// Acceptance and context changes can reuse history under an unchanged declared
// goal. Applying a received result requires the exact original contract instead.
func (c *Context) sessionIdentities(scope string, a drivers.CallArgs) (string, string, string) {
	requirements := make([][3]string, 0, len(c.Spec.Requirements))
	for _, r := range c.Spec.Requirements {
		requirements = append(requirements, [3]string{r.ID, r.Title, r.Milestone})
	}
	goal := []any{c.Spec.Goal, c.Spec.NonGoals, requirements}
	contractInput := []any{c.Spec}
	if p := c.Packets[strings.TrimPrefix(scope, "final:")]; p != nil {
		goal = append(goal, p.Goal, p.Requirements, p.Boundaries, p.Body)
		contractInput = append(contractInput, p, p.Body)
	} else {
		for _, milestone := range c.Spec.Milestones {
			if milestone.ID == scope {
				goal = append(goal, [2]string{milestone.ID, milestone.Title})
				break
			}
		}
		for _, p := range c.PacketList {
			if milestone, _ := contract.MilestoneOfPacket(p.PacketID); milestone == scope {
				// Plan/Judge history belongs to this milestone's declared packet
				// goals, not merely its ID. Acceptance-only revisions stay compatible.
				goal = append(goal, []any{p.PacketID, p.Goal, p.Requirements, p.Boundaries, p.Body})
				contractInput = append(contractInput, p, p.Body)
			}
		}
	}
	// A saved terminal result needs its declared credential references, never a
	// currently available credential value. Keep those names when a secret is
	// removed between dispatch and result application, without changing child env.
	identityArgs := a
	identityArgs.APIKeys = map[string]string{}
	for name := range a.APIKeys {
		identityArgs.APIKeys[name] = ""
	}
	if key := c.Config.Channels[a.Binding.Channel].EnvKey; key != nil {
		identityArgs.APIKeys[*key] = ""
	}
	if packet := c.Packets[a.PacketID]; packet != nil && packet.NeedsLiveCredentials {
		if credentials, ok := c.State.Packets[a.PacketID].Evidence.Credentials.(map[string]any); ok {
			if injected, ok := credentials["injected-env"].([]any); ok {
				for _, value := range injected {
					if name, ok := value.(string); ok {
						identityArgs.APIKeys[name] = ""
					}
				}
			}
		}
	}
	bindingInput := []any{config.SessionPolicyVersion, a.Binding, a.Model, a.PermissionMode,
		c.Config.Channels[a.Binding.Channel].EnvKey, sessionRuntimeIdentity(identityArgs)}
	if a.SessionMode != "" {
		bindingInput = append(bindingInput, a.SessionMode)
	}
	binding := sessionHash(bindingInput)
	return binding, sessionHash(goal), sessionHash(contractInput)
}

// Persist the original role boundary so restart cannot reset its baseline to
// an already-mutated candidate. Review/Judge also bind the actual input proof.
func (c *Context) sessionGuard(role, scope string) (string, error) {
	switch role {
	case "plan":
		return c.planScopeIdentity()
	case "review":
		packet := c.Packets[strings.TrimPrefix(scope, "final:")]
		if packet == nil {
			return "", fmt.Errorf("session review scope has no current packet")
		}
		identity, err := c.verificationIdentity(packet)
		if err != nil {
			return "", err
		}
		evidence := c.State.Packets[packet.PacketID].Evidence
		return sessionHash([]any{identity, evidence.L1, evidence.FinalGate, evidence.DefectInjection}), nil
	case "judge", "judge-panel":
		candidate, err := c.candidateFingerprint(c.Cwd)
		if err != nil {
			return "", err
		}
		return sessionHash([]any{candidate, c.State.Milestones[scope].Gate}), nil
	default: // Execute may change its candidate.
		return "", nil
	}
}

func rejectChangedSessionGuard(role, before, after string, result *drivers.Result) {
	if before == "" || after == "" || before == after {
		return
	}
	result.Session.ResumeCandidate = false
	result.ErrorCode = errs.INVALID_REQUEST
	message := fmt.Sprintf("%s role guard changed during the recorded invocation; result rejected and history retired", role)
	result.ErrorMessage = &message
}

func (c *Context) sessionRecord(role, scope string) *state.SessionRecord {
	if c.State.Sessions == nil {
		return nil
	}
	return c.State.Sessions.Records[sessionKey(role, scope)]
}

func (c *Context) sessionReplay(role, scope string) bool {
	r := c.sessionRecord(role, scope)
	return r != nil && r.Last.Status == "received"
}

func (c *Context) sessionGuardViolated(role, scope string) bool {
	r := c.sessionRecord(role, scope)
	return r != nil && r.Last.GuardBefore != "" && r.Last.GuardAfter != "" && r.Last.GuardBefore != r.Last.GuardAfter
}

func (c *Context) sessionBlock(key, detail string) error {
	err := fmt.Errorf("session %s requires reconciliation: %s; resume and fresh replay are blocked", key, detail)
	c.State.Status = "escalated"
	entry := map[string]any{"at": c.nowISO(), "reason": err.Error(), "session-key": key}
	c.State.Inbox = append(c.State.Inbox, entry)
	c.appendInbox(entry)
	if c.State.Sessions != nil {
		if r := c.State.Sessions.Records[key]; r != nil {
			r.Action, r.Reason, r.ResumeCandidate = "wait", detail, false
		}
	}
	c.save()
	if c.fatal == nil {
		c.fatal = err
	}
	return c.fatal
}

func safeSessionOutcome(r drivers.Result, id string) bool {
	s := r.Session
	return s != nil && s.RequestID == id && s.CleanupConfirmed && r.CleanupError == "" &&
		(s.Dispatch == "dispatched" || s.Dispatch == "not-dispatched") &&
		(s.Outcome == "completed" || s.Outcome == "failed" || s.Outcome == "cancelled")
}

func applySessionFacts(record *state.SessionRecord, result drivers.Result, digest, candidate string) {
	s := result.Session
	record.Last.ReceiptDigest, record.Last.CandidateAfter = digest, candidate
	record.Last.Status, record.ResumeCandidate = "blocked", false
	if !record.Last.Persistent {
		record.ThreadID = ""
	}
	if s != nil {
		record.Last.Dispatch, record.Last.Outcome = s.Dispatch, s.Outcome
		record.Last.CleanupConfirmed, record.Last.UsageStatus = s.CleanupConfirmed, s.UsageStatus
		record.Protocol = s.Protocol
		if record.Last.Persistent && s.SessionID != "" {
			record.ThreadID = s.SessionID
		}
	}
	if safeSessionOutcome(result, record.Last.ID) && candidate != "" {
		record.Last.Status = "received"
		record.ResumeCandidate = record.Last.Persistent && s.ResumeCandidate && s.SessionID != ""
		// A proven startup/admission failure did not touch the previously
		// confirmed history. Keep that checkpoint for the existing bounded retry.
		if record.Last.Persistent && record.Last.RequestedThread != "" && (s.SessionID == "" || s.SessionID == record.Last.RequestedThread) && s.Dispatch == "not-dispatched" && s.Outcome != "completed" {
			record.ThreadID, record.ResumeCandidate = record.Last.RequestedThread, true
		}
	}
	observeSessionQuality(record, result)
}

var persistSessionReceipt = state.WriteSessionReceipt
var loadSessionReceipt = state.ReadSessionReceipt

func (c *Context) readSessionResult(key string, r *state.SessionRecord) (*state.SessionReceipt, drivers.Result, string, error) {
	receipt, digest, err := loadSessionReceipt(c.TaskDir, key, r.Last.ID, r.Last.ReceiptDigest)
	if err != nil {
		return nil, drivers.Result{}, "", err
	}
	if receipt.Cwd != r.Cwd || receipt.Binding != r.Binding {
		return nil, drivers.Result{}, "", fmt.Errorf("receipt binding or directory changed")
	}
	var result drivers.Result
	if err := json.Unmarshal(receipt.Result, &result); err != nil {
		return nil, result, "", err
	}
	s := result.Session
	if s == nil || s.RequestID != r.Last.ID || s.Protocol == "" ||
		(s.Dispatch != "dispatched" && s.Dispatch != "not-dispatched" && s.Dispatch != "unknown") ||
		(s.Outcome != "completed" && s.Outcome != "failed" && s.Outcome != "cancelled" && s.Outcome != "unknown") ||
		(s.UsageStatus != "complete" && s.UsageStatus != "partial" && s.UsageStatus != "unknown") {
		return nil, result, "", fmt.Errorf("receipt invocation identity or facts are invalid")
	}
	if u := result.Usage; (u != nil && (u.Input < 0 || u.Output < 0 || u.Reasoning < 0 || u.CacheRead < 0 || u.CacheWrite < 0 || u.TotalTokens < 0)) || (result.CostUSD != nil && *result.CostUSD < 0) {
		return nil, result, "", fmt.Errorf("receipt usage or cost is invalid")
	}
	if r.Role != "execute" && (r.Last.GuardBefore == "" || receipt.GuardBefore != r.Last.GuardBefore) {
		return nil, result, "", fmt.Errorf("original role guard checkpoint is missing or mismatched")
	}
	if !safeSessionOutcome(result, r.Last.ID) {
		// The bound receipt can attest usage without granting permission to
		// replay effects. Return that evidence so recovery accounts it first.
		return receipt, result, digest, fmt.Errorf("dispatch, outcome or cleanup remains unknown")
	}
	if r.Role != "execute" {
		if receipt.GuardAfter == "" {
			return receipt, result, digest, fmt.Errorf("terminal role guard checkpoint is missing")
		}
		rejectChangedSessionGuard(r.Role, receipt.GuardBefore, receipt.GuardAfter, &result)
	}
	return receipt, result, digest, nil
}

func (c *Context) accountSessionReceipt(r *state.SessionRecord, result drivers.Result) {
	if result.Recovered && r.Last.Accounted && r.Last.Accounting != nil {
		if err := c.reconcileSessionAccounting(r, result); err != nil && c.fatal == nil {
			c.fatal = err
		}
		return
	}
	packetID := ""
	if r.Role == "execute" || r.Role == "review" {
		packetID = strings.TrimPrefix(r.Scope, "final:")
	}
	c.recordSpawnMetrics(spawnMetricsArgs{role: r.Role, packetID: packetID, model: result.Model, result: &result, attempt: &r.Last.Attempt})
}

// Always reconcile under the task lease, including after a mode/backend change.
// A lost terminal receipt never grants permission to repeat its possible effects.
func (c *Context) reconcileSessionReceipts() error {
	if legacy, ok := c.State.PlanSession.(map[string]any); ok && legacy["outcome-unknown"] == true {
		return c.sessionBlock("plan/legacy", "previous resident Plan result remains unknown")
	}
	if c.State.Sessions == nil {
		return nil
	}
	for key, r := range c.State.Sessions.Records {
		if r.Last.Status != "intent" && r.Last.Status != "blocked" && (r.Last.Status != "received" || r.Last.Accounted) {
			continue
		}
		recovered := r.Last.Status != "received"
		receipt, result, digest, err := c.readSessionResult(key, r)
		if err != nil && r.Last.Recovery != nil {
			if recoveryErr := c.recoverSessionReceipt(key, r); recoveryErr == nil {
				receipt, result, digest, err = c.readSessionResult(key, r)
			} else if receipt == nil {
				err = recoveryErr
			}
		}
		if err != nil {
			if receipt != nil {
				applySessionFacts(r, result, digest, receipt.CandidateAfter)
				r.Last.GuardAfter = receipt.GuardAfter
				// A missing post-call proof still forbids application even when
				// the process itself has an otherwise safe terminal result.
				r.Last.Status, r.ResumeCandidate = "blocked", false
				c.accountSessionReceipt(r, result)
			}
			return c.sessionBlock(key, err.Error())
		}
		applySessionFacts(r, result, digest, receipt.CandidateAfter)
		r.Last.GuardAfter = receipt.GuardAfter
		// Attribution does not depend on today's candidate, binding or guards.
		// Account the original bound receipt before any replay prerequisite can
		// block it, keeping the role result unconsumed for normal application.
		c.accountSessionReceipt(r, result)
		if r.Last.Status != "received" {
			return c.sessionBlock(key, "terminal candidate checkpoint is missing")
		}
		if recovered {
			r.Action, r.Reason = "reconcile", "durable terminal receipt recovered; apply without dispatch"
		}
		c.save()
		if c.fatal != nil {
			return c.fatal
		}
	}
	return nil
}

// Received results remain replayable until their stage and accounting are saved
// together. requestId correlates evidence; it is not a backend idempotency key.
func (c *Context) invokeScoped(role, scope string, args drivers.CallArgs, call func(drivers.CallArgs) drivers.Result) (out drivers.Result, callErr error) {
	defer c.measureManagedInvocation(role, scope)()
	if c.parallel != nil {
		release, err := c.parallel.enter(c, args.Binding.Channel)
		if err != nil {
			return drivers.Result{}, err
		}
		c.parallel.release = release
		defer func() {
			if callErr != nil {
				c.parallel.finishTurn()
			}
		}()
	}

	if err := c.err(); err != nil {
		return drivers.Result{}, err
	}
	if err := c.reconcileSessionReceipts(); err != nil {
		return drivers.Result{}, err
	}
	key := sessionKey(role, scope)
	if c.sessionReports == nil {
		c.sessionReports = map[string][]byte{}
	}
	r := c.sessionRecord(role, scope)
	if err := c.checkPendingSessionRole(key, role, scope); err != nil {
		return drivers.Result{}, err
	}
	var scopeErr error
	args.CandidateScope, scopeErr = c.sharedInvocationScope(role, scope)
	if scopeErr != nil {
		return drivers.Result{}, scopeErr
	}
	if r != nil && (r.Last.Status == "received" || args.ExpectedSessionID != "") {
		args.CandidateScope = r.CandidateScope
	}
	args.WorkspaceCwd = c.WorkspaceCwd
	// A terminal result from the previous version must first reconcile with its
	// original identity. A later invocation may rebuild in the project workspace.
	if r != nil && (r.Last.Status == "received" || (r.WorkspaceCwd == "" && r.Cwd == c.WorkspaceCwd)) {
		args.WorkspaceCwd = r.WorkspaceCwd
	}
	args.SessionTitle = "Flywheel · " + role + "/" + scope + " · " + c.State.Slug
	originalPrompt := args.Prompt
	args.Prompt = c.sessionPrompt(role, scope, args)
	tracked, persistent := c.trackedSessions(role, args.Binding.Harness), c.scopedSessions(role, args.Binding.Harness)
	if args.SessionMode != "" {
		persistent = args.SessionMode == "scoped" && tracked
	}
	if r == nil && !tracked {
		return call(args), nil
	}
	if r != nil && r.Last.Status != "received" && !tracked {
		r.ResumeCandidate, r.Action, r.Reason = false, "rebuild", "backend does not support durable invocation tracking"
		c.save()
		if err := c.err(); err != nil {
			return drivers.Result{}, err
		}
		return call(args), nil
	}
	cwd, err := canonicalDirectory(args.Cwd)
	if err != nil {
		return drivers.Result{}, err
	}
	args.Cwd = cwd
	binding, goal, contractIdentity := c.sessionIdentities(scope, args)
	if r != nil && r.Last.Status == "received" {
		// Plan may have changed its own contracts. The exact post-call candidate
		// plus its original outside-contract guard bind those permitted outputs.
		if r.Task != c.TaskDir || r.Cwd != cwd || r.Binding != binding || (role != "plan" && (r.Goal != goal || r.Contract != contractIdentity)) {
			return drivers.Result{}, c.sessionBlock(key, "pending result belongs to different task, binding, goal or contract")
		}
		receipt, result, _, err := c.readSessionResult(key, r)
		if err != nil {
			return drivers.Result{}, c.sessionBlock(key, err.Error())
		}
		candidate, err := c.sessionCandidateFingerprint(cwd, args.CandidateScope)
		if err != nil || candidate != receipt.CandidateAfter {
			return drivers.Result{}, c.sessionBlock(key, "candidate changed after the recorded terminal result")
		}
		guard, err := c.sessionGuard(role, scope)
		if err != nil || guard != receipt.GuardAfter {
			return drivers.Result{}, c.sessionBlock(key, "role guard or original verification evidence changed after the recorded result")
		}
		c.sessionReports[key] = receipt.Report
		c.Log(fmt.Sprintf("session %s: reconcile invocation %s without model dispatch", key, r.Last.ID))
		return result, nil
	}
	if c.State.Sessions == nil {
		c.State.Sessions = &state.SessionLedger{Version: 2, Records: map[string]*state.SessionRecord{}}
	}
	c.State.Sessions.Version = 2
	action, reason, generation, thread := "create", "first invocation in this role scope", 1, ""
	protocol := "flywheel/admission-v1"
	if r != nil {
		generation = r.Generation
		if persistent && r.Task == c.TaskDir && r.Cwd == cwd && r.Binding == binding && r.Goal == goal && r.ResumeCandidate {
			action, reason, thread = "resume", "same role, goal and effective binding after confirmed terminal receipt", r.ThreadID
			protocol = r.Protocol
		} else {
			action, reason, generation = "rebuild", "history unavailable or scope, goal, directory or binding changed", generation+1
		}
	}
	if action == "resume" {
		if qualityReason := sessionQualityReason(role, r, c.Config.Sessions.Quality); qualityReason != "" {
			action, reason, generation, thread = "rebuild", qualityReason, generation+1, ""
		}
	}
	if !persistent && role == "plan" && args.SessionID != "" && c.Config.Sessions.Roles["plan"] != "fresh" {
		action, reason, thread = "resume", "explicit legacy resident Plan binding matched", args.SessionID
	}
	if args.ExpectedSessionID != "" && (thread != args.ExpectedSessionID || action != "resume") {
		return drivers.Result{}, fmt.Errorf("exact session resume refused: %s", reason)
	}
	candidate, err := c.sessionCandidateFingerprint(cwd, args.CandidateScope)
	if err != nil {
		return drivers.Result{}, err
	}
	guardBefore, err := c.sessionGuard(role, scope)
	if err != nil {
		return drivers.Result{}, err
	}
	attempt := 0
	if ps := c.State.Packets[scope]; ps != nil {
		attempt = ps.Attempts
	}
	previous := r
	contextArgs := args
	contextArgs.Prompt = originalPrompt
	contextCheckpoint, contextPrompt := c.sessionContextPrompt(role, scope, action, candidate, previous, contextArgs)
	contextArgs.Prompt = contextPrompt
	args.Prompt = c.sessionPrompt(role, scope, contextArgs)
	if action == "rebuild" {
		args.Prompt = sessionRebuildHandoff(previous, reason) + args.Prompt
	}
	historyMode := "fresh"
	if persistent {
		historyMode = "scoped"
	} else if action == "resume" && role == "plan" {
		historyMode = "legacy-plan"
	}
	r = &state.SessionRecord{Context: contextCheckpoint, Configuration: &state.SessionConfiguration{Harness: args.Binding.Harness, Model: args.Model, Provider: args.Binding.Provider, Channel: args.Binding.Channel, Thinking: args.Binding.Thinking, Permission: args.PermissionMode, SessionMode: historyMode}, Task: c.TaskDir, Role: role, Scope: scope, Cwd: cwd, WorkspaceCwd: args.WorkspaceCwd, CandidateScope: args.CandidateScope, Binding: binding, Goal: goal, Contract: contractIdentity,
		Generation: generation, Action: action, Reason: reason, ThreadID: thread,
		Quality: nextSessionQuality(previous, action, reason),
		Last: &state.SessionInvocation{EngineRunID: c.runID, ID: "call-" + rand.Text(), Status: "intent", StartedAt: c.nowISO(), Attempt: attempt,
			Persistent: persistent, RequestedThread: thread, CandidateBefore: candidate, GuardBefore: guardBefore,
			Dispatch: "unknown", Outcome: "unknown", UsageStatus: "unknown"}}
	if !persistent {
		r.ThreadID = ""
	}
	c.State.Sessions.Records[key] = r
	if err := c.prepareSessionLifecycle(r, &args); err != nil {
		return drivers.Result{}, err
	}
	c.save()
	if c.fatal != nil {
		return drivers.Result{}, c.fatal
	}
	args.SessionID, args.PersistentSession, args.RequestID = thread, persistent, r.Last.ID
	c.Log(fmt.Sprintf("session %s: %s (%s)", key, action, reason))
	var result drivers.Result
	cancelledBeforeDispatch := c.err() != nil
	if cancelledBeforeDispatch {
		// Go has not called the adapter, so this exact invocation has no
		// backend effects or charges. Save that positive local fact instead
		// of leaving an ambiguous intent or inventing a physical exit code.
		r.Reason = "cancelled locally before adapter dispatch; no backend call or charge"
		result = drivers.Result{Harness: args.Binding.Harness, Model: args.Model, Cancelled: true,
			Usage: &drivers.Usage{}, CostUSD: new(float64),
			Session: &drivers.SessionOutcome{Protocol: protocol, RequestID: args.RequestID,
				Dispatch: "not-dispatched", Outcome: "cancelled", CleanupConfirmed: true, UsageStatus: "complete"}}
	} else {
		result = call(args)
	}
	if result.Session != nil {
		unsent := safeSessionOutcome(result, r.Last.ID) && result.Session.Dispatch == "not-dispatched" && result.Session.Outcome != "completed"
		invalid := thread != "" && ((result.Session.SessionID != "" && result.Session.SessionID != thread) || (!unsent && (result.Session.SessionID != thread || !result.Session.ResumeAccepted)))
		for otherKey, other := range c.State.Sessions.Records {
			if otherKey != key && result.Session.SessionID != "" && other.ThreadID == result.Session.SessionID && other.Protocol == result.Session.Protocol {
				invalid = true
			}
		}
		if invalid {
			result.Session.Dispatch, result.Session.Outcome, result.Session.UsageStatus = "unknown", "unknown", "unknown"
			result.Session.ResumeCandidate, result.Session.SessionID = false, ""
			result.Usage, result.CostUSD = nil, nil
		}
	}
	if result.Session == nil || result.Session.RequestID != r.Last.ID {
		result.Session = &drivers.SessionOutcome{RequestID: r.Last.ID, Protocol: "unconfirmed", Dispatch: "unknown", Outcome: "unknown", UsageStatus: "unknown"}
		result.Usage, result.CostUSD = nil, nil
	}
	after, snapshotErr := c.sessionCandidateFingerprint(cwd, args.CandidateScope)
	guardAfter, guardErr := c.sessionGuard(role, scope)
	rejectChangedSessionGuard(role, guardBefore, guardAfter, &result)
	var report []byte
	if role == "execute" {
		report, _ = readUnitReport(cwd, filepath.Join(c.taskDirPrefix(), "reports", scope+".done.json"))
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return drivers.Result{}, c.sessionBlock(key, "result cannot be serialized")
	}
	digest, err := persistSessionReceipt(c.TaskDir, key, state.SessionReceipt{Version: 1, InvocationID: r.Last.ID, Key: key,
		Task: c.TaskDir, Cwd: cwd, Binding: binding, CandidateAfter: after, GuardBefore: guardBefore, GuardAfter: guardAfter, Report: report, Result: raw})
	if err != nil {
		return drivers.Result{}, c.sessionBlock(key, "terminal receipt could not be saved: "+err.Error())
	}
	applySessionFacts(r, result, digest, after)
	r.Last.GuardAfter = guardAfter
	if c.sessionGuardViolated(role, scope) {
		r.Reason = *result.ErrorMessage
	}
	c.sessionReports[key] = report
	c.save()
	if c.fatal != nil {
		return drivers.Result{}, c.fatal
	}
	if snapshotErr != nil || guardErr != nil || r.Last.Status == "blocked" {
		c.recordSpawnMetrics(spawnMetricsArgs{role: role, packetID: args.PacketID, model: args.Model, channel: args.Binding.Channel, prompt: args.Prompt, result: &result})
		return result, c.sessionBlock(key, "dispatch, outcome, cleanup or terminal candidate is unconfirmed")
	}
	if c.err() != nil {
		// Cancellation prevents stage application, not durable accounting of
		// the invocation that already returned. Keep normal results received
		// for later replay; a local no-dispatch admission has no stage output.
		c.recordSpawnMetrics(spawnMetricsArgs{role: role, packetID: args.PacketID, model: args.Model, channel: args.Binding.Channel, prompt: args.Prompt, result: &result})
		if cancelledBeforeDispatch {
			c.consumeSession(role, scope)
		}
		c.save()
		return result, c.err()
	}
	return result, nil
}

func (c *Context) checkPendingSessionRole(key, role, scope string) error {
	if c.State.Sessions != nil {
		for otherKey, other := range c.State.Sessions.Records {
			if otherKey != key && other.Last.Status == "received" && !(role == "judge-panel" && other.Role == "judge" && other.Scope == scope) && !(role == "judge" && other.Role == "judge-panel" && other.Scope == scope) {
				return c.sessionBlock(otherKey, "apply the recorded role result before starting another invocation")
			}
		}
	}
	return nil
}

func (c *Context) consumeSession(role, scope string) {
	if r := c.sessionRecord(role, scope); r != nil && r.Last.Status == "received" {
		r.Last.Status = "consumed"
	}
}

func (c *Context) retireSession(role, scope, reason string) {
	if r := c.sessionRecord(role, scope); r != nil {
		r.ResumeCandidate, r.Reason = false, reason
		if r.Last.Accounted {
			c.consumeSession(role, scope)
		}
	}
}

func (c *Context) pendingSessionMilestone(milestone string) bool {
	if c.State.Sessions == nil {
		return false
	}
	for _, r := range c.State.Sessions.Records {
		scope := strings.TrimPrefix(r.Scope, "final:")
		if r.Last.Status == "received" && r.Role != "plan" && (scope == milestone || strings.HasPrefix(scope, strings.ReplaceAll(milestone, "-", "")+"-")) {
			return true
		}
	}
	return false
}
