import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { runControl } from '../src/control.js';
import { taskAssociationProjection as definition, taskProjectionKey } from '../src/task-association.js';

function log(cwd = '/repo', inheritedEventCount = 0) {
  let state = definition.init({ cwd }, inheritedEventCount);
  const events = [];
  return { get state() { return state; }, get view() { return definition.wire.view(state); }, events,
    append(type, data) {
      const event = { type, data, seq: events.length, time: 1000 + events.length };
      events.push(event);
      state = definition.apply(state, event);
      assert.deepEqual(definition.stateSchema.parse(JSON.parse(JSON.stringify(state))), state);
      assert.deepEqual(definition.wire.viewSchema.parse(definition.wire.view(state)), definition.wire.view(state));
    },
  };
}
const result = (taskDir, cwd = '/repo') => ({ protocol: 1, ok: true, running: true, taskDir, cwd, state: null });
const content = value => [{ type: 'text', text: JSON.stringify(value) }];
const tool = (l, callId, args, name = 'flywheel_launch') => l.append('tool/call', { callId, name, arguments: JSON.stringify(args) });
const done = (l, callId, value, isError = false) => l.append('tool/result', {
  message: { toolCallId: callId, content: content(value), isError },
});
const command = (l, commandId, args) => l.append('command/run', { commandId, name: 'flywheel', args: JSON.stringify(args) });
const commandDone = (l, commandId, value, kind = 'success') => l.append('command/done', { commandId, kind, text: JSON.stringify(value) });

test('exact launch association folds public tool events and current-directory evidence, without inferring paths', () => {
  const l = log('/original');
  assert.equal(taskProjectionKey, 'flywheelTask');
  l.append('working-directory/change', { cwd: '/worktree' });
  tool(l, 'launch-1', { taskDir: '/worktree/docs/fix', mode: 'auto' });
  assert.deepEqual(l.view, { operationId: 'tool:launch-1', sequence: 1, action: 'launch',
    taskDir: '/worktree/docs/fix', cwd: '/worktree', outcome: 'pending', message: '' });
  done(l, 'launch-1', result('/physical/task', '/physical/project'));
  assert.equal(l.view.taskDir, '/physical/task');
  assert.equal(l.view.cwd, '/physical/project');
  assert.equal(l.view.requestedTaskDir, '/worktree/docs/fix');
  assert.equal(l.view.requestedCwd, '/worktree');
  assert.equal(l.view.outcome, 'confirmed');
  const stableState = l.state;
  const stableView = l.view;
  l.append('assistant/message', { message: { content: content(result('/fake/task')) } });
  assert.equal(l.state, stableState, 'unrelated events preserve the projection state reference');
  assert.equal(l.view, stableView, 'unrelated events cannot generate view work or forge association');
  assert.equal(log().view, null, 'other conversations start without this association');
});

test('an unknown or missing mutation response keeps its original task until that task is observed', () => {
  const l = log();
  command(l, 'first', { action: 'launch', taskDir: '/repo/docs/first' });
  command(l, 'second', { action: 'start', taskDir: '/repo/docs/second' });
  commandDone(l, 'second', { protocol: 1, type: 'flywheel-control-error', action: 'start',
    dispatch: 'not-dispatched', message: 'An earlier mutation needs reconciliation' }, 'error');
  assert.equal(l.view.operationId, 'command:first', 'a blocked later request cannot erase the original request');
  commandDone(l, 'first', { protocol: 1, type: 'flywheel-control-error', action: 'launch',
    dispatch: 'unknown', message: 'transport interrupted' }, 'error');
  assert.equal(l.view.outcome, 'unknown');
  command(l, 'wrong-status', { action: 'status', taskDir: '/repo/docs/second' });
  commandDone(l, 'wrong-status', result('/repo/docs/second'));
  assert.equal(l.view.outcome, 'unknown', 'a different task cannot reconcile the original mutation');
  command(l, 'status', { action: 'status', taskDir: '/repo/docs/first' });
  commandDone(l, 'status', result('/physical/first', '/physical/repo'));
  assert.equal(l.view.outcome, 'confirmed');
  assert.equal(l.view.taskDir, '/physical/first', 'a confirmed status adopts the host-validated realpath');
  assert.equal(l.view.requestedTaskDir, '/repo/docs/first');
  assert.equal(l.view.requestedCwd, '/repo');
  tool(l, 'next', { taskDir: '/repo/docs/second', cwd: '/repo', mode: 'quick' });
  assert.equal(l.view.taskDir, '/repo/docs/second');
  assert.equal(l.view.outcome, 'pending', 'a new launch may bind after reconciliation');
  commandDone(l, 'first', result('/repo/docs/first'));
  assert.equal(l.view.operationId, 'tool:next', 'late or duplicate completion cannot replace the newer association');
});

