// Regression tests for the design/implementation audit fixes (P1/P2, 2026-08-21):
//   P1-1 scoped unit commit (volatile ledger excluded, unit diff only)
//   P1-2 single-instance pid guard
//   P1-3 contract reload after a successful routine plan step
//   P1-4 acceptance expect validated at contract load
//   P2-3 inbox re-run guard (no re-spawn, no duplicate entries) + credentials re-arm
//   P2-4 milestone-scoped human-interventions count
//   P2 config version validation
//   install-skills anchor file

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, appendFileSync, mkdirSync, mkdtempSync } from 'node:fs';

import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { runTask, assertNoRunningOrchestrator, pidFile } from '../orchestrator/core/machine.js';
import { commitUnitChanges } from '../orchestrator/core/verify.js';
import { loadConfig } from '../orchestrator/core/config.js';
import { loadState } from '../orchestrator/core/state.js';
import { loadPackets, loadSpec } from '../orchestrator/core/contracts.js';
import { createMockDriver } from '../orchestrator/drivers/mock.js';
import { installSkills } from '../orchestrator/modules/install-skills.js';
import { makeFixture, makeGitFixture, packetMd, SPEC_MD } from './helpers.js';

const noSleep = async () => {};
const noLog = () => {};

function okResult(extra = {}) {
  return {
    exitCode: 0, signal: null, timedOut: false, idleTimedOut: false, wallTimedOut: false,
    stdout: '', stderr: '', truncated: false, durationMs: 1, spawnError: null,
    harness: 'mock', provider: 'mock', model: 'mock-model',
    usage: { input: 120, output: 60, reasoning: 15, cacheRead: 40, cacheWrite: 10, totalTokens: 180 },
    reasoningChars: 42, costUsd: 0.001, settled: true, text: '', ...extra,
  };
}

function failResult(stderr, exitCode = 1) {
  return okResult({ exitCode, stderr, settled: false });
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

function git(dir, args) {
  return execFileSync('git', ['-C', dir, '-c', 'user.name=test', '-c', 'user.email=test@localhost', ...args], {
    encoding: 'utf8',
  });
}

// ---------------------------------------------------------------------------
// P1-1: scoped unit commit
// ---------------------------------------------------------------------------

test('P1-1 caveat: foreign dirty files warn at run start; task-dir ledger dirt does not', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  // Pre-existing uncommitted WIP outside the task dir — exactly what the
  // scoped unit commit will sweep in.
  appendFileSync(join(fx.root, 'src/app.ts'), '// user wip\n');
  // Task-dir runtime dirt is expected on a resume — must NOT warn.
  writeFileSync(join(fx.taskDir, 'metrics.jsonl'), '{"ts":"x"}\n', 'utf8');

  const warnings = [];
  const result = await runTask({
    taskDir: fx.taskDir, cwd: fx.root, config: loadConfig(fx.taskDir, fx.root),
    drivers: wrapDrivers(), sleep: noSleep, log: (m) => warnings.push(m),
  });
  assert.equal(result.status, 'completed');
  assert.ok(warnings.some((w) => /dirty file\(s\) outside the task dir/.test(w)), `warnings: ${JSON.stringify(warnings)}`);
  assert.ok(warnings.some((w) => w.includes('src/app.ts')), 'names the foreign file');
  assert.ok(!warnings.some((w) => /metrics\.jsonl/.test(w)), 'task-dir ledger dirt does not warn');

  // Clean worktree at start: no warning.
  const fx2 = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const warnings2 = [];
  await runTask({
    taskDir: fx2.taskDir, cwd: fx2.root, config: loadConfig(fx2.taskDir, fx2.root),
    drivers: wrapDrivers(), sleep: noSleep, log: (m) => warnings2.push(m),
  });
  assert.ok(!warnings2.some((w) => /dirty file/.test(w)), `unexpected warnings: ${JSON.stringify(warnings2)}`);
});

