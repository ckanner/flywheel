import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { appendFile, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Context } from '@deepseek-ai/cordis';
import { ConsoleReadService } from '../src/console-read.js';
import { buildResumeInfo, readTaskTraceDetail, readTaskTraces, unwrapStreamingContractMarkdown } from '../src/trace-parser.js';
import { act, closeClientHarness, envelope, mountWorkbench } from './support/client-harness.js';

after(closeClientHarness);

test('buildResumeInfo produces exact safe provider resume commands for all 5 harnesses and rejects unsafe IDs', () => {
  assert.deepEqual(buildResumeInfo('dsh', 'flywheel-abc-123', '/repo'), {
    kind: 'dsh-native',
    harness: 'dsh',
    providerLabel: 'DSH Desktop',
    sessionId: 'flywheel-abc-123',
    cwd: '/repo',
    command: '',
  });
  assert.equal(
    buildResumeInfo('claude-code-pro', 'sess-claude-1', "/tmp/a'b").command,
    "cd '/tmp/a'\\''b' && claude --resume 'sess-claude-1'",
  );
  assert.equal(
    buildResumeInfo('codex-pro', 'thread-99', '/repo').command,
    "cd '/repo' && codex resume 'thread-99'",
  );
  const kimi = buildResumeInfo('kimi', 'session_k1', '/repo');
  assert.equal(kimi.command, "cd '/repo' && kimi --session 'session_k1'");
  assert.equal(kimi.secondaryCommand, "cd '/repo' && kimi vis 'session_k1'");
  assert.equal(
    buildResumeInfo('antigravity', 'conv-acp-1', '/repo').command,
    "cd '/repo' && agy --conversation 'conv-acp-1'",
  );

  // Rejects shell injection in sessionId or empty IDs
  assert.equal(buildResumeInfo('codex-pro', 'bad; rm -rf /', '/repo'), null);
  assert.equal(buildResumeInfo('codex-pro', '', '/repo'), null);
});

