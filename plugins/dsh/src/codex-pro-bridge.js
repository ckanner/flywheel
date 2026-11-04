// Public machine API consumer. Go remains the owner of scheduling and acceptance.
import { createRequire } from 'node:module';

export const ADAPTER = Object.freeze({ package: 'dsh-subagent-codex-pro', version: '0.8.0', protocol: 1 });
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
    throw new WorkerError('ADAPTER_UNAVAILABLE', 'Install the exact dsh-subagent-codex-pro@0.8.0 dependency with Flywheel');
  }
  if (manifest.name !== ADAPTER.package || manifest.version !== ADAPTER.version) {
    throw new WorkerError('ADAPTER_MISMATCH', 'Codex Pro package identity or version does not match the pinned contract');
  }
  try { api = await import('dsh-subagent-codex-pro/machine'); }
  catch { throw new WorkerError('ADAPTER_UNAVAILABLE', 'The pinned Codex Pro public machine API could not be imported'); }
  if (api.MACHINE_PROTOCOL_VERSION !== ADAPTER.protocol ||
      typeof api.parseMachineRequest !== 'function' || typeof api.runMachine !== 'function' || typeof api.MachineInputError !== 'function') {
    throw new WorkerError('ADAPTER_MISMATCH', 'Codex Pro package identity, version or public machine API does not match the pinned contract');
  }
  return api;
}

/** Reject unsupported fields and permissions before any process or prompt starts. */
export function machineRequest(value, api, env) {
  const keys = new Set(['type', 'protocol', 'requestId', 'cwd', 'model', 'reasoningEffort', 'permissionMode', 'prompt', 'timeoutMs']);
  if (value === null || typeof value !== 'object' || Array.isArray(value) || value.type !== 'start' || value.protocol !== 1 ||
      Object.keys(value).some(key => !keys.has(key)) || typeof value.requestId !== 'string' || value.requestId.length > 256 ||
      !['read-only', 'workspace-write'].includes(value.permissionMode)) {
    throw new WorkerError('INVALID_REQUEST', 'Expected one protocol=1 start request with an explicit supported permission mode');
  }
  if (value.timeoutMs !== undefined && (!Number.isSafeInteger(value.timeoutMs) || value.timeoutMs < 1 || value.timeoutMs > 2147483647)) {
    throw new WorkerError('INVALID_REQUEST', 'timeoutMs must be a positive safe integer no greater than 2147483647');
  }
  try {
    return api.parseMachineRequest({ protocol: 1, requestId: value.requestId, cwd: value.cwd, model: value.model,
      effort: value.reasoningEffort, permissionMode: value.permissionMode, prompt: value.prompt,
      ...(value.timeoutMs === undefined ? {} : { timeoutMs: value.timeoutMs }),
      command: env.FLYWHEEL_CODEX_BIN || 'codex', threadMode: 'ephemeral', maxOutputBytes: LIMITS.machineOutputBytes });
  } catch (error) {
    throw new WorkerError('INVALID_REQUEST', error instanceof api.MachineInputError ? error.message : 'Invalid Codex Pro machine request');
  }
}

export function doctorResult() {
  return { protocol: 1, type: 'doctor', package: ADAPTER.package, version: ADAPTER.version,
    machineProtocol: ADAPTER.protocol, available: true, inferenceTested: false };
}
