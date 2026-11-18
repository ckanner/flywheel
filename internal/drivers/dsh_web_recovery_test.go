package drivers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func webRecoveryFrame(q dshWebRecoveryRequest, scenario string) map[string]any {
	a := q.Original
	r := dshStart{Protocol: 2, RequestID: q.RequestID, Cwd: a.Cwd, WorkspaceCwd: a.WorkspaceCwd,
		Provider: a.Provider, Model: a.Model, ReasoningEffort: a.ReasoningEffort, PermissionMode: a.PermissionMode, Persistent: true, SessionID: a.RequestedSessionID,
		Web: &DSHWebRequest{Version: 1, ExecutionDomainID: a.ExecutionDomainID, RequireIsolation: true}}
	ready, result := dshFixtureFrames(r)
	if scenario == "unsent" {
		result["status"], result["text"], result["error"] = "error", "", map[string]any{"code": "TRANSPORT", "message": "original call never dispatched"}
		lifecycle := result["lifecycle"].(map[string]any)
		lifecycle["dispatch"], lifecycle["outcome"], lifecycle["resumeCandidate"], lifecycle["turnId"] = "not-dispatched", "failed", false, ""
		receipt := result["web"].(DSHWebReceipt)
		receipt.Facts.Dispatch, receipt.Facts.Outcome = "not-dispatched", "failed"
		result["web"] = receipt
	}
	if scenario == "unconfirmed" {
		result["settled"] = false
	}
	return map[string]any{"protocol": 1, "type": "web-recovery", "requestId": q.RequestID, "revision": strings.Repeat("a", 64), "frames": []any{ready, result}}
}

func TestDSHWebReadOnlyRecoveryValidatesOriginalFrames(t *testing.T) {
	q := dshWebRecoveryRequest{RequestID: "original", Original: dshWebRecoveryOriginal{Cwd: "/repo", Provider: "chatgpt", Model: DSHWebModel,
		ReasoningEffort: "pro", PermissionMode: "read-only", Persistent: true, ExecutionDomainID: "task-domain"}}
	for _, scenario := range []string{"ok", "unsent", "unconfirmed", "wrong-id", "bad-revision", "short-revision", "extra-frame", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			frame := webRecoveryFrame(q, scenario)
			switch scenario {
			case "wrong-id":
				frame["requestId"] = "foreign"
			case "bad-revision":
				frame["revision"] = strings.Repeat("z", 64)
			case "short-revision":
				frame["revision"] = "aa"
			case "extra-frame":
				frame["frames"] = []any{}
			}
			raw := dshJSONL(frame)
			if scenario == "malformed" {
				raw = "{"
			}
			got, err := parseDSHWebRecovery(raw, q)
			if scenario != "ok" && scenario != "unsent" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, got.Recovered)
			assert.True(t, got.Settled)
			assert.Nil(t, got.Usage)
			assert.Nil(t, got.CostUSD)
			assert.Equal(t, q.Original.Provider, got.Provider)
			if scenario == "unsent" {
				assert.Equal(t, 1, *got.ExitCode)
				assert.NotNil(t, got.ErrorMessage)
				assert.Equal(t, "not-dispatched", got.Session.Dispatch)
			} else {
				assert.Equal(t, 0, *got.ExitCode)
				assert.Equal(t, "verified response", got.Text)
			}
		})
	}
}

