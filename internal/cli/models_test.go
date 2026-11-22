package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestControlModelsIsExplicitReadOnlyAndTaskless(t *testing.T) {
	dir := t.TempDir()
	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(`{"version":1,"harness":"kimi","channel":"kimi-code"}`))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"control", "models", "--cwd", dir})
	require.NoError(t, cmd.Execute(), output.String())
	var reply controlResponse
	require.NoError(t, json.Unmarshal(output.Bytes(), &reply))
	require.True(t, reply.OK)
	require.Nil(t, reply.State)
	require.Empty(t, reply.TaskDir)
	require.Equal(t, "unsupported", reply.ModelCatalog.Status)
	require.False(t, reply.ModelCatalog.InferencePerformed)
	require.Equal(t, "unknown", reply.ModelCatalog.AccountEntitlement)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files)
	for _, input := range []string{`null`, `[]`, `{`, `{}`, `{"version":1,"harness":"kimi","prompt":"must not run"}`, `{"version":1,"harness":"kimi","provider":null}`, strings.Repeat(" ", 4097)} {
		command := &cobra.Command{}
		command.SetIn(strings.NewReader(input))
		value, err := controlModels(command, dir)
		require.Error(t, err, input)
		require.Nil(t, value)
	}
	command := &cobra.Command{}
	command.SetIn(failingEditorReader{})
	_, err = controlModels(command, dir)
	require.ErrorContains(t, err, "read failed")
}
