// kimi driver: kimi code CLI for the Plan Loop (DESIGN.md §4.1: kimi code +
// K3 max, long-lived session).
//
// Verified against kimi CLI 0.37.2 (2026-08-20):
// - `kimi -p "<prompt>" --output-format stream-json` prints NDJSON:
//   {"role":"meta","type":"system.version"} / {"role":"assistant","content":...}
//   / {"role":"meta","type":"session.resume_hint","session_id":"session_..."}.
//   No usage/token events are exposed -> usage is recorded as null.
// - Resident session: first call captures session_id from the resume hint;
//   later calls resume with `kimi -r session_<id>` (same id confirmed echoed).
// - Session death (hang/expiry): `kimi -r <dead-id>` exits 1 with
//   "Session ... not found" on stderr -> the machine cold-starts a fresh
//   session from the contract layer (§4.1 rebuild protocol).
// - reasoning_effort=max per invocation WITHOUT touching the user's global
//   config: env KIMI_MODEL_THINKING_EFFORT=max (official, kimi provider only;
//   docs: kimi-code-cli/configuration/env-vars.html). The user's global
//   [thinking].effort stays "high"; we never write to ~/.kimi-code.
// - 1M-context + max stability under long sessions remains empirical gate
//   §10-3 (NOT covered here): the watchdog is the mitigation.

import { runWithWatchdog } from '../core/watchdog.js';
import { buildChildEnv } from './pi.js';

export const KIMI_DEFAULT_MODEL = 'kimi-code/k3';

/** Parse kimi's stream-json: session id, assistant text, any error/usage. */
export function parseKimiStreamJson(stdout) {
  let sessionId = null;
  let text = '';
  let errorMessage = null;
  let usage = null;
  for (const line of stdout.split('\n')) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    let event;
    try {
      event = JSON.parse(trimmed);
    } catch {
      continue;
    }
    if (event.role === 'meta' && event.type === 'session.resume_hint' && event.session_id) {
      sessionId = event.session_id;
      continue;
    }
    if (event.role === 'assistant') {
      if (typeof event.content === 'string') text = event.content;
      else if (Array.isArray(event.content)) {
        text = event.content.map((p) => p.text ?? '').join('');
      }
      continue;
    }
    if (event.type === 'error' || event.role === 'error') {
      errorMessage = event.message ?? event.error ?? JSON.stringify(event);
      continue;
    }
    if (event.usage && typeof event.usage === 'object') {
      usage = event.usage; // not emitted by 0.37.2; captured if it ever appears
    }
  }
  return { sessionId, text, errorMessage, usage };
}

/**
 * @param {object} args
 * @param {string} args.prompt
 * @param {string|null} [args.sessionId] resume handle for the resident Plan session
 * @param {string} [args.model] model alias, e.g. kimi-code/k3
 * @param {string} [args.cwd]
 * @param {object} [args.apiKeys] extra env to inject
 * @param {object} [args.watchdogOpts]
 */
export async function runKimi({ prompt, sessionId = null, model = null, cwd, apiKeys = {}, watchdogOpts = {} }) {
  const args = [];
  if (sessionId) args.push('-r', sessionId);
  args.push('-m', model ?? KIMI_DEFAULT_MODEL);
  args.push('-p', prompt, '--output-format', 'stream-json');
  const env = {
    ...buildChildEnv(process.env, apiKeys),
    // Per-invocation max effort; never touches the user's global config (§7).
    KIMI_MODEL_THINKING_EFFORT: 'max',
  };
  const result = await runWithWatchdog('kimi', args, {
    cwd,
    env,
    idleTimeoutMs: watchdogOpts.idleTimeoutMs,
    wallTimeoutMs: watchdogOpts.wallTimeoutMs,
    killGraceMs: watchdogOpts.killGraceMs,
  });
  const parsed = parseKimiStreamJson(result.stdout);
  return {
    ...result,
    harness: 'kimi',
    provider: 'kimi',
    model: model ?? KIMI_DEFAULT_MODEL,
    usage: parsed.usage, // null: not exposed by the CLI stream (0.37.2)
    reasoningChars: null,
    costUsd: null,
    settled: result.exitCode === 0 && !result.timedOut,
    text: parsed.text,
    sessionId: parsed.sessionId,
    errorMessage: parsed.errorMessage ?? (result.exitCode !== 0 ? result.stderr.trim() || null : null),
  };
}
