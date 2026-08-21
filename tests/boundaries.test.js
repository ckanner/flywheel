import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, mkdirSync, appendFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import {
  checkBoundaries, parseExpect, runGateCommands, getChangedFiles, getBaseRef, commitUnitChanges, gateEnv, getDiffText,
} from '../orchestrator/core/verify.js';

function gitRepo() {
  const dir = mkdtempSync(join(tmpdir(), 'flywheel-git-'));
  const git = (args) => execFileSync('git', ['-C', dir, '-c', 'user.name=t', '-c', 'user.email=t@l', ...args], { encoding: 'utf8' });
  git(['init', '-q']);
  writeFileSync(join(dir, 'a.txt'), 'one\n');
  git(['add', '-A']);
  git(['commit', '-q', '-m', 'initial']);
  return { dir, git };
}

test('boundaries: must-not-touch directory prefix fails the unit', () => {
  const r = checkBoundaries(['src/auth/a.ts', 'src/payments/x.ts'], { 'must-not-touch': ['src/payments/'] });
  assert.equal(r.pass, false);
  assert.deepEqual(r.violations, [{ type: 'must-not-touch', path: 'src/payments/x.ts' }]);
});

test('boundaries: exact path and glob entries work', () => {
  const exact = checkBoundaries(['secrets.env'], { 'must-not-touch': ['secrets.env'] });
  assert.equal(exact.pass, false);
  const glob = checkBoundaries(['src/generated/client.ts'], { 'must-not-touch': ['src/generated/*'] });
  assert.equal(glob.pass, false);
});

test('boundaries: test files in the diff fail the unit (§4.4 v0.10)', () => {
  for (const f of ['tests/auth.test.ts', 'src/x.spec.ts', 'src/__tests__/y.js', 'test/z.ts']) {
    const r = checkBoundaries([f], {});
    assert.equal(r.pass, false, f);
    assert.equal(r.violations[0].type, 'test-file', f);
  }
  const ok = checkBoundaries(['src/auth/token.ts', 'docs/spec.md'], {});
  assert.equal(ok.pass, true);
});

test('boundaries: max-files limit', () => {
  const r = checkBoundaries(['a.ts', 'b.ts', 'c.ts'], { 'max-files': 2 });
  assert.equal(r.pass, false);
  assert.equal(r.violations[0].type, 'max-files');
});

test('boundaries: clean diff passes', () => {
  const r = checkBoundaries(['src/auth/a.ts'], { 'must-not-touch': ['src/payments/'], 'max-files': 6 });
  assert.equal(r.pass, true);
  assert.equal(r.violations.length, 0);
});

test('boundaries: unavailable diff (not a git checkout) is recorded as skipped, not failed', () => {
  const r = checkBoundaries(null, { 'must-not-touch': ['src/'] });
  assert.equal(r.pass, true);
  assert.equal(r.skipped, true);
});

test('getChangedFiles returns null outside a git repository', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'flywheel-nogit-'));
  assert.equal(await getChangedFiles(dir), null);
});

test('getDiffText: real diff content, ledger paths excluded, null outside git', async () => {
  const { dir } = gitRepo();
  const base = await getBaseRef(dir);
  appendFileSync(join(dir, 'a.txt'), 'two\n');
  mkdirSync(join(dir, 'docs', 'x'), { recursive: true });
  writeFileSync(join(dir, 'docs', 'x', 'state.json'), '{"rev":1}\n');

  const diff = await getDiffText(dir, base, { exclude: ['docs/x/state.json'] });
  assert.match(diff, /two/);
  assert.match(diff, /a\.txt/);
  assert.ok(!diff.includes('state.json'), 'ledger path excluded from the diff');

  const plain = mkdtempSync(join(tmpdir(), 'flywheel-nogit-'));
  assert.equal(await getDiffText(plain, null), null);
});

test('getBaseRef: HEAD sha in a repo, null outside one', async () => {
  const { dir, git } = gitRepo();
  const head = git(['rev-parse', 'HEAD']).trim();
  assert.equal(await getBaseRef(dir), head);
  const plain = mkdtempSync(join(tmpdir(), 'flywheel-nogit-'));
  assert.equal(await getBaseRef(plain), null);
});

test('getChangedFiles: base-ref pins the diff across the executor\'s own commits', async () => {
  const { dir, git } = gitRepo();
  const base = await getBaseRef(dir);

  // Executor changes files and COMMITS them itself mid-unit.
  appendFileSync(join(dir, 'a.txt'), 'two\n');
  writeFileSync(join(dir, 'b.ts'), 'export {}\n');
  git(['add', '-A']);
  git(['commit', '-q', '-m', 'executor wip']);
  writeFileSync(join(dir, 'c.ts'), 'export {}\n'); // untracked

  // HEAD-diff (the old behaviour) is drained by the self-commit: vulnerability.
  assert.deepEqual(await getChangedFiles(dir), ['c.ts']);
  // Base-ref diff still sees the whole net change, committed or not.
  assert.deepEqual(await getChangedFiles(dir, base), ['a.txt', 'b.ts', 'c.ts']);
});

