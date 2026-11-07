import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { chmod, copyFile, mkdir, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ADAPTER, LIMITS, loadAdapter, machineRequest } from '../src/codex-pro-bridge.js';

const worker = fileURLToPath(new URL('../dist/codex-pro-worker.js', import.meta.url));
const fixture = fileURLToPath(new URL('./fixtures/fake-codex-pro.js', import.meta.url));
await chmod(fixture, 0o755);
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

async function setup(t, scenario = 'ok') {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel codex pro $() ')));
  const log = join(cwd, 'rpc.jsonl');
  const runs = [];
  const env = { ...process.env, FLYWHEEL_CODEX_BIN: fixture, TEST_CODEX_PRO_SCENARIO: scenario,
    TEST_CODEX_PRO_LOG: log, CODEX_HOME: join(cwd, 'isolated-codex-home') };
  delete env.NODE_OPTIONS;
  delete env.CODEX_SUBAGENT_LIVE;
  const request = { protocol: 1, type: 'start', requestId: 'attempt-1', cwd, model: 'fixture-model', reasoningEffort: 'high',
    permissionMode: 'workspace-write', prompt: 'Do the offline task.', timeoutMs: 5000 };
  const records = async () => (await readFile(log, 'utf8').catch(() => '')).trim().split('\n').filter(Boolean).map(value => JSON.parse(value));
  const launch = ({ args = [], input = request, binary = worker, environment = env } = {}) => {
    const child = spawn(process.execPath, [binary, ...args], { cwd, env: environment, stdio: ['pipe', 'pipe', 'pipe'] });
    const run = { child, stdout: '', stderr: '', closed: false };
    child.stdout.setEncoding('utf8'); child.stderr.setEncoding('utf8');
    child.stdout.on('data', value => { run.stdout += value; });
    child.stderr.on('data', value => { run.stderr += value; });
    child.stdin.on('error', () => {});
    run.done = new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('close', (code, signal) => { run.closed = true; resolve({ code, signal, stdout: run.stdout, stderr: run.stderr }); });
    });
    if (input !== null) child.stdin.write(typeof input === 'string' ? input : JSON.stringify(input) + '\n');
    runs.push(run);
    return run;
  };
  t.after(async () => {
    for (const run of runs) {
      if (!run.closed) { run.child.stdin.end(); run.child.kill('SIGTERM'); }
      let timer;
      await Promise.race([run.done, new Promise(resolve => { timer = setTimeout(resolve, 10000); })]);
      clearTimeout(timer);
      if (!run.closed) run.child.kill('SIGKILL');
      await run.done;
    }
    await rm(cwd, { recursive: true, force: true });
  });
  return { cwd, request, records, launch, env };
}

function resultFrame(result, code = 0) {
  assert.equal(result.code, code, result.stderr + result.stdout);
  assert.equal(result.signal, null);
  const frames = result.stdout.trim().split('\n').filter(Boolean).map(line => JSON.parse(line));
  assert.equal(frames.filter(frame => frame.type === 'result').length, 1);
  assert.ok(frames.slice(0, -1).every(frame => frame.type === 'progress'));
  assert.deepEqual(frames.at(-1).adapter, ADAPTER);
  return frames.at(-1);
}

function absent(pid) {
  assert.ok(Number.isSafeInteger(pid) && pid > 0);
  assert.throws(() => process.kill(pid, 0), { code: 'ESRCH' });
}

function absentGroup(execution) {
  absent(execution.pid);
  absent(execution.supervisorPid);
  assert.throws(() => process.kill(-execution.supervisorPid, 0), { code: 'ESRCH' });
}

async function waitTurn(f) {
  const deadline = Date.now() + 5000;
  while (!(await f.records()).some(record => record.method === 'turn/start')) {
    assert.ok(Date.now() < deadline, 'offline app-server never received turn/start');
    await delay(15);
  }
}

