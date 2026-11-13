import test, { after } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { act, clientDocument, closeClientHarness, deferred, loadClient } from './support/client-harness.js';

after(closeClientHarness);

// These are test-only host dependencies. The production plugin loader below
// exposes React alone, as required for an external plain-JS Harness plugin.
const React = await import('react');
const ReactDOM = await import('react-dom');
const ReactDOMClient = await import('react-dom/client');
const JSX = await import('react/jsx-runtime');
const hostRequire = createRequire(process.env.FLYWHEEL_TEST_DSH_HOST_DIR
  ? resolve(process.env.FLYWHEEL_TEST_DSH_HOST_DIR, 'package.json') : new URL('../package.json', import.meta.url));
const Cordis = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/cordis')));
const Slots = await import(pathToFileURL(hostRequire.resolve('@deepseek-ai/dsh-client-ui-slots')));
const source = await readFile(hostRequire.resolve('@deepseek-ai/dsh-client-ui-renderer/client'), 'utf8');

function publishedRenderer() {
  const shared = { react: React, 'react-dom': ReactDOM, 'react-dom/client': ReactDOMClient,
    'react/jsx-runtime': JSX, '@deepseek-ai/cordis': Cordis, '@deepseek-ai/dsh-client-ui-slots': Slots };
  let renderer;
  vm.runInNewContext(source, { window: { __ModuleLoader__: { load(definition) {
    assert.equal(definition.id, '@deepseek-ai/dsh-client-ui-renderer');
    renderer = definition.factory(name => {
      assert.ok(name in shared, `published renderer dependency ${name} is explicitly supplied by the fixture`);
      return shared[name];
    });
  } } }, console, Error, queueMicrotask, process });
  return renderer;
}

// Session selection and shell navigation belong to the host fixture. Their
// observables feed the published renderer, which alone creates all React hooks.
function hostSource(initial) {
  let value = initial;
  const listeners = new Set();
  return { getSnapshot: () => value, subscribe: listener => {
    listeners.add(listener);
    return () => listeners.delete(listener);
  }, set: next => { value = next; for (const listener of [...listeners]) listener(); } };
}

