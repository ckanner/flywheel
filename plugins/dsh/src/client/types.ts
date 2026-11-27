export type Translate = (key: string, params?: Record<string, string | number>) => string;

export type MainTab = 'design' | 'execution' | 'investigation' | 'delivery' | 'traces' | 'settings';
export type WorkbenchView = 'new-task' | 'main' | 'sub';
export type WorkbenchDrawer = 'design' | 'dag' | 'delivery' | 'settings' | null;
export type ExecutionViewMode = 'waves' | 'dag';
export type DesignViewMode = 'synthesized' | 'side-by-side' | 'structured' | 'raw';
export type InspectorTab = 'overview' | 'output' | 'evidence' | 'timeline' | 'contract';

export interface RunSummary {
  taskDir: string;
  cwd?: string;
  workspaceCwd?: string;
  repoName?: string;
  slug?: string;
  title?: string;
  status?: string;
  phase?: string;
  mode?: string;
  workflow?: string;
  branch?: string;
  running?: boolean;
  hostSessionId?: string;
  startedAt?: string;
  finishedAt?: string;
  updatedAt?: string;
  revision?: number;
  planningStatus?: string;
  investigationStatus?: string;
  runCount?: number;
  isTemp?: boolean;
  summary?: {
    slug?: string;
    status?: string;
    revision?: number;
    'planning-phase'?: string;
    'packets-completed'?: string;
    'current-milestone'?: string | null;
    'last-progress-at'?: string | null;
    tokens?: number;
    'cost-usd'?: number;
  };
}

export interface PlanningRef {
  revision: number;
  digest: string;
}

export interface PlanningRequirement {
  id: string;
  title: string;
  milestone?: string;
  verification?: string;
}

export interface PlanningDecision {
  id: string;
  kind?: 'fixed' | 'delegated' | 'unresolved' | string;
  statement: string;
  rationale?: string;
}

export interface PlanningMilestone {
  id: string;
  title: string;
  gate?: string;
}

export interface PlanningDesignRecord {
  ref: PlanningRef;
  'input-revision'?: number;
  document?: string;
  requirements?: PlanningRequirement[];
  decisions?: PlanningDecision[];
  milestones?: PlanningMilestone[];
  baseline?: string;
  'planner-invocation'?: string;
  'implementation-scope'?: string[];
}

export interface PlanningFinding {
  id?: string;
  severity?: 'blocker' | 'blocking' | 'major' | 'minor' | 'advisory' | string;
  status?: 'open' | 'resolved' | string;
  title?: string;
  target?: string;
  summary?: string;
  detail?: string;
  evidence?: string;
  impact?: string;
  resolution?: string;
  section?: string;
}

export interface PlanningReviewRecord {
  id: string;
  kind: string;
  target: PlanningRef;
  'input-revision'?: number;
  invocation?: string;
  role?: string;
  binding?: string;
  verdict: string;
  'design-verdict'?: string;
  'coverage-verdict'?: string;
  'scope-verdict'?: string;
  summary?: string;
  findings?: PlanningFinding[];
}

export interface PlanningApprovalRecord {
  kind: 'human' | 'delegated' | string;
  design: PlanningRef;
  'review-id'?: string;
  'request-id'?: string;
  source?: string;
  at?: string;
  start?: boolean;
}

export interface PlanningRequestRecord {
  id: string;
  scope: string;
  'scope-id'?: string;
  'input-revision'?: number;
  rounds?: number;
  retries?: number;
  'max-rounds'?: number;
  'grace-rounds'?: number;
  status?: string;
  'planning-only'?: boolean;
  feedback?: string;
  stage?: string;
  role?: string;
  channel?: string;
  'binding-source'?: string;
  'binding-granularity'?: string;
  'design-granularity'?: string;
}

export interface PlanningDesignSnapshot {
  revision?: number;
  digest?: string;
  status?: string;
  summary?: string;
  content?: string;
  markdown?: string;
  path?: string;
  approved?: boolean;
  approvedAt?: string;
  reviewedBy?: string;
  findings?: PlanningFinding[];
}

