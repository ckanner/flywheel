import test from 'node:test';
import assert from 'node:assert/strict';
import { chmod, mkdtemp, mkdir, readFile, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { runWorker, makeOverlay } from '../src/bridge.js';
import { validateWebRequest, validateWebBinding, validateWebFacts } from '../src/web-policy.js';

const executable = fileURLToPath(new URL('./fixtures/persisted-sdk.js', import.meta.url));
await chmod(executable, 0o755);
const binding = { protocol: 1, transport: 'chatgpt-web', provider: 'offline-fixture', model: 'gpt-6-pro-web', reasoningEffort: 'pro',
  connectionId: 'offline-connection', accountFingerprint: 'offline-account', policyVersion: 'fixture-policy/1', contextPolicy: 'project-only',
  observation: 'opaque', usage: 'unknown', hostContractVersion: 1, runtimeVersion: 'web-runtime/1' };
async function setup(t, scenario = 'ok') {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-web-sdk-')));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const calls = join(cwd, 'calls.jsonl'), frames = [];
  const request = { type: 'start', protocol: 2, persistent: true, requestId: 'web-host-invocation', cwd,
    provider: 'offline-fixture', model: 'gpt-6-pro-web', reasoningEffort: 'pro', permissionMode: 'danger-full-access',
    prompt: 'Public SDK Web contract fixture', timeoutMs: 15000, idleTimeoutMs: 20,
    web: { version: 1, executionDomainId: 'persistent-task-domain', requireIsolation: true } };
  const options = { emit: frame => frames.push(frame), stderr: () => {}, env: { PATH: process.env.PATH,
    FLYWHEEL_DSH_BIN: executable, TEST_DSH_WEB_SCENARIO: scenario, TEST_DSH_HISTORY: join(cwd, 'history'), TEST_DSH_LLM_CALLS: calls } };
  return { cwd, calls, frames, request, options };
}

test('Web extension is strict, preserves CompactionEngine, and cannot be requested for another route or effort', () => {
  const request = { provider: 'offline-fixture', model: 'gpt-6-pro-web', reasoningEffort: 'pro', web: { version: 1, executionDomainId: 'task', requireIsolation: true } };
  validateWebRequest(request);
  assert.deepEqual(validateWebBinding(binding, request), binding);
  assert.throws(() => validateWebBinding({ ...binding, contextPolicy: 'ordinary' }, request));
  assert.throws(() => validateWebBinding({ ...binding, additional: true }, request));
  for (const changed of [{ model: 'ordinary' }, { reasoningEffort: 'high' }, { web: undefined },
    { web: { ...request.web, extra: true } }, { web: { ...request.web, requireIsolation: false } },
    { web: { ...request.web, probe: false } }, { web: { ...request.web, expectedBinding: { ...binding, model: 'ordinary' } } }]) {
    assert.throws(() => validateWebRequest({ ...request, ...changed }));
  }
  validateWebRequest({ model: 'ordinary' });
  const overlays = makeOverlay({ ...request, cwd: '/repo', permissionMode: 'danger-full-access' }, undefined, { requestId: 'r', nonce: 'n', receiptPath: '/tmp/policy' });
  assert.ok(!overlays.some(row => row.id === 'compaction-basic' && row.disabled));
  const identity = { hostInvocationId: 'call', executionDomainId: 'task', sessionId: 'session' };
  const facts = { version: 1, ...identity, dispatch: 'dispatched', outcome: 'unknown', remoteGeneration: 'unknown', localCleanupConfirmed: true, usageStatus: 'unknown', invocations: [{ private: 'not forwarded' }] };
  assert.equal(validateWebFacts(facts, identity).outcome, 'unknown');
  assert.ok(!('invocations' in validateWebFacts(facts, identity)));
  for (const changed of [{ hostInvocationId: 'old' }, { outcome: 'completed' }, { usageStatus: 'complete' }, { localCleanupConfirmed: 'yes' }]) assert.throws(() => validateWebFacts({ ...facts, ...changed }, identity));
});

test('real published SDK cold and prewarmed calls attest Web identity and preserve unknown usage', async t => {
  for (const warm of [false, true]) {
    const f = await setup(t);
    let input = f.request;
    if (warm) {
      const receiptDirectory = join(f.cwd, 'warm'); await mkdir(receiptDirectory, { mode: 0o700 });
      input = { type: 'prepare', protocol: 1, warmId: 'web-warm-fixture', binding: 'a'.repeat(64), ttlMs: 10000, receiptDirectory,
        cwd: f.cwd, provider: f.request.provider, model: f.request.model, reasoningEffort: 'pro', permissionMode: 'danger-full-access', idleTimeoutMs: 20, web: f.request.web };
      let dispatch;
      f.options.prewarm = { ready: () => dispatch(f.request), dispatch: new Promise(resolve => { dispatch = resolve; }) };
    }
    const first = await runWorker(input, f.options);
    assert.equal(first.status, 'ok', JSON.stringify(first));
    assert.equal(first.usage, null);
    assert.equal(first.costUsd, null);
    assert.deepEqual(first.web.binding, binding);
    assert.equal(first.web.facts.hostInvocationId, f.request.requestId);
    assert.equal(first.web.facts.executionDomainId, f.request.web.executionDomainId);
    assert.equal(first.web.facts.outcome, 'completed');
    assert.equal(first.lifecycle.usageStatus, 'unknown');
    assert.ok(f.frames.some(row => row.type === 'activity' && row.activity.eventTypes.includes('provider/chatgpt-web')));
    const resumed = await runWorker({ ...f.request, requestId: 'web-next-invocation', sessionId: first.sessionId,
      web: { ...f.request.web, expectedBinding: first.web.binding } }, { ...f.options, prewarm: undefined });
    assert.equal(resumed.status, 'ok', JSON.stringify(resumed));
    assert.equal(resumed.sessionId, first.sessionId);
    assert.equal(resumed.web.facts.hostInvocationId, 'web-next-invocation');
    assert.equal((await readFile(f.calls, 'utf8')).trim().split('\n').length, 2);
  }
});

for (const scenario of ['missing-host', 'unsupported-effort', 'account-change']) {
  test(`Web ${scenario} fails before prompt dispatch`, async t => {
    const f = await setup(t, scenario);
    f.request.web.expectedBinding = binding;
    const result = await runWorker(f.request, f.options);
    assert.equal(result.status, 'error', JSON.stringify(result));
    assert.equal(result.lifecycle.dispatch, 'not-dispatched');
    await assert.rejects(readFile(f.calls), { code: 'ENOENT' });
  });
}

for (const scenario of ['unknown', 'unclean']) {
  test(`Web ${scenario} cannot become a reusable completed Session`, async t => {
    const f = await setup(t, scenario);
    const result = await runWorker(f.request, f.options);
    assert.equal(result.status, 'error', JSON.stringify(result));
    assert.equal(result.lifecycle.resumeCandidate, false);
    if (scenario === 'unknown') { assert.equal(result.lifecycle.outcome, 'unknown'); assert.equal(result.settled, true); }
    else assert.equal(result.settled, false);
    assert.equal(result.usage, null);
  });
}

test('Web doctor probes only admission and cannot submit or fabricate a completed generation', async t => {
  const f = await setup(t);
  const { persistent, ...request } = f.request;
  const result = await runWorker({ ...request, protocol: 1, web: { ...request.web, probe: true } }, { ...f.options, doctor: true });
  assert.equal(result.status, 'ok', JSON.stringify(result));
  assert.equal(result.web.facts.dispatch, 'not-dispatched');
  assert.equal(result.web.facts.outcome, 'failed');
  assert.equal(result.web.facts.remoteGeneration, 'idle');
  assert.equal(result.usage, null);
  await assert.rejects(readFile(f.calls), { code: 'ENOENT' });
  const rejected = await runWorker({ ...request, protocol: 1, web: { ...request.web, probe: true } }, f.options);
  assert.equal(rejected.status, 'error');
});
