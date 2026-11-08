package drivers

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Opt in after the plugin build: Go's normal gate needs only Go, while this
// verifies the real Go -> Node worker -> fake SDK stdio boundary without a model.
func TestDSHNodeWorkerOfflineBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("set FLYWHEEL_TEST_NODE_BRIDGE=1 after building plugins/dsh")
	}
	_, err := exec.LookPath("node")
	require.NoError(t, err)
	worker, err := filepath.Abs("../../plugins/dsh/dist/worker.js")
	require.NoError(t, err)
	fixture, err := filepath.Abs("../../plugins/dsh/test/fixtures/fake-dsh.js")
	require.NoError(t, err)
	t.Setenv("FLYWHEEL_DSH_BIN", fixture)
	r := RunDSH(DSHArgs{Command: worker, Cwd: t.TempDir(), Prompt: "offline boundary verification",
		Provider: "deepseek-official", Model: "deepseek-flash", Thinking: "high", PermissionMode: "workspace-write",
		Watchdog: watchdog.Opts{IdleTimeoutMs: 3000, WallTimeoutMs: 10000, KillGraceMs: 100}})
	require.True(t, r.Settled, "%+v", r)
	assert.Equal(t, "fixture complete", r.Text)
	require.NotNil(t, r.Usage)
	assert.Equal(t, Usage{Input: 120, Output: 8, Reasoning: 3, CacheRead: 20, TotalTokens: 128}, *r.Usage)
	assert.Nil(t, r.CostUSD)

	t.Run("scoped persisted public SDK history", func(t *testing.T) {
		fixture, err := filepath.Abs("../../plugins/dsh/test/fixtures/persisted-sdk.js")
		require.NoError(t, err)
		require.NoError(t, os.Chmod(fixture, 0o755))
		t.Setenv("FLYWHEEL_DSH_BIN", fixture)
		root := t.TempDir()
		calls := filepath.Join(root, "model-calls.jsonl")
		a := DSHArgs{Command: worker, Cwd: t.TempDir(), Prompt: "FIRST_CONTRACT_MARKER",
			Provider: "offline-fixture", Model: "test-model", Thinking: "high", PermissionMode: "danger-full-access",
			PersistentSession: true, RequestID: "first-invocation",
			APIKeys:  map[string]string{"TEST_DSH_HISTORY": filepath.Join(root, "history"), "TEST_DSH_LLM_CALLS": calls},
			Watchdog: watchdog.Opts{IdleTimeoutMs: 3000, WallTimeoutMs: 10000, KillGraceMs: 100}}
		first := RunDSH(a)
		require.True(t, first.Settled, "%+v", first)
		require.NotNil(t, first.Session)
		assert.True(t, first.Session.ResumeCandidate)
		assert.False(t, first.Session.ResumeAccepted)
		require.NotEmpty(t, first.Session.SessionID)
		a.SessionID, a.RequestID, a.Prompt = first.Session.SessionID, "second-invocation", "CURRENT_FAILURE_MARKER"
		second := RunDSH(a)
		require.True(t, second.Settled, "%+v", second)
		require.NotNil(t, second.Session)
		assert.Equal(t, first.Session.SessionID, second.Session.SessionID)
		assert.True(t, second.Session.ResumeAccepted)
		assert.True(t, second.Session.CleanupConfirmed)
		assert.True(t, second.Session.ResumeCandidate)
		assert.NotEqual(t, first.Session.TurnID, second.Session.TurnID)
		require.NotNil(t, second.Usage)
		assert.Equal(t, int64(12), second.Usage.TotalTokens)
		raw, err := os.ReadFile(calls)
		require.NoError(t, err)
		entries := strings.Split(strings.TrimSpace(string(raw)), "\n")
		require.Len(t, entries, 2)
		assert.Contains(t, entries[1], "FIRST_CONTRACT_MARKER")
		assert.Contains(t, entries[1], "published SDK fixture complete")
		assert.Contains(t, entries[1], "CURRENT_FAILURE_MARKER")
		a.SessionID, a.RequestID = "absent-history", "third-invocation"
		missing := RunDSH(a)
		assert.False(t, missing.Settled)
		require.NotNil(t, missing.Session)
		assert.Equal(t, "not-dispatched", missing.Session.Dispatch)
		assert.Equal(t, "unknown", missing.Session.Outcome)
		assert.False(t, missing.Session.ResumeCandidate)
		after, err := os.ReadFile(calls)
		require.NoError(t, err)
		assert.Equal(t, string(raw), string(after), "a failed restore must not create or prompt a replacement session")
	})

	for scenario, want := range map[string]errs.ErrorCode{
		"account-sign-in-required": errs.AUTH, "account-token-invalid": errs.AUTH,
		"account-signed-out": errs.AUTH, "account-quota": errs.QUOTA,
		"account-code-in-rpc-message": errs.TRANSPORT,
		"account-quota-corrupt-tail":  errs.TRANSPORT, "account-quota-incomplete-tail": errs.TRANSPORT,
		"account-token-invalid-corrupt-tail": errs.TRANSPORT, "account-token-invalid-incomplete-tail": errs.TRANSPORT,
		"shutdown-corrupt-tail": errs.TRANSPORT, "shutdown-incomplete-tail": errs.TRANSPORT,
		"account-token-invalid-corrupt-tail-unclean-exit": errs.TRANSPORT,
		"account-quota-incomplete-tail-unclean-exit":      errs.TRANSPORT,
	} {
		t.Run(scenario, func(t *testing.T) {
			result := RunDSH(DSHArgs{Command: worker, Cwd: t.TempDir(), Prompt: "offline account error receipt",
				Provider: "deepseek-account", Model: "deepseek-flash", Thinking: "high", PermissionMode: "read-only",
				APIKeys:  map[string]string{"TEST_DSH_SCENARIO": scenario},
				Watchdog: watchdog.Opts{IdleTimeoutMs: 3000, WallTimeoutMs: 10000, KillGraceMs: 100}})
			assert.False(t, result.Settled)
			assert.False(t, result.Cancelled)
			if strings.HasSuffix(scenario, "-unclean-exit") {
				assert.Contains(t, result.CleanupError, "did not confirm SDK process cleanup")
			} else {
				assert.Empty(t, result.CleanupError)
			}
			require.NotNil(t, result.ErrorMessage)
			assert.Equal(t, want, result.ErrorCode, *result.ErrorMessage)
			assert.Equal(t, want, errs.ClassifyError(errs.SpawnResult{ExitCode: 1, ErrorCode: result.ErrorCode, Stderr: *result.ErrorMessage}))
		})
	}

	t.Run("full access execution and no-inference probe", func(t *testing.T) {
		for _, doctor := range []bool{false, true} {
			log := filepath.Join(t.TempDir(), "full-access.jsonl")
			r := RunDSH(DSHArgs{Command: worker, Doctor: doctor, Cwd: t.TempDir(), Prompt: "offline full-access task",
				Provider: "deepseek-official", Model: "deepseek-flash", Thinking: "high", PermissionMode: "danger-full-access",
				APIKeys:  map[string]string{"TEST_DSH_LOG": log},
				Watchdog: watchdog.Opts{IdleTimeoutMs: 3000, WallTimeoutMs: 10000, KillGraceMs: 100}})
			require.True(t, r.Settled, "%+v", r)
			prompted := false
			for _, request := range readDSHSDKRequests(t, log) {
				prompted = prompted || request.Method == "session/prompt"
			}
			assert.Equal(t, !doctor, prompted)
		}
	})

	t.Run("explicit provider differs from account channel", func(t *testing.T) {
		t.Setenv(DSHWorkerEnv, worker)
		log := filepath.Join(t.TempDir(), "sdk.jsonl")
		r := InvokeHarness(CallArgs{Prompt: "offline provider routing", Cwd: t.TempDir(),
			Binding:        config.LoopBinding{Harness: "dsh", Provider: "chatgpt", Channel: "personal-subscription", Model: "gpt-test", Thinking: "high"},
			PermissionMode: "read-only", APIKeys: map[string]string{"TEST_DSH_LOG": log},
			Watchdog: watchdog.Opts{IdleTimeoutMs: 3000, WallTimeoutMs: 10000, KillGraceMs: 100}}, "")
		require.True(t, r.Settled, "%+v", r)
		assert.Equal(t, "chatgpt", r.Provider)
		requests := readDSHSDKRequests(t, log)
		assert.Equal(t, "chatgpt", requests[0].Params["provider"])
		assert.Equal(t, "gpt-test", requests[0].Params["model"])
	})

	for _, scenario := range []string{"ok", "missing-policy", "wrong-policy", "wrong-cwd", "malformed", "forced-shutdown"} {
		t.Run("doctor/"+scenario, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "sdk.jsonl")
			r := RunDSH(DSHArgs{Command: worker, Doctor: true, Cwd: t.TempDir(),
				Prompt: "never forward this application task", Provider: "chatgpt", Model: "gpt-test", Thinking: "high", PermissionMode: "read-only",
				APIKeys:  map[string]string{"TEST_DSH_LOG": log, "TEST_DSH_SCENARIO": scenario},
				Watchdog: watchdog.Opts{IdleTimeoutMs: 3000, WallTimeoutMs: 10000, KillGraceMs: 100}})
			assert.Equal(t, scenario == "ok", r.Settled, "%+v", r)
			assert.Nil(t, r.Usage, "a local probe has no inference usage")
			assert.Nil(t, r.CostUSD)
			requests := readDSHSDKRequests(t, log)
			for _, request := range requests {
				assert.NotEqual(t, "session/prompt", request.Method)
				assert.NotEqual(t, "session/wait", request.Method)
			}
			if scenario == "ok" {
				require.Len(t, requests, 3)
				assert.Equal(t, "initialize", requests[0].Method)
				assert.Equal(t, "chatgpt", requests[0].Params["provider"])
				assert.Equal(t, "session/working-directory/get", requests[1].Method)
				assert.Equal(t, "shutdown", requests[2].Method)
				assert.Contains(t, r.Text, "no model prompt sent")
			}
		})
	}
}

type dshSDKRequest struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func readDSHSDKRequests(t *testing.T, path string) []dshSDKRequest {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var requests []dshSDKRequest
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var request dshSDKRequest
		require.NoError(t, json.Unmarshal([]byte(line), &request))
		requests = append(requests, request)
	}
	return requests
}
