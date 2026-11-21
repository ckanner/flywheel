package machine

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestPacketEligibilityUsesExecutionRulesAndDoesNotFreezeOrRewriteState(t *testing.T) {
	fx, c := collaborationProjectionFixture(t, true)
	base, err := ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
	require.NoError(t, err)
	require.Equal(t, "verified", base.Eligibility.Status, base.Eligibility.Reason)
	require.Equal(t, "configuration", base.Eligibility.Source)
	require.Len(t, base.Eligibility.Candidates, 1)
	require.True(t, base.Eligibility.Candidates[0].Eligible)
	originalChannels := jsonClone(c.Config.Channels)
	enableExecutionPolicy(c)
	originalChannels["console-account"] = config.Channel{Harness: "dsh", Provider: "deepseek-account", Fallback: "console-backup"}
	originalChannels["console-backup"] = config.Channel{Harness: "dsh", Provider: "deepseek-official", Models: map[string]string{"default": "flash"}}
	c.Config.Channels = originalChannels
	for id, binding := range c.Config.ExecutionPolicy.Bindings {
		binding.Channel = "console-account"
		if id == "backup" {
			binding.Channel = "console-backup"
		}
		c.Config.ExecutionPolicy.Bindings[id] = binding
	}
	addSwarmChoice(c)
	var configured map[string]any
	require.NoError(t, json.Unmarshal(mustReadFile(t, c.Config.File), &configured))
	configured["execution-policy"], configured["native-profiles"] = c.Config.ExecutionPolicy, c.Config.NativeProfiles
	channels, _ := configured["channels"].(map[string]any)
	if channels == nil {
		channels = map[string]any{}
	}
	for _, name := range []string{"console-account", "console-backup", "google-native"} {
		channels[name] = c.Config.Channels[name]
	}
	configured["channels"] = channels
	raw, err := json.Marshal(configured)
	require.NoError(t, err)
	writeFile(t, c.Config.File, string(raw))
	// Changed verification policy must be reflected in the same frozen contract
	// fingerprint the execution boundary will consume.
	packetRaw := mustReadFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"))
	specRaw := mustReadFile(t, filepath.Join(fx.taskDir, "spec.md"))
	c.State.ContractFingerprints["M01-T01"] = c.packetContractFingerprint(c.Packets["M01-T01"], specRaw, packetRaw)
	before := jsonClone(c.State)
	node, err := ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
	require.NoError(t, err)
	require.Equal(t, "verified", node.Eligibility.Status, node.Eligibility.Reason)
	require.Len(t, node.Eligibility.Candidates, 4)
	require.Equal(t, before, c.State, "eligibility must not freeze an execution policy")
	for _, candidate := range node.Eligibility.Candidates {
		require.Equal(t, c.executionCandidateError(c.Packets["M01-T01"], candidate.Binding) == nil, candidate.Eligible)
	}
	_, _, err = c.executionPolicy(c.Packets["M01-T01"])
	require.NoError(t, err)
	node, err = ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
	require.NoError(t, err)
	require.Equal(t, "frozen", node.Eligibility.Source)
	c.State.Workflow = &state.WorkflowState{WorkflowSelection: contract.WorkflowSelection{Effective: "quick"}}
	c.State.ContractFingerprints["M01-T01"] = c.packetContractFingerprint(c.Packets["M01-T01"], specRaw, packetRaw)
	node, err = ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
	require.NoError(t, err)
	require.Equal(t, "verified", node.Eligibility.Status, node.Eligibility.Reason)
	require.Len(t, node.Eligibility.Candidates, 4)
	require.False(t, node.Eligibility.Candidates[3].Eligible)
	c.State.Packets["M01-T01"].ExecutionPolicy.Identity = "changed"
	node, err = ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
	require.NoError(t, err)
	require.Equal(t, "unknown", node.Eligibility.Status)
	require.Empty(t, node.Eligibility.Candidates)
}

func TestPacketEditorExplainsUnavailableApprovalDraftAndScope(t *testing.T) {
	fx, c := collaborationProjectionFixture(t, true)
	packet := c.Packets["M01-T01"]
	for _, scenario := range []string{"no-planning", "no-plan", "missing-packet", "missing-ref", "pending-review", "handoff", "no-writes", "completed"} {
		t.Run(scenario, func(t *testing.T) {
			copy := *c
			copy.State = jsonClone(c.State)
			copy.Spec = jsonClone(c.Spec)
			p := jsonClone(packet)
			switch scenario {
			case "no-planning":
				copy.State.Planning = nil
			case "no-plan":
				copy.State.Planning.CurrentPlans = nil
			case "missing-packet":
				for _, ref := range copy.State.Planning.CurrentPlans {
					delete(planning.FindPlan(copy.State.Planning, ref).Files, "task-packet-M01-T01.md")
				}
			case "missing-ref":
				copy.Spec.PlanningRef = nil
			case "pending-review":
				copy.State.Planning.Phase = "reviewing"
			case "handoff":
				copy.State.Handoff = &state.Handoff{Key: "execute/M01-T01"}
			case "no-writes":
				p.Schedule = nil
			case "completed":
				copy.State.Packets[p.PacketID].Status = "completed"
			}
			v := copy.collaborationEditor(p, c.State.ContractFingerprints[p.PacketID])
			require.False(t, v.Editable, v.Reason)
			require.NotEmpty(t, v.Reason)
		})
	}
	legacy := jsonClone(c.State)
	legacy.Planning = nil
	node, err := ReadNode(legacy, packet.PacketID, fx.taskDir, fx.root)
	require.NoError(t, err)
	require.Equal(t, "unknown", node.Eligibility.Status, "removing planning cannot bypass the frozen approval identity")
	require.False(t, node.CollaborationEditor.Editable)
	copy := *c
	copy.State = jsonClone(c.State)
	copy.Config = &config.Config{}
	_, err = ReadNode(copy.State, "M99-T99", fx.taskDir, fx.root)
	require.Error(t, err)
}
