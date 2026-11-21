import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { validPlanningCommand } from '../src/planning-control.js';
import { act, mountPanel, closeClientHarness, deferred } from './support/client-harness.js';
import { designRef, planningState } from './support/planning-fixture.js';

after(closeClientHarness);

const packetID = 'M01-T01';
const fingerprint = 'a'.repeat(64);
const planRef = { revision: 3, digest: 'c'.repeat(64) };
const collaboration = () => ({ version: 1, tasks: [
  { goal: 'Implement parsing', writes: ['src/parser/'] },
  { goal: 'Implement rendering', writes: ['src/render/'] },
] });
const binding = { harness: 'antigravity', provider: '', channel: 'google-native', model: 'gemini-pro-orchestrator-neon', thinking: '',
  'native-profile': 'agy-swarm', permission: 'workspace-write', 'session-mode': 'scoped' };
function taskState() {
  return { revision: 7, status: 'paused', 'current-milestone': 'M01', packets: { [packetID]: {
    status: 'pending', route: 'default', attempts: 0, schedule: { writes: ['src/'] },
  } }, planning: planningState({ phase: 'ready', 'start-authorized': true,
    active: { version: 1, design: designRef, plans: [planRef] },
    plans: [{ ref: planRef, design: designRef, scope: 'packet', 'scope-id': packetID, files: { ['task-packet-' + packetID + '.md']: 'Existing immutable plan' } }],
    'current-plan': planRef, 'current-plans': { ['packet/' + packetID]: planRef },
    approvals: [{ kind: 'human', design: designRef, 'review-id': 'review-2', source: 'dsh:user-command' }],
  }) };
}
function editor(state, patch = {}) {
  return { packet: packetID, revision: state.revision, contractFingerprint: fingerprint, plan: { ...planRef }, editable: true,
    reason: 'Saving requires a new independent plan review.', writes: ['src/'], value: collaboration(), ...patch };
}
function eligibility(state, patch = {}) {
  return { packet: packetID, revision: state.revision, status: 'verified', reason: 'Inputs checked; no account or model invocation was tested.', source: 'configuration', route: 'default', candidates: [
    { id: 'swarm', binding, eligible: false, conditions: [
      { code: 'image-input', passed: true, message: 'This packet does not require images.' },
      { code: 'full-workflow', passed: true, message: 'Full is selected.' },
      { code: 'collaboration-declared', passed: false, message: 'Independent tasks have not been reviewed.' },
    ] },
    { id: 'native', binding: { ...binding, 'native-profile': 'agy-native' }, eligible: true, conditions: [
      { code: 'image-input', passed: true, message: 'This packet does not require images.' },
    ] },
  ], ...patch };
}
function node(state, patch = {}) {
  return { id: packetID, revision: state.revision, packet: state.packets[packetID],
    collaboration: { status: 'verified', reason: 'reviewed-contract-matched', contractFingerprint: fingerprint, value: collaboration() },
    collaborationEditor: editor(state), eligibility: eligibility(state), ...patch };
}
const reply = (request, fields) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({
  protocol: 1, ok: true, running: false, taskDir: request.taskDir ?? '', cwd: '/repo', state: null, ...fields,
}) } } });
function command(patch = {}) {
  return { action: 'collaboration', revision: 7, 'request-id': 'collaboration-1', start: true,
    collaboration: { packet: packetID, contractFingerprint: fingerprint, plan: planRef, value: collaboration() }, ...patch };
}
async function inspect(ui) {
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新');
  await openNode(ui);
}
const openNode = async ui => act(async () => ui.container.querySelector('[data-node-id="' + packetID + '"]').click());
async function input(ui, row, field, value) {
  const element = ui.container.querySelector('[data-collaboration-task="' + row + '"] textarea[aria-label$="' + field + '"]');
  assert.ok(element, 'the collaboration field is visible');
  await act(async () => {
    Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLTextAreaElement.prototype, 'value').set.call(element, value);
    element.dispatchEvent(new ui.document.defaultView.Event('input', { bubbles: true }));
  });
}
const draftGoal = ui => ui.container.querySelector('[data-collaboration-task="0"] textarea').value;

