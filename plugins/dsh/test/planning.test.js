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

  // Main stream shows inline approval card & attention strip; opening Design drawer shows full DesignStudio
  assert.ok(ui.container.querySelector('[data-testid="flywheel-main-stream"]'));
  assert.ok(ui.container.querySelector('[data-testid="main-approval-card"]'));
  await ui.click('展开完整设计稿与双稿对比 →');
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

test('v2 DesignStudio handles real Go planning.phase=drafting live state, keeps Design tab active, and syncs task switcher status', async t => {
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          runs: [{
            taskDir: '/repo/docs/aws-usage',
            cwd: '/repo',
            slug: 'aws-usage',
            workflow: 'planning',
            summary: { slug: 'aws-usage', status: 'pending', 'planning-phase': 'drafting', revision: 1 },
          }],
        });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: '/repo/docs/aws-usage',
          running: true,
          state: {
            slug: 'aws-usage',
            status: 'pending',
            revision: 3,
            packets: {},
            planning: {
              version: 1,
              phase: 'drafting',
              policy: 'review-first',
              rounds: 1,
              'max-rounds': 3,
              brief: {
                id: 'aws-usage',
                goal: '新增 AWS 日本区每日用量统计并合并到现有云资源推送与多维表格',
                'original-request': '新增 AWS 日本区每日用量统计并合并到现有云资源推送与多维表格',
                policy: 'review-first',
                mode: 'auto',
              },
              requests: [{
                id: 'engine-1-1',
                scope: 'design',
                stage: 'planner',
                role: 'design-plan',
                channel: 'google-subscription',
                status: 'running',
                rounds: 1,
                retries: 0,
              }],
              designs: [],
              reviews: [],
              approvals: [],
            },
          },
        });
      }
      if (input.action === 'traces') {
        return envelope({
          protocol: 1,
          ok: true,
          taskDir: '/repo/docs/aws-usage',
          traces: [
            {
              id: 'live-p1',
              key: 'design-plan/planning:engine-1-1:primary',
              role: 'design-plan',
              scope: 'planning:engine-1-1:primary',
              harness: 'antigravity',
              model: 'gemini-4-carbon-high',
              thinking: 'high',
              status: 'running',
              running: true,
              preview: '正在扫描 ../loveops AWS 标签规范…',
            },
            {
              id: 'live-p2',
              key: 'design-plan/planning:engine-1-1:secondary',
              role: 'design-plan',
              scope: 'planning:engine-1-1:secondary',
              harness: 'claude-code-pro',
              model: 'claude-opus-5-5',
              thinking: 'high',
              status: 'running',
              running: true,
              preview: '正在分析 ../meclaw-apps CronJob 配置…',
            },
          ],
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', revisions: [] });
    },
  });

  // Stays on Main Stream showing live dual-draft cards; opening Design drawer shows DesignStudio live view
  assert.ok(ui.container.querySelector('[data-testid="flywheel-main-stream"]'));
  assert.ok(ui.container.querySelector('[data-testid="agent-card-live-p1"]'));
  assert.ok(ui.container.querySelector('[data-testid="agent-card-live-p2"]'));
  assert.match(ui.container.textContent, /Gemini 4 Carbon 高/);
  assert.match(ui.container.textContent, /Claude Opus 5\.5/);
  assert.match(ui.container.innerHTML, /gemini-4-carbon-high/);
  assert.match(ui.container.innerHTML, /claude-opus-5-5/);

  // Header status and Task Switcher popover item are synchronized to 规划中 (never stale 待开始)
  assert.equal(ui.container.querySelector('[data-testid="flywheel-header-status"]').textContent, '规划中');
  await ui.click(/aws-usage/);
  const popover = ui.container.querySelector('[data-testid="flywheel-task-popover"]');
  assert.ok(popover);
  assert.match(popover.textContent, /规划中/);
  assert.doesNotMatch(popover.textContent, /待开始/);

  // Opening Design drawer renders brief, stepper & live cards; DAG drawer shows pre-execution guidance back to Design
  await ui.click('展开完整设计稿与双稿对比 →');
  assert.ok(ui.container.querySelector('[data-testid="flywheel-design-studio"]'));
  assert.ok(ui.container.querySelector('[data-testid="design-brief-card"]'));
  assert.ok(ui.container.querySelector('[data-testid="design-stepper"]'));
  assert.ok(ui.container.querySelector('[data-testid="design-live-cards"]'));

  await ui.click('DAG 图谱');
  assert.ok(ui.container.querySelector('[data-testid="flywheel-exec-empty"]'));
  assert.match(ui.container.textContent, /尚未进入执行阶段/);
  await ui.click('返回设计评审查看进度');
  assert.ok(ui.container.querySelector('[data-testid="flywheel-design-studio"]'));
});

