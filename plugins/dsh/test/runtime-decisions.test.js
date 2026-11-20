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
  await ui.click('刷新');
  const panel = ui.container.querySelector('[data-runtime-decisions="task"]');
  assert.ok(panel);
  assert.match(panel.textContent, /任务请求 5\/8 · 其中调查移交 2/);
  assert.match(panel.textContent, /显示最近 9 条，共 14 条/);
  assert.match(panel.textContent, /切换批准的模型配置/);
  assert.match(panel.querySelector('[data-decision-id="decision-1"] [data-raw-evidence]').textContent, /"action": "model-1"/);
  assert.match(panel.querySelector('[data-decision-id="decision-1"]').textContent, /决策请求待核对/);
  assert.doesNotMatch(panel.querySelector('[data-decision-id="decision-1"]').textContent, /决策已应用/);
  assert.match(panel.querySelector('[data-decision-id="decision-2"]').textContent, /请求重新规划/);
  assert.match(panel.querySelector('[data-decision-id="decision-2"]').textContent, /控制决策已应用 · 后续执行见节点状态/);
  assert.match(panel.querySelector('[data-decision-id="decision-3"] [data-decision-adoption]').textContent, /沿用基线 · JEV 总开关已关闭/);
  assert.match(panel.querySelector('[data-decision-id="decision-4"]').textContent, /反馈结果未知 · 核对原执行，禁止重放/);
  assert.match(panel.querySelector('[data-decision-id="decision-5"]').textContent, /结果未知 · 核对原回执/);
  assert.match(panel.querySelector('[data-decision-id="decision-6"]').textContent, /复查失败的验收项/);
  assert.match(panel.querySelector('[data-decision-id="decision-6"]').textContent, /反馈改变了候选或验证输入 · 已阻断/);
  assert.match(panel.querySelector('[data-decision-id="decision-7"]').textContent, /控制动作已收尾 · 任务结果见节点状态/);
  assert.match(panel.querySelector('[data-decision-id="decision-8"]').textContent, /JEV 选择未应用/);
  assert.doesNotMatch(panel.querySelector('[data-decision-id="decision-8"]').textContent, /采用 JEV 选择/);
  assert.match(panel.querySelector('[data-decision-id="decision-9"]').textContent, /重规划未收尾 · 恢复前核对原会话/);
  assert.doesNotMatch(panel.querySelector('[data-decision-id="decision-9"]').textContent, /控制动作已收尾/);
  assert.equal(panel.querySelector('button,input,select'), null);
  await ui.setLocale('en');
  assert.match(ui.container.textContent, /Switch approved model configuration/);
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
  await ui.click('刷新');
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
  await ui.click('刷新');
  for (const invalid of [
    { records: {} }, { used: -1 }, { status: {} }, { 'omitted-records': -1 },
    { records: [entry('bad', 'repair', 'applied', { selection: { adopted: {}, 'action-id': 'repair', reason: 'bad' } })] },
    { records: [entry('bad', 'repair', 'applied', { feedback: { pass: true, results: [{ cmd: {}, pass: true, exitCode: 0 }] } })] },
    { records: [entry('bad', 'repair', 'applied', { feedback: { pass: true, results: [{ 'omitted-items': {} }] } })] },
    { records: [entry('bad', 'model-1', 'applied', { 'model-bindings': { 'model-1': { bindingId: 'candidate', binding: { harness: 'antigravity', model: {} } } } })] },
    { records: [entry('bad', 'model-1', 'applied', { 'model-bindings-omitted': -1 })] },
    { records: [entry('bad', 'model-1', 'applied', { 'model-bindings-omitted': '1' })] },
    { records: [entry('bad', 'model-1', 'applied', { phase: 'invented-phase' })] },
    { records: [entry('bad', 'model-1', 'applied', { actions: [{ id: 'model-1', kind: 'model', description: {} }] })] },
  ]) {
    value = { ...valid, ...invalid };
    await ui.click('刷新');
    assert.match(ui.container.textContent, /malformed task status/);
    assert.match(ui.container.textContent, /saved-decision/);
    assert.equal(ui.button('继续流程').disabled, true);
    value = valid;
    await ui.click('刷新');
    assert.equal(ui.button('继续流程').disabled, false);
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
  await ui.click('刷新');
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
  await ui.click('刷新');
  const change = { kind: 'jev-model-selection', from: 'fast', to: 'strong', before: { model: 'fast-model' }, after: { model: 'strong-model' }, reason: 'approved', at: '2026-11-13T09:00:00Z', attempt: 1 };
  for (const history of [{}, [{ ...change, after: { model: {} } }], [{ ...change, reason: {} }], [{ ...change, attempt: -1 }]]) {
    value = { ...original, packets: { 'M01-T01': { ...original.packets['M01-T01'], 'execution-policy': { history } } } };
    await ui.click('刷新');
    assert.match(ui.container.textContent, /malformed task status/);
    assert.match(ui.container.textContent, /saved-model-choice/);
    assert.equal(ui.button('继续流程').disabled, true);
    value = original;
    await ui.click('刷新');
  }
});

