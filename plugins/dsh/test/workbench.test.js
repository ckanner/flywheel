import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, mountPanel, closeClientHarness } from './support/client-harness.js';

after(closeClientHarness);
const reply = (r, fields) => ({
  ok: true,
  value: {
    result: {
      kind: 'success',
      text: JSON.stringify({
        protocol: 1,
        ok: true,
        running: false,
        taskDir: r.taskDir ?? '',
        cwd: r.cwd ?? '/repo',
        state: null,
        ...fields,
      }),
    },
  },
});
const template = {
  sessions: { mode: 'scoped', 'policy-version': 1 },
  scheduler: { mode: 'serial', 'max-parallel': 2 },
  permissions: { mode: 'full-access' },
  watchdog: { 'wall-timeout-ms': 3600000, 'idle-timeout-ms': 300000 },
  budget: { enforce: 'soft', 'daily-token-limit': 2000000 },
  retry: { 'max-transport-retries': 5 },
  loops: { review: { harness: 'dsh', model: 'gpt-6.1-sol', channel: 'chatgpt', thinking: 'high' } },
  routes: {
    default: {
      harness: 'dsh',
      provider: 'deepseek-account',
      channel: 'deepseek-account',
      model: 'deepseek-flash',
      thinking: 'high',
    },
  },
  channels: { 'deepseek-account': { 'env-key': null } },
  jev: {
    enabled: false,
    endpoint: '',
    model: 'jev-1.14.0',
    'key-env': 'MEROUTER_API_KEY',
    'timeout-ms': 5000,
    'max-calls': 8,
    'min-applicability': 0.8,
    'min-choice': 0.6,
    'allowed-data': ['task-summary', 'diagnostic-facts', 'source'],
  },
};
function editorResponder() {
  const stored = {
    global: structuredClone(template),
    project: { ...structuredClone(template), sessions: { mode: 'fresh', 'policy-version': 1 } },
  };
  return (request) => {
    const scope = request.editor.scope;
    if (request.action === 'config-save')
      stored[scope] = {
        ...(request.editor.reset ? structuredClone(template) : stored[scope]),
        ...request.editor.values,
      };
    return reply(request, {
      editor: {
        scope,
        target: `/${scope}/flywheel.config.json`,
        source: `/${scope}/flywheel.config.json`,
        revision: scope + '-one',
        values: stored[scope],
        defaults: template,
        applied: request.action === 'config-save',
      },
    });
  };
}
function field(ui, label) {
  const node = [...ui.container.querySelectorAll('label')].find((node) => node.textContent === label);
  assert.ok(node, `field ${label}`);
  return ui.document.getElementById(node.htmlFor) ?? node.querySelector('input,select,textarea');
}
async function input(ui, label, value) {
  const node = field(ui, label);
  await act(async () => {
    const proto =
      node.tagName === 'SELECT'
        ? ui.document.defaultView.HTMLSelectElement.prototype
        : ui.document.defaultView.HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, 'value').set.call(node, value);
    node.dispatchEvent(
      new ui.document.defaultView.Event(node.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true }),
    );
  });
}
async function pick(ui, id) {
  await act(async () => ui.container.querySelector(`[data-node-id="${id}"]`).click());
}

test('configuration opens with one read, retains a draft across tabs, and requires an explicit scope discard', async (t) => {
  const ui = await mountPanel(t, editorResponder());
  assert.equal(ui.requests.length, 0);
  await ui.click('配置');
  assert.equal(ui.requests.length, 1);
  assert.equal(ui.requests[0].action, 'config-edit');
  assert.equal(ui.container.querySelectorAll('button').length > 0, true);
  assert.equal(
    [...ui.container.querySelectorAll('button')].some((b) => b.textContent === '保存修改'),
    false,
  );
  await input(ui, '会话策略', 'fresh');
  await ui.click('流程');
  await ui.click('配置');
  assert.equal(ui.requests.length, 1);
  assert.equal(field(ui, '会话策略').value, 'fresh');
  await ui.click('项目');
  assert.equal(ui.requests.length, 1);
  await ui.click('取消');
  assert.equal(field(ui, '会话策略').value, 'fresh');
  await ui.click('项目');
  await ui.click('放弃草稿并继续');
  assert.equal(ui.requests.at(-1).editor.scope, 'project');
  assert.equal(ui.requests.filter((r) => r.action === 'config-save').length, 0);
  assert.match(ui.container.textContent, /选择完整配置文件，不逐字段叠加/);
});

