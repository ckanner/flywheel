import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readFile, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { randomUUID } from 'node:crypto';
import { PassThrough } from 'node:stream';
import { Context } from '@deepseek-ai/cordis';
import { agentEvents } from '@deepseek-ai/dsh-agent';
import { LlmAdapter } from '@deepseek-ai/dsh-llm';
import { mountAgentLoopTestDependencies, mountAgentLoopTestHarness } from '@deepseek-ai/dsh-agent-loop-testkit';
import Subagent from '@deepseek-ai/dsh-subagent';
import Team from '@deepseek-ai/dsh-agent-team';
import * as TeamTools from '@deepseek-ai/dsh-tool-agent-team';
import * as SubagentTools from '@deepseek-ai/dsh-tool-subagent';
import * as SubagentControl from '@deepseek-ai/dsh-tool-subagent-control';
import * as Spawn from '@deepseek-ai/dsh-subagent-spawn-in-process';
import * as Fork from '@deepseek-ai/dsh-subagent-fork-in-process';
import * as Jsonl from '@deepseek-ai/dsh-session-persistence-jsonl';
import SessionQuery from '@deepseek-ai/dsh-session-query';
import * as SdkServer from '@deepseek-ai/dsh-sdk-jsonrpc-server';
import { JsonRpcLineTransport } from '@deepseek-ai/dsh-sdk-protocol';
import { createNativePolicy, nativeOverlay } from '../src/native-policy.js';
import { NativeTree } from '../src/native.js';
import { SessionFold } from '../src/protocol.js';
import { validatePng } from './fixtures/png-fixture.js';

const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
const signal = () => AbortSignal.timeout(10000);
class ExactSessionQuery extends SessionQuery {
  async searchSessions() { throw new Error('This fixture uses only the real backend-independent exact session reads'); }
  async searchEvents() { throw new Error('This fixture uses only the real backend-independent exact session reads'); }
}
class ScriptedModel extends LlmAdapter {
  calls = [];
  rootCalls = 0;
  entered = Promise.withResolvers();
  gate;
  constructor(sdk = false) { super(); this.sdk = sdk; }
  async resolveModel(provider, model) { return { id: model, provider, name: model, inputModalities: ['text', 'image'] }; }
  async *stream(options) {
    this.calls.push(options);
    options.signal?.throwIfAborted();
    this.entered.resolve();
    if (this.gate) await Promise.race([this.gate.promise, new Promise((_, reject) => {
      if (options.signal?.aborted) reject(options.signal.reason);
      else options.signal?.addEventListener('abort', () => reject(options.signal.reason), { once: true });
    })]);
    const root = this.sdk && options.messages.some(message => message.content.some(block => block.type === 'text' && block.text === 'sdk root fixture'));
    if (root && this.rootCalls++ === 0) {
      const block = { type: 'tool-call', id: 'sdk-native-delegation', name: 'run_code', arguments: JSON.stringify({
        code: "const task = await tools.team_task_create({subject:'SDK fixture',description:'Test actual native dispatch'}); const child = await tools.subagent({name:'sdk-child',description:'SDK native child',prompt:'sdk child fixture'}); console.log(JSON.stringify({task,child}));",
        description: 'Run native Team and child fixture through the SDK',
      }) };
      yield { type: 'block-start', index: 0, blockType: 'tool-call' };
      yield { type: 'tool-call-delta', index: 0, id: block.id, name: block.name, argumentsDelta: block.arguments };
      yield { type: 'block-end', index: 0, block };
      yield { type: 'usage', usage: { inputTokens: 3, outputTokens: 2 } };
      yield { type: 'finish', reason: { kind: 'tool-calls' } };
      return;
    }
    const text = root ? 'SDK root final answer' : 'offline child result';
    yield { type: 'block-start', index: 0, blockType: 'text' };
    yield { type: 'text-delta', index: 0, text };
    yield { type: 'block-end', index: 0, block: { type: 'text', text } };
    yield { type: 'usage', usage: { inputTokens: 3, outputTokens: 2 } };
    yield { type: 'finish', reason: { kind: 'stop' } };
  }
}

