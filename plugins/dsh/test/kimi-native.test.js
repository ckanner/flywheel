import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtemp, mkdir, readFile, writeFile, rm, stat, realpath, symlink } from 'node:fs/promises';
import { spawn } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { runKimiWorker, validateKimiNativeStart, KimiConversation, kimiObservedUsage, createKimiActivityReporter } from '../src/kimi-native-bridge.js';
import { towerGit, towerSnapshot } from '../src/kimi-tower-candidate.js';

const server = fileURLToPath(new URL('./fixtures/kimi-native-server.mjs', import.meta.url));
const cli = fileURLToPath(new URL('../src/kimi-native-worker.js', import.meta.url));
const directories = [];
afterEach(async () => { await Promise.all(directories.splice(0).map(path => rm(path, { recursive: true, force: true }))); });
async function fixture(mode = 'agent', fixtureCase = 'normal') {
  const root = await mkdtemp(join(tmpdir(), 'kimi-native-test-')); directories.push(root);
  const cwd = join(root, 'work'), taskDir = join(cwd, 'task'), home = join(root, 'account');
  await mkdir(taskDir, { recursive: true });
  const input = { type: 'start', protocol: 1, requestId: 'request-1', cwd, model: 'kimi-for-coding', reasoningEffort: 'high',
    permissionMode: 'danger-full-access', prompt: 'Perform the bound role', timeoutMs: 15_000, native: { version: 1, mode },
    managed: { version: 1, runId: '', invocationId: 'request-1', role: 'execute', scope: 'candidate', candidateId: 'candidate-1', channel: 'kimi', taskDir } };
  const env = { ...process.env, KIMI_CODE_HOME: home, FLYWHEEL_KIMI_BIN: server, KIMI_FIXTURE_CASE: fixtureCase, KIMI_FIXTURE_LOG: join(root, 'requests.jsonl') };
  const frames = [], stderr = [];
  return { root, input, env, frames, stderr, options: { env, emit: frame => frames.push(frame), stderr: line => stderr.push(line) },
    log: async () => (await readFile(env.KIMI_FIXTURE_LOG, 'utf8')).trim().split('\n').map(line => JSON.parse(line)) };
}

test('Agent and Swarm use the actual owned REST/WS session, preserve account/model, and do not leak the bearer banner', async () => {
  for (const mode of ['agent', 'swarm']) {
    const f = await fixture(mode);
    const result = await runKimiWorker(f.input, f.options);
    assert.equal(result.status, 'ok', JSON.stringify(result.error));
    assert.equal(result.native.effective.matched, true);
    assert.deepEqual(result.native.completion, { confirmed: true, pendingIds: [] });
    assert.deepEqual(result.native.requested, f.input.native);
    assert.equal(result.native.effective.swarm, mode === 'swarm');
    assert.equal(result.native.effective.model, 'kimi-for-coding');
    assert.equal(result.native.effective.concurrency.upperBound, null);
    assert.equal(result.lifecycle.dispatch, 'dispatched');
    assert.equal(result.lifecycle.cleanupConfirmed, true);
    assert.equal(result.lifecycle.turnId, 'turn-1');
    assert.deepEqual(result.usage, { input: 16, output: 7, cacheRead: 3, cacheWrite: 2, totalTokens: 23 });
    assert.equal(result.costUsd, null);
    assert.equal(f.frames.filter(frame => frame.type === 'ready').length, 1);
    assert.equal(f.frames.filter(frame => frame.type === 'result').length, 0, 'only CLI owns final emission');
    assert.doesNotMatch(JSON.stringify([result, f.frames, f.stderr]), /fixture-secret/);
    const log = await f.log();
    assert.equal(log.filter(row => row.path?.endsWith('/prompts') && row.method === 'POST').length, 1);
    assert.deepEqual(log.find(row => row.path?.endsWith('/prompts') && row.method === 'POST').body.disabled_tools, ['CronCreate']);
  }
});

