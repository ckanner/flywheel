import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, mountPanel, closeClientHarness, deferred } from './support/client-harness.js';
import { planningState, planningTask } from './support/planning-fixture.js';

after(closeClientHarness);
const original = { taskDir: '/repo/docs/existing-task', cwd: '/repo' };
const oldState = { revision: 3, status: 'completed', slug: 'existing-task', packets: {} };
const oldAssociation = { ...original, action: 'planning', operationId: 'tool:existing', sequence: 1, outcome: 'confirmed' };
const reply = (request, fields = {}) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({ protocol: 1, ok: true, running: false, taskDir: request.taskDir ?? '', cwd: request.cwd ?? '/repo', state: null, ...fields }) } } });
const existing = request => reply(request, { state: oldState, taskTitle: 'Existing workflow' });
const created = request => reply(request, { state: planningTask(planningState({ brief: request.planning.brief })) });
const field = (ui, label) => [...ui.container.querySelectorAll('input,select,textarea')].find(node => node.getAttribute('aria-label') === label);
async function input(ui, label, value) {
  const node = field(ui, label);
  assert.ok(node, `field ${label}`);
  await act(async () => {
    const prototype = node.tagName === 'SELECT' ? ui.document.defaultView.HTMLSelectElement.prototype : ui.document.defaultView.HTMLTextAreaElement.prototype;
    Object.getOwnPropertyDescriptor(prototype, 'value').set.call(node, value);
    node.dispatchEvent(new ui.document.defaultView.Event(node.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true }));
  });
}
const mountExisting = (t, respond) => mountPanel(t, request => request.action === 'status' && request.taskDir === original.taskDir ? existing(request) : respond(request), { association: oldAssociation });
const selected = ui => ({ taskDir: ui.selection().taskDir, cwd: ui.selection().cwd });

test('an existing task exposes explicit intake without changing the selected task or starting work', async t => {
  const ui = await mountExisting(t, created);
  const reads = ui.requests.length;
  assert.equal(ui.container.querySelector('form[aria-label="创建另一个任务"]'), null);
  await ui.click('创建另一个任务');
  assert.deepEqual(selected(ui), original);
  assert.equal(ui.requests.length, reads, 'opening the form is local UI state');
  assert.match(ui.container.textContent, /Existing workflow/);
  assert.equal(field(ui, '方案授权').value, 'review-first');
  assert.equal(field(ui, '实施深度').value, 'auto');
  await input(ui, '目标与问题现象', 'A separate engineering task');
  await ui.click('收起并保留草稿');
  assert.deepEqual(selected(ui), original);
  assert.equal(ui.requests.length, reads);
  await ui.click('创建另一个任务');
  assert.equal(field(ui, '目标与问题现象').value, 'A separate engineering task');
});

test('a new task keeps the old canvas during dispatch and selects only the confirmed created task', async t => {
  const gate = deferred();
  const ui = await mountExisting(t, request => request.action === 'planning' ? gate.promise : reply(request));
  await ui.click('创建另一个任务');
  await input(ui, '目标与问题现象', 'Create a new reviewed plan');
  await ui.click('开始设计与审查');
  const request = ui.requests.at(-1);
  assert.equal(request.action, 'planning');
  assert.equal(request.planning.action, 'prepare');
  assert.equal(request.planning.start, true);
  assert.notEqual(request.taskDir, original.taskDir);
  assert.match(request.taskDir, /^\/repo\/docs\/task-dsh-/);
  assert.deepEqual(selected(ui), original);
  assert.match(ui.container.textContent, /Existing workflow/);
  assert.equal(ui.button('收起并保留草稿').disabled, true);
  assert.equal(ui.button('放弃新任务草稿').disabled, true);
  assert.equal(ui.button('创建另一个任务').disabled, true);
  await act(async () => gate.resolve(created(request)));
  assert.equal(ui.selection().taskDir, request.taskDir);
  assert.equal(ui.container.querySelector('form[aria-label="创建另一个任务"]'), null);
  assert.equal(ui.requests.filter(item => item.action === 'planning').length, 1);
  await ui.click('创建另一个任务');
  assert.equal(field(ui, '目标与问题现象').value, '');
  assert.equal(field(ui, '方案授权').value, 'review-first');
});

