import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { chmod, mkdtemp, readFile, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { runControl, validateControl } from '../src/control.js';
import { apply } from '../src/index.js';
import { apply as applyPolicy } from '../src/policy.js';
import { act, closeClientHarness, mountPanel } from './support/client-harness.js';

after(closeClientHarness);

const binary = fileURLToPath(new URL('./fixtures/fake-control.js', import.meta.url));
await chmod(binary, 0o755);

function withEnv(t, name, value) {
  const previous = process.env[name];
  process.env[name] = value;
  t.after(() => { if (previous === undefined) delete process.env[name]; else process.env[name] = previous; });
}

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

test('configuration and doctor need no task directory, and probe is an explicit doctor-only boolean', async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-preflight-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const physical = await realpath(cwd);
  const log = join(cwd, 'argv.jsonl');
  withEnv(t, 'TEST_CONTROL_LOG', log);
  const config = await runControl({ action: 'config' }, { binary, defaultCwd: cwd });
  assert.equal(config.taskDir, '');
  assert.equal(config.configuration.bindings[0].provider, 'deepseek-official');
  const local = await runControl({ action: 'doctor', cwd }, { binary });
  assert.equal(local.diagnostic.probeRequested, false);
  assert.equal(local.diagnostic.inferenceTested, false);
  const probe = await runControl({ action: 'doctor', taskDir: cwd, probe: true }, { binary, defaultCwd: cwd });
  assert.equal(probe.diagnostic.probeRequested, true);
  assert.deepEqual((await readFile(log, 'utf8')).trim().split('\n').map(line => JSON.parse(line)), [
    ['control', 'config', '--cwd', physical],
    ['control', 'doctor', '--cwd', physical],
    ['control', 'doctor', '--cwd', physical, '--task-dir', physical, '--probe'],
  ]);
  for (const input of [
    { action: 'start' }, { action: 'status' }, { action: 'stop' },
    { action: 'config', probe: false }, { action: 'status', taskDir: cwd, probe: true },
    { action: 'doctor', probe: 'true' }, { action: 'doctor', probe: null },
    { action: 'config', taskDir: '' }, { action: 'doctor', taskDir: null },
  ]) await assert.rejects(validateControl(input, cwd));
});

test('doctor returns a valid failed report but rejects process, protocol, and identity failures', async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-report-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  withEnv(t, 'TEST_CONTROL_MODE', 'doctor-fail');
  const request = { action: 'doctor', cwd, probe: true };
  const value = await runControl(request, { binary });
  assert.equal(value.ok, false);
  assert.equal(value.diagnostic.ok, false);
  assert.equal(value.diagnostic.checks[1].status, 'fail');
  assert.match(value.diagnostic.checks[1].remedy, /executable/);
  for (const mode of ['unexpected-exit', 'fail-exit-zero', 'wrong-task', 'wrong-protocol',
    'wrong-diagnostic-task', 'wrong-probe', 'claims-inference', 'bad-check', 'disagree',
    'missing-report', 'invalid-json']) {
    process.env.TEST_CONTROL_MODE = mode;
    await assert.rejects(runControl(request, { binary }), undefined, mode);
  }
  process.env.TEST_CONTROL_MODE = 'bad-binding';
  await assert.rejects(runControl({ action: 'config', cwd }, { binary }), /malformed/);
  process.env.TEST_CONTROL_MODE = 'legacy-binding';
  const legacy = await runControl({ action: 'config', cwd }, { binary });
  assert.equal(legacy.configuration.bindings[0].model, '', 'compatible inline defaults are inspectable');
  process.env.TEST_CONTROL_MODE = 'split-utf8';
  const decoded = await runControl(request, { binary });
  assert.equal(decoded.diagnostic.checks[0].summary, '账号尚未验证，不发送推理请求。');
});

test('hung controls are bounded and cancellation is never a diagnostic success', async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-control-timeout-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  withEnv(t, 'TEST_CONTROL_MODE', 'hang');
  const request = { action: 'doctor', cwd };
  await assert.rejects(runControl(request, { binary, timeoutMs: 100 }), /timed out/);
  await assert.rejects(runControl(request, { binary, signal: AbortSignal.timeout(100) }), /cancelled/);
});

