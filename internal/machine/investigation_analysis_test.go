package machine

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/investigation"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func analysisFixture(t *testing.T) (fixture, *state.PreparationState, config.Config) {
	t.Helper()
	fx, p := managedPreparationFixture(t, false)
	p.Limits.MaxActions = 12
	startManagedPreparation(t, fx, p, investigation.Options{})
	st, err := investigation.Run(context.Background(), fx.taskDir, investigation.Options{})
	require.NoError(t, err)
	cfg := *mustConfig(t, fx)
	cfg.Loops["investigate"] = config.LoopBinding{Harness: "antigravity", Model: "gemini-4-neon", Channel: "google-ultra", Thinking: "high"}
	cfg.Subscription = &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}
	return fx, st, cfg
}

func investigationDriver(fn func(drivers.CallArgs) drivers.Result) drivers.Drivers {
	return drivers.Drivers{Investigate: &fnDriver{role: "investigate", log: &scriptLog{}, fn: fn}}
}

func analysisResult(t *testing.T, st *state.PreparationState, requestID, nativeID string) drivers.Result {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"version": 1, "report": supportedPreparationReport(st)})
	require.NoError(t, err)
	return drivers.MockResult(func(r *drivers.Result) {
		r.Harness, r.Model, r.Text = "antigravity", "gemini-4-neon", string(raw)
		r.DurationMS = 0
		r.Usage = &drivers.Usage{Input: 120, Output: 8, TotalTokens: 128}
		r.CostUSD = nil
		r.Session = &drivers.SessionOutcome{RequestID: requestID, RunID: nativeID, SessionID: "analysis-history", Dispatch: "dispatched", Outcome: "completed", CleanupConfirmed: true, UsageStatus: "complete"}
	})
}

func analysisLifecycle(a drivers.CallArgs, run string, sequence int) error {
	return a.OnLifecycle(drivers.MachineLifecycle{LifecycleVersion: 1, Type: "lifecycle", RequestID: a.RequestID, RunID: run, Sequence: sequence, Persistent: true})
}

func TestInvestigationAnalysisUsesOnlyEvidenceAndPersistsExactOutcome(t *testing.T) {
	fx, st, cfg := analysisFixture(t)
	calls := 0
	ds := investigationDriver(func(a drivers.CallArgs) drivers.Result {
		calls++
		require.True(t, a.EvidenceOnly)
		require.Equal(t, cfg.Subscription, a.Subscription)
		require.Empty(t, a.APIKeys)
		require.Contains(t, a.Prompt, st.Observations[0].Invocation.ID)
		require.Contains(t, a.Prompt, "untrusted evidence")
		pending, err := state.LoadPreparation(fx.taskDir)
		require.NoError(t, err)
		require.Equal(t, a.RequestID, pending.AnalysisPending.Request.ID, "intent must be durable before dispatch")
		require.NoError(t, analysisLifecycle(a, "native-analysis", 1))
		require.NoError(t, analysisLifecycle(a, "native-analysis", 2))
		return analysisResult(t, st, a.RequestID, "native-analysis")
	})
	result, err := AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.NoError(t, err)
	require.Equal(t, "supported", result.Status)
	require.Nil(t, result.AnalysisPending)
	require.Equal(t, state.PreparationHash(result.Report), result.Analysis.ReportHash)
	require.Equal(t, cfg.Loops["investigate"], result.Analysis.Request.Binding)
	_, err = AnalyzeInvestigation(context.Background(), fx.taskDir, cfg, ds)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.False(t, state.Exists(fx.taskDir), "a diagnosis does not create implementation authority")
	report, err := metrics.ReadReport(filepath.Join(fx.taskDir, "metrics.jsonl"))
	require.NoError(t, err)
	require.Equal(t, 1, report.TotalCalls)
	accounted := &state.State{Budget: state.Budget{Daily: map[string]state.DailyBucket{}, Cache: state.CacheStats{ByProvider: map[string]state.ProviderCache{}}}}
	accountPreparationUsage(accounted, result)
	require.EqualValues(t, 128, accounted.Budget.Totals.Tokens)
	require.EqualValues(t, 1, accounted.Budget.UnknownCostCalls)
}

