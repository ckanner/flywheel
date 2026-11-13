package machine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func runtimeReplanFixture(t *testing.T, priorPlan bool) (fixture, *Context, *contract.Packet, *scriptedDrivers) {
	t.Helper()
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	server, _ := runtimeServer(t, func(map[string]string) string { return "replan" }, nil)
	enableRuntimeJEV(t, c, server.URL)
	if priorPlan {
		c.Drivers = sessionDriverSet(happyDrivers())
		out, err := c.planUntilValid(c.Spec.Milestones[0])
		require.NoError(t, err)
		require.True(t, out.OK)
	}
	p := c.Packets["M01-T01"]
	facts := runtimeFailureFacts(t, c, p)
	require.Equal(t, "spin", c.runtimeFailureDecision(p, errL1Failed, facts, false))
	require.NoError(t, c.err())
	sd := happyDrivers()
	c.Drivers = sessionDriverSet(sd)
	return fx, c, p, sd
}

func TestRuntimeQuickReplanUsesTheExistingOneWayFullUpgrade(t *testing.T) {
	fx := newQuickFixture(t, false)
	c := quickContext(t, fx, quickDrivers())
	enableSessionPolicy(c)
	enableExecutionPolicy(c)
	server, count := runtimeServer(t, func(map[string]string) string { return "replan" }, nil)
	enableRuntimeJEV(t, c, server.URL)
	c.Drivers = sessionDriverSet(failingFirstDriver(t))
	p := c.Packets["M01-T01"]
	out, err := c.runExecuteLoop(p)
	require.NoError(t, err)
	require.True(t, out.Spun)
	require.True(t, c.quick())
	status, err := c.escalatePacket(p)
	require.NoError(t, err)
	require.Equal(t, "workflow-upgraded", status)
	require.False(t, c.quick())
	require.Equal(t, "full", c.State.Workflow.Effective)
	require.Equal(t, int32(1), count.Load())
	require.Empty(t, c.State.Packets[p.PacketID].PendingStrategy)
	require.Equal(t, "completed", c.State.RuntimeDecisions.Records[0].Status)
	require.Equal(t, fx.slug, c.State.Slug)
	require.NoError(t, c.err())
	require.Equal(t, "full", loadFixtureState(t, fx).Workflow.Effective)
}

func TestRuntimeConfirmedPlanRecoversWithoutDispatchingASecondPlan(t *testing.T) {
	fx, c, p, sd := runtimeReplanFixture(t, true)
	record := c.State.RuntimeDecisions.Records[0]
	previous := c.sessionRecord("plan", c.Spec.Milestones[0].ID).Last.ID
	// Run exactly the dispatch phase, then reload the committed state as if the
	// process ended before escalation could persist the valid revised contracts.
	out, err := c.runtimePlan(p, c.Spec.Milestones[0], "Repair the recorded failure")
	require.NoError(t, err)
	require.True(t, out.OK)
	require.Equal(t, "replan-running", record.Status)
	require.Equal(t, previous, record.PlanInvocationBefore)
	require.NotEqual(t, previous, c.sessionRecord("plan", c.Spec.Milestones[0].ID).Last.ID)
	require.Len(t, sd.log.plan, 1)
	loaded := loadFixtureState(t, fx)
	c.State, c.revision = loaded, loaded.Revision
	require.Equal(t, "spin", c.resumeRuntimeDecision(p))
	status, err := c.escalatePacket(p)
	require.NoError(t, err)
	require.Equal(t, "revised", status)
	require.Len(t, sd.log.plan, 1, "the precise confirmed invocation is reused, not called again")
	require.Empty(t, c.State.Packets[p.PacketID].PendingStrategy)
	require.Equal(t, "completed", c.State.RuntimeDecisions.Records[0].Status)
	require.Equal(t, 1, c.State.Packets[p.PacketID].PlanRevisions)
}

func TestRuntimeStartedPlanWithoutProofBlocksReplay(t *testing.T) {
	for _, scenario := range []string{"missing-progress", "old-invocation", "changed-proof", "missing-receipt", "changed-candidate"} {
		t.Run(scenario, func(t *testing.T) {
			_, c, p, sd := runtimeReplanFixture(t, true)
			record := c.State.RuntimeDecisions.Records[0]
			out, err := c.runtimePlan(p, c.Spec.Milestones[0], "Failure feedback")
			require.NoError(t, err)
			require.True(t, out.OK)
			require.Len(t, sd.log.plan, 1)
			milestone := c.Spec.Milestones[0]
			switch scenario {
			case "missing-progress":
				ms := c.State.Milestones[milestone.ID]
				ms.Plan = nil
				c.State.Milestones[milestone.ID] = ms
			case "old-invocation":
				record.PlanInvocationBefore = c.sessionRecord("plan", milestone.ID).Last.ID
			case "changed-proof":
				c.State.Milestones[milestone.ID].Plan.Checkpoint.Receipt = "unrelated"
			case "missing-receipt":
				require.NoError(t, os.RemoveAll(filepath.Join(c.TaskDir, ".sessions")))
			case "changed-candidate":
				writeFile(t, filepath.Join(c.Cwd, "outside.txt"), "changed after confirmed Plan")
			}
			for i := 0; i < 2; i++ {
				require.Equal(t, "spin", c.resumeRuntimeDecision(p))
				_, err := c.escalatePacket(p)
				require.Error(t, err)
			}
			require.Len(t, sd.log.plan, 1)
			require.Equal(t, "replan-running", record.Status)
			require.Equal(t, record.ID, c.State.Packets[p.PacketID].PendingStrategy)
		})
	}
}

