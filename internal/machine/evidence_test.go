package machine

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func candidateID(t *testing.T, c *Context, cwd string) string {
	t.Helper()
	id, err := c.candidateFingerprint(cwd)
	require.NoError(t, err)
	require.Len(t, id, 64)
	return id
}

func TestCandidateIdentityTracksContentWithoutDependingOnIndexOrMtime(t *testing.T) {
	fx := newGitFixture(t, "binary.bin", "delete-me.txt")
	c := unitTestContext(t, fx)
	binary := filepath.Join(fx.root, "binary.bin")
	contents := []byte{0, 1, 2, 0xff, '\n'}
	require.NoError(t, os.WriteFile(binary, contents, 0o644))
	gitCmd(t, fx.root, "add", "binary.bin")
	gitCmd(t, fx.root, "commit", "-m", "binary baseline")
	base := candidateID(t, c, fx.root)
	require.NoError(t, os.Chtimes(binary, time.Now().Add(time.Hour), time.Now().Add(time.Hour)))
	assert.Equal(t, base, candidateID(t, c, fx.root), "mtime不代表候选内容改变")
	require.NoError(t, os.WriteFile(binary, []byte{0, 1, 3, 0xff, '\n'}, 0o644))
	assert.NotEqual(t, base, candidateID(t, c, fx.root), "二进制内容也必须改变身份")
	require.NoError(t, os.WriteFile(binary, contents, 0o644))
	require.NoError(t, os.Chmod(binary, 0o755))
	assert.NotEqual(t, base, candidateID(t, c, fx.root), "执行权限属于候选身份")
	require.NoError(t, os.Chmod(binary, 0o644))
	assert.Equal(t, base, candidateID(t, c, fx.root))

	newPath := "untracked 中文 file.go"
	writeFile(t, filepath.Join(fx.root, newPath), "new implementation")
	withNewFile := candidateID(t, c, fx.root)
	assert.NotEqual(t, base, withNewFile)
	gitCmd(t, fx.root, "add", "--", newPath)
	assert.Equal(t, withNewFile, candidateID(t, c, fx.root), "stage不改变候选")
	gitCmd(t, fx.root, "commit", "-m", "persist addition")
	assert.Equal(t, withNewFile, candidateID(t, c, fx.root), "commit不改变候选")

	require.NoError(t, os.Remove(filepath.Join(fx.root, "delete-me.txt")))
	deleted := candidateID(t, c, fx.root)
	assert.NotEqual(t, withNewFile, deleted)
	gitCmd(t, fx.root, "add", "-u", "--", "delete-me.txt")
	gitCmd(t, fx.root, "commit", "-m", "persist deletion")
	assert.Equal(t, deleted, candidateID(t, c, fx.root), "删除前后不能因index墓碑消失使已验收内容过期")

	writeFile(t, filepath.Join(fx.root, ".gitignore"), "ignored-cache/\n")
	beforeIgnored := candidateID(t, c, fx.root)
	writeFile(t, filepath.Join(fx.root, "ignored-cache", "cache.bin"), "generated cache")
	assert.Equal(t, beforeIgnored, candidateID(t, c, fx.root), "非跟踪且gitignored的缓存不属于候选")

	original := snapshotGitFiles
	t.Cleanup(func() { snapshotGitFiles = original })
	snapshotGitFiles = func(cwd string) ([]byte, error) {
		raw, err := original(cwd)
		return append(raw, []byte("binary.bin\x00\x00")...), err
	}
	assert.Equal(t, beforeIgnored, candidateID(t, c, fx.root), "重复inventory条目不能改变身份")
}

func TestCandidateSymlinkIsHashedWithoutFollowingTarget(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeFile(t, outside, "outside content")
	link := filepath.Join(fx.root, "source.link")
	require.NoError(t, os.Symlink(outside, link))
	base := candidateID(t, c, fx.root)
	writeFile(t, outside, "changed outside content")
	assert.Equal(t, base, candidateID(t, c, fx.root))
	require.NoError(t, os.Remove(outside))
	assert.Equal(t, base, candidateID(t, c, fx.root), "dangling link仍按目标字符串计算，不读取目标")
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink("different-target", link))
	assert.NotEqual(t, base, candidateID(t, c, fx.root))
}

