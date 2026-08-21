import { test } from 'node:test';
import assert from 'node:assert/strict';
import { evaluateExitContract } from '../orchestrator/core/exit-contract.js';

const deliverableOk = [{ path: 'reports/M01-T01.done.json', exists: true, valid: true }];

test('exit contract: all three green = success', () => {
  const r = evaluateExitContract({
    result: { exitCode: 0, signal: null, timedOut: false, stderr: '' },
    deliverables: deliverableOk,
  });
  assert.equal(r.ok, true);
  assert.equal(r.errorCode, null);
});

test('exit contract: exit 0 but no deliverable = EMPTY_RESPONSE (retryable), not silent success', () => {
  const r = evaluateExitContract({
    result: { exitCode: 0, signal: null, timedOut: false, stderr: '' },
    deliverables: [{ path: 'reports/x.done.json', exists: false, valid: false }],
  });
  assert.equal(r.ok, false);
  assert.equal(r.processOk, true);
  assert.equal(r.deliverablesOk, false);
  assert.equal(r.errorCode, 'EMPTY_RESPONSE');
});

test('exit contract: malformed deliverable is also EMPTY_RESPONSE', () => {
  const r = evaluateExitContract({
    result: { exitCode: 0, signal: null, timedOut: false, stderr: '' },
    deliverables: [{ path: 'reports/x.done.json', exists: true, valid: false, detail: 'missing-key' }],
  });
  assert.equal(r.ok, false);
  assert.equal(r.errorCode, 'EMPTY_RESPONSE');
});

test('exit contract: timeout trapped into exit 0 is NOT green', () => {
  const r = evaluateExitContract({
    result: { exitCode: 0, signal: null, timedOut: true, stderr: '' },
    deliverables: deliverableOk,
  });
  assert.equal(r.ok, false);
  assert.equal(r.processOk, false);
  assert.equal(r.errorCode, 'TIMEOUT');
});

test('exit contract: stderr error fails even with exit 0 and deliverable', () => {
  const r = evaluateExitContract({
    result: { exitCode: 0, signal: null, timedOut: false, stderr: 'Error: 429 rate limit exceeded' },
    deliverables: deliverableOk,
  });
  assert.equal(r.ok, false);
  assert.equal(r.stderrOk, false);
  assert.equal(r.errorCode, 'RATE_LIMIT');
});

test('exit contract: non-zero exit is classified from stderr', () => {
  const r = evaluateExitContract({
    result: { exitCode: 1, signal: null, timedOut: false, stderr: '502 bad gateway' },
    deliverables: deliverableOk,
  });
  assert.equal(r.ok, false);
  assert.equal(r.errorCode, 'SERVER');
});

test('exit contract: signal kill without timeout is not success', () => {
  const r = evaluateExitContract({
    result: { exitCode: null, signal: 'SIGKILL', timedOut: false, stderr: '' },
    deliverables: deliverableOk,
  });
  assert.equal(r.ok, false);
  assert.equal(r.processOk, false);
});

test('exit contract: an EMPTY deliverable list fails (nothing checked = nothing proven)', () => {
  const r = evaluateExitContract({
    result: { exitCode: 0, signal: null, timedOut: false, stderr: '' },
    deliverables: [],
  });
  assert.equal(r.ok, false);
  assert.equal(r.deliverablesOk, false);
  assert.equal(r.errorCode, 'EMPTY_RESPONSE');
});
