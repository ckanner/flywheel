import type { ClientContext } from './context.ts';
import { resolvePlanningView } from './planning.ts';
import { FlywheelRemoteClient, FlywheelRemoteError } from './remote.ts';
import type {
  ConfigEditorSnapshot,
  ConfigSnapshot,
  ControlResponse,
  DeliverySnapshot,
  DesignCandidateRevision,
  DesignViewMode,
  DoctorReport,
  ExecutionViewMode,
  InspectorTab,
  MainTab,
  ModelCatalogReport,
  NodeDetailSnapshot,
  PlanningState,
  RepoImageEntry,
  RunSummary,
  SessionHistoryTurn,
  TaskState,
  TraceDetail,
  TraceSummary,
  UXReviewReport,
  WorkbenchDrawer,
  WorkbenchView,
} from './types.ts';

export interface StoreError {
  message: string;
  dispatch?: string;
  action?: string | null;
}

export interface PreviewArtifactState {
  title: string;
  relPath: string;
  absPath: string;
  kind: 'html' | 'markdown' | 'json' | 'code' | 'text';
  content: string;
  loading: boolean;
  fullscreen: boolean;
  error: string | null;
}

export interface FlywheelSnapshot {
  runs: RunSummary[];
  runsLoading: boolean;
  runsError: string | null;
  selectedTaskDir: string | null;
  selectedCwd: string;
  railCollapsed: boolean;
  showTempRuns: boolean;
  activeTab: MainTab;
  workbenchView: WorkbenchView;
  activeNodeId: string | null;
  activeSessionKey: string | null;
  activeDrawer: WorkbenchDrawer;
  executionView: ExecutionViewMode;
  designView: DesignViewMode;
  selectedPacketId: string | null;
  inspectorTab: InspectorTab;
  createModalOpen: boolean;

  // Selected task data
  taskLoading: boolean;
  running: boolean;
  state: TaskState | null;
  planning: PlanningState | null;
  designCandidates: DesignCandidateRevision[];
  uiPrototypeHtml: string;
  uiPrototypeFile: string;
  previewArtifact: PreviewArtifactState | null;
  nodeDetail: NodeDetailSnapshot | null;
  uxReviews: Record<string, UXReviewReport | null>;
  delivery: DeliverySnapshot | null;
  traces: TraceSummary[];
  tracesLoading: boolean;
  selectedTraceId: string | null;
  traceDetail: TraceDetail | null;
  sessionHistories: Record<string, SessionHistoryTurn[]>;
  claimedSessions: Record<string, boolean>;

  // Settings & diagnostics
  configScope: 'global' | 'project';
  configEditor: ConfigEditorSnapshot | null;
  effectiveConfig: ConfigSnapshot | null;
  doctor: DoctorReport | null;
  modelCatalogs: Record<string, ModelCatalogReport>;
  repoImages: RepoImageEntry[];

  // Action feedback
  busyAction: string | null;
  error: StoreError | null;
  notice: string | null;
  lastUpdatedAt: number | null;
}

function initialSnapshot(): FlywheelSnapshot {
  return {
    runs: [],
    runsLoading: false,
    runsError: null,
    selectedTaskDir: null,
    selectedCwd: '',
    railCollapsed: false,
    showTempRuns: false,
    activeTab: 'execution',
    workbenchView: 'main',
    activeNodeId: null,
    activeSessionKey: null,
    activeDrawer: null,
    executionView: 'waves',
    designView: 'synthesized',
    selectedPacketId: null,
    inspectorTab: 'overview',
    createModalOpen: false,

    taskLoading: false,
    running: false,
    state: null,
    planning: null,
    designCandidates: [],
    uiPrototypeHtml: '',
    uiPrototypeFile: '',
    previewArtifact: null,
    nodeDetail: null,
    uxReviews: {},
    delivery: null,
    traces: [],
    tracesLoading: false,
    selectedTraceId: null,
    traceDetail: null,
    sessionHistories: {},
    claimedSessions: {},

    configScope: 'global',
    configEditor: null,
    effectiveConfig: null,
    doctor: null,
    modelCatalogs: {},
    repoImages: [],

    busyAction: null,
    error: null,
    notice: null,
    lastUpdatedAt: null,
  };
}

function basenameOf(pathValue?: string): string {
  if (!pathValue) return '';
  const trimmed = pathValue.replace(/\/+$/, '');
  const idx = trimmed.lastIndexOf('/');
  return idx >= 0 ? trimmed.slice(idx + 1) : trimmed;
}

function isTempPath(pathValue?: string): boolean {
  if (!pathValue) return false;
  return /^(\/private)?\/(tmp|var\/folders)\//.test(pathValue);
}

function runTimestampMs(run: RunSummary): number {
  const raw = run.updatedAt || run.finishedAt || run.startedAt || '';
  if (!raw) return 0;
  const parsed = Date.parse(raw);
  return Number.isFinite(parsed) ? parsed : 0;
}

/**
 * Normalizes raw `flywheel control runs` entries (both nested Go `summary.*`
 * records and flat mock summaries), deduplicates by `taskDir` keeping the
 * freshest record + run count, and tags temporary `/tmp` runs cleanly.
 */
export function normalizeRuns(rawRuns: RunSummary[]): RunSummary[] {
  const byTask = new Map<string, RunSummary>();
  for (const raw of rawRuns) {
    if (!raw || typeof raw.taskDir !== 'string' || !raw.taskDir) continue;
    const slug = raw.slug || raw.summary?.slug || basenameOf(raw.taskDir);
    const cwd = raw.workspaceCwd || raw.cwd || '';
    const repoName = raw.repoName || basenameOf(cwd) || basenameOf(raw.taskDir.replace(/\/docs\/[^/]+$/, ''));
    const status = raw.status || raw.summary?.status || 'pending';
    const mode = raw.mode || raw.workflow || undefined;
    const updatedAt = raw.updatedAt || raw.finishedAt || raw.startedAt || undefined;
    const revision = raw.revision ?? raw.summary?.revision;
    const planningStatus = raw.planningStatus ?? raw.summary?.['planning-phase'];
    const isTemp = raw.isTemp ?? (isTempPath(raw.taskDir) || isTempPath(cwd));

    const normalized: RunSummary = {
      ...raw,
      cwd,
      slug,
      title: raw.title || slug,
      repoName,
      status,
      mode,
      updatedAt,
      revision,
      planningStatus,
      isTemp,
      runCount: raw.runCount ?? 1,
    };

    const existing = byTask.get(raw.taskDir);
    if (!existing) {
      byTask.set(raw.taskDir, normalized);
    } else {
      const nextCount = (existing.runCount ?? 1) + 1;
      const keepIncoming =
        (Boolean(normalized.running) && !existing.running) ||
        (Boolean(normalized.running) === Boolean(existing.running) && runTimestampMs(normalized) > runTimestampMs(existing));
      const winner = keepIncoming ? normalized : existing;
      byTask.set(raw.taskDir, { ...winner, runCount: nextCount });
    }
  }

  return [...byTask.values()].sort((a, b) => {
    if (Boolean(a.running) !== Boolean(b.running)) return a.running ? -1 : 1;
    return runTimestampMs(b) - runTimestampMs(a);
  });
}

