package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ckanner/flywheel/internal/circuitbreaker"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/decisionexec"
	"github.com/ckanner/flywheel/internal/diagnostic"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
)

// The action set is generated from the frozen contract and completed failures.
// JEV never supplies commands, model names, permissions, or additional retries.
func (c *Context) failureActions(packet *contract.Packet, code errs.ErrorCode, spin bool) ([]decision.Action, string) {
	ps := c.State.Packets[packet.PacketID]
	actions := []decision.Action{}
	add := func(id string, kind decision.ActionKind, description string, binding any) {
		if len(actions) < 16 {
			actions = append(actions, decision.Action{ID: id, Kind: kind, Description: description, BindingHash: sessionHash(binding)})
		}
	}
	baseline := "repair"
	if !spin {
		add("repair", decision.Repair, "Continue one bounded repair attempt with the current executor and the recorded failure feedback.", []any{packet, ps.Attempts, ps.FailedAttempts, executionPolicyDecisionFacts(ps.ExecutionPolicy)})
	}
	canPlan := c.quick() || packet.Escalation.OnFail == "return-to-planner" && ps.PlanRevisions < 1
	if canPlan {
		add("replan", decision.Replan, "Use the existing bounded planning escalation to revisit the strategy within the same approved goal; Quick first upgrades to Full.", []any{packet.Escalation, ps.PlanRevisions, c.workflowEvidencePolicy()})
		if spin {
			baseline = "replan"
		}
	}
	record, route, err := c.executionPolicy(packet)
	if err != nil {
		c.fatal = err
		return nil, ""
	}
	if record != nil && record.Switches < route.MaxSwitches {
		for _, id := range route.Allowed {
			if id == record.Current {
				continue
			}
			b := c.Config.ExecutionPolicy.Bindings[id]
			if c.executionCandidateError(packet, b) != nil {
				continue
			}
			actionID := "model-" + strconv.Itoa(slices.Index(route.Allowed, id))
			add(actionID, decision.Model, c.executionChoiceDescription(id, b), []any{id, b, record.Identity, record.Current, record.Switches})
			index := slices.Index(route.QualityOrder, record.Current)
			if spin && code == errL1Failed && index >= 0 && index+1 < len(route.QualityOrder) && route.QualityOrder[index+1] == id {
				baseline = actionID
			}
		}
	}
	if !spin {
		for i, check := range packet.FeedbackChecks {
			if strings.TrimSpace(check.Purpose) != "" {
				add("feedback-"+strconv.Itoa(i), decision.Diagnostic, "Run approved short feedback check "+strconv.Itoa(i+1)+": "+check.Purpose+". Collect supplemental evidence once before another repair; final acceptance remains mandatory.", []any{i, check, c.Config.Verify})
			}
		}
		var l1 struct {
			Commands []verify.GateResult `json:"commands"`
		}
		raw, _ := json.Marshal(ps.Evidence.L1)
		_ = json.Unmarshal(raw, &l1)
		for i, result := range l1.Commands {
			if i < len(packet.Acceptance.L1) && !result.Pass && result.ObservationComplete {
				add("recheck-"+strconv.Itoa(i), decision.Diagnostic, "Run the already approved failed L1 check "+result.CheckID+" once to collect fresh feedback before the next repair. It cannot replace final verification.", []any{i, packet.Acceptance.L1[i], c.Config.Verify})
			}
		}
	}
	return actions, baseline
}

