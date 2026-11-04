import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { chmod, mkdir, mkdtemp, readFile, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { runControl } from '../src/control.js';
import { apply as applyHost } from '../src/index.js';
import { act, closeClientHarness, deadlineClock, deferred, mountPanel } from './support/client-harness.js';

after(closeClientHarness);

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
const statusReply = (request, slug = 'example', status = 'pending') => commandReply(envelope(request, {
  state: { slug, status, packets: {} },
}));
const statusRegion = ui => ui.container.querySelector('[aria-label="任务状态"]');
const alerts = ui => [...ui.container.querySelectorAll('[role="alert"]')].map(node => node.textContent).join('\n');
const detail = (ui, name) => {
  const summary = [...ui.container.querySelectorAll('summary')].find(node => node.textContent === name);
  assert.ok(summary, `details ${name} exists`);
  return summary.parentElement;
};

function hostCommand(t, config, cwd) {
  let command, dispose;
  applyHost({
    on: (event, callback) => { assert.equal(event, 'dispose'); dispose = callback; },
    commands: { register: definition => { command = definition; } },
    tools: { register: () => {} },
  }, config);
  t.after(() => dispose());
  const transport = { results: [], completion: null, execute(request) {
    transport.completion = command.handler({ rawInput: JSON.stringify(request),
      agent: { session: { header: { cwd } } }, signal: new AbortController().signal,
    }).then(result => {
      transport.results.push(result);
      // rc.5 normalizes a CommandResult to its supported kind/text fields.
      return { ok: true, value: { result: { kind: result.kind, text: result.text } } };
    });
    return transport.completion;
  } };
  return transport;
}

test('native Flywheel view is additive, idle on opening, and ships its scoped stylesheet', async t => {
  const ui = await mountPanel(t, request => statusReply(request));
  assert.equal(ui.definition.name, 'conversation.view');
  assert.equal(ui.definition.id, 'flywheel');
  assert.equal(ui.definition.label(), 'Flywheel');
  assert.equal(ui.requests.length, 0, 'opening a view does not start a probe or task');
  assert.match(statusRegion(ui).textContent, /还没有任务状态/);
  assert.equal(detail(ui, '模型配置与环境检查').open, false);
  for (const name of ['刷新状态', '启动', '停止']) assert.equal(ui.button(name).disabled, true);
  for (const name of ['查看模型配置', '检查环境', 'SDK 无推理检查']) assert.equal(ui.button(name).disabled, false);
  const styles = [...ui.document.querySelectorAll('style[data-plugin="@ckanner/dsh-plugin-flywheel"]')];
  assert.equal(styles.length, 1, 'the view renders exactly one plugin-owned stylesheet');
  assert.ok(ui.container.contains(styles[0]), 'React owns the stylesheet inside the slot component');
  assert.equal(ui.document.head.querySelector('[data-plugin="@ckanner/dsh-plugin-flywheel"]'), null);
  assert.match(styles[0].textContent, /--dsw-alias-/);
  for (const className of ui.container.firstElementChild.classList) {
    assert.ok(styles[0].textContent.includes(`.${className}`), 'compiled classes resolve in the shipped stylesheet');
  }
  await ui.setVisible(false);
  assert.equal(ui.document.querySelector('style[data-plugin="@ckanner/dsh-plugin-flywheel"]'), null,
    'unmounting the view releases its style rather than leaving a bootstrap DOM write');
  await ui.setVisible(true);
  assert.equal(ui.container.querySelectorAll('style[data-plugin="@ckanner/dsh-plugin-flywheel"]').length, 1);
  assert.equal(ui.requests.length, 0, 'tab reopening does not dispatch a command');
});

test('effective models can be inspected without a task and every binding remains readable', async t => {
  const ui = await mountPanel(t, request => commandReply(envelope(request, { configuration: configuration() })));
  await ui.click('查看模型配置');
  assert.deepEqual(ui.requests, [{ action: 'config' }]);
  const report = ui.container.querySelector('[aria-label="实际模型配置"]');
  assert.match(report.textContent, /\/repo\/flywheel\.config\.json/);
  for (const value of ['deepseek-flash', 'deepseek-official', 'deepseek-account', 'workspace-write',
    'Profile', 'fast', 'kimi-code/k3.1-pro', '会话恢复', '启用', '未用于本次检查',
    '由原生运行时管理', '规划', '执行', 'high']) assert.ok(report.textContent.includes(value), value);
  const raw = detail(ui, '引擎原始结果（JSON）');
  assert.equal(raw.open, false, 'raw JSON is folded by default');
  await act(async () => raw.querySelector('summary').click());
  assert.equal(raw.open, true);
  await ui.type(1, '/other-repo');
  assert.equal(ui.container.querySelector('[aria-label="实际模型配置"]'), null);
  assert.equal([...ui.container.querySelectorAll('summary')].some(node => node.textContent === '引擎原始结果（JSON）'), false);
  await ui.click('查看模型配置');
  assert.deepEqual(ui.requests[1], { action: 'config', cwd: '/other-repo' });
});

test('task lifecycle labels distinguish pending, completed, stopped and unconfirmed engine state', async t => {
  let stateStatus = 'pending', running = false, stateExists = true;
  const ui = await mountPanel(t, request => commandReply(envelope(request, { running, pid: running ? 42 : 0,
    state: stateExists ? { slug: 'example', status: stateStatus, packets: {} } : null })));
  await ui.type(0, '  /repo/docs/task  ');
  for (const [status, label] of [
    ['pending', '待开始'], ['completed', '已完成'], ['stopped', '已停止'], ['paused', '已暂停'],
    ['escalated', '需要人工处理'], ['budget-stopped', '预算已停止'], ['running', '引擎未运行，状态待核对'],
    ['future-status', '未知状态（future-status）'],
  ]) {
    stateStatus = status;
    await ui.click('刷新状态');
    assert.ok(statusRegion(ui).textContent.includes(label), label);
  }
  running = true;
  await ui.click('启动');
  assert.match(statusRegion(ui).textContent, /运行中/);
  assert.match(statusRegion(ui).textContent, /PID 42/);
  running = false;
  stateStatus = 'stopped';
  await ui.click('停止');
  assert.equal(ui.requests.at(-1).action, 'stop');
  stateExists = false;
  await ui.click('刷新状态');
  assert.match(statusRegion(ui).textContent, /尚未初始化/);
  assert.equal(ui.requests[0].taskDir, '/repo/docs/task');
});

test('all packets and attention records remain accessible, text is escaped, and unknown cost stays unknown', async t => {
  const slug = '<img src=x onerror=alert(1)>';
  const packets = Object.fromEntries(Array.from({ length: 24 }, (_, index) => [`u${index + 1}`, {
    status: index === 23 ? 'review-failed' : 'completed', route: 'default',
  }]));
  let cost = 0, unknownCost = 2, unknownUsage = 1;
  const ui = await mountPanel(t, request => commandReply(envelope(request, { state: {
    slug, status: 'pending', packets, 'current-milestone': 'm1', inbox: [{ reason: 'Review needs input' }],
    budget: { enforce: 'hard', 'daily-token-limit': 10000, 'unknown-cost-calls': unknownCost,
      'unknown-usage-calls': unknownUsage, totals: { tokens: 42, 'cost-usd': cost },
      warnings: ['Unknown usage is excluded from the known total.'] },
  } })));
  await ui.type(0, '/repo/docs/task');
  await ui.click('刷新状态');
  const state = statusRegion(ui);
  assert.equal(state.querySelectorAll('tbody tr').length, 24);
  assert.match(state.textContent, /u24评审退回default/);
  assert.ok(state.textContent.includes(slug));
  assert.equal(ui.container.querySelector('img'), null);
  assert.match(state.textContent, /成本：未知（2 次调用未报告结算成本）/);
  assert.match(state.textContent, /1 次调用用量未知，合计不完整/);
  assert.doesNotMatch(state.textContent, /\$0\.0000/);
  assert.match(state.textContent, /当前里程碑：m1/);
  assert.match(state.textContent, /每日 Token 限额：10000/);
  assert.equal(detail(ui, '待人工处理记录：1 条').open, false);
  await act(async () => detail(ui, '待人工处理记录：1 条').querySelector('summary').click());
  assert.match(detail(ui, '待人工处理记录：1 条').textContent, /Review needs input/);
  cost = 0.25;
  await ui.click('刷新状态');
  assert.match(state.textContent, /已知成本合计：\$0\.2500；另有 2 次调用成本未知/);
  unknownCost = 0;
  unknownUsage = 0;
  await ui.click('刷新状态');
  assert.match(state.textContent, /已报告成本合计：\$0\.2500/);
  cost = undefined;
  await ui.click('刷新状态');
  assert.match(state.textContent, /成本：未知/);
});

test('local and SDK checks retain structured failures, remedies and the inference boundary', async t => {
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
  await ui.click('检查环境');
  assert.deepEqual(ui.requests[0], { action: 'doctor' });
  let report = ui.container.querySelector('[aria-label="环境检查结果"]');
  assert.match(report.textContent, /检查结束：存在阻断项/);
  assert.match(report.textContent, /未启动 SDK 探测/);
  for (const label of ['通过', 'config', '失败', 'binary.kimi', '未验证', 'account', '注意', 'sandbox', '跳过', 'route.unused',
    '错误码：INVALID_REQUEST', '处理建议：Configure an absolute executable path.']) assert.ok(report.textContent.includes(label), label);
  assert.equal(ui.container.querySelector('[role="alert"]'), null, 'a usable diagnostic failure is not a transport failure');
  await ui.click('SDK 无推理检查');
  assert.deepEqual(ui.requests[1], { action: 'doctor', probe: true });
  report = ui.container.querySelector('[aria-label="环境检查结果"]');
  assert.match(report.textContent, /已请求 SDK 无推理检查/);
  assert.match(report.textContent, /未发送推理请求/);
  assert.match(report.textContent, /账号授权、额度和真实模型能力未通过本次检查验证/);
});

test('editing a read target invalidates its late result without permitting overlapping operations', async t => {
  const oldRequest = deferred(), newRequest = deferred();
  const ui = await mountPanel(t, request => request.taskDir.endsWith('/old') ? oldRequest.promise : newRequest.promise);
  await ui.type(0, '/repo/docs/old');
  await ui.click('刷新状态');
  assert.ok(ui.container.querySelector('[role="status"]'));
  await ui.type(0, '/repo/docs/new');
  assert.match(ui.container.querySelector('[role="status"]').textContent, /上一个目录/);
  assert.equal(ui.button('刷新状态').disabled, true, 'the previous operation still owns the transport wait');
  await ui.click('刷新状态');
  assert.equal(ui.requests.length, 1);
  await act(async () => oldRequest.resolve(statusReply({ taskDir: '/repo/docs/old' }, 'OLD_RESULT', 'completed')));
  assert.doesNotMatch(ui.container.textContent, /OLD_RESULT/);
  assert.equal(ui.container.querySelector('[role="status"]'), null);
  await ui.click('刷新状态');
  assert.equal(ui.requests.length, 2);
  await act(async () => newRequest.resolve(statusReply({ taskDir: '/repo/docs/new' }, 'NEW_RESULT')));
  assert.match(statusRegion(ui).textContent, /NEW_RESULT/);
  await ui.type(1, '/another-repo');
  assert.doesNotMatch(statusRegion(ui).textContent, /NEW_RESULT/);
  assert.match(statusRegion(ui).textContent, /还没有任务状态/);
});

test('late query failures are ignored after editing, and active protocol or transport errors can recover', async t => {
  const delayed = deferred();
  let attempt = 0;
  const ui = await mountPanel(t, request => {
    attempt += 1;
    if (attempt === 1) return delayed.promise;
    if (attempt === 2) return { ok: false, error: { message: 'Desktop connection lost' } };
    if (attempt === 3) return commandReply({ protocol: 2 });
    return commandReply(envelope(request, { configuration: configuration() }));
  });
  await ui.click('查看模型配置');
  await ui.type(1, '/repo');
  await act(async () => delayed.reject(new Error('OLD_FAILURE')));
  assert.equal(alerts(ui), '');
  await ui.click('查看模型配置');
  assert.match(alerts(ui), /Desktop connection lost/);
  assert.match(alerts(ui), /检查连接和目录/);
  assert.equal(ui.button('查看模型配置').disabled, false);
  await ui.click('查看模型配置');
  assert.match(alerts(ui), /protocol.*mismatch/);
  await ui.click('查看模型配置');
  assert.equal(alerts(ui), '');
  assert.ok(ui.container.querySelector('[aria-label="实际模型配置"]'));
});

test('failed status refresh retains the previous data with a visible unconfirmed marker', async t => {
  let fail = false;
  const ui = await mountPanel(t, request => fail ? Promise.reject(new Error('Connection unavailable')) : statusReply(request, 'KNOWN_RESULT'));
  await ui.type(0, '/repo/docs/task');
  await ui.click('刷新状态');
  assert.match(statusRegion(ui).textContent, /KNOWN_RESULT/);
  fail = true;
  await ui.click('刷新状态');
  assert.match(statusRegion(ui).textContent, /KNOWN_RESULT/);
  assert.match(statusRegion(ui).textContent, /本次状态未确认，下方保留上次结果/);
  assert.match(statusRegion(ui).textContent, /状态待确认/);
  assert.match(statusRegion(ui).textContent, /上次确认状态：待开始/);
  assert.match(alerts(ui), /Connection unavailable/);
  fail = false;
  await ui.click('刷新状态');
  assert.doesNotMatch(statusRegion(ui).textContent, /本次状态未确认/);
  assert.equal(alerts(ui), '');
});

test('start is single-flight with a frozen target, including hide and reopen before confirmation', async t => {
  const pending = deferred();
  const ui = await mountPanel(t, () => pending.promise);
  await ui.type(0, '/repo/docs/original');
  await ui.type(1, '/repo/task-worktree');
  const start = ui.button('启动');
  await act(async () => { start.click(); start.click(); });
  assert.deepEqual(ui.requests, [{ action: 'start', taskDir: '/repo/docs/original', cwd: '/repo/task-worktree' }]);
  for (const input of ui.container.querySelectorAll('input')) assert.equal(input.disabled, true);
  for (const name of ['启动', '停止', '刷新状态']) assert.equal(ui.button(name).disabled, true);
  await ui.setVisible(false);
  await ui.setVisible(true);
  assert.equal(ui.container.querySelector('input').value, '/repo/docs/original');
  assert.equal(ui.button('启动').disabled, true);
  await ui.click('停止');
  assert.equal(ui.requests.length, 1, 'UI reopening cannot dispatch another mutation');
  await act(async () => pending.resolve(commandReply(envelope(ui.requests[0], {
    running: true, pid: 42, state: { slug: 'CONFIRMED_START', status: 'running', packets: {} },
  }))));
  assert.match(statusRegion(ui).textContent, /CONFIRMED_START/);
  assert.match(statusRegion(ui).textContent, /运行中/);
  for (const input of ui.container.querySelectorAll('input')) assert.equal(input.disabled, false);
  assert.equal(ui.requests.filter(request => request.action === 'stop').length, 0);
});

test('an unknown mutation stays blocked across path changes until the same task is successfully reconciled', async t => {
  let originalChecks = 0;
  const configurationWait = deferred();
  const ui = await mountPanel(t, request => {
    if (request.action === 'start') throw new Error('Reply was lost after dispatch');
    if (request.action === 'config') return configurationWait.promise;
    if (request.taskDir === '/repo/docs/original' && ++originalChecks === 1) throw new Error('Still disconnected');
    return statusReply(request, request.taskDir);
  });
  await ui.type(0, '/repo/docs/original');
  await ui.type(1, '/repo/original-worktree');
  await ui.click('启动');
  assert.match(alerts(ui), /结果未确认/);
  assert.match(alerts(ui), /请求可能已经送达引擎/);
  for (const name of ['启动', '停止']) assert.equal(ui.button(name).disabled, true);
  await ui.type(0, '/repo/docs/other');
  await ui.type(1, '/repo/other-worktree');
  await ui.click('刷新状态');
  assert.match(statusRegion(ui).textContent, /\/repo\/docs\/other/);
  assert.match(alerts(ui), /\/repo\/docs\/original/);
  assert.equal(ui.button('启动').disabled, true, 'another task status cannot confirm the original mutation');
  await ui.click('查看模型配置');
  assert.match(alerts(ui), /“启动”结果未确认/, 'a pending read cannot hide the unresolved mutation');
  assert.match(alerts(ui), /\/repo\/docs\/original/);
  assert.equal(ui.button('刷新待确认任务').disabled, true, 'reconciliation waits for the current read instead of overlapping it');
  await act(async () => configurationWait.resolve(commandReply(envelope(ui.requests.at(-1), { configuration: configuration() }))));
  assert.ok(ui.container.querySelector('[aria-label="实际模型配置"]'));
  assert.match(alerts(ui), /\/repo\/docs\/original/);
  assert.equal(ui.button('启动').disabled, true, 'a successful configuration read cannot confirm a mutation');
  await ui.setVisible(false);
  await ui.setVisible(true);
  assert.equal(ui.button('启动').disabled, true);
  await ui.click('刷新待确认任务');
  assert.deepEqual(ui.requests.at(-1), { action: 'status', taskDir: '/repo/docs/original', cwd: '/repo/original-worktree' });
  assert.equal(ui.container.querySelectorAll('input')[0].value, '/repo/docs/original');
  assert.equal(ui.container.querySelectorAll('input')[1].value, '/repo/original-worktree');
  assert.match(alerts(ui), /结果未确认/);
  assert.equal(ui.button('启动').disabled, true, 'a failed reconciliation cannot unlock mutations');
  await ui.click('刷新待确认任务');
  assert.equal(alerts(ui), '');
  assert.equal(ui.button('启动').disabled, false);
  assert.equal(ui.requests.filter(request => request.action === 'start').length, 1);
  assert.equal(ui.requests.filter(request => request.action === 'stop').length, 0);
});

test('proven host preflight rejections can be corrected without an impossible status reconciliation', async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-ui-preflight-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const binary = fileURLToPath(new URL('./fixtures/fake-control.js', import.meta.url));
  await chmod(binary, 0o755);
  const missingBinary = join(cwd, 'engine-not-installed');
  const cases = [
    { name: 'nonexistent task', task: join(cwd, 'missing-task'), cwd, binary,
      repair: ui => ui.type(0, cwd) },
    { name: 'nonexistent explicit cwd', task: cwd, cwd, explicitCwd: join(cwd, 'missing-repo'), binary,
      repair: ui => ui.type(1, cwd) },
    { name: 'missing session cwd', task: cwd, binary,
      repair: ui => ui.type(1, cwd) },
    { name: 'missing engine executable', task: cwd, cwd, binary: missingBinary,
      repair: () => symlink(binary, missingBinary) },
  ];
  for (const fixture of cases) {
    const transport = hostCommand(t, { flywheelBinary: fixture.binary }, fixture.cwd);
    const ui = await mountPanel(t, transport.execute);
    await ui.type(0, fixture.task);
    if (fixture.explicitCwd) await ui.type(1, fixture.explicitCwd);
    await ui.click('启动');
    await act(async () => { await transport.completion; });
    const rejected = transport.results.at(-1);
    assert.equal(rejected.kind, 'error', fixture.name);
    assert.equal(JSON.parse(rejected.text).dispatch, 'not-dispatched', fixture.name);
    assert.match(alerts(ui), /未发送到引擎/, fixture.name);
    assert.doesNotMatch(alerts(ui), /结果未确认/, fixture.name);
    assert.equal(ui.button('启动').disabled, false, fixture.name);
    assert.equal(ui.requests.length, 1, 'a proven rejection never causes an automatic retry');
    await fixture.repair(ui);
    await ui.click('启动');
    await act(async () => { await transport.completion; });
    assert.equal(transport.results.at(-1).kind, 'success', fixture.name);
    assert.equal(alerts(ui), '', fixture.name);
    assert.match(statusRegion(ui).textContent, /运行中/, fixture.name);
    assert.deepEqual(ui.requests.map(request => request.action), ['start', 'start'], fixture.name);
    await ui.unload();
  }
});

