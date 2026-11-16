package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/diagnostic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runMigrationCLI(t *testing.T, args ...string) (int, diagnostic.MigrationReport) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runRoot(append([]string{"config", "migrate", "--json"}, args...), &stdout, &stderr)
	assert.Empty(t, stderr.String(), "checked migration errors belong in its single JSON report")
	var report diagnostic.MigrationReport
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &report), stdout.String())
	require.NotNil(t, report.Migration)
	assert.Equal(t, 1, report.Protocol)
	assert.Equal(t, "config-migration", report.Kind)
	return code, report
}

func TestConfigMigrateCLICompletesPreviewApplyInspectAndIdempotentUpgrade(t *testing.T) {
	root, taskDir := diagnosticCLIFixture(t)
	path := filepath.Join(taskDir, "flywheel.config.json")
	original := []byte(`{
  "_note_user":"preserve my acceptance policy",
  "loops":{"plan":{"harness":"kimi","model":"kimi-code/k3","channel":"kimi-subscription","thinking":"max","resident-session":false}},
  "routes":{"default":{"harness":"zcode","model":"zai/glm-5.3","channel":"zai-coding-plan","thinking":"max"}},
  "channels":{"kimi-subscription":{"env-key":null,"fallback":"zai-coding-plan"},"zai-coding-plan":{"harness":"zcode","env-key":null}},
  "budget":{"enforce":"hard","daily-token-limit":3000000},"worktree":{"enabled":false}
}`)
	require.NoError(t, os.WriteFile(path, original, 0o600))
	args := []string{"--preset", "subscriptions", "--cwd", root, "--task-dir", "docs/clitask"}
	code, preview := runMigrationCLI(t, args...)
	require.Zero(t, code, preview.Error)
	assert.True(t, preview.OK)
	assert.True(t, preview.Changed)
	assert.False(t, preview.Applied)
	assert.Equal(t, "proposed", preview.ConfigurationStatus)
	assert.Equal(t, path, preview.Source)
	assert.Equal(t, "task", preview.Scope)
	assert.Equal(t, taskDir, preview.TaskDir)
	assert.NotEmpty(t, preview.Current.Bindings)
	require.Len(t, preview.Configuration.Bindings, 11)
	assert.Equal(t, original, configFileBytesCLI(t, path))
	code, human := runCLI(t, append([]string{"config", "migrate"}, args...)...)
	require.Zero(t, code, human)
	assert.Contains(t, human, "仅预览，尚未写入")
	assert.Contains(t, human, "拟议配置（尚未应用）")
	assert.Contains(t, human, "loops.plan.model")
	assert.Contains(t, human, "deepseek-flash")
	assert.Contains(t, human, "deepseek-account")
	assert.Contains(t, human, "下次引擎启动")
	applyArgs := append(append([]string{}, args...), "--apply", "--expect-sha256", strings.ToUpper(preview.SHA256))
	code, applied := runMigrationCLI(t, applyArgs...)
	require.Zero(t, code, applied.Error)
	assert.True(t, applied.Applied)
	assert.Equal(t, "effective", applied.ConfigurationStatus)
	assert.Equal(t, original, configFileBytesCLI(t, applied.Backup))
	var stdout, stderr bytes.Buffer
	code = runRoot([]string{"config", "inspect", "--cwd", root, "--task-dir", taskDir, "--json"}, &stdout, &stderr)
	require.Zero(t, code, stderr.String())
	var actual diagnostic.Configuration
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
	assert.Equal(t, applied.Configuration, &actual)
	for _, binding := range actual.Bindings {
		assert.Equal(t, "danger-full-access", binding.PermissionMode)
		assert.NotEqual(t, "zcode", binding.Harness)
		if binding.ID == "loops.intake" || binding.ID == "routes.default" {
			assert.Equal(t, "deepseek-account", binding.Provider)
			assert.Equal(t, "deepseek-account", binding.Channel)
			assert.Equal(t, "deepseek-flash", binding.Model)
		}
		if binding.ID == "loops.plan" {
			assert.Equal(t, "kimi-code/k3.1-pro", binding.Model)
			assert.Equal(t, "high", binding.Thinking)
			assert.False(t, binding.ResidentSession)
		}
	}
	assert.Contains(t, string(configFileBytesCLI(t, path)), "preserve my acceptance policy")
	assert.NoFileExists(t, filepath.Join(taskDir, "state.json"))
	assert.NoFileExists(t, filepath.Join(taskDir, "orchestrator.log"))
	code, repeated := runMigrationCLI(t, append(args, "--apply")...)
	require.Zero(t, code, repeated.Error)
	assert.False(t, repeated.Changed)
	assert.Empty(t, repeated.Backup)
	code, human = runCLI(t, append([]string{"config", "migrate", "--apply"}, args...)...)
	require.Zero(t, code, human)
	assert.Contains(t, human, "无需修改或创建备份")
	code, stale := runMigrationCLI(t, applyArgs...)
	assert.Equal(t, 1, code)
	assert.Contains(t, stale.Error, "changed since preview")
	assert.False(t, stale.Applied)
	// Human apply output exposes the concrete backup and effective new bindings.
	require.NoError(t, os.WriteFile(path, original, 0o600))
	code, human = runCLI(t, append([]string{"config", "migrate", "--apply"}, args...)...)
	require.Zero(t, code, human)
	assert.Contains(t, human, "原始文件备份:")
	assert.Contains(t, human, "配置已应用")
	assert.Contains(t, human, "重新读取的实际配置")
}

