import { useEffect, useRef, useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { formatEffortLabel, formatModelBadge } from '../planning.ts';
import type { TaskState, TraceDetail, TraceSummary } from '../types.ts';

interface UsageNumbers {
  input: number;
  output: number;
  reasoning: number;
  cacheRead: number;
  cacheWrite: number;
  total: number;
  hasCacheRead: boolean;
}

function readNum(obj: Record<string, unknown> | null | undefined, ...keys: string[]): number | null {
  if (!obj || typeof obj !== 'object') return null;
  for (const k of keys) {
    const v = obj[k];
    if (typeof v === 'number' && Number.isFinite(v)) return v;
  }
  return null;
}

function extractUsage(raw: Record<string, unknown> | null | undefined): UsageNumbers | null {
  if (!raw || typeof raw !== 'object') return null;
  const input = readNum(raw, 'input', 'input_tokens', 'inputTokens') ?? 0;
  const output = readNum(raw, 'output', 'output_tokens', 'outputTokens') ?? 0;
  const reasoning = readNum(raw, 'reasoning', 'reasoning_tokens', 'reasoningTokens') ?? 0;
  const cacheReadRaw = readNum(raw, 'cache_read', 'cacheRead', 'cache-read', 'cached_input_tokens');
  const cacheWrite = readNum(raw, 'cache_write', 'cacheWrite', 'cache-write') ?? 0;
  const totalRaw = readNum(raw, 'total_tokens', 'totalTokens', 'total-tokens');
  const total = totalRaw ?? input + output;
  if (input === 0 && output === 0 && total === 0 && cacheReadRaw === null) return null;
  return {
    input,
    output,
    reasoning,
    cacheRead: cacheReadRaw ?? 0,
    cacheWrite,
    total,
    hasCacheRead: cacheReadRaw !== null && input > 0,
  };
}

function formatCompactTokens(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0';
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`;
  if (n >= 10_000) return `${(n / 1000).toFixed(1)}K`;
  if (n >= 1000) return `${(n / 1000).toFixed(2)}K`;
  return String(Math.round(n));
}

function formatBytes(bytes?: number | null): string {
  if (typeof bytes !== 'number' || !Number.isFinite(bytes) || bytes <= 0) return '—';
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${bytes} B`;
}

function formatDurationCompact(ms?: number | null): string {
  if (typeof ms !== 'number' || !Number.isFinite(ms) || ms < 0) return '';
  if (ms < 1000) return `${Math.round(ms)}ms`;
  const sec = ms / 1000;
  if (sec < 60) return `${sec.toFixed(1)}s`;
  const min = Math.floor(sec / 60);
  const rem = Math.round(sec % 60);
  return `${min}m${String(rem).padStart(2, '0')}s`;
}

function GaugeIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m12 14 4-4" />
      <path d="M3.34 19a10 10 0 1 1 17.32 0" />
    </svg>
  );
}

function DatabaseIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <ellipse cx="12" cy="5" rx="9" ry="3" />
      <path d="M3 5v14a9 3 0 0 0 18 0V5" />
      <path d="M3 12a9 3 0 0 0 18 0" />
    </svg>
  );
}

function ContextRingIcon({ pct, tone }: { pct: number | null; tone?: 'accent' | 'warn' | 'danger' }) {
  const clamped = pct === null ? 0 : Math.max(0, Math.min(100, pct));
  const r = 5.5;
  const circ = 2 * Math.PI * r;
  const dashOffset = circ * (1 - clamped / 100);
  const strokeColor =
    pct === null
      ? 'currentColor'
      : tone === 'danger'
        ? 'var(--fw-danger)'
        : tone === 'warn'
          ? 'var(--fw-warn)'
          : tone === 'accent'
            ? 'var(--fw-accent)'
            : clamped >= 85
              ? 'var(--fw-danger)'
              : clamped >= 65
                ? 'var(--fw-warn)'
                : 'var(--fw-accent)';
  return (
    <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true" style={{ flexShrink: 0 }}>
      <circle cx="7" cy="7" r={r} fill="none" stroke="currentColor" strokeOpacity="0.22" strokeWidth="1.6" />
      {pct !== null && (
        <circle
          cx="7"
          cy="7"
          r={r}
          fill="none"
          stroke={strokeColor}
          strokeWidth="1.7"
          strokeLinecap="round"
          strokeDasharray={circ.toFixed(2)}
          strokeDashoffset={dashOffset.toFixed(2)}
          transform="rotate(-90 7 7)"
        />
      )}
    </svg>
  );
}

