package machine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/investigation"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/require"
)

func managedPreparationFixture(t *testing.T, repair bool) (fixture, contract.Preparation) {
	t.Helper()
	fx := newGitFixture(t)
	require.NoError(t, os.Remove(state.StateFile(fx.taskDir)))
	writeFile(t, filepath.Join(fx.root, "settings.json"), `{"timeout":0}`)
	writeFile(t, filepath.Join(fx.root, "src", "loader.go"), "package loader\nvar timeout = 0\n")
	gitCmd(t, fx.root, "add", "settings.json", "src")
	gitCmd(t, fx.root, "commit", "-q", "-m", "test: establish investigation inputs")
	raw, err := templates.FS.ReadFile("preparation.example.json")
	require.NoError(t, err)
	p, err := contract.ValidatePreparation(raw)
	require.NoError(t, err)
	if repair {
		p.Intent, p.ImplementationScope = "investigate-and-fix", []string{"build"}
	}
	return fx, *p
}

func startManagedPreparation(t *testing.T, fx fixture, p contract.Preparation, opts investigation.Options) *state.PreparationState {
	t.Helper()
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	st, err := investigation.Start(fx.taskDir, fx.root, raw, opts)
	require.NoError(t, err)
	return st
}

func supportedPreparationReport(st *state.PreparationState) state.InvestigationReport {
	findings := []state.InvestigationFinding{}
	for i, hypothesis := range st.Contract.Hypotheses {
		finding := state.InvestigationFinding{Hypothesis: hypothesis.ID, Status: "unknown", Evidence: []string{}, Reason: "No claim beyond the collected local evidence."}
		if i == 0 {
			finding.Status, finding.Evidence = "supported", []string{st.Observations[0].Invocation.ID}
		}
		findings = append(findings, finding)
	}
	return state.InvestigationReport{Status: "supported", Summary: "The local source supports a bounded repair hypothesis.", Findings: findings,
		CounterEvidence: "Runtime and production overrides were not checked.", Limitations: []string{"Local files only."}, Recommendation: "Repair within the granted scope.", Regression: "Run the frozen regression.", Rollback: "Restore the prior implementation if required."}
}

func finishManagedPreparation(t *testing.T, fx fixture, opts investigation.Options) *state.PreparationState {
	t.Helper()
	st, err := investigation.Run(context.Background(), fx.taskDir, opts)
	require.NoError(t, err)
	require.Equal(t, "awaiting-report", st.Status)
	require.False(t, state.Exists(fx.taskDir), "investigation never creates implementation state")
	opts.ExpectedRevision = st.Revision
	st, err = investigation.Finish(fx.taskDir, supportedPreparationReport(st), opts)
	require.NoError(t, err)
	return st
}

func TestPreparationHandoffImportsKnownAndUnknownUsageOnceOnOriginalDays(t *testing.T) {
	fx, p := managedPreparationFixture(t, true)
	start := time.Date(2026, 11, 11, 23, 59, 59, 0, time.UTC)
	var elapsed, calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			elapsed.Store(2000)
			_, _ = w.Write([]byte(`{"model":"jev-1.14.0","answers":{"applicable":{"type":"noul","noul":0.99},"action":{"type":"choice","choice":"read-settings","confidence":0.8,"probabilities":{"read-settings":0.9,"find-default":0.05,"list-source":0.05}}},"usage":{"input_tokens":12,"output_tokens":4}}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":"usage unavailable"}`))
	}))
	defer server.Close()
	je := config.Default().JEV
	je.Enabled, je.Endpoint, je.KeyEnv, je.TimeoutMS, je.MaxCalls = true, server.URL+"/v1/systemone", "PREPARATION_FIXTURE_KEY", 1000, 2
	rewriteConfig(t, fx, func(values map[string]any) { values["jev"] = je })
	opts := investigation.Options{Now: func() time.Time { return start.Add(time.Duration(elapsed.Load()) * time.Millisecond) }, LookupEnv: func(string) (string, bool) { return "fixture-key", true }}
	startManagedPreparation(t, fx, p, opts)
	prepared := finishManagedPreparation(t, fx, opts)
	require.EqualValues(t, 2, calls.Load())
	require.Len(t, prepared.Decisions, 2)
	writeFile(t, state.StateFile(fx.taskDir), fixtureState(fx.slug))
	c := unitTestContext(t, fx)
	require.NotNil(t, c.State.PreparationSource)
	require.Equal(t, []string{"build"}, c.State.PreparationSource.ImplementationScope)
	require.EqualValues(t, 16, c.State.Budget.Totals.Tokens)
	require.EqualValues(t, 16, c.State.Budget.Daily["2026-11-11"].Tokens)
	require.Zero(t, c.State.Budget.Daily["2026-11-12"].Tokens)
	require.EqualValues(t, 1, c.State.Budget.UnknownUsageCalls)
	require.EqualValues(t, 1, c.State.Budget.Daily["2026-11-12"].UnknownUsageCalls)
	require.EqualValues(t, 2, c.State.Budget.UnknownCostCalls)
	c.save()
	require.NoError(t, c.err())
	before := jsonClone(c.State.Budget)
	resumed := unitTestContext(t, fx)
	require.Equal(t, before, resumed.State.Budget, "provenance and import share one CAS; resume cannot bill twice")
	require.NoError(t, resumed.attachPreparation())
	require.Equal(t, before, resumed.State.Budget)
	prepared.Report.Summary = "A revised report changes the admitted source identity."
	_, err := state.SavePreparation(fx.taskDir, prepared)
	require.NoError(t, err)
	require.ErrorContains(t, resumed.attachPreparation(), "provenance changed")
	require.NoError(t, os.Remove(state.PreparationFile(fx.taskDir)))
	require.ErrorContains(t, resumed.attachPreparation(), "load investigation provenance")
}

