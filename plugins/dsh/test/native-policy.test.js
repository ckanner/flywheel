import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readFile, realpath, rm, mkdir, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { randomUUID } from 'node:crypto';
import { Context } from '@deepseek-ai/cordis';
import { agentEvents } from '@deepseek-ai/dsh-agent';
import { LlmAdapter } from '@deepseek-ai/dsh-llm';
import { mountAgentLoopTestDependencies, mountAgentLoopTestHarness } from '@deepseek-ai/dsh-agent-loop-testkit';
import { PtcExecutor } from '@deepseek-ai/dsh-tool-executor-ptc';
import { createSdkPtcComposition, validateSdkPtcAdmission } from 'dsh-preset-generator/composition';
import { createNativeJournal, createNativePolicy, nativeOverlay, probePtc } from '../src/native-policy.js';
import { validatePng } from './fixtures/png-fixture.js';

const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
class OfflineModel extends LlmAdapter {
  async resolveModel(provider, model) { return { id: model, provider, name: model, inputModalities: ['text', 'image'] }; }
  async *stream() { throw new Error('Native policy admission must never make a model request'); }
}

async function fixture(t, { images = true, corruptHandle = false, inspectWithoutImage = false, language = 'typescript' } = {}) {
  const directory = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-native-policy-')));
  const ctx = new Context();
  await mountAgentLoopTestDependencies(ctx, { workingDirectory: true });
  ctx.llm.registerAdapter(['offline'], new OfflineModel());
  const programs = [];
  if (images) ctx.provide('attachments', { async saveImages(images) {
    return images.map(image => { validatePng(image.data); return { attachmentId: randomUUID(), mediaType: image.mediaType, bytes: image.data.length, width: 1, height: 1 }; });
  } });
  ctx.provide('ptcRuntime', {
    language, isolation: 'offline-fixture', programFormat: 'function-body', retainsState: false,
    resolve: spec => ({ ...spec, timeoutMs: 10000 }), release() {},
    async run(spec) {
      programs.push(spec.program);
      const logs = [];
      const functions = spec.bindings[0].functions;
      const tools = corruptHandle ? { ...functions, flywheel_ptc_probe: async args => ({ ...await functions.flywheel_ptc_probe(args), __handle: 'corrupted' }) } : functions;
      // Evaluate the actual generated probe, using real PTC executor bindings.
      // Only the process/runtime boundary is offline; no model is requested.
      try {
        const value = await new AsyncFunction('tools', 'console', 'inspect', 'Image', spec.program)(tools,
          { log: value => logs.push(String(value)) }, inspectWithoutImage ? value => logs.push(String(value)) : undefined, undefined);
        return { logs, value };
      } catch (error) { return { logs, error: { kind: 'execution', message: error.message } }; }
    },
  });
  const driver = await mountAgentLoopTestHarness(ctx);
  const root = await driver.create('root', { provider: 'offline', model: 'vision' }, { cwd: directory });
  root.ctx.tools.setExecutor(new PtcExecutor(root.ctx));
  const native = { version: 1, preset: 'heavy' };
  const composition = createSdkPtcComposition({ runtimeProfile: 'sdk', preset: 'heavy' });
  const config = { native, composition, sessionId: root.id, requestId: 'invocation', nonce: randomUUID(),
    provider: 'offline', model: 'vision', permissionMode: 'workspace-write', cwd: directory,
    receiptPath: join(directory, 'policy.json') };
  t.after(async () => { await ctx.fiber.dispose(); await rm(directory, { recursive: true, force: true }); });
  return { ctx, root, config, composition, programs, directory, driver };
}

test('actual rc.3 PTC executes the generated probe with canonical handles, console output and three deferred images', async t => {
  const f = await fixture(t);
  const before = f.ctx.tools.schemas(f.root).map(value => value.name);
  const observation = await probePtc(f.ctx, f.root, f.composition);
  assert.equal(observation.outputContract, 'deferred-context-v1');
  assert.equal(observation.imageProbeStatus, 'passed');
  assert.equal(observation.probes.multiImageProjection, 'passed');
  assert.equal(observation.probes.imageIdentity, 'not-run');
  assert.equal(validateSdkPtcAdmission(f.composition, observation, { requireImages: true }).eligible, true);
  assert.equal(f.programs.length, 1);
  assert.ok(f.programs[0].includes('instanceof Image'), 'modern branch checks actual Image identity instead of only the inspect global');
  assert.deepEqual(f.ctx.tools.schemas(f.root).map(value => value.name), before, 'temporary probe tools must be disposed');
  assert.equal(f.root.session.header.agentPreset, undefined, 'SDK composition must not fabricate a desktop preset header');
});

test('missing attachments remain not-run and cannot admit a requested image capability', async t => {
  const f = await fixture(t, { images: false });
  const observed = await probePtc(f.ctx, f.root, f.composition);
  assert.equal(observed.imageProbeStatus, 'not-run');
  assert.equal(validateSdkPtcAdmission(f.composition, observed).eligible, true);
  assert.equal(validateSdkPtcAdmission(f.composition, observed, { requireImages: true }).eligible, false);
});

for (const [name, options] of [['canonical handle corruption', { corruptHandle: true }], ['an inspect global without Image identity', { inspectWithoutImage: true }], ['a non-TypeScript runtime', { language: 'python' }]]) {
  test(`PTC admission refuses ${name} through the real dispatcher`, async t => {
    const f = await fixture(t, options);
    await assert.rejects(() => probePtc(f.ctx, f.root, f.composition), languageError(options));
    assert.ok(f.ctx.tools.schemas(f.root).every(tool => !tool.name.startsWith('flywheel_ptc_')));
  });
}
function languageError(options) { return options.language ? /TypeScript PTC runtime/ : /canonical-value\/output probe/; }

