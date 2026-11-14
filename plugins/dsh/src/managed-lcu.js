// Host adapter for the public LCU attachment API. The scheduler owns the physical
// desktop lease; LCU owns each real Agent's runtime and awaited lifecycle hooks.
import { createHash } from 'node:crypto';
import { constants } from 'node:fs';
import { lstat, mkdir, open, realpath } from 'node:fs/promises';
import { basename, dirname, isAbsolute, join, resolve } from 'node:path';
import { attachManagedLcu, ManagedLcuError } from 'dsh-plugin-lcu/managed';

const MAX_IMAGE_BYTES = 64 * 1024 * 1024;
const MIME_EXTENSIONS = new Map([['image/png', 'png'], ['image/jpeg', 'jpg'], ['image/webp', 'webp'], ['image/gif', 'gif']]);
const identityKeys = ['runId', 'invocationId', 'rootSessionId', 'candidateId'];
const leaseKeys = ['leaseId', 'resourceId', 'leaseEpoch', 'ownerInvocationId'];
const hash = data => createHash('sha256').update(data).digest('hex');
const errorText = error => (error instanceof Error ? error.message : String(error)).slice(0, 2048);

function requireIdentity(value, label) {
  if (typeof value !== 'string' || !value.trim() || value.length > 4096 || value.includes('\0')) throw new Error(`Invalid managed LCU ${label}`);
}

async function physicalDirectory(path, create = false) {
  if (create) await mkdir(path, { mode: 0o700 }).catch(error => { if (error.code !== 'EEXIST') throw error; });
  const stat = await lstat(path);
  if (!stat.isDirectory() || (stat.mode & 0o022) !== 0 ||
      (process.getuid && stat.uid !== process.getuid()) || await realpath(path) !== path) {
    throw new Error('LCU evidence requires an owned physical directory without shared write permission');
  }
  return path;
}

async function evidenceDirectory(taskDir, invocationId) {
  let path = await physicalDirectory(taskDir);
  for (const component of ['reports', 'native', invocationId]) path = await physicalDirectory(join(path, component), true);
  return path;
}

async function syncDirectory(path) {
  const directory = await open(path, constants.O_RDONLY | constants.O_DIRECTORY);
  try { await directory.sync(); } finally { await directory.close(); }
}

