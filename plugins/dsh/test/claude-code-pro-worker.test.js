import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';
import { chmod, copyFile, mkdir, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ADAPTER, loadAdapter, machineRequest, invokeAdapter, recoverAdapter, runtimeOptions } from '../src/claude-code-pro-bridge.js';

const require = createRequire(import.meta.url);
let installed = false;
try { require.resolve('dsh-subagent-claude-code-pro/package.json'); installed = true; }
catch (error) { if (error.code !== 'MODULE_NOT_FOUND') throw error; }
const liveContract = { skip: installed ? false : 'optional Claude Code Pro 0.5.1 not installed; install the source candidate to run this public-API integration', timeout: 15000 };
const worker = fileURLToPath(new URL('../dist/claude-code-pro-worker.js', import.meta.url));
const fixture = fileURLToPath(new URL('./fixtures/fake-claude-code-pro.js', import.meta.url));
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

async function setup(t, scenario = 'ok') {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel claude $() ')));
  const runs = [];
  await writeFile(join(cwd, 'claude-fixture.json'), JSON.stringify({ scenario }));
  const executable = join(cwd, 'offline-claude.mjs');
  await copyFile(fixture, executable);
  await chmod(executable, 0o755);
  const env = { ...process.env, FLYWHEEL_CLAUDE_BIN: executable, CLAUDE_CONFIG_DIR: join(cwd, 'isolated-claude') };
  for (const key of ['NODE_OPTIONS', 'NODE_PATH', 'ANTHROPIC_API_KEY', 'ANTHROPIC_AUTH_TOKEN', 'ANTHROPIC_BASE_URL', 'CLAUDE_CODE_OAUTH_TOKEN']) delete env[key];
  const request = { type: 'start', protocol: 1, requestId: 'claude-attempt-1', cwd, model: 'fixture-model',
    reasoningEffort: 'high', permissionMode: 'danger-full-access', prompt: 'offline task', timeoutMs: 5000 };
  const launch = ({ input = request, args = [], binary = worker, environment = env } = {}) => {
    const child = spawn(process.execPath, [binary, ...args], { cwd, env: environment, stdio: ['pipe', 'pipe', 'pipe'] });
    const run = { child, stdout: '', stderr: '', closed: false };
    child.stdout.setEncoding('utf8'); child.stderr.setEncoding('utf8');
    child.stdout.on('data', text => { run.stdout += text; });
    child.stderr.on('data', text => { run.stderr += text; });
    child.stdin.on('error', () => {});
    run.done = new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('close', (code, signal) => { run.closed = true; resolve({ code, signal, stdout: run.stdout, stderr: run.stderr }); });
    });
    if (input !== null) child.stdin.write(typeof input === 'string' ? input : JSON.stringify(input) + '\n');
    runs.push(run);
    return run;
  };
  const records = async () => (await readFile(join(cwd, 'claude-fixture.jsonl'), 'utf8').catch(() => '')).trim().split('\n').filter(Boolean).map(JSON.parse);
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
  return { cwd, request, env, launch, records };
}

function resultFrame(result, code = 0) {
  assert.equal(result.code, code, result.stderr + result.stdout);
  assert.equal(result.signal, null);
  const frames = result.stdout.trim().split('\n').map(JSON.parse);
  assert.equal(frames.filter(frame => frame.type === 'result').length, 1);
  assert.ok(frames.slice(0, -1).every(frame => ['progress', 'native-event'].includes(frame.type)));
  assert.deepEqual(frames.at(-1).adapter, ADAPTER);
  return frames.at(-1);
}

async function waitStarted(f) {
  const deadline = Date.now() + 5000;
  while ((await f.records()).length === 0) {
    assert.ok(Date.now() < deadline, 'offline Claude process never started');
    await delay(15);
  }
}

