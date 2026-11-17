package drivers

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The public plugin, both of its supervisors, the built Node worker and Go's
// watchdog all participate. The only fake component is the native CLI itself.
func TestClaudeCodeProProviderActivityOfflineBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("set FLYWHEEL_TEST_NODE_BRIDGE=1 with the built worker and pinned Claude 0.5.3 candidate installed")
	}
	_, err := exec.LookPath("node")
	require.NoError(t, err)
	worker, err := filepath.Abs("../../plugins/dsh/dist/claude-code-pro-worker.js")
	require.NoError(t, err)
	source, err := os.ReadFile("../../plugins/dsh/test/fixtures/fake-claude-code-pro.js")
	require.NoError(t, err)
	fixture := filepath.Join(t.TempDir(), "claude-offline.mjs")
	require.NoError(t, os.WriteFile(fixture, source, 0o755))
	t.Setenv(ClaudeCodeProBinaryEnv, fixture)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	args := func(t *testing.T, scenario string, durationMS int) ClaudeCodeProArgs {
		t.Helper()
		cwd, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		settings, err := json.Marshal(map[string]any{"scenario": scenario, "durationMs": durationMS, "intervalMs": 60})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(cwd, "claude-fixture.json"), settings, 0o600))
		return ClaudeCodeProArgs{Command: worker, Cwd: cwd, Prompt: "Verify real provider activity", Model: "sonnet",
			ReasoningEffort: "high", PermissionMode: "danger-full-access", RequestID: "activity-" + scenario,
			RecoveryDirectory: filepath.Join(cwd, "journal"), OnLifecycle: func(MachineLifecycle) error { return nil },
			Watchdog: watchdog.Opts{IdleTimeoutMs: 2000, WallTimeoutMs: 10000}}
	}
	for _, scenario := range []string{"activity-root-tools", "activity-child-thinking", "activity-native-progress"} {
		t.Run(scenario+" lasts beyond idle without root answer text", func(t *testing.T) {
			a := args(t, scenario, 4500)
			var observed strings.Builder
			a.OnEvent = func(line string) { observed.WriteString(line + "\n") }
			if scenario == "activity-native-progress" {
				a.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true}}`)
			}
			out := RunClaudeCodePro(a)
			require.True(t, out.Settled, "%+v", out)
			assert.False(t, out.TimedOut)
			assert.False(t, out.Cancelled)
			assert.False(t, out.Truncated)
			assert.True(t, out.Session.CleanupConfirmed)
			assert.Equal(t, "completed", out.Session.Outcome)
			assert.Equal(t, "echo:"+a.Prompt, out.Text)
			assert.Greater(t, out.DurationMS, int64(2*a.Watchdog.IdleTimeoutMs))
			frames := providerClaudeActivityFrames(t, observed.String())
			require.GreaterOrEqual(t, len(frames), 4)
			assert.Less(t, len(frames), 50, "activity remains coalesced rather than mirroring every native event")
			kind := "tool-result"
			if scenario == "activity-child-thinking" {
				kind = "stream-delta"
			} else if scenario == "activity-native-progress" {
				kind = "task-progress"
			}
			observations := 0
			for _, frame := range frames {
				for _, event := range frame.Activity.EventTypes {
					if event == kind {
						observations++
					}
				}
			}
			assert.GreaterOrEqual(t, observations, 3, "later real activity must survive durable state deduplication")
			assert.NotContains(t, out.Stdout, "PRIVATE_PROVIDER_BODY")
			assert.NotContains(t, observed.String(), "PRIVATE_PROVIDER_BODY")
			assert.Len(t, providerClaudeActivityFrames(t, out.Stdout), 1, "capture keeps only the latest advisory observation")
			log := readCodexProLog(t, filepath.Join(a.Cwd, "claude-fixture.jsonl"))
			require.Len(t, log, 1)
			assert.Contains(t, log[0]["argv"], "--include-partial-messages")
			assert.Contains(t, log[0]["argv"], "--forward-subagent-text")
			assert.Greater(t, len(readCodexProLog(t, filepath.Join(a.Cwd, "claude-fixture-activity.jsonl"))), 30)
			if scenario == "activity-child-thinking" {
				assert.Empty(t, out.NativeReceipt, "child activity also works with no explicit native profile")
			}
			assertClaudeCodeProStopped(t, log)
		})
	}
	t.Run("true silence remains idle timeout rather than user cancellation", func(t *testing.T) {
		a := args(t, "hang", 0)
		var observed strings.Builder
		a.OnEvent = func(line string) { observed.WriteString(line + "\n") }
		out := RunClaudeCodePro(a)
		assert.False(t, out.Settled)
		assert.True(t, out.TimedOut)
		assert.True(t, out.IdleTimedOut)
		assert.False(t, out.WallTimedOut)
		assert.False(t, out.Cancelled, "the adapter cancellation receipt records cleanup mechanics, not the initiating reason")
		assert.Equal(t, errs.TIMEOUT, out.ErrorCode)
		require.NotNil(t, out.ErrorMessage)
		assert.Contains(t, *out.ErrorMessage, "idle timeout")
		assert.True(t, out.Session.CleanupConfirmed)
		assert.Equal(t, "unknown", out.Session.Outcome)
		assert.Empty(t, out.CleanupError)
		require.Len(t, providerClaudeActivityFrames(t, observed.String()), 1, "the plugin must not generate periodic heartbeats")
		var receipt claudeCodeProReceipt
		require.NoError(t, json.Unmarshal(out.ClaudeCodeProReceipt, &receipt))
		require.NotNil(t, receipt.Execution.Cancelled)
		assert.True(t, *receipt.Execution.Cancelled)
		assertClaudeCodeProStopped(t, readCodexProLog(t, filepath.Join(a.Cwd, "claude-fixture.jsonl")))
	})
	t.Run("active child still obeys the total invocation deadline", func(t *testing.T) {
		a := args(t, "activity-child-thinking", 5000)
		var observed strings.Builder
		a.OnEvent = func(line string) { observed.WriteString(line + "\n") }
		a.Watchdog.WallTimeoutMs = 2600
		out := RunClaudeCodePro(a)
		assert.False(t, out.Settled)
		assert.True(t, out.TimedOut)
		assert.True(t, out.WallTimedOut)
		assert.False(t, out.IdleTimedOut)
		assert.False(t, out.Cancelled)
		assert.Equal(t, errs.TIMEOUT, out.ErrorCode)
		assert.True(t, out.Session.CleanupConfirmed)
		assert.GreaterOrEqual(t, len(providerClaudeActivityFrames(t, observed.String())), 2)
		assertClaudeCodeProStopped(t, readCodexProLog(t, filepath.Join(a.Cwd, "claude-fixture.jsonl")))
	})
	t.Run("fresh title is forwarded and resume preserves a user name", func(t *testing.T) {
		a := args(t, "ok", 0)
		a.PersistentSession, a.SessionTitle = true, "Flywheel · Alerts · Planner"
		first := RunClaudeCodePro(a)
		require.True(t, first.Settled, "%+v", first)
		history := filepath.Join(a.Cwd, "claude-history-"+first.SessionID+".json")
		stored := map[string]any{"prompt": a.Prompt, "name": "User edited Claude title"}
		data, err := json.Marshal(stored)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(history, data, 0o600))
		a.SessionID, a.RequestID, a.SessionTitle = first.SessionID, "resume-activity-name", "Do not replace user title"
		a.RecoveryDirectory = filepath.Join(a.Cwd, "resume-journal")
		resumed := RunClaudeCodePro(a)
		require.True(t, resumed.Settled, "%+v", resumed)
		data, err = os.ReadFile(history)
		require.NoError(t, err)
		var updated map[string]any
		require.NoError(t, json.Unmarshal(data, &updated))
		assert.Equal(t, stored["name"], updated["name"])
		log := readCodexProLog(t, filepath.Join(a.Cwd, "claude-fixture.jsonl"))
		require.Len(t, log, 2)
		assert.Contains(t, log[0]["argv"], "Flywheel · Alerts · Planner")
		assert.NotContains(t, log[1]["argv"], "-n")
		assert.Contains(t, log[1]["argv"], "--resume")
		assertClaudeCodeProStopped(t, log)
	})
}

type providerClaudeActivityFrame struct {
	Type     string `json:"type"`
	Activity struct {
		EventTypes []string `json:"eventTypes"`
	} `json:"activity"`
}

func providerClaudeActivityFrames(t *testing.T, raw string) []providerClaudeActivityFrame {
	t.Helper()
	var frames []providerClaudeActivityFrame
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		var frame providerClaudeActivityFrame
		require.NoError(t, json.Unmarshal([]byte(line), &frame))
		if frame.Type == "activity" {
			frames = append(frames, frame)
		}
	}
	return frames
}