export interface PlanningClarificationQuestion {
  id: string;
  title: string;
  kind: 'choice' | 'quality-gate' | 'text' | string;
  options?: string[];
  recommended?: string;
  rationale?: string;
  required?: boolean;
}

export interface PlanningClarification {
  status: 'pending' | 'skipped' | 'awaiting-answers' | 'confirmed' | string;
  'decided-by'?: string;
  'needs-clarification': boolean;
  'is-frontend-ui': boolean;
  summary?: string;
  'suggested-gates'?: string[];
  questions?: PlanningClarificationQuestion[];
  answers?: Record<string, string>;
  'selected-gates'?: string[];
  'freeform-note'?: string;
  'ui-prototype-mode'?: 'off' | 'auto' | 'always' | string;
  invocation?: string;
  'answered-at'?: string;
  'request-id'?: string;
}

export interface PlanningUIPrototypeReview {
  verdict: 'pass' | 'revise' | string;
  summary: string;
  findings: PlanningFinding[];
  model?: string;
  at?: string;
}

export interface PlanningUIPrototype {
  revision: number;
  design: PlanningRef;
  status: 'drafting' | 'reviewing' | 'awaiting-approval' | 'approved' | 'skipped' | string;
  'html-file'?: string;
  summary?: string;
  'design-tokens'?: Record<string, string>;
  states?: string[];
  review?: PlanningUIPrototypeReview;
  feedback?: string[];
  'approved-at'?: string;
  'approval-source'?: string;
  'approval-request-id'?: string;
  model?: string;
  invocation?: string;
}

export interface PlanningRetryTarget {
  scope: 'design' | 'milestone' | 'packet' | string;
  'scope-id'?: string;
  'continue-execution'?: boolean;
  'failed-request-id'?: string;
  'cancellable-request-id'?: string;
}

export interface PlanningProjection {
  kind?: string;
  'approval-complete'?: boolean;
  ledger?: string;
  omitted?: Record<string, number>;
  'retry-target'?: PlanningRetryTarget;
}

export interface PlanningState {
  projection?: PlanningProjection | null;
  version?: number;
  phase?: string;
  status?: string;
  policy?: 'review-first' | 'delegated' | string;
  mode?: string;
  revision?: number;
  'input-revision'?: number;
  rounds?: number;
  'max-rounds'?: number;
  'start-authorized'?: boolean;
  reason?: string;
  requests?: PlanningRequestRecord[];
  designs?: PlanningDesignRecord[];
  'current-design'?: PlanningRef | null;
  reviews?: PlanningReviewRecord[];
  approvals?: PlanningApprovalRecord[];
  clarification?: PlanningClarification | null;
  'ui-prototype'?: PlanningUIPrototype | null;
  design?: PlanningDesignSnapshot;
  findings?: PlanningFinding[];
  brief?: {
    id?: string;
    goal?: string;
    'original-request'?: string;
    summary?: string;
    title?: string;
    policy?: string;
    mode?: string;
    'design-granularity'?: string;
    'clarify-mode'?: 'off' | 'auto' | 'always' | string;
    'ui-prototype-mode'?: 'off' | 'auto' | 'always' | string;
    'quality-gates'?: string[];
    constraints?: string[];
    evidence?: string[];
  };
  pendingRequestId?: string;
  lastError?: string;
  dualDraft?: {
    enabled?: boolean;
    primaryProfile?: string;
    secondaryProfile?: string;
    synthesizerProfile?: string;
  };
}

export interface DesignCandidateModelMeta {
  profile?: string;
  harness?: string;
  model?: string;
}

