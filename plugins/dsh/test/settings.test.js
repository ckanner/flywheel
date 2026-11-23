import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { act, closeClientHarness, envelope, mountWorkbench } from './support/client-harness.js';

after(closeClientHarness);

function setControlValue(el, value) {
  const proto = el.tagName === 'SELECT'
    ? el.ownerDocument.defaultView.HTMLSelectElement.prototype
    : el.ownerDocument.defaultView.HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value').set;
  setter.call(el, value);
  el.dispatchEvent(new el.ownerDocument.defaultView.Event('input', { bubbles: true }));
  el.dispatchEvent(new el.ownerDocument.defaultView.Event('change', { bubbles: true }));
}

test('SettingsPage edits real Go-resolved inline loops, routes, planning refs, session reuse policy, model catalogs, and reset', async t => {
  // Matches exact resolved shape returned by `flywheel control config-edit`
  const baseValues = {
    sessions: { mode: 'scoped', 'policy-version': 1 },
    planning: {
      'dual-draft': { enabled: true, 'secondary-profile': 'opus-plan', 'synthesizer-profile': 'opus-plan' },
      compact: { 'design-plan': 'hydrogen-local', 'design-review': 'sol-plan' },
    },
    'frontend-ux-review': { enabled: true, profile: 'neon' },
    delivery: { endpoint: 'local' },
    guardrails: { 'max-packet-attempts': 3, 'max-review-rounds': 2, 'max-task-calls': 60 },
    budget: { enforce: 'off' },
    loops: {
      'design-plan': { harness: 'antigravity', channel: 'google-subscription', model: 'gemini-4-carbon-high', thinking: 'high' },
      'design-review': { harness: 'codex-pro', channel: 'chatgpt-subscription', model: 'gpt-6.2-astra', thinking: 'high' },
      plan: { harness: 'antigravity', channel: 'google-subscription', model: 'gemini-4-neon-high', thinking: 'high' },
      'plan-review': { harness: 'codex-pro', channel: 'chatgpt-subscription', model: 'gpt-6.2-sol', thinking: 'medium' },
      review: { harness: 'codex-pro', channel: 'chatgpt-subscription', model: 'gpt-6.2-sol', thinking: 'high' },
      judge: { harness: 'codex-pro', channel: 'chatgpt-subscription', model: 'gpt-6.2-astra', thinking: 'high' },
      investigate: { harness: 'antigravity', channel: 'google-subscription', model: 'gemini-4-neon-high', thinking: 'high' },
      intake: { harness: 'antigravity', channel: 'google-subscription', model: 'gemini-4-hydrogen-medium', thinking: 'medium' },
      garden: { harness: 'kimi', channel: 'kimi-subscription', model: 'kimi-code/k3.1-pro', thinking: 'high' },
    },
    routes: {
      default: { harness: 'antigravity', channel: 'google-subscription', model: 'gemini-4-hydrogen-medium', thinking: 'medium' },
      complex: { harness: 'antigravity', channel: 'google-subscription', model: 'gemini-4-neon-high', thinking: 'high' },
      frontend: { harness: 'claude-code-pro', channel: 'claude-native', model: 'claude-opus-5-5', thinking: 'high' },
      multimodal: { harness: 'antigravity', channel: 'google-subscription', model: 'gemini-4-neon-high', thinking: 'high' },
      backend: { harness: 'codex-pro', channel: 'chatgpt-subscription', model: 'gpt-6.2-sol', thinking: 'high' },
      batch: { harness: 'kimi', channel: 'kimi-subscription', model: 'kimi-code/k3.1-pro', thinking: 'high' },
      hard: { harness: 'codex-pro', channel: 'chatgpt-subscription', model: 'gpt-6.2-astra', thinking: 'high' },
    },
    profiles: {
      carbon: { harness: 'antigravity', channel: 'google-subscription', model: 'gemini-4-carbon-high', thinking: 'high' },
      neon: { harness: 'antigravity', channel: 'google-subscription', model: 'gemini-4-neon-high', thinking: 'high' },
      'hydrogen-local': { harness: 'antigravity', channel: 'google-subscription', model: 'gemini-4-hydrogen-medium', thinking: 'medium' },
      'opus-plan': { harness: 'claude-code-pro', channel: 'claude-native', model: 'claude-opus-5-5', thinking: 'high' },
      sol: { harness: 'codex-pro', channel: 'chatgpt-subscription', model: 'gpt-6.2-sol', thinking: 'high' },
      'sol-plan': { harness: 'codex-pro', channel: 'chatgpt-subscription', model: 'gpt-6.2-sol', thinking: 'medium' },
      astra: { harness: 'codex-pro', channel: 'chatgpt-subscription', model: 'gpt-6.2-astra', thinking: 'high' },
      kimi: { harness: 'kimi', channel: 'kimi-subscription', model: 'kimi-code/k3.1-pro', thinking: 'high' },
    },
    'execution-policy': {
      version: 1,
      bindings: {
        'hydrogen-local': { harness: 'antigravity', provider: '', channel: 'google-subscription', model: 'gemini-4-hydrogen-medium', thinking: 'medium', permission: 'danger-full-access', 'session-mode': 'scoped' },
        sol: { harness: 'codex-pro', provider: '', channel: 'chatgpt-subscription', model: 'gpt-6.2-sol', thinking: 'high', permission: 'danger-full-access', 'session-mode': 'scoped' },
        astra: { harness: 'codex-pro', provider: '', channel: 'chatgpt-subscription', model: 'gpt-6.2-astra', thinking: 'high', permission: 'danger-full-access', 'session-mode': 'scoped' },
      },
      routes: {
        default: { initial: 'hydrogen-local', allowed: ['hydrogen-local'], 'quality-order': [], 'after-stalled-attempts': 1, 'max-switches': 2 },
        backend: { initial: 'sol', allowed: ['sol', 'astra'], 'quality-order': [], 'after-stalled-attempts': 1, 'max-switches': 2 },
      },
    },
  };

  const ui = await mountWorkbench(
    t,
    {
      query: async input => {
        if (input.action === 'runs') {
          return envelope({ protocol: 1, ok: true, cwd: '/repo', runs: [] });
        }
        if (input.action === 'config-edit') {
          return envelope({
            protocol: 1,
            ok: true,
            cwd: '/repo',
            editor: {
              scope: input.editor.scope,
              target: input.editor.scope === 'global' ? '/home/.flywheel/config.json' : '/repo/flywheel.config.json',
              revision: `rev-${input.editor.scope}`,
              values: baseValues,
              defaults: {},
            },
          });
        }
        if (input.action === 'config') {
          return envelope({
            protocol: 1,
            ok: true,
            cwd: '/repo',
            configuration: {
              source: '/home/.flywheel/config.json',
              scope: 'global',
              bindings: [
                { id: 'loops.review', role: 'review', harness: 'codex-pro', profile: 'sol', model: 'gpt-6.2-sol', permissionMode: 'read-only' },
              ],
            },
          });
        }
        if (input.action === 'models') {
          return envelope({
            protocol: 1,
            ok: true,
            cwd: '/repo',
            modelCatalog: {
              version: 1,
              harness: input.models.harness,
              channel: input.models.channel,
              status: 'ok',
              models: [{ id: `${input.models.harness}-live-model` }],
              truncated: false,
              inferencePerformed: false,
            },
          });
        }
        if (input.action === 'doctor') {
          return envelope({
            protocol: 1,
            ok: true,
            cwd: '/repo',
            doctor: {
              kind: 'doctor',
              ok: true,
              cwd: '/repo',
              taskDir: '',
              probeRequested: false,
              inferenceTested: false,
              checks: [
                { id: 'antigravity-acp', status: 'pass', summary: 'ACP ready' },
                { id: 'claude-code-pro', status: 'pass', summary: '0.7.0 ready' },
                { id: 'codex-pro', status: 'pass', summary: '0.10.2 ready' },
                { id: 'kimi-native', status: 'pass', summary: 'Kimi ready' },
              ],
            },
          });
        }
        return envelope({ protocol: 1, ok: true, cwd: '/repo' });
      },
    },
    { slot: 'settings.section' },
  );

  // 1. Auto-matches Go-resolved inline loops to profiles and displays read-only badges
  const profileSel = ui.container.querySelector('[data-testid="role-profile-loops.design-plan"]');
  const harnessSel = ui.container.querySelector('[data-testid="role-harness-loops.design-plan"]');
  const modelInput = ui.container.querySelector('[data-testid="role-model-loops.design-plan"]');
  const thinkingSel = ui.container.querySelector('[data-testid="role-thinking-loops.design-plan"]');
  assert.equal(profileSel.value, 'carbon');
  assert.equal(harnessSel.value, 'antigravity');
  assert.equal(modelInput.value, 'gemini-4-carbon-high');
  assert.equal(thinkingSel.value, 'high');

  const health = ui.container.querySelector('[data-testid="settings-health-strip"]');
  const tier2 = ui.container.querySelector('[data-testid="settings-tier2"]');
  assert.match(tier2.textContent, /只读锁定/);

  // 2. Refresh live model catalogs & run offline Doctor preflight
  await ui.click('刷新实时模型目录');
  await act(async () => {
    await new Promise(resolve => setTimeout(resolve, 10));
  });
  assert.match(health.textContent, /1 models/);
  await ui.click('运行无推理自检 (Doctor)');
  assert.match(health.textContent, /Ready/);

  // 3. Switch loops.design-plan profile -> opus-plan, then override model & thinking inline (without mutating other stages)
  await act(async () => {
    setControlValue(profileSel, 'opus-plan');
  });
  assert.equal(harnessSel.value, 'claude-code-pro');
  assert.equal(modelInput.value, 'claude-opus-5-5');

  await act(async () => {
    setControlValue(modelInput, 'claude-opus-5-5-custom');
    setControlValue(thinkingSel, 'max');
  });

  // 4. Switch to Execution Routes tab and change routes.default profile -> sol (syncs execution-policy)
  await ui.click('执行路由 (Routes)');
  const routeDefaultSel = ui.container.querySelector('[data-testid="role-profile-routes.default"]');
  assert.equal(routeDefaultSel.value, 'hydrogen-local');
  await act(async () => {
    setControlValue(routeDefaultSel, 'sol');
  });

  // 5. Switch Session Reuse Policy from scoped -> fresh
  await ui.click('每次新建 (fresh)');

  // 6. Save global configuration and verify full Go-compatible payload
  await ui.click('保存配置');
  const saveAct = ui.actCalls.at(-1);
  assert.equal(saveAct.action, 'config-save');
  assert.equal(saveAct.editor.scope, 'global');
  assert.equal(saveAct.editor.revision, 'rev-global');
  assert.equal(saveAct.editor.values.loops['design-plan'].harness, 'claude-code-pro');
  assert.equal(saveAct.editor.values.loops['design-plan'].channel, 'claude-native');
  assert.equal(saveAct.editor.values.loops['design-plan'].model, 'claude-opus-5-5-custom');
  assert.equal(saveAct.editor.values.loops['design-plan'].thinking, 'max');
  assert.equal(saveAct.editor.values.routes.default.harness, 'codex-pro');
  assert.equal(saveAct.editor.values.routes.default.model, 'gpt-6.2-sol');
  assert.equal(saveAct.editor.values['execution-policy'].routes.default.initial, 'sol');
  assert.equal(saveAct.editor.values.sessions.mode, 'fresh');
  assert.equal(saveAct.editor.values['execution-policy'].bindings.sol['session-mode'], 'fresh');

  // 7. Switch to project scope and test reset button
  await ui.click('项目配置');
  assert.match(ui.container.querySelector('[data-testid="settings-scope-bar"]').textContent, /\/repo\/flywheel\.config\.json/);
  await ui.click('恢复默认配置');
  const resetAct = ui.actCalls.at(-1);
  assert.equal(resetAct.action, 'config-save');
  assert.equal(resetAct.editor.scope, 'project');
  assert.equal(resetAct.editor.reset, true);
});