test('public installed machine API carries exact settings and a complete unchanged receipt', { timeout: 10000 }, async t => {
  const f = await setup(t);
  const frame = resultFrame(await f.launch().done);
  assert.equal(frame.requestId, f.request.requestId);
  assert.equal(frame.error, null);
  const { receipt } = frame;
  assert.equal(receipt.ok, true);
  assert.equal(receipt.requested.effort, 'high');
  assert.equal(receipt.requested.threadId, null);
  assert.deepEqual(receipt.outcome.output, [{ type: 'text', text: 'fixture complete\n' }]);
  assert.equal(receipt.outcome.configuration.effective.cwd, f.cwd);
  assert.deepEqual(receipt.outcome.configuration.effective.sandboxPolicy.writableRoots, [f.cwd]);
  assert.deepEqual(receipt.outcome.turnConfiguration, { requestedEffort: 'high', effectiveEffort: null, evidence: 'turn/start-request', accepted: true });
  assert.equal(receipt.outcome.execution.cancelled, false);
  assert.equal(receipt.outcome.cleanup.confirmed, true);
  absentGroup(receipt.outcome.execution);
  assert.equal(receipt.outcome.usage.status, 'complete');
  assert.equal(receipt.outcome.usage.run.totalTokens, 108);
  assert.equal(receipt.costUSD, null);
  assert.equal(receipt.costStatus, 'unknown');
  const records = await f.records();
  assert.deepEqual(records[0].argv, ['app-server', '--stdio']);
  const start = records.find(record => record.method === 'thread/start').params;
  assert.deepEqual(start, { cwd: f.cwd, ephemeral: true, model: 'fixture-model', approvalPolicy: 'never', sandbox: 'workspace-write' });
  assert.equal(records.find(record => record.method === 'turn/start').params.effort, 'high');
});

test('canonical full access maps to the public machine mode and keeps its actual policy receipt', { timeout: 10000 }, async t => {
  const f = await setup(t);
  const frame = resultFrame(await f.launch({ input: { ...f.request, permissionMode: 'danger-full-access' } }).done);
  assert.equal(frame.error, null);
  assert.equal(frame.receipt.ok, true);
  assert.equal(frame.receipt.requested.permissionMode, 'dangerously-bypass-approvals-and-sandbox');
  const effective = frame.receipt.outcome.configuration.effective;
  assert.equal(effective.sandbox, 'danger-full-access');
  assert.equal(effective.approvalPolicy, 'never');
  assert.deepEqual(effective.sandboxPolicy, { type: 'dangerFullAccess' });
  assert.equal(frame.receipt.outcome.cleanup.confirmed, true);
  absentGroup(frame.receipt.outcome.execution);
  const records = await f.records();
  assert.deepEqual(records.find(record => record.method === 'thread/start').params,
    { cwd: f.cwd, ephemeral: true, model: 'fixture-model', approvalPolicy: 'never', sandbox: 'danger-full-access' });
  assert.equal(records.find(record => record.method === 'turn/start').params.effort, 'high');
});

test('a full-access request cannot silently accept a constrained app-server policy', async t => {
  const f = await setup(t, 'wrong-sandbox');
  const frame = resultFrame(await f.launch({ input: { ...f.request, permissionMode: 'danger-full-access' } }).done, 1);
  assert.equal(frame.receipt.requested.permissionMode, 'dangerously-bypass-approvals-and-sandbox');
  assert.equal(frame.receipt.ok, false);
  assert.equal(frame.receipt.failure.configuration.matched, false);
  assert.equal(frame.receipt.failure.cleanup.confirmed, true);
  assert.ok(!(await f.records()).some(record => record.method === 'turn/start'));
});

test('doctor imports the exact dependency without starting any Codex process', async t => {
  const f = await setup(t);
  const result = await f.launch({ args: ['--doctor'], input: null }).done;
  assert.equal(result.code, 0, result.stderr);
  assert.deepEqual(JSON.parse(result.stdout), { protocol: 1, type: 'doctor', package: ADAPTER.package, version: ADAPTER.version,
    machineProtocol: 1, available: true, inferenceTested: false });
  assert.deepEqual(await f.records(), []);
});

