import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, realpath, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Context } from '@deepseek-ai/cordis';
import { SessionStore } from '@deepseek-ai/dsh-session';
import { SessionProjectionRegistry } from '@deepseek-ai/dsh-session-projection';
import { runControl, ControlNotDispatchedError, validateControl } from '../src/control.js';
import { apply, inject } from '../src/index.js';
import { validPlanningCommand, planningUserProjection as projection, planningUserAction } from '../src/planning-control.js';
import { act, mountPanel, closeClientHarness } from './support/client-harness.js';
import { designRef, previousRef, planningState, planningTask } from './support/planning-fixture.js';

after(closeClientHarness);
const binary = fileURLToPath(new URL('./fixtures/fake-planning-control.js', import.meta.url));
const approval = { action: 'approve', revision: 7, 'request-id': 'approval-1', design: designRef, start: false };
const request = (taskDir, planning = approval) => ({ action: 'planning', taskDir, cwd: taskDir, planning });
const reply = (request, fields) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({ protocol: 1, ok: true, running: false, taskDir: request.taskDir ?? '', state: planningTask(), ...fields }) } } });
function env(t, name, value) {
  const old = process.env[name]; process.env[name] = value;
  t.after(() => old === undefined ? delete process.env[name] : process.env[name] = old);
}
async function directory(t) {
  const dir = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-planning-')));
  t.after(() => rm(dir, { recursive: true, force: true })); return dir;
}

test('planning control validates exact typed commands and keeps provenance out of model JSON', async t => {
  const dir = await directory(t);
  for (const value of [approval, { action: 'prepare', brief: {} }, { action: 'plan', revision: 7, 'request-id': 'plan-1', scope: 'milestone', 'scope-id': 'M01' }, { action: 'feedback', revision: 7, 'request-id': 'feedback-1', text: 'Use a durable checkpoint', design: designRef }, { action: 'status' }, { action: 'migrate', revision: 7, 'request-id': 'migration-1', policy: 'review-first' }]) {
    assert.equal(validPlanningCommand(value), true); await validateControl(request(dir, value));
  }
  assert.equal(validPlanningCommand({ action: 'status', 'request-id': 'feedback-1' }), true);
  assert.equal(validPlanningCommand({ action: 'plan', revision: 7, 'request-id': 'plan-local', scope: 'milestone', 'scope-id': 'M-01', text: 'Expand restart recovery without changing the API.' }), true);
  for (const value of [{ ...approval, actor: 'user' }, { ...approval, source: 'dsh' }, { ...approval, revision: 0 }, { ...approval, 'request-id': '' }, { ...approval, design: { ...designRef, digest: 'latest' } }, { action: 'feedback', revision: 7, 'request-id': 'f1', text: '' }, { action: 'plan', revision: 7, 'request-id': 'p1', scope: 'execute' }, { action: 'status', start: true }, { ...approval, start: 'yes' }]) {
    assert.equal(validPlanningCommand(value), false); await assert.rejects(validateControl(request(dir, value)));
  }
  await assert.rejects(validateControl({ action: 'status', taskDir: dir, cwd: dir, planning: { action: 'status' } }));
});

test('planning approval proof comes from an exact live user command projection and excludes forks and consumed events', () => {
  const input = request('/repo'), raw = JSON.stringify(input), session = { id: 'desktop-session' };
  const event = { type: 'command/run', seq: 0, data: { name: 'flywheel', commandId: 'c1', args: raw, source: { kind: 'user' } } };
  const state = projection.apply(projection.init({}, 0), event);
  assert.equal(projection.stateSchema.safeParse(state).success, true);
  assert.deepEqual(planningUserAction(input, session, 'c1', raw, state), { kind: 'dsh', sessionId: session.id, commandId: 'c1', 'request-id': 'approval-1', revision: 7, design: designRef, start: false });
  for (const invalid of [projection.init({}, 0), projection.apply(projection.init({}, 1), event), projection.apply(state, event), projection.apply(state, { type: 'command/done', seq: 1, data: { commandId: 'c1' } }), projection.apply(projection.init({}, 0), { ...event, data: { ...event.data, source: { kind: 'model' } } })]) {
    assert.throws(() => planningUserAction(input, session, 'c1', raw, invalid), /unconsumed/);
  }
  assert.throws(() => planningUserAction(input, session, 'c1', raw + ' ', state));
  assert.throws(() => planningUserAction(input, undefined, 'c1', raw, state));
  assert.equal(planningUserAction(request('/repo', { action: 'status' }), session), undefined);
});

