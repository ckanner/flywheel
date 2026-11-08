package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dshFixtureFrames(r dshStart) (map[string]any, map[string]any) {
	sessionID := r.SessionID
	if sessionID == "" {
		sessionID = "sdk-session-one"
	}
	ready := map[string]any{
		"type": "ready", "protocol": r.Protocol, "requestId": r.RequestID, "sessionId": sessionID,
		"cwd": r.Cwd, "provider": r.Provider, "model": r.Model, "reasoningEffort": r.ReasoningEffort,
		"permissionMode": r.PermissionMode, "approvalPolicy": "never",
	}
	result := map[string]any{
		"type": "result", "protocol": r.Protocol, "requestId": r.RequestID, "sessionId": sessionID,
		"status": "ok", "settled": true, "text": "verified response", "usage": nil, "costUsd": nil,
	}
	if r.Protocol == 2 {
		ready["persistent"], ready["resumed"] = true, r.SessionID != ""
		result["lifecycle"] = map[string]any{"protocol": DSHSessionProtocol, "requestId": r.RequestID,
			"sessionId": sessionID, "dispatch": "dispatched", "outcome": "completed", "cleanupConfirmed": true,
			"resumeCandidate": true, "resumeAccepted": r.SessionID != "", "turnId": "1", "usageStatus": "unknown"}
	}
	return ready, result
}