test('readTaskTraces & readTaskTraceDetail parse in-flight live streams, failed attempts, and all provider formats', async t => {
  const taskDir = await mkdtemp(join(tmpdir(), 'fw-traces-'));
  t.after(() => rm(taskDir, { recursive: true, force: true }));
  await mkdir(join(taskDir, '.traces'), { recursive: true });

  // 1. Live in-flight Antigravity ACP trace (no end frame yet -> running: true)
  const liveAcp = [
    JSON.stringify({
      kind: 'start',
      ts: '2026-11-23T12:01:00Z',
      invocationId: 'call-live-acp',
      key: 'execute/M01-T02',
      role: 'execute',
      scope: 'M01-T02',
      attempt: 2,
      harness: 'antigravity',
      model: 'gemini-4-neon-high',
      thinking: 'high',
      cwd: '/repo',
      requestedThread: 'conv-acp-42',
      sessionTitle: 'Flywheel · execute · Inspect CSS tokens',
    }),
    JSON.stringify({
      kind: 'event',
      ts: '2026-11-23T12:01:02Z',
      line: JSON.stringify({
        type: 'native-event',
        event: { params: { update: { sessionUpdate: 'agent_thought_chunk', content: { text: 'Inspecting CSS tokens' } } } },
      }),
    }),
    JSON.stringify({
      kind: 'event',
      ts: '2026-11-23T12:01:03Z',
      line: JSON.stringify({
        type: 'native-event',
        event: { params: { update: { sessionUpdate: 'tool_call', title: 'read_file', status: 'in_progress', rawInput: { path: 'a.css' } } } },
      }),
    }),
  ].join('\n') + '\n';
  await writeFile(join(taskDir, '.traces', 'live-call-live-acp.jsonl'), liveAcp);

  // 2. Completed failed Claude trace with thinking + error + stderr
  const failedClaude = [
    JSON.stringify({
      kind: 'start',
      ts: '2026-11-23T12:00:00Z',
      invocationId: 'call-failed-claude',
      key: 'execute/M01-T02',
      role: 'execute',
      scope: 'M01-T02',
      attempt: 1,
      harness: 'claude-code-pro',
      model: 'claude-opus-5-5',
      thinking: 'high',
      cwd: '/repo',
    }),
    JSON.stringify({
      kind: 'event',
      ts: '2026-11-23T12:00:05Z',
      line: JSON.stringify({
        type: 'assistant',
        message: {
          content: [
            { type: 'thinking', thinking: 'Planning button refactor' },
            { type: 'tool_use', name: 'Bash', input: { command: 'npm test' } },
          ],
        },
      }),
    }),
    JSON.stringify({
      kind: 'end',
      ts: '2026-11-23T12:00:10Z',
      exitCode: 1,
      durationMs: 10000,
      harness: 'claude-code-pro',
      model: 'claude-opus-5-5',
      sessionId: 'claude-sess-77',
      errorCode: 'TIMEOUT',
      errorMessage: 'verification failed on attempt 1',
      stderr: 'AssertionError: expected 1 to equal 2',
    }),
  ].join('\n') + '\n';
  await writeFile(join(taskDir, '.traces', 'live-call-failed-claude.jsonl'), failedClaude);

  const list = await readTaskTraces(taskDir);
  assert.equal(list.traces.length, 2);
  // Running trace sorts first
  assert.equal(list.traces[0].id, 'call-live-acp');
  assert.equal(list.traces[0].running, true);
  assert.equal(list.traces[0].sessionTitle, 'Flywheel · execute · Inspect CSS tokens');
  assert.ok(list.traces[0].byteSize > 0);
  assert.equal(list.traces[0].resume.command, "cd '/repo' && agy --conversation 'conv-acp-42'");

  assert.equal(list.traces[1].id, 'call-failed-claude');
  assert.equal(list.traces[1].status, 'failed');
  assert.equal(list.traces[1].exitCode, 1);
  assert.equal(list.traces[1].resume.command, "cd '/repo' && claude --resume 'claude-sess-77'");

  const liveDetail = await readTaskTraceDetail(taskDir, 'call-live-acp');
  assert.equal(liveDetail.trace.blocks.some(b => b.kind === 'thinking' && b.text.includes('Inspecting CSS tokens')), true);
  assert.equal(liveDetail.trace.blocks.some(b => b.kind === 'tool_call' && b.title.includes('read_file')), true);

  const failDetail = await readTaskTraceDetail(taskDir, 'call-failed-claude');
  assert.equal(failDetail.trace.blocks.some(b => b.kind === 'thinking' && b.text.includes('Planning button refactor')), true);
  assert.equal(failDetail.trace.blocks.some(b => b.kind === 'error' && b.text.includes('AssertionError')), true);

  // Verify waitSinceBytes wakes up immediately via fs.watch when new live frames append
  const ctx = new Context();
  t.after(() => ctx.fiber.dispose());
  const svc = new ConsoleReadService(ctx, async () => ({}), (_in, err) => ({ ok: false, message: err.message }), taskDir);
  const initialBytes = liveDetail.trace.byteSize;
  const started = Date.now();
  const waitPromise = svc.query({
    action: 'trace-detail',
    taskDir,
    traceId: 'call-live-acp',
    waitSinceBytes: initialBytes,
  });
  setTimeout(() => {
    void appendFile(
      join(taskDir, '.traces', 'live-call-live-acp.jsonl'),
      JSON.stringify({
        kind: 'event',
        ts: '2026-11-23T12:01:04Z',
        line: JSON.stringify({
          type: 'native-event',
          event: { params: { update: { sessionUpdate: 'agent_message_chunk', content: { text: 'Streaming live delta!' } } } },
        }),
      }) + '\n',
    );
  }, 40);
  const wakeRes = await waitPromise;
  const elapsed = Date.now() - started;
  const parsed = JSON.parse(wakeRes.result.text);
  assert.equal(parsed.ok, true);
  assert.ok(parsed.trace.byteSize > initialBytes);
  assert.equal(parsed.trace.blocks.some(b => b.kind === 'assistant' && b.text.includes('Streaming live delta!')), true);
  assert.ok(elapsed < 1200, `expected fs.watch wakeup < 1200ms, got ${elapsed}ms`);
});

