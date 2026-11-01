package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInvalidHarnessAndProtectedPatternsFailAtLoad(t *testing.T) {
	for _, raw := range []string{
		`{"loops":{"review":{"harness":"typo"}}}`,
		`{"routes":{"default":{"harness":"typo"}}}`,
		`{"channels":{"fallback":{"harness":"typo"}}}`,
		`{"judge-panel":{"harness":"typo","model":"x","channel":"x"}}`,
		`{"verify":{"test-file-patterns":["["]}}`,
		`{"verify":{"protected-test-patterns":["["]}}`,
	} {
		_, err := parseConfig([]byte(raw), "test config")
		require.Error(t, err, raw)
	}
	_, err := parseConfig([]byte(`{"judge-panel":{"harness":"dsh","model":"x","channel":"chatgpt"},"verify":{"protected-test-patterns":["^acceptance/"]}}`), "test config")
	require.NoError(t, err)
}
