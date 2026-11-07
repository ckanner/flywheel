import { spawn } from 'node:child_process';
import { directory, nonempty, record } from './protocol.js';

const actions = ['launch', 'start', 'status', 'stop', 'config', 'doctor'];
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
    throw new Error('action must be launch, start, status, stop, config, or doctor');
  }
  if (Object.keys(input).some(key => !['action', 'taskDir', 'cwd', 'probe', 'mode'].includes(key))) {
    throw new Error('unknown Flywheel control argument');
  }
  if ('probe' in input && (input.action !== 'doctor' || typeof input.probe !== 'boolean')) {
    throw new Error('probe must be a boolean and is only supported by doctor');
  }
  if ('mode' in input && (!['launch', 'start'].includes(input.action) || !['auto', 'quick', 'full'].includes(input.mode))) {
    throw new Error('mode must be auto, quick, or full and is only supported by launch/start');
  }
  const needsTask = !['config', 'doctor'].includes(input.action);
  return {
    action: input.action,
    taskDir: needsTask || 'taskDir' in input ? await directory(input.taskDir, 'taskDir') : '',
    cwd: await directory(input.cwd ?? defaultCwd, 'cwd'),
    ...(input.action === 'doctor' ? { probe: input.probe ?? false } : {}),
    ...(input.action === 'launch' || 'mode' in input ? { mode: input.mode ?? 'auto' } : {}),
  };
}

function validConfiguration(value) {
  return record(value) && typeof value.source === 'string' && typeof value.scope === 'string' &&
    Array.isArray(value.bindings) && value.bindings.every(binding => record(binding) &&
      ['id', 'role', 'harness'].every(key =>
        typeof binding[key] === 'string' && binding[key].trim() !== '') &&
      ['route', 'profile', 'provider', 'channel', 'model', 'thinking', 'permissionMode'].every(key =>
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
export async function runControl(input, { binary = 'flywheel', defaultCwd, signal, timeoutMs = 60000 } = {}) {
  let request;
  try {
    nonempty(binary, 'flywheelBinary', 4096);
    if (binary.startsWith('-')) throw new Error('flywheelBinary must name an executable');
    if (!Number.isSafeInteger(timeoutMs) || timeoutMs < 100 || timeoutMs > 120000) {
      throw new Error('controlTimeoutMs must be between 100 and 120000');
    }
    request = await validateControl(input, defaultCwd);
    if (signal?.aborted) throw new Error('Flywheel control request cancelled');
  } catch (error) {
    throw new ControlNotDispatchedError(error);
  }
  // Diagnostic commands own watchdog child groups. Its TERM grace, pipe
  // WaitDelay, and quiescence confirmation may each take 5s; keep the Go owner
  // alive long enough to finish all three instead of orphaning its workers.
  const killGraceMs = ['config', 'doctor'].includes(request.action) ? 20000 : 500;
  const args = ['control', request.action, '--cwd', request.cwd];
  if (request.taskDir) args.push('--task-dir', request.taskDir);
  if (request.probe) args.push('--probe');
  if (request.mode) args.push('--mode', request.mode);
  return new Promise((resolve, reject) => {
    let child;
    try {
      child = spawn(binary, args, {
        cwd: request.cwd, detached: false, stdio: ['ignore', 'pipe', 'pipe'],
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
      if ((request.action === 'config' && !response.configuration) ||
          (request.action === 'doctor' && !response.diagnostic)) {
        return reject(new Error('Flywheel control response is missing its requested report'));
      }
      resolve(response);
    });
  });
}
