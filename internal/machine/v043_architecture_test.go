package machine

import (
	"fmt"
	"os"
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
	// Exhausted grace safety ceiling.
	assert.False(t, EvaluateConvergence(ConvergenceInput{History: []ConvergenceRound{{OpenBlockers: 5}, {OpenBlockers: 1}}, GraceUsed: 3, MaxGrace: 3}).AllowGrace)
	// Insufficient history without prior non-review retries.
	assert.False(t, EvaluateConvergence(ConvergenceInput{History: []ConvergenceRound{{OpenBlockers: 1}}, GraceUsed: 0, MaxGrace: 3}).AllowGrace)
	// First review after prior L1 retries gets first-review-grace via the unified kernel.
	decFirst := EvaluateConvergence(ConvergenceInput{History: []ConvergenceRound{{OpenMajors: 3, OpenIDs: []string{"F-1", "F-2", "F-3"}}}, GraceUsed: 0, MaxGrace: 3, PriorNonReviewFailures: 2})
	assert.True(t, decFirst.AllowGrace)
	assert.Equal(t, "first-review-grace", decFirst.Policy)
	// Case A: monotonic drop (including 5 -> 3 without an arbitrary <=2 cliff).
	decA := EvaluateConvergence(ConvergenceInput{History: []ConvergenceRound{{OpenBlockers: 5}, {OpenBlockers: 3, OpenIDs: []string{"F-1", "F-4", "F-5"}}}, GraceUsed: 0, MaxGrace: 3})
	assert.True(t, decA.AllowGrace)
	assert.Equal(t, "convergence-grace", decA.Policy)
	assert.False(t, decA.Stalled)
	// Case B: all prior open IDs cleared and 1 new localized finding remains (< first).
	decB := EvaluateConvergence(ConvergenceInput{
		History: []ConvergenceRound{
			{OpenBlockers: 3, OpenMajors: 2, OpenIDs: []string{"A", "B", "C", "D", "E"}},
			{OpenBlockers: 1, OpenIDs: []string{"A"}},
			{OpenBlockers: 1, OpenIDs: []string{"NEW-PROBE"}},
		},
		GraceUsed: 0,
		MaxGrace:  3,
	})
	assert.True(t, decB.AllowGrace)
	// Stalled: same finding repeats.
	decS := EvaluateConvergence(ConvergenceInput{
		History: []ConvergenceRound{
			{OpenBlockers: 2, OpenIDs: []string{"D-1", "D-2"}},
			{OpenBlockers: 1, OpenIDs: []string{"D-1"}},
			{OpenBlockers: 1, OpenIDs: []string{"D-1"}},
		},
		GraceUsed: 0,
		MaxGrace:  3,
	})
	assert.False(t, decS.AllowGrace)
	assert.True(t, decS.Stalled)
	assert.False(t, allPriorOpenIDsCleared(nil, []string{"X"}))

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

func TestV0431SystemInvariantsRetrySupersessionInboxL1AnchorContextAndConciseMemory(t *testing.T) {
	// 1. resolvePlanningRetryTarget never resurrects cancelled requests or failures superseded by completed requests.
	designRef := planning.Ref{Revision: 1, Digest: strings.Repeat("a", 64)}
	p := &planning.State{
		CurrentDesign:   &designRef,
		StartAuthorized: true,
		Approvals:       []planning.Approval{{Kind: "human", Design: designRef, Start: true}},
		Requests: []planning.Request{
			{ID: "engine-1-14", Scope: "milestone", ScopeID: "M-02", Status: "budget-stopped"},
			{ID: "plan-retry-mistake", Scope: "design", Status: "cancelled"},
			{ID: "retry-m02", Scope: "milestone", ScopeID: "M-02", Status: "completed"},
		},
	}
	rt := resolvePlanningRetryTarget(p, "M-02")
	assert.Equal(t, "milestone", rt.Scope)
	assert.Equal(t, "M-02", rt.ScopeID)
	assert.True(t, rt.ContinueExecution)
	assert.Empty(t, rt.FailedRequestID)

	// 2. compactInboxLastFeedback retains failed-commands & bounded failing L1 output (capped at 4), handles non-map values.
	assert.Equal(t, "plain", compactInboxLastFeedback("plain"))
	longOut := strings.Repeat("x", 500)
	cmds := []any{
		"non-map",
		map[string]any{"cmd": "npm run typecheck", "pass": true, "exitCode": 0},
		map[string]any{"cmd": "npm run test:coverage", "pass": false, "exitCode": 1, "output": longOut},
		map[string]any{"cmd": "cmd2", "pass": false, "exitCode": 2, "output": "e2"},
		map[string]any{"cmd": "cmd3", "pass": false, "exitCode": 3, "output": "e3"},
		map[string]any{"cmd": "cmd4", "pass": false, "exitCode": 4, "output": "e4"},
		map[string]any{"cmd": "cmd5", "pass": false, "exitCode": 5, "output": "e5"},
	}
	stInbox := &state.State{
		Packets: map[string]*state.PacketState{
			"M02-T01": {
				Status: "inbox",
				LastFeedback: map[string]any{
					"error-code":      "L1_FAILED",
					"failed-commands": []any{"npm run test:coverage"},
					"l1":              map[string]any{"commands": cmds},
				},
			},
		},
	}
	proj, err := ConsoleProjection(stInbox)
	require.NoError(t, err)
	lf := proj.Packets["M02-T01"].LastFeedback.(map[string]any)
	assert.Equal(t, []any{"npm run test:coverage"}, lf["failed-commands"])
	details := lf["failed-l1-details"].([]any)
	require.Len(t, details, 4)
	firstDetail := details[0].(map[string]any)
	assert.Equal(t, "npm run test:coverage", firstDetail["cmd"])
	assert.Contains(t, firstDetail["output"].(string), "[truncated]")

	// 3. normalizeContextRef strips #anchor fragments.
	assert.Equal(t, "docs/slug/spec.md", normalizeContextRef(" docs/slug/spec.md#M-02 "))
	assert.Equal(t, "", normalizeContextRef("#only-anchor"))

	// 4. isConciseToolchainCommand filters long/multiline/inline-eval commands.
	assert.True(t, isConciseToolchainCommand("npm run test:coverage"))
	assert.False(t, isConciseToolchainCommand(""))
	assert.False(t, isConciseToolchainCommand("line1\nline2"))
	assert.False(t, isConciseToolchainCommand(strings.Repeat("a", 161)))
	assert.False(t, isConciseToolchainCommand("node -e 'console.log(1)'"))
	assert.False(t, isConciseToolchainCommand("python3 -c 'print(1)'"))

	// 5. packetEscalationFeedback never returns literal "null" and compacts verbose L1 command output.
	assert.Equal(t, "{}", packetEscalationFeedback(nil))
	assert.Equal(t, "{}", packetEscalationFeedback(&state.PacketState{}))
	assert.Contains(t, packetEscalationFeedback(&state.PacketState{Evidence: state.Evidence{Review: map[string]any{"verdict": "return"}}}), `"verdict":"return"`)
	assert.Contains(t, packetEscalationFeedback(&state.PacketState{LastFeedback: map[string]any{"error-code": "L1_FAILED"}}), `"L1_FAILED"`)
	hugeL1Feedback := packetEscalationFeedback(&state.PacketState{
		LastFeedback: map[string]any{
			"error-code":      "L1_FAILED",
			"failed-commands": []any{"npm run test:coverage"},
			"l1":              map[string]any{"commands": cmds},
		},
	})
	assert.Contains(t, hugeL1Feedback, `"failed-l1-details"`)
	assert.Contains(t, hugeL1Feedback, "[truncated]")
	assert.Less(t, len(hugeL1Feedback), 4000, "escalation feedback must stay compact instead of embedding raw L1 dumps")
}

func TestV0433FirstReviewGraceAndWirePromptBytes(t *testing.T) {
	// 1. First-review grace: when prior failures were L1-only (2 L1 failures) and
	//    Review #1 returns on attempt 3/3 (reviewReturnCount == 1, even with >2 open
	//    findings where convergence-grace would NOT fire), Execute receives +1 dedicated
	//    first-review-grace fix attempt instead of escalating to inbox on its very first review.
	fx := newGitFixture(t)
	sd := happyDrivers()
	packetText := strings.Replace(
		fixturePacket("M01-T01", "R-01"),
		`    - {cmd: "test -f build/M01-T01.txt", expect: "exit 0"}`,
		"    - {cmd: \"test -f build/M01-T01.txt\", expect: \"exit 0\"}\n"+
			"    - {cmd: \"test -f build/l1-flag\", expect: \"exit 0\"}", 1)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), packetText)

	m01ExecCalls := 0
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		pid := "M01-T01"
		if strings.Contains(a.Prompt, "packet-id: M01-T02") {
			pid = "M01-T02"
		}
		if pid == "M01-T01" {
			m01ExecCalls++
			writeFile(t, filepath.Join(a.Cwd, "build", "M01-T01.txt"), strings.Repeat("v", m01ExecCalls))
			if m01ExecCalls >= 3 {
				writeFile(t, filepath.Join(a.Cwd, "build", "l1-flag"), "ready")
			}
			writeDoneReportFile(t, a.TaskDir, pid)
			return drivers.MockResult(nil)
		}
		return sd.mock.Execute(a)
	}
	m01RevCalls := 0
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		if strings.Contains(a.Prompt, "M01-T01") {
			m01RevCalls++
			if m01RevCalls == 1 {
				return textResult(`{"verdict":"return","score":0.5,"reasons":["fix F-1..F-3"],"findings":[` +
					`{"id":"F-1","severity":"major","target":"a.ts:1","evidence":"e1","fix":"f1","status":"open"},` +
					`{"id":"F-2","severity":"major","target":"a.ts:2","evidence":"e2","fix":"f2","status":"open"},` +
					`{"id":"F-3","severity":"major","target":"a.ts:3","evidence":"e3","fix":"f3","status":"open"}]}`)
			}
			return passVerdict()
		}
		return passVerdict()
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "completed", ps.Status)
	assert.Equal(t, 4, m01ExecCalls, "Execute must receive +1 first-review-grace attempt after 2 L1 failures + 1st review return")
	assert.Equal(t, 2, m01RevCalls)
	assert.True(t, gracePolicyUsed(ps, "first-review-grace"))
	assert.Equal(t, 0, ps.PlanRevisions)

	// 2. recordSpawnMetrics records WirePromptBytes when present instead of pre-incremental full prompt length.
	_, c := newReviewedFixture(t)
	resWithWire := drivers.Result{Settled: true, ExitCode: new(0), WirePromptBytes: 42}
	c.recordSpawnMetrics(spawnMetricsArgs{
		role: "execute", packetID: "M01-T01", model: "m", channel: "ch",
		prompt: strings.Repeat("x", 500), result: &resWithWire,
	})
	rawMetrics, err := os.ReadFile(filepath.Join(c.TaskDir, "metrics.jsonl"))
	require.NoError(t, err)
	assert.Contains(t, string(rawMetrics), `"prompt-bytes":42`)
}
