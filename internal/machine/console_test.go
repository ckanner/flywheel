package machine

import (
	"errors"
	"fmt"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsoleStepAdmissionAndRevision(t *testing.T) {
	fx, c := sessionFixture(t)
	require.NoError(t, c.admitConsoleStep("plan", ""))
	require.Error(t, c.admitConsoleStep("plan", "M01-T01"))
	require.Error(t, c.admitConsoleStep("execute", "M01-T99"))
	require.Error(t, c.admitConsoleStep("verify", "M01-T01"))
	require.NoError(t, c.admitConsoleStep("execute", "M01-T01"))
	c.State.Packets["M01-T01"].Attempts = 1
	require.NoError(t, c.admitConsoleStep("verify", "M01-T01"))
	milestones := c.Spec.Milestones
	c.Spec.Milestones = nil
	require.Error(t, c.admitConsoleStep("plan", ""))
	c.Spec.Milestones = milestones
	revision := int64(999)
	_, err := RunSingleStep(RunSingleStepOpts{RunOpts: RunOpts{TaskDir: fx.taskDir, Cwd: fx.root}, Only: "execute", ExpectedRevision: &revision})
	require.ErrorContains(t, err, "revision")
	_, err = RunSingleStep(RunSingleStepOpts{RunOpts: RunOpts{TaskDir: t.TempDir(), Cwd: fx.root}, Only: "execute", ExpectedRevision: &revision})
	require.Error(t, err)
	current, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	revision = current.Revision
	opts := stepOpts(fx, happyDrivers().set())
	opts.ExpectedRevision = &revision
	opts.Only = "execute"
	opts.PacketID = "M01-T99"
	_, err = RunSingleStep(opts)
	require.Error(t, err)
	current, err = state.Load(fx.taskDir)
	require.NoError(t, err)
	revision = current.Revision
	opts.PacketID = "M01-T01"
	summary, err := RunSingleStep(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	require.NotNil(t, summary.Step)
}

func TestConsoleManualContinuationIsOneAuditedAttempt(t *testing.T) {
	for _, scenario := range []string{"green", "failure", "save-failure"} {
		t.Run(scenario, func(t *testing.T) {
			fx := newGitFixture(t)
			c := unitTestContext(t, fx)
			packet := c.Packets["M01-T01"]
			c.State.Packets[packet.PacketID].FailedAttempts = packet.Escalation.MaxRetries
			originalFailures := c.State.Packets[packet.PacketID].FailedAttempts
			c.save()
			if scenario == "failure" {
				scripted := happyDrivers()
				scripted.execFn = func(drivers.CallArgs) drivers.Result { return drivers.Result{ErrorCode: errs.SERVER} }
				c.Drivers = scripted.set()
			}
			if scenario == "save-failure" {
				c.revision--
			}
			outcome, err := c.runExecuteLoop(packet, "Environment repaired; retry once")
			if scenario == "save-failure" {
				require.Error(t, err)
				require.True(t, outcome.Stopped)
				require.Zero(t, c.State.Packets[packet.PacketID].Attempts)
				return
			}
			require.NoError(t, err)
			require.Equal(t, scenario == "green", outcome.Green)
			require.Equal(t, scenario == "failure", outcome.Bounded)
			ps := c.State.Packets[packet.PacketID]
			require.Equal(t, 1, ps.Attempts)
			require.GreaterOrEqual(t, ps.FailedAttempts, originalFailures)
			require.Equal(t, "human-continue-once", ps.RetryHistory[0].(map[string]any)["policy"])
		})
	}
	_, err := RunSingleStep(RunSingleStepOpts{RetryReason: "reason"})
	require.ErrorContains(t, err, "revision-bound")
}

func TestConsoleManualContinuationRetainsHardBudget(t *testing.T) {
	c := unitTestContext(t, newGitFixture(t))
	limit := int64(1)
	c.Config.Budget.Enforce, c.Config.Budget.DailyTokenLimit = "hard", &limit
	c.State.Budget.Enforce, c.State.Budget.DailyTokenLimit = "hard", &limit
	c.State.Budget.Daily[c.Now().UTC().Format("2006-01-02")] = state.DailyBucket{Tokens: 2}
	packet := c.Packets["M01-T01"]
	outcome, err := c.runExecuteLoop(packet, "Try once after environment repair")
	require.NoError(t, err)
	require.True(t, outcome.Stopped)
	require.Zero(t, c.State.Packets[packet.PacketID].Attempts)
}

func TestConsoleReopenArchivesDownstreamWithoutErasingBudget(t *testing.T) {
	_, c := sessionFixture(t)
	first := c.State.Packets["M01-T01"]
	first.Status = "completed"
	first.Attempts, first.FailedAttempts = 5, 3
	first.Commit = map[string]any{"sha": "delivered"}
	first.Evidence.L1 = map[string]any{"pass": true}
	c.State.Budget.Totals.Tokens = 123
	c.State.Status = "completed"
	ms := c.State.Milestones[first.Milestone]
	ms.Status = "closed"
	ms.Gate = map[string]any{"pass": true}
	c.State.Milestones[first.Milestone] = ms
	require.Error(t, c.admitConsoleStep("execute", "M01-T01"))
	c.consoleRetryReason = "Recheck the implementation"
	require.NoError(t, c.admitConsoleStep("execute", "M01-T01"))
	reopened := c.State.Packets["M01-T01"]
	require.Equal(t, "pending", reopened.Status)
	require.Equal(t, 5, reopened.Attempts)
	require.Equal(t, 3, reopened.FailedAttempts)
	require.Equal(t, int64(123), c.State.Budget.Totals.Tokens)
	require.Nil(t, reopened.Evidence.L1)
	require.Nil(t, reopened.Commit)
	require.Len(t, reopened.EvidenceHistory, 2)
	require.Equal(t, "pending", c.State.Milestones[first.Milestone].Status)
}

func TestConsoleReopenRejectsUnknownReceiptsAndRetainedWorktrees(t *testing.T) {
	_, c := sessionFixture(t)
	require.Error(t, c.reopenConsolePacket("absent"))
	c.State.Sessions = &state.SessionLedger{Records: map[string]*state.SessionRecord{"execute/packet": {Last: &state.SessionInvocation{Status: "blocked"}}}}
	require.ErrorContains(t, c.reopenConsolePacket("M01-T01"), "pending session")
	c.State.Sessions.Records["execute/packet"].Last.Status = "consumed"
	c.State.Sessions.Records["execute/packet"].Last.CleanupConfirmed = true
	c.State.Packets["M01-T01"].Status = "completed"
	c.State.Packets["M01-T01"].Worktree = &state.WorktreeInfo{Mode: "worktree", Path: t.TempDir()}
	require.ErrorContains(t, c.reopenConsolePacket("M01-T01"), "retained")
	c.State.Packets["M01-T01"].Worktree = nil
	c.revision--
	require.Error(t, c.reopenConsolePacket("M01-T01"))
}

func TestConsoleReopenTargetsLaterUnitAndRejectsUnsettledHistory(t *testing.T) {
	_, c := sessionFixture(t)
	c.State.Packets["M01-T01"].Status = "completed"
	require.ErrorContains(t, c.reopenConsolePacket("missing"), "unknown packet")
	c.State.Packets["M01-T02"].Status = "completed"
	require.ErrorContains(t, c.reopenConsolePacket("missing"), "unknown packet")
	c.consoleRetryReason = "redo the second unit"
	require.NoError(t, c.admitConsoleStep("execute", "M01-T02"))
	require.Equal(t, "completed", c.State.Packets["M01-T01"].Status)
	c.State.Packets["M01-T02"].Status = "completed"
	c.State.Sessions = &state.SessionLedger{Records: map[string]*state.SessionRecord{"blocked": {Last: &state.SessionInvocation{Status: "blocked"}}}}
	require.ErrorContains(t, c.admitConsoleStep("execute", "M01-T02"), "pending session")
}

func TestManualIntegrationAndFinalAcceptanceDoNotStartPlanOrExecute(t *testing.T) {
	fx := newGitFixture(t)
	d := happyDrivers()
	opts := runOpts(fx, d.set()).RunOpts
	for _, packet := range []string{"M01-T01", "M01-T02"} {
		for _, stage := range []string{"execute", "review", "integrate"} {
			result, err := RunSingleStep(RunSingleStepOpts{RunOpts: opts, Only: stage, PacketID: packet})
			require.NoError(t, err)
			require.True(t, result.Step.OK)
		}
	}
	before := len(d.log.exec)
	result, err := RunSingleStep(RunSingleStepOpts{RunOpts: opts, Only: "final"})
	require.NoError(t, err)
	require.True(t, result.Step.OK)
	require.Equal(t, "completed", result.Status)
	require.Empty(t, d.log.plan)
	require.Equal(t, before, len(d.log.exec))
	require.Len(t, d.log.judge, 1)
	_, c := sessionFixture(t)
	require.NoError(t, c.admitConsoleStep("final", ""))
	require.Error(t, c.admitConsoleStep("final", "M01-T01"))
	ok, err := c.runFinalStage()
	require.False(t, ok)
	require.Error(t, err)
}

func TestIndependentManualNodesRespectDependenciesAndIsolation(t *testing.T) {
	fx, cfg := parallelFixture(t)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set()})
	require.NoError(t, err)
	second := c.PacketList[1]
	second.Schedule.After = []string{"M01-T01"}
	require.ErrorContains(t, c.admitConsoleStep("execute", second.PacketID), "dependency")
	second.Schedule.After = []string{}
	require.NoError(t, c.admitConsoleStep("execute", second.PacketID))
	require.True(t, c.State.Packets[second.PacketID].Parallel)
	require.NoError(t, c.admitConsoleStep("execute", second.PacketID))
	disabled := false
	cfg.Worktree.Enabled = &disabled
	require.Error(t, c.admitConsoleStep("execute", second.PacketID))
	cfg.Worktree.Enabled = nil
	c.revision--
	require.Error(t, c.admitConsoleStep("execute", second.PacketID))
	noGit := unitTestContext(t)
	noGit.PacketList[0].Schedule = &contract.PacketSchedule{After: []string{}, Writes: []string{"build/"}}
	require.ErrorContains(t, noGit.admitConsoleStep("execute", "M01-T01"), "requires Git")
	fx, cfg = parallelFixture(t)
	c, err = CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set()})
	require.NoError(t, err)
	_, path := verify.WorktreeNames(c.Cwd, c.State.Slug, "M01-T02")
	writeFile(t, path, "preserve")
	require.ErrorContains(t, c.admitConsoleStep("execute", "M01-T02"), "worktree unavailable")
	require.ErrorContains(t, c.reopenConsolePacket("M01-T02"), "earlier units")
	c.Spec.Milestones = append([]contract.Milestone{{ID: "M-00"}}, c.Spec.Milestones...)
	require.ErrorContains(t, c.reopenConsolePacket("M01-T01"), "earlier milestones")
}

