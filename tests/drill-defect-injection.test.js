import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { runDefectInjection } from '../orchestrator/core/defect-injection.js';

// Defect-injection drill against a REAL git repository (DESIGN.md §4.3):
// real git apply / real tests going red / real `git checkout -- .` restore /
// real clean-tree verification — no fake exec. See drills/defect-injection.md.

function makeRepo() {
  const dir = mkdtempSync(join(tmpdir(), 'flywheel-drill-di-'));
  const git = (args) => execFileSync('git', ['-C', dir, '-c', 'user.name=t', '-c', 'user.email=t@l', ...args], { encoding: 'utf8' });
  mkdirSync(join(dir, 'src'), { recursive: true });
  writeFileSync(join(dir, 'src', 'sum.js'), 'export function sum(a, b) {\n  return a + b;\n}\n');
  writeFileSync(
    join(dir, 'sum.test.js'),
    "import { test } from 'node:test';\nimport assert from 'node:assert/strict';\nimport { sum } from './src/sum.js';\ntest('sum adds', () => assert.equal(sum(2, 3), 5));\n",
  );
  writeFileSync(join(dir, 'package.json'), JSON.stringify({ name: 'drill', type: 'module' }));
  git(['init', '-q']);
  git(['add', '-A']);
  git(['commit', '-q', '-m', 'initial']);
  return dir;
}

const PATCH = `--- a/src/sum.js
+++ b/src/sum.js
@@ -1,3 +1,3 @@
 export function sum(a, b) {
-  return a + b;
+  return a - b;
 }
`;

test('drill (real git): injected defect turns tests red, restore leaves a clean tree', async () => {
  const dir = makeRepo();
  const patchFile = join(dir, 'defect.patch');
  writeFileSync(patchFile, PATCH);
  execFileSync('git', ['-C', dir, '-c', 'user.name=t', '-c', 'user.email=t@l', 'add', 'defect.patch']);
  execFileSync('git', ['-C', dir, '-c', 'user.name=t', '-c', 'user.email=t@l', 'commit', '-q', '-m', 'add patch']);

  const before = readFileSync(join(dir, 'src', 'sum.js'), 'utf8');
  const outcome = await runDefectInjection({ patchFile, testCmd: 'node --test sum.test.js', cwd: dir });
  console.log('[drill-defect-injection]', JSON.stringify(outcome));
  assert.equal(outcome.injected, true);
  assert.equal(outcome.testsWentRed, true); // the test suite actually catches the defect
  assert.equal(outcome.restored, true);
  assert.equal(outcome.workspaceClean, true);
  assert.equal(outcome.pass, true);
  assert.equal(readFileSync(join(dir, 'src', 'sum.js'), 'utf8'), before); // restored byte-for-byte
});

test('drill (real git): a patch the tests do NOT catch fails the check', async () => {
  const dir = makeRepo();
  // Cosmetic change the test cannot detect.
  const patchFile = join(dir, 'cosmetic.patch');
  writeFileSync(patchFile, `--- a/src/sum.js
+++ b/src/sum.js
@@ -1,3 +1,3 @@
 export function sum(a, b) {
-  return a + b;
+  return (a + b);
 }
`);
  execFileSync('git', ['-C', dir, '-c', 'user.name=t', '-c', 'user.email=t@l', 'add', 'cosmetic.patch']);
  execFileSync('git', ['-C', dir, '-c', 'user.name=t', '-c', 'user.email=t@l', 'commit', '-q', '-m', 'add patch']);

  const outcome = await runDefectInjection({ patchFile, testCmd: 'node --test sum.test.js', cwd: dir });
  assert.equal(outcome.injected, true);
  assert.equal(outcome.testsWentRed, false); // tests stayed green = weak tests detected
  assert.equal(outcome.pass, false);
  assert.equal(outcome.workspaceClean, true); // still restored
});
