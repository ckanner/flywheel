import { useEffect, useMemo, useState, useSyncExternalStore } from 'react';
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
  const [tier3Open, setTier3Open] = useState(false);
  const [catalogLoading, setCatalogLoading] = useState(false);

  const isZh = t('settings.scope.global') === '全局配置';

  useEffect(() => {
    void store.loadConfig(snap.configScope);
  }, [store, snap.configScope]);

  useEffect(() => {
    if (snap.configEditor?.values) {
      setDraft(cloneRecord(snap.configEditor.values));
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
        <span style={{ fontSize: 12, color: dirty ? 'var(--fw-warn)' : 'var(--fw-text-subtle)' }}>
          {dirty ? t('settings.unsaved') : t('settings.synced')}
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
            disabled={snap.busyAction !== null || !snap.configEditor}
            onClick={() => void store.saveConfig(draft)}
          >
            {snap.busyAction === 'config-save' ? t('settings.saving') : t('settings.save')}
          </button>
        </div>
      </div>
    </StyleScope>
  );
}
