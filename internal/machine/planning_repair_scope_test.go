package machine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestReviewedRepairPlanCannotWidenOriginalGrantAfterApproveOnly(t *testing.T) {
	opts, st, _, _ := planningReportFixture(t, true, false)
	require.True(t, st.Planning.Brief.ImplementationAuthorized)
	st = reviewedControlDesign(t, opts, st)
	design := st.Planning.Designs[0]
	design.Ref.Revision++
	design.ImplementationScope, design.PlannerInvocation = []string{"forbidden"}, "expanded-design-proposal"
	design, err := planning.SealDesign(design)
	require.NoError(t, err)
	require.NoError(t, planning.RecordDesign(st.Planning, design))
	require.NoError(t, planning.RecordReview(st.Planning, planning.Review{ID: "expanded-design-review", Kind: "design", Target: design.Ref,
		InputRevision: st.Planning.InputRevision, Invocation: "expanded-independent-review", Role: "design-review", Binding: "independent", Verdict: "pass", Summary: "Review does not grant additional paths.", Findings: []planning.Finding{}}))
	st, err = state.Save(opts.TaskDir, st, nil)
	require.NoError(t, err)
	st, _, err = ControlPlanning(opts, PlanningCommand{Action: "approve", Revision: st.Revision, RequestID: "approve-only-existing-grant", Design: st.Planning.CurrentDesign, Start: new(false)}, "tty:review-design")
	require.NoError(t, err)
	require.True(t, planning.StartAllowed(st.Planning, design.Ref), "the task's original grant exists but remains bounded by its original paths")
	files := enginePlanFiles()
	files["spec.md"] = strings.Replace(files["spec.md"], "Prove the orchestrator loop end to end.", st.Planning.Brief.Goal, 1)
	for name, raw := range files {
		files[name] = strings.ReplaceAll(raw, "build/", "forbidden/")
	}
	called := 0
	drv := newScripted(nil).set()
	drv.Plan = &fnDriver{role: "plan", base: drivers.NewMockDriver(nil), log: &scriptLog{}, fn: func(a drivers.CallArgs) drivers.Result {
		called++
		return planningJSONResult(t, prompts.PlanOutput{Files: files})
	}}
	drv.Execute = &fnDriver{role: "execute", base: drivers.NewMockDriver(nil), log: &scriptLog{}, fn: func(drivers.CallArgs) drivers.Result {
		t.Fatal("scope failure must be detected before Execute")
		return drivers.Result{}
	}}
	opts.Drivers = drv
	summary, err := RunTask(RunTaskOpts{RunOpts: opts})
	require.NoError(t, err)
	require.Greater(t, called, 0)
	require.Equal(t, "blocked", summary.PlanningPhase)
	after, err := state.Load(opts.TaskDir)
	require.NoError(t, err)
	require.Contains(t, after.Planning.Reason, "original investigation implementation scope")
	require.Empty(t, after.Planning.Plans)
	require.Nil(t, after.Planning.PendingActivation)
	require.NoFileExists(t, filepath.Join(opts.TaskDir, "spec.md"))
	require.NoDirExists(t, filepath.Join(opts.Cwd, "forbidden"))
}

func TestReviewedReportOnlyMayPlanButCannotActivateBeforeExactRepairGrant(t *testing.T) {
	opts, st, _, prepared := planningReportFixture(t, false, false)
	gitCmd(t, opts.Cwd, "add", "settings.json", "src")
	gitCmd(t, opts.Cwd, "commit", "-qm", "test: preserve investigation inputs")
	st = reviewedControlDesign(t, opts, st)
	approve := PlanningCommand{Action: "approve", Revision: st.Revision, RequestID: "approve-report-only", Design: st.Planning.CurrentDesign, Start: new(false)}
	st, _, err := ControlPlanning(opts, approve, "tty:review-report")
	require.NoError(t, err)
	c, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Now: opts.Now})
	require.NoError(t, err)
	files := enginePlanFiles()
	files["spec.md"] = strings.Replace(files["spec.md"], "Prove the orchestrator loop end to end.", st.Planning.Brief.Goal, 1)
	require.NoError(t, c.validatePlanningFiles(files, planning.Request{Scope: "milestone", ScopeID: "M-01"}), "the proposed repair scope remains reviewable before Start")
	require.NoError(t, c.checkPlanningExports(true))
	addReviewedPlan(t, c, files, "milestone", "M-01")
	require.ErrorContains(t, c.activatePlanningPlans(), "exact human Start")
	require.Nil(t, c.State.Planning.PendingActivation)
	require.NoFileExists(t, filepath.Join(opts.TaskDir, "spec.md"))
	approve.Revision, approve.RequestID, approve.Start = c.State.Revision, "start-exact-report", new(true)
	_, _, err = ControlPlanning(opts, approve, "tty:start-report")
	require.NoError(t, err)
	c, err = CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Now: opts.Now})
	require.NoError(t, err)
	write := writePlanningBytes
	t.Cleanup(func() { writePlanningBytes = write })
	writePlanningBytes = func(*os.Root, string, []byte) error { return os.ErrPermission }
	require.ErrorIs(t, c.activatePlanningPlans(), os.ErrPermission)
	writePlanningBytes = write
	require.NotNil(t, c.State.Planning.PendingActivation)
	original := *prepared.Report
	prepared.Report.Summary = "The evidence changed after the original exact repair grant."
	prepared, err = state.SavePreparation(opts.TaskDir, prepared)
	require.NoError(t, err)
	require.ErrorContains(t, c.resumePlanningActivation(), "provenance changed")
	require.NoFileExists(t, filepath.Join(opts.TaskDir, "spec.md"), "recovery checks report authority before writing any executable contract")
	prepared.Report = &original
	_, err = state.SavePreparation(opts.TaskDir, prepared)
	require.NoError(t, err)
	require.NoError(t, c.resumePlanningActivation())
	require.NotNil(t, c.State.Planning.Active)
	require.NoError(t, c.admitReviewedExecution())
}

func TestReviewedRepairScopeRequiresAvailableImportedExactInvestigation(t *testing.T) {
	opts, st, _, _ := planningReportFixture(t, true, false)
	st = reviewedControlDesign(t, opts, st)
	c, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Now: opts.Now})
	require.NoError(t, err)
	design := planning.FindDesign(c.State.Planning, *c.State.Planning.CurrentDesign)
	require.NoError(t, c.checkPlanningRepairScope(design, false))
	source := c.State.PreparationSource
	c.State.PreparationSource = nil
	require.ErrorContains(t, c.checkPlanningRepairScope(design, false), "must be imported")
	c.State.PreparationSource = source
	file := state.PreparationFile(opts.TaskDir)
	raw := mustReadFile(t, file)
	require.NoError(t, os.Remove(file))
	require.ErrorContains(t, c.checkPlanningRepairScope(design, false), "no longer available")
	writeFile(t, file, "{invalid")
	require.Error(t, c.checkPlanningRepairScope(design, false))
	writeFile(t, file, string(raw))
	require.NoError(t, c.checkPlanningRepairScope(design, false))
}
