import test from 'node:test';
import assert from 'node:assert/strict';
import { chmod, mkdir, mkdtemp, readFile, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { createRequire } from 'node:module';
import { apply, inject } from '../src/index.js';
import { taskProjectionKey } from '../src/task-association.js';

// Exercise the public registry and Session implementation from both supported
// published hosts. The fixture records their documented command/PTC vocabulary;
// the production plugin never appends an event of its own.
const hostRequire = createRequire(process.env.FLYWHEEL_TEST_DSH_HOST_DIR
  ? resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR, 'package.json') : new URL('../package.json', import.meta.url));
const { Context } = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/cordis')));
const { SessionStore } = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/dsh-session')));
const { SessionProjectionRegistry } = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/dsh-session-projection')));
const binary = fileURLToPath(new URL('./fixtures/fake-control.js', import.meta.url));
await chmod(binary, 0o755);

function env(t, key, value) {
  const previous = process.env[key];
  process.env[key] = value;
  t.after(() => { if (previous === undefined) delete process.env[key]; else process.env[key] = previous; });
}

async function fixture(t, config = {}) {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-projected-host-')));
  const first = join(cwd, 'first');
  const second = join(cwd, 'second');
  await Promise.all([mkdir(first), mkdir(second)]);
  const ctx = new Context();
  t.after(async () => { await ctx.fiber.dispose(); await rm(cwd, { force: true, recursive: true }); });
  await ctx.plugin(SessionStore);
  await ctx.plugin(SessionProjectionRegistry);
  const tools = new Map();
  let command;
  ctx.provide('commands', { register: value => { command = value; } });
  ctx.provide('tools', { register: value => { tools.set(value.name, value); } });
  ctx.provide('workingDirectory', { get: session => session.header.cwd ?? cwd });
  await ctx.plugin({ apply, inject }, { flywheelBinary: binary, ...config });
  const session = ctx.sessions.create('desktop-session', { meta: { cwd } });
  const other = ctx.sessions.create('other-session', { meta: { cwd } });
  let seq = 0;
  const view = (target = session) => ctx.sessionProjections.snapshot(target).values[taskProjectionKey];
  const runCommand = async (input, target = session) => {
    const commandId = 'command-' + seq++;
    target.append('command/run', { commandId, name: 'flywheel', args: JSON.stringify(input), source: { kind: 'user' } });
    const value = await command.handler({ commandId, rawInput: JSON.stringify(input), agent: { session: target } });
    target.append('command/done', { commandId, ...value });
    return value;
  };
  const runTool = async (name, args, target = session, signal = new AbortController().signal) => {
    const definition = tools.get(name);
    const callId = 'root:ptc:' + seq++;
    const dispatch = { name, arguments: args, rootCallId: 'root', parentCallId: 'root', subCallId: callId };
    target.append('tool/ptc-dispatch-start', dispatch);
    const execution = { callId, rootCallId: 'root', token: Symbol(), name, arguments: args,
      agent: { session: target }, signal };
    let result;
    try {
      const value = await definition.execute(args, execution);
      result = { isError: false, value, content: definition.output.render(args, value) };
    } catch (error) {
      result = { isError: true, error: { message: error.message }, content: [{ type: 'text', text: 'Error: ' + error.message }] };
    }
    result.content = definition.finalizeContent?.(execution, result) ?? result.content;
    target.append('tool/ptc-dispatch', { ...dispatch, isError: result.isError, content: result.content });
    return result;
  };
  return { ctx, session, other, cwd, first, second, tools, view, runCommand, runTool };
}

test('published session projections bind slash and tool launches, survive replay and preserve session isolation', async t => {
  const f = await fixture(t);
  assert.equal(f.view(), null);
  const result = await f.runTool('flywheel_launch', { taskDir: f.first });
  assert.equal(result.isError, false);
  assert.equal(f.view().taskDir, f.first);
  assert.equal(f.view().cwd, f.cwd);
  assert.equal(f.view().outcome, 'confirmed');
  assert.equal(f.view(f.other), null);
  const check = f.ctx.sessionProjections.checkpoint(f.session);
  const events = f.session.snapshotEvents();
  const restored = f.ctx.sessionProjections.restore(check, events, 0, f.session.header, 0);
  assert.deepEqual(restored.snapshot.values[taskProjectionKey], f.view());
  const replay = f.ctx.sessions.create('restored-session', { seed: events, meta: { cwd: f.cwd } });
  assert.deepEqual(f.view(replay), f.view(), 'cold replay restores the exact task without probing directories');
  const fork = f.ctx.sessions.create('forked-session', { seed: events,
    inheritedEventCount: events.length, meta: { cwd: f.cwd, isSeeded: true } });
  assert.equal(f.view(fork), null, 'inherited events do not bind a parent task to a new fork');
  const fromCommand = await f.runCommand({ action: 'launch', taskDir: f.second, mode: 'full' });
  assert.equal(fromCommand.kind, 'success');
  assert.equal(f.view().taskDir, f.second);
  assert.equal(f.tools.get('flywheel_launch').isConcurrencySafe({ taskDir: f.first }), false);
});

