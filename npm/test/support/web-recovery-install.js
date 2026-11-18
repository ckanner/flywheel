"use strict";

const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { createHash } = require('node:crypto');

function invoke(script, input, env, cwd) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [script], { env, cwd, stdio: ['pipe', 'pipe', 'pipe'] });
    let stdout = '', stderr = '';
    child.stdout.on('data', value => { stdout += value; });
    child.stderr.on('data', value => { stderr += value; });
    const timer = setTimeout(() => { child.kill('SIGKILL'); reject(new Error('Web recovery fixture timed out: ' + stderr)); }, 45000);
    child.on('error', error => { clearTimeout(timer); reject(error); });
    child.on('close', code => {
      clearTimeout(timer);
      if (code !== 0) return reject(new Error(`Web recovery fixture exited ${code}: ${stderr}\n${stdout}`));
      try { resolve(JSON.parse(stdout)); } catch (error) { reject(new Error(`Invalid fixture output: ${stdout}\n${stderr}`, { cause: error })); }
    });
    child.stdin.end(JSON.stringify(input) + '\n');
  });
}

// Include inode, permissions and timestamps: rewriting identical bytes is also
// forbidden during recovery. atime is excluded because this query reads files.
async function snapshot(root) {
  const files = {};
  async function visit(directory) {
    const directoryStat = await fs.lstat(directory);
    files[path.relative(root, directory) + '/'] = { ino: directoryStat.ino, mode: directoryStat.mode,
      mtime: directoryStat.mtimeMs, ctime: directoryStat.ctimeMs };
    for (const entry of await fs.readdir(directory, { withFileTypes: true })) {
      const filename = path.join(directory, entry.name);
      const stat = await fs.lstat(filename);
      assert.equal(stat.isSymbolicLink(), false, 'fixture evidence must be physical');
      if (entry.isDirectory()) await visit(filename);
      else {
        assert.equal(stat.isFile(), true);
        files[path.relative(root, filename)] = { hash: createHash('sha256').update(await fs.readFile(filename)).digest('hex'),
          ino: stat.ino, mode: stat.mode, size: stat.size, mtime: stat.mtimeMs, ctime: stat.ctimeMs };
      }
    }
  }
  await visit(root);
  return files;
}

