package contract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowStateSchemaEnforcesStageApplicability(t *testing.T) {
	spec, packets := qualifiedWorkflowContract()
	selection, err := SelectWorkflow(spec, packets, "auto")
	require.NoError(t, err)
	raw, err := json.Marshal(selection)
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(raw, &value))
	value["selected-at"] = "2026-11-06T17:00:00Z"
	value["upgrades"] = []any{}
	require.NoError(t, ValidateWorkflowStateValue(value))
	value["stages"].(map[string]any)["judge"].(map[string]any)["applicable"] = true
	require.Error(t, ValidateWorkflowStateValue(value), "Quick cannot claim an applicable Judge stage")
	value["effective"] = "full"
	value["stages"].(map[string]any)["plan"].(map[string]any)["applicable"] = true
	require.NoError(t, ValidateWorkflowStateValue(value))
}

func qualifiedWorkflowContract() (*Spec, []*Packet) {
	spec := &Spec{
		Milestones:   []Milestone{{ID: "M-01"}},
		Requirements: []Requirement{{ID: "R-01", Milestone: "M-01", Verification: "L1 regression verifies the correction"}},
		Workflow: &Workflow{Version: 1, Mode: "auto", Reason: "Located bug with an existing implementation pattern",
			ScopeClear: true, ImplementationKnown: true, VerificationComplete: true, ReviewSufficient: true,
			Evidence: []string{"parser.go and parser_test.go were inspected"}, Risks: []string{},
			NotApplicable: map[string]string{"L2": "No integration boundary changes", "L3": "No user interface changes"}},
	}
	packets := []*Packet{{PacketID: "M01-T01", Requirements: []string{"R-01"},
		Acceptance: Acceptance{L1: []GateEntry{{Cmd: "go test ./...", Expect: "exit 0"}}}}}
	return spec, packets
}

func TestSelectWorkflowKeepsLegacyAndExplicitFull(t *testing.T) {
	spec, packets := qualifiedWorkflowContract()
	spec.Workflow = nil
	selection, err := SelectWorkflow(spec, packets, "")
	require.NoError(t, err)
	assert.Equal(t, "full", selection.Requested)
	assert.Equal(t, "full", selection.Effective)
	for _, stage := range selection.Stages {
		assert.True(t, stage.Applicable)
		assert.NotEmpty(t, stage.Reason)
	}
	selection, err = SelectWorkflow(spec, packets, "quick")
	require.NoError(t, err)
	assert.Equal(t, "quick", selection.Requested)
	assert.Equal(t, "full", selection.Effective)
	assert.Contains(t, selection.Reason, "missing")
	spec, packets = qualifiedWorkflowContract()
	selection, err = SelectWorkflow(spec, packets, "full")
	require.NoError(t, err)
	assert.Equal(t, "full", selection.Effective)
	spec.Workflow.Mode = "full"
	for _, requested := range []string{"", "auto", "quick"} {
		selection, err = SelectWorkflow(spec, packets, requested)
		require.NoError(t, err)
		assert.Equal(t, "full", selection.Effective, "a prepared Full contract is not silently weakened")
	}
}

func TestSelectWorkflowRequiresAllQualificationEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		change       func(*Spec, *[]*Packet)
	}{
		{"unclear scope", "clear scope", func(s *Spec, _ *[]*Packet) { s.Workflow.ScopeClear = false }},
		{"unknown implementation", "known implementation", func(s *Spec, _ *[]*Packet) { s.Workflow.ImplementationKnown = false }},
		{"incomplete verification", "complete checks", func(s *Spec, _ *[]*Packet) { s.Workflow.VerificationComplete = false }},
		{"insufficient review", "independent review", func(s *Spec, _ *[]*Packet) { s.Workflow.ReviewSufficient = false }},
		{"missing explanation", "reason", func(s *Spec, _ *[]*Packet) { s.Workflow.Reason = "  " }},
		{"missing code evidence", "code evidence", func(s *Spec, _ *[]*Packet) { s.Workflow.Evidence = nil }},
		{"declared risk", "permissions", func(s *Spec, _ *[]*Packet) { s.Workflow.Risks = []string{"permissions"} }},
		{"multiple milestones", "one milestone", func(s *Spec, _ *[]*Packet) { s.Milestones = append(s.Milestones, Milestone{ID: "M-02"}) }},
		{"multiple packets", "one independently", func(_ *Spec, p *[]*Packet) { *p = append(*p, &Packet{PacketID: "M01-T02"}) }},
		{"no packet", "one independently", func(_ *Spec, p *[]*Packet) { *p = nil }},
		{"wrong milestone", "sole milestone", func(_ *Spec, p *[]*Packet) { (*p)[0].PacketID = "M02-T01" }},
		{"invalid packet id", "sole milestone", func(_ *Spec, p *[]*Packet) { (*p)[0].PacketID = "invalid" }},
		{"unmapped requirement", "R-01", func(_ *Spec, p *[]*Packet) { (*p)[0].Requirements = nil }},
		{"requirement wrong milestone", "R-01", func(s *Spec, _ *[]*Packet) { s.Requirements[0].Milestone = "M-02" }},
		{"no requirement verification", "explicit verification", func(s *Spec, _ *[]*Packet) { s.Requirements[0].Verification = "" }},
		{"no L1", "L1", func(_ *Spec, p *[]*Packet) { (*p)[0].Acceptance.L1 = nil }},
		{"unexplained L2", "L2", func(s *Spec, _ *[]*Packet) { delete(s.Workflow.NotApplicable, "L2") }},
		{"unexplained L3", "L3", func(s *Spec, _ *[]*Packet) { delete(s.Workflow.NotApplicable, "L3") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec, packets := qualifiedWorkflowContract()
			tc.change(spec, &packets)
			for _, mode := range []string{"auto", "quick"} {
				selection, err := SelectWorkflow(spec, packets, mode)
				require.NoError(t, err)
				assert.Equal(t, "full", selection.Effective)
				assert.Contains(t, selection.Reason, tc.reason)
			}
		})
	}
}

