import assert from 'node:assert/strict';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test, { after } from 'node:test';
import { Context } from '@deepseek-ai/cordis';
import { apply } from '../src/index.js';
import { validPacketTriageCommand, validPacketUserAction, packetUserAction } from '../src/packet-control.js';
import { readTaskTraceDetail } from '../src/trace-parser.js';
import { act, closeClientHarness, envelope, mountWorkbench } from './support/client-harness.js';

after(closeClientHarness);

test('packet-control validates retry, replan, and accept payloads and host proofs', () => {
  const retryCmd = {
    action: 'retry',
    'packet-id': 'M01-T02',
    revision: 12,
    reason: 'Fix F-1 and skip meta-tests',
    guidance: 'Only fix real contract defects.',
  };
  assert.equal(validPacketTriageCommand(retryCmd), true);
  assert.equal(validPacketTriageCommand({ ...retryCmd, action: 'replan' }), true);
  assert.equal(validPacketTriageCommand({ ...retryCmd, action: 'accept' }), true);
  assert.equal(validPacketTriageCommand({ ...retryCmd, action: 'invalid' }), false);
  assert.equal(validPacketTriageCommand({ ...retryCmd, revision: 0 }), false);
  assert.equal(validPacketTriageCommand({ ...retryCmd, reason: '   ' }), false);

  const rawInput = JSON.stringify({ action: 'packet', taskDir: '/repo/docs/t', packet: retryCmd });
  const proof = packetUserAction(
    JSON.parse(rawInput),
    { id: 'sess-1' },
    'cmd-1',
    rawInput,
    { commands: { 'cmd-1': rawInput } },
  );
  assert.equal(validPacketUserAction(proof, retryCmd, '/repo/docs/t'), true);
  assert.equal(validPacketUserAction({ ...proof, packetId: 'M01-T99' }, retryCmd, '/repo/docs/t'), false);
});

test('apply() degrades gracefully on invalid installation record instead of crashing plugin registration', async () => {
  const home = await mkdtemp(join(tmpdir(), 'fw-degraded-home-'));
  const origHome = process.env.HOME;
  try {
    await mkdir(join(home, '.config', 'flywheel'), { recursive: true });
    await writeFile(join(home, '.config', 'flywheel', 'installation.json'), '{broken', 'utf8');
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
      inject(_deps, cb) {
        mountedPromise = cb(cordis);
      },
    };
    assert.doesNotThrow(() => apply(ctx, {}));
    await mountedPromise;
    assert.ok(cordis.flywheelConsole, 'expected ConsoleReadService mounted even when installation.json is invalid');
    const queryRes = await cordis.flywheelConsole.query({ action: 'runs', cwd: home });
    assert.equal(queryRes.result.kind, 'error');
    assert.match(queryRes.result.text, /npx -y @ckanner\/flywheel@latest install/);
    assert.ok(registeredTools.has('flywheel_control'));
    const tool = registeredTools.get('flywheel_control');
    await assert.rejects(
      () => tool.execute({ action: 'packet', taskDir: home, packet: { action: 'retry', 'packet-id': 'M01-T01', revision: 1, reason: 'bot' } }, { agent: {}, callId: 'c1' }),
      /model tools cannot triage escalated inbox packets/i,
    );
    await assert.rejects(
      () => tool.execute({ action: 'status', taskDir: home }, { agent: {}, callId: 'c2' }),
      /npx -y @ckanner\/flywheel@latest install/,
    );
  } finally {
    process.env.HOME = origHome;
    await rm(home, { recursive: true, force: true });
  }
});

