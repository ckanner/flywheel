import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, realpath, rm, writeFile, symlink, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { preparationDirectory } from '../src/task-directory.js';
import { validImageSelection, validImageInspection } from '../src/image-contract.js';
import { validateControl, runControl } from '../src/control.js';
import { validInvestigationCommand } from '../src/protocol.js';
import { act, mountPanel, closeClientHarness, deferred } from './support/client-harness.js';
import { planningState, planningTask } from './support/planning-fixture.js';

after(closeClientHarness);
const reply = (request, fields = {}) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({ protocol: 1, ok: true, running: false, taskDir: request.taskDir ?? '', cwd: request.cwd ?? '/repo', state: null, ...fields }) } } });
const preview = (paths = ['design/界面.png']) => ({ references: { version: 1, images: paths.map(path => ({ path, sha256: 'a'.repeat(64), 'media-type': 'image/png' })) }, previews: paths.map(path => ({ path, sha256: 'a'.repeat(64), mimeType: 'image/png', bytes: 80, width: 2, height: 2, dataUrl: 'data:image/png;base64,aGVsbG8=' })) });
async function input(ui, label, value) {
  const field = [...ui.container.querySelectorAll('input,select,textarea')].find(node => node.getAttribute('aria-label') === label);
  assert.ok(field, `field ${label}`);
  await act(async () => {
    const proto = field.tagName === 'SELECT' ? ui.document.defaultView.HTMLSelectElement.prototype : ui.document.defaultView.HTMLTextAreaElement.prototype;
    Object.getOwnPropertyDescriptor(proto, 'value').set.call(field, value);
    field.dispatchEvent(new ui.document.defaultView.Event(field.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true }));
  });
}
async function fixtureDirectory(t) {
  const dir = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-ui-intake-')));
  t.after(() => rm(dir, { recursive: true, force: true }));
  return dir;
}

test('new ledgers resolve physical parents without creation, only for explicit preparation', async t => {
  const dir = await fixtureDirectory(t), path = join(dir, 'docs', 'new-task');
  assert.equal(await preparationDirectory(path), path);
  await assert.rejects(stat(join(dir, 'docs')), { code: 'ENOENT' });
  await mkdir(join(dir, 'real'));
  await symlink(join(dir, 'real'), join(dir, 'alias'));
  assert.equal(await preparationDirectory(join(dir, 'alias', 'nested', 'new-task')), join(dir, 'real', 'nested', 'new-task'));
  await writeFile(join(dir, 'file'), 'not a directory');
  for (const value of ['relative', dir + '/../escape', dir + '\n', join(dir, 'file', 'new-task')]) await assert.rejects(preparationDirectory(value));
  for (const request of [{ action: 'planning', planning: { action: 'prepare', brief: {} } }, { action: 'investigate', investigation: { action: 'start', contract: {}, run: true } }]) {
    assert.equal((await validateControl({ ...request, taskDir: path, cwd: dir })).taskDir, path);
  }
  for (const request of [{ action: 'status' }, { action: 'launch' }, { action: 'planning', planning: { action: 'status' } }, { action: 'investigate', investigation: { action: 'run', revision: 1 } }]) await assert.rejects(validateControl({ ...request, taskDir: path, cwd: dir }));
  assert.equal(validInvestigationCommand({ action: 'start', contract: {}, run: true }), true);
  for (const command of [{ action: 'status', run: true }, { action: 'start', contract: {}, run: 'yes' }, { action: 'run', revision: 1, run: true }]) assert.equal(validInvestigationCommand(command), false);
});

test('image reads use explicit bounded paths, preserve stdin, and reject substituted preview evidence', async t => {
  const dir = await fixtureDirectory(t), binary = join(dir, 'image-control.mjs');
  await writeFile(binary, `#!/usr/bin/env node\nlet raw='';for await(const part of process.stdin)raw+=part;const request=JSON.parse(raw);const path=request.paths[0];const args=process.argv;process.stdout.write(JSON.stringify({protocol:1,ok:true,running:false,taskDir:'',cwd:args[args.indexOf('--cwd')+1],state:null,inputImages:{references:{version:1,images:[{path,sha256:'a'.repeat(64),'media-type':'image/png'}]},previews:[{path,mimeType:'image/png',sha256:'a'.repeat(64),bytes:80,width:2,height:2,dataUrl:'data:image/png;base64,aGVsbG8='}]}}));\n`, { mode: 0o755 });
  const value = await runControl({ action: 'images', images: { paths: ['design/界面.png'] } }, { binary, defaultCwd: dir });
  assert.equal(value.inputImages.references.images[0].path, 'design/界面.png');
  for (const selection of [{ paths: [] }, { paths: ['../private.png'] }, { paths: ['a.png', 'a.png'] }, { paths: ['/absolute.png'] }, { paths: ['.git/config'] }, { paths: ['a.png'], automatic: true }]) {
    assert.equal(validImageSelection(selection), false);
    await assert.rejects(validateControl({ action: 'images', cwd: dir, images: selection }));
  }
  await assert.rejects(validateControl({ action: 'config', cwd: dir, images: { paths: ['a.png'] } }));
  assert.equal(validImageInspection(preview(), ['design/界面.png']), true);
  for (const mutate of [v => v.references.images[0].path = 'another.png', v => v.previews[0].sha256 = 'b'.repeat(64), v => v.previews[0].dataUrl = 'https://remote.invalid/image.png', v => v.previews[0].bytes = 8388609, v => v.previews[0].width = 8193]) {
    const candidate = preview(); mutate(candidate); assert.equal(validImageInspection(candidate, ['design/界面.png']), false);
  }
});

