// Compiled into the same lazy factory as client.js. All writes use the Go control boundary.
Object.assign(zh, {
  consoleLastOperation: '最近节点操作', consoleOperation_running: '等待结束回执', consoleOperation_passed: '已完成', consoleOperation_failed: '未通过', consoleOperation_cancelled: '已取消', consoleSnapshotStale: '快照已过期', consoleLaunchOptions: '准备任务与启动选项', consoleTaskSummary: '任务总览与累计用量', consoleMockModel: '离线演练 · 不调用模型', consoleFullNode: '读取完整节点证据', consoleExcerpt: '画布使用有界摘要；完整节点证据按需读取', consoleAssociated: '本会话关联的任务', consoleResource: '资源 / Provider', consoleCustom: '自定义绑定',
  consoleReopenHelp: '重新执行将使此节点及后续节点的验收与交付证据失效；Git 历史、累计用量和失败记录保留。填写原因后追加一次执行，再独立复验。', consoleManual: '人工继续一次', consoleManualReason: '继续原因', consoleManualHelp: '追加一次 Execute / L1 尝试（含有界传输重试），累计失败次数和预算继续计入',
  consoleDelivery: 'Git 交付',
  consoleInherit: '沿用默认策略', consoleBudget: '每日 Token 上限', consoleBudgetPolicy: '预算策略', consoleTimeout: '单次运行超时（毫秒）', consoleRetries: '传输重试上限', consoleRouteSession: '执行角色共用此历史策略，每个任务的 Session 仍独立', consolePermissionEffective: '阶段覆盖优先于全局策略；后端不支持的权限会明确拒绝',
  consoleTask: '任务目录', consoleCwd: '执行目录', consoleBranch: '分支', consoleRevision: '状态版本', consoleAttempts: '尝试次数', consoleFailures: '失败次数', consoleAction: '本次动作', consoleReason: '原因', consoleOutcome: '执行结果', consoleCleanup: '清理确认',
  consoleCanvas: '运行画布', consoleConfig: '全局 / 项目配置', consoleRuns: '运行与对比',
  consoleTitle: 'Flywheel 流程控制台', consoleHelp: '查看流程与任务分支，选择节点运行阶段或核对证据',
  consoleParallel: '并行执行 · 串行集成', consoleConcurrency: '最大并发任务', consolePlanSplit: 'Plan · 规划与拆分', consoleSchedule: '调度', consoleAggregate: '集成与聚合', consoleSerial: '串行调度', consoleScheduling: '调度策略', consoleRolePermissions: '按角色分配（执行可写／评审只读）', consoleScope: '配置作用域', consoleGlobal: '全局', consoleProject: '项目',
  consoleRead: '读取配置草稿', consoleSave: '保存配置', consoleDefaults: '载入本版本默认值',
  consoleNext: '保存后的配置用于下次启动；任务级配置优先于项目和全局配置',
  consoleSession: '会话策略', consoleFresh: '每次新会话 · fresh', consoleScoped: '同任务同角色续接 · scoped',
  consoleLegacy: 'Kimi Plan 历史续接 · 已启用', consoleUnknown: '未报告',
  consoleExpand: '展开 / 收起步骤', consoleDetails: '节点详情', consoleEvidence: '证据与失败原因',
  consoleStageRun: '运行此阶段', consoleStageHelp: '阶段结束后等待继续；Execute 包含有界执行与 L1 验证',
  consoleBlocked: '先停止运行并刷新状态；阶段需满足前置验收和引擎接纳条件',
  consoleExternal: '外部交付状态未关联', consoleWhole: '继续整个流程', consoleSaved: '配置已保存，请重新查看生效配置',
  consoleReload: '保存结果未确认，请重新读取配置后核对', consoleDraft: '修改草稿',
  consoleValues: '配置字段（JSON）', consolePreview: '保存差异', consoleInvalid: '请输入合法的配置 JSON',
  consolePolicies: '历史与权限', consoleFull: '完整流程 Full', consoleQuick: 'ultraWork / Quick',
  consoleHistory: '调用与返修记录', consoleNoHistory: '暂无记录', consoleIdentity: '项目与运行位置',
  consoleUnconfirmed: '结果未确认', consoleCurrent: '定位当前节点', consoleLeft: '向左移动', consoleRight: '向右移动',
  consolePage: '任务 {from}–{to} / {total}', consolePrevious: '上一组', consoleMore: '下一组',
  consoleAdapter: '执行环境 Harness', consoleModel: '模型', consoleThinking: '推理等级',
  consoleProvider: 'Provider', consoleChannel: '账号渠道', consoleScopeReason: '阶段与任务隔离；Intake / Garden 使用独立调用',
  consoleCompareHelp: '按任务、契约与配置摘要核对比较条件；用量、成本和时延是已知量，部分与缺失调用单列。费用可能按配置单价估算。调用失败率不等于代码质量。',
  consoleConfigHelp: '先读取作用域。新文件以本版本默认配置创建，现有文件保留未编辑字段',
  consoleResetHelp: '完整替换为本版本默认模板；保存前查看差异',
});
Object.assign(en, {
  consoleLastOperation: 'Latest node operation', consoleOperation_running: 'Awaiting terminal receipt', consoleOperation_passed: 'Complete', consoleOperation_failed: 'Failed', consoleOperation_cancelled: 'Cancelled', consoleSnapshotStale: 'Stale snapshot', consoleLaunchOptions: 'Task preparation and launch options', consoleTaskSummary: 'Task summary and lifetime usage', consoleMockModel: 'Offline fixture · no inference', consoleFullNode: 'Read full node evidence', consoleExcerpt: 'The canvas uses bounded excerpts; read full evidence on demand', consoleAssociated: 'Tasks associated with this session', consoleResource: 'Resource / Provider', consoleCustom: 'Custom binding',
  consoleReopenHelp: 'Reopen this and downstream units. Evidence is archived; Git history, billing and failures are retained. One execution attempt requires a reason and independent revalidation.', consoleManual: 'Continue once', consoleManualReason: 'Reason for continuing', consoleManualHelp: 'One Execute/L1 attempt including bounded transport retries; accumulated failures and budgets remain accounted',
  consoleDelivery: 'Git delivery',
  consoleInherit: 'Inherit default policy', consoleBudget: 'Daily token limit', consoleBudgetPolicy: 'Budget policy', consoleTimeout: 'Run timeout (ms)', consoleRetries: 'Transport retry limit', consoleRouteSession: 'Execution routes share this history policy; each packet still has its own session', consolePermissionEffective: 'Stage overrides take precedence; unsupported permissions are rejected',
  consoleTask: 'Task directory', consoleCwd: 'Working directory', consoleBranch: 'Branch', consoleRevision: 'State revision', consoleAttempts: 'Attempts', consoleFailures: 'Failed attempts', consoleAction: 'Action', consoleReason: 'Reason', consoleOutcome: 'Outcome', consoleCleanup: 'Cleanup',
  consoleCanvas: 'Workflow canvas', consoleConfig: 'Global / project configuration', consoleRuns: 'Runs and comparison',
  consoleTitle: 'Flywheel workflow console', consoleHelp: 'Inspect task branches, run a stage and review its evidence',
  consoleParallel: 'Parallel execution · serial integration', consoleConcurrency: 'Maximum parallel packets', consolePlanSplit: 'Plan and decomposition', consoleSchedule: 'Scheduling', consoleAggregate: 'Integration', consoleSerial: 'Serial scheduling', consoleScheduling: 'Scheduling policy', consoleRolePermissions: 'Role based: execution writes, review reads', consoleScope: 'Configuration scope', consoleGlobal: 'Global', consoleProject: 'Project',
  consoleRead: 'Read configuration draft', consoleSave: 'Save configuration', consoleDefaults: 'Load version defaults',
  consoleNext: 'Saved configuration applies on the next start; task configuration takes precedence',
  consoleSession: 'Session policy', consoleFresh: 'Fresh history per call', consoleScoped: 'Reuse history within each role scope',
  consoleLegacy: 'Kimi Plan history enabled', consoleUnknown: 'Not reported', consoleExpand: 'Expand / collapse stages',
  consoleDetails: 'Node details', consoleEvidence: 'Evidence and failures', consoleStageRun: 'Run stage',
  consoleStageHelp: 'Wait after this stage; Execute includes its bounded execution and L1 loop',
  consoleBlocked: 'Stop and refresh first; stage admission and prior verification are enforced by the engine',
  consoleExternal: 'External delivery is not associated', consoleWhole: 'Continue workflow',
  consoleSaved: 'Configuration saved; inspect effective configuration again', consoleReload: 'Save unconfirmed; reread and reconcile',
  consoleDraft: 'Edit draft', consoleValues: 'Configuration fields (JSON)', consolePreview: 'Changes to save',
  consoleInvalid: 'Enter valid configuration JSON', consolePolicies: 'History and permissions',
  consoleFull: 'Full workflow', consoleQuick: 'ultraWork / Quick', consoleHistory: 'Calls and repairs',
  consoleNoHistory: 'No records', consoleIdentity: 'Project and run location', consoleUnconfirmed: 'Unconfirmed result',
  consoleCurrent: 'Locate selected node', consoleLeft: 'Pan left', consoleRight: 'Pan right',
  consolePage: 'Tasks {from}–{to} / {total}', consolePrevious: 'Previous', consoleMore: 'Next',
  consoleAdapter: 'Harness', consoleModel: 'Model', consoleThinking: 'Reasoning effort', consoleProvider: 'Provider',
  consoleChannel: 'Account channel', consoleScopeReason: 'Histories are isolated by task and role; Intake / Garden run independently',
  consoleCompareHelp: 'Cost totals include known values only; paired cross-run quality and latency data are not connected yet',
  consoleConfigHelp: 'Read a scope first. New files start from version defaults; unedited fields in existing files are retained',
  consoleResetHelp: 'Replace with the complete version template; inspect changes before saving',
});