test('the process boundary strips inherited approval and forwards only host-verified exact proof', async t => {
  const dir = await directory(t), log = join(dir, 'calls.jsonl');
  env(t, 'TEST_PLANNING_LOG', log); env(t, 'FLYWHEEL_PLANNING_USER_ACTION', 'inherited-forgery');
  const options = { binary, hostSessionID: 'desktop-session' };
  await runControl(request(dir, { action: 'plan', revision: 7, 'request-id': 'plan-1', scope: 'design', start: true }), options);
  await assert.rejects(runControl(request(dir), options), ControlNotDispatchedError);
  const proof = { kind: 'dsh', sessionId: 'desktop-session', commandId: 'c1', 'request-id': 'approval-1', revision: 7, design: designRef, start: false };
  await assert.rejects(runControl(request(dir), { ...options, planningUserAction: { ...proof, start: true } }), ControlNotDispatchedError);
  await runControl(request(dir), { ...options, planningUserAction: proof });
  const calls = (await readFile(log, 'utf8')).trim().split('\n').map(JSON.parse);
  assert.equal(calls.length, 2);
  assert.equal(calls[0].proof, undefined); assert.equal(calls[0].command.scope, 'design');
  assert.deepEqual(JSON.parse(calls[1].proof), proof); assert.deepEqual(calls[1].command, approval);
  assert.deepEqual(calls[1].args, ['control', 'planning', '--cwd', dir, '--compact', '--task-dir', dir]);
});

test('published DSH host accepts a user command, associates planning, and rejects tool and fork approval', async t => {
  const dir = await directory(t), ctx = new Context(), tools = new Map(); let command;
  t.after(() => ctx.fiber.dispose());
  await ctx.plugin(SessionStore); await ctx.plugin(SessionProjectionRegistry);
  ctx.provide('commands', { register: value => { command = value; } });
  ctx.provide('tools', { register: value => tools.set(value.name, value) });
  ctx.provide('workingDirectory', { get: () => dir });
  await ctx.plugin({ apply, inject }, { flywheelBinary: binary });
  const session = ctx.sessions.create('approval-session', { meta: { cwd: dir } });
  const input = request(dir), raw = JSON.stringify(input);
  const absent = await command.handler({ rawInput: raw, commandId: 'c1', agent: { session } });
  assert.equal(JSON.parse(absent.text).dispatch, 'not-dispatched');
  session.append('command/run', { commandId: 'c1', name: 'flywheel', args: raw, source: { kind: 'user' } });
  const result = await command.handler({ rawInput: raw, commandId: 'c1', agent: { session } });
  assert.equal(result.kind, 'success');
  const fork = ctx.sessions.create('fork', { seed: session.snapshotEvents(), inheritedEventCount: session.seq, meta: { cwd: dir, isSeeded: true } });
  assert.equal((await command.handler({ rawInput: raw, commandId: 'c1', agent: { session: fork } })).kind, 'error');
  session.append('command/done', { commandId: 'c1', ...result });
  assert.equal(ctx.sessionProjections.snapshot(session).values.flywheelTask.taskDir, dir);
  assert.equal((await command.handler({ rawInput: raw, commandId: 'c1', agent: { session } })).kind, 'error');
  const definition = tools.get('flywheel_control');
  assert.equal(definition.parameters.properties.planning.properties.action.enum.includes('approve'), false);
  await assert.rejects(definition.execute(input, { callId: 't1', arguments: input, agent: { session } }), /model tools cannot approve/);
  const lost = request(dir, { action: 'feedback', revision: 7, 'request-id': 'lost-feedback', text: 'Discuss recovery.' });
  session.append('command/run', { commandId: 'lost', name: 'flywheel', args: JSON.stringify(lost), source: { kind: 'user' } });
  session.append('command/done', { commandId: 'lost', kind: 'error', text: JSON.stringify({ protocol: 1, type: 'flywheel-control-error', action: 'planning', dispatch: 'unknown', message: 'Reply lost.' }) });
  assert.equal(ctx.sessionProjections.snapshot(session).values.flywheelTask.outcome, 'unknown');
  const receiptInput = JSON.stringify(request(dir, { action: 'status', 'request-id': 'lost-feedback' }));
  session.append('command/run', { commandId: 'read-receipt', name: 'flywheel', args: receiptInput, source: { kind: 'user' } });
  assert.equal(ctx.sessionProjections.snapshot(session).values.flywheelTask.operationId, 'command:lost', 'a read cannot replace an uncertain mutation before its result');
  const receiptResult = await command.handler({ rawInput: receiptInput, commandId: 'read-receipt', agent: { session } });
  assert.equal(receiptResult.kind, 'success', 'a request receipt remains readable after a lost mutation reply');
  session.append('command/done', { commandId: 'read-receipt', ...receiptResult });
  assert.equal(ctx.sessionProjections.snapshot(session).values.flywheelTask.outcome, 'confirmed');
});

