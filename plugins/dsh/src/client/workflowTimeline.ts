import type {
  PacketNode,
  PlanningRequestRecord,
  PlanningState,
  TaskState,
  TraceSummary,
} from './types.ts';

export type SemanticTone = 'running' | 'ok' | 'warn' | 'danger' | 'neutral';

export interface ResolvedRetryTarget {
  scope: 'design' | 'milestone' | 'packet';
  scopeId: string;
  continueExecution: boolean;
  failedRequestId: string;
  cancellableRequestId: string;
}

export interface AnnotatedPlanStep {
  trace: TraceSummary;
  round: number;
  retryIndex: number;
  isSupersededRetry: boolean;
  title: string;
  verdict: 'pass' | 'revise' | 'blocked' | 'needs-user-decision' | null;
  statusLabel: string;
  tone: SemanticTone;
}

export interface PlanScopeGroup {
  scope: string;
  requestId: string;
  milestoneId: string | null;
  isMilestonePlan: boolean;
  label: string;
  triggerPacketId: string | null;
  triggerSummary: string | null;
  items: TraceSummary[];
  steps: AnnotatedPlanStep[];
  running: boolean;
  tone: SemanticTone;
  statusLabel: string;
  totalDurationMs: number;
}

export interface MilestoneTimelineNode {
  id: string;
  title: string;
  status: 'closed' | 'running' | 'blocked' | 'budget-stopped' | 'escalated' | 'paused' | 'pending';
  tone: SemanticTone;
  statusLabel: string;
  isClosed: boolean;
  isRunning: boolean;
  isAttention: boolean;
  isPending: boolean;
  hasBlockedPlanning: boolean;
  planningBlockedDetail: string | null;
  planningGroups: PlanScopeGroup[];
  localRevisions: PlanScopeGroup[];
  packets: PacketNode[];
  verificationTraces: TraceSummary[];
  completedPackets: number;
  totalPackets: number;
  totalDurationMs: number;
}

export function extractMilestoneIdFromPacket(packetId?: string | null): string | null {
  if (!packetId) return null;
  const m = /^M(\d+)-T\d+$/i.exec(packetId.trim());
  if (!m) return null;
  return `M-${m[1]!.padStart(2, '0')}`;
}

export function extractMilestoneIdFromScope(
  scope?: string | null,
  triggerPacketId?: string | null,
  request?: PlanningRequestRecord | null,
): string | null {
  if (request?.scope === 'milestone' && request['scope-id']) {
    return request['scope-id'].toUpperCase();
  }
  if (request?.scope === 'packet' && request['scope-id']) {
    return extractMilestoneIdFromPacket(request['scope-id']);
  }
  const s = (scope ?? '').trim();
  const direct = /(?:^|[:/])(M-\d{2,})(?:$|[:/])/i.exec(s);
  if (direct) return direct[1]!.toUpperCase();
  const fromPkt = /(M\d{2,}-T\d{2,})/i.exec(s);
  if (fromPkt) return extractMilestoneIdFromPacket(fromPkt[1]);
  // Compact unhyphenated packet/milestone tokens inside slugged request IDs (e.g. "strengthen-m01t01-l1").
  const compactPkt = /(?:^|[^a-z0-9])m(\d{1,2})t\d{1,2}(?:[^a-z0-9]|$)/i.exec(s);
  if (compactPkt) return `M-${compactPkt[1]!.padStart(2, '0')}`;
  const compactMs = /(?:^|[-_/:])m(\d{2})(?:[-_/:]|$)/i.exec(s);
  if (compactMs) return `M-${compactMs[1]!.padStart(2, '0')}`;
  if (triggerPacketId) return extractMilestoneIdFromPacket(triggerPacketId);
  return null;
}

export function isPacketEffectivelyRunning(
  packet: PacketNode,
  traces: TraceSummary[],
  taskRunning: boolean,
): boolean {
  const hasRunningTrace = traces.some(
    t => t.running && (t.scope === packet.id || t.scope === `final:${packet.id}` || t.key.endsWith(`/${packet.id}`)),
  );
  if (hasRunningTrace) return true;
  if (!taskRunning) return false;
  const st = (packet.status ?? '').toLowerCase();
  return ['running', 'executing', 'reviewing', 'verifying', 'in-progress', 'in_progress'].includes(st);
}

/**
 * Attributes a legacy unscoped planning trace/group (e.g. `engine-1-6`) to the
 * chronologically active milestone based on observed milestone start boundaries,
 * preventing historical M-01 revisions from appearing after active M-02.
 */
export function inferMilestoneByTimestamp(
  timestampMs: number,
  orderedMilestones: string[],
  milestoneStartTs: Map<string, number>,
): string | null {
  if (orderedMilestones.length === 0) return null;
  if (!Number.isFinite(timestampMs) || timestampMs <= 0) return orderedMilestones[0]!;
  let chosen = orderedMilestones[0]!;
  for (const msId of orderedMilestones) {
    const startTs = milestoneStartTs.get(msId);
    if (typeof startTs === 'number' && Number.isFinite(startTs) && startTs > 0 && timestampMs >= startTs) {
      chosen = msId;
    }
  }
  return chosen;
}

