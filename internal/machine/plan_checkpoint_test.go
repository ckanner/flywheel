package machine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func trustedPlanFixture(t *testing.T) (fixture, *Context) {
	t.Helper()
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	r, err := c.planUntilValid(c.Spec.Milestones[0])
	require.NoError(t, err)
	require.True(t, r.OK)
	require.NotNil(t, c.State.Milestones["M-01"].Plan.Checkpoint)
	require.Empty(t, c.State.Milestones["M-01"].Plan.Checkpoint.Head, "validation alone must not claim persistence")
	require.NoError(t, c.persistPlanContracts("docs: persist tested Plan"))
	c.save()
	require.NoError(t, c.err())
	return fx, c
}

func TestPlanCheckpointResumeAvoidsAnUnchangedPlan(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	opts := runOpts(fx, sd.set())
	opts.PauseBefore = "review"
	paused, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", paused.Status)
	require.Len(t, sd.log.plan, 1)
	saved := loadFixtureState(t, fx).Milestones["M-01"].Plan
	require.NotNil(t, saved.Checkpoint)
	require.NotEmpty(t, saved.Checkpoint.Head)

	completed, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	require.Equal(t, "completed", completed.Status)
	assert.Len(t, sd.log.plan, 1, "normal resume must keep the persisted valid Plan")
	assert.Contains(t, loadFixtureState(t, fx).Milestones["M-01"].Plan.Reason, "remains valid")
}

func TestExplicitPlanStillRunsWithTrustedCheckpoint(t *testing.T) {
	fx, _ := trustedPlanFixture(t)
	sd := happyDrivers()
	opts := stepOpts(fx, sd.set())
	opts.Only = "plan"
	for range 2 {
		result, err := RunSingleStep(opts)
		require.NoError(t, err)
		require.True(t, result.Step.OK)
	}
	assert.Len(t, sd.log.plan, 2, "explicit Plan is a replan request, not a resume")
	assert.Equal(t, "explicit Plan or repair request", loadFixtureState(t, fx).Milestones["M-01"].Plan.Reason)
}

func TestTrustedCheckpointCannotBypassUnknownPlanInvocation(t *testing.T) {
	fx, c := trustedPlanFixture(t)
	c.State.PlanSession = map[string]any{"outcome-unknown": true}
	c.save()
	require.NoError(t, c.err())
	sd := happyDrivers()
	_, err := RunTask(runOpts(fx, sd.set()))
	require.ErrorContains(t, err, "previous resident Plan result remains unknown")
	assert.Empty(t, sd.log.plan)
	assert.Empty(t, sd.log.exec)
}

func TestVerificationGapReplansEvenWhenCheckpointWasReusable(t *testing.T) {
	fx, _ := trustedPlanFixture(t)
	sd := happyDrivers()
	reviews := 0
	sd.reviewFn = func(drivers.CallArgs) drivers.Result {
		reviews++
		if reviews == 1 {
			return gapVerdict()
		}
		return passVerdict()
	}
	result, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.Len(t, sd.log.plan, 1, "the valid initial checkpoint is reused, but a later verification gap forces Plan")
	assert.Equal(t, 1, loadFixtureState(t, fx).Packets["M01-T01"].PlanRevisions)
}

func TestPlanCheckpointRequiresVersionedCompleteProof(t *testing.T) {
	_, c := trustedPlanFixture(t)
	baseline := *c.State.Milestones["M-01"].Plan
	checkpoint := *baseline.Checkpoint
	for name, change := range map[string]func(*state.PlanProgress){
		"legacy passed":        func(p *state.PlanProgress) { p.Checkpoint = nil },
		"running":              func(p *state.PlanProgress) { p.Status = "running" },
		"unfinished":           func(p *state.PlanProgress) { p.FinishedAt = "" },
		"unknown version":      func(p *state.PlanProgress) { p.Checkpoint.Version++ },
		"different milestone":  func(p *state.PlanProgress) { p.Checkpoint.Milestone = "M-02" },
		"missing confirmation": func(p *state.PlanProgress) { p.Checkpoint.Receipt = "" },
		"not persisted":        func(p *state.PlanProgress) { p.Checkpoint.Head = "" },
		"changed inputs":       func(p *state.PlanProgress) { p.Checkpoint.Inputs = strings.Repeat("a", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			progress, proof := baseline, checkpoint
			progress.Checkpoint = &proof
			change(&progress)
			ms := c.State.Milestones["M-01"]
			ms.Plan = &progress
			c.State.Milestones["M-01"] = ms
			required, reason := c.needsPlan("M-01")
			assert.True(t, required)
			assert.NotEmpty(t, reason)
		})
	}
	ms := c.State.Milestones["M-01"]
	ms.Plan = &baseline
	c.State.Milestones["M-01"] = ms
	c.Config.Verify.CommandTimeoutMs++
	required, reason := c.needsPlan("M-01")
	assert.True(t, required)
	assert.Contains(t, reason, "policy changed")
}