test('a failed automatic read does not loop or hide its retry, and retry does not save', async (t) => {
  let failure = true;
  const respond = editorResponder();
  const ui = await mountPanel(t, (r) => (failure ? Promise.reject(new Error('unavailable')) : respond(r)));
  await ui.click('配置');
  assert.equal(ui.requests.length, 1);
  assert.match(ui.container.textContent, /配置读取失败/);
  await ui.click('预算与重试');
  assert.equal(ui.requests.length, 1);
  failure = false;
  await ui.click('重新读取配置');
  assert.equal(ui.requests.length, 2);
  assert.equal(ui.requests[1].action, 'config-edit');
  assert.equal(field(ui, '单次运行超时').value, '60');
});

test('task navigation preserves unsaved drafts but a changed file revision cannot be saved from cache', async (t) => {
  let revision = 'one';
  const ui = await mountPanel(t, (r) =>
    reply(r, {
      editor: {
        scope: r.editor.scope,
        target: '/global/config.json',
        source: '/global/config.json',
        revision,
        values: template,
        defaults: template,
      },
    }),
  );
  await ui.selectPath(0, '/repo/docs/a');
  await ui.click('配置');
  await input(ui, '会话策略', 'fresh');
  await ui.selectPath(0, '/repo/docs/b');
  assert.equal(field(ui, '会话策略').value, 'scoped');
  await ui.selectPath(0, '/repo/docs/a');
  assert.equal(field(ui, '会话策略').value, 'fresh');
  assert.equal(ui.button('保存修改').disabled, false);
  await ui.selectPath(0, '/repo/docs/b');
  revision = 'two';
  await ui.selectPath(0, '/repo/docs/a');
  assert.equal(field(ui, '会话策略').value, 'fresh');
  assert.equal(ui.button('保存修改').disabled, true);
  assert.match(ui.container.textContent, /文件已被外部修改；草稿已保留/);
  const count = ui.requests.length;
  await ui.click('保存修改');
  assert.equal(ui.requests.length, count);
  await ui.click('放弃修改');
  assert.equal(field(ui, '会话策略').value, 'scoped');
});

test('minute conversion preserves unedited watchdog fields and invalid numeric drafts cannot save', async (t) => {
  const ui = await mountPanel(t, editorResponder());
  await ui.click('配置');
  await ui.click('预算与重试');
  assert.equal(field(ui, '单次运行超时').value, '60');
  await input(ui, '单次运行超时', '0.5');
  await ui.click('保存修改');
  assert.deepEqual(ui.requests.at(-1).editor.values, {
    watchdog: { 'wall-timeout-ms': 30000, 'idle-timeout-ms': 300000 },
  });
  assert.equal(ui.requests.at(-1).editor.reset, false);
  await input(ui, '传输重试上限', 'oops');
  assert.equal(ui.button('保存修改').disabled, true);
  assert.equal(field(ui, '传输重试上限').value, 'oops');
  const count = ui.requests.length;
  await ui.click('保存修改');
  assert.equal(ui.requests.length, count);
  await ui.click('放弃修改');
  assert.equal(field(ui, '传输重试上限').value, '5');
});

test('worktree isolation defaults on and saves an explicit off switch without altering session policy', async (t) => {
  const ui = await mountPanel(t, editorResponder());
  await ui.click('配置');
  assert.equal(field(ui, '自动管理任务 Worktree').value, 'true');
  await input(ui, '自动管理任务 Worktree', 'false');
  await ui.click('保存修改');
  assert.deepEqual(ui.requests.at(-1).editor.values, { worktree: { enabled: false } });
  assert.equal(field(ui, '会话策略').value, 'scoped');
  await input(ui, '自动管理任务 Worktree', 'true');
  await ui.click('保存修改');
  assert.deepEqual(ui.requests.at(-1).editor.values, { worktree: { enabled: true } });
});

