Object.assign(zh, {
  investigate: '调查', investigationTitle: '有界调查', investigationHelp: '由 Go 运行已批准的读取与搜索动作。调查结论独立于代码验收和交付。',
  investigationRun: '继续调查', investigationReconcile: '核对原动作回执', investigationReport: '调查报告', investigationEvidence: '已收集证据', investigationBudget: '动作 {used}/{limit} · 累计 {ms} ms', investigationPartial: '有界片段 · 内容未完整展示', investigationDecision: '决策建议与实际采用',
  investigationState_ready: '等待调查', investigationState_running: '调查中', 'investigationState_awaiting-report': '等待证据报告', investigationState_blocked: '结果未知 · 需核对', investigationState_supported: '报告有证据支持', investigationState_inconclusive: '尚无定论', 'investigationState_access-blocked': '缺少访问', investigationState_cancelled: '调查已取消',
  investigationRepair: '已授权原范围内修复；形成合法 Spec 后由 Auto 选择 Quick / Full。', investigationReportOnly: '本目标以调查报告结束；没有启动代码实现。',
  investigationJEVObserved: '最近观察到的 JEV 状态', investigationJEV_enabled: 'JEV 已开启 · 在符合条件的决策点参与', investigationJEV_disabled: 'JEV 已关闭 · 后续动作沿用原路径', 'investigationJEV_config-unavailable': 'JEV 配置不可用 · 沿用原路径', 'investigationJEV_not-configured': '本次调查未配置 JEV · 沿用原路径', 'investigationJEV_policy-changed': 'JEV 配置已变更 · 本次调查沿用原路径',
  investigationAdopted: 'JEV 已采用', investigationBaseline: '原路径', investigationEffect_defer: '依据证据跳过后续动作', investigationEffect_finish: '提前结束取证 · 等待报告', investigationSkipped: '跳过动作', investigationCitations: '依据观察',
});
Object.assign(en, {
  investigate: 'Investigate', investigationTitle: 'Bounded investigation', investigationHelp: 'Go runs approved reads and searches. An investigation conclusion is separate from code acceptance and delivery.',
  investigationRun: 'Continue investigation', investigationReconcile: 'Reconcile original receipt', investigationReport: 'Investigation report', investigationEvidence: 'Collected evidence', investigationBudget: 'Actions {used}/{limit} · elapsed {ms} ms', investigationPartial: 'Bounded excerpt · incomplete content', investigationDecision: 'Advice and recorded adoption',
  investigationState_ready: 'Ready to investigate', investigationState_running: 'Investigating', 'investigationState_awaiting-report': 'Awaiting evidence report', investigationState_blocked: 'Unknown result · reconcile', investigationState_supported: 'Report supported by cited evidence', investigationState_inconclusive: 'Inconclusive', 'investigationState_access-blocked': 'Access blocked', investigationState_cancelled: 'Investigation cancelled',
  investigationRepair: 'Repair is authorized within the original scope. Prepare a valid Spec before Auto selects Quick or Full.', investigationReportOnly: 'The goal ends with an investigation report; implementation has not started.',
  investigationJEVObserved: 'Last observed JEV state', investigationJEV_enabled: 'JEV enabled · participates at eligible decision points', investigationJEV_disabled: 'JEV disabled · subsequent actions use the original path', 'investigationJEV_config-unavailable': 'JEV configuration unavailable · original path', 'investigationJEV_not-configured': 'JEV not configured for this investigation · original path', 'investigationJEV_policy-changed': 'JEV policy changed · this investigation uses the original path',
  investigationAdopted: 'JEV adopted', investigationBaseline: 'Original path', investigationEffect_defer: 'Evidence-backed actions skipped', investigationEffect_finish: 'Evidence collection ended early · awaiting report', investigationSkipped: 'Skipped actions', investigationCitations: 'Cited observations',
});

function validPreparationSnapshot(p) {
  const strings = (object, keys) => record(object) && keys.every(key => typeof object[key] === 'string');
  const stringArray = value => Array.isArray(value) && value.every(item => typeof item === 'string');
  return record(p) && p.version === 1 && Number.isSafeInteger(p.revision) && p.revision > 0 &&
    ['ready', 'running', 'awaiting-report', 'blocked', 'supported', 'inconclusive', 'access-blocked', 'cancelled'].includes(p.status) &&
    typeof p.reason === 'string' &&
    strings(p.contract, ['id', 'goal', 'intent']) && ['investigate', 'investigate-and-fix'].includes(p.contract.intent) &&
    record(p.contract.limits) && Number.isSafeInteger(p.contract.limits['max-actions']) &&
    Array.isArray(p.observations) && p.observations.every(observation => record(observation) && strings(observation.invocation, ['id', 'action-id']) &&
      ['output', 'error'].every(key => observation[key] == null || typeof observation[key] === 'string')) &&
    (p.decisions == null || Array.isArray(p.decisions) && p.decisions.every(record)) &&
    (p['jev-status'] == null || typeof p['jev-status'] === 'string') &&
    (p['decision-effects'] == null || Array.isArray(p['decision-effects']) && p['decision-effects'].every(effect =>
      strings(effect, ['decision-id', 'action-id', 'kind']) && ['defer', 'finish'].includes(effect.kind) && stringArray(effect.actions) && stringArray(effect.evidence))) &&
    (p.report == null || strings(p.report, ['summary', 'counter-evidence', 'recommendation', 'regression', 'rollback']) && Array.isArray(p.report.findings) &&
      p.report.findings.every(finding => strings(finding, ['hypothesis', 'status', 'reason'])) &&
      (p.report.limitations == null || Array.isArray(p.report.limitations) && p.report.limitations.every(limit => typeof limit === 'string')));
}

