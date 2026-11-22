import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, mountPanel, closeClientHarness, deferred } from './support/client-harness.js';

after(closeClientHarness);

const reply = (request, fields) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({
  protocol: 1, ok: true, running: false, cwd: request.cwd ?? '/repo', taskDir: request.taskDir ?? '', state: null, ...fields,
}) } } });
const agy = { harness: 'antigravity', provider: '', channel: 'google-subscription', model: 'gemini-4-neon-high', thinking: 'high' };
const complete = value => ({ ...value, permission: 'danger-full-access', 'session-mode': 'scoped' });
const route = (initial, allowed) => ({ initial, allowed, 'quality-order': [], 'after-stalled-attempts': 1, 'max-switches': 2 });
function configuration() {
  return {
    sessions: { mode: 'scoped', 'policy-version': 1 }, permissions: { mode: 'full-access' },
    scheduler: { mode: 'serial', 'max-parallel': 2 }, budget: { enforce: 'off' },
    retry: { 'max-transport-retries': 3 }, watchdog: { 'wall-timeout-ms': 60000 },
    loops: { plan: { ...agy, 'native-profile': 'shared-native' }, review: { ...agy }, judge: { ...agy } },
    routes: { default: { ...agy }, hard: { ...agy } },
    planning: { compact: { 'design-plan': 'planner', 'design-review': 'planner' } },
    profiles: {
      ordinary: { ...agy, model: 'gemini-4-hydrogen-medium', thinking: 'medium' },
      planner: { ...agy, 'native-profile': 'shared-native' },
      swarm: { ...agy, 'native-profile': 'shared-swarm' },
      orchestrator: { ...agy, model: 'gemini-pro-orchestrator-neon', thinking: '', 'native-profile': 'shared-native' },
    },
    'native-profiles': {
      'shared-native': { harness: 'antigravity', options: { version: 1, mode: 'native', maxObservedAgents: 4 } },
      'shared-subagents': { harness: 'antigravity', options: { version: 1, mode: 'subagents', maxObservedAgents: 4 } },
      'shared-swarm': { harness: 'antigravity', options: { version: 1, mode: 'swarm', maxObservedAgents: 4 } },
    },
    'execution-policy': { version: 1, bindings: {
      shared: complete({ ...agy, 'native-profile': 'shared-native' }),
      claude: complete({ harness: 'claude-code-pro', provider: '', channel: 'claude-native', model: 'claude-opus-5-5', thinking: 'medium' }),
      swarm: complete({ ...agy, 'native-profile': 'shared-swarm' }),
      local: complete({ ...agy, model: 'gemini-4-hydrogen-medium', thinking: 'medium' }),
      rule: complete({ ...agy, model: 'gemini-4-carbon-high' }),
      otherAuthority: { ...complete(agy), permission: 'workspace-write', 'session-mode': 'fresh' },
    }, routes: {
      default: { ...route('shared', ['shared', 'claude', 'swarm', 'rule']), 'quality-order': ['rule'] },
      hard: route('shared', ['shared']),
    } },
    channels: { 'google-subscription': { 'env-key': null }, 'claude-native': { 'env-key': null } },
    jev: { enabled: false, backend: 'subscription', profile: 'ordinary', 'timeout-ms': 5000, 'max-calls': 8,
      'min-applicability': 0.8, 'min-choice': 0.6, 'allowed-data': ['task-summary', 'diagnostic-facts'] },
  };
}
function fixture(initial = configuration(), options = {}) {
  let stored = structuredClone(initial), revision = 1;
  const report = request => {
    const effective = options.effective?.(stored, request) ?? { values: stored, scope: 'global', source: '/global/config.json' };
    const values = effective.values, profile = values.profiles?.[values.jev?.profile];
    return { source: effective.source, scope: effective.scope, cwd: request.cwd ?? '/repo', taskDir: request.taskDir ?? '', bindings: [],
      jev: { ...values.jev, model: values.jev?.model ?? '' },
      ...(profile ? { jevBinding: { id: 'jev', profile: values.jev.profile, bindingSource: 'profiles.' + values.jev.profile, ...profile } } : {}),
    };
  };
  const respond = async request => {
    if (request.action === 'models' && options.models) return options.models(request);
    if (request.action === 'config') {
      if (options.config) return options.config(request, report(request));
      return reply(request, { configuration: report(request) });
    }
    assert.ok(['config-edit', 'config-save'].includes(request.action), request.action);
    if (request.action === 'config-save') {
      if (options.save) return options.save(request);
      stored = { ...stored, ...structuredClone(request.editor.values) };
      revision += 1;
    }
    return reply(request, { editor: { scope: request.editor.scope, target: '/' + request.editor.scope + '/config.json',
      source: '/' + request.editor.scope + '/config.json', revision: String(revision), values: stored, defaults: initial,
      applied: request.action === 'config-save' } });
  };
  return { respond, get stored() { return stored; } };
}
const saveRequest = ui => ui.requests.filter(request => request.action === 'config-save').at(-1);
const draft = ui => JSON.parse(ui.container.querySelector('textarea[aria-label="配置字段（JSON）"]').value);
function field(ui, label) {
  const node = [...ui.container.querySelectorAll('label')].find(node => node.textContent === label);
  assert.ok(node, 'field ' + label);
  return ui.document.getElementById(node.htmlFor) ?? node.querySelector('input,select');
}
async function change(ui, element, value) {
  assert.ok(element);
  await act(async () => {
    if (element.type === 'checkbox') { if (element.checked !== value) element.click(); return; }
    const prototype = element.tagName === 'SELECT' ? ui.document.defaultView.HTMLSelectElement.prototype : ui.document.defaultView.HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(prototype, 'value').set.call(element, value);
    element.dispatchEvent(new ui.document.defaultView.Event(element.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true }));
  });
}
const bindingNative = (ui, id) => ui.container.querySelector(`[data-binding-id="${id}"] select[aria-label$="原生能力"]`);
const candidateCheck = (ui, route, id) => ui.container.querySelector(`[data-policy-route="${route}"] input[aria-label$=" · ${id}"]`);
const labelledButton = (ui, label) => [...ui.container.querySelectorAll('button')].find(button => button.getAttribute('aria-label') === label);
const clickLabel = async (ui, label) => act(async () => {
  const button = labelledButton(ui, label);
  assert.ok(button, label);
  button.click();
});
const modelReport = (request, models, extra = {}) => reply(request, { modelCatalog: {
  ...request.models, status: 'available', source: 'agy models', scope: 'native-client-catalog', models,
  inferencePerformed: false, accountEntitlement: 'unknown', limitations: [], truncated: false, ...extra,
} });