test('collaboration commands bind the exact packet, contract and plan, reject authority fields and bound the payload', () => {
  assert.equal(validPlanningCommand(command()), true);
  const unicode = command(); unicode.collaboration.value.tasks[0].goal = '😀'.repeat(512);
  assert.equal(validPlanningCommand(unicode), true, 'the goal limit counts Unicode characters like the engine');
  const invalid = [
    value => { value.revision = 0; }, value => { delete value['request-id']; }, value => { value.actor = 'user'; },
    value => { value.design = designRef; }, value => { value.start = 'yes'; }, value => { delete value.collaboration; },
    value => { value.collaboration.packet = '../M01-T01'; }, value => { value.collaboration.contractFingerprint = 'latest'; },
    value => { value.collaboration.plan.digest = 'b'.repeat(63); }, value => { value.collaboration.plan.approved = true; },
    value => { value.collaboration.approved = true; }, value => { value.collaboration.value.version = 2; },
    value => { value.collaboration.value.tasks = []; }, value => { value.collaboration.value.tasks = Array.from({ length: 9 }, () => collaboration().tasks[0]); },
    value => { value.collaboration.value.tasks[0].goal = ' '; }, value => { value.collaboration.value.tasks[0].goal = '😀'.repeat(513); },
    value => { value.collaboration.value.tasks[0].prompt = 'invented schema'; }, value => { value.collaboration.value.tasks[0].writes = []; },
    value => { value.collaboration.value.tasks[0].writes = Array(65).fill('src/parser/'); },
    value => { value.collaboration.value.tasks[0].writes = ['\0']; },
    value => { value.collaboration.value.tasks[0].writes = Array(8).fill('x'.repeat(8192)); },
  ];
  for (const mutate of invalid) {
    const value = structuredClone(command()); mutate(value);
    assert.equal(validPlanningCommand(value), false, String(mutate));
  }
});

test('execution eligibility displays engine conditions and input capabilities without inventing provider observations', async t => {
  const state = taskState();
  let snapshot = node(state);
  const ui = await mountPanel(t, request => reply(request, { state, ...(request.action === 'node' ? { node: snapshot } : {}) }));
  await inspect(ui);
  const view = () => ui.container.querySelector('[data-node-eligibility]');
  assert.match(view().textContent, /尚未检查当前执行候选/);
  assert.equal(ui.requests.length, 1, 'opening a node does not probe an account or invoke a model');
  await ui.click('检查执行资格');
  assert.match(view().textContent, /检查依据：当前生效配置/);
  const swarm = view().querySelector('[data-eligibility-candidate="swarm"]');
  assert.match(swarm.textContent, /gemini-pro-orchestrator-neon/);
  assert.match(swarm.textContent, /原生固定/);
  assert.match(swarm.textContent, /当前不可用/);
  assert.match(swarm.textContent, /尚未准备可并行的独立分工/);
  assert.match(view().querySelector('[data-eligibility-candidate="native"]').textContent, /满足当前输入与协作要求/);
  assert.equal(ui.container.querySelector('[data-native-child]'), null, 'checking a split does not fabricate actual children');
  snapshot = node(state, { eligibility: eligibility(state, { source: 'frozen', candidates: [
    { id: 'swarm', binding, eligible: false, conditions: [{ code: 'image-input', passed: false, message: 'The chosen transport cannot receive images.' }] },
  ] }) });
  await ui.click('检查执行资格');
  assert.match(view().textContent, /当前运行已冻结的配置/);
  assert.match(view().textContent, /这个步骤需要原图；所选通道尚未接通图片传输/);
  await ui.setLocale('en');
  assert.match(view().textContent, /Current execution eligibility/);
  assert.match(view().textContent, /selected channel has no connected image transport/);
  assert.deepEqual(ui.requests.map(request => request.action), ['status', 'node', 'node']);
});

