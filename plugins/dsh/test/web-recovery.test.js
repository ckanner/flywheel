import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, mkdir, mkdtemp, readFile, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { nativeDigest } from '../src/native.js';
import { recoverWebWorker, WEB_RECOVERY_VERSION } from '../src/web-recovery.js';

const sdk = fileURLToPath(new URL('./fixtures/persisted-sdk.js', import.meta.url));
const owner = fileURLToPath(new URL('./fixtures/web-recovery-owner.js', import.meta.url));
await chmod(sdk, 0o755);
async function fixture(t, scenario = 'ok') {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-web-recovery-')));
  t.after(() => rm(root, { recursive: true, force: true }));
  await mkdir(join(root, '.sessions'), { mode: 0o700 });
  const requestId = 'original-web-invocation';
  const recoveryDirectory = join(root, '.sessions', 'recovery-' + createHash('sha256').update(requestId).digest('hex'));
  const request = { type: 'start', protocol: 2, persistent: true, requestId, cwd: root,
    provider: 'offline-fixture', model: 'gpt-6-pro-web', reasoningEffort: 'pro', permissionMode: 'danger-full-access',
    prompt: 'Original SDK operation', timeoutMs: 15000, idleTimeoutMs: 20,
    web: { version: 1, executionDomainId: 'task-domain', requireIsolation: true, recoveryDirectory } };
  const env = { PATH: process.env.PATH, FLYWHEEL_DSH_BIN: sdk, TEST_DSH_WEB_SCENARIO: scenario,
    TEST_DSH_HISTORY: join(root, 'history'), TEST_DSH_LLM_CALLS: join(root, 'calls.jsonl') };
  const output = await new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [owner], { stdio: ['pipe','pipe','pipe'] });
    let stdout = '', stderr = '';
    child.stdout.on('data', data => { stdout += data; });
    child.stderr.on('data', data => { stderr += data; });
    child.on('error', reject);
    child.on('exit', code => code === 0 ? resolve(JSON.parse(stdout)) : reject(new Error(stderr)));
    child.stdin.end(JSON.stringify({ request, env }));
  });
  assert.equal(output.result.status, scenario === 'ok' ? 'ok' : 'error', JSON.stringify(output));
  const path = join(recoveryDirectory, 'worker.json');
  const saved = await readFile(path, 'utf8'), journal = JSON.parse(saved);
  assert.equal(journal.phase, 'settled');
  assert.equal(journal.business.completed, scenario === 'ok');
  assert.equal(journal.business.sdkCleanupConfirmed, true);
  const { nativeDigest: _digest, requestId: _id, ...original } = journal.original;
  const query = { type: 'web-recover', protocol: 1, requestId, recoveryDirectory, original: { ...original, native: null }, expectedBinding: journal.ready.web };
  const evidence = { version: 1, contract: WEB_RECOVERY_VERSION, identity: journal.checkpoint.identity,
    checkpointDigest: nativeDigest(journal.checkpoint), revision: 'a'.repeat(64), disposition: 'committed', binding: journal.ready.web,
    facts: { version: 1, ...journal.checkpoint.identity, dispatch: 'dispatched', outcome: 'completed', remoteGeneration: 'idle', localCleanupConfirmed: true, usageStatus: 'unknown' },
    terminalMain: [{ text: journal.business.text, hostCommitId: 'original-host-commit' }] };
  const inspect = async checkpoint => { assert.deepEqual(checkpoint, journal.checkpoint); return structuredClone(evidence); };
  return { root, path, saved, journal, query, env, evidence, inspect, save: () => writeFile(path, JSON.stringify(journal), { mode: 0o600 }) };
}

test('reads a complete original SDK terminal after process exit without another model call or any evidence writes', async t => {
  const f = await fixture(t);
  const calls = await readFile(join(f.root, 'calls.jsonl'), 'utf8');
  const recovered = await recoverWebWorker(f.query, f);
  assert.equal(recovered.frames[1].status, 'ok');
  assert.equal(recovered.frames[1].text, f.journal.business.text);
  assert.equal(recovered.frames[1].lifecycle.outcome, 'completed');
  assert.equal(recovered.frames[1].lifecycle.usageStatus, 'unknown');
  assert.equal(recovered.frames[1].usage, null);
  assert.equal(await readFile(f.path, 'utf8'), f.saved);
  assert.equal(await readFile(join(f.root, 'calls.jsonl'), 'utf8'), calls);
  assert.deepEqual(await recoverWebWorker(f.query, f), recovered);
  await assert.rejects(recoverWebWorker(f.query, { ...f, env: { ...f.env, FLYWHEEL_DSH_PROFILE: 'different' } }), /profile/);
  await assert.rejects(recoverWebWorker({ ...f.query, original: { ...f.query.original, executionDomainId: 'foreign' } }, f), /identity/);
  f.evidence.binding.accountFingerprint = 'different-account';
  await assert.rejects(recoverWebWorker(f.query, f), /binding/);
});