test('serial scheduling omits concurrency and toggling parallel preserves the saved limit', async (t) => {
  const ui = await mountPanel(t, editorResponder());
  await ui.click('配置');
  assert.equal(
    [...ui.container.querySelectorAll('label')].some((l) => l.textContent === '最大并发任务'),
    false,
  );
  await input(ui, '调度策略', 'parallel');
  await input(ui, '最大并发任务', '3');
  await ui.click('保存修改');
  assert.deepEqual(ui.requests.at(-1).editor.values, { scheduler: { mode: 'parallel', 'max-parallel': 3 } });
  await input(ui, '调度策略', 'serial');
  await input(ui, '调度策略', 'parallel');
  assert.equal(field(ui, '最大并发任务').value, '3');
});

test('restoring defaults previews a full replacement and only save sends reset=true', async (t) => {
  const ui = await mountPanel(t, editorResponder());
  await ui.click('配置');
  await ui.click('恢复安装版本默认配置');
  assert.equal(ui.requests.length, 1);
  await ui.click('取消');
  assert.equal(ui.requests.length, 1);
  await ui.click('恢复安装版本默认配置');
  await ui.click('载入待保存修改');
  assert.equal(ui.requests.length, 1);
  await ui.click('保存修改');
  assert.equal(ui.requests.at(-1).editor.reset, true);
  assert.deepEqual(ui.requests.at(-1).editor.values, template);
});

test('a model dialog can be cancelled and an unchanged assignment creates no unrelated overrides', async (t) => {
  const ui = await mountPanel(t, editorResponder());
  await ui.click('配置');
  await ui.click('模型分配');
  const open = () =>
    act(async () => ui.container.querySelector('[data-binding-id="routes.default"] button').click());
  await open();
  await input(ui, '模型', 'changed-model');
  await ui.click('取消');
  assert.equal(
    [...ui.container.querySelectorAll('button')].some((b) => b.textContent === '保存修改'),
    false,
  );
  await open();
  await ui.click('加入待保存修改');
  assert.equal(
    [...ui.container.querySelectorAll('button')].some((b) => b.textContent === '保存修改'),
    false,
  );
  await open();
  await input(ui, '模型', 'deepseek-custom');
  await ui.click('加入待保存修改');
  await ui.click('保存修改');
  assert.deepEqual(Object.keys(ui.requests.at(-1).editor.values), ['routes']);
});

test('node dialogs freeze revision and a newer snapshot cannot silently reuse a confirmation', async (t) => {
  let revision = 7;
  const ui = await mountPanel(t, (r) =>
    reply(r, {
      state: {
        revision,
        status: 'paused',
        packets: { 'M01-T01': { status: 'l1-passed', evidence: { l1: { pass: true } } } },
      },
    }),
  );
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  await pick(ui, 'M01-T01');
  await ui.click('展开任务画布');
  await pick(ui, 'M01-T01/review');
  await ui.click('运行此阶段');
  assert.equal(ui.button('确认运行').disabled, false);
  revision = 8;
  await ui.click('刷新状态');
  assert.equal(ui.button('确认运行').disabled, true);
  assert.match(ui.container.querySelector('dialog').textContent, /状态已变化/);
  await ui.click('确认运行');
  assert.equal(
    ui.requests.some((r) => r.action === 'step'),
    false,
  );
});

test('shared batch and workspace association warnings remain visible without a control failure', async (t) => {
  const ui = await mountPanel(t, (r) => reply(r, {
    cwd: '/repo/task-checkout',
    workspaceWarnings: ['工作区归属未同步：registry busy'],
    state: {
      revision: 9, status: 'paused', project: { 'workspace-cwd': '/repo' },
      'shared-execution': { packets: ['M01-T01', 'M01-T02'], 'last-error': 'commit hook rejected delivery' },
      packets: { 'M01-T01': { status: 'l1-passed' }, 'M01-T02': { status: 'executed' } },
    },
  }));
  await ui.selectPath(0, '/repo/task-checkout/docs/task');
  await ui.click('刷新状态');
  assert.match(ui.container.textContent, /共享目录批次/);
  assert.match(ui.container.textContent, /commit hook rejected delivery/);
  assert.match(ui.container.textContent, /registry busy/);
  assert.equal(ui.requests.length, 1, 'rendering pending collective delivery never launches another task');
});

