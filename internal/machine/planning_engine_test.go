package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func briefEngineFixture(t *testing.T, delegated bool) (fixture, RunTaskOpts, map[string]int) {
	t.Helper()
	fx := newGitFixture(t)
	for _, name := range []string{"state.json", "spec.md", "task-packet-M01-T01.md", "task-packet-M01-T02.md"} {
		require.NoError(t, os.Remove(filepath.Join(fx.taskDir, name)))
	}
	gitCmd(t, fx.root, "add", "-u")
	gitCmd(t, fx.root, "commit", "-qm", "test: leave only a thin task entry")
	cfg := mustConfig(t, fx)
	brief := planning.Brief{Version: 1, ID: fx.slug, Goal: "Prove the orchestrator loop end to end.", OriginalRequest: "Design and implement a verified fixture with two independent requirements.", Constraints: []string{}, Evidence: []string{}, Policy: "review-first"}
	if delegated {
		brief.Policy = "delegated"
		brief.Authorization = "original-user-request:fixture"
		brief.ImplementationAuthorized = true
	}
	st, err := state.InitPlanning(brief, cfg)
	require.NoError(t, err)
	st.Project = &state.ProjectIdentity{Cwd: fx.root, WorkspaceCwd: fx.root}
	_, err = state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	sd := newScripted(nil)
	drv := sd.set()
	calls := map[string]int{}
	output := func(role string, value any) drivers.Driver {
		return &fnDriver{role: role, base: sd.mock, log: sd.log, fn: func(a drivers.CallArgs) drivers.Result {
			calls[role]++
			require.Contains(t, a.Prompt, fx.root)
			return planningJSONResult(t, value)
		}}
	}
	drv.DesignPlan = output("design-plan", engineDesignOutput(t))
	drv.DesignReview = output("design-review", prompts.DesignReviewOutput{Verdict: "pass", Summary: "Requirements, recovery and acceptance are explicit.", Findings: []planning.Finding{}})
	drv.Plan = output("plan", prompts.PlanOutput{Files: enginePlanFiles()})
	drv.PlanReview = output("plan-review", prompts.DesignReviewOutput{Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "Both packet boundaries and acceptance preserve the approved design.", Findings: []planning.Finding{}})
	opts := runOpts(fx, drv)
	opts.Log = func(msg string) { t.Log(msg) }
	return fx, opts, calls
}

func planningJSONResult(t *testing.T, value any) drivers.Result {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return drivers.MockResult(func(r *drivers.Result) { r.Text = string(raw) })
}

func engineDesignOutput(t *testing.T) prompts.DesignOutput {
	t.Helper()
	spec, err := contract.ParseSpec([]byte(fixtureSpec), "spec.md")
	require.NoError(t, err)
	d := prompts.DesignOutput{Document: "# Verified fixture design\nTwo packets produce separate artifacts. Missing artifacts fail real shell acceptance. The executor can recover before committing, and milestones close only after independent review.", ImplementationScope: []string{"build"}, Requirements: []planning.Requirement{}, Milestones: []planning.Milestone{}, Decisions: []planning.Decision{}}
	for _, r := range spec.Requirements {
		d.Requirements = append(d.Requirements, planning.Requirement{ID: r.ID, Title: r.Title, Milestone: r.Milestone, Verification: r.Verification})
	}
	for _, m := range spec.Milestones {
		d.Milestones = append(d.Milestones, planning.Milestone{ID: m.ID, Title: m.Title, Gate: m.Gate})
	}
	return d
}

func enginePlanFiles() map[string]string {
	return map[string]string{"spec.md": fixtureSpec,
		"task-packet-M01-T01.md": strings.Replace(fixturePacket("M01-T01", "R-01"), "route: default", "route: default\nschedule: {after: [], writes: [build/M01-T01.txt]}", 1),
		"task-packet-M01-T02.md": strings.Replace(fixturePacket("M01-T02", "R-02"), "route: default", "route: default\nschedule: {after: [M01-T01], writes: [build/M01-T02.txt]}", 1)}
}