function inferDefaultTab(state: TaskState | null, planning: PlanningState | null): MainTab {
  if (!state) return 'execution';
  const packetCount = state.packets ? Object.keys(state.packets).length : 0;
  if (state.investigation && (state.investigation.status || state.investigation.goal) && packetCount === 0) {
    return 'investigation';
  }
  const st = (state.status ?? '').toLowerCase();
  if (st === 'completed' || st === 'done') return 'delivery';
  const resolved = resolvePlanningView(planning ?? state.planning ?? null);
  if (resolved && (packetCount === 0 || resolved.needsHumanApproval)) {
    return 'design';
  }
  return 'execution';
}

function reconcileSelectedRun(
  runs: RunSummary[],
  taskDir: string | null,
  running: boolean,
  state: TaskState | null,
  planning: PlanningState | null,
): RunSummary[] {
  if (!taskDir) return runs;
  const resolved = resolvePlanningView(planning ?? state?.planning ?? null);
  const liveStatus = state?.status;
  const livePhase = resolved?.phase || undefined;
  const liveRevision = state?.revision;
  let changed = false;
  const next = runs.map(run => {
    if (run.taskDir !== taskDir) return run;
    const nextStatus = liveStatus || run.status;
    const nextPhase = livePhase ?? run.planningStatus;
    const nextRevision = liveRevision ?? run.revision;
    if (
      Boolean(run.running) === running &&
      run.status === nextStatus &&
      run.planningStatus === nextPhase &&
      run.revision === nextRevision
    ) {
      return run;
    }
    changed = true;
    return {
      ...run,
      running,
      status: nextStatus,
      planningStatus: nextPhase,
      revision: nextRevision,
    };
  });
  return changed ? next : runs;
}

export class FlywheelStore {
  readonly remote: FlywheelRemoteClient;
  private snapshot: FlywheelSnapshot = initialSnapshot();
  private readonly listeners = new Set<() => void>();
  private pollTimer: ReturnType<typeof setTimeout> | null = null;
  private started = false;
  private disposed = false;
  private taskRequestSeq = 0;
  private liveStreamGen = 0;
  private visibilityHandler: (() => void) | null = null;

  constructor(ctx: ClientContext, ready: Promise<string | undefined>) {
    this.remote = new FlywheelRemoteClient(ctx, ready);
  }

  getSnapshot = (): FlywheelSnapshot => this.snapshot;

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  private setState(patch: Partial<FlywheelSnapshot>): void {
    this.snapshot = { ...this.snapshot, ...patch };
    for (const listener of [...this.listeners]) listener();
  }

  private toStoreError(error: unknown, fallbackAction?: string): StoreError {
    if (error instanceof FlywheelRemoteError) {
      return { message: error.message, dispatch: error.dispatch, action: error.action ?? fallbackAction };
    }
    return {
      message: error instanceof Error ? error.message : String(error),
      action: fallbackAction,
    };
  }

  start(): void {
    if (this.started || this.disposed) return;
    this.started = true;
    if (typeof document !== 'undefined' && typeof document.addEventListener === 'function') {
      this.visibilityHandler = () => {
        if (document.visibilityState === 'visible') {
          this.schedulePoll(100);
        } else if (this.pollTimer) {
          clearTimeout(this.pollTimer);
          this.pollTimer = null;
        }
      };
      document.addEventListener('visibilitychange', this.visibilityHandler);
    }
    void this.refreshRuns().then(() => {
      if (this.started && !this.disposed) this.schedulePoll();
    });
  }

  stop(): void {
    this.started = false;
    this.disposed = true;
    this.liveStreamGen += 1;
    if (this.pollTimer) {
      clearTimeout(this.pollTimer);
      this.pollTimer = null;
    }
    if (this.visibilityHandler && typeof document !== 'undefined' && typeof document.removeEventListener === 'function') {
      document.removeEventListener('visibilitychange', this.visibilityHandler);
      this.visibilityHandler = null;
    }
  }

  private schedulePoll(overrideMs?: number): void {
    if (!this.started || this.disposed) return;
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;
    if (this.pollTimer) clearTimeout(this.pollTimer);
    const viewingLiveTraces =
      this.snapshot.workbenchView === 'main' ||
      this.snapshot.workbenchView === 'sub' ||
      this.snapshot.activeTab === 'traces' ||
      this.snapshot.activeTab === 'design' ||
      (this.snapshot.selectedPacketId !== null && this.snapshot.inspectorTab === 'output');
    const anyTraceRunning = this.snapshot.traces.some(tr => tr.running);
    const liveActive = this.snapshot.running || anyTraceRunning;
    const delay = overrideMs ?? (viewingLiveTraces && liveActive ? 1500 : liveActive ? 2500 : 12000);
    this.pollTimer = setTimeout(async () => {
      this.pollTimer = null;
      if (!this.started || this.disposed) return;
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;
      await this.refreshRuns(true);
      if (this.snapshot.selectedTaskDir) {
        await this.refreshSelectedTask(true);
        if (viewingLiveTraces) {
          await this.loadTraces(true);
          if (this.snapshot.workbenchView === 'sub' && this.snapshot.selectedTraceId) {
            await this.loadTraceDetail(this.snapshot.selectedTraceId, true);
          }
        }
      }
      this.schedulePoll();
    }, delay);
  }

  private pumpLiveTraceStream(taskDir: string, traceId: string): void {
    const gen = ++this.liveStreamGen;
    const step = async () => {
      if (this.disposed || !this.started || gen !== this.liveStreamGen) return;
      if (this.snapshot.selectedTaskDir !== taskDir || this.snapshot.workbenchView !== 'sub') return;
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;
      const cur = this.snapshot.traceDetail;
      const waitSinceBytes = cur && cur.id === traceId && cur.running && typeof cur.byteSize === 'number'
        ? cur.byteSize
        : undefined;
      try {
        const res = await this.remote.query({
          action: 'trace-detail',
          taskDir,
          traceId,
          ...(waitSinceBytes !== undefined ? { waitSinceBytes } : {}),
        });
        if (this.disposed || gen !== this.liveStreamGen || this.snapshot.selectedTaskDir !== taskDir) return;
        const trace = (res.trace ?? null) as TraceDetail | null;
        if (trace && this.snapshot.selectedTraceId === traceId) {
          this.setState({ traceDetail: trace });
          if (trace.running) {
            setTimeout(() => void step(), 60);
          }
        }
      } catch {
        // Fallback polling continues cleanly.
      }
    };
    void step();
  }