test('explicit retirement settles a failed original invocation but adoption alone does not invent SDK completion', async t => {
  const f = await fixture(t);
  f.journal.business.completed = false;
  f.journal.business.terminalKnown = false;
  f.journal.business.outcome = 'unknown';
  await f.save();
  await assert.rejects(recoverWebWorker(f.query, f), /adoption did not complete/);
  f.evidence.disposition = 'committed'; f.evidence.facts.outcome = 'failed'; f.evidence.terminalMain = [];
  await assert.rejects(recoverWebWorker(f.query, f), /adoption did not complete/);
  f.evidence.disposition = 'retired';
  const failed = await recoverWebWorker(f.query, f);
  assert.equal(failed.frames[1].status, 'error');
  assert.equal(failed.frames[1].lifecycle.outcome, 'failed');
  assert.equal(failed.frames[1].lifecycle.resumeCandidate, false);
  assert.equal(failed.frames[1].text, '');
  assert.equal(failed.frames[1].web.facts.dispatch, 'dispatched');
  assert.equal(failed.frames[1].lifecycle.turnId, f.journal.business.turnId);
  assert.equal((await readFile(join(f.root, 'calls.jsonl'), 'utf8')).trim().split('\n').length, 1);
  f.journal.business.sdkCleanupConfirmed = false; await f.save();
  await assert.rejects(recoverWebWorker(f.query, f), /shutdown proof/);
  f.journal.business.sdkCleanupConfirmed = true; f.journal.business.turnId = ''; await f.save();
  await assert.rejects(recoverWebWorker(f.query, f), /original SDK turn identity/);
});

test('an actual rejected SDK turn retains its owned turn identity when the plugin later retires the response', async t => {
  const f = await fixture(t, 'rejected-response');
  assert.equal(f.journal.business.terminalKnown, true);
  assert.equal(f.journal.business.outcome, 'failed');
  assert.notEqual(f.journal.business.turnId, '');
  f.evidence.disposition = 'retired'; f.evidence.facts.outcome = 'failed'; f.evidence.terminalMain = [];
  const result = (await recoverWebWorker(f.query, f)).frames[1];
  assert.equal(result.status, 'error'); assert.equal(result.lifecycle.outcome, 'failed');
  assert.equal(result.lifecycle.turnId, f.journal.business.turnId);
  assert.equal((await readFile(join(f.root, 'calls.jsonl'), 'utf8')).trim().split('\n').length, 1);
});

test('preserves a confirmed original failed SDK terminal and blocks a later adopted final', async t => {
  const f = await fixture(t);
  f.journal.business.completed = false; f.journal.business.terminalKnown = true; f.journal.business.outcome = 'failed'; await f.save();
  await assert.rejects(recoverWebWorker(f.query, f), /later adopted final/);
  f.evidence.facts.outcome = 'failed'; f.evidence.terminalMain = [];
  assert.equal((await recoverWebWorker(f.query, f)).frames[1].lifecycle.outcome, 'failed');
});

test('allows positively unsent invocation without synthesizing a successful business result', async t => {
  const f = await fixture(t);
  f.journal.phase = 'admitted'; delete f.journal.business; delete f.journal.result; await f.save();
  f.evidence.disposition = 'not-dispatched'; f.evidence.facts.dispatch = 'not-dispatched'; f.evidence.facts.outcome = 'failed'; f.evidence.terminalMain = [];
  const result = (await recoverWebWorker(f.query, f)).frames[1];
  assert.equal(result.status, 'error'); assert.equal(result.lifecycle.dispatch, 'not-dispatched'); assert.equal(result.lifecycle.outcome, 'failed');
});

test('requires original owner exit, terminal content, and complete immutable plugin evidence', async t => {
  const f = await fixture(t);
  const producer = f.journal.producerPid;
  f.journal.producerPid = process.pid; await f.save();
  await assert.rejects(recoverWebWorker(f.query, f), /worker exit/);
  f.journal.producerPid = producer; await f.save();
  for (const change of [
    e => { e.checkpointDigest = 'b'.repeat(64); }, e => { e.disposition = 'blocked'; e.facts.outcome = 'unknown'; },
    e => { e.terminalMain[0].text = 'different final'; }, e => { e.facts.localCleanupConfirmed = false; e.facts.outcome = 'unknown'; },
  ]) {
    const evidence = structuredClone(f.evidence); change(evidence);
    await assert.rejects(recoverWebWorker(f.query, { env: f.env, inspect: async () => evidence }));
  }
  await assert.rejects(recoverWebWorker(f.query, { env: f.env, inspect: async () => {
    f.journal.phase = 'prompt-accepted'; await f.save(); return f.evidence;
  } }), /evidence changed/);
});

test('rejects missing older checkpoints and symlinked worker evidence before querying a plugin', async t => {
  const f = await fixture(t);
  await assert.rejects(recoverWebWorker({ ...f.query, extra: true }, f), /Expected/);
  await assert.rejects(recoverWebWorker({ ...f.query, recoveryDirectory: join(f.root, 'unowned') }, f), /directory/);
  await rm(f.path);
  await assert.rejects(recoverWebWorker(f.query, f), { code: 'ENOENT' });
  const other = join(f.root, 'other.json'); await writeFile(other, f.saved, { mode: 0o600 }); await symlink(other, f.path);
  await assert.rejects(recoverWebWorker(f.query, f), { code: 'ELOOP' });
});
