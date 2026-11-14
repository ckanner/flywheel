// One-use, no-prompt SDK startup. No model Session exists until the owned start.
import { open, realpath, stat, link, unlink, readdir } from 'node:fs/promises';
import { join, isAbsolute } from 'node:path';
import { BridgeError, MAX_FRAME_BYTES, directory, nonempty, record } from './protocol.js';
import { validateNative, nativeDigest } from './native.js';

export const PREWARM_MS = 60000;
const MAX_RECEIPT_BYTES = 8192;
const TRUNCATED_DIAGNOSTIC = ' [truncated]';
const BINDING_FIELDS = ['cwd', 'workspaceCwd', 'provider', 'model', 'reasoningEffort', 'permissionMode'];

export async function validatePrepare(value) {
  const keys = new Set(['type', 'protocol', 'warmId', 'binding', 'receiptDirectory', 'ttlMs', 'native', ...BINDING_FIELDS]);
  if (!record(value) || value.type !== 'prepare' || value.protocol !== 1 ||
      Object.keys(value).some(key => !keys.has(key)) || typeof value.warmId !== 'string' || typeof value.binding !== 'string' ||
      !/^[a-zA-Z0-9_-]{8,128}$/.test(value.warmId) ||
      !/^[a-f0-9]{64}$/.test(value.binding ?? '') || !Number.isSafeInteger(value.ttlMs) || value.ttlMs < 1 || value.ttlMs > PREWARM_MS ||
      !['read-only', 'workspace-write', 'danger-full-access'].includes(value.permissionMode)) {
    throw new BridgeError('INVALID_REQUEST', 'expected a bounded, identified SDK prepare request without a prompt');
  }
  const request = { ...value, cwd: await directory(value.cwd, 'cwd'),
    ...(value.workspaceCwd === undefined ? {} : { workspaceCwd: await directory(value.workspaceCwd, 'workspaceCwd') }) };
  validateNative(value.native);
  for (const key of ['provider', 'model', 'reasoningEffort']) nonempty(value[key], key);
  if (request.workspaceCwd && request.workspaceCwd !== request.cwd && request.permissionMode === 'workspace-write') {
    throw new BridgeError('INVALID_REQUEST', 'separate execution roots require an explicitly compatible permission mode');
  }
  const path = value.receiptDirectory;
  if (typeof path !== 'string' || !isAbsolute(path) || await realpath(path) !== path) {
    throw new BridgeError('INVALID_REQUEST', 'prewarm receipt directory must be physical and absolute');
  }
  const owner = await stat(path);
  if (!owner.isDirectory() || (owner.mode & 0o077) !== 0) throw new BridgeError('INVALID_REQUEST', 'prewarm receipt directory must be private');
  if ((await readdir(path)).length) throw new BridgeError('INVALID_REQUEST', 'prewarm receipt directory must be new and empty');
  return request;
}

export function matchWarmStart(prepared, request) {
  if (request.maxTokens !== undefined) throw new BridgeError('INVALID_REQUEST', 'maxTokens cannot be introduced after SDK initialization');
  if (BINDING_FIELDS.some(key => (prepared[key] ?? '') !== (request[key] ?? ''))) {
    throw new BridgeError('INVALID_REQUEST', 'formal request differs from the initialized SDK binding');
  }
  if (nativeDigest(prepared.native ?? null) !== nativeDigest(request.native ?? null)) throw new BridgeError('INVALID_REQUEST', 'native composition changed after prewarm initialization');
}

