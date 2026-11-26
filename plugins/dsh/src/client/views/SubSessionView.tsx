import { type ClipboardEvent, type DragEvent, useMemo, useRef, useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { SubSessionTelemetryDock } from '../components/ComposerTelemetryDock.tsx';
import { MarkdownView } from '../components/MarkdownView.tsx';
import type { ClientRuntime } from '../context.ts';
import { formatModelBadge, normalizePacketNode, resolvePlanningView } from '../planning.ts';
import { useSmartAutoScroll } from '../scroll.ts';
import type { FlywheelSnapshot, FlywheelStore } from '../store.ts';
import type { PacketNode, SessionHistoryTurn, TraceDetail, TraceSummary, Translate } from '../types.ts';
import { SessionTopStrip } from './SessionPopover.tsx';

const ROLE_LABELS_ZH: Record<string, string> = {
  clarify: 'Stage 0 需求澄清',
  'design-plan': '架构设计主稿',
  'design-review': '设计架构审查',
  'ui-prototype': 'UI/UX 交互原型',
  'ui-prototype-review': 'UI/UX 原型审查',
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

function roleLabel(role: string, isZh: boolean, scope?: string, sessionTitle?: string): string {
  if (role === 'design-plan') {
    if (scope?.endsWith(':secondary')) return isZh ? '架构设计对照稿' : 'Secondary Design Draft';
    if (scope?.endsWith(':synthesize') || sessionTitle?.includes(':synthesize')) return isZh ? '双稿统稿合并' : 'Design Synthesis';
    if (scope?.endsWith(':primary')) return isZh ? '架构设计主稿' : 'Primary Design Draft';
  }
  if (!isZh) return role;
  return ROLE_LABELS_ZH[role] ?? role;
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

type ChainEntry =
  | { kind: 'thinking'; text: string; ts?: string; encrypted?: boolean }
  | { kind: 'tool'; title: string; ts?: string; input?: string; output?: string }
  | { kind: 'note'; title?: string; ts?: string; text: string };

function groupChainEntries(items: TraceDetail['blocks']): ChainEntry[] {
  const entries: ChainEntry[] = [];
  for (let i = 0; i < items.length; i += 1) {
    const item = items[i]!;
    if (item.kind === 'thinking') {
      if (item.text.trim()) {
        entries.push({
          kind: 'thinking',
          text: item.text,
          ts: item.ts,
          encrypted: item.title === 'encrypted',
        });
      }
      continue;
    }
    if (item.kind === 'tool_call') {
      const next = items[i + 1];
      if (next && next.kind === 'tool_result') {
        entries.push({ kind: 'tool', title: item.title || 'tool', ts: item.ts || next.ts, input: item.text, output: next.text });
        i += 1;
      } else {
        entries.push({ kind: 'tool', title: item.title || 'tool', ts: item.ts, input: item.text });
      }
      continue;
    }
    if (item.kind === 'tool_result') {
      // Attach to most recent unmatched tool_call (handles parallel tool_calls followed by parallel GENERIC results)
      const unmatched = entries.find(e => e.kind === 'tool' && e.output === undefined);
      if (unmatched && unmatched.kind === 'tool') {
        unmatched.output = item.text;
        if (!unmatched.ts && item.ts) unmatched.ts = item.ts;
      } else {
        entries.push({ kind: 'tool', title: item.title || 'tool', ts: item.ts, output: item.text });
      }
      continue;
    }
    if (item.kind === 'progress' && item.text.trim()) {
      entries.push({ kind: 'note', title: item.title, ts: item.ts, text: item.text });
    }
  }
  return entries;
}

function formatShortClock(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function formatExactClock(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
}

function UserPromptBubble({
  text,
  ts,
  writeScopes,
  isZh,
}: {
  text: string;
  ts?: string;
  writeScopes?: string[];
  isZh: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const trimmed = text.trim();
  const isLong = trimmed.length > 360 || trimmed.split('\n').length > 6;
  const previewLines = trimmed.split('\n').slice(0, 5).join('\n');
  const preview = previewLines.length > 340 ? `${previewLines.slice(0, 340)}…` : previewLines;
  const clock = formatExactClock(ts);

  return (
    <div className={styles.userBubbleRow} data-testid="sub-real-prompt-bubble">
      <span className={styles.userBubbleLabel}>
        {isZh ? '发送给 Provider 的提示词' : 'Prompt Sent to Provider'}
        {clock ? ` · ${clock}` : ''}
      </span>
      <div className={styles.userBubble} style={{ maxWidth: '92%' }}>
        {expanded || !isLong ? (
          <div style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontSize: 12.5, lineHeight: 1.55 }}>
            {trimmed}
          </div>
        ) : (
          <div style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontSize: 12.5, lineHeight: 1.55 }}>
            {preview}
          </div>
        )}
        {isLong && (
          <div style={{ marginTop: 6 }}>
            <button
              type="button"
              className={styles.linkBtn}
              style={{ fontSize: 11.5 }}
              onClick={() => setExpanded(v => !v)}
            >
              {expanded
                ? (isZh ? '收起完整提示词 ▴' : 'Collapse prompt ▴')
                : (isZh ? `展开完整提示词（共 ${trimmed.length.toLocaleString()} 字）▾` : `Expand full prompt (${trimmed.length.toLocaleString()} chars) ▾`)}
            </button>
          </div>
        )}
        {writeScopes && writeScopes.length > 0 && (
          <div className={styles.userBubbleMeta}>
            <span style={{ fontSize: 11, color: 'var(--fw-text-subtle)', marginRight: 2 }}>
              {isZh ? '允许写入范围:' : 'Write scope:'}
            </span>
            {writeScopes.slice(0, 6).map(sc => (
              <span key={sc} className={styles.pill}>{sc}</span>
            ))}
            {writeScopes.length > 6 && (
              <span className={styles.pill}>+{writeScopes.length - 6}</span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function ThoughtChainDisclosure({
  items,
  idx,
  running,
  hasAssistantOutput,
  isZh,
}: {
  items: TraceDetail['blocks'];
  idx: number;
  running: boolean;
  hasAssistantOutput: boolean;
  isZh: boolean;
}) {
  const [userOpen, setUserOpen] = useState<boolean | null>(null);
  const entries = useMemo(() => groupChainEntries(items), [items]);
  const thinkCount = entries.filter(e => e.kind === 'thinking').length;
  const toolCount = entries.filter(e => e.kind === 'tool').length;

  const isOpen = userOpen !== null ? userOpen : (running || !hasAssistantOutput);

  const thinkSummary = thinkCount > 0
    ? (running ? (isZh ? '正在思考与推理…' : 'Thinking…') : (isZh ? `已思考 (${thinkCount})` : `Thought (${thinkCount})`))
    : '';
  const toolSummary = toolCount > 0
    ? (isZh ? `执行了 ${toolCount} 个工具` : `${toolCount} tool call${toolCount > 1 ? 's' : ''}`)
    : '';
  const chainTitle = [thinkSummary, toolSummary].filter(Boolean).join(' · ') || (isZh ? '思考与执行过程' : 'Thought & Tool Process');

  return (
    <details className={styles.thoughtChain} data-testid={`thought-chain-${idx}`} open={isOpen}>
      <summary
        className={styles.thoughtChainHead}
        aria-expanded={isOpen}
        onClick={e => {
          e.preventDefault();
          setUserOpen(prev => (prev === null ? !isOpen : !prev));
        }}
      >
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7, minWidth: 0 }}>
          <span className={running ? styles.stepsDotRunning : styles.stepsDot} />
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{chainTitle}</span>
        </span>
        <span style={{ fontSize: 11, color: 'var(--fw-text-subtle)', flexShrink: 0 }}>
          {isOpen ? (isZh ? '收起 ▾' : 'Hide ▾') : (isZh ? '展开过程 ▸' : 'Show steps ▸')}
        </span>
      </summary>
      <div className={styles.thoughtChainBody}>
        {entries.map((entry, subIdx) => {
          const stepClock = formatExactClock(entry.ts);
          if (entry.kind === 'thinking') {
            if (entry.encrypted) {
              return (
                <div
                  key={subIdx}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    gap: 8,
                    padding: '4px 10px',
                    fontSize: 11.5,
                    color: 'var(--fw-text-subtle)',
                    fontStyle: 'italic',
                    borderLeft: '2px dashed var(--fw-border-strong)',
                    marginLeft: 2,
                  }}
                >
                  <span>
                    {isZh
                      ? '深度思考（Provider 加密保护，未返回明文推理）'
                      : 'Deep reasoning (encrypted by provider; no plaintext summary)'}
                  </span>
                  {stepClock && (
                    <span style={{ fontFamily: 'var(--fw-mono)', fontSize: 10.5, fontStyle: 'normal', flexShrink: 0 }}>
                      {stepClock}
                    </span>
                  )}
                </div>
              );
            }
            return (
              <div key={subIdx} className={styles.thoughtStepProse}>
                {stepClock && (
                  <div style={{ fontSize: 10.5, fontFamily: 'var(--fw-mono)', color: 'var(--fw-text-subtle)', marginBottom: 3 }}>
                    {stepClock} · {isZh ? '思考' : 'Thinking'}
                  </div>
                )}
                <MarkdownView content={entry.text} />
              </div>
            );
          }
          if (entry.kind === 'note') {
            return (
              <div
                key={subIdx}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: 8,
                  fontSize: 11.5,
                  color: 'var(--fw-text-subtle)',
                  padding: '2px 8px',
                }}
              >
                <span>{entry.title ? `[${entry.title}] ` : ''}{entry.text}</span>
                {stepClock && (
                  <span style={{ fontFamily: 'var(--fw-mono)', fontSize: 10.5, flexShrink: 0 }}>{stepClock}</span>
                )}
              </div>
            );
          }
          return (
            <details key={subIdx} className={styles.modelToolRow}>
              <summary>
                <span className={styles.modelToolTag}>
                  {entry.output !== undefined ? (isZh ? '工具 ✓' : 'Tool ✓') : (isZh ? '执行中…' : 'Running…')}
                </span>
                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1 }}>
                  {entry.title}
                </span>
                {stepClock && (
                  <span style={{ fontFamily: 'var(--fw-mono)', fontSize: 10.5, color: 'var(--fw-text-subtle)', flexShrink: 0 }}>
                    {stepClock}
                  </span>
                )}
              </summary>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginTop: 8 }}>
                {entry.input && (
                  <div>
                    <div style={{ fontSize: 10.5, fontWeight: 600, color: 'var(--fw-text-subtle)', marginBottom: 3 }}>
                      {isZh ? '调用参数' : 'Input'}
                    </div>
                    <pre className={styles.monoPre}>{entry.input}</pre>
                  </div>
                )}
                {entry.output && (
                  <div>
                    <div style={{ fontSize: 10.5, fontWeight: 600, color: 'var(--fw-text-subtle)', marginBottom: 3 }}>
                      {isZh ? '返回输出' : 'Output'}
                    </div>
                    <pre className={styles.monoPre}>{entry.output}</pre>
                  </div>
                )}
              </div>
            </details>
          );
        })}
      </div>
    </details>
  );
}

