import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { chmod, mkdir, mkdtemp, readFile, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { createRequire } from 'node:module';
import { promisify } from 'node:util';
import { closeClientHarness, loadClient } from './support/client-harness.js';

// Exercise built output in routine checks and the exact extracted candidate in
// packed checks. The companion workspace API must come from that same package.
const packageRequire = createRequire(process.env.FLYWHEEL_TEST_DSH_PACKAGE_DIR
  ? resolve(process.env.FLYWHEEL_TEST_DSH_PACKAGE_DIR, 'package.json') : new URL('../package.json', import.meta.url));
const { apply, inject } = await import(process.env.FLYWHEEL_TEST_DSH_PACKAGE_DIR
  ? pathToFileURL(resolve(process.env.FLYWHEEL_TEST_DSH_PACKAGE_DIR, 'dist/index.mjs')) : new URL('../dist/index.js', import.meta.url));
const { createWorkspace } = await import(pathToFileURL(packageRequire.resolve('@ckanner/dsh-subagent-antigravity/acp/workspace')));
const hostRequire = createRequire(process.env.FLYWHEEL_TEST_DSH_HOST_DIR
  ? resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR, 'package.json') : new URL('../package.json', import.meta.url));
const { Context } = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/cordis')));
const { default: TypertRegistry } = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/dsh-typert-registry')));
const { SessionStore } = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/dsh-session')));
const { SessionProjectionRegistry } = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/dsh-session-projection')));
const { default: CommandRuntime } = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/dsh-commands')));

after(closeClientHarness);
const execute = promisify(execFile);
const binary = fileURLToPath(new URL('./fixtures/fake-control.js', import.meta.url));
await chmod(binary, 0o755);
async function temporary(t) {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-acp-')));
  t.after(() => rm(root, { recursive: true, force: true }));
  return root;
}

test('the normal Flywheel Host installs the public ACP service and human entry without model tools', { timeout: 5000 }, async t => {
  const cwd = await temporary(t), ctx = new Context();
  t.after(() => ctx.fiber.dispose());
  await ctx.plugin(TypertRegistry);
  await ctx.plugin(SessionStore);
  await ctx.plugin(SessionProjectionRegistry);
  await ctx.plugin(CommandRuntime);
  const tools = new Map();
  ctx.provide('tools', { register: definition => tools.set(definition.name, definition) });
  ctx.provide('workingDirectory', { get: () => cwd });
  const session = ctx.sessions.create('acp-host-session', { meta: { cwd } });
  const agent = { session, id: session.id, ctx };
  // An initially pending Cordis injection may settle before its dependency
  // exists. Observe the public service's actual activation, not a timer.
  const available = new Promise(resolve => ctx.inject(['antigravityAcp'], owner => { resolve(owner.antigravityAcp); }));
  await ctx.plugin({ apply, inject }, { flywheelBinary: binary });
  const service = await available;
  const defaults = await service.action(agent, { version: 2, action: 'defaults' }, new AbortController().signal);
  assert.equal(defaults.transport, 'acp');
  assert.equal(defaults.workspaceMode, 'isolated-git-worktree');
  assert.ok(ctx.commands.find(agent, 'flywheel-antigravity'));
  assert.deepEqual([...tools.keys()].sort(), ['flywheel_control', 'flywheel_launch'], 'ACP is never an outer-agent delegation tool');
  const entry = await ctx.commands.execute(agent, '/flywheel-antigravity', [], new AbortController().signal);
  assert.equal(entry.result.kind, 'success');
  assert.match(entry.result.text, /has not started an agent or sent a task/);
  const result = await ctx.commands.execute(agent,
    '/flywheel-antigravity {"version":2,"action":"prompt","prompt":"PRIVATE_CODE"}', [], new AbortController().signal);
  assert.equal(result.result.kind, 'error');
  assert.equal(JSON.stringify(session.snapshotEvents()).includes('PRIVATE_CODE'), false, 'command logs never retain pasted ACP code');
});