  startNewTaskChat(): void {
    this.liveStreamGen += 1;
    this.setState({
      workbenchView: 'new-task',
      activeNodeId: null,
      activeSessionKey: null,
      activeDrawer: null,
      selectedPacketId: null,
      error: null,
    });
  }

  returnToMainStream(): void {
    this.liveStreamGen += 1;
    this.setState({
      workbenchView: this.snapshot.selectedTaskDir ? 'main' : 'new-task',
      activeNodeId: null,
      activeSessionKey: null,
      selectedPacketId: null,
    });
    if (this.snapshot.selectedTaskDir) {
      void this.refreshSelectedTask(true, false);
      void this.loadTraces(true);
    }
  }

  drillIntoNode(
    nodeId: string,
    opts?: { traceId?: string | null; sessionKey?: string | null; packetId?: string | null },
  ): void {
    this.liveStreamGen += 1;
    const matchedTrace =
      (opts?.traceId ? this.snapshot.traces.find(tr => tr.id === opts.traceId) : undefined) ??
      this.snapshot.traces.find(
        tr => tr.id === nodeId || tr.scope === nodeId || tr.key === nodeId || tr.key.endsWith(`/${nodeId}`),
      ) ??
      null;
    const nextTraceId = opts?.traceId ?? matchedTrace?.id ?? null;
    const nextSessionKey = opts?.sessionKey ?? matchedTrace?.key ?? (opts?.packetId ? `execute/${opts.packetId}` : null);
    this.setState({
      workbenchView: 'sub',
      activeNodeId: nodeId,
      activeSessionKey: nextSessionKey,
      activeDrawer: null,
      selectedPacketId: opts?.packetId ?? null,
      ...(nextTraceId ? { selectedTraceId: nextTraceId } : {}),
    });
    if (this.snapshot.selectedTaskDir) {
      if (nextSessionKey) void this.loadSessionHistory(nextSessionKey);
      if (opts?.packetId) void this.loadNodeDetail(opts.packetId);
      if (nextTraceId) {
        void this.loadTraceDetail(nextTraceId).then(detail => {
          if (detail?.running && this.snapshot.selectedTaskDir) {
            this.pumpLiveTraceStream(this.snapshot.selectedTaskDir, nextTraceId);
          }
        });
      } else {
        void this.loadTraces(true);
      }
    }
  }

  openDrawer(drawer: WorkbenchDrawer): void {
    this.setState({ activeDrawer: drawer });
    if (drawer === 'design' && this.snapshot.selectedTaskDir) {
      void this.loadDesignCandidates();
      void this.loadTraces(true);
    }
    if (drawer === 'delivery' && this.snapshot.selectedTaskDir) {
      void this.loadDelivery();
    }
    if (drawer === 'settings') {
      void this.loadConfig(this.snapshot.configScope);
    }
  }

  closeDrawer(): void {
    this.setState({ activeDrawer: null });
  }

  setRailCollapsed(collapsed: boolean): void {
    this.setState({ railCollapsed: collapsed });
  }

  setShowTempRuns(show: boolean): void {
    this.setState({ showTempRuns: show });
  }

  setActiveTab(tab: MainTab): void {
    this.setState({ activeTab: tab });
    if (tab === 'design') {
      this.openDrawer('design');
      return;
    }
    if (tab === 'execution') {
      this.openDrawer('dag');
      return;
    }
    if (tab === 'delivery') {
      this.openDrawer('delivery');
      return;
    }
    if (tab === 'settings') {
      this.openDrawer('settings');
      return;
    }
    if (tab === 'traces') {
      const target = this.snapshot.selectedTraceId ?? this.snapshot.traces[0]?.id ?? null;
      this.openTraceInWorkbench(target);
      return;
    }
    this.closeDrawer();
  }

  openTraceInWorkbench(traceId?: string | null): void {
    const matched = traceId ? this.snapshot.traces.find(tr => tr.id === traceId) : this.snapshot.traces[0];
    const nodeId = matched?.scope || matched?.role || traceId || 'trace';
    this.drillIntoNode(nodeId, {
      traceId: traceId ?? matched?.id ?? null,
      sessionKey: matched?.key ?? null,
    });
  }

  setExecutionView(mode: ExecutionViewMode): void {
    this.setState({ executionView: mode });
  }

  setDesignView(mode: DesignViewMode): void {
    this.setState({ designView: mode });
  }

  setCreateModalOpen(open: boolean): void {
    this.setState({ createModalOpen: open, error: open ? null : this.snapshot.error });
  }

  clearError(): void {
    this.setState({ error: null });
  }

  clearNotice(): void {
    this.setState({ notice: null });
  }

  setInspectorTab(tab: InspectorTab): void {
    this.setState({ inspectorTab: tab });
    if (tab === 'output' && this.snapshot.selectedTaskDir) {
      void this.loadTraces(true);
    }
  }

  selectPacket(packetId: string | null): void {
    this.setState({ selectedPacketId: packetId, inspectorTab: 'overview', nodeDetail: null });
    if (packetId && this.snapshot.selectedTaskDir) {
      void this.loadNodeDetail(packetId);
      void this.loadUXReview(packetId);
      void this.loadTraces(true);
    }
  }

  async refreshRuns(silent = false): Promise<RunSummary[]> {
    if (!silent) this.setState({ runsLoading: true, runsError: null });
    try {
      const res = await this.remote.query({
        action: 'runs',
        ...(this.snapshot.selectedCwd ? { cwd: this.snapshot.selectedCwd } : {}),
      });
      const normalizedRuns = normalizeRuns(Array.isArray(res.runs) ? res.runs : []);
      const runs = reconcileSelectedRun(
        normalizedRuns,
        this.snapshot.selectedTaskDir,
        this.snapshot.running,
        this.snapshot.state,
        this.snapshot.planning,
      );
      const preferredRun = runs.find(r => !r.isTemp) ?? runs[0];
      const nextCwd = res.cwd || this.snapshot.selectedCwd || preferredRun?.cwd || '';
      const currentSelected = this.snapshot.selectedTaskDir;
      const stillPresent = currentSelected && runs.some(r => r.taskDir === currentSelected);
      const nextSelected = stillPresent ? currentSelected : (preferredRun?.taskDir ?? currentSelected);

      this.setState({
        runs,
        runsLoading: false,
        runsError: null,
        selectedCwd: nextCwd,
        lastUpdatedAt: Date.now(),
      });
      if (nextSelected && nextSelected !== currentSelected) {
        await this.selectTask(nextSelected, runs.find(r => r.taskDir === nextSelected)?.cwd ?? nextCwd);
      }
      return runs;
    } catch (error) {
      this.setState({
        runsLoading: false,
        runsError: error instanceof Error ? error.message : String(error),
      });
      return this.snapshot.runs;
    }
  }