test('v2 DesignStudio surfaces real Go planning.phase=blocked diagnostics and dispatches valid retry planning.plan', async t => {
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          runs: [{
            taskDir: '/repo/docs/blocked-task',
            cwd: '/repo',
            slug: 'blocked-task',
            workflow: 'planning',
            summary: { slug: 'blocked-task', status: 'paused', 'planning-phase': 'blocked', revision: 19 },
          }],
        });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: '/repo/docs/blocked-task',
          running: false,
          state: {
            slug: 'blocked-task',
            status: 'paused',
            revision: 19,
            packets: {},
            planning: {
              version: 1,
              phase: 'blocked',
              policy: 'review-first',
              reason: '规划调用/输出重试已耗尽：TRANSPORT invalid request: Antigravity requires subscription-only policy version 1',
              brief: {
                id: 'blocked-task',
                goal: '测试阻塞诊断展示',
                'original-request': '测试阻塞诊断展示',
                policy: 'review-first',
                mode: 'auto',
              },
              requests: [{
                id: 'engine-1-1',
                scope: 'design',
                stage: 'planner',
                role: 'design-plan',
                channel: 'google-subscription',
                status: 'blocked',
                retries: 8,
              }],
              designs: [],
              reviews: [],
              approvals: [],
            },
          },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', revisions: [], traces: [] });
    },
  });

  assert.equal(ui.container.querySelector('[data-testid="flywheel-header-status"]').textContent, '规划阻塞');
  const mainBlocked = ui.container.querySelector('[data-testid="main-planning-blocked"]');
  assert.ok(mainBlocked);
  assert.match(mainBlocked.textContent, /Antigravity requires subscription-only policy version 1/);

  await ui.click('展开完整设计稿与双稿对比 →');
  assert.ok(ui.container.querySelector('[data-testid="flywheel-design-studio"]'));
  const banner = ui.container.querySelector('[data-testid="design-blocked-banner"]');
  assert.ok(banner);
  assert.match(banner.textContent, /Antigravity requires subscription-only policy version 1/);
  assert.match(banner.textContent, /已重试 8 次/);

  await ui.click('重新尝试规划');
  const lastAct = ui.actCalls.at(-1);
  assert.equal(lastAct.action, 'planning');
  assert.equal(lastAct.planning.action, 'plan');
  assert.equal(lastAct.planning.scope, 'design');
  assert.equal(lastAct.planning.revision, 19);
  assert.equal(lastAct.planning.start, true);
  assert.equal(validPlanningCommand(lastAct.planning), true);
});

