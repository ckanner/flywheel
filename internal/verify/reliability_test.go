package verify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGateTimeoutNeverMatchesExpectedExit(t *testing.T) {
	for _, exit := range []int{0, 1} {
		t.Run(fmt.Sprint(exit), func(t *testing.T) {
			r := RunGateCommands([]contract.GateEntry{
				gateEntry(fmt.Sprintf(`trap 'exit %d' TERM; while :; do :; done`, exit), fmt.Sprintf("exit %d", exit)),
			}, GateOpts{Cwd: t.TempDir(), TimeoutMS: 100, KillGraceMS: 100})
			require.Len(t, r.Results, 1)
			assert.False(t, r.Pass)
			assert.True(t, r.Results[0].TimedOut)
			assert.False(t, r.Results[0].ObservationComplete)
			assert.Empty(t, r.Results[0].OutputDigest)
			assert.Equal(t, exit, r.Results[0].ExitCode, "即使trap恰好返回预期退出码，超时仍失败")
		})
	}
}

func TestGateTruncatedOutputNeverMatchesExpectedExit(t *testing.T) {
	originalLimit := watchdog.MaxBuffer
	watchdog.MaxBuffer = 8
	t.Cleanup(func() { watchdog.MaxBuffer = originalLimit })
	for _, stream := range []string{"stdout", "stderr"} {
		for _, exit := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/exit-%d", stream, exit), func(t *testing.T) {
				redirect := ""
				if stream == "stderr" {
					redirect = " >&2"
				}
				r := RunGateCommands([]contract.GateEntry{
					gateEntry(fmt.Sprintf("printf 123456789abcdef%s; exit %d", redirect, exit), fmt.Sprintf("exit %d", exit)),
				}, GateOpts{Cwd: t.TempDir()})
				require.Len(t, r.Results, 1)
				assert.False(t, r.Pass, "正常退出不能代替完整的验收证据")
				assert.False(t, r.Results[0].Pass)
				assert.True(t, r.Results[0].Truncated)
				assert.False(t, r.Results[0].ObservationComplete)
				assert.False(t, r.Results[0].OutputTruncated, "physical loss and a compact summary are separate facts")
				assert.Empty(t, r.Results[0].OutputDigest)
				assert.Equal(t, exit, r.Results[0].ExitCode)
				assert.Contains(t, r.Results[0].Output, "12345678")
				assert.Contains(t, r.Results[0].Output, "command output truncated")
			})
		}
	}
}

func TestGateSpawnAndCleanupFailureNeverPass(t *testing.T) {
	r := RunGateCommands([]contract.GateEntry{gateEntry("true", "exit 1")},
		GateOpts{Cwd: filepath.Join(t.TempDir(), "missing")})
	assert.False(t, r.Pass)
	assert.NotEmpty(t, r.Results[0].SpawnError)
	assert.False(t, r.Results[0].ObservationComplete)
	assert.Empty(t, r.Results[0].OutputDigest)
	assert.Equal(t, -1, r.Results[0].ExitCode)
	assert.NotEmpty(t, r.Results[0].Output)

	r = RunGateCommands([]contract.GateEntry{gateEntry("sleep 30 & exit 0", "exit 0")},
		GateOpts{Cwd: t.TempDir(), TimeoutMS: 30_000, KillGraceMS: 80})
	assert.False(t, r.Pass)
	assert.NotEmpty(t, r.Results[0].CleanupError)
	assert.False(t, r.Results[0].ObservationComplete)
	assert.Empty(t, r.Results[0].OutputDigest)
	assert.Contains(t, r.Results[0].Output, "output did not close")

	r = RunGateCommands([]contract.GateEntry{gateEntry("sleep 30 & exit 1", "exit 1")},
		GateOpts{Cwd: t.TempDir(), TimeoutMS: 30_000, KillGraceMS: 80})
	assert.False(t, r.Pass, "非零退出不能掩盖WaitDelay截断排空")
	assert.Equal(t, 1, r.Results[0].ExitCode)
	assert.NotEmpty(t, r.Results[0].CleanupError)
}

func TestGateCancelledContextDoesNotStartCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	marker := filepath.Join(t.TempDir(), "should-not-exist")
	r := RunGateCommands([]contract.GateEntry{gateEntry(fmt.Sprintf("touch %q", marker), "exit 0")},
		GateOpts{Context: ctx})
	assert.False(t, r.Pass)
	assert.True(t, r.Results[0].Cancelled)
	assert.False(t, r.Results[0].ObservationComplete)
	assert.Empty(t, r.Results[0].OutputDigest)
	assert.NoFileExists(t, marker)
}

func TestGateKillSwitchStopsDescendantsAndRemainingCommands(t *testing.T) {
	dir := t.TempDir()
	marker, next := filepath.Join(dir, "writes"), filepath.Join(dir, "next")
	script := fmt.Sprintf(`trap 'exit 0' TERM
(trap '' TERM; while :; do printf x >> %q; sleep 0.01; done) >/dev/null 2>&1 &
wait`, marker)
	done := make(chan GateReport, 1)
	t.Cleanup(func() { watchdog.KillActiveChildren(20 * time.Millisecond) })
	go func() {
		done <- RunGateCommands([]contract.GateEntry{
			gateEntry(script, "exit 0"),
			gateEntry(fmt.Sprintf("touch %q", next), "exit 0"),
		}, GateOpts{Cwd: dir, TimeoutMS: 30_000, KillGraceMS: 100})
	}()
	require.Eventually(t, func() bool {
		data, _ := os.ReadFile(marker)
		return len(data) > 0
	}, 3*time.Second, 10*time.Millisecond)
	started := time.Now()
	watchdog.KillActiveChildren(50 * time.Millisecond)
	var r GateReport
	select {
	case r = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled gate did not reach quiescence")
	}
	assert.Less(t, time.Since(started), 3*time.Second)
	require.Len(t, r.Results, 2)
	assert.False(t, r.Pass)
	assert.True(t, r.Results[0].Cancelled)
	assert.Equal(t, 0, r.Results[0].ExitCode, "取消原因不能被trap exit 0覆盖")
	assert.True(t, r.Results[1].Cancelled)
	assert.False(t, r.Results[0].ObservationComplete)
	assert.False(t, r.Results[1].ObservationComplete)
	assert.Empty(t, r.Results[0].OutputDigest)
	assert.Empty(t, r.Results[1].OutputDigest)
	assert.NoFileExists(t, next)
	before, err := os.ReadFile(marker)
	require.NoError(t, err)
	time.Sleep(150 * time.Millisecond)
	after, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, before, after, "取消返回后不得有遗留孙进程写入")
}

func TestBoundariesAllowRegressionTestsAndProtectAcceptance(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, filepath.Join(repo, "acceptance", "contract_test.go"), "frozen oracle\n")
	gitMust(t, repo, "add", ".")
	gitMust(t, repo, "commit", "-m", "acceptance contract")
	base, ok := GetBaseRef(repo)
	require.True(t, ok)
	writeFile(t, filepath.Join(repo, "tests", "parser_test.go"), "unit test\n")
	writeFile(t, filepath.Join(repo, "src", "parser.spec.ts"), "regression test\n")
	changed, ok := GetChangedFiles(repo, base)
	require.True(t, ok)
	bounds := contract.Boundaries{MustNotTouch: []string{"acceptance/"}}
	assert.True(t, CheckBoundaries(changed, bounds, nil).Pass)
	assert.True(t, CheckBoundaries(changed, bounds, []string{}).Pass)
	writeFile(t, filepath.Join(repo, "acceptance", "contract_test.go"), "weaker oracle\n")
	changed, ok = GetChangedFiles(repo, base)
	require.True(t, ok)
	r := CheckBoundaries(changed, bounds, nil)
	assert.False(t, r.Pass)
	assert.Contains(t, r.Violations, BoundaryViolation{Type: "must-not-touch", Path: "acceptance/contract_test.go"})
	r = CheckBoundaries(changed, contract.Boundaries{}, []string{`^acceptance/`})
	assert.False(t, r.Pass)
	assert.Contains(t, r.Violations, BoundaryViolation{Type: "test-file", Path: "acceptance/contract_test.go"})
	r = CheckBoundaries(changed, contract.Boundaries{}, []string{"["})
	assert.False(t, r.Pass)
	assert.Contains(t, r.Reason, "invalid protected test pattern")
	assert.False(t, CheckBoundaries(nil, contract.Boundaries{}, []string{"["}).Pass,
		"无diff也不能让非法保护配置静默放行")
	r = CheckBoundaries([]string{"?private.txt"}, contract.Boundaries{MustNotTouch: []string{"?*"}}, nil)
	assert.False(t, r.Pass, "路径中的问号必须按字面量处理，不能构造非法正则")
}

