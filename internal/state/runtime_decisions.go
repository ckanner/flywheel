package state

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
)

// RuntimeDecisions belongs to the task, not a worker or an individual run.
// Reserved/unknown requests and imported investigation calls share its cap.
type RuntimeDecisions struct {
	Subscription  *decision.SubscriptionBinding `json:"subscription-binding,omitempty"`
	Version       int                           `json:"version"`
	Config        *config.JEVConfig             `json:"config,omitempty"`
	Used          int                           `json:"used"`
	ImportedCalls int                           `json:"imported-calls"`
	Status        string                        `json:"status"`
	Records       []*RuntimeDecisionRecord      `json:"records"`
	MaxCalls      int                           `json:"max-calls,omitempty"` // display projection
	Omitted       int                           `json:"omitted-records,omitempty"`
}

type RuntimeDecisionRecord struct {
	ID                   string              `json:"id"`
	Packet               string              `json:"packet"`
	Attempt              int                 `json:"attempt"`
	PlanRevisions        int                 `json:"plan-revisions"`
	StartedAt            string              `json:"started-at"`
	Snapshot             *decision.Snapshot  `json:"snapshot,omitempty"`
	Receipt              *decision.Receipt   `json:"receipt,omitempty"`
	Selection            *decision.Selection `json:"selection,omitempty"`
	Action               string              `json:"action"`
	Status               string              `json:"status"`
	Accounted            bool                `json:"accounted"`
	Feedback             any                 `json:"feedback,omitempty"`
	PlanInvocationBefore string              `json:"plan-invocation-before,omitempty"`
}

var runtimeStateSchema = contract.StateSchema

func validateRuntimeDecisions(st *State) error {
	ledger := st.RuntimeDecisions
	if ledger == nil {
		return validatePendingStrategies(st, nil)
	}
	schema, err := runtimeStateSchema()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(ledger)
	if err != nil {
		return err
	}
	var value any
	_ = json.Unmarshal(raw, &value)
	if err := schema.Properties["runtime-decisions"].Validate(value); err != nil {
		return fmt.Errorf("runtime decisions: %w", err)
	}
	if ledger.Config.Backend == "subscription" {
		if err := decision.ValidateSubscriptionBinding(ledger.Subscription); err != nil {
			return err
		}
		if ledger.Subscription.Profile != ledger.Config.Profile {
			return fmt.Errorf("runtime decision profile contradicts its frozen binding")
		}
	}
	seen, used := map[string]*RuntimeDecisionRecord{}, ledger.ImportedCalls
	for _, record := range ledger.Records {
		if seen[record.ID] != nil || st.Packets[record.Packet] == nil || record.Snapshot == nil || record.Snapshot.ID != record.ID || record.Snapshot.Identity.TaskID != st.Slug || record.Snapshot.Version != 2 || record.Snapshot.Identity.Revision > st.Revision {
			return fmt.Errorf("runtime decision identity mismatch")
		}
		seen[record.ID] = record
		if _, err := time.Parse(time.RFC3339Nano, record.StartedAt); err != nil {
			return fmt.Errorf("runtime decision dispatch time is invalid")
		}
		if err := decision.Validate(*record.Snapshot, ledger.Config.DecisionPolicy()); err != nil {
			return err
		}
		if err := validateRuntimeDecisionRecord(st, record, ledger.Config); err != nil {
			return err
		}
		if record.Receipt == nil || record.Receipt.Status != "skipped" {
			used++
		}
	}
	if used != ledger.Used {
		return fmt.Errorf("runtime decision call count mismatch")
	}
	return validatePendingStrategies(st, seen)
}

func validatePendingStrategies(st *State, records map[string]*RuntimeDecisionRecord) error {
	for id, packet := range st.Packets {
		if packet == nil || packet.PendingStrategy == "" {
			continue
		}
		if record := records[packet.PendingStrategy]; record == nil || record.Packet != id {
			return fmt.Errorf("pending strategy has no decision record for its packet")
		}
	}
	return nil
}

