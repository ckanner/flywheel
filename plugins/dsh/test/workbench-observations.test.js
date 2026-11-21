import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, mountPanel, closeClientHarness, deferred } from './support/client-harness.js';

after(closeClientHarness);

const packetID = 'M01-T01';
const initial = { harness: 'antigravity', provider: '', channel: 'google-subscription', model: 'gemini-4-neon-high', thinking: 'high', permission: 'danger-full-access', sessionMode: 'scoped' };
const selected = { ...initial, model: 'gemini-pro-orchestrator-neon', thinking: '', nativeProfile: 'agy-orchestrator' };
const previous = { ...initial, model: 'gemini-4-hydrogen-high' };
const childID = '00000000-0000-4000-8000-000000000001';
const otherChildID = '00000000-0000-4000-8000-000000000002';
const native = (fields = {}) => ({ version: 1, requestedMode: 'swarm', dispatchMethod: 'slash-command', observedAgents: 1, collaborationObserved: true,
  metadataTruncated: false, maxObservedAgents: 4, hardLimit: false, guardExceeded: false, completionScope: 'cli-invocation', invocationCompletion: 'completed',
  childCompletion: 'unknown', aggregateUsageStatus: 'unknown', childIds: [childID], omittedChildren: 0, ...fields });
const taskState = () => ({ revision: 7, status: 'paused', 'current-milestone': 'M01', packets: { [packetID]: {
  status: 'l1-failed', route: 'default', attempts: 2, 'failed-attempts': 1, evidence: { l1: { pass: false } }, schedule: { writes: ['src/'] },
  'execution-policy': { version: 1, current: 'orchestrator', history: [{ kind: 'jev-model-selection', from: 'neon', to: 'orchestrator', before: initial, after: selected,
    attempt: 2, at: '2026-11-20T11:00:00Z', reason: 'approved-model-choice' }] },
} }, run: { id: 'current-run', configurationSource: '/repo/task-config.json', configurationScope: 'task',
  initialBindings: { 'routes.default': { bindingId: 'neon', binding: initial } },
  bindings: { 'routes.default': { ...initial, model: 'legacy-declaration-must-not-replace-facts' } } },
sessions: { records: { executor: { role: 'execute', scope: packetID, configuration: previous, last: {
  id: 'original-call', 'engine-run-id': 'original-run', 'started-at': '2026-11-20T10:00:00Z', status: 'consumed', dispatch: 'accepted', outcome: 'failed',
} } } } });
const reply = (request, fields) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({
  protocol: 1, ok: true, running: false, taskDir: request.taskDir ?? '', cwd: '/repo', state: null, ...fields,
}) } } });
async function inspect(ui) {
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新');
  await act(async () => ui.container.querySelector('[data-node-id="M01-T01"]').click());
  return ui.container.querySelector('[data-node-bindings]');
}

test('node bindings distinguish initial, selected and confirmed historical calls without applying current declarations', async t => {
  const state = taskState();
  const ui = await mountPanel(t, request => reply(request, { state }));
  const view = await inspect(ui);
  const first = view.querySelector('[data-binding-fact="initial"]');
  const next = view.querySelector('[data-binding-fact="selected"]');
  const actual = view.querySelector('[data-binding-fact="actual"]');
  assert.match(first.textContent, /运行初始配置/);
  assert.match(first.textContent, /gemini-4-neon-high/);
  assert.doesNotMatch(first.textContent, /gemini-pro-orchestrator/);
  assert.match(next.textContent, /下一次已选配置/);
  assert.match(next.textContent, /gemini-pro-orchestrator-neon/);
  assert.match(next.textContent, /原生固定/);
  assert.match(actual.textContent, /最近实际调用/);
  assert.match(actual.textContent, /gemini-4-hydrogen-high/);
  assert.match(actual.textContent, /已确认派发/);
  assert.doesNotMatch(actual.textContent, /orchestrator|legacy-declaration/);
  assert.match(view.textContent, /\/repo\/task-config.json/);
  assert.match(ui.container.querySelector('[data-node-id="M01-T01"]').textContent, /最近实际调用.*Hydrogen|最近实际调用.*gemini-4-hydrogen/);
  await ui.setLocale('en');
  assert.match(view.textContent, /Initial run binding/);
  assert.match(view.textContent, /Latest confirmed dispatch/);
  assert.equal(ui.requests.length, 1, 'reading bindings does not start a session or reload current settings');
});

