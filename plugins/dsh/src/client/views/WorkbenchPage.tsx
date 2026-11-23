import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { StyleScope } from '../components/StyleScope.tsx';
import type { ClientRuntime } from '../context.ts';
import type { FlywheelStore } from '../store.ts';
import type { MainTab, Translate } from '../types.ts';
import { CreateTaskModal } from './CreateTaskModal.tsx';
import { DeliveryTrack } from './DeliveryTrack.tsx';
import { DesignStudio } from './DesignStudio.tsx';
import { ExecutionBoard } from './ExecutionBoard.tsx';
import { InvestigationBoard } from './InvestigationBoard.tsx';
import { SettingsPage } from './SettingsPage.tsx';

function runStatusPill(status?: string, running?: boolean): string {
  if (running) return styles.pillRunning;
  const s = (status ?? '').toLowerCase();
  if (['completed', 'done', 'passed'].includes(s)) return styles.pillSuccess;
  if (['failed', 'blocked', 'escalated'].includes(s)) return styles.pillDanger;
  if (['awaiting_approval', 'stopped', 'paused'].includes(s)) return styles.pillWarn;
  return '';
}

function statusDotClass(status?: string, running?: boolean): string {
  if (running) return styles.statusDotRunning;
  const s = (status ?? '').toLowerCase();
  if (['completed', 'done', 'passed'].includes(s)) return styles.statusDotSuccess;
  if (['failed', 'blocked', 'escalated'].includes(s)) return styles.statusDotDanger;
  if (['awaiting_approval', 'stopped', 'paused'].includes(s)) return styles.statusDotWarn;
  return '';
}

function localizedStatus(t: Translate, status?: string, running?: boolean): string {
  if (running) return t('header.running');
  const key = (status ?? '').toLowerCase();
  if (!key) return t('header.idle');
  const lookup = `status.${key}`;
  const translated = t(lookup);
  return translated === lookup ? key : translated;
}

function formatShortTime(iso?: string): string {
  if (!iso) return '';
  const ts = Date.parse(iso);
  if (!Number.isFinite(ts)) return '';
  const d = new Date(ts);
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  const hh = String(d.getHours()).padStart(2, '0');
  const mi = String(d.getMinutes()).padStart(2, '0');
  return `${mm}-${dd} ${hh}:${mi}`;
}

export function FlywheelRailIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <circle cx="12" cy="12" r="9" />
      <circle cx="12" cy="12" r="3" />
      <path d="M12 3v6M19.8 16.5l-5.2-3M4.2 16.5l5.2-3" />
    </svg>
  );
}

function SettingsIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9c.26.604.852.997 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z" />
    </svg>
  );
}

