import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import { styles } from 'virtual:flywheel-styles';
import { StyleScope } from '../components/StyleScope.tsx';
import type { ClientRuntime } from '../context.ts';
import { buildTraceRoundLabels, formatModelBadge, normalizePackets, resolvePlanningView, resolveTaskDisplayStatus, type DisplayStatusTone } from '../planning.ts';
import {
  buildPlanningScopeGroups,
  compareMilestoneIds,
  computeMilestoneStartTimestamps,
  extractMilestoneIdFromPacket,
  extractMilestoneIdFromScope,
  formatLocalizedStepStatus,
  formatRetryPlanningLabel,
  inferMilestoneByTimestamp,
  isPacketEffectivelyRunning,
  type PlanScopeGroup,
  resolveRetryPlanningTarget,
} from '../workflowTimeline.ts';
import type { FlywheelStore } from '../store.ts';
import type { Translate } from '../types.ts';
import { ChatStreamView } from './ChatStreamView.tsx';
import { CreateTaskModal } from './CreateTaskModal.tsx';
import { DeliveryTrack } from './DeliveryTrack.tsx';
import { DesignStudio } from './DesignStudio.tsx';
import { ExecutionBoard } from './ExecutionBoard.tsx';
import { PreviewPane } from './PreviewPane.tsx';
import { SessionPopoverButton } from './SessionPopover.tsx';
import { SettingsPage } from './SettingsPage.tsx';
import { SubSessionView } from './SubSessionView.tsx';

function tonePillClass(tone: DisplayStatusTone): string {
  if (tone === 'running') return styles.pillRunning;
  if (tone === 'success') return styles.pillSuccess;
  if (tone === 'danger') return styles.pillDanger;
  if (tone === 'warn') return styles.pillWarn;
  return '';
}

function toneDotClass(tone: DisplayStatusTone): string {
  if (tone === 'running') return styles.statusDotRunning;
  if (tone === 'success') return styles.statusDotSuccess;
  if (tone === 'danger') return styles.statusDotDanger;
  if (tone === 'warn') return styles.statusDotWarn;
  return '';
}

function statusDotForNode(status?: string, running?: boolean): string {
  const s = (status ?? '').toLowerCase();
  if (running || ['running', 'executing', 'reviewing'].includes(s)) return styles.statusDotRunning;
  if (['done', 'completed', 'passed'].includes(s)) return styles.statusDotSuccess;
  if (['failed', 'blocked'].includes(s)) return styles.statusDotDanger;
  if (['escalated', 'stopped', 'cancelled', 'awaiting_approval'].includes(s)) return styles.statusDotWarn;
  return '';
}

function formatDisplayLabel(t: Translate, labelKey: string, fallback: string): string {
  const translated = t(labelKey);
  return translated === labelKey ? fallback : translated;
}

function formatShortTime(iso?: string): string {
  if (!iso) return '';
  const ts = Date.parse(iso);
  if (!Number.isFinite(ts)) return '';
  const d = new Date(ts);
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  const hh = String(d.getHours()).padStart(2, '0');
  const mi = String(d.getMinutes()).padStart(2, '0');
  return `${mm}-${dd} ${hh}:${mi}`;
}

export function FlywheelRailIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <circle cx="12" cy="12" r="9" />
      <circle cx="12" cy="12" r="3" />
      <path d="M12 3v6M19.8 16.5l-5.2-3M4.2 16.5l5.2-3" />
    </svg>
  );
}

function SettingsIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9c.26.604.852.997 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z" />
    </svg>
  );
}