test('legacy declarations and changed driver defaults do not become historical initial or actual bindings', async t => {
  const state = taskState();
  delete state.run.initialBindings;
  state.run.drivers = { execute: 'mock' };
  state.packets[packetID]['execution-policy'].history = [];
  delete state.sessions.records.executor.configuration;
  state.sessions.records.executor.last.dispatch = 'unknown';
  const ui = await mountPanel(t, request => reply(request, { state }));
  const view = await inspect(ui);
  assert.match(view.querySelector('[data-binding-fact="initial"]').textContent, /没有保存可确认的初始模型配置/);
  assert.match(view.querySelector('[data-binding-fact="selected"]').textContent, /尚无可确认的后续模型选择/);
  const actual = view.querySelector('[data-binding-fact="actual"]');
  assert.match(actual.textContent, /派发未确认/);
  assert.match(actual.textContent, /没有保存完整模型配置/);
  assert.doesNotMatch(actual.textContent, /legacy-declaration|gemini-4-neon|离线/);
  const declaration = view.querySelector('[data-raw-evidence]');
  assert.match(declaration.querySelector('summary').textContent, /不作为实际模型证明/);
  assert.match(declaration.textContent, /legacy-declaration-must-not-replace-facts/);
  assert.equal(declaration.open, false);
});

test('a new policy freezes its initial candidate as the next choice before any JEV decision or model invocation', async t => {
  const state = taskState();
  state.packets[packetID]['execution-policy'] = { version: 1, current: 'neon', currentBinding: { bindingId: 'neon', binding: initial }, history: [] };
  state.sessions.records = {};
  const ui = await mountPanel(t, request => reply(request, { state }));
  const view = await inspect(ui);
  assert.match(view.querySelector('[data-binding-fact="selected"]').textContent, /gemini-4-neon-high/);
  assert.match(view.querySelector('[data-binding-fact="actual"]').textContent, /尚无已记录的模型调用/);
  assert.equal(ui.container.querySelector('[data-node-native]'), null);
  assert.match(ui.container.querySelector('[data-node-id="M01-T01"]').textContent, /下一次已选配置/);
});

test('legacy policies do not adopt a later run initial merely because the candidate ID was reused', async t => {
  const state = taskState();
  state.packets[packetID]['execution-policy'] = { version: 1, current: 'neon', history: [] };
  state.run.initialBindings['routes.default'].binding = { ...initial, model: 'later-run-model', nativeOptions: { version: 1, mode: 'swarm' } };
  const ui = await mountPanel(t, request => reply(request, { state }));
  const view = await inspect(ui);
  assert.match(view.querySelector('[data-binding-fact="initial"]').textContent, /later-run-model/);
  const next = view.querySelector('[data-binding-fact="selected"]');
  assert.match(next.textContent, /neon.*尚无可确认的后续模型选择/);
  assert.doesNotMatch(next.textContent, /later-run-model|swarm|gemini-4-hydrogen-high/);
  assert.match(view.querySelector('[data-binding-fact="actual"]').textContent, /gemini-4-hydrogen-high/);
});

