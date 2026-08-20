// Watchdog (DESIGN.md §6): dual timers — stream idle timeout (reset on any output
// activity, so long tasks are not killed) plus an overall wall-clock cap.
// Kill targets the whole process group and must await real exit (quiescence)
// before the caller retries, so no orphan can corrupt the workspace.
//
// Active children are registered so the orchestrator can kill everything
// in-flight on SIGTERM (flywheel stop), again awaiting quiescence before exit.

import { spawn } from 'node:child_process';

const MAX_BUFFER = 16 * 1024 * 1024; // 16MB per stream

// pid -> { killGroup, exited: Promise }
const activeChildren = new Map();

/**
 * Kill every in-flight child process group and await real exit (quiescence).
 * Used by the orchestrator's SIGTERM handler before it exits itself.
 */
export async function killActiveChildren(graceMs = 5000) {
  const exits = [];
  for (const { killGroup, exited } of activeChildren.values()) {
    killGroup('SIGTERM');
    exits.push(exited);
  }
  if (exits.length === 0) return;
  await Promise.race([
    Promise.allSettled(exits),
    new Promise((r) => setTimeout(r, graceMs)),
  ]);
  // Last resort for anything that trapped SIGTERM.
  for (const { killGroup } of activeChildren.values()) {
    killGroup('SIGKILL');
  }
  await Promise.allSettled(exits);
}

/**
 * @param {string} command
 * @param {string[]} args
 * @param {object} opts
 * @param {string} [opts.cwd]
 * @param {object} [opts.env]
 * @param {number} [opts.idleTimeoutMs=300000] reset on every stdout/stderr activity
 * @param {number} [opts.wallTimeoutMs=3600000] absolute cap
 * @param {number} [opts.killGraceMs=5000] SIGTERM -> SIGKILL escalation grace
 * @param {(chunk: string, stream: 'stdout'|'stderr') => void} [opts.onActivity]
 * @returns {Promise<{
 *   exitCode: number|null, signal: string|null,
 *   timedOut: boolean, idleTimedOut: boolean, wallTimedOut: boolean,
 *   stdout: string, stderr: string, truncated: boolean,
 *   durationMs: number, spawnError: Error|null
 * }>}
 */
export function runWithWatchdog(command, args = [], opts = {}) {
  const {
    cwd,
    env,
    idleTimeoutMs = 300_000,
    wallTimeoutMs = 3_600_000,
    killGraceMs = 5_000,
    onActivity,
  } = opts;

  return new Promise((resolve) => {
    const startedAt = Date.now();

    const done = (extra) => ({
      exitCode: null,
      signal: null,
      timedOut: false,
      idleTimedOut: false,
      wallTimedOut: false,
      stdout: '',
      stderr: '',
      truncated: false,
      durationMs: Date.now() - startedAt,
      spawnError: null,
      ...extra,
    });

    let child;
    try {
      child = spawn(command, args, {
        cwd,
        env,
        stdio: ['ignore', 'pipe', 'pipe'],
        detached: true, // own process group, so we can kill the whole tree
      });
    } catch (spawnError) {
      resolve(done({ spawnError }));
      return;
    }

    let stdout = '';
    let stderr = '';
    let truncated = false;
    let settled = false;
    let idleTimedOut = false;
    let wallTimedOut = false;
    let killTimer = null;
    let killSent = false;
    let exitResolve;
    const exited = new Promise((r) => { exitResolve = r; });

    const finish = (extra = {}) => {
      if (settled) return;
      settled = true;
      clearTimeout(idleTimer);
      clearTimeout(wallTimer);
      if (killTimer) clearTimeout(killTimer);
      activeChildren.delete(child.pid);
      exitResolve();
      resolve(done({ stdout, stderr, truncated, ...extra }));
    };

    const killGroup = (signal) => {
      killSent = true;
      try {
        // Negative pid = the whole process group (child is group leader).
        process.kill(-child.pid, signal);
      } catch {
        try { child.kill(signal); } catch { /* already dead */ }
      }
    };

    activeChildren.set(child.pid, { killGroup, exited });

    const beginKill = () => {
      killGroup('SIGTERM');
      killTimer = setTimeout(() => killGroup('SIGKILL'), killGraceMs);
      if (killTimer.unref) killTimer.unref();
    };

    const idleTimer = setTimeout(() => {
      idleTimedOut = true;
      beginKill();
    }, idleTimeoutMs);

    const wallTimer = setTimeout(() => {
      wallTimedOut = true;
      beginKill();
    }, wallTimeoutMs);

    const resetIdle = () => {
      if (settled || idleTimedOut) return;
      idleTimer.refresh();
    };

    const onData = (stream) => (chunk) => {
      const text = chunk.toString('utf8');
      if (stream === 'stdout') {
        if (stdout.length < MAX_BUFFER) stdout += text;
        else truncated = true;
      } else {
        if (stderr.length < MAX_BUFFER) stderr += text;
        else truncated = true;
      }
      resetIdle(); // any activity proves the stream is alive
      if (onActivity) onActivity(text, stream);
    };

    child.stdout.on('data', onData('stdout'));
    child.stderr.on('data', onData('stderr'));

    child.on('error', (spawnError) => {
      finish({ spawnError, stdout, stderr, truncated });
    });

    // Await quiescence: we resolve only from 'exit' (or 'error'), i.e. after the
    // process really died — never right after sending the kill signal.
    child.on('exit', (exitCode, signal) => {
      if (killSent) {
        // Best-effort last sweep: the group leader is dead, but a grandchild
        // that trapped SIGTERM may linger — never leave an orphan behind.
        try { process.kill(-child.pid, 'SIGKILL'); } catch { /* group gone */ }
      }
      finish({
        exitCode,
        signal,
        timedOut: idleTimedOut || wallTimedOut,
        idleTimedOut,
        wallTimedOut,
      });
    });
  });
}
