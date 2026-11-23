import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, closeClientHarness, envelope, mountWorkbench } from './support/client-harness.js';

after(closeClientHarness);

test('ExecutionBoard shows Wave Swimlanes (default), renders all 12 DAG nodes without pagination, highlights lineage, and opens 4-tab NodeInspector with UX audit', async t => {
  const packets = {};
  for (let i = 1; i <= 12; i++) {
    const id = `M01-T${String(i).padStart(2, '0')}`;
    packets[id] = {
      id,
      goal: `Implement module ${i}`,
      status: i <= 3 ? 'done' : i === 4 ? 'running' : 'pending',
      route: i === 4 ? 'frontend' : 'default',
      profile: i === 4 ? 'opus-frontend' : 'hydrogen-local',
      dependsOn: i === 1 ? [] : i <= 4 ? ['M01-T01'] : i <= 8 ? ['M01-T04'] : ['M01-T05'],
      l1Status: i <= 3 ? 'pass' : 'pending',
      uxStatus: i === 4 ? 'needs_polish' : undefined,
      reviewStatus: i <= 3 ? 'pass' : 'pending',
      sessionKey: i === 4 ? 'task:M01-T04' : undefined,
      unknownInvocation: i === 4 ? 'inv-99' : undefined,
      writeScopes: ['plugins/dsh/src/client/'],
      requirements: ['Must preserve responsive layout'],
    };
  }

  let budgetEnforce = 'off';
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({ protocol: 1, ok: true, cwd: '/repo', runs: [{ taskDir: '/repo/docs/big', cwd: '/repo', slug: 'big', status: 'running', running: true }] });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: '/repo/docs/big',
          running: true,
          state: {
            slug: 'big',
            title: 'Large Pipeline',
            status: 'running',
            revision: 5,
            packets,
            guardrails: { maxPacketAttempts: 3, maxReviewRounds: 2, maxTaskCalls: 60, currentCalls: 14 },
            budget: { enforce: budgetEnforce, spentUsd: 1.5, limitUsd: 10 },
          },
        });
      }
      if (input.action === 'node') {
        return envelope({
          protocol: 1,
          ok: true,
          node: {
            packet: packets[input.packet],
            l1: [{ name: 'typecheck', passed: true, output: '0 errors' }],
            review: { verdict: 'PASS', blocking: [], advisory: ['Consider extracting helper'] },
            timeline: [{ at: '2026-11-23T02:00:00Z', event: 'execute:start', detail: 'Dispatched packet' }],
            contract: { id: input.packet, route: 'frontend' },
          },
        });
      }
      if (input.action === 'ux-review') {
        return envelope({
          protocol: 1,
          ok: true,
          packet: input.packet,
          uxReview: {
            verdict: 'needs_polish',
            summary: 'Clarify primary CTA copy',
            refined: true,
            refinedBy: 'claude-sonnet',
            findings: [{ severity: 'advisory', location: 'Header.tsx', summary: 'Shorten CTA label', suggestion: 'Use Approve & Start' }],
          },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', revisions: [] });
    },
    act: async input => envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir: input.taskDir }),
  });

  // 1. Default view is Wave Swimlanes; guardrails shown; dollar budget banner HIDDEN when enforce === 'off'
  assert.ok(ui.container.querySelector('[data-testid="flywheel-wave-swimlanes"]'));
  assert.ok(ui.container.querySelector('[data-testid="flywheel-guardrails-strip"]'));
  assert.equal(ui.container.querySelector('[data-testid="flywheel-budget-banner"]'), null, 'dollar budget banner must stay hidden when budget.enforce is off');
  assert.match(ui.container.textContent, /14\/60/);

  // 2. Switch to DAG Graph: ALL 12 nodes render simultaneously with ZERO 6-node pagination
  await ui.click('DAG 图谱');
  const dag = ui.container.querySelector('[data-testid="flywheel-dag-canvas"]');
  assert.ok(dag);
  const renderedNodes = dag.querySelectorAll('button[data-packet-id]');
  assert.equal(renderedNodes.length, 12, 'DAG canvas must render all 12 packets without legacy 6-node pagination');

  // Zoom button updates zoom label
  await ui.click('+');
  assert.equal(ui.container.querySelector('[data-testid="dag-zoom-label"]').textContent, '115%');
  await ui.click('重置视图');
  assert.equal(ui.container.querySelector('[data-testid="dag-zoom-label"]').textContent, '100%');

  // 3. Click M01-T04 to open 520px 4-tab Node Inspector
  await act(async () => {
    dag.querySelector('button[data-packet-id="M01-T04"]').click();
    await Promise.resolve();
    await Promise.resolve();
  });
  const inspector = ui.container.querySelector('[data-testid="flywheel-node-inspector"]');
  assert.ok(inspector);
  assert.match(inspector.textContent, /Consider extracting helper/);

  // Check Evidence & UX tab renders Neon UX Audit report
  await ui.click('证据与 UX');
  assert.match(inspector.textContent, /Clarify primary CTA copy/);
  assert.match(inspector.textContent, /Shorten CTA label/);
  assert.match(inspector.textContent, /0 errors/);

  // Check Timeline & Contract tabs
  await ui.click('时间线与协作');
  assert.match(inspector.textContent, /Dispatched packet/);
  await ui.click('契约');
  assert.match(inspector.textContent, /plugins\/dsh\/src\/client\//);

  // Manual step execute from Overview tab
  await ui.click('概览');
  await ui.click('单步执行');
  assert.equal(ui.actCalls.at(-1).action, 'step');
  assert.equal(ui.actCalls.at(-1).packet, 'M01-T04');
  assert.equal(ui.actCalls.at(-1).only, 'execute');

  // 4. Opt-in budget.enforce === 'hard' reveals dollar budget banner
  budgetEnforce = 'hard';
  await act(async () => {
    await ui.store.refreshSelectedTask();
  });
  assert.ok(ui.container.querySelector('[data-testid="flywheel-budget-banner"]'));
});
