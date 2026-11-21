import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, realpath, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ControlNotDispatchedError, runControl } from '../src/control.js';

const binary = fileURLToPath(new URL('./fixtures/fake-model-collaboration-control.js', import.meta.url));
function env(t, name, value) {
  const previous = process.env[name];
  if (value === undefined) delete process.env[name]; else process.env[name] = value;
  t.after(() => previous === undefined ? delete process.env[name] : process.env[name] = previous);
}
async function directory(t) {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-packet-images-')));
  t.after(() => rm(cwd, { recursive: true, force: true })); return cwd;
}
const command = value => ({ action: 'input-images', revision: 7, 'request-id': 'image-edit-1', start: true,
  'input-images': { packet: 'M01-T01', contractFingerprint: 'a'.repeat(64), plan: { revision: 3, digest: 'b'.repeat(64) }, value } });
const images = () => ({ version: 1, images: [{ path: 'references/screen.png', sha256: 'c'.repeat(64), 'media-type': 'image/png' }] });

test('image plan mutations preserve explicit null or frozen references over stdin and require the exact action and request receipt', async t => {
  const cwd = await directory(t), log = join(cwd, 'calls.jsonl');
  env(t, 'TEST_MODEL_COLLAB_LOG', log); env(t, 'TEST_MODEL_COLLAB_PATCH', undefined); env(t, 'TEST_MODEL_COLLAB_RECEIPT', undefined);
  const request = { action: 'planning', cwd, taskDir: cwd, planning: command(images()) };
  for (const value of [images(), null]) {
    const result = await runControl({ ...request, planning: command(value) }, { binary, hostSessionID: 'mine' });
    assert.equal(result['planning-control'].action, 'input-images');
    assert.equal(result['planning-control']['request-id'], 'image-edit-1');
  }
  const calls = (await readFile(log, 'utf8')).trim().split('\n').map(JSON.parse);
  assert.deepEqual(calls.map(call => call.input['input-images'].value), [images(), null]);
  assert.deepEqual(calls[0].args, ['control', 'planning', '--cwd', cwd, '--compact', '--task-dir', cwd]);
  for (const mutate of [
    value => { delete value.planning['input-images'].value; },
    value => { value.planning['input-images'].value.images = []; },
    value => { value.planning['input-images'].value.images[0].data = 'aGVsbG8='; },
    value => { value.planning['input-images'].value.images[0].path = '../secret.png'; },
    value => { value.planning['input-images'].confirmed = true; },
  ]) {
    const invalid = structuredClone(request); mutate(invalid);
    await assert.rejects(runControl(invalid, { binary }), ControlNotDispatchedError);
  }
  assert.equal((await readFile(log, 'utf8')).trim().split('\n').length, 2, 'invalid image mutations never spawn the engine');
  for (const patch of [{ action: 'collaboration' }, { action: 'approve' }, { 'request-id': 'another-edit' }]) {
    process.env.TEST_MODEL_COLLAB_PATCH = JSON.stringify(patch);
    await assert.rejects(runControl(request, { binary }), /missing its requested report/);
  }
  delete process.env.TEST_MODEL_COLLAB_PATCH;
  process.env.TEST_MODEL_COLLAB_RECEIPT = 'missing';
  await assert.rejects(runControl(request, { binary }), /missing its requested report/);
  delete process.env.TEST_MODEL_COLLAB_RECEIPT;
  process.env.TEST_MODEL_COLLAB_PATCH = JSON.stringify({ 'request-id': 'another-request' });
  await assert.rejects(runControl({ ...request, planning: { action: 'status', 'request-id': 'image-edit-1' } }, { binary }), /missing its requested report/);
});

test('node image metadata validates its exact packet, revision, plan and reference shape before crossing the host boundary', async t => {
  const cwd = await directory(t);
  env(t, 'TEST_MODEL_COLLAB_LOG', undefined); env(t, 'TEST_MODEL_COLLAB_PATCH', undefined);
  const request = { action: 'node', cwd, taskDir: cwd, packet: 'M01-T01' };
  assert.equal((await runControl(request, { binary })).node.imageInputEditor.value, null);
  for (const patch of [
    { packet: 'M01-T02' }, { revision: 8 }, { plan: null }, { value: {} }, { status: 'stale' }, { status: 'unknown' },
    { value: { version: 1, images: [] } }, { value: { ...images(), preview: 'data:image/png;base64,aGVsbG8=' } },
    { value: { version: 1, images: [{ ...images().images[0], dataUrl: 'data:image/png;base64,aGVsbG8=' }] } },
  ]) {
    process.env.TEST_MODEL_COLLAB_PATCH = JSON.stringify(patch);
    await assert.rejects(runControl(request, { binary }), /missing its requested report/);
  }
  process.env.TEST_MODEL_COLLAB_PATCH = JSON.stringify({ value: images() });
  assert.deepEqual((await runControl(request, { binary })).node.imageInputEditor.value, images());
});
