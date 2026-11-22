package investigation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/require"
)

func preparationFixture(t *testing.T) (string, string, contract.Preparation) {
	t.Helper()
	cwd := t.TempDir()
	var err error
	cwd, err = filepath.EvalSymlinks(cwd)
	require.NoError(t, err)
	dir := filepath.Join(cwd, "docs", "investigation")
	require.NoError(t, os.MkdirAll(filepath.Join(cwd, "src"), 0o700))
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "settings.json"), []byte(`{"timeout":0}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "src", "loader.go"), []byte("package loader\nvar timeout = 0\n"), 0o600))
	raw, err := templates.FS.ReadFile("preparation.example.json")
	require.NoError(t, err)
	p, err := contract.ValidatePreparation(raw)
	require.NoError(t, err)
	cfg, err := templates.FS.ReadFile("flywheel.config.example.json")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "flywheel.config.json"), cfg, 0o600))
	return dir, cwd, *p
}

func startFixture(t *testing.T, dir, cwd string, p contract.Preparation) *state.PreparationState {
	t.Helper()
	if p.Decision != nil {
		configureDecision(t, dir, p.Decision, p.Decision.Policy.Mode != decision.Off)
	}
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	st, err := Start(dir, cwd, raw, Options{})
	require.NoError(t, err)
	return st
}

func configureDecision(t *testing.T, dir string, d *contract.InvestigationDecision, enabled bool) {
	t.Helper()
	path := filepath.Join(dir, "flywheel.config.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(raw, &cfg))
	delete(cfg, "subscription") // This fixture deliberately tests the explicit legacy HTTP backend.
	cfg["jev"] = map[string]any{"enabled": enabled, "endpoint": d.Endpoint, "model": d.Model, "key-env": d.KeyEnv, "timeout-ms": d.TimeoutMS, "max-calls": d.Policy.MaxCalls, "min-applicability": d.Policy.MinApplicability, "min-choice": d.Policy.MinChoice, "allowed-data": d.Policy.AllowedData}
	raw, err = json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}

func reportFixture(st *state.PreparationState) state.InvestigationReport {
	findings := []state.InvestigationFinding{}
	for i, hypothesis := range st.Contract.Hypotheses {
		finding := state.InvestigationFinding{Hypothesis: hypothesis.ID, Status: "unknown", Evidence: []string{}, Reason: "The observation does not establish this hypothesis."}
		if i == 0 {
			finding.Status, finding.Evidence, finding.Reason = "supported", []string{st.Observations[0].Invocation.ID}, "The cited local source supports this limited conclusion."
		}
		findings = append(findings, finding)
	}
	return state.InvestigationReport{Status: "supported", Summary: "A source-level timeout issue is supported; runtime causality remains to be checked.", Findings: findings, CounterEvidence: "Other environment overrides remain possible.", Limitations: []string{"No production environment was queried."}, Recommendation: "Correct the default within the approved scope.", Regression: "Add an omitted-timeout regression.", Rollback: "Restore the previous loader behavior if the regression fails."}
}

func TestInvestigationCollectsBoundedFactsAndEndsWithReportNotCodeCompletion(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	before, err := os.ReadFile(filepath.Join(cwd, "settings.json"))
	require.NoError(t, err)
	st := startFixture(t, dir, cwd, p)
	st, err = Run(context.Background(), dir, Options{ExpectedRevision: st.Revision})
	require.NoError(t, err)
	require.Equal(t, "awaiting-report", st.Status)
	require.Len(t, st.Observations, 3)
	require.Equal(t, 3, st.ActionsUsed)
	require.Contains(t, st.Observations[1].Output, "2:var timeout")
	require.Contains(t, st.Observations[2].Output, "loader.go")
	require.Empty(t, st.Decisions)
	after, err := os.ReadFile(filepath.Join(cwd, "settings.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.False(t, state.Exists(dir))
	metric, err := metrics.ReadReport(filepath.Join(dir, "metrics.jsonl"))
	require.NoError(t, err)
	require.Zero(t, metric.TotalCalls)
	require.Equal(t, 3, metric.Timeline.Complete)
	report := reportFixture(st)
	st, err = Finish(dir, report, Options{ExpectedRevision: st.Revision})
	require.NoError(t, err)
	require.Equal(t, "supported", st.Status)
	revision := st.Revision
	st, err = Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, revision, st.Revision)
	_, err = Finish(dir, report, Options{})
	require.ErrorContains(t, err, "terminal")
	st, err = Cancel(dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "supported", st.Status)
}

func TestInvestigationBudgetsCancellationAndCASDoNotResetOnResume(t *testing.T) {
	for _, test := range []string{"actions", "output", "deadline", "cancel"} {
		t.Run(test, func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			if test == "actions" {
				p.Limits.MaxActions = 1
			}
			if test == "output" {
				p.Limits.MaxOutputBytes = 128
				require.NoError(t, os.WriteFile(filepath.Join(cwd, "settings.json"), []byte(strings.Repeat("中", 100)), 0o600))
			}
			st := startFixture(t, dir, cwd, p)
			_, err := Run(context.Background(), dir, Options{ExpectedRevision: st.Revision + 1})
			require.ErrorContains(t, err, "stale")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := Options{}
			if test == "deadline" {
				opts.Now = func() time.Time { return time.Now().Add(time.Hour) }
			}
			if test == "cancel" {
				cancel()
			}
			st, err = Run(ctx, dir, opts)
			require.NoError(t, err)
			if test == "cancel" {
				require.Equal(t, "cancelled", st.Status)
			} else {
				require.Equal(t, "awaiting-report", st.Status)
			}
			used := st.ActionsUsed
			st, err = Run(ctx, dir, opts)
			require.NoError(t, err)
			require.Equal(t, used, st.ActionsUsed)
			if test == "actions" {
				require.Equal(t, 1, used)
			}
			if test == "output" {
				require.True(t, st.Observations[0].Truncated)
				require.LessOrEqual(t, st.OutputBytes, 128)
			}
		})
	}
}

func TestUnknownActionBlocksAndMatchingReceiptRecoversExactlyOnce(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	st := startFixture(t, dir, cwd, p)
	action := p.Actions[0]
	st.Pending = &state.PreparationInvocation{ID: state.PreparationHash("lost-action"), ActionID: action.ID, BindingHash: state.PreparationHash(action), CandidateHash: state.PreparationHash("candidate"), StartedAt: time.Now().Format(time.RFC3339Nano), Reason: "baseline"}
	st.ActionsUsed, st.Status = 1, "running"
	st, err := state.SavePreparation(dir, st)
	require.NoError(t, err)
	st, err = Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "blocked", st.Status)
	require.Equal(t, 1, st.ActionsUsed)
	_, err = Cancel(dir, Options{})
	require.ErrorContains(t, err, "unresolved")
	_, err = Finish(dir, state.InvestigationReport{}, Options{})
	require.ErrorContains(t, err, "unresolved")
	receipt := state.InvestigationObservation{Invocation: *st.Pending, Status: "observed", Output: "original-result", ContentHash: digest([]byte("original-result")), FinishedAt: time.Now().Format(time.RFC3339Nano)}
	require.NoError(t, state.SavePreparationReceipt(dir, receipt))
	st, err = Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "awaiting-report", st.Status)
	require.Equal(t, "original-result", st.Observations[0].Output)
	require.Len(t, st.Observations, 3)
	require.Equal(t, 3, st.ActionsUsed)
}

func TestInvestigationValidatesReportReferencesAndCounterEvidence(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	startFixture(t, dir, cwd, p)
	st, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	for _, mutate := range []func(*state.InvestigationReport){
		func(r *state.InvestigationReport) { r.Status = "completed" },
		func(r *state.InvestigationReport) { r.Summary = "" },
		func(r *state.InvestigationReport) { r.CounterEvidence = "" },
		func(r *state.InvestigationReport) { r.Findings[0].Hypothesis = "invented" },
		func(r *state.InvestigationReport) { r.Findings[0].Evidence = nil },
		func(r *state.InvestigationReport) { r.Findings[0].Evidence = []string{"invented"} },
		func(r *state.InvestigationReport) { r.Findings[0].Status = "unknown" },
		func(r *state.InvestigationReport) { r.Status = "inconclusive"; r.Limitations = nil },
	} {
		r := reportFixture(st)
		mutate(&r)
		_, err := Finish(dir, r, Options{})
		require.Error(t, err)
	}
	r := reportFixture(st)
	r.Status = "inconclusive"
	st, err = Finish(dir, r, Options{})
	require.NoError(t, err)
	require.Equal(t, "inconclusive", st.Status)
}

func TestJEVAdoptsOnlyApprovedDiagnosticAndBudgetsTheLiveDecision(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"model":"jev-1.14.0","answers":{"applicable":{"type":"noul","noul":0.99},"action":{"type":"choice","choice":"find-default","confidence":0.8,"probabilities":{"find-default":0.9,"read-settings":0.05,"list-source":0.05}}},"usage":{"input_tokens":12,"output_tokens":4}}`))
	}))
	defer server.Close()
	p.Decision = &contract.InvestigationDecision{Policy: decision.Policy{Version: "experiment-1", Mode: decision.Adopt, AllowedData: []decision.Category{decision.TaskSummary, decision.DiagnosticFacts, decision.Source}, MinApplicability: 0.9, MinChoice: 0.8, MaxCalls: 1}, Endpoint: server.URL + "/v1/systemone", Model: "jev-1.14.0", KeyEnv: "INVESTIGATION_TEST_KEY", TimeoutMS: 1000}
	startFixture(t, dir, cwd, p)
	st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "test-key", true }})
	require.NoError(t, err)
	require.Equal(t, 1, requests)
	require.Equal(t, "find-default", st.Observations[0].Invocation.ActionID)
	require.True(t, st.Observations[0].Invocation.Adopted)
	require.EqualValues(t, 12, *st.Decisions[0].Usage.Input)
	require.Len(t, st.Decisions, 1)
	raw, err := os.ReadFile(state.PreparationFile(dir))
	require.NoError(t, err)
	require.NotContains(t, string(raw), "test-key")
}

