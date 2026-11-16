package diagnostic

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bindingByID(t *testing.T, view *Configuration, id string) Binding {
	t.Helper()
	for _, b := range view.Bindings {
		if b.ID == id {
			return b
		}
	}
	t.Fatalf("missing binding %q in %+v", id, view.Bindings)
	return Binding{}
}

func TestPlanningInspectionShowsSourceWithoutSharingRoleHistoryOrPermissions(t *testing.T) {
	root, _ := diagnosticFixture(t, `{
 "sessions":{"mode":"scoped","policy-version":1,"roles":{"plan":"fresh","review":"fresh"}},
 "permissions":{"mode":"role-based","bindings":{"loops.review":"danger-full-access"}},
 "native-profiles":{"team":{"harness":"dsh","options":{"version":1,"preset":"heavy","agents":{"enabled":true},"team":{"enabled":true}}}},
 "profiles":{"planner":{"harness":"kimi","model":"kimi-code/k3.1-pro","channel":"kimi-subscription"},"reviewer":{"harness":"dsh","provider":"chatgpt","model":"review-model","channel":"account","thinking":"high","native-profile":"team"}},
 "loops":{"plan":{"profile":"planner","resident-session":true},"review":{"profile":"reviewer"},"plan-review":{"profile":"reviewer","native-profile":"","thinking":"low"}}
}`)
	view, err := Inspect(root, "")
	require.NoError(t, err)
	require.Len(t, view.Bindings, 5)
	planner := bindingByID(t, view, "loops.design-plan")
	assert.Equal(t, "loops.plan", planner.BindingSource)
	assert.Equal(t, "planner", planner.Profile)
	assert.Equal(t, "scoped", planner.Session.Mode)
	assert.False(t, planner.ResidentSession, "inheriting a backend never reuses the legacy Plan session")
	reviewer := bindingByID(t, view, "loops.design-review")
	assert.Equal(t, "loops.review", reviewer.BindingSource)
	assert.Equal(t, "reviewer", reviewer.Profile)
	assert.Equal(t, "team", reviewer.NativeProfile)
	assert.Equal(t, "read-only", reviewer.PermissionMode, "review binding permission does not widen independent roles")
	assert.Equal(t, "scoped", reviewer.Session.Mode)
	assert.False(t, reviewer.Session.HistoryRecoveryTested)
	explicit := bindingByID(t, view, "loops.plan-review")
	assert.Equal(t, "loops.plan-review", explicit.BindingSource)
	assert.Empty(t, explicit.NativeProfile, "an explicit native opt-out survives profile inheritance")
	assert.Equal(t, "low", explicit.Thinking)
	assert.Equal(t, "fresh", bindingByID(t, view, "loops.review").Session.Mode)
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"bindingSource":"loops.plan"`)
	assert.NoFileExists(t, state.StateFile(root))
}

func TestPlanningDoctorDoesNotSilentlyWidenAnInheritedClaudeReview(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{
 "permissions":{"mode":"role-based","bindings":{"loops.review":"danger-full-access"}},
 "drivers":{"review":"mock","design-review":"real","plan-review":"real"},
 "loops":{"review":{"harness":"claude-code-pro","model":"opus","channel":"subscription"}}
}`)
	report := Doctor(context.Background(), root, "", true)
	assert.False(t, report.OK)
	assert.Equal(t, "skipped", checkByID(t, report, "loops.review").Status)
	for _, role := range []string{"design-review", "plan-review"} {
		check := checkByID(t, report, "loops."+role+".permission")
		assert.Equal(t, "fail", check.Status)
		assert.Contains(t, check.Remedy, "permissions.bindings.loops."+role)
	}
	assert.Empty(t, runtime.probes)
	for _, call := range runtime.commands {
		assert.Equal(t, "git", call.binary, "unsupported permission must fail before worker initialization")
	}
}

func diagnosticPlanningState(t *testing.T) *state.State {
	t.Helper()
	st, err := state.InitPlanning(planning.Brief{Version: 1, ID: "task", Goal: "Review a system design", OriginalRequest: "Discuss and review before implementation", Policy: "review-first", Constraints: []string{}, Evidence: []string{}}, nil)
	require.NoError(t, err)
	return st
}

