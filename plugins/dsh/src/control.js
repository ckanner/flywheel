import { spawn } from 'node:child_process';
import { directory, nonempty, record } from './protocol.js';

export async function validateControl(input, defaultCwd) {
  if (!record(input) || !['start', 'status', 'stop'].includes(input.action)) {
    throw new Error('action must be start, status, or stop');
  }
  if (Object.keys(input).some(key => !['action', 'taskDir', 'cwd'].includes(key))) {
    throw new Error('unknown Flywheel control argument');
  }
  return {
    action: input.action,
    taskDir: await directory(input.taskDir, 'taskDir'),
    cwd: await directory(input.cwd ?? defaultCwd, 'cwd'),
  };
}

/** Only the engine executable can mutate engine state. No shell interpolation. */
export async function runControl(input, { binary = 'flywheel', defaultCwd, signal, timeoutMs = 60000 } = {}) {
  nonempty(binary, 'flywheelBinary', 4096);
  if (binary.startsWith('-')) throw new Error('flywheelBinary must name an executable');
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs < 100 || timeoutMs > 120000) {
    throw new Error('controlTimeoutMs must be between 100 and 120000');
  }
  const request = await validateControl(input, defaultCwd);
  if (signal?.aborted) throw new Error('Flywheel control request cancelled');
  return new Promise((resolve, reject) => {
    const child = spawn(binary, ['control', request.action, '--task-dir', request.taskDir, '--cwd', request.cwd], {
      cwd: request.cwd, detached: false, stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    let failure;
    let killTimer;
    const stop = error => {
      failure ??= error;
      child.kill('SIGTERM');
      killTimer ??= setTimeout(() => child.kill('SIGKILL'), 500);
    };
    const cancel = () => stop(new Error('Flywheel control request cancelled'));
    const timer = setTimeout(() => stop(new Error('Flywheel control request timed out')), timeoutMs);
    signal?.addEventListener('abort', cancel, { once: true });
    child.stdout.on('data', chunk => {
      if (failure) return;
      stdout += chunk;
      if (Buffer.byteLength(stdout) > 1024 * 1024) stop(new Error('Flywheel control response exceeds 1 MiB'));
    });
    child.stderr.on('data', chunk => { stderr = (stderr + chunk).slice(-8192); });
    child.on('error', error => { failure = error; });
    child.on('close', (code, childSignal) => {
      clearTimeout(timer);
      clearTimeout(killTimer);
      signal?.removeEventListener('abort', cancel);
      if (failure) return reject(failure);
      let response;
      try { response = JSON.parse(stdout); } catch {
        return reject(new Error(`Flywheel returned invalid JSON (exit ${code}): ${stderr.trim()}`));
      }
      if (!record(response) || response.protocol !== 1 || response.taskDir !== request.taskDir) {
        return reject(new Error('Flywheel control response has incompatible protocol or taskDir'));
      }
      if (code !== 0 || childSignal || response.ok !== true) {
        return reject(new Error(typeof response.error === 'string' ? response.error :
          `Flywheel control failed (exit ${code}): ${stderr.trim()}`));
      }
      resolve(response);
    });
  });
}
