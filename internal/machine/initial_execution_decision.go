package machine

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
)

// A first-turn choice uses the same task owner, call cap and durable journal as
// failure decisions. Parallel workers cannot each reserve an independent cap.
func (c *Context) runtimeInitialExecutionDecision(packet *contract.Packet) error {
	if c.parallel == nil {
		return c.initialExecutionDecision(packet)
	}
	g := c.parallel.group
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := c.parallel.merge(c); err != nil {
		return err
	}
	g.logMu.Lock()
	defer g.logMu.Unlock()
	prior := g.owner.RunContext
	g.owner.RunContext = c.RunContext
	defer func() { g.owner.RunContext = prior }()
	err := g.owner.initialExecutionDecision(packet)
	c.copyRuntimeDecisionState(packet, g.owner)
	if err != nil {
		g.cancel()
	}
	return err
}

func (c *Context) initialExecutionActions(packet *contract.Packet) ([]decision.Action, string, error) {
	policy, route, err := c.executionPolicy(packet)
	if err != nil || policy == nil {
		return nil, "", err
	}
	baseline := "model-" + strconv.Itoa(slices.Index(route.Allowed, policy.Current))
	if err := c.executionCandidateError(packet, c.Config.ExecutionPolicy.Bindings[policy.Current]); err != nil {
		return nil, "", err
	}
	actions := []decision.Action{}
	// Include the baseline first even when a custom allowlist contains >16
	// entries; bounded optional selection must always retain a legal baseline.
	ids := append([]string{policy.Current}, route.Allowed...)
	for i, id := range ids {
		if i > 0 && (id == policy.Current || policy.Switches >= route.MaxSwitches) {
			continue
		}
		b := c.Config.ExecutionPolicy.Bindings[id]
		if c.executionCandidateError(packet, b) != nil {
			continue
		}
		actions = append(actions, decision.Action{ID: "model-" + strconv.Itoa(slices.Index(route.Allowed, id)), Kind: decision.Model,
			Description: c.executionChoiceDescription(id, b), BindingHash: sessionHash([]any{id, b, policy.Identity, policy.Current, policy.Switches})})
		if len(actions) == 16 {
			break
		}
	}
	return actions, baseline, nil
}

func (c *Context) initialExecutionSnapshot(packet *contract.Packet, revision int64, budgetFact string) (decision.Snapshot, error) {
	identity, candidate, err := c.verificationSnapshot(packet)
	if err != nil {
		return decision.Snapshot{}, err
	}
	actions, baseline, err := c.initialExecutionActions(packet)
	if err != nil {
		return decision.Snapshot{}, err
	}
	ps := c.State.Packets[packet.PacketID]
	mode := "full"
	if c.quick() {
		mode = "quick"
	}
	data, _ := json.Marshal(map[string]any{"route": packet.Route, "workflow": mode, "input-images": packet.InputImages != nil,
		"acceptance-checks":  map[string]int{"L1": len(packet.Acceptance.L1), "L2": len(packet.Acceptance.L2), "L3": len(packet.Acceptance.L3)},
		"attempts-remaining": packet.Escalation.MaxRetries - ps.FailedAttempts})
	facts := []decision.Fact{{Category: decision.TaskSummary, Text: boundedDecisionText(packet.Goal, 2048)},
		{Category: decision.DiagnosticFacts, Text: boundedDecisionText(string(data), 2048)},
		{Category: decision.DiagnosticFacts, Text: budgetFact}}
	for _, item := range []struct {
		label string
		value any
	}{{"Reviewed boundaries", packet.Boundaries}, {"Approved schedule", packet.Schedule}, {"Approved acceptance requirements", packet.Acceptance}, {"Requirements", packet.Requirements}} {
		raw, _ := json.Marshal(item.value)
		facts = append(facts, decision.Fact{Category: decision.DiagnosticFacts, Text: boundedDecisionText(item.label+": "+string(raw), 2048)})
	}
	if packet.Collaboration != nil {
		for _, task := range packet.Collaboration.Tasks {
			raw, _ := json.Marshal(task)
			facts = append(facts, decision.Fact{Category: decision.DiagnosticFacts, Text: boundedDecisionText("Approved independent task: "+string(raw), 2048)})
		}
	}
	return decision.Snapshot{Version: 3, ID: "initial-" + packet.PacketID,
		Identity: decision.Identity{TaskID: c.State.Slug, RunID: sessionHash(c.TaskDir), Revision: revision,
			ContractHash:  sessionHash([]any{c.Spec, packet, executionChoiceConfigurationHash(c.Config), c.Config.Verify, c.workflowEvidencePolicy()}),
			CandidateHash: candidate, ObservationHash: sessionHash([]any{identity, ps.Attempts, ps.FailedAttempts, ps.PlanRevisions})},
		Facts: facts, MissingFacts: []string{"No implementation has run for this packet. Account quota, model success rates and complete native child usage are unknown. Bounded summaries may omit contract detail; final acceptance and independent review remain mandatory."},
		Actions: actions, BaselineID: baseline, SemanticUncertainty: true}, nil
}

