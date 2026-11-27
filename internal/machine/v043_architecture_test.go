package machine

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPillar1CancelPlanAndContinueExecutionAndRetryTarget(t *testing.T) {
	// Validation errors for new fields and actions.
	require.ErrorContains(t, ValidatePlanningCommand(PlanningCommand{Action: "plan", Revision: 1, RequestID: "r1", Scope: "design", TargetRequestID: "x"}), "do not belong")
	require.ErrorContains(t, ValidatePlanningCommand(PlanningCommand{Action: "cancel-plan", Revision: 1, RequestID: "r1", ContinueExecution: true}), "do not belong")
	require.ErrorContains(t, ValidatePlanningCommand(PlanningCommand{Action: "plan", Revision: 1, RequestID: "r1", Scope: "design", ContinueExecution: true}), "milestone or packet")
	require.ErrorContains(t, ValidatePlanningCommand(PlanningCommand{Action: "cancel-plan", Revision: 1, RequestID: "r1", TargetRequestID: "bad id!"}), "target-request-id")

	// Start with an approved design + StartAllowed + milestone M-02 budget-stopped + mistaken running design request.
	opts, st, _ := controlPlanningFixture(t)
	// Unauthorized continue-execution before approval must fail.
	_, _, err := ControlPlanning(opts, PlanningCommand{Action: "plan", Revision: st.Revision, RequestID: "m02-unauth", Scope: "milestone", ScopeID: "M-02", ContinueExecution: true}, "")
	require.ErrorContains(t, err, "execution authorization")

	// Approve design with Start=true via reviewed fixture.
	fx, c := newReviewedFixture(t)
	opts = RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config}
	c.State.CurrentMilestone = new("M-02")
	c.State.Milestones["M-01"] = state.MilestoneState{Status: "closed"}
	c.State.Planning.Requests = append(c.State.Planning.Requests,
		planning.Request{ID: "engine-m02", Scope: "milestone", ScopeID: "M-02", InputRevision: 1, Rounds: 3, MaxRounds: 3, Status: "budget-stopped", PlanningOnly: false},
		planning.Request{ID: "plan-other-pending", Scope: "milestone", ScopeID: "M-02", InputRevision: 1, Rounds: 0, MaxRounds: 3, Status: "pending", PlanningOnly: false},
		planning.Request{ID: "plan-retry-mistake", Scope: "design", ScopeID: "", InputRevision: 1, Rounds: 1, MaxRounds: 3, Status: "running", PlanningOnly: true},
	)
	c.State.Planning.Rounds += 4
	c.save()
	require.NoError(t, c.err())

	// Projection exposes authoritative retry-target (milestone/M-02, continue-execution=true) + cancellable request.
	view, err := ConsoleProjection(c.State)
	require.NoError(t, err)
	require.NotNil(t, view.Planning.Projection.RetryTarget)
	rt := view.Planning.Projection.RetryTarget
	assert.Equal(t, "milestone", rt.Scope)
	assert.Equal(t, "M-02", rt.ScopeID)
	assert.True(t, rt.ContinueExecution)
	assert.Equal(t, "engine-m02", rt.FailedRequestID)
	assert.Equal(t, "plan-retry-mistake", rt.CancellableRequestID)

	// Cancel targeted mistaken design request without cancelling the sibling pending request.
	saved, receipt, err := ControlPlanning(opts, PlanningCommand{Action: "cancel-plan", Revision: c.State.Revision, RequestID: "cancel-1", TargetRequestID: "plan-retry-mistake"}, "")
	require.NoError(t, err)
	assert.Equal(t, "applied", receipt.Status)
	assert.Equal(t, "ready", saved.Planning.Phase)
	assert.Equal(t, "pending", saved.Planning.Requests[len(saved.Planning.Requests)-2].Status)
	assert.Equal(t, "cancelled", saved.Planning.Requests[len(saved.Planning.Requests)-1].Status)

	// Idempotent replay with same request-id and target returns unchanged; mismatched target fails.
	_, replayReceipt, err := ControlPlanning(opts, PlanningCommand{Action: "cancel-plan", Revision: saved.Revision, RequestID: "cancel-1", TargetRequestID: "plan-retry-mistake"}, "")
	require.NoError(t, err)
	assert.Equal(t, "unchanged", replayReceipt.Status)
	_, _, err = ControlPlanning(opts, PlanningCommand{Action: "cancel-plan", Revision: saved.Revision, RequestID: "cancel-1", TargetRequestID: "other"}, "")
	require.ErrorContains(t, err, "already used")

	// Cancel the remaining pending request by empty TargetRequestID, then verify empty cancel errors.
	saved, _, err = ControlPlanning(opts, PlanningCommand{Action: "cancel-plan", Revision: saved.Revision, RequestID: "cancel-other"}, "")
	require.NoError(t, err)
	assert.Equal(t, "cancelled", saved.Planning.Requests[len(saved.Planning.Requests)-2].Status)
	_, _, err = ControlPlanning(opts, PlanningCommand{Action: "cancel-plan", Revision: saved.Revision, RequestID: "cancel-empty"}, "")
	require.ErrorContains(t, err, "no pending or running")

	// Enqueue milestone M-02 retry with continue-execution: true.
	saved, receipt, err = ControlPlanning(opts, PlanningCommand{Action: "plan", Revision: saved.Revision, RequestID: "retry-m02-exec", Scope: "milestone", ScopeID: "M-02", ContinueExecution: true}, "")
	require.NoError(t, err)
	assert.Equal(t, "applied", receipt.Status)
	lastReq := saved.Planning.Requests[len(saved.Planning.Requests)-1]
	assert.Equal(t, "milestone", lastReq.Scope)
	assert.Equal(t, "M-02", lastReq.ScopeID)
	assert.False(t, lastReq.PlanningOnly)

	// Cancel without TargetRequestID cancels the pending request cleanly; also cover approved/awaiting-design-approval/cancelled phase recovery.
	saved, _, err = ControlPlanning(opts, PlanningCommand{Action: "cancel-plan", Revision: saved.Revision, RequestID: "cancel-2"}, "")
	require.NoError(t, err)
	assert.Equal(t, "ready", saved.Planning.Phase)

	// Phase recovery branches: approved (no active/milestones), awaiting-design-approval (unapproved), cancelled (no review).
	for _, tc := range []struct {
		name      string
		mutate    func(*state.State)
		wantPhase string
	}{
		{
			name: "approved-without-milestones",
			mutate: func(s *state.State) {
				s.Planning.Active = nil
				s.Milestones = map[string]state.MilestoneState{}
				s.Planning.InputRevision = 2
				s.Planning.Requests = append(s.Planning.Requests,
					planning.Request{ID: "p-old", Scope: "design", InputRevision: 1, MaxRounds: 3, Status: "pending"},
					planning.Request{ID: "p-a", Scope: "design", InputRevision: 2, MaxRounds: 3, Status: "pending"},
				)
			},
			wantPhase: "approved",
		},
		{
			name: "awaiting-design-approval",
			mutate: func(s *state.State) {
				s.Planning.Approvals = nil
				s.Planning.StartAuthorized = false
				s.Planning.Requests = append(s.Planning.Requests, planning.Request{ID: "p-b", Scope: "design", InputRevision: 1, MaxRounds: 3, Status: "pending"})
			},
			wantPhase: "awaiting-design-approval",
		},
		{
			name: "cancelled-no-review",
			mutate: func(s *state.State) {
				s.Planning.CurrentDesign = nil
				s.Planning.Approvals = nil
				s.Planning.StartAuthorized = false
				s.Planning.Requests = append(s.Planning.Requests, planning.Request{ID: "p-c", Scope: "design", InputRevision: 1, MaxRounds: 3, Status: "pending"})
			},
			wantPhase: "cancelled",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := jsonClone(saved)
			tc.mutate(s)
			require.NoError(t, applyPlanningCommand(opts, s, PlanningCommand{Action: "cancel-plan", RequestID: "c-" + tc.name}, ""))
			assert.Equal(t, tc.wantPhase, s.Planning.Phase)
		})
	}

	// RetryTarget fallback to CurrentMilestone when no failed request exists.
	noFailed := jsonClone(saved.Planning)
	noFailed.Requests = nil
	rtFallback := resolvePlanningRetryTarget(noFailed, "M-03")
	assert.Equal(t, "milestone", rtFallback.Scope)
	assert.Equal(t, "M-03", rtFallback.ScopeID)
	assert.True(t, rtFallback.ContinueExecution)
}

