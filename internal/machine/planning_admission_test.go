package machine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/stretchr/testify/require"
)

func reviewedAdmissionBoundaries() map[string]func(*Context) error {
	return map[string]func(*Context) error{
		"unit delivery": func(c *Context) error { return c.completeUnit(c.PacketList[0]) },
		"parallel dispatch": func(c *Context) error {
			_, err := c.runParallelExecute(c.PacketList)
			return err
		},
		"parallel integration": func(c *Context) error { return c.prepareParallelIntegration(c.PacketList[0]) },
		"shared commit":        func(c *Context) error { return c.commitSharedExecution(c.PacketList) },
		"milestone gate": func(c *Context) error {
			_, err := c.sessionMilestoneGate(c.Spec.Milestones[0])
			return err
		},
		"task completion": func(c *Context) error { return c.finishTask() },
	}
}

func queueAdmissionFeedback(t *testing.T, fx fixture, opts RunOpts, requestID string) {
	t.Helper()
	before := loadFixtureState(t, fx)
	stop := false
	st, receipt, err := ControlPlanning(opts, PlanningCommand{Action: "feedback", Revision: before.Revision,
		RequestID: requestID, Design: before.Planning.CurrentDesign, Text: "Discuss recovery before any further implementation.", Start: &stop}, "")
	require.NoError(t, err)
	require.Equal(t, "queued", receipt.Status)
	require.Equal(t, before.Revision, st.Revision)
	require.Equal(t, before.Planning.InputRevision, st.Planning.InputRevision)
	require.Empty(t, st.Planning.Feedback, "publication does not claim adoption while the prior call still owns its input")
	require.FileExists(t, filepath.Join(fx.taskDir, ".control", "planning", "feedback-"+requestID+".json"))
}

func TestReviewedAdmissionProtectsEveryImplementationBoundary(t *testing.T) {
	for name, boundary := range reviewedAdmissionBoundaries() {
		t.Run(name, func(t *testing.T) {
			fx, c := newReviewedFixture(t)
			require.NoError(t, c.activatePlanningPlans())
			head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
			c.State.Planning.Approvals = []planning.Approval{}
			err := boundary(c)
			require.Error(t, err, "an existing Spec is not an execution authorization")
			require.NoDirExists(t, filepath.Join(fx.root, "build"))
			require.Equal(t, head, strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")))
			require.NotEqual(t, "completed", c.State.Status)
			for _, packet := range c.State.Packets {
				require.Empty(t, packet.Commit)
				require.Nil(t, packet.Worktree)
				require.Nil(t, packet.Evidence.L1)
			}
		})
	}
}

func TestReviewedQueuedFeedbackStopsBoundariesUntilOriginalReceiptIsConsumed(t *testing.T) {
	boundaries := reviewedAdmissionBoundaries()
	boundaries["ready queue replacement"] = func(c *Context) error {
		return c.prepareParallelPacket(c.PacketList[1], c.State.Planning.Active.Head)
	}
	for name, boundary := range boundaries {
		t.Run(name, func(t *testing.T) {
			fx, c := newReviewedFixture(t)
			require.NoError(t, c.activatePlanningPlans())
			enableSessionPolicy(c)
			a := sessionArgs(c)
			result, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
				return sessionResult(a, drivers.MockResult(nil))
			})
			require.NoError(t, err)
			require.Equal(t, "received", c.sessionRecord("execute", a.PacketID).Last.Status)
			release, err := AcquireRunLease(fx.taskDir)
			require.NoError(t, err)
			defer release()
			queueAdmissionFeedback(t, fx, RunOpts{TaskDir: fx.taskDir, Cwd: fx.root}, "feedback-boundary")
			before := c.State.Planning.InputRevision
			err = boundary(c)
			require.ErrorContains(t, err, "planning")
			require.Equal(t, before, c.State.Planning.InputRevision, "a boundary must not reinterpret an unconsumed receipt")
			require.Empty(t, c.State.Planning.Feedback)
			require.Equal(t, "received", c.sessionRecord("execute", a.PacketID).Last.Status)
			// Recovery may adopt the original terminal result, but never call the
			// provider again or use its old authority for the next stage.
			replayed, err := c.invokeScoped("execute", a.PacketID, a, func(drivers.CallArgs) drivers.Result {
				t.Fatal("queued feedback must not redispatch a received invocation")
				return drivers.Result{}
			})
			require.NoError(t, err)
			require.Equal(t, result.Session.RequestID, replayed.Session.RequestID)
			applySessionResult(t, c, "execute", a.PacketID, replayed)
			require.Error(t, c.admitReviewedBoundary(), "adopted feedback invalidates the old design authorization")
			require.Equal(t, before+1, c.State.Planning.InputRevision)
			require.Len(t, c.State.Planning.Feedback, 1)
			require.NoFileExists(t, filepath.Join(fx.taskDir, ".control", "planning", "feedback-feedback-boundary.json"))
			require.FileExists(t, filepath.Join(fx.taskDir, ".control", "planning", "receipt-feedback-boundary.json"))
			require.NoDirExists(t, filepath.Join(fx.root, "build"))
		})
	}
}