export function computeMilestoneStartTimestamps(
  orderedMilestones: string[],
  traces: TraceSummary[],
  requests: PlanningRequestRecord[] = [],
): Map<string, number> {
  const starts = new Map<string, number>();
  const record = (msId: string | null, iso?: string) => {
    if (!msId || !orderedMilestones.includes(msId)) return;
    const ts = Date.parse(iso || '') || 0;
    if (ts <= 0) return;
    const prev = starts.get(msId);
    if (prev === undefined || ts < prev) starts.set(msId, ts);
  };
  for (const tr of traces) {
    if (tr.role === 'plan' || tr.role === 'plan-review' || tr.role === 'judge') {
      const reqId = (tr.scope ?? '').replace(/^planning:/, '');
      const req = requests.find(r => r.id === reqId || `planning:${r.id}` === tr.scope);
      record(extractMilestoneIdFromScope(tr.scope, null, req), tr.startedAt || tr.updatedAt);
    } else if (tr.scope) {
      const pktId = tr.scope.replace(/^final:/, '');
      record(extractMilestoneIdFromPacket(pktId), tr.startedAt || tr.updatedAt);
    }
  }
  return starts;
}

export function compareMilestoneIds(a: string, b: string): number {
  const ma = /^M-(\d+)$/i.exec(a);
  const mb = /^M-(\d+)$/i.exec(b);
  if (ma && mb) return Number(ma[1]) - Number(mb[1]);
  return a.localeCompare(b);
}

/**
 * Resolves the authoritative planning retry target, preferring the Go engine's
 * read-only console projection (`planning.projection['retry-target']`) and
 * falling back to the exact same deterministic derivation for legacy snapshots.
 */
export function resolveRetryPlanningTarget(
  planning?: PlanningState | null,
  state?: TaskState | null,
): ResolvedRetryTarget {
  const projected = planning?.projection?.['retry-target'];
  if (projected && (projected.scope === 'design' || projected.scope === 'milestone' || projected.scope === 'packet')) {
    return {
      scope: projected.scope,
      scopeId: projected['scope-id'] ?? '',
      continueExecution: Boolean(projected['continue-execution']),
      failedRequestId: projected['failed-request-id'] ?? '',
      cancellableRequestId: projected['cancellable-request-id'] ?? '',
    };
  }

  const startAllowed = Boolean(planning?.['start-authorized']);
  const requests = Array.isArray(planning?.requests) ? planning.requests : [];
  let scope: 'design' | 'milestone' | 'packet' = 'design';
  let scopeId = '';
  let continueExecution = false;
  let failedRequestId = '';
  let cancellableRequestId = '';

  let superseded = false;
  for (let i = requests.length - 1; i >= 0; i -= 1) {
    const r = requests[i]!;
    const st = (r.status ?? '').toLowerCase();
    if (!cancellableRequestId && (st === 'pending' || st === 'running')) {
      cancellableRequestId = r.id;
    }
    if (st === 'completed') {
      superseded = true;
      continue;
    }
    if (!superseded && !failedRequestId && (st === 'budget-stopped' || st === 'blocked' || st === 'awaiting-input')) {
      failedRequestId = r.id;
      const rawScope = r.scope === 'replan'
        ? (r['scope-id'] && /^M-\d{2,}$/i.test(r['scope-id'])
          ? 'milestone'
          : r['scope-id'] && /^M\d{2,}-T\d{2,}$/i.test(r['scope-id'])
            ? 'packet'
            : 'design')
        : r.scope;
      if (rawScope === 'milestone' || rawScope === 'packet') {
        scope = rawScope;
        scopeId = r['scope-id'] ?? '';
        continueExecution = startAllowed;
      } else {
        scope = 'design';
        scopeId = '';
        continueExecution = false;
      }
    }
  }

  const curMs =
    (state as { 'current-milestone'?: string | null } | null)?.['current-milestone'] ??
    state?.currentMilestone ??
    '';
  if (!failedRequestId && startAllowed && typeof curMs === 'string' && curMs.trim()) {
    scope = 'milestone';
    scopeId = curMs.trim();
    continueExecution = true;
  }

  return { scope, scopeId, continueExecution, failedRequestId, cancellableRequestId };
}

export function formatRetryPlanningLabel(target: ResolvedRetryTarget, isZh: boolean): string {
  if (target.scope === 'milestone' && target.scopeId) {
    return isZh ? `重试 ${target.scopeId} 拆包规划` : `Retry ${target.scopeId} Plan`;
  }
  if (target.scope === 'packet' && target.scopeId) {
    return isZh ? `重试 ${target.scopeId} 契约修订` : `Retry ${target.scopeId} Revision`;
  }
  return isZh ? '重新尝试架构设计' : 'Retry Architecture Design';
}