test('task goal creates a real thin brief with Auto and waits for exact human design approval', async t => {
  const ui = await mountPanel(t, request => reply(request, { state: planningTask(planningState({ brief: request.planning.brief })) }));
  assert.equal(ui.requests.length, 0, 'opening intake never starts inference');
  await input(ui, '目标与问题现象', '修复配置保存后模型显示错误');
  await input(ui, '这次要做什么', 'repair');
  await input(ui, '约束（每行一项，可选）', '保留既有权限\n支持旧任务');
  await ui.click('开始设计与审查');
  assert.equal(ui.requests.length, 1);
  const request = ui.requests[0];
  assert.equal(request.action, 'planning');
  assert.equal(request.planning.action, 'prepare');
  assert.equal(request.planning.start, true);
  assert.match(request.taskDir, /^\/repo\/docs\/task-dsh-/);
  assert.equal(request.planning.brief.mode, 'auto');
  assert.equal(request.planning.brief.policy, 'review-first');
  assert.equal(request.planning.brief['implementation-authorized'], false);
  assert.equal(request.planning.brief.authorization, undefined);
  assert.deepEqual(request.planning.brief.constraints, ['Task intent: repair', '保留既有权限', '支持旧任务']);
  assert.equal(ui.selection().taskDir, request.taskDir);
  assert.match(ui.container.textContent, /等待批准方案/);
});

test('delegation requires a separate explicit checkbox and records the selected depth', async t => {
  const ui = await mountPanel(t, request => reply(request, { state: planningTask(planningState({ brief: request.planning.brief, policy: 'delegated' })) }));
  await input(ui, '目标与问题现象', '实现可恢复队列');
  await input(ui, '方案授权', 'delegated');
  assert.equal(ui.button('开始设计与审查').disabled, true);
  await input(ui, '实施深度', 'full');
  await act(async () => ui.container.querySelector('input[type="checkbox"]').click());
  await ui.click('开始设计与审查');
  assert.equal(ui.requests[0].planning.brief['implementation-authorized'], true);
  assert.equal(ui.requests[0].planning.brief.mode, 'full');
  assert.match(ui.requests[0].planning.brief.authorization, /User selected and confirmed/);
});

test('investigation intake grants exact local text reads and report-only execution', async t => {
  const ui = await mountPanel(t, request => reply(request, { preparation: { version: 1, revision: 1, status: 'running', reason: 'running', contract: request.investigation.contract, observations: [], decisions: [] } }));
  await input(ui, '这次要做什么', 'investigate');
  await input(ui, '目标与问题现象', '排查超时为何是零');
  await input(ui, '预期行为', '未设置时保留默认超时');
  await input(ui, '待验证的假设', '默认值被空配置覆盖');
  await input(ui, '什么证据能推翻假设', '加载器保留未设置字段的默认值');
  await input(ui, '允许读取的文件（相对会话工作目录，每行一个，最多 8 个）', '../outside.txt');
  await ui.click('创建并开始调查');
  assert.equal(ui.requests.length, 0);
  await input(ui, '允许读取的文件（相对会话工作目录，每行一个，最多 8 个）', 'src/config.go\nsettings.json');
  await ui.click('创建并开始调查');
  assert.equal(ui.requests.length, 1);
  const request = ui.requests[0];
  assert.equal(request.investigation.run, true);
  const contract = request.investigation.contract;
  assert.equal(contract.intent, 'investigate');
  assert.deepEqual(contract['implementation-scope'], []);
  assert.deepEqual(contract['read-paths'], ['src/config.go', 'settings.json']);
  assert.deepEqual(contract.actions.map(action => [action.kind, action.path]), [['read_file', 'src/config.go'], ['read_file', 'settings.json']]);
  assert.equal(contract.limits['max-actions'], 2);
  assert.match(ui.container.textContent, /有界调查/);
});