export function WorkbenchPage({
  store,
  t,
}: {
  store: FlywheelStore;
  t: Translate;
  runtime?: ClientRuntime;
}) {
  const snap = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  const [switcherOpen, setSwitcherOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const switcherRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!switcherOpen) return;
    const handleOutside = (event: MouseEvent) => {
      if (switcherRef.current && !switcherRef.current.contains(event.target as Node)) {
        setSwitcherOpen(false);
      }
    };
    document.addEventListener('mousedown', handleOutside);
    return () => document.removeEventListener('mousedown', handleOutside);
  }, [switcherOpen]);

  const planning = snap.planning ?? snap.state?.planning ?? null;
  const design = planning?.design;
  const needsApproval = Boolean(design?.revision && design?.digest && !design?.approved);
  const hasInvestigation = Boolean(snap.state?.investigation);
  const activeRun = snap.runs.find(r => r.taskDir === snap.selectedTaskDir) ?? null;

  const tempCount = useMemo(() => snap.runs.filter(r => r.isTemp).length, [snap.runs]);
  const visibleRuns = useMemo(() => {
    const base = snap.showTempRuns ? snap.runs : snap.runs.filter(r => !r.isTemp || r.taskDir === snap.selectedTaskDir);
    const q = searchQuery.trim().toLowerCase();
    if (!q) return base;
    return base.filter(r =>
      (r.title || '').toLowerCase().includes(q) ||
      (r.slug || '').toLowerCase().includes(q) ||
      (r.repoName || '').toLowerCase().includes(q) ||
      r.taskDir.toLowerCase().includes(q),
    );
  }, [snap.runs, snap.showTempRuns, snap.selectedTaskDir, searchQuery]);

  const title =
    snap.state?.title ||
    snap.state?.slug ||
    activeRun?.title ||
    activeRun?.slug ||
    t('header.noTask');

  const repoLabel = activeRun?.repoName || (snap.selectedCwd ? snap.selectedCwd.replace(/\/+$/, '').split('/').pop() : '');
  const currentStatus = snap.state?.status ?? activeRun?.status;

  // Single dynamic Primary CTA driven by task lifecycle
  const renderPrimaryCta = () => {
    if (snap.activeTab === 'settings') {
      return null;
    }
    if (!snap.selectedTaskDir) {
      return (
        <button
          type="button"
          className={`${styles.btn} ${styles.btnPrimary}`}
          onClick={() => store.setCreateModalOpen(true)}
        >
          + {t('rail.newTask')}
        </button>
      );
    }
    if (snap.running) {
      return (
        <button
          type="button"
          className={`${styles.btn} ${styles.btnDanger}`}
          disabled={snap.busyAction !== null}
          onClick={() => void store.stopRun()}
        >
          {t('header.cta.stop')}
        </button>
      );
    }
    if (needsApproval) {
      return (
        <button
          type="button"
          className={`${styles.btn} ${styles.btnPrimary}`}
          disabled={snap.busyAction !== null}
          onClick={() => void store.approveDesign(true)}
        >
          {t('header.cta.approve')}
        </button>
      );
    }
    if (snap.activeTab === 'investigation' && hasInvestigation) {
      return (
        <button
          type="button"
          className={`${styles.btn} ${styles.btnPrimary}`}
          disabled={snap.busyAction !== null}
          onClick={() => void store.runInvestigation()}
        >
          {t('header.cta.runInvestigation')}
        </button>
      );
    }
    if (snap.activeTab === 'delivery' || snap.state?.status === 'completed' || snap.state?.status === 'done') {
      return (
        <button
          type="button"
          className={`${styles.btn} ${styles.btnPrimary}`}
          onClick={() => store.setActiveTab('delivery')}
        >
          {t('header.cta.deliver')}
        </button>
      );
    }
    return (
      <button
        type="button"
        className={`${styles.btn} ${styles.btnPrimary}`}
        disabled={snap.busyAction !== null}
        onClick={() => void store.startRun('auto')}
      >
        {t('header.cta.start')}
      </button>
    );
  };

  const lifecycleTabs: Array<{ id: MainTab; label: string }> = [
    { id: 'design', label: t('tab.design') },
    { id: 'execution', label: t('tab.execution') },
    { id: 'investigation', label: t('tab.investigation') },
    { id: 'delivery', label: t('tab.delivery') },
  ];

  return (
    <StyleScope className={styles.workbenchShell}>
      <div className={styles.mainColumn}>
        {/* 54px Single-Row Header: Task Switcher + Status + New Task + Primary CTA */}
        <header className={styles.header} data-testid="flywheel-header">
          <div className={styles.headerLeft} ref={switcherRef}>
            <div className={styles.taskSwitcherWrap}>
              <button
                type="button"
                className={styles.taskSwitcherBtn}
                data-testid="flywheel-task-switcher"
                aria-expanded={switcherOpen}
                onClick={() => setSwitcherOpen(v => !v)}
              >
                <span className={`${styles.statusDot} ${statusDotClass(currentStatus, snap.running)}`} />
                <span className={styles.taskSwitcherLabel}>{title}</span>
                {repoLabel && <span className={styles.taskSwitcherRepo}>· {repoLabel}</span>}
                <span className={styles.taskSwitcherCaret}>{switcherOpen ? '▴' : '▾'}</span>
              </button>

              {switcherOpen && (
                <div className={styles.taskSwitcherPopover} data-testid="flywheel-task-popover" role="dialog" aria-label={t('rail.tasks')}>
                  <div className={styles.taskSwitcherHead}>
                    <input
                      type="text"
                      className={styles.taskSwitcherSearch}
                      value={searchQuery}
                      onChange={e => setSearchQuery(e.target.value)}
                      placeholder={t('switcher.placeholder')}
                    />
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnSm}`}
                      onClick={() => void store.refreshRuns()}
                      title={t('rail.refresh')}
                    >
                      ↻
                    </button>
                  </div>

                  <div className={styles.taskSwitcherList}>
                    {visibleRuns.length === 0 ? (
                      <div style={{ padding: '16px 12px', textAlign: 'center', fontSize: 12, color: 'var(--fw-text-subtle)' }}>
                        {snap.runs.length === 0 ? t('rail.empty') : t('switcher.noMatch')}
                      </div>
                    ) : (
                      visibleRuns.map(run => {
                        const active = run.taskDir === snap.selectedTaskDir;
                        return (
                          <button
                            key={run.taskDir}
                            type="button"
                            className={`${styles.taskSwitcherItem} ${active ? styles.taskSwitcherItemActive : ''}`}
                            onClick={() => {
                              setSwitcherOpen(false);
                              if (snap.activeTab === 'settings') store.setActiveTab('execution');
                              void store.selectTask(run.taskDir, run.cwd);
                            }}
                          >
                            <div className={styles.taskSwitcherItemTop}>
                              <span className={styles.taskSwitcherItemName}>
                                <span className={`${styles.statusDot} ${statusDotClass(run.status, run.running)}`} />
                                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                                  {run.title || run.slug}
                                </span>
                              </span>
                              <span className={`${styles.pill} ${runStatusPill(run.status, run.running)}`}>
                                {localizedStatus(t, run.status, run.running)}
                              </span>
                            </div>
                            <div className={styles.taskSwitcherItemMeta}>
                              <span>
                                {run.repoName || run.cwd || run.taskDir}
                                {(run.runCount ?? 1) > 1 ? ` · ${t('switcher.runsCount', { count: run.runCount ?? 1 })}` : ''}
                              </span>
                              <span>{formatShortTime(run.updatedAt)}</span>
                            </div>
                          </button>
                        );
                      })
                    )}
                  </div>

                  <div className={styles.taskSwitcherFooter}>
                    {tempCount > 0 ? (
                      <label style={{ display: 'inline-flex', alignItems: 'center', gap: 6, cursor: 'pointer' }}>
                        <input
                          type="checkbox"
                          checked={snap.showTempRuns}
                          onChange={e => store.setShowTempRuns(e.target.checked)}
                        />
                        <span>{t('switcher.showTemp', { count: tempCount })}</span>
                      </label>
                    ) : (
                      <span>{t('switcher.count', { count: visibleRuns.length })}</span>
                    )}
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnSm}`}
                      onClick={() => {
                        setSwitcherOpen(false);
                        store.setCreateModalOpen(true);
                      }}
                    >
                      + {t('rail.newTask')}
                    </button>
                  </div>
                </div>
              )}
            </div>

            {(snap.state?.mode || activeRun?.mode) && (
              <span className={`${styles.pill} ${styles.pillPurple}`}>
                {snap.state?.mode ?? activeRun?.mode}
              </span>
            )}
            {snap.selectedTaskDir && (
              <span className={`${styles.pill} ${runStatusPill(currentStatus, snap.running)}`}>
                {localizedStatus(t, currentStatus, snap.running)}
              </span>
            )}
          </div>

          <div className={styles.headerRight}>
            {snap.selectedTaskDir && (
              <button
                type="button"
                className={styles.btn}
                onClick={() => store.setCreateModalOpen(true)}
              >
                + {t('rail.newTask')}
              </button>
            )}
            {renderPrimaryCta()}
          </div>
        </header>

        {/* Lifecycle Tab Bar + Direct Inline Settings Tab */}
        <nav className={styles.tabBar} role="tablist" aria-label={t('panel.title')}>
          {lifecycleTabs.map(tab => (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={snap.activeTab === tab.id}
              className={`${styles.tabBtn} ${snap.activeTab === tab.id ? styles.tabBtnActive : ''}`}
              onClick={() => store.setActiveTab(tab.id)}
            >
              {tab.label}
            </button>
          ))}
          <span className={styles.tabBarSpacer} />
          <button
            type="button"
            role="tab"
            aria-selected={snap.activeTab === 'settings'}
            data-testid="flywheel-settings-tab"
            className={`${styles.tabBtn} ${snap.activeTab === 'settings' ? styles.tabBtnActive : ''}`}
            onClick={() => store.setActiveTab('settings')}
          >
            <SettingsIcon />
            <span>{t('tab.settings')}</span>
          </button>
        </nav>

        {/* Full-Bleed Workspace Body */}
        <main className={styles.workspaceBody}>
          {snap.error && snap.activeTab !== 'settings' && (
            <div className={styles.bannerError} role="alert" data-testid="flywheel-error-banner">
              <span>{snap.error.message}</span>
              <button type="button" className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`} onClick={() => store.clearError()}>
                ×
              </button>
            </div>
          )}

          {snap.runsError && snap.activeTab !== 'settings' && (
            <div className={styles.bannerError} role="alert">
              <span>{snap.runsError}</span>
            </div>
          )}

          {snap.activeTab === 'settings' ? (
            <SettingsPage store={store} t={t} inline />
          ) : !snap.selectedTaskDir ? (
            <div className={styles.emptyState}>
              <div>{t('rail.empty')}</div>
              <button
                type="button"
                className={`${styles.btn} ${styles.btnPrimary}`}
                onClick={() => store.setCreateModalOpen(true)}
              >
                + {t('rail.newTask')}
              </button>
            </div>
          ) : snap.activeTab === 'design' ? (
            <DesignStudio snap={snap} store={store} t={t} />
          ) : snap.activeTab === 'execution' ? (
            <ExecutionBoard snap={snap} store={store} t={t} />
          ) : snap.activeTab === 'investigation' ? (
            <InvestigationBoard snap={snap} store={store} t={t} />
          ) : (
            <DeliveryTrack snap={snap} store={store} t={t} />
          )}
        </main>
      </div>

      <CreateTaskModal snap={snap} store={store} t={t} />
    </StyleScope>
  );
}
