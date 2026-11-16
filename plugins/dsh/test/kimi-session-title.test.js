import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { prepareKimiPrintSession } from '../src/kimi-print-session.js';
import { runKimiWorker } from '../src/kimi-native-bridge.js';
import { OwnedKimiRuntime } from '../src/kimi-native-runtime.js';

const command = fileURLToPath(new URL('./fixtures/kimi-native-server.mjs', import.meta.url));
async function setup(t, fixtureCase = 'normal') {
  const root = await mkdtemp(join(tmpdir(), 'flywheel-kimi-title-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const cwd = join(root, 'project'); await mkdir(cwd);
  const env = { ...process.env, KIMI_CODE_HOME: join(root, 'home'), FLYWHEEL_KIMI_BIN: command,
    KIMI_FIXTURE_LOG: join(root, 'requests.jsonl'), KIMI_FIXTURE_CASE: fixtureCase };
  const input = { protocol: 1, type: 'prepare-print', requestId: 'invocation-one', cwd, model: 'kimi-model', reasoningEffort: 'high',
    sessionTitle: 'Flywheel · design-plan · 排查告警重复发送', timeoutMs: 10_000 };
  const log = async () => (await readFile(env.KIMI_FIXTURE_LOG, 'utf8').catch(() => '')).trim().split('\n').filter(Boolean).map(JSON.parse);
  return { input, env, log };
}

test('a new print history has its task title before any inference and closes its owner', async t => {
  const f = await setup(t), result = await prepareKimiPrintSession(f.input, { env: f.env });
  assert.equal(result.status, 'ok', JSON.stringify(result));
  assert.equal(result.sessionTitle, f.input.sessionTitle);
  assert.equal(result.cleanupConfirmed, true);
  assert.equal(result.inferenceDispatched, false);
  const calls = await f.log(), created = calls.find(row => row.path === '/api/v1/sessions' && row.method === 'POST');
  assert.equal(created.body.title, f.input.sessionTitle);
  assert.deepEqual(created.body.agent_config, { model: 'kimi-model', permission_mode: 'auto', plan_mode: false });
  assert.equal(created.body.environment_id, undefined, 'preserve native configured environment defaults');
  assert.ok(calls.some(row => row.path === '/api/v1/shutdown'));
  assert.ok(!calls.some(row => row.path?.endsWith('/prompts') || row.path?.endsWith('/title/generate')));
  assert.doesNotMatch(JSON.stringify(result), /fixture-secret/);
});

test('a real absolute workspace path longer than 512 characters can prepare a named print history', async t => {
  const f = await setup(t);
  const cwd = join(f.input.cwd, ...Array.from({ length: 5 }, (_, index) => `workspace-${index}-` + 'x'.repeat(110)));
  await mkdir(cwd, { recursive: true });
  const physical = await realpath(cwd);
  assert.ok(physical.length > 512 && physical.length < 4096);
  const result = await prepareKimiPrintSession({ ...f.input, cwd }, { env: f.env });
  assert.equal(result.status, 'ok', JSON.stringify(result));
  assert.equal(result.cleanupConfirmed, true);
  assert.equal(result.inferenceDispatched, false);
  assert.equal(result.sessionTitle, f.input.sessionTitle);
  const calls = await f.log(), created = calls.find(row => row.path === '/api/v1/sessions' && row.method === 'POST');
  assert.equal(created.body.metadata.cwd, physical);
  assert.ok(calls.some(row => row.path === '/api/v1/shutdown'));
  assert.ok(!calls.some(row => row.path?.endsWith('/prompts')));
});

for (const cleanupFailure of ['unconfirmed', 'throws']) {
  test(`the 1ms preparation deadline retains TIMEOUT when cleanup ${cleanupFailure}`, { timeout: 15000 }, async t => {
    const f = await setup(t), originalStop = OwnedKimiRuntime.prototype.stop;
    const physicalStops = [];
    t.mock.method(OwnedKimiRuntime.prototype, 'stop', async function () {
      // Always physically stop any real fixture process. Fault injection only
      // removes the evidence available to the caller; it never fabricates a
      // cleanup success or leaves an unowned process running after the test.
      const stopped = await originalStop.call(this);
      physicalStops.push(stopped);
      if (cleanupFailure === 'throws') throw new Error('injected cleanup inspection failure');
      return { ...stopped, processesConfirmed: false };
    });
    const result = await prepareKimiPrintSession({ ...f.input, timeoutMs: 1 }, { env: f.env });
    assert.equal(physicalStops.length, 1);
    assert.equal(physicalStops[0].exited, true);
    assert.equal(physicalStops[0].processesConfirmed, true);
    assert.equal(result.status, 'error', JSON.stringify(result));
    assert.equal(result.error.code, 'TIMEOUT');
    assert.equal(result.cleanupConfirmed, false);
    assert.equal(result.inferenceDispatched, false);
    assert.ok(!(await f.log()).some(row => row.path?.endsWith('/prompts')));
    assert.doesNotMatch(JSON.stringify(result), /injected cleanup inspection failure|fixture-secret/);
  });
}

test('a real caller cancellation remains cancelled when cleanup evidence is unavailable', { timeout: 15000 }, async t => {
  const f = await setup(t), controller = new AbortController(), originalStop = OwnedKimiRuntime.prototype.stop;
  const physicalStops = [];
  t.mock.method(OwnedKimiRuntime.prototype, 'stop', async function () {
    controller.abort(new Error('caller requested stop'));
    const stopped = await originalStop.call(this);
    physicalStops.push(stopped);
    return { ...stopped, processesConfirmed: false };
  });
  const result = await prepareKimiPrintSession(f.input, { env: f.env, signal: controller.signal });
  assert.equal(physicalStops.length, 1);
  assert.equal(physicalStops[0].exited, true);
  assert.equal(physicalStops[0].processesConfirmed, true);
  assert.equal(result.status, 'cancelled', JSON.stringify(result));
  assert.equal(result.cleanupConfirmed, false);
  assert.equal(result.inferenceDispatched, false);
  assert.notEqual(result.error.code, 'TIMEOUT');
});

for (const fixtureCase of ['no-title-schema', 'wrong-title']) {
  test('unconfirmed naming cannot silently become an unnamed print dispatch: ' + fixtureCase, async t => {
    const f = await setup(t, fixtureCase), result = await prepareKimiPrintSession(f.input, { env: f.env });
    assert.equal(result.status, 'error');
    assert.equal(result.inferenceDispatched, false);
    assert.equal(result.cleanupConfirmed, true);
    assert.ok(!(await f.log()).some(row => row.path?.endsWith('/prompts')));
  });
}

test('print preparation rejects prompts, resume handles and invalid titles before starting a runtime', async t => {
  const f = await setup(t);
  for (const mutation of [{ prompt: 'must never be sent' }, { sessionId: 'existing-user-session' }, { sessionTitle: 'bad\nname' }, { sessionTitle: 'x'.repeat(201) },
    { cwd: '' }, { cwd: 'relative/path' }, { cwd: '/bad\0path' }, { cwd: '/' + 'x'.repeat(4096) },
    { requestId: 'x'.repeat(513) }, { model: 'x'.repeat(513) }]) {
    const result = await prepareKimiPrintSession({ ...f.input, ...mutation }, { env: f.env });
    assert.equal(result.status, 'error');
    assert.equal(result.cleanupConfirmed, true);
  }
  const controller = new AbortController(); controller.abort();
  const cancelled = await prepareKimiPrintSession(f.input, { env: f.env, signal: controller.signal });
  assert.equal(cancelled.status, 'cancelled');
  assert.equal(cancelled.inferenceDispatched, false);
  assert.deepEqual(await f.log(), []);
});

test('native Kimi receives the full task title and resume never renames history', async t => {
  const f = await setup(t);
  const input = { ...f.input, type: 'start', persistent: true, prompt: 'bound work', permissionMode: 'danger-full-access',
    native: { version: 1, mode: 'agent' }, managed: { version: 1, runId: '', invocationId: f.input.requestId,
      role: 'design-plan', scope: 'planning:opaque-id', channel: 'kimi', taskDir: f.input.cwd } };
  const first = await runKimiWorker(input, { env: f.env });
  assert.equal(first.status, 'ok', JSON.stringify(first.error));
  const second = await runKimiWorker({ ...input, sessionId: first.sessionId, sessionTitle: 'A different proposed title' }, { env: f.env });
  assert.equal(second.status, 'ok', JSON.stringify(second.error));
  const calls = await f.log();
  const creates = calls.filter(row => row.path === '/api/v1/sessions' && row.method === 'POST');
  assert.equal(creates.length, 1);
  assert.equal(creates[0].body.title, f.input.sessionTitle);
  assert.ok(!calls.some(row => row.path?.endsWith('/profile') && row.body?.title));
});