function sessionModeLabel(binding, t) {
  const mode = binding?.session?.mode;
  if (mode === 'scoped') return t('consoleScoped');
  if (mode === 'fresh') return t('consoleFresh');
  if (mode === 'legacy-plan' || binding?.residentSession) return t('consoleLegacy');
  return t('consoleUnknown');
}

function consoleTone(status) {
  if (['completed', 'passed', 'pass', 'l1-passed', 'executed'].includes(status)) return 'success';
  if (['executing', 'running', 'intent'].includes(status)) return 'active';
  if (['return', 'review-failed', 'paused', 'stopped'].includes(status)) return 'warning';
  if (['blocked', 'unknown', 'interrupted', 'escalated', 'budget-stopped', 'l1-failed', 'failed'].includes(status)) return 'error';
  return 'idle';
}

function ConsoleCanvas({ control, selection, run, t }) {
  const [expanded, setExpanded] = React.useState(null);
  const [selected, setSelected] = React.useState(null);
  const [page, setPage] = React.useState(0);
  const [retryReason, setRetryReason] = React.useState('');
  const [whole, setWhole] = React.useState(false);
  const viewport = React.useRef(null);
  const drag = React.useRef(null);
  const state = control.status?.state;
  const single = singleRunKind(control);
  const focused = !!single && !whole;
  const entries = Object.entries(state?.packets ?? {}).map(([id, packet]) => [id, record(packet) ? packet : {}]).sort(([a], [b]) => a.localeCompare(b));
  const pageIndex = Math.min(page, Math.max(0, Math.ceil(entries.length / 6) - 1));
  const visible = entries.slice(pageIndex * 6, pageIndex * 6 + 6);
  const quick = state?.workflow?.effective === 'quick';
  const nodes = [];
  const edges = [];
  const center = 24 + Math.max(0, visible.length - 1) * 90;
  const add = (id, title, status, x, y, packet, stage) => nodes.push({ id, title, status, x, y, packet, stage });
  if (!state) return null;
  const plan = state.milestones?.[state['current-milestone']]?.plan ?? Object.values(state.milestones ?? {}).map(m => m?.plan).filter(Boolean).sort((a,b) => (b['started-at'] || '').localeCompare(a['started-at'] || ''))[0];
  add('plan', t('consolePlanSplit'), quick ? t('notApplicable') : plan?.status ?? t('consoleUnknown'), 24, center, null, quick ? null : 'plan');
  add('schedule', t('consoleSchedule'), control.status.running ? 'running' : state.status, 272, center);
  edges.push(['plan', 'schedule']);
  for (const [index, [id, packet]] of visible.entries()) {
    add(id, id, packet.status, 520, 24 + index * 180, packet);
    edges.push(['schedule', id]);
    if (expanded === id) {
      const evidence = packet.evidence ?? {};
      const stages = [
        ['execute', t('executeRole'), packet.status === 'executing' ? 'executing' : evidence.executor ? 'executed' : 'pending'],
        ['verify', 'L1' + (quick ? ' / L2 / L3' : ''), evidence.l1?.pass === true ? 'pass' : evidence.l1?.pass === false ? 'failed' : 'pending'],
        ['review', t('reviewRole'), evidence.review?.verdict ?? 'pending'],
        ['integrate', t('consoleDelivery'), packet.status === 'completed' ? 'completed' : packet.commit?.error || packet.commit?.['merge-error'] ? 'failed' : 'pending'],
      ];
      stages.forEach(([stage, title, status], i) => {
        const key = id + '/' + stage;
        add(key, title, status, 768 + i * 248, 24 + index * 180, packet, stage);
        edges.push([i ? id + '/' + stages[i - 1][0] : id, key]);
      });
    }
  }
  const finalX = visible.some(([id]) => expanded === id) ? 2008 : 1016;
  const milestones = Object.values(state.milestones ?? {}).filter(record);
  const delivered = entries.length > 0 && entries.every(([, packet]) => packet.status === 'completed');
  const closed = milestones.length > 0 && milestones.every(milestone => milestone.status === 'closed');
  const gateFailed = milestones.some(milestone => milestone.gate?.pass === false || milestone.gate?.judge?.verdict && milestone.gate.judge.verdict !== 'pass');
  add('aggregate', t('consoleAggregate'), delivered ? 'completed' : t('consoleUnknown'), finalX - 248, center);
  add('judge', quick ? t('consoleDelivery') : 'L2 / L3 · Judge', (quick ? delivered : closed) ? 'completed' : gateFailed ? 'failed' : t('consoleUnknown'), finalX, center, null, 'final');
  const remote = control.delivery;
  const pull = remote?.pulls?.find(p => p.headRefOid === remote.head && p.state === 'OPEN') ?? remote?.pulls?.find(p => p.headRefOid === remote.head);
  add('delivery', 'PR', pull ? (pull.state === 'OPEN' ? 'running' : 'completed') : t('consoleExternal'), finalX + 248, center);
  add('merge', 'Merge', pull?.state === 'MERGED' ? 'completed' : 'pending', finalX + 496, center);
  for (const [id] of visible) edges.push([expanded === id ? id + '/integrate' : id, 'aggregate']);
  edges.push(['aggregate', 'judge']); edges.push(['judge', 'delivery']); edges.push(['delivery', 'merge']);
  let canvasWidth = finalX + 720;
  if (focused) {
    nodes.length = 0; edges.length = 0;
    const scope = state.control.packet ? entries.filter(([id]) => id === state.control.packet) : visible;
    const operationStatus = control.stale || state.control.status === 'running' && !control.status.running ? 'unknown' : state.control.status;
    if (['plan','final','integrate'].includes(single)) {
      add(single, stageName(single,t), operationStatus, 24, 24, null, single);
    } else {
      for (const [i, [id, packet]] of scope.entries()) {
        const stages = single === 'review' ? ['verify', 'review'] : single === 'execute' ? ['execute','verify'] : ['verify'];
        stages.forEach((stage, column) => {
          const evidence = packet.evidence ?? {};
          let status = stage === 'review' ? evidence.review?.verdict ?? 'unknown' : stage === 'verify' ? evidence.l1?.pass === true ? 'pass' : evidence.l1?.pass === false ? 'failed' : 'pending' : evidence.executor ? 'executed' : 'pending';
          if (stage === single && operationStatus !== 'passed') status = operationStatus;
          add(id + '/' + stage, id + ' · ' + stageName(stage,t), status, 24 + column * 248, 24 + i * 180, packet, stage);
          if (column) edges.push([id + '/' + stages[column-1], id + '/' + stage]);
        });
      }
      if (!nodes.length) add(single, stageName(single,t), 'unknown', 24, 24, null, null);
    }
    canvasWidth = single === 'review' || single === 'execute' ? 520 : 272;
  }
  const chosen = nodes.find(node => node.id === selected) ?? (focused ? nodes.find(node => node.stage === single) : null) ?? nodes.find(node => node.packet && node.packet.status !== 'completed') ?? nodes[0];
  const packetID = chosen.packet ? chosen.id.split('/')[0] : undefined;
  const detailPacket = control.node && control.node.id === packetID && control.node.revision === state.revision ? control.node.packet : chosen.packet;
  const sessions = Object.entries(state.sessions?.records ?? {}).filter(([, value]) => record(value)).filter(([, value]) =>
    !packetID || value?.scope === packetID || value?.scope === 'final:' + packetID);
  const stageBlocked = control.pending !== null || control.uncertainty !== null || control.stale ||
    control.status.running || !!state.handoff || !Number.isSafeInteger(state.revision);
  const bindingOf = node => {
    const id = node.packet ? node.id.split('/')[0] : null;
    const role = node.stage === 'review' ? 'review' : node.stage === 'plan' ? 'plan' : node.stage === 'final' && !quick ? 'judge' : node.packet && (!node.stage || node.stage === 'execute') ? 'execute' : null;
    if (!role) return null;
    const scope = id || state['current-milestone'];
    const actual = Object.values(state.sessions?.records ?? {}).filter(session => session.role === role && (!scope || session.scope === scope || session.scope === 'final:' + scope)).sort((a,b) => (b.last?.['started-at'] || '').localeCompare(a.last?.['started-at'] || ''))[0];
    if (state.run?.drivers?.[role] === 'mock') return { harness: 'mock', model: t('consoleMockModel'), permission: '', sessionMode: 'fresh' };
    return actual ? actual.configuration ?? null : state.run?.bindings?.[role === 'execute' ? 'routes.' + node.packet.route : 'loops.' + role];
  };
  const chosenBinding = bindingOf(chosen);
  const height = Math.max(240, visible.length * 180 + 40);
  const locate = () => { const current = nodes.find(node => node.stage && node.status === 'executing') ?? nodes.find(node => node.packet?.status === 'executing') ?? chosen; if (viewport.current) { viewport.current.scrollLeft = Math.max(0,current.x - 24); viewport.current.scrollTop = Math.max(0,current.y - 24); } };
  const statusText = value => Object.hasOwn(taskLabels, value) ? t(taskLabels[value]) : Object.hasOwn(packetLabels, value) ? t(packetLabels[value]) :
    ({ passed: t('pass'), stale: t('consoleSnapshotStale'), pass: t('pass'), failed: t('fail'), return: t('reviewFailed'), unknown: t('consoleUnconfirmed') }[value] ?? textValue(value));
  const detail = (label, value) => h(React.Fragment, { key: label }, h('dt', null, label), h('dd', null, textValue(value)));
  return h('section', { className: css.consoleCanvas, 'aria-label': t('consoleCanvas') },
    single && h('section', { className: css.stageScope },
      h('strong', null, singleRunLabel(control,t)), h('p', { className: css.meta }, t('stageScopeHelp')),
      h('p', { className: css.meta }, t('stageFullState', { status: t(state.status === 'completed' ? 'stageDelivered' : 'stageUndelivered') })),
      h('div', { className: css.actions },
        h(Button, { 'aria-pressed': !whole, onClick: () => setWhole(false) }, t('stageFocus')),
        h(Button, { 'aria-pressed': whole, onClick: () => setWhole(true) }, t('stageFull')))),
    h('div', { className: css.consoleToolbar },
      h('span', { className: css.meta }, (focused ? t('stageOnly', { stage: stageName(single,t) }) : t(quick ? 'consoleQuick' : 'consoleFull')) + ' · ' + t(state.scheduler?.mode === 'parallel' ? 'consoleParallel' : 'consoleSerial')),
      h('div', { className: css.actions, hidden: focused },
        h(Button, { size: 'sm', disabled: pageIndex === 0, onClick: () => setPage(pageIndex - 1) }, t('consolePrevious')),
        h('span', { className: css.meta }, t('consolePage', { from: entries.length ? pageIndex * 6 + 1 : 0, to: Math.min((pageIndex + 1) * 6, entries.length), total: entries.length })),
        h(Button, { size: 'sm', disabled: (pageIndex + 1) * 6 >= entries.length, onClick: () => setPage(pageIndex + 1) }, t('consoleMore')))),
    h('div', { className: css.actions },
      h(Button, { size: 'sm', onClick: () => { viewport.current.scrollLeft -= 300; }, 'aria-label': t('consoleLeft') }, '←'),
      h(Button, { size: 'sm', onClick: locate }, t('consoleCurrent')),
      h(Button, { size: 'sm', onClick: () => { viewport.current.scrollLeft += 300; }, 'aria-label': t('consoleRight') }, '→')),
    h('div', { className: css.canvasViewport, ref: viewport, onPointerDown: event => {
      if (event.pointerType !== 'mouse' || event.button !== 0 || event.target.closest('button')) return;
      drag.current = { x: event.clientX, scroll: viewport.current.scrollLeft };
      event.currentTarget.setPointerCapture?.(event.pointerId);
    }, onPointerMove: event => { if (drag.current) viewport.current.scrollLeft = drag.current.scroll - event.clientX + drag.current.x; },
    onPointerUp: () => { drag.current = null; }, onPointerCancel: () => { drag.current = null; }, onLostPointerCapture: () => { drag.current = null; } },
      h('div', { className: css.canvasWorld, style: { width: canvasWidth, height } },
        h('svg', { className: css.canvasEdges, width: canvasWidth, height, 'aria-hidden': true }, ...edges.map(([from, to]) => {
          const a = nodes.find(n => n.id === from), b = nodes.find(n => n.id === to);
          return h('path', { key: from + to, d: `M${a.x + 200} ${a.y + 80} H${b.x - 20} V${b.y + 80} H${b.x}` });
        })),
        ...nodes.map(node => h('button', { key: node.id, type: 'button', className: css.canvasNode,
          'data-node-id': node.id, 'data-tone': consoleTone(node.status), 'aria-pressed': chosen.id === node.id,
          style: { left: node.x, top: node.y }, onClick: () => {
            setSelected(node.id);
            if (node.packet && !node.stage) setExpanded(expanded === node.id ? null : node.id);
          } }, h('span', { className: css.nodeState }, statusText(node.status)), h('strong', null, node.title),
        h('span', { className: css.meta }, bindingOf(node) ? [bindingOf(node).harness, bindingOf(node).provider].filter(Boolean).join(' / ') : quick && node.id === 'plan' ? t('notApplicable') : ['plan','execute','review','final'].includes(node.stage) || node.packet && !node.stage ? t('consoleUnknown') : 'Go / Git'),
        h('span', { className: css.meta }, bindingOf(node) ? bindingOf(node).model || t('consoleUnknown') : node.packet?.route ?? ''),
        node.packet && !node.stage && h('span', { className: css.meta }, t('consoleExpand')))))),
    h('section', { className: css.nodeInspector, 'aria-label': t('consoleDetails') },
      h('div', { className: css.consoleToolbar }, h('h3', { className: css.heading }, chosen.title),
        h('div', { className: css.actions }, chosen.stage && h(Button, { variant: 'primary', disabled: stageBlocked || chosen.packet?.status === 'completed',
          onClick: () => run('step', selection, false, { only: chosen.stage, packet: packetID, revision: state.revision }) }, t('consoleStageRun')),
        !single && h(Button, { disabled: stageBlocked, onClick: () => run('start', selection) }, t('consoleWhole')))),
      single && h('details', { className: css.details }, h('summary', null, t('stageContinue')),
        h(Button, { disabled: stageBlocked, onClick: () => run('start', selection) }, t('consoleWhole'))),
      h('p', { className: css.meta }, t(stageBlocked ? 'consoleBlocked' : 'consoleStageHelp')),
      ['delivery', 'merge'].includes(chosen.id) && h(DeliveryPanel, { control, selection, run, t }),
      chosen.stage === 'execute' && h('details', { className: css.details }, h('summary', null, t('consoleManual')),
        h('p', { className: css.meta }, t(chosen.packet?.status === 'completed' ? 'consoleReopenHelp' : 'consoleManualHelp')),
        h('label', { className: css.editorField }, t('consoleManualReason'), h('input', { className: css.editorInput, value: retryReason, maxLength: 300, disabled: stageBlocked, onChange: event => setRetryReason(event.target.value) })),
        h(Button, { disabled: stageBlocked || !retryReason.trim(), onClick: () => run('step', selection, false, { only: 'execute', packet: packetID, revision: state.revision, retryReason: retryReason.trim() }) }, t('consoleManual'))),
      chosenBinding && h('dl', { className: css.consoleProperties }, detail(t('consoleProvider'), chosenBinding.provider || chosenBinding.harness), detail(t('consoleModel'), chosenBinding.model), detail(t('consoleThinking'), chosenBinding.thinking), detail(t('permission'), permissionLabels[chosenBinding.permission] ? t(permissionLabels[chosenBinding.permission]) : chosenBinding.permission), detail(t('consoleSession'), sessionModeLabel({ session: { mode: chosenBinding.sessionMode } }, t))),
      chosen.stage === 'review' && detailPacket?.evidence?.review && h('dl', { className: css.consoleProperties },
        detail(t('stageRecordedVerdict'), detailPacket.evidence.review.verdict), detail(t('stageRecordedScore'), detailPacket.evidence.review.score)),
      h('details', { className: css.details }, h('summary', null, t('stageMetadata')),
      h('dl', { className: css.consoleProperties },
        detail('Run', state.run?.id), detail('HEAD', state.run?.head), detail('配置 / 契约摘要', state.run?.conditions),
        detail(t('consoleTask'), control.status.taskDir), detail(t('consoleCwd'), chosen.packet?.worktree?.path ?? control.status.cwd),
        detail('依赖 / after', chosen.packet?.schedule?.after?.join(', ')), detail('写入范围 / writes', chosen.packet?.schedule?.writes?.join(', ')),
        detail(t('consoleBranch'), chosen.packet?.worktree?.branch || state.run?.branch), detail(t('consoleRevision'), state.revision),
        ...(!focused || single === 'execute' ? [detail(t('consoleAttempts'), chosen.packet?.attempts), detail(t('consoleFailures'), chosen.packet?.['failed-attempts'])] : []))),
      control.status.projection && h('p', { className: css.meta }, t('consoleExcerpt')),
      packetID && h(Button, { disabled: control.pending !== null, onClick: () => run('node', selection, false, { packet: packetID }) }, t('consoleFullNode')),
      h('details', { className: css.details }, h('summary', null, t('consoleEvidence')),
        h('pre', { className: css.raw }, JSON.stringify({ milestones: ['judge','plan'].includes(chosen.id) ? state.milestones : undefined, commit: chosen.id === 'judge' && quick ? entries.map(([id, packet]) => ({ id, commit: packet.commit })) : undefined, evidence: detailPacket?.evidence, feedback: detailPacket?.['last-feedback'], retries: detailPacket?.['retry-history'], history: detailPacket?.['evidence-history'] }, null, 2))),
      h('h4', { className: css.heading }, 'Provider sessions'),
      sessions.length ? sessions.map(([key, value]) => h(SessionInspector, { key, sessionKey: key, value, control, selection, run, t })) : h('p', { className: css.meta }, t('consoleNoHistory'))));
}

