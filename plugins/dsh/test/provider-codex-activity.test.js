import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { chmod, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createInterface } from 'node:readline';
import { fileURLToPath } from 'node:url';
import { ADAPTER, LIMITS } from '../src/codex-pro-bridge.js';

const worker = fileURLToPath(new URL('../dist/codex-pro-worker.js', import.meta.url));
const fixture = fileURLToPath(new URL('./fixtures/fake-codex-pro.js', import.meta.url));
await chmod(fixture, 0o755);

async function setup(t, scenario) {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-codex-activity-')));
  const log = join(cwd, 'requests.jsonl'), history = join(cwd, 'history.json');
  const environment = { ...process.env, FLYWHEEL_CODEX_BIN: fixture, CODEX_HOME: join(cwd, 'codex-home'),
    TEST_CODEX_PRO_LOG: log, TEST_CODEX_PRO_HISTORY: history, TEST_CODEX_PRO_SCENARIO: scenario };
  delete environment.NODE_OPTIONS; delete environment.CODEX_SUBAGENT_LIVE;
  const request = { protocol: 3, type: 'start', requestId: 'activity-attempt', cwd, model: 'fixture-model',
    reasoningEffort: 'high', permissionMode: 'workspace-write', prompt: 'Private offline task.',
    threadMode: 'ephemeral', recoveryDirectory: join(cwd, 'journal'), timeoutMs: 10000, idleTimeoutMs: 1500 };
  const runs = [];
  const launch = (input = request) => {
    const child = spawn(process.execPath, [worker], { cwd, env: environment, stdio: ['pipe', 'pipe', 'pipe'] });
    const run = { child, frames: [], stderr: '', closed: false };
    child.stderr.setEncoding('utf8'); child.stderr.on('data', text => { run.stderr += text; });
    child.stdin.on('error', () => {});
    createInterface({ input: child.stdout }).on('line', line => {
      const frame = JSON.parse(line); run.frames.push(frame);
      if (frame.type === 'lifecycle') child.stdin.write(JSON.stringify({ type: 'ack', protocol: 3,
        requestId: input.requestId, runId: frame.lifecycle.runId, sequence: frame.lifecycle.sequence }) + '\n');
    });
    run.done = new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('close', (code, signal) => { run.closed = true; resolve({ ...run, code, signal }); });
    });
    child.stdin.write(JSON.stringify(input) + '\n'); runs.push(run);
    return run;
  };
  t.after(async () => {
    for (const run of runs) {
      if (!run.closed) run.child.kill('SIGTERM');
      let timer;
      await Promise.race([run.done, new Promise(resolve => { timer = setTimeout(resolve, 10000); })]);
      clearTimeout(timer);
      if (!run.closed) run.child.kill('SIGKILL');
      await run.done;
    }
    await rm(cwd, { recursive: true, force: true });
  });
  const records = async () => (await readFile(log, 'utf8').catch(() => '')).trim().split('\n').filter(Boolean).map(JSON.parse);
  return { cwd, log, history, request, launch, records };
}

function completed(run) {
  assert.equal(run.code, 0, run.stderr + JSON.stringify(run.frames.at(-1)));
  assert.equal(run.signal, null);
  assert.equal(run.frames.filter(f => f.type === 'result').length, 1);
  const frame = run.frames.at(-1);
  assert.equal(frame.type, 'result'); assert.deepEqual(frame.adapter, ADAPTER);
  assert.equal(frame.error, null); assert.equal(frame.receipt.ok, true);
  assert.equal(frame.receipt.outcome.cleanup.confirmed, true);
  assert.equal(frame.receipt.outcome.execution.truncated, false);
  assert.equal(frame.receipt.outcome.output[0].text, 'fixture complete\n');
  return frame.receipt;
}

