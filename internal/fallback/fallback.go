// Package fallback — 渠道降级（DESIGN.md §6 降级）：限流/配额（403）切到
// 下一配置渠道。常备链（用户裁决 2026-08-20，无 DeepSeek 直连兜底）：
//
//	DeepSeek 系: opencode-go -> opencode (Zen 按量, 终端)
//	Kimi 系:     kimi-subscription (kimi code CLI) -> kimi-coding (pi, KIMI_API_KEY)
//
// 选择逻辑是纯函数；实际重试由 machine 在下一渠道执行，渠道可携带 harness
// 切换（kimi CLI -> pi）。
package fallback

import (
	"fmt"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
)

// Binding 一次调用的 harness+model+channel+thinking 解析结果。
type Binding struct {
	Harness  string
	Model    string
	Channel  string
	Thinking string
}

// ResolveBinding 为角色解析绑定：execute 走 routes[route]，其余走 loops[role]。
func ResolveBinding(cfg *config.Config, role, route string) (Binding, error) {
	if role == "execute" {
		b, ok := cfg.Routes[route]
		if !ok {
			return Binding{}, fmt.Errorf("no route binding for %q in config", route)
		}
		return Binding{Harness: b.Harness, Model: b.Model, Channel: b.Channel, Thinking: b.Thinking}, nil
	}
	b, ok := cfg.Loops[role]
	if !ok {
		return Binding{}, fmt.Errorf("no loop binding for %q in config", role)
	}
	return Binding{Harness: b.Harness, Model: b.Model, Channel: b.Channel, Thinking: b.Thinking}, nil
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

// MissingChannelEnv 渠道声明了 env-key 但环境未设时返回该键名；已设或无 key
// 需求返回 ""。调用方必须 fail-loud 而非裸 spawn（P1-12②：pi 会静默回退到
// 全局 auth.json）。
func MissingChannelEnv(cfg *config.Config, channel string, lookup func(string) (string, bool)) string {
	envKey := EnvKeyForChannel(cfg, channel)
	if envKey == "" {
		return ""
	}
	if _, ok := lookup(envKey); !ok {
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
