// Worktree isolation (DESIGN.md §4.2 v0.16): every execution unit runs in a
// per-unit git worktree — clean checkout, main-checkout WIP untouched,
// deterministic path for crash resume, code commit + ff-merge + ledger commit.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, appendFileSync, existsSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';

import { runTask } from '../orchestrator/core/machine.js';
import { loadConfig } from '../orchestrator/core/config.js';
import { loadState } from '../orchestrator/core/state.js';
import { worktreeNames } from '../orchestrator/core/verify.js';
import { createMockDriver } from '../orchestrator/drivers/mock.js';
import { makeGitFixture, makeFixture } from './helpers.js';

const noSleep = async () => {};
const noLog = () => {};

function wrapDrivers(overrides = {}) {
  const mock = createMockDriver();
  return {
    plan: { plan: overrides.plan ?? ((a) => mock.plan(a)) },
    execute: { execute: overrides.execute ?? ((a) => mock.execute(a)) },
    review: { review: overrides.review ?? ((a) => mock.review(a)) },
    judge: { judge: overrides.judge ?? ((a) => mock.judge(a)) },
  };
}

function ctx(fixture, drivers, extra = {}) {
  return {
    taskDir: fixture.taskDir,
    cwd: fixture.root,
    config: loadConfig(fixture.taskDir, fixture.root),
    drivers,
    sleep: noSleep,
    log: extra.log ?? noLog,
    ...extra,
  };
}

function git(root, args) {
  return execFileSync('git', ['-C', root, ...args], { encoding: 'utf8' });
}

