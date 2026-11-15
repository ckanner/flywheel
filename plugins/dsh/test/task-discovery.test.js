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

test('opening discovers registered runs without starting a task and never renders path inputs', async t => {
 const ui = await mountPanel(t, request => reply(request, {}));
 assert.deepEqual(ui.requests, []);
 assert.deepEqual(ui.discoveryRequests, [{ action: 'runs' }]);
 assert.equal(ui.container.querySelectorAll('input').length, 0);
 assert.doesNotMatch(ui.container.textContent, /手动指定|Users\/me/);
 assert.match(ui.container.textContent, /加载 Skill 本身不会启动引擎/);
 assert.equal(ui.container.querySelector('details'), null);
 assert.doesNotMatch(ui.container.textContent, /切换任务|选择任务|刷新任务列表/);
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
 assert.ok(ui.container.querySelector('[data-node-id="plan"]'));
 await ui.click('本次运行');
 assert.ok(ui.container.querySelector('[data-node-id="M01-T01/review"]'));
 assert.equal(ui.container.querySelector('[data-node-id="plan"]'), null);
 assert.equal(ui.container.querySelector('[data-node-id="merge"]'), null);
 assert.equal(ui.container.querySelector('[data-node-id="M01-T01/review"]').getAttribute('data-tone'), 'success');
 assert.equal(ui.container.querySelector('[aria-label="节点详情"]'), null, 'details are opened deliberately');
 await act(async () => ui.container.querySelector('[data-node-id="M01-T01/review"]').click());
 assert.equal(ui.container.querySelector('[aria-label="节点详情"] h3').textContent, 'M01-T01 · 评审');
 await ui.click('任务全貌');
 assert.ok(ui.container.querySelector('[data-node-id="plan"]'));
 assert.ok(ui.container.querySelector('[data-node-id="delivery"]'));
 await ui.click('本次运行');
 await act(async () => ui.container.querySelector('[data-node-id="M01-T01/review"]').click());
 await ui.click('运行此阶段');
 assert.deepEqual(ui.requests.at(-1), { action: 'step', taskDir: own.taskDir, cwd: own.cwd, only: 'review', packet: 'M01-T01', revision: 48 });
});

test('foreign and ownerless runs stay invisible; multiple owned tasks select the latest deterministically', async t => {
 const rows = [item('legacy', undefined), item('foreign', 'other-session'), item('a', 'desktop-session'), { ...item('b', 'desktop-session'), startedAt:'2026-11-11T10:00:00Z' }];
 const ui = await mountPanel(t, request => reply(request,{state:stageState('b')}), { catalog: request => reply(request,{runs:rows}) });
 assert.equal(ui.selection().taskDir,'/repo/docs/b');
 assert.equal(ui.container.querySelector('section[aria-label="当前任务"] select'),null);
 assert.doesNotMatch(ui.container.textContent,/docs\/foreign|docs\/legacy/);
 assert.equal(ui.requests.every(r=>r.taskDir==='/repo/docs/b'),true);
});

test('a session with no owned task remains empty even if other projects have recent runs', async t => {
 const ui = await mountPanel(t, request=>reply(request,{}), {catalog:request=>reply(request,{runs:[item('foreign','other-session'),item('legacy',undefined)]})});
 assert.equal(ui.selection().taskDir,'');
 assert.equal(ui.requests.length,0);
 assert.equal(ui.container.querySelector('[data-task-path]'),null);
 await ui.click('记录');
 assert.doesNotMatch(ui.container.textContent,/docs\/foreign|docs\/legacy/);
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
 await ui.click('继续流程');
 rows=[item('automatic','desktop-session')];
 await ui.refreshDiscovery();
 assert.equal(ui.requests.filter(r=>r.action==='start').length,1);
 assert.equal(ui.selection().taskDir,'/repo/docs/manual');
 assert.equal(ui.container.querySelector('[data-task-path]').getAttribute('data-task-path'),'/repo/docs/manual');
 await act(async()=>launch.reject(new Error('transport lost')));
 await ui.refreshDiscovery();
 assert.equal(ui.selection().taskDir,'/repo/docs/manual');
 assert.equal(ui.button('继续流程').disabled,true);
 assert.equal(ui.requests.some(r=>r.taskDir==='/repo/docs/automatic'),false);
});

