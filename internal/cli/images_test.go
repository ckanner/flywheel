package cli

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/state"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestControlImageSelectionIsBoundedReadOnlyAndTaskless(t *testing.T) {
	dir := t.TempDir()
	var raster bytes.Buffer
	require.NoError(t, png.Encode(&raster, image.NewNRGBA(image.Rect(0, 0, 4, 3))))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "selected.png"), raster.Bytes(), 0o600))
	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(`{"paths":["selected.png"]}`))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"control", "images", "--cwd", dir})
	require.NoError(t, cmd.Execute(), out.String())
	var reply controlResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &reply))
	require.True(t, reply.OK)
	require.Empty(t, reply.TaskDir)
	require.Nil(t, reply.State)
	require.Len(t, reply.InputImages.Previews, 1)
	require.Equal(t, "selected.png", reply.InputImages.References.Images[0].Path)
	require.Equal(t, 4, reply.InputImages.Previews[0].Width)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "preview does not create a task, configuration, or cache")
	for _, input := range []string{`{`, `null`, `[]`, `{}`, `{"paths":[]}`, `{"paths":["../private.png"]}`, `{"paths":["selected.png"],"write":true}`, strings.Repeat(" ", 8193)} {
		command := &cobra.Command{}
		command.SetIn(strings.NewReader(input))
		value, err := controlImages(command, dir)
		require.Error(t, err, input)
		require.Nil(t, value)
	}
	command := &cobra.Command{}
	command.SetIn(failingEditorReader{})
	_, err = controlImages(command, dir)
	require.ErrorContains(t, err, "read failed")
}

func TestInvestigationIntakeCreatesAndRunsOneNewLedger(t *testing.T) {
	controlFixture(t, "real-run")
	controlPollInterval = time.Hour
	_, cwd, contract := investigationCLIFixture(t)
	e2eConfigT(t, cwd)
	dir := filepath.Join(cwd, "docs", "new-parent", "task-from-ui")
	reply, err := investigationControl(t, dir, cwd, investigationRequest{Action: "start", Contract: json.RawMessage(mustJSONT(t, contract)), Run: new(true)})
	require.NoError(t, err, reply.Error)
	require.NotNil(t, reply.Preparation)
	require.NotEqual(t, "ready", reply.Preparation.Status, "the command starts the engine after registering the contract")
	require.False(t, state.Exists(dir), "investigation never authorizes implementation")
	if reply.Running {
		_, err = stopTask(dir)
		require.NoError(t, err)
	}
	missing := filepath.Join(cwd, "docs", "not-created-by-status")
	for _, request := range []investigationRequest{{Action: "status"}, {Action: "run", Revision: 1}, {Action: "cancel", Revision: 1}} {
		_, err := investigationControl(t, missing, cwd, request)
		require.Error(t, err)
		require.NoDirExists(t, missing)
	}
	_, err = investigationControl(t, dir, cwd, investigationRequest{Action: "status", Run: new(false)})
	require.Error(t, err)
	command := &cobra.Command{}
	command.SetIn(strings.NewReader(mustJSONT(t, investigationRequest{Action: "start", Contract: json.RawMessage(mustJSONT(t, contract))})))
	_, err = controlInvestigation(command, filepath.Join(cwd, "settings.json", "child"), cwd)
	require.Error(t, err)
}