test('a missing command result is recoverable by status and only structured non-dispatch proof releases a failure', () => {
  const l = log();
  tool(l, 'missing', { taskDir: '/repo/task' });
  command(l, 'status', { action: 'status', taskDir: '/repo/task' });
  commandDone(l, 'status', result('/repo/task'));
  assert.equal(l.view.outcome, 'confirmed', 'replay preserves a missing result and a later explicit read reconciles it');
  tool(l, 'failed', { taskDir: '/repo/task' });
  done(l, 'failed', 'Error: not-dispatched', true);
  assert.equal(l.view.outcome, 'unknown', 'error prose cannot fabricate dispatch proof');
  command(l, 'status2', { action: 'status', taskDir: '/repo/task' });
  commandDone(l, 'status2', result('/repo/task'));
  tool(l, 'preflight', { taskDir: '/repo/task' });
  done(l, 'preflight', { protocol: 1, type: 'flywheel-control-error', action: 'launch', dispatch: 'not-dispatched', message: 'missing binary' }, true);
  assert.equal(l.view.outcome, 'confirmed');
  assert.equal(l.view.message, 'missing binary');
});

test('nested PTC dispatches use the public durable start/result vocabulary', () => {
  const l = log();
  const dispatch = { rootCallId: 'root', parentCallId: 'parent', subCallId: 'parent:ptc:1',
    name: 'flywheel_launch', arguments: { taskDir: '/repo/docs/nested', mode: 'full' } };
  l.append('tool/ptc-dispatch-start', dispatch);
  assert.equal(l.view.outcome, 'pending');
  l.append('tool/ptc-dispatch', { ...dispatch, content: content(result('/repo/docs/nested')), isError: false });
  assert.equal(l.view.outcome, 'confirmed');
  assert.equal(l.view.operationId, 'tool:parent:ptc:1');
  assert.equal(l.view.requestedTaskDir, '/repo/docs/nested');
  assert.equal(l.view.requestedCwd, '/repo');
});

test('canonical command settlement retains its recorded alias and accepts older snapshots without origin fields', async t => {
  const directory = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-association-alias-')));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const physicalCwd = join(directory, 'project'), physicalTask = join(physicalCwd, 'docs', 'task');
  await mkdir(physicalTask, { recursive: true });
  const requestedCwd = join(directory, 'project-alias');
  await symlink(physicalCwd, requestedCwd);
  const requestedTaskDir = join(requestedCwd, 'docs', 'task');
  const binary = join(directory, 'control-fixture.mjs');
  await writeFile(binary, `#!/usr/bin/env node\nconst args=process.argv;process.stdout.write(JSON.stringify({protocol:1,ok:true,running:false,taskDir:args[args.indexOf('--task-dir')+1],cwd:args[args.indexOf('--cwd')+1],state:null,requestedTaskDir:'/forged',requestedCwd:'/forged'}));\n`, { mode: 0o755 });
  const args = { action: 'launch', taskDir: requestedTaskDir, cwd: requestedCwd, mode: 'auto' };
  const l = log();
  command(l, 'alias-create', args);
  const response = await runControl(args, { binary, defaultCwd: requestedCwd });
  assert.equal(response.taskDir, physicalTask);
  assert.equal(response.cwd, physicalCwd);
  commandDone(l, 'alias-create', response);
  assert.equal(l.view.taskDir, physicalTask);
  assert.equal(l.view.cwd, physicalCwd);
  assert.equal(l.view.requestedTaskDir, requestedTaskDir, 'the public command request owns the original identity, not response fields');
  assert.equal(l.view.requestedCwd, requestedCwd);
  assert.equal(l.view.tasks[0].requestedTaskDir, requestedTaskDir);
  assert.equal(l.view.tasks[0].requestedCwd, requestedCwd);
  const legacy = JSON.parse(JSON.stringify(l.state, (key, value) => ['requestedTaskDir', 'requestedCwd'].includes(key) ? undefined : value));
  assert.deepEqual(definition.stateSchema.parse(legacy), legacy, 'optional display evidence does not invalidate existing projection snapshots');
  assert.deepEqual(definition.wire.viewSchema.parse(legacy.view), legacy.view);
});

