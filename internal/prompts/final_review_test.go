package prompts

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func finalSpec() *contract.Spec {
	return &contract.Spec{Goal: "Observable task outcome", Requirements: []contract.Requirement{
		{ID: "R-01", Title: "first", Verification: "unit test"},
		{ID: "R-02", Title: "second", Verification: "integration test"},
	}}
}

func finalPayload() map[string]any {
	return map[string]any{
		"verdict": "pass", "score": 1.0, "reasons": []string{"unit and integration commands exit 0"}, "quick-eligible": true,
		"requirements": []any{
			map[string]any{"id": "R-01", "pass": true, "evidence": []string{"unit command exit 0"}},
			map[string]any{"id": "R-02", "pass": true, "evidence": []string{"integration command exit 0"}},
		},
	}
}

func TestFinalReviewRequiresCompleteStructuredEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"verdict", func(p map[string]any) { p["verdict"] = "maybe" }},
		{"missing-score", func(p map[string]any) { delete(p, "score") }},
		{"negative-score", func(p map[string]any) { p["score"] = -0.1 }},
		{"large-score", func(p map[string]any) { p["score"] = 1.1 }},
		{"missing-reasons", func(p map[string]any) { p["reasons"] = []string{} }},
		{"blank-reasons", func(p map[string]any) { p["reasons"] = []string{" "} }},
		{"missing-eligibility", func(p map[string]any) { delete(p, "quick-eligible") }},
		{"ineligible-pass", func(p map[string]any) { p["quick-eligible"] = false }},
		{"eligible-upgrade", func(p map[string]any) { p["verdict"] = "upgrade" }},
		{"missing-requirements", func(p map[string]any) { delete(p, "requirements") }},
		{"missing-one", func(p map[string]any) { p["requirements"] = p["requirements"].([]any)[:1] }},
		{"duplicate", func(p map[string]any) { p["requirements"].([]any)[1].(map[string]any)["id"] = "R-01" }},
		{"extra", func(p map[string]any) { p["requirements"].([]any)[1].(map[string]any)["id"] = "R-99" }},
		{"missing-result", func(p map[string]any) { delete(p["requirements"].([]any)[0].(map[string]any), "pass") }},
		{"failed-requirement", func(p map[string]any) { p["requirements"].([]any)[0].(map[string]any)["pass"] = false }},
		{"no-evidence", func(p map[string]any) { p["requirements"].([]any)[0].(map[string]any)["evidence"] = []string{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := finalPayload()
			tc.mutate(p)
			raw, err := json.Marshal(p)
			require.NoError(t, err)
			_, err = ParseFinalReviewVerdict(string(raw), finalSpec())
			require.Error(t, err)
		})
	}
	for _, verdict := range []string{"pass", "return", "verification-gap", "upgrade"} {
		p := finalPayload()
		p["verdict"] = verdict
		if verdict != "pass" {
			p["requirements"].([]any)[0].(map[string]any)["pass"] = false
		}
		if verdict == "upgrade" {
			p["quick-eligible"] = false
		}
		raw, err := json.Marshal(p)
		require.NoError(t, err)
		got, err := ParseFinalReviewVerdict("Evidence above\n```json\n"+string(raw)+"\n```", finalSpec())
		require.NoError(t, err)
		require.Equal(t, verdict, got.Verdict)
	}
}

func TestFinalReviewNeverReusesEarlierPassAfterInvalidFinalOutput(t *testing.T) {
	raw, err := json.Marshal(finalPayload())
	require.NoError(t, err)
	for _, text := range []string{"", "```", "invalid", string(raw) + "\n{}", string(raw) + "\nI retract the prior verdict"} {
		_, err := ParseFinalReviewVerdict(text, finalSpec())
		require.Error(t, err)
	}
	_, err = ParseFinalReviewVerdict("```json"+string(raw)+"```", finalSpec())
	require.NoError(t, err)
}

func TestFinalReviewPromptContainsWholeTaskAndCandidateGates(t *testing.T) {
	p := &contract.Packet{PacketID: "M01-T01", Goal: "fixture", Requirements: []string{"R-01", "R-02"}, ReviewRubric: "assertion quality"}
	prompt := BuildReviewPrompt(ReviewArgs{
		Packet: p, FinalSpec: finalSpec(), L1Evidence: map[string]any{"pass": true},
		DiffText: "authoritative diff", FinalGate: map[string]any{"L2": "actual candidate command exit 0"},
		Workflow: map[string]any{"effective": "quick"},
	})
	for _, want := range []string{"task FINAL Review", "R-01", "R-02", "integration test", "actual candidate command exit 0", "quick-eligible", "verification-gap", "upgrade", "authoritative diff"} {
		require.Contains(t, prompt, want)
	}
	require.Equal(t, 1, strings.Count(prompt, "## Output"))
	p.DoneReportFormat = map[string][]string{"workflow-risks": {}}
	require.Contains(t, BuildExecutePrompt(p, finalSpec(), "/task", "", 1), "workflow-risks string array")
	st := &state.State{Workflow: &state.WorkflowState{WorkflowSelection: contract.WorkflowSelection{Effective: "full"}}}
	require.Contains(t, BuildPlanPrompt(PlanArgs{Spec: finalSpec(), State: st}), "never downgrade or rewrite state")
}
