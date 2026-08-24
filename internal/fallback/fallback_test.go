package fallback

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
)

func testConfig() *config.Config {
	cfg := config.Default()
	cfg.Loops = map[string]config.LoopBinding{
		"plan":   {Harness: "kimi", Model: "kimi-code/k3", Channel: "kimi-subscription", Thinking: "max"},
		"review": {Harness: "pi", Model: "opencode-go/deepseek-v4-pro", Channel: "opencode-go", Thinking: "max"},
	}
	cfg.Routes = map[string]config.RouteBinding{
		"default":  {Harness: "pi", Model: "opencode-go/deepseek-v4-flash", Channel: "opencode-go", Thinking: "max"},
		"frontend": {Harness: "kimi", Model: "kimi-code/k3", Channel: "kimi-subscription", Thinking: "max"},
	}
	kimiSubKey := ""
	opencodeKey := "OPENCODE_API_KEY"
	kimiCodingKey := "KIMI_API_KEY"
	cfg.Channels = map[string]config.Channel{
		"opencode-go": {
			EnvKey:   &opencodeKey,
			Fallback: "opencode",
		},
		"opencode": {
			EnvKey: &opencodeKey,
			Models: map[string]string{
				"default": "opencode/deepseek-v4-flash",
				"review":  "opencode/deepseek-v4-pro",
			},
		},
		"kimi-subscription": {
			EnvKey:   &kimiSubKey,
			Fallback: "kimi-coding",
		},
		"kimi-coding": {
			EnvKey:   &kimiCodingKey,
			Harness:  "pi",
			Fallback: "opencode",
			Models: map[string]string{
				"plan": "kimi-coding/k3",
			},
		},
	}
	return &cfg
}

func lookup(keys ...string) func(string) (string, bool) {
	set := map[string]bool{}
	for _, k := range keys {
		set[k] = true
	}
	return func(k string) (string, bool) {
		_, ok := set[k]
		return "", ok
	}
}

func TestResolveBinding(t *testing.T) {
	cfg := testConfig()
	b, err := ResolveBinding(cfg, "execute", "default")
	assert.NoError(t, err)
	assert.Equal(t, "opencode-go/deepseek-v4-flash", b.Model)
	assert.Equal(t, "pi", b.Harness)

	b, err = ResolveBinding(cfg, "execute", "frontend")
	assert.NoError(t, err)
	assert.Equal(t, "kimi-code/k3", b.Model)

	b, err = ResolveBinding(cfg, "review", "")
	assert.NoError(t, err)
	assert.Equal(t, "opencode-go/deepseek-v4-pro", b.Model)

	_, err = ResolveBinding(cfg, "execute", "nonexistent-route")
	assert.Error(t, err)
	_, err = ResolveBinding(cfg, "nonexistent-role", "")
	assert.Error(t, err)
}

func TestNextChannel(t *testing.T) {
	cfg := testConfig()
	assert.Equal(t, "opencode", NextChannel(cfg, "opencode-go"))
	assert.Equal(t, "kimi-coding", NextChannel(cfg, "kimi-subscription"))
	assert.Equal(t, "", NextChannel(cfg, "opencode"))
	assert.Equal(t, "", NextChannel(cfg, "unknown"))
}

func TestModelForChannel(t *testing.T) {
	cfg := testConfig()
	m := ModelForChannel(cfg, "opencode", "execute", "default")
	assert.NotNil(t, m)
	assert.Equal(t, "opencode/deepseek-v4-flash", m.Model)
	assert.Equal(t, "", m.Harness, "opencode 渠道未声明 harness")

	m = ModelForChannel(cfg, "kimi-coding", "plan", "")
	assert.NotNil(t, m)
	assert.Equal(t, "kimi-coding/k3", m.Model)
	assert.Equal(t, "pi", m.Harness, "渠道可携带 harness 切换（kimi CLI -> pi）")

	assert.Nil(t, ModelForChannel(cfg, "opencode-go", "execute", "default"), "无映射=链终止")
	assert.Nil(t, ModelForChannel(cfg, "unknown", "plan", ""))
	assert.Nil(t, ModelForChannel(cfg, "opencode", "plan", ""))
}

func TestEnvKeyAndMissing(t *testing.T) {
	cfg := testConfig()
	assert.Equal(t, "OPENCODE_API_KEY", EnvKeyForChannel(cfg, "opencode-go"))
	assert.Equal(t, "", EnvKeyForChannel(cfg, "kimi-subscription"), "订阅 OAuth 无 key")
	assert.Equal(t, "", EnvKeyForChannel(cfg, "unknown"))

	assert.Equal(t, "OPENCODE_API_KEY", MissingChannelEnv(cfg, "opencode-go", lookup()))
	// lookup() 返回空值 → 新语义下等同缺失。
	assert.Equal(t, "OPENCODE_API_KEY", MissingChannelEnv(cfg, "opencode-go", lookup("OPENCODE_API_KEY")))
	// review 修正：空值与缺失同等对待（pi 对空 --api-key 静默吃 auth.json）。
	assert.Equal(t, "", MissingChannelEnv(cfg, "kimi-subscription", lookup()))
	assert.Equal(t, "", MissingChannelEnv(cfg, "unknown", lookup()))

	// key 存在且非空 → 返回 ""（key 可用，不缺失）。
	nonEmpty := func(k string) (string, bool) {
		if k == "OPENCODE_API_KEY" {
			return "sk-real-key", true
		}
		return "", false
	}
	assert.Equal(t, "", MissingChannelEnv(cfg, "opencode-go", nonEmpty))
}

func TestAuthFallbackAllowed(t *testing.T) {
	cfg := testConfig()
	// key 渠道 AUTH 永不降级。
	assert.False(t, AuthFallbackAllowed(cfg, "opencode-go", errs.AUTH))
	// 订阅渠道 AUTH（登录死）且下一渠道有 key → 允许降级。
	assert.True(t, AuthFallbackAllowed(cfg, "kimi-subscription", errs.AUTH))
	// 非 AUTH 一律不允许（降级由 ShouldFallback 管）。
	assert.False(t, AuthFallbackAllowed(cfg, "kimi-subscription", errs.QUOTA))
	// 未知渠道。
	assert.False(t, AuthFallbackAllowed(cfg, "unknown", errs.AUTH))
}
