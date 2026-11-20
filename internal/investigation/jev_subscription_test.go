package investigation

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func subscriptionPreparation(t *testing.T) (string, *state.PreparationState) {
	t.Helper()
	dir, cwd, p := preparationFixture(t)
	changeRuntimeConfig(t, dir, func(values map[string]any) {
		values["jev"] = map[string]any{"enabled": true, "backend": "subscription", "profile": "hydrogen-local", "max-calls": 1}
	})
	return dir, startFixture(t, dir, cwd, p)
}

func fakeSubscription(t *testing.T, hook func(string) decision.SubscriptionResult, recover func(string) decision.SubscriptionResult) {
	t.Helper()
	saved := subscriptionOptions
	t.Cleanup(func() { subscriptionOptions = saved })
	subscriptionOptions = func(_ context.Context, b *decision.SubscriptionBinding, cwd, dir string, timeout time.Duration) decision.Options {
		require.Equal(t, "antigravity", b.Harness)
		require.Equal(t, "hydrogen-local", b.Profile)
		require.True(t, filepath.IsAbs(cwd))
		require.True(t, filepath.IsAbs(dir))
		return decision.Options{Backend: "subscription", Subscription: b, Model: b.Model, ExpectedModel: b.Model, Timeout: timeout,
			SubscriptionCall: func(_ context.Context, prompt, _ string) decision.SubscriptionResult { return hook(prompt) }, SubscriptionRecover: recover}
	}
}

