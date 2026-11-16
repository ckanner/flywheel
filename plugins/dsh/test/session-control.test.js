import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, realpath, readFile, rm, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Context } from '@deepseek-ai/cordis';
import { SessionStore } from '@deepseek-ai/dsh-session';
import { SessionProjectionRegistry } from '@deepseek-ai/dsh-session-projection';
import { runControl, ControlNotDispatchedError } from '../src/control.js';
import { apply, inject } from '../src/index.js';
import {
  validSessionAbandonCommand, sessionUserProjection as projection, sessionUserAction, validSessionUserAction,
} from '../src/session-control.js';

const binary = fileURLToPath(new URL('./fixtures/fake-session-control.js', import.meta.url));
const session = { action: 'abandon', key: 'design-plan/planning:engine-1-1', invocation: 'call-blocked1', revision: 7, reason: 'The idle watchdog killed a live call; retire it.' };
const request = (taskDir) => ({ action: 'session', taskDir, cwd: taskDir, session });
function env(t, name, value) {
  const old = process.env[name]; process.env[name] = value;
  t.after(() => old === undefined ? delete process.env[name] : process.env[name] = old);
}
async function directory(t) {
  const dir = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-session-')));
  t.after(() => rm(dir, { recursive: true, force: true })); return dir;
}

test('session abandonment requires a typed bounded reason and no prompt', async t => {
  const dir = await directory(t);
  assert.equal(validSessionAbandonCommand(session), true);
  assert.equal(validSessionAbandonCommand({ ...session, reason: '' }), false);
  assert.equal(validSessionAbandonCommand({ ...session, prompt: 'x' }), false);
  assert.equal(validSessionAbandonCommand({ ...session, revision: 0 }), false);
  assert.equal(validSessionAbandonCommand({ ...session, invocation: '' }), false);
  await assert.rejects(runControl({ ...request(dir), session: { ...session, reason: '' } }), ControlNotDispatchedError);
});

test('session abandonment proof is bound to an exact live user command and cannot be reused', () => {
  const input = request('/repo'), raw = JSON.stringify(input), agentSession = { id: 'desktop-session' };
  const event = { type: 'command/run', seq: 0, data: { name: 'flywheel', commandId: 'c1', args: raw, source: { kind: 'user' } } };
  const state = projection.apply(projection.init({}, 0), event);
  const proof = sessionUserAction(input, agentSession, 'c1', raw, state);
  assert.deepEqual(proof, { kind: 'dsh', sessionId: 'desktop-session', commandId: 'c1', taskDir: '/repo', key: session.key, invocation: session.invocation, revision: 7, action: 'abandon', reason: session.reason });
  assert.equal(validSessionUserAction(proof, session, '/repo'), true);
  assert.equal(validSessionUserAction({ ...proof, reason: 'other' }, session, '/repo'), false);
  assert.equal(validSessionUserAction({ ...proof, invocation: 'call-other' }, session, '/repo'), false);
  assert.equal(validSessionUserAction(proof, session, '/other'), false);
  for (const invalid of [projection.init({}, 0), projection.apply(projection.init({}, 1), event), projection.apply(state, event), projection.apply(state, { type: 'command/done', seq: 1, data: { commandId: 'c1' } }), projection.apply(projection.init({}, 0), { ...event, data: { ...event.data, source: { kind: 'model' } } })]) {
    assert.throws(() => sessionUserAction(input, agentSession, 'c1', raw, invalid), /unconsumed/);
  }
  assert.throws(() => sessionUserAction(input, agentSession, 'c1', raw + ' ', state));
  assert.throws(() => sessionUserAction(input, undefined, 'c1', raw, state));
});

