package metrics

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFailedDurabilityCannotBeAcknowledgedAndRetryDoesNotDuplicateUsage(t *testing.T) {
	for _, boundary := range []string{"file-sync", "directory-open", "directory-sync"} {
		t.Run(boundary, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "metrics.jsonl")
			originalSync, originalOpen := syncMetricsHandle, openMetricsSync
			t.Cleanup(func() { syncMetricsHandle, openMetricsSync = originalSync, originalOpen })
			failure := errors.New("storage could not make the audit record durable")
			syncMetricsHandle = func(handle *os.File) error {
				if (boundary == "file-sync" && handle.Name() == file) ||
					(boundary == "directory-sync" && handle.Name() == filepath.Dir(file)) {
					return failure
				}
				return originalSync(handle)
			}
			openMetricsSync = func(name string) (*os.File, error) {
				if boundary == "directory-open" && name == filepath.Dir(file) {
					return nil, failure
				}
				return originalOpen(name)
			}
			entry := map[string]any{"usage": map[string]int{"input": 100, "output": 8}}
			require.ErrorIs(t, AppendOnce(file, "one-invocation", "one-receipt", entry), failure)
			before, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Contains(t, string(before), `"invocation-id":"one-invocation"`)
			// Readable bytes are insufficient after a failed durability barrier.
			require.ErrorIs(t, AppendOnce(file, "one-invocation", "one-receipt", entry), failure)
			syncMetricsHandle, openMetricsSync = originalSync, originalOpen
			require.NoError(t, AppendOnce(file, "one-invocation", "one-receipt", entry))
			after, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, 1, strings.Count(string(after), "\n"))
		})
	}
}

func TestMatchedAuditRecordStillRequiresAnAccessibleDurabilityHandle(t *testing.T) {
	file := filepath.Join(t.TempDir(), "metrics.jsonl")
	require.NoError(t, AppendOnce(file, "call", "receipt", nil))
	original := openMetricsSync
	t.Cleanup(func() { openMetricsSync = original })
	failure := errors.New("file descriptor unavailable")
	openMetricsSync = func(string) (*os.File, error) { return nil, failure }
	require.ErrorIs(t, AppendOnce(file, "call", "receipt", nil), failure)
}

func TestAuditWriteFailurePropagatesWithoutClaimingPersistence(t *testing.T) {
	file := filepath.Join(t.TempDir(), "metrics.jsonl")
	original := writeMetricsLine
	t.Cleanup(func() { writeMetricsLine = original })
	failure := errors.New("audit device exhausted")
	writeMetricsLine = func(*os.File, []byte) (int, error) { return 0, failure }
	require.ErrorIs(t, Append(file, map[string]any{"usage": 42}), failure)
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Empty(t, raw)
}