test('hiding retains one draft authorization, while discard and another conversation clear it', async t => {
  const ui = await mountExisting(t, created);
  await ui.click('创建另一个任务');
  await input(ui, '目标与问题现象', 'Only this draft is delegated');
  await input(ui, '方案授权', 'delegated');
  await act(async () => ui.container.querySelector('form input[type="checkbox"]').click());
  await ui.click('收起并保留草稿');
  await ui.setVisible(false); await ui.setVisible(true);
  await ui.click('创建另一个任务');
  assert.equal(field(ui, '目标与问题现象').value, 'Only this draft is delegated');
  assert.equal(field(ui, '方案授权').value, 'delegated');
  assert.equal(ui.container.querySelector('form input[type="checkbox"]').checked, true);
  await ui.switchSession('separate-conversation');
  assert.equal(field(ui, '目标与问题现象').value, '');
  assert.equal(field(ui, '方案授权').value, 'review-first');
  await ui.switchSession('desktop-session');
  assert.equal(field(ui, '目标与问题现象').value, 'Only this draft is delegated');
  await ui.click('放弃新任务草稿');
  await ui.click('创建另一个任务');
  assert.equal(field(ui, '目标与问题现象').value, '');
  assert.equal(field(ui, '方案授权').value, 'review-first');
  assert.equal(ui.requests.filter(item => item.action === 'planning').length, 0);
});

test('an old task pending or unknown mutation cannot be bypassed through new-task intake', async t => {
  const gate = deferred();
  const ui = await mountPanel(t, request => request.action === 'status'
    ? reply(request, { state: { ...oldState, status: 'paused' } }) : gate.promise, { association: oldAssociation });
  await ui.click('继续流程');
  assert.equal(ui.button('创建另一个任务').disabled, true);
  await act(async () => gate.resolve({ ok: false, error: { message: 'The old task start outcome is unknown' } }));
  assert.equal(ui.button('创建另一个任务').disabled, true);
  await ui.click('创建另一个任务');
  assert.equal(ui.container.querySelector('form'), null);
  assert.deepEqual(selected(ui), original);
  assert.equal(ui.requests.filter(item => item.action === 'planning').length, 0);
});

test('unknown new creation retains its target and draft across projection, remount and empty reconciliation', async t => {
  let attempts = 0, statusGate;
  const ui = await mountExisting(t, request => {
    if (request.action === 'status') return statusGate.promise;
    if (++attempts === 1) return { ok: false, error: { message: 'Creation response lost' } };
    return created(request);
  });
  await ui.click('创建另一个任务');
  await input(ui, '目标与问题现象', 'Retain this exact draft for an explicit retry');
  await ui.click('开始设计与审查');
  const first = ui.requests.at(-1);
  await ui.setAssociation({ taskDir: first.taskDir, cwd: first.cwd, action: 'planning', operationId: 'command:new-create', sequence: 2, outcome: 'unknown' });
  assert.deepEqual(selected(ui), original, 'unknown host association cannot select the unconfirmed task');
  await ui.setVisible(false); await ui.setVisible(true);
  assert.equal(field(ui, '目标与问题现象').value, first.planning.brief.goal);
  assert.equal(ui.button('放弃新任务草稿').disabled, true);
  assert.equal(ui.button('开始设计与审查').disabled, true);
  statusGate = deferred();
  await ui.click('刷新控制状态');
  assert.deepEqual(selected(ui), original, 'status dispatch is still provisional');
  const status = ui.requests.at(-1);
  assert.equal(status.action, 'status');
  assert.equal(status.taskDir, first.taskDir);
  await act(async () => statusGate.resolve(reply(status)));
  assert.deepEqual(selected(ui), original, 'an empty directory does not replace an existing task');
  assert.equal(attempts, 1, 'reconciliation never replays creation');
  assert.equal(ui.button('开始设计与审查').disabled, false);
  await ui.click('开始设计与审查');
  const repeated = ui.requests.at(-1);
  assert.equal(repeated.taskDir, first.taskDir);
  assert.deepEqual(repeated.planning.brief, first.planning.brief);
  assert.equal(ui.selection().taskDir, first.taskDir);
  assert.equal(attempts, 2);
});

test('same-operation canonical association waits for a live created-task receipt before selection', async t => {
  const gate = deferred(), canonical = { taskDir: '/physical/docs/new-task', cwd: '/physical' };
  const ui = await mountExisting(t, request => request.action === 'planning'
    ? { ok: false, error: { message: 'Lost creation result' } } : gate.promise);
  await ui.click('创建另一个任务');
  await input(ui, '目标与问题现象', 'Recover the new physical task');
  await ui.click('开始设计与审查');
  const first = ui.requests.at(-1);
  await ui.setAssociation({ ...canonical, requestedTaskDir: first.taskDir, requestedCwd: first.cwd,
    action: 'planning', operationId: 'command:canonical', sequence: 2, outcome: 'confirmed' });
  assert.deepEqual(selected(ui), original);
  const status = ui.requests.at(-1);
  assert.equal(status.action, 'status');
  assert.equal(status.taskDir, canonical.taskDir);
  await act(async () => gate.resolve(reply(status, { ...canonical, state: planningTask(planningState({ brief: first.planning.brief })) })));
  assert.deepEqual(selected(ui), canonical);
  assert.equal(ui.container.querySelector('form[aria-label="创建另一个任务"]'), null);
  assert.equal(ui.requests.filter(item => item.action === 'planning').length, 1);
});

