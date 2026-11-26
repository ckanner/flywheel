package machine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func commitSafetyFixture(t *testing.T, worktree bool) (fixture, *Context, *contract.Packet) {
	t.Helper()
	fx := newGitFixture(t, "source.txt")
	c := unitTestContext(t, fx)
	p := c.Packets["M01-T01"]
	ps := c.State.Packets[p.PacketID]
	base, ok := verify.GetBaseRef(fx.root)
	require.True(t, ok)
	ps.BaseRef = &base
	ps.Status = "l1-passed"
	if worktree {
		wi := verify.EnsureUnitWorktree(fx.root, fx.slug, p.PacketID, base)
		require.Equal(t, "worktree", wi.Mode)
		ps.Worktree = &wi
	}
	writeFile(t, filepath.Join(c.unitCwd(p), "source.txt"), "verified candidate\n")
	writeFile(t, filepath.Join(fx.taskDir, "reports", p.PacketID+".done.json"), `{"verified":true}`)
	seedVerification(t, c, p.PacketID)
	return fx, c, p
}

func installCommitHook(t *testing.T, fx fixture, body string) {
	t.Helper()
	path := filepath.Join(fx.root, ".git", "hooks", "pre-commit")
	writeFile(t, path, "#!/bin/sh\nset -eu\n"+body+"\n")
	require.NoError(t, os.Chmod(path, 0o755))
}

func assertCommitRefused(t *testing.T, c *Context, p *contract.Packet, stage string) {
	t.Helper()
	err := c.completeUnit(p)
	require.ErrorIs(t, err, ErrCommitFailed)
	ps := c.State.Packets[p.PacketID]
	assert.NotEqual(t, "completed", ps.Status)
	rec := ps.Commit.(map[string]any)
	assert.Equal(t, stage, rec["stage"])
	assert.NotEmpty(t, rec["error"])
	loaded, err := state.Load(c.TaskDir)
	require.NoError(t, err)
	assert.NotEqual(t, "completed", loaded.Packets[p.PacketID].Status)
	assert.Equal(t, stage, loaded.Packets[p.PacketID].Commit.(map[string]any)["stage"])
	if ps.Worktree != nil {
		assert.DirExists(t, ps.Worktree.Path, "失败后保留有已验证实现的worktree")
	}
}

func TestCommitRejectsIndexOnlyHookEvenWhenWorkingCandidateStillVerified(t *testing.T) {
	for _, worktree := range []bool{true, false} {
		t.Run(fmt.Sprint(worktree), func(t *testing.T) {
			fx, c, p := commitSafetyFixture(t, worktree)
			cwd := c.unitCwd(p)
			installCommitHook(t, fx, "blob=$(printf 'unverified index blob\\n' | git hash-object -w --stdin)\ngit update-index --add --cacheinfo 100644 \"$blob\" source.txt")
			assertCommitRefused(t, c, p, "code-tree")
			assert.Equal(t, "unverified index blob\n", gitCmd(t, cwd, "show", "HEAD:source.txt"))
			actual, err := os.ReadFile(filepath.Join(cwd, "source.txt"))
			require.NoError(t, err)
			assert.Equal(t, "verified candidate\n", string(actual))
			assert.True(t, c.currentL1Evidence(p), "漏洞关键：只检查工作区identity仍然会绿")
			if worktree {
				assert.Equal(t, "// source.txt\n", gitCmd(t, fx.root, "show", "HEAD:source.txt"), "错误blob不能合并入主分支")
			}
		})
	}
}

func TestCommitRejectsHookThatChangesAndPersistsWorkingCandidate(t *testing.T) {
	for _, worktree := range []bool{true, false} {
		t.Run(fmt.Sprint(worktree), func(t *testing.T) {
			fx, c, p := commitSafetyFixture(t, worktree)
			installCommitHook(t, fx, "printf 'changed candidate\\n' > source.txt\ngit add -- source.txt")
			assertCommitRefused(t, c, p, "code-evidence")
			assert.Equal(t, "changed candidate\n", gitCmd(t, c.unitCwd(p), "show", "HEAD:source.txt"))
			assert.False(t, c.currentL1Evidence(p))
		})
	}
}