function ConfigEditor({ control, selection, run, t }) {
  const [scope, setScope] = React.useState('global');
  const [draft, setDraft] = React.useState(null);
  const [base, setBase] = React.useState(null);
  const [raw, setRaw] = React.useState('');
  const [invalid, setInvalid] = React.useState(false);
  const [reset, setReset] = React.useState(false);
  const [saved, setSaved] = React.useState(false);
  const doc = control.editor;
  React.useEffect(() => {
    if (doc && doc.scope === scope) {
      setDraft(doc.values); setBase(doc.values); setRaw(JSON.stringify(doc.values, null, 2)); setInvalid(false); setReset(false); setSaved(doc.applied);
    }
  }, [doc, scope]);
  const replace = values => { setDraft(values); setRaw(JSON.stringify(values, null, 2)); setInvalid(false); setSaved(false); };
  const update = (section, key, field, value) => {
    const next = JSON.parse(JSON.stringify(draft));
    if (!record(next[section])) next[section] = {};
    if (key === null) next[section][field] = value;
    else next[section][key][field] = value;
    replace(next);
  };
  const numeric = (section, fieldName, value, nullable = false) => {
    const number = value.trim() === '' && nullable ? null : Number(value);
    if (number !== null && (!value.trim() || !Number.isSafeInteger(number) || number < 0)) { setInvalid(true); return; }
    replace({ ...draft, [section]: { ...draft[section], [fieldName]: number } });
  };
  const busy = control.pending !== null;
  const ready = doc && doc.scope === scope && draft && !invalid;
  const field = (label, value, onChange, choices, disabled = false) => h('label', { className: css.editorField }, label,
    choices ? h('select', { value, className: css.modeSelect, disabled: busy || disabled, onChange: e => onChange(e.target.value) },
      ...choices.map(([key, name]) => h('option', { key, value: key }, name))) :
      h('input', { className: css.editorInput, value: value ?? '', disabled: busy, onChange: e => onChange(e.target.value) }));
  const resources = [['deepseek-account', 'DeepSeek Account · Flash'], ['chatgpt', 'ChatGPT · DSH'], ['kimi', 'Kimi Code · K3.1 Pro'], ['codex-pro', 'ChatGPT · Codex Pro'], ['claude-code-pro', 'Claude Code Pro'], ['custom', t('consoleCustom')]];
  const resourceOf = binding => binding.harness === 'dsh' ? (['chatgpt', 'deepseek-account'].includes(binding.provider || binding.channel) ? binding.provider || binding.channel : 'custom') : (['kimi', 'codex-pro', 'claude-code-pro'].includes(binding.harness) ? binding.harness : 'custom');
  const chooseResource = (section, key, resource) => {
    if (resource === 'custom') return;
    const defaults = doc.defaults;
    const preset = {
      'deepseek-account': defaults.routes.default,
      chatgpt: defaults.loops[key === 'judge' ? 'judge' : 'review'],
      kimi: defaults.loops.plan,
      'codex-pro': defaults.routes.hard,
      'claude-code-pro': { harness: 'claude-code-pro', model: 'sonnet', channel: 'claude-subscription', thinking: 'high' },
    }[resource];
    const next = JSON.parse(JSON.stringify(draft));
    next[section][key] = { harness: preset.harness, model: preset.model, channel: preset.channel, thinking: preset.thinking,
      ...(preset.provider ? { provider: preset.provider } : {}) };
    next.channels = { ...next.channels };
    if (!next.channels[preset.channel]) next.channels[preset.channel] = defaults.channels?.[preset.channel] ?? { 'env-key': null };
    replace(next);
  };
  const bindingOrder = ['loops.plan', 'routes.default', 'routes.frontend', 'routes.hard', 'routes.multimodal', 'loops.review', 'loops.judge', 'loops.intake', 'loops.garden'];
  const bindingRows = ['loops', 'routes'].flatMap(section => Object.entries(draft?.[section] ?? {}).filter(([, value]) => record(value)).map(([key, binding]) => [section, key, binding])).sort((a,b) => (bindingOrder.indexOf(a[0] + '.' + a[1]) + 1 || 999) - (bindingOrder.indexOf(b[0] + '.' + b[1]) + 1 || 999));
  const changed = ready ? Object.keys(draft).filter(key => JSON.stringify(draft[key]) !== JSON.stringify(base?.[key])) : [];
  return h('section', { className: css.editor, 'aria-label': t('consoleConfig') },
    h('h3', { className: css.heading }, t('consoleConfig')),
    h('p', { className: css.meta }, t('consoleConfigHelp')),
    h('div', { className: css.consoleToolbar },
      field(t('consoleScope'), scope, value => { setScope(value); setDraft(null); setBase(null); setSaved(false); }, [['global', t('consoleGlobal')], ['project', t('consoleProject')]]),
      h('div', { className: css.actions },
        h(Button, { disabled: busy, onClick: () => run('config-edit', selection, false, { editor: { scope } }) }, t('consoleRead')),
        h(Button, { disabled: !ready || busy, onClick: () => { replace(doc.defaults); setReset(true); } }, t('consoleDefaults')),
        h(Button, { variant: 'primary', disabled: !ready || busy || control.configUncertainty || (!changed.length && !reset),
          onClick: () => run('config-save', selection, false, { editor: { scope, revision: doc.revision, values: reset ? draft : Object.fromEntries(changed.map(key => [key, draft[key]])), reset } }) }, t('consoleSave')))),
    control.configUncertainty && h('p', { role: 'alert', className: css.errorText }, t('consoleReload')),
    saved && h('p', { role: 'status', className: css.meta }, t('consoleSaved')),
    draft && h(React.Fragment, null,
      h('p', { className: css.path }, doc.target), h('p', { className: css.meta }, t('consoleNext')),
      h('div', { className: css.editorGrid },
        field(t('consoleSession'), draft.sessions?.mode ?? 'fresh', value => replace({ ...draft, sessions: { ...draft.sessions, mode: value, 'policy-version': 1 } }), [['scoped', t('consoleScoped')], ['fresh', t('consoleFresh')]]),
        field(t('permission'), draft.permissions?.mode ?? 'role-based', value => replace({ ...draft, permissions: { ...draft.permissions, mode: value } }), [['role-based', t('consoleRolePermissions')], ['full-access', t('permissionFull')]]),
        field(t('consoleScheduling'), draft.scheduler?.mode ?? 'serial', value => replace({ ...draft, scheduler: { 'max-parallel': 2, ...draft.scheduler, mode: value } }), [['serial', t('consoleSerial')], ['parallel', t('consoleParallel')]]),
        field(t('consoleConcurrency'), draft.scheduler?.['max-parallel'] ?? 2, value => numeric('scheduler', 'max-parallel', value)),
        field(t('consoleBudget'), draft.budget?.['daily-token-limit'], value => numeric('budget', 'daily-token-limit', value, true)),
        field(t('consoleBudgetPolicy'), draft.budget?.enforce ?? 'soft', value => update('budget', null, 'enforce', value), [['off', 'off'], ['soft', 'soft'], ['hard', 'hard']]),
        field(t('consoleTimeout'), draft.watchdog?.['wall-timeout-ms'], value => numeric('watchdog', 'wall-timeout-ms', value)),
        field(t('consoleRetries'), draft.retry?.['max-transport-retries'], value => numeric('retry', 'max-transport-retries', value))),
      h('h4', { className: css.heading }, t('consoleJEV')),
      h('p', { className: css.meta }, t('consoleJEVHelp')),
      h('div', { className: css.editorGrid },
        field(t('consoleJEV'), String(draft['jev-shadow']?.enabled ?? false), value => update('jev-shadow', null, 'enabled', value === 'true'), [['false', t('consoleJEVOff')], ['true', t('consoleJEVOn')]]),
        field('System One endpoint', draft['jev-shadow']?.endpoint, value => update('jev-shadow', null, 'endpoint', value)),
        field(t('consoleModel'), draft['jev-shadow']?.model ?? 'jev', value => update('jev-shadow', null, 'model', value)),
        field(t('consoleJEVVersion'), draft['jev-shadow']?.['expected-model'] ?? 'jev-1.14.0', value => update('jev-shadow', null, 'expected-model', value)),
        field(t('consoleJEVEnv'), draft['jev-shadow']?.['key-env'] ?? 'MEROUTER_API_KEY', value => update('jev-shadow', null, 'key-env', value)),
        field(t('consoleJEVTimeout'), draft['jev-shadow']?.['timeout-ms'] ?? 5000, value => numeric('jev-shadow', 'timeout-ms', value))),
      h('div', { className: css.editorBindings }, ...bindingRows.map(([section, key, binding]) =>
        h('article', { key: section + key, className: css.editorBinding, 'data-binding-id': section + '.' + key }, h('h4', { className: css.heading }, (section === 'routes' ? t('executeRole') : t(roleLabels[key] || key)) + ' · ' + key), h('span', { className: css.meta }, section + '.' + key),
          h('div', { className: css.editorGrid },
            field(t('consoleResource'), resourceOf(binding), value => chooseResource(section, key, value), resources),
            field(t('permission'), draft.permissions?.bindings?.[section + '.' + key] ?? 'inherit', value => {
              const bindings = { ...draft.permissions?.bindings };
              if (value === 'inherit') delete bindings[section + '.' + key]; else bindings[section + '.' + key] = value;
              replace({ ...draft, permissions: { ...draft.permissions, bindings } });
            }, [['inherit', t('consoleInherit')], ['read-only', t('permissionReadOnly')], ['workspace-write', t('permissionWorkspace')], ['danger-full-access', t('permissionFull')]]),
            field(t('consoleAdapter'), binding.harness, v => update(section, key, 'harness', v), ['dsh', 'kimi', 'codex-pro', 'claude-code-pro'].concat(['dsh', 'kimi', 'codex-pro', 'claude-code-pro'].includes(binding.harness) ? [] : [binding.harness]).map(v => [v, v])),
            field(t('consoleModel'), binding.model, v => update(section, key, 'model', v)),
            field(t('consoleThinking'), binding.thinking, v => update(section, key, 'thinking', v)),
            field(t('consoleProvider'), binding.provider, v => update(section, key, 'provider', v)),
            field(t('consoleChannel'), binding.channel, v => update(section, key, 'channel', v)),
            field(t('consoleSession'), draft.sessions?.roles?.[section === 'routes' ? 'execute' : key] ?? 'inherit', value => {
              const roles = { ...draft.sessions?.roles };
              const role = section === 'routes' ? 'execute' : key;
              if (value === 'inherit') delete roles[role]; else roles[role] = value;
              replace({ ...draft, sessions: { ...draft.sessions, roles } });
            }, [['inherit', t('consoleInherit')], ['scoped', t('consoleScoped')], ['fresh', t('consoleFresh')]], ['intake', 'garden'].includes(key))),
          h('p', { className: css.meta }, t('consolePermissionEffective')), section === 'routes' && h('p', { className: css.meta }, t('consoleRouteSession'))))),
      h('details', { className: css.details }, h('summary', null, t('consoleValues')),
        h('textarea', { className: css.editorJSON, value: raw, disabled: busy, 'aria-label': t('consoleValues'), 'aria-invalid': invalid,
          onChange: event => { setRaw(event.target.value); try { const value = JSON.parse(event.target.value); if (!record(value)) throw new Error(); setDraft(value); setInvalid(false); setSaved(false); } catch { setInvalid(true); } } })),
      invalid && h('p', { role: 'alert', className: css.errorText }, t('consoleInvalid')),
      h('section', { className: css.section, 'aria-label': t('consolePreview') }, h('h4', { className: css.heading }, t('consolePreview')),
        reset && h('p', { className: css.warningText }, t('consoleResetHelp')),
        ...changed.map(key => h('details', { key, className: css.details }, h('summary', null, key),
          h('pre', { className: css.raw }, JSON.stringify({ before: base?.[key], after: draft[key] }, null, 2)))))));
}

