import type {
  DesignCandidateRevision,
  PacketNode,
  PlanningDecision,
  PlanningFinding,
  PlanningMilestone,
  PlanningRequestRecord,
  PlanningRequirement,
  PlanningState,
  TaskState,
  TraceSummary,
} from './types.ts';

export function buildTraceRoundLabels(traces: TraceSummary[], isZh: boolean): Map<string, string> {
  const chronological = [...traces].sort((a, b) => {
    const ta = Date.parse(a.startedAt || a.updatedAt || '') || 0;
    const tb = Date.parse(b.startedAt || b.updatedAt || '') || 0;
    return ta - tb;
  });

  // First pass: assign round indices for Stage 1 (design), Stage 1.5 (prototype), and Stage 2 (per-scope plan).
  let designRound = 0;
  let seenDesignReview = false;
  let inDesign = false;

  let protoRound = 0;
  let seenProtoReview = false;

  const planStateByScope = new Map<string, { round: number; seenReview: boolean }>();
  const traceRoundNum = new Map<string, number>();

  for (const tr of chronological) {
    if (tr.role === 'design-plan' || tr.role === 'design-review') {
      const startsNew =
        !inDesign ||
        (tr.role === 'design-plan' &&
          !tr.scope?.endsWith(':secondary') &&
          !tr.scope?.endsWith(':synthesize') &&
          !tr.sessionTitle?.includes(':synthesize') &&
          seenDesignReview);
      if (startsNew) {
        designRound += 1;
        seenDesignReview = false;
      }
      inDesign = true;
      traceRoundNum.set(tr.id, Math.max(1, designRound));
      if (tr.role === 'design-review') seenDesignReview = true;
      continue;
    }
    inDesign = false;

    if (tr.role === 'ui-prototype' || tr.role === 'ui-prototype-review') {
      if (protoRound === 0 || (tr.role === 'ui-prototype' && seenProtoReview)) {
        protoRound += 1;
        seenProtoReview = false;
      }
      traceRoundNum.set(tr.id, Math.max(1, protoRound));
      if (tr.role === 'ui-prototype-review') seenProtoReview = true;
      continue;
    }

    if (tr.role === 'plan' || tr.role === 'plan-review') {
      const scopeKey = tr.scope || tr.key || tr.id;
      const st = planStateByScope.get(scopeKey) ?? { round: 0, seenReview: false };
      if (st.round === 0 || (tr.role === 'plan' && st.seenReview)) {
        st.round += 1;
        st.seenReview = false;
      }
      planStateByScope.set(scopeKey, st);
      traceRoundNum.set(tr.id, Math.max(1, st.round));
      if (tr.role === 'plan-review') st.seenReview = true;
    }
  }

  const map = new Map<string, string>();
  for (const tr of chronological) {
    const round = traceRoundNum.get(tr.id) ?? 1;
    if (tr.role === 'clarify') {
      map.set(tr.id, isZh ? 'Stage 0 · 需求澄清' : 'Stage 0 · Clarify');
    } else if (tr.role === 'ui-prototype') {
      map.set(
        tr.id,
        protoRound > 1
          ? (isZh ? `Stage 1.5 第 ${round} 轮 · UI/UX 交互原型` : `Stage 1.5 R${round} · UI Prototype`)
          : (isZh ? 'Stage 1.5 · UI/UX 交互原型' : 'Stage 1.5 · UI Prototype'),
      );
    } else if (tr.role === 'ui-prototype-review') {
      map.set(
        tr.id,
        protoRound > 1
          ? (isZh ? `Stage 1.5 第 ${round} 轮 · 原型审查` : `Stage 1.5 R${round} · Prototype Review`)
          : (isZh ? 'Stage 1.5 · 原型审查' : 'Stage 1.5 · Prototype Review'),
      );
    } else if (tr.role === 'design-plan') {
      if (tr.scope?.endsWith(':secondary')) {
        map.set(tr.id, isZh ? `第 ${round} 轮 · 架构设计对照稿` : `R${round} · Secondary Draft`);
      } else if (tr.scope?.endsWith(':synthesize') || tr.sessionTitle?.includes(':synthesize')) {
        map.set(tr.id, isZh ? `第 ${round} 轮 · 双稿统稿合并` : `R${round} · Synthesis`);
      } else {
        map.set(
          tr.id,
          round > 1
            ? (isZh ? `第 ${round} 轮 · 按审修订设计稿` : `R${round} · Design Revision`)
            : (isZh ? `第 ${round} 轮 · 架构设计主稿` : `R${round} · Primary Draft`),
        );
      }
    } else if (tr.role === 'design-review') {
      map.set(tr.id, isZh ? `第 ${round} 轮 · 设计架构审查` : `R${round} · Design Review`);
    } else if (tr.role === 'plan' || tr.role === 'plan-review') {
      const scopeKey = tr.scope || tr.key || tr.id;
      const totalRounds = planStateByScope.get(scopeKey)?.round ?? 1;
      const tag = tr.scope?.replace(/^planning:/, '') || '';
      const base = tr.role === 'plan' ? (isZh ? '拆包规划' : 'Plan') : (isZh ? '规划审查' : 'Plan Review');
      const roundTag = totalRounds > 1 ? (isZh ? ` 第 ${round} 轮` : ` R${round}`) : '';
      map.set(tr.id, `${base}${roundTag}${tag ? ` · ${tag}` : ''}`);
    } else if (tr.role === 'judge') {
      map.set(tr.id, isZh ? `里程碑裁决${tr.scope ? ` · ${tr.scope}` : ''}` : `Milestone Judge${tr.scope ? ` · ${tr.scope}` : ''}`);
    } else if (tr.role === 'frontend-ux-review') {
      map.set(tr.id, isZh ? `前端 UX 审查${tr.scope ? ` · ${tr.scope}` : ''}` : `Frontend UX Review${tr.scope ? ` · ${tr.scope}` : ''}`);
    } else if (tr.role === 'investigate') {
      map.set(tr.id, isZh ? '只读根因调查' : 'Investigation');
    } else {
      map.set(tr.id, `${tr.role}${tr.scope ? ` · ${tr.scope}` : ''}`);
    }
  }
  return map;
}