test('post-dispatch failures and forged engine messages remain uncertain until the original task is observed', async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-ui-dispatch-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  for (const mode of ['invalid-json', 'nonzero-exit', 'forged-error', 'timeout']) {
    const binary = join(cwd, `engine-${mode}.cjs`);
    const log = join(cwd, `${mode}.jsonl`);
    const forged = { protocol: 1, type: 'flywheel-control-error', action: 'start',
      dispatch: 'not-dispatched', message: 'This message came from a process that already ran.' };
    await writeFile(binary, `#!/usr/bin/env node
const fs = require('node:fs');
const args = process.argv.slice(2);
const action = args[1];
const taskDir = args[args.indexOf('--task-dir') + 1];
fs.appendFileSync(${JSON.stringify(log)}, JSON.stringify({ action }) + '\\n');
const value = { protocol: 1, ok: true, running: false, taskDir, state: { slug: 'OBSERVED_TASK', status: 'stopped', packets: {} } };
if (action === 'status') process.stdout.write(JSON.stringify(value));
else if (${JSON.stringify(mode)} === 'invalid-json') process.stdout.write('invalid JSON');
else if (${JSON.stringify(mode)} === 'timeout') setInterval(() => {}, 1000);
else {
  value.ok = false;
  value.error = ${JSON.stringify(mode === 'forged-error' ? JSON.stringify(forged) : 'The launched engine rejected the operation.')};
  process.stdout.write(JSON.stringify(value));
  process.exitCode = 1;
}
`, { mode: 0o755 });
    const transport = hostCommand(t, { flywheelBinary: binary, controlTimeoutMs: 1000 }, cwd);
    const ui = await mountPanel(t, transport.execute);
    await ui.type(0, cwd);
    await ui.click('启动');
    await act(async () => { await transport.completion; });
    const rejected = transport.results.at(-1);
    assert.equal(rejected.kind, 'error', mode);
    const evidence = JSON.parse(rejected.text);
    assert.equal(evidence.dispatch, 'unknown', mode);
    if (mode === 'forged-error') assert.equal(JSON.parse(evidence.message).dispatch, 'not-dispatched');
    assert.match(alerts(ui), /结果未确认/, mode);
    assert.equal(ui.button('启动').disabled, true, mode);
    await ui.click('启动');
    assert.equal(ui.requests.length, 1, 'an uncertain operation cannot be repeated');
    await ui.click('刷新待确认任务');
    await act(async () => { await transport.completion; });
    assert.equal(alerts(ui), '', mode);
    assert.match(statusRegion(ui).textContent, /OBSERVED_TASK/, mode);
    assert.equal(ui.button('启动').disabled, false, mode);
    assert.deepEqual((await readFile(log, 'utf8')).trim().split('\n').map(line => JSON.parse(line).action), ['start', 'status'], mode);
    await ui.unload();
  }
});

