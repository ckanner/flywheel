import test from 'node:test';
import assert from 'node:assert/strict';
import { chmod, mkdtemp, readFile, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';
import { runControl, validateControl } from '../src/control.js';
import { apply } from '../src/index.js';
import { apply as applyPolicy } from '../src/policy.js';

const binary = fileURLToPath(new URL('./fixtures/fake-control.js', import.meta.url));
await chmod(binary, 0o755);

test('control invokes only the Go JSON CLI using literal argv, including paths with shell syntax', async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-control-$(literal) '));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const value = await runControl({ action: 'status', taskDir: cwd }, { binary, defaultCwd: cwd });
  assert.equal(value.taskDir, await realpath(cwd));
  assert.equal(value.running, false);
  assert.equal(value.state.model, 'deepseek-flash');
  for (const input of [{ action: 'rm', taskDir: cwd }, { action: 'status', taskDir: cwd, shell: true },
    { action: 'status', taskDir: '../relative' }]) {
    await assert.rejects(validateControl(input, cwd));
  }
  await assert.rejects(runControl({ action: 'status', taskDir: cwd }, {
    binary, defaultCwd: cwd, signal: AbortSignal.abort(),
  }), /cancelled/);
});

test('slash command and model tool use the same validated engine control boundary', async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-host-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const registrations = {};
  const ctx = {
    on: (name, fn) => { registrations[name] = fn; },
    commands: { register: definition => { registrations.command = definition; } },
    tools: { register: definition => { registrations.tool = definition; } },
  };
  apply(ctx, { flywheelBinary: binary });
  const agent = { session: { header: { cwd } } };
  const request = { action: 'start', taskDir: cwd };
  const fromCommand = await registrations.command.handler({ rawInput: JSON.stringify(request), agent });
  const fromTool = await registrations.tool.execute(request, { agent });
  assert.equal(fromCommand.kind, 'success');
  assert.deepEqual(JSON.parse(fromCommand.text), JSON.parse(fromTool));
  assert.equal(JSON.parse(fromTool).running, true);
  assert.equal(registrations.tool.timeoutMs, 60000);
  assert.equal((await registrations.command.handler({ rawInput: 'not-json', agent })).kind, 'error');
  await assert.rejects(registrations.tool.execute({ ...request, action: 'shell' }, { agent }));
  registrations.dispose();
});

test('policy uses the shipped shell probe and pins effective session facts, rejecting partial enforcement', async () => {
  let callback;
  let seenSpec;
  const events = [];
  const session = { append: (type, data) => events.push({ type, data }) };
  const ctx = {
    on: (name, fn) => { assert.equal(name, 'session/created'); callback = fn; },
    shell: {
      sandboxMode: 'read-only',
      resolve: spec => { seenSpec = spec; return spec; },
      execute: async () => ({ result: async () => ({ exitCode: 0, sandbox: { mode: 'read-only', enforcement: 'full' } }) }),
    },
    sandboxPolicy: { resolve: () => ({ mode: 'read-only', workspaceRoot: '/workspace' }) },
    approval: { effectivePolicy: () => 'never' },
  };
  await applyPolicy(ctx, { cwd: '/workspace', permissionMode: 'read-only' });
  assert.equal(seenSpec.command, ':');
  assert.equal(seenSpec.sandboxPolicy.mode, 'read-only');
  callback(session);
  assert.deepEqual(events.map(event => event.type), ['sandbox/mode', 'approval/policy', 'flywheel/policy']);
  assert.equal(events[2].data.enforcement, 'full');
  ctx.shell.execute = async () => ({ result: async () => ({ exitCode: 0, sandbox: { mode: 'read-only', enforcement: 'partial' } }) });
  await assert.rejects(applyPolicy(ctx, { cwd: '/workspace', permissionMode: 'read-only' }), /full enforcement/);
  ctx.shell.sandboxMode = undefined;
  await assert.rejects(applyPolicy(ctx, { cwd: '/workspace', permissionMode: 'read-only' }), /confined/);
});

test('browser module registers the actual rc.5 dock and calls existing command Remote', async () => {
  const source = await readFile(new URL('../src/client.js', import.meta.url), 'utf8');
  const exported = {};
  vm.runInNewContext(source, { exports: exported, require: name => {
    assert.equal(name, 'react');
    return { createElement: () => null };
  } });
  let definition;
  let invocation;
  exported.apply({
    slots: {
      inject: (name, activate) => { assert.equal(name, 'conversation.composer.dock'); activate(); },
      register: value => { definition = value; },
    },
    remote: { commands: { execute: async (...args) => {
      invocation = args;
      return { ok: true, value: { result: { kind: 'success', text: '{"protocol":1,"running":false}' } } };
    } } },
  });
  assert.equal(definition.id, 'flywheel');
  const face = definition.inject('desktop-session');
  const response = await face.execute({ action: 'status', taskDir: '/workspace/docs/task' });
  assert.equal(response.protocol, 1);
  assert.equal(invocation[0], 'desktop-session');
  assert.equal(invocation[1], '/flywheel {"action":"status","taskDir":"/workspace/docs/task"}');
  assert.equal(invocation[2].length, 0);
});