func TestPillar2UnifiedConvergenceKernelAndPlanningGrace(t *testing.T) {
	// Exhausted grace budget.
	assert.False(t, EvaluateConvergence([]ConvergenceRound{{OpenBlockers: 5}, {OpenBlockers: 1}}, 2, 2).AllowGrace)
	// Insufficient history.
	assert.False(t, EvaluateConvergence([]ConvergenceRound{{OpenBlockers: 1}}, 0, 2).AllowGrace)
	// Case A: monotonic drop to <=2.
	decA := EvaluateConvergence([]ConvergenceRound{{OpenBlockers: 4}, {OpenBlockers: 1, OpenIDs: []string{"F-1"}}}, 0, 2)
	assert.True(t, decA.AllowGrace)
	assert.False(t, decA.Stalled)
	// Case B: all prior open IDs cleared and 1 new localized finding remains (< first).
	decB := EvaluateConvergence([]ConvergenceRound{
		{OpenBlockers: 3, OpenMajors: 2, OpenIDs: []string{"A", "B", "C", "D", "E"}},
		{OpenBlockers: 1, OpenIDs: []string{"A"}},
		{OpenBlockers: 1, OpenIDs: []string{"NEW-PROBE"}},
	}, 0, 2)
	assert.True(t, decB.AllowGrace)
	// Stalled: same finding repeats.
	decS := EvaluateConvergence([]ConvergenceRound{
		{OpenBlockers: 2, OpenIDs: []string{"D-1", "D-2"}},
		{OpenBlockers: 1, OpenIDs: []string{"D-1"}},
		{OpenBlockers: 1, OpenIDs: []string{"D-1"}},
	}, 0, 2)
	assert.False(t, decS.AllowGrace)
	assert.True(t, decS.Stalled)
	assert.False(t, allPriorOpenIDsCleared(nil, []string{"X"}))
	assert.True(t, evaluateExecutionConvergenceGrace(true, 1, 3, 3, false))
	assert.False(t, evaluateExecutionConvergenceGrace(false, 1, 3, 3, false))

	// End-to-end planning loop: 3 rounds revise (5 -> 1 -> 1 new) triggers +1 convergence grace and passes on round 4!
	fx, opts, calls := briefEngineFixture(t, false)
	reviewRound := 0
	opts.Drivers.DesignReview = &fnDriver{role: "design-review", base: drivers.NewMockDriver(nil), log: &scriptLog{}, fn: func(a drivers.CallArgs) drivers.Result {
		reviewRound++
		switch reviewRound {
		case 1:
			return planningJSONResult(t, prompts.DesignReviewOutput{
				Verdict: "revise", Summary: "Round 1 has 3 blockers",
				Findings: []planning.Finding{
					{ID: "D-1", Severity: "blocker", Target: "a", Evidence: "e", Impact: "i", Status: "open"},
					{ID: "D-2", Severity: "major", Target: "b", Evidence: "e", Impact: "i", Status: "open"},
					{ID: "D-3", Severity: "minor", Target: "c", Evidence: "e", Impact: "i", Status: "open"},
				},
			})
		case 2:
			return planningJSONResult(t, prompts.DesignReviewOutput{
				Verdict: "revise", Summary: "Round 2 resolved D-2",
				Findings: []planning.Finding{
					{ID: "D-1", Severity: "blocker", Target: "a", Evidence: "e", Impact: "i", Status: "open"},
					{ID: "D-2", Severity: "major", Target: "b", Evidence: "e", Impact: "i", Status: "resolved", Resolution: "fixed"},
				},
			})
		case 3:
			return planningJSONResult(t, prompts.DesignReviewOutput{
				Verdict: "revise", Summary: "Round 3 cleared D-1, 1 tiny new interface note",
				Findings: []planning.Finding{
					{ID: "D-1", Severity: "blocker", Target: "a", Evidence: "e", Impact: "i", Status: "resolved", Resolution: "fixed"},
					{ID: "D-4", Severity: "blocker", Target: "probe", Evidence: "e", Impact: "i", Status: "open"},
				},
			})
		default:
			return planningJSONResult(t, prompts.DesignReviewOutput{
				Verdict: "pass", Summary: "All resolved on grace round 4",
				Findings: []planning.Finding{
					{ID: "D-4", Severity: "blocker", Target: "probe", Evidence: "e", Impact: "i", Status: "resolved", Resolution: "fixed"},
				},
			})
		}
	}}
	opts.PlanningOnly = true
	_, err := RunTask(opts)
	require.NoError(t, err)
	st := loadFixtureState(t, fx)
	assert.Equal(t, "awaiting-design-approval", st.Planning.Phase)
	assert.Equal(t, 4, calls["design-plan"])
	assert.Equal(t, 4, st.Planning.Requests[0].Rounds)
	assert.Equal(t, 1, st.Planning.Requests[0].GraceRounds)

	// Plan-scope convergence history lookup covers non-design scope, truncation to req.Rounds, and non-revise skipping.
	planRef1 := planning.Ref{Revision: 1, Digest: strings.Repeat("1", 64)}
	planRef2 := planning.Ref{Revision: 2, Digest: strings.Repeat("2", 64)}
	ps := &planning.State{
		Plans: []planning.ExecutionPlan{
			{Ref: planRef1, InputRevision: 1, Scope: "milestone", ScopeID: "M-02"},
			{Ref: planRef2, InputRevision: 1, Scope: "milestone", ScopeID: "M-02"},
		},
		Reviews: []planning.Review{
			{ID: "r0", Kind: "plan", Target: planRef1, InputRevision: 1, Verdict: "pass"},
			{ID: "r1", Kind: "plan", Target: planRef1, InputRevision: 1, Verdict: "revise", Findings: []planning.Finding{{ID: "P-1", Severity: "blocker", Target: "t1", Status: "open"}}},
			{ID: "r2", Kind: "plan", Target: planRef2, InputRevision: 1, Verdict: "revise", Findings: []planning.Finding{{ID: "P-2", Severity: "major", Target: "t2", Status: "open"}}},
		},
	}
	hist, openLabels := planningConvergenceHistory(ps, planning.Request{Scope: "milestone", ScopeID: "M-02", InputRevision: 1, Rounds: 2})
	assert.Len(t, hist, 2)
	assert.Equal(t, []string{"P-2(major): t2"}, openLabels)
	// Include pass review within window to cover non-revise skip branch.
	histAll, _ := planningConvergenceHistory(ps, planning.Request{Scope: "milestone", ScopeID: "M-02", InputRevision: 1, Rounds: 3})
	assert.Len(t, histAll, 2)
	// Empty reviews returns nil.
	emptyHist, emptyOpen := planningConvergenceHistory(&planning.State{}, planning.Request{Scope: "design", InputRevision: 1})
	assert.Nil(t, emptyHist)
	assert.Nil(t, emptyOpen)
}