func TestVerificationIdentityBindsAuthoritativeContractsChecksAndOwnReport(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	p := c.Packets["M01-T01"]
	base, ok := verify.GetBaseRef(fx.root)
	require.True(t, ok)
	wi := verify.EnsureUnitWorktree(fx.root, fx.slug, p.PacketID, base)
	require.Equal(t, "worktree", wi.Mode)
	c.State.Packets[p.PacketID].Worktree = &wi
	identity := func() string {
		id, err := c.verificationIdentity(p)
		require.NoError(t, err)
		return id
	}
	original := identity()
	for _, rel := range []string{"state.json", "retro.md", "metrics.jsonl", "orchestrator.log", "reports/other.done.json", "approvals/keys.json"} {
		writeFile(t, filepath.Join(wi.Path, "docs", fx.slug, rel), "runtime ledger")
	}
	assert.Equal(t, original, identity(), "worktree账本不会使候选身份抖动")
	writeFile(t, filepath.Join(fx.taskDir, "reports", "other.done.json"), "other packet report")
	assert.Equal(t, original, identity())

	for _, name := range []string{"spec.md", "task-packet-" + p.PacketID + ".md"} {
		path := filepath.Join(fx.taskDir, name)
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		writeFile(t, path, string(raw)+"\nNew authoritative requirement\n")
		assert.NotEqual(t, original, identity(), "主目录契约原文修订必须使worktree里的旧证据失效")
		require.NoError(t, os.WriteFile(path, raw, 0o644))
	}
	c.Config.Verify.CommandTimeoutMs++
	assert.NotEqual(t, original, identity())
	c.Config.Verify.CommandTimeoutMs--
	c.Config.Verify.ProtectedTestPatterns = []string{`^acceptance/`}
	assert.NotEqual(t, original, identity())
	c.Config.Verify.ProtectedTestPatterns = nil
	packetGoal := p.Goal
	p.Goal += " changed"
	assert.NotEqual(t, original, identity())
	p.Goal = packetGoal
	specGoal := c.Spec.Goal
	c.Spec.Goal += " changed"
	assert.NotEqual(t, original, identity())
	c.Spec.Goal = specGoal
	assert.Equal(t, original, identity())

	reportPath := filepath.Join(fx.taskDir, "reports", p.PacketID+".done.json")
	writeFile(t, reportPath, `{"requirements-covered":["R-01"]}`)
	withReport := identity()
	assert.NotEqual(t, original, withReport)
	writeFile(t, reportPath, `{"requirements-covered":[]}`)
	assert.NotEqual(t, withReport, identity(), "评审中修改canonical done-report也必须失效")
	require.NoError(t, os.Remove(reportPath))
	assert.Equal(t, original, identity(), "manual L1缺少executor报告是明确的空身份")
	require.NoError(t, os.Mkdir(reportPath, 0o755))
	_, err := c.verificationIdentity(p)
	require.Error(t, err, "报告存在但不可读不是允许缺失")
}

type failingSnapshotReader struct{}

func (failingSnapshotReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failingSnapshotReader) Close() error             { return nil }

func TestSnapshotIOFailuresInvalidateGreenEvidence(t *testing.T) {
	for _, fault := range []string{"inventory", "stat", "open", "read", "readlink"} {
		t.Run(fault, func(t *testing.T) {
			fx := newGitFixture(t, "source.txt")
			require.NoError(t, os.Symlink("source.txt", filepath.Join(fx.root, "source.link")))
			c := unitTestContext(t, fx)
			p := c.Packets["M01-T01"]
			seedVerification(t, c, p.PacketID)
			require.True(t, c.currentL1Evidence(p))
			oldFiles, oldStat, oldOpen, oldLink := snapshotGitFiles, snapshotLstat, snapshotOpen, snapshotReadlink
			t.Cleanup(func() {
				snapshotGitFiles, snapshotLstat, snapshotOpen, snapshotReadlink = oldFiles, oldStat, oldOpen, oldLink
			})
			switch fault {
			case "inventory":
				snapshotGitFiles = func(string) ([]byte, error) { return nil, os.ErrPermission }
			case "stat":
				snapshotLstat = func(path string) (os.FileInfo, error) {
					if strings.HasSuffix(path, "source.txt") {
						return nil, os.ErrPermission
					}
					return oldStat(path)
				}
			case "open":
				snapshotOpen = func(string) (io.ReadCloser, error) { return nil, os.ErrPermission }
			case "read":
				snapshotOpen = func(string) (io.ReadCloser, error) { return failingSnapshotReader{}, nil }
			case "readlink":
				snapshotReadlink = func(string) (string, error) { return "", os.ErrPermission }
			}
			_, err := c.verificationIdentity(p)
			require.Error(t, err)
			assert.False(t, c.currentL1Evidence(p), "读取故障不能复用上次的绿灯")
		})
	}
}

func TestGitFailureCannotMasqueradeAsNonGitSnapshot(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	p := c.Packets["M01-T01"]
	seedVerification(t, c, p.PacketID)
	t.Setenv("PATH", t.TempDir())
	_, err := c.candidateFingerprint(fx.root)
	require.ErrorContains(t, err, "Git metadata")
	assert.False(t, c.currentL1Evidence(p))
}

