import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, existsSync, appendFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { runTask } from '../orchestrator/core/machine.js';
import { loadConfig } from '../orchestrator/core/config.js';
import { loadState } from '../orchestrator/core/state.js';
import { createMockDriver } from '../orchestrator/drivers/mock.js';
import { makeFixture, makeGitFixture } from './helpers.js';

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

test('machine: happy path runs plan -> execute -> L1 -> review -> milestone close', async () => {
  const fx = makeFixture();
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const state = loadState(fx.taskDir);
  assert.equal(state.milestones['M-01'].status, 'closed');
  for (const id of ['M01-T01', 'M01-T02']) {
    const p = state.packets[id];
    assert.equal(p.status, 'completed');
    // Three evidence levels recorded with different strength (§5.2)
    assert.ok(p.evidence.executor, 'executor self-report');
    assert.equal(p.evidence.l1.pass, true, 'L1 deterministic');
    assert.equal(p.evidence.review.verdict, 'pass', 'Review verdict');
    assert.deepEqual(p.exit, { exitCode: 0, signal: null, timedOut: false, idleTimedOut: false, wallTimedOut: false });
  }
  assert.ok(state['last-progress-at']);
  assert.ok(state.revision > 0);
  assert.ok(state.budget.totals.tokens > 0); // recording always on
});

