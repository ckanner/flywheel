// Claude Code is an independent harness, not a DSH native model provider.
import { createRequire } from 'node:module';

export const ADAPTER = Object.freeze({ package: 'dsh-subagent-claude-code-pro', version: '0.3.2', protocol: 1 });
export const LIMITS = Object.freeze({ requestBytes: 1024 * 1024, machineOutputBytes: 1024 * 1024,
  progressBytes: 1024 * 1024, receiptFrameBytes: 8 * 1024 * 1024, admissionMs: 10000 });
const require = createRequire(import.meta.url);

export class WorkerError extends Error {
  constructor(code, message) { super(message); this.name = 'WorkerError'; this.code = code; }
}

/** Optional dependency: importing a different adapter never requires Claude. */
export async function loadAdapter() {
  const [major, minor] = process.versions.node.split('.').map(Number);
  if (major < 22 || (major === 22 && minor < 18) || !['darwin', 'linux'].includes(process.platform)) {
    throw new WorkerError('UNSUPPORTED_RUNTIME', 'Claude Code Pro requires Node.js 22.18.0 or newer on a supported POSIX platform');
  }
  let manifest, api;
  try { manifest = require('dsh-subagent-claude-code-pro/package.json'); }
  catch { throw new WorkerError('ADAPTER_UNAVAILABLE', 'Install dsh-subagent-claude-code-pro@0.3.2 alongside @ckanner/dsh-plugin-flywheel; the optional adapter is not installed'); }
  if (manifest.name !== ADAPTER.package || manifest.version !== ADAPTER.version) {
    throw new WorkerError('ADAPTER_MISMATCH', 'Claude Code Pro package identity or version does not match the pinned 0.3.2 machine contract');
  }
  try { api = await import('dsh-subagent-claude-code-pro/machine'); }
  catch { throw new WorkerError('ADAPTER_UNAVAILABLE', 'The pinned Claude Code Pro public machine API could not be imported'); }
  if (api.MACHINE_PROTOCOL_VERSION !== ADAPTER.protocol || typeof api.parseMachineRequest !== 'function' ||
      typeof api.runMachine !== 'function' || typeof api.MachineInputError !== 'function') {
    throw new WorkerError('ADAPTER_MISMATCH', 'Claude Code Pro does not expose the pinned public machine API');
  }
  return api;
}

/** Native approval policies cannot substitute for a requested OS sandbox. */
export function machineRequest(value, api, env) {
  const persistent = value?.protocol === 2;
  const keys = new Set(['type', 'protocol', 'requestId', 'cwd', 'model', 'reasoningEffort', 'permissionMode', 'prompt', 'timeoutMs',
    ...(persistent ? ['threadMode', 'threadId'] : [])]);
  if (value === null || typeof value !== 'object' || Array.isArray(value) || value.type !== 'start' ||
      ![1, 2].includes(value.protocol) || Object.keys(value).some(key => !keys.has(key)) ||
      typeof value.requestId !== 'string' || !value.requestId.trim() || value.requestId.length > 256 || value.requestId.includes('\0') ||
      typeof value.model !== 'string' || !value.model.trim() ||
      (persistent && (value.threadMode !== 'persistent' || (value.threadId !== undefined &&
        (typeof value.threadId !== 'string' || !value.threadId.trim() || value.threadId.includes('\0'))))) ||
      value.permissionMode !== 'danger-full-access') {
    throw new WorkerError('INVALID_REQUEST', 'Expected a versioned start request with explicit full access; Claude native approval modes do not implement read-only/workspace-write sandboxes');
  }
  if (value.timeoutMs !== undefined && (!Number.isSafeInteger(value.timeoutMs) || value.timeoutMs < 1 || value.timeoutMs > 2147483647)) {
    throw new WorkerError('INVALID_REQUEST', 'timeoutMs must be a positive safe integer no greater than 2147483647');
  }
  try {
    return api.parseMachineRequest({ protocol: 1, requestId: value.requestId, cwd: value.cwd, prompt: value.prompt,
      model: value.model, effort: value.reasoningEffort, permissionMode: 'bypassPermissions',
      sessionMode: persistent ? 'persistent' : 'ephemeral',
      ...(persistent && value.threadId !== undefined ? { sessionId: value.threadId } : {}),
      ...(value.timeoutMs === undefined ? {} : { timeoutMs: value.timeoutMs }),
      command: env.FLYWHEEL_CLAUDE_BIN || 'claude', maxOutputBytes: LIMITS.machineOutputBytes });
  } catch (error) {
    throw new WorkerError('INVALID_REQUEST', error instanceof api.MachineInputError ? error.message : 'Invalid Claude Code Pro machine request');
  }
}

// Go has already filtered its child's environment: credentials only enter via
// the explicitly selected channel, never from the orchestrator's ambient env.
export function runtimeOptions(env) {
  const keys = ['ANTHROPIC_API_KEY', 'ANTHROPIC_AUTH_TOKEN', 'ANTHROPIC_BASE_URL', 'CLAUDE_CODE_OAUTH_TOKEN'];
  return { credentialEnv: Object.fromEntries(keys.filter(key => typeof env[key] === 'string' && env[key] !== '')
    .map(key => [key, env[key]])) };
}

export function doctorResult() {
  return { protocol: 1, type: 'doctor', package: ADAPTER.package, version: ADAPTER.version,
    machineProtocol: ADAPTER.protocol, available: true, inferenceTested: false };
}
