import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, closeClientHarness, envelope, mountWorkbench } from './support/client-harness.js';

after(closeClientHarness);

function setInputValue(el, value) {
  const proto = el.tagName === 'TEXTAREA'
    ? el.ownerDocument.defaultView.HTMLTextAreaElement.prototype
    : el.ownerDocument.defaultView.HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value').set;
  setter.call(el, value);
  el.dispatchEvent(new el.ownerDocument.defaultView.Event('input', { bubbles: true }));
}

test('CreateTaskModal creates one-sentence Investigation tasks without raw JSON, scans repo images, and DeliveryTrack requires explicit confirmation', async t => {
  const runsList = [{ taskDir: '/repo/docs/inv-1', cwd: '/repo', slug: 'inv-1', title: 'Login Bug', status: 'ready' }];
  const ui = await mountWorkbench(t, {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          runs: runsList,
        });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: '/repo/docs/inv-1',
          running: false,
          state: {
            slug: 'inv-1',
            title: 'Login Bug',
            status: 'ready',
            revision: 2,
            investigation: {
              status: 'ready',
              revision: 2,
              goal: 'Why does refresh fail after token expiry?',
              hypotheses: [{ id: 'H1', title: 'Clock skew', status: 'open' }],
              actions: [{ id: 'A1', invocationId: 'inv-01', summary: 'Checked auth.go', status: 'done' }],
            },
            delivery: { endpoint: 'draft-pr', branch: 'feat/login-fix', baseBranch: 'main' },
          },
        });
      }
      if (input.action === 'delivery') {
        return envelope({
          protocol: 1,
          ok: true,
          delivery: {
            endpoint: 'draft-pr',
            repo: 'ckanner/flywheel',
            branch: 'feat/login-fix',
            baseBranch: 'main',
            head: 'abc1234567890',
            pulls: [{ number: 88, title: 'Fix token refresh', state: 'OPEN', draft: true, checks: [{ name: 'ci', conclusion: 'SUCCESS' }] }],
          },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo' });
    },
    act: async input => {
      if (input.action === 'investigate' && input.investigation?.action === 'start' && input.taskDir) {
        runsList.unshift({ taskDir: input.taskDir, cwd: input.cwd ?? '/repo', slug: 'new-inv', title: 'New Inv', status: 'ready' });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir: input.taskDir ?? '' });
    },
    listRepoImages: async () => envelope({ protocol: 1, ok: true, cwd: '/repo', files: [{ path: 'docs/ui.png', bytes: 1024, ext: 'png' }] }),
  });

  // 1. InvestigationBoard renders hypotheses & evidence log automatically
  assert.ok(ui.container.querySelector('[data-testid="flywheel-investigation-board"]'));
  assert.match(ui.container.textContent, /Clock skew/);
  assert.match(ui.container.textContent, /Checked auth\.go/);

  // 2. Open 30s CreateTaskModal, scan images on Build, then create 1-sentence Investigation
  await ui.click('+ 新建任务');
  const modal = ui.container.querySelector('[data-testid="flywheel-create-modal"]');
  assert.ok(modal);
  await ui.click('扫描仓库图片');
  assert.match(modal.textContent, /docs\/ui\.png/);

  await act(async () => {
    modal.querySelector('button[data-intent="investigate"]').click();
  });
  const textarea = modal.querySelector('textarea');
  await act(async () => {
    setInputValue(textarea, 'Investigate why SSE drops after 30s');
  });
  await ui.click('创建并启动');
  const invAct = ui.actCalls.at(-1);
  assert.equal(invAct.action, 'investigate');
  assert.equal(invAct.investigation.action, 'start');
  assert.equal(invAct.investigation.goal, 'Investigate why SSE drops after 30s');
  assert.equal(invAct.investigation.intent, 'investigate');
  assert.equal(invAct.investigation.run, true);

  // 3. Switch to Delivery tab and test explicit confirmation for Create Draft PR & Merge PR
  await ui.click('交付追踪');
  assert.ok(ui.container.querySelector('[data-testid="flywheel-delivery-track"]'));
  assert.match(ui.container.textContent, /draft-pr/);
  assert.match(ui.container.textContent, /#88/);

  await ui.click('创建 Draft PR');
  await ui.click('确认执行');
  const createPrAct = ui.actCalls.at(-1);
  assert.equal(createPrAct.action, 'deliver');
  assert.equal(createPrAct.delivery.action, 'create');
  assert.equal(createPrAct.delivery.confirmed, true);
  assert.equal(createPrAct.delivery.draft, true);

  await ui.click('确认合并 PR #88');
  await ui.click('确认执行');
  const mergeAct = ui.actCalls.at(-1);
  assert.equal(mergeAct.action, 'deliver');
  assert.equal(mergeAct.delivery.action, 'merge');
  assert.equal(mergeAct.delivery.confirmed, true);
  assert.equal(mergeAct.delivery.number, 88);
  assert.equal(mergeAct.delivery.expectedHead, 'abc1234567890');
});