func TestControlReceiptsDistinguishFailureCancellationAndLongErrors(t *testing.T) {
	for _, kind := range []string{"failed", "detailed", "cancelled", "long"} {
		_, c := sessionFixture(t)
		c.State.Control = &state.ControlResult{Kind: "execute", Status: "running", StartedAt: c.nowISO()}
		summary := TaskSummary{Step: &StepSum{Name: "execute", OK: false}}
		var err error
		switch kind {
		case "detailed":
			summary.Step.Detail = new("validation failed")
		case "cancelled":
			err = ErrInterrupted
		case "long":
			err = errors.New(strings.Repeat("中", 3000))
		}
		c.finishControl(summary, &err)
		if kind == "cancelled" {
			require.Equal(t, "cancelled", c.State.Control.Status)
		} else {
			require.Equal(t, "failed", c.State.Control.Status)
		}
		if kind == "long" {
			require.Len(t, []rune(c.State.Control.Message), 2000)
		}
	}
}

func TestManualFinalAcceptanceRetainsItsRealGateResults(t *testing.T) {
	fx := newGitFixture(t)
	d := happyDrivers()
	opts := runOpts(fx, d.set())
	_, err := RunTask(opts)
	require.NoError(t, err)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: d.set()})
	require.NoError(t, err)
	ok, err := c.runFinalStage()
	require.NoError(t, err)
	require.True(t, ok)
	// A later milestone is still pending; closing this one cannot complete it.
	fx = newGitFixture(t)
	spec := strings.Replace(fixtureSpec, "requirements:", "  - id: M-02\n    title: Follow-up\nrequirements:", 1)
	writeFile(t, filepath.Join(fx.taskDir, "spec.md"), spec)
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-q", "-m", "docs: freeze two milestones")
	opts = runOpts(fx, happyDrivers().set())
	for _, packet := range []string{"M01-T01", "M01-T02"} {
		for _, stage := range []string{"execute", "review", "integrate"} {
			_, err = RunSingleStep(RunSingleStepOpts{RunOpts: opts.RunOpts, Only: stage, PacketID: packet})
			require.NoError(t, err)
		}
	}
	c, err = CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: happyDrivers().set()})
	require.NoError(t, err)
	var ms state.MilestoneState
	ok, err = c.runFinalStage()
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "pending", c.State.Milestones["M-02"].Status)
	ok, err = c.runFinalStage()
	require.Error(t, err)
	require.False(t, ok)
	// Existing completion proof cannot bless newly edited bytes.
	writeFile(t, filepath.Join(fx.root, "changed.txt"), "new candidate")
	ms = c.State.Milestones["M-01"]
	ms.Status = "in-progress"
	c.State.Milestones["M-01"] = ms
	_, err = c.runFinalStage()
	require.ErrorContains(t, err, "delivered unit evidence")
	fx = newQuickFixture(t, false)
	qd := quickDrivers()
	qopts := runOpts(fx, qd.set())
	_, err = RunTask(qopts)
	require.NoError(t, err)
	q := quickContext(t, fx, qd)
	ok, err = q.runFinalStage()
	require.NoError(t, err)
	require.True(t, ok)
}

