package machine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/investigation"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func enableRuntimeSubscription(t *testing.T, c *Context) {
	t.Helper()
	enableRuntimeJEV(t, c, "https://unused.invalid")
	c.Config.JEV.Backend, c.Config.JEV.Profile = "subscription", "hydrogen-local"
	c.Config.JEV.Endpoint, c.Config.JEV.Model, c.Config.JEV.KeyEnv = "", "", ""
	if c.Config.Profiles == nil {
		c.Config.Profiles = map[string]config.ModelProfile{}
	}
	c.Config.Profiles["hydrogen-local"] = config.ModelProfile{Harness: "antigravity", Channel: "jev-google", Model: "gemini-4-hydrogen-low", Thinking: "low"}
	c.Config.Channels["jev-google"] = config.Channel{Harness: "antigravity"}
	persistRuntimeConfig(t, c)
	gitCmd(t, c.Cwd, "add", "--", filepath.Join(c.TaskDir, "flywheel.config.json"))
	gitCmd(t, c.Cwd, "commit", "-q", "-m", "test: enable subscribed decision harness")
}

func runtimeSubscriptionReply(t *testing.T, prompt string) decision.SubscriptionResult {
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
	probabilities["repair"] = 1
	raw, err := json.Marshal(map[string]any{"action-id": "repair", "applicability": 1, "choice-confidence": 1, "probabilities": probabilities})
	require.NoError(t, err)
	return decision.SubscriptionResult{Model: "gemini-4-hydrogen-low", Text: string(raw), Native: decision.NativeExecution{RequestID: "jev-runtime-one", Dispatch: "accepted", Outcome: "completed", CleanupConfirmed: true, ReceiptHash: strings.Repeat("a", 64)}}
}

func fakeRuntimeSubscription(t *testing.T, call func(string) decision.SubscriptionResult, recover func(string) decision.SubscriptionResult) {
	t.Helper()
	saved := subscriptionDecisionOptions
	t.Cleanup(func() { subscriptionDecisionOptions = saved })
	subscriptionDecisionOptions = func(_ context.Context, b *decision.SubscriptionBinding, cwd, dir string, timeout time.Duration) decision.Options {
		return decision.Options{Backend: "subscription", Subscription: b, Model: b.Model, ExpectedModel: b.Model, Timeout: timeout, SubscriptionCall: func(_ context.Context, prompt, _ string) decision.SubscriptionResult { return call(prompt) }, SubscriptionRecover: recover}
	}
}

func TestRuntimeSubscriptionSelectsOnlyFrozenActionsAndCountsUnknownUsage(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	enableRuntimeSubscription(t, c)
	p := c.Packets["M01-T01"]
	facts := runtimeFailureFacts(t, c, p)
	calls := 0
	fakeRuntimeSubscription(t, func(prompt string) decision.SubscriptionResult { calls++; return runtimeSubscriptionReply(t, prompt) }, nil)
	c.EnvLookup = func(string) (string, bool) { t.Fatal("subscription decision read API key"); return "", false }
	before := c.State.Budget.UnknownUsageCalls
	c.failureDecision(p, errL1Failed, facts, false)
	require.NoError(t, c.err())
	require.Equal(t, 1, calls)
	ledger := c.State.RuntimeDecisions
	require.NotNil(t, ledger.Subscription)
	require.Equal(t, "hydrogen-local", ledger.Subscription.Profile)
	r := ledger.Records[0]
	require.True(t, r.Selection.Adopted)
	require.Equal(t, "repair", r.Action)
	require.Nil(t, r.Receipt.Usage.Input)
	require.Equal(t, before+1, c.State.Budget.UnknownUsageCalls)
	_, err := state.Load(c.TaskDir)
	require.NoError(t, err)
}