export function formatLocalizedStepStatus(
  status?: string,
  running?: boolean,
  isZh = true,
  isRevising = false,
): {
  label: string;
  tone: SemanticTone;
} {
  if (running) return { label: isZh ? '进行中' : 'Running', tone: 'running' };
  if (isRevising) return { label: isZh ? '修订中' : 'Revising', tone: 'running' };
  const s = (status ?? '').toLowerCase();
  if (['running', 'executing', 'reviewing', 'verifying'].includes(s)) {
    // When liveness is explicitly false (task paused/interrupted mid-step), never claim active Running.
    if (running === false) {
      return { label: isZh ? '已暂停' : 'Paused', tone: 'warn' };
    }
    return { label: isZh ? '进行中' : 'Running', tone: 'running' };
  }
  if (['completed', 'done', 'passed', 'ok', 'closed'].includes(s)) {
    return { label: isZh ? '已完成' : 'Completed', tone: 'ok' };
  }
  if (s === 'l1-passed') {
    return { label: isZh ? '测试已通过' : 'Tests Passed', tone: 'ok' };
  }
  if (s === 'l1-failed') {
    return { label: isZh ? '测试未通过' : 'Tests Failed', tone: 'warn' };
  }
  if (['review-failed', 'returned'].includes(s)) {
    return { label: isZh ? '待返修' : 'Needs Fix', tone: 'warn' };
  }
  if (['failed', 'error', 'blocked', 'budget-stopped'].includes(s)) {
    return { label: isZh ? '已阻塞' : 'Blocked', tone: 'danger' };
  }
  if (['cancelled', 'stopped'].includes(s)) {
    return { label: isZh ? '已停止' : 'Stopped', tone: 'warn' };
  }
  if (['inbox', 'escalated', 'awaiting-input'].includes(s)) {
    return { label: isZh ? '待人工处理' : 'Action Needed', tone: 'warn' };
  }
  return { label: isZh ? '待开始' : 'Pending', tone: 'neutral' };
}

