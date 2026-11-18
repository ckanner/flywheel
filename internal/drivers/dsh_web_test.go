package drivers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dshWebFixture(r dshStart, session string) (DSHWebBinding, DSHWebReceipt) {
	b := DSHWebBinding{Protocol: 1, Transport: "chatgpt-web", Provider: r.Provider, Model: DSHWebModel, ReasoningEffort: "pro",
		ConnectionID: "owned-connection", AccountFingerprint: "opaque-account-fingerprint", PolicyVersion: "web-policy/1",
		ContextPolicy: "project-only", Observation: "opaque", Usage: "unknown", HostContractVersion: 1, RuntimeVersion: "web-runtime/1"}
	f := DSHWebFacts{Version: 1, HostInvocationID: r.RequestID, ExecutionDomainID: r.Web.ExecutionDomainID, SessionID: session,
		Dispatch: "dispatched", Outcome: "completed", RemoteGeneration: "idle", LocalCleanupConfirmed: true, UsageStatus: "unknown"}
	if r.Web.Probe {
		f.Dispatch, f.Outcome = "not-dispatched", "failed"
	}
	return b, DSHWebReceipt{Version: 1, Binding: b, Facts: f}
}

func TestDSHWebAdmissionAndResumeIdentity(t *testing.T) {
	r := dshStart{Model: DSHWebModel, Provider: "chatgpt", ReasoningEffort: "pro", Cwd: "/repo", RequestID: "invoke-1"}
	w, err := prepareDSHWeb(nil, r)
	require.NoError(t, err)
	require.NotEmpty(t, w.ExecutionDomainID)
	r.Web = w
	copy, err := prepareDSHWeb(w, r)
	require.NoError(t, err)
	assert.Equal(t, w, copy)
	assert.NotSame(t, w, copy)
	r.WorkspaceCwd = "/project"
	project, err := prepareDSHWeb(nil, r)
	require.NoError(t, err)
	assert.NotEqual(t, w.ExecutionDomainID, project.ExecutionDomainID)
	b, _ := dshWebFixture(r, "session")
	expected := b
	r.Web.ExpectedBinding = &expected
	_, err = prepareDSHWeb(r.Web, r)
	require.NoError(t, err)
	raw, _ := json.Marshal(b)
	actual, err := validateDSHWebReady(raw, r)
	require.NoError(t, err)
	assert.Equal(t, b, *actual)
	b.AccountFingerprint = "changed-account"
	raw, _ = json.Marshal(b)
	_, err = validateDSHWebReady(raw, r)
	assert.ErrorContains(t, err, "changed")
	for _, bad := range []DSHWebRequest{{Version: 2, ExecutionDomainID: "domain", RequireIsolation: true}, {Version: 1, ExecutionDomainID: " ", RequireIsolation: true}, {Version: 1, ExecutionDomainID: "domain"}, {Version: 1, ExecutionDomainID: "domain", RequireIsolation: true, ExpectedBinding: &DSHWebBinding{}}} {
		_, err = prepareDSHWeb(&bad, r)
		assert.Error(t, err)
	}
	r.ReasoningEffort = "high"
	_, err = prepareDSHWeb(nil, r)
	assert.ErrorContains(t, err, "explicit thinking pro")
	r.Model = "ordinary-model"
	_, err = prepareDSHWeb(w, r)
	assert.Error(t, err)
	_, err = prepareDSHWeb(nil, r)
	assert.NoError(t, err)
	r.Web = nil
	_, err = validateDSHWebReady(raw, r)
	assert.Error(t, err)
	_, err = validateDSHWebReady(nil, r)
	assert.NoError(t, err)
	for _, text := range []string{"", " ", "a\n", "a\r", "a\x00", strings.Repeat("a", 513)} {
		assert.False(t, dshWebString(text))
	}
}

