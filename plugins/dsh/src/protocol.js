import { constants } from 'node:fs';
import { open, realpath, stat } from 'node:fs/promises';
import { dirname, isAbsolute } from 'node:path';
import { validateNative, validateManaged } from './native.js';

export const PROTOCOL = 1;
export const SESSION_PROTOCOL = 'dsh-sdk/session-resume/worker-v2';
export const MAX_FRAME_BYTES = 8 * 1024 * 1024;
export const ERROR_CODES = new Set([
  'AUTH', 'QUOTA', 'RATE_LIMIT', 'SERVER', 'TRANSPORT', 'TIMEOUT',
  'EMPTY_RESPONSE', 'INVALID_REQUEST', 'CONTEXT_WINDOW_EXCEEDED',
  'WEB_UNCERTAIN', 'WEB_DELIVERY_CLAIMED', 'WEB_CONNECTION_LOST', 'WEB_PROTOCOL_ERROR',
]);

export const requiresWebReconciliation = code => ['WEB_UNCERTAIN', 'WEB_DELIVERY_CLAIMED', 'WEB_CONNECTION_LOST', 'WEB_PROTOCOL_ERROR'].includes(code);
export const isChatGPTWeb = value => value?.provider === 'chatgpt' && value?.model === 'gpt-6-pro-web';

export class BridgeError extends Error {
  constructor(code, message) {
    super(message);
    this.code = code;
  }
}

