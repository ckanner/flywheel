package machine

import (
	"strings"

	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/state"
)

func runtimeExecutionCheckpoint(st *state.State, packet string) *state.RuntimeDecisionExecution {
	if fingerprint := st.ContractFingerprints[packet]; fingerprint != "" {
		return &state.RuntimeDecisionExecution{ContractFingerprint: fingerprint}
	}
	return nil
}

// Correlate before saving the invocation intent, using the frozen selection,
// exact contract and next execution attempt. Similar names or later model
// settings are never enough to attach a call to historical advice.
func (c *Context) executionDecisionForInvocation(session *state.SessionRecord) string {
	ledger := c.State.RuntimeDecisions
	if ledger == nil || session.Role != "execute" || session.Configuration == nil {
		return ""
	}
	packet := c.State.Packets[session.Scope]
	if packet == nil || packet.ExecutionPolicy == nil {
		return ""
	}
	actual := *session.Configuration
	actual.InputImages, actual.Subscription, actual.EvidenceOnly = nil, nil, false
	actual.Provider = fallback.ResolveProvider(c.Config, actual.Harness, actual.Provider, actual.Channel)
	for i := len(ledger.Records) - 1; i >= 0; i-- {
		r := ledger.Records[i]
		if r.Packet != session.Scope || r.Attempt+1 != session.Last.Attempt || r.PlanRevisions != packet.PlanRevisions ||
			r.Execution == nil || r.Execution.ContractFingerprint != c.State.ContractFingerprints[r.Packet] ||
			(r.Status != "applied" && r.Status != "baseline") {
			continue
		}
		choice, ok := r.ModelBindings[r.Action]
		if ok && packet.ExecutionPolicy.Current == choice.BindingID && strings.HasPrefix(r.Action, "model-") && sessionHash(choice.Binding) == sessionHash(actual) {
			return r.ID
		}
	}
	return ""
}

func runtimeInvocationSummary(call *state.SessionInvocation) *state.RuntimeDecisionInvocation {
	return &state.RuntimeDecisionInvocation{ID: call.ID, EngineRunID: call.EngineRunID, StartedAt: call.StartedAt, Attempt: call.Attempt,
		Status: call.Status, Dispatch: call.Dispatch, DispatchObserved: call.DispatchObserved, Outcome: call.Outcome, CleanupConfirmed: call.CleanupConfirmed}
}

// This runs in the owner immediately before the ordinary CAS save, including
// after parallel lanes merge their exact session. Keeping the bounded latest
// linked call retains terminal evidence after the session slot is replaced.
func syncRuntimeDecisionExecutions(st *state.State) {
	if st.RuntimeDecisions == nil || st.Sessions == nil {
		return
	}
	for _, r := range st.RuntimeDecisions.Records {
		if r == nil || r.Execution == nil {
			continue
		}
		s := st.Sessions.Records[sessionKey("execute", r.Packet)]
		if s != nil && s.Last != nil && s.Last.DecisionID == r.ID {
			r.Execution.Invocation = runtimeInvocationSummary(s.Last)
		}
	}
}

func runtimeDecisionExecutionView(st *state.State, r *state.RuntimeDecisionRecord) *state.RuntimeDecisionExecutionView {
	if !strings.HasPrefix(r.Action, "model-") {
		return nil
	}
	v := &state.RuntimeDecisionExecutionView{Status: "unknown", Reason: "execution-link-unrecorded"}
	if r.Status != "baseline" && r.Status != "applied" {
		v.Status, v.Reason = "not-selected", "selection-not-applied"
		return v
	}
	if r.Execution == nil {
		return v
	}
	call := r.Execution.Invocation
	if st.Sessions != nil {
		if s := st.Sessions.Records[sessionKey("execute", r.Packet)]; s != nil && s.Last != nil && s.Last.DecisionID == r.ID {
			call = runtimeInvocationSummary(s.Last)
		}
	}
	if call == nil {
		packet := st.Packets[r.Packet]
		choice, found := r.ModelBindings[r.Action]
		if packet != nil && packet.Attempts == r.Attempt && packet.PlanRevisions == r.PlanRevisions &&
			st.ContractFingerprints[r.Packet] == r.Execution.ContractFingerprint && found && packet.ExecutionPolicy != nil &&
			packet.ExecutionPolicy.Current == choice.BindingID &&
			(packet.ExecutionPolicy.CurrentBinding == nil || sessionHash(packet.ExecutionPolicy.CurrentBinding.Binding) == sessionHash(choice.Binding)) {
			v.Status, v.Reason = "selected", "awaiting-execution"
		}
		return v
	}
	v.Invocation = jsonClone(call)
	switch call.Dispatch {
	case "not-dispatched":
		v.Status, v.Reason = "not-started", "confirmed-not-dispatched"
		if call.DispatchObserved {
			v.Status, v.Reason = "unknown", "conflicting-dispatch-evidence"
		}
	case "dispatched":
		v.Status, v.Reason = "started", "execution-outcome-unconfirmed"
		if call.CleanupConfirmed && (call.Status == "received" || call.Status == "consumed") &&
			(call.Outcome == "completed" || call.Outcome == "failed" || call.Outcome == "cancelled") {
			v.Status, v.Reason = call.Outcome, "confirmed-invocation-result"
		}
	default:
		v.Reason = "execution-dispatch-unconfirmed"
		if call.DispatchObserved {
			v.Status, v.Reason = "started", "accepted-lifecycle-awaiting-receipt"
		}
	}
	return v
}
