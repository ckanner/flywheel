import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, readFile, readlink, symlink, unlink, rm, chmod, lstat, realpath } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { createTowerCandidate, freezeTowerCandidate, transferTowerCandidate, towerGit, towerSnapshot, towerPrompt } from '../src/kimi-tower-candidate.js';

const directories = [];
afterEach(async () => { await Promise.all(directories.splice(0).map(path => rm(path, { recursive: true, force: true }))); });

async function fixture() {
  // Production admission resolves cwd/taskDir before using the trusted helpers.
  // macOS tmpdir may retain /var while Git reports the physical /private/var.
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'tower-source-test-')));
  directories.push(cwd);
  const taskDir = join(cwd, 'docs', 'task');
  await mkdir(taskDir, { recursive: true });
  await towerGit(cwd, ['init', '--quiet', '-b', 'main']);
  await writeFile(join(cwd, 'source.txt'), 'committed\n');
  await writeFile(join(cwd, 'remove.txt'), 'remove me\n');
  await writeFile(join(cwd, 'binary.dat'), Buffer.from([0, 1, 255, 23]));
  await writeFile(join(taskDir, 'spec.md'), '# frozen contract\n');
  await symlink('source.txt', join(cwd, 'link'));
  await towerGit(cwd, ['add', '.']);
  await towerGit(cwd, ['commit', '--quiet', '-m', 'fixture']);
  await writeFile(join(cwd, 'source.txt'), 'preexisting dirty work\n');
  await writeFile(join(cwd, 'untracked.txt'), 'preexisting untracked work\n');
  return { type: 'start', requestId: 'tower-test', cwd, native: { version: 1, mode: 'tower' }, managed: { role: 'execute', taskDir } };
}

async function candidateOf(input) {
  const candidate = await createTowerCandidate(input);
  directories.push(dirname(candidate.path));
  return candidate;
}

test('Tower uses an independent Git repo and roundtrips dirty, untracked, binary, symlink, mode and deletion changes', async () => {
  const input = await fixture();
  const originalHead = (await towerGit(input.cwd, ['rev-parse', 'HEAD'])).toString();
  const candidate = await candidateOf(input);
  assert.ok((await lstat(join(candidate.path, '.git'))).isDirectory());
  assert.equal((await towerGit(candidate.path, ['remote'])).toString(), '');
  assert.equal(await readFile(join(candidate.path, 'source.txt'), 'utf8'), 'preexisting dirty work\n');
  assert.equal(await readFile(join(candidate.path, 'untracked.txt'), 'utf8'), 'preexisting untracked work\n');
  await writeFile(join(candidate.path, 'source.txt'), 'candidate change\n');
  await writeFile(join(candidate.path, 'untracked.txt'), 'updated untracked\n');
  await writeFile(join(candidate.path, 'binary.dat'), Buffer.from([255, 0, 252, 10, 55]));
  await unlink(join(candidate.path, 'remove.txt'));
  await unlink(join(candidate.path, 'link'));
  await symlink('untracked.txt', join(candidate.path, 'link'));
  await writeFile(join(candidate.path, 'run.sh'), '#!/bin/sh\nexit 0\n');
  await chmod(join(candidate.path, 'run.sh'), 0o755);
  await mkdir(join(candidate.path, '.tower'));
  await writeFile(join(candidate.path, '.tower', 'internal.json'), '{}');
  const frozen = await freezeTowerCandidate(candidate, input);
  const result = await transferTowerCandidate(frozen, input, true);
  assert.equal(result.applied, true);
  assert.equal(await readFile(join(input.cwd, 'source.txt'), 'utf8'), 'candidate change\n');
  assert.deepEqual(await readFile(join(input.cwd, 'binary.dat')), Buffer.from([255, 0, 252, 10, 55]));
  assert.equal(await readlink(join(input.cwd, 'link')), 'untracked.txt');
  assert.ok((await lstat(join(input.cwd, 'run.sh'))).mode & 0o111);
  await assert.rejects(lstat(join(input.cwd, 'remove.txt')), { code: 'ENOENT' });
  await assert.rejects(lstat(join(input.cwd, '.tower')), { code: 'ENOENT' });
  assert.equal((await towerGit(input.cwd, ['rev-parse', 'HEAD'])).toString(), originalHead, 'the source HEAD is never committed or moved');
  assert.match(await readFile(frozen.patchPath, 'utf8'), /GIT binary patch/);
});

