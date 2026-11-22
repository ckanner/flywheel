import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { validImageReferences, validImageInputEditor } from '../src/image-contract.js';
import { validPlanningCommand } from '../src/planning-control.js';
import { act, mountPanel, closeClientHarness, deferred } from './support/client-harness.js';
import { designRef, planningState } from './support/planning-fixture.js';

after(closeClientHarness);
const packetID = 'M01-T01', fingerprint = 'a'.repeat(64), plan = { revision: 3, digest: 'c'.repeat(64) };
const refs = (paths = ['references/original.png'], digest = 'a') => ({ version: 1, images: paths.map(path => ({ path, sha256: digest.repeat(64), 'media-type': 'image/png' })) });
const preview = (paths, digest = 'b') => ({ references: refs(paths, digest), previews: paths.map(path => ({ path, sha256: digest.repeat(64), mimeType: 'image/png',
  width: 20, height: 10, bytes: 80, dataUrl: 'data:image/png;base64,aGVsbG8=' })) });
function taskState() {
  return { revision: 7, status: 'paused', 'current-milestone': 'M01', packets: { [packetID]: { status: 'pending', route: 'multimodal', attempts: 0, schedule: { writes: ['src/'] } } },
    planning: planningState({ phase: 'ready', 'start-authorized': false, active: { version: 1, design: designRef, plans: [plan] },
      plans: [{ ref: plan, design: designRef, scope: 'packet', 'scope-id': packetID, files: { ['task-packet-' + packetID + '.md']: 'Existing plan' } }],
      'current-plan': plan, 'current-plans': { ['packet/' + packetID]: plan }, approvals: [{ kind: 'human', design: designRef, 'review-id': 'review-2', source: 'dsh:user-command' }] }) };
}
const editor = (state, patch = {}) => ({ packet: packetID, revision: state.revision, status: 'known', contractFingerprint: fingerprint, plan: { ...plan }, editable: true,
  reason: 'A new independent plan review is required.', value: refs(), ...patch });
const node = (state, patch = {}) => ({ id: packetID, revision: state.revision, packet: state.packets[packetID], imageInputEditor: editor(state, patch) });
const reply = (request, fields = {}) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({
  protocol: 1, ok: true, running: false, taskDir: request.taskDir ?? '', cwd: '/repo', state: null, ...fields,
}) } } });
const command = value => ({ action: 'input-images', revision: 7, 'request-id': 'image-revision-1', start: true,
  'input-images': { packet: packetID, contractFingerprint: fingerprint, plan: { ...plan }, value } });
const settled = (request, state, status = 'applied', patch = {}) => reply(request, { state,
  'planning-control': { action: 'input-images', 'request-id': request.planning['request-id'], status, ...patch } });
const openNode = async ui => act(async () => {
  const button = ui.container.querySelector('[data-node-id="' + packetID + '"]');
  assert.ok(button, 'packet node is visible: ' + (controller(ui).hooks.control.getSnapshot().error?.detail ?? 'no control error')); button.click();
});
async function open(ui) { await ui.selectPath(0, '/repo/docs/task'); await ui.click('刷新'); await openNode(ui); await ui.click('编辑图片输入'); }
async function path(ui, index, value) {
  const input = ui.container.querySelector('[aria-label="仓库图片路径 ' + (index + 1) + '"]');
  assert.ok(input, 'the image path is editable');
  await act(async () => {
    Object.getOwnPropertyDescriptor(ui.document.defaultView.HTMLInputElement.prototype, 'value').set.call(input, value);
    input.dispatchEvent(new ui.document.defaultView.Event('input', { bubbles: true }));
  });
}
const pathValue = ui => ui.container.querySelector('[aria-label="仓库图片路径 1"]').value;
const view = ui => ui.container.querySelector('[data-packet-image-editor]');
const controller = ui => ui.definition.inject('desktop-session', ui.bindings.get('desktop-session:0').instance.actions);
const retained = ui => [...controller(ui).packetDrafts.entries()].find(([key]) => JSON.parse(key)[3] === 'input-images')[1];

