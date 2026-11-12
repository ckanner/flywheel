package verify

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGateCheckIdentityIncludesPositionCommandAndExpectation(t *testing.T) {
	first := gateCheckID(0, "go test ./...", "exit 0")
	assert.Regexp(t, `^check-[0-9a-f]{64}$`, first)
	assert.Equal(t, first, gateCheckID(0, "go test ./...", "exit 0"))
	assert.NotEqual(t, first, gateCheckID(1, "go test ./...", "exit 0"), "repeated checks have distinct identities")
	assert.NotEqual(t, first, gateCheckID(0, "go test ./one-package", "exit 0"), "narrowing the check set is not comparable evidence")
	assert.NotEqual(t, first, gateCheckID(0, "go test ./...", "exit 1"), "changing the expected outcome invalidates the check identity")
}

func TestGateEnvironmentIdentityIsCanonicalAndTracksValues(t *testing.T) {
	first := gateEnvironmentID(map[string]string{"PATH": "/tools", "LANG": "C", "APP_ENV": "test"})
	assert.Regexp(t, `^[0-9a-f]{64}$`, first)
	assert.Equal(t, first, gateEnvironmentID(map[string]string{"APP_ENV": "test", "LANG": "C", "PATH": "/tools"}))
	assert.NotEqual(t, first, gateEnvironmentID(map[string]string{"APP_ENV": "production", "LANG": "C", "PATH": "/tools"}))
	assert.NotEqual(t, first, gateEnvironmentID(map[string]string{"LANG": "C", "PATH": "/tools"}))
}

func TestGateOutputDigestPreservesCompleteStreams(t *testing.T) {
	common := strings.Repeat("same intermediate line\n", 500)
	first := gateOutputDigest(common+"first failing assertion", "stderr")
	assert.Regexp(t, `^[0-9a-f]{64}$`, first)
	assert.Equal(t, first, gateOutputDigest(common+"first failing assertion", "stderr"))
	assert.NotEqual(t, first, gateOutputDigest(common+"different failing assertion", "stderr"))
	assert.NotEqual(t, gateOutputDigest("a", "bc"), gateOutputDigest("ab", "c"), "stdout and stderr boundaries are part of the observed evidence")
	assert.NotEqual(t, gateOutputDigest("\xff", ""), gateOutputDigest("\xfe", ""), "raw non-UTF-8 bytes must remain distinguishable before summary normalization")
}

func TestFeedbackSummaryPreservesContextAndTerminalFailure(t *testing.T) {
	const head = "building dependency graph\n"
	const tail = "\nFAIL: assertion got 2, want 3\n"
	output := head + strings.Repeat("irrelevant intermediate output\n", 200) + tail
	summary, clipped := summarizeOutput(output)
	require.True(t, clipped)
	assert.LessOrEqual(t, len(summary), feedbackOutputLimit)
	assert.True(t, strings.HasPrefix(summary, head))
	assert.True(t, strings.HasSuffix(summary, tail))
	assert.Contains(t, summary, "[output summary truncated]")
}

func TestFeedbackSummaryBoundsAndUTF8(t *testing.T) {
	for _, output := range []string{"", "short failure", strings.Repeat("x", feedbackOutputLimit)} {
		summary, clipped := summarizeOutput(output)
		assert.False(t, clipped)
		assert.Equal(t, output, summary)
	}
	for _, output := range []string{
		strings.Repeat("x", feedbackOutputLimit+1),
		strings.Repeat("错误🧪", 401),
		"ab" + strings.Repeat("错误🧪", 401),
	} {
		summary, clipped := summarizeOutput(output)
		require.True(t, clipped)
		assert.LessOrEqual(t, len(summary), feedbackOutputLimit)
		assert.True(t, utf8.ValidString(summary), "head/tail boundaries must not split a UTF-8 code point")
		assert.NotContains(t, summary, "\uFFFD", "valid log bytes must remain intact")
	}
	summary, clipped := summarizeOutput("before\xffafter")
	assert.False(t, clipped)
	assert.Equal(t, "before\uFFFDafter", summary)
	assert.True(t, utf8.ValidString(summary), "non-text output must still form a valid JSON diagnostic")
}
