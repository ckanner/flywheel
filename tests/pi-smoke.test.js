import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { runPi, parsePiEventStream, buildChildEnv } from '../orchestrator/drivers/pi.js';

const HAS_PI = (() => {
  try {
    execFileSync('pi', ['--version'], { stdio: 'pipe' });
    return true;
  } catch {
    return false;
  }
})();
const HAS_KEY = Boolean(process.env.OPENCODE_API_KEY);
const SKIP = !HAS_PI || !HAS_KEY;

test('pi driver: event stream parser aggregates usage from message_end (recorded sample)', () => {
  const sample = [
    JSON.stringify({ type: 'session', version: 3, id: 'x' }),
    JSON.stringify({ type: 'agent_start' }),
    JSON.stringify({ type: 'message_end', message: { role: 'user', content: [] } }),
    JSON.stringify({
      type: 'message_end',
      message: {
        role: 'assistant',
        provider: 'opencode-go',
        model: 'deepseek-v4-flash',
        content: [
          { type: 'thinking', thinking: 'abc' },
          { type: 'text', text: 'ok' },
        ],
        usage: { input: 2045, output: 15, cacheRead: 100, cacheWrite: 20, reasoning: 13, totalTokens: 2060, cost: { total: 0.0003 } },
      },
    }),
    JSON.stringify({ type: 'agent_settled' }),
    'not-json noise is tolerated',
  ].join('\n');
  const r = parsePiEventStream(sample);
  assert.equal(r.settled, true);
  assert.equal(r.text, 'ok');
  assert.equal(r.provider, 'opencode-go');
  assert.equal(r.model, 'deepseek-v4-flash');
  assert.deepEqual(r.usage, { input: 2045, output: 15, reasoning: 13, cacheRead: 100, cacheWrite: 20, totalTokens: 2060 });
  assert.equal(r.reasoningChars, 3);
  assert.equal(r.costUsd, 0.0003);
});

test('pi driver: missing reasoning breakdown falls back without crashing (附录 D)', () => {
  const sample = JSON.stringify({
    type: 'message_end',
    message: {
      role: 'assistant',
      content: [{ type: 'thinking', thinking: 'x'.repeat(500) }, { type: 'text', text: 'ok' }],
      usage: { input: 100, output: 50, totalTokens: 150 }, // no reasoning/cache fields at max tier
    },
  });
  const r = parsePiEventStream(sample);
  assert.equal(r.usage.reasoning, 0); // absent -> 0, never assumed present
  assert.equal(r.reasoningChars, 500); // fallback evidence retained
  assert.equal(r.costUsd, null);
});

test('pi driver: child env is minimal (credential hygiene §9)', () => {
  const env = buildChildEnv({ PATH: '/bin', HOME: '/home/x', AWS_SECRET_ACCESS_KEY: 'nope', OPENCODE_API_KEY: 'k' }, { OPENCODE_API_KEY: 'k' });
  assert.equal(env.PATH, '/bin');
  assert.equal(env.OPENCODE_API_KEY, 'k');
  assert.equal(env.AWS_SECRET_ACCESS_KEY, undefined); // never forwarded
});

test('pi smoke (real): tiny prompt, event stream parsed, usage extracted', { timeout: 300_000, skip: SKIP && 'pi or OPENCODE_API_KEY unavailable' }, async () => {
  const cwd = mkdtempSync(join(tmpdir(), 'flywheel-pi-smoke-'));
  const r = await runPi({
    prompt: 'Reply with exactly the word: ok',
    model: 'opencode-go/deepseek-v4-flash',
    thinking: 'max',
    cwd,
    apiKey: process.env.OPENCODE_API_KEY ?? null,
    watchdogOpts: { idleTimeoutMs: 120_000, wallTimeoutMs: 280_000, killGraceMs: 3000 },
  });
  console.log('[pi-smoke]', JSON.stringify({
    exitCode: r.exitCode, settled: r.settled, usage: r.usage,
    reasoningChars: r.reasoningChars, costUsd: r.costUsd, durationMs: r.durationMs, text: r.text,
  }));
  assert.equal(r.exitCode, 0, `stderr: ${r.stderr.slice(0, 500)}`);
  assert.equal(r.timedOut, false);
  assert.equal(r.settled, true); // saw agent_settled
  assert.ok(r.usage.input > 0);
  assert.ok(r.usage.output > 0);
  assert.match(r.text, /ok/i);
});
