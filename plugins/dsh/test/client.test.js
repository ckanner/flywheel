import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import { JSDOM } from 'jsdom';

// Load ReactDOM after a browser document exists so its real event system is
// exercised. This checks the built rc.5 module, not a fake createElement shim.
const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost/' });
const globals = ['window', 'document', 'navigator', 'IS_REACT_ACT_ENVIRONMENT'];
const previousGlobals = globals.map(name => Object.getOwnPropertyDescriptor(globalThis, name));
for (const name of ['window', 'document', 'navigator']) {
  Object.defineProperty(globalThis, name, { configurable: true, writable: true, value: name === 'window' ? dom.window : dom.window[name] });
}
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
const React = await import('react');
const { createRoot } = await import('react-dom/client');
const { act } = React;
const source = await readFile(new URL('../dist/client.js', import.meta.url), 'utf8');
let client;
vm.runInNewContext(source, { window: { __ModuleLoader__: { load(definition) {
  assert.equal(definition.id, '@ckanner/dsh-plugin-flywheel');
  client = definition.factory(name => {
    assert.equal(name, 'react', 'the client must use only the host React module');
    return React;
  });
} } } });
after(() => {
  dom.window.close();
  globals.forEach((name, index) => {
    if (previousGlobals[index]) Object.defineProperty(globalThis, name, previousGlobals[index]);
    else delete globalThis[name];
  });
});

const configuration = () => ({ source: '/repo/flywheel.config.json', scope: 'project', bindings: [
  { id: 'routes.default', role: 'execute', route: 'default', profile: 'fast', harness: 'dsh',
    provider: 'deepseek-official', channel: 'deepseek-account', model: 'deepseek-flash', thinking: 'high',
    residentSession: false, permissionMode: 'workspace-write', inScope: true },
  { id: 'loops.plan', role: 'plan', profile: 'planner', harness: 'kimi', channel: 'kimi-subscription',
    model: 'kimi-code/k3.1-pro', thinking: 'high', residentSession: true, inScope: false },
] });
const envelope = (request, extra = {}) => ({ protocol: 1, ok: true, running: false, pid: 0,
  taskDir: request.taskDir ?? '', state: null, ...extra });
const commandReply = value => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify(value) } } });
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
};

async function mountPanel(t, respond) {
  let definition, Panel;
  const requests = [];
  client.apply({
    slots: {
      inject: (name, activate) => { assert.equal(name, 'conversation.composer.dock'); activate(); },
      register: (value, component) => { definition = value; Panel = component; },
    },
    remote: { commands: { execute: async (sessionId, command, attachments) => {
      assert.equal(sessionId, 'desktop-session');
      assert.equal(attachments.length, 0);
      assert.ok(command.startsWith('/flywheel '));
      const request = JSON.parse(command.slice('/flywheel '.length));
      requests.push(request);
      return respond(request);
    } } },
  });
  const container = dom.window.document.createElement('div');
  dom.window.document.body.appendChild(container);
  const root = createRoot(container);
  await act(async () => root.render(React.createElement(Panel, definition.inject('desktop-session'))));
  t.after(async () => {
    await act(async () => root.unmount());
    container.remove();
  });
  const button = name => {
    const found = [...container.querySelectorAll('button')].find(item => item.textContent === name);
    assert.ok(found, `button ${name} exists`);
    return found;
  };
  const click = async name => { await act(async () => button(name).click()); };
  const type = async (index, value) => {
    const input = container.querySelectorAll('input')[index];
    assert.ok(input, `input ${index} exists`);
    await act(async () => {
      Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, 'value').set.call(input, value);
      input.dispatchEvent(new dom.window.Event('input', { bubbles: true }));
    });
  };
  return { container, root, requests, button, click, type };
}