test('subscription-only native Kimi validates the owned account/config and disables ambiguous service routes per prompt', async () => {
  const f = await fixture();
  f.input.subscription = { version: 1, mode: 'subscription-only' };
  f.input.native.agents = { enabled: false };
  const result = await runKimiWorker(f.input, f.options);
  assert.equal(result.status, 'ok', JSON.stringify(result.error));
  assert.equal(result.native.observed.subscription.authentication, 'native-oauth');
  assert.equal(result.native.observed.subscription.extraUsage, 'unknown');
  const calls = await f.log();
  const prompt = calls.find(row => row.method === 'POST' && row.path.endsWith('/prompts'));
  assert.deepEqual(prompt.body.disabled_tools, ['CronCreate', 'Agent', 'AgentSwarm', 'WebSearch', 'FetchURL']);
  assert.equal(calls.filter(row => row.path === '/api/v1/providers').length, 3);
  assert.ok(calls.findIndex(row => row.path === '/api/v1/providers') < calls.indexOf(prompt));
  assert.ok(calls.findIndex(row => row.path === '/api/v1/config') < calls.indexOf(prompt));
  assert.doesNotMatch(JSON.stringify(result), /fixture-secret/);
});

for (const fixtureCase of ['subscription-api', 'subscription-endpoint', 'subscription-model-key', 'subscription-drift']) {
test(`${fixtureCase} blocks Kimi before prompt and confirms owned runtime cleanup`, async () => {
  const f = await fixture('agent', fixtureCase);
  f.input.subscription = { version: 1, mode: 'subscription-only' };
  f.input.native.agents = { enabled: false };
  const result = await runKimiWorker(f.input, f.options);
  assert.equal(result.status, 'error');
  assert.equal(result.error.code, 'AUTH');
  assert.equal(result.lifecycle.dispatch, 'not-dispatched');
  assert.equal(result.lifecycle.cleanupConfirmed, true);
  assert.ok(!(await f.log()).some(row => row.method === 'POST' && row.path.endsWith('/prompts')));
});
}

test('subscription-only cannot silently admit Swarm, Tower or unrestricted native children', async () => {
  const f = await fixture();
  f.input.subscription = { version: 1, mode: 'subscription-only' };
  for (const native of [{ version: 1, mode: 'swarm' }, { version: 1, mode: 'agent' }, { version: 1, mode: 'tower' },
    { version: 1, mode: 'agent', agents: { enabled: false }, agentProfile: 'custom' }]) {
    assert.throws(() => validateKimiNativeStart({ ...f.input, native }), { code: 'INVALID_REQUEST' });
  }
});

test('cancelled subscription lookup stops the actual owned Kimi runtime without sending a prompt', async () => {
  const f = await fixture('agent', 'subscription-hang');
  f.input.subscription = { version: 1, mode: 'subscription-only' };
  f.input.native.agents = { enabled: false };
  f.input.timeoutMs = 500;
  const result = await runKimiWorker(f.input, f.options);
  assert.equal(result.error.code, 'TIMEOUT');
  assert.equal(result.lifecycle.dispatch, 'not-dispatched');
  assert.equal(result.lifecycle.cleanupConfirmed, true);
  assert.ok(!(await f.log()).some(row => row.method === 'POST' && row.path.endsWith('/prompts')));
});

test('native child identity and complete step usage are reconciled before success', async () => {
  const f = await fixture('swarm', 'child');
  const result = await runKimiWorker(f.input, f.options);
  assert.equal(result.status, 'ok', JSON.stringify(result.error));
  assert.equal(result.native.children[0].agentId, 'child-1');
  assert.equal(result.native.children[0].model, 'native-child-model');
  assert.equal(result.usage.totalTokens, 30);
  assert.equal(result.native.children[0].status, 'ended');
  assert.equal(result.native.observed.usage.observedSteps.length, 2);
});

