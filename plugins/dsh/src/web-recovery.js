import { constants } from 'node:fs';
import { createHash, randomUUID } from 'node:crypto';
import { lstat, mkdir, open, realpath, rename, unlink } from 'node:fs/promises';
import { basename, dirname, isAbsolute, join } from 'node:path';
import { BridgeError, nonempty, record, SESSION_PROTOCOL } from './protocol.js';
import { nativeDigest } from './native.js';
import { validateWebBinding, validateWebFacts } from './web-policy.js';

export const WEB_RECOVERY_VERSION = 'chatgpt-web/read-only-recovery/1';
const limit = 32 * 1024 * 1024;
const rawHash = value => createHash('sha256').update(value).digest('hex');

async function privateDirectory(path) {
  if (!isAbsolute(path) || await realpath(path) !== path) throw new BridgeError('INVALID_REQUEST', 'Web recovery directory must be physical');
  const info = await lstat(path);
  if (!info.isDirectory() || info.isSymbolicLink() || (info.mode & 0o077) !== 0 || process.getuid && info.uid !== process.getuid()) throw new BridgeError('INVALID_REQUEST', 'Web recovery directory must be private and owned');
}
async function syncDirectory(path) {
  const handle = await open(path, 'r');
  try { await handle.sync(); } finally { await handle.close(); }
}
async function save(path, value) {
  const text = JSON.stringify(value);
  if (Buffer.byteLength(text) > limit) throw new BridgeError('TRANSPORT', 'Web recovery evidence is oversized');
  const pending = path + '.' + randomUUID() + '.pending';
  const handle = await open(pending, constants.O_CREAT | constants.O_EXCL | constants.O_WRONLY | constants.O_NOFOLLOW, 0o600);
  try { await handle.writeFile(text); await handle.sync(); } finally { await handle.close(); }
  try { await rename(pending, path); await syncDirectory(dirname(path)); }
  finally { await unlink(pending).catch(error => { if (error.code !== 'ENOENT') throw error; }); }
}
async function read(path) {
  const handle = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  try {
    const info = await handle.stat();
    if (!info.isFile() || info.size > limit || (info.mode & 0o077) !== 0 || process.getuid && info.uid !== process.getuid()) throw new BridgeError('TRANSPORT', 'Web recovery evidence must be a bounded private regular file');
    const text = await handle.readFile('utf8');
    if (Buffer.byteLength(text) !== info.size) throw new BridgeError('TRANSPORT', 'Web recovery evidence changed while reading');
    return { text, value: JSON.parse(text) };
  } finally { await handle.close(); }
}
function environment(env) {
  return { home: env.DSH_HOME ?? '', userHome: env.HOME ?? env.USERPROFILE ?? '', profile: env.FLYWHEEL_DSH_PROFILE || 'sdk', binary: env.FLYWHEEL_DSH_BIN || 'dsh' };
}
function original(request) {
  return { requestId: request.requestId, cwd: request.cwd, workspaceCwd: request.workspaceCwd ?? '', provider: request.provider, model: request.model,
    reasoningEffort: request.reasoningEffort, permissionMode: request.permissionMode, persistent: request.persistent === true,
    requestedSessionId: request.sessionId ?? '', executionDomainId: request.web.executionDomainId, nativeDigest: nativeDigest(request.native ?? null) };
}
function validateDirectory(path, requestId) {
  if (typeof path !== 'string' || !isAbsolute(path) || path.includes('\0') || basename(dirname(path)) !== '.sessions' || basename(path) !== 'recovery-' + rawHash(requestId)) {
    throw new BridgeError('INVALID_REQUEST', 'Web recovery directory does not belong to the original invocation');
  }
}

