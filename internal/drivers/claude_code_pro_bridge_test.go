package drivers

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise Go -> the built Node bridge -> the actual pinned public plugin ->
// a fake native CLI. No model, subscription request or private plugin import is
// involved. Enabling the bridge requires the optional peer to be installed.
func TestClaudeCodeProNodeWorkerOfflineBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("set FLYWHEEL_TEST_NODE_BRIDGE=1 after building plugins/dsh and installing its pinned optional Claude peer")
	}
	_, err := exec.LookPath("node")
	require.NoError(t, err)
	worker, err := filepath.Abs("../../plugins/dsh/dist/claude-code-pro-worker.js")
	require.NoError(t, err)
	source, err := os.ReadFile("../../plugins/dsh/test/fixtures/fake-claude-code-pro.js")
	require.NoError(t, err)
	// Never change the shared source file's permissions: Node tests may be
	// rebuilding or copying the same fixture in a separate process.
	fixture := filepath.Join(t.TempDir(), "fake-claude.mjs")
	require.NoError(t, os.WriteFile(fixture, source, 0o755))
	t.Setenv(ClaudeCodeProBinaryEnv, fixture)
	t.Setenv("ANTHROPIC_API_KEY", "ambient-key-must-not-leak")
	t.Setenv("DSH_SECRET", "other-backend-secret-must-not-leak")
	testMachineLifecycleNodeBridge(t, "claude-code-pro")
	args := func(t *testing.T, scenario string) ClaudeCodeProArgs {
		t.Helper()
		cwd := t.TempDir()
		raw, err := json.Marshal(map[string]string{"scenario": scenario})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(cwd, "claude-fixture.json"), raw, 0o600))
		return ClaudeCodeProArgs{Command: worker, Cwd: cwd, Prompt: "offline boundary verification", Model: "sonnet",
			ReasoningEffort: "max", PermissionMode: "danger-full-access", RequestID: "owned-" + scenario,
			Watchdog: watchdog.Opts{IdleTimeoutMs: 5000, WallTimeoutMs: 10000}}
	}
	t.Run("native child receipt is preserved without adding task snapshots to root usage", func(t *testing.T) {
		a := args(t, "native-child")
		// Resolve the fixture before deriving the private protocol 3 journal path.
		cwd, err := filepath.EvalSymlinks(a.Cwd)
		require.NoError(t, err)
		a.Cwd = cwd
		a.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true,"maxConcurrent":3,"definitions":{"scout":{"description":"Find relevant code","prompt":"Inspect the target","model":"sonnet","effort":"low"},"auditor":{"description":"Check edge cases","prompt":"Report defects","effort":"xhigh"}}},"workflows":{"enabled":true}}`)
		a.Managed = &NativeAdmission{Version: 1, RunID: "run", InvocationID: a.RequestID, Role: "Executor", Scope: "candidate", Channel: "local", TaskDir: a.Cwd}
		a.RecoveryDirectory, a.OnLifecycle = filepath.Join(a.Cwd, "journal"), func(MachineLifecycle) error { return nil }
		out := RunClaudeCodePro(a)
		require.True(t, out.Settled, "%+v", out)
		assert.Nil(t, out.Usage)
		assert.Equal(t, "unknown", out.Session.UsageStatus)
		assert.True(t, out.Session.CleanupConfirmed)
		assert.Contains(t, out.Stdout, `"type":"native-event"`)
		var native proNativeReceipt
		require.NoError(t, json.Unmarshal(out.NativeReceipt, &native))
		assert.True(t, proJSONEqual(a.Native, native.Requested))
		require.Len(t, *native.Children, 1)
		var receipt claudeCodeProReceipt
		require.NoError(t, json.Unmarshal(out.ClaudeCodeProReceipt, &receipt))
		root, _, err := parseClaudeCodeProUsage(receipt.Usage)
		require.NoError(t, err)
		assert.Equal(t, int64(22), root.TotalTokens)
		log := readCodexProLog(t, filepath.Join(a.Cwd, "claude-fixture.jsonl"))
		require.Len(t, log, 1)
		argv := log[0]["argv"].([]any)
		values := map[string]string{}
		for i, value := range argv[:len(argv)-1] {
			if value == "--agents" || value == "--effort" {
				values[value.(string)] = argv[i+1].(string)
			}
		}
		assert.Equal(t, "max", values["--effort"], "the parent effort must remain independent")
		assert.JSONEq(t, `{"scout":{"description":"Find relevant code","prompt":"Inspect the target","model":"sonnet","effort":"low"},"auditor":{"description":"Check edge cases","prompt":"Report defects","effort":"xhigh"}}`, values["--agents"])
	})

	t.Run("fresh calls keep native permission and cost semantics", func(t *testing.T) {
		a := args(t, "ok")
		r := RunClaudeCodePro(a)
		require.True(t, r.Settled, "%+v", r)
		assert.Equal(t, "echo:"+a.Prompt, r.Text)
		assert.Equal(t, "claude-code-pro/"+ClaudeCodeProPackageVersion+"/worker-v1", r.Session.Protocol)
		assert.Equal(t, "completed", r.Session.Outcome)
		assert.Equal(t, "dispatched", r.Session.Dispatch)
		assert.True(t, r.Session.CleanupConfirmed)
		assert.False(t, r.Session.ResumeCandidate)
		assert.Empty(t, r.Session.TurnID, "native public protocol has no idempotent turn identity")
		assert.Empty(t, r.Provider)
		assert.Nil(t, r.CostUSD)
		assert.Equal(t, &Usage{Input: 17, Output: 5, CacheRead: 4, CacheWrite: 3, TotalTokens: 22}, r.Usage)
		var receipt claudeCodeProReceipt
		require.NoError(t, json.Unmarshal(r.ClaudeCodeProReceipt, &receipt))
		assert.Equal(t, "bypassPermissions", receipt.Requested.PermissionMode)
		assert.Equal(t, "ephemeral", receipt.Requested.SessionMode)
		assert.Equal(t, "max", receipt.Requested.Effort)
		assert.Equal(t, []string{"user", "project", "local"}, receipt.Requested.SettingSources)
		assert.Equal(t, 0.12, *receipt.ReportedCostUSD)
		assert.Equal(t, "unknown", receipt.CostStatus)
		log := readCodexProLog(t, filepath.Join(a.Cwd, "claude-fixture.jsonl"))
		require.Len(t, log, 1)
		assert.Contains(t, log[0]["argv"], "--no-session-persistence")
		assert.Contains(t, log[0]["argv"], "--effort")
		assert.Contains(t, log[0]["argv"], "max")
		assert.Equal(t, false, log[0]["credentialPresent"])
		assert.Equal(t, false, log[0]["leakedDSH"])
		assertClaudeCodeProStopped(t, log)
	})

	t.Run("scoped history resumes through a new process with per-call usage", func(t *testing.T) {
		a := args(t, "ok")
		a.PersistentSession, a.RequestID, a.Prompt = true, "owned-first", "first scoped prompt"
		a.APIKeys = map[string]string{"ANTHROPIC_API_KEY": "explicit-test-key"}
		first := RunClaudeCodePro(a)
		require.True(t, first.Settled, "%+v", first)
		assert.True(t, first.Session.ResumeCandidate)
		assert.False(t, first.Session.ResumeAccepted)
		a.SessionID, a.RequestID, a.Prompt = first.SessionID, "owned-second", "current repair constraints"
		second := RunClaudeCodePro(a)
		require.True(t, second.Settled, "%+v", second)
		assert.Equal(t, "claude-code-pro/"+ClaudeCodeProPackageVersion+"/worker-v2", second.Session.Protocol)
		assert.Equal(t, first.SessionID, second.SessionID)
		assert.True(t, second.Session.ResumeAccepted)
		assert.True(t, second.Session.ResumeCandidate)
		assert.NotEqual(t, first.Session.RunID, second.Session.RunID)
		assert.NotEqual(t, first.Session.RequestID, second.Session.RequestID)
		assert.Equal(t, "prior:first scoped prompt|echo:current repair constraints", second.Text)
		assert.Equal(t, first.Usage, second.Usage, "a history replay is not cumulative billing")
		log := readCodexProLog(t, filepath.Join(a.Cwd, "claude-fixture.jsonl"))
		require.Len(t, log, 2)
		assert.NotEqual(t, log[0]["pid"], log[1]["pid"])
		assert.NotContains(t, log[0]["argv"], "--no-session-persistence")
		assert.Contains(t, log[1]["argv"], "--resume")
		assert.Contains(t, log[1]["argv"], first.SessionID)
		assert.Equal(t, true, log[1]["credentialPresent"])
		assertClaudeCodeProStopped(t, log)
		require.NoError(t, os.WriteFile(filepath.Join(a.Cwd, "claude-fixture.json"), []byte(`{"scenario":"resume-missing"}`), 0o600))
		a.RequestID = "owned-third"
		refused := RunClaudeCodePro(a)
		assert.False(t, refused.Settled)
		assert.False(t, refused.Session.ResumeCandidate)
		assert.Equal(t, "unknown", refused.Session.Outcome)
		assert.True(t, refused.Session.CleanupConfirmed)
		log = readCodexProLog(t, filepath.Join(a.Cwd, "claude-fixture.jsonl"))
		require.Len(t, log, 3, "a refused resume cannot silently start a new history")
		assert.Contains(t, log[2]["argv"], "--resume")
		assertClaudeCodeProStopped(t, log)
	})

	for _, scenario := range []string{"unknown-usage", "partial-usage", "wrong-model", "wrong-cwd", "wrong-permission", "wrong-session", "missing-result", "duplicate-result", "malformed-after-terminal", "abnormal-exit"} {
		t.Run(scenario, func(t *testing.T) {
			a := args(t, scenario)
			r := RunClaudeCodePro(a)
			valid := scenario == "unknown-usage" || scenario == "partial-usage"
			require.Equal(t, valid, r.Settled, "%+v", r)
			require.NotEmpty(t, r.ClaudeCodeProReceipt)
			assert.Nil(t, r.CostUSD)
			assert.Nil(t, r.Usage)
			assert.False(t, r.Session.ResumeCandidate)
			assert.True(t, r.Session.CleanupConfirmed)
			if valid {
				assert.Equal(t, strings.TrimSuffix(scenario, "-usage"), r.Session.UsageStatus)
			} else {
				assert.Equal(t, "unknown", r.Session.Outcome)
			}
			assertClaudeCodeProStopped(t, readCodexProLog(t, filepath.Join(a.Cwd, "claude-fixture.jsonl")))
		})
	}

	t.Run("missing native binary proves no dispatch and preserves requested history", func(t *testing.T) {
		a := args(t, "ok")
		a.PersistentSession, a.SessionID = true, "44444444-4444-4444-8444-444444444444"
		t.Setenv(ClaudeCodeProBinaryEnv, filepath.Join(t.TempDir(), "missing-native-cli"))
		r := RunClaudeCodePro(a)
		assert.False(t, r.Settled)
		assert.Equal(t, "not-dispatched", r.Session.Dispatch)
		assert.Equal(t, "failed", r.Session.Outcome)
		assert.Equal(t, a.SessionID, r.Session.SessionID)
		assert.False(t, r.Session.ResumeAccepted)
		assert.True(t, r.Session.CleanupConfirmed)
		assert.Equal(t, &Usage{}, r.Usage)
		require.NotNil(t, r.CostUSD)
		assert.Zero(t, *r.CostUSD)
		assert.NoFileExists(t, filepath.Join(a.Cwd, "claude-fixture.jsonl"))
	})

	for _, kind := range []string{"cancel", "timeout"} {
		t.Run(kind+" drains the real native process", func(t *testing.T) {
			a := args(t, "hang")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a.Watchdog.Context = ctx
			if kind == "timeout" {
				a.Watchdog.WallTimeoutMs = 1500
			} else {
				a.OnEvent = func(line string) {
					if strings.Contains(line, "fixture running") {
						cancel()
					}
				}
			}
			r := RunClaudeCodePro(a)
			assert.False(t, r.Settled)
			assert.Equal(t, "unknown", r.Session.Outcome)
			assert.True(t, r.Session.CleanupConfirmed)
			assert.NotEmpty(t, r.ClaudeCodeProReceipt)
			if kind == "timeout" {
				assert.True(t, r.TimedOut)
			} else {
				assert.True(t, r.Cancelled)
			}
			assertClaudeCodeProStopped(t, readCodexProLog(t, filepath.Join(a.Cwd, "claude-fixture.jsonl")))
		})
	}
}

func assertClaudeCodeProStopped(t *testing.T, entries []map[string]any) {
	t.Helper()
	for _, entry := range entries {
		pid := int(entry["pid"].(float64))
		require.Eventually(t, func() bool { return codexProProcessInactive(pid) }, 10*time.Second, 20*time.Millisecond,
			"native Claude process %d survived its invocation", pid)
	}
}
