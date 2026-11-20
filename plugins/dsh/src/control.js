import { spawn } from 'node:child_process';
import { directory, nonempty, record, validInvestigationCommand } from './protocol.js';
import { validPlanningCommand, validPlanningUserAction } from './planning-control.js';
import { validSessionUserAction } from './session-control.js';
import { preparationDirectory } from './task-directory.js';
import { validImageSelection, validImageInspection } from './image-contract.js';

const actions = ['planning', 'investigate', 'launch', 'start', 'status', 'stop', 'config', 'doctor', 'step', 'config-edit', 'config-save', 'runs', 'metrics', 'session', 'history', 'delivery', 'deliver', 'node', 'images'];
const checkStatuses = ['pass', 'fail', 'warn', 'unknown', 'skipped'];

// This marker is issued only where the host can prove that no control process
// started. An engine/transport error after spawn must never acquire it.
export class ControlNotDispatchedError extends Error {
  constructor(cause) {
    super(cause instanceof Error ? cause.message : String(cause), { cause });
    this.name = 'ControlNotDispatchedError';
  }
}

export async function validateControl(input, defaultCwd) {
  if (!record(input) || !actions.includes(input.action)) {
    throw new Error('action must be launch, start, step, status, stop, config, config-edit, config-save, or doctor');
  }
  if (Object.keys(input).some(key => !['action', 'taskDir', 'cwd', 'probe', 'mode', 'only', 'packet', 'revision', 'retryReason', 'editor', 'session', 'sessionKey', 'delivery', 'runId', 'investigation', 'planning', 'images'].includes(key))) {
    throw new Error('unknown Flywheel control argument');
  }
  if (input.action === 'investigate' ? !validInvestigationCommand(input.investigation) : 'investigation' in input) throw new Error('investigate requires a typed, revision-bound preparation command');
  if (input.action === 'planning' ? !validPlanningCommand(input.planning) : 'planning' in input) throw new Error('planning requires a typed, version-bound planning command');
  if (input.action === 'images' ? !validImageSelection(input.images) : 'images' in input) throw new Error('images requires one to four explicit repository-relative image paths');
  if (input.planning && Buffer.byteLength(JSON.stringify(input.planning)) > 131072) throw new Error('planning input exceeds 128 KiB');
  if (input.investigation && Buffer.byteLength(JSON.stringify(input.investigation)) > 131072) throw new Error('investigation input exceeds 128 KiB');
  if ('probe' in input && (input.action !== 'doctor' || typeof input.probe !== 'boolean')) {
    throw new Error('probe must be a boolean and is only supported by doctor');
  }
  if ('mode' in input && (!['launch', 'start'].includes(input.action) || !['auto', 'quick', 'full'].includes(input.mode))) {
    throw new Error('mode must be auto, quick, or full and is only supported by launch/start');
  }
  if (input.action === 'session') {
    if (!record(input.session) || !['claim', 'send', 'return', 'reconcile', 'release', 'abandon'].includes(input.session.action) ||
        typeof input.session.key !== 'string' || !Number.isSafeInteger(input.session.revision) || input.session.revision < 1) {
      throw new Error('session requires a typed revision-bound command');
    }
    if (input.session.action === 'abandon' && (typeof input.session.reason !== 'string' || input.session.reason.trim() === '' ||
        Buffer.byteLength(input.session.reason) > 1000 || typeof input.session.invocation !== 'string' || input.session.invocation.trim() === '' ||
        input.session.invocation.length > 200 || 'prompt' in input.session)) {
      throw new Error('session abandonment requires a bounded reason, the exact invocation, and no prompt');
    }
  } else if ('session' in input) {
    throw new Error('session requires a typed revision-bound command');
  }
  if (input.action === 'history' ? typeof input.sessionKey !== 'string' || !input.sessionKey : 'sessionKey' in input) throw new Error('history requires an exact session key');
  if (input.session && Buffer.byteLength(JSON.stringify(input.session)) > 131072) throw new Error('session command too large');
  if (['delivery','deliver'].includes(input.action) ? !record(input.delivery) || (input.action === 'delivery' ? input.delivery.action !== 'inspect' : !['create','merge'].includes(input.delivery.action) || input.delivery.confirmed !== true) : 'delivery' in input) throw new Error('delivery writes require explicit confirmation');
  if (input.delivery && Buffer.byteLength(JSON.stringify(input.delivery)) > 131072) throw new Error('delivery request too large');
  if ('runId' in input && (input.action !== 'metrics' || typeof input.runId !== 'string' || !input.runId || input.runId.length > 128 || /[\0\r\n]/.test(input.runId))) throw new Error('runId requires metrics and an exact engine ID');
  const editing = ['config-edit', 'config-save'].includes(input.action);
  if (editing ? !record(input.editor) || !['global', 'project'].includes(input.editor.scope) : 'editor' in input) {
    throw new Error('editor requires config-edit/config-save and an explicit global/project scope');
  }
  if (input.action === 'step') {
    if (!['plan', 'execute', 'review', 'verify', 'integrate', 'final'].includes(input.only) || !Number.isSafeInteger(input.revision) || input.revision < 1 ||
        (input.packet !== undefined && (typeof input.packet !== 'string' || !/^M\d+-T\d+$/.test(input.packet)))) {
      throw new Error('step requires a valid stage, state revision and optional packet id');
    }
  } else if (input.action === 'node') {
    if (typeof input.packet !== 'string' || !/^M\d+-T\d+$/.test(input.packet) || 'only' in input || 'revision' in input) throw new Error('node requires an exact packet ID');
  } else if (['only', 'packet', 'revision'].some(key => key in input)) {
    throw new Error('stage arguments require step');
  }
  if ('retryReason' in input && (input.action !== 'step' || input.only !== 'execute' || typeof input.retryReason !== 'string' || !input.retryReason.trim() || Buffer.byteLength(input.retryReason) > 1000)) throw new Error('retryReason requires Execute and a bounded explanation');
  if (editing && Buffer.byteLength(JSON.stringify(input.editor)) > 1024 * 1024) throw new Error('configuration exceeds 1 MiB');
  const needsTask = !['config', 'doctor', 'config-edit', 'config-save', 'runs', 'images'].includes(input.action);
  const createsTask = input.action === 'planning' && input.planning.action === 'prepare' || input.action === 'investigate' && input.investigation.action === 'start';
  return {
    action: input.action,
    ...(input.planning ? { planning: input.planning } : {}),
    ...(input.images ? { images: input.images } : {}),
    ...(input.investigation ? { investigation: input.investigation } : {}),
    ...(input.action === 'node' ? { packet: input.packet } : {}),
    ...(input.runId ? { runId: input.runId } : {}),
    ...(input.delivery ? { delivery: input.delivery } : {}),
    ...(input.session ? { session: input.session } : {}),
    ...(input.sessionKey ? { sessionKey: input.sessionKey } : {}),
    taskDir: needsTask || 'taskDir' in input ? createsTask ? await preparationDirectory(input.taskDir) : await directory(input.taskDir, 'taskDir') : '',
    cwd: await directory(input.cwd ?? defaultCwd, 'cwd'),
    ...(input.action === 'doctor' ? { probe: input.probe ?? false } : {}),
    ...(input.action === 'launch' || 'mode' in input ? { mode: input.mode ?? 'auto' } : {}),
    ...(input.action === 'step' ? { only: input.only, packet: input.packet, revision: input.revision, ...(input.retryReason ? { retryReason: input.retryReason } : {}) } : {}),
    ...(editing ? { editor: input.editor } : {}),
  };
}