func dshJSONL(frames ...map[string]any) string {
	var b strings.Builder
	for _, frame := range frames {
		raw, _ := json.Marshal(frame)
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestDSHProtocolReceiptAndTerminalValidation(t *testing.T) {
	request := dshStart{Protocol: 1, RequestID: "request-one", Cwd: "/repo", Provider: "deepseek", Model: "deepseek-v4-flash", ReasoningEffort: "high", PermissionMode: "workspace-write"}
	ready, result := dshFixtureFrames(request)
	event := map[string]any{"type": "event", "protocol": 1, "requestId": request.RequestID, "sessionId": "sdk-session-one", "event": map[string]any{"type": "assistant/message"}}
	parsed := parseDSHStream("\n"+dshJSONL(ready, event, result), request)
	assert.True(t, parsed.Settled)
	assert.Equal(t, "verified response", parsed.Text)
	assert.Equal(t, "sdk-session-one", parsed.SessionID)
	assert.Nil(t, parsed.Usage)
	assert.Nil(t, parsed.CostUSD)
	assert.Empty(t, parsed.Error)

	for _, tc := range []struct {
		name   string
		change func(map[string]any, map[string]any)
	}{
		{"wrong version", func(r, _ map[string]any) { r["protocol"] = 2 }},
		{"wrong request", func(r, _ map[string]any) { r["requestId"] = "someone-else" }},
		{"missing session", func(r, _ map[string]any) { delete(r, "sessionId") }},
		{"wrong cwd", func(r, _ map[string]any) { r["cwd"] = "/other" }},
		{"wrong provider", func(r, _ map[string]any) { r["provider"] = "chatgpt" }},
		{"wrong model", func(r, _ map[string]any) { r["model"] = "deepseek-v4-pro" }},
		{"wrong thinking", func(r, _ map[string]any) { r["reasoningEffort"] = "max" }},
		{"wrong permission", func(r, _ map[string]any) { r["permissionMode"] = "full-access" }},
		{"interactive approval", func(r, _ map[string]any) { r["approvalPolicy"] = "on-request" }},
		{"unknown frame", func(r, _ map[string]any) { r["type"] = "future-ready" }},
		{"unfinished cleanup", func(_, r map[string]any) { r["settled"] = false }},
		{"wrong result session", func(_, r map[string]any) { r["sessionId"] = "other-session" }},
		{"empty result", func(_, r map[string]any) { r["text"] = " " }},
		{"unknown status", func(_, r map[string]any) { r["status"] = "pending" }},
		{"contradictory error", func(_, r map[string]any) { r["error"] = map[string]any{"message": "failed"} }},
		{"invalid usage", func(_, r map[string]any) { r["usage"] = map[string]any{"input_tokens": 0} }},
		{"negative cost", func(_, r map[string]any) { r["costUsd"] = -0.01 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, end := dshFixtureFrames(request)
			tc.change(r, end)
			parsed := parseDSHStream(dshJSONL(r, end), request)
			assert.False(t, parsed.Settled)
			assert.NotEmpty(t, parsed.Error)
		})
	}
	for name, stream := range map[string]string{
		"malformed": "{bad-json}\n", "missing terminal": dshJSONL(ready),
		"missing receipt": dshJSONL(result), "duplicate ready": dshJSONL(ready, ready, result),
		"event before ready": dshJSONL(event), "after terminal": dshJSONL(ready, result, event),
		"event wrong session": dshJSONL(ready, map[string]any{"type": "event", "protocol": 1, "requestId": request.RequestID, "sessionId": "wrong", "event": map[string]any{}}),
		"null event":          dshJSONL(ready, map[string]any{"type": "event", "protocol": 1, "requestId": request.RequestID, "sessionId": "sdk-session-one", "event": nil}),
	} {
		t.Run(name, func(t *testing.T) {
			parsed := parseDSHStream(stream, request)
			assert.False(t, parsed.Settled)
			assert.NotEmpty(t, parsed.Error)
		})
	}
	errorFrame := map[string]any{"type": "result", "protocol": 1, "requestId": request.RequestID, "status": "error", "settled": true, "error": map[string]any{"code": "AUTH", "message": "log in"}}
	failed := parseDSHStream(dshJSONL(errorFrame), request)
	assert.Equal(t, "AUTH: log in", failed.Error)
	assert.False(t, failed.Settled)
	errorFrame["status"] = "cancelled"
	delete(errorFrame, "error")
	cancelled := parseDSHStream(dshJSONL(errorFrame), request)
	assert.True(t, cancelled.Cancelled)
	assert.False(t, cancelled.Settled)
}

func TestDSHUsageContract(t *testing.T) {
	known := `{"input_tokens":120,"output_tokens":40,"reasoning_tokens":25,"cache_read_tokens":80,"cache_write_tokens":10,"total_tokens":160}`
	u, err := parseDSHUsage(json.RawMessage(known))
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, Usage{Input: 120, Output: 40, Reasoning: 25, CacheRead: 80, CacheWrite: 10, TotalTokens: 160}, *u)
	for _, raw := range []string{`{}`, `[]`, `{"input_tokens":null}`, `{"input_tokens":-1}`, `{"input_tokens":"10"}`} {
		_, err := parseDSHUsage(json.RawMessage(raw))
		assert.Error(t, err, raw)
	}
}

// The fake worker is another instance of the Go test binary: tests exercise real
// stdin/stdout pipes, cwd, signals, and exit status without Node or model access.
func TestDSHWorkerProcessFixture(t *testing.T) {
	scenario := os.Getenv("FLYWHEEL_TEST_DSH_SCENARIO")
	if scenario == "" {
		return
	}
	if scenario == "closed-stdin" {
		_ = os.Stdin.Close()
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
	var request dshStart
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if request.Type != "start" || (request.Protocol != 1 && request.Protocol != 2) || request.RequestID == "" {
		os.Exit(2)
	}
	ready, result := dshFixtureFrames(request)
	cwd, _ := os.Getwd()
	ready["cwd"] = cwd
	if request.Prompt == "record input" || scenario == "doctor" {
		data, _ := json.Marshal(map[string]any{"request": request, "env": os.Environ()})
		_ = os.WriteFile("worker-input.json", data, 0o600)
	}
	if scenario == "wrong-settings" {
		ready["model"] = "wrong-model"
	}
	fmt.Print(dshJSONL(ready))
	fmt.Fprintln(os.Stderr, "worker diagnostic")
	if scenario == "exit-error-auth-text" {
		fmt.Fprintln(os.Stderr, "401 unauthorized appears in a process diagnostic")
	}
	if scenario == "hang" {
		time.Sleep(5 * time.Second)
	}
	if scenario == "missing-terminal" {
		os.Exit(1)
	}
	if scenario == "malformed-terminal" {
		fmt.Print("{broken-json}\n")
		os.Exit(1)
	}
	if scenario == "unknown-usage" || scenario == "doctor" {
		result["usage"] = nil
	} else {
		result["usage"] = map[string]any{"input_tokens": 10, "output_tokens": 5, "reasoning_tokens": 3, "cache_read_tokens": 8, "cache_write_tokens": 0, "total_tokens": 15}
		result["costUsd"] = 0.00005
		if request.Protocol == 2 {
			result["lifecycle"].(map[string]any)["usageStatus"] = "complete"
		}
	}
	if scenario == "scoped-empty" {
		result["status"], result["text"] = "error", ""
		result["error"] = map[string]string{"code": "EMPTY_RESPONSE", "message": "complete native turn without text"}
		fmt.Print(dshJSONL(result))
		os.Exit(1)
	}
	if scenario == "cancelled" {
		result["status"] = "cancelled"
	}
	if strings.HasPrefix(scenario, "structured-") || strings.HasPrefix(scenario, "error-") {
		code := "AUTH"
		if scenario == "structured-quota" {
			code = "QUOTA"
		}
		result["status"] = "error"
		result["settled"] = scenario != "error-unconfirmed-cleanup"
		result["error"] = map[string]string{"code": code, "message": "请重新检查账号状态"}
		fmt.Print(dshJSONL(result))
		if scenario == "error-malformed-tail" {
			fmt.Print("{broken-json}\n")
		}
		if scenario == "error-before-timeout" {
			time.Sleep(5 * time.Second)
		}
		if scenario == "error-exit-zero" {
			os.Exit(0)
		}
		if scenario == "error-exit-other" {
			os.Exit(42)
		}
		os.Exit(1)
	}
	fmt.Print(strings.TrimSpace(dshJSONL(result)))
	if scenario == "success-malformed-tail" {
		fmt.Print("\n{broken-json}\n")
	}
	if scenario == "success-extra-frame" {
		fmt.Print("\n" + dshJSONL(ready))
	}
	if scenario == "success-stderr-overflow" {
		fmt.Fprint(os.Stderr, strings.Repeat("diagnostic overflow\n", 2048))
	}
	if scenario == "success-exit-one" {
		os.Exit(1)
	}
	if scenario == "exit-error" {
		os.Exit(7)
	}
	if scenario == "exit-error-auth-text" {
		os.Exit(42)
	}
	if scenario == "orphan-stdio" {
		child := exec.Command("/bin/sh", "-c", "sleep 5")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		_ = child.Start()
	}
	os.Exit(0)
}

func dshWorkerScript(t *testing.T) string {
	t.Helper()
	return kimiScript(t, "exec "+shQuote(os.Args[0])+" -test.run '^TestDSHWorkerProcessFixture$'\n")
}

func dshRunArgs(t *testing.T, scenario string) DSHArgs {
	t.Helper()
	return DSHArgs{Command: dshWorkerScript(t), Cwd: t.TempDir(), Provider: "deepseek", Model: "deepseek-v4-flash", Prompt: "task", PermissionMode: "workspace-write", APIKeys: map[string]string{"FLYWHEEL_TEST_DSH_SCENARIO": scenario}, Watchdog: watchdog.Opts{IdleTimeoutMs: 3000, WallTimeoutMs: 3000, KillGraceMs: 30}}
}

func TestRunDSHWorkerBindingEnvironmentAndUsage(t *testing.T) {
	a := dshRunArgs(t, "ok")
	a.Prompt = "record input"
	a.Thinking = "max"
	var events []string
	a.OnEvent = func(line string) { events = append(events, line) }
	t.Setenv("FLYWHEEL_DSH_BIN", "/Applications/DSH.app/Contents/MacOS/dsh")
	t.Setenv("FLYWHEEL_DSH_PROFILE", "sdk-test")
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "dsh home"))
	t.Setenv("PRIVATE_UNRELATED_KEY", "must-not-leak")
	r := RunDSH(a)
	require.True(t, r.Settled, "%+v", r)
	assert.Equal(t, "deepseek", r.Provider)
	assert.Equal(t, "deepseek-v4-flash", r.Model)
	assert.Equal(t, "verified response", r.Text)
	assert.Equal(t, "sdk-session-one", r.SessionID)
	require.NotNil(t, r.Usage)
	assert.Equal(t, int64(15), r.Usage.TotalTokens, "reasoning is already included in output")
	require.NotNil(t, r.CostUSD)
	assert.Equal(t, 0.00005, *r.CostUSD)
	assert.Len(t, events, 2, "stdout tail is delivered and stderr is separate")
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "worker-input.json"))
	require.NoError(t, err)
	var captured struct {
		Request dshStart `json:"request"`
		Env     []string `json:"env"`
	}
	require.NoError(t, json.Unmarshal(raw, &captured))
	assert.Equal(t, "max", captured.Request.ReasoningEffort)
	assert.Equal(t, "workspace-write", captured.Request.PermissionMode)
	assert.Contains(t, captured.Env, "FLYWHEEL_DSH_PROFILE=sdk-test")
	assert.Contains(t, captured.Env, "DSH_HOME="+os.Getenv("DSH_HOME"))
	assert.NotContains(t, strings.Join(captured.Env, "\n"), "PRIVATE_UNRELATED_KEY")
}