const EFFORT_LABELS_ZH: Record<string, string> = {
  xhigh: '超高',
  high: '高',
  medium: '中',
  low: '低',
  minimal: '极低',
  max: '最高',
  none: '关闭',
};

const EFFORT_LABELS_EN: Record<string, string> = {
  xhigh: 'Extra High',
  high: 'High',
  medium: 'Medium',
  low: 'Low',
  minimal: 'Minimal',
  max: 'Max',
  none: 'None',
};

function titleWord(word: string): string {
  if (!word) return '';
  const lower = word.toLowerCase();
  if (lower === 'gpt') return 'GPT';
  if (lower === 'pro') return 'Pro';
  if (lower === 'mini') return 'Mini';
  if (lower === 'codex') return 'Codex';
  if (lower === 'flash') return 'Flash';
  if (lower === 'thinking') return 'Thinking';
  if (/^[a-z]/.test(word)) return word[0]!.toUpperCase() + word.slice(1);
  return word;
}

export function formatEffortLabel(thinking?: string | null, isZh = true): string {
  const key = typeof thinking === 'string' ? thinking.trim().toLowerCase() : '';
  if (!key) return '';
  const table = isZh ? EFFORT_LABELS_ZH : EFFORT_LABELS_EN;
  return table[key] ?? thinking!.trim();
}

