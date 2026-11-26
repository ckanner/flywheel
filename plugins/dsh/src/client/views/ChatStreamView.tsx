import { type ClipboardEvent, type DragEvent, useMemo, useRef, useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { MainStreamTelemetryDock } from '../components/ComposerTelemetryDock.tsx';
import { HtmlPreviewFrame } from '../components/HtmlPreviewFrame.tsx';
import { formatModelBadge, normalizePackets, resolveCandidateLabels, resolvePlanningView } from '../planning.ts';
import { useSmartAutoScroll } from '../scroll.ts';
import type { FlywheelSnapshot, FlywheelStore } from '../store.ts';
import type { PacketNode, TraceSummary, Translate } from '../types.ts';

type NewTaskIntent = 'build' | 'quick' | 'investigate';
interface AttachedImage {
  path: string;
  previewUrl?: string;
}

interface MentionTarget {
  id: string;
  group: 'packets' | 'sessions';
  label: string;
  subtitle: string;
  dotClass: string;
  sessionKey: string;
  traceId?: string | null;
  packetId?: string | null;
  pillText: string;
  searchKey: string;
}

const ROLE_LABELS_ZH: Record<string, string> = {
  clarify: 'Stage 0 需求澄清',
  'design-plan': '架构设计主稿',
  'design-review': '设计架构审查',
  'ui-prototype': 'UI/UX 交互原型',
  'ui-prototype-review': 'UI/UX 原型审查',
  plan: '实施计划拆解',
  'plan-review': '实施计划审查',
  execute: '节点执行',
  review: '代码审查',
  'frontend-ux-review': '前端 UX 审查',
  judge: '里程碑裁决',
  investigate: '只读根因调查',
};

function roleLabel(role: string, isZh: boolean, scope?: string, sessionTitle?: string, roundIdx?: number): string {
  if (role === 'design-plan') {
    if (scope?.endsWith(':secondary')) return isZh ? '架构设计对照稿' : 'Secondary Design Draft';
    if (scope?.endsWith(':synthesize') || sessionTitle?.includes(':synthesize')) return isZh ? '双稿统稿合并' : 'Design Synthesis';
    if (scope?.endsWith(':primary')) return isZh ? '架构设计主稿' : 'Primary Design Draft';
    if (typeof roundIdx === 'number' && roundIdx > 1) return isZh ? '按审修订设计稿' : 'Design Revision';
  }
  if (!isZh) return role;
  return ROLE_LABELS_ZH[role] ?? role;
}

function formatClock(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso.slice(11, 19);
  return d.toTimeString().slice(0, 8);
}

function formatDurationShort(ms?: number | null): string {
  if (typeof ms !== 'number' || !Number.isFinite(ms) || ms < 0) return '';
  if (ms < 1000) return `${Math.round(ms)}ms`;
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  return `${Math.floor(s / 60)}m${String(s % 60).padStart(2, '0')}s`;
}

function humanizeBlockedKey(key: string, isZh: boolean): string {
  if (key.startsWith('design-plan/')) return isZh ? '架构方案设计' : 'Architecture Design';
  if (key.startsWith('design-review/')) return isZh ? '设计独立审查' : 'Design Review';
  if (key.startsWith('plan/')) return isZh ? '里程碑实施计划拆解' : 'Milestone Plan Refinement';
  if (key.startsWith('plan-review/')) return isZh ? '实施计划独立审查' : 'Plan Review';
  if (key.startsWith('execute/')) return isZh ? `执行节点 ${key.slice(8)}` : `Execution ${key.slice(8)}`;
  if (key.startsWith('review/')) return isZh ? `代码审查 ${key.slice(7)}` : `Code Review ${key.slice(7)}`;
  return key;
}

interface TimelineRound {
  round: number;
  label: string;
  items: TraceSummary[];
  running: boolean;
  totalDurationMs: number;
}

interface MidExecPlanRevisionGroup {
  scope: string;
  requestId: string;
  label: string;
  triggerPacketId: string | null;
  triggerSummary: string;
  items: TraceSummary[];
  running: boolean;
  totalDurationMs: number;
}

function parseReviewStepSignal(tr: TraceSummary): {
  tone: 'pass' | 'fail' | 'warn' | 'running' | 'neutral';
  icon: string;
  scoreLabel: string;
} {
  if (tr.running || tr.status === 'running') {
    return { tone: 'running', icon: '●', scoreLabel: '' };
  }
  const preview = tr.preview || '';
  const verdictMatch = /(?:\*\*)?审查结论(?:\*\*)?:\s*`?([a-z-]+)`?|"verdict"\s*:\s*"([^"]+)"/i.exec(preview);
  const verdict = (verdictMatch?.[1] || verdictMatch?.[2] || '').toLowerCase();
  const scoreMatch = /(?:\*\*)?评分(?:\*\*)?:\s*`?([0-9.]+)`?|"score"\s*:\s*([0-9.]+)/.exec(preview);
  const score = scoreMatch?.[1] || scoreMatch?.[2] || '';
  if (tr.role === 'review' || tr.role === 'frontend-ux-review') {
    if (verdict === 'pass') return { tone: 'pass', icon: '✓', scoreLabel: score };
    if (verdict === 'verification-gap') return { tone: 'warn', icon: '△', scoreLabel: score };
    if (verdict === 'return' || verdict === 'needs_polish' || verdict === 'fail') {
      return { tone: 'fail', icon: '✗', scoreLabel: score };
    }
  }
  if (tr.status === 'completed') return { tone: 'pass', icon: '✓', scoreLabel: score };
  if (tr.status === 'failed' || tr.status === 'blocked') return { tone: 'fail', icon: '✗', scoreLabel: score };
  if (tr.status === 'cancelled') return { tone: 'warn', icon: '⊘', scoreLabel: score };
  return { tone: 'neutral', icon: '·', scoreLabel: score };
}

function summarizePlanRevisionFeedback(rawFeedback?: string, isZh = true): { packetId: string | null; summary: string } {
  const text = (rawFeedback || '').trim();
  if (!text) return { packetId: null, summary: '' };
  const pktMatch = /\b(M\d+-T\d+)\b/.exec(text);
  const packetId = pktMatch ? pktMatch[1]! : null;
  if (text.includes('verification-gap') || text.includes('strengthen')) {
    return {
      packetId,
      summary: isZh
        ? `${packetId ? `${packetId} ` : ''}审查发现验收缺口（verification-gap），自动退回规划层补强 L1 验收契约`
        : `${packetId ? `${packetId} ` : ''}verification-gap detected — strengthening acceptance contract`,
    };
  }
  const returnedIdx = text.indexOf('Returned packet ');
  if (returnedIdx >= 0) {
    const reasonMatch = /"reasons"\s*:\s*\[\s*"([^"]{1,180})/.exec(text);
    const reason = reasonMatch ? reasonMatch[1]!.replace(/\\n/g, ' ').replace(/\\"/g, '"') : '';
    return {
      packetId,
      summary: reason
        ? `${isZh ? '触发原因' : 'Trigger'}：${reason}${reason.length >= 175 ? '…' : ''}`
        : isZh
          ? `${packetId ? `${packetId} ` : ''}退回规划层进行契约修订`
          : `${packetId ? `${packetId} ` : ''}returned to planner for contract revision`,
    };
  }
  return {
    packetId,
    summary: text.length > 180 ? `${text.slice(0, 180)}…` : text,
  };
}

function groupPlanningTracesIntoRounds(traces: TraceSummary[], isZh: boolean): TimelineRound[] {
  const chronological = [...traces].sort((a, b) => {
    const ta = Date.parse(a.startedAt || a.updatedAt || '') || 0;
    const tb = Date.parse(b.startedAt || b.updatedAt || '') || 0;
    return ta - tb;
  });
  const rounds: TimelineRound[] = [];
  let current: TimelineRound | null = null;
  let seenReviewInCurrent = false;
  for (const tr of chronological) {
    const isPlanPhase = tr.role === 'plan' || tr.role === 'plan-review';
    const isProtoPhase = tr.role === 'ui-prototype' || tr.role === 'ui-prototype-review';
    const isClarify = tr.role === 'clarify';
    const startsNewDesignRound =
      !isPlanPhase &&
      !isProtoPhase &&
      !isClarify &&
      tr.role === 'design-plan' &&
      !tr.scope?.endsWith(':secondary') &&
      !tr.scope?.endsWith(':synthesize') &&
      !tr.sessionTitle?.includes(':synthesize') &&
      seenReviewInCurrent;

    const bucketKind = isClarify ? 'clarify' : isProtoPhase ? 'proto' : isPlanPhase ? 'plan' : 'design';
    const prevKind = current
      ? (current.items[0]?.role === 'clarify'
          ? 'clarify'
          : current.items[0]?.role?.startsWith('ui-prototype')
            ? 'proto'
            : current.items[0]?.role === 'plan' || current.items[0]?.role === 'plan-review'
              ? 'plan'
              : 'design')
      : null;

    if (!current || startsNewDesignRound || prevKind !== bucketKind) {
      const roundNum = rounds.filter(r => r.label.includes('轮') || r.label.includes('Round')).length + 1;
      const label = isClarify
        ? (isZh ? 'Stage 0 · 需求澄清与质量指标' : 'Stage 0 · Clarification')
        : isProtoPhase
          ? (isZh ? 'Stage 1.5 · UI/UX 交互原型' : 'Stage 1.5 · UI/UX Prototype')
          : isPlanPhase
            ? (isZh ? '里程碑拆包与计划审查' : 'Milestone Plan & Review')
            : (isZh ? `第 ${roundNum} 轮 · 架构设计与审查` : `Round ${roundNum} · Design & Review`);
      current = { round: rounds.length + 1, label, items: [], running: false, totalDurationMs: 0 };
      rounds.push(current);
      seenReviewInCurrent = false;
    }
    current.items.push(tr);
    if (tr.running) current.running = true;
    if (typeof tr.durationMs === 'number' && Number.isFinite(tr.durationMs)) {
      current.totalDurationMs += tr.durationMs;
    }
    if (tr.role === 'design-review' || tr.role === 'plan-review') {
      seenReviewInCurrent = true;
    }
  }
  return rounds;
}

function packetTone(status?: string): { dot: string; pill: string; rowExtra: string } {
  const s = (status ?? '').toLowerCase();
  if (['done', 'completed', 'passed'].includes(s)) {
    return { dot: styles.statusDotSuccess, pill: styles.pillSuccess, rowExtra: '' };
  }
  if (['running', 'executing', 'reviewing'].includes(s)) {
    return { dot: styles.statusDotRunning, pill: styles.pillRunning, rowExtra: '' };
  }
  if (['failed', 'blocked'].includes(s)) {
    return { dot: styles.statusDotDanger, pill: styles.pillDanger, rowExtra: styles.subAgentRowCardDanger };
  }
  if (['escalated', 'inbox', 'stopped', 'awaiting_approval'].includes(s)) {
    return { dot: styles.statusDotWarn, pill: styles.pillWarn, rowExtra: styles.subAgentRowCardWarn };
  }
  return { dot: '', pill: '', rowExtra: '' };
}

function traceTone(status: string, running: boolean): { dot: string; pill: string } {
  if (running || status === 'running') return { dot: styles.statusDotRunning, pill: styles.pillRunning };
  if (status === 'completed') return { dot: styles.statusDotSuccess, pill: styles.pillSuccess };
  if (status === 'failed' || status === 'blocked') return { dot: styles.statusDotDanger, pill: styles.pillDanger };
  if (status === 'cancelled') return { dot: styles.statusDotWarn, pill: styles.pillWarn };
  return { dot: '', pill: '' };
}

function groupIntoWaves(packets: PacketNode[], declaredWaves?: string[][]): Array<{ index: number; items: PacketNode[] }> {
  const byId = new Map<string, PacketNode>();
  for (const p of packets) byId.set(p.id, p);

  if (Array.isArray(declaredWaves) && declaredWaves.length > 0) {
    const used = new Set<string>();
    const groups = declaredWaves
      .map((ids, idx) => {
        const items = ids.map(id => byId.get(id)).filter((p): p is PacketNode => Boolean(p));
        for (const p of items) used.add(p.id);
        return { index: idx + 1, items };
      })
      .filter(g => g.items.length > 0);
    const leftovers = packets.filter(p => !used.has(p.id));
    if (leftovers.length > 0) groups.push({ index: groups.length + 1, items: leftovers });
    return groups;
  }

  const depthCache = new Map<string, number>();
  const visiting = new Set<string>();
  const getDepth = (id: string): number => {
    if (depthCache.has(id)) return depthCache.get(id)!;
    if (visiting.has(id)) return 0;
    visiting.add(id);
    const pkt = byId.get(id);
    let d = typeof pkt?.wave === 'number' && Number.isInteger(pkt.wave) && pkt.wave >= 0 ? pkt.wave : 0;
    const deps = pkt?.dependsOn ?? pkt?.depends_on ?? [];
    for (const dep of deps) {
      if (byId.has(dep)) d = Math.max(d, getDepth(dep) + 1);
    }
    visiting.delete(id);
    depthCache.set(id, d);
    return d;
  };

  const buckets = new Map<number, PacketNode[]>();
  for (const p of packets) {
    const w = getDepth(p.id);
    const list = buckets.get(w) ?? [];
    list.push(p);
    buckets.set(w, list);
  }
  return [...buckets.keys()]
    .sort((a, b) => a - b)
    .map((key, idx) => ({
      index: idx + 1,
      items: buckets.get(key)!.sort((a, b) => String(a.id || '').localeCompare(String(b.id || ''))),
    }));
}