for (const scenario of ['activity-root', 'activity-child', 'activity-default-child']) {
  test(`installed machine worker forwards ${scenario} reasoning, text and command observations`, { timeout: 20000 }, async t => {
    const f = await setup(t, scenario);
    const input = { ...f.request, ...(scenario === 'activity-child' ? { native: { version: 1, agents: { enabled: true } } } : {}) };
    const run = await f.launch(input).done, receipt = completed(run);
    const activity = run.frames.filter(x => x.type === 'activity');
    assert.ok(activity.length >= 4, 'work lasts beyond several idle intervals and emits new observations');
    assert.ok(activity.length < 40, 'the complete tree uses a bounded coalesced channel');
    assert.ok(activity.every((frame, i) => frame.requestId === input.requestId && frame.protocol === 3
      && frame.activity.source === 'codex-pro' && (i === 0 || frame.activity.sequence > activity[i - 1].activity.sequence)));
    const types = new Set(activity.flatMap(x => x.activity.eventTypes));
    for (const kind of ['item/reasoning/summaryTextDelta', 'item/reasoning/textDelta', 'item/agentMessage/delta', 'item/commandExecution/outputDelta']) assert.ok(types.has(kind), kind);
    if (scenario !== 'activity-root') assert.ok(activity.some(x => ['child', 'mixed'].includes(x.activity.scope)));
    assert.equal(JSON.stringify(activity).includes('PRIVATE_REASONING_AND_TOOL_CONTENT'), false);
    assert.equal(JSON.stringify(activity).includes('itemId'), false);
    const records = await f.records();
    assert.equal(records.find(x => x.event === 'activity-complete').count, 70);
    if (scenario === 'activity-default-child') {
      assert.equal(receipt.native, undefined);
      assert.equal(records.some(x => x.method === 'config/read'), false);
    }
    assert.ok(run.frames.some(x => x.type === 'lifecycle' && x.lifecycle.event === 'cleanup-confirmed'));
    if (scenario === 'activity-root') {
      const recovered = await f.launch({ protocol: 3, type: 'recover', requestId: input.requestId, runId: receipt.runId,
        recoveryDirectory: input.recoveryDirectory, original: { cwd: input.cwd, model: input.model,
          reasoningEffort: input.reasoningEffort, permissionMode: input.permissionMode, threadMode: input.threadMode } }).done;
      assert.equal(recovered.code, 0, recovered.stderr + JSON.stringify(recovered.frames));
      assert.equal(recovered.frames.at(-1).recovery.receipt.ok, true);
      assert.equal((await f.records()).filter(x => x.event === 'spawn').length, 1, 'read-only recovery does not dispatch again');
    }
  });
}

for (const scenario of ['activity-raw-volume', 'activity-display-volume']) {
  test(`installed worker completes ${scenario} beyond the old cumulative 1 MiB limit`, { timeout: 15000 }, async t => {
    const f = await setup(t, scenario), run = await f.launch().done;
    completed(run);
    assert.ok((await f.records()).find(x => x.event === 'activity-complete').rawBytes > 1024 * 1024);
    assert.ok(run.frames.filter(x => x.type === 'activity').length < 40);
    const progressBytes = run.frames.filter(x => x.type === 'progress').reduce((sum, frame) => sum + Buffer.byteLength(JSON.stringify(frame) + '\n'), 0);
    assert.ok(progressBytes <= LIMITS.progressBytes + 1024, 'optional display drops excess without losing final receipt');
  });
}

for (const scenario of ['native-child', 'native-stale-spawn', 'native-new-turn-completed']) {
  test(`installed worker preserves exact child completion through ${scenario}`, { timeout: 15000 }, async t => {
    const f = await setup(t, scenario);
    const receipt = completed(await f.launch({ ...f.request, native: { version: 1, agents: { enabled: true } } }).done);
    assert.equal(receipt.native.completion.confirmed, true);
    assert.deepEqual(receipt.native.completion.pendingIds, []);
    assert.equal(receipt.native.children.length, 1);
    const child = receipt.native.children[0];
    assert.equal(child.id, 'child-thread');
    assert.equal(child.state, 'completed');
    assert.equal(child.turnId, scenario === 'native-new-turn-completed' ? 'child-turn-2' : 'child-turn');
    assert.equal(child.usage.status, 'complete');
    assert.equal(receipt.native.usage.totalTokens, scenario === 'native-new-turn-completed' ? 30 : 15);
  });
}

test('installed worker keeps a new child turn pending despite a late completed spawn snapshot', { timeout: 15000 }, async t => {
  const f = await setup(t, 'native-new-turn-pending');
  const run = await f.launch({ ...f.request, native: { version: 1, agents: { enabled: true } } }).done;
  assert.equal(run.code, 1, JSON.stringify(run.frames.at(-1)));
  const receipt = run.frames.at(-1).receipt;
  assert.equal(receipt.ok, false);
  assert.equal(receipt.outcome.execution.failure.stage, 'native');
  assert.equal(receipt.outcome.execution.failure.category, 'completion');
  assert.equal(receipt.outcome.cleanup.confirmed, true);
  assert.equal(receipt.native.completion.confirmed, false);
  assert.deepEqual(receipt.native.completion.pendingIds, ['child-thread']);
  assert.equal(receipt.native.children[0].turnId, 'child-turn-2');
  assert.equal(receipt.native.children[0].state, 'running');
  assert.equal(receipt.native.children[0].usage.status, 'partial');
  assert.equal(receipt.native.usage.totalTokens, null);
});

