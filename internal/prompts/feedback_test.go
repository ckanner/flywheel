package prompts

import (
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/stretchr/testify/assert"
)

func TestExecutorFeedbackChecksPreserveExternalCompletionGates(t *testing.T) {
	prompt := BuildExecutePrompt(testPacket(), testSpec(), "/task", "", 1)
	assert.Contains(t, prompt, "Use targeted feedback checks")
	assert.Contains(t, prompt, "Report exactly which commands you ran and their observed outcomes")
	assert.Contains(t, prompt, "do not claim unrun acceptance commands passed")
	assert.Contains(t, prompt, "The orchestrator runs every required final gate, including full L1 and coverage")
	assert.Contains(t, prompt, "a redundant full-suite run is not required solely to prove completion")
	assert.Contains(t, prompt, "- go test ./...  (expect exit 0)", "the frozen full check remains visible to the executor")
	assert.Contains(t, prompt, "Do not weaken, delete, or bypass assertions")
	assert.Contains(t, prompt, "NEVER declare the task done")
	assert.NotContains(t, prompt, "Run the acceptance commands yourself")
}

func TestDeclaredFeedbackChecksStayDistinctFromCompletionEvidence(t *testing.T) {
	packet := testPacket()
	packet.FeedbackChecks = []contract.GateEntry{{Cmd: "go test ./config -run Timeout", Expect: "exit 0", Purpose: "Check whether timeout defaults caused the failure"}, {Script: "scripts/reproduce.sh", Expect: "exit 1"}}
	prompt := BuildExecutePrompt(packet, testSpec(), "/task", "", 1)
	assert.Contains(t, prompt, "Approved short feedback checks (diagnostic only)")
	assert.Contains(t, prompt, "- go test ./config -run Timeout  (expect exit 0)")
	assert.Contains(t, prompt, "- scripts/reproduce.sh  (expect exit 1)")
	assert.Contains(t, prompt, "Diagnostic purpose: Check whether timeout defaults caused the failure")
	assert.Contains(t, prompt, "They do not replace any acceptance command or establish completion")
	assert.Contains(t, prompt, "- go test ./...  (expect exit 0)")
	packet.FeedbackChecks = nil
	assert.NotContains(t, BuildExecutePrompt(packet, testSpec(), "/task", "", 1), "Approved short feedback checks")
}
