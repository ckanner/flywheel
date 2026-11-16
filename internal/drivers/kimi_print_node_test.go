package drivers

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/require"
)

func TestKimiPrintSessionTitleOfflineBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("enable the real Node worker boundary with FLYWHEEL_TEST_NODE_BRIDGE=1")
	}
	node, err := exec.LookPath("node")
	require.NoError(t, err)
	root, err := filepath.Abs("../../plugins/dsh")
	require.NoError(t, err)
	cwd, home := t.TempDir(), t.TempDir()
	log := filepath.Join(cwd, "requests.jsonl")
	worker := kimiScript(t, "exec "+shQuote(node)+" "+shQuote(filepath.Join(root, "dist/kimi-native-worker.js"))+"\n")
	server := kimiScript(t, "KIMI_FIXTURE_LOG="+shQuote(log)+" exec "+shQuote(node)+" "+shQuote(filepath.Join(root, "test/fixtures/kimi-native-server.mjs"))+" \"$@\"\n")
	t.Setenv(KimiNativeWorkerEnv, worker)
	t.Setenv(KimiCommandEnv, server)
	t.Setenv("KIMI_CODE_HOME", home)
	a := CallArgs{Cwd: cwd, Prompt: "BUSINESS_PROMPT_MUST_ONLY_REACH_PRINT", RequestID: "print-one", PersistentSession: true,
		SessionTitle: "Flywheel · design-plan · 排查重复告警", Binding: config.LoopBinding{Harness: "kimi", Model: "kimi-for-coding", Thinking: "high"},
		Watchdog: watchdog.Opts{IdleTimeoutMs: 50, WallTimeoutMs: 15000}}
	first := InvokeHarness(a, "")
	require.True(t, first.Settled, "%+v", first)
	require.False(t, first.Session.ResumeAccepted)
	require.True(t, first.Session.CleanupConfirmed)
	require.False(t, first.TimedOut, "quiet metadata preparation is not an inference stall")
	a.RequestID, a.SessionID, a.SessionTitle = "print-two", first.SessionID, "must not rename"
	second := InvokeHarness(a, "")
	require.True(t, second.Settled, "%+v", second)
	require.True(t, second.Session.ResumeAccepted)
	raw, err := os.ReadFile(log)
	require.NoError(t, err)
	creates, prints, shutdowns := 0, 0, 0
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["method"] == "POST" && record["path"] == "/api/v1/sessions" {
			creates++
			body := record["body"].(map[string]any)
			require.Equal(t, "Flywheel · design-plan · 排查重复告警", body["title"])
			require.NotContains(t, string(line), a.Prompt)
			require.NotContains(t, body["agent_config"].(map[string]any), "thinking", "naming cannot override a disabled-thinking profile")
		}
		if record["path"] == "/api/v1/shutdown" {
			shutdowns++
		}
		if record["type"] == "print" {
			prints++
			require.Equal(t, 1, shutdowns, "the preparation owner must close before business inference")
			require.Equal(t, first.SessionID, record["sessionId"])
			require.Contains(t, line, a.Prompt)
		}
		require.NotEqual(t, "/api/v1/sessions/"+first.SessionID+"/prompts", record["path"])
	}
	require.Equal(t, 1, creates)
	require.Equal(t, 2, prints)
}
