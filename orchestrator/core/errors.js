// Error classification and retry policy (DESIGN.md §6, sourced from deepseek-harness).
// Spawn failures are normalized into stable error codes; only a fixed subset is retryable.

export const ERROR_CODES = Object.freeze([
  'AUTH',
  'QUOTA',
  'RATE_LIMIT',
  'SERVER',
  'TRANSPORT',
  'TIMEOUT',
  'EMPTY_RESPONSE',
  'INVALID_REQUEST',
  'CONTEXT_WINDOW_EXCEEDED',
]);

// DESIGN.md §6: only these are retried by default.
export const RETRYABLE_CODES = Object.freeze(['EMPTY_RESPONSE', 'RATE_LIMIT', 'SERVER', 'TIMEOUT', 'TRANSPORT']);

// DESIGN.md §6 降级: rate limit / quota (HTTP 403 over-quota or fair-use ban,
// verified 2026-08-20 via live probe) trigger channel fallback.
// AUTH (HTTP 401 invalid key) does NOT fall back — a bad key means broken
// configuration; switching channel with the same broken key helps nothing.
export const FALLBACK_CODES = Object.freeze(['QUOTA', 'RATE_LIMIT']);

export function isRetryable(code) {
  return RETRYABLE_CODES.includes(code);
}

export function shouldFallback(code) {
  return FALLBACK_CODES.includes(code);
}

// Numeric status codes are matched only at token boundaries (start or a
// non-alphanumeric, followed by a non-alphanumeric or end): "500ms" or
// "port 4291" must never read as HTTP 500/429 and kill a healthy unit.
const statusCode = (digits) => new RegExp(`(^|[^0-9A-Za-z])${digits}(?![0-9A-Za-z])`, 'i');

const PATTERNS = [
  // Order matters: first match wins. More specific patterns first.
  // 401 (invalid key) -> AUTH, never falls back; 403 (over-quota / fair-use
  // ban on OpenCode Go, §7) -> QUOTA, falls back (drilled 2026-08-20).
  ['CONTEXT_WINDOW_EXCEEDED', /context.{0,20}(window|length|too long|exceeded)|maximum context|prompt is too long/i],
  ['RATE_LIMIT', new RegExp(`${statusCode('429').source}|rate.?limit|too many requests`, 'i')],
  ['AUTH', new RegExp(`${statusCode('401').source}|unauthorized|invalid api key|authentication failed|autherror`, 'i')],
  ['QUOTA', new RegExp(`${statusCode('403').source}|forbidden|quota|insufficient.{0,20}(balance|credit|fund|quota)|payment required|billing`, 'i')],
  ['SERVER', new RegExp(`${statusCode('5\\d\\d').source}|server error|internal error|bad gateway|service unavailable|overloaded`, 'i')],
  ['TIMEOUT', /timed? ?out|timeout|deadline exceeded/i],
  ['TRANSPORT', /econnrefused|econnreset|enotfound|eai_again|socket hang up|network error|fetch failed|connection refused|dns/i],
  ['INVALID_REQUEST', new RegExp(`${statusCode('400').source}|invalid request|bad request|malformed`, 'i')],
];

/**
 * Normalize a spawn result into a stable error code.
 * Returns null when nothing looks like an error.
 *
 * @param {object} r
 * @param {number|null} r.exitCode
 * @param {string|null} r.signal
 * @param {boolean} [r.timedOut]
 * @param {string} [r.stderr]
 * @param {string} [r.stdout]
 * @param {Error|null} [r.spawnError]  spawn() itself failed (ENOENT etc.)
 */
export function classifyError({ exitCode = null, signal = null, timedOut = false, stderr = '', stdout = '', spawnError = null } = {}) {
  if (timedOut) return 'TIMEOUT';
  if (spawnError) return 'TRANSPORT';
  const haystack = `${stderr}\n${stdout}`;
  for (const [code, re] of PATTERNS) {
    if (re.test(haystack)) return code;
  }
  if (signal) return 'TRANSPORT'; // killed by something other than our watchdog
  if (exitCode !== null && exitCode !== 0) {
    // Unclassified non-zero exit: treat as an unexpected transport-level failure
    // (retryable; retries exhaust into escalation if it persists).
    return 'TRANSPORT';
  }
  return null;
}

/** Extract a Retry-After hint (seconds or HTTP date) from provider output. */
export function extractRetryAfterMs(text = '') {
  const m = /retry-after:\s*(\d+)/i.exec(text);
  if (m) return parseInt(m[1], 10) * 1000;
  return 0;
}

/**
 * Exponential backoff with jitter (DESIGN.md §6: 500ms -> 10s, +/-10% jitter,
 * honoring the provider's Retry-After).
 *
 * @param {number} attempt zero-based retry attempt index
 * @param {object} opts
 * @param {number} [opts.baseDelayMs=500]
 * @param {number} [opts.maxDelayMs=10000]
 * @param {number} [opts.jitter=0.1]
 * @param {number} [opts.retryAfterMs=0] provider hint; wins when larger
 * @param {() => number} [opts.random] injectable for deterministic tests
 */
export function computeBackoff(attempt, {
  baseDelayMs = 500,
  maxDelayMs = 10000,
  jitter = 0.1,
  retryAfterMs = 0,
  random = Math.random,
} = {}) {
  const exponential = Math.min(baseDelayMs * 2 ** attempt, maxDelayMs);
  const spread = exponential * jitter;
  const jittered = exponential - spread + random() * 2 * spread;
  return Math.round(Math.max(jittered, retryAfterMs));
}