function formatNodeDuration(ms?: number | null): string {
  if (typeof ms !== 'number' || !Number.isFinite(ms) || ms <= 0) return '';
  if (ms < 1000) return `${Math.round(ms)}ms`;
  const sec = Math.round(ms / 1000);
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m${String(sec % 60).padStart(2, '0')}s`;
}

function nodeStatusPill(status?: string, running?: boolean, isZh = true): { label: string; cls: string } {
  const loc = formatLocalizedStepStatus(status, running, isZh);
  const cls =
    loc.tone === 'running'
      ? styles.pillRunning
      : loc.tone === 'ok'
        ? styles.pillSuccess
        : loc.tone === 'danger'
          ? styles.pillDanger
          : loc.tone === 'warn'
            ? styles.pillWarn
            : '';
  return { label: loc.label, cls };
}

interface AgentSwitchItem {
  id: string;
  groupKey: string;
  groupTitle: string;
  label: string;
  meta: string;
  timeText?: string;
  durationText?: string;
  statusLabel?: string;
  statusPillClass?: string;
  dotClass: string;
  running?: boolean;
  traceId?: string | null;
  sessionKey?: string | null;
  packetId?: string | null;
}

export function WorkbenchPage({
  store,
  t,
  runtime,
}: {
  store: FlywheelStore;
  t: Translate;
  runtime?: ClientRuntime;
}) {
  const snap = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  const [taskSwitcherOpen, setTaskSwitcherOpen] = useState(false);
  const [agentSwitcherOpen, setAgentSwitcherOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const headerLeftRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!taskSwitcherOpen && !agentSwitcherOpen) return;
    const handleOutside = (event: MouseEvent) => {
      if (headerLeftRef.current && !headerLeftRef.current.contains(event.target as Node)) {
        setTaskSwitcherOpen(false);
        setAgentSwitcherOpen(false);
      }
    };
    document.addEventListener('mousedown', handleOutside);
    return () => document.removeEventListener('mousedown', handleOutside);
  }, [taskSwitcherOpen, agentSwitcherOpen]);

  const planning = snap.planning ?? snap.state?.planning ?? null;
  const resolvedPlanning = useMemo(() => resolvePlanningView(planning), [planning]);
  const retryTarget = useMemo(() => resolveRetryPlanningTarget(planning, snap.state), [planning, snap.state]);
  const needsApproval = Boolean(resolvedPlanning?.needsHumanApproval);
  const planningBlocked = Boolean(resolvedPlanning?.isBlocked);
  const activeRun = snap.runs.find(r => r.taskDir === snap.selectedTaskDir) ?? null;

  const packetEntries = useMemo(() => normalizePackets(snap.state?.packets), [snap.state?.packets]);

  const currentDisplay = useMemo(
    () =>
      resolveTaskDisplayStatus({
        status: snap.state?.status ?? activeRun?.status,
        planningPhase: activeRun?.planningStatus,
        planning,
        state: snap.state,
        running: snap.running,
      }),
    [snap.state, activeRun?.status, activeRun?.planningStatus, planning, snap.running],
  );

  // Build unified Agent-Team node list in true chronological milestone-nested order:
  // Global Design -> UI Prototype -> Milestone M-01 (Plan -> Waves -> Local Revisions -> Judge) -> M-02 ...
  const isZh = t('tab.traces') === '运行记录';
  const agentItems = useMemo<AgentSwitchItem[]>(() => {
    const list: AgentSwitchItem[] = [];
    const roundLabels = buildTraceRoundLabels(snap.traces, isZh);
    const chronological = [...snap.traces].sort((a, b) => {
      const ta = Date.parse(a.startedAt || a.updatedAt || '') || 0;
      const tb = Date.parse(b.startedAt || b.updatedAt || '') || 0;
      return ta - tb;
    });

    const requests = Array.isArray(planning?.requests) ? planning.requests : [];
    const packetIdSet = new Set(packetEntries.map(p => p.id));
    const milestonesMap = (snap.state as { milestones?: Record<string, { status?: string }> } | null)?.milestones ?? null;
    const milestoneIdsSet = new Set<string>();
    for (const id of Object.keys(milestonesMap ?? {})) milestoneIdsSet.add(id.toUpperCase());
    for (const p of packetEntries) {
      const mid = extractMilestoneIdFromPacket(p.id);
      if (mid) milestoneIdsSet.add(mid);
    }
    for (const tr of chronological) {
      if (tr.role === 'plan' || tr.role === 'plan-review' || tr.role === 'judge') {
        const reqId = (tr.scope ?? '').replace(/^planning:/, '');
        const req = requests.find(r => r.id === reqId || `planning:${r.id}` === tr.scope);
        const mid = extractMilestoneIdFromScope(tr.scope, null, req);
        if (mid) milestoneIdsSet.add(mid);
      }
    }
    const orderedMilestones = [...milestoneIdsSet].sort(compareMilestoneIds);

    const toneToDot = (tone: string) =>
      tone === 'running'
        ? styles.statusDotRunning
        : tone === 'ok'
          ? styles.statusDotSuccess
          : tone === 'danger'
            ? styles.statusDotDanger
            : tone === 'warn'
              ? styles.statusDotWarn
              : '';
    const toneToPill = (tone: string) =>
      tone === 'running'
        ? styles.pillRunning
        : tone === 'ok'
          ? styles.pillSuccess
          : tone === 'danger'
            ? styles.pillDanger
            : tone === 'warn'
              ? styles.pillWarn
              : '';

    // 1. Global Design & UI Prototype traces
    const GLOBAL_ROLES = new Set(['clarify', 'design-plan', 'design-review', 'ui-prototype', 'ui-prototype-review', 'investigate']);
    const globalGrouped = new Map<string, { latest: (typeof snap.traces)[number]; label: string; count: number; orderTs: number }>();
    for (const tr of chronological) {
      if (!GLOBAL_ROLES.has(tr.role)) continue;
      const label = roundLabels.get(tr.id) || `${tr.role}${tr.scope ? ` · ${tr.scope}` : ''}`;
      const gk = `${tr.role}::${label}`;
      const ts = Date.parse(tr.startedAt || tr.updatedAt || '') || 0;
      const prev = globalGrouped.get(gk);
      if (!prev) globalGrouped.set(gk, { latest: tr, label, count: 1, orderTs: ts });
      else {
        prev.count += 1;
        prev.latest = tr;
        prev.orderTs = ts || prev.orderTs;
      }
    }
    // Sort global items by stage group first (Requirements & Architecture -> UI Prototype)
    // so the same group header is never duplicated when later design traces occur after UI prototype.
    const sortedGlobal = [...globalGrouped.values()].sort((a, b) => {
      const aProto = a.latest.role === 'ui-prototype' || a.latest.role === 'ui-prototype-review' ? 1 : 0;
      const bProto = b.latest.role === 'ui-prototype' || b.latest.role === 'ui-prototype-review' ? 1 : 0;
      if (aProto !== bProto) return aProto - bProto;
      return a.orderTs - b.orderTs;
    });
    for (const entry of sortedGlobal) {
      const { latest, label, count } = entry;
      const pill = nodeStatusPill(latest.status, latest.running, isZh);
      const modelMeta = formatModelBadge(latest.model, latest.thinking, isZh) || latest.harness || '';
      const countMeta = count > 1 ? (isZh ? `共 ${count} 次` : `${count} runs`) : '';
      const isProto = latest.role === 'ui-prototype' || latest.role === 'ui-prototype-review';
      list.push({
        id: latest.scope || latest.id,
        groupKey: isProto ? 'global-proto' : 'global-design',
        groupTitle: isProto ? (isZh ? 'UI 交互原型' : 'UI Prototype') : (isZh ? '需求与架构' : 'Requirements & Architecture'),
        label,
        meta: [modelMeta, countMeta].filter(Boolean).join(' · '),
        timeText: formatShortTime(latest.startedAt || latest.updatedAt),
        durationText: !latest.running ? formatNodeDuration(latest.durationMs) : '',
        statusLabel: pill.label,
        statusPillClass: pill.cls,
        dotClass: statusDotForNode(latest.status, latest.running),
        running: Boolean(latest.running),
        traceId: latest.id,
        sessionKey: latest.key,
      });
    }

    // Helper: wave depth sort for packets
    const pktById = new Map(packetEntries.map(p => [p.id, p]));
    const depthCache = new Map<string, number>();
    const getDepth = (id: string, stack = new Set<string>()): number => {
      if (depthCache.has(id)) return depthCache.get(id)!;
      if (stack.has(id)) return 0;
      stack.add(id);
      const pkt = pktById.get(id);
      let d = typeof pkt?.wave === 'number' && Number.isInteger(pkt.wave) && pkt.wave >= 0 ? pkt.wave : 0;
      for (const dep of pkt?.dependsOn ?? pkt?.depends_on ?? []) {
        if (pktById.has(dep)) d = Math.max(d, getDepth(dep, stack) + 1);
      }
      stack.delete(id);
      depthCache.set(id, d);
      return d;
    };
    const packetStepLabel = (role?: string): string => {
      if (role === 'review') return isZh ? '代码审查' : 'Review';
      if (role === 'frontend-ux-review') return isZh ? 'UX 审查' : 'UX Review';
      if (role === 'execute') return isZh ? '节点执行' : 'Execute';
      return '';
    };

    const pushPacketItem = (p: (typeof packetEntries)[number], groupKey: string, groupTitle: string) => {
      const pktTraces = chronological.filter(
        x => x.scope === p.id || x.key === `execute/${p.id}` || x.key === `review/${p.id}`,
      );
      const latestTr = pktTraces[pktTraces.length - 1] ?? null;
      const pktRunning = isPacketEffectivelyRunning(p, pktTraces, snap.running);
      const waveNum = getDepth(p.id) + 1;
      const pill = nodeStatusPill(p.status, pktRunning, isZh);
      const timeRaw = latestTr?.startedAt || latestTr?.updatedAt || p.startedAt;
      const modelMeta =
        formatModelBadge(
          latestTr?.model || p.model,
          latestTr?.thinking || (typeof p.thinking === 'string' ? p.thinking : ''),
          isZh,
        ) ||
        latestTr?.harness ||
        p.route ||
        '';
      const stepTag = packetStepLabel(latestTr?.role);
      const countMeta = pktTraces.length > 1 ? (isZh ? `共 ${pktTraces.length} 轮` : `${pktTraces.length} runs`) : '';
      const cleanTitle = p.title && p.title !== p.id ? `${p.id} ${p.title}` : p.id;
      list.push({
        id: p.id,
        groupKey,
        groupTitle,
        label: `Wave ${waveNum} · ${cleanTitle}`,
        meta: [stepTag, modelMeta, countMeta].filter(Boolean).join(' · '),
        timeText: formatShortTime(timeRaw),
        durationText: !pktRunning ? formatNodeDuration(latestTr?.durationMs) : '',
        statusLabel: pill.label,
        statusPillClass: pill.cls,
        dotClass: statusDotForNode(p.status, pktRunning),
        running: pktRunning,
        traceId: latestTr?.id ?? null,
        sessionKey: latestTr?.key || p.sessionKey || `execute/${p.id}`,
        packetId: p.id,
      });
    };

    // Build chronologically segmented plan groups (shared single-source with Main Stream)
    const allPlanTraces = chronological.filter(tr => tr.role === 'plan' || tr.role === 'plan-review');
    const segmentedPlanGroups = buildPlanningScopeGroups({
      traces: allPlanTraces,
      allTraces: chronological,
      requests,
      isZh,
    });

    const pushPlanGroupSteps = (grp: PlanScopeGroup, groupKey: string, groupTitle: string) => {
      for (const st of grp.steps) {
        const tr = st.trace;
        const modelMeta = formatModelBadge(tr.model, tr.thinking, isZh) || tr.harness || '';
        const suffix = !grp.isMilestonePlan && grp.triggerPacketId
          ? ` · ${grp.triggerPacketId}`
          : !grp.isMilestonePlan && grp.requestId && !grp.requestId.startsWith('milestone/')
            ? ` · ${grp.requestId}`
            : '';
        list.push({
          id: tr.scope || tr.id,
          groupKey,
          groupTitle,
          label: `${st.title}${suffix}`,
          meta: modelMeta,
          timeText: formatShortTime(tr.startedAt || tr.updatedAt),
          durationText: !tr.running ? formatNodeDuration(tr.durationMs) : '',
          statusLabel: st.statusLabel,
          statusPillClass: toneToPill(st.tone),
          dotClass: toneToDot(st.tone),
          running: Boolean(tr.running),
          traceId: tr.id,
          sessionKey: tr.key,
        });
      }
    };

    const assignedGroupScopes = new Set<string>();
    const assignedPackets = new Set<string>();
    const assignedPostTraceIds = new Set<string>();

    const milestoneStarts = computeMilestoneStartTimestamps(orderedMilestones, chronological, requests);

    // 2. Milestone-nested groups (chronological M-01 -> M-02 -> ... with packet-linked local revisions placed right after their packet!)
    for (const msId of orderedMilestones) {
      const rawMsStatus = (milestonesMap?.[msId]?.status ?? milestonesMap?.[msId.toLowerCase()]?.status ?? '').toLowerCase();
      const msPackets = packetEntries
        .filter(p => extractMilestoneIdFromPacket(p.id) === msId)
        .sort((a, b) => getDepth(a.id) - getDepth(b.id) || a.id.localeCompare(b.id));
      const msPlanGroups: PlanScopeGroup[] = [];
      for (const grp of segmentedPlanGroups) {
        if (assignedGroupScopes.has(grp.scope)) continue;
        let mid = grp.milestoneId;
        if (!mid) {
          const firstTs = Date.parse(grp.items[0]?.startedAt || grp.items[0]?.updatedAt || '') || 0;
          mid = inferMilestoneByTimestamp(firstTs, orderedMilestones, milestoneStarts);
        }
        if (mid === msId) {
          msPlanGroups.push(grp);
          assignedGroupScopes.add(grp.scope);
        }
      }
      const msPostTraces = chronological.filter(
        tr =>
          (tr.role === 'judge' || (tr.role === 'frontend-ux-review' && (!tr.scope || !packetIdSet.has(tr.scope)))) &&
          extractMilestoneIdFromScope(tr.scope) === msId,
      );
      if (msPackets.length === 0 && msPlanGroups.length === 0 && msPostTraces.length === 0) continue;

      const msRunning =
        msPlanGroups.some(g => g.running) ||
        msPostTraces.some(t => t.running) ||
        msPackets.some(p => isPacketEffectivelyRunning(p, chronological, snap.running));
      const msBlocked = !msRunning && msPlanGroups.some(g => g.tone === 'danger');
      const statusSuffix =
        rawMsStatus === 'closed'
          ? (isZh ? ' · 已完成' : ' · Completed')
          : msRunning
            ? (isZh ? ' · 进行中' : ' · Running')
            : msBlocked
              ? (isZh ? ' · 规划阻塞' : ' · Blocked')
              : '';
      const groupKey = `milestone:${msId}`;
      const groupTitle = isZh ? `里程碑 ${msId}${statusSuffix}` : `Milestone ${msId}${statusSuffix}`;

      // 2a. Initial milestone planning scopes first
      for (const g of msPlanGroups.filter(x => x.isMilestonePlan)) {
        pushPlanGroupSteps(g, groupKey, groupTitle);
      }
      // 2b. Milestone execution packets in wave order, with packet-triggered local revisions immediately after their packet
      const renderedRevScopes = new Set<string>();
      for (const p of msPackets) {
        assignedPackets.add(p.id);
        pushPacketItem(p, groupKey, groupTitle);
        for (const rev of msPlanGroups.filter(x => !x.isMilestonePlan && x.triggerPacketId === p.id)) {
          renderedRevScopes.add(rev.scope);
          pushPlanGroupSteps(rev, groupKey, groupTitle);
        }
      }
      // 2c. Any remaining non-packet-triggered local revisions within this milestone
      for (const rev of msPlanGroups.filter(x => !x.isMilestonePlan && !renderedRevScopes.has(x.scope))) {
        pushPlanGroupSteps(rev, groupKey, groupTitle);
      }
      // 2d. Milestone verification (Judge / UX review)
      for (const tr of msPostTraces) {
        assignedPostTraceIds.add(tr.id);
        const pill = nodeStatusPill(tr.status, tr.running, isZh);
        const modelMeta = formatModelBadge(tr.model, tr.thinking, isZh) || tr.harness || '';
        list.push({
          id: tr.scope || tr.id,
          groupKey,
          groupTitle,
          label: roundLabels.get(tr.id) || (isZh ? `里程碑裁决 · ${msId}` : `Milestone Judge · ${msId}`),
          meta: modelMeta,
          timeText: formatShortTime(tr.startedAt || tr.updatedAt),
          durationText: !tr.running ? formatNodeDuration(tr.durationMs) : '',
          statusLabel: pill.label,
          statusPillClass: pill.cls,
          dotClass: statusDotForNode(tr.status, tr.running),
          running: Boolean(tr.running),
          traceId: tr.id,
          sessionKey: tr.key,
        });
      }
    }

    // 3. Fallback for any non-milestone plan scopes, packets, or post-packet traces
    for (const grp of segmentedPlanGroups) {
      if (assignedGroupScopes.has(grp.scope)) continue;
      pushPlanGroupSteps(grp, 'fallback-plan', isZh ? '实施拆包规划' : 'Milestone Plan');
    }
    const remainingPackets = packetEntries
      .filter(p => !assignedPackets.has(p.id))
      .sort((a, b) => getDepth(a.id) - getDepth(b.id) || a.id.localeCompare(b.id));
    for (const p of remainingPackets) {
      pushPacketItem(p, 'fallback-exec', isZh ? 'Wave 执行与审查' : 'Wave Execution & Review');
    }
    for (const tr of chronological) {
      if (assignedPostTraceIds.has(tr.id)) continue;
      if (tr.role !== 'judge' && !(tr.role === 'frontend-ux-review' && (!tr.scope || !packetIdSet.has(tr.scope)))) continue;
      const pill = nodeStatusPill(tr.status, tr.running, isZh);
      const modelMeta = formatModelBadge(tr.model, tr.thinking, isZh) || tr.harness || '';
      list.push({
        id: tr.scope || tr.id,
        groupKey: 'fallback-exec',
        groupTitle: isZh ? 'Wave 执行与审查' : 'Wave Execution & Review',
        label: roundLabels.get(tr.id) || `${tr.role}${tr.scope ? ` · ${tr.scope}` : ''}`,
        meta: modelMeta,
        timeText: formatShortTime(tr.startedAt || tr.updatedAt),
        durationText: !tr.running ? formatNodeDuration(tr.durationMs) : '',
        statusLabel: pill.label,
        statusPillClass: pill.cls,
        dotClass: statusDotForNode(tr.status, tr.running),
        running: Boolean(tr.running),
        traceId: tr.id,
        sessionKey: tr.key,
      });
    }

    return list;
  }, [snap.traces, packetEntries, planning?.requests, snap.state, isZh]);

  const activeAgent = useMemo(
    () =>
      agentItems.find(item => item.traceId && item.traceId === snap.selectedTraceId) ??
      agentItems.find(item => item.sessionKey && item.sessionKey === snap.activeSessionKey) ??
      agentItems.find(item => item.id === snap.activeNodeId) ??
      null,
    [agentItems, snap.activeNodeId, snap.selectedTraceId, snap.activeSessionKey],
  );

  const activeSubTrace = useMemo(() => {
    if (snap.traceDetail && (!snap.selectedTraceId || snap.traceDetail.id === snap.selectedTraceId)) {
      return snap.traceDetail;
    }
    return snap.traces.find(tr => tr.id === snap.selectedTraceId) ?? null;
  }, [snap.traceDetail, snap.selectedTraceId, snap.traces]);

  const tempCount = useMemo(() => snap.runs.filter(r => r.isTemp).length, [snap.runs]);
  const visibleRuns = useMemo(() => {
    const base = snap.showTempRuns ? snap.runs : snap.runs.filter(r => !r.isTemp || r.taskDir === snap.selectedTaskDir);
    const q = searchQuery.trim().toLowerCase();
    if (!q) return base;
    return base.filter(r =>
      (r.title || '').toLowerCase().includes(q) ||
      (r.slug || '').toLowerCase().includes(q) ||
      (r.repoName || '').toLowerCase().includes(q) ||
      r.taskDir.toLowerCase().includes(q),
    );
  }, [snap.runs, snap.showTempRuns, snap.selectedTaskDir, searchQuery]);

  const title =
    snap.workbenchView === 'new-task'
      ? t('chat.newTaskHeader')
      : snap.state?.title ||
        snap.state?.slug ||
        activeRun?.title ||
        activeRun?.slug ||
        t('header.noTask');

  const isSubView = snap.workbenchView === 'sub';
  const activeSessionKey = snap.activeSessionKey || activeSubTrace?.key || null;
  const isClaimed = Boolean(activeSessionKey && snap.claimedSessions[activeSessionKey]);

  const handleInterruptTakeover = async () => {
    if (snap.running) {
      await store.stopRun();
    }
    if (activeSessionKey) {
      await store.claimSession(activeSessionKey);
    }
  };

  const drawerTitle =
    snap.activeDrawer === 'design'
      ? t('tab.design')
      : snap.activeDrawer === 'dag'
        ? t('tab.execution')
        : snap.activeDrawer === 'delivery'
          ? t('tab.delivery')
          : snap.activeDrawer === 'settings'
            ? t('tab.settings')
            : '';

  return (
    <StyleScope className={styles.workbenchShell}>
      <div className={styles.mainColumn}>
        {/* Minimal 50px DSH Single-Row Header: Consistent Boxed Switchers (Task / Main Flow / Node) */}
        <header className={styles.header} data-testid="flywheel-header">
          <div className={styles.headerLeft} ref={headerLeftRef}>
            <div className={styles.taskSwitcherWrap}>
              <button
                type="button"
                className={styles.taskSwitcherBtn}
                data-testid="flywheel-task-switcher"
                aria-expanded={taskSwitcherOpen}
                onClick={() => {
                  setAgentSwitcherOpen(false);
                  setTaskSwitcherOpen(v => !v);
                }}
              >
                {snap.workbenchView !== 'new-task' && (
                  <span className={`${styles.statusDot} ${toneDotClass(currentDisplay.tone)}`} />
                )}
                <span className={styles.taskSwitcherLabel}>{title}</span>
                <span className={styles.taskSwitcherCaret}>{taskSwitcherOpen ? '▴' : '▾'}</span>
              </button>

                {taskSwitcherOpen && (
                  <div className={styles.taskSwitcherPopover} data-testid="flywheel-task-popover" role="dialog" aria-label={t('rail.tasks')}>
                    <div className={styles.taskSwitcherHead}>
                      <input
                        type="text"
                        className={styles.taskSwitcherSearch}
                        value={searchQuery}
                        onChange={e => setSearchQuery(e.target.value)}
                        placeholder={t('switcher.placeholder')}
                      />
                      <button
                        type="button"
                        className={`${styles.btn} ${styles.btnSm}`}
                        onClick={() => void store.refreshRuns()}
                        title={t('rail.refresh')}
                      >
                        ↻
                      </button>
                    </div>

                    <div className={styles.taskSwitcherList}>
                      {visibleRuns.length === 0 ? (
                        <div style={{ padding: '16px 12px', textAlign: 'center', fontSize: 12, color: 'var(--fw-text-subtle)' }}>
                          {snap.runs.length === 0 ? t('rail.empty') : t('switcher.noMatch')}
                        </div>
                      ) : (
                        visibleRuns.map(run => {
                          const active = run.taskDir === snap.selectedTaskDir && snap.workbenchView !== 'new-task';
                          const runDisplay = active
                            ? currentDisplay
                            : resolveTaskDisplayStatus({
                                status: run.status,
                                planningPhase: run.planningStatus,
                                running: run.running,
                              });
                          return (
                            <button
                              key={run.taskDir}
                              type="button"
                              className={`${styles.taskSwitcherItem} ${active ? styles.taskSwitcherItemActive : ''}`}
                              onClick={() => {
                                setTaskSwitcherOpen(false);
                                void store.selectTask(run.taskDir, run.cwd);
                              }}
                            >
                              <div className={styles.taskSwitcherItemTop}>
                                <span className={styles.taskSwitcherItemName}>
                                  <span className={`${styles.statusDot} ${toneDotClass(runDisplay.tone)}`} />
                                  <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                                    {run.title || run.slug}
                                  </span>
                                </span>
                                <span className={`${styles.pill} ${tonePillClass(runDisplay.tone)}`}>
                                  {formatDisplayLabel(t, runDisplay.labelKey, runDisplay.code)}
                                </span>
                              </div>
                              <div className={styles.taskSwitcherItemMeta}>
                                <span>
                                  {run.repoName || run.cwd || run.taskDir}
                                  {(run.runCount ?? 1) > 1 ? ` · ${t('switcher.runsCount', { count: run.runCount ?? 1 })}` : ''}
                                </span>
                                <span>{formatShortTime(run.updatedAt)}</span>
                              </div>
                            </button>
                          );
                        })
                      )}
                    </div>

                    <div className={styles.taskSwitcherFooter}>
                      {tempCount > 0 ? (
                        <label style={{ display: 'inline-flex', alignItems: 'center', gap: 6, cursor: 'pointer' }}>
                          <input
                            type="checkbox"
                            checked={snap.showTempRuns}
                            onChange={e => store.setShowTempRuns(e.target.checked)}
                          />
                          <span>{t('switcher.showTemp', { count: tempCount })}</span>
                        </label>
                      ) : (
                        <span>{t('switcher.count', { count: visibleRuns.length })}</span>
                      )}
                      <button
                        type="button"
                        className={`${styles.btn} ${styles.btnSm}`}
                        onClick={() => {
                          setTaskSwitcherOpen(false);
                          store.startNewTaskChat();
                        }}
                      >
                        + {t('rail.newTask')}
                      </button>
                    </div>
                  </div>
                )}
            </div>

            {isSubView && (
              <>
                <span style={{ color: 'var(--fw-text-subtle)', fontSize: 13 }}>/</span>
                <button
                  type="button"
                  className={styles.taskSwitcherBtn}
                  data-testid="flywheel-back-main"
                  onClick={() => store.returnToMainStream()}
                >
                  <span>◎</span>
                  <span className={styles.taskSwitcherLabel}>{t('chat.mainFlow')}</span>
                </button>
                <span style={{ color: 'var(--fw-text-subtle)', fontSize: 13 }}>/</span>
              </>
            )}

            {/* DSH Agent-Team Dropdown Switcher (Main Stream <-> Any Node Provider Session) */}
            {snap.selectedTaskDir && snap.workbenchView !== 'new-task' && (
              <div className={styles.taskSwitcherWrap}>
                <button
                  type="button"
                  className={styles.taskSwitcherBtn}
                  data-testid="flywheel-agent-switcher"
                  aria-expanded={agentSwitcherOpen}
                  onClick={() => {
                    setTaskSwitcherOpen(false);
                    setAgentSwitcherOpen(v => !v);
                  }}
                >
                  {isSubView && activeAgent && <span className={`${styles.statusDot} ${activeAgent.dotClass}`} />}
                  <span className={styles.taskSwitcherLabel}>
                    {isSubView
                      ? activeAgent?.label || snap.activeNodeId || t('chat.mainFlow')
                      : `${t('chat.mainFlow')}${agentItems.length > 0 ? ` · ${t('chat.nodesCount', { count: agentItems.length })}` : ''}`}
                  </span>
                  <span className={styles.taskSwitcherCaret}>{agentSwitcherOpen ? '▴' : '▾'}</span>
                </button>

                {agentSwitcherOpen && (
                  <div
                    className={styles.taskSwitcherPopover}
                    data-testid="flywheel-agent-popover"
                    role="dialog"
                    aria-label={t('chat.agentSwitcherTitle')}
                  >
                    <div className={styles.taskSwitcherList}>
                      <button
                        type="button"
                        className={`${styles.taskSwitcherItem} ${!isSubView ? styles.taskSwitcherItemActive : ''}`}
                        onClick={() => {
                          setAgentSwitcherOpen(false);
                          store.returnToMainStream();
                        }}
                      >
                        <div className={styles.taskSwitcherItemTop}>
                          <span className={styles.taskSwitcherItemName}>
                            <span>◎</span>
                            <span>{t('chat.mainFlow')}</span>
                          </span>
                          <span className={`${styles.pill} ${tonePillClass(currentDisplay.tone)}`}>
                            {formatDisplayLabel(t, currentDisplay.labelKey, currentDisplay.code)}
                          </span>
                        </div>
                        <div className={styles.taskSwitcherItemMeta}>
                          <span>{t('chat.mainFlowDesc')}</span>
                        </div>
                      </button>

                      {(() => {
                        const runningItems = agentItems.filter(it => it.running);
                        const renderSwitcherRow = (item: AgentSwitchItem, key: string) => {
                          const active =
                            isSubView &&
                            (item.traceId && snap.selectedTraceId
                              ? item.traceId === snap.selectedTraceId
                              : (item.sessionKey && item.sessionKey === snap.activeSessionKey) ||
                                snap.activeNodeId === item.id);
                          return (
                            <button
                              key={key}
                              type="button"
                              className={`${styles.taskSwitcherItem} ${active ? styles.taskSwitcherItemActive : ''}`}
                              onClick={() => {
                                setAgentSwitcherOpen(false);
                                store.drillIntoNode(item.id, {
                                  traceId: item.traceId,
                                  sessionKey: item.sessionKey,
                                  packetId: item.packetId,
                                });
                              }}
                            >
                              <div className={styles.taskSwitcherItemTop}>
                                <span className={styles.taskSwitcherItemName}>
                                  <span className={`${styles.statusDot} ${item.dotClass}`} />
                                  <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{item.label}</span>
                                </span>
                                {item.statusLabel && (
                                  <span className={`${styles.pill} ${item.statusPillClass || ''}`} style={{ flexShrink: 0 }}>
                                    {item.statusLabel}
                                  </span>
                                )}
                              </div>
                              <div className={styles.taskSwitcherItemMeta}>
                                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                  {item.meta || '—'}
                                </span>
                                <span style={{ flexShrink: 0, fontFamily: 'var(--fw-mono)', fontSize: 10.5 }}>
                                  {item.timeText || ''}
                                  {item.timeText && item.durationText ? ' · ' : ''}
                                  {item.durationText ? (isZh ? `耗时 ${item.durationText}` : item.durationText) : ''}
                                </span>
                              </div>
                            </button>
                          );
                        };
                        let lastGroupKey: string | null = null;
                        return (
                          <>
                            {runningItems.length > 0 && (
                              <div className={styles.switcherLiveSection} data-testid="switcher-live-quick-jump">
                                <div className={styles.switcherGroupHeader}>
                                  {isZh ? '⚡ 当前运行中' : '⚡ Running Now'}
                                </div>
                                {runningItems.map((item, idx) =>
                                  renderSwitcherRow(item, `live-${item.sessionKey || item.id}-${idx}`),
                                )}
                              </div>
                            )}
                            {agentItems.map((item, idx) => {
                              const showHeader = item.groupKey !== lastGroupKey;
                              lastGroupKey = item.groupKey;
                              return (
                                <div key={`${item.groupKey}-${item.sessionKey || item.id}-${idx}`}>
                                  {showHeader && (
                                    <div className={styles.switcherGroupHeader}>{item.groupTitle}</div>
                                  )}
                                  {renderSwitcherRow(item, `row-${item.sessionKey || item.id}-${idx}`)}
                                </div>
                              );
                            })}
                          </>
                        );
                      })()}
                    </div>
                  </div>
                )}
              </div>
            )}

            {snap.selectedTaskDir && !isSubView && snap.workbenchView !== 'new-task' && (
              <span className={`${styles.pill} ${tonePillClass(currentDisplay.tone)}`} data-testid="flywheel-header-status">
                {formatDisplayLabel(t, currentDisplay.labelKey, currentDisplay.code)}
              </span>
            )}
          </div>

          <div className={styles.headerRight}>
            {isSubView ? (
              <>
                <SessionPopoverButton trace={activeSubTrace} store={store} t={t} runtime={runtime} />
                {snap.running && (
                  <button
                    type="button"
                    className={`${styles.btn} ${styles.btnDanger} ${styles.btnSm}`}
                    data-testid="flywheel-interrupt-takeover"
                    disabled={snap.busyAction !== null}
                    onClick={() => void handleInterruptTakeover()}
                  >
                    {t('chat.takeover.interruptBtn')}
                  </button>
                )}
              </>
            ) : (
              <>
                {snap.selectedTaskDir && snap.workbenchView !== 'new-task' && (
                  <>
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
                      data-testid="drawer-trigger-design"
                      onClick={() => store.openDrawer('design')}
                    >
                      {t('chat.drawer.design')}
                    </button>
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
                      data-testid="drawer-trigger-dag"
                      onClick={() => store.openDrawer('dag')}
                    >
                      {t('chat.drawer.dag')}
                    </button>
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
                      data-testid="drawer-trigger-delivery"
                      onClick={() => store.openDrawer('delivery')}
                    >
                      {t('chat.drawer.delivery')}
                    </button>
                  </>
                )}

                <button
                  type="button"
                  className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
                  data-testid="flywheel-settings-tab"
                  aria-label={t('tab.settings')}
                  title={t('tab.settings')}
                  onClick={() => store.openDrawer('settings')}
                >
                  <SettingsIcon />
                </button>

                {snap.selectedTaskDir && snap.workbenchView !== 'new-task' && (
                  <button
                    type="button"
                    className={`${styles.btn} ${styles.btnSm}`}
                    onClick={() => store.startNewTaskChat()}
                  >
                    + {t('rail.newTask')}
                  </button>
                )}

                {snap.selectedTaskDir && snap.workbenchView !== 'new-task' && snap.running && (
                  <button
                    type="button"
                    className={`${styles.btn} ${styles.btnDanger} ${styles.btnSm}`}
                    disabled={snap.busyAction !== null}
                    onClick={() => void store.stopRun()}
                  >
                    {t('header.cta.stop')}
                  </button>
                )}

                {snap.selectedTaskDir && snap.workbenchView !== 'new-task' && !snap.running && needsApproval && (
                  <button
                    type="button"
                    className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                    disabled={snap.busyAction !== null}
                    onClick={() => void store.approveDesign(true)}
                  >
                    {t('header.cta.approve')}
                  </button>
                )}

                {snap.selectedTaskDir && snap.workbenchView !== 'new-task' && !snap.running && planningBlocked && (
                  <button
                    type="button"
                    className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                    disabled={snap.busyAction !== null}
                    onClick={() => void store.retryPlanning()}
                  >
                    {formatRetryPlanningLabel(retryTarget, isZh)}
                  </button>
                )}

                {snap.selectedTaskDir &&
                  snap.workbenchView !== 'new-task' &&
                  !snap.running &&
                  !needsApproval &&
                  !planningBlocked &&
                  snap.state?.status !== 'completed' &&
                  snap.state?.status !== 'done' && (
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
                      disabled={snap.busyAction !== null}
                      onClick={() => void store.startRun('auto')}
                    >
                      {t('header.cta.start')}
                    </button>
                  )}
              </>
            )}
          </div>
        </header>

        {/* Contextual Top Strip: Attention (Main) or Takeover (Sub-Session) */}
        {!isSubView && snap.workbenchView === 'main' && needsApproval && resolvedPlanning?.design?.revision && (
          <div className={styles.contextStripWarn} data-testid="flywheel-attention-strip">
            <span>{t('chat.attention.approval', { rev: resolvedPlanning.design.revision })}</span>
            <button
              type="button"
              className={`${styles.btn} ${styles.btnSm}`}
              onClick={() => store.openDrawer('design')}
            >
              {t('chat.attention.reviewNow')}
            </button>
          </div>
        )}

        {isSubView && isClaimed && activeSessionKey && (
          <div className={styles.contextStripSuccess} data-testid="flywheel-takeover-strip">
            <div>
              <strong>{t('chat.takeover.activeTitle')}</strong>
              <span style={{ marginLeft: 8, opacity: 0.9 }}>{t('chat.takeover.activeDesc')}</span>
            </div>
            <button
              type="button"
              className={`${styles.btn} ${styles.btnPrimary} ${styles.btnSm}`}
              data-testid="flywheel-hand-back"
              disabled={snap.busyAction !== null}
              onClick={() => void store.returnSession(activeSessionKey)}
            >
              {t('chat.takeover.handBack')}
            </button>
          </div>
        )}

        {(snap.notice === 'warp-opened' || snap.notice === 'terminal-opened') && (
          <div className={styles.contextStripSuccess} data-testid="flywheel-terminal-notice">
            <span>{snap.notice === 'warp-opened' ? t('chat.session.warpLaunched') : t('traces.terminalLaunched')}</span>
            <button
              type="button"
              className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
              onClick={() => store.clearNotice()}
            >
              ×
            </button>
          </div>
        )}

        {snap.error && snap.activeDrawer !== 'settings' && (
          <div className={styles.bannerError} role="alert" data-testid="flywheel-error-banner" style={{ margin: '10px 20px 0' }}>
            <span>{snap.error.message}</span>
            <button type="button" className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`} onClick={() => store.clearError()}>
              ×
            </button>
          </div>
        )}

        {snap.runsError && snap.activeDrawer !== 'settings' && (
          <div className={styles.bannerError} role="alert" style={{ margin: '10px 20px 0' }}>
            <span>{snap.runsError}</span>
          </div>
        )}

        {/* Main Chat Canvas or Drilled-In Provider Sub-Session + Docked Right-Side Preview Pane */}
        <main style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'row', position: 'relative', overflow: 'hidden' }}>
          <div
            style={{
              flex: 1,
              minWidth: 0,
              minHeight: 0,
              display: snap.previewArtifact?.fullscreen ? 'none' : 'flex',
              flexDirection: 'column',
              position: 'relative',
              overflow: 'hidden',
            }}
          >
            {isSubView ? (
              <SubSessionView snap={snap} store={store} t={t} runtime={runtime} />
            ) : (
              <ChatStreamView snap={snap} store={store} t={t} />
            )}
          </div>
          {snap.previewArtifact && <PreviewPane snap={snap} store={store} isZh={t('tab.traces') === '运行记录'} />}

          {/* On-Demand Slide-Over Drawers (Design / DAG / Delivery / Settings) */}
          {snap.activeDrawer && (
            <div
              className={styles.sideDrawerBackdrop}
              data-testid="flywheel-side-drawer"
              onClick={e => {
                if (e.target === e.currentTarget) store.closeDrawer();
              }}
            >
              <aside className={styles.sideDrawerPanel} role="dialog" aria-label={drawerTitle}>
                <div className={styles.sideDrawerHeader}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                    <button
                      type="button"
                      className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
                      onClick={() => store.closeDrawer()}
                    >
                      ← {t('chat.mainFlow')}
                    </button>
                    <strong style={{ fontSize: 14 }}>{drawerTitle}</strong>
                  </div>
                  <button
                    type="button"
                    className={`${styles.btn} ${styles.btnGhost} ${styles.btnSm}`}
                    data-testid="flywheel-drawer-close"
                    aria-label={t('chat.drawer.close')}
                    onClick={() => store.closeDrawer()}
                  >
                    ×
                  </button>
                </div>
                <div className={styles.sideDrawerBody}>
                  {snap.activeDrawer === 'design' && <DesignStudio snap={snap} store={store} t={t} />}
                  {snap.activeDrawer === 'dag' && <ExecutionBoard snap={snap} store={store} t={t} runtime={runtime} />}
                  {snap.activeDrawer === 'delivery' && <DeliveryTrack snap={snap} store={store} t={t} />}
                  {snap.activeDrawer === 'settings' && <SettingsPage store={store} t={t} inline />}
                </div>
              </aside>
            </div>
          )}
        </main>
      </div>

      <CreateTaskModal snap={snap} store={store} t={t} />
    </StyleScope>
  );
}