test('built panel is idle until a click and can inspect effective models without a task', async t => {
  const ui = await mountPanel(t, request => commandReply(envelope(request, { configuration: configuration() })));
  assert.equal(ui.requests.length, 0);
  await ui.click('Flywheel');
  assert.equal(ui.requests.length, 0, 'opening the panel must not start a probe or task');
  for (const name of ['刷新状态', '启动', '停止']) assert.equal(ui.button(name).disabled, true);
  for (const name of ['查看模型配置', '检查环境', 'SDK 无推理检查']) assert.equal(ui.button(name).disabled, false);
  await ui.click('查看模型配置');
  assert.deepEqual(ui.requests, [{ action: 'config' }]);
  const report = ui.container.querySelector('[aria-label="实际模型配置"]');
  assert.match(report.textContent, /\/repo\/flywheel\.config\.json/);
  for (const value of ['deepseek-flash', 'deepseek-official', 'deepseek-account', 'workspace-write', 'Profile：fast',
    'kimi-code/k3.1-pro', '会话恢复：启用', '未用于本次检查']) assert.ok(report.textContent.includes(value), value);
  assert.equal(ui.container.querySelector('details').open, false, 'raw JSON is folded by default');
  await act(async () => ui.container.querySelector('summary').click());
  assert.equal(ui.container.querySelector('details').open, true);
  await ui.type(1, '/other-repo');
  assert.equal(ui.container.querySelector('[aria-label="实际模型配置"]'), null);
  assert.equal(ui.container.querySelector('details'), null);
  await ui.click('查看模型配置');
  assert.deepEqual(ui.requests[1], { action: 'config', cwd: '/other-repo' });
});

test('task lifecycle labels do not confuse completed or pending work with a stopped engine', async t => {
  let stateStatus = 'pending', running = false, stateExists = true;
  const ui = await mountPanel(t, request => commandReply(envelope(request, { running, pid: running ? 42 : 0,
    state: stateExists ? { slug: 'example', status: stateStatus, packets: {} } : null })));
  await ui.click('Flywheel');
  await ui.type(0, '  /repo/docs/task  ');
  for (const [status, label] of [
    ['pending', '待开始'], ['completed', '已完成'], ['stopped', '已停止'], ['paused', '已暂停'],
    ['escalated', '需要人工处理'], ['budget-stopped', '预算已停止'], ['running', '引擎未运行，状态待核对'],
  ]) {
    stateStatus = status;
    await ui.click('刷新状态');
    assert.equal(ui.container.querySelector('button').textContent, `Flywheel · ${label}`);
  }
  running = true;
  await ui.click('启动');
  assert.equal(ui.container.querySelector('button').textContent, 'Flywheel · 运行中');
  assert.match(ui.container.querySelector('[aria-label="任务状态"]').textContent, /PID 42/);
  running = false;
  stateStatus = 'stopped';
  await ui.click('停止');
  assert.equal(ui.requests.at(-1).action, 'stop');
  stateExists = false;
  await ui.click('刷新状态');
  assert.equal(ui.container.querySelector('button').textContent, 'Flywheel · 尚未初始化');
  assert.equal(ui.requests[0].taskDir, '/repo/docs/task');
});

test('all packets remain accessible, text is escaped, and missing costs remain unknown', async t => {
  const slug = '<img src=x onerror=alert(1)>';
  const packets = Object.fromEntries(Array.from({ length: 24 }, (_, index) => [`u${index + 1}`, {
    status: index === 23 ? 'review-failed' : 'completed', route: 'default',
  }]));
  const ui = await mountPanel(t, request => commandReply(envelope(request, { state: {
    slug, status: 'pending', packets, 'current-milestone': 'm1', inbox: [{ reason: 'Review needs input' }],
    budget: { enforce: 'hard', 'daily-token-limit': 10000, 'unknown-cost-calls': 2, 'unknown-usage-calls': 1,
      totals: { tokens: 42, 'cost-usd': 0 }, warnings: ['Unknown usage is excluded from the known total.'] },
  } })));
  await ui.click('Flywheel');
  await ui.type(0, '/repo/docs/task');
  await ui.click('刷新状态');
  const state = ui.container.querySelector('[aria-label="任务状态"]');
  assert.equal(state.querySelectorAll('tbody tr').length, 24);
  assert.match(state.textContent, /u24评审退回default/);
  assert.ok(state.textContent.includes(slug));
  assert.equal(ui.container.querySelector('img'), null);
  assert.match(state.textContent, /成本：未知（2 次调用未报告结算成本）/);
  assert.match(state.textContent, /1 次调用用量未知，合计不完整/);
  assert.doesNotMatch(state.textContent, /\$0\.0000/);
  assert.match(state.textContent, /当前里程碑：m1/);
  assert.match(state.textContent, /待人工处理记录：1 条/);
});