export function formatModelDisplayName(model?: string | null): { displayName: string; embeddedEffort: string } {
  const raw = typeof model === 'string' ? model.trim() : '';
  if (!raw) return { displayName: '', embeddedEffort: '' };

  // Strip provider channel prefix if present (e.g. "openai/gpt-6.2-sol" -> "gpt-6.2-sol").
  const slug = raw.includes('/') ? raw.slice(raw.lastIndexOf('/') + 1) : raw;
  const lower = slug.toLowerCase();

  if (lower === 'gemini-pro-orchestrator-neon') return { displayName: 'Orchestrator (Neon)', embeddedEffort: '' };
  if (lower === 'gemini-pro-orchestrator-argon') return { displayName: 'Orchestrator (Argon)', embeddedEffort: '' };
  if (lower === 'gemini-pro-orchestrator') return { displayName: 'Orchestrator', embeddedEffort: '' };

  let base = slug;
  let embeddedEffort = '';
  const effortSuffix = /^(.+?)-(xhigh|high|medium|low|minimal|max)$/i.exec(slug);
  if (effortSuffix) {
    base = effortSuffix[1]!;
    embeddedEffort = effortSuffix[2]!.toLowerCase();
  }

  const baseLower = base.toLowerCase();

  // Gemini 4 series: gemini-4-carbon / gemini-4-neon / gemini-4-hydrogen / gemini-4-argon
  const gem4 = /^gemini-(\d+(?:\.\d+)?)-(carbon|neon|hydrogen|argon|pro|flash)(?:-(.+))?$/i.exec(baseLower);
  if (gem4) {
    const tail = gem4[3] ? ` ${gem4[3].split('-').map(titleWord).join(' ')}` : '';
    return { displayName: `Gemini ${gem4[1]} ${titleWord(gem4[2]!)}${tail}`, embeddedEffort };
  }

  // GPT series: gpt-6.2-sol, gpt-5.4-codex, gpt-5.3, etc.
  const gpt = /^gpt-(\d+(?:\.\d+)?)(?:-(.+))?$/i.exec(baseLower);
  if (gpt) {
    const suffix = gpt[2] ? ` ${gpt[2].split('-').map(titleWord).join(' ')}` : '';
    return { displayName: `GPT-${gpt[1]}${suffix}`, embeddedEffort };
  }

  // Claude series: claude-opus-5-5, claude-sonnet-4-6, claude-4-5-sonnet, etc.
  const claudeNew = /^claude-(opus|sonnet|haiku)-(\d+)[.-](\d+)(?:-(.+))?$/i.exec(baseLower);
  if (claudeNew) {
    const extra = claudeNew[4] && !/^\d{8}$/.test(claudeNew[4]) ? ` ${claudeNew[4].split('-').map(titleWord).join(' ')}` : '';
    return { displayName: `Claude ${titleWord(claudeNew[1]!)} ${claudeNew[2]}.${claudeNew[3]}${extra}`, embeddedEffort };
  }
  const claudeOld = /^claude-(\d+)[.-](\d+)-(opus|sonnet|haiku)$/i.exec(baseLower);
  if (claudeOld) {
    return { displayName: `Claude ${titleWord(claudeOld[3]!)} ${claudeOld[1]}.${claudeOld[2]}`, embeddedEffort };
  }

  // Kimi series: kimi-k2.5, kimi-k2-thinking
  const kimi = /^kimi-k(\d+(?:\.\d+)?)(?:-(.+))?$/i.exec(baseLower);
  if (kimi) {
    const suffix = kimi[2] ? ` ${kimi[2].split('-').map(titleWord).join(' ')}` : '';
    return { displayName: `Kimi K${kimi[1]}${suffix}`, embeddedEffort };
  }

  // DeepSeek series
  const deepseek = /^deepseek-(.+)$/i.exec(baseLower);
  if (deepseek) {
    return { displayName: `DeepSeek ${deepseek[1]!.split('-').map(titleWord).join(' ')}`, embeddedEffort };
  }

  return { displayName: base, embeddedEffort };
}

export function formatModelBadge(model?: string | null, thinking?: string | null, isZh = true): string {
  const { displayName, embeddedEffort } = formatModelDisplayName(model);
  const rawThinking = typeof thinking === 'string' ? thinking.trim() : '';
  const effectiveEffort = rawThinking || embeddedEffort;
  const effortText = formatEffortLabel(effectiveEffort, isZh);
  if (!displayName) return effortText;
  if (!effortText) return displayName;
  return `${displayName} ${effortText}`;
}