test('an unclaimed fresh session can initialize through profile only with native lifecycle admission, and resume never reclaims it', async () => {
  const f = await fixture('swarm', 'create-ignores-configuration');
  f.input.persistent = true;
  const first = await runKimiWorker(f.input, f.options);
  assert.equal(first.status, 'ok', JSON.stringify(first.error));
  assert.equal(first.native.effective.matched, true);
  const second = await runKimiWorker({ ...f.input, sessionId: first.sessionId }, f.options);
  assert.equal(second.status, 'ok', JSON.stringify(second.error));
  assert.equal(second.lifecycle.resumeAccepted, true);
  const calls = await f.log(), profiles = calls.filter(row => row.method === 'POST' && row.path?.endsWith('/profile'));
  assert.equal(profiles.length, 1, 'only the freshly created history can be initialized');
  assert.equal(profiles[0].body.metadata.flywheel_native_binding, first.native.observed.binding);
  assert.deepEqual(profiles[0].body.agent_config, { model: f.input.model, thinking: 'high', permission_mode: 'yolo', plan_mode: false, swarm_mode: true });
  assert.ok(calls.findIndex(row => row.method === 'POST' && row.path?.endsWith('/profile')) < calls.findIndex(row => row.method === 'POST' && row.path?.endsWith('/prompts')));
});

test('an ignored nondefault native profile is not silently replaced by the default agent during fresh initialization', async () => {
  const f = await fixture('agent', 'create-ignores-configuration');
  f.input.native.agentProfile = 'custom-reviewer';
  const result = await runKimiWorker(f.input, f.options);
  assert.equal(result.status, 'error');
  assert.equal(result.error.code, 'UNSUPPORTED_SCHEMA');
  assert.equal(result.lifecycle.dispatch, 'not-dispatched');
  assert.equal(result.lifecycle.cleanupConfirmed, true);
  assert.ok(!(await f.log()).some(row => row.method === 'POST' && /\/sessions\/[^/]+/.test(row.path)));
});

test('Kimi 2.5.3 print compatibility does not bypass its unsupported native conversation contract', async () => {
  const f = await fixture('agent', 'kimi-2.5.3');
  const result = await runKimiWorker(f.input, f.options);
  assert.equal(result.status, 'error');
  assert.equal(result.error.code, 'UNSUPPORTED_SCHEMA');
  assert.equal(result.lifecycle.dispatch, 'not-dispatched');
  assert.equal(result.lifecycle.cleanupConfirmed, true);
  assert.equal(result.native.effective.matched, false);
  assert.ok(!(await f.log()).some(row => row.method === 'POST' && row.path?.startsWith('/api/v1/sessions')));
});

for (const [fixtureCase, code] of [['id-mismatch-remote-on', 'RUNTIME_IDENTITY'],
  ...['malformed-meta', 'missing-meta', 'null-meta-envelope', 'missing-meta-code'].map(name => [name, 'RUNTIME_PROTOCOL'])]) {
  test(`a rejected runtime receives no remote-control or shutdown cleanup requests: ${fixtureCase}`, async () => {
    const f = await fixture('agent', fixtureCase);
    const result = await runKimiWorker(f.input, f.options);
    assert.equal(result.status, 'error');
    assert.equal(result.error.code, code);
    assert.equal(result.lifecycle.dispatch, 'not-dispatched');
    assert.equal(result.lifecycle.cleanupConfirmed, true, 'the owned local process still exits');
    assert.equal(result.native.observed.server, null);
    assert.equal(result.native.cleanup.remoteControlOff, null, 'the rejected endpoint remote state is not observed or rewritten');
    assert.deepEqual((await f.log()).map(({ method, path }) => ({ method, path })), [{ method: 'GET', path: '/api/v1/meta' }]);
    assert.equal(f.frames.filter(frame => frame.type === 'ready').length, 0);
  });
}

test('live root and child progress reaches the parent without tool or reasoning payloads', async () => {
  for (const fixtureCase of ['root-progress', 'child-progress']) {
    const f = await fixture('swarm', fixtureCase);
    const result = await runKimiWorker(f.input, f.options);
    assert.equal(result.status, 'ok', JSON.stringify(result.error));
    const agentId = fixtureCase === 'child-progress' ? 'child-1' : 'main';
    const progress = f.frames.filter(frame => frame.type === 'event' && frame.event.agentId === agentId);
    const reportedTypes = new Set(progress.flatMap(frame => frame.event.coalescedTypes || [frame.event.type]));
    for (const type of ['item.delta', 'item.updated', 'turn.updated']) {
      assert.ok(reportedTypes.has(type), `${agentId} ${type} must reach the process watchdog`);
    }
    assert.doesNotMatch(JSON.stringify(f.frames), /private-tool-progress|fixture-secret/);
    assert.ok(progress.every(frame => !('delta' in frame.event) && !('set' in frame.event) && !('item' in frame.event)));
    assert.equal(result.lifecycle.cleanupConfirmed, true);
    assert.equal(result.native.completion.confirmed, true);
  }
});

