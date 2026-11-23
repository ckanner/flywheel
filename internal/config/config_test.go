package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	d := Default()
	assert.Equal(t, "real", d.Drivers["plan"])
	assert.Equal(t, "real", d.Drivers["execute"])
	assert.Equal(t, "off", d.Budget.Enforce)
	assert.Nil(t, d.Budget.DailyTokenLimit)
	assert.Equal(t, 3, *d.Guardrails.MaxPacketAttempts)
	assert.Equal(t, 2, *d.Guardrails.MaxReviewRounds)
	assert.Equal(t, 60, *d.Guardrails.MaxTaskCalls)
	assert.Equal(t, "local", d.Delivery.Endpoint)
	assert.False(t, d.FrontendUXReview.Enabled)
	assert.Equal(t, int64(300_000), d.Watchdog.IdleTimeoutMs)
	assert.Equal(t, int64(5_000), d.Watchdog.KillGraceMs)
	assert.Equal(t, 0.1, *d.Retry.Jitter)
	assert.Equal(t, 5, d.Retry.MaxTransportRetries)
	assert.Equal(t, 5, *d.DefectInjection.EveryNUnits)
	assert.True(t, *d.Worktree.Enabled, "worktree 默认开启（v0.16）")
	assert.Equal(t, int64(86_400_000), d.Credentials.TimeoutMs)
}

func TestNormalizeFillsZeros(t *testing.T) {
	var c Config
	c.Normalize()
	d := Default()
	assert.Equal(t, d.Drivers, c.Drivers)
	assert.Equal(t, "off", c.Budget.Enforce)
	assert.Equal(t, 3, *c.Guardrails.MaxPacketAttempts)
	assert.Equal(t, 2, *c.Guardrails.MaxReviewRounds)
	assert.Equal(t, 60, *c.Guardrails.MaxTaskCalls)
	assert.Equal(t, "local", c.Delivery.Endpoint)
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
	bad4.DefectInjection.EveryNUnits = new(0)
	assert.Error(t, bad4.Validate())

	badAttempts := Default()
	badAttempts.Guardrails.MaxPacketAttempts = new(0)
	assert.ErrorContains(t, badAttempts.Validate(), "guardrails.max-packet-attempts")

	badRounds := Default()
	badRounds.Guardrails.MaxReviewRounds = new(11)
	assert.ErrorContains(t, badRounds.Validate(), "guardrails.max-review-rounds")

	badCalls := Default()
	badCalls.Guardrails.MaxTaskCalls = new(1001)
	assert.ErrorContains(t, badCalls.Validate(), "guardrails.max-task-calls")

	badDelivery := Default()
	badDelivery.Delivery.Endpoint = "prod"
	assert.ErrorContains(t, badDelivery.Validate(), "delivery.endpoint")

	badUX := Default()
	badUX.FrontendUXReview = FrontendUXReviewConfig{Enabled: true, Profile: "missing"}
	assert.ErrorContains(t, badUX.Validate(), "frontend-ux-review.profile")

	for _, ep := range []string{"", "local", "draft-pr", "pr-review", "auto-merge"} {
		okDelivery := Default()
		okDelivery.Delivery.Endpoint = ep
		assert.NoError(t, okDelivery.Validate(), ep)
	}
}

func TestLoadAndFind(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "docs", "task")
	assert.NoError(t, os.MkdirAll(taskDir, 0o755))
	// 全局配置隔离到临时路径（四级链第三级；不写真 HOME）。
	globalPath := filepath.Join(dir, "global", "flywheel.config.json")
	t.Setenv(GlobalConfigEnv, globalPath)

	// 四级链：task/cwd/全局全缺 → 物化全局（首装语义）并从全局加载。
	cfgG, err := Load(taskDir, dir)
	assert.NoError(t, err)
	assert.Equal(t, globalPath, cfgG.File, "全局物化后作为配置源")
	assert.Equal(t, "antigravity", cfgG.Routes["default"].Harness, "全局 = 内嵌 §13 法定表")
	assert.Equal(t, "", cfgG.Routes["default"].Provider)
	assert.Equal(t, "", cfgG.Loops["intake"].Provider)
	_, created, mErr := MaterializeGlobal()
	assert.NoError(t, mErr)
	assert.False(t, created, "已存在不覆盖")
	assert.Equal(t, "", FindConfigFile(taskDir, dir))

	// 用户改全局即改默认（物化后的文件是可编辑的真实文件）。
	assert.NoError(t, os.WriteFile(globalPath,
		[]byte(`{"version": 7, "budget": {"enforce": "hard"}}`), 0o644))
	cfgU, err := Load(filepath.Join(dir, "fresh-task"), filepath.Join(dir, "fresh-proj"))
	assert.NoError(t, err)
	assert.Equal(t, 7, *cfgU.Version)
	assert.Equal(t, "hard", cfgU.Budget.Enforce)

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
	assert.Equal(t, "real", cfg.Drivers["plan"])
	assert.Equal(t, "real", cfg.Drivers["execute"], "缺省 drivers 回填 real（mock|real 二值，harness 由绑定表决定）")
	assert.Equal(t, 0.2, cfg.Pricing["deepseek-v4-flash"].Input)
	assert.Equal(t, filepath.Join(taskDir, "flywheel.config.json"), cfg.File)

	// cwd 级配置兜底（taskDir 无配置文件时）。
	other := filepath.Join(dir, "other")
	assert.NoError(t, os.MkdirAll(other, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "flywheel.config.json"), []byte(`{"version": 2}`), 0o644))
	cfg2, err := Load(other, dir)
	assert.NoError(t, err)
	assert.Equal(t, 2, *cfg2.Version)

	// 第四级兜底：全局路径不可写（指到普通文件之下）→ 内嵌法定表。
	blocker := filepath.Join(dir, "blocker")
	assert.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	t.Setenv(GlobalConfigEnv, filepath.Join(blocker, "sub", "flywheel.config.json"))
	cfgE, err := Load(filepath.Join(dir, "nope"), filepath.Join(dir, "nope2"))
	assert.NoError(t, err)
	assert.Equal(t, "(embedded legal table)", cfgE.File)
	assert.Equal(t, "codex-pro", cfgE.Loops["review"].Harness)

	// JSON 合法但校验失败 → Load 报错。
	badCfg := filepath.Join(dir, "badcfg")
	assert.NoError(t, os.MkdirAll(badCfg, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(badCfg, "flywheel.config.json"), []byte(`{"budget": {"enforce": "nuclear"}}`), 0o644))
	_, err = Load(badCfg, dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "budget.enforce")
}

