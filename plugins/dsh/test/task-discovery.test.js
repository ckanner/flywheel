import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, mountPanel, closeClientHarness, deferred, deadlineClock } from './support/client-harness.js';
after(closeClientHarness);
const reply = (r, fields) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({ protocol: 1, ok: true, running: false, taskDir: r.taskDir ?? '', cwd: r.cwd ?? '/repo', state: null, ...fields }) } } });
const item = (id, owner, name = id) => ({ id, hostSessionId: owner, taskDir: '/repo/docs/' + name, cwd: '/repo', kind: 'review', startedAt: '2026-11-10T10:00:00Z', summary: { slug: name, status: 'pending' } });
const stageState = (id = 'run-one', status = 'passed') => ({
 slug: 'questionnaire', status: 'pending', revision: 48, workflow: { effective: 'full' },
 run: { id, kind: 'review', bindings: { 'loops.review': { harness: 'dsh', provider: 'chatgpt', model: 'gpt-6.1-sol', thinking: 'high' } } },
 control: { kind: 'review', packet: 'M01-T01', status },
 packets: { 'M01-T01': { status: 'l1-passed', attempts: 0, evidence: { l1: { pass: true, identity: 'same' }, review: { verdict: 'pass', score: 0.92, identity: 'same' } } } },
});
async function select(ui, run) {
 const choice = ui.container.querySelector('section[aria-label="选择任务"] select');
 assert.ok(choice);
 await act(async () => { choice.value = JSON.stringify([run.taskDir,run.cwd]); choice.dispatchEvent(new ui.document.defaultView.Event('change', { bubbles: true })); });
}

test('opening discovers registered runs without starting a task and never renders path inputs', async t => {
 const ui = await mountPanel(t, request => reply(request, {}));
 assert.deepEqual(ui.requests, []);
 assert.deepEqual(ui.discoveryRequests, [{ action: 'runs' }]);
 assert.equal(ui.container.querySelectorAll('input').length, 0);
 assert.doesNotMatch(ui.container.textContent, /手动指定|Users\/me/);
 assert.match(ui.container.textContent, /加载 Skill 不代表引擎已经启动/);
 assert.equal(ui.button('刷新任务列表').disabled, false);
});

test('one current-session CLI task auto-opens a fresh stage canvas; other sessions never win', async t => {
 const own = item('run-one','desktop-session');
 const other = item('other','other-session');
 const older = { ...own, id: 'older', startedAt: '2026-11-09T10:00:00Z' };
 const ui = await mountPanel(t, request => reply(request, { state: stageState() }), { catalog: request => reply(request, { runs: [other,older,own] }) });
 assert.deepEqual(ui.requests, [{ action: 'status', taskDir: own.taskDir, cwd: own.cwd }]);
 assert.match(ui.container.textContent, /本次评审已通过/);
 assert.equal(ui.container.querySelectorAll('input').length, 0);
 assert.match(ui.container.querySelector('[aria-label="任务位置（自动取得）"]').textContent, /docs\/run-one/);
 assert.match(ui.container.textContent, /整体交付：尚未完成/);
 assert.ok(ui.container.querySelector('[data-node-id="M01-T01/review"]'));
 assert.equal(ui.container.querySelector('[data-node-id="plan"]'), null);
 assert.equal(ui.container.querySelector('[data-node-id="merge"]'), null);
 assert.equal(ui.container.querySelector('[data-node-id="M01-T01/review"]').getAttribute('data-tone'), 'success');
 assert.equal(ui.container.querySelector('[aria-label="节点详情"] h3').textContent, 'M01-T01 · 评审');
 const continueDetails = [...ui.container.querySelectorAll('details')].find(x => x.querySelector('summary')?.textContent === '需要继续完整流程时');
 assert.equal(continueDetails.open, false);
 await ui.click('完整流程');
 assert.ok(ui.container.querySelector('[data-node-id="plan"]'));
 assert.ok(ui.container.querySelector('[data-node-id="merge"]'));
 await ui.click('本次运行');
 await ui.click('运行此阶段');
 assert.deepEqual(ui.requests.at(-1), { action: 'step', taskDir: own.taskDir, cwd: own.cwd, only: 'review', packet: 'M01-T01', revision: 48 });
});

test('legacy runs and multiple associated tasks require an explicit choice', async t => {
 const legacy = item('legacy',undefined);
 const rows = [legacy,item('a','desktop-session'),item('b','desktop-session')];
 const ui = await mountPanel(t, request => reply(request, { state: stageState('legacy') }), { catalog: request => reply(request,{runs:rows}) });
 assert.deepEqual(ui.requests, []);
 assert.match(ui.container.textContent,/本会话有多个任务/);
 await select(ui,legacy);
 assert.deepEqual(ui.requests,[{action:'status',taskDir:legacy.taskDir,cwd:legacy.cwd}]);
 await ui.click('刷新任务列表');
 assert.equal(ui.requests.length,1,'refreshing discovery cannot replace an explicit selection');
});

test('late discovery from an old session cannot select a task in the current session', async t => {
 const first = deferred();
 const other = item('new','other-session');
 const ui = await mountPanel(t, request => reply(request,{state:stageState('new')}), {catalog:(request,id)=>id==='desktop-session'?first.promise:reply(request,{runs:[other]})});
 await ui.switchSession('other-session');
 assert.deepEqual(ui.sessions,['other-session']);
 assert.equal(ui.requests[0].taskDir,other.taskDir);
 await act(async()=>first.resolve(reply({action:'runs'},{runs:[item('old','desktop-session')]})));
 assert.equal(ui.requests.length,1);
 assert.equal(ui.selection().taskDir,other.taskDir);
});

