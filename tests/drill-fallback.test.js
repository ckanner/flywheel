import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { runPi } from '../orchestrator/drivers/pi.js';
import { classifyError, shouldFallback, isRetryable } from '../orchestrator/core/errors.js';

// 403 fallback drill (DESIGN.md §10 gate 4, reachable part): how does a bad
// OpenCode Go key actually surface through pi, and does our classification
// route it correctly? Full transcript and findings: drills/403-fallback.md.

const HAS_PI = (() => {
  try {
    execFileSync('pi', ['--version'], { stdio: 'pipe' });
    return true;
  } catch {
    return false;
  }
})();

test('drill (real): invalid OPENCODE_API_KEY surfaces INSIDE the pi event stream as 401', { timeout: 300_000, skip: !HAS_PI && 'pi unavailable' }, async () => {
  const cwd = mkdtempSync(join(tmpdir(), 'flywheel-drill-401-'));
  const r = await runPi({
    prompt: 'Reply with exactly: ok',
    model: 'opencode-go/deepseek-v4-flash',
    thinking: 'max',
    cwd,
    apiKey: 'sk-invalid-drill-0000000000000000',
    watchdogOpts: { idleTimeoutMs: 120_000, wallTimeoutMs: 280_000, killGraceMs: 3000 },
  });
  console.log('[drill-401]', JSON.stringify({ exitCode: r.exitCode, errorMessage: r.errorMessage, settled: r.settled }));
  // pi exits 0 even though the provider rejected the key — the error lives in
  // the stream (stopReason:"error"). The exit contract alone would misread
  // this as success; the driver must surface errorMessage.
  assert.equal(r.exitCode, 0);
  assert.ok(r.errorMessage, 'errorMessage captured from the stream');
  assert.match(r.errorMessage, /401/);

  const code = classifyError({ exitCode: 1, stderr: r.errorMessage });
  assert.equal(code, 'AUTH');
  assert.equal(shouldFallback(code), false, '401 invalid key must NOT fall back');
  assert.equal(isRetryable(code), false, '401 is not retried either');
});

test('drill (classification): a Go 403 (over-quota / fair-use) classifies as QUOTA and falls back', () => {
  for (const text of [
    '403: {"type":"QuotaError","message":"fair use limit reached"}',
    'Error 403: forbidden',
    'HTTP 403 Forbidden: subscription quota exceeded',
  ]) {
    const code = classifyError({ exitCode: 1, stderr: text });
    assert.equal(code, 'QUOTA', text);
    assert.equal(shouldFallback(code), true, `should fall back: ${text}`);
    assert.equal(isRetryable(code), false, 'quota is not retried on the same channel');
  }
});

test('drill (classification): 401 and 403 never get conflated', () => {
  assert.equal(classifyError({ exitCode: 1, stderr: '401: {"type":"AuthError","message":"Invalid API key."}' }), 'AUTH');
  assert.equal(classifyError({ exitCode: 1, stderr: '403: forbidden' }), 'QUOTA');
});