test('progress reporting coalesces a busy agent tree within short idle windows without inventing heartbeats', () => {
  let at = 0;
  const events = [], idleTimeoutMs = 100;
  const report = createKimiActivityReporter(idleTimeoutMs, event => events.push({ at, event }), () => at);
  const kinds = ['item.delta', 'item.updated', 'turn.updated'];
  for (let index = 0; index < 100_000; index++) {
    at = index / 100;
    report({ type: kinds[index % kinds.length], agent_id: `child-${index % 12}`, turn_id: 'turn', item_id: 'item',
      delta: 'private-thinking-or-tool-output', set: { text: 'private-thinking-or-tool-output' } });
  }
  assert.ok(events.length >= 35 && events.length <= 41, '100,000 real progress frames share one bounded notification cadence across the tree');
  for (let index = 1; index < events.length; index++) {
    assert.ok(events[index].at - events[index - 1].at < idleTimeoutMs, 'a custom short idle deadline must not inherit a fixed one-second throttle');
  }
  assert.deepEqual(new Set(events.flatMap(row => row.event.coalescedTypes)), new Set(kinds));
  assert.doesNotMatch(JSON.stringify(events), /private-thinking-or-tool-output/);
  const count = events.length;
  report({ type: 'turn.ended', agent_id: 'main', turn: { turn_id: 'turn', status: 'ended', outcome: 'completed' } });
  assert.equal(events.length, count + 1, 'lifecycle evidence is never delayed behind content progress');
  at += 60_000;
  report({ type: 'ping', agent_id: 'main' });
  assert.equal(events.length, count + 1, 'elapsed local time and transport pings cannot emit liveness');
});

test('native progress interval input is optional but explicit invalid deadlines are rejected', async () => {
  const f = await fixture();
  assert.doesNotThrow(() => validateKimiNativeStart(f.input));
  assert.doesNotThrow(() => validateKimiNativeStart({ ...f.input, idleTimeoutMs: 1 }));
  for (const idleTimeoutMs of [0, -1, 0.5, '100', null, Number.MAX_SAFE_INTEGER + 1]) {
    assert.throws(() => validateKimiNativeStart({ ...f.input, idleTimeoutMs }), /bounded protocol 1/);
  }
});

test('placeholder session usage never becomes known zero when real turn usage is absent', async () => {
  const f = await fixture('agent', 'missing-usage');
  const result = await runKimiWorker(f.input, f.options);
  assert.equal(result.status, 'ok');
  assert.equal(result.usage, null);
  assert.equal(result.costUsd, null);
  assert.equal(result.lifecycle.usageStatus, 'unknown');
});

test('missing API capability, mismatched permission, and remote environment refuse dispatch with honest admission receipts', async () => {
  for (const fixtureCase of ['missing-schema', 'mismatch-permission', 'remote-environment']) {
    const f = await fixture('agent', fixtureCase);
    const result = await runKimiWorker(f.input, f.options);
    assert.equal(result.status, 'error');
    assert.equal(result.lifecycle.dispatch, 'not-dispatched');
    assert.equal(result.native.effective.matched, false);
    assert.equal(result.usage, null);
    assert.equal(result.settled, true, JSON.stringify(result.native.cleanup));
    assert.equal((await f.log()).some(row => row.path?.endsWith('/prompts') && row.method === 'POST'), false);
  }
});