test('design view shows the reviewed exact version and keeps approval separate from implementation start', async t => {
  let p = planningState(), revision = 7;
  const ui = await mountPanel(t, request => {
    if (request.action === 'planning') {
      assert.equal(request.planning.action, 'approve'); assert.equal(request.planning.revision, revision);
      assert.deepEqual(request.planning.design, designRef);
      p = { ...p, phase: 'approved', approvals: [...p.approvals, { kind: 'human', design: designRef, 'review-id': 'review-2', source: 'dsh:user-command', 'request-id': request.planning['request-id'], start: request.planning.start }], 'start-authorized': request.planning.start };
      revision++;
    }
    return reply(request, { state: planningTask(p, revision) });
  });
  await ui.selectPath(0, '/repo/docs/planning'); await ui.click('刷新');
  const panel = ui.container.querySelector('[aria-label="方案与审查"]'); assert.ok(panel);
  assert.match(panel.textContent, /Persist the checkpoint before acknowledgement/); assert.match(panel.textContent, /上一版本/); assert.match(panel.textContent, /The previous acknowledgement preceded persistence/);
  assert.match(panel.textContent, /根方案可以先独立批准/); assert.equal(ui.requests.length, 1);
  assert.equal(ui.button('继续流程').disabled, true);
  await ui.click('批准此版本'); assert.equal(ui.requests.at(-1).planning.start, false);
  assert.equal(ui.button('批准此版本').disabled, true); assert.equal(ui.button('继续流程').disabled, true);
  await ui.click('开始已批准方案'); assert.equal(ui.requests.at(-1).planning.start, true);
  assert.equal(ui.button('继续流程').disabled, false);
  await ui.setLocale('en'); assert.match(panel.textContent, /Design and review/);
  await ui.setVisible(false); await ui.setVisible(true); assert.equal(ui.requests.length, 3, 'remount never approves or launches');
});

test('unreviewed, stale, changed-input and running designs cannot be approved', async t => {
  for (const variant of [{ reviews: [] }, { 'input-revision': 3 }, { 'pending-activation': { stage: 'files' } },
    { designs: planningState().designs.map(design => ({ ...design, document: '' })) },
    { projection: { kind: 'console-excerpt', 'approval-complete': false, ledger: 'state.json', omitted: { 'review-details': 1 } } }]) {
    const ui = await mountPanel(t, request => reply(request, { state: planningTask(planningState(variant)) }));
    await ui.selectPath(0, '/repo/docs/planning'); await ui.click('刷新');
    assert.equal(ui.button('批准此版本').disabled, true); assert.equal(ui.button('批准此版本并开始').disabled, true);
    if (variant.projection) {
      assert.match(ui.container.textContent, /审批需要完整方案与审查记录/);
      assert.match(ui.container.querySelector('[data-planning-excerpt]').textContent, /\/repo\/docs\/planning\/state\.json/);
      assert.equal(ui.requests.length, 1, 'an omitted review never triggers an approval or automatic detail mutation');
    }
  }
  const ui = await mountPanel(t, request => reply(request, { running: true }));
  await ui.selectPath(0, '/repo/docs/planning'); await ui.click('刷新'); assert.equal(ui.button('批准此版本').disabled, true);
});