Object.assign(zh, { runsObserved: '本轮新增交付 {completed} · 执行尝试 {attempts} · 失败尝试 {failed} · 总耗时 {seconds} s', runsPartial: '部分数据 {count} 次', runsBounded: '每个已显示分组均汇总其全部调用；明细最多 200 条、分组最多 500 个。按 run 筛选可查看其完整汇总。', runsAll: '当前任务全部调用（含早期记录）', runsRefresh: '刷新最近运行', runsOpen: '在画布中打开', runsMetrics: '读取所选运行指标', runsCompare: '比较条件', runsEqual: '相同契约与配置', runsDifferent: '条件不同，仅并列展示', runsLegacy: '早期调用（未登记 run ID）', runsSamples: '调用 / 失败', runsLatency: '已知调用耗时', runsUsage: '已知 Token / 缺失调用', runsCost: '已知费用 / 缺失调用', runsQuality: '验收质量见各节点证据；模型质量需要独立 oracle', runsEmpty: '暂无已登记运行。CLI 启动或桌面启动后会自动登记。', runsReference: '对照运行', runsLive: '尚无退出回执，打开画布核对', runsReadOnly: '登记记录用于发现；当前运行状态以刷新后的引擎回执为准。' });
Object.assign(en, { runsObserved: 'New deliveries {completed} · attempts {attempts} · failed attempts {failed} · elapsed {seconds} s', runsPartial: 'partial {count}', runsBounded: 'Each displayed group aggregates all its calls. Details are limited to 200 calls and 500 groups; filter by run for its full summary.', runsAll: 'All calls for this task, including legacy records', runsRefresh: 'Refresh recent runs', runsOpen: 'Open canvas', runsMetrics: 'Read selected run metrics', runsCompare: 'Comparison conditions', runsEqual: 'Same contracts and configuration', runsDifferent: 'Different conditions; descriptive comparison only', runsLegacy: 'Legacy call without run ID', runsSamples: 'Calls / failed', runsLatency: 'Known call duration', runsUsage: 'Known tokens / missing calls', runsCost: 'Known cost / missing calls', runsQuality: 'Validation quality is recorded in node evidence; model quality needs an independent oracle', runsEmpty: 'No registered runs. CLI and desktop launches register automatically.', runsReference: 'Reference run', runsLive: 'No exit receipt; open canvas to reconcile', runsReadOnly: 'The index is for discovery; refresh engine status to establish liveness.' });

