import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { readTaskTraceDetail, readTaskTraces } from '../src/trace-parser.js';
import { validPlanningCommand } from '../src/planning-control.js';

test('trace-parser never duplicates truncated tail end.text below streamed assistant contract', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'fw-trace-dedup-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  await mkdir(join(dir, '.traces'), { recursive: true });
  const contractJson = JSON.stringify({
    document: '# Pixel Tetris Architecture\n\n- Clean 60Hz pure engine\n- 100% unit test coverage',
    'implementation-scope': ['src'],
    requirements: [{ id: 'R-01', title: 'Board', milestone: 'M-01', verification: 'tests' }],
    decisions: [],
    milestones: [{ id: 'M-01', title: 'Core', gate: '100% cov' }],
  });
  const lines = [
    JSON.stringify({ kind: 'start', ts: '2026-11-25T00:00:00Z', invocationId: 'inv-dedup', key: 'design-plan/planning:engine-1-1:primary', role: 'design-plan', scope: 'planning:engine-1-1:primary', harness: 'antigravity', model: 'gemini-4-carbon-high' }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T00:00:01Z', line: JSON.stringify({ type: 'progress', text: contractJson }) }),
    JSON.stringify({ kind: 'end', ts: '2026-11-25T00:00:05Z', exitCode: 0, text: '… [truncated]\nntHighScore)\\n - **broken tail**' }),
  ];
  await writeFile(join(dir, '.traces', 'live-inv-dedup.jsonl'), lines.join('\n') + '\n', 'utf8');

  const detailRes = await readTaskTraceDetail(dir, 'inv-dedup');
  assert.equal(detailRes.ok, true);
  const assistantBlocks = detailRes.trace.blocks.filter(b => b.kind === 'assistant');
  assert.equal(assistantBlocks.length, 1, 'must NOT append truncated end.text as a second block');
  assert.match(assistantBlocks[0].text, /Pixel Tetris Architecture/);
  assert.doesNotMatch(assistantBlocks[0].text, /broken tail/);
});

test('trace-parser parses Codex rollout reasoning and tool_call items cleanly', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'fw-trace-codex-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  await mkdir(join(dir, '.traces'), { recursive: true });
  const lines = [
    JSON.stringify({ kind: 'start', ts: '2026-11-25T00:00:00Z', invocationId: 'inv-codex', key: 'design-review/planning:engine-1-1', role: 'design-review', scope: 'planning:engine-1-1', harness: 'codex-pro', model: 'gpt-6.2-astra' }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T00:00:01Z', line: JSON.stringify({ type: 'response_item', timestamp: '2026-11-25T00:00:01Z', payload: { type: 'reasoning', summary: [{ text: 'Checking SRS wall kick table invariants.' }] } }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T00:00:02Z', line: JSON.stringify({ type: 'response_item', timestamp: '2026-11-25T00:00:02Z', payload: { type: 'custom_tool_call', name: 'exec', input: 'ls -la src' } }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T00:00:03Z', line: JSON.stringify({ type: 'response_item', timestamp: '2026-11-25T00:00:03Z', payload: { type: 'custom_tool_call_output', call_id: 'c1', output: [{ text: 'engine.ts\nrender.ts' }] } }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T00:00:04Z', line: JSON.stringify({ type: 'progress', text: JSON.stringify({ verdict: 'pass', summary: 'Architecture verified', findings: [] }) }) }),
    JSON.stringify({ kind: 'end', ts: '2026-11-25T00:00:05Z', exitCode: 0, text: '' }),
  ];
  await writeFile(join(dir, '.traces', 'live-inv-codex.jsonl'), lines.join('\n') + '\n', 'utf8');

  const list = await readTaskTraces(dir);
  assert.equal(list.traces.length, 1);
  const detail = await readTaskTraceDetail(dir, 'inv-codex');
  assert.equal(detail.trace.blocks.some(b => b.kind === 'thinking' && b.text.includes('SRS wall kick')), true);
  assert.equal(detail.trace.blocks.some(b => b.kind === 'tool_call' && b.title.includes('exec')), true);
  assert.equal(detail.trace.blocks.some(b => b.kind === 'tool_result' && b.text.includes('engine.ts')), true);
  assert.equal(detail.trace.blocks.some(b => b.kind === 'steps'), false, 'noisy activity bullets suppressed when rich blocks exist');
});