test('only the actual task runtime files are excluded; contract or source changes block transfer and retain the candidate', async () => {
  const input = await fixture();
  await writeFile(join(input.managed.taskDir, 'state.json'), '{"revision":1}');
  const candidate = await candidateOf(input);
  await writeFile(join(candidate.path, 'source.txt'), 'candidate\n');
  const frozen = await freezeTowerCandidate(candidate, input);
  await mkdir(join(input.managed.taskDir, '.sessions'));
  await writeFile(join(input.managed.taskDir, '.sessions', 'receipt.json'), '{}');
  await writeFile(join(input.managed.taskDir, 'state.json'), '{"revision":2}');
  assert.equal((await towerSnapshot(input.cwd, input.managed.taskDir)).digest, candidate.sourceSnapshotDigest);
  await writeFile(join(input.managed.taskDir, 'spec.md'), '# changed contract\n');
  await assert.rejects(transferTowerCandidate(frozen, input, true), /source or contract changed/);
  assert.equal(await readFile(join(input.cwd, 'source.txt'), 'utf8'), 'preexisting dirty work\n');
  assert.ok((await lstat(frozen.patchPath)).size > 0);
  assert.equal(await readFile(join(candidate.path, 'source.txt'), 'utf8'), 'candidate\n');
});

test('unknown cleanup and non-execute roles never import a Tower patch', async () => {
  const input = await fixture();
  const candidate = await candidateOf(input);
  await writeFile(join(candidate.path, 'source.txt'), 'candidate\n');
  const frozen = await freezeTowerCandidate(candidate, input);
  await assert.rejects(transferTowerCandidate(frozen, input, false), /confirmed runtime cleanup/);
  assert.deepEqual(await transferTowerCandidate(frozen, { ...input, managed: { ...input.managed, role: 'plan' } }, true), {
    applied: false, reason: 'role-does-not-own-workspace-transfer',
  });
  assert.equal(await readFile(join(input.cwd, 'source.txt'), 'utf8'), 'preexisting dirty work\n');
});

test('a moved source HEAD and a changed frozen patch are rejected without rollback or artifact deletion', async () => {
  const input = await fixture();
  const candidate = await candidateOf(input);
  await writeFile(join(candidate.path, 'source.txt'), 'candidate\n');
  const frozen = await freezeTowerCandidate(candidate, input);
  await writeFile(frozen.patchPath, 'tampered');
  await assert.rejects(transferTowerCandidate(frozen, input, true), /patch changed/);
  await towerGit(input.cwd, ['commit', '--quiet', '--allow-empty', '-m', 'another writer moved HEAD']);
  await assert.rejects(transferTowerCandidate(frozen, input, true), /source or contract changed/);
  assert.equal(await readFile(frozen.patchPath, 'utf8'), 'tampered');
});

test('source snapshots refuse non-root execution and tracked paths that traverse a symlink directory', async () => {
  const input = await fixture();
  await mkdir(join(input.cwd, 'nested'));
  await assert.rejects(towerSnapshot(join(input.cwd, 'nested'), input.managed.taskDir), /repository root/);
  await writeFile(join(input.cwd, 'nested', 'file'), 'tracked');
  await towerGit(input.cwd, ['add', 'nested/file']);
  await towerGit(input.cwd, ['commit', '--quiet', '-m', 'nested']);
  await rm(join(input.cwd, 'nested'), { recursive: true });
  await symlink(input.managed.taskDir, join(input.cwd, 'nested'));
  await assert.rejects(towerSnapshot(input.cwd, input.managed.taskDir), /symlink directory/);
});