test('a browser deadline reports unknown outcome and a late original success cannot silently clear it', async t => {
  const pending = deferred();
  const clock = deadlineClock();
  const ui = await mountPanel(t, request => request.action === 'stop' ? pending.promise : statusReply(request, 'RECONCILED'), { timers: clock });
  await ui.type(0, '/repo/docs/task');
  await ui.click('停止');
  assert.equal(clock.pending.size, 1);
  assert.ok([...clock.pending.values()][0].delay > 140000, 'UI wait allows the maximum host timeout and cleanup');
  await clock.expire();
  assert.match(alerts(ui), /“停止”结果未确认/);
  assert.equal(ui.button('停止').disabled, true);
  await act(async () => pending.resolve(statusReply(ui.requests[0], 'LATE_STOP', 'stopped')));
  assert.doesNotMatch(statusRegion(ui).textContent, /LATE_STOP/);
  assert.match(alerts(ui), /结果未确认/);
  await ui.click('刷新待确认任务');
  assert.match(statusRegion(ui).textContent, /RECONCILED/);
  assert.equal(alerts(ui), '');
  assert.deepEqual(ui.requests.map(request => request.action), ['stop', 'status']);
  assert.equal(clock.pending.size, 0);
});

test('session drafts and results stay separate, and a replacement binding cannot adopt an old reply', async t => {
  const pending = deferred();
  const ui = await mountPanel(t, (request, sessionId) => sessionId === 'desktop-session' ? pending.promise : statusReply(request, 'SESSION_B'));
  await ui.type(0, '/repo/docs/session-a');
  await ui.type(1, '/repo/a');
  await ui.click('刷新状态');
  await ui.switchSession('other-session');
  assert.equal(ui.container.querySelector('input').value, '');
  assert.equal(ui.requests.length, 1);
  await ui.type(0, '/repo/docs/session-b');
  await ui.click('刷新状态');
  assert.match(statusRegion(ui).textContent, /SESSION_B/);
  await ui.switchSession('desktop-session');
  assert.equal(ui.container.querySelector('input').value, '/repo/docs/session-a');
  assert.equal(ui.container.querySelectorAll('input')[1].value, '/repo/a');
  assert.equal(ui.button('刷新状态').disabled, true);
  await ui.switchSession('desktop-session', 1);
  assert.equal(ui.container.querySelector('input').value, '', 'a fresh binding has a fresh view store');
  await act(async () => pending.resolve(statusReply(ui.requests[0], 'OLD_BINDING_RESULT')));
  assert.doesNotMatch(statusRegion(ui).textContent, /OLD_BINDING_RESULT/);
  await ui.switchSession('other-session');
  assert.equal(ui.container.querySelector('input').value, '/repo/docs/session-b');
  assert.match(statusRegion(ui).textContent, /SESSION_B/);
  assert.deepEqual(ui.sessions, ['desktop-session', 'other-session']);
  assert.equal(ui.requests.some(request => ['start', 'stop'].includes(request.action)), false);
});