func TestRuntimeReplanRechecksAuthorityBeforeDispatch(t *testing.T) {
	for _, entry := range []string{"resume", "dispatch", "cas"} {
		t.Run(entry, func(t *testing.T) {
			_, c, p, sd := runtimeReplanFixture(t, false)
			r := c.State.RuntimeDecisions.Records[0]
			if entry == "cas" {
				stored, err := state.Load(c.TaskDir)
				require.NoError(t, err)
				_, err = state.Save(c.TaskDir, stored, nil)
				require.NoError(t, err)
			} else {
				copy := *c
				copy.Config = jsonClone(c.Config)
				copy.Config.JEV.Enabled = false
				persistRuntimeConfig(t, &copy)
			}
			if entry == "resume" {
				require.Equal(t, "failed", c.resumeRuntimeDecision(p))
				require.Equal(t, "stale", r.Status)
			} else {
				_, err := c.runtimePlan(p, c.Spec.Milestones[0], "failure")
				require.Error(t, err)
			}
			require.Empty(t, sd.log.plan)
		})
	}
}

func TestRuntimeRecoveryRefusesCorruptOrUnsettledControlEffects(t *testing.T) {
	for _, scenario := range []string{"no-ledger", "missing-record", "wrong-status", "feedback-running", "feedback-unknown", "feedback-stale", "parallel-error"} {
		t.Run(scenario, func(t *testing.T) {
			_, c, p, facts := runtimeSafetyFixture(t)
			r := pendingRuntimeFixture(t, c, p, facts, false)
			switch scenario {
			case "no-ledger":
				c.State.RuntimeDecisions = nil
			case "missing-record":
				c.State.Packets[p.PacketID].PendingStrategy = "does-not-exist"
			case "wrong-status":
				r.Status = "completed"
			case "feedback-running", "feedback-unknown", "feedback-stale":
				r.Status = scenario
			case "parallel-error":
				r.Status = "feedback-running"
				g, workers := runtimeWorkers(c, p.PacketID)
				defer g.cancel()
				c = workers[0]
			}
			require.Empty(t, c.resumeRuntimeDecision(p))
			require.Error(t, c.err())
		})
	}
}

func TestRuntimeStaleSpunRequestReturnsTheOrdinaryEscalation(t *testing.T) {
	_, c, p, facts := runtimeSafetyFixture(t)
	r := pendingRuntimeFixture(t, c, p, facts, true)
	writeFile(t, filepath.Join(c.Cwd, "outside.txt"), "different candidate")
	require.Equal(t, "spin", c.resumeRuntimeDecision(p))
	require.Equal(t, "stale", r.Status)
	require.Empty(t, c.State.Packets[p.PacketID].ExecutionPolicy.History)
}

func TestRuntimePlanRejectsMissingBoundRecordAndFinishIsIdempotent(t *testing.T) {
	_, c, p, _ := runtimeReplanFixture(t, false)
	c.State.Packets[p.PacketID].PendingStrategy = "missing"
	_, err := c.runtimePlan(p, c.Spec.Milestones[0], "failure")
	require.ErrorContains(t, err, "record is missing")
	c.finishRuntimeReplan(p)
	require.Empty(t, c.State.Packets[p.PacketID].PendingStrategy)
	c.finishRuntimeReplan(p)
	c.State.RuntimeDecisions = nil
	c.State.Packets[p.PacketID].PendingStrategy = "old"
	c.finishRuntimeReplan(p)
	require.Equal(t, "old", c.State.Packets[p.PacketID].PendingStrategy)
}

func TestRuntimePendingRecoveryHonorsCancellationWithoutReissuing(t *testing.T) {
	_, c, p, facts := runtimeSafetyFixture(t)
	r := pendingRuntimeFixture(t, c, p, facts, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.RunContext = ctx
	require.Equal(t, "failed", c.resumeRuntimeDecision(p))
	require.ErrorIs(t, c.err(), context.Canceled)
	require.True(t, r.Accounted)
	require.Equal(t, "stale", r.Status)
	require.False(t, r.Selection.Adopted)
}

func TestRuntimeReplanInterruptedDriverDoesNotBecomeASecondAttempt(t *testing.T) {
	_, c, p, sd := runtimeReplanFixture(t, false)
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		return sessionResult(a, drivers.MockResult(func(r *drivers.Result) { r.CleanupError = "cleanup unknown" }))
	}
	c.Drivers = sd.set()
	// The managed Plan call owns its cleanup/receipt evidence. A control action
	// cannot declare the old call safe merely because a subsequent run restarted.
	_, err := c.escalatePacket(p)
	require.Error(t, err)
	require.Len(t, sd.log.plan, 1)
	c.fatal = nil
	_, err = c.escalatePacket(p)
	require.Error(t, err)
	require.Len(t, sd.log.plan, 1)
	require.Equal(t, "replan-running", c.State.RuntimeDecisions.Records[0].Status)
}
