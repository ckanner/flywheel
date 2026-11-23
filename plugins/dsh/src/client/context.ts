import type { ComponentType } from 'react';
import type { RemoteContribution } from './remote.ts';
import type { Translate } from './types.ts';

export interface RemoteEnvelope {
  result?: {
    kind?: 'success' | 'error' | string;
    text?: string;
  };
  ok?: boolean;
  value?: {
    result?: {
      kind?: 'success' | 'error' | string;
      text?: string;
    };
  };
  error?: { message?: string } | string;
}

export interface FlywheelConsoleRemote {
  query(input: Record<string, unknown>): Promise<RemoteEnvelope>;
  act(input: Record<string, unknown>): Promise<RemoteEnvelope>;
  listRepoImages(cwd?: string): Promise<RemoteEnvelope>;
}

export interface RemoteService {
  $mount(contribution: RemoteContribution): Promise<() => void> | (() => void);
  flywheelConsole?: FlywheelConsoleRemote;
}

export interface LocaleService {
  register(namespace: string, dicts: Record<string, Record<string, string>>): () => void;
  bind(namespace: string): Translate;
}

export interface SlotRegistration {
  name: string;
  id?: string;
  key?: string;
  order?: number;
  priority?: number;
  locale?: string;
  label?: () => string;
  inject?: (...args: unknown[]) => Record<string, unknown>;
}

export interface SlotService {
  inject(slotName: string, register: () => () => void): void;
  register(definition: SlotRegistration, component: ComponentType<never>): () => void;
}

export interface SidebarRightTabsService {
  register(spec: {
    id: string;
    kind: string;
    priority: string;
    title: () => string;
    guide?: Array<{ id: string; title: () => string; order?: number }>;
  }): () => void;
}

export interface LayoutService {
  selectPanel?: (id: string) => void;
  openPanel?: (id: string) => void;
}

export interface ClientContext {
  slots: SlotService;
  locale: LocaleService;
  remote: RemoteService;
  effect(fn: () => void | (() => void), label?: string): void;
  inject(services: string[], callback: (scoped: ClientContext) => void): void;
  get<T = unknown>(name: string): T | undefined;
}

export interface ClientRuntime {
  getService<T = unknown>(name: string): T | undefined;
}