func TestRemoveWorktreePreservesDirtyWorkAndIsIdempotent(t *testing.T) {
	repo := initRepo(t)
	base, _ := GetBaseRef(repo)
	wi := EnsureUnitWorktree(repo, "cleanup", "M01-T01", base)
	require.Equal(t, "worktree", wi.Mode)
	file := filepath.Join(wi.Path, "unfinished.txt")
	writeFile(t, file, "uncommitted implementation\n")
	err := RemoveUnitWorktree(repo, wi.Branch, wi.Path)
	require.Error(t, err)
	assert.FileExists(t, file)
	assert.NotEmpty(t, gitMust(t, repo, "show-ref", "--verify", "refs/heads/"+wi.Branch))
	gitMust(t, wi.Path, "add", ".")
	gitMust(t, wi.Path, "commit", "-m", "save implementation")
	require.Error(t, RemoveUnitWorktree(repo, wi.Branch, wi.Path), "持久提交但未合并仍保留")
	_, err = MergeUnitWorktree(repo, wi.Branch)
	require.NoError(t, err)
	require.NoError(t, RemoveUnitWorktree(repo, wi.Branch, wi.Path))
	require.NoError(t, RemoveUnitWorktree(repo, wi.Branch, wi.Path), "恢复可重复清理")
	assert.NoDirExists(t, wi.Path)
	assert.Equal(t, "uncommitted implementation\n", gitMust(t, repo, "show", "HEAD:unfinished.txt"))
}