test('diagnostic cancellation and timeout keep the Go owner alive until delayed group cleanup finishes', { timeout: 25000 }, async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-control-cleanup-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  withEnv(t, 'TEST_CONTROL_MODE', 'cleanup');
  withEnv(t, 'TEST_CONTROL_LIFECYCLE_LOG', '');
  for (const action of ['doctor', 'config']) {
    const log = join(cwd, `${action}.jsonl`);
    process.env.TEST_CONTROL_LIFECYCLE_LOG = log;
    let ready;
    let reaped = false;
    t.after(() => {
      if (reaped) return;
      // Also clean up a fixture child if a regression kills its owner first.
      for (const pid of [ready?.child ? -ready.child : null, ready?.pid]) {
        if (pid) { try { process.kill(pid, 'SIGKILL'); } catch { /* already reaped */ } }
      }
    });
    const controller = new AbortController();
    const run = runControl({ action, cwd }, { binary, timeoutMs: 1000, signal: controller.signal });
    const rejected = assert.rejects(run, action === 'doctor' ? /cancelled/ : /timed out/);
    const deadline = Date.now() + 5000;
    while (!ready && Date.now() < deadline) {
      try { ready = (await readFile(log, 'utf8')).trim().split('\n').map(line => JSON.parse(line)).find(event => event.event === 'ready'); }
      catch (error) { if (error.code !== 'ENOENT') throw error; }
      if (!ready) await new Promise(resolve => setTimeout(resolve, 20));
    }
    assert.ok(ready, 'the child installed its cancellation handler before cancellation');
    if (action === 'doctor') controller.abort();
    await rejected;
    const events = (await readFile(log, 'utf8')).trim().split('\n').map(line => JSON.parse(line));
    assert.deepEqual(events.map(event => event.event), ['ready', 'term', 'cleanup-complete']);
    assert.throws(() => process.kill(ready.child, 0), { code: 'ESRCH' }, 'the detached fixture worker was reaped');
    assert.throws(() => process.kill(ready.pid, 0), { code: 'ESRCH' }, 'the control owner exited before rejection');
    reaped = true;
  }
});

test('an unresponsive diagnostic owner is eventually force-stopped after its cleanup window', { timeout: 30000 }, async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-control-stubborn-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const log = join(cwd, 'lifecycle.jsonl');
  withEnv(t, 'TEST_CONTROL_MODE', 'ignore-term');
  withEnv(t, 'TEST_CONTROL_LIFECYCLE_LOG', log);
  let ready;
  let reaped = false;
  t.after(() => {
    if (!reaped && ready?.pid) { try { process.kill(ready.pid, 'SIGKILL'); } catch { /* already reaped */ } }
  });
  const rejected = assert.rejects(runControl({ action: 'doctor', cwd }, { binary, timeoutMs: 1000 }), /timed out/);
  const deadline = Date.now() + 5000;
  while (!ready && Date.now() < deadline) {
    try { ready = (await readFile(log, 'utf8')).trim().split('\n').map(line => JSON.parse(line)).find(event => event.event === 'ready'); }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
    if (!ready) await new Promise(resolve => setTimeout(resolve, 20));
  }
  assert.ok(ready);
  await rejected;
  const events = (await readFile(log, 'utf8')).trim().split('\n').map(line => JSON.parse(line));
  assert.deepEqual(events.map(event => event.event), ['ready', 'term']);
  assert.throws(() => process.kill(events[0].pid, 0), { code: 'ESRCH' });
  reaped = true;
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
  assert.deepEqual(registrations.tool.parameters.required, ['action']);
  assert.equal(registrations.tool.parameters.properties.probe.type, 'boolean');
  assert.equal(registrations.tool.isConcurrencySafe({ action: 'config' }), true);
  assert.equal(registrations.tool.isConcurrencySafe({ action: 'doctor' }), true);
  assert.equal(registrations.tool.isConcurrencySafe({ action: 'start' }), false);
  const config = await registrations.tool.execute({ action: 'config' }, { agent });
  assert.equal(JSON.parse(config).configuration.scope, 'project');
  withEnv(t, 'TEST_CONTROL_MODE', 'doctor-fail');
  const failedCheck = await registrations.command.handler({ rawInput: '{"action":"doctor"}', agent });
  assert.equal(failedCheck.kind, 'success');
  assert.equal(JSON.parse(failedCheck.text).diagnostic.ok, false);
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
  await assert.rejects(applyPolicy(ctx, { cwd: '/workspace', permissionMode: 'read-only' }), /sandbox-capable/);
});