func TestDSHWebStrictReceiptAndUnknownFacts(t *testing.T) {
	r := dshStart{Protocol: 2, Persistent: true, Model: DSHWebModel, Provider: "chatgpt", ReasoningEffort: "pro", Cwd: "/repo", RequestID: "invoke-1", PermissionMode: "read-only",
		Web: &DSHWebRequest{Version: 1, ExecutionDomainID: "task-domain", RequireIsolation: true}}
	b, receipt := dshWebFixture(r, "sdk-session-one")
	check := func(value DSHWebReceipt, status string, usage json.RawMessage, cost *float64, settled bool, lifecycle *SessionOutcome) error {
		raw, _ := json.Marshal(value)
		return validateDSHWebResult(raw, r, &b, "sdk-session-one", status, usage, cost, settled, lifecycle)
	}
	require.NoError(t, check(receipt, "ok", nil, nil, true, nil))
	ready, last := dshFixtureFrames(r)
	parsed := parseDSHStream(dshJSONL(ready, last), r)
	require.Empty(t, parsed.Error)
	assert.NotEmpty(t, parsed.WebReceipt)
	assert.Nil(t, parsed.Usage)
	for _, mutate := range []func(*DSHWebReceipt){
		func(x *DSHWebReceipt) { x.Version = 2 }, func(x *DSHWebReceipt) { x.Binding.ConnectionID = "foreign" },
		func(x *DSHWebReceipt) { x.Facts.HostInvocationID = "old-call" }, func(x *DSHWebReceipt) { x.Facts.ExecutionDomainID = "foreign" },
		func(x *DSHWebReceipt) { x.Facts.SessionID = "foreign" }, func(x *DSHWebReceipt) { x.Facts.UsageStatus = "complete" },
		func(x *DSHWebReceipt) { x.Facts.Dispatch = "unknown" }, func(x *DSHWebReceipt) { x.Facts.Dispatch = "not-dispatched" },
		func(x *DSHWebReceipt) { x.Facts.RemoteGeneration = "running" }, func(x *DSHWebReceipt) { x.Facts.LocalCleanupConfirmed = false },
		func(x *DSHWebReceipt) { x.Facts.Outcome = "failed" },
	} {
		bad := receipt
		mutate(&bad)
		assert.Error(t, check(bad, "ok", nil, nil, true, nil))
	}
	cost := 0.0
	assert.Error(t, check(receipt, "ok", json.RawMessage(`{"total_tokens":0}`), nil, true, nil))
	assert.Error(t, check(receipt, "ok", nil, &cost, true, nil))
	assert.Error(t, check(receipt, "ok", nil, nil, true, &SessionOutcome{Outcome: "failed"}))
	receipt.Facts.Outcome, receipt.Facts.RemoteGeneration = "unknown", "unknown"
	require.NoError(t, check(receipt, "error", json.RawMessage("null"), nil, true, &SessionOutcome{Outcome: "unknown"}))
	assert.Error(t, check(receipt, "error", nil, nil, true, &SessionOutcome{Outcome: "completed"}))
	r.Web.Probe = true
	_, receipt = dshWebFixture(r, "sdk-session-one")
	require.NoError(t, check(receipt, "ok", nil, nil, true, nil))
	r.Web.Probe = false
	assert.Error(t, check(receipt, "ok", nil, nil, true, nil))
	assert.NoError(t, validateDSHWebResult(nil, r, nil, "", "error", nil, nil, true, nil))
	assert.Error(t, validateDSHWebResult(nil, r, &b, "", "error", nil, nil, true, nil))
	assert.Error(t, validateDSHWebResult(json.RawMessage(`{}`), r, nil, "", "error", nil, nil, true, nil))
	r.Web = nil
	assert.NoError(t, validateDSHWebResult(nil, r, nil, "", "ok", nil, nil, true, nil))
	assert.Error(t, validateDSHWebResult(json.RawMessage(`{}`), r, nil, "", "ok", nil, nil, true, nil))
	r.Web = &DSHWebRequest{Version: 1, ExecutionDomainID: "task-domain", RequireIsolation: true}
	delete(ready, "web")
	assert.NotEmpty(t, parseDSHStream(dshJSONL(ready, last), r).Error)
	ready, last = dshFixtureFrames(r)
	delete(last, "web")
	assert.NotEmpty(t, parseDSHStream(dshJSONL(ready, last), r).Error)
}

func TestDSHWebDecodingAndBudgets(t *testing.T) {
	for _, raw := range []string{"", "null", strings.Repeat(" ", 16385), "{", `{"extra":true}`, `{} {}`, `{} garbage`} {
		assert.Error(t, decodeDSHWeb(json.RawMessage(raw), &DSHWebBinding{}), raw)
	}
	r := dshStart{Model: DSHWebModel, Provider: "chatgpt", ReasoningEffort: "pro", Web: &DSHWebRequest{Version: 1, ExecutionDomainID: "domain", RequireIsolation: true}}
	_, err := validateDSHWebReady(json.RawMessage(`{}`), r)
	assert.Error(t, err)
	_, err = validateDSHWebReady(nil, r)
	assert.Error(t, err)
	a := DSHArgs{Cwd: t.TempDir(), Provider: "chatgpt", Model: DSHWebModel, Thinking: "pro", Doctor: true, Watchdog: watchdog.Opts{WallTimeoutMs: 5000, IdleTimeoutMs: 10}}
	request, _, argv, _, err := dshCall(a)
	require.NoError(t, err)
	assert.True(t, request.Web.Probe)
	assert.Equal(t, []string{"--doctor"}, argv)
	assert.Equal(t, 6*time.Second, dshWebIdleTimeout(a))
	assert.Equal(t, int64(20000), dshCleanupGrace(a))
	a.Model = "ordinary-model"
	assert.Equal(t, 10*time.Millisecond, dshWebIdleTimeout(a))
	a.Model, a.Thinking = DSHWebModel, "max"
	_, _, _, _, err = dshCall(a)
	assert.Error(t, err)
	a.Thinking = "pro"
	b, _ := dshWebFixture(r, "session")
	a.Web = &DSHWebRequest{Version: 1, ExecutionDomainID: "domain", RequireIsolation: true, ExpectedBinding: &b}
	a.Doctor = false
	_, err = DSHPrewarmBinding(a)
	assert.NoError(t, err)
	assert.NotNil(t, a.Web.ExpectedBinding, "prewarm cannot mutate caller's expected binding")
}

