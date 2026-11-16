import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { chmod, mkdtemp, mkdir, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createInterface } from 'node:readline';
import { runWorker } from '../src/bridge.js';
import { validatePrepare, matchWarmStart, prewarmInput, writeWarmReceipt } from '../src/prewarm.js';
import { PassThrough } from 'node:stream';
import { apply as applyPolicy } from '../src/policy.js';

const fixture = fileURLToPath(new URL('./fixtures/fake-dsh.js', import.meta.url));
const worker = fileURLToPath(new URL('../src/worker.js', import.meta.url));
await chmod(fixture, 0o755);

async function setup(t, scenario = 'ok') {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-prewarm-')));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const receiptDirectory = join(cwd, 'receipt');
  await mkdir(receiptDirectory, { mode: 0o700 });
  const prepared = { type: 'prepare', protocol: 1, warmId: 'warm-owned-123', binding: 'a'.repeat(64), receiptDirectory,
    ttlMs: 3000, cwd, provider: 'deepseek-account', model: 'deepseek-flash', reasoningEffort: 'high', permissionMode: 'read-only' };
  const request = { type: 'start', protocol: 2, requestId: 'formal-review', persistent: true, sessionId: 'previous-review',
    cwd, provider: prepared.provider, model: prepared.model, reasoningEffort: prepared.reasoningEffort,
    permissionMode: prepared.permissionMode, prompt: 'Review this verified candidate.', timeoutMs: 3000 };
  const log = join(cwd, 'sdk.jsonl');
  const options = { env: { PATH: process.env.PATH, FLYWHEEL_DSH_BIN: fixture, TEST_DSH_SCENARIO: scenario, TEST_DSH_LOG: log } };
  const methods = async () => (await readFile(log, 'utf8')).trim().split('\n').map(line => JSON.parse(line).method);
  return { prepared, request, options, methods };
}

test('SDK initialization overlaps checks without creating a Session, then consumes the same runtime once', async t => {
  const f = await setup(t);
  let dispatch, ready;
  const waiting = new Promise(resolve => { ready = resolve; });
  const frames = [];
  const task = runWorker(f.prepared, { ...f.options, emit: frame => frames.push(frame), prewarm: {
    ready, dispatch: new Promise(resolve => { dispatch = resolve; }),
  } });
  await waiting;
  assert.deepEqual(await f.methods(), ['initialize']);
  assert.deepEqual(frames[0], { protocol: 1, type: 'warm-ready', warmId: f.prepared.warmId,
    binding: f.prepared.binding, level: 'sdk-initialized', dispatched: false });
  dispatch(f.request);
  const result = await task;
  assert.equal(result.status, 'ok');
  assert.equal(result.lifecycle.requestId, 'formal-review');
  assert.equal(result.lifecycle.resumeAccepted, true);
  assert.deepEqual(await f.methods(), ['initialize', 'session/resume', 'session/working-directory/get', 'session/prompt', 'session/wait', 'shutdown']);
  await assert.rejects(readFile(join(f.prepared.receiptDirectory, 'terminal.json')), { code: 'ENOENT' });
});

test('unused SDK closes with durable cleanup proof and no prompt or Session', async t => {
  const f = await setup(t);
  let close;
  const frames = [];
  const result = await runWorker(f.prepared, { ...f.options, emit: frame => frames.push(frame), prewarm: {
    ready() { close(null); }, dispatch: new Promise(resolve => { close = resolve; }),
  } });
  assert.equal(result.status, 'closed');
  assert.deepEqual(await f.methods(), ['initialize', 'shutdown']);
  const receipt = JSON.parse(await readFile(join(f.prepared.receiptDirectory, 'terminal.json'), 'utf8'));
  assert.deepEqual(receipt, { protocol: 1, warmId: f.prepared.warmId, binding: f.prepared.binding,
    workerPid: process.pid, dispatched: false, cleanupConfirmed: true });
  assert.equal(frames.at(-1).type, 'warm-result');
});

