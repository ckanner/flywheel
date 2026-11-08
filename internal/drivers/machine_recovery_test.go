package drivers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func recoveryFrame(request machineRecoveryRequest, harness string, receipt any) map[string]any {
	version := CodexProPackageVersion
	if harness == "claude-code-pro" {
		version = ClaudeCodeProPackageVersion
	}
	return map[string]any{"protocol": 3, "type": "recovery", "requestId": request.RequestID, "error": nil,
		"adapter":  machineAdapter{Package: "dsh-subagent-" + harness, Version: version, Protocol: 1},
		"recovery": map[string]any{"protocol": 1, "type": "recovery", "requestId": request.RequestID, "runId": request.RunID, "status": "terminal", "receipt": receipt, "lifecycle": nil}}
}

func TestReadOnlyRecoveryRequiresCorrelatedDurableTerminalEvidence(t *testing.T) {
	request := machineRecoveryRequest{Type: "recover", Protocol: 3, RequestID: "call-one", RunID: "run-one"}
	frame := recoveryFrame(request, "codex-pro", map[string]any{"raw": "retained"})
	raw, err := parseMachineRecovery(dshJSONL(frame), request, "codex-pro")
	require.NoError(t, err)
	assert.JSONEq(t, `{"raw":"retained"}`, string(raw))
	for _, edit := range []func(map[string]any){
		func(f map[string]any) { f["protocol"] = 2 }, func(f map[string]any) { f["error"] = map[string]any{"code": "READ_FAILED"} },
		func(f map[string]any) { f["recovery"] = nil }, func(f map[string]any) { proObject(f, "recovery")["runId"] = "other" },
		func(f map[string]any) { proObject(f, "recovery")["status"] = "pending" }, func(f map[string]any) { proObject(f, "recovery")["receipt"] = nil },
	} {
		f := recoveryFrame(request, "codex-pro", map[string]any{"raw": "retained"})
		edit(f)
		_, err := parseMachineRecovery(dshJSONL(f), request, "codex-pro")
		require.Error(t, err)
	}
	_, err = parseMachineRecovery("{}\n{}", request, "codex-pro")
	require.Error(t, err)
}

