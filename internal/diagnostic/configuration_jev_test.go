package diagnostic

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigurationUsesCanonicalProjectScopeForJEV(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, exec.Command("git", "init", "-q", root).Run())
	project, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	subdir := filepath.Join(project, "src", "nested")
	require.NoError(t, os.MkdirAll(subdir, 0700))
	global := filepath.Join(t.TempDir(), "global.json")
	t.Setenv(config.GlobalConfigEnv, global)
	writeFixture(t, global, `{"jev":{"enabled":true,"endpoint":"https://global.example.org/v1/systemone"}}`)
	writeFixture(t, filepath.Join(project, "flywheel.config.json"), `{"jev":{"enabled":false}}`)
	t.Chdir(subdir)
	cfg, err := LoadConfigurationContext(context.Background(), "", "")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(project, "flywheel.config.json"), cfg.File)
	require.False(t, cfg.JEV.Enabled, "omitted cwd must not bypass the current project's disabled master")
	view, err := InspectContext(context.Background(), ".", "")
	require.NoError(t, err)
	require.Equal(t, cfg.JEV, view.JEV)
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"jev":`)
	require.NotContains(t, string(raw), `"jev-shadow":`)
}

func TestLoadConfigurationResolvesRelativeTaskAndDoesNotFallBackOnFailure(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, exec.Command("git", "init", "-q", root).Run())
	project, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	t.Setenv(config.GlobalConfigEnv, filepath.Join(t.TempDir(), "global.json"))
	writeFixture(t, filepath.Join(project, "flywheel.config.json"), `{"jev":{"enabled":true,"endpoint":"https://project.example.org/v1/systemone"}}`)
	task := filepath.Join(project, "docs", "task")
	writeFixture(t, filepath.Join(task, "flywheel.config.json"), `{"jev":{"enabled":false}}`)
	t.Chdir(t.TempDir())
	cfg, err := LoadConfigurationContext(context.Background(), project, "docs/task")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(task, "flywheel.config.json"), cfg.File)
	require.False(t, cfg.JEV.Enabled, "relative task paths must be anchored to the explicit cwd")
	writeFixture(t, filepath.Join(task, "flywheel.config.json"), `{"jev":{"enabled":"yes"}}`)
	cfg, err = LoadConfigurationContext(context.Background(), project, "docs/task")
	require.ErrorContains(t, err, "effective configuration is invalid")
	require.Nil(t, cfg, "an invalid task config cannot fall back to an enabled project config")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, err = LoadConfigurationContext(ctx, project, "docs/task")
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, cfg)
}