function InvestigationPanel({ control, selection, run, t }) {
  const preparation = control.status?.preparation;
  if (!preparation) return null;
  const busy = control.pending !== null || control.uncertainty !== null || control.stale;
  const ended = !!preparation.report || preparation.status === 'cancelled';
  const mutate = action => run('investigate', selection, false, { investigation: { action, revision: preparation.revision } });
  return h('section', { className: css.section, 'aria-label': t('investigationTitle') },
    h('h3', { className: css.heading }, t('investigationTitle') + ' · ' + preparation.contract.id),
    h('p', { className: css.meta }, t('investigationHelp')),
    h('p', null, preparation.contract.goal),
    h('p', { role: 'status' }, t('investigationState_' + preparation.status)),
    h('p', { className: css.meta }, t('investigationBudget', { used: preparation['actions-used'], limit: preparation.contract.limits['max-actions'], ms: preparation['elapsed-ms'] })),
    h('p', { className: css.meta }, preparation.reason),
    preparation['jev-status'] && h('p', { className: css.meta }, t('investigationJEVObserved') + ': ' + t('investigationJEV_' + preparation['jev-status'])),
    h('div', { className: css.actions },
      h(Button, { disabled: busy || control.status.running || ended || preparation.status === 'awaiting-report', onClick: () => mutate('run') }, t(preparation.pending || preparation['decision-pending'] ? 'investigationReconcile' : 'investigationRun')),
      h(Button, { disabled: busy || ended || !control.status.running && !!preparation.pending, onClick: () => control.status.running ? run('stop', selection) : mutate('cancel') }, t('stop')),
      h(Button, { disabled: control.pending !== null, onClick: () => run('status', selection) }, t('status'))),
    h('p', { className: css.notice }, t(preparation.contract.intent === 'investigate-and-fix' ? 'investigationRepair' : 'investigationReportOnly')),
    preparation.report && h('article', null, h('h4', null, t('investigationReport')), h('p', null, preparation.report.summary),
      ...preparation.report.findings.map(finding => h('p', { key: finding.hypothesis }, finding.hypothesis + ' · ' + finding.status + ': ' + finding.reason)),
      h('p', null, preparation.report['counter-evidence']),
      ...(preparation.report.limitations ?? []).map((limit, i) => h('p', { key: i, className: css.warningText }, limit)),
      h('p', null, preparation.report.recommendation), h('p', null, preparation.report.regression), h('p', null, preparation.report.rollback)),
    h('details', { className: css.details }, h('summary', null, t('investigationEvidence')),
      ...preparation.observations.map(observation => h('article', { key: 'observation:' + observation.invocation.id },
        h('h4', null, observation.invocation['action-id'] + ' · ' + observation.status),
        h('p', { className: css.meta }, observation.invocation.id + ' · ' + observation['duration-ms'] + ' ms'),
        observation.truncated && h('p', { className: css.warningText }, t('investigationPartial')),
        h('pre', { className: css.raw }, observation.output || observation.error)))),
    (preparation.decisions?.length > 0 || preparation['decision-effects']?.length > 0) && h('details', { className: css.details }, h('summary', null, t('investigationDecision')),
      ...(preparation.decisions ?? []).map(receipt => h('p', { key: 'receipt:' + receipt['decision-id'] }, receipt['decision-id'] + ' · ' + receipt.status + ' · ' + receipt.reason)),
      ...preparation.observations.map(observation => h('p', { key: 'observation:' + observation.invocation.id }, observation.invocation['action-id'] + ' · ' + t(observation.invocation.adopted === true ? 'investigationAdopted' : 'investigationBaseline') + ' · ' + observation.invocation.reason)),
      ...(preparation['decision-effects'] ?? []).map(effect => h('article', { key: 'effect:' + effect['decision-id'] },
        h('h4', null, t('investigationEffect_' + effect.kind)),
        h('p', { className: css.meta }, effect['decision-id']),
        effect.actions.length > 0 && h('p', null, t('investigationSkipped') + ': ' + effect.actions.join(', ')),
        h('p', { className: css.meta }, t('investigationCitations') + ': ' + effect.evidence.join(', '))))));
}

Object.assign(zh, { sharedBatch: '共享目录批次 · 统一验收与提交', sharedBatchError: '上次批次错误' });
Object.assign(en, { sharedBatch: 'Shared checkout · collective verification and delivery', sharedBatchError: 'Previous batch error' });
// Native Conversation View content; no host DOM replacement or second global sidebar.
Object.assign(zh, {
  consoleCanvas: '流程',
  consoleConfig: '配置',
  consoleRuns: '运行记录',
  consoleTitle: 'Flywheel 流程工作台',
  consoleScoped: '同任务、同角色续接',
  consoleFresh: '每次创建新会话',
  wbOverview: '任务全貌',
  wbProgress: '进展',
  wbSession: '会话',
  wbConfiguration: '本次配置',
  wbClose: '关闭详情',
  wbCloseDialog: '关闭对话框',
  wbCanvas: '画布',
  wbCanvasRegion: '运行画布',
  wbList: '列表',
  wbBack: '返回任务全貌',
  wbExpand: '展开任务画布',
  wbExpandGroup: '展开阶段',
  wbFit: '适合宽度',
  wbFocus: '专注画布',
  wbExitFocus: '退出专注画布',
  wbZoomIn: '放大画布',
  wbZoomOut: '缩小画布',
  wbLocate: '定位当前步骤',
  wbBlocked: '处理卡点',
  wbAggregate: '汇总与终验',
  wbDelivery: 'PR 与 Merge',
  wbNoRun: '尚未启动',
  wbReadFirst: '读取任务状态后显示流程',
  wbStartHelp: '在当前对话使用 /flywheel-task 描述目标；引擎启动后自动关联画布',
  wbProposalHelp: '只准备方案可用 /flywheel-new；加载 Skill 本身不会启动引擎',
  wbSwitchMode: '启动选项',
  wbLoading: '正在读取',
  wbPending: '等待操作回执',
  wbUnknown: '先核对上次调用',
  wbRunning: '运行中，需先停止并确认收尾',
  wbStale: '状态尚未核对，请刷新',
  wbHeld: '人工接管中，需验证并交回',
  wbRevision: '缺少有效状态版本，请刷新',
  wbCompleted: '已完成的任务需说明原因后重新执行',
  wbL1Required: '需先通过本次确定性验证',
  wbReviewRequired: '需先通过独立 Review',
  wbPacketsRequired: '需先完成所有任务交付',
  wbDependencies: '等待依赖任务交付；依赖编号见运行记录与文件位置',
  wbRunTask: '运行此任务',
  wbRetry: '重试一次',
  wbReopen: '重新执行',
  wbConfirmStep: '确认节点操作',
  wbConfirm: '确认运行',
  wbCancel: '取消',
  wbRetryHelp: '追加一次 Execute / L1 尝试，保留累计失败与预算；不会自动重跑其他任务',
  wbStateChanged: '状态已变化，请关闭后重新选择',
  wbNoReason: '请说明继续的原因',
  wbTakeover: '人工接管',
  wbSnapshot: '本次运行的配置快照；修改默认配置只影响下次启动',
  wbExecutorHistory: '执行与 Review 使用独立角色会话',
  wbEvidence: '查看证据与失败原因',
  wbNoEvidence: '当前没有已记录的失败说明',
  wbFailureCode: '失败代码',
  wbCurrentResult: '本次结果',
  wbSerialReview: '执行按调度策略推进，Review 与集成串行',
  wbOperation: '最近操作',
  wbReadOnly: '运行位置由引擎取得，只读',
  wbPaths: '位置与来源',
  wbRepository: '仓库',
  wbNoSessions: '此节点还没有已登记的 Provider 会话',
  wbTechnical: '运行标识与详细证据',
  wbPRClosed: 'PR 已关闭',
  wbPRReady: 'PR 已创建',
  wbUnobserved: '尚未查询远端',
  wbScheduled: '已有执行记录',
  wbNotApplicable: '本流程不适用',
  wbGate: '确定性验证',
});
Object.assign(en, {
  consoleCanvas: 'Workflow',
  consoleConfig: 'Configuration',
  consoleRuns: 'Run history',
  consoleTitle: 'Flywheel workbench',
  wbOverview: 'Task overview',
  wbProgress: 'Progress',
  wbSession: 'Sessions',
  wbConfiguration: 'Run configuration',
  wbClose: 'Close details',
  wbCloseDialog: 'Close dialog',
  wbCanvas: 'Canvas',
  wbCanvasRegion: 'Workflow canvas',
  wbList: 'List',
  wbBack: 'Back to overview',
  wbExpand: 'Open task workflow',
  wbExpandGroup: 'Open stages',
  wbFit: 'Fit width',
  wbFocus: 'Focus canvas',
  wbExitFocus: 'Exit focus canvas',
  wbZoomIn: 'Zoom in',
  wbZoomOut: 'Zoom out',
  wbLocate: 'Locate current step',
  wbBlocked: 'Inspect blocker',
  wbAggregate: 'Integration and final checks',
  wbDelivery: 'PR and Merge',
  wbNoRun: 'Not started',
  wbReadFirst: 'Read task status to display its workflow',
  wbStartHelp:
    'Describe your goal with /flywheel-task in this conversation; the canvas is associated after engine launch',
  wbProposalHelp: 'Use /flywheel-new to prepare a proposal. Loading a skill does not launch the engine',
  wbSwitchMode: 'Launch options',
  wbLoading: 'Reading',
  wbPending: 'Awaiting operation receipt',
  wbUnknown: 'Reconcile the previous invocation first',
  wbRunning: 'Running; stop and confirm cleanup first',
  wbStale: 'Refresh to reconcile the snapshot',
  wbHeld: 'Human control is held; validate and return first',
  wbRevision: 'No valid state revision; refresh first',
  wbCompleted: 'Reopening a completed task requires a reason',
  wbL1Required: 'Current deterministic verification must pass first',
  wbReviewRequired: 'Independent review must pass first',
  wbPacketsRequired: 'All tasks must be delivered first',
  wbDependencies: 'Waiting for dependency delivery; dependency IDs are listed in run details',
  wbRunTask: 'Run this task',
  wbRetry: 'Retry once',
  wbReopen: 'Reopen task',
  wbConfirmStep: 'Confirm node operation',
  wbConfirm: 'Run',
  wbCancel: 'Cancel',
  wbRetryHelp:
    'Add one Execute/L1 attempt while retaining failures and budget. Other tasks are not automatically rerun',
  wbStateChanged: 'State changed. Close and select the operation again',
  wbNoReason: 'Explain why another attempt is needed',
  wbTakeover: 'Human takeover',
  wbSnapshot: 'Configuration frozen for this run; default edits apply to the next start',
  wbExecutorHistory: 'Execution and Review retain separate role histories',
  wbEvidence: 'Inspect evidence and failures',
  wbNoEvidence: 'No failure explanation has been recorded',
  wbFailureCode: 'Failure code',
  wbCurrentResult: 'Current result',
  wbSerialReview: 'Execution follows the scheduling policy; Review and integration remain serial',
  wbOperation: 'Latest operation',
  wbReadOnly: 'Read-only locations supplied by the engine',
  wbPaths: 'Locations and source',
  wbRepository: 'Repository',
  wbNoSessions: 'No Provider session has been registered for this node',
  wbTechnical: 'Run identity and detailed evidence',
  wbPRClosed: 'PR closed',
  wbPRReady: 'PR created',
  wbUnobserved: 'Remote not inspected',
  wbScheduled: 'Execution recorded',
  wbNotApplicable: 'Not applicable to this workflow',
  wbGate: 'Deterministic checks',
});

