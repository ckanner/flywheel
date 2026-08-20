import { test } from 'node:test';
import assert from 'node:assert/strict';
import { checkBudget, failureSignature, detectSpin, jaccard } from '../orchestrator/core/circuit-breaker.js';
import { initState } from '../orchestrator/core/state.js';

function stateWithDaily(tokens, day = '2026-08-19') {
  const s = initState('x', { milestones: [{ id: 'M-01', title: 'm' }] }, []);
  s.budget.daily[day] = { tokens, 'cost-usd': 0 };
  return s;
}

const config = (enforce, limit = 1000) => ({ budget: { enforce, 'daily-token-limit': limit } });
const day = new Date('2026-08-19T10:00:00Z');

test('budget: under limit is ok in every mode', () => {
  for (const enforce of ['off', 'soft', 'hard']) {
    assert.equal(checkBudget(stateWithDaily(500), config(enforce), day).action, 'ok');
  }
});

test('budget: off never stops, even over limit (recording stays on regardless)', () => {
  assert.equal(checkBudget(stateWithDaily(5000), config('off'), day).action, 'ok');
});

test('budget: soft warns but does not stop (default)', () => {
  const r = checkBudget(stateWithDaily(5000), config('soft'), day);
  assert.equal(r.action, 'warn');
  assert.match(r.reason, /5000 >= 1000/);
});

test('budget: hard stops at the daily cap (last gate for unattended runs)', () => {
  const r = checkBudget(stateWithDaily(5000), config('hard'), day);
  assert.equal(r.action, 'stop');
});

test('budget: no limit configured is ok', () => {
  assert.equal(checkBudget(stateWithDaily(0), { budget: { enforce: 'hard', 'daily-token-limit': null } }, day).action, 'ok');
});

test('failureSignature: mode key ignores order, file set preserved for Jaccard', () => {
  const a = failureSignature({ errorCodes: ['L1_FAILED'], failedCommands: ['b', 'a'], changedFiles: ['y', 'x'] });
  const b = failureSignature({ errorCodes: ['L1_FAILED'], failedCommands: ['a', 'b'], changedFiles: ['x', 'y'] });
  assert.equal(JSON.parse(a).key, JSON.parse(b).key);
  assert.deepEqual(JSON.parse(a).files, ['x', 'y']);
  const c = failureSignature({ errorCodes: ['L1_FAILED'], failedCommands: ['a'], changedFiles: ['x'] });
  assert.notEqual(JSON.parse(a).key, JSON.parse(c).key);
});

test('jaccard similarity', () => {
  assert.equal(jaccard([], []), 1);
  assert.equal(jaccard(['a'], ['a']), 1);
  assert.equal(jaccard(['a', 'b'], ['a', 'b']), 1);
  assert.equal(jaccard(['a', 'b', 'c'], ['a', 'b', 'd']), 0.5); // 2/4
  assert.equal(jaccard(['a'], ['b']), 0);
});

test('detectSpin (DC4): same failure mode AND file-set Jaccard >= 0.7', () => {
  const sig = (files) => failureSignature({ errorCodes: ['L1_FAILED'], failedCommands: ['pnpm test'], changedFiles: files });
  assert.equal(detectSpin(sig(['a', 'b', 'c']), sig(['a', 'b', 'c']), 2), true); // identical
  assert.equal(detectSpin(sig(['a', 'b', 'c']), sig(['a', 'b', 'c', 'd']), 2), true); // 3/4 = 0.75 >= 0.7
  assert.equal(detectSpin(sig(['a', 'b', 'c']), sig(['a', 'b', 'x']), 2), false); // 0.5 < 0.7
  assert.equal(detectSpin(sig(['a']), sig(['b']), 2), false); // disjoint
  assert.equal(detectSpin(sig(['a']), sig(['a']), 1), false); // first failure never spins
  // Different failure mode never spins even with identical files
  const other = failureSignature({ errorCodes: ['TIMEOUT'], failedCommands: [], changedFiles: ['a'] });
  assert.equal(detectSpin(sig(['a']), other, 2), false);
  assert.equal(detectSpin(null, sig(['a']), 2), false);
});