test('Antigravity Swarm choices enforce actual roles, including inherited and compact planning roles', async t => {
  const source = fixture();
  const ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  for (const role of ['plan', 'design-plan', 'review', 'design-review', 'plan-review', 'judge']) {
    const select = bindingNative(ui, 'loops.' + role);
    const swarm = [...select.options].find(option => option.value === 'shared-swarm');
    assert.equal(swarm.disabled, true, role);
    assert.match(swarm.textContent, /仅用于 Execute/);
    await change(ui, select, 'shared-swarm');
    assert.notEqual(draft(ui).loops[role]?.['native-profile'], 'shared-swarm', 'a synthetic change cannot bypass the role guard');
  }
  for (const label of ['联合方案生成', '联合方案审查']) {
    const select = field(ui, label);
    assert.equal([...select.options].find(option => option.value === 'swarm').disabled, true);
    await change(ui, select, 'swarm');
  }
  assert.deepEqual(draft(ui).planning, source.stored.planning);
  const execute = bindingNative(ui, 'routes.default');
  assert.equal([...execute.options].find(option => option.value === 'shared-swarm').disabled, false);
  await change(ui, execute, 'shared-swarm');
  assert.match(ui.container.textContent, /Full.*已审批.*2–8.*无原生图片输入/);
  assert.match(ui.container.textContent, /没有图片的任务可以使用/);
  assert.match(ui.container.textContent, /不代表当前任务已满足条件/);
  await ui.click('保存修改');
  const value = source.stored, initial = value['execution-policy'].routes.default.initial;
  assert.equal(value['execution-policy'].bindings[initial]['native-profile'], 'shared-swarm');
  assert.equal(value['execution-policy'].bindings.shared['native-profile'], 'shared-native');
  assert.deepEqual(ui.requests.map(request => request.action), ['config-edit', 'config', 'config-save', 'config']);
});

test('an existing illegal non-Execute Swarm remains visible and must be repaired before saving', async t => {
  const original = configuration(); original.loops.review['native-profile'] = 'shared-swarm';
  const ui = await mountPanel(t, fixture(original).respond);
  await ui.click('设置'); await ui.click('模型与会话');
  const select = bindingNative(ui, 'loops.review');
  assert.equal(select.value, 'shared-swarm');
  assert.equal(select.selectedOptions[0].disabled, true);
  assert.equal(ui.button('保存修改').disabled, true);
  await change(ui, select, 'shared-subagents');
  assert.equal(ui.button('保存修改').disabled, false);
  await ui.click('保存修改');
  assert.equal(saveRequest(ui).editor.values.loops.review['native-profile'], 'shared-subagents');
});

test('an invalid native-profile draft stays repairable when reopening graphical model settings', async t => {
  const original = configuration(), source = fixture(original);
  const ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('诊断与配置文件');
  const value = structuredClone(original);
  value['native-profiles']['shared-native'] = null;
  const area = ui.container.querySelector('textarea[aria-label="配置字段（JSON）"]');
  await act(async () => {
    Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLTextAreaElement.prototype, 'value').set.call(area, JSON.stringify(value));
    area.dispatchEvent(new ui.document.defaultView.Event('input', { bubbles: true }));
  });
  await ui.click('模型与会话');
  assert.equal(ui.button('保存修改').disabled, true);
  assert.match(ui.container.textContent, /native-profiles.shared-native/);
  await ui.click('诊断与配置文件');
  assert.equal(draft(ui)['native-profiles']['shared-native'], null, 'invalid JSON intent is retained for an explicit repair');
  await ui.click('撤销');
  await ui.click('模型与会话');
  assert.equal(bindingNative(ui, 'loops.plan').value, 'shared-native');
  assert.deepEqual(source.stored, original);
  assert.equal(ui.requests.some(request => request.action === 'config-save'), false);
});

