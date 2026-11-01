package machine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanCheckpointRejectsCommitHookContractDrift(t *testing.T) {
	for _, scenario := range []string{"worktree-contract", "index-contract", "index-code"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, id := pausedContractFixture(t, true)
			path := "docs/minitask/task-packet-" + id + ".md"
			appendPacketRevision(t, fx, id)
			require.NoError(t, c.reloadPackets())
			expectedBase := *c.State.Packets[id].BaseRef
			if scenario == "worktree-contract" {
				installCommitHook(t, fx, "printf '\\nUnexpected hook revision\\n' >> '"+path+"'\ngit add -- '"+path+"'")
			} else {
				target := path
				if scenario == "index-code" {
					target = "hook-only-source.go"
				}
				replacement := filepath.Join(t.TempDir(), "replacement")
				writeFile(t, replacement, "unexpected persisted blob\n")
				blob := strings.TrimSpace(gitCmd(t, fx.root, "hash-object", "-w", "--", replacement))
				installCommitHook(t, fx, "git update-index --add --cacheinfo '100644,"+blob+","+target+"'")
			}
			err := c.persistPlanContracts("docs: enforce validated checkpoint")
			require.Error(t, err)
			if scenario == "index-code" {
				assert.ErrorIs(t, err, ErrMergeConflict)
				assert.Contains(t, err.Error(), "unexpected changes")
			} else {
				assert.ErrorIs(t, err, ErrCommitFailed)
				assert.Contains(t, err.Error(), "contract")
			}
			ps := c.State.Packets[id]
			assert.Equal(t, expectedBase, *ps.BaseRef)
			assert.True(t, ps.ContractSyncPending)
			assert.FileExists(t, filepath.Join(ps.Worktree.Path, "wip.txt"))
			assert.NotEqual(t, "completed", ps.Status)
		})
	}
}

func TestPlanCheckpointSnapshotAndPersistenceReadFailures(t *testing.T) {
	for _, phase := range []string{"before", "after", "persisted-inventory"} {
		t.Run(phase, func(t *testing.T) {
			fx, c, id := pausedContractFixture(t, true)
			appendPacketRevision(t, fx, id)
			require.NoError(t, c.reloadPackets())
			oldSnapshot, oldDirty := snapshotGitFiles, inspectPlanDirtyFiles
			t.Cleanup(func() { snapshotGitFiles = oldSnapshot; inspectPlanDirtyFiles = oldDirty })
			if phase == "persisted-inventory" {
				inspectPlanDirtyFiles = func(string, string) ([]string, bool) { return nil, false }
			} else {
				reads := 0
				snapshotGitFiles = func(cwd string) ([]byte, error) {
					reads++
					if phase == "before" || reads == 2 {
						return nil, os.ErrPermission
					}
					return oldSnapshot(cwd)
				}
			}
			err := c.persistPlanContracts("docs: preserve unreadable checkpoint")
			require.ErrorIs(t, err, ErrCommitFailed)
			assert.False(t, errors.Is(err, ErrMergeConflict))
			assert.DirExists(t, c.State.Packets[id].Worktree.Path)
			assert.True(t, c.State.Packets[id].ContractSyncPending)
		})
	}
}