test('local and SDK doctor buttons display structured failures, remedies, and unverified account facts', async t => {
  const ui = await mountPanel(t, request => commandReply(envelope(request, { ok: false, diagnostic: {
    kind: 'doctor', ok: false, cwd: '/repo', taskDir: '', probeRequested: request.probe ?? false,
    inferenceTested: false, configuration: configuration(), checks: [
      { id: 'config', status: 'pass', summary: 'Configuration loaded.' },
      { id: 'binary.kimi', status: 'fail', summary: 'Kimi CLI is missing.', code: 'INVALID_REQUEST', remedy: 'Configure an absolute executable path.' },
      { id: 'account', status: 'unknown', summary: 'Account authorization was not checked.' },
      { id: 'sandbox', status: 'warn', summary: 'SDK probe was not run.' },
      { id: 'route.unused', status: 'skipped', summary: 'This route is unused.' },
    ],
  } })));
  await ui.click('Flywheel');
  await ui.click('检查环境');
  assert.deepEqual(ui.requests[0], { action: 'doctor' });
  let report = ui.container.querySelector('[aria-label="环境检查结果"]');
  assert.match(report.textContent, /检查结束：存在阻断项/);
  assert.match(report.textContent, /未启动 SDK 探测/);
  for (const label of ['通过 · config', '失败 · binary.kimi', '未验证 · account', '注意 · sandbox', '跳过 · route.unused',
    '错误码：INVALID_REQUEST', '处理建议：Configure an absolute executable path.']) assert.ok(report.textContent.includes(label), label);
  assert.equal(ui.container.querySelector('[role="alert"]'), null, 'a logical failure remains a structured report');
  await ui.click('SDK 无推理检查');
  assert.deepEqual(ui.requests[1], { action: 'doctor', probe: true });
  report = ui.container.querySelector('[aria-label="环境检查结果"]');
  assert.match(report.textContent, /已请求 SDK 无推理检查/);
  assert.match(report.textContent, /未发送推理请求/);
  assert.match(report.textContent, /账号授权、额度和真实模型能力未通过本次检查验证/);
});

test('switching paths clears old results and late requests cannot overwrite the new selection or busy state', async t => {
  const oldRequest = deferred(), newRequest = deferred();
  const ui = await mountPanel(t, request => request.taskDir.endsWith('/old') ? oldRequest.promise : newRequest.promise);
  await ui.click('Flywheel');
  await ui.type(0, '/repo/docs/old');
  await ui.click('刷新状态');
  assert.ok(ui.container.querySelector('[role="status"]'));
  await ui.type(0, '/repo/docs/new');
  assert.equal(ui.container.querySelector('[role="status"]'), null);
  await ui.click('刷新状态');
  assert.equal(ui.button('刷新状态').disabled, true);
  await act(async () => oldRequest.resolve(commandReply(envelope({ taskDir: '/repo/docs/old' }, {
    state: { slug: 'OLD_RESULT', status: 'completed', packets: {} },
  }))));
  assert.doesNotMatch(ui.container.textContent, /OLD_RESULT/);
  assert.ok(ui.container.querySelector('[role="status"]'), 'an old finally must not clear the current busy state');
  await act(async () => newRequest.resolve(commandReply(envelope({ taskDir: '/repo/docs/new' }, {
    state: { slug: 'NEW_RESULT', status: 'pending', packets: {} },
  }))));
  assert.match(ui.container.querySelector('[aria-label="任务状态"]').textContent, /NEW_RESULT/);
  assert.equal(ui.container.querySelector('[role="status"]'), null);
  await ui.type(1, '/another-repo');
  assert.equal(ui.container.querySelector('[aria-label="任务状态"]'), null);
  assert.equal(ui.container.querySelector('details'), null);
  assert.equal(ui.container.querySelector('button').textContent, 'Flywheel');
});

test('late failures are ignored after a path change and active errors can recover', async t => {
  const delayed = deferred();
  let attempt = 0;
  const ui = await mountPanel(t, request => {
    attempt += 1;
    if (attempt === 1) return delayed.promise;
    if (attempt === 2) return { ok: false, error: { message: 'Desktop connection lost' } };
    if (attempt === 3) return commandReply({ protocol: 2 });
    return commandReply(envelope(request, { configuration: configuration() }));
  });
  await ui.click('Flywheel');
  await ui.click('查看模型配置');
  await ui.type(1, '/repo');
  await act(async () => delayed.reject(new Error('OLD_FAILURE')));
  assert.equal(ui.container.querySelector('[role="alert"]'), null);
  await ui.click('查看模型配置');
  assert.match(ui.container.querySelector('[role="alert"]').textContent, /Desktop connection lost/);
  assert.equal(ui.button('查看模型配置').disabled, false);
  await ui.click('查看模型配置');
  assert.match(ui.container.querySelector('[role="alert"]').textContent, /protocol mismatch/);
  await ui.click('查看模型配置');
  assert.equal(ui.container.querySelector('[role="alert"]'), null);
  assert.ok(ui.container.querySelector('[aria-label="实际模型配置"]'));
});
