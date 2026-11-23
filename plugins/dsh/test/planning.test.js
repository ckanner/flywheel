import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, realpath, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Context } from '@deepseek-ai/cordis';
import { SessionStore } from '@deepseek-ai/dsh-session';
import { SessionProjectionRegistry } from '@deepseek-ai/dsh-session-projection';
import { runControl, ControlNotDispatchedError, validateControl } from '../src/control.js';
import { apply, inject } from '../src/index.js';
import { validPlanningCommand, planningUserProjection as projection, planningUserAction } from '../src/planning-control.js';
import { closeClientHarness, envelope, mountWorkbench } from './support/client-harness.js';
import { designRef } from './support/planning-fixture.js';

after(closeClientHarness);

const binary = fileURLToPath(new URL('./fixtures/fake-planning-control.js', import.meta.url));
const approval = { action: 'approve', revision: 7, 'request-id': 'approval-1', design: designRef, start: false };
const request = (taskDir, planning = approval) => ({ action: 'planning', taskDir, cwd: taskDir, planning });

function env(t, name, value) {
  const old = process.env[name];
  process.env[name] = value;
  t.after(() => (old === undefined ? delete process.env[name] : (process.env[name] = old)));
}

async function directory(t) {
  const dir = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-planning-')));
  t.after(() => rm(dir, { recursive: true, force: true }));
  return dir;
}

test('planning control validates exact typed commands and keeps provenance out of model JSON', async t => {
  const dir = await directory(t);
  for (const value of [
    approval,
    { action: 'prepare', brief: {} },
    { action: 'plan', revision: 7, 'request-id': 'plan-1', scope: 'milestone', 'scope-id': 'M01' },
    { action: 'feedback', revision: 7, 'request-id': 'feedback-1', text: 'Use a durable checkpoint', design: designRef },
    { action: 'status' },
    { action: 'migrate', revision: 7, 'request-id': 'migration-1', policy: 'review-first' },
  ]) {
    assert.equal(validPlanningCommand(value), true);
    await validateControl(request(dir, value));
  }
  assert.equal(validPlanningCommand({ action: 'status', 'request-id': 'feedback-1' }), true);
  assert.equal(validPlanningCommand({ action: 'plan', revision: 7, 'request-id': 'plan-local', scope: 'milestone', 'scope-id': 'M-01', text: 'Expand restart recovery without changing the API.' }), true);
  for (const value of [
    { ...approval, actor: 'user' },
    { ...approval, source: 'dsh' },
    { ...approval, revision: 0 },
    { ...approval, 'request-id': '' },
    { ...approval, design: { ...designRef, digest: 'latest' } },
    { action: 'feedback', revision: 7, 'request-id': 'f1', text: '' },
    { action: 'plan', revision: 7, 'request-id': 'p1', scope: 'execute' },
    { action: 'status', start: true },
    { ...approval, start: 'yes' },
  ]) {
    assert.equal(validPlanningCommand(value), false);
    await assert.rejects(validateControl(request(dir, value)));
  }
  await assert.rejects(validateControl({ action: 'status', taskDir: dir, cwd: dir, planning: { action: 'status' } }));
});

test('planning approval proof comes from an exact live user command projection and excludes forks and consumed events', () => {
  const input = request('/repo'), raw = JSON.stringify(input), session = { id: 'desktop-session' };
  const event = { type: 'command/run', seq: 0, data: { name: 'flywheel', commandId: 'c1', args: raw, source: { kind: 'user' } } };
  const state = projection.apply(projection.init({}, 0), event);
  assert.equal(projection.stateSchema.safeParse(state).success, true);
  assert.deepEqual(planningUserAction(input, session, 'c1', raw, state), { kind: 'dsh', sessionId: session.id, commandId: 'c1', 'request-id': 'approval-1', revision: 7, design: designRef, start: false });
  for (const invalid of [
    projection.init({}, 0),
    projection.apply(projection.init({}, 1), event),
    projection.apply(state, event),
    projection.apply(state, { type: 'command/done', seq: 1, data: { commandId: 'c1' } }),
    projection.apply(projection.init({}, 0), { ...event, data: { ...event.data, source: { kind: 'model' } } }),
  ]) {
    assert.throws(() => planningUserAction(input, session, 'c1', raw, invalid), /unconsumed/);
  }
  assert.throws(() => planningUserAction(input, session, 'c1', raw + ' ', state));
  assert.throws(() => planningUserAction(input, undefined, 'c1', raw, state));
  assert.equal(planningUserAction(request('/repo', { action: 'status' }), session), undefined);
});

test('the process boundary strips inherited approval and forwards only host-verified exact proof', async t => {
  const dir = await directory(t), log = join(dir, 'calls.jsonl');
  env(t, 'TEST_PLANNING_LOG', log);
  env(t, 'FLYWHEEL_PLANNING_USER_ACTION', 'inherited-forgery');
  const options = { binary, hostSessionID: 'desktop-session' };
  await runControl(request(dir, { action: 'plan', revision: 7, 'request-id': 'plan-1', scope: 'design', start: true }), options);
  await assert.rejects(runControl(request(dir), options), ControlNotDispatchedError);
  const proof = { kind: 'dsh', sessionId: 'desktop-session', commandId: 'c1', 'request-id': 'approval-1', revision: 7, design: designRef, start: false };
  await assert.rejects(runControl(request(dir), { ...options, planningUserAction: { ...proof, start: true } }), ControlNotDispatchedError);
  await runControl(request(dir), { ...options, planningUserAction: proof });
  const calls = (await readFile(log, 'utf8')).trim().split('\n').map(JSON.parse);
  assert.equal(calls.length, 2);
  assert.equal(calls[0].proof, undefined);
  assert.equal(calls[0].command.scope, 'design');
  assert.deepEqual(JSON.parse(calls[1].proof), proof);
  assert.deepEqual(calls[1].command, approval);
});