export function SubSessionView({
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
  const [draft, setDraft] = useState('');
  const [showRaw, setShowRaw] = useState(false);
  const [attachMenuOpen, setAttachMenuOpen] = useState(false);
  const [showRepoImages, setShowRepoImages] = useState(false);
  const [dragging, setDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const isZh = t('tab.traces') === '运行记录';

  const handleUploadFiles = async (files: FileList | File[]) => {
    const cwd = snap.selectedCwd;
    if (!cwd) return;
    for (const file of Array.from(files)) {
      if (!file.type.startsWith('image/')) continue;
      const b64 = await new Promise<string>(resolve => {
        const reader = new FileReader();
        reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '');
        reader.onerror = () => resolve('');
        reader.readAsDataURL(file);
      });
      if (!b64) continue;
      const saved = await store.uploadAttachment(cwd, {
        name: file.name || 'pasted-image.png',
        mimeType: file.type,
        dataBase64: b64,
      });
      if (saved?.path) {
        setDraft(prev => (prev.trim() ? `${prev.trim()} [${saved.path}]` : `[${saved.path}] `));
      }
    }
    setAttachMenuOpen(false);
  };

  const handlePaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    const imageFiles = Array.from(e.clipboardData?.files ?? []).filter(f => f.type.startsWith('image/'));
    if (imageFiles.length > 0) {
      e.preventDefault();
      void handleUploadFiles(imageFiles);
    }
  };

  const handleDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setDragging(false);
    const imageFiles = Array.from(e.dataTransfer?.files ?? []).filter(f => f.type.startsWith('image/'));
    if (imageFiles.length > 0) {
      void handleUploadFiles(imageFiles);
    }
  };

  const activeNodeId = snap.activeNodeId || '';
  const packet: PacketNode | null = useMemo(() => {
    const packets = snap.state?.packets;
    if (!packets) return null;
    if (snap.selectedPacketId && packets[snap.selectedPacketId]) {
      return normalizePacketNode(snap.selectedPacketId, packets[snap.selectedPacketId]!);
    }
    if (activeNodeId && packets[activeNodeId]) {
      return normalizePacketNode(activeNodeId, packets[activeNodeId]!);
    }
    return null;
  }, [snap.state?.packets, snap.selectedPacketId, activeNodeId]);

  const trace: TraceDetail | null = useMemo(() => {
    if (snap.traceDetail && (!snap.selectedTraceId || snap.traceDetail.id === snap.selectedTraceId)) {
      return snap.traceDetail;
    }
    return snap.traceDetail;
  }, [snap.traceDetail, snap.selectedTraceId]);

  // Scope sibling call history to the same role+scope key (never mix planner &
  // reviewer calls into one flat button strip), sorted chronologically oldest -> newest.
  const matchingTraces = useMemo<TraceSummary[]>(() => {
    const selectedSummary = snap.traces.find(tr => tr.id === (trace?.id ?? snap.selectedTraceId)) ?? null;
    const targetKey = trace?.key || selectedSummary?.key || '';
    let matched: TraceSummary[] = [];
    if (!packet && targetKey) {
      matched = snap.traces.filter(tr => tr.key === targetKey);
    } else if (activeNodeId) {
      matched = snap.traces.filter(
        tr =>
          tr.id === activeNodeId ||
          tr.scope === activeNodeId ||
          tr.key === activeNodeId ||
          tr.key.endsWith(`/${activeNodeId}`),
      );
    } else {
      matched = snap.traces;
    }
    if (matched.length === 0 && snap.selectedTraceId) {
      matched = snap.traces.filter(tr => tr.id === snap.selectedTraceId);
    }
    return [...matched].sort((a, b) => {
      const ta = Date.parse(a.startedAt || a.updatedAt || '') || 0;
      const tb = Date.parse(b.startedAt || b.updatedAt || '') || 0;
      return ta - tb;
    });
  }, [snap.traces, activeNodeId, snap.selectedTraceId, trace?.id, trace?.key, packet]);

  const sessionKey = snap.activeSessionKey || trace?.key || (packet ? packet.sessionKey || `execute/${packet.id}` : null);
  const historyTurns: SessionHistoryTurn[] = sessionKey ? (snap.sessionHistories[sessionKey] ?? []) : [];

  const resolvedPlanning = useMemo(
    () => resolvePlanningView(snap.planning ?? snap.state?.planning ?? null),
    [snap.planning, snap.state?.planning],
  );

  const initialPromptText = useMemo(() => {
    if (packet) {
      const parts = [packet.goal || packet.title || packet.id];
      if (packet.requirements && packet.requirements.length > 0) {
        parts.push(packet.requirements.map(r => `• ${r}`).join('\n'));
      }
      return parts.filter(Boolean).join('\n\n');
    }
    if (resolvedPlanning?.briefOriginalRequest || resolvedPlanning?.briefGoal) {
      return resolvedPlanning.briefOriginalRequest || resolvedPlanning.briefGoal;
    }
    if (snap.state?.investigation?.goal) {
      return snap.state.investigation.goal;
    }
    return '';
  }, [packet, resolvedPlanning, snap.state?.investigation?.goal]);

  const nodeDisplayName = packet
    ? `${packet.id}${packet.title ? ` · ${packet.title}` : ''}`
    : trace
      ? `${roleLabel(trace.role, isZh, trace.scope, trace.sessionTitle)}${trace.scope ? ` · ${trace.scope}` : ''}`
      : activeNodeId || t('chat.mainFlow');

  const providerLabel = trace?.resume?.providerLabel || trace?.harness || packet?.route || 'Provider';
  const packetThinking = typeof (packet as Record<string, unknown> | null)?.thinking === 'string'
    ? String((packet as Record<string, unknown>).thinking)
    : '';
  const rawModelId = trace?.model || packet?.model || '';
  const modelLabel = formatModelBadge(rawModelId, trace?.thinking || packetThinking, isZh);

  const blockCount = trace?.blocks.length ?? 0;
  const lastBlockLen = blockCount > 0 ? (trace?.blocks[blockCount - 1]?.text?.length ?? 0) : 0;
  const subScrollResetKey = `sub::${snap.selectedTaskDir || ''}::${activeNodeId || ''}::${trace?.id || snap.selectedTraceId || ''}`;
  const subScrollContentKey = `${trace?.id || ''}:${blockCount}:${lastBlockLen}:${historyTurns.length}:${trace?.running ? 1 : 0}:${trace?.updatedAt || ''}`;
  const {
    scrollRef: subScrollRef,
    showScrollToBottom: showSubScrollBottom,
    scrollToBottom: scrollSubToBottom,
    handleScroll: handleSubScroll,
  } = useSmartAutoScroll(subScrollResetKey, subScrollContentKey);

  const handleSend = async () => {
    const text = draft.trim();
    if (!text || snap.busyAction !== null) return;
    setDraft('');
    if (sessionKey) {
      await store.sendSessionMessage(sessionKey, text);
    } else if (resolvedPlanning && !resolvedPlanning.isApproved) {
      await store.submitDesignFeedback(text);
    }
  };

  return (
    <div className={styles.chatWorkspace} data-testid="flywheel-sub-session">
      <div className={styles.chatScrollArea} ref={subScrollRef} onScroll={handleSubScroll}>
        <div className={styles.chatColumn}>
          {/* Lightweight scrollable Provider Session Strip */}
          {trace && <SessionTopStrip trace={trace} store={store} t={t} runtime={runtime} />}

          {/* Invocation history bar when this same role+scope has multiple rounds/retries */}
          {matchingTraces.length > 1 && (
            <div className={styles.callHistoryBar} data-testid="sub-call-history">
              <span className={styles.callHistoryLabel}>
                {isZh ? `历史调用 (${matchingTraces.length} 轮)` : `Invocations (${matchingTraces.length})`}
              </span>
              <div className={styles.callHistoryList}>
                {matchingTraces.map((tr, idx) => {
                  const active = tr.id === (trace?.id ?? snap.selectedTraceId);
                  const dotCls =
                    tr.running
                      ? styles.statusDotRunning
                      : tr.status === 'failed' || tr.status === 'blocked'
                        ? styles.statusDotDanger
                        : tr.status === 'completed'
                          ? styles.statusDotSuccess
                          : styles.statusDot;
                  const clock = formatShortClock(tr.startedAt || tr.updatedAt);
                  const statusNote = tr.running
                    ? (isZh ? '进行中' : 'Live')
                    : tr.status === 'failed'
                      ? (isZh ? '失败' : 'Failed')
                      : '';
                  return (
                    <button
                      key={tr.id}
                      type="button"
                      className={`${styles.callHistoryChip} ${active ? styles.callHistoryChipActive : ''}`}
                      onClick={() => store.selectTrace(tr.id)}
                      title={`${roleLabel(tr.role, isZh, tr.scope, tr.sessionTitle)} · ${tr.id}`}
                    >
                      <span className={`${styles.statusDot} ${dotCls}`} />
                      <span>{isZh ? `第 ${idx + 1} 轮` : `#${idx + 1}`}</span>
                      {packet && (
                        <span style={{ opacity: 0.78 }}>{roleLabel(tr.role, isZh, tr.scope, tr.sessionTitle)}</span>
                      )}
                      {clock && <span style={{ opacity: 0.65, fontFamily: 'var(--fw-mono)', fontSize: 10.5 }}>{clock}</span>}
                      {statusNote && (
                        <span style={{ fontSize: 10.5, fontWeight: 600, color: tr.running ? 'var(--fw-accent)' : 'var(--fw-danger)' }}>
                          {statusNote}
                        </span>
                      )}
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {/* Fallback prompt summary ONLY when native session transcript has not emitted real user_prompt block(s) yet */}
          {initialPromptText && !(trace?.blocks?.some(b => b.kind === 'user_prompt')) && (
            <div className={styles.userBubbleRow}>
              <span className={styles.userBubbleLabel}>
                {isZh ? '任务上下文摘要' : t('chat.sub.promptLabel')}
              </span>
              <div className={styles.userBubble}>
                <div>{initialPromptText}</div>
                {packet && (packet.writeScopes?.length || packet.write_scopes?.length) ? (
                  <div className={styles.userBubbleMeta}>
                    <span style={{ fontSize: 11, color: 'var(--fw-text-subtle)', marginRight: 2 }}>
                      {isZh ? '允许写入范围:' : 'Write scope:'}
                    </span>
                    {(packet.writeScopes ?? packet.write_scopes ?? []).slice(0, 6).map(sc => (
                      <span key={sc} className={styles.pill}>{sc}</span>
                    ))}
                  </div>
                ) : null}
              </div>
            </div>
          )}

          {/* Left-aligned Borderless Model Turn (1:1 DSH Chat Style) */}
          <div className={styles.modelTurn}>
            <div className={styles.modelTurnHead}>
              <span className={styles.orchAvatar}>◎</span>
              <strong>{nodeDisplayName}</strong>
              {modelLabel && (
                <span className={`${styles.pill} ${styles.pillPurple}`} title={rawModelId || undefined}>
                  {modelLabel}
                </span>
              )}
              {trace?.running && (
                <span className={`${styles.pill} ${styles.pillRunning}`}>
                  <span className={styles.tabLiveDot} style={{ marginRight: 4 }} />
                  {t('traces.liveBadge')}
                </span>
              )}
              {trace?.durationMs ? (
                <span style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>{formatDuration(trace.durationMs)}</span>
              ) : null}
              {trace && (
                <button
                  type="button"
                  className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
                  style={{ marginLeft: 'auto' }}
                  onClick={() => setShowRaw(v => !v)}
                >
                  {t('traces.rawEvents')} ({trace.rawTail?.length ?? 0})
                </button>
              )}
            </div>

            {showRaw && trace ? (
              <pre className={styles.monoPre}>
                {trace.rawTail && trace.rawTail.length > 0 ? trace.rawTail.join('\n') : '—'}
              </pre>
            ) : !trace || trace.blocks.length === 0 ? (
              <div style={{ padding: '14px 2px', color: 'var(--fw-text-subtle)', fontSize: 13 }}>
                {trace?.errorSummary ? (
                  <div className={styles.traceBlockError} data-testid="sub-session-error">
                    <strong>{trace.errorCode || t('traces.error')}</strong>
                    <div>{trace.errorSummary}</div>
                  </div>
                ) : (
                  <span>{t('chat.sub.noMessages')}</span>
                )}
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                {(() => {
                  // Group consecutive thinking / tool_call / tool_result blocks into
                  // collapsible native-grade Thought & Tool Chains (auto-open while
                  // running, auto-collapsed once finished).
                  type RenderUnit =
                    | { kind: 'chain'; items: TraceDetail['blocks'] }
                    | { kind: 'block'; block: TraceDetail['blocks'][number] };
                  const units: RenderUnit[] = [];
                  for (const block of trace.blocks) {
                    const isChainable =
                      block.kind === 'thinking' ||
                      block.kind === 'tool_call' ||
                      block.kind === 'tool_result' ||
                      block.kind === 'progress';
                    if (isChainable) {
                      const prev = units[units.length - 1];
                      if (prev && prev.kind === 'chain') prev.items.push(block);
                      else units.push({ kind: 'chain', items: [block] });
                    } else {
                      units.push({ kind: 'block', block });
                    }
                  }
                  const hasAssistantOutput = trace.blocks.some(b => b.kind === 'assistant');

                  return units.map((unit, idx) => {
                    if (unit.kind === 'chain') {
                      return (
                        <ThoughtChainDisclosure
                          key={`chain-${trace.id}-${idx}`}
                          items={unit.items}
                          idx={idx}
                          running={Boolean(trace.running)}
                          hasAssistantOutput={hasAssistantOutput}
                          isZh={isZh}
                        />
                      );
                    }

                    const block = unit.block;
                    if (block.kind === 'user_prompt') {
                      const writeScopes = idx === 0 ? (packet?.writeScopes ?? packet?.write_scopes ?? undefined) : undefined;
                      return (
                        <UserPromptBubble
                          key={`prompt-${idx}`}
                          text={block.text}
                          ts={block.ts}
                          writeScopes={writeScopes}
                          isZh={isZh}
                        />
                      );
                    }
                    if (block.kind === 'error') {
                      return (
                        <div key={idx} className={styles.traceBlockError}>
                          <strong>{block.title || t('traces.error')}</strong>
                          <pre className={styles.monoPre} style={{ background: 'transparent', borderColor: 'var(--fw-danger-border)', color: 'inherit' }}>
                            {block.text}
                          </pre>
                        </div>
                      );
                    }
                    if (block.kind === 'steps') {
                      const stepItems = Array.isArray(block.items) && block.items.length > 0 ? block.items : [block.text];
                      return (
                        <details key={idx} className={styles.stepsDisclosure} open={trace.running && !hasAssistantOutput}>
                          <summary className={styles.stepsSummary}>
                            <span className={styles.stepsDot} />
                            <span>{t('chat.sub.stepsTitle', { count: stepItems.length })}</span>
                          </summary>
                          <ul className={styles.stepsList}>
                            {stepItems.map((item, i) => (
                              <li key={i}>{item}</li>
                            ))}
                          </ul>
                        </details>
                      );
                    }
                    if (block.kind === 'progress') {
                      return (
                        <div key={idx} style={{ fontSize: 12, color: 'var(--fw-text-subtle)' }}>
                          · {block.title ? `[${block.title}] ` : ''}{block.text}
                        </div>
                      );
                    }
                    return (
                      <div key={idx} style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                        <MarkdownView content={block.text} streaming={trace.running} />
                      </div>
                    );
                  });
                })()}
              </div>
            )}

            {trace?.errorSummary && trace.blocks.length > 0 && (
              <div className={styles.traceBlockError} data-testid="sub-session-error">
                <strong>{trace.errorCode || t('traces.error')}</strong>
                <div>{trace.errorSummary}</div>
              </div>
            )}
          </div>

          {/* Human-in-the-loop follow-up turns from session history */}
          {historyTurns.map((turn, idx) => {
            const isUser = turn.role === 'user' || Boolean(turn.prompt);
            const content = turn.prompt || turn.text || turn.summary || '';
            if (!content) return null;
            if (isUser) {
              return (
                <div key={turn.id || `hist-${idx}`} className={styles.userBubbleRow}>
                  <span className={styles.userBubbleLabel}>{t('chat.sub.userLabel')}</span>
                  <div className={styles.userBubble}>{content}</div>
                </div>
              );
            }
            return (
              <div key={turn.id || `hist-${idx}`} className={styles.modelTurn}>
                <MarkdownView content={content} />
              </div>
            );
          })}
        </div>
      </div>

      {showSubScrollBottom && (
        <div className={styles.scrollBottomAnchor}>
          <button
            type="button"
            className={styles.scrollBottomBtn}
            data-testid="flywheel-sub-scroll-bottom"
            onClick={() => scrollSubToBottom('smooth')}
          >
            ↓ {isZh ? '跳到最新' : 'Jump to latest'}
          </button>
        </div>
      )}

      {/* Floating Centered DSH Composer (1:1 Single-Button + Real Local Upload/Paste/Drop) */}
      <div className={styles.composerDock}>
        <input
          ref={fileInputRef}
          type="file"
          accept="image/png,image/jpeg,image/webp,image/gif"
          multiple
          style={{ display: 'none' }}
          onChange={e => {
            if (e.target.files && e.target.files.length > 0) {
              void handleUploadFiles(e.target.files);
              e.target.value = '';
            }
          }}
        />
        <div
          className={`${styles.composerBox}${dragging ? ` ${styles.composerBoxDrag}` : ''}`}
          onDragOver={e => {
            e.preventDefault();
            if (!dragging) setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={handleDrop}
        >
          {dragging && (
            <div style={{ fontSize: 12, color: 'var(--fw-accent)', textAlign: 'center', padding: '4px 0' }}>
              {t('chat.composer.dropHint')}
            </div>
          )}

          {showRepoImages && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, paddingBottom: 6, borderBottom: '1px solid var(--fw-border)' }}>
              {snap.repoImages.length === 0 ? (
                <span style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>{t('create.imagesEmpty')}</span>
              ) : (
                snap.repoImages.slice(0, 12).map(img => (
                  <button
                    key={img.path}
                    type="button"
                    className={styles.jevDataChip}
                    onClick={() => {
                      setDraft(prev => (prev.trim() ? `${prev.trim()} [${img.path}]` : `[${img.path}] `));
                      setShowRepoImages(false);
                    }}
                  >
                    🖼 {img.path}
                  </button>
                ))
              )}
            </div>
          )}

          <textarea
            className={styles.composerTextarea}
            data-testid="flywheel-sub-composer"
            rows={2}
            value={draft}
            onChange={e => setDraft(e.target.value)}
            onPaste={handlePaste}
            onKeyDown={e => {
              if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                e.preventDefault();
                void handleSend();
              }
            }}
            placeholder={t('chat.composer.subPlaceholder', { name: nodeDisplayName, provider: providerLabel })}
          />
          <div className={styles.composerBar}>
            <div className={styles.composerTools} style={{ position: 'relative' }}>
              <button
                type="button"
                className={styles.composerPlusBtn}
                title={t('chat.composer.imagesBtn')}
                aria-label={t('chat.composer.imagesBtn')}
                onClick={() => setAttachMenuOpen(v => !v)}
              >
                +
              </button>
              {attachMenuOpen && (
                <div className={styles.attachMenu}>
                  <button
                    type="button"
                    className={styles.attachMenuItem}
                    onClick={() => {
                      setAttachMenuOpen(false);
                      fileInputRef.current?.click();
                    }}
                  >
                    🖼 {t('chat.composer.uploadLocal')}
                  </button>
                  <button
                    type="button"
                    className={styles.attachMenuItem}
                    onClick={() => {
                      setAttachMenuOpen(false);
                      setShowRepoImages(v => !v);
                      void store.loadRepoImages(snap.selectedCwd || undefined);
                    }}
                  >
                    📁 {t('chat.composer.scanRepo')}
                  </button>
                </div>
              )}
              {snap.running && (
                <span style={{ color: 'var(--fw-warn)' }}>{t('chat.composer.interruptHint')}</span>
              )}
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <span style={{ fontSize: 12, color: 'var(--fw-text-muted)' }} title={rawModelId || undefined}>
                {modelLabel || providerLabel}
              </span>
              {snap.running && !draft.trim() ? (
                <button
                  type="button"
                  className={styles.composerStopBtn}
                  title={t('header.cta.stop')}
                  aria-label={t('header.cta.stop')}
                  disabled={snap.busyAction !== null}
                  onClick={() => void store.stopRun()}
                >
                  ■
                </button>
              ) : (
                <button
                  type="button"
                  className={styles.composerSendBtn}
                  data-testid="flywheel-sub-send"
                  title={t('chat.composer.send')}
                  aria-label={t('chat.composer.send')}
                  disabled={!draft.trim() || (!sessionKey && !resolvedPlanning) || snap.busyAction !== null}
                  onClick={() => void handleSend()}
                >
                  ↑
                </button>
              )}
            </div>
          </div>
        </div>
        <SubSessionTelemetryDock trace={trace} state={snap.state} siblingCount={matchingTraces.length} isZh={isZh} />
      </div>
    </div>
  );
}