test('image selection is previewed explicitly and carries digests as unreviewed brief evidence', async t => {
  const ui = await mountPanel(t, request => request.action === 'images' ? reply(request, { inputImages: preview(request.images.paths) }) : reply(request, { state: planningTask(planningState({ brief: request.planning.brief })) }));
  await input(ui, '目标与问题现象', '按照截图修复页面');
  await input(ui, '图片路径（仓库相对路径，每行一个，最多 4 张）', 'design/界面.png');
  await ui.click('开始设计与审查');
  assert.equal(ui.requests.length, 0, 'uninspected image paths cannot be submitted as accepted inputs');
  await ui.click('检查并预览选定图片');
  assert.equal(ui.requests[0].action, 'images');
  assert.equal(ui.container.querySelectorAll('form img').length, 1);
  assert.match(ui.container.textContent, /尚未成为已审执行契约/);
  await ui.click('开始设计与审查');
  const selected = JSON.parse(ui.requests[1].planning.brief.evidence[0]);
  assert.equal(selected.status, 'unreviewed-selection');
  assert.equal(selected['planning-consumption'], 'paths-and-digests-only');
  assert.equal(selected['selected-input-images'].images[0].sha256, 'a'.repeat(64));
  assert.equal(JSON.stringify(ui.requests[1]).includes('data:image'), false, 'thumbnail bytes are not sent as original model input');
});

test('investigation deduplicates evidence and ignores fields hidden by the selected task kind', async t => {
  const ui = await mountPanel(t, request => reply(request, { preparation: { version: 1, revision: 1, status: 'running', reason: 'running', contract: request.investigation.contract, observations: [], decisions: [] } }));
  await input(ui, '约束（每行一项，可选）', Array.from({ length: 33 }, (_, i) => 'engineering constraint ' + i).join('\n'));
  await input(ui, '这次要做什么', 'investigate');
  await input(ui, '目标与问题现象', 'Find the timeout cause');
  await input(ui, '预期行为', 'Keep the default timeout');
  await input(ui, '待验证的假设', 'The loader replaces the default');
  await input(ui, '什么证据能推翻假设', 'The loader preserves the default');
  await input(ui, '允许读取的文件（相对会话工作目录，每行一个，最多 8 个）', 'src/config.go');
  await input(ui, '已有证据（每行一项，可选）', 'The log reports zero\nThe log reports zero');
  await ui.click('创建并开始调查');
  assert.equal(ui.requests.length, 1);
  assert.deepEqual(ui.requests[0].investigation.contract['known-evidence'], ['The log reports zero']);
  assert.equal(ui.requests[0].investigation.contract.constraints, undefined);
});

test('stale or failed image results and session changes cannot authorize or leak image evidence', async t => {
  const gate = deferred();
  const ui = await mountPanel(t, request => request.action === 'images' ? gate.promise : reply(request));
  await input(ui, '图片路径（仓库相对路径，每行一个，最多 4 张）', 'design/界面.png');
  await ui.click('检查并预览选定图片');
  await ui.switchSession('other-conversation');
  await act(async () => gate.resolve(reply(ui.requests[0], { inputImages: preview() })));
  assert.equal(ui.container.querySelectorAll('form img').length, 0);
  assert.equal(ui.container.querySelector('textarea[aria-label="图片路径（仓库相对路径，每行一个，最多 4 张）"]').value, '');
  assert.equal(ui.requests.length, 1);
  const failed = await mountPanel(t, request => reply(request, { inputImages: { ...preview(), previews: [] } }));
  await input(failed, '图片路径（仓库相对路径，每行一个，最多 4 张）', 'design/界面.png');
  await failed.click('检查并预览选定图片');
  assert.equal(failed.container.querySelectorAll('form img').length, 0);
  assert.match(failed.container.textContent, /图片检查未完成/);
});