test('requested collaboration modes use each frozen binding rather than profile names or another invocation', async t => {
  const state = taskState();
  state.run.initialBindings['routes.default'].binding = { ...initial, nativeProfile: 'shared-name', nativeOptions: { version: 1, mode: 'native' } };
  state.packets[packetID]['execution-policy'].history[0].after = { ...selected, nativeProfile: 'shared-name', nativeOptions: { version: 1, mode: 'swarm' } };
  state.sessions.records.executor.configuration = { ...previous, nativeProfile: 'shared-name', nativeOptions: { version: 1, mode: 'subagents' } };
  const ui = await mountPanel(t, request => reply(request, { state }));
  const view = await inspect(ui);
  assert.match(view.querySelector('[data-binding-fact="initial"]').textContent, /请求的协作方式普通原生编排（native）/);
  assert.match(view.querySelector('[data-binding-fact="selected"]').textContent, /请求的协作方式蜂群协作（swarm）/);
  assert.match(view.querySelector('[data-binding-fact="actual"]').textContent, /请求的协作方式子智能体编排（subagents）/);
  assert.equal(ui.container.querySelector('[data-native-child]'), null, 'a requested mode never invents observed children');
  await ui.setLocale('en');
  assert.match(view.querySelector('[data-binding-fact="selected"]').textContent, /Requested collaboration modeSwarm collaboration \(swarm\)/);
  for (const nativeOptions of [undefined, null, 'swarm', { version: 1, mode: 'toString' }, { version: 1, mode: '__proto__' }, { version: 1, mode: 'future' }]) {
    state.packets[packetID]['execution-policy'].history[0].after.nativeOptions = nativeOptions;
    await ui.click('Refresh');
    assert.match(view.querySelector('[data-binding-fact="selected"]').textContent, /did not retain a confirmed collaboration mode/);
    assert.doesNotMatch(view.querySelector('[data-binding-fact="selected"]').textContent, /Swarm collaboration|Native orchestration|function toString|\[object Object\]/);
  }
  assert.equal(ui.requests.length, 7, 'displaying a mode does not start or query a provider');
});

test('the frozen current binding outranks switch history and changed run defaults without filling display omissions', async t => {
  const state = taskState();
  const policy = state.packets[packetID]['execution-policy'];
  policy.currentBinding = { bindingId: policy.current, binding: { ...selected, nativeOptions: { version: 1, mode: 'swarm' } } };
  policy.history[0].after = { ...previous, nativeOptions: { version: 1, mode: 'native' } };
  state.run.initialBindings['routes.default'] = { bindingId: policy.current, binding: { ...initial, model: 'new-run-default' } };
  const ui = await mountPanel(t, request => reply(request, { state }));
  const view = await inspect(ui);
  const next = () => view.querySelector('[data-binding-fact="selected"]');
  assert.match(next().textContent, /gemini-pro-orchestrator-neon/);
  assert.match(next().textContent, /蜂群协作（swarm）/);
  assert.doesNotMatch(next().textContent, /new-run-default|gemini-4-hydrogen-high/);
  delete policy.currentBinding.binding.nativeOptions;
  policy.currentBinding.nativeOptionsOmitted = true;
  policy.currentBinding.nativeOptionsHash = 'd'.repeat(64);
  await ui.click('刷新');
  assert.match(next().textContent, /原生配置正文已从本视图省略/);
  assert.match(next().textContent, new RegExp('d'.repeat(64)));
  assert.match(next().textContent, /此记录未保留可确认的协作方式/);
  delete policy.currentBinding;
  policy.currentBindingOmitted = true;
  await ui.click('刷新');
  assert.match(next().textContent, /本视图未包含当前阶段的完整配置/);
  assert.doesNotMatch(next().textContent, /new-run-default|gemini-4-hydrogen-high|gemini-pro-orchestrator-neon/);
  delete policy.currentBindingOmitted;
  await ui.click('刷新');
  assert.match(next().textContent, /gemini-4-hydrogen-high/, 'legacy records may retain their historical selection evidence');
});