test('eligibility distinguishes a pending or failed read, unknown evidence, stale revisions and a confirmed empty candidate set', async t => {
  let state = taskState(), outcome = node(state), pending;
  const ui = await mountPanel(t, request => request.action === 'node' && pending ? pending.promise : reply(request, { state, ...(request.action === 'node' ? { node: outcome } : {}) }));
  await inspect(ui);
  pending = deferred();
  await ui.click('检查执行资格');
  const view = () => ui.container.querySelector('[data-node-eligibility]');
  assert.match(view().textContent, /正在检查当前执行资格/);
  assert.doesNotMatch(view().textContent, /未返回可检查/);
  await act(async () => pending.reject(new Error('node read unavailable')));
  pending = null;
  assert.match(view().textContent, /无法确认当前执行资格/);
  assert.doesNotMatch(view().textContent, /未返回可检查/);
  outcome = node(state, { eligibility: eligibility(state, { status: 'unknown', reason: 'contract-changed-or-unfrozen', candidates: [] }) });
  await ui.click('检查执行资格');
  assert.match(view().textContent, /无法确认当前执行资格/);
  assert.match(view().textContent, /contract-changed-or-unfrozen/);
  assert.doesNotMatch(view().textContent, /未返回可检查/);
  outcome = node(state, { eligibility: eligibility(state, { candidates: [] }) });
  await ui.click('检查执行资格');
  assert.match(view().textContent, /引擎未返回可检查/);
  state = { ...state, revision: 8 };
  await ui.click('刷新');
  assert.match(view().textContent, /检查结果已过期|尚未检查当前执行候选/);
  assert.equal(view().querySelector('[data-eligibility-candidate]'), null);
});

test('structured collaboration edits keep the approved split visible and submit only the exact draft for independent review', async t => {
  let state = taskState(), saved;
  const ui = await mountPanel(t, request => {
    if (request.planning?.action === 'collaboration') {
      saved = request.planning;
      state = { ...state, revision: 8 };
      return reply(request, { state, 'planning-control': { action: 'collaboration', 'request-id': saved['request-id'], status: 'applied', revision: 8 } });
    }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await inspect(ui);
  await ui.click('编辑分工草案');
  const form = () => ui.container.querySelector('[data-collaboration-editor]');
  assert.match(form().textContent, /父任务允许写入范围: src\//);
  assert.match(form().textContent, /不会启动实施/);
  assert.equal(form().querySelectorAll('[data-collaboration-task]').length, 2);
  assert.equal(ui.button('保存草案并重新审查').disabled, true, 'an unchanged plan does not start another review');
  await input(ui, 0, '子任务目标', 'Implement a streaming parser');
  await input(ui, 0, '写入范围（每行一项）', 'src/parser/\nsrc/decoder/');
  assert.match(ui.container.querySelector('[data-node-collaboration] > ol').textContent, /Implement parsing/);
  assert.doesNotMatch(ui.container.querySelector('[data-node-collaboration] > ol').textContent, /streaming/);
  assert.equal(ui.button('保存草案并重新审查').disabled, false);
  await ui.click('保存草案并重新审查');
  assert.equal(saved.action, 'collaboration');
  assert.equal(saved.revision, 7);
  assert.equal(saved.start, true, 'the operation requests its review-only engine path');
  assert.match(saved['request-id'], /^dsh-/);
  assert.deepEqual(saved.collaboration, { packet: packetID, contractFingerprint: fingerprint, plan: planRef, value: { version: 1, tasks: [
    { goal: 'Implement a streaming parser', writes: ['src/parser/', 'src/decoder/'] },
    { goal: 'Implement rendering', writes: ['src/render/'] },
  ] } });
  assert.match(form().textContent, /新的分工计划已保存并请求独立审查/);
  assert.equal(form().querySelector('textarea'), null);
  assert.equal(ui.requests.filter(request => request.planning?.action === 'approve' || ['launch', 'start', 'step'].includes(request.action)).length, 0);
  assert.equal(ui.container.querySelector('[data-native-child]'), null);
  await ui.setLocale('en');
  assert.match(form().textContent, /saved for independent review/);
});

test('task counts, goal bounds and write scopes are edited with bounded controls', async t => {
  const state = taskState();
  const ui = await mountPanel(t, request => reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) }));
  await inspect(ui); await ui.click('编辑分工草案');
  assert.equal(ui.button('移除此子任务').disabled, true);
  for (let i = 0; i < 6; i++) await ui.click('添加子任务');
  assert.equal(ui.container.querySelectorAll('[data-collaboration-task]').length, 8);
  assert.equal(ui.button('添加子任务').disabled, true);
  assert.equal(ui.button('保存草案并重新审查').disabled, true, 'new incomplete assignments cannot be submitted');
  for (let i = 0; i < 6; i++) {
    const rows = ui.container.querySelectorAll('[data-collaboration-task]');
    await act(async () => rows[rows.length - 1].querySelector('button').click());
  }
  await input(ui, 0, '子任务目标', '😀'.repeat(513));
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
  await input(ui, 0, '子任务目标', '😀'.repeat(512));
  assert.equal(ui.button('保存草案并重新审查').disabled, false);
  await input(ui, 0, '写入范围（每行一项）', Array(65).fill('src/parser/').join('\n'));
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
  assert.equal(ui.requests.filter(request => request.action === 'planning').length, 0);
});