async function assertInstalledWebRecovery(context, workerPath) {
  const root = await fs.realpath(await fs.mkdtemp(path.join(context.root, 'packed web evidence ')));
  await fs.chmod(root, 0o700);
  await fs.mkdir(path.join(root, '.sessions'), { mode: 0o700 });
  const home = path.join(root, 'user-home');
  await fs.mkdir(home, { mode: 0o700 });
  const sdk = path.join(__dirname, 'web-recovery-sdk.mjs');
  await fs.chmod(sdk, 0o755);
  const env = { ...context.env, HOME: home, DSH_HOME: path.join(root, 'dsh-home'),
    FLYWHEEL_DSH_PROFILE: 'packed-web-recovery', FLYWHEEL_DSH_BIN: sdk,
    FLYWHEEL_TEST_WEB_STATE: path.join(root, 'plugin'), FLYWHEEL_TEST_WEB_CALLS: path.join(root, 'calls.jsonl'),
    FLYWHEEL_TEST_WEB_HISTORY: path.join(root, 'sdk-history') };
  const requestId = 'packed-original-web-invocation';
  const recoveryDirectory = path.join(root, '.sessions', 'recovery-' + createHash('sha256').update(requestId).digest('hex'));
  const request = { type: 'start', protocol: 2, persistent: true, requestId, cwd: root,
    provider: 'chatgpt', model: 'gpt-6-pro-web', reasoningEffort: 'pro', permissionMode: 'danger-full-access',
    prompt: 'Original operation for the installed read-only recovery test', timeoutMs: 30000, idleTimeoutMs: 10000,
    web: { version: 1, executionDomainId: 'packed-task-domain', requireIsolation: true, recoveryDirectory } };
  const original = await invoke(path.join(__dirname, 'web-recovery-owner.mjs'), { request, env }, env, root);
  assert.equal(original.result.status, 'ok', JSON.stringify(original));
  assert.equal(original.result.lifecycle.outcome, 'completed');
  const journalPath = path.join(recoveryDirectory, 'worker.json');
  const journal = JSON.parse(await fs.readFile(journalPath, 'utf8'));
  assert.equal(journal.phase, 'settled');
  assert.equal(journal.business.completed, true);
  assert.equal(journal.business.sdkCleanupConfirmed, true);
  assert.equal(journal.business.protocolIntact, true);
  assert.ok(journal.business.turnId);
  const calls = (await fs.readFile(env.FLYWHEEL_TEST_WEB_CALLS, 'utf8')).trim().split('\n').map(JSON.parse);
  for (const kind of ['sdk-start', 'model', 'page-open', 'page-submit', 'durable-ack']) {
    assert.equal(calls.filter(event => event.kind === kind).length, 1, `exactly one original ${kind}`);
  }
  assert.equal(calls.filter(event => event.kind === 'tool').length, 0);
  assert.equal(calls.find(event => event.kind === 'durable-ack').sessionId, journal.ready.sessionId);
  const { nativeDigest: _digest, requestId: _id, ...identity } = journal.original;
  const query = { type: 'web-recover', protocol: 1, requestId, recoveryDirectory,
    original: { ...identity, native: null }, expectedBinding: journal.ready.web };
  const modulePath = path.join(path.dirname(workerPath), 'web-recovery.mjs');
  const queryScript = path.join(context.fixtures, 'installed-web-recovery-query.mjs');
  await fs.copyFile(path.join(__dirname, 'web-recovery-query.mjs'), queryScript);
  const before = await snapshot(root);
  const recovered = await invoke(queryScript, { modulePath, query }, env, context.project);
  assert.equal(recovered.ok, true, JSON.stringify(recovered));
  const result = recovered.result.frames[1];
  assert.equal(recovered.result.type, 'web-recovery');
  assert.match(recovered.result.revision, /^[a-f0-9]{64}$/);
  assert.equal(result.status, 'ok');
  assert.equal(result.text, journal.business.text);
  assert.equal(result.lifecycle.outcome, 'completed');
  assert.equal(result.lifecycle.turnId, journal.business.turnId);
  assert.equal(result.lifecycle.cleanupConfirmed, true);
  assert.equal(result.lifecycle.usageStatus, 'unknown');
  assert.equal(result.usage, null);
  assert.equal(result.costUsd, null);
  assert.deepEqual(result.web.binding, journal.ready.web);
  assert.deepEqual(await invoke(queryScript, { modulePath, query }, env, context.project), recovered,
    'separate query processes must recover the same original evidence');
  assert.deepEqual(await invoke(workerPath, query, env, context.project), recovered.result,
    'the installed public worker must expose the same read-only recovery with one JSON line followed by EOF');
  assert.deepEqual(await snapshot(root), before, 'recovery must neither write evidence nor restart SDK, model, browser or tools');

  // Keep the worker envelope internally consistent so the negative case
  // reaches the actual bundled plugin reader, not an earlier digest guard.
  journal.checkpoint.contract = 'unsupported-checkpoint';
  const { pathToFileURL } = require('node:url');
  const { nativeDigest } = await import(pathToFileURL(path.resolve(__dirname, '../../../plugins/dsh/src/native.js')));
  journal.checkpointDigest = nativeDigest(journal.checkpoint);
  await fs.writeFile(journalPath, JSON.stringify(journal), { mode: 0o600 });
  const invalidBefore = await snapshot(root);
  const invalid = await invoke(queryScript, { modulePath, query }, env, context.project);
  assert.equal(invalid.ok, false);
  assert.notEqual(invalid.code, 'ERR_MODULE_NOT_FOUND');
  assert.notEqual(invalid.code, 'MODULE_NOT_FOUND');
  assert.match(invalid.message, /checkpoint|recovery contract/i);
  assert.doesNotMatch(invalid.message, /Cannot find (?:package|module)/);
  assert.deepEqual(await snapshot(root), invalidBefore, 'rejected recovery is also strictly read-only');
}

module.exports = { assertInstalledWebRecovery };
