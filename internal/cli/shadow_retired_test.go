package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShadowCommandExplainsMigrationWithoutRunningOldExperiments(t *testing.T) {
	for _, command := range [][]string{{"shadow"}, {"shadow", "prepare", "--task-dir", "docs/old"}, {"shadow", "run", "--endpoint", "https://example.invalid/v1/systemone"}, {"shadow", "report"}} {
		code, out := runCLI(t, command...)
		require.Equal(t, 1, code, out)
		require.Contains(t, out, "shadow evaluation has been retired")
		require.Contains(t, out, "jev.enabled")
	}
	code, out := runCLI(t, "--help")
	require.Zero(t, code, out)
	require.NotContains(t, out, "  shadow")
}