func configFileBytesCLI(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}

func TestConfigMigrateCLIReportsCustomBindingAndInvalidRequestWithoutWriting(t *testing.T) {
	root, taskDir := diagnosticCLIFixture(t)
	path := filepath.Join(taskDir, "flywheel.config.json")
	raw := []byte(`{"loops":{"custom":{"harness":"kimi"}},"routes":{"team-route":{"harness":"dsh"}}}`)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	args := []string{"--preset", "subscriptions", "--cwd", root, "--task-dir", taskDir, "--apply"}
	code, blocked := runMigrationCLI(t, args...)
	assert.Equal(t, 1, code)
	assert.False(t, blocked.OK)
	require.Len(t, blocked.Blockers, 2)
	assert.False(t, blocked.Applied)
	assert.Equal(t, raw, configFileBytesCLI(t, path))
	code, human := runCLI(t, append([]string{"config", "migrate"}, args...)...)
	assert.Equal(t, 1, code)
	assert.Contains(t, human, "阻断:")
	assert.Contains(t, human, "team-route")
	code, invalid := runMigrationCLI(t, "--preset", "unsupported", "--cwd", root)
	assert.Equal(t, 1, code)
	assert.Contains(t, invalid.Error, "--preset must be")
	code, human = runCLI(t, "config", "migrate", "--preset", "subscriptions", "--cwd", filepath.Join(root, "missing"))
	assert.Equal(t, 1, code)
	assert.Contains(t, human, "迁移失败:")
	assert.Equal(t, raw, configFileBytesCLI(t, path))
	// Failed application must label the proposed models, never present them as
	// the already effective config. The exact original file remains readable.
	require.NoError(t, os.WriteFile(path, []byte(`{"loops":{"plan":{"harness":"kimi","model":"kimi-code/k3","channel":"kimi-subscription"}}}`), 0o600))
	require.NoError(t, os.Chmod(path, 0o400))
	code, failed := runMigrationCLI(t, args...)
	assert.Equal(t, 1, code)
	assert.False(t, failed.Applied)
	assert.Equal(t, "proposed", failed.ConfigurationStatus)
	code, human = runCLI(t, append([]string{"config", "migrate"}, args...)...)
	assert.Equal(t, 1, code)
	assert.Contains(t, human, "拟议配置（尚未应用）")
	assert.NotContains(t, human, "重新读取的实际配置")
}
