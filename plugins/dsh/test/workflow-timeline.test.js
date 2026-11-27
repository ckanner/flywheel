import test from 'node:test';
import assert from 'node:assert/strict';
import { FlywheelRemoteClient, remoteContribution } from '../src/client/remote.ts';
import { validPlanningCommand } from '../src/planning-control.js';
import {
  annotatePlanSteps,
  buildMilestoneTimeline,
  formatRetryPlanningLabel,
  resolvePlanGroupTone,
  resolveRetryPlanningTarget,
} from '../src/client/workflowTimeline.ts';

function makeTrace(overrides = {}) {
  return {
    id: overrides.id ?? 't-1',
    key: overrides.key ?? 'plan/planning:milestone/M-02',
    role: overrides.role ?? 'plan',
    scope: overrides.scope ?? 'planning:milestone/M-02',
    status: overrides.status ?? 'completed',
    running: overrides.running ?? false,
    startedAt: overrides.startedAt ?? '2026-11-27T11:03:50.000Z',
    updatedAt: overrides.updatedAt ?? '2026-11-27T11:12:34.000Z',
    durationMs: overrides.durationMs ?? 60000,
    preview: overrides.preview ?? '',
    ...overrides,
  };
}

test('PILLAR-1: resolveRetryPlanningTarget prefers engine projection and never hardcodes design for milestone M-02', () => {
  const projected = resolveRetryPlanningTarget({
    phase: 'budget-stopped',
    'start-authorized': true,
    projection: {
      kind: 'console-excerpt',
      'retry-target': {
        scope: 'milestone',
        'scope-id': 'M-02',
        'continue-execution': true,
        'failed-request-id': 'engine-1-14',
        'cancellable-request-id': 'plan-retry-mistake',
      },
    },
    requests: [{ id: 'plan-retry-mistake', scope: 'design', status: 'running' }],
  });
  assert.deepEqual(projected, {
    scope: 'milestone',
    scopeId: 'M-02',
    continueExecution: true,
    failedRequestId: 'engine-1-14',
    cancellableRequestId: 'plan-retry-mistake',
  });
  assert.equal(formatRetryPlanningLabel(projected, true), '重试 M-02 拆包规划');
  assert.equal(formatRetryPlanningLabel(projected, false), 'Retry M-02 Plan');

  // Legacy fallback without projection still derives milestone M-02 accurately
  const fallback = resolveRetryPlanningTarget({
    phase: 'budget-stopped',
    'start-authorized': true,
    requests: [
      { id: 'engine-1-14', scope: 'milestone', 'scope-id': 'M-02', status: 'budget-stopped' },
    ],
  });
  assert.equal(fallback.scope, 'milestone');
  assert.equal(fallback.scopeId, 'M-02');
  assert.equal(fallback.continueExecution, true);

  // Design failure before execution authorization targets design without continue-execution
  const designTarget = resolveRetryPlanningTarget({
    phase: 'blocked',
    'start-authorized': false,
    requests: [{ id: 'engine-1-1', scope: 'design', status: 'blocked' }],
  });
  assert.equal(designTarget.scope, 'design');
  assert.equal(designTarget.continueExecution, false);
  assert.equal(formatRetryPlanningLabel(designTarget, true), '重新尝试架构设计');
});

test('PILLAR-5: annotatePlanSteps distinguishes rounds, schema retries, and localized verdicts', () => {
  const steps = annotatePlanSteps(
    [
      makeTrace({ id: 'r1-p', role: 'plan', startedAt: '2026-11-27T11:03:50Z' }),
      makeTrace({ id: 'r1-r', role: 'plan-review', startedAt: '2026-11-27T11:12:34Z', preview: '审查结论：revise · 5 项缺陷' }),
      makeTrace({ id: 'r2-p1', role: 'plan', startedAt: '2026-11-27T11:15:18Z' }),
      makeTrace({ id: 'r2-p2', role: 'plan', startedAt: '2026-11-27T11:22:29Z' }),
      makeTrace({ id: 'r2-r', role: 'plan-review', startedAt: '2026-11-27T11:23:48Z', preview: '{"verdict":"pass"}' }),
    ],
    true,
  );
  assert.deepEqual(
    steps.map(s => [s.title, s.statusLabel, s.tone]),
    [
      ['第 1 轮 · 实施计划拆解', '已完成', 'ok'],
      ['第 1 轮 · 实施计划审查', '需修订', 'warn'],
      ['第 2 轮 · 实施计划拆解', '校验重试', 'warn'],
      ['第 2 轮 · 实施计划拆解（校验重试 #1）', '已完成', 'ok'],
      ['第 2 轮 · 实施计划审查', '审查通过', 'ok'],
    ],
  );

  assert.deepEqual(resolvePlanGroupTone(steps, 'completed', false, true), { tone: 'ok', statusLabel: '已完成' });
  assert.deepEqual(resolvePlanGroupTone(steps, 'budget-stopped', false, true), { tone: 'danger', statusLabel: '已达轮次上限' });
  assert.deepEqual(resolvePlanGroupTone(steps, 'running', true, true), { tone: 'running', statusLabel: '进行中' });
});