for (const sessionTitle of ['Flywheel · Alerts · Design review', undefined]) {
  test(`installed persistent worker fails before dispatch when ${sessionTitle ? 'the explicit title' : 'the fallback title'} is rejected`, { timeout: 15000 }, async t => {
    const f = await setup(t, 'name-rejected');
    const input = { ...f.request, threadMode: 'persistent', ...(sessionTitle === undefined ? {} : { sessionTitle }) };
    const run = await f.launch(input).done;
    assert.equal(run.code, 1, JSON.stringify(run.frames.at(-1)));
    const receipt = run.frames.at(-1).receipt;
    assert.equal(receipt.ok, false);
    assert.equal(receipt.outcome, null);
    assert.equal(receipt.failure.stage, 'thread-name');
    assert.equal(receipt.failure.category, 'protocol');
    assert.match(receipt.failure.diagnostic, /thread name.*no task prompt was sent/);
    assert.equal(receipt.failure.execution.threadId, 'fixture-thread');
    assert.equal(receipt.failure.execution.turnId, null);
    assert.equal(receipt.failure.execution.dispatch, 'not-dispatched');
    assert.equal(receipt.failure.execution.cancelled, false);
    assert.equal(receipt.failure.execution.timedOut, false);
    assert.equal(receipt.failure.cleanup.confirmed, true);
    const lifecycle = run.frames.filter(x => x.type === 'lifecycle').map(x => x.lifecycle);
    assert.deepEqual(lifecycle.map(x => x.event), ['terminal', 'cleanup-confirmed']);
    assert.ok(lifecycle.every(x => x.threadId === 'fixture-thread' && x.turnId === null
      && x.dispatch === 'not-dispatched' && x.outcome === 'failed'));

    const recovered = await f.launch({ protocol: 3, type: 'recover', requestId: input.requestId, runId: receipt.runId,
      recoveryDirectory: input.recoveryDirectory, original: { cwd: input.cwd, model: input.model,
        reasoningEffort: input.reasoningEffort, permissionMode: input.permissionMode, threadMode: input.threadMode } }).done;
    assert.equal(recovered.code, 0, recovered.stderr + JSON.stringify(recovered.frames));
    assert.deepEqual(recovered.frames.at(-1).recovery.receipt, receipt);
    const records = await f.records();
    assert.equal(records.filter(x => x.event === 'spawn').length, 1, 'failure and recovery do not start a replacement provider');
    const names = records.filter(x => x.method === 'thread/name/set');
    assert.equal(names.length, 1);
    if (sessionTitle !== undefined) assert.equal(names[0].params.name, sessionTitle);
    assert.equal(records.some(x => ['turn/start', 'review/start'].includes(x.method)), false, 'no business prompt is dispatched');
    assert.equal(JSON.parse(await readFile(f.history, 'utf8')).name, undefined);
  });
}

test('quiet installed provider has no generated heartbeat and preserves its real timeout receipt', { timeout: 10000 }, async t => {
  const f = await setup(t, 'activity-quiet'), run = await f.launch({ ...f.request, timeoutMs: 1300 }).done;
  assert.equal(run.code, 1);
  const activity = run.frames.filter(x => x.type === 'activity');
  assert.equal(activity.length, 1);
  assert.deepEqual(activity[0].activity.eventTypes, ['turn/started']);
  const receipt = run.frames.at(-1).receipt;
  assert.equal(receipt.ok, false);
  assert.equal((receipt.outcome ?? receipt.failure).execution.timedOut, true);
  assert.equal((receipt.outcome ?? receipt.failure).cleanup.confirmed, true);
});

test('persistent worker titles are readable and a resume preserves a user edit', { timeout: 15000 }, async t => {
  const f = await setup(t, 'ok');
  const initial = { ...f.request, protocol: 2, threadMode: 'persistent', sessionTitle: 'Flywheel · Alerts · Design review' };
  delete initial.recoveryDirectory;
  const first = completed(await f.launch(initial).done);
  const history = JSON.parse(await readFile(f.history, 'utf8'));
  history.name = 'User edited investigation title';
  await writeFile(f.history, JSON.stringify(history));
  completed(await f.launch({ ...initial, requestId: 'activity-resume', threadId: first.outcome.threadId, sessionTitle: 'Must not overwrite' }).done);
  assert.equal(JSON.parse(await readFile(f.history, 'utf8')).name, history.name);
  completed(await f.launch({ ...f.request, sessionTitle: 'Ephemeral stays ephemeral' }).done);
  const records = await f.records();
  assert.deepEqual(records.filter(x => x.method === 'thread/name/set').map(x => x.params.name), [initial.sessionTitle]);
  const starts = records.filter(x => x.method === 'thread/start');
  assert.deepEqual(starts.map(x => x.params.ephemeral), [false, true]);
});
