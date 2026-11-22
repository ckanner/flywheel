import { createRequire } from 'node:module';
import { isAbsolute } from 'node:path';
import { validateNativeAdmission, validateObservationRequest } from './machine-worker.js';

export const ADAPTER = Object.freeze({ package: '@ckanner/dsh-subagent-antigravity', version: '0.2.0', protocol: 1 });
export const LIMITS = Object.freeze({ requestBytes: 1024 * 1024, machineOutputBytes: 1024 * 1024,
  progressBytes: 1024 * 1024, receiptFrameBytes: 8 * 1024 * 1024, admissionMs: 10000 });
const require = createRequire(import.meta.url);
export class WorkerError extends Error {
  constructor(code, message) { super(message); this.name = 'WorkerError'; this.code = code; }
}
export async function loadAdapter() {
  let manifest, api;
  try { manifest = require('@ckanner/dsh-subagent-antigravity/package.json'); api = await import('@ckanner/dsh-subagent-antigravity/machine'); }
  catch { throw new WorkerError('ADAPTER_UNAVAILABLE', 'Install @ckanner/dsh-subagent-antigravity@0.2.0 alongside Flywheel'); }
  if (manifest.name !== ADAPTER.package || manifest.version !== ADAPTER.version || api.MACHINE_PROTOCOL_VERSION !== 1 ||
      api.MACHINE_LIFECYCLE_VERSION !== 1 || api.MACHINE_ACTIVITY_VERSION !== 1 ||
      api.MACHINE_LIFECYCLE_CAPABILITIES?.durableJournal !== true || api.MACHINE_LIFECYCLE_CAPABILITIES?.readOnlyRecovery !== true ||
      api.MACHINE_LIFECYCLE_CAPABILITIES?.threadReadyBeforeDispatch !== false ||
      api.MACHINE_CAPABILITIES?.evidenceOnly !== 'verified-managed-agent' ||
      api.MACHINE_CAPABILITIES?.nativeOrchestrator !== true ||
      !['native', 'subagents', 'swarm'].every(mode => api.MACHINE_CAPABILITIES?.nativeModes?.includes(mode)) ||
      api.MACHINE_CAPABILITIES?.nativeAgentLimit !== 'observed-total-children-guard' ||
      api.MACHINE_CAPABILITIES?.hardNativeAgentLimit !== false || api.MACHINE_CAPABILITIES?.nativeCompletionScope !== 'cli-invocation' ||
      !['parseMachineRequest', 'runMachine', 'recoverMachine', 'doctor', 'MachineInputError'].every(key => typeof api[key] === 'function')) {
    throw new WorkerError('ADAPTER_MISMATCH', 'Antigravity does not expose the pinned machine, lifecycle and evidence-only contracts');
  }
  return api;
}
export function machineRequest(value, api, env) {
  const keys = new Set(['type', 'protocol', 'requestId', 'cwd', 'model', 'reasoningEffort', 'permissionMode', 'prompt',
    'timeoutMs', 'idleTimeoutMs', 'sessionTitle', 'threadMode', 'threadId', 'recoveryDirectory', 'subscription', 'evidenceOnly', 'native', 'managed']);
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
  validateNativeAdmission(value.managed, value.requestId, WorkerError);
  if ((evidence && (value.native !== undefined || value.managed !== undefined)) ||
      (value.native?.mode === 'swarm' && value.managed !== undefined && value.managed.role !== 'execute')) {
    throw new WorkerError('INVALID_REQUEST', 'Evidence-only calls cannot use native children; Flywheel Swarm belongs to approved Execute collaboration');
  }
  try {
    return api.parseMachineRequest({ protocol: 1, requestId: value.requestId, cwd: value.cwd, prompt: value.prompt,
      command: env.FLYWHEEL_AGY_BIN || 'agy', model: value.model, effort: value.reasoningEffort,
      executionMode: evidence ? 'plan' : 'accept-edits', permissionMode: evidence ? 'request-review' : 'always-proceed',
      sandbox: false, toolPolicy: evidence ? 'evidence-only' : 'native', subscriptionPolicy: value.subscription,
      sessionMode: value.threadId === undefined ? 'new' : 'resume',
      ...(value.native === undefined ? {} : { native: value.native }),
      ...(value.threadId === undefined ? {} : { sessionId: value.threadId }),
      ...(value.sessionTitle === undefined ? {} : { sessionName: value.sessionTitle }),
      ...(value.timeoutMs === undefined ? {} : { timeoutMs: value.timeoutMs }), maxOutputBytes: LIMITS.machineOutputBytes });
  } catch (error) { throw new WorkerError('INVALID_REQUEST', error instanceof api.MachineInputError ? error.message : 'Invalid Antigravity machine request'); }
}

export function invokeAdapter(api, request, options) {
  return api.runMachine(request, { ...options, onActivity(event) {
    // Antigravity may attach its current native snapshot to child activity.
    // The shared activity wire carries only bounded liveness metadata; the
    // complete snapshot is validated separately in the terminal receipt.
    // Strip only that documented extension, leaving all other unexpected
    // fields for the shared strict activity validator to reject.
    let activity = event;
    if (event && typeof event === 'object' && !Array.isArray(event)) {
      activity = { ...event };
      delete activity.native;
    }
    options.onActivity?.(activity);
  } });
}

export function doctorResult() {
  return { protocol: 1, type: 'doctor', package: ADAPTER.package, version: ADAPTER.version, machineProtocol: 1,
    available: true, inferenceTested: false, accountEntitlement: 'unknown', quota: 'unknown', evidenceOnly: 'verified-managed-agent',
    nativeVersion: 1, nativeOrchestrator: true, nativeModes: ['native', 'subagents', 'swarm'],
    nativeAgentLimit: 'observed-total-children-guard', hardNativeAgentLimit: false, nativeCompletionScope: 'cli-invocation' };
}