test('private cleanup receipts bound serialized bytes without changing proof or short diagnostics', async t => {
  for (const [name, message] of [
    ['short', 'initialization-error: "quoted" \\ path\nnext line'],
    ['ascii', 'initialization-error:' + 'x'.repeat(9000)],
    ['multibyte', 'initialization-error:' + '初始失败😀'.repeat(2500)],
    ['escaped', 'initialization-error:' + '"\\\n\t\u0000\ud800'.repeat(2000)],
  ]) {
    await t.test(name, async t => {
      const f = await setup(t);
      f.prepared.warmId = 'w'.repeat(128);
      await writeWarmReceipt(f.prepared, true, { message });
      const raw = await readFile(join(f.prepared.receiptDirectory, 'terminal.json'));
      assert.ok(raw.length <= 8192, `published cleanup receipt is ${raw.length} bytes`);
      assert.equal(raw.at(-1), 10, 'the trailing newline counts toward the limit');
      const { error, ...proof } = JSON.parse(raw.toString('utf8'));
      assert.deepEqual(proof, { protocol: 1, warmId: f.prepared.warmId, binding: f.prepared.binding,
        workerPid: process.pid, dispatched: false, cleanupConfirmed: true });
      if (name === 'short') assert.equal(error, message);
      else {
        assert.ok(error.endsWith(' [truncated]'));
        assert.ok(message.startsWith(error.slice(0, -' [truncated]'.length)));
      }
    });
  }
});

test('private cleanup receipt accepts the exact byte boundary and preserves unknown cleanup', async t => {
  for (const extra of [0, 1]) {
    const f = await setup(t);
    const empty = { protocol: 1, warmId: f.prepared.warmId, binding: f.prepared.binding,
      workerPid: process.pid, dispatched: false, cleanupConfirmed: false, error: '' };
    const message = 'x'.repeat(8192 - Buffer.byteLength(JSON.stringify(empty) + '\n') + extra);
    await writeWarmReceipt(f.prepared, false, { message });
    const raw = await readFile(join(f.prepared.receiptDirectory, 'terminal.json'));
    assert.equal(raw.length, 8192);
    const receipt = JSON.parse(raw.toString('utf8'));
    assert.equal(receipt.cleanupConfirmed, false);
    assert.equal(receipt.dispatched, false);
    if (extra === 0) assert.equal(receipt.error, message);
    else assert.ok(receipt.error.endsWith(' [truncated]'));
  }
});

test('private cleanup receipts never truncate identity fields to fit', async t => {
  const f = await setup(t);
  await assert.rejects(writeWarmReceipt({ ...f.prepared, warmId: 'w'.repeat(9000) }, true),
    /cleanup identity exceeds the private receipt limit/);
  await assert.rejects(readFile(join(f.prepared.receiptDirectory, 'terminal.pending')), { code: 'ENOENT' });
  await assert.rejects(readFile(join(f.prepared.receiptDirectory, 'terminal.json')), { code: 'ENOENT' });
});

for (const scenario of ['forced-shutdown', 'shutdown-corrupt-tail']) {
  test(`${scenario}: unused startup cannot attest safe cleanup`, async t => {
    const f = await setup(t, scenario);
    let close;
    const result = await runWorker(f.prepared, { ...f.options, emit() {}, prewarm: {
      ready() { close(null); }, dispatch: new Promise(resolve => { close = resolve; }),
    } });
    assert.equal(result.status, 'error');
    const receipt = JSON.parse(await readFile(join(f.prepared.receiptDirectory, 'terminal.json'), 'utf8'));
    assert.equal(receipt.cleanupConfirmed, false);
    assert.ok(!(await f.methods()).includes('session/prompt'));
  });
}

