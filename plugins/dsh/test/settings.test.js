import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { closeClientHarness, envelope, mountWorkbench } from './support/client-harness.js';

after(closeClientHarness);

test('SettingsPage renders 3-tier settings, 4-subscription health strip, read-only role locks, and saves validated config', async t => {
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
              values: {
                planning: { 'dual-draft': { enabled: true } },
                'frontend-ux-review': { enabled: true },
                delivery: { endpoint: 'local' },
                guardrails: { 'max-packet-attempts': 3, 'max-review-rounds': 2, 'max-task-calls': 60 },
                budget: { enforce: 'off' },
              },
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
                { id: 'loops.review', role: 'review', harness: 'codex-pro', profile: 'codex-astra', model: 'gpt-6.2-astra', permissionMode: 'read-only' },
                { id: 'routes.default', role: 'execute', harness: 'kimi', profile: 'hydrogen-local', model: 'kimi-k2.6', permissionMode: 'workspace-write' },
              ],
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

  // Scope Precedence Bar + 4-Subscription Health Strip + Tier 2 read-only badge
  assert.ok(ui.container.querySelector('[data-testid="settings-scope-bar"]'));
  const health = ui.container.querySelector('[data-testid="settings-health-strip"]');
  assert.ok(health);
  assert.match(health.textContent, /Antigravity \(ACP\)/);
  assert.match(health.textContent, /Claude Code Pro/);
  assert.match(health.textContent, /Codex Pro/);
  assert.match(health.textContent, /Kimi Native/);

  const tier2 = ui.container.querySelector('[data-testid="settings-tier2"]');
  assert.match(tier2.textContent, /只读锁定/);
  assert.match(tier2.textContent, /gpt-6\.2-astra/);

  // Trigger offline Doctor preflight
  await ui.click('运行无推理自检 (Doctor)');
  assert.match(health.textContent, /Ready/);

  // Switch scope to Project and save configuration
  await ui.click('项目配置');
  assert.match(ui.container.querySelector('[data-testid="settings-scope-bar"]').textContent, /\/repo\/flywheel\.config\.json/);

  await ui.click('保存配置');
  const saveAct = ui.actCalls.at(-1);
  assert.equal(saveAct.action, 'config-save');
  assert.equal(saveAct.editor.scope, 'project');
  assert.equal(saveAct.editor.revision, 'rev-project');
});