test('the shipped client exposes a separate authenticated ACP view and never dispatches a Go command for it', async () => {
  const definitions = new Map(), mounted = [], calls = [], disposers = [];
  const remote = {
    $mount: async value => { mounted.push(value); return () => {}; },
    commands: { execute() { throw new Error('ACP does not use the Go control command'); } },
    flywheelConsole: { read() { throw new Error('ACP does not read machine receipts'); } },
    antigravityAcp: { action: async (owner, input) => { calls.push({ owner, input }); return { ok: true, value: { protocol: 2, transport: 'acp' } }; } },
  };
  const ctx = {
    remote, locale: { register: () => () => {}, bind: () => key => key },
    effect: factory => { const dispose = factory(); if (typeof dispose === 'function') disposers.push(dispose); return dispose; }, on: (_event, callback) => disposers.push(callback),
    inject: (_deps, callback) => { const result = Promise.resolve().then(() => callback({ remote })); result.dispose = async () => {}; return result; },
    slots: { inject: (_name, callback) => callback(), register: (definition, Component) => { definitions.set(definition.id, { definition, Component }); return () => {}; } },
  };
  loadClient().apply(ctx);
  assert.deepEqual([...definitions.keys()].sort(), ['flywheel', 'flywheel-antigravity']);
  const { definition } = definitions.get('flywheel-antigravity');
  const actions = definition.store.create('host-session').actions;
  const view = definition.inject('host-session', actions);
  assert.equal((await view.execute({ version: 2, action: 'defaults' })).transport, 'acp');
  assert.deepEqual(calls, [{ owner: 'host-session', input: { version: 2, action: 'defaults' } }]);
  assert.ok(mounted.some(value => value.descriptors.some(descriptor => descriptor.namespace === 'antigravityAcp' && descriptor.parameters[0].lookup === 'agent')));
  for (const dispose of disposers) dispose();
  await assert.rejects(view.execute({ version: 2, action: 'defaults' }), /binding has ended/);
  assert.equal(calls.length, 1);
});

test('human and model machine mutations reject an ACP worktree and its aliases before spawning Go', async t => {
  const root = await temporary(t), source = join(root, 'source');
  await mkdir(source);
  const git = async (...args) => execute('git', ['-C', source, ...args]);
  await git('init', '-b', 'main');
  await git('config', 'user.name', 'ACP Test');
  await git('config', 'user.email', 'acp@example.invalid');
  await writeFile(join(source, 'file.txt'), 'original\n');
  await git('add', 'file.txt');
  await git('commit', '-m', 'initial');
  const workspace = await createWorkspace({ sourceCwd: source, stateRoot: join(root, 'state'), id: 'owned', owner: 'host' });
  const alias = join(root, 'alias');
  await symlink(workspace.cwd, alias, 'dir');
  const taskDir = join(workspace.cwd, 'docs', 'task');
  await mkdir(taskDir, { recursive: true });
  const log = join(root, 'go-argv.jsonl'), previousLog = process.env.TEST_CONTROL_LOG;
  process.env.TEST_CONTROL_LOG = log;
  t.after(() => { if (previousLog === undefined) delete process.env.TEST_CONTROL_LOG; else process.env.TEST_CONTROL_LOG = previousLog; });
  const commands = new Map(), tools = new Map(), agent = { session: { id: 'host', header: { cwd: workspace.cwd } } };
  const ctx = {
    on() {}, commands: { register: value => commands.set(value.name, value) }, tools: { register: value => tools.set(value.name, value) },
    workingDirectory: { get: () => alias }, sessionProjections: { register() {}, stateOf: () => ({ view: null, cwd: workspace.cwd }) },
  };
  apply(ctx, { flywheelBinary: binary });
  const input = { action: 'start', taskDir };
  const result = await commands.get('flywheel').handler({ rawInput: JSON.stringify(input), agent });
  assert.equal(result.kind, 'error');
  const error = JSON.parse(result.text);
  assert.equal(error.dispatch, 'not-dispatched');
  assert.match(error.message, /interactive Antigravity ACP session/);
  await assert.rejects(tools.get('flywheel_control').execute(input, { agent }), /interactive Antigravity ACP session/);
  await assert.rejects(readFile(log), { code: 'ENOENT' }, 'neither rejected entry spawns the Go process');
  const status = await commands.get('flywheel').handler({ rawInput: JSON.stringify({ action: 'status', taskDir }), agent });
  assert.equal(status.kind, 'success', 'read-only machine inspection remains available');
  assert.equal(await readFile(join(workspace.cwd, 'file.txt'), 'utf8'), 'original\n');
});