test('P1-1: unit commit is scoped — volatile ledger files never enter git history', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  // Runtime artifacts accumulate as UNTRACKED files (flywheel-new generates a
  // task-dir .gitignore for exactly these) — never tracked before the unit.
  writeFileSync(join(fx.taskDir, 'metrics.jsonl'), '{"ts":"x"}\n', 'utf8');
  writeFileSync(join(fx.taskDir, 'orchestrator.log'), '[flywheel] live log\n', 'utf8');
  writeFileSync(join(fx.root, 'user-wip.ts'), '// pre-existing user work\n', 'utf8');
  git(fx.root, ['add', 'user-wip.ts']);
  git(fx.root, ['commit', '-q', '-m', 'chore: user wip committed before the unit']);

  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.ok(p.commit.sha, 'unit commit exists');

  const inCommit = (sha, rel) => {
    try {
      git(fx.root, ['cat-file', '-e', `${sha}:${rel}`]);
      return true;
    } catch {
      return false;
    }
  };
  // Executor work and contract-layer files land in the commit...
  assert.ok(inCommit(p.commit.sha, 'build/M01-T01.txt'), 'executor artifact committed');
  assert.ok(inCommit(p.commit.sha, 'docs/minitask/state.json'), 'state.json committed (contract layer)');
  assert.ok(inCommit(p.commit.sha, 'docs/minitask/reports/M01-T01.done.json'), 'done-report committed (evidence)');
  // ...the volatile runtime files do NOT.
  assert.ok(!inCommit(p.commit.sha, 'docs/minitask/metrics.jsonl'), 'metrics.jsonl excluded from the commit');
  assert.ok(!inCommit(p.commit.sha, 'docs/minitask/orchestrator.log'), 'orchestrator.log excluded from the commit');
  assert.ok(!inCommit(p.commit.sha, 'docs/minitask/orchestrator.pid'), 'orchestrator.pid excluded from the commit');
  // The pre-existing user file is untouched by the unit commit.
  const unitTree = git(fx.root, ['show', '--name-only', '--format=', p.commit.sha]).split('\n').filter(Boolean);
  assert.ok(!unitTree.includes('user-wip.ts'), 'user wip not swept into the unit commit');
});

test('P1-1: commitUnitChanges only stages the unit diff — files unchanged since base are untouched', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'flywheel-scoped-'));
  git(dir, ['init', '-q']);
  writeFileSync(join(dir, 'baseline.ts'), 'export const b = 1;\n');
  git(dir, ['add', '-A']);
  git(dir, ['commit', '-q', '-m', 'baseline']);

  writeFileSync(join(dir, 'feature.ts'), 'export const x = 1;\n');
  writeFileSync(join(dir, 'volatile.log'), 'live\n');
  const committed = await commitUnitChanges(dir, { 'packet-id': 'M01-T01', goal: 'scoped' }, {
    excludePaths: ['volatile.log'],
  });
  assert.ok(committed.sha);
  const names = git(dir, ['show', '--name-only', '--format=', 'HEAD']).split('\n').filter(Boolean);
  assert.deepEqual(names, ['feature.ts']);
  // baseline.ts stayed exactly as committed by the previous commit.
  assert.equal(git(dir, ['status', '--porcelain']).trim(), '?? volatile.log');
});

// ---------------------------------------------------------------------------
// P1-2: single-instance pid guard
// ---------------------------------------------------------------------------

test('P1-2: a live pid file blocks a second orchestrator; a stale one does not', async () => {
  const fx = makeFixture();
  // Live pid: our own test process (kill(pid, 0) succeeds).
  writeFileSync(pidFile(fx.taskDir), JSON.stringify({ pid: process.pid, startedAt: 'x' }), 'utf8');
  assert.throws(() => assertNoRunningOrchestrator(fx.taskDir), /another orchestrator is running/);
  await assert.rejects(() => runTask(ctx(fx, wrapDrivers())), /another orchestrator is running/);

  // Dead pid: stale file is safe to overwrite; the run proceeds and finishes.
  writeFileSync(pidFile(fx.taskDir), JSON.stringify({ pid: 999_999_999, startedAt: 'x' }), 'utf8');
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
});