test('Agent-Team drill-down renders live SubSessionView, Provider Session Popover (Warp & DSH), interrupt takeover, and hand-back', async t => {
  const openedSessions = [];
  const traces = [
    {
      id: 'call-fail-1',
      key: 'execute/M01-T01',
      role: 'execute',
      scope: 'M01-T01',
      attempt: 1,
      harness: 'codex-pro',
      model: 'gpt-6.2-astra',
      thinking: 'high',
      cwd: '/repo',
      status: 'failed',
      running: false,
      exitCode: 1,
      durationMs: 4200,
      sessionId: 'codex-thread-9',
      sessionTitle: 'Flywheel · execute · Fix worker timeout',
      resume: {
        kind: 'cli',
        harness: 'codex-pro',
        providerLabel: 'Codex',
        sessionId: 'codex-thread-9',
        cwd: '/repo',
        command: "cd '/repo' && codex resume 'codex-thread-9'",
      },
      errorCode: 'TIMEOUT',
      errorSummary: 'Unit test timed out',
    },
    {
      id: 'call-dsh-2',
      key: 'design-review/planning:engine-1-1',
      role: 'design-review',
      scope: 'planning:engine-1-1',
      attempt: 1,
      harness: 'dsh',
      model: 'gpt-6.1-sol',
      cwd: '/repo',
      status: 'completed',
      running: false,
      exitCode: 0,
      durationMs: 8500,
      sessionId: 'flywheel-dsh-session-1',
      sessionTitle: 'Flywheel · design-review · Architecture Audit',
      resume: {
        kind: 'dsh-native',
        harness: 'dsh',
        providerLabel: 'DSH Desktop',
        sessionId: 'flywheel-dsh-session-1',
        cwd: '/repo',
        command: '',
      },
    },
  ];

  const details = {
    'call-fail-1': {
      ...traces[0],
      blocks: [
        { kind: 'thinking', text: 'Checking lock contention in worker.go' },
        { kind: 'tool_call', title: 'Bash', text: 'go test ./...' },
        { kind: 'error', title: 'stderr', text: 'FAIL TestWorkerTimeout' },
      ],
      rawTail: ['{"type":"error","message":"timeout"}'],
    },
    'call-dsh-2': {
      ...traces[1],
      blocks: [
        { kind: 'thinking', text: 'Reviewing architecture boundaries' },
        { kind: 'assistant', text: 'Design looks solid with one advisory note.' },
      ],
      rawTail: [],
    },
  };

  const ui = await mountWorkbench(
    t,
    {
      query: async input => {
        if (input.action === 'runs') {
          return envelope({ protocol: 1, ok: true, cwd: '/repo', runs: [{ taskDir: '/repo/docs/t1', cwd: '/repo', slug: 't1', status: 'running', running: true }] });
        }
        if (input.action === 'status') {
          return envelope({
            protocol: 1,
            ok: true,
            cwd: '/repo',
            taskDir: '/repo/docs/t1',
            running: true,
            state: {
              slug: 't1',
              status: 'running',
              revision: 4,
              packets: { 'M01-T01': { id: 'M01-T01', goal: 'Fix worker timeout', status: 'failed', route: 'hard' } },
            },
          });
        }
        if (input.action === 'traces') {
          return envelope({ protocol: 1, ok: true, taskDir: '/repo/docs/t1', traces });
        }
        if (input.action === 'trace-detail') {
          return envelope({ protocol: 1, ok: true, taskDir: '/repo/docs/t1', trace: details[input.traceId] ?? null });
        }
        if (input.action === 'history') {
          return envelope({ protocol: 1, ok: true, taskDir: '/repo/docs/t1', history: [] });
        }
        return envelope({ protocol: 1, ok: true, cwd: '/repo' });
      },
      act: async input => {
        if (input.action === 'open-terminal') {
          return envelope({ protocol: 1, ok: true, terminal: 'warp', harness: input.harness, sessionId: input.sessionId });
        }
        return envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir: input.taskDir, running: false });
      },
    },
    {
      services: {
        uiWorkspace: { openSession: id => openedSessions.push(id) },
      },
    },
  );

  // 1. Click M01-T01 card in Main Stream -> drills straight into SubSessionView
  await act(async () => {
    ui.container.querySelector('[data-testid="packet-card-M01-T01"]').click();
    await Promise.resolve();
    await Promise.resolve();
  });
  assert.ok(ui.container.querySelector('[data-testid="flywheel-sub-session"]'));
  assert.match(ui.container.textContent, /Checking lock contention in worker\.go/);
  assert.match(ui.container.textContent, /FAIL TestWorkerTimeout/);
  assert.match(ui.container.querySelector('[data-testid="flywheel-session-strip"]').textContent, /Flywheel · execute · Fix worker timeout/);

  // 2. Open top-right Provider Session Popover -> verifies sessionName, sessionId, CLI command, and Warp launch
  await act(async () => {
    ui.container.querySelector('[data-testid="flywheel-session-capsule"]').click();
  });
  const pop = ui.container.querySelector('[data-testid="flywheel-session-popover"]');
  assert.ok(pop);
  assert.match(pop.textContent, /Flywheel · execute · Fix worker timeout/);
  assert.match(pop.querySelector('[data-testid="session-popover-id"]').textContent, /codex-thread-9/);
  assert.match(pop.querySelector('[data-testid="session-popover-cmd"]').textContent, /codex resume 'codex-thread-9'/);

  await act(async () => {
    pop.querySelector('[data-testid="session-open-warp"]').click();
    await Promise.resolve();
  });
  const termAct = ui.actCalls.at(-1);
  assert.equal(termAct.action, 'open-terminal');
  assert.equal(termAct.harness, 'codex-pro');
  assert.equal(termAct.sessionId, 'codex-thread-9');
  assert.equal(termAct.sessionTitle, 'Flywheel · execute · Fix worker timeout');

  // 3. Interrupt & Take Over -> claims session, sends message, then Hands Back to Engine
  await act(async () => {
    ui.container.querySelector('[data-testid="flywheel-interrupt-takeover"]').click();
    await Promise.resolve();
    await Promise.resolve();
  });
  assert.ok(ui.actCalls.some(c => c.action === 'stop'));
  assert.ok(ui.actCalls.some(c => c.action === 'session' && c.session?.action === 'claim' && c.session?.key === 'execute/M01-T01'));
  assert.ok(ui.container.querySelector('[data-testid="flywheel-takeover-strip"]'));

  await act(async () => {
    ui.container.querySelector('[data-testid="flywheel-hand-back"]').click();
    await Promise.resolve();
    await Promise.resolve();
  });
  assert.ok(ui.actCalls.some(c => c.action === 'session' && c.session?.action === 'return' && c.session?.key === 'execute/M01-T01'));

  // 4. Use top-left Agent-Team Switcher to jump to DSH native review session and open in DSH Desktop
  await act(async () => {
    ui.container.querySelector('[data-testid="flywheel-agent-switcher"]').click();
  });
  const agentPop = ui.container.querySelector('[data-testid="flywheel-agent-popover"]');
  assert.ok(agentPop);
  const dshItem = [...agentPop.querySelectorAll('button')].find(
    b => b.textContent.includes('设计架构审查') || b.textContent.includes('design-review'),
  );
  assert.ok(dshItem);
  await act(async () => {
    dshItem.click();
    await Promise.resolve();
    await Promise.resolve();
  });
  assert.match(ui.container.textContent, /Reviewing architecture boundaries/);
  await act(async () => {
    ui.container.querySelector('[data-testid="flywheel-session-capsule"]').click();
  });
  await act(async () => {
    ui.container.querySelector('[data-testid="session-open-dsh"]').click();
  });
  assert.deepEqual(openedSessions, ['flywheel-dsh-session-1']);
});