test('full-access shell readiness records the absence of confinement and rejects fabricated enforcement', async () => {
  let callback;
  let seenSpec;
  let registrations = 0;
  let result = { exitCode: 0, sandbox: { mode: 'danger-full-access', denied: false } };
  const events = [];
  const ctx = {
    on: (name, fn) => { assert.equal(name, 'session/created'); registrations += 1; callback = fn; },
    shell: {
      sandboxMode: 'danger-full-access',
      resolve: spec => { seenSpec = spec; return spec; },
      execute: async () => ({ result: async () => result }),
    },
    sandboxPolicy: { resolve: () => ({ mode: 'danger-full-access', workspaceRoot: '/workspace' }) },
    approval: { effectivePolicy: () => 'never' },
  };
  const config = { cwd: '/workspace', permissionMode: 'danger-full-access' };
  await applyPolicy(ctx, config);
  assert.equal(seenSpec.command, ':');
  assert.deepEqual(seenSpec.sandboxPolicy, { mode: 'danger-full-access', workspaceRoot: '/workspace' });
  callback({ append: (type, data) => events.push({ type, data }) });
  assert.deepEqual(events, [
    { type: 'sandbox/mode', data: { mode: 'danger-full-access' } },
    { type: 'approval/policy', data: { policy: 'never' } },
    { type: 'flywheel/policy', data: { protocol: 1, cwd: '/workspace', permissionMode: 'danger-full-access',
      approvalPolicy: 'never', confinedShell: false } },
  ]);
  for (const sandbox of [
    { mode: 'danger-full-access', denied: true },
    { mode: 'danger-full-access' },
    { mode: 'danger-full-access', denied: false, enforcement: 'full' },
    { mode: 'danger-full-access', denied: false, enforcement: 'partial' },
    { mode: 'workspace-write', denied: false, enforcement: 'full' },
  ]) {
    result = { exitCode: 0, sandbox };
    await assert.rejects(applyPolicy(ctx, config), /explicit full access/);
  }
  result = { exitCode: 1, sandbox: { mode: 'danger-full-access', denied: false } };
  await assert.rejects(applyPolicy(ctx, config), /explicit full access/);
  assert.equal(registrations, 1, 'a failed readiness probe cannot register session policy evidence');
});

test('the native rc.5 view reaches the registered slash command through the session-scoped Remote', async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-view-remote-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const registrations = {};
  apply({
    on: (name, callback) => { registrations[name] = callback; },
    commands: { register: definition => { registrations.command = definition; } },
    tools: { register: definition => { registrations.tool = definition; } },
  }, { flywheelBinary: binary });
  t.after(() => registrations.dispose());
  const agent = { session: { header: { cwd } } };
  let completion;
  let invocation;
  const ui = await mountPanel(t, (request, sessionId, command) => {
    invocation = { sessionId, command };
    completion = registrations.command.handler({
      rawInput: command.slice('/flywheel'.length), agent, signal: new AbortController().signal,
    }).then(result => ({ ok: true, value: { result } }));
    return completion;
  });
  assert.equal(ui.definition.name, 'conversation.view');
  await ui.type(0, cwd);
  await ui.click('刷新状态');
  await act(async () => { await completion; });
  assert.deepEqual(invocation, { sessionId: 'desktop-session',
    command: `/flywheel ${JSON.stringify({ action: 'status', taskDir: cwd })}` });
  assert.match(ui.container.querySelector('[aria-label="任务状态"]').textContent, /example/);
  assert.equal(ui.container.querySelector('[role="alert"]'), null);
  assert.deepEqual(ui.requests.map(request => request.action), ['status']);
});