test('machine: transport failures retry with backoff inside one attempt, then succeed', async () => {
  const fx = makeFixture();
  const mock = createMockDriver();
  let calls = 0;
  const drivers = wrapDrivers({
    execute: async (args) => {
      calls += 1;
      if (args.packet['packet-id'] === 'M01-T01' && calls <= 2) return failResult('502 bad gateway');
      return mock.execute(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(p.attempts, 1); // transport retries stay inside the attempt
  assert.equal(p['failed-attempts'], 0);
  const backoffs = p['retry-history'].filter((r) => r.scope === 'transport' && r.policy === 'backoff');
  assert.equal(backoffs.length, 2);
  assert.equal(backoffs[0]['error-code'], 'SERVER');
  assert.ok(backoffs.every((r) => r['planned-delay-ms'] >= 0)); // auditable
});

test('machine: empty deliverable (exit 0, nothing written) = EMPTY_RESPONSE, retried', async () => {
  const fx = makeFixture();
  let calls = 0;
  const drivers = wrapDrivers({
    execute: async (args) => {
      calls += 1;
      if (calls <= 1) return okResult(); // exit 0 but writes nothing
      return createMockDriver().execute(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.ok(p['retry-history'].some((r) => r['error-code'] === 'EMPTY_RESPONSE' && r.policy === 'backoff'));
});

test('machine: retries exhausted -> return-to-planner -> still failing -> inbox', async () => {
  const fx = makeFixture();
  const drivers = wrapDrivers({ execute: async () => failResult('502 bad gateway') });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated');
  const state = loadState(fx.taskDir);
  const p = state.packets['M01-T01'];
  assert.equal(p['plan-revisions'], 1); // Plan Loop got one revision round
  assert.equal(p.status, 'inbox');
  assert.equal(state.inbox.length, 1);
  assert.match(state.inbox[0].reason, /M01-T01/);
  assert.ok(existsSync(join(fx.taskDir, 'inbox.md')));
  // T02 never started: serial loop stopped at the escalation
  assert.equal(state.packets['M01-T02'].status, 'pending');
});

test('machine: review "return" bounces the unit back to execution', async () => {
  const fx = makeFixture();
  const mock = createMockDriver();
  let reviewCalls = 0;
  const drivers = wrapDrivers({
    review: async (args) => {
      reviewCalls += 1;
      if (reviewCalls === 1) {
        return okResult({ text: JSON.stringify({ verdict: 'return', score: 0.4, reasons: ['weak assertions in tests'] }) });
      }
      return mock.review(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(p.attempts, 2); // one bounce
  assert.equal(p['failed-attempts'], 1);
  assert.ok(p['retry-history'].some((r) => r['error-code'] === 'REVIEW_RETURNED'));
});

test('machine: verification-gap returns to Plan, reloads the revised contract, re-verifies (DC2)', async () => {
  const fx = makeFixture();
  let executeCalls = 0;
  let reviewCalls = 0;
  const mock = createMockDriver();
  const drivers = wrapDrivers({
    execute: async (args) => { executeCalls += 1; return mock.execute(args); },
    review: async () => {
      reviewCalls += 1;
      if (reviewCalls === 1) {
        return okResult({ text: JSON.stringify({ verdict: 'verification-gap', score: 0.9, reasons: ['acceptance too weak'] }) });
      }
      return okResult({ text: JSON.stringify({ verdict: 'pass', score: 1, reasons: [] }) });
    },
    plan: async (args) => {
      // Planner strengthens the acceptance on disk (only on revision rounds).
      const file = join(fx.taskDir, 'task-packet-M01-T01.md');
      const content = readFileSync(file, 'utf8');
      if (!content.includes('test -s build/M01-T01.txt')) {
        writeFileSync(file, content.replace('test -f build/M01-T01.txt', 'test -s build/M01-T01.txt'));
      }
      return mock.plan(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(executeCalls, 2); // T01 once + T02 once — a gap NEVER bounces back to execution
  assert.equal(p['plan-revisions'], 1); // revision budget consumed
  // P0-2: the reloaded (strengthened) acceptance was used for re-verification.
  assert.ok(p.evidence.l1.commands.some((c) => c.cmd === 'test -s build/M01-T01.txt'));
  assert.equal(p.status, 'completed');
});

test('machine: verification-gap is NEVER completed when the planner cannot fix it (DC2)', async () => {
  const fx = makeFixture();
  const drivers = wrapDrivers({
    review: async () => okResult({ text: JSON.stringify({ verdict: 'verification-gap', score: 0.9, reasons: ['still too weak'] }) }),
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.notEqual(p.status, 'completed'); // never completed on a gap
  assert.equal(p.status, 'inbox'); // revision budget exhausted -> human
  assert.equal(p['plan-revisions'], 1);
});

test('machine: spin detection escalates before retries are exhausted', async () => {
  const fx = makeFixture();
  // Make L1 fail identically every attempt.
  writeFileSync(
    join(fx.taskDir, 'task-packet-M01-T01.md'),
    readFileSync(join(fx.taskDir, 'task-packet-M01-T01.md'), 'utf8').replace('test -f build/M01-T01.txt', 'false'),
  );
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'escalated');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  // 2 identical failures -> spin -> planner revision -> 2 more -> spin -> inbox.
  // max-retries is 3; escalation happened at 2 consecutive identical failures.
  assert.equal(p.attempts, 4);
  assert.equal(p['failed-attempts'], 2);
  assert.equal(p['plan-revisions'], 1);
  assert.equal(p.status, 'inbox');
});

test('machine: --pause-before review freezes with L1 evidence and resumes AT REVIEW (no re-execute)', async () => {
  const fx = makeFixture();
  const paused = await runTask({ ...ctx(fx, wrapDrivers()), pauseBefore: 'review' });
  assert.equal(paused.status, 'paused');
  let state = loadState(fx.taskDir);
  const p = state.packets['M01-T01'];
  assert.equal(p.status, 'l1-passed');
  assert.equal(p.evidence.l1.pass, true);
  assert.equal(p.evidence.review, null); // reviewer has not spoken yet

  const resumed = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(resumed.status, 'completed');
  state = loadState(fx.taskDir);
  const after = state.packets['M01-T01'];
  assert.equal(after.evidence.review.verdict, 'pass');
  assert.equal(after.attempts, 1); // #6: resume did NOT re-execute
  assert.equal(after.evidence.executor['done-report'], 'reports/M01-T01.done.json'); // report intact
});

test('machine: base-ref is sampled on the first attempt only and reused across retries', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const initialHead = execFileSync('git', ['-C', fx.root, 'rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
  const mock = createMockDriver();
  let reviewCalls = 0;
  const drivers = wrapDrivers({
    // One review bounce -> two attempts; the executor's first attempt gets
    // committed by... nothing here (mock doesn't commit), but completeUnit of
    // OTHER units would move HEAD. Force a bounce and check the anchor.
    review: async (args) => {
      reviewCalls += 1;
      if (reviewCalls === 1) return okResult({ text: JSON.stringify({ verdict: 'return', score: 0.4, reasons: ['bounce'] }) });
      return mock.review(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(p.attempts, 2);
  assert.equal(p['base-ref'], initialHead); // #5: retries keep the FIRST base-ref
});

test('machine: executor tampering with contract files fails the unit (contract-file violation)', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const mock = createMockDriver();
  const drivers = wrapDrivers({
    execute: async (args) => {
      const r = await mock.execute(args);
      // Executor edits the contract layer itself.
      appendFileSync(join(fx.taskDir, 'spec.md'), '\ntampered by executor\n');
      return r;
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.notEqual(p.status, 'completed');
  assert.equal(p.evidence.l1.pass, false);
  assert.ok(
    p.evidence.l1.boundary.violations.some((v) => v.type === 'contract-file' && v.path.endsWith('spec.md')),
    `violations: ${JSON.stringify(p.evidence.l1.boundary.violations)}`,
  );
});

test('machine: ps.exit records the orthogonal failure facts on non-happy paths', async () => {
  const fx = makeFixture();
  let calls = 0;
  const mock = createMockDriver();
  const drivers = wrapDrivers({
    execute: async (args) => {
      calls += 1;
      if (args.packet['packet-id'] === 'M01-T01' && calls === 1) {
        return { ...okResult(), exitCode: null, signal: 'SIGKILL', timedOut: true, idleTimedOut: true, wallTimedOut: false };
      }
      return mock.execute(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed'); // TIMEOUT is retryable -> second attempt succeeds
  const p = loadState(fx.taskDir).packets['M01-T01'];
  // The last recorded exit is the successful one; the retry-history carries
  // the TIMEOUT. Both facts live in state.json (§6 orthogonal recording).
  assert.deepEqual(p.exit, { exitCode: 0, signal: null, timedOut: false, idleTimedOut: false, wallTimedOut: false });
  assert.ok(p['retry-history'].some((r) => r['error-code'] === 'TIMEOUT'));
});

test('machine: no spin when the changed-file sets differ (DC4 Jaccard < 0.7)', async () => {
  const fx = makeFixture();
  // L1 always fails; each attempt changes DIFFERENT files -> no spin -> the
  // full retry budget is consumed before escalation.
  writeFileSync(
    join(fx.taskDir, 'task-packet-M01-T01.md'),
    readFileSync(join(fx.taskDir, 'task-packet-M01-T01.md'), 'utf8').replace('test -f build/M01-T01.txt', 'false'),
  );
  const mock = createMockDriver();
  let n = 0;
  const drivers = wrapDrivers({
    execute: async (args) => {
      if (args.packet['packet-id'] !== 'M01-T01') return mock.execute(args);
      n += 1;
      const r = await mock.execute(args);
      // Rewrite the done-report with a DIFFERENT file set each attempt.
      writeFileSync(
        join(args.taskDir, 'reports', 'M01-T01.done.json'),
        JSON.stringify({ 'requirements-covered': ['R-01'], 'changed-files': [`build/v${n}.ts`], 'commands-run': [], failures: [] }),
      );
      return r;
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  // max-retries=3, no early spin: 3 attempts -> planner revision -> 3 more -> inbox.
  assert.equal(p.attempts, 6);
  assert.equal(p['plan-revisions'], 1);
  assert.equal(p.status, 'inbox');
});

test('machine: defect injection is always on — sampled without patch/test-cmd goes to inbox but the unit still completes', async () => {
  const fx = makeFixture();
  // Sample every unit, with NO patch/test-cmd configured (contract defect).
  const cfg = JSON.parse(readFileSync(join(fx.taskDir, 'flywheel.config.json'), 'utf8'));
  cfg['defect-injection'] = { 'every-n-units': 1, 'test-file-ratio-boost': 0.5 };
  writeFileSync(join(fx.taskDir, 'flywheel.config.json'), JSON.stringify(cfg));
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed'); // never blocks the unit
  const state = loadState(fx.taskDir);
  const p = state.packets['M01-T01'];
  assert.equal(p.status, 'completed');
  assert.equal(p.evidence['defect-injection'].skipped, 'missing-config');
  // No silent skip: a human is asked to configure patch/test-cmd for the task.
  assert.ok(state.inbox.some((e) => /defect-injection\.patch\/test-cmd/.test(e.reason) && e['packet-id'] === 'M01-T01'));
  assert.match(readFileSync(join(fx.taskDir, 'inbox.md'), 'utf8'), /defect-injection\.patch\/test-cmd/);
});

test('machine: cache hit-rate below the per-channel threshold raises a soft warning (§8-7)', async () => {  const fx = makeFixture();
  const mock = createMockDriver();
  const drivers = wrapDrivers({
    execute: async (args) => {
      const r = await mock.execute(args);
      // Zero cache reads at meaningful volume -> hit-rate 0 < 0.3.
      r.usage = { input: 2000, output: 60, reasoning: 0, cacheRead: 0, cacheWrite: 10, totalTokens: 2060 };
      return r;
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed'); // soft semantics: never stops
  const state = loadState(fx.taskDir);
  assert.ok(state.budget.warnings.some((w) => /cache hit-rate/.test(w)), JSON.stringify(state.budget.warnings));
});

test('machine: restart marks in-flight unit interrupted, preserves artifacts, then completes it', async () => {
  const fx = makeFixture();
  // Simulate a crash: T01 was mid-flight when the orchestrator died.
  const crashed = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
  crashed.status = 'stopped';
  crashed.packets['M01-T01'].status = 'executing';
  crashed.packets['M01-T01'].artifacts = ['partial-output.tmp'];
  writeFileSync(join(fx.taskDir, 'state.json'), JSON.stringify(crashed));

  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(p.status, 'completed');
  assert.equal(p.interrupted, false); // cleared after successful re-run
  assert.ok(p.artifacts.includes('partial-output.tmp')); // preserved, not wiped
  assert.equal(p['failed-attempts'], 0); // interruption is not a failure
});

test('machine: budget enforce=hard stops the run but keeps recording', async () => {
  const fx = makeFixture();
  const cfg = JSON.parse(readFileSync(join(fx.taskDir, 'flywheel.config.json'), 'utf8'));
  cfg.budget = { enforce: 'hard', 'daily-token-limit': 100 };
  writeFileSync(join(fx.taskDir, 'flywheel.config.json'), JSON.stringify(cfg));
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'budget-stopped');
  const state = loadState(fx.taskDir);
  assert.ok(state.budget.totals.tokens > 0); // recording never turns off (§2 rule 9)
});

test('machine: budget enforce=soft warns and continues', async () => {
  const fx = makeFixture();
  const cfg = JSON.parse(readFileSync(join(fx.taskDir, 'flywheel.config.json'), 'utf8'));
  cfg.budget = { enforce: 'soft', 'daily-token-limit': 100 };
  writeFileSync(join(fx.taskDir, 'flywheel.config.json'), JSON.stringify(cfg));
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const state = loadState(fx.taskDir);
  assert.ok(state.budget.warnings.length > 0);
});

test('machine: 403/quota falls back to the next configured channel', async () => {
  const fx = makeFixture();
  const mock = createMockDriver();
  const seenChannels = [];
  const drivers = wrapDrivers({
    execute: async (args) => {
      seenChannels.push(args.channel);
      if (args.channel === 'opencode-go') return failResult('403: {"type":"QuotaError","message":"fair use limit"}');
      return mock.execute(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  assert.deepEqual(seenChannels.slice(0, 2), ['opencode-go', 'opencode']);
  const p = loadState(fx.taskDir).packets['M01-T01'];
  const fb = p['retry-history'].find((r) => r.scope === 'fallback');
  assert.ok(fb);
  assert.equal(fb['error-code'], 'QUOTA');
  assert.match(fb.policy, /opencode-go -> opencode/);
});

test('machine: 401/AUTH (invalid key) does NOT fall back — straight to attempt failure', async () => {
  const fx = makeFixture();
  const seenChannels = [];
  const drivers = wrapDrivers({
    execute: async (args) => {
      seenChannels.push(args.channel);
      return failResult('401: {"type":"AuthError","message":"Invalid API key."}');
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated');
  // Only the primary channel was ever tried — a bad key follows the
  // escalation path, not the fallback chain.
  assert.deepEqual([...new Set(seenChannels)], ['opencode-go']);
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(p['retry-history'].some((r) => r.scope === 'fallback'), false);
  assert.ok(p['retry-history'].some((r) => r['error-code'] === 'AUTH' && r.policy === 'no-retry'));
});

test('machine: milestone gate (L2/L3) failure escalates instead of closing', async () => {
  const fx = makeFixture();
  // Add a failing L2 command to T01's acceptance.
  const file = join(fx.taskDir, 'task-packet-M01-T01.md');
  writeFileSync(file, readFileSync(file, 'utf8').replace(
    '    - {cmd: "test -f build/M01-T01.txt", expect: "exit 0"}',
    '    - {cmd: "test -f build/M01-T01.txt", expect: "exit 0"}\n  L2:\n    - {cmd: "false", expect: "exit 0"}',
  ));
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'escalated');
  const state = loadState(fx.taskDir);
  assert.equal(state.packets['M01-T01'].status, 'completed'); // units finished
  assert.equal(state.milestones['M-01'].status, 'in-progress'); // but not closed
  assert.equal(state.milestones['M-01'].gate.pass, false);
  assert.equal(state.inbox.length, 1);
  assert.match(state.inbox[0].reason, /gate/);
});

test('machine: commit is skipped with a recorded reason outside git checkouts', async () => {
  const fx = makeFixture(); // plain tmp dir, not a git repo
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.equal(p.commit.skipped, 'not a git checkout');
  assert.equal(p.commit.sha, null);
  assert.equal(p['base-ref'], null);
});

test('machine: orchestrator commits each completed unit (§4.2) and records base-ref', async () => {
  const fx = makeGitFixture({ extraFiles: ['src/app.ts'] });
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const state = loadState(fx.taskDir);
  const git = (args) => execFileSync('git', ['-C', fx.root, ...args], { encoding: 'utf8' });
  const log = git(['log', '--format=%s']);
  for (const id of ['M01-T01', 'M01-T02']) {
    const p = state.packets[id];
    assert.ok(p['base-ref'], `${id} base-ref recorded before spawn`);
    assert.ok(p.commit.sha, `${id} has orchestrator commit`);
    assert.match(p.commit.message, new RegExp(`feat\\(M-01\\): ${id}`));
    assert.match(log, new RegExp(`feat\\(M-01\\): ${id}`));
  }
  // Unit granularity: T01's commit is the parent of T02's commit.
  const t01 = state.packets['M01-T01'].commit.sha;
  const t02 = state.packets['M01-T02'].commit.sha;
  assert.equal(git(['rev-parse', `${t02}^`]).trim(), t01);
});

test('machine: executor self-committing test-file changes cannot bypass boundary judgement', async () => {
  // Regression: the boundary check used to diff against HEAD — an executor
  // committing mid-unit drained the diff and smuggled test edits through.
  const fx = makeGitFixture({ extraFiles: ['src/app.ts', 'tests/existing.test.ts'] });
  const mock = createMockDriver();
  const drivers = wrapDrivers({
    execute: async (args) => {
      const r = await mock.execute(args);
      // Executor weakens an existing TEST FILE and commits it itself.
      appendFileSync(join(fx.root, 'tests/existing.test.ts'), '// weakened assertion\n');
      execFileSync('git', ['-C', fx.root, '-c', 'user.name=exec', '-c', 'user.email=exec@x', 'add', '-A']);
      execFileSync('git', ['-C', fx.root, '-c', 'user.name=exec', '-c', 'user.email=exec@x', 'commit', '-q', '-m', 'wip']);
      return r;
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated'); // spins on the repeated violation -> planner -> inbox
  const state = loadState(fx.taskDir);
  const p = state.packets['M01-T01'];
  assert.notEqual(p.status, 'completed');
  assert.equal(p.evidence.l1.pass, false);
  assert.ok(
    p.evidence.l1.boundary.violations.some((v) => v.type === 'test-file' && v.path === 'tests/existing.test.ts'),
    `violations: ${JSON.stringify(p.evidence.l1.boundary.violations)}`,
  );
});
