package machine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/investigation"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/require"
)

func planningReportFixture(t *testing.T, repair, thinFirst bool) (RunOpts, *state.State, planning.Brief, *state.PreparationState) {
	t.Helper()
	opts, st, brief := controlPlanningFixture(t)
	if !thinFirst {
		require.NoError(t, os.Remove(state.StateFile(opts.TaskDir)))
	}
	writeFile(t, filepath.Join(opts.TaskDir, "flywheel.config.json"), fixtureConfigJSON)
	writeFile(t, filepath.Join(opts.Cwd, "settings.json"), "{\"timeout\":0}")
	writeFile(t, filepath.Join(opts.Cwd, "src", "loader.go"), "package loader\nvar timeout = 0\n")
	raw, err := templates.FS.ReadFile("preparation.example.json")
	require.NoError(t, err)
	p, err := contract.ValidatePreparation(raw)
	require.NoError(t, err)
	if repair {
		p.Intent, p.ImplementationScope = "investigate-and-fix", []string{"build"}
	}
	raw, err = json.Marshal(p)
	require.NoError(t, err)
	_, err = investigation.Start(opts.TaskDir, opts.Cwd, raw, investigation.Options{})
	require.NoError(t, err)
	prepared, err := investigation.Run(context.Background(), opts.TaskDir, investigation.Options{})
	require.NoError(t, err)
	prepared, err = investigation.Finish(opts.TaskDir, supportedPreparationReport(prepared), investigation.Options{ExpectedRevision: prepared.Revision})
	require.NoError(t, err)
	if !thinFirst {
		st, _, err = ControlPlanning(opts, PlanningCommand{Action: "prepare", Brief: &brief, Start: new(false)}, "")
		require.NoError(t, err)
	}
	return opts, st, brief, prepared
}

func TestPlanningReportOnlyAllowsExactHumanRepairGrantWithoutChangingOriginalIntent(t *testing.T) {
	opts, st, brief, prepared := planningReportFixture(t, false, true)
	require.Nil(t, st.PreparationSource, "the earlier thin brief has not consumed research yet")
	st, receipt, err := ControlPlanning(opts, PlanningCommand{Action: "prepare", Brief: &brief, Start: new(false)}, "")
	require.NoError(t, err)
	require.Equal(t, "applied", receipt.Status)
	require.NotNil(t, st.PreparationSource)
	require.Empty(t, st.PreparationSource.ImplementationScope, "standard report-only template grants no repair paths")
	budget := jsonClone(st.Budget)
	st, receipt, err = ControlPlanning(opts, PlanningCommand{Action: "prepare", Brief: &brief, Start: new(false)}, "")
	require.NoError(t, err)
	require.Equal(t, "unchanged", receipt.Status)
	st = reviewedControlDesign(t, opts, st)
	approve := PlanningCommand{Action: "approve", Revision: st.Revision, RequestID: "review-report-only", Design: st.Planning.CurrentDesign, Start: new(false)}
	st, _, err = ControlPlanning(opts, approve, "tty:reported-design")
	require.NoError(t, err)
	c, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Now: opts.Now})
	require.NoError(t, err)
	require.ErrorContains(t, c.attachPreparation(), "repair authority")
	require.False(t, c.planningRepairApproved(prepared))
	approve.RequestID, approve.Revision, approve.Start = "repair-exact-report", st.Revision, new(true)
	st, _, err = ControlPlanning(opts, approve, "dsh:session:exact-command")
	require.NoError(t, err)
	require.True(t, st.Planning.StartAuthorized)
	require.Equal(t, state.PreparationHash(prepared.Report), st.Planning.Approvals[len(st.Planning.Approvals)-1].ReportHash)
	c, err = CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Now: opts.Now})
	require.NoError(t, err)
	require.True(t, c.planningRepairApproved(prepared))
	require.NoError(t, c.attachPreparation())
	require.Equal(t, []string{"build"}, c.effectivePreparationScope())
	require.Empty(t, c.State.PreparationSource.ImplementationScope)
	require.Equal(t, []boundaryViolationLite{{Type: "investigation-scope", Path: "outside/file.go"}}, c.preparationScopeViolations([]string{"build/file.txt", "outside/file.go"}))
	require.Equal(t, budget, c.State.Budget)
	unchanged, err := state.LoadPreparation(opts.TaskDir)
	require.NoError(t, err)
	require.Equal(t, "investigate", unchanged.Contract.Intent)
	require.Empty(t, unchanged.Contract.ImplementationScope)
	require.Equal(t, prepared.ContractHash, unchanged.ContractHash)
	text, err := c.planningResearchContext()
	require.NoError(t, err)
	require.Contains(t, text, prepared.Report.Summary)
	require.Contains(t, text, prepared.Report.Limitations[0])
	require.Contains(t, text, prepared.ContractHash)
	before := c.State.Revision
	_, err = c.planningResearchContext()
	require.NoError(t, err)
	require.Equal(t, before, c.State.Revision, "already imported research must not add a CAS or charge")
	prepared.Report.Summary = "Revised investigation report must require a new explicit handoff."
	_, err = state.SavePreparation(opts.TaskDir, prepared)
	require.NoError(t, err)
	_, err = c.planningResearchContext()
	require.ErrorContains(t, err, "provenance changed")
	approve.Revision, approve.RequestID = st.Revision, "changed-report"
	_, _, err = ControlPlanning(opts, approve, "tty:changed-report")
	require.ErrorContains(t, err, "report changed")
	require.Empty(t, c.effectivePreparationScope(), "an old report grant cannot authorize a changed report")
	_, _, err = ControlPlanning(opts, PlanningCommand{Action: "prepare", Brief: &brief}, "")
	require.ErrorContains(t, err, "provenance changed")
	require.NoError(t, os.Remove(state.PreparationFile(opts.TaskDir)))
	_, err = c.planningResearchContext()
	require.ErrorContains(t, err, "no longer available")
}