func validateRuntimeDecisionRecord(st *State, r *RuntimeDecisionRecord, j *config.JEVConfig) error {
	snapshot := r.Snapshot
	if !snapshot.SemanticUncertainty || snapshot.ExecutorUnknown || len(snapshot.Actions) < 2 {
		return fmt.Errorf("runtime decision did not have eligible known evidence")
	}
	var selected decision.Action
	for _, action := range snapshot.Actions {
		if !validRuntimeAction(action) {
			return fmt.Errorf("runtime decision action does not match an implemented binding")
		}
		if action.ID == r.Action {
			selected = action
		}
	}
	if selected.ID == "" {
		return fmt.Errorf("runtime decision selects an absent action")
	}
	pending := r.Status == "pending" || r.Status == "feedback-running" || r.Status == "feedback-unknown" || r.Status == "feedback-stale" || r.Status == "replan-running" || (r.Status == "applied" && r.Action == "replan")
	if (st.Packets[r.Packet].PendingStrategy == r.ID) != pending {
		return fmt.Errorf("runtime decision status and pending strategy disagree")
	}
	if r.PlanInvocationBefore != "" && r.Action != "replan" {
		return fmt.Errorf("only replan may retain a planner invocation boundary")
	}
	if r.Status == "pending" {
		if r.Feedback != nil || r.Action != snapshot.BaselineID || (r.Selection != nil && r.Selection.Adopted) {
			return fmt.Errorf("pending runtime decision already has a terminal or effect")
		}
		if r.Receipt == nil && r.Selection == nil && !r.Accounted {
			return nil
		}
		// Recovery may persist the accounted baseline while a budget stop is
		// being saved. It remains pending; no action has been applied yet.
	}
	if r.Receipt == nil || r.Selection == nil || !r.Accounted || r.Selection.ActionID != r.Action || strings.TrimSpace(r.Selection.Reason) == "" {
		return fmt.Errorf("settled runtime decision requires its receipt, selection and accounting")
	}
	options := decision.Options{Endpoint: j.Endpoint, Model: j.Model, ExpectedModel: j.Model, Timeout: time.Duration(j.TimeoutMS) * time.Millisecond,
		AllowedKinds: []decision.ActionKind{decision.Diagnostic, decision.Repair, decision.Replan, decision.Model}}
	if j.Backend == "subscription" {
		options.Backend, options.Subscription = j.Backend, st.RuntimeDecisions.Subscription
		if options.Subscription != nil {
			options.Model, options.ExpectedModel = options.Subscription.Model, options.Subscription.Model
		}
	}
	if err := decision.ValidateReceiptIdentity(*snapshot, j.DecisionPolicy(), options, *r.Receipt); err != nil {
		return err
	}
	if r.Selection.Adopted {
		if expected := decision.Select(*snapshot, *r.Receipt, j.DecisionPolicy(), options); !expected.Adopted || expected != *r.Selection {
			return fmt.Errorf("runtime selection is not authorized by its frozen receipt")
		}
	} else if r.Action != snapshot.BaselineID || r.Selection.InvocationID != "" {
		return fmt.Errorf("non-adopted runtime selection must retain the deterministic baseline")
	}
	feedback := strings.HasPrefix(r.Status, "feedback-")
	if feedback {
		if selected.Kind != decision.Diagnostic || !r.Selection.Adopted || (r.Feedback != nil) != (r.Status != "feedback-running") {
			return fmt.Errorf("runtime feedback state lacks its approved action or result")
		}
	} else if r.Feedback != nil || ((r.Status == "completed" || r.Status == "replan-running") && r.Action != "replan") || (r.Status == "applied" && (selected.Kind == decision.Diagnostic || (r.Action != "replan" && !r.Selection.Adopted))) {
		return fmt.Errorf("runtime decision effect does not match its terminal state")
	}
	return nil
}

func validRuntimeAction(action decision.Action) bool {
	if action.ID == "repair" {
		return action.Kind == decision.Repair
	}
	if action.ID == "replan" {
		return action.Kind == decision.Replan
	}
	for prefix, kind := range map[string]decision.ActionKind{"model-": decision.Model, "feedback-": decision.Diagnostic, "recheck-": decision.Diagnostic} {
		if suffix, found := strings.CutPrefix(action.ID, prefix); found {
			index, err := strconv.Atoi(suffix)
			return err == nil && index >= 0 && strconv.Itoa(index) == suffix && action.Kind == kind
		}
	}
	return false
}