func TestAnalysisCASRecoveryConsumesReceiptWithoutAnotherDispatchOrMetric(t *testing.T) {
	fx, st, cfg := analysisFixture(t)
	oldSave := saveAnalysisState
	t.Cleanup(func() { saveAnalysisState = oldSave })
	sentinel := errors.New("report CAS interrupted")
	saveAnalysisState = func(dir string, next *state.PreparationState) (*state.PreparationState, error) {
		if next.Analysis != nil {
			return nil, sentinel
		}
		return oldSave(dir, next)
	}
	calls := 0
	ds := investigationDriver(func(a drivers.CallArgs) drivers.Result {
		calls++
		return analysisResult(t, st, a.RequestID, "")
	})
	result, err := AnalyzeInvestigation(context.Background(), fx.taskDir, cfg, ds)
	require.ErrorIs(t, err, sentinel)
	require.Nil(t, result.Report, "an uncommitted report must not be presented as adopted")
	saveAnalysisState = oldSave
	result, err = AnalyzeInvestigation(context.Background(), fx.taskDir, cfg, ds)
	require.NoError(t, err)
	require.Equal(t, "supported", result.Status)
	require.Equal(t, 1, calls)
	report, err := metrics.ReadReport(filepath.Join(fx.taskDir, "metrics.jsonl"))
	require.NoError(t, err)
	require.Equal(t, 1, report.TotalCalls)
}

func TestUnknownAnalysisRecoversReadOnlyUsingFrozenIdentityAndCannotBeReplayed(t *testing.T) {
	fx, st, cfg := analysisFixture(t)
	calls, recovered := 0, 0
	ds := investigationDriver(func(a drivers.CallArgs) drivers.Result {
		calls++
		require.NoError(t, analysisLifecycle(a, "native-original", 1))
		result := analysisResult(t, st, a.RequestID, "native-original")
		result.Session.Outcome, result.Session.CleanupConfirmed = "unknown", false
		return result
	})
	blocked, err := AnalyzeInvestigation(context.Background(), fx.taskDir, cfg, ds)
	require.ErrorContains(t, err, "analysis-outcome-unknown")
	require.Equal(t, "blocked", blocked.Status)
	_, err = investigation.Finish(fx.taskDir, supportedPreparationReport(st), investigation.Options{})
	require.ErrorContains(t, err, "unresolved")
	_, err = investigation.Cancel(fx.taskDir, investigation.Options{})
	require.ErrorContains(t, err, "unresolved")
	same, err := investigation.Run(context.Background(), fx.taskDir, investigation.Options{})
	require.NoError(t, err)
	require.Equal(t, blocked.Revision, same.Revision)
	oldRecover := recoverInvestigationAnalysis
	t.Cleanup(func() { recoverInvestigationAnalysis = oldRecover })
	recoverInvestigationAnalysis = func(a drivers.MachineRecoveryArgs) (drivers.Result, error) {
		recovered++
		require.Equal(t, "gemini-4-neon", a.Model)
		require.Equal(t, "native-original", a.RunID)
		require.Equal(t, cfg.Subscription, a.Subscription)
		require.True(t, a.EvidenceOnly)
		return analysisResult(t, st, a.RequestID, a.RunID), nil
	}
	cfg.Loops["investigate"] = config.LoopBinding{Harness: "retarget", Model: "must-not-apply"}
	result, err := AnalyzeInvestigation(context.Background(), fx.taskDir, cfg, ds)
	require.NoError(t, err)
	require.True(t, result.Analysis.Recovered)
	require.Equal(t, "supported", result.Status)
	require.Equal(t, 1, calls)
	require.Equal(t, 1, recovered)
}