func TestPlanningResearchImportsBeforeBudgetAndRecoversOnlyFromSavedSource(t *testing.T) {
	opts, st, _, prepared := planningReportFixture(t, false, true)
	c, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Now: opts.Now})
	require.NoError(t, err)
	text, err := c.planningResearchContext()
	require.NoError(t, err)
	require.Contains(t, text, state.PreparationHash(prepared.Report))
	require.Greater(t, c.State.Revision, st.Revision)
	require.True(t, c.State.PreparationSource.Accounted)
	require.Equal(t, prepared.ActionsUsed, c.State.PreparationSource.Actions)
	saved, err := state.Load(opts.TaskDir)
	require.NoError(t, err)
	require.Equal(t, c.State.PreparationSource, saved.PreparationSource)
	// Persistence failure blocks a planning call before any budget or report use.
	opts, _, _, _ = planningReportFixture(t, false, true)
	c, err = CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Now: opts.Now})
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(opts.TaskDir, ".state.lock")))
	require.NoError(t, os.Mkdir(filepath.Join(opts.TaskDir, ".state.lock"), 0700))
	_, err = c.planningResearchContext()
	require.Error(t, err)
	saved, err = state.Load(opts.TaskDir)
	require.NoError(t, err)
	require.Nil(t, saved.PreparationSource)
	// A malformed or unsettled investigation never enters the prompt.
	writeFile(t, state.PreparationFile(opts.TaskDir), "{broken")
	_, err = c.planningResearchContext()
	require.Error(t, err)
}

func TestPlanningPreparationKeepsExistingRepairEnvelopeAndInitialAuthority(t *testing.T) {
	for _, thinFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "research-before-brief", true: "brief-before-research"}[thinFirst], func(t *testing.T) {
			opts, st, brief, prepared := planningReportFixture(t, true, thinFirst)
			if !thinFirst {
				require.True(t, st.Planning.Brief.ImplementationAuthorized)
				require.Equal(t, "investigation:"+prepared.ContractHash, st.Planning.Brief.Authorization)
			}
			st, _, err := ControlPlanning(opts, PlanningCommand{Action: "prepare", Brief: &brief, Start: new(false)}, "")
			require.NoError(t, err)
			require.NotNil(t, st.PreparationSource)
			st = reviewedControlDesign(t, opts, st)
			r := PlanningCommand{Action: "approve", Revision: st.Revision, RequestID: "repair-approved", Design: st.Planning.CurrentDesign, Start: new(true)}
			hash, err := planningRepairAuthority(opts, st, r)
			require.NoError(t, err)
			require.NotEmpty(t, hash)
			st, _, err = ControlPlanning(opts, r, "tty:repair-existing")
			require.NoError(t, err)
			c, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Now: opts.Now})
			require.NoError(t, err)
			require.True(t, c.planningRepairApproved(prepared))
			require.Equal(t, []string{"build"}, c.effectivePreparationScope())
			c.State.Planning.Designs[0].ImplementationScope = []string{"src"}
			require.False(t, c.planningRepairApproved(prepared))
			_, err = planningRepairAuthority(opts, c.State, r)
			require.ErrorContains(t, err, "supported investigation scope")
			c.State.Planning.Designs = nil
			require.False(t, c.planningRepairApproved(prepared))
			_, err = planningRepairAuthority(opts, c.State, r)
			require.Error(t, err)
		})
	}
}

func TestPlanningPreparationRejectsImplicitReportRepairAndInvalidEvidence(t *testing.T) {
	opts, st, brief, prepared := planningReportFixture(t, false, false)
	brief.ImplementationAuthorized, brief.Authorization = true, "unrelated-implicit-grant"
	_, _, err := ControlPlanning(opts, PlanningCommand{Action: "prepare", Brief: &brief}, "")
	require.ErrorContains(t, err, "exact human design approval")
	c := &Context{TaskDir: opts.TaskDir, Cwd: opts.Cwd, State: st}
	c.State.Planning.CurrentDesign = nil
	require.False(t, c.planningRepairApproved(prepared))
	c.State.Planning = nil
	require.False(t, c.planningRepairApproved(prepared))
	require.Equal(t, c.State.PreparationSource.ImplementationScope, c.effectivePreparationScope())
	prepared.Cwd = t.TempDir()
	_, err = state.SavePreparation(opts.TaskDir, prepared)
	require.NoError(t, err)
	_, err = planningPreparation(opts.TaskDir, opts.Cwd)
	require.ErrorContains(t, err, "settled investigation report")
	_, err = planningRepairAuthority(opts, st, PlanningCommand{Start: new(true)})
	require.Error(t, err)
	require.True(t, planningScopeWithin([]string{"build", "build/sub/file.go"}, []string{"build"}))
	require.False(t, planningScopeWithin([]string{"build-elsewhere/file.go"}, []string{"build"}))
	require.True(t, planningScopeWithin([]string{"anywhere"}, []string{"."}))
}