test('request translation preserves explicit native choices and rejects unsupported policy before dispatch', () => {
  class MachineInputError extends Error {}
  const api = { MachineInputError, parseMachineRequest: value => value };
  const request = { type: 'start', protocol: 1, requestId: 'r', cwd: '/tmp', prompt: 'task', model: 'sonnet',
    reasoningEffort: 'max', permissionMode: 'danger-full-access', timeoutMs: 3000 };
  const machine = machineRequest(request, api, { FLYWHEEL_CLAUDE_BIN: '/custom/claude' });
  assert.equal(machine.permissionMode, 'bypassPermissions');
  assert.equal(machine.model, 'sonnet');
  assert.equal(machine.effort, 'max');
  assert.equal(machine.sessionMode, 'ephemeral');
  assert.equal(machine.command, '/custom/claude');
  const resumed = machineRequest({ ...request, protocol: 2, threadMode: 'persistent', threadId: 'opaque-in-bridge' }, api, {});
  assert.equal(resumed.sessionId, 'opaque-in-bridge');
  assert.equal(resumed.sessionMode, 'persistent');
  for (const change of [ { permissionMode: 'read-only' }, { permissionMode: 'workspace-write' }, { protocol: 3 },
    { protocol: 2 }, { threadId: 'not-in-v1' }, { promptExtra: true }, { model: '' }, { requestId: '\0' }, { timeoutMs: 0 } ]) {
    assert.throws(() => machineRequest({ ...request, ...change }, api, {}), { code: 'INVALID_REQUEST' });
  }
  assert.throws(() => machineRequest(request, { ...api, parseMachineRequest() { throw new MachineInputError('unsupported model'); } }, {}), /unsupported model/);
  assert.deepEqual(runtimeOptions({ ANTHROPIC_API_KEY: 'explicit-test-only', DSH_SECRET: 'never', NODE_OPTIONS: 'never' }),
    { credentialEnv: { ANTHROPIC_API_KEY: 'explicit-test-only' } });
});

test('missing, stale and incompatible optional peers prove non-dispatch without breaking other adapters', async t => {
  const f = await setup(t);
  const directory = join(f.cwd, 'isolated-worker');
  await mkdir(directory);
  await writeFile(join(directory, 'package.json'), '{"type":"module"}');
  for (const name of ['claude-code-pro-worker.js', 'claude-code-pro-bridge.js', 'machine-worker.js']) {
    await copyFile(new URL('../src/' + name, import.meta.url), join(directory, name));
  }
  const options = { binary: join(directory, 'claude-code-pro-worker.js') };
  const missing = resultFrame(await f.launch(options).done, 1);
  assert.equal(missing.receipt, null);
  assert.equal(missing.error.code, 'ADAPTER_UNAVAILABLE');
  assert.deepEqual(missing.admission, { dispatched: false, cleanupConfirmed: true });
  const peer = join(directory, 'node_modules', ADAPTER.package);
  await mkdir(peer, { recursive: true });
  const manifest = { name: ADAPTER.package, version: '0.5.0', type: 'module', exports: { './package.json': './package.json', './machine': './machine.js' } };
  await writeFile(join(peer, 'package.json'), JSON.stringify(manifest));
  assert.equal(resultFrame(await f.launch(options).done, 1).error.code, 'ADAPTER_MISMATCH');
  await writeFile(join(peer, 'package.json'), JSON.stringify({ ...manifest, version: ADAPTER.version }));
  await writeFile(join(peer, 'machine.js'), 'export const MACHINE_PROTOCOL_VERSION=999;');
  assert.equal(resultFrame(await f.launch(options).done, 1).error.code, 'ADAPTER_MISMATCH');
  await writeFile(join(peer, 'machine.js'), 'export const MACHINE_PROTOCOL_VERSION=1; export const MACHINE_LIFECYCLE_VERSION=1; export const MACHINE_LIFECYCLE_CAPABILITIES={durableJournal:true,readOnlyRecovery:true,threadReadyBeforeDispatch:false}; export const MACHINE_NATIVE_VERSION=1; export const MACHINE_NATIVE_CAPABILITIES={version:1,agents:true,workflows:true}; export const validateNativeReviewRequest=()=>{}; export const validateInteractiveTeamRequest=()=>{}; export const runNativeReview=()=>{}; export const runInteractiveTeam=()=>{}; export const recoverNativeJob=()=>{}; export const controlInteractiveTeam=()=>{}; export async function recoverMachine(){}; export class MachineInputError extends Error{}; export const parseMachineRequest=x=>x; export async function runMachine(){throw new Error("unknown after invocation")}');
  const unknown = resultFrame(await f.launch(options).done, 1);
  assert.equal(unknown.admission, undefined);
  assert.equal(unknown.error.code, 'WORKER_ERROR');
  assert.deepEqual(await f.records(), []);
});