test('trace-parser extracts true single-turn contextUsage from Codex rollout token_count (including post-compaction drop)', async () => {
  const root = await mkdtemp(join(tmpdir(), 'fw-trace-ctx-'));
  const fakeHome = join(root, 'home');
  const taskDir = join(root, 'task');
  const origHome = process.env.HOME;
  try {
    process.env.HOME = fakeHome;
    await mkdir(join(taskDir, '.traces'), { recursive: true });
    const sessionId = '019d0000-0000-7000-8000-000000000041';
    const rolloutDir = join(fakeHome, '.codex', 'sessions', '2026', '11', '26');
    await mkdir(rolloutDir, { recursive: true });

    const rolloutLines = [
      JSON.stringify({
        timestamp: '2026-11-26T10:00:05.000Z',
        type: 'event_msg',
        payload: {
          type: 'token_count',
          info: {
            model_context_window: 828400,
            last_token_usage: { input_tokens: 410000, cached_input_tokens: 390000, output_tokens: 1000 },
          },
        },
      }),
      JSON.stringify({
        timestamp: '2026-11-26T10:00:10.000Z',
        type: 'event_msg',
        payload: {
          type: 'token_count',
          info: {
            model_context_window: 828400,
            last_token_usage: { input_tokens: 125979, cached_input_tokens: 125312, output_tokens: 421 },
          },
        },
      }),
      JSON.stringify({
        timestamp: '2026-11-26T10:00:11.000Z',
        type: 'response_item',
        payload: { type: 'message', role: 'assistant', phase: 'final_answer', content: [{ type: 'output_text', text: 'Done' }] },
      }),
    ].join('\n');
    await writeFile(join(rolloutDir, `rollout-2026-11-26-${sessionId}.jsonl`), rolloutLines, 'utf8');

    const liveLines = [
      JSON.stringify({
        kind: 'start',
        invocationId: 'call-ctx-1',
        key: 'review/M01-T02',
        role: 'review',
        scope: 'M01-T02',
        harness: 'codex-pro',
        model: 'gpt-5.3-codex',
        cwd: root,
        workspaceCwd: root,
        ts: '2026-11-26T10:00:00.000Z',
      }),
      JSON.stringify({
        kind: 'end',
        ts: '2026-11-26T10:00:15.000Z',
        exitCode: 0,
        sessionId,
        harness: 'codex-pro',
        model: 'gpt-5.3-codex',
        usage: { input: 1350000, output: 12000, cache_read: 1200000, total_tokens: 1362000 },
        text: 'Done',
      }),
    ].join('\n');
    await writeFile(join(taskDir, '.traces', 'live-call-ctx-1.jsonl'), liveLines, 'utf8');

    const detail = await readTaskTraceDetail(taskDir, 'call-ctx-1');
    assert.ok(detail.trace?.contextUsage, 'expected single-turn contextUsage from Codex rollout');
    assert.equal(detail.trace.contextUsage.size, 828400);
    assert.equal(detail.trace.contextUsage.used, 125979 + 421);
    assert.equal(detail.trace.contextUsage.source, 'codex-last-turn');
    assert.equal(detail.trace.contextUsage.lastInput, 125979);
    assert.equal(detail.trace.contextUsage.lastCacheRead, 125312);
  } finally {
    process.env.HOME = origHome;
    await rm(root, { recursive: true, force: true });
  }
});

