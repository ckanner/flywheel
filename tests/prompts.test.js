import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { buildExecutePrompt, parseReviewVerdict } from '../orchestrator/core/prompts.js';

const PACKET = {
  'packet-id': 'M01-T01',
  goal: 'Make the artifact.',
  requirements: ['R-01'],
  route: 'default',
  'cache-scope': [],
  boundaries: { 'must-not-touch': ['forbidden/'], 'max-files': 5 },
  acceptance: { L1: [{ cmd: 'true', expect: 'exit 0' }] },
};
const SPEC = { goal: 'Prove the loop.', 'non-goals': ['x'] };

test('execute prompt: DC1 discipline — implement only, tests come from the contract', () => {
  const prompt = buildExecutePrompt({ packet: PACKET, spec: SPEC, taskDir: '/t', cwd: null, attempt: 1 });
  assert.match(prompt, /never write or modify tests/i);
  assert.match(prompt, /provided by the contract|already exist/i);
  assert.ok(!/\bTDD\b/.test(prompt), 'TDD instruction is gone (v0.14 DC1)');
});

test('cache-scope files land in the stable prefix before the volatile section (§8.1)', () => {
  const dir = mkdtempSync(join(tmpdir(), 'flywheel-cs-'));
  mkdirSync(join(dir, 'docs'), { recursive: true });
  writeFileSync(join(dir, 'docs', 'spec.md'), 'STABLE-CONTENT-MARKER\n');
  const packet = { ...PACKET, 'cache-scope': ['docs/spec.md', 'docs/missing.md'] };
  const prompt = buildExecutePrompt({ packet, spec: SPEC, taskDir: '/t', cwd: dir, attempt: 3 });
  assert.match(prompt, /Cache-scope files/);
  assert.match(prompt, /STABLE-CONTENT-MARKER/);
  assert.match(prompt, /docs\/missing\.md[\s\S]*not found/);
  assert.ok(prompt.indexOf('STABLE-CONTENT-MARKER') < prompt.indexOf('## Runtime'), 'stable content precedes volatile runtime section');
  assert.ok(prompt.indexOf('attempt: 3') > prompt.indexOf('## Runtime'), 'attempt number stays volatile-last (§8.2)');
});

test('cache-scope over the 50KB cap is truncated with a marker', () => {
  const dir = mkdtempSync(join(tmpdir(), 'flywheel-cs-'));
  writeFileSync(join(dir, 'big.txt'), 'Y'.repeat(60_000));
  const packet = { ...PACKET, 'cache-scope': ['big.txt'] };
  const prompt = buildExecutePrompt({ packet, spec: SPEC, taskDir: '/t', cwd: dir, attempt: 1 });
  assert.match(prompt, /cache-scope truncated/);
  assert.ok(prompt.length < 60_000 + 5_000);
});

test('parseReviewVerdict: fenced JSON and garbage tolerance', () => {
  assert.equal(parseReviewVerdict('blah\n```json\n{"verdict":"pass","score":0.9,"reasons":[]}\n```').verdict.verdict, 'pass');
  assert.equal(parseReviewVerdict('no json here').ok, false);
  assert.equal(parseReviewVerdict('').ok, false);
  assert.equal(parseReviewVerdict('{"verdict":"banana"}').ok, false);
  assert.equal(parseReviewVerdict('text\n{"verdict":"verification-gap","reasons":["weak"]}').verdict.verdict, 'verification-gap');
});
