import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, symlinkSync, chmodSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createHash } from 'node:crypto';
import { resolveFlywheelBinary } from '../src/installation.js';

function fixture(t) {
  const home = mkdtempSync(join(tmpdir(), 'flywheel gui $() '));
  t.after(() => rmSync(home, { recursive: true, force: true }));
  const dir = join(home, '.config', 'flywheel');
  mkdirSync(dir, { recursive: true });
  const launcher = join(dir, 'npm-launcher');
  const filename = join(dir, 'installation.json');
  const content = '#!/bin/sh\nexit 0\n';
  writeFileSync(launcher, content, { mode: 0o755 });
  const receipt = { protocol: 1, owner: '@ckanner/flywheel', launcher: {
    path: launcher, sha256: createHash('sha256').update(content).digest('hex'),
  } };
  const save = value => writeFileSync(filename, JSON.stringify(value));
  save(receipt);
  return { home, launcher, filename, receipt, save };
}

test('desktop resolves a verified npm launcher without a shell PATH', t => {
  const f = fixture(t);
  assert.equal(resolveFlywheelBinary({}, { HOME: f.home, PATH: '/usr/bin' }), f.launcher);
  assert.equal(resolveFlywheelBinary({ flywheelBinary: '/custom/flywheel' }, { HOME: f.home }), '/custom/flywheel');
  assert.throws(() => resolveFlywheelBinary({ flywheelBinary: '' }, { HOME: f.home }), /nonempty/);
  rmSync(f.filename);
  assert.equal(resolveFlywheelBinary({}, { HOME: f.home }), 'flywheel');
});

test('invalid or stale records do not silently select another executable', t => {
  const f = fixture(t);
  writeFileSync(f.filename, '{');
  assert.throws(() => resolveFlywheelBinary({}, { HOME: f.home }), /record is invalid/);
  f.save({ ...f.receipt, owner: 'someone-else' });
  assert.throws(() => resolveFlywheelBinary({}, { HOME: f.home }), /unsupported contract/);
  f.save(f.receipt);
  writeFileSync(f.launcher, '#!/bin/sh\necho user-change\n');
  assert.throws(() => resolveFlywheelBinary({}, { HOME: f.home }), /missing or changed/);
  rmSync(f.launcher);
  assert.throws(() => resolveFlywheelBinary({}, { HOME: f.home }), /missing or changed/);
});

test('receipt and launcher links or non-executable files are rejected', t => {
  const f = fixture(t);
  chmodSync(f.launcher, 0o644);
  assert.throws(() => resolveFlywheelBinary({}, { HOME: f.home }), /missing or changed/);
  rmSync(f.launcher);
  symlinkSync(f.filename, f.launcher);
  assert.throws(() => resolveFlywheelBinary({}, { HOME: f.home }), /missing or changed/);
  rmSync(f.filename);
  symlinkSync(f.launcher, f.filename);
  assert.throws(() => resolveFlywheelBinary({}, { HOME: f.home }), /record is invalid/);
});
