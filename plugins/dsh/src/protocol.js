import { realpath, stat } from 'node:fs/promises';
import { isAbsolute } from 'node:path';

export const PROTOCOL = 1;
export const MAX_FRAME_BYTES = 8 * 1024 * 1024;
export const ERROR_CODES = new Set([
  'AUTH', 'QUOTA', 'RATE_LIMIT', 'SERVER', 'TRANSPORT', 'TIMEOUT',
  'EMPTY_RESPONSE', 'INVALID_REQUEST', 'CONTEXT_WINDOW_EXCEEDED',
]);

export class BridgeError extends Error {
  constructor(code, message) {
    super(message);
    this.code = code;
  }
}

export function record(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
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
  if (!record(value) || value.type !== 'start' || value.protocol !== PROTOCOL) {
    throw new BridgeError('INVALID_REQUEST', 'expected one protocol 1 start request');
  }
  const allowed = new Set(['type', 'protocol', 'requestId', 'cwd', 'provider', 'model',
    'reasoningEffort', 'permissionMode', 'prompt', 'maxTokens', 'timeoutMs']);
  for (const key of Object.keys(value)) {
    if (!allowed.has(key)) throw new BridgeError('INVALID_REQUEST', `unsupported start field: ${key}`);
  }
  const requestId = nonempty(value.requestId, 'requestId', 128);
  const provider = nonempty(value.provider, 'provider');
  const model = nonempty(value.model, 'model');
  const prompt = nonempty(value.prompt, 'prompt', MAX_FRAME_BYTES);
  if (!['read-only', 'workspace-write'].includes(value.permissionMode)) {
    throw new BridgeError('INVALID_REQUEST', 'permissionMode must be read-only or workspace-write');
  }
  if (value.reasoningEffort !== undefined) nonempty(value.reasoningEffort, 'reasoningEffort', 64);
  if (value.maxTokens !== undefined && (!Number.isSafeInteger(value.maxTokens) || value.maxTokens < 1)) {
    throw new BridgeError('INVALID_REQUEST', 'maxTokens must be a positive integer');
  }
  const timeoutMs = value.timeoutMs ?? 30 * 60 * 1000;
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs < 1 || timeoutMs > 24 * 60 * 60 * 1000) {
    throw new BridgeError('INVALID_REQUEST', 'timeoutMs must be between 1 and 86400000');
  }
  return { ...value, requestId, provider, model, prompt, timeoutMs, cwd: await directory(value.cwd, 'cwd') };
}

export function normalizedError(error) {
  return {
    code: ERROR_CODES.has(error?.code) ? error.code : 'TRANSPORT',
    message: error instanceof Error ? error.message : String(error),
  };
}

export function modelErrorCode(code) {
  if (ERROR_CODES.has(code)) return code;
  if (code === 'MISSING_CREDENTIAL' || code === 'INVALID_CREDENTIAL') return 'AUTH';
  if (code === 'ACCOUNT_QUOTA') return 'QUOTA';
  if (code === 'NO_ADAPTER' || code === 'UNSUPPORTED_REASONING_EFFORT') return 'INVALID_REQUEST';
  return 'SERVER';
}

/** Fold committed per-call records; streamed usage snapshots are never summed. */
export class SessionFold {
  constructor(sessionId) {
    this.sessionId = sessionId;
    this.seen = new Set();
    this.receipts = new Set();
    this.policy = null;
    this.text = '';
    this.assistantCount = 0;
    this.usageComplete = true;
    this.usage = { input_tokens: 0, output_tokens: 0, reasoning_tokens: 0,
      cache_read_tokens: 0, cache_write_tokens: 0, total_tokens: 0 };
    this.terminal = null;
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
    if (event.type === 'flywheel/policy') this.policy = data;
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
      if (!record(data.reason) || typeof data.reason.kind !== 'string') {
        throw new BridgeError('TRANSPORT', 'malformed DSH turn terminal');
      }
      this.terminal = data.reason;
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
    if (!policy || policy.protocol !== PROTOCOL || policy.permissionMode !== request.permissionMode ||
        policy.cwd !== request.cwd || policy.approvalPolicy !== 'never' ||
        policy.confinedShell !== true || policy.enforcement !== 'full') {
      throw new BridgeError('INVALID_REQUEST', 'DSH did not attest the requested cwd and confined permission policy; prompt withheld');
    }
  }

  finish(messageId) {
    if (!this.receipts.has(messageId)) throw new BridgeError('TRANSPORT', 'missing durable prompt receipt');
    if (!this.terminal) throw new BridgeError('TRANSPORT', 'idle without committed turn/end');
    if (this.terminal.kind !== 'completed') {
      const detail = this.terminal.error;
      throw new BridgeError(modelErrorCode(detail?.code),
        detail?.message ?? `DSH turn ended as ${this.terminal.kind}`);
    }
    if (!this.assistantCount || this.text.trim() === '') {
      throw new BridgeError('EMPTY_RESPONSE', 'DSH completed without assistant text');
    }
    return { text: this.text, usage: this.usageComplete ? this.usage : null };
  }
}
