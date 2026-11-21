package machine

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestAntigravityHistoryIdentityIncludesRuntimeAndSubscriptionControls(t *testing.T) {
	a := drivers.CallArgs{Binding: config.LoopBinding{Harness: "antigravity"}}
	for _, name := range []string{drivers.AntigravityBinaryEnv, drivers.AntigravityWorkerEnv} {
		t.Setenv(name, "/original/command")
		before := sessionRuntimeIdentity(a)
		t.Setenv(name, "/changed/command")
		require.NotEqual(t, before, sessionRuntimeIdentity(a), "a changed runtime cannot resume old history")
	}
	legacy := sessionRuntimeIdentity(a)
	a.Subscription = &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}
	subscription := sessionRuntimeIdentity(a)
	require.NotEqual(t, legacy, subscription)
	a.EvidenceOnly = true
	require.NotEqual(t, subscription, sessionRuntimeIdentity(a), "tool policy is part of the frozen binding")
	before := sessionRuntimeIdentity(a)
	t.Setenv("GEMINI_API_KEY", "ambient-value-never-forwarded")
	require.Equal(t, before, sessionRuntimeIdentity(a), "ambient credential values are not session identity or permission")
	a.Subscription = nil
	require.NotEqual(t, legacy, sessionRuntimeIdentity(a), "an evidence-only flag is retained even without a subscription policy")
}

func TestAntigravitySessionRecoveryUsesFrozenPolicyAndNeverRedispatches(t *testing.T) {
	fx, c := sessionFixture(t)
	policy := &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}
	c.Config.Subscription = policy
	a := sessionArgs(c)
	a.Model = "gemini-4-neon-high"
	a.Binding = config.LoopBinding{Harness: "antigravity", Model: a.Model, Channel: "google-ultra", Thinking: "high"}
	a.PermissionMode, a.EvidenceOnly = "read-only", true
	previous := persistSessionReceipt
	t.Cleanup(func() { persistSessionReceipt = previous })
	persistSessionReceipt = func(task, key string, receipt state.SessionReceipt) (string, error) {
		if !strings.HasSuffix(key, "/lifecycle") {
			return "", errors.New("interrupted before terminal receipt adoption")
		}
		return previous(task, key, receipt)
	}
	calls := 0
	var completed drivers.Result
	_, err := c.invokeScoped("review", "M01-T01", a, func(call drivers.CallArgs) drivers.Result {
		calls++
		require.Equal(t, policy, call.Subscription)
		require.True(t, call.EvidenceOnly)
		require.True(t, call.PersistentSession)
		require.Empty(t, call.SessionID)
		completed = sessionResult(call, drivers.Result{Harness: "antigravity", Model: call.Model, Settled: true, ExitCode: new(0), Text: "completed evidence review"})
		completed.Session.Protocol = "antigravity/" + drivers.AntigravityPackageVersion + "/worker-v3"
		completed.Session.TurnID, completed.Session.UsageStatus = "", "unknown"
		event := drivers.MachineLifecycle{LifecycleVersion: 1, Type: "lifecycle", RequestID: call.RequestID, RunID: completed.Session.RunID,
			Persistent: true, Configuration: json.RawMessage("null"), Dispatch: "not-dispatched", Outcome: "unknown"}
		for i, stage := range []string{"dispatch-intent", "dispatch-possible", "terminal", "cleanup-confirmed"} {
			event.Sequence, event.Event = i+1, stage
			if stage == "dispatch-possible" {
				event.ThreadID, event.Dispatch = &completed.Session.SessionID, "possible"
			}
			if stage == "terminal" {
				event.Dispatch, event.Outcome = "accepted", "completed"
			}
			event.CleanupConfirmed = stage == "cleanup-confirmed"
			require.NoError(t, call.OnLifecycle(event))
		}
		return completed
	})
	require.ErrorContains(t, err, "terminal receipt could not be saved")
	persistSessionReceipt = previous
	c = restartSessionGuardContext(t, fx, c)
	r := c.sessionRecord("review", "M01-T01")
	require.Equal(t, "antigravity", r.Last.Recovery.Harness)
	require.Equal(t, drivers.AntigravityPackageVersion, r.Last.Recovery.AdapterVersion)
	require.Equal(t, policy, r.Configuration.Subscription)
	require.True(t, r.Configuration.EvidenceOnly)
	c.Config.Subscription = nil // Current policy changes cannot retarget the old invocation.
	oldRecovery := recoverSessionMachine
	t.Cleanup(func() { recoverSessionMachine = oldRecovery })
	queries := 0
	recoverSessionMachine = func(query drivers.MachineRecoveryArgs) (drivers.Result, error) {
		queries++
		require.Equal(t, "antigravity", query.Harness)
		require.Equal(t, policy, query.Subscription)
		require.True(t, query.EvidenceOnly)
		require.Equal(t, "read-only", query.PermissionMode)
		require.Equal(t, a.Model, query.Model)
		require.Equal(t, "high", query.ReasoningEffort)
		require.Equal(t, completed.Session.RequestID, query.RequestID)
		require.Equal(t, completed.Session.RunID, query.RunID)
		require.True(t, query.PersistentSession)
		require.Empty(t, query.SessionID, "a fresh original invocation must not become an implicit resume")
		return completed, nil
	}
	require.NoError(t, c.reconcileSessionReceipts())
	require.Equal(t, 1, queries)
	require.EqualValues(t, 1, c.State.Budget.UnknownUsageCalls, "conversation counters are not per-call token deltas")
	require.EqualValues(t, 1, c.State.Budget.UnknownCostCalls)
	c.Config.Subscription = policy
	out, err := c.invokeScoped("review", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
		t.Fatal("durable terminal recovery must not send another prompt")
		return drivers.Result{}
	})
	require.NoError(t, err)
	require.True(t, out.Recovered)
	require.Equal(t, completed.Text, out.Text)
	require.Equal(t, 1, calls)
	applySessionResult(t, c, "review", "M01-T01", out)
	c = restartSessionGuardContext(t, fx, c)
	require.NoError(t, c.reconcileSessionReceipts())
	require.Equal(t, 1, queries)
	require.EqualValues(t, 1, c.State.Budget.UnknownUsageCalls)
}
