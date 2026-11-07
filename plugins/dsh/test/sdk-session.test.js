import test from 'node:test';
import assert from 'node:assert/strict';
import { chmod, mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { runWorker } from '../src/bridge.js';

const executable = fileURLToPath(new URL('./fixtures/persisted-sdk.js', import.meta.url));
await chmod(executable, 0o755);

test('published SDK restores a persisted Agent in a new process without replaying historical usage', async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-public-sdk-resume-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const calls = join(cwd, 'model-calls.jsonl');
  const frames = [];
  const diagnostics = [];
  const options = {
    env: { PATH: process.env.PATH, FLYWHEEL_DSH_BIN: executable,
      TEST_DSH_HISTORY: join(cwd, 'history'), TEST_DSH_LLM_CALLS: calls },
    emit: frame => frames.push(frame), stderr: chunk => diagnostics.push(String(chunk)),
  };
  const start = { type: 'start', protocol: 2, persistent: true, requestId: 'first', cwd,
    provider: 'offline-fixture', model: 'test-model', reasoningEffort: 'high',
    permissionMode: 'danger-full-access', prompt: 'Implement FIRST_CONTRACT_MARKER.', timeoutMs: 10000 };
  const first = await runWorker(start, options);
  assert.equal(first.status, 'ok', JSON.stringify({ first, diagnostics }));
  assert.equal(first.lifecycle.resumeAccepted, false);
  assert.equal(first.lifecycle.resumeCandidate, true);
  assert.equal(first.lifecycle.cleanupConfirmed, true);
  assert.equal(first.usage.total_tokens, 12);
  const firstId = first.sessionId;
  frames.length = 0;
  const second = await runWorker({ ...start, requestId: 'second', sessionId: firstId,
    prompt: 'Repair with CURRENT_FAILURE_MARKER and inspect the complete current candidate.' }, options);
  assert.equal(second.status, 'ok', JSON.stringify({ second, diagnostics }));
  assert.equal(second.sessionId, firstId);
  assert.equal(second.lifecycle.resumeAccepted, true);
  assert.equal(second.lifecycle.cleanupConfirmed, true);
  assert.equal(second.lifecycle.usageStatus, 'complete');
  assert.equal(second.usage.total_tokens, 12, 'only the current model call is charged after restoration');
  assert.notEqual(second.lifecycle.turnId, first.lifecycle.turnId, 'the persisted Agent advances its own turn identity');
  assert.equal(frames.filter(frame => frame.event?.type === 'assistant/message').length, 1,
    'the SDK emits only new committed assistant notifications');
  const invocations = (await readFile(calls, 'utf8')).trim().split('\n').map(JSON.parse);
  assert.equal(invocations.length, 2, 'restore and shutdown do not submit an auxiliary model call');
  assert.match(JSON.stringify(invocations[1].messages), /FIRST_CONTRACT_MARKER/);
  assert.match(JSON.stringify(invocations[1].messages), /published SDK fixture complete/);
  assert.match(JSON.stringify(invocations[1].messages), /CURRENT_FAILURE_MARKER/);
  assert.equal(invocations[1].provider, 'offline-fixture');
  assert.equal(invocations[1].model, 'test-model');
  assert.equal(invocations[1].reasoningEffort, 'high');

  const missing = await runWorker({ ...start, requestId: 'missing', sessionId: 'does-not-exist' }, options);
  assert.equal(missing.status, 'error');
  assert.equal(missing.lifecycle.dispatch, 'not-dispatched');
  assert.equal(missing.lifecycle.outcome, 'unknown', 'generic restore errors do not authorize a replacement');
  assert.equal(missing.lifecycle.resumeCandidate, false);
  assert.equal((await readFile(calls, 'utf8')).trim().split('\n').length, 2,
    'missing history never causes a fresh model turn');
});