func TestAnalysisRejectsMalformedReportsAndAllowsExplicitHostReportAfterSettling(t *testing.T) {
	for _, scenario := range []string{"unknown field", "bad citation", "failed", "truncated", "wrong-model", "error-with-pass", "unknown-no-native-id"} {
		t.Run(scenario, func(t *testing.T) {
			fx, st, cfg := analysisFixture(t)
			calls := 0
			ds := investigationDriver(func(a drivers.CallArgs) drivers.Result {
				calls++
				r := analysisResult(t, st, a.RequestID, "")
				switch scenario {
				case "unknown field":
					r.Text = `{"version":1,"execute":"curl unapproved.example"}`
				case "bad citation":
					r.Text = strings.ReplaceAll(r.Text, st.Observations[0].Invocation.ID, strings.Repeat("f", 64))
				case "failed":
					r.Session.Outcome = "failed"
				case "truncated":
					r.Truncated = true
				case "wrong-model":
					r.Model = "foreign-model"
				case "error-with-pass":
					r.ErrorMessage = strPtr("The backend did not produce a trustworthy report")
				case "unknown-no-native-id":
					r.Session = nil
				}
				return r
			})
			result, err := AnalyzeInvestigation(context.Background(), fx.taskDir, cfg, ds)
			require.Error(t, err)
			require.Nil(t, result.Report)
			_, err = AnalyzeInvestigation(context.Background(), fx.taskDir, cfg, ds)
			require.Error(t, err)
			require.Equal(t, 1, calls)
			if scenario != "unknown-no-native-id" {
				result, err = investigation.Finish(fx.taskDir, supportedPreparationReport(st), investigation.Options{})
				require.NoError(t, err)
				require.Equal(t, "supported", result.Status)
			}
		})
	}
}

func TestAnalysisAdmissionRejectsInvalidInputsBeforeDispatch(t *testing.T) {
	fx, st, cfg := analysisFixture(t)
	ds := investigationDriver(func(drivers.CallArgs) drivers.Result { t.Fatal("unexpected dispatch"); return drivers.Result{} })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := AnalyzeInvestigation(ctx, fx.taskDir, cfg, ds)
	require.ErrorIs(t, err, context.Canceled)
	release, err := state.AcquirePreparationLease(fx.taskDir)
	require.NoError(t, err)
	_, err = AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.Error(t, err)
	release()
	_, err = AnalyzeInvestigation(nil, t.TempDir(), cfg, ds)
	require.Error(t, err)
	_, err = AnalyzeInvestigation(nil, fx.taskDir, cfg, drivers.Drivers{})
	require.ErrorContains(t, err, "configured investigate")
	limit := int64(0)
	cfg.Budget.Enforce, cfg.Budget.DailyTokenLimit = "hard", &limit
	_, err = AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.ErrorContains(t, err, "analysis budget")
	cfg.Budget.Enforce = "off"
	clock := analysisNow
	t.Cleanup(func() { analysisNow = clock })
	analysisNow = func() time.Time { return time.Now().Add(time.Hour) }
	_, err = AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.ErrorContains(t, err, "remaining time")
	analysisNow = clock
	binding := cfg.Loops["investigate"]
	binding.NativeProfile = "missing"
	cfg.Loops["investigate"] = binding
	_, err = AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.ErrorContains(t, err, "native profile")
	binding.NativeProfile = ""
	cfg.Loops["investigate"] = binding
	require.NoError(t, os.WriteFile(filepath.Join(fx.taskDir, ".sessions"), []byte("not a directory"), 0o600))
	_, err = AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.Error(t, err)
	st.Status = "ready"
	_, err = state.SavePreparation(fx.taskDir, st)
	require.NoError(t, err)
	_, err = AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.ErrorContains(t, err, "completed evidence collection")
	st.Contract.KnownEvidence = []string{strings.Repeat("x", 1<<20)}
	_, err = investigationAnalysisPrompt(st)
	require.ErrorContains(t, err, "bounded prompt")
}

