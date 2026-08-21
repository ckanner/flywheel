// Config loading (DESIGN.md §13): the Loop x Harness x Model binding table
// lives in flywheel.config.json — never in skills, never hardcoded.

import { readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';

export const CONFIG_DEFAULTS = {
  drivers: { plan: 'kimi', execute: 'pi', review: 'pi' },
  budget: { enforce: 'soft', 'daily-token-limit': null },
  watchdog: { 'idle-timeout-ms': 300_000, 'wall-timeout-ms': 3_600_000, 'kill-grace-ms': 5_000 },
  retry: { 'base-delay-ms': 500, 'max-delay-ms': 10_000, jitter: 0.1, 'max-transport-retries': 5 },
  'defect-injection': { 'every-n-units': 5, 'test-file-ratio-boost': 0.5 },
  verify: { 'command-timeout-ms': 300_000 },
};

export class ConfigError extends Error {}

function deepMerge(base, override) {
  if (Array.isArray(base) || Array.isArray(override) || typeof base !== 'object' || typeof override !== 'object' || base === null || override === null) {
    return override === undefined ? base : override;
  }
  const out = { ...base };
  for (const [k, v] of Object.entries(override)) {
    out[k] = k in base ? deepMerge(base[k], v) : v;
  }
  return out;
}

/**
 * Resolve flywheel.config.json: task directory first, then the working
 * directory (target repo root). Returns null when none exists.
 */
export function findConfigFile(taskDir, cwd) {
  const candidates = [join(taskDir, 'flywheel.config.json'), join(cwd, 'flywheel.config.json')];
  for (const c of candidates) {
    if (existsSync(c)) return c;
  }
  return null;
}

export function loadConfig(taskDir, cwd) {
  const file = findConfigFile(taskDir, cwd);
  if (!file) {
    throw new ConfigError(`flywheel.config.json not found (looked in ${taskDir} and ${cwd})`);
  }
  let raw;
  try {
    raw = JSON.parse(readFileSync(file, 'utf8'));
  } catch (err) {
    throw new ConfigError(`${file}: invalid JSON: ${err.message}`);
  }
  const config = deepMerge(CONFIG_DEFAULTS, raw);
  // §5 version discipline: the config is a contract layer file too (铁律 6).
  if (raw.version !== undefined && (!Number.isInteger(raw.version) || raw.version < 1)) {
    throw new ConfigError(`${file}: version must be a positive integer`);
  }
  const enforce = config.budget.enforce;
  if (!['off', 'soft', 'hard'].includes(enforce)) {
    throw new ConfigError(`${file}: budget.enforce must be off|soft|hard`);
  }
  const dailyLimit = config.budget['daily-token-limit'];
  if (dailyLimit !== null && (!Number.isInteger(dailyLimit) || dailyLimit <= 0)) {
    throw new ConfigError(`${file}: budget.daily-token-limit must be a positive integer or null`);
  }
  const everyN = config['defect-injection']?.['every-n-units'];
  if (everyN !== undefined && (!Number.isInteger(everyN) || everyN < 1)) {
    throw new ConfigError(`${file}: defect-injection.every-n-units must be an integer >= 1`);
  }
  return { ...config, _file: file };
}