test('new-task image previews preserve the current task and never send an intake flag to the host', async t => {
  const ui = await mountExisting(t, request => reply(request, { inputImages: {
    references: { version: 1, images: [{ path: 'layout.png', sha256: 'a'.repeat(64), 'media-type': 'image/png' }] },
    previews: [{ path: 'layout.png', sha256: 'a'.repeat(64), mimeType: 'image/png', bytes: 80, width: 2, height: 2, dataUrl: 'data:image/png;base64,aGVsbG8=' }],
  } }));
  await ui.click('创建另一个任务');
  await input(ui, '图片路径（仓库相对路径，每行一个，最多 4 张）', 'layout.png');
  await ui.click('检查并预览选定图片');
  const request = ui.requests.at(-1);
  assert.equal(request.action, 'images');
  assert.equal(request.taskDir, undefined);
  assert.equal(request.intake, undefined);
  assert.deepEqual(selected(ui), original);
  assert.match(ui.container.textContent, /Existing workflow/);
  assert.equal(ui.container.querySelectorAll('form img').length, 1);
  await ui.click('收起并保留草稿');
  await ui.click('创建另一个任务');
  assert.equal(ui.container.querySelectorAll('form img').length, 0, 'a reopened draft requires fresh image inspection');
});

test('a creation completed after changing conversations updates only its original retained binding', async t => {
  const gate = deferred();
  const ui = await mountExisting(t, () => gate.promise);
  await ui.click('创建另一个任务');
  await input(ui, '目标与问题现象', 'Complete in the original conversation');
  await ui.click('开始设计与审查');
  const request = ui.requests.at(-1);
  await ui.switchSession('other-conversation');
  assert.equal(ui.selection().taskDir, '');
  await act(async () => gate.resolve(created(request)));
  assert.equal(ui.selection().taskDir, '');
  assert.equal(field(ui, '目标与问题现象').value, '');
  await ui.switchSession('desktop-session');
  assert.equal(ui.selection().taskDir, request.taskDir);
  assert.equal(ui.container.querySelector('form[aria-label="创建另一个任务"]'), null);
});

test('an existing task can create a separate report-only investigation without implementation authority', async t => {
  const gate = deferred();
  const ui = await mountExisting(t, () => gate.promise);
  await ui.click('创建另一个任务');
  await input(ui, '这次要做什么', 'investigate');
  await input(ui, '目标与问题现象', 'Explain the missing timeout');
  await input(ui, '预期行为', 'Retain the configured timeout');
  await input(ui, '待验证的假设', 'An unset field overwrites the default');
  await input(ui, '什么证据能推翻假设', 'The loader retains the default');
  await input(ui, '允许读取的文件（相对会话工作目录，每行一个，最多 8 个）', 'src/config.go');
  await ui.click('创建并开始调查');
  const request = ui.requests.at(-1);
  assert.equal(request.action, 'investigate');
  assert.notEqual(request.taskDir, original.taskDir);
  assert.deepEqual(request.investigation.contract['implementation-scope'], []);
  assert.deepEqual(request.investigation.contract['read-paths'], ['src/config.go']);
  assert.equal(request.investigation.run, true);
  assert.deepEqual(selected(ui), original);
  await act(async () => gate.resolve(reply(request, { preparation: { version: 1, revision: 1, status: 'running', reason: 'running',
    contract: request.investigation.contract, observations: [], decisions: [] } })));
  assert.equal(ui.selection().taskDir, request.taskDir);
  assert.equal(ui.container.querySelector('form[aria-label="创建另一个任务"]'), null);
  assert.match(ui.container.textContent, /有界调查/);
});

test('confirmed host non-dispatch leaves the old task selected and permits explicit draft discard', async t => {
  const ui = await mountExisting(t, () => ({ ok: true, value: { result: { kind: 'error', text: JSON.stringify({ protocol: 1, type: 'flywheel-control-error',
    action: 'planning', dispatch: 'not-dispatched', message: 'Rejected before engine dispatch.' }) } } }));
  await ui.click('创建另一个任务');
  await input(ui, '目标与问题现象', 'The rejected draft is safe to discard');
  await input(ui, '方案授权', 'delegated');
  await act(async () => ui.container.querySelector('form input[type="checkbox"]').click());
  await ui.click('开始设计与审查');
  const request = ui.requests.at(-1), count = ui.requests.length;
  await ui.setAssociation({ taskDir: request.taskDir, cwd: request.cwd, action: 'planning', operationId: 'command:rejected', sequence: 2,
    outcome: 'confirmed', message: 'Rejected before engine dispatch.' });
  assert.deepEqual(selected(ui), original);
  assert.equal(ui.requests.length, count, 'a pre-dispatch host confirmation does not probe or select the absent task');
  assert.equal(ui.button('开始设计与审查').disabled, false);
  assert.equal(ui.button('放弃新任务草稿').disabled, false);
  await ui.click('放弃新任务草稿');
  await ui.click('创建另一个任务');
  assert.equal(field(ui, '目标与问题现象').value, '');
  assert.equal(field(ui, '方案授权').value, 'review-first');
});