  async selectTask(taskDir: string, cwd?: string): Promise<void> {
    this.liveStreamGen += 1;
    const resolvedCwd = cwd || this.snapshot.runs.find(r => r.taskDir === taskDir)?.cwd || this.snapshot.selectedCwd;
    this.setState({
      selectedTaskDir: taskDir,
      selectedCwd: resolvedCwd,
      workbenchView: 'main',
      activeNodeId: null,
      activeSessionKey: null,
      activeDrawer: null,
      selectedPacketId: null,
      nodeDetail: null,
      designCandidates: [],
      uiPrototypeHtml: '',
      uiPrototypeFile: '',
      delivery: null,
      traces: [],
      selectedTraceId: null,
      traceDetail: null,
      sessionHistories: {},
      error: null,
    });
    await this.refreshSelectedTask(false, true);
    void this.loadTraces(true);
  }

  async refreshSelectedTask(silent = false, autoSelectTab = false): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return undefined;
    const seq = ++this.taskRequestSeq;
    if (!silent) this.setState({ taskLoading: true });
    try {
      const res = await this.remote.query({
        action: 'status',
        taskDir,
        ...(this.snapshot.selectedCwd ? { cwd: this.snapshot.selectedCwd } : {}),
      });
      if (seq !== this.taskRequestSeq) return undefined;
      const state = res.state ?? null;
      const planning = res.planning ?? state?.planning ?? null;
      const running = Boolean(res.running);
      const resolvedPlanning = resolvePlanningView(planning ?? state?.planning ?? null);
      const nextTab = autoSelectTab ? inferDefaultTab(state, planning) : this.snapshot.activeTab;
      const runs = reconcileSelectedRun(this.snapshot.runs, taskDir, running, state, planning);
      this.setState({
        runs,
        taskLoading: false,
        running,
        state,
        planning,
        selectedCwd: res.cwd || this.snapshot.selectedCwd,
        activeTab: nextTab,
        lastUpdatedAt: Date.now(),
      });
      if (nextTab === 'design' || resolvedPlanning?.design || resolvedPlanning?.phase === 'awaiting-input' || resolvedPlanning?.isBlocked) {
        void this.loadDesignCandidates();
        void this.loadTraces(true);
      }
      const protoFile = (planning ?? state?.planning)?.['ui-prototype']?.['html-file'];
      if (protoFile && protoFile !== this.snapshot.uiPrototypeFile) {
        void this.loadUIPrototype(protoFile);
      }
      if (nextTab === 'delivery') {
        void this.loadDelivery();
      }
      if (nextTab === 'traces') {
        void this.loadTraces(silent);
      }
      return res;
    } catch (error) {
      if (seq !== this.taskRequestSeq) return undefined;
      this.setState({
        taskLoading: false,
        error: this.toStoreError(error, 'status'),
      });
      return undefined;
    }
  }

  async loadUIPrototype(htmlFile?: string): Promise<void> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return;
    try {
      const res = await this.remote.query({
        action: 'ui-prototype',
        taskDir,
        ...(htmlFile ? { htmlFile } : {}),
      });
      if (this.snapshot.selectedTaskDir !== taskDir) return;
      this.setState({
        uiPrototypeHtml: typeof res.html === 'string' ? res.html : '',
        uiPrototypeFile: typeof res.htmlFile === 'string' ? res.htmlFile : (htmlFile ?? ''),
      });
    } catch {
      // Optional artifact.
    }
  }

  async loadDesignCandidates(): Promise<void> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return;
    try {
      const res = await this.remote.query({ action: 'design-candidates', taskDir });
      if (this.snapshot.selectedTaskDir !== taskDir) return;
      this.setState({
        designCandidates: Array.isArray(res.revisions) ? res.revisions : [],
      });
    } catch {
      // Optional artifact directory.
    }
  }

  async loadNodeDetail(packetId: string): Promise<void> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return;
    try {
      const res = await this.remote.query({
        action: 'node',
        taskDir,
        packet: packetId,
        ...(this.snapshot.selectedCwd ? { cwd: this.snapshot.selectedCwd } : {}),
      });
      if (this.snapshot.selectedPacketId !== packetId) return;
      if (res.node) {
        this.setState({ nodeDetail: res.node });
      }
    } catch (error) {
      this.setState({ error: this.toStoreError(error, 'node') });
    }
  }

  async loadUXReview(packetId: string): Promise<void> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return;
    try {
      const res = await this.remote.query({ action: 'ux-review', taskDir, packet: packetId });
      if (this.snapshot.selectedTaskDir !== taskDir) return;
      this.setState({
        uxReviews: {
          ...this.snapshot.uxReviews,
          [packetId]: res.uxReview ?? null,
        },
      });
    } catch {
      // Optional UX review report.
    }
  }

  async loadDelivery(): Promise<void> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return;
    try {
      const res = await this.remote.query({
        action: 'delivery',
        taskDir,
        ...(this.snapshot.selectedCwd ? { cwd: this.snapshot.selectedCwd } : {}),
        delivery: { action: 'inspect' },
      });
      if (this.snapshot.selectedTaskDir !== taskDir) return;
      if (res.delivery) {
        this.setState({ delivery: res.delivery });
      }
    } catch (error) {
      this.setState({ error: this.toStoreError(error, 'delivery') });
    }
  }

  async loadRepoImages(cwd?: string): Promise<RepoImageEntry[]> {
    try {
      const res = await this.remote.listRepoImages(cwd || this.snapshot.selectedCwd || undefined);
      const files = Array.isArray(res.files) ? res.files : [];
      this.setState({ repoImages: files });
      return files;
    } catch (error) {
      this.setState({ error: this.toStoreError(error, 'images') });
      return [];
    }
  }

  async loadConfig(scope: 'global' | 'project' = this.snapshot.configScope): Promise<void> {
    this.setState({ configScope: scope, error: null });
    try {
      const cwdArg = this.snapshot.selectedCwd ? { cwd: this.snapshot.selectedCwd } : {};
      const taskArg = this.snapshot.selectedTaskDir ? { taskDir: this.snapshot.selectedTaskDir } : {};
      const [editRes, effectiveRes] = await Promise.all([
        this.remote.query({ action: 'config-edit', ...cwdArg, editor: { scope } }),
        this.remote.query({ action: 'config', ...cwdArg, ...taskArg }),
      ]);
      this.setState({
        configEditor: editRes.editor ?? null,
        effectiveConfig: effectiveRes.configuration ?? null,
      });
    } catch (error) {
      this.setState({ error: this.toStoreError(error, 'config-edit') });
    }
  }

  async loadDoctor(probe = false): Promise<DoctorReport | null> {
    this.setState({ busyAction: 'doctor', error: null });
    try {
      const res = await this.remote.query({
        action: 'doctor',
        probe,
        ...(this.snapshot.selectedCwd ? { cwd: this.snapshot.selectedCwd } : {}),
      });
      const report = (res.doctor ?? res.diagnostic ?? null) as DoctorReport | null;
      this.setState({ doctor: report, busyAction: null });
      return report;
    } catch (error) {
      this.setState({ busyAction: null, error: this.toStoreError(error, 'doctor') });
      return null;
    }
  }

  async loadModelCatalog(
    request: { version: number; harness: string; channel?: string; provider?: string },
    options: { silent?: boolean } = {},
  ): Promise<ModelCatalogReport | undefined> {
    try {
      const res = await this.remote.query({
        action: 'models',
        ...(this.snapshot.selectedCwd ? { cwd: this.snapshot.selectedCwd } : {}),
        models: request,
      });
      if (res.modelCatalog) {
        const key = `${request.harness}:${request.channel ?? request.provider ?? 'default'}`;
        this.setState({
          modelCatalogs: { ...this.snapshot.modelCatalogs, [key]: res.modelCatalog },
        });
        return res.modelCatalog;
      }
      return undefined;
    } catch (error) {
      if (!options.silent) {
        this.setState({ error: this.toStoreError(error, 'models') });
      }
      return undefined;
    }
  }

  async performAct(label: string, payload: Record<string, unknown>, refreshAfter = true): Promise<ControlResponse | undefined> {
    this.setState({ busyAction: label, error: null, notice: null });
    try {
      const res = await this.remote.act(payload);
      this.setState({
        busyAction: null,
        ...(res.running !== undefined ? { running: Boolean(res.running) } : {}),
        ...(res.state ? { state: res.state } : {}),
        ...(res.planning ? { planning: res.planning } : {}),
        ...(res.delivery ? { delivery: res.delivery } : {}),
        ...(res.editor ? { configEditor: res.editor } : {}),
      });
      if (refreshAfter) {
        await this.refreshRuns(true);
        if (res.taskDir && res.taskDir !== this.snapshot.selectedTaskDir) {
          await this.selectTask(res.taskDir, res.cwd || this.snapshot.selectedCwd);
        } else if (this.snapshot.selectedTaskDir) {
          await this.refreshSelectedTask(true, false);
          if (this.snapshot.activeTab === 'delivery') {
            await this.loadDelivery();
          }
        }
        this.schedulePoll(1500);
      }
      return res;
    } catch (error) {
      this.setState({ busyAction: null, error: this.toStoreError(error, String(payload.action ?? label)) });
      return undefined;
    }
  }

  async createBuildTask(opts: {
    cwd: string;
    slug: string;
    goal: string;
    mode: 'auto' | 'quick' | 'full';
    clarifyMode?: 'off' | 'auto' | 'always';
    uiPrototypeMode?: 'off' | 'auto' | 'always';
    qualityGates?: string[];
    images?: string[];
  }): Promise<ControlResponse | undefined> {
    const cleanCwd = opts.cwd.replace(/\/+$/, '');
    const safeSlug =
      opts.slug
        .trim()
        .replace(/[^A-Za-z0-9_.-]+/g, '-')
        .replace(/^[^A-Za-z0-9]+|-+$/g, '')
        .slice(0, 120) || `task-${Date.now()}`;
    const taskDir = `${cleanCwd}/docs/${safeSlug}`;
    const requestId = `prepare-${Date.now()}`;
    const policy = opts.mode === 'quick' ? 'delegated' : 'review-first';
    const trimmedGoal = opts.goal.trim();
    const constraints = [`Task intent: ${opts.mode === 'quick' ? 'quick' : 'engineering'}`];
    const evidence: string[] = [];
    if (opts.images && opts.images.length > 0) {
      constraints.push(
        'Carry the explicitly selected input-images references into the relevant reviewed packet. Original images require the supported Claude SDK transport. Do not claim planning or review consumed these images from this metadata.',
      );
      evidence.push(
        JSON.stringify({
          'selected-input-images': opts.images,
          status: 'unreviewed-selection',
          'planning-consumption': 'paths-and-digests-only',
        }),
      );
    }
    const brief = {
      version: 1,
      id: safeSlug,
      goal: trimmedGoal,
      'original-request': trimmedGoal,
      constraints,
      evidence,
      policy,
      mode: opts.mode,
      ...(opts.clarifyMode ? { 'clarify-mode': opts.clarifyMode } : {}),
      ...(opts.uiPrototypeMode ? { 'ui-prototype-mode': opts.uiPrototypeMode } : {}),
      ...(opts.qualityGates && opts.qualityGates.length > 0 ? { 'quality-gates': opts.qualityGates } : {}),
      'implementation-authorized': policy === 'delegated',
      ...(policy === 'delegated'
        ? {
            authorization: `User selected Quick / delegated execution in the Flywheel Create Task form. Goal: ${trimmedGoal}`,
          }
        : {}),
    };
    return this.performAct('create-build', {
      action: 'planning',
      cwd: cleanCwd,
      taskDir,
      planning: {
        action: 'prepare',
        'request-id': requestId,
        brief,
        start: true,
      },
    });
  }

  async submitClarification(payload: {
    answers: Record<string, string>;
    selectedGates: string[];
    freeformNote?: string;
    uiPrototypeMode?: 'off' | 'auto' | 'always';
  }): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return undefined;
    const rev = this.snapshot.state?.revision ?? this.snapshot.planning?.revision ?? 1;
    return this.performAct('clarify', {
      action: 'planning',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      planning: {
        action: 'clarify',
        'request-id': `clarify-${Date.now()}`,
        revision: rev,
        clarify: {
          answers: payload.answers,
          'selected-gates': payload.selectedGates,
          ...(payload.freeformNote?.trim() ? { 'freeform-note': payload.freeformNote.trim() } : {}),
          ...(payload.uiPrototypeMode ? { 'ui-prototype-mode': payload.uiPrototypeMode } : {}),
        },
        start: true,
      },
    });
  }

  async approveUIPrototype(): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return undefined;
    const rev = this.snapshot.state?.revision ?? this.snapshot.planning?.revision ?? 1;
    return this.performAct('approve-prototype', {
      action: 'planning',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      planning: {
        action: 'approve-prototype',
        'request-id': `approve-proto-${Date.now()}`,
        revision: rev,
        start: true,
      },
    });
  }

  async submitUIPrototypeFeedback(text: string): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir || !text.trim()) return undefined;
    const rev = this.snapshot.state?.revision ?? this.snapshot.planning?.revision ?? 1;
    return this.performAct('prototype-feedback', {
      action: 'planning',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      planning: {
        action: 'prototype-feedback',
        'request-id': `proto-fb-${Date.now()}`,
        revision: rev,
        text: text.trim(),
        start: true,
      },
    });
  }

  async skipUIPrototype(): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return undefined;
    const rev = this.snapshot.state?.revision ?? this.snapshot.planning?.revision ?? 1;
    return this.performAct('skip-prototype', {
      action: 'planning',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      planning: {
        action: 'skip-prototype',
        'request-id': `skip-proto-${Date.now()}`,
        revision: rev,
        start: true,
      },
    });
  }

  async createInvestigationTask(opts: {
    cwd: string;
    slug: string;
    goal: string;
    readPaths?: string[];
  }): Promise<ControlResponse | undefined> {
    const cleanCwd = opts.cwd.replace(/\/+$/, '');
    const safeSlug =
      opts.slug
        .trim()
        .replace(/[^A-Za-z0-9_.-]+/g, '-')
        .replace(/^[^A-Za-z0-9]+|-+$/g, '')
        .slice(0, 120) || `investigate-${Date.now()}`;
    const taskDir = `${cleanCwd}/docs/${safeSlug}`;
    return this.performAct('create-investigate', {
      action: 'investigate',
      cwd: cleanCwd,
      taskDir,
      investigation: {
        action: 'start',
        goal: opts.goal.trim(),
        intent: 'investigate',
        run: true,
        ...(opts.readPaths && opts.readPaths.length > 0 ? { 'read-paths': opts.readPaths } : {}),
      },
    });
  }

  async approveDesign(startExecution: boolean): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const resolved = resolvePlanningView(this.snapshot.planning ?? this.snapshot.state?.planning ?? null);
    const design = resolved?.design;
    const rev = this.snapshot.state?.revision ?? this.snapshot.planning?.revision ?? 1;
    if (!taskDir || !design?.revision || !design?.digest) return undefined;
    return this.performAct('approve-design', {
      action: 'planning',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      planning: {
        action: 'approve',
        'request-id': `approve-${Date.now()}`,
        revision: rev,
        design: { revision: design.revision, digest: design.digest },
        start: startExecution,
      },
    });
  }

  async retryPlanning(opts?: { draftMode?: 'dual' | 'single' }): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return undefined;
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
      await this.refreshSelectedTask(true, false);
    }
    const rev = this.snapshot.state?.revision ?? this.snapshot.planning?.revision ?? 1;
    return this.performAct('retry-planning', {
      action: 'planning',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      planning: {
        action: 'plan',
        'request-id': `plan-retry-${Date.now()}`,
        revision: rev,
        scope: 'design',
        ...(opts?.draftMode ? { 'draft-mode': opts.draftMode } : {}),
        start: true,
      },
    });
  }

  async uploadAttachment(
    cwd: string,
    file: { name: string; mimeType: string; dataBase64: string },
  ): Promise<{ path: string; bytes: number; mimeType: string } | null> {
    if (!cwd || !file.dataBase64) return null;
    try {
      const res = await this.remote.act({
        action: 'save-attachment',
        cwd,
        name: file.name,
        mimeType: file.mimeType,
        dataBase64: file.dataBase64,
      });
      if (typeof res.path === 'string' && res.path) {
        return {
          path: res.path,
          bytes: typeof res.bytes === 'number' ? res.bytes : 0,
          mimeType: typeof res.mimeType === 'string' ? res.mimeType : file.mimeType,
        };
      }
      return null;
    } catch (error) {
      this.setState({ error: this.toStoreError(error, 'save-attachment') });
      return null;
    }
  }

  async submitDesignFeedback(text: string): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir || !text.trim()) return undefined;
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
      await this.refreshSelectedTask(true, false);
    }
    const rev = this.snapshot.state?.revision ?? this.snapshot.planning?.revision ?? 1;
    return this.performAct('design-feedback', {
      action: 'planning',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      planning: {
        action: 'feedback',
        'request-id': `feedback-${Date.now()}`,
        revision: rev,
        text: text.trim(),
        start: true,
      },
    });
  }

  async startRun(mode: 'auto' | 'quick' | 'full' = 'auto'): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return undefined;
    return this.performAct('start', {
      action: 'start',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      mode,
    });
  }

  async stopRun(): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return undefined;
    return this.performAct('stop', {
      action: 'stop',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
    });
  }

  async stepPacket(only: 'execute' | 'review' | 'verify', packetId: string, retryReason?: string): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const revision = this.snapshot.state?.revision ?? 1;
    if (!taskDir) return undefined;
    return this.performAct(`step-${only}`, {
      action: 'step',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      only,
      packet: packetId,
      revision,
      ...(retryReason ? { retryReason } : {}),
    });
  }

  async loadSessionHistory(sessionKey: string): Promise<SessionHistoryTurn[]> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir || !sessionKey) return [];
    try {
      const res = await this.remote.query({
        action: 'history',
        taskDir,
        sessionKey,
        ...(this.snapshot.selectedCwd ? { cwd: this.snapshot.selectedCwd } : {}),
      });
      if (this.snapshot.selectedTaskDir !== taskDir) return [];
      const raw = res.history as unknown;
      let turns: SessionHistoryTurn[] = [];
      if (Array.isArray(raw)) {
        turns = raw as SessionHistoryTurn[];
      } else if (raw && typeof raw === 'object' && Array.isArray((raw as { turns?: unknown[] }).turns)) {
        turns = (raw as { turns: SessionHistoryTurn[] }).turns;
      } else if (raw && typeof raw === 'object' && Array.isArray((raw as { entries?: unknown[] }).entries)) {
        turns = (raw as { entries: SessionHistoryTurn[] }).entries;
      }
      this.setState({
        sessionHistories: {
          ...this.snapshot.sessionHistories,
          [sessionKey]: turns,
        },
      });
      return turns;
    } catch {
      return this.snapshot.sessionHistories[sessionKey] ?? [];
    }
  }

  async claimSession(key: string): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const revision = this.snapshot.state?.revision ?? 1;
    if (!taskDir || !key) return undefined;
    const res = await this.performAct('claim-session', {
      action: 'session',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      session: { action: 'claim', key, revision },
    });
    if (res) {
      this.setState({
        claimedSessions: { ...this.snapshot.claimedSessions, [key]: true },
      });
    }
    return res;
  }

  async sendSessionMessage(key: string, prompt: string): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const clean = prompt.trim();
    if (!taskDir || !key || !clean) return undefined;
    if (this.snapshot.running) {
      const stopped = await this.stopRun();
      if (!stopped) return undefined;
    }
    if (!this.snapshot.claimedSessions[key]) {
      const claimed = await this.claimSession(key);
      if (!claimed) return undefined;
    }
    const revision = this.snapshot.state?.revision ?? 1;
    const res = await this.performAct('send-session', {
      action: 'session',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      session: { action: 'send', key, revision, prompt: clean },
    });
    if (res) {
      await this.loadSessionHistory(key);
      await this.loadTraces(true);
      if (this.snapshot.selectedTraceId) {
        void this.loadTraceDetail(this.snapshot.selectedTraceId, true);
      }
    }
    return res;
  }

  async returnSession(key: string): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const revision = this.snapshot.state?.revision ?? 1;
    if (!taskDir || !key) return undefined;
    const res = await this.performAct('return-session', {
      action: 'session',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      session: { action: 'return', key, revision },
    });
    if (res) {
      const next = { ...this.snapshot.claimedSessions };
      delete next[key];
      this.setState({ claimedSessions: next });
    }
    return res;
  }

  async abandonSession(key: string, invocation: string, reason: string): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return undefined;
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
      await this.refreshSelectedTask(true, false);
    }
    const revision = this.snapshot.state?.revision ?? 1;
    return this.performAct('abandon-session', {
      action: 'session',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      session: {
        action: 'abandon',
        key,
        invocation,
        revision,
        reason,
      },
    });
  }

  async triagePacket(
    action: 'retry' | 'replan' | 'accept',
    packetId: string,
    reason: string,
    guidance?: string,
  ): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir || !packetId || !reason.trim()) return undefined;
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
      await this.refreshSelectedTask(true, false);
    }
    const revision = this.snapshot.state?.revision ?? 1;
    const trimmedGuidance = guidance?.trim() ?? '';
    return this.performAct(`packet-${action}`, {
      action: 'packet',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      packet: {
        action,
        'packet-id': packetId,
        revision,
        reason: reason.trim(),
        ...(trimmedGuidance ? { guidance: trimmedGuidance } : {}),
      },
    });
  }

  async triageMilestone(
    action: 'retry' | 'accept',
    milestoneId: string,
    reason: string,
    guidance?: string,
  ): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir || !milestoneId || !reason.trim()) return undefined;
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
      await this.refreshSelectedTask(true, false);
    }
    const revision = this.snapshot.state?.revision ?? 1;
    const trimmedGuidance = guidance?.trim() ?? '';
    return this.performAct(`milestone-${action}`, {
      action: 'milestone',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      milestone: {
        action,
        'milestone-id': milestoneId,
        revision,
        reason: reason.trim(),
        ...(trimmedGuidance ? { guidance: trimmedGuidance } : {}),
      },
    });
  }

  async runInvestigation(): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const revision = this.snapshot.state?.investigation?.revision ?? this.snapshot.state?.revision ?? 1;
    if (!taskDir) return undefined;
    return this.performAct('inv-run', {
      action: 'investigate',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      investigation: { action: 'run', revision },
    });
  }

  async continueInvestigation(goal: string, readPaths: string[]): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const existing = this.snapshot.state?.investigation;
    const revision = existing?.revision ?? this.snapshot.state?.revision ?? 1;
    if (!taskDir || !goal.trim()) return undefined;
    const contract = {
      ...(existing?.contract ?? {}),
      version: 1,
      intent: 'investigate',
      goal: goal.trim(),
      ...(readPaths.length > 0 ? { 'read-paths': readPaths } : {}),
    };
    return this.performAct('inv-continue', {
      action: 'investigate',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      investigation: { action: 'continue', revision, contract },
    });
  }

  async cancelInvestigation(): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const revision = this.snapshot.state?.investigation?.revision ?? this.snapshot.state?.revision ?? 1;
    if (!taskDir) return undefined;
    return this.performAct('inv-cancel', {
      action: 'investigate',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      investigation: { action: 'cancel', revision },
    });
  }

  async createPullRequest(draft: boolean): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const delivery = this.snapshot.delivery;
    const revision = this.snapshot.state?.revision ?? 1;
    if (!taskDir) return undefined;
    return this.performAct('deliver-create', {
      action: 'deliver',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      delivery: {
        action: 'create',
        confirmed: true,
        draft,
        revision,
        ...(delivery?.head ? { expectedHead: delivery.head } : {}),
        ...(delivery?.repo ? { expectedRepo: delivery.repo } : {}),
      },
    });
  }

  async mergePullRequest(prNumber: number): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const delivery = this.snapshot.delivery;
    const revision = this.snapshot.state?.revision ?? 1;
    if (!taskDir) return undefined;
    return this.performAct('deliver-merge', {
      action: 'deliver',
      cwd: this.snapshot.selectedCwd || undefined,
      taskDir,
      delivery: {
        action: 'merge',
        confirmed: true,
        number: prNumber,
        revision,
        ...(delivery?.head ? { expectedHead: delivery.head } : {}),
        ...(delivery?.repo ? { expectedRepo: delivery.repo } : {}),
      },
    });
  }

  async saveConfig(values: Record<string, unknown>): Promise<ControlResponse | undefined> {
    const editor = this.snapshot.configEditor;
    if (!editor) return undefined;
    const res = await this.performAct(
      'config-save',
      {
        action: 'config-save',
        ...(this.snapshot.selectedCwd ? { cwd: this.snapshot.selectedCwd } : {}),
        editor: {
          scope: editor.scope,
          revision: editor.revision,
          values,
        },
      },
      false,
    );
    if (res) {
      this.setState({ notice: 'saved' });
      await this.loadConfig(editor.scope);
    }
    return res;
  }

  async resetConfig(): Promise<ControlResponse | undefined> {
    const editor = this.snapshot.configEditor;
    if (!editor) return undefined;
    const res = await this.performAct(
      'config-save',
      {
        action: 'config-save',
        ...(this.snapshot.selectedCwd ? { cwd: this.snapshot.selectedCwd } : {}),
        editor: {
          scope: editor.scope,
          revision: editor.revision,
          reset: true,
          values: {},
        },
      },
      false,
    );
    if (res) {
      this.setState({ notice: 'reset' });
      await this.loadConfig(editor.scope);
    }
    return res;
  }

  async loadTraces(silent = false): Promise<TraceSummary[]> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir) return [];
    if (!silent) this.setState({ tracesLoading: true });
    try {
      const res = await this.remote.query({ action: 'traces', taskDir });
      if (this.snapshot.selectedTaskDir !== taskDir) return [];
      const traces = Array.isArray(res.traces) ? res.traces : [];
      const current = this.snapshot.selectedTraceId;
      const stillExists = current && traces.some(tr => tr.id === current);
      const nextId = stillExists ? current : (traces[0]?.id ?? null);
      this.setState({
        traces,
        tracesLoading: false,
        selectedTraceId: nextId,
      });
      if (nextId && (!this.snapshot.traceDetail || this.snapshot.traceDetail.id !== nextId || traces.find(tr => tr.id === nextId)?.running)) {
        void this.loadTraceDetail(nextId, silent);
      }
      return traces;
    } catch {
      if (this.snapshot.selectedTaskDir === taskDir) {
        this.setState({ tracesLoading: false });
      }
      return this.snapshot.traces;
    }
  }

  selectTrace(traceId: string | null): void {
    this.setState({ selectedTraceId: traceId });
    if (traceId && this.snapshot.selectedTaskDir) {
      void this.loadTraceDetail(traceId);
    }
  }

  async loadTraceDetail(traceId: string, silent = false): Promise<TraceDetail | null> {
    const taskDir = this.snapshot.selectedTaskDir;
    if (!taskDir || !traceId) return null;
    try {
      const res = await this.remote.query({ action: 'trace-detail', taskDir, traceId });
      if (this.snapshot.selectedTaskDir !== taskDir) return null;
      const trace = (res.trace ?? null) as TraceDetail | null;
      if (trace && (this.snapshot.selectedTraceId === traceId || !this.snapshot.selectedTraceId)) {
        this.setState({ traceDetail: trace });
      }
      return trace;
    } catch (error) {
      if (!silent) {
        this.setState({ error: this.toStoreError(error, 'trace-detail') });
      }
      return null;
    }
  }

  async openResumeTerminal(
    harness: string,
    sessionId: string,
    cwd?: string,
    sessionTitle?: string,
    terminal?: 'warp' | 'terminal',
  ): Promise<ControlResponse | undefined> {
    const res = await this.performAct(
      'open-terminal',
      {
        action: 'open-terminal',
        harness,
        sessionId,
        ...(cwd ? { cwd } : {}),
        ...(sessionTitle ? { sessionTitle } : {}),
        ...(terminal ? { terminal } : {}),
      },
      false,
    );
    if (res) {
      this.setState({ notice: res.terminal === 'warp' ? 'warp-opened' : 'terminal-opened' });
    }
    return res;
  }

  openInlineHtmlPreview(title: string, relPath: string, html: string, fullscreen = false): void {
    const taskDir = this.snapshot.selectedTaskDir || '';
    const absPath = taskDir && relPath ? `${taskDir.replace(/\/+$/, '')}/${relPath.replace(/^\/+/, '')}` : '';
    this.setState({
      activeDrawer: null,
      previewArtifact: {
        title: title || relPath || 'HTML Preview',
        relPath,
        absPath,
        kind: 'html',
        content: html,
        loading: false,
        fullscreen,
        error: null,
      },
    });
  }

  async openArtifactPreview(relPath: string, opts?: { title?: string; fullscreen?: boolean }): Promise<void> {
    const taskDir = this.snapshot.selectedTaskDir;
    const cwd = this.snapshot.selectedCwd;
    if (!relPath || (!taskDir && !cwd)) return;
    const title = opts?.title || basenameOf(relPath) || relPath;
    this.setState({
      activeDrawer: null,
      previewArtifact: {
        title,
        relPath,
        absPath: '',
        kind: relPath.endsWith('.html') ? 'html' : relPath.endsWith('.md') ? 'markdown' : 'text',
        content: '',
        loading: true,
        fullscreen: Boolean(opts?.fullscreen),
        error: null,
      },
    });
    try {
      const res = await this.remote.query({
        action: 'read-artifact',
        ...(taskDir ? { taskDir } : {}),
        ...(cwd ? { cwd } : {}),
        relPath,
      });
      const content = typeof res.content === 'string' ? res.content : '';
      const kind = (['html', 'markdown', 'json', 'code', 'text'].includes(String(res.kind))
        ? String(res.kind)
        : 'text') as PreviewArtifactState['kind'];
      this.setState({
        previewArtifact: {
          title,
          relPath: typeof res.relPath === 'string' ? res.relPath : relPath,
          absPath: typeof res.absPath === 'string' ? res.absPath : '',
          kind,
          content,
          loading: false,
          fullscreen: Boolean(this.snapshot.previewArtifact?.fullscreen ?? opts?.fullscreen),
          error: null,
        },
      });
    } catch (error) {
      this.setState({
        previewArtifact: {
          title,
          relPath,
          absPath: '',
          kind: 'text',
          content: '',
          loading: false,
          fullscreen: Boolean(this.snapshot.previewArtifact?.fullscreen ?? opts?.fullscreen),
          error: this.toStoreError(error, 'read-artifact').message,
        },
      });
    }
  }

  togglePreviewFullscreen(): void {
    const cur = this.snapshot.previewArtifact;
    if (!cur) return;
    this.setState({ previewArtifact: { ...cur, fullscreen: !cur.fullscreen } });
  }

  closeArtifactPreview(): void {
    if (this.snapshot.previewArtifact) {
      this.setState({ previewArtifact: null });
    }
  }

  async openArtifactExternal(relPath: string): Promise<void> {
    const taskDir = this.snapshot.selectedTaskDir;
    const cwd = this.snapshot.selectedCwd;
    if (!relPath || (!taskDir && !cwd)) return;
    await this.performAct(
      'open-artifact',
      {
        action: 'open-artifact',
        ...(taskDir ? { taskDir } : {}),
        ...(cwd ? { cwd } : {}),
        relPath,
      },
      false,
    );
  }
}
