import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { StyleScope } from '../components/StyleScope.tsx';
import type { FlywheelStore } from '../store.ts';
import type { Translate } from '../types.ts';

type RoleGroup = 'loops' | 'routes' | 'planning';

interface ProfileRecord {
  harness?: string;
  channel?: string;
  model?: string;
  thinking?: string;
  'native-profile'?: string;
  provider?: string;
  [key: string]: unknown;
}

interface ResolvedStageValues {
  profileName: string;
  harness: string;
  model: string;
  thinking: string;
}

interface StageSpec {
  id: string;
  group: RoleGroup;
  kind: 'loop' | 'route' | 'profile-ref';
  key: string;
  path?: string[];
  labelZh: string;
  labelEn: string;
  readOnly?: boolean;
}

const HARNESS_OPTIONS = [
  { id: 'antigravity', label: 'Antigravity (Google)', channel: 'google-subscription' },
  { id: 'claude-code-pro', label: 'Claude Code Pro', channel: 'claude-native' },
  { id: 'codex-pro', label: 'Codex Pro (OpenAI)', channel: 'chatgpt-subscription' },
  { id: 'kimi', label: 'Kimi Native', channel: 'kimi-subscription' },
] as const;

const HARNESS_CHANNEL: Record<string, string> = {
  antigravity: 'google-subscription',
  'claude-code-pro': 'claude-native',
  'codex-pro': 'chatgpt-subscription',
  kimi: 'kimi-subscription',
};

const CANONICAL_MODELS: Record<string, string[]> = {
  antigravity: [
    'gemini-4-carbon-high',
    'gemini-4-carbon-medium',
    'gemini-4-neon-high',
    'gemini-4-hydrogen-high',
    'gemini-4-hydrogen-medium',
    'gemini-4-hydrogen-low',
    'gemini-pro-orchestrator-neon',
    'gemini-pro-orchestrator',
    'gemini-pro-orchestrator-argon',
  ],
  'claude-code-pro': [
    'claude-opus-5-5',
    'claude-fable-5-2',
    'claude-sonnet-5-1',
  ],
  'codex-pro': [
    'gpt-6.2-astra',
    'gpt-6.2-sol',
  ],
  kimi: [
    'kimi-code/k3.1-pro',
  ],
};

const THINKING_OPTIONS = ['', 'low', 'medium', 'high', 'max'] as const;

const JEV_ALLOWED_DATA_CATEGORIES = [
  'task-summary',
  'diagnostic-facts',
  'logs',
  'source',
  'diff',
] as const;

const JEV_MODEL_RE = /^jev-[0-9]+\.[0-9]+\.[0-9]+$/;
const JEV_KEY_ENV_RE = /^[A-Za-z_][A-Za-z0-9_]{0,119}$/;

const SUBSCRIPTIONS = [
  { id: 'antigravity', name: 'Antigravity (ACP)', channel: 'google-subscription', desc: 'Gemini 4 Carbon / Neon / Hydrogen' },
  { id: 'claude-code-pro', name: 'Claude Code Pro', channel: 'claude-native', desc: 'Claude Opus 5.5 / Fable' },
  { id: 'codex-pro', name: 'Codex Pro', channel: 'chatgpt-subscription', desc: 'GPT-6.2 Astra / Sol' },
  { id: 'kimi', name: 'Kimi Native', channel: 'kimi-subscription', desc: 'K3.1 Pro (WebSearch / Swarm)' },
] as const;

const READ_ONLY_ROLES = new Set([
  'loops.design-review',
  'loops.plan-review',
  'loops.review',
  'loops.judge',
  'loops.investigate',
  'frontend-ux-review',
  'planning.compact.design-review',
]);

const STAGE_SPECS: StageSpec[] = [
  // Workflow Stages (Loops)
  { id: 'loops.design-plan', group: 'loops', kind: 'loop', key: 'design-plan', labelZh: '架构设计主稿', labelEn: 'Design Primary Draft' },
  { id: 'loops.design-review', group: 'loops', kind: 'loop', key: 'design-review', labelZh: '设计架构审查', labelEn: 'Design Architecture Review', readOnly: true },
  { id: 'loops.plan', group: 'loops', kind: 'loop', key: 'plan', labelZh: '拆包与波次规划', labelEn: 'Wave & Packet Planning' },
  { id: 'loops.plan-review', group: 'loops', kind: 'loop', key: 'plan-review', labelZh: '规划契约审查', labelEn: 'Plan Review', readOnly: true },
  { id: 'loops.review', group: 'loops', kind: 'loop', key: 'review', labelZh: '代码质量审查', labelEn: 'Code Review', readOnly: true },
  { id: 'loops.judge', group: 'loops', kind: 'loop', key: 'judge', labelZh: '里程碑终审裁决', labelEn: 'Milestone Judge', readOnly: true },
  { id: 'loops.investigate', group: 'loops', kind: 'loop', key: 'investigate', labelZh: '只读根因调查', labelEn: 'Read-Only Investigation', readOnly: true },
  { id: 'frontend-ux-review', group: 'loops', kind: 'profile-ref', key: 'frontend-ux-review', path: ['frontend-ux-review', 'profile'], labelZh: '前端 UX 与文案审查', labelEn: 'Frontend UX Audit', readOnly: true },
  { id: 'loops.intake', group: 'loops', kind: 'loop', key: 'intake', labelZh: '需求收件整理', labelEn: 'Intake Triage' },
  { id: 'loops.garden', group: 'loops', kind: 'loop', key: 'garden', labelZh: '知识库整理', labelEn: 'Garden Maintenance' },

  // Execution Routes
  { id: 'routes.default', group: 'routes', kind: 'route', key: 'default', labelZh: '默认常规执行 (default)', labelEn: 'Default Route' },
  { id: 'routes.complex', group: 'routes', kind: 'route', key: 'complex', labelZh: '复杂跨模块 (complex)', labelEn: 'Complex Route' },
  { id: 'routes.frontend', group: 'routes', kind: 'route', key: 'frontend', labelZh: '前端界面实现 (frontend)', labelEn: 'Frontend Route' },
  { id: 'routes.multimodal', group: 'routes', kind: 'route', key: 'multimodal', labelZh: '多模态 / 视觉 (multimodal)', labelEn: 'Multimodal Route' },
  { id: 'routes.backend', group: 'routes', kind: 'route', key: 'backend', labelZh: '后端核心逻辑 (backend)', labelEn: 'Backend Route' },
  { id: 'routes.batch', group: 'routes', kind: 'route', key: 'batch', labelZh: '批量迁移与调研 (batch)', labelEn: 'Batch / Research Route' },
  { id: 'routes.hard', group: 'routes', kind: 'route', key: 'hard', labelZh: '高难并发 / 攻关 (hard)', labelEn: 'Hard Route' },

  // Dual-Draft & Compact Planning
  { id: 'planning.dual-draft.secondary-profile', group: 'planning', kind: 'profile-ref', key: 'secondary-profile', path: ['planning', 'dual-draft', 'secondary-profile'], labelZh: 'Full 独立对照稿', labelEn: 'Full Independent Draft' },
  { id: 'planning.dual-draft.synthesizer-profile', group: 'planning', kind: 'profile-ref', key: 'synthesizer-profile', path: ['planning', 'dual-draft', 'synthesizer-profile'], labelZh: 'Full 双稿统稿模型', labelEn: 'Full Synthesizer' },
  { id: 'planning.compact.design-plan', group: 'planning', kind: 'profile-ref', key: 'compact-design-plan', path: ['planning', 'compact', 'design-plan'], labelZh: 'Compact 轻量设计稿', labelEn: 'Compact Design Draft' },
  { id: 'planning.compact.design-review', group: 'planning', kind: 'profile-ref', key: 'compact-design-review', path: ['planning', 'compact', 'design-review'], labelZh: 'Compact 轻量设计审查', labelEn: 'Compact Design Review', readOnly: true },
];