test('discovery stays independent of unresolved mutations and retains the selected task', async t => {
 const launch = deferred();
 let rows = [];
 const timers = deadlineClock();
 const ui = await mountPanel(t,request=>request.action==='start'?launch.promise:reply(request,{state:stageState()}), {timers,catalog:request=>reply(request,{runs:rows})});
 await ui.selectPath(0,'/repo/docs/manual'); await ui.selectPath(1,'/repo');
 await ui.click('启动');
 rows=[item('automatic','desktop-session')];
 await ui.click('刷新任务列表');
 assert.equal(ui.requests.filter(r=>r.action==='start').length,1);
 assert.equal(ui.selection().taskDir,'/repo/docs/manual');
 await act(async()=>launch.reject(new Error('transport lost')));
 await ui.click('刷新任务列表');
 assert.equal(ui.selection().taskDir,'/repo/docs/manual');
 assert.equal(ui.button('启动').disabled,true);
 assert.equal(ui.requests.some(r=>r.taskDir==='/repo/docs/automatic'),false);
});

test('failed or cancelled review receipts cannot inherit an older green verdict', async t => {
 for(const status of ['failed','cancelled','running']) {
  const one=item('run-one','desktop-session');
  const ui=await mountPanel(t,request=>reply(request,{state:stageState('run-one',status)}),{catalog:request=>reply(request,{runs:[one]})});
  assert.notEqual(ui.container.querySelector('[data-node-id="M01-T01/review"]').getAttribute('data-tone'),'success');
  assert.doesNotMatch(ui.container.querySelector('section[aria-label="运行画布"]').textContent,/本次评审已通过/);
  await ui.unload();
 }
});

test('a later full workflow is not mislabeled as review-only by a retained control receipt', async t => {
 const state=stageState();state.run.kind='workflow';state.status='running';
 const ui=await mountPanel(t,request=>reply(request,{state,running:true}),{catalog:request=>reply(request,{runs:[item('run-one','desktop-session')]})});
 assert.ok(ui.container.querySelector('[data-node-id="plan"]'));
 assert.doesNotMatch(ui.container.textContent,/本次评审已通过/);
});

test('catalog failures keep a loaded canvas and support retry without tasks changing', async t => {
 let fail=false;
 const one=item('run-one','desktop-session');
 const ui=await mountPanel(t,request=>reply(request,{state:stageState()}),{catalog:request=>fail?Promise.reject(new Error('unavailable')):reply(request,{runs:[one]})});
 fail=true;await ui.click('刷新任务列表');
 assert.ok(ui.container.querySelector('[data-node-id="M01-T01/review"]'));
 assert.match(ui.container.textContent,/任务列表读取失败/);
 fail=false;await ui.click('刷新任务列表');
 assert.doesNotMatch(ui.container.textContent,/任务列表读取失败/);
 assert.equal(ui.requests.length,1);
});

test('a registered run preceding a failed state save cannot create a status refresh loop', { timeout: 3000 }, async t => {
 const latest=item('not-persisted','desktop-session');
 const ui=await mountPanel(t,request=>reply(request,{state:stageState('previous-run')}),{catalog:request=>reply(request,{runs:[latest]})});
 assert.equal(ui.requests.length,1);
 await ui.click('刷新任务列表');
 assert.equal(ui.requests.length,2,'one status reconciliation for one new catalog snapshot');
});

test('prepared tasks associated by status are selectable without a run index or a path form', async t => {
 const prepared = { operationId:'command:prepare', sequence:4, action:'status', taskDir:'/repo/docs/prepared', cwd:'/repo', outcome:'confirmed', message:'' };
 const other = {...prepared,taskDir:'/repo/docs/another',operationId:'command:another',sequence:3};
 const ui=await mountPanel(t,request=>reply(request,{state:null}),{association:{...prepared,tasks:[prepared,other]}});
 await ui.click('切换任务');
 const options=[...ui.container.querySelector('section[aria-label="选择任务"] select').options].map(x=>x.value);
 assert.ok(options.includes(JSON.stringify([other.taskDir,other.cwd])));
 await select(ui,other);
 assert.equal(ui.requests.at(-1).taskDir,other.taskDir);
 assert.equal(ui.container.querySelector('[data-task-path]').getAttribute('data-task-path'),other.taskDir);
 assert.equal(ui.container.querySelectorAll('input').length,0);
 assert.equal(ui.requests.every(x=>x.action==='status'),true);
});

test('location rows appear only when the engine or conversation supplies real values', async t => {
 let known=false;
 const ui=await mountPanel(t,request=>reply(request,{state:null}),{catalog:request=>reply(request,{cwd:known?'/known/workspace':undefined,runs:[]})});
 assert.equal(ui.container.querySelector('[data-workspace-path]'),null);
 assert.equal(ui.container.querySelector('[data-task-path]'),null);
 assert.equal(ui.container.querySelectorAll('input').length,0);
 known=true;await ui.click('刷新任务列表');
 assert.equal(ui.container.querySelector('[data-workspace-path]').textContent,'/known/workspace');
 assert.equal(ui.container.querySelector('[data-task-path]'),null);
 assert.equal(ui.container.querySelectorAll('input').length,0);
});