test('commitUnitChanges: skips cleanly outside git, commits per unit inside', async () => {
  const packet = { 'packet-id': 'M01-T01', goal: 'Make the token parser' };

  const plain = mkdtempSync(join(tmpdir(), 'flywheel-nogit-'));
  const skipped = await commitUnitChanges(plain, packet);
  assert.equal(skipped.skipped, 'not a git checkout');
  assert.equal(skipped.sha, null);

  const { dir, git } = gitRepo();
  const noChanges = await commitUnitChanges(dir, packet);
  assert.equal(noChanges.skipped, 'no changes');

  writeFileSync(join(dir, 'feature.ts'), 'export const x = 1;\n');
  const committed = await commitUnitChanges(dir, packet);
  assert.ok(committed.sha);
  assert.equal(committed.message, 'feat(M-01): M01-T01 Make the token parser');
  assert.equal(git(['rev-parse', 'HEAD']).trim(), committed.sha);
  assert.match(git(['log', '-1', '--format=%s']), /M01-T01/);
  assert.equal(git(['status', '--porcelain']).trim(), ''); // tree clean after commit
});

test('parseExpect parses "exit N"', () => {
  assert.equal(parseExpect('exit 0'), 0);
  assert.equal(parseExpect('exit 3'), 3);
  assert.throws(() => parseExpect('ok'));
});

test('runGateCommands runs every command and reports per-command results', async () => {
  const cwd = mkdtempSync(join(tmpdir(), 'flywheel-gate-'));
  writeFileSync(join(cwd, 'exists.txt'), 'x');
  const gate = await runGateCommands([
    { cmd: 'test -f exists.txt', expect: 'exit 0' },
    { cmd: 'test -f missing.txt', expect: 'exit 0' },
    { cmd: 'exit 3', expect: 'exit 3' },
  ], { cwd });
  assert.equal(gate.results.length, 3);
  assert.equal(gate.results[0].pass, true);
  assert.equal(gate.results[1].pass, false);
  assert.equal(gate.results[2].pass, true); // non-zero expectations work
  assert.equal(gate.pass, false);
});

test('runGateCommands: all green passes', async () => {
  const cwd = mkdtempSync(join(tmpdir(), 'flywheel-gate-'));
  const gate = await runGateCommands([{ cmd: 'true', expect: 'exit 0' }], { cwd });
  assert.equal(gate.pass, true);
});

test('runGateCommands: script entries run too', async () => {
  const cwd = mkdtempSync(join(tmpdir(), 'flywheel-gate-'));
  mkdirSync(join(cwd, 'e2e'), { recursive: true });
  writeFileSync(join(cwd, 'e2e', 'flow.sh'), '#!/bin/sh\nexit 0\n');
  const gate = await runGateCommands([{ script: 'sh e2e/flow.sh', expect: 'exit 0' }], { cwd });
  assert.equal(gate.pass, true);
});

test('gateEnv: NODE_TEST_CONTEXT is scrubbed so nested runners keep true exit codes', async () => {
  // node --test in inherited "child" mode exits 0 even when tests fail
  // (verified 2026-08-20) — gate commands must never see that variable.
  const env = gateEnv({ NODE_TEST_CONTEXT: 'child-v8', PATH: '/bin' });
  assert.equal(env.NODE_TEST_CONTEXT, undefined);
  assert.equal(env.PATH, '/bin');

  const cwd = mkdtempSync(join(tmpdir(), 'flywheel-gate-'));
  writeFileSync(join(cwd, 'fail.test.js'), "import { test } from 'node:test';\nimport assert from 'node:assert/strict';\ntest('red', () => assert.equal(1, 2));\n");
  writeFileSync(join(cwd, 'package.json'), '{"type":"module"}');
  const gate = await runGateCommands(
    [{ cmd: 'node --test fail.test.js', expect: 'exit 0' }],
    { cwd, env: { ...process.env, NODE_TEST_CONTEXT: 'child-v8' } },
  );
  assert.equal(gate.pass, false); // the red test must read red, never green-washed
});

test('gateEnv (P0-4): whitelisted env — API keys never reach gate commands', async () => {
  const env = gateEnv({ PATH: '/usr/bin', HOME: '/h', OPENCODE_API_KEY: 'secret', KIMI_API_KEY: 'secret2', CUSTOM: 'x' });
  assert.equal(env.PATH, '/usr/bin');
  assert.equal(env.HOME, '/h');
  assert.equal(env.OPENCODE_API_KEY, undefined);
  assert.equal(env.KIMI_API_KEY, undefined);
  assert.equal(env.CUSTOM, undefined);
  // explicit allowlist escape (config verify.env-allowlist)
  const withAllow = gateEnv({ PATH: '/bin', NPM_CONFIG_REGISTRY: 'https://priv' }, { allow: ['NPM_CONFIG_REGISTRY'] });
  assert.equal(withAllow.NPM_CONFIG_REGISTRY, 'https://priv');

  // End to end: a gate command running executor code sees no keys.
  const cwd = mkdtempSync(join(tmpdir(), 'flywheel-gate-'));
  const gate = await runGateCommands(
    [{ cmd: 'test -z "$OPENCODE_API_KEY" -a -z "$KIMI_API_KEY"', expect: 'exit 0' }],
    { cwd, env: { ...process.env, OPENCODE_API_KEY: 'probe', KIMI_API_KEY: 'probe' } },
  );
  assert.equal(gate.pass, true, 'keys leaked into gate env');
});