async function readImage(path, expected) {
  if (!isAbsolute(path)) throw new Error('Attachment store returned a relative image path');
  // The source comes from the attachment service. Pin its physical parent and
  // refuse a final symlink/non-file before reading any bytes.
  const physical = join(await realpath(dirname(path)), basename(path));
  const file = await open(physical, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  try {
    const stat = await file.stat();
    if (!stat.isFile() || stat.size < 1 || stat.size > MAX_IMAGE_BYTES || stat.size !== expected.bytes) {
      throw new Error('Attachment bytes are not a bounded regular image of the observed size');
    }
    const data = await file.readFile();
    if (data.length !== expected.bytes || hash(data) !== expected.sha256) throw new Error('Attachment hash differs from the captured image');
    return data;
  } finally { await file.close(); }
}

async function writeVerified(path, data) {
  let file;
  try { file = await open(path, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o600); }
  catch (error) {
    if (error.code !== 'EEXIST') throw error;
    // A repeated content hash is reusable only after reading the real existing
    // file. A symlink, old unrelated file or partial write must never be reused.
    await readImage(path, { bytes: data.length, sha256: hash(data) });
    return;
  }
  try { await file.writeFile(data); await file.sync(); } finally { await file.close(); }
  await readImage(path, { bytes: data.length, sha256: hash(data) });
  await syncDirectory(dirname(path));
}

async function attachMediaEvidence(ctx, taskDir, event) {
  const images = Array.isArray(event.facts.images) ? event.facts.images : [];
  const evidence = {
    version: 1, identity: event.identity, lease: event.lease,
    agentSessionId: event.agentSessionId, runtimeEpoch: event.runtimeEpoch,
    turnId: event.turnId ?? null, callId: event.callId ?? null,
    sourceSequence: event.sequence, capturedAt: event.facts.observedAt ?? null,
    method: event.facts.method ?? null, target: event.facts.target ?? null,
    modelRoute: event.facts.modelRoute ?? {},
    screenshotCaptured: images.length > 0,
    imageDeliveredToModel: 'unverified', evidenceAttachedToRun: false,
    status: 'unavailable', images: [],
  };
  let directory;
  try { directory = await evidenceDirectory(taskDir, event.identity.invocationId); }
  catch (error) { return { ...evidence, error: errorText(error) }; }
  const attachments = ctx.get('attachments');
  for (const image of images) {
    const entry = { capturedSha256: image.sha256, capturedMimeType: image.mimeType, capturedBytes: image.bytes, status: 'unavailable' };
    try {
      if (!/^[a-f0-9]{64}$/.test(image.sha256) || !Number.isSafeInteger(image.bytes) || image.bytes < 1 ||
          image.bytes > MAX_IMAGE_BYTES || !MIME_EXTENSIONS.has(image.mimeType)) throw new Error('Image facts are not verifiable');
      if (typeof attachments?.readImage !== 'function' || !image.attachment) throw new Error('No verifiable attachment-store reference was prepared');
      // DSH may normalize/rescale a capture before publishing its reference.
      // The public reader verifies that normalized object, whose hash must be
      // recorded separately from the raw runtime capture's hash.
      const stored = await attachments.readImage(image.attachment);
      const data = stored?.data instanceof Uint8Array ? Buffer.from(stored.data) : undefined;
      const ref = stored?.ref;
      if (!data || data.length < 1 || data.length > MAX_IMAGE_BYTES || ref?.attachmentId !== image.attachment.attachmentId ||
          data.length !== ref.bytes || ref.bytes !== image.attachment.bytes || !MIME_EXTENSIONS.has(ref.mediaType)) {
        throw new Error('Attachment reader did not verify the referenced normalized image');
      }
      entry.sha256 = hash(data); entry.mimeType = ref.mediaType; entry.bytes = data.length;
      entry.attachment = ref;
      entry.normalized = entry.sha256 !== image.sha256 || entry.mimeType !== image.mimeType || entry.bytes !== image.bytes;
      const path = join(directory, `${entry.sha256}.${MIME_EXTENSIONS.get(entry.mimeType)}`);
      await physicalDirectory(directory);
      await writeVerified(path, data);
      entry.path = path;
      entry.status = 'attached';
    } catch (error) { entry.error = errorText(error); }
    evidence.images.push(entry);
  }
  const attached = evidence.images.filter(image => image.status === 'attached').length;
  evidence.status = attached === images.length && images.length > 0 ? 'attached' : attached > 0 ? 'partial' : 'unavailable';
  evidence.evidenceAttachedToRun = evidence.status === 'attached';
  const manifestName = hash(JSON.stringify([event.agentSessionId, event.runtimeEpoch, event.turnId, event.callId, event.sequence]));
  const manifest = join(directory, `${manifestName}.json`);
  try {
    await physicalDirectory(directory);
    await writeVerified(manifest, Buffer.from(JSON.stringify(evidence) + '\n'));
    return { ...evidence, manifest };
  } catch (error) {
    // The bytes may exist, but no durable candidate-bound manifest was verified.
    return { ...evidence, status: 'unavailable', evidenceAttachedToRun: false, error: errorText(error) };
  }
}

/**
 * Mount the actual public LCU API after native-policy has admitted the Agent.
 * identityOf is consulted again for every call and approval, so a removed or
 * unrelated live Agent cannot keep using a previously copied grant.
 */
export async function createManagedLcu(ctx, { options = {}, managed, rootSessionId, identityOf, verifyLease, onEvent }) {
  if (managed?.version !== 1) throw new Error('Managed LCU requires invocation identity version 1');
  const identity = Object.freeze({ runId: managed.runId, invocationId: managed.invocationId, rootSessionId,
    ...(managed.candidateId === undefined ? {} : { candidateId: managed.candidateId }) });
  for (const [key, value] of Object.entries(identity)) requireIdentity(value, key);
  if (!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$/.test(managed.invocationId)) throw new Error('Managed LCU invocation ID must be a safe evidence directory name');
  if (typeof identityOf !== 'function' || typeof verifyLease !== 'function' || typeof onEvent !== 'function') throw new Error('Managed LCU requires lineage, lease and event callbacks');
  const lease = Object.freeze(Object.fromEntries(leaseKeys.map(key => [key, managed.lease?.[key]])));
  for (const [key, value] of Object.entries(lease)) requireIdentity(value, `lease.${key}`);
  if (lease.ownerInvocationId !== identity.invocationId) throw new Error('Managed LCU lease belongs to another invocation');
  if (typeof managed.taskDir !== 'string' || !isAbsolute(managed.taskDir) || resolve(managed.taskDir) !== managed.taskDir) throw new Error('Managed LCU requires a physical absolute task directory');
  const taskDir = await physicalDirectory(managed.taskDir);
  const host = Object.freeze({ role: managed.role, scope: managed.scope, channel: managed.channel });
  const config = structuredClone(options);
  const handles = new Map();
  const pending = new Map();
  const failedReceipts = [];
  let accepting = true;
  let closing;
  let eventTail = Promise.resolve();

  const verifyIdentity = agent => {
    const actual = identityOf(agent);
    if (!actual || identityKeys.some(key => actual[key] !== identity[key])) throw new Error('LCU Agent identity differs from the admitted invocation');
    return identity;
  };
  const emit = event => {
    const operation = eventTail.catch(() => {}).then(async () => {
      let envelope = { ...event, host };
      if (event.type === 'media') {
        const evidence = await attachMediaEvidence(ctx, taskDir, event);
        envelope = { ...envelope, facts: { ...event.facts, evidence,
          evidenceAttachedToRun: evidence.evidenceAttachedToRun,
          imageDeliveredToModel: 'unverified' } };
      }
      await onEvent(envelope);
    });
    eventTail = operation;
    return operation;
  };
  const snapshot = () => [...failedReceipts, ...[...handles.values()].map(handle => handle.snapshot())];
  return {
    snapshot,
    async attach(agent) {
      if (!accepting) throw new Error('Managed LCU invocation is closing');
      verifyIdentity(agent);
      if (handles.has(agent)) return handles.get(agent).snapshot();
      if (pending.has(agent)) return pending.get(agent);
      const operation = (async () => {
        try {
          const handle = await attachManagedLcu(ctx, { agent, identity, lease, config,
            verifyLease: async admission => { verifyIdentity(agent); await verifyLease(admission); }, onEvent: emit });
          handles.set(agent, handle);
          return handle.snapshot();
        } catch (error) {
          if (error instanceof ManagedLcuError && error.receipt) failedReceipts.push(error.receipt);
          throw error;
        }
      })();
      pending.set(agent, operation);
      try { return await operation; } finally { pending.delete(agent); }
    },
    async close(reason = 'Interrupt') {
      if (closing) return closing;
      accepting = false;
      const event = ['Stop', 'Interrupt', 'SubagentStop'].includes(reason) ? reason : 'Interrupt';
      closing = (async () => {
        await Promise.allSettled([...pending.values()]);
        const outcomes = await Promise.allSettled([...handles.values()].map(handle => handle.close(event)));
        // A public close normally returns a receipt. Unexpected rejection still
        // keeps its real snapshot; append unknown instead of hiding the owner.
        const owned = [...handles.values()];
        outcomes.forEach((outcome, index) => {
          if (outcome.status === 'rejected') {
            const receipt = owned[index].snapshot();
            failedReceipts.push({ ...receipt, cleanup: 'unknown', state: 'blocked', errors: [...receipt.errors, errorText(outcome.reason)] });
          }
        });
        await eventTail.catch(() => {}); // LCU snapshots already retain failed event receipts.
        return snapshot();
      })();
      const receipts = await closing;
      if (receipts.some(receipt => receipt.cleanup !== 'confirmed')) closing = undefined;
      return receipts;
    },
  };
}
