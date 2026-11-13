package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

const approvedDiagnosticResponse = `{"model":"jev-1.14.0","answers":{"applicable":{"type":"noul","noul":0.99},"action":{"type":"choice","choice":"find-default","confidence":0.8,"probabilities":{"find-default":0.9,"read-settings":0.05,"list-source":0.05}}},"usage":{"input_tokens":12,"output_tokens":4}}`

func withDecision(p *contract.Preparation, endpoint string) {
	p.Decision = &contract.InvestigationDecision{Policy: decision.Policy{Version: "experiment-1", Mode: decision.Adopt, AllowedData: []decision.Category{decision.TaskSummary, decision.DiagnosticFacts, decision.Source}, MinApplicability: .9, MinChoice: .8, MaxCalls: 1}, Endpoint: endpoint, Model: "jev-1.14.0", KeyEnv: "INVESTIGATION_TEST_KEY", TimeoutMS: 1000}
}

func reserveDecision(t *testing.T, dir string, st *state.PreparationState) *state.PreparationState {
	t.Helper()
	identity, err := candidate(context.Background(), st)
	require.NoError(t, err)
	s := snapshot(st, remainingActions(st), identity)
	st.DecisionPending = &s
	if st.DecisionStartedAt == nil {
		st.DecisionStartedAt = map[string]string{}
	}
	st.DecisionStartedAt[s.ID] = time.Now().Format(time.RFC3339Nano)
	require.NoError(t, persist(dir, &st, Options{}))
	return st
}

func blockPreparationWrites(t *testing.T, dir string) {
	t.Helper()
	lock := filepath.Join(dir, ".state.lock")
	require.NoError(t, os.Remove(lock))
	require.NoError(t, os.Mkdir(lock, 0o700))
}

func allowPreparationWrites(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.Remove(filepath.Join(dir, ".state.lock")))
}

