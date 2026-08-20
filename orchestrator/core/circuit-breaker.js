// Circuit breaker (DESIGN.md §6, §2 rule 9): budget enforcement is a switch —
// off | soft | hard (default soft: warn and record, never stop). Token/cost
// recording is ALWAYS on, in every mode. Spin detection is not about money:
// the same packet failing repeatedly in the same way escalates in every mode,
// because it means the loop is unhealthy.

import { createHash } from 'node:crypto';

export function todayKey(now = new Date()) {
  return now.toISOString().slice(0, 10);
}

/**
 * Check the daily budget against the configured limit.
 * @returns {{action: 'ok'|'warn'|'stop', reason: string|null, enforce: string}}
 */
export function checkBudget(state, config, now = new Date()) {
  const enforce = config?.budget?.enforce ?? 'soft';
  const limit = config?.budget?.['daily-token-limit'] ?? null;
  if (enforce === 'off' || limit === null) {
    return { action: 'ok', reason: null, enforce };
  }
  const day = todayKey(now);
  const used = state.budget?.daily?.[day]?.tokens ?? 0;
  if (used < limit) {
    return { action: 'ok', reason: null, enforce };
  }
  const reason = `daily token budget exceeded: ${used} >= ${limit} (${day})`;
  return enforce === 'hard' ? { action: 'stop', reason, enforce } : { action: 'warn', reason, enforce };
}

/**
 * Failure signature for spin detection (v0.14 DC4): the failure MODE key
 * (error codes + failed commands, order-independent) plus the changed-file
 * set for similarity comparison. Serialized as JSON so it can live in
 * state.json.
 */
export function failureSignature({ errorCodes = [], failedCommands = [], changedFiles = [] } = {}) {
  const mode = {
    errorCodes: [...new Set(errorCodes)].sort(),
    failedCommands: [...new Set(failedCommands)].sort(),
  };
  return JSON.stringify({
    key: createHash('sha256').update(JSON.stringify(mode)).digest('hex').slice(0, 16),
    files: [...new Set(changedFiles)].sort(),
  });
}

/** Jaccard similarity of two string arrays (|A∩B| / |A∪B|; both empty -> 1). */
export function jaccard(a, b) {
  const setA = new Set(a);
  const setB = new Set(b);
  if (setA.size === 0 && setB.size === 0) return 1;
  let intersection = 0;
  for (const x of setA) {
    if (setB.has(x)) intersection += 1;
  }
  return intersection / (setA.size + setB.size - intersection);
}

export const SPIN_JACCARD_THRESHOLD = 0.7;

/**
 * Spin (v0.14 DC4): >= 2 consecutive failures of the same packet where the
 * failure mode is identical (error codes + failed commands) AND the changed
 * file sets are highly similar (Jaccard >= 0.7).
 *
 * @param {string|null} previousSignature failureSignature() JSON, or null
 * @param {string} currentSignature failureSignature() JSON
 * @param {number} consecutiveFailures failures in a row for this packet
 */
export function detectSpin(previousSignature, currentSignature, consecutiveFailures) {
  if (consecutiveFailures < 2 || previousSignature === null) return false;
  const prev = JSON.parse(previousSignature);
  const curr = JSON.parse(currentSignature);
  return prev.key === curr.key && jaccard(prev.files, curr.files) >= SPIN_JACCARD_THRESHOLD;
}