export function record(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

// The engine validates contract/report contents. The host validates the typed
// envelope before dispatch so an invalid command cannot become an unknown run.
export function validInvestigationCommand(value) {
  if (!record(value) || !['start', 'run', 'status', 'report', 'cancel'].includes(value.action) ||
      Object.keys(value).some(key => !['action', 'revision', 'contract', 'report'].includes(key))) return false;
  if ('revision' in value && (!Number.isSafeInteger(value.revision) || value.revision < 0)) return false;
  if (!['start', 'status'].includes(value.action) && !(value.revision > 0)) return false;
  if ((value.action === 'start') !== ('contract' in value) ||
      (value.action === 'report') !== ('report' in value)) return false;
  return (!('contract' in value) || record(value.contract)) && (!('report' in value) || record(value.report));
}

export function nonempty(value, label, max = 512) {
  if (typeof value !== 'string' || value.length > max || value.trim() === '' || value.includes('\0')) {
    throw new BridgeError('INVALID_REQUEST', `${label} must be a non-empty string (max ${max})`);
  }
  return value;
}

export async function directory(value, label) {
  nonempty(value, label, 4096);
  if (!isAbsolute(value)) throw new BridgeError('INVALID_REQUEST', `${label} must be absolute`);
  const resolved = await realpath(value);
  if (!(await stat(resolved)).isDirectory()) {
    throw new BridgeError('INVALID_REQUEST', `${label} must be a directory`);
  }
  return resolved;
}

export async function validateStart(value) {
  if (!record(value) || value.type !== 'start' || ![PROTOCOL, 2].includes(value.protocol)) {
    throw new BridgeError('INVALID_REQUEST', 'expected one protocol 1 or 2 start request');
  }
  const allowed = new Set(['type', 'protocol', 'requestId', 'cwd', 'provider', 'model',
    'reasoningEffort', 'permissionMode', 'prompt', 'maxTokens', 'timeoutMs', 'idleTimeoutMs', 'workspaceCwd', 'sessionTitle', 'native', 'managed']);
  if (value.protocol === 2) {
    allowed.add('sessionId');
    allowed.add('persistent');
    if (value.persistent !== true) throw new BridgeError('INVALID_REQUEST', 'protocol 2 requires explicit persistent history');
    if (value.sessionId !== undefined) nonempty(value.sessionId, 'sessionId', 512);
  }
  for (const key of Object.keys(value)) {
    if (!allowed.has(key)) throw new BridgeError('INVALID_REQUEST', `unsupported start field: ${key}`);
  }
  const requestId = nonempty(value.requestId, 'requestId', 128);
  validateNative(value.native);
  validateManaged(value.managed, requestId);
  if (value.managed !== undefined && value.native === undefined) throw new BridgeError('INVALID_REQUEST', 'managed identity requires native options');
  if (value.native?.lcu?.enabled && (!value.managed || value.permissionMode !== 'danger-full-access')) {
    throw new BridgeError('INVALID_REQUEST', 'LCU requires managed identity and explicitly selected full access');
  }
  const provider = nonempty(value.provider, 'provider');
  const model = nonempty(value.model, 'model');
  const prompt = nonempty(value.prompt, 'prompt', MAX_FRAME_BYTES);
  if (!['read-only', 'workspace-write', 'danger-full-access'].includes(value.permissionMode)) {
    throw new BridgeError('INVALID_REQUEST', 'permissionMode must be read-only, workspace-write or danger-full-access');
  }
  if (value.reasoningEffort !== undefined) nonempty(value.reasoningEffort, 'reasoningEffort', 64);
  if (isChatGPTWeb(value) && value.reasoningEffort !== 'pro') {
    throw new BridgeError('INVALID_REQUEST', 'ChatGPT Web requires reasoningEffort pro');
  }
  if (value.idleTimeoutMs !== undefined && (!Number.isSafeInteger(value.idleTimeoutMs) || value.idleTimeoutMs < 1 || value.idleTimeoutMs > 86400000)) {
    throw new BridgeError('INVALID_REQUEST', 'idleTimeoutMs must be between 1 and 86400000');
  }
  if (value.maxTokens !== undefined && (!Number.isSafeInteger(value.maxTokens) || value.maxTokens < 1)) {
    throw new BridgeError('INVALID_REQUEST', 'maxTokens must be a positive integer');
  }
  if (isChatGPTWeb(value) && value.maxTokens !== undefined) {
    throw new BridgeError('INVALID_REQUEST', 'ChatGPT Web does not support maxTokens; use an explicit supported auxiliary model');
  }
  const timeoutMs = value.timeoutMs ?? 30 * 60 * 1000;
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs < 1 || timeoutMs > 24 * 60 * 60 * 1000) {
    throw new BridgeError('INVALID_REQUEST', 'timeoutMs must be between 1 and 86400000');
  }
  if (value.sessionTitle !== undefined) nonempty(value.sessionTitle, 'sessionTitle', 512);
  const request = { ...value, requestId, provider, model, prompt, timeoutMs, cwd: await directory(value.cwd, 'cwd'),
    ...(value.workspaceCwd === undefined ? {} : { workspaceCwd: await directory(value.workspaceCwd, 'workspaceCwd') }) };
  if (request.workspaceCwd && request.workspaceCwd !== request.cwd && request.permissionMode === 'workspace-write') {
    throw new BridgeError('INVALID_REQUEST', 'DSH workspace-write binds its sandbox to the session project; separate execution roots require an explicitly compatible permission mode');
  }
  return request;
}

export async function readPolicyReceipt(path, identity) {
  if (!isAbsolute(path) || await realpath(dirname(path)) !== dirname(path)) {
    throw new BridgeError('INVALID_REQUEST', 'policy receipt directory is not physical');
  }
  const parent = await stat(dirname(path));
  if (!parent.isDirectory() || (parent.mode & 0o077) !== 0) {
    throw new BridgeError('INVALID_REQUEST', 'policy receipt directory is not private');
  }
  // A malicious/incorrect SDK must not stall the worker by substituting a FIFO.
  const file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  try {
    const info = await file.stat();
    if (!info.isFile() || info.size > 65536 || (info.mode & 0o077) !== 0) {
      throw new BridgeError('INVALID_REQUEST', 'policy receipt is not a bounded private regular file');
    }
    const buffer = Buffer.alloc(65537);
    const { bytesRead } = await file.read(buffer, 0, buffer.length, 0);
    if (bytesRead !== info.size) throw new BridgeError('INVALID_REQUEST', 'policy receipt changed while being read');
    const receipt = JSON.parse(buffer.subarray(0, bytesRead).toString('utf8'));
    if (!record(receipt) || Object.entries(identity).some(([key, value]) => receipt[key] !== value)) {
      throw new BridgeError('INVALID_REQUEST', 'policy receipt identity mismatch; prompt withheld');
    }
    return receipt;
  } finally {
    await file.close();
  }
}

