package prompts

import (
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
)

func TestExecuteProtectedPatternsAreStableAcrossFeedback(t *testing.T) {
	options := ExecuteOptions{ProtectedTestPatterns: []string{`^acceptance/auth/`, `^e2e/auth-flow\.ts$`}}
	initial := BuildExecutePrompt(testPacket(), testSpec(), "/task", "", 1, options)
	options.Feedback = map[string]any{"output": "assertion got 2 want 3"}
	retry := BuildExecutePrompt(testPacket(), testSpec(), "/task", "", 2, options)
	stable := strings.Split(initial, "## Runtime (volatile)")[0]
	assert.Equal(t, stable, strings.Split(retry, "## Runtime (volatile)")[0])
	assert.Contains(t, stable, "verify.protected-test-patterns (regular expressions): ^acceptance/auth/, ^e2e/auth-flow\\.ts$")
	assert.Contains(t, stable, "boundaries.must-not-touch: src/payments/")
	assert.NotContains(t, stable, "assertion got 2 want 3")
	assert.Contains(t, retry, "assertion got 2 want 3")
	assert.NotContains(t, initial, "Previous attempt feedback")

	legacy := BuildExecutePrompt(testPacket(), testSpec(), "/task", "", 3, nil)
	assert.Contains(t, legacy, "verify.protected-test-patterns (regular expressions): none")
	assert.NotContains(t, legacy, "Previous attempt feedback")
}

func TestPlanCannotDirectlyReviseExternalFrozenOracles(t *testing.T) {
	prompt := BuildPlanPrompt(PlanArgs{
		Spec: testSpec(), State: &state.State{}, Milestone: contract.Milestone{ID: "M-01"},
	})
	assert.Contains(t, prompt, "only spec.md and task-packet-*.md inside the task directory")
	assert.Contains(t, prompt, "Do not modify flywheel.config.json")
	assert.Contains(t, prompt, "require the user to edit the configuration and restart")
	assert.Contains(t, prompt, "Do not modify implementation files or acceptance oracles outside that directory")
	assert.Contains(t, prompt, "including files listed in boundaries.must-not-touch")
	assert.Contains(t, prompt, "stop for an explicit human task")
	assert.Contains(t, prompt, "cannot apply or synchronize that revision automatically")
}
