import assert from 'node:assert/strict';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test, { after } from 'node:test';
import { Context } from '@deepseek-ai/cordis';
import { apply } from '../src/index.js';
import {
  validMilestoneTriageCommand,
  validMilestoneUserAction,
  milestoneUserAction,
  milestoneUserProjection,
} from '../src/milestone-control.js';
import { readTaskTraceDetail } from '../src/trace-parser.js';
import { act, closeClientHarness, envelope, mountWorkbench } from './support/client-harness.js';

after(closeClientHarness);

test('milestone-control validates retry/accept commands, projection, host proofs, and blocks LLM tool invocation', async () => {
  const retryCmd = {
    action: 'retry',
    'milestone-id': 'M-01',
    revision: 15,
    reason: 'Re-run L2 after npm ci',
    guidance: 'Ignore synthetic preload fuzzing edge cases.',
  };
  assert.equal(validMilestoneTriageCommand(retryCmd), true);
  assert.equal(validMilestoneTriageCommand({ ...retryCmd, action: 'accept' }), true);
  assert.equal(validMilestoneTriageCommand({ ...retryCmd, action: 'replan' }), false);
  assert.equal(validMilestoneTriageCommand({ ...retryCmd, revision: 0 }), false);
  assert.equal(validMilestoneTriageCommand({ ...retryCmd, reason: '   ' }), false);
  assert.equal(validMilestoneTriageCommand({ ...retryCmd, extra: 1 }), false);

  const rawInput = JSON.stringify({ action: 'milestone', taskDir: '/repo/docs/t', milestone: retryCmd });
  let proj = milestoneUserProjection.init({}, 0);
  proj = milestoneUserProjection.apply(proj, {
    seq: 1,
    type: 'command/run',
    data: { name: 'flywheel', commandId: 'cmd-m1', args: rawInput, source: { kind: 'user' } },
  });
  const proof = milestoneUserAction(JSON.parse(rawInput), { id: 'sess-m' }, 'cmd-m1', rawInput, proj);
  assert.equal(validMilestoneUserAction(proof, retryCmd, '/repo/docs/t'), true);
  assert.equal(validMilestoneUserAction({ ...proof, milestoneId: 'M-99' }, retryCmd, '/repo/docs/t'), false);

  // Verify model tool rejects action === 'milestone'
  const home = await mkdtemp(join(tmpdir(), 'fw-ms-tool-'));
  const origHome = process.env.HOME;
  try {
    process.env.HOME = home;
    const cordis = new Context();
    let mountedPromise = null;
    const registeredTools = new Map();
    const ctx = {
      sessionProjections: { register() {}, stateOf() { return null; } },
      commands: { register() {} },
      tools: { register(def) { registeredTools.set(def.name, def); } },
      workingDirectory: { get() { return home; } },
      on() {},
      inject(_deps, cb) { mountedPromise = cb(cordis); },
    };
    apply(ctx, {});
    await mountedPromise;
    const tool = registeredTools.get('flywheel_control');
    await assert.rejects(
      () => tool.execute({ action: 'milestone', taskDir: home, milestone: retryCmd }, { agent: {}, callId: 'c1' }),
      /model tools cannot triage escalated milestones/i,
    );
  } finally {
    process.env.HOME = origHome;
    await rm(home, { recursive: true, force: true });
  }
});

