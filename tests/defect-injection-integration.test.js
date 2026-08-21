// P0-1 integration: defect injection BETWEEN L1 and Review, against a real
// git fixture — inject -> tests go red -> reviewer SEES the outcome ->
// scoped restore leaves the orchestrator's ledger (state.json) untouched.
// DC3: an injection the tests do not catch escalates to the inbox.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { runTask } from '../orchestrator/core/machine.js';
import { loadConfig } from '../orchestrator/core/config.js';
import { loadState } from '../orchestrator/core/state.js';
import { createMockDriver } from '../orchestrator/drivers/mock.js';
import { makeGitFixture } from './helpers.js';

const noSleep = async () => {};
const noLog = () => {};

const SUM_SRC = 'export function sum(a, b) {\n  return a + b;\n}\n';
const SUM_TEST = "import { test } from 'node:test';\nimport assert from 'node:assert/strict';\nimport { sum } from './src/sum.js';\ntest('sum adds', () => assert.equal(sum(2, 3), 5));\n";
const BREAK_PATCH = `--- a/src/sum.js
+++ b/src/sum.js
@@ -1,3 +1,3 @@
 export function sum(a, b) {
-  return a + b;
+  return a - b;
 }
`;
const COSMETIC_PATCH = `--- a/src/sum.js
+++ b/src/sum.js
@@ -1,3 +1,3 @@
 export function sum(a, b) {
-  return a + b;
+  return (a + b);
 }
`;

function diFixture(patchName, patchContent) {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  writeFileSync(join(fx.root, 'src', 'sum.js'), SUM_SRC);
  writeFileSync(join(fx.root, 'sum.test.js'), SUM_TEST);
  writeFileSync(join(fx.root, patchName), patchContent);
  // Config change is committed BEFORE the run — the contract layer must be
  // committed content, or the diff judgement (correctly) flags it.
  const cfg = JSON.parse(readFileSync(join(fx.taskDir, 'flywheel.config.json'), 'utf8'));
  cfg['defect-injection'] = { 'every-n-units': 1, 'test-file-ratio-boost': 0.5, patch: patchName, 'test-cmd': 'node --test sum.test.js' };
  writeFileSync(join(fx.taskDir, 'flywheel.config.json'), JSON.stringify(cfg));
  const git = (args) => execFileSync('git', ['-C', fx.root, '-c', 'user.name=t', '-c', 'user.email=t@l', ...args]);
  git(['add', '-A']);
  git(['commit', '-q', '-m', 'add sum package + patch + di config']);
  return fx;
}

function wrapDrivers(overrides = {}) {
  const mock = createMockDriver();
  return {
    plan: { plan: overrides.plan ?? ((a) => mock.plan(a)) },
    execute: { execute: overrides.execute ?? ((a) => mock.execute(a)) },
    review: { review: overrides.review ?? ((a) => mock.review(a)) },
    judge: { judge: overrides.judge ?? ((a) => mock.judge(a)) },
  };
}

function ctx(fixture, drivers) {
  const config = loadConfig(fixture.taskDir, fixture.root);
  return { taskDir: fixture.taskDir, cwd: fixture.root, config, drivers, sleep: noSleep, log: noLog };
}

test('P0-1: injection runs BEFORE review, reviewer sees the outcome, ledger untouched by restore', async () => {
  const fx = diFixture('defect.patch', BREAK_PATCH);
  const reviewPrompts = [];
  const drivers = wrapDrivers({
    review: async (args) => {
      reviewPrompts.push(args.prompt);
      return createMockDriver().review(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');

  const state = loadState(fx.taskDir);
  const p = state.packets['M01-T01'];
  // Injection outcome recorded as evidence and it PASSED (tests caught the defect).
  const di = p.evidence['defect-injection'];
  assert.ok(di, 'defect-injection evidence recorded');
  assert.equal(di.outcome, 'pass');
  assert.equal(di.testsWentRed, true);
  assert.equal(di.restored, true); // via git apply -R (P0-1)
  assert.equal(di.baselineGreen, true);

  // §4.3: the outcome was fed to the reviewer.
  assert.ok(reviewPrompts.length > 0);
  assert.match(reviewPrompts[0], /Defect-injection spot check/);
  assert.match(reviewPrompts[0], /"testsWentRed": true/);

  // The reverse-apply restore never touched the ledger or the executor's
  // work: state.json CAS chain intact, source byte-restored, build artifact kept.
  assert.equal(state.status, 'completed');
  assert.ok(state.revision > 2);
  assert.equal(readFileSync(join(fx.root, 'src', 'sum.js'), 'utf8'), SUM_SRC);
  assert.ok(existsSync(join(fx.root, 'build', 'M01-T01.txt')), 'executor artifact preserved through injection+restore');
  const status = execFileSync('git', ['-C', fx.root, 'status', '--porcelain', '--', 'src/sum.js', 'sum.test.js'], { encoding: 'utf8' });
  assert.equal(status.trim(), '');
});

test('P0-1/DC3: injection the tests do NOT catch escalates to inbox (fake tests proven)', async () => {
  const fx = diFixture('cosmetic.patch', COSMETIC_PATCH);
  let reviewCalls = 0;
  const drivers = wrapDrivers({ review: async () => { reviewCalls += 1; return createMockDriver().review({}); } });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated');
  const state = loadState(fx.taskDir);
  const p = state.packets['M01-T01'];
  assert.equal(p.status, 'inbox');
  assert.equal(p.evidence['defect-injection'].outcome, 'fake-tests');
  assert.equal(p.evidence['defect-injection'].testsWentRed, false);
  assert.ok(state.inbox.some((e) => /fake\/weak tests/.test(e.reason)));
  assert.equal(reviewCalls, 0, 'reviewer is not asked once fake tests are proven');
  // Restore still left the tree and the ledger intact.
  assert.equal(readFileSync(join(fx.root, 'src', 'sum.js'), 'utf8'), SUM_SRC);
  assert.ok(state.revision > 0);
});

test('P0-2: stale patch = uninjectable -> inbox for a refresh, unit still completes (no fake-test accusation)', async () => {
  // Patch context does not match the file content -> git apply fails.
  const fx = diFixture('stale.patch', `--- a/src/sum.js
+++ b/src/sum.js
@@ -1,3 +1,3 @@
 export function sum(a, b) {
-  return a * b;
+  return a / b;
 }
`);
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const state = loadState(fx.taskDir);
  const p = state.packets['M01-T01'];
  assert.equal(p.status, 'completed');
  assert.equal(p.evidence['defect-injection'].outcome, 'uninjectable');
  assert.ok(state.inbox.some((e) => /uninjectable/.test(e.reason)));
  assert.ok(!state.inbox.some((e) => /fake\/weak tests/.test(e.reason))); // no false accusation
});

test('P0-2: baseline red = inbox (broken suite), unit still completes', async () => {
  const fx = diFixture('defect.patch', `--- a/src/sum.js
+++ b/src/sum.js
@@ -1,3 +1,3 @@
 export function sum(a, b) {
-  return a + b;
+  return a - b;
 }
`);
  // Break the suite itself BEFORE the run (baseline must fail).
  writeFileSync(join(fx.root, 'src', 'sum.js'), 'export function sum(a, b) {\n  return a * b;\n}\n');
  execFileSync('git', ['-C', fx.root, '-c', 'user.name=t', '-c', 'user.email=t@l', 'commit', '-qam', 'break suite']);
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(p.status, 'completed');
  assert.equal(p.evidence['defect-injection'].outcome, 'baseline-red');
  assert.ok(loadState(fx.taskDir).inbox.some((e) => /baseline-red/.test(e.reason)));
});
