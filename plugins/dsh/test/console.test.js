import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, mountPanel, closeClientHarness } from './support/client-harness.js';
import { runControl, ControlNotDispatchedError } from '../src/control.js';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

after(closeClientHarness);
const reply = (r, fields) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({ protocol: 1, ok: true, running: false, taskDir: r.taskDir ?? '', state: null, ...fields }) } } });
const values = { loops: { review: { harness: 'dsh', model: 'gpt-6.1-sol', channel: 'chatgpt', thinking: 'high' } }, routes: { default: { harness: 'dsh', model: 'deepseek-flash', channel: 'deepseek-account', thinking: 'high' } }, sessions: { mode: 'scoped', 'policy-version': 1 }, permissions: { mode: 'full-access' } };

test('console draws real packet branches and forwards one revision-bound stage command', async t => {
  const ui = await mountPanel(t, request => reply(request, { state: { slug: 'checkout', revision: 7, status: 'stopped', packets: { 'M01-T01': { status: 'l1-failed', route: 'default', attempts: 2, evidence: { l1: { pass: false } } } } } }));
  await ui.selectPath(0, '/repo/docs/checkout');
  await ui.click('刷新状态');
  const pick = async id => act(async () => ui.container.querySelector(`[data-node-id="${id}"]`).click());
  await pick('M01-T01');
  assert.equal(ui.container.querySelector('[data-node-id="M01-T01/execute"]'), null, 'selection opens details without expanding the whole graph');
  await ui.click('展开任务画布');
  await pick('M01-T01/execute');
  assert.match(ui.container.querySelector('[aria-label="节点详情"]').textContent, /尝试次数2/);
  await ui.click('重试一次');
  assert.equal(ui.requests.length, 1, 'opening a retry dialog does not dispatch');
  assert.equal(ui.button('确认运行').disabled, true);
  const reason = [...ui.container.querySelectorAll('label')].find(label => label.textContent === '继续原因').querySelector('textarea');
  await act(async () => {
    Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLTextAreaElement.prototype, 'value').set.call(reason, '修复了环境依赖');
    reason.dispatchEvent(new ui.document.defaultView.Event('input', { bubbles: true }));
  });
  await ui.click('确认运行');
  assert.deepEqual(ui.requests.at(-1), { action: 'step', taskDir: '/repo/docs/checkout', only: 'execute', packet: 'M01-T01', revision: 7, retryReason: '修复了环境依赖' });
});

test('configuration scope is explicit, changed sections are saved and source conflicts retain the draft', async t => {
  let reject = false;
  const ui = await mountPanel(t, request => {
    if (request.action === 'config-save' && reject) throw new Error('configuration changed');
    return reply(request, { editor: { scope: request.editor.scope, target: '/global/config.json', revision: 'version-one', values, defaults: values, applied: request.action === 'config-save' } });
  });
  await ui.click('配置');
  assert.equal(ui.requests.at(-1).editor.scope, 'global');
  const area = ui.container.querySelector('textarea');
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(area, JSON.stringify({ ...values, sessions: { mode: 'fresh', 'policy-version': 1 } }));
    area.dispatchEvent(new ui.document.defaultView.Event('input', { bubbles: true }));
  });
  reject = true;
  await ui.click('保存修改');
  assert.deepEqual(ui.requests.at(-1).editor.values, { sessions: { mode: 'fresh', 'policy-version': 1 } });
  assert.equal(ui.button('保存修改').disabled, true);
  assert.match(ui.container.textContent, /保存结果未确认/);
  reject = false;
  await ui.click('重新读取配置');
  assert.match(ui.container.querySelector('dialog').textContent, /未保存的修改/);
  await ui.click('放弃草稿并继续');
  assert.doesNotMatch(ui.container.textContent, /保存结果未确认/);
  await ui.click('模型分配');
  const binding = [...ui.container.querySelectorAll('article')].find(article => article.getAttribute('data-binding-id') === 'routes.default');
  await act(async () => binding.querySelector('button').click());
  await act(async () => {
    const resource = ui.container.querySelector('dialog select');
    resource.value = 'claude-code-pro';
    resource.dispatchEvent(new ui.document.defaultView.Event('change', { bubbles: true }));
  });
  await ui.click('加入待保存修改');
  await ui.click('保存修改');
  const change = ui.requests.at(-1).editor.values;
  assert.equal(change.routes.default.harness, 'claude-code-pro');
  assert.equal(change.routes.default.model, 'sonnet');
  assert.equal(change.routes.default.provider, undefined);
  assert.deepEqual(change.channels['claude-subscription'], { 'env-key': null });
});

