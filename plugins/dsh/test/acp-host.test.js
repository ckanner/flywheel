import test from 'node:test';
import assert from 'node:assert/strict';
import { acpCommandSummary, installFlywheelAcp } from '../src/acp-host.js';

test('human ACP bootstrap commands forward only the allowed identity-bound controls', async () => {
  const commands = new Map(), disposers = [], calls = [];
  const agent = { session: { id: 'host-session' } }, controller = new AbortController();
  let configuration;
  const ctx = {
    effect: factory => { const dispose = factory(); disposers.push(dispose); return dispose; },
    commands: { register: definition => commands.set(definition.name, definition) },
    inject: (_dependencies, callback) => callback(ctx),
  };
  const installed = installFlywheelAcp(ctx, () => ({ command: '/official/agy' }), { loadHost: async () => ({
    installAcpHost(scope, getConfig) {
      assert.equal(scope, ctx);
      configuration = getConfig();
      return { action: async (...args) => { calls.push(args); return { protocol: 2, status: 'ready', messages: [{ text: 'private transcript' }], login: { url: 'private auth URL' } }; } };
    },
  }) });
  await installed.ready;
  assert.deepEqual(configuration, { command: '/official/agy' });
  const command = commands.get('flywheel-antigravity');
  assert.equal(command.recordInput, false, 'even rejected pasted input must not be logged by DSH');
  const guidance = await command.handler({ rawInput: '', agent, signal: controller.signal });
  assert.equal(guidance.kind, 'success');
  assert.match(guidance.text, /Antigravity view/);
  assert.equal(calls.length, 0, 'the blank-conversation entry must not start an agent, login or task');
  const input = { version: 2, action: 'connect' };
  const result = await command.handler({ rawInput: JSON.stringify(input), agent, signal: controller.signal });
  assert.equal(result.kind, 'success');
  assert.deepEqual(calls, [[agent, input, controller.signal]]);
  assert.equal(result.text.includes('private'), false, 'command receipts must not duplicate transcript or sign-in data');
  for (const input of [
    { version: 2, action: 'prompt', id: 'session', prompt: 'SECRET_PROMPT' },
    { version: 2, action: 'connect', cwd: '/another/project' },
    { version: 2, action: 'connect', id: 'forged-session' },
    { version: 2, action: 'login-submit', input: 'SECRET_CODE' },
    { version: 2, action: 'cancel' },
    { action: 'doctor' },
  ]) {
    const rejected = await command.handler({ rawInput: JSON.stringify(input), agent });
    assert.equal(rejected.kind, 'error');
    assert.equal(rejected.text.includes('SECRET'), false);
  }
  const malformed = await command.handler({ rawInput: 'SECRET_NOT_JSON', agent });
  assert.equal(malformed.kind, 'error');
  assert.equal(malformed.text.includes('SECRET_NOT_JSON'), false);
  assert.equal(calls.length, 1, 'rejected controls never reach the ACP manager');
  for (const dispose of disposers) dispose();
  assert.equal((await command.handler({ rawInput: '', agent })).kind, 'error');
  assert.equal((await command.handler({ rawInput: JSON.stringify(input), agent })).kind, 'error');
  assert.equal(calls.length, 1);
});

test('logged ACP summaries keep operational facts and exclude provider content and auth state', () => {
  const snapshot = {
    protocol: 2, id: 'one', sourceCwd: '/source', cwd: '/state/workspaces/one', branch: 'antigravity/one',
    status: 'prompting', connection: 'connected', baseCommit: 'a'.repeat(40),
    knownWork: { tools: 1, permissions: 1, children: 0, unknown: false, raw: 'private' },
    cleanup: { process: 'running', session: 'open', token: 'private' },
    runtime: { executable: '/official/agy', version: '1.5.14', sha256: 'hash', identity: { profile: 'private' } },
    messages: [{ text: 'private' }], permissions: [{ rawInput: 'private' }], tools: [{ content: 'private' }],
    prompt: { operationId: 'private' }, error: { message: 'private' }, login: { url: 'private' },
  };
  const result = acpCommandSummary({ protocol: 2, sessions: [snapshot], login: { url: 'private' } });
  assert.equal(result.sessions[0].status, 'prompting');
  assert.equal(result.sessions[0].knownWork.permissions, 1);
  assert.equal(result.sessions[0].runtime.version, '1.5.14');
  assert.equal(JSON.stringify(result).includes('private'), false);
  assert.throws(() => acpCommandSummary({ protocol: 1 }), /invalid control response/);
});

test('CLI-only contexts do not load an ACP Host and failed Host installation is explicit', async () => {
  let loaded = false;
  assert.equal(installFlywheelAcp({}, () => ({}), { loadHost: async () => { loaded = true; } }), null);
  assert.equal(loaded, false);
  const ctx = { effect: factory => factory(), commands: { register() {} }, inject: (_deps, callback) => callback(ctx) };
  await assert.rejects(installFlywheelAcp(ctx, () => ({}), { loadHost: async () => ({}) }).ready, /does not provide its ACP Host/);
});
