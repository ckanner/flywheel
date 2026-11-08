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
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This opt-in bridge uses the built worker and the real, installed 0.9.0 public
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
	testMachineLifecycleNodeBridge(t, "codex-pro")

	t.Run("scoped history crosses processes with independent invocation and run delta", func(t *testing.T) {
		cwd := t.TempDir()
		log := filepath.Join(cwd, "scoped.jsonl")
		a := CodexProArgs{Command: worker, Cwd: cwd, Prompt: "offline scoped first attempt",
			Model: "gpt-test", ReasoningEffort: "high", PermissionMode: "workspace-write",
			PersistentSession: true, RequestID: "owned-attempt-one",
			APIKeys:  map[string]string{"TEST_CODEX_PRO_LOG": log, "TEST_CODEX_PRO_HISTORY": filepath.Join(cwd, "history.json")},
			Watchdog: watchdog.Opts{IdleTimeoutMs: 5000, WallTimeoutMs: 10000}}
		first := RunCodexPro(a)
		require.True(t, first.Settled, "%+v", first)
		require.NotNil(t, first.Session)
		assert.True(t, first.Session.ResumeCandidate)
		assert.False(t, first.Session.ResumeAccepted)
		assert.Equal(t, "completed", first.Session.Outcome)
		assert.Equal(t, a.RequestID, first.Session.RequestID)
		a.SessionID, a.RequestID, a.Prompt = first.SessionID, "owned-attempt-two", "continue the same scoped task"
		second := RunCodexPro(a)
		require.True(t, second.Settled, "%+v", second)
		require.NotNil(t, second.Session)
		assert.True(t, second.Session.ResumeCandidate)
		assert.True(t, second.Session.ResumeAccepted)
		assert.True(t, second.Session.CleanupConfirmed)
		assert.Equal(t, "dispatched", second.Session.Dispatch)
		assert.Equal(t, first.SessionID, second.SessionID)
		assert.NotEqual(t, first.Session.RunID, second.Session.RunID)
		assert.NotEqual(t, first.Session.TurnID, second.Session.TurnID)
		assert.Equal(t, "complete", second.Session.UsageStatus)
		require.NotNil(t, second.Usage)
		assert.Equal(t, int64(108), second.Usage.TotalTokens)
		assert.Nil(t, second.CostUSD)
		var receipt codexProReceipt
		require.NoError(t, json.Unmarshal(second.CodexProReceipt, &receipt))
		require.NotNil(t, receipt.Outcome)
		assert.Equal(t, "thread/resume", receipt.Outcome.Configuration.Source)
		var usage struct {
			ThreadTotal struct {
				TotalTokens int64 `json:"totalTokens"`
			} `json:"threadTotal"`
		}
		require.NoError(t, json.Unmarshal(receipt.Outcome.Usage, &usage))
		assert.Equal(t, int64(216), usage.ThreadTotal.TotalTokens)
		entries := readCodexProLog(t, log)
		starts, resumes, turns := 0, 0, 0
		for _, entry := range entries {
			switch entry["method"] {
			case "thread/start":
				starts++
				assert.Equal(t, false, entry["params"].(map[string]any)["ephemeral"])
			case "thread/resume":
				resumes++
				assert.Equal(t, first.SessionID, entry["params"].(map[string]any)["threadId"])
			case "turn/start":
				turns++
			}
		}
		assert.Equal(t, 1, starts)
		assert.Equal(t, 1, resumes)
		assert.Equal(t, 2, turns)
		assertCodexProProcessesStopped(t, entries)
		a.RequestID = "owned-attempt-three"
		a.APIKeys["TEST_CODEX_PRO_SCENARIO"] = "resume-no-baseline"
		partial := RunCodexPro(a)
		require.True(t, partial.Settled, "%+v", partial)
		assert.Equal(t, "partial", partial.Session.UsageStatus)
		assert.Nil(t, partial.Usage)
		assert.True(t, partial.Session.ResumeCandidate)
		a.RequestID = "owned-attempt-four"
		a.APIKeys["TEST_CODEX_PRO_SCENARIO"] = "resume-busy"
		busy := RunCodexPro(a)
		assert.False(t, busy.Settled)
		assert.Equal(t, "unknown", busy.Session.Outcome)
		assert.Equal(t, "unknown", busy.Session.Dispatch)
		assert.False(t, busy.Session.ResumeCandidate)
		assert.True(t, busy.Session.CleanupConfirmed)
		entries = readCodexProLog(t, log)
		turns = 0
		for _, entry := range entries {
			if entry["method"] == "turn/start" {
				turns++
			}
		}
		assert.Equal(t, 3, turns, "resume refusal cannot silently dispatch a fresh turn")
		assertCodexProProcessesStopped(t, entries)
	})

	t.Run("full-access public machine mapping", func(t *testing.T) {
		log := filepath.Join(t.TempDir(), "full-access.jsonl")
		r := RunCodexPro(CodexProArgs{Command: worker, Cwd: t.TempDir(), Prompt: "offline full-access verification",
			Model: "gpt-test", ReasoningEffort: "high", PermissionMode: "danger-full-access",
			APIKeys: map[string]string{"TEST_CODEX_PRO_LOG": log}, Watchdog: watchdog.Opts{IdleTimeoutMs: 5000, WallTimeoutMs: 10000}})
		require.True(t, r.Settled, "%+v", r)
		var receipt codexProReceipt
		require.NoError(t, json.Unmarshal(r.CodexProReceipt, &receipt))
		assert.Equal(t, "dangerously-bypass-approvals-and-sandbox", receipt.Requested.PermissionMode)
		require.NotNil(t, receipt.Outcome)
		assert.Equal(t, "danger-full-access", receipt.Outcome.Configuration.Effective.Sandbox)
		assert.JSONEq(t, `{"type":"dangerFullAccess"}`, string(receipt.Outcome.Configuration.Effective.SandboxPolicy))
		entries := readCodexProLog(t, log)
		assert.True(t, codexProLogHasMethod(entries, "turn/start"))
		for _, entry := range entries {
			if entry["method"] == "thread/start" {
				params := entry["params"].(map[string]any)
				assert.Equal(t, "danger-full-access", params["sandbox"])
				assert.Equal(t, "never", params["approvalPolicy"])
			}
		}
		assertCodexProProcessesStopped(t, entries)
	})

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
			if strings.HasPrefix(scenario, "wrong-") {
				assert.Equal(t, "not-dispatched", r.Session.Dispatch)
				require.NotNil(t, r.Usage)
				assert.Equal(t, Usage{}, *r.Usage)
				require.NotNil(t, r.CostUSD)
				assert.Zero(t, *r.CostUSD, "a proven pre-turn configuration rejection has no model charge")
			} else {
				assert.Nil(t, r.CostUSD, "a dispatched or unproven call has no known subscription cost")
			}
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
				assert.True(t, *receipt.OK, "legacy protocol 1 has no durable callback; protocol 3 checks full roots before ACK")
				assert.Contains(t, *r.ErrorMessage, "writable root escapes")
			}
			assertCodexProProcessesStopped(t, entries)
		})
	}
	t.Run("read-only", func(t *testing.T) {
		log := filepath.Join(t.TempDir(), "readonly.jsonl")
		r := RunCodexPro(CodexProArgs{Command: worker, Cwd: t.TempDir(), Prompt: "offline readonly verification",
			Model: "gpt-test", ReasoningEffort: "high", PermissionMode: "read-only", RequestID: "call-freshEngine123",
			APIKeys: map[string]string{"TEST_CODEX_PRO_LOG": log}, Watchdog: watchdog.Opts{IdleTimeoutMs: 5000, WallTimeoutMs: 10000}})
		require.True(t, r.Settled, "%+v", r)
		require.NotNil(t, r.Session)
		assert.Equal(t, "codex-pro/0.9.0/worker-v1", r.Session.Protocol)
		assert.Equal(t, "call-freshEngine123", r.Session.RequestID)
		assert.Equal(t, "completed", r.Session.Outcome)
		assert.False(t, r.Session.ResumeCandidate)
		var receipt codexProReceipt
		require.NoError(t, json.Unmarshal(r.CodexProReceipt, &receipt))
		assert.Equal(t, "call-freshEngine123", receipt.RequestID)
		assert.JSONEq(t, "null", string(receipt.Requested.ThreadID))
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
	t.Run("worker SIGKILL leaves cleanup unknown until independent confirmation", func(t *testing.T) {
		log := filepath.Join(t.TempDir(), "worker-death.jsonl")
		pidFile := filepath.Join(t.TempDir(), "worker.pid")
		wrapper := kimiScript(t, "echo $$ > "+shQuote(pidFile)+"\nexec "+shQuote(worker)+"\n")
		killed := make(chan error, 1)
		var once sync.Once
		r := RunCodexPro(CodexProArgs{Command: wrapper, Cwd: t.TempDir(), Prompt: "offline worker lifetime verification",
			Model: "gpt-test", PermissionMode: "workspace-write",
			APIKeys:  map[string]string{"TEST_CODEX_PRO_SCENARIO": "stubborn", "TEST_CODEX_PRO_LOG": log},
			Watchdog: watchdog.Opts{IdleTimeoutMs: 10000, WallTimeoutMs: 20000},
			OnEvent: func(line string) {
				if strings.Contains(line, `"type":"progress"`) {
					once.Do(func() {
						raw, err := os.ReadFile(pidFile)
						if err != nil {
							killed <- err
							return
						}
						pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
						if err != nil {
							killed <- err
							return
						}
						killed <- syscall.Kill(pid, syscall.SIGKILL)
					})
				}
			},
		})
		select {
		case err := <-killed:
			require.NoError(t, err)
		default:
			t.Fatal("worker did not reach the running-turn kill point")
		}
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
		assert.False(t, r.Settled)
		assert.False(t, r.Cancelled, "unexpected death is distinct from caller cancellation")
		assert.NotEmpty(t, r.Signal)
		assert.Empty(t, r.CodexProReceipt)
		assert.Contains(t, r.CleanupError, "no trustworthy cleanup receipt",
			"worker exit alone must not permit a retry while the separate guardian is cleaning")
		assertCodexProProcessesStopped(t, entries)
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