test('historical implementation start cannot hide the exact new design start action or authorize stale input', async t => {
  let p = planningState({ phase: 'approved', 'start-authorized': true, approvals: [
    { kind: 'human', design: previousRef, 'review-id': 'review-1', start: true, source: 'dsh:old-start' },
    { kind: 'human', design: designRef, 'review-id': 'review-2', start: false, source: 'dsh:new-approve-only' },
  ] });
  const ui = await mountPanel(t, request => {
    if (request.planning?.action === 'approve') {
      assert.deepEqual(request.planning.design, designRef);
      assert.equal(request.planning.start, true);
      p = { ...p, approvals: [...p.approvals, { kind: 'human', design: designRef, 'review-id': 'review-2', start: true, source: 'dsh:new-start' }] };
    }
    return reply(request, { state: planningTask(p) });
  });
  await ui.selectPath(0, '/repo/docs/planning'); await ui.click('刷新');
  assert.equal(ui.button('继续流程').disabled, true, 'an earlier design start does not authorize this approved-only design');
  assert.equal(ui.button('开始已批准方案').disabled, false, 'the user can grant start for the exact new root');
  assert.match(ui.container.textContent, /方案已批准；尚未授权启动实现/);
  await ui.click('开始已批准方案');
  assert.equal(ui.button('继续流程').disabled, false);
  p = { ...p, 'input-revision': 3 };
  await ui.click('刷新');
  assert.equal(ui.button('继续流程').disabled, true, 'changed input invalidates the displayed root approval');
  assert.equal(ui.button('批准此版本并开始').disabled, true);
});

test('a blocked planning invocation is visible and can be retired only through the user command', async t => {
  const blocked = {
    role: 'design-plan', scope: 'planning:engine-1-1', action: 'wait',
    reason: 'dispatch, outcome or cleanup remains unknown', 'resume-candidate': false,
    last: { id: 'call-blocked1', status: 'blocked', dispatch: 'unknown', outcome: 'unknown', 'cleanup-confirmed': true, accounted: true, 'usage-status': 'unknown' },
  };
  const ui = await mountPanel(t, request => {
    if (request.action === 'session') {
      assert.deepEqual(request.session.action, 'abandon');
      assert.deepEqual(request.session.key, 'design-plan/planning:engine-1-1');
      assert.equal(request.session.invocation, 'call-blocked1');
      assert.equal(request.session.revision, 7);
      assert.equal(request.session.reason, 'Retire this exact call');
      return reply(request, {
        state: { ...planningTask(planningState({ phase: 'drafting' })), sessions: { version: 4, records: { 'design-plan/planning:engine-1-1': { ...blocked, last: { ...blocked.last, status: 'abandoned' } } } } },
        'session-control': { key: request.session.key, action: 'abandon', applied: true, revision: 7, status: 'abandoned', invocation: 'call-blocked1' },
      });
    }
    return reply(request, { state: { ...planningTask(planningState({ phase: 'drafting' })), sessions: { version: 4, records: { 'design-plan/planning:engine-1-1': blocked } } } });
  });
  await ui.selectPath(0, '/repo/docs/planning'); await ui.click('刷新');
  assert.match(ui.container.textContent, /未解开的规划调用/);
  assert.match(ui.container.textContent, /call-blocked1/);
  const reason = ui.container.querySelector('textarea[aria-label="退役原因"]');
  await act(async () => { Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLTextAreaElement.prototype, 'value').set.call(reason, 'Retire this exact call'); reason.dispatchEvent(new ui.document.defaultView.Event('input', { bubbles: true })); });
  const confirm = ui.container.querySelector('input[type="checkbox"]');
  await act(async () => { confirm.click(); });
  await ui.click('退役未知调用');
  assert.equal(ui.requests.at(-1).action, 'session');
  assert.equal(ui.container.textContent.includes('未解开的规划调用'), false, 'the retired invocation disappears from the blocked panel');
});