func subscriptionAnswer(t *testing.T, prompt string) decision.SubscriptionResult {
	t.Helper()
	var request struct {
		Questions map[string]struct {
			Criteria map[string]string `json:"criteria"`
		} `json:"questions"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.SplitN(prompt, "\n", 2)[1]), &request))
	probabilities := map[string]float64{}
	for id := range request.Questions["action"].Criteria {
		probabilities[id] = 0
	}
	selected := "find-default"
	if _, ok := probabilities[selected]; !ok {
		for id := range probabilities {
			selected = id
			break
		}
	}
	probabilities[selected] = 1
	raw, err := json.Marshal(map[string]any{"action-id": selected, "applicability": 1, "choice-confidence": 1, "probabilities": probabilities})
	require.NoError(t, err)
	return decision.SubscriptionResult{Model: "gemini-4-hydrogen-medium", Text: string(raw), Native: decision.NativeExecution{RequestID: "jev-one", Dispatch: "accepted", Outcome: "completed", CleanupConfirmed: true, ReceiptHash: strings.Repeat("a", 64)}}
}

func TestInvestigationUsesSubscriptionProfileAndPreservesAuthorizedActions(t *testing.T) {
	dir, st := subscriptionPreparation(t)
	calls := 0
	fakeSubscription(t, func(prompt string) decision.SubscriptionResult { calls++; return subscriptionAnswer(t, prompt) }, nil)
	require.Equal(t, "subscription", st.Contract.Decision.Backend)
	require.Equal(t, "gemini-4-hydrogen-medium", st.Contract.Decision.Model)
	require.Empty(t, st.Contract.Decision.KeyEnv)
	st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { t.Fatal("subscription must not read an API key"); return "", false }})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "awaiting-report", st.Status)
	require.True(t, st.Observations[0].Invocation.Adopted)
	require.Equal(t, "find-default", st.Observations[0].Invocation.ActionID)
	require.Nil(t, st.Decisions[0].Usage.Input)
	_, err = Finish(dir, reportFixture(st), Options{})
	require.NoError(t, err)
}

func TestInvestigationRechecksResolvedSubscriptionBeforeAdopting(t *testing.T) {
	dir, _ := subscriptionPreparation(t)
	calls := 0
	fakeSubscription(t, func(prompt string) decision.SubscriptionResult {
		calls++
		changeRuntimeConfig(t, dir, func(values map[string]any) {
			p := values["profiles"].(map[string]any)["hydrogen-local"].(map[string]any)
			p["model"], p["thinking"] = "gemini-4-hydrogen-high", "high"
		})
		return subscriptionAnswer(t, prompt)
	}, nil)
	st, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "policy-changed", st.JEVStatus)
	require.False(t, st.Observations[0].Invocation.Adopted)
	require.Equal(t, "read-settings", st.Observations[0].Invocation.ActionID)
	cfg, err := config.LoadReadOnly(dir, st.Cwd)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.False(t, subscriptionDecisionCurrent(ctx, cfg, st.Contract.Decision))
	cfg.JEV.Backend = "http"
	require.False(t, subscriptionDecisionCurrent(context.Background(), cfg, st.Contract.Decision))
	frozen := freezeDecision(cfg, cfg.JEV.DecisionPolicy())
	require.Empty(t, frozen.Backend)
	cfg.JEV.Backend = "subscription"
	cfg.JEV.Profile = "absent"
	frozen = freezeDecision(cfg, cfg.JEV.DecisionPolicy())
	require.Nil(t, frozen.Subscription)
}

func TestInvestigationUnknownNativeCleanupBlocksActionsAndRecoversWithoutPrompt(t *testing.T) {
	for _, failSave := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved-block", true: "interrupted-block"}[failSave], func(t *testing.T) {
			dir, _ := subscriptionPreparation(t)
			calls, queries := 0, 0
			allowCleanup := false
			fakeSubscription(t, func(prompt string) decision.SubscriptionResult {
				calls++
				r := subscriptionAnswer(t, prompt)
				r.Native.CleanupConfirmed = false
				if failSave {
					blockPreparationWrites(t, dir)
				}
				return r
			}, func(string) decision.SubscriptionResult {
				queries++
				return decision.SubscriptionResult{Native: decision.NativeExecution{RequestID: "jev-one", Dispatch: "accepted", Outcome: "completed", CleanupConfirmed: allowCleanup, ReceiptHash: strings.Repeat("a", 64)}}
			})
			_, err := Run(context.Background(), dir, Options{})
			require.Error(t, err)
			if failSave {
				allowPreparationWrites(t, dir)
			}
			st, err := state.LoadPreparation(dir)
			require.NoError(t, err)
			require.NotNil(t, st.DecisionPending)
			require.Empty(t, st.Observations)
			_, err = Run(context.Background(), dir, Options{})
			require.ErrorContains(t, err, "cleanup requires reconciliation")
			allowCleanup = true
			st, err = Run(context.Background(), dir, Options{})
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.Equal(t, 2, queries)
			require.Nil(t, st.DecisionPending)
			require.Equal(t, "subscription-cleanup-reconciled", st.Decisions[0].Reason)
			require.False(t, st.Observations[0].Invocation.Adopted)
			require.Nil(t, st.Decisions[0].Usage.Input)
		})
	}
}

func TestInvestigationCannotFinishWhileReservedNativeCleanupIsUnknown(t *testing.T) {
	dir, _ := subscriptionPreparation(t)
	calls, confirmed := 0, false
	fakeSubscription(t, func(prompt string) decision.SubscriptionResult {
		calls++
		r := subscriptionAnswer(t, prompt)
		r.Native.CleanupConfirmed = false
		return r
	}, func(string) decision.SubscriptionResult {
		return decision.SubscriptionResult{Native: decision.NativeExecution{RequestID: "jev-one", Dispatch: "accepted", Outcome: "completed", CleanupConfirmed: confirmed, ReceiptHash: strings.Repeat("a", 64)}}
	})
	st, err := Run(context.Background(), dir, Options{})
	require.ErrorContains(t, err, "cleanup requires reconciliation")
	report := unknownReport(st)
	st, err = Finish(dir, report, Options{})
	require.ErrorContains(t, err, "cleanup requires reconciliation")
	require.Nil(t, st.Report)
	require.NotNil(t, st.DecisionPending, "report attempt cannot erase the exact recovery input")
	require.Empty(t, st.Decisions)
	_, err = Run(context.Background(), dir, Options{})
	require.ErrorContains(t, err, "cleanup requires reconciliation")
	confirmed = true
	st, err = Finish(dir, report, Options{})
	require.NoError(t, err)
	require.NotNil(t, st.Report)
	require.Nil(t, st.DecisionPending)
	require.Equal(t, 1, calls)
	require.Len(t, st.Decisions, 1)
	require.Equal(t, "unknown", st.Decisions[0].Status)
}

func TestNativePendingSurvivesEveryTerminationBoundary(t *testing.T) {
	for _, stop := range []string{"deadline", "budget", "cancel"} {
		t.Run(stop, func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			p.Limits.MaxActions = 1
			changeRuntimeConfig(t, dir, func(values map[string]any) {
				values["jev"] = map[string]any{"enabled": true, "backend": "subscription", "profile": "hydrogen-local"}
			})
			st := startFixture(t, dir, cwd, p)
			if stop == "budget" {
				a := p.Actions[0]
				invocation := state.PreparationInvocation{ID: state.PreparationHash("completed"), ActionID: a.ID, BindingHash: state.PreparationHash(a), CandidateHash: state.PreparationHash("old"), StartedAt: time.Now().Format(time.RFC3339Nano), Reason: "baseline"}
				observation := state.InvestigationObservation{Invocation: invocation, Status: "observed", Output: "observation", ContentHash: digest([]byte("observation")), FinishedAt: time.Now().Format(time.RFC3339Nano)}
				st.ActionsUsed = 1
				consume(st, observation)
				require.NoError(t, persist(dir, &st, Options{}))
			}
			st = reserveDecision(t, dir, st)
			var err error
			switch stop {
			case "deadline":
				st, err = Run(context.Background(), dir, Options{Now: func() time.Time { return time.Now().Add(time.Hour) }})
			case "budget":
				st, err = Run(context.Background(), dir, Options{})
			case "cancel":
				st, err = Cancel(dir, Options{})
			}
			require.ErrorContains(t, err, "cleanup requires reconciliation")
			require.Equal(t, "blocked", st.Status)
			require.NotNil(t, st.DecisionPending)
			require.Empty(t, st.Decisions)
		})
	}
}

func TestArchivedUnknownNativeReceiptCannotBeBypassedByRunOrReport(t *testing.T) {
	dir, st := subscriptionPreparation(t)
	st = reserveDecision(t, dir, st)
	st.Decisions = []decision.Receipt{{Version: 1, Backend: "subscription", DecisionID: st.DecisionPending.ID, Status: "unknown", Reason: "historical-unknown-cleanup"}}
	st.DecisionPending = nil
	require.NoError(t, persist(dir, &st, Options{}))
	_, err := Run(context.Background(), dir, Options{})
	require.ErrorContains(t, err, "cleanup requires reconciliation")
	_, err = Finish(dir, unknownReport(st), Options{})
	require.ErrorContains(t, err, "unresolved investigation")
}

func TestInvestigationMissingNativeJournalIsNotProofOfNonDispatch(t *testing.T) {
	dir, st := subscriptionPreparation(t)
	identity, err := candidate(context.Background(), st)
	require.NoError(t, err)
	s := snapshot(st, remainingActions(st), identity)
	st.DecisionPending = &s
	st.DecisionStartedAt = map[string]string{s.ID: time.Now().Format(time.RFC3339Nano)}
	_, err = state.SavePreparation(dir, st)
	require.NoError(t, err)
	_, err = Run(context.Background(), dir, Options{})
	require.ErrorContains(t, err, "cleanup requires reconciliation")
}