test('SPEC-09: trace-parser reliably extracts ACP usage_update contextUsage in top-level and nested forms', async () => {
  const root = await mkdtemp(join(tmpdir(), 'fw-acp-usage-'));
  const taskDir = join(root, 'task');
  try {
    await mkdir(join(taskDir, '.traces'), { recursive: true });
    const lines = [
      JSON.stringify({
        kind: 'start',
        invocationId: 'call-agy-1',
        key: 'execute/M01-T01',
        role: 'execute',
        scope: 'M01-T01',
        harness: 'antigravity',
        model: 'gemini-4-neon',
        cwd: root,
        workspaceCwd: root,
        ts: '2026-11-27T01:00:00.000Z',
      }),
      JSON.stringify({
        kind: 'event',
        ts: '2026-11-27T01:00:05.000Z',
        line: JSON.stringify({ sessionUpdate: 'usage_update', used: 42000, size: 1000000 }),
      }),
      JSON.stringify({
        kind: 'event',
        ts: '2026-11-27T01:00:10.000Z',
        line: JSON.stringify({ type: 'usage_update', data: { contextUsed: 84000, contextSize: 1000000 } }),
      }),
      JSON.stringify({
        kind: 'end',
        ts: '2026-11-27T01:00:15.000Z',
        exitCode: 0,
        harness: 'antigravity',
        model: 'gemini-4-neon',
        text: 'Done',
      }),
    ].join('\n');
    await writeFile(join(taskDir, '.traces', 'live-call-agy-1.jsonl'), lines, 'utf8');
    const detail = await readTaskTraceDetail(taskDir, 'call-agy-1');
    assert.deepEqual(detail.trace?.contextUsage, { used: 84000, size: 1000000 });
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test('SPEC-10 to SPEC-14: Stage 1 blocked separation, bottom Milestone triage & blocked details, collapsed plan revision, running highlight, and 1-based turn counter', async t => {
  const actCalls = [];
  const taskDir = '/repo/docs/sample-app';
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({ protocol: 1, ok: true, cwd: '/repo', runs: [{ taskDir, cwd: '/repo', slug: 'sample-app', status: 'escalated', running: false }] });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir,
          running: false,
          state: {
            slug: 'sample-app',
            status: 'escalated',
            revision: 88,
            planning: {
              phase: 'approved',
              requests: [
                { id: 'rev-1', scope: 'milestone', 'scope-id': 'M-01', status: 'passed', feedback: 'Returned packet M01-T01: fix lint' },
              ],
            },
            sessions: {
              records: {
                'review/M01-T02': {
                  reason: 'output-limit truncated before terminal receipt',
                  quality: { 'confirmed-turns': 0 },
                  last: { id: 'call-blocked-rev', status: 'blocked' },
                },
              },
            },
            milestones: {
              'M-01': {
                status: 'in-progress',
                gate: {
                  pass: true,
                  identity: 'sha-head-1',
                  judge: {
                    verdict: 'return',
                    score: 0.78,
                    reasons: ['Synthetic preload edge case in helper script'],
                  },
                },
              },
            },
            packets: {
              'M01-T01': { id: 'M01-T01', goal: 'Done unit', status: 'completed', milestone: 'M-01' },
              'M01-T02': { id: 'M01-T02', goal: 'Reviewing unit', status: 'l1-passed', milestone: 'M-01' },
            },
            inbox: [
              { at: '2026-11-27T01:00:00Z', milestone: 'M-01', reason: 'milestone M-01 judge verdict: return — Synthetic preload edge case' },
            ],
          },
        });
      }
      if (input.action === 'traces') {
        return envelope({
          protocol: 1,
          ok: true,
          taskDir,
          traces: [
            {
              id: 'call-plan-rev-1',
              key: 'plan/planning:rev-1',
              role: 'plan',
              scope: 'planning:rev-1',
              harness: 'antigravity',
              model: 'gemini-4-neon',
              status: 'completed',
              running: false,
              startedAt: '2026-11-27T00:05:00Z',
            },
            {
              id: 'call-rev-running',
              key: 'review/M01-T02',
              role: 'review',
              scope: 'M01-T02',
              harness: 'codex-pro',
              model: 'gpt-6.2-astra',
              status: 'running',
              running: true,
              startedAt: '2026-11-27T00:01:00Z',
            },
          ],
        });
      }
      if (input.action === 'trace-detail') {
        return envelope({
          protocol: 1,
          ok: true,
          taskDir,
          trace: {
            id: 'call-rev-running',
            key: 'review/M01-T02',
            role: 'review',
            scope: 'M01-T02',
            harness: 'codex-pro',
            model: 'gpt-6.2-astra',
            status: 'running',
            running: true,
            blocks: [{ kind: 'thinking', text: 'Inspecting diff' }],
          },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir });
    },
    act: async input => {
      actCalls.push(input);
      if (input.action === 'milestone') {
        return envelope({
          protocol: 1,
          ok: true,
          taskDir,
          state: { slug: 'sample-app', status: 'completed', revision: 89, milestones: { 'M-01': { status: 'closed' } }, packets: {} },
          'milestone-control': { 'milestone-id': 'M-01', action: input.milestone.action, applied: true, revision: 89, status: 'closed' },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir });
    },
  });

  // SPEC-10: review/M01-T02 blocked session must NOT appear inside Stage 1 inline red banner
  assert.equal(ui.container.querySelector('[data-testid="main-blocked-sessions"]'), null);

  // SPEC-11: Bottom escalation summary shows blocked session details AND Milestone triage card
  const bottomBlocked = ui.container.querySelector('[data-testid="bottom-blocked-sessions"]');
  assert.ok(bottomBlocked, 'expected execution/review blocked session in bottom escalation summary');
  assert.match(bottomBlocked.textContent, /output-limit truncated/);
  assert.ok(ui.container.querySelector('[data-testid="bottom-blocked-details-call-blocked-rev"]'));

  const msCard = ui.container.querySelector('[data-testid="milestone-triage-card-M-01"]');
  assert.ok(msCard, 'expected milestone triage card in bottom escalation summary');
  assert.match(msCard.textContent, /0\.78/);
  assert.ok(ui.container.querySelector('[data-testid="milestone-triage-retry-M-01"]'));
  const acceptBtn = ui.container.querySelector('[data-testid="milestone-triage-accept-M-01"]');
  assert.ok(acceptBtn, 'expected conditional Accept Milestone button when gate.pass === true');

  // SPEC-12: Completed mid-execution plan revision is collapsed by default
  const revCard = ui.container.querySelector('[data-testid="plan-revision-card-rev-1"]');
  assert.ok(revCard, 'expected completed plan revision card');
  assert.equal(revCard.hasAttribute('open'), false, 'completed plan revision must default to collapsed');

  // SPEC-13: l1-passed packet with running review trace gets subAgentRowCardRunning class
  const pktCard = ui.container.querySelector('[data-testid="packet-card-M01-T02"]');
  assert.ok(pktCard, 'expected packet card M01-T02');
  assert.match(pktCard.className, /subAgentRowCardRunning/);

  // Click Accept Milestone and verify milestone act payload
  await act(async () => {
    acceptBtn.click();
  });
  const msCall = actCalls.find(c => c.action === 'milestone');
  assert.ok(msCall, 'expected milestone act call');
  assert.equal(msCall.milestone.action, 'accept');
  assert.equal(msCall.milestone['milestone-id'], 'M-01');
  assert.equal(msCall.milestone.revision, 88);

  // SPEC-14: Drill into running sub-session with confirmed-turns = 0 -> must show Turn 1 / 第 1 轮, NEVER 0 轮
  await act(async () => {
    pktCard.click();
  });
  const dockText = ui.container.textContent;
  assert.doesNotMatch(dockText, /\b0\s*轮|\b0t\b/);
  assert.match(dockText, /第 1 轮|Turn 1/);
});

