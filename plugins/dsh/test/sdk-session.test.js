import test from 'node:test';
import assert from 'node:assert/strict';
import { chmod, mkdir, mkdtemp, readFile, readdir, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { runWorker } from '../src/bridge.js';

const executable = fileURLToPath(new URL('./fixtures/persisted-sdk.js', import.meta.url));
await chmod(executable, 0o755);

test('published SDK preserves project identity while executing and resuming in an independent checkout; probes leave no user history', async t => {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-project-session-')));
  t.after(() => rm(root, { recursive: true, force: true }));
  const project = join(root, 'project'), cwd = join(root, 'unit'), history = join(root, 'history');
  await Promise.all([mkdir(project), mkdir(cwd), mkdir(history)]);
  const calls = join(root, 'calls.jsonl');
  const options = { emit: () => {}, stderr: () => {}, env: { PATH: process.env.PATH, FLYWHEEL_DSH_BIN: executable, TEST_DSH_HISTORY: history, TEST_DSH_LLM_CALLS: calls } };
  const start = { type: 'start', protocol: 2, persistent: true, requestId: 'project-first', cwd, workspaceCwd: project,
    sessionTitle: 'Alert agent failure recovery and routing · execute/M01-T01',
    provider: 'offline-fixture', model: 'test-model', permissionMode: 'danger-full-access', prompt: 'offline', timeoutMs: 10000 };
  const first = await runWorker(start, options);
  assert.equal(first.status, 'ok', JSON.stringify(first));
  const second = await runWorker({ ...start, requestId: 'project-resume', sessionId: first.sessionId, sessionTitle: 'must not overwrite the existing title' }, options);
  assert.equal(second.status, 'ok', JSON.stringify(second));
  const invocations = (await readFile(calls, 'utf8')).trim().split('\n').map(JSON.parse);
  assert.equal(invocations.length, 2);
  for (const call of invocations) assert.deepEqual(call.directories, [{ header: project, current: cwd }]);
  for (const call of invocations) assert.deepEqual(call.titles, [start.sessionTitle]);
  const bad = await runWorker({ ...start, workspaceCwd: cwd, requestId: 'wrong-project', sessionId: first.sessionId }, options);
  assert.equal(bad.status, 'error');
  assert.equal((await readFile(calls, 'utf8')).trim().split('\n').length, 2);
  const before = (await readdir(history, { recursive: true })).sort();
  const { persistent, ...probe } = start;
  const doctor = await runWorker({ ...probe, protocol: 1, requestId: 'probe-project' }, { ...options, doctor: true });
  assert.equal(doctor.status, 'ok', JSON.stringify(doctor));
  assert.deepEqual((await readdir(history, { recursive: true })).sort(), before);
  assert.equal((await readFile(calls, 'utf8')).trim().split('\n').length, 2);
});

test('published SDK refuses an explicit fresh title without its public title service before dispatch', async t => {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-sdk-title-missing-')));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const calls = join(cwd, 'calls.jsonl');
  const result = await runWorker({ type: 'start', protocol: 2, persistent: true, requestId: 'title-required', cwd,
    provider: 'offline-fixture', model: 'test-model', permissionMode: 'read-only', prompt: 'Must not reach the model',
    sessionTitle: 'Alert repository issue · plan review', timeoutMs: 10000 }, {
    emit: () => {}, stderr: () => {}, env: { PATH: process.env.PATH, FLYWHEEL_DSH_BIN: executable,
      TEST_DSH_HISTORY: join(cwd, 'history'), TEST_DSH_LLM_CALLS: calls, TEST_DSH_NO_TITLE_SERVICE: '1' },
  });
  assert.equal(result.status, 'error');
  assert.match(result.error.message, /public sessionTitle service/);
  assert.equal(result.lifecycle.dispatch, 'not-dispatched');
  assert.equal(result.lifecycle.cleanupConfirmed, true);
  await assert.rejects(readFile(calls), { code: 'ENOENT' });
});

test('published SDK cold and prewarmed root streaming reaches the worker throughout one long model attempt', async t => {
  for (const warm of [false, true]) {
    const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-sdk-activity-')));
    t.after(() => rm(cwd, { recursive: true, force: true }));
    const frames = [], controller = new AbortController();
    let idle, dispatch;
    const request = { type: 'start', protocol: 2, persistent: true, requestId: 'streaming-root', cwd,
      provider: 'offline-fixture', model: 'test-model', reasoningEffort: 'high', permissionMode: 'danger-full-access',
      prompt: 'A streaming fixture', timeoutMs: 15000, idleTimeoutMs: 500 };
    const options = { env: { PATH: process.env.PATH, FLYWHEEL_DSH_BIN: executable,
      TEST_DSH_HISTORY: join(cwd, 'history'), TEST_DSH_STREAM_CHUNKS: '12', TEST_DSH_STREAM_DELAY_MS: '100' },
    signal: controller.signal, stderr: () => {}, emit: frame => {
      frames.push(frame);
      // Admission has its own broad startup budget. The short regression window
      // begins at ready, so slow package import is not confused with lost tokens.
      if (frame.type !== 'warm-ready' && frame.type !== 'result') {
        clearTimeout(idle); idle = setTimeout(() => controller.abort(), request.idleTimeoutMs);
      }
    } };
    let input = request;
    if (warm) {
      const receiptDirectory = join(cwd, 'warm'); await mkdir(receiptDirectory, { mode: 0o700 });
      input = { type: 'prepare', protocol: 1, warmId: 'owned-warm-stream', binding: 'a'.repeat(64), ttlMs: 10000, receiptDirectory,
        cwd, provider: request.provider, model: request.model, reasoningEffort: request.reasoningEffort,
        permissionMode: request.permissionMode, idleTimeoutMs: request.idleTimeoutMs };
      options.prewarm = { ready: () => dispatch(request), dispatch: new Promise(resolve => { dispatch = resolve; }) };
    }
    let result;
    try { result = await runWorker(input, options); } finally { clearTimeout(idle); }
    assert.equal(result.status, 'ok', JSON.stringify(result));
    assert.equal(controller.signal.aborted, false);
    const activity = frames.filter(frame => frame.type === 'activity');
    assert.ok(activity.length >= 5, JSON.stringify(activity));
    assert.ok(activity.every(frame => frame.sessionId === result.sessionId && frame.activity.scope === 'root'));
    assert.ok(activity.some(frame => frame.activity.eventTypes.includes('assistant/text-delta')));
    assert.ok(!JSON.stringify(activity).includes('published SDK fixture complete'));
    assert.ok(frames.find(frame => frame.type === 'ready').activity.opaquePhases.includes('before-first-event'));
    assert.equal(result.lifecycle.cleanupConfirmed, true);
  }
});

test('published SDK silent model phase still permits finite idle cancellation with unknown outcome and confirmed cleanup', async t => {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-sdk-silent-')));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const controller = new AbortController(); let idle;
  let result;
  try {
    result = await runWorker({ type: 'start', protocol: 2, persistent: true, requestId: 'silent-root', cwd,
      provider: 'offline-fixture', model: 'test-model', permissionMode: 'danger-full-access',
      prompt: 'A silent fixture', timeoutMs: 15000, idleTimeoutMs: 300 }, {
      env: { PATH: process.env.PATH, FLYWHEEL_DSH_BIN: executable, TEST_DSH_HISTORY: join(cwd, 'history'),
        TEST_DSH_STREAM_CHUNKS: '4', TEST_DSH_STREAM_DELAY_MS: '1000' }, signal: controller.signal, stderr: () => {},
      emit: frame => { if (frame.type !== 'result') { clearTimeout(idle); idle = setTimeout(() => controller.abort(), 300); } },
    });
  } finally { clearTimeout(idle); }
  assert.equal(result.status, 'cancelled');
  assert.equal(result.lifecycle.dispatch, 'dispatched');
  assert.equal(result.lifecycle.outcome, 'unknown');
  assert.equal(result.lifecycle.cleanupConfirmed, true);
});

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