export interface DesignCandidateRevision {
  revision: number;
  primary: string;
  secondary: string;
  meta: {
    revision?: number;
    inputRevision?: number;
    primary?: DesignCandidateModelMeta;
    secondary?: DesignCandidateModelMeta;
    synthesizer?: DesignCandidateModelMeta;
    primaryProfile?: string;
    secondaryProfile?: string;
    synthesizerProfile?: string;
    primaryModel?: string;
    secondaryModel?: string;
    createdAt?: string;
    generatedAt?: string;
    fallback?: boolean;
    fallbackReason?: string;
    partial?: boolean;
    completedSide?: 'primary' | 'secondary' | string;
    failedSide?: 'primary' | 'secondary' | string;
    failureCode?: string;
    failureError?: string;
    [key: string]: unknown;
  } | null;
}

export interface DesignCandidatesResponse {
  protocol: 1;
  ok: boolean;
  taskDir: string;
  revisions: DesignCandidateRevision[];
}

export interface UXReviewFinding {
  severity?: 'blocking' | 'advisory' | string;
  category?: string;
  location?: string;
  summary?: string;
  suggestion?: string;
}

export interface UXReviewReport {
  verdict?: 'pass' | 'needs_polish' | string;
  summary?: string;
  findings?: UXReviewFinding[];
  profile?: string;
  model?: string;
  refined?: boolean;
  refinedBy?: string;
}

export interface PacketNode {
  id: string;
  title?: string;
  goal?: string;
  status?: string;
  wave?: number;
  dependsOn?: string[];
  depends_on?: string[];
  route?: string;
  profile?: string;
  model?: string;
  attempts?: number;
  reviewRounds?: number;
  review_rounds?: number;
  l1Status?: string;
  uxStatus?: string;
  reviewStatus?: string;
  reviewVerdict?: string;
  advisory?: string[];
  blocking?: string[];
  writeScopes?: string[];
  write_scopes?: string[];
  requirements?: string[];
  boundaries?: string[];
  images?: string[];
  sessionKey?: string;
  sessionStatus?: string;
  unknownInvocation?: string;
  lastError?: string;
  summary?: string;
  startedAt?: string;
  finishedAt?: string;
  collaboration?: {
    children?: Array<{
      id: string;
      goal?: string;
      status?: string;
      route?: string;
    }>;
  };
  [key: string]: unknown;
}

export interface GuardrailsState {
  maxPacketAttempts?: number;
  maxReviewRounds?: number;
  maxTaskCalls?: number;
  currentCalls?: number;
}

export interface BudgetState {
  enforce?: 'off' | 'warn' | 'hard' | string;
  spentUsd?: number;
  limitUsd?: number;
  totals?: {
    calls?: number;
    cost_usd?: number;
    input_tokens?: number;
    output_tokens?: number;
  };
}

export interface InvestigationHypothesis {
  id?: string;
  title?: string;
  statement?: string;
  status?: 'open' | 'confirmed' | 'refuted' | 'inconclusive' | string;
  confidence?: string;
  evidence?: string[];
}

export interface InvestigationActionLog {
  id?: string;
  invocationId?: string;
  summary?: string;
  command?: string;
  status?: string;
  timestamp?: string;
  finding?: string;
}

export interface InvestigationState {
  status?: string;
  revision?: number;
  goal?: string;
  intent?: string;
  readPaths?: string[];
  hypotheses?: InvestigationHypothesis[];
  actions?: InvestigationActionLog[];
  report?: {
    summary?: string;
    conclusion?: string;
    recommendations?: string[];
    citations?: string[];
  };
  contract?: Record<string, unknown>;
}

export interface TaskState {
  slug?: string;
  title?: string;
  status?: string;
  phase?: string;
  mode?: string;
  modeReason?: string;
  revision?: number;
  packets?: Record<string, PacketNode>;
  waves?: string[][];
  planning?: PlanningState;
  investigation?: InvestigationState;
  guardrails?: GuardrailsState;
  budget?: BudgetState;
  delivery?: {
    endpoint?: string;
    branch?: string;
    baseBranch?: string;
  };
  activePacket?: string;
  lastError?: string;
  updatedAt?: string;
  [key: string]: unknown;
}

export interface DeliveryPullRequest {
  number: number;
  title?: string;
  state?: string;
  draft?: boolean;
  url?: string;
  headRefName?: string;
  baseRefName?: string;
  headSha?: string;
  mergeable?: string;
  checks?: Array<{
    name: string;
    status?: string;
    conclusion?: string;
  }>;
}

