import test, { after } from 'node:test';
import { loadClient, closeClientHarness } from './support/client-harness.js';
after(closeClientHarness);
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { resolve } from 'node:path';
import { tmpdir } from 'node:os';
const hostRequire = createRequire(process.env.FLYWHEEL_TEST_DSH_HOST_DIR
  ? resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR, 'package.json') : new URL('../package.json', import.meta.url));
const load = name => import(pathToFileURL(hostRequire.resolve(name)));
const { Context } = await load('@deepseek-ai/cordis');
const { default: TypertRegistry } = await load('@deepseek-ai/dsh-typert-registry');
const { default: Gateway } = await load('@deepseek-ai/dsh-api-gateway');
const GatewayClient = await load('@deepseek-ai/dsh-api-gateway/client');
const { SessionStore } = await load('@deepseek-ai/dsh-session');
const { default: AgentRegistry } = await load('@deepseek-ai/dsh-agent');
const { SessionProjectionRegistry } = await load('@deepseek-ai/dsh-session-projection');
const source = name => process.env.FLYWHEEL_TEST_DSH_PACKAGE_DIR
  ? pathToFileURL(resolve(process.env.FLYWHEEL_TEST_DSH_PACKAGE_DIR, 'dist', name.replace(/\.js$/, '.mjs'))) : process.env.FLYWHEEL_TEST_DSH_HOST_DIR
  ? pathToFileURL(resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR, name)) : new URL('../src/' + name, import.meta.url);
const { ConsoleReadService } = await import(source('console-read.js'));
const { taskAssociationProjection } = await import(source('task-association.js'));
const { ControlNotDispatchedError } = await import(source('control.js'));
import { mkdtemp, realpath, rm } from 'node:fs/promises';

