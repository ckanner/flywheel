// A desktop is a physical resource shared by all local SDK workers. A crashed
// owner leaves a blocking receipt: PID death alone cannot prove GUI cleanup.
import { createHash, randomUUID } from 'node:crypto';
import { constants } from 'node:fs';
import { mkdir, open, realpath, rm, lstat, rename } from 'node:fs/promises';
import { homedir } from 'node:os';
import { join, isAbsolute, dirname, basename, normalize } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { BridgeError } from './protocol.js';

const MAX_RECEIPT_BYTES = 16384;
const valueString = value => typeof value === 'string' && value.length > 0 && value.length <= 4096 && !value.includes('\0');
const uuid = value => typeof value === 'string' && /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/.test(value);
const owned = stat => !process.getuid || stat.uid === process.getuid();

async function privateDirectory(path) {
  const info = await lstat(path);
  if (!info.isDirectory() || (info.mode & 0o077) !== 0 || !owned(info) || await realpath(path) !== path) {
    throw new Error('Native resource directory is not an owned private physical directory');
  }
}

async function syncDirectory(path) {
  const file = await open(path, constants.O_RDONLY | constants.O_DIRECTORY | constants.O_NOFOLLOW);
  try { await file.sync(); } finally { await file.close(); }
}

async function receiptLocation(path) {
  if (!valueString(path) || !isAbsolute(path) || normalize(path) !== path || basename(path) !== 'lease.json' || basename(dirname(path)) !== 'local-desktop') {
    throw new Error('GUI lease receipt does not name the canonical resource location');
  }
  await privateDirectory(dirname(dirname(path)));
  await privateDirectory(dirname(path));
}

async function readLease(path) {
  await receiptLocation(path);
  const file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  try {
    const info = await file.stat();
    if (!info.isFile() || info.size < 1 || info.size > MAX_RECEIPT_BYTES || (info.mode & 0o077) !== 0 || !owned(info)) throw new Error('GUI lease receipt is not a bounded private file');
    const bytes = Buffer.alloc(MAX_RECEIPT_BYTES + 1);
    const { bytesRead } = await file.read(bytes, 0, bytes.length, 0);
    if (bytesRead !== info.size) throw new Error('GUI lease changed during verification');
    const current = JSON.parse(bytes.subarray(0, bytesRead).toString('utf8'));
    if (current.version !== 1 || current.resourceId !== 'local-desktop' || !uuid(current.leaseId) || !uuid(current.leaseEpoch) ||
        !valueString(current.ownerInvocationId) || typeof current.runId !== 'string' || current.runId.length > 4096 ||
        !Number.isSafeInteger(current.pid) || current.pid < 1 || !['held', 'released', 'cleanup-unknown'].includes(current.state) ||
        current.receiptPath !== path || !valueString(current.createdAt) || !Number.isFinite(Date.parse(current.createdAt))) {
      throw new Error('GUI lease receipt has an invalid identity or lifecycle');
    }
    return current;
  } finally { await file.close(); }
}

export async function verifyGuiLease(lease) {
  if (!lease || lease.resourceId !== 'local-desktop' || !uuid(lease.leaseId) || !uuid(lease.leaseEpoch) || !valueString(lease.ownerInvocationId)) {
    throw new Error('Missing managed desktop grant');
  }
  const current = await readLease(lease.receiptPath);
  if (current.state !== 'held' || ['leaseId', 'leaseEpoch', 'ownerInvocationId', 'resourceId'].some(key => current[key] !== lease[key])) {
    throw new Error('Desktop grant expired or belongs to another invocation');
  }
  return current;
}