function RunExplorer({ control, selection, run, changeSelection, showCanvas, t }) {
  const [reference, setReference] = React.useState('');
  const runs = control.runs ?? [];
  const baseline = reference ? runs.find(item => item.id === reference) : null;
  const busy = control.pending !== null;
  return h('section', { className: css.section, 'aria-label': t('consoleRuns') },
    h('div', { className: css.consoleToolbar }, h('h3', { className: css.heading }, t('consoleRuns')),
      h('div', { className: css.actions },
        h(Button, { disabled: busy, onClick: () => run('runs', selection) }, t('runsRefresh')),
        h(Button, { disabled: busy || (!selection.taskDir && !baseline), onClick: () => { const next = baseline ? { taskDir: baseline.taskDir, cwd: baseline.cwd } : selection; if (baseline) changeSelection(next); return run('metrics', next, false, { runId: baseline?.id }); } }, t('runsMetrics')))),
    h('p', { className: css.meta }, t('runsReadOnly')),
    runs.length ? h(React.Fragment, null,
      h('label', { className: css.editorField }, t('runsReference'), h('select', { className: css.modeSelect, value: baseline?.id ?? '', onChange: event => setReference(event.target.value) },
        h('option', { value: '' }, t('runsAll')),
        ...runs.map(item => h('option', { key: item.id, value: item.id }, item.summary?.slug + ' · ' + item.startedAt)))),
      h('div', { className: css.editorBindings }, ...runs.slice(0,100).map(item => h('article', { key: item.id, className: css.editorBinding },
        h('div', { className: css.consoleToolbar }, h('h4', { className: css.heading }, item.summary?.slug ?? item.taskDir),
          h(Button, { disabled: busy || control.uncertainty !== null, onClick: async () => {
            const next = { taskDir: item.taskDir, cwd: item.cwd };
            changeSelection(next); showCanvas(); await run('status', next);
          } }, t('runsOpen'))),
        h('p', { className: css.path }, item.cwd),
        h('p', { className: css.meta }, [item.kind, item.branch, item.head?.slice(0,12)].filter(Boolean).join(' · ')),
        item.quality && item.finishedAt && !item.error && h('p', { className: css.meta }, t('runsObserved', { completed: item.quality.newCompletions, attempts: item.quality.newAttempts, failed: item.quality.newFailedAttempts, seconds: item.elapsedMs == null ? t('consoleUnknown') : (item.elapsedMs / 1000).toFixed(1) })),
        h('p', { className: css.meta }, item.startedAt + ' · ' + (item.finishedAt ? item.error ? t('fail') : (taskLabels[item.summary?.status] ? t(taskLabels[item.summary.status]) : item.summary?.status) : t('runsLive'))),
        h('p', { className: css.meta }, t('runsCompare') + ' · ' + (baseline ? t(item.conditions === baseline.conditions ? 'runsEqual' : 'runsDifferent') : textValue(item.conditions).slice(0,12))),
        h('details', { className: css.details }, h('summary', null, 'Run / ' + item.id), h('pre', { className: css.raw }, JSON.stringify(item, null, 2))))))) : h('p', { className: css.meta }, t('runsEmpty')),
    h('p', { className: css.meta }, t('consoleCompareHelp')),
    h('p', { className: css.meta }, t('runsQuality')),
    !!(control.metrics?.omittedGroups || control.metrics?.omittedCalls) && h('p', { className: css.meta }, t('runsBounded')),
    h('div', { className: css.editorBindings }, ...(control.metrics?.groups ?? []).map((group, index) => h('article', { key: index, className: css.editorBinding },
      h('h4', { className: css.heading }, [group.role, group.harness, group.model].filter(Boolean).join(' · ')),
      h('p', { className: css.path }, group.runId || t('runsLegacy')),
      h('dl', { className: css.consoleProperties },
        h('dt', null, t('runsSamples')), h('dd', null, group.calls + ' / ' + group.failedCalls),
        h('dt', null, t('runsLatency')), h('dd', null, (group.knownDurationMs / 1000).toFixed(1) + ' s · ' + group.missingDuration + ' unknown'),
        h('dt', null, t('runsUsage')), h('dd', null, group.knownTokens + ' / ' + group.missingUsage + ' · ' + t('runsPartial', { count: group.partialUsage ?? 0 })),
        h('dt', null, t('runsCost')), h('dd', null, '$' + group.knownCostUSD.toFixed(4) + ' / ' + group.missingCost))))));
}