test('trace-parser coalesces streamed progress deltas, collapses activity steps, and incrementally unwraps JSON document markdown', async t => {
  const taskDir = await mkdtemp(join(tmpdir(), 'fw-stream-unwrap-'));
  t.after(() => rm(taskDir, { recursive: true, force: true }));
  await mkdir(join(taskDir, '.traces'), { recursive: true });

  // Streaming incomplete JSON document unwrapping (pure JSON + preamble + embedded JSON)
  const partial = unwrapStreamingContractMarkdown('{"document": "# Pixel Tetris\\n\\n```ts\\nconst x = 1;');
  assert.ok(partial);
  assert.equal(partial.markdown, '# Pixel Tetris\n\n```ts\nconst x = 1;');
  const withPreamble = unwrapStreamingContractMarkdown('先只读核对仓库与工具链现状。{"document":"# 俄罗斯方块\\n\\n## 1. 概述"}');
  assert.ok(withPreamble);
  assert.equal(withPreamble.markdown, '先只读核对仓库与工具链现状。\n\n# 俄罗斯方块\n\n## 1. 概述');

  const lines = [
    JSON.stringify({
      kind: 'start',
      ts: '2026-11-24T10:00:00Z',
      invocationId: 'call-stream-1',
      key: 'design-plan/planning:r1:primary',
      role: 'design-plan',
      scope: 'planning:r1:primary',
      harness: 'antigravity',
      model: 'gemini-4-carbon-high',
      cwd: '/repo',
    }),
    JSON.stringify({ kind: 'event', ts: '2026-11-24T10:00:01Z', line: JSON.stringify({ type: 'activity', activity: { source: 'antigravity', eventTypes: ['agent_thought_chunk'] } }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-24T10:00:02Z', line: JSON.stringify({ type: 'progress', text: '{"document": "# 俄罗' }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-24T10:00:02Z', line: JSON.stringify({ type: 'activity', activity: { source: 'antigravity', eventTypes: ['agent_message_chunk'] } }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-24T10:00:03Z', line: JSON.stringify({ type: 'progress', text: '斯方块\\n\\n- 经典 10x20 棋盘' }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-24T10:00:03Z', line: JSON.stringify({ type: 'activity', activity: { source: 'antigravity', eventTypes: ['agent_message_chunk'] } }) }),
  ].join('\n') + '\n';
  await writeFile(join(taskDir, '.traces', 'live-call-stream-1.jsonl'), lines);

  const detail = await readTaskTraceDetail(taskDir, 'call-stream-1');
  assert.equal(detail.trace.blocks.length, 2);
  assert.equal(detail.trace.blocks[0].kind, 'steps');
  assert.deepEqual(detail.trace.blocks[0].items, ['[antigravity] 思考推理中', '[antigravity] 生成回复中 × 2']);
  assert.equal(detail.trace.blocks[1].kind, 'assistant');
  assert.equal(detail.trace.blocks[1].text, '# 俄罗斯方块\n\n- 经典 10x20 棋盘');
});

test('save-attachment validates image magic bytes, saves into .flywheel-attachments, and rejects non-images', async t => {
  const repoDir = await mkdtemp(join(tmpdir(), 'fw-attach-'));
  t.after(() => rm(repoDir, { recursive: true, force: true }));
  const ctx = new Context();
  t.after(() => ctx.fiber.dispose());
  const svc = new ConsoleReadService(ctx, async () => ({}), (_in, err) => ({ ok: false, message: err.message }), repoDir);

  // Valid minimal PNG header
  const pngHeader = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d]).toString('base64');
  const okRes = JSON.parse((await svc.act({ action: 'save-attachment', cwd: repoDir, name: 'mockup.png', dataBase64: `data:image/png;base64,${pngHeader}` })).result.text);
  assert.equal(okRes.ok, true);
  assert.equal(okRes.mimeType, 'image/png');
  assert.match(okRes.path, /^\.flywheel-attachments\/.*mockup\.png$/);

  // Invalid non-image payload is rejected
  const badRes = JSON.parse((await svc.act({ action: 'save-attachment', cwd: repoDir, name: 'evil.sh', dataBase64: Buffer.from('#!/bin/sh\nrm -rf /').toString('base64') })).result.text);
  assert.equal(badRes.ok, false);
  assert.match(badRes.message, /Unsupported image format/);
});

test('Dual-Draft partial failure renders explicit user decision card without silent downgrade', async t => {
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({ protocol: 1, ok: true, cwd: '/repo', runs: [{ taskDir: '/repo/docs/t1', cwd: '/repo', slug: 't1', status: 'paused', running: false }] });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: '/repo/docs/t1',
          running: false,
          state: {
            slug: 't1',
            status: 'paused',
            revision: 3,
            planning: {
              phase: 'awaiting-input',
              reason: 'dual-draft partial completion requires user decision',
              brief: { goal: 'Build Pixel Tetris' },
            },
          },
        });
      }
      if (input.action === 'design-candidates') {
        return envelope({
          protocol: 1,
          ok: true,
          taskDir: '/repo/docs/t1',
          revisions: [{
            revision: 1,
            primary: '# Carbon Draft',
            secondary: '',
            meta: {
              partial: true,
              completedSide: 'primary',
              failedSide: 'secondary',
              failureCode: 'AUTH',
              failureError: 'AUTH: claude failed',
              primary: { model: 'gemini-4-carbon-high' },
              secondary: { model: 'claude-opus-5-5' },
            },
          }],
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', traces: [] });
    },
    act: async input => envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir: input.taskDir, running: true }),
  });

  const card = ui.container.querySelector('[data-testid="main-dual-draft-partial"]');
  assert.ok(card, 'expected explicit partial dual-draft decision card in Main Stream');
  assert.match(card.textContent, /gemini-4-carbon-high/);
  assert.match(card.textContent, /AUTH: claude failed/);

  await act(async () => {
    card.querySelector('[data-testid="main-partial-proceed-single"]').click();
    await Promise.resolve();
  });
  const actCall = ui.actCalls.at(-1);
  assert.equal(actCall.action, 'planning');
  assert.equal(actCall.planning.action, 'plan');
  assert.equal(actCall.planning['draft-mode'], 'single');
});

