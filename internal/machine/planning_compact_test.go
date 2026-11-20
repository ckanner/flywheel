package machine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func compactEngineFixture(t *testing.T, delegated bool) (fixture, RunTaskOpts, map[string]int, prompts.DesignOutput) {
	t.Helper()
	fx := newQuickFixture(t, false)
	raw, err := os.ReadFile(filepath.Join(fx.taskDir, "spec.md"))
	require.NoError(t, err)
	spec, err := contract.ParseSpec(raw, "spec.md")
	require.NoError(t, err)
	files := map[string]string{"spec.md": string(raw), "task-packet-M01-T01.md": strings.Replace(fixturePacket("M01-T01", "R-01"), "route: default", "route: default\nschedule: {after: [], writes: [build/M01-T01.txt]}", 1)}
	for name := range files {
		require.NoError(t, os.Remove(filepath.Join(fx.taskDir, name)))
	}
	gitCmd(t, fx.root, "add", "-u")
	gitCmd(t, fx.root, "commit", "-qm", "test: compact brief before projection")
	brief := planning.Brief{Version: 1, ID: fx.slug, Goal: spec.Goal, OriginalRequest: "Implement the localized artifact and verify it", Constraints: []string{}, Evidence: []string{"existing artifact test"}, Policy: "review-first", Mode: "auto", DesignGranularity: "compact", GranularityReason: "One independent local artifact with executable regression"}
	if delegated {
		brief.Policy = "delegated"
		brief.ImplementationAuthorized = true
		brief.Authorization = brief.OriginalRequest
	}
	st, err := state.InitPlanning(brief, mustConfig(t, fx))
	require.NoError(t, err)
	st.Project = &state.ProjectIdentity{Cwd: fx.root, WorkspaceCwd: fx.root}
	_, err = state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	output := engineDesignOutput(t)
	output.Requirements = output.Requirements[:1]
	output.Compact = &prompts.CompactOutput{Version: 1, Files: files}
	sd := quickDrivers()
	drv := sd.set()
	calls := map[string]int{}
	drv.DesignPlan = &fnDriver{role: "design-plan", base: sd.mock, log: sd.log, fn: func(a drivers.CallArgs) drivers.Result {
		calls["design-plan"]++
		require.Contains(t, a.Prompt, "compact")
		return planningJSONResult(t, output)
	}}
	drv.DesignReview = &fnDriver{role: "design-review", base: sd.mock, log: sd.log, fn: func(a drivers.CallArgs) drivers.Result {
		calls["design-review"]++
		require.Contains(t, a.Prompt, "compact-execution-plan")
		return planningJSONResult(t, prompts.DesignReviewOutput{Verdict: "pass", DesignVerdict: "valid", CoverageVerdict: "complete", ScopeVerdict: "within-envelope", Summary: "Exact design and executable checks cover the complete task", Findings: []planning.Finding{}})
	}}
	drv.Plan = &fnDriver{role: "plan", base: sd.mock, log: sd.log, fn: func(a drivers.CallArgs) drivers.Result {
		calls["plan"]++
		return planningJSONResult(t, prompts.PlanOutput{Files: files})
	}}
	drv.PlanReview = &fnDriver{role: "plan-review", base: sd.mock, log: sd.log, fn: func(a drivers.CallArgs) drivers.Result {
		calls["plan-review"]++
		return planningJSONResult(t, prompts.DesignReviewOutput{Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "Scoped", Findings: []planning.Finding{}})
	}}
	return fx, runOpts(fx, drv), calls, output
}

func TestCompactPreparationCompletesWithoutAnotherMilestonePlanningPair(t *testing.T) {
	fx, opts, calls, _ := compactEngineFixture(t, true)
	result, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.Equal(t, map[string]int{"design-plan": 1, "design-review": 1}, calls)
	st := loadFixtureState(t, fx)
	require.Equal(t, "quick", st.Workflow.Effective)
	require.Len(t, st.Planning.Reviews, 1)
	require.Equal(t, "compact-v1", st.Planning.Reviews[0].Kind)
	require.Len(t, st.Planning.Plans, 1)
	require.Equal(t, st.Planning.Designs[0].PlannerInvocation, st.Planning.Plans[0].PlannerInvocation)
	require.NoError(t, planning.Admit(st.Planning, planning.ActivationRefs(*st.Planning.Active)))
	_, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, 1, calls["design-plan"])
}

func TestCompactReviewFirstStillWaitsForExactApprovalAndStart(t *testing.T) {
	fx, opts, calls, _ := compactEngineFixture(t, false)
	result, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "awaiting-design-approval", result.PlanningPhase)
	require.NoFileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))
	require.NoFileExists(t, filepath.Join(fx.taskDir, "spec.md"))
	approveEngineDesign(t, fx, false, "approve-only")
	_, err = RunTask(opts)
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))
	require.Equal(t, 1, calls["design-review"])
	approveEngineDesign(t, fx, true, "start")
	result, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.Zero(t, calls["plan"])
	require.Zero(t, calls["plan-review"])
}