func TestMachineRecoveryProcessFixture(t *testing.T) {
	selector := os.Getenv("FLYWHEEL_TEST_MACHINE_RECOVERY")
	if selector == "" {
		return
	}
	parts := strings.Split(selector, "/")
	harness, scenario := parts[0], parts[1]
	var q machineRecoveryRequest
	if json.NewDecoder(os.Stdin).Decode(&q) != nil || q.Type != "recover" {
		os.Exit(2)
	}
	raw, _ := json.Marshal(q)
	_ = os.WriteFile(filepath.Join(q.Original.Cwd, "recovery-query.json"), raw, 0o600)
	if os.Getenv("OPENAI_API_KEY") != "" {
		os.Exit(3)
	}
	r := codexProStart{Type: "start", Protocol: 3, RequestID: q.RequestID, Cwd: q.Original.Cwd, Model: q.Original.Model,
		ReasoningEffort: q.Original.ReasoningEffort, PermissionMode: q.Original.PermissionMode, ThreadMode: q.Original.ThreadMode, ThreadID: q.Original.ThreadID}
	receipt := codexProFixture(r)
	if harness == "claude-code-pro" {
		receipt = claudeCodeProFixture(claudeCodeProStart(r))
	} else {
		proObject(receipt, "outcome", "execution")["runId"] = q.RunID
	}
	receipt["runId"] = q.RunID
	frame := recoveryFrame(q, harness, receipt)
	if scenario == "pending" {
		proObject(frame, "recovery")["status"] = "pending"
		proObject(frame, "recovery")["receipt"] = nil
	}
	if scenario == "wrong-run" {
		receipt["runId"] = "foreign"
	}
	if scenario == "bad-envelope" {
		frame["requestId"] = "foreign"
	}
	fmt.Print(dshJSONL(frame))
	if scenario == "bad-exit" {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestRecoverMachineUsesOnlyPublicReadOnlyQueryAndOriginalSettings(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		for _, scenario := range []string{"ok", "pending", "wrong-run", "bad-envelope", "bad-exit"} {
			t.Run(harness+"/"+scenario, func(t *testing.T) {
				command := kimiScript(t, "FLYWHEEL_TEST_MACHINE_RECOVERY="+harness+"/"+scenario+" exec "+shQuote(os.Args[0])+" -test.run '^TestMachineRecoveryProcessFixture$'\n")
				if harness == "codex-pro" {
					t.Setenv(CodexProWorkerEnv, command)
				} else {
					t.Setenv(ClaudeCodeProWorkerEnv, command)
				}
				t.Setenv("OPENAI_API_KEY", "must-not-reach-readonly-query")
				a := MachineRecoveryArgs{Harness: harness, RequestID: "call-recover", RunID: "run-recover", RecoveryDirectory: filepath.Join(t.TempDir(), "recovery"), Cwd: t.TempDir(), Model: "gpt-test", ReasoningEffort: "high", PermissionMode: "danger-full-access", PersistentSession: true, SessionID: "11111111-1111-4111-8111-111111111111"}
				if harness == "claude-code-pro" {
					a.Model = "sonnet"
				}
				canonical, err := filepath.EvalSymlinks(a.Cwd)
				require.NoError(t, err)
				a.Cwd = canonical
				r, err := RecoverMachine(a)
				if scenario != "ok" {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				assert.True(t, r.Settled)
				assert.True(t, r.Recovered)
				assert.True(t, r.Session.CleanupConfirmed)
				assert.Equal(t, a.SessionID, r.Session.SessionID)
				raw, err := os.ReadFile(filepath.Join(a.Cwd, "recovery-query.json"))
				require.NoError(t, err)
				assert.NotContains(t, string(raw), "prompt")
				assert.NotContains(t, string(raw), "API_KEY")
				assert.Contains(t, string(raw), `"type":"recover"`)
			})
		}
	}
	a := MachineRecoveryArgs{Harness: "codex-pro", RequestID: "call-one", RunID: "run-one", RecoveryDirectory: t.TempDir(), Cwd: t.TempDir(), Model: "test", ReasoningEffort: "high", PermissionMode: "read-only"}
	bad := a
	bad.RecoveryDirectory = "relative"
	_, err := RecoverMachine(bad)
	require.Error(t, err)
	t.Setenv(CodexProWorkerEnv, filepath.Join(t.TempDir(), "missing"))
	_, err = RecoverMachine(a)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Watchdog.Context = ctx
	_, err = RecoverMachine(a)
	require.Error(t, err)
	a.Watchdog.Context = nil
	command := kimiScript(t, "FLYWHEEL_TEST_MACHINE_RECOVERY=codex-pro/ok exec "+shQuote(os.Args[0])+" -test.run '^TestMachineRecoveryProcessFixture$'\n")
	t.Setenv(CodexProWorkerEnv, command)
	old := encodeMachineRecovery
	t.Cleanup(func() { encodeMachineRecovery = old })
	encodeMachineRecovery = func(*json.Encoder, any) error { return errors.New("broken pipe") }
	_, err = RecoverMachine(a)
	require.ErrorContains(t, err, "write failed")
}

func TestDurableReceiptDispatchFactsRemainIndependentFromCleanup(t *testing.T) {
	r := codexProRequest(t)
	r.Protocol, r.ThreadMode = 3, "persistent"
	m := codexProFixture(r)
	e := proObject(m, "outcome", "execution")
	e["threadId"], e["turnId"], e["dispatch"] = nil, nil, "not-dispatched"
	e["failure"] = map[string]any{"stage": "lifecycle", "category": "protocol"}
	m["ok"], m["outcome"], m["failure"] = false, nil, map[string]any{"stage": "lifecycle", "category": "protocol", "diagnostic": "ack refused", "execution": e, "cleanup": map[string]any{"confirmed": true}, "usage": nil, "configuration": nil, "turnConfiguration": nil}
	out := recoveredMachineResult("codex-pro", mustJSONDriver(t, m), r)
	assert.Equal(t, "not-dispatched", out.Session.Dispatch)
	assert.NotNil(t, out.CostUSD)
	assert.NotNil(t, out.ErrorMessage)
	e["dispatch"] = "possible"
	out = recoveredMachineResult("codex-pro", mustJSONDriver(t, m), r)
	assert.Equal(t, "unknown", out.Session.Dispatch)
	assert.Nil(t, out.CostUSD)
	e["dispatch"] = "invented"
	out = recoveredMachineResult("codex-pro", mustJSONDriver(t, m), r)
	assert.Equal(t, "unknown", out.Session.Outcome)
	cr := claudeCodeProRequest(t)
	cr.Protocol = 3
	cm := claudeCodeProFixture(cr)
	proObject(cm, "execution")["turnDispatch"] = "possible"
	out = recoveredMachineResult("claude-code-pro", mustJSONDriver(t, cm), codexProStart(cr))
	assert.False(t, out.Settled)
	assert.Equal(t, "unknown", out.Session.Outcome)
	cm = claudeCodeProFixture(cr)
	cm["ok"], cm["output"], cm["usage"] = false, []any{}, claudeCodeProUnknownUsage()
	cm["failure"] = map[string]any{"stage": "dispatch", "category": "cancelled", "detail": "acknowledgement lost"}
	cm["execution"] = map[string]any{"pid": nil, "processGroupId": nil, "ownership": "none", "dispatch": "not-dispatched", "turnDispatch": "possible",
		"outcome": "not-started", "terminalReceived": false, "exitCode": nil, "signal": nil, "cancelled": false, "timedOut": false, "truncated": false}
	cm["cleanup"] = map[string]any{"confirmed": true, "method": "not-started"}
	proObject(cm, "session")["id"], proObject(cm, "session")["resumed"], proObject(cm, "session")["resumable"] = nil, false, false
	cm["configuration"] = map[string]any{"source": nil, "observed": nil, "matched": false}
	out = recoveredMachineResult("claude-code-pro", mustJSONDriver(t, cm), codexProStart(cr))
	assert.Equal(t, "unknown", out.Session.Dispatch)
	assert.Nil(t, out.CostUSD)
	assert.Nil(t, out.Usage)
	proObject(cm, "execution")["turnDispatch"] = "not-dispatched"
	out = recoveredMachineResult("claude-code-pro", mustJSONDriver(t, cm), codexProStart(cr))
	assert.Equal(t, "not-dispatched", out.Session.Dispatch)
	assert.NotNil(t, out.CostUSD)
}

func mustJSONDriver(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}
