import { useMemo } from 'react';
import { styles } from 'virtual:flywheel-styles';
import type { ClientRuntime } from '../context.ts';
import { resolvePlanningView } from '../planning.ts';
import { normalizePackets } from '../planning.ts';
import { isPacketEffectivelyRunning } from '../workflowTimeline.ts';
import type { FlywheelSnapshot, FlywheelStore } from '../store.ts';
import type { PacketNode, Translate } from '../types.ts';
import { DagCanvas } from './DagCanvas.tsx';
import { NodeInspector } from './NodeInspector.tsx';

function groupIntoWaves(packets: PacketNode[], declaredWaves?: string[][]): Array<{ index: number; items: PacketNode[] }> {
  const byId = new Map<string, PacketNode>();
  for (const p of packets) byId.set(p.id, p);

  if (Array.isArray(declaredWaves) && declaredWaves.length > 0) {
    const used = new Set<string>();
    const groups = declaredWaves.map((ids, idx) => {
      const items = ids.map(id => byId.get(id)).filter((p): p is PacketNode => Boolean(p));
      for (const p of items) used.add(p.id);
      return { index: idx + 1, items };
    }).filter(g => g.items.length > 0);
    const leftovers = packets.filter(p => !used.has(p.id));
    if (leftovers.length > 0) groups.push({ index: groups.length + 1, items: leftovers });
    return groups;
  }

  const depth = new Map<string, number>();
  const visiting = new Set<string>();
  const getDepth = (id: string): number => {
    if (depth.has(id)) return depth.get(id)!;
    if (visiting.has(id)) return 0;
    visiting.add(id);
    const p = byId.get(id);
    if (!p) return 0;
    if (typeof p.wave === 'number' && Number.isInteger(p.wave) && p.wave >= 0) {
      depth.set(id, p.wave);
      visiting.delete(id);
      return p.wave;
    }
    const deps = (p.dependsOn ?? p.depends_on ?? []).filter(d => byId.has(d));
    const d = deps.length === 0 ? 0 : Math.max(...deps.map(getDepth)) + 1;
    depth.set(id, d);
    visiting.delete(id);
    return d;
  };
  for (const p of packets) getDepth(p.id);

  const buckets = new Map<number, PacketNode[]>();
  for (const p of packets) {
    const d = depth.get(p.id) ?? 0;
    const list = buckets.get(d) ?? [];
    list.push(p);
    buckets.set(d, list);
  }
  return [...buckets.keys()]
    .sort((a, b) => a - b)
    .map((key, idx) => ({
      index: idx + 1,
      items: buckets.get(key)!.sort((a, b) => a.id.localeCompare(b.id)),
    }));
}

function statusPillClass(status?: string): string {
  const s = (status ?? '').toLowerCase();
  if (['done', 'completed', 'passed'].includes(s)) return styles.pillSuccess;
  if (['running', 'executing', 'reviewing'].includes(s)) return styles.pillRunning;
  if (['failed', 'blocked', 'escalated'].includes(s)) return styles.pillDanger;
  if (['stopped', 'awaiting_approval'].includes(s)) return styles.pillWarn;
  return '';
}

function dotTone(status?: string): string {
  const s = (status ?? '').toLowerCase();
  if (['pass', 'passed', 'done', 'completed', 'ok', 'approved'].includes(s)) return styles.dotPass;
  if (['fail', 'failed', 'blocked', 'rejected', 'error'].includes(s)) return styles.dotFail;
  if (['needs_polish', 'warn', 'returned', 'advisory'].includes(s)) return styles.dotWarn;
  if (['running', 'executing', 'reviewing'].includes(s)) return styles.dotRun;
  return '';
}