func TestDoctorAcceptsBriefOnlyPlanningWithoutInventingImplementationContracts(t *testing.T) {
	for _, config := range []string{dshConfiguration, `{}`} {
		t.Run(config, func(t *testing.T) {
			root, runtime := diagnosticFixture(t, config)
			task := filepath.Join(root, "docs", "task")
			writeFixture(t, filepath.Join(task, "brief.md"), "Discuss the proposed system before implementation.\n")
			_, err := state.Save(task, diagnosticPlanningState(t), nil)
			require.NoError(t, err)
			before, err := os.ReadFile(state.StateFile(task))
			require.NoError(t, err)
			report := Doctor(context.Background(), root, task, true)
			assert.Equal(t, "pass", checkByID(t, report, "planning").Status)
			assert.Equal(t, "skipped", checkByID(t, report, "contracts").Status)
			assert.Equal(t, config != `{}`, report.OK)
			if config == `{}` {
				assert.Equal(t, "fail", checkByID(t, report, "loops.design-plan").Status)
				assert.Equal(t, "fail", checkByID(t, report, "loops.design-review").Status)
				assert.Empty(t, runtime.probes)
			} else {
				for _, b := range report.Configuration.Bindings {
					assert.Equal(t, b.Role == "design-plan" || b.Role == "design-review", b.InScope, b.ID)
				}
				require.Len(t, runtime.probes, 2)
			}
			after, err := os.ReadFile(state.StateFile(task))
			require.NoError(t, err)
			assert.Equal(t, before, after)
			assert.NoFileExists(t, filepath.Join(task, "spec.md"))
			assert.NoFileExists(t, filepath.Join(task, "task-packet-M01-T01.md"))
		})
	}
}

func TestPlanningScopeSeparatesDesignRefinementAndPendingActivation(t *testing.T) {
	for _, tc := range []struct {
		name, phase, scope, planner, reviewer        string
		currentPlan, active, pending, implementation bool
	}{
		{name: "new brief", phase: "drafting", planner: "design-plan", reviewer: "design-review"},
		{name: "human design checkpoint", phase: "awaiting-design-approval", scope: "design", planner: "design-plan", reviewer: "design-review"},
		{name: "approved design needs refinement", phase: "approved", planner: "plan", reviewer: "plan-review"},
		{name: "reviewing a plan", phase: "reviewing", currentPlan: true, planner: "plan", reviewer: "plan-review"},
		{name: "milestone request", phase: "drafting", scope: "milestone", planner: "plan", reviewer: "plan-review"},
		{name: "packet request", phase: "drafting", scope: "packet", active: true, implementation: true, planner: "plan", reviewer: "plan-review"},
		{name: "design revision retains active contracts", phase: "drafting", scope: "design", active: true, implementation: true, planner: "design-plan", reviewer: "design-review"},
		{name: "explicit root replan retains child history", phase: "drafting", scope: "replan", currentPlan: true, active: true, implementation: true, planner: "design-plan", reviewer: "design-review"},
		{name: "ready implementation", phase: "ready", active: true, implementation: true},
		{name: "half projected activation", phase: "activating", active: true, pending: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := diagnosticPlanningState(t)
			st.Planning.Phase = tc.phase
			if tc.scope != "" {
				st.Planning.Requests = []planning.Request{{Scope: tc.scope, InputRevision: st.Planning.InputRevision}}
			}
			if tc.currentPlan {
				st.Planning.CurrentPlan = &planning.Ref{Revision: 1}
				st.Planning.Plans = []planning.ExecutionPlan{{Ref: *st.Planning.CurrentPlan, InputRevision: st.Planning.InputRevision}}
			}
			if tc.active {
				st.Planning.Active = &planning.Activation{}
			}
			if tc.pending {
				st.Planning.PendingActivation = &planning.Activation{}
			}
			r := Report{OK: true}
			scope, implementation := r.planningScope(st)
			assert.Equal(t, tc.implementation, implementation)
			want := map[string]bool{}
			if tc.planner != "" {
				want["loops."+tc.planner], want["loops."+tc.reviewer] = true, true
			}
			assert.Equal(t, want, scope)
			if tc.pending {
				assert.Equal(t, "unknown", checkByID(t, r, "planning.activation").Status)
			}
		})
	}
}

