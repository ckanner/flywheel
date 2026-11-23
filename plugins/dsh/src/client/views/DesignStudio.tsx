import { useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { MarkdownView } from '../components/MarkdownView.tsx';
import type { FlywheelSnapshot, FlywheelStore } from '../store.ts';
import type { DesignViewMode, Translate } from '../types.ts';

function severityPillClass(severity?: string): string {
  const s = (severity ?? '').toLowerCase();
  if (s === 'blocker' || s === 'blocking') return styles.pillDanger;
  if (s === 'major') return styles.pillWarn;
  return styles.pillPurple;
}

export function DesignStudio({
  snap,
  store,
  t,
}: {
  snap: FlywheelSnapshot;
  store: FlywheelStore;
  t: Translate;
}) {
  const [feedbackOpen, setFeedbackOpen] = useState(false);
  const [feedbackText, setFeedbackText] = useState('');

  const planning = snap.planning ?? snap.state?.planning ?? null;
  const design = planning?.design;
  const findings = design?.findings ?? planning?.findings ?? [];
  const latestCandidates = snap.designCandidates.at(-1) ?? null;

  const synthesizedMarkdown =
    design?.markdown ||
    design?.content ||
    design?.summary ||
    '';
  const hasCandidates = Boolean(latestCandidates && (latestCandidates.primary || latestCandidates.secondary));
  const hasAnyDesign = Boolean(synthesizedMarkdown || hasCandidates);
  const approved = Boolean(design?.approved);
  const canApprove = Boolean(design?.revision && design?.digest && !approved);

  const modes: Array<{ id: DesignViewMode; label: string }> = [
    { id: 'synthesized', label: t('design.view.synthesized') },
    { id: 'side-by-side', label: t('design.view.sideBySide') },
    { id: 'raw', label: t('design.view.raw') },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }} data-testid="flywheel-design-studio">
      <div className={styles.designToolbar}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <h3 className={styles.cardTitle}>{t('design.title')}</h3>
          {design?.revision && (
            <span className={styles.pill}>{t('design.revision', { rev: design.revision })}</span>
          )}
          {approved && (
            <span className={`${styles.pill} ${styles.pillSuccess}`}>{t('design.approvedBadge')}</span>
          )}
        </div>

        <div className={styles.segmented} role="tablist" aria-label="Design view mode">
          {modes.map(m => (
            <button
              key={m.id}
              type="button"
              role="tab"
              aria-selected={snap.designView === m.id}
              className={`${styles.segmentedBtn} ${snap.designView === m.id ? styles.segmentedBtnActive : ''}`}
              onClick={() => store.setDesignView(m.id)}
            >
              {m.label}
            </button>
          ))}
        </div>
      </div>

      {!hasAnyDesign ? (
        <div className={styles.emptyState}>
          <div>{t('design.empty')}</div>
        </div>
      ) : (
        <div className={styles.designGrid}>
          {/* Left main design pane(s) */}
          <div style={{ minWidth: 0, display: 'flex', flexDirection: 'column', gap: 12 }}>
            {snap.designView === 'synthesized' && (
              <div className={styles.designPane}>
                <div className={styles.cardHeader} style={{ marginBottom: 8 }}>
                  <h4 className={styles.cardTitle}>{t('design.synthesizedHeader')}</h4>
                  {latestCandidates?.meta?.synthesizerProfile && (
                    <span className={`${styles.pill} ${styles.pillPurple}`}>
                      {String(latestCandidates.meta.synthesizerProfile)}
                    </span>
                  )}
                </div>
                <MarkdownView content={synthesizedMarkdown || latestCandidates?.primary || ''} />
              </div>
            )}

            {snap.designView === 'side-by-side' && (
              <div className={styles.designSplit} data-testid="design-side-by-side">
                <div className={styles.designPane}>
                  <div className={styles.cardHeader} style={{ marginBottom: 8 }}>
                    <h4 className={styles.cardTitle}>{t('design.primaryDraft')}</h4>
                    {latestCandidates?.meta?.primaryModel && (
                      <span className={styles.pill}>{String(latestCandidates.meta.primaryModel)}</span>
                    )}
                  </div>
                  <MarkdownView content={latestCandidates?.primary || synthesizedMarkdown || '—'} />
                </div>
                <div className={styles.designPane}>
                  <div className={styles.cardHeader} style={{ marginBottom: 8 }}>
                    <h4 className={styles.cardTitle}>{t('design.secondaryDraft')}</h4>
                    {latestCandidates?.meta?.secondaryModel && (
                      <span className={styles.pill}>{String(latestCandidates.meta.secondaryModel)}</span>
                    )}
                  </div>
                  <MarkdownView content={latestCandidates?.secondary || '—'} />
                </div>
              </div>
            )}

            {snap.designView === 'raw' && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div className={styles.designPane}>
                  <h4 className={styles.cardTitle} style={{ marginBottom: 8 }}>{t('design.primaryDraft')}</h4>
                  <pre className={styles.monoPre}>{latestCandidates?.primary || synthesizedMarkdown || '—'}</pre>
                </div>
                <div className={styles.designPane}>
                  <h4 className={styles.cardTitle} style={{ marginBottom: 8 }}>{t('design.secondaryDraft')}</h4>
                  <pre className={styles.monoPre}>{latestCandidates?.secondary || '—'}</pre>
                </div>
              </div>
            )}
          </div>

          {/* Right Astra independent review panel */}
          <aside className={styles.card} data-testid="design-review-panel">
            <div className={styles.cardHeader}>
              <h4 className={styles.cardTitle}>{t('design.reviewPanel')}</h4>
              <span className={styles.pill}>{findings.length}</span>
            </div>
            {findings.length === 0 ? (
              <div style={{ color: 'var(--fw-text-muted)', fontSize: 12 }}>{t('design.noFindings')}</div>
            ) : (
              findings.map((f, idx) => (
                <div key={f.id ?? idx} className={styles.findingItem}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 6 }}>
                    <span className={`${styles.pill} ${severityPillClass(f.severity)}`}>
                      {f.severity ?? 'minor'}
                    </span>
                    <span className={styles.pill}>{f.status ?? 'open'}</span>
                  </div>
                  {f.title && <strong style={{ fontSize: 12 }}>{f.title}</strong>}
                  <div style={{ fontSize: 12, color: 'var(--fw-text-muted)' }}>{f.summary || f.detail}</div>
                </div>
              ))
            )}
          </aside>
        </div>
      )}

      {/* Sticky human approval & feedback bar */}
      {canApprove && (
        <div className={styles.stickyApprovalBar} data-testid="design-approval-bar">
          {feedbackOpen ? (
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 8 }}>
              <textarea
                className={styles.textarea}
                value={feedbackText}
                placeholder={t('design.feedbackPlaceholder')}
                onChange={e => setFeedbackText(e.target.value)}
              />
              <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
                <button type="button" className={styles.btn} onClick={() => setFeedbackOpen(false)}>
                  {t('design.cancel')}
                </button>
                <button
                  type="button"
                  className={`${styles.btn} ${styles.btnPrimary}`}
                  disabled={snap.busyAction !== null || !feedbackText.trim()}
                  onClick={async () => {
                    await store.submitDesignFeedback(feedbackText);
                    setFeedbackText('');
                    setFeedbackOpen(false);
                  }}
                >
                  {t('design.submitFeedback')}
                </button>
              </div>
            </div>
          ) : (
            <>
              <div style={{ fontSize: 12, color: 'var(--fw-text-muted)' }}>
                {t('design.revision', { rev: design?.revision ?? 1 })} · digest <code>{String(design?.digest ?? '').slice(0, 12)}</code>
              </div>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                <button
                  type="button"
                  className={styles.btn}
                  disabled={snap.busyAction !== null}
                  onClick={() => setFeedbackOpen(true)}
                >
                  {t('design.requestRevision')}
                </button>
                <button
                  type="button"
                  className={styles.btn}
                  disabled={snap.busyAction !== null}
                  onClick={() => void store.approveDesign(false)}
                >
                  {t('design.approveOnly')}
                </button>
                <button
                  type="button"
                  className={`${styles.btn} ${styles.btnPrimary}`}
                  disabled={snap.busyAction !== null}
                  onClick={() => void store.approveDesign(true)}
                >
                  {t('design.approveAndStart')}
                </button>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}
