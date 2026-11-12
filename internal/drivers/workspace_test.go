package drivers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNativeWorkspaceDispatchKeepsProjectSessionHome(t *testing.T) {
	project, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	unit := t.TempDir()
	command := kimiScript(t, "pwd > session-project\nprintf '%s\\n' "+shQuote(kimiComplete)+"\n")
	t.Setenv(KimiCommandEnv, command)
	r := InvokeHarness(CallArgs{Cwd: unit, WorkspaceCwd: project, Prompt: "work in the explicit task checkout", PermissionMode: "danger-full-access", Binding: config.LoopBinding{Harness: "kimi"}}, "")
	require.True(t, r.Settled, "%+v", r)
	pwd, err := os.ReadFile(filepath.Join(project, "session-project"))
	require.NoError(t, err)
	require.Equal(t, project+"\n", string(pwd))
	require.NoFileExists(t, filepath.Join(unit, "session-project"))
	for _, harness := range []string{"kimi", "dsh", "codex-pro", "claude-code-pro"} {
		r := InvokeHarness(CallArgs{Cwd: unit, WorkspaceCwd: project, PermissionMode: "workspace-write", Binding: config.LoopBinding{Harness: harness}}, "")
		require.NotNil(t, r.ErrorMessage)
		require.Contains(t, *r.ErrorMessage, "cannot separate")
	}
}

func TestDSHWorkspaceAndExecutionDirectoriesBothReachWorker(t *testing.T) {
	a := dshRunArgs(t, "ok")
	project, projectErr := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, projectErr)
	t.Setenv(DSHWorkerEnv, a.Command)
	r := InvokeHarness(CallArgs{Cwd: a.Cwd, WorkspaceCwd: project, SessionTitle: "Flywheel · execute/M01-T01", Prompt: "record input",
		PermissionMode: "danger-full-access", Model: "test", Binding: config.LoopBinding{Harness: "dsh", Provider: "test"}, APIKeys: a.APIKeys}, "")
	// The deliberately old protocol fixture cannot attest the new workspace
	// identity. Refuse its ready frame, but inspect the exact outgoing request.
	require.False(t, r.Settled)
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "worker-input.json"))
	require.NoError(t, err)
	var captured struct {
		Request dshStart `json:"request"`
	}
	require.NoError(t, json.Unmarshal(raw, &captured))
	require.Equal(t, project, captured.Request.WorkspaceCwd)
	physical, err := filepath.EvalSymlinks(a.Cwd)
	require.NoError(t, err)
	require.Equal(t, physical, captured.Request.Cwd)
	require.Equal(t, "Flywheel · execute/M01-T01", captured.Request.SessionTitle)
	ready, terminal := dshFixtureFrames(captured.Request)
	ready["workspaceCwd"] = project
	require.True(t, parseDSHStream(dshJSONL(ready, terminal), captured.Request).Settled)
	a.WorkspaceCwd = filepath.Join(project, "missing")
	bad := RunDSH(a)
	require.NotNil(t, bad.ErrorMessage)
	require.Contains(t, *bad.ErrorMessage, "project workspace cannot be resolved")
}
