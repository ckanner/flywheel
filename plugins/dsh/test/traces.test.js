import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { buildResumeInfo, readTaskTraceDetail, readTaskTraces } from '../src/trace-parser.js';
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
});

test('TracesView and NodeInspector Output tab render live reasoning, failed diagnostics, DSH native jump, and CLI terminal resume', async t => {
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
        return envelope({ protocol: 1, ok: true, cwd: '/repo' });
      },
    },
    {
      services: {
        uiWorkspace: { openSession: id => openedSessions.push(id) },
      },
    },
  );

  // 1. Open packet inspector -> Model Output tab shows failed Codex trace + resume command
  await act(async () => {
    ui.container.querySelector('button[data-packet-id="M01-T01"]')?.click() ??
      [...ui.container.querySelectorAll('button')].find(b => b.textContent.includes('M01-T01')).click();
    await Promise.resolve();
    await Promise.resolve();
  });
  const outputTab = [...ui.container.querySelectorAll('button')].find(b => b.textContent.includes('模型输出'));
  assert.ok(outputTab);
  await act(async () => {
    outputTab.click();
    await Promise.resolve();
    await Promise.resolve();
  });
  assert.match(ui.container.textContent, /Checking lock contention in worker\.go/);
  assert.match(ui.container.textContent, /FAIL TestWorkerTimeout/);
  assert.match(ui.container.querySelector('[data-testid="resume-cmd-preview"]').textContent, /codex resume 'codex-thread-9'/);

  // Trigger open-terminal act
  await ui.click('在终端打开 Resume');
  const termAct = ui.actCalls.at(-1);
  assert.equal(termAct.action, 'open-terminal');
  assert.equal(termAct.harness, 'codex-pro');
  assert.equal(termAct.sessionId, 'codex-thread-9');

  // 2. Jump from inspector into full-screen Live Traces tab, switch to DSH trace, and open in DSH Desktop
  await ui.click('在运行记录中全屏查看 ↗');
  assert.ok(ui.container.querySelector('[data-testid="flywheel-traces-view"]'));
  const dshBtn = ui.container.querySelector('button[data-trace-id="call-dsh-2"]');
  assert.ok(dshBtn);
  await act(async () => {
    dshBtn.click();
    await Promise.resolve();
    await Promise.resolve();
  });
  assert.match(ui.container.textContent, /Reviewing architecture boundaries/);
  await ui.click('在 DSH 中打开会话 ↗');
  assert.deepEqual(openedSessions, ['flywheel-dsh-session-1']);
});