test('legacy pi and omp bindings may omit an explicit model without invalidating status or run history', async t => {
  const state = taskState();
  const emptyModel = harness => ({ harness, provider: '', channel: '', model: '', thinking: '', permission: 'danger-full-access', sessionMode: 'fresh' });
  state.run.initialBindings = { 'loops.plan': { binding: emptyModel('pi') }, 'routes.default': { binding: emptyModel('omp') } };
  delete state.packets[packetID]['execution-policy'];
  state.sessions.records.executor.configuration = emptyModel('omp');
  const run = { id: 'legacy-run', hostSessionId: 'desktop-session', taskDir: '/repo/docs/task', cwd: '/repo', startedAt: '2026-11-20', summary: { status: 'paused' }, initialBindings: state.run.initialBindings };
  const ui = await mountPanel(t, request => reply(request, { state }), { catalog: request => reply(request, { runs: [run] }) });
  const view = await inspect(ui);
  assert.doesNotMatch(ui.container.textContent, /malformed task status/);
  assert.match(view.querySelector('[data-binding-fact="initial"]').textContent, /未指定具体模型/);
  assert.match(view.querySelector('[data-binding-fact="actual"]').textContent, /omp/);
  assert.match(view.querySelector('[data-binding-fact="actual"]').textContent, /未指定具体模型/);
  await ui.click('记录');
  const history = ui.container.querySelector('[data-run-initial-bindings="legacy-run"]');
  assert.ok(history);
  assert.match(history.textContent, /pi/);
  assert.match(history.textContent, /omp/);
  assert.match(history.textContent, /未指定具体模型/);
});

test('bounded binding views retain native identity and distinguish omitted snapshots from absent history', async t => {
  const state = taskState();
  const snapshot = { bindingId: 'orchestrator', binding: selected, nativeOptionsOmitted: true, nativeOptionsHash: 'b'.repeat(64) };
  state.run.initialBindings = { 'routes.default': snapshot };
  state.packets[packetID]['execution-policy'] = { version: 1, current: 'orchestrator', currentBinding: snapshot, history: [] };
  const run = { id: 'bounded-run', hostSessionId: 'desktop-session', taskDir: '/repo/docs/task', cwd: '/repo', startedAt: '2026-11-20', summary: { status: 'paused' }, initialBindings: {}, initialBindingsOmitted: 3 };
  const ui = await mountPanel(t, request => reply(request, { state }), { catalog: request => reply(request, { runs: [run] }) });
  const view = await inspect(ui);
  for (const kind of ['initial', 'selected']) {
    const binding = view.querySelector('[data-binding-fact="' + kind + '"]');
    assert.match(binding.textContent, /agy-orchestrator/);
    assert.match(binding.querySelector('[data-native-options-omitted]').textContent, /原生配置正文已从本视图省略/);
    assert.match(binding.textContent, new RegExp('b'.repeat(64)));
    assert.match(binding.textContent, /gemini-pro-orchestrator-neon/);
  }
  state.run.initialBindings = {};
  state.run.initialBindingsOmitted = 2;
  delete state.packets[packetID]['execution-policy'].currentBinding;
  state.packets[packetID]['execution-policy'].currentBindingOmitted = true;
  await ui.click('刷新');
  assert.match(view.querySelector('[data-initial-bindings-omitted]').textContent, /省略了 2 项初始配置/);
  for (const kind of ['initial', 'selected']) {
    const binding = view.querySelector('[data-binding-fact="' + kind + '"]');
    assert.match(binding.textContent, /本视图未包含当前阶段的完整配置/);
    assert.doesNotMatch(binding.textContent, /没有保存可确认|尚无可确认|legacy-declaration/);
  }
  await ui.click('记录');
  const history = ui.container.querySelector('[data-run-initial-bindings="bounded-run"]');
  assert.match(history.textContent, /省略了 3 项初始配置/);
  assert.doesNotMatch(history.textContent, /没有保存可确认/);
  await ui.setLocale('en');
  assert.match(history.textContent, /3 initial bindings were omitted from this view/);
});