func TestPreparationHandoffDoesNotChargeSkippedJEVRequests(t *testing.T) {
	fx, p := managedPreparationFixture(t, true)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(500)
	}))
	defer server.Close()
	je := config.Default().JEV
	je.Enabled, je.Endpoint = true, server.URL+"/v1/systemone"
	je.AllowedData = []decision.Category{}
	rewriteConfig(t, fx, func(values map[string]any) { values["jev"] = je })
	opts := investigation.Options{LookupEnv: func(string) (string, bool) { return "fixture-key", true }}
	startManagedPreparation(t, fx, p, opts)
	prepared := finishManagedPreparation(t, fx, opts)
	require.NotEmpty(t, prepared.Decisions)
	for _, receipt := range prepared.Decisions {
		require.Equal(t, "skipped", receipt.Status)
		require.Equal(t, "data-not-authorized", receipt.Reason)
	}
	require.Zero(t, calls.Load(), "a skipped decision must not reach the service")
	writeFile(t, state.StateFile(fx.taskDir), fixtureState(fx.slug))
	c := unitTestContext(t, fx)
	require.NotNil(t, c.State.PreparationSource)
	require.Zero(t, c.State.Budget.Totals.Tokens)
	require.Zero(t, c.State.Budget.UnknownUsageCalls)
	require.Zero(t, c.State.Budget.UnknownCostCalls, "no dispatched request means no unknown charge")
	c.save()
	require.NoError(t, c.err())
	resumed := unitTestContext(t, fx)
	require.Equal(t, c.State.Budget, resumed.State.Budget)
}

func TestPreparationHandoffRequiresSettledRepairAuthority(t *testing.T) {
	for _, scenario := range []string{"report-only", "unsettled", "different-workspace", "corrupt"} {
		t.Run(scenario, func(t *testing.T) {
			fx, p := managedPreparationFixture(t, scenario != "report-only")
			startManagedPreparation(t, fx, p, investigation.Options{})
			if scenario != "unsettled" {
				prepared := finishManagedPreparation(t, fx, investigation.Options{})
				if scenario == "different-workspace" {
					prepared.Cwd = t.TempDir()
					_, err := state.SavePreparation(fx.taskDir, prepared)
					require.NoError(t, err)
				}
			}
			if scenario == "corrupt" {
				writeFile(t, state.PreparationFile(fx.taskDir), "invalid JSON")
			}
			writeFile(t, state.StateFile(fx.taskDir), fixtureState(fx.slug))
			_, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: configFor(fx), Drivers: happyDrivers().set()})
			require.Error(t, err)
			require.Zero(t, loadFixtureState(t, fx).Packets["M01-T01"].Attempts)
		})
	}
}

func TestPreparationScopeRemainsFrozenAgainstPacketExpansionAndRealDiff(t *testing.T) {
	fx, p := managedPreparationFixture(t, true)
	startManagedPreparation(t, fx, p, investigation.Options{})
	finishManagedPreparation(t, fx, investigation.Options{})
	writeFile(t, state.StateFile(fx.taskDir), fixtureState(fx.slug))
	c := unitTestContext(t, fx)
	packet := c.Packets["M01-T01"]
	packet.Boundaries.MustNotTouch = nil
	packet.Boundaries.MaxFiles = new(100)
	c.Config.Worktree.Enabled = new(false)
	out, err := c.executeStep(packet)
	require.NoError(t, err)
	require.True(t, out.Success)
	writeFile(t, filepath.Join(fx.root, "outside.txt"), "outside original repair authorization")
	gate := c.l1Step(packet)
	require.True(t, gate.Gate.Pass)
	require.False(t, gate.Pass)
	require.Contains(t, gate.Boundary.Violations, verify.BoundaryViolation{Type: "investigation-scope", Path: "outside.txt"})
	require.Equal(t, []string{"build"}, c.State.PreparationSource.ImplementationScope)
	violations := c.preparationScopeViolations([]string{"build", "build/file.txt", "build-more/file.txt"})
	require.Equal(t, []boundaryViolationLite{{Type: "investigation-scope", Path: "build-more/file.txt"}}, violations)
	c.State.PreparationSource.ImplementationScope = []string{"."}
	require.Empty(t, c.preparationScopeViolations([]string{"elsewhere/file.txt"}))
}
