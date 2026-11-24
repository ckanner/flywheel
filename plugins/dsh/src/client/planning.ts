import type {
  DesignCandidateRevision,
  PlanningDecision,
  PlanningFinding,
  PlanningMilestone,
  PlanningRequestRecord,
  PlanningRequirement,
  PlanningState,
  TaskState,
} from './types.ts';

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
