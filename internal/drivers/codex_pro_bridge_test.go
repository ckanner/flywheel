package drivers

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This opt-in bridge uses the built worker and the real, installed 0.8.0 public
// machine API/guardian. Only the app-server peer is fake; no model is contacted.
func TestCodexProNodeWorkerOfflineBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("set FLYWHEEL_TEST_NODE_BRIDGE=1 after building plugins/dsh")
	}
	_, err := exec.LookPath("node")
	require.NoError(t, err)
	worker, err := filepath.Abs("../../plugins/dsh/dist/codex-pro-worker.js")
	require.NoError(t, err)
	fixture, err := filepath.Abs("../../plugins/dsh/test/fixtures/fake-codex-pro.js")
	require.NoError(t, err)
	require.NoError(t, os.Chmod(fixture, 0o755))
	t.Setenv(CodexProBinaryEnv, fixture)

	for _, scenario := range []string{"ok", "unknown-usage", "partial-usage", "wrong-model", "wrong-cwd", "wrong-approval", "wrong-sandbox", "broad-policy", "malformed", "output-limit"} {
		t.Run(scenario, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "app-server.jsonl")
			r := RunCodexPro(CodexProArgs{Command: worker, Cwd: t.TempDir(), Prompt: "offline public API boundary verification",
				Model: "gpt-test", ReasoningEffort: "xhigh", PermissionMode: "workspace-write",
				APIKeys:  map[string]string{"TEST_CODEX_PRO_SCENARIO": scenario, "TEST_CODEX_PRO_LOG": log},
				Watchdog: watchdog.Opts{IdleTimeoutMs: 5000, WallTimeoutMs: 10000}})
			valid := scenario == "ok" || scenario == "unknown-usage" || scenario == "partial-usage"
			require.Equal(t, valid, r.Settled, "%+v", r)
			require.NotEmpty(t, r.CodexProReceipt, "the real machine receipt survives both success and failure")
			assert.Empty(t, r.Provider)
			assert.Nil(t, r.CostUSD)
			var receipt codexProReceipt
			require.NoError(t, json.Unmarshal(r.CodexProReceipt, &receipt))
			if valid {
				assert.Equal(t, "fixture complete\n", r.Text)
				require.NotNil(t, receipt.Outcome)
				require.NotNil(t, receipt.Outcome.Cleanup)
				assert.True(t, receipt.Outcome.Cleanup.Confirmed)
				assert.Equal(t, "turn/start-request", receipt.Outcome.TurnConfiguration.Evidence)
				assert.JSONEq(t, "null", string(receipt.Outcome.TurnConfiguration.EffectiveEffort))
			}
			if scenario == "ok" {
				require.NotNil(t, r.Usage)
				assert.Equal(t, Usage{Input: 100, Output: 8, Reasoning: 3, CacheRead: 25, TotalTokens: 108}, *r.Usage)
			} else if scenario == "partial-usage" || scenario == "unknown-usage" {
				assert.Nil(t, r.Usage)
				assert.Contains(t, string(receipt.Outcome.Usage), strings.TrimSuffix(scenario, "-usage"))
			}
			entries := readCodexProLog(t, log)
			if strings.HasPrefix(scenario, "wrong-") {
				assert.False(t, codexProLogHasMethod(entries, "turn/start"), "startup configuration mismatch must stop before inference")
			} else {
				assert.True(t, codexProLogHasMethod(entries, "turn/start"))
			}
			if scenario == "broad-policy" {
				require.NotNil(t, receipt.OK)
				assert.True(t, *receipt.OK, "public v0.8 checks mode before turn; Flywheel rejects broad roots after it")
				assert.Contains(t, *r.ErrorMessage, "writable root escapes")
			}
			assertCodexProProcessesStopped(t, entries)
		})
	}
	t.Run("read-only", func(t *testing.T) {
		log := filepath.Join(t.TempDir(), "readonly.jsonl")
		r := RunCodexPro(CodexProArgs{Command: worker, Cwd: t.TempDir(), Prompt: "offline readonly verification",
			Model: "gpt-test", ReasoningEffort: "high", PermissionMode: "read-only",
			APIKeys: map[string]string{"TEST_CODEX_PRO_LOG": log}, Watchdog: watchdog.Opts{IdleTimeoutMs: 5000, WallTimeoutMs: 10000}})
		require.True(t, r.Settled, "%+v", r)
		entries := readCodexProLog(t, log)
		for _, entry := range entries {
			if entry["method"] == "thread/start" {
				params := entry["params"].(map[string]any)
				assert.Equal(t, "read-only", params["sandbox"])
				assert.Equal(t, "never", params["approvalPolicy"])
				assert.Equal(t, true, params["ephemeral"])
			}
			if entry["method"] == "turn/start" {
				assert.Equal(t, "high", entry["params"].(map[string]any)["effort"])
			}
		}
	})
	for _, scenario := range []string{"initialize-hang", "hang"} {
		t.Run("timeout/"+scenario, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "timeout.jsonl")
			r := RunCodexPro(CodexProArgs{Command: worker, Cwd: t.TempDir(), Prompt: "offline timeout verification", Model: "gpt-test",
				PermissionMode: "workspace-write", APIKeys: map[string]string{"TEST_CODEX_PRO_SCENARIO": scenario, "TEST_CODEX_PRO_LOG": log},
				Watchdog: watchdog.Opts{IdleTimeoutMs: 5000, WallTimeoutMs: 1000}})
			assert.False(t, r.Settled)
			assert.True(t, r.TimedOut)
			require.NotEmpty(t, r.CodexProReceipt)
			assertCodexProProcessesStopped(t, readCodexProLog(t, log))
		})
	}
	t.Run("caller cancellation awaits real guardian", func(t *testing.T) {
		log := filepath.Join(t.TempDir(), "cancel.jsonl")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := RunCodexPro(CodexProArgs{Command: worker, Cwd: t.TempDir(), Prompt: "offline cancellation verification", Model: "gpt-test",
			PermissionMode: "workspace-write", APIKeys: map[string]string{"TEST_CODEX_PRO_SCENARIO": "hang", "TEST_CODEX_PRO_LOG": log},
			OnEvent: func(line string) {
				if strings.Contains(line, `"type":"progress"`) {
					cancel()
				}
			},
			Watchdog: watchdog.Opts{Context: ctx, IdleTimeoutMs: 5000, WallTimeoutMs: 10000}})
		assert.True(t, r.Cancelled)
		assert.False(t, r.Settled)
		require.NotEmpty(t, r.CodexProReceipt)
		assertCodexProProcessesStopped(t, readCodexProLog(t, log))
	})
	t.Run("Go SIGKILL closes lifetime pipe and stops owned descendants", func(t *testing.T) {
		log := filepath.Join(t.TempDir(), "parent-death.jsonl")
		workerPIDFile := filepath.Join(t.TempDir(), "worker.pid")
		wrapper := kimiScript(t, "echo $$ > "+shQuote(workerPIDFile)+"\nexec "+shQuote(worker)+"\n")
		cmd := exec.Command(os.Args[0], "-test.run", "^TestCodexProOrchestratorProcessFixture$")
		cmd.Env = append(os.Environ(), "FLYWHEEL_TEST_CODEX_PRO_PARENT=1", "FLYWHEEL_TEST_CODEX_PRO_WORKER="+wrapper,
			"FLYWHEEL_TEST_CODEX_PRO_CWD="+t.TempDir(), "TEST_CODEX_PRO_LOG="+log)
		require.NoError(t, cmd.Start())
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		require.Eventually(t, func() bool {
			data, _ := os.ReadFile(log)
			return bytes.Contains(data, []byte(`"method":"turn/start"`)) && bytes.Contains(data, []byte(`"event":"descendant"`))
		}, 8*time.Second, 20*time.Millisecond, "fixture never started")
		pidRaw, err := os.ReadFile(workerPIDFile)
		require.NoError(t, err)
		workerPID, err := strconv.Atoi(strings.TrimSpace(string(pidRaw)))
		require.NoError(t, err)
		require.NoError(t, cmd.Process.Kill())
		require.Error(t, cmd.Wait(), "the orchestrator was killed, not gracefully cancelled")
		entries := readCodexProLog(t, log)
		for _, entry := range entries {
			if entry["event"] == "spawn" {
				supervisorPID := int(entry["supervisorPid"].(float64))
				t.Cleanup(func() {
					if !codexProProcessInactive(supervisorPID) {
						_ = syscall.Kill(-supervisorPID, syscall.SIGKILL)
					}
				})
			}
		}
		t.Cleanup(func() {
			if !codexProProcessInactive(workerPID) {
				_ = syscall.Kill(-workerPID, syscall.SIGKILL)
			}
		})
		assertCodexProProcessesStopped(t, entries)
		require.Eventually(t, func() bool { return codexProProcessInactive(workerPID) }, 10*time.Second, 20*time.Millisecond, "worker survived Go parent death")
	})
}