test('an explicit unsupported hard cap, Tower resume, and malformed ownership are rejected before launching a CLI', async () => {
  const f = await fixture();
  for (const input of [
    { ...f.input, native: { version: 1, mode: 'tower', agents: { enabled: true, maxConcurrent: 3 } } },
    { ...f.input, native: { version: 1, mode: 'tower' }, persistent: true, sessionId: 'old' },
    { ...f.input, managed: { ...f.input.managed, invocationId: 'another' } },
    { ...f.input, typo: true },
    { ...f.input, managed: { ...f.input.managed, typo: true } },
  ]) {
    const result = await runKimiWorker(input, f.options);
    assert.equal(result.status, 'error');
    assert.equal(result.lifecycle.dispatch, 'not-dispatched');
    assert.equal(result.settled, true);
    assert.equal(result.native.observed.server, null);
  }
  await assert.rejects(stat(f.env.KIMI_FIXTURE_LOG), { code: 'ENOENT' });
  assert.doesNotThrow(() => validateKimiNativeStart(f.input), 'empty run id is valid with a strict invocation id');
});

test('cancellation withdraws queued work and stops child tasks before exiting the owned runtime', async () => {
  const f = await fixture('swarm', 'cancel-queue');
  const controller = new AbortController();
  f.options.emit = frame => { f.frames.push(frame); if (frame.type === 'event' && frame.event.type === 'task') controller.abort(); };
  const result = await runKimiWorker(f.input, { ...f.options, signal: controller.signal });
  assert.equal(result.status, 'cancelled');
  assert.equal(result.lifecycle.dispatch, 'dispatched');
  assert.equal(result.lifecycle.resumeCandidate, false);
  assert.equal(result.native.completion.confirmed, false);
  assert.ok(result.native.completion.pendingIds.length > 0);
  assert.equal(result.settled, true, JSON.stringify(result.native.cleanup));
  const log = await f.log();
  const queuedAbort = log.findIndex(row => row.path?.includes('/prompts/queued-future:abort'));
  const childStop = log.findIndex(row => row.websocket === 'task.stop');
  const shutdown = log.findIndex(row => row.path === '/api/v1/shutdown');
  assert.ok(queuedAbort >= 0 && childStop > queuedAbort && shutdown > childStop);
  assert.equal(result.native.children[0].outcome, 'cancelled');
  assert.equal(result.usage, null, 'unfinished child metering is unknown');
});

test('interactive questions are surfaced as a failure without inventing an answer', async () => {
  const f = await fixture('agent', 'question');
  const result = await runKimiWorker(f.input, f.options);
  assert.equal(result.status, 'error');
  assert.equal(result.error.code, 'INTERACTION_REQUIRED');
  assert.equal((await f.log()).some(row => row.path?.includes('/interactions/')), false);
});

test('history resume preserves canonical native options across reordered config, run/candidate changes, and only counts this invocation', async () => {
  const f = await fixture();
  f.input.persistent = true;
  f.input.native = { version: 1, mode: 'agent', agents: { enabled: true }, agentProfile: 'agent' };
  const first = await runKimiWorker(f.input, f.options);
  assert.equal(first.lifecycle.resumeCandidate, true);
  const resumed = { ...f.input, requestId: 'request-2', sessionId: first.sessionId,
    native: { agents: { enabled: true }, agentProfile: 'agent', mode: 'agent', version: 1 },
    managed: { ...f.input.managed, invocationId: 'request-2', runId: 'new-run', candidateId: 'new-candidate' } };
  const second = await runKimiWorker(resumed, f.options);
  assert.equal(second.status, 'ok', JSON.stringify(second.error));
  assert.equal(second.lifecycle.resumeAccepted, true);
  assert.equal(second.native.observed.binding, first.native.observed.binding);
  assert.equal(JSON.stringify(second.native.requested), JSON.stringify(resumed.native), 'canonical binding does not rewrite the requested receipt');
  assert.equal(second.lifecycle.turnId, 'turn-2');
  assert.equal(second.usage.totalTokens, 23);
  assert.doesNotMatch(second.text, /result 1/);
  for (const role of ['review', 'design-plan', 'design-review', 'plan-review']) {
    const changed = await runKimiWorker({ ...resumed, managed: { ...resumed.managed, role } }, f.options);
    assert.equal(changed.error.code, 'RESUME_BINDING_MISMATCH');
    assert.equal(changed.lifecycle.dispatch, 'not-dispatched');
    assert.equal(changed.lifecycle.resumeAccepted, false);
  }
});

