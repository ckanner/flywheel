package defectinjection

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitForTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	raw, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, raw)
	return string(raw)
}

func injectionRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	gitForTest(t, dir, "init")
	gitForTest(t, dir, "config", "user.email", "test@example.com")
	gitForTest(t, dir, "config", "user.name", "test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source.txt"), []byte("baseline\n"), 0o644))
	gitForTest(t, dir, "add", ".")
	gitForTest(t, dir, "commit", "-m", "baseline")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source.txt"), []byte("executor implementation\n"), 0o644))
	return dir, writePatch(t, dir)
}

func TestRealInjectionRestoresWithoutDiscardingExecutorChanges(t *testing.T) {
	dir, patch := injectionRepo(t)
	before := gitForTest(t, dir, "diff")
	o := RunDefectInjection(Args{
		Cwd: dir, PatchFile: patch, TestCmd: "test ! -f poison.txt", TimeoutMs: 3000, KillGraceMs: 100,
	})
	require.Equal(t, "pass", o.Outcome)
	assert.True(t, o.Pass)
	assert.True(t, o.TestsWentRed)
	assert.True(t, o.Restored)
	assert.NoFileExists(t, filepath.Join(dir, "poison.txt"))
	assert.Equal(t, before, gitForTest(t, dir, "diff"))
}

func TestInjectedTestTimeoutIsInconclusiveAndRestores(t *testing.T) {
	dir, patch := injectionRepo(t)
	before := gitForTest(t, dir, "diff")
	var injected RunResult
	o := RunDefectInjection(Args{
		Cwd: dir, PatchFile: patch,
		TestCmd:   `if [ -f poison.txt ]; then trap 'exit 1' TERM; while :; do :; done; fi`,
		TimeoutMs: 3000, KillGraceMs: 1000,
		Run: func(command string, args []string, cwd string, timeoutMs int64, envAllowlist []string) RunResult {
			// Only the deliberately hanging, poisoned test gets a short
			// deadline. Real git apply/rollback must not depend on a busy
			// CI runner completing process startup and IO within 150 ms.
			if command == "/bin/sh" {
				if _, err := os.Stat(filepath.Join(cwd, "poison.txt")); err == nil {
					injected = managedRun(context.Background(), command, args, cwd, 150, 1000, envAllowlist)
					return injected
				}
			}
			return managedRun(context.Background(), command, args, cwd, timeoutMs, 1000, envAllowlist)
		},
	})
	require.True(t, injected.TimedOut, "the injected test must actually hit the watchdog deadline")
	assert.Equal(t, "inconclusive", o.Outcome)
	assert.False(t, o.Pass)
	assert.False(t, o.TestsWentRed, "超时恰好exit1也不能证明缺陷被检测")
	assert.True(t, o.Restored)
	assert.NoFileExists(t, filepath.Join(dir, "poison.txt"))
	assert.Equal(t, before, gitForTest(t, dir, "diff"))
}

func TestInjectionCancellationReapsWritersAndRunsIndependentRollback(t *testing.T) {
	dir, patch := injectionRepo(t)
	before := gitForTest(t, dir, "diff")
	marker := filepath.Join(dir, "writes")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	script := fmt.Sprintf(`if [ -f poison.txt ]; then
(trap '' TERM; while :; do printf x >> %q; sleep 0.01; done) >/dev/null 2>&1 &
wait
fi`, marker)
	done := make(chan Outcome, 1)
	go func() {
		done <- RunDefectInjection(Args{
			Context: ctx, Cwd: dir, PatchFile: patch, TestCmd: script,
			TimeoutMs: 3000, KillGraceMs: 100,
		})
	}()
	require.Eventually(t, func() bool {
		raw, _ := os.ReadFile(marker)
		return len(raw) > 0
	}, 3*time.Second, 10*time.Millisecond)
	cancel()
	var o Outcome
	select {
	case o = <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("cancelled injection did not finish rollback")
	}
	assert.Equal(t, "inconclusive", o.Outcome)
	assert.False(t, o.TestsWentRed)
	assert.True(t, o.Restored, "取消只终止试验，独立context必须允许补偿操作")
	assert.NoFileExists(t, filepath.Join(dir, "poison.txt"))
	assert.Equal(t, before, gitForTest(t, dir, "diff"))
	writes, err := os.ReadFile(marker)
	require.NoError(t, err)
	time.Sleep(150 * time.Millisecond)
	after, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, writes, after, "取消返回后测试孙进程不得继续写入")
}

