import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { closeClientHarness, envelope, mountWorkbench } from './support/client-harness.js';

after(closeClientHarness);

test('v2 registers 4 native DSH slots, switches locales, collapses task rail, and jumps from right sidebar & command card', async t => {
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          runs: [
            { taskDir: '/repo/docs/alpha', cwd: '/repo', slug: 'alpha', title: 'Alpha Launch', status: 'running', running: true, mode: 'full' },
            { taskDir: '/repo/docs/beta', cwd: '/repo', slug: 'beta', title: 'Beta Fix', status: 'pending', running: false, mode: 'quick' },
          ],
        });
      }
      if (input.action === 'status') {
        const isBeta = input.taskDir === '/repo/docs/beta';
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: input.taskDir,
          running: !isBeta,
          state: {
            slug: isBeta ? 'beta' : 'alpha',
            title: isBeta ? 'Beta Fix' : 'Alpha Launch',
            status: isBeta ? 'pending' : 'running',
            mode: isBeta ? 'quick' : 'full',
            packets: {
              'M01-T01': { id: 'M01-T01', goal: 'Scaffold API', status: 'done' },
              'M01-T02': { id: 'M01-T02', goal: 'Wire UI', status: isBeta ? 'pending' : 'running' },
            },
          },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', revisions: [] });
    },
  });

  // 1. Verify 4 native slots and header CTA for running task
  assert.equal(ui.slots.has('conversation.view'), false);
  assert.equal(ui.sidebarTabs.length, 1);
  assert.match(ui.container.textContent, /Alpha Launch/);
  assert.ok(ui.button('停止运行'));

  // 2. Collapse and expand 240px task rail
  await ui.click('«');
  assert.equal(ui.container.textContent.includes('新建任务'), false);
  await ui.click('»');
  assert.ok(ui.button('+ 新建任务'));

  // 3. Switch locale to English
  await ui.setLocale('en');
  assert.ok(ui.button('Stop Run'));
  assert.ok(ui.button('Execution Board'));

  // 4. Right sidebar companion slot shows progress and navigates to workbench via layout.selectPanel
  await ui.switchSlot('sidebar.right.pane.tab');
  assert.match(ui.container.textContent, /1\/2 packets complete/);
  await ui.click('Open Flywheel Workbench');
  assert.deepEqual(ui.selectedPanels, ['flywheel']);

  // 5. Command card slot renders read-only summary and jumps to workbench
  await ui.switchSlot('conversation.chat.commandview', {
    result: {
      kind: 'success',
      text: JSON.stringify({ protocol: 1, ok: true, action: 'status', taskDir: '/repo/docs/beta' }),
    },
  });
  assert.ok(ui.container.querySelector('[data-testid="flywheel-command-card"]'));
  await ui.click('Open in Workbench');
  assert.deepEqual(ui.selectedPanels, ['flywheel', 'flywheel']);
});
