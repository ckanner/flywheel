import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { act, clientDocument, closeClientHarness, envelope, loadClient } from './support/client-harness.js';

after(closeClientHarness);

const React = await import('react');
const ReactDOM = await import('react-dom');
const ReactDOMClient = await import('react-dom/client');
const JSX = await import('react/jsx-runtime');
const hostRequire = createRequire(
  process.env.FLYWHEEL_TEST_DSH_HOST_DIR
    ? resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR, 'package.json')
    : new URL('../package.json', import.meta.url),
);
const Cordis = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/cordis')));
const Slots = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/dsh-client-ui-slots')));
const rendererSource = await readFile(hostRequire.resolve('@deepseek-ai/dsh-client-ui-renderer/client'), 'utf8');

function publishedRenderer() {
  const shared = {
    react: React,
    'react-dom': ReactDOM,
    'react-dom/client': ReactDOMClient,
    'react/jsx-runtime': JSX,
    '@deepseek-ai/cordis': Cordis,
    '@deepseek-ai/dsh-client-ui-slots': Slots,
  };
  let renderer;
  vm.runInNewContext(rendererSource, {
    window: {
      __ModuleLoader__: {
        load(definition) {
          assert.equal(definition.id, '@deepseek-ai/dsh-client-ui-renderer');
          renderer = definition.factory(name => {
            assert.ok(name in shared, `published renderer dependency ${name} must be supplied`);
            return shared[name];
          });
        },
      },
    },
    console,
    Error,
    queueMicrotask,
    process,
  });
  return renderer;
}

