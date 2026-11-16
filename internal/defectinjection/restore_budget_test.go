package defectinjection

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestoreReceivesItsEffectiveCommandBudget(t *testing.T) {
	for _, tc := range []struct {
		name               string
		requested, restore int64
	}{
		{"default", 0, MaxRestoreTimeoutMs},
		{"negative default", -1, MaxRestoreTimeoutMs},
		{"short positive", 1, 1000},
		{"below floor", 999, 1000},
		{"at floor", 1000, 1000},
		{"unchanged", 2500, 2500},
		{"at cap", MaxRestoreTimeoutMs, MaxRestoreTimeoutMs},
		{"capped", 20_000, MaxRestoreTimeoutMs},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var budgets []int64
			var reversed bool
			o := RunDefectInjection(Args{
				Context: ctx, Cwd: dir, PatchFile: writePatch(t, dir), TestCmd: "suite", TimeoutMs: tc.requested,
				Run: func(command string, args []string, _ string, timeoutMs int64, _ []string) RunResult {
					budgets = append(budgets, timeoutMs)
					if command == "git" && args[1] == "-R" {
						reversed = true
						require.ErrorIs(t, ctx.Err(), context.Canceled, "rollback must remain admitted after the test is cancelled")
					}
					if len(budgets) == 3 {
						cancel()
						return RunResult{ExitCode: -1, Cancelled: true}
					}
					return RunResult{ExitCode: 0}
				},
			})
			assert.Equal(t, []int64{tc.requested, tc.requested, tc.requested, tc.restore}, budgets,
				"only reverse-apply gets the independent compensation budget")
			assert.True(t, reversed)
			assert.Equal(t, "inconclusive", o.Outcome)
			assert.True(t, o.Restored)
			assert.False(t, o.TestsWentRed, "cancellation cannot become evidence that the defect was detected")
		})
	}
}

func TestShortTestBudgetAllowsDelayedRealReverseApply(t *testing.T) {
	dir, patch := injectionRepo(t)
	before := gitForTest(t, dir, "diff")
	var restore RunResult
	var restoreBudget int64
	o := RunDefectInjection(Args{
		Cwd: dir, PatchFile: patch, TestCmd: "test ! -f poison.txt", TimeoutMs: 1,
		Run: func(command string, args []string, cwd string, timeoutMs int64, allowlist []string) RunResult {
			if command == "git" && args[1] == "-R" {
				restoreBudget = timeoutMs
				// Exercise the effective budget through the real watchdog and git.
				// The delay cannot fit the original 1 ms test budget, but leaves
				// most of the independent 1 s restore budget for process startup.
				delayed := append([]string{"-c", `sleep 0.1; exec "$@"`, "restore", command}, args...)
				restore = managedRun(context.Background(), "/bin/sh", delayed, cwd, timeoutMs, 1000, allowlist)
				return restore
			}
			// Setup and the healthy/poisoned test are not the timing experiment;
			// keep their real process startup independent of a busy CI runner.
			return managedRun(context.Background(), command, args, cwd, 3000, 1000, allowlist)
		},
	})
	require.Equal(t, int64(1000), restoreBudget)
	require.Empty(t, restore.incompleteReason())
	require.Equal(t, "pass", o.Outcome)
	assert.True(t, o.Restored)
	assert.NoFileExists(t, filepath.Join(dir, "poison.txt"))
	assert.Equal(t, before, gitForTest(t, dir, "diff"), "rollback must preserve the executor's unrelated changes")
}
