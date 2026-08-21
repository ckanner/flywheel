// Child-process environment discipline (§9): everything the orchestrator
// spawns — agent CLIs, gate commands, intake source commands — gets a
// WHITELISTED environment. No API keys, no tokens, nothing beyond what the
// child needs to run; the channel's executor key is injected separately and
// explicitly (drivers/pi.js --api-key). Escapes are explicit only
// (config verify.env-allowlist).

export const CHILD_ENV_KEYS = [
  'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL', 'TERM', 'SHELL', 'USER', 'LOGNAME',
  'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_CACHE_HOME', 'XDG_STATE_HOME',
];

/**
 * Whitelisted env for agent harness children; `apiKeys` are injected on top
 * (the low-value executor key, or human-approved live credentials).
 */
export function buildChildEnv(baseEnv = process.env, apiKeys = {}) {
  const env = {};
  for (const k of CHILD_ENV_KEYS) {
    if (baseEnv[k] !== undefined) env[k] = baseEnv[k];
  }
  for (const [k, v] of Object.entries(apiKeys)) {
    if (v !== undefined && v !== null) env[k] = v;
  }
  return env;
}

/**
 * Whitelisted env for deterministic gate/test commands (P0-4): an L1/L2/L3
 * command runs code the executor wrote, so it must NOT inherit the
 * orchestrator's keys. `allow` explicitly appends extra keys (config
 * verify.env-allowlist — e.g. private registry tokens).
 * NODE_TEST_CONTEXT is always scrubbed: a nested `node --test` in inherited
 * "child" mode exits 0 even when red (verified 2026-08-20) — the oracle needs
 * true exit codes.
 */
export function gateEnv(env = process.env, { allow = [] } = {}) {
  const out = {};
  for (const k of [...CHILD_ENV_KEYS, ...allow]) {
    if (env[k] !== undefined) out[k] = env[k];
  }
  delete out.NODE_TEST_CONTEXT;
  return out;
}
