import test from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { resolve } from 'node:path';
const hostRequire = createRequire(process.env.FLYWHEEL_TEST_DSH_HOST_DIR
  ? resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR, 'package.json') : new URL('../package.json', import.meta.url));
const load = name => import(pathToFileURL(hostRequire.resolve(name)));
const { Context } = await load('@deepseek-ai/cordis');
const { default: TypertRegistry } = await load('@deepseek-ai/dsh-typert-registry');
const { default: Gateway } = await load('@deepseek-ai/dsh-api-gateway');
const { SessionStore } = await load('@deepseek-ai/dsh-session');
const source = name => process.env.FLYWHEEL_TEST_DSH_HOST_DIR
  ? pathToFileURL(resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR, name)) : new URL('../src/' + name, import.meta.url);
const { ConsoleReadService } = await import(source('console-read.js'));
const { ControlNotDispatchedError } = await import(source('control.js'));
import { realpath } from 'node:fs/promises';

test('public Gateway reads are session-bound and append no conversation events', async t => {
  const ctx = new Context();
  t.after(() => ctx.fiber.dispose());
  await ctx.plugin(TypertRegistry);
  ctx.provide('remoteAccess', { decide: () => ({ kind:'deny', reason:'fixture-deny' }) });
  await ctx.plugin(Gateway);
  await ctx.plugin(SessionStore);
  const cwd = await realpath(process.cwd());
  const session = ctx.sessions.create('mine', { meta: { cwd } });
  const agent = { session };
  ctx.typert.lookups.register('agent', { parameter: 'agent', wire: 'sessionId', hostTypeSymbol: 'fixture#Agent', wireTypeSymbol: 'fixture#SessionId', resolve: id => id === session.id ? agent : undefined });
  let view = null;
  ctx.provide('sessionProjections', { stateOf: () => ({ view }) });
  ctx.provide('workingDirectory', { get: () => cwd });
  const calls = [];
  const control = async (input, actual, signal) => {
    assert.equal(actual, agent); assert.ok(signal instanceof AbortSignal);
    calls.push(input);
    return { protocol: 1, ok: true, running: false, state: null, taskDir: input.taskDir ?? '', cwd,
      ...(input.action === 'runs' ? { runs: [ { hostSessionId: 'foreign', taskDir: cwd, cwd }, { hostSessionId: 'mine', taskDir: cwd, cwd } ] } : {}) };
  };
  new ConsoleReadService(ctx, control, (input, error) => ({ protocol: 1, type: 'flywheel-control-error', action: input?.action, dispatch: error instanceof ControlNotDispatchedError ? 'not-dispatched' : 'unknown', message: error.message }));
  const peer = { id:'test', ctx, principal: { kind: 'owner' }, signal:new AbortController().signal, dispose:async()=>{} };
  const read = input => ctx.typertGateway.invoke({ namespace:'flywheelConsole', method:'read', args:{ sessionId:'mine', input }, peer });
  const before = session.snapshotEvents();
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
  const denied = await read({ action:'status', taskDir:'/private/tmp', cwd });
  assert.match(denied.result.text,/not associated/);
  assert.equal(calls.at(-1).action,'runs','unrelated task status never reaches Go');
  await assert.rejects(ctx.typertGateway.invoke({ namespace:'flywheelConsole', method:'read', args:{sessionId:'wrong',input:{action:'runs'}},peer }));
  view = { taskDir:'/private/tmp', cwd, outcome:'unknown' };
  assert.equal((await read({ action:'status',taskDir:'/private/tmp',cwd })).result.kind,'success','original unresolved target remains readable');
  assert.deepEqual(session.snapshotEvents(),before,'all passive reads leave session log untouched');
});