export function normalizePacketNode(id: string, raw: PacketNode): PacketNode {
  const rec = raw as Record<string, unknown>;
  const schedule = rec.schedule && typeof rec.schedule === 'object' ? (rec.schedule as { after?: unknown; writes?: unknown }) : null;
  const execPolicy =
    rec['execution-policy'] && typeof rec['execution-policy'] === 'object'
      ? (rec['execution-policy'] as { current?: string; currentBinding?: { binding?: { model?: string; thinking?: string } } })
      : null;
  const deps =
    raw.dependsOn ??
    raw.depends_on ??
    (Array.isArray(schedule?.after) ? schedule.after.filter((v): v is string => typeof v === 'string') : undefined);
  const writeScopes =
    raw.writeScopes ??
    raw.write_scopes ??
    (Array.isArray(schedule?.writes) ? schedule.writes.filter((v): v is string => typeof v === 'string') : undefined);
  const model = raw.model || execPolicy?.currentBinding?.binding?.model || undefined;
  const thinking = (typeof rec.thinking === 'string' && rec.thinking) || execPolicy?.currentBinding?.binding?.thinking || undefined;
  const profile = raw.profile || execPolicy?.current || undefined;
  return {
    ...raw,
    id: typeof raw.id === 'string' && raw.id.trim() ? raw.id : id,
    ...(deps ? { dependsOn: deps } : {}),
    ...(writeScopes ? { writeScopes } : {}),
    ...(model ? { model } : {}),
    ...(thinking ? { thinking } : {}),
    ...(profile ? { profile } : {}),
  };
}

export function normalizePackets(record?: Record<string, PacketNode> | null): PacketNode[] {
  if (!record || typeof record !== 'object') return [];
  return Object.entries(record)
    .filter(([, p]) => Boolean(p && typeof p === 'object'))
    .map(([id, p]) => normalizePacketNode(id, p));
}

export interface ResolvedDesign {
  revision?: number;
  digest?: string;
  document: string;
  requirements: PlanningRequirement[];
  decisions: PlanningDecision[];
  milestones: PlanningMilestone[];
  implementationScope: string[];
  approved: boolean;
}

export interface ResolvedReview {
  id?: string;
  verdict?: string;
  summary?: string;
  binding?: string;
  role?: string;
  findings: PlanningFinding[];
}

export interface ResolvedPlanningView {
  phase: string;
  policy: string;
  mode: string;
  briefGoal: string;
  briefOriginalRequest: string;
  constraints: string[];
  reason: string;
  rounds: number;
  maxRounds: number;
  activeRequest: PlanningRequestRecord | null;
  design: ResolvedDesign | null;
  review: ResolvedReview | null;
  isApproved: boolean;
  needsHumanApproval: boolean;
  isBlocked: boolean;
  isPlanningActive: boolean;
  stepperStep: 1 | 2 | 3 | 4 | 5;
}

export type DisplayStatusTone = 'running' | 'success' | 'warn' | 'danger' | 'neutral';

export interface DisplayTaskStatus {
  code: string;
  tone: DisplayStatusTone;
  labelKey: string;
}