test('linked unit worktrees retain independent Git state and bind the external task contract', async () => {
  const input = await fixture();
  const unit = await realpath(await mkdtemp(join(tmpdir(), 'tower-unit-test-')));
  directories.push(unit);
  await towerGit(input.cwd, ['worktree', 'add', '--quiet', '--detach', unit, 'HEAD']);
  const unitInput = { ...input, cwd: unit };
  const candidate = await candidateOf(unitInput);
  assert.ok((await lstat(join(unit, '.git'))).isFile(), 'source is a linked worktree');
  assert.ok((await lstat(join(candidate.path, '.git'))).isDirectory(), 'Tower has private Git administration');
  assert.ok(candidate.source.externalContract.digest);
  await writeFile(join(candidate.path, 'source.txt'), 'unit candidate\n');
  const frozen = await freezeTowerCandidate(candidate, unitInput);
  await writeFile(join(input.managed.taskDir, 'state.json.tmp-99'), '{"revision":2}');
  await mkdir(join(input.managed.taskDir, '.sessions'));
  await writeFile(join(input.managed.taskDir, '.sessions', 'receipt.json'), '{}');
  assert.equal((await transferTowerCandidate(frozen, unitInput, true)).applied, true);
  assert.equal(await readFile(join(unit, 'source.txt'), 'utf8'), 'unit candidate\n');
  assert.equal(await readFile(join(input.cwd, 'source.txt'), 'utf8'), 'preexisting dirty work\n');

  const next = await candidateOf(unitInput);
  await writeFile(join(next.path, 'source.txt'), 'second candidate\n');
  const nextFrozen = await freezeTowerCandidate(next, unitInput);
  await writeFile(join(input.managed.taskDir, 'spec.md'), '# concurrent external contract edit\n');
  await assert.rejects(transferTowerCandidate(nextFrozen, unitInput, true), /source or contract changed/);
  assert.equal(await readFile(join(unit, 'source.txt'), 'utf8'), 'unit candidate\n');
  assert.ok((await lstat(nextFrozen.patchPath)).size > 0);
});

test('Tower prompt mapping changes bounded source path references and records both prompt digests', () => {
  const input = { cwd: '/source/repo', prompt: 'Edit /source/repo/src/a.js and `/source/repo`. cwd="/source/repo"; alias=/source/repo/link/file; nested=/source/repo/link-other/file; keep /source/repo-other and https://example.com/source/repo intact. Query https://example.com/?cwd=/source/repo/link/file&next=/source/repo/next. Read /external/task/spec.md.' };
  for (const alias of ['/source/repo/link', '/source/repo/link/', '/source/repo/link/.']) {
    const result = towerPrompt(input, { path: '/source/repo/candidate' }, alias);
    assert.equal(result.mapping.replacements, 5);
    assert.match(result.prompt, /Edit \/source\/repo\/candidate\/src\/a.js/);
    assert.match(result.prompt, /`\/source\/repo\/candidate`/);
    assert.match(result.prompt, /alias=\/source\/repo\/candidate\/file/);
    assert.match(result.prompt, /nested=\/source\/repo\/candidate\/link-other\/file/);
    assert.doesNotMatch(result.prompt, /candidate\/candidate/);
    assert.match(result.prompt, /\/source\/repo-other/);
    assert.match(result.prompt, /https:\/\/example.com\/source\/repo/);
    assert.ok(result.prompt.includes('https://example.com/?cwd=/source/repo/link/file&next=/source/repo/next'));
    assert.match(result.prompt, /Read \/external\/task\/spec.md/);
    assert.equal(result.mapping.declaredSourceCwd, alias);
    assert.equal(result.mapping.filesystemSandbox, false);
    assert.notEqual(result.mapping.originalPromptDigest, result.mapping.effectivePromptDigest);
  }
});
