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

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/require"
)

func kimiNativeFrames(r kimiNativeStart) (map[string]any, map[string]any) {
	id := r.SessionID
	if id == "" {
		id = "kimi-one"
	}
	ready := map[string]any{"type": "ready", "protocol": 1, "requestId": r.RequestID, "sessionId": id}
	end := map[string]any{"type": "result", "protocol": 1, "requestId": r.RequestID, "sessionId": id,
		"status": "ok", "settled": true, "text": "root response", "usage": nil,
		"native": nativeReceiptFixture(r.Native, true, true),
		"lifecycle": map[string]any{"protocol": KimiNativeProtocol, "requestId": r.RequestID, "sessionId": id,
			"dispatch": "dispatched", "outcome": "completed", "cleanupConfirmed": true, "resumeCandidate": r.Persistent,
			"resumeAccepted": r.SessionID != "", "turnId": "turn-one", "usageStatus": "unknown"}}
	return ready, end
}

func TestKimiNativeStrictStreamAndTurnEvidence(t *testing.T) {
	r := kimiNativeStart{RequestID: "kimi-call", Persistent: true, Native: json.RawMessage(`{"version":1,"mode":"swarm"}`)}
	ready, end := kimiNativeFrames(r)
	event := map[string]any{"type": "event", "protocol": 1, "requestId": r.RequestID, "sessionId": "kimi-one", "event": map[string]any{"type": "child", "id": "worker"}}
	out, err := parseKimiNativeStream("\n"+dshJSONL(ready, event, end), r)
	require.NoError(t, err)
	require.True(t, out.Settled)
	require.Equal(t, "root response", out.Text)
	require.Nil(t, out.Usage)
	for name, mutate := range map[string]func(map[string]any, map[string]any){
		"wrong protocol":           func(a, b map[string]any) { a["protocol"] = 2 },
		"unknown frame":            func(a, b map[string]any) { a["type"] = "other" },
		"ready without id":         func(a, b map[string]any) { delete(a, "sessionId") },
		"result without cleanup":   func(a, b map[string]any) { delete(b, "settled") },
		"wrong result id":          func(a, b map[string]any) { b["sessionId"] = "other" },
		"missing admission":        func(a, b map[string]any) { delete(b, "native") },
		"missing lifecycle":        func(a, b map[string]any) { delete(b, "lifecycle") },
		"wrong lifecycle":          func(a, b map[string]any) { b["lifecycle"].(map[string]any)["protocol"] = "other" },
		"unknown dispatch":         func(a, b map[string]any) { b["lifecycle"].(map[string]any)["dispatch"] = "unknown" },
		"invented usage certainty": func(a, b map[string]any) { b["lifecycle"].(map[string]any)["usageStatus"] = "complete" },
	} {
		t.Run(name, func(t *testing.T) {
			a, b := kimiNativeFrames(r)
			mutate(a, b)
			_, err := parseKimiNativeStream(dshJSONL(a, b), r)
			require.Error(t, err)
		})
	}
	for _, stream := range []string{`{`, dshJSONL(ready), dshJSONL(ready, ready, end), dshJSONL(ready, end, event), dshJSONL(event)} {
		_, err := parseKimiNativeStream(stream, r)
		require.Error(t, err)
	}
	end["usage"] = map[string]any{"input": 10, "output": 3, "totalTokens": 13}
	end["lifecycle"].(map[string]any)["usageStatus"] = "complete"
	out, err = parseKimiNativeStream(dshJSONL(ready, end), r)
	require.NoError(t, err)
	require.Equal(t, int64(13), out.Usage.TotalTokens)
	end["status"] = "error"
	end["error"] = map[string]any{"code": "QUOTA", "message": "account limit"}
	end["lifecycle"].(map[string]any)["outcome"] = "failed"
	end["lifecycle"].(map[string]any)["resumeCandidate"] = false
	out, err = parseKimiNativeStream(dshJSONL(ready, end), r)
	require.NoError(t, err)
	require.False(t, out.Settled)
	require.Contains(t, *out.ErrorMessage, "QUOTA")
}

func TestKimiNativeProcessFixture(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_KIMI_NATIVE") == "" {
		return
	}
	var r kimiNativeStart
	if err := json.NewDecoder(os.Stdin).Decode(&r); err != nil {
		os.Exit(2)
	}
	ready, end := kimiNativeFrames(r)
	if r.Prompt == "record" {
		_ = os.WriteFile(filepath.Join(r.Cwd, "native-input.json"), []byte(dshJSONL(map[string]any{"request": r, "home": os.Getenv("KIMI_CODE_HOME")})), 0600)
	}
	fmt.Print(dshJSONL(ready, end))
	os.Exit(0)
}

