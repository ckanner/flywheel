// Defect-injection spot check (DESIGN.md §4.3): guards against fake tests.
// The orchestrator runs this deterministically BETWEEN L1 and Review —
// establish a GREEN baseline, inject a reverse-appliable patch, run the tests
// expecting them to go RED, then restore by REVERSE-APPLYING the patch.
// ALWAYS ON (user decision 2026-08-20 — no switch).
//
// Outcome cases (P0-2, user-verified):
//   pass          baseline green + injection made tests red + restore clean
//   fake-tests    tests stayed green with the defect in (DC3 -> inbox + stop)
//   uninjectable  patch unreadable / touches ledger paths / does not apply
//                 (stale patch -> inbox asking a human to refresh, NO
//                 fake-test accusation, unit continues)
//   baseline-red  the suite was already red before injection (broken suite
//                 -> inbox, unit continues)
//   restore-failed reverse apply failed (workspace may still hold the
//                 defect -> inbox + stop)
//
// Restore is `git apply -R` (P0-1, user-verified): it removes exactly the
// injected hunks and preserves everything else the executor did — a
// `git checkout -- <files>` restore would wipe the executor's uncommitted
// work on those files. Reverse apply also covers binary/rename/mode-change
// patches, and no "workspaceClean" check is needed afterwards: the executor's
// own diff on touched files is SUPPOSED to be there.

import { readFileSync } from 'node:fs';
import { isAbsolute, join } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { gateEnv } from './child-env.js';

const defaultExec = promisify(execFile);

async function defaultRun(cmd, args, cwd, timeoutMs, envAllowlist = []) {
  try {
    const { stdout, stderr } = await defaultExec(cmd, args, {
      cwd,
      env: gateEnv(process.env, { allow: envAllowlist }),
      timeout: timeoutMs,
      maxBuffer: 4 * 1024 * 1024,
    });
    return { exitCode: 0, stdout: stdout ?? '', stderr: stderr ?? '' };
  } catch (err) {
    return {
      exitCode: typeof err.code === 'number' ? err.code : 1,
      stdout: err.stdout ?? '',
      stderr: err.stderr ?? String(err.message ?? ''),
    };
  }
}

/**
 * Parse a unified patch into its touched files (paths only — used for the
 * ledger-path guard; restore no longer depends on this list).
 * @returns {{created: string[], modified: string[]}}
 */
export function parsePatchFiles(patchContent) {
  const created = [];
  const modified = [];
  let oldPath = null;
  for (const line of patchContent.split('\n')) {
    if (line.startsWith('--- ')) {
      oldPath = line.slice(4).trim();
    } else if (line.startsWith('+++ ')) {
      const newPath = line.slice(4).trim();
      const strip = (p) => (p.startsWith('a/') || p.startsWith('b/') ? p.slice(2) : p);
      if (oldPath === '/dev/null') created.push(strip(newPath));
      else if (newPath !== '/dev/null') modified.push(strip(newPath));
      else modified.push(strip(oldPath)); // deleted file
      oldPath = null;
    }
  }
  return { created: [...new Set(created)], modified: [...new Set(modified)] };
}

/**
 * Sampling decision (always on — there is no off switch, user decision
 * 2026-08-20): every N units (the CURRENT unit counts as completedCount + 1,
 * so the very first unit is sampled when N == 1), and more eagerly when the
 * diff is dominated by test files. Accepts both kebab-case (config file) and
 * camelCase option keys.
 */
export function shouldSample(completedUnitCount, opts = {}) {
  const everyNUnits = opts['every-n-units'] ?? opts.everyNUnits ?? 5;
  const boost = opts['test-file-ratio-boost'] ?? opts.testFileRatioBoost ?? 0.5;
  const testFileRatio = opts.testFileRatio ?? 0;
  if (testFileRatio >= boost && completedUnitCount >= 1) return true;
  return (completedUnitCount + 1) % everyNUnits === 0;
}

/**
 * Run one injection cycle. `run` is injectable so the logic is testable
 * without a real git repository. Never throws — a broken patch file or any
 * spawn failure maps to an outcome, not an orchestrator crash (P0-2).
 *
 * @param {object} args
 * @param {string} args.patchFile path to a reverse-appliable patch
 * @param {string} args.testCmd command expected to go red after injection
 * @param {string} args.cwd workspace
 * @param {number} [args.timeoutMs] cap for each spawned command
 * @param {string[]} [args.envAllowlist] extra env keys for the test command
 * @param {string[]} [args.forbidPaths] repo-relative paths the patch must not
 *   touch (the orchestrator's ledger zone) — a hit refuses injection (P0-3)
 * @param {(cmd:string, args:string[], cwd:string, timeoutMs?:number, envAllowlist?:string[]) => Promise<{exitCode:number, stdout:string, stderr:string}>} [args.run]
 */
export async function runDefectInjection({ patchFile, testCmd, cwd, timeoutMs = 300_000, envAllowlist = [], forbidPaths = [], run = defaultRun }) {
  const base = { injected: false, baselineGreen: null, testsWentRed: false, restored: false, pass: false };
  let patchAbs;
  let patchContent;
  try {
    patchAbs = isAbsolute(patchFile) ? patchFile : join(cwd, patchFile);
    patchContent = readFileSync(patchAbs, 'utf8');
  } catch (err) {
    return { ...base, outcome: 'uninjectable', detail: `patch unreadable: ${err.message}` };
  }

  // P0-3: a patch touching the ledger zone is refused outright.
  const { created, modified } = parsePatchFiles(patchContent);
  const touched = [...created, ...modified];
  const forbidden = touched.filter((f) => forbidPaths.includes(f));
  if (forbidden.length > 0) {
    return { ...base, outcome: 'uninjectable', detail: `patch touches forbidden (ledger) paths: ${forbidden.join(', ')}` };
  }

  // Baseline first (P0-2 case 3): "tests went red" proves nothing when the
  // suite was already broken before the injection.
  const baseline = await run('/bin/sh', ['-c', testCmd], cwd, timeoutMs, envAllowlist);
  if (baseline.exitCode !== 0) {
    return { ...base, outcome: 'baseline-red', detail: 'test suite red before injection (baseline broken)' };
  }

  const apply = await run('git', ['apply', patchAbs], cwd, timeoutMs, envAllowlist);
  if (apply.exitCode !== 0) {
    return { ...base, outcome: 'uninjectable', detail: `git apply failed: ${apply.stderr.trim()}`, baselineGreen: true };
  }

  const tests = await run('/bin/sh', ['-c', testCmd], cwd, timeoutMs, envAllowlist);
  const testsWentRed = tests.exitCode !== 0;

  // Restore by reverse-applying the patch (P0-1).
  const restore = await run('git', ['apply', '-R', patchAbs], cwd, timeoutMs, envAllowlist);
  const restored = restore.exitCode === 0;
  if (!restored) {
    return { injected: true, baselineGreen: true, testsWentRed, restored: false, pass: false, outcome: 'restore-failed', detail: `git apply -R failed: ${restore.stderr.trim()}` };
  }

  const pass = testsWentRed;
  return {
    injected: true,
    baselineGreen: true,
    testsWentRed,
    restored: true,
    pass,
    outcome: pass ? 'pass' : 'fake-tests',
    detail: testsWentRed ? undefined : 'tests stayed green with the defect injected (fake/weak tests)',
  };
}