export interface DeliveryLocalCommit {
  sha: string;
  subject: string;
  at?: string;
}

export interface DeliverySnapshot {
  mode?: 'local' | 'github' | string;
  remoteStatus?: 'ready' | 'no-origin' | 'non-github' | 'gh-unavailable' | string;
  remoteMessage?: string;
  endpoint?: 'local' | 'draft-pr' | 'pr-review' | 'auto-merge' | string;
  repo?: string;
  url?: string;
  localBranch?: string;
  branch?: string;
  baseBranch?: string;
  defaultBase?: string;
  head?: string;
  dirty?: boolean;
  workingTreeClean?: boolean;
  recentCommits?: DeliveryLocalCommit[];
  changedFiles?: string[];
  ahead?: number;
  behind?: number;
  pulls?: DeliveryPullRequest[];
  canCreate?: boolean;
  canMerge?: boolean;
  reason?: string;
}

export interface ConfigBinding {
  id: string;
  role: string;
  harness: string;
  route?: string;
  profile?: string;
  nativeProfile?: string;
  provider?: string;
  channel?: string;
  model?: string;
  thinking?: string;
  permissionMode?: string;
  residentSession?: boolean;
  inScope?: boolean;
  session?: {
    mode?: string;
    supported?: boolean;
    reason?: string;
  };
}

export interface JEVConfigSnapshot {
  enabled?: boolean;
  backend?: 'subscription' | 'http' | string;
  profile?: string;
  endpoint?: string;
  model?: string;
  'key-env'?: string;
  'timeout-ms'?: number;
  'max-calls'?: number;
  'min-applicability'?: number;
  'min-choice'?: number;
  'allowed-data'?: string[];
}

export interface ConfigSnapshot {
  source: string;
  scope: string;
  bindings: ConfigBinding[];
  sessions?: Record<string, unknown>;
  subscription?: {
    version?: number;
    mode?: string;
  };
  jev?: JEVConfigSnapshot;
  jevBinding?: ConfigBinding & { bindingSource?: string };
}

export interface ConfigEditorSnapshot {
  scope: 'global' | 'project';
  target?: string;
  source?: string;
  revision: string;
  values: Record<string, unknown>;
  defaults: Record<string, unknown>;
  applied?: boolean;
}

export interface DoctorCheck {
  id: string;
  status: 'pass' | 'fail' | 'warn' | 'unknown' | 'skipped' | string;
  summary: string;
  code?: string;
  remedy?: string;
}

export interface DoctorReport {
  kind: 'doctor';
  ok: boolean;
  cwd: string;
  taskDir: string;
  probeRequested: boolean;
  inferenceTested: boolean;
  configuration?: ConfigSnapshot;
  checks: DoctorCheck[];
}

export interface ModelCatalogEntry {
  id: string;
  label?: string;
  efforts?: string[];
  defaultEffort?: string;
}

export interface ModelCatalogReport {
  version: number;
  harness: string;
  channel?: string;
  provider?: string;
  status: string;
  source?: string;
  scope?: string;
  models: ModelCatalogEntry[];
  truncated: boolean;
  inferencePerformed: boolean;
  accountEntitlement?: string;
  limitations?: string[];
}

export interface RepoImageEntry {
  path: string;
  bytes: number;
  ext: string;
}

export interface NodeDetailSnapshot {
  packet: PacketNode;
  l1?: Array<{ name?: string; command?: string; passed?: boolean; output?: string }>;
  review?: {
    verdict?: string;
    summary?: string;
    blocking?: string[];
    advisory?: string[];
    round?: number;
  };
  timeline?: Array<{
    at?: string;
    event?: string;
    attempt?: number;
    status?: string;
    detail?: string;
  }>;
  contract?: Record<string, unknown>;
  images?: {
    paths?: string[];
    frozen?: Array<{ path: string; sha256?: string; bytes?: number }>;
  };
}