func TestInvestigationStartAndOwnershipFailuresPreserveExistingWork(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	_, err = Start(dir, cwd, []byte(`{}`), Options{})
	require.Error(t, err)
	_, err = Start(dir, filepath.Join(cwd, "missing"), raw, Options{})
	require.Error(t, err)
	_, err = Start(filepath.Join(cwd, "settings.json", "child"), cwd, raw, Options{})
	require.Error(t, err)
	release, err := state.AcquirePreparationLease(dir)
	require.NoError(t, err)
	_, err = Start(dir, cwd, raw, Options{})
	require.ErrorContains(t, err, "owner")
	_, err = Run(context.Background(), dir, Options{})
	require.ErrorContains(t, err, "owner")
	release()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"existing":"implementation"}`), 0o600))
	_, err = Start(dir, cwd, raw, Options{})
	require.ErrorContains(t, err, "implementation task already exists")
	unchanged, err := os.ReadFile(filepath.Join(dir, "state.json"))
	require.NoError(t, err)
	require.Equal(t, `{"existing":"implementation"}`, string(unchanged))
	_, err = Finish(t.TempDir(), state.InvestigationReport{}, Options{})
	require.Error(t, err)
	_, err = Cancel(t.TempDir(), Options{})
	require.Error(t, err)

	t.Run("working-directory-resolution-fails", func(t *testing.T) {
		original := absoluteInvestigationPath
		t.Cleanup(func() { absoluteInvestigationPath = original })
		failure := &os.PathError{Op: "getwd", Err: os.ErrNotExist}
		absoluteInvestigationPath = func(string) (string, error) { return "", failure }
		_, err := Start(dir, ".", raw, Options{})
		require.ErrorIs(t, err, failure)
		preserved, err := os.ReadFile(filepath.Join(dir, "state.json"))
		require.NoError(t, err)
		require.Equal(t, unchanged, preserved)
	})
}

func TestDecisionReceiptAndActionIntentRecoverAcrossFailedCAS(t *testing.T) {
	for _, invalidConfig := range []bool{false, true} {
		t.Run(fmt.Sprintf("config-invalid-%t", invalidConfig), func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				blockPreparationWrites(t, dir)
				if invalidConfig {
					require.NoError(t, os.WriteFile(filepath.Join(dir, "flywheel.config.json"), []byte(`{`), 0o600))
				}
				_, _ = w.Write([]byte(approvedDiagnosticResponse))
			}))
			defer server.Close()
			withDecision(&p, server.URL+"/v1/systemone")
			startFixture(t, dir, cwd, p)
			_, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
			require.Error(t, err)
			frozen, err := state.LoadPreparation(dir)
			require.NoError(t, err)
			require.NotNil(t, frozen.DecisionPending, "failed CAS must retain the recoverable frozen request")
			require.Nil(t, frozen.Pending)
			require.Empty(t, frozen.Decisions)
			require.Zero(t, frozen.ActionsUsed)
			require.EqualValues(t, 1, requests.Load())
			allowPreparationWrites(t, dir)
			st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { t.Fatal("recovery must not need credentials"); return "", false }})
			require.NoError(t, err)
			require.Equal(t, "awaiting-report", st.Status)
			require.Len(t, st.Decisions, 1)
			require.Equal(t, "ok", st.Decisions[0].Status)
			require.EqualValues(t, 12, *st.Decisions[0].Usage.Input)
			require.EqualValues(t, 1, requests.Load(), "restart consumes the original terminal without another HTTP call")
			require.Equal(t, !invalidConfig, st.Observations[0].Invocation.Adopted)
			if invalidConfig {
				require.Equal(t, "read-settings", st.Observations[0].Invocation.ActionID)
			} else {
				require.Equal(t, "find-default", st.Observations[0].Invocation.ActionID)
			}
			require.Len(t, st.Observations, 3)
			require.Nil(t, st.DecisionPending)
		})
	}
}

func TestUnpersistedDecisionIntentCannotDispatchAdviceOrAction(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(approvedDiagnosticResponse))
	}))
	defer server.Close()
	withDecision(&p, server.URL+"/v1/systemone")
	startFixture(t, dir, cwd, p)
	blockPreparationWrites(t, dir)
	_, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
	require.Error(t, err)
	st, err := state.LoadPreparation(dir)
	require.NoError(t, err)
	require.Zero(t, st.ActionsUsed)
	require.Nil(t, st.Pending)
	require.Nil(t, st.DecisionPending)
	require.Zero(t, requests.Load())
}

func TestReceiptSurvivesFailedConsumptionAndIsNeverReexecuted(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	p.Limits.MaxActions = 1
	startFixture(t, dir, cwd, p)
	open := openInspectionFile
	t.Cleanup(func() { openInspectionFile = open })
	reads := 0
	openInspectionFile = func(root *os.Root, path string, flags int, mode os.FileMode) (*os.File, error) {
		if path == "settings.json" {
			reads++
			if reads == 3 { // two candidate inspections, then the dispatched action
				blockPreparationWrites(t, dir)
			}
		}
		return open(root, path, flags, mode)
	}
	_, err := Run(context.Background(), dir, Options{})
	require.Error(t, err)
	st, err := state.LoadPreparation(dir)
	require.NoError(t, err)
	require.NotNil(t, st.Pending)
	receipt, err := state.ReadPreparationReceipt(dir, *st.Pending)
	require.NoError(t, err)
	require.Equal(t, `{"timeout":0}`, receipt.Output)
	_, err = Run(context.Background(), dir, Options{})
	require.Error(t, err, "recovery also requires a successful state CAS")
	require.Equal(t, 3, reads)
	allowPreparationWrites(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "settings.json"), []byte(`{"timeout":99}`), 0o600))
	st, err = Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, 3, reads, "reconciliation must not reread a completed action")
	require.Equal(t, "awaiting-report", st.Status)
	require.Len(t, st.Observations, 1)
	require.Equal(t, receipt.Output, st.Observations[0].Output)
	require.Equal(t, 1, st.ActionsUsed)
}

func TestActionFaultsRetainUnknownIntentOrRecoverPublishedReceipt(t *testing.T) {
	for _, fault := range []string{"phase-start", "receipt", "phase-end"} {
		t.Run(fault, func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			p.Limits.MaxActions = 1
			startFixture(t, dir, cwd, p)
			metricFile := filepath.Join(dir, "metrics.jsonl")
			if fault == "phase-start" {
				require.NoError(t, os.Mkdir(metricFile, 0o700))
			}
			if fault == "receipt" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".investigation"), nil, 0o600))
			}
			open := openInspectionFile
			t.Cleanup(func() { openInspectionFile = open })
			reads := 0
			openInspectionFile = func(root *os.Root, path string, flags int, mode os.FileMode) (*os.File, error) {
				if path == "settings.json" {
					reads++
					if reads == 3 && fault == "phase-end" {
						require.NoError(t, os.Rename(metricFile, metricFile+".saved"))
						require.NoError(t, os.Mkdir(metricFile, 0o700))
					}
				}
				return open(root, path, flags, mode)
			}
			_, err := Run(context.Background(), dir, Options{})
			require.Error(t, err)
			st, err := state.LoadPreparation(dir)
			require.NoError(t, err)
			require.NotNil(t, st.Pending)
			require.Equal(t, 1, st.ActionsUsed)
			if fault == "phase-start" {
				require.Equal(t, 2, reads, "a failed phase start cannot dispatch the action")
			} else {
				require.Equal(t, 3, reads)
			}
			if fault == "receipt" {
				require.NoError(t, os.Remove(filepath.Join(dir, ".investigation")))
			} else {
				require.NoError(t, os.Remove(metricFile))
			}
			if fault == "phase-end" {
				require.NoError(t, os.Rename(metricFile+".saved", metricFile))
			}
			observedReads := reads
			st, err = Run(context.Background(), dir, Options{})
			require.NoError(t, err)
			require.Equal(t, observedReads, reads)
			if fault == "phase-end" {
				require.Equal(t, "awaiting-report", st.Status)
				require.Len(t, st.Observations, 1)
				report, err := metrics.ReadReport(metricFile)
				require.NoError(t, err)
				require.Equal(t, 1, report.Timeline.Unknown, "a missing phase end remains unknown after action recovery")
			} else {
				require.Equal(t, "blocked", st.Status)
				require.Empty(t, st.Observations)
				_, err = Cancel(dir, Options{})
				require.ErrorContains(t, err, "unresolved")
			}
		})
	}
}

func TestCancellationAtInspectionBoundariesNeverDispatchesExtraWork(t *testing.T) {
	for _, readNumber := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(readNumber), func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			startFixture(t, dir, cwd, p)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			open := openInspectionFile
			t.Cleanup(func() { openInspectionFile = open })
			reads := 0
			openInspectionFile = func(root *os.Root, path string, flags int, mode os.FileMode) (*os.File, error) {
				if path == "settings.json" {
					reads++
					if reads == readNumber {
						cancel()
					}
				}
				return open(root, path, flags, mode)
			}
			st, err := Run(ctx, dir, Options{})
			require.NoError(t, err)
			require.Equal(t, "cancelled", st.Status)
			require.Nil(t, st.Pending)
			if readNumber == 3 {
				require.Equal(t, 1, st.ActionsUsed)
				require.Equal(t, "cancelled", st.Observations[0].Status)
				require.Contains(t, st.Observations[0].Error, "context canceled")
			} else {
				require.Zero(t, st.ActionsUsed)
				require.Empty(t, st.Observations)
			}
		})
	}
}

func TestCancellationDuringAdviceSettlesItWithoutDispatchingAction(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		cancel()
		_, _ = w.Write([]byte(approvedDiagnosticResponse))
	}))
	defer server.Close()
	withDecision(&p, server.URL+"/v1/systemone")
	startFixture(t, dir, cwd, p)
	st, err := Run(ctx, dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
	require.NoError(t, err)
	require.Equal(t, "cancelled", st.Status)
	require.Nil(t, st.DecisionPending)
	require.Len(t, st.Decisions, 1)
	require.Zero(t, st.ActionsUsed)
	require.EqualValues(t, 1, requests.Load())
}

func TestChangedCandidateRejectsOtherwiseValidAdvice(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.NoError(t, os.WriteFile(filepath.Join(cwd, "settings.json"), []byte(`{"timeout":55}`), 0o600))
		_, _ = w.Write([]byte(approvedDiagnosticResponse))
	}))
	defer server.Close()
	withDecision(&p, server.URL+"/v1/systemone")
	startFixture(t, dir, cwd, p)
	st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
	require.NoError(t, err)
	require.Equal(t, "ok", st.Decisions[0].Status)
	require.False(t, st.Observations[0].Invocation.Adopted)
	require.Equal(t, "read-settings", st.Observations[0].Invocation.ActionID)
	require.Equal(t, "candidate-changed-baseline", st.Observations[0].Invocation.Reason)
	require.Equal(t, `{"timeout":55}`, st.Observations[0].Output)
	require.EqualValues(t, 1, requests.Load())
}

func TestUnavailableOptionalAdviceDoesNotBlockApprovedDiagnostics(t *testing.T) {
	for _, fault := range []string{"config", "goal-bounds", "hypothesis-bounds", "journal", "missing-key"} {
		t.Run(fault, func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			withDecision(&p, "http://127.0.0.1:1/v1/systemone")
			if fault == "goal-bounds" {
				p.Goal = strings.Repeat("x", 2049)
			}
			if fault == "hypothesis-bounds" {
				p.Hypotheses[0].Claim = strings.Repeat("x", 2048)
			}
			st := startFixture(t, dir, cwd, p)
			if fault == "config" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "flywheel.config.json"), []byte(`{`), 0o600))
			}
			if fault == "journal" {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".investigation", "decisions"), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".investigation", "decisions", st.RunID), nil, 0o600))
			}
			t.Setenv(p.Decision.KeyEnv, "")
			options := Options{}
			if fault == "journal" {
				options.LookupEnv = func(string) (string, bool) { return "key", true }
			}
			st, err := Run(context.Background(), dir, options)
			require.NoError(t, err)
			require.Equal(t, "awaiting-report", st.Status)
			require.Equal(t, "read-settings", st.Observations[0].Invocation.ActionID)
			require.False(t, st.Observations[0].Invocation.Adopted)
			require.Len(t, st.Observations, 3)
			switch fault {
			case "config", "goal-bounds", "hypothesis-bounds":
				require.Empty(t, st.Decisions, "no dispatch was reserved for unavailable optional advice")
			case "journal":
				require.Len(t, st.Decisions, 1)
				require.Equal(t, "unknown", st.Decisions[0].Status)
				require.Nil(t, st.Decisions[0].Usage.Input)
			case "missing-key":
				require.Equal(t, "skipped", st.Decisions[0].Status)
				require.Zero(t, *st.Decisions[0].Usage.Input)
			}
		})
	}
}

func TestSnapshotBoundsObservationExcerptWithoutSplittingUTF8(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	p.Limits.MaxActions = 1
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "settings.json"), []byte(strings.Repeat("中", 500)), 0o600))
	startFixture(t, dir, cwd, p)
	st, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	s := snapshot(st, remainingActions(st), state.PreparationHash("current"))
	source := s.Facts[len(s.Facts)-1]
	require.Equal(t, decision.Source, source.Category)
	require.True(t, utf8.ValidString(source.Text))
	require.True(t, strings.HasSuffix(source.Text, strings.Repeat("中", 341)))
	require.NotContains(t, source.Text, strings.Repeat("中", 342))
	require.Contains(t, source.Text, st.Observations[0].Invocation.ID)
}

func unknownReport(st *state.PreparationState) state.InvestigationReport {
	r := state.InvestigationReport{Status: "inconclusive", Summary: "No completed observation supports a diagnosis.", CounterEvidence: "The approved hypotheses have not yet been tested.", Limitations: []string{"The investigation stopped before collecting evidence."}, Recommendation: "Review the evidence needs before authorizing another investigation.", Regression: "Keep the original reproducer.", Rollback: "No repository change was performed."}
	for _, h := range st.Contract.Hypotheses {
		r.Findings = append(r.Findings, state.InvestigationFinding{Hypothesis: h.ID, Status: "unknown", Evidence: []string{}, Reason: "No successful observation is available."})
	}
	return r
}

func TestPendingAdviceCanBeSettledAtEveryTerminationBoundary(t *testing.T) {
	for _, stop := range []string{"deadline", "cancel", "report", "budget", "corrupt-journal"} {
		t.Run(stop, func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = w.Write([]byte(approvedDiagnosticResponse))
			}))
			defer server.Close()
			withDecision(&p, server.URL+"/v1/systemone")
			if stop == "budget" {
				p.Limits.MaxActions = 1
			}
			st := startFixture(t, dir, cwd, p)
			if stop == "budget" {
				// The durable observation consumes the last action allowance. Even
				// an outstanding optional request cannot reopen that allowance.
				a := p.Actions[0]
				invocation := state.PreparationInvocation{ID: state.PreparationHash("completed"), ActionID: a.ID, BindingHash: state.PreparationHash(a), CandidateHash: state.PreparationHash("observed-candidate"), StartedAt: time.Now().Format(time.RFC3339Nano), Reason: "baseline"}
				observation := state.InvestigationObservation{Invocation: invocation, Status: "observed", Output: "observation", ContentHash: digest([]byte("observation")), FinishedAt: time.Now().Format(time.RFC3339Nano)}
				st.ActionsUsed = 1
				consume(st, observation)
				require.NoError(t, persist(dir, &st, Options{}))
			}
			st = reserveDecision(t, dir, st)
			decisionID := st.DecisionPending.ID
			// Frozen recovery must not depend on current global configuration.
			require.NoError(t, os.WriteFile(filepath.Join(dir, "flywheel.config.json"), []byte(`{`), 0o600))
			if stop == "corrupt-journal" {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".investigation", "decisions"), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".investigation", "decisions", st.RunID), nil, 0o600))
			}
			var err error
			switch stop {
			case "deadline":
				st, err = Run(context.Background(), dir, Options{Now: func() time.Time { return time.Now().Add(time.Hour) }})
			case "budget":
				st, err = Run(context.Background(), dir, Options{})
			case "report":
				st, err = Finish(dir, unknownReport(st), Options{})
			default:
				st, err = Cancel(dir, Options{})
			}
			require.NoError(t, err)
			require.Nil(t, st.DecisionPending)
			require.Nil(t, st.Pending)
			require.Len(t, st.Decisions, 1)
			require.Equal(t, decisionID, st.Decisions[0].DecisionID)
			require.Equal(t, "unknown", st.Decisions[0].Status)
			require.Nil(t, st.Decisions[0].Usage.Input, "lack of a terminal never means zero usage")
			require.Zero(t, requests.Load(), "termination recovery must not send an HTTP request")
			if stop == "budget" {
				require.Equal(t, 1, st.ActionsUsed)
			} else {
				require.Zero(t, st.ActionsUsed)
			}
			if stop == "report" {
				require.Equal(t, "inconclusive", st.Status)
				require.NotNil(t, st.Report)
			} else if stop == "deadline" || stop == "budget" {
				require.Equal(t, "awaiting-report", st.Status)
			} else {
				require.Equal(t, "cancelled", st.Status)
			}
			loaded, err := state.LoadPreparation(dir)
			require.NoError(t, err)
			require.Equal(t, st, loaded)
		})
	}
}

func TestCancelRecoversKnownAdviceUsageWithoutAdoptingTheAction(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(approvedDiagnosticResponse))
	}))
	defer server.Close()
	withDecision(&p, server.URL+"/v1/systemone")
	st := reserveDecision(t, dir, startFixture(t, dir, cwd, p))
	receipt, err := decision.Evaluate(context.Background(), filepath.Join(dir, ".investigation", "decisions", st.RunID), *st.DecisionPending, st.Contract.Decision.Policy, decision.Options{Endpoint: p.Decision.Endpoint, Model: p.Decision.Model, ExpectedModel: p.Decision.Model, APIKey: "key", Timeout: time.Second})
	require.NoError(t, err)
	require.Equal(t, "ok", receipt.Status)
	st, err = Cancel(dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "cancelled", st.Status)
	require.Equal(t, []decision.Receipt{receipt}, st.Decisions)
	require.Zero(t, st.ActionsUsed)
	require.Nil(t, st.Pending)
	require.EqualValues(t, 1, requests.Load())
}

func TestRecoveredAdviceCannotChooseWithoutCurrentCandidateIdentity(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	withDecision(&p, "http://127.0.0.1:1/v1/systemone")
	st := reserveDecision(t, dir, startFixture(t, dir, cwd, p))
	legal := remainingActions(st)
	action, selection, err := choose(context.Background(), dir, &st, legal, "", Options{})
	require.NoError(t, err)
	require.Equal(t, legal[0], action)
	require.Equal(t, "deterministic-baseline", selection.Reason)
	require.False(t, selection.Adopted)
	require.Nil(t, st.DecisionPending)
	require.Equal(t, "unknown", st.Decisions[0].Status)
	unchanged, err := state.LoadPreparation(dir)
	require.NoError(t, err)
	require.NotNil(t, unchanged.DecisionPending, "selection itself cannot publish a state transition without its caller's action CAS")
	require.Empty(t, unchanged.Decisions)
}

func TestCounterEvidenceAndRecommendationNeverExpandImplementationScope(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	p.Intent, p.ImplementationScope = "investigate-and-fix", []string{"src/loader.go"}
	startFixture(t, dir, cwd, p)
	st, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	r := reportFixture(st)
	r.Findings[1].Status = "contradicted"
	r.Findings[1].Evidence = []string{st.Observations[1].Invocation.ID}
	r.Findings[1].Reason = "The cited source is counter-evidence to this hypothesis."
	r.Recommendation = "A different configuration may also need investigation before any broader repair is authorized."
	st, err = Finish(dir, r, Options{})
	require.NoError(t, err)
	require.Equal(t, []string{"src/loader.go"}, st.Contract.ImplementationScope)
	require.Equal(t, "contradicted", st.Report.Findings[1].Status)
	require.False(t, state.Exists(dir), "a diagnosis report never creates completed implementation state")
	raw, err := os.ReadFile(filepath.Join(cwd, "settings.json"))
	require.NoError(t, err)
	require.Equal(t, `{"timeout":0}`, string(raw))
}