func TestRunDSHWorkerDefaultsAndFailures(t *testing.T) {
	for _, scenario := range []string{"unknown-usage", "wrong-settings", "cancelled", "exit-error", "hang", "orphan-stdio"} {
		t.Run(scenario, func(t *testing.T) {
			a := dshRunArgs(t, scenario)
			a.PermissionMode = ""
			if scenario == "hang" {
				a.Watchdog.WallTimeoutMs = 100
			}
			r := RunDSH(a)
			if scenario == "unknown-usage" {
				assert.True(t, r.Settled, "%+v", r)
				assert.Nil(t, r.Usage)
				assert.Nil(t, r.CostUSD)
				return
			}
			assert.False(t, r.Settled)
			if scenario == "cancelled" {
				assert.True(t, r.Cancelled)
				assert.Nil(t, r.ErrorMessage)
			} else {
				require.NotNil(t, r.ErrorMessage)
			}
			if scenario == "hang" {
				assert.True(t, r.TimedOut)
			}
			if scenario == "orphan-stdio" {
				assert.NotEmpty(t, r.CleanupError)
			}
		})
	}
	for name, mutate := range map[string]func(*DSHArgs){
		"missing provider":       func(a *DSHArgs) { a.Provider = "" },
		"missing model":          func(a *DSHArgs) { a.Model = "" },
		"unsupported resume":     func(a *DSHArgs) { a.SessionID = "old" },
		"unsupported permission": func(a *DSHArgs) { a.PermissionMode = "full-access" },
		"missing cwd":            func(a *DSHArgs) { a.Cwd = filepath.Join(t.TempDir(), "missing") },
		"missing worker":         func(a *DSHArgs) { a.Command = filepath.Join(t.TempDir(), "missing") },
	} {
		t.Run(name, func(t *testing.T) {
			a := dshRunArgs(t, "ok")
			mutate(&a)
			r := RunDSH(a)
			assert.False(t, r.Settled)
			require.NotNil(t, r.ErrorMessage)
		})
	}
}