function generateSlug(goal: string): string {
  const ascii = goal
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 36);
  const stamp = new Date().toISOString().replace(/[-:T.Z]/g, '').slice(4, 12);
  return ascii ? `${ascii}-${stamp}` : `task-${stamp}`;
}

export function ChatStreamView({
  snap,
  store,
  t,
}: {
  snap: FlywheelSnapshot;
  store: FlywheelStore;
  t: Translate;
}) {
  const isNewTaskMode = snap.workbenchView === 'new-task' || !snap.selectedTaskDir;
  const isZh = t('tab.traces') === '运行记录';

  // New-task chat state
  const [intent, setIntent] = useState<NewTaskIntent>('build');
  const [planMode, setPlanMode] = useState<'auto' | 'full' | 'quick'>('auto');
  const [clarifyMode, setClarifyMode] = useState<'auto' | 'always' | 'off'>('auto');
  const [uiPrototypeMode, setUiPrototypeMode] = useState<'auto' | 'always' | 'off'>('auto');
  const [cwdOverride, setCwdOverride] = useState('');
  const [editingCwd, setEditingCwd] = useState(false);
  const [customSlug, setCustomSlug] = useState('');
  const [readPathsText, setReadPathsText] = useState('');
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [selectedImages, setSelectedImages] = useState<AttachedImage[]>([]);
  const [attachMenuOpen, setAttachMenuOpen] = useState(false);
  const [showImages, setShowImages] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [newGoal, setNewGoal] = useState('');
  const newFileInputRef = useRef<HTMLInputElement | null>(null);

  // Stage 0 Clarification & Stage 1.5 UI Prototype local state
  const [clarifyAnswers, setClarifyAnswers] = useState<Record<string, string>>({});
  const [selectedGates, setSelectedGates] = useState<Record<string, boolean>>({});
  const [customGateInput, setCustomGateInput] = useState('');
  const [clarifyNote, setClarifyNote] = useState('');
  const [clarifyProtoMode, setClarifyProtoMode] = useState<'auto' | 'always' | 'off'>('auto');
  const [protoFeedback, setProtoFeedback] = useState('');

  // Main stream composer state
  const [mainInput, setMainInput] = useState('');
  const [selectedMention, setSelectedMention] = useState<MentionTarget | null>(null);
  const [mentionQuery, setMentionQuery] = useState<string | null>(null);
  const [mentionActiveIdx, setMentionActiveIdx] = useState(0);
  const [mainAttachMenuOpen, setMainAttachMenuOpen] = useState(false);
  const [abandonReasons, setAbandonReasons] = useState<Record<string, string>>({});
  const [abandonErrors, setAbandonErrors] = useState<Record<string, string>>({});
  const mainFileInputRef = useRef<HTMLInputElement | null>(null);
  const mainTextareaRef = useRef<HTMLTextAreaElement | null>(null);

  const blockedSessions = useMemo(() => {
    const raw = (snap.state as { sessions?: { records?: Record<string, { reason?: string; last?: { id?: string; status?: string } }> } } | null)?.sessions?.records;
    if (!raw || typeof raw !== 'object') return [];
    return Object.entries(raw)
      .filter(([, rec]) => rec?.last?.status === 'blocked' && typeof rec.last.id === 'string' && rec.last.id)
      .map(([key, rec]) => ({
        key,
        invocation: rec.last!.id!,
        reason: rec.reason || 'dispatch, outcome, cleanup or terminal candidate is unconfirmed',
      }));
  }, [snap.state]);

  const effectiveCwd = cwdOverride.trim() || snap.selectedCwd || '';

  const readDataUrl = (file: File) =>
    new Promise<string>(resolve => {
      const reader = new FileReader();
      reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '');
      reader.onerror = () => resolve('');
      reader.readAsDataURL(file);
    });

  const handleUploadNewImages = async (files: FileList | File[]) => {
    if (!effectiveCwd) return;
    for (const file of Array.from(files)) {
      if (!file.type.startsWith('image/')) continue;
      if (selectedImages.length >= 4) break;
      const dataUrl = await readDataUrl(file);
      if (!dataUrl) continue;
      const saved = await store.uploadAttachment(effectiveCwd, {
        name: file.name || 'image.png',
        mimeType: file.type,
        dataBase64: dataUrl,
      });
      if (saved?.path) {
        setSelectedImages(prev =>
          prev.some(item => item.path === saved.path) || prev.length >= 4
            ? prev
            : [...prev, { path: saved.path, previewUrl: dataUrl }],
        );
      }
    }
    setAttachMenuOpen(false);
  };

  const handleUploadMainImages = async (files: FileList | File[]) => {
    if (!effectiveCwd) return;
    for (const file of Array.from(files)) {
      if (!file.type.startsWith('image/')) continue;
      const dataUrl = await readDataUrl(file);
      if (!dataUrl) continue;
      const saved = await store.uploadAttachment(effectiveCwd, {
        name: file.name || 'image.png',
        mimeType: file.type,
        dataBase64: dataUrl,
      });
      if (saved?.path) {
        setMainInput(prev => (prev.trim() ? `${prev.trim()} [${saved.path}]` : `[${saved.path}] `));
      }
    }
    setMainAttachMenuOpen(false);
  };

  const handlePickDirectory = async () => {
    const picker = (
      window as unknown as {
        __DSH_DIRECTORY_PICKER__?: { pick?: () => Promise<string | null> };
      }
    ).__DSH_DIRECTORY_PICKER__;
    if (picker && typeof picker.pick === 'function') {
      try {
        const picked = await picker.pick();
        if (typeof picked === 'string' && picked.trim()) {
          setCwdOverride(picked.trim());
          setEditingCwd(false);
        }
        return;
      } catch {
        // Fallback to inline path editor.
      }
    }
    setEditingCwd(v => !v);
  };

  const planning = snap.planning ?? snap.state?.planning ?? null;
  const resolvedPlanning = useMemo(() => resolvePlanningView(planning), [planning]);
  const latestCandidate = snap.designCandidates.at(-1) ?? null;
  const candidateLabels = useMemo(() => resolveCandidateLabels(latestCandidate), [latestCandidate]);
  const isPartialDualDraft = Boolean(
    resolvedPlanning &&
      !resolvedPlanning.isApproved &&
      !resolvedPlanning.design &&
      !snap.running &&
      (resolvedPlanning.phase === 'awaiting-input' || resolvedPlanning.isBlocked) &&
      latestCandidate?.meta?.partial,
  );
  const investigation = snap.state?.investigation ?? null;

  const packets = useMemo<PacketNode[]>(() => normalizePackets(snap.state?.packets), [snap.state?.packets]);

  const mainScrollResetKey = isNewTaskMode ? 'new-task' : `main::${snap.selectedTaskDir || ''}`;
  const mainScrollContentKey = [
    snap.state?.revision ?? 0,
    snap.traces.length,
    packets.length,
    resolvedPlanning?.phase ?? '',
    snap.running ? 1 : 0,
    snap.delivery ? 1 : 0,
    snap.traces[0]?.updatedAt ?? '',
  ].join(':');
  const {
    scrollRef: mainScrollRef,
    showScrollToBottom: showMainScrollBottom,
    scrollToBottom: scrollMainToBottom,
    handleScroll: handleMainScroll,
  } = useSmartAutoScroll(mainScrollResetKey, mainScrollContentKey);

  const waves = useMemo(() => groupIntoWaves(packets, snap.state?.waves), [packets, snap.state?.waves]);

  const firstExecTs = useMemo(() => {
    let minTs = Number.POSITIVE_INFINITY;
    for (const tr of snap.traces) {
      if (['execute', 'review', 'frontend-ux-review', 'judge'].includes(tr.role)) {
        const ts = Date.parse(tr.startedAt || tr.updatedAt || '');
        if (Number.isFinite(ts) && ts < minTs) minTs = ts;
      }
    }
    return minTs;
  }, [snap.traces]);

  const { initialPlanningTraces, midExecPlanRevisions } = useMemo(() => {
    const initial: TraceSummary[] = [];
    const midExec: TraceSummary[] = [];
    for (const tr of snap.traces) {
      if (['clarify', 'design-plan', 'design-review', 'ui-prototype', 'ui-prototype-review'].includes(tr.role)) {
        initial.push(tr);
      } else if (tr.role === 'plan' || tr.role === 'plan-review') {
        const ts = Date.parse(tr.startedAt || tr.updatedAt || '');
        if (Number.isFinite(firstExecTs) && Number.isFinite(ts) && ts >= firstExecTs) {
          midExec.push(tr);
        } else {
          initial.push(tr);
        }
      }
    }
    const sortedMid = [...midExec].sort((a, b) => {
      const ta = Date.parse(a.startedAt || a.updatedAt || '') || 0;
      const tb = Date.parse(b.startedAt || b.updatedAt || '') || 0;
      return ta - tb;
    });
    const requests = Array.isArray(planning?.requests) ? planning.requests : [];
    const groups: MidExecPlanRevisionGroup[] = [];
    const byScope = new Map<string, MidExecPlanRevisionGroup>();
    for (const tr of sortedMid) {
      const scopeKey = tr.scope || tr.key || tr.id;
      let grp = byScope.get(scopeKey);
      if (!grp) {
        const requestId = scopeKey.replace(/^planning:/, '');
        const req = requests.find(r => r.id === requestId || `planning:${r.id}` === scopeKey);
        const { packetId, summary } = summarizePlanRevisionFeedback(req?.feedback, isZh);
        const scopePkt = packetId || (/\b(M\d+-T\d+)\b/i.exec(requestId)?.[1]?.toUpperCase() ?? null);
        grp = {
          scope: scopeKey,
          requestId,
          label: isZh ? `计划修订 · ${requestId}` : `Plan Revision · ${requestId}`,
          triggerPacketId: scopePkt,
          triggerSummary: summary,
          items: [],
          running: false,
          totalDurationMs: 0,
        };
        byScope.set(scopeKey, grp);
        groups.push(grp);
      }
      grp.items.push(tr);
      if (tr.running) grp.running = true;
      if (typeof tr.durationMs === 'number' && Number.isFinite(tr.durationMs)) {
        grp.totalDurationMs += tr.durationMs;
      }
    }
    return { initialPlanningTraces: initial, midExecPlanRevisions: groups };
  }, [snap.traces, firstExecTs, planning?.requests, isZh]);

  const timelineRounds = useMemo(
    () => groupPlanningTracesIntoRounds(initialPlanningTraces, isZh),
    [initialPlanningTraces, isZh],
  );

  const packetTracesMap = useMemo(() => {
    const map = new Map<string, TraceSummary[]>();
    const execRoles = new Set(['execute', 'review', 'frontend-ux-review']);
    for (const tr of snap.traces) {
      if (!execRoles.has(tr.role) || !tr.scope) continue;
      const pktId = tr.scope.replace(/^final:/, '');
      const list = map.get(pktId) ?? [];
      list.push(tr);
      map.set(pktId, list);
    }
    for (const list of map.values()) {
      list.sort((a, b) => {
        const ta = Date.parse(a.startedAt || a.updatedAt || '') || 0;
        const tb = Date.parse(b.startedAt || b.updatedAt || '') || 0;
        return ta - tb;
      });
    }
    return map;
  }, [snap.traces]);

  const revisionsByPacket = useMemo(() => {
    const map = new Map<string, MidExecPlanRevisionGroup[]>();
    for (const rev of midExecPlanRevisions) {
      if (!rev.triggerPacketId) continue;
      const list = map.get(rev.triggerPacketId) ?? [];
      list.push(rev);
      map.set(rev.triggerPacketId, list);
    }
    return map;
  }, [midExecPlanRevisions]);

  const investigationTraces = useMemo<TraceSummary[]>(() => {
    return snap.traces.filter(tr => tr.role === 'investigate');
  }, [snap.traces]);

  const traceByPacket = useMemo(() => {
    const map = new Map<string, TraceSummary>();
    for (const tr of snap.traces) {
      if (tr.scope && !map.has(tr.scope)) {
        map.set(tr.scope, tr);
      }
    }
    return map;
  }, [snap.traces]);

  const mentionTargets = useMemo<MentionTarget[]>(() => {
    const targets: MentionTarget[] = [];
    const seenKeys = new Set<string>();
    for (const p of packets) {
      const tr = traceByPacket.get(p.id) ?? null;
      const sessionKey = p.sessionKey || tr?.key || `execute/${p.id}`;
      seenKeys.add(sessionKey);
      seenKeys.add(`execute/${p.id}`);
      const pktThinking = typeof (p as Record<string, unknown>).thinking === 'string'
        ? String((p as Record<string, unknown>).thinking)
        : '';
      const modelTxt = formatModelBadge(p.model || tr?.model, pktThinking || tr?.thinking, isZh);
      const tone = packetTone(p.status);
      const titleTxt = p.title || p.goal || '';
      targets.push({
        id: p.id,
        group: 'packets',
        label: p.id,
        subtitle: [titleTxt, modelTxt].filter(Boolean).join(' · '),
        dotClass: tone.dot,
        sessionKey,
        traceId: tr?.id ?? null,
        packetId: p.id,
        pillText: `@${p.id}${isZh ? ' · 执行' : ' · Exec'}`,
        searchKey: `${p.id} ${titleTxt} ${modelTxt} execute`.toLowerCase(),
      });
    }
    for (const tr of snap.traces) {
      if (!tr.key || seenKeys.has(tr.key)) continue;
      seenKeys.add(tr.key);
      const rLabel = roleLabel(tr.role, isZh, tr.scope, tr.sessionTitle);
      const scopeTag = tr.scope ? tr.scope.replace(/^planning:/, '') : '';
      const modelTxt = formatModelBadge(tr.model, tr.thinking, isZh);
      const tone = traceTone(tr.status, tr.running);
      const shortName = scopeTag && scopeTag !== tr.role ? `${tr.role}/${scopeTag}` : tr.role;
      targets.push({
        id: tr.scope || tr.key,
        group: 'sessions',
        label: scopeTag ? `${rLabel} · ${scopeTag}` : rLabel,
        subtitle: [modelTxt || tr.harness, tr.status].filter(Boolean).join(' · '),
        dotClass: tone.dot,
        sessionKey: tr.key,
        traceId: tr.id,
        packetId: null,
        pillText: `@${shortName}`,
        searchKey: `${tr.key} ${tr.role} ${tr.scope} ${rLabel} ${modelTxt}`.toLowerCase(),
      });
    }
    return targets;
  }, [packets, traceByPacket, snap.traces, isZh]);

  const filteredMentions = useMemo(() => {
    if (mentionQuery === null) return [];
    const q = mentionQuery.trim().toLowerCase();
    if (!q) return mentionTargets;
    return mentionTargets.filter(item => item.searchKey.includes(q));
  }, [mentionQuery, mentionTargets]);

  const applyMentionTarget = (target: MentionTarget) => {
    setSelectedMention(target);
    setMainInput(prev => prev.replace(/(?:^|\s)@[^\s@]*$/, m => (m.startsWith(' ') ? ' ' : '')).trimStart());
    setMentionQuery(null);
    setMentionActiveIdx(0);
    setTimeout(() => mainTextareaRef.current?.focus(), 0);
  };

  const handleCreateFromChat = async () => {
    const goalText = newGoal.trim();
    if (!goalText || !effectiveCwd || snap.busyAction !== null) return;
    const slug = customSlug.trim() || generateSlug(goalText);
    const readPaths = readPathsText
      .split(/[\n,]+/)
      .map(s => s.trim())
      .filter(Boolean);
    const res =
      intent === 'investigate'
        ? await store.createInvestigationTask({ cwd: effectiveCwd, slug, goal: goalText, readPaths })
        : await store.createBuildTask({
            cwd: effectiveCwd,
            slug,
            goal: goalText,
            mode: intent === 'quick' ? 'quick' : planMode,
            clarifyMode: intent === 'quick' ? 'off' : clarifyMode,
            uiPrototypeMode: intent === 'quick' ? 'off' : uiPrototypeMode,
            images: selectedImages.map(item => item.path),
          });
    if (res) {
      setNewGoal('');
      setCustomSlug('');
      setReadPathsText('');
      setSelectedImages([]);
      setShowImages(false);
      setAttachMenuOpen(false);
      setAdvancedOpen(false);
    }
  };

  const handleMainSend = async () => {
    const text = mainInput.trim();
    if (!text || snap.busyAction !== null) return;

    // Structured atomic `@mention` pill routing takes highest priority
    if (selectedMention) {
      const target = selectedMention;
      setMainInput('');
      setSelectedMention(null);
      setMentionQuery(null);
      store.drillIntoNode(target.packetId || target.id, {
        traceId: target.traceId ?? null,
        sessionKey: target.sessionKey,
        packetId: target.packetId ?? null,
      });
      await store.sendSessionMessage(target.sessionKey, text);
      return;
    }

    // Legacy raw `@packetId message` direct routing fallback
    const mentionMatch = /^@([A-Za-z0-9_.:/-]+)\s+([\s\S]+)$/.exec(text);
    if (mentionMatch) {
      const targetId = mentionMatch[1]!;
      const body = mentionMatch[2]!.trim();
      const targetPacket = packets.find(p => p.id.toLowerCase() === targetId.toLowerCase());
      const targetTrace = targetPacket
        ? traceByPacket.get(targetPacket.id)
        : snap.traces.find(tr => tr.scope?.toLowerCase() === targetId.toLowerCase() || tr.key.toLowerCase() === targetId.toLowerCase() || tr.id === targetId);
      const sessionKey = targetPacket?.sessionKey || targetTrace?.key || (targetPacket ? `execute/${targetPacket.id}` : null);
      if (sessionKey) {
        setMainInput('');
        setMentionQuery(null);
        store.drillIntoNode(targetPacket?.id || targetId, {
          traceId: targetTrace?.id ?? null,
          sessionKey,
          packetId: targetPacket?.id ?? null,
        });
        await store.sendSessionMessage(sessionKey, body);
        return;
      }
    }

    setMainInput('');
    setMentionQuery(null);
    if (investigation && packets.length === 0) {
      await store.continueInvestigation(text, []);
      return;
    }
    await store.submitDesignFeedback(text);
  };

  // ---- Mode A: Clean DSH New-Task Chat ----
  if (isNewTaskMode) {
    const repoLabel = effectiveCwd ? effectiveCwd.replace(/\/+$/, '').split('/').pop() : t('create.cwd');

    return (
      <div className={styles.chatWorkspace} data-testid="flywheel-new-task-chat">
        <div className={styles.chatScrollArea}>
          <div className={styles.chatWelcome}>
            <div className={styles.chatWelcomeMark} aria-hidden="true">◎</div>
            <h2 style={{ margin: 0, fontSize: 20, fontWeight: 600, letterSpacing: '-0.02em' }}>
              {t('chat.newTaskTitle')}
            </h2>
            <p style={{ margin: 0, fontSize: 13, color: 'var(--fw-text-muted)', maxWidth: 520, lineHeight: 1.6 }}>
              {t('chat.newTaskSubtitle')}
            </p>

            <div className={styles.chatIntentGrid}>
              {(
                [
                  { id: 'build', title: t('create.intent.build'), desc: t('create.intent.buildDesc') },
                  { id: 'quick', title: t('create.intent.quick'), desc: t('create.intent.quickDesc') },
                  { id: 'investigate', title: t('create.intent.investigate'), desc: t('create.intent.investigateDesc') },
                ] as const
              ).map(item => (
                <button
                  key={item.id}
                  type="button"
                  className={`${styles.chatIntentBtn} ${intent === item.id ? styles.chatIntentBtnActive : ''}`}
                  onClick={() => {
                    setIntent(item.id);
                    if (item.id === 'quick') setPlanMode('quick');
                    else if (item.id === 'build' && planMode === 'quick') setPlanMode('auto');
                  }}
                >
                  <strong style={{ fontSize: 12.5 }}>{item.title}</strong>
                  <span style={{ fontSize: 11.5, color: 'var(--fw-text-muted)', lineHeight: 1.45 }}>{item.desc}</span>
                </button>
              ))}
            </div>
          </div>
        </div>

        <div className={styles.composerDock}>
          <input
            ref={newFileInputRef}
            type="file"
            accept="image/png,image/jpeg,image/webp,image/gif"
            multiple
            style={{ display: 'none' }}
            data-testid="flywheel-new-file-input"
            onChange={e => {
              if (e.target.files && e.target.files.length > 0) {
                void handleUploadNewImages(e.target.files);
                e.target.value = '';
              }
            }}
          />

          <div className={styles.composerMetaRow}>
            <div className={styles.composerMetaGroup}>
              <button
                type="button"
                className={styles.composerMetaBtn}
                onClick={() => void handlePickDirectory()}
                title={effectiveCwd || t('create.cwd')}
                data-testid="flywheel-pick-cwd"
              >
                📁 {repoLabel} ▾
              </button>
              <button
                type="button"
                className={styles.composerMetaBtn}
                onClick={() => setEditingCwd(v => !v)}
                title={t('chat.composer.manualDir')}
                aria-label={t('chat.composer.manualDir')}
              >
                ✎
              </button>
              {intent !== 'investigate' && (
                <>
                  <button
                    type="button"
                    className={styles.composerMetaBtn}
                    onClick={() =>
                      setPlanMode(m => (m === 'auto' ? 'full' : m === 'full' ? 'quick' : 'auto'))
                    }
                  >
                    ◎{' '}
                    {planMode === 'full'
                      ? t('create.mode.full')
                      : planMode === 'quick'
                        ? t('create.mode.quick')
                        : t('create.mode.auto')}{' '}
                    ▾
                  </button>
                  {intent !== 'quick' && (
                    <>
                      <button
                        type="button"
                        className={styles.composerMetaBtn}
                        data-testid="flywheel-clarify-toggle"
                        onClick={() =>
                          setClarifyMode(m => (m === 'auto' ? 'always' : m === 'always' ? 'off' : 'auto'))
                        }
                      >
                        {isZh
                          ? `需求澄清: ${clarifyMode === 'auto' ? 'JEV 智能' : clarifyMode === 'always' ? '总是开启' : '关闭'}`
                          : `Clarify: ${clarifyMode}`}
                      </button>
                      <button
                        type="button"
                        className={styles.composerMetaBtn}
                        data-testid="flywheel-uiproto-toggle"
                        onClick={() =>
                          setUiPrototypeMode(m => (m === 'auto' ? 'always' : m === 'always' ? 'off' : 'auto'))
                        }
                      >
                        {isZh
                          ? `UI 原型: ${uiPrototypeMode === 'auto' ? '前端自动' : uiPrototypeMode === 'always' ? '总是生成' : '关闭'}`
                          : `UI Mockup: ${uiPrototypeMode}`}
                      </button>
                    </>
                  )}
                </>
              )}
              <button
                type="button"
                className={styles.composerMetaBtn}
                data-testid="flywheel-new-advanced-toggle"
                onClick={() => setAdvancedOpen(v => !v)}
              >
                ⚙ {t('chat.composer.advanced')} {advancedOpen ? '▴' : '▾'}
              </button>
            </div>
            {selectedImages.length > 0 && (
              <span className={styles.pill}>🖼 {selectedImages.length}/4</span>
            )}
          </div>

          <div
            className={`${styles.composerBox}${dragging ? ` ${styles.composerBoxDrag}` : ''}`}
            onDragOver={(e: DragEvent<HTMLDivElement>) => {
              e.preventDefault();
              if (!dragging) setDragging(true);
            }}
            onDragLeave={() => setDragging(false)}
            onDrop={(e: DragEvent<HTMLDivElement>) => {
              e.preventDefault();
              setDragging(false);
              const imgs = Array.from(e.dataTransfer?.files ?? []).filter(f => f.type.startsWith('image/'));
              if (imgs.length > 0 && intent !== 'investigate') void handleUploadNewImages(imgs);
            }}
          >
            {dragging && (
              <div style={{ fontSize: 12, color: 'var(--fw-accent)', textAlign: 'center', padding: '4px 0' }}>
                {t('chat.composer.dropHint')}
              </div>
            )}

            {editingCwd && (
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, paddingBottom: 4 }}>
                <input
                  type="text"
                  className={styles.input}
                  value={cwdOverride || snap.selectedCwd}
                  onChange={e => setCwdOverride(e.target.value)}
                  placeholder="/path/to/repo"
                  data-testid="flywheel-chat-cwd-input"
                />
                <button type="button" className={`${styles.btn} ${styles.btnSm}`} onClick={() => setEditingCwd(false)}>
                  OK
                </button>
              </div>
            )}

            {advancedOpen && (
              <div className={styles.advancedPanel} data-testid="flywheel-new-advanced-panel">
                <div className={styles.advancedRow}>
                  <label className={styles.advancedField}>
                    <span>{t('create.slug')}</span>
                    <input
                      type="text"
                      className={styles.input}
                      value={customSlug}
                      onChange={e => setCustomSlug(e.target.value)}
                      placeholder="auto-generated"
                      data-testid="flywheel-new-slug-input"
                    />
                  </label>
                  {intent !== 'investigate' ? (
                    <label className={styles.advancedField}>
                      <span>{t('create.mode')}</span>
                      <select
                        className={styles.select}
                        value={planMode}
                        onChange={e => {
                          const v = e.target.value as 'auto' | 'full' | 'quick';
                          setPlanMode(v);
                          if (v === 'quick') setIntent('quick');
                          else setIntent('build');
                        }}
                      >
                        <option value="auto">{t('create.mode.auto')}</option>
                        <option value="full">{t('create.mode.full')}</option>
                        <option value="quick">{t('create.mode.quick')}</option>
                      </select>
                    </label>
                  ) : (
                    <label className={styles.advancedField}>
                      <span>{t('create.readPaths')}</span>
                      <input
                        type="text"
                        className={styles.input}
                        value={readPathsText}
                        onChange={e => setReadPathsText(e.target.value)}
                        placeholder="src/, internal/"
                        data-testid="flywheel-new-readpaths-input"
                      />
                    </label>
                  )}
                </div>
              </div>
            )}

            {selectedImages.length > 0 && (
              <div className={styles.attachThumbStrip} data-testid="flywheel-attach-strip">
                {selectedImages.map(item => (
                  <div key={item.path} className={styles.attachThumb}>
                    {item.previewUrl ? (
                      <img src={item.previewUrl} alt={item.path} className={styles.attachThumbImg} />
                    ) : (
                      <span className={styles.attachThumbIcon}>🖼</span>
                    )}
                    <span className={styles.attachThumbName} title={item.path}>
                      {item.path.split('/').pop()}
                    </span>
                    <button
                      type="button"
                      className={styles.attachThumbRemove}
                      aria-label={`remove ${item.path}`}
                      onClick={() => setSelectedImages(prev => prev.filter(p => p.path !== item.path))}
                    >
                      ×
                    </button>
                  </div>
                ))}
              </div>
            )}

            {showImages && (
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, paddingBottom: 6, borderBottom: '1px solid var(--fw-border)' }}>
                {snap.repoImages.length === 0 ? (
                  <span style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>{t('create.imagesEmpty')}</span>
                ) : (
                  snap.repoImages.slice(0, 12).map(img => {
                    const active = selectedImages.some(item => item.path === img.path);
                    return (
                      <button
                        key={img.path}
                        type="button"
                        className={`${styles.jevDataChip} ${active ? styles.jevDataChipActive : ''}`}
                        onClick={() =>
                          setSelectedImages(prev =>
                            prev.some(p => p.path === img.path)
                              ? prev.filter(p => p.path !== img.path)
                              : prev.length < 4
                                ? [...prev, { path: img.path }]
                                : prev,
                          )
                        }
                      >
                        {img.path}
                      </button>
                    );
                  })
                )}
              </div>
            )}

            <textarea
              className={styles.composerTextarea}
              data-testid="flywheel-chat-input"
              rows={3}
              value={newGoal}
              onChange={e => setNewGoal(e.target.value)}
              onPaste={(e: ClipboardEvent<HTMLTextAreaElement>) => {
                if (intent === 'investigate') return;
                const imgs = Array.from(e.clipboardData?.files ?? []).filter(f => f.type.startsWith('image/'));
                if (imgs.length > 0) {
                  e.preventDefault();
                  void handleUploadNewImages(imgs);
                }
              }}
              onKeyDown={e => {
                if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  void handleCreateFromChat();
                }
              }}
              placeholder={t('chat.composer.newPlaceholder')}
            />

            <div className={styles.composerBar}>
              <div className={styles.composerTools} style={{ position: 'relative' }}>
                {intent !== 'investigate' && (
                  <>
                    <button
                      type="button"
                      className={styles.composerPlusBtn}
                      data-testid="flywheel-new-attach-btn"
                      title={t('chat.composer.imagesBtn')}
                      aria-label={t('chat.composer.imagesBtn')}
                      onClick={() => setAttachMenuOpen(v => !v)}
                    >
                      +
                    </button>
                    {attachMenuOpen && (
                      <div className={styles.attachMenu} data-testid="flywheel-attach-menu">
                        <button
                          type="button"
                          className={styles.attachMenuItem}
                          onClick={() => {
                            setAttachMenuOpen(false);
                            newFileInputRef.current?.click();
                          }}
                        >
                          🖼 {t('chat.composer.uploadLocal')}
                        </button>
                        <button
                          type="button"
                          className={styles.attachMenuItem}
                          onClick={() => {
                            setAttachMenuOpen(false);
                            setShowImages(v => !v);
                            void store.loadRepoImages(effectiveCwd || undefined);
                          }}
                        >
                          📁 {t('chat.composer.scanRepo')}
                        </button>
                      </div>
                    )}
                  </>
                )}
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <button
                  type="button"
                  className={styles.composerMetaBtn}
                  onClick={() => store.openDrawer('settings')}
                >
                  {t('chat.composer.routingLink')}
                </button>
                <button
                  type="button"
                  className={styles.composerSendBtn}
                  data-testid="flywheel-chat-send"
                  title={t('chat.composer.createSend')}
                  aria-label={t('chat.composer.createSend')}
                  disabled={!newGoal.trim() || !effectiveCwd || snap.busyAction !== null}
                  onClick={() => void handleCreateFromChat()}
                >
                  ↑
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // ---- Mode B: Active Task Main Stream ----
  const activeRun = snap.runs.find(r => r.taskDir === snap.selectedTaskDir) ?? null;
  const userRequestText =
    resolvedPlanning?.briefOriginalRequest ||
    resolvedPlanning?.briefGoal ||
    investigation?.goal ||
    snap.state?.title ||
    snap.state?.slug ||
    activeRun?.title ||
    activeRun?.slug ||
    '';

  const mainPlaceholder =
    packets.length > 0
      ? t('chat.composer.execPlaceholder')
      : investigation
        ? t('chat.composer.invPlaceholder')
        : t('chat.composer.planPlaceholder');

  const showDeliveryBlock = Boolean(
    snap.delivery ||
      snap.state?.status === 'completed' ||
      snap.state?.status === 'done' ||
      (packets.length > 0 && packets.every(p => ['done', 'completed', 'passed'].includes((p.status ?? '').toLowerCase()))),
  );

  return (
    <div className={styles.chatWorkspace} data-testid="flywheel-main-stream">
      <div className={styles.chatScrollArea} ref={mainScrollRef} onScroll={handleMainScroll}>
        <div className={styles.chatColumn}>
          {/* Right-aligned Original User Requirement Bubble */}
          {userRequestText && (
            <div className={styles.userBubbleRow} data-testid="flywheel-user-requirement">
              <div className={styles.userBubble}>
                <div>{userRequestText}</div>
                <div className={styles.userBubbleMeta}>
                  {activeRun?.repoName && <span className={styles.pill}>{activeRun.repoName}</span>}
                  {(snap.state?.mode || resolvedPlanning?.mode || activeRun?.mode) && (
                    <span className={`${styles.pill} ${styles.pillPurple}`}>
                      {snap.state?.mode || resolvedPlanning?.mode || activeRun?.mode}
                    </span>
                  )}
                  {snap.state?.slug && <span className={styles.pill}>{snap.state.slug}</span>}
                </div>
              </div>
            </div>
          )}

          {/* Stage 0: Requirements Clarification & Measurable Quality Gates */}
          {planning?.clarification && (
            <section className={styles.orchBlock} data-testid="flywheel-stage-clarify">
              <div className={styles.orchBlockHead}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                  <span className={styles.orchAvatar}>◎</span>
                  <strong style={{ fontSize: 13.5 }}>
                    {isZh ? '阶段 0 · 需求澄清与可量化质量门禁' : 'Stage 0 · Clarification & Quality Gates'}
                  </strong>
                  <span
                    className={`${styles.pill} ${
                      planning.clarification.status === 'confirmed' || planning.clarification.status === 'skipped'
                        ? styles.pillSuccess
                        : styles.pillWarn
                    }`}
                  >
                    {planning.clarification.status === 'awaiting-answers'
                      ? (isZh ? '等待你确认' : 'Awaiting Answers')
                      : planning.clarification.status === 'confirmed'
                        ? (isZh ? '已冻结约束' : 'Confirmed')
                        : (isZh ? `已跳过 (${planning.clarification['decided-by'] || 'auto'})` : 'Skipped')}
                  </span>
                </div>
              </div>

              <div className={styles.orchIndent}>
                {planning.clarification.status === 'awaiting-answers' ? (
                  <div className={styles.clarifyCard} data-testid="stage0-clarify-form">
                    {planning.clarification.summary && (
                      <div style={{ fontSize: 12.5, color: 'var(--fw-text-muted)', lineHeight: 1.55 }}>
                        {planning.clarification.summary}
                      </div>
                    )}

                    {(planning.clarification.questions ?? []).map(q => {
                      const currentVal = clarifyAnswers[q.id] ?? q.recommended ?? '';
                      return (
                        <div key={q.id} className={styles.clarifyQuestion}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
                            <strong style={{ fontSize: 12.5 }}>{q.title}</strong>
                            {q.required && <span className={styles.pill}>{isZh ? '必选' : 'Required'}</span>}
                          </div>
                          {q.rationale && (
                            <div style={{ fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>{q.rationale}</div>
                          )}
                          {Array.isArray(q.options) && q.options.length > 0 ? (
                            <div className={styles.clarifyOptions}>
                              {q.options.map(opt => {
                                const active = currentVal === opt;
                                return (
                                  <button
                                    key={opt}
                                    type="button"
                                    className={`${styles.clarifyOption} ${active ? styles.clarifyOptionActive : ''}`}
                                    onClick={() => setClarifyAnswers(prev => ({ ...prev, [q.id]: opt }))}
                                  >
                                    {opt}
                                  </button>
                                );
                              })}
                            </div>
                          ) : (
                            <input
                              type="text"
                              className={styles.input}
                              value={currentVal}
                              onChange={e => setClarifyAnswers(prev => ({ ...prev, [q.id]: e.target.value }))}
                              placeholder={isZh ? '输入你的要求…' : 'Enter your answer…'}
                            />
                          )}
                        </div>
                      );
                    })}

                    {/* Measurable Objective Quality Gates */}
                    <div className={styles.clarifyQuestion}>
                      <strong style={{ fontSize: 12.5 }}>
                        {isZh ? '可量化工程质量门禁（反向驱动代码架构与可测性）' : 'Measurable Quality Gates'}
                      </strong>
                      <div className={styles.clarifyOptions}>
                        {(planning.clarification['suggested-gates'] ?? []).map(gate => {
                          const checked = selectedGates[gate] ?? true;
                          return (
                            <button
                              key={gate}
                              type="button"
                              className={`${styles.clarifyOption} ${checked ? styles.clarifyOptionActive : ''}`}
                              onClick={() => setSelectedGates(prev => ({ ...prev, [gate]: !checked }))}
                            >
                              {checked ? '✓ ' : '+ '}{gate}
                            </button>
                          );
                        })}
                      </div>
                      <input
                        type="text"
                        className={styles.input}
                        value={customGateInput}
                        onChange={e => setCustomGateInput(e.target.value)}
                        placeholder={isZh ? '追加自定义客观指标（例如：核心引擎单元测试覆盖率 100%）…' : 'Add custom objective gate…'}
                      />
                    </div>

                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, flexWrap: 'wrap' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', flex: 1 }}>
                        <input
                          type="text"
                          className={styles.input}
                          style={{ flex: 1, minWidth: 200 }}
                          value={clarifyNote}
                          onChange={e => setClarifyNote(e.target.value)}
                          placeholder={isZh ? '补充说明（可选）…' : 'Additional notes (optional)…'}
                        />
                        {planning.clarification['is-frontend-ui'] && (
                          <button
                            type="button"
                            className={styles.composerMetaBtn}
                            onClick={() =>
                              setClarifyProtoMode(m => (m === 'auto' ? 'always' : m === 'always' ? 'off' : 'auto'))
                            }
                          >
                            {isZh
                              ? `UI/UX 原型: ${clarifyProtoMode === 'off' ? '跳过' : '架构后生成交互原型'}`
                              : `UI Prototype: ${clarifyProtoMode}`}
                          </button>
                        )}
                      </div>
                      <button
                        type="button"
                        className={`${styles.btn} ${styles.btnPrimary}`}
                        data-testid="stage0-clarify-submit"
                        disabled={snap.busyAction !== null}
                        onClick={() => {
                          const finalAnswers: Record<string, string> = {};
                          for (const q of planning.clarification?.questions ?? []) {
                            const v = (clarifyAnswers[q.id] ?? q.recommended ?? '').trim();
                            if (v) finalAnswers[q.id] = v;
                          }
                          const gates = (planning.clarification?.['suggested-gates'] ?? []).filter(
                            g => (selectedGates[g] ?? true),
                          );
                          if (customGateInput.trim()) gates.push(customGateInput.trim());
                          void store.submitClarification({
                            answers: finalAnswers,
                            selectedGates: gates,
                            freeformNote: clarifyNote,
                            uiPrototypeMode: clarifyProtoMode,
                          });
                        }}
                      >
                        {isZh ? '确认需求与质量指标，开始架构设计 →' : 'Confirm & Start Design →'}
                      </button>
                    </div>
                  </div>
                ) : (
                  <details className={styles.roundGroup}>
                    <summary className={styles.roundGroupHead}>
                      <span>
                        {planning.clarification.summary ||
                          (isZh ? 'Stage 0 需求澄清已完成' : 'Stage 0 Clarification complete')}
                      </span>
                      {(planning.clarification['selected-gates']?.length ?? 0) > 0 && (
                        <span className={`${styles.pill} ${styles.pillSuccess}`}>
                          {isZh
                            ? `已锁定 ${planning.clarification['selected-gates']!.length} 条质量门禁`
                            : `${planning.clarification['selected-gates']!.length} Quality Gates`}
                        </span>
                      )}
                    </summary>
                    <div style={{ padding: '10px 14px', fontSize: 12, color: 'var(--fw-text-muted)', display: 'flex', flexDirection: 'column', gap: 6 }}>
                      {Object.entries(planning.clarification.answers ?? {}).map(([k, v]) => (
                        <div key={k}><strong>{k}:</strong> {v}</div>
                      ))}
                      {(planning.clarification['selected-gates'] ?? []).length > 0 && (
                        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                          {planning.clarification['selected-gates']!.map(g => (
                            <span key={g} className={`${styles.pill} ${styles.pillSuccess}`}>✓ {g}</span>
                          ))}
                        </div>
                      )}
                    </div>
                  </details>
                )}
              </div>
            </section>
          )}

          {/* Stage 1: Planning & Dual-Draft Review Block */}
          {(resolvedPlanning || initialPlanningTraces.length > 0) && (
            <section className={styles.orchBlock} data-testid="flywheel-stage-planning">
              <div className={styles.orchBlockHead}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                  <span className={styles.orchAvatar}>◎</span>
                  <strong style={{ fontSize: 13.5 }}>{t('chat.stage.planning')}</strong>
                  {resolvedPlanning?.isApproved ? (
                    <span className={`${styles.pill} ${styles.pillSuccess}`}>{t('design.approvedBadge')}</span>
                  ) : resolvedPlanning?.isBlocked ? (
                    <span className={`${styles.pill} ${styles.pillDanger}`}>{t('design.blockedBadge')}</span>
                  ) : resolvedPlanning?.needsHumanApproval ? (
                    <span className={`${styles.pill} ${styles.pillWarn}`}>{t('design.awaitingBadge')}</span>
                  ) : snap.running && packets.length === 0 ? (
                    <span className={`${styles.pill} ${styles.pillRunning}`}>{t('status.planning')}</span>
                  ) : null}
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
                  {resolvedPlanning?.design?.revision && (
                    <button
                      type="button"
                      className={styles.artifactLinkChip}
                      onClick={() =>
                        void store.openArtifactPreview(
                          `planning/design-${String(resolvedPlanning.design!.revision).padStart(6, '0')}.md`,
                          { title: `design-rev${resolvedPlanning.design!.revision}.md` },
                        )
                      }
                    >
                      📄 {`planning/design-${String(resolvedPlanning.design.revision).padStart(6, '0')}.md`}
                    </button>
                  )}
                  <button
                    type="button"
                    className={styles.linkBtn}
                    onClick={() => store.openDrawer('design')}
                  >
                    {t('chat.openDesignDrawer')}
                  </button>
                </div>
              </div>

              <div className={styles.orchIndent}>
                {/* Human-Friendly Interrupted Session Recovery Card */}
                {blockedSessions.length > 0 && !snap.running && (
                  <div className={styles.blockedBanner} data-testid="main-blocked-sessions">
                    {blockedSessions.map(item => {
                      const val = abandonReasons[item.key] ?? '';
                      const stageName = humanizeBlockedKey(item.key, isZh);
                      const isReadOnlyPlanning =
                        item.key.startsWith('design-plan/') ||
                        item.key.startsWith('design-review/') ||
                        item.key.startsWith('plan/') ||
                        item.key.startsWith('plan-review/');
                      return (
                        <div key={item.key} style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                          <div className={styles.blockedHeader}>
                            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                              <span className={`${styles.pill} ${styles.pillDanger}`}>
                                {isZh ? '执行中断' : t('status.blocked')}
                              </span>
                              <strong style={{ fontSize: 13 }}>
                                {isZh
                                  ? `「${stageName}」意外中断，确认安全后可一键重试`
                                  : `${stageName} interrupted unexpectedly`}
                              </strong>
                            </div>
                          </div>
                          <div style={{ fontSize: 12.5, color: 'var(--fw-text-muted)', lineHeight: 1.55 }}>
                            {isReadOnlyPlanning
                              ? (isZh
                                  ? '上一次只读规划调用未能正常结束，引擎已自动暂停以防状态冲突（该阶段不修改业务代码）。点击右侧按钮即可清理此次中断记录并重新执行。'
                                  : 'The previous read-only planning call ended unexpectedly. Click below to clear the interrupted record and safely retry.')
                              : (isZh
                                  ? '上一次执行调用未能正常结束，引擎已暂停保护工作区。请确认本地工作区无异常改动后，点击右侧按钮安全重试。'
                                  : 'The previous execution call ended unexpectedly. Confirm the workspace is safe and click below to retry.')}
                          </div>
                          <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                            <input
                              type="text"
                              className={styles.input}
                              style={{ flex: 1, minWidth: 200 }}
                              data-testid={`abandon-reason-${item.invocation}`}
                              value={val}
                              onChange={e => setAbandonReasons(prev => ({ ...prev, [item.key]: e.target.value }))}
                              placeholder={isZh ? '备注原因（可选，留空将自动填充默认安全确认）…' : t('inspector.abandonPlaceholder')}
                            />
                            <button
                              type="button"
                              className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                              data-testid={`abandon-submit-${item.invocation}`}
                              disabled={snap.busyAction !== null}
                              onClick={async () => {
                                const reason =
                                  val.trim() ||
                                  'Human confirmed workspace safe and cleared interrupted invocation to retry';
                                setAbandonErrors(prev => {
                                  const next = { ...prev };
                                  delete next[item.key];
                                  return next;
                                });
                                const res = await store.abandonSession(item.key, item.invocation, reason);
                                const ctrl = res?.sessionControl as { applied?: boolean; message?: string } | undefined;
                                if (res?.ok && (ctrl ? ctrl.applied !== false : true)) {
                                  setAbandonReasons(prev => ({ ...prev, [item.key]: '' }));
                                  await store.startRun();
                                } else {
                                  const errMsg =
                                    ctrl?.message ||
                                    (typeof res?.error === 'string' ? res.error : '') ||
                                    (isZh ? '未能清理阻塞会话，请查看引擎状态后重试。' : 'Failed to abandon blocked session.');
                                  setAbandonErrors(prev => ({ ...prev, [item.key]: errMsg }));
                                }
                              }}
                            >
                              {isZh ? '确认安全并重试' : 'Confirm Safe & Retry'}
                            </button>
                          </div>
                          {abandonErrors[item.key] && (
                            <div
                              data-testid={`abandon-error-${item.invocation}`}
                              style={{ marginTop: 6, fontSize: 12, color: 'var(--fw-danger)', lineHeight: 1.5 }}
                            >
                              {abandonErrors[item.key]}
                            </div>
                          )}
                          <details className={styles.recoveryDetails}>
                            <summary>{isZh ? '查看技术详情' : 'Technical details'}</summary>
                            <div className={styles.recoveryDetailsBody}>
                              <div><strong>Session:</strong> {item.key}</div>
                              <div><strong>Call ID:</strong> {item.invocation}</div>
                              <div><strong>Engine:</strong> {item.reason}</div>
                            </div>
                          </details>
                        </div>
                      );
                    })}
                  </div>
                )}

                {/* Inline Human Approval Card (pinned above trace history so it is always visible) */}
                {resolvedPlanning?.needsHumanApproval && resolvedPlanning.design?.revision && (
                  <div className={styles.approvalBar} data-testid="main-approval-card">
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, flexWrap: 'wrap' }}>
                      <div>
                        <strong style={{ fontSize: 13.5 }}>
                          {t('design.approvePrompt', { rev: resolvedPlanning.design.revision })}
                        </strong>
                        <div style={{ fontSize: 12, color: 'var(--fw-text-muted)', marginTop: 2 }}>
                          {resolvedPlanning.review?.summary || t('design.approveSub')}
                        </div>
                      </div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnSm}`}
                          onClick={() => store.openDrawer('design')}
                        >
                          {t('chat.drawer.design')}
                        </button>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnSm}`}
                          disabled={snap.busyAction !== null}
                          onClick={() => void store.approveDesign(false)}
                        >
                          {t('design.approveOnly')}
                        </button>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                          disabled={snap.busyAction !== null}
                          onClick={() => void store.approveDesign(true)}
                        >
                          {t('design.approveAndStart')}
                        </button>
                      </div>
                    </div>
                  </div>
                )}

                {/* Explicit Dual-Draft Partial-Failure Decision Card (NO silent downgrade) */}
                {isPartialDualDraft && latestCandidate?.meta && (
                  <div className={styles.partialBanner} data-testid="main-dual-draft-partial">
                    <div className={styles.blockedHeader}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                        <span className={`${styles.pill} ${styles.pillWarn}`}>{t('design.partialBadge')}</span>
                        <strong style={{ fontSize: 13 }}>{t('design.partialTitle')}</strong>
                      </div>
                      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnSm}`}
                          onClick={() => {
                            store.setDesignView('side-by-side');
                            store.openDrawer('design');
                          }}
                        >
                          {t('design.partialInspectDraft')}
                        </button>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnSm}`}
                          data-testid="main-partial-proceed-single"
                          disabled={snap.busyAction !== null}
                          onClick={() => void store.retryPlanning({ draftMode: 'single' })}
                        >
                          {t('design.partialProceedSingle')}
                        </button>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                          data-testid="main-partial-retry-dual"
                          disabled={snap.busyAction !== null}
                          onClick={() => void store.retryPlanning({ draftMode: 'dual' })}
                        >
                          {t('design.partialRetryDual')}
                        </button>
                      </div>
                    </div>
                    <div className={styles.blockedMetaGrid}>
                      <span>
                        {t('design.partialCompleted')}:{' '}
                        <strong>
                          {latestCandidate.meta.completedSide === 'secondary'
                            ? `${t('design.secondaryDraft')} (${candidateLabels.secondaryModel || 'secondary'})`
                            : `${t('design.primaryDraft')} (${candidateLabels.primaryModel || 'primary'})`}
                        </strong>
                      </span>
                      <span>
                        {t('design.partialFailed')}:{' '}
                        <strong>
                          {latestCandidate.meta.failedSide === 'primary'
                            ? `${t('design.primaryDraft')} (${candidateLabels.primaryModel || 'primary'})`
                            : `${t('design.secondaryDraft')} (${candidateLabels.secondaryModel || 'secondary'})`}
                        </strong>
                      </span>
                    </div>
                    {(latestCandidate.meta.failureError || resolvedPlanning?.reason) && (
                      <div className={styles.blockedReasonBox}>
                        {String(latestCandidate.meta.failureError || resolvedPlanning?.reason || '')}
                      </div>
                    )}
                  </div>
                )}

                {/* Planning Blocked Diagnostic Card */}
                {!isPartialDualDraft && resolvedPlanning?.isBlocked && (
                  <div className={styles.blockedBanner} data-testid="main-planning-blocked">
                    <div className={styles.blockedHeader}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                        <span className={`${styles.pill} ${styles.pillDanger}`}>{t('design.blockedBadge')}</span>
                        <strong style={{ fontSize: 13 }}>{t('design.blockedTitle')}</strong>
                      </div>
                      <div style={{ display: 'flex', gap: 8 }}>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                          disabled={snap.busyAction !== null}
                          onClick={() => void store.retryPlanning()}
                        >
                          {t('design.retryPlanning')}
                        </button>
                      </div>
                    </div>
                    {resolvedPlanning.reason && (
                      <div className={styles.blockedReasonBox}>{resolvedPlanning.reason}</div>
                    )}
                  </div>
                )}

                {/* Chronological Round Timeline (Replaces flat card grid; auto-collapses past rounds) */}
                {timelineRounds.length > 0 && (
                  <div className={styles.roundTimeline} data-testid="planning-round-timeline">
                    {timelineRounds.map((group, gIdx) => {
                      const isLatest = gIdx === timelineRounds.length - 1;
                      const durStr = formatDurationShort(group.totalDurationMs);
                      return (
                        <details
                          key={`round-${group.round}-${group.label}`}
                          className={styles.roundGroup}
                          open={isLatest || group.running}
                        >
                          <summary className={styles.roundGroupHead}>
                            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                              <span
                                className={`${styles.statusDot} ${
                                  group.running ? styles.statusDotRunning : styles.statusDotSuccess
                                }`}
                              />
                              <strong>{group.label}</strong>
                              <span className={styles.pill}>
                                {isZh ? `${group.items.length} 个步骤` : `${group.items.length} steps`}
                              </span>
                            </span>
                            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>
                              {durStr && <span>{isZh ? `用时 ${durStr}` : durStr}</span>}
                              {group.running && (
                                <span className={`${styles.pill} ${styles.pillRunning}`}>{t('traces.liveBadge')}</span>
                              )}
                            </span>
                          </summary>
                          <div className={styles.roundStepsList}>
                            {group.items.map(tr => {
                              const tone = traceTone(tr.status, tr.running);
                              const timeStr = formatClock(tr.startedAt || tr.updatedAt);
                              const stepDur = formatDurationShort(tr.durationMs);
                              return (
                                <button
                                  key={tr.id}
                                  type="button"
                                  className={styles.roundStepRow}
                                  data-testid={`agent-card-${tr.id}`}
                                  onClick={() =>
                                    store.drillIntoNode(tr.scope || tr.role, {
                                      traceId: tr.id,
                                      sessionKey: tr.key,
                                    })
                                  }
                                >
                                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7, fontWeight: 600, fontSize: 12.5 }}>
                                      <span className={`${styles.statusDot} ${tone.dot}`} />
                                      <span>{roleLabel(tr.role, isZh, tr.scope, tr.sessionTitle, group.round)}</span>
                                      {tr.model && (
                                        <span className={styles.pill} title={tr.model}>
                                          {formatModelBadge(tr.model, tr.thinking, isZh)}
                                        </span>
                                      )}
                                    </span>
                                    <span className={`${styles.pill} ${tone.pill}`}>
                                      {tr.running ? t('traces.liveBadge') : tr.status}
                                    </span>
                                  </div>
                                  <div style={{ fontSize: 12, color: 'var(--fw-text-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                    {tr.preview || tr.errorSummary || (tr.running ? t('design.liveWaiting') : '—')}
                                  </div>
                                  <div className={styles.roundStepMeta}>
                                    <span>
                                      {timeStr ? `${timeStr}` : ''}
                                      {timeStr && stepDur ? ' · ' : ''}
                                      {stepDur ? (isZh ? `耗时 ${stepDur}` : stepDur) : ''}
                                    </span>
                                    <span style={{ color: 'var(--fw-accent)', fontWeight: 500 }}>
                                      {t('chat.enterSession')} →
                                    </span>
                                  </div>
                                </button>
                              );
                            })}
                          </div>
                        </details>
                      );
                    })}
                  </div>
                )}
              </div>
            </section>
          )}

          {/* Stage 1.5: Independent UI/UX Visual & Interactive Prototype Preview */}
          {(planning?.['ui-prototype'] || planning?.phase === 'prototyping') && (
            <section className={styles.orchBlock} data-testid="flywheel-stage-uiproto">
              <div className={styles.orchBlockHead}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                  <span className={styles.orchAvatar}>◎</span>
                  <strong style={{ fontSize: 13.5 }}>
                    {planning?.['ui-prototype']
                      ? (isZh
                          ? `阶段 1.5 · UI/UX 交互与视觉原型 (Rev #${planning['ui-prototype'].revision})`
                          : `Stage 1.5 · UI/UX Interactive Prototype (Rev #${planning['ui-prototype'].revision})`)
                      : (isZh ? '阶段 1.5 · UI/UX 交互与视觉原型' : 'Stage 1.5 · UI/UX Interactive Prototype')}
                  </strong>
                  <span
                    className={`${styles.pill} ${
                      planning?.['ui-prototype']?.status === 'approved'
                        ? styles.pillSuccess
                        : planning?.['ui-prototype']?.status === 'awaiting-approval'
                          ? styles.pillWarn
                          : planning?.['ui-prototype']?.status === 'skipped'
                            ? ''
                            : !snap.running && !planning?.['ui-prototype']
                              ? styles.pillDanger
                              : styles.pillRunning
                    }`}
                  >
                    {planning?.['ui-prototype']?.status === 'approved'
                      ? (isZh ? '视觉稿已批准' : 'Approved')
                      : planning?.['ui-prototype']?.status === 'awaiting-approval'
                        ? (isZh ? '等待你预览拍板' : 'Awaiting Approval')
                        : planning?.['ui-prototype']?.status === 'skipped'
                          ? (isZh ? '已跳过' : 'Skipped')
                          : !snap.running && !planning?.['ui-prototype']
                            ? (isZh ? '生成异常已暂停' : 'Paused')
                            : (isZh ? '生成原型中…' : 'Generating…')}
                  </span>
                </div>
                {snap.uiPrototypeHtml && (
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                    <button
                      type="button"
                      className={styles.linkBtn}
                      data-testid="uiproto-open-right-pane"
                      onClick={() =>
                        store.openInlineHtmlPreview(
                          isZh
                            ? `UI/UX 交互原型 (Rev #${planning?.['ui-prototype']?.revision ?? 1})`
                            : `UI/UX Prototype (Rev #${planning?.['ui-prototype']?.revision ?? 1})`,
                          planning?.['ui-prototype']?.['html-file'] || snap.uiPrototypeFile || 'ui-prototypes/ui-prototype-000001.html',
                          snap.uiPrototypeHtml,
                          false,
                        )
                      }
                    >
                      {isZh ? '右侧分栏预览 ◫' : 'Side preview ◫'}
                    </button>
                    <button
                      type="button"
                      className={styles.linkBtn}
                      data-testid="uiproto-expand-btn"
                      onClick={() =>
                        store.openInlineHtmlPreview(
                          isZh
                            ? `UI/UX 交互原型 (Rev #${planning?.['ui-prototype']?.revision ?? 1})`
                            : `UI/UX Prototype (Rev #${planning?.['ui-prototype']?.revision ?? 1})`,
                          planning?.['ui-prototype']?.['html-file'] || snap.uiPrototypeFile || 'ui-prototypes/ui-prototype-000001.html',
                          snap.uiPrototypeHtml,
                          true,
                        )
                      }
                    >
                      {isZh ? '全屏放大预览 ⤢' : 'Fullscreen preview ⤢'}
                    </button>
                  </div>
                )}
              </div>

              <div className={styles.orchIndent}>
                {!planning?.['ui-prototype'] ? (
                  <div className={snap.running ? styles.protoCard : styles.blockedBanner} data-testid="uiproto-paused-card">
                    <div style={{ fontSize: 12.5, lineHeight: 1.55 }}>
                      {snap.running
                        ? (isZh
                            ? '正在根据已批准的架构设计生成离线可交互 HTML/CSS/SVG 视觉原型…'
                            : 'Generating self-contained interactive HTML/CSS/SVG UI prototype…')
                        : (planning?.reason ||
                            (isZh ? 'UI/UX 视觉原型生成未完成，请重试或跳过原型直接进入编码。' : 'UI/UX prototype paused; retry or skip to coding.'))}
                    </div>
                    {!snap.running && (
                      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 4 }}>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                          data-testid="uiproto-retry-btn"
                          disabled={snap.busyAction !== null}
                          onClick={() => void store.startRun()}
                        >
                          {isZh ? '重试生成 UI 原型' : 'Retry UI Prototype'}
                        </button>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnSm}`}
                          data-testid="uiproto-skip-btn"
                          disabled={snap.busyAction !== null}
                          onClick={() => void store.skipUIPrototype()}
                        >
                          {isZh ? '跳过原型直接编码' : 'Skip Prototype & Code'}
                        </button>
                      </div>
                    )}
                  </div>
                ) : (
                <div className={styles.protoCard}>
                  {planning['ui-prototype'].summary && (
                    <div style={{ fontSize: 12.5, color: 'var(--fw-text-muted)', lineHeight: 1.55 }}>
                      {planning['ui-prototype'].summary}
                    </div>
                  )}

                  {planning['ui-prototype']['design-tokens'] &&
                    Object.keys(planning['ui-prototype']['design-tokens']!).length > 0 && (
                      <div className={styles.protoTokensRow}>
                        {Object.entries(planning['ui-prototype']['design-tokens']!).slice(0, 12).map(([k, v]) => (
                          <span key={k} className={styles.protoTokenChip}>
                            {/^#([0-9a-f]{3,8})$|^rgb|^hsl/i.test(v) && (
                              <span className={styles.protoTokenSwatch} style={{ background: v }} />
                            )}
                            <span>{k}: {v}</span>
                          </span>
                        ))}
                      </div>
                    )}

                  {planning['ui-prototype']['html-file'] && (
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                      <button
                        type="button"
                        className={styles.artifactLinkChip}
                        data-testid="uiproto-file-link"
                        onClick={() => {
                          const rel = planning['ui-prototype']!['html-file']!;
                          if (snap.uiPrototypeHtml) {
                            store.openInlineHtmlPreview(
                              rel.split('/').pop() || rel,
                              rel,
                              snap.uiPrototypeHtml,
                              false,
                            );
                          } else {
                            void store.openArtifactPreview(rel);
                          }
                        }}
                      >
                        📄 {planning['ui-prototype']['html-file']} · {isZh ? '右侧交互预览' : 'Preview'}
                      </button>
                      <button
                        type="button"
                        className={styles.artifactLinkChip}
                        data-testid="uiproto-open-browser"
                        onClick={() => void store.openArtifactExternal(planning['ui-prototype']!['html-file']!)}
                      >
                        🌐 {isZh ? '在浏览器打开 ↗' : 'Open in browser ↗'}
                      </button>
                    </div>
                  )}

                  {snap.uiPrototypeHtml ? (
                    <div className={styles.protoFrameWrap}>
                      <HtmlPreviewFrame
                        title="UI/UX Interactive Prototype Preview"
                        html={snap.uiPrototypeHtml}
                        className={styles.protoFrame}
                        testId="uiproto-iframe"
                      />
                    </div>
                  ) : planning['ui-prototype'].status !== 'skipped' ? (
                    <div style={{ fontSize: 12, color: 'var(--fw-text-subtle)', padding: '12px 0' }}>
                      {isZh ? '正在加载交互式 HTML 视觉原型预览…' : 'Loading interactive HTML prototype preview…'}
                    </div>
                  ) : null}

                  {planning['ui-prototype'].review && (
                    <div
                      style={{
                        padding: '8px 10px',
                        borderRadius: 8,
                        background: 'var(--fw-bg)',
                        border: '1px solid var(--fw-border)',
                        fontSize: 12,
                        color: 'var(--fw-text-muted)',
                      }}
                    >
                      <strong>
                        {isZh ? '独立 UX 审查' : 'UX Review'} ({planning['ui-prototype'].review.verdict}):
                      </strong>{' '}
                      {planning['ui-prototype'].review.summary}
                    </div>
                  )}

                  {planning['ui-prototype'].status === 'awaiting-approval' && (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                      <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                        <input
                          type="text"
                          className={styles.input}
                          style={{ flex: 1, minWidth: 220 }}
                          value={protoFeedback}
                          onChange={e => setProtoFeedback(e.target.value)}
                          placeholder={isZh ? '对配色、布局或交互有意见？输入修改意见后重新生成原型…' : 'Request visual or layout changes…'}
                        />
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnSm}`}
                          disabled={!protoFeedback.trim() || snap.busyAction !== null}
                          onClick={async () => {
                            const text = protoFeedback.trim();
                            if (!text) return;
                            setProtoFeedback('');
                            await store.submitUIPrototypeFeedback(text);
                          }}
                        >
                          {isZh ? '提交视觉修改' : 'Revise Prototype'}
                        </button>
                      </div>
                      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnSm}`}
                          disabled={snap.busyAction !== null}
                          onClick={() => void store.skipUIPrototype()}
                        >
                          {isZh ? '跳过原型直接编码' : 'Skip Prototype'}
                        </button>
                        <button
                          type="button"
                          className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                          data-testid="uiproto-approve-btn"
                          disabled={snap.busyAction !== null}
                          onClick={() => void store.approveUIPrototype()}
                        >
                          {isZh ? '批准视觉原型并开始编码 →' : 'Approve Prototype & Code →'}
                        </button>
                      </div>
                    </div>
                  )}
                </div>
                )}
              </div>
            </section>
          )}

          {/* Investigation Block */}
          {investigation && (
            <section className={styles.orchBlock} data-testid="flywheel-stage-investigation">
              <div className={styles.orchBlockHead}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span className={styles.orchAvatar}>◎</span>
                  <strong style={{ fontSize: 13.5 }}>{t('chat.stage.investigation')}</strong>
                  {investigation.status && <span className={styles.pill}>{investigation.status}</span>}
                </div>
                <div style={{ display: 'flex', gap: 6 }}>
                  {!snap.running && (
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnSm}`}
                      disabled={snap.busyAction !== null}
                      onClick={() => void store.runInvestigation()}
                    >
                      {t('inv.run')}
                    </button>
                  )}
                </div>
              </div>

              <div className={styles.orchIndent}>
                {investigationTraces.length > 0 && (
                  <div className={styles.subAgentGrid}>
                    {investigationTraces.map(tr => {
                      const tone = traceTone(tr.status, tr.running);
                      return (
                        <button
                          key={tr.id}
                          type="button"
                          className={styles.subAgentCard}
                          onClick={() =>
                            store.drillIntoNode(tr.scope || 'investigate', { traceId: tr.id, sessionKey: tr.key })
                          }
                        >
                          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 6 }}>
                            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontWeight: 600, fontSize: 12.5 }}>
                              <span className={`${styles.statusDot} ${tone.dot}`} />
                              <span>{roleLabel(tr.role, isZh)}</span>
                            </span>
                            <span className={`${styles.pill} ${tone.pill}`}>{tr.status}</span>
                          </div>
                          <div style={{ fontSize: 11.5, color: 'var(--fw-text-muted)' }}>
                            {tr.preview || formatModelBadge(tr.model, tr.thinking, isZh) || tr.harness || '—'}
                          </div>
                        </button>
                      );
                    })}
                  </div>
                )}

                {investigation.hypotheses && investigation.hypotheses.length > 0 && (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                    {investigation.hypotheses.map((h, i) => (
                      <div key={h.id || i} className={styles.card} style={{ padding: '10px 12px' }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                          <strong style={{ fontSize: 12.5 }}>{h.title || h.statement || `H${i + 1}`}</strong>
                          {h.status && <span className={styles.pill}>{h.status}</span>}
                        </div>
                        {h.statement && h.title && (
                          <div style={{ fontSize: 12, color: 'var(--fw-text-muted)', marginTop: 4 }}>{h.statement}</div>
                        )}
                      </div>
                    ))}
                  </div>
                )}

                {investigation.report?.conclusion && (
                  <div className={styles.card} style={{ padding: '12px 14px' }}>
                    <strong style={{ fontSize: 12.5 }}>{t('inv.report')}</strong>
                    <div style={{ fontSize: 12.5, color: 'var(--fw-text-muted)', marginTop: 6, lineHeight: 1.6 }}>
                      {investigation.report.conclusion}
                    </div>
                  </div>
                )}
              </div>
            </section>
          )}

          {/* Stage 2: Parallel Execution Wave Cards */}
          {waves.map(wave => {
            const doneCount = wave.items.filter(p =>
              ['done', 'completed', 'passed'].includes((p.status ?? '').toLowerCase()),
            ).length;
            return (
              <section key={wave.index} className={styles.orchBlock} data-testid={`flywheel-wave-${wave.index}`}>
                <div className={styles.orchBlockHead}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <span className={styles.orchAvatar}>◎</span>
                    <strong style={{ fontSize: 13.5 }}>{t('chat.stage.execution', { n: wave.index })}</strong>
                    <span className={styles.pill}>{doneCount}/{wave.items.length}</span>
                  </div>
                  {wave.index === 1 && (
                    <button
                      type="button"
                      className={styles.linkBtn}
                      onClick={() => store.openDrawer('dag')}
                    >
                      {t('chat.openDagDrawer')}
                    </button>
                  )}
                </div>

                <div className={styles.orchIndent}>
                  {wave.items.map(packet => {
                    const tone = packetTone(packet.status);
                    const tr = traceByPacket.get(packet.id) ?? null;
                    const needsAttention = ['failed', 'blocked', 'escalated', 'inbox'].includes((packet.status ?? '').toLowerCase());
                    const pktThinking = typeof (packet as Record<string, unknown>).thinking === 'string'
                      ? String((packet as Record<string, unknown>).thinking)
                      : '';
                    const pktModelLabel = formatModelBadge(packet.model || tr?.model, pktThinking || tr?.thinking, isZh) || packet.route;
                    const inboxList = Array.isArray((snap.state as { inbox?: unknown } | null)?.inbox)
                      ? ((snap.state as { inbox?: Array<{ 'packet-id'?: string; reason?: string; at?: string }> }).inbox ?? [])
                      : [];
                    const inboxEntry = (packet.status ?? '').toLowerCase() === 'inbox'
                      ? [...inboxList].reverse().find(item => item?.['packet-id'] === packet.id) ?? null
                      : null;
                    const pktTraces = packetTracesMap.get(packet.id) ?? [];
                    const pktRevisions = revisionsByPacket.get(packet.id) ?? [];
                    const latestReviewTrace = [...pktTraces]
                      .reverse()
                      .find(item => item.role === 'review' || item.role === 'frontend-ux-review') ?? null;
                    const isCompleted = ['done', 'completed', 'passed'].includes((packet.status ?? '').toLowerCase());
                    const roleCounts: Record<string, number> = {};
                    return (
                      <div key={packet.id} style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
                        <button
                          type="button"
                          className={`${styles.subAgentRowCard} ${tone.rowExtra}`}
                          style={pktTraces.length > 0 ? { borderBottomLeftRadius: 0, borderBottomRightRadius: 0 } : undefined}
                          data-testid={`packet-card-${packet.id}`}
                          onClick={() =>
                            store.drillIntoNode(packet.id, {
                              traceId: tr?.id ?? null,
                              sessionKey: packet.sessionKey || tr?.key || `execute/${packet.id}`,
                              packetId: packet.id,
                            })
                          }
                        >
                          <div style={{ display: 'flex', flexDirection: 'column', gap: 4, minWidth: 0, flex: 1 }}>
                            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                              <span className={`${styles.statusDot} ${tone.dot}`} />
                              <span className={styles.packetId}>{packet.id}</span>
                              <strong style={{ fontSize: 13, fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                {packet.title || packet.goal || packet.id}
                              </strong>
                            </div>
                            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', fontSize: 11.5, color: 'var(--fw-text-muted)' }}>
                              {pktModelLabel && <span title={packet.model || tr?.model || undefined}>{pktModelLabel}</span>}
                              {packet.lastError && (
                                <span style={{ color: 'var(--fw-danger)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                  {packet.lastError}
                                </span>
                              )}
                              {!packet.lastError && (tr?.preview || packet.summary) && (
                                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                  {tr?.preview || packet.summary}
                                </span>
                              )}
                            </div>
                          </div>

                          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
                            <span className={`${styles.pill} ${tone.pill}`}>
                              {packet.status || 'pending'}
                            </span>
                            <span style={{ fontSize: 12, fontWeight: 500, color: needsAttention ? 'var(--fw-danger)' : 'var(--fw-accent)' }}>
                              {needsAttention ? t('chat.enterAction') : t('chat.enterSession')} →
                            </span>
                          </div>
                        </button>
                        {pktTraces.length > 0 && (
                          <div className={styles.packetTimelineRow} data-testid={`packet-timeline-${packet.id}`}>
                            <div className={styles.packetTimelineStrip}>
                              {pktTraces.map((stepTr, sIdx) => {
                                roleCounts[stepTr.role] = (roleCounts[stepTr.role] ?? 0) + 1;
                                const nth = roleCounts[stepTr.role];
                                const shortRole =
                                  stepTr.role === 'execute'
                                    ? (isZh ? `执行 #${nth}` : `Exec #${nth}`)
                                    : stepTr.role === 'frontend-ux-review'
                                      ? (isZh ? `UX审 #${nth}` : `UX #${nth}`)
                                      : (isZh ? `审查 #${nth}` : `Review #${nth}`);
                                const sig = parseReviewStepSignal(stepTr);
                                const chipTone =
                                  sig.tone === 'pass'
                                    ? styles.packetStepChipPass
                                    : sig.tone === 'fail'
                                      ? styles.packetStepChipFail
                                      : sig.tone === 'warn'
                                        ? styles.packetStepChipWarn
                                        : sig.tone === 'running'
                                          ? styles.packetStepChipRunning
                                          : '';
                                const dur = formatDurationShort(stepTr.durationMs);
                                return (
                                  <span key={stepTr.id} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                                    {sIdx > 0 && <span className={styles.packetStepArrow} aria-hidden="true">→</span>}
                                    <button
                                      type="button"
                                      className={`${styles.packetStepChip} ${chipTone}`}
                                      data-testid={`packet-step-${stepTr.id}`}
                                      title={`${shortRole}${stepTr.model ? ` · ${formatModelBadge(stepTr.model, stepTr.thinking, isZh)}` : ''}${dur ? ` · ${dur}` : ''}`}
                                      onClick={() =>
                                        store.drillIntoNode(packet.id, {
                                          traceId: stepTr.id,
                                          sessionKey: stepTr.key,
                                          packetId: packet.id,
                                        })
                                      }
                                    >
                                      <span>{sig.icon}</span>
                                      <span>{shortRole}</span>
                                      {sig.scoreLabel && <span style={{ fontFamily: 'var(--fw-mono)', fontSize: 10.5 }}>{sig.scoreLabel}</span>}
                                      {dur && <span style={{ opacity: 0.72, fontSize: 10 }}>{dur}</span>}
                                    </button>
                                  </span>
                                );
                              })}
                              {pktRevisions.map(rev => {
                                const latestRevTr = rev.items[rev.items.length - 1];
                                return (
                                  <span key={rev.scope} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                                    <span className={styles.packetStepArrow} aria-hidden="true">→</span>
                                    <button
                                      type="button"
                                      className={`${styles.packetStepChip} ${rev.running ? styles.packetStepChipRunning : styles.packetStepChipWarn}`}
                                      data-testid={`packet-revision-link-${rev.requestId}`}
                                      onClick={() => {
                                        if (latestRevTr) {
                                          store.drillIntoNode(latestRevTr.scope || latestRevTr.role, {
                                            traceId: latestRevTr.id,
                                            sessionKey: latestRevTr.key,
                                          });
                                        }
                                      }}
                                    >
                                      <span>🔄</span>
                                      <span>{isZh ? '计划修订' : 'Plan Rev'}</span>
                                    </button>
                                  </span>
                                );
                              })}
                            </div>
                            {!isCompleted && latestReviewTrace && latestReviewTrace.preview && (
                              <div className={styles.packetLatestReviewHint}>
                                {latestReviewTrace.preview}
                              </div>
                            )}
                          </div>
                        )}
                        {inboxEntry && (
                          <div className={styles.decisionCard} data-testid={`inbox-escalation-${packet.id}`} style={{ marginTop: 6, borderColor: 'var(--fw-warn-border)', background: 'var(--fw-warn-soft)' }}>
                            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, flexWrap: 'wrap' }}>
                              <strong style={{ fontSize: 12.5, color: 'var(--fw-warn)' }}>
                                {isZh ? `已暂停并转交人工收件箱 · ${packet.id}` : `Escalated to Human Inbox · ${packet.id}`}
                              </strong>
                              <span className={`${styles.pill} ${styles.pillWarn}`}>{isZh ? '已达自动返修上限' : 'Auto-retry limit reached'}</span>
                            </div>
                            <div style={{ fontSize: 12, color: 'var(--fw-text)', lineHeight: 1.55 }}>
                              {inboxEntry.reason}
                            </div>
                            <div style={{ fontSize: 11.5, color: 'var(--fw-text-muted)', lineHeight: 1.5 }}>
                              {isZh
                                ? '系统按安全护栏自动停止（未强行放过未闭合的验证项）。请点击上方节点查看完整审查意见与工作区现场，修复或确认后重新启动。'
                                : 'Paused safely by escalation guardrail. Open the packet above to inspect the full review verdict before resuming.'}
                            </div>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </section>
            );
          })}

          {/* Mid-Execution Plan Revisions (shown chronologically AFTER Stage 3 waves so flow jumps are immediately visible) */}
          {midExecPlanRevisions.length > 0 && (
            <section className={styles.orchBlock} data-testid="mid-exec-plan-revisions">
              <div className={styles.orchBlockHead}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                  <span className={styles.orchAvatar}>↻</span>
                  <strong style={{ fontSize: 13.5 }}>
                    {isZh ? '执行期计划修订与验收补强' : 'Mid-Execution Plan Revisions'}
                  </strong>
                  <span className={`${styles.pill} ${styles.pillWarn}`}>
                    {isZh ? `共 ${midExecPlanRevisions.length} 次修订` : `${midExecPlanRevisions.length} revisions`}
                  </span>
                </div>
              </div>

              <div className={styles.orchIndent}>
                <div className={styles.roundTimeline}>
                  {midExecPlanRevisions.map((rev, rIdx) => {
                    const isLatest = rIdx === midExecPlanRevisions.length - 1;
                    const durStr = formatDurationShort(rev.totalDurationMs);
                    return (
                      <details
                        key={rev.scope}
                        className={styles.planRevisionCard}
                        data-testid={`plan-revision-card-${rev.requestId}`}
                        open={isLatest || rev.running}
                      >
                        <summary className={styles.roundGroupHead}>
                          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                            <span
                              className={`${styles.statusDot} ${
                                rev.running ? styles.statusDotRunning : styles.statusDotWarn
                              }`}
                            />
                            <strong>{rev.label}</strong>
                            {rev.triggerPacketId && (
                              <span className={`${styles.pill} ${styles.pillWarn}`}>
                                {isZh ? `触发源 · ${rev.triggerPacketId}` : `From ${rev.triggerPacketId}`}
                              </span>
                            )}
                            <span className={styles.pill}>
                              {isZh ? `${rev.items.length} 个步骤` : `${rev.items.length} steps`}
                            </span>
                          </span>
                          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontSize: 11.5, color: 'var(--fw-text-subtle)' }}>
                            {durStr && <span>{isZh ? `用时 ${durStr}` : durStr}</span>}
                            {rev.running && (
                              <span className={`${styles.pill} ${styles.pillRunning}`}>{t('traces.liveBadge')}</span>
                            )}
                          </span>
                        </summary>
                        {rev.triggerSummary && (
                          <div className={styles.planRevisionTrigger}>{rev.triggerSummary}</div>
                        )}
                        <div className={styles.roundStepsList}>
                          {rev.items.map(tr => {
                            const tone = traceTone(tr.status, tr.running);
                            const timeStr = formatClock(tr.startedAt || tr.updatedAt);
                            const stepDur = formatDurationShort(tr.durationMs);
                            return (
                              <button
                                key={tr.id}
                                type="button"
                                className={styles.roundStepRow}
                                data-testid={`agent-card-${tr.id}`}
                                onClick={() =>
                                  store.drillIntoNode(tr.scope || tr.role, {
                                    traceId: tr.id,
                                    sessionKey: tr.key,
                                  })
                                }
                              >
                                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7, fontWeight: 600, fontSize: 12.5 }}>
                                    <span className={`${styles.statusDot} ${tone.dot}`} />
                                    <span>{roleLabel(tr.role, isZh, tr.scope, tr.sessionTitle)}</span>
                                    {tr.model && (
                                      <span className={styles.pill} title={tr.model}>
                                        {formatModelBadge(tr.model, tr.thinking, isZh)}
                                      </span>
                                    )}
                                  </span>
                                  <span className={`${styles.pill} ${tone.pill}`}>
                                    {tr.running ? t('traces.liveBadge') : tr.status}
                                  </span>
                                </div>
                                <div style={{ fontSize: 12, color: 'var(--fw-text-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                  {tr.preview || tr.errorSummary || (tr.running ? t('design.liveWaiting') : '—')}
                                </div>
                                <div className={styles.roundStepMeta}>
                                  <span>
                                    {timeStr ? `${timeStr}` : ''}
                                    {timeStr && stepDur ? ' · ' : ''}
                                    {stepDur ? (isZh ? `耗时 ${stepDur}` : stepDur) : ''}
                                  </span>
                                  <span style={{ color: 'var(--fw-accent)', fontWeight: 500 }}>
                                    {t('chat.enterSession')} →
                                  </span>
                                </div>
                              </button>
                            );
                          })}
                        </div>
                      </details>
                    );
                  })}
                </div>
              </div>
            </section>
          )}

          {/* Stage 3: Delivery Summary Card */}
          {showDeliveryBlock && (
            <section className={styles.orchBlock} data-testid="flywheel-stage-delivery">
              <div className={styles.orchBlockHead}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span className={styles.orchAvatar}>◎</span>
                  <strong style={{ fontSize: 13.5 }}>{t('chat.stage.delivery')}</strong>
                  {snap.delivery?.branch && <span className={styles.pill}>{snap.delivery.branch}</span>}
                </div>
                <button
                  type="button"
                  className={styles.linkBtn}
                  onClick={() => store.openDrawer('delivery')}
                >
                  {t('chat.openDeliveryDrawer')}
                </button>
              </div>
            </section>
          )}
        </div>
      </div>

      {showMainScrollBottom && (
        <div className={styles.scrollBottomAnchor}>
          <button
            type="button"
            className={styles.scrollBottomBtn}
            data-testid="flywheel-main-scroll-bottom"
            onClick={() => scrollMainToBottom('smooth')}
          >
            ↓ {isZh ? '跳到最新' : 'Jump to latest'}
          </button>
        </div>
      )}

      {/* Floating Main Stream Composer (1:1 DSH Single-Button + Real Upload/Paste/Drop) */}
      <div className={styles.composerDock}>
        <input
          ref={mainFileInputRef}
          type="file"
          accept="image/png,image/jpeg,image/webp,image/gif"
          multiple
          style={{ display: 'none' }}
          onChange={e => {
            if (e.target.files && e.target.files.length > 0) {
              void handleUploadMainImages(e.target.files);
              e.target.value = '';
            }
          }}
        />
        <div
          className={`${styles.composerBox}${dragging ? ` ${styles.composerBoxDrag}` : ''}`}
          onDragOver={(e: DragEvent<HTMLDivElement>) => {
            e.preventDefault();
            if (!dragging) setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e: DragEvent<HTMLDivElement>) => {
            e.preventDefault();
            setDragging(false);
            const imgs = Array.from(e.dataTransfer?.files ?? []).filter(f => f.type.startsWith('image/'));
            if (imgs.length > 0) void handleUploadMainImages(imgs);
          }}
        >
          {showImages && (
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
                      setMainInput(prev => (prev.trim() ? `${prev.trim()} [${img.path}]` : `[${img.path}] `));
                      setShowImages(false);
                    }}
                  >
                    🖼 {img.path}
                  </button>
                ))
              )}
            </div>
          )}

          {mentionQuery !== null && mentionTargets.length > 0 && (
            <div
              className={styles.mentionPopover}
              data-testid="flywheel-mention-popover"
              role="listbox"
              aria-label={isZh ? '选择要 @ 的节点或会话' : 'Select node or session to mention'}
            >
              <div className={styles.mentionPopoverHead}>
                <span>{isZh ? '指定节点或会话直接对话' : 'Route directly to a node or session'}</span>
                <span style={{ fontFamily: 'var(--fw-mono)', fontSize: 10.5 }}>↑↓ · Enter · Esc</span>
              </div>
              {filteredMentions.length === 0 ? (
                <div className={styles.mentionEmpty}>
                  {isZh ? `未找到匹配 "${mentionQuery}" 的节点` : `No node matching "${mentionQuery}"`}
                </div>
              ) : (
                <div className={styles.mentionList}>
                  {(() => {
                    let lastGroup: MentionTarget['group'] | null = null;
                    return filteredMentions.map((item, idx) => {
                      const showGroup = item.group !== lastGroup;
                      lastGroup = item.group;
                      const isActive = idx === mentionActiveIdx;
                      return (
                        <div key={`${item.group}-${item.sessionKey}`}>
                          {showGroup && (
                            <div className={styles.mentionGroupTitle}>
                              {item.group === 'packets'
                                ? (isZh ? '执行节点 (Packets)' : 'Execution Packets')
                                : (isZh ? '规划与审查会话 (Sessions)' : 'Active Sessions')}
                            </div>
                          )}
                          <button
                            type="button"
                            role="option"
                            aria-selected={isActive}
                            data-testid={`mention-option-${item.id}`}
                            className={`${styles.mentionOption}${isActive ? ` ${styles.mentionOptionActive}` : ''}`}
                            onMouseEnter={() => setMentionActiveIdx(idx)}
                            onMouseDown={e => {
                              e.preventDefault();
                              applyMentionTarget(item);
                            }}
                          >
                            <span className={`${styles.statusDot} ${item.dotClass}`} />
                            <span className={styles.mentionOptionLabel}>{item.label}</span>
                            {item.subtitle && (
                              <span className={styles.mentionOptionSub}>{item.subtitle}</span>
                            )}
                          </button>
                        </div>
                      );
                    });
                  })()}
                </div>
              )}
            </div>
          )}

          {selectedMention && (
            <div className={styles.mentionPillRow}>
              <span className={styles.mentionPill} data-testid="flywheel-mention-pill">
                <span>{selectedMention.pillText}</span>
                <button
                  type="button"
                  className={styles.mentionPillRemove}
                  data-testid="flywheel-mention-pill-remove"
                  aria-label={isZh ? '移除指定节点' : 'Remove mention'}
                  onClick={() => {
                    setSelectedMention(null);
                    mainTextareaRef.current?.focus();
                  }}
                >
                  ×
                </button>
              </span>
            </div>
          )}

          <textarea
            ref={mainTextareaRef}
            className={styles.composerTextarea}
            data-testid="flywheel-main-composer"
            rows={2}
            value={mainInput}
            onChange={e => {
              const val = e.target.value;
              setMainInput(val);
              const caret = e.target.selectionStart ?? val.length;
              const prefix = val.slice(0, caret);
              const m = /(?:^|\s)@([^\s@]*)$/.exec(prefix);
              if (m && mentionTargets.length > 0) {
                setMentionQuery(m[1]!);
                setMentionActiveIdx(0);
              } else if (mentionQuery !== null) {
                setMentionQuery(null);
              }
            }}
            onPaste={(e: ClipboardEvent<HTMLTextAreaElement>) => {
              const imgs = Array.from(e.clipboardData?.files ?? []).filter(f => f.type.startsWith('image/'));
              if (imgs.length > 0) {
                e.preventDefault();
                void handleUploadMainImages(imgs);
              }
            }}
            onKeyDown={e => {
              if (mentionQuery !== null && filteredMentions.length > 0) {
                if (e.key === 'ArrowDown') {
                  e.preventDefault();
                  setMentionActiveIdx(i => (i + 1) % filteredMentions.length);
                  return;
                }
                if (e.key === 'ArrowUp') {
                  e.preventDefault();
                  setMentionActiveIdx(i => (i - 1 + filteredMentions.length) % filteredMentions.length);
                  return;
                }
                if ((e.key === 'Enter' || e.key === 'Tab') && !e.shiftKey && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  const chosen = filteredMentions[Math.min(mentionActiveIdx, filteredMentions.length - 1)];
                  if (chosen) applyMentionTarget(chosen);
                  return;
                }
                if (e.key === 'Escape') {
                  e.preventDefault();
                  setMentionQuery(null);
                  return;
                }
              }
              if (
                e.key === 'Backspace' &&
                selectedMention &&
                e.currentTarget.selectionStart === 0 &&
                e.currentTarget.selectionEnd === 0
              ) {
                e.preventDefault();
                setSelectedMention(null);
                return;
              }
              if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                e.preventDefault();
                void handleMainSend();
              }
            }}
            placeholder={
              selectedMention
                ? (isZh ? `直接向 ${selectedMention.pillText} 发送指令…` : `Send directly to ${selectedMention.pillText}…`)
                : mainPlaceholder
            }
          />
          <div className={styles.composerBar}>
            <div className={styles.composerTools} style={{ position: 'relative' }}>
              <button
                type="button"
                className={styles.composerPlusBtn}
                title={t('chat.composer.imagesBtn')}
                aria-label={t('chat.composer.imagesBtn')}
                onClick={() => setMainAttachMenuOpen(v => !v)}
              >
                +
              </button>
              {mainAttachMenuOpen && (
                <div className={styles.attachMenu}>
                  <button
                    type="button"
                    className={styles.attachMenuItem}
                    onClick={() => {
                      setMainAttachMenuOpen(false);
                      mainFileInputRef.current?.click();
                    }}
                  >
                    🖼 {t('chat.composer.uploadLocal')}
                  </button>
                  <button
                    type="button"
                    className={styles.attachMenuItem}
                    onClick={() => {
                      setMainAttachMenuOpen(false);
                      setShowImages(v => !v);
                      void store.loadRepoImages(effectiveCwd || undefined);
                    }}
                  >
                    📁 {t('chat.composer.scanRepo')}
                  </button>
                </div>
              )}
              {mentionTargets.length > 0 && (
                <button
                  type="button"
                  className={styles.composerMetaBtn}
                  data-testid="flywheel-mention-trigger"
                  onClick={() => {
                    setMentionQuery(prev => (prev === null ? '' : null));
                    setMentionActiveIdx(0);
                    mainTextareaRef.current?.focus();
                  }}
                >
                  {t('chat.composer.mentionNode')}
                </button>
              )}
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <button
                type="button"
                className={styles.composerMetaBtn}
                onClick={() => store.openDrawer('settings')}
              >
                {t('chat.composer.routingLink')}
              </button>
              {snap.running && !mainInput.trim() ? (
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
                  data-testid="flywheel-main-send"
                  title={t('chat.composer.send')}
                  aria-label={t('chat.composer.send')}
                  disabled={!mainInput.trim() || snap.busyAction !== null}
                  onClick={() => void handleMainSend()}
                >
                  ↑
                </button>
              )}
            </div>
          </div>
        </div>
        <MainStreamTelemetryDock
          state={snap.state}
          traces={snap.traces}
          maxTaskCalls={
            typeof (snap.effectiveConfig as { config?: { guardrails?: { 'max-task-calls'?: number } } } | null)?.config?.guardrails?.['max-task-calls'] === 'number'
              ? (snap.effectiveConfig as { config?: { guardrails?: { 'max-task-calls'?: number } } }).config!.guardrails!['max-task-calls']
              : null
          }
          isZh={isZh}
        />
      </div>
    </div>
  );
}