test('image revision contracts require explicit references or null and never accept preview bytes or implied deletion', () => {
  assert.equal(validPlanningCommand(command(refs())), true);
  assert.equal(validPlanningCommand(command(null)), true);
  assert.equal(validImageReferences(refs()), true);
  assert.equal(validImageInputEditor(editor(taskState())), true);
  assert.equal(validImageInputEditor(editor(taskState(), { value: null })), true);
  for (const mutate of [
    value => { delete value['input-images'].value; }, value => { value['input-images'].value = []; },
    value => { value['input-images'].value.images = []; }, value => { value['input-images'].value.version = 2; },
    value => { value['input-images'].value.images = Array(5).fill(refs().images[0]); },
    value => { value['input-images'].value.images.push(refs().images[0]); },
    value => { value['input-images'].value.images[0].path = '../secret.png'; },
    value => { value['input-images'].value.images[0].path = 'a'.repeat(513); },
    value => { value['input-images'].value.images[0].sha256 = 'unknown'; },
    value => { value['input-images'].value.images[0]['media-type'] = 'image/svg+xml'; },
    value => { value['input-images'].value.images[0].dataUrl = 'data:image/png;base64,aGVsbG8='; },
    value => { value['input-images'].value.extra = 'authority'; }, value => { value['input-images'].plan.digest = 'latest'; },
    value => { value['input-images'].packet = '../M01-T01'; }, value => { value['input-images'].approved = true; },
    value => { value.actor = 'human'; }, value => { value.start = 'true'; }, value => { value['request-id'] = ''; },
  ]) {
    const value = command(refs()); mutate(value); assert.equal(validPlanningCommand(value), false, String(mutate));
  }
  const missing = editor(taskState()); delete missing.value;
  assert.equal(validImageInputEditor(missing), false);
  assert.equal(validImageInputEditor(editor(taskState(), { value: { version: 1, images: [] } })), false);
  assert.equal(validImageInputEditor(editor(taskState(), { plan: undefined })), false);
  assert.equal(validImageInputEditor(editor(taskState(), { status: undefined })), false);
  assert.equal(validImageInputEditor(editor(taskState(), { status: 'unknown' })), false);
  assert.equal(validImageInputEditor(editor(taskState(), { status: 'unknown', editable: false, value: null, plan: undefined, contractFingerprint: undefined })), true);
  assert.equal(validImageInputEditor(editor(taskState(), { editable: false, plan: undefined, contractFingerprint: undefined, value: null })), true);
});