test('graphical initial, allowlist and switch-limit controls preserve complete and shared bindings', async t => {
  const original = configuration(), source = fixture(original);
  const ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  assert.equal(candidateCheck(ui, 'default', 'shared').disabled, true, 'initial cannot be removed');
  assert.equal(candidateCheck(ui, 'default', 'rule').disabled, true, 'referenced quality candidate cannot be removed');
  assert.equal(candidateCheck(ui, 'default', 'otherAuthority').disabled, true, 'different permissions/session cannot be silently selected');
  await change(ui, field(ui, '普通执行 · 初始配置'), 'claude');
  await change(ui, candidateCheck(ui, 'default', 'shared'), false);
  await change(ui, candidateCheck(ui, 'default', 'local'), true);
  await change(ui, field(ui, '普通执行 · 最多切换次数'), '0');
  assert.equal(ui.container.querySelector('[data-binding-id="routes.default"] input').value, 'claude-opus-5-5');
  await ui.click('保存修改');
  const policy = source.stored['execution-policy'];
  assert.equal(policy.routes.default.initial, 'claude');
  assert.deepEqual(policy.routes.default.allowed, ['claude', 'swarm', 'rule', 'local']);
  assert.equal(policy.routes.default['max-switches'], 0);
  assert.deepEqual(policy.bindings, original['execution-policy'].bindings);
  assert.deepEqual(policy.routes.hard, original['execution-policy'].routes.hard);
  assert.deepEqual(source.stored.routes, original.routes, 'policy initial does not overwrite the baseline route');
  assert.deepEqual(Object.keys(saveRequest(ui).editor.values), ['execution-policy']);
  assert.equal(field(ui, '普通执行 · 初始配置').value, 'claude', 'confirmed save round-trips the actual selected initial');
  await change(ui, field(ui, '普通执行 · 最多切换次数'), '33');
  assert.equal(ui.button('保存修改').disabled, true);
  await change(ui, field(ui, '普通执行 · 最多切换次数'), '32');
  assert.equal(ui.button('保存修改').disabled, false);
});

test('native option edits clone only the route target and retain shared profiles, authorities and sessions', async t => {
  const original = configuration(), source = fixture(original);
  const ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  await act(async () => ui.container.querySelector('[data-binding-id="routes.default"] button').click());
  await change(ui, field(ui, '原生协作方式'), 'subagents');
  await change(ui, field(ui, '观察到的子代理总数上限'), '3');
  assert.match(ui.container.textContent, /不是硬并发上限/);
  await ui.click('保存修改');
  const value = source.stored, initial = value['execution-policy'].routes.default.initial;
  const binding = value['execution-policy'].bindings[initial], native = binding['native-profile'];
  assert.notEqual(native, 'shared-native');
  assert.deepEqual(value['native-profiles'][native], { harness: 'antigravity', options: { version: 1, mode: 'subagents', maxObservedAgents: 3 } });
  assert.deepEqual(value['native-profiles']['shared-native'], original['native-profiles']['shared-native']);
  assert.deepEqual(value['execution-policy'].bindings.shared, original['execution-policy'].bindings.shared);
  assert.deepEqual(value['execution-policy'].routes.hard, original['execution-policy'].routes.hard);
  assert.deepEqual(value.loops, original.loops);
  assert.deepEqual(value.profiles, original.profiles);
  assert.equal(binding.permission, 'danger-full-access'); assert.equal(binding['session-mode'], 'scoped');
  assert.equal(saveRequest(ui).editor.values.permissions, undefined); assert.equal(saveRequest(ui).editor.values.sessions, undefined);
  await change(ui, field(ui, '观察到的子代理总数上限'), '9'); assert.equal(ui.button('保存修改').disabled, true);
  await change(ui, field(ui, '观察到的子代理总数上限'), '');
  await ui.click('保存修改');
  assert.equal(Object.hasOwn(source.stored['native-profiles'][native].options, 'maxObservedAgents'), false);
  assert.equal(Object.keys(source.stored['native-profiles']).length, 4, 'repeat edits reuse only this target’s unshared generated profile');
});

test('a previously generated native profile is cloned again when another route shares its candidate', async t => {
  const original = configuration(), name = 'ui-routes-default-native';
  original['native-profiles'][name] = structuredClone(original['native-profiles']['shared-native']);
  original['execution-policy'].bindings.shared['native-profile'] = name;
  const source = fixture(original), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  await act(async () => ui.container.querySelector('[data-binding-id="routes.default"] button').click());
  await change(ui, field(ui, '原生协作方式'), 'subagents'); await ui.click('保存修改');
  const policy = source.stored['execution-policy'], initial = policy.bindings[policy.routes.default.initial];
  assert.notEqual(initial['native-profile'], name);
  assert.deepEqual(source.stored['native-profiles'][name], original['native-profiles'][name]);
  assert.deepEqual(policy.bindings.shared, original['execution-policy'].bindings.shared);
  assert.equal(policy.routes.hard.initial, 'shared');
});