function WorkbenchDialog({ title, children, onClose, actions, t }) {
  const ref = React.useRef(null);
  const titleID = React.useId();
  React.useEffect(() => {
    const dialog = ref.current,
      previous = document.activeElement;
    if (typeof dialog.showModal === 'function') dialog.showModal();
    else dialog.setAttribute('open', ''); // jsdom / legacy browser fallback; owned subtree only.
    return () => {
      if (dialog.open && typeof dialog.close === 'function') dialog.close();
      if (previous?.isConnected) previous.focus?.();
    };
  }, []);
  return h(
    'dialog',
    {
      ref,
      className: css.workbenchDialog,
      'aria-labelledby': titleID,
      onCancel: (event) => {
        event.preventDefault();
        onClose();
      },
      onClick: (event) => {
        if (event.target === ref.current) onClose();
      },
    },
    h(
      'div',
      { className: css.dialogHeader },
      h('h3', { className: css.dialogTitle, id: titleID }, title),
      h(Button, { onClick: onClose, 'aria-label': t('wbCloseDialog') }, '×'),
    ),
    h('div', { className: css.dialogBody }, children),
    actions && h('div', { className: css.dialogActions }, actions),
  );
}

function canvasStatus(value, t) {
  const known = {
    pass: 'pass',
    passed: 'pass',
    failed: 'fail',
    return: 'reviewFailed',
    unknown: 'consoleUnconfirmed',
    closed: 'wbPRClosed',
    open: 'wbPRReady',
    na: 'wbNotApplicable',
    executed: 'executed',
    running: 'running',
    unobserved: 'wbUnobserved',
    scheduled: 'wbScheduled',
    cancelled: 'consoleOperation_cancelled',
    idle: 'pending',
  };
  return t(taskLabels[value] || packetLabels[value] || known[value] || 'unknown');
}

// Projection only: these labels never grant admission or write state.
function packetStages(packet, quick, t) {
  const evidence = packet?.evidence ?? {};
  const gate = evidence['final-gate'],
    injection = evidence['defect-injection'];
  const verification =
    evidence.l1?.pass === false ||
    (quick && (gate?.pass === false || (injection?.outcome && injection.outcome !== 'pass')))
      ? 'failed'
      : evidence.l1?.pass === true && (!quick || gate?.pass === true)
        ? 'passed'
        : 'pending';
  return [
    [
      'execute',
      t('executeRole'),
      packet?.status === 'executing' ? 'running' : evidence.executor ? 'executed' : 'pending',
    ],
    ['verify', quick ? 'L1 / L2 / L3' : 'L1', verification],
    ['review', t('reviewRole'), evidence.review?.verdict ?? 'pending'],
    [
      'integrate',
      t('consoleDelivery'),
      packet?.status === 'completed'
        ? 'completed'
        : packet?.commit?.error || packet?.commit?.['merge-error']
          ? 'failed'
          : 'pending',
    ],
  ];
}

