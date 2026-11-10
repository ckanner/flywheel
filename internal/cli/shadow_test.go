package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/ckanner/flywheel/internal/shadow"
	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestShadowCLIIsExplicitAndReportsFailures(t *testing.T) {
	dir := t.TempDir()
	for _, pair := range [][2]string{{"spec.example.md", "spec.md"}, {"task-packet.example.md", "task-packet-M01-T01.md"}} {
		raw, err := templates.FS.ReadFile(pair[0])
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, pair[1]), raw, 0600))
	}
	code, out := runCLI(t, "shadow", "prepare", "--task-dir", dir)
	require.Zero(t, code, out)
	var data shadow.Dataset
	require.NoError(t, json.Unmarshal([]byte(out), &data))
	require.Len(t, data.Cases, 1)
	code, _ = runCLI(t, "shadow", "prepare", "--task-dir", filepath.Join(dir, "missing"))
	require.Equal(t, 1, code)
	data.Cases[0].HardBlockers = []string{}
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	input := filepath.Join(dir, "cases.json")
	require.NoError(t, os.WriteFile(input, raw, 0600))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer server.Close()
	t.Setenv("FLYWHEEL_SHADOW_TEST_KEY", "cli-test-key-123456789")
	output := filepath.Join(dir, "records")
	args := []string{"shadow", "run", "--input", input, "--directory", output, "--endpoint", server.URL + "/v1/systemone", "--key-env", "FLYWHEEL_SHADOW_TEST_KEY"}
	code, out = runCLI(t, args...)
	require.Equal(t, 3, code, out)
	require.Contains(t, out, `"errors": 1`)
	code, out = runCLI(t, "shadow", "report", "--input", input, "--directory", output)
	require.Zero(t, code, out)
	require.Contains(t, out, `"http-status": 401`)
	for _, action := range []string{"run", "report"} {
		code, _ = runCLI(t, "shadow", action, "--input", filepath.Join(dir, "missing"))
		require.Equal(t, 1, code)
		code, _ = runCLI(t, "shadow", action, "--input", input, "--directory", filepath.Join(dir, "missing-output"))
		require.Equal(t, 1, code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := newRootCmd()
	root.SetContext(ctx)
	root.SetArgs(args)
	var buffer bytes.Buffer
	root.SetOut(&buffer)
	root.SetErr(&buffer)
	err = root.Execute()
	var exit *exitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 143, exit.code)
	data.Cases[0].HardBlockers = []string{"Explicit Full"}
	raw, _ = json.Marshal(data)
	require.NoError(t, os.WriteFile(input, raw, 0600))
	code, out = runCLI(t, args...)
	require.Zero(t, code, out)
	require.Contains(t, out, `"skipped": 1`)
}
