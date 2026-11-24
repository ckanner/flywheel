import { useMemo, useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { MarkdownView } from '../components/MarkdownView.tsx';
import { resolveCandidateLabels, resolvePlanningView } from '../planning.ts';
import type { FlywheelSnapshot, FlywheelStore } from '../store.ts';
import type { DesignViewMode, PlanningFinding, TraceSummary, Translate } from '../types.ts';

function severityPillClass(severity?: string): string {
  const s = (severity ?? '').toLowerCase();
  if (s === 'blocker' || s === 'blocking') return styles.pillDanger;
  if (s === 'major' || s === 'advisory') return styles.pillWarn;
  return styles.pillPurple;
}

function formatDuration(ms?: number | null): string {
  if (typeof ms !== 'number' || !Number.isFinite(ms) || ms < 0) return '';
  if (ms < 1000) return `${Math.round(ms)}ms`;
  const sec = ms / 1000;
  if (sec < 60) return `${sec.toFixed(1)}s`;
  const min = Math.floor(sec / 60);
  const rem = Math.round(sec % 60);
  return `${min}m${String(rem).padStart(2, '0')}s`;
}

function formatFindingQuote(f: PlanningFinding): string {
  const head = f.title || f.target || f.id || 'Finding';
  const body = f.summary || f.impact || f.detail || f.evidence || '';
  return body ? `[${head}] ${body}` : `[${head}]`;
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
  const [briefExpanded, setBriefExpanded] = useState(false);

  const planning = snap.planning ?? snap.state?.planning ?? null;
  const resolved = useMemo(() => resolvePlanningView(planning), [planning]);
  const design = resolved?.design ?? null;
  const review = resolved?.review ?? null;
  const findings = review?.findings ?? [];
  const latestCandidates = snap.designCandidates.at(-1) ?? null;
  const candidateLabels = useMemo(() => resolveCandidateLabels(latestCandidates), [latestCandidates]);

  const synthesizedMarkdown = design?.document || '';
  const hasCandidates = Boolean(latestCandidates && (latestCandidates.primary || latestCandidates.secondary));
  const hasAnyDesign = Boolean(synthesizedMarkdown || hasCandidates);
  const approved = Boolean(resolved?.isApproved);
  const canApprove = Boolean(resolved?.needsHumanApproval && design?.revision && design?.digest);
  const showBlockedBanner = Boolean(resolved && (resolved.isBlocked || (!snap.running && resolved.reason && !hasAnyDesign)));
  const isLivePlanning = Boolean(!hasAnyDesign && !showBlockedBanner && (snap.running || resolved?.isPlanningActive));

  const planningTraces = useMemo<TraceSummary[]>(() => {
    const matched = snap.traces.filter(tr =>
      ['design-plan', 'design-review', 'plan', 'plan-review'].includes(tr.role),
    );
    return matched.slice(0, 4);
  }, [snap.traces]);

  const firstFailedTrace = useMemo(
    () => planningTraces.find(tr => tr.status === 'failed') ?? snap.traces.find(tr => tr.status === 'failed') ?? planningTraces[0] ?? null,
    [planningTraces, snap.traces],
  );

  const briefText = resolved?.briefOriginalRequest || resolved?.briefGoal || '';
  const briefIsLong = briefText.length > 180 || briefText.includes('\n');
  const briefPreview = briefExpanded || !briefIsLong ? briefText : `${briefText.slice(0, 180).trimEnd()}…`;

  const modes: Array<{ id: DesignViewMode; label: string }> = [
    { id: 'synthesized', label: t('design.view.synthesized') },
    { id: 'side-by-side', label: t('design.view.sideBySide') },
    { id: 'structured', label: t('design.view.structured') },
    { id: 'raw', label: t('design.view.raw') },
  ];

  const steps = [
    { idx: 1, title: t('design.step1.title'), desc: t('design.step1.desc') },
    { idx: 2, title: t('design.step2.title'), desc: t('design.step2.desc') },
    { idx: 3, title: t('design.step3.title'), desc: t('design.step3.desc') },
    { idx: 4, title: t('design.step4.title'), desc: t('design.step4.desc') },
  ];

  const quoteFinding = (f: PlanningFinding) => {
    const line = formatFindingQuote(f);
    setFeedbackOpen(true);
    setFeedbackText(prev => (prev.trim() ? `${prev.trim()}\n${line}` : line));
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }} data-testid="flywheel-design-studio">
      {/* 1. Prominent Blocked Diagnostic Banner */}
      {showBlockedBanner && resolved && (
        <section className={styles.blockedBanner} role="alert" data-testid="design-blocked-banner">
          <div className={styles.blockedHeader}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
              <span className={`${styles.pill} ${styles.pillDanger}`}>{t('design.blockedBadge')}</span>
              <strong style={{ fontSize: 13.5 }}>{t('design.blockedTitle')}</strong>
              {typeof resolved.activeRequest?.retries === 'number' && resolved.activeRequest.retries > 0 && (
                <span className={styles.pill}>{t('design.blockedRetries', { count: resolved.activeRequest.retries })}</span>
              )}
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <button
                type="button"
                className={`${styles.btn} ${styles.btnPrimary}`}
                disabled={snap.busyAction !== null || snap.running}
                onClick={() => void store.retryPlanning()}
              >
                {t('design.retryPlanning')}
              </button>
              <button
                type="button"
                className={styles.btn}
                onClick={() => store.openTraceInWorkbench(firstFailedTrace?.id ?? null)}
              >
                {t('design.viewFailedTraces')}
              </button>
            </div>
          </div>

          {resolved.reason && <div className={styles.blockedReasonBox}>{resolved.reason}</div>}

          {resolved.activeRequest && (
            <div className={styles.blockedMetaGrid}>
              <span>
                {t('design.blockedStage')}: <strong>{resolved.activeRequest.role || resolved.activeRequest.stage || 'design-plan'}</strong>
              </span>
              <span>
                {t('design.blockedRequest')}: <code>{resolved.activeRequest.id}</code>
              </span>
              {resolved.activeRequest.channel && (
                <span>
                  {t('design.blockedChannel')}: <strong>{resolved.activeRequest.channel}</strong>
                </span>
              )}
            </div>
          )}
        </section>
      )}

      {/* 2. Task Brief Card */}
      {briefText && (
        <section className={styles.briefCard} data-testid="design-brief-card">
          <div className={styles.briefBody}>
            <div className={styles.briefMetaRow}>
              <strong style={{ fontSize: 13 }}>{t('design.briefTitle')}</strong>
              <span className={styles.pill}>
                {resolved?.policy === 'delegated'
                  ? t('design.briefPolicy.delegated')
                  : t('design.briefPolicy.reviewFirst')}
              </span>
              {resolved?.mode && <span className={`${styles.pill} ${styles.pillPurple}`}>{resolved.mode}</span>}
            </div>
            <p className={styles.briefText}>{briefPreview}</p>
          </div>
          {briefIsLong && (
            <button
              type="button"
              className={`${styles.btn} ${styles.btnSm}`}
              onClick={() => setBriefExpanded(v => !v)}
            >
              {briefExpanded ? t('design.briefCollapse') : t('design.briefExpand')}
            </button>
          )}
        </section>
      )}

      {/* 3. 4-Step Planning Lifecycle Stepper */}
      {resolved && (
        <div className={styles.stepperGrid} data-testid="design-stepper">
          {steps.map(step => {
            const done = step.idx < resolved.stepperStep;
            const current = step.idx === resolved.stepperStep;
            const isErr = current && showBlockedBanner;
            const itemCls = [
              styles.stepperItem,
              isErr ? styles.stepperItemError : current ? styles.stepperItemActive : done ? styles.stepperItemDone : '',
            ]
              .filter(Boolean)
              .join(' ');
            const badgeCls = [
              styles.stepperBadge,
              isErr
                ? styles.stepperBadgeError
                : current
                  ? styles.stepperBadgeActive
                  : done
                    ? styles.stepperBadgeDone
                    : '',
            ]
              .filter(Boolean)
              .join(' ');
            return (
              <div key={step.idx} className={itemCls}>
                <span className={badgeCls}>{done ? '✓' : isErr ? '!' : step.idx}</span>
                <div className={styles.stepperText}>
                  <span className={styles.stepperTitle}>{step.title}</span>
                  <span className={styles.stepperDesc}>{step.desc}</span>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* 4. Live Planning Activity Cards (while drafting/reviewing before design lands) */}
      {isLivePlanning && (
        <section style={{ display: 'flex', flexDirection: 'column', gap: 12 }} data-testid="design-live-cards">
          <div className={styles.cardHeader}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span className={`${styles.statusDot} ${styles.statusDotRunning}`} />
              <h3 className={styles.cardTitle}>{t('design.liveActivityTitle')}</h3>
            </div>
            <button
              type="button"
              className={styles.linkBtn}
              onClick={() => store.openTraceInWorkbench(planningTraces[0]?.id ?? null)}
            >
              {t('design.liveOpenTraces')}
            </button>
          </div>

          {planningTraces.length > 0 ? (
            <div className={styles.liveDraftGrid}>
              {planningTraces.map(tr => (
                <div key={tr.id} className={styles.liveDraftCard}>
                  <div className={styles.cardHeader}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
                      <span
                        className={`${styles.statusDot} ${
                          tr.running
                            ? styles.statusDotRunning
                            : tr.status === 'failed'
                              ? styles.statusDotDanger
                              : styles.statusDotSuccess
                        }`}
                      />
                      <strong style={{ fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {tr.model || tr.harness || tr.role}
                      </strong>
                    </div>
                    <span
                      className={`${styles.pill} ${
                        tr.running ? styles.pillRunning : tr.status === 'failed' ? styles.pillDanger : styles.pillSuccess
                      }`}
                    >
                      {tr.running ? t('traces.liveBadge') : tr.status}
                      {tr.durationMs ? ` · ${formatDuration(tr.durationMs)}` : ''}
                    </span>
                  </div>

                  <div className={styles.liveActivityBox}>
                    {tr.errorSummary ? (
                      <div style={{ color: 'var(--fw-danger)' }}>{tr.errorSummary}</div>
                    ) : tr.preview ? (
                      <div style={{ color: 'var(--fw-text)' }}>{tr.preview}</div>
                    ) : (
                      <div>{t('design.liveWaiting')}</div>
                    )}
                  </div>

                  <div className={styles.liveCardFoot}>
                    <span>
                      {tr.role}
                      {tr.scope ? ` · ${tr.scope}` : ''}
                      {tr.thinking ? ` · ${tr.thinking}` : ''}
                    </span>
                    <button
                      type="button"
                      className={styles.linkBtn}
                      onClick={() => store.openTraceInWorkbench(tr.id)}
                    >
                      {t('design.liveOpenTraces')}
                    </button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className={styles.liveDraftGrid}>
              <div className={styles.liveDraftCard}>
                <div className={styles.cardHeader}>
                  <strong style={{ fontSize: 13 }}>{t('design.livePrimary')}</strong>
                  <span className={`${styles.pill} ${styles.pillRunning}`}>{t('traces.liveBadge')}</span>
                </div>
                <div className={styles.liveActivityBox}>{t('design.liveWaiting')}</div>
              </div>
              <div className={styles.liveDraftCard}>
                <div className={styles.cardHeader}>
                  <strong style={{ fontSize: 13 }}>{t('design.liveSecondary')}</strong>
                  <span className={`${styles.pill} ${styles.pillRunning}`}>{t('traces.liveBadge')}</span>
                </div>
                <div className={styles.liveActivityBox}>{t('design.liveWaiting')}</div>
              </div>
            </div>
          )}

          <div style={{ fontSize: 12, color: 'var(--fw-text-subtle)', textAlign: 'center' }}>
            {t('design.liveBanner')}
          </div>
        </section>
      )}

      {/* 5. Design Document + Dual-Draft Comparison + Structured View + Astra Review */}
      {hasAnyDesign ? (
        <>
          <div className={styles.designToolbar}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
              <h3 className={styles.cardTitle}>{t('design.title')}</h3>
              {design?.revision && (
                <span className={styles.pill}>{t('design.revision', { rev: design.revision })}</span>
              )}
              {approved ? (
                <span className={`${styles.pill} ${styles.pillSuccess}`}>{t('design.approvedBadge')}</span>
              ) : canApprove ? (
                <span className={`${styles.pill} ${styles.pillWarn}`}>{t('design.awaitingBadge')}</span>
              ) : null}
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

          <div className={styles.designGrid}>
            {/* Left main design pane(s) */}
            <div style={{ minWidth: 0, display: 'flex', flexDirection: 'column', gap: 12 }}>
              {snap.designView === 'synthesized' && (
                <div className={styles.designPane}>
                  <div className={styles.cardHeader} style={{ marginBottom: 8 }}>
                    <h4 className={styles.cardTitle}>{t('design.synthesizedHeader')}</h4>
                    {candidateLabels.synthesizerLabel && (
                      <span className={`${styles.pill} ${styles.pillPurple}`}>
                        {candidateLabels.synthesizerLabel}
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
                      {candidateLabels.primaryModel && (
                        <span className={styles.pill}>{candidateLabels.primaryModel}</span>
                      )}
                    </div>
                    <MarkdownView content={latestCandidates?.primary || synthesizedMarkdown || '—'} />
                  </div>
                  <div className={styles.designPane}>
                    <div className={styles.cardHeader} style={{ marginBottom: 8 }}>
                      <h4 className={styles.cardTitle}>{t('design.secondaryDraft')}</h4>
                      {candidateLabels.secondaryModel && (
                        <span className={styles.pill}>{candidateLabels.secondaryModel}</span>
                      )}
                    </div>
                    <MarkdownView content={latestCandidates?.secondary || '—'} />
                  </div>
                </div>
              )}

              {snap.designView === 'structured' && (
                <div className={styles.designPane} data-testid="design-structured-view" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                  {design &&
                  (design.requirements.length > 0 || design.decisions.length > 0 || design.milestones.length > 0) ? (
                    <>
                      {design.requirements.length > 0 && (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                          <h4 className={styles.cardTitle}>{t('design.structuredReq')}</h4>
                          <div className={styles.structuredTableWrap}>
                            <table className={styles.structuredTable}>
                              <thead>
                                <tr>
                                  <th>ID</th>
                                  <th>Requirement</th>
                                  <th>Milestone</th>
                                  <th>Verification</th>
                                </tr>
                              </thead>
                              <tbody>
                                {design.requirements.map(req => (
                                  <tr key={req.id}>
                                    <td><code>{req.id}</code></td>
                                    <td>{req.title}</td>
                                    <td>{req.milestone || '—'}</td>
                                    <td style={{ color: 'var(--fw-text-muted)' }}>{req.verification || '—'}</td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          </div>
                        </div>
                      )}

                      {design.decisions.length > 0 && (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                          <h4 className={styles.cardTitle}>{t('design.structuredDec')}</h4>
                          <div className={styles.structuredTableWrap}>
                            <table className={styles.structuredTable}>
                              <thead>
                                <tr>
                                  <th>ID</th>
                                  <th>Kind</th>
                                  <th>Decision</th>
                                  <th>Rationale</th>
                                </tr>
                              </thead>
                              <tbody>
                                {design.decisions.map(dec => (
                                  <tr key={dec.id}>
                                    <td><code>{dec.id}</code></td>
                                    <td><span className={styles.pill}>{dec.kind || 'fixed'}</span></td>
                                    <td>{dec.statement}</td>
                                    <td style={{ color: 'var(--fw-text-muted)' }}>{dec.rationale || '—'}</td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          </div>
                        </div>
                      )}

                      {design.milestones.length > 0 && (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                          <h4 className={styles.cardTitle}>{t('design.structuredMs')}</h4>
                          <div className={styles.structuredTableWrap}>
                            <table className={styles.structuredTable}>
                              <thead>
                                <tr>
                                  <th>ID</th>
                                  <th>Milestone</th>
                                  <th>Gate</th>
                                </tr>
                              </thead>
                              <tbody>
                                {design.milestones.map(ms => (
                                  <tr key={ms.id}>
                                    <td><code>{ms.id}</code></td>
                                    <td>{ms.title}</td>
                                    <td style={{ color: 'var(--fw-text-muted)' }}>{ms.gate || '—'}</td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          </div>
                        </div>
                      )}
                    </>
                  ) : (
                    <div style={{ color: 'var(--fw-text-muted)', fontSize: 12 }}>{t('design.structuredEmpty')}</div>
                  )}
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
                <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                  <h4 className={styles.cardTitle}>{t('design.reviewPanel')}</h4>
                  {review?.binding && (
                    <span style={{ fontSize: 11, color: 'var(--fw-text-subtle)' }}>{review.binding}</span>
                  )}
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  {review?.verdict && (
                    <span
                      className={`${styles.pill} ${
                        review.verdict.toLowerCase() === 'pass' ? styles.pillSuccess : styles.pillWarn
                      }`}
                    >
                      {review.verdict.toLowerCase() === 'pass' ? t('design.reviewPass') : t('design.reviewFail')}
                    </span>
                  )}
                  <span className={styles.pill}>{findings.length}</span>
                </div>
              </div>

              {review?.summary && (
                <div
                  style={{
                    fontSize: 12,
                    color: 'var(--fw-text-muted)',
                    background: 'var(--fw-bg-subtle)',
                    border: '1px solid var(--fw-border)',
                    borderRadius: 7,
                    padding: '8px 10px',
                  }}
                >
                  {review.summary}
                </div>
              )}

              {findings.length === 0 ? (
                <div style={{ color: 'var(--fw-text-muted)', fontSize: 12 }}>{t('design.noFindings')}</div>
              ) : (
                findings.map((f, idx) => (
                  <div key={f.id ?? idx} className={styles.findingItem}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 6 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                        <span className={`${styles.pill} ${severityPillClass(f.severity)}`}>
                          {f.severity ?? 'minor'}
                        </span>
                        <span className={styles.pill}>{f.status ?? 'open'}</span>
                      </div>
                      {canApprove && (
                        <button
                          type="button"
                          className={styles.linkBtn}
                          onClick={() => quoteFinding(f)}
                        >
                          {t('design.quoteFinding')}
                        </button>
                      )}
                    </div>
                    {(f.title || f.target) && <strong style={{ fontSize: 12 }}>{f.title || f.target}</strong>}
                    <div style={{ fontSize: 12, color: 'var(--fw-text-muted)' }}>
                      {f.summary || f.impact || f.detail || f.evidence}
                    </div>
                    {f.resolution && (
                      <div style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>{f.resolution}</div>
                    )}
                  </div>
                ))
              )}
            </aside>
          </div>
        </>
      ) : !isLivePlanning && !showBlockedBanner ? (
        <div className={styles.emptyState}>
          <div>{t('design.empty')}</div>
        </div>
      ) : null}

      {/* 6. Sticky Human Approval & Feedback Bar */}
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
              <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <strong style={{ fontSize: 12.5 }}>
                  {t('design.approvePrompt', { rev: design?.revision ?? 1 })}
                </strong>
                <span style={{ fontSize: 11.5, color: 'var(--fw-text-muted)' }}>{t('design.approveSub')}</span>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                <button type="button" className={styles.btn} onClick={() => setFeedbackOpen(true)}>
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