test('plugin disposal stops local publication without sending a task cancellation', async t => {
  const pending = deferred();
  const clock = deadlineClock();
  const ui = await mountPanel(t, () => pending.promise, { timers: clock });
  await ui.type(0, '/repo/docs/task');
  await ui.click('启动');
  await ui.unload();
  assert.equal(ui.container.childElementCount, 0);
  assert.equal(clock.pending.size, 0);
  await act(async () => pending.resolve(statusReply(ui.requests[0], 'LATE_AFTER_UNLOAD')));
  assert.equal(ui.container.childElementCount, 0);
  assert.deepEqual(ui.requests.map(request => request.action), ['start']);
});

test('malformed protocol results are visible and never become successful task observations', async t => {
  const failures = [
    () => ({ ok: true, value: { result: { kind: 'failure', text: 'Command unavailable' } } }),
    () => ({ ok: true, value: { result: { kind: 'success', text: '{bad json' } } }),
    request => commandReply(envelope(request, { protocol: 2 })),
    request => commandReply(envelope(request, { taskDir: 'relative/task' })),
    request => commandReply(envelope(request, { taskDir: '' })),
    request => commandReply(envelope(request, { taskDir: null })),
    request => commandReply(envelope(request, { running: 'yes' })),
    request => commandReply(envelope(request, { state: [] })),
    request => commandReply(envelope(request, { ok: false, error: 'Engine rejected status' })),
  ];
  let index = 0;
  const ui = await mountPanel(t, request => index < failures.length ? failures[index++](request) : statusReply(request, 'VALID_RESULT'));
  await ui.type(0, '/repo/docs/task');
  for (let i = 0; i < failures.length; i += 1) {
    await ui.click('刷新状态');
    assert.match(alerts(ui), /“刷新状态”未完成/);
    assert.match(statusRegion(ui).textContent, /还没有任务状态/);
    assert.equal(ui.button('刷新状态').disabled, false);
  }
  await ui.click('刷新状态');
  assert.equal(alerts(ui), '');
  assert.match(statusRegion(ui).textContent, /VALID_RESULT/);
});