test('the process boundary strips inherited proof and forwards only the host-verified exact proof', async t => {
  const dir = await directory(t), log = join(dir, 'calls.jsonl');
  env(t, 'TEST_SESSION_LOG', log); env(t, 'FLYWHEEL_SESSION_USER_ACTION', 'inherited-forgery');
  const options = { binary, hostSessionID: 'desktop-session' };
  await assert.rejects(runControl(request(dir), options), ControlNotDispatchedError);
  const proof = { kind: 'dsh', sessionId: 'desktop-session', commandId: 'c1', taskDir: dir, key: session.key, invocation: session.invocation, revision: 7, action: 'abandon', reason: session.reason };
  await assert.rejects(runControl(request(dir), { ...options, sessionUserAction: { ...proof, reason: 'other' } }), ControlNotDispatchedError);
  await runControl(request(dir), { ...options, sessionUserAction: proof });
  const calls = (await readFile(log, 'utf8')).trim().split('\n').map(JSON.parse);
  assert.equal(calls.length, 1);
  assert.deepEqual(calls[0].command, session);
  assert.deepEqual(JSON.parse(calls[0].proof), proof);
  assert.equal(calls[0].dsh, 'desktop-session');
});

test('human abandonment keeps exact command authority while normalizing task aliases for the engine', async t => {
  const dir = await directory(t), other = await directory(t), alias = join(other, 'task-alias');
  await symlink(dir, alias, 'dir');
  const log = join(other, 'calls.jsonl');
  env(t, 'TEST_SESSION_LOG', log);
  const options = { binary, hostSessionID: 'desktop-session' };
  for (const taskDir of [alias, dir + '/', dir + '/./']) {
    const input = request(taskDir), raw = JSON.stringify(input);
    const event = { type: 'command/run', seq: 0, data: { name: 'flywheel', commandId: 'c1', args: raw, source: { kind: 'user' } } };
    const proof = sessionUserAction(input, { id: 'desktop-session' }, 'c1', raw, projection.apply(projection.init({}, 0), event));
    const result = await runControl(input, { ...options, sessionUserAction: proof });
    assert.equal(result.taskDir, dir);
    assert.equal(proof.taskDir, taskDir, 'normalization must not mutate the original human-event proof');
    await assert.rejects(runControl(request(other), { ...options, sessionUserAction: proof }), ControlNotDispatchedError,
      'a proof for the same session key and revision must not authorize a different task');
    await assert.rejects(runControl(request(dir), { ...options, sessionUserAction: proof }), ControlNotDispatchedError,
      'the proof must first match the exact original command, even when a different spelling resolves to the same task');
  }
  const calls = (await readFile(log, 'utf8')).trim().split('\n').map(JSON.parse);
  assert.equal(calls.length, 3, 'only the three exact human commands were dispatched');
  for (const call of calls) {
    assert.equal(JSON.parse(call.proof).taskDir, dir, 'the child receives the canonical task directory in its proof');
    assert.deepEqual(call.command, session);
  }
});

test('published host accepts a user command and rejects model tool abandonment', async t => {
  const dir = await directory(t), ctx = new Context(), tools = new Map(); let command;
  t.after(() => ctx.fiber.dispose());
  await ctx.plugin(SessionStore); await ctx.plugin(SessionProjectionRegistry);
  ctx.provide('commands', { register: value => { command = value; } });
  ctx.provide('tools', { register: value => tools.set(value.name, value) });
  ctx.provide('workingDirectory', { get: () => dir });
  await ctx.plugin({ apply, inject }, { flywheelBinary: binary });
  const sessionHandle = ctx.sessions.create('session-abandon', { meta: { cwd: dir } });
  const input = request(dir), raw = JSON.stringify(input);
  assert.equal((await command.handler({ rawInput: raw, commandId: 'c1', agent: { session: sessionHandle } })).kind, 'error');
  sessionHandle.append('command/run', { commandId: 'c1', name: 'flywheel', args: raw, source: { kind: 'user' } });
  const result = await command.handler({ rawInput: raw, commandId: 'c1', agent: { session: sessionHandle } });
  assert.equal(result.kind, 'success');
  const tool = tools.get('flywheel_control');
  assert.equal(tool.parameters.properties.action.enum.includes('session'), false, 'model tools must not expose session abandonment');
});