function canvasModel(control, scope, page, whole, t) {
  const state = control.status?.state;
  if (!state) return null;
  const entries = Object.entries(state.packets ?? {})
    .filter(([, p]) => record(p))
    .sort(([a], [b]) => a.localeCompare(b));
  const pageIndex = Math.min(page, Math.max(0, Math.ceil(entries.length / 6) - 1));
  const visible = entries.slice(pageIndex * 6, pageIndex * 6 + 6);
  const quick = state.workflow?.effective === 'quick',
    single = singleRunKind(control),
    focused = !!single && !whole;
  const nodes = [],
    edges = [];
  const add = (id, title, status, x, y, packet, stage, group) =>
    nodes.push({ id, title, status, x, y, packet, stage, group });
  const milestones = Object.values(state.milestones ?? {}).filter(record);
  const plan =
    state.milestones?.[state['current-milestone']]?.plan ??
    milestones
      .map((m) => m.plan)
      .filter(Boolean)
      .sort((a, b) => (b['started-at'] || '').localeCompare(a['started-at'] || ''))[0];
  const delivered = entries.length > 0 && entries.every(([, p]) => p.status === 'completed');
  const closed = milestones.length > 0 && milestones.every((m) => m.status === 'closed');
  const failedGate = milestones.some(
    (m) => m.gate?.pass === false || (m.gate?.judge?.verdict && m.gate.judge.verdict !== 'pass'),
  );
  const finalStatus = (quick ? delivered : closed) ? 'completed' : failedGate ? 'failed' : 'pending';
  const remote = control.delivery;
  const pull =
    remote?.pulls?.find((p) => p.headRefOid === remote.head && p.state === 'OPEN') ??
    remote?.pulls?.find((p) => p.headRefOid === remote.head);
  const remoteStatus = control.deliveryStale
    ? 'unknown'
    : pull?.state === 'MERGED'
      ? 'completed'
      : pull?.state === 'OPEN'
        ? 'open'
        : pull?.state === 'CLOSED'
          ? 'closed'
          : state.delivery && ['pending', 'unknown'].includes(state.delivery.status)
            ? 'unknown'
            : 'unobserved';
  const packetScope = scope?.packet && entries.find(([id]) => id === scope.packet);
  let height = 420,
    width = 984;
  if (focused) {
    const operationStatus =
      control.stale || (state.control.status === 'running' && !control.status.running)
        ? 'unknown'
        : state.control.status;
    if (['plan', 'final', 'integrate'].includes(single))
      add(single, stageName(single, t), operationStatus, 24, 110, null, single);
    else {
      const units = state.control.packet ? entries.filter(([id]) => id === state.control.packet) : visible;
      units.forEach(([id, packet], row) => {
        const stages =
          single === 'review'
            ? ['verify', 'review']
            : single === 'execute'
              ? ['execute', 'verify']
              : ['verify'];
        stages.forEach((stage, col) => {
          let status = packetStages(packet, quick, t).find((s) => s[0] === stage)[2];
          if (stage === single && operationStatus !== 'passed') status = operationStatus;
          add(
            id + '/' + stage,
            id + ' · ' + stageName(stage, t),
            status,
            24 + col * 224,
            48 + row * 148,
            packet,
            stage,
          );
          if (col) edges.push([id + '/' + stages[col - 1], id + '/' + stage]);
        });
      });
      height = Math.max(height, units.length * 148 + 72);
      if (!nodes.length) add(single, stageName(single, t), 'unknown', 24, 110);
    }
    width = nodes.length > 1 ? 472 : 248;
  } else if (packetScope || (quick && entries.length === 1 && !scope)) {
    const [id, packet] = packetScope || entries[0];
    packetStages(packet, quick, t).forEach(([stage, title, status], col) => {
      add(id + '/' + stage, title, status, 24 + col * 224, 120, packet, stage);
      if (col) edges.push([nodes[col - 1].id, id + '/' + stage]);
    });
    width = 900;
  } else if (scope?.group === 'final') {
    add('aggregate', t('consoleAggregate'), delivered ? 'completed' : 'pending', 24, 120);
    add('judge', quick ? t('consoleDelivery') : 'L2 / L3 · Judge', finalStatus, 248, 120, null, 'final');
    edges.push(['aggregate', 'judge']);
    width = 472;
  } else if (scope?.group === 'remote') {
    add('delivery', 'PR', remoteStatus, 24, 120);
    add(
      'merge',
      'Merge',
      control.deliveryStale ? 'unknown' : pull?.state === 'MERGED' ? 'completed' : 'pending',
      248,
      120,
    );
    edges.push(['delivery', 'merge']);
    width = 472;
  } else {
    const center = 24 + Math.max(0, visible.length - 1) * 68;
    add(
      'plan',
      t('consolePlanSplit'),
      quick ? 'na' : (plan?.status ?? 'pending'),
      24,
      center,
      null,
      quick ? null : 'plan',
    );
    add(
      'schedule',
      t('consoleSchedule'),
      control.status.running
        ? 'running'
        : entries.some(([, packet]) => packet.attempts > 0 || packet.evidence?.executor)
          ? 'scheduled'
          : 'pending',
      220,
      center,
    );
    edges.push(['plan', 'schedule']);
    visible.forEach(([id, packet], row) => {
      add(id, id, packet.status, 416, 24 + row * 136, packet);
      edges.push(['schedule', id]);
      edges.push([id, 'aggregate']);
    });
    add('aggregate', t('wbAggregate'), finalStatus, 612, center, null, null, 'final');
    add('delivery', t('wbDelivery'), remoteStatus, 808, center, null, null, 'remote');
    edges.push(['aggregate', 'delivery']);
    height = Math.max(height, visible.length * 136 + 48);
  }
  return {
    state,
    entries,
    visible,
    pageIndex,
    nodes,
    edges,
    quick,
    single,
    focused,
    scope: packetScope,
    width,
    height,
  };
}

function nodeBinding(node, state, quick, t) {
  const id = node.packet ? node.id.split('/')[0] : null;
  const role =
    node.stage === 'review'
      ? 'review'
      : node.stage === 'plan'
        ? 'plan'
        : node.stage === 'final' && !quick
          ? 'judge'
          : node.packet && (!node.stage || node.stage === 'execute')
            ? 'execute'
            : null;
  if (!role) return null;
  const scope = id || state['current-milestone'];
  const actual = Object.values(state.sessions?.records ?? {})
    .filter((s) => s.role === role && (!scope || s.scope === scope || s.scope === 'final:' + scope))
    .sort((a, b) => (b.last?.['started-at'] || '').localeCompare(a.last?.['started-at'] || ''))[0];
  if (state.run?.drivers?.[role] === 'mock')
    return { harness: 'mock', model: t('consoleMockModel'), sessionMode: 'fresh' };
  return actual
    ? (actual.configuration ?? null)
    : state.run?.bindings?.[role === 'execute' ? 'routes.' + node.packet.route : 'loops.' + role];
}

function bindingCaption(binding) {
  const provider = binding.provider || binding.harness;
  const label =
    {
      'deepseek-account': 'DeepSeek',
      chatgpt: 'ChatGPT',
      kimi: 'Kimi Code',
      'codex-pro': 'Codex Pro',
      'claude-code-pro': 'Claude Code Pro',
    }[provider] || provider;
  const model =
    { 'deepseek-flash': 'Flash', 'kimi-code/k3.1-pro': 'K3.1 Pro' }[binding.model] || binding.model;
  return [label, model].filter(Boolean).join(' · ');
}

function stageBlockReason(control, node, state) {
  if (control.pending) return 'wbPending';
  if (control.uncertainty) return 'wbUnknown';
  if (control.stale) return 'wbStale';
  if (control.status.running) return 'wbRunning';
  if (state.handoff) return 'wbHeld';
  if (!Number.isSafeInteger(state.revision) || state.revision < 1) return 'wbRevision';
  if (
    node?.packet &&
    (!node.stage || node.stage === 'execute') &&
    node.packet.schedule?.after?.some((id) => state.packets?.[id]?.status !== 'completed')
  )
    return 'wbDependencies';
  if (node?.stage === 'review' && node.packet?.evidence?.l1?.pass !== true) return 'wbL1Required';
  if (node?.stage === 'integrate' && node.packet && node.packet.evidence?.review?.verdict !== 'pass')
    return 'wbReviewRequired';
  if (node?.stage === 'final' && Object.values(state.packets ?? {}).some((p) => p.status !== 'completed'))
    return 'wbPacketsRequired';
  return null;
}

