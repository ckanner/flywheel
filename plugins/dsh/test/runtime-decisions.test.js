import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, mountPanel, closeClientHarness } from './support/client-harness.js';

after(closeClientHarness);
const entry = (id, action, status = 'applied', fields = {}) => ({
  id, packet: 'M01-T01', attempt: 2, 'plan-revisions': 0, 'started-at': '2026-11-13T09:00:00Z', action, status, accounted: true,
  selection: { 'action-id': action, adopted: true, reason: 'approved-runtime-choice' },
  receipt: { status: 'ok', reason: 'evaluated', model: 'jev-1.14.0' }, ...fields,
});
const ledger = (fields = {}) => ({ version: 1, used: 5, 'imported-calls': 2, 'max-calls': 8, status: 'enabled', records: [], ...fields });
const state = value => ({ revision: 7, status: 'paused', packets: { 'M01-T01': { status: 'l1-failed', attempts: 2, 'failed-attempts': 1, evidence: { l1: { pass: false } } } }, 'runtime-decisions': value });
const reply = (request, value) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({ protocol: 1, ok: true, running: false, taskDir: request.taskDir ?? '', cwd: '/repo', state: value }) } } });

test('runtime choices distinguish applied control, baseline and unsettled feedback without creating new switches', async t => {
  const decisions = ledger({ records: [
    entry('decision-1', 'model-1', 'pending', { selection: undefined }),
    entry('decision-2', 'replan'),
    entry('decision-3', 'repair', 'baseline', { selection: { 'action-id': 'repair', adopted: false, reason: 'master-disabled' }, receipt: { status: 'skipped', reason: 'master-disabled' } }),
    entry('decision-4', 'feedback-0', 'feedback-unknown'),
    entry('decision-5', 'repair', 'pending', { selection: undefined, receipt: { status: 'unknown', reason: 'cancelled-after-dispatch' } }),
    entry('decision-6', 'recheck-0', 'feedback-stale'),
    entry('decision-7', 'replan', 'completed'),
    entry('decision-8', 'model-1', 'stale'),
    entry('decision-9', 'replan', 'replan-running'),
  ], 'omitted-records': 5 });
  const ui = await mountPanel(t, request => reply(request, state(decisions)));
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  const panel = ui.container.querySelector('[data-runtime-decisions="task"]');
  assert.ok(panel);
  assert.match(panel.textContent, /任务请求 5\/8 · 其中调查移交 2/);
  assert.match(panel.textContent, /显示最近 9 条，共 14 条/);
  assert.match(panel.textContent, /切换批准的模型配置 · model-1/);
  assert.match(panel.querySelector('[data-decision-id="decision-1"]').textContent, /决策请求待核对/);
  assert.doesNotMatch(panel.querySelector('[data-decision-id="decision-1"]').textContent, /决策已应用/);
  assert.match(panel.querySelector('[data-decision-id="decision-2"]').textContent, /请求重新规划/);
  assert.match(panel.querySelector('[data-decision-id="decision-2"]').textContent, /控制决策已应用 · 后续执行见节点状态/);
  assert.match(panel.querySelector('[data-decision-id="decision-3"]').textContent, /沿用基线 · master-disabled/);
  assert.match(panel.querySelector('[data-decision-id="decision-4"]').textContent, /反馈结果未知 · 核对原执行，禁止重放/);
  assert.match(panel.querySelector('[data-decision-id="decision-5"]').textContent, /结果未知 · 核对原回执/);
  assert.match(panel.querySelector('[data-decision-id="decision-6"]').textContent, /复查失败的验收项 · recheck-0/);
  assert.match(panel.querySelector('[data-decision-id="decision-6"]').textContent, /反馈改变了候选或验证输入 · 已阻断/);
  assert.match(panel.querySelector('[data-decision-id="decision-7"]').textContent, /控制动作已收尾 · 任务结果见节点状态/);
  assert.match(panel.querySelector('[data-decision-id="decision-8"]').textContent, /JEV 选择未应用/);
  assert.doesNotMatch(panel.querySelector('[data-decision-id="decision-8"]').textContent, /采用 JEV 选择/);
  assert.match(panel.querySelector('[data-decision-id="decision-9"]').textContent, /重规划未收尾 · 恢复前核对原会话/);
  assert.doesNotMatch(panel.querySelector('[data-decision-id="decision-9"]').textContent, /控制动作已收尾/);
  assert.equal(panel.querySelector('button,input,select'), null);
  await ui.setLocale('en');
  assert.match(ui.container.textContent, /Switch approved model configuration · model-1/);
  assert.match(ui.container.textContent, /Decision request needs reconciliation/);
  assert.equal(ui.requests.length, 1);
});