func TestManualFinalGateAndJudgeFailuresAreNotCompletion(t *testing.T) {
	fx := newGitFixture(t)
	marker := filepath.Join(t.TempDir(), "service-down")
	packetFile := filepath.Join(fx.taskDir, "task-packet-M01-T02.md")
	raw, err := os.ReadFile(packetFile)
	require.NoError(t, err)
	content := strings.Replace(string(raw), "review-rubric:", fmt.Sprintf("  L2:\n    - {cmd: \"test ! -f %s\", expect: \"exit 0\"}\nreview-rubric:", marker), 1)
	writeFile(t, packetFile, content)
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-q", "-m", "test: freeze environment-dependent gate")
	d := happyDrivers()
	opts := runOpts(fx, d.set())
	_, err = RunTask(opts)
	require.NoError(t, err)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: d.set()})
	require.NoError(t, err)
	writeFile(t, marker, "offline")
	ok, err := c.runFinalStage()
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, os.Remove(marker))
	d.judgeFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			r.Text = `{"verdict":"return","score":0.4,"reasons":["regression needs attention"]}`
		})
	}
	c.Drivers = d.set()
	ok, err = c.runFinalStage()
	require.NoError(t, err)
	require.False(t, ok)
}

func TestCompletedWorkspaceCleanupCanRecoverItsFinalStateCAS(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	packet := c.PacketList[0]
	executed, err := c.runExecuteLoop(packet)
	require.NoError(t, err)
	require.True(t, executed.Green)
	reviewed, err := c.reviewStep(packet, nil)
	require.NoError(t, err)
	require.True(t, reviewed.Pass)
	before := jsonClone(c.State)
	require.NoError(t, c.completeUnit(packet))
	// Reconstruct the exact pre-final-CAS checkpoint after successful physical
	// cleanup. Its candidate must still match before adopting the canonical tree.
	before.Revision = c.State.Revision
	before.Packets[packet.PacketID].Parallel = true
	before, err = state.Save(fx.taskDir, before, &before.Revision)
	require.NoError(t, err)
	resumed, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: happyDrivers().set()})
	require.NoError(t, err)
	require.NoError(t, resumed.prepareParallelIntegration(packet))
	require.Nil(t, resumed.State.Packets[packet.PacketID].Worktree)
	// The same recovery also works for a serial unit whose cleanup preceded CAS.
	resumed.State.Packets[packet.PacketID].Worktree = before.Packets[packet.PacketID].Worktree
	require.NoError(t, resumed.completeUnit(packet))
	require.Equal(t, "completed", resumed.State.Packets[packet.PacketID].Status)
}