test('published DSH renderer owns the v2 4-slot plugin lifecycle without conversation.view or import-time DOM writes', async t => {
  const container = clientDocument.createElement('div');
  clientDocument.body.appendChild(container);
  const headBefore = clientDocument.head.innerHTML;
  const bodyChildrenBefore = clientDocument.body.childElementCount;

  const client = loadClient();
  assert.equal(clientDocument.head.innerHTML, headBefore, 'evaluating the plugin cannot write to document.head');
  assert.equal(clientDocument.body.childElementCount, bodyChildrenBefore, 'evaluating the plugin cannot append a body surface');

  const ctx = new Cordis.Context();
  let unmount;
  t.after(async () => {
    await act(async () => {
      unmount?.();
      await ctx.fiber.dispose();
    });
    container.remove();
  });

  await ctx.plugin(publishedRenderer());
  const crashes = [];
  ctx.effect(() => ctx.slots.onEntryError((key, _entry, error) => crashes.push({ key, error })));

  const dictionaries = new Map();
  const localeRevision = { revision: 0 };
  const locale = {
    bind: namespace => (key, params = {}) =>
      String(dictionaries.get(namespace)?.zh?.[key] ?? key).replace(/\{(\w+)\}/g, (match, name) =>
        name in params ? String(params[name]) : match,
      ),
    register: (namespace, values) => {
      dictionaries.set(namespace, values);
      return () => dictionaries.delete(namespace);
    },
    getSnapshot: () => localeRevision,
    subscribe: () => () => {},
  };
  ctx.provide('locale', locale);
  ctx.slots.installLocale(locale);
  const noopSnap = Object.freeze({ key: undefined, hooks: {} });
  const noopSource = { getSnapshot: () => noopSnap, subscribe: () => () => {} };
  ctx.slots.installScope('session', {
    current: noopSource,
    bindingSource: () => noopSource,
    renderArea: (_value, props) => props.children({}),
  });
  ctx.slots.provideRoot({});

  const selectedPanels = [];
  const sidebarTabSpecs = [];
  const flywheelConsole = {
    query: async input => {
      if (input.action === 'runs') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          runs: [{ taskDir: '/repo/docs/ui-v2', cwd: '/repo', slug: 'ui-v2', title: 'UI v2 Rebuild', status: 'running', running: true }],
        });
      }
      if (input.action === 'status') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          taskDir: '/repo/docs/ui-v2',
          running: true,
          state: {
            slug: 'ui-v2',
            title: 'UI v2 Rebuild',
            status: 'running',
            mode: 'full',
            packets: {
              'M01-T01': { id: 'M01-T01', goal: 'Build workbench shell', status: 'running', route: 'frontend' },
            },
          },
        });
      }
      if (input.action === 'config-edit') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          editor: { scope: input.editor.scope, revision: 'r1', values: { budget: { enforce: 'off' } }, defaults: {} },
        });
      }
      if (input.action === 'config') {
        return envelope({
          protocol: 1,
          ok: true,
          cwd: '/repo',
          configuration: { source: '/global/flywheel.config.json', scope: 'global', bindings: [] },
        });
      }
      return envelope({ protocol: 1, ok: true, cwd: '/repo', revisions: [] });
    },
    act: async input => envelope({ protocol: 1, ok: true, cwd: '/repo', taskDir: input.taskDir ?? '' }),
    listRepoImages: async () => envelope({ protocol: 1, ok: true, cwd: '/repo', files: [] }),
  };

  ctx.provide('remote', { $mount: async () => () => {}, flywheelConsole });
  ctx.provide('remote.flywheelConsole', flywheelConsole);
  ctx.provide('layout', { selectPanel: id => selectedPanels.push(id) });
  ctx.provide('sidebarRightTabs', {
    register: spec => {
      sidebarTabSpecs.push(spec);
      return () => {};
    },
  });

  // Host shell declares the native slots (including conversation.view to prove Flywheel never claims it).
  ctx.slots.register(
    {
      name: 'root',
      children: {
        main: { kind: 'keyed', scope: 'root' },
        'sidebar.panellist': { kind: 'list', scope: 'root' },
        'sidebar.right.pane.tab': { kind: 'keyed', scope: 'root' },
        'conversation.chat.commandview': { kind: 'keyed', scope: 'root' },
        'settings.section': { kind: 'list', scope: 'root' },
        'conversation.view': { kind: 'list', scope: 'root' },
      },
    },
    ({ renderSlot }) =>
      React.createElement(
        'div',
        { 'data-fixture-shell': '' },
        renderSlot('main', {}, { entryKey: 'flywheel' }),
      ),
  );

  const plugin = await ctx.plugin(client);

  // Verify exact native slot registrations and ZERO conversation.view
  assert.equal(ctx.slots.entries('conversation.view').length, 0, 'legacy conversation.view must not be registered');
  assert.equal(ctx.slots.entries('main').length, 1, 'main workbench slot registered');
  assert.equal(ctx.slots.entries('sidebar.panellist').length, 1, 'sidebar.panellist rail icon registered');
  assert.equal(ctx.slots.entries('sidebar.right.pane.tab').length, 1, 'sidebar.right.pane.tab companion registered');
  assert.equal(ctx.slots.entries('conversation.chat.commandview').length, 1, 'conversation.chat.commandview registered');
  assert.equal(ctx.slots.entries('settings.section').length, 1, 'settings.section registered');
  assert.equal(sidebarTabSpecs.length, 1);

  await act(async () => {
    unmount = ctx.uiRenderer.mount(container);
    await Promise.resolve();
    await Promise.resolve();
  });

  assert.deepEqual(crashes, [], 'no renderer errors during v2 workbench mount');
  assert.ok(container.querySelector('[data-flywheel-v2="true"]'), 'StyleScope mounted inside slot');
  assert.match(container.textContent, /UI v2 Rebuild/);
  assert.match(container.textContent, /M01-T01/);
  assert.equal(clientDocument.head.innerHTML, headBefore, 'rendering slot keeps styles scoped instead of mutating document.head');

  await act(async () => {
    await plugin.dispose();
  });
  assert.equal(ctx.slots.entries('main').length, 0, 'plugin disposal unregisters all slots cleanly');
});