test('Stage 3 ChatStreamView and WorkbenchPage normalize Go state.json packet maps without inline id fields', async t => {
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          runs: [{ slug: 'tetris', taskDir: '/repo/docs/tetris', status: 'running', running: true }],
        });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: '/repo/docs/tetris',
          running: true,
          state: {
            slug: 'tetris',
            status: 'running',
            revision: 12,
            packets: {
              'M01-T01': {
                status: 'running',
                schedule: { after: [], writes: ['package.json'] },
                'execution-policy': { current: 'hydrogen-local', currentBinding: { binding: { model: 'gemini-4-hydrogen-medium' } } },
              },
              'M01-T02': {
                status: 'pending',
                schedule: { after: ['M01-T01'], writes: ['src/core/'] },
              },
            },
          },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', traces: [] });
    },
    act: async input => envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir: input.taskDir, running: true }),
  });

  const html = ui.container.innerHTML;
  assert.match(html, /M01-T01/);
  assert.match(html, /M01-T02/);
  assert.match(html, /Gemini 4 Hydrogen 中/);
  assert.match(html, /gemini-4-hydrogen-medium/);
});

test('Main Stream renders Stage 3 packet execute/review micro-timeline and places mid-execution plan revisions chronologically below waves', async t => {
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          runs: [{ slug: 'tetris', taskDir: '/repo/docs/tetris', status: 'running', running: true }],
        });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: '/repo/docs/tetris',
          running: true,
          state: {
            slug: 'tetris',
            status: 'running',
            revision: 40,
            planning: {
              phase: 'approved',
              requests: [
                { id: 'engine-1-5', scope: 'milestone', status: 'approved' },
                { id: 'engine-1-11', scope: 'milestone', status: 'planning', feedback: 'Returned packet M01-T02: {"review":{"reasons":["missing exports in src/core/engine.ts"]}}' },
              ],
            },
            packets: {
              'M01-T02': {
                status: 'review-failed',
                schedule: { after: [], writes: ['src/core/'] },
              },
            },
          },
        });
      }
      if (input.action === 'traces') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          traces: [
            { id: 'call-plan-rev', key: 'plan/planning:engine-1-11', role: 'plan', scope: 'planning:engine-1-11', status: 'running', running: true, startedAt: '2026-11-25T16:00:00Z', model: 'gemini-4-carbon-high' },
            { id: 'call-rev-2', key: 'review/M01-T02', role: 'review', scope: 'M01-T02', status: 'completed', running: false, startedAt: '2026-11-25T15:30:00Z', durationMs: 60000, model: 'gpt-6.2-sol', preview: '**审查结论**: `return` · **评分**: `0.76` missing exports' },
            { id: 'call-exec-2', key: 'execute/M01-T02', role: 'execute', scope: 'M01-T02', status: 'completed', running: false, startedAt: '2026-11-25T15:00:00Z', durationMs: 120000, model: 'gemini-4-neon-high' },
            { id: 'call-rev-1', key: 'review/M01-T02', role: 'review', scope: 'M01-T02', status: 'completed', running: false, startedAt: '2026-11-25T14:30:00Z', durationMs: 45000, model: 'gpt-6.2-sol', preview: '**审查结论**: `return` · **评分**: `0.68`' },
            { id: 'call-exec-1', key: 'execute/M01-T02', role: 'execute', scope: 'M01-T02', status: 'completed', running: false, startedAt: '2026-11-25T14:00:00Z', durationMs: 90000, model: 'gemini-4-neon-high' },
            { id: 'call-plan-init', key: 'plan/planning:engine-1-5', role: 'plan', scope: 'planning:engine-1-5', status: 'completed', running: false, startedAt: '2026-11-25T12:00:00Z', durationMs: 80000, model: 'gemini-4-carbon-high' },
          ],
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', traces: [] });
    },
    act: async input => envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir: input.taskDir, running: true }),
  });

  const timeline = ui.container.querySelector('[data-testid="packet-timeline-M01-T02"]');
  assert.ok(timeline, 'expected embedded execute/review micro-timeline inside M01-T02 packet card');
  assert.match(timeline.textContent, /执行 #1.*审查 #1.*0\.68.*执行 #2.*审查 #2.*0\.76/);
  assert.ok(timeline.querySelector('[data-testid="packet-revision-link-engine-1-11"]'), 'expected plan revision chip linked on packet timeline');

  const revSection = ui.container.querySelector('[data-testid="mid-exec-plan-revisions"]');
  assert.ok(revSection, 'expected mid-execution plan revision section');
  assert.match(revSection.textContent, /engine-1-11/);
  assert.match(revSection.textContent, /M01-T02/);
  assert.match(revSection.textContent, /missing exports/);

  // Verify chronological DOM order: initial planning -> Stage 3 wave -> mid-execution plan revision
  const fullHtml = ui.container.innerHTML;
  const idxInitPlan = fullHtml.indexOf('agent-card-call-plan-init');
  const idxWave = fullHtml.indexOf('flywheel-wave-1');
  const idxMidRev = fullHtml.indexOf('mid-exec-plan-revisions');
  assert.ok(idxInitPlan > 0 && idxWave > idxInitPlan && idxMidRev > idxWave, 'mid-execution plan revisions must appear chronologically below Stage 3 waves');
});

