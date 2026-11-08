package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendOnceReconcilesUsageWithoutDuplicateRecords(t *testing.T) {
	file := filepath.Join(t.TempDir(), "task", "metrics.jsonl")
	digest := strings.Repeat("a", 64)
	entry := map[string]any{"usage": map[string]any{"input": 100, "output": 8}}
	require.NoError(t, AppendOnce(file, "call-one", digest, entry))
	assert.NotContains(t, entry, "invocation-id", "metadata must not mutate the reusable caller entry")
	before, err := os.ReadFile(file)
	require.NoError(t, err)
	require.NoError(t, AppendOnce(file, "call-one", digest, entry))
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	require.NoError(t, AppendOnce(file, "call-two", strings.Repeat("b", 64), entry))
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	require.Len(t, lines, 2)
	var tokens float64
	for _, line := range lines {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		usage := record["usage"].(map[string]any)
		tokens += usage["input"].(float64) + usage["output"].(float64)
	}
	assert.Equal(t, float64(216), tokens, "only the two actual invocations are represented")
}

func TestAppendOnceAcceptsLegacyLinesAndNilEntries(t *testing.T) {
	file := filepath.Join(t.TempDir(), "metrics.jsonl")
	require.NoError(t, AppendOnce(file, "", "", nil))
	require.NoError(t, AppendOnce(file, "", "", map[string]any{"legacy": true}))
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString(" \t\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, AppendOnce(file, "call-one", strings.Repeat("a", 64), nil))
	require.NoError(t, AppendOnce(file, "call-one", strings.Repeat("a", 64), nil))
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(raw), `"invocation-id":"call-one"`))
	assert.Equal(t, 4, strings.Count(string(raw), "\n"))
}

func TestAppendOnceRejectsAmbiguousOrTornHistoryWithoutWriting(t *testing.T) {
	digest := strings.Repeat("a", 64)
	matching := `{"invocation-id":"call-one","receipt-sha256":"` + digest + `","usage":{"input":100}}` + "\n"
	conflicting := `{"invocation-id":"call-one","receipt-sha256":"` + strings.Repeat("b", 64) + `"}` + "\n"
	for _, tc := range []struct{ name, content, message string }{
		{"different receipt", conflicting, "identity mismatch"},
		{"conflicting duplicate after match", matching + conflicting, "identity mismatch"},
		{"identical duplicate", matching + matching, "duplicate invocation"},
		{"corruption after match", matching + "{broken\n", "cannot reconcile"},
		{"truncated after match", matching + `{"invocation-id":`, "unterminated"},
		{"complete JSON missing newline", strings.TrimSuffix(matching, "\n"), "unterminated"},
		{"torn unrelated record", `{"usage":`, "unterminated"},
		{"nonobject", "[]\n", "cannot reconcile"},
		{"null object", "null\n", "must be an object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "metrics.jsonl")
			require.NoError(t, os.WriteFile(file, []byte(tc.content), 0o600))
			err := AppendOnce(file, "call-one", digest, map[string]any{"usage": 108})
			require.ErrorContains(t, err, tc.message)
			after, err := os.ReadFile(file)
			require.NoError(t, err)
			assert.Equal(t, tc.content, string(after))
		})
	}
}

func TestAppendOnceReadAndAppendFailuresPropagate(t *testing.T) {
	require.Error(t, AppendOnce(t.TempDir(), "call-one", "receipt", nil))
	file := filepath.Join(t.TempDir(), "metrics.jsonl")
	require.Error(t, AppendOnce(file, "call-one", "receipt", map[string]any{"invalid": make(chan int)}))
	_, err := os.Stat(file)
	require.True(t, os.IsNotExist(err), "serialization failure must leave no partial record")
}
