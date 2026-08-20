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
  const run = async (cmd, args) => {
    calls.push([cmd, ...args].join(' '));
    for (const [pattern, result] of script) {
      if (calls[calls.length - 1].includes(pattern)) return result;
    }
    return { exitCode: 0, stdout: '', stderr: '' };
  };
  return { calls, run };
}

test('runDefectInjection: red tests + scoped restore = pass', async () => {
  const { dir, file } = patchDir();
  const { calls, run } = fakeRun([
    ['sh -c npm test', { exitCode: 1, stdout: 'FAIL', stderr: '' }], // tests went red
  ]);
  const r = await runDefectInjection({ patchFile: file, testCmd: 'npm test', cwd: dir, run });
  assert.equal(r.pass, true);
  assert.deepEqual({ injected: r.injected, testsWentRed: r.testsWentRed, restored: r.restored, workspaceClean: r.workspaceClean }, {
    injected: true, testsWentRed: true, restored: true, workspaceClean: true,
  });
  // Restore is scoped to the patch-touched files — never a repo-wide checkout
  // (which would revert the orchestrator's own ledger, P0-1).
  assert.ok(calls.some((c) => c === 'git checkout -- src/sum.js'));
  assert.ok(calls.some((c) => c === 'git clean -f -- src/new-helper.js'));
  assert.ok(!calls.some((c) => c === 'git checkout -- .'));
  assert.ok(calls.some((c) => c.startsWith('git status --porcelain -- src/sum.js')));
});

test('runDefectInjection: tests staying green means fake tests (fail)', async () => {
  const { dir, file } = patchDir();
  const { run } = fakeRun([
    ['sh -c npm test', { exitCode: 0, stdout: 'PASS', stderr: '' }],
  ]);
  const r = await runDefectInjection({ patchFile: file, testCmd: 'npm test', cwd: dir, run });
  assert.equal(r.injected, true);
  assert.equal(r.testsWentRed, false);
  assert.equal(r.pass, false);
});

test('runDefectInjection: dirty touched path after restore = fail', async () => {
  const { dir, file } = patchDir();
  const { run } = fakeRun([
    ['sh -c npm test', { exitCode: 1, stdout: '', stderr: '' }],
    ['git status --porcelain', { exitCode: 0, stdout: ' M src/sum.js\n', stderr: '' }],
  ]);
  const r = await runDefectInjection({ patchFile: file, testCmd: 'npm test', cwd: dir, run });
  assert.equal(r.restored, true);
  assert.equal(r.workspaceClean, false);
  assert.equal(r.pass, false);
});

test('runDefectInjection: un-appliable patch aborts early', async () => {
  const { dir, file } = patchDir();
  const { run } = fakeRun([
    ['git apply', { exitCode: 1, stdout: '', stderr: 'patch does not apply' }],
  ]);
  const r = await runDefectInjection({ patchFile: file, testCmd: 'npm test', cwd: dir, run });
  assert.equal(r.injected, false);
  assert.equal(r.pass, false);
  assert.match(r.detail, /does not apply/);
});
