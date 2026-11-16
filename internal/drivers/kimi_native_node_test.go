package drivers

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/require"
)

func TestKimiNativeNodeWorkerOfflineBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("enable the real Node worker boundary with FLYWHEEL_TEST_NODE_BRIDGE=1")
	}
	node, err := exec.LookPath("node")
	require.NoError(t, err)
	root, err := filepath.Abs("../../plugins/dsh")
	require.NoError(t, err)
	worker := kimiScript(t, "exec "+shQuote(node)+" "+shQuote(filepath.Join(root, "src/kimi-native-worker.js"))+"\n")
	server := kimiScript(t, "exec "+shQuote(node)+" "+shQuote(filepath.Join(root, "test/fixtures/kimi-native-server.mjs"))+" \"$@\"\n")
	t.Setenv(KimiNativeWorkerEnv, worker)
	t.Setenv(KimiCommandEnv, server)
	t.Setenv("KIMI_CODE_HOME", t.TempDir())
	cwd := t.TempDir()
	task := filepath.Join(cwd, "task")
	require.NoError(t, os.Mkdir(task, 0700))
	a := KimiArgs{Cwd: cwd, Model: "kimi-for-coding", Thinking: "high", Prompt: "Bound offline test", PermissionMode: "danger-full-access", RequestID: "offline-1", PersistentSession: true,
		Native: json.RawMessage(`{"version":1,"mode":"swarm"}`), Managed: &NativeAdmission{Version: 1, InvocationID: "offline-1", Role: "execute", Scope: "packet", Channel: "subscription", TaskDir: task},
		Watchdog: watchdog.Opts{IdleTimeoutMs: 15000, WallTimeoutMs: 20000}}
	r := RunKimi(a)
	require.True(t, r.Settled, "%+v", r)
	require.NotNil(t, r.Usage)
	require.Equal(t, int64(23), r.Usage.TotalTokens)
	require.True(t, r.Session.ResumeCandidate)
	a.RequestID, a.Managed.InvocationID, a.SessionID = "offline-2", "offline-2", r.SessionID
	r = RunKimi(a)
	require.True(t, r.Settled, "%+v", r)
	require.True(t, r.Session.ResumeAccepted)
	require.Equal(t, int64(23), r.Usage.TotalTokens)
	require.NotContains(t, r.Stdout+r.Stderr, "fixture-secret")

	for _, fixtureCase := range []string{"root-progress", "child-progress", "burst-progress"} {
		t.Run(fixtureCase, func(t *testing.T) {
			// The owned server streams real item/step progress for three seconds,
			// longer than the parent's idle window, without ending the turn.
			server := kimiScript(t, "KIMI_FIXTURE_CASE="+shQuote(fixtureCase)+" exec "+shQuote(node)+" "+shQuote(filepath.Join(root, "test/fixtures/kimi-native-server.mjs"))+" \"$@\"\n")
			t.Setenv(KimiCommandEnv, server)
			t.Setenv("KIMI_CODE_HOME", t.TempDir())
			call := a
			call.SessionID = ""
			call.Watchdog.IdleTimeoutMs = 1500
			if fixtureCase == "burst-progress" {
				// Thousands of valid tiny deltas must not consume the final
				// receipt's buffer budget merely to report ongoing activity.
				original := watchdog.MaxBuffer
				watchdog.MaxBuffer = 64 * 1024
				t.Cleanup(func() { watchdog.MaxBuffer = original })
			}
			result := RunKimi(call)
			require.True(t, result.Settled, "%+v", result)
			require.False(t, result.IdleTimedOut)
			require.False(t, result.Truncated)
			require.Contains(t, result.Stdout, `"type":"item.delta"`)
			require.NotContains(t, result.Stdout, "private-tool-progress")
			require.True(t, result.Session.CleanupConfirmed)
		})
	}
}