func TestIncompleteBaselineNeverInjects(t *testing.T) {
	for _, tc := range []struct {
		name, command, reason string
	}{
		{"timeout", "sleep 30", "timed out"},
		{"signal", "kill -KILL $$", "signal"},
		{"open pipes", "sleep 30 & exit 0", "output did not close"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, patch := injectionRepo(t)
			o := RunDefectInjection(Args{
				Cwd: dir, PatchFile: patch, TestCmd: tc.command, TimeoutMs: 200, KillGraceMs: 50,
			})
			assert.Equal(t, "inconclusive", o.Outcome)
			assert.False(t, o.Injected)
			assert.Nil(t, o.BaselineGreen)
			require.NotNil(t, o.Detail)
			assert.Contains(t, *o.Detail, tc.reason)
			assert.NoFileExists(t, filepath.Join(dir, "poison.txt"))
		})
	}
	t.Run("spawn failure", func(t *testing.T) {
		dir, patch := injectionRepo(t)
		o := RunDefectInjection(Args{Cwd: filepath.Join(dir, "missing"), PatchFile: patch, TestCmd: "true"})
		assert.Equal(t, "inconclusive", o.Outcome)
		assert.False(t, o.Injected)
	})
	t.Run("cancelled before start", func(t *testing.T) {
		dir, patch := injectionRepo(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		o := RunDefectInjection(Args{Context: ctx, Cwd: dir, PatchFile: patch, TestCmd: "touch should-not-exist"})
		assert.Equal(t, "inconclusive", o.Outcome)
		assert.NoFileExists(t, filepath.Join(dir, "should-not-exist"))
	})
	t.Run("truncated output", func(t *testing.T) {
		old := maxBuffer
		maxBuffer = 8
		t.Cleanup(func() { maxBuffer = old })
		dir, patch := injectionRepo(t)
		o := RunDefectInjection(Args{Cwd: dir, PatchFile: patch, TestCmd: "printf 'too much output'"})
		assert.Equal(t, "inconclusive", o.Outcome)
		assert.Contains(t, *o.Detail, "truncated")
	})
	t.Run("adapter did not report exit", func(t *testing.T) {
		dir, patch := injectionRepo(t)
		o := RunDefectInjection(Args{Cwd: dir, PatchFile: patch, TestCmd: "suite",
			Run: func(string, []string, string, int64, []string) RunResult { return RunResult{ExitCode: -1} },
		})
		assert.Equal(t, "inconclusive", o.Outcome)
		assert.Contains(t, *o.Detail, "normal exit")
	})
}

func TestInterruptedApplyAttemptsRollbackAndNeverPasses(t *testing.T) {
	for _, applied := range []bool{false, true} {
		t.Run(fmt.Sprint(applied), func(t *testing.T) {
			dir, patch := injectionRepo(t)
			o := RunDefectInjection(Args{Cwd: dir, PatchFile: patch, TestCmd: "test ! -f poison.txt",
				Run: func(command string, args []string, cwd string, timeoutMs int64, allowlist []string) RunResult {
					if command == "git" && args[1] != "-R" {
						if applied {
							require.Equal(t, 0, DefaultRun(command, args, cwd, timeoutMs, allowlist).ExitCode)
						}
						return RunResult{ExitCode: -1, TimedOut: true}
					}
					return DefaultRun(command, args, cwd, timeoutMs, allowlist)
				},
			})
			assert.False(t, o.Pass)
			assert.False(t, o.TestsWentRed)
			if applied {
				assert.Equal(t, "inconclusive", o.Outcome)
				assert.True(t, o.Restored)
			} else {
				assert.Equal(t, "restore-failed", o.Outcome, "无法确认中断的apply状态，保守停机")
			}
			assert.NoFileExists(t, filepath.Join(dir, "poison.txt"))
		})
	}
}

func TestInterruptedRollbackPreservesFailureAndWorkspace(t *testing.T) {
	dir, patch := injectionRepo(t)
	o := RunDefectInjection(Args{Cwd: dir, PatchFile: patch, TestCmd: "test ! -f poison.txt",
		Run: func(command string, args []string, cwd string, timeoutMs int64, allowlist []string) RunResult {
			if command == "git" && args[1] == "-R" {
				return managedRun(context.Background(), "/bin/sh", []string{"-c", "sleep 30"}, cwd, 50, 20, allowlist)
			}
			return DefaultRun(command, args, cwd, timeoutMs, allowlist)
		},
	})
	assert.Equal(t, "restore-failed", o.Outcome)
	assert.False(t, o.Pass)
	assert.False(t, o.Restored)
	assert.True(t, o.TestsWentRed)
	assert.FileExists(t, filepath.Join(dir, "poison.txt"), "未恢复的证据必须保留供人工修复")
	assert.Contains(t, *o.Detail, "timed out")
	assert.Contains(t, strings.TrimSpace(gitForTest(t, dir, "diff")), "+executor implementation")
}