func TestDSHWorkerTransportAndCancellation(t *testing.T) {
	t.Run("broken stdin", func(t *testing.T) {
		a := dshRunArgs(t, "closed-stdin")
		a.Prompt = strings.Repeat("x", 2*1024*1024)
		r := RunDSH(a)
		assert.False(t, r.Settled)
		require.NotNil(t, r.ErrorMessage)
		assert.Contains(t, *r.ErrorMessage, "request write failed")
		assert.Equal(t, errs.TRANSPORT, r.ErrorCode)
	})
	t.Run("truncated transcript", func(t *testing.T) {
		old := watchdog.MaxBuffer
		watchdog.MaxBuffer = 20
		t.Cleanup(func() { watchdog.MaxBuffer = old })
		r := RunDSH(dshRunArgs(t, "ok"))
		assert.True(t, r.Truncated)
		assert.False(t, r.Settled)
		require.NotNil(t, r.ErrorMessage)
		assert.Contains(t, *r.ErrorMessage, "truncated")
		assert.Equal(t, errs.TRANSPORT, r.ErrorCode)
	})
	t.Run("context cancellation", func(t *testing.T) {
		a := dshRunArgs(t, "hang")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		a.Watchdog.Context = ctx
		a.OnEvent = func(line string) { cancel() }
		r := RunDSH(a)
		assert.True(t, r.Cancelled)
		assert.False(t, r.TimedOut)
		assert.False(t, r.Settled)
	})
}