test('stale revisions, fingerprint changes and running tasks preserve the draft while blocking submission', async t => {
  let state = taskState(), running = false, hash = fingerprint;
  const ui = await mountPanel(t, request => reply(request, { state, running, ...(request.action === 'node' ? { node: node(state, { collaborationEditor: editor(state, { contractFingerprint: hash }) }) } : {}) }));
  await inspect(ui); await ui.click('编辑分工草案');
  await input(ui, 0, '子任务目标', 'Retain my local draft');
  running = true; await ui.click('刷新');
  assert.equal(draftGoal(ui), 'Retain my local draft');
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
  running = false; state = { ...state, revision: 8 }; await ui.click('刷新');
  assert.equal(draftGoal(ui), 'Retain my local draft');
  assert.match(ui.container.querySelector('[data-collaboration-stale]').textContent, /草案基于旧版本/);
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
  await ui.click('重新读取并替换草案');
  assert.equal(draftGoal(ui), 'Implement parsing');
  await input(ui, 0, '子任务目标', 'Another local draft');
  hash = 'd'.repeat(64); await ui.click('检查执行资格');
  assert.equal(draftGoal(ui), 'Another local draft');
  assert.equal(ui.button('保存草案并重新审查').disabled, true, 'same state revision with a changed contract identity is also stale');
  await ui.click('刷新版本并保留草案');
  assert.equal(draftGoal(ui), 'Another local draft');
  assert.match(ui.container.querySelector('[data-collaboration-result]').textContent, /不能自动移到新版本/);
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
});

test('rejected edits and malformed rereads retain text and do not become saved plans', async t => {
  let state = taskState(), malformed = false;
  const ui = await mountPanel(t, request => {
    if (request.planning?.action === 'collaboration') {
      state = { ...state, revision: state.revision + 1 };
      return reply(request, { state, 'planning-control': { action: 'collaboration', 'request-id': request.planning['request-id'], status: 'rejected', message: 'The second write scope overlaps another task.' } });
    }
    const current = node(state); if (malformed) current.collaborationEditor.plan.digest = 'not-a-digest';
    return reply(request, { state, ...(request.action === 'node' ? { node: current } : {}) });
  });
  await inspect(ui); await ui.click('编辑分工草案');
  await input(ui, 0, '子任务目标', 'Keep this after a rejected save');
  await input(ui, 0, '写入范围（每行一项）', 'src/render/');
  await ui.click('保存草案并重新审查');
  assert.match(ui.container.querySelector('[data-collaboration-result]').textContent, /未被采用.*overlaps/);
  assert.equal(draftGoal(ui), 'Keep this after a rejected save');
  assert.equal(ui.button('保存草案并重新审查').disabled, true, 'the durable rejection advanced the state revision');
  await ui.click('刷新版本并保留草案');
  assert.equal(draftGoal(ui), 'Keep this after a rejected save');
  assert.equal(ui.button('保存草案并重新审查').disabled, false, 'an unchanged contract may refresh its audit revision without discarding edits');
  malformed = true; await ui.click('重新读取并替换草案');
  assert.equal(draftGoal(ui), 'Keep this after a rejected save');
  assert.match(ui.container.querySelector('[data-collaboration-result]').textContent, /未能读取可编辑的分工快照/);
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
});