func TestDSHWebRecoveryProcessFixture(t *testing.T) {
	scenario := os.Getenv("FLYWHEEL_TEST_WEB_RECOVERY")
	if scenario == "" {
		return
	}
	var q dshWebRecoveryRequest
	input, inputErr := io.ReadAll(os.Stdin)
	if inputErr != nil || json.Unmarshal(input, &q) != nil || q.Type != "web-recover" || q.Protocol != 1 || os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("FLYWHEEL_DSH_PROFILE") != "sdk" || os.Getenv("DSH_HOME") == "" {
		os.Exit(2)
	}
	if scenario == "original-cwd" {
		cwd, err := os.Getwd()
		if err != nil || cwd != q.Original.Cwd {
			os.Exit(3)
		}
	}
	raw, _ := json.Marshal(q)
	_ = os.WriteFile(filepath.Join(q.Original.Cwd, "query.json"), raw, 0o600)
	fmt.Print(dshJSONL(webRecoveryFrame(q, scenario)))
	if scenario == "bad-exit" {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestDSHWebRecoverQueryNeverStartsSDKOrSendsPrompt(t *testing.T) {
	a := DSHWebRecoveryArgs{RequestID: "original", RecoveryDirectory: t.TempDir(), Cwd: t.TempDir(), Provider: "chatgpt", PermissionMode: "read-only", ExecutionDomainID: "domain"}
	t.Setenv("OPENAI_API_KEY", "must-not-reach-the-query")
	t.Setenv("DSH_HOME", t.TempDir())
	t.Setenv("FLYWHEEL_DSH_PROFILE", "sdk")
	t.Setenv("FLYWHEEL_DSH_BIN", "original-owned-dsh")
	for _, scenario := range []string{"ok", "unsent", "unconfirmed", "bad-exit"} {
		command := kimiScript(t, "FLYWHEEL_TEST_WEB_RECOVERY="+scenario+" exec "+shQuote(os.Args[0])+" -test.run '^TestDSHWebRecoveryProcessFixture$'\n")
		t.Setenv(DSHWorkerEnv, command)
		got, err := RecoverDSHWeb(a)
		if scenario == "unconfirmed" || scenario == "bad-exit" {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		assert.True(t, got.Recovered)
		raw, err := os.ReadFile(filepath.Join(a.Cwd, "query.json"))
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "prompt")
		assert.Contains(t, string(raw), `"model":"gpt-6-pro-web"`)
		assert.Contains(t, string(raw), `"executionDomainId":"domain"`)
	}
	bad := a
	bad.RecoveryDirectory = "relative"
	_, err := RecoverDSHWeb(bad)
	require.Error(t, err)
	t.Setenv(DSHWorkerEnv, filepath.Join(t.TempDir(), "missing"))
	_, err = RecoverDSHWeb(a)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Watchdog.Context = ctx
	_, err = RecoverDSHWeb(a)
	require.Error(t, err)
	a.Watchdog.Context = nil
	t.Setenv(DSHWorkerEnv, kimiScript(t, "FLYWHEEL_TEST_WEB_RECOVERY=ok exec "+shQuote(os.Args[0])+" -test.run '^TestDSHWebRecoveryProcessFixture$'\n"))
	old := encodeDSHWebRecovery
	t.Cleanup(func() { encodeDSHWebRecovery = old })
	encodeDSHWebRecovery = func(*json.Encoder, any) error { return errors.New("broken pipe") }
	_, err = RecoverDSHWeb(a)
	require.ErrorContains(t, err, "write failed")
}

func TestDSHWebRecoveryKeepsOriginalWorkerDirectory(t *testing.T) {
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	a := DSHWebRecoveryArgs{RequestID: "original", RecoveryDirectory: t.TempDir(), Cwd: cwd,
		Provider: "chatgpt", PermissionMode: "read-only", ExecutionDomainID: "domain"}
	t.Setenv("OPENAI_API_KEY", "must-not-reach-the-query")
	t.Setenv("DSH_HOME", t.TempDir())
	t.Setenv("FLYWHEEL_DSH_PROFILE", "sdk")
	worker := filepath.Join(cwd, "owned-worker")
	require.NoError(t, os.WriteFile(worker, []byte("#!/bin/sh\nFLYWHEEL_TEST_WEB_RECOVERY=original-cwd exec "+shQuote(os.Args[0])+" -test.run '^TestDSHWebRecoveryProcessFixture$'\n"), 0o700))
	for name, command := range map[string]string{"absolute-worker": worker, "relative-worker": "./owned-worker"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(DSHWorkerEnv, command)
			result, err := RecoverDSHWeb(a)
			require.NoError(t, err)
			require.True(t, result.Recovered)
			require.Equal(t, "completed", result.Session.Outcome)
		})
	}
}

func TestDSHWebRecoveryDirectoryRequiresFormalPersistentInvocation(t *testing.T) {
	r := dshStart{Protocol: 2, Persistent: true, Provider: "chatgpt", Model: DSHWebModel, ReasoningEffort: "pro", RequestID: "original", Cwd: "/repo"}
	w := &DSHWebRequest{Version: 1, ExecutionDomainID: "domain", RequireIsolation: true, RecoveryDirectory: "/repo/task/.sessions/recovery-call"}
	_, err := prepareDSHWeb(w, r)
	require.NoError(t, err)
	w.RecoveryDirectory = "relative"
	_, err = prepareDSHWeb(w, r)
	require.ErrorContains(t, err, "recovery")
	w.RecoveryDirectory = "/repo/task/.sessions/recovery-call"
	r.Persistent = false
	_, err = prepareDSHWeb(w, r)
	require.ErrorContains(t, err, "recovery")
}