test('malformed configuration and diagnostic reports stay recoverable instead of breaking rendering', async t => {
  const failures = [
    { action: 'config', name: '查看模型配置', value: { configuration: { bindings: {} } } },
    { action: 'config', name: '查看模型配置', value: { configuration: { bindings: [null] } } },
    { action: 'doctor', name: '检查环境', value: { diagnostic: { ok: true, checks: [null] } } },
    { action: 'doctor', name: '检查环境', value: { diagnostic: { ok: false, checks: [] } } },
    { action: 'doctor', name: '检查环境', value: { diagnostic: { ok: true, checks: [], configuration: [] } } },
  ];
  let index = 0;
  const ui = await mountPanel(t, request => {
    if (index < failures.length) {
      const failure = failures[index++];
      assert.equal(request.action, failure.action);
      return commandReply(envelope(request, failure.value));
    }
    return commandReply(envelope(request, { configuration: configuration(), diagnostic: {
      ok: true, checks: [{ id: 'account', status: 'unknown', summary: 'Authorization remains unverified.' }],
      inferenceTested: false, probeRequested: false,
    } }));
  });
  for (const failure of failures) {
    await ui.click(failure.name);
    assert.match(alerts(ui), /malformed/);
    assert.equal(ui.button(failure.name).disabled, false);
    assert.equal(ui.container.querySelector('[aria-label="实际模型配置"]'), null);
    assert.equal(ui.container.querySelector('[aria-label="环境检查结果"]'), null);
  }
  await ui.click('查看模型配置');
  assert.equal(alerts(ui), '');
  assert.ok(ui.container.querySelector('[aria-label="实际模型配置"]'));
  await ui.click('检查环境');
  assert.equal(alerts(ui), '');
  assert.match(ui.container.querySelector('[aria-label="环境检查结果"]').textContent, /Authorization remains unverified/);
});