func TestNonGitSnapshotSkipsNestedMetadataAndRejectsUnsupportedFiles(t *testing.T) {
	fx := newFixture(t)
	c := unitTestContext(t, fx)
	base := candidateID(t, c, fx.root)
	writeFile(t, filepath.Join(fx.root, "nested", ".git", "objects", "unrelated"), "metadata")
	assert.Equal(t, base, candidateID(t, c, fx.root))
	_, err := c.candidateFingerprint(filepath.Join(fx.root, "missing"))
	require.Error(t, err)
	require.NoError(t, syscall.Mkfifo(filepath.Join(fx.root, "pipe"), 0o600))
	_, err = c.candidateFingerprint(fx.root)
	require.ErrorContains(t, err, "not a regular file or symlink")
}

func TestEvidenceRequiresMatchingSuccessfulIdentityAndPreservesCredentials(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	p := c.Packets["M01-T01"]
	ps := c.State.Packets[p.PacketID]
	assert.False(t, c.currentL1Evidence(p))
	ps.Evidence.L1 = map[string]any{"pass": true, "identity": 17}
	assert.False(t, c.currentL1Evidence(p))
	seedVerification(t, c, p.PacketID)
	ps.Evidence.L1.(map[string]any)["pass"] = false
	assert.False(t, c.currentL1Evidence(p))
	ps.Evidence.L1.(map[string]any)["pass"] = true
	assert.True(t, c.currentL1Evidence(p))
	writeFile(t, filepath.Join(fx.root, "new.txt"), "changed candidate")
	assert.False(t, c.currentL1Evidence(p))
	require.NoError(t, os.Remove(filepath.Join(fx.root, "new.txt")))
	assert.True(t, c.currentL1Evidence(p))
	require.NoError(t, os.Remove(filepath.Join(fx.taskDir, "spec.md")))
	_, err := c.verificationIdentity(p)
	require.Error(t, err)
	assert.False(t, c.currentL1Evidence(p))
	ps.Evidence.Executor = map[string]any{"old": true}
	ps.Evidence.DefectInjection = map[string]any{"old": true}
	credentials := map[string]any{"injected-env": []string{"EXAMPLE_KEY"}}
	ps.Evidence.Credentials = credentials
	invalidateEvidence(ps)
	assert.Equal(t, state.Evidence{Credentials: credentials}, ps.Evidence)
	assert.Empty(t, evidenceIdentity("legacy evidence"))
}

func TestInvalidationArchivesEvidenceWithoutReusingItsGreenAfterReload(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	p := c.Packets["M01-T01"]
	ps := c.State.Packets[p.PacketID]
	seedVerification(t, c, p.PacketID)
	ps.Evidence.Executor = map[string]any{"report": "verified done report"}
	ps.Evidence.DefectInjection = map[string]any{"outcome": "pass"}
	ps.Evidence.Credentials = map[string]any{"injected-env": []string{"EXAMPLE_KEY"}}
	prior := ps.Evidence
	require.True(t, c.currentL1Evidence(p))
	invalidateEvidence(ps)
	require.Len(t, ps.EvidenceHistory, 1)
	entry := ps.EvidenceHistory[0].(map[string]any)
	_, err := time.Parse(time.RFC3339Nano, entry["at"].(string))
	require.NoError(t, err)
	assert.Equal(t, prior, entry["evidence"], "保留完整上一轮证据快照供审计")
	assert.Equal(t, state.Evidence{Credentials: prior.Credentials}, ps.Evidence)
	assert.False(t, c.currentL1Evidence(p), "候选未变也不能把失效历史当现行绿灯")
	invalidateEvidence(ps)
	assert.Len(t, ps.EvidenceHistory, 1, "只有credentials或空证据不重复归档")
	_, err = state.Save(fx.taskDir, c.State, nil)
	require.NoError(t, err)
	c.State, err = state.Load(fx.taskDir)
	require.NoError(t, err)
	ps = c.State.Packets[p.PacketID]
	require.Len(t, ps.EvidenceHistory, 1)
	archived := ps.EvidenceHistory[0].(map[string]any)["evidence"].(map[string]any)
	assert.Equal(t, evidenceIdentity(prior.L1), evidenceIdentity(archived["l1"]))
	assert.Nil(t, ps.Evidence.L1)
	assert.NotNil(t, ps.Evidence.Credentials)
	assert.False(t, c.currentL1Evidence(p), "恢复运行只能读取现行证据，历史即使匹配也不能放行")
}
