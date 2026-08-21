import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { loadConfig, ConfigError } from '../orchestrator/core/config.js';

function configDir(raw) {
  const dir = mkdtempSync(join(tmpdir(), 'flywheel-cfg-'));
  writeFileSync(join(dir, 'flywheel.config.json'), JSON.stringify(raw));
  return dir;
}

const BASE = { budget: { enforce: 'soft', 'daily-token-limit': 1000 } };

test('config: valid file loads with defaults merged', () => {
  const dir = configDir(BASE);
  const cfg = loadConfig(dir, dir);
  assert.equal(cfg.budget.enforce, 'soft');
  assert.equal(cfg.watchdog['idle-timeout-ms'], 300_000); // defaults merged
});

test('config: bad enforce rejected', () => {
  const dir = configDir({ budget: { enforce: 'loud' } });
  assert.throws(() => loadConfig(dir, dir), ConfigError);
});

test('config: daily-token-limit must be a positive integer or null (P2)', () => {
  for (const bad of [-5, 0, 'lots', 12.5]) {
    const dir = configDir({ budget: { enforce: 'hard', 'daily-token-limit': bad } });
    assert.throws(() => loadConfig(dir, dir), /daily-token-limit/, String(bad));
  }
  const ok = configDir({ budget: { enforce: 'hard', 'daily-token-limit': null } });
  assert.doesNotThrow(() => loadConfig(ok, ok));
});

test('config: defect-injection.every-n-units must be an integer >= 1 (P2)', () => {
  for (const bad of [0, -1, 2.5, 'five']) {
    const dir = configDir({ ...BASE, 'defect-injection': { 'every-n-units': bad } });
    assert.throws(() => loadConfig(dir, dir), /every-n-units/, String(bad));
  }
  const ok = configDir({ ...BASE, 'defect-injection': { 'every-n-units': 1 } });
  assert.doesNotThrow(() => loadConfig(ok, ok));
});
