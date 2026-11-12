import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, realpath, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { act, mountPanel, closeClientHarness } from './support/client-harness.js';
import { runControl, ControlNotDispatchedError } from '../src/control.js';
import { taskAssociationProjection as projection } from '../src/task-association.js';

after(closeClientHarness);
const reply = (request, fields) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({ protocol: 1, ok: true, running: false, taskDir: request.taskDir ?? '', state: null, ...fields }) } } });
const preparation = (fields = {}) => ({
  version: 1, revision: 7, 'run-id': 'investigation-run', status: 'ready', reason: 'approved-read-only-actions',
  contract: { id: 'failure', goal: 'Explain why the request fails', intent: 'investigate', limits: { 'max-actions': 3 } },
  'actions-used': 0, 'elapsed-ms': 0, observations: [], decisions: [], ...fields,
});
const report = { status: 'supported', summary: 'The log supports the timeout hypothesis', findings: [{ hypothesis: 'timeout', status: 'supported', reason: 'The configured timeout precedes the reply', evidence: ['action-1'] }], 'counter-evidence': 'The service is reachable', limitations: ['One bounded trace'], recommendation: 'Review the timeout setting', regression: 'Exercise a delayed response', rollback: 'Restore the recorded value' };
const invocation = { id: 'original-action', 'action-id': 'inspect-log', reason: 'baseline' };

test('report-only preparation displays its evidence report without implementation launch or completion', async t => {
  const p = preparation({ status: 'supported', report, 'actions-used': 1, observations: [{ invocation, status: 'completed', output: 'A timeout occurred', 'duration-ms': 4, truncated: true }] });
  const ui = await mountPanel(t, request => reply(request, { preparation: p }));
  await ui.selectPath(0, '/repo/docs/failure');
  await ui.click('刷新状态');
  const panel = ui.container.querySelector('[aria-label="有界调查"]');
  assert.ok(panel);
  assert.match(panel.textContent, /报告有证据支持/);
  assert.match(panel.textContent, /本目标以调查报告结束；没有启动代码实现/);
  assert.match(panel.textContent, /The log supports the timeout hypothesis/);
  assert.match(panel.textContent, /有界片段/);
  assert.equal(ui.container.querySelector('[data-node-id]'), null);
  assert.equal([...ui.container.querySelectorAll('button')].some(button => ['启动', '创建并启动'].includes(button.textContent)), false);
  assert.equal(ui.button('继续调查').disabled, true);
  assert.equal(ui.button('停止').disabled, true);
  assert.equal(ui.requests.length, 1);
  await ui.setLocale('en');
  assert.match(ui.container.textContent, /Report supported by cited evidence/);
  assert.match(ui.container.textContent, /implementation has not started/);
});

test('continue binds the observed preparation revision and running stop uses the owned background run', async t => {
  let running = false;
  const ui = await mountPanel(t, request => {
    if (request.action === 'investigate') running = true;
    if (request.action === 'stop') running = false;
    return reply(request, { running, preparation: preparation({ revision: running ? 8 : 7, status: running ? 'running' : 'ready' }) });
  });
  await ui.selectPath(0, '/repo/docs/failure');
  await ui.click('刷新状态');
  await ui.click('继续调查');
  assert.deepEqual(ui.requests.at(-1), { action: 'investigate', taskDir: '/repo/docs/failure', investigation: { action: 'run', revision: 7 } });
  assert.equal(ui.button('继续调查').disabled, true);
  assert.equal(ui.button('停止').disabled, false);
  await ui.click('继续调查');
  assert.equal(ui.requests.length, 2, 'an active owner cannot be launched twice');
  await ui.click('停止');
  assert.deepEqual(ui.requests.at(-1), { action: 'stop', taskDir: '/repo/docs/failure' });
  assert.equal(ui.button('继续调查').disabled, false);
});

test('an unconfirmed investigation cannot restart; status leads only to explicit original-receipt reconciliation', async t => {
  let uncertain = false;
  const ui = await mountPanel(t, request => {
    if (request.action === 'investigate') { uncertain = true; throw new Error('connection closed after dispatch'); }
    return reply(request, { preparation: preparation(uncertain ? { revision: 9, status: 'blocked', pending: invocation } : {}) });
  });
  await ui.selectPath(0, '/repo/docs/failure');
  await ui.click('刷新状态');
  await ui.click('继续调查');
  assert.equal(ui.button('继续调查').disabled, true);
  assert.equal(ui.button('停止').disabled, true);
  await ui.click('继续调查');
  assert.equal(ui.requests.length, 2);
  await ui.click('刷新状态');
  assert.equal(ui.requests.at(-1).action, 'status');
  assert.equal(ui.button('核对原动作回执').disabled, false);
  assert.equal(ui.button('停止').disabled, true, 'a pending executor receipt must be reconciled before cancellation');
  await ui.click('核对原动作回执');
  assert.deepEqual(ui.requests.at(-1).investigation, { action: 'run', revision: 9 });
  assert.equal(ui.requests.some(request => ['launch', 'start'].includes(request.action)), false);
});