test('feedback displays queued acknowledgement separately from applied input and Plan stays scoped', async t => {
  let feedbackCommand, adopted = false;
  const ui = await mountPanel(t, request => {
    if (request.planning?.action === 'feedback') feedbackCommand = request.planning;
    const p = planningState(adopted ? { feedback: [{ 'request-id': feedbackCommand['request-id'], text: feedbackCommand.text, target: designRef, source: 'user feedback' }] } : {});
    return reply(request, { state: planningTask(p), ...(request.planning ? { 'planning-control': { 'request-id': request.planning['request-id'], status: request.planning.action === 'feedback' ? 'queued' : 'applied' } } : {}) });
  });
  await ui.selectPath(0, '/repo/docs/planning'); await ui.click('刷新');
  const input = ui.container.querySelector('textarea[aria-label="方案反馈"]');
  await act(async () => { Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLTextAreaElement.prototype, 'value').set.call(input, 'Keep backward compatibility'); input.dispatchEvent(new ui.document.defaultView.Event('input', { bubbles: true })); });
  await ui.click('提交反馈');
  assert.deepEqual(feedbackCommand.design, designRef); assert.equal(feedbackCommand.revision, 7); assert.equal(feedbackCommand.start, false);
  assert.equal(ui.container.querySelector('[data-planning-receipt]').dataset.planningReceipt, 'queued');
  assert.equal(ui.container.querySelector('[aria-label="方案反馈"]').textContent.includes('已采用的反馈'), false);
  adopted = true; await ui.click('刷新'); assert.equal(ui.container.querySelector('[data-planning-receipt]').dataset.planningReceipt, 'applied');
  assert.deepEqual(ui.requests.at(-1).planning, { action: 'status', 'request-id': feedbackCommand['request-id'] });
  const instructions = ui.container.querySelector('textarea[aria-label="本次规划说明（可选）"]');
  await act(async () => { Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLTextAreaElement.prototype, 'value').set.call(instructions, 'Explain recovery within the approved API'); instructions.dispatchEvent(new ui.document.defaultView.Event('input', { bubbles: true })); });
  await ui.click('运行 Plan'); assert.equal(ui.requests.at(-1).planning.scope, 'design'); assert.equal(ui.requests.at(-1).planning.start, true); assert.equal(ui.requests.at(-1).planning.text, 'Explain recovery within the approved API'); assert.equal(ui.requests.some(r => r.action === 'start'), false);
  assert.equal(ui.requests.filter(r => r.planning?.action === 'feedback').length, 1, 'Plan instructions do not create another global feedback mutation');
});

test('a queued feedback rejection is read by its original request id and never shown as adopted', async t => {
  let queued;
  const ui = await mountPanel(t, request => {
    const extra = {};
    if (request.planning?.action === 'feedback') {
      queued = request.planning['request-id'];
      extra['planning-control'] = { 'request-id': queued, status: 'queued', message: 'Waiting for the original receipt' };
    } else if (request.planning?.action === 'status') {
      assert.equal(request.planning['request-id'], queued);
      extra['planning-control'] = { 'request-id': queued, status: 'rejected', message: 'Input changed before adoption; inspect the original request' };
    }
    return reply(request, { state: planningTask(), ...extra });
  });
  await ui.selectPath(0, '/repo/docs/planning'); await ui.click('刷新');
  const input = ui.container.querySelector('textarea[aria-label="方案反馈"]');
  await act(async () => { Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLTextAreaElement.prototype, 'value').set.call(input, 'Discuss a different recovery constraint'); input.dispatchEvent(new ui.document.defaultView.Event('input', { bubbles: true })); });
  await ui.click('提交反馈');
  assert.equal(ui.container.querySelector('[data-planning-receipt]').dataset.planningReceipt, 'queued');
  await ui.click('刷新');
  assert.equal(ui.container.querySelector('[data-planning-receipt]').dataset.planningReceipt, 'rejected');
  assert.match(ui.container.textContent, /Input changed before adoption/);
  assert.equal(ui.requests.filter(r => r.planning?.action === 'feedback').length, 1, 'status never resubmits the feedback');
  assert.equal(ui.requests.some(r => ['start', 'launch'].includes(r.action)), false);
});