export function resolvePlanningView(planning?: PlanningState | null): ResolvedPlanningView | null {
  if (!planning) return null;

  const phase = (planning.phase ?? planning.status ?? '').toLowerCase();
  const policy = planning.policy ?? planning.brief?.policy ?? 'review-first';
  const mode = planning.mode ?? planning.brief?.mode ?? '';
  const briefGoal = planning.brief?.goal ?? planning.brief?.summary ?? planning.brief?.title ?? '';
  const briefOriginalRequest = planning.brief?.['original-request'] ?? briefGoal;
  const constraints = Array.isArray(planning.brief?.constraints) ? planning.brief.constraints : [];
  const reason = (planning.reason ?? planning.lastError ?? '').trim();
  const rounds = typeof planning.rounds === 'number' ? planning.rounds : 0;
  const maxRounds = typeof planning['max-rounds'] === 'number' ? planning['max-rounds'] : 3;

  const requests = Array.isArray(planning.requests) ? planning.requests : [];
  const activeRequest = requests.length > 0 ? requests[requests.length - 1]! : null;

  // Match authoritative Go `designs[]` + `current-design`, falling back to legacy mock `design`.
  const designs = Array.isArray(planning.designs) ? planning.designs : [];
  const currentRef = planning['current-design'] ?? null;
  const matchedGoDesign =
    (currentRef
      ? designs.find(d => d.ref?.revision === currentRef.revision && d.ref?.digest === currentRef.digest)
      : null) ?? (designs.length > 0 ? designs[designs.length - 1]! : null);

  const approvals = Array.isArray(planning.approvals) ? planning.approvals : [];
  const legacyDesign = planning.design;

  let design: ResolvedDesign | null = null;
  if (matchedGoDesign) {
    const rev = matchedGoDesign.ref?.revision;
    const digest = matchedGoDesign.ref?.digest;
    const approvedByRecord = Boolean(
      rev && digest && approvals.some(a => a.design?.revision === rev && a.design?.digest === digest),
    );
    const approvedByPhase = ['approved', 'activating', 'ready'].includes(phase);
    design = {
      revision: rev,
      digest,
      document: matchedGoDesign.document ?? '',
      requirements: Array.isArray(matchedGoDesign.requirements) ? matchedGoDesign.requirements : [],
      decisions: Array.isArray(matchedGoDesign.decisions) ? matchedGoDesign.decisions : [],
      milestones: Array.isArray(matchedGoDesign.milestones) ? matchedGoDesign.milestones : [],
      implementationScope: Array.isArray(matchedGoDesign['implementation-scope'])
        ? matchedGoDesign['implementation-scope']
        : [],
      approved: approvedByRecord || approvedByPhase,
    };
  } else if (legacyDesign) {
    const approvedByPhase = ['approved', 'activating', 'ready'].includes(phase);
    design = {
      revision: legacyDesign.revision,
      digest: legacyDesign.digest,
      document: legacyDesign.markdown || legacyDesign.content || legacyDesign.summary || '',
      requirements: [],
      decisions: [],
      milestones: [],
      implementationScope: [],
      approved: Boolean(legacyDesign.approved) || approvedByPhase,
    };
  }

  // Match authoritative Go `reviews[]` for the current design, falling back to legacy findings.
  const reviews = Array.isArray(planning.reviews) ? planning.reviews : [];
  let matchedReview = null as (typeof reviews)[number] | null;
  if (reviews.length > 0) {
    const designReviews = reviews.filter(r => r.kind === 'design' || r.kind === 'compact-v1');
    if (design?.revision && design?.digest) {
      for (let i = designReviews.length - 1; i >= 0; i -= 1) {
        const rev = designReviews[i]!;
        if (rev.target?.revision === design.revision && rev.target?.digest === design.digest) {
          matchedReview = rev;
          break;
        }
      }
    }
    if (!matchedReview && designReviews.length > 0) {
      matchedReview = designReviews[designReviews.length - 1]!;
    }
  }

  const legacyFindings = legacyDesign?.findings ?? planning.findings ?? [];
  let review: ResolvedReview | null = null;
  if (matchedReview) {
    review = {
      id: matchedReview.id,
      verdict: matchedReview.verdict || matchedReview['design-verdict'],
      summary: matchedReview.summary,
      binding: matchedReview.binding,
      role: matchedReview.role,
      findings: Array.isArray(matchedReview.findings) ? matchedReview.findings : [],
    };
  } else if (legacyFindings.length > 0) {
    review = {
      findings: legacyFindings,
    };
  }

  const isApproved = Boolean(design?.approved) || ['approved', 'activating', 'ready'].includes(phase);
  const isBlocked = ['blocked', 'budget-stopped'].includes(phase);
  const isPlanningActive =
    ['drafting', 'reviewing', 'activating'].includes(phase) || activeRequest?.status === 'running';
  const hasApprovableRef = Boolean(design?.revision && design?.digest);
  const needsHumanApproval = Boolean(
    !isApproved &&
      hasApprovableRef &&
      (phase === 'awaiting-design-approval' ||
        phase === 'awaiting_approval' ||
        (legacyDesign && !legacyDesign.approved)),
  );

  let stepperStep: 1 | 2 | 3 | 4 | 5 = 1;
  if (isApproved) {
    stepperStep = 5;
  } else if (needsHumanApproval) {
    stepperStep = 4;
  } else if (phase === 'reviewing' || activeRequest?.stage === 'reviewer' || (design && !isBlocked)) {
    stepperStep = 3;
  } else if (
    phase === 'drafting' ||
    isBlocked ||
    phase === 'cancelled' ||
    activeRequest?.stage === 'planner'
  ) {
    stepperStep = 2;
  }

  return {
    phase,
    policy,
    mode,
    briefGoal,
    briefOriginalRequest,
    constraints,
    reason,
    rounds,
    maxRounds,
    activeRequest,
    design,
    review,
    isApproved,
    needsHumanApproval,
    isBlocked,
    isPlanningActive,
    stepperStep,
  };
}