func (c *Context) initialExecutionDecision(packet *contract.Packet) error {
	if err := c.err(); err != nil {
		return err
	}
	if err := c.runtimeExecutionBarrier(packet); err != nil {
		return err
	}
	ps := c.State.Packets[packet.PacketID]
	if ps.Attempts != 0 || ps.PlanRevisions != 0 || c.sessionRecord("execute", packet.PacketID) != nil {
		return nil
	}
	ledger := c.State.RuntimeDecisions
	var frozen *config.JEVConfig
	if ledger != nil {
		for _, record := range ledger.Records {
			if record.ID == "initial-"+packet.PacketID {
				if ps.PendingStrategy == record.ID {
					c.resumeInitialExecutionDecision(packet, record)
				}
				return c.err()
			}
		}
		frozen = ledger.Config
	}
	if !c.Config.JEV.Enabled && ledger == nil || c.Config.ExecutionPolicy == nil || ps.PendingStrategy != "" {
		return nil
	}
	j, status := c.runtimeJEVConfig(frozen)
	if ledger != nil {
		ledger.Status = status
	}
	if j == nil || !slices.Contains(j.AllowedData, decision.TaskSummary) || !slices.Contains(j.AllowedData, decision.DiagnosticFacts) || j.Backend != "subscription" && c.decisionOptions(j).APIKey == "" || c.budgetGate() == "stop" {
		return nil
	}
	if ledger == nil {
		ledger = c.initializeRuntimeDecisions(j)
	}
	if ledger == nil || ledger.Used >= j.MaxCalls || len(ledger.Records) >= 100 {
		return nil
	}
	day := c.Now().UTC().Format("2006-01-02")
	budget, _ := json.Marshal(map[string]any{"budget-policy": c.Config.Budget, "known-task-tokens": c.State.Budget.Totals.Tokens,
		"day": day, "daily-usage": c.State.Budget.Daily[day], "unknown-usage-calls": c.State.Budget.UnknownUsageCalls,
		"decision-calls-used": ledger.Used, "decision-call-limit": j.MaxCalls, "decision-calls-remaining": j.MaxCalls - ledger.Used})
	budgetFact := boundedDecisionText(string(budget), 2048)
	snapshot, err := c.initialExecutionSnapshot(packet, c.revision+1, budgetFact)
	if err != nil {
		return err
	}
	if len(snapshot.Actions) < 2 || decision.Validate(snapshot, j.DecisionPolicy()) != nil {
		return nil
	}
	record := &state.RuntimeDecisionRecord{ModelBindings: c.runtimeModelBindings(packet, snapshot.Actions), ID: snapshot.ID, Packet: packet.PacketID, StartedAt: c.nowISO(), Snapshot: &snapshot, Action: snapshot.BaselineID, Status: "pending"}
	record.Execution = runtimeExecutionCheckpoint(c.State, packet.PacketID)
	ledger.Records = append(ledger.Records, record)
	ledger.Used++
	ps.PendingStrategy = record.ID
	c.save()
	if err := c.err(); err != nil {
		return err
	}
	options := c.decisionOptions(j)
	receipt, err := decision.Evaluate(c.decisionContext(), filepath.Join(c.TaskDir, ".investigation", "runtime-decisions"), snapshot, j.DecisionPolicy(), options)
	if err != nil {
		receipt.Status, receipt.Reason = "unknown", "decision-journal-or-request-error"
	}
	record.Receipt = &receipt
	// The request's budget fact describes its reservation point. Our own JEV
	// usage is charged afterwards and separately rechecked before adoption.
	current, currentErr := c.initialExecutionSnapshot(packet, snapshot.Identity.Revision, budgetFact)
	c.accountRuntimeDecision(record)
	selection := decision.Selection{ActionID: snapshot.BaselineID, Reason: "deterministic-baseline"}
	active, status := c.runtimeJEVConfig(j)
	ledger.Status = status
	unchanged := currentErr == nil && sessionHash(current) == sessionHash(snapshot)
	if c.err() == nil && unchanged && active != nil && c.budgetGate() != "stop" {
		selection = decision.Select(current, receipt, j.DecisionPolicy(), options)
	}
	record.Selection, record.Action = &selection, selection.ActionID
	if decision.CleanupUnknown(receipt) {
		ledger.Status = "native-cleanup-unconfirmed"
		c.save()
		c.fatal = fmt.Errorf("initial subscription decision cleanup requires reconciliation; refusing execution")
		return c.fatal
	}
	ps.PendingStrategy, record.Status = "", "baseline"
	if !unchanged {
		record.Status = "stale"
		c.save()
		return fmt.Errorf("initial execution inputs changed during selection; reload the prepared task")
	}
	if selection.Adopted {
		c.applyInitialExecutionSelection(packet, record)
	}
	c.save()
	return c.err()
}