/** The task owns the directory name; only this worker may create its new leaf. */
export async function beginWebRecovery(request, checkpoint, ready, env) {
  const root = request.web?.recoveryDirectory;
  if (!root) return null;
  validateDirectory(root, request.requestId);
  await privateDirectory(dirname(root));
  if (!record(checkpoint) || checkpoint.version !== 1 || checkpoint.contract !== WEB_RECOVERY_VERSION ||
      checkpoint.identity?.hostInvocationId !== request.requestId || checkpoint.identity.executionDomainId !== request.web.executionDomainId || checkpoint.identity.sessionId !== ready.sessionId) {
    throw new BridgeError('INVALID_REQUEST', 'Plugin recovery checkpoint does not match the admitted invocation');
  }
  await mkdir(root, { mode: 0o700 });
  await privateDirectory(root);
  await syncDirectory(dirname(root));
  const journal = { version: 1, contract: WEB_RECOVERY_VERSION, original: original(request), environment: environment(env),
    producerPid: process.pid, checkpoint, checkpointDigest: nativeDigest(checkpoint), ready, phase: 'admitted' };
  const path = join(root, 'worker.json');
  await save(path, journal);
  return {
    async intent() { journal.phase = 'dispatch-intent'; await save(path, journal); },
    async accepted(promptId) { journal.phase = 'prompt-accepted'; journal.promptId = promptId; await save(path, journal); },
    async finish(business, result) { journal.phase = 'settled'; journal.business = business; journal.result = result; await save(path, journal); },
  };
}

function exited(pid) {
  if (!Number.isSafeInteger(pid) || pid < 1 || pid === process.pid) return false;
  try { process.kill(pid, 0); return false; } catch (error) { return error.code === 'ESRCH'; }
}
const blocked = message => { throw new BridgeError('TRANSPORT', 'Web read-only recovery remains blocked: ' + message); };

/**
 * Runs in a separate, short-lived query process. Importing the plugin's narrow
 * helper cannot launch DSH, Chrome, a tool, or a model. Browser observation and
 * explicit host adoption remain separate user actions in the ChatGPT plugin.
 */
