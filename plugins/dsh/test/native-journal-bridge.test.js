import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, rm, writeFile, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { readNativeEvents } from '../src/bridge.js';

test('worker admits only the complete durable journal for its owned invocation', async t => {
  const directory = await mkdtemp(join(tmpdir(), 'native-journal-bridge-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const path = join(directory, 'native-events.jsonl');
  const identity = { requestId: 'call', nonce: 'private-nonce', rootSessionId: 'root' };
  const entry = { ...identity, version: 1, journalSequence: 1, type: 'runtime.opened' };
  const line = JSON.stringify(entry) + '\n';
  await writeFile(path, line);
  const summary = { complete: true, events: 1, bytes: Buffer.byteLength(line) };
  assert.deepEqual(await readNativeEvents(directory, identity, summary), [entry]);
  for (const field of ['requestId', 'nonce', 'rootSessionId']) {
    await assert.rejects(readNativeEvents(directory, { ...identity, [field]: 'foreign' }), /ownership/);
  }
  for (const changed of [{ complete: false }, { events: 0 }, { bytes: 1 }]) {
    await assert.rejects(readNativeEvents(directory, identity, { ...summary, ...changed }), /complete durable/);
  }
  await writeFile(path, line + line);
  await assert.rejects(readNativeEvents(directory, identity), /sequence/);
  await writeFile(path, line.slice(0, -1));
  await assert.rejects(readNativeEvents(directory, identity), /incomplete/);
  await writeFile(path, 'x'.repeat(2 * 1024 * 1024 + 1));
  await assert.rejects(readNativeEvents(directory, identity), /bounded/);
  await writeFile(path, '');
  assert.deepEqual(await readNativeEvents(directory, identity, { complete: true, events: 0, bytes: 0 }), []);
  await rm(path);
  await symlink(join(directory, 'outside.jsonl'), path);
  await assert.rejects(readNativeEvents(directory, identity));
});