func TestCommitRejectsUnpersistedExecutableModeWhenGitIgnoresMode(t *testing.T) {
	for _, worktree := range []bool{true, false} {
		t.Run(fmt.Sprint(worktree), func(t *testing.T) {
			fx, c, p := commitSafetyFixture(t, worktree)
			gitCmd(t, fx.root, "config", "core.filemode", "false")
			cwd := c.unitCwd(p)
			require.NoError(t, os.Chmod(filepath.Join(cwd, "source.txt"), 0o755))
			seedVerification(t, c, p.PacketID)
			assertCommitRefused(t, c, p, "code-tree")
			assert.True(t, c.currentL1Evidence(p), "工作区身份仍绿，但执行位未入库，不能completed")
			assert.Contains(t, gitCmd(t, cwd, "ls-tree", "HEAD", "source.txt"), "100644")
			assert.Equal(t, "false", strings.TrimSpace(gitCmd(t, fx.root, "config", "--get", "core.filemode")))
		})
	}
}

func TestLedgerCommitCannotChangeMainCodeOrCanonicalReport(t *testing.T) {
	for _, scenario := range []struct {
		name, body, stage string
	}{
		{"index-only", "blob=$(printf 'ledger index attack\\n' | git hash-object -w --stdin)\ngit update-index --add --cacheinfo 100644 \"$blob\" source.txt", "main-tree"},
		{"worktree-only", "printf 'ledger worktree attack\\n' > source.txt", "main-tree"},
		{"durable-code-change", "printf 'ledger durable attack\\n' > source.txt\ngit add -- source.txt", "main-candidate"},
		{"canonical-report", "printf '{\"verified\":false}' > docs/minitask/reports/M01-T01.done.json", "final-evidence"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fx, c, p := commitSafetyFixture(t, true)
			wi := *c.State.Packets[p.PacketID].Worktree
			installCommitHook(t, fx, fmt.Sprintf("if [ \"$(git rev-parse --show-toplevel)\" = %q ]; then\n%s\nfi", fx.root, scenario.body))
			assertCommitRefused(t, c, p, scenario.stage)
			assert.Equal(t, "verified candidate\n", gitCmd(t, wi.Path, "show", "HEAD:source.txt"), "保留正确分支供恢复")
			rec := c.State.Packets[p.PacketID].Commit.(map[string]any)
			assert.NotEmpty(t, rec["sha"])
			assert.NotEmpty(t, rec["ledger-sha"])
		})
	}
}

func TestLedgerCommitFailureKeepsMergedWorktreeForSafeRetry(t *testing.T) {
	fx, c, p := commitSafetyFixture(t, true)
	installCommitHook(t, fx, fmt.Sprintf("if [ \"$(git rev-parse --show-toplevel)\" = %q ]; then exit 1; fi", fx.root))
	assertCommitRefused(t, c, p, "ledger-commit")
	assert.NotEmpty(t, c.State.Packets[p.PacketID].Commit.(map[string]any)["ledger-error"])
	assert.Equal(t, "verified candidate\n", gitCmd(t, fx.root, "show", "HEAD:source.txt"))
	require.NoError(t, os.Remove(filepath.Join(fx.root, ".git", "hooks", "pre-commit")))
	require.NoError(t, c.completeUnit(p))
	assert.Equal(t, "completed", c.State.Packets[p.PacketID].Status)
	assert.Nil(t, c.State.Packets[p.PacketID].Worktree)
}