func TestManualDeliveryRejectsUnreadyUnitsAndConfirmsQuick(t *testing.T) {
	c := unitTestContext(t, newGitFixture(t))
	for _, step := range []struct{ name, id string }{{"integrate", "missing"}, {"integrate", "M01-T01"}, {"final", ""}} {
		_, err := c.runSingle(step.name, step.id)
		require.Error(t, err)
	}
	c.State.Packets["M01-T01"].Parallel = true
	_, err := c.runSingle("integrate", "M01-T01")
	require.Error(t, err)
	for _, failAfterCommit := range []bool{false, true} {
		t.Run(fmt.Sprint(failAfterCommit), func(t *testing.T) {
			fx := newQuickFixture(t, false)
			d := quickDrivers()
			opts := runOpts(fx, d.set()).RunOpts
			for _, stage := range []string{"execute", "review"} {
				result, err := RunSingleStep(RunSingleStepOpts{RunOpts: opts, Only: stage, PacketID: "M01-T01"})
				require.NoError(t, err)
				require.True(t, result.Step.OK)
			}
			original := snapshotGitFiles
			t.Cleanup(func() { snapshotGitFiles = original })
			if failAfterCommit {
				snapshotGitFiles = func(cwd string) ([]byte, error) {
					st, err := state.Load(fx.taskDir)
					if err == nil && st.Packets["M01-T01"].Status == "completed" {
						return nil, errors.New("candidate unavailable after commit")
					}
					return original(cwd)
				}
			}
			result, err := RunSingleStep(RunSingleStepOpts{RunOpts: opts, Only: "integrate", PacketID: "M01-T01"})
			if failAfterCommit {
				require.Error(t, err)
				require.Equal(t, "completed", loadFixtureState(t, fx).Packets["M01-T01"].Status)
			} else {
				require.NoError(t, err)
				require.True(t, result.Step.OK)
				require.Equal(t, "completed", result.Status)
			}
		})
	}
}