async function fixture(t, { sdk = false, maxDepth = 3, hold = false } = {}) {
  const directory = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-native-flow-')));
  const ctx = new Context();
  t.after(async () => { await ctx.fiber.dispose(); await rm(directory, { recursive: true, force: true }); });
  await mountAgentLoopTestDependencies(ctx, { workingDirectory: true, tools: { mode: 'ptc' } });
  await ctx.plugin(Jsonl, { root: join(directory, 'sessions'), compression: 'none' });
  await ctx.plugin(ExactSessionQuery);
  ctx.provide('attachments', { async saveImages(images) {
    return images.map(image => { validatePng(image.data); return { attachmentId: randomUUID(), mediaType: image.mediaType, bytes: image.data.length, width: 1, height: 1 }; });
  } });
  ctx.provide('ptcRuntime', {
    language: 'typescript', isolation: 'offline-fixture', programFormat: 'function-body', retainsState: false,
    resolve: spec => ({ ...spec, timeoutMs: 10000 }), release() {},
    async run(spec) {
      const logs = [];
      try {
        const value = await new AsyncFunction('tools', 'console', spec.program)(spec.bindings[0].functions, { log: value => logs.push(String(value)) });
        return { logs, value };
      } catch (error) { return { logs, error: { kind: 'execution', message: error.message } }; }
    },
  });
  // This is the global executor composition used by the SDK overlay. Children
  // inherit the actual PTC service rather than a test-only root tool override.
  const model = new ScriptedModel(sdk); ctx.llm.registerAdapter(['offline'], model);
  if (hold) model.gate = Promise.withResolvers();
  const driver = await mountAgentLoopTestHarness(ctx);
  const native = { version: 1, preset: 'heavy', agents: { enabled: true, maxDepth, maxConcurrent: 4 }, team: { enabled: true } };
  const { composition, overlay } = nativeOverlay({ native });
  const configOf = id => overlay.find(row => row.id === id).config;
  await ctx.plugin(Subagent, configOf('subagent'));
  await ctx.plugin(Spawn, { providerName: 'spawn' });
  await ctx.plugin(Fork, { providerName: 'fork' });
  await ctx.plugin(SubagentControl, configOf('tool-subagent-control'));
  await ctx.plugin(SubagentTools, configOf('tool-subagent'));
  await ctx.plugin({ name: 'fixture-fork-delegation', inject: SubagentTools.inject,
    apply: context => SubagentTools.apply(context, configOf('tool-subagent-fork')) });
  await ctx.plugin(Team, { ...configOf('agent-team'), defaultEnabled: false });
  await ctx.plugin(TeamTools);
  const config = { native, composition, sessionId: 'root', requestId: 'native-flow', nonce: randomUUID(),
    provider: 'offline', model: 'vision', permissionMode: 'workspace-write', cwd: directory,
    receiptPath: join(directory, 'policy.json') };
  const tree = new NativeTree('root', native);
  const observedErrors = [], steps = [], admissions = [];
  // The same public event mapping used by the published SDK JSON-RPC server.
  if (!sdk) ctx.on('session/created', session => {
    if (session.header.origin === 'subagent') tree.notify('subagent.started', { parentSessionId: session.header.parentSession, childSessionId: session.id });
  });
  if (!sdk) ctx.on('session/event', (session, event) => { try { tree.notify('session.event', { sessionId: session.id, event }); } catch (error) { observedErrors.push(error); } });
  if (!sdk) ctx.on('subagent/end', info => {
    if (info.local) tree.notify('subagent.finished', { parentSessionId: info.parentSessionId, childSessionId: info.id, stopReason: info.stopReason });
  });
  const policy = await createNativePolicy(ctx, config);
  ctx.on('agent/created', async ({ agent }) => { admissions.push({ id: agent.id, receipt: await policy.admit(agent, agent.id === 'root') }); });
  ctx.on('agent/pre-step', async ({ agent }, next) => {
    const result = await next(); steps.push({ id: agent.id, tools: (await ctx.systemPrompt.assemble({ scope: agent })).tools.map(tool => tool.name), team: ctx.agentTeams.isEnabled(agent) }); return result;
  });
  const fold = new SessionFold('root', { strict: true });
  let root, client, exitPromise;
  const notifications = [];
  if (sdk) {
    const requestBytes = new PassThrough(), responseBytes = new PassThrough();
    let exited;
    exitPromise = new Promise(resolve => { exited = resolve; });
    client = new JsonRpcLineTransport(responseBytes, requestBytes);
    client.onNotification((method, params) => {
      notifications.push({ method, params });
      try { if (method === 'session.event') fold.accept(params); tree.notify(method, params); }
      catch (error) { observedErrors.push(error); }
    });
    client.start();
    t.after(() => { client.close(); requestBytes.destroy(); responseBytes.destroy(); });
    SdkServer.apply(ctx, { input: requestBytes, output: responseBytes, exit: exited, maxTokensAsSuccess: false });
    const initialized = await client.request('initialize', { cwd: directory, provider: 'offline', model: 'vision' }, signal());
    assert.equal(initialized.serverInfo.name, 'deepseek-harness-sdk-runtime');
    await client.request('session/working-directory/get', { sessionId: 'root' }, signal());
    root = ctx.agents.get('root');
  } else root = await driver.create('root', { provider: 'offline', model: 'vision' }, { cwd: directory });
  let callId = 0;
  async function program(code, agent = root) {
    const inputSignal = signal();
    const toolSet = await ctx.tools.resolve({ agent, modelSelection: agent.model.current(), signal: inputSignal });
    let batch;
    try { batch = await ctx.tools.execute({ agent, toolSet, signal: inputSignal, calls: [{ type: 'tool-call', id: `flow-${++callId}`, name: 'run_code', arguments: JSON.stringify({ code, description: 'Run deterministic native integration fixture' }) }] }); }
    finally { toolSet.dispose(); }
    const result = batch.results[0].result;
    assert.equal(result.isError, false, JSON.stringify(batch));
    return result.content.filter(block => block.type === 'text').map(block => block.text).join('\n');
  }
  async function runtimeEvents() {
    await agentEvents(ctx, root).serial('agent/turn-stopping', { turn: 1, signal: new AbortController().signal });
    const events = (await readFile(join(directory, 'native-events.jsonl'), 'utf8')).trim().split('\n').filter(Boolean).map(line => JSON.parse(line));
    tree.observeRuntime(events);
    assert.deepEqual(observedErrors, []);
    return events;
  }
  return { ctx, root, config, policy, tree, model, directory, program, runtimeEvents, steps, admissions, client, exitPromise, fold, notifications, observedErrors };
}