function ConsoleCanvas({ control, selection, run, t, focusRequest = 0 }) {
  const [scope, setScope] = React.useState(null),
    [selected, setSelected] = React.useState(null);
  const [page, setPage] = React.useState(0),
    [whole, setWhole] = React.useState(false),
    [tab, setTab] = React.useState('progress');
  const [view, setView] = React.useState('canvas'),
    [zoom, setZoom] = React.useState(1);
  const [request, setRequest] = React.useState(null),
    [reason, setReason] = React.useState('');
  const [focusMode, setFocusMode] = React.useState(false);
  const canvasDialog = React.useRef(null);
  const focusToggle = React.useRef(null),
    wasFocused = React.useRef(false);
  const viewport = React.useRef(null),
    drag = React.useRef(null),
    lastFocus = React.useRef(0);
  const model = canvasModel(control, scope, page, whole, t);
  const manualView = React.useRef(false);
  React.useEffect(() => {
    const container = viewport.current?.parentElement;
    if (!container) return;
    const resize = () => {
      const width = container.clientWidth;
      if (width > 0 && !manualView.current) setView(width < 520 ? 'list' : 'canvas');
    };
    resize();
    if (typeof ResizeObserver !== 'function') return;
    const observer = new ResizeObserver(resize);
    observer.observe(container);
    return () => observer.disconnect();
  }, [!!model, focusMode]);
  React.useEffect(() => {
    if (!focusMode || !model) {
      if (wasFocused.current) focusToggle.current?.focus();
      wasFocused.current = false;
      return;
    }
    wasFocused.current = true;
    const dialog = canvasDialog.current;
    if (typeof dialog.showModal === 'function') dialog.showModal();
    else dialog.setAttribute('open', '');
    return () => {
      if (dialog.open && typeof dialog.close === 'function') dialog.close();
    };
  }, [focusMode, !!model]);
  React.useEffect(() => {
    if (control.uncertainty) setFocusMode(false);
  }, [control.uncertainty]);
  const locate = () => {
    if (!model) return;
    const { nodes } = model;
    if (!model.focused) {
      const index = model.entries.findIndex(([, packet]) => packet.status === 'executing');
      if (index >= 0 && !nodes.some((node) => node.packet === model.entries[index][1])) {
        setScope(null);
        setPage(Math.floor(index / 6));
        setSelected(model.entries[index][0]);
        setTab('progress');
        if (viewport.current) {
          viewport.current.scrollLeft = 392 * zoom;
          viewport.current.scrollTop = (index % 6) * 148 * zoom;
        }
        return;
      }
    }
    const target =
      nodes.find((n) => ['running', 'executing'].includes(n.status)) ||
      nodes.find(
        (n) =>
          ['failed', 'l1-failed', 'review-failed', 'escalated', 'blocked', 'unknown'].includes(n.status) &&
          n.packet,
      ) ||
      nodes.find((n) => n.packet?.status !== 'completed' && n.packet) ||
      nodes[0];
    setSelected(target.id);
    setTab('progress');
    if (viewport.current) {
      viewport.current.scrollLeft = Math.max(0, target.x * zoom - 24);
      viewport.current.scrollTop = Math.max(0, target.y * zoom - 48);
    }
  };
  React.useEffect(() => {
    if (focusRequest && focusRequest !== lastFocus.current && model) {
      lastFocus.current = focusRequest;
      locate();
    }
  }, [focusRequest, !!model]);
  if (!model) return h(InvestigationPanel, { control, selection, run, t });
  const { nodes, edges, state, quick, single, focused, entries, pageIndex } = model;
  const chosen = nodes.find((n) => n.id === selected);
  const selectNode = (node) => {
    setSelected(node.id);
    setTab('progress');
  };
  const expand = (node) => {
    setScope(node.packet ? { packet: node.id.split('/')[0] } : { group: node.group });
    setSelected(null);
    setZoom(1);
  };
  const back = () => {
    setScope(null);
    setSelected(null);
    setZoom(1);
  };
  const planRequest = (node, retry = false) => {
    setReason('');
    setRequest({
      only: node.stage || 'execute',
      packet: node.packet ? node.id.split('/')[0] : undefined,
      revision: state.revision,
      retry,
      reopen: node.packet?.status === 'completed',
    });
  };
  const finishRequest = () => {
    const next = request;
    setRequest(null);
    void run('step', selection, false, {
      only: next.only,
      packet: next.packet,
      revision: next.revision,
      ...(next.retry ? { retryReason: reason.trim() } : {}),
    });
  };
  const nodeButton = (node) => {
    const binding = nodeBinding(node, state, quick, t);
    return h(
      'button',
      {
        key: node.id,
        type: 'button',
        className: css.canvasNode,
        'data-node-id': node.id,
        'data-tone': consoleTone(node.status),
        'aria-pressed': selected === node.id,
        style: { '--node-x': node.x + 'px', '--node-y': node.y + 'px' },
        onClick: () => selectNode(node),
        onFocus: (event) => {
          if (event.currentTarget.matches?.(':focus-visible'))
            event.currentTarget.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
        },
      },
      h('span', { className: css.nodeState }, canvasStatus(node.status, t)),
      h('strong', null, node.title),
      h(
        'span',
        { className: css.nodeProvider },
        binding
          ? bindingCaption(binding)
          : node.packet?.route ||
              (node.stage && ['execute', 'review', 'plan', 'final'].includes(node.stage)
                ? t('consoleUnknown')
                : 'Go / Git'),
      ),
      node.packet &&
        !node.stage &&
        h(
          'span',
          { className: css.nodeStages },
          ...packetStages(node.packet, quick, t).map(([stage, title, status]) =>
            h(
              'span',
              {
                key: stage,
                'data-tone': consoleTone(status),
                title: title + ' · ' + canvasStatus(status, t),
              },
              title,
            ),
          ),
        ),
    );
  };
  return h(
    'section',
    { className: css.consoleCanvas, 'aria-label': t('wbCanvasRegion') },
    single &&
      h(
        'div',
        { className: css.stageScope },
        h(
          'div',
          null,
          h('strong', null, singleRunLabel(control, t)),
          h(
            'p',
            { className: css.meta },
            t('stageFullState', {
              status: t(state.status === 'completed' ? 'stageDelivered' : 'stageUndelivered'),
            }),
          ),
        ),
        h(
          'div',
          { className: css.actions },
          h(
            Button,
            {
              'aria-pressed': !whole,
              onClick: () => {
                setWhole(false);
                setSelected(null);
              },
            },
            t('stageFocus'),
          ),
          h(
            Button,
            {
              'aria-pressed': whole,
              onClick: () => {
                setWhole(true);
                back();
              },
            },
            t('stageFull'),
          ),
        ),
      ),
    h(
      focusMode ? 'dialog' : 'div',
      {
        ref: canvasDialog,
        className: [css.canvasWorkbench, focusMode ? css.focusCanvas : ''].join(' '),
        'data-inspecting': !!chosen,
        'aria-label': focusMode ? t('wbFocus') : undefined,
        onCancel: (event) => {
          event.preventDefault();
          setFocusMode(false);
        },
      },
      h(
        'div',
        { className: css.canvasMain },
        h(
          'div',
          { className: css.canvasHeader },
          h(
            'div',
            { className: css.canvasBreadcrumb },
            focusMode &&
              h(
                'span',
                { className: css.focusIdentity },
                [state.slug, state.run?.branch].filter(Boolean).join(' · '),
              ),
            scope && !focused
              ? h(Button, { onClick: back }, t('wbBack'))
              : h('span', null, t(focused ? 'stageFocus' : 'wbOverview')),
            h(
              'span',
              { className: css.meta },
              scope?.packet ||
                (focused
                  ? stageName(single, t)
                  : t(quick ? 'consoleQuick' : 'consoleFull') +
                    ' · ' +
                    t(state.scheduler?.mode === 'parallel' ? 'consoleParallel' : 'consoleSerial')),
            ),
          ),
          h(
            'div',
            { className: css.segmented },
            ...['canvas', 'list'].map((v) =>
              h(
                Button,
                {
                  key: v,
                  'aria-pressed': view === v,
                  onClick: () => {
                    manualView.current = true;
                    setView(v);
                  },
                },
                t(v === 'canvas' ? 'wbCanvas' : 'wbList'),
              ),
            ),
          ),
        ),
        h(
          'div',
          {
            className: css.canvasViewport,
            ref: viewport,
            hidden: view !== 'canvas',
            'aria-label': t('wbCanvas'),
            onPointerDown: (event) => {
              if (event.pointerType !== 'mouse' || event.button !== 0 || event.target.closest('button'))
                return;
              drag.current = {
                x: event.clientX,
                y: event.clientY,
                left: viewport.current.scrollLeft,
                top: viewport.current.scrollTop,
              };
              event.currentTarget.setPointerCapture?.(event.pointerId);
            },
            onPointerMove: (event) => {
              if (drag.current) {
                viewport.current.scrollLeft = drag.current.left - event.clientX + drag.current.x;
                viewport.current.scrollTop = drag.current.top - event.clientY + drag.current.y;
              }
            },
            onPointerUp: () => {
              drag.current = null;
            },
            onPointerCancel: () => {
              drag.current = null;
            },
            onLostPointerCapture: () => {
              drag.current = null;
            },
          },
          h(
            'div',
            {
              className: css.canvasScaleBounds,
              style: {
                '--scaled-width': model.width * zoom + 'px',
                '--scaled-height': model.height * zoom + 'px',
              },
            },
            h(
              'div',
              {
                className: css.canvasWorld,
                style: {
                  '--canvas-width': model.width + 'px',
                  '--canvas-height': model.height + 'px',
                  '--canvas-scale': zoom,
                },
              },
              h(
                'svg',
                { className: css.canvasEdges, width: model.width, height: model.height, 'aria-hidden': true },
                ...edges.map(([from, to]) => {
                  const a = nodes.find((n) => n.id === from),
                    b = nodes.find((n) => n.id === to),
                    mid = (a.x + 164 + b.x) / 2;
                  return h('path', {
                    key: from + to,
                    d: `M${a.x + 164} ${a.y + 58} C${mid} ${a.y + 58} ${mid} ${b.y + 58} ${b.x} ${b.y + 58}`,
                  });
                }),
              ),
              ...nodes.map(nodeButton),
            ),
          ),
        ),
        view === 'list' &&
          h(
            'div',
            { className: css.canvasList },
            ...nodes.map((node) =>
              h(
                'button',
                {
                  type: 'button',
                  key: node.id,
                  className: css.canvasListNode,
                  'data-tone': consoleTone(node.status),
                  onClick: () => selectNode(node),
                },
                h('strong', null, node.title),
                h('span', null, canvasStatus(node.status, t)),
              ),
            ),
          ),
        h(
          'div',
          { className: css.canvasFooter },
          h(
            'div',
            { className: css.canvasLegend },
            ...['completed', 'running', 'failed', 'pending'].map((status) =>
              h('span', { key: status, 'data-tone': consoleTone(status) }, canvasStatus(status, t)),
            ),
          ),
          h(
            'div',
            { className: css.actions },
            h(Button, { onClick: locate }, t('wbLocate')),
            view === 'canvas' &&
              h(
                React.Fragment,
                null,
                h(
                  Button,
                  {
                    'aria-label': t('wbZoomOut'),
                    disabled: zoom <= 0.85,
                    onClick: () => setZoom(Math.max(0.85, zoom - 0.1)),
                  },
                  '−',
                ),
                h('span', { className: css.meta }, Math.round(zoom * 100) + '%'),
                h(
                  Button,
                  {
                    'aria-label': t('wbZoomIn'),
                    disabled: zoom >= 1.5,
                    onClick: () => setZoom(Math.min(1.5, zoom + 0.1)),
                  },
                  '+',
                ),
                h(
                  Button,
                  {
                    onClick: () =>
                      setZoom(
                        Math.max(
                          0.85,
                          Math.min(1, ((viewport.current?.clientWidth || model.width) - 24) / model.width),
                        ),
                      ),
                  },
                  t('wbFit'),
                ),
              ),
            h(
              Button,
              { ref: focusToggle, onClick: () => setFocusMode((value) => !value) },
              t(focusMode ? 'wbExitFocus' : 'wbFocus'),
            ),
          ),
        ),
        !scope &&
          !focused &&
          entries.length > 6 &&
          h(
            'div',
            { className: css.canvasPagination },
            h(
              Button,
              {
                disabled: pageIndex === 0,
                onClick: () => {
                  setPage(pageIndex - 1);
                  setSelected(null);
                },
              },
              t('consolePrevious'),
            ),
            h(
              'span',
              { className: css.meta },
              t('consolePage', {
                from: pageIndex * 6 + 1,
                to: Math.min((pageIndex + 1) * 6, entries.length),
                total: entries.length,
              }),
            ),
            h(
              Button,
              {
                disabled: (pageIndex + 1) * 6 >= entries.length,
                onClick: () => {
                  setPage(pageIndex + 1);
                  setSelected(null);
                },
              },
              t('consoleMore'),
            ),
          ),
      ),
      chosen &&
        h(WorkbenchNodeInspector, {
          chosen,
          model,
          control,
          selection,
          run,
          t,
          tab,
          setTab,
          onClose: () => setSelected(null),
          expand,
          planRequest,
        }),
    ),
    request &&
      h(
        WorkbenchDialog,
        {
          t,
          title: t(request.retry ? 'wbRetry' : 'wbConfirmStep'),
          onClose: () => setRequest(null),
          actions: h(
            React.Fragment,
            null,
            h(Button, { onClick: () => setRequest(null) }, t('wbCancel')),
            h(
              Button,
              {
                variant: 'primary',
                disabled:
                  !!stageBlockReason(control, null, state) ||
                  state.revision !== request.revision ||
                  (request.retry && !reason.trim()),
                onClick: finishRequest,
              },
              t('wbConfirm'),
            ),
          ),
        },
        h('p', null, (request.packet ? request.packet + ' · ' : '') + stageName(request.only, t)),
        h(
          'p',
          { className: css.meta },
          t(request.reopen ? 'consoleReopenHelp' : request.retry ? 'wbRetryHelp' : 'consoleStageHelp'),
        ),
        request.retry &&
          h(
            'label',
            { className: css.editorField },
            t('consoleManualReason'),
            h('textarea', {
              className: css.reasonInput,
              value: reason,
              maxLength: 300,
              onChange: (e) => setReason(e.target.value),
            }),
          ),
        state.revision !== request.revision && h('p', { role: 'alert' }, t('wbStateChanged')),
      ),
  );
}

