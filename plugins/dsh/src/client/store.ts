import type { ClientContext } from './context.ts';
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
  TaskState,
  UXReviewReport,
} from './types.ts';

export interface StoreError {
  message: string;
  dispatch?: string;
  action?: string | null;
}

export interface FlywheelSnapshot {
  runs: RunSummary[];
  runsLoading: boolean;
  runsError: string | null;
  selectedTaskDir: string | null;
  selectedCwd: string;
  railCollapsed: boolean;
  activeTab: MainTab;
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
  nodeDetail: NodeDetailSnapshot | null;
  uxReviews: Record<string, UXReviewReport | null>;
  delivery: DeliverySnapshot | null;

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
    activeTab: 'execution',
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
    nodeDetail: null,
    uxReviews: {},
    delivery: null,

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

function inferDefaultTab(state: TaskState | null, planning: PlanningState | null): MainTab {
  if (!state) return 'execution';
  if (state.investigation && (state.investigation.status || state.investigation.goal)) {
    const packets = state.packets ? Object.keys(state.packets).length : 0;
    if (packets === 0) return 'investigation';
  }
  const planStatus = planning?.status ?? state.planning?.status ?? '';
  const approved = planning?.design?.approved ?? state.planning?.design?.approved ?? false;
  if (planStatus && !approved && ['drafting', 'reviewing', 'awaiting_approval', 'needs_revision', 'ready'].includes(planStatus)) {
    return 'design';
  }
  const st = state.status ?? '';
  if (st === 'completed' || st === 'done') return 'delivery';
  return 'execution';
}

export class FlywheelStore {
  readonly remote: FlywheelRemoteClient;
  private snapshot: FlywheelSnapshot = initialSnapshot();
  private readonly listeners = new Set<() => void>();
  private pollTimer: ReturnType<typeof setTimeout> | null = null;
  private started = false;
  private disposed = false;
  private taskRequestSeq = 0;
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
    const delay = overrideMs ?? (this.snapshot.running ? 2500 : 12000);
    this.pollTimer = setTimeout(async () => {
      this.pollTimer = null;
      if (!this.started || this.disposed) return;
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;
      await this.refreshRuns(true);
      if (this.snapshot.selectedTaskDir) {
        await this.refreshSelectedTask(true);
      }
      this.schedulePoll();
    }, delay);
  }

  setRailCollapsed(collapsed: boolean): void {
    this.setState({ railCollapsed: collapsed });
  }

  setActiveTab(tab: MainTab): void {
    this.setState({ activeTab: tab });
    if (tab === 'delivery' && this.snapshot.selectedTaskDir) {
      void this.loadDelivery();
    }
    if (tab === 'design' && this.snapshot.selectedTaskDir) {
      void this.loadDesignCandidates();
    }
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
  }

  selectPacket(packetId: string | null): void {
    this.setState({ selectedPacketId: packetId, inspectorTab: 'overview', nodeDetail: null });
    if (packetId && this.snapshot.selectedTaskDir) {
      void this.loadNodeDetail(packetId);
      void this.loadUXReview(packetId);
    }
  }

  async refreshRuns(silent = false): Promise<RunSummary[]> {
    if (!silent) this.setState({ runsLoading: true, runsError: null });
    try {
      const res = await this.remote.query({
        action: 'runs',
        ...(this.snapshot.selectedCwd ? { cwd: this.snapshot.selectedCwd } : {}),
      });
      const runs = Array.isArray(res.runs) ? res.runs : [];
      const nextCwd = res.cwd || this.snapshot.selectedCwd || runs[0]?.cwd || '';
      const currentSelected = this.snapshot.selectedTaskDir;
      const stillPresent = currentSelected && runs.some(r => r.taskDir === currentSelected);
      const nextSelected = stillPresent ? currentSelected : (runs[0]?.taskDir ?? currentSelected);

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
    const resolvedCwd = cwd || this.snapshot.runs.find(r => r.taskDir === taskDir)?.cwd || this.snapshot.selectedCwd;
    this.setState({
      selectedTaskDir: taskDir,
      selectedCwd: resolvedCwd,
      selectedPacketId: null,
      nodeDetail: null,
      designCandidates: [],
      delivery: null,
      error: null,
    });
    await this.refreshSelectedTask(false, true);
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
      const nextTab = autoSelectTab ? inferDefaultTab(state, planning) : this.snapshot.activeTab;
      this.setState({
        taskLoading: false,
        running: Boolean(res.running),
        state,
        planning,
        selectedCwd: res.cwd || this.snapshot.selectedCwd,
        activeTab: nextTab,
        lastUpdatedAt: Date.now(),
      });
      if (nextTab === 'design' || planning?.design) {
        void this.loadDesignCandidates();
      }
      if (nextTab === 'delivery') {
        void this.loadDelivery();
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
      const [editRes, effectiveRes] = await Promise.all([
        this.remote.query({ action: 'config-edit', ...cwdArg, editor: { scope } }),
        this.remote.query({ action: 'config', ...cwdArg }),
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

  async loadModelCatalog(request: { version: number; harness: string; channel?: string; provider?: string }): Promise<ModelCatalogReport | undefined> {
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
      this.setState({ error: this.toStoreError(error, 'models') });
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
    images?: string[];
  }): Promise<ControlResponse | undefined> {
    const taskDir = `${opts.cwd.replace(/\/+$/, '')}/docs/${opts.slug}`;
    const requestId = `prepare-${Date.now()}`;
    return this.performAct('create-build', {
      action: 'planning',
      cwd: opts.cwd,
      taskDir,
      planning: {
        action: 'prepare',
        'request-id': requestId,
        policy: opts.mode === 'quick' ? 'delegated' : 'review-first',
        start: true,
        brief: {
          goal: opts.goal,
          mode: opts.mode,
          ...(opts.images && opts.images.length > 0 ? { images: opts.images } : {}),
        },
      },
    });
  }

  async createInvestigationTask(opts: {
    cwd: string;
    slug: string;
    goal: string;
    readPaths?: string[];
  }): Promise<ControlResponse | undefined> {
    const taskDir = `${opts.cwd.replace(/\/+$/, '')}/docs/${opts.slug}`;
    return this.performAct('create-investigate', {
      action: 'investigate',
      cwd: opts.cwd,
      taskDir,
      investigation: {
        action: 'start',
        goal: opts.goal,
        intent: 'investigate',
        run: true,
        ...(opts.readPaths && opts.readPaths.length > 0 ? { 'read-paths': opts.readPaths } : {}),
      },
    });
  }

  async approveDesign(startExecution: boolean): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const design = this.snapshot.planning?.design ?? this.snapshot.state?.planning?.design;
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

  async submitDesignFeedback(text: string): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const rev = this.snapshot.state?.revision ?? this.snapshot.planning?.revision ?? 1;
    if (!taskDir || !text.trim()) return undefined;
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

  async abandonSession(key: string, invocation: string, reason: string): Promise<ControlResponse | undefined> {
    const taskDir = this.snapshot.selectedTaskDir;
    const revision = this.snapshot.state?.revision ?? 1;
    if (!taskDir) return undefined;
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
      ...(readPaths.length > 0 ? { read_paths: readPaths } : {}),
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
}