for (const scenario of ['wrong-model', 'wrong-cwd', 'wrong-approval', 'wrong-sandbox']) {
  test(`${scenario} retains startup evidence and prevents the model turn`, async t => {
    const f = await setup(t, scenario);
    const frame = resultFrame(await f.launch().done, 1);
    assert.equal(frame.receipt.ok, false);
    assert.equal(frame.receipt.failure.configuration.matched, false);
    assert.equal(frame.receipt.failure.cleanup.confirmed, true);
    assert.ok(!(await f.records()).some(record => record.method === 'turn/start'));
  });
}

for (const scenario of ['unknown-usage', 'partial-usage']) {
  test(`${scenario} stays unknown or partial in the raw successful receipt`, async t => {
    const f = await setup(t, scenario);
    const frame = resultFrame(await f.launch().done);
    assert.equal(frame.receipt.ok, true);
    assert.notEqual(frame.receipt.outcome.usage.status, 'complete');
    assert.equal(frame.receipt.costUSD, null);
  });
}

test('full policy is preserved, without claiming a pre-turn consumer policy guard', async t => {
  const f = await setup(t, 'broad-policy');
  const frame = resultFrame(await f.launch().done);
  assert.deepEqual(frame.receipt.outcome.configuration.effective.sandboxPolicy.writableRoots, ['/']);
  assert.ok((await f.records()).some(record => record.method === 'turn/start'));
});

for (const signal of ['SIGTERM', 'SIGINT']) {
  test(`${signal} awaits adapter cleanup and returns its actual cancellation receipt`, { timeout: 15000 }, async t => {
    const f = await setup(t, 'hang');
    const run = f.launch();
    await waitTurn(f);
    run.child.kill(signal);
    const frame = resultFrame(await run.done, signal === 'SIGTERM' ? 143 : 130);
    assert.equal(frame.receipt.ok, false);
    assert.equal(frame.receipt.outcome.execution.cancelled, true);
    assert.equal(frame.receipt.outcome.cleanup.confirmed, true);
    absentGroup(frame.receipt.outcome.execution);
  });
}

test('stdin EOF after admission cancels the owned run and preserves its cleanup receipt', { timeout: 15000 }, async t => {
  const f = await setup(t, 'hang');
  const run = f.launch();
  await waitTurn(f);
  run.child.stdin.end();
  const frame = resultFrame(await run.done, 1);
  assert.equal(frame.error.code, 'PARENT_DISCONNECTED');
  assert.equal(frame.receipt.outcome.execution.cancelled, true);
  assert.equal(frame.receipt.outcome.cleanup.confirmed, true);
  absentGroup(frame.receipt.outcome.execution);
});

test('a stubborn app-server is killed within the adapter cleanup budget', { timeout: 15000 }, async t => {
  const f = await setup(t, 'stubborn');
  const run = f.launch();
  await waitTurn(f);
  run.child.kill('SIGTERM');
  const frame = resultFrame(await run.done, 143);
  assert.equal(frame.receipt.ok, false);
  assert.equal(frame.receipt.outcome.execution.cancelled, true);
  assert.equal(frame.receipt.outcome.cleanup.confirmed, true);
  absentGroup(frame.receipt.outcome.execution);
});

test('a broken stdout waits for the app-server and guardian to disappear', { timeout: 15000 }, async t => {
  const f = await setup(t, 'progress-hang');
  const run = f.launch();
  await waitTurn(f);
  run.child.stdout.destroy();
  const result = await run.done;
  assert.equal(result.code, 1, result.stderr);
  const records = await f.records();
  absentGroup(records[0]);
  assert.doesNotMatch(result.stderr, /Unhandled|EPIPE/);
});

test('progress budget failure preserves the raw cancellation receipt instead of dropping frames silently', { timeout: 10000 }, async t => {
  const f = await setup(t, 'progress-limit');
  const frame = resultFrame(await f.launch({ input: { ...f.request, requestId: 'r'.repeat(256) } }).done, 1);
  assert.equal(frame.error.code, 'OUTPUT_LIMIT');
  assert.equal(frame.receipt.ok, false);
  assert.equal(frame.receipt.outcome.execution.cancelled, true);
  assert.equal(frame.receipt.outcome.cleanup.confirmed, true);
  absentGroup(frame.receipt.outcome.execution);
});

