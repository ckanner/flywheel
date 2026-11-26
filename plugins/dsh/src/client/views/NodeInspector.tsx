import { useEffect, useMemo, useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import type { ClientRuntime } from '../context.ts';
import { formatModelBadge } from '../planning.ts';
import type { FlywheelSnapshot, FlywheelStore } from '../store.ts';
import type { InspectorTab, PacketNode, Translate } from '../types.ts';
import { TraceStreamPanel } from './TracesView.tsx';

export function NodeInspector({
  packet,
  snap,
  store,
  t,
  runtime,
}: {
  packet: PacketNode;
  snap: FlywheelSnapshot;
  store: FlywheelStore;
  t: Translate;
  runtime?: ClientRuntime;
}) {
  const [retryReason, setRetryReason] = useState('');
  const [abandonReason, setAbandonReason] = useState('');

  const detail = snap.nodeDetail;
  const uxReview = snap.uxReviews[packet.id] ?? null;
  const packetTraces = useMemo(
    () =>
      snap.traces.filter(
        tr => tr.scope === packet.id || tr.scope === `final:${packet.id}` || tr.key.endsWith(`/${packet.id}`),
      ),
    [snap.traces, packet.id],
  );
  const [activePacketTraceId, setActivePacketTraceId] = useState<string | null>(null);

  useEffect(() => {
    if (snap.inspectorTab !== 'output') return;
    const preferred =
      (activePacketTraceId && packetTraces.some(tr => tr.id === activePacketTraceId) ? activePacketTraceId : null) ??
      packetTraces[0]?.id ??
      null;
    if (preferred && preferred !== activePacketTraceId) {
      setActivePacketTraceId(preferred);
    }
    if (preferred && snap.selectedTraceId !== preferred) {
      store.selectTrace(preferred);
    }
  }, [snap.inspectorTab, packetTraces, activePacketTraceId, snap.selectedTraceId, store]);

  const activeTraceDetail =
    snap.traceDetail && packetTraces.some(tr => tr.id === snap.traceDetail!.id) ? snap.traceDetail : null;

  const tabs: Array<{ id: InspectorTab; label: string }> = [
    { id: 'overview', label: t('inspector.tab.overview') },
    { id: 'output', label: `${t('inspector.tab.output')}${packetTraces.length > 0 ? ` (${packetTraces.length})` : ''}` },
    { id: 'evidence', label: t('inspector.tab.evidence') },
    { id: 'timeline', label: t('inspector.tab.timeline') },
    { id: 'contract', label: t('inspector.tab.contract') },
  ];

  const writeScopes = packet.writeScopes ?? packet.write_scopes ?? [];
  const requirements = packet.requirements ?? [];
  const blocking = detail?.review?.blocking ?? packet.blocking ?? [];
  const advisory = detail?.review?.advisory ?? packet.advisory ?? [];
  const images = detail?.images?.paths ?? packet.images ?? [];
  const timeline = detail?.timeline ?? [];
  const children = packet.collaboration?.children ?? [];

  return (
    <div className={styles.drawerOverlay} role="dialog" aria-label={packet.id} onClick={e => {
      if (e.target === e.currentTarget) store.selectPacket(null);
    }}>
      <aside className={styles.drawer} data-testid="flywheel-node-inspector">
        <div className={styles.drawerHeader}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span className={styles.packetId}>{packet.id}</span>
            <span className={styles.pill}>{packet.status ?? 'pending'}</span>
            {packet.route && <span className={`${styles.pill} ${styles.pillPurple}`}>{packet.route}</span>}
          </div>
          <button type="button" className={`${styles.btn} ${styles.btnSm}`} onClick={() => store.selectPacket(null)}>
            {t('inspector.close')}
          </button>
        </div>

        <div className={styles.tabBar}>
          {tabs.map(tab => (
            <button
              key={tab.id}
              type="button"
              className={`${styles.tabBtn} ${snap.inspectorTab === tab.id ? styles.tabBtnActive : ''}`}
              onClick={() => store.setInspectorTab(tab.id)}
            >
              {tab.label}
            </button>
          ))}
        </div>

        <div className={styles.drawerBody}>
          {snap.inspectorTab === 'overview' && (
            <>
              <div className={styles.card}>
                <h4 className={styles.cardTitle}>{t('inspector.goal')}</h4>
                <div>{packet.goal || packet.title || packet.id}</div>
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', fontSize: 12, color: 'var(--fw-text-muted)' }}>
                  <span title={packet.model || undefined}>
                    {t('inspector.route')}: {packet.route ?? 'default'} / {formatModelBadge(packet.model, typeof packet.thinking === 'string' ? packet.thinking : '', t('tab.traces') === '运行记录') || packet.profile || 'auto'}
                  </span>
                  {packet.sessionKey && (
                    <span>
                      {t('inspector.sessionReuse')}: {packet.sessionStatus || 'scoped'} · <code>{packet.sessionKey}</code>
                    </span>
                  )}
                  {typeof packet.attempts === 'number' && <span>Attempts: {packet.attempts}</span>}
                  {typeof (packet.reviewRounds ?? packet.review_rounds) === 'number' && (
                    <span>Review rounds: {packet.reviewRounds ?? packet.review_rounds}</span>
                  )}
                </div>
              </div>

              {(blocking.length > 0 || advisory.length > 0 || detail?.review?.verdict) && (
                <div className={styles.card}>
                  <h4 className={styles.cardTitle}>{t('inspector.verdict')}: {detail?.review?.verdict ?? packet.reviewVerdict ?? 'pending'}</h4>
                  {blocking.length > 0 && (
                    <div>
                      <strong style={{ color: 'var(--fw-danger)', fontSize: 12 }}>{t('inspector.blocking')}</strong>
                      <ul style={{ margin: '4px 0 0', paddingLeft: 18 }}>
                        {blocking.map((item, idx) => <li key={idx}>{item}</li>)}
                      </ul>
                    </div>
                  )}
                  {advisory.length > 0 && (
                    <div>
                      <strong style={{ color: 'var(--fw-warn)', fontSize: 12 }}>{t('inspector.advisory')}</strong>
                      <ul style={{ margin: '4px 0 0', paddingLeft: 18 }}>
                        {advisory.map((item, idx) => <li key={idx}>{item}</li>)}
                      </ul>
                    </div>
                  )}
                </div>
              )}

              {requirements.length > 0 && (
                <div className={styles.card}>
                  <h4 className={styles.cardTitle}>{t('inspector.requirements')}</h4>
                  <ul style={{ margin: 0, paddingLeft: 18 }}>
                    {requirements.map((req, idx) => <li key={idx}>{req}</li>)}
                  </ul>
                </div>
              )}

              <div className={styles.card}>
                <h4 className={styles.cardTitle}>{t('inspector.actions')}</h4>
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  <button
                    type="button"
                    className={styles.btn}
                    disabled={snap.busyAction !== null}
                    onClick={() => void store.stepPacket('execute', packet.id)}
                  >
                    {t('inspector.stepExecute')}
                  </button>
                  <button
                    type="button"
                    className={styles.btn}
                    disabled={snap.busyAction !== null}
                    onClick={() => void store.stepPacket('review', packet.id)}
                  >
                    {t('inspector.stepReview')}
                  </button>
                </div>

                <div className={styles.field}>
                  <label className={styles.fieldLabel}>{t('inspector.retryLabel')}</label>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <input
                      className={styles.input}
                      value={retryReason}
                      placeholder={t('inspector.retryPlaceholder')}
                      onChange={e => setRetryReason(e.target.value)}
                    />
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnPrimary}`}
                      disabled={snap.busyAction !== null || !retryReason.trim()}
                      onClick={async () => {
                        await store.stepPacket('execute', packet.id, retryReason.trim());
                        setRetryReason('');
                      }}
                    >
                      {t('inspector.retrySubmit')}
                    </button>
                  </div>
                </div>

                {packet.sessionKey && packet.unknownInvocation && (
                  <div className={styles.field}>
                    <label className={styles.fieldLabel}>{t('inspector.abandonSession')}</label>
                    <div style={{ display: 'flex', gap: 8 }}>
                      <input
                        className={styles.input}
                        value={abandonReason}
                        placeholder={t('inspector.abandonPlaceholder')}
                        onChange={e => setAbandonReason(e.target.value)}
                      />
                      <button
                        type="button"
                        className={`${styles.btn} ${styles.btnDanger}`}
                        disabled={snap.busyAction !== null || !abandonReason.trim()}
                        onClick={async () => {
                          await store.abandonSession(packet.sessionKey!, packet.unknownInvocation!, abandonReason.trim());
                          setAbandonReason('');
                        }}
                      >
                        {t('inspector.abandonSubmit')}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            </>
          )}

          {snap.inspectorTab === 'output' && (
            <>
              {packetTraces.length === 0 ? (
                <div className={styles.emptyState} style={{ padding: '28px 16px' }}>
                  <div>{t('inspector.noTraces')}</div>
                  <button
                    type="button"
                    className={`${styles.btn} ${styles.btnSm}`}
                    onClick={() => store.openTraceInWorkbench(null)}
                  >
                    {t('inspector.openTraces')}
                  </button>
                </div>
              ) : (
                <>
                  <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                    {packetTraces.map(tr => {
                      const active = (activePacketTraceId ?? packetTraces[0]?.id) === tr.id;
                      return (
                        <button
                          key={tr.id}
                          type="button"
                          className={`${styles.btn} ${styles.btnSm} ${active ? styles.btnPrimary : ''}`}
                          onClick={() => {
                            setActivePacketTraceId(tr.id);
                            store.selectTrace(tr.id);
                          }}
                        >
                          {tr.role}{tr.attempt ? ` #${tr.attempt}` : ''} · {tr.running ? t('traces.liveBadge') : tr.status}
                        </button>
                      );
                    })}
                  </div>
                  <TraceStreamPanel
                    trace={activeTraceDetail}
                    store={store}
                    t={t}
                    runtime={runtime}
                    compact
                    onOpenFull={() => store.openTraceInWorkbench(activePacketTraceId ?? packetTraces[0]?.id ?? null)}
                  />
                </>
              )}
            </>
          )}

          {snap.inspectorTab === 'evidence' && (
            <>
              <div className={styles.card}>
                <h4 className={styles.cardTitle}>{t('inspector.uxTitle')}</h4>
                {uxReview ? (
                  <>
                    <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                      <span className={`${styles.pill} ${uxReview.verdict === 'pass' ? styles.pillSuccess : styles.pillWarn}`}>
                        {uxReview.verdict ?? 'unknown'}
                      </span>
                      {uxReview.refined && (
                        <span className={`${styles.pill} ${styles.pillPurple}`}>Refined by {uxReview.refinedBy ?? 'Claude'}</span>
                      )}
                    </div>
                    {uxReview.summary && <div>{uxReview.summary}</div>}
                    {Array.isArray(uxReview.findings) && uxReview.findings.length > 0 && (
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                        {uxReview.findings.map((f, idx) => (
                          <div key={idx} className={styles.findingItem}>
                            <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                              <span className={`${styles.pill} ${f.severity === 'blocking' ? styles.pillDanger : styles.pillWarn}`}>
                                {f.severity ?? 'advisory'}
                              </span>
                              {f.location && <code style={{ fontSize: 11 }}>{f.location}</code>}
                            </div>
                            <div>{f.summary}</div>
                            {f.suggestion && <div style={{ color: 'var(--fw-text-muted)', fontSize: 12 }}>{f.suggestion}</div>}
                          </div>
                        ))}
                      </div>
                    )}
                  </>
                ) : (
                  <div style={{ color: 'var(--fw-text-muted)' }}>{t('inspector.uxNone')}</div>
                )}
              </div>

              <div className={styles.card}>
                <h4 className={styles.cardTitle}>{t('inspector.l1Title')}</h4>
                {detail?.l1 && detail.l1.length > 0 ? (
                  detail.l1.map((check, idx) => (
                    <div key={idx} className={styles.findingItem}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <strong>{check.name || check.command || `Check #${idx + 1}`}</strong>
                        <span className={`${styles.pill} ${check.passed ? styles.pillSuccess : styles.pillDanger}`}>
                          {check.passed ? 'PASS' : 'FAIL'}
                        </span>
                      </div>
                      {check.output && <pre className={styles.monoPre}>{check.output}</pre>}
                    </div>
                  ))
                ) : (
                  <div style={{ color: 'var(--fw-text-muted)' }}>—</div>
                )}
              </div>

              <div className={styles.card}>
                <div className={styles.cardHeader}>
                  <h4 className={styles.cardTitle}>{t('inspector.imagesTitle')}</h4>
                  <button type="button" className={`${styles.btn} ${styles.btnSm}`} onClick={() => void store.loadRepoImages()}>
                    {t('inspector.imagesEdit')}
                  </button>
                </div>
                {images.length > 0 ? (
                  <ul style={{ margin: 0, paddingLeft: 18 }}>
                    {images.map(img => <li key={img}><code>{img}</code></li>)}
                  </ul>
                ) : (
                  <div style={{ color: 'var(--fw-text-muted)' }}>{t('inspector.imagesNone')}</div>
                )}
              </div>
            </>
          )}

          {snap.inspectorTab === 'timeline' && (
            <>
              <div className={styles.card}>
                <h4 className={styles.cardTitle}>{t('inspector.tab.timeline')}</h4>
                {timeline.length > 0 ? (
                  timeline.map((ev, idx) => (
                    <div key={idx} className={styles.findingItem}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: 'var(--fw-text-subtle)' }}>
                        <span>{ev.event ?? 'event'}</span>
                        <span>{ev.at ?? ''}</span>
                      </div>
                      {ev.detail && <div>{ev.detail}</div>}
                    </div>
                  ))
                ) : (
                  <div style={{ color: 'var(--fw-text-muted)' }}>{t('inspector.timelineEmpty')}</div>
                )}
              </div>

              <div className={styles.card}>
                <h4 className={styles.cardTitle}>{t('inspector.collabTitle')}</h4>
                {children.length > 0 ? (
                  children.map(child => (
                    <div key={child.id} className={styles.findingItem}>
                      <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                        <span className={styles.packetId}>{child.id}</span>
                        <span className={styles.pill}>{child.status ?? 'pending'}</span>
                      </div>
                      {child.goal && <div>{child.goal}</div>}
                    </div>
                  ))
                ) : (
                  <div style={{ color: 'var(--fw-text-muted)' }}>{t('inspector.collabEmpty')}</div>
                )}
              </div>
            </>
          )}

          {snap.inspectorTab === 'contract' && (
            <>
              <div className={styles.card}>
                <h4 className={styles.cardTitle}>{t('inspector.boundaries')}</h4>
                {writeScopes.length > 0 ? (
                  <ul style={{ margin: 0, paddingLeft: 18 }}>
                    {writeScopes.map(scope => <li key={scope}><code>{scope}</code></li>)}
                  </ul>
                ) : (
                  <div style={{ color: 'var(--fw-text-muted)' }}>—</div>
                )}
              </div>
              <div className={styles.card}>
                <h4 className={styles.cardTitle}>{t('inspector.tab.contract')}</h4>
                <pre className={styles.monoPre}>{JSON.stringify(detail?.contract ?? packet, null, 2)}</pre>
              </div>
            </>
          )}
        </div>
      </aside>
    </div>
  );
}