test('Tower enables the real mode in an independent repo and transfers only Execute after confirmed cleanup', async () => {
  for (const role of ['execute', 'review', 'design-plan', 'design-review', 'plan-review']) {
    const f = await fixture('tower');
    f.input.persistent = true; f.input.managed.role = role;
    await towerGit(f.input.cwd, ['init', '--quiet', '-b', 'main']);
    await writeFile(join(f.input.cwd, 'source.txt'), 'original\n');
    await towerGit(f.input.cwd, ['add', '.']); await towerGit(f.input.cwd, ['commit', '--quiet', '-m', 'fixture']);
    const head = (await towerGit(f.input.cwd, ['rev-parse', 'HEAD'])).toString();
    const result = await runKimiWorker(f.input, f.options);
    if (result.native.candidate) directories.push(dirname(result.native.candidate.path));
    assert.equal(result.status, 'ok', JSON.stringify(result.error));
    assert.equal(result.native.effective.tower, true);
    assert.equal(result.lifecycle.resumeCandidate, false);
    assert.equal(result.usage, null, 'Tower cannot certify whole-tree usage from local agent counters');
    assert.equal(result.native.candidate.transfer.applied, role === 'execute');
    assert.notEqual(result.native.observed.environment.cwd, f.input.cwd);
    assert.equal((await towerGit(f.input.cwd, ['rev-parse', 'HEAD'])).toString(), head);
    if (role === 'execute') assert.equal(await readFile(join(f.input.cwd, 'tower-result.txt'), 'utf8'), 'native Tower candidate\n');
    else await assert.rejects(stat(join(f.input.cwd, 'tower-result.txt')), { code: 'ENOENT' });
    assert.ok(result.native.candidate.patchDigest);
  }
});

test('Tower admission resolves source and task directory aliases before fingerprinting and transfer', async () => {
  const f = await fixture('tower');
  const cwd = await realpath(f.input.cwd), taskDir = await realpath(f.input.managed.taskDir);
  await towerGit(cwd, ['init', '--quiet', '-b', 'main']);
  await writeFile(join(cwd, 'source.txt'), 'original\n');
  await writeFile(join(taskDir, 'spec.md'), 'frozen contract\n');
  await towerGit(cwd, ['add', '.']);
  await towerGit(cwd, ['commit', '--quiet', '-m', 'fixture']);
  await writeFile(join(taskDir, 'state.json'), '{"revision":1}');
  const before = await towerSnapshot(cwd, taskDir);
  const alias = join(f.root, 'project-alias');
  await symlink(cwd, alias, 'dir');
  f.input.cwd = alias;
  f.input.managed.taskDir = join(alias, 'task');
  f.input.prompt = `Edit "${join(alias, 'source.txt')}" and inspect "${join(cwd, 'source.txt')}". ` +
    `Keep "${alias}-other/source.txt", https://example.invalid${alias}/source.txt and https://example.invalid/?cwd=${alias}/source.txt unchanged.`;
  assert.notEqual(alias, cwd, 'exercise a real directory alias even on Linux');
  const result = await runKimiWorker(f.input, f.options);
  if (result.native.candidate) directories.push(dirname(result.native.candidate.path));
  assert.equal(result.status, 'ok', JSON.stringify(result.error));
  assert.equal(result.native.observed.promptMapping.sourceCwd, cwd);
  assert.equal(result.native.observed.promptMapping.declaredSourceCwd, alias);
  assert.equal(result.native.observed.managed.taskDir, taskDir);
  assert.equal(result.native.candidate.sourceSnapshotDigest, before.digest, 'aliases preserve the physical source/contract fingerprint');
  const submitted = (await f.log()).find(row => row.method === 'POST' && row.path?.endsWith('/prompts')).body.content[0].text;
  const target = join(result.native.candidate.path, 'source.txt');
  assert.ok(submitted.includes(`Edit "${target}" and inspect "${target}".`), 'both declared alias and physical paths must point at the candidate');
  assert.ok(submitted.includes(`"${alias}-other/source.txt"`));
  assert.ok(submitted.includes(`https://example.invalid${alias}/source.txt`));
  assert.ok(submitted.includes(`https://example.invalid/?cwd=${alias}/source.txt`));
  assert.equal(result.native.observed.promptMapping.replacements, 2);
  assert.equal(result.native.observed.promptMapping.originalPromptDigest, createHash('sha256').update(f.input.prompt).digest('hex'));
  assert.equal(result.native.observed.promptMapping.effectivePromptDigest, createHash('sha256').update(submitted).digest('hex'));
  assert.deepEqual(result.native.completion, { confirmed: true, pendingIds: [] });
  assert.equal(result.native.cleanup.confirmed, true);
  assert.equal(result.native.candidate.transfer.applied, true);
  assert.equal(await readFile(join(cwd, 'tower-result.txt'), 'utf8'), 'native Tower candidate\n');
  assert.equal(await readFile(join(taskDir, 'spec.md'), 'utf8'), 'frozen contract\n');
  assert.equal(await readFile(join(taskDir, 'state.json'), 'utf8'), '{"revision":1}');
  assert.equal((await towerGit(cwd, ['rev-parse', 'HEAD'])).toString().trim(), before.head);
});