func TestCompletedWithCleanupWarningKeepsWorktreeReferenceAndDurableCode(t *testing.T) {
	fx, c, p := commitSafetyFixture(t, true)
	ps := c.State.Packets[p.PacketID]
	wi := *ps.Worktree
	// 无法安全清理的额外账本不是源码变化，也不允许force删除。
	writeFile(t, filepath.Join(wi.Path, "docs", fx.slug, "reports", "orphan.done.json"), "retain this recovery artifact")
	require.NoError(t, c.completeUnit(p))
	assert.Equal(t, "completed", ps.Status, "代码及最终tree已确认持久化，清理是独立警告")
	assert.Equal(t, &wi, ps.Worktree)
	assert.DirExists(t, wi.Path)
	assert.FileExists(t, filepath.Join(wi.Path, "docs", fx.slug, "reports", "orphan.done.json"))
	rec := ps.Commit.(map[string]any)
	assert.NotEmpty(t, rec["cleanup-error"])
	assert.Equal(t, strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")), rec["sha"])
	assert.Equal(t, "verified candidate\n", gitCmd(t, fx.root, "show", "HEAD:source.txt"))
	gitCmd(t, fx.root, "merge-base", "--is-ancestor", wi.Branch, "HEAD")
	loaded := loadFixtureState(t, fx).Packets[p.PacketID]
	assert.Equal(t, "completed", loaded.Status)
	assert.Equal(t, &wi, loaded.Worktree)
	assert.NotEmpty(t, loaded.Commit.(map[string]any)["cleanup-error"])
}

func TestNonGitCannotCompleteEvenWithMatchingGreenEvidence(t *testing.T) {
	fx := newFixture(t)
	c := unitTestContext(t, fx)
	p := c.Packets["M01-T01"]
	seedVerification(t, c, p.PacketID)
	assertCommitRefused(t, c, p, "code-tree")
	assert.Equal(t, "not a git checkout", c.State.Packets[p.PacketID].Commit.(map[string]any)["skipped"])
}

func TestCompletionRejectsCandidateInventoryFailureBeforeOrAfterMerge(t *testing.T) {
	for _, stage := range []string{"candidate", "main-candidate"} {
		t.Run(stage, func(t *testing.T) {
			fx, c, p := commitSafetyFixture(t, true)
			original := snapshotGitFiles
			t.Cleanup(func() { snapshotGitFiles = original })
			calls := 0
			snapshotGitFiles = func(cwd string) ([]byte, error) {
				calls++
				if stage == "candidate" && calls == 2 || stage == "main-candidate" && cwd == fx.root {
					return nil, errors.New("candidate inventory unreadable")
				}
				return original(cwd)
			}
			assertCommitRefused(t, c, p, stage)
		})
	}
}

func TestCompletionRequiresCurrentPassAndStopsOnPriorFatalError(t *testing.T) {
	for _, scenario := range []string{"stale-l1", "review-return", "stale-review", "fatal"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, p := commitSafetyFixture(t, true)
			ps := c.State.Packets[p.PacketID]
			fatal := errors.New("previous state persistence failure")
			switch scenario {
			case "stale-l1":
				writeFile(t, filepath.Join(c.unitCwd(p), "source.txt"), "changed after review")
			case "review-return":
				ps.Evidence.Review.(map[string]any)["verdict"] = "return"
			case "stale-review":
				ps.Evidence.Review.(map[string]any)["identity"] = "old-candidate"
			case "fatal":
				c.fatal = fatal
			}
			err := c.completeUnit(p)
			if scenario == "fatal" {
				require.ErrorIs(t, err, fatal)
			} else {
				require.ErrorIs(t, err, ErrCommitFailed)
			}
			assert.NotEqual(t, "completed", ps.Status)
			assert.DirExists(t, ps.Worktree.Path)
			assert.Equal(t, "// source.txt\n", gitCmd(t, fx.root, "show", "HEAD:source.txt"))
		})
	}
}

func TestNoChangeInPlaceCompletionRecordsExistingDurableHead(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	p := c.Packets["M01-T01"]
	seedVerification(t, c, p.PacketID)
	head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
	require.NoError(t, c.completeUnit(p))
	ps := c.State.Packets[p.PacketID]
	assert.Equal(t, "completed", ps.Status)
	assert.Equal(t, head, ps.Commit.(map[string]any)["sha"])
	assert.Equal(t, "no changes", ps.Commit.(map[string]any)["skipped"])
}