func TestAnalysisPersistenceFailuresKeepIntentAndDoNotAuthorizeReplay(t *testing.T) {
	for _, scenario := range []string{"intent", "lifecycle", "encode", "receipt", "metrics"} {
		t.Run(scenario, func(t *testing.T) {
			fx, st, cfg := analysisFixture(t)
			oldSave, oldReceipt := saveAnalysisState, saveAnalysisReceipt
			t.Cleanup(func() { saveAnalysisState, saveAnalysisReceipt = oldSave, oldReceipt })
			if scenario == "intent" || scenario == "lifecycle" {
				saveAnalysisState = func(dir string, next *state.PreparationState) (*state.PreparationState, error) {
					if scenario == "intent" || next.AnalysisPending != nil && next.AnalysisPending.Sequence > 0 {
						return nil, errors.New("injected write failure")
					}
					return oldSave(dir, next)
				}
			}
			if scenario == "receipt" {
				saveAnalysisReceipt = func(string, state.PreparationAnalysisReceipt, bool) (string, error) {
					return "", errors.New("receipt fsync failure")
				}
			}
			if scenario == "metrics" {
				require.NoError(t, os.Remove(filepath.Join(fx.taskDir, "metrics.jsonl")))
				require.NoError(t, os.Mkdir(filepath.Join(fx.taskDir, "metrics.jsonl"), 0o700))
			}
			calls := 0
			ds := investigationDriver(func(a drivers.CallArgs) drivers.Result {
				calls++
				r := analysisResult(t, st, a.RequestID, "")
				if scenario == "lifecycle" {
					require.Error(t, analysisLifecycle(a, "native", 1))
					r.Session = nil
				}
				if scenario == "encode" {
					nan := math.NaN()
					r.CostUSD = &nan
				}
				return r
			})
			_, err := AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
			require.Error(t, err)
			if scenario == "intent" {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
				stored, err := state.LoadPreparation(fx.taskDir)
				require.NoError(t, err)
				require.NotNil(t, stored.AnalysisPending)
				require.Nil(t, stored.Report)
			}
		})
	}
}

func TestAnalysisLifecycleValidationAndExplicitNativeConfiguration(t *testing.T) {
	fx, st, cfg := analysisFixture(t)
	binding := cfg.Loops["investigate"]
	binding.NativeProfile = "analysis-native"
	cfg.Loops["investigate"] = binding
	cfg.NativeProfiles = map[string]config.NativeProfile{"analysis-native": {Harness: "antigravity", Options: json.RawMessage(`{"version":1}`)}}
	cfg.Watchdog.WallTimeoutMs = 120000
	ds := investigationDriver(func(a drivers.CallArgs) drivers.Result {
		require.JSONEq(t, `{"version":1}`, string(a.Native))
		require.LessOrEqual(t, a.Watchdog.WallTimeoutMs, int64(60000))
		require.Error(t, analysisLifecycle(a, "native", 2), "out-of-order ACK cannot advance durable identity")
		require.NoError(t, analysisLifecycle(a, "native", 1))
		require.Error(t, analysisLifecycle(a, "foreign", 2))
		return analysisResult(t, st, a.RequestID, "native")
	})
	_, err := AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.NoError(t, err)
}

func blockedAnalysisFixture(t *testing.T) (fixture, *state.PreparationState, config.Config, drivers.Drivers) {
	t.Helper()
	fx, st, cfg := analysisFixture(t)
	ds := investigationDriver(func(a drivers.CallArgs) drivers.Result {
		require.NoError(t, analysisLifecycle(a, "native-blocked", 1))
		r := analysisResult(t, st, a.RequestID, "native-blocked")
		r.Session.Outcome = "unknown"
		return r
	})
	blocked, err := AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.Error(t, err)
	ds.Investigate = &fnDriver{role: "investigate", log: &scriptLog{}, fn: func(drivers.CallArgs) drivers.Result {
		t.Fatal("an unresolved analysis was replayed")
		return drivers.Result{}
	}}
	return fx, blocked, cfg, ds
}

