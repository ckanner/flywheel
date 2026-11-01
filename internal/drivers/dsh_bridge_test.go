package drivers

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
}