test('failed creation keeps its draft and exact target through reconciliation and remount without replay', async t => {
  let planningCalls = 0;
  const ui = await mountPanel(t, request => {
    if (request.action === 'status') return reply(request);
    if (++planningCalls === 1) return { ok: false, error: { message: 'Creation response was lost after the directory was registered' } };
    return reply(request, { state: planningTask(planningState({ brief: request.planning.brief })) });
  });
  await input(ui, '目标与问题现象', '保留这个目标并在原目录恢复');
  await ui.click('开始设计与审查');
  const original = ui.requests[0];
  await ui.setAssociation({ taskDir: original.taskDir, cwd: original.cwd, action: 'planning', operationId: 'command:create', sequence: 1, outcome: 'unknown' });
  assert.equal(ui.button('开始设计与审查').disabled, true);
  await ui.setVisible(false); await ui.setVisible(true);
  assert.equal(ui.container.querySelector('textarea[aria-label="目标与问题现象"]').value, original.planning.brief.goal);
  assert.equal(planningCalls, 1);
  await ui.click('刷新控制状态');
  assert.equal(ui.button('开始设计与审查').disabled, false);
  assert.match(ui.container.textContent, /继续创建任务/);
  await ui.click('开始设计与审查');
  const repeated = ui.requests.filter(request => request.action === 'planning')[1];
  assert.equal(repeated.taskDir, original.taskDir);
  assert.deepEqual(repeated.planning.brief, original.planning.brief);
  assert.equal(planningCalls, 2);
  await ui.switchSession('fresh-conversation');
  assert.equal(ui.container.querySelector('textarea[aria-label="目标与问题现象"]').value, '');
});

test('reconciliation that confirms a created task retires its old draft and authorization', async t => {
  let original;
  const ui = await mountPanel(t, request => {
    if (request.action === 'planning' && !original) {
      original = request;
      return { ok: false, error: { message: 'The create receipt was lost' } };
    }
    return reply(request, { state: planningTask(planningState({ brief: request.planning?.brief ?? original.planning.brief })) });
  });
  await input(ui, '目标与问题现象', 'This authorized draft must retire after confirmed creation');
  await input(ui, '方案授权', 'delegated');
  await act(async () => ui.container.querySelector('input[type="checkbox"]').click());
  await ui.click('开始设计与审查');
  await ui.setAssociation({ taskDir: original.taskDir, cwd: original.cwd, action: 'planning', operationId: 'command:created', sequence: 1, outcome: 'unknown' });
  await ui.click('刷新控制状态');
  assert.equal(ui.requests.filter(request => request.action === 'planning').length, 1, 'reconciliation never replays creation');
  await ui.selectPath(0, '');
  assert.equal(ui.container.querySelector('textarea[aria-label="目标与问题现象"]').value, '');
  assert.equal(ui.container.querySelector('select[aria-label="方案授权"]').value, 'review-first');
  await input(ui, '目标与问题现象', 'A separate task needs its own review');
  await ui.click('开始设计与审查');
  const next = ui.requests.filter(request => request.action === 'planning')[1];
  assert.notEqual(next.taskDir, original.taskDir);
  assert.equal(next.planning.brief['implementation-authorized'], false);
  assert.equal(next.planning.brief.authorization, undefined);
});

for (const receipt of ['direct response', 'durable same-operation association']) {
  test(`creation recovers a canonical directory through ${receipt} without losing or duplicating its draft`, async t => {
    let original;
    const canonical = { taskDir: '/physical/ledger/recovered-task', cwd: '/physical/repo' };
    const ui = await mountPanel(t, request => {
      if (request.action === 'status') return reply(request, canonical);
      if (!original) {
        original = request;
        return { ok: false, error: { message: 'Creation response lost after resolving a directory alias' } };
      }
      return reply(request, { ...canonical, state: planningTask(planningState({ brief: request.planning.brief })) });
    });
    await input(ui, '目标与问题现象', 'Keep this draft across physical directory resolution');
    await ui.click('开始设计与审查');
    if (receipt === 'direct response') {
      await ui.setAssociation({ taskDir: original.taskDir, cwd: original.cwd, action: 'planning', operationId: 'command:alias', sequence: 1, outcome: 'unknown' });
      await ui.click('刷新控制状态');
    } else {
      // A retained host can coalesce the pending event. The confirmed record
      // must carry its own original-request identity to prove this mapping.
      await ui.setAssociation({ ...canonical, requestedTaskDir: original.taskDir, requestedCwd: original.cwd,
        action: 'planning', operationId: 'command:alias', sequence: 1, outcome: 'confirmed' });
    }
    assert.equal(ui.selection().taskDir, canonical.taskDir);
    assert.equal(ui.selection().cwd, canonical.cwd);
    assert.equal(ui.container.querySelector('textarea[aria-label="目标与问题现象"]').value, original.planning.brief.goal);
    assert.equal(ui.button('开始设计与审查').disabled, false);
    await ui.setVisible(false); await ui.setVisible(true);
    await ui.click('开始设计与审查');
    const attempts = ui.requests.filter(request => request.action === 'planning');
    assert.equal(attempts.length, 2);
    assert.equal(attempts[1].taskDir, canonical.taskDir);
    assert.equal(attempts[1].cwd, canonical.cwd);
    assert.deepEqual(attempts[1].planning.brief, original.planning.brief);
  });
}
