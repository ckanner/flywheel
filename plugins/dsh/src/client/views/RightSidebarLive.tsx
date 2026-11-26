import { useSyncExternalStore } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { StyleScope } from '../components/StyleScope.tsx';
import type { ClientRuntime, LayoutService } from '../context.ts';
import { normalizePackets } from '../planning.ts';
import type { FlywheelStore } from '../store.ts';
import type { Translate } from '../types.ts';

export function RightSidebarLive({
  store,
  t,
  runtime,
}: {
  store: FlywheelStore;
  t: Translate;
  runtime: ClientRuntime;
}) {
  const snap = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  const packets = normalizePackets(snap.state?.packets);
  const total = packets.length;
  const done = packets.filter(p => ['done', 'completed', 'passed'].includes((p.status ?? '').toLowerCase())).length;
  const activePacket = packets.find(p => ['running', 'executing', 'reviewing'].includes((p.status ?? '').toLowerCase()));
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;

  const openWorkbench = () => {
    const layout = runtime.getService<LayoutService>('layout');
    if (layout?.selectPanel) layout.selectPanel('flywheel');
    else if (layout?.openPanel) layout.openPanel('flywheel');
  };

  return (
    <StyleScope className={styles.sidebarCard}>
      {!snap.selectedTaskDir ? (
        <>
          <div style={{ color: 'var(--fw-text-muted)', fontSize: 12 }}>{t('right.noActive')}</div>
          <button type="button" className={`${styles.btn} ${styles.btnPrimary}`} onClick={openWorkbench}>
            {t('right.openWorkbench')}
          </button>
        </>
      ) : (
        <>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 6 }}>
            <strong style={{ fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {snap.state?.title || snap.state?.slug || snap.selectedTaskDir}
            </strong>
            <span className={`${styles.pill} ${snap.running ? styles.pillRunning : styles.pillSuccess}`}>
              {snap.running ? t('header.running') : (snap.state?.status ?? t('header.idle'))}
            </span>
          </div>

          {total > 0 && (
            <>
              <div style={{ fontSize: 12, color: 'var(--fw-text-muted)' }}>
                {t('right.progress', { done, total })}
              </div>
              <div className={styles.progressTrack}>
                <div className={styles.progressFill} style={{ width: `${pct}%` }} />
              </div>
            </>
          )}

          {activePacket && (
            <div className={styles.findingItem}>
              <span className={styles.packetId}>{activePacket.id}</span>
              <span style={{ fontSize: 12 }}>{activePacket.goal || activePacket.title}</span>
            </div>
          )}

          <div style={{ display: 'flex', gap: 8 }}>
            <button type="button" className={`${styles.btn} ${styles.btnPrimary}`} style={{ flex: 1 }} onClick={openWorkbench}>
              {t('right.openWorkbench')}
            </button>
            {snap.running && (
              <button
                type="button"
                className={`${styles.btn} ${styles.btnDanger}`}
                disabled={snap.busyAction !== null}
                onClick={() => void store.stopRun()}
              >
                {t('right.stop')}
              </button>
            )}
          </div>
        </>
      )}
    </StyleScope>
  );
}