test('an unresolved subscription profile stays visible and cannot be saved as an implicitly selected model', async t => {
  const original = configuration(); original.jev.profile = 'missing-profile';
  const source = fixture(original), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('JEV'); await change(ui, field(ui, '启用 JEV'), true);
  const profile = field(ui, '判断模型配置');
  assert.equal(profile.value, 'missing-profile'); assert.equal(profile.selectedOptions[0].disabled, true);
  assert.match(profile.selectedOptions[0].textContent, /判断模型未解析/);
  assert.equal(ui.button('保存修改').disabled, true);
  await change(ui, profile, 'ordinary'); await ui.click('保存修改');
  assert.equal(source.stored.jev.profile, 'ordinary'); assert.equal(source.stored.jev.enabled, true);
});

test('subscription JEV shows effort, edits allowed data, and does not claim a global switch overrides a task file', async t => {
  const original = configuration(); original.jev.enabled = true;
  const task = structuredClone(original);
  const source = fixture(original, { effective: () => ({ values: task, scope: 'task', source: '/repo/docs/task/flywheel.config.json' }) });
  const ui = await mountPanel(t, source.respond);
  await ui.selectPath(0, '/repo/docs/task'); await ui.click('设置'); await ui.click('JEV');
  assert.match(field(ui, '判断模型配置').selectedOptions[0].textContent, /gemini-4-hydrogen-medium · medium/);
  assert.deepEqual([...field(ui, '判断模型配置').options].map(option => option.value), ['ordinary']);
  const scope = () => ui.container.querySelector('[data-jev-scope]');
  assert.match(scope().textContent, /正在编辑的文件全局 · 开启/);
  assert.match(scope().textContent, /当前任务实际采用的配置任务 · 开启/);
  await change(ui, ui.container.querySelector('input[aria-label="允许发送的数据类别 · source"]'), true);
  await ui.click('保存修改');
  assert.deepEqual(source.stored.jev['allowed-data'], ['task-summary', 'diagnostic-facts', 'source']);
  await change(ui, field(ui, '启用 JEV'), false); await ui.click('保存修改');
  assert.equal(source.stored.jev.enabled, false); assert.equal(task.jev.enabled, true);
  assert.match(scope().textContent, /正在编辑的文件全局 · 关闭/);
  assert.match(scope().textContent, /当前任务实际采用的配置任务 · 开启/);
  assert.match(scope().textContent, /保存此处的开关不会越过配置优先级/);
  assert.deepEqual(ui.requests.map(request => request.action), ['config-edit', 'config', 'config-save', 'config', 'config-save', 'config']);
});

test('an effective read failure leaves a confirmed save intact and reports unknown until an explicit retry', async t => {
  let fail = false;
  const source = fixture(configuration(), { config: (request, configuration) => fail ? Promise.reject(new Error('effective read unavailable')) : reply(request, { configuration }) });
  const ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('JEV');
  await change(ui, field(ui, '启用 JEV'), true); fail = true; await ui.click('保存修改');
  assert.match(ui.container.querySelector('footer').textContent, /配置已保存/);
  assert.match(ui.container.querySelector('[data-jev-scope]').textContent, /当前有效配置尚未确认/);
  assert.equal(ui.requests.filter(request => request.action === 'config').length, 2);
  await ui.click('模型与会话'); await ui.click('JEV');
  assert.equal(ui.requests.filter(request => request.action === 'config').length, 2, 'failed reads do not loop on render');
  fail = false; await ui.click('重新核对当前生效配置');
  assert.match(ui.container.querySelector('[data-jev-scope]').textContent, /当前任务实际采用的配置全局 · 开启/);
});

test('an explicit diagnostics configuration read updates the JEV card and a later failed read leaves it unknown', async t => {
  let useTask = false, fail = false;
  const task = configuration(); task.jev.enabled = true;
  const source = fixture(configuration(), {
    effective: stored => useTask ? { values: task, scope: 'task', source: '/repo/docs/task/flywheel.config.json' }
      : { values: stored, scope: 'global', source: '/global/config.json' },
    config: (request, configuration) => fail ? Promise.reject(new Error('explicit read unavailable')) : reply(request, { configuration }),
  });
  const ui = await mountPanel(t, source.respond);
  await ui.selectPath(0, '/repo/docs/task'); await ui.click('设置'); await ui.click('JEV');
  const scope = () => ui.container.querySelector('[data-jev-scope]').textContent;
  assert.match(scope(), /当前任务实际采用的配置全局 · 关闭/);
  useTask = true;
  await ui.click('诊断与配置文件'); await ui.click('查看模型配置'); await ui.click('JEV');
  assert.match(scope(), /当前任务实际采用的配置任务 · 开启/);
  assert.match(scope(), /\/repo\/docs\/task\/flywheel.config.json/);
  fail = true;
  await ui.click('诊断与配置文件'); await ui.click('查看模型配置'); await ui.click('JEV');
  assert.match(scope(), /当前有效配置尚未确认/);
  assert.doesNotMatch(scope(), /当前任务实际采用的配置任务 · 开启/);
  assert.deepEqual(ui.requests.map(request => request.action), ['config-edit', 'config', 'config', 'config']);
});