Object.assign(zh, { sessionAction_create: '新建', sessionAction_resume: '续接', sessionAction_rebuild: '重建', sessionAction_wait: '等待核对', sessionAction_reconcile: '核对回执', sessionRelease: '退出接管 · 保持暂停', sessionReconcile: '核对原会话回执', sessionLatest: '查看最近回复', sessionLatestHelp: '这里读取经过摘要校验的最近一次回复；Provider 的完整会话历史仍保存在原后端。', sessionClaim: '暂停并接管会话', sessionReason: '接管原因', sessionSend: '发送到原 Session', sessionReturn: '验证并交回流程', sessionPrompt: '人工 Prompt', sessionHeld: '人工接管中 · 自动执行已锁定', sessionCommand: '终端交互命令', sessionNoClaim: '仅已确认收尾、可恢复的任务会话支持接管；里程碑会话可查看回复。', sessionKeep: '离开页面不会释放接管；测试和独立 Review 通过后才交回。' });
Object.assign(en, { sessionAction_create: 'Create', sessionAction_resume: 'Resume', sessionAction_rebuild: 'Rebuild', sessionAction_wait: 'Await reconciliation', sessionAction_reconcile: 'Reconcile', sessionRelease: 'Release control and remain paused', sessionReconcile: 'Reconcile original invocation', sessionLatest: 'Read latest response', sessionLatestHelp: 'This is the latest digest-verified response, not the provider’s full history.', sessionClaim: 'Take over session', sessionReason: 'Takeover reason', sessionSend: 'Send to original session', sessionReturn: 'Validate and return', sessionPrompt: 'Human prompt', sessionHeld: 'Human control · automatic execution locked', sessionCommand: 'Terminal command', sessionNoClaim: 'Only settled, resumable packet sessions support interactive takeover. Milestone history remains readable.', sessionKeep: 'Leaving the page retains human control until validation and independent review pass.' });
const shellQuote = value => "'" + String(value).replaceAll("'", "'\\''") + "'";
function SessionInspector({ sessionKey, value, control, selection, run, t }) {
  const [reason, setReason] = React.useState('');
  const [prompt, setPrompt] = React.useState('');
  const state = control.status.state;
  const held = state.handoff?.key === sessionKey;
  const busy = control.pending !== null || control.status.running || control.stale || control.uncertainty !== null;
  const canClaim = !state.handoff && value['resume-candidate'] && value.last?.status === 'consumed' && value.last?.['cleanup-confirmed'] && /^(final:)?M\d+-T\d+$/.test(value.scope);
  const send = action => run('session', selection, false, { session: { action, key: sessionKey, revision: state.revision, ...(action === 'claim' ? { reason } : action === 'send' ? { prompt } : {}) } });
  const history = control.history?.key === sessionKey ? control.history : null;
  return h('article', { className: css.sessionRecord },
    h('h4', { className: css.heading }, (roleLabels[value.role] ? t(roleLabels[value.role]) : value.role) + ' / ' + value.scope),
    h('p', { className: css.path }, value['thread-id'] || t('consoleUnknown')),
    h('p', { className: css.meta }, [t('sessionAction_' + value.action), value.reason, value.last?.outcome].filter(Boolean).join(' · ')),
    h('div', { className: css.actions }, h(Button, { disabled: control.pending !== null, onClick: () => run('history', selection, false, { sessionKey }) }, t('sessionLatest'))),
    history && h('details', { className: css.details, open: true }, h('summary', null, t('sessionLatest')), h('p', { className: css.meta }, t('sessionLatestHelp')), h('pre', { className: css.raw }, history.text)),
    held ? h('div', null, h('p', { role: 'status', className: css.notice }, t('sessionHeld')), h('p', { className: css.meta }, t('sessionKeep')),
      h('label', { className: css.editorField }, t('sessionPrompt'), h('textarea', { className: css.editorJSON, value: prompt, disabled: busy, maxLength: 16000, onChange: event => setPrompt(event.target.value) })),
      h('div', { className: css.actions }, h(Button, { disabled: busy || state.handoff?.pending || !prompt.trim(), onClick: () => send('send') }, t('sessionSend')), h(Button, { disabled: busy, onClick: () => send('reconcile') }, t('sessionReconcile')), h(Button, { disabled: busy || state.handoff?.pending, onClick: () => send('return') }, t('sessionReturn')), h(Button, { disabled: busy || state.handoff?.pending, onClick: () => send('release') }, t('sessionRelease')))) :
      h('details', { className: css.details }, h('summary', null, t('sessionClaim')), h('p', { className: css.meta }, t('sessionNoClaim')),
        h('label', { className: css.editorField }, t('sessionReason'), h('input', { className: css.editorInput, disabled: busy || !canClaim, value: reason, maxLength: 300, onChange: event => setReason(event.target.value) })),
        h(Button, { disabled: busy || !canClaim || !reason.trim(), onClick: () => send('claim') }, t('sessionClaim'))),
    /^(final:)?M\d+-T\d+$/.test(value.scope) && h('details', { className: css.details }, h('summary', null, t('sessionCommand')),
      h('code', { className: css.path }, 'cd -- ' + shellQuote(control.status.cwd || selection.cwd || value.cwd) + ' && flywheel session ' + shellQuote(control.status.taskDir) + ' --key ' + shellQuote(sessionKey))));
}