test('Bottom-of-stream Escalation Triage Card renders findings, all 3 actions, and telemetry dock uses truthful single-turn occupancy', async t => {
  const actCalls = [];
  const taskDir = '/repo/docs/tetris';
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({ protocol: 1, ok: true, cwd: '/repo', runs: [{ taskDir, cwd: '/repo', slug: 'tetris', status: 'escalated', running: false }] });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir,
          running: false,
          state: {
            slug: 'tetris',
            status: 'escalated',
            revision: 42,
            inbox: [{ at: '2026-11-26T10:00:00Z', 'packet-id': 'M01-T02', reason: 'packet M01-T02 code review did not converge (3 review returns). Human needed.' }],
            packets: {
              'M01-T02': {
                id: 'M01-T02',
                goal: 'Scoring & palette helpers',
                status: 'inbox',
                evidence: {
                  l1: { pass: true, identity: 'cand-1' },
                  review: {
                    verdict: 'return',
                    findings: [
                      { id: 'F-1', severity: 'major', status: 'open', target: 'src/score.ts:12', evidence: 'b2b flag mismatch', fix: 'Return false on non-difficult clear' },
                      { id: 'F-0', severity: 'major', status: 'resolved', target: 'src/a.ts:1', evidence: 'fixed' },
                    ],
                    advisory: ['Optional test polish'],
                  },
                },
              },
            },
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
              id: 'call-rev-3',
              key: 'review/M01-T02',
              role: 'review',
              scope: 'M01-T02',
              harness: 'codex-pro',
              model: 'gpt-5.3-codex',
              status: 'completed',
              running: false,
              usage: { input: 1350000, output: 12000, cache_read: 1243000, total_tokens: 1362000 },
              contextUsage: { used: 126400, size: 828400, source: 'codex-last-turn', lastInput: 125979, lastCacheRead: 125312 },
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
            id: 'call-rev-3',
            key: 'review/M01-T02',
            role: 'review',
            scope: 'M01-T02',
            harness: 'codex-pro',
            model: 'gpt-5.3-codex',
            status: 'completed',
            running: false,
            usage: { input: 1350000, output: 12000, cache_read: 1243000, total_tokens: 1362000 },
            contextUsage: { used: 126400, size: 828400, source: 'codex-last-turn', lastInput: 125979, lastCacheRead: 125312 },
            blocks: [{ kind: 'assistant', text: 'Review complete' }],
          },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir });
    },
    act: async input => {
      actCalls.push(input);
      if (input.action === 'packet') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir,
          running: false,
          'packet-control': { 'packet-id': 'M01-T02', action: input.packet.action, applied: true, revision: 43, status: 'pending' },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir, running: true });
    },
  });

  const summary = ui.container.querySelector('[data-testid="main-escalation-summary"]');
  assert.ok(summary, 'expected bottom-of-stream persistent escalation triage card');
  assert.match(summary.textContent, /M01-T02/);
  assert.match(summary.textContent, /L1 确定性门禁已绿/);
  assert.match(summary.textContent, /F-1/);
  assert.match(summary.textContent, /b2b flag mismatch/);
  assert.ok(summary.querySelector('[data-testid="triage-retry-M01-T02"]'));
  assert.ok(summary.querySelector('[data-testid="triage-replan-M01-T02"]'));
  assert.ok(summary.querySelector('[data-testid="triage-accept-M01-T02"]'));

  // Click Retry with Guidance -> sends action: 'packet' then calls startRun
  await act(async () => {
    summary.querySelector('[data-testid="triage-retry-M01-T02"]').click();
    await Promise.resolve();
    await Promise.resolve();
  });
  assert.equal(actCalls[0]?.action, 'packet');
  assert.equal(actCalls[0]?.packet?.action, 'retry');
  assert.equal(actCalls[1]?.action, 'start');

  // Drill into review node -> SubSession telemetry pill 3 shows true 15% (126.4K / 828.4K), NOT 1.35M!
  await act(async () => {
    ui.container.querySelector('[data-testid="packet-step-call-rev-3"]').click();
    await Promise.resolve();
    await Promise.resolve();
  });
  const ctxPill = ui.container.querySelector('[data-testid="telemetry-sub-context"]');
  assert.ok(ctxPill, 'expected SubSession context pill');
  assert.match(ctxPill.textContent, /15%/);
  assert.doesNotMatch(ctxPill.textContent, /1\.35M/);
  await act(async () => {
    ctxPill.click();
  });
  const ctxPop = ui.container.querySelector('[data-testid="telemetry-sub-context-popover"]');
  assert.ok(ctxPop);
  assert.match(ctxPop.textContent, /126\.4K \/ 828\.4K \(15\.3%\)/);

  await act(async () => {
    ui.container.querySelector('[data-testid="telemetry-sub-cache"]').click();
  });
  const cachePop = ui.container.querySelector('[data-testid="telemetry-sub-cache-popover"]');
  assert.ok(cachePop);
  assert.match(cachePop.textContent, /末步缓存命中率/);
  assert.match(cachePop.textContent, /99\.5%/);
});