func TestInvokeHarnessDSHDispatchAndPermissionFailures(t *testing.T) {
	command := dshWorkerScript(t)
	t.Setenv(DSHWorkerEnv, command)
	r := InvokeHarness(CallArgs{Prompt: "p", Model: "deepseek-v4-flash", Binding: config.LoopBinding{Harness: "dsh", Provider: "deepseek-official", Channel: "team-deepseek-account"}, APIKeys: map[string]string{"FLYWHEEL_TEST_DSH_SCENARIO": "ok"}, Cwd: t.TempDir()}, "")
	assert.True(t, r.Settled, "%+v", r)
	assert.Equal(t, "deepseek-official", r.Provider, "channel is an account label, not the adapter sent to DSH")
	t.Setenv(DSHWorkerEnv, "")
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "flywheel-dsh-worker"), []byte("#!/bin/sh\nexec "+shQuote(command)+"\n"), 0o755))
	setPath(t, bin)
	r = InvokeHarness(CallArgs{Prompt: "p", Binding: config.LoopBinding{Harness: "dsh", Channel: "chatgpt", Model: "gpt-6-astra", Thinking: "high"}, APIKeys: map[string]string{"FLYWHEEL_TEST_DSH_SCENARIO": "unknown-usage"}, Cwd: t.TempDir()}, "")
	assert.True(t, r.Settled, "%+v", r)
	assert.Equal(t, "chatgpt", r.Provider)
	assert.Nil(t, r.Usage)

	for _, a := range []CallArgs{
		{Binding: config.LoopBinding{Harness: "ds-h"}},
		{Binding: config.LoopBinding{Harness: "kimi"}, PermissionMode: "read-only"},
		{Binding: config.LoopBinding{Harness: "pi"}, PermissionMode: "invalid"},
		{Binding: config.LoopBinding{Harness: "pi"}, PermissionMode: "danger-full-access"},
		{Binding: config.LoopBinding{Harness: "omp"}, PermissionMode: "danger-full-access"},
		{Binding: config.LoopBinding{Harness: "zcode"}, PermissionMode: "danger-full-access"},
		{Binding: config.LoopBinding{Harness: "codex", Provider: "chatgpt"}},
	} {
		r := InvokeHarness(a, "")
		assert.False(t, r.Settled)
		require.NotNil(t, r.ErrorMessage)
		assert.Contains(t, *r.ErrorMessage, "invalid request")
		assert.Empty(t, r.Stdout, "unsupported scope must fail before any CLI is spawned")
	}
}

func TestDSHDoctorSelectsWorkerAndDiscardsCallerPrompt(t *testing.T) {
	a := dshRunArgs(t, "doctor")
	argsFile := filepath.Join(t.TempDir(), "arguments")
	command := kimiScript(t, "printf '%s\\n' \"$@\" > "+shQuote(argsFile)+"\nexec "+shQuote(a.Command)+"\n")
	t.Setenv(DSHWorkerEnv, command)
	assert.Equal(t, command, DSHCommand())
	a.Command = ""
	a.Doctor = true
	a.Prompt = "this application task must not be sent by doctor"
	r := RunDSH(a)
	require.True(t, r.Settled, "%+v", r)
	assert.Nil(t, r.Usage)
	assert.Nil(t, r.CostUSD)
	args, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	assert.Equal(t, "--doctor\n", string(args))
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "worker-input.json"))
	require.NoError(t, err)
	var captured struct {
		Request dshStart `json:"request"`
	}
	require.NoError(t, json.Unmarshal(raw, &captured))
	assert.NotEqual(t, a.Prompt, captured.Request.Prompt)
	assert.Contains(t, captured.Request.Prompt, "no model prompt")
	t.Setenv(DSHWorkerEnv, "")
	assert.Equal(t, "flywheel-dsh-worker", DSHCommand())
}

func TestDSHDoctorRetainsPhysicalAndProtocolGates(t *testing.T) {
	for _, scenario := range []string{"wrong-settings", "exit-error", "cancelled", "hang", "truncated"} {
		t.Run(scenario, func(t *testing.T) {
			a := dshRunArgs(t, scenario)
			a.Doctor = true
			if scenario == "hang" {
				a.Watchdog.WallTimeoutMs = 100
			}
			if scenario == "truncated" {
				old := watchdog.MaxBuffer
				watchdog.MaxBuffer = 20
				t.Cleanup(func() { watchdog.MaxBuffer = old })
			}
			r := RunDSH(a)
			assert.False(t, r.Settled, "doctor must not relax the worker terminal contract")
			if scenario == "cancelled" {
				assert.True(t, r.Cancelled)
			}
			if scenario == "hang" {
				assert.True(t, r.TimedOut)
			}
			if scenario == "truncated" {
				assert.True(t, r.Truncated)
			}
		})
	}
}