func TestRemoveWorktreeRetainsBranchWhenDeletionFails(t *testing.T) {
	repo := initRepo(t)
	base, _ := GetBaseRef(repo)
	wi := EnsureUnitWorktree(repo, "cleanup", "M01-T01", base)
	lock := filepath.Join(repo, ".git", "refs", "heads", wi.Branch+".lock")
	writeFile(t, lock, "concurrent git lock")
	err := RemoveUnitWorktree(repo, wi.Branch, wi.Path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete unit branch")
	assert.NoDirExists(t, wi.Path)
	assert.NotEmpty(t, gitMust(t, repo, "show-ref", "--verify", "refs/heads/"+wi.Branch))
	require.NoError(t, os.Remove(lock))
	require.NoError(t, RemoveUnitWorktree(repo, wi.Branch, wi.Path))
}

func TestRemoveWorktreeFailsClosedOnUnverifiableOwnership(t *testing.T) {
	repo := initRepo(t)
	base, _ := GetBaseRef(repo)
	wi := EnsureUnitWorktree(repo, "cleanup", "M01-T01", base)
	gitMust(t, repo, "update-ref", "-d", "refs/heads/"+wi.Branch)
	require.ErrorContains(t, RemoveUnitWorktree(repo, wi.Branch, wi.Path), "unit branch")
	assert.DirExists(t, wi.Path)
	gitMust(t, repo, "update-ref", "refs/heads/"+wi.Branch, base)
	require.NoError(t, RemoveUnitWorktree(repo, wi.Branch, wi.Path))
	unregistered := t.TempDir()
	assert.ErrorContains(t, RemoveUnitWorktree(repo, wi.Branch, unregistered), "unregistered")

	original := gitOut
	t.Cleanup(func() { gitOut = original })
	for _, fail := range []string{"for-each-ref", "worktree"} {
		t.Run(fail, func(t *testing.T) {
			gitOut = func(cwd string, args ...string) (string, error) {
				if args[0] == fail {
					return "", fmt.Errorf("cannot inspect repository")
				}
				return original(cwd, args...)
			}
			assert.ErrorContains(t, RemoveUnitWorktree(repo, wi.Branch, wi.Path), "cannot inspect repository")
		})
	}
}

func TestRemoveWorktreeRefusesDetachedUnmergedWork(t *testing.T) {
	repo := initRepo(t)
	base, _ := GetBaseRef(repo)
	wi := EnsureUnitWorktree(repo, "detached", "M01-T01", base)
	gitMust(t, wi.Path, "checkout", "--detach")
	writeFile(t, filepath.Join(wi.Path, "detached.txt"), "unique candidate")
	gitMust(t, wi.Path, "add", ".")
	gitMust(t, wi.Path, "commit", "-m", "detached work")
	assert.ErrorContains(t, RemoveUnitWorktree(repo, wi.Branch, wi.Path), "no longer bound")
	assert.FileExists(t, filepath.Join(wi.Path, "detached.txt"), "已合并的旧分支不能证明另一个候选安全可删")
}

func TestPersistedHeadRequiresMatchingTreeAndIncludesUntrackedPaths(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, filepath.Join(repo, "source.txt"), "verified source\n")
	writeFile(t, filepath.Join(repo, "ledger[1]", "state.json"), "old state")
	gitMust(t, repo, "add", ".")
	gitMust(t, repo, "commit", "-m", "verified candidate")
	head := strings.TrimSpace(gitMust(t, repo, "rev-parse", "HEAD"))
	writeFile(t, filepath.Join(repo, "ledger[1]", "state.json"), "new state")
	writeFile(t, filepath.Join(repo, "ledger[1]", "reports", "done.json"), "runtime report")
	excludes := []string{"ledger[1]/state.json", "ledger[1]/reports/"}
	got, err := PersistedHead(repo, excludes)
	require.NoError(t, err)
	assert.Equal(t, head, got, "账本排除按字面路径处理，包括目录及pathspec特殊字符")

	writeFile(t, filepath.Join(repo, "source.txt"), "uncommitted candidate\n")
	_, err = PersistedHead(repo, excludes)
	require.ErrorContains(t, err, "differs from verified candidate")
	gitMust(t, repo, "checkout", "--", "source.txt")
	untracked := "new file\nwith newline.txt"
	writeFile(t, filepath.Join(repo, untracked), "unpersisted implementation")
	_, err = PersistedHead(repo, excludes)
	require.ErrorContains(t, err, "files were not persisted")
	assert.Contains(t, err.Error(), `new file\nwith newline.txt`)
}

func TestPersistedHeadFailsClosedOnRepositoryOrInventoryFailure(t *testing.T) {
	_, err := PersistedHead(t.TempDir(), nil)
	require.ErrorContains(t, err, "persisted HEAD commit")
	repo := initRepo(t)
	original := gitOut
	t.Cleanup(func() { gitOut = original })
	for _, command := range []string{"diff", "ls-files"} {
		t.Run(command, func(t *testing.T) {
			gitOut = func(cwd string, args ...string) (string, error) {
				if command == "diff" && slices.Contains(args, "diff") || args[0] == command && args[1] == "--others" {
					return "", fmt.Errorf("repository scan failed")
				}
				return original(cwd, args...)
			}
			_, err := PersistedHead(repo, nil)
			require.ErrorContains(t, err, "repository scan failed")
		})
	}
}

