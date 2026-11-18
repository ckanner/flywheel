import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { copyFile, mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

test('the built public recovery reader is independent of the provider installation and rejects invalid evidence', async t => {
  const directory = await mkdtemp(join(tmpdir(), 'flywheel-independent-web-reader-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const file = join(directory, 'reader.mjs');
  await copyFile(new URL('../dist/web-recovery-reader.js', import.meta.url), file);
  const module = await import(pathToFileURL(file));
  assert.deepEqual(Object.keys(module), ['readWebInvocationRecovery']);
  await assert.rejects(module.readWebInvocationRecovery({}), error => {
    assert.equal(error.name, 'WebPolicyError');
    assert.notEqual(error.code, 'ERR_MODULE_NOT_FOUND');
    assert.match(error.message, /checkpoint|version|contract/i);
    return true;
  });
});

test('the built recovery reader records exact source, bounded imports and all bundled licenses', async () => {
  const vendor = JSON.parse(await readFile(new URL('../vendor/chatgpt.json', import.meta.url), 'utf8'));
  const manifest = JSON.parse(await readFile(new URL('../dist/web-recovery-reader.manifest.json', import.meta.url), 'utf8'));
  assert.equal(manifest.source.commit, vendor.sourceCommit);
  assert.equal(manifest.source.tree, vendor.sourceTree);
  assert.equal(manifest.source.archiveSHA256, vendor.sha256);
  assert.deepEqual(manifest.imports, ['node:crypto', 'node:fs', 'node:fs/promises', 'node:os', 'node:path']);
  assert.deepEqual(manifest.bundledPackages.map(item => item.name), ['ajv', 'dsh-plugin-chatgpt', 'fast-deep-equal', 'fast-uri', 'json-schema-traverse']);
  for (const item of [manifest, manifest.licenses]) {
    const bytes = await readFile(new URL('../dist/' + item.file, import.meta.url));
    assert.equal(bytes.length, item.bytes);
    assert.equal(createHash('sha256').update(bytes).digest('hex'), item.sha256);
  }
  const licenses = await readFile(new URL('../dist/' + manifest.licenses.file, import.meta.url), 'utf8');
  for (const item of manifest.bundledPackages) assert.ok(licenses.includes(item.name + '@' + item.version));
});
