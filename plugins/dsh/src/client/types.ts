export type Translate = (key: string, params?: Record<string, string | number>) => string;

export type MainTab = 'design' | 'execution' | 'investigation' | 'delivery' | 'traces' | 'settings';
export type ExecutionViewMode = 'waves' | 'dag';
export type DesignViewMode = 'synthesized' | 'side-by-side' | 'raw';
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

export interface PlanningFinding {
  id?: string;
  severity?: 'blocker' | 'major' | 'minor' | string;
  status?: 'open' | 'resolved' | string;
  title?: string;
  summary?: string;
  detail?: string;
  section?: string;
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

export interface PlanningState {
  status?: string;
  policy?: 'review-first' | 'delegated' | string;
  mode?: string;
  revision?: number;
  design?: PlanningDesignSnapshot;
  findings?: PlanningFinding[];
  brief?: {
    goal?: string;
    summary?: string;
    title?: string;
    constraints?: string[];
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

export interface DesignCandidateRevision {
  revision: number;
  primary: string;
  secondary: string;
  meta: {
    revision?: number;
    primaryProfile?: string;
    secondaryProfile?: string;
    synthesizerProfile?: string;
    primaryModel?: string;
    secondaryModel?: string;
    generatedAt?: string;
    fallback?: boolean;
    fallbackReason?: string;
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

export interface DeliverySnapshot {
  endpoint?: 'local' | 'draft-pr' | 'pr-review' | 'auto-merge' | string;
  repo?: string;
  branch?: string;
  baseBranch?: string;
  head?: string;
  dirty?: boolean;
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

export interface ConfigSnapshot {
  source: string;
  scope: string;
  bindings: ConfigBinding[];
  sessions?: Record<string, unknown>;
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
  kind: 'thinking' | 'tool_call' | 'tool_result' | 'assistant' | 'progress' | 'error';
  ts?: string;
  title?: string;
  text: string;
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
  status: 'running' | 'completed' | 'failed' | 'cancelled' | 'blocked' | string;
  running: boolean;
  exitCode?: number | null;
  signal?: string;
  durationMs?: number | null;
  startedAt?: string;
  updatedAt?: string;
  sessionId?: string;
  resume?: TraceResumeInfo | null;
  errorCode?: string;
  errorSummary?: string;
  usage?: Record<string, unknown> | null;
  source?: 'live' | 'receipt' | 'state' | string;
  eventCount?: number;
}

export interface TraceDetail extends TraceSummary {
  blocks: TraceBlock[];
  stderr?: string;
  text?: string;
  truncated?: boolean;
  rawTail?: string[];
}

export interface ControlResponse {
  protocol: 1;
  ok: boolean;
  type?: string;
  action?: string | null;
  dispatch?: 'not-dispatched' | 'unknown' | string;
  message?: string;
  taskDir?: string;
  cwd?: string;
  running?: boolean;
  state?: TaskState | null;
  runs?: RunSummary[];
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