func TestPillar3SingleSourceProjectContextAndToolchainFacts(t *testing.T) {
	cmds := make([]any, 0, 14)
	for i := 0; i < 14; i++ {
		cmds = append(cmds, map[string]any{"cmd": fmt.Sprintf("npm run check-%02d", i), "pass": true})
	}
	st := &state.State{
		Planning: &planning.State{
			Brief:         planning.Brief{Goal: "Build resilient app"},
			CurrentDesign: &planning.Ref{Revision: 1, Digest: strings.Repeat("a", 64)},
			Designs: []planning.Design{{
				Ref:        planning.Ref{Revision: 1, Digest: strings.Repeat("a", 64)},
				Milestones: []planning.Milestone{{ID: "M-01", Title: "Foundation", Gate: "L2"}},
			}},
		},
		Milestones: map[string]state.MilestoneState{
			"M-01": {Status: "closed", HumanSignoff: &state.PacketSignoff{Reason: "verified"}},
			"M-02": {Status: "closed", Gate: map[string]any{"judge": map[string]any{"score": 0.94}}},
			"M-03": {Status: "closed"},
			"M-04": {Status: "in-progress"},
		},
		Packets: map[string]*state.PacketState{
			"M01-T01": {Status: "completed", Evidence: state.Evidence{L1: map[string]any{"pass": true, "commands": cmds}}},
			"M01-T02": {Status: "pending"},
		},
	}
	rendered := string(renderProjectContext(st, "npm ci"))
	assert.Contains(t, rendered, "## Delivered Milestones")
	assert.Contains(t, rendered, "- M-01 (Foundation): packets=[M01-T01], outcome=gate pass, human-accepted (verified)")
	assert.Contains(t, rendered, "- M-02 (M-02): packets=[], outcome=gate pass, judge 0.94")
	assert.Contains(t, rendered, "- M-03 (M-03): packets=[], outcome=closed")
	assert.NotContains(t, rendered, "- M-04")
	assert.Contains(t, rendered, "## Verified Workspace & Toolchain Facts")
	assert.Contains(t, rendered, "- verify.prepare-cmd: npm ci")
	assert.Contains(t, rendered, "npm run check-00")
	assert.NotContains(t, rendered, "npm run check-13", "verified commands list must stay bounded at 12")

	// checkPlanningExports(true) preserves delivered milestones deterministically instead of wiping them.
	fx, c := newReviewedFixture(t)
	c.Config = &config.Config{Verify: config.VerifyConfig{PrepareCmd: "npm ci"}}
	c.State.Milestones["M-01"] = state.MilestoneState{Status: "closed", HumanSignoff: &state.PacketSignoff{Reason: "accepted"}}
	require.NoError(t, c.checkPlanningExports(true))
	first := string(mustReadFile(t, filepath.Join(fx.taskDir, "project-context.md")))
	assert.Contains(t, first, "## Delivered Milestones")
	assert.Contains(t, first, "accepted")
	require.NoError(t, c.checkPlanningExports(true))
	second := string(mustReadFile(t, filepath.Join(fx.taskDir, "project-context.md")))
	assert.Equal(t, first, second)
	// verifyPrepareCmd with nil config returns empty.
	assert.Equal(t, "", (&Context{}).verifyPrepareCmd())
}