func TestQualifiedQuickRetainsRequiredChecksAndMarksStagesNotApplicable(t *testing.T) {
	for _, mode := range []string{"auto", "quick"} {
		spec, packets := qualifiedWorkflowContract()
		spec.Workflow.Mode = mode
		selection, err := SelectWorkflow(spec, packets, "")
		require.NoError(t, err)
		assert.Equal(t, mode, selection.Requested)
		assert.Equal(t, "quick", selection.Effective)
		assert.Equal(t, spec.Workflow.Evidence, selection.Evidence)
		for _, stage := range selection.Stages {
			assert.False(t, stage.Applicable)
			assert.Contains(t, stage.Reason, "not applicable")
		}
		packets[0].Acceptance.L2 = []GateEntry{{Cmd: "integration-check", Expect: "exit 0"}}
		packets[0].Acceptance.L3 = []GateEntry{{Cmd: "user-path-check", Expect: "exit 0"}}
		spec.Workflow.NotApplicable = nil
		selection, err = SelectWorkflow(spec, packets, mode)
		require.NoError(t, err)
		assert.Equal(t, "quick", selection.Effective, "Quick supports actual L2/L3 instead of requiring them to be absent")
		assert.Equal(t, "integration-check", packets[0].Acceptance.L2[0].Cmd)
	}
}

func TestWorkflowRejectsUnknownModesVersionsAndFieldsAtLoad(t *testing.T) {
	for _, mode := range []string{"Auto", "fast", "standard", "strict", "", " quick"} {
		assert.False(t, ValidWorkflowMode(mode))
	}
	spec, packets := qualifiedWorkflowContract()
	_, err := SelectWorkflow(spec, packets, "fast")
	require.ErrorContains(t, err, "unsupported workflow mode")
	spec.Workflow.Version = 2
	_, err = SelectWorkflow(spec, packets, "auto")
	require.ErrorContains(t, err, "unsupported workflow version")
	spec.Workflow.Version, spec.Workflow.Mode = 1, "fast"
	_, err = SelectWorkflow(spec, packets, "auto")
	require.Error(t, err)
	for _, value := range []string{
		"{version: 2, mode: auto}", "{version: 1, mode: fast}", "{version: 1}",
		"{version: 1, mode: auto, trust-me: true}", "{version: 1, mode: auto, scope-clear: yes}",
		"{version: 1, mode: auto, evidence: ['   ']}", "{version: 1, mode: auto, not-applicable: {L1: none}}",
		"null",
	} {
		_, err = ParseSpec([]byte(strings.Replace(validSpec, "spec-version: 1", "spec-version: 1\nworkflow: "+value, 1)), "spec.md")
		require.Error(t, err, value)
	}
	loaded, err := ParseSpec([]byte(strings.Replace(validSpec, "spec-version: 1", "spec-version: 1\nworkflow: {version: 1, mode: auto}", 1)), "spec.md")
	require.NoError(t, err)
	assert.Equal(t, "auto", loaded.Workflow.Mode)
	packet, err := ParsePacket([]byte(strings.Replace(validPacket, "done-report-format:", "done-report-format:\n  workflow-risks: []", 1)), "task-packet-M01-T01.md")
	require.NoError(t, err)
	assert.Contains(t, packet.DoneReportFormat, "workflow-risks")
}