test('real rc.3 Team mode and all task tools operate through the actual PTC policy composition', async t => {
  const f = await fixture(t);
  assert.equal(f.admissions[0].receipt.matched, true);
  assert.equal(f.ctx.agentTeams.isEnabled(f.root), true, 'admission enables the root through the public persisted Team API');
  const output = JSON.parse(await f.program(`
const task = await tools.team_task_create({subject:'Verify native composition',description:'Deterministic fixture task'});
const claimed = await tools.team_task_update({task_id:task.id,expected_revision:task.revision,action:'claim'});
const completed = await tools.team_task_update({task_id:task.id,expected_revision:claimed.revision,action:'complete'});
const found = await tools.team_task_get({task_id:task.id});
const listed = await tools.team_task_list({});
console.log(JSON.stringify({completed,found,listed}));`));
  assert.equal(output.completed.status, 'completed');
  assert.equal(output.found.id, output.completed.id);
  assert.ok(JSON.stringify(output.listed).includes(output.completed.id));
  assert.ok(f.root.session.ownEvents().some(event => event.type === 'team/mode' && event.data.enabled === true));
  assert.equal(f.model.calls.length, 0, 'Team tool dispatch itself is offline');
  assert.equal(f.root.session.header.agentPreset, undefined);
  await f.runtimeEvents();
});

