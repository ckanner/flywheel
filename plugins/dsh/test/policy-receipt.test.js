import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { chmod, mkdir, mkdtemp, readFile, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { promisify } from 'node:util';
import { apply as applyPolicy } from '../src/policy.js';
import { readPolicyReceipt } from '../src/protocol.js';

async function fixture(t) {
  const directory = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-owned-receipt-')));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const identity = { requestId: 'current-invocation', sessionId: 'owned-session', nonce: 'private-nonce' };
  const receiptPath = join(directory, 'policy.json');
  const config = { ...identity, receiptPath, cwd: directory, permissionMode: 'read-only' };
  const receipt = { protocol: 1, ...identity, cwd: directory, permissionMode: 'read-only',
    approvalPolicy: 'never', confinedShell: true, enforcement: 'full' };
  return { directory, identity, receiptPath, config, receipt };
}

test('admission receipt is bounded, private, physical and correlated with this invocation', async t => {
  const f = await fixture(t);
  const save = value => writeFile(f.receiptPath, JSON.stringify(value), { mode: 0o600 });
  await save(f.receipt);
  assert.deepEqual(await readPolicyReceipt(f.receiptPath, f.identity), f.receipt);
  for (const key of ['requestId', 'sessionId', 'nonce']) {
    await assert.rejects(readPolicyReceipt(f.receiptPath, { ...f.identity, [key]: 'unrelated' }), /identity mismatch/);
  }
  for (const malformed of [{}, [], null]) {
    await save(malformed);
    await assert.rejects(readPolicyReceipt(f.receiptPath, f.identity), /identity mismatch/);
  }
  await writeFile(f.receiptPath, 'not-json');
  await assert.rejects(readPolicyReceipt(f.receiptPath, f.identity), SyntaxError);
  await writeFile(f.receiptPath, 'x'.repeat(65537));
  await assert.rejects(readPolicyReceipt(f.receiptPath, f.identity), /bounded private regular file/);
  await save(f.receipt);
  await chmod(f.receiptPath, 0o644);
  await assert.rejects(readPolicyReceipt(f.receiptPath, f.identity), /bounded private regular file/);
  await chmod(f.receiptPath, 0o600);
  await chmod(f.directory, 0o755);
  await assert.rejects(readPolicyReceipt(f.receiptPath, f.identity), /directory is not private/);
  await chmod(f.directory, 0o700);
  await assert.rejects(readPolicyReceipt('.', f.identity), /directory is not physical/);
  await assert.rejects(readPolicyReceipt(f.directory, f.identity));
});

test('receipt reads reject symlinks, escaped paths and non-regular files before reading', async t => {
  const f = await fixture(t);
  const target = join(f.directory, 'real.json');
  await writeFile(target, JSON.stringify(f.receipt), { mode: 0o600 });
  await symlink(target, f.receiptPath);
  await assert.rejects(readPolicyReceipt(f.receiptPath, f.identity), { code: 'ELOOP' });
  const alias = join(f.directory, 'alias');
  await symlink(f.directory, alias);
  await assert.rejects(readPolicyReceipt(join(alias, 'real.json'), f.identity), /directory is not physical/);
  const nested = join(f.directory, 'nested');
  await mkdir(nested, { mode: 0o700 });
  await assert.rejects(readPolicyReceipt(`${nested}/../real.json`, f.identity), /directory is not physical/);
  await assert.rejects(readPolicyReceipt(nested, f.identity), /bounded private regular file/);
  if (process.platform !== 'win32') {
    const fifo = join(f.directory, 'fifo');
    await promisify(execFile)('mkfifo', [fifo]);
    await chmod(fifo, 0o600);
    await assert.rejects(readPolicyReceipt(fifo, f.identity), /bounded private regular file/);
  }
  assert.deepEqual(JSON.parse(await readFile(target, 'utf8')), f.receipt, 'no rejected read changes the target');
});

function policyContext(config) {
  const events = [];
  let callback;
  const ctx = {
    get: () => undefined,
    on: (name, fn) => { assert.equal(name, 'agent/created'); callback = fn; },
    shell: {
      sandboxMode: 'read-only', resolve: spec => spec,
      execute: async () => ({ result: async () => ({ exitCode: 0, sandbox: { mode: 'read-only', enforcement: 'full' } }) }),
    },
    sandboxPolicy: { resolve: () => ({ mode: config.permissionMode, workspaceRoot: config.cwd }) },
    approval: { overrideOf: () => 'never' },
  };
  return { ctx, events, created: () => callback({ agent: { session: {
    id: config.sessionId, append: (type, data) => events.push({ type, data }),
  } } }) };
}

test('policy publication rejects invalid ownership and cannot follow an existing receipt symlink', async t => {
  const f = await fixture(t);
  const { ctx, created } = policyContext(f.config);
  for (const invalid of [{ receiptPath: 'relative.json' }, { receiptPath: undefined },
    { requestId: '' }, { sessionId: '' }, { nonce: '' }, { nonce: 'bad\0nonce' }]) {
    await assert.rejects(applyPolicy(ctx, { ...f.config, ...invalid }), /owned, correlated admission receipt/);
  }
  const alias = join(f.directory, 'alias');
  await symlink(f.directory, alias);
  await assert.rejects(applyPolicy(ctx, { ...f.config, receiptPath: join(alias, 'policy.json') }), /private physical directory/);
  const nested = join(f.directory, 'nested');
  await mkdir(nested, { mode: 0o700 });
  await assert.rejects(applyPolicy(ctx, { ...f.config, receiptPath: `${nested}/../policy.json` }), /private physical directory/);
  await chmod(f.directory, 0o755);
  await assert.rejects(applyPolicy(ctx, f.config), /private physical directory/);
  await chmod(f.directory, 0o700);
  const outside = join(nested, 'untouched.json');
  await writeFile(outside, 'untouched', { mode: 0o600 });
  await symlink(outside, f.receiptPath);
  await applyPolicy(ctx, f.config);
  await assert.rejects(created(), { code: 'EEXIST' });
  assert.equal(await readFile(outside, 'utf8'), 'untouched');
});

test('policy publication cannot follow a pre-existing staging symlink', async t => {
  const f = await fixture(t);
  const target = join(f.directory, 'untouched.json');
  await writeFile(target, 'untouched', { mode: 0o600 });
  await symlink(target, f.receiptPath + '.pending');
  const { ctx, created } = policyContext(f.config);
  await applyPolicy(ctx, f.config);
  await assert.rejects(created(), { code: 'EEXIST' });
  assert.equal(await readFile(target, 'utf8'), 'untouched');
});

test('an explicitly requested fresh title requires the public service and a matching persisted snapshot', async t => {
  for (const service of [undefined, { rename() {} },
    { rename: () => ({ title: '' }), get: () => ({ title: '' }) },
    { rename: () => ({ title: 'accepted' }), get: () => ({ title: 'different' }) }]) {
    const f = await fixture(t);
    const { ctx, created } = policyContext(f.config);
    ctx.get = () => service;
    await applyPolicy(ctx, { ...f.config, sessionTitle: 'Requested task title · review' });
    await assert.rejects(created(), /sessionTitle service|could not verify/);
    await assert.rejects(readFile(f.receiptPath), { code: 'ENOENT' });
  }
  const f = await fixture(t);
  const { ctx, created } = policyContext(f.config);
  const title = 'Task title longer than thirty-five characters · plan review';
  let requested;
  const snapshot = { title: 'Native normalized title' };
  ctx.get = () => ({ rename: (_session, value) => { requested = value; return snapshot; }, get: () => snapshot });
  await applyPolicy(ctx, { ...f.config, sessionTitle: title });
  await created();
  assert.equal(requested, title, 'the native byte limit owns normalization');
  assert.equal((await readPolicyReceipt(f.receiptPath, f.identity)).sessionId, f.identity.sessionId);
});