func approveEngineDesign(t *testing.T, fx fixture, start bool, requestID string) {
	t.Helper()
	st := loadFixtureState(t, fx)
	d := *st.Planning.CurrentDesign
	r := planning.LatestReview(st.Planning, "design", d)
	require.NotNil(t, r)
	require.NoError(t, planning.Approve(st.Planning, planning.Approval{Kind: "human", Design: d, ReviewID: r.ID, RequestID: requestID, Source: "tty:fixture", At: state.NowISO(), Start: start}))
	_, err := state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
}

func TestReviewedPlanningBriefReviewApprovalAndExecutionEndToEnd(t *testing.T) {
	fx, opts, calls := briefEngineFixture(t, false)
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "awaiting-design-approval", summary.PlanningPhase)
	require.Equal(t, "paused", summary.Status)
	require.NoFileExists(t, filepath.Join(fx.taskDir, "spec.md"))
	require.NoDirExists(t, filepath.Join(fx.root, "build"))
	require.Equal(t, map[string]int{"design-plan": 1, "design-review": 1}, calls)
	before := loadFixtureState(t, fx)
	_, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, 1, calls["design-plan"], "restart at approval must not charge another design call")
	require.Equal(t, before.Budget.Totals, loadFixtureState(t, fx).Budget.Totals)
	approveEngineDesign(t, fx, false, "review-only")
	_, err = RunTask(opts)
	require.NoError(t, err)
	require.Zero(t, calls["plan"], "approve-only does not authorize implementation")
	approveEngineDesign(t, fx, true, "start-reviewed-design")
	summary, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status, "state: %+v packets: %+v", loadFixtureState(t, fx).Inbox, loadFixtureState(t, fx).Packets)
	require.Equal(t, 1, calls["plan"], "the reviewed child plan replaces the redundant legacy Plan call")
	require.Equal(t, 1, calls["plan-review"])
	require.FileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))
	require.FileExists(t, filepath.Join(fx.root, "build", "M01-T02.txt"))
	after := loadFixtureState(t, fx)
	require.Equal(t, 4, after.Sessions.Version)
	require.Equal(t, "ready", after.Planning.Phase)
	require.Nil(t, after.Planning.PendingActivation)
	require.NotEmpty(t, after.Planning.Active.Head)
	require.Len(t, after.Planning.Approvals, 2)
	require.Len(t, after.Planning.Designs, 1)
	require.Len(t, after.Planning.Plans, 1)
	require.NoError(t, planning.Admit(after.Planning, planning.ActivationRefs(*after.Planning.Active)))
}

func TestReviewedPlanningOldStartDoesNotStartAnApprovedOnlyRevision(t *testing.T) {
	fx, opts, calls := briefEngineFixture(t, false)
	_, err := RunTask(opts)
	require.NoError(t, err)
	approveEngineDesign(t, fx, true, "start-first-design")
	before := loadFixtureState(t, fx)
	_, _, err = ControlPlanning(opts.RunOpts, PlanningCommand{Action: "feedback", Revision: before.Revision,
		RequestID: "revise-before-starting", Design: before.Planning.CurrentDesign,
		Text: "Discuss the recovery tradeoffs again before proceeding with this design."}, "")
	require.NoError(t, err)
	_, err = RunTask(opts)
	require.NoError(t, err)
	st := loadFixtureState(t, fx)
	require.Equal(t, 2, st.Planning.CurrentDesign.Revision)
	approveEngineDesign(t, fx, false, "approve-second-design-only")
	before = loadFixtureState(t, fx)
	head := gitCmd(t, fx.root, "rev-parse", "HEAD")
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	require.Zero(t, calls["plan"], "starting a previous design cannot pay to refine an approve-only revision")
	require.Equal(t, before.Budget.Totals, loadFixtureState(t, fx).Budget.Totals)
	require.Equal(t, head, gitCmd(t, fx.root, "rev-parse", "HEAD"))
	require.NoFileExists(t, filepath.Join(fx.taskDir, "spec.md"))
	approveEngineDesign(t, fx, true, "start-second-design")
	summary, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.Equal(t, 1, calls["plan"])
}