test('failed or cancelled review receipts cannot inherit an older green verdict', async t => {
 for(const status of ['failed','cancelled','running']) {
  const one=item('run-one','desktop-session');
  const ui=await mountPanel(t,request=>reply(request,{state:stageState('run-one',status)}),{catalog:request=>reply(request,{runs:[one]})});
  await ui.click('本次运行');
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
 fail=true;await ui.refreshDiscovery();
 assert.ok(ui.container.querySelector('[data-node-id="plan"]'));
 assert.match(ui.container.textContent,/当前会话任务读取失败/);
 fail=false;await ui.refreshDiscovery();
 assert.doesNotMatch(ui.container.textContent,/当前会话任务读取失败/);
 assert.equal(ui.requests.length,1);
});

test('a registered run preceding a failed state save cannot create a status refresh loop', { timeout: 3000 }, async t => {
 const latest=item('not-persisted','desktop-session');
 const ui=await mountPanel(t,request=>reply(request,{state:stageState('previous-run')}),{catalog:request=>reply(request,{runs:[latest]})});
 assert.equal(ui.requests.length,1);
 await ui.refreshDiscovery();
 assert.equal(ui.requests.length,2,'one status reconciliation for one new catalog snapshot');
});

test('prepared task follows its session projection without selection or run history', async t => {
 const prepared = { operationId:'tool:prepare', sequence:4, action:'status', taskDir:'/repo/docs/prepared', cwd:'/repo', outcome:'confirmed', message:'' };
 const other = {...prepared,taskDir:'/repo/docs/another',operationId:'tool:another',sequence:3};
 const ui=await mountPanel(t,request=>reply(request,{state:null}),{association:{...prepared,tasks:[prepared,other]}});
 assert.equal(ui.container.querySelector('[data-task-path]').getAttribute('data-task-path'),prepared.taskDir);
 assert.doesNotMatch(ui.container.textContent,/切换任务|选择任务/);
 assert.equal(ui.requests.every(x=>x.action==='status' && x.taskDir===prepared.taskDir),true);
 assert.equal(ui.container.querySelector('[data-task-path]').closest('details'),null);
});

test('location rows appear only when the engine or conversation supplies real values', async t => {
 let known=false;
 const ui=await mountPanel(t,request=>reply(request,{state:null}),{catalog:request=>reply(request,{cwd:known?'/known/workspace':undefined,runs:[]})});
 assert.equal(ui.container.querySelector('[data-workspace-path]'),null);
 assert.equal(ui.container.querySelector('[data-task-path]'),null);
 assert.equal(ui.container.querySelectorAll('input').length,0);
 known=true;await ui.refreshDiscovery();
 assert.equal(ui.container.querySelector('[data-workspace-path]').textContent,'/known/workspace');
 assert.equal(ui.container.querySelector('[data-task-path]'),null);
 assert.equal(ui.container.querySelectorAll('input').length,0);
});

test('new CLI runs in this session follow automatically without borrowing a foreign run identity', async t => {
 let rows=[item('first','desktop-session')];
 const ui=await mountPanel(t,r=>reply(r,{state:stageState(r.taskDir.split('/').at(-1))}),{catalog:r=>reply(r,{runs:rows})});
 rows=[{...item('second','desktop-session'),startedAt:'2026-11-12T10:00:00Z'}, {...item('foreign','other-session','second'),startedAt:'2026-11-13T10:00:00Z'}];
 await ui.refreshDiscovery();
 assert.equal(ui.selection().taskDir,'/repo/docs/second');
 assert.equal(ui.requests.at(-1).action,'status');
 assert.equal(ui.requests.filter(r=>r.action!=='status').length,0);
 assert.equal(ui.container.querySelector('details'),null);
});

test('an unavailable read Remote reports an error without falling back to chat commands', async t => {
 const ui=await mountPanel(t,r=>reply(r,{}),{mountError:new Error('read Remote unavailable')});
 assert.equal(ui.allRequests.length,0);
 assert.match(ui.container.textContent,/当前会话任务读取失败/);
 await ui.click('重试');
 assert.equal(ui.allRequests.length,0);
 assert.equal(ui.selection().taskDir,'');
});

test('task title, task ID and paths have distinct header roles while settings keeps association live', async t => {
 const own = item('run-one', 'desktop-session');
 const ui = await mountPanel(t, r => reply(r, { taskTitle: '验证筛选修复', state: stageState() }), {catalog:r=>reply(r,{runs:[own]})});
 const header=ui.container.querySelector('header');
 assert.equal(header.querySelector('h3').textContent,'验证筛选修复');
 assert.match(header.textContent,/任务 ID.*questionnaire/);
 assert.ok(header.querySelector('[data-task-path]'));
 assert.equal(ui.container.querySelector('footer [data-task-path]'),null);
 await ui.click('设置');
 assert.equal(header.hidden,true);
 await ui.setAssociation({sequence:55,operationId:'tool:next',action:'status',taskDir:'/repo/docs/next',cwd:'/repo',outcome:'confirmed',message:''});
 assert.equal(ui.selection().taskDir,'/repo/docs/next','hiding the header must not unmount association effects');
 await ui.click('← 返回流程');
 assert.equal(header.hidden,false);
});

test('a missing display title never presents a slug as a conversation name', async t => {
 const ui=await mountPanel(t,r=>reply(r,{state:stageState()}),{catalog:r=>reply(r,{runs:[item('run-one','desktop-session')]})});
 const header=ui.container.querySelector('header');
 assert.equal(header.querySelector('h3').textContent,'当前关联任务');
 assert.match(header.textContent,/任务 ID.*questionnaire/);
});

const taskButtons = ui => [...ui.container.querySelectorAll('[aria-label="本会话任务"] button[data-task-key]')];
const taskButton = (ui, slug) => taskButtons(ui).find(b => JSON.parse(b.dataset.taskKey)[0] === '/repo/docs/' + slug);
const ownedTasks = () => [item('a','desktop-session'), {...item('b','desktop-session'), startedAt:'2026-11-12T10:00:00Z'}];

test('conversation navigation deduplicates runs, reads selected task, and keeps manual choice on refresh', async t => {
 let rows=[...ownedTasks(), {...item('old-a','desktop-session','a'),startedAt:'2026-11-01T10:00:00Z'},item('foreign','foreign')];
 const ui=await mountPanel(t,r=>reply(r,{taskTitle:'Title '+r.taskDir,state:{slug:r.taskDir.split('/').at(-1),status:'pending',packets:{}}}),{catalog:r=>reply(r,{runs:rows})});
 assert.equal(taskButtons(ui).length,2);
 assert.match(taskButton(ui,'a').textContent,/最近记录/);
 const before=ui.requests.length;
 await act(async()=>taskButton(ui,'a').click());
 assert.equal(ui.selection().taskDir,'/repo/docs/a');
 assert.deepEqual(ui.requests.slice(before).map(r=>r.action),['status']);
 assert.match(ui.container.querySelector('header h3').textContent,/Title.*docs\/a/);
 rows=[...rows,{...item('c','desktop-session'),startedAt:'2026-11-13T10:00:00Z'}];
 await ui.refreshDiscovery();
 assert.equal(taskButtons(ui).length,3);
 assert.equal(ui.selection().taskDir,'/repo/docs/a','catalog changes cannot steal a manual view');
 await ui.click('记录');
 assert.doesNotMatch(ui.container.querySelector('[aria-label="记录"]').textContent,/docs\/b|docs\/c/);
 await ui.setVisible(false);await ui.setVisible(true);
 assert.equal(ui.selection().taskDir,'/repo/docs/a');
 assert.equal(ui.requests.every(r=>r.action==='status'),true,'navigation never launches or stops tasks');
});

test('zero or one task needs no navigator, and a failed task read never retains another task canvas', async t => {
 const ui=await mountPanel(t,r=>r.taskDir.endsWith('/a')?Promise.reject(new Error('offline')):reply(r,{taskTitle:'B title',state:stageState()}),{catalog:r=>reply(r,{runs:ownedTasks()})});
 await act(async()=>taskButton(ui,'a').click());
 assert.equal(ui.selection().taskDir,'/repo/docs/a');
 assert.equal(ui.container.querySelector('header h3').textContent,'当前关联任务');
 assert.equal(ui.container.querySelector('[data-node-id="plan"]'),null);
 assert.match(ui.container.textContent,/offline/);
 const one=await mountPanel(t,r=>reply(r,{state:stageState()}),{catalog:r=>reply(r,{runs:[item('only','desktop-session')]})});
 assert.equal(one.container.querySelector('[aria-label="本会话任务"]'),null);
});

test('navigation freezes during a read or uncertain mutation and never dispatches to the wrong task', async t => {
 const held=deferred();let hold=false;
 const ui=await mountPanel(t,r=>r.action==='start'?Promise.reject(new Error('lost receipt')):hold?held.promise:reply(r,{state:{slug:'b',status:'pending',packets:{}}}),{catalog:r=>reply(r,{runs:ownedTasks()})});
 hold=true;
 await act(async()=>{taskButton(ui,'a').click();taskButton(ui,'b').click();});
 assert.equal(ui.selection().taskDir,'/repo/docs/a');
 assert.equal(taskButton(ui,'b').disabled,true);
 await act(async()=>held.resolve(reply({taskDir:'/repo/docs/a'},{state:{slug:'a',status:'pending',packets:{}}})));
 hold=false;
 await ui.click('继续流程');
 assert.equal(taskButton(ui,'b').disabled,true);
 await act(async()=>taskButton(ui,'b').click());
 assert.equal(ui.selection().taskDir,'/repo/docs/a');
 assert.match(ui.container.textContent,/操作尚未确认/);
});

test('prepared task history is navigable and a new explicit association still follows the conversation', async t => {
 const task=(sequence,slug)=>({sequence,operationId:'tool:task-'+sequence,action:'status',taskDir:'/repo/docs/'+slug,cwd:'/repo',outcome:'confirmed',message:''});
 const a=task(1,'a'),b=task(2,'b');
 const ui=await mountPanel(t,r=>reply(r,{}),{association:{...b,tasks:[a,b]},catalog:r=>reply(r,{runs:[]})});
 assert.equal(taskButtons(ui).length,2);
 assert.match(taskButton(ui,'a').textContent,/点击读取状态/);
 await act(async()=>taskButton(ui,'a').click());
 await ui.setAssociation({...b,tasks:[a,b]});
 assert.equal(ui.selection().taskDir,'/repo/docs/a','duplicate projection cannot reset a manual choice');
 const c=task(3,'c');await ui.setAssociation({...c,tasks:[a,b,c]});
 assert.equal(ui.selection().taskDir,'/repo/docs/c');
 await ui.switchSession('foreign');
 assert.equal(taskButtons(ui).length,0);
 assert.doesNotMatch(ui.container.textContent,/docs\/a|docs\/b|docs\/c/);
});
