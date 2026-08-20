// Defect-injection spot check (DESIGN.md §4.3): guards against fake tests.
// The orchestrator runs this deterministically BETWEEN L1 and Review —
// inject a reverse-appliable patch, run the tests expecting them to go RED,
// restore EXACTLY the patch-touched files, and verify those paths are clean.
// The outcome feeds the reviewer; a fail (tests staying green = fake tests
// proven) escalates to the inbox (v0.14 DC3). ALWAYS ON — there is no off
// switch (user decision 2026-08-20); a sampled unit whose task lacks
// patch/test-cmd config goes to the inbox for a human to supply them.
//
// Restore is scoped to the patch's file list — never a repo-wide
// `git checkout -- .`, which would also revert the orchestrator's own ledger
// (state.json written mid-run) and crash CAS.

import { readFileSync } from 'node:fs';
import { isAbsolute, join } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { gateEnv } from './verify.js';

const defaultExec = promisify(execFile);

async function defaultRun(cmd, args, cwd, timeoutMs) {
  try {
    // gateEnv: a nested `node --test` in child-context mode exits 0 even when
    // red — the "tests must go red" check needs true exit codes.
    const { stdout, stderr } = await defaultExec(cmd, args, { cwd, env: gateEnv(), timeout: timeoutMs, maxBuffer: 4 * 1024 * 1024 });
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
 * Parse a unified patch into its touched files.
 * @returns {{created: string[], modified: string[]}} created = patch adds the
 * file (--- /dev/null); modified = everything else (edit or delete).
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
      else modified.push(strip(oldPath)); // deleted file: checkout restores it
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
 * without a real git repository.
 *
 * @param {object} args
 * @param {string} args.patchFile path to a reverse-appliable patch
 * @param {string} args.testCmd command expected to go red after injection
 * @param {string} args.cwd workspace
 * @param {number} [args.timeoutMs] cap for each spawned command
 * @param {(cmd:string, args:string[], cwd:string, timeoutMs?:number) => Promise<{exitCode:number, stdout:string, stderr:string}>} [args.run]
 */
export async function runDefectInjection({ patchFile, testCmd, cwd, timeoutMs = 300_000, run = defaultRun }) {
  const steps = { injected: false, testsWentRed: false, restored: false, workspaceClean: false };
  const patchAbs = isAbsolute(patchFile) ? patchFile : join(cwd, patchFile);

  const apply = await run('git', ['apply', patchAbs], cwd, timeoutMs);
  steps.injected = apply.exitCode === 0;
  if (!steps.injected) {
    return { ...steps, pass: false, detail: `git apply failed: ${apply.stderr.trim()}` };
  }

  const tests = await run('/bin/sh', ['-c', testCmd], cwd, timeoutMs);
  steps.testsWentRed = tests.exitCode !== 0;

  // Scoped restore: exactly the files the patch touched.
  const { created, modified } = parsePatchFiles(readFileSync(patchAbs, 'utf8'));
  let restoreOk = true;
  if (modified.length > 0) {
    const checkout = await run('git', ['checkout', '--', ...modified], cwd, timeoutMs);
    restoreOk = checkout.exitCode === 0;
  }
  if (restoreOk && created.length > 0) {
    const clean = await run('git', ['clean', '-f', '--', ...created], cwd, timeoutMs);
    restoreOk = clean.exitCode === 0;
  }
  steps.restored = restoreOk;

  if (steps.restored) {
    const touched = [...modified, ...created];
    const status = await run('git', ['status', '--porcelain', '--', ...touched], cwd, timeoutMs);
    steps.workspaceClean = status.exitCode === 0 && status.stdout.trim() === '';
  }

  // pass = the injected defect was actually caught by the tests AND the
  // touched paths were fully restored.
  const pass = steps.injected && steps.testsWentRed && steps.restored && steps.workspaceClean;
  return { ...steps, pass };
}