test('installed public machine API receives exact model, effort, full access and reports actual usage', liveContract, async t => {
  const f = await setup(t);
  const frame = resultFrame(await f.launch().done);
  const r = frame.receipt;
  assert.equal(frame.error, null);
  assert.equal(r.ok, true);
  assert.equal(r.configuration.source, 'cli-init');
  assert.equal(r.configuration.observed.cwd, f.cwd);
  assert.equal(r.configuration.observed.permissionMode, 'bypassPermissions');
  assert.equal(r.requested.model, f.request.model);
  assert.equal(r.requested.effort, 'high');
  assert.equal(r.session.mode, 'ephemeral');
  assert.equal(r.session.resumable, false);
  assert.equal(r.cleanup.confirmed, true);
  assert.equal(r.execution.outcome, 'completed');
  assert.equal(r.usage.totalTokens, 22);
  assert.equal(r.reportedCostUSD, 0.12);
  assert.equal(r.costUSD, null);
  assert.equal(r.costStatus, 'unknown');
  assert.throws(() => process.kill(-r.execution.processGroupId, 0), { code: 'ESRCH' });
  const [record] = await f.records();
  assert.ok(record.argv.includes('--no-session-persistence'));
  assert.ok(record.argv.includes('--dangerously-skip-permissions'));
  assert.equal(record.credentialPresent, false);
});

test('installed public API carries each child effort to --agents without changing parent effort', liveContract, async t => {
  const f = await setup(t);
  for (const effort of ['low', 'medium', 'high', 'xhigh', 'max']) {
    const definitions = { scout: { description: 'Inspect the target', prompt: 'Report concrete findings', model: 'sonnet', effort } };
    const native = { version: 1, agents: { enabled: true, definitions } };
    const frame = resultFrame(await f.launch({ input: { ...f.request, requestId: 'child-effort-' + effort, native } }).done);
    assert.equal(frame.error, null);
    assert.equal(frame.receipt.ok, true);
    assert.deepEqual(frame.receipt.native.requested, native);
    const record = (await f.records()).at(-1);
    const option = name => record.argv[record.argv.indexOf(name) + 1];
    assert.deepEqual(JSON.parse(option('--agents')), definitions);
    assert.equal(option('--effort'), 'high');
  }
});

test('the installed public parser rejects invalid child effort before native dispatch', liveContract, async t => {
  const f = await setup(t);
  const native = { version: 1, agents: { enabled: true, definitions: {
    scout: { description: 'Inspect the target', prompt: 'Report findings', effort: 'ultra' },
  } } };
  const frame = resultFrame(await f.launch({ input: { ...f.request, native } }).done, 1);
  assert.equal(frame.receipt, null);
  assert.equal(frame.error.code, 'INVALID_REQUEST');
  assert.deepEqual(frame.admission, { dispatched: false, cleanupConfirmed: true });
  assert.deepEqual(await f.records(), []);
});

test('persistent repair resumes only the provided session across independent worker processes', liveContract, async t => {
  const f = await setup(t);
  const input = { ...f.request, protocol: 2, threadMode: 'persistent' };
  const first = resultFrame(await f.launch({ input }).done).receipt;
  const next = resultFrame(await f.launch({ input: { ...input, requestId: 'repair', threadId: first.session.id, prompt: 'repair' } }).done).receipt;
  assert.equal(first.session.resumable, true);
  assert.equal(next.session.id, first.session.id);
  assert.equal(next.session.resumed, true);
  assert.match(next.output[0].text, /prior:offline task\|echo:repair/);
  assert.equal(next.usage.totalTokens, 22);
  assert.notEqual(next.runId, first.runId);
  const calls = await f.records();
  assert.ok(calls[0].argv.includes('--session-id'));
  assert.ok(calls[1].argv.includes('--resume'));
  assert.ok(!calls[1].argv.includes('--session-id'));
});

test('optional adapter doctor imports the public API without running Claude', liveContract, async t => {
  const f = await setup(t);
  assert.equal((await loadAdapter()).MACHINE_PROTOCOL_VERSION, 1);
  const out = await f.launch({ input: null, args: ['--doctor'] }).done;
  assert.equal(out.code, 0, out.stderr);
  assert.deepEqual(JSON.parse(out.stdout), { protocol: 1, type: 'doctor', package: ADAPTER.package,
    version: ADAPTER.version, machineProtocol: 1, nativeVersion: 1, available: true, inferenceTested: false });
  assert.deepEqual(await f.records(), []);
});

for (const role of ['execute', 'design-plan', 'design-review', 'plan-review']) {
test(`${role}: native Agent metadata stays separate from root output and managed identity reaches events`, liveContract, async t => {
  const f = await setup(t, 'native-child');
  const native = { version: 1, agents: { enabled: true }, workflows: { enabled: true, maxConcurrent: 3 } };
  const managed = { version: 1, runId: 'run', invocationId: f.request.requestId, role, scope: role + ':candidate', channel: 'c', taskDir: f.cwd };
  const result = await f.launch({ input: { ...f.request, native, managed } }).done;
  const receipt = resultFrame(result).receipt;
  assert.deepEqual(receipt.native.requested, native);
  assert.equal(receipt.native.effective.matched, true);
  assert.equal(receipt.native.completion.confirmed, true);
  assert.equal(receipt.native.children[0].usage.total_tokens, 10);
  assert.equal(receipt.native.usage.childSnapshotsAreAdditive, false);
  assert.equal(receipt.usage.totalTokens, 22);
  const events = result.stdout.trim().split('\n').map(JSON.parse).filter(f => f.type === 'native-event');
  assert.ok(events.length > 0);
  assert.ok(events.every(f => JSON.stringify(f.managed) === JSON.stringify(managed)));
});
}

