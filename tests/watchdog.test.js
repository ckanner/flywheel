import { test } from 'node:test';
import assert from 'node:assert/strict';
import { runWithWatchdog, killActiveChildren } from '../orchestrator/core/watchdog.js';

test('watchdog: normal completion captures stdout and exit code', async () => {
  const r = await runWithWatchdog('node', ['-e', 'console.log("hello"); process.exit(0)'], {
    idleTimeoutMs: 5000,
    wallTimeoutMs: 10_000,
  });
  assert.equal(r.exitCode, 0);
  assert.equal(r.signal, null);
  assert.equal(r.timedOut, false);
  assert.match(r.stdout, /hello/);
});

test('watchdog: silent process is killed by the IDLE timeout', async () => {
  const r = await runWithWatchdog('node', ['-e', 'setTimeout(() => {}, 60_000)'], {
    idleTimeoutMs: 400,
    wallTimeoutMs: 30_000,
    killGraceMs: 300,
  });
  assert.equal(r.timedOut, true);
  assert.equal(r.idleTimedOut, true);
  assert.equal(r.wallTimedOut, false);
  assert.ok(r.signal === 'SIGTERM' || r.signal === 'SIGKILL', `signal=${r.signal}`);
  assert.ok(r.durationMs < 10_000, `took ${r.durationMs}ms`);
});

test('watchdog: steady output resets the idle timer (long task not killed)', async () => {
  // Prints every 150ms for ~1s, then exits — idle timeout 600ms must not fire.
  const script = `
    let i = 0;
    const t = setInterval(() => {
      console.log('tick' + i);
      if (++i >= 6) { clearInterval(t); process.exit(0); }
    }, 150);
  `;
  const r = await runWithWatchdog('node', ['-e', script], {
    idleTimeoutMs: 600,
    wallTimeoutMs: 15_000,
  });
  assert.equal(r.timedOut, false);
  assert.equal(r.exitCode, 0);
  assert.match(r.stdout, /tick5/);
});

test('watchdog: wall-clock cap fires even while output is flowing', async () => {
  const script = 'setInterval(() => console.log("noise"), 100); setTimeout(() => {}, 60_000);';
  const r = await runWithWatchdog('node', ['-e', script], {
    idleTimeoutMs: 5_000,
    wallTimeoutMs: 700,
    killGraceMs: 300,
  });
  assert.equal(r.timedOut, true);
  assert.equal(r.wallTimedOut, true);
  assert.equal(r.idleTimedOut, false);
});

test('watchdog: kill covers the whole process group (grandchild dies too)', async () => {
  // Child spawns a detached-in-group grandchild (`sleep`), prints its pid, hangs.
  let grandchildPid = null;
  const script = `sleep 60 & echo GRANDCHILD=$!; wait`;
  const r = await runWithWatchdog('bash', ['-c', script], {
    idleTimeoutMs: 600,
    wallTimeoutMs: 30_000,
    killGraceMs: 300,
    onActivity: (chunk) => {
      const m = /GRANDCHILD=(\d+)/.exec(chunk);
      if (m) grandchildPid = parseInt(m[1], 10);
    },
  });
  assert.equal(r.timedOut, true);
  assert.ok(grandchildPid, 'grandchild pid captured');
  // Quiescence: the grandchild must really die. Reaping races the child's
  // exit event under parallel load, so poll instead of asserting instantly.
  const deadline = Date.now() + 2000;
  let alive = true;
  while (Date.now() < deadline) {
    try {
      process.kill(grandchildPid, 0); // ESRCH = gone; EPERM = exists but not ours
      await new Promise((resolve) => setTimeout(resolve, 50));
    } catch (err) {
      if (err.code === 'ESRCH') {
        alive = false;
        break;
      }
      throw err;
    }
  }
  assert.equal(alive, false, `grandchild ${grandchildPid} still alive 2s after group kill`);
});

test('watchdog: spawn failure resolves with spawnError, never throws', async () => {
  const r = await runWithWatchdog('definitely-not-a-real-binary-xyz', [], {
    idleTimeoutMs: 1000,
    wallTimeoutMs: 5000,
  });
  assert.ok(r.spawnError);
  assert.equal(r.exitCode, null);
});

test('watchdog: resolves only after real exit (quiescence), not at kill time', async () => {
  // Child ignores SIGTERM and only dies on SIGKILL — resolution must still
  // come from the actual exit event.
  const script = 'process.on("SIGTERM", () => {}); setTimeout(() => {}, 60_000);';
  const r = await runWithWatchdog('node', ['-e', script], {
    idleTimeoutMs: 300,
    wallTimeoutMs: 30_000,
    killGraceMs: 400,
  });
  assert.equal(r.timedOut, true);
  assert.equal(r.signal, 'SIGKILL'); // SIGTERM was ignored, escalated
});

test('watchdog: a grandchild trapping SIGTERM still dies (post-exit group sweep)', async () => {
  // Leader AND grandchild both trap SIGTERM; only SIGKILL can take them.
  let gcPid = null;
  const script = `trap '' TERM; bash -c 'trap "" TERM; echo GC=$$; sleep 60' & wait`;
  const r = await runWithWatchdog('bash', ['-c', script], {
    idleTimeoutMs: 400,
    wallTimeoutMs: 30_000,
    killGraceMs: 300,
    onActivity: (chunk) => {
      const m = /GC=(\d+)/.exec(chunk);
      if (m) gcPid = parseInt(m[1], 10);
    },
  });
  assert.equal(r.timedOut, true);
  assert.ok(gcPid, 'grandchild pid captured');
  const deadline = Date.now() + 2000;
  let alive = true;
  while (Date.now() < deadline) {
    try {
      process.kill(gcPid, 0);
      await new Promise((resolve) => setTimeout(resolve, 50));
    } catch (err) {
      if (err.code === 'ESRCH') {
        alive = false;
        break;
      }
      throw err;
    }
  }
  assert.equal(alive, false, `trapping grandchild ${gcPid} survived the group sweep`);
});

test('watchdog: killActiveChildren terminates in-flight groups for a graceful stop', async () => {
  const pending = runWithWatchdog('node', ['-e', 'setTimeout(() => {}, 60_000)'], {
    idleTimeoutMs: 60_000,
    wallTimeoutMs: 60_000,
  });
  await new Promise((r) => setTimeout(r, 150)); // let it spawn and register
  await killActiveChildren();
  const r = await pending;
  assert.ok(r.signal === 'SIGTERM' || r.signal === 'SIGKILL', `signal=${r.signal}`);
});