Object.assign(zh, { deliveryRefresh: '查询项目与 PR', deliveryRepo: '目标仓库 / 分支 / HEAD', deliveryConfirm: '授权按当前 HEAD 推送并创建 PR，或合并选定 PR', deliveryCreate: '推送并创建 PR', deliveryMerge: '合并此 PR', deliveryHead: '远端任务分支', deliveryBase: '目标分支', deliveryTitle: 'PR 标题', deliveryBody: 'PR 说明', deliveryHelp: '远端写入要求本地任务完成、当前候选仍通过验收，并在执行前再次核对 revision 和 HEAD。未确认的结果先查询；不会强推或绕过 GitHub 合并规则。', deliveryStale: '远端快照已过期，请重新查询', deliveryPending: '合并请求已接收，等待远端最终结果', deliveryChecks: '远端检查', deliveryNoChecks: '未配置或未报告检查', deliveryUnknown: '尚未查询远端；本地完成与远端合并分别记录' });
Object.assign(en, { deliveryRefresh: 'Inspect project and PRs', deliveryRepo: 'Repository / branch / HEAD', deliveryConfirm: 'Authorize push/create or merge at this exact HEAD', deliveryCreate: 'Push and create PR', deliveryMerge: 'Merge PR', deliveryHead: 'Remote task branch', deliveryBase: 'Base branch', deliveryTitle: 'PR title', deliveryBody: 'PR description', deliveryHelp: 'Remote writes require completed local acceptance and a matching revision and HEAD. Reconcile uncertain results first. No force push or GitHub rule bypass.', deliveryStale: 'Remote snapshot is stale; inspect again', deliveryPending: 'Merge requested; awaiting the final remote result', deliveryChecks: 'Remote checks', deliveryNoChecks: 'No configured or reported checks', deliveryUnknown: 'Remote state has not been queried; local completion and remote merge are separate' });
function DeliveryPanel({ control, selection, run, t }) {
  const [confirmed, setConfirmed] = React.useState(false);
  const [base, setBase] = React.useState('');
  const [headBranch, setHeadBranch] = React.useState('');
  const [title, setTitle] = React.useState('');
  const [body, setBody] = React.useState('');
  const remote = control.delivery;
  React.useEffect(() => { setBase(remote?.defaultBase ?? ''); setHeadBranch(remote ? remote.branch === remote.defaultBase ? 'flywheel/' + (control.status?.state?.slug || 'task') : remote.branch : ''); }, [remote?.repo, remote?.head]);
  React.useEffect(() => { setConfirmed(false); }, [remote?.repo, remote?.head, base, headBranch]);
  const blocked = control.pending !== null || control.status?.running || control.uncertainty !== null;
  const writeBlocked = blocked || control.stale || control.deliveryStale || !confirmed || !remote || control.status?.state?.status !== 'completed';
  const deliver = (action, number) => run('deliver', selection, false, { delivery: { action, confirmed: true, revision: control.status.state.revision, expectedHead: remote.head, expectedRepo: remote.repo, headBranch, ...(action === 'create' ? { base, title, body } : { number, base: remote.pulls.find(pull => pull.number === number)?.baseRefName }) } });
  return h('section', { className: css.section, 'aria-label': 'PR / Merge' },
    h('div', { className: css.actions }, h(Button, { disabled: blocked, onClick: () => run('delivery', selection, false, { delivery: { action: 'inspect', ...(headBranch ? { headBranch } : {}) } }) }, t('deliveryRefresh'))),
    h('p', { className: css.meta }, t('deliveryHelp')),
    control.status?.state?.delivery?.action === 'merge' && control.status.state.delivery.status === 'pending' && h('p', { role: 'status' }, t('deliveryPending')),
    control.deliveryStale && h('p', { role: 'alert' }, t('deliveryStale')),
    remote ? h(React.Fragment, null,
      h('a', { href: remote.url, target: '_blank', rel: 'noopener noreferrer' }, remote.repo),
      h('p', { className: css.path }, (remote.localBranch || remote.branch) + ' → ' + remote.branch + ' · ' + remote.head),
      h('label', { className: css.confirmation }, h('input', { type: 'checkbox', checked: confirmed, disabled: blocked, onChange: event => setConfirmed(event.target.checked) }), t('deliveryConfirm')),
      h('div', { className: css.editorGrid },
        h('label', { className: css.editorField }, t('deliveryHead'), h('input', { className: css.editorInput, value: headBranch, disabled: blocked, onChange: event => setHeadBranch(event.target.value) })),
        h('label', { className: css.editorField }, t('deliveryBase'), h('input', { className: css.editorInput, value: base, disabled: blocked, onChange: event => setBase(event.target.value) })),
        h('label', { className: css.editorField }, t('deliveryTitle'), h('input', { className: css.editorInput, value: title, maxLength: 256, disabled: blocked, onChange: event => setTitle(event.target.value) }))),
      h('label', { className: css.editorField }, t('deliveryBody'), h('textarea', { className: css.editorJSON, value: body, disabled: blocked, maxLength: 16000, onChange: event => setBody(event.target.value) })),
      h(Button, { disabled: writeBlocked || !title.trim() || !base.trim() || !headBranch.trim() || headBranch === base, onClick: () => deliver('create') }, t('deliveryCreate')),
      ...remote.pulls.map(pull => h('article', { className: css.sessionRecord, key: pull.number },
        h('a', { href: pull.url, target: '_blank', rel: 'noopener noreferrer' }, '#' + pull.number + ' · ' + pull.state),
        h('p', { className: css.meta }, [pull.baseRefName, pull.mergeStateStatus, pull.reviewDecision].filter(Boolean).join(' · ')),
        h('details', { className: css.details }, h('summary', null, t('deliveryChecks')),
          pull.statusCheckRollup?.length ? pull.statusCheckRollup.map((check,index) => h('p', { key: index, className: css.meta }, [check.name || check.context, check.conclusion || check.state || check.status].filter(Boolean).join(' · '))) : h('p', { className: css.meta }, t('deliveryNoChecks'))),
        h(Button, { disabled: writeBlocked || pull.headRefOid !== remote.head || pull.state !== 'OPEN' || pull.isDraft || pull.mergeStateStatus !== 'CLEAN', onClick: () => deliver('merge', pull.number) }, t('deliveryMerge'))))) : h('p', { className: css.meta }, t('deliveryUnknown')));
}