test('machine schema and image requests reach the CLI through the pinned public parser', liveContract, async t => {
  const f = await setup(t);
  const frame = resultFrame(await f.launch({ input: { ...f.request, native: { version: 1 }, maxBudgetUsd: 2,
    images: [{ mimeType: 'image/png', data: 'aGVsbG8=' }],
    outputSchema: { type: 'object', properties: { answer: { type: 'integer' } }, required: ['answer'] } } }).done);
  assert.deepEqual(frame.receipt.structured, { answer: 42 });
  const [call] = await f.records();
  assert.equal(call.userContent[1].type, 'image');
  assert.equal(call.argv[call.argv.indexOf('--max-budget-usd') + 1], '2');
});

test('dedicated jobs preflight once and route through their independent public interfaces', async () => {
  class MachineInputError extends Error {}
  const calls = [], api = { MachineInputError,
    validateNativeReviewRequest: request => { calls.push(['review-validate', request]); if (request.consent !== true) throw new MachineInputError('consent'); },
    validateInteractiveTeamRequest: request => calls.push(['team-validate', request]),
    runNativeReview: (request, options) => ({ kind: 'review', request, options }),
    runInteractiveTeam: (request, options) => ({ kind: 'team', request, options }),
    recoverNativeJob: request => ({ kind: 'native-recovery', request }) };
  const value = { type: 'start', protocol: 3, requestId: 'job', cwd: '/tmp', model: 'sonnet', reasoningEffort: 'high',
    prompt: 'team task', permissionMode: 'danger-full-access', threadMode: 'ephemeral', recoveryDirectory: '/tmp/evidence' };
  const remote = machineRequest({ ...value, nativeOperation: { kind: 'remote-review', target: 'main', consent: true } }, api, {});
  assert.equal(invokeAdapter(api, remote, {}).kind, 'review');
  assert.equal(remote.nativeJob.request.model, undefined);
  assert.equal(remote.nativeJob.request.prompt, undefined);
  const team = machineRequest({ ...value, nativeOperation: { kind: 'interactive-team', columns: 90 }, native: { version: 1, agents: { enabled: true } } }, api, {});
  assert.equal(invokeAdapter(api, team, {}).kind, 'team');
  assert.equal(team.nativeJob.request.prompt, value.prompt);
  assert.equal(team.nativeJob.request.permissionMode, 'bypassPermissions');
  assert.equal(recoverAdapter(api, { runId: 'original' }, { nativeOperation: { kind: 'remote-review' } }).kind, 'native-recovery');
  for (const changed of [{ nativeOperation: { kind: 'remote-review', consent: false } },
    { nativeOperation: { kind: 'remote-review', consent: true }, native: { version: 1 } },
    { nativeOperation: { kind: 'interactive-team' }, threadMode: 'persistent' },
    { nativeOperation: { kind: 'interactive-team', guessedControl: true } }]) {
    assert.throws(() => machineRequest({ ...value, ...changed }, api, {}), { code: 'INVALID_REQUEST' });
  }
  assert.equal(calls[0][0], 'review-validate');
});

for (const scenario of ['wrong-cwd', 'wrong-model', 'wrong-permission', 'wrong-session', 'duplicate-result', 'malformed-after-terminal', 'missing-result', 'abnormal-exit']) {
  test(`${scenario} cannot yield a reusable successful Claude result`, liveContract, async t => {
    const f = await setup(t, scenario);
    const frame = resultFrame(await f.launch().done, 1);
    assert.equal(frame.receipt.ok, false);
    assert.equal(frame.receipt.session.resumable, false);
    assert.equal(frame.receipt.cleanup.confirmed, true);
  });
}

test('explicit channel credentials reach only the selected native backend', liveContract, async t => {
  const f = await setup(t);
  const environment = { ...f.env, ANTHROPIC_API_KEY: 'offline-placeholder', DSH_SECRET: 'must-not-leak' };
  resultFrame(await f.launch({ environment }).done);
  const [record] = await f.records();
  assert.equal(record.credentialPresent, true);
  assert.equal(record.leakedDSH, false);
});

