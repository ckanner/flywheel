package drivers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/stretchr/testify/require"
)

func TestImagesCannotBeSilentlyDroppedByOtherHarnesses(t *testing.T) {
	for _, harness := range []string{"pi", "omp", "codex", "codex-pro", "dsh", "kimi", "antigravity"} {
		t.Run(harness, func(t *testing.T) {
			out := InvokeHarness(CallArgs{RequestID: "image-rejected", Binding: config.LoopBinding{Harness: harness}, Images: []imageinput.Image{{MIMEType: "image/png", Data: []byte("image")}}}, "")
			require.False(t, out.Settled)
			require.NotNil(t, out.ErrorMessage)
			require.Contains(t, *out.ErrorMessage, "image")
			require.Empty(t, out.SpawnError)
			if config.SupportsScopedSessions(harness) {
				require.NotNil(t, out.Session)
				require.Equal(t, "not-dispatched", out.Session.Dispatch)
			}
		})
	}
}

func TestClaudeReceiptBindsActualImageBytesAndOrder(t *testing.T) {
	r := claudeCodeProRequest(t)
	r.Images = []imageinput.Image{{MIMEType: "image/png", Data: []byte("first-image")}, {MIMEType: "image/jpeg", Data: []byte("second-image")}}
	r.InputImageManifest = imageinput.Describe(r.Images)
	parse := func(receipt map[string]any) claudeCodeProParsed {
		return parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, receipt)), r)
	}
	require.True(t, parse(claudeCodeProFixture(r)).Settled)
	for _, invalid := range []*imageinput.Manifest{
		nil,
		{Version: 2, Images: r.InputImageManifest.Images},
		imageinput.Describe([]imageinput.Image{{MIMEType: "image/png", Data: []byte("changed-image")}, r.Images[1]}),
		imageinput.Describe([]imageinput.Image{r.Images[1], r.Images[0]}),
		imageinput.Describe(r.Images[:1]),
	} {
		receipt := claudeCodeProFixture(r)
		proObject(receipt, "requested")["inputImages"] = invalid
		out := parse(receipt)
		require.False(t, out.Settled)
		require.Contains(t, out.Error, "image manifest mismatch")
	}
	receipt := claudeCodeProFixture(r)
	r.Images, r.InputImageManifest = nil, nil
	require.Contains(t, parse(receipt).Error, "image manifest mismatch", "an unrequested attachment cannot be accepted")
}

func TestRecoveryImageManifestRemainsLocalAndMustMatchOriginal(t *testing.T) {
	a := MachineRecoveryArgs{Harness: "claude-code-pro", RequestID: "image-recover", RunID: "original-run", Cwd: t.TempDir(), RecoveryDirectory: t.TempDir(), Model: "sonnet", ReasoningEffort: "high", PermissionMode: "danger-full-access"}
	// The original request records a physical cwd, not a temporary-root alias.
	cwd, err := filepath.EvalSymlinks(a.Cwd)
	require.NoError(t, err)
	a.Cwd = cwd
	a.InputImages = imageinput.Describe([]imageinput.Image{{MIMEType: "image/png", Data: []byte("original-image")}})
	command := kimiScript(t, "FLYWHEEL_TEST_MACHINE_RECOVERY=claude-code-pro/images exec "+shQuote(os.Args[0])+" -test.run '^TestMachineRecoveryProcessFixture$'\n")
	t.Setenv(ClaudeCodeProWorkerEnv, command)
	result, err := RecoverMachine(a)
	require.NoError(t, err)
	require.True(t, result.Settled)
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "recovery-query.json"))
	require.NoError(t, err)
	for _, forbidden := range []string{"prompt", "images", "inputImages", "original-image", a.InputImages.Images[0].SHA256} {
		require.NotContains(t, string(raw), forbidden, "recovery is only a read-only identity query")
	}
	a.InputImages = imageinput.Describe([]imageinput.Image{{MIMEType: "image/png", Data: []byte("new-image")}})
	_, err = RecoverMachine(a)
	require.Error(t, err, "a mismatched original manifest cannot adopt a recovered receipt")
	a.InputImages.Version = 2
	_, err = RecoverMachine(a)
	require.ErrorContains(t, err, "image")
	a.InputImages.Version, a.Harness = 1, "codex-pro"
	_, err = RecoverMachine(a)
	require.ErrorContains(t, err, "image")
}
