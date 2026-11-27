package prompts

import (
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/stretchr/testify/require"
)

func TestHumanGuidanceAndUniversalReviewPrinciples(t *testing.T) {
	pkt := &contract.Packet{
		PacketID:     "M01-T01",
		Goal:         "Implement feature",
		Requirements: []string{"R-01"},
		ReviewRubric: "All requirements pass",
	}
	spec := &contract.Spec{Goal: "Ship system"}

	execPrompt := BuildExecutePrompt(pkt, spec, "/tmp/task", "", 2, ExecuteOptions{
		HumanGuidance: "  Focus on R-01 core behavior; defer cosmetic polish.  ",
	})
	require.Contains(t, execPrompt, "## Human triage guidance (authoritative scope decision)")
	require.Contains(t, execPrompt, "Focus on R-01 core behavior; defer cosmetic polish.")

	revPrompt := BuildReviewPrompt(ReviewArgs{
		Packet:        pkt,
		L1Evidence:    map[string]any{"pass": true},
		HumanGuidance: "  Defer optional meta-tests to later milestone. ",
	})
	require.Contains(t, revPrompt, "Closed-world DoD:")
	require.Contains(t, revPrompt, "Test proportionality:")
	require.Contains(t, revPrompt, "Round-2+ scope freeze (delta review):")
	require.Contains(t, revPrompt, "## Human triage guidance (authoritative scope decision)")
	require.Contains(t, revPrompt, "Defer optional meta-tests to later milestone.")
}
