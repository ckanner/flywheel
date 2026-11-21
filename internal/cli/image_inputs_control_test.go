package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestPlanningCLIImageEditRequiresExplicitNullOrFrozenReferences(t *testing.T) {
	input := `{"action":"input-images","revision":1,"request-id":"image-edit","input-images":{"packet":"M01-T01","contractFingerprint":"` + strings.Repeat("a", 64) + `","plan":{"revision":1,"digest":"` + strings.Repeat("b", 64) + `"},"value":null}}`
	request, err := readPlanningCommand(strings.NewReader(input))
	require.NoError(t, err)
	require.Equal(t, "input-images", request.Action)
	require.JSONEq(t, "null", string(request.InputImages.Value))
	for _, invalid := range []string{
		strings.Replace(input, `,"value":null`, "", 1),
		strings.Replace(input, `"value":null`, `"value":null,"bytes":"untrusted"`, 1),
		strings.Replace(input, `"value":null`, `"value":{"version":1,"images":[],"data":"untrusted"}`, 1),
		strings.Replace(input, `"action":"input-images"`, `"action":"collaboration"`, 1),
		strings.Replace(input, `"value":null`, `"value":{"version":1,"images":[]}`, 1),
		strings.Replace(input, `"value":null`, `"value":{"extra":"`+strings.Repeat("x", 33<<10)+`"}`, 1),
	} {
		_, err := readPlanningCommand(strings.NewReader(invalid))
		require.Error(t, err)
	}
	refs := `{"version":1,"images":[{"path":"references/layout.png","sha256":"` + strings.Repeat("c", 64) + `","media-type":"image/png"}]}`
	request, err = readPlanningCommand(strings.NewReader(strings.Replace(input, `"value":null`, `"value":`+refs, 1)))
	require.NoError(t, err)
	require.JSONEq(t, refs, string(request.InputImages.Value))
}

func TestPlanningCLIRejectedImageEditNeverStartsAProcessAndCanBeReconciled(t *testing.T) {
	taskDir, cwd, st := planningCLIFixture(t, true)
	previous := controlExecutable
	t.Cleanup(func() { controlExecutable = previous })
	started := false
	controlExecutable = func() (string, error) { started = true; return "", errors.New("unexpected process start") }
	request := machine.PlanningCommand{Action: "input-images", Revision: st.Revision + 1, RequestID: "stale-image-inputs", Start: new(true),
		InputImages: &machine.InputImagesEdit{Packet: "M01-T01", ContractFingerprint: strings.Repeat("a", 64), Plan: planning.Ref{Revision: 1, Digest: strings.Repeat("b", 64)}, Value: json.RawMessage("null")}}
	reply, err := planningCLIControl(t, taskDir, cwd, request)
	require.NoError(t, err, reply.Error)
	require.True(t, reply.OK)
	require.False(t, started)
	require.False(t, reply.Running)
	require.Equal(t, "input-images", reply.PlanningControl.Action)
	require.Equal(t, "rejected", reply.PlanningControl.Status)
	require.Equal(t, st.Planning, reply.State.Planning)
	read, err := planningCLIControl(t, taskDir, cwd, machine.PlanningCommand{Action: "status", RequestID: request.RequestID})
	require.NoError(t, err)
	require.Equal(t, reply.PlanningControl, read.PlanningControl)
	require.False(t, started)
}

func TestImageInputPreviewUsesProjectRootFromNestedSession(t *testing.T) {
	_, cwd, _ := planningCLIFixture(t, true)
	raw, err := os.ReadFile("../../plugins/dsh/test/fixtures/multimodal-reference.png")
	require.NoError(t, err)
	writeFileCli(t, filepath.Join(cwd, "references/layout.png"), string(raw))
	nested := filepath.Join(cwd, "src/nested-session")
	require.NoError(t, os.MkdirAll(nested, 0755))
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(`{"paths":["references/layout.png"]}`))
	preview, err := controlImages(cmd, nested)
	require.NoError(t, err)
	require.Len(t, preview.References.Images, 1)
	require.Equal(t, "references/layout.png", preview.References.Images[0].Path)
	require.Equal(t, planning.DigestBytes(raw), preview.References.Images[0].SHA256)
	require.Equal(t, "image/png", preview.References.Images[0].MediaType)
}
