import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import { JSDOM } from 'jsdom';

const dom = new JSDOM('<!doctype html><html><head></head><body></body></html>', { url: 'http://localhost/' });
const globals = ['window', 'document', 'navigator', 'IS_REACT_ACT_ENVIRONMENT', 'HTMLElement', 'Element', 'Node'];
const previousGlobals = globals.map(name => Object.getOwnPropertyDescriptor(globalThis, name));
for (const name of ['window', 'document', 'navigator', 'HTMLElement', 'Element', 'Node']) {
  Object.defineProperty(globalThis, name, {
    configurable: true,
    writable: true,
    value: name === 'window' ? dom.window : dom.window[name],
  });
}
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const React = await import('react');
const { createRoot } = await import('react-dom/client');
export const { act } = React;
export const clientDocument = dom.window.document;
export const h = React.createElement;

export const clientSource = await readFile(
  process.env.FLYWHEEL_TEST_DSH_PACKAGE_DIR
    ? process.env.FLYWHEEL_TEST_DSH_PACKAGE_DIR + '/dist/client.js'
    : new URL('../../dist/client.js', import.meta.url),
  'utf8',
);

const bundleImports = [...clientSource.matchAll(/\brequire\(\s*(['"])([^'"]+)\1\s*\)/g)].map(match => match[2]);
assert.deepEqual([...new Set(bundleImports)], ['react'], 'the shipped v2 client bundle depends on React alone');
assert.equal(clientSource.includes(':has([data-composer-seat])'), false, 'v2 bundle must not contain legacy :has() layout hack');

export function closeClientHarness() {
  dom.window.close();
  globals.forEach((name, index) => {
    if (previousGlobals[index]) Object.defineProperty(globalThis, name, previousGlobals[index]);
    else delete globalThis[name];
  });
}

export function loadClient(timers) {
  let client;
  const modules = { react: React };
  vm.runInNewContext(clientSource, {
    document: dom.window.document,
    window: {
      __ModuleLoader__: {
        load(definition) {
          assert.equal(definition.id, '@ckanner/flywheel');
          client = definition.factory(name => {
            assert.ok(name in modules, `external client may load only React, not ${name}`);
            return modules[name];
          });
        },
      },
      document: dom.window.document,
      Node: dom.window.Node,
      NodeFilter: dom.window.NodeFilter,
      DocumentFragment: dom.window.DocumentFragment,
      Element: dom.window.Element,
      HTMLElement: dom.window.HTMLElement,
      DOMParser: dom.window.DOMParser,
    },
    console,
    Error,
    AbortController,
    setTimeout: timers?.setTimeout ?? ((...args) => setTimeout(...args)),
    clearTimeout: timers?.clearTimeout ?? ((...args) => clearTimeout(...args)),
    queueMicrotask,
  });
  assert.equal(typeof client?.apply, 'function');
  return client;
}

export function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

export function envelope(payload, kind = 'success') {
  return { ok: true, value: { result: { kind, text: JSON.stringify(payload) } } };
}

/**
 * Mounts the v2 3-slot DSH client in jsdom with Direct Typert Remote (`query`, `act`, `listRepoImages`)
 * and returns helpers to render any of the registered slots (`main`, `sidebar.panellist`,
 * `sidebar.right.pane.tab`, `conversation.chat.commandview`). Settings lives inline inside `main`.
 */
export async function mountWorkbench(
  t,
  handlers = {},
  { locale = 'zh', slot = 'main', timers, extraProps = {}, mountError, services: extraServices = {} } = {},
) {
  const client = loadClient(timers);
  const slots = new Map();
  const sidebarTabs = [];
  const effects = [];
  const dictionaries = new Map();
  const queryCalls = [];
  const actCalls = [];
  const imageCalls = [];
  const selectedPanels = [];
  let mountedContribution = null;
  let activeLocale = locale;
  let disposed = false;

  const translate = namespace => (key, params) => {
    const values = dictionaries.get(namespace);
    const value = values?.[activeLocale]?.[key] ?? values?.en?.[key] ?? key;
    return String(value).replace(/\{(\w+)\}/g, (match, name) =>
      name in (params ?? {}) ? String(params[name]) : match,
    );
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

  const remoteNamespace = {
    query: async input => {
      const cloned = JSON.parse(JSON.stringify(input));
      queryCalls.push(cloned);
      if (handlers.query) return handlers.query(cloned);
      if (cloned.action === 'runs') {
        return envelope({ protocol: 1, ok: true, running: false, taskDir: '', cwd: '/repo', state: null, runs: [] });
      }
      if (cloned.action === 'design-candidates') {
        return envelope({ protocol: 1, ok: true, taskDir: cloned.taskDir ?? '', revisions: [] });
      }
      if (cloned.action === 'ux-review') {
        return envelope({ protocol: 1, ok: true, taskDir: cloned.taskDir ?? '', packet: cloned.packet, uxReview: null });
      }
      return envelope({ protocol: 1, ok: true, running: false, taskDir: cloned.taskDir ?? '', cwd: '/repo', state: null });
    },
    act: async input => {
      const cloned = JSON.parse(JSON.stringify(input));
      actCalls.push(cloned);
      if (handlers.act) return handlers.act(cloned);
      return envelope({ protocol: 1, ok: true, running: false, taskDir: cloned.taskDir ?? '', cwd: cloned.cwd ?? '/repo', state: null });
    },
    listRepoImages: async cwd => {
      imageCalls.push(cwd);
      if (handlers.listRepoImages) return handlers.listRepoImages(cwd);
      return envelope({ protocol: 1, ok: true, cwd: cwd ?? '/repo', files: [] });
    },
  };

  const services = new Map([
    ['remote.flywheelConsole', remoteNamespace],
    ['layout', { selectPanel: id => selectedPanels.push(id) }],
    ...Object.entries(extraServices),
  ]);

  const ctx = {
    effect: trackEffect,
    get: name => services.get(name),
    locale: {
      register: (namespace, values) => {
        dictionaries.set(namespace, values);
        return () => dictionaries.delete(namespace);
      },
      bind: translate,
    },
    remote: {
      $mount: async contribution => {
        if (mountError) throw mountError;
        mountedContribution = contribution;
        return () => {};
      },
      flywheelConsole: remoteNamespace,
    },
    slots: {
      inject: (_name, register) => trackEffect(register),
      register: (definition, Component) => {
        assert.notEqual(definition.name, 'conversation.view', 'legacy conversation.view slot must be deleted');
        slots.set(definition.name, { definition, Component });
        return () => slots.delete(definition.name);
      },
    },
    inject: (deps, callback) => {
      if (deps.includes('sidebarRightTabs')) {
        callback({
          ...ctx,
          sidebarRightTabs: {
            register: spec => {
              sidebarTabs.push(spec);
              return () => {};
            },
          },
        });
      }
    },
  };

  client.apply(ctx);
  assert.equal(slots.has('conversation.view'), false, 'conversation.view must not be registered');
  assert.ok(slots.has('main'), 'main workbench slot must be registered');
  assert.ok(slots.has('sidebar.panellist'), 'sidebar.panellist rail slot must be registered');
  assert.ok(slots.has('sidebar.right.pane.tab'), 'sidebar.right.pane.tab slot must be registered');
  assert.ok(slots.has('conversation.chat.commandview'), 'conversation.chat.commandview slot must be registered');
  assert.equal(slots.has('settings.section'), false, 'duplicate settings.section slot must not be registered');

  const container = dom.window.document.createElement('div');
  dom.window.document.body.appendChild(container);
  const root = createRoot(container);

  let activeSlot = slot;
  let currentExtraProps = extraProps;

  const renderSlot = (slotName = activeSlot, propsOverride = currentExtraProps) => {
    activeSlot = slotName;
    currentExtraProps = propsOverride;
    const entry = slots.get(slotName);
    assert.ok(entry, `slot ${slotName} must be registered`);
    const injected = entry.definition.inject ? entry.definition.inject() : {};
    root.render(!disposed ? h(entry.Component, { ...injected, ...propsOverride }) : null);
    return injected;
  };

  let injectedProps;
  await act(async () => {
    injectedProps = renderSlot(activeSlot, currentExtraProps);
    await Promise.resolve();
    await Promise.resolve();
  });

  const unload = async () => {
    await act(async () => {
      for (const cleanup of [...effects].reverse()) cleanup();
      disposed = true;
      root.render(null);
    });
  };

  t.after(async () => {
    if (!disposed) await unload();
    await act(async () => root.unmount());
    container.remove();
  });

  const button = name => {
    const found = [...container.querySelectorAll('button')].find(item =>
      typeof name === 'string' ? item.textContent.trim() === name : name.test(item.textContent),
    );
    assert.ok(
      found,
      `button ${name} exists; visible: ${[...container.querySelectorAll('button')].map(x => x.textContent.trim()).join(' | ')}`,
    );
    return found;
  };

  const click = async name => {
    await act(async () => {
      button(name).click();
      await Promise.resolve();
      await Promise.resolve();
    });
  };

  const setLocale = async next => {
    activeLocale = next;
    await act(async () => {
      renderSlot(activeSlot, currentExtraProps);
    });
  };

  const switchSlot = async (nextSlot, nextExtra = {}) => {
    await act(async () => {
      renderSlot(nextSlot, nextExtra);
      await Promise.resolve();
    });
  };

  return {
    container,
    slots,
    sidebarTabs,
    mountedContribution,
    store: injectedProps?.store,
    queryCalls,
    actCalls,
    imageCalls,
    selectedPanels,
    button,
    click,
    setLocale,
    switchSlot,
    unload,
    document: dom.window.document,
  };
}