test('v2 DesignStudio renders real Go designs[], reviews[], structured requirements/decisions, quote-finding feedback, and approval', async t => {
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          runs: [{
            taskDir: '/repo/docs/go-design',
            cwd: '/repo',
            slug: 'go-design',
            workflow: 'planning',
            summary: { slug: 'go-design', status: 'paused', 'planning-phase': 'awaiting-design-approval', revision: 9 },
          }],
        });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: '/repo/docs/go-design',
          running: false,
          state: {
            slug: 'go-design',
            status: 'paused',
            revision: 9,
            packets: {},
            planning: {
              version: 1,
              phase: 'awaiting-design-approval',
              policy: 'review-first',
              brief: { id: 'go-design', goal: '合并 AWS 与火山引擎日报', 'original-request': '合并 AWS 与火山引擎日报', policy: 'review-first', mode: 'auto' },
              'current-design': { revision: 1, digest: designRef.digest },
              designs: [{
                ref: { revision: 1, digest: designRef.digest },
                document: '# 统一云资源日报架构\n\n抽象 `CloudUsageFetcher` 统一采集。',
                requirements: [{ id: 'REQ-1', title: '拉取 AWS ap-northeast-1 费用', milestone: 'M01', verification: 'go test ./internal/cloud/...' }],
                decisions: [{ id: 'DEC-1', kind: 'fixed', statement: '跨仓仅只读检查', rationale: '避免越界修改基础设施仓库' }],
                milestones: [{ id: 'M01', title: 'AWS 采集与推送上线', gate: 'go test ./...' }],
              }],
              reviews: [{
                id: 'rev-astra-1',
                kind: 'design',
                target: { revision: 1, digest: designRef.digest },
                binding: 'gpt-6.2-astra / xhigh',
                verdict: 'pass',
                summary: '边界清晰，无阻断风险。',
                findings: [{ id: 'F-ADV-1', severity: 'advisory', status: 'open', target: 'Cost Explorer 缓存', impact: '建议同日幂等缓存降低调用费用' }],
              }],
              approvals: [],
              requests: [],
            },
          },
        });
      }
      if (input.action === 'design-candidates') {
        return envelope({
          protocol: 1,
          ok: true,
          taskDir: '/repo/docs/go-design',
          revisions: [{
            revision: 1,
            primary: '## Carbon Primary',
            secondary: '## Opus Secondary',
            meta: {
              revision: 1,
              primary: { profile: 'carbon', harness: 'antigravity', model: 'gemini-4-carbon-high' },
              secondary: { profile: 'opus-plan', harness: 'claude-code-pro', model: 'claude-opus-5-5' },
              synthesizer: { profile: 'opus-plan', harness: 'claude-code-pro', model: 'claude-opus-5-5' },
            },
          }],
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', traces: [] });
    },
  });

  assert.equal(ui.container.querySelector('[data-testid="flywheel-header-status"]').textContent, '待你拍板');
  await ui.click('展开完整设计稿与双稿对比 →');
  assert.match(ui.container.textContent, /统一云资源日报架构/);
  assert.match(ui.container.querySelector('[data-testid="design-review-panel"]').textContent, /边界清晰，无阻断风险/);
  assert.match(ui.container.querySelector('[data-testid="design-review-panel"]').textContent, /Cost Explorer 缓存/);

  // Structured view renders real Go requirements, decisions, and milestones
  await ui.click('需求与决策表');
  const structured = ui.container.querySelector('[data-testid="design-structured-view"]');
  assert.ok(structured);
  assert.match(structured.textContent, /REQ-1/);
  assert.match(structured.textContent, /DEC-1/);
  assert.match(structured.textContent, /M01/);

  // Side-by-side renders nested Go meta.json primary/secondary models
  await ui.click('双稿并排对比');
  const split = ui.container.querySelector('[data-testid="design-side-by-side"]');
  assert.match(split.textContent, /gemini-4-carbon-high/);
  assert.match(split.textContent, /claude-opus-5-5/);

  // Quote finding opens feedback textarea pre-populated with finding summary
  await ui.click('引用提意见');
  const ta = ui.container.querySelector('[data-testid="flywheel-side-drawer"] textarea');
  assert.ok(ta);
  assert.match(ta.value, /Cost Explorer 缓存/);
  await ui.click('取消');

  // Approve & Start uses real Go current-design revision + digest
  await ui.click('批准设计并立即执行');
  const lastAct = ui.actCalls.at(-1);
  assert.equal(lastAct.action, 'planning');
  assert.equal(lastAct.planning.action, 'approve');
  assert.equal(lastAct.planning.revision, 9);
  assert.equal(lastAct.planning.design.revision, 1);
  assert.equal(lastAct.planning.design.digest, designRef.digest);
  assert.equal(lastAct.planning.start, true);
  assert.equal(validPlanningCommand(lastAct.planning), true);
});