test('Model space grammar, 3-pill Telemetry Dock (Main & SubSession), and @ mention autocomplete with atomic pill deletion', async t => {
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          runs: [{ slug: 'tetris', taskDir: '/repo/docs/tetris', status: 'running', running: true }],
        });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: '/repo/docs/tetris',
          running: false,
          state: {
            slug: 'tetris',
            status: 'running',
            revision: 42,
            budget: {
              totals: { calls: 12, tokens: 650000, 'cost-usd': 1.25 },
              cache: {
                'by-provider': {
                  openai: { calls: 6, input: 500000, 'cache-read': 481000, 'cache-write': 0, 'hit-rate': 0.962 },
                },
              },
            },
            sessions: {
              records: {
                'review/M01-T01': { action: 'resume', quality: { 'confirmed-turns': 3 }, context: { mode: 'full' } },
              },
            },
            packets: {
              'M01-T01': { status: 'completed', title: 'Core Engine', schedule: { after: [], writes: ['src/core/'] }, model: 'gpt-6.2-sol', thinking: 'xhigh' },
              'M01-T02': { status: 'running', title: 'Canvas Renderer', schedule: { after: ['M01-T01'], writes: ['src/ui/'] }, model: 'gemini-4-carbon-high' },
            },
          },
        });
      }
      if (input.action === 'traces') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          traces: [
            {
              id: 'call-rev-1',
              key: 'review/M01-T01',
              role: 'review',
              scope: 'M01-T01',
              harness: 'codex-pro',
              model: 'gpt-6.2-sol',
              thinking: 'xhigh',
              status: 'completed',
              running: false,
              durationMs: 42000,
              promptBytes: 14200,
              usage: { input: 100000, cacheRead: 96200, output: 1800, totalTokens: 101800 },
              startedAt: '2026-11-26T00:00:00Z',
            },
            {
              id: 'call-plan-1',
              key: 'plan/planning:milestone:M-01',
              role: 'plan',
              scope: 'planning:milestone:M-01',
              harness: 'antigravity',
              model: 'gemini-4-carbon-high',
              status: 'completed',
              running: false,
              durationMs: 90000,
              contextUsage: { used: 58000, size: 200000 },
              startedAt: '2026-11-25T23:50:00Z',
            },
          ],
        });
      }
      if (input.action === 'trace-detail') {
        const isRev = input.traceId === 'call-rev-1';
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          trace: isRev
            ? {
                id: 'call-rev-1',
                key: 'review/M01-T01',
                role: 'review',
                scope: 'M01-T01',
                harness: 'codex-pro',
                model: 'gpt-6.2-sol',
                thinking: 'xhigh',
                status: 'completed',
                running: false,
                durationMs: 42000,
                promptBytes: 14200,
                usage: { input: 100000, cacheRead: 96200, output: 1800, totalTokens: 101800 },
                blocks: [{ kind: 'assistant', text: 'All checks pass.' }],
              }
            : {
                id: 'call-plan-1',
                key: 'plan/planning:milestone:M-01',
                role: 'plan',
                scope: 'planning:milestone:M-01',
                harness: 'antigravity',
                model: 'gemini-4-carbon-high',
                status: 'completed',
                running: false,
                durationMs: 90000,
                contextUsage: { used: 58000, size: 200000 },
                blocks: [{ kind: 'assistant', text: 'Plan ready.' }],
              },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', traces: [] });
    },
    act: async input => envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir: input.taskDir, running: false }),
  });

  // 1. Space-formatted model display names on Main Stream
  assert.match(ui.container.textContent, /GPT-6\.2 Sol 超高/);
  assert.match(ui.container.textContent, /Gemini 4 Carbon 高/);

  // 2. Main Stream 3-Pill Telemetry Dock & upward hover/click popover with provider breakdown
  const mainDock = ui.container.querySelector('[data-testid="flywheel-main-telemetry-dock"]');
  assert.ok(mainDock, 'expected Main Stream 3-pill telemetry dock');
  assert.match(mainDock.textContent, /12 次调用/);
  assert.match(mainDock.textContent, /缓存命中 96\.2%/);
  await act(async () => {
    ui.container.querySelector('[data-testid="telemetry-main-cache"]').click();
  });
  const mainPop = ui.container.querySelector('[data-testid="telemetry-main-cache-popover"]');
  assert.ok(mainPop, 'expected upward cache popover on Main Stream');
  assert.match(mainPop.textContent, /openai/);
  assert.match(mainPop.textContent, /96\.2%/);

  // 3. @ Mention trigger -> fuzzy popover -> select atomic blue pill -> single Backspace removes whole pill
  await act(async () => {
    ui.container.querySelector('[data-testid="flywheel-mention-trigger"]').click();
  });
  const mentionPop = ui.container.querySelector('[data-testid="flywheel-mention-popover"]');
  assert.ok(mentionPop, 'expected upward @ mention popover');
  assert.match(mentionPop.textContent, /M01-T02/);
  assert.match(mentionPop.textContent, /planning:milestone:M-01|milestone:M-01/);
  await act(async () => {
    const opt = ui.container.querySelector('[data-testid="mention-option-M01-T02"]');
    opt.dispatchEvent(new window.MouseEvent('mousedown', { bubbles: true, cancelable: true }));
  });
  const pill = ui.container.querySelector('[data-testid="flywheel-mention-pill"]');
  assert.ok(pill, 'expected atomic blue mention pill');
  assert.match(pill.textContent, /@M01-T02/);

  const textarea = ui.container.querySelector('[data-testid="flywheel-main-composer"]');
  textarea.selectionStart = 0;
  textarea.selectionEnd = 0;
  await act(async () => {
    textarea.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Backspace', bubbles: true, cancelable: true }));
  });
  assert.equal(ui.container.querySelector('[data-testid="flywheel-mention-pill"]'), null, 'single Backspace at caret 0 must delete the entire pill atomically');

  // 4. Drill into Codex review node -> SubSession telemetry dock shows 3 轮 · 42.0s (no duplicate model badge) & 96.2% cache hit; Pill 1 popover carries full model + raw ID
  await act(async () => {
    ui.container.querySelector('[data-testid="packet-step-call-rev-1"]').click();
    await Promise.resolve();
    await Promise.resolve();
  });
  const subDock = ui.container.querySelector('[data-testid="flywheel-sub-telemetry-dock"]');
  assert.ok(subDock, 'expected SubSession 3-pill telemetry dock');
  assert.match(subDock.textContent, /3 轮 · 42\.0s/);
  assert.doesNotMatch(subDock.textContent, /GPT-6\.2 Sol 超高/, 'SubSession Pill 1 must not duplicate the model badge already shown in composer bar');
  assert.match(subDock.textContent, /缓存命中 96\.2%/);
  await act(async () => {
    ui.container.querySelector('[data-testid="telemetry-sub-model"]').click();
  });
  const subModelPop = ui.container.querySelector('[data-testid="telemetry-sub-model-popover"]');
  assert.ok(subModelPop);
  assert.match(subModelPop.textContent, /GPT-6\.2 Sol 超高/);
  assert.match(subModelPop.textContent, /gpt-6\.2-sol/);
  await act(async () => {
    ui.container.querySelector('[data-testid="telemetry-sub-cache"]').click();
  });
  const subPop = ui.container.querySelector('[data-testid="telemetry-sub-cache-popover"]');
  assert.ok(subPop);
  assert.match(subPop.textContent, /96,200/);
});