test('snapshot v3 initial selection is visible with fixed native effort and an explicit initial reason',async t=>{
  const value=state(ledger({used:1,records:[entry('initial-choice','model-1','applied',{attempt:0,snapshot:{version:3,point:'execution-initial-selection',task:{route:'complex'}}})]}));
  value.packets['M01-T01']['execution-policy']={version:1,route:'complex',current:'orchestrator',history:[{kind:'jev-initial-selection',from:'neon',to:'orchestrator',attempt:0,at:'2026-11-20T10:00:00Z',reason:'approved decomposition needs orchestration',
    before:{harness:'antigravity',model:'gemini-4-neon-high',thinking:'high'},after:{harness:'antigravity',model:'gemini-pro-orchestrator-neon',thinking:'',nativeProfile:'antigravity-native'}}]};
  const ui=await mountPanel(t,request=>reply(request,value));
  await ui.selectPath(0,'/repo/docs/task'); await ui.click('刷新');
  const panel=ui.container.querySelector('[data-execution-switches="task"]');
  assert.match(panel.textContent,/采用 JEV 首轮配置选择/);
  assert.match(panel.textContent,/原生固定/);
  assert.match(panel.textContent,/approved decomposition needs orchestration/);
  await ui.setLocale('en');
  assert.match(ui.container.textContent,/JEV initial configuration applied/);
  assert.equal(ui.requests.length,1);
});

test('first-turn recommendations show frozen bindings separately from the baseline actually retained', async t => {
  const before = { harness: 'antigravity', model: 'gemini-4-neon-high', thinking: 'high', channel: 'google-subscription' };
  const after = { ...before, model: 'gemini-pro-orchestrator-neon', thinking: '', nativeProfile: 'approved-orchestrator' };
  const record = entry('initial-not-adopted', 'model-0', 'baseline', { attempt: 0, phase: 'initial-execution', 'baseline-action-id': 'model-0',
    'model-bindings': { 'model-0': { bindingId: 'neon', binding: before }, 'model-1': { bindingId: 'orchestrator', binding: after } },
    actions: [{ id: 'model-0', kind: 'model', description: 'Frozen ordinary baseline' }, { id: 'model-1', kind: 'model', description: 'Frozen approved orchestration' }],
    receipt: { status: 'ok', reason: 'evaluated', 'action-id': 'model-1' },
    selection: { adopted: false, 'action-id': 'model-0', reason: 'decision-not-applicable' },
  });
  const ui = await mountPanel(t, request => reply(request, state(ledger({ records: [record] }))));
  await ui.selectPath(0, '/repo/docs/task'); await ui.click('刷新');
  const panel = ui.container.querySelector('[data-decision-id="initial-not-adopted"]');
  assert.match(panel.querySelector('h4').textContent, /首次执行前的选择/);
  const suggested = panel.querySelector('[data-decision-choice="runtimeDecisionSuggested"]');
  assert.match(suggested.textContent, /gemini-pro-orchestrator-neon/);
  assert.match(suggested.textContent, /原生固定/);
  const chosen = panel.querySelector('[data-decision-choice="runtimeDecisionChosen"]');
  assert.match(chosen.textContent, /gemini-4-neon-high/);
  assert.doesNotMatch(chosen.textContent, /gemini-pro-orchestrator/);
  assert.match(panel.querySelector('[data-decision-adoption]').textContent, /未达到适用性或选择阈值/);
  assert.match(panel.querySelector('[data-baseline-retained]').textContent, /没有切换模型/);
  assert.match(panel.textContent, /配置选择不代表模型已经启动/);
  const raw = panel.querySelector('[data-raw-evidence]');
  assert.equal(raw.open, false);
  assert.match(raw.textContent, /decision-not-applicable/);
  await act(async () => { raw.open = true; });
  assert.equal(raw.open, true);
  assert.equal(ui.requests.length, 1, 'expanding evidence never re-evaluates a decision');
  await ui.setLocale('en');
  assert.match(panel.textContent, /Choice before first execution/);
  assert.match(panel.textContent, /Initial baseline retained/);
});