func boundedDecisionText(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	s = s[:limit]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func (c *Context) failureSnapshot(packet *contract.Packet, code errs.ErrorCode, progress circuitbreaker.ProgressEvidence, spin bool, id string, revision int64) (decision.Snapshot, error) {
	identity, candidate, err := c.verificationSnapshot(packet)
	if err != nil {
		return decision.Snapshot{}, err
	}
	actions, baseline := c.failureActions(packet, code, spin)
	ps := c.State.Packets[packet.PacketID]
	var currentExecutor *state.SessionConfiguration
	if ps.ExecutionPolicy != nil && c.Config.ExecutionPolicy != nil {
		binding := executionConfiguration(c.Config.ExecutionPolicy.Bindings[ps.ExecutionPolicy.Current])
		currentExecutor = &binding
	}
	facts, _ := json.Marshal(struct {
		Code                     errs.ErrorCode `json:"error"`
		Attempt, Failed, Maximum int
		Complete                 bool
		Checks                   []circuitbreaker.CheckObservation
		CurrentExecutor          *state.SessionConfiguration `json:"current-executor,omitempty"`
	}{code, ps.Attempts, ps.FailedAttempts, packet.Escalation.MaxRetries, progress.Complete, progress.Checks, currentExecutor})
	return decision.Snapshot{Version: 2, ID: id,
		Identity:     decision.Identity{TaskID: c.State.Slug, RunID: sessionHash(c.TaskDir), Revision: revision, ContractHash: sessionHash([]any{c.Spec, packet, c.Config.ExecutionPolicy, c.Config.Verify, c.workflowEvidencePolicy()}), CandidateHash: candidate, ObservationHash: sessionHash([]any{identity, ps.LastFeedback, ps.Attempts, ps.FailedAttempts, ps.PlanRevisions})},
		Facts:        []decision.Fact{{Category: decision.TaskSummary, Text: boundedDecisionText(packet.Goal, 2048)}, {Category: decision.DiagnosticFacts, Text: boundedDecisionText(string(facts), 2048)}},
		MissingFacts: []string{"These are process/check observations, not proof of the root cause. Raw logs and private transcripts are not included."},
		Actions:      actions, BaselineID: baseline, SemanticUncertainty: true}, nil
}

func (c *Context) decisionContext() context.Context {
	if c.RunContext != nil {
		return c.RunContext
	}
	return context.Background()
}

func (c *Context) runtimeJEVConfig(frozen *config.JEVConfig) (*config.JEVConfig, string) {
	cfg, err := diagnostic.LoadConfigurationContext(c.decisionContext(), c.Cwd, c.TaskDir)
	if err != nil {
		return nil, "config-unavailable"
	}
	if !cfg.JEV.Enabled {
		return nil, "disabled"
	}
	if cfg.Budget.Enforce == "hard" || c.Config.Budget.Enforce == "hard" {
		return nil, "hard-budget-without-enforceable-bound"
	}
	if executionChoiceConfigurationHash(cfg) != executionChoiceConfigurationHash(c.Config) {
		return nil, "execution-policy-changed"
	}
	j := cfg.JEV
	if j.Backend == "subscription" {
		current, err := cfg.ResolveJEVSubscription()
		expected, initialErr := c.Config.ResolveJEVSubscription()
		if c.State.RuntimeDecisions != nil {
			expected = c.State.RuntimeDecisions.Subscription
		}
		if err != nil || initialErr != nil || sessionHash(current) != sessionHash(expected) {
			return nil, "subscription-binding-changed"
		}
	}
	if frozen != nil {
		if (j.Backend == "subscription") != (frozen.Backend == "subscription") || j.Profile != frozen.Profile || j.Endpoint != frozen.Endpoint || j.Model != frozen.Model || j.KeyEnv != frozen.KeyEnv || j.TimeoutMS != frozen.TimeoutMS || j.MaxCalls < frozen.MaxCalls || j.MinApplicability > frozen.MinApplicability || j.MinChoice > frozen.MinChoice {
			return nil, "policy-changed"
		}
		for _, category := range frozen.AllowedData {
			if !slices.Contains(j.AllowedData, category) {
				return nil, "data-policy-changed"
			}
		}
		return frozen, "enabled"
	}
	return &j, "enabled"
}

var subscriptionDecisionOptions = decisionexec.Options

func (c *Context) decisionOptions(j *config.JEVConfig) decision.Options {
	if j.Backend == "subscription" {
		binding, _ := c.Config.ResolveJEVSubscription()
		if c.State.RuntimeDecisions != nil {
			binding = c.State.RuntimeDecisions.Subscription
		}
		options := subscriptionDecisionOptions(c.decisionContext(), binding, c.Cwd, c.TaskDir, time.Duration(j.TimeoutMS)*time.Millisecond)
		options.AllowedKinds = []decision.ActionKind{decision.Diagnostic, decision.Repair, decision.Replan, decision.Model}
		return options
	}
	lookup := c.EnvLookup
	if lookup == nil {
		lookup = os.LookupEnv
	}
	key, _ := lookup(j.KeyEnv)
	return decision.Options{Endpoint: j.Endpoint, Model: j.Model, ExpectedModel: j.Model, APIKey: key, Timeout: time.Duration(j.TimeoutMS) * time.Millisecond,
		AllowedKinds: []decision.ActionKind{decision.Diagnostic, decision.Repair, decision.Replan, decision.Model}}
}

func (c *Context) initializeRuntimeDecisions(j *config.JEVConfig) *state.RuntimeDecisions {
	imported := 0
	if source := c.State.PreparationSource; source != nil {
		preparation, err := state.LoadPreparation(c.TaskDir)
		if err != nil || preparation.RunID != source.RunID || preparation.ContractHash != source.ContractHash {
			return nil
		}
		for _, round := range state.PreparationRounds(preparation) {
			for _, receipt := range round.Decisions {
				if receipt.Status != "skipped" {
					imported++
				}
			}
		}
	}
	ledger := &state.RuntimeDecisions{Version: 1, Config: j, Used: imported, ImportedCalls: imported, Status: "enabled", Records: []*state.RuntimeDecisionRecord{}}
	if j.Backend == "subscription" {
		ledger.Subscription, _ = c.Config.ResolveJEVSubscription()
	}
	c.State.RuntimeDecisions = ledger
	return ledger
}

// Parallel decisions use the parent state's mutex across reservation, HTTP and
// adoption. No clone can reserve a separate budget or overwrite another result.
func (c *Context) runtimeFailureDecision(packet *contract.Packet, code errs.ErrorCode, progress circuitbreaker.ProgressEvidence, spin bool) string {
	if c.parallel == nil {
		return c.failureDecision(packet, code, progress, spin)
	}
	g := c.parallel.group
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := c.parallel.merge(c); err != nil {
		c.fatal = err
		return ""
	}
	g.logMu.Lock()
	defer g.logMu.Unlock()
	prior := g.owner.RunContext
	g.owner.RunContext = c.RunContext
	defer func() { g.owner.RunContext = prior }()
	result := g.owner.failureDecision(packet, code, progress, spin)
	c.copyRuntimeDecisionState(packet, g.owner)
	if err := g.owner.err(); err != nil {
		c.fatal = err
		g.cancel()
	}
	return result
}

// Keep owner-side cold-start retirement as well as action effects. A later
// worker save must not restore the old resumable execution history.
func (c *Context) copyRuntimeDecisionState(packet *contract.Packet, owner *Context) {
	*c.State.Packets[packet.PacketID] = *jsonClone(owner.State.Packets[packet.PacketID])
	c.State.Budget = jsonClone(owner.State.Budget)
	c.State.RuntimeDecisions = jsonClone(owner.State.RuntimeDecisions)
	if record := owner.sessionRecord("execute", packet.PacketID); record != nil {
		if c.State.Sessions == nil {
			c.State.Sessions = &state.SessionLedger{Version: owner.State.Sessions.Version, Records: map[string]*state.SessionRecord{}}
		}
		c.State.Sessions.Records[sessionKey("execute", packet.PacketID)] = jsonClone(record)
	}
}

func (c *Context) failureDecision(packet *contract.Packet, code errs.ErrorCode, progress circuitbreaker.ProgressEvidence, spin bool) string {
	ps := c.State.Packets[packet.PacketID]
	if c.err() != nil || !progress.Complete || (code != errL1Failed && code != errReviewReturned) || ps.FailedAttempts >= packet.Escalation.MaxRetries {
		return ""
	}
	if r := c.sessionRecord("execute", packet.PacketID); r != nil && r.Last.Status != "consumed" {
		return ""
	}
	ledger := c.State.RuntimeDecisions
	var frozen *config.JEVConfig
	if ledger != nil {
		frozen = ledger.Config
	}
	j, status := c.runtimeJEVConfig(frozen)
	if ledger != nil {
		ledger.Status = status
	}
	if j == nil || !slices.Contains(j.AllowedData, decision.TaskSummary) || !slices.Contains(j.AllowedData, decision.DiagnosticFacts) || j.Backend != "subscription" && c.decisionOptions(j).APIKey == "" || c.budgetGate() == "stop" {
		return ""
	}
	if ledger == nil {
		ledger = c.initializeRuntimeDecisions(j)
	}
	if ledger == nil || ledger.Used >= j.MaxCalls || len(ledger.Records) >= 100 {
		return ""
	}
	id := fmt.Sprintf("failure-%s-%d-%d-%d", packet.PacketID, ps.PlanRevisions, ps.Attempts, ps.FailedAttempts)
	snapshot, err := c.failureSnapshot(packet, code, progress, spin, id, c.revision+1)
	if err != nil || len(snapshot.Actions) < 2 || decision.Validate(snapshot, j.DecisionPolicy()) != nil {
		return ""
	}
	record := &state.RuntimeDecisionRecord{ModelBindings: c.runtimeModelBindings(packet, snapshot.Actions), ID: id, Packet: packet.PacketID, Attempt: ps.Attempts, PlanRevisions: ps.PlanRevisions, StartedAt: c.nowISO(), Snapshot: &snapshot, Action: snapshot.BaselineID, Status: "pending"}
	record.Execution = runtimeExecutionCheckpoint(c.State, packet.PacketID)
	ledger.Records = append(ledger.Records, record)
	ledger.Used++
	ps.PendingStrategy = id
	c.save()
	if c.err() != nil {
		return ""
	}
	options := c.decisionOptions(j)
	receipt, err := decision.Evaluate(c.decisionContext(), filepath.Join(c.TaskDir, ".investigation", "runtime-decisions"), snapshot, j.DecisionPolicy(), options)
	if err != nil {
		receipt.Status, receipt.Reason = "unknown", "decision-journal-or-request-error"
	}
	record.Receipt = &receipt
	c.accountRuntimeDecision(record)
	selection := decision.Selection{ActionID: snapshot.BaselineID, Reason: "deterministic-baseline"}
	current, currentErr := c.failureSnapshot(packet, code, progress, spin, id, snapshot.Identity.Revision)
	active, status := c.runtimeJEVConfig(j)
	ledger.Status = status
	if c.err() == nil && currentErr == nil && sessionHash(snapshot) == sessionHash(current) && active != nil && c.budgetGate() != "stop" {
		selection = decision.Select(current, receipt, j.DecisionPolicy(), options)
	}
	record.Selection, record.Action = &selection, selection.ActionID
	if decision.CleanupUnknown(receipt) {
		ledger.Status = "native-cleanup-unconfirmed"
		c.save()
		c.fatal = fmt.Errorf("subscription decision cleanup requires reconciliation; refusing another action")
		return ""
	}
	if currentErr != nil || sessionHash(snapshot) != sessionHash(current) {
		record.Status, ps.PendingStrategy = "stale", ""
		c.save()
		return ""
	}
	if c.err() != nil || active == nil || c.State.Status == "budget-stopped" {
		record.Status = "baseline"
		ps.PendingStrategy = ""
		c.save()
		return ""
	}
	return c.applyRuntimeDecision(packet, record)
}

func (c *Context) accountRuntimeDecision(record *state.RuntimeDecisionRecord) {
	if record.Accounted {
		return
	}
	record.Accounted = true
	if record.Receipt.Status == "skipped" {
		c.State.RuntimeDecisions.Used--
		return
	}
	when, _ := time.Parse(time.RFC3339Nano, record.StartedAt)
	day := when.UTC().Format("2006-01-02")
	u := state.Usage{}
	if record.Receipt.Usage.Input != nil {
		u.Input = *record.Receipt.Usage.Input
	}
	if record.Receipt.Usage.Output != nil {
		u.Output = *record.Receipt.Usage.Output
	}
	state.RecordUsage(c.State, u, state.UsageOpts{Provider: "jev-decision", Day: day})
	bucket, present := c.State.Budget.Daily[day]
	if record.Receipt.Usage.Input == nil || record.Receipt.Usage.Output == nil {
		c.State.Budget.UnknownUsageCalls++
		bucket.UnknownUsageCalls++
	}
	c.State.Budget.UnknownCostCalls++
	bucket.UnknownCostCalls++
	if present {
		c.State.Budget.Daily[day] = bucket
	}
}

var runRuntimeFeedback = verify.RunGateCommands

func (c *Context) applyRuntimeDecision(packet *contract.Packet, record *state.RuntimeDecisionRecord) string {
	ps := c.State.Packets[packet.PacketID]
	record.Status = "baseline"
	if record.Selection.Adopted {
		record.Status = "applied"
	}
	ps.PendingStrategy = ""
	result := "failed"
	switch {
	case record.Action == "replan":
		ps.PendingStrategy = record.ID
		record.Status = "applied"
		result = "spin"
	case strings.HasPrefix(record.Action, "model-"):
		policy, route, err := c.executionPolicy(packet)
		index, parseErr := strconv.Atoi(strings.TrimPrefix(record.Action, "model-"))
		kind := "quality-upgrade"
		if record.Selection.Adopted {
			kind = "jev-model-selection"
		}
		if err != nil || policy == nil || parseErr != nil || index < 0 || index >= len(route.Allowed) || !c.switchExecutionPolicy(packet, policy, route, route.Allowed[index], kind, record.Selection.Reason+"; decision "+record.ID) {
			record.Status, result = "stale", ""
		}
	case strings.HasPrefix(record.Action, "feedback-") || strings.HasPrefix(record.Action, "recheck-"):
		checks, prefix := packet.FeedbackChecks, "feedback-"
		if strings.HasPrefix(record.Action, "recheck-") {
			checks, prefix = packet.Acceptance.L1, "recheck-"
		}
		index, err := strconv.Atoi(strings.TrimPrefix(record.Action, prefix))
		if err != nil || index < 0 || index >= len(checks) {
			record.Status, result = "stale", ""
			break
		}
		before, _, err := c.verificationSnapshot(packet)
		if err != nil {
			record.Status = "stale"
			c.save()
			if c.fatal == nil {
				c.fatal = err
			}
			return ""
		}
		ps.PendingStrategy, record.Status = record.ID, "feedback-running"
		c.save()
		if c.err() != nil {
			return ""
		}
		gate := runRuntimeFeedback([]contract.GateEntry{checks[index]}, verify.GateOpts{Context: c.RunContext, Cwd: c.unitCwd(packet), TimeoutMS: c.Config.Verify.CommandTimeoutMs, EnvAllowlist: c.Config.Verify.EnvAllowlist, KillGraceMS: c.Config.Watchdog.KillGraceMs})
		record.Feedback = gate
		ps.PendingStrategy, record.Status = "", "feedback-completed"
		feedback, ok := ps.LastFeedback.(map[string]any)
		if !ok || feedback == nil {
			feedback = map[string]any{}
			ps.LastFeedback = feedback
		}
		feedback["jev-feedback"] = gate
		after, _, afterErr := c.verificationSnapshot(packet)
		var problem error
		if afterErr != nil || before != after {
			ps.PendingStrategy, record.Status = record.ID, "feedback-stale"
			problem = fmt.Errorf("feedback check changed candidate or verification inputs; reconciliation required")
		}
		for _, r := range gate.Results {
			if r.CleanupError != "" {
				ps.PendingStrategy, record.Status = record.ID, "feedback-unknown"
				problem = fmt.Errorf("feedback process cleanup is unknown")
			}
		}
		c.save()
		if c.fatal == nil && problem != nil {
			c.fatal = problem
		}
		return result
	default:
		if record.Action != "repair" {
			record.Status, result = "stale", ""
		}
	}
	c.save()
	return result
}

// Only current facts can authorize an effect after a crash. Journal recovery
// is independent: it settles usage even when the old action is now stale.
func (c *Context) runtimeRecordCurrent(packet *contract.Packet, record *state.RuntimeDecisionRecord) bool {
	ps := c.State.Packets[packet.PacketID]
	if record.Snapshot == nil || record.Attempt != ps.Attempts || record.PlanRevisions != ps.PlanRevisions {
		return false
	}
	identity, candidate, err := c.verificationSnapshot(packet)
	if err != nil || candidate != record.Snapshot.Identity.CandidateHash || sessionHash([]any{c.Spec, packet, c.Config.ExecutionPolicy, c.Config.Verify, c.workflowEvidencePolicy()}) != record.Snapshot.Identity.ContractHash || sessionHash([]any{identity, ps.LastFeedback, ps.Attempts, ps.FailedAttempts, ps.PlanRevisions}) != record.Snapshot.Identity.ObservationHash {
		return false
	}
	if r := c.sessionRecord("execute", packet.PacketID); r != nil && r.Last.Status != "consumed" {
		return false
	}
	spin := true
	for _, action := range record.Snapshot.Actions {
		if action.ID == "repair" {
			spin = false
		}
	}
	var facts struct {
		Code errs.ErrorCode `json:"error-code"`
	}
	raw, _ := json.Marshal(ps.LastFeedback)
	_ = json.Unmarshal(raw, &facts)
	actions, baseline := c.failureActions(packet, facts.Code, spin)
	return baseline == record.Snapshot.BaselineID && sessionHash(actions) == sessionHash(record.Snapshot.Actions)
}

func (c *Context) recoveredFailureBaseline(packet *contract.Packet, record *state.RuntimeDecisionRecord) string {
	ps := c.State.Packets[packet.PacketID]
	ps.PendingStrategy = ""
	record.Status = "stale"
	raw, _ := json.Marshal(ps.LastFeedback)
	var facts struct {
		Code     errs.ErrorCode                    `json:"error-code"`
		Progress circuitbreaker.ProgressAssessment `json:"progress"`
	}
	_ = json.Unmarshal(raw, &facts)
	upgraded := c.upgradeExecutionPolicy(packet, facts.Code, facts.Progress)
	c.save()
	if upgraded {
		return "failed"
	}
	for _, action := range record.Snapshot.Actions {
		if action.ID == "repair" {
			return "failed"
		}
	}
	return "spin"
}

func (c *Context) resumeRuntimeDecision(packet *contract.Packet) string {
	if c.parallel != nil && c.State.Packets[packet.PacketID].PendingStrategy != "" {
		g := c.parallel.group
		g.mu.Lock()
		defer g.mu.Unlock()
		if err := c.parallel.merge(c); err != nil {
			c.fatal = err
			return ""
		}
		g.logMu.Lock()
		defer g.logMu.Unlock()
		prior := g.owner.RunContext
		g.owner.RunContext = c.RunContext
		defer func() { g.owner.RunContext = prior }()
		result := g.owner.resumeRuntimeDecision(packet)
		c.copyRuntimeDecisionState(packet, g.owner)
		if err := g.owner.err(); err != nil {
			c.fatal = err
			g.cancel()
		}
		return result
	}
	ps := c.State.Packets[packet.PacketID]
	if ps.PendingStrategy == "" {
		return ""
	}
	ledger := c.State.RuntimeDecisions
	if ledger == nil {
		c.fatal = fmt.Errorf("pending strategy has no decision ledger")
		return ""
	}
	for _, record := range ledger.Records {
		if record.ID != ps.PendingStrategy {
			continue
		}
		if record.Snapshot.Version == 3 {
			return c.resumeInitialExecutionDecision(packet, record)
		}
		if record.Status == "feedback-running" || record.Status == "feedback-unknown" || record.Status == "feedback-stale" {
			c.fatal = fmt.Errorf("feedback action outcome requires reconciliation; refusing replay")
			return ""
		}
		if record.Status == "replan-running" {
			return "spin" // only runtimePlan may settle this already-dispatched action
		}
		if record.Status == "applied" && record.Action == "replan" {
			active, status := c.runtimeJEVConfig(ledger.Config)
			ledger.Status = status
			if c.err() != nil || active == nil || c.budgetGate() == "stop" || !c.runtimeRecordCurrent(packet, record) {
				return c.recoveredFailureBaseline(packet, record)
			}
			return "spin"
		}
		if record.Status != "pending" {
			c.fatal = fmt.Errorf("pending strategy has inconsistent status")
			return ""
		}
		receipt, _, err := decision.Recover(filepath.Join(c.TaskDir, ".investigation", "runtime-decisions"), *record.Snapshot, ledger.Config.DecisionPolicy(), c.decisionOptions(ledger.Config))
		if err != nil {
			c.fatal = err
			return ""
		}
		record.Receipt = &receipt
		c.accountRuntimeDecision(record)
		// Optional request recovery never sends again or applies an unconsumed
		// model proposal. Resume the frozen deterministic baseline instead.
		record.Selection = &decision.Selection{ActionID: record.Snapshot.BaselineID, Reason: "recovered-without-adoption"}
		record.Action = record.Snapshot.BaselineID
		if decision.CleanupUnknown(receipt) {
			ledger.Status = "native-cleanup-unconfirmed"
			c.save()
			c.fatal = fmt.Errorf("subscription decision cleanup requires reconciliation; refusing another action")
			return ""
		}
		active, status := c.runtimeJEVConfig(ledger.Config)
		ledger.Status = status
		if c.err() != nil || active == nil || c.budgetGate() == "stop" || !c.runtimeRecordCurrent(packet, record) {
			return c.recoveredFailureBaseline(packet, record)
		}
		return c.applyRuntimeDecision(packet, record)
	}
	c.fatal = fmt.Errorf("pending strategy record is missing")
	return ""
}

func (c *Context) finishRuntimeReplan(packet *contract.Packet) {
	ps := c.State.Packets[packet.PacketID]
	if ps.PendingStrategy == "" || c.State.RuntimeDecisions == nil {
		return
	}
	for _, record := range c.State.RuntimeDecisions.Records {
		if record.ID == ps.PendingStrategy {
			record.Status = "completed"
		}
	}
	ps.PendingStrategy = ""
}

// A separately persisted dispatch phase prevents a crash after a successful
// Plan from requesting a second revision. Recovery needs the exact new Plan
// receipt and its verified checkpoint, never just a "consumed" session flag.
func (c *Context) runtimePlan(packet *contract.Packet, milestone contract.Milestone, feedback string) (planOutcome, error) {
	ps := c.State.Packets[packet.PacketID]
	if ps.PendingStrategy == "" || c.State.RuntimeDecisions == nil {
		return c.planUntilValid(milestone, feedback)
	}
	for _, record := range c.State.RuntimeDecisions.Records {
		if record.ID != ps.PendingStrategy {
			continue
		}
		reason := "JEV failure strategy " + record.ID
		if record.Status == "applied" && record.Action == "replan" {
			active, status := c.runtimeJEVConfig(c.State.RuntimeDecisions.Config)
			c.State.RuntimeDecisions.Status = status
			if c.err() != nil || active == nil || c.budgetGate() == "stop" || !c.runtimeRecordCurrent(packet, record) {
				record.Status, ps.PendingStrategy = "stale", ""
				c.save()
				return planOutcome{}, fmt.Errorf("replan authorization changed before dispatch")
			}
			if previous := c.sessionRecord("plan", milestone.ID); previous != nil {
				record.PlanInvocationBefore = previous.Last.ID
			}
			record.Status = "replan-running"
			c.save()
			if err := c.err(); err != nil {
				return planOutcome{}, err
			}
			return c.planUntilValidForReason(milestone, reason, feedback)
		}
		progress := c.State.Milestones[milestone.ID].Plan
		session := c.sessionRecord("plan", milestone.ID)
		if record.Status != "replan-running" || record.Action != "replan" || progress == nil || progress.Status != "passed" || progress.Reason != reason || progress.Checkpoint == nil || session == nil || session.Last.ID == record.PlanInvocationBefore || session.Last.Status != "consumed" {
			return planOutcome{}, fmt.Errorf("replan outcome requires reconciliation; refusing replay")
		}
		receipt, result, _, err := c.readSessionResult(sessionKey("plan", milestone.ID), session)
		if err != nil {
			return planOutcome{}, fmt.Errorf("replan receipt cannot confirm completion: %w", err)
		}
		guard, err := c.sessionGuard("plan", milestone.ID)
		checkpoint := c.confirmedPlanCheckpoint(milestone.ID, &result)
		if err != nil || guard != receipt.GuardAfter || checkpoint == nil || checkpoint.Inputs != progress.Checkpoint.Inputs || checkpoint.Receipt != progress.Checkpoint.Receipt {
			return planOutcome{}, fmt.Errorf("replan checkpoint changed; refusing replay")
		}
		return planOutcome{OK: true, Packets: c.PacketList, Result: &result}, nil
	}
	return planOutcome{}, fmt.Errorf("replan decision record is missing")
}