func TestReviewedPlanningDelegatedAuthorizationAndExplicitPlanStop(t *testing.T) {
	for _, only := range []bool{false, true} {
		t.Run(map[bool]string{false: "delegated executes", true: "Plan always stops"}[only], func(t *testing.T) {
			fx, opts, calls := briefEngineFixture(t, true)
			opts.PlanningOnly = only
			summary, err := RunTask(opts)
			require.NoError(t, err)
			st := loadFixtureState(t, fx)
			require.Len(t, st.Planning.Approvals, 1)
			require.Equal(t, "delegated", st.Planning.Approvals[0].Kind)
			if only {
				require.Equal(t, "paused", summary.Status)
				require.Zero(t, calls["plan"])
				require.NoDirExists(t, filepath.Join(fx.root, "build"))
			} else {
				require.Equal(t, "completed", summary.Status, "state: %+v packets: %+v", loadFixtureState(t, fx).Inbox, loadFixtureState(t, fx).Packets)
			}
		})
	}
}

func TestReviewedPlanningThreeRoundLimitSurvivesRestart(t *testing.T) {
	fx, opts, calls := briefEngineFixture(t, false)
	opts.Drivers.DesignReview = &fnDriver{role: "review", base: drivers.NewMockDriver(nil), log: &scriptLog{}, fn: func(a drivers.CallArgs) drivers.Result {
		return planningJSONResult(t, prompts.DesignReviewOutput{Verdict: "revise", Summary: "The recovery behavior still needs a concrete rollback proof.", Findings: []planning.Finding{{ID: "D-1", Severity: "major", Target: "recovery", Evidence: "No rollback proof", Impact: "May lose interrupted work", Status: "open"}}})
	}}
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "budget-stopped", summary.Status)
	st := loadFixtureState(t, fx)
	require.Equal(t, 3, st.Planning.Requests[0].Rounds)
	require.Len(t, st.Planning.Designs, 3)
	require.Empty(t, st.Planning.Approvals)
	_, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, 3, calls["design-plan"])
	require.NoFileExists(t, filepath.Join(fx.taskDir, "spec.md"))
}

func TestReviewedPlanningUnknownCleanupCannotRetryOrApprove(t *testing.T) {
	fx, opts, calls := briefEngineFixture(t, false)
	opts.Drivers.DesignReview = &fnDriver{role: "review", base: drivers.NewMockDriver(nil), log: &scriptLog{}, fn: func(a drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) { r.CleanupError = "native child status unknown" })
	}}
	_, err := RunTask(opts)
	require.ErrorContains(t, err, "cleanup is unconfirmed")
	st := loadFixtureState(t, fx)
	require.Empty(t, st.Planning.Reviews)
	require.Empty(t, st.Planning.Approvals)
	require.Equal(t, 1, calls["design-plan"])
	require.Zero(t, st.Planning.Requests[0].Retries)
}

func TestReviewedPlanningGuardsFileWritesAndManualBypass(t *testing.T) {
	fx, opts, _ := briefEngineFixture(t, false)
	opts.Drivers.DesignPlan = &fnDriver{role: "plan", base: drivers.NewMockDriver(nil), log: &scriptLog{}, fn: func(a drivers.CallArgs) drivers.Result {
		writeFile(t, filepath.Join(a.Cwd, "unapproved.go"), "package forbidden\n")
		return planningJSONResult(t, engineDesignOutput(t))
	}}
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "blocked", summary.PlanningPhase)
	require.FileExists(t, filepath.Join(fx.root, "unapproved.go"), "unexpected work is preserved for human review")
	require.Empty(t, loadFixtureState(t, fx).Planning.Designs)
	for _, role := range []string{"execute", "review", "verify", "integrate", "final"} {
		single := RunSingleStepOpts{RunOpts: opts.RunOpts, Only: role, PacketID: "M01-T01"}
		_, err := RunSingleStep(single)
		require.Error(t, err, "%s cannot bypass an unapproved design", role)
	}
}