test('worker SIGKILL makes the installed guardian clean ordinary descendants without a forged receipt', { timeout: 15000 }, async t => {
  const f = await setup(t, 'descendant');
  const run = f.launch();
  await waitTurn(f);
  const deadline = Date.now() + 5000;
  while (!(await f.records()).some(record => record.event === 'descendant')) {
    assert.ok(Date.now() < deadline, 'ordinary descendant did not start');
    await delay(15);
  }
  const records = await f.records();
  run.child.kill('SIGKILL');
  const result = await run.done;
  assert.equal(result.code, null);
  assert.equal(result.signal, 'SIGKILL');
  assert.ok(!result.stdout.split('\n').filter(Boolean).map(line => JSON.parse(line)).some(frame => frame.type === 'result'));
  const until = Date.now() + 8000;
  for (;;) {
    try { process.kill(-records[0].supervisorPid, 0); }
    catch (error) { if (error.code === 'ESRCH') break; throw error; }
    assert.ok(Date.now() < until, 'guardian process group remained after caller death');
    await delay(25);
  }
  absentGroup(records[0]);
  absent(records.find(record => record.event === 'descendant').pid);
});

test('an additional request cancels instead of silently starting or ignoring another task', { timeout: 15000 }, async t => {
  const f = await setup(t, 'hang');
  const run = f.launch();
  await waitTurn(f);
  run.child.stdin.write('{}\n');
  const frame = resultFrame(await run.done, 1);
  assert.equal(frame.error.code, 'INVALID_REQUEST');
  assert.equal(frame.receipt.outcome.cleanup.confirmed, true);
  assert.equal((await f.records()).filter(record => record.method === 'turn/start').length, 1);
});

test('startup timeout includes initialize and cannot submit a late prompt', { timeout: 10000 }, async t => {
  const f = await setup(t, 'initialize-hang');
  const frame = resultFrame(await f.launch({ input: { ...f.request, timeoutMs: 150 } }).done, 1);
  assert.equal(frame.receipt.failure.execution.timedOut, true);
  assert.equal(frame.receipt.failure.cleanup.confirmed, true);
  assert.ok(!(await f.records()).some(record => record.method === 'turn/start'));
});

test('output truncation is a failed attempt with a complete cleanup receipt', { timeout: 10000 }, async t => {
  const f = await setup(t, 'output-limit');
  const frame = resultFrame(await f.launch().done, 1);
  assert.equal(frame.receipt.ok, false);
  assert.equal(frame.receipt.outcome.execution.truncated, true);
  assert.equal(frame.receipt.outcome.cleanup.confirmed, true);
});

test('invalid requests and arguments fail before launching any app-server', async t => {
  const f = await setup(t);
  for (const input of ['{bad-json\n', '{}\n', { ...f.request, permissionMode: 'dangerously-bypass-approvals-and-sandbox' },
    { ...f.request, provider: 'ignored-provider' }, { ...f.request, reasoningEffort: '' }, { ...f.request, timeoutMs: -1 },
    { ...f.request, timeoutMs: 0 }, { ...f.request, timeoutMs: 2147483648 },
    { ...f.request, requestId: 'r'.repeat(257) },
    ' '.repeat(LIMITS.requestBytes + 1)]) {
    const frame = resultFrame(await f.launch({ input }).done, 1);
    assert.equal(frame.receipt, null);
    assert.match(frame.error.code, /INVALID_REQUEST|INPUT_LIMIT/);
  }
  const invalidArgs = resultFrame(await f.launch({ args: ['--unsupported'], input: null }).done, 1);
  assert.equal(invalidArgs.error.code, 'INVALID_REQUEST');
  assert.deepEqual(await f.records(), []);
});

