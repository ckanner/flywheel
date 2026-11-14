import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { once } from 'node:events';
import { createHash, randomUUID } from 'node:crypto';
import { chmod, mkdir, mkdtemp, readFile, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { acquireGuiLease, settleGuiLease, verifyGuiLease } from '../src/native-resources.js';

async function fixture(t) {
  const directory = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-desktop-lease-')));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const env = { FLYWHEEL_RESOURCE_DIR: directory };
  return { directory, env, acquire: (invocationId = randomUUID(), options = {}) => acquireGuiLease({ invocationId, runId: 'run' }, { env, ...options }) };
}

function ownerProcess(t, directory, id) {
  const child = spawn(process.execPath, [fileURLToPath(new URL('./fixtures/native-lease-process.mjs', import.meta.url)), directory, id], { stdio: ['pipe', 'pipe', 'pipe'] });
  const rows = [], waits = [];
  const lines = createInterface({ input: child.stdout });
  lines.on('line', line => {
    const value = JSON.parse(line); rows.push(value);
    for (const waiter of [...waits]) if (waiter.type === value.type) { waits.splice(waits.indexOf(waiter), 1); waiter.resolve(value); }
  });
  let errors = ''; child.stderr.on('data', data => { errors += data; });
  t.after(async () => { lines.close(); if (child.exitCode === null && child.signalCode === null) { child.kill('SIGKILL'); await once(child, 'exit'); } });
  const wait = async type => {
    const present = rows.find(value => value.type === type); if (present) return present;
    let timeout;
    try { return await Promise.race([new Promise(resolve => waits.push({ type, resolve })), new Promise((_, reject) => { timeout = setTimeout(() => reject(new Error(`missing ${type}: ${JSON.stringify(rows)} ${errors}`)), 12000); })]); }
    finally { clearTimeout(timeout); }
  };
  return { child, rows, wait, send: command => child.stdin.write(command + '\n') };
}

test('GUI leases verify the current owner and epoch, and only confirmed cleanup retires a grant', async t => {
  const f = await fixture(t), lease = await f.acquire('first');
  assert.deepEqual(await verifyGuiLease(lease), lease);
  for (const changed of [{ ownerInvocationId: 'second' }, { leaseEpoch: randomUUID() }, { leaseId: randomUUID() }, { resourceId: 'another-desktop' }]) {
    await assert.rejects(() => verifyGuiLease({ ...lease, ...changed }));
  }
  const receipt = await settleGuiLease(lease, true, { cleanup: 'confirmed' });
  assert.equal(receipt.state, 'released');
  await assert.rejects(() => verifyGuiLease(lease), /ENOENT/);
  const next = await f.acquire('second');
  assert.notEqual(next.leaseEpoch, lease.leaseEpoch); assert.notEqual(next.leaseId, lease.leaseId);
  await assert.rejects(() => settleGuiLease(lease, true), /another invocation/);
  await settleGuiLease(next, true);
});

test('unconfirmed cleanup leaves a bounded integrity-linked blocker even with a large evidence ledger', async t => {
  const f = await fixture(t), lease = await f.acquire('unknown-owner');
  const evidence = { screenshots: 'x'.repeat(200000) };
  const receipt = await settleGuiLease(lease, false, evidence);
  assert.equal(receipt.state, 'cleanup-unknown');
  assert.equal(receipt.evidence.sha256, createHash('sha256').update(JSON.stringify(evidence)).digest('hex'));
  assert.equal(receipt.evidence.bytes, Buffer.byteLength(JSON.stringify(evidence)));
  assert.ok((await readFile(lease.receiptPath)).length < 16384);
  await assert.rejects(() => f.acquire('later'), /cleanup is unconfirmed/);
  await assert.rejects(() => verifyGuiLease(lease), /expired/);
  assert.equal(JSON.parse(await readFile(lease.receiptPath, 'utf8')).leaseEpoch, lease.leaseEpoch);
});

test('independent Node workers serialize on one physical desktop and never overlap leases', async t => {
  const f = await fixture(t);
  const first = ownerProcess(t, f.directory, 'worker-1');
  const a = (await first.wait('acquired')).lease;
  const second = ownerProcess(t, f.directory, 'worker-2');
  assert.equal((await second.wait('waiting')).owner.leaseId, a.leaseId);
  assert.equal(second.rows.some(row => row.type === 'acquired'), false);
  assert.equal((await verifyGuiLease(a)).ownerInvocationId, 'worker-1');
  first.send('release'); await first.wait('settled');
  const b = (await second.wait('acquired')).lease;
  assert.notEqual(b.leaseEpoch, a.leaseEpoch);
  assert.equal((await verifyGuiLease(b)).ownerInvocationId, 'worker-2');
  second.send('release'); await second.wait('settled');
});

test('an actual dead owner is not stolen or treated as successful physical cleanup', async t => {
  const f = await fixture(t);
  const first = ownerProcess(t, f.directory, 'crashed-worker');
  const lease = (await first.wait('acquired')).lease;
  first.child.kill('SIGKILL'); await once(first.child, 'exit');
  await assert.rejects(() => f.acquire('replacement'), /cleanup is unconfirmed/);
  assert.equal(JSON.parse(await readFile(lease.receiptPath, 'utf8')).leaseId, lease.leaseId);
});

test('a live owner is preserved when a waiting invocation is cancelled', async t => {
  const f = await fixture(t), lease = await f.acquire('live');
  const abort = new AbortController();
  const waiting = f.acquire('cancelled-waiter', { signal: abort.signal, onWait: () => abort.abort(new Error('fixture cancellation')) });
  await assert.rejects(() => waiting, /abort|cancellation/i);
  assert.deepEqual(await verifyGuiLease(lease), lease);
  await settleGuiLease(lease, true);
});

test('private ownership, physical directories and bounded receipt identities are enforced', async t => {
  const f = await fixture(t);
  await chmod(f.directory, 0o755); await assert.rejects(() => f.acquire(), /private physical/); await chmod(f.directory, 0o700);
  const outside = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-desktop-outside-')));
  t.after(() => rm(outside, { recursive: true, force: true }));
  await symlink(outside, join(f.directory, 'local-desktop'));
  await assert.rejects(() => f.acquire(), /physical directory/);
  await rm(join(f.directory, 'local-desktop'));
  const lease = await f.acquire('receipt-owner');
  await chmod(lease.receiptPath, 0o644); await assert.rejects(() => verifyGuiLease(lease), /private file/); await chmod(lease.receiptPath, 0o600);
  await assert.rejects(() => verifyGuiLease({ ...lease, receiptPath: join(dirname(lease.receiptPath), 'different.json') }), /canonical/);
  await writeFile(lease.receiptPath, JSON.stringify({ ...lease, version: 2 })); await assert.rejects(() => verifyGuiLease(lease), /invalid identity/);
  await writeFile(lease.receiptPath, 'x'.repeat(16385)); await assert.rejects(() => verifyGuiLease(lease), /bounded/);
});

test('an incomplete directory remains blocked and cannot be reclaimed as an expired lock', async t => {
  const f = await fixture(t);
  await mkdir(join(f.directory, 'local-desktop'), { mode: 0o700 });
  await assert.rejects(() => f.acquire('later'), /ownership is incomplete/);
  await assert.rejects(() => acquireGuiLease({ invocationId: 'x', runId: 'run' }, { env: { FLYWHEEL_RESOURCE_DIR: 'relative' } }), /absolute/);
  await assert.rejects(() => acquireGuiLease({ invocationId: '' }, { env: f.env }), /identity/);
});

test('a symlink receipt and a second settlement attempt never remove another owner', async t => {
  const f = await fixture(t), lease = await f.acquire('owner');
  const original = await readFile(lease.receiptPath);
  const alternate = join(f.directory, 'copy'); await writeFile(alternate, original, { mode: 0o600 });
  await rm(lease.receiptPath); await symlink(alternate, lease.receiptPath);
  await assert.rejects(() => verifyGuiLease(lease), /ELOOP/);
  await rm(lease.receiptPath); await writeFile(lease.receiptPath, original, { mode: 0o600 });
  await writeFile(lease.receiptPath + '.terminal', 'incomplete settlement', { mode: 0o600 });
  await assert.rejects(() => settleGuiLease(lease, true), /EEXIST/);
  assert.equal((await verifyGuiLease(lease)).leaseId, lease.leaseId);
  await assert.rejects(() => settleGuiLease(lease, 'yes'), /explicit/);
});