test('a missing save receipt stays unknown and reconciles the exact request without resubmitting the mutation', async t => {
  let state = taskState(), requestID, saves = 0;
  const ui = await mountPanel(t, request => {
    if (request.planning?.action === 'collaboration') {
      saves++; requestID = request.planning['request-id'];
      return reply(request, { state });
    }
    if (request.planning?.action === 'status') {
      assert.equal(request.planning['request-id'], requestID);
      state = { ...state, revision: 8 };
      return reply(request, { state, 'planning-control': { action: 'collaboration', 'request-id': requestID, status: 'applied', revision: 8 } });
    }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await inspect(ui); await ui.click('编辑分工草案');
  await input(ui, 0, '子任务目标', 'Save outcome must be reconciled');
  await ui.click('保存草案并重新审查');
  assert.equal(draftGoal(ui), 'Save outcome must be reconciled');
  assert.match(ui.container.querySelector('[data-collaboration-result]').textContent, /没有收到可确认的保存结果/);
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
  assert.equal(ui.button('重新读取并替换草案').disabled, true);
  await ui.click('刷新任务并核对结果');
  assert.equal(saves, 1);
  assert.equal(ui.requests.at(-1).planning.action, 'status');
  assert.match(ui.container.querySelector('[data-collaboration-result]').textContent, /已保存并请求独立审查/);
});

test('a receipt for another edit or approval cannot confirm this collaboration mutation', async t => {
  for (const mismatch of [{ 'request-id': 'another-edit' }, { action: 'approve' }]) {
    const state = taskState();
    const ui = await mountPanel(t, request => request.planning?.action === 'collaboration' ? reply(request, { state,
      'planning-control': { action: 'collaboration', 'request-id': request.planning['request-id'], status: 'applied', ...mismatch } })
      : reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) }));
    await inspect(ui); await ui.click('编辑分工草案');
    await input(ui, 0, '子任务目标', 'Must match this exact edit');
    await ui.click('保存草案并重新审查');
    const bound = ui.bindings.get('desktop-session:0');
    const controller = ui.definition.inject('desktop-session', bound.instance.actions).hooks.control.getSnapshot();
    assert.equal(controller.uncertainty.action, 'planning');
    assert.equal(controller.stale, true);
    assert.equal(draftGoal(ui), 'Must match this exact edit');
    assert.equal(ui.button('保存草案并重新审查').disabled, true);
    assert.match(ui.container.querySelector('[data-collaboration-result]').textContent, /没有收到可确认的保存结果/);
    assert.equal(ui.container.querySelector('[data-collaboration-result="saved"]'), null);
  }
});

