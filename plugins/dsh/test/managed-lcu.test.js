import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { constants } from 'node:fs';
import { copyFile, lstat, mkdir, mkdtemp, open, readFile, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { Context } from '@deepseek-ai/cordis';
import { agentEvents } from '@deepseek-ai/dsh-agent';
import { LlmAdapter } from '@deepseek-ai/dsh-llm';
import { mountAgentLoopTestDependencies, mountAgentLoopTestHarness } from '@deepseek-ai/dsh-agent-loop-testkit';
import { PtcExecutor } from '@deepseek-ai/dsh-tool-executor-ptc';
import { connectCua } from 'dsh-plugin-lcu/bridge';
import { createManagedLcu } from '../src/managed-lcu.js';
import { NORMALIZED_PNG, validatePng } from './fixtures/png-fixture.js';

const hash = data => createHash('sha256').update(data).digest('hex');
class OfflineModel extends LlmAdapter {
  async resolveModel(provider, model) { return { id: model, provider, name: model, inputModalities: ['text', 'image'] }; }
  async *stream() { throw new Error('These attachment tests must not make a model request'); }
}

async function fixture(t, { cleanupFailure = false, images = 2, attachmentFault, normalize = false, onEvent, verifyLease } = {}) {
  const directory = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-managed-lcu-')));
  const app = join(directory, 'Fixture.app');
  const runtime = join(app, 'Contents/Resources/cua_node');
  const launcher = join(runtime, 'lib/node_modules/@oai/cua-repl/bin/cua-repl.mjs');
  await mkdir(dirname(launcher), { recursive: true });
  await copyFile(new URL('./fixtures/managed-lcu-runtime.mjs', import.meta.url), launcher);
  await mkdir(join(runtime, 'bin'));
  await writeFile(join(runtime, 'bin/node'), `#!/bin/sh\nexec '${process.execPath.replaceAll("'", "'\"'\"'")}' "$@"\n`, { mode: 0o700 });
  await mkdir(join(runtime, 'lib/node_modules/@oai/sky/Codex Computer Use.app'), { recursive: true });
  await mkdir(join(app, 'Contents/Resources/codex-cli/bin'), { recursive: true });
  await writeFile(join(app, 'Contents/Resources/codex-cli/bin/codex'), 'fixture', { mode: 0o700 });
  await writeFile(join(runtime, 'manifest.json'), '{}');
  await writeFile(join(app, 'Contents/Info.plist'), '<key>CFBundleShortVersionString</key><string>fixture</string>');
  const trace = join(directory, 'trace.jsonl');
  await writeFile(trace, '');
  await writeFile(join(app, 'Contents/Resources/fixture.json'), JSON.stringify({ cleanupFailure, images, trace }));
  const store = join(directory, 'attachments');
  const taskDir = join(directory, 'task');
  await mkdir(store); await mkdir(taskDir);
  const ctx = new Context();
  await mountAgentLoopTestDependencies(ctx, { workingDirectory: true });
  ctx.llm.registerAdapter(['offline-fixture'], new OfflineModel());
  const stored = new Map();
  ctx.provide('attachments', {
    async saveImages(images) {
      return Promise.all(images.map(async image => {
        validatePng(image.data);
        const attachmentId = randomUUID();
        const path = join(store, attachmentId);
        const bytes = normalize ? NORMALIZED_PNG : image.data;
        stored.set(attachmentId, hash(bytes));
        await writeFile(path, bytes, { mode: 0o600 });
        if (attachmentFault === 'hash') await writeFile(path, Buffer.alloc(bytes.length, 42));
        if (attachmentFault === 'symlink') { await rm(path); await symlink(trace, path); }
        return { attachmentId, mediaType: image.mediaType, bytes: bytes.length, width: 1, height: 1 };
      }));
    },
    async readImage(ref) {
      const file = await open(join(store, ref.attachmentId), constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
      try {
        const data = await file.readFile();
        assert.equal(data.length, ref.bytes); assert.equal(hash(data), stored.get(ref.attachmentId));
        validatePng(data);
        return { ref, data };
      } finally { await file.close(); }
    },
    imageHostPath: ref => join(store, ref.attachmentId),
  });
  ctx.provide('userQuestions', { ask: async () => ({ answers: [] }) });
  const driver = await mountAgentLoopTestHarness(ctx);
  const admitted = new Map();
  const root = await driver.create('managed-root', { provider: 'offline-fixture', model: 'vision' }, { cwd: directory });
  admitted.set(root.id, root);
  const events = [], admissions = [];
  const managed = {
    version: 1, runId: 'run-1', invocationId: 'invocation-1', candidateId: 'candidate-sha-1',
    role: 'executor', scope: 'packet-1', channel: 'fixture', taskDir,
    lease: { leaseId: 'lease-1', resourceId: directory, leaseEpoch: 'epoch-1', ownerInvocationId: 'invocation-1' },
  };
  const identityOf = agent => {
    if (admitted.get(agent.id) !== agent || ctx.agents.get(agent.id) !== agent) throw new Error('unadmitted Agent');
    if (agent !== root && (agent.session.header.parentSession !== root.id || agent.session.header.origin !== 'subagent')) throw new Error('unrelated Agent');
    return { runId: managed.runId, invocationId: managed.invocationId, rootSessionId: root.id, candidateId: managed.candidateId };
  };
  const adapter = await createManagedLcu(ctx, {
    options: { app }, managed, rootSessionId: root.id, identityOf,
    verifyLease: async request => { admissions.push(request); await verifyLease?.(request); },
    onEvent: async event => { await onEvent?.(event); events.push(event); },
  });
  t.after(async () => {
    await adapter.close();
    // A cleanup-unknown test intentionally retains its receipt. Cordis contains
    // disposal observer errors; this must not change the recorded owner outcome.
    await ctx.fiber.dispose();
    await rm(directory, { recursive: true, force: true });
  });
  async function begin(agent = root, turn = 1) {
    await agentEvents(ctx, agent).waterfall('agent/pre-step', { turn, step: 1, signal: new AbortController().signal }, async () => ({ action: 'continue' }));
  }
  let nextCall = 0;
  async function call(args, agent = root, signal = new AbortController().signal) {
    const batch = await ctx.tools.execute({ agent, signal,
      calls: [{ type: 'tool-call', id: `call-${++nextCall}`, name: 'lcu_cua', arguments: JSON.stringify(args) }] });
    return batch.results[0].result;
  }
  return { adapter, ctx, root, events, admissions, managed, admitted, directory, taskDir, app, driver, begin, call,
    rows: async () => (await readFile(trace, 'utf8')).trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) };
}

test('real DSH Agent scopes mount public managed LCU, serialize descendants and await both runtimes', async t => {
  const f = await fixture(t, { images: 0 });
  const child = await f.driver.create('managed-child', { provider: 'offline-fixture', model: 'vision' },
    { parentSession: f.root.id, origin: 'subagent', delegationDepth: 1 });
  f.admitted.set(child.id, child);
  await Promise.all([f.adapter.attach(f.root), f.adapter.attach(child)]);
  await f.adapter.attach(f.root); // idempotent, no second runtime or tool registration
  assert.equal(f.root.session.header.agentPreset, undefined);
  assert.equal(child.session.header.parentSession, f.root.id);
  assert.ok(f.ctx.tools.schemas(f.root).some(tool => tool.name === 'lcu_cua'));
  assert.ok(f.ctx.tools.schemas(child).some(tool => tool.name === 'lcu_cua'));
  assert.equal(f.ctx.tools.schemas().some(tool => tool.name === 'lcu_cua'), false);
  await f.begin(); await f.begin(child);
  const results = await Promise.all([f.call({ method: 'action', args: ['root'] }), f.call({ method: 'action', args: ['child'] }, child)]);
  assert.ok(results.every(result => !result.isError), JSON.stringify(results));
  assert.deepEqual((await f.rows()).filter(row => ['start', 'end'].includes(row.event)).map(row => row.event), ['start', 'end', 'start', 'end']);
  assert.deepEqual(new Set(f.admissions.map(value => value.agentSessionId)), new Set([f.root.id, child.id]));
  const receipts = await f.adapter.close('owned SDK shutdown');
  assert.equal(receipts.length, 2);
  assert.ok(receipts.every(value => value.cleanup === 'confirmed' && value.runtimeExitConfirmed && value.turnCleanupConfirmed));
  assert.deepEqual(f.adapter.snapshot(), receipts);
  assert.equal(f.ctx.tools.schemas(f.root).some(tool => tool.name === 'lcu_cua'), false);
});

test('media evidence is copied from verified attachment bytes and bound to the invocation candidate', async t => {
  const f = await fixture(t);
  await f.adapter.attach(f.root); await f.begin();
  const result = await f.call({ method: 'getApp', args: ['Finder'] });
  assert.equal(result.isError, false, JSON.stringify(result));
  const event = f.events.find(event => event.type === 'media');
  assert.equal(event.facts.preparedImages, 2);
  assert.equal(event.facts.imageDeliveredToModel, 'unverified');
  assert.equal(event.facts.evidenceAttachedToRun, true);
  const evidence = event.facts.evidence;
  assert.equal(evidence.status, 'attached');
  assert.equal(evidence.identity.candidateId, 'candidate-sha-1');
  assert.equal(evidence.identity.rootSessionId, f.root.id);
  assert.equal(evidence.agentSessionId, f.root.id);
  assert.equal(evidence.images.length, 2);
  const manifest = JSON.parse(await readFile(evidence.manifest, 'utf8'));
  assert.deepEqual(manifest.identity, evidence.identity);
  for (const image of manifest.images) {
    assert.ok(image.path.startsWith(join(f.taskDir, 'reports/native/invocation-1') + '/'));
    assert.equal(hash(await readFile(image.path)), image.sha256);
    assert.equal((await lstat(image.path)).mode & 0o077, 0);
  }
  assert.equal(manifest.imageDeliveredToModel, 'unverified');
  assert.equal((await lstat(evidence.manifest)).mode & 0o077, 0);
});

test('the published rc.3 PTC dispatcher keeps callable handles and defers multiple images', async t => {
  const f = await fixture(t);
  await f.adapter.attach(f.root); await f.begin();
  f.ctx.provide('ptcRuntime', {
    language: 'typescript', isolation: 'fixture', programFormat: 'function-body', retainsState: false,
    resolve: spec => ({ ...spec, timeoutMs: 10000 }), release() {},
    async run(spec) {
      // The program-runtime seam is offline. The actual published PTC executor
      // owns binding dispatch, tool execution, canonical values and image routing.
      const cua = connectCua(args => spec.bindings[0].functions.lcu_cua(args));
      const app = await cua.getApp('Finder');
      assert.equal(app.name, 'Finder');
      assert.deepEqual(await app.activate(), { active: true });
      return { logs: ['canonical-ptc-handles-ok'], value: { name: app.name } };
    },
  });
  f.root.ctx.tools.setExecutor(new PtcExecutor(f.root.ctx));
  const batch = await f.ctx.tools.execute({ agent: f.root, signal: new AbortController().signal,
    calls: [{ type: 'tool-call', id: 'actual-ptc-dispatch', name: 'run_code',
      arguments: JSON.stringify({ code: 'offline program-runtime fixture', description: 'Verify LCU handle and image routing' }) }] });
  assert.equal(batch.results[0].result.isError, false, JSON.stringify(batch));
  assert.match(JSON.stringify(batch.results[0].result.content), /canonical-ptc-handles-ok/);
  assert.equal(batch.additionalContexts.flatMap(message => message.content).filter(block => block.type === 'image').length, 2);
  assert.equal(f.events.find(event => event.type === 'media').facts.imageDeliveredToModel, 'unverified');
});

test('normalized attachment bytes retain their own hash and the original capture provenance', async t => {
  const f = await fixture(t, { normalize: true });
  await f.adapter.attach(f.root); await f.begin();
  assert.equal((await f.call({ method: 'getApp', args: ['Finder'] })).isError, false);
  const evidence = f.events.find(event => event.type === 'media').facts.evidence;
  assert.equal(evidence.evidenceAttachedToRun, true);
  for (const image of evidence.images) {
    assert.equal(image.normalized, true);
    assert.notEqual(image.sha256, image.capturedSha256);
    assert.equal(hash(await readFile(image.path)), hash(NORMALIZED_PNG));
  }
});

for (const attachmentFault of ['hash', 'symlink']) test(`untrusted ${attachmentFault} attachment cannot become run evidence`, async t => {
  const f = await fixture(t, { attachmentFault });
  await f.adapter.attach(f.root); await f.begin();
  assert.equal((await f.call({ method: 'getApp', args: ['Finder'] })).isError, false);
  const evidence = f.events.find(event => event.type === 'media').facts.evidence;
  assert.equal(evidence.evidenceAttachedToRun, false);
  assert.equal(evidence.status, 'unavailable');
  assert.ok(evidence.images.every(image => image.status === 'unavailable' && image.error));
});

test('an existing evidence-directory symlink is refused without writing outside the task', async t => {
  const f = await fixture(t);
  const outside = join(f.directory, 'outside');
  await mkdir(outside); await mkdir(join(f.taskDir, 'reports'));
  await symlink(outside, join(f.taskDir, 'reports/native'));
  await f.adapter.attach(f.root); await f.begin();
  assert.equal((await f.call({ method: 'getApp', args: ['Finder'] })).isError, false);
  const evidence = f.events.find(event => event.type === 'media').facts.evidence;
  assert.equal(evidence.evidenceAttachedToRun, false);
  assert.match(evidence.error, /owned physical directory/);
  const { readdir } = await import('node:fs/promises');
  assert.deepEqual(await readdir(outside), []);
});

test('live identity and lease are checked again before native dispatch', async t => {
  let revoked = false;
  const f = await fixture(t, { images: 0, verifyLease: async () => { if (revoked) throw new Error('revoked lease'); } });
  await f.adapter.attach(f.root); await f.begin();
  revoked = true;
  assert.equal((await f.call({ method: 'action', args: ['revoked'] })).isError, true);
  revoked = false; f.admitted.delete(f.root.id);
  assert.equal((await f.call({ method: 'action', args: ['unrelated'] })).isError, true);
  assert.equal((await f.rows()).some(row => row.event === 'start'), false);
});

test('unknown turn cleanup remains unknown after actual kernel exit in aggregate close receipts', async t => {
  const f = await fixture(t, { images: 0, cleanupFailure: true });
  await f.adapter.attach(f.root); await f.begin();
  assert.equal((await f.call({ method: 'getState' })).isError, false);
  const receipts = await f.adapter.close();
  assert.equal(receipts.length, 1);
  assert.equal(receipts[0].runtimeExitConfirmed, true);
  assert.equal(receipts[0].turnCleanupConfirmed, false);
  assert.equal(receipts[0].cleanup, 'unknown');
  assert.equal(f.adapter.snapshot()[0].cleanup, 'unknown');
  await assert.rejects(() => f.adapter.attach(f.root), /closing/);
});

test('close waits for an in-flight attachment and then returns that runtime receipt', async t => {
  let entered, release;
  const entering = new Promise(resolve => { entered = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  const f = await fixture(t, { images: 0, verifyLease: async admission => { if (admission.phase === 'attach') { entered(); await gate; } } });
  const attaching = f.adapter.attach(f.root);
  await entering;
  let closed = false;
  const closing = f.adapter.close().then(value => { closed = true; return value; });
  assert.equal(closed, false);
  release(); await attaching;
  const receipts = await closing;
  assert.equal(receipts.length, 1);
  assert.equal(receipts[0].cleanup, 'confirmed');
});

test('real turn-stopping dispatch cancels a pending MCP call before acknowledging cleanup', async t => {
  const f = await fixture(t, { images: 0 });
  await f.adapter.attach(f.root); await f.begin();
  const running = f.call({ method: 'wait' });
  const deadline = Date.now() + 5000;
  while (!(await f.rows()).some(row => row.event === 'waiting')) {
    assert.ok(Date.now() < deadline, 'the real generated program must enter the runtime');
    await new Promise(resolve => setTimeout(resolve, 5));
  }
  await agentEvents(f.ctx, f.root).serial('agent/turn-stopping', { turn: 1, signal: AbortSignal.abort() });
  assert.equal((await running).isError, true);
  assert.ok((await f.rows()).some(row => row.event === 'cancel'));
  assert.equal((await f.adapter.close())[0].cleanup, 'confirmed');
});

test('a failed host event receipt prevents GUI dispatch and cleanup remains explicitly observable', async t => {
  const f = await fixture(t, { images: 0, onEvent: async event => { if (event.type === 'call.started') throw new Error('event journal unavailable'); } });
  await f.adapter.attach(f.root); await f.begin();
  assert.equal((await f.call({ method: 'action', args: ['never'] })).isError, true);
  assert.equal((await f.rows()).some(row => row.event === 'start'), false);
  assert.equal((await f.adapter.close())[0].cleanup, 'confirmed');
  assert.ok(f.events.some(event => event.type === 'cleanup'));
});