func TestReviewedFeedbackDuringExecuteStopsBeforeVerificationPrewarmAndNextPacket(t *testing.T) {
	fx, c := newReviewedFixture(t)
	require.NoError(t, c.activatePlanningPlans())
	enableSessionPolicy(c)
	disabled := false
	c.Config.Worktree.Enabled = &disabled
	c.Config.Scheduler.Prewarm, c.prewarmReview = true, true
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		st := loadFixtureState(t, fx)
		require.Equal(t, "intent", st.Sessions.Records[sessionKey("execute", a.PacketID)].Last.Status)
		queueAdmissionFeedback(t, fx, RunOpts{TaskDir: fx.taskDir, Cwd: fx.root}, "feedback-during-execute")
		return sd.mock.Execute(a)
	}
	c.Drivers = sessionDriverSet(sd)
	stubReviewPrewarm(t, func(drivers.DSHArgs) drivers.PreparedDSH {
		t.Fatal("feedback must stop before reviewer prewarm")
		return nil
	})
	// This marker is a deterministic observation of L1 starting. It remains
	// absent even though the original executor legitimately produced its file.
	c.PacketList[0].Acceptance.L1 = append(c.PacketList[0].Acceptance.L1, contract.GateEntry{Cmd: "touch verification-started", Expect: "exit 0"})
	release, err := AcquireRunLease(fx.taskDir)
	require.NoError(t, err)
	defer release()
	head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
	outcome, err := c.runExecuteLoop(c.PacketList[0])
	require.Error(t, err)
	require.True(t, outcome.Stopped)
	require.Len(t, sd.log.exec, 1)
	require.Empty(t, sd.log.review)
	require.Empty(t, sd.log.judge)
	require.Equal(t, "consumed", c.sessionRecord("execute", "M01-T01").Last.Status)
	require.Len(t, c.State.Planning.Feedback, 1)
	require.FileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))
	require.NoFileExists(t, filepath.Join(fx.root, "build", "M01-T02.txt"))
	require.NoFileExists(t, filepath.Join(fx.root, "verification-started"))
	require.Nil(t, c.State.Packets["M01-T01"].Evidence.L1)
	require.Empty(t, c.State.Packets["M01-T01"].Commit)
	require.Nil(t, c.State.WorkerPrewarm)
	require.Equal(t, head, strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")))
	_, err = c.runExecuteLoop(c.PacketList[1])
	require.Error(t, err)
	require.Len(t, sd.log.exec, 1, "the next packet cannot reuse the old root approval")
}

func TestReviewedFeedbackDuringPlanningStopsBeforeASecondDesignOrReview(t *testing.T) {
	fx, opts, calls := briefEngineFixture(t, false)
	original := opts.Drivers.DesignPlan.(*fnDriver).fn
	opts.Drivers.DesignPlan.(*fnDriver).fn = func(a drivers.CallArgs) drivers.Result {
		queueAdmissionFeedback(t, fx, opts.RunOpts, "feedback-during-design")
		return original(a)
	}
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	require.Equal(t, 1, calls["design-plan"])
	require.Zero(t, calls["design-review"], "the user asked to discuss the next input, not launch another paid stage")
	require.Zero(t, calls["plan"])
	st := loadFixtureState(t, fx)
	require.Len(t, st.Planning.Feedback, 1)
	require.Len(t, st.Planning.Designs, 1, "the already returned proposal is preserved against its original input")
	require.Equal(t, 1, st.Planning.Designs[0].InputRevision)
	require.Equal(t, 2, st.Planning.InputRevision)
	require.Empty(t, st.Planning.Approvals)
	require.NoFileExists(t, filepath.Join(fx.taskDir, "spec.md"))
	require.NoDirExists(t, filepath.Join(fx.root, "build"))
}

func TestReviewedRemoteDeliveryRequiresAuthorityAndCompletedLocalAcceptance(t *testing.T) {
	old := deliveryRun
	t.Cleanup(func() { deliveryRun = old })
	deliveryRun = func(context.Context, string, string, ...string) (string, error) {
		t.Fatal("remote delivery must reject before inspecting or modifying a remote")
		return "", nil
	}
	for _, scenario := range []string{"thin brief", "projection with stale approval", "ready but incomplete"} {
		t.Run(scenario, func(t *testing.T) {
			var fx fixture
			var opts RunTaskOpts
			if scenario == "thin brief" {
				fx, opts, _ = briefEngineFixture(t, false)
			} else {
				var c *Context
				fx, c = newReviewedFixture(t)
				require.NoError(t, c.activatePlanningPlans())
				if scenario == "projection with stale approval" {
					require.NoError(t, applyPlanningCommand(opts.RunOpts, c.State, PlanningCommand{Action: "feedback", RequestID: "delivery-new-input",
						Revision: c.State.Revision, Design: c.State.Planning.CurrentDesign, Text: "Review the changed delivery target before continuing."}, ""))
					c.save()
					require.NoError(t, c.err())
				}
				opts = runOpts(fx, c.Drivers)
			}
			request := DeliveryRequest{Action: "create", Confirmed: true, Revision: loadFixtureState(t, fx).Revision,
				ExpectedHead: strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")), ExpectedRepo: "github.com/example/fixture", Base: "main", Title: "Reviewed task"}
			snapshot, err := Deliver(opts.RunOpts, request)
			require.Error(t, err)
			require.Nil(t, snapshot)
			if scenario == "ready but incomplete" {
				require.ErrorContains(t, err, "complete the local task and its acceptance")
			}
			require.Nil(t, loadFixtureState(t, fx).Delivery)
		})
	}
}