test('Tower timeout with unconfirmed native cleanup cannot freeze or transfer a partial candidate', async () => {
  const f = await fixture('tower', 'unstoppable');
  f.input.timeoutMs = 700;
  await towerGit(f.input.cwd, ['init', '--quiet', '-b', 'main']);
  await writeFile(join(f.input.cwd, 'source.txt'), 'original\n');
  await towerGit(f.input.cwd, ['add', '.']); await towerGit(f.input.cwd, ['commit', '--quiet', '-m', 'fixture']);
  const result = await runKimiWorker(f.input, f.options);
  if (result.native.candidate) directories.push(dirname(result.native.candidate.path));
  assert.equal(result.status, 'error');
  assert.equal(result.error.code, 'TIMEOUT');
  assert.equal(result.native.completion.confirmed, false);
  assert.equal(result.native.cleanup.confirmed, false);
  assert.equal(result.lifecycle.cleanupConfirmed, false);
  assert.equal(result.native.candidate.transfer, undefined);
  await assert.rejects(stat(join(f.input.cwd, 'tower-result.txt')), { code: 'ENOENT' });
  assert.equal(await readFile(join(result.native.candidate.path, 'tower-result.txt'), 'utf8'), 'native Tower candidate\n');
});

test('a source contract change during Tower execution preserves the candidate and fails transfer', async () => {
  const f = await fixture('tower');
  await towerGit(f.input.cwd, ['init', '--quiet', '-b', 'main']);
  await writeFile(join(f.input.managed.taskDir, 'spec.md'), 'original contract\n');
  await towerGit(f.input.cwd, ['add', '.']); await towerGit(f.input.cwd, ['commit', '--quiet', '-m', 'fixture']);
  let edit;
  f.options.emit = frame => { if (frame.type === 'ready') edit = writeFile(join(f.input.managed.taskDir, 'spec.md'), 'changed contract\n'); };
  const result = await runKimiWorker(f.input, f.options);
  await edit;
  if (result.native.candidate) directories.push(dirname(result.native.candidate.path));
  assert.equal(result.status, 'error');
  assert.equal(result.error.code, 'CANDIDATE_TRANSFER_FAILED');
  assert.equal(result.native.cleanup.confirmed, true);
  assert.equal(result.native.completion.confirmed, true, 'native work ended but the candidate is not transferable');
  assert.equal(result.native.candidate.transfer.applied, false);
  assert.ok((await stat(result.native.candidate.patchPath)).size > 0);
  await assert.rejects(stat(join(f.input.cwd, 'tower-result.txt')), { code: 'ENOENT' });
  assert.equal(await readFile(join(f.input.managed.taskDir, 'spec.md'), 'utf8'), 'changed contract\n');
});

