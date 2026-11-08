import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { createRequire } from 'node:module';
import { apply, inject } from '../src/index.js';

const hostRequire = createRequire(process.env.FLYWHEEL_TEST_DSH_HOST_DIR
  ? resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR, 'package.json') : new URL('../package.json', import.meta.url));
const imported = async name => import(pathToFileURL(hostRequire.resolve(name)));
const [{ Context }, { SessionStore }, { SessionProjectionRegistry }, Tools] = await Promise.all([
  imported('@deepseek-ai/cordis'), imported('@deepseek-ai/dsh-session'),
  imported('@deepseek-ai/dsh-session-projection'), imported('@deepseek-ai/dsh-tools'),
]);
const modern = typeof Tools.ToolsService === 'function';
const binary = fileURLToPath(new URL('./fixtures/fake-control.js', import.meta.url));

async function runtimeFixture(t, mode, flywheelBinary = binary) {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-runtime-')));
  const ctx = new Context();
  t.after(async () => { await ctx.fiber.dispose(); await rm(cwd, { recursive: true, force: true }); });
  await ctx.plugin(SessionStore);
  await ctx.plugin(SessionProjectionRegistry);
  // Prompt assembly is outside this test; these are the public registration
  // seats used by the real tool registry and executor plugins.
  ctx.provide('systemPrompt', { tools: () => () => {}, section: () => () => {}, getSectionOrder: () => 100 });
  await ctx.plugin(Tools.default, modern ? {} : { mode });
  ctx.provide('commands', { register() {} });
  ctx.provide('workingDirectory', { get: session => session.header.cwd, ensure: async agent => agent.session.header.cwd });
  let nested = { name: 'flywheel_launch', args: { taskDir: cwd } };
  ctx.provide('ptcRuntime', {
    language: 'typescript', isolation: 'fixture', programFormat: 'function-body', retainsState: false,
    resolve: spec => spec, release() {},
    async run(spec) {
      // Exercise the published dispatcher's actual binding, scheduler, tool
      // body, finalizer and durable event logging, without executing user code
      // or involving a model. The program runtime is a separate public seam.
      try { return { logs: [], value: await spec.bindings[0].functions[nested.name](nested.args) }; }
      catch (error) { return { logs: [], error: { kind: 'program', message: error.message } }; }
    },
  });
  if (modern) {
    const Executor = mode === 'ptc'
      ? (await imported('@deepseek-ai/dsh-tool-executor-ptc')).PtcExecutor
      : (await imported('@deepseek-ai/dsh-tool-executor-native')).NativeToolExecutor;
    ctx.tools.setExecutor(new Executor(ctx));
  }
  await ctx.plugin({ apply, inject }, { flywheelBinary });
  const session = ctx.sessions.create('runtime-session', { meta: { cwd } });
  const agent = { session, ctx };
  const settled = [];
  ctx.on('tools/result', (execution, result) => { settled.push({ execution, result }); });
  let seq = 0;
  const invoke = async (name, args, signal = new AbortController().signal) => {
    const id = 'root-' + seq++;
    if (modern) {
      const batch = await ctx.tools.execute({ calls: [{ type: 'tool-call', id, name, arguments: JSON.stringify(args) }],
        agent, signal, log: { turn: 0, step: seq } });
      return batch.results[0].result;
    }
    // rc.5 logs native calls in agent-loop; its PTC dispatcher still owns and
    // emits every nested durable call/result itself.
    return ctx.tools.execute({ callId: id, name, arguments: args, agent, signal });
  };
  return { ctx, session, cwd, invoke, settled, setNested: value => { nested = value; },
    view: () => ctx.sessionProjections.snapshot(session).values.flywheelTask };
}

test('published PTC dispatchers use the same subCallId for execution, finalization and durable task association', async t => {
  const f = await runtimeFixture(t, 'ptc');
  const first = await f.invoke('run_code', { code: 'fixture', description: 'Launch the prepared task' });
  assert.equal(first.isError, false, JSON.stringify(first));
  const nested = f.settled.find(value => value.execution.name === 'flywheel_launch');
  assert.ok(nested);
  const start = f.session.snapshotEvents().find(event => event.type === 'tool/ptc-dispatch-start');
  const done = f.session.snapshotEvents().find(event => event.type === 'tool/ptc-dispatch');
  assert.equal(nested.execution.callId, start.data.subCallId);
  assert.equal(done.data.subCallId, start.data.subCallId);
  assert.equal(f.view().operationId, 'tool:' + nested.execution.callId);
  assert.equal(f.view().taskDir, f.cwd);
  assert.equal(f.view().outcome, 'confirmed');
  const previous = process.env.TEST_CONTROL_MODE;
  process.env.TEST_CONTROL_MODE = 'invalid-json';
  t.after(() => { if (previous === undefined) delete process.env.TEST_CONTROL_MODE; else process.env.TEST_CONTROL_MODE = previous; });
  const failure = await f.invoke('run_code', { code: 'fixture', description: 'Exercise an unconfirmed response' });
  assert.equal(failure.isError, true);
  const failedNested = f.settled.findLast(value => value.execution.name === 'flywheel_launch');
  assert.equal(JSON.parse(failedNested.result.content[0].text).dispatch, 'unknown');
  assert.equal(f.view().outcome, 'unknown', 'a real PTC tool failure preserves the original task identity');
  process.env.TEST_CONTROL_MODE = '';
  f.setNested({ name: 'flywheel_control', args: { action: 'status', taskDir: f.cwd } });
  await f.invoke('run_code', { code: 'fixture', description: 'Reconcile the original task' });
  assert.equal(f.view().outcome, 'confirmed');
});

test('published native tool pipelines preserve finalizer identity and positive pre-dispatch proof', async t => {
  const f = await runtimeFixture(t, 'native', '/missing/flywheel');
  const result = await f.invoke('flywheel_launch', { taskDir: f.cwd });
  assert.equal(result.isError, true);
  assert.equal(JSON.parse(result.content[0].text).dispatch, 'not-dispatched',
    'the definition execute/finalize hooks share the same WeakMap-keyable execution identity');
  const cancelled = await f.invoke('flywheel_launch', { taskDir: f.cwd }, AbortSignal.abort());
  assert.equal(cancelled.isError, true);
  // rc.1's batch scheduler records a skipped call without entering the
  // definition pipeline; its structured error code is the public proof.
  if (modern) assert.equal(cancelled.error.info.code, 'ABORTED_BEFORE_DISPATCH');
  else assert.equal(JSON.parse(cancelled.content[0].text).dispatch, 'not-dispatched');
  if (modern) assert.equal(f.view().outcome, 'confirmed', 'rc.1 native executor logs reach the same session projection');
});
