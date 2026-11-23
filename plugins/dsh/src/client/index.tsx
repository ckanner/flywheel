/**
 * Flywheel Next v2 — Native 4-Slot DSH Client Entry
 *
 * Registers Direct Typert Remote (`flywheelConsole`: query, act, listRepoImages)
 * and mounts 4 clean native DSH slots:
 *   1. `main` + `sidebar.panellist` (Full-page Engineering Workbench)
 *   2. `sidebar.right.pane.tab`     (Compact Live Progress Companion)
 *   3. `conversation.chat.commandview` (Read-only `/flywheel` Command Card)
 *   4. `settings.section`           (3-Tier Progressive Settings)
 *
 * Zero `conversation.view` slot, zero `:has([data-composer-seat])` hack,
 * and zero import-time DOM mutations.
 */

import type { ComponentType } from 'react';
import type { ClientContext, ClientRuntime, SidebarRightTabsService } from './context.ts';
import { DICTS, NS } from './locale.ts';
import { remoteContribution } from './remote.ts';
import { FlywheelStore } from './store.ts';
import { CommandCard } from './views/CommandCard.tsx';
import { RightSidebarLive } from './views/RightSidebarLive.tsx';
import { SettingsPage } from './views/SettingsPage.tsx';
import { FlywheelRailIcon, WorkbenchPage } from './views/WorkbenchPage.tsx';

export const PANEL_ID = 'flywheel';
export const SETTINGS_ID = 'flywheel';
export const SIDEBAR_KIND = 'flywheel';
export const COMMAND_VIEW_ID = 'flywheel';

export const inject = ['slots', 'locale', 'remote'];

export function apply(ctx: ClientContext): void {
  ctx.effect(() => ctx.locale.register(NS, DICTS), 'flywheel: dictionaries');
  const t = ctx.locale.bind(NS);

  const mounting: Promise<string | undefined> = Promise.resolve(
    ctx.remote.$mount(remoteContribution()),
  ).then(
    () => undefined,
    (error: unknown) => (error instanceof Error ? error.message : String(error)),
  );

  const runtime: ClientRuntime = {
    getService: <T = unknown,>(name: string) => ctx.get<T>(name),
  };

  const store = new FlywheelStore(ctx, mounting);
  ctx.effect(() => {
    store.start();
    return () => {
      store.stop();
    };
  }, 'flywheel: store');

  // Slot 1: Full-page Workbench + Left Navigation Rail Icon
  ctx.slots.inject('main', () =>
    ctx.slots.register(
      {
        name: 'main',
        key: PANEL_ID,
        locale: NS,
        inject: () => ({ store, t, runtime }),
      },
      WorkbenchPage as unknown as ComponentType<never>,
    ),
  );

  ctx.slots.inject('sidebar.panellist', () =>
    ctx.slots.register(
      {
        name: 'sidebar.panellist',
        id: PANEL_ID,
        order: 25,
        locale: NS,
        label: () => t('panel'),
      },
      FlywheelRailIcon as unknown as ComponentType<never>,
    ),
  );

  // Slot 2: Compact Right-Sidebar Companion Tab (optional host service)
  if (typeof ctx.inject === 'function') {
    ctx.inject(['sidebarRightTabs'], scoped => {
      const tabs = (scoped as unknown as { sidebarRightTabs?: SidebarRightTabsService }).sidebarRightTabs;
      if (!tabs) return;
      scoped.effect(
        () =>
          tabs.register({
            id: SIDEBAR_KIND,
            kind: SIDEBAR_KIND,
            priority: 'extension',
            title: () => t('sidebar.tab'),
            guide: [{ id: SIDEBAR_KIND, title: () => t('sidebar.tab'), order: 40 }],
          }),
        'flywheel: right-sidebar tab',
      );
      scoped.slots.inject('sidebar.right.pane.tab', () =>
        scoped.slots.register(
          {
            name: 'sidebar.right.pane.tab',
            key: SIDEBAR_KIND,
            locale: NS,
            inject: () => ({ store, t, runtime }),
          },
          RightSidebarLive as unknown as ComponentType<never>,
        ),
      );
    });
  }

  // Slot 3: Compact Read-Only Command Card for `/flywheel`
  ctx.slots.inject('conversation.chat.commandview', () =>
    ctx.slots.register(
      {
        name: 'conversation.chat.commandview',
        id: COMMAND_VIEW_ID,
        key: COMMAND_VIEW_ID,
        locale: NS,
        inject: () => ({ store, t, runtime }),
      },
      CommandCard as unknown as ComponentType<never>,
    ),
  );

  // Slot 4: 3-Tier Progressive Settings Page
  ctx.slots.inject('settings.section', () =>
    ctx.slots.register(
      {
        name: 'settings.section',
        id: SETTINGS_ID,
        order: 35,
        locale: NS,
        label: () => t('settings.title'),
        inject: () => ({ store, t }),
      },
      SettingsPage as unknown as ComponentType<never>,
    ),
  );
}
