import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { validateControl } from '../src/control.js';
import { act, closeClientHarness, envelope, mountWorkbench } from './support/client-harness.js';

after(closeClientHarness);

function setControlValue(el, value) {
  const win = el.ownerDocument.defaultView;
  const proto =
    el.tagName === 'SELECT'
      ? win.HTMLSelectElement.prototype
      : el.tagName === 'TEXTAREA'
        ? win.HTMLTextAreaElement.prototype
        : win.HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value').set;
  setter.call(el, value);
  el.dispatchEvent(new win.Event('input', { bubbles: true }));
  el.dispatchEvent(new win.Event('change', { bubbles: true }));
}

async function submitModal(modal) {
  await act(async () => {
    modal.dispatchEvent(new modal.ownerDocument.defaultView.Event('submit', { bubbles: true, cancelable: true }));
    await new Promise(resolve => setTimeout(resolve, 30));
  });
}

test('New-Task Chat composer and CreateTaskModal produce valid host-validated planning prepare & investigate payloads', async t => {
  const repoDir = await mkdtemp(join(tmpdir(), 'fw-create-task-'));
  t.after(() => rm(repoDir, { recursive: true, force: true }));

  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({ protocol: 1, ok: true, cwd: repoDir, runs: [] });
      }
      return envelope({ protocol: 1, ok: true, cwd: repoDir });
    },
    listRepoImages: async () =>
      envelope({
        protocol: 1,
        ok: true,
        cwd: repoDir,
        files: [{ path: 'docs/mockup.png', sizeBytes: 1234, mediaType: 'image/png' }],
      }),
    act: async input => {
      const validated = await validateControl(input, repoDir);
      return envelope({
        protocol: 1,
        ok: true,
        cwd: validated.cwd,
        taskDir: validated.taskDir,
      });
    },
  });

  // 1. Empty state opens directly on the DSH New-Task Chat screen
  assert.ok(ui.container.querySelector('[data-testid="flywheel-new-task-chat"]'));
  await ui.click('图片');
  await ui.click(/从仓库已有图片选/);
  await ui.click('docs/mockup.png');
  await act(async () => {
    setControlValue(
      ui.container.querySelector('[data-testid="flywheel-chat-input"]'),
      '重构告警聚合队列并补充回归测试',
    );
  });
  await act(async () => {
    ui.container.querySelector('[data-testid="flywheel-chat-send"]').click();
    await new Promise(resolve => setTimeout(resolve, 20));
  });

  const buildAct = ui.actCalls.at(-1);
  assert.equal(buildAct.action, 'planning');
  assert.equal(buildAct.planning.action, 'prepare');
  assert.equal('policy' in buildAct.planning, false, 'policy must live inside brief, not top-level planning');
  assert.equal(buildAct.planning.start, true);
  assert.equal(buildAct.planning.brief.version, 1);
  assert.equal(buildAct.planning.brief.goal, '重构告警聚合队列并补充回归测试');
  assert.equal(buildAct.planning.brief['original-request'], '重构告警聚合队列并补充回归测试');
  assert.equal(buildAct.planning.brief.policy, 'review-first');
  assert.equal(buildAct.planning.brief['implementation-authorized'], false);
  assert.equal(buildAct.planning.brief.mode, 'auto');
  assert.equal('images' in buildAct.planning.brief, false, 'brief must not carry raw images key');
  assert.ok(buildAct.planning.brief.evidence.some(entry => entry.includes('docs/mockup.png')));

  // 2. Quick Fix via CreateTaskModal (still supported when triggered via store)
  await act(async () => {
    ui.store.setCreateModalOpen(true);
  });
  let modal = ui.container.querySelector('[data-testid="flywheel-create-modal"]');
  assert.ok(modal);
  await act(async () => {
    modal.querySelector('[data-intent="quick"]').click();
  });
  await act(async () => {
    setControlValue(modal.querySelector('input[placeholder="/absolute/path/to/repo"]'), repoDir);
    setControlValue(modal.querySelector('textarea'), 'Fix nil pointer in config loader');
    setControlValue(modal.querySelector('input[placeholder="auto-generated"]'), 'quick-nil-fix');
  });
  await submitModal(modal);

  const quickAct = ui.actCalls.at(-1);
  assert.equal(quickAct.planning.brief.policy, 'delegated');
  assert.equal(quickAct.planning.brief['implementation-authorized'], true);
  assert.equal(quickAct.planning.brief.mode, 'quick');
  assert.match(quickAct.planning.brief.authorization, /Fix nil pointer in config loader/);
  assert.equal(ui.container.querySelector('[data-testid="flywheel-create-modal"]'), null, 'modal closes on success');

  // 3. Investigate intent via modal with read-paths
  await act(async () => {
    ui.store.setCreateModalOpen(true);
  });
  modal = ui.container.querySelector('[data-testid="flywheel-create-modal"]');
  await act(async () => {
    modal.querySelector('[data-intent="investigate"]').click();
  });
  await act(async () => {
    setControlValue(modal.querySelector('input[placeholder="/absolute/path/to/repo"]'), repoDir);
    setControlValue(modal.querySelector('textarea'), 'Investigate sudden latency spike in webhook worker');
    setControlValue(modal.querySelector('input[placeholder="src/, internal/"]'), 'internal/machine, internal/drivers');
  });
  await submitModal(modal);

  const invAct = ui.actCalls.at(-1);
  assert.equal(invAct.action, 'investigate');
  assert.equal(invAct.investigation.action, 'start');
  assert.equal(invAct.investigation.intent, 'investigate');
  assert.deepEqual(invAct.investigation['read-paths'], ['internal/machine', 'internal/drivers']);
});

test('CreateTaskModal surfaces host errors inline inside the dialog', async t => {
  const repoDir = await mkdtemp(join(tmpdir(), 'fw-create-err-'));
  t.after(() => rm(repoDir, { recursive: true, force: true }));

  const ui = await mountWorkbench(t, {
    query: async input => envelope({ protocol: 1, ok: true, cwd: repoDir, ...(input.action === 'runs' ? { runs: [] } : {}) }),
    act: async () => {
      throw new Error('task directory already exists');
    },
  });

  await act(async () => {
    ui.store.setCreateModalOpen(true);
  });
  const modal = ui.container.querySelector('[data-testid="flywheel-create-modal"]');
  await act(async () => {
    setControlValue(modal.querySelector('input[placeholder="/absolute/path/to/repo"]'), repoDir);
    setControlValue(modal.querySelector('textarea'), 'Test error display');
  });
  await submitModal(modal);

  const errBanner = ui.container.querySelector('[data-testid="create-modal-error"]');
  assert.ok(errBanner, 'error must be visible inside modal');
  assert.match(errBanner.textContent, /task directory already exists/);
});