func TestRunKimiNativeRealPipesAndBinding(t *testing.T) {
	worker := kimiScript(t, "GORACE=atexit_sleep_ms=0 exec "+shQuote(os.Args[0])+" -test.run '^TestKimiNativeProcessFixture$'\n")
	t.Setenv(KimiNativeWorkerEnv, worker)
	t.Setenv("KIMI_CODE_HOME", "/explicit-account-home")
	cwd := t.TempDir()
	args := KimiArgs{Native: json.RawMessage(`{"version":1,"mode":"agent"}`), Cwd: cwd, Prompt: "record", PermissionMode: "danger-full-access", APIKeys: map[string]string{"FLYWHEEL_TEST_KIMI_NATIVE": "1"}}
	got := RunKimi(args)
	require.True(t, got.Settled, "%+v", got)
	require.NotEmpty(t, got.NativeReceipt)
	require.Equal(t, KimiNativeProtocol, got.Session.Protocol)
	raw, err := os.ReadFile(filepath.Join(cwd, "native-input.json"))
	require.NoError(t, err)
	require.Contains(t, string(raw), KimiDefaultModel)
	require.Contains(t, string(raw), "/explicit-account-home")
	require.Contains(t, string(raw), `"idleTimeoutMs":300000`)
	var events []string
	args.OnEvent = func(s string) { events = append(events, s) }
	args.Command = "/explicit/kimi"
	args.RequestID = "second"
	args.SessionID = "kimi-one"
	args.PersistentSession = true
	args.Model = "chosen"
	args.Thinking = "max"
	args.Watchdog.IdleTimeoutMs = 1200
	got = RunKimi(args)
	require.True(t, got.Settled)
	require.True(t, got.Session.ResumeAccepted)
	require.Len(t, events, 2)
	raw, err = os.ReadFile(filepath.Join(cwd, "native-input.json"))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"idleTimeoutMs":1200`, "the progress cadence must use the parent's effective idle deadline")
	a := CallArgs{Cwd: cwd, WorkspaceCwd: t.TempDir(), Native: args.Native, Prompt: "record", PermissionMode: "danger-full-access", Binding: config.LoopBinding{Harness: "kimi", Model: "chosen"}, APIKeys: args.APIKeys}
	got = InvokeHarness(a, "")
	require.True(t, got.Settled, "%+v", got)
	raw, err = os.ReadFile(filepath.Join(cwd, "native-input.json"))
	require.NoError(t, err)
	require.Contains(t, string(raw), a.WorkspaceCwd)
	t.Setenv(KimiNativeWorkerEnv, "")
	require.Equal(t, "flywheel-kimi-native-worker", KimiNativeCommand())
}

func TestKimiNativeAdmissionFailuresNeverDispatch(t *testing.T) {
	base := KimiArgs{Native: json.RawMessage(`{"version":1,"mode":"agent"}`), Cwd: t.TempDir(), Prompt: "p", PermissionMode: "danger-full-access"}
	for _, mutate := range []func(*KimiArgs){
		func(a *KimiArgs) { a.PermissionMode = "workspace-write" }, func(a *KimiArgs) { a.Prompt = " " }, func(a *KimiArgs) { a.SessionID = "foreign" },
		func(a *KimiArgs) { a.Cwd = filepath.Join(a.Cwd, "missing") }, func(a *KimiArgs) { a.Native = json.RawMessage(`{`) }, func(a *KimiArgs) { a.Prompt = strings.Repeat("x", machineRequestBytes) },
	} {
		a := base
		mutate(&a)
		out := RunKimi(a)
		require.False(t, out.Settled)
		require.Equal(t, "not-dispatched", out.Session.Dispatch)
	}
	t.Setenv(KimiNativeWorkerEnv, filepath.Join(t.TempDir(), "missing-worker"))
	out := RunKimi(base)
	require.NotEmpty(t, out.SpawnError)
	require.Equal(t, "not-dispatched", out.Session.Dispatch)
	require.NotNil(t, out.Usage)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	base.Watchdog.Context = ctx
	out = RunKimi(base)
	require.True(t, out.Cancelled)
	require.Equal(t, "not-dispatched", out.Session.Dispatch)
}

func TestKimiNativeOuterProcessCannotSanitizeUnknownCleanup(t *testing.T) {
	r := kimiNativeStart{RequestID: "call", Persistent: true, Native: json.RawMessage(`{"version":1,"mode":"agent"}`)}
	a, b := kimiNativeFrames(r)
	ok := 0
	bad := 7
	for _, process := range []watchdog.Result{
		{Stdout: dshJSONL(a, b), ExitCode: &bad}, {Stdout: dshJSONL(a, b), ExitCode: &ok, TimedOut: true},
		{Stdout: dshJSONL(a, b), ExitCode: &ok, Truncated: true}, {Stdout: dshJSONL(a, b), ExitCode: &ok, CleanupError: "group still alive"},
		{Stdout: dshJSONL(a, b), Signal: "SIGKILL"}, {Stdout: dshJSONL(a, b)},
	} {
		out := finishKimiNative(r, process, nil)
		require.False(t, out.Settled)
		require.False(t, out.Session.ResumeCandidate)
		require.Equal(t, "unknown", out.Session.Outcome)
	}
	out := finishKimiNative(r, watchdog.Result{ExitCode: &ok}, errors.New("write failed"))
	require.NotEmpty(t, out.CleanupError)
	require.Nil(t, out.Usage)
	b["status"] = "cancelled"
	b["lifecycle"].(map[string]any)["outcome"] = "cancelled"
	b["lifecycle"].(map[string]any)["resumeCandidate"] = false
	out = finishKimiNative(r, watchdog.Result{Stdout: dshJSONL(a, b), ExitCode: &bad}, nil)
	require.True(t, out.Cancelled)
	require.False(t, out.Settled)
}