func TestReviewedPlanningNativeReceiptRecoveryWithoutRedispatch(t *testing.T) {
	fx, opts, calls := briefEngineFixture(t, false)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: opts.Drivers})
	require.NoError(t, err)
	enableSessionPolicy(c)
	for _, driver := range []drivers.Driver{opts.Drivers.DesignPlan, opts.Drivers.DesignReview, opts.Drivers.Plan, opts.Drivers.PlanReview} {
		d := driver.(*fnDriver)
		old := d.fn
		d.fn = func(a drivers.CallArgs) drivers.Result { return sessionResult(a, old(a)) }
	}
	require.NoError(t, c.checkPlanningExports(true))
	index, err := c.newPlanningRequest("design", "", "initial request", false)
	require.NoError(t, err)
	require.NoError(t, planning.BeginRound(c.State.Planning, c.State.Planning.Requests[index].ID))
	r, role, invocation, err := c.invokePlanning(&c.State.Planning.Requests[index], false)
	require.NoError(t, err)
	require.NotEmpty(t, invocation)
	require.Equal(t, "design-plan", role)
	record := c.sessionRecord(role, planningSessionScope(&c.State.Planning.Requests[index]))
	require.Equal(t, "received", record.Last.Status)
	require.True(t, record.Last.Accounted)
	originalBudget := c.State.Budget.Totals
	// Simulate loss of the caller after the durable receipt/accounting but
	// before adoption. The next engine owns and applies the original result.
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "awaiting-design-approval", summary.PlanningPhase)
	require.Equal(t, 1, calls["design-plan"])
	require.Equal(t, 1, calls["design-review"])
	st := loadFixtureState(t, fx)
	require.Equal(t, invocation, st.Planning.Designs[0].PlannerInvocation)
	require.NotEqual(t, invocation, st.Planning.Reviews[0].Invocation)
	require.Equal(t, 2, len(st.Sessions.Records), "planner and reviewer own distinct role histories")
	require.Equal(t, originalBudget.Tokens+r.Usage.TotalTokens, st.Budget.Totals.Tokens, "original planning usage is not billed again on replay")
	for _, record := range st.Sessions.Records {
		require.Equal(t, "consumed", record.Last.Status)
	}
}

func TestReviewedPlanningOutputRetriesAndFallbackRemainBounded(t *testing.T) {
	fx, opts, calls := briefEngineFixture(t, false)
	original := opts.Drivers.DesignPlan.(*fnDriver).fn
	seen := []string{}
	first := true
	opts.Drivers.DesignPlan.(*fnDriver).fn = func(a drivers.CallArgs) drivers.Result {
		seen = append(seen, a.Binding.Channel)
		if first {
			first = false
			return drivers.MockResult(func(r *drivers.Result) { code := 1; r.ExitCode = &code; r.ErrorCode = errs.RATE_LIMIT })
		}
		return original(a)
	}
	channel := opts.Config.Channels["kimi-coding"]
	channel.Models["design-plan"] = "kimi-coding/k3"
	opts.Config.Channels["kimi-coding"] = channel
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "awaiting-design-approval", summary.PlanningPhase)
	require.Equal(t, []string{"kimi-subscription", "kimi-coding"}, seen)
	require.Equal(t, 1, calls["design-plan"])
	require.Equal(t, 1, loadFixtureState(t, fx).Planning.Requests[0].Retries)
	_, err = RunTask(opts)
	require.NoError(t, err)
	require.Len(t, seen, 2)

	fx, opts, _ = briefEngineFixture(t, false)
	opts.Drivers.DesignPlan.(*fnDriver).fn = func(a drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) { r.Text = `{"approved":true}` })
	}
	summary, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "blocked", summary.PlanningPhase)
	st := loadFixtureState(t, fx)
	require.Empty(t, st.Planning.Designs)
	require.Empty(t, st.Planning.Approvals)
	require.Equal(t, opts.Config.Retry.MaxTransportRetries+planValidationRetries+1, st.Planning.Requests[0].Retries)
}

