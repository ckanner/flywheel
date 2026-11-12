package machine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceRootSeparatesGitCheckoutFromProject(t *testing.T) {
	fx := newGitFixture(t)
	unit := filepath.Join(t.TempDir(), "unit")
	gitCmd(t, fx.root, "worktree", "add", "-b", "workspace-unit", unit)
	require.Equal(t, fx.root, WorkspaceRoot(unit))
	require.Equal(t, fx.root, WorkspaceRoot(fx.root))
	nonGit := t.TempDir()
	require.Equal(t, nonGit, WorkspaceRoot(nonGit))
	require.NoError(t, os.WriteFile(filepath.Join(nonGit, ".git"), []byte("invalid linked metadata"), 0600))
	require.Equal(t, nonGit, WorkspaceRoot(nonGit))
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	for _, output := range []string{"bare", "worktree /missing-flywheel-project"} {
		require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nprintf '%s\\n' '"+output+"'\n"), 0700))
		require.Equal(t, nonGit, WorkspaceRoot(nonGit))
	}
}

func TestWorkspaceSessionIdentityAndPromptSurviveReconciliation(t *testing.T) {
	fx, c := sessionFixture(t)
	c.WorkspaceCwd = fx.root
	a := sessionArgs(c)
	a.Prompt = "implement packet"
	result, err := c.invokeScoped("execute", "M01-T01", a, func(call drivers.CallArgs) drivers.Result {
		require.Equal(t, fx.root, call.WorkspaceCwd)
		require.Equal(t, fx.root, call.Cwd)
		require.Contains(t, call.Prompt, "实际执行目录")
		require.Contains(t, call.Prompt, "implement packet")
		require.Contains(t, call.SessionTitle, "execute/M01-T01")
		return sessionResult(call, drivers.Result{Settled: true})
	})
	require.NoError(t, err)
	require.Equal(t, fx.root, c.State.Sessions.Records["execute/M01-T01"].WorkspaceCwd)
	_, err = c.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
		t.Fatal("a persisted result must not redispatch")
		return drivers.Result{}
	})
	require.NoError(t, err)
	applySessionResult(t, c, "execute", "M01-T01", result)
	require.Equal(t, "", workspacePrompt("", "", "", ""))
}

func TestNativeSessionDirectoryDoesNotReplaceTaskEvidenceDirectory(t *testing.T) {
	r := &state.SessionRecord{Cwd: "/task", WorkspaceCwd: "/project"}
	require.Equal(t, "/task", sessionNativeCwd(r))
	r.Configuration = &state.SessionConfiguration{Harness: "dsh"}
	require.Equal(t, "/task", sessionNativeCwd(r))
	r.Configuration.Harness = "codex-pro"
	require.Equal(t, "/project", sessionNativeCwd(r))
	r.WorkspaceCwd = ""
	require.Equal(t, "/task", sessionNativeCwd(r))
}

func TestLegacyProjectSessionReconcilesAndResumesWithoutRelabelingHistory(t *testing.T) {
	fx, c := sessionFixture(t)
	c.WorkspaceCwd = ""
	a := sessionArgs(c)
	first, err := c.invokeScoped("execute", "M01-T01", a, func(call drivers.CallArgs) drivers.Result {
		return sessionResult(call, drivers.Result{Settled: true})
	})
	require.NoError(t, err)
	c.WorkspaceCwd = fx.root
	_, err = c.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
		t.Fatal("legacy received result must reconcile without dispatch")
		return drivers.Result{}
	})
	require.NoError(t, err)
	applySessionResult(t, c, "execute", "M01-T01", first)
	_, err = c.invokeScoped("execute", "M01-T01", a, func(call drivers.CallArgs) drivers.Result {
		require.Equal(t, first.SessionID, call.SessionID)
		require.Empty(t, call.WorkspaceCwd, "legacy identity remains unchanged")
		require.Contains(t, call.Prompt, "项目工作区")
		return sessionResult(call, drivers.Result{Settled: true})
	})
	require.NoError(t, err)
}