test('editor transport sends bounded JSON over stdin and never includes the document in argv', async t => {
  const dir = await mkdtemp(join(tmpdir(), 'flywheel-console-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const binary = join(dir, 'engine');
  await writeFile(binary, '#!/usr/bin/env node\nlet raw="";process.stdin.on("data",c=>raw+=c);process.stdin.on("end",()=>{const r=JSON.parse(raw);const args=process.argv.slice(2);console.log(JSON.stringify({protocol:1,ok:true,taskDir:"",cwd:args[args.indexOf("--cwd")+1],editor:{scope:r.scope,revision:"v1",values:{},defaults:{},target:"/config"}}));});\n', { mode: 0o755 });
  const response = await runControl({ action: 'config-edit', cwd: dir, editor: { scope: 'global' } }, { binary });
  assert.equal(response.editor.revision, 'v1');
  await assert.rejects(runControl({ action: 'step', taskDir: dir, cwd: dir, only: 'execute', revision: 0 }, { binary }), ControlNotDispatchedError);
  await assert.rejects(runControl({ action: 'status', taskDir: dir, cwd: dir, editor: {} }, { binary }), ControlNotDispatchedError);
});

test('running canvas refreshes through the read controller and stops polling when hidden', async t => {
  const { deadlineClock } = await import('./support/client-harness.js');
  const timers = deadlineClock();
  const ui = await mountPanel(t, request => reply(request, { running: true, state: { revision: 4, status: 'running', packets: {} } }), { timers });
  const previous = Object.getOwnPropertyDescriptor(ui.document, 'hidden');
  Object.defineProperty(ui.document, 'hidden', { configurable: true, value: false });
  t.after(() => { if (previous) Object.defineProperty(ui.document, 'hidden', previous); else delete ui.document.hidden; });
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  assert.equal(ui.requests.length, 1);
  assert.ok([...timers.pending.values()].some(timer => timer.delay === 2000));
  await timers.expire();
  assert.equal(ui.requests.length, 2);
  assert.equal(ui.requests.at(-1).action, 'status');
  await ui.setVisible(false);
  assert.equal(timers.pending.size, 0);
});

test('registered CLI runs open a freshly read canvas and metrics retain missing observations', async t => {
 const run = { id: 'run-one', taskDir: '/repo/docs/cli', cwd: '/repo', startedAt: '2026-11-08T10:00:00Z', conditions: 'one', summary: { slug: 'cli', status: 'paused' } };
 const ui = await mountPanel(t, request => reply(request, request.action === 'runs' ? { runs: [run] } : request.action === 'metrics' ? { metrics: { groups: [{ role: 'execute', model: 'Flash', harness: 'dsh', calls: 2, failedCalls: 1, runId: 'run-one', knownTokens: 5, knownCostUSD: 0, knownDurationMs: 25, missingDuration: 1, missingUsage: 1, missingCost: 2 }] } } : { state: { revision: 4, status: 'paused', packets: {} } }), { catalog: request => reply(request, { runs: [run] }) });
 await ui.click('运行记录'); await ui.click('刷新最近运行');
 assert.equal(ui.discoveryRequests.at(-1).action, 'runs');
 const reference = [...ui.container.querySelectorAll('label')].find(label => label.textContent.startsWith('对照运行')).querySelector('select');
 await act(async () => { reference.value = 'run-one'; reference.dispatchEvent(new ui.document.defaultView.Event('change', { bubbles: true })); });
 assert.match(ui.container.textContent, /相同契约与配置/);
 await ui.click('在画布中打开');
 assert.equal(ui.requests.at(-1).action, 'status'); assert.equal(ui.requests.at(-1).taskDir, run.taskDir);
 await ui.click('运行记录'); await ui.click('读取所选运行指标');
 assert.match(ui.container.textContent, /未报告 \/ 2/);
 assert.match(ui.container.textContent, /调用失败率不等于代码质量/);
});

test('session handoff keeps exact identity, sends prompts via stdin contract and locks automatic execution', async t => {
 let held = false;
 const key = 'execute/M01-T01';
 const session = { role: 'execute', scope: 'M01-T01', cwd: '/repo/unit', 'thread-id': 'provider-thread', 'resume-candidate': true, last: { status: 'consumed', 'cleanup-confirmed': true, outcome: 'completed' } };
 const ui = await mountPanel(t, request => {
  if (request.action === 'session' && request.session.action === 'claim') held = true;
  if (request.action === 'session' && request.session.action === 'return') held = false;
  return reply(request, request.action === 'history' ? { history: { key, text: 'reply from exact receipt' } } : { cwd: '/repo', state: { revision: 7, status: 'paused', ...(held ? { handoff: { key } } : {}), sessions: { records: { [key]: session } }, packets: { 'M01-T01': { status: 'l1-failed', route: 'default', attempts: 1 } } } });
 });
 await ui.selectPath(0, '/repo/docs/cli'); await ui.click('刷新状态');
 await act(async () => ui.container.querySelector('[data-node-id="M01-T01"]').click()); await ui.click('会话');
 assert.match(ui.container.textContent, /cd -- '\/repo\/unit' && flywheel session '\/repo\/docs\/cli'/);
 await ui.click('查看最近回复'); assert.equal(ui.requests.at(-1).sessionKey, key);
 assert.match(ui.container.textContent, /reply from exact receipt/);
 const input = [...ui.container.querySelectorAll('label')].find(label => label.textContent === '接管原因').querySelector('input');
 await act(async () => { Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLInputElement.prototype,'value').set.call(input,'修复失败');input.dispatchEvent(new ui.document.defaultView.Event('input',{bubbles:true})); });
 await ui.click('暂停并接管会话'); assert.deepEqual(ui.requests.at(-1).session,{action:'claim',key,revision:7,reason:'修复失败'});
 assert.equal(ui.button('启动').disabled,true);
 const textarea = [...ui.container.querySelectorAll('label')].find(label => label.textContent === '人工 Prompt').querySelector('textarea');
 await act(async () => { Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLTextAreaElement.prototype,'value').set.call(textarea,'修复并解释原因');textarea.dispatchEvent(new ui.document.defaultView.Event('input',{bubbles:true})); });
 await ui.click('发送到原 Session'); assert.equal(ui.requests.at(-1).session.prompt,'修复并解释原因');
 await ui.click('验证并交回流程'); assert.equal(ui.requests.at(-1).session.action,'return');
 assert.equal(ui.button('启动').disabled,false);
});