test('actual spawn and fork providers inherit PTC/Team, produce owned lifecycle and contribute only committed usage', async t => {
  const f = await fixture(t);
  for (const provider of ['spawn', 'fork']) {
    const activation = await f.ctx.subagents.startActivation({ provider, parent: f.root, delivery: 'caller',
      prompt: [{ type: 'text', text: 'Run the offline child fixture' }], label: 'Offline integration', signal: signal() });
    const result = await activation.result;
    assert.equal(result.stopReason, 'completed');
    await activation.dispose();
  }
  const events = await f.runtimeEvents();
  assert.equal(events.filter(event => event.type === 'subagent.started').length, 2);
  assert.equal(events.filter(event => event.type === 'subagent.finished').length, 2);
  assert.equal(f.admissions.length, 3);
  assert.ok(f.steps.filter(step => step.id !== 'root').every(step => step.team && step.tools.length === 1 && step.tools[0] === 'run_code'));
  assert.deepEqual(f.tree.completion(), { confirmed: true, pendingIds: [] });
  const totals = f.tree.addUsage({ input_tokens: 10, output_tokens: 5, reasoning_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0, total_tokens: 15 });
  assert.equal(totals.input_tokens, 16); assert.equal(totals.output_tokens, 9);
  assert.ok(!JSON.stringify(events).includes('offline child result'));
  assert.ok(!JSON.stringify(f.tree.receipt({}, {})).includes('offline child result'));
});

test('send_message cold-resumes the same owned child with a new live Agent and fresh admission', async t => {
  const f = await fixture(t);
  const activation = await f.ctx.subagents.startActivation({ provider: 'spawn', parent: f.root, delivery: 'caller',
    prompt: [{ type: 'text', text: 'Initial child turn' }], label: 'Continuable fixture', signal: signal() });
  await activation.result;
  await activation.dispose();
  const before = f.admissions.filter(row => row.id === activation.childId).length;
  await f.ctx.subagents.sendMessage(f.root, activation.childId, [{ type: 'text', text: 'Continue the owned child' }], { signal: signal() });
  await f.ctx.subagents.waitForTreeWork(f.root);
  await f.root.whenIdle();
  assert.equal(f.admissions.filter(row => row.id === activation.childId).length, before + 1);
  const events = await f.runtimeEvents();
  assert.equal(events.filter(row => row.type === 'subagent.started').length, 2);
  assert.equal(f.tree.receipt({}, {}).children.length, 1);
  assert.equal(f.tree.completion().confirmed, true);
});

test('local depth admission also covers a workflow-style start that omits the optional provider cap', async t => {
  const f = await fixture(t, { maxDepth: 1, hold: true });
  const activation = await f.ctx.subagents.startActivation({ provider: 'spawn', parent: f.root, delivery: 'caller',
    prompt: [{ type: 'text', text: 'Keep this parent resident' }], label: 'Depth fixture', signal: signal() });
  await f.model.entered.promise;
  const child = f.ctx.agents.get(activation.childId);
  assert.ok(child);
  await assert.rejects(() => f.ctx.subagents.startActivation({ provider: 'spawn', parent: child, delivery: 'caller',
    prompt: [{ type: 'text', text: 'This second depth was not admitted' }], label: 'Denied depth', signal: signal() }), /delegation depth/);
  assert.equal(f.model.calls.length, 1, 'no rejected child reaches the model');
  f.model.gate.resolve();
  await activation.result;
  await activation.dispose();
  const events = await f.runtimeEvents();
  const rejection = events.find(event => event.type === 'agent.admission-rejected');
  assert.equal(rejection.beforeModelDispatch, true);
  assert.equal(rejection.parentId, activation.childId);
  assert.equal(f.tree.completion().confirmed, true);
  const attempt = f.tree.receipt({}, {}).children.find(value => value.id === rejection.id);
  assert.equal(attempt.state, 'admission-rejected');
  assert.equal(attempt.usage.total_tokens, 0);
});

