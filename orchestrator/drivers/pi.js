// pi driver (DESIGN.md §4.2/§4.3): drives Execute and Review through the pi CLI.
//   pi -p "<prompt>" --model <provider/model> --thinking max --no-session --mode json
// stdout is an NDJSON event stream; usage (input/output/reasoning/cacheRead/
// cacheWrite + cost) is read from assistant `message_end` events, and
// `agent_settled` marks a clean end. Appendix D warning: at thinking=max the
// usage may lack the reasoning breakdown — never assume the field exists.
//
// Credential hygiene (§9): the child gets a minimal env — the low-value
// executor key (OPENCODE_API_KEY) plus what the CLI needs to run. Nothing else.

import { runWithWatchdog } from '../core/watchdog.js';
import { buildChildEnv } from '../core/child-env.js';

export { CHILD_ENV_KEYS, buildChildEnv } from '../core/child-env.js';

/** Parse pi's NDJSON event stream: aggregate usage, final text, settled flag,
 * and — critically — stream-level errors. pi exits 0 even when the provider
 * call fails (verified 2026-08-20: a bad key yields stopReason:"error" with
 * errorMessage "401: ..." INSIDE the stream), so the error must be read here. */
export function parsePiEventStream(stdout) {
  const usage = { input: 0, output: 0, reasoning: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0 };
  let reasoningChars = 0;
  let costUsd = 0;
  let sawCost = false;
  let settled = false;
  let text = '';
  let provider = null;
  let model = null;
  let errorMessage = null;

  for (const line of stdout.split('\n')) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    let event;
    try {
      event = JSON.parse(trimmed);
    } catch {
      continue; // non-JSON noise on stdout is tolerated, never fatal
    }
    if (event.type === 'agent_settled') {
      settled = true;
      continue;
    }
    if (event.type !== 'message_end' || event.message?.role !== 'assistant') continue;
    const msg = event.message;
    provider ??= msg.provider ?? null;
    model ??= msg.model ?? null;
    if (msg.stopReason === 'error' && msg.errorMessage) {
      errorMessage = msg.errorMessage;
    }
    const u = msg.usage;
    if (u) {
      usage.input += u.input ?? 0;
      usage.output += u.output ?? 0;
      usage.reasoning += u.reasoning ?? 0; // may be absent at thinking=max (附录 D)
      usage.cacheRead += u.cacheRead ?? 0;
      usage.cacheWrite += u.cacheWrite ?? 0;
      usage.totalTokens += u.totalTokens ?? (u.input ?? 0) + (u.output ?? 0);
      if (typeof u.cost?.total === 'number') {
        costUsd += u.cost.total;
        sawCost = true;
      }
    }
    for (const part of msg.content ?? []) {
      if (part.type === 'thinking') reasoningChars += (part.thinking ?? '').length;
      if (part.type === 'text') text = part.text ?? text;
    }
  }
  return { usage, reasoningChars, costUsd: sawCost ? costUsd : null, settled, text, provider, model, errorMessage };
}

/**
 * @param {object} args
 * @param {string} args.prompt
 * @param {string} args.model e.g. opencode-go/deepseek-v4-flash
 * @param {string} [args.thinking='max'] always max across the whole system (§7)
 * @param {string} [args.cwd]
 * @param {object} [args.apiKeys] env keys to inject, e.g. {OPENCODE_API_KEY: '...'}
 * @param {object} [args.watchdogOpts] idleTimeoutMs/wallTimeoutMs/killGraceMs
 * @param {(line:string)=>void} [args.onEvent] raw NDJSON line callback
 */
export async function runPi({ prompt, model, thinking = 'max', cwd, apiKey = null, apiKeys = {}, watchdogOpts = {}, onEvent }) {
  const args = ['-p', prompt, '--model', model, '--thinking', thinking, '--no-session', '--mode', 'json'];
  // Make the injected CHANNEL key authoritative: pi's own auth store
  // (~/.pi/agent/auth.json) otherwise silently wins over the environment
  // (verified 2026-08-20), which would void credential hygiene (§9).
  // `apiKey` comes ONLY from the channel's env-key — never from the
  // live-credentials approval set (#14).
  if (apiKey) args.push('--api-key', apiKey);
  let buf = '';
  const result = await runWithWatchdog('pi', args, {
    cwd,
    env: buildChildEnv(process.env, apiKeys),
    idleTimeoutMs: watchdogOpts.idleTimeoutMs,
    wallTimeoutMs: watchdogOpts.wallTimeoutMs,
    killGraceMs: watchdogOpts.killGraceMs,
    onActivity: onEvent
      ? (chunk, stream) => {
          if (stream !== 'stdout') return;
          buf += chunk;
          let idx;
          while ((idx = buf.indexOf('\n')) !== -1) {
            onEvent(buf.slice(0, idx));
            buf = buf.slice(idx + 1);
          }
        }
      : undefined,
  });
  const parsed = parsePiEventStream(result.stdout);
  return {
    ...result,
    harness: 'pi',
    provider: parsed.provider ?? model.split('/')[0] ?? null,
    model: parsed.model ?? model,
    usage: parsed.usage,
    reasoningChars: parsed.reasoningChars,
    costUsd: parsed.costUsd,
    settled: parsed.settled,
    text: parsed.text,
    errorMessage: parsed.errorMessage,
  };
}
