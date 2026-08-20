import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseFrontmatter, validatePacket, validateSpec } from '../orchestrator/core/contracts.js';
import { CONFIG_DEFAULTS } from '../orchestrator/core/config.js';

const TEMPLATES = join(dirname(fileURLToPath(import.meta.url)), '..', 'templates');

test('templates: task-packet.example.md passes packet validation', () => {
  const { data, body } = parseFrontmatter(readFileSync(join(TEMPLATES, 'task-packet.example.md'), 'utf8'));
  validatePacket(data, 'task-packet.example.md');
  assert.match(body, /Work instructions/);
});

test('templates: spec.example.md passes spec validation with consistent mapping', () => {
  const { data } = parseFrontmatter(readFileSync(join(TEMPLATES, 'spec.example.md'), 'utf8'));
  const spec = validateSpec(data, 'spec.example.md');
  assert.deepEqual([...spec._milestoneIds].sort(), ['M-01', 'M-02']);
  assert.equal(spec.requirements.length, 3);
});

test('templates: all schemas are valid JSON with required top-level shape', () => {
  for (const f of ['task-packet.schema.json', 'spec.schema.json', 'state.schema.json']) {
    const schema = JSON.parse(readFileSync(join(TEMPLATES, f), 'utf8'));
    assert.equal(schema.type, 'object', f);
    assert.ok(Array.isArray(schema.required), f);
  }
});

test('templates: flywheel.config.example.json is loadable and self-consistent', () => {
  const config = JSON.parse(readFileSync(join(TEMPLATES, 'flywheel.config.example.json'), 'utf8'));
  assert.ok(['off', 'soft', 'hard'].includes(config.budget.enforce));
  assert.equal(config.budget.enforce, 'soft'); // DESIGN default
  // Every loop/route binding references a declared channel, and fallback
  // chains only point at declared channels (§13 法定配置表 + §6 降级).
  const channels = new Set(Object.keys(config.channels));
  for (const [name, binding] of [...Object.entries(config.loops), ...Object.entries(config.routes)]) {
    if (name.startsWith('_')) continue;
    assert.ok(channels.has(binding.channel), `${name} -> ${binding.channel}`);
  }
  for (const [name, ch] of Object.entries(config.channels)) {
    if (ch.fallback) assert.ok(channels.has(ch.fallback), `${name} fallback -> ${ch.fallback}`);
  }
  // Watchdog default: 300s idle (§6)
  assert.equal(CONFIG_DEFAULTS.watchdog['idle-timeout-ms'], 300_000);
});