test('the published DSH renderer owns the external plugin lifecycle without Harness Client module imports', async t => {
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
  const locale = { bind: namespace => (key, params = {}) =>
    String(dictionaries.get(namespace)?.zh[key] ?? key).replace(/\{(\w+)\}/g, (match, name) => name in params ? params[name] : match),
  register: (namespace, values) => { dictionaries.set(namespace, values); return () => dictionaries.delete(namespace); },
  getSnapshot: () => localeRevision, subscribe: () => () => {} };
  ctx.provide('locale', locale);
  ctx.slots.installLocale(locale);

  const pending = deferred(), replacementPending = deferred();
  let requestsFromA = 0;
  const requests = [], discoveries = [];
  const sessions = [];
  const reply = (request, slug) => ({ ok: true, value: { result: { kind: 'success', text: JSON.stringify({
    protocol: 1, ok: true, taskDir: request.taskDir, running: false, state: { slug, status: 'pending', packets: {} },
  }) } } });
  const remote = { commands: { execute: async (sessionId, command, attachments) => {
    assert.equal(attachments.length, 0);
    assert.ok(command.startsWith('/flywheel '));
    const request = JSON.parse(command.slice('/flywheel '.length));
    if (request.action === 'runs') {
      discoveries.push({ sessionId, request });
      return { ok: true, value: { result: { kind: 'success', text: JSON.stringify({ protocol: 1, ok: true, taskDir: '', cwd: '/repo', running: false, state: null, runs: [] }) } } };
    }
    requests.push(request);
    sessions.push(sessionId);
    return sessionId === 'session-a' ? (++requestsFromA === 1 ? pending.promise : replacementPending.promise) : reply(request, 'SESSION_B_RESULT');
  } } };
  remote.$mount = async () => () => {};
  remote.flywheelConsole = { read: (sessionId, request) => remote.commands.execute(sessionId, '/flywheel ' + JSON.stringify(request), []) };
  ctx.provide('remote', remote);
  ctx.provide('remote.commands', remote.commands);

  const sessionPlugin = { name: 'fixture-session', apply() {} };
  const sessionA = await ctx.plugin(sessionPlugin);
  const sessionB = await ctx.plugin(sessionPlugin);
  const projectionA = hostSource(undefined);
  const projectionB = hostSource(undefined);
  const binding = (key, fiber) => ({ key, ctx: fiber.ctx, hooks: {},
    keyedHooks: { projection: name => {
      assert.equal(name, 'flywheelTask');
      return key === 'session-a' ? projectionA : projectionB;
    } }, props: {} });
  const bindingA = binding('session-a', sessionA);
  const bindingB = binding('session-b', sessionB);
  const current = hostSource(bindingA);
  const shell = hostSource({ visible: true });
  ctx.slots.installScope('session', { current, bindingSource: () => current,
    renderArea: (value, props) => value.key === undefined ? props.empty ?? null : props.children({ sessionId: value.key }) });
  ctx.slots.provideRoot({ hooks: { fixture: shell } });
  ctx.slots.register({ name: 'root', children: { 'fixture.conversation': { kind: 'single', scope: 'root' } } },
    ({ renderSlot }) => React.createElement('div', { 'data-fixture-shell': '' }, renderSlot('fixture.conversation', {})));

  // The plugin must wait for the native declaration, then contribute beside
  // existing views. Only the fixture shell owns root and its children.
  const plugin = await ctx.plugin(client);
  assert.equal(ctx.slots.spec('conversation.view'), undefined);
  const ownerPlugin = { name: 'fixture-conversation-owner', inject: ['slots'], apply(owner) {
    owner.slots.register({ name: 'fixture.conversation', children: { 'conversation.view': { kind: 'list', scope: 'session' } } },
      ({ renderSlot, useFixture }) => React.createElement('main', null,
        React.createElement('nav', { 'aria-label': 'Host views' }, '对话 / 轨迹 / Flywheel'),
        renderSlot('conversation.view', {}, { only: useFixture(state => state.visible) ? 'flywheel' : 'chat' })));
    owner.slots.register({ name: 'conversation.view', id: 'chat', label: '对话' },
      () => React.createElement('p', null, 'Native conversation'));
    owner.slots.register({ name: 'conversation.view', id: 'trajectory', label: '轨迹' },
      () => React.createElement('p', null, 'Native trajectory'));
  } };
  let owner = await ctx.plugin(ownerPlugin);
  assert.deepEqual(new Set(ctx.slots.entries('conversation.view').map(entry => entry.options.id)),
    new Set(['flywheel', 'chat', 'trajectory']));
  await act(async () => { unmount = ctx.uiRenderer.mount(container); });
  assert.ok(container.querySelector('section[aria-label="Flywheel 流程工作台"]'));
  assert.equal(requests.length, 0);
  assert.equal(discoveries.length, 1);
  const styleSelector = 'style[data-plugin="@ckanner/dsh-plugin-flywheel"]';
  assert.ok(container.querySelector('[data-slot="conversation.view"]').contains(container.querySelector(styleSelector)));
  assert.equal(clientDocument.head.innerHTML, headBefore);

  const location = () => container.querySelector('[data-task-path]')?.getAttribute('data-task-path') ?? '';
  const associate = async (source, taskDir, sequence = 1) => act(async () => source.set({
    operationId: 'fixture:associate-' + sequence, sequence, action: 'status', taskDir, cwd: '/repo', outcome: 'confirmed', message: '',
  }));
  const button = label => {
    const value = [...container.querySelectorAll('button')].find(node => node.textContent === label);
    assert.ok(value, `${label} is rendered by the plugin-owned control`);
    return value;
  };
  await associate(projectionA, '/repo/task-a');
  await act(async () => button('刷新').click());
  assert.equal(button('刷新').disabled, true);
  await act(async () => shell.set({ visible: false }));
  assert.match(container.textContent, /Native conversation/);
  assert.equal(container.querySelector(styleSelector), null);
  await act(async () => shell.set({ visible: true }));
  assert.equal(location(), '/repo/task-a');
  assert.equal(button('刷新').disabled, true, 'tab remount reuses the registry-owned pending controller');
  assert.equal(container.querySelectorAll(styleSelector).length, 1);

  await act(async () => current.set(bindingB));
  assert.equal(location(), '');
  await associate(projectionB, '/repo/task-b');
  assert.match(container.textContent, /SESSION_B_RESULT/);
  await act(async () => current.set(bindingA));
  assert.equal(location(), '/repo/task-a');
  assert.equal(button('刷新').disabled, true);

  const replacementA = await ctx.plugin(sessionPlugin);
  await act(async () => current.set(binding('session-a', replacementA)));
  assert.equal(location(), '/repo/task-a', 'a new binding recovers the durable association and starts its own fresh read');
  assert.equal(requests.length, 3);
  await act(async () => pending.resolve(reply(requests[0], 'RETIRED_GENERATION_RESULT')));
  assert.doesNotMatch(container.textContent, /RETIRED_GENERATION_RESULT/);
  await act(async () => replacementPending.resolve(reply(requests[2], 'CURRENT_GENERATION_RESULT')));
  assert.match(container.textContent, /CURRENT_GENERATION_RESULT/);
  await act(async () => { await sessionA.dispose(); });
  await act(async () => shell.set({ visible: false }));
  await act(async () => shell.set({ visible: true }));
  assert.equal(location(), '/repo/task-a', 'late disposal of the old Context cannot release the new generation');
  await act(async () => current.set(bindingB));
  assert.equal(location(), '/repo/task-b');
  assert.match(container.textContent, /SESSION_B_RESULT/);

  await act(async () => { await owner.dispose(); });
  assert.equal(ctx.slots.spec('conversation.view'), undefined);
  assert.equal(container.querySelector(styleSelector), null);
  await act(async () => { owner = await ctx.plugin(ownerPlugin); });
  assert.equal(ctx.slots.entries('conversation.view').filter(entry => entry.options.id === 'flywheel').length, 1);
  assert.equal(location(), '/repo/task-b', 'reassembly recovers its logged association through a fresh read');
  await act(async () => projectionB.set({ operationId: 'tool:published-launch', sequence: 10,
    action: 'launch', taskDir: '/repo/auto-bound', cwd: '/repo', outcome: 'confirmed', message: '' }));
  assert.equal(location(), '/repo/auto-bound', 'the published useProjection hook delivers the exact host task association');
  assert.equal(container.querySelector('[data-workspace-path]')?.getAttribute('data-workspace-path'), '/repo');
  assert.match(container.textContent, /SESSION_B_RESULT/);
  await act(async () => shell.set({ visible: false }));
  await act(async () => shell.set({ visible: true }));
  assert.equal(requests.length, 5, 'restoring the same tab does not repeat a launch or status read');
  await act(async () => { await plugin.dispose(); });
  assert.deepEqual(new Set(ctx.slots.entries('conversation.view').map(entry => entry.options.id)), new Set(['chat', 'trajectory']));
  assert.equal(container.querySelector(styleSelector), null);
  await act(async () => shell.set({ visible: false }));
  assert.match(container.textContent, /Native conversation/);
  assert.deepEqual(requests.map(request => request.action), ['status', 'status', 'status', 'status', 'status']);
  assert.deepEqual(sessions, ['session-a', 'session-b', 'session-a', 'session-b', 'session-b']);
  assert.deepEqual(crashes, [], 'the real renderer reported no crashed or abdicated plugin entry');
  assert.equal(clientDocument.head.innerHTML, headBefore);
  assert.equal(clientDocument.body.childElementCount, bodyChildrenBefore);
});