function cloneRecord(value: Record<string, unknown> | undefined): Record<string, unknown> {
  return JSON.parse(JSON.stringify(value ?? {})) as Record<string, unknown>;
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function getNested(obj: Record<string, unknown>, path: string[]): unknown {
  let curr: unknown = obj;
  for (const key of path) {
    if (!curr || typeof curr !== 'object' || Array.isArray(curr)) return undefined;
    curr = (curr as Record<string, unknown>)[key];
  }
  return curr;
}

function setNested(obj: Record<string, unknown>, path: string[], value: unknown): Record<string, unknown> {
  const next = cloneRecord(obj);
  let curr: Record<string, unknown> = next;
  for (let i = 0; i < path.length - 1; i++) {
    const key = path[i];
    const existing = curr[key];
    if (!existing || typeof existing !== 'object' || Array.isArray(existing)) {
      curr[key] = {};
    }
    curr = curr[key] as Record<string, unknown>;
  }
  curr[path[path.length - 1]] = value;
  return next;
}

function matchProfileName(
  profiles: Record<string, unknown>,
  harness: string,
  model: string,
  thinking: string,
): string {
  if (!harness && !model) return '';
  for (const [name, raw] of Object.entries(profiles)) {
    const p = asRecord(raw);
    if (
      String(p.harness ?? '') === harness &&
      String(p.model ?? '') === model &&
      String(p.thinking ?? '') === thinking
    ) {
      return name;
    }
  }
  return '';
}

function resolvedInlineFromProfile(prof: Record<string, unknown>): ProfileRecord {
  const harness = String(prof.harness ?? '');
  const out: ProfileRecord = {
    harness,
    channel: String(prof.channel ?? HARNESS_CHANNEL[harness] ?? ''),
    model: String(prof.model ?? ''),
    thinking: String(prof.thinking ?? ''),
  };
  if (typeof prof['native-profile'] === 'string' && prof['native-profile']) {
    out['native-profile'] = prof['native-profile'];
  }
  return out;
}

function applyPatchToInline(
  draft: Record<string, unknown>,
  existing: Record<string, unknown>,
  patch: Partial<ProfileRecord>,
): ProfileRecord {
  const merged: ProfileRecord = { ...existing, ...patch };
  delete merged.profile;
  if (patch.harness && patch.harness !== existing.harness) {
    merged.channel = HARNESS_CHANNEL[patch.harness] ?? merged.channel ?? '';
    const nativeProfiles = asRecord(draft['native-profiles']);
    const npName = typeof merged['native-profile'] === 'string' ? merged['native-profile'] : '';
    if (npName) {
      const npDef = asRecord(nativeProfiles[npName]);
      if (npDef.harness && npDef.harness !== patch.harness) {
        delete merged['native-profile'];
      }
    }
  }
  return merged;
}

function resolveStageValues(draft: Record<string, unknown>, spec: StageSpec): ResolvedStageValues {
  const profiles = asRecord(draft.profiles);

  if (spec.kind === 'profile-ref' && spec.path) {
    const profileName = String(getNested(draft, spec.path) ?? '');
    const prof = asRecord(profiles[profileName]);
    return {
      profileName,
      harness: String(prof.harness ?? ''),
      model: String(prof.model ?? ''),
      thinking: String(prof.thinking ?? ''),
    };
  }

  if (spec.kind === 'route') {
    const initial = String(getNested(draft, ['execution-policy', 'routes', spec.key, 'initial']) ?? '');
    const policyBinding = initial ? asRecord(getNested(draft, ['execution-policy', 'bindings', initial])) : {};
    const routeEntry = asRecord(getNested(draft, ['routes', spec.key]));
    const routeProfName = typeof routeEntry.profile === 'string' ? routeEntry.profile : '';
    const baseProf = asRecord(profiles[initial || routeProfName]);
    const effective = Object.keys(policyBinding).length > 0 ? policyBinding : Object.keys(routeEntry).length > 0 ? routeEntry : baseProf;
    const harness = String(effective.harness ?? baseProf.harness ?? '');
    const model = String(effective.model ?? baseProf.model ?? '');
    const thinking = String(effective.thinking ?? baseProf.thinking ?? '');
    const profileName = initial || routeProfName || matchProfileName(profiles, harness, model, thinking);
    return { profileName, harness, model, thinking };
  }

  // kind === 'loop' (Go config-edit resolves loops.<key> into inline { harness, model, channel, thinking })
  const loopEntry = asRecord(getNested(draft, ['loops', spec.key]));
  const explicitProfile = typeof loopEntry.profile === 'string' ? loopEntry.profile : '';
  const baseProf = explicitProfile ? asRecord(profiles[explicitProfile]) : {};
  const harness = String(loopEntry.harness ?? baseProf.harness ?? '');
  const model = String(loopEntry.model ?? baseProf.model ?? '');
  const thinking = String(loopEntry.thinking ?? baseProf.thinking ?? '');
  const profileName = explicitProfile || matchProfileName(profiles, harness, model, thinking);
  return { profileName, harness, model, thinking };
}

function updateProfileAndPolicyBinding(
  draft: Record<string, unknown>,
  profileName: string,
  patch: Partial<ProfileRecord>,
): Record<string, unknown> {
  if (!profileName) return draft;
  const profiles = asRecord(draft.profiles);
  const merged = applyPatchToInline(draft, asRecord(profiles[profileName]), patch);
  let next = setNested(draft, ['profiles', profileName], merged);

  const execBindings = asRecord(getNested(next, ['execution-policy', 'bindings']));
  if (execBindings[profileName]) {
    const existingBinding = asRecord(execBindings[profileName]);
    const nextBinding: Record<string, unknown> = {
      ...existingBinding,
      ...(merged.harness !== undefined ? { harness: merged.harness } : {}),
      ...(merged.channel !== undefined ? { channel: merged.channel } : {}),
      ...(merged.model !== undefined ? { model: merged.model } : {}),
      ...(merged.thinking !== undefined ? { thinking: merged.thinking } : {}),
    };
    if (merged['native-profile']) nextBinding['native-profile'] = merged['native-profile'];
    else delete nextBinding['native-profile'];
    next = setNested(next, ['execution-policy', 'bindings', profileName], nextBinding);
  }
  return next;
}

function applyStageProfileChange(
  draft: Record<string, unknown>,
  spec: StageSpec,
  profileName: string,
): Record<string, unknown> {
  if (!profileName) return draft;
  const profiles = asRecord(draft.profiles);
  const prof = asRecord(profiles[profileName]);
  const inline = Object.keys(prof).length > 0 ? resolvedInlineFromProfile(prof) : { profile: profileName };

  if (spec.kind === 'profile-ref' && spec.path) {
    return setNested(draft, spec.path, profileName);
  }

  if (spec.kind === 'loop') {
    return setNested(draft, ['loops', spec.key], inline);
  }

  // kind === 'route'
  let next = setNested(draft, ['routes', spec.key], inline);
  const execPolicy = asRecord(next['execution-policy']);
  if (Object.keys(execPolicy).length > 0) {
    const routeEntry = asRecord(getNested(next, ['execution-policy', 'routes', spec.key]));
    const allowed = Array.isArray(routeEntry.allowed) ? [...(routeEntry.allowed as string[])] : [];
    if (!allowed.includes(profileName)) allowed.unshift(profileName);
    next = setNested(next, ['execution-policy', 'routes', spec.key, 'initial'], profileName);
    next = setNested(next, ['execution-policy', 'routes', spec.key, 'allowed'], allowed);

    const bindings = asRecord(execPolicy.bindings);
    if (!bindings[profileName] && Object.keys(prof).length > 0) {
      const sessionMode = String(getNested(next, ['sessions', 'mode']) ?? 'scoped');
      next = setNested(next, ['execution-policy', 'bindings', profileName], {
        harness: String(prof.harness ?? ''),
        provider: String(prof.provider ?? ''),
        channel: String(prof.channel ?? HARNESS_CHANNEL[String(prof.harness ?? '')] ?? ''),
        model: String(prof.model ?? ''),
        thinking: String(prof.thinking ?? ''),
        permission: 'danger-full-access',
        'session-mode': sessionMode,
        ...(prof['native-profile'] ? { 'native-profile': prof['native-profile'] } : {}),
      });
    }
  }
  return next;
}

function applyStageFieldChange(
  draft: Record<string, unknown>,
  spec: StageSpec,
  patch: Partial<ProfileRecord>,
): Record<string, unknown> {
  const current = resolveStageValues(draft, spec);

  if (spec.kind === 'profile-ref') {
    return updateProfileAndPolicyBinding(draft, current.profileName, patch);
  }

  if (spec.kind === 'loop') {
    const existing = asRecord(getNested(draft, ['loops', spec.key]));
    const base: Record<string, unknown> = {
      harness: current.harness,
      channel: HARNESS_CHANNEL[current.harness] ?? String(existing.channel ?? ''),
      model: current.model,
      thinking: current.thinking,
      ...existing,
    };
    const merged = applyPatchToInline(draft, base, patch);
    return setNested(draft, ['loops', spec.key], merged);
  }

  // kind === 'route'
  const existingRoute = asRecord(getNested(draft, ['routes', spec.key]));
  const baseRoute: Record<string, unknown> = {
    harness: current.harness,
    channel: HARNESS_CHANNEL[current.harness] ?? String(existingRoute.channel ?? ''),
    model: current.model,
    thinking: current.thinking,
    ...existingRoute,
  };
  const mergedRoute = applyPatchToInline(draft, baseRoute, patch);
  let next = setNested(draft, ['routes', spec.key], mergedRoute);

  const initial = String(getNested(next, ['execution-policy', 'routes', spec.key, 'initial']) ?? '');
  if (initial) {
    next = updateProfileAndPolicyBinding(next, initial, patch);
  }
  return next;
}

function syncSessionMode(draft: Record<string, unknown>, mode: 'scoped' | 'fresh'): Record<string, unknown> {
  let next = setNested(draft, ['sessions', 'mode'], mode);
  if (getNested(next, ['sessions', 'policy-version']) === undefined) {
    next = setNested(next, ['sessions', 'policy-version'], 1);
  }
  const execBindings = asRecord(getNested(next, ['execution-policy', 'bindings']));
  for (const key of Object.keys(execBindings)) {
    next = setNested(next, ['execution-policy', 'bindings', key, 'session-mode'], mode);
  }
  return next;
}

function isEligibleJevProfile(prof: Record<string, unknown>): boolean {
  const model = typeof prof.model === 'string' ? prof.model : '';
  return (
    prof.harness === 'antigravity' &&
    !prof.provider &&
    !prof['native-profile'] &&
    model.length > 0 &&
    !model.startsWith('gemini-pro-orchestrator')
  );
}

function isValidJevHttpEndpoint(raw: string): boolean {
  const trimmed = raw.trim();
  if (!trimmed || trimmed.length > 2000 || /\s/.test(trimmed)) return false;
  const match = /^(https?):\/\/(\[[0-9a-fA-F:]+\]|[^/:?#]+)(?::(\d+))?(?:[/?#].*)?$/.exec(trimmed);
  if (!match) return false;
  const [, scheme, host, port] = match;
  if (port !== undefined) {
    const portNum = Number(port);
    if (!Number.isInteger(portNum) || portNum < 1 || portNum > 65535) return false;
  }
  if (scheme === 'https') return Boolean(host);
  const lower = host.toLowerCase();
  return lower === 'localhost' || lower === '127.0.0.1' || lower === '[::1]';
}

function defaultJevProfileName(
  draft: Record<string, unknown>,
  defaults: Record<string, unknown> | undefined,
): string {
  const profiles = asRecord(draft.profiles);
  const defJev = asRecord(defaults?.jev);
  if (typeof defJev.profile === 'string' && defJev.profile && isEligibleJevProfile(asRecord(profiles[defJev.profile]))) {
    return defJev.profile;
  }
  if (isEligibleJevProfile(asRecord(profiles['hydrogen-summary']))) return 'hydrogen-summary';
  if (isEligibleJevProfile(asRecord(profiles['hydrogen-local']))) return 'hydrogen-local';
  const eligible = Object.keys(profiles).sort().find(name => isEligibleJevProfile(asRecord(profiles[name])));
  return eligible ?? (typeof defJev.profile === 'string' && defJev.profile ? defJev.profile : 'hydrogen-summary');
}

function buildJevSection(
  draft: Record<string, unknown>,
  defaults: Record<string, unknown> | undefined,
  patch: Record<string, unknown>,
): Record<string, unknown> {
  const prev = asRecord(draft.jev);
  const nextJev: Record<string, unknown> = { ...prev, ...patch };
  for (const [k, v] of Object.entries(nextJev)) {
    if (v === undefined) delete nextJev[k];
  }
  const backend = nextJev.backend === 'http' ? 'http' : 'subscription';
  nextJev.backend = backend;
  if (backend === 'subscription') {
    delete nextJev.endpoint;
    delete nextJev.model;
    delete nextJev['key-env'];
    if (typeof nextJev.profile !== 'string' || !nextJev.profile) {
      nextJev.profile = defaultJevProfileName(draft, defaults);
    }
  } else {
    delete nextJev.profile;
    if (typeof nextJev.endpoint !== 'string') nextJev.endpoint = '';
    if (typeof nextJev.model !== 'string' || !nextJev.model) nextJev.model = 'jev-1.14.0';
    if (typeof nextJev['key-env'] !== 'string' || !nextJev['key-env']) nextJev['key-env'] = 'MEROUTER_API_KEY';
  }
  return setNested(draft, ['jev'], nextJev);
}

function toggleJevAllowedData(
  draft: Record<string, unknown>,
  defaults: Record<string, unknown> | undefined,
  category: string,
): Record<string, unknown> {
  const prev = asRecord(draft.jev);
  const raw = Array.isArray(prev['allowed-data'])
    ? (prev['allowed-data'] as unknown[]).map(String)
    : ['task-summary', 'diagnostic-facts', 'source'];
  const set = new Set(raw);
  if (set.has(category)) set.delete(category);
  else set.add(category);
  const ordered = JEV_ALLOWED_DATA_CATEGORIES.filter(item => set.has(item));
  return buildJevSection(draft, defaults, { 'allowed-data': ordered });
}

function validateJevDraft(
  draft: Record<string, unknown>,
  profilesMap: Record<string, unknown>,
  t: Translate,
): Record<string, string> {
  const errors: Record<string, string> = {};
  if (draft.jev === undefined) return errors;
  const jev = asRecord(draft.jev);
  const enabled = Boolean(jev.enabled);
  const backend = jev.backend === 'http' ? 'http' : 'subscription';
  const subMode = String(getNested(draft, ['subscription', 'mode']) ?? '');

  if (enabled && subMode === 'subscription-only' && backend !== 'subscription') {
    errors.backend = t('settings.jev.errSubOnly');
  }

  if (backend === 'subscription') {
    const profileName = typeof jev.profile === 'string' ? jev.profile.trim() : '';
    const hasProfiles = Object.keys(profilesMap).length > 0;
    if (enabled || (hasProfiles && profileName)) {
      const targetProf = asRecord(profilesMap[profileName]);
      if (!profileName || (hasProfiles && !isEligibleJevProfile(targetProf))) {
        errors.profile = t('settings.jev.errProfile');
      }
    }
  } else {
    const endpoint = typeof jev.endpoint === 'string' ? jev.endpoint : '';
    const model = typeof jev.model === 'string' ? jev.model : '';
    const keyEnv = typeof jev['key-env'] === 'string' ? jev['key-env'] : '';
    if (enabled && !isValidJevHttpEndpoint(endpoint)) {
      errors.endpoint = t('settings.jev.errEndpoint');
    } else if (endpoint.trim() && !isValidJevHttpEndpoint(endpoint)) {
      errors.endpoint = t('settings.jev.errEndpoint');
    }
    if (model && (model.length > 120 || !JEV_MODEL_RE.test(model))) {
      errors.model = t('settings.jev.errModel');
    } else if (enabled && !model) {
      errors.model = t('settings.jev.errModel');
    }
    if (keyEnv && !JEV_KEY_ENV_RE.test(keyEnv)) {
      errors.keyEnv = t('settings.jev.errKeyEnv');
    } else if (enabled && !keyEnv) {
      errors.keyEnv = t('settings.jev.errKeyEnv');
    }
  }

  if (jev['timeout-ms'] !== undefined) {
    const v = Number(jev['timeout-ms']);
    if (!Number.isInteger(v) || v < 1 || v > 30000) {
      errors.timeoutMs = t('settings.jev.errTimeout');
    }
  }
  if (jev['max-calls'] !== undefined) {
    const v = Number(jev['max-calls']);
    if (!Number.isInteger(v) || v < 1 || v > 100) {
      errors.maxCalls = t('settings.jev.errMaxCalls');
    }
  }
  if (jev['min-applicability'] !== undefined) {
    const v = Number(jev['min-applicability']);
    if (!Number.isFinite(v) || v <= 0 || v > 1) {
      errors.minApplicability = t('settings.jev.errThreshold');
    }
  }
  if (jev['min-choice'] !== undefined) {
    const v = Number(jev['min-choice']);
    if (!Number.isFinite(v) || v <= 0 || v > 1) {
      errors.minChoice = t('settings.jev.errThreshold');
    }
  }

  return errors;
}

const WATCHDOG_PLANNING_ROLES = [
  'design-plan',
  'design-review',
  'plan',
  'plan-review',
] as const;

function formatSecondsHint(seconds: number, isZh: boolean): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '';
  if (seconds < 60) return isZh ? `${seconds} 秒` : `${seconds}s`;
  const mins = seconds / 60;
  if (mins < 60) {
    const label = Number.isInteger(mins) ? String(mins) : mins.toFixed(1);
    return isZh ? `约 ${label} 分钟` : `~${label}m`;
  }
  const hrs = mins / 60;
  const label = Number.isInteger(hrs) ? String(hrs) : hrs.toFixed(1);
  return isZh ? `约 ${label} 小时` : `~${label}h`;
}

function patchScheduler(draft: Record<string, unknown>, patch: Record<string, unknown>): Record<string, unknown> {
  const prev = asRecord(draft.scheduler);
  const next: Record<string, unknown> = {
    mode: prev.mode === 'serial' ? 'serial' : 'parallel',
    'max-parallel': typeof prev['max-parallel'] === 'number' ? prev['max-parallel'] : 2,
    dispatch: prev.dispatch === 'ready-queue' ? 'ready-queue' : 'batch',
    ...prev,
    ...patch,
  };
  return setNested(draft, ['scheduler'], next);
}

function patchWatchdog(draft: Record<string, unknown>, patch: Record<string, unknown>): Record<string, unknown> {
  const prev = asRecord(draft.watchdog);
  const next: Record<string, unknown> = {
    'idle-timeout-ms': typeof prev['idle-timeout-ms'] === 'number' ? prev['idle-timeout-ms'] : 300_000,
    'wall-timeout-ms': typeof prev['wall-timeout-ms'] === 'number' ? prev['wall-timeout-ms'] : 3_600_000,
    'kill-grace-ms': typeof prev['kill-grace-ms'] === 'number' ? prev['kill-grace-ms'] : 5_000,
    ...prev,
    ...patch,
  };
  return setNested(draft, ['watchdog'], next);
}

function setPlanningIdleTimeoutMs(draft: Record<string, unknown>, ms: number): Record<string, unknown> {
  const prevWatchdog = asRecord(draft.watchdog);
  const prevRoles = asRecord(prevWatchdog.roles);
  const nextRoles: Record<string, unknown> = { ...prevRoles };
  for (const role of WATCHDOG_PLANNING_ROLES) {
    nextRoles[role] = {
      ...asRecord(prevRoles[role]),
      'idle-timeout-ms': ms,
    };
  }
  return patchWatchdog(draft, { roles: nextRoles });
}

function patchRetry(draft: Record<string, unknown>, patch: Record<string, unknown>): Record<string, unknown> {
  const prev = asRecord(draft.retry);
  const next: Record<string, unknown> = {
    'base-delay-ms': typeof prev['base-delay-ms'] === 'number' ? prev['base-delay-ms'] : 500,
    'max-delay-ms': typeof prev['max-delay-ms'] === 'number' ? prev['max-delay-ms'] : 10_000,
    jitter: typeof prev.jitter === 'number' ? prev.jitter : 0.1,
    'max-transport-retries': typeof prev['max-transport-retries'] === 'number' ? prev['max-transport-retries'] : 5,
    ...prev,
    ...patch,
  };
  return setNested(draft, ['retry'], next);
}

function validateRuntimeDraft(draft: Record<string, unknown>, t: Translate): Record<string, string> {
  const errors: Record<string, string> = {};

  if (draft.scheduler !== undefined) {
    const sched = asRecord(draft.scheduler);
    if (sched['max-parallel'] !== undefined) {
      const mp = Number(sched['max-parallel']);
      if (!Number.isInteger(mp) || mp < 1 || mp > 8) {
        errors.maxParallel = t('settings.runtime.errMaxParallel');
      }
    }
  }

  if (draft.watchdog !== undefined) {
    const wd = asRecord(draft.watchdog);
    if (wd['idle-timeout-ms'] !== undefined) {
      const v = Number(wd['idle-timeout-ms']);
      if (!Number.isInteger(v) || v < 1000 || v > 86_400_000) {
        errors.idleTimeout = t('settings.runtime.errTimeoutSec');
      }
    }
    if (wd['wall-timeout-ms'] !== undefined) {
      const v = Number(wd['wall-timeout-ms']);
      if (!Number.isInteger(v) || v < 1000 || v > 86_400_000) {
        errors.wallTimeout = t('settings.runtime.errTimeoutSec');
      }
    }
    if (wd['kill-grace-ms'] !== undefined) {
      const v = Number(wd['kill-grace-ms']);
      if (!Number.isInteger(v) || v < 1000 || v > 60_000) {
        errors.killGrace = t('settings.runtime.errKillGraceSec');
      }
    }
    const roles = asRecord(wd.roles);
    for (const role of WATCHDOG_PLANNING_ROLES) {
      const entry = asRecord(roles[role]);
      if (entry['idle-timeout-ms'] !== undefined) {
        const v = Number(entry['idle-timeout-ms']);
        if (!Number.isInteger(v) || v < 1000 || v > 86_400_000) {
          errors.planIdleTimeout = t('settings.runtime.errTimeoutSec');
          break;
        }
      }
    }
  }

  if (draft.retry !== undefined) {
    const rt = asRecord(draft.retry);
    const baseDelay = rt['base-delay-ms'] !== undefined ? Number(rt['base-delay-ms']) : 500;
    const maxDelay = rt['max-delay-ms'] !== undefined ? Number(rt['max-delay-ms']) : 10_000;
    if (rt['max-transport-retries'] !== undefined) {
      const v = Number(rt['max-transport-retries']);
      if (!Number.isInteger(v) || v < 0 || v > 20) {
        errors.maxRetries = t('settings.runtime.errRetries');
      }
    }
    if (rt['base-delay-ms'] !== undefined) {
      if (!Number.isFinite(baseDelay) || baseDelay < 1 || baseDelay > 60_000) {
        errors.baseDelay = t('settings.runtime.errBaseDelay');
      }
    }
    if (rt['max-delay-ms'] !== undefined) {
      if (!Number.isFinite(maxDelay) || maxDelay < baseDelay || maxDelay > 300_000) {
        errors.maxDelay = t('settings.runtime.errMaxDelay');
      }
    }
    if (rt.jitter !== undefined) {
      const v = Number(rt.jitter);
      if (!Number.isFinite(v) || v < 0 || v > 1) {
        errors.jitter = t('settings.runtime.errJitter');
      }
    }
  }

  return errors;
}

export function SettingsPage({
  store,
  t,
  inline = false,
}: {
  store: FlywheelStore;
  t: Translate;
  inline?: boolean;
}) {
  const snap = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  const [draft, setDraft] = useState<Record<string, unknown>>({});
  const [roleGroup, setRoleGroup] = useState<RoleGroup>('loops');
  const [jevAdvancedOpen, setJevAdvancedOpen] = useState(false);
  const [jevNumText, setJevNumText] = useState<Record<string, string>>({});
  const [runtimeRetryOpen, setRuntimeRetryOpen] = useState(false);
  const [runtimeNumText, setRuntimeNumText] = useState<Record<string, string>>({});
  const [tier3Open, setTier3Open] = useState(false);
  const [catalogLoading, setCatalogLoading] = useState(false);

  const isZh = t('settings.scope.global') === '全局配置';

  const initialLoadedRef = useRef(false);
  useEffect(() => {
    if (initialLoadedRef.current) return;
    initialLoadedRef.current = true;
    void store.loadConfig(snap.configScope);
  }, [store, snap.configScope]);

  useEffect(() => {
    if (snap.configEditor?.values) {
      setDraft(cloneRecord(snap.configEditor.values));
      setJevNumText({});
      setRuntimeNumText({});
    }
  }, [snap.configEditor]);

  const dirty = useMemo(() => {
    if (!snap.configEditor?.values) return false;
    return JSON.stringify(draft) !== JSON.stringify(snap.configEditor.values);
  }, [draft, snap.configEditor?.values]);

  const dualDraftEnabled = Boolean(getNested(draft, ['planning', 'dual-draft', 'enabled']) ?? true);
  const uxAuditEnabled = Boolean(getNested(draft, ['frontend-ux-review', 'enabled']) ?? true);
  const sessionMode = String(getNested(draft, ['sessions', 'mode']) ?? 'scoped') === 'fresh' ? 'fresh' : 'scoped';
  const deliveryEndpoint = String(getNested(draft, ['delivery', 'endpoint']) ?? 'local');
  const maxAttempts = Number(getNested(draft, ['guardrails', 'max-packet-attempts']) ?? 3);
  const maxReviewRounds = Number(getNested(draft, ['guardrails', 'max-review-rounds']) ?? 2);
  const maxTaskCalls = Number(getNested(draft, ['guardrails', 'max-task-calls']) ?? 60);
  const budgetEnforce = String(getNested(draft, ['budget', 'enforce']) ?? 'off');

  const profilesMap = useMemo(() => asRecord(draft.profiles), [draft.profiles]);
  const profileNames = useMemo(() => Object.keys(profilesMap).sort(), [profilesMap]);
  const eligibleJevProfiles = useMemo(
    () => profileNames.filter(name => isEligibleJevProfile(asRecord(profilesMap[name]))),
    [profileNames, profilesMap],
  );
  const jevDraft = asRecord(draft.jev);
  const jevDefaults = asRecord(snap.configEditor?.defaults?.jev);
  const jevEnabled = Boolean(jevDraft.enabled);
  const jevBackend: 'subscription' | 'http' = jevDraft.backend === 'http' ? 'http' : 'subscription';
  const jevProfile = typeof jevDraft.profile === 'string' && jevDraft.profile
    ? jevDraft.profile
    : defaultJevProfileName(draft, snap.configEditor?.defaults);
  const jevResolvedProfile = asRecord(profilesMap[jevProfile]);
  const jevAllowedData = useMemo(() => {
    if (Array.isArray(jevDraft['allowed-data'])) {
      return new Set((jevDraft['allowed-data'] as unknown[]).map(String));
    }
    if (Array.isArray(jevDefaults['allowed-data'])) {
      return new Set((jevDefaults['allowed-data'] as unknown[]).map(String));
    }
    return new Set(['task-summary', 'diagnostic-facts', 'source']);
  }, [jevDraft, jevDefaults]);
  const jevErrors = useMemo(
    () => validateJevDraft(draft, profilesMap, t),
    [draft, profilesMap, t],
  );
  const runtimeErrors = useMemo(
    () => validateRuntimeDraft(draft, t),
    [draft, t],
  );
  const hasJevErrors = Object.keys(jevErrors).length > 0;
  const hasRuntimeErrors = Object.keys(runtimeErrors).length > 0;
  const hasValidationErrors = hasJevErrors || hasRuntimeErrors;

  const schedulerDraft = asRecord(draft.scheduler);
  const schedulerMode: 'parallel' | 'serial' = schedulerDraft.mode === 'serial' ? 'serial' : 'parallel';
  const schedulerDispatch: 'batch' | 'ready-queue' =
    schedulerDraft.dispatch === 'ready-queue' ? 'ready-queue' : 'batch';
  const schedulerPrewarm = Boolean(schedulerDraft.prewarm ?? false);
  const worktreeEnabled = getNested(draft, ['worktree', 'enabled']) === undefined
    ? true
    : Boolean(getNested(draft, ['worktree', 'enabled']));
  const watchdogDraft = asRecord(draft.watchdog);
  const watchdogRolesDraft = asRecord(watchdogDraft.roles);
  const retryDraft = asRecord(draft.retry);

  const idleTimeoutSecNum = Math.round(Number(watchdogDraft['idle-timeout-ms'] ?? 300_000) / 1000);
  const planIdleTimeoutSecNum = Math.round(
    Number(asRecord(watchdogRolesDraft['design-plan'])['idle-timeout-ms'] ?? 900_000) / 1000,
  );
  const wallTimeoutSecNum = Math.round(Number(watchdogDraft['wall-timeout-ms'] ?? 3_600_000) / 1000);
  const killGraceSecNum = Math.round(Number(watchdogDraft['kill-grace-ms'] ?? 5_000) / 1000);

  const runtimeInputVal = (key: string, fallback: number): string => {
    if (Object.hasOwn(runtimeNumText, key)) return runtimeNumText[key];
    return Number.isFinite(fallback) ? String(fallback) : '';
  };

  const handleRuntimeSecChange = (
    key: 'idleTimeout' | 'planIdleTimeout' | 'wallTimeout' | 'killGrace',
    raw: string,
  ) => {
    setRuntimeNumText(prev => ({ ...prev, [key]: raw }));
    const parsed = Number(raw.trim());
    const ms = raw.trim() && Number.isFinite(parsed) && Number.isInteger(parsed) ? parsed * 1000 : Number.NaN;
    if (key === 'planIdleTimeout') {
      setDraft(d => setPlanningIdleTimeoutMs(d, ms));
    } else if (key === 'idleTimeout') {
      setDraft(d => patchWatchdog(d, { 'idle-timeout-ms': ms }));
    } else if (key === 'wallTimeout') {
      setDraft(d => patchWatchdog(d, { 'wall-timeout-ms': ms }));
    } else {
      setDraft(d => patchWatchdog(d, { 'kill-grace-ms': ms }));
    }
  };

  const handleSchedulerParallelChange = (raw: string) => {
    setRuntimeNumText(prev => ({ ...prev, maxParallel: raw }));
    const parsed = Number(raw.trim());
    const val = raw.trim() && Number.isFinite(parsed) ? parsed : Number.NaN;
    setDraft(d => patchScheduler(d, { 'max-parallel': val }));
  };

  const handleRetryNumChange = (
    field: 'max-transport-retries' | 'base-delay-ms' | 'max-delay-ms' | 'jitter',
    raw: string,
  ) => {
    setRuntimeNumText(prev => ({ ...prev, [field]: raw }));
    const parsed = Number(raw.trim());
    const val = raw.trim() && Number.isFinite(parsed) ? parsed : Number.NaN;
    setDraft(d => patchRetry(d, { [field]: val }));
  };
  const effectiveJev = snap.effectiveConfig?.jev;
  const effectiveJevBinding = snap.effectiveConfig?.jevBinding;
  const jevNumValue = (field: 'max-calls' | 'timeout-ms' | 'min-applicability' | 'min-choice', fallback: number): string => {
    if (Object.hasOwn(jevNumText, field)) return jevNumText[field];
    const v = jevDraft[field] ?? jevDefaults[field];
    return v === undefined || v === null ? String(fallback) : String(v);
  };
  const handleJevNumChange = (
    field: 'max-calls' | 'timeout-ms' | 'min-applicability' | 'min-choice',
    raw: string,
    integer: boolean,
  ) => {
    setJevNumText(prev => ({ ...prev, [field]: raw }));
    const trimmed = raw.trim();
    if (!trimmed) {
      setDraft(d => buildJevSection(d, snap.configEditor?.defaults, { [field]: undefined }));
      return;
    }
    const parsed = Number(trimmed);
    if (!Number.isFinite(parsed)) {
      setDraft(d => buildJevSection(d, snap.configEditor?.defaults, { [field]: Number.NaN }));
      return;
    }
    const val = integer ? (Number.isInteger(parsed) ? parsed : parsed) : parsed;
    setDraft(d => buildJevSection(d, snap.configEditor?.defaults, { [field]: val }));
  };
  const bindings = snap.effectiveConfig?.bindings ?? [];
  const checks = snap.doctor?.checks ?? [];

  const modelsByHarness = useMemo(() => {
    const map: Record<string, string[]> = {};
    for (const h of HARNESS_OPTIONS) {
      const set = new Set<string>(CANONICAL_MODELS[h.id] ?? []);
      for (const prof of Object.values(profilesMap)) {
        const rec = asRecord(prof);
        if (rec.harness === h.id && typeof rec.model === 'string' && rec.model) {
          set.add(rec.model);
        }
      }
      for (const catalog of Object.values(snap.modelCatalogs)) {
        if (catalog.harness === h.id) {
          for (const entry of catalog.models) {
            if (entry.id) set.add(entry.id);
          }
        }
      }
      map[h.id] = [...set];
    }
    return map;
  }, [profilesMap, snap.modelCatalogs]);

  const handleRefreshCatalogs = async () => {
    setCatalogLoading(true);
    try {
      await Promise.allSettled(
        SUBSCRIPTIONS.map(sub =>
          store.loadModelCatalog({ version: 1, harness: sub.id, channel: sub.channel }, { silent: true }),
        ),
      );
    } finally {
      setCatalogLoading(false);
    }
  };

  const checkStatusFor = (subId: string): { label: string; cls: string } => {
    const match = checks.find(c => c.id.toLowerCase().includes(subId.replace('-pro', '')));
    if (!match) return { label: '—', cls: '' };
    if (match.status === 'pass') return { label: 'Ready', cls: styles.pillSuccess };
    if (match.status === 'warn') return { label: 'Warn', cls: styles.pillWarn };
    if (match.status === 'fail') return { label: 'Issue', cls: styles.pillDanger };
    return { label: match.status, cls: '' };
  };

  const activeSpecs = useMemo(() => STAGE_SPECS.filter(s => s.group === roleGroup), [roleGroup]);
  const hasEditableConfig =
    profileNames.length > 0 ||
    Object.keys(asRecord(draft.loops)).length > 0 ||
    Object.keys(asRecord(draft.routes)).length > 0;

  return (
    <StyleScope className={`${styles.settingsPage} ${inline ? styles.settingsPageInline : ''}`}>
      <div className={styles.cardHeader}>
        <div>
          <h2 className={styles.headerTitle} style={{ fontSize: 16 }}>{t('settings.title')}</h2>
          <div style={{ fontSize: 12, color: 'var(--fw-text-muted)' }}>{t('settings.subtitle')}</div>
        </div>
        <div className={styles.segmented} role="tablist" aria-label="Config scope">
          <button
            type="button"
            role="tab"
            aria-selected={snap.configScope === 'global'}
            className={`${styles.segmentedBtn} ${snap.configScope === 'global' ? styles.segmentedBtnActive : ''}`}
            onClick={() => void store.loadConfig('global')}
          >
            {t('settings.scope.global')}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={snap.configScope === 'project'}
            className={`${styles.segmentedBtn} ${snap.configScope === 'project' ? styles.segmentedBtnActive : ''}`}
            onClick={() => void store.loadConfig('project')}
          >
            {t('settings.scope.project')}
          </button>
        </div>
      </div>

      {/* Scope Precedence Bar */}
      <div className={styles.pill} style={{ alignSelf: 'flex-start' }} data-testid="settings-scope-bar">
        {t('settings.scopeBar')}
        {snap.configEditor?.target ? ` · ${snap.configEditor.target}` : ''}
      </div>

      {snap.error && (
        <div className={styles.bannerError} role="alert">
          <span>{snap.error.message}</span>
          <button type="button" className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`} onClick={() => store.clearError()}>
            ×
          </button>
        </div>
      )}

      {(snap.notice === 'saved' || snap.notice === 'reset') && (
        <div className={styles.bannerSuccess}>
          <span>{snap.notice === 'reset' ? t('settings.resetDone') : t('settings.saved')}</span>
          <button type="button" className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`} onClick={() => store.clearNotice()}>
            ×
          </button>
        </div>
      )}

      {/* 4-Subscription Health Strip */}
      <section className={styles.card} data-testid="settings-health-strip">
        <div className={styles.cardHeader}>
          <div>
            <h3 className={styles.cardTitle}>{t('settings.healthTitle')}</h3>
            <div style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>{t('settings.preflightNote')}</div>
          </div>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <button
              type="button"
              className={styles.btn}
              disabled={catalogLoading || snap.busyAction !== null}
              onClick={() => void handleRefreshCatalogs()}
            >
              {t('settings.refreshCatalog')}
            </button>
            <button
              type="button"
              className={styles.btn}
              disabled={snap.busyAction !== null}
              onClick={() => void store.loadDoctor(false)}
            >
              {t('settings.preflight')}
            </button>
          </div>
        </div>

        <div className={styles.healthGrid}>
          {SUBSCRIPTIONS.map(sub => {
            const st = checkStatusFor(sub.id);
            const catalogCount = Object.values(snap.modelCatalogs).find(c => c.harness === sub.id)?.models.length;
            return (
              <div key={sub.id} className={styles.healthCard}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 6 }}>
                  <strong style={{ fontSize: 12.5 }}>{sub.name}</strong>
                  <span className={`${styles.pill} ${st.cls}`}>{st.label}</span>
                </div>
                <span style={{ fontSize: 11.5, color: 'var(--fw-text-muted)' }}>
                  {sub.desc}
                  {typeof catalogCount === 'number' && catalogCount > 0 ? ` · ${catalogCount} models` : ''}
                </span>
              </div>
            );
          })}
        </div>
      </section>

      {/* JEV Bounded Decision Settings */}
      <section className={styles.card} data-testid="settings-jev">
        <div className={styles.cardHeader} style={{ alignItems: 'flex-start' }}>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
              <h3 className={styles.cardTitle}>{t('settings.jev.title')}</h3>
              <span
                className={`${styles.pill} ${jevEnabled ? styles.pillSuccess : ''}`}
                data-testid="jev-status-pill"
              >
                {jevEnabled ? t('settings.jev.on') : t('settings.jev.off')}
              </span>
            </div>
            <div style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)', marginTop: 2 }}>
              {t('settings.jev.subtitle')}
            </div>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--fw-text)' }}>
              {t('settings.jev.enable')}
            </span>
            <button
              type="button"
              role="switch"
              aria-checked={jevEnabled}
              aria-label={t('settings.jev.enable')}
              data-testid="jev-enabled-toggle"
              className={`${styles.switchBtn} ${jevEnabled ? styles.switchBtnOn : ''}`}
              onClick={() => {
                const nextEnabled = !jevEnabled;
                if (nextEnabled) setJevAdvancedOpen(true);
                setDraft(d => buildJevSection(d, snap.configEditor?.defaults, { enabled: nextEnabled }));
              }}
            >
              <span className={styles.switchThumb} />
            </button>
          </div>
        </div>

        {/* Effective runtime JEV status bar (separate from editing draft) */}
        {effectiveJev && (
          <div className={styles.jevEffectiveBar} data-testid="jev-effective-bar">
            <span>
              <strong>{t('settings.jev.effective')}:</strong>{' '}
              <span className={`${styles.pill} ${effectiveJev.enabled ? styles.pillSuccess : ''}`}>
                {effectiveJev.enabled ? t('settings.jev.on') : t('settings.jev.off')}
              </span>
            </span>
            <span>
              {t('settings.jev.effectiveScope')}: <code>{snap.effectiveConfig?.scope || '—'}</code>
            </span>
            {effectiveJevBinding && (
              <span>
                {t('settings.jev.resolvedBinding')}:{' '}
                <code>
                  {effectiveJevBinding.profile || effectiveJev.profile || '—'}
                  {effectiveJevBinding.model ? ` · ${effectiveJevBinding.model}` : ''}
                  {effectiveJevBinding.thinking ? ` (${effectiveJevBinding.thinking})` : ''}
                </code>
              </span>
            )}
          </div>
        )}

        <div className={styles.settingRow} style={{ paddingTop: 4 }}>
          <div className={styles.settingRowText}>
            <span className={styles.settingRowTitle}>{t('settings.jev.backend')}</span>
            <span className={styles.settingRowDesc}>{t('settings.jev.enableDesc')}</span>
            {jevErrors.backend && <span className={styles.fieldError} role="alert">{jevErrors.backend}</span>}
          </div>
          <div className={styles.segmented} role="tablist" aria-label={t('settings.jev.backend')}>
            <button
              type="button"
              role="tab"
              aria-selected={jevBackend === 'subscription'}
              data-testid="jev-backend-subscription"
              className={`${styles.segmentedBtn} ${jevBackend === 'subscription' ? styles.segmentedBtnActive : ''}`}
              onClick={() =>
                setDraft(d => buildJevSection(d, snap.configEditor?.defaults, { backend: 'subscription' }))
              }
            >
              {t('settings.jev.backendSub')}
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={jevBackend === 'http'}
              data-testid="jev-backend-http"
              className={`${styles.segmentedBtn} ${jevBackend === 'http' ? styles.segmentedBtnActive : ''}`}
              onClick={() =>
                setDraft(d => buildJevSection(d, snap.configEditor?.defaults, { backend: 'http' }))
              }
            >
              {t('settings.jev.backendHttp')}
            </button>
          </div>
        </div>

        {jevBackend === 'subscription' ? (
          <div className={styles.jevProfileRow}>
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-jev-profile">{t('settings.jev.profile')}</label>
              <select
                id="fw-jev-profile"
                className={styles.select}
                data-testid="jev-profile-select"
                value={jevProfile}
                onChange={e =>
                  setDraft(d => buildJevSection(d, snap.configEditor?.defaults, { profile: e.target.value }))
                }
              >
                {jevProfile && !eligibleJevProfiles.includes(jevProfile) && (
                  <option value={jevProfile}>{jevProfile}</option>
                )}
                {eligibleJevProfiles.map(name => {
                  const p = asRecord(profilesMap[name]);
                  const summary = [p.model, p.thinking].filter(Boolean).join(' · ');
                  return (
                    <option key={name} value={name}>
                      {summary ? `${name} (${summary})` : name}
                    </option>
                  );
                })}
              </select>
              {jevErrors.profile && <span className={styles.fieldError} role="alert">{jevErrors.profile}</span>}
            </div>
            <div className={styles.jevResolvedBox} data-testid="jev-resolved-preview">
              <span>{t('settings.jev.resolvedBinding')}:</span>
              {Object.keys(jevResolvedProfile).length > 0 ? (
                <>
                  <code>{String(jevResolvedProfile.harness ?? 'antigravity')}</code>
                  <code>{String(jevResolvedProfile.model ?? '—')}</code>
                  {jevResolvedProfile.thinking ? (
                    <span className={styles.pill}>{String(jevResolvedProfile.thinking)}</span>
                  ) : null}
                  <span className={`${styles.pill} ${styles.pillSuccess}`}>{t('settings.readOnlyBadge')}</span>
                </>
              ) : (
                <span>{t('settings.jev.profileHint')}</span>
              )}
            </div>
          </div>
        ) : (
          <div className={styles.jevHttpGrid}>
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-jev-endpoint">{t('settings.jev.endpoint')}</label>
              <input
                id="fw-jev-endpoint"
                className={styles.input}
                data-testid="jev-endpoint-input"
                placeholder="https://example.com/v1/systemone"
                value={String(jevDraft.endpoint ?? '')}
                onChange={e =>
                  setDraft(d => buildJevSection(d, snap.configEditor?.defaults, { endpoint: e.target.value }))
                }
              />
              {jevErrors.endpoint && <span className={styles.fieldError} role="alert">{jevErrors.endpoint}</span>}
            </div>
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-jev-model">{t('settings.jev.model')}</label>
              <input
                id="fw-jev-model"
                className={styles.input}
                data-testid="jev-model-input"
                placeholder="jev-1.14.0"
                value={String(jevDraft.model ?? '')}
                onChange={e =>
                  setDraft(d => buildJevSection(d, snap.configEditor?.defaults, { model: e.target.value }))
                }
              />
              {jevErrors.model && <span className={styles.fieldError} role="alert">{jevErrors.model}</span>}
            </div>
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-jev-key-env">{t('settings.jev.keyEnv')}</label>
              <input
                id="fw-jev-key-env"
                className={styles.input}
                data-testid="jev-key-env-input"
                placeholder="MEROUTER_API_KEY"
                value={String(jevDraft['key-env'] ?? '')}
                onChange={e =>
                  setDraft(d => buildJevSection(d, snap.configEditor?.defaults, { 'key-env': e.target.value }))
                }
              />
              {jevErrors.keyEnv && <span className={styles.fieldError} role="alert">{jevErrors.keyEnv}</span>}
            </div>
          </div>
        )}

        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 2 }}>
          <span style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>
            {jevBackend === 'subscription' ? t('settings.jev.profileHint') : ''}
          </span>
          <button
            type="button"
            className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
            data-testid="jev-advanced-toggle"
            onClick={() => setJevAdvancedOpen(v => !v)}
          >
            {jevAdvancedOpen ? t('settings.jev.advancedHide') : t('settings.jev.advancedShow')}
            {' '}
            {jevAdvancedOpen ? '▾' : '▸'}
          </button>
        </div>

        {jevAdvancedOpen && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, paddingTop: 4 }} data-testid="jev-advanced-panel">
            <div className={styles.jevGrid}>
              <div className={styles.field}>
                <label className={styles.fieldLabel} htmlFor="fw-jev-max-calls">{t('settings.jev.maxCalls')}</label>
                <input
                  id="fw-jev-max-calls"
                  type="number"
                  min={1}
                  max={100}
                  step={1}
                  className={styles.input}
                  data-testid="jev-max-calls-input"
                  value={jevNumValue('max-calls', 8)}
                  onChange={e => handleJevNumChange('max-calls', e.target.value, true)}
                />
                {jevErrors.maxCalls && <span className={styles.fieldError} role="alert">{jevErrors.maxCalls}</span>}
              </div>
              <div className={styles.field}>
                <label className={styles.fieldLabel} htmlFor="fw-jev-timeout-ms">{t('settings.jev.timeoutMs')}</label>
                <input
                  id="fw-jev-timeout-ms"
                  type="number"
                  min={1}
                  max={30000}
                  step={500}
                  className={styles.input}
                  data-testid="jev-timeout-ms-input"
                  value={jevNumValue('timeout-ms', 30000)}
                  onChange={e => handleJevNumChange('timeout-ms', e.target.value, true)}
                />
                {jevErrors.timeoutMs && <span className={styles.fieldError} role="alert">{jevErrors.timeoutMs}</span>}
              </div>
              <div className={styles.field}>
                <label className={styles.fieldLabel} htmlFor="fw-jev-min-app">{t('settings.jev.minApplicability')}</label>
                <input
                  id="fw-jev-min-app"
                  type="number"
                  min={0.05}
                  max={1}
                  step={0.05}
                  className={styles.input}
                  data-testid="jev-min-applicability-input"
                  value={jevNumValue('min-applicability', 0.8)}
                  onChange={e => handleJevNumChange('min-applicability', e.target.value, false)}
                />
                {jevErrors.minApplicability && <span className={styles.fieldError} role="alert">{jevErrors.minApplicability}</span>}
              </div>
              <div className={styles.field}>
                <label className={styles.fieldLabel} htmlFor="fw-jev-min-choice">{t('settings.jev.minChoice')}</label>
                <input
                  id="fw-jev-min-choice"
                  type="number"
                  min={0.05}
                  max={1}
                  step={0.05}
                  className={styles.input}
                  data-testid="jev-min-choice-input"
                  value={jevNumValue('min-choice', 0.6)}
                  onChange={e => handleJevNumChange('min-choice', e.target.value, false)}
                />
                {jevErrors.minChoice && <span className={styles.fieldError} role="alert">{jevErrors.minChoice}</span>}
              </div>
            </div>

            <div className={styles.field}>
              <span className={styles.fieldLabel}>{t('settings.jev.allowedData')}</span>
              <div className={styles.jevDataChips}>
                {JEV_ALLOWED_DATA_CATEGORIES.map(category => {
                  const active = jevAllowedData.has(category);
                  return (
                    <button
                      key={category}
                      type="button"
                      aria-pressed={active}
                      data-testid={`jev-data-${category}`}
                      className={`${styles.jevDataChip} ${active ? styles.jevDataChipActive : ''}`}
                      onClick={() =>
                        setDraft(d => toggleJevAllowedData(d, snap.configEditor?.defaults, category))
                      }
                    >
                      <span>{active ? '✓' : '+'}</span>
                      <span>{t(`settings.jev.data.${category}`)}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          </div>
        )}
      </section>

      {/* Tier 2: Editable Per-Stage & Per-Route Model Matrix */}
      <section className={styles.card} data-testid="settings-tier2">
        <div className={styles.cardHeader} style={{ flexWrap: 'wrap' }}>
          <div>
            <h3 className={styles.cardTitle}>{t('settings.tier2')}</h3>
            <div style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>{t('settings.tier2Note')}</div>
          </div>
          {hasEditableConfig && (
            <div className={styles.segmented} role="tablist" aria-label="Role category">
              {(['loops', 'routes', 'planning'] as const).map(group => (
                <button
                  key={group}
                  type="button"
                  role="tab"
                  aria-selected={roleGroup === group}
                  className={`${styles.segmentedBtn} ${roleGroup === group ? styles.segmentedBtnActive : ''}`}
                  onClick={() => setRoleGroup(group)}
                >
                  {t(`settings.group.${group}`)}
                </button>
              ))}
            </div>
          )}
        </div>

        {hasEditableConfig ? (
          <div className={styles.matrixTableWrap}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>{t('settings.roleCol')}</th>
                  <th>{t('settings.profileCol')}</th>
                  <th>{t('settings.harnessCol')}</th>
                  <th>{t('settings.modelCol')}</th>
                  <th>{t('settings.thinkingCol')}</th>
                  <th>{t('settings.permCol')}</th>
                </tr>
              </thead>
              <tbody>
                {activeSpecs.map(spec => {
                  const resolved = resolveStageValues(draft, spec);
                  const modelSuggestions = modelsByHarness[resolved.harness] ?? [];
                  const datalistId = `fw-models-${spec.id.replace(/[^a-zA-Z0-9_-]/g, '-')}`;
                  const isReadOnly = Boolean(spec.readOnly) || READ_ONLY_ROLES.has(spec.id);
                  const fieldsDisabled = spec.kind === 'profile-ref' && !resolved.profileName;

                  return (
                    <tr key={spec.id} data-testid={`role-row-${spec.id}`}>
                      <td>
                        <div className={styles.roleMeta}>
                          <span className={styles.roleLabel}>{isZh ? spec.labelZh : spec.labelEn}</span>
                          <span className={styles.roleCode}>{spec.id}</span>
                        </div>
                      </td>
                      <td>
                        <select
                          className={styles.matrixSelect}
                          aria-label={`${spec.id} profile`}
                          data-testid={`role-profile-${spec.id}`}
                          value={resolved.profileName}
                          onChange={e => setDraft(d => applyStageProfileChange(d, spec, e.target.value))}
                        >
                          {!resolved.profileName && (
                            <option value="">{isZh ? '自定义' : 'Custom'}</option>
                          )}
                          {resolved.profileName && !profileNames.includes(resolved.profileName) && (
                            <option value={resolved.profileName}>{resolved.profileName}</option>
                          )}
                          {profileNames.map(name => (
                            <option key={name} value={name}>{name}</option>
                          ))}
                        </select>
                      </td>
                      <td>
                        <select
                          className={styles.matrixSelect}
                          aria-label={`${spec.id} harness`}
                          data-testid={`role-harness-${spec.id}`}
                          value={resolved.harness}
                          disabled={fieldsDisabled}
                          onChange={e =>
                            setDraft(d => applyStageFieldChange(d, spec, { harness: e.target.value }))
                          }
                        >
                          {resolved.harness && !HARNESS_OPTIONS.some(h => h.id === resolved.harness) && (
                            <option value={resolved.harness}>{resolved.harness}</option>
                          )}
                          {HARNESS_OPTIONS.map(opt => (
                            <option key={opt.id} value={opt.id}>{opt.label}</option>
                          ))}
                        </select>
                      </td>
                      <td>
                        <input
                          className={styles.matrixInput}
                          list={datalistId}
                          aria-label={`${spec.id} model`}
                          data-testid={`role-model-${spec.id}`}
                          value={resolved.model}
                          disabled={fieldsDisabled}
                          onChange={e =>
                            setDraft(d => applyStageFieldChange(d, spec, { model: e.target.value }))
                          }
                        />
                        <datalist id={datalistId}>
                          {modelSuggestions.map(m => (
                            <option key={m} value={m} />
                          ))}
                        </datalist>
                      </td>
                      <td>
                        <select
                          className={styles.matrixSelect}
                          style={{ minWidth: 82 }}
                          aria-label={`${spec.id} thinking`}
                          data-testid={`role-thinking-${spec.id}`}
                          value={resolved.thinking}
                          disabled={fieldsDisabled}
                          onChange={e =>
                            setDraft(d => applyStageFieldChange(d, spec, { thinking: e.target.value }))
                          }
                        >
                          {THINKING_OPTIONS.map(level => (
                            <option key={level || 'default'} value={level}>
                              {level || t('settings.thinking.default')}
                            </option>
                          ))}
                        </select>
                      </td>
                      <td>
                        {isReadOnly ? (
                          <span className={`${styles.pill} ${styles.pillSuccess}`}>{t('settings.readOnlyBadge')}</span>
                        ) : (
                          <span className={styles.pill}>{t('settings.fullAccessBadge')}</span>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : bindings.length === 0 ? (
          <div style={{ color: 'var(--fw-text-muted)', fontSize: 12 }}>—</div>
        ) : (
          <div className={styles.matrixTableWrap}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>{t('settings.roleCol')}</th>
                  <th>{t('settings.profileCol')}</th>
                  <th>{t('settings.modelCol')}</th>
                  <th>{t('settings.permCol')}</th>
                </tr>
              </thead>
              <tbody>
                {bindings.map(b => {
                  const isReadOnly = b.permissionMode === 'read-only' || READ_ONLY_ROLES.has(b.id) || READ_ONLY_ROLES.has(b.role);
                  return (
                    <tr key={b.id}>
                      <td><code>{b.id}</code></td>
                      <td>{b.profile || b.nativeProfile || '—'}</td>
                      <td>{b.model || b.channel || b.harness}</td>
                      <td>
                        {isReadOnly ? (
                          <span className={`${styles.pill} ${styles.pillSuccess}`}>{t('settings.readOnlyBadge')}</span>
                        ) : (
                          <span className={styles.pill}>{b.permissionMode ?? t('settings.fullAccessBadge')}</span>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* Tier 1: Session Reuse, Workflow Presets & Guardrails */}
      <section className={styles.card} data-testid="settings-tier1">
        <h3 className={styles.cardTitle}>{t('settings.tier1')}</h3>

        <div className={styles.settingRow}>
          <div className={styles.settingRowText}>
            <span className={styles.settingRowTitle}>{t('settings.sessionMode')}</span>
            <span className={styles.settingRowDesc}>{t('settings.sessionModeDesc')}</span>
          </div>
          <div className={styles.segmented} role="tablist" aria-label={t('settings.sessionMode')} data-testid="settings-session-mode">
            <button
              type="button"
              role="tab"
              aria-selected={sessionMode === 'scoped'}
              className={`${styles.segmentedBtn} ${sessionMode === 'scoped' ? styles.segmentedBtnActive : ''}`}
              onClick={() => setDraft(d => syncSessionMode(d, 'scoped'))}
            >
              {t('settings.session.scoped')}
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={sessionMode === 'fresh'}
              className={`${styles.segmentedBtn} ${sessionMode === 'fresh' ? styles.segmentedBtnActive : ''}`}
              onClick={() => setDraft(d => syncSessionMode(d, 'fresh'))}
            >
              {t('settings.session.fresh')}
            </button>
          </div>
        </div>

        <div className={styles.settingRow}>
          <div className={styles.settingRowText}>
            <span className={styles.settingRowTitle}>{t('settings.dualDraft')}</span>
            <span className={styles.settingRowDesc}>{t('settings.dualDraftDesc')}</span>
          </div>
          <input
            type="checkbox"
            checked={dualDraftEnabled}
            onChange={e => setDraft(d => setNested(d, ['planning', 'dual-draft', 'enabled'], e.target.checked))}
          />
        </div>

        <div className={styles.settingRow}>
          <div className={styles.settingRowText}>
            <span className={styles.settingRowTitle}>{t('settings.uxAudit')}</span>
            <span className={styles.settingRowDesc}>{t('settings.uxAuditDesc')}</span>
          </div>
          <input
            type="checkbox"
            checked={uxAuditEnabled}
            onChange={e => setDraft(d => setNested(d, ['frontend-ux-review', 'enabled'], e.target.checked))}
          />
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: 10, paddingTop: 6 }}>
          <div className={styles.field}>
            <label className={styles.fieldLabel}>{t('settings.deliveryEndpoint')}</label>
            <select
              className={styles.select}
              value={deliveryEndpoint}
              onChange={e => setDraft(d => setNested(d, ['delivery', 'endpoint'], e.target.value))}
            >
              <option value="local">local</option>
              <option value="draft-pr">draft-pr</option>
              <option value="pr-review">pr-review</option>
              <option value="auto-merge">auto-merge</option>
            </select>
          </div>
          <div className={styles.field}>
            <label className={styles.fieldLabel}>{t('settings.maxAttempts')}</label>
            <input
              type="number"
              min={1}
              max={10}
              className={styles.input}
              value={maxAttempts}
              onChange={e => setDraft(d => setNested(d, ['guardrails', 'max-packet-attempts'], Number(e.target.value)))}
            />
          </div>
          <div className={styles.field}>
            <label className={styles.fieldLabel}>{t('settings.maxReviewRounds')}</label>
            <input
              type="number"
              min={1}
              max={5}
              className={styles.input}
              value={maxReviewRounds}
              onChange={e => setDraft(d => setNested(d, ['guardrails', 'max-review-rounds'], Number(e.target.value)))}
            />
          </div>
          <div className={styles.field}>
            <label className={styles.fieldLabel}>{t('settings.maxTaskCalls')}</label>
            <input
              type="number"
              min={5}
              max={500}
              className={styles.input}
              value={maxTaskCalls}
              onChange={e => setDraft(d => setNested(d, ['guardrails', 'max-task-calls'], Number(e.target.value)))}
            />
          </div>
        </div>
      </section>

      {/* Concurrency, Timeouts & Transport Retries */}
      <section className={styles.card} data-testid="settings-runtime">
        <div>
          <h3 className={styles.cardTitle}>{t('settings.runtime.title')}</h3>
          <div style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)', marginTop: 2 }}>
            {t('settings.runtime.subtitle')}
          </div>
        </div>

        <div className={styles.settingRow} style={{ paddingTop: 4 }}>
          <div className={styles.settingRowText}>
            <span className={styles.settingRowTitle}>{t('settings.runtime.schedulerMode')}</span>
            <span className={styles.settingRowDesc}>{t('settings.runtime.schedulerModeDesc')}</span>
          </div>
          <div className={styles.segmented} role="tablist" aria-label={t('settings.runtime.schedulerMode')}>
            <button
              type="button"
              role="tab"
              aria-selected={schedulerMode === 'parallel'}
              data-testid="runtime-mode-parallel"
              className={`${styles.segmentedBtn} ${schedulerMode === 'parallel' ? styles.segmentedBtnActive : ''}`}
              onClick={() => setDraft(d => patchScheduler(d, { mode: 'parallel' }))}
            >
              {t('settings.runtime.mode.parallel')}
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={schedulerMode === 'serial'}
              data-testid="runtime-mode-serial"
              className={`${styles.segmentedBtn} ${schedulerMode === 'serial' ? styles.segmentedBtnActive : ''}`}
              onClick={() => setDraft(d => patchScheduler(d, { mode: 'serial' }))}
            >
              {t('settings.runtime.mode.serial')}
            </button>
          </div>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 10 }}>
          <div className={styles.field}>
            <label className={styles.fieldLabel} htmlFor="fw-runtime-max-parallel">
              {t('settings.runtime.maxParallel')}
            </label>
            <input
              id="fw-runtime-max-parallel"
              type="number"
              min={1}
              max={8}
              step={1}
              className={styles.input}
              data-testid="runtime-max-parallel-input"
              disabled={schedulerMode === 'serial'}
              value={runtimeInputVal('maxParallel', Number(schedulerDraft['max-parallel'] ?? 2))}
              onChange={e => handleSchedulerParallelChange(e.target.value)}
            />
            {runtimeErrors.maxParallel && (
              <span className={styles.fieldError} role="alert">{runtimeErrors.maxParallel}</span>
            )}
          </div>
          <div className={styles.field}>
            <label className={styles.fieldLabel} htmlFor="fw-runtime-dispatch">
              {t('settings.runtime.dispatch')}
            </label>
            <select
              id="fw-runtime-dispatch"
              className={styles.select}
              data-testid="runtime-dispatch-select"
              value={schedulerDispatch}
              onChange={e => setDraft(d => patchScheduler(d, { dispatch: e.target.value }))}
            >
              <option value="batch">{t('settings.runtime.dispatch.batch')}</option>
              <option value="ready-queue">{t('settings.runtime.dispatch.readyQueue')}</option>
            </select>
          </div>
        </div>

        <div className={styles.settingRow}>
          <div className={styles.settingRowText}>
            <span className={styles.settingRowTitle}>{t('settings.runtime.worktree')}</span>
            <span className={styles.settingRowDesc}>{t('settings.runtime.worktreeDesc')}</span>
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={worktreeEnabled}
            aria-label={t('settings.runtime.worktree')}
            data-testid="runtime-worktree-toggle"
            className={`${styles.switchBtn} ${worktreeEnabled ? styles.switchBtnOn : ''}`}
            onClick={() => setDraft(d => setNested(d, ['worktree', 'enabled'], !worktreeEnabled))}
          >
            <span className={styles.switchThumb} />
          </button>
        </div>

        <div className={styles.settingRow}>
          <div className={styles.settingRowText}>
            <span className={styles.settingRowTitle}>{t('settings.runtime.prewarm')}</span>
            <span className={styles.settingRowDesc}>{t('settings.runtime.prewarmDesc')}</span>
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={schedulerPrewarm}
            aria-label={t('settings.runtime.prewarm')}
            data-testid="runtime-prewarm-toggle"
            className={`${styles.switchBtn} ${schedulerPrewarm ? styles.switchBtnOn : ''}`}
            onClick={() => setDraft(d => patchScheduler(d, { prewarm: !schedulerPrewarm }))}
          >
            <span className={styles.switchThumb} />
          </button>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4 }}>
          <span className={styles.settingRowTitle} style={{ fontSize: 12.5 }}>
            {t('settings.runtime.watchdogTitle')}
          </span>
          <div className={styles.jevGrid}>
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-wd-idle">
                {t('settings.runtime.idleTimeoutSec')}
              </label>
              <input
                id="fw-wd-idle"
                type="number"
                min={1}
                max={86400}
                step={30}
                className={styles.input}
                data-testid="runtime-idle-timeout-input"
                value={runtimeInputVal('idleTimeout', idleTimeoutSecNum)}
                onChange={e => handleRuntimeSecChange('idleTimeout', e.target.value)}
              />
              {!runtimeErrors.idleTimeout && (
                <span style={{ fontSize: 11, color: 'var(--fw-text-subtle)' }}>
                  {formatSecondsHint(idleTimeoutSecNum, isZh)}
                </span>
              )}
              {runtimeErrors.idleTimeout && (
                <span className={styles.fieldError} role="alert">{runtimeErrors.idleTimeout}</span>
              )}
            </div>
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-wd-plan-idle">
                {t('settings.runtime.planIdleTimeoutSec')}
              </label>
              <input
                id="fw-wd-plan-idle"
                type="number"
                min={1}
                max={86400}
                step={60}
                className={styles.input}
                data-testid="runtime-plan-idle-timeout-input"
                value={runtimeInputVal('planIdleTimeout', planIdleTimeoutSecNum)}
                onChange={e => handleRuntimeSecChange('planIdleTimeout', e.target.value)}
              />
              {!runtimeErrors.planIdleTimeout && (
                <span style={{ fontSize: 11, color: 'var(--fw-text-subtle)' }}>
                  {formatSecondsHint(planIdleTimeoutSecNum, isZh)}
                </span>
              )}
              {runtimeErrors.planIdleTimeout && (
                <span className={styles.fieldError} role="alert">{runtimeErrors.planIdleTimeout}</span>
              )}
            </div>
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-wd-wall">
                {t('settings.runtime.wallTimeoutSec')}
              </label>
              <input
                id="fw-wd-wall"
                type="number"
                min={1}
                max={86400}
                step={60}
                className={styles.input}
                data-testid="runtime-wall-timeout-input"
                value={runtimeInputVal('wallTimeout', wallTimeoutSecNum)}
                onChange={e => handleRuntimeSecChange('wallTimeout', e.target.value)}
              />
              {!runtimeErrors.wallTimeout && (
                <span style={{ fontSize: 11, color: 'var(--fw-text-subtle)' }}>
                  {formatSecondsHint(wallTimeoutSecNum, isZh)}
                </span>
              )}
              {runtimeErrors.wallTimeout && (
                <span className={styles.fieldError} role="alert">{runtimeErrors.wallTimeout}</span>
              )}
            </div>
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-wd-grace">
                {t('settings.runtime.killGraceSec')}
              </label>
              <input
                id="fw-wd-grace"
                type="number"
                min={1}
                max={60}
                step={1}
                className={styles.input}
                data-testid="runtime-kill-grace-input"
                value={runtimeInputVal('killGrace', killGraceSecNum)}
                onChange={e => handleRuntimeSecChange('killGrace', e.target.value)}
              />
              {runtimeErrors.killGrace && (
                <span className={styles.fieldError} role="alert">{runtimeErrors.killGrace}</span>
              )}
            </div>
          </div>
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <button
            type="button"
            className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
            data-testid="runtime-retry-toggle"
            onClick={() => setRuntimeRetryOpen(v => !v)}
          >
            {runtimeRetryOpen ? t('settings.runtime.retryHide') : t('settings.runtime.retryShow')}
            {' '}
            {runtimeRetryOpen ? '▾' : '▸'}
          </button>
        </div>

        {runtimeRetryOpen && (
          <div className={styles.jevGrid} data-testid="runtime-retry-panel">
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-retry-max">
                {t('settings.runtime.maxTransportRetries')}
              </label>
              <input
                id="fw-retry-max"
                type="number"
                min={0}
                max={20}
                step={1}
                className={styles.input}
                data-testid="runtime-retry-max-input"
                value={runtimeInputVal('max-transport-retries', Number(retryDraft['max-transport-retries'] ?? 5))}
                onChange={e => handleRetryNumChange('max-transport-retries', e.target.value)}
              />
              {runtimeErrors.maxRetries && (
                <span className={styles.fieldError} role="alert">{runtimeErrors.maxRetries}</span>
              )}
            </div>
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-retry-base">
                {t('settings.runtime.baseDelayMs')}
              </label>
              <input
                id="fw-retry-base"
                type="number"
                min={1}
                max={60000}
                step={100}
                className={styles.input}
                data-testid="runtime-retry-base-input"
                value={runtimeInputVal('base-delay-ms', Number(retryDraft['base-delay-ms'] ?? 500))}
                onChange={e => handleRetryNumChange('base-delay-ms', e.target.value)}
              />
              {runtimeErrors.baseDelay && (
                <span className={styles.fieldError} role="alert">{runtimeErrors.baseDelay}</span>
              )}
            </div>
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-retry-max-delay">
                {t('settings.runtime.maxDelayMs')}
              </label>
              <input
                id="fw-retry-max-delay"
                type="number"
                min={1}
                max={300000}
                step={500}
                className={styles.input}
                data-testid="runtime-retry-max-delay-input"
                value={runtimeInputVal('max-delay-ms', Number(retryDraft['max-delay-ms'] ?? 10_000))}
                onChange={e => handleRetryNumChange('max-delay-ms', e.target.value)}
              />
              {runtimeErrors.maxDelay && (
                <span className={styles.fieldError} role="alert">{runtimeErrors.maxDelay}</span>
              )}
            </div>
            <div className={styles.field}>
              <label className={styles.fieldLabel} htmlFor="fw-retry-jitter">
                {t('settings.runtime.jitter')}
              </label>
              <input
                id="fw-retry-jitter"
                type="number"
                min={0}
                max={1}
                step={0.05}
                className={styles.input}
                data-testid="runtime-retry-jitter-input"
                value={runtimeInputVal('jitter', Number(retryDraft.jitter ?? 0.1))}
                onChange={e => handleRetryNumChange('jitter', e.target.value)}
              />
              {runtimeErrors.jitter && (
                <span className={styles.fieldError} role="alert">{runtimeErrors.jitter}</span>
              )}
            </div>
          </div>
        )}
      </section>

      {/* Tier 3: Collapsible Advanced & Raw JSON */}
      <section className={styles.card} data-testid="settings-tier3">
        <div className={styles.cardHeader}>
          <h3 className={styles.cardTitle}>{t('settings.tier3')}</h3>
          <button type="button" className={`${styles.btn} ${styles.btnSm}`} onClick={() => setTier3Open(v => !v)}>
            {tier3Open ? '▾' : '▸'}
          </button>
        </div>

        {tier3Open && (
          <>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={budgetEnforce !== 'off'}
                onChange={e => setDraft(d => setNested(d, ['budget', 'enforce'], e.target.checked ? 'hard' : 'off'))}
              />
              <span>{t('settings.budgetOptIn')}</span>
            </label>
            <pre className={styles.monoPre}>{JSON.stringify(draft, null, 2)}</pre>
          </>
        )}
      </section>

      {/* Sticky Save & Reset Bar */}
      <div className={styles.settingsFooter}>
        <span
          style={{
            fontSize: 12,
            color: hasValidationErrors ? 'var(--fw-danger)' : dirty ? 'var(--fw-warn)' : 'var(--fw-text-subtle)',
          }}
        >
          {hasJevErrors
            ? t('settings.jev.validationBlocked')
            : hasRuntimeErrors
              ? t('settings.runtime.validationBlocked')
              : dirty
                ? t('settings.unsaved')
                : t('settings.synced')}
        </span>
        <div style={{ display: 'flex', gap: 8 }}>
          <button
            type="button"
            className={styles.btn}
            disabled={snap.busyAction !== null || !snap.configEditor}
            onClick={() => void store.resetConfig()}
          >
            {t('settings.reset')}
          </button>
          <button
            type="button"
            className={`${styles.btn} ${styles.btnPrimary}`}
            disabled={snap.busyAction !== null || !snap.configEditor || hasValidationErrors}
            onClick={() => void store.saveConfig(draft)}
          >
            {snap.busyAction === 'config-save' ? t('settings.saving') : t('settings.save')}
          </button>
        </div>
      </div>
    </StyleScope>
  );
}
