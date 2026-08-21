import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { shouldSample, runDefectInjection, parsePatchFiles } from '../orchestrator/core/defect-injection.js';

const PATCH = `--- a/src/sum.js
+++ b/src/sum.js
@@ -1,3 +1,3 @@
 export function sum(a, b) {
-  return a + b;
+  return a - b;
 }
--- /dev/null
+++ b/src/new-helper.js
@@ -0,0 +1 @@
+export const x = 1;
`;

function patchDir(content = PATCH) {
  const dir = mkdtempSync(join(tmpdir(), 'flywheel-di-'));
  const file = join(dir, 'defect.patch');
  writeFileSync(file, content);
  return { dir, file };
}

test('parsePatchFiles: separates created from modified/deleted files', () => {
  const { created, modified } = parsePatchFiles(PATCH);
  assert.deepEqual(created, ['src/new-helper.js']);
  assert.deepEqual(modified, ['src/sum.js']);
});

test('shouldSample: always on (no switch), every N units with the current unit counted', () => {
  // Current unit is the (completedCount + 1)-th: samples land on the 5th, 10th, ...
  assert.equal(shouldSample(4, { 'every-n-units': 5 }), true);
  assert.equal(shouldSample(5, { 'every-n-units': 5 }), false);
  assert.equal(shouldSample(9, { 'every-n-units': 5 }), true);
  assert.equal(shouldSample(0, { 'every-n-units': 5 }), false);
  assert.equal(shouldSample(0, { 'every-n-units': 1 }), true); // first unit sampled at N=1
  assert.equal(shouldSample(9, { everyNUnits: 5 }), true); // camelCase accepted too
});

test('shouldSample: test-heavy diffs boost the ratio', () => {
  const opts = { 'every-n-units': 5, 'test-file-ratio-boost': 0.5, testFileRatio: 0.8 };
  assert.equal(shouldSample(2, opts), true); // not a multiple of 5, but test-dominated
});

function fakeRun(script) {
  const calls = [];
  const entries = Array.isArray(script) ? script : Object.entries(script);
  const run = async (cmd, args) => {
    calls.push([cmd, ...args].join(' '));
    for (const [pattern, result] of entries) {
      if (calls[calls.length - 1].includes(pattern)) return result;
    }
    return { exitCode: 0, stdout: '', stderr: '' };
  };
  return { calls, run };
}

/** Baseline green, post-injection red (or always-green when redAfterApply=false). */
function scriptedTestRun({ redAfterApply = true, applyFails = false, reverseFails = false, patchFile }) {
  const calls = [];
  let testRuns = 0;
  const run = async (cmd, args) => {
    const joined = [cmd, ...args].join(' ');
    calls.push(joined);
    if (joined.includes('npm test')) {
      testRuns += 1;
      const isBaseline = testRuns === 1;
      return { exitCode: isBaseline ? 0 : redAfterApply ? 1 : 0, stdout: '', stderr: '' };
    }
    if (joined.startsWith('git apply -R')) return { exitCode: reverseFails ? 1 : 0, stdout: '', stderr: reverseFails ? 'conflict' : '' };
    if (joined === `git apply ${patchFile}`) return { exitCode: applyFails ? 1 : 0, stdout: '', stderr: applyFails ? 'patch does not apply' : '' };
    return { exitCode: 0, stdout: '', stderr: '' };
  };
  return { calls, run, get testRuns() { return testRuns; } };
}

test('runDefectInjection: baseline green + tests red + reverse-apply restore = pass', async () => {
  const { dir, file } = patchDir();
  const scripted = scriptedTestRun({ patchFile: file });
  const r = await runDefectInjection({ patchFile: file, testCmd: 'npm test', cwd: dir, run: scripted.run });
  assert.equal(r.outcome, 'pass');
  assert.equal(r.pass, true);
  assert.equal(r.testsWentRed, true);
  assert.equal(r.restored, true);
  assert.equal(r.baselineGreen, true);
  // Restore is reverse-apply, NEVER a checkout of touched files (P0-1).
  assert.ok(scripted.calls.some((c) => c.startsWith('git apply -R')));
  assert.ok(!scripted.calls.some((c) => c.includes('git checkout')));
  assert.ok(!scripted.calls.some((c) => c.includes('git clean')));
  assert.ok(!scripted.calls.some((c) => c.includes('git status'))); // no workspaceClean check anymore
  assert.equal(scripted.testRuns, 2); // baseline + post-injection
});

test('runDefectInjection: tests staying green = fake-tests (DC3)', async () => {
  const { dir, file } = patchDir();
  const { run } = scriptedTestRun({ redAfterApply: false, patchFile: file });
  const r = await runDefectInjection({ patchFile: file, testCmd: 'npm test', cwd: dir, run });
  assert.equal(r.injected, true);
  assert.equal(r.testsWentRed, false);
  assert.equal(r.outcome, 'fake-tests');
  assert.equal(r.pass, false);
});

test('runDefectInjection: baseline red = baseline-red, patch never applied', async () => {
  const { dir, file } = patchDir();
  const calls = [];
  const run = async (cmd, args) => {
    calls.push([cmd, ...args].join(' '));
    return { exitCode: 1, stdout: 'FAIL', stderr: '' }; // every command fails — baseline is red
  };
  const r = await runDefectInjection({ patchFile: file, testCmd: 'npm test', cwd: dir, run });
  assert.equal(r.outcome, 'baseline-red');
  assert.equal(r.injected, false);
  assert.ok(!calls.some((c) => c.startsWith('git apply')));
});

test('runDefectInjection: stale patch that does not apply = uninjectable (no fake-test accusation)', async () => {
  const { dir, file } = patchDir();
  const scripted = scriptedTestRun({ applyFails: true, patchFile: file });
  const r = await runDefectInjection({ patchFile: file, testCmd: 'npm test', cwd: dir, run: scripted.run });
  assert.equal(r.outcome, 'uninjectable');
  assert.match(r.detail, /does not apply/);
  assert.equal(r.pass, false);
  assert.equal(scripted.testRuns, 1); // baseline only
});

test('runDefectInjection: reverse-apply failure = restore-failed', async () => {
  const { dir, file } = patchDir();
  const { run } = scriptedTestRun({ reverseFails: true, patchFile: file });
  const r = await runDefectInjection({ patchFile: file, testCmd: 'npm test', cwd: dir, run });
  assert.equal(r.outcome, 'restore-failed');
  assert.equal(r.restored, false);
  assert.match(r.detail, /apply -R failed/);
});

test('runDefectInjection: unreadable patch = uninjectable, never throws (P0-2)', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'flywheel-di-'));
  const r = await runDefectInjection({ patchFile: 'no-such.patch', testCmd: 'true', cwd: dir, run: fakeRun({}).run });
  assert.equal(r.outcome, 'uninjectable');
  assert.match(r.detail, /patch unreadable/);
});

test('runDefectInjection: patch touching a forbidden (ledger) path is refused (P0-3)', async () => {
  const ledgerPatch = `--- a/docs/x/state.json
+++ b/docs/x/state.json
@@ -1 +1 @@
-{}
+{"evil":true}
`;
  const { dir, file } = patchDir(ledgerPatch);
  const { calls, run } = fakeRun({});
  const r = await runDefectInjection({
    patchFile: file, testCmd: 'npm test', cwd: dir, run,
    forbidPaths: ['docs/x/state.json', 'docs/x/reports/'],
  });
  assert.equal(r.outcome, 'uninjectable');
  assert.match(r.detail, /ledger|forbidden/);
  assert.equal(calls.length, 0); // refused before any command
});
