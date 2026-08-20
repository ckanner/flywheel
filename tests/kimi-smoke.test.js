import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { runKimi, parseKimiStreamJson } from '../orchestrator/drivers/kimi.js';

const HAS_KIMI = (() => {
  try {
    execFileSync('kimi', ['--version'], { stdio: 'pipe' });
    return true;
  } catch {
    return false;
  }
})();

test('kimi driver: stream-json parser (recorded sample)', () => {
  const sample = [
    JSON.stringify({ role: 'meta', type: 'system.version', version: '0.37.2' }),
    JSON.stringify({ role: 'assistant', content: 'ok' }),
    JSON.stringify({ role: 'meta', type: 'session.resume_hint', session_id: 'session_abc-123', command: 'kimi -r session_abc-123' }),
  ].join('\n');
  const r = parseKimiStreamJson(sample);
  assert.equal(r.sessionId, 'session_abc-123');
  assert.equal(r.text, 'ok');
  assert.equal(r.usage, null); // CLI stream exposes no usage (0.37.2)
  assert.equal(r.errorMessage, null);
});

test('kimi smoke (real): create session -> resume -> same session id reused', { timeout: 600_000, skip: !HAS_KIMI && 'kimi CLI unavailable' }, async () => {
  const cwd = mkdtempSync(join(tmpdir(), 'flywheel-kimi-smoke-'));
  const watchdogOpts = { idleTimeoutMs: 180_000, wallTimeoutMs: 280_000, killGraceMs: 3000 };

  const first = await runKimi({ prompt: 'Reply with exactly the word: ok', cwd, watchdogOpts });
  console.log('[kimi-smoke] create:', JSON.stringify({ exitCode: first.exitCode, sessionId: first.sessionId, text: first.text, durationMs: first.durationMs }));
  assert.equal(first.exitCode, 0, `stderr: ${first.stderr.slice(0, 300)}`);
  assert.equal(first.timedOut, false);
  assert.ok(first.sessionId, 'session id captured from resume hint');
  assert.match(first.text, /ok/i);

  const second = await runKimi({
    prompt: 'What single word did I ask you to reply with? Answer with just that word.',
    sessionId: first.sessionId,
    cwd,
    watchdogOpts,
  });
  console.log('[kimi-smoke] resume:', JSON.stringify({ exitCode: second.exitCode, sessionId: second.sessionId, text: second.text, durationMs: second.durationMs }));
  assert.equal(second.exitCode, 0, `stderr: ${second.stderr.slice(0, 300)}`);
  assert.equal(second.sessionId, first.sessionId, 'resident session id reused across calls');
  assert.match(second.text, /ok/i); // the resumed session remembers the context
});