test('an unavailable resume fails once without creating a replacement session', liveContract, async t => {
  const f = await setup(t, 'resume-missing');
  const threadId = 'f96acc3d-f4c4-4d5d-b569-71215ba3cd92';
  const receipt = resultFrame(await f.launch({ input: { ...f.request, protocol: 2, threadMode: 'persistent', threadId } }).done, 1).receipt;
  assert.equal(receipt.ok, false);
  assert.equal(receipt.session.resumable, false);
  assert.equal(receipt.cleanup.confirmed, true);
  const calls = await f.records();
  assert.equal(calls.length, 1);
  assert.equal(calls[0].argv[calls[0].argv.indexOf('--resume') + 1], threadId);
  assert.ok(!calls[0].argv.includes('--session-id'));
});

test('the native deadline returns a failed receipt after cleanup', liveContract, async t => {
  const f = await setup(t, 'hang');
  const receipt = resultFrame(await f.launch({ input: { ...f.request, timeoutMs: 350 } }).done, 1).receipt;
  assert.equal(receipt.execution.timedOut, true);
  assert.equal(receipt.cleanup.confirmed, true);
  assert.equal(receipt.session.resumable, false);
  assert.throws(() => process.kill(-receipt.execution.processGroupId, 0), { code: 'ESRCH' });
});

test('an oversized native output fails with confirmed cleanup', liveContract, async t => {
  const f = await setup(t, 'progress-hang');
  const frame = resultFrame(await f.launch().done, 1);
  assert.equal(frame.receipt.ok, false);
  assert.equal(frame.receipt.cleanup.confirmed, true);
  assert.equal(frame.receipt.session.resumable, false);
  assert.ok(frame.error?.code === 'OUTPUT_LIMIT' || frame.receipt.execution.truncated);
});

for (const scenario of ['partial-usage', 'unknown-usage']) {
  test(`${scenario} preserves unknown counters and subscription cost`, liveContract, async t => {
    const f = await setup(t, scenario);
    const r = resultFrame(await f.launch().done).receipt;
    assert.equal(r.usage.status, scenario === 'partial-usage' ? 'partial' : 'unknown');
    assert.equal(r.usage.cacheCreationTokens, null);
    assert.equal(r.costUSD, null);
  });
}

for (const scenario of ['hang', 'stubborn']) {
  test(`${scenario}: SIGTERM waits for the owned Claude group to stop`, liveContract, async t => {
    const f = await setup(t, scenario);
    const run = f.launch();
    await waitStarted(f);
    run.child.kill('SIGTERM');
    const frame = resultFrame(await run.done, 143);
    assert.equal(frame.receipt.ok, false);
    assert.equal(frame.receipt.execution.cancelled, true);
    assert.equal(frame.receipt.cleanup.confirmed, true);
    assert.throws(() => process.kill(-frame.receipt.execution.processGroupId, 0), { code: 'ESRCH' });
  });
}

test('parent disconnect and an extra request cancel rather than dispatching a second task', liveContract, async t => {
  for (const extra of [null, '{}\n']) {
    const f = await setup(t, 'hang');
    const run = f.launch();
    await waitStarted(f);
    if (extra === null) run.child.stdin.end(); else run.child.stdin.write(extra);
    const frame = resultFrame(await run.done, 1);
    assert.equal(frame.error.code, extra === null ? 'PARENT_DISCONNECTED' : 'INVALID_REQUEST');
    assert.equal(frame.receipt.cleanup.confirmed, true);
    assert.equal(frame.receipt.ok, false);
    assert.equal((await f.records()).length, 1);
  }
});

test('SIGKILL of the worker closes its guardian lifeline and stops native work', liveContract, async t => {
  const f = await setup(t, 'stubborn');
  const run = f.launch();
  await waitStarted(f);
  const [{ pid }] = await f.records();
  t.after(() => { try { process.kill(-pid, 'SIGKILL'); } catch (error) { if (error.code !== 'ESRCH') throw error; } });
  run.child.kill('SIGKILL');
  const result = await run.done;
  assert.equal(result.code, null);
  assert.equal(result.signal, 'SIGKILL');
  assert.doesNotMatch(result.stdout, /"type":"result"/);
  const deadline = Date.now() + 8000;
  for (;;) {
    try { process.kill(-pid, 0); }
    catch (error) {
      if (error.code === 'ESRCH') break;
      // Darwin can temporarily report EPERM while a killed group is being
      // reaped. Keep waiting under the same deadline; only ESRCH proves exit.
      assert.equal(error.code, 'EPERM');
    }
    assert.ok(Date.now() < deadline, 'native Claude process group survived loss of its worker');
    await delay(20);
  }
});