test('a host directory fallback without logged cwd cannot dispatch mutations until an explicit directory is supplied', async t => {
  const f = await fixture(t);
  const session = f.ctx.sessions.create('without-recorded-directory');
  assert.equal(session.header.cwd, undefined);
  assert.equal(f.ctx.workingDirectory.get(session), f.cwd, 'the host getter has a usable fallback outside the durable session facts');
  const log = join(f.cwd, 'dispatch.jsonl');
  env(t, 'TEST_CONTROL_LOG', log);
  for (const action of ['launch', 'start', 'stop', 'launch']) {
    const result = await f.runCommand({ action, taskDir: f.first }, session);
    const failure = JSON.parse(result.text);
    assert.equal(result.kind, 'error');
    assert.equal(failure.dispatch, 'not-dispatched');
    assert.match(failure.message, /explicit absolute cwd/);
  }
  for (let attempt = 0; attempt < 2; attempt += 1) {
    const result = await f.runTool('flywheel_launch', { taskDir: f.first }, session);
    assert.equal(result.isError, true);
    assert.equal(JSON.parse(result.content[0].text).dispatch, 'not-dispatched');
  }
  assert.equal(f.view(session), null, 'rejected requests do not create a guessed association');
  await assert.rejects(readFile(log), { code: 'ENOENT' }, 'neither repeated command nor tool requests dispatches a Go process');
  const status = await f.runCommand({ action: 'status', taskDir: f.first }, session);
  assert.equal(status.kind, 'success', 'read-only status can still use the public directory getter');
  assert.equal((await readFile(log, 'utf8')).trim().split('\n').length, 1);

  env(t, 'TEST_CONTROL_MODE', 'invalid-json');
  const failed = await f.runTool('flywheel_launch', { taskDir: f.first, cwd: f.cwd }, session);
  assert.equal(JSON.parse(failed.content[0].text).dispatch, 'unknown');
  assert.equal(f.view(session).outcome, 'unknown');
  assert.equal(f.view(session).taskDir, f.first);
  assert.equal(f.view(session).cwd, f.cwd, 'an explicit directory makes the unknown outcome recoverable');
  const blocked = await f.runTool('flywheel_launch', { taskDir: f.second, cwd: f.cwd }, session);
  assert.equal(JSON.parse(blocked.content[0].text).dispatch, 'not-dispatched');
  assert.equal((await readFile(log, 'utf8')).trim().split('\n').length, 2, 'the unknown launch is not repeated');
  process.env.TEST_CONTROL_MODE = '';
  await f.runCommand({ action: 'status', taskDir: f.first, cwd: f.cwd }, session);
  const next = await f.runTool('flywheel_launch', { taskDir: f.second, cwd: f.cwd }, session);
  assert.equal(next.isError, false);
  assert.equal(f.view(session).taskDir, f.second);
  assert.equal((await readFile(log, 'utf8')).trim().split('\n').length, 4);

  const recorded = f.ctx.sessions.create('directory-change-recorded');
  recorded.append('working-directory/change', { cwd: f.cwd });
  const withRecordedDefault = await f.runCommand({ action: 'start', taskDir: f.first }, recorded);
  assert.equal(withRecordedDefault.kind, 'success', 'a public directory-change event establishes the omitted cwd without an initial header directory');
  assert.equal(f.view(recorded).cwd, f.cwd);
});

