// Channel fallback (DESIGN.md §6 降级): on rate-limit / quota (403) switch to
// the next configured channel. Standing chains (user decision 2026-08-20 —
// no direct DeepSeek API fallback;兜底 stays inside the OpenCode family):
//   DeepSeek roles: opencode-go -> opencode (Zen pay-as-you-go, terminal)
//   Kimi roles:     kimi-subscription (kimi code CLI) -> kimi-coding (pi, KIMI_API_KEY)
// Selection here is pure; the machine performs the actual retry on the next
// channel, and a channel may carry a harness switch (kimi CLI -> pi).

import { shouldFallback } from './errors.js';

export { shouldFallback };

/**
 * Resolve harness/model/channel for a role.
 * @param {object} config
 * @param {'plan'|'execute'|'review'} role
 * @param {string} [route] packet route, for role=execute
 */
export function resolveBinding(config, role, route = null) {
  if (role === 'execute') {
    const binding = config.routes?.[route ?? 'default'];
    if (!binding) throw new Error(`no route binding for "${route}" in config`);
    return binding;
  }
  const binding = config.loops?.[role];
  if (!binding) throw new Error(`no loop binding for "${role}" in config`);
  return binding;
}

/** Next channel in the configured fallback chain, or null at the end. */
export function nextChannel(config, channel) {
  const next = config.channels?.[channel]?.fallback;
  return next ?? null;
}

/**
 * Model + harness to use when falling back to another channel for a role.
 * A channel may imply a harness switch (e.g. kimi-subscription -> kimi-coding
 * moves Plan from the kimi CLI to pi). Returns null when the channel declares
 * NO explicit mapping for the role — falling back to the primary binding's
 * model would assemble an invalid cross-provider combo (P1-12①), so the
 * chain ends there instead.
 * @returns {{model: string, harness: string|null}|null}
 */
export function modelForChannel(config, channel, role, route = null) {
  const key = role === 'execute' ? (route ?? 'default') : role;
  const mapped = config.channels?.[channel]?.models?.[key];
  if (!mapped) return null;
  return {
    model: mapped,
    harness: config.channels?.[channel]?.harness ?? null,
  };
}

/** Env var name that must hold the channel's API key, or null (subscription login). */
export function envKeyForChannel(config, channel) {
  return config.channels?.[channel]?.['env-key'] ?? null;
}

/**
 * The channel declares an env-key that is unset in the environment, or null
 * when it is set / the channel needs none (kimi-subscription OAuth).
 * Callers must fail loud instead of spawning keyless (P1-12②).
 */
export function missingChannelEnv(config, channel) {
  const envKey = envKeyForChannel(config, channel);
  return envKey && !process.env[envKey] ? envKey : null;
}

/**
 * P0-7 (user-verified): AUTH never falls back for key-based channels (a bad
 * key is broken config). But a subscription/OAuth channel (env-key: null)
 * whose login died SHOULD fall back when the next channel has a key — that is
 * exactly what the pay-as-you-go fallback is for.
 */
export function authFallbackAllowed(config, channel, code) {
  if (code !== 'AUTH') return false;
  if (envKeyForChannel(config, channel) !== null) return false;
  const next = nextChannel(config, channel);
  return next !== null && envKeyForChannel(config, next) !== null;
}