test('a native SDK admission records observed tools, immutable composition and exact route without model inference', async t => {
  const f = await fixture(t);
  const policy = await createNativePolicy(f.ctx, f.config);
  const receipt = await policy.admit(f.root, true);
  assert.equal(receipt.matched, true);
  assert.equal(receipt.observed.namedPresetMounted, false);
  assert.equal(receipt.observed.admission.eligible, true);
  assert.equal(receipt.observed.runtimeBindingDigest.length, 64);
  await agentEvents(f.ctx, f.root).waterfall('agent/pre-step', { turn: 1, step: 1, signal: new AbortController().signal }, async () => ({ action: 'continue' }));
  await agentEvents(f.ctx, f.root).serial('agent/turn-stopping', { turn: 1, signal: new AbortController().signal });
  const events = (await readFile(join(f.directory, 'native-events.jsonl'), 'utf8')).trim().split('\n').map(line => JSON.parse(line));
  assert.equal(events[0].type, 'agent.admitted');
  assert.equal(events[0].version, 1); assert.equal(events[0].journalSequence, 1);
  assert.equal(events[0].requestId, f.config.requestId); assert.equal(events[0].nonce, f.config.nonce);
  await f.ctx.fiber.dispose();
  const cleanup = JSON.parse(await readFile(join(f.directory, 'native-cleanup.json'), 'utf8'));
  assert.equal(cleanup.journal.complete, true);
  assert.equal(cleanup.journal.events, events.length);
});

test('a requested but absent capability is recorded matched:false and cannot admit descendants', async t => {
  const f = await fixture(t);
  f.config.native = { ...f.config.native, agents: { enabled: true } };
  const policy = await createNativePolicy(f.ctx, f.config);
  await assert.rejects(() => policy.admit(f.root, true), /did not register/);
  const child = await f.driver.create('child', { provider: 'offline', model: 'vision' }, { origin: 'subagent', parentSession: f.root.id, cwd: f.directory });
  assert.equal(policy.isChild(child), false);
  const events = (await readFile(join(f.directory, 'native-events.jsonl'), 'utf8')).trim().split('\n').map(line => JSON.parse(line));
  assert.equal(events.at(-1).type, 'agent.admission-rejected');
  assert.equal(events.at(-1).matched, false);
  assert.ok(events.every(value => value.type !== 'agent.admitted'));
});

test('foreign root and changed route fail before any probe or native dispatch', async t => {
  const f = await fixture(t);
  const policy = await createNativePolicy(f.ctx, { ...f.config, sessionId: 'different' });
  await assert.rejects(() => policy.admit(f.root, true), /unrelated/);
  assert.equal(f.programs.length, 0);
});

test('native overlay enables the selected public APIs and preserves the SDK composition contract', () => {
  const options = { native: { version: 1, preset: 'daily', agents: { enabled: true, maxConcurrent: 5, maxDepth: 3 }, team: { enabled: true }, workflows: { enabled: true }, web: { search: true, fetch: false } } };
  const value = nativeOverlay(options);
  assert.deepEqual(value.overlay.find(row => row.id === 'subagent').config, { maxActiveSubagents: 5, maxDepth: 3 });
  assert.equal(value.overlay.find(row => row.id === 'tool-agent-team').disabled, false);
  assert.deepEqual(value.overlay.find(row => row.id === 'tool-web').config, { search: true, fetch: false });
  assert.equal(value.composition.presetPolicy, 'daily');
});

test('journal enforces a bounded durable queue and canonical host correlation fields', async t => {
  const directory = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-native-journal-')));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const path = join(directory, 'events.jsonl');
  const journal = await createNativeJournal(path, { requestId: 'owned', rootSessionId: 'root' });
  await journal.emit({ version: 99, requestId: 'forged', journalSequence: 100, type: 'bounded' });
  const row = JSON.parse((await readFile(path, 'utf8')).trim());
  assert.equal(row.version, 1); assert.equal(row.requestId, 'owned'); assert.equal(row.journalSequence, 1);
  await assert.rejects(() => journal.emit({ value: 'x'.repeat(2 * 1024 * 1024) }), /budget/);
  await assert.rejects(() => journal.flush(), /budget/);
  assert.equal((await journal.close()).complete, false);
  assert.equal((await readFile(path, 'utf8')).trim().split('\n').length, 1);
});

test('journal count limit is explicit and files cannot be preplaced or redirected through directory links', async t => {
  const directory = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-native-journal-')));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const journal = await createNativeJournal(join(directory, 'events.jsonl'), {});
  // Reserve before awaiting writes: this also bounds concurrent producer memory.
  const writes = Array.from({ length: 4096 }, (_, index) => journal.emit({ type: 'fixture', index }));
  await assert.rejects(() => journal.emit({ type: 'over-limit' }), /budget/);
  await Promise.allSettled(writes);
  assert.equal((await journal.close()).complete, false);
  await assert.rejects(() => createNativeJournal(join(directory, 'events.jsonl'), {}), /EEXIST/);
  await mkdir(join(directory, 'physical')); await symlink(join(directory, 'physical'), join(directory, 'alias'));
  await assert.rejects(() => createNativeJournal(join(directory, 'alias/events.jsonl'), {}), /physical directory/);
});
