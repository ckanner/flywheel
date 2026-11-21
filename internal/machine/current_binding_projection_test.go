package machine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestCurrentExecutionBindingFreezesSelectionsAndDoesNotBackfillLegacyState(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	addSwarmChoice(c)
	p := c.Packets["M01-T01"]
	collaborationFixture(p)
	policy, route, err := c.executionPolicy(p)
	require.NoError(t, err)
	require.Equal(t, "low", policy.CurrentBinding.BindingID)
	require.Equal(t, "low", policy.CurrentBinding.Binding.Thinking)
	require.True(t, c.switchExecutionPolicy(p, policy, route, "swarm", "quality-upgrade", "reviewed parallel work"))
	require.Equal(t, "swarm", policy.CurrentBinding.BindingID)
	require.JSONEq(t, `{"version":1,"mode":"swarm","maxObservedAgents":6}`, string(policy.CurrentBinding.Binding.NativeOptions))
	frozen := jsonClone(policy.CurrentBinding)
	c.save()
	require.NoError(t, c.err())
	loaded := loadFixtureState(t, fx)
	require.Equal(t, frozen, jsonClone(loaded.Packets[p.PacketID].ExecutionPolicy.CurrentBinding))
	copy(c.Config.NativeProfiles["swarm"].Options, []byte(`{"version":1,"mode":"swarm","maxObservedAgents":5}`))
	require.Equal(t, frozen, policy.CurrentBinding, "the saved native options must not alias mutable configuration bytes")

	// Reusing a candidate/profile name for a new run cannot rename the old
	// selection's native mode. The new configuration still fails execution's
	// original frozen-policy identity check.
	c.Config.NativeProfiles["swarm"] = config.NativeProfile{Harness: "antigravity", Options: json.RawMessage(`{"version":1,"mode":"native"}`)}
	updated := c.Config.ExecutionPolicy.Routes[p.Route]
	updated.Initial = "swarm"
	c.Config.ExecutionPolicy.Routes[p.Route] = updated
	finish, err := c.beginRunRecord(t.TempDir(), "workflow")
	require.NoError(t, err)
	var outcome error
	finish(&outcome)
	require.NoError(t, outcome)
	require.JSONEq(t, `{"version":1,"mode":"native"}`, string(c.State.Run.InitialBindings["routes.default"].Binding.NativeOptions))
	require.Equal(t, frozen, c.State.Packets[p.PacketID].ExecutionPolicy.CurrentBinding)
	_, _, err = c.executionPolicy(p)
	require.ErrorContains(t, err, "identity changed")

	c.Config.ExecutionPolicy.Routes[p.Route] = route
	c.Config.NativeProfiles["swarm"] = config.NativeProfile{Harness: "antigravity", Options: append(json.RawMessage(nil), frozen.Binding.NativeOptions...)}
	policy.CurrentBinding = nil // an existing pre-projection state record
	_, _, err = c.executionPolicy(p)
	require.NoError(t, err)
	require.Nil(t, policy.CurrentBinding, "reading a legacy policy must not invent historical display evidence")
	require.True(t, c.switchExecutionPolicy(p, policy, route, "low", "quality-upgrade", "explicit next selection"))
	require.Equal(t, "low", policy.CurrentBinding.BindingID, "a new actual selection may freeze new evidence")
	require.Empty(t, policy.CurrentBinding.Binding.NativeOptions)
}

func TestCurrentExecutionBindingDoesNotAlterLegacyDecisionFacts(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	p := c.Packets["M01-T01"]
	policy, _, err := c.executionPolicy(p)
	require.NoError(t, err)
	withBinding, baseline := c.failureActions(p, errL1Failed, false)
	frozen := policy.CurrentBinding
	policy.CurrentBinding = nil
	legacy, legacyBaseline := c.failureActions(p, errL1Failed, false)
	require.Equal(t, baseline, legacyBaseline)
	require.Equal(t, legacy, withBinding, "v2 action IDs and binding hashes ignore new display-only metadata")
	policy.CurrentBinding, policy.CurrentBindingOmitted = frozen, true
	omitted, _ := c.failureActions(p, errL1Failed, false)
	require.Equal(t, legacy, omitted)
	require.NotNil(t, policy.CurrentBinding, "computing decision facts must not edit the stored projection")
	require.True(t, policy.CurrentBindingOmitted)
	require.Nil(t, executionPolicyDecisionFacts(nil))
}

func TestCurrentBindingProjectionSharesBudgetWithNodeAndPreservesFullEvidence(t *testing.T) {
	_, c := nativeBindingProjectionFixture(t)
	p := c.Packets["M01-T01"]
	policy, _, err := c.executionPolicy(p)
	require.NoError(t, err)
	policy.CurrentBinding.Binding.Model = strings.Repeat("large-model-name", 2600)
	c.State.Packets[p.PacketID].Evidence.Executor = map[string]any{"source": "stored-result", "full-report": "keep node details"}
	node, err := ReadNode(c.State, p.PacketID)
	require.NoError(t, err)
	before, err := json.Marshal(c.State)
	require.NoError(t, err)
	view, err := ConsoleProjection(c.State, nil, &NodeDetail{}, &NodeDetail{Packet: &state.PacketState{}}, node)
	require.NoError(t, err)
	shown := view.Packets[p.PacketID].ExecutionPolicy
	require.NotNil(t, shown.CurrentBinding)
	require.False(t, shown.CurrentBindingOmitted)
	require.True(t, shown.CurrentBinding.NativeOptionsOmitted)
	require.Len(t, shown.CurrentBinding.NativeOptionsHash, 64)
	require.Equal(t, policy.CurrentBinding.Binding.Model, shown.CurrentBinding.Binding.Model, "whole identities are never shortened")
	require.Nil(t, node.Packet.ExecutionPolicy.CurrentBinding, "duplicating the same large identity cannot escape the shared reply budget")
	require.True(t, node.Packet.ExecutionPolicy.CurrentBindingOmitted)
	require.Equal(t, c.State.Packets[p.PacketID].Evidence.Executor, node.Packet.Evidence.Executor)
	after, err := json.Marshal(c.State)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NotSame(t, c.State.Packets[p.PacketID], node.Packet)

	// An identity too large to fit is omitted entirely rather than transformed
	// into a different model, even when no other metadata consumes the budget.
	policy.CurrentBinding.Binding.Model = strings.Repeat("x", displayBindingBudget)
	view, err = ConsoleProjection(c.State)
	require.NoError(t, err)
	require.Nil(t, view.Packets[p.PacketID].ExecutionPolicy.CurrentBinding)
	require.True(t, view.Packets[p.PacketID].ExecutionPolicy.CurrentBindingOmitted)
	require.NotNil(t, policy.CurrentBinding)
}
