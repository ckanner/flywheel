package machine

import (
	"strconv"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
)

// Display additions must not change the existing v2 repair action hash. In
// particular, a legacy pending decision is recovered from its original facts.
func executionPolicyDecisionFacts(policy *state.ExecutionPolicyRecord) *state.ExecutionPolicyRecord {
	if policy == nil {
		return nil
	}
	facts := *policy
	facts.CurrentBinding, facts.CurrentBindingOmitted = nil, false
	return &facts
}

// Persist display metadata alongside the original decision. Historical action
// IDs must never be reinterpreted through a newer configuration or allowlist.
// The evaluator and recovery still use the unchanged Snapshot/hash contract.
func (c *Context) runtimeModelBindings(packet *contract.Packet, actions []decision.Action) map[string]state.BindingSnapshot {
	result := map[string]state.BindingSnapshot{}
	if c.Config.ExecutionPolicy == nil {
		return result
	}
	for index, id := range c.Config.ExecutionPolicy.Routes[packet.Route].Allowed {
		actionID := "model-" + strconv.Itoa(index)
		for _, action := range actions {
			if action.ID == actionID && action.Kind == decision.Model {
				result[actionID] = state.BindingSnapshot{BindingID: id, Binding: c.executionBindingSnapshot(c.Config.ExecutionPolicy.Bindings[id])}
			}
		}
	}
	return result
}
