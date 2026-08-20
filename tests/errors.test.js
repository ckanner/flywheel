import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  classifyError, isRetryable, shouldFallback, computeBackoff, extractRetryAfterMs,
  RETRYABLE_CODES,
} from '../orchestrator/core/errors.js';

test('classifyError: timeout from watchdog wins over everything', () => {
  assert.equal(classifyError({ timedOut: true, exitCode: null, stderr: 'rate limit' }), 'TIMEOUT');
});

test('classifyError: stderr pattern mapping', () => {
  assert.equal(classifyError({ exitCode: 1, stderr: 'HTTP 429 too many requests' }), 'RATE_LIMIT');
  assert.equal(classifyError({ exitCode: 1, stderr: '403: forbidden — fair use limit reached' }), 'QUOTA');
  assert.equal(classifyError({ exitCode: 1, stderr: '401: {"type":"AuthError","message":"Invalid API key."}' }), 'AUTH');
  assert.equal(classifyError({ exitCode: 1, stderr: 'invalid api key provided' }), 'AUTH');
  assert.equal(classifyError({ exitCode: 1, stderr: 'insufficient balance in account' }), 'QUOTA');
  assert.equal(classifyError({ exitCode: 1, stderr: '502 bad gateway' }), 'SERVER');
  assert.equal(classifyError({ exitCode: 1, stderr: 'maximum context length exceeded' }), 'CONTEXT_WINDOW_EXCEEDED');
  assert.equal(classifyError({ exitCode: 1, stderr: 'fetch failed: ECONNREFUSED' }), 'TRANSPORT');
  assert.equal(classifyError({ exitCode: 1, stderr: 'request timed out after 30s' }), 'TIMEOUT');
  assert.equal(classifyError({ exitCode: 1, stderr: '400 bad request: malformed input' }), 'INVALID_REQUEST');
});

test('classifyError: unknown non-zero exit maps to retryable TRANSPORT', () => {
  const code = classifyError({ exitCode: 17, stderr: 'something weird happened' });
  assert.equal(code, 'TRANSPORT');
  assert.ok(isRetryable(code));
});

test('classifyError: status-code patterns are token-boundaried (no "500ms" kills)', () => {
  // Benign numbers that merely LOOK like HTTP codes must not classify.
  assert.equal(classifyError({ exitCode: 0, stderr: 'done in 500ms' }), null);
  assert.equal(classifyError({ exitCode: 0, stderr: 'compiled in 403ms, 4291 files' }), null);
  assert.equal(classifyError({ exitCode: 1, stderr: 'listening on port 4291, then crashed' }), 'TRANSPORT'); // stays "unknown"
  // Real code shapes still classify.
  assert.equal(classifyError({ exitCode: 1, stderr: 'HTTP 500 from upstream' }), 'SERVER');
  assert.equal(classifyError({ exitCode: 1, stderr: 'Error: HTTP 429 too many requests' }), 'RATE_LIMIT');
  assert.equal(classifyError({ exitCode: 1, stderr: '401: {"type":"AuthError"}' }), 'AUTH');
  assert.equal(classifyError({ exitCode: 1, stderr: 'HTTP 403 Forbidden' }), 'QUOTA');
});

test('classifyError: clean exit with clean stderr is not an error', () => {
  assert.equal(classifyError({ exitCode: 0, stderr: '' }), null);
  assert.equal(classifyError({ exitCode: 0, stderr: 'npm warn something benign' }), null);
});

test('classifyError: spawn failure is TRANSPORT', () => {
  assert.equal(classifyError({ spawnError: new Error('spawn pi ENOENT') }), 'TRANSPORT');
});

test('retry policy: exactly EMPTY_RESPONSE/RATE_LIMIT/SERVER/TIMEOUT/TRANSPORT are retryable', () => {
  assert.deepEqual([...RETRYABLE_CODES].sort(), ['EMPTY_RESPONSE', 'RATE_LIMIT', 'SERVER', 'TIMEOUT', 'TRANSPORT'].sort());
  for (const c of ['AUTH', 'QUOTA', 'INVALID_REQUEST', 'CONTEXT_WINDOW_EXCEEDED']) {
    assert.equal(isRetryable(c), false, c);
  }
  for (const c of RETRYABLE_CODES) assert.equal(isRetryable(c), true, c);
});

test('fallback triggers on QUOTA/RATE_LIMIT; AUTH (401) never falls back', () => {
  assert.ok(shouldFallback('QUOTA')); // 403 over-quota / fair-use (drilled 2026-08-20)
  assert.ok(shouldFallback('RATE_LIMIT'));
  assert.ok(!shouldFallback('AUTH')); // 401 invalid key = broken config, no fallback
  assert.ok(!shouldFallback('SERVER'));
  assert.ok(!shouldFallback('TIMEOUT'));
});

test('computeBackoff: exponential 500ms -> 10s cap', () => {
  const random = () => 0.5; // jitter midpoint
  const opts = { baseDelayMs: 500, maxDelayMs: 10_000, jitter: 0.1, random };
  assert.equal(computeBackoff(0, opts), 500);
  assert.equal(computeBackoff(1, opts), 1000);
  assert.equal(computeBackoff(2, opts), 2000);
  assert.equal(computeBackoff(3, opts), 4000);
  assert.equal(computeBackoff(4, opts), 8000);
  assert.equal(computeBackoff(5, opts), 10_000); // capped
  assert.equal(computeBackoff(20, opts), 10_000);
});

test('computeBackoff: jitter stays within +/-10%', () => {
  for (let i = 0; i < 200; i++) {
    const d = computeBackoff(2, { baseDelayMs: 500, maxDelayMs: 10_000, jitter: 0.1 });
    assert.ok(d >= 1800 && d <= 2200, `got ${d}`);
  }
});

test('computeBackoff: Retry-After wins when larger', () => {
  const random = () => 0.5;
  assert.equal(computeBackoff(0, { retryAfterMs: 3000, random }), 3000);
  assert.equal(computeBackoff(3, { retryAfterMs: 1000, random }), 4000); // computed wins when larger
});

test('extractRetryAfterMs parses header-style hints', () => {
  assert.equal(extractRetryAfterMs('HTTP/1.1 429\nRetry-After: 7\n'), 7000);
  assert.equal(extractRetryAfterMs('no hint here'), 0);
});