test('locate moves to a running task outside the current page without executing it', async (t) => {
  const packets = Object.fromEntries(
    Array.from({ length: 8 }, (_, i) => [`M01-T0${i + 1}`, { status: i === 7 ? 'executing' : 'completed' }]),
  );
  const ui = await mountPanel(t, (r) => reply(r, { state: { revision: 4, status: 'paused', packets } }));
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  assert.equal(ui.container.querySelector('[data-node-id="M01-T08"]'), null);
  await ui.click('定位当前步骤');
  assert.ok(ui.container.querySelector('[data-node-id="M01-T08"]'));
  assert.match(ui.container.querySelector('section[aria-label="节点详情"]').textContent, /M01-T08/);
  assert.equal(ui.requests.length, 1);
});

test('focus canvas is an owned dialog that exits without starting or stopping a task', async (t) => {
  const ui = await mountPanel(t, (r) =>
    reply(r, { state: { revision: 4, status: 'paused', packets: { 'M01-T01': { status: 'pending' } } } }),
  );
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  await ui.click('专注画布');
  assert.ok(ui.container.querySelector('dialog[aria-label="专注画布"][open]'));
  await ui.click('退出专注画布');
  assert.equal(ui.container.querySelector('dialog'), null);
  assert.equal(ui.requests.length, 1);
  assert.equal(ui.document.activeElement, ui.button('专注画布'));
});

test('Quick verification requires its final gate rather than painting L1 alone green', async (t) => {
  let gate = { pass: false };
  const ui = await mountPanel(t, (r) =>
    reply(r, {
      state: {
        revision: 4,
        status: 'paused',
        workflow: { effective: 'quick' },
        packets: { 'M01-T01': { status: 'l1-passed', evidence: { l1: { pass: true }, 'final-gate': gate } } },
      },
    }),
  );
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  const node = () => ui.container.querySelector('[data-node-id="M01-T01/verify"]');
  assert.equal(node().getAttribute('data-tone'), 'error');
  gate = undefined;
  await ui.click('刷新状态');
  assert.notEqual(node().getAttribute('data-tone'), 'success');
  gate = { pass: true };
  await ui.click('刷新状态');
  assert.equal(node().getAttribute('data-tone'), 'success');
  assert.equal(
    ui.requests.some((r) => r.action === 'step'),
    false,
  );
});

test('a waiting dependency remains visible and cannot be bypassed by the task run button', async (t) => {
  const ui = await mountPanel(t, (r) =>
    reply(r, {
      state: {
        revision: 4,
        status: 'paused',
        scheduler: { mode: 'parallel' },
        packets: {
          'M01-T01': { status: 'pending' },
          'M01-T02': { status: 'pending', schedule: { after: ['M01-T01'], writes: ['src/'] } },
        },
      },
    }),
  );
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  await pick(ui, 'M01-T02');
  assert.match(
    ui.container.querySelector('section[aria-label="运行画布"]').textContent,
    /并行执行 · 串行集成/,
  );
  assert.equal(ui.button('运行此任务').disabled, true);
  assert.match(ui.container.textContent, /等待依赖任务交付/);
  await ui.click('运行此任务');
  assert.equal(ui.requests.length, 1);
});