func TestCollectedTrackedReportRestoresHistoricalWorktreeFileAndMode(t *testing.T) {
	fx := newGitFixture(t)
	rel := "reports/M01-T01.done.json"
	prior := `{"requirements-covered":[],"changed-files":[],"commands-run":[],"failures":[]}`
	writeFile(t, filepath.Join(fx.taskDir, rel), prior)
	require.NoError(t, os.Chmod(filepath.Join(fx.taskDir, rel), 0o755))
	gitCmd(t, fx.root, "add", "--", "docs/minitask/"+rel)
	gitCmd(t, fx.root, "commit", "-m", "prior report for repeated execution")
	c := unitTestContext(t, fx)
	p := c.Packets["M01-T01"]
	result, err := c.executeStep(p)
	require.NoError(t, err)
	require.True(t, result.Success)
	wt := c.unitCwd(p)
	require.NotEqual(t, fx.root, wt)
	localPath := filepath.Join(wt, "docs", fx.slug, rel)
	raw, err := os.ReadFile(localPath)
	require.NoError(t, err)
	assert.Equal(t, prior, string(raw))
	info, err := os.Stat(localPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	canonical, err := os.ReadFile(filepath.Join(fx.taskDir, rel))
	require.NoError(t, err)
	assert.Contains(t, string(canonical), "build/M01-T01.txt", "主目录收到的是本轮新报告")
	assert.Empty(t, strings.TrimSpace(gitCmd(t, wt, "diff", "HEAD", "--", "docs/minitask/"+rel)), "收集不留下tracked报告删除")
	assert.FileExists(t, filepath.Join(wt, "build", "M01-T01.txt"), "恢复报告不能清理实现")
}

func TestReportRestoreFailurePreservesFreshReportAndRefusesExecutorSuccess(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		r := sd.mock.Execute(a)
		require.NoError(t, os.Remove(filepath.Join(a.Cwd, ".git")), "模拟报告收集期间Git元数据丢失")
		return r
	}
	c.Drivers = sd.set()
	p := c.Packets["M01-T01"]
	r, err := c.executeStep(p)
	require.ErrorContains(t, err, "inspect prior report")
	assert.False(t, r.Success)
	ps := c.State.Packets[p.PacketID]
	assert.NotEqual(t, "executed", ps.Status)
	assert.Nil(t, ps.Evidence.Executor)
	assert.FileExists(t, filepath.Join(fx.taskDir, "reports", p.PacketID+".done.json"))
	assert.FileExists(t, filepath.Join(ps.Worktree.Path, "docs", fx.slug, "reports", p.PacketID+".done.json"))
}

func TestRepeatedDeliveryWithTrackedReportsCleansWorktree(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(fmt.Sprint(manual), func(t *testing.T) {
			fx := newGitFixture(t)
			first, err := RunTask(runOpts(fx, happyDrivers().set()))
			require.NoError(t, err)
			require.Equal(t, "completed", first.Status)
			if manual {
				opts := stepOpts(fx, happyDrivers().set())
				opts.Only, opts.PacketID = "execute", "M01-T01"
				opts.Cwd = fx.taskDir // CLI可从任务子目录进入，最终仍使用Git根工作区。
				step, err := RunSingleStep(opts)
				require.NoError(t, err)
				require.True(t, step.Step.OK)
			} else {
				appendPacketRevision(t, fx, "M01-T01")
			}
			result, err := RunTask(runOpts(fx, happyDrivers().set()))
			require.NoError(t, err)
			require.Equal(t, "completed", result.Status)
			ps := loadFixtureState(t, fx).Packets["M01-T01"]
			assert.Equal(t, 2, ps.Attempts)
			assert.Nil(t, ps.Worktree)
			assert.Nil(t, ps.Commit.(map[string]any)["cleanup-error"])
			assert.NotContains(t, gitCmd(t, fx.root, "worktree", "list", "--porcelain"), ".flywheel-worktrees")
		})
	}
}

func TestCompleteUnitIgnoresSiblingFlywheelTaskLedgersButStillRejectsNonLedgerFiles(t *testing.T) {
	fx, c, p := commitSafetyFixture(t, true)
	// Seed a sibling Flywheel task directory with untracked ledger artifacts.
	writeFile(t, filepath.Join(fx.root, "docs", "older-task", "state.json"), `{"slug":"older-task"}`)
	writeFile(t, filepath.Join(fx.root, "docs", "older-task", ".traces", "live-1.jsonl"), "{}\n")
	writeFile(t, filepath.Join(fx.root, "docs", "older-task", "metrics.jsonl"), "{}\n")
	require.NoError(t, c.completeUnit(p))
	assert.Equal(t, "completed", c.State.Packets[p.PacketID].Status)

	// Non-directory entries and non-task subdirectories under docs/ are ignored,
	// and a missing parent directory returns nil cleanly.
	writeFile(t, filepath.Join(fx.root, "docs", "README.md"), "docs index")
	writeFile(t, filepath.Join(fx.root, "docs", "notes", "design.txt"), "not a flywheel task")
	assert.Equal(t, []string{"docs/older-task/"}, c.siblingTaskPrefixes())
	missingCtx := &Context{Cwd: fx.root, TaskDir: filepath.Join(fx.root, "no-such-parent", "task")}
	assert.Nil(t, missingCtx.siblingTaskPrefixes())
}
