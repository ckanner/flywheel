import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, mountPanel, closeClientHarness } from './support/client-harness.js';

after(closeClientHarness);
const call = (fields = {}) => ({ id: 'call-exact1', 'started-at': '2026-11-21T06:00:00Z', attempt: 1,
  status: 'consumed', dispatch: 'dispatched', outcome: 'completed', 'cleanup-confirmed': true, ...fields });
const entry = (status, invocation) => ({ id: 'initial-M01-T01', packet: 'M01-T01', attempt: 0, action: 'model-0', status: 'applied',
  'started-at': '2026-11-21T05:59:59Z', selection: { 'action-id': 'model-0', adopted: true, reason: 'approved-model-choice' },
  'model-bindings': { 'model-0': { bindingId: 'native', binding: { harness: 'antigravity', model: 'gemini-pro-orchestrator-neon', thinking: '', nativeOptions: { version: 1, mode: 'native' } } } },
  ...(status ? { 'execution-view': { status, reason: 'recorded-evidence', ...(invocation ? { invocation } : {}) } } : {}) });
const state = row => ({ revision: 7, status: 'paused', packets: { 'M01-T01': { status: 'executing', attempts: 1, evidence: {} } },
  'runtime-decisions': { version: 1, used: 1, 'imported-calls': 0, status: 'enabled', records: [row] } });
const reply = (request, value) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({ protocol: 1, ok: true, running: false,
  taskDir: request.taskDir ?? '', cwd: '/repo', state: value }) } } });

test('decision cards show selected, accepted live execution and terminal receipts for the exact linked call', async t => {
  let value = state(entry('selected'));
  const ui = await mountPanel(t, request => reply(request, value));
  await ui.selectPath(0, '/repo/docs/task'); await ui.click('刷新');
  const status = () => ui.container.querySelector('[data-runtime-decisions="task"] [data-decision-execution]');
  assert.match(status().textContent, /已选择，尚未启动执行调用/);
  for (const [phase, invocation, text] of [
    ['unknown', call({ status: 'intent', dispatch: 'unknown', outcome: 'unknown', 'cleanup-confirmed': false }), /执行状态未知/],
    ['started', call({ status: 'intent', dispatch: 'unknown', 'dispatch-observed': true, outcome: 'unknown', 'cleanup-confirmed': false }), /已启动，执行结果或收尾仍待确认/],
    ['completed', call(), /对应执行调用已完成/],
    ['failed', call({ outcome: 'failed' }), /对应执行调用失败/],
    ['cancelled', call({ outcome: 'cancelled' }), /对应执行调用已取消/],
    ['not-started', call({ dispatch: 'not-dispatched', outcome: 'failed' }), /已确认未派发/],
  ]) {
    value = state(entry(phase, invocation)); await ui.click('刷新');
    assert.equal(status().dataset.decisionExecution, phase);
    assert.match(status().textContent, text);
    assert.match(status().textContent, /call-exact1/);
    assert.match(status().textContent, /任务验收和实际子智能体结果另行记录/);
  }
  value = state(entry('completed', call())); await ui.click('刷新');
  await act(async () => ui.container.querySelector('[data-node-id="M01-T01"]').click());
  assert.match(ui.container.querySelector('[data-runtime-decisions="M01-T01"] [data-decision-execution]').textContent, /对应执行调用已完成/);
  await ui.setLocale('en');
  assert.match(status().textContent, /Linked execution call completed/);
  assert.equal(ui.requests.every(request => request.action === 'status'), true, 'displaying a receipt does not run a model');
});

test('legacy decisions and contradictory lifecycle payloads never become completed executions', async t => {
  let value = state(entry());
  const ui = await mountPanel(t, request => reply(request, value));
  await ui.selectPath(0, '/repo/docs/task'); await ui.click('刷新');
  assert.match(ui.container.querySelector('[data-decision-execution]').textContent, /执行状态未知/);
  for (const invalid of [
    { status: 'completed', reason: 'unlinked' },
    { status: 'selected', reason: 'has-a-call', invocation: call() },
    { status: 'completed', reason: 'wrong-dispatch', invocation: call({ dispatch: 'unknown' }) },
    { status: 'completed', reason: 'wrong-outcome', invocation: call({ outcome: 'failed' }) },
    { status: 'completed', reason: 'cleanup-unknown', invocation: call({ 'cleanup-confirmed': false }) },
    { status: 'started', reason: 'possible-only', invocation: call({ dispatch: 'unknown' }) },
    { status: 'not-started', reason: 'contradiction', invocation: call({ dispatch: 'not-dispatched', 'dispatch-observed': true }) },
    { status: 'unknown', reason: {}, invocation: call() },
    { status: 'unknown', reason: 'bad-call', invocation: call({ id: 'not-a-call' }) },
    { status: 'unknown', reason: 'bad-observation', invocation: call({ 'dispatch-observed': {} }) },
  ]) {
    value = state({ ...entry(), 'execution-view': invalid }); await ui.click('刷新');
    assert.match(ui.container.textContent, /malformed task status/);
    assert.equal(ui.container.querySelector('[data-decision-execution]').dataset.decisionExecution, 'unknown');
    value = state(entry()); await ui.click('刷新');
  }
});