func TestAnalysisReadOnlyRecoveryFailuresRetainOriginalIntent(t *testing.T) {
	for _, scenario := range []string{"corrupt-envelope", "corrupt-result", "query-failed", "foreign-run", "encode", "receipt", "block-write"} {
		t.Run(scenario, func(t *testing.T) {
			fx, st, cfg, ds := blockedAnalysisFixture(t)
			request := st.AnalysisPending.Request
			oldRecover, oldReceipt, oldSave := recoverInvestigationAnalysis, saveAnalysisReceipt, saveAnalysisState
			t.Cleanup(func() {
				recoverInvestigationAnalysis, saveAnalysisReceipt, saveAnalysisState = oldRecover, oldReceipt, oldSave
			})
			recoverInvestigationAnalysis = func(a drivers.MachineRecoveryArgs) (drivers.Result, error) {
				r := analysisResult(t, st, a.RequestID, a.RunID)
				switch scenario {
				case "query-failed", "block-write":
					return drivers.Result{}, errors.New(strings.Repeat("native unavailable ", 700))
				case "foreign-run":
					r.Session.RunID = "another-native-run"
				case "encode":
					nan := math.NaN()
					r.CostUSD = &nan
				}
				return r, nil
			}
			if strings.HasPrefix(scenario, "corrupt-") {
				path := filepath.Join(fx.taskDir, ".investigation", "analysis-"+state.PreparationHash(request.ID)+"-original.json")
				raw := []byte(`{}`)
				if scenario == "corrupt-result" {
					raw, _ = json.Marshal(state.PreparationAnalysisReceipt{Version: 1, Request: request, Result: json.RawMessage(`{"settled":"invalid-type"}`)})
				}
				require.NoError(t, os.WriteFile(path, raw, 0o600))
			}
			if scenario == "receipt" {
				saveAnalysisReceipt = func(string, state.PreparationAnalysisReceipt, bool) (string, error) {
					return "", errors.New("recovery receipt unavailable")
				}
			}
			if scenario == "block-write" {
				saveAnalysisState = func(string, *state.PreparationState) (*state.PreparationState, error) {
					return nil, errors.New("blocked-state fsync failed")
				}
			}
			result, err := AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
			require.Error(t, err)
			require.NotNil(t, result.AnalysisPending)
			require.Nil(t, result.Report)
			if scenario == "query-failed" {
				require.Len(t, result.Reason, 8192)
			}
		})
	}
}

func TestAnalysisRejectsCorruptTypedReceiptAndBoundsValidationDiagnostic(t *testing.T) {
	fx, st, cfg, ds := blockedAnalysisFixture(t)
	_, err := adoptAnalysis(fx.taskDir, st, state.PreparationAnalysisReceipt{Request: st.AnalysisPending.Request, Result: json.RawMessage(`{"settled":"invalid"}`)}, "ignored", false)
	require.Error(t, err)
	fx, st, cfg = analysisFixture(t)
	ds = investigationDriver(func(a drivers.CallArgs) drivers.Result {
		r := analysisResult(t, st, a.RequestID, "")
		report := supportedPreparationReport(st)
		report.Findings = make([]state.InvestigationFinding, 400)
		raw, marshalErr := json.Marshal(map[string]any{"version": 1, "report": report})
		require.NoError(t, marshalErr)
		r.Text = string(raw)
		return r
	})
	result, err := AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.Error(t, err)
	require.Len(t, result.Analysis.Error, 8192)
}

func TestManagedInvestigationAutomaticallyAnalyzesConfiguredRole(t *testing.T) {
	fx, st, cfg := analysisFixture(t)
	oldConfiguration, oldDrivers := investigationConfiguration, investigationDrivers
	t.Cleanup(func() { investigationConfiguration, investigationDrivers = oldConfiguration, oldDrivers })
	investigationConfiguration = func(context.Context, string, string) (*config.Config, error) { return &cfg, nil }
	calls := 0
	investigationDrivers = func(*config.Config) *drivers.Drivers {
		ds := investigationDriver(func(a drivers.CallArgs) drivers.Result {
			calls++
			return analysisResult(t, st, a.RequestID, "")
		})
		return &ds
	}
	result, err := RunInvestigation(nil, fx.taskDir, st.Revision)
	require.NoError(t, err)
	require.Equal(t, "supported", result.Status)
	require.Equal(t, 1, calls)
	fx, st, _ = analysisFixture(t)
	investigationConfiguration = func(context.Context, string, string) (*config.Config, error) {
		return nil, errors.New("config became unavailable")
	}
	_, err = RunInvestigation(nil, fx.taskDir, st.Revision)
	require.ErrorContains(t, err, "config became unavailable")
}