function validConfiguration(value) {
  return record(value) && typeof value.source === 'string' && typeof value.scope === 'string' &&
    Array.isArray(value.bindings) && value.bindings.every(binding => record(binding) &&
      ['id', 'role', 'harness'].every(key =>
        typeof binding[key] === 'string' && binding[key].trim() !== '') &&
      ['route', 'profile', 'nativeProfile', 'provider', 'channel', 'model', 'thinking', 'permissionMode'].every(key =>
        binding[key] === undefined || typeof binding[key] === 'string') &&
      ['residentSession', 'inScope'].every(key => binding[key] === undefined || typeof binding[key] === 'boolean'));
}

function validDiagnostic(value, request) {
  return record(value) && value.kind === 'doctor' && typeof value.ok === 'boolean' &&
    typeof value.cwd === 'string' && value.cwd.trim() !== '' && value.taskDir === request.taskDir &&
    value.probeRequested === request.probe && value.inferenceTested === false &&
    (value.configuration === undefined || validConfiguration(value.configuration)) &&
    Array.isArray(value.checks) && value.checks.every(check => record(check) &&
      typeof check.id === 'string' && check.id.trim() !== '' && checkStatuses.includes(check.status) &&
      typeof check.summary === 'string' && check.summary.trim() !== '' &&
      ['code', 'remedy'].every(key => check[key] === undefined || typeof check[key] === 'string'));
}