func TestDSHWebPrewarmRejectsMissingAttestation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &DSHPrewarm{ctx: ctx, cancel: cancel, ready: make(chan struct{}), prepare: dshPrepare{WarmID: "warm", Binding: "binding", Provider: "chatgpt", Model: DSHWebModel, ReasoningEffort: "pro", Web: &DSHWebRequest{Version: 1, ExecutionDomainID: "domain", RequireIsolation: true}}}
	h.line(`{"type":"warm-ready","protocol":1,"warmId":"warm","binding":"binding","level":"sdk-initialized","dispatched":false}`)
	require.Error(t, h.streamError)
	require.Error(t, ctx.Err())
	require.False(t, h.readySeen)
}

func TestRunWebDoesNotTreatOpaqueSilenceAsIdle(t *testing.T) {
	a := dshRunArgs(t, "web-opaque")
	a.Model, a.Thinking = DSHWebModel, "pro"
	a.Watchdog.IdleTimeoutMs = 5
	result := RunDSH(a)
	require.True(t, result.Settled, "%+v", result)
	require.False(t, result.TimedOut)
	require.Nil(t, result.Usage)
	require.NotEmpty(t, result.WebReceipt)
	a = dshRunArgs(t, "hang")
	a.Model, a.Thinking = DSHWebModel, "pro"
	a.Watchdog.IdleTimeoutMs, a.Watchdog.WallTimeoutMs = 5, 150
	result = RunDSH(a)
	require.True(t, result.TimedOut, "the wall bound remains effective for Web")
}

func TestWebPrewarmKeepsOpaqueTimeoutsAndUnknownUsage(t *testing.T) {
	for _, scenario := range []string{"web-opaque", "failed-initialize", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			a := warmArgs(t, scenario)
			a.Model, a.Thinking, a.RequestID, a.PersistentSession = DSHWebModel, "pro", "web-formal", true
			a.Watchdog.IdleTimeoutMs = 5
			a.Web = &DSHWebRequest{Version: 1, ExecutionDomainID: "persistent-domain", RequireIsolation: true}
			binding, _ := dshWebFixture(dshStart{Provider: a.Provider, Model: a.Model, Web: a.Web}, "")
			a.Web.ExpectedBinding = &binding
			h := PrepareDSH(a)
			t.Cleanup(func() { _ = h.Close() })
			require.Nil(t, h.prepare.Web.ExpectedBinding, "static prepare does not consume formal history")
			if scenario == "web-opaque" {
				awaitWarm(t, h)
			} else {
				<-h.done
			}
			result := h.Run(a)
			require.Nil(t, result.Usage)
			require.Nil(t, result.CostUSD)
			require.Equal(t, "unknown", result.Session.UsageStatus)
			if scenario == "failed-initialize" {
				require.False(t, result.Settled)
				require.Equal(t, "not-dispatched", result.Session.Dispatch)
			} else {
				require.True(t, result.Settled, "%+v", result)
				require.False(t, result.IdleTimedOut)
			}
		})
	}
}

func TestDSHWebNodeWorkerOfflineBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("requires built Node worker and published SDK dependencies")
	}
	worker, err := filepath.Abs("../../plugins/dsh/dist/worker.js")
	require.NoError(t, err)
	sdk, err := filepath.Abs("../../plugins/dsh/test/fixtures/persisted-sdk.js")
	require.NoError(t, err)
	t.Setenv("FLYWHEEL_DSH_BIN", sdk)
	for _, warm := range []bool{false, true} {
		a := warmArgs(t, "unused")
		a.Command, a.Provider, a.Model, a.Thinking = worker, "offline-fixture", DSHWebModel, "pro"
		a.RequestID, a.PersistentSession = "actual-go-web-invocation", true
		a.Web = &DSHWebRequest{Version: 1, ExecutionDomainID: "stable-go-domain", RequireIsolation: true}
		a.Watchdog.IdleTimeoutMs, a.Watchdog.WallTimeoutMs = 5, 15000
		a.APIKeys = map[string]string{"TEST_DSH_WEB_SCENARIO": "ok", "TEST_DSH_HISTORY": filepath.Join(a.Cwd, "history")}
		if warm {
			h := PrepareDSH(a)
			t.Cleanup(func() { _ = h.Close() })
			awaitWarm(t, h)
			a.Prewarm = h
		}
		result := RunDSH(a)
		require.True(t, result.Settled, "%+v", result)
		require.False(t, result.TimedOut)
		require.Nil(t, result.Usage)
		require.Nil(t, result.CostUSD)
		require.Equal(t, "unknown", result.Session.UsageStatus)
		var receipt DSHWebReceipt
		require.NoError(t, json.Unmarshal(result.WebReceipt, &receipt))
		require.Equal(t, a.RequestID, receipt.Facts.HostInvocationID)
		require.Equal(t, "stable-go-domain", receipt.Facts.ExecutionDomainID)
	}
}