test('SPEC-14: running sub-session without confirmed-turns record shows Turn 1 (never Turn 2 or 0 轮)', async t => {
  const taskDir = '/repo/docs/first-turn';
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({ protocol: 1, ok: true, cwd: '/repo', runs: [{ taskDir, cwd: '/repo', slug: 'first-turn', status: 'running', running: true }] });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir,
          running: true,
          state: {
            slug: 'first-turn',
            status: 'running',
            revision: 3,
            packets: { 'M01-T01': { id: 'M01-T01', goal: 'Init', status: 'executing', milestone: 'M-01' } },
          },
        });
      }
      if (input.action === 'traces') {
        return envelope({
          protocol: 1,
          ok: true,
          taskDir,
          traces: [
            {
              id: 'call-exec-1',
              key: 'execute/M01-T01',
              role: 'execute',
              scope: 'M01-T01',
              harness: 'antigravity',
              model: 'gemini-4-neon',
              status: 'running',
              running: true,
              startedAt: '2026-11-27T01:00:00Z',
            },
          ],
        });
      }
      if (input.action === 'trace-detail') {
        return envelope({
          protocol: 1,
          ok: true,
          taskDir,
          trace: {
            id: 'call-exec-1',
            key: 'execute/M01-T01',
            role: 'execute',
            scope: 'M01-T01',
            harness: 'antigravity',
            model: 'gemini-4-neon',
            status: 'running',
            running: true,
            blocks: [{ kind: 'thinking', text: 'Working' }],
          },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir });
    },
    act: async () => envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir }),
  });

  const pktCard = ui.container.querySelector('[data-testid="packet-card-M01-T01"]');
  assert.ok(pktCard);
  assert.match(pktCard.className, /subAgentRowCardRunning/);
  await act(async () => {
    pktCard.click();
  });
  const dockText = ui.container.textContent;
  assert.doesNotMatch(dockText, /\b0\s*轮|\b0t\b|第 2 轮|Turn 2/);
  assert.match(dockText, /第 1 轮|Turn 1/);
});
