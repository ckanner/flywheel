package verify

import (
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGateFeedbackRecordsStableCommandObservations(t *testing.T) {
	commands := []contract.GateEntry{
		gateEntry("printf failure; exit 7", "exit 0"),
		gateEntry("printf failure; exit 7", "exit 0"),
		scriptEntry("true", "exit 0"),
		gateEntry("printf unexpected-success", "exit 1"),
		gateEntry("true", "invalid expect"),
	}
	opts := GateOpts{Cwd: t.TempDir()}
	first := RunGateCommands(commands, opts)
	second := RunGateCommands(commands, opts)
	require.Len(t, first.Results, len(commands))
	require.Len(t, second.Results, len(commands))
	assert.False(t, first.Pass)
	assert.NotEmpty(t, first.EnvironmentID)
	assert.Equal(t, first.EnvironmentID, second.EnvironmentID)
	for i, result := range first.Results {
		assert.Equal(t, gateCheckID(i, commands[i].Command(), commands[i].Expect), result.CheckID)
		assert.Equal(t, result.CheckID, second.Results[i].CheckID)
		assert.Equal(t, i < 4, result.ObservationComplete, "ordinary success/failure is known; invalid expectation was never executed")
		assert.False(t, result.OutputTruncated)
		if result.ObservationComplete && !result.Pass {
			assert.NotEmpty(t, result.OutputDigest)
			assert.Equal(t, result.OutputDigest, second.Results[i].OutputDigest)
		} else {
			assert.Empty(t, result.OutputDigest, "only complete failed command output is comparable failure evidence")
		}
	}
	assert.NotEqual(t, first.Results[0].CheckID, first.Results[1].CheckID)
	assert.True(t, first.Results[2].Pass, "ordinary failure must not prevent the remaining gate commands from running")
	assert.False(t, first.Results[3].Pass)
	assert.Equal(t, "unexpected-success", first.Results[3].Output, "a failed expected-nonzero check still needs its diagnostic output")
}

func TestGateEnvironmentObservationIncludesOnlySuppliedFilteredValues(t *testing.T) {
	opts := GateOpts{Cwd: t.TempDir(), Env: map[string]string{"OBSERVED": "first", "SECRET": "hidden"}, EnvAllowlist: []string{"OBSERVED"}}
	commands := []contract.GateEntry{gateEntry("printf failure; exit 1", "exit 0")}
	first := RunGateCommands(commands, opts)
	opts.Env["SECRET"] = "changed hidden value"
	second := RunGateCommands(commands, opts)
	assert.Equal(t, first.EnvironmentID, second.EnvironmentID, "filtered secrets must not affect the actual command environment")
	opts.Env["OBSERVED"] = "second"
	third := RunGateCommands(commands, opts)
	assert.NotEqual(t, second.EnvironmentID, third.EnvironmentID)
	assert.NotContains(t, third.EnvironmentID, "second")
}

func TestGateFeedbackSummaryClippingDoesNotImplyProcessOutputLoss(t *testing.T) {
	command := "printf 'BEGIN\\n'; printf '%05000d' 0; printf '\\nFAIL: terminal diagnostic\\n'; exit 1"
	report := RunGateCommands([]contract.GateEntry{gateEntry(command, "exit 1")}, GateOpts{Cwd: t.TempDir()})
	require.Len(t, report.Results, 1)
	result := report.Results[0]
	assert.True(t, report.Pass, "complete expected exit remains a pass when only its diagnostic summary is clipped")
	assert.True(t, result.ObservationComplete)
	assert.True(t, result.OutputTruncated)
	assert.False(t, result.Truncated, "watchdog output loss has separate gate semantics")
	assert.Empty(t, result.OutputDigest, "expected nonzero success is not recorded as a failed check")
	assert.LessOrEqual(t, len(result.Output), feedbackOutputLimit)
	assert.True(t, strings.HasPrefix(result.Output, "BEGIN\n"))
	assert.True(t, strings.HasSuffix(result.Output, "FAIL: terminal diagnostic\n"))
}
