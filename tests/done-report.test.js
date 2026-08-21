import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseDoneReport } from '../orchestrator/core/done-report.js';

const FORMAT = {
  'requirements-covered': [],
  'changed-files': [],
  'commands-run': [],
  failures: [],
};

test('done-report: valid report parses', () => {
  const r = parseDoneReport(JSON.stringify({
    'requirements-covered': ['R-01'],
    'changed-files': ['src/a.ts'],
    'commands-run': ['pnpm test'],
    failures: [],
  }), FORMAT);
  assert.equal(r.ok, true);
  assert.deepEqual(r.report['requirements-covered'], ['R-01']);
});

test('done-report: malformed JSON is rejected (caller maps to retryable error)', () => {
  const r = parseDoneReport('{not json', FORMAT);
  assert.equal(r.ok, false);
  assert.deepEqual(r.errors, ['invalid-json']);
});

test('done-report: non-object JSON is rejected', () => {
  assert.equal(parseDoneReport('[1,2]', FORMAT).ok, false);
  assert.equal(parseDoneReport('"str"', FORMAT).ok, false);
  assert.equal(parseDoneReport('null', FORMAT).ok, false);
});

test('done-report: missing key is reported', () => {
  const r = parseDoneReport(JSON.stringify({
    'requirements-covered': [],
    'changed-files': [],
    // commands-run missing
    failures: [],
  }), FORMAT);
  assert.equal(r.ok, false);
  assert.ok(r.errors.some((e) => e.includes('commands-run')));
});

test('done-report: wrong type for a key is reported', () => {
  const r = parseDoneReport(JSON.stringify({
    'requirements-covered': 'R-01',
    'changed-files': [],
    'commands-run': [],
    failures: [],
  }), FORMAT);
  assert.equal(r.ok, false);
  assert.ok(r.errors.some((e) => e.includes('requirements-covered')));
});

test('done-report: non-string ELEMENTS are rejected (P0-8 — they broke spin detection sets)', () => {
  for (const bad of [
    { 'requirements-covered': [['R-01']], 'changed-files': [], 'commands-run': [], failures: [] },
    { 'requirements-covered': ['R-01'], 'changed-files': [{ path: 'a.ts' }], 'commands-run': [], failures: [] },
    { 'requirements-covered': [], 'changed-files': [], 'commands-run': [], failures: [null] },
  ]) {
    const r = parseDoneReport(JSON.stringify(bad), FORMAT);
    assert.equal(r.ok, false, JSON.stringify(bad));
    assert.ok(r.errors.some((e) => e.includes('non-string-element')), r.errors.join(','));
  }
});

test('done-report: extra keys are tolerated', () => {
  const r = parseDoneReport(JSON.stringify({
    'requirements-covered': [],
    'changed-files': [],
    'commands-run': [],
    failures: [],
    note: 'extra context is fine',
  }), FORMAT);
  assert.equal(r.ok, true);
});

test('done-report: null format falls back to default keys', () => {
  const r = parseDoneReport(JSON.stringify({
    'requirements-covered': [], 'changed-files': [], 'commands-run': [], failures: [],
  }), null);
  assert.equal(r.ok, true);
});
