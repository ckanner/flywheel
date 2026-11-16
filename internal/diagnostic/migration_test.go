package diagnostic

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationUsesInspectionDiscoveryAndShowsEffectiveBindings(t *testing.T) {
	root, _ := diagnosticFixture(t, `{"loops":{"plan":{"harness":"kimi","model":"kimi-code/k3","channel":"kimi-subscription","resident-session":false}},"routes":{"default":{"harness":"zcode","model":"zai/glm-5.3","channel":"zai-coding-plan"}}}`)
	alias := filepath.Join(t.TempDir(), "repo alias")
	require.NoError(t, os.Symlink(root, alias))
	options := config.MigrationOptions{Preset: "subscriptions", Cwd: alias}
	preview, err := MigrateContext(context.Background(), options)
	require.NoError(t, err)
	assert.True(t, preview.OK)
	assert.Equal(t, "proposed", preview.ConfigurationStatus)
	assert.Equal(t, root, preview.Cwd)
	require.NotNil(t, preview.Current)
	require.NotNil(t, preview.Configuration)
	assert.Equal(t, "kimi-code/k3", preview.Current.Bindings[0].Model)
	require.Len(t, preview.Configuration.Bindings, 11)
	for _, binding := range preview.Configuration.Bindings {
		assert.NotEqual(t, "zcode", binding.Harness)
		assert.Equal(t, "danger-full-access", binding.PermissionMode)
		if binding.ID == "loops.intake" || binding.ID == "routes.default" {
			assert.Equal(t, "deepseek-account", binding.Provider)
			assert.Equal(t, "deepseek-account", binding.Channel)
			assert.Equal(t, "deepseek-flash", binding.Model)
		}
	}
	options.Apply, options.ExpectSHA256 = true, preview.SHA256
	applied, err := MigrateContext(context.Background(), options)
	require.NoError(t, err)
	assert.True(t, applied.Applied)
	assert.Equal(t, "effective", applied.ConfigurationStatus)
	actual, err := InspectContext(context.Background(), root, "")
	require.NoError(t, err)
	assert.Equal(t, actual, applied.Configuration)
	assert.NotEmpty(t, applied.Backup)
}

func TestMigrationReportsUnconfirmedConfigurationWhenPostWriteInspectionFails(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	calls := 0
	runtime.command = func(call commandCall) watchdog.Result {
		calls++
		if calls > 1 {
			return watchdog.Result{TimedOut: true}
		}
		return defaultCommand(t, root, call)
	}
	report, err := MigrateContext(context.Background(), config.MigrationOptions{Preset: "subscriptions", Cwd: root, Apply: true})
	require.Error(t, err)
	assert.True(t, report.Applied)
	assert.False(t, report.OK)
	assert.Equal(t, "unconfirmed", report.ConfigurationStatus)
	assert.Nil(t, report.Configuration)
	assert.FileExists(t, report.Backup)
	actual, err := config.LoadReadOnly("", root)
	require.NoError(t, err)
	assert.Equal(t, "full-access", actual.Permissions.Mode)
}

func TestMigrationDiagnosticTaskScopeAndFailures(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	task := filepath.Join(root, "task")
	require.NoError(t, os.Mkdir(task, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(task, "flywheel.config.json"), []byte(`{"routes":{"custom":{"harness":"kimi"}}}`), 0o600))
	options := config.MigrationOptions{Preset: "subscriptions", Cwd: root, TaskDir: "task", Apply: true}
	blocked, err := MigrateContext(context.Background(), options)
	require.NoError(t, err)
	assert.False(t, blocked.OK)
	assert.Equal(t, task, blocked.TaskDir)
	assert.Equal(t, "task", blocked.Scope)
	require.Len(t, blocked.Blockers, 1)
	assert.False(t, blocked.Applied)
	options.TaskDir = filepath.Join(root, "missing")
	failed, err := MigrateContext(context.Background(), options)
	assert.ErrorContains(t, err, "resolve task directory")
	assert.False(t, failed.OK)
	assert.NotEmpty(t, failed.Error)
	options.TaskDir = ""
	options.Cwd = filepath.Join(root, "missing")
	_, err = MigrateContext(context.Background(), options)
	assert.ErrorContains(t, err, "resolve working directory")
	options.Cwd = root
	options.Preset = "unknown"
	failed, err = MigrateContext(context.Background(), options)
	assert.ErrorContains(t, err, "--preset")
	assert.False(t, failed.OK)
	options.Preset = "subscriptions"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = MigrateContext(ctx, options)
	assert.ErrorIs(t, err, context.Canceled)
	runtime.command = func(commandCall) watchdog.Result { return watchdog.Result{TimedOut: true} }
	_, err = MigrateContext(context.Background(), options)
	assert.ErrorContains(t, err, "discovery was interrupted")
}