func TestDoctorWithActivePlanningKeepsContractChecksButNeverLoadsPartialActivation(t *testing.T) {
	for _, phase := range []string{"drafting", "ready", "activating"} {
		t.Run(phase, func(t *testing.T) {
			root, runtime := diagnosticFixture(t, dshConfiguration)
			task := taskFixture(t, root, "frontend")
			st := diagnosticPlanningState(t)
			design, err := planning.SealDesign(planning.Design{Ref: planning.Ref{Revision: 1}, InputRevision: 1, Document: "Reviewed architecture", Requirements: []planning.Requirement{{ID: "R-01", Title: "Inspect", Milestone: "M-01", Verification: "Inspect only"}}, Milestones: []planning.Milestone{{ID: "M-01", Title: "Inspect", Gate: "Review"}}, Baseline: "base", PlannerInvocation: "design-call", ImplementationScope: []string{"src"}})
			require.NoError(t, err)
			require.NoError(t, planning.RecordDesign(st.Planning, design))
			require.NoError(t, planning.RecordReview(st.Planning, planning.Review{ID: "root-review", Kind: "design", Target: design.Ref, InputRevision: 1, Invocation: "root-review-call", Role: "design-review", Binding: "independent-reviewer", Verdict: "pass", Summary: "Requirements and acceptance are explicit", Findings: []planning.Finding{}}))
			require.NoError(t, planning.Approve(st.Planning, planning.Approval{Kind: "human", Design: design.Ref, ReviewID: "root-review", RequestID: "root-approval", Source: "tty:reviewed-root", At: state.NowISO(), Start: true}))
			plan, err := planning.SealPlan(planning.ExecutionPlan{Ref: planning.Ref{Revision: 1}, Design: design.Ref, InputRevision: 1, Scope: "milestone", ScopeID: "M-01", Files: map[string]string{"spec.md": specFixture}, Dependencies: "base", ScopeVerdict: "within-envelope", PlannerInvocation: "plan-call"})
			require.NoError(t, err)
			require.NoError(t, planning.RecordPlan(st.Planning, plan))
			require.NoError(t, planning.RecordReview(st.Planning, planning.Review{ID: "child-review", Kind: "plan", Target: plan.Ref, Design: &design.Ref, InputRevision: 1, Invocation: "child-review-call", Role: "plan-review", Binding: "independent-reviewer", Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "The child plan remains within the approved design", Findings: []planning.Finding{}}))
			st.Planning.Active = &planning.Activation{Design: design.Ref, Plans: []planning.Ref{plan.Ref}, Files: map[string]string{"spec.md": planning.DigestBytes([]byte(specFixture))}, OldFiles: map[string]string{}, Stage: "active", Base: "base", Head: "head"}
			st.Planning.Phase = phase
			if phase == "drafting" {
				require.NoError(t, planning.ApplyFeedback(st.Planning, planning.Feedback{RequestID: "discuss-recovery", Target: &design.Ref, Text: "Discuss recovery before changing the active implementation", Source: "control:discussion", At: state.NowISO()}))
				require.NotNil(t, st.Planning.CurrentPlan, "feedback retains the old sealed child as history")
			}
			if phase == "activating" {
				pending := *st.Planning.Active
				pending.Stage, pending.Head = "projected", ""
				st.Planning.PendingActivation = &pending
				writeFixture(t, filepath.Join(task, "spec.md"), "partially projected contract is not yet valid")
			}
			_, err = state.Save(task, st, nil)
			require.NoError(t, err)
			report := Doctor(context.Background(), root, task, true)
			require.True(t, report.OK, "%+v", report.Checks)
			if phase == "activating" {
				assert.Equal(t, "unknown", checkByID(t, report, "planning.activation").Status)
				assert.Empty(t, runtime.probes)
				for _, b := range report.Configuration.Bindings {
					assert.False(t, b.InScope)
				}
			} else {
				assert.Equal(t, "pass", checkByID(t, report, "contracts").Status)
				assert.True(t, bindingByID(t, report.Configuration, "routes.frontend").InScope)
				assert.Equal(t, phase == "drafting", bindingByID(t, report.Configuration, "loops.design-plan").InScope)
			}
		})
	}
}