func TestScopedCommitsPreserveUnselectedPartialStaging(t *testing.T) {
	for _, taskDir := range []bool{false, true} {
		t.Run(fmt.Sprint(taskDir), func(t *testing.T) {
			repo := initRepo(t)
			outside := filepath.Join(repo, "user.txt")
			selected := filepath.Join(repo, "docs", "task", "spec.md")
			writeFile(t, outside, "base\n")
			writeFile(t, selected, "base spec\n")
			gitMust(t, repo, "add", ".")
			gitMust(t, repo, "commit", "-m", "baseline")
			base, _ := GetBaseRef(repo)
			writeFile(t, outside, "user staged\n")
			gitMust(t, repo, "add", "user.txt")
			writeFile(t, outside, "user staged\nuser unstaged\n")
			beforeIndex := gitMust(t, repo, "diff", "--cached", "--", "user.txt")
			beforeWorktree := gitMust(t, repo, "diff", "--", "user.txt")
			writeFile(t, selected, "new spec\n")
			var out CommitOutcome
			if taskDir {
				out = CommitTaskDirChanges(repo, "docs/task/", "docs: update contract", nil, regexp.MustCompile(`^spec\.md$`))
			} else {
				out = CommitUnitChanges(repo, &contract.Packet{PacketID: "M01-T01", Goal: "update contract"}, nil, base, "docs/task/")
			}
			require.Empty(t, out.Error)
			require.NotEmpty(t, out.SHA)
			assert.Equal(t, "base\n", gitMust(t, repo, "show", "HEAD:user.txt"), "其它staged文件不得混入提交")
			assert.Equal(t, beforeIndex, gitMust(t, repo, "diff", "--cached", "--", "user.txt"))
			assert.Equal(t, beforeWorktree, gitMust(t, repo, "diff", "--", "user.txt"))
			assert.Equal(t, "new spec\n", gitMust(t, repo, "show", "HEAD:docs/task/spec.md"))
		})
	}
}

func TestScopedCommitsExcludeLedgerDirectoriesAndExactFiles(t *testing.T) {
	for _, taskDir := range []bool{false, true} {
		t.Run(fmt.Sprint(taskDir), func(t *testing.T) {
			repo := initRepo(t)
			prefix := "docs/task/"
			writeFile(t, filepath.Join(repo, prefix, "spec.md"), "spec")
			writeFile(t, filepath.Join(repo, prefix, "reports", "unit.done.json"), "report sidecar")
			writeFile(t, filepath.Join(repo, prefix, "state.json"), "untrusted state")
			gitMust(t, repo, "add", prefix+"reports/", prefix+"state.json")
			before := gitMust(t, repo, "diff", "--cached")
			excludes := []string{prefix + "reports/", prefix + "state.json"}
			var out CommitOutcome
			if taskDir {
				out = CommitTaskDirChanges(repo, prefix, "docs: spec", excludes, nil)
			} else {
				out = CommitUnitChanges(repo, &contract.Packet{PacketID: "M01-T01", Goal: "spec"}, excludes, "", "")
			}
			require.Empty(t, out.Error)
			require.NotEmpty(t, out.SHA)
			assert.Equal(t, prefix+"spec.md\n", gitMust(t, repo, "ls-tree", "--name-only", "-r", "HEAD", "--", prefix))
			assert.Equal(t, before, gitMust(t, repo, "diff", "--cached"), "排除的账本及原有index不得混入实现提交")
		})
	}
}