test('an unknown approval cannot repeat and a refreshed design uses its new exact identity', async t => {
  let failed = false;
  const next = { revision: 3, digest: 'c'.repeat(64) };
  const ui = await mountPanel(t, request => {
    if (request.planning?.action === 'approve' && !failed) { failed = true; throw new Error('reply lost after dispatch'); }
    const p = planningState();
    if (failed) {
      p['current-design'] = next;
      p.designs.push({ ...p.designs.at(-1), ref: next, document: 'Reviewed revised design' });
      p.reviews.push({ ...p.reviews.at(-1), id: 'review-3', target: next });
    }
    return reply(request, { state: planningTask(p, failed ? 10 : 7) });
  });
  await ui.selectPath(0, '/repo/docs/planning'); await ui.click('刷新');
  await ui.click('批准此版本'); assert.equal(ui.button('批准此版本').disabled, true);
  await ui.click('批准此版本'); assert.equal(ui.requests.length, 2);
  await ui.click('刷新'); assert.equal(ui.button('批准此版本').disabled, false);
  await ui.click('批准此版本');
  assert.deepEqual(ui.requests.at(-1).planning.design, next); assert.equal(ui.requests.at(-1).planning.revision, 10);
});

test('root approval and reviewed child activation remain separate records', async t => {
  const planRef = { revision: 1, digest: 'd'.repeat(64) };
  const p = planningState({ phase: 'approved', approvals: [{ kind: 'human', design: designRef, 'review-id': 'review-2', source: 'dsh:review' }],
    plans: [{ ref: planRef, design: designRef, scope: 'milestone', 'scope-id': 'M01', files: { 'task-packet-M01-T01.md': 'Checkpoint acceptance' } }], 'current-plans': { 'milestone:M01': planRef } });
  p.reviews.push({ id: 'child-review', kind: 'plan', target: planRef, verdict: 'revise', summary: 'Add the interrupted checkpoint test', findings: [] });
  const ui = await mountPanel(t, request => reply(request, { state: planningTask(p) }));
  await ui.selectPath(0, '/repo/docs/planning'); await ui.click('刷新');
  const child = ui.container.querySelector('[aria-label="当前执行子计划"]');
  assert.match(child.textContent, /Add the interrupted checkpoint test/); assert.match(child.textContent, /尚未激活/);
  assert.match(child.textContent, /Checkpoint acceptance/); assert.equal(ui.button('批准此版本').disabled, true);
  assert.equal(ui.button('开始已批准方案').disabled, false, 'root start authority is independent; Go still enforces child review before execution');
});

test('inherited planning settings do not silently save channels and editing creates only a new role binding', async t => {
  const values = { loops: { plan: { harness: 'kimi', model: 'k3', channel: 'kimi-subscription', thinking: 'high', 'resident-session': true }, review: { harness: 'dsh', model: 'review-model', provider: 'chatgpt', channel: 'chatgpt', thinking: 'high' } }, channels: { 'kimi-subscription': { 'env-key': null }, chatgpt: { 'env-key': null } }, sessions: { mode: 'scoped', 'policy-version': 1 } };
  const ui = await mountPanel(t, request => request.action === 'config'
    ? reply(request, { state: null, configuration: { source: '/config', scope: 'global', bindings: [] } })
    : reply(request, { state: null, editor: { scope: request.editor.scope, revision: 'config-one', target: '/config', values, defaults: values } }));
  await ui.click('设置'); await ui.click('模型与会话');
  assert.match(ui.container.textContent, /继承 loops.plan/);
  const input = ui.container.querySelector('[data-binding-id="loops.design-plan"] input');
  await act(async () => { Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLInputElement.prototype, 'value').set.call(input, 'design-model'); input.dispatchEvent(new ui.document.defaultView.Event('input', { bubbles: true })); });
  await ui.click('保存修改');
  assert.deepEqual(ui.requests.map(request => request.action), ['config-edit', 'config', 'config-save', 'config']);
  const saved = ui.requests.find(request => request.action === 'config-save').editor.values;
  assert.deepEqual(Object.keys(saved), ['loops']);
  assert.deepEqual(saved.loops.plan, values.loops.plan); assert.deepEqual(saved.loops.review, values.loops.review);
  assert.equal(saved.loops['design-plan'].model, 'design-model'); assert.equal(saved.loops['design-plan']['resident-session'], undefined);
  assert.equal(saved.loops['design-review'], undefined); assert.equal(saved.loops['plan-review'], undefined);
});
