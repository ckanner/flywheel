// Metrics (DESIGN.md §6 可观测): every spawn appends one JSONL record —
// model, harness, prompt hash, tokens, cost, cache hit, duration, exit code,
// error code. This table is how a human supervises the loop without watching it.

import { appendFileSync, mkdirSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { dirname } from 'node:path';

export function promptHash(prompt) {
  return createHash('sha256').update(prompt, 'utf8').digest('hex').slice(0, 12);
}

/**
 * Append one metrics record. Synchronous on purpose: it must land even if the
 * orchestrator dies right after.
 */
export function appendMetrics(file, entry) {
  mkdirSync(dirname(file), { recursive: true });
  appendFileSync(file, `${JSON.stringify({ ts: new Date().toISOString(), ...entry })}\n`, 'utf8');
}

/**
 * Price lookup for a model id (P1-13): try the full id, the short id
 * (after the last "/"), then the kimi-prefixed short id — "kimi-code/k3"
 * and "kimi-coding/k3" must both land on the "kimi-k3" pricing entry.
 */
export function priceForModel(pricing, model) {
  if (!pricing || !model) return null;
  const short = model.split('/').pop();
  return pricing[model] ?? pricing[short] ?? pricing[`kimi-${short}`] ?? null;
}

/**
 * Cost fallback when the driver does not report one (DESIGN.md 附录 D: usage
 * details may be missing fields; never assume they exist).
 *
 * @param {object|null} usage {input, output, cacheRead, cacheWrite}
 * @param {object|null} price {input, output, cache-read} USD per 1M tokens
 */
export function estimateCostUsd(usage, price) {
  if (!usage || !price) return null;
  const perM = 1_000_000;
  const input = ((usage.input ?? 0) / perM) * (price.input ?? 0);
  const output = ((usage.output ?? 0) / perM) * (price.output ?? 0);
  const cacheRead = ((usage.cacheRead ?? 0) / perM) * (price['cache-read'] ?? 0);
  // Cache writes are billed at the uncached input rate (DeepSeek semantics);
  // pi's usage buckets are disjoint (verified in pi dist usage-totals.js).
  const cacheWrite = ((usage.cacheWrite ?? 0) / perM) * (price.input ?? 0);
  return Math.round((input + output + cacheRead + cacheWrite) * 1e8) / 1e8;
}