func TestRuntimeSubscriptionUnknownCleanupStopsAndOnlyReadsOriginalReceipt(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	enableRuntimeSubscription(t, c)
	p := c.Packets["M01-T01"]
	facts := runtimeFailureFacts(t, c, p)
	calls, queries := 0, 0
	confirmed := false
	fakeRuntimeSubscription(t, func(prompt string) decision.SubscriptionResult {
		calls++
		r := runtimeSubscriptionReply(t, prompt)
		r.Native.CleanupConfirmed = false
		return r
	}, func(string) decision.SubscriptionResult {
		queries++
		return decision.SubscriptionResult{Native: decision.NativeExecution{RequestID: "jev-runtime-one", Dispatch: "accepted", Outcome: "completed", CleanupConfirmed: confirmed, ReceiptHash: strings.Repeat("a", 64)}}
	})
	c.failureDecision(p, errL1Failed, facts, false)
	require.ErrorContains(t, c.err(), "cleanup requires reconciliation")
	loaded := loadFixtureState(t, fx)
	require.NotEmpty(t, loaded.Packets[p.PacketID].PendingStrategy)
	c.State, c.revision, c.fatal = loaded, loaded.Revision, nil
	c.resumeRuntimeDecision(p)
	require.ErrorContains(t, c.err(), "cleanup requires reconciliation")
	before := c.State.Budget.UnknownUsageCalls
	confirmed = true
	c.fatal = nil
	c.resumeRuntimeDecision(p)
	require.NoError(t, c.err())
	require.Equal(t, 1, calls)
	require.Equal(t, 2, queries)
	require.Empty(t, c.State.Packets[p.PacketID].PendingStrategy)
	require.False(t, c.State.RuntimeDecisions.Records[0].Selection.Adopted)
	require.Equal(t, before, c.State.Budget.UnknownUsageCalls, "recovery cannot recount or invent per-call usage")
}

func TestRuntimeSubscriptionDetectsProfileEditsAndKeepsLegacyBackendHashes(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	enableRuntimeSubscription(t, c)
	initial, status := c.runtimeJEVConfig(nil)
	require.Equal(t, "enabled", status)
	require.NotNil(t, initial)
	c.initializeRuntimeDecisions(initial)
	p := c.Config.Profiles["hydrogen-local"]
	p.Model, p.Thinking = "gemini-4-hydrogen-high", "high"
	c.Config.Profiles["hydrogen-local"] = p
	persistRuntimeConfig(t, c)
	_, status = c.runtimeJEVConfig(initial)
	require.Equal(t, "subscription-binding-changed", status)
	_, legacy := sessionFixture(t)
	enableExecutionPolicy(legacy)
	enableRuntimeJEV(t, legacy, "https://unused.invalid")
	frozen := legacy.Config.JEV
	frozen.Backend = "" // pre-backend persisted ledger
	_, status = legacy.runtimeJEVConfig(&frozen)
	require.Equal(t, "enabled", status)
}

func TestRuntimeImportsDecisionCallsFromEveryInvestigationRound(t *testing.T) {
	fx, p := managedPreparationFixture(t, true)
	p.Limits.MaxActions = 6
	p.Limits.MaxSeconds = 120
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer server.Close()
	j := config.Default().JEV
	j.Enabled, j.Endpoint, j.MaxCalls = true, server.URL+"/v1/systemone", 8
	rewriteConfig(t, fx, func(v map[string]any) { delete(v, "subscription"); v["jev"] = j })
	opts := investigation.Options{LookupEnv: func(string) (string, bool) { return "fixture", true }}
	startManagedPreparation(t, fx, p, opts)
	first := finishManagedPreparation(t, fx, opts)
	p.Limits = state.PreparationRemaining(first)
	p.KnownEvidence = []string{"New evidence requires a follow-up round"}
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	_, err = investigation.Continue(fx.taskDir, raw, investigation.Options{ExpectedRevision: first.Revision})
	require.NoError(t, err)
	prepared := finishManagedPreparation(t, fx, opts)
	want := 0
	for _, round := range state.PreparationRounds(prepared) {
		for _, r := range round.Decisions {
			if r.Status != "skipped" {
				want++
			}
		}
	}
	require.Greater(t, want, len(prepared.Decisions))
	writeFile(t, state.StateFile(fx.taskDir), fixtureState(fx.slug))
	c := unitTestContext(t, fx)
	ledger := c.initializeRuntimeDecisions(&j)
	require.NotNil(t, ledger)
	require.Equal(t, want, ledger.ImportedCalls)
	require.Equal(t, want, ledger.Used)
}