export function normalizedError(error) {
  return {
    code: ERROR_CODES.has(error?.code) ? error.code : 'TRANSPORT',
    message: error instanceof Error ? error.message : String(error),
    ...(/^turn-[a-f0-9]{64}$/.test(error?.requestId ?? '') ? { requestId: error.requestId } : {}),
  };
}

export function modelErrorCode(code) {
  if (ERROR_CODES.has(code)) return code;
  if (code === 'MISSING_CREDENTIAL' || code === 'INVALID_CREDENTIAL' ||
      code === 'ACCOUNT_SIGN_IN_REQUIRED' || code === 'ACCOUNT_TOKEN_INVALID') return 'AUTH';
  if (code === 'ACCOUNT_QUOTA') return 'QUOTA';
  if (code === 'NO_ADAPTER' || code === 'UNSUPPORTED_REASONING_EFFORT') return 'INVALID_REQUEST';
  if (['WEB_SETUP_REQUIRED', 'WEB_BINDING_MISMATCH', 'WEB_INVALID_REQUEST', 'WEB_UNSUPPORTED_CONTROL'].includes(code)) return 'INVALID_REQUEST';
  return 'SERVER';
}

const ACCOUNT_RECOVERY = new Map([
  ['ACCOUNT_SIGN_IN_REQUIRED', 'Sign in to DeepSeek in DSH, then ensure the SDK runtime can access that account and the configured endpoint permits account authentication before retrying.'],
  ['ACCOUNT_TOKEN_INVALID', 'The DeepSeek account login was rejected. Sign in again in DSH and ensure the SDK runtime can access the renewed login before retrying.'],
  ['ACCOUNT_QUOTA', 'Check the DeepSeek account balance and usage; retry when account quota is available.'],
]);

function modelErrorMessage(detail, fallback) {
  const message = detail?.message ?? fallback;
  const recovery = ACCOUNT_RECOVERY.get(detail?.code);
  return recovery === undefined ? message : `${detail.code}: ${message}\n${recovery}`;
}

/** Fold committed per-call records; streamed usage snapshots are never summed. */
export class SessionFold {
  constructor(sessionId, { strict = false } = {}) {
    this.sessionId = sessionId;
    this.strict = strict;
    this.seen = new Set();
    this.policy = null;
    this.beginCall();
  }

  // Resume may append model selection/policy facts, but those are not a paid
  // invocation. Never inherit previous assistant output, terminal or usage.
  beginCall() {
    this.receipts = new Set();
    this.text = '';
    this.assistantCount = 0;
    this.usageComplete = true;
    this.usage = { input_tokens: 0, output_tokens: 0, reasoning_tokens: 0,
      cache_read_tokens: 0, cache_write_tokens: 0, total_tokens: 0 };
    this.terminal = null;
    this.turnId = '';
  }

  accept(params) {
    if (!record(params) || params.sessionId !== this.sessionId) return false;
    const event = params.event;
    if (!record(event) || typeof event.type !== 'string' ||
        !Number.isSafeInteger(event.seq) || event.seq < 0 || !record(event.data)) {
      throw new BridgeError('TRANSPORT', 'malformed DSH session event');
    }
    if (this.seen.has(event.seq)) return false;
    this.seen.add(event.seq);
    const data = event.data;
    if (event.type === 'agent/inbox/spliced' && Array.isArray(data.inserted)) {
      for (const message of data.inserted) if (typeof message?.id === 'string') this.receipts.add(message.id);
    }
    if (event.type === 'assistant/message') {
      if (!Array.isArray(data.message?.content) || !data.message.content.every(block =>
        record(block) && typeof block.type === 'string' &&
        (block.type !== 'text' || typeof block.text === 'string'))) {
        throw new BridgeError('TRANSPORT', 'malformed committed assistant message');
      }
      this.assistantCount += 1;
      this.text = data.message.content.filter(block => block.type === 'text').map(block => block.text).join('');
      this.addUsage(data.usage);
    }
    if (event.type === 'turn/end') {
      if (!record(data.reason) || typeof data.reason.kind !== 'string' ||
          (this.strict && (!Number.isSafeInteger(data.turn) || data.turn < 0 ||
            !['completed', 'aborted', 'blocked', 'error', 'max-tokens'].includes(data.reason.kind)))) {
        throw new BridgeError('TRANSPORT', 'malformed DSH turn terminal');
      }
      this.terminal = data.reason;
      this.turnId = Number.isSafeInteger(data.turn) ? String(data.turn) : '';
    }
    return true;
  }