test('real model-facing delegation tools dispatch children without giving them a separate user-selected route', async t => {
  const f = await fixture(t);
  const result = JSON.parse(await f.program(`
const spawned = await tools.subagent({name:'spawned',description:'Offline spawn fixture',prompt:'Complete the deterministic fixture'});
const forked = await tools.subagent_fork({name:'forked',description:'Offline fork fixture',prompt:'Complete the deterministic fixture'});
console.log(JSON.stringify([spawned,forked]));`));
  assert.deepEqual(result.map(row => row.kind), ['activation', 'activation']);
  await f.ctx.subagents.waitForTreeWork(f.root);
  await f.root.whenIdle();
  const events = await f.runtimeEvents();
  assert.equal(events.filter(event => event.type === 'subagent.started').length, 2);
  assert.equal(f.tree.completion().confirmed, true);
  assert.ok(f.model.calls.every(call => call.provider === 'offline' && call.model === 'vision'));
});

test('actual external provider epochs are observed without task text; pending and unmetered work remain explicit', async t => {
  const f = await fixture(t);
  let settle;
  const outcome = new Promise(resolve => { settle = resolve; });
  let disposed = false;
  f.ctx.subagents.registerProvider({ name: 'external-fixture', inheritsParentContext: false,
    capabilities: { agentOptions: false, agentPreset: false, outputSchema: false, depthLimit: false },
    start: async () => ({ id: 'external-child', result: outcome, dispose: async () => { disposed = true; } }),
  });
  const activation = await f.ctx.subagents.startActivation({ provider: 'external-fixture', parent: f.root, delivery: 'caller',
    prompt: [{ type: 'text', text: 'private external task text' }], label: 'External fixture', signal: signal() });
  await f.runtimeEvents();
  const atRootEnd = f.tree.completion();
  assert.deepEqual(atRootEnd, { confirmed: false, pendingIds: ['external-child'] });
  settle({ stopReason: 'completed', output: [{ type: 'text', text: 'private external answer' }] });
  await activation.result;
  const events = await f.runtimeEvents();
  assert.equal(disposed, true);
  assert.equal(f.tree.completion().confirmed, true);
  assert.equal(atRootEnd.confirmed, false);
  assert.equal(f.tree.addUsage({ input_tokens: 0, output_tokens: 0, reasoning_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0, total_tokens: 0 }), null);
  assert.ok(!JSON.stringify(events).includes('private external'));
  assert.equal(events.find(event => event.type === 'subagent.started').child.local, false);
});

test('real SDK JSON-RPC initialization, creation, prompt, native child dispatch and shutdown retain complete receipts', async t => {
  const f = await fixture(t, { sdk: true });
  assert.equal(f.model.calls.length, 0, 'native admission and its PTC probe precede model inference');
  const prompt = await f.client.request('session/prompt', { sessionId: 'root', contentBlocks: [{ type: 'text', text: 'sdk root fixture' }] }, signal());
  await f.client.request('session/wait', { sessionId: 'root' }, signal());
  const result = f.fold.finish(prompt.messageId);
  assert.equal(result.text, 'SDK root final answer');
  const events = await f.runtimeEvents();
  const completionAtRootEnd = f.tree.completion();
  assert.deepEqual(completionAtRootEnd, { confirmed: true, pendingIds: [] });
  assert.equal(f.tree.receipt({}, {}).children.length, 1);
  const usage = f.tree.addUsage(result.usage);
  assert.equal(usage.input_tokens, result.usage.input_tokens + 3);
  assert.equal(usage.output_tokens, result.usage.output_tokens + 2);
  assert.ok(f.notifications.some(row => row.method === 'subagent.started'));
  assert.ok(f.notifications.some(row => row.method === 'subagent.finished'));
  await f.client.request('shutdown', {}, signal());
  assert.equal(await f.exitPromise, 0);
  const cleanup = JSON.parse(await readFile(join(f.directory, 'native-cleanup.json'), 'utf8'));
  const journal = await readFile(join(f.directory, 'native-events.jsonl'));
  assert.equal(cleanup.journal.complete, true);
  assert.equal(cleanup.journal.bytes, journal.length);
  assert.equal(cleanup.journal.events, events.length);
  assert.equal(cleanup.requestId, f.config.requestId);
  assert.equal(cleanup.nonce, f.config.nonce);
  assert.deepEqual(f.observedErrors, []);
});