test('an unknown save is not retried or treated as a new effective configuration on remount', async t => {
  const source = fixture(configuration(), { save: () => Promise.reject(new Error('save receipt lost')) });
  const ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('JEV'); await change(ui, field(ui, '启用 JEV'), true); await ui.click('保存修改');
  assert.match(ui.container.textContent, /保存结果未确认/);
  assert.match(ui.container.querySelector('[data-jev-scope]').textContent, /当前有效配置尚未确认/);
  assert.doesNotMatch(ui.container.querySelector('[data-jev-scope]').textContent, /当前任务实际采用的配置全局 · 关闭/,
    'the previous read cannot establish the current switch after an uncertain write');
  await ui.setVisible(false); await ui.setVisible(true);
  assert.equal(ui.button('保存修改').disabled, true);
  assert.deepEqual(ui.requests.map(request => request.action), ['config-edit', 'config', 'config-save']);
});

test('a late save in a different conversation does not trigger an old-task effective read or steal selection', async t => {
  const pending = deferred();
  let saved;
  const source = fixture(configuration(), { save: request => { saved = request; return pending.promise; } });
  const ui = await mountPanel(t, source.respond);
  await ui.selectPath(0, '/repo/docs/a'); await ui.click('设置'); await ui.click('JEV');
  await change(ui, field(ui, '启用 JEV'), true); await ui.click('保存修改');
  await ui.switchSession('another-conversation');
  await ui.selectPath(0, '/repo/docs/b'); await ui.click('设置'); await ui.click('JEV');
  await act(async () => pending.resolve(reply(saved, { editor: { scope: 'global', target: '/global/config.json', source: '/global/config.json',
    revision: 'saved-a', values: { ...configuration(), ...saved.editor.values }, defaults: configuration(), applied: true } })));
  assert.deepEqual(ui.requests.filter(request => request.action === 'config').map(request => request.taskDir), ['/repo/docs/a', '/repo/docs/b']);
  assert.equal(ui.requests.at(-1).taskDir, '/repo/docs/b');
  assert.equal(field(ui, '启用 JEV').checked, false, 'the late save did not replace the newly selected task draft');
});

