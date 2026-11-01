package machine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextUsesRepositoryRootFromTaskSubdirectory(t *testing.T) {
	fx := newGitFixture(t, "src/main.txt")
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.taskDir, Config: mustConfig(t, fx)})
	require.NoError(t, err)
	assert.Equal(t, fx.root, c.Cwd)
	assert.Equal(t, "docs/minitask/", c.taskDirPrefix())
	assert.True(t, c.snapshotExcluded("docs/minitask/state.json"))
	assert.True(t, c.isPlannerContract("docs/minitask/spec.md"))
	assert.False(t, c.isPlannerContract("src/main.txt"))
	// A task directory that is itself the workspace has an empty relative prefix.
	nonGit := newFixture(t)
	c, err = CreateContext(Opts{TaskDir: nonGit.taskDir, Cwd: nonGit.taskDir, Config: mustConfig(t, nonGit)})
	require.NoError(t, err)
	assert.Empty(t, c.taskDirPrefix())
	assert.True(t, c.snapshotExcluded("state.json"))
	assert.True(t, c.isPlannerContract("spec.md"))
}

func TestReviewRejectsIncompleteHarnessAndStaleEvidence(t *testing.T) {
	for _, failure := range []string{"timeout", "cancelled", "cleanup", "unsettled", "truncated"} {
		t.Run(failure, func(t *testing.T) {
			fx := newGitFixture(t)
			c := unitTestContext(t, fx)
			c.Config.Retry.MaxTransportRetries = 0
			sd := happyDrivers()
			sd.reviewFn = func(drivers.CallArgs) drivers.Result {
				r := passVerdict()
				switch failure {
				case "timeout":
					r.TimedOut = true
					message := "401 unauthorized"
					r.ErrorMessage = &message // physical timeout wins over provider text
				case "cancelled":
					r.Cancelled = true
				case "cleanup":
					r.CleanupError = "owned process still running"
				case "unsettled":
					r.Settled = false
				case "truncated":
					r.Truncated = true
				}
				return r
			}
			c.Drivers = sd.set()
			seedVerification(t, c, "M01-T01")
			out, err := c.reviewStep(c.Packets["M01-T01"], nil)
			require.NoError(t, err)
			assert.False(t, out.Pass)
			if failure == "timeout" {
				assert.Equal(t, errs.TIMEOUT, out.ErrorCode)
			}
		})
	}
	t.Run("stale candidate rejected before billed call", func(t *testing.T) {
		fx := newGitFixture(t)
		c := unitTestContext(t, fx)
		sd := happyDrivers()
		c.Drivers = sd.set()
		seedVerification(t, c, "M01-T01")
		writeFile(t, filepath.Join(fx.root, "source.txt"), "changed after L1")
		out, err := c.reviewStep(c.Packets["M01-T01"], nil)
		require.NoError(t, err)
		assert.False(t, out.Pass)
		assert.Equal(t, errs.INVALID_REQUEST, out.ErrorCode)
		assert.Empty(t, sd.log.review)
	})
}

func TestDefectInjectionIncompleteStopsAndPreservesCandidate(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	c.Config.DefectInjection.EveryNUnits = new(1)
	c.Config.DefectInjection.Patch = writeDefectPatch(t, &fx)
	c.Config.DefectInjection.TestCmd = "sleep 1"
	c.Config.Verify.CommandTimeoutMs = 40
	result, err := c.processPacket(c.Packets["M01-T01"], processOpts{})
	require.NoError(t, err)
	assert.Equal(t, "stop", result)
	ps := c.State.Packets["M01-T01"]
	assert.Equal(t, "inbox", ps.Status)
	assert.Equal(t, "inconclusive", ps.Evidence.DefectInjection.(map[string]any)["outcome"])
	assert.Contains(t, c.State.Inbox[len(c.State.Inbox)-1].(map[string]any)["reason"], "not proven to detect")
	require.FileExists(t, filepath.Join(c.unitCwd(c.Packets["M01-T01"]), "build", "M01-T01.txt"))
}

func TestProcessStopsWhenL1EvidenceCannotBePersisted(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	c.Config.DefectInjection.EveryNUnits = new(1)
	// The gate itself makes the canonical ledger unavailable for the next save.
	// The candidate remains intact and no model is called after this I/O failure.
	p := c.Packets["M01-T01"]
	p.Acceptance.L1 = append(p.Acceptance.L1, contract.GateEntry{
		Cmd: fmt.Sprintf("chmod 0555 '%s'", fx.taskDir), Expect: "exit 0",
	})
	t.Cleanup(func() { _ = os.Chmod(fx.taskDir, 0o755) })
	sd := happyDrivers()
	c.Drivers = sd.set()
	_, err := c.processPacket(p, processOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	assert.Empty(t, sd.log.review)
	assert.NotEqual(t, "completed", c.State.Packets[p.PacketID].Status)
}

func TestCancellationAtReviewPauseIsNotReportedAsSuccess(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	runContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.RunContext = runContext
	c.Log = func(message string) {
		if strings.Contains(message, "paused before review") {
			cancel()
		}
	}
	_, err := c.processPacket(c.Packets["M01-T01"], processOpts{PauseBefore: "review"})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "l1-passed", c.State.Packets["M01-T01"].Status)
	require.NotNil(t, c.State.Packets["M01-T01"].Worktree)
}

