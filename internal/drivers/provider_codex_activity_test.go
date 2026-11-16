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

// The actual installed package, guardian, durable supervisor, worker and Go
// watchdog participate. Only the Codex app-server peer is an offline fixture.
func TestCodexProProviderActivityOfflineBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("set FLYWHEEL_TEST_NODE_BRIDGE=1 after building plugins/dsh with the reviewed adapter tarball")
	}
	_, err := exec.LookPath("node")
	require.NoError(t, err)
	worker, err := filepath.Abs("../../plugins/dsh/dist/codex-pro-worker.js")
	require.NoError(t, err)
	fixture, err := filepath.Abs("../../plugins/dsh/test/fixtures/fake-codex-pro.js")
	require.NoError(t, err)
	require.NoError(t, os.Chmod(fixture, 0o755))
	t.Setenv(CodexProBinaryEnv, fixture)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "isolated-codex-home"))
	setup := func(t *testing.T, scenario string) CodexProArgs {
		cwd, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		return CodexProArgs{Command: worker, Cwd: cwd, Prompt: "private offline task", Model: "fixture-model",
			ReasoningEffort: "high", PermissionMode: "workspace-write", RequestID: "provider-activity",
			RecoveryDirectory: filepath.Join(cwd, "journal"), OnLifecycle: func(MachineLifecycle) error { return nil },
			APIKeys: map[string]string{"TEST_CODEX_PRO_SCENARIO": scenario, "TEST_CODEX_PRO_LOG": filepath.Join(cwd, "requests.jsonl"),
				"TEST_CODEX_PRO_HISTORY": filepath.Join(cwd, "history.json")},
			Watchdog: watchdog.Opts{IdleTimeoutMs: 1500, WallTimeoutMs: 10000}}
	}

	for _, scenario := range []string{"activity-root", "activity-child", "activity-default-child"} {
		t.Run(scenario+" survives real idle watchdog", func(t *testing.T) {
			a := setup(t, scenario)
			var observed strings.Builder
			a.OnEvent = func(line string) { observed.WriteString(line + "\n") }
			if scenario == "activity-child" {
				a.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true}}`)
			}
			out := RunCodexPro(a)
			require.True(t, out.Settled, "%+v", out)
			assert.False(t, out.TimedOut)
			assert.False(t, out.Cancelled)
			assert.Equal(t, "fixture complete\n", out.Text)
			require.NotNil(t, out.Session)
			assert.True(t, out.Session.CleanupConfirmed)
			activity := codexActivityFrames(t, observed.String())
			require.GreaterOrEqual(t, len(activity), 4)
			assert.Less(t, len(activity), 40, "root and children share the bounded coalescer")
			assert.Len(t, codexActivityFrames(t, out.Stdout), 1, "retained stdout keeps a bounded observation summary")
			kinds := map[string]bool{}
			childSeen := false
			for _, frame := range activity {
				for _, kind := range frame.EventTypes {
					kinds[kind] = true
				}
				childSeen = childSeen || frame.Scope == "child" || frame.Scope == "mixed"
			}
			for _, kind := range []string{"item/reasoning/summaryTextDelta", "item/reasoning/textDelta", "item/agentMessage/delta", "item/commandExecution/outputDelta"} {
				assert.True(t, kinds[kind], kind)
			}
			if scenario != "activity-root" {
				assert.True(t, childSeen)
			}
			assert.NotContains(t, out.Stdout, "PRIVATE_REASONING_AND_TOOL_CONTENT")
			entries := readCodexProLog(t, a.APIKeys["TEST_CODEX_PRO_LOG"])
			var completedEvents float64
			for _, entry := range entries {
				if entry["event"] == "activity-complete" {
					completedEvents = entry["count"].(float64)
				}
			}
			assert.Equal(t, float64(70), completedEvents, "the provider really finishes a turn longer than the idle budget")
			if scenario == "activity-default-child" {
				assert.Empty(t, out.NativeReceipt)
				assert.False(t, codexProLogHasMethod(entries, "config/read"), "observation does not request native controls")
			}
			assertCodexProProcessesStopped(t, entries)
		})
	}

	for _, scenario := range []string{"activity-raw-volume", "activity-display-volume"} {
		t.Run(scenario+" preserves small final answer", func(t *testing.T) {
			a := setup(t, scenario)
			out := RunCodexPro(a)
			require.True(t, out.Settled, "%+v", out)
			assert.False(t, out.Truncated)
			assert.Equal(t, "fixture complete\n", out.Text)
			entries := readCodexProLog(t, a.APIKeys["TEST_CODEX_PRO_LOG"])
			var rawBytes float64
			for _, entry := range entries {
				if entry["event"] == "activity-complete" {
					rawBytes = entry["rawBytes"].(float64)
				}
			}
			assert.Greater(t, rawBytes, float64(1024*1024))
			assertCodexProProcessesStopped(t, entries)
		})
	}

	for _, scenario := range []string{"activity-quiet", "activity-foreign", "activity-stale-root", "activity-completed-child"} {
		t.Run(scenario+" cannot renew real idle deadline", func(t *testing.T) {
			a := setup(t, scenario)
			out := RunCodexPro(a)
			assert.False(t, out.Settled)
			assert.True(t, out.TimedOut, "%+v", out)
			assert.True(t, out.IdleTimedOut, "%+v", out)
			assert.False(t, out.Cancelled, "the provider cancellation is watchdog cleanup, not a user cancellation")
			assert.Equal(t, errs.TIMEOUT, out.ErrorCode)
			require.NotNil(t, out.ErrorMessage)
			assert.Contains(t, *out.ErrorMessage, "idle timeout")
			var receipt codexProReceipt
			require.NoError(t, json.Unmarshal(out.CodexProReceipt, &receipt))
			execution := receipt.Failure
			if receipt.Outcome != nil {
				require.NotNil(t, receipt.Outcome.Execution.Cancelled)
				assert.True(t, *receipt.Outcome.Execution.Cancelled)
			} else {
				require.NotNil(t, execution)
				require.NotNil(t, execution.Execution.Cancelled)
				assert.True(t, *execution.Execution.Cancelled)
			}
			for _, frame := range codexActivityFrames(t, out.Stdout) {
				assert.NotContains(t, frame.EventTypes, "item/reasoning/summaryTextDelta")
				assert.NotContains(t, frame.EventTypes, "item/commandExecution/outputDelta")
			}
			assertCodexProProcessesStopped(t, readCodexProLog(t, a.APIKeys["TEST_CODEX_PRO_LOG"]))
		})
	}

	t.Run("active observations never extend the wall budget", func(t *testing.T) {
		a := setup(t, "activity-root")
		var observed strings.Builder
		a.OnEvent = func(line string) { observed.WriteString(line + "\n") }
		a.Watchdog.WallTimeoutMs = 1800
		out := RunCodexPro(a)
		assert.False(t, out.Settled)
		assert.True(t, out.TimedOut)
		assert.True(t, out.WallTimedOut)
		assert.False(t, out.IdleTimedOut)
		assert.False(t, out.Cancelled)
		assert.Equal(t, errs.TIMEOUT, out.ErrorCode)
		require.NotNil(t, out.ErrorMessage)
		assert.Contains(t, *out.ErrorMessage, "wall timeout")
		assert.GreaterOrEqual(t, len(codexActivityFrames(t, observed.String())), 3)
		assertCodexProProcessesStopped(t, readCodexProLog(t, a.APIKeys["TEST_CODEX_PRO_LOG"]))
	})

	t.Run("readable persistent title and resumed user title cross the full driver", func(t *testing.T) {
		a := setup(t, "ok")
		a.PersistentSession, a.SessionTitle = true, "Flywheel · Alerts · Design review"
		first := RunCodexPro(a)
		require.True(t, first.Settled, "%+v", first)
		historyFile := a.APIKeys["TEST_CODEX_PRO_HISTORY"]
		raw, err := os.ReadFile(historyFile)
		require.NoError(t, err)
		var history map[string]any
		require.NoError(t, json.Unmarshal(raw, &history))
		history["name"] = "Human renamed this investigation"
		raw, err = json.Marshal(history)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(historyFile, raw, 0o600))
		a.RequestID, a.SessionID, a.SessionTitle = "resumed-activity", first.SessionID, "Must not overwrite"
		a.RecoveryDirectory = filepath.Join(a.Cwd, "resumed-journal")
		second := RunCodexPro(a)
		require.True(t, second.Settled, "%+v", second)
		raw, err = os.ReadFile(historyFile)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &history))
		assert.Equal(t, "Human renamed this investigation", history["name"])
		a.RequestID, a.SessionID, a.PersistentSession = "ephemeral-activity", "", false
		a.RecoveryDirectory = filepath.Join(a.Cwd, "ephemeral-journal")
		third := RunCodexPro(a)
		require.True(t, third.Settled, "%+v", third)
		var names []string
		var ephemeral []bool
		entries := readCodexProLog(t, a.APIKeys["TEST_CODEX_PRO_LOG"])
		for _, entry := range entries {
			if entry["method"] == "thread/name/set" {
				names = append(names, entry["params"].(map[string]any)["name"].(string))
			}
			if entry["method"] == "thread/start" {
				ephemeral = append(ephemeral, entry["params"].(map[string]any)["ephemeral"].(bool))
			}
		}
		assert.Equal(t, []string{"Flywheel · Alerts · Design review"}, names)
		assert.Equal(t, []bool{false, true}, ephemeral)
		assertCodexProProcessesStopped(t, entries)
	})
}

type codexActivityObservation struct {
	Scope      string   `json:"scope"`
	EventTypes []string `json:"eventTypes"`
}

func codexActivityFrames(t *testing.T, stdout string) []codexActivityObservation {
	t.Helper()
	var observations []codexActivityObservation
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var frame struct {
			Type     string                   `json:"type"`
			Activity codexActivityObservation `json:"activity"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &frame))
		if frame.Type == "activity" {
			observations = append(observations, frame.Activity)
		}
	}
	return observations
}