// ---------------------------------------------------------------------------
// P1-3: routine plan step contract reload
// ---------------------------------------------------------------------------

test('P1-3: packets the planner adds during a routine plan step are executed in the same run', async () => {
  const fx = makeFixture();
  const mock = createMockDriver();
  const drivers = wrapDrivers({
    plan: async (args) => {
      const r = await mock.plan(args);
      // The planner decides milestone M-01 needs one more packet and writes it.
      writeFileSync(join(fx.taskDir, 'task-packet-M01-T03.md'), packetMd('M01-T03', 'R-02'), 'utf8');
      return r;
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  const state = loadState(fx.taskDir);
  const p3 = state.packets['M01-T03'];
  assert.ok(p3, 'state entry auto-created for the planner-added packet');
  assert.equal(p3.status, 'completed', 'the added packet ran to completion in the same run');
  assert.ok(state.packets['M01-T01'].status === 'completed');
  assert.ok(state.packets['M01-T02'].status === 'completed');
});

// ---------------------------------------------------------------------------
// P1-4: acceptance validated at contract load
// ---------------------------------------------------------------------------

test('P1-4: malformed acceptance entries fail at contract load, before any spawn', () => {
  {
    const fx = makeFixture();
    const file = join(fx.taskDir, 'task-packet-M01-T01.md');
    writeFileSync(file, readFileSync(file, 'utf8').replace('expect: "exit 0"', 'expect: "ok"'), 'utf8');
    assert.throws(() => loadPackets(fx.taskDir), /unsupported expect: ok/);
  }
  {
    const fx = makeFixture();
    const file = join(fx.taskDir, 'task-packet-M01-T01.md');
    writeFileSync(file, readFileSync(file, 'utf8').replace('cmd: "test -f build/M01-T01.txt"', 'note: "no command"'), 'utf8');
    assert.throws(() => loadPackets(fx.taskDir), /non-empty cmd or script/);
  }
});

test('P1-4/P2: duplicate requirement ids in the spec fail at load', () => {
  const fx = makeFixture();
  const specFile = join(fx.taskDir, 'spec.md');
  const dup = SPEC_MD.replace('---\n\n# Mini spec', '  - id: R-01\n    title: Duplicate first thing\n    milestone: M-01\n---\n\n# Mini spec');
  writeFileSync(specFile, dup, 'utf8');
  assert.throws(() => loadSpec(fx.taskDir), /duplicate requirement id R-01/);
});

// ---------------------------------------------------------------------------
// P2-3: inbox re-run guard
// ---------------------------------------------------------------------------

test('P2-3: re-running an inbox-escalated task does not re-spawn or duplicate inbox entries', async () => {
  const fx = makeFixture();
  let executeCalls = 0;
  const drivers = wrapDrivers({
    execute: async () => {
      executeCalls += 1;
      return failResult('502 bad gateway');
    },
  });
  const first = await runTask(ctx(fx, drivers));
  assert.equal(first.status, 'escalated');
  const afterFirst = loadState(fx.taskDir);
  assert.equal(afterFirst.packets['M01-T01'].status, 'inbox');
  assert.equal(afterFirst.inbox.length, 1);

  const callsAfterFirst = executeCalls;
  const second = await runTask(ctx(fx, drivers));
  assert.equal(second.status, 'escalated');
  const afterSecond = loadState(fx.taskDir);
  assert.equal(executeCalls, callsAfterFirst, 'no executor spawn on the re-run');
  assert.equal(afterSecond.inbox.length, 1, 'no duplicate inbox entry on the re-run');
});

test('P2-3: an approval file appearing after an inbox timeout re-arms the unit', async () => {
  const fx = makeFixture();
  const file = join(fx.taskDir, 'task-packet-M01-T01.md');
  writeFileSync(file, readFileSync(file, 'utf8').replace('needs-live-credentials: false', 'needs-live-credentials: true'), 'utf8');

  const first = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(first.status, 'escalated');
  const afterFirst = loadState(fx.taskDir);
  assert.equal(afterFirst.packets['M01-T01'].status, 'inbox');
  assert.ok(afterFirst.inbox.some((e) => /timed out/.test(e.reason)));

  // Human fix: write the approval file, then re-run.
  mkdirSync(join(fx.taskDir, 'approvals'), { recursive: true });
  writeFileSync(join(fx.taskDir, 'approvals', 'M01-T01.json'), JSON.stringify({ 'approved-env': [] }), 'utf8');
  const second = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(second.status, 'completed');
  const afterSecond = loadState(fx.taskDir);
  assert.equal(afterSecond.packets['M01-T01'].status, 'completed');
  assert.ok(afterSecond.packets['M01-T01'].evidence.credentials, 'approval recorded as evidence');
});

// ---------------------------------------------------------------------------
// P2-4: milestone-scoped human-interventions
// ---------------------------------------------------------------------------

test('P2-4: retro summary counts interventions for THIS milestone only', async () => {
  const fx = makeFixture();
  // Defect injection samples every unit, patch/test-cmd unconfigured -> one
  // inbox entry per unit (contract defect path, unit still completes).
  const cfg = JSON.parse(readFileSync(join(fx.taskDir, 'flywheel.config.json'), 'utf8'));
  cfg['defect-injection'] = { 'every-n-units': 1 };
  writeFileSync(join(fx.taskDir, 'flywheel.config.json'), JSON.stringify(cfg), 'utf8');
  // Pre-seed inbox noise belonging to OTHER milestones/tasks.
  const state = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
  state.inbox = [
    { at: 'x', 'packet-id': 'M99-T99', reason: 'other milestone' },
    { at: 'x', milestone: 'M-99', reason: 'other milestone gate' },
  ];
  writeFileSync(join(fx.taskDir, 'state.json'), JSON.stringify(state), 'utf8');

  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const retro = readFileSync(join(fx.taskDir, 'retro.md'), 'utf8');
  assert.match(retro, /human-interventions=2/, 'only the two M-01 packet entries count');
});

// ---------------------------------------------------------------------------
// P2: config version validation
// ---------------------------------------------------------------------------

test('P2: config version must be a positive integer when present', () => {
  const fx = makeFixture();
  const cfgFile = join(fx.taskDir, 'flywheel.config.json');
  const cfg = JSON.parse(readFileSync(cfgFile, 'utf8'));
  cfg.version = 'one';
  writeFileSync(cfgFile, JSON.stringify(cfg), 'utf8');
  assert.throws(() => loadConfig(fx.taskDir, fx.root), /version must be a positive integer/);
});

// ---------------------------------------------------------------------------
// install-skills anchor file
// ---------------------------------------------------------------------------

test('install-skills writes the flywheel-home.txt anchor next to the skills', () => {
  const home = mkdtempSync(join(tmpdir(), 'flywheel-anchor-'));
  const root = mkdtempSync(join(tmpdir(), 'flywheel-repo-'));
  mkdirSync(join(root, 'skills', 'demo-skill'), { recursive: true });
  writeFileSync(join(root, 'skills', 'demo-skill', 'SKILL.md'), '---\nname: demo-skill\ndescription: demo\nmetadata:\n  version: "1.0.0"\n---\n\n# demo\n', 'utf8');
  const { targetBase } = installSkills({ homeDir: home, root });
  const anchor = readFileSync(join(targetBase, 'flywheel-home.txt'), 'utf8');
  assert.equal(anchor.trim(), root);
});
