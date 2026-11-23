import { useEffect, useState, useSyncExternalStore } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { StyleScope } from '../components/StyleScope.tsx';
import type { FlywheelStore } from '../store.ts';
import type { Translate } from '../types.ts';

function cloneRecord(value: Record<string, unknown> | undefined): Record<string, unknown> {
  return JSON.parse(JSON.stringify(value ?? {})) as Record<string, unknown>;
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

const SUBSCRIPTIONS = [
  { id: 'antigravity', name: 'Antigravity (ACP)', channel: 'Gemini 4 Carbon / Neon' },
  { id: 'claude-code-pro', name: 'Claude Code Pro', channel: 'Opus 5.5 / Sonnet 5.1' },
  { id: 'codex-pro', name: 'Codex Pro', channel: 'GPT-6.2 Astra / Sol' },
  { id: 'kimi', name: 'Kimi Native', channel: 'K2.6 (WebSearch / Swarm)' },
] as const;

const READ_ONLY_ROLES = new Set([
  'loops.design-review',
  'loops.plan-review',
  'loops.review',
  'loops.judge',
  'loops.investigate',
  'frontend-ux-review',
]);

export function SettingsPage({
  store,
  t,
}: {
  store: FlywheelStore;
  t: Translate;
}) {
  const snap = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  const [draft, setDraft] = useState<Record<string, unknown>>({});
  const [tier3Open, setTier3Open] = useState(false);

  useEffect(() => {
    void store.loadConfig(snap.configScope);
  }, [store, snap.configScope]);

  useEffect(() => {
    if (snap.configEditor?.values) {
      setDraft(cloneRecord(snap.configEditor.values));
    }
  }, [snap.configEditor]);

  const dualDraftEnabled = Boolean(getNested(draft, ['planning', 'dual-draft', 'enabled']) ?? true);
  const uxAuditEnabled = Boolean(getNested(draft, ['frontend-ux-review', 'enabled']) ?? true);
  const deliveryEndpoint = String(getNested(draft, ['delivery', 'endpoint']) ?? 'local');
  const maxAttempts = Number(getNested(draft, ['guardrails', 'max-packet-attempts']) ?? 3);
  const maxReviewRounds = Number(getNested(draft, ['guardrails', 'max-review-rounds']) ?? 2);
  const maxTaskCalls = Number(getNested(draft, ['guardrails', 'max-task-calls']) ?? 60);
  const budgetEnforce = String(getNested(draft, ['budget', 'enforce']) ?? 'off');

  const bindings = snap.effectiveConfig?.bindings ?? [];
  const checks = snap.doctor?.checks ?? [];

  const checkStatusFor = (subId: string): { label: string; cls: string } => {
    const match = checks.find(c => c.id.toLowerCase().includes(subId.replace('-pro', '')));
    if (!match) return { label: '—', cls: '' };
    if (match.status === 'pass') return { label: 'Ready', cls: styles.pillSuccess };
    if (match.status === 'warn') return { label: 'Warn', cls: styles.pillWarn };
    if (match.status === 'fail') return { label: 'Issue', cls: styles.pillDanger };
    return { label: match.status, cls: '' };
  };

  return (
    <StyleScope className={styles.settingsPage}>
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

      {snap.notice === 'saved' && (
        <div className={styles.bannerSuccess}>
          <span>{t('settings.saved')}</span>
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
          <button
            type="button"
            className={styles.btn}
            disabled={snap.busyAction !== null}
            onClick={() => void store.loadDoctor(false)}
          >
            {t('settings.preflight')}
          </button>
        </div>

        <div className={styles.healthGrid}>
          {SUBSCRIPTIONS.map(sub => {
            const st = checkStatusFor(sub.id);
            return (
              <div key={sub.id} className={styles.healthCard}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 6 }}>
                  <strong style={{ fontSize: 12 }}>{sub.name}</strong>
                  <span className={`${styles.pill} ${st.cls}`}>{st.label}</span>
                </div>
                <span style={{ fontSize: 11, color: 'var(--fw-text-muted)' }}>{sub.channel}</span>
              </div>
            );
          })}
        </div>
      </section>

      {/* Tier 1: Workflow Presets & Guardrails */}
      <section className={styles.card} data-testid="settings-tier1">
        <h3 className={styles.cardTitle}>{t('settings.tier1')}</h3>

        <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
          <input
            type="checkbox"
            checked={dualDraftEnabled}
            onChange={e => setDraft(d => setNested(d, ['planning', 'dual-draft', 'enabled'], e.target.checked))}
          />
          <span>{t('settings.dualDraft')}</span>
        </label>

        <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
          <input
            type="checkbox"
            checked={uxAuditEnabled}
            onChange={e => setDraft(d => setNested(d, ['frontend-ux-review', 'enabled'], e.target.checked))}
          />
          <span>{t('settings.uxAudit')}</span>
        </label>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: 10 }}>
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

      {/* Tier 2: Grouped Role Table with Read-Only Lock Badges */}
      <section className={styles.card} data-testid="settings-tier2">
        <h3 className={styles.cardTitle}>{t('settings.tier2')}</h3>
        {bindings.length === 0 ? (
          <div style={{ color: 'var(--fw-text-muted)', fontSize: 12 }}>—</div>
        ) : (
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
                        <span className={styles.pill}>{b.permissionMode ?? 'workspace-write'}</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
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

      <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <button
          type="button"
          className={`${styles.btn} ${styles.btnPrimary}`}
          disabled={snap.busyAction !== null || !snap.configEditor}
          onClick={() => void store.saveConfig(draft)}
        >
          {snap.busyAction === 'config-save' ? t('settings.saving') : t('settings.save')}
        </button>
      </div>
    </StyleScope>
  );
}