func TestAnalysisPromptBoundsAccumulatedImmutableReports(t *testing.T) {
	fx, first, cfg := analysisFixture(t)
	report := supportedPreparationReport(first)
	report.Summary, report.CounterEvidence, report.Recommendation, report.Regression, report.Rollback = strings.Repeat("s", 8192), strings.Repeat("c", 8192), strings.Repeat("r", 8192), strings.Repeat("g", 8192), strings.Repeat("b", 8192)
	for i := range report.Findings {
		report.Findings[i].Reason = strings.Repeat("e", 8192)
	}
	st, err := investigation.Finish(fx.taskDir, report, investigation.Options{})
	require.NoError(t, err)
	for i := 0; i < 19; i++ {
		p := st.Contract
		p.KnownEvidence = []string{"The user supplied additional context for the same task."}
		p.Limits = state.PreparationRemaining(st)
		raw, marshalErr := json.Marshal(p)
		require.NoError(t, marshalErr)
		st, err = investigation.Continue(fx.taskDir, raw, investigation.Options{ExpectedRevision: st.Revision})
		require.NoError(t, err)
		if i < 18 {
			st, err = investigation.Finish(fx.taskDir, report, investigation.Options{})
			require.NoError(t, err)
		}
	}
	st, err = investigation.Run(context.Background(), fx.taskDir, investigation.Options{})
	require.NoError(t, err)
	ds := investigationDriver(func(drivers.CallArgs) drivers.Result {
		t.Fatal("oversized evidence was dispatched")
		return drivers.Result{}
	})
	_, err = AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.ErrorContains(t, err, "bounded prompt")
}

func TestAnalysisRoundHandoffImportsAllUsageExactlyOnce(t *testing.T) {
	fx, first, cfg := analysisFixture(t)
	// This fixture is a fresh report-only task, without legacy implementation
	// contracts that would require the separate migration command.
	require.NoError(t, os.Remove(filepath.Join(fx.taskDir, "spec.md")))
	ds := investigationDriver(func(a drivers.CallArgs) drivers.Result { return analysisResult(t, first, a.RequestID, "") })
	first, err := AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.NoError(t, err)
	p := first.Contract
	p.KnownEvidence, p.Limits = []string{"Additional diagnostic evidence was supplied."}, state.PreparationRemaining(first)
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	_, err = investigation.Continue(fx.taskDir, raw, investigation.Options{ExpectedRevision: first.Revision})
	require.NoError(t, err)
	second, err := investigation.Run(context.Background(), fx.taskDir, investigation.Options{})
	require.NoError(t, err)
	ds = investigationDriver(func(a drivers.CallArgs) drivers.Result {
		r := analysisResult(t, second, a.RequestID, "")
		r.Usage, r.Session.UsageStatus = nil, "unknown"
		cost := .25
		r.CostUSD = &cost
		return r
	})
	second, err = AnalyzeInvestigation(nil, fx.taskDir, cfg, ds)
	require.NoError(t, err)
	budget := &state.State{Budget: state.Budget{Daily: map[string]state.DailyBucket{}, Cache: state.CacheStats{ByProvider: map[string]state.ProviderCache{}}}}
	c := &Context{State: budget}
	require.NoError(t, c.importPreparation(second))
	require.Equal(t, 2, budget.PreparationSource.Round)
	require.Equal(t, 6, budget.PreparationSource.Actions)
	require.EqualValues(t, 128, budget.Budget.Totals.Tokens)
	require.Equal(t, .25, budget.Budget.Totals.CostUSD)
	require.EqualValues(t, 1, budget.Budget.UnknownUsageCalls)
	require.EqualValues(t, 1, budget.Budget.UnknownCostCalls)
	require.NoError(t, c.importPreparation(second))
	require.Equal(t, .25, budget.Budget.Totals.CostUSD)
	require.EqualValues(t, 1, budget.Budget.UnknownUsageCalls)
	// Exercise the actual reviewed-planning admission and schema-backed CAS,
	// not only the in-memory import. Report-only rounds retain their empty
	// repair scope until a separate exact repair grant is recorded.
	brief := planning.Brief{ID: fx.slug, Goal: "Review the accumulated investigation", OriginalRequest: "Discuss the report before authorizing a repair"}
	opts := RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: &cfg}
	_, _, err = ControlPlanning(opts, PlanningCommand{Action: "prepare", Brief: &brief, Start: new(false)}, "")
	require.NoError(t, err)
	saved, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	require.Equal(t, 2, saved.PreparationSource.Round)
	require.Empty(t, saved.PreparationSource.ImplementationScope)
	require.Equal(t, budget.PreparationSource, saved.PreparationSource)
	require.Equal(t, budget.Budget.Totals, saved.Budget.Totals)
}