export function resolveCandidateLabels(candidate: DesignCandidateRevision | null): {
  primaryModel: string;
  secondaryModel: string;
  synthesizerLabel: string;
} {
  const meta = candidate?.meta;
  const primaryModel = meta?.primary?.model || meta?.primaryModel || meta?.primary?.profile || meta?.primaryProfile || '';
  const secondaryModel =
    meta?.secondary?.model || meta?.secondaryModel || meta?.secondary?.profile || meta?.secondaryProfile || '';
  const synthesizerLabel =
    meta?.synthesizer?.profile || meta?.synthesizer?.model || meta?.synthesizerProfile || '';
  return { primaryModel, secondaryModel, synthesizerLabel };
}

export function resolveTaskDisplayStatus(opts: {
  status?: string;
  planningPhase?: string;
  planning?: PlanningState | null;
  state?: TaskState | null;
  running?: boolean;
}): DisplayTaskStatus {
  const running = Boolean(opts.running);
  const resolved = resolvePlanningView(opts.planning ?? opts.state?.planning ?? null);
  const phase = (resolved?.phase ?? opts.planningPhase ?? '').toLowerCase();
  const status = (opts.status ?? opts.state?.status ?? '').toLowerCase();
  const packetCount = opts.state?.packets ? Object.keys(opts.state.packets).length : 0;

  if (running) {
    if (packetCount > 0 && ['drafting', 'reviewing', 'activating'].includes(phase)) {
      return { code: 'refining', tone: 'running', labelKey: 'status.refining' };
    }
    if (
      (resolved && !resolved.isApproved && packetCount === 0) ||
      ['drafting', 'reviewing', 'activating'].includes(phase)
    ) {
      return { code: 'planning', tone: 'running', labelKey: 'status.planning' };
    }
    return { code: 'running', tone: 'running', labelKey: 'status.running' };
  }

  if (resolved?.isBlocked || phase === 'blocked' || phase === 'budget-stopped') {
    return { code: 'planning_blocked', tone: 'danger', labelKey: 'status.planning_blocked' };
  }

  if (
    resolved?.needsHumanApproval ||
    phase === 'awaiting-design-approval' ||
    phase === 'awaiting_approval' ||
    status === 'awaiting_approval'
  ) {
    return { code: 'awaiting_approval', tone: 'warn', labelKey: 'status.awaiting_approval' };
  }

  if (['completed', 'done', 'passed'].includes(status)) {
    return { code: status, tone: 'success', labelKey: `status.${status}` };
  }
  if (['failed', 'blocked', 'escalated'].includes(status)) {
    return { code: status, tone: 'danger', labelKey: `status.${status}` };
  }
  if (['stopped', 'paused'].includes(status)) {
    return { code: status, tone: 'warn', labelKey: `status.${status}` };
  }
  if (['running', 'executing', 'reviewing'].includes(status)) {
    return { code: status, tone: 'running', labelKey: `status.${status}` };
  }
  if (status) {
    return { code: status, tone: 'neutral', labelKey: `status.${status}` };
  }
  return { code: 'idle', tone: 'neutral', labelKey: 'header.idle' };
}