test('parent disconnection before input does not wait for the admission timeout', { timeout: 5000 }, async t => {
  const f = await setup(t);
  const run = f.launch({ input: null });
  run.child.stdin.end();
  const frame = resultFrame(await run.done, 1);
  assert.equal(frame.receipt, null);
  assert.equal(frame.error.code, 'PARENT_DISCONNECTED');
  assert.deepEqual(await f.records(), []);
});

test('public request validation always fixes fresh sessions and bounded output', async t => {
  const f = await setup(t);
  const request = machineRequest(f.request, await loadAdapter(), f.env);
  assert.equal(request.threadMode, 'ephemeral');
  assert.equal(request.threadId, undefined);
  assert.equal(request.maxOutputBytes, LIMITS.machineOutputBytes);
  assert.equal(request.command, fixture);
});

test('worker v2 opts into the public persistent thread contract and v1 stays fresh', async t => {
  const f = await setup(t);
  const adapter = await loadAdapter();
  const start = { ...f.request, protocol: 2, threadMode: 'persistent' };
  const parsed = machineRequest(start, adapter, f.env);
  assert.equal(parsed.protocol, 1, 'the installed public machine API remains protocol 1');
  assert.equal(parsed.threadMode, 'persistent');
  assert.equal(parsed.threadId, undefined);
  assert.equal(machineRequest({ ...start, threadId: 'exact-opaque-thread' }, adapter, f.env).threadId, 'exact-opaque-thread');
  for (const input of [{ ...f.request, threadMode: 'persistent' }, { ...f.request, threadId: 'foreign-thread' },
    { ...start, threadMode: 'ephemeral' }, { ...start, threadMode: undefined },
    { ...start, threadId: '' }, { ...start, threadId: '  ' }, { ...start, threadId: 'nul\0thread' }]) {
    assert.throws(() => machineRequest(input, adapter, f.env));
    const frame = resultFrame(await f.launch({ input }).done, 1);
    assert.equal(frame.protocol, input.protocol);
    assert.equal(frame.receipt, null);
    assert.equal(frame.error.code, 'INVALID_REQUEST');
  }
  assert.deepEqual(await f.records(), []);
});

test('persistent history crosses worker processes while accounting only each run delta', { timeout: 15000 }, async t => {
  const f = await setup(t);
  const environment = { ...f.env, TEST_CODEX_PRO_HISTORY: join(f.cwd, 'history.json') };
  const input = { ...f.request, protocol: 2, threadMode: 'persistent', requestId: 'owned-attempt-one' };
  const first = resultFrame(await f.launch({ input, environment }).done);
  assert.equal(first.protocol, 2);
  assert.equal(first.receipt.protocol, 1);
  assert.equal(first.receipt.requested.threadId, null);
  assert.equal(first.receipt.outcome.configuration.source, 'thread/start');
  assert.equal(first.receipt.outcome.usage.run.totalTokens, 108);
  const threadId = first.receipt.outcome.threadId;
  const second = resultFrame(await f.launch({ environment,
    input: { ...input, requestId: 'owned-attempt-two', threadId, prompt: 'Continue the same scoped task.' } }).done);
  assert.equal(second.protocol, 2);
  assert.equal(second.receipt.requestId, 'owned-attempt-two');
  assert.equal(second.receipt.requested.threadId, threadId);
  assert.equal(second.receipt.outcome.threadId, threadId);
  assert.equal(second.receipt.outcome.execution.threadId, threadId);
  assert.equal(second.receipt.outcome.configuration.source, 'thread/resume');
  assert.notEqual(second.receipt.runId, first.receipt.runId);
  assert.notEqual(second.receipt.outcome.execution.turnId, first.receipt.outcome.execution.turnId);
  assert.equal(second.receipt.outcome.usage.status, 'complete');
  assert.equal(second.receipt.outcome.usage.replayedBaseline, true);
  assert.equal(second.receipt.outcome.usage.run.totalTokens, 108);
  assert.equal(second.receipt.outcome.usage.threadTotal.totalTokens, 216);
  assert.equal(second.receipt.costUSD, null);
  absentGroup(first.receipt.outcome.execution);
  absentGroup(second.receipt.outcome.execution);
  const records = await f.records();
  assert.equal(records.filter(record => record.method === 'thread/start').length, 1);
  assert.equal(records.find(record => record.method === 'thread/start').params.ephemeral, false);
  assert.equal(records.filter(record => record.method === 'thread/resume').length, 1);
  assert.equal(records.find(record => record.method === 'thread/resume').params.threadId, threadId);
  assert.equal(records.filter(record => record.method === 'turn/start').length, 2);
});

