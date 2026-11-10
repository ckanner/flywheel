import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import { JSDOM } from 'jsdom';

// This adapter exercises the built plugin with its own controls and StoreHandle.
// It supplies the renderer-owned hook bridge and session binding generations;
// jsdom does not verify native host rendering or geometry.
// An assembled-browser/Mac check is a separate verification gate.
const dom = new JSDOM('<!doctype html><html><head></head><body></body></html>', { url: 'http://localhost/' });
const globals = ['window', 'document', 'navigator', 'IS_REACT_ACT_ENVIRONMENT'];
const previousGlobals = globals.map(name => Object.getOwnPropertyDescriptor(globalThis, name));
for (const name of ['window', 'document', 'navigator']) {
  Object.defineProperty(globalThis, name, {
    configurable: true, writable: true, value: name === 'window' ? dom.window : dom.window[name],
  });
}
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
const React = await import('react');
const { createRoot } = await import('react-dom/client');
export const { act } = React;
export const clientDocument = dom.window.document;
const h = React.createElement;
const source = await readFile(new URL('../../dist/client.js', import.meta.url), 'utf8');
const bundleImports = [...source.matchAll(/\brequire\(\s*(['"])([^'"]+)\1\s*\)/g)].map(match => match[2]);
assert.deepEqual([...new Set(bundleImports)], ['react'], 'the shipped plain-JS bundle depends on React alone');

export function closeClientHarness() {
  dom.window.close();
  globals.forEach((name, index) => {
    if (previousGlobals[index]) Object.defineProperty(globalThis, name, previousGlobals[index]);
    else delete globalThis[name];
  });
}

function snapshotHook(source) {
  return function useSnapshot(selector = value => value) {
    const snapshot = React.useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
    return selector(snapshot);
  };
}

export function loadClient(timers) {
  let client;
  const modules = { react: React };
  vm.runInNewContext(source, {
    document: dom.window.document, console, Error, AbortController,
    setTimeout: timers?.setTimeout ?? ((...args) => setTimeout(...args)),
    clearTimeout: timers?.clearTimeout ?? ((...args) => clearTimeout(...args)),
    queueMicrotask,
    window: { __ModuleLoader__: { load(definition) {
      assert.equal(definition.id, '@ckanner/dsh-plugin-flywheel');
      client = definition.factory(name => {
        assert.ok(name in modules, `the external plain-JS client may load only React, not ${name}`);
        return modules[name];
      });
    } } },
  });
  assert.equal(typeof client?.apply, 'function');
  return client;
}

export function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

export function deadlineClock() {
  let nextId = 0;
  const pending = new Map();
  return {
    setTimeout: (callback, delay) => {
      const id = ++nextId;
      pending.set(id, { callback, delay });
      return id;
    },
    clearTimeout: id => pending.delete(id),
    expire: async () => {
      const deadlines = [...pending.values()];
      pending.clear();
      await act(async () => { for (const { callback } of deadlines) callback(); });
    },
    pending,
  };
}

export async function mountPanel(t, respond, { timers, locale = 'zh', association, catalog } = {}) {
  const client = loadClient(timers);
  const definitions = new Map();
  const effects = [];
  const dictionaries = new Map();
  // The new passive catalog has its own read lane. Existing task protocol
  // fixtures stay scoped to their commands; discovery scenarios exercise both.
  const requests = [], discoveryRequests = [], allRequests = [];
  const sessions = [], discoverySessions = [];
  let activeLocale = locale;
  let disposed = false;
  const translate = namespace => (key, params) => {
    const values = dictionaries.get(namespace);
    const value = values?.[activeLocale]?.[key] ?? values?.en?.[key] ?? key;
    return String(value).replace(/\{(\w+)\}/g, (match, name) => name in (params ?? {}) ? String(params[name]) : match);
  };
  const trackEffect = factory => {
    const cleanup = factory();
    let live = true;
    const dispose = () => {
      if (!live) return;
      live = false;
      if (typeof cleanup === 'function') cleanup();
    };
    effects.push(dispose);
    return dispose;
  };
  client.apply({
    effect: trackEffect,
    on: (event, callback) => {
      assert.equal(event, 'dispose');
      return trackEffect(() => callback);
    },
    locale: {
      register: (namespace, values) => {
        assert.ok(!dictionaries.has(namespace), 'each locale namespace has one owner');
        dictionaries.set(namespace, values);
        return () => dictionaries.delete(namespace);
      },
      bind: translate,
    },
    slots: {
      inject: (name, activate) => {
        assert.equal(name, 'conversation.view', 'Flywheel adds a native view without replacing host chrome');
        return trackEffect(activate);
      },
      register: (definition, Component) => {
        assert.equal(definition.name, 'conversation.view');
        assert.equal(definition.id, 'flywheel');
        assert.ok(!definitions.has(definition.name));
        definitions.set(definition.name, { definition, Component });
        return () => definitions.delete(definition.name);
      },
    },
    remote: { commands: { execute: async (sessionId, command, attachments) => {
      assert.equal(attachments.length, 0);
      assert.ok(command.startsWith('/flywheel '));
      const request = JSON.parse(command.slice('/flywheel '.length));
      allRequests.push(request);
      if (request.action === 'runs') {
        discoveryRequests.push(request); discoverySessions.push(sessionId);
        if (catalog) return catalog(request, sessionId);
        return { ok: true, value: { result: { kind: 'success', text: JSON.stringify({ protocol: 1, ok: true, running: false, taskDir: '', cwd: '/repo', state: null, runs: [] }) } } };
      }
      requests.push(request);
      sessions.push(sessionId);
      return respond(request, sessionId, command);
    } } },
  });
  assert.equal(definitions.size, 1, 'one additive native view is registered');
  const { definition, Component } = definitions.get('conversation.view');
  assert.equal(typeof definition.store?.create, 'function', 'view state implements the public StoreHandle contract');
  assert.equal(typeof definition.store?.spec?.init, 'function');
  assert.equal(typeof definition.store?.spec?.actions, 'object');
  const bindings = new Map();
  const projections = new Map();
  const projectionSource = sessionId => {
    if (!projections.has(sessionId)) {
      let value = sessionId === 'desktop-session' ? association : undefined;
      const listeners = new Set();
      projections.set(sessionId, { getSnapshot: () => value,
        subscribe: listener => { listeners.add(listener); return () => listeners.delete(listener); },
        set: next => { value = next; for (const listener of [...listeners]) listener(); } });
    }
    return projections.get(sessionId);
  };
  let currentSession = 'desktop-session';
  let generation = 0;
  let visible = true;
  const getBinding = (sessionId, version) => {
    const key = `${sessionId}:${version}`;
    if (!bindings.has(key)) {
      const instance = definition.store.create(sessionId);
      const injected = definition.inject?.(sessionId, instance.actions) ?? {};
      const { hooks = {}, ...face } = injected;
      const sessionSnapshot = Object.freeze({ cwd: '/repo', running: false });
      const sessionSource = { getSnapshot: () => sessionSnapshot, subscribe: () => () => {} };
      const props = {
        ...face, sessionId, actions: instance.actions,
        useStore: snapshotHook(instance),
        useSession: snapshotHook(sessionSource),
        useProjection: (key, selector = value => value) => {
          assert.equal(key, 'flywheelTask');
          return snapshotHook(projectionSource(sessionId))(selector);
        },
        t: translate(definition.locale),
      };
      for (const [name, value] of Object.entries(hooks)) {
        assert.equal(typeof value.getSnapshot, 'function', `${name} is a bare observable`);
        props[`use${name[0].toUpperCase()}${name.slice(1)}`] = snapshotHook(value);
      }
      bindings.set(key, { props, instance });
    }
    return bindings.get(key);
  };
  const container = dom.window.document.createElement('div');
  dom.window.document.body.appendChild(container);
  const root = createRoot(container);
  const render = () => root.render(visible && !disposed ? h(Component, {
    ...getBinding(currentSession, generation).props, key: `${currentSession}:${generation}`,
  }) : null);
  await act(async () => render());
  const unload = async () => {
    await act(async () => {
      for (const cleanup of [...effects].reverse()) cleanup();
      disposed = true;
      render();
    });
  };
  t.after(async () => {
    if (!disposed) await unload();
    await act(async () => root.unmount());
    container.remove();
  });
  const button = name => {
    const found = [...container.querySelectorAll('button')].find(item =>
      typeof name === 'string' ? item.textContent === name : name.test(item.textContent));
    assert.ok(found, `button ${name} exists; visible: ${[...container.querySelectorAll('button')].map(x => x.textContent).join(', ')}`);
    return found;
  };
  const reveal = element => {
    const ancestors = [];
    for (let node = element.parentElement; node; node = node.parentElement) {
      if (node.tagName === 'DETAILS' && !node.open) ancestors.push(node);
    }
    for (const details of ancestors.reverse()) details.querySelector(':scope > summary').click();
  };
  const click = async name => { await act(async () => {
    const target = button(name);
    reveal(target);
    target.click();
  }); };
  // Seed the public selection binding for controller/protocol scenarios.
  // DOM task picking is exercised separately in task-discovery.test.js.
  const selection = () => getBinding(currentSession, generation).instance.getSnapshot();
  const selectPath = async (index, value) => act(async () => {
    const binding = getBinding(currentSession, generation);
    binding.props.changeSelection({ taskDir: selection().taskDir, cwd: selection().cwd, [index === 0 ? 'taskDir' : 'cwd']: value });
  });
  const switchSession = async (sessionId, version = 0) => {
    currentSession = sessionId;
    generation = version;
    await act(async () => render());
  };
  const setVisible = async value => {
    visible = value;
    await act(async () => render());
  };
  const setLocale = async value => {
    activeLocale = value;
    await act(async () => render());
  };
  const setAssociation = async (value, sessionId = currentSession) => {
    await act(async () => projectionSource(sessionId).set(value));
  };
  return { container, root, definition, requests, discoveryRequests, allRequests, sessions, discoverySessions, button, click, selectPath, selection,
    switchSession, setVisible, setLocale, setAssociation, unload, bindings, document: dom.window.document,
    translate: translate(definition.locale) };
}