func TestStrengthenedAcceptanceFailureReentersExecutionWithFeedback(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	plans, reviews, executions := 0, 0, 0
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		plans++
		if plans == 2 {
			path := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			strengthened := strings.Replace(string(raw), "review-rubric:", "    - {cmd: \"test -f build/regression.txt\", expect: \"exit 0\"}\nreview-rubric:", 1)
			writeFile(t, path, strengthened)
		}
		return sd.mock.Plan(a)
	}
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		reviews++
		if reviews == 1 {
			return gapVerdict()
		}
		return sd.mock.Review(a)
	}
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		executions++
		if executions == 2 {
			assert.Contains(t, a.Prompt, "test -f build/regression.txt")
			assert.Contains(t, a.Prompt, "Previous attempt feedback")
			writeFile(t, filepath.Join(a.Cwd, "build", "regression.txt"), "regression proven")
		}
		return sd.mock.Execute(a)
	}
	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	st := loadFixtureState(t, fx)
	assert.Equal(t, 2, st.Packets["M01-T01"].Attempts)
	assert.Equal(t, 1, st.Packets["M01-T01"].PlanRevisions)
	assert.Contains(t, gitCmd(t, fx.root, "show", "HEAD:build/regression.txt"), "regression proven")
}

func TestManualExecutionReopensCompletedMilestoneAndResumes(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
	opts := stepOpts(fx, sd.set())
	opts.Only, opts.PacketID = "execute", "M01-T01"
	step, err := RunSingleStep(opts)
	require.NoError(t, err)
	require.True(t, step.Step.OK)
	st := loadFixtureState(t, fx)
	assert.Equal(t, "running", st.Status)
	assert.Equal(t, "in-progress", st.Milestones["M-01"].Status)
	assert.Nil(t, st.Milestones["M-01"].Gate)
	assert.Nil(t, st.Milestones["M-01"].ClosedAt)
	assert.Equal(t, head, *st.Packets["M01-T01"].BaseRef)
	assert.NotEmpty(t, st.Packets["M01-T01"].EvidenceHistory)
	before := len(sd.log.exec)
	summary, err = RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	assert.Len(t, sd.log.exec, before, "same validated candidate resumes review without another model execution")
}

func TestCompletedTaskCannotHideUncommittedChangesOnRestart(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	writeFile(t, filepath.Join(fx.root, "build", "M01-T01.txt"), "modified after delivery")
	before := len(sd.log.exec)
	summary, err = RunTask(runOpts(fx, sd.set()))
	require.ErrorIs(t, err, ErrCommitFailed)
	assert.Equal(t, "escalated", summary.Status)
	assert.Equal(t, "escalated", loadFixtureState(t, fx).Status)
	assert.Len(t, sd.log.exec, before)
	assert.Contains(t, err.Error(), "final task tree is not persisted")
}

func TestPlannerEscalationCommitFailurePreservesFailedAttempt(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	p := c.Packets["M01-T01"]
	_, err := c.executeStep(p)
	require.NoError(t, err)
	worktree := c.unitCwd(p)
	hook := filepath.Join(fx.root, ".git", "hooks", "pre-commit")
	writeFile(t, hook, "#!/bin/sh\nexit 1\n")
	require.NoError(t, os.Chmod(hook, 0o755))
	sd := happyDrivers()
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		file := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
		raw, err := os.ReadFile(file)
		require.NoError(t, err)
		writeFile(t, file, string(raw)+"\nClarify the implementation boundary.\n")
		return sd.mock.Plan(a)
	}
	c.Drivers = sd.set()
	_, err = c.escalatePacket(p)
	require.ErrorIs(t, err, ErrCommitFailed)
	assert.NotEqual(t, "completed", c.State.Packets[p.PacketID].Status)
	require.DirExists(t, worktree)
	require.FileExists(t, filepath.Join(worktree, "build", p.PacketID+".txt"))
	revised, err := os.ReadFile(filepath.Join(fx.taskDir, "task-packet-M01-T01.md"))
	require.NoError(t, err)
	assert.Contains(t, string(revised), "Clarify the implementation boundary.")
}