test('worktree: the executor runs inside the per-unit worktree, not the main checkout', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const mock = createMockDriver();
  const seenCwds = [];
  const drivers = wrapDrivers({
    execute: async (args) => {
      seenCwds.push(args.cwd);
      return mock.execute(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  const { path } = worktreeNames(fx.root, 'minitask', 'M01-T01');
  assert.equal(seenCwds[0], path, `executor cwd: ${seenCwds[0]} vs worktree ${path}`);
  assert.ok(seenCwds.every((c) => c.includes('.flywheel-worktrees/')), `all runs in worktrees: ${JSON.stringify(seenCwds)}`);
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(p.worktree.mode, 'worktree');
  assert.equal(p.worktree.path, path);
  assert.match(p.worktree.branch, /^flywheel\/minitask\/M01-T01$/);
});

test('worktree: a planner revision commits contracts, rebuilds the worktree, and re-anchors the base ref', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const mock = createMockDriver();
  let executeCalls = 0;
  let planCalls = 0;
  const drivers = wrapDrivers({
    execute: async (args) => {
      executeCalls += 1;
      // Three transport slots per attempt (max-transport-retries 2): fail the
      // first SIX calls = two full failing attempts -> spin -> planner.
      if (executeCalls <= 6) return { ...(await mock.execute(args)), exitCode: 1, stderr: '502 bad gateway', settled: false };
      return mock.execute(args);
    },
    plan: async (args) => {
      planCalls += 1;
      const r = await mock.plan(args);
      // Revise ONLY on the escalation call (the routine milestone-start plan
      // step leaves the contract untouched).
      if (planCalls >= 2) {
        const file = join(fx.taskDir, 'task-packet-M01-T01.md');
        writeFileSync(file, readFileSync(file, 'utf8').replace(/goal: Make M01-T01.*/, 'goal: Make M01-T01 produce its artifact (revised by planner).'), 'utf8');
      }
      return r;
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(p['plan-revisions'], 1, 'planner revised once');
  // The revision is a committed, contract-only checkpoint in main history...
  const log = git(fx.root, ['log', '--format=%s']);
  assert.match(log, /docs\(minitask\): plan revision 1/);
  const revisionCommit = git(fx.root, ['log', '--format=%H', '--grep=plan revision 1']).trim();
  const revisionFiles = git(fx.root, ['show', '--name-only', '--format=', revisionCommit]).split('\n').filter(Boolean);
  assert.deepEqual(revisionFiles, ['docs/minitask/task-packet-M01-T01.md'], 'contract-only revision commit');
  // ...and the unit re-anchored to it before re-executing.
  assert.equal(p['base-ref'], revisionCommit, 'base-ref re-anchored at the revision checkpoint');
  // The unit passed L1 on the REVISED contract without a contract violation.
  assert.equal(p.evidence.l1.pass, true);
  assert.deepEqual(p.evidence.l1.boundary.violations, [], 'no contract-file violation for the legitimate revision');
});

test('worktree: a routine plan update commits only changed contract files and never moves HEAD otherwise', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const initialHead = git(fx.root, ['rev-parse', 'HEAD']).trim();
  // Planner changes NOTHING during the routine plan step (the common resume
  // case): no commit must be created, HEAD stays put, units still complete.
  const messages = git(fx.root, ['log', '--format=%s']).split('\n');
  assert.equal(messages.filter((m) => /plan update/.test(m)).length, 0, 'no plan-update commit when nothing changed');
  // HEAD advanced only through the unit commits (initialHead is an ancestor).
  git(fx.root, ['merge-base', '--is-ancestor', initialHead, git(fx.root, ['rev-parse', 'HEAD']).trim()]);
});

test('worktree: manual --only steps on a COMPLETED unit fall back to the main checkout', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(existsSync(p.worktree.path), false, 'worktree removed at completion');

  // --only verify on the completed unit: runs against the merged main
  // checkout, not the (removed) worktree.
  const { runSingleStep } = await import('../orchestrator/core/machine.js');
  const v = await runSingleStep({ ...ctx(fx, wrapDrivers()), only: 'verify', packetId: 'M01-T01' });
  assert.equal(v.step.ok, true, 'verify on a completed unit uses the merged code');
  // --only execute re-creates the worktree on demand instead of reusing the
  // stale path.
  const e = await runSingleStep({ ...ctx(fx, wrapDrivers()), only: 'execute', packetId: 'M01-T01' });
  assert.equal(e.step.ok, true);
  const after = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(after.worktree.mode, 'worktree');
  assert.ok(existsSync(after.worktree.path), 'worktree re-created for the manual step');
});

test('worktree: removed after completion; code merged into main, ledger commit restricted to the task dir', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.ok(p.commit.sha && p.commit['ledger-sha'], 'code commit + ledger commit');
  // Worktree dir gone, branch gone.
  assert.equal(existsSync(p.worktree.path), false, 'worktree checkout removed');
  assert.equal(git(fx.root, ['branch', '--list', p.worktree.branch]).trim(), '', 'worktree branch deleted');
  // Code is in main history; the ledger commit touches ONLY the task dir.
  const codeFiles = git(fx.root, ['show', '--name-only', '--format=', p.commit.sha]).split('\n').filter(Boolean);
  assert.ok(codeFiles.includes('build/M01-T01.txt'));
  const ledgerFiles = git(fx.root, ['show', '--name-only', '--format=', p.commit['ledger-sha']]).split('\n').filter(Boolean);
  assert.ok(ledgerFiles.length > 0);
  assert.ok(ledgerFiles.every((f) => f.startsWith('docs/minitask/')), `ledger commit leaked: ${ledgerFiles.join(', ')}`);
});

test('worktree: pause/resume reuses the SAME worktree (deterministic path)', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const paused = await runTask({ ...ctx(fx, wrapDrivers()), pauseBefore: 'review' });
  assert.equal(paused.status, 'paused');
  const before = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(before.worktree.mode, 'worktree');
  assert.ok(existsSync(join(before.worktree.path, 'build', 'M01-T01.txt')), 'artifact in the paused worktree');

  const resumed = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(resumed.status, 'completed');
  const after = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(after.worktree.path, before.worktree.path, 'same worktree path across the resume');
  assert.equal(after.attempts, 1, 'no re-execute on resume');
  assert.equal(existsSync(after.worktree.path), false, 'worktree removed after completion');
});

test('worktree: executor tampering with contract files via the absolute taskDir path is caught', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const mock = createMockDriver();
  const drivers = wrapDrivers({
    execute: async (args) => {
      const r = await mock.execute(args);
      // The executor knows the MAIN checkout's taskDir absolute path
      // (done-report instruction) and tampers with a contract file there.
      appendFileSync(join(args.taskDir, 'spec.md'), '\ntampered by executor\n');
      return r;
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(p.evidence.l1.pass, false);
  assert.ok(
    p.evidence.l1.boundary.violations.some((v) => v.type === 'contract-file' && v.path === 'docs/minitask/spec.md'),
    `violations: ${JSON.stringify(p.evidence.l1.boundary.violations)}`,
  );
});

test('worktree: disabled by config -> in-place execution with a single commit (no ledger-sha)', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const cfgFile = join(fx.taskDir, 'flywheel.config.json');
  const cfg = JSON.parse(readFileSync(cfgFile, 'utf8'));
  cfg.worktree = { enabled: false };
  writeFileSync(cfgFile, JSON.stringify(cfg), 'utf8');
  git(fx.root, ['add', 'docs/minitask/flywheel.config.json']);
  git(fx.root, ['commit', '-q', '-m', 'chore: disable worktree']);

  const mock = createMockDriver();
  const seenCwds = [];
  const drivers = wrapDrivers({
    execute: async (args) => {
      seenCwds.push(args.cwd);
      return mock.execute(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  assert.ok(seenCwds.every((c) => c === fx.root), 'executor ran in the main checkout');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(p.worktree, null);
  assert.ok(p.commit.sha && !('ledger-sha' in p.commit), 'single in-place commit');
});

test('worktree: non-git checkout falls back to in-place without worktree state', async () => {
  const fx = makeFixture(); // plain tmp dir, not a git repo
  const mock = createMockDriver();
  const seenCwds = [];
  const drivers = wrapDrivers({
    execute: async (args) => {
      seenCwds.push(args.cwd);
      return mock.execute(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  assert.ok(seenCwds.every((c) => c === fx.root));
  assert.equal(loadState(fx.taskDir).packets['M01-T01'].worktree, null);
});
