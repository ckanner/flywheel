package state

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func workflowFixture(t *testing.T) (*contract.Spec, []*contract.Packet, *State) {
	t.Helper()
	spec := &contract.Spec{Milestones: []contract.Milestone{{ID: "M-01"}},
		Requirements: []contract.Requirement{{ID: "R-01", Milestone: "M-01", Verification: "unit regression"}},
		Workflow: &contract.Workflow{Version: 1, Mode: "auto", Reason: "Known local repair",
			ScopeClear: true, ImplementationKnown: true, VerificationComplete: true, ReviewSufficient: true,
			Evidence: []string{"Existing unit regression"}, Risks: []string{},
			NotApplicable: map[string]string{"L2": "No integration boundary", "L3": "No user interface"}}}
	packets := []*contract.Packet{{PacketID: "M01-T01", Route: "default", Requirements: []string{"R-01"},
		Acceptance: contract.Acceptance{L1: []contract.GateEntry{{Cmd: "test", Expect: "exit 0"}}}}}
	st, err := Init("task", spec, packets, nil)
	require.NoError(t, err)
	return spec, packets, st
}

func attachQuickWorkflow(t *testing.T, st *State, spec *contract.Spec, packets []*contract.Packet) {
	t.Helper()
	selection, err := ResolveWorkflow(nil, spec, packets, "auto")
	require.NoError(t, err)
	require.Equal(t, "quick", selection.Effective)
	st.Workflow = &WorkflowState{WorkflowSelection: selection, SelectedAt: NowISO(), Upgrades: []WorkflowUpgrade{}}
}

func TestResolveWorkflowRespectsDurablePolicyAndDoesNotMutate(t *testing.T) {
	spec, packets, st := workflowFixture(t)
	selection, err := ResolveWorkflow(nil, spec, packets, "")
	require.NoError(t, err)
	assert.Equal(t, "quick", selection.Effective)
	selection, err = ResolveWorkflow(st, spec, packets, "auto")
	require.NoError(t, err)
	assert.Equal(t, "full", selection.Effective, "legacy state remains Full despite a new Quick assessment")
	assert.Nil(t, st.Workflow)
	assert.Equal(t, "auto", spec.Workflow.Mode, "legacy fallback must not edit the live Spec")
	_, err = ResolveWorkflow(st, spec, packets, "quick")
	require.ErrorContains(t, err, "cannot be downgraded")
	_, err = ResolveWorkflow(nil, spec, packets, "fast")
	require.ErrorContains(t, err, "unsupported workflow mode")
	attachQuickWorkflow(t, st, spec, packets)
	before, err := json.Marshal(st)
	require.NoError(t, err)
	for _, mode := range []string{"", "auto", "quick"} {
		selection, err = ResolveWorkflow(st, spec, packets, mode)
		require.NoError(t, err)
		assert.Equal(t, "quick", selection.Effective)
	}
	selection, err = ResolveWorkflow(st, spec, packets, "full")
	require.NoError(t, err)
	assert.Equal(t, "full", selection.Effective)
	assert.True(t, selection.Stages["plan"].Applicable)
	assert.True(t, selection.Stages["judge"].Applicable)
	after, err := json.Marshal(st)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a resolver returns a proposal; only the engine persists an upgrade")
	st.Workflow.WorkflowSelection = selection
	_, err = ResolveWorkflow(st, spec, packets, "quick")
	require.ErrorContains(t, err, "cannot be downgraded")
	selection, err = ResolveWorkflow(st, spec, packets, "auto")
	require.NoError(t, err)
	assert.Equal(t, "full", selection.Effective)
	st.Workflow.Version = 99
	_, err = ResolveWorkflow(st, spec, packets, "auto")
	require.Error(t, err)
}

func TestWorkflowStateRoundTripAndStrictExtensionValidation(t *testing.T) {
	spec, packets, st := workflowFixture(t)
	attachQuickWorkflow(t, st, spec, packets)
	dir := t.TempDir()
	st.Budget.Totals.Tokens = 89
	st.Packets["M01-T01"].Attempts = 2
	st, err := Save(dir, st, nil)
	require.NoError(t, err)
	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, st.Workflow, loaded.Workflow)
	assert.Equal(t, int64(89), loaded.Budget.Totals.Tokens)
	assert.Equal(t, 2, loaded.Packets["M01-T01"].Attempts)
	valid, err := os.ReadFile(StateFile(dir))
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"unknown version", func(w map[string]any) { w["version"] = 99 }},
		{"unknown field", func(w map[string]any) { w["unsafe"] = true }},
		{"unknown effective", func(w map[string]any) { w["effective"] = "fast" }},
		{"contradictory request", func(w map[string]any) { w["requested"] = "full" }},
		{"pretend Plan applicable", func(w map[string]any) { w["stages"].(map[string]any)["plan"].(map[string]any)["applicable"] = true }},
		{"missing reason", func(w map[string]any) { w["reason"] = " " }},
		{"null evidence", func(w map[string]any) { w["evidence"] = nil }},
		{"risk still Quick", func(w map[string]any) { w["risks"] = []string{"payment"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value map[string]any
			require.NoError(t, json.Unmarshal(valid, &value))
			tc.change(value["workflow"].(map[string]any))
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			writeRaw(t, StateFile(dir), string(raw))
			_, err = Load(dir)
			require.Error(t, err, "unknown or inconsistent policy must fail before execution")
			after, err := os.ReadFile(StateFile(dir))
			require.NoError(t, err)
			assert.Equal(t, raw, after)
		})
	}
	writeRaw(t, StateFile(dir), `{"revision":1,"workflow":null}`)
	_, err = Load(dir)
	require.Error(t, err, "explicit null is not a legacy absent policy")
	writeRaw(t, StateFile(dir), string(valid))
	st.Workflow.Effective = "full"
	st.Workflow.Stages = contract.WorkflowStages("full")
	st.Workflow.Upgrades = []WorkflowUpgrade{{At: NowISO(), From: "quick", To: "full", Reason: "Risk found",
		Evidence: []string{"review found a transaction boundary"}, ContractFingerprints: map[string]string{"M01-T01": "verified-contract"}}}
	st, err = Save(dir, st, nil)
	require.NoError(t, err)
	loaded, err = Load(dir)
	require.NoError(t, err)
	assert.Equal(t, st.Workflow, loaded.Workflow)
	assert.Equal(t, int64(89), loaded.Budget.Totals.Tokens)
	assert.Equal(t, 2, loaded.Packets["M01-T01"].Attempts)
}
