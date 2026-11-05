package drivers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDSHFullAccessReachesWorkerAndRequiresMatchingReady(t *testing.T) {
	a := dshRunArgs(t, "ok")
	a.PermissionMode, a.Prompt = "danger-full-access", "record input"
	r := RunDSH(a)
	require.True(t, r.Settled, "%+v", r)
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "worker-input.json"))
	require.NoError(t, err)
	var captured struct {
		Request dshStart `json:"request"`
	}
	require.NoError(t, json.Unmarshal(raw, &captured))
	assert.Equal(t, "danger-full-access", captured.Request.PermissionMode)
	assert.Equal(t, "high", captured.Request.ReasoningEffort)
	ready, result := dshFixtureFrames(captured.Request)
	ready["permissionMode"] = "workspace-write"
	parsed := parseDSHStream(dshJSONL(ready, result), captured.Request)
	assert.False(t, parsed.Settled, "a restricted runtime must not be accepted as requested full access")
	assert.NotEmpty(t, parsed.Error)
}

func TestCodexProFullAccessUsesMachinePresetAndExactEffectivePolicy(t *testing.T) {
	a := codexProRunArgs(t, "record")
	a.PermissionMode = "danger-full-access"
	r := RunCodexPro(a)
	require.True(t, r.Settled, "%+v", r)
	var receipt map[string]any
	require.NoError(t, json.Unmarshal(r.CodexProReceipt, &receipt))
	assert.Equal(t, "dangerously-bypass-approvals-and-sandbox", proObject(receipt, "requested")["permissionMode"])
	assert.Equal(t, "danger-full-access", proObject(receipt, "outcome", "configuration", "effective")["sandbox"])
	assert.Equal(t, map[string]any{"type": "dangerFullAccess"}, proObject(receipt, "outcome", "configuration", "effective", "sandboxPolicy"))
	assert.Equal(t, "never", proObject(receipt, "outcome", "configuration", "effective")["approvalPolicy"])
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "worker-input.json"))
	require.NoError(t, err)
	var captured struct {
		Request codexProStart `json:"request"`
	}
	require.NoError(t, json.Unmarshal(raw, &captured))
	assert.Equal(t, "danger-full-access", captured.Request.PermissionMode, "the Flywheel wire keeps the canonical mode")
	for _, change := range []func(map[string]any){
		func(r map[string]any) { proObject(r, "requested")["permissionMode"] = "danger-full-access" },
		func(r map[string]any) {
			proObject(r, "outcome", "configuration", "effective")["sandbox"] = "workspace-write"
		},
		func(r map[string]any) {
			proObject(r, "outcome", "configuration", "effective")["approvalPolicy"] = "on-request"
		},
		func(r map[string]any) {
			proObject(r, "outcome", "configuration", "effective")["sandboxPolicy"] = map[string]any{"type": "readOnly", "networkAccess": false}
		},
	} {
		bad := codexProFixture(captured.Request)
		change(bad)
		parsed := parseCodexProStream(dshJSONL(codexProFrame(captured.Request, bad)), captured.Request)
		assert.False(t, parsed.Settled)
		assert.NotEmpty(t, parsed.Error)
	}
}

func TestKimiFullAccessKeepsNativePrintAutoAndUpdatedDefault(t *testing.T) {
	bin, cwd := fakeBin(t, "kimi", kimiComplete)
	setPath(t, bin)
	r := InvokeHarness(CallArgs{Prompt: "plan the task", Cwd: cwd,
		Binding: config.LoopBinding{Harness: "kimi"}, PermissionMode: "danger-full-access"}, "")
	require.True(t, r.Settled, "%+v", r)
	assert.Equal(t, "kimi-code/k3.1-pro", r.Model)
	args, err := os.ReadFile(filepath.Join(cwd, "args.txt"))
	require.NoError(t, err)
	assert.Equal(t, []string{"-m", "kimi-code/k3.1-pro", "-p", "plan the task", "--output-format", "stream-json"}, strings.Split(strings.TrimSpace(string(args)), "\n"))
	env, err := os.ReadFile(filepath.Join(cwd, "env.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(env), "KIMI_MODEL_THINKING_EFFORT=high\n")
}