test('projection applies replacements and does not count duplicated usage or trust context token frames', () => {
  const c = new KimiConversation();
  const turn = { turn_id: 't', ordinal: 1, kind: 'agent', status: 'ended', outcome: 'completed', items: [], steps: [{ index: 1,
    usage: { input_other: 1, output: 2, input_cache_read: 3, input_cache_creation: 4 } }] };
  c.fold({ type: 'turns', agent_id: 'main', turns: [turn] });
  c.fold({ type: 'turn.ended', agent_id: 'main', turn });
  c.fold({ type: 'usage', agent_id: 'main', context_tokens: 90000 });
  assert.equal(kimiObservedUsage(c, 0, 'agent').usage.totalTokens, 10);
  c.fold({ type: 'agent', agent_id: 'main', status: 'running', compacting: true });
  c.fold({ type: 'agent', agent_id: 'main', status: 'idle', compacting: false });
  assert.equal(kimiObservedUsage(c, 0, 'agent').usage, null, 'compaction model calls have no complete step usage in this API');
  assert.throws(() => c.fold({ type: 'turns', agent_id: 'main', turns: [turn] }), /replaced a subscribed conversation/);
  c.fold({ type: 'turns.removed', agent_id: 'main', turn_ids: ['t'] });
  assert.equal(kimiObservedUsage(c, 0, 'agent').usage, null);
});

test('a resumed child history is not charged again as current invocation usage', () => {
  const c = new KimiConversation();
  const step = { index: 1, usage: { input_other: 1, output: 2, input_cache_read: 0, input_cache_creation: 0 } };
  c.fold({ type: 'turns', agent_id: 'main', turns: [{ turn_id: 'root', ordinal: 1, kind: 'agent', status: 'ended', items: [], steps: [step] }] });
  c.fold({ type: 'task', agent_id: 'main', task_id: 'reused', kind: 'subagent', child_agent_id: 'old-child', status: 'ended' });
  c.fold({ type: 'turns', agent_id: 'old-child', turns: [{ turn_id: 'old', ordinal: 0, kind: 'agent', status: 'ended', items: [], steps: [step] }] });
  const observed = kimiObservedUsage(c, 0, 'agent', { resumed: true });
  assert.equal(observed.usage, null);
  assert.equal(observed.observedSteps.length, 1);
  assert.equal(observed.observedSteps[0].agentId, 'main');
  assert.match(observed.scope, /resumed-child-history/);
});

async function runCLI(f, { eof = false, doctor = false } = {}) {
  const child = spawn(process.execPath, [cli, ...(doctor ? ['--doctor'] : [])], { env: f.env, stdio: ['pipe', 'pipe', 'pipe'] });
  let stdout = '', stderr = '';
  child.stdout.on('data', data => { stdout += data; if (eof && stdout.includes('"type":"ready"')) child.stdin.end(); });
  child.stderr.on('data', data => { stderr += data; });
  if (!doctor) child.stdin.write(JSON.stringify(f.input) + '\n');
  const code = await new Promise(resolve => child.once('exit', resolve));
  return { code, stdout, stderr, frames: stdout.trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) };
}

test('CLI emits exactly one attributable final, treats parent EOF as cancellation, and doctor does no inference', async () => {
  const f = await fixture();
  const normal = await runCLI(f);
  assert.equal(normal.code, 0);
  assert.equal(normal.frames.filter(frame => frame.type === 'result').length, 1);
  assert.equal(normal.frames.at(-1).status, 'ok');
  const cancelled = await runCLI(await fixture('agent', 'cancel-queue'), { eof: true });
  assert.equal(cancelled.code, 130);
  assert.equal(cancelled.frames.filter(frame => frame.type === 'result').length, 1);
  assert.equal(cancelled.frames.at(-1).status, 'cancelled');
  assert.equal(cancelled.frames.at(-1).settled, true);
  const doctor = await runCLI(await fixture(), { doctor: true });
  assert.equal(doctor.code, 0);
  assert.equal(doctor.frames[0].inferenceTested, false);
  assert.equal(doctor.frames[0].recoverySupported, false);
  assert.doesNotMatch(normal.stdout + normal.stderr + cancelled.stdout + cancelled.stderr + doctor.stdout, /fixture-secret/);
});