func TestValidateJitterNegativeAndPanel(t *testing.T) {
	c := Default()
	j := -0.5
	c.Retry.Jitter = &j
	assert.ErrorContains(t, c.Validate(), "retry.jitter")

	ok := Default()
	ok.JudgePanel = &LoopBinding{Harness: "pi", Model: "m/x", Channel: "ch"}
	assert.NoError(t, ok.Validate())

	bad := Default()
	bad.JudgePanel = &LoopBinding{Harness: "pi"} // 缺 model/channel
	err := bad.Validate()
	assert.ErrorContains(t, err, "judge-panel")
}

// TestGlobalConfigPathAndMaterializeArms env 覆盖、默认路径、物化错误臂。
func TestGlobalConfigPathAndMaterializeArms(t *testing.T) {
	p, err := GlobalConfigPath()
	require.NoError(t, err)
	assert.Contains(t, p, ".config/flywheel/flywheel.config.json", "默认 XDG 路径")

	t.Setenv(GlobalConfigEnv, "/custom/cfg.json")
	p2, err2 := GlobalConfigPath()
	require.NoError(t, err2)
	assert.Equal(t, "/custom/cfg.json", p2)

	// HOME 解析失败 → GlobalConfigPath/MaterializeGlobal 报错。
	t.Setenv(GlobalConfigEnv, "")
	t.Setenv("HOME", "")
	_, err3 := GlobalConfigPath()
	assert.Error(t, err3)
	_, _, err4 := MaterializeGlobal()
	assert.Error(t, err4)

	// 目录不可写（父路径是普通文件）→ 物化报错。
	blocker := filepath.Join(t.TempDir(), "blocker")
	assert.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	t.Setenv(GlobalConfigEnv, filepath.Join(blocker, "sub", "cfg.json"))
	_, _, err5 := MaterializeGlobal()
	assert.Error(t, err5)
}

// TestEmbeddedTableSeamArms 内嵌表读取失败缝：物化与 Load 兜底两臂。
func TestEmbeddedTableSeamArms(t *testing.T) {
	orig := embeddedTable
	t.Cleanup(func() { embeddedTable = orig })
	embeddedTable = func() ([]byte, error) { return nil, errors.New("embed boom") }

	// 物化路径：新文件 + 表读取失败 → 报错。
	t.Setenv(GlobalConfigEnv, filepath.Join(t.TempDir(), "cfg.json"))
	_, _, err := MaterializeGlobal()
	assert.ErrorContains(t, err, "embed boom")

	// Load 兜底路径：全局不可写 + 表读取失败 → 报错。
	blocker := filepath.Join(t.TempDir(), "blk")
	assert.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	t.Setenv(GlobalConfigEnv, filepath.Join(blocker, "s", "cfg.json"))
	_, err = Load(t.TempDir(), t.TempDir())
	assert.ErrorContains(t, err, "embed boom")
}

// TestMaterializeGlobalWriteFail 目录存在但只读 → 写入失败臂。
func TestMaterializeGlobalWriteFail(t *testing.T) {
	roDir := t.TempDir()
	assert.NoError(t, os.Chmod(roDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })
	t.Setenv(GlobalConfigEnv, filepath.Join(roDir, "cfg.json"))
	_, _, err := MaterializeGlobal()
	assert.Error(t, err)
}