export async function writeWarmReceipt(prepared, settled, error) {
  // A parent may recover this receipt only after proving this worker has exited.
  // Exclusive publication prevents overwriting evidence from another lifetime.
  const pending = join(prepared.receiptDirectory, 'terminal.pending');
  const diagnostic = typeof error?.message === 'string' ? error.message : undefined;
  const receipt = { protocol: 1, warmId: prepared.warmId, binding: prepared.binding,
    workerPid: process.pid, dispatched: false, cleanupConfirmed: settled,
    ...(diagnostic === undefined ? {} : { error: '' }) };
  let bytes = Buffer.byteLength(JSON.stringify(receipt) + '\n');
  // Only diagnostic text is expendable. Never publish oversized or shortened
  // proof fields; a validated prepare leaves ample room for the marker.
  if (bytes > MAX_RECEIPT_BYTES || (diagnostic !== undefined && bytes + TRUNCATED_DIAGNOSTIC.length > MAX_RECEIPT_BYTES)) {
    throw new BridgeError('INVALID_REQUEST', 'prewarm cleanup identity exceeds the private receipt limit');
  }
  if (diagnostic !== undefined) {
    const parts = [], sizes = [];
    // Read only the prefix that can fit. Per-code-point JSON size accounts for
    // UTF-8, escapes and lone surrogates without splitting a surrogate pair.
    for (const character of diagnostic) {
      const size = Buffer.byteLength(JSON.stringify(character)) - 2;
      if (bytes + size > MAX_RECEIPT_BYTES) {
        while (bytes + TRUNCATED_DIAGNOSTIC.length > MAX_RECEIPT_BYTES) {
          bytes -= sizes.pop(); parts.pop();
        }
        parts.push(TRUNCATED_DIAGNOSTIC);
        break;
      }
      parts.push(character); sizes.push(size); bytes += size;
    }
    receipt.error = parts.join('');
  }
  const file = await open(pending, 'wx', 0o600);
  try { await file.writeFile(JSON.stringify(receipt) + '\n'); await file.sync(); }
  finally { await file.close(); }
  await link(pending, join(prepared.receiptDirectory, 'terminal.json'));
  await unlink(pending);
  const directory = await open(prepared.receiptDirectory, 'r');
  try { await directory.sync(); } finally { await directory.close(); }
}

/** Both phases keep stdin as the parent lifeline; EOF is never a start signal. */
export function prewarmInput(controller, input = process.stdin) {
  let buffer = Buffer.alloc(0), first, phase = 0, ready = false, finished = false;
  let admit, rejectAdmission, next, rejectNext;
  const admission = new Promise((resolve, reject) => { admit = resolve; rejectAdmission = reject; });
  const dispatch = new Promise((resolve, reject) => { next = resolve; rejectNext = reject; });
  dispatch.catch(() => {}); // Initialization may fail before a dispatch is awaited.
  const fail = error => {
    if (finished) return;
    clearTimeout(timer);
    rejectAdmission(error); rejectNext(error); controller.abort(error);
  };
  const timer = setTimeout(() => fail(new BridgeError('TIMEOUT', 'no complete prepare request within 10 seconds')), 10000);
  const onData = chunk => {
    if (finished || controller.signal.aborted) return;
    buffer = Buffer.concat([buffer, chunk]);
    while (buffer.length) {
      const newline = buffer.indexOf(10);
      if ((newline < 0 ? buffer.length : newline + 1) > MAX_FRAME_BYTES) {
        fail(new BridgeError('INVALID_REQUEST', 'prewarm frame exceeds 8 MiB')); return;
      }
      if (newline < 0) return;
      const bytes = buffer.subarray(0, newline); buffer = buffer.subarray(newline + 1);
      try {
        const value = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes));
        if (phase === 0) { first = value; phase = 1; clearTimeout(timer); admit(value); }
        else {
          const close = value?.type === 'close';
          if (phase !== 1 || !record(value) || value.protocol !== 1 || value.warmId !== first?.warmId ||
              Object.keys(value).some(key => !['type', 'protocol', 'warmId', ...(close ? [] : ['request'])].includes(key)) ||
              (!close && (value.type !== 'start' || !ready || !record(value.request)))) {
            throw new BridgeError('INVALID_REQUEST', 'expected one matching ready start or close frame');
          }
          phase = 2;
          next(close ? null : value.request);
          if (close && !ready) controller.abort(new BridgeError('TRANSPORT', 'prewarm closed before readiness'));
        }
      } catch (error) {
        fail(error instanceof BridgeError ? error : new BridgeError('INVALID_REQUEST', 'prewarm frames must be complete UTF-8 JSON'));
        return;
      }
    }
  };
  const onEnd = () => fail(new BridgeError('TRANSPORT', 'prewarm parent lifeline closed'));
  const onError = () => fail(new BridgeError('TRANSPORT', 'prewarm parent lifeline failed'));
  const onAbort = () => { clearTimeout(timer); rejectAdmission(controller.signal.reason); rejectNext(controller.signal.reason); };
  controller.signal.addEventListener('abort', onAbort, { once: true });
  if (controller.signal.aborted) onAbort();
  input.on('data', onData); input.once('end', onEnd); input.once('error', onError);
  return { admission, dispatch, ready() { ready = true; }, stop() {
    finished = true; clearTimeout(timer); controller.signal.removeEventListener('abort', onAbort);
    input.off('data', onData); input.off('end', onEnd); input.off('error', onError); input.pause();
  } };
}