test('PILLAR-5: buildMilestoneTimeline orders M-01 closed -> M-02 active -> M-03 pending chronologically', () => {
  const m02PlanTrace = makeTrace({ id: 'm02-live', role: 'plan', scope: 'planning:milestone/M-02', running: true });
  const m01JudgeTrace = makeTrace({ id: 'm01-judge', role: 'judge', scope: 'M-01', running: false });
  const res = buildMilestoneTimeline({
    milestonesMap: {
      'M-01': { status: 'closed' },
      'M-02': { status: 'in-progress' },
      'M-03': { status: 'pending' },
    },
    designMilestones: [
      { id: 'M-01', title: 'Scaffold' },
      { id: 'M-02', title: 'Core Engine' },
      { id: 'M-03', title: 'Canvas Renderer' },
    ],
    currentMilestone: 'M-02',
    packets: [
      { id: 'M01-T01', status: 'completed' },
      { id: 'M01-T02', status: 'completed' },
    ],
    planGroups: [
      {
        scope: 'planning:milestone/M-02',
        requestId: 'milestone/M-02',
        milestoneId: 'M-02',
        isMilestonePlan: true,
        label: '里程碑拆包规划 · M-02',
        triggerPacketId: null,
        triggerSummary: null,
        items: [m02PlanTrace],
        steps: annotatePlanSteps([m02PlanTrace], true),
        running: true,
        tone: 'running',
        statusLabel: '进行中',
        totalDurationMs: 60000,
      },
    ],
    traces: [m01JudgeTrace, m02PlanTrace],
    taskStatus: 'running',
    planningPhase: 'drafting',
  });

  assert.deepEqual(res.milestones.map(m => m.id), ['M-01', 'M-02', 'M-03']);
  assert.equal(res.milestones[0].isClosed, true);
  assert.equal(res.milestones[0].completedPackets, 2);
  assert.equal(res.milestones[0].verificationTraces.length, 1);
  assert.equal(res.milestones[1].isRunning, true);
  assert.equal(res.milestones[1].planningGroups.length, 1);
  assert.equal(res.milestones[2].isPending, true);
});

test('PILLAR-5: remoteContribution accepts milestone action and translates HTTP 404 to installation.json guidance', async () => {
  const contribution = remoteContribution();
  const actDesc = contribution.descriptors.find(d => d.method === 'act');
  const codec = actDesc.parameters[0].codec.create();
  assert.doesNotThrow(() => codec.parse({ action: 'milestone' }));

  const ctx = {
    get: () => ({
      query: async () => {
        throw new Error('transport failure for /api/flywheelConsole/query: HTTP 404');
      },
      act: async () => ({}),
      listRepoImages: async () => ({}),
    }),
  };
  const client = new FlywheelRemoteClient(ctx, Promise.resolve(undefined));
  await assert.rejects(() => client.query({ action: 'runs' }), /installation\.json/);
});

test('PILLAR-1: host planning-control validates cancel-plan and plan continue-execution boundaries', () => {
  assert.equal(validPlanningCommand({
    action: 'cancel-plan',
    revision: 12,
    'request-id': 'cancel-1',
    'target-request-id': 'plan-retry-123',
    start: false,
  }), true);
  assert.equal(validPlanningCommand({
    action: 'cancel-plan',
    revision: 12,
    'request-id': 'cancel-2',
    'target-request-id': 'bad id!',
  }), false);
  assert.equal(validPlanningCommand({
    action: 'plan',
    revision: 12,
    'request-id': 'retry-m02',
    scope: 'milestone',
    'scope-id': 'M-02',
    'continue-execution': true,
    start: true,
  }), true);
  assert.equal(validPlanningCommand({
    action: 'plan',
    revision: 12,
    'request-id': 'retry-design-bad',
    scope: 'design',
    'continue-execution': true,
  }), false);
});