test('prewarm expiry and changed formal binding never submit a model prompt', async t => {
  for (const change of ['expire', 'model', 'maxTokens']) {
    const f = await setup(t);
    const prepared = { ...f.prepared, ttlMs: change === 'expire' ? 100 : 3000 };
    const frames = [];
    const result = await runWorker(prepared, { ...f.options, emit: frame => frames.push(frame), prewarm: {
      ready() {}, dispatch: change === 'expire' ? new Promise(() => {})
        : Promise.resolve(change === 'model' ? { ...f.request, model: 'changed' } : { ...f.request, maxTokens: 1 }),
    } });
    assert.equal(result.status, 'error');
    assert.ok(!(await f.methods()).includes('session/prompt'));
  }
});

test('real two-phase worker uses stdin as a lifeline, including before formal dispatch', async t => {
  const f = await setup(t);
  const child = spawn(process.execPath, [worker, '--prewarm'], { env: f.options.env, stdio: ['pipe', 'pipe', 'pipe'] });
  let stderr = '';
  child.stderr.on('data', chunk => { stderr += chunk; });
  const exit = new Promise(resolve => child.once('close', (code, signal) => resolve({ code, signal })));
  const frames = [];
  const ready = new Promise(resolve => {
    createInterface({ input: child.stdout }).on('line', line => {
      const frame = JSON.parse(line); frames.push(frame);
      if (frame.type === 'warm-ready') resolve();
    });
  });
  child.stdin.write(JSON.stringify(f.prepared) + '\n');
  await ready;
  assert.deepEqual(await f.methods(), ['initialize']);
  child.stdin.end();
  const ended = await exit;
  assert.equal(ended.signal, null, stderr);
  assert.equal(ended.code, 143, stderr);
  assert.equal(frames.at(-1).cleanupConfirmed, true);
  assert.deepEqual(await f.methods(), ['initialize', 'shutdown']);
});

test('strict prepare validation and transport reject unexpected, duplicate and early start frames', async t => {
  const f = await setup(t);
  for (const patch of [{ prompt: 'forbidden' }, { sessionId: 'forbidden' }, { ttlMs: 60001 }, { binding: 'wrong' },
    { permissionMode: 'silent-full-access' }, { receiptDirectory: 'relative' }, { warmId: '' },
    { warmId: 12345678 }, { binding: ['a'.repeat(64)] }, { warmId: ['valid-looking-id'] },
    { idleTimeoutMs: 0 }, { idleTimeoutMs: 1.5 }, { idleTimeoutMs: 86400001 }]) {
    await assert.rejects(validatePrepare({ ...f.prepared, ...patch }));
  }
  assert.throws(() => matchWarmStart(f.prepared, { ...f.request, model: 'changed' }));
  const prepared = await validatePrepare({ ...f.prepared, idleTimeoutMs: 1200 });
  assert.doesNotThrow(() => matchWarmStart(prepared, { ...f.request, idleTimeoutMs: 1200 }));
  assert.throws(() => matchWarmStart(prepared, { ...f.request, idleTimeoutMs: 2400 }));
  assert.throws(() => matchWarmStart(prepared, f.request));
  for (const payload of ['{bad}\n', Buffer.from([0xff, 10]), JSON.stringify({ ...f.prepared, type: 'start' }) + '\n']) {
    const controller = new AbortController(), input = new PassThrough();
    const transport = prewarmInput(controller, input);
    input.write(payload);
    if (typeof payload === 'string' && payload.includes('"type":"start"')) {
      await assert.rejects(validatePrepare(await transport.admission));
    } else await assert.rejects(transport.admission);
    transport.stop(); input.destroy();
  }
  for (const frame of [{ type: 'start', protocol: 1, warmId: f.prepared.warmId, request: f.request },
    { type: 'close', protocol: 1, warmId: 'wrong' }]) {
    const controller = new AbortController(), input = new PassThrough();
    const transport = prewarmInput(controller, input);
    input.write(JSON.stringify(f.prepared) + '\n'); await transport.admission;
    input.write(JSON.stringify(frame) + '\n');
    await assert.rejects(transport.dispatch);
    assert.equal(controller.signal.aborted, true);
    transport.stop(); input.destroy();
  }
});

