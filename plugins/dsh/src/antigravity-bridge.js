import { createRequire } from 'node:module';
import { isAbsolute } from 'node:path';
import { validateObservationRequest } from './machine-worker.js';

export const ADAPTER = Object.freeze({ package: 'dsh-subagent-antigravity', version: '0.1.0', protocol: 1 });
export const LIMITS = Object.freeze({ requestBytes: 1024 * 1024, machineOutputBytes: 1024 * 1024,
  progressBytes: 1024 * 1024, receiptFrameBytes: 8 * 1024 * 1024, admissionMs: 10000 });
const require = createRequire(import.meta.url);
export class WorkerError extends Error {
  constructor(code, message) { super(message); this.name = 'WorkerError'; this.code = code; }
}
export async function loadAdapter() {
  let manifest, api;
  try { manifest = require('dsh-subagent-antigravity/package.json'); api = await import('dsh-subagent-antigravity/machine'); }
  catch { throw new WorkerError('ADAPTER_UNAVAILABLE', 'Install dsh-subagent-antigravity@0.1.0 alongside Flywheel'); }
  if (manifest.name !== ADAPTER.package || manifest.version !== ADAPTER.version || api.MACHINE_PROTOCOL_VERSION !== 1 ||
      api.MACHINE_LIFECYCLE_VERSION !== 1 || api.MACHINE_ACTIVITY_VERSION !== 1 ||
      api.MACHINE_LIFECYCLE_CAPABILITIES?.durableJournal !== true || api.MACHINE_LIFECYCLE_CAPABILITIES?.readOnlyRecovery !== true ||
      api.MACHINE_LIFECYCLE_CAPABILITIES?.threadReadyBeforeDispatch !== false ||
      api.MACHINE_CAPABILITIES?.evidenceOnly !== 'verified-empty-init-tools' ||
      !['parseMachineRequest', 'runMachine', 'recoverMachine', 'doctor', 'MachineInputError'].every(key => typeof api[key] === 'function')) {
    throw new WorkerError('ADAPTER_MISMATCH', 'Antigravity does not expose the pinned machine, lifecycle and evidence-only contracts');
  }
  return api;
}
export function machineRequest(value, api, env) {
  const keys = new Set(['type', 'protocol', 'requestId', 'cwd', 'model', 'reasoningEffort', 'permissionMode', 'prompt',
    'timeoutMs', 'idleTimeoutMs', 'sessionTitle', 'threadMode', 'threadId', 'recoveryDirectory', 'subscription', 'evidenceOnly']);
  const persistent = value?.threadMode === 'persistent', lifecycle = value?.protocol === 3;
  const evidence = value?.evidenceOnly === true;
  if (!value || typeof value !== 'object' || Array.isArray(value) || value.type !== 'start' ||
      ![1, 2, 3].includes(value.protocol) || Object.keys(value).some(key => !keys.has(key)) ||
      typeof value.requestId !== 'string' || !value.requestId.trim() || value.requestId.length > 256 || value.requestId.includes('\0') ||
      (value.evidenceOnly !== undefined && typeof value.evidenceOnly !== 'boolean') ||
      !((evidence && value.permissionMode === 'read-only') || (!evidence && value.permissionMode === 'danger-full-access')) ||
      value.subscription?.version !== 1 || value.subscription.mode !== 'subscription-only' || Object.keys(value.subscription).length !== 2 ||
      (value.protocol === 1 && (value.threadMode !== undefined || value.threadId !== undefined || value.recoveryDirectory !== undefined)) ||
      (value.protocol === 2 && (!persistent || value.recoveryDirectory !== undefined)) ||
      (lifecycle && (!['ephemeral', 'persistent'].includes(value.threadMode) || typeof value.recoveryDirectory !== 'string' ||
        !isAbsolute(value.recoveryDirectory) || value.recoveryDirectory.includes('\0'))) ||
      (value.threadId !== undefined && !persistent)) {
    throw new WorkerError('INVALID_REQUEST', 'Antigravity requires a versioned subscription-only request with explicit full access or evidence-only permissions');
  }
  validateObservationRequest(value, WorkerError);
  try {
    return api.parseMachineRequest({ protocol: 1, requestId: value.requestId, cwd: value.cwd, prompt: value.prompt,
      command: env.FLYWHEEL_AGY_BIN || 'agy', model: value.model, effort: value.reasoningEffort,
      executionMode: evidence ? 'plan' : 'accept-edits', permissionMode: evidence ? 'request-review' : 'always-proceed',
      sandbox: false, toolPolicy: evidence ? 'evidence-only' : 'native', subscriptionPolicy: value.subscription,
      sessionMode: value.threadId === undefined ? 'new' : 'resume',
      ...(value.threadId === undefined ? {} : { sessionId: value.threadId }),
      ...(value.sessionTitle === undefined ? {} : { sessionName: value.sessionTitle }),
      ...(value.timeoutMs === undefined ? {} : { timeoutMs: value.timeoutMs }), maxOutputBytes: LIMITS.machineOutputBytes });
  } catch (error) { throw new WorkerError('INVALID_REQUEST', error instanceof api.MachineInputError ? error.message : 'Invalid Antigravity machine request'); }
}
export function doctorResult() {
  return { protocol: 1, type: 'doctor', package: ADAPTER.package, version: ADAPTER.version, machineProtocol: 1,
    available: true, inferenceTested: false, accountEntitlement: 'unknown', quota: 'unknown', evidenceOnly: 'verified-empty-init-tools' };
}
