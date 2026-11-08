package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateAcknowledgementRequiresFileAndDirectoryDurability(t *testing.T) {
	require.Error(t, syncStatePath(filepath.Join(t.TempDir(), "missing")))
	for _, failure := range []int{1, 2} {
		t.Run(map[int]string{1: "temporary-file", 2: "parent-directory"}[failure], func(t *testing.T) {
			dir := t.TempDir()
			st := &State{Slug: "durable"}
			old := syncStateFile
			t.Cleanup(func() { syncStateFile = old })
			calls := 0
			syncStateFile = func(file *os.File) error {
				calls++
				if calls == failure {
					return errors.New("fsync unavailable")
				}
				return old(file)
			}
			_, err := Save(dir, st, nil)
			require.ErrorContains(t, err, "fsync")
			syncStateFile = old
			if failure == 1 {
				assert.False(t, Exists(dir))
			} else {
				loaded, loadErr := Load(dir)
				require.NoError(t, loadErr)
				assert.Equal(t, int64(1), loaded.Revision)
				_, err = Save(dir, st, nil)
				var conflict *CasConflictError
				require.ErrorAs(t, err, &conflict)
			}
		})
	}
}

func TestRecoveryDirectoryIsPrivateAndOnlyItsParentIsPrepared(t *testing.T) {
	dir := t.TempDir()
	recovery, err := PrepareSessionRecovery(dir, "call-one")
	require.NoError(t, err)
	assert.Equal(t, SessionRecoveryDirectory(dir, "call-one"), recovery)
	assert.NotEqual(t, recovery, SessionRecoveryDirectory(dir, "call-two"))
	_, err = os.Stat(recovery)
	assert.True(t, os.IsNotExist(err), "the adapter must create its leaf exclusively")
	info, err := os.Stat(filepath.Dir(recovery))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	unsafe := t.TempDir()
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(unsafe, ".sessions")))
	_, err = PrepareSessionRecovery(unsafe, "call-one")
	require.Error(t, err)
}

func TestRecoveryStateKeepsExactInvocationPathsAndAccounting(t *testing.T) {
	dir := t.TempDir()
	st := sessionTestState(t, dir)
	r := st.Sessions.Records["execute/M01-T01"]
	r.Last.Recovery = &SessionRecovery{Version: 1, Harness: "codex-pro", AdapterVersion: "0.9.0", Directory: SessionRecoveryDirectory(dir, r.Last.ID), Model: "gpt-test", Effort: "high", PermissionMode: "workspace-write"}
	r.Quality = &SessionQuality{ConfirmedTurns: 2, UnproductiveTurns: 1, LastUsageStatus: "complete", LastInputTokens: new(int64(17)), Rebuilds: 1, RebuildReason: "measured limit", LastInvocationID: r.Last.ID}
	r.Last.Accounted = true
	r.Last.Accounting = &SessionAccounting{Day: "2026-11-07", Provider: "subscription", ReceiptDigest: r.Last.ReceiptDigest, Usage: &SessionUsage{Input: 17, Output: 5, TotalTokens: 22}}
	_, err := Save(dir, st, nil)
	require.NoError(t, err)
	got, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, st.Sessions, got.Sessions)
	for _, edit := range []func(*SessionRecord){
		func(r *SessionRecord) { r.Last.Recovery.Directory = t.TempDir() },
		func(r *SessionRecord) { r.Last.Recovery.RunID = "unacknowledged" },
		func(r *SessionRecord) { r.Last.Recovery.Sequence = 1 },
		func(r *SessionRecord) { r.Last.Recovery.Version = 2 },
		func(r *SessionRecord) { r.Last.Recovery.Sequence = 65 },
		func(r *SessionRecord) { r.Quality.ConfirmedTurns = -1 },
		func(r *SessionRecord) { r.Last.Accounting.Usage.Input = -1 },
	} {
		raw, _ := json.Marshal(st.Sessions)
		var changed SessionLedger
		require.NoError(t, json.Unmarshal(raw, &changed))
		edit(changed.Records["execute/M01-T01"])
		raw, _ = json.Marshal(changed)
		require.Error(t, validateSessions(raw))
	}
	r.Last.Recovery.Sequence, r.Last.Recovery.RunID, r.Last.Recovery.Event, r.Last.Recovery.CheckpointDigest = 1, "run-one", "thread-ready", strings.Repeat("a", 64)
	raw, _ := json.Marshal(st.Sessions)
	require.NoError(t, validateSessions(raw))
}
