// Package fallback — 渠道降级（docs/design.md §6 降级）：限流/配额（403）切到
// 下一配置渠道。
//
//	（v0.19 起法定配置无降级链——每角色单一渠道；本包机制保留为 config
//	 驱动，将来恢复备用 provider 时加回渠道绑定即可，无需改代码。）
//
// 历史链形态（v0.18 及以前，供理解测试 fixture）：
//
//	DeepSeek 系: opencode-go -> opencode (Zen 按量, 终端)
//	Kimi 系:     kimi-subscription (kimi code CLI) -> kimi-coding (pi, KIMI_API_KEY)
package fallback

import (
	"fmt"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
)

// Binding 一次调用的 harness+model+channel+thinking 解析结果。
type Binding struct {
	NativeProfile string
	Harness       string
	Provider      string // DSH LLM adapter identity, distinct from the billing/account channel
	Model         string
	Channel       string
	Thinking      string
}

// ResolveBinding 为角色解析绑定：execute 走 routes[route]，其余走 loops[role]。
func ResolveBinding(cfg *config.Config, role, route string) (Binding, error) {
	if role == "execute" {
		b, ok := cfg.Routes[route]
		if !ok {
			return Binding{}, fmt.Errorf("no route binding for %q in config", route)
		}
		return Binding{NativeProfile: b.NativeProfile, Harness: b.Harness, Provider: ResolveProvider(cfg, b.Harness, b.Provider, b.Channel), Model: b.Model, Channel: b.Channel, Thinking: b.Thinking}, nil
	}
	b, ok := cfg.Loops[role]
	if !ok {
		return Binding{}, fmt.Errorf("no loop binding for %q in config", role)
	}
	return Binding{NativeProfile: b.NativeProfile, Harness: b.Harness, Provider: ResolveProvider(cfg, b.Harness, b.Provider, b.Channel), Model: b.Model, Channel: b.Channel, Thinking: b.Thinking}, nil
}

// ResolveProvider separates a DSH adapter from its account channel. A fallback
// must pass the new channel and no previous binding override. Other harnesses
// own their provider namespaces and do not accept this DSH-only field.
func ResolveProvider(cfg *config.Config, harness, provider, channel string) string {
	if harness != "dsh" {
		return ""
	}
	if provider != "" {
		return provider
	}
	if configured := cfg.Channels[channel].Provider; configured != "" {
		return configured
	}
	return channel // legacy DSH configurations used the provider as channel name
}

// NextChannel 配置的下一降级渠道，链尾返回 ""。
func NextChannel(cfg *config.Config, channel string) string {
	c, ok := cfg.Channels[channel]
	if !ok {
		return ""
	}
	return c.Fallback
}

// ChannelModel 降级到某渠道时该角色要用的模型 + harness。渠道未声明该角色
// 的显式映射时返回 nil — 用主绑定的模型组装跨厂商组合是非法的（P1-12①），
// 链在此终止。
type ChannelModel struct {
	Model   string
	Harness string
}

// ModelForChannel 渠道的模型映射（execute 按 route，其余按 role）。
func ModelForChannel(cfg *config.Config, channel, role, route string) *ChannelModel {
	key := role
	if role == "execute" {
		key = route
	}
	c, ok := cfg.Channels[channel]
	if !ok {
		return nil
	}
	mapped, ok := c.Models[key]
	if !ok {
		return nil
	}
	return &ChannelModel{Model: mapped, Harness: c.Harness}
}

// EnvKeyForChannel 该渠道 API key 的环境变量名；订阅 OAuth 渠道为 ""。
func EnvKeyForChannel(cfg *config.Config, channel string) string {
	c, ok := cfg.Channels[channel]
	if !ok || c.EnvKey == nil {
		return ""
	}
	return *c.EnvKey
}

// MissingChannelEnv 渠道声明了 env-key 但环境未设或设为空串时返回该键名；
// 已设非空值或无 key 需求返回 ""。空值与缺失同等对待（review 修正）：pi 对
// 空 --api-key 会静默回退到全局 auth.json，个人凭证被计费——P1-12 的完整
// 语义是"必须存在且非空"，调用方必须 fail-loud 而非裸 spawn。
func MissingChannelEnv(cfg *config.Config, channel string, lookup func(string) (string, bool)) string {
	envKey := EnvKeyForChannel(cfg, channel)
	if envKey == "" {
		return ""
	}
	v, ok := lookup(envKey)
	if !ok || strings.TrimSpace(v) == "" {
		return envKey
	}
	return ""
}

// AuthFallbackAllowed P0-7（用户实测）：AUTH 对 key 渠道永不降级（坏 key 是
// 坏配置）；但订阅/OAuth 渠道（env-key 为 null）登录死了应当降级到有 key 的
// 下一渠道——这正是按量兜底的用途。
func AuthFallbackAllowed(cfg *config.Config, channel string, code errs.ErrorCode) bool {
	if code != errs.AUTH {
		return false
	}
	if EnvKeyForChannel(cfg, channel) != "" {
		return false
	}
	next := NextChannel(cfg, channel)
	return next != "" && EnvKeyForChannel(cfg, next) != ""
}
