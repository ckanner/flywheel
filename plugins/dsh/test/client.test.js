import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, closeClientHarness, envelope, mountWorkbench } from './support/client-harness.js';

after(closeClientHarness);

test('v2 registers 4 native DSH slots, deduplicates nested Go runs in top Task Switcher, opens inline Settings tab, and jumps from right sidebar & command card', async t => {
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          runs: [
            // Nested Go summary format + duplicate taskDir + temp run
            {
              taskDir: '/Users/dev/meclaw-alert/docs/alert-triage-loop',
              workspaceCwd: '/Users/dev/meclaw-alert',
              cwd: '/Users/dev/meclaw-alert-wt/alert-triage-loop',
              workflow: 'full',
              startedAt: '2026-11-18T11:44:09.973Z',
              finishedAt: '2026-11-18T13:25:02.160Z',
              summary: { slug: 'alert-triage-loop', status: 'paused', revision: 38 },
            },
            {
              taskDir: '/Users/dev/meclaw-alert/docs/alert-triage-loop',
              workspaceCwd: '/Users/dev/meclaw-alert',
              cwd: '/Users/dev/meclaw-alert',
              workflow: 'full',
              startedAt: '2026-11-17T10:00:00.000Z',
              finishedAt: '2026-11-17T10:20:00.000Z',
              summary: { slug: 'alert-triage-loop', status: 'escalated', revision: 12 },
            },
            {
              taskDir: '/private/tmp/flywheel-0342-ui-demo/docs/add',
              workspaceCwd: '/private/tmp/flywheel-0342-ui-demo',
              cwd: '/private/tmp/flywheel-0342-ui-demo',
              summary: { slug: 'add', status: 'pending', revision: 6 },
            },
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
      if (input.action === 'config-edit') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          editor: { scope: 'global', revision: 'rev-1', values: {}, defaults: {} },
        });
      }
      if (input.action === 'config') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          configuration: { source: '/home/.config/flywheel/flywheel.config.json', scope: 'global', bindings: [] },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', revisions: [] });
    },
  });

  // 1. Verify 4 native slots, zero redundant left rail, running task selected first
  assert.equal(ui.slots.has('conversation.view'), false);
  assert.equal(ui.container.querySelector('[data-testid="flywheel-task-rail"]'), null);
  assert.equal(ui.sidebarTabs.length, 1);
  assert.match(ui.container.textContent, /Alpha Launch/);
  assert.ok(ui.button('停止运行'));
  assert.ok(ui.button('+ 新建任务'));

  // 2. Open Task Switcher popover: deduplicates alert-triage-loop (shows "2 次运行"), hides /private/tmp demo by default
  const switcher = ui.container.querySelector('[data-testid="flywheel-task-switcher"]');
  assert.ok(switcher);
  await act(async () => {
    switcher.click();
  });
  const popover = ui.container.querySelector('[data-testid="flywheel-task-popover"]');
  assert.ok(popover);
  assert.match(popover.textContent, /alert-triage-loop/);
  assert.match(popover.textContent, /2 次运行/);
  assert.match(popover.textContent, /meclaw-alert/);
  assert.equal(popover.textContent.includes('flywheel-0342-ui-demo'), false, 'temp runs hidden by default');

  // Toggling temp checkbox reveals the temp run
  const checkbox = popover.querySelector('input[type="checkbox"]');
  assert.ok(checkbox);
  await act(async () => {
    checkbox.click();
  });
  assert.match(popover.textContent, /add/);

  // Close popover by selecting Beta Fix
  const betaBtn = [...popover.querySelectorAll('button')].find(b => b.textContent.includes('Beta Fix'));
  assert.ok(betaBtn);
  await act(async () => {
    betaBtn.click();
    await Promise.resolve();
    await Promise.resolve();
  });
  assert.equal(ui.container.querySelector('[data-testid="flywheel-task-popover"]'), null);

  // 3. Inline Settings tab works directly inside Workbench
  const settingsTab = ui.container.querySelector('[data-testid="flywheel-settings-tab"]');
  assert.ok(settingsTab);
  await act(async () => {
    settingsTab.click();
    await Promise.resolve();
    await Promise.resolve();
  });
  assert.ok(ui.container.querySelector('[data-testid="settings-health-strip"]'));

  // Switch back to Execution tab & test English locale
  await ui.click('执行流水线');
  await ui.setLocale('en');
  assert.ok(ui.button('Execution Board'));
  assert.ok(ui.button('Settings'));

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