export function ExecutionBoard({
  snap,
  store,
  t,
  runtime,
}: {
  snap: FlywheelSnapshot;
  store: FlywheelStore;
  t: Translate;
  runtime?: ClientRuntime;
}) {
  const packets = useMemo<PacketNode[]>(() => normalizePackets(snap.state?.packets), [snap.state?.packets]);

  const waves = useMemo(() => groupIntoWaves(packets, snap.state?.waves), [packets, snap.state?.waves]);
  const selectedPacket = snap.selectedPacketId
    ? packets.find(p => p.id === snap.selectedPacketId) ?? null
    : null;

  const maxAttempts = snap.state?.guardrails?.maxPacketAttempts ?? 3;
  const maxReviewRounds = snap.state?.guardrails?.maxReviewRounds ?? 2;
  const maxCalls = snap.state?.guardrails?.maxTaskCalls ?? 60;
  const usedCalls = snap.state?.guardrails?.currentCalls ?? snap.state?.budget?.totals?.calls ?? 0;

  const budgetEnforce = snap.state?.budget?.enforce ?? 'off';
  const showBudgetBanner = budgetEnforce !== 'off' && budgetEnforce !== '';
  const resolvedPlanning = useMemo(
    () => resolvePlanningView(snap.planning ?? snap.state?.planning ?? null),
    [snap.planning, snap.state?.planning],
  );
  const planningNotApproved = Boolean(resolvedPlanning && !resolvedPlanning.isApproved);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      {/* Guardrails strip + Wave/DAG switcher */}
      <div className={styles.execTopBar}>
        <div className={styles.guardrailsStrip} data-testid="flywheel-guardrails-strip">
          <span className={styles.pill}>{t('exec.guardrails')}</span>
          <span className={styles.pill}>{t('exec.guardrail.attempts', { max: maxAttempts })}</span>
          <span className={styles.pill}>{t('exec.guardrail.reviews', { max: maxReviewRounds })}</span>
          <span className={styles.pill}>{t('exec.guardrail.calls', { used: usedCalls, max: maxCalls })}</span>
        </div>

        <div className={styles.segmented} role="tablist" aria-label="Execution view">
          <button
            type="button"
            role="tab"
            aria-selected={snap.executionView === 'waves'}
            className={`${styles.segmentedBtn} ${snap.executionView === 'waves' ? styles.segmentedBtnActive : ''}`}
            onClick={() => store.setExecutionView('waves')}
          >
            {t('exec.view.waves')}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={snap.executionView === 'dag'}
            className={`${styles.segmentedBtn} ${snap.executionView === 'dag' ? styles.segmentedBtnActive : ''}`}
            onClick={() => store.setExecutionView('dag')}
          >
            {t('exec.view.dag')}
          </button>
        </div>
      </div>

      {/* Dollar budget banner ONLY when explicitly opted in */}
      {showBudgetBanner && (
        <div className={styles.bannerWarn} data-testid="flywheel-budget-banner">
          {t('exec.budgetBanner', {
            spent: `$${(snap.state?.budget?.spentUsd ?? snap.state?.budget?.totals?.cost_usd ?? 0).toFixed(2)}`,
            limit: `$${(snap.state?.budget?.limitUsd ?? 0).toFixed(2)}`,
          })}
        </div>
      )}

      {packets.length === 0 ? (
        <div className={styles.emptyState} data-testid="flywheel-exec-empty">
          {planningNotApproved ? (
            <>
              <strong style={{ fontSize: 14, color: 'var(--fw-text)' }}>{t('exec.planningPendingTitle')}</strong>
              <div style={{ maxWidth: 480 }}>{t('exec.planningPendingDesc')}</div>
              <button
                type="button"
                className={`${styles.btn} ${styles.btnPrimary}`}
                onClick={() => store.setActiveTab('design')}
              >
                {t('exec.backToDesign')}
              </button>
            </>
          ) : (
            <div>{t('exec.empty')}</div>
          )}
        </div>
      ) : snap.executionView === 'waves' ? (
        <div className={styles.waveGrid} data-testid="flywheel-wave-swimlanes">
          {waves.map(wave => (
            <section key={wave.index} className={styles.waveColumn}>
              <div className={styles.waveHeader}>
                <span>{t('exec.waveTitle', { n: wave.index })}</span>
                <span>{wave.items.length}</span>
              </div>
              {wave.items.map(p => {
                const isRunning = isPacketEffectivelyRunning(p, snap.traces, snap.running);
                const cls = [
                  styles.packetCard,
                  snap.selectedPacketId === p.id ? styles.packetCardSelected : '',
                  isRunning ? styles.runningFlowCard : '',
                ]
                  .filter(Boolean)
                  .join(' ');
                return (
                  <button
                    key={p.id}
                    type="button"
                    className={cls}
                    data-packet-id={p.id}
                    onClick={() => store.selectPacket(p.id)}
                  >
                    <div className={styles.packetCardTop}>
                      <span className={styles.packetId}>{p.id}</span>
                      <span className={`${styles.pill} ${statusPillClass(p.status)}`}>
                        {p.status ?? 'pending'}
                      </span>
                    </div>
                    <div className={styles.packetGoal}>{p.goal || p.title || p.id}</div>
                    <div className={styles.packetFooter}>
                      <span>{p.route || p.profile || 'default'}</span>
                      <span className={styles.microDots}>
                        <span className={styles.microDot}>
                          <span className={`${styles.dotCircle} ${dotTone(p.l1Status)}`} />
                          {t('exec.dot.l1')}
                        </span>
                        <span className={styles.microDot}>
                          <span className={`${styles.dotCircle} ${dotTone(p.uxStatus)}`} />
                          {t('exec.dot.ux')}
                        </span>
                        <span className={styles.microDot}>
                          <span className={`${styles.dotCircle} ${dotTone(p.reviewStatus ?? p.reviewVerdict)}`} />
                          {t('exec.dot.review')}
                        </span>
                      </span>
                    </div>
                  </button>
                );
              })}
            </section>
          ))}
        </div>
      ) : (
        <DagCanvas
          packets={packets}
          waves={snap.state?.waves}
          selectedPacketId={snap.selectedPacketId}
          onSelectPacket={id => store.selectPacket(id)}
          t={t}
        />
      )}

      {selectedPacket && (
        <NodeInspector packet={selectedPacket} snap={snap} store={store} t={t} runtime={runtime} />
      )}
    </div>
  );
}