test('configuration saves and planning mutations invalidate node checks even before another execution starts', async t => {
  let state = taskState();
  const ui = await mountPanel(t, request => {
    if (request.action === 'config-save') return reply(request, { editor: { scope: 'project', revision: 'saved-config', values: {}, defaults: {} } });
    if (request.action === 'planning') {
      state = { ...state, revision: 8 };
      return reply(request, { state, 'planning-control': { action: request.planning.action, 'request-id': request.planning['request-id'], status: 'applied' } });
    }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await inspect(ui); await ui.click('编辑分工草案');
  await input(ui, 0, '子任务目标', 'Retain local edits across a settings save');
  const bound = ui.bindings.get('desktop-session:0');
  const controller = ui.definition.inject('desktop-session', bound.instance.actions);
  assert.equal(controller.hooks.control.getSnapshot().node.id, packetID);
  await act(async () => { await controller.run('config-save', ui.selection(), false, { editor: { scope: 'project', revision: 'old-config', values: {} } }); });
  assert.equal(controller.hooks.control.getSnapshot().node, null, 'a settings save invalidates eligibility even if the task revision did not change');
  assert.equal(draftGoal(ui), 'Retain local edits across a settings save');
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
  assert.equal(ui.container.querySelector('[data-eligibility-candidate]'), null);
  await ui.click('重新读取并替换草案');
  assert.equal(controller.hooks.control.getSnapshot().node.id, packetID);
  await act(async () => { await controller.run('planning', ui.selection(), false, {
    planning: { action: 'feedback', revision: 7, 'request-id': 'feedback-invalidates-node', text: 'Reconsider the write split.', start: false },
  }); });
  assert.equal(controller.hooks.control.getSnapshot().node, null);
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
  assert.equal(ui.requests.some(request => ['start', 'launch', 'step'].includes(request.action)), false);
});

test('collaboration text survives inspector, evidence, task and view remounts within its retained conversation binding', async t => {
  const state = taskState();
  const ui = await mountPanel(t, request => reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) }));
  await inspect(ui); await ui.click('编辑分工草案');
  await input(ui, 0, '子任务目标', 'My original task draft');
  await act(async () => ui.container.querySelector('[aria-label="' + ui.translate('consoleDetails') + '"] header button').click());
  await openNode(ui);
  assert.equal(draftGoal(ui), 'My original task draft');
  await ui.click(ui.translate('wbEvidence')); await ui.click(ui.translate('wbBackToNode'));
  assert.equal(draftGoal(ui), 'My original task draft');
  await ui.selectPath(0, '/repo/docs/another-task'); await ui.click('刷新'); await openNode(ui);
  assert.equal(ui.container.querySelector('[data-collaboration-task]'), null, 'another task never receives this draft');
  await ui.click('编辑分工草案'); await input(ui, 0, '子任务目标', 'Other task draft');
  await ui.selectPath(0, '/repo/docs/task'); await ui.click('刷新'); await openNode(ui);
  assert.equal(draftGoal(ui), 'My original task draft');
  await ui.setVisible(false); await ui.setVisible(true); await openNode(ui);
  assert.equal(draftGoal(ui), 'My original task draft');
  await ui.switchSession('another-conversation'); await ui.switchSession('desktop-session'); await openNode(ui);
  assert.equal(draftGoal(ui), 'My original task draft');
  assert.equal(ui.requests.filter(request => request.planning).length, 0, 'restoring a draft never saves or reviews it');
  await ui.switchSession('desktop-session', 1); await inspect(ui);
  assert.equal(ui.container.querySelector('[data-collaboration-task]'), null, 'a fresh host Session binding does not inherit the old in-memory draft');
});