test('delayed workspace discovery gates intake and later discoveries cannot clear or retarget an open draft', async t => {
  const gate = deferred(), selectedWorktree = { taskDir: '/worktree/docs/existing-task', cwd: '/worktree' };
  let catalogResult = gate.promise;
  const ui = await mountPanel(t, request => request.action === 'status' ? existing(request) : created(request), {
    association: { ...oldAssociation, ...selectedWorktree }, catalog: () => catalogResult,
  });
  assert.deepEqual(selected(ui), selectedWorktree);
  assert.equal(ui.button('创建另一个任务').disabled, true, 'the selected worktree is not the unconfirmed conversation workspace');
  assert.match(ui.container.textContent, /正在取得当前会话的项目目录/);
  await ui.click('创建另一个任务');
  assert.equal(field(ui, '目标与问题现象'), undefined);
  await act(async () => gate.resolve(reply({}, { cwd: '/repo', runs: [] })));
  assert.equal(ui.button('创建另一个任务').disabled, false);
  await ui.click('创建另一个任务');
  await input(ui, '目标与问题现象', 'Keep this draft in its confirmed workspace');
  await input(ui, '方案授权', 'delegated');
  await act(async () => ui.container.querySelector('form input[type="checkbox"]').click());
  assert.match(ui.container.querySelector('form').textContent, /\/repo\/docs/);
  catalogResult = reply({}, { cwd: '/moved-session', runs: [] });
  await ui.refreshDiscovery();
  await ui.setVisible(false); await ui.setVisible(true);
  assert.equal(field(ui, '目标与问题现象').value, 'Keep this draft in its confirmed workspace');
  assert.equal(field(ui, '方案授权').value, 'delegated');
  assert.equal(ui.container.querySelector('form input[type="checkbox"]').checked, true);
  assert.match(ui.container.querySelector('form').textContent, /\/repo\/docs/);
  assert.doesNotMatch(ui.container.querySelector('form').textContent, /\/moved-session\/docs/);
  assert.deepEqual(selected(ui), selectedWorktree);
  await ui.click('放弃新任务草稿');
  await ui.click('创建另一个任务');
  assert.equal(field(ui, '目标与问题现象').value, '');
  assert.equal(field(ui, '方案授权').value, 'review-first');
  assert.match(ui.container.querySelector('form').textContent, /\/moved-session\/docs/);
  await input(ui, '目标与问题现象', 'A new draft uses the newly confirmed workspace');
  await ui.click('开始设计与审查');
  assert.equal(ui.requests.at(-1).cwd, '/moved-session');
  catalogResult = reply({}, { cwd: '/third-session', runs: [] });
  await ui.refreshDiscovery();
  await ui.click('创建另一个任务');
  assert.equal(field(ui, '目标与问题现象').value, '');
  assert.equal(field(ui, '方案授权').value, 'review-first');
  assert.match(ui.container.querySelector('form').textContent, /\/third-session\/docs/, 'successful creation retires the previous workspace pin');
});

test('failed workspace discovery explains the blocked entry and its Retry recovers creation', async t => {
  const selectedWorktree = { taskDir: '/worktree/docs/existing-task', cwd: '/worktree' };
  let unavailable = true;
  const ui = await mountPanel(t, request => existing(request), {
    association: { ...oldAssociation, ...selectedWorktree },
    catalog: () => unavailable ? Promise.reject(new Error('Workspace discovery failed')) : reply({}, { cwd: '/repo', runs: [] }),
  });
  assert.equal(ui.button('创建另一个任务').disabled, true);
  assert.match(ui.container.textContent, /暂时无法确认当前会话的项目目录/);
  assert.match(ui.container.textContent, /Workspace discovery failed/);
  assert.equal(ui.button('重试').disabled, false);
  unavailable = false;
  await ui.click('重试');
  assert.equal(ui.button('创建另一个任务').disabled, false);
  assert.deepEqual(selected(ui), selectedWorktree);
  await ui.click('创建另一个任务');
  assert.match(ui.container.querySelector('form').textContent, /\/repo\/docs/);
  assert.equal(ui.requests.filter(request => request.action !== 'status').length, 0, 'discovery and opening the form never create a task');
});