function WorkbenchNodeInspector({
  chosen,
  model,
  control,
  selection,
  run,
  t,
  tab,
  setTab,
  onClose,
  expand,
  planRequest,
}) {
  const { state, quick } = model,
    packetID = chosen.packet ? chosen.id.split('/')[0] : undefined;
  const packet =
    packetID && control.node?.id === packetID && control.node.revision === state.revision
      ? control.node.packet
      : chosen.packet;
  const binding = nodeBinding(chosen, state, quick, t),
    blocked = stageBlockReason(control, chosen, state);
  const sessions = Object.entries(state.sessions?.records ?? {})
    .filter(([, s]) => record(s))
    .filter(([, s]) =>
      packetID
        ? s.scope === packetID || s.scope === 'final:' + packetID
        : chosen.stage === 'plan'
          ? s.role === 'plan'
          : chosen.stage === 'final'
            ? s.role === 'judge'
            : false,
    );
  const primaryStage = chosen.stage || (chosen.packet ? 'execute' : null);
  const retry =
    primaryStage === 'execute' &&
    (packet?.status === 'completed' ||
      ['l1-failed', 'review-failed', 'escalated', 'blocked', 'failed'].includes(packet?.status));
  const property = (label, value) =>
    h(React.Fragment, { key: label }, h('dt', null, label), h('dd', null, textValue(value)));
  const evidence = {
    milestones: ['judge', 'plan', 'final'].includes(chosen.id) ? state.milestones : undefined,
    evidence: packet?.evidence,
    feedback: packet?.['last-feedback'],
    retries: packet?.['retry-history'],
    history: packet?.['evidence-history'],
    commit: packet?.commit,
  };
  return h(
    'section',
    { className: css.nodeInspector, 'aria-label': t('consoleDetails') },
    h(
      'header',
      { className: css.inspectorHeader },
      h(
        'div',
        null,
        packetID && h('span', { className: css.meta }, packetID),
        h('h3', { className: css.heading }, chosen.title),
      ),
      h(Button, { onClick: onClose, 'aria-label': t('wbClose') }, '×'),
    ),
    h(
      'nav',
      { className: css.inspectorTabs, 'aria-label': t('consoleDetails') },
      ...['progress', 'session', 'config'].map((key) =>
        h(
          Button,
          { key, 'aria-pressed': tab === key, onClick: () => setTab(key) },
          t({ progress: 'wbProgress', session: 'wbSession', config: 'wbConfiguration' }[key]),
        ),
      ),
    ),
    h(
      'div',
      { className: css.inspectorBody },
      tab === 'progress' &&
        h(
          React.Fragment,
          null,
          h(
            'p',
            { className: css.nodeOutcome, 'data-tone': consoleTone(chosen.status) },
            canvasStatus(chosen.status, t),
          ),
          ['error', 'warning'].includes(consoleTone(chosen.status)) &&
            packet &&
            h(
              'div',
              { className: css.configCallout },
              h(
                'p',
                null,
                textValue(
                  typeof packet['last-feedback'] === 'string'
                    ? packet['last-feedback']
                    : packet['last-feedback']?.reason ||
                        packet['last-feedback']?.['parallel-error'] ||
                        packet.commit?.error,
                  t('wbEvidence'),
                ).slice(0, 800),
              ),
              packet['last-feedback']?.['error-code'] &&
                h(
                  'p',
                  { className: css.meta },
                  t('wbFailureCode') + ' · ' + textValue(packet['last-feedback']['error-code']),
                ),
            ),
          ['delivery', 'merge'].includes(chosen.id) && h(DeliveryPanel, { control, selection, run, t }),
          chosen.packet &&
            !chosen.stage &&
            h(
              'ol',
              { className: css.stageTimeline },
              ...packetStages(packet, quick, t).map(([stage, title, status]) =>
                h(
                  'li',
                  { key: stage, 'data-tone': consoleTone(status) },
                  h('span', null, title),
                  h('span', { className: css.meta }, canvasStatus(status, t)),
                ),
              ),
            ),
          ((chosen.packet && !chosen.stage) || chosen.group) &&
            h(Button, { onClick: () => expand(chosen) }, t(chosen.packet ? 'wbExpand' : 'wbExpandGroup')),
          chosen.stage === 'review' &&
            packet?.evidence?.review &&
            h(
              'dl',
              { className: css.consoleProperties },
              property(t('stageRecordedVerdict'), packet.evidence.review.verdict),
              property(t('stageRecordedScore'), packet.evidence.review.score),
            ),
          packetID &&
            h(
              'div',
              { className: css.actions },
              h(
                Button,
                {
                  disabled: !!control.pending,
                  onClick: () => run('node', selection, false, { packet: packetID }),
                },
                t('consoleFullNode'),
              ),
            ),
          h(
            'details',
            { className: css.details, open: consoleTone(chosen.status) === 'error' },
            h('summary', null, t('wbEvidence')),
            h('pre', { className: css.raw }, JSON.stringify(evidence, null, 2)),
          ),
          packet &&
            h(
              'dl',
              { className: css.consoleProperties },
              property(t('consoleAttempts'), packet.attempts),
              property(t('consoleFailures'), packet['failed-attempts']),
            ),
        ),
      tab === 'session' &&
        h(
          React.Fragment,
          null,
          h('p', { className: css.meta }, t('wbExecutorHistory')),
          sessions.length
            ? sessions.map(([key, value]) =>
                h(SessionInspector, { key, sessionKey: key, value, control, selection, run, t }),
              )
            : h('p', { className: css.meta }, t('wbNoSessions')),
        ),
      tab === 'config' &&
        h(
          React.Fragment,
          null,
          h('p', { className: css.meta }, t('wbSnapshot')),
          binding
            ? h(
                'dl',
                { className: css.consoleProperties },
                property(t('consoleProvider'), binding.provider || binding.harness),
                property(t('consoleModel'), binding.model),
                property(t('consoleThinking'), binding.thinking),
                property(
                  t('permission'),
                  permissionLabels[binding.permission]
                    ? t(permissionLabels[binding.permission])
                    : binding.permission,
                ),
                property(
                  t('consoleSession'),
                  sessionModeLabel({ session: { mode: binding.sessionMode } }, t),
                ),
              )
            : h('p', { className: css.meta }, t('consoleUnknown')),
        ),
      h(
        'details',
        { className: css.details },
        h('summary', null, t('stageMetadata')),
        h(
          'dl',
          { className: css.consoleProperties },
          property('Run', state.run?.id),
          property('HEAD', state.run?.head),
          property(t('consoleTask'), control.status.taskDir),
          property(t('projectHome'), state.project?.['workspace-cwd']),
          property(t('executionHome'), packet?.worktree?.path ?? control.status.cwd),
          packet?.worktree?.external && property(t('externalWorktree'), packet.worktree.path),
          property(t('consoleBranch'), packet?.worktree?.branch || state.run?.branch),
          property(t('consoleRevision'), state.revision),
          property('after', packet?.schedule?.after?.join(', ')),
          property('writes', packet?.schedule?.writes?.join(', ')),
        ),
      ),
    ),
    h(
      'footer',
      { className: css.inspectorFooter },
      blocked && h('p', { className: css.meta }, t(blocked)),
      h(
        'div',
        { className: css.actions },
        primaryStage &&
          h(
            Button,
            {
              variant: 'primary',
              disabled: !!blocked || (packet?.status === 'completed' && !retry),
              onClick: () => planRequest({ ...chosen, stage: primaryStage }, retry),
            },
            t(
              retry
                ? packet.status === 'completed'
                  ? 'wbReopen'
                  : 'wbRetry'
                : chosen.stage
                  ? 'consoleStageRun'
                  : 'wbRunTask',
            ),
          ),
        sessions.length > 0 && h(Button, { onClick: () => setTab('session') }, t('wbTakeover')),
      ),
    ),
  );
}

