package machine

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func webSessionResult(a drivers.CallArgs) drivers.Result {
	r := sessionResult(a, drivers.MockResult(nil))
	r.Usage, r.CostUSD, r.Session.UsageStatus = nil, nil, "unknown"
	binding := drivers.DSHWebBinding{Protocol: 1, Transport: "chatgpt-web", Provider: "chatgpt", Model: drivers.DSHWebModel, ReasoningEffort: "pro",
		ConnectionID: "connection", AccountFingerprint: "account", PolicyVersion: "policy/1", ContextPolicy: "project-only",
		Observation: "opaque", Usage: "unknown", HostContractVersion: 1, RuntimeVersion: "web-runtime/1"}
	r.WebReceipt, _ = json.Marshal(drivers.DSHWebReceipt{Version: 1, Binding: binding,
		Facts: drivers.DSHWebFacts{Version: 1, HostInvocationID: a.RequestID, ExecutionDomainID: a.Web.ExecutionDomainID, SessionID: r.SessionID,
			Dispatch: "dispatched", Outcome: "completed", RemoteGeneration: "idle", LocalCleanupConfirmed: true, UsageStatus: "unknown"}})
	return r
}

func TestWebSessionDomainAndAccountBindingSurviveWorkerRestart(t *testing.T) {
	_, c := sessionFixture(t)
	a := sessionArgs(c)
	a.Model, a.Binding.Model, a.Binding.Provider, a.Binding.Thinking = drivers.DSHWebModel, drivers.DSHWebModel, "chatgpt", "pro"
	var first drivers.CallArgs
	run := func(check func(drivers.CallArgs)) drivers.Result {
		r, err := c.invokeScoped("execute", "M01-T01", a, func(actual drivers.CallArgs) drivers.Result {
			check(actual)
			return webSessionResult(actual)
		})
		require.NoError(t, err)
		applySessionResult(t, c, "execute", "M01-T01", r)
		return r
	}
	run(func(actual drivers.CallArgs) {
		first = actual
		require.NotNil(t, actual.Web)
		require.Nil(t, actual.Web.ExpectedBinding)
	})
	require.Equal(t, 5, c.State.Sessions.Version)
	record := c.State.Sessions.Records["execute/M01-T01"]
	require.NotEmpty(t, record.WebBinding)
	require.Equal(t, "unknown", record.Last.UsageStatus)
	run(func(actual drivers.CallArgs) {
		require.NotEqual(t, first.RequestID, actual.RequestID)
		require.Equal(t, first.Web.ExecutionDomainID, actual.Web.ExecutionDomainID)
		require.NotEmpty(t, actual.SessionID)
		require.Equal(t, "account", actual.Web.ExpectedBinding.AccountFingerprint)
	})
	c.State.Sessions.Records["execute/M01-T01"].WebBinding = nil
	run(func(actual drivers.CallArgs) {
		require.Empty(t, actual.SessionID)
		require.Nil(t, actual.Web.ExpectedBinding)
	})
	require.Equal(t, "rebuild", c.State.Sessions.Records["execute/M01-T01"].Action)
	c.State.Sessions.Records["execute/M01-T01"].WebBinding = json.RawMessage(`[]`)
	_, err := c.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
		t.Fatal("invalid saved binding cannot dispatch")
		return drivers.Result{}
	})
	require.ErrorContains(t, err, "schema violation")
}

func TestWebUnknownResultFencesFreshAndChangedProvider(t *testing.T) {
	for _, change := range []string{"fresh", "provider"} {
		t.Run(change, func(t *testing.T) {
			_, c := sessionFixture(t)
			a := sessionArgs(c)
			a.Model, a.Binding.Model, a.Binding.Provider, a.Binding.Thinking = drivers.DSHWebModel, drivers.DSHWebModel, "chatgpt", "pro"
			_, err := c.invokeScoped("execute", "M01-T01", a, func(actual drivers.CallArgs) drivers.Result {
				r := webSessionResult(actual)
				r.Session.Outcome, r.Session.ResumeCandidate = "unknown", false
				return r
			})
			require.Error(t, err)
			if change == "fresh" {
				c.Config.Sessions.Mode = "fresh"
			} else {
				a.Binding.Harness, a.Model = "codex-pro", "another-model"
			}
			_, err = c.invokeScoped("review", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
				t.Fatal("pending Web work cannot be bypassed")
				return drivers.Result{}
			})
			require.Error(t, err)
			require.Equal(t, "unknown", c.State.Sessions.Records["execute/M01-T01"].Last.Outcome)
		})
	}
}

func TestWebPrewarmUsesTaskDomainAndNewLedgerFence(t *testing.T) {
	_, c := prewarmFixture(t)
	b := c.Config.Loops["review"]
	b.Model, b.Provider, b.Thinking = drivers.DSHWebModel, "chatgpt", "pro"
	c.Config.Loops["review"] = b
	s := &preparedReviewStub{matches: true}
	stubReviewPrewarm(t, func(a drivers.DSHArgs) drivers.PreparedDSH {
		require.NotNil(t, a.Web)
		require.Equal(t, "flywheel-"+sessionHash([]any{"web-domain-v1", c.TaskDir}), a.Web.ExecutionDomainID)
		require.Nil(t, a.Web.ExpectedBinding)
		disk, err := state.Load(c.TaskDir)
		require.NoError(t, err)
		require.Equal(t, 5, disk.Sessions.Version)
		s.id = a.PrewarmID
		return s
	})
	require.NoError(t, c.beginReviewPrewarm(c.PacketList[0]))
	require.NotNil(t, c.prewarm)
	require.NoError(t, c.closeReviewPrewarm("test closed before dispatch"))
}