func TestReviewedPlanningDeferredMilestonesKeepCompletedPacketsClosed(t *testing.T) {
	fx, opts, calls := briefEngineFixture(t, true)
	spec := strings.Replace(fixtureSpec, "requirements:\n", "  - id: M-02\n    title: The second milestone\n    gate: Second artifact passes independent acceptance.\nrequirements:\n", 1)
	spec = strings.Replace(spec, "title: Second thing works\n    milestone: M-01", "title: Second thing works\n    milestone: M-02", 1)
	design := engineDesignOutput(t)
	design.Requirements[1].Milestone = "M-02"
	design.Milestones = append(design.Milestones, planning.Milestone{ID: "M-02", Title: "The second milestone", Gate: "Second artifact passes independent acceptance."})
	opts.Drivers.DesignPlan.(*fnDriver).fn = func(a drivers.CallArgs) drivers.Result { return planningJSONResult(t, design) }
	firstFile := enginePlanFiles()["task-packet-M01-T01.md"]
	secondFile := strings.Replace(fixturePacket("M02-T01", "R-02"), "route: default", "route: default\nschedule: {after: [M01-T01], writes: [build/M02-T01.txt]}", 1)
	// The approved R-02 verification string remains an explicit floor.
	spec = strings.Replace(spec, "verification: test -f build/M01-T02.txt", "verification: test -f build/M02-T01.txt", 1)
	design.Requirements[1].Verification = "test -f build/M02-T01.txt"
	opts.Drivers.Plan.(*fnDriver).fn = func(a drivers.CallArgs) drivers.Result {
		calls["plan"]++
		files := map[string]string{"spec.md": spec, "task-packet-M01-T01.md": firstFile}
		if a.MilestoneID == "M-01" {
			files["spec.md"] = strings.Replace(spec, "title: The second milestone", "planning: deferred\n    title: The second milestone", 1)
		} else {
			require.Equal(t, "M-02", a.MilestoneID)
			st := loadFixtureState(t, fx)
			require.Equal(t, "completed", st.Packets["M01-T01"].Status)
			files["task-packet-M02-T01.md"] = secondFile
		}
		return planningJSONResult(t, prompts.PlanOutput{Files: files})
	}
	summary, err := RunTask(opts)
	require.NoError(t, err)
	st := loadFixtureState(t, fx)
	require.Equal(t, "completed", summary.Status, "inbox: %+v", st.Inbox)
	require.Equal(t, 2, calls["plan"])
	require.Equal(t, 2, calls["plan-review"])
	require.Equal(t, 1, st.Packets["M01-T01"].Attempts, "expanding a deferred milestone must not reopen completed work")
	require.Equal(t, "closed", st.Milestones["M-01"].Status)
	require.Equal(t, "closed", st.Milestones["M-02"].Status)
	require.Len(t, st.Planning.Active.Plans, 2)
	require.Contains(t, st.Planning.Plans[0].Files["spec.md"], "planning: deferred")
	require.FileExists(t, filepath.Join(fx.root, "build", "M02-T01.txt"))
}

func TestReviewedPlanningRetainsQuickQualification(t *testing.T) {
	fx, opts, calls := briefEngineFixture(t, true)
	spec := strings.Replace(fixtureSpec, "spec-version: 1\n", "spec-version: 1\n"+quickAssessment, 1)
	start := strings.Index(spec, "  - id: R-02")
	end := strings.Index(spec[start:], "---\n") + start
	spec = spec[:start] + spec[end:]
	design := engineDesignOutput(t)
	design.Requirements = design.Requirements[:1]
	opts.Drivers.DesignPlan.(*fnDriver).fn = func(a drivers.CallArgs) drivers.Result { return planningJSONResult(t, design) }
	opts.Drivers.Plan.(*fnDriver).fn = func(a drivers.CallArgs) drivers.Result {
		calls["plan"]++
		return planningJSONResult(t, prompts.PlanOutput{Files: map[string]string{"spec.md": spec, "task-packet-M01-T01.md": enginePlanFiles()["task-packet-M01-T01.md"]}})
	}
	opts.Drivers.Review = quickDrivers().set().Review
	summary, err := RunTask(opts)
	require.NoError(t, err)
	st := loadFixtureState(t, fx)
	require.Equal(t, "completed", summary.Status, "inbox: %+v", st.Inbox)
	require.Equal(t, "quick", st.Workflow.Effective)
	require.Empty(t, st.Workflow.Upgrades)
	require.Equal(t, 1, calls["plan"], "one reviewed preparation plan, no redundant Full Plan stage")
}
