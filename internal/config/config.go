// Package config — 配置加载（DESIGN.md §13）：Loop × Harness × Model 绑定表
// 活在 flywheel.config.json，不在 skill 里，不硬编码。默认值表与 Node 版
// CONFIG_DEFAULTS 逐字一致；字段缺省由 Normalize 回填。
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoopBinding 非 execute 角色的绑定（loops 表）。
type LoopBinding struct {
	Harness         string `json:"harness"`
	Model           string `json:"model"`
	Channel         string `json:"channel"`
	Thinking        string `json:"thinking"`
	ResidentSession bool   `json:"resident-session"`
}

// RouteBinding execute 角色按 packet.route 的绑定（routes 表）。
type RouteBinding struct {
	Harness  string `json:"harness"`
	Model    string `json:"model"`
	Channel  string `json:"channel"`
	Thinking string `json:"thinking"`
}

// Channel 渠道定义：env-key 可为 null（订阅 OAuth 登录，无 key 注入）。
type Channel struct {
	EnvKey          *string           `json:"env-key"`
	Fallback        string            `json:"fallback"`
	Harness         string            `json:"harness"`
	Models          map[string]string `json:"models"`
	CacheHitRateMin *float64          `json:"cache-hit-rate-min"`
}

// Price 每百万 token 的美元价（pricing 表）。
type Price struct {
	Input     float64 `json:"input"`
	Output    float64 `json:"output"`
	CacheRead float64 `json:"cache-read"`
}

// BudgetConfig 预算熔断（铁律 9）：enforce off|soft|hard。
type BudgetConfig struct {
	Enforce         string `json:"enforce"`
	DailyTokenLimit *int64 `json:"daily-token-limit"`
}

// WatchdogConfig 双计时看门狗参数（§6）。
type WatchdogConfig struct {
	IdleTimeoutMs int64 `json:"idle-timeout-ms"`
	WallTimeoutMs int64 `json:"wall-timeout-ms"`
	KillGraceMs   int64 `json:"kill-grace-ms"`
}

// RetryConfig 传输层重试（§6）。
type RetryConfig struct {
	BaseDelayMs         float64 `json:"base-delay-ms"`
	MaxDelayMs          float64 `json:"max-delay-ms"`
	Jitter              float64 `json:"jitter"`
	MaxTransportRetries int     `json:"max-transport-retries"`
}

// CredentialsConfig live 凭证人工确认轮询（§9）。
type CredentialsConfig struct {
	PollIntervalMs int64 `json:"poll-interval-ms"`
	TimeoutMs      int64 `json:"timeout-ms"`
}

// VerifyConfig 闸门命令参数（§4.4）。
type VerifyConfig struct {
	CommandTimeoutMs int64    `json:"command-timeout-ms"`
	EnvAllowlist     []string `json:"env-allowlist"`
	TestFilePatterns []string `json:"test-file-patterns"`
}

// DefectInjectionConfig 缺陷注入抽检（§4.3，常备无开关）。
type DefectInjectionConfig struct {
	EveryNUnits        int     `json:"every-n-units"`
	TestFileRatioBoost float64 `json:"test-file-ratio-boost"`
	Patch              string  `json:"patch"`
	TestCmd            string  `json:"test-cmd"`
}

// WorktreeConfig 单元级 worktree 隔离开关（v0.16，默认开启）。
type WorktreeConfig struct {
	Enabled *bool `json:"enabled"`
}

// Source intake 任务源（§4.5）：github-issues | command。
type Source struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Repo   string   `json:"repo"`
	Labels []string `json:"labels"`
	Limit  int      `json:"limit"`
	Cmd    string   `json:"cmd"`
}

// IntakeConfig intake 源表。
type IntakeConfig struct {
	Sources []Source `json:"sources"`
}

// Config 完整配置（缺省值已归一）。
type Config struct {
	Version         *int                    `json:"version"`
	Drivers         map[string]string       `json:"drivers"`
	Loops           map[string]LoopBinding  `json:"loops"`
	Routes          map[string]RouteBinding `json:"routes"`
	Channels        map[string]Channel      `json:"channels"`
	Budget          BudgetConfig            `json:"budget"`
	Watchdog        WatchdogConfig          `json:"watchdog"`
	Retry           RetryConfig             `json:"retry"`
	Credentials     CredentialsConfig       `json:"credentials"`
	Verify          VerifyConfig            `json:"verify"`
	DefectInjection DefectInjectionConfig   `json:"defect-injection"`
	Worktree        WorktreeConfig          `json:"worktree"`
	Intake          IntakeConfig            `json:"intake"`
	Pricing         map[string]Price        `json:"pricing"`
	MockBehaviors   map[string]any          `json:"mock-behaviors"`
	File            string                  `json:"-"`
}