func TestCompactProposalValidationAndEscalationPreserveUnreviewedState(t *testing.T) {
	fx, opts, _, output := compactEngineFixture(t, true)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: mustConfig(t, fx), Drivers: opts.Drivers})
	require.NoError(t, err)
	p := c.State.Planning
	require.NoError(t, planning.BeginRequest(p, planning.Request{ID: "compact", Scope: "design", InputRevision: 1}))
	r := &p.Requests[0]
	d, err := planning.SealDesign(planning.Design{Ref: planning.Ref{Revision: 1}, InputRevision: 1, Document: output.Document, Requirements: output.Requirements, Decisions: output.Decisions, Milestones: output.Milestones, ImplementationScope: output.ImplementationScope, Baseline: "base", PlannerInvocation: "planner"})
	require.NoError(t, err)
	_, err = c.prepareCompactProjection(*r, d, nil)
	require.NoError(t, err)
	bad := *output.Compact
	bad.Version = 2
	_, err = c.prepareCompactProjection(*r, d, &bad)
	require.Error(t, err)
	bad = *output.Compact
	bad.Files = map[string]string{"spec.md": "invalid"}
	_, err = c.prepareCompactProjection(*r, d, &bad)
	require.ErrorIs(t, err, errCompactIneligible)
	bad = *output.Compact
	bad.Files = map[string]string{"spec.md": "invalid", "task-packet-M01-T01.md": "bad"}
	_, err = c.prepareCompactProjection(*r, d, &bad)
	require.Error(t, err)
	invalidDesign := d
	invalidDesign.Ref.Digest = planning.DigestBytes([]byte("changed"))
	_, err = c.prepareCompactProjection(*r, invalidDesign, output.Compact)
	require.ErrorContains(t, err, "digest")
	p.Brief.Mode = ""
	_, err = c.prepareCompactProjection(*r, d, output.Compact)
	require.NoError(t, err, "an unspecified mode retains Auto qualification")
	bad = *output.Compact
	bad.Files = map[string]string{"spec.md": output.Compact.Files["spec.md"] + strings.Repeat("x", 131073), "task-packet-M01-T01.md": output.Compact.Files["task-packet-M01-T01.md"]}
	_, err = c.prepareCompactProjection(*r, d, &bad)
	require.Error(t, err, "structurally valid Markdown must still fit the artifact protocol bound")
	oldCwd := c.Cwd
	c.Cwd = filepath.Join(t.TempDir(), "missing")
	_, err = c.prepareCompactProjection(*r, d, output.Compact)
	require.Error(t, err, "repository identity must be observable before adoption")
	c.Cwd = oldCwd
	c.planningMode = "full"
	_, err = c.prepareCompactProjection(*r, d, output.Compact)
	require.ErrorIs(t, err, errCompactIneligible)
	c.planningMode = ""
	require.Empty(t, p.Designs)
	require.Empty(t, p.Plans)
	output.Compact = nil
	require.NoError(t, c.adoptPlanningProposal(r, planningJSONResult(t, output).Text, "planner"))
	require.Equal(t, "system", r.DesignGranularity)
	require.Empty(t, p.Designs)
	require.Empty(t, r.Stage)
}

func TestCompactEngineEscalatesIneligibleProposalAndRejectsInvalidBytes(t *testing.T) {
	for _, scenario := range []string{"ineligible", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			fx, opts, _, output := compactEngineFixture(t, true)
			c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: mustConfig(t, fx), Drivers: opts.Drivers})
			require.NoError(t, err)
			p := c.State.Planning
			require.NoError(t, planning.BeginRequest(p, planning.Request{ID: "compact", Scope: "design", InputRevision: 1}))
			r := &p.Requests[0]
			output.Compact.Files = map[string]string{"spec.md": "invalid"}
			if scenario == "invalid" {
				output.Compact.Files["task-packet-M01-T01.md"] = "invalid"
			}
			err = c.adoptPlanningProposal(r, planningJSONResult(t, output).Text, "planner")
			if scenario == "ineligible" {
				require.NoError(t, err)
				require.Equal(t, "system", r.DesignGranularity)
			} else {
				require.Error(t, err)
			}
			require.Empty(t, p.Designs)
			require.Empty(t, p.Plans)
		})
	}
}

func TestCompactJointReviewRiskEscalatesAndConsoleCannotApproveMissingFiles(t *testing.T) {
	for _, verdict := range []string{"pass", "revise"} {
		t.Run(verdict, func(t *testing.T) {
			fx, opts, _, output := compactEngineFixture(t, false)
			c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: mustConfig(t, fx), Drivers: opts.Drivers})
			require.NoError(t, err)
			p := c.State.Planning
			require.NoError(t, planning.BeginRequest(p, planning.Request{ID: "joint", Scope: "design", InputRevision: 1}))
			r := &p.Requests[0]
			require.NoError(t, c.adoptPlanningProposal(r, planningJSONResult(t, output).Text, "planner"))
			review := prompts.DesignReviewOutput{Verdict: verdict, ScopeVerdict: "within-envelope", DesignVerdict: "valid", CoverageVerdict: "complete", Summary: "Review exact artifacts", Findings: []planning.Finding{}}
			if verdict == "revise" {
				review.ScopeVerdict = "unclear"
			}
			require.NoError(t, c.adoptPlanningReview(r, planningJSONResult(t, review).Text, "design-review", "independent-review"))
			if verdict == "revise" {
				require.Equal(t, "system", r.DesignGranularity)
				require.Empty(t, r.Stage)
				require.Empty(t, p.Approvals)
				return
			}
			complete, err := consolePlanningProjection(p, 1<<20)
			require.NoError(t, err)
			require.True(t, complete.Projection.ApprovalComplete)
			bounded, err := consolePlanningProjection(p, planningProjectionSize(complete)-1)
			require.NoError(t, err)
			require.False(t, bounded.Projection.ApprovalComplete)
			require.Greater(t, bounded.Projection.Omitted["plan-files"], 0)
			require.NotEmpty(t, p.Plans[0].Files, "console omission cannot rewrite the authoritative joint artifacts")
		})
	}
}