test('the UI accepts realpath identities already validated by the host for every control action', async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-ui-canonical-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const task = join(cwd, 'physical-task');
  await mkdir(join(task, 'child'), { recursive: true });
  const alias = join(cwd, 'task-link');
  await symlink(task, alias, 'dir');
  const inputTask = `${alias}/child/../`;
  const canonicalTask = await realpath(inputTask);
  assert.notEqual(inputTask, canonicalTask, 'the fixture exercises symlinks, parent traversal and a trailing slash');
  const binary = fileURLToPath(new URL('./fixtures/fake-control.js', import.meta.url));
  await chmod(binary, 0o755);
  let completion;
  const observations = [];
  const ui = await mountPanel(t, request => {
    completion = runControl(request, { binary, defaultCwd: cwd }).then(value => {
      observations.push(value);
      return commandReply(value);
    });
    return completion;
  });
  await ui.type(0, inputTask);
  for (const [name, action] of [
    ['刷新状态', 'status'], ['启动', 'start'], ['停止', 'stop'], ['查看模型配置', 'config'],
    ['检查环境', 'doctor'], ['SDK 无推理检查', 'doctor'],
  ]) {
    await ui.click(name);
    await act(async () => { await completion; });
    assert.equal(ui.requests.at(-1).action, action);
    assert.equal(ui.requests.at(-1).taskDir, inputTask, 'the browser preserves the user selection');
    assert.equal(observations.at(-1).taskDir, canonicalTask, 'runControl validates the canonical engine identity');
    assert.equal(alerts(ui), '', `${action} must not become an unknown outcome after a successful host validation`);
    assert.equal(ui.button('启动').disabled, false);
    assert.equal(ui.button('停止').disabled, false);
  }
  assert.equal(ui.requests.length, 6, 'there is no retry or repair mutation');
  assert.ok(ui.container.querySelector('[aria-label="实际模型配置"]'));
  assert.ok(ui.container.querySelector('[aria-label="环境检查结果"]'));
});

