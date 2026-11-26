package machine

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/stretchr/testify/require"
)

func trackedPlanningDrivers(drvs *drivers.Drivers) {
	wrapped := map[*fnDriver]bool{}
	for _, role := range []string{"design-plan", "design-review", "plan", "plan-review"} {
		if d, ok := drvs.ForRole(role).(*fnDriver); ok && d.fn != nil && !wrapped[d] {
			wrapped[d] = true
			original := d.fn
			d.fn = func(a drivers.CallArgs) drivers.Result { return sessionResult(a, original(a)) }
		}
	}
}

func queueDuringChild(t *testing.T, fx fixture, opts RunOpts, role string, start bool) *string {
	t.Helper()
	invocation := ""
	d := opts.Drivers.ForRole(role).(*fnDriver)
	original := d.fn
	d.fn = func(a drivers.CallArgs) drivers.Result {
		if invocation == "" {
			before := loadFixtureState(t, fx)
			lastReq := before.Planning.Requests[len(before.Planning.Requests)-1]
			record := before.Sessions.Records[sessionKey(role, planningSessionScope(&lastReq))]
			require.NotNil(t, record)
			require.Equal(t, "intent", record.Last.Status)
			require.Equal(t, a.RequestID, record.Last.ID)
			invocation = a.RequestID
			_, receipt, err := ControlPlanning(opts, PlanningCommand{Action: "feedback", Revision: before.Revision, RequestID: "child-feedback", Design: before.Planning.CurrentDesign,
				Text: "Discuss the original recovery assumptions before adopting this child plan.", Start: &start}, "")
			require.NoError(t, err)
			require.Equal(t, "queued", receipt.Status)
			after := loadFixtureState(t, fx)
			require.Equal(t, before.Planning.InputRevision, after.Planning.InputRevision)
			require.Empty(t, after.Planning.Feedback, "an in-flight child still owns its original input")
		}
		return original(a)
	}
	return &invocation
}

func assertChildFeedbackConsumed(t *testing.T, fx fixture, role, invocation string) {
	t.Helper()
	st := loadFixtureState(t, fx)
	require.Equal(t, 2, st.Planning.InputRevision)
	require.Len(t, st.Planning.Feedback, 1)
	found := false
	for _, record := range st.Sessions.Records {
		if record.Role == role && record.Last.ID == invocation {
			found = true
			require.Equal(t, "consumed", record.Last.Status)
			require.True(t, record.Last.Accounted)
		}
	}
	require.True(t, found, "the original child invocation remains accounted in its own role scope")
	_, receipt, err := ControlPlanning(RunOpts{TaskDir: fx.taskDir, Cwd: fx.root}, PlanningCommand{Action: "status", RequestID: "child-feedback"}, "")
	require.NoError(t, err)
	require.Equal(t, "applied", receipt.Status)
	require.NoFileExists(t, filepath.Join(fx.taskDir, ".control", "planning", "feedback-child-feedback.json"))
	require.NoDirExists(t, filepath.Join(fx.root, "build"))
}

func TestReviewedAutomaticFirstChildReturnsToRootLoopAfterQueuedFeedback(t *testing.T) {
	for _, role := range []string{"plan", "plan-review"} {
		for _, start := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-start-%t", role, start), func(t *testing.T) {
				fx, opts, calls := briefEngineFixture(t, false)
				enableSessionPolicy(&Context{Config: opts.Config})
				trackedPlanningDrivers(opts.Drivers)
				_, err := RunTask(opts)
				require.NoError(t, err)
				approveEngineDesign(t, fx, true, "first-design-start")
				invocation := queueDuringChild(t, fx, opts.RunOpts, role, start)
				summary, err := RunTask(opts)
				require.NoError(t, err, "new input is an ordinary planning boundary, not an activation error")
				require.Equal(t, "paused", summary.Status)
				assertChildFeedbackConsumed(t, fx, role, *invocation)
				st := loadFixtureState(t, fx)
				require.Nil(t, st.Planning.Active)
				require.Nil(t, st.Planning.PendingActivation)
				require.NoFileExists(t, filepath.Join(fx.taskDir, "spec.md"))
				require.Equal(t, 1, calls["plan"])
				if role == "plan" {
					require.Zero(t, calls["plan-review"])
				} else {
					require.Equal(t, 1, calls["plan-review"])
				}
				if start {
					require.Equal(t, 2, calls["design-plan"])
					require.Equal(t, 2, calls["design-review"])
					require.Equal(t, "awaiting-design-approval", summary.PlanningPhase)
					require.False(t, planning.StartAllowed(st.Planning, *st.Planning.CurrentDesign), "start:true on feedback cannot approve or authorize a replacement root")
				} else {
					require.Equal(t, 1, calls["design-plan"])
					require.Equal(t, 1, calls["design-review"])
					require.Equal(t, "drafting", summary.PlanningPhase)
				}
			})
		}
	}
}

func TestReviewedMilestoneAndSinglePlanPauseAfterQueuedChildFeedback(t *testing.T) {
	for _, entry := range []string{"milestone", "single"} {
		for _, role := range []string{"plan", "plan-review"} {
			for _, start := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s-%s-start-%t", entry, role, start), func(t *testing.T) {
					fx, c, calls := adoptionPlannerContext(t)
					require.NoError(t, c.activatePlanningPlans())
					active := *c.State.Planning.Active
					head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
					spec := mustReadFile(t, filepath.Join(fx.taskDir, "spec.md"))
					enableSessionPolicy(c)
					trackedPlanningDrivers(c.Drivers)
					invocation := queueDuringChild(t, fx, RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: c.Drivers}, role, start)
					release, err := AcquireRunLease(fx.taskDir)
					require.NoError(t, err)
					defer release()
					if entry == "milestone" {
						out, err := c.reviewedMilestonePlan(c.Spec.Milestones[0], "Refine active milestone", nil)
						require.NoError(t, err)
						require.True(t, out.Stopped)
						require.False(t, out.OK)
					} else {
						summary, err := c.runReviewedSinglePlan("M01-T01")
						require.NoError(t, err)
						require.Equal(t, "paused", summary.Status)
					}
					assertChildFeedbackConsumed(t, fx, role, *invocation)
					require.Equal(t, 1, calls["plan"])
					if role == "plan" {
						require.Zero(t, calls["plan-review"])
					} else {
						require.Equal(t, 1, calls["plan-review"])
					}
					require.Equal(t, active, *c.State.Planning.Active, "the prior committed projection remains intact for review")
					require.Nil(t, c.State.Planning.PendingActivation)
					require.Equal(t, spec, mustReadFile(t, filepath.Join(fx.taskDir, "spec.md")))
					require.Equal(t, head, strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")))
					require.Equal(t, "drafting", c.State.Planning.Phase)
					require.False(t, planning.Authorized(c.State.Planning, *c.State.Planning.CurrentDesign))
				})
			}
		}
	}
}
