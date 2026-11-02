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
