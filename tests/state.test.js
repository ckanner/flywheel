import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import {
  loadState, saveState, initState, markInterruptedUnits, recordUsage, CasConflictError,
} from '../orchestrator/core/state.js';
import { loadSpec, loadPackets, milestoneOfPacket, ContractError } from '../orchestrator/core/contracts.js';
import { makeFixture } from './helpers.js';

test('milestoneOfPacket derives milestone from packet-id', () => {
  assert.equal(milestoneOfPacket('M02-T03'), 'M-02');
  assert.equal(milestoneOfPacket('M01-T01'), 'M-01');
  assert.throws(() => milestoneOfPacket('T01'), ContractError);
});

test('contracts: fixture spec and packets load and cross-validate', () => {
  const { taskDir } = makeFixture();
  const spec = loadSpec(taskDir);
  assert.equal(spec.milestones.length, 1);
  const packets = loadPackets(taskDir, spec);
  assert.deepEqual(packets.map((p) => p['packet-id']), ['M01-T01', 'M01-T02']);
});

test('contracts: unknown requirement reference is rejected', () => {
  const { taskDir } = makeFixture();
  const bad = readFileSync(join(taskDir, 'task-packet-M01-T01.md'), 'utf8').replace('R-01', 'R-99');
  writeFileSync(join(taskDir, 'task-packet-M01-T01.md'), bad);
  const spec = loadSpec(taskDir);
  assert.throws(() => loadPackets(taskDir, spec), /R-99/);
});

test('contracts: unknown fields, max-retries=0, string needs-live-credentials are rejected', () => {
  const { taskDir } = makeFixture();
  const spec = loadSpec(taskDir);
  const file = join(taskDir, 'task-packet-M01-T01.md');
  const original = readFileSync(file, 'utf8');
  const mutate = (fn, pattern) => {
    fn();
    assert.throws(() => loadPackets(taskDir, spec), pattern);
    writeFileSync(file, original);
  };
  mutate(() => writeFileSync(file, original.replace('route: default', 'route: default\nsurprise-field: 1')), /unknown field/);
  mutate(() => writeFileSync(file, original.replace('max-retries: 3', 'max-retries: 0')), /max-retries/);
  mutate(() => writeFileSync(file, original.replace('needs-live-credentials: false', 'needs-live-credentials: "yes"')), /boolean/);
});

test('contracts: missing boundaries or context-files is rejected (铁律 6)', () => {
  const { taskDir } = makeFixture();
  const spec = loadSpec(taskDir);
  const file = join(taskDir, 'task-packet-M01-T01.md');
  const original = readFileSync(file, 'utf8');
  writeFileSync(file, original.replace('boundaries:\n  must-not-touch: [forbidden/]\n  max-files: 5\n', ''));
  assert.throws(() => loadPackets(taskDir, spec), /boundaries/);
  writeFileSync(file, original);
  writeFileSync(file, original.replace('context-files: []\n', ''));
  assert.throws(() => loadPackets(taskDir, spec), /context-files/);
  writeFileSync(file, original);
});

test('state: save/load roundtrip increments revision atomically', () => {
  const { taskDir } = makeFixture();
  const state = loadState(taskDir);
  assert.equal(state.revision, 0);
  const saved = saveState(taskDir, state, { expectedRevision: 0 });
  assert.equal(saved.revision, 1);
  assert.equal(loadState(taskDir).revision, 1);
});

test('state: CAS refuses a stale writer', () => {
  const { taskDir } = makeFixture();
  const state = loadState(taskDir); // revision 0
  saveState(taskDir, state, { expectedRevision: 0 }); // -> revision 1
  // A second writer holding the old snapshot must lose.
  assert.throws(() => saveState(taskDir, state, { expectedRevision: 0 }), CasConflictError);
  // On-disk content is untouched by the failed write.
  assert.equal(loadState(taskDir).revision, 1);
});

test('state: external modification between load and save is detected', () => {
  const { taskDir } = makeFixture();
  const state = loadState(taskDir);
  // Someone (crash + restart, or a human) writes a newer state in between.
  const foreign = JSON.parse(readFileSync(join(taskDir, 'state.json'), 'utf8'));
  foreign.revision = 5;
  writeFileSync(join(taskDir, 'state.json'), JSON.stringify(foreign));
  assert.throws(() => saveState(taskDir, state), /CAS conflict/);
});

test('state: markInterruptedUnits flags only mid-flight units, preserving artifacts', () => {
  const state = initState('x', { milestones: [{ id: 'M-01', title: 'm' }] }, [
    { 'packet-id': 'M01-T01', route: 'default', requirements: ['R-01'] },
    { 'packet-id': 'M01-T02', route: 'default', requirements: ['R-02'] },
  ]);
  state.packets['M01-T01'].status = 'executing';
  state.packets['M01-T01'].artifacts = ['reports/M01-T01.done.json'];
  state.packets['M01-T02'].status = 'executed'; // post-spawn state is safe to resume
  const affected = markInterruptedUnits(state);
  assert.deepEqual(affected, ['M01-T01']);
  assert.equal(state.packets['M01-T01'].status, 'interrupted');
  assert.equal(state.packets['M01-T01'].interrupted, true);
  assert.deepEqual(state.packets['M01-T01'].artifacts, ['reports/M01-T01.done.json']); // preserved
  assert.equal(state.packets['M01-T02'].status, 'executed');
});

test('state: recordUsage accumulates totals, daily bucket, and per-provider cache hit rate', () => {
  const state = initState('x', { milestones: [{ id: 'M-01', title: 'm' }] }, []);
  recordUsage(state, { input: 100, output: 50, reasoning: 10, cacheRead: 40, cacheWrite: 5, totalTokens: 150 }, { provider: 'opencode-go', costUsd: 0.01, day: '2026-08-19' });
  recordUsage(state, { input: 100, output: 50, reasoning: 10, cacheRead: 60, cacheWrite: 5, totalTokens: 150 }, { provider: 'opencode-go', costUsd: 0.01, day: '2026-08-19' });
  assert.equal(state.budget.totals.tokens, 300);
  assert.equal(state.budget.totals['cost-usd'], 0.02);
  assert.equal(state.budget.daily['2026-08-19'].tokens, 300);
  const bp = state.budget.cache['by-provider']['opencode-go'];
  assert.equal(bp['cache-read'], 100);
  assert.equal(bp['hit-rate'], 0.5); // 100 cacheRead / 200 input
  assert.equal(state.budget.cache['hit-rate'], 0.5);
});

test('state: daily ledger is pruned to the last 31 days (P2)', () => {
  const state = initState('x', { milestones: [{ id: 'M-01', title: 'm' }] }, []);
  for (let i = 1; i <= 40; i++) {
    recordUsage(state, { input: 1, output: 1, totalTokens: 2 }, { day: `2026-06-${String(i).padStart(2, '0')}` });
  }
  const keys = Object.keys(state.budget.daily);
  assert.ok(keys.length <= 31, `got ${keys.length}`);
  assert.ok(keys.includes('2026-06-40')); // newest kept
  assert.ok(!keys.includes('2026-06-01')); // oldest pruned
});
