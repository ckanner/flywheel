import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { readFileSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { makeFixture, makeGitFixture } from './helpers.js';

const BIN = join(dirname(fileURLToPath(import.meta.url)), '..', 'bin', 'flywheel.js');

function cli(args, cwd) {
  return new Promise((resolve) => {
    const child = spawn('node', [BIN, ...args], { cwd, stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (d) => { stdout += d; });
    child.stderr.on('data', (d) => { stderr += d; });
    child.on('exit', (exitCode) => resolve({ exitCode, stdout, stderr }));
  });
}

test('e2e (mock drivers): full loop plan -> execute -> L1 -> review -> milestone close', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const run = await cli(['run', `docs/${fx.slug}`], fx.root);
  assert.equal(run.exitCode, 0, run.stderr);
  const summary = JSON.parse(run.stdout.slice(run.stdout.indexOf('{')));
  assert.equal(summary.status, 'completed');
  assert.equal(summary['packets-completed'], '2/2');

  // state.json advanced with all three evidence levels recorded
  const state = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
  assert.equal(state.status, 'completed');
  assert.equal(state.milestones['M-01'].status, 'closed');
  assert.ok(state.milestones['M-01'].gate.pass);
  assert.equal(state.milestones['M-01'].gate.judge.verdict, 'pass'); // §7 终验证据
  assert.ok(state.revision > 0);
  assert.ok(state['last-progress-at']);
  for (const id of ['M01-T01', 'M01-T02']) {
    const p = state.packets[id];
    assert.equal(p.status, 'completed');
    assert.ok(p.evidence.executor, `${id} executor evidence`);
    assert.equal(p.evidence.l1.pass, true, `${id} L1 evidence`);
    assert.equal(p.evidence.review.verdict, 'pass', `${id} review evidence`);
    assert.equal(p.exit.timedOut, false);
  }

  // metrics.jsonl: one record per spawn (1 plan + 2 execute + 2 review + 1 judge)
  const metricsFile = join(fx.taskDir, 'metrics.jsonl');
  assert.ok(existsSync(metricsFile));
  const entries = readFileSync(metricsFile, 'utf8').trim().split('\n').map((l) => JSON.parse(l));
  assert.equal(entries.length, 6);
  assert.deepEqual(
    entries.map((e) => e.role).sort(),
    ['execute', 'execute', 'judge', 'plan', 'review', 'review'],
  );
  for (const e of entries) {
    assert.ok(e['prompt-hash']);
    assert.ok(e.usage.input > 0);
    assert.equal(typeof e['duration-ms'], 'number');
    assert.equal(e['exit-code'], 0);
  }

  // budget accumulation + retro record (§5.3 milestone summary fields)
  assert.ok(state.budget.totals.tokens > 0);
  const retro = readFileSync(join(fx.taskDir, 'retro.md'), 'utf8');
  assert.match(retro, /milestone M-01: closed/);
  assert.match(retro, /summary: rounds=\d+, defects\(review-failed\)=\d+, reopens=\d+, scope-changes=\d+/);
  assert.match(retro, /budget=\+\d+ tokens \/ \$[\d.]+/);
  assert.match(retro, /human-interventions=\d+/);
  assert.match(retro, /improvements/);

  // orchestrator-owned unit commits (§4.2): one per completed unit, packet-id in message
  const gitLog = execFileSync('git', ['-C', fx.root, 'log', '--format=%s'], { encoding: 'utf8' });
  for (const id of ['M01-T01', 'M01-T02']) {
    const p = state.packets[id];
    assert.ok(p['base-ref'], `${id} base-ref recorded`);
    assert.ok(p.commit?.sha, `${id} orchestrator commit recorded in state`);
    assert.match(p.commit.message, new RegExp(`feat\\(M-01\\): ${id}`));
    assert.match(gitLog, new RegExp(`feat\\(M-01\\): ${id}`));
  }

  // status command renders the final picture
  const status = await cli(['status', `docs/${fx.slug}`], fx.root);
  assert.equal(status.exitCode, 0, status.stderr);
  assert.match(status.stdout, /completed/);
  assert.match(status.stdout, /M01-T01/);
  assert.match(status.stdout, /缓存命中率/);
});

test('e2e (mock drivers): --only execute --packet performs exactly one step', async () => {
  const fx = makeFixture();
  const run = await cli(['run', `docs/${fx.slug}`, '--only', 'execute', '--packet', 'M01-T01'], fx.root);
  assert.equal(run.exitCode, 0, run.stderr);
  const state = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
  assert.equal(state.packets['M01-T01'].status, 'executed'); // executed, no L1/review yet
  assert.equal(state.packets['M01-T01'].evidence.l1, null);
  assert.equal(state.packets['M01-T02'].status, 'pending');
});

test('e2e (mock drivers): --only plan performs exactly one plan step and reports it', async () => {
  const fx = makeFixture();
  const run = await cli(['run', `docs/${fx.slug}`, '--only', 'plan'], fx.root);
  assert.equal(run.exitCode, 0, run.stderr);
  const summary = JSON.parse(run.stdout.slice(run.stdout.indexOf('{')));
  assert.equal(summary.step.name, 'plan');
  assert.equal(summary.step.ok, true);
  const state = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
  assert.equal(state.packets['M01-T01'].status, 'pending'); // nothing executed
});

test('e2e (mock drivers): a failed single step exits non-zero (step result in summary)', async () => {
  const fx = makeFixture();
  // L1 cannot pass on a fresh fixture (no build artifact) -> verify step fails.
  const run = await cli(['run', `docs/${fx.slug}`, '--only', 'verify', '--packet', 'M01-T01'], fx.root);
  assert.equal(run.exitCode, 3, run.stdout + run.stderr);
  const summary = JSON.parse(run.stdout.slice(run.stdout.indexOf('{')));
  assert.equal(summary.step.name, 'verify');
  assert.equal(summary.step.ok, false);
});

test('e2e (mock drivers): stop on a non-running task freezes state', async () => {
  const fx = makeFixture();
  const stop = await cli(['stop', `docs/${fx.slug}`], fx.root);
  assert.equal(stop.exitCode, 0, stop.stderr);
  const state = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
  assert.equal(state.status, 'stopped');
});

test('e2e (mock drivers): manual single-step chain execute -> verify -> review', async () => {
  const fx = makeFixture();

  // review before any passing L1 evidence is rejected
  const early = await cli(['run', `docs/${fx.slug}`, '--only', 'review', '--unit', 'M01-T01'], fx.root);
  assert.equal(early.exitCode, 1);
  assert.match(early.stderr, /no PASSING L1 evidence/);

  await cli(['run', `docs/${fx.slug}`, '--only', 'execute', '--packet', 'M01-T01'], fx.root);
  const verify = await cli(['run', `docs/${fx.slug}`, '--only', 'verify', '--packet', 'M01-T01'], fx.root);
  assert.equal(verify.exitCode, 0, verify.stderr);
  let state = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
  assert.equal(state.packets['M01-T01'].status, 'l1-passed');

  const review = await cli(['run', `docs/${fx.slug}`, '--only', 'review', '--unit', 'M01-T01'], fx.root);
  assert.equal(review.exitCode, 0, review.stderr);
  state = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
  assert.equal(state.packets['M01-T01'].evidence.review.verdict, 'pass');
});
