import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile, rm, stat } from 'node:fs/promises';
import { spawn } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { runKimiWorker, validateKimiNativeStart, KimiConversation, kimiObservedUsage } from '../src/kimi-native-bridge.js';
import { towerGit } from '../src/kimi-tower-candidate.js';

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
  const third = await runKimiWorker({ ...resumed, managed: { ...resumed.managed, role: 'review' } }, f.options);
  assert.equal(third.error.code, 'RESUME_BINDING_MISMATCH');
  assert.equal(third.lifecycle.dispatch, 'not-dispatched');
  assert.equal(third.lifecycle.resumeAccepted, false);
});

test('Tower enables the real mode in an independent repo and transfers only Execute after confirmed cleanup', async () => {
  for (const role of ['execute', 'review']) {
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