test('published Client and Host Gateway reads are session-bound and append no conversation events', async t => {
  const ctx = new Context();
  t.after(() => ctx.fiber.dispose());
  await ctx.plugin(TypertRegistry);
  ctx.provide('remoteAccess', { decide: () => ({ kind:'deny', reason:'fixture-deny' }) });
  await ctx.plugin(Gateway);
  await ctx.plugin(SessionStore);
  await ctx.plugin(SessionProjectionRegistry);
  await ctx.sessionProjections.register(taskAssociationProjection);
  const cwd = await realpath(process.cwd());
  const unrelatedTask = await realpath(await mkdtemp(resolve(tmpdir(), 'flywheel-unassociated-')));
  t.after(() => rm(unrelatedTask, { recursive: true, force: true }));
  const session = ctx.sessions.create('mine', { meta: { cwd } });
  const agent = { id: session.id, session, ctx, status: 'idle', options: {}, whenIdle: () => Promise.resolve() };
  await ctx.plugin(AgentRegistry);
  await ctx.agents.register(agent);
  const before = session.snapshotEvents();
  ctx.provide('workingDirectory', { get: () => cwd });
  const calls = [];
  const control = async (input, actual, signal) => {
    assert.equal(actual, agent); assert.ok(signal instanceof AbortSignal);
    calls.push(input);
    return { protocol: 1, ok: true, running: false, state: null, taskDir: input.taskDir ?? '', cwd,
      ...(input.action === 'config-edit' ? { editor: { scope: input.editor.scope, revision: 'fixture-revision', values: {}, defaults: {} } } : {}),
      ...(input.action === 'runs' ? { runs: [ { hostSessionId: 'foreign', taskDir: cwd, cwd }, { hostSessionId: 'mine', taskDir: cwd, cwd } ] } : {}) };
  };
  new ConsoleReadService(ctx, control, (input, error) => ({ protocol: 1, type: 'flywheel-control-error', action: input?.action, dispatch: error instanceof ControlNotDispatchedError ? 'not-dispatched' : 'unknown', message: error.message }));
  const peer = { id:'test', ctx, principal: { kind: 'owner' }, signal:new AbortController().signal, dispose:async()=>{} };
  // Exercise the shipped Client descriptor through the real public Client
  // mount, not a mock $mount that accepts unsupported src-json inputs.
  const clientCtx = new Context();
  t.after(() => clientCtx.fiber.dispose());
  await clientCtx.plugin(TypertRegistry);
  clientCtx.provide('connection', {
    rpc: {
      call: async (route, endpoint, payload, signal) => {
        assert.equal(route, '/api'); assert.equal(endpoint, 'flywheelConsole/read');
        // The browser carrier crosses a JSON boundary between Client/Host realms.
        return { ok: true, value: await ctx.typertGateway.invoke({ namespace: 'flywheelConsole', method: 'read', args: JSON.parse(JSON.stringify(payload.args)), peer: { ...peer, signal } }) };
      },
      open: async function* () {},
    },
    start: () => ({ stop() {} }), registerGenerationSource: () => () => {},
  });
  await clientCtx.plugin(GatewayClient);
  let definition;
  clientCtx.provide('locale', { register: () => () => {}, bind: () => key => key });
  clientCtx.provide('slots', { inject: (_name, register) => register(), register: value => { definition = value; return () => {}; } });
  clientCtx.provide('remote.commands', { execute() { throw new Error('Passive reads must never execute chat commands'); } });
  const clientFiber = clientCtx.plugin(loadClient());
  await clientFiber;
  const clientView = definition.inject('mine', { setSelection() {} });
  await clientView.discover();
  assert.equal(clientView.hooks.control.getSnapshot().runsError, null, 'the actual plugin context can read its mounted Remote namespace');
  await clientView.run('config-edit', { taskDir: '', cwd }, false, { editor: { scope: 'global' } });
  assert.equal(clientView.hooks.control.getSnapshot().error, null);
  assert.equal(clientView.hooks.control.getSnapshot().editor.scope, 'global');
  const descriptor = clientCtx.typert.remotes.get('flywheelConsole/read');
  assert.equal(descriptor.parameters[0].wire, ctx.typert.lookups.get('agent').wire);
  const sessionSchema = descriptor.parameters[0].codec.create();
  const inputSchema = descriptor.parameters[1].codec.create();
  const resultSchema = descriptor.result.create();
  for (const value of ['', '  ', null, 1, []]) assert.throws(() => sessionSchema.parse(value));
  for (const input of [
    { action: 'runs' }, { action: 'status', taskDir: cwd, cwd }, { action: 'config' },
    { action: 'config-edit', editor: { scope: 'global' } }, { action: 'config-edit', editor: { scope: 'project' } },
    { action: 'metrics', runId: 'run-1' }, { action: 'history', sessionKey: 'task:review' },
    { action: 'node', packet: 'M01-T01' }, { action: 'delivery', delivery: { action: 'inspect', headBranch: 'feature/example' } },
    { action: 'doctor', probe: false },
  ]) assert.deepEqual(JSON.parse(JSON.stringify(inputSchema.parse(input))), input);
  for (const input of [null, [], {}, { action: 'start' }, { action: 'runs', unknown: true },
    { action: 'runs', cwd: null }, { action: 'runs', probe: true }, { action: 'doctor', probe: 'yes' },
    { action: 'config-edit', editor: {} }, { action: 'config-edit', editor: { scope: 'global', values: {} } },
    { action: 'runs', editor: { scope: 'global' } }, { action: 'delivery', delivery: { action: 'merge' } },
    { action: 'runs', delivery: { action: 'inspect' } }, { action: 'node', packet: '../foreign' },
    { action: 'runs', packet: 'M01-T01' }, { action: 'history' }, { action: 'runs', sessionKey: 'foreign' },
    { action: 'metrics', runId: 'bad\nID' }, { action: 'runs', runId: 'run-1' },
  ]) assert.throws(() => inputSchema.parse(input), JSON.stringify(input));
  for (const kind of ['success', 'error']) assert.equal(resultSchema.parse({ result: { kind, text: '{}' } }).result.kind, kind);
  for (const value of [null, {}, { result: { kind: 'unknown', text: '{}' } }, { result: { kind: 'success', text: 1 } }]) {
    assert.throws(() => resultSchema.parse(value)); assert.throws(() => descriptor.result.decode(value));
  }
  const read = async input => {
    const result = await clientCtx.remote.flywheelConsole.read('mine', input);
    if (!result.ok) throw result.error;
    return result.value;
  };
  for (let i=0; i<3; i++) {
    const result = await read({ action:'runs' });
    assert.deepEqual(JSON.parse(result.result.text).runs.map(r => r.hostSessionId), ['mine']);
  }
  assert.equal((await read({ action:'status', taskDir:cwd, cwd })).result.kind,'success');
  const count = calls.length;
  for (const action of ['start', 'launch', 'stop', 'step', 'session', 'deliver', 'config-save', 'investigate']) {
    assert.equal((await read({ action, taskDir:cwd, cwd })).result.kind,'error');
  }
  assert.equal(calls.length,count,'read endpoint never dispatches a mutation');
  const denied = await read({ action:'status', taskDir:unrelatedTask, cwd });
  assert.match(denied.result.text,/not associated/);
  assert.equal(calls.at(-1).action,'runs','unrelated task status never reaches Go');
  await assert.rejects(ctx.typertGateway.invoke({ namespace:'flywheelConsole', method:'read', args:{agentId:'wrong',input:{action:'runs'}},peer }));
  assert.deepEqual(session.snapshotEvents(),before,'all passive reads leave session log untouched');
  const historicalTask = await realpath(await mkdtemp(resolve(tmpdir(), 'flywheel-history-')));
  t.after(() => rm(historicalTask, { recursive: true, force: true }));
  for (const [id, taskDir] of [['history', historicalTask], ['current', cwd]]) {
    session.append('command/run', { commandId: id, name: 'flywheel', args: JSON.stringify({ action: 'launch', taskDir, cwd }), source: { kind: 'user' } });
    session.append('command/done', { commandId: id, kind: 'success', text: JSON.stringify({ protocol: 1, ok: true, running: false, taskDir, cwd, state: null }) });
  }
  const historyBefore = session.snapshotEvents();
  assert.equal((await read({ action: 'status', taskDir: historicalTask, cwd })).result.kind, 'success', 'confirmed preparation history does not require a run index');
  assert.equal((await read({ action: 'status', taskDir: historicalTask, cwd: unrelatedTask })).result.kind, 'error', 'history authorizes the exact task and execution root pair');
  assert.deepEqual(session.snapshotEvents(), historyBefore, 'browsing history does not rebind the conversation or append messages');
  session.append('command/run', { commandId: 'fixture-pending', name: 'flywheel', args: JSON.stringify({ action: 'launch', taskDir: unrelatedTask, cwd }), source: { kind: 'user' } });
  const pendingBefore = session.snapshotEvents();
  assert.equal((await read({ action:'status',taskDir:unrelatedTask,cwd })).result.kind,'success','original pending target remains readable');
  assert.deepEqual(session.snapshotEvents(), pendingBefore);
  const countBeforeDispose = calls.length;
  await clientFiber.dispose();
  await clientView.discover();
  assert.equal(calls.length, countBeforeDispose, 'a disposed plugin cannot keep its read scope alive');
  assert.equal(clientCtx.typert.remotes.get('flywheelConsole/read'), undefined);
});