test('pending collaboration identity survives unmount and an applied late receipt settles only its cached draft', async t => {
  let state = taskState(), saved;
  const pending = deferred();
  const ui = await mountPanel(t, request => {
    if (request.planning?.action === 'collaboration') { saved = request; return pending.promise; }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await inspect(ui); await ui.click('编辑分工草案'); await input(ui, 0, '子任务目标', 'A pending draft survives closing the view');
  await ui.click('保存草案并重新审查');
  const bound = ui.bindings.get('desktop-session:0');
  const controller = ui.definition.inject('desktop-session', bound.instance.actions);
  const retained = [...controller.packetDrafts.values()][0];
  assert.equal(retained.getSnapshot().notice.requestID, saved.planning['request-id'], 'the request ID is retained before awaiting the response');
  assert.equal(retained.getSnapshot().notice.kind, 'pending');
  await ui.setVisible(false);
  state = { ...state, revision: 8 };
  await act(async () => pending.resolve(reply(saved, { state,
    'planning-control': { action: 'collaboration', 'request-id': saved.planning['request-id'], status: 'applied' } })));
  assert.equal(retained.getSnapshot().notice.kind, 'saved');
  await ui.setVisible(true); await openNode(ui);
  assert.equal(ui.container.querySelector('[data-collaboration-task]'), null);
  assert.match(ui.container.querySelector('[data-collaboration-result]').textContent, /已保存并请求独立审查/);
  assert.equal(ui.requests.filter(request => request.planning?.action === 'collaboration').length, 1);
});

test('an unknown result retains its request across remount and wrong-action status receipts cannot unlock it', async t => {
  const state = taskState(); let saved, wrongAction = true;
  const pending = deferred();
  const ui = await mountPanel(t, request => {
    if (request.planning?.action === 'collaboration') { saved = request; return pending.promise; }
    if (request.planning?.action === 'status') {
      assert.equal(request.planning['request-id'], saved.planning['request-id']);
      return reply(request, { state, 'planning-control': { action: wrongAction ? 'approve' : 'collaboration', 'request-id': saved.planning['request-id'], status: 'applied' } });
    }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await inspect(ui); await ui.click('编辑分工草案'); await input(ui, 0, '子任务目标', 'Retain unknown draft and request identity');
  await ui.click('保存草案并重新审查'); await ui.setVisible(false);
  await act(async () => pending.resolve(reply(saved, { state })));
  await ui.setVisible(true); await openNode(ui);
  assert.equal(draftGoal(ui), 'Retain unknown draft and request identity');
  assert.match(ui.container.querySelector('[data-collaboration-result]').textContent, /没有收到可确认的保存结果/);
  await ui.click('刷新任务并核对结果');
  const bound = ui.bindings.get('desktop-session:0');
  const controller = ui.definition.inject('desktop-session', bound.instance.actions);
  assert.equal(controller.hooks.control.getSnapshot().uncertainty.planningAction, 'collaboration');
  assert.equal(draftGoal(ui), 'Retain unknown draft and request identity');
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
  assert.equal(ui.container.querySelector('[data-collaboration-result="saved"]'), null);
  wrongAction = false; await ui.click('刷新任务并核对结果');
  assert.equal(controller.hooks.control.getSnapshot().uncertainty, null);
  assert.match(ui.container.querySelector('[data-collaboration-result]').textContent, /已保存并请求独立审查/);
  assert.equal(ui.requests.filter(request => request.planning?.action === 'collaboration').length, 1);
});

test('the browser enforces the complete UTF-8 payload bound and known not-dispatched failures remain repairable', async t => {
  const state = taskState();
  const ui = await mountPanel(t, request => request.planning?.action === 'collaboration'
    ? { ok: true, value: { result: { kind: 'error', text: JSON.stringify({ protocol: 1, type: 'flywheel-control-error', action: 'planning',
      dispatch: 'not-dispatched', message: 'The configured host binary is unavailable.' }) } } }
    : reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) }));
  await inspect(ui); await ui.click('编辑分工草案');
  await input(ui, 0, '写入范围（每行一项）', Array(3).fill('src/' + '界'.repeat(4000)).join('\n'));
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
  assert.match(ui.container.querySelector('[data-collaboration-too-large]').textContent, /超过 32 KiB/);
  assert.equal(ui.requests.filter(request => request.planning).length, 0, 'UTF-16 character count cannot bypass the UTF-8 byte bound');
  await input(ui, 0, '写入范围（每行一项）', 'src/new-parser/');
  assert.equal(ui.button('保存草案并重新审查').disabled, false);
  await ui.click('保存草案并重新审查');
  assert.match(ui.container.querySelector('[data-collaboration-result]').textContent, /未被采用.*host binary is unavailable/);
  assert.equal(ui.button('保存草案并重新审查').disabled, false, 'confirmed non-dispatch does not require a receipt that cannot exist');
  assert.equal(ui.button('重新读取并替换草案').disabled, false);
  assert.equal(ui.container.querySelector('[data-collaboration-result="unknown"]'), null);
  await input(ui, 0, '子任务目标', 'The draft is still editable');
  assert.equal(draftGoal(ui), 'The draft is still editable');
});

test('a confirmed pre-dispatch rejection settles an unmounted draft before transient control errors are cleared', async t => {
  const state = taskState(), pending = deferred();
  let saved;
  const ui = await mountPanel(t, request => {
    if (request.planning?.action === 'collaboration') { saved = request; return pending.promise; }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await inspect(ui); await ui.click('编辑分工草案');
  await input(ui, 0, '子任务目标', 'Keep the rejected draft across navigation');
  await ui.click('保存草案并重新审查'); await ui.setVisible(false);
  const bound = ui.bindings.get('desktop-session:0');
  const controller = ui.definition.inject('desktop-session', bound.instance.actions);
  const retained = [...controller.packetDrafts.values()][0];
  assert.equal(retained.getSnapshot().notice.requestID, saved.planning['request-id']);
  await act(async () => pending.resolve({ ok: true, value: { result: { kind: 'error', text: JSON.stringify({
    protocol: 1, type: 'flywheel-control-error', action: 'planning', dispatch: 'not-dispatched', message: 'Host validation stopped this request before dispatch.',
  }) } } }));
  assert.equal(retained.getSnapshot().notice.kind, 'rejected', 'promise completion must settle without a mounted effect');
  assert.equal(retained.getSnapshot().pending, false);
  await act(async () => { await controller.run('status', ui.selection()); });
  assert.equal(controller.hooks.control.getSnapshot().error, null, 'an unrelated read clears the transient controller error');
  assert.equal(controller.hooks.control.getSnapshot().uncertainty, null);
  await ui.setVisible(true); await openNode(ui);
  assert.equal(draftGoal(ui), 'Keep the rejected draft across navigation');
  assert.match(ui.container.querySelector('[data-collaboration-result="rejected"]').textContent, /Host validation stopped/);
  assert.equal(ui.button('保存草案并重新审查').disabled, false);
  assert.equal(ui.button('重新读取并替换草案').disabled, false);
  assert.equal(ui.container.querySelector('[data-collaboration-result="unknown"]'), null);
  assert.equal(ui.requests.filter(request => request.planning?.action === 'status').length, 0, 'no receipt lookup is needed for a request that never dispatched');
});

test('uneditable snapshots explain the restriction and late replies cannot populate another task', async t => {
  const state = taskState(); let pending;
  const ui = await mountPanel(t, request => request.action === 'node' && pending ? pending.promise : reply(request, {
    state, ...(request.action === 'node' ? { node: node(state, { collaborationEditor: editor(state, { editable: false, plan: undefined, contractFingerprint: undefined,
      reason: 'A reviewed planning reference is required.', value: undefined, writes: [] }) }) } : {}),
  }));
  await inspect(ui); await ui.click('编辑分工草案');
  assert.match(ui.container.querySelector('[data-collaboration-uneditable]').textContent, /当前阶段不可编辑分工.*reviewed planning/);
  assert.equal(ui.container.querySelector('[data-collaboration-task]'), null);
  assert.equal(ui.button('保存草案并重新审查').disabled, true);
  assert.equal(ui.container.querySelector('[data-collaboration-stale]'), null);
  pending = deferred(); await ui.click('重新读取并替换草案');
  await ui.selectPath(0, '/repo/docs/another-task');
  const request = ui.requests.at(-1);
  await act(async () => pending.resolve(reply(request, { state, node: node(state) })));
  assert.equal(ui.container.querySelector('[data-collaboration-task]'), null);
  assert.equal(ui.selection().taskDir, '/repo/docs/another-task');
});