test('run comparison never redirects metrics to the reference task and all-calls scope is explicit', async (t) => {
  const rows = ['alpha', 'beta'].map((id) => ({
    id,
    taskDir: '/repo/docs/' + id,
    cwd: '/repo',
    startedAt: '2026-11-10',
    conditions: id,
    summary: { slug: id, status: 'paused' },
  }));
  const ui = await mountPanel(
    t,
    (r) => reply(r, r.action === 'metrics' ? { metrics: { groups: [] } } : { runs: rows }),
    { catalog: (r) => reply(r, { runs: rows }) },
  );
  await ui.click('运行记录');
  await act(async () =>
    [...ui.container.querySelectorAll('nav[aria-label="运行记录"] button')]
      .find((b) => b.textContent.startsWith('beta'))
      .click(),
  );
  const reference = [...ui.container.querySelectorAll('label')]
    .find((l) => l.textContent.startsWith('对照运行'))
    .querySelector('select');
  await act(async () => {
    reference.value = 'alpha';
    reference.dispatchEvent(new ui.document.defaultView.Event('change', { bubbles: true }));
  });
  await ui.click('读取所选运行指标');
  assert.equal(ui.requests.at(-1).runId, 'beta');
  assert.equal(ui.requests.at(-1).taskDir, '/repo/docs/beta');
  const scope = [...ui.container.querySelectorAll('label')]
    .find((l) => l.textContent.startsWith('指标范围'))
    .querySelector('select');
  await act(async () => {
    scope.value = 'task';
    scope.dispatchEvent(new ui.document.defaultView.Event('change', { bubbles: true }));
  });
  await ui.click('读取所选运行指标');
  assert.equal(ui.requests.at(-1).runId, undefined);
  assert.equal(ui.requests.at(-1).taskDir, '/repo/docs/beta');
});

test('Review never becomes runnable from a red L1 and remote closed PR is not a merge receipt', async (t) => {
  const ui = await mountPanel(t, (r) =>
    reply(r, {
      state: {
        revision: 4,
        status: 'paused',
        packets: { 'M01-T01': { status: 'l1-failed', evidence: { l1: { pass: false } } } },
      },
      ...(r.action === 'delivery'
        ? {
            delivery: {
              repo: 'github.com/example/repo',
              url: 'https://github.com/example/repo',
              branch: 'feature',
              head: 'one',
              pulls: [{ number: 1, headRefOid: 'one', state: 'CLOSED' }],
            },
          }
        : {}),
    }),
  );
  await ui.selectPath(0, '/repo/docs/task');
  await ui.click('刷新状态');
  await pick(ui, 'M01-T01');
  await ui.click('展开任务画布');
  await pick(ui, 'M01-T01/review');
  assert.equal(ui.button('运行此阶段').disabled, true);
  assert.match(ui.container.textContent, /需先通过本次确定性验证/);
  await ui.click('返回任务全貌');
  await pick(ui, 'delivery');
  await ui.click('查询项目与 PR');
  await ui.click('展开阶段');
  assert.equal(ui.container.querySelector('[data-node-id="merge"]').getAttribute('data-tone'), 'idle');
  assert.match(ui.container.querySelector('[data-node-id="delivery"]').textContent, /PR 已关闭/);
});


test('JEV request limits and fractional confidence thresholds survive the unified configuration save', async t => {
  const ui = await mountPanel(t, editorResponder());
  await ui.click('配置');
  await ui.click('JEV 决策');
  await input(ui, '启用 JEV', 'true');
  await input(ui, 'MeRouter System One API 地址', 'https://jev.example/v1/systemone');
  await input(ui, '固定模型版本', 'jev-1.15.0');
  await input(ui, '密钥环境变量名（不要填密钥）', 'JEV_TEST_API_KEY');
  await input(ui, '请求超时', '2.5');
  await input(ui, '每项任务最多 JEV 请求次数', '6');
  await input(ui, '最低适用概率', '1.1');
  assert.equal(ui.button('保存修改').disabled, true);
  assert.match(ui.container.textContent, /请输入大于 0 且不超过 1 的概率/);
  await input(ui, '最低适用概率', '0.85');
  await input(ui, '最低选择概率', '0');
  assert.equal(ui.button('保存修改').disabled, true);
  await input(ui, '最低选择概率', '0.65');
  await ui.click('保存修改');
  assert.deepEqual(ui.requests.at(-1).editor.values, { jev: {
    enabled: true, endpoint: 'https://jev.example/v1/systemone', model: 'jev-1.15.0', 'key-env': 'JEV_TEST_API_KEY',
    'timeout-ms': 2500, 'max-calls': 6, 'min-applicability': 0.85, 'min-choice': 0.65,
    'allowed-data': ['task-summary', 'diagnostic-facts', 'source'],
  } });
  assert.equal(ui.container.querySelector('dialog'), null);
  assert.match(ui.container.textContent, /source/);
});