test('a resumed thread without replayed accounting baseline remains partial', { timeout: 10000 }, async t => {
  const f = await setup(t);
  const environment = { ...f.env, TEST_CODEX_PRO_HISTORY: join(f.cwd, 'history.json') };
  const input = { ...f.request, protocol: 2, threadMode: 'persistent' };
  const first = resultFrame(await f.launch({ input, environment }).done);
  const second = resultFrame(await f.launch({ input: { ...input, requestId: 'attempt-2', threadId: first.receipt.outcome.threadId },
    environment: { ...environment, TEST_CODEX_PRO_SCENARIO: 'resume-no-baseline' } }).done);
  assert.equal(second.receipt.ok, true);
  assert.equal(second.receipt.outcome.usage.status, 'partial');
  assert.equal(second.receipt.outcome.usage.run, undefined);
  assert.equal(second.receipt.outcome.usage.threadTotal.totalTokens, 216);
  assert.equal(second.receipt.costUSD, null);
  absentGroup(second.receipt.outcome.execution);
});

test('resume refusal or wrong thread identity does not silently create a replacement turn', { timeout: 15000 }, async t => {
  const f = await setup(t);
  const environment = { ...f.env, TEST_CODEX_PRO_HISTORY: join(f.cwd, 'history.json') };
  const input = { ...f.request, protocol: 2, threadMode: 'persistent' };
  const first = resultFrame(await f.launch({ input, environment }).done);
  for (const scenario of ['resume-busy', 'resume-no-rollout', 'wrong-resume-thread']) {
    const frame = resultFrame(await f.launch({ input: { ...input, requestId: scenario, threadId: first.receipt.outcome.threadId },
      environment: { ...environment, TEST_CODEX_PRO_SCENARIO: scenario } }).done, 1);
    assert.equal(frame.protocol, 2);
    assert.equal(frame.receipt.ok, false);
    assert.equal(frame.receipt.failure.cleanup.confirmed, true);
    absentGroup(frame.receipt.failure.execution);
  }
  const records = await f.records();
  assert.equal(records.filter(record => record.method === 'thread/start').length, 1);
  assert.equal(records.filter(record => record.method === 'thread/resume').length, 3);
  assert.equal(records.filter(record => record.method === 'turn/start').length, 1);
});

test('missing and mismatched installed dependencies fail locally without fallback', async t => {
  const f = await setup(t);
  const packageRoot = join(f.cwd, 'isolated-worker');
  await mkdir(packageRoot);
  await writeFile(join(packageRoot, 'package.json'), '{"type":"module"}');
  for (const file of ['codex-pro-worker.js', 'codex-pro-bridge.js']) await copyFile(new URL('../src/' + file, import.meta.url), join(packageRoot, file));
  const options = { binary: join(packageRoot, 'codex-pro-worker.js'), args: ['--doctor'], input: null };
  assert.equal(resultFrame(await f.launch(options).done, 1).error.code, 'ADAPTER_UNAVAILABLE');
  const failedRun = resultFrame(await f.launch({ ...options, args: [], input: f.request }).done, 1);
  assert.equal(failedRun.error.code, 'ADAPTER_UNAVAILABLE');
  assert.equal(failedRun.requestId, f.request.requestId);
  const adapter = join(packageRoot, 'node_modules', ADAPTER.package);
  await mkdir(adapter, { recursive: true });
  await writeFile(join(adapter, 'package.json'), JSON.stringify({ name: ADAPTER.package, version: '0.7.0', exports: { './package.json': './package.json' } }));
  assert.equal(resultFrame(await f.launch(options).done, 1).error.code, 'ADAPTER_MISMATCH');
  assert.deepEqual(await f.records(), []);
});