function detectReviewVerdict(tr: TraceSummary): 'pass' | 'revise' | 'blocked' | 'needs-user-decision' | null {
  const text = `${tr.preview ?? ''} ${tr.errorSummary ?? ''}`;
  if (/\bverdict["':\s]+pass\b|审查结论[:：\s]*pass\b|通过\s*\(pass\)/i.test(text)) return 'pass';
  if (/\bverdict["':\s]+revise\b|审查结论[:：\s]*revise\b|需修订/i.test(text)) return 'revise';
  if (/\bverdict["':\s]+needs-user-decision\b|审查结论[:：\s]*needs-user-decision\b/i.test(text)) {
    return 'needs-user-decision';
  }
  if (/\bverdict["':\s]+blocked\b|审查结论[:：\s]*blocked\b/i.test(text)) return 'blocked';
  return null;
}

/**
 * Annotates a chronological sequence of `plan` / `plan-review` traces within a
 * single planning scope with explicit 1-based rounds, schema/validation retry
 * indices, and localized verdict badges (never bare English `completed`).
 */
export function annotatePlanSteps(items: TraceSummary[], isZh: boolean): AnnotatedPlanStep[] {
  const ordered = [...items].sort((a, b) => {
    const ta = Date.parse(a.startedAt || a.updatedAt || '') || 0;
    const tb = Date.parse(b.startedAt || b.updatedAt || '') || 0;
    return ta - tb;
  });

  const steps: AnnotatedPlanStep[] = [];
  let round = 1;
  let planInRound = 0;

  for (let i = 0; i < ordered.length; i += 1) {
    const tr = ordered[i]!;
    const isReview = tr.role === 'plan-review' || tr.role === 'design-review';
    const baseStatus = formatLocalizedStepStatus(tr.status, tr.running, isZh);

    if (!isReview) {
      planInRound += 1;
      const retryIndex = Math.max(0, planInRound - 1);
      // If another `plan` immediately follows in this same round without an intervening review,
      // this attempt was superseded by a validation/format retry.
      const next = ordered[i + 1];
      const nextIsSameRoundPlan = Boolean(next && next.role === tr.role && !tr.running);
      const roleBase = isZh ? '实施计划拆解' : 'Implementation Plan';
      const title = retryIndex > 0
        ? (isZh ? `第 ${round} 轮 · ${roleBase}（校验重试 #${retryIndex}）` : `R${round} · ${roleBase} (Retry #${retryIndex})`)
        : (isZh ? `第 ${round} 轮 · ${roleBase}` : `R${round} · ${roleBase}`);
      const statusLabel = tr.running
        ? (isZh ? '进行中' : 'Running')
        : nextIsSameRoundPlan && baseStatus.tone === 'ok'
          ? (isZh ? '校验重试' : 'Validation Retry')
          : baseStatus.label;
      const tone: SemanticTone = tr.running
        ? 'running'
        : nextIsSameRoundPlan && baseStatus.tone === 'ok'
          ? 'warn'
          : baseStatus.tone;

      steps.push({
        trace: tr,
        round,
        retryIndex,
        isSupersededRetry: nextIsSameRoundPlan,
        title,
        verdict: null,
        statusLabel,
        tone,
      });
    } else {
      const verdict = tr.running ? null : detectReviewVerdict(tr);
      const roleBase = isZh ? '实施计划审查' : 'Plan Review';
      const title = isZh ? `第 ${round} 轮 · ${roleBase}` : `R${round} · ${roleBase}`;
      let statusLabel = tr.running ? (isZh ? '进行中' : 'Running') : baseStatus.label;
      let tone: SemanticTone = tr.running ? 'running' : baseStatus.tone;
      if (!tr.running && baseStatus.tone === 'ok') {
        if (verdict === 'pass') {
          statusLabel = isZh ? '审查通过' : 'Approved';
          tone = 'ok';
        } else if (verdict === 'revise') {
          statusLabel = isZh ? '需修订' : 'Revise';
          tone = 'warn';
        } else if (verdict === 'needs-user-decision') {
          statusLabel = isZh ? '待拍板' : 'Needs Decision';
          tone = 'warn';
        } else if (verdict === 'blocked') {
          statusLabel = isZh ? '审查阻断' : 'Blocked';
          tone = 'danger';
        }
      }
      steps.push({
        trace: tr,
        round,
        retryIndex: 0,
        isSupersededRetry: false,
        title,
        verdict,
        statusLabel,
        tone,
      });
      round += 1;
      planInRound = 0;
    }
  }
  return steps;
}

export function resolvePlanGroupTone(
  steps: AnnotatedPlanStep[],
  reqStatus?: string,
  running?: boolean,
  isZh = true,
): { tone: SemanticTone; statusLabel: string } {
  if (running) return { tone: 'running', statusLabel: isZh ? '进行中' : 'Running' };
  const rs = (reqStatus ?? '').toLowerCase();
  if (rs === 'budget-stopped') return { tone: 'danger', statusLabel: isZh ? '已达轮次上限' : 'Round Limit' };
  if (rs === 'blocked' || rs === 'failed') return { tone: 'danger', statusLabel: isZh ? '规划阻塞' : 'Plan Blocked' };
  if (rs === 'cancelled') return { tone: 'warn', statusLabel: isZh ? '已取消' : 'Cancelled' };
  if (rs === 'completed') return { tone: 'ok', statusLabel: isZh ? '已完成' : 'Completed' };
  const last = steps[steps.length - 1];
  if (last?.tone === 'danger') return { tone: 'danger', statusLabel: isZh ? '失败' : 'Failed' };
  if (last?.verdict === 'revise' || last?.verdict === 'needs-user-decision') {
    return { tone: 'warn', statusLabel: isZh ? '待修订' : 'Needs Revision' };
  }
  return { tone: 'ok', statusLabel: isZh ? '已完成' : 'Completed' };
}

function summarizeFeedbackShort(feedback: string | undefined, isZh: boolean): {
  packetId: string | null;
  summary: string | null;
} {
  const text = (feedback || '').trim();
  if (!text) return { packetId: null, summary: null };
  const pktMatch = /Returned packet\s+(M\d+-T\d+)/i.exec(text) ?? /\b(M\d+-T\d+)\b/i.exec(text);
  const packetId = pktMatch?.[1]?.toUpperCase() ?? null;
  if (text.includes('verification-gap') || text.includes('strengthen')) {
    return {
      packetId,
      summary: isZh
        ? `${packetId ? `${packetId} ` : ''}审查发现验收缺口（verification-gap），自动退回规划层补强 L1 验收契约`
        : `${packetId ? `${packetId} ` : ''}verification-gap detected — strengthening acceptance contract`,
    };
  }
  if (text.includes('L1_FAILED')) {
    return {
      packetId,
      summary: isZh
        ? `${packetId ? `${packetId} ` : ''}L1 验收连续未通过，自动退回规划层修订契约`
        : `${packetId ? `${packetId} ` : ''}L1 verification failed — returned to planner for contract revision`,
    };
  }
  if (text.includes('Returned packet ')) {
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
  const firstLine = text.split('\n')[0]?.trim() ?? '';
  const summary = firstLine.length > 160 ? `${firstLine.slice(0, 159)}…` : firstLine;
  return {
    packetId,
    summary: summary || (packetId ? (isZh ? `由 ${packetId} 触发修订` : `Triggered by ${packetId}`) : null),
  };
}

/**
 * Segments `plan` / `plan-review` traces chronologically so shared milestone
 * scopes (`planning:milestone/M-02`) split into:
 *  - Episode 0 (before any packet of M-02 executes): initial milestone plan
 *  - Episode 1..N (after packet execution): packet-linked local revisions
 * Also matches milestone planning requests (`scope: 'milestone', scope-id: 'M-XX'`)
 * accurately even when `req.id` is `engine-1-20` rather than `milestone/M-XX`.
 */
export function buildPlanningScopeGroups(params: {
  traces: TraceSummary[];
  allTraces: TraceSummary[];
  requests: PlanningRequestRecord[];
  isZh: boolean;
}): PlanScopeGroup[] {
  const { traces, allTraces, requests, isZh } = params;
  const sorted = [...traces].sort((a, b) => {
    const ta = Date.parse(a.startedAt || a.updatedAt || '') || 0;
    const tb = Date.parse(b.startedAt || b.updatedAt || '') || 0;
    return ta - tb;
  });

  // Collect packet execution/review events per milestone in chronological order.
  const execEventsByMs = new Map<string, Array<{ ts: number; packetId: string }>>();
  for (const tr of allTraces) {
    if (!['execute', 'review', 'frontend-ux-review'].includes(tr.role) || !tr.scope) continue;
    const pktId = tr.scope.replace(/^final:/, '').toUpperCase();
    const mid = extractMilestoneIdFromPacket(pktId);
    if (!mid) continue;
    const ts = Date.parse(tr.startedAt || tr.updatedAt || '') || 0;
    if (ts <= 0) continue;
    const list = execEventsByMs.get(mid) ?? [];
    list.push({ ts, packetId: pktId });
    execEventsByMs.set(mid, list);
  }
  for (const list of execEventsByMs.values()) {
    list.sort((a, b) => a.ts - b.ts);
  }

  // Group raw traces by scope first while preserving chronological order.
  const byScope = new Map<string, TraceSummary[]>();
  for (const tr of sorted) {
    const scopeKey = tr.scope || tr.key || tr.id;
    const list = byScope.get(scopeKey) ?? [];
    list.push(tr);
    byScope.set(scopeKey, list);
  }

  const groups: PlanScopeGroup[] = [];

  for (const [scopeKey, scopeTraces] of byScope.entries()) {
    const rawReqId = scopeKey.replace(/^planning:/, '');
    const directReq = requests.find(r => r.id === rawReqId || `planning:${r.id}` === scopeKey) ?? null;
    const directMilestoneMatch = /^milestone\/(M-\d{2,})$/i.exec(rawReqId);
    const msIdFromScope = directMilestoneMatch
      ? directMilestoneMatch[1]!.toUpperCase()
      : extractMilestoneIdFromScope(scopeKey, null, directReq);

    // If this is a shared `planning:milestone/M-XX` scope, split into chronological episodes
    // whenever packet executions occurred between planning traces.
    if (directMilestoneMatch && msIdFromScope) {
      const msExecs = execEventsByMs.get(msIdFromScope) ?? [];
      const firstExecTs = msExecs[0]?.ts ?? Number.POSITIVE_INFINITY;
      const msRequests = requests.filter(
        r => r.scope === 'milestone' && (r['scope-id'] ?? '').toUpperCase() === msIdFromScope,
      );

      interface Episode {
        isInitial: boolean;
        triggerPacketId: string | null;
        items: TraceSummary[];
      }
      const episodes: Episode[] = [];
      let current: Episode | null = null;
      let lastTraceTs = 0;

      for (const tr of scopeTraces) {
        const ts = Date.parse(tr.startedAt || tr.updatedAt || '') || 0;
        const isInitial = ts <= 0 || ts < firstExecTs;
        if (isInitial) {
          if (!current || !current.isInitial) {
            current = { isInitial: true, triggerPacketId: null, items: [] };
            episodes.push(current);
          }
          current.items.push(tr);
        } else {
          // Find any packet execution between lastTraceTs and ts.
          const intervening = msExecs.filter(ev => ev.ts > lastTraceTs && ev.ts <= ts);
          const triggerPkt =
            intervening[intervening.length - 1]?.packetId ??
            msExecs.filter(ev => ev.ts <= ts).pop()?.packetId ??
            null;
          if (!current || current.isInitial || intervening.length > 0) {
            current = { isInitial: false, triggerPacketId: triggerPkt, items: [] };
            episodes.push(current);
          }
          current.items.push(tr);
        }
        if (ts > 0) lastTraceTs = ts;
      }

      // Match each episode to its corresponding milestone PlanningRequestRecord.
      const nonPacketReqs = msRequests.filter(r => !summarizeFeedbackShort(r.feedback, isZh).packetId);
      const usedReqIds = new Set<string>();

      episodes.forEach((ep, epIdx) => {
        let matchedReq: PlanningRequestRecord | null = null;
        if (ep.isInitial) {
          // Prefer the latest active/blocked/completed non-packet milestone request (ignoring cancelled unless all cancelled).
          const nonCancelled = nonPacketReqs.filter(r => (r.status ?? '').toLowerCase() !== 'cancelled');
          const pool = nonCancelled.length > 0 ? nonCancelled : nonPacketReqs;
          matchedReq = pool[pool.length - 1] ?? msRequests[msRequests.length - 1] ?? null;
        } else {
          // Match chronologically to the next unused milestone request triggered by this packet.
          const pktReqs = msRequests.filter(r => {
            if (usedReqIds.has(r.id)) return false;
            const fbPkt = summarizeFeedbackShort(r.feedback, isZh).packetId;
            return ep.triggerPacketId ? fbPkt === ep.triggerPacketId : Boolean(fbPkt);
          });
          matchedReq = pktReqs[0] ?? null;
          if (matchedReq) usedReqIds.add(matchedReq.id);
        }

        const running = ep.items.some(t => t.running);
        const totalDurationMs = ep.items.reduce(
          (sum, t) => sum + (typeof t.durationMs === 'number' && Number.isFinite(t.durationMs) ? t.durationMs : 0),
          0,
        );
        const steps = annotatePlanSteps(ep.items, isZh);
        const reqStatus = matchedReq?.status;
        const resolved = resolvePlanGroupTone(steps, reqStatus, running, isZh);
        const fb = summarizeFeedbackShort(matchedReq?.feedback, isZh);
        const reqId = matchedReq?.id ?? (ep.isInitial ? rawReqId : `${rawReqId}#rev-${epIdx}`);
        const label = ep.isInitial
          ? (isZh ? `里程碑拆包规划 · ${msIdFromScope}` : `Milestone Plan · ${msIdFromScope}`)
          : ep.triggerPacketId
            ? (isZh ? `局部修订 · ${ep.triggerPacketId}` : `Local Revision · ${ep.triggerPacketId}`)
            : (isZh ? `计划修订 · ${reqId}` : `Plan Revision · ${reqId}`);

        groups.push({
          scope: ep.isInitial ? scopeKey : `${scopeKey}#rev-${epIdx}`,
          requestId: reqId,
          milestoneId: msIdFromScope,
          isMilestonePlan: ep.isInitial,
          label,
          triggerPacketId: ep.isInitial ? null : (ep.triggerPacketId ?? fb.packetId),
          triggerSummary: ep.isInitial ? null : fb.summary,
          items: ep.items,
          steps,
          running,
          tone: resolved.tone,
          statusLabel: resolved.statusLabel,
          totalDurationMs,
        });
      });
      continue;
    }

    // Standard single-request scope (e.g. `planning:engine-1-6`, `planning:strengthen-m01t01-l1`).
    const fb = summarizeFeedbackShort(directReq?.feedback, isZh);
    const scopePkt = fb.packetId || (/\b(M\d+-T\d+)\b/i.exec(rawReqId)?.[1]?.toUpperCase() ?? null);
    const milestoneId = extractMilestoneIdFromScope(scopeKey, scopePkt, directReq);
    const isMilestonePlan =
      rawReqId.startsWith('milestone/') ||
      (directReq?.scope === 'milestone' && Boolean(directReq?.['scope-id']) && !scopePkt);
    const running = scopeTraces.some(t => t.running);
    const totalDurationMs = scopeTraces.reduce(
      (sum, t) => sum + (typeof t.durationMs === 'number' && Number.isFinite(t.durationMs) ? t.durationMs : 0),
      0,
    );
    const steps = annotatePlanSteps(scopeTraces, isZh);
    const resolved = resolvePlanGroupTone(steps, directReq?.status, running, isZh);
    const label = isMilestonePlan && milestoneId
      ? (isZh ? `里程碑拆包规划 · ${milestoneId}` : `Milestone Plan · ${milestoneId}`)
      : (isZh ? `计划修订 · ${rawReqId}` : `Plan Revision · ${rawReqId}`);

    groups.push({
      scope: scopeKey,
      requestId: rawReqId,
      milestoneId,
      isMilestonePlan,
      label,
      triggerPacketId: scopePkt,
      triggerSummary: fb.summary,
      items: scopeTraces,
      steps,
      running,
      tone: resolved.tone,
      statusLabel: resolved.statusLabel,
      totalDurationMs,
    });
  }

  return groups;
}

/**
 * Builds the canonical milestone-nested timeline shared across the Main Stream
 * and TopBar dropdown so completed milestones collapse cleanly and the active
 * milestone always sits at the bottom frontier.
 */
export function buildMilestoneTimeline(params: {
  milestonesMap?: Record<string, { status?: string; gate?: unknown; 'human-signoff'?: unknown }> | null;
  designMilestones?: Array<{ id: string; title: string; gate?: string }> | null;
  currentMilestone?: string | null;
  packets: PacketNode[];
  planGroups: PlanScopeGroup[];
  traces: TraceSummary[];
  requests?: PlanningRequestRecord[] | null;
  taskStatus?: string | null;
  taskRunning?: boolean;
  planningPhase?: string | null;
  isZh?: boolean;
}): {
  milestones: MilestoneTimelineNode[];
  unassignedPackets: PacketNode[];
  unassignedRevisions: PlanScopeGroup[];
} {
  const {
    milestonesMap,
    designMilestones,
    currentMilestone,
    packets,
    planGroups,
    traces,
    requests,
    taskStatus,
    taskRunning = false,
    planningPhase,
    isZh = true,
  } = params;

  const idSet = new Set<string>();
  const titleById = new Map<string, string>();
  for (const dm of designMilestones ?? []) {
    if (dm?.id) {
      idSet.add(dm.id.toUpperCase());
      titleById.set(dm.id.toUpperCase(), dm.title || dm.id);
    }
  }
  for (const id of Object.keys(milestonesMap ?? {})) {
    idSet.add(id.toUpperCase());
  }
  for (const pkt of packets) {
    const mid = extractMilestoneIdFromPacket(pkt.id);
    if (mid) idSet.add(mid);
  }
  for (const grp of planGroups) {
    if (grp.milestoneId) idSet.add(grp.milestoneId);
  }

  // Only activate nested milestone layout when at least one milestone is declared or inferred.
  const orderedIds = [...idSet].sort(compareMilestoneIds);
  if (orderedIds.length === 0) {
    return { milestones: [], unassignedPackets: packets, unassignedRevisions: planGroups };
  }

  const packetsByMs = new Map<string, PacketNode[]>();
  const unassignedPackets: PacketNode[] = [];
  for (const pkt of packets) {
    const mid = extractMilestoneIdFromPacket(pkt.id);
    if (mid && idSet.has(mid)) {
      const list = packetsByMs.get(mid) ?? [];
      list.push(pkt);
      packetsByMs.set(mid, list);
    } else {
      unassignedPackets.push(pkt);
    }
  }

  const milestoneStarts = computeMilestoneStartTimestamps(orderedIds, traces, requests ?? []);
  const planByMs = new Map<string, PlanScopeGroup[]>();
  const revByMs = new Map<string, PlanScopeGroup[]>();
  const unassignedRevisions: PlanScopeGroup[] = [];
  for (const grp of planGroups) {
    let targetMs = grp.milestoneId && idSet.has(grp.milestoneId) ? grp.milestoneId : null;
    if (!targetMs) {
      const firstTs = Date.parse(grp.items[0]?.startedAt || grp.items[0]?.updatedAt || '') || 0;
      targetMs = inferMilestoneByTimestamp(firstTs, orderedIds, milestoneStarts);
    }
    if (targetMs && idSet.has(targetMs)) {
      if (grp.isMilestonePlan) {
        const list = planByMs.get(targetMs) ?? [];
        list.push(grp);
        planByMs.set(targetMs, list);
      } else {
        const list = revByMs.get(targetMs) ?? [];
        list.push(grp);
        revByMs.set(targetMs, list);
      }
    } else {
      unassignedRevisions.push(grp);
    }
  }

  const packetIdSet = new Set(packets.map(p => p.id));
  const verifyByMs = new Map<string, TraceSummary[]>();
  for (const tr of traces) {
    if (tr.role !== 'judge' && !(tr.role === 'frontend-ux-review' && (!tr.scope || !packetIdSet.has(tr.scope)))) {
      continue;
    }
    const mid = extractMilestoneIdFromScope(tr.scope);
    if (mid && idSet.has(mid)) {
      const list = verifyByMs.get(mid) ?? [];
      list.push(tr);
      verifyByMs.set(mid, list);
    }
  }
  for (const list of verifyByMs.values()) {
    list.sort((a, b) => (Date.parse(a.startedAt || a.updatedAt || '') || 0) - (Date.parse(b.startedAt || b.updatedAt || '') || 0));
  }

  const curMsUpper = (currentMilestone ?? '').toUpperCase();
  const normalizedTaskStatus = (taskStatus ?? '').toLowerCase();
  const normalizedPlanPhase = (planningPhase ?? '').toLowerCase();

  const milestones: MilestoneTimelineNode[] = orderedIds.map(id => {
    const rawState = milestonesMap?.[id] ?? milestonesMap?.[id.toLowerCase()];
    const rawStatus = (rawState?.status ?? '').toLowerCase();
    const msPackets = packetsByMs.get(id) ?? [];
    const msPlanning = planByMs.get(id) ?? [];
    const msRevisions = revByMs.get(id) ?? [];
    const msVerify = verifyByMs.get(id) ?? [];

    const completedPackets = msPackets.filter(p =>
      ['done', 'completed', 'passed'].includes((p.status ?? '').toLowerCase()),
    ).length;

    const hasRunningTrace =
      msPlanning.some(g => g.running) ||
      msRevisions.some(g => g.running) ||
      msVerify.some(t => t.running) ||
      msPackets.some(p => isPacketEffectivelyRunning(p, traces, taskRunning));

    const isClosed = rawStatus === 'closed';
    const isCurrent = curMsUpper === id;
    const msReqs = (requests ?? []).filter(
      r => r.scope === 'milestone' && (r['scope-id'] ?? '').toUpperCase() === id,
    );
    const latestNonCancelledReq = [...msReqs].reverse().find(r => (r.status ?? '').toLowerCase() !== 'cancelled') ?? null;
    const latestReqStatus = (latestNonCancelledReq?.status ?? '').toLowerCase();

    const isBudgetStopped =
      !isClosed &&
      !hasRunningTrace &&
      ((isCurrent && (normalizedTaskStatus === 'budget-stopped' || normalizedPlanPhase === 'budget-stopped')) ||
        latestReqStatus === 'budget-stopped' ||
        msPlanning.some(g => !g.running && g.statusLabel === (isZh ? '已达轮次上限' : 'Round Limit')));
    const hasBlockedPlanning =
      !isClosed &&
      !hasRunningTrace &&
      ((isCurrent && (normalizedPlanPhase === 'blocked' || normalizedPlanPhase === 'failed')) ||
        latestReqStatus === 'blocked' ||
        latestReqStatus === 'failed' ||
        msPlanning.some(g => !g.running && g.tone === 'danger') ||
        msRevisions.some(g => !g.running && g.tone === 'danger'));
    const rawGate = rawState?.gate as { pass?: boolean; judge?: { verdict?: string; error?: string } } | undefined;
    const hasFailedGate = Boolean(
      rawGate &&
        (rawGate.pass === false ||
          (typeof rawGate.judge?.verdict === 'string' && rawGate.judge.verdict !== '' && rawGate.judge.verdict !== 'pass') ||
          Boolean(rawGate.judge?.error)),
    );
    const isEscalated =
      !isClosed &&
      !hasRunningTrace &&
      (rawStatus === 'escalated' ||
        hasFailedGate ||
        (isCurrent && normalizedTaskStatus === 'escalated') ||
        msPackets.some(p => ['inbox', 'blocked', 'failed', 'escalated'].includes((p.status ?? '').toLowerCase())));

    const hasStarted =
      rawStatus === 'in-progress' ||
      isCurrent ||
      hasRunningTrace ||
      msPackets.length > 0 ||
      msPlanning.length > 0 ||
      msRevisions.length > 0 ||
      msVerify.length > 0;

    // Strict parent-child status invariant:
    // A milestone can show "进行中" / tone="running" ONLY when actively running (child trace running or engine transitioning).
    // Never render a gray "进行中" badge with zero running children!
    let status: MilestoneTimelineNode['status'] = 'pending';
    let tone: SemanticTone = 'neutral';
    let statusLabel = isZh ? '待开始' : 'Pending';
    let isRunning = false;

    if (isClosed) {
      status = 'closed';
      tone = 'ok';
      statusLabel = isZh ? '已完成' : 'Completed';
    } else if (hasRunningTrace) {
      status = 'running';
      tone = 'running';
      statusLabel = isZh ? '进行中' : 'Running';
      isRunning = true;
    } else if (isBudgetStopped) {
      status = 'budget-stopped';
      tone = 'danger';
      statusLabel = isZh ? '已达轮次上限' : 'Round Limit';
    } else if (hasBlockedPlanning) {
      status = 'blocked';
      tone = 'danger';
      statusLabel = isZh ? '规划阻塞' : 'Plan Blocked';
    } else if (isEscalated) {
      status = 'escalated';
      tone = 'warn';
      statusLabel = isZh ? '待人工处理' : 'Needs Action';
    } else if (hasStarted) {
      if (taskRunning && isCurrent) {
        status = 'running';
        tone = 'running';
        statusLabel = isZh ? '进行中' : 'Running';
        isRunning = true;
      } else {
        status = 'paused';
        tone = 'warn';
        statusLabel = isZh ? '已暂停' : 'Paused';
      }
    }

    let planningBlockedDetail: string | null = null;
    if (hasBlockedPlanning || isBudgetStopped) {
      const latestPlanTrace = [...msPlanning, ...msRevisions]
        .flatMap(g => g.items)
        .sort((a, b) => (Date.parse(a.startedAt || a.updatedAt || '') || 0) - (Date.parse(b.startedAt || b.updatedAt || '') || 0))
        .pop();
      planningBlockedDetail =
        latestPlanTrace?.errorSummary ||
        latestPlanTrace?.preview ||
        latestNonCancelledReq?.feedback ||
        null;
    }

    let totalDurationMs = 0;
    for (const g of [...msPlanning, ...msRevisions]) totalDurationMs += g.totalDurationMs;
    for (const t of msVerify) {
      if (typeof t.durationMs === 'number' && Number.isFinite(t.durationMs)) totalDurationMs += t.durationMs;
    }
    for (const p of msPackets) {
      for (const t of traces) {
        if ((t.scope === p.id || t.scope === `final:${p.id}` || t.key.endsWith(`/${p.id}`)) && typeof t.durationMs === 'number' && Number.isFinite(t.durationMs)) {
          totalDurationMs += t.durationMs;
        }
      }
    }

    return {
      id,
      title: titleById.get(id) || id,
      status,
      tone,
      statusLabel,
      isClosed,
      isRunning,
      isAttention: status === 'escalated' || status === 'budget-stopped' || status === 'blocked',
      isPending: status === 'pending',
      hasBlockedPlanning: hasBlockedPlanning || isBudgetStopped,
      planningBlockedDetail,
      planningGroups: msPlanning,
      localRevisions: msRevisions,
      packets: msPackets,
      verificationTraces: msVerify,
      completedPackets,
      totalPackets: msPackets.length,
      totalDurationMs,
    };
  });

  return { milestones, unassignedPackets, unassignedRevisions };
}