test('PR actions require an explicit reviewed target and keep merge source/base identity', async t => {
 const remote = { repo: 'github.com/example/repo', url: 'https://github.com/example/repo', localBranch: 'feature', branch: 'feature', head: 'a'.repeat(40), defaultBase: 'main', pulls: [{ number: 9, url: 'https://github.com/example/repo/pull/9', state: 'OPEN', headRefOid: 'a'.repeat(40), baseRefName: 'main', isDraft: false, mergeStateStatus: 'CLEAN', statusCheckRollup: [{ name: 'tests', conclusion: 'SUCCESS' }] }] };
 const state = { slug: 'fixture', revision: 12, status: 'completed', packets: { 'M01-T01': { status: 'completed', route: 'default' } } };
 const ui = await mountPanel(t, request => reply(request, { state, ...(request.action === 'delivery' || request.action === 'deliver' ? { delivery: remote } : {}) }));
 await ui.selectPath(0, '/repo/docs/fixture'); await ui.click('刷新状态');
 await act(async () => ui.container.querySelector('[data-node-id="delivery"]').click());
 await ui.click('查询项目与 PR');
 assert.equal(ui.button('合并此 PR').disabled, true);
 assert.match(ui.container.textContent, /tests · SUCCESS/);
 const consent = ui.container.querySelector('input[type="checkbox"]');
 await act(async () => consent.click());
 await ui.click('合并此 PR');
 assert.deepEqual(ui.requests.at(-1).delivery, { action: 'merge', confirmed: true, revision: 12, expectedHead: remote.head, expectedRepo: remote.repo, headBranch: 'feature', number: 9, base: 'main' });
});

test('full packet evidence is matched to the same refreshed state revision', async t => {
 const packet = { status: 'l1-failed', route: 'default', evidence: { l1: { pass: false, 'details-omitted': true } } };
 const ui = await mountPanel(t, request => reply(request, request.action === 'node' ? { projection: 'console-excerpt', state: { revision: 8, status: 'paused', packets: { 'M01-T01': packet } }, node: { id: 'M01-T01', revision: 8, packet: { ...packet, evidence: { l1: { pass: false, commands: [{ output: 'complete diagnostic' }] } } } } } : { projection: 'console-excerpt', state: { revision: 7, status: 'paused', packets: { 'M01-T01': packet } } }));
 await ui.selectPath(0, '/repo/docs/task'); await ui.click('刷新状态');
 await act(async () => ui.container.querySelector('[data-node-id="M01-T01"]').click()); await ui.click('读取完整节点证据');
 assert.equal(ui.requests.at(-1).action, 'node'); assert.equal(ui.requests.at(-1).packet, 'M01-T01');
 assert.match(ui.container.textContent, /complete diagnostic/);
});