test('an idle optional decision can be cancelled without resuming any baseline action', async t => {
  const p = preparation({ contract: { ...preparation().contract, intent: 'investigate-and-fix' }, 'decision-pending': { 'decision-id': 'choice-1' } });
  const ui = await mountPanel(t, request => reply(request, { preparation: request.investigation?.action === 'cancel' ? { ...p, status: 'cancelled', 'decision-pending': undefined } : p }));
  await ui.selectPath(0, '/repo/docs/failure');
  await ui.click('刷新状态');
  assert.match(ui.container.textContent, /形成合法 Spec 后由 Auto 选择 Quick \/ Full/);
  assert.equal(ui.button('停止').disabled, false);
  assert.equal(ui.button('核对原动作回执').disabled, false);
  await ui.click('停止');
  assert.deepEqual(ui.requests.at(-1).investigation, { action: 'cancel', revision: 7 });
  assert.match(ui.container.textContent, /调查已取消/);
  assert.equal(ui.button('停止').disabled, true);
  assert.equal(ui.requests.some(request => request.investigation?.action === 'run'), false);
  assert.equal(ui.requests.some(request => ['launch', 'start'].includes(request.action)), false);
});

test('nested investigation status accepts the implementation handoff but a mutation still needs its preparation receipt', async t => {
  const state = { revision: 2, status: 'paused', packets: {} };
  const ui = await mountPanel(t, request => reply(request, { state }));
  await ui.selectPath(0, '/repo/docs/failure');
  const { run } = [...ui.bindings.values()][0].props;
  await act(async () => run('investigate', ui.selection(), false, { investigation: { action: 'status' } }));
  assert.deepEqual(ui.requests.at(-1).investigation, { action: 'status' });
  assert.equal(ui.container.querySelector('[aria-label="有界调查"]'), null);
  assert.equal(ui.button('启动').disabled, false);
  assert.doesNotMatch(ui.container.textContent, /malformed investigation status/);
  await act(async () => run('investigate', ui.selection(), false, { investigation: { action: 'run', revision: 7 } }));
  assert.match(ui.container.textContent, /malformed investigation status/);
  assert.equal(ui.button('启动').disabled, true, 'ordinary implementation state cannot confirm a preparation mutation');
});

test('malformed preparation responses remain a visible transport failure, without crashing the renderer', async t => {
  let malformed = false;
  const ui = await mountPanel(t, request => reply(request, { preparation: preparation(malformed ? { report: { summary: 'bad', findings: {} } } : {}) }));
  await ui.selectPath(0, '/repo/docs/failure');
  await ui.click('刷新状态');
  malformed = true;
  await ui.click('继续调查');
  assert.match(ui.container.textContent, /malformed investigation status/);
  assert.equal(ui.button('继续调查').disabled, true);
  assert.ok(ui.container.querySelector('[aria-label="有界调查"]'));
});

test('untrusted response text must be text before entering the report renderer', async t => {
  let fields = {};
  const ui = await mountPanel(t, request => reply(request, { preparation: preparation(fields) }));
  await ui.selectPath(0, '/repo/docs/failure');
  await ui.click('刷新状态');
  for (const malformed of [{ reason: {} }, { observations: [{ invocation, output: {} }] }, { report: { ...report, recommendation: {} } }]) {
    fields = malformed;
    await ui.click('刷新状态');
    assert.match(ui.container.textContent, /malformed investigation status/);
    assert.ok(ui.container.querySelector('[aria-label="有界调查"]'));
    assert.equal(ui.button('继续调查').disabled, true);
    fields = {};
    await ui.click('刷新状态');
    assert.equal(ui.button('继续调查').disabled, false);
  }
});