func TestPlanCheckpointTracksKnownDeliveryInsteadOfEveryCodeEdit(t *testing.T) {
	fx, c := trustedPlanFixture(t)
	required, _ := c.needsPlan("M-01")
	require.False(t, required)
	writeFile(t, filepath.Join(fx.root, "implementation.go"), "package implementation\n")
	required, _ = c.needsPlan("M-01")
	assert.False(t, required, "ordinary Execute work is expected to change candidate contents")

	gitCmd(t, fx.root, "add", "implementation.go")
	gitCmd(t, fx.root, "commit", "-qm", "external baseline change")
	required, reason := c.needsPlan("M-01")
	assert.True(t, required)
	assert.Contains(t, reason, "baseline")
	head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
	ps := c.State.Packets["M01-T01"]
	ps.Status, ps.Commit = "completed", map[string]any{"sha": head}
	required, _ = c.needsPlan("M-01")
	assert.False(t, required, "an orchestrator-recorded packet delivery advances the known baseline")

	checkpoint := c.State.Milestones["M-01"].Plan.Checkpoint
	checkpoint.Head = strings.Repeat("0", 40)
	assert.False(t, c.knownPlanBaseline(checkpoint), "an unavailable/non-ancestor anchor cannot be trusted")
	c.Cwd = t.TempDir()
	assert.False(t, c.knownPlanBaseline(checkpoint))
	gitCmd(t, c.Cwd, "init", "-q")
	assert.False(t, c.knownPlanBaseline(checkpoint), "an empty repository is not persisted Plan proof")
}

func TestPlanPersistenceFailureCannotPromoteValidatedResult(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	sd := happyDrivers()
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		path := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		writeFile(t, path, string(raw)+"\nPlanner clarification.\n")
		return sd.mock.Plan(a)
	}
	c.Drivers = sd.set()
	result, err := c.planUntilValid(c.Spec.Milestones[0])
	require.NoError(t, err)
	require.True(t, result.OK)
	installCommitHook(t, fx, "exit 1")
	require.ErrorIs(t, c.persistPlanContracts("docs: failed Plan checkpoint"), ErrCommitFailed)
	assert.Empty(t, c.State.Milestones["M-01"].Plan.Checkpoint.Head)
	required, _ := c.needsPlan("M-01")
	assert.True(t, required)
}

func TestCheckpointConfirmationRequiresCleanPhysicalAndSessionOutcome(t *testing.T) {
	c := unitTestContext(t, newGitFixture(t))
	assert.Nil(t, c.confirmedPlanCheckpoint("M-01", nil))
	for name, change := range map[string]func(*drivers.Result){
		"unsettled":    func(r *drivers.Result) { r.Settled = false },
		"no exit":      func(r *drivers.Result) { r.ExitCode = nil },
		"failure exit": func(r *drivers.Result) { *r.ExitCode = 1 },
		"timeout":      func(r *drivers.Result) { r.TimedOut = true },
		"cancel":       func(r *drivers.Result) { r.Cancelled = true },
		"truncated":    func(r *drivers.Result) { r.Truncated = true },
		"signal":       func(r *drivers.Result) { r.Signal = "SIGTERM" },
		"spawn":        func(r *drivers.Result) { r.SpawnError = "failed" },
		"cleanup":      func(r *drivers.Result) { r.CleanupError = "unknown" },
		"provider":     func(r *drivers.Result) { r.ErrorCode = errs.SERVER },
		"message":      func(r *drivers.Result) { msg := "failed"; r.ErrorMessage = &msg },
		"unknown session": func(r *drivers.Result) {
			r.Session = &drivers.SessionOutcome{RequestID: "call-a", Dispatch: "dispatched", Outcome: "unknown", CleanupConfirmed: true}
		},
		"failed session": func(r *drivers.Result) {
			r.Session = &drivers.SessionOutcome{RequestID: "call-a", Dispatch: "dispatched", Outcome: "failed", CleanupConfirmed: true}
		},
	} {
		t.Run(name, func(t *testing.T) {
			result := drivers.MockResult(change)
			assert.Nil(t, c.confirmedPlanCheckpoint("M-01", &result))
		})
	}
	result := drivers.MockResult(func(r *drivers.Result) {
		r.Session = &drivers.SessionOutcome{RequestID: "call-a", Dispatch: "dispatched", Outcome: "completed", CleanupConfirmed: true}
	})
	assert.NotNil(t, c.confirmedPlanCheckpoint("M-01", &result))
	c.State.ContractFingerprints = nil
	assert.Nil(t, c.confirmedPlanCheckpoint("M-01", &result), "legacy state cannot fabricate contract proof")
}

func TestCheckpointPersistenceSkipsLegacyAndInvalidProofs(t *testing.T) {
	c := unitTestContext(t, newGitFixture(t))
	c.State.Milestones["M-02"] = state.MilestoneState{Plan: &state.PlanProgress{Status: "passed"}}
	c.State.Milestones["M-03"] = state.MilestoneState{Plan: &state.PlanProgress{Status: "passed", Checkpoint: &state.PlanCheckpoint{Version: 2}}}
	c.persistPlanCheckpoints("")
	c.persistPlanCheckpoints(verify.EmptyTree)
	c.persistPlanCheckpoints(strings.Repeat("a", 40))
	assert.Nil(t, c.State.Milestones["M-01"].Plan)
	assert.Nil(t, c.State.Milestones["M-02"].Plan.Checkpoint)
	assert.Empty(t, c.State.Milestones["M-03"].Plan.Checkpoint.Head)
	required, _ := c.needsPlan("M-01")
	assert.True(t, required)
}

func TestPlanPersistenceAndRunStopBeforeActionsOnRecordedError(t *testing.T) {
	c := unitTestContext(t, newGitFixture(t))
	c.fatal = errors.New("phase could not be persisted")
	require.ErrorIs(t, c.persistPlanContracts("docs: must not commit"), c.fatal)
	_, err := c.run()
	require.ErrorIs(t, err, c.fatal)
	assert.Nil(t, c.State.Milestones["M-01"].Plan)
}