// The bridge kills this whole Go caller; only stdin EOF can notify its worker.
func TestCodexProOrchestratorProcessFixture(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_CODEX_PRO_PARENT") != "1" {
		return
	}
	RunCodexPro(CodexProArgs{Command: os.Getenv("FLYWHEEL_TEST_CODEX_PRO_WORKER"), Cwd: os.Getenv("FLYWHEEL_TEST_CODEX_PRO_CWD"),
		Prompt: "offline parent lifetime verification", Model: "gpt-test", PermissionMode: "workspace-write",
		APIKeys:  map[string]string{"TEST_CODEX_PRO_SCENARIO": "descendant", "TEST_CODEX_PRO_LOG": os.Getenv("TEST_CODEX_PRO_LOG")},
		Watchdog: watchdog.Opts{IdleTimeoutMs: 30000, WallTimeoutMs: 30000}})
}

func readCodexProLog(t *testing.T, path string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		entries = append(entries, entry)
	}
	return entries
}

func codexProLogHasMethod(entries []map[string]any, method string) bool {
	for _, entry := range entries {
		if entry["method"] == method {
			return true
		}
	}
	return false
}

func assertCodexProProcessesStopped(t *testing.T, entries []map[string]any) {
	t.Helper()
	for _, entry := range entries {
		for _, key := range []string{"pid", "supervisorPid"} {
			if value, ok := entry[key].(float64); ok {
				pid := int(value)
				require.Eventually(t, func() bool { return syscall.Kill(pid, 0) == syscall.ESRCH }, 10*time.Second, 20*time.Millisecond, "owned %s %d still exists", key, pid)
			}
		}
	}
}

func codexProProcessInactive(pid int) bool {
	if syscall.Kill(pid, 0) == syscall.ESRCH {
		return true
	}
	// An orphan can be a zombie until the test container's init reaps it. It
	// cannot execute or retain an open pipe, so distinguish it from a live leak.
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}