export async function recoverWebWorker(request, { env = process.env, inspect } = {}) {
  if (!record(request) || Object.keys(request).some(key => !['type','protocol','requestId','recoveryDirectory','original','expectedBinding'].includes(key)) ||
      request.type !== 'web-recover' || request.protocol !== 1 || !record(request.original)) throw new BridgeError('INVALID_REQUEST', 'Expected one original Web recovery request');
  nonempty(request.requestId, 'requestId', 128);
  validateDirectory(request.recoveryDirectory, request.requestId);
  await privateDirectory(dirname(request.recoveryDirectory));
  await privateDirectory(request.recoveryDirectory);
  const path = join(request.recoveryDirectory, 'worker.json');
  const snapshot = await read(path), journal = snapshot.value;
  const requestedOriginal = { requestId: request.requestId, ...request.original, nativeDigest: nativeDigest(request.original.native ?? null) };
  delete requestedOriginal.native;
  if (!record(journal) || journal.version !== 1 || journal.contract !== WEB_RECOVERY_VERSION || journal.original?.requestId !== request.requestId ||
      nativeDigest(journal.original) !== nativeDigest(requestedOriginal) ||
      nativeDigest(journal.environment) !== nativeDigest(environment(env)) || journal.checkpointDigest !== nativeDigest(journal.checkpoint) ||
      !['admitted','dispatch-intent','prompt-accepted','settled'].includes(journal.phase)) blocked('original identity, SDK profile, or checkpoint changed');
  if (!exited(journal.producerPid)) blocked('original worker exit is unconfirmed');
  const bindingRequest = { ...request.original, web: { expectedBinding: request.expectedBinding } };
  const binding = validateWebBinding(journal.ready?.web, bindingRequest);
  const inspector = inspect ?? (await import('./web-recovery-reader.js')).readWebInvocationRecovery;
  if (typeof inspector !== 'function') blocked('bundled ChatGPT reader has no public read-only recovery contract');
  const evidence = await inspector(journal.checkpoint);
  if (!record(evidence) || evidence.version !== 1 || evidence.contract !== WEB_RECOVERY_VERSION || evidence.checkpointDigest !== journal.checkpointDigest ||
      !/^[a-f0-9]{64}$/.test(evidence.revision ?? '') || !['not-dispatched','committed','retired','blocked'].includes(evidence.disposition) ||
      nativeDigest(evidence.identity) !== nativeDigest(journal.checkpoint.identity)) blocked('plugin evidence does not match the original checkpoint');
  validateWebBinding(evidence.binding, bindingRequest, binding);
  const facts = validateWebFacts(evidence.facts, journal.checkpoint.identity);
  if (evidence.disposition === 'blocked' || facts.outcome === 'unknown' || facts.remoteGeneration !== 'idle' || !facts.localCleanupConfirmed) blocked('remote response, local tool, or cleanup is unresolved');
  // No new identity/profile observation can settle a native child tree. A
  // recorded, already-confirmed normal terminal may be recovered verbatim.
  if (journal.original.nativeDigest !== nativeDigest(null) && (!journal.result?.settled || journal.result?.lifecycle?.outcome === 'unknown')) blocked('native child completion lacks its original terminal proof');
  const remoteUnsent = evidence.disposition === 'not-dispatched' && facts.dispatch === 'not-dispatched';
  const business = journal.business;
  // Browser submission and SDK prompt admission are different facts. Once the
  // SDK accepted a prompt, its persisted history is no longer the untouched
  // checkpoint that Go may reuse after a positively unsent SDK invocation.
  const sdkUnsent = remoteUnsent && !journal.promptId && !business?.promptId &&
    (journal.phase === 'admitted' || journal.phase === 'settled' && journal.result?.lifecycle?.dispatch === 'not-dispatched');
  if ((!sdkUnsent || journal.phase === 'settled') &&
      (!business?.sdkCleanupConfirmed || business.protocolIntact !== true || journal.phase !== 'settled')) blocked('original SDK business or shutdown proof is missing');
  if (remoteUnsent && !sdkUnsent && (!business.terminalKnown || !['failed','cancelled'].includes(business.outcome) ||
      !business.promptId || business.promptId !== journal.promptId || !business.turnId)) blocked('browser no-dispatch evidence does not prove the original SDK terminal');
  let outcome = 'failed', status = 'error', text = '', turnId = sdkUnsent ? '' : business.turnId ?? '';
  if (evidence.disposition === 'retired' && !turnId) blocked('retired response lacks its original SDK turn identity');
  if (!remoteUnsent && evidence.disposition === 'committed' && facts.outcome === 'completed' && business.completed === true) {
    if (!business.promptId || business.promptId !== journal.promptId || !business.turnId || !Array.isArray(evidence.terminalMain) ||
        !evidence.terminalMain.some(item => typeof item.text === 'string' && item.text === business.text && typeof item.hostCommitId === 'string' && item.hostCommitId)) blocked('committed model output is not the original complete SDK result');
    outcome = 'completed'; status = 'ok'; text = business.text; turnId = business.turnId;
  } else if (!remoteUnsent && evidence.disposition !== 'retired') {
    // A recovered tool response or compaction is not the rest of an Agent run.
    if (!business.terminalKnown || !['failed','cancelled'].includes(business.outcome)) blocked('model response adoption did not complete the original SDK invocation');
    if (facts.outcome !== 'failed') blocked('a later adopted final is not the original SDK terminal');
    outcome = 'failed'; status = 'error'; turnId = business.turnId;
  }
  if ((await read(path)).text !== snapshot.text) blocked('worker evidence changed during the query');
  const result = { ...(journal.result ?? {}), protocol: 2, requestId: request.requestId, type: 'result', status, settled: true,
    sessionId: journal.ready.sessionId, text, usage: null, costUsd: null,
    web: { version: 1, binding, facts },
    lifecycle: { protocol: SESSION_PROTOCOL, requestId: request.requestId, sessionId: journal.ready.sessionId,
      dispatch: sdkUnsent ? 'not-dispatched' : 'dispatched', outcome, cleanupConfirmed: true, resumeCandidate: outcome === 'completed', resumeAccepted: journal.ready.resumed === true,
      turnId, usageStatus: 'unknown' },
  };
  if (status === 'ok') delete result.error;
  else result.error = { code: remoteUnsent ? 'TRANSPORT' : 'INVALID_REQUEST', message: sdkUnsent ? 'Original SDK invocation is confirmed not dispatched' : remoteUnsent ? 'Original SDK invocation failed before Web submission' : 'Original Web invocation settled without a successful SDK result' };
  return { protocol: 1, type: 'web-recovery', requestId: request.requestId, revision: evidence.revision, frames: [journal.ready, result] };
}