type PillId = 'p1' | 'p2' | 'p3';

interface PillConfig {
  id: PillId;
  testId: string;
  icon: JSX.Element;
  label: string;
  accentTone?: 'good' | 'warn' | 'danger' | 'neutral';
  popoverTitle: string;
  rows: Array<{ label: string; value: string; mono?: boolean; tone?: 'good' | 'warn' | 'danger' }>;
  footnote?: string;
}

function TelemetryDockBar({ pills, testId }: { pills: PillConfig[]; testId: string }) {
  const [hoveredId, setHoveredId] = useState<PillId | null>(null);
  const [pinnedId, setPinnedId] = useState<PillId | null>(null);
  const dockRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!pinnedId) return;
    const onOutside = (e: MouseEvent) => {
      if (dockRef.current && !dockRef.current.contains(e.target as Node)) {
        setPinnedId(null);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setPinnedId(null);
    };
    document.addEventListener('mousedown', onOutside);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onOutside);
      document.removeEventListener('keydown', onKey);
    };
  }, [pinnedId]);

  const activeId = pinnedId ?? hoveredId;

  return (
    <div className={styles.telemetryDock} ref={dockRef} data-testid={testId}>
      {pills.map(pill => {
        const isOpen = activeId === pill.id;
        return (
          <div
            key={pill.id}
            className={styles.telemetryPillWrap}
            onMouseEnter={() => setHoveredId(pill.id)}
            onMouseLeave={() => setHoveredId(prev => (prev === pill.id ? null : prev))}
          >
            <button
              type="button"
              className={`${styles.telemetryPill}${isOpen ? ` ${styles.telemetryPillActive}` : ''}${
                pill.accentTone === 'good'
                  ? ` ${styles.telemetryPillGood}`
                  : pill.accentTone === 'warn'
                    ? ` ${styles.telemetryPillWarn}`
                    : pill.accentTone === 'danger'
                      ? ` ${styles.telemetryPillDanger}`
                      : ''
              }`}
              data-testid={pill.testId}
              aria-haspopup="dialog"
              aria-expanded={isOpen}
              onClick={() => setPinnedId(prev => (prev === pill.id ? null : pill.id))}
            >
              {pill.icon}
              <span>{pill.label}</span>
            </button>
            {isOpen && (
              <div
                className={styles.telemetryPopover}
                data-testid={`${pill.testId}-popover`}
                role="dialog"
                aria-label={pill.popoverTitle}
              >
                <div className={styles.telemetryPopoverTitle}>{pill.popoverTitle}</div>
                <div className={styles.telemetryPopoverRows}>
                  {pill.rows.map((row, idx) => (
                    <div key={idx} className={styles.telemetryPopoverRow}>
                      <span className={styles.telemetryPopoverKey}>{row.label}</span>
                      <span
                        className={`${styles.telemetryPopoverVal}${row.mono ? ` ${styles.telemetryMono}` : ''}${
                          row.tone === 'good'
                            ? ` ${styles.telemetryValGood}`
                            : row.tone === 'warn'
                              ? ` ${styles.telemetryValWarn}`
                              : row.tone === 'danger'
                                ? ` ${styles.telemetryValDanger}`
                                : ''
                        }`}
                      >
                        {row.value}
                      </span>
                    </div>
                  ))}
                </div>
                {pill.footnote && <div className={styles.telemetryPopoverFoot}>{pill.footnote}</div>}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

export function SubSessionTelemetryDock({
  trace,
  state,
  siblingCount,
  isZh,
}: {
  trace: TraceDetail | TraceSummary | null;
  state?: TaskState | null;
  siblingCount: number;
  isZh: boolean;
}) {
  if (!trace) return null;

  const usage = extractUsage(trace.usage as Record<string, unknown> | null);
  const rawRecords = (state as { sessions?: { records?: Record<string, Record<string, unknown>> } } | null)?.sessions?.records;
  const sessionRec = trace.key && rawRecords ? rawRecords[trace.key] : null;
  const quality = (sessionRec?.quality as Record<string, unknown> | undefined) ?? null;
  const rawConfirmedTurns = readNum(quality, 'confirmed-turns', 'confirmedTurns');
  const confirmedTurns = rawConfirmedTurns ?? (trace.running ? Math.max(0, siblingCount - 1) : Math.max(1, siblingCount));
  const sessionAction = typeof sessionRec?.action === 'string' ? sessionRec.action : '';
  const contextMode = typeof (sessionRec?.context as Record<string, unknown> | undefined)?.mode === 'string'
    ? String((sessionRec?.context as Record<string, unknown>).mode)
    : '';

  const modelDisplay = formatModelBadge(trace.model, trace.thinking, isZh) || trace.harness || '—';
  const effortDisplay = formatEffortLabel(trace.thinking, isZh);
  const durStr = formatDurationCompact(trace.durationMs);
  const blocks = Array.isArray((trace as TraceDetail).blocks) ? (trace as TraceDetail).blocks : [];
  const stepCount = blocks.filter(b => b.kind === 'tool_call' || b.kind === 'thinking').length;

  // Pill 1: Turns, steps, duration (1:1 DSH native style — model badge already sits in composer bar above;
  // full model/effort/raw ID details appear in the upward popover). Never render "0 轮".
  const turnLabel = trace.running
    ? (isZh ? `第 ${confirmedTurns + 1} 轮` : `Turn ${confirmedTurns + 1}`)
    : (() => {
        const displayTurns = confirmedTurns > 0 ? confirmedTurns : Math.max(1, siblingCount);
        return isZh ? `${displayTurns} 轮` : `${displayTurns}t`;
      })();
  const p1Parts: string[] = [turnLabel];
  if (stepCount > 0) p1Parts.push(isZh ? `${stepCount} 步` : `${stepCount} steps`);
  if (durStr) p1Parts.push(durStr);

  // Pill 2: Tokens & Cache Hit Rate (truthful: never fabricate for providers without cacheRead)
  const hitRate = usage && usage.hasCacheRead ? (usage.cacheRead / usage.input) * 100 : null;
  const hitTone: 'good' | 'warn' | 'danger' | 'neutral' =
    hitRate === null ? 'neutral' : hitRate >= 90 ? 'good' : hitRate >= 60 ? 'warn' : 'danger';
  const p2Label =
    hitRate !== null
      ? `${formatCompactTokens(usage!.total)} · ${isZh ? '缓存命中' : 'Cache'} ${hitRate.toFixed(1)}%`
      : usage
        ? `${formatCompactTokens(usage.total)} · ${isZh ? '缓存 —' : 'Cache —'}`
        : isZh
          ? '缓存 —'
          : 'Cache —';

  // Pill 3: True single-turn provider context window occupancy (Codex token_count.last_token_usage,
  // Claude assistant message.usage, or ACP usage_update). Never fabricate occupancy from cumulative
  // multi-step billed input.
  const ctxUsage = trace.contextUsage && trace.contextUsage.size > 0 ? trace.contextUsage : null;
  const ctxUsed = ctxUsage ? ctxUsage.used : null;
  const ctxWindow = ctxUsage ? ctxUsage.size : null;
  const ctxSource = ctxUsage?.source ?? (ctxUsage ? 'acp' : null);
  const rawCtxPct = ctxUsed !== null && ctxWindow ? (ctxUsed / ctxWindow) * 100 : null;
  const ctxPct = rawCtxPct !== null ? Math.min(100, rawCtxPct) : null;
  const ringTone: 'accent' | 'warn' | 'danger' | undefined =
    ctxPct !== null && ctxPct >= 85
      ? 'danger'
      : ctxPct !== null && ctxPct >= 65
        ? 'warn'
        : 'accent';
  const p3Label =
    ctxUsed === null || rawCtxPct === null
      ? '—'
      : rawCtxPct > 0 && rawCtxPct < 1
        ? '<1%'
        : `${Math.round(ctxPct!)}%`;

  const lastTurnHitRate =
    ctxUsage && typeof ctxUsage.lastInput === 'number' && ctxUsage.lastInput > 0 && typeof ctxUsage.lastCacheRead === 'number'
      ? (ctxUsage.lastCacheRead / ctxUsage.lastInput) * 100
      : null;
  const lastTurnHitTone: 'good' | 'warn' | 'danger' | undefined =
    lastTurnHitRate === null ? undefined : lastTurnHitRate >= 90 ? 'good' : lastTurnHitRate >= 60 ? 'warn' : 'danger';
  const ctxSourceLabel =
    ctxSource === 'codex-last-turn'
      ? (isZh ? 'Codex 末步窗口' : 'Codex last-turn window')
      : ctxSource === 'claude-last-turn'
        ? (isZh ? 'Claude 末步窗口' : 'Claude last-turn window')
        : (isZh ? 'Provider 窗口 (ACP)' : 'Provider window (ACP)');

  const pills: PillConfig[] = [
    {
      id: 'p1',
      testId: 'telemetry-sub-model',
      icon: <GaugeIcon />,
      label: p1Parts.join(' · '),
      popoverTitle: isZh ? '当前节点会话与模型' : 'Node Session & Model',
      rows: [
        { label: isZh ? '模型' : 'Model', value: modelDisplay },
        { label: isZh ? '原始模型 ID' : 'Raw Model ID', value: trace.model || '—', mono: true },
        ...(effortDisplay ? [{ label: isZh ? '思考等级' : 'Reasoning Effort', value: effortDisplay }] : []),
        { label: isZh ? '运行时 / Provider' : 'Harness', value: trace.harness || '—', mono: true },
        { label: isZh ? '已确认轮次' : 'Confirmed Turns', value: String(confirmedTurns), mono: true },
        ...(sessionAction ? [{ label: isZh ? '会话动作' : 'Session Action', value: contextMode ? `${sessionAction} (${contextMode})` : sessionAction, mono: true }] : []),
        ...(trace.promptBytes ? [{ label: isZh ? '提示词体积' : 'Prompt Size', value: formatBytes(trace.promptBytes), mono: true }] : []),
        ...(durStr ? [{ label: isZh ? '本轮耗时' : 'Duration', value: durStr, mono: true }] : []),
      ],
    },
    {
      id: 'p2',
      testId: 'telemetry-sub-cache',
      icon: <DatabaseIcon />,
      label: p2Label,
      accentTone: hitTone,
      popoverTitle: isZh ? '本节点 Token 与缓存命中' : 'Node Tokens & Prompt Cache',
      rows: usage
        ? [
            {
              label: isZh ? '累计缓存命中率' : 'Cumulative Cache Hit',
              value: hitRate !== null ? `${hitRate.toFixed(1)}%` : (isZh ? '未上报' : 'Not reported'),
              mono: true,
              tone: hitTone === 'neutral' ? undefined : hitTone,
            },
            ...(lastTurnHitRate !== null
              ? [
                  {
                    label: isZh ? '末步缓存命中率' : 'Last-Turn Cache Hit',
                    value: `${lastTurnHitRate.toFixed(1)}% (${formatCompactTokens(ctxUsage!.lastCacheRead!)}/${formatCompactTokens(ctxUsage!.lastInput!)})`,
                    mono: true,
                    tone: lastTurnHitTone,
                  },
                ]
              : []),
            { label: isZh ? '总 Token' : 'Total Tokens', value: usage.total.toLocaleString(), mono: true },
            { label: isZh ? '累计输入 Token' : 'Cumulative Input', value: usage.input.toLocaleString(), mono: true },
            { label: isZh ? '缓存读取 (Cache Read)' : 'Cache Read', value: usage.hasCacheRead ? usage.cacheRead.toLocaleString() : '—', mono: true },
            ...(usage.cacheWrite > 0 ? [{ label: isZh ? '缓存写入 (Cache Write)' : 'Cache Write', value: usage.cacheWrite.toLocaleString(), mono: true }] : []),
            { label: isZh ? '输出 Token' : 'Output Tokens', value: usage.output.toLocaleString(), mono: true },
            ...(usage.reasoning > 0 ? [{ label: isZh ? '推理 Token' : 'Reasoning Tokens', value: usage.reasoning.toLocaleString(), mono: true }] : []),
          ]
        : [
            {
              label: isZh ? '用量状态' : 'Usage Status',
              value: trace.running ? (isZh ? '流式执行中，结束后汇总' : 'Streaming; summarized on exit') : (isZh ? 'Provider 未上报用量' : 'Not reported by provider'),
            },
          ],
      footnote: usage
        ? (isZh
            ? '单次调用包含多步工具交互时，总输入为各步累计计费 Token；单步真实上下文窗口占用见右侧圆环。'
            : 'Cumulative input sums all tool steps in this call; see the ring pill on the right for single-turn window occupancy.')
        : undefined,
    },
    {
      id: 'p3',
      testId: 'telemetry-sub-context',
      icon: <ContextRingIcon pct={ctxPct} tone={ringTone} />,
      label: p3Label,
      popoverTitle: isZh ? '上下文窗口占用' : 'Context Window Occupancy',
      rows:
        ctxUsed !== null && ctxWindow
          ? [
              {
                label: isZh ? '当前窗口占用' : 'Window Occupancy',
                value: `${formatCompactTokens(ctxUsed)} / ${formatCompactTokens(ctxWindow)} (${ctxPct!.toFixed(1)}%)`,
                mono: true,
              },
              {
                label: isZh ? '数据来源' : 'Source',
                value: ctxSourceLabel,
              },
            ]
          : [
              {
                label: isZh ? '当前窗口占用' : 'Window Occupancy',
                value: isZh ? 'Provider 未上报单步窗口' : 'Not reported by provider',
              },
            ],
      footnote:
        ctxUsed !== null && ctxWindow
          ? (isZh
              ? '反映 Provider 最后一次推理请求的单步真实上下文占用；若 Provider 发生上下文压缩，该占比会自动回落。'
              : 'Shows true single-turn context occupancy on the latest request; drops automatically after provider compaction.')
          : (isZh
              ? '该运行时未暴露单步上下文窗口大小；多步累计计费输入见左侧缓存统计，避免将累计输入误算作窗口占用。'
              : 'This runtime does not expose single-turn window size; cumulative multi-step tokens remain in the cache pill.'),
    },
  ];

  return <TelemetryDockBar pills={pills} testId="flywheel-sub-telemetry-dock" />;
}

export function MainStreamTelemetryDock({
  state,
  traces,
  maxTaskCalls,
  isZh,
}: {
  state?: TaskState | null;
  traces: TraceSummary[];
  maxTaskCalls?: number | null;
  isZh: boolean;
}) {
  const budget = (state as {
    budget?: {
      totals?: { calls?: number; tokens?: number; 'cost-usd'?: number; input?: number; output?: number; 'cache-read'?: number; 'cache-write'?: number };
      'unknown-usage-calls'?: number;
      cache?: {
        'by-provider'?: Record<string, { calls?: number; input?: number; 'cache-read'?: number; 'cache-write'?: number; 'hit-rate'?: number }>;
      };
    };
  } | null)?.budget;

  const totals = budget?.totals;
  const byProvider = budget?.cache?.['by-provider'] ?? {};

  let totalCalls = totals?.calls ?? traces.length;
  let totalTokens = totals?.tokens ?? 0;
  let totalInput = 0;
  let totalCacheRead = 0;
  let totalCacheWrite = 0;
  let totalOutput = 0;
  let hasAnyCacheMetric = false;

  const providerRows: Array<{ label: string; value: string; mono?: boolean; tone?: 'good' | 'warn' | 'danger' }> = [];
  for (const [prov, entry] of Object.entries(byProvider)) {
    const inp = entry?.input ?? 0;
    const cr = entry?.['cache-read'] ?? 0;
    const cw = entry?.['cache-write'] ?? 0;
    const calls = entry?.calls ?? 0;
    totalInput += inp;
    totalCacheRead += cr;
    totalCacheWrite += cw;
    if (inp > 0) {
      hasAnyCacheMetric = true;
      const rate = typeof entry?.['hit-rate'] === 'number' ? entry['hit-rate'] * 100 : (cr / inp) * 100;
      const effectiveCachedPct = ((cr + cw) / inp) * 100;
      const isColdStartWarming =
        cw > 0 &&
        effectiveCachedPct >= 85 &&
        (calls > 0 ? calls <= 2 : cw / inp >= 0.25);
      const tone: 'good' | 'warn' | 'danger' | undefined =
        rate >= 90 ? 'good' : isColdStartWarming ? undefined : rate >= 60 ? 'warn' : 'danger';
      const callsLabel = calls > 0 ? (isZh ? `${calls}次 · ` : `${calls}× · `) : '';
      const writePart = cw > 0 ? (isZh ? ` · 写 ${formatCompactTokens(cw)}` : ` · write ${formatCompactTokens(cw)}`) : '';
      const warmTag = isColdStartWarming ? (isZh ? ' [预热]' : ' [warming]') : '';
      providerRows.push({
        label: prov,
        value: `${callsLabel}${rate.toFixed(1)}% (${formatCompactTokens(cr)}/${formatCompactTokens(inp)})${writePart}${warmTag}`,
        mono: true,
        tone,
      });
    }
  }

  // Fallback aggregation from live traces when state.budget.cache has not populated yet.
  if (!hasAnyCacheMetric && traces.length > 0) {
    for (const tr of traces) {
      const u = extractUsage(tr.usage as Record<string, unknown> | null);
      if (!u) continue;
      totalTokens = Math.max(totalTokens, totalTokens + (totals?.tokens ? 0 : u.total));
      totalInput += u.input;
      totalOutput += u.output;
      if (u.hasCacheRead) {
        hasAnyCacheMetric = true;
        totalCacheRead += u.cacheRead;
        totalCacheWrite += u.cacheWrite;
      }
    }
  } else {
    totalOutput = totals?.output ?? 0;
  }

  if (totalCalls === 0 && totalTokens === 0 && traces.length === 0) return null;

  const planRoles = new Set(['clarify', 'design-plan', 'design-review', 'ui-prototype', 'ui-prototype-review', 'plan', 'plan-review']);
  const planCalls = traces.filter(tr => planRoles.has(tr.role)).length;
  const execCalls = traces.filter(tr => tr.role === 'execute' || tr.role === 'review' || tr.role === 'frontend-ux-review' || tr.role === 'judge').length;

  const overallHit = hasAnyCacheMetric && totalInput > 0 ? (totalCacheRead / totalInput) * 100 : null;
  const hitTone: 'good' | 'warn' | 'danger' | 'neutral' =
    overallHit === null ? 'neutral' : overallHit >= 90 ? 'good' : overallHit >= 60 ? 'warn' : 'danger';

  // Pill 3 on Main Stream reflects task call guardrail headroom (not a single-node context window).
  const configuredGuardrail =
    typeof maxTaskCalls === 'number' && maxTaskCalls > 0
      ? maxTaskCalls
      : totalCalls > 60
        ? Math.max(300, Math.ceil(totalCalls / 100) * 100)
        : 60;
  const guardrailPct = Math.min(100, (totalCalls / configuredGuardrail) * 100);
  const guardrailTone: 'accent' | 'warn' | 'danger' =
    guardrailPct >= 90 ? 'danger' : guardrailPct >= 75 ? 'warn' : 'accent';
  const guardrailDetail = `${totalCalls} / ${configuredGuardrail} (${guardrailPct.toFixed(1)}%)`;

  const pills: PillConfig[] = [
    {
      id: 'p1',
      testId: 'telemetry-main-calls',
      icon: <GaugeIcon />,
      label: isZh ? `${totalCalls} 次调用` : `${totalCalls} calls`,
      popoverTitle: isZh ? '任务调用分布' : 'Task Invocation Split',
      rows: [
        { label: isZh ? '累计调用次数' : 'Total Calls', value: String(totalCalls), mono: true },
        { label: isZh ? '规划 / 审查调用' : 'Planning / Review', value: String(planCalls), mono: true },
        { label: isZh ? '执行 / 代码审查' : 'Execute / Review', value: String(execCalls), mono: true },
        ...(budget?.['unknown-usage-calls']
          ? [{ label: isZh ? '未上报用量调用' : 'Unknown Usage Calls', value: String(budget['unknown-usage-calls']), mono: true }]
          : []),
      ],
    },
    {
      id: 'p2',
      testId: 'telemetry-main-cache',
      icon: <DatabaseIcon />,
      label:
        overallHit !== null
          ? `${formatCompactTokens(totalTokens)} · ${isZh ? '缓存命中' : 'Cache'} ${overallHit.toFixed(1)}%`
          : `${formatCompactTokens(totalTokens)} · ${isZh ? '缓存 —' : 'Cache —'}`,
      accentTone: hitTone,
      popoverTitle: isZh ? '任务总用量与分 Provider 缓存命中' : 'Task Usage & Per-Provider Cache Hit',
      rows: [
        {
          label: isZh ? '加权缓存命中率' : 'Weighted Cache Hit',
          value: overallHit !== null ? `${overallHit.toFixed(1)}%` : (isZh ? '暂无缓存统计' : 'No cache stats'),
          mono: true,
          tone: hitTone === 'neutral' ? undefined : hitTone,
        },
        { label: isZh ? '总 Token' : 'Total Tokens', value: totalTokens.toLocaleString(), mono: true },
        ...(totalInput > 0 ? [{ label: isZh ? '输入 / 缓存读取' : 'Input / Cache Read', value: `${formatCompactTokens(totalInput)} / ${formatCompactTokens(totalCacheRead)}`, mono: true }] : []),
        ...(totalCacheWrite > 0 ? [{ label: isZh ? '缓存写入' : 'Cache Write', value: formatCompactTokens(totalCacheWrite), mono: true }] : []),
        ...(totalOutput > 0 ? [{ label: isZh ? '输出 Token' : 'Output Tokens', value: formatCompactTokens(totalOutput), mono: true }] : []),
        ...(typeof totals?.['cost-usd'] === 'number' && totals['cost-usd'] > 0
          ? [{ label: isZh ? '估算费用' : 'Estimated Cost', value: `$${totals['cost-usd'].toFixed(4)}`, mono: true }]
          : []),
        ...providerRows,
      ],
    },
    {
      id: 'p3',
      testId: 'telemetry-main-context',
      icon: <ContextRingIcon pct={guardrailPct} tone={guardrailTone} />,
      label: `${Math.round(guardrailPct)}%`,
      popoverTitle: isZh ? '任务护栏与上下文管理' : 'Task Guardrail & Context Policy',
      rows: [
        { label: isZh ? '调用护栏使用' : 'Call Guardrail', value: guardrailDetail, mono: true },
        { label: isZh ? '上下文管理' : 'Context Compaction', value: isZh ? '由 Provider 自动压缩' : 'Provider auto-compact' },
      ],
      footnote: isZh
        ? '默认遵循各 Provider 自身上下文压缩与缓存复用；如需硬性上限，可在配置中显式设置 max-run-input-tokens。'
        : 'Uses provider-native compaction and prompt caching by default; configure max-run-input-tokens explicitly to enforce a hard cap.',
    },
  ];

  return <TelemetryDockBar pills={pills} testId="flywheel-main-telemetry-dock" />;
}