test('an image editor reads declared references only, then explicitly previews and submits exact added or replaced images for review', async t => {
  let state = taskState();
  const ui = await mountPanel(t, request => {
    if (request.action === 'images') return reply(request, { inputImages: preview(request.images.paths) });
    if (request.planning?.action === 'input-images') { state = { ...state, revision: 8 }; return settled(request, state); }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await open(ui);
  assert.deepEqual(ui.requests.map(request => request.action), ['status', 'node']);
  assert.equal(view(ui).querySelector('img'), null, 'reading a contract never reads original image bytes');
  assert.match(view(ui).querySelector('[data-packet-image-current]').textContent, /references\/original.png/);
  await path(ui, 0, 'references/replacement.png'); await ui.click('添加图片路径'); await path(ui, 1, 'references/detail.png');
  assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
  await ui.click('检查并预览图片草案');
  assert.equal(view(ui).querySelectorAll('img').length, 2);
  assert.match(view(ui).textContent, /image\/png.*20 × 10 · 80 bytes/s);
  assert.match(view(ui).textContent, /不会修改活动契约或授予实施授权/);
  const requests = ui.requests.length;
  await ui.click('移除图片 2');
  assert.equal(view(ui).querySelectorAll('img').length, 0, 'deleting one path invalidates the whole preview');
  assert.equal(ui.requests.length, requests, 'changing paths does not read files');
  assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
  await ui.click('检查并预览图片草案'); await ui.click('保存图片草案并重新审查');
  const save = ui.requests.at(-1);
  assert.equal(save.planning.action, 'input-images'); assert.equal(save.planning.start, true);
  assert.equal(save.planning.revision, 7); assert.match(save.planning['request-id'], /^dsh-/);
  assert.deepEqual(save.planning['input-images'], { packet: packetID, contractFingerprint: fingerprint, plan, value: refs(['references/replacement.png'], 'b') });
  assert.equal(JSON.stringify(save).includes('data:image'), false);
  assert.equal(state.planning['start-authorized'], false, 'image editing does not authorize implementation');
  assert.equal(ui.requests.some(request => ['start', 'launch', 'step'].includes(request.action) || request.planning?.action === 'approve'), false);
  assert.match(view(ui).querySelector('[data-image-edit-result="saved"]').textContent, /独立审查/);
});

test('clearing all images is a separate explicit confirmed action and an empty field can never remove the declaration', async t => {
  const state = taskState(); let saved;
  const pending = deferred();
  const ui = await mountPanel(t, request => {
    if (request.planning?.action === 'input-images') { saved = request; return pending.promise; }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await open(ui); await path(ui, 0, '');
  assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
  assert.equal(ui.button('检查并预览图片草案').disabled, true);
  assert.equal(ui.button('移除图片 1').disabled, true);
  await ui.click('明确清空全部图片声明');
  assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
  assert.match(view(ui).querySelector('[data-image-clear]').textContent, /不能用于绕过图片输入要求/);
  await act(async () => view(ui).querySelector('[data-image-clear] input').click());
  await ui.click('保存图片草案并重新审查');
  assert.equal(saved.planning['input-images'].value, null);
  assert.ok(Object.hasOwn(saved.planning['input-images'], 'value'));
  assert.match(view(ui).querySelector('[data-packet-image-current]').textContent, /references\/original.png/, 'a pending clear does not rewrite the visible plan');
  await act(async () => pending.resolve(settled(saved, state, 'rejected', { message: 'The reviewed task still requires the original screenshot.' })));
  assert.match(view(ui).querySelector('[data-image-edit-result]').textContent, /未被采用.*still requires/);
  assert.equal(retained(ui).getSnapshot().snapshot.value.images.length, 1);
});

test('unknown declarations, confirmed absence, legacy references and pending-review plans remain distinct', async t => {
  const state = taskState(); let patch = { status: 'unknown', editable: false, plan: undefined, contractFingerprint: undefined, value: null, reason: 'The packet context could not be read.' };
  const ui = await mountPanel(t, request => reply(request, { state, ...(request.action === 'node' ? { node: node(state, patch) } : {}) }));
  await open(ui);
  assert.match(view(ui).querySelector('[data-packet-image-current]').textContent, /图片声明尚未确认.*packet context could not be read/s);
  assert.doesNotMatch(view(ui).querySelector('[data-packet-image-current]').textContent, /没有声明图片输入/);
  assert.equal(view(ui).querySelector('[data-packet-image-references]'), null);
  patch = { status: 'unknown', editable: false, value: null, reason: 'The current plan packet could not be decoded.' };
  await ui.click('重新读取并替换图片草案');
  assert.match(view(ui).querySelector('[data-packet-image-current]').textContent, /尚未确认.*could not be decoded/s);
  assert.doesNotMatch(view(ui).querySelector('[data-packet-image-current]').textContent, /没有声明图片输入|references\/original.png/);
  patch = { value: null }; await ui.click('重新读取并替换图片草案');
  assert.match(view(ui).querySelector('[data-packet-image-current]').textContent, /没有声明图片输入/);
  assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
  assert.equal([...view(ui).querySelectorAll('button')].some(button => button.textContent === '明确清空全部图片声明'), false);
  patch = { value: refs(['references/new-plan.png']), editable: false, reason: 'The exact plan is awaiting independent review.' };
  await ui.click('重新读取并替换图片草案');
  assert.match(view(ui).querySelector('[data-packet-image-current]').textContent, /new-plan.png.*awaiting independent review/s);
  assert.match(view(ui).querySelector('[data-image-uneditable]').textContent, /当前阶段不可编辑/);
  assert.equal(view(ui).querySelector('[data-image-path-row]'), null);
  assert.equal(view(ui).querySelector('[data-invocation-images]'), null);
  patch = { editable: false, plan: undefined, reason: 'Reviewed planning is required.' }; await ui.click('重新读取并替换图片草案');
  assert.match(view(ui).querySelector('[data-packet-image-current]').textContent, /已读取的图片声明.*original.png/s);
  assert.doesNotMatch(view(ui).querySelector('[data-packet-image-current]').textContent, /当前计划/);
});

test('malformed image snapshots and substituted or failed previews preserve paths without enabling save or automatic retry', async t => {
  const state = taskState(); let malformed = false, imageResult = 'substituted';
  const ui = await mountPanel(t, request => {
    if (request.action === 'images') {
      if (imageResult === 'failed') return { ok: false, error: { message: 'preview unavailable' } };
      const result = preview(request.images.paths); result.references.images[0].sha256 = 'c'.repeat(64);
      return reply(request, { inputImages: result });
    }
    const value = node(state); if (malformed) delete value.imageInputEditor.value;
    return reply(request, { state, ...(request.action === 'node' ? { node: value } : {}) });
  });
  await open(ui); await path(ui, 0, 'references/my-draft.png');
  await ui.click('检查并预览图片草案');
  assert.match(view(ui).querySelector('[data-image-edit-result]').textContent, /图片检查未确认/);
  assert.equal(view(ui).querySelector('img'), null); assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
  imageResult = 'failed'; await ui.click('检查并预览图片草案');
  assert.equal(ui.requests.filter(request => request.action === 'images').length, 2);
  assert.equal(pathValue(ui), 'references/my-draft.png');
  malformed = true; await ui.click('重新读取并替换图片草案');
  assert.equal(pathValue(ui), 'references/my-draft.png');
  assert.match(view(ui).querySelector('[data-image-edit-result]').textContent, /未能读取准确的图片编辑快照/);
  assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
});

test('a durable rejection can refresh an unchanged audit revision without discarding paths, but changed plans and running tasks block editing', async t => {
  let state = taskState(), running = false, hash = fingerprint;
  const ui = await mountPanel(t, request => {
    if (request.action === 'images') return reply(request, { inputImages: preview(request.images.paths) });
    if (request.planning?.action === 'input-images') { state = { ...state, revision: state.revision + 1 }; return settled(request, state, 'rejected', { message: 'The original file changed before save.' }); }
    return reply(request, { state, running, ...(request.action === 'node' ? { node: node(state, { contractFingerprint: hash }) } : {}) });
  });
  await open(ui); await path(ui, 0, 'references/retained.png'); await ui.click('检查并预览图片草案'); await ui.click('保存图片草案并重新审查');
  assert.equal(pathValue(ui), 'references/retained.png'); assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
  await ui.click('刷新版本并保留图片草案');
  assert.equal(pathValue(ui), 'references/retained.png');
  assert.equal(ui.button('保存图片草案并重新审查').disabled, true, 'rebasing preserves text but requires a new preview');
  await ui.click('检查并预览图片草案'); assert.equal(ui.button('保存图片草案并重新审查').disabled, false);
  running = true; await ui.click('刷新'); assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
  running = false; hash = 'd'.repeat(64); await ui.click('检查执行资格'); await ui.click('刷新版本并保留图片草案');
  assert.equal(pathValue(ui), 'references/retained.png');
  assert.match(view(ui).querySelector('[data-image-edit-result]').textContent, /不能自动移到新版本/);
  assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
});

test('retained image drafts and controller snapshots contain metadata only; remounts restore paths but require a fresh preview', async t => {
  const state = taskState();
  const ui = await mountPanel(t, request => request.action === 'images' ? reply(request, { inputImages: preview(request.images.paths) })
    : reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) }));
  await open(ui); await path(ui, 0, 'references/preserved.png'); await ui.click('检查并预览图片草案');
  assert.equal(view(ui).querySelectorAll('img').length, 1);
  assert.equal(JSON.stringify(retained(ui).getSnapshot()).includes('data:image'), false);
  assert.equal(JSON.stringify(controller(ui).hooks.control.getSnapshot()).includes('data:image'), false);
  assert.equal(retained(ui).getSnapshot().references.images[0].path, 'references/preserved.png');
  await ui.setVisible(false); await ui.setVisible(true); await openNode(ui);
  assert.equal(pathValue(ui), 'references/preserved.png'); assert.equal(view(ui).querySelector('img'), null);
  assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
  assert.equal(ui.requests.filter(request => request.action === 'images').length, 1, 'restoring the editor does not reread files');
  await ui.click('检查并预览图片草案'); assert.equal(ui.button('保存图片草案并重新审查').disabled, false);
  await ui.switchSession('another-conversation'); assert.equal(ui.container.querySelector('[data-image-path-row]'), null);
  await ui.switchSession('desktop-session'); await openNode(ui);
  assert.equal(pathValue(ui), 'references/preserved.png'); assert.equal(view(ui).querySelector('img'), null);
});

test('a preview completing after unmount cannot be cached or appear in another task', async t => {
  const state = taskState(), pending = deferred(); let imageRequest;
  const ui = await mountPanel(t, request => {
    if (request.action === 'images') { imageRequest = request; return pending.promise; }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await open(ui); await path(ui, 0, 'references/private.png'); await ui.click('检查并预览图片草案'); await ui.setVisible(false);
  await act(async () => pending.resolve(reply(imageRequest, { inputImages: preview(imageRequest.images.paths) })));
  assert.equal(retained(ui).getSnapshot().pending, false);
  assert.equal(retained(ui).getSnapshot().references, null);
  assert.equal(JSON.stringify(controller(ui).hooks.control.getSnapshot()).includes('data:image'), false);
  await ui.setVisible(true); await openNode(ui);
  assert.equal(pathValue(ui), 'references/private.png'); assert.equal(view(ui).querySelector('img'), null);
  assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
  await ui.selectPath(0, '/repo/docs/other'); await ui.click('刷新'); await openNode(ui);
  assert.equal(view(ui).querySelector('[data-image-path-row]'), null);
});

test('unknown saves retain exact image request identity across unmount and reject wrong-action status receipts before recovery', async t => {
  const state = taskState(), pending = deferred(); let saved, wrong = true;
  const ui = await mountPanel(t, request => {
    if (request.action === 'images') return reply(request, { inputImages: preview(request.images.paths) });
    if (request.planning?.action === 'input-images') { saved = request; return pending.promise; }
    if (request.planning?.action === 'status') {
      assert.equal(request.planning['request-id'], saved.planning['request-id']);
      return settled(request, state, 'applied', { action: wrong ? 'collaboration' : 'input-images' });
    }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await open(ui); await path(ui, 0, 'references/unknown.png'); await ui.click('检查并预览图片草案'); await ui.click('保存图片草案并重新审查');
  assert.equal(retained(ui).getSnapshot().notice.requestID, saved.planning['request-id']);
  await ui.setVisible(false); await act(async () => pending.resolve(reply(saved, { state })));
  await ui.setVisible(true); await openNode(ui);
  assert.equal(pathValue(ui), 'references/unknown.png');
  assert.match(view(ui).querySelector('[data-image-edit-result]').textContent, /结果未知/);
  assert.equal(ui.button('重新读取并替换图片草案').disabled, true);
  await ui.click('核对图片修订结果');
  assert.equal(controller(ui).hooks.control.getSnapshot().uncertainty.planningAction, 'input-images');
  assert.equal(view(ui).querySelector('[data-image-edit-result="saved"]'), null);
  wrong = false;
  await act(async () => { await controller(ui).run('status', ui.selection()); });
  assert.equal(ui.requests.at(-1).planning.action, 'status', 'normal refresh reconciles the exact original image request');
  assert.equal(controller(ui).hooks.control.getSnapshot().uncertainty, null);
  assert.match(view(ui).querySelector('[data-image-edit-result]').textContent, /修订已保存/);
  assert.equal(ui.requests.filter(request => request.planning?.action === 'input-images').length, 1);
});

test('queued image revisions stay unresolved until their exact action and request settle', async t => {
  const state = taskState(); let saved, wrong = true, done = false;
  const ui = await mountPanel(t, request => {
    if (request.action === 'images') return reply(request, { inputImages: preview(request.images.paths) });
    if (request.planning?.action === 'input-images') { saved = request; return settled(request, state, 'queued'); }
    if (request.planning?.action === 'status') return settled(request, state, done ? 'rejected' : wrong ? 'applied' : 'queued', { action: wrong ? 'approve' : 'input-images', message: done ? 'The plan is no longer current.' : 'Waiting.' });
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await open(ui); await ui.click('检查并预览图片草案'); await ui.click('保存图片草案并重新审查');
  assert.match(view(ui).querySelector('[data-image-edit-result]').textContent, /已排队/);
  assert.equal(ui.button('保存图片草案并重新审查').disabled, true);
  await ui.click('核对图片修订结果'); assert.equal(ui.requests.at(-1).planning['request-id'], saved.planning['request-id']);
  assert.equal(controller(ui).hooks.control.getSnapshot().planningReceipt.status, 'queued', 'a wrong action cannot replace the retained queued receipt');
  assert.equal(view(ui).querySelector('[data-image-edit-result="saved"]'), null);
  wrong = false; await ui.click('核对图片修订结果');
  done = true; await ui.click('核对图片修订结果');
  assert.match(view(ui).querySelector('[data-image-edit-result]').textContent, /未被采用.*no longer current/);
});

test('known pre-dispatch failure settles the hidden image draft even after an unrelated read clears the controller error', async t => {
  const state = taskState(), pending = deferred(); let saved;
  const ui = await mountPanel(t, request => {
    if (request.action === 'images') return reply(request, { inputImages: preview(request.images.paths) });
    if (request.planning?.action === 'input-images') { saved = request; return pending.promise; }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await open(ui); await path(ui, 0, 'references/recoverable.png'); await ui.click('检查并预览图片草案'); await ui.click('保存图片草案并重新审查');
  await ui.setVisible(false);
  await act(async () => pending.resolve({ ok: true, value: { result: { kind: 'error', text: JSON.stringify({ protocol: 1, type: 'flywheel-control-error',
    action: 'planning', dispatch: 'not-dispatched', message: 'Host validation rejected this image request before dispatch.' }) } } }));
  assert.equal(retained(ui).getSnapshot().notice.kind, 'rejected');
  await act(async () => { await controller(ui).run('status', ui.selection()); });
  assert.equal(controller(ui).hooks.control.getSnapshot().error, null);
  await ui.setVisible(true); await openNode(ui);
  assert.equal(pathValue(ui), 'references/recoverable.png');
  assert.match(view(ui).querySelector('[data-image-edit-result]').textContent, /Host validation rejected/);
  assert.equal(ui.button('重新读取并替换图片草案').disabled, false);
  await path(ui, 0, 'references/corrected.png'); await ui.click('检查并预览图片草案');
  assert.equal(ui.button('保存图片草案并重新审查').disabled, false);
  assert.equal(ui.requests.filter(request => request.planning?.action === 'status').length, 0);
  assert.equal(saved.planning['input-images'].value.images[0].path, 'references/recoverable.png');
});

test('an applied image save can settle its retained draft after the inspector has unmounted', async t => {
  const state = taskState(), pending = deferred(); let saved;
  const ui = await mountPanel(t, request => {
    if (request.action === 'images') return reply(request, { inputImages: preview(request.images.paths) });
    if (request.planning?.action === 'input-images') { saved = request; return pending.promise; }
    return reply(request, { state, ...(request.action === 'node' ? { node: node(state) } : {}) });
  });
  await open(ui); await ui.click('检查并预览图片草案'); await ui.click('保存图片草案并重新审查'); await ui.setVisible(false);
  await act(async () => pending.resolve(settled(saved, { ...state, revision: 8 })));
  assert.equal(retained(ui).getSnapshot().notice.kind, 'saved');
  assert.equal(retained(ui).getSnapshot().references, null);
  await ui.setVisible(true); await openNode(ui);
  assert.equal(view(ui).querySelector('[data-image-path-row]'), null);
  assert.match(view(ui).querySelector('[data-image-edit-result]').textContent, /修订已保存/);
});
