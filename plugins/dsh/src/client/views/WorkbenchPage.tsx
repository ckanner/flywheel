import { useSyncExternalStore } from 'react';
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

function runStatusPill(status?: string, running?: boolean): string {
  if (running) return styles.pillRunning;
  const s = (status ?? '').toLowerCase();
  if (['completed', 'done', 'passed'].includes(s)) return styles.pillSuccess;
  if (['failed', 'blocked', 'escalated'].includes(s)) return styles.pillDanger;
  if (['awaiting_approval', 'stopped'].includes(s)) return styles.pillWarn;
  return '';
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

export function WorkbenchPage({
  store,
  t,
}: {
  store: FlywheelStore;
  t: Translate;
  runtime?: ClientRuntime;
}) {
  const snap = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);

  const planning = snap.planning ?? snap.state?.planning ?? null;
  const design = planning?.design;
  const needsApproval = Boolean(design?.revision && design?.digest && !design?.approved);
  const hasInvestigation = Boolean(snap.state?.investigation);
  const activeRun = snap.runs.find(r => r.taskDir === snap.selectedTaskDir) ?? null;

  const title =
    snap.state?.title ||
    snap.state?.slug ||
    activeRun?.title ||
    activeRun?.slug ||
    t('header.noTask');

  // Single dynamic Primary CTA driven by task lifecycle
  const renderPrimaryCta = () => {
    if (!snap.selectedTaskDir) {
      return (
        <button
          type="button"
          className={`${styles.btn} ${styles.btnPrimary}`}
          onClick={() => store.setCreateModalOpen(true)}
        >
          {t('header.cta.create')}
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

  const tabs: Array<{ id: MainTab; label: string }> = [
    { id: 'design', label: t('tab.design') },
    { id: 'execution', label: t('tab.execution') },
    { id: 'investigation', label: t('tab.investigation') },
    { id: 'delivery', label: t('tab.delivery') },
  ];

  return (
    <StyleScope className={styles.workbenchShell}>
      {/* Collapsible 240px Task Rail */}
      <aside className={`${styles.rail} ${snap.railCollapsed ? styles.railCollapsed : ''}`} data-testid="flywheel-task-rail">
        <div className={styles.railHeader}>
          {!snap.railCollapsed && <span className={styles.railTitle}>{t('rail.tasks')}</span>}
          <button
            type="button"
            className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
            title={snap.railCollapsed ? t('rail.expand') : t('rail.collapse')}
            onClick={() => store.setRailCollapsed(!snap.railCollapsed)}
          >
            {snap.railCollapsed ? '»' : '«'}
          </button>
        </div>

        {!snap.railCollapsed && (
          <>
            <div className={styles.railActions}>
              <button
                type="button"
                className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                style={{ flex: 1 }}
                onClick={() => store.setCreateModalOpen(true)}
              >
                + {t('rail.newTask')}
              </button>
              <button
                type="button"
                className={`${styles.btn} ${styles.btnSm}`}
                onClick={() => void store.refreshRuns()}
                title={t('rail.refresh')}
              >
                ↻
              </button>
            </div>

            <div className={styles.railList}>
              {snap.runs.length === 0 ? (
                <div style={{ padding: 12, fontSize: 12, color: 'var(--fw-text-subtle)' }}>
                  {t('rail.empty')}
                </div>
              ) : (
                snap.runs.map(run => {
                  const active = run.taskDir === snap.selectedTaskDir;
                  return (
                    <button
                      key={run.taskDir}
                      type="button"
                      className={`${styles.railItem} ${active ? styles.railItemActive : ''}`}
                      onClick={() => void store.selectTask(run.taskDir, run.cwd)}
                    >
                      <div className={styles.railItemRow}>
                        <span className={styles.railItemTitle}>{run.title || run.slug || run.taskDir}</span>
                        <span className={`${styles.pill} ${runStatusPill(run.status, run.running)}`}>
                          {run.running ? t('header.running') : (run.status ?? 'ready')}
                        </span>
                      </div>
                      <span className={styles.railItemMeta}>{run.taskDir}</span>
                    </button>
                  );
                })
              )}
            </div>
          </>
        )}
      </aside>

      {/* Main Column */}
      <div className={styles.mainColumn}>
        {/* 52px Single-Row Header */}
        <header className={styles.header} data-testid="flywheel-header">
          <div className={styles.headerLeft}>
            <h2 className={styles.headerTitle}>{title}</h2>
            {(snap.state?.mode || activeRun?.mode) && (
              <span className={`${styles.pill} ${styles.pillPurple}`}>
                {snap.state?.mode ?? activeRun?.mode}
              </span>
            )}
            <span className={`${styles.pill} ${runStatusPill(snap.state?.status, snap.running)}`}>
              {snap.running ? t('header.running') : (snap.state?.status ?? t('header.idle'))}
            </span>
            {snap.selectedCwd && <span className={styles.headerCrumb}>{snap.selectedCwd}</span>}
          </div>
          <div className={styles.headerRight}>{renderPrimaryCta()}</div>
        </header>

        {/* Lifecycle Tabs */}
        <nav className={styles.tabBar} role="tablist" aria-label={t('panel.title')}>
          {tabs.map(tab => (
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
        </nav>

        {/* Workspace Body */}
        <main className={styles.workspaceBody}>
          {snap.error && (
            <div className={styles.bannerError} role="alert" data-testid="flywheel-error-banner">
              <span>{snap.error.message}</span>
              <button type="button" className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`} onClick={() => store.clearError()}>
                ×
              </button>
            </div>
          )}

          {snap.runsError && (
            <div className={styles.bannerError} role="alert">
              <span>{snap.runsError}</span>
            </div>
          )}

          {!snap.selectedTaskDir ? (
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
          ) : (
            <>
              {snap.activeTab === 'design' && <DesignStudio snap={snap} store={store} t={t} />}
              {snap.activeTab === 'execution' && <ExecutionBoard snap={snap} store={store} t={t} />}
              {snap.activeTab === 'investigation' && <InvestigationBoard snap={snap} store={store} t={t} />}
              {snap.activeTab === 'delivery' && <DeliveryTrack snap={snap} store={store} t={t} />}
            </>
          )}
        </main>
      </div>

      <CreateTaskModal snap={snap} store={store} t={t} />
    </StyleScope>
  );
}
