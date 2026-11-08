// Public machine API consumer. Go remains the owner of scheduling and acceptance.
import { createRequire } from 'node:module';
import { isAbsolute } from 'node:path';

export const ADAPTER = Object.freeze({ package: 'dsh-subagent-codex-pro', version: '0.9.0', protocol: 1 });
export const LIMITS = Object.freeze({ requestBytes: 1024 * 1024, machineOutputBytes: 1024 * 1024,
  progressBytes: 1024 * 1024, receiptFrameBytes: 8 * 1024 * 1024, admissionMs: 10000 });
const require = createRequire(import.meta.url);

export class WorkerError extends Error {
  constructor(code, message) { super(message); this.name = 'WorkerError'; this.code = code; }
}

/** The doctor executes this same import, but never calls runMachine. */
export async function loadAdapter() {
  const [major, minor] = process.versions.node.split('.').map(Number);
  if (major < 22 || (major === 22 && minor < 18)) throw new WorkerError('UNSUPPORTED_RUNTIME', 'Codex Pro requires Node.js 22.18.0 or newer');
  if (!['darwin', 'linux'].includes(process.platform)) throw new WorkerError('UNSUPPORTED_RUNTIME', 'Codex Pro requires a supported POSIX platform');
  let manifest, api;
  try {
    manifest = require('dsh-subagent-codex-pro/package.json');
  } catch {
    throw new WorkerError('ADAPTER_UNAVAILABLE', 'Install the exact dsh-subagent-codex-pro@0.9.0 dependency with Flywheel');
  }
  if (manifest.name !== ADAPTER.package || manifest.version !== ADAPTER.version) {
    throw new WorkerError('ADAPTER_MISMATCH', 'Codex Pro package identity or version does not match the pinned contract');
  }
  try { api = await import('dsh-subagent-codex-pro/machine'); }
  catch { throw new WorkerError('ADAPTER_UNAVAILABLE', 'The pinned Codex Pro public machine API could not be imported'); }
  if (api.MACHINE_PROTOCOL_VERSION !== ADAPTER.protocol ||
      typeof api.parseMachineRequest !== 'function' || typeof api.runMachine !== 'function' || typeof api.MachineInputError !== 'function' ||
      api.MACHINE_LIFECYCLE_VERSION !== 1 || typeof api.recoverMachine !== 'function' ||
      api.MACHINE_LIFECYCLE_CAPABILITIES?.durableJournal !== true || api.MACHINE_LIFECYCLE_CAPABILITIES?.readOnlyRecovery !== true ||
      api.MACHINE_LIFECYCLE_CAPABILITIES?.threadReadyBeforeDispatch !== true) {
    throw new WorkerError('ADAPTER_MISMATCH', 'Codex Pro package identity, version or public machine API does not match the pinned contract');
  }
  return api;
}

/** Reject unsupported fields and permissions before any process or prompt starts. */
export function machineRequest(value, api, env) {
  const lifecycle = value?.protocol === 3;
  const persistent = value?.protocol === 2 || (lifecycle && value.threadMode === 'persistent');
  const keys = new Set(['type', 'protocol', 'requestId', 'cwd', 'model', 'reasoningEffort', 'permissionMode', 'prompt', 'timeoutMs',
    ...(persistent || lifecycle ? ['threadMode', 'threadId'] : []), ...(lifecycle ? ['recoveryDirectory'] : [])]);
  if (value === null || typeof value !== 'object' || Array.isArray(value) || value.type !== 'start' || ![1, 2, 3].includes(value.protocol) ||
      Object.keys(value).some(key => !keys.has(key)) || typeof value.requestId !== 'string' || value.requestId.length > 256 ||
      (lifecycle && (!['ephemeral', 'persistent'].includes(value.threadMode) ||
        typeof value.recoveryDirectory !== 'string' || !isAbsolute(value.recoveryDirectory) ||
        value.recoveryDirectory.length > 4096 || value.recoveryDirectory.includes('\0') || (!persistent && value.threadId !== undefined))) ||
      (persistent && (value.threadMode !== 'persistent' || (value.threadId !== undefined &&
        (typeof value.threadId !== 'string' || !value.threadId.trim() || value.threadId.includes('\0'))))) ||
      !['read-only', 'workspace-write', 'danger-full-access'].includes(value.permissionMode)) {
    throw new WorkerError('INVALID_REQUEST', 'Expected one versioned start request with explicit supported permissions and history; protocol 3 requires an absolute recovery directory');
  }
  if (value.timeoutMs !== undefined && (!Number.isSafeInteger(value.timeoutMs) || value.timeoutMs < 1 || value.timeoutMs > 2147483647)) {
    throw new WorkerError('INVALID_REQUEST', 'timeoutMs must be a positive safe integer no greater than 2147483647');
  }
  try {
    return api.parseMachineRequest({ protocol: 1, requestId: value.requestId, cwd: value.cwd, model: value.model,
      effort: value.reasoningEffort,
      permissionMode: value.permissionMode === 'danger-full-access' ? 'dangerously-bypass-approvals-and-sandbox' : value.permissionMode,
      prompt: value.prompt,
      ...(value.timeoutMs === undefined ? {} : { timeoutMs: value.timeoutMs }),
      command: env.FLYWHEEL_CODEX_BIN || 'codex', threadMode: persistent ? 'persistent' : 'ephemeral',
      ...(persistent && value.threadId !== undefined ? { threadId: value.threadId } : {}), maxOutputBytes: LIMITS.machineOutputBytes });
  } catch (error) {
    throw new WorkerError('INVALID_REQUEST', error instanceof api.MachineInputError ? error.message : 'Invalid Codex Pro machine request');
  }
}

export function doctorResult() {
  return { protocol: 1, type: 'doctor', package: ADAPTER.package, version: ADAPTER.version,
    machineProtocol: ADAPTER.protocol, available: true, inferenceTested: false };
}