test('trace-parser parses Antigravity PLANNER_RESPONSE, GENERIC tool outputs, lifecycle.threadId, and Stage 0 clarification JSON', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'fw-trace-agy-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  await mkdir(join(dir, '.traces'), { recursive: true });
  const clarifyJson = JSON.stringify({
    'needs-clarification': true,
    summary: '仓库为空白项目，需确认规则与门禁。',
    'suggested-gates': ['核心引擎单元测试覆盖率 100%'],
    questions: [{ id: 'Q-1', title: '规则体系', options: ['SRS（推荐）', 'NES'], recommended: 'SRS（推荐）' }],
  });
  const lines = [
    JSON.stringify({ kind: 'start', ts: '2026-11-25T00:00:00Z', invocationId: 'inv-agy', key: 'clarify/stage0', role: 'clarify', scope: 'stage0', harness: 'antigravity', model: 'gemini-4-carbon-high' }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T00:00:01Z', line: JSON.stringify({ type: 'lifecycle', lifecycle: { event: 'thread-ready', threadId: 'thr-live-123' } }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T00:00:02Z', line: JSON.stringify({ type: 'PLANNER_RESPONSE', created_at: '2026-11-25T00:00:02Z', thinking: 'Inspecting target repo first.', tool_calls: [{ name: 'read_file', args: { Path: '"/repo/README.md"' } }] }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T00:00:03Z', line: JSON.stringify({ type: 'GENERIC', created_at: '2026-11-25T00:00:03Z', content: 'Created At: 2026-11-25\nCompleted At: 2026-11-25\n# Tetris' }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T00:00:04Z', line: JSON.stringify({ type: 'progress', text: clarifyJson }) }),
    JSON.stringify({ kind: 'end', ts: '2026-11-25T00:00:05Z', exitCode: 0, text: '' }),
  ];
  await writeFile(join(dir, '.traces', 'live-inv-agy.jsonl'), lines.join('\n') + '\n', 'utf8');

  const detail = await readTaskTraceDetail(dir, 'inv-agy');
  assert.equal(detail.trace.sessionId, 'thr-live-123');
  assert.equal(detail.trace.blocks.some(b => b.kind === 'thinking' && b.text.includes('Inspecting target repo')), true);
  assert.equal(detail.trace.blocks.some(b => b.kind === 'tool_call' && b.title === 'read_file · README.md'), true);
  assert.equal(detail.trace.blocks.some(b => b.kind === 'tool_result' && b.text === '# Tetris'), true);
  const assistant = detail.trace.blocks.find(b => b.kind === 'assistant');
  assert.ok(assistant, 'assistant block must exist');
  assert.match(assistant.text, /建议可量化质量门禁/);
  assert.match(assistant.text, /核心引擎单元测试覆盖率 100%/);
});

test('trace-parser filters Codex worker status progress, formats JS exec tools, extracts commentary thinking, and unwraps plan & review JSON', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'fw-trace-codex-clean-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  await mkdir(join(dir, '.traces'), { recursive: true });
  const reviewVerdict = JSON.stringify({
    verdict: 'revise',
    'scope-verdict': 'within-envelope',
    summary: '核对完成，D-9 仍需收紧外围层依赖约束。',
    findings: [{ id: 'D-9', severity: 'high', title: '核心层依赖边界', summary: '外围层不可在运行时导入核心类型模块', recommendation: '改为 import type' }],
  });
  const lines = [
    JSON.stringify({ kind: 'start', ts: '2026-11-25T12:49:49Z', invocationId: 'inv-codex-clean', key: 'plan-review/planning:engine-1-3', role: 'plan-review', scope: 'planning:engine-1-3', harness: 'codex-pro', model: 'gpt-6.2-sol' }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T12:49:49Z', line: JSON.stringify({ type: 'progress', text: 'thread 01a212bc-4eab-7d40-993e-f8d9f8e49725 (resumed) configuration matched=true' }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T12:49:50Z', line: JSON.stringify({ type: 'progress', text: 'userMessage' }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T12:49:51Z', line: JSON.stringify({ type: 'progress', text: '我会重新核对目录和基线，逐项验证第 2 版修订。' }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T12:49:52Z', line: JSON.stringify({ type: 'progress', text: 'commandExecution completed: /bin/zsh -lc "pwd && git branch --show-current"' }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T12:49:53Z', line: JSON.stringify({ type: 'response_item', timestamp: '2026-11-25T12:49:53Z', payload: { type: 'custom_tool_call', name: 'exec', input: 'text(await tools.exec_command({cmd:"pwd && git branch --show-current"}))' } }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T12:49:54Z', line: JSON.stringify({ type: 'response_item', timestamp: '2026-11-25T12:49:54Z', payload: { type: 'custom_tool_call', name: 'exec', input: 'text(await tools.web__run({search_query:[{q:"site:vitest.dev config coverage"}]}))' } }) }),
    JSON.stringify({ kind: 'end', ts: '2026-11-25T12:50:51Z', exitCode: 0, text: reviewVerdict }),
  ];
  await writeFile(join(dir, '.traces', 'live-inv-codex-clean.jsonl'), lines.join('\n') + '\n', 'utf8');

  const detail = await readTaskTraceDetail(dir, 'inv-codex-clean');
  const thinking = detail.trace.blocks.filter(b => b.kind === 'thinking');
  const tools = detail.trace.blocks.filter(b => b.kind === 'tool_call');
  const assistant = detail.trace.blocks.filter(b => b.kind === 'assistant');
  assert.equal(thinking.some(b => b.text.includes('我会重新核对目录和基线')), true);
  assert.equal(tools[0].title, 'Shell · pwd && git branch --show-current');
  assert.equal(tools[1].title, 'Web Search · site:vitest.dev config coverage');
  assert.equal(assistant.length, 1);
  assert.doesNotMatch(assistant[0].text, /configuration matched=true|commandExecution completed/);
  assert.match(assistant[0].text, /\*\*审查结论\*\*: `revise`/);
  assert.match(assistant[0].text, /D-9/);
  assert.match(assistant[0].text, /建议修复/);

  // Plan JSON with `files` unwraps into readable Markdown sections
  const planJson = JSON.stringify({ files: { 'spec.md': '---\nversion: 1\n---\n# Core Spec\nDetails', 'task-packet-M01-T01.md': '# Packet 1' } });
  const planLines = [
    JSON.stringify({ kind: 'start', ts: '2026-11-25T13:00:00Z', invocationId: 'inv-plan', key: 'plan/planning:engine-1-5', role: 'plan', scope: 'planning:engine-1-5', harness: 'antigravity', model: 'gemini-4-neon-high' }),
    JSON.stringify({ kind: 'end', ts: '2026-11-25T13:01:00Z', exitCode: 0, text: planJson }),
  ];
  await writeFile(join(dir, '.traces', 'live-inv-plan.jsonl'), planLines.join('\n') + '\n', 'utf8');
  const planDetail = await readTaskTraceDetail(dir, 'inv-plan');
  const planOut = planDetail.trace.blocks.find(b => b.kind === 'assistant');
  assert.ok(planOut);
  assert.match(planOut.text, /实施计划拆解产物（共 2 个文件）/);
  assert.match(planOut.text, /```yaml/);
  assert.match(planOut.text, /#### `task-packet-M01-T01\.md`/);

  // Streaming (incomplete) AND malformed-quote `{"files": {...}}` JSON still unwraps cleanly as YAML + Markdown
  const streamingPlanRaw = '{"files":{"spec.md":"---\\nversion: 1\\n---\\n# Live Spec\\nStreaming body","task-packet-M01-T02.md":"---\\npacket-id: M01-T02\\n---\\n# Packet 2\\nRun node -e "console.log(1)" ok';
  const streamPlanLines = [
    JSON.stringify({ kind: 'start', ts: '2026-11-25T13:01:10Z', invocationId: 'inv-plan-stream', key: 'plan/planning:engine-1-11', role: 'plan', scope: 'planning:engine-1-11', harness: 'antigravity', model: 'gemini-4-carbon-high' }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T13:01:20Z', line: JSON.stringify({ type: 'progress', text: streamingPlanRaw }) }),
  ];
  await writeFile(join(dir, '.traces', 'live-inv-plan-stream.jsonl'), streamPlanLines.join('\n') + '\n', 'utf8');
  const streamPlanDetail = await readTaskTraceDetail(dir, 'inv-plan-stream');
  const streamPlanOut = streamPlanDetail.trace.blocks.find(b => b.kind === 'assistant');
  assert.ok(streamPlanOut, 'streaming plan JSON must unwrap into readable assistant markdown');
  assert.match(streamPlanOut.text, /实施计划拆解产物（已解析 2 个文件 · 流式\/容错视图）/);
  assert.match(streamPlanOut.text, /#### `spec\.md`/);
  assert.match(streamPlanOut.text, /#### `task-packet-M01-T02\.md`/);
  assert.match(streamPlanOut.text, /console\.log\(1\)/);
  assert.doesNotMatch(streamPlanOut.text, /^\{"files":/);

  // Packet code-review verdict JSON (`{verdict, score, reasons, advisory}`) unwraps cleanly
  const pktRevJson = JSON.stringify({
    verdict: 'verification-gap',
    score: 0.94,
    reasons: ['阻塞验收缺口：L1 磁盘断言未验证返回契约'],
    advisory: ['缺陷注入记录为 null'],
  });
  const pktLines = [
    JSON.stringify({ kind: 'start', ts: '2026-11-25T13:02:00Z', invocationId: 'inv-pkt-rev', key: 'review/M01-T01', role: 'review', scope: 'M01-T01', harness: 'codex-pro', model: 'gpt-6.2-sol', thinking: 'medium' }),
    JSON.stringify({ kind: 'end', ts: '2026-11-25T13:03:00Z', exitCode: 0, text: pktRevJson }),
  ];
  await writeFile(join(dir, '.traces', 'live-inv-pkt-rev.jsonl'), pktLines.join('\n') + '\n', 'utf8');
  const pktDetail = await readTaskTraceDetail(dir, 'inv-pkt-rev');
  const pktOut = pktDetail.trace.blocks.find(b => b.kind === 'assistant');
  assert.ok(pktOut);
  assert.match(pktOut.text, /\*\*审查结论\*\*: `verification-gap` · \*\*评分\*\*: `0\.94`/);
  assert.match(pktOut.text, /阻塞验收缺口/);
  assert.match(pktOut.text, /建议改进/);
  assert.match(pktOut.text, /缺陷注入记录为 null/);

  // Escape-free Multi-File Plan Protocol (`<<<FLYWHEEL_FILE path="...">>>`) + ACP usage_update + promptBytes
  const fencedPlan = [
    '<<<FLYWHEEL_PLAN_META>>>',
    '{"mode":"patch","keep":["spec.md"]}',
    '<<<END_FLYWHEEL_PLAN_META>>>',
    '<<<FLYWHEEL_FILE path="task-packet-M01-T03.md">>>',
    '---',
    'packet-id: M01-T03',
    '---',
    '# Packet 3 body',
    '<<<END_FLYWHEEL_FILE>>>',
    '<<<FLYWHEEL_FILE path="oracles/verify-M01-T03.mjs">>>',
    'console.log("oracle-ok");',
    '<<<END_FLYWHEEL_FILE>>>',
  ].join('\n');
  const fencedLines = [
    JSON.stringify({ kind: 'start', ts: '2026-11-25T13:05:00Z', invocationId: 'inv-fenced-plan', key: 'plan/planning:milestone:M-01', role: 'plan', scope: 'planning:milestone:M-01', harness: 'antigravity', model: 'gemini-4-carbon-high', promptBytes: 18420 }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T13:05:10Z', line: JSON.stringify({ update: { sessionUpdate: 'usage_update', used: 58000, size: 200000 } }) }),
    JSON.stringify({ kind: 'end', ts: '2026-11-25T13:05:30Z', exitCode: 0, text: fencedPlan }),
  ];
  await writeFile(join(dir, '.traces', 'live-inv-fenced-plan.jsonl'), fencedLines.join('\n') + '\n', 'utf8');
  const fencedDetail = await readTaskTraceDetail(dir, 'inv-fenced-plan');
  assert.equal(fencedDetail.trace.promptBytes, 18420);
  assert.deepEqual(fencedDetail.trace.contextUsage, { used: 58000, size: 200000 });
  const fencedOut = fencedDetail.trace.blocks.find(b => b.kind === 'assistant');
  assert.ok(fencedOut);
  assert.match(fencedOut.text, /实施计划拆解产物（共 2 个文件）/);
  assert.match(fencedOut.text, /#### `task-packet-M01-T03\.md`/);
  assert.match(fencedOut.text, /#### `oracles\/verify-M01-T03\.mjs`/);
  assert.match(fencedOut.text, /```javascript[\s\S]*oracle-ok/);
});

test('trace-parser extracts real user_prompt turns across Antigravity, Codex, and Claude native transcripts without polluting previews', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'fw-trace-prompts-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  await mkdir(join(dir, '.traces'), { recursive: true });
  const longPrompt = '## 审查 M01-T01\n' + '验收细节说明。'.repeat(3000);
  const lines = [
    JSON.stringify({ kind: 'start', ts: '2026-11-25T12:00:00Z', invocationId: 'inv-prompt', key: 'review/M01-T01', role: 'review', scope: 'M01-T01', harness: 'codex-pro', model: 'gpt-6.2-sol', thinking: 'high', cwd: '/wt/M01-T01', workspaceCwd: '/repo' }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T12:00:01Z', line: JSON.stringify({ type: 'event_msg', timestamp: '2026-11-25T12:00:01Z', payload: { type: 'item_completed', item: { type: 'UserMessage', content: [{ type: 'text', text: longPrompt }] } } }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T12:00:02Z', line: JSON.stringify({ type: 'USER_INPUT', created_at: '2026-11-25T12:00:02Z', content: '<USER_REQUEST>\n实现像素风俄罗斯方块核心引擎\n</USER_REQUEST>' }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T12:00:03Z', line: JSON.stringify({ type: 'user', timestamp: '2026-11-25T12:00:03Z', message: { content: '审阅双稿差异并统稿' } }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T12:00:04Z', line: JSON.stringify({ type: 'response_item', timestamp: '2026-11-25T12:00:04Z', payload: { type: 'reasoning', summary: [], encrypted_content: 'gAAAAAB...' } }) }),
    JSON.stringify({ kind: 'event', ts: '2026-11-25T12:00:05Z', line: JSON.stringify({ type: 'assistant', timestamp: '2026-11-25T12:00:05Z', message: { content: [{ type: 'redacted_thinking', data: 'enc' }] } }) }),
    JSON.stringify({ kind: 'end', ts: '2026-11-25T12:01:00Z', exitCode: 0, sessionId: '01a213b6-1111-2222-3333-444455556666', text: JSON.stringify({ verdict: 'pass', score: 0.98, reasons: ['全部通过'] }) }),
  ];
  await writeFile(join(dir, '.traces', 'live-inv-prompt.jsonl'), lines.join('\n') + '\n', 'utf8');

  const detail = await readTaskTraceDetail(dir, 'inv-prompt');
  const prompts = detail.trace.blocks.filter(b => b.kind === 'user_prompt');
  assert.equal(prompts.length, 3);
  assert.match(prompts[0].text, /审查 M01-T01/);
  assert.equal(prompts[0].text.length, longPrompt.length, 'full user_prompt > 16KB must not be truncated');
  assert.doesNotMatch(prompts[0].text, /\[truncated\]/);
  assert.equal(prompts[1].text, '实现像素风俄罗斯方块核心引擎');
  assert.equal(prompts[2].text, '审阅双稿差异并统稿');
  const encryptedThoughts = detail.trace.blocks.filter(b => b.kind === 'thinking' && b.title === 'encrypted');
  assert.equal(encryptedThoughts.length, 2, 'both Codex encrypted_content and Claude redacted_thinking must remain visible in chronological timeline');
  assert.equal(encryptedThoughts[0].ts, '2026-11-25T12:00:04Z');
  assert.equal(encryptedThoughts[1].ts, '2026-11-25T12:00:05Z');
  assert.equal(detail.trace.workspaceCwd, '/repo');
  assert.equal(detail.trace.resume?.cwd, '/repo', 'non-dsh native CLI resume must target stable repo root (workspaceCwd), not ephemeral worktree');
  assert.match(detail.trace.resume?.command || '', /cd ['"]?\/repo['"]? && codex resume/);
  assert.doesNotMatch(detail.trace.preview, /审查 M01-T01/);
});

test('planning-control validates clarify, approve-prototype, prototype-feedback, and skip-prototype', () => {
  assert.equal(validPlanningCommand({
    action: 'clarify', revision: 1, 'request-id': 'cl-1',
    clarify: { answers: { 'Q-1': 'Pixel' }, 'selected-gates': ['100% cov'] }, start: true,
  }), true);
  assert.equal(validPlanningCommand({ action: 'clarify', revision: 1, 'request-id': 'cl-1' }), false);
  assert.equal(validPlanningCommand({ action: 'approve-prototype', revision: 2, 'request-id': 'ap-1', start: true }), true);
  assert.equal(validPlanningCommand({ action: 'prototype-feedback', revision: 2, 'request-id': 'pf-1', text: 'Use retro palette', start: true }), true);
  assert.equal(validPlanningCommand({ action: 'prototype-feedback', revision: 2, 'request-id': 'pf-1', text: '   ' }), false);
  assert.equal(validPlanningCommand({ action: 'skip-prototype', revision: 2, 'request-id': 'sp-1', start: true }), true);
});