test('image identities come from the exact session manifest and uncertain dispatch is not presented as sent', async t => {
  const state = taskState();
  const invocation = state.sessions.records.executor;
  invocation.configuration = { ...previous, harness: 'claude-code-pro', model: 'claude-fable-5.2', inputImages: {
    version: 1, images: [{ mimeType: 'image/png', sha256: 'a'.repeat(64), bytes: 1234 }],
  } };
  const ui = await mountPanel(t, request => reply(request, { state }));
  await inspect(ui);
  const images = () => ui.container.querySelector('[data-invocation-images="original-call"]');
  assert.match(images().textContent, /已派发调用的图片身份/);
  assert.match(images().textContent, /Claude SDK/);
  assert.match(images().textContent, /image\/png/);
  assert.match(images().textContent, new RegExp('a'.repeat(64)));
  assert.match(images().textContent, /1234/);
  assert.equal(images().querySelector('img'), null, 'no arbitrary file or URI is fetched to invent a historical preview');
  invocation.last.dispatch = 'unknown';
  invocation.last.status = 'intent';
  invocation.last.outcome = 'unknown';
  await ui.click('刷新');
  assert.match(images().textContent, /记录的图片输入配置 · 未确认派发/);
  assert.doesNotMatch(images().textContent, /已派发调用的图片身份|传给 Claude SDK/);
  assert.match(ui.container.querySelector('[data-binding-fact="actual"]').textContent, /派发未确认/);
  invocation.last.dispatch = 'not-dispatched';
  await ui.click('刷新');
  assert.match(ui.container.querySelector('[data-binding-fact="actual"]').textContent, /最近调用记录 · 未派发/);
});

