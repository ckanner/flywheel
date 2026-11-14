import test from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import * as composition from 'dsh-preset-generator/composition';
import { nativeDoctor } from '../src/native-doctor.js';

const load = (overrides = {}) => async name => {
  if (Object.hasOwn(overrides, name)) {
    if (overrides[name] instanceof Error) throw overrides[name];
    return overrides[name];
  }
  return {
    'dsh-preset-generator/composition': composition,
    'dsh-preset-generator/package.json': { default: { name: 'dsh-preset-generator', version: '0.4.0' } },
    'dsh-plugin-lcu/managed': { attachManagedLcu() { assert.fail('doctor must not instantiate LCU'); }, ManagedLcuError: Error },
    'dsh-plugin-lcu/package.json': { default: { name: 'dsh-plugin-lcu', version: '0.5.0' } },
  }[name];
};

test('native doctor imports composition and managed LCU without admitting a runtime', async () => {
  const result = await nativeDoctor({ load: load() });
  assert.equal(result.available, true);
  assert.equal(result.lcu.available, true);
  assert.deepEqual(result.composition.presets, ['daily', 'heavy']);
  assert.equal(result.composition.namedPresetMounted, false);
  assert.equal(result.inferenceTested, false);
  assert.equal(result.admissionTested, false);
});

test('native doctor cannot substitute a missing, unpinned or inconsistent composition API', async () => {
  for (const overrides of [
    { 'dsh-preset-generator/composition': new Error('private-loader-detail') },
    { 'dsh-preset-generator/package.json': { default: { name: 'dsh-preset-generator', version: '0.3.0' } } },
    { 'dsh-preset-generator/composition': { ...composition, COMPOSITION_PROTOCOL: 2 } },
    { 'dsh-preset-generator/composition': { ...composition, validateSdkPtcAdmission: null } },
    { 'dsh-preset-generator/composition': { ...composition, createSdkPtcComposition: () => ({ version: 1 }) } },
  ]) {
    const result = await nativeDoctor({ load: load(overrides) });
    assert.equal(result.available, false);
    assert.equal(result.lcu.available, true);
    assert.doesNotMatch(JSON.stringify(result), /private-loader-detail/);
  }
});

test('optional LCU availability is separate from composition availability', async () => {
  for (const overrides of [
    { 'dsh-plugin-lcu/managed': new Error('not installed') },
    { 'dsh-plugin-lcu/managed': { ManagedLcuError: Error } },
    { 'dsh-plugin-lcu/package.json': { default: { name: 'dsh-plugin-lcu', version: '0.4.0' } } },
  ]) {
    const result = await nativeDoctor({ load: load(overrides) });
    assert.equal(result.available, true);
    assert.equal(result.lcu.available, false);
    assert.equal(result.admissionTested, false);
  }
});

test('installed worker native manifest exits without stdin, DSH, desktop or an SDK process', () => {
  const result = spawnSync(process.execPath, [fileURLToPath(new URL('../src/worker.js', import.meta.url)), '--native-doctor'], {
    encoding: 'utf8', timeout: 5000, env: { ...process.env, FLYWHEEL_DSH_BIN: '/missing/doctor-must-not-start-dsh' },
  });
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout.trim().split('\n').length, 1);
  const manifest = JSON.parse(result.stdout);
  assert.equal(manifest.type, 'native-doctor');
  assert.equal(manifest.nativeVersion, 1);
  assert.equal(manifest.composition.version, '0.4.0');
  assert.equal(manifest.composition.available, true);
  assert.equal(manifest.lcu.version, '0.5.0');
  assert.equal(manifest.lcu.available, true);
  assert.equal(manifest.admissionTested, false);
});