test('typed investigation transport uses bounded stdin and rejects invalid envelopes before dispatch', async t => {
  const dir = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-investigation-')));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const binary = join(dir, 'engine');
  await writeFile(binary, '#!/usr/bin/env node\nlet raw="";process.stdin.on("data",c=>raw+=c);process.stdin.on("end",()=>{const body=JSON.parse(raw),args=process.argv.slice(2);console.log(JSON.stringify({protocol:1,ok:true,running:body.action==="run",state:null,taskDir:args[args.indexOf("--task-dir")+1],cwd:args[args.indexOf("--cwd")+1],preparation:' + JSON.stringify(preparation()) + ',received:body,argv:args}));});\n', { mode: 0o755 });
  const base = { action: 'investigate', taskDir: dir, cwd: dir };
  const bodies = [
    { action: 'start', contract: { goal: 'Inspect literal $(never-execute) content' } },
    { action: 'run', revision: 7 }, { action: 'status' }, { action: 'cancel', revision: 8 },
    { action: 'report', revision: 9, report },
  ];
  for (const body of bodies) {
    const value = await runControl({ ...base, investigation: body }, { binary });
    assert.deepEqual(value.received, body);
    assert.deepEqual(value.argv, ['control', 'investigate', '--cwd', dir, '--compact', '--task-dir', dir]);
    assert.equal(value.running, body.action === 'run');
  }
  const invalid = [undefined, null, {}, { action: 'launch' }, { action: 'start' }, { action: 'start', contract: null },
    { action: 'start', contract: [] }, { action: 'start', contract: {}, report }, { action: 'run' },
    { action: 'run', revision: 0 }, { action: 'run', revision: 1.5 }, { action: 'run', revision: 7, command: 'shell' },
    { action: 'status', revision: -1 }, { action: 'status', contract: {} }, { action: 'cancel', revision: 7, report },
    { action: 'report', revision: 7 }, { action: 'report', revision: 7, report: null },
    { action: 'start', contract: { goal: 'x'.repeat(131072) } }];
  for (const body of invalid) {
    await assert.rejects(runControl({ ...base, investigation: body }, { binary: join(dir, 'not-present') }), error => error instanceof ControlNotDispatchedError && !/ENOENT/.test(error.message));
  }
  await assert.rejects(runControl({ action: 'status', taskDir: dir, cwd: dir, investigation: { action: 'status' } }, { binary }), ControlNotDispatchedError);
});

test('host accepts only a valid implementation ledger for nested status after handoff', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'flywheel-investigation-handoff-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const binary = join(dir, 'engine');
  await writeFile(binary, '#!/usr/bin/env node\nlet raw="";process.stdin.on("data",c=>raw+=c);process.stdin.on("end",()=>{const body=JSON.parse(raw),args=process.argv.slice(2);console.log(JSON.stringify({protocol:1,ok:true,running:false,state:{revision:body.revision===99?"bad":2,status:"paused",packets:{}},taskDir:args[args.indexOf("--task-dir")+1],cwd:args[args.indexOf("--cwd")+1]}));});\n', { mode: 0o755 });
  const base = { action: 'investigate', taskDir: dir, cwd: dir };
  const value = await runControl({ ...base, investigation: { action: 'status' } }, { binary });
  assert.equal(value.state.revision, 2);
  assert.equal(value.preparation, undefined);
  await assert.rejects(runControl({ ...base, investigation: { action: 'status', revision: 99 } }, { binary }), /missing its requested report/);
  for (const body of [{ action: 'start', contract: {} }, { action: 'run', revision: 7 }, { action: 'cancel', revision: 7 }, { action: 'report', revision: 7, report }]) {
    await assert.rejects(runControl({ ...base, investigation: body }, { binary }), /missing its requested report/);
  }
});

test('native investigation tool association preserves unknown identity until the same task status is observed', () => {
  let state = projection.init({ cwd: '/repo' }, 0), sequence = 0;
  const append = (type, data) => { state = projection.apply(state, { type, data, seq: sequence++ }); projection.stateSchema.parse(state); };
  const call = (id, args) => append('tool/call', { callId: id, name: 'flywheel_control', arguments: JSON.stringify(args) });
  const result = (id, value, isError = false) => append('tool/result', { message: { toolCallId: id, content: [{ type: 'text', text: JSON.stringify(value) }], isError } });
  call('bad', { action: 'investigate', taskDir: '/repo/docs/task', investigation: { action: 'run' } });
  assert.equal(state.view, null);
  call('first', { action: 'investigate', taskDir: '/repo/docs/task', investigation: { action: 'run', revision: 7 } });
  assert.equal(state.view.outcome, 'pending');
  result('first', { protocol: 1, type: 'flywheel-control-error', action: 'investigate', dispatch: 'unknown', message: 'unconfirmed' }, true);
  call('second', { action: 'investigate', taskDir: '/repo/docs/other', investigation: { action: 'run', revision: 3 } });
  assert.equal(state.view.taskDir, '/repo/docs/task');
  assert.equal(state.view.outcome, 'unknown');
  call('status', { action: 'status', taskDir: '/repo/docs/task' });
  result('status', { protocol: 1, ok: true, running: false, taskDir: '/repo/docs/task', cwd: '/repo', preparation: preparation({ pending: invocation }) });
  assert.equal(state.view.outcome, 'confirmed');
  assert.equal(state.view.taskDir, '/repo/docs/task');
});