export interface TraceResumeInfo {
  kind: 'dsh-native' | 'cli';
  harness: string;
  providerLabel: string;
  sessionId: string;
  cwd?: string;
  command?: string;
  secondaryCommand?: string;
}

export interface TraceBlock {
  kind: 'user_prompt' | 'thinking' | 'tool_call' | 'tool_result' | 'assistant' | 'progress' | 'steps' | 'error';
  ts?: string;
  title?: string;
  text: string;
  items?: string[];
  rawJson?: string;
}

export interface TraceSummary {
  id: string;
  key: string;
  role: string;
  scope: string;
  attempt?: number;
  harness: string;
  model: string;
  thinking?: string;
  cwd?: string;
  workspaceCwd?: string;
  status: 'running' | 'completed' | 'failed' | 'cancelled' | 'blocked' | string;
  running: boolean;
  exitCode?: number | null;
  signal?: string;
  durationMs?: number | null;
  startedAt?: string;
  updatedAt?: string;
  sessionId?: string;
  sessionTitle?: string;
  resume?: TraceResumeInfo | null;
  errorCode?: string;
  errorSummary?: string;
  usage?: Record<string, unknown> | null;
  contextUsage?: {
    used: number;
    size: number;
    source?: string;
    lastInput?: number;
    lastCacheRead?: number;
  } | null;
  promptBytes?: number | null;
  source?: 'live' | 'receipt' | 'state' | string;
  eventCount?: number;
  byteSize?: number;
  preview?: string;
}

export interface SessionHistoryTurn {
  id?: string;
  kind?: string;
  role?: 'user' | 'assistant' | 'system' | string;
  at?: string;
  prompt?: string;
  text?: string;
  summary?: string;
  status?: string;
  [key: string]: unknown;
}

export interface TraceDetail extends TraceSummary {
  blocks: TraceBlock[];
  stderr?: string;
  text?: string;
  truncated?: boolean;
  rawTail?: string[];
}

export interface PacketReviewFinding {
  id?: string;
  severity?: 'blocker' | 'major' | 'minor' | string;
  target?: string;
  evidence?: string;
  fix?: string;
  status?: 'open' | 'resolved' | string;
  resolution?: string;
}

export interface PacketControlReceipt {
  'packet-id'?: string;
  action?: string;
  applied?: boolean;
  revision?: number;
  status?: string;
  message?: string;
}

export interface MilestoneControlReceipt {
  'milestone-id'?: string;
  action?: string;
  applied?: boolean;
  revision?: number;
  status?: string;
  message?: string;
}

export interface ControlResponse {
  protocol: 1;
  ok: boolean;
  type?: string;
  action?: string | null;
  dispatch?: 'not-dispatched' | 'unknown' | string;
  message?: string;
  error?: string;
  'session-control'?: { key?: string; action?: string; applied?: boolean; message?: string };
  sessionControl?: { key?: string; action?: string; applied?: boolean; message?: string };
  'packet-control'?: PacketControlReceipt;
  packetControl?: PacketControlReceipt;
  'milestone-control'?: MilestoneControlReceipt;
  milestoneControl?: MilestoneControlReceipt;
  taskDir?: string;
  cwd?: string;
  running?: boolean;
  state?: TaskState | null;
  runs?: RunSummary[];
  html?: string;
  htmlFile?: string;
  planning?: PlanningState;
  delivery?: DeliverySnapshot;
  configuration?: ConfigSnapshot;
  editor?: ConfigEditorSnapshot;
  doctor?: DoctorReport;
  diagnostic?: DoctorReport;
  modelCatalog?: ModelCatalogReport;
  node?: NodeDetailSnapshot;
  metrics?: Record<string, unknown>;
  history?: unknown[];
  revisions?: DesignCandidateRevision[];
  uxReview?: UXReviewReport | null;
  traces?: TraceSummary[];
  trace?: TraceDetail | null;
  files?: RepoImageEntry[];
  workspaceWarnings?: string[];
  [key: string]: unknown;
}