  addUsage(usage) {
    const valid = value => Number.isSafeInteger(value) && value >= 0;
    const fields = ['inputTokens', 'outputTokens', 'cacheReadTokens', 'cacheWriteTokens', 'reasoningTokens', 'totalTokens'];
    if (!record(usage) || !valid(usage.inputTokens) || !valid(usage.outputTokens) ||
        fields.some(key => usage[key] !== undefined && !valid(usage[key]))) {
      this.usageComplete = false;
      return;
    }
    const cacheRead = usage.cacheReadTokens ?? 0;
    const cacheWrite = usage.cacheWriteTokens ?? 0;
    const input = usage.inputTokens + cacheRead + cacheWrite;
    const values = {
      input_tokens: input, output_tokens: usage.outputTokens,
      reasoning_tokens: usage.reasoningTokens ?? 0,
      cache_read_tokens: cacheRead, cache_write_tokens: cacheWrite,
      total_tokens: usage.totalTokens ?? input + usage.outputTokens,
    };
    for (const [key, value] of Object.entries(values)) {
      if (!Number.isSafeInteger(this.usage[key] + value)) this.usageComplete = false;
      else this.usage[key] += value;
    }
  }

  verifiedPolicy(request) {
    const policy = this.policy;
    const fullAccess = request.permissionMode === 'danger-full-access';
    if (!policy || policy.protocol !== PROTOCOL || policy.permissionMode !== request.permissionMode ||
        policy.cwd !== (request.workspaceCwd ?? request.cwd) || policy.approvalPolicy !== 'never' ||
        policy.confinedShell !== !fullAccess ||
        (fullAccess ? policy.enforcement !== undefined : policy.enforcement !== 'full')) {
      throw new BridgeError('INVALID_REQUEST', 'DSH did not attest the requested cwd and permission policy; prompt withheld');
    }
  }

  finish(messageId) {
    if (!this.receipts.has(messageId)) throw new BridgeError('TRANSPORT', 'missing durable prompt receipt');
    if (!this.terminal) throw new BridgeError('TRANSPORT', 'idle without committed turn/end');
    if (this.terminal.kind !== 'completed') {
      // DSH's account-tasks hook cancels the active account turn after sign-out,
      // including when rejecting an expired token. Do not infer this from text
      // or treat other cancellation hooks as authentication failures.
      if (this.terminal.kind === 'aborted' && this.terminal.reason?.kind === 'hook' &&
          this.terminal.reason.reason === 'deepseek-account/signed-out') {
        throw new BridgeError('AUTH', 'DeepSeek account signed out (deepseek-account/signed-out). Sign in again in DSH and ensure the SDK runtime can access that account before retrying.');
      }
      const detail = this.terminal.error;
      const error = new BridgeError(modelErrorCode(detail?.code),
        modelErrorMessage(detail, `DSH turn ended as ${this.terminal.kind}`));
      if (/^WEB_/.test(detail?.code ?? '') && /^turn-[a-f0-9]{64}$/.test(detail?.requestId ?? '')) error.requestId = detail.requestId;
      throw error;
    }
    if (!this.assistantCount || this.text.trim() === '') {
      throw new BridgeError('EMPTY_RESPONSE', 'DSH completed without assistant text');
    }
    return { text: this.text, usage: this.usageComplete ? this.usage : null };
  }
}