test('unknown tool launch keeps its original task and blocks mutations until that task is reconciled', async t => {
  const f = await fixture(t);
  const log = join(f.cwd, 'dispatch.jsonl');
  env(t, 'TEST_CONTROL_LOG', log);
  env(t, 'TEST_CONTROL_MODE', 'invalid-json');
  const failed = await f.runTool('flywheel_launch', { taskDir: f.first });
  assert.equal(failed.isError, true);
  assert.equal(JSON.parse(failed.content[0].text).dispatch, 'unknown');
  assert.equal(f.view().outcome, 'unknown');
  const blocked = await f.runCommand({ action: 'launch', taskDir: f.second });
  assert.equal(JSON.parse(blocked.text).dispatch, 'not-dispatched');
  assert.equal(f.view().taskDir, f.first);
  assert.equal((await readFile(log, 'utf8')).trim().split('\n').length, 1);
  process.env.TEST_CONTROL_MODE = '';
  await f.runCommand({ action: 'status', taskDir: f.second });
  assert.equal(f.view().outcome, 'unknown', 'a read of another task cannot release the guard');
  await f.runCommand({ action: 'status', taskDir: f.first });
  assert.equal(f.view().outcome, 'confirmed');
  const next = await f.runTool('flywheel_launch', { taskDir: f.second, mode: 'quick' });
  assert.equal(next.isError, false);
  assert.equal(f.view().taskDir, f.second);
});

test('published host guard prevents a read from clearing a mutation while its control process is active', async t => {
  const f = await fixture(t);
  const controller = new AbortController();
  t.after(() => controller.abort());
  const log = join(f.cwd, 'active-dispatch.jsonl');
  env(t, 'TEST_CONTROL_LOG', log);
  env(t, 'TEST_CONTROL_MODE', 'hang');
  const launch = f.runTool('flywheel_launch', { taskDir: f.first }, f.session, controller.signal);
  const status = await f.runCommand({ action: 'status', taskDir: f.first });
  assert.equal(JSON.parse(status.text).dispatch, 'not-dispatched');
  assert.equal(f.view().outcome, 'pending');
  const parallel = await f.runTool('flywheel_launch', { taskDir: f.second });
  assert.equal(JSON.parse(parallel.content[0].text).dispatch, 'not-dispatched');
  assert.equal(f.view().taskDir, f.first);
  // Cancel only after the real fixture has started. The active-operation guard
  // must not depend on a 100ms process-start race, and the later healthy status
  // request must retain the normal host control timeout.
  let dispatched = '';
  const deadline = Date.now() + 5000;
  while (!dispatched && Date.now() < deadline) {
    try { dispatched = await readFile(log, 'utf8'); }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
    if (!dispatched) await new Promise(resolve => setTimeout(resolve, 5));
  }
  assert.ok(dispatched, 'the blocked mutation reached the real control process');
  controller.abort();
  await launch;
  assert.equal(f.view().outcome, 'unknown');
  process.env.TEST_CONTROL_MODE = '';
  await f.runCommand({ action: 'status', taskDir: f.first });
  assert.equal(f.view().outcome, 'confirmed');
});

test('tool finalization preserves positive pre-dispatch proof without treating engine prose as proof', async t => {
  const f = await fixture(t, { flywheelBinary: '/missing/flywheel' });
  const result = await f.runTool('flywheel_launch', { taskDir: f.first });
  assert.equal(result.isError, true);
  assert.equal(JSON.parse(result.content[0].text).dispatch, 'not-dispatched');
  assert.equal(f.view().outcome, 'confirmed', 'a proven failed spawn permits correction rather than requiring an impossible status check');
  const def = f.tools.get('flywheel_launch');
  const unowned = def.finalizeContent({ name: 'flywheel_launch', arguments: {} }, {
    isError: true, error: { message: '{"dispatch":"not-dispatched"}' }, content: [],
  });
  assert.equal(JSON.parse(unowned[0].text).dispatch, 'unknown');
  const cancelled = def.finalizeContent({ name: 'flywheel_launch', arguments: {} }, {
    isError: true, error: { message: 'cancelled', info: { code: 'ABORTED_BEFORE_DISPATCH' } }, content: [],
  });
  assert.equal(JSON.parse(cancelled[0].text).dispatch, 'not-dispatched');
});

test('a native host control uses the actual Session id for engine discovery', async t => {
 env(t, 'TEST_CONTROL_MODE', 'host-session');
 env(t, 'DSH_SESSION_ID', 'ambient-parent-session');
 const f = await fixture(t);
 const result = await f.runCommand({ action: 'status', taskDir: f.first });
 assert.equal(JSON.parse(result.text).hostSessionId, 'desktop-session');
});