// Execute must not bypass an unresolved decision or local feedback process by
// selecting a different packet after restart. Recovery of the original initial
// choice is read-only and remains available through this same entry point.
func (c *Context) runtimeExecutionBarrier(packet *contract.Packet) error {
	if c.State.RuntimeDecisions == nil {
		return nil
	}
	for _, record := range c.State.RuntimeDecisions.Records {
		if record.Packet == packet.PacketID && record.Snapshot.Version == 3 && record.Status == "pending" {
			continue
		}
		switch record.Status {
		case "pending", "feedback-running", "feedback-unknown", "feedback-stale", "replan-running":
			return fmt.Errorf("packet %s has an unresolved execution strategy; reconcile its original decision before executing %s", record.Packet, packet.PacketID)
		case "applied":
			if record.Packet == packet.PacketID && record.Action == "replan" {
				return fmt.Errorf("packet %s requires its pending replan before execution", packet.PacketID)
			}
		}
	}
	return nil
}

func (c *Context) applyInitialExecutionSelection(packet *contract.Packet, record *state.RuntimeDecisionRecord) {
	policy, route, err := c.executionPolicy(packet)
	index, parseErr := strconv.Atoi(strings.TrimPrefix(record.Action, "model-"))
	record.Status = "stale"
	if err != nil || policy == nil || parseErr != nil || index < 0 || index >= len(route.Allowed) {
		return
	}
	if route.Allowed[index] == policy.Current || c.switchExecutionPolicy(packet, policy, route, route.Allowed[index], "jev-initial-selection", record.Selection.Reason+"; decision "+record.ID) {
		record.Status = "applied"
	}
}

// A recovered first-turn request consumes its original reservation but never
// adopts a late model answer. The configured baseline remains selected.
func (c *Context) resumeInitialExecutionDecision(packet *contract.Packet, record *state.RuntimeDecisionRecord) string {
	ledger := c.State.RuntimeDecisions
	if record.Status != "pending" {
		c.fatal = fmt.Errorf("pending initial execution choice has inconsistent status")
		return ""
	}
	receipt, _, err := decision.Recover(filepath.Join(c.TaskDir, ".investigation", "runtime-decisions"), *record.Snapshot, ledger.Config.DecisionPolicy(), c.decisionOptions(ledger.Config))
	if err != nil {
		c.fatal = err
		return ""
	}
	record.Receipt = &receipt
	c.accountRuntimeDecision(record)
	record.Selection = &decision.Selection{ActionID: record.Snapshot.BaselineID, Reason: "recovered-without-adoption"}
	record.Action = record.Snapshot.BaselineID
	if decision.CleanupUnknown(receipt) {
		ledger.Status = "native-cleanup-unconfirmed"
		c.save()
		c.fatal = fmt.Errorf("initial subscription decision cleanup requires reconciliation; refusing execution")
		return ""
	}
	_, ledger.Status = c.runtimeJEVConfig(ledger.Config)
	packetState := c.State.Packets[packet.PacketID]
	packetState.PendingStrategy, record.Status = "", "baseline"
	c.save()
	return ""
}