/** Only the engine executable can mutate engine state. No shell interpolation. */
export async function runControl(input, { binary = 'flywheel', defaultCwd, hostSessionID, planningUserAction, sessionUserAction, signal, timeoutMs = 60000 } = {}) {
  let request;
  try {
    nonempty(binary, 'flywheelBinary', 4096);
    if (binary.startsWith('-')) throw new Error('flywheelBinary must name an executable');
    if (!Number.isSafeInteger(timeoutMs) || timeoutMs < 100 || timeoutMs > 120000) {
      throw new Error('controlTimeoutMs must be between 100 and 120000');
    }
    request = await validateControl(input, defaultCwd);
    if (request.planning?.action === 'approve' && !validPlanningUserAction(planningUserAction, request.planning)) throw new Error('Design approval requires a verified human command');
    if (planningUserAction !== undefined && request.planning?.action !== 'approve') throw new Error('User approval proof is only valid for design approval');
    if (request.session?.action === 'abandon') {
      // First bind authority to the exact user event, including its original
      // path spelling. Only then use the resolved task identity for both argv
      // and the engine proof (macOS aliases, symlinks and trailing slashes).
      if (!validSessionUserAction(sessionUserAction, request.session, input.taskDir)) throw new Error('Unknown-invocation abandonment requires a verified human command');
      sessionUserAction = { ...sessionUserAction, taskDir: request.taskDir };
    }
    if (sessionUserAction !== undefined && request.session?.action !== 'abandon') throw new Error('Session abandonment proof is only valid for abandon');
    if (signal?.aborted) throw new Error('Flywheel control request cancelled');
  } catch (error) {
    throw new ControlNotDispatchedError(error);
  }
  // Diagnostic commands own watchdog child groups. Its TERM grace, pipe
  // WaitDelay, and quiescence confirmation may each take 5s; keep the Go owner
  // alive long enough to finish all three instead of orphaning its workers.
  const killGraceMs = ['config', 'doctor', 'config-edit', 'config-save', 'delivery', 'deliver'].includes(request.action) ? 20000 : 500;
  const args = ['control', request.action, '--cwd', request.cwd, '--compact'];
  if (request.taskDir) args.push('--task-dir', request.taskDir);
  if (request.probe) args.push('--probe');
  if (request.mode) args.push('--mode', request.mode);
  if (request.only) args.push('--only', request.only, '--expect-revision', String(request.revision));
  if (request.packet) args.push('--packet', request.packet);
  if (request.runId) args.push('--run-id', request.runId);
  if (request.sessionKey) args.push('--session-key', request.sessionKey);
  if (request.retryReason) args.push('--retry-reason', request.retryReason);
  const env = { ...process.env };
  delete env.DSH_SESSION_ID;
  delete env.FLYWHEEL_PLANNING_USER_ACTION;
  delete env.FLYWHEEL_SESSION_USER_ACTION;
  if (typeof hostSessionID === 'string' && /^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$/.test(hostSessionID)) env.DSH_SESSION_ID = hostSessionID;
  if (planningUserAction !== undefined) env.FLYWHEEL_PLANNING_USER_ACTION = JSON.stringify(planningUserAction);
  if (sessionUserAction !== undefined) env.FLYWHEEL_SESSION_USER_ACTION = JSON.stringify(sessionUserAction);
  return new Promise((resolve, reject) => {
    let child;
    try {
      child = spawn(binary, args, {
        cwd: request.cwd, env, detached: false, stdio: [request.editor || request.session || request.delivery || request.investigation || request.planning || request.images ? 'pipe' : 'ignore', 'pipe', 'pipe'],
      });
    } catch (error) {
      reject(new ControlNotDispatchedError(error));
      return;
    }
    let spawned = false;
    child.once('spawn', () => { spawned = true; });
    let stdout = '';
    let stderr = '';
    let failure;
    let killTimer;
    const stop = error => {
      if (failure) return;
      failure = error;
      clearTimeout(timer);
      child.kill('SIGTERM');
      killTimer = setTimeout(() => child.kill('SIGKILL'), killGraceMs);
    };
    const cancel = () => stop(new Error('Flywheel control request cancelled'));
    const timer = setTimeout(() => stop(new Error('Flywheel control request timed out')), timeoutMs);
    if (request.editor || request.session || request.delivery || request.investigation || request.planning || request.images) {
      child.stdin.on('error', error => stop(error));
      child.stdin.end(JSON.stringify(request.editor ?? request.session ?? request.delivery ?? request.investigation ?? request.planning ?? request.images));
    }
    signal?.addEventListener('abort', cancel, { once: true });
    // Decode across chunks so a split UTF-8 code point is not replaced in a
    // Chinese diagnostic, model name, or path.
    child.stdout.setEncoding('utf8');
    child.stderr.setEncoding('utf8');
    child.stdout.on('data', chunk => {
      if (failure) return;
      stdout += chunk;
      if (Buffer.byteLength(stdout) > 1024 * 1024) stop(new Error('Flywheel control response exceeds 1 MiB'));
    });
    child.stderr.on('data', chunk => { stderr = (stderr + chunk).slice(-8192); });
    child.on('error', error => {
      // Node emits spawn before other events after successful process creation.
      // Require both the missing spawn event and missing PID for positive proof.
      failure = !spawned && child.pid === undefined ? new ControlNotDispatchedError(error) : error;
    });
    child.on('close', (code, childSignal) => {
      clearTimeout(timer);
      clearTimeout(killTimer);
      signal?.removeEventListener('abort', cancel);
      if (failure) return reject(failure);
      let response;
      try { response = JSON.parse(stdout); } catch {
        return reject(new Error(`Flywheel returned invalid JSON (exit ${code}): ${stderr.trim()}`));
      }
      if (!record(response) || response.protocol !== 1 || response.taskDir !== request.taskDir ||
          (response.cwd !== undefined && response.cwd !== request.cwd) ||
          typeof response.ok !== 'boolean') {
        return reject(new Error('Flywheel control response has incompatible protocol, taskDir, or cwd'));
      }
      if ((response.configuration !== undefined && !validConfiguration(response.configuration)) ||
          (response.diagnostic !== undefined && !validDiagnostic(response.diagnostic, request))) {
        return reject(new Error('Flywheel control returned a malformed configuration or diagnostic'));
      }
      if (request.action === 'doctor' && response.diagnostic !== undefined &&
          response.diagnostic.ok !== response.ok) {
        return reject(new Error('Flywheel doctor result disagrees with its control envelope'));
      }
      // A failed preflight is a usable report, unlike transport/protocol errors.
      // Only the documented failure exit is allowed through this exception.
      if (request.action === 'doctor' && code === 1 && !childSignal && response.ok === false &&
          response.diagnostic?.ok === false) return resolve(response);
      if (code !== 0 || childSignal || response.ok !== true) {
        return reject(new Error(typeof response.error === 'string' ? response.error :
          `Flywheel control failed (exit ${code}): ${stderr.trim()}`));
      }
      // After a repair handoff the implementation ledger owns status. This
      // read-only subcommand may return that ledger; mutations still require
      // their preparation receipt and cannot be confirmed by unrelated state.
      const investigationStatusAfterHandoff = request.investigation?.action === 'status' && response.preparation === undefined &&
        record(response.state) && Number.isSafeInteger(response.state.revision) && response.state.revision > 0 &&
        typeof response.state.status === 'string' && response.state.status !== '' && record(response.state.packets);
      if ((request.action === 'investigate' && !investigationStatusAfterHandoff && (!record(response.preparation) || response.preparation.version !== 1 || !Number.isSafeInteger(response.preparation.revision) || response.preparation.revision < 1 || !record(response.preparation.contract) || !Array.isArray(response.preparation.observations))) ||
          (request.action === 'images' && !validImageInspection(response.inputImages, request.images.paths)) ||
          (request.action === 'planning' && (!record(response.state?.planning) || response.state.planning.version !== 1 || !Number.isSafeInteger(response.state.revision) || response.state.revision < 1)) ||
          (request.action === 'node' && (!record(response.node) || response.node.id !== request.packet || !record(response.node.packet))) ||
          (['delivery','deliver'].includes(request.action) && (!record(response.delivery) || !Array.isArray(response.delivery.pulls))) ||
          (request.action === 'history' && (!record(response.history) || response.history.key !== request.sessionKey)) ||
          (request.action === 'runs' && !Array.isArray(response.runs)) ||
          (request.action === 'metrics' && (!record(response.metrics) || !Array.isArray(response.metrics.groups))) ||
          (request.action === 'config' && !response.configuration) ||
          (request.action === 'doctor' && !response.diagnostic) ||
          (request.editor && (!record(response.editor) || response.editor.scope !== request.editor.scope ||
            typeof response.editor.revision !== 'string' || !record(response.editor.values) || !record(response.editor.defaults)))) {
        return reject(new Error('Flywheel control response is missing its requested report'));
      }
      resolve(response);
    });
  });
}
