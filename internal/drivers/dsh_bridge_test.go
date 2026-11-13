package drivers

import (
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

	t.Run("one-use SDK prewarm and early discard", func(t *testing.T) {
		for _, consume := range []bool{false, true} {
			a := warmArgs(t, "ok")
			a.Command, a.Provider, a.Model, a.PermissionMode = worker, "deepseek-account", "deepseek-flash", "read-only"
			log := filepath.Join(a.Cwd, "sdk.jsonl")
			a.APIKeys = map[string]string{"TEST_DSH_LOG": log}
			a.Watchdog.KillGraceMs = 5000
			h := PrepareDSH(a)
			if consume {
				awaitWarm(t, h)
				requests := readDSHSDKRequests(t, log)
				require.Len(t, requests, 1)
				assert.Equal(t, "initialize", requests[0].Method)
				a.PersistentSession, a.RequestID, a.SessionID, a.Prewarm = true, "warm-formal-review", "previous-review", h
				result := RunDSH(a)
				require.True(t, result.Settled, "%+v", result)
				assert.True(t, result.Session.ResumeAccepted)
				assert.Equal(t, "warm-formal-review", result.Session.RequestID)
				var methods []string
				for _, request := range readDSHSDKRequests(t, log) {
					methods = append(methods, request.Method)
				}
				assert.Equal(t, []string{"initialize", "session/resume", "session/working-directory/get", "session/prompt", "session/wait", "shutdown"}, methods)
			}
			require.NoError(t, h.Close())
		}
	})

	t.Run("safe unused expiry uses one cold invocation without transport retry", func(t *testing.T) {
		originalTTL := dshWarmTTL
		dshWarmTTL = time.Second
		t.Cleanup(func() { dshWarmTTL = originalTTL })
		a := warmArgs(t, "ok")
		a.Command, a.Provider, a.Model = worker, "deepseek-account", "deepseek-flash"
		log := filepath.Join(a.Cwd, "sdk.jsonl")
		a.APIKeys = map[string]string{"TEST_DSH_LOG": log}
		a.Watchdog.KillGraceMs = 5000
		h := PrepareDSH(a)
		awaitWarm(t, h)
		<-h.done
		a.RequestID, a.Prewarm = "same-expired-formal-invocation", h
		result := RunDSH(a)
		require.True(t, result.Settled, "%+v", result)
		assert.Equal(t, a.RequestID, result.Session.RequestID)
		var methods []string
		for _, request := range readDSHSDKRequests(t, log) {
			methods = append(methods, request.Method)
		}
		assert.Equal(t, []string{"initialize", "shutdown", "initialize", "session/working-directory/get", "session/prompt", "session/wait", "shutdown"}, methods)
		require.NoError(t, h.Close())
	})

	t.Run("prewarm initialization keeps nonretryable worker configuration errors", func(t *testing.T) {
		t.Setenv("FLYWHEEL_DSH_PROFILE", "desktop")
		a := warmArgs(t, "ok")
		a.Command = worker
		a.RequestID, a.PersistentSession = "invalid-prewarm-profile", true
		log := filepath.Join(a.Cwd, "sdk.jsonl")
		a.APIKeys = map[string]string{"TEST_DSH_LOG": log}
		h := PrepareDSH(a)
		t.Cleanup(func() { _ = h.Close() })
		<-h.done
		result := h.Run(a)
		assert.False(t, result.Settled)
		assert.Equal(t, errs.INVALID_REQUEST, result.ErrorCode)
		require.NotNil(t, result.ErrorMessage)
		assert.Contains(t, *result.ErrorMessage, "FLYWHEEL_DSH_PROFILE must name an SDK profile")
		require.NotNil(t, result.Session)
		assert.Equal(t, "not-dispatched", result.Session.Dispatch)
		assert.True(t, result.Session.CleanupConfirmed)
		assert.Equal(t, &Usage{}, result.Usage)
		assert.NoFileExists(t, log, "invalid initialization never starts an SDK or submits a prompt")
		require.NoError(t, h.Close())
	})

	t.Run("long initialization diagnostics retain bounded recoverable cleanup proof", func(t *testing.T) {
		for _, kind := range []string{"ascii", "multibyte", "escaped"} {
			t.Run(kind, func(t *testing.T) {
				a := warmArgs(t, "ok")
				a.Command = worker
				a.RequestID, a.PersistentSession = "failed-initialize-no-cold-call", true
				log := filepath.Join(a.Cwd, "sdk.jsonl")
				a.APIKeys = map[string]string{"TEST_DSH_LOG": log, "TEST_DSH_SCENARIO": "initialize-long-" + kind}
				h := PrepareDSH(a)
				t.Cleanup(func() { _ = h.Close() })
				select {
				case <-h.done:
				case <-time.After(10 * time.Second):
					t.Fatal("initialization failure did not finish its cleanup")
				}
				result := h.Run(a)
				assert.False(t, result.Settled, "cleaned-up initialization failure is not a successful invocation")
				assert.Equal(t, errs.TRANSPORT, result.ErrorCode)
				require.NotNil(t, result.Session)
				assert.Equal(t, "not-dispatched", result.Session.Dispatch)
				assert.True(t, result.Session.CleanupConfirmed)
				assert.Empty(t, result.CleanupError)
				assert.Equal(t, &Usage{}, result.Usage)
				path := filepath.Join(a.PrewarmDirectory, "terminal.json")
				raw, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.LessOrEqual(t, len(raw), 8192, "full serialized receipt, including newline, obeys the Go limit")
				receipt, err := readDSHWarmReceipt(h.ID(), a.PrewarmDirectory, h.prepare.Binding)
				require.NoError(t, err)
				assert.Equal(t, h.ID(), receipt.WarmID)
				assert.Equal(t, h.prepare.Binding, receipt.Binding)
				assert.True(t, strings.HasPrefix(receipt.Error, "initialize: initialization-error:"))
				assert.True(t, strings.HasSuffix(receipt.Error, " [truncated]"))
				require.NoError(t, RecoverDSHPrewarm(h.ID(), a.PrewarmDirectory, h.prepare.Binding))
				var methods []string
				for _, request := range readDSHSDKRequests(t, log) {
					methods = append(methods, request.Method)
				}
				assert.Equal(t, []string{"initialize", "shutdown"}, methods, "no Session, prompt, or replacement cold worker")

				// Diagnostic truncation cannot weaken the independently checked evidence.
				require.NoError(t, os.Remove(path))
				assert.Error(t, RecoverDSHPrewarm(h.ID(), a.PrewarmDirectory, h.prepare.Binding))
				for _, invalid := range []string{"{broken", strings.Replace(string(raw), `"cleanupConfirmed":true`, `"cleanupConfirmed":false`, 1)} {
					require.NoError(t, os.WriteFile(path, []byte(invalid), 0o600))
					assert.Error(t, RecoverDSHPrewarm(h.ID(), a.PrewarmDirectory, h.prepare.Binding))
				}
				require.NoError(t, os.WriteFile(path, raw, 0o600))
				require.NoError(t, h.Close())
			})
		}
	})

	t.Run("public SDK prewarmed read-only Review through InvokeHarness", func(t *testing.T) {
		publicSDK, err := filepath.Abs("../../plugins/dsh/test/fixtures/persisted-sdk.js")
		require.NoError(t, err)
		t.Setenv("FLYWHEEL_DSH_BIN", publicSDK)
		t.Setenv(DSHWorkerEnv, worker)
		root := t.TempDir()
		calls := filepath.Join(root, "calls.jsonl")
		a := DSHArgs{Cwd: t.TempDir(), WorkspaceCwd: t.TempDir(), Provider: "offline-fixture", Model: "test-model", Thinking: "high",
			PermissionMode: "read-only", Prompt: "Review verified candidate through original entry",
			APIKeys:  map[string]string{"TEST_DSH_HISTORY": filepath.Join(root, "history"), "TEST_DSH_LLM_CALLS": calls},
			Watchdog: watchdog.Opts{IdleTimeoutMs: 3000, WallTimeoutMs: 10000, KillGraceMs: 5000}}
		var scoped string
		for i := 0; i < 3; i++ {
			a.PrewarmID = fmt.Sprintf("warm-readonly-%d", i)
			a.PrewarmDirectory, err = filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			require.NoError(t, os.Chmod(a.PrewarmDirectory, 0o700))
			a.PersistentSession, a.RequestID = i > 0, fmt.Sprintf("readonly-formal-%d", i)
			if i == 2 {
				a.SessionID = scoped
			}
			h := PrepareDSH(a)
			awaitWarm(t, h)
			if i == 0 {
				assert.NoFileExists(t, calls)
			}
			result := InvokeHarness(CallArgs{Prewarm: h, Prompt: a.Prompt, Cwd: a.Cwd, WorkspaceCwd: a.WorkspaceCwd,
				Model: a.Model, Binding: config.LoopBinding{Harness: "dsh", Provider: a.Provider, Model: a.Model, Thinking: a.Thinking, Channel: "offline-account"},
				PermissionMode: a.PermissionMode, PersistentSession: a.PersistentSession, SessionID: a.SessionID, RequestID: a.RequestID,
				APIKeys: a.APIKeys, Watchdog: a.Watchdog}, "unused")
			require.True(t, result.Settled, "%+v", result)
			assert.Equal(t, i == 2, result.Session.ResumeAccepted)
			if i == 1 {
				scoped = result.SessionID
			}
			if i == 2 {
				assert.Equal(t, scoped, result.SessionID)
			}
			require.NoError(t, h.Close())
		}
		raw, err := os.ReadFile(calls)
		require.NoError(t, err)
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		require.Len(t, lines, 3)
		project, err := filepath.EvalSymlinks(a.WorkspaceCwd)
		require.NoError(t, err)
		execution, err := filepath.EvalSymlinks(a.Cwd)
		require.NoError(t, err)
		for _, line := range lines {
			var call struct {
				Directories []struct {
					Header  string
					Current string
				}
			}
			require.NoError(t, json.Unmarshal([]byte(line), &call))
			require.Len(t, call.Directories, 1)
			assert.Equal(t, project, call.Directories[0].Header)
			assert.Equal(t, execution, call.Directories[0].Current)
		}
	})

	t.Run("scoped persisted public SDK history", func(t *testing.T) {
		fixture, err := filepath.Abs("../../plugins/dsh/test/fixtures/persisted-sdk.js")
		require.NoError(t, err)
		require.NoError(t, os.Chmod(fixture, 0o755))
		t.Setenv("FLYWHEEL_DSH_BIN", fixture)
		root := t.TempDir()
		calls := filepath.Join(root, "model-calls.jsonl")
		a := DSHArgs{Command: worker, Cwd: t.TempDir(), WorkspaceCwd: t.TempDir(), SessionTitle: "Flywheel · execute/M01-T01", Prompt: "FIRST_CONTRACT_MARKER",
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
		project, err := filepath.EvalSymlinks(a.WorkspaceCwd)
		require.NoError(t, err)
		execution, err := filepath.EvalSymlinks(a.Cwd)
		require.NoError(t, err)
		for _, line := range entries {
			var call struct {
				Directories []struct {
					Header  string `json:"header"`
					Current string `json:"current"`
				} `json:"directories"`
			}
			require.NoError(t, json.Unmarshal([]byte(line), &call))
			require.Len(t, call.Directories, 1)
			require.Equal(t, project, call.Directories[0].Header)
			require.Equal(t, execution, call.Directories[0].Current)
		}

		assert.Contains(t, entries[1], "FIRST_CONTRACT_MARKER")
		assert.Contains(t, entries[1], "published SDK fixture complete")
		assert.Contains(t, entries[1], "CURRENT_FAILURE_MARKER")
		a.PrewarmID = "warm-public-sdk-123"
		a.PrewarmDirectory, err = filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		require.NoError(t, os.Chmod(a.PrewarmDirectory, 0o700))
		a.RequestID, a.Prompt = "warmed-invocation", "WARMED_PUBLIC_SDK_MARKER"
		h := PrepareDSH(a)
		awaitWarm(t, h)
		beforeWarm, err := os.ReadFile(calls)
		require.NoError(t, err)
		assert.Equal(t, string(raw), string(beforeWarm), "public SDK initialize does not call the model")
		a.Prewarm = h
		warmed := RunDSH(a)
		require.True(t, warmed.Settled, "%+v", warmed)
		assert.Equal(t, first.Session.SessionID, warmed.SessionID)
		assert.True(t, warmed.Session.ResumeAccepted)
		require.NoError(t, h.Close())
		a.Prewarm = nil
		raw, err = os.ReadFile(calls)
		require.NoError(t, err)
		assert.Len(t, strings.Split(strings.TrimSpace(string(raw)), "\n"), 3)

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