test('runtime feedback remains diagnostic even when a selected check passes and is also visible on its packet', async t => {
  const decisions = ledger({ records: [entry('feedback-choice', 'feedback-0', 'feedback-completed', { feedback: { pass: false, results: [
    { 'omitted-items': 2 },
    { cmd: 'check the approved reproducer', pass: true, exitCode: 1, timedOut: false, cancelled: false, truncated: false, output: '<b>expected failure reproduced</b>' },
    { cmd: 'check the interrupted read', pass: false, exitCode: -1, timedOut: true, cancelled: true, truncated: true, observationComplete: false, signal: 'SIGTERM', cleanupError: 'cleanup needs reconciliation' },
  ] } })] });
  const ui = await mountPanel(t, request => reply(request, state(decisions)));
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  const panel = ui.container.querySelector('[data-runtime-decisions="task"]');
  assert.match(panel.textContent, /这是诊断反馈；最终仍执行完整验收/);
  assert.match(panel.textContent, /反馈检查已收尾/);
  assert.match(panel.textContent, /省略了 2 项较早的反馈结果/);
  assert.match(panel.textContent, /退出码: 1 · 通过/);
  assert.match(panel.textContent, /动作超时/);
  assert.match(panel.textContent, /动作已取消/);
  assert.match(panel.textContent, /反馈结果不完整/);
  assert.match(panel.textContent, /有界片段/);
  assert.match(panel.textContent, /cleanup needs reconciliation/);
  assert.equal(panel.querySelector('b'), null, 'evidence output remains text');
  await act(async () => ui.container.querySelector('[data-node-id="M01-T01"]').click());
  const node = ui.container.querySelector('[data-runtime-decisions="M01-T01"]');
  assert.ok(node);
  assert.match(node.textContent, /feedback-choice/);
  assert.equal(ui.requests.length, 1, 'opening a local decision summary does not rerun feedback');
});

test('malformed runtime decisions retain the previous valid view and cannot enter React as arbitrary objects', async t => {
  const valid = ledger({ records: [entry('saved-decision', 'repair')] });
  let value = valid;
  const ui = await mountPanel(t, request => reply(request, state(value)));
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  for (const invalid of [
    { records: {} }, { used: -1 }, { status: {} }, { 'omitted-records': -1 },
    { records: [entry('bad', 'repair', 'applied', { selection: { adopted: {}, 'action-id': 'repair', reason: 'bad' } })] },
    { records: [entry('bad', 'repair', 'applied', { feedback: { pass: true, results: [{ cmd: {}, pass: true, exitCode: 0 }] } })] },
    { records: [entry('bad', 'repair', 'applied', { feedback: { pass: true, results: [{ 'omitted-items': {} }] } })] },
  ]) {
    value = { ...valid, ...invalid };
    await ui.click('刷新状态');
    assert.match(ui.container.textContent, /malformed task status/);
    assert.match(ui.container.textContent, /saved-decision/);
    assert.equal(ui.button('启动').disabled, true);
    value = valid;
    await ui.click('刷新状态');
    assert.equal(ui.button('启动').disabled, false);
  }
});

test('applied model history shows concrete bindings and reasons without replacing the last actual session', async t => {
  const before = { harness: 'dsh', provider: 'deepseek-account', channel: 'work', model: 'deepseek-flash', thinking: 'high', permission: 'full-access', sessionMode: 'scoped' };
  const after = { harness: 'codex-pro', provider: 'chatgpt', channel: 'research', model: 'approved-strong-model', thinking: 'max', permission: 'read-only', sessionMode: 'fresh' };
  const change = { kind: 'jev-model-selection', from: 'fast', to: 'strong', before, after, reason: 'insufficient-progress; decision switch-1', at: '2026-11-13T09:00:00Z', attempt: 2 };
  const value = state(ledger({ records: [entry('switch-1', 'model-1')] }));
  value.packets['M01-T01']['execution-policy'] = { version: 1, current: 'strong', history: [change] };
  value.sessions = { records: { previous: { role: 'execute', scope: 'M01-T01', configuration: before, last: { 'started-at': '2026-11-13T08:59:00Z', outcome: 'completed' } } } };
  const ui = await mountPanel(t, request => reply(request, value));
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  const panel = ui.container.querySelector('[data-execution-switches="task"]');
  assert.match(panel.textContent, /fast → strong/);
  assert.match(panel.textContent, /采用 JEV 模型选择/);
  for (const field of Object.values(after)) assert.ok(panel.textContent.includes(field), field);
  assert.match(panel.textContent, /insufficient-progress; decision switch-1/);
  assert.match(panel.textContent, /实际启动的模型与会话以节点会话记录为准/);
  const node = ui.container.querySelector('[data-node-id="M01-T01"]');
  assert.match(node.textContent, /DeepSeek · Flash/);
  assert.doesNotMatch(node.textContent, /approved-strong-model/);
  await act(async () => node.click());
  assert.match(ui.container.querySelector('[data-execution-switches="M01-T01"]').textContent, /approved-strong-model/);
  await ui.setLocale('en');
  assert.match(ui.container.textContent, /JEV model selection applied/);
  assert.equal(ui.requests.length, 1, 'inspecting a model change does not start a new session');
});

test('malformed applied model history retains the last valid view and blocks starting', async t => {
  const original = state(ledger({ records: [entry('saved-model-choice', 'model-1')] }));
  let value = original;
  const ui = await mountPanel(t, request => reply(request, value));
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  const change = { kind: 'jev-model-selection', from: 'fast', to: 'strong', before: { model: 'fast-model' }, after: { model: 'strong-model' }, reason: 'approved', at: '2026-11-13T09:00:00Z', attempt: 1 };
  for (const history of [{}, [{ ...change, after: { model: {} } }], [{ ...change, reason: {} }], [{ ...change, attempt: -1 }]]) {
    value = { ...original, packets: { 'M01-T01': { ...original.packets['M01-T01'], 'execution-policy': { history } } } };
    await ui.click('刷新状态');
    assert.match(ui.container.textContent, /malformed task status/);
    assert.match(ui.container.textContent, /saved-model-choice/);
    assert.equal(ui.button('启动').disabled, true);
    value = original;
    await ui.click('刷新状态');
  }
});