test('published DSH host accepts a user command, associates planning, and rejects tool and fork approval', async t => {
  const dir = await directory(t), ctx = new Context(), tools = new Map();
  let command;
  t.after(() => ctx.fiber.dispose());
  await ctx.plugin(SessionStore);
  await ctx.plugin(SessionProjectionRegistry);
  ctx.provide('commands', { register: value => { command = value; } });
  ctx.provide('tools', { register: value => tools.set(value.name, value) });
  ctx.provide('workingDirectory', { get: () => dir });
  await ctx.plugin({ apply, inject }, { flywheelBinary: binary });
  const session = ctx.sessions.create('approval-session', { meta: { cwd: dir } });
  const input = request(dir), raw = JSON.stringify(input);
  const absent = await command.handler({ rawInput: raw, commandId: 'c1', agent: { session } });
  assert.equal(JSON.parse(absent.text).dispatch, 'not-dispatched');
  session.append('command/run', { commandId: 'c1', name: 'flywheel', args: raw, source: { kind: 'user' } });
  const result = await command.handler({ rawInput: raw, commandId: 'c1', agent: { session } });
  assert.equal(result.kind, 'success');
  const fork = ctx.sessions.create('fork', { seed: session.snapshotEvents(), inheritedEventCount: session.seq, meta: { cwd: dir, isSeeded: true } });
  assert.equal((await command.handler({ rawInput: raw, commandId: 'c1', agent: { session: fork } })).kind, 'error');
  session.append('command/done', { commandId: 'c1', ...result });
  assert.equal(ctx.sessionProjections.snapshot(session).values.flywheelTask.taskDir, dir);
  const definition = tools.get('flywheel_control');
  assert.equal(definition.parameters.properties.planning.properties.action.enum.includes('approve'), false);
  await assert.rejects(definition.execute(input, { callId: 't1', arguments: input, agent: { session } }), /model tools cannot approve/);
});

test('v2 DesignStudio renders synthesized Markdown table, Side-by-Side Carbon vs Opus drafts, Astra findings, and human-only approval via Direct RPC act', async t => {
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({ protocol: 1, ok: true, cwd: '/repo', runs: [{ taskDir: '/repo/docs/arch', cwd: '/repo', slug: 'arch', title: 'Dual-Draft Arch', status: 'awaiting_approval', mode: 'full' }] });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: '/repo/docs/arch',
          running: false,
          state: {
            slug: 'arch',
            title: 'Dual-Draft Arch',
            status: 'awaiting_approval',
            revision: 7,
            mode: 'full',
            planning: {
              status: 'awaiting_approval',
              revision: 7,
              design: {
                revision: 2,
                digest: designRef.digest,
                approved: false,
                markdown: '# Synthesized Plan\n\n| Decision | Source |\n| --- | --- |\n| Direct RPC | Carbon + Opus |\n',
                findings: [{ id: 'F1', severity: 'major', status: 'open', title: 'Keep read-only review sandbox', summary: 'Verify permissions' }],
              },
            },
          },
        });
      }
      if (input.action === 'design-candidates') {
        return envelope({
          protocol: 1,
          ok: true,
          taskDir: '/repo/docs/arch',
          revisions: [{
            revision: 2,
            primary: '## Carbon Proposal\nFast modular layers.',
            secondary: '## Opus Proposal\nStrict CAS boundaries.',
            meta: { revision: 2, primaryModel: 'gemini-4-carbon', secondaryModel: 'claude-opus-5.5', synthesizerProfile: 'opus-plan' },
          }],
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo' });
    },
  });

  // Auto-selects Design Studio tab when awaiting_approval
  assert.ok(ui.container.querySelector('[data-testid="flywheel-design-studio"]'));
  assert.ok(ui.container.querySelector('table'), 'synthesized Markdown GFM table rendered');
  assert.match(ui.container.textContent, /Direct RPC/);
  assert.match(ui.container.querySelector('[data-testid="design-review-panel"]').textContent, /Keep read-only review sandbox/);

  // Switch to Side-by-Side view to inspect Carbon vs Opus drafts
  await ui.click('双稿并排对比');
  const split = ui.container.querySelector('[data-testid="design-side-by-side"]');
  assert.ok(split);
  assert.match(split.textContent, /Carbon Proposal/);
  assert.match(split.textContent, /Opus Proposal/);

  // Approve & Start from sticky bar dispatches Direct RPC act with exact revision + digest + start: true
  await ui.click('批准设计并立即执行');
  const lastAct = ui.actCalls.at(-1);
  assert.equal(lastAct.action, 'planning');
  assert.equal(lastAct.planning.action, 'approve');
  assert.equal(lastAct.planning.revision, 7);
  assert.equal(lastAct.planning.design.digest, designRef.digest);
  assert.equal(lastAct.planning.start, true);
});