test('invalid arguments, unrelated tool results and earlier reads cannot change the current task', () => {
  const l = log();
  for (const args of [{ taskDir: 'relative' }, { taskDir: '/repo/task\n' }, { taskDir: '/repo/task', shell: true },
    { taskDir: '/repo/task', mode: 'fast' }, { taskDir: '/repo/task', cwd: '' }]) tool(l, 'bad', args);
  done(l, 'bad', result('/repo/task'));
  assert.equal(l.view, null);
  tool(l, 'other', { taskDir: '/repo/task' }, 'shell');
  done(l, 'other', result('/repo/task'));
  assert.equal(l.view, null);
  command(l, 'old-read', { action: 'status', taskDir: '/repo/old' });
  tool(l, 'launch', { taskDir: '/repo/new' });
  done(l, 'launch', result('/repo/new'));
  commandDone(l, 'old-read', result('/repo/old'));
  assert.equal(l.view.taskDir, '/repo/new');
});

test('cold replay restores identity while a fork ignores inherited controls and retains directory changes', () => {
  const original = log();
  original.append('working-directory/change', { cwd: '/worktree' });
  tool(original, 'parent-launch', { taskDir: '/worktree/docs/task' });
  done(original, 'parent-launch', result('/worktree/docs/task', '/worktree'));
  let replay = definition.init({ cwd: '/repo' }, 0);
  for (const event of original.events) replay = definition.apply(replay, event);
  assert.deepEqual(replay, original.state, 'a reload derives the same association from existing events');
  let fork = definition.init({ cwd: '/repo' }, original.events.length);
  for (const event of original.events) fork = definition.apply(fork, event);
  assert.equal(fork.view, null, 'inherited parent controls never grant the fork a task association');
  assert.equal(fork.cwd, '/worktree');
  fork = definition.apply(fork, { seq: original.events.length, type: 'tool/call',
    data: { callId: 'child', name: 'flywheel_launch', arguments: JSON.stringify({ taskDir: '/worktree/docs/child' }) } });
  assert.equal(fork.view.taskDir, '/worktree/docs/child');
});

test('one session keeps multiple confirmed task identities without letting history erase uncertainty', () => {
 const l = log();
 tool(l,'one',{taskDir:'/repo/docs/one'});done(l,'one',result('/repo/docs/one'));
 tool(l,'two',{taskDir:'/repo/docs/two'});done(l,'two',result('/repo/docs/two'));
 assert.deepEqual(l.view.tasks.map(task => task.taskDir),['/repo/docs/two','/repo/docs/one']);
 tool(l,'three',{taskDir:'/repo/docs/three'});
 command(l,'read-one',{action:'status',taskDir:'/repo/docs/one'});commandDone(l,'read-one',result('/repo/docs/one'));
 assert.equal(l.view.operationId,'tool:three');assert.equal(l.view.outcome,'pending');assert.equal(l.view.tasks.length,2);
 done(l,'three',result('/repo/docs/three'));
 assert.equal(l.view.tasks.length,3);
});

test('legacy UI status polling cannot create or replace a session task; explicit tools can', () => {
  const l = log();
  command(l,'poll',{action:'status',taskDir:'/repo/docs/unrelated'});
  commandDone(l,'poll',result('/repo/docs/unrelated'));
  assert.equal(l.view,null);
  assert.deepEqual(l.state.tasks,{});
  tool(l,'prepare',{action:'status',taskDir:'/repo/docs/prepared'},'flywheel_control');
  done(l,'prepare',result('/repo/docs/prepared'));
  assert.equal(l.view.taskDir,'/repo/docs/prepared');
  command(l,'poll-other',{action:'status',taskDir:'/repo/docs/unrelated'});
  commandDone(l,'poll-other',result('/repo/docs/unrelated'));
  assert.equal(l.view.taskDir,'/repo/docs/prepared');
  assert.equal(Object.keys(l.state.tasks).length,1);
  tool(l,'next',{action:'status',taskDir:'/repo/docs/next'},'flywheel_control');
  done(l,'next',result('/repo/docs/next'));
  assert.equal(l.view.taskDir,'/repo/docs/next');
});
