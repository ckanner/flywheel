package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaults(t *testing.T) {
	d := Default()
	assert.Equal(t, "kimi", d.Drivers["plan"])
	assert.Equal(t, "pi", d.Drivers["execute"])
	assert.Equal(t, "soft", d.Budget.Enforce)
	assert.Nil(t, d.Budget.DailyTokenLimit)
	assert.Equal(t, int64(300_000), d.Watchdog.IdleTimeoutMs)
	assert.Equal(t, int64(5_000), d.Watchdog.KillGraceMs)
	assert.Equal(t, 0.1, d.Retry.Jitter)
	assert.Equal(t, 5, d.Retry.MaxTransportRetries)
	assert.Equal(t, 5, d.DefectInjection.EveryNUnits)
	assert.True(t, *d.Worktree.Enabled, "worktree 默认开启（v0.16）")
	assert.Equal(t, int64(86_400_000), d.Credentials.TimeoutMs)
}

func TestNormalizeFillsZeros(t *testing.T) {
	var c Config
	c.Normalize()
	d := Default()
	assert.Equal(t, d.Drivers, c.Drivers)
	assert.Equal(t, "soft", c.Budget.Enforce)
	assert.Equal(t, int64(300_000), c.Watchdog.IdleTimeoutMs)
	assert.True(t, *c.Worktree.Enabled)
	assert.Equal(t, 5, c.Retry.MaxTransportRetries)
}

func TestValidate(t *testing.T) {
	good := Default()
	assert.NoError(t, good.Validate())

	v := 0
	bad := Default()
	bad.Version = &v
	assert.Error(t, bad.Validate())

	bad2 := Default()
	bad2.Budget.Enforce = "nuclear"
	assert.Error(t, bad2.Validate())

	limit := int64(0)
	bad3 := Default()
	bad3.Budget.DailyTokenLimit = &limit
	assert.Error(t, bad3.Validate())

	bad4 := Default()
	bad4.DefectInjection.EveryNUnits = 0
	assert.Error(t, bad4.Validate())
}

func TestLoadAndFind(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "docs", "task")
	assert.NoError(t, os.MkdirAll(taskDir, 0o755))

	// 缺失 → 错误。
	_, err := Load(taskDir, dir)
	assert.Error(t, err)
	assert.Equal(t, "", FindConfigFile(taskDir, dir))

	// 非法 JSON → 错误。
	assert.NoError(t, os.WriteFile(filepath.Join(taskDir, "flywheel.config.json"), []byte("{oops"), 0o644))
	_, err = Load(taskDir, dir)
	assert.Error(t, err)

	// 不可读（路径是目录）→ 读取错误分支。
	assert.NoError(t, os.Remove(filepath.Join(taskDir, "flywheel.config.json")))
	assert.NoError(t, os.MkdirAll(filepath.Join(taskDir, "flywheel.config.json"), 0o755))
	_, err = Load(taskDir, dir)
	assert.Error(t, err)
	assert.NoError(t, os.Remove(filepath.Join(taskDir, "flywheel.config.json")))

	// 合法 + 自定义覆盖。
	raw := `{
	  "version": 1,
	  "budget": {"enforce": "hard", "daily-token-limit": 1000},
	  "loops": {"plan": {"harness": "kimi", "model": "kimi-code/k3", "channel": "kimi-subscription"}},
	  "worktree": {"enabled": false},
	  "verify": {"command-timeout-ms": 70000, "env-allowlist": ["TOKEN"]},
	  "pricing": {"deepseek-v4-flash": {"input": 0.2, "output": 0.6, "cache-read": 0.02}}
	}`
	assert.NoError(t, os.WriteFile(filepath.Join(taskDir, "flywheel.config.json"), []byte(raw), 0o644))
	cfg, err := Load(taskDir, dir)
	assert.NoError(t, err)
	assert.Equal(t, "hard", cfg.Budget.Enforce)
	assert.Equal(t, int64(1000), *cfg.Budget.DailyTokenLimit)
	assert.False(t, *cfg.Worktree.Enabled)
	assert.Equal(t, int64(70_000), cfg.Verify.CommandTimeoutMs)
	assert.Equal(t, []string{"TOKEN"}, cfg.Verify.EnvAllowlist)
	// 缺省字段仍回填。
	assert.Equal(t, int64(300_000), cfg.Watchdog.IdleTimeoutMs)
	assert.Equal(t, 5, cfg.Retry.MaxTransportRetries)
	assert.Equal(t, "kimi", cfg.Drivers["plan"])
	assert.Equal(t, "pi", cfg.Drivers["execute"], "execute 默认 pi")
	assert.Equal(t, 0.2, cfg.Pricing["deepseek-v4-flash"].Input)
	assert.Equal(t, filepath.Join(taskDir, "flywheel.config.json"), cfg.File)

	// cwd 级配置兜底（taskDir 无配置文件时）。
	other := filepath.Join(dir, "other")
	assert.NoError(t, os.MkdirAll(other, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "flywheel.config.json"), []byte(`{"version": 2}`), 0o644))
	cfg2, err := Load(other, dir)
	assert.NoError(t, err)
	assert.Equal(t, 2, *cfg2.Version)

	// taskDir 不存在：FindConfigFile 全 miss → not found 错误。
	_, err = Load(filepath.Join(dir, "nope"), filepath.Join(dir, "nope2"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// JSON 合法但校验失败 → Load 报错。
	badCfg := filepath.Join(dir, "badcfg")
	assert.NoError(t, os.MkdirAll(badCfg, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(badCfg, "flywheel.config.json"), []byte(`{"budget": {"enforce": "nuclear"}}`), 0o644))
	_, err = Load(badCfg, dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "budget.enforce")
}