test('adopting the original first-turn binding does not invent a model switch or dispatch', async t => {
  const binding = { harness: 'antigravity', model: 'gemini-4-neon-high', thinking: 'high' };
  const record = entry('initial-same-binding', 'model-0', 'applied', { attempt: 0, phase: 'initial-execution', 'baseline-action-id': 'model-0',
    'model-bindings': { 'model-0': { bindingId: 'neon', binding } },
    receipt: { status: 'ok', reason: 'evaluated', 'action-id': 'model-0' },
    selection: { adopted: true, 'action-id': 'model-0', reason: 'approved-model-choice' },
  });
  const ui = await mountPanel(t, request => reply(request, state(ledger({ records: [record] }))));
  await ui.selectPath(0, '/repo/docs/task'); await ui.click('刷新');
  const panel = ui.container.querySelector('[data-decision-id="initial-same-binding"]');
  assert.match(panel.querySelector('[data-decision-adoption]').textContent, /采用 JEV 选择/);
  assert.match(panel.querySelector('[data-baseline-retained]').textContent, /保留初始基线/);
  assert.equal(ui.container.querySelector('[data-execution-switches]'), null);
  assert.equal(ui.requests.length, 1);
});

test('bounded decision snapshots preserve native profile identity and explain candidates omitted from display', async t => {
  const binding = { harness: 'claude-code-pro', model: 'claude-fable-5.2', thinking: 'high', nativeProfile: 'approved-team' };
  const record = entry('bounded-choice', 'model-1', 'baseline', { phase: 'initial-execution', 'baseline-action-id': 'model-0',
    'model-bindings': { 'model-0': { bindingId: 'team', binding, nativeOptionsOmitted: true, nativeOptionsHash: 'c'.repeat(64) } },
    'model-bindings-omitted': 2, receipt: { status: 'ok', reason: 'evaluated', 'action-id': 'model-1' },
  });
  const ui = await mountPanel(t, request => reply(request, state(ledger({ records: [record] }))));
  await ui.selectPath(0, '/repo/docs/task'); await ui.click('刷新');
  const panel = ui.container.querySelector('[data-decision-id="bounded-choice"]');
  const baseline = panel.querySelector('[data-decision-choice="runtimeDecisionInitialBaseline"]');
  assert.match(baseline.textContent, /approved-team/);
  assert.match(baseline.textContent, /claude-fable-5.2/);
  assert.match(baseline.textContent, new RegExp('c'.repeat(64)));
  assert.match(baseline.querySelector('[data-native-options-omitted]').textContent, /原生配置正文已从本视图省略/);
  assert.match(panel.querySelector('[data-model-bindings-omitted]').textContent, /省略了 2 项候选配置/);
  const suggested = panel.querySelector('[data-decision-choice="runtimeDecisionSuggested"]');
  assert.match(suggested.textContent, /本视图未包含该候选的完整配置/);
  assert.doesNotMatch(suggested.textContent, /此历史记录没有保存/);
  await ui.setLocale('en');
  assert.match(panel.textContent, /2 candidate bindings were omitted from this view/);
});

test('stale failure advice retains its suggested binding without claiming the action was applied', async t => {
  const record = entry('failed-choice', 'model-1', 'stale', { phase: 'after-failure', 'baseline-action-id': 'repair',
    'model-bindings': { 'model-1': { bindingId: 'high', binding: { harness: 'codex-pro', model: 'gpt-6.2-sol', thinking: 'high' } } },
    receipt: { status: 'ok', reason: 'evaluated', 'action-id': 'model-1' },
  });
  const ui = await mountPanel(t, request => reply(request, state(ledger({ records: [record] }))));
  await ui.selectPath(0, '/repo/docs/task'); await ui.click('刷新');
  const panel = ui.container.querySelector('[data-decision-id="failed-choice"]');
  assert.match(panel.querySelector('h4').textContent, /失败后的调整/);
  assert.match(panel.querySelector('[data-decision-choice="runtimeDecisionSuggested"]').textContent, /gpt-6.2-sol/);
  assert.match(panel.querySelector('[data-decision-adoption]').textContent, /JEV 选择未应用/);
  assert.ok(panel.querySelector('[data-decision-choice="runtimeDecisionRecordedAction"]'));
  assert.equal(panel.querySelector('[data-decision-choice="runtimeDecisionChosen"]'), null);
});