test('reviewed tasks and native observations stay separate and observations match the exact invocation', async t => {
  const state = taskState();
  let collaboration = { status: 'verified', reason: 'reviewed-contract-matched', contractFingerprint: 'a'.repeat(64), value: {
    version: 1, tasks: [{ goal: 'Implement parsing', writes: ['src/parser/'] }, { goal: 'Implement rendering', writes: ['src/render/'] }],
  } };
  let calls = [{ harness: 'antigravity', 'invocation-id': 'another-call', native: native({ childIds: [otherChildID] }) },
    { harness: 'antigravity', role: 'execute', 'packet-id': packetID, 'invocation-id': 'original-call', model: previous.model, native: native() }];
  const ui = await mountPanel(t, request => reply(request, request.action === 'metrics' ? { metrics: { groups: [], calls } }
    : { state, ...(request.action === 'node' ? { node: { id: packetID, revision: state.revision, packet: state.packets[packetID], collaboration } } : {}) }));
  await inspect(ui);
  assert.equal(ui.container.querySelector('[data-native-child]'), null);
  await ui.click('读取节点协作证据');
  const plan = () => ui.container.querySelector('[data-node-collaboration]');
  assert.match(plan().textContent, /Implement parsing/);
  assert.match(plan().textContent, /src\/render\//);
  assert.equal(ui.container.querySelector('[data-native-child]'), null, 'reviewed subtasks are not provider child observations');
  await ui.click('读取本次调用原生观察');
  assert.equal(ui.requests.at(-1).action, 'metrics');
  assert.equal(ui.requests.at(-1).runId, 'original-run', 'an old invocation reads its own run rather than the latest run');
  const observed = () => ui.container.querySelector('[data-node-native="original-call"]');
  assert.equal(observed().querySelectorAll('[data-native-child]').length, 1);
  assert.match(observed().textContent, new RegExp(childID));
  assert.doesNotMatch(observed().textContent, new RegExp(otherChildID));
  assert.match(observed().textContent, /CLI 调用完成状态已完成/);
  assert.match(observed().querySelector('[data-native-child]').textContent, /状态未知 · 模型未报告/);
  calls = calls.slice(0, 1);
  await ui.click('读取本次调用原生观察');
  assert.equal(observed().querySelector('[data-native-child]'), null, 'a different invocation cannot supply the missing observation');
  assert.match(observed().textContent, /当前指标没有包含这次调用/);
  calls = [{ harness: 'antigravity', 'invocation-id': 'original-call', native: native({ observedAgents: 0, collaborationObserved: false, childIds: [], invocationCompletion: 'failed' }) }];
  await ui.click('读取本次调用原生观察');
  assert.equal(observed().querySelector('[data-native-child]'), null);
  assert.match(observed().textContent, /尚未观察到子智能体/);
  collaboration = { status: 'unverified', reason: 'contract-changed' };
  await ui.click('读取节点协作证据');
  assert.doesNotMatch(plan().textContent, /Implement parsing/);
  assert.match(plan().textContent, /协作契约身份尚未验证/);
  collaboration = { status: 'unverified', reason: 'display-limit-exceeded' };
  await ui.click('读取节点协作证据');
  assert.match(plan().textContent, /超出本视图的显示范围/);
  assert.doesNotMatch(plan().textContent, /Implement parsing|此节点没有已审协作分工/);
  assert.ok(ui.requests.every(request => ['status', 'node', 'metrics'].includes(request.action)));
});

test('pending or failed native observation reads do not claim a completed lookup with missing invocation data', async t => {
  const state = taskState();
  const pending = deferred();
  let metricsCalls = 0;
  const ui = await mountPanel(t, request => request.action === 'metrics' ? ++metricsCalls === 1 ? pending.promise
    : reply(request, { metrics: { groups: [], calls: [] } }) : reply(request, { state }));
  await inspect(ui);
  const observed = () => ui.container.querySelector('[data-node-native="original-call"]');
  await ui.click('读取本次调用原生观察');
  assert.match(observed().textContent, /本次调用尚无可用的原生观察/);
  assert.doesNotMatch(observed().textContent, /当前指标没有包含这次调用/);
  await act(async () => pending.resolve({ ok: false, error: { message: 'The observation lookup failed' } }));
  assert.match(ui.container.textContent, /The observation lookup failed/);
  assert.match(observed().textContent, /本次调用尚无可用的原生观察/);
  assert.doesNotMatch(observed().textContent, /当前指标没有包含这次调用/);
  await ui.click('读取本次调用原生观察');
  assert.match(observed().textContent, /当前指标没有包含这次调用/);
  assert.equal(metricsCalls, 2);
});

test('invalid current binding identities retain the last verified selection and block starting', async t => {
  const state = taskState();
  const policy = state.packets[packetID]['execution-policy'];
  policy.currentBinding = { bindingId: policy.current, binding: selected };
  const ui = await mountPanel(t, request => reply(request, { state }));
  const view = await inspect(ui);
  for (const invalid of [
    { currentBinding: { bindingId: 'foreign-candidate', binding: previous } },
    { currentBinding: { bindingId: policy.current, binding: { model: {} } } },
    { currentBinding: { bindingId: policy.current, binding: selected }, currentBindingOmitted: true },
    { currentBindingOmitted: 'true' },
    { current: '', currentBinding: { bindingId: '', binding: selected } },
    { current: '', currentBinding: null, currentBindingOmitted: true },
  ]) {
    const original = structuredClone(policy);
    Object.assign(policy, invalid);
    await ui.click('刷新');
    assert.match(ui.container.textContent, /malformed task status/);
    assert.match(view.querySelector('[data-binding-fact="selected"]').textContent, /gemini-pro-orchestrator-neon/);
    assert.equal(ui.button('修复并重试').disabled, true);
    for (const key of Object.keys(policy)) delete policy[key];
    Object.assign(policy, original);
    await ui.click('刷新');
  }
});

test('malformed new binding and collaboration facts retain the last verified view', async t => {
  const state = taskState();
  let badRun = null;
  const ui = await mountPanel(t, request => reply(request, { state: badRun ? { ...state, run: { ...state.run, ...badRun } } : state,
    ...(request.action === 'node' ? { node: { id: packetID, revision: 7, packet: state.packets[packetID], collaboration: {
      status: 'unverified', reason: 'changed', value: { version: 1, tasks: [{ goal: 'forged', writes: ['src/'] }, { goal: 'forged2', writes: ['tests/'] }] },
    } } } : {}) }));
  const view = await inspect(ui);
  for (const invalid of [
    { initialBindings: { 'routes.default': { binding: { model: {} } } } },
    { initialBindingsOmitted: -1 },
    { initialBindingsOmitted: '2' },
    { initialBindings: { 'routes.default': { binding: selected, nativeOptionsOmitted: true } } },
    { initialBindings: { 'routes.default': { binding: selected, nativeOptionsHash: 'b'.repeat(64) } } },
    { initialBindings: { 'routes.default': { binding: { ...selected, nativeOptions: {} }, nativeOptionsOmitted: true, nativeOptionsHash: 'b'.repeat(64) } } },
  ]) {
    badRun = invalid;
    await ui.click('刷新');
    assert.match(ui.container.textContent, /malformed task status/);
    assert.match(view.querySelector('[data-binding-fact="initial"]').textContent, /gemini-4-neon-high/);
    badRun = null;
    await ui.click('刷新');
  }
  await ui.click('读取节点协作证据');
  assert.match(ui.container.textContent, /Malformed node detail/);
  assert.doesNotMatch(ui.container.querySelector('[data-node-collaboration]').textContent, /forged/);
});

test('run history displays the initial candidate snapshot without interpreting legacy routes as execution', async t => {
  const run = { id: 'history-run', hostSessionId: 'desktop-session', taskDir: '/repo/docs/task', cwd: '/repo', startedAt: '2026-11-20', summary: { status: 'paused' },
    configurationSource: '/repo/history-config.json', configurationScope: 'project',
    initialBindings: { 'routes.default': { bindingId: 'orchestrator', binding: selected } }, routes: { default: previous } };
  const ui = await mountPanel(t, request => reply(request, {}), { catalog: request => reply(request, { runs: [run] }) });
  await ui.click('记录');
  const view = ui.container.querySelector('[data-run-initial-bindings="history-run"]');
  assert.ok(view);
  assert.match(view.textContent, /gemini-pro-orchestrator-neon/);
  assert.match(view.textContent, /原生固定/);
  assert.match(view.textContent, /\/repo\/history-config.json/);
  assert.doesNotMatch(view.textContent, /gemini-4-hydrogen-high/);
  await ui.setLocale('en');
  assert.match(view.textContent, /Initial run binding/);
  assert.match(view.textContent, /Execute/);
});

test('subscription JEV diagnostics use the resolved profile binding when the legacy model field is absent', async t => {
  const jev = { enabled: true, backend: 'subscription', profile: 'decision-neon', 'max-calls': 8 };
  const values = { routes: { default: initial }, loops: {}, jev };
  let resolved = { ...initial, profile: 'decision-neon', role: 'jev', thinking: 'medium', model: 'gemini-4-neon-medium' };
  const ui = await mountPanel(t, request => reply(request, request.action === 'config' ? { configuration: {
    source: '/repo/task-config.json', scope: 'task', bindings: [], jev, ...(resolved ? { jevBinding: resolved } : {}),
  } } : request.action === 'config-edit' ? { editor: { scope: request.editor.scope, target: '/global/config.json', revision: 'fixture-one', values, defaults: values } } : {}));
  await ui.click('设置');
  await ui.click('诊断与配置文件');
  await ui.click('查看模型配置');
  let summary = ui.container.querySelector('[data-jev-binding]');
  assert.ok(summary);
  assert.match(summary.textContent, /decision-neon/);
  assert.match(summary.textContent, /gemini-4-neon-medium/);
  assert.doesNotMatch(ui.container.textContent, /malformed model configuration/);
  await ui.setLocale('en');
  assert.match(summary.textContent, /Medium/);
  resolved = null;
  await ui.click('Inspect model configuration');
  summary = ui.container.querySelector('[data-jev-binding]');
  assert.match(summary.textContent, /did not retain a complete model binding/);
  assert.doesNotMatch(summary.textContent, /gemini-4-neon-medium/);
});