for (const scenario of ['early-close', 'forced-shutdown']) {
  test(`real worker ${scenario}: terminal exit agrees with independent cleanup evidence`, async t => {
    const f = await setup(t, scenario === 'forced-shutdown' ? scenario : 'ok');
    const child = spawn(process.execPath, [worker, '--prewarm'], { env: f.options.env, stdio: ['pipe', 'pipe', 'pipe'] });
    const frames = [];
    let stderr = '';
    child.stderr.on('data', chunk => { stderr += chunk; });
    const exit = new Promise(resolve => child.once('close', (code, signal) => resolve({ code, signal })));
    const close = JSON.stringify({ type: 'close', protocol: 1, warmId: f.prepared.warmId }) + '\n';
    createInterface({ input: child.stdout }).on('line', line => {
      const frame = JSON.parse(line); frames.push(frame);
      if (frame.type === 'warm-ready' && scenario === 'forced-shutdown') child.stdin.write(close);
    });
    child.stdin.write(JSON.stringify(f.prepared) + '\n' + (scenario === 'early-close' ? close : ''));
    const ended = await exit;
    child.stdin.destroy();
    assert.equal(ended.signal, null, stderr);
    const receipt = JSON.parse(await readFile(join(f.prepared.receiptDirectory, 'terminal.json'), 'utf8'));
    assert.equal(receipt.dispatched, false);
    assert.equal(receipt.cleanupConfirmed, scenario === 'early-close');
    if (scenario === 'forced-shutdown') {
      assert.equal(ended.code, 1, stderr);
      assert.equal(frames.at(-1).status, 'error');
      assert.equal(frames.at(-1).cleanupConfirmed, false);
    } else {
      assert.equal(ended.code, 143, stderr);
      assert.equal(frames.at(-1).cleanupConfirmed, true);
      await assert.rejects(readFile(join(f.prepared.cwd, 'sdk.jsonl')), { code: 'ENOENT' });
    }
  });
}

test('deferred public permission hook requires exact private admission and actual restored Session', async t => {
  const f = await setup(t);
  const receiptPath = join(f.prepared.receiptDirectory, 'policy.json');
  const deferredAdmissionPath = join(f.prepared.receiptDirectory, 'admission.json');
  const identity = { requestId: f.prepared.warmId, nonce: 'private-nonce' };
  let created;
  const ctx = { on(_event, callback) { created = callback; },
    shell: { sandboxMode: 'read-only', resolve: x => x, execute: async () => ({ result: async () => ({ exitCode: 0,
      sandbox: { mode: 'read-only', enforcement: 'full' } }) }) },
    sandboxPolicy: { resolve: () => ({ mode: 'read-only', workspaceRoot: f.prepared.cwd }) },
    approval: { overrideOf: () => 'never' } };
  const config = { ...identity, receiptPath, deferredAdmissionPath, cwd: f.prepared.cwd, permissionMode: 'read-only' };
  await applyPolicy(ctx, config);
  const agent = { session: { id: f.request.sessionId, append() {} } };
  await assert.rejects(created({ agent }), { code: 'ENOENT' });
  await writeFile(deferredAdmissionPath, JSON.stringify({ ...identity, sessionId: 'unrelated' }), { mode: 0o600 });
  await assert.rejects(created({ agent }), /unrelated SDK session/);
  await writeFile(deferredAdmissionPath, JSON.stringify({ ...identity, sessionId: f.request.sessionId }));
  await created({ agent });
  const receipt = JSON.parse(await readFile(receiptPath, 'utf8'));
  assert.equal(receipt.sessionId, f.request.sessionId);
  assert.equal(receipt.requestId, f.prepared.warmId);
  assert.equal(receipt.approvalPolicy, 'never');
});
