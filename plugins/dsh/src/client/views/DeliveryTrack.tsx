import { useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import type { FlywheelSnapshot, FlywheelStore } from '../store.ts';
import type { Translate } from '../types.ts';

export function DeliveryTrack({
  snap,
  store,
  t,
}: {
  snap: FlywheelSnapshot;
  store: FlywheelStore;
  t: Translate;
}) {
  const [confirmAction, setConfirmAction] = useState<
    | { kind: 'create'; draft: boolean }
    | { kind: 'merge'; number: number }
    | null
  >(null);

  const delivery = snap.delivery;
  const isZh = t('tab.traces') === '运行记录';
  const endpoint = delivery?.endpoint ?? snap.state?.delivery?.endpoint ?? 'local';
  const isLocalOnly = delivery?.mode === 'local';
  const pulls = delivery?.pulls ?? [];
  const commits = delivery?.recentCommits ?? [];
  const changedFiles = delivery?.changedFiles ?? [];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }} data-testid="flywheel-delivery-track">
      <div className={styles.card}>
        <div className={styles.cardHeader}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            <h3 className={styles.cardTitle}>{t('delivery.title')}</h3>
            <span className={`${styles.pill} ${isLocalOnly ? styles.pillSuccess : styles.pillPurple}`}>
              {isLocalOnly ? (isZh ? '本地仓库交付' : 'Local Delivery') : `${t('delivery.endpoint')}: ${endpoint}`}
            </span>
            {delivery?.workingTreeClean !== undefined && (
              <span className={`${styles.pill} ${delivery.workingTreeClean ? styles.pillSuccess : styles.pillWarn}`}>
                {delivery.workingTreeClean
                  ? (isZh ? '工作区干净' : 'Working Tree Clean')
                  : (isZh ? `${changedFiles.length} 处未提交改动` : `${changedFiles.length} Uncommitted`)}
              </span>
            )}
          </div>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <button type="button" className={styles.btn} onClick={() => void store.loadDelivery()}>
              {t('delivery.refresh')}
            </button>
            {!isLocalOnly && (
              <>
                <button
                  type="button"
                  className={styles.btn}
                  disabled={snap.busyAction !== null}
                  onClick={() => setConfirmAction({ kind: 'create', draft: true })}
                >
                  {t('delivery.createDraftPr')}
                </button>
                <button
                  type="button"
                  className={`${styles.btn} ${styles.btnPrimary}`}
                  disabled={snap.busyAction !== null}
                  onClick={() => setConfirmAction({ kind: 'create', draft: false })}
                >
                  {t('delivery.createPr')}
                </button>
              </>
            )}
          </div>
        </div>

        <div style={{ display: 'flex', gap: 18, flexWrap: 'wrap', fontSize: 12, color: 'var(--fw-text-muted)' }}>
          <span>
            {t('delivery.branch')}: <code>{delivery?.localBranch ?? delivery?.branch ?? snap.state?.delivery?.branch ?? '—'}</code>
          </span>
          {!isLocalOnly && (
            <span>
              {t('delivery.base')}: <code>{delivery?.defaultBase ?? delivery?.baseBranch ?? snap.state?.delivery?.baseBranch ?? 'main'}</code>
            </span>
          )}
          <span>
            {t('delivery.head')}: <code>{delivery?.head ? delivery.head.slice(0, 10) : '—'}</code>
          </span>
          {delivery?.repo && (
            <span>
              Repo: <code>{delivery.repo}</code>
            </span>
          )}
        </div>

        {isLocalOnly && delivery?.remoteMessage && (
          <div
            style={{
              marginTop: 8,
              padding: '10px 12px',
              borderRadius: 8,
              background: 'var(--fw-bg-subtle)',
              border: '1px solid var(--fw-border)',
              fontSize: 12,
              color: 'var(--fw-text-muted)',
              lineHeight: 1.55,
            }}
            data-testid="delivery-local-notice"
          >
            <div>{delivery.remoteMessage}</div>
            <div style={{ marginTop: 4, fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>
              {isZh
                ? '如需推送到 GitHub 创建 PR，只需在终端执行 git remote add origin <GitHub仓库地址> 并完成 gh auth login 后刷新即可。'
                : 'To push and create a GitHub PR later, run `git remote add origin <url>` and `gh auth login`, then refresh.'}
            </div>
          </div>
        )}
      </div>

      {commits.length > 0 && (
        <div className={styles.card} data-testid="delivery-local-commits">
          <h4 className={styles.cardTitle}>{isZh ? '本地最近提交记录' : 'Recent Local Commits'}</h4>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {commits.map(c => (
              <div key={c.sha} className={styles.findingItem} style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
                  <code style={{ fontSize: 11.5, color: 'var(--fw-accent)' }}>{c.sha}</code>
                  <span style={{ fontSize: 12.5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.subject}</span>
                </div>
                {c.at && <span style={{ fontSize: 11, color: 'var(--fw-text-subtle)', flexShrink: 0 }}>{c.at.slice(0, 19).replace('T', ' ')}</span>}
              </div>
            ))}
          </div>
        </div>
      )}

      {changedFiles.length > 0 && (
        <div className={styles.card}>
          <h4 className={styles.cardTitle}>{isZh ? '工作区待提交变动' : 'Uncommitted Changes'}</h4>
          <pre className={styles.monoPre}>{changedFiles.join('\n')}</pre>
        </div>
      )}

      {!isLocalOnly && (
        <div className={styles.card}>
          <h4 className={styles.cardTitle}>{t('delivery.pulls')}</h4>
        {pulls.length === 0 ? (
          <div style={{ color: 'var(--fw-text-muted)', fontSize: 12 }}>{t('delivery.noPr')}</div>
        ) : (
          pulls.map(pr => (
            <div key={pr.number} className={styles.findingItem}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <strong>#{pr.number} {pr.title ?? ''}</strong>
                  {pr.draft && <span className={styles.pill}>Draft</span>}
                  <span className={`${styles.pill} ${pr.state === 'OPEN' ? styles.pillSuccess : ''}`}>
                    {pr.state ?? 'OPEN'}
                  </span>
                </div>
                {pr.state !== 'MERGED' && (
                  <button
                    type="button"
                    className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                    disabled={snap.busyAction !== null}
                    onClick={() => setConfirmAction({ kind: 'merge', number: pr.number })}
                  >
                    {t('delivery.mergePr', { num: pr.number })}
                  </button>
                )}
              </div>
              {Array.isArray(pr.checks) && pr.checks.length > 0 && (
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 4 }}>
                  {pr.checks.map(chk => (
                    <span
                      key={chk.name}
                      className={`${styles.pill} ${
                        chk.conclusion === 'SUCCESS'
                          ? styles.pillSuccess
                          : chk.conclusion === 'FAILURE'
                            ? styles.pillDanger
                            : styles.pillWarn
                      }`}
                    >
                      {chk.name}: {chk.conclusion ?? chk.status ?? 'pending'}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ))
        )}
      </div>
      )}

      {/* Explicit human confirmation modal for PR creation / merge */}
      {confirmAction && (
        <div className={styles.modalBackdrop} role="dialog" aria-modal="true">
          <div className={styles.modal} style={{ width: 440 }}>
            <h3 className={styles.cardTitle}>
              {confirmAction.kind === 'create'
                ? confirmAction.draft
                  ? t('delivery.createDraftPr')
                  : t('delivery.createPr')
                : t('delivery.mergePr', { num: confirmAction.number })}
            </h3>
            <div style={{ color: 'var(--fw-text-muted)' }}>
              {confirmAction.kind === 'create'
                ? t('delivery.confirmCreate')
                : t('delivery.confirmMerge', {
                    num: confirmAction.number,
                    sha: (delivery?.head ?? 'HEAD').slice(0, 10),
                  })}
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
              <button type="button" className={styles.btn} onClick={() => setConfirmAction(null)}>
                {t('design.cancel')}
              </button>
              <button
                type="button"
                className={`${styles.btn} ${styles.btnPrimary}`}
                disabled={snap.busyAction !== null}
                onClick={async () => {
                  const action = confirmAction;
                  setConfirmAction(null);
                  if (action.kind === 'create') {
                    await store.createPullRequest(action.draft);
                  } else {
                    await store.mergePullRequest(action.number);
                  }
                }}
              >
                {t('delivery.confirmBtn')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