func TestMergeFailureIncludesConflictingPath(t *testing.T) {
	repo := initRepo(t)
	base, _ := GetBaseRef(repo)
	wi := EnsureUnitWorktree(repo, "conflict", "M01-T01", base)
	writeFile(t, filepath.Join(wi.Path, "conflict.txt"), "candidate")
	gitMust(t, wi.Path, "add", ".")
	gitMust(t, wi.Path, "commit", "-m", "candidate")
	writeFile(t, filepath.Join(repo, "conflict.txt"), "user work")
	_, err := MergeUnitWorktree(repo, wi.Branch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict.txt")
	assert.Contains(t, err.Error(), "untracked")
	assert.DirExists(t, wi.Path)
}

func TestCommitFailurePreservesSelectedPartialIndexAndImplementation(t *testing.T) {
	repo := initRepo(t)
	pkt := &contract.Packet{PacketID: "M01-T01", Goal: "implementation"}
	file := filepath.Join(repo, "README.md")
	writeFile(t, file, "staged content\n")
	gitMust(t, repo, "add", "README.md")
	writeFile(t, file, "working implementation\n")
	before := gitMust(t, repo, "diff", "--cached")
	hook := filepath.Join(repo, ".git", "hooks", "pre-commit")
	writeFile(t, hook, "#!/bin/sh\nexit 1\n")
	require.NoError(t, os.Chmod(hook, 0o755))
	out := CommitUnitChanges(repo, pkt, nil, "", "")
	require.NotEmpty(t, out.Error)
	assert.Equal(t, before, gitMust(t, repo, "diff", "--cached"), "失败不能覆写已有partial index")
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "working implementation\n", string(data))
}

func TestCommitUsesOnlyRemainingWorkAfterExecutorCommit(t *testing.T) {
	repo := initRepo(t)
	pkt := &contract.Packet{PacketID: "M01-T01", Goal: "implementation"}
	base, _ := GetBaseRef(repo)
	gitMust(t, repo, "rm", "README.md")
	gitMust(t, repo, "commit", "-m", "executor already committed deletion")
	out := CommitUnitChanges(repo, pkt, nil, base, "")
	assert.Empty(t, out.Error)
	assert.Equal(t, "no changes", out.Skipped)
	writeFile(t, filepath.Join(repo, "added.txt"), "new implementation")
	out = CommitUnitChanges(repo, pkt, nil, base, "")
	require.Empty(t, out.Error)
	require.NotEmpty(t, out.SHA)
	assert.Equal(t, "new implementation", gitMust(t, repo, "show", "HEAD:added.txt"))
	require.NoError(t, os.Remove(filepath.Join(repo, "added.txt")))
	out = CommitUnitChanges(repo, pkt, nil, base, "")
	require.Empty(t, out.Error)
	assert.Empty(t, strings.TrimSpace(gitMust(t, repo, "status", "--porcelain")))
}

func TestCommittedHEADReadFailureIsReportedWithoutLosingCommit(t *testing.T) {
	for _, taskDir := range []bool{false, true} {
		t.Run(fmt.Sprint(taskDir), func(t *testing.T) {
			repo := initRepo(t)
			writeFile(t, filepath.Join(repo, "docs", "task", "spec.md"), "persisted")
			original := gitOut
			t.Cleanup(func() { gitOut = original })
			headReads := 0
			gitOut = func(cwd string, args ...string) (string, error) {
				if len(args) == 2 && args[0] == "rev-parse" && args[1] == "HEAD" {
					headReads++
					if headReads > 1 {
						return "", fmt.Errorf("HEAD read failed")
					}
				}
				return original(cwd, args...)
			}
			var out CommitOutcome
			if taskDir {
				out = CommitTaskDirChanges(repo, "docs/task/", "docs: persist", nil, nil)
			} else {
				out = CommitUnitChanges(repo, &contract.Packet{PacketID: "M01-T01", Goal: "persist"}, nil, "", "")
			}
			assert.Contains(t, out.Error, "read committed HEAD")
			assert.Equal(t, "persisted", gitMust(t, repo, "show", "HEAD:docs/task/spec.md"))
		})
	}
}

func TestDiffFileRemovedBetweenStatAndRead(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "transient.txt")
	writeFile(t, file, "do not fabricate content after the file disappears")
	original := readDiffFile
	t.Cleanup(func() { readDiffFile = original })
	readDiffFile = func(path string) ([]byte, error) {
		require.NoError(t, os.Remove(path))
		return original(path)
	}
	var diff strings.Builder
	appendUntrackedSections(&diff, dir, []string{"transient.txt"})
	assert.NotContains(t, diff.String(), "+do not fabricate")
	assert.NoFileExists(t, file)
}
