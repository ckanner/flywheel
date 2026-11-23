import { useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import type { FlywheelSnapshot, FlywheelStore } from '../store.ts';
import type { Translate } from '../types.ts';

function hypothesisPillClass(status?: string): string {
  const s = (status ?? '').toLowerCase();
  if (s === 'confirmed') return styles.pillSuccess;
  if (s === 'refuted') return styles.pillDanger;
  if (s === 'inconclusive') return styles.pillWarn;
  return styles.pillPurple;
}

export function InvestigationBoard({
  snap,
  store,
  t,
}: {
  snap: FlywheelSnapshot;
  store: FlywheelStore;
  t: Translate;
}) {
  const [followUpGoal, setFollowUpGoal] = useState('');
  const [followUpPaths, setFollowUpPaths] = useState('');

  const inv = snap.state?.investigation;
  if (!inv) {
    return (
      <div className={styles.emptyState}>
        <div>{t('inv.empty')}</div>
      </div>
    );
  }

  const hypotheses = inv.hypotheses ?? [];
  const actions = inv.actions ?? [];

  const handleContinue = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!followUpGoal.trim()) return;
    const paths = followUpPaths
      .split(',')
      .map(s => s.trim())
      .filter(Boolean);
    await store.continueInvestigation(followUpGoal, paths);
    setFollowUpGoal('');
    setFollowUpPaths('');
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }} data-testid="flywheel-investigation-board">
      <div className={styles.card}>
        <div className={styles.cardHeader}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <h3 className={styles.cardTitle}>{t('inv.title')}</h3>
            <span className={styles.pill}>{inv.status ?? 'ready'}</span>
          </div>
          <div style={{ display: 'flex', gap: 8 }}>
            <button
              type="button"
              className={`${styles.btn} ${styles.btnPrimary}`}
              disabled={snap.busyAction !== null || snap.running}
              onClick={() => void store.runInvestigation()}
            >
              {t('inv.run')}
            </button>
            <button
              type="button"
              className={`${styles.btn} ${styles.btnDanger}`}
              disabled={snap.busyAction !== null}
              onClick={() => void store.cancelInvestigation()}
            >
              {t('inv.cancel')}
            </button>
            <button
              type="button"
              className={styles.btn}
              onClick={() => store.setCreateModalOpen(true)}
            >
              {t('inv.convert')}
            </button>
          </div>
        </div>
        {inv.goal && <div>{inv.goal}</div>}
      </div>

      {/* Hypotheses */}
      <div className={styles.card}>
        <h4 className={styles.cardTitle}>{t('inv.hypotheses')}</h4>
        {hypotheses.length === 0 ? (
          <div style={{ color: 'var(--fw-text-muted)', fontSize: 12 }}>—</div>
        ) : (
          hypotheses.map((h, idx) => (
            <div key={h.id ?? idx} className={styles.findingItem}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <strong>{h.title || h.statement || `H${idx + 1}`}</strong>
                <span className={`${styles.pill} ${hypothesisPillClass(h.status)}`}>
                  {h.status ?? 'open'}
                </span>
              </div>
              {h.title && h.statement && <div style={{ color: 'var(--fw-text-muted)' }}>{h.statement}</div>}
            </div>
          ))
        )}
      </div>

      {/* Action Evidence Log */}
      <div className={styles.card}>
        <h4 className={styles.cardTitle}>{t('inv.actions')}</h4>
        {actions.length === 0 ? (
          <div style={{ color: 'var(--fw-text-muted)', fontSize: 12 }}>—</div>
        ) : (
          actions.map((act, idx) => (
            <div key={act.id ?? act.invocationId ?? idx} className={styles.findingItem}>
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: 'var(--fw-text-subtle)' }}>
                <code>{act.invocationId ?? act.id ?? `#${idx + 1}`}</code>
                <span>{act.status ?? ''}</span>
              </div>
              <div>{act.summary || act.finding || act.command}</div>
            </div>
          ))
        )}
      </div>

      {/* Report */}
      {inv.report && (
        <div className={styles.card}>
          <h4 className={styles.cardTitle}>{t('inv.report')}</h4>
          {inv.report.summary && <div>{inv.report.summary}</div>}
          {inv.report.conclusion && <div style={{ color: 'var(--fw-text-muted)' }}>{inv.report.conclusion}</div>}
        </div>
      )}

      {/* Visual Continue Investigation form (zero raw JSON editing) */}
      <form className={styles.card} onSubmit={handleContinue}>
        <h4 className={styles.cardTitle}>{t('inv.continueTitle')}</h4>
        <div className={styles.field}>
          <label className={styles.fieldLabel}>{t('inv.continueGoal')}</label>
          <textarea
            className={styles.textarea}
            value={followUpGoal}
            onChange={e => setFollowUpGoal(e.target.value)}
            placeholder={t('create.goalPlaceholder')}
          />
        </div>
        <div className={styles.field}>
          <label className={styles.fieldLabel}>{t('inv.continuePaths')}</label>
          <input
            className={styles.input}
            value={followUpPaths}
            onChange={e => setFollowUpPaths(e.target.value)}
            placeholder="internal/machine, plugins/dsh/src"
          />
        </div>
        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <button
            type="submit"
            className={`${styles.btn} ${styles.btnPrimary}`}
            disabled={snap.busyAction !== null || !followUpGoal.trim()}
          >
            {t('inv.continue')}
          </button>
        </div>
      </form>
    </div>
  );
}