export async function acquireGuiLease(managed, { env = process.env, signal, onWait = () => {}, alive = pid => {
  try { process.kill(pid, 0); return true; } catch (error) { return error.code !== 'ESRCH'; }
} } = {}) {
  if (!managed || !valueString(managed.invocationId) || typeof managed.runId !== 'string' || managed.runId.length > 4096) {
    throw new BridgeError('INVALID_REQUEST', 'LCU requires a managed invocation identity');
  }
  const root = env.FLYWHEEL_RESOURCE_DIR || join(homedir(), '.flywheel', 'resources');
  if (!isAbsolute(root)) throw new BridgeError('INVALID_REQUEST', 'FLYWHEEL_RESOURCE_DIR must be absolute');
  await mkdir(root, { recursive: true, mode: 0o700 });
  const physical = await realpath(root);
  await privateDirectory(physical);
  const directory = join(physical, 'local-desktop');
  const receiptPath = join(directory, 'lease.json');
  let waitingPublished = false, missingSince;
  for (;;) {
    signal?.throwIfAborted();
    let created = false;
    try { await mkdir(directory, { mode: 0o700 }); created = true; }
    catch (error) { if (error.code !== 'EEXIST') throw error; }
    if (created) {
      // Any failure after mkdir leaves an incomplete, blocking owner. Never
      // erase it speculatively: this is the same fail-closed crash boundary.
      const receipt = { version: 1, leaseId: randomUUID(), leaseEpoch: randomUUID(), resourceId: 'local-desktop',
        ownerInvocationId: managed.invocationId, runId: managed.runId, pid: process.pid, state: 'held', createdAt: new Date().toISOString(), receiptPath };
      const body = JSON.stringify(receipt);
      if (Buffer.byteLength(body) > MAX_RECEIPT_BYTES) throw new Error('Managed desktop receipt exceeded its size limit');
      const file = await open(receiptPath, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o600);
      try { await file.writeFile(body); await file.sync(); } finally { await file.close(); }
      await syncDirectory(directory); await syncDirectory(physical);
      // Ownership has committed. Return it even if cancellation raced the
      // fsync, so the caller can prove no dispatch and settle its own receipt.
      return receipt;
    }
    let current;
    try { current = await readLease(receiptPath); missingSince = undefined; }
    catch (error) {
      if (error.code !== 'ENOENT') throw error;
      // A confirmed owner may have removed the directory between our mkdir
      // attempt and read. Retry acquisition; an existing empty directory is a
      // distinct incomplete-owner state and cannot be stolen after a crash.
      try { await lstat(directory); }
      catch (stateError) { if (stateError.code === 'ENOENT') { missingSince = undefined; continue; } throw stateError; }
      missingSince ??= Date.now();
      if (Date.now() - missingSince > 1000) throw new Error(`Desktop ownership is incomplete; reconcile ${directory} before another GUI run`);
      await delay(50, undefined, { signal });
      continue;
    }
    if (current.state !== 'held' || !alive(current.pid)) {
      throw new Error(`Desktop cleanup is unconfirmed for invocation ${current.ownerInvocationId}; reconcile lease ${current.leaseId} before another GUI run`);
    }
    if (!waitingPublished) { await onWait({ resourceId: 'local-desktop', ownerInvocationId: current.ownerInvocationId, leaseId: current.leaseId }); waitingPublished = true; }
    await delay(100, undefined, { signal });
  }
}

export async function settleGuiLease(lease, confirmed, evidence = null) {
  if (typeof confirmed !== 'boolean') throw new Error('Desktop cleanup confirmation must be explicit');
  const current = await verifyGuiLease(lease);
  // Full receipts live in the invocation ledger. Keep a bounded integrity link
  // here so a large media/child receipt cannot make a blocker unreadable.
  const body = JSON.stringify(evidence ?? null);
  const evidenceRef = { sha256: createHash('sha256').update(body).digest('hex'), bytes: Buffer.byteLength(body) };
  const updated = { ...current, state: confirmed ? 'released' : 'cleanup-unknown', finishedAt: new Date().toISOString(), evidence: evidenceRef };
  const pending = lease.receiptPath + '.terminal';
  const file = await open(pending, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o600);
  try { await file.writeFile(JSON.stringify(updated)); await file.sync(); } finally { await file.close(); }
  await syncDirectory(dirname(lease.receiptPath));
  // Only this invocation may retire the lease. A timeout, PID death or another
  // process never permits stealing it. Unknown cleanup remains a durable block.
  if (confirmed) {
    await rm(dirname(lease.receiptPath), { recursive: true });
    await syncDirectory(dirname(dirname(lease.receiptPath)));
  } else {
    await rename(pending, lease.receiptPath);
    await syncDirectory(dirname(lease.receiptPath));
  }
  return updated;
}
