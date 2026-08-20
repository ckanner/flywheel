import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { runTask } from '../orchestrator/core/machine.js';
import { loadConfig } from '../orchestrator/core/config.js';
import { loadState } from '../orchestrator/core/state.js';
import { createMockDriver } from '../orchestrator/drivers/mock.js';
import { makeFixture } from './helpers.js';

const noSleep = async () => {};
const noLog = () => {};
const realSleep = (ms) => new Promise((r) => setTimeout(r, ms));

function okResult(extra = {}) {
  return {
    exitCode: 0, signal: null, timedOut: false, idleTimedOut: false, wallTimedOut: false,
    stdout: '', stderr: '', truncated: false, durationMs: 1, spawnError: null,
    harness: 'mock', provider: 'mock', model: 'mock-model',
    usage: { input: 120, output: 60, reasoning: 15, cacheRead: 40, cacheWrite: 10, totalTokens: 180 },
    reasoningChars: 42, costUsd: 0.001, settled: true, text: '', ...extra,
  };
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

function ctx(fixture, drivers, sleep = noSleep) {
  const config = loadConfig(fixture.taskDir, fixture.root);
  return { taskDir: fixture.taskDir, cwd: fixture.root, config, drivers, sleep, log: noLog };
}

// --- Judge (§7 里程碑终验) ---------------------------------------------------

test('judge: pass verdict lets the milestone close with evidence recorded', async () => {
  const fx = makeFixture();
  const result = await runTask(ctx(fx, wrapDrivers()));
  assert.equal(result.status, 'completed');
  const gate = loadState(fx.taskDir).milestones['M-01'].gate;
  assert.equal(gate.judge.verdict, 'pass');
  assert.equal(typeof gate.judge.score, 'number');
  assert.ok(gate.judge.at);
});

test('judge: fail verdict blocks the milestone close and escalates to inbox', async () => {
  const fx = makeFixture();
  const drivers = wrapDrivers({
    judge: async () => okResult({ text: JSON.stringify({ verdict: 'return', score: 0.3, reasons: ['R-02 has no real evidence'] }) }),
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated');
  const state = loadState(fx.taskDir);
  assert.equal(state.milestones['M-01'].status, 'in-progress'); // NOT closed
  assert.equal(state.milestones['M-01'].gate.judge.verdict, 'return');
  assert.equal(state.inbox.length, 1);
  assert.match(state.inbox[0].reason, /judge/);
  assert.match(state.inbox[0].reason, /R-02/);
});

test('judge: L2/L3 gate runs BEFORE the judge (deterministic first, §2 rule 4)', async () => {
  const fx = makeFixture();
  const file = join(fx.taskDir, 'task-packet-M01-T01.md');
  writeFileSync(file, readFileSync(file, 'utf8').replace(
    '    - {cmd: "test -f build/M01-T01.txt", expect: "exit 0"}',
    '    - {cmd: "test -f build/M01-T01.txt", expect: "exit 0"}\n  L2:\n    - {cmd: "false", expect: "exit 0"}',
  ));
  let judgeCalls = 0;
  const drivers = wrapDrivers({ judge: async () => { judgeCalls += 1; return okResult(); } });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated');
  assert.equal(judgeCalls, 0); // red gate means the judge never speaks
});

test('judge: transport failure blocks the milestone close with an error recorded', async () => {
  const fx = makeFixture();
  const drivers = wrapDrivers({
    judge: async () => okResult({ exitCode: 1, stderr: '401: invalid api key', errorMessage: '401: invalid api key' }),
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated');
  const state = loadState(fx.taskDir);
  assert.equal(state.milestones['M-01'].status, 'in-progress');
  assert.equal(state.milestones['M-01'].gate.judge.verdict, 'error');
  assert.equal(state.milestones['M-01'].gate.judge.error, 'AUTH'); // 401: no retry, no fallback
  assert.match(state.inbox[0].reason, /judge/);
});

test('review: quota error falls back to the next configured channel', async () => {
  const fx = makeFixture();
  const mock = createMockDriver();
  const seen = [];
  const drivers = wrapDrivers({
    review: async (args) => {
      seen.push(args.channel);
      if (args.channel === 'opencode-go') {
        return okResult({ exitCode: 1, stderr: '403 quota', errorMessage: '403: fair use limit', text: '' });
      }
      return mock.review(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  assert.deepEqual(seen.slice(0, 2), ['opencode-go', 'opencode']);
});

test('judge: quota error falls back to the next configured channel', async () => {
  const fx = makeFixture();
  const mock = createMockDriver();
  const seen = [];
  const drivers = wrapDrivers({
    judge: async (args) => {
      seen.push({ channel: args.channel, model: args.model });
      if (args.channel === 'kimi-coding') {
        return okResult({ exitCode: 1, stderr: '429', errorMessage: '429: too many requests', text: '' });
      }
      return mock.judge(args);
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  assert.deepEqual(seen[0], { channel: 'kimi-coding', model: 'kimi-coding/k3' });
  const fb = seen.find((s) => s.channel === 'opencode');
  assert.ok(fb, 'judge fell back to opencode');
  assert.equal(fb.model, 'opencode/deepseek-v4-pro'); // channel model mapping
});

test('plan cold start: prompt carries the cold-start note and retro tail (§4.1)', async () => {
  const fx = makeFixture();
  writeFileSync(join(fx.taskDir, 'retro.md'), '- 2026-08-19 packet M01-T00: completed\n');
  const state = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
  state['plan-session'] = { 'session-id': 'session_DEAD', channel: 'kimi-subscription', model: 'kimi-code/k3', 'created-at': 'x', 'updated-at': 'x', resumes: 0 };
  writeFileSync(join(fx.taskDir, 'state.json'), JSON.stringify(state));
  const prompts = [];
  const drivers = wrapDrivers({
    plan: async (args) => {
      prompts.push({ sessionId: args.sessionId ?? null, prompt: args.prompt });
      if (args.sessionId === 'session_DEAD') {
        return okResult({ exitCode: 1, stderr: 'Session "session_DEAD" not found.', errorMessage: 'Session "session_DEAD" not found.' });
      }
      return okResult({ sessionId: 'session_NEW', text: 'ok' });
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  const cold = prompts.find((p) => p.sessionId === null);
  assert.ok(cold, 'a cold-start prompt was issued');
  assert.match(cold.prompt, /Cold start note/);
  assert.match(cold.prompt, /M01-T00/); // retro tail included
});

// --- Plan resident session (§4.1) -------------------------------------------

test('plan session: first call creates the session and stores its handle in state.json', async () => {
  const fx = makeFixture();
  const seen = [];
  const drivers = wrapDrivers({
    plan: async (args) => {
      seen.push(args.sessionId ?? null);
      return okResult({ sessionId: 'session_plan-1', text: 'ok' });
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  assert.deepEqual(seen, [null]); // first call starts fresh, no resume handle yet
  const session = loadState(fx.taskDir)['plan-session'];
  assert.equal(session['session-id'], 'session_plan-1');
  assert.ok(session['created-at']);
  assert.ok(session['updated-at']);
  // Resume of the stored handle is covered by the dead-session test below.
});

test('plan session: dead session triggers cold restart with a fresh session (§4.1)', async () => {
  const fx = makeFixture();
  // Pre-seed a resident session handle (model+channel pinned to the binding).
  const state = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
  state['plan-session'] = { 'session-id': 'session_DEAD', channel: 'kimi-subscription', model: 'kimi-code/k3', 'created-at': 'x', 'updated-at': 'x', resumes: 3 };
  writeFileSync(join(fx.taskDir, 'state.json'), JSON.stringify(state));
  const seen = [];
  const drivers = wrapDrivers({
    plan: async (args) => {
      seen.push(args.sessionId ?? null);
      if (args.sessionId === 'session_DEAD') {
        return okResult({ exitCode: 1, stderr: 'error: failed to run prompt: Session "session_DEAD" not found.', errorMessage: 'Session "session_DEAD" not found.' });
      }
      return okResult({ sessionId: 'session_NEW', text: 'ok' });
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  assert.deepEqual(seen[0], 'session_DEAD'); // resume attempted first
  assert.ok(seen.includes(null), 'cold restart with a fresh session followed');
  assert.equal(loadState(fx.taskDir)['plan-session']['session-id'], 'session_NEW');
});

test('plan session: binding mismatch skips the stored session — cold start (铁律 7)', async () => {
  const fx = makeFixture();
  const state = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
  // Stored on a DIFFERENT model than the current binding requires.
  state['plan-session'] = { 'session-id': 'session_OLD', channel: 'kimi-subscription', model: 'kimi-code/k2.6', 'created-at': 'x', 'updated-at': 'x', resumes: 1 };
  writeFileSync(join(fx.taskDir, 'state.json'), JSON.stringify(state));
  const seen = [];
  const drivers = wrapDrivers({
    plan: async (args) => {
      seen.push(args.sessionId ?? null);
      return okResult({ sessionId: 'session_FRESH', text: 'ok' });
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  assert.deepEqual(seen, [null]); // never resumed the mismatched session
  assert.equal(loadState(fx.taskDir)['plan-session']['session-id'], 'session_FRESH');
});

test('plan failure on every channel escalates the task to inbox', async () => {
  const fx = makeFixture();
  const drivers = wrapDrivers({
    plan: async () => okResult({ exitCode: 1, stderr: '401: invalid api key', errorMessage: '401: invalid api key' }),
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'escalated');
  const state = loadState(fx.taskDir);
  assert.equal(state.inbox.length, 1);
  assert.match(state.inbox[0].reason, /plan step failed/);
  assert.equal(state.packets['M01-T01'].status, 'pending'); // nothing executed
});

test('plan channel fallback: quota error switches to the next channel with its harness/model', async () => {
  const fx = makeFixture();
  const seen = [];
  const drivers = wrapDrivers({
    plan: async (args) => {
      seen.push({ channel: args.channel, model: args.model, harness: args.binding?.harness });
      if (args.channel === 'kimi-subscription') {
        return okResult({ exitCode: 1, stderr: '403 forbidden', errorMessage: '403: fair use limit' });
      }
      return okResult({ sessionId: 'session_fb', text: 'ok' });
    },
  });
  const result = await runTask(ctx(fx, drivers));
  assert.equal(result.status, 'completed');
  assert.equal(seen[0].channel, 'kimi-subscription');
  const fb = seen.find((s) => s.channel === 'kimi-coding');
  assert.ok(fb, 'fell back to kimi-coding');
  assert.equal(fb.model, 'kimi-coding/k3');
  assert.equal(fb.harness, 'pi'); // harness switch travels with the channel
});

// --- needs-live-credentials (§9) ---------------------------------------------

test('credentials: approval injects only the approved keys and records evidence', async () => {
  const fx = makeFixture();
  // Mark T01 as needing live credentials; pre-write the human approval.
  const file = join(fx.taskDir, 'task-packet-M01-T01.md');
  writeFileSync(file, readFileSync(file, 'utf8').replace('needs-live-credentials: false', 'needs-live-credentials: true'));
  mkdirSync(join(fx.taskDir, 'approvals'), { recursive: true });
  writeFileSync(join(fx.taskDir, 'approvals', 'M01-T01.json'), JSON.stringify({ 'approved-env': ['FLYWHEEL_PROBE_KEY', 'FLYWHEEL_ABSENT_KEY'] }));

  process.env.FLYWHEEL_PROBE_KEY = 'probe-secret';
  delete process.env.FLYWHEEL_ABSENT_KEY;
  let capturedKeys = null;
  const mock = createMockDriver();
  const drivers = wrapDrivers({
    execute: async (args) => {
      if (args.packet['packet-id'] === 'M01-T01') capturedKeys = args.apiKeys;
      return mock.execute(args);
    },
  });
  try {
    const result = await runTask(ctx(fx, drivers));
    assert.equal(result.status, 'completed');
  } finally {
    delete process.env.FLYWHEEL_PROBE_KEY;
  }
  assert.equal(capturedKeys.FLYWHEEL_PROBE_KEY, 'probe-secret');
  assert.equal(capturedKeys.FLYWHEEL_ABSENT_KEY, undefined); // approved but absent -> not injected
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.deepEqual(p.evidence.credentials['injected-env'], ['FLYWHEEL_PROBE_KEY']);
  assert.deepEqual(p.evidence.credentials['missing-env'], ['FLYWHEEL_ABSENT_KEY']);
});

test('credentials: approval timeout escalates to inbox without executing', async () => {
  const fx = makeFixture();
  const file = join(fx.taskDir, 'task-packet-M01-T01.md');
  writeFileSync(file, readFileSync(file, 'utf8').replace('needs-live-credentials: false', 'needs-live-credentials: true'));
  // Fixture config: credentials timeout 200ms, poll 10ms — real sleep needed.
  let executed = false;
  const drivers = wrapDrivers({ execute: async () => { executed = true; return okResult(); } });
  const result = await runTask(ctx(fx, drivers, realSleep));
  assert.equal(result.status, 'escalated');
  assert.equal(executed, false);
  const state = loadState(fx.taskDir);
  const p = state.packets['M01-T01'];
  assert.equal(p.status, 'inbox');
  assert.ok(state.inbox.some((e) => /timed out/.test(e.reason)));
  assert.ok(state.inbox.some((e) => /awaiting|approved-env/.test(e.reason))); // human was asked first
});

test('credentials: waiting state is visible as awaiting-credentials while polling', async () => {
  const fx = makeFixture();
  const file = join(fx.taskDir, 'task-packet-M01-T01.md');
  writeFileSync(file, readFileSync(file, 'utf8').replace('needs-live-credentials: false', 'needs-live-credentials: true'));
  // Approve mid-wait, and observe the intermediate state while polling.
  setTimeout(() => {
    mkdirSync(join(fx.taskDir, 'approvals'), { recursive: true });
    writeFileSync(join(fx.taskDir, 'approvals', 'M01-T01.json'), JSON.stringify({ 'approved-env': [] }));
  }, 60);
  const run = runTask(ctx(fx, wrapDrivers(), realSleep));
  let midState = null;
  for (let i = 0; i < 40 && midState === null; i++) {
    await new Promise((r) => setTimeout(r, 5));
    const s = JSON.parse(readFileSync(join(fx.taskDir, 'state.json'), 'utf8'));
    if (s.packets['M01-T01'].status === 'awaiting-credentials') midState = s;
  }
  const result = await run;
  assert.ok(midState, 'awaiting-credentials was observable while polling');
  assert.ok(midState.inbox.some((e) => /approved-env/.test(e.reason))); // human was asked
  assert.equal(result.status, 'completed');
  const p = loadState(fx.taskDir).packets['M01-T01'];
  assert.deepEqual(p.evidence.credentials['approved-env'], []);
});