function TaskView({
  hostSessionID,
  useStore,
  useProjection,
  actions,
  useControl,
  changeSelection,
  associate,
  run,
  discover,
  reconcile,
  t,
}) {
  const selection = useStore((value) => value),
    control = useControl((value) => value),
    association = useProjection('flywheelTask');
  const [page, setPage] = React.useState('canvas'),
    [focusRequest, setFocusRequest] = React.useState(0);
  const configDrafts = React.useRef(new Map());
  React.useEffect(() => {
    associate(association);
  }, [association, associate]);
  React.useEffect(() => {
    if (page !== 'canvas') return;
    let live = true,
      timer;
    void discover();
    const refresh = async () => {
      if (!document.hidden) await discover();
      if (live) timer = setTimeout(refresh, 5000);
    };
    timer = setTimeout(refresh, 5000);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [page, discover]);
  React.useEffect(() => {
    if (page !== 'canvas' || !control.status?.running || !selection.taskDir) return;
    let live = true,
      timer;
    const refresh = async () => {
      let value;
      if (!document.hidden) value = await run('status', selection);
      if (live) timer = setTimeout(refresh, value ? 2000 : 10000);
    };
    timer = setTimeout(refresh, 2000);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [page, control.status?.running, selection.taskDir, selection.cwd, run]);
  const busy = control.pending !== null,
    locked = busy && mutates(control.pending.action),
    state = control.status?.state,
    preparation = control.status?.preparation;
  const invalid = !!pathProblem(selection.taskDir) || !!pathProblem(selection.cwd),
    hasTask = !!selection.taskDir.trim();
  const button = (action, label, needsTask, probe = false, variant = 'outline') =>
    h(
      Button,
      {
        variant,
        disabled:
          busy ||
          invalid ||
          (needsTask && !hasTask) ||
          (mutates(action) && !!control.uncertainty) ||
          (['launch', 'start'].includes(action) && !!state?.handoff),
        onClick: () => run(action, selection, probe),
      },
      t(label),
    );
  const blocked = Object.values(state?.packets ?? {}).some((p) =>
    ['l1-failed', 'review-failed', 'escalated', 'blocked'].includes(p.status),
  );
  return h(
    'section',
    { className: css.page, 'aria-label': t('consoleTitle') },
    h(
      'style',
      {
        'data-plugin': '@ckanner/dsh-plugin-flywheel',
        'data-plugin-css': '@ckanner/dsh-plugin-flywheel/client.module.css',
      },
      stylesheet,
    ),
    h(
      'header',
      { className: css.workbenchHeader },
      h(TaskChooser, { control, selection, hostSessionID, association, changeSelection, run, discover, t }),
      h(
        'div',
        { className: css.repositoryLinks },
        state?.project?.url &&
          h(
            'a',
            { href: state.project.url, target: '_blank', rel: 'noopener noreferrer' },
            state.project.repo,
          ),
        h(
          'a',
          { href: 'https://github.com/ckanner/flywheel', target: '_blank', rel: 'noopener noreferrer' },
          'Flywheel · GitHub',
        ),
      ),
    ),
    h(
      'nav',
      { className: css.consoleTabs, 'aria-label': t('view') },
      ...['canvas', 'runs', 'config'].map((key) =>
        h(
          Button,
          { key, 'aria-pressed': page === key, onClick: () => setPage(key) },
          t({ canvas: 'consoleCanvas', runs: 'consoleRuns', config: 'consoleConfig' }[key]),
        ),
      ),
    ),
    (control.pending || control.error || control.uncertainty || control.status?.workspaceWarnings?.length) &&
      !(
        page === 'config' &&
        (control.pending?.action === 'config-edit' || control.error?.action === 'config-edit') &&
        !control.uncertainty
      ) &&
      h(Feedback, { control, selection, reconcile, t }),
    h(
      'div',
      { hidden: page !== 'canvas' },
      state?.['shared-execution'] && h('section', { className: css.notice, role: 'status' },
        h('strong', null, t('sharedBatch')),
        h('p', null, state['shared-execution'].packets.join(' · ')),
        state['shared-execution']['last-error'] && h('p', { role: 'alert' }, t('sharedBatchError') + ': ' + state['shared-execution']['last-error'])),
      hasTask && !preparation &&
        h(
          'div',
          { className: css.workbenchStatus },
          state &&
            h(
              'span',
              {
                'data-tone': consoleTone(
                  control.stale ? 'unknown' : control.status.running ? 'running' : state.status,
                ),
              },
              singleRunKind(control) ? singleRunLabel(control, t) : taskLabel(control.status, t),
            ),
          button('status', 'status', true),
          blocked &&
            !control.status?.running &&
            h(Button, { variant: 'primary', onClick: () => setFocusRequest((v) => v + 1) }, t('wbBlocked')),
          !singleRunKind(control) && button('start', 'start', true, false, blocked ? 'outline' : 'primary'),
          singleRunKind(control) &&
            h(
              'details',
              { className: css.launchOptions },
              h('summary', null, t('stageContinue')),
              button('start', 'consoleWhole', true),
            ),
          (control.status?.running || state?.status === 'running' || locked || control.uncertainty) &&
            button('stop', 'stop', true),
          h(
            'details',
            { className: css.launchOptions },
            h('summary', null, t('wbSwitchMode')),
            h(
              'div',
              { className: css.launchOptionsBody },
              h(
                'label',
                { className: css.editorField },
                t('launchMode'),
                h(
                  'select',
                  {
                    className: css.modeSelect,
                    value: selection.mode,
                    disabled: locked,
                    onChange: (e) => actions.setMode(e.target.value),
                  },
                  ...['auto', 'quick', 'full'].map((mode) =>
                    h(
                      'option',
                      { key: mode, value: mode },
                      mode === 'auto' ? 'Auto' : mode === 'quick' ? 'ultraWork / Quick' : 'Full',
                    ),
                  ),
                ),
              ),
              button('launch', 'launch', true),
              h('p', { className: css.meta }, t('launchHelp')),
            ),
          ),
        ),
      preparation && h(InvestigationPanel, { control, selection, run, t }),
      !state && !preparation &&
        h(
          'div',
          { className: css.workbenchEmpty },
          h('h2', { className: css.title }, t(hasTask ? 'wbReadFirst' : 'taskNoSelection')),
          h('p', null, t('wbStartHelp')),
          h('code', null, '/flywheel-task <goal>'),
          h('p', { className: css.meta }, t('wbProposalHelp')),
        ),
      !preparation && h(ConsoleCanvas, {
        key: selectionKey(selectionOf(selection)),
        control,
        selection,
        run,
        t,
        focusRequest,
      }),
      hasTask && !preparation &&
        h(
          'details',
          { className: css.legacySummary },
          h('summary', null, t('consoleTaskSummary')),
          h(TaskStatus, { control, selection, t }),
        ),
    ),
    h(
      'div',
      { hidden: page !== 'config' },
      h(ConfigEditor, {
        key: selectionKey(selectionOf(selection)),
        contextKey: selectionKey(selectionOf(selection)),
        cache: configDrafts.current,
        control,
        selection,
        run,
        t,
        active: page === 'config',
      }),
      h(
        'details',
        {
          className: css.advanced,
          open: selection.advancedOpen,
          onToggle: (e) => actions.setAdvancedOpen(e.currentTarget.open),
        },
        h('summary', { className: css.heading }, t('advanced')),
        h(
          'div',
          { className: css.advancedBody },
          h('p', { className: css.meta }, t('advancedHelp')),
          h(
            'div',
            { className: css.actions },
            button('config', 'config', false),
            button('doctor', 'doctor', false),
            button('doctor', 'probe', false, true),
          ),
          h('p', { className: css.meta }, t('doctorHelp')),
          h('p', { className: css.meta }, t('probeHelp')),
          h(Diagnostic, { value: control.diagnostic, t }),
          h(Configuration, { value: control.configuration, t }),
        ),
      ),
      control.raw &&
        h(
          'details',
          { className: css.details },
          h('summary', null, t('raw')),
          h('pre', { className: css.raw }, JSON.stringify(control.raw, null, 2)),
        ),
    ),
    h(
      'div',
      { hidden: page !== 'runs' },
      h(RunExplorer, {
        control,
        selection,
        run,
        changeSelection,
        showCanvas: () => setPage('canvas'),
        t,
        active: page === 'runs',
      }),
    ),
  );
}