test('a newer state revision invalidates an earlier PR snapshot before another remote write',async t=>{
 let revision=4;
 const remote={repo:'github.com/example/repo',url:'https://github.com/example/repo',branch:'feature',head:'a'.repeat(40),defaultBase:'main',pulls:[{number:9,url:'https://github.com/example/repo/pull/9',state:'OPEN',headRefOid:'a'.repeat(40),baseRefName:'main',isDraft:false,mergeStateStatus:'CLEAN'}]};
 const ui=await mountPanel(t,r=>reply(r,{state:{revision,status:'completed',packets:{}},...(r.action==='delivery'?{delivery:remote}:{})}));
 await ui.selectPath(0,'/repo/docs/task');await ui.click('刷新状态');
 await act(async()=>ui.container.querySelector('[data-node-id="delivery"]').click());await ui.click('查询项目与 PR');
 await act(async()=>ui.container.querySelector('input[type="checkbox"]').click());assert.equal(ui.button('合并此 PR').disabled,false);
 revision=5;await ui.click('刷新状态');assert.equal(ui.button('合并此 PR').disabled,true);assert.match(ui.container.textContent,/远端快照已过期/);
 await ui.click('查询项目与 PR');assert.equal(ui.button('合并此 PR').disabled,false);
 assert.equal(ui.requests.some(r=>r.action==='deliver'),false);
});

test('host session and delivery requests use bounded stdin, with no prompt or body in argv', async t => {
 const dir = await mkdtemp(join(tmpdir(), 'flywheel-console-stdin-'));
 t.after(() => rm(dir, { recursive: true, force: true }));
 const binary = join(dir, 'engine');
 await writeFile(binary, `#!/usr/bin/env node
let raw='';process.stdin.on('data',c=>raw+=c);process.stdin.on('end',()=>{const value=JSON.parse(raw),args=process.argv.slice(2);if(args.some(arg=>arg.includes('private prompt')))process.exit(4);console.log(JSON.stringify({protocol:1,ok:true,running:false,taskDir:args[args.indexOf('--task-dir')+1],cwd:args[args.indexOf('--cwd')+1],state:null,delivery:{repo:'github.com/example/repo',head:'a',branch:'feature',pulls:[]},received:value}));});`, { mode: 0o755 });
 const result = await runControl({ action: 'session', taskDir: dir, cwd: dir, session: { action: 'send', key: 'execute/M01-T01', revision: 3, prompt: 'private prompt' } }, { binary });
 assert.equal(result.received.prompt, 'private prompt');
 const delivery = await runControl({ action: 'deliver', taskDir: dir, cwd: dir, delivery: { action: 'create', confirmed: true, body: 'private prompt' } }, { binary });
 assert.equal(delivery.received.body, 'private prompt');
 for (const input of [{ action: 'deliver', delivery: { action: 'merge' } }, { action: 'delivery', delivery: { action: 'merge', confirmed: true } }, { action: 'node', packet: '../escape' }, { action: 'status', runId: 'fake' }]) await assert.rejects(runControl({ taskDir: dir, cwd: dir, ...input }, { binary }), ControlNotDispatchedError);
});

test('JEV starts disabled and saves an optional boolean without exposing a key field', async t => {
  const ui = await mountPanel(t, request => reply(request, { editor: { scope: 'global', target: '/global/config.json', revision: 'one', values, defaults: values } }));
  await ui.click('配置');
  await ui.click('JEV 影子建议');
  const label = [...ui.container.querySelectorAll('label')].find(label => label.textContent.startsWith('JEV 影子评估'));
  const select = ui.document.getElementById(label.htmlFor);
  assert.equal(select.value, 'false');
  assert.match(ui.container.textContent, /不改变 Quick \/ Full/);
  await act(async () => { select.value = 'true'; select.dispatchEvent(new ui.document.defaultView.Event('change', { bubbles: true })); });
  await ui.click('保存修改');
  assert.equal(ui.requests.at(-1).action, 'config-edit', 'enabling shadow sends no request before confirmation');
  await ui.click('开启并保存');
  assert.deepEqual(ui.requests.at(-1).editor.values, { 'jev-shadow': { enabled: true } });
});
