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

test('CreateTaskModal produces valid host-validated planning prepare & investigate payloads for Build, Quick, and Investigate', async t => {
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
      // Feed the exact client act payload into the REAL host control validator
      const validated = await validateControl(input, repoDir);
      return envelope({
        protocol: 1,
        ok: true,
        cwd: validated.cwd,
        taskDir: validated.taskDir,
      });
    },
  });

  // 1. Build (Auto, review-first) with image attachment
  await ui.click('+ 新建任务');
  let modal = ui.container.querySelector('[data-testid="flywheel-create-modal"]');
  assert.ok(modal);

  const cwdInput = modal.querySelector('input[placeholder="/absolute/path/to/repo"]');
  const goalInput = modal.querySelector('textarea');
  const slugInput = modal.querySelector('input[placeholder="auto-generated"]');

  await act(async () => {
    setControlValue(cwdInput, repoDir);
    setControlValue(goalInput, '重构告警聚合队列并补充回归测试');
    setControlValue(slugInput, 'alert queue fix');
  });

  await ui.click('扫描仓库图片');
  await ui.click('docs/mockup.png');

  await submitModal(modal);

  const buildAct = ui.actCalls.at(-1);
  assert.equal(buildAct.action, 'planning');
  assert.equal(buildAct.planning.action, 'prepare');
  assert.equal('policy' in buildAct.planning, false, 'policy must live inside brief, not top-level planning');
  assert.equal(buildAct.planning.start, true);
  assert.equal(buildAct.planning.brief.version, 1);
  assert.equal(buildAct.planning.brief.id, 'alert-queue-fix');
  assert.equal(buildAct.planning.brief.goal, '重构告警聚合队列并补充回归测试');
  assert.equal(buildAct.planning.brief['original-request'], '重构告警聚合队列并补充回归测试');
  assert.equal(buildAct.planning.brief.policy, 'review-first');
  assert.equal(buildAct.planning.brief['implementation-authorized'], false);
  assert.equal(buildAct.planning.brief.mode, 'auto');
  assert.equal('images' in buildAct.planning.brief, false, 'brief must not carry raw images key');
  assert.ok(buildAct.planning.brief.evidence.some(entry => entry.includes('docs/mockup.png')));
  assert.equal(ui.container.querySelector('[data-testid="flywheel-create-modal"]'), null, 'modal closes on success');

  // 2. Quick Fix (delegated with explicit authorization)
  await ui.click('+ 新建任务');
  modal = ui.container.querySelector('[data-testid="flywheel-create-modal"]');
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

  // 3. Investigate intent
  await ui.click('+ 新建任务');
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

  await ui.click('+ 新建任务');
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
