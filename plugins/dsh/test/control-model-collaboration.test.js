import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, realpath, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ControlNotDispatchedError, runControl, validateControl } from '../src/control.js';

const binary = fileURLToPath(new URL('./fixtures/fake-model-collaboration-control.js', import.meta.url));
const selectors = { version: 1, harness: 'antigravity', channel: 'google-native' };
const collaboration = { action: 'collaboration', revision: 7, 'request-id': 'collaboration-1', start: true,
  collaboration: { packet: 'M01-T01', contractFingerprint: 'a'.repeat(64), plan: { revision: 3, digest: 'b'.repeat(64) },
    value: { version: 1, tasks: [{ goal: 'Parser', writes: ['src/parser/'] }, { goal: 'Renderer', writes: ['src/render/'] }] } } };
function env(t, name, value) {
  const previous = process.env[name];
  if (value === undefined) delete process.env[name]; else process.env[name] = value;
  t.after(() => previous === undefined ? delete process.env[name] : process.env[name] = previous);
}
async function directory(t) {
  const path = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-model-collaboration-')));
  t.after(() => rm(path, { recursive: true, force: true })); return path;
}

test('host model inspection forwards only bounded selectors over stdin and validates catalog identity without requiring a task', async t => {
  const cwd = await directory(t), log = join(cwd, 'calls.jsonl');
  env(t, 'TEST_MODEL_COLLAB_LOG', log); env(t, 'TEST_MODEL_COLLAB_PATCH', undefined);
  const request = { action: 'models', cwd, models: selectors };
  const value = await runControl(request, { binary, hostSessionID: 'my-session' });
  assert.equal(value.taskDir, '');
  assert.equal(value.modelCatalog.models[0].id, 'gemini-pro-orchestrator-neon');
  assert.equal(value.modelCatalog.models[0].defaultEffort, '');
  assert.equal(value.modelCatalog.inferencePerformed, false);
  assert.equal(value.modelCatalog.accountEntitlement, 'unknown');
  assert.deepEqual(JSON.parse((await readFile(log, 'utf8')).trim()), { args: ['control', 'models', '--cwd', cwd, '--compact'], input: selectors });
  for (const input of [
    { ...request, models: { ...selectors, prompt: 'invoke a model' } },
    { ...request, models: { ...selectors, model: 'arbitrary-model' } },
    { ...request, models: { ...selectors, provider: '' } },
    { ...request, probe: true }, { action: 'config', cwd, models: selectors },
  ]) {
    await assert.rejects(validateControl(input));
    await assert.rejects(runControl(input, { binary }), ControlNotDispatchedError);
  }
  assert.equal((await readFile(log, 'utf8')).trim().split('\n').length, 1, 'malformed selectors never start the process');
  for (const patch of [
    { harness: 'codex-pro' }, { channel: 'foreign-account' }, { source: 'unverified-cache' },
    { inferencePerformed: true }, { accountEntitlement: 'confirmed' },
    { models: [{ id: 'bad\nmodel', label: 'bad model' }] },
    { status: 'unavailable' },
  ]) {
    process.env.TEST_MODEL_COLLAB_PATCH = JSON.stringify(patch);
    await assert.rejects(runControl(request, { binary }), /missing its requested report/);
  }
  process.env.TEST_MODEL_COLLAB_PATCH = '{}';
  const unsupported = await runControl({ ...request, models: { version: 1, harness: 'custom-harness' } }, { binary });
  assert.equal(unsupported.modelCatalog.status, 'unsupported');
  assert.deepEqual(unsupported.modelCatalog.models, []);
});

test('host collaboration mutations require the exact applied receipt and a status read needs no mutation proof', async t => {
  const cwd = await directory(t), request = { action: 'planning', cwd, taskDir: cwd, planning: collaboration };
  env(t, 'TEST_MODEL_COLLAB_PATCH', undefined); env(t, 'TEST_MODEL_COLLAB_RECEIPT', undefined);
  const result = await runControl(request, { binary, hostSessionID: 'my-session' });
  assert.equal(result['planning-control']['request-id'], collaboration['request-id']);
  assert.equal(result['planning-control'].action, 'collaboration');
  for (const patch of [{ 'request-id': 'another-edit' }, { action: 'approve' }]) {
    process.env.TEST_MODEL_COLLAB_PATCH = JSON.stringify(patch);
    await assert.rejects(runControl(request, { binary }), /missing its requested report/);
  }
  delete process.env.TEST_MODEL_COLLAB_PATCH;
  process.env.TEST_MODEL_COLLAB_RECEIPT = 'missing';
  await assert.rejects(runControl(request, { binary }), /missing its requested report/);
  delete process.env.TEST_MODEL_COLLAB_RECEIPT;
  const inspected = await runControl({ ...request, planning: { action: 'status', 'request-id': collaboration['request-id'] } }, { binary });
  assert.equal(inspected['planning-control']['request-id'], collaboration['request-id']);
});
