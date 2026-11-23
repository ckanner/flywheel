import { useEffect, useMemo, useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { MarkdownView } from '../components/MarkdownView.tsx';
import type { ClientRuntime, UIWorkspaceService } from '../context.ts';
import type { FlywheelSnapshot, FlywheelStore } from '../store.ts';
import type { TraceDetail, TraceSummary, Translate } from '../types.ts';

type TraceFilter = 'all' | 'running' | 'failed';

const ROLE_LABELS_ZH: Record<string, string> = {
  'design-plan': '架构设计主稿',
  'design-review': '设计架构审查',
  plan: '拆包规划',
  'plan-review': '规划审查',
  execute: '节点执行',
  review: '代码审查',
  'frontend-ux-review': '前端 UX 审查',
  judge: '里程碑裁决',
  investigate: '只读根因调查',
  intake: '需求收件',
  garden: '知识整理',
};

function roleLabel(role: string, isZh: boolean): string {
  if (!isZh) return role;
  return ROLE_LABELS_ZH[role] ?? role;
}

function statusDotClass(status: string, running: boolean): string {
  if (running || status === 'running') return styles.statusDotRunning;
  if (status === 'completed') return styles.statusDotSuccess;
  if (status === 'failed' || status === 'blocked') return styles.statusDotDanger;
  if (status === 'cancelled') return styles.statusDotWarn;
  return '';
}

function statusPillClass(status: string, running: boolean): string {
  if (running || status === 'running') return styles.pillRunning;
  if (status === 'completed') return styles.pillSuccess;
  if (status === 'failed' || status === 'blocked') return styles.pillDanger;
  if (status === 'cancelled') return styles.pillWarn;
  return '';
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

async function copyToClipboard(text: string): Promise<boolean> {
  if (typeof navigator !== 'undefined' && navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fallback below.
    }
  }
  if (typeof document !== 'undefined') {
    try {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      const ok = document.execCommand('copy');
      document.body.removeChild(ta);
      return ok;
    } catch {
      return false;
    }
  }
  return false;
}

export function TraceStreamPanel({
  trace,
  store,
  t,
  runtime,
  compact = false,
  onOpenFull,
}: {
  trace: TraceDetail | null;
  store: FlywheelStore;
  t: Translate;
  runtime?: ClientRuntime;
  compact?: boolean;
  onOpenFull?: () => void;
}) {
  const [expandAllThinking, setExpandAllThinking] = useState(false);
  const [showRaw, setShowRaw] = useState(false);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  const isZh = t('tab.traces') === '运行记录';

  if (!trace) {
    return (
      <div className={styles.emptyState}>
        <div>{t('traces.selectPrompt')}</div>
      </div>
    );
  }

  const handleCopy = async (key: string, cmd: string) => {
    await copyToClipboard(cmd);
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(prev => (prev === key ? null : prev)), 2000);
  };

  const handleOpenDshSession = () => {
    if (!trace.resume?.sessionId) return;
    const workspace = runtime?.getService<UIWorkspaceService>('uiWorkspace');
    workspace?.openSession?.(trace.resume.sessionId);
  };

  const durationText = formatDuration(trace.durationMs);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }} data-testid="flywheel-trace-stream">
      {/* Header Metadata Card */}
      <div className={styles.card}>
        <div className={styles.cardHeader} style={{ flexWrap: 'wrap' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            <span className={`${styles.statusDot} ${statusDotClass(trace.status, trace.running)}`} />
            <strong style={{ fontSize: 13.5 }}>
              {roleLabel(trace.role, isZh)}
              {trace.scope ? ` · ${trace.scope}` : ''}
            </strong>
            <span className={`${styles.pill} ${statusPillClass(trace.status, trace.running)}`}>
              {trace.running ? t('traces.liveBadge') : trace.status}
            </span>
            {trace.exitCode !== null && trace.exitCode !== undefined && trace.exitCode !== 0 && (
              <span className={`${styles.pill} ${styles.pillDanger}`}>exit {trace.exitCode}</span>
            )}
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
            {trace.harness && <span className={styles.pill}>{trace.harness}</span>}
            {trace.model && <span className={`${styles.pill} ${styles.pillPurple}`}>{trace.model}</span>}
            {trace.thinking && <span className={styles.pill}>{trace.thinking}</span>}
            {durationText && <span className={styles.pill}>{durationText}</span>}
            {onOpenFull && (
              <button type="button" className={`${styles.btn} ${styles.btnSm}`} onClick={onOpenFull}>
                {t('inspector.openTraces')}
              </button>
            )}
          </div>
        </div>

        {/* Provider Native Session Jump / Resume Bar */}
        <div className={styles.resumeBar} data-testid="flywheel-resume-bar">
          <div className={styles.resumeBarRow}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
              <strong style={{ fontSize: 12 }}>{t('traces.resumeTitle')}</strong>
              {trace.resume ? (
                <>
                  <span className={styles.pill}>{trace.resume.providerLabel}</span>
                  <code style={{ fontSize: 11, color: 'var(--fw-text-muted)' }}>{trace.resume.sessionId}</code>
                </>
              ) : (
                <span style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>{t('traces.noSessionId')}</span>
              )}
            </div>

            {trace.resume && (
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
                {trace.resume.kind === 'dsh-native' ? (
                  <>
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                      data-testid="resume-open-dsh"
                      onClick={handleOpenDshSession}
                    >
                      {t('traces.openDsh')}
                    </button>
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnSm}`}
                      onClick={() => void handleCopy('dsh-id', trace.resume!.sessionId)}
                    >
                      {copiedKey === 'dsh-id' ? t('traces.copied') : 'Copy ID'}
                    </button>
                  </>
                ) : (
                  <>
                    {trace.resume.command && (
                      <>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnSm}`}
                          data-testid="resume-copy-cmd"
                          onClick={() => void handleCopy('cmd', trace.resume!.command!)}
                        >
                          {copiedKey === 'cmd' ? t('traces.copied') : t('traces.copyResume')}
                        </button>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                          data-testid="resume-open-terminal"
                          onClick={() =>
                            void store.openResumeTerminal(
                              trace.resume!.harness,
                              trace.resume!.sessionId,
                              trace.resume!.cwd || trace.cwd,
                            )
                          }
                        >
                          {t('traces.openTerminal')}
                        </button>
                      </>
                    )}
                    {trace.resume.secondaryCommand && (
                      <button
                        type="button"
                        className={`${styles.btn} ${styles.btnSm}`}
                        onClick={() => void handleCopy('vis', trace.resume!.secondaryCommand!)}
                      >
                        {copiedKey === 'vis' ? t('traces.copied') : t('traces.copyVis')}
                      </button>
                    )}
                  </>
                )}
              </div>
            )}
          </div>

          {trace.resume?.command && (
            <div className={styles.resumeCmd} data-testid="resume-cmd-preview">
              {trace.resume.command}
            </div>
          )}

          {trace.running && (
            <div style={{ fontSize: 11.5, color: 'var(--fw-warn)' }}>
              {t('traces.runningWarn')}
            </div>
          )}
        </div>
      </div>

      {/* Failure Diagnostic Summary */}
      {(trace.status === 'failed' || trace.errorSummary) && (
        <div className={styles.traceBlockError} data-testid="trace-failure-banner">
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <strong>{t('traces.error')}</strong>
            {trace.errorCode && <span className={`${styles.pill} ${styles.pillDanger}`}>{trace.errorCode}</span>}
            {trace.signal && <span className={`${styles.pill} ${styles.pillDanger}`}>{trace.signal}</span>}
          </div>
          {trace.errorSummary && <div style={{ fontSize: 12.5 }}>{trace.errorSummary}</div>}
        </div>
      )}

      {/* Stream Controls */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', gap: 6 }}>
          <button
            type="button"
            className={`${styles.btn} ${styles.btnSm}`}
            onClick={() => setExpandAllThinking(v => !v)}
          >
            {expandAllThinking ? t('traces.collapseThinking') : t('traces.expandThinking')}
          </button>
          <button
            type="button"
            className={`${styles.btn} ${styles.btnSm}`}
            onClick={() => setShowRaw(v => !v)}
          >
            {t('traces.rawEvents')} ({trace.rawTail?.length ?? 0})
          </button>
        </div>
        <button
          type="button"
          className={`${styles.btn} ${styles.btnSm}`}
          onClick={() => void store.loadTraceDetail(trace.id)}
        >
          ↻ {t('rail.refresh')}
        </button>
      </div>

      {/* Structured Session Conversation Stream */}
      {showRaw ? (
        <pre className={styles.monoPre}>
          {(trace.rawTail && trace.rawTail.length > 0 ? trace.rawTail.join('\n') : '—')}
        </pre>
      ) : trace.blocks.length === 0 ? (
        <div className={styles.emptyState} style={{ padding: compact ? '24px 16px' : undefined }}>
          <div>{t('traces.noBlocks')}</div>
        </div>
      ) : (
        <div className={styles.traceStream}>
          {trace.blocks.map((block, idx) => {
            if (block.kind === 'thinking') {
              return (
                <details
                  key={idx}
                  className={styles.traceBlockThinking}
                  open={expandAllThinking || trace.running}
                >
                  <summary>{t('traces.thinking')}{block.ts ? ` · ${block.ts.slice(11, 19)}` : ''}</summary>
                  <div className={styles.traceBlockThinkingBody}>{block.text}</div>
                </details>
              );
            }
            if (block.kind === 'tool_call' || block.kind === 'tool_result') {
              return (
                <details key={idx} className={styles.traceBlockTool}>
                  <summary>
                    <span className={styles.pill}>
                      {block.kind === 'tool_call' ? t('traces.toolCall') : t('traces.toolResult')}
                    </span>
                    <code style={{ fontSize: 11.5 }}>{block.title || 'tool'}</code>
                  </summary>
                  {block.text && (
                    <pre className={styles.monoPre} style={{ marginTop: 8 }}>{block.text}</pre>
                  )}
                </details>
              );
            }
            if (block.kind === 'error') {
              return (
                <div key={idx} className={styles.traceBlockError}>
                  <strong style={{ fontSize: 12 }}>{block.title || t('traces.error')}</strong>
                  <pre className={styles.monoPre} style={{ background: 'transparent', borderColor: 'var(--fw-danger-border)', color: 'inherit' }}>
                    {block.text}
                  </pre>
                </div>
              );
            }
            if (block.kind === 'progress') {
              return (
                <div key={idx} className={styles.traceBlockProgress}>
                  · {block.title ? `[${block.title}] ` : ''}{block.text}
                </div>
              );
            }
            return (
              <div key={idx} className={styles.traceBlockAssistant}>
                <div style={{ fontSize: 11, fontWeight: 600, color: 'var(--fw-text-subtle)', marginBottom: 6 }}>
                  {t('traces.assistant')}
                </div>
                <MarkdownView content={block.text} />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function TracesView({
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
  const [filter, setFilter] = useState<TraceFilter>('all');
  const isZh = t('tab.traces') === '运行记录';

  useEffect(() => {
    if (snap.selectedTaskDir) {
      void store.loadTraces();
    }
  }, [store, snap.selectedTaskDir]);

  const counts = useMemo(() => ({
    all: snap.traces.length,
    running: snap.traces.filter(tr => tr.running || tr.status === 'running').length,
    failed: snap.traces.filter(tr => tr.status === 'failed' || tr.status === 'blocked').length,
  }), [snap.traces]);

  const filtered = useMemo<TraceSummary[]>(() => {
    if (filter === 'running') return snap.traces.filter(tr => tr.running || tr.status === 'running');
    if (filter === 'failed') return snap.traces.filter(tr => tr.status === 'failed' || tr.status === 'blocked');
    return snap.traces;
  }, [snap.traces, filter]);

  if (snap.traces.length === 0) {
    return (
      <div className={styles.emptyState} data-testid="flywheel-traces-view">
        <div>{t('traces.empty')}</div>
        <button type="button" className={styles.btn} onClick={() => void store.loadTraces()}>
          ↻ {t('rail.refresh')}
        </button>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }} data-testid="flywheel-traces-view">
      {snap.notice === 'terminal-opened' && (
        <div className={styles.bannerSuccess}>
          <span>{t('traces.terminalLaunched')}</span>
          <button type="button" className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`} onClick={() => store.clearNotice()}>
            ×
          </button>
        </div>
      )}

      <div className={styles.designToolbar}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <h3 className={styles.cardTitle}>{t('traces.title')}</h3>
          <span className={styles.pill}>{counts.all}</span>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <div className={styles.segmented} role="tablist" aria-label="Trace filter">
            {(
              [
                { id: 'all', label: `${t('traces.filter.all')} (${counts.all})` },
                { id: 'running', label: `${t('traces.filter.running')} (${counts.running})` },
                { id: 'failed', label: `${t('traces.filter.failed')} (${counts.failed})` },
              ] as const
            ).map(item => (
              <button
                key={item.id}
                type="button"
                role="tab"
                aria-selected={filter === item.id}
                className={`${styles.segmentedBtn} ${filter === item.id ? styles.segmentedBtnActive : ''}`}
                onClick={() => setFilter(item.id)}
              >
                {item.label}
              </button>
            ))}
          </div>
          <button type="button" className={`${styles.btn} ${styles.btnSm}`} onClick={() => void store.loadTraces()}>
            ↻
          </button>
        </div>
      </div>

      <div className={styles.tracesLayout}>
        {/* Left Invocation List */}
        <aside className={styles.card} style={{ padding: 8 }}>
          <div className={styles.traceList} data-testid="flywheel-trace-list">
            {filtered.map(tr => {
              const active = tr.id === snap.selectedTraceId;
              const dur = formatDuration(tr.durationMs);
              return (
                <button
                  key={tr.id}
                  type="button"
                  data-trace-id={tr.id}
                  className={`${styles.traceItem} ${active ? styles.traceItemActive : ''}`}
                  onClick={() => store.selectTrace(tr.id)}
                >
                  <div className={styles.traceItemTop}>
                    <span className={styles.traceItemTitle}>
                      <span className={`${styles.statusDot} ${statusDotClass(tr.status, tr.running)}`} />
                      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                        {roleLabel(tr.role, isZh)}
                      </span>
                    </span>
                    <span className={`${styles.pill} ${statusPillClass(tr.status, tr.running)}`}>
                      {tr.running ? t('traces.liveBadge') : tr.status}
                    </span>
                  </div>
                  {tr.scope && (
                    <div className={styles.traceItemMeta}>
                      <span style={{ fontFamily: 'var(--fw-mono)', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                        {tr.scope}{tr.attempt ? ` #${tr.attempt}` : ''}
                      </span>
                      <span>{dur}</span>
                    </div>
                  )}
                  <div className={styles.traceItemMeta}>
                    <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                      {tr.model || tr.harness || '—'}
                    </span>
                    {tr.exitCode !== null && tr.exitCode !== undefined && tr.exitCode !== 0 && (
                      <span style={{ color: 'var(--fw-danger)', fontWeight: 600 }}>exit {tr.exitCode}</span>
                    )}
                  </div>
                </button>
              );
            })}
          </div>
        </aside>

        {/* Right Session-Style Stream & Provider Resume Panel */}
        <div style={{ minWidth: 0 }}>
          <TraceStreamPanel
            trace={snap.traceDetail}
            store={store}
            t={t}
            runtime={runtime}
          />
        </div>
      </div>
    </div>
  );
}