Object.assign(zh, { consoleJEV: 'JEV 影子评估（可选）', consoleJEVOff: '关闭 · 沿用原流程', consoleJEVOn: '开启 · 只记录建议', consoleJEVVersion: '固定响应模型版本', consoleJEVEnv: '密钥环境变量名（不要填密钥）', consoleJEVTimeout: '请求时限（毫秒，最多 5000）', consoleJEVHelp: '默认关闭。开启后，新任务最多发送一次目标与验收摘要，独立记录用量与建议，不改变 Quick / Full。缺少密钥、超时或服务故障不阻断任务；硬预算任务跳过。桌面启动的引擎需能读取该环境变量。摘要可能含业务信息，请确认 endpoint 后再开启。' });
Object.assign(en, { consoleJEV: 'JEV shadow evaluation (optional)', consoleJEVOff: 'Off · existing workflow', consoleJEVOn: 'On · advice only', consoleJEVVersion: 'Pinned response model', consoleJEVEnv: 'API key environment variable name (not the key)', consoleJEVTimeout: 'Request timeout (ms, at most 5000)', consoleJEVHelp: 'Off by default. Enabling sends one goal and acceptance summary per new task, with separate usage and advisory records. Quick / Full is unchanged. Missing credentials, timeouts and service failures do not block tasks; hard-budget tasks skip observation. The desktop engine must inherit the named variable. Summaries may contain business details; review the endpoint before enabling.' });