test('an effective read completing after task navigation cannot replace the newly selected source', async t => {
  const pending = deferred(); let first;
  const source = fixture(configuration(), { config: (request, configuration) => {
    if (!first) { first = request; return pending.promise; }
    return reply(request, { configuration: { ...configuration, scope: 'task', source: '/repo/docs/b/flywheel.config.json' } });
  } });
  const ui = await mountPanel(t, source.respond);
  await ui.selectPath(0, '/repo/docs/a'); await ui.click('设置');
  await ui.selectPath(0, '/repo/docs/b');
  await act(async () => pending.resolve(reply(first, { configuration: { source: '/repo/docs/a/flywheel.config.json', scope: 'task', bindings: [] } })));
  await ui.click('JEV');
  assert.match(ui.container.querySelector('[data-jev-scope]').textContent, /\/repo\/docs\/b\/flywheel.config.json/);
  assert.doesNotMatch(ui.container.querySelector('[data-jev-scope]').textContent, /\/repo\/docs\/a\//);
  assert.deepEqual(ui.requests.map(request => [request.action, request.taskDir]), [
    ['config-edit', '/repo/docs/a'], ['config', '/repo/docs/a'], ['config-edit', '/repo/docs/b'], ['config', '/repo/docs/b'],
  ]);
});

test('a copied candidate edits every explicit field without changing its source or automatically authorizing it', async t => {
  const original = configuration(), source = fixture(original), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  assert.equal(labelledButton(ui, '删除候选 shared').disabled, true);
  await clickLabel(ui, '复制候选 shared');
  await change(ui, field(ui, 'Provider'), 'claude-code-pro');
  await change(ui, field(ui, '模型'), 'claude-opus-5-5');
  await change(ui, field(ui, '推理等级'), 'low');
  await change(ui, field(ui, '原生能力'), '');
  await change(ui, field(ui, '阶段权限要求'), 'workspace-write');
  await change(ui, field(ui, '会话策略'), 'fresh');
  await change(ui, field(ui, '账号渠道'), 'new');
  await change(ui, field(ui, '账号渠道'), 'new-claude-subscription');
  await ui.click('保存修改');
  assert.deepEqual(source.stored['execution-policy'].bindings['shared-copy'], {
    harness: 'claude-code-pro', provider: '', channel: 'new-claude-subscription', model: 'claude-opus-5-5', thinking: 'low',
    permission: 'workspace-write', 'session-mode': 'fresh',
  });
  assert.deepEqual(source.stored['execution-policy'].bindings.shared, original['execution-policy'].bindings.shared);
  assert.deepEqual(source.stored['execution-policy'].routes, original['execution-policy'].routes);
  assert.deepEqual(source.stored['native-profiles'], original['native-profiles']);
  assert.deepEqual(source.stored.channels['new-claude-subscription'], { 'env-key': null });
  assert.equal(source.stored.channels.new, undefined, 'partial typed channels never become persisted credentials or providers');
  await ui.click('← 返回模型列表');
  assert.equal(labelledButton(ui, '删除候选 shared-copy').disabled, false);
  await clickLabel(ui, '删除候选 shared-copy');
  assert.equal(draft(ui)['execution-policy'].bindings['shared-copy'], undefined);
});

test('explicit route authorization isolates shared candidates and remaps every reference while preserving other routes', async t => {
  const original = configuration();
  original['execution-policy'].routes.default['quality-order'] = ['shared', 'rule'];
  original['execution-policy'].routes.default['channel-fallbacks'] = { shared: 'claude' };
  original.channels['google-subscription'].fallback = 'claude-native';
  Object.assign(original.channels['claude-native'], { harness: 'claude-code-pro', models: { default: 'claude-opus-5-5' } });
  const source = fixture(original), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  await change(ui, field(ui, '普通执行 · 路线授权权限'), 'read-only');
  const next = draft(ui), route = next['execution-policy'].routes.default;
  assert.notEqual(route.initial, 'shared');
  assert.equal(route.allowed.includes('shared'), false);
  assert.deepEqual(route['quality-order'], [route.initial, 'rule']);
  assert.deepEqual(route['channel-fallbacks'], { [route.initial]: 'claude' });
  assert.ok(route.allowed.every(id => next['execution-policy'].bindings[id].permission === 'read-only'));
  assert.deepEqual(next['execution-policy'].bindings.shared, original['execution-policy'].bindings.shared);
  assert.deepEqual(next['execution-policy'].routes.hard, original['execution-policy'].routes.hard);
  assert.equal(next.permissions.bindings['routes.default'], 'read-only');
  assert.equal(next.permissions.bindings['routes.hard'], undefined);
  assert.deepEqual(next.routes, original.routes);
  await ui.click('保存修改');
  assert.equal(source.stored['execution-policy'].bindings.shared.permission, 'danger-full-access');
});

test('candidate permission mismatches require explicit route reconciliation while session differences stay selectable', async t => {
  const original = configuration();
  original['execution-policy'].bindings.local['session-mode'] = 'fresh';
  const source = fixture(original), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  assert.equal(candidateCheck(ui, 'default', 'local').disabled, false);
  await change(ui, candidateCheck(ui, 'default', 'local'), true);
  await clickLabel(ui, '编辑执行候选 local');
  await change(ui, field(ui, '阶段权限要求'), 'read-only');
  assert.equal(ui.button('保存修改').disabled, true);
  await ui.click('← 返回模型列表');
  assert.match(ui.container.textContent, /候选权限与路线授权不符/);
  await ui.click('按此路线授权同步候选');
  assert.equal(draft(ui)['execution-policy'].bindings.local.permission, 'danger-full-access');
  assert.equal(draft(ui)['execution-policy'].bindings.local['session-mode'], 'fresh');
  assert.equal(ui.button('保存修改').disabled, false);
  await ui.click('保存修改');
});

test('quality order and channel fallback rules are editable without JSON and require explicit channel mapping changes', async t => {
  const source = fixture(), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  await clickLabel(ui, '移出质量次序 rule');
  assert.equal(candidateCheck(ui, 'default', 'rule').disabled, false);
  await change(ui, candidateCheck(ui, 'default', 'rule'), false);
  const quality = ui.container.querySelector('[data-quality-route="default"] select');
  await change(ui, quality, 'shared');
  await change(ui, quality, 'claude');
  await clickLabel(ui, '上移 claude');
  assert.deepEqual(draft(ui)['execution-policy'].routes.default['quality-order'], ['claude', 'shared']);
  await change(ui, ui.container.querySelector('[data-policy-route="default"] [data-fallback-from="shared"] select'), 'claude');
  assert.equal(draft(ui).channels['google-subscription'].fallback, undefined);
  assert.equal(ui.button('保存修改').disabled, true, 'a rule cannot silently grant a different channel path');
  assert.match(ui.container.textContent, /目标模型 claude-opus-5-5/);
  await ui.click('应用所列渠道映射');
  assert.equal(draft(ui).channels['google-subscription'].fallback, 'claude-native');
  assert.deepEqual(draft(ui).channels['claude-native'].models, { default: 'claude-opus-5-5' });
  assert.equal(ui.button('保存修改').disabled, false);
  assert.equal(candidateCheck(ui, 'default', 'claude').disabled, true, 'quality and fallback references protect the allowed set');
  await ui.click('保存修改');
  assert.deepEqual(source.stored['execution-policy'].routes.default['channel-fallbacks'], { shared: 'claude' });
  await change(ui, ui.container.querySelector('[data-policy-route="default"] [data-fallback-from="shared"] select'), '');
  assert.equal(draft(ui).channels['google-subscription'].fallback, 'claude-native', 'removing the rule preserves the shared chain until an explicit edit');
  await clickLabel(ui, '移除渠道后备链 google-subscription');
  assert.equal(draft(ui).channels['google-subscription'].fallback, undefined);
});

test('fallback mapping refuses cycles and conflicting rules without changing the shared channel graph', async t => {
  const original = configuration();
  original.channels['claude-native'].fallback = 'google-subscription';
  const source = fixture(original), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  await change(ui, ui.container.querySelector('[data-policy-route="default"] [data-fallback-from="shared"] select'), 'claude');
  assert.match(ui.container.textContent, /此后备链会形成循环/);
  assert.equal(ui.button('保存修改').disabled, true);
  assert.deepEqual(draft(ui).channels, original.channels);
  await clickLabel(ui, '移除渠道后备链 claude-native');
  await ui.click('应用所列渠道映射');
  assert.equal(ui.button('保存修改').disabled, false);
  await ui.click('保存修改');
  assert.equal(source.stored.channels['claude-native'].fallback, undefined);
});

test('a legacy route can opt into a complete policy with its exact model and zero automatic switches', async t => {
  const original = configuration();
  delete original['execution-policy'];
  const source = fixture(original), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  await change(ui, field(ui, '启用路线执行策略'), 'default');
  const policy = draft(ui)['execution-policy'], route = policy.routes.default;
  assert.deepEqual(policy.bindings[route.initial], complete(original.routes.default));
  assert.deepEqual(route.allowed, [route.initial]);
  assert.equal(route['max-switches'], 0);
  await ui.click('新建执行候选');
  assert.equal(Object.keys(draft(ui)['execution-policy'].bindings).length, 2);
  assert.deepEqual(draft(ui)['execution-policy'].routes.default, route);
  await ui.click('保存修改');
});

test('catalog selection is user-triggered, exact, bounded by verified efforts and separate from account entitlement', async t => {
  const source = fixture(configuration(), { models: request => modelReport(request, [
    { id: 'gemini-pro-orchestrator-neon', label: 'Orchestrator Neon', efforts: [''], defaultEffort: '' },
    { id: 'gemini-4-hydrogen-medium', label: 'Hydrogen', efforts: ['low', 'medium', 'high', 'xhigh'], defaultEffort: 'medium' },
  ]) });
  const ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话'); await clickLabel(ui, '编辑执行候选 shared');
  assert.equal(ui.requests.some(request => request.action === 'models'), false);
  await ui.click('读取模型目录');
  const request = ui.requests.find(request => request.action === 'models');
  assert.deepEqual(request.models, { version: 1, harness: 'antigravity', channel: 'google-subscription' });
  assert.match(ui.container.querySelector('[data-model-catalog]').textContent, /agy models.*native-client-catalog/);
  assert.match(ui.container.querySelector('[data-model-catalog]').textContent, /账号资格仍为未知/);
  await change(ui, field(ui, '从目录选择模型'), 'gemini-pro-orchestrator-neon');
  assert.equal(field(ui, '模型').value, 'gemini-pro-orchestrator-neon');
  assert.equal(draft(ui)['execution-policy'].bindings.shared.thinking, '');
  assert.equal(field(ui, '推理等级').options.length, 1);
  await change(ui, field(ui, '模型'), 'gemini-4-carbon-high');
  assert.equal(field(ui, '从目录选择模型').value, '', 'unlisted exact IDs stay manual rather than silently picking the first catalog model');
  assert.equal(draft(ui)['execution-policy'].bindings.shared.model, 'gemini-4-carbon-high');
});

test('unsupported and failed model catalog reads preserve the manual binding and never trigger automatic retries', async t => {
  let reads = 0;
  const source = fixture(configuration(), { models: request => {
    if (++reads === 1) throw new Error('native client unavailable');
    return modelReport(request, [], { status: 'unavailable', error: { code: 'missing-client', message: 'Install the native client' } });
  } });
  const ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话'); await clickLabel(ui, '编辑执行候选 local');
  await change(ui, field(ui, '模型'), 'gemini-4-carbon-high');
  const before = draft(ui);
  await ui.click('读取模型目录');
  assert.match(ui.container.querySelector('[data-model-catalog]').textContent, /手动模型和其它草稿均已保留/);
  assert.deepEqual(draft(ui), before);
  assert.equal(reads, 1);
  await ui.click('读取模型目录');
  assert.match(ui.container.querySelector('[data-model-catalog]').textContent, /Install the native client/);
  assert.deepEqual(draft(ui), before);
  assert.equal(reads, 2);
});

test('a model catalog completing after task navigation cannot populate the new task binding', async t => {
  const pending = deferred(); let requested;
  const source = fixture(configuration(), { models: request => { requested = request; return pending.promise; } });
  const ui = await mountPanel(t, source.respond);
  await ui.selectPath(0, '/repo/docs/a'); await ui.click('设置'); await ui.click('模型与会话'); await clickLabel(ui, '编辑执行候选 shared');
  await ui.click('读取模型目录');
  await ui.selectPath(0, '/repo/docs/b');
  await act(async () => pending.resolve(modelReport(requested, [{ id: 'old-task-model', label: 'Old task only' }])));
  await ui.click('模型与会话'); await clickLabel(ui, '编辑执行候选 shared');
  assert.doesNotMatch(ui.container.querySelector('[data-model-catalog]').textContent, /old-task-model|Old task only/);
  assert.equal(field(ui, '模型').value, agy.model);
  assert.equal(ui.requests.filter(request => request.action === 'models').length, 1);
});

test('a candidate provider change clears only obsolete native numeric errors and preserves explicit authority', async t => {
  const source = fixture(), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话'); await clickLabel(ui, '编辑执行候选 shared');
  await change(ui, field(ui, '观察到的子代理总数上限'), '9');
  assert.equal(ui.button('保存修改').disabled, true);
  await change(ui, field(ui, 'Provider'), 'claude-code-pro');
  assert.equal(ui.button('保存修改').disabled, false);
  assert.equal(draft(ui)['execution-policy'].bindings.shared.permission, 'danger-full-access');
  assert.equal(draft(ui)['execution-policy'].bindings.shared['session-mode'], 'scoped');
  assert.deepEqual(draft(ui)['native-profiles']['shared-native'].options, { version: 1, mode: 'native', maxObservedAgents: 4 });
});

test('catalog efforts do not silently widen the execution-policy adapter vocabulary', async t => {
  const source = fixture(configuration(), { models: request => modelReport(request, [
    { id: 'gpt-6.2-sol', label: 'Native model', efforts: ['none', 'minimal', 'low'], defaultEffort: 'none' },
  ], { source: 'codex app-server model/list' }) });
  const ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话'); await clickLabel(ui, '编辑执行候选 local');
  await change(ui, field(ui, 'Provider'), 'codex-pro');
  await ui.click('读取模型目录');
  await change(ui, field(ui, '从目录选择模型'), 'gpt-6.2-sol');
  const choices = [...field(ui, '推理等级').options].map(option => option.value);
  assert.equal(choices.includes('none'), false);
  assert.equal(choices.includes('minimal'), false);
  assert.equal(choices.includes('low'), true);
  assert.equal(draft(ui)['execution-policy'].bindings.local.thinking, 'high', 'an unsupported catalog default cannot silently replace the explicit effort');
  assert.match(ui.container.textContent, /额外列出的档位不会自动写入/);
});

test('route authorization refuses the binding limit without partially mutating permissions or shared references', async t => {
  const original = configuration();
  for (let index = 0; Object.keys(original['execution-policy'].bindings).length < 32; index += 1)
    original['execution-policy'].bindings['unused-' + index] = complete(agy);
  const source = fixture(original), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  await change(ui, field(ui, '普通执行 · 路线授权权限'), 'read-only');
  assert.deepEqual(draft(ui), original);
  assert.match(ui.container.textContent, /请先移除未使用候选/);
  assert.equal(ui.button('保存修改').disabled, true);
  await clickLabel(ui, '删除候选 unused-0');
  await change(ui, field(ui, '普通执行 · 路线授权权限'), 'read-only');
  assert.equal(Object.keys(draft(ui)['execution-policy'].bindings).length, 32);
  assert.equal(ui.button('保存修改').disabled, false);
});

test('fallback editing rejects an incompatible shared channel consumer instead of silently remapping its backend', async t => {
  const original = configuration();
  original['execution-policy'].bindings.otherBackend = complete({ harness: 'codex-pro', provider: '', channel: 'claude-native', model: 'gpt-6.2-sol', thinking: 'high' });
  const source = fixture(original), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('模型与会话');
  await change(ui, ui.container.querySelector('[data-policy-route="default"] [data-fallback-from="shared"] select'), 'claude');
  assert.match(ui.container.textContent, /现有其它规则或渠道后端与此映射冲突/);
  assert.equal(ui.button('保存修改').disabled, true);
  assert.deepEqual(draft(ui).channels, original.channels);
});

test('malformed execution policy drafts remain repairable and never crash structured settings', async t => {
  const original = configuration(), source = fixture(original), ui = await mountPanel(t, source.respond);
  await ui.click('设置'); await ui.click('诊断与配置文件');
  const malformed = structuredClone(original);
  malformed['execution-policy'].bindings.local = null;
  malformed['execution-policy'].routes.default.allowed = 1;
  malformed['execution-policy'].routes.default['channel-fallbacks'] = { shared: 'claude' };
  await act(async () => {
    const area = ui.container.querySelector('textarea[aria-label="配置字段（JSON）"]');
    Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLTextAreaElement.prototype, 'value').set.call(area, JSON.stringify(malformed));
    area.dispatchEvent(new ui.document.defaultView.Event('input', { bubbles: true }));
  });
  await ui.click('模型与会话');
  assert.equal(ui.button('保存修改').disabled, true);
  assert.deepEqual(draft(ui), malformed);
  await ui.click('撤销');
  assert.deepEqual(draft(ui), original);
});