// Default 返回 §13 法定默认值表（与 Node CONFIG_DEFAULTS 逐字一致）。
func Default() Config {
	return Config{
		Drivers: map[string]string{"plan": "kimi", "execute": "pi", "review": "pi"},
		Budget:  BudgetConfig{Enforce: "soft", DailyTokenLimit: nil},
		Watchdog: WatchdogConfig{
			IdleTimeoutMs: 300_000,
			WallTimeoutMs: 3_600_000,
			KillGraceMs:   5_000,
		},
		Retry: RetryConfig{
			BaseDelayMs:         500,
			MaxDelayMs:          10_000,
			Jitter:              0.1,
			MaxTransportRetries: 5,
		},
		DefectInjection: DefectInjectionConfig{EveryNUnits: 5, TestFileRatioBoost: 0.5},
		Verify:          VerifyConfig{CommandTimeoutMs: 300_000},
		Worktree:        WorktreeConfig{Enabled: boolPtr(true)},
		Credentials:     CredentialsConfig{PollIntervalMs: 30_000, TimeoutMs: 86_400_000},
	}
}

func boolPtr(b bool) *bool { return &b }

// Normalize 将缺省字段回填为默认值（Node deepMerge 语义的零值等价）。
func (c *Config) Normalize() {
	d := Default()
	if c.Drivers == nil {
		c.Drivers = d.Drivers
	}
	if c.Budget.Enforce == "" {
		c.Budget.Enforce = "soft"
	}
	if c.Watchdog.IdleTimeoutMs == 0 {
		c.Watchdog.IdleTimeoutMs = d.Watchdog.IdleTimeoutMs
	}
	if c.Watchdog.WallTimeoutMs == 0 {
		c.Watchdog.WallTimeoutMs = d.Watchdog.WallTimeoutMs
	}
	if c.Watchdog.KillGraceMs == 0 {
		c.Watchdog.KillGraceMs = d.Watchdog.KillGraceMs
	}
	if c.Retry.BaseDelayMs == 0 {
		c.Retry.BaseDelayMs = d.Retry.BaseDelayMs
	}
	if c.Retry.MaxDelayMs == 0 {
		c.Retry.MaxDelayMs = d.Retry.MaxDelayMs
	}
	if c.Retry.Jitter == 0 {
		c.Retry.Jitter = d.Retry.Jitter
	}
	if c.Retry.MaxTransportRetries == 0 {
		c.Retry.MaxTransportRetries = d.Retry.MaxTransportRetries
	}
	if c.DefectInjection.EveryNUnits == 0 {
		c.DefectInjection.EveryNUnits = d.DefectInjection.EveryNUnits
	}
	if c.DefectInjection.TestFileRatioBoost == 0 {
		c.DefectInjection.TestFileRatioBoost = d.DefectInjection.TestFileRatioBoost
	}
	if c.Verify.CommandTimeoutMs == 0 {
		c.Verify.CommandTimeoutMs = d.Verify.CommandTimeoutMs
	}
	if c.Credentials.PollIntervalMs == 0 {
		c.Credentials.PollIntervalMs = d.Credentials.PollIntervalMs
	}
	if c.Credentials.TimeoutMs == 0 {
		c.Credentials.TimeoutMs = d.Credentials.TimeoutMs
	}
	if c.Worktree.Enabled == nil {
		c.Worktree.Enabled = d.Worktree.Enabled
	}
}

// Validate 执行 §5 版本纪律与法定枚举校验（与 Node loadConfig 校验集一致）。
func (c *Config) Validate() error {
	if c.Version != nil && (*c.Version < 1) {
		return errors.New("version must be a positive integer")
	}
	switch c.Budget.Enforce {
	case "off", "soft", "hard":
	default:
		return errors.New("budget.enforce must be off|soft|hard")
	}
	if c.Budget.DailyTokenLimit != nil && *c.Budget.DailyTokenLimit <= 0 {
		return errors.New("budget.daily-token-limit must be a positive integer or null")
	}
	if c.DefectInjection.EveryNUnits < 1 {
		return errors.New("defect-injection.every-n-units must be an integer >= 1")
	}
	return nil
}

// Load 读取 flywheel.config.json：先 task 目录，再工作目录（目标仓库根）。
// 缺失抛 ConfigError。
func Load(taskDir, cwd string) (*Config, error) {
	file := FindConfigFile(taskDir, cwd)
	if file == "" {
		return nil, fmt.Errorf("flywheel.config.json not found (looked in %s and %s)", taskDir, cwd)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %w", file, err)
	}
	c.File = file
	c.Normalize()
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return &c, nil
}

// FindConfigFile 返回首个存在的候选配置路径，无则返回 ""。
func FindConfigFile(taskDir, cwd string) string {
	for _, c := range []string{filepath.Join(taskDir, "flywheel.config.json"), filepath.Join(cwd, "flywheel.config.json")} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