func TestMasterNormalizesHistoricalModesAndRecoversUnknownRequests(t *testing.T) {
	for _, mode := range []decision.Mode{decision.Off, decision.Offline, decision.Shadow, decision.Advisory} {
		t.Run(string(mode), func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			p.Decision = &contract.InvestigationDecision{Policy: decision.Policy{Version: "experiment-1", Mode: mode, AllowedData: []decision.Category{decision.TaskSummary, decision.DiagnosticFacts, decision.Source}, MinApplicability: 0.9, MinChoice: 0.8, MaxCalls: 1}, Endpoint: "http://127.0.0.1:1/v1/systemone", Model: "jev-1.14.0", KeyEnv: "INVESTIGATION_TEST_KEY", TimeoutMS: 10}
			st := startFixture(t, dir, cwd, p)
			if mode == decision.Off {
				require.Nil(t, st.Contract.Decision)
			} else {
				require.Equal(t, decision.Adopt, st.Contract.Decision.Policy.Mode)
			}
			if mode == decision.Advisory {
				identity, err := candidate(context.Background(), st)
				require.NoError(t, err)
				s := snapshot(st, remainingActions(st), identity)
				st.DecisionPending = &s
				st.DecisionStartedAt = map[string]string{s.ID: time.Now().Format(time.RFC3339Nano)}
				st, err = state.SavePreparation(dir, st)
				require.NoError(t, err)
			}
			st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "test-key", true }})
			require.NoError(t, err)
			require.Equal(t, "read-settings", st.Observations[0].Invocation.ActionID)
			require.False(t, st.Observations[0].Invocation.Adopted)
			if mode == decision.Off {
				require.Empty(t, st.Decisions)
			}
			if mode == decision.Advisory {
				require.Len(t, st.Decisions, 1)
				require.Equal(t, "unknown", st.Decisions[0].Status)
				require.Nil(t, st.Decisions[0].Usage.Input)
			}
		})
	}
}

func TestReadActionsRejectEscapesSymlinksSpecialFilesAndExcessiveInput(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	_ = dir
	outside := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(outside, []byte("private"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(cwd, "link")))
	for _, path := range []string{"link", "../secret", "missing"} {
		_, _, err := readPath(context.Background(), cwd, path, 20)
		require.Error(t, err)
	}
	_, _, _, err := executeAction(context.Background(), cwd, contract.InvestigationAction{Kind: "read_file", Path: "src"}, p.Limits, 100)
	require.Error(t, err)
	_, _, _, err = executeAction(context.Background(), cwd, p.Actions[0], contract.InvestigationLimits{MaxFileBytes: 2}, 100)
	require.Error(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "binary"), []byte{0xff}, 0o600))
	_, _, err = readPath(context.Background(), cwd, "binary", 10)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = readPath(ctx, cwd, "settings.json", 100)
	require.Error(t, err)
}