test('labelled fields validate paths without a request, retain focus, and follow the host locale', async t => {
  const ui = await mountPanel(t, request => commandReply(envelope(request, { configuration: configuration() })));
  for (const input of ui.container.querySelectorAll('input')) {
    const label = [...ui.container.querySelectorAll('label')].find(node => node.htmlFor === input.id);
    assert.ok(label?.textContent, 'each input has a native associated label');
    assert.ok(ui.document.getElementById(input.getAttribute('aria-describedby')), 'field help remains associated');
  }
  await ui.type(0, 'relative/task');
  assert.equal(ui.container.querySelector('input').getAttribute('aria-invalid'), 'true');
  assert.match(ui.container.textContent, /请输入以 \/ 开头的绝对路径/);
  assert.equal(ui.button('启动').disabled, true);
  await ui.click('查看模型配置');
  assert.equal(ui.requests.length, 0);
  await ui.type(0, '/repo/docs/task');
  const input = ui.container.querySelector('input');
  input.focus();
  assert.equal(ui.document.activeElement, input);
  await ui.setLocale('en');
  assert.equal(ui.document.activeElement, input, 'locale refresh does not remount focused fields');
  assert.equal(input.value, '/repo/docs/task');
  assert.match(ui.container.textContent, /Task controls/);
  assert.equal(ui.button('Refresh status').disabled, false);
  assert.equal(ui.requests.length, 0, 'locale changes cannot trigger a command');
  await ui.click('Inspect model configuration');
  assert.ok(ui.container.querySelector('[aria-label="Effective model configuration"]'));
  assert.match(ui.container.textContent, /deepseek-flash/);
  await ui.setLocale('zh');
  assert.equal(input.value, '/repo/docs/task');
  assert.ok(ui.container.querySelector('[aria-label="实际模型配置"]'));
  for (const button of ui.container.querySelectorAll('button')) assert.equal(button.type, 'button');
});
