// The build wraps this file in DSH's lazy browser ModuleLoader factory.
// React is the only runtime module dependency. External plain-JS plugins must
// not import Harness Client packages (DSH practices, b830ec950faba0a46dda564be3e2d854afed94f1).
const React = require('react');
const h = React.createElement;
const NS = 'flywheel-ui';
// build.mjs supplies scoped CSS; React owns its style element inside this view.
const css = __FLYWHEEL_CSS_CLASSES__;
const stylesheet = __FLYWHEEL_CSS_TEXT__;
// Host controlTimeoutMs is bounded at 120 s, with up to 20 s diagnostic cleanup.
// A browser timeout reports uncertainty; it cannot cancel a detached Go task.
const CONTROL_WAIT_MS = 150000;

// Own the button control we use. Markup and native-prop/ref forwarding follow
// DSH b830ec950faba0a46dda564be3e2d854afed94f1:
// packages/client/ui-primitives/src/Button.tsx. Scoped styles below copy
// their compact geometry; only public alias colors are shared with the host.
// The copied portions retain their MIT notice in THIRD_PARTY_NOTICES.md.
const Button = React.forwardRef(function FlywheelButton({
  variant = 'outline', size = 'sm', className, children, ...rest
}, ref) {
  return h('button', { type: 'button', ...rest, ref,
    className: [css.button, variant === 'primary' ? css.buttonPrimary : css.buttonOutline,
      size === 'sm' ? css.buttonSmall : '', className].filter(Boolean).join(' '),
  }, children);
});

// The ongoing glyph and reduced-motion behavior are copied from the same
// pinned host's ui-primitives StateDot; this plugin owns the SVG and CSS.
function Spinner() {
  const synchronize = element => {
    if (!element) return;
    for (const animation of element.getAnimations?.({ subtree: true }) ?? []) animation.startTime = 0;
  };
  return h('svg', { className: css.spinner, width: 20, height: 20,
    viewBox: '0 0 24 24', 'aria-hidden': true, ref: synchronize,
    onAnimationStart: event => synchronize(event.currentTarget) },
  h('g', { className: css.spinnerMotion },
    h('circle', { className: css.spinnerTrack, cx: 12, cy: 12, r: 9.5 }),
    h('circle', { className: css.spinnerArc, cx: 12, cy: 12, r: 9.5 })));
}

// A small immutable snapshot source for this plugin's flat write sets. This is
// not a generic deep-draft engine: all updates replace root fields, and nested
// engine reports are retained as read-only values. Only the host subscribes.
function createSnapshot(initial) {
  let snapshot = Object.freeze({ ...initial });
  const listeners = new Set();
  return {
    getSnapshot: () => snapshot,
    subscribe: listener => {
      listeners.add(listener);
      return () => { listeners.delete(listener); };
    },
    update: mutate => {
      const next = { ...snapshot };
      mutate(next);
      if (Object.keys(next).every(key => Object.is(next[key], snapshot[key]))) return;
      snapshot = Object.freeze(next);
      for (const listener of [...listeners]) listener();
    },
  };
}

// Public structural StoreHandle / StoreInstance contract, verified at pinned
// packages/client/store/src/contract.ts. The renderer creates and caches each
// instance by session binding, then provides useStore/actions to the component.
// We do not import its engine, subscribe from React, or persist live state.
function createViewStore() {
  const spec = {
    init: () => ({ taskDir: '', cwd: '', mode: 'auto' }),
    actions: {
      setSelection: (draft, next) => { draft.taskDir = next.taskDir; draft.cwd = next.cwd; },
      setMode: (draft, mode) => { draft.mode = mode; },
    },
  };
  return {
    spec,
    create: () => {
      const source = createSnapshot(spec.init());
      const actions = Object.fromEntries(Object.entries(spec.actions).map(([name, action]) =>
        [name, (...args) => source.update(draft => action(draft, ...args))]));
      return { actions, getSnapshot: source.getSnapshot, subscribe: source.subscribe,
        clearPersisted: () => {},
      };
    },
  };
}

const zh = {
  view: 'Flywheel',
  title: 'Flywheel 任务',
  description: '准备任务后，在这里查看进度、启动、继续或停止',
  lifecycle: '任务在后台独立运行。切换视图或会话不会停止任务；需要停止时请使用这里的“停止”',
  selection: '打开已有任务',
  taskDir: '任务资料目录',
  taskHint: '填写助手准备任务时给出的完整路径，里面保存任务说明和验收清单；只查看配置或做检查时可留空',
  cwd: '项目目录（可选）',
  cwdHint: '留空时使用当前 DSH 会话的工作目录。若它不是本任务的项目根目录，填写助手给出的项目完整路径',
  pathInvalid: '请填写以 / 开头的完整路径',
  pathControl: '路径不能包含换行或空字符',
  taskRequired: '填写任务资料目录后，可刷新状态、启动或停止任务',
  status: '刷新状态',
  start: '启动',
  launch: '启动准备好的任务',
  launchMode: '新任务流程',
  launchHelp: 'Auto 由引擎判断 Quick 或 Full；继续已有任务使用“启动”，沿用已保存的流程',
  workflow: '执行流程',
  requestedMode: '请求：{mode}',
  effectiveMode: '生效：{mode}',
  workflowReason: '选择原因：{reason}',
  legacyFull: '这个任务未保存自适应流程信息，按原有 Full 流程运行',
  applicable: '适用',
  notApplicable: '不适用',
  upgrades: '流程升级记录',
  upgrade: '{from} → {to}：{reason}',
  finalReview: 'Quick 仍须通过验收、最终检查与独立的任务级 Review；Plan / Judge 不适用不代表通过',
  associationUnconfirmed: '会话记录中存在任务请求；引擎结果尚未确认',
  stop: '停止',
  config: '查看模型配置',
  doctor: '检查安装与配置',
  probe: '检查 DSH 接入',
  doctorHelp: '检查安装与配置：核对本机工具、模型配置和所选任务文件，不会启动任务',
  probeHelp: '检查 DSH 接入：临时启动独立的 DSH 运行环境，检查配置、项目目录和权限后退出；可能联网读取模型目录',
  busy: '正在等待“{action}”确认…',
  busyOther: '正在等待上一个目录的“{action}”结束；其结果不会覆盖当前选择',
  mutationLock: '操作结果确认前，保持当前任务关联并暂停后续操作',
  unknownOutcome: '“{action}”结果未确认',
  unknownHelp: '请求可能已经送达引擎。先刷新这个任务的状态，再决定下一步操作',
  reconcile: '刷新待确认任务',
  errorTitle: '“{action}”未完成',
  errorHelp: '检查连接和目录后，重新执行这项只读检查',
  notDispatchedTitle: '“{action}”未发送到引擎',
  notDispatchedHelp: '检查错误详情，修正目录或引擎配置后重试',
  errorDetails: '错误详情',
  feedback: '操作反馈',
  statusHeading: '任务状态',
  emptyHeading: '还没有任务状态',
  emptyBody: '在“对话”输入 /flywheel-task 和需求。助手准备并启动任务后，会自动关联到本会话，无需复制路径。/flywheel-new 仍可只准备任务；也可填写已有任务目录后操作',
  emptySelected: '点击“刷新状态”查看这个任务；打开本页不会自动启动任务',
  lastChecked: '上次确认：{time}',
  stale: '本次状态未确认，下方保留上次结果',
  stateUnconfirmed: '状态待确认',
  previousState: '上次确认状态：{status}',
  task: '任务',
  running: '运行中',
  uninitialized: '尚未初始化',
  engineMissing: '引擎未运行，状态待核对',
  unknownState: '未知状态（{status}）',
  unknown: '未知',
  unreported: '未报告',
  pending: '待开始',
  paused: '已暂停',
  stopped: '已停止',
  completed: '已完成',
  escalated: '需要人工处理',
  budgetStopped: '预算已停止',
  milestone: '当前里程碑：{name}',
  packets: '工作单元',
  packetsCount: '工作单元（共 {count} 个）',
  packet: '工作单元',
  state: '状态',
  route: '路由',
  packetPending: '待执行',
  awaitingCredentials: '等待凭证',
  executing: '执行中',
  executed: '待验收',
  l1Passed: '验收已通过',
  l1Failed: '验收未通过',
  reviewFailed: '评审退回',
  interrupted: '已中断',
  inbox: '等待处理',
  inboxCount: '待人工处理记录：{count} 条',
  budget: '用量与成本',
  cost: '已报告成本合计：$' + '{amount}',
  costUnknown: '成本：未知',
  costPartial: '已知成本合计：$' + '{amount}；另有 {count} 次调用成本未知，合计不完整',
  costMissing: '成本：未知（{count} 次调用未报告结算成本）',
  tokens: '已报告 Token：{tokens}',
  tokensMissing: '；{count} 次调用用量未知，合计不完整',
  tokenLimit: '每日 Token 限额：{limit} · 预算策略：{policy}',
  budgetWarning: '预算提示：{message}',
  advanced: '配置与排查（按需）',
  advancedHelp: '首次安装、升级或运行失败时可用，平时不必每次点击；两项检查都不向模型发送消息，也不能验证账号、额度或实际回答能力',
  configuration: '实际模型配置',
  configurationHelp: '这里显示 Flywheel 从下方来源读取的实际生效配置；DSH 对话输入框当前选择的模型不会替代它',
  source: '配置来源：{source}',
  embedded: '内嵌模板',
  scope: '配置层级：{scope}',
  outOfScope: '未用于本次检查',
  model: '模型',
  profile: 'Profile',
  harness: 'Harness',
  provider: 'Provider',
  channel: '渠道',
  thinking: '推理',
  permission: '阶段权限要求',
  permissionFull: '完全权限（无需逐步确认）',
  permissionWorkspace: '项目内写入',
  permissionReadOnly: '只读',
  session: '会话策略',
  step: '运行此阶段', 'config-edit': '读取配置', 'config-save': '保存配置',
  enabled: '启用',
  disabled: '关闭',
  notConfigured: '未配置',
  nativeProvider: '由原生运行时管理',
  default: '默认',
  undeclared: '未声明',
  planRole: '规划',
  executeRole: '执行',
  reviewRole: '评审',
  judgeRole: '终验',
  intakeRole: '需求收集',
  gardenRole: '知识维护',
  panelRole: '第二意见',
  diagnostic: '环境检查结果',
  doctorPassed: '检查结束：未发现阻断项',
  doctorFailed: '检查结束：存在阻断项',
  localOnly: '本次检查本机工具、配置和所选任务文件，未启动 DSH 接入检查',
  probeRequested: '已请求 DSH 接入检查（不调用模型）',
  inferenceBoundary: '没有向模型发送消息；账号授权、可用额度和实际回答能力仍未验证',
  pass: '通过',
  fail: '失败',
  warn: '注意',
  unverified: '未验证',
  skipped: '跳过',
  errorCode: '错误码：{code}',
  remedy: '处理建议：{remedy}',
  raw: '引擎原始结果（JSON）',
};
const en = {
  view: 'Flywheel',
  title: 'Flywheel tasks',
  description: 'After preparing a task, inspect its progress, start, resume or stop it here',
  lifecycle: 'Tasks run independently in the background. Switching views or sessions does not stop a task; use Stop here when needed.',
  selection: 'Open an existing task',
  taskDir: 'Task files directory',
  taskHint: 'Use the full path supplied by the assistant when it prepared the task instructions and acceptance checks; leave blank to inspect configuration or run diagnostics',
  cwd: 'Project directory (optional)',
  cwdHint: 'Leave blank to use this DSH session’s current working directory. If that is not this task’s project root, enter the full project path supplied by the assistant.',
  pathInvalid: 'Enter a full path beginning with /',
  pathControl: 'Paths cannot contain line breaks or null characters',
  taskRequired: 'Enter the task files directory to refresh status, start or stop the task',
  status: 'Refresh status',
  start: 'Start',
  launch: 'Launch prepared task',
  launchMode: 'New task workflow',
  launchHelp: 'Auto lets the engine choose Quick or Full. Use Start to continue an existing task with its saved workflow.',
  workflow: 'Workflow',
  requestedMode: 'Requested: {mode}',
  effectiveMode: 'Effective: {mode}',
  workflowReason: 'Selection reason: {reason}',
  legacyFull: 'This task has no saved adaptive workflow; the original Full workflow applies.',
  applicable: 'Applicable',
  notApplicable: 'Not applicable',
  upgrades: 'Workflow upgrades',
  upgrade: '{from} → {to}: {reason}',
  finalReview: 'Quick still requires verification, final checks and an independent task-level Review. Not-applicable Plan / Judge stages do not mean passed.',
  associationUnconfirmed: 'The conversation records a task request; its engine outcome is still unconfirmed.',
  stop: 'Stop',
  config: 'Inspect model configuration',
  doctor: 'Check installation and configuration',
  probe: 'Check DSH integration',
  doctorHelp: 'Installation and configuration: check local tools, model settings and selected task files without starting the task.',
  probeHelp: 'DSH integration: briefly start a separate DSH runtime, verify its configuration, project directory and permissions, then close it. It may fetch a model catalog online.',
  busy: 'Waiting for “{action}” confirmation…',
  busyOther: 'Waiting for “{action}” for the previous directory; its result will not replace the current selection',
  mutationLock: 'Task and project directories are locked until start or stop is confirmed',
  unknownOutcome: '“{action}” outcome is unconfirmed',
  unknownHelp: 'The request may have reached the engine. Refresh this task before choosing the next action.',
  reconcile: 'Refresh unconfirmed task',
  errorTitle: '“{action}” did not complete',
  errorHelp: 'Check the connection and directories, then retry this read-only check.',
  notDispatchedTitle: '“{action}” was not dispatched to the engine',
  notDispatchedHelp: 'Check the error details, check the session association or engine configuration, then retry.',
  errorDetails: 'Error details',
  feedback: 'Operation feedback',
  statusHeading: 'Task status',
  emptyHeading: 'No task status yet',
  emptyBody: 'In Chat, enter /flywheel-task and your requirement. After the assistant prepares and launches the task, this conversation links to its exact directory automatically. /flywheel-new still prepares only. You can also enter an existing task directory here.',
  emptySelected: 'Refresh status to inspect this task. Opening this view does not start it.',
  lastChecked: 'Last confirmed: {time}',
  stale: 'Current status is unconfirmed; the previous result is retained below',
  stateUnconfirmed: 'Status unconfirmed',
  previousState: 'Previously confirmed state: {status}',
  task: 'Task',
  running: 'Running',
  uninitialized: 'Not initialized',
  engineMissing: 'Engine is not running; state needs verification',
  unknownState: 'Unknown state ({status})',
  unknown: 'Unknown',
  unreported: 'Not reported',
  pending: 'Pending',
  paused: 'Paused',
  stopped: 'Stopped',
  completed: 'Completed',
  escalated: 'Needs human attention',
  budgetStopped: 'Stopped by budget',
  milestone: 'Current milestone: {name}',
  packets: 'Work packets',
  packetsCount: 'Work packets ({count})',
  packet: 'Work packet',
  state: 'State',
  route: 'Route',
  packetPending: 'Pending',
  awaitingCredentials: 'Waiting for credentials',
  executing: 'Executing',
  executed: 'Awaiting verification',
  l1Passed: 'Verification passed',
  l1Failed: 'Verification failed',
  reviewFailed: 'Changes requested',
  interrupted: 'Interrupted',
  inbox: 'Waiting for attention',
  inboxCount: 'Human attention records: {count}',
  budget: 'Usage and cost',
  cost: 'Reported cost total: $' + '{amount}',
  costUnknown: 'Cost: unknown',
  costPartial: 'Known cost: $' + '{amount}; {count} other calls have unknown cost, so this total is incomplete',
  costMissing: 'Cost: unknown ({count} calls did not report billed cost)',
  tokens: 'Reported tokens: {tokens}',
  tokensMissing: '; usage is unknown for {count} calls, so this total is incomplete',
  tokenLimit: 'Daily token limit: {limit} · Budget policy: {policy}',
  budgetWarning: 'Budget notice: {message}',
  advanced: 'Configuration and troubleshooting (optional)',
  advancedHelp: 'Useful after installation, upgrades or a failed run; neither check is required before every task. Neither sends messages to a model or verifies account access, quota or real model responses.',
  configuration: 'Effective model configuration',
  configurationHelp: 'These are the effective Flywheel settings loaded from the source below. The model selected in the DSH chat composer does not replace them.',
  source: 'Configuration source: {source}',
  embedded: 'Embedded template',
  scope: 'Configuration level: {scope}',
  outOfScope: 'Not used for this check',
  model: 'Model',
  profile: 'Profile',
  harness: 'Harness',
  provider: 'Provider',
  channel: 'Channel',
  thinking: 'Reasoning',
  permission: 'Required permission',
  permissionFull: 'Full access (no step-by-step approval)',
  permissionWorkspace: 'Write within the project',
  permissionReadOnly: 'Read only',
  session: 'Session policy',
  step: 'Run stage', 'config-edit': 'Read configuration', 'config-save': 'Save configuration',
  enabled: 'Enabled',
  disabled: 'Disabled',
  notConfigured: 'Not configured',
  nativeProvider: 'Managed by the native runtime',
  default: 'Default',
  undeclared: 'Not declared',
  planRole: 'Plan',
  executeRole: 'Execute',
  reviewRole: 'Review',
  judgeRole: 'Judge',
  intakeRole: 'Intake',
  gardenRole: 'Knowledge maintenance',
  panelRole: 'Second opinion',
  diagnostic: 'Environment check results',
  doctorPassed: 'Check complete: no blockers found',
  doctorFailed: 'Check complete: blockers found',
  localOnly: 'Local tools, configuration and selected task files were checked; no DSH integration check ran.',
  probeRequested: 'A DSH integration check was requested (no model call)',
  inferenceBoundary: 'No message was sent to a model. Account access, available quota and real model responses remain unverified.',
  pass: 'Passed',
  fail: 'Failed',
  warn: 'Notice',
  unverified: 'Unverified',
  skipped: 'Skipped',
  errorCode: 'Error code: {code}',
  remedy: 'Next step: {remedy}',
  raw: 'Raw engine response (JSON)',
};

const taskLabels = {
  pending: 'pending', paused: 'paused', stopped: 'stopped', completed: 'completed',
  escalated: 'escalated', 'budget-stopped': 'budgetStopped',
};
const packetLabels = {
  pending: 'packetPending', 'awaiting-credentials': 'awaitingCredentials',
  executing: 'executing', executed: 'executed', 'l1-passed': 'l1Passed',
  'l1-failed': 'l1Failed', 'review-failed': 'reviewFailed', completed: 'completed',
  interrupted: 'interrupted', escalated: 'escalated', inbox: 'inbox',
};
const roleLabels = { plan: 'planRole', execute: 'executeRole', review: 'reviewRole', judge: 'judgeRole',
  intake: 'intakeRole', garden: 'gardenRole', 'judge-panel': 'panelRole' };
const checkLabels = { pass: 'pass', fail: 'fail', warn: 'warn', unknown: 'unverified', skipped: 'skipped' };
const permissionLabels = {
  'danger-full-access': 'permissionFull',
  'workspace-write': 'permissionWorkspace',
  'read-only': 'permissionReadOnly',
};
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const textValue = (value, fallback = '—') => typeof value === 'string' || typeof value === 'number' ? String(value) : fallback;
const selectionOf = value => ({ taskDir: value.taskDir.trim(), cwd: value.cwd.trim() });
const selectionKey = value => JSON.stringify([value.taskDir, value.cwd]);
const mutates = action => ['investigate', 'launch', 'start', 'stop', 'step', 'session', 'deliver', 'config-save'].includes(action);
const actionKey = value => value.probe ? 'probe' : value.action;
class NotDispatchedControlReply extends Error {}

function pathProblem(value) {
  if (/[\0\r\n]/.test(value)) return 'pathControl';
  if (value.trim() && !value.trim().startsWith('/')) return 'pathInvalid';
  return null;
}

function taskLabel(value, t) {
  if (value.running) return t('running');
  if (value.preparation) return t('investigationState_' + value.preparation.status);
  if (!value.state) return t('uninitialized');
  if (value.state.status === 'running') return t('engineMissing');
  const key = taskLabels[value.state.status];
  return key ? t(key) : t('unknownState', { status: textValue(value.state.status, t('unreported')) });
}

function readReply(reply, request) {
  if (!reply?.ok) throw new Error(reply?.error?.message ?? 'DSH command transport failed');
  const result = reply.value?.result;
  if (result?.kind !== 'success') {
    let failure;
    // This is a versioned command envelope, not a heuristic over error prose.
    // Older hosts and malformed/ambiguous replies keep the conservative outcome.
    if (result?.kind === 'error' && typeof result.text === 'string') {
      try { failure = JSON.parse(result.text); } catch { /* legacy unstructured error */ }
    }
    const typed = record(failure) && failure.protocol === 1 &&
      failure.type === 'flywheel-control-error' && failure.action === request.action &&
      ['not-dispatched', 'unknown'].includes(failure.dispatch) && typeof failure.message === 'string';
    const message = typed ? failure.message : result?.text ?? 'Flywheel command unavailable';
    if (typed && failure.dispatch === 'not-dispatched') throw new NotDispatchedControlReply(message);
    throw new Error(message);
  }
  const value = JSON.parse(result.text);
  // runControl has already matched the engine's task identity against the
  // host's realpath-normalized request. A browser cannot compare that identity
  // lexically with a user path (/var, symlinks, trailing slash or "..").
  if (!record(value) || value.protocol !== 1 || typeof value.ok !== 'boolean' ||
      typeof value.taskDir !== 'string' ||
      (request.taskDir ? !value.taskDir.startsWith('/') : value.taskDir !== '')) {
    throw new Error('Flywheel control protocol or task identity mismatch');
  }
  if (!value.ok && !(request.action === 'doctor' && value.diagnostic?.ok === false)) {
    throw new Error(typeof value.error === 'string' ? value.error : 'Flywheel control failed');
  }
  const state = value.state;
  const configurationValid = report => record(report) && Array.isArray(report.bindings) &&
    report.bindings.every(record) && (report.jev === undefined || record(report.jev) &&
      typeof report.jev.enabled === 'boolean' && typeof report.jev.model === 'string' &&
      Number.isSafeInteger(report.jev['max-calls']) && report.jev['max-calls'] > 0);
  if (['status', 'launch', 'start', 'stop', 'step', 'session', 'deliver', 'node', 'investigate'].includes(request.action) &&
      (typeof value.running !== 'boolean' || (state !== null && !record(state)) ||
       (state?.packets != null && (!record(state.packets) || !Object.values(state.packets).every(validExecutionPolicyHistory))) ||
       (state?.inbox != null && !Array.isArray(state.inbox)) ||
       !validRuntimeDecisions(state?.['runtime-decisions']) ||
       (state?.budget != null && (!record(state.budget) ||
        (state.budget.warnings != null && !Array.isArray(state.budget.warnings)))))) {
    throw new Error('Flywheel control returned malformed task status');
  }
  const investigationStatusAfterHandoff = request.investigation?.action === 'status' && value.preparation === undefined &&
    record(state) && Number.isSafeInteger(state.revision) && state.revision > 0 && typeof state.status === 'string' &&
    state.status !== '' && record(state.packets);
  if ((value.preparation !== undefined && !validPreparationSnapshot(value.preparation)) ||
      (request.action === 'investigate' && !value.preparation && !investigationStatusAfterHandoff)) throw new Error('Flywheel control returned malformed investigation status');
  if (['config-edit', 'config-save'].includes(request.action) && (!record(value.editor) || !record(value.editor.values) || !record(value.editor.defaults) || value.editor.scope !== request.editor.scope || typeof value.editor.revision !== 'string')) {
    throw new Error('Flywheel configuration editor returned a malformed report');
  }
  if (['delivery','deliver'].includes(request.action) && (!record(value.delivery) || !Array.isArray(value.delivery.pulls))) throw new Error('Malformed delivery snapshot');
  if (request.action === 'node' && (!record(value.node) || value.node.id !== request.packet || !validExecutionPolicyHistory(value.node.packet))) throw new Error('Malformed node detail');
  if (request.action === 'history' && (!record(value.history) || value.history.key !== request.sessionKey)) throw new Error('Malformed session history');
  if (request.action === 'runs' && (!Array.isArray(value.runs) || !value.runs.every(record))) throw new Error('Malformed run index');
  if (request.action === 'metrics' && (!record(value.metrics) || !Array.isArray(value.metrics.groups))) throw new Error('Malformed run metrics');
  if (request.action === 'config' && !configurationValid(value.configuration)) {
    throw new Error('Flywheel control returned malformed model configuration');
  }
  if (request.action === 'doctor' && (!record(value.diagnostic) ||
      value.diagnostic.ok !== value.ok || !Array.isArray(value.diagnostic.checks) ||
      !value.diagnostic.checks.every(record) ||
      (value.diagnostic.configuration !== undefined && !configurationValid(value.diagnostic.configuration)))) {
    throw new Error('Flywheel control returned malformed environment report');
  }
  return value;
}

function initialControl() {
  return { pending: null, status: null, checkedAt: null, stale: false, configuration: null,
    node: null, delivery: null, deliveryStale: false, runs: null, runsLoading: false, runsError: null, runsCwd: null, metrics: null, history: null, diagnostic: null, editor: null, configUncertainty: false, raw: null, error: null, uncertainty: null };
}

// Transport state belongs to this apply-owned controller, not a component or
// the draft store. Its observable reaches React only through slot injection.
function createControl(execute, pendingOperations, isDisposed) {
  const source = createSnapshot(initialControl());
  let selected = null;
  let epoch = 0;
  let active = null;
  let lastAssociation = null;
  let associatedKey = null;
  let deferredAssociation = null;
  let discovering = false;
  // Discovery is a separate read lane: it cannot select a task, clear a pending
  // mutation, or invalidate an in-flight status/configuration response.
  const discover = async () => {
    if (isDisposed() || discovering) return;
    discovering = true;
    source.update(draft => { draft.runsLoading = true; draft.runsError = null; });
    let timer, disposeWait;
    try {
      const deadline = new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error('Task discovery timed out')), CONTROL_WAIT_MS);
        disposeWait = () => reject(new Error('Task discovery closed'));
        pendingOperations.add(disposeWait);
      });
      const value = await Promise.race([execute({ action: 'runs' }), deadline]);
      if (!isDisposed()) source.update(draft => { draft.runs = value.runs; draft.runsCwd = value.cwd; });
      return value;
    } catch (error) {
      if (!isDisposed()) source.update(draft => { draft.runsError = error instanceof Error ? error.message : String(error); });
    } finally {
      clearTimeout(timer);
      pendingOperations.delete(disposeWait);
      discovering = false;
      if (!isDisposed()) source.update(draft => { draft.runsLoading = false; });
    }
  };
  const select = selection => {
    const normalized = selectionOf(selection);
    const key = selectionKey(normalized);
    if (selected === key) return;
    selected = key;
    epoch += 1;
    source.update(draft => {
      Object.assign(draft, initialControl(), { pending: draft.pending, uncertainty: draft.uncertainty, runs: draft.runs, runsLoading: draft.runsLoading, runsError: draft.runsError, runsCwd: draft.runsCwd });
    });
  };
  const run = async (action, selection, probe = false, extra = {}) => {
    if (isDisposed() || active || (mutates(action) && source.getSnapshot().uncertainty)) return;
    if (!['status', 'launch', 'start', 'stop', 'step', 'config', 'doctor', 'config-edit', 'config-save', 'runs', 'metrics', 'session', 'history', 'delivery', 'deliver', 'node', 'investigate'].includes(action)) return;
    if (action === 'config-save' && source.getSnapshot().configUncertainty) return;
    if (pathProblem(selection.taskDir) || pathProblem(selection.cwd)) return;
    const target = selectionOf(selection);
    if (['status', 'launch', 'start', 'stop', 'step', 'session', 'history', 'metrics', 'delivery', 'deliver', 'node', 'investigate'].includes(action) && !target.taskDir) return;
    select(selection);
    if (['launch', 'start'].includes(action) && source.getSnapshot().stale) return;
    const requestEpoch = epoch;
    const requestKey = selected;
    const request = { action, ...(target.taskDir ? { taskDir: target.taskDir } : {}),
      ...(target.cwd ? { cwd: target.cwd } : {}), ...(probe ? { probe: true } : {}),
      ...(action === 'launch' ? { mode: selection.mode ?? 'auto' } : {}),
      ...(['config-edit', 'config-save'].includes(action) ? { editor: extra.editor } : {}),
      ...(['delivery','deliver'].includes(action) ? { delivery: extra.delivery } : {}),
      ...(action === 'node' ? { packet: extra.packet } : {}),
      ...(action === 'metrics' && extra.runId ? { runId: extra.runId } : {}),
      ...(action === 'history' ? { sessionKey: extra.sessionKey } : {}),
      ...(action === 'session' ? { session: extra.session } : {}),
      ...(action === 'investigate' ? { investigation: extra.investigation } : {}),
      ...(action === 'step' ? { only: extra.only, packet: extra.packet, revision: extra.revision, ...(extra.retryReason ? { retryReason: extra.retryReason } : {}) } : {}) };
    const operation = { action, probe, ...target, key: requestKey };
    active = operation;
    source.update(draft => { draft.pending = operation; draft.error = null; });
    let timer;
    let disposeWait;
    try {
      const deadline = new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error('Flywheel control confirmation timed out')), CONTROL_WAIT_MS);
        disposeWait = () => reject(new Error('Flywheel UI was unloaded before confirmation'));
        pendingOperations.add(disposeWait);
      });
      const value = await Promise.race([execute(request), deadline]);
      if (isDisposed() || requestEpoch !== epoch) return;
      source.update(draft => {
        if (['status', 'launch', 'start', 'stop', 'step', 'session', 'deliver', 'node', 'investigate'].includes(action)) {
          // A remote snapshot is bound to the previously observed candidate.
          // A new engine revision requires another explicit remote inspection.
          if (draft.delivery && value.state?.revision !== draft.status?.state?.revision) draft.deliveryStale = true;
          draft.status = value;
          draft.checkedAt = Date.now();
          draft.stale = false;
          if (action !== 'node' && draft.uncertainty?.key === requestKey) draft.uncertainty = null;
        }
        if (['config', 'doctor'].includes(action)) {
          draft.configuration = value.configuration ?? value.diagnostic?.configuration ?? null;
          draft.diagnostic = value.diagnostic ?? null;
        }
        if (['config-edit', 'config-save'].includes(action)) { draft.editor = value.editor; draft.configUncertainty = false; }
        if (['delivery','deliver'].includes(action)) { draft.delivery = value.delivery; draft.deliveryStale = false; }
        if (action === 'node') draft.node = value.node;
        if (action === 'history') draft.history = value.history;
        if (action === 'runs') draft.runs = value.runs;
        if (action === 'metrics') draft.metrics = value.metrics;
        draft.raw = value;
      });
      return value;
    } catch (error) {
      if (isDisposed() || requestEpoch !== epoch) return;
      source.update(draft => {
        const detail = error instanceof Error ? error.message : String(error);
        const dispatch = error instanceof NotDispatchedControlReply ? 'not-dispatched' : 'unknown';
        draft.error = { action, probe, detail, dispatch };
        if (['delivery','deliver'].includes(action)) draft.deliveryStale = true;
        if (action === 'status' || (mutates(action) && dispatch === 'unknown')) draft.stale = true;
        if (action === 'config-save' && dispatch === 'unknown') draft.configUncertainty = true;
        else if (mutates(action) && dispatch === 'unknown') draft.uncertainty = { ...operation, detail };
      });
    } finally {
      clearTimeout(timer);
      pendingOperations.delete(disposeWait);
      active = null;
      if (!isDisposed()) source.update(draft => { draft.pending = null; });
      if (deferredAssociation) {
        const next = deferredAssociation;
        deferredAssociation = null;
        associate(...next);
      }
    }
  };
  const associate = (value, setSelection) => {
    if (isDisposed() || !record(value) || !Number.isSafeInteger(value.sequence) ||
        typeof value.operationId !== 'string' || !['pending', 'confirmed', 'unknown'].includes(value.outcome) ||
        typeof value.taskDir !== 'string' || !value.taskDir.startsWith('/') || pathProblem(value.taskDir) ||
        typeof value.cwd !== 'string' || !value.cwd.startsWith('/') || pathProblem(value.cwd)) return;
    if (lastAssociation && (value.sequence < lastAssociation.sequence ||
        (value.operationId === lastAssociation.operationId && value.outcome === lastAssociation.outcome &&
         value.taskDir === lastAssociation.taskDir && value.cwd === lastAssociation.cwd))) return;
    if (active) { deferredAssociation = [value, setSelection]; return; }
    const next = { taskDir: value.taskDir, cwd: value.cwd };
    const sameOperation = lastAssociation?.operationId === value.operationId;
    const shouldSelect = !lastAssociation || (!sameOperation && (mutates(value.action) || value.action === 'status' && value.operationId.startsWith('tool:'))) ||
      (sameOperation && selected === associatedKey);
    lastAssociation = value;
    if (shouldSelect) {
      setSelection(next);
      select(next);
      associatedKey = selectionKey(next);
    }
    source.update(draft => {
      if (value.outcome !== 'confirmed' && mutates(value.action)) {
        draft.uncertainty = { ...next, action: value.action, operationId: value.operationId,
          key: selectionKey(next), detail: value.message || '' };
        draft.stale = true;
      } else if (draft.uncertainty && (draft.uncertainty.operationId === value.operationId ||
          draft.uncertainty.key === selectionKey(next))) {
        draft.uncertainty = null;
      }
    });
    // Logs recover identity, never a live running state. A confirmed association
    // gets one fresh read; pending/unknown mutations require explicit reconcile.
    if (shouldSelect && value.outcome === 'confirmed' &&
        (mutates(value.action) || !source.getSnapshot().status)) void run('status', next);
  };
  return { source, select, run, discover, associate, isMutating: () => active !== null && mutates(active.action) };
}

function Configuration({ value, t }) {
  if (!value) return null;
  const field = (name, value, code = false) => h(React.Fragment, { key: name },
    h('dt', null, t(name)), h('dd', null, code ? h('code', null, value) : value));
  return h('section', { className: css.section, 'aria-label': t('configuration') },
    h('h3', { className: css.heading }, t('configuration')),
    h('p', { className: css.meta }, t('configurationHelp')),
    h('p', { className: css.meta }, t('source', { source: textValue(value.source, '') || t('embedded') })),
    h('p', { className: css.meta }, t('scope', { scope: textValue(value.scope) })),
    record(value.jev) && h('article', { className: css.binding },
      h('h4', { className: css.bindingTitle }, t('cfgJev')),
      h('p', null, t(value.jev.enabled === true ? 'consoleJEVConfiguredOn' : 'consoleJEVConfiguredOff')),
      value.jev.enabled === true && h('dl', { className: css.properties },
        field('consoleJEVVersion', textValue(value.jev.model), true),
        field('cfgJevMaxCalls', value.jev['max-calls'] ?? t('unknown')))),
    h('div', { className: css.bindings }, ...(value.bindings ?? []).map((binding, index) =>
      h('article', { className: css.binding, key: textValue(binding?.id, String(index)) },
        h('div', { className: css.bindingHeader },
          h('h4', { className: css.bindingTitle }, roleLabels[binding?.role] ? t(roleLabels[binding.role]) : textValue(binding?.role)),
          h('code', { className: css.meta }, textValue(binding?.route || binding?.id))),
        binding?.inScope === false && h('p', { className: css.meta }, t('outOfScope')),
        h('dl', { className: css.properties },
          field('model', textValue(binding?.model, '') || t('notConfigured'), true),
          binding?.profile && field('profile', textValue(binding.profile)),
          field('harness', textValue(binding?.harness), true),
          field('provider', textValue(binding?.provider, '') || (binding?.harness === 'dsh' ? t('notConfigured') : t('nativeProvider'))),
          field('channel', textValue(binding?.channel, '') || t('notConfigured')),
          field('thinking', textValue(binding?.thinking, '') || t('default')),
          field('permission', Object.hasOwn(permissionLabels, binding?.permissionMode) ?
            t(permissionLabels[binding.permissionMode]) : textValue(binding?.permissionMode, '') || t('undeclared')),
          field('session', sessionModeLabel(binding, t)))))));
}

function Diagnostic({ value, t }) {
  if (!value) return null;
  return h('section', { className: css.section, 'aria-label': t('diagnostic') },
    h('h3', { className: css.heading }, t(value.ok ? 'doctorPassed' : 'doctorFailed')),
    h('p', { className: css.meta }, t(value.probeRequested ? 'probeRequested' : 'localOnly')),
    h('p', { className: css.meta }, t('inferenceBoundary')),
    h('ul', { className: css.checks }, ...(value.checks ?? []).map((check, index) =>
      h('li', { className: css.check, key: textValue(check?.id, '') + ':' + index },
        h('span', { className: css.checkState, 'data-state': textValue(check?.status, 'unknown') },
          t(checkLabels[check?.status] ?? 'unverified')),
        h('div', { className: css.checkBody },
          h('h4', { className: css.bindingTitle }, textValue(check?.id)),
          h('p', null, textValue(check?.summary)),
          check?.code && h('p', { className: css.meta }, t('errorCode', { code: textValue(check.code) })),
          check?.remedy && h('p', null, t('remedy', { remedy: textValue(check.remedy) })))))));
}

function Budget({ value, t }) {
  if (!value) return null;
  const cost = value.totals?.['cost-usd'];
  const unknownCost = value['unknown-cost-calls'] ?? 0;
  const unknownUsage = value['unknown-usage-calls'] ?? 0;
  let costText = Number.isFinite(cost) ? t('cost', { amount: cost.toFixed(4) }) : t('costUnknown');
  if (unknownCost > 0) {
    costText = Number.isFinite(cost) && cost > 0 ?
      t('costPartial', { amount: cost.toFixed(4), count: unknownCost }) : t('costMissing', { count: unknownCost });
  }
  return h('section', { className: css.budget, 'aria-label': t('budget') },
    h('p', null, costText),
    h('p', null, t('tokens', { tokens: value.totals?.tokens ?? t('unknown') }) +
      (unknownUsage > 0 ? t('tokensMissing', { count: unknownUsage }) : '')),
    value['daily-token-limit'] != null && h('p', { className: css.meta },
      t('tokenLimit', { limit: value['daily-token-limit'], policy: textValue(value.enforce) })),
    ...(value.warnings ?? []).map((warning, index) => h('p', { key: index },
      t('budgetWarning', { message: textValue(warning) }))));
}

function Workflow({ state, t }) {
  if (!state) return null;
  const workflow = state.workflow;
  const modes = { auto: 'Auto', quick: 'Quick', full: 'Full' };
  const mode = value => Object.hasOwn(modes, value) ? modes[value] : t('unknown');
  const legacy = workflow == null;
  return h('section', { className: css.workflow, 'aria-label': t('workflow') },
    h('h4', { className: css.bindingTitle }, t('workflow')),
    h('div', { className: css.actions },
      h('span', { className: css.badge }, t('requestedMode', { mode: mode(legacy ? 'full' : workflow.requested) })),
      h('span', { className: css.badge }, t('effectiveMode', { mode: mode(legacy ? 'full' : workflow.effective) }))),
    h('p', null, legacy ? t('legacyFull') : t('workflowReason', { reason: textValue(workflow.reason, t('unreported')) })),
    ...['plan', 'judge'].map(stage => {
      const value = workflow?.stages?.[stage];
      return h('p', { key: stage, className: css.meta }, t(roleLabels[stage]) + ' · ' +
        (legacy || value?.applicable === true ? t('applicable') : value?.applicable === false ? t('notApplicable') : t('unreported')) +
        (typeof value?.reason === 'string' && value.reason ? ' · ' + value.reason : ''));
    }),
    workflow?.effective === 'quick' && h('p', { className: css.meta }, t('finalReview')),
    Array.isArray(workflow?.upgrades) && workflow.upgrades.length > 0 && h('div', null,
      h('h4', { className: css.bindingTitle }, t('upgrades')),
      h('ul', null, ...workflow.upgrades.map((upgrade, index) => h('li', { key: index },
        t('upgrade', { from: mode(upgrade?.from), to: mode(upgrade?.to), reason: textValue(upgrade?.reason) }),
        typeof upgrade?.at === 'string' && h('span', { className: css.meta }, ' · ' + upgrade.at))))));
}

function TaskStatus({ control, selection, t }) {
  const { status, checkedAt, stale } = control;
  if (!status) return h('section', { className: css.empty, 'aria-label': t('statusHeading') },
    h('h3', { className: css.heading }, t('emptyHeading')),
    h('p', { className: css.meta }, t(selection.taskDir.trim() ? 'emptySelected' : 'emptyBody')));
  const packets = Object.entries(status.state?.packets ?? {});
  return h('section', { className: css.section, 'aria-label': t('statusHeading') },
    h('div', { className: css.sectionHeader },
      h('h3', { className: css.heading }, textValue(status.state?.slug, t('task'))),
      h('span', { className: css.badge }, stale ? t('stateUnconfirmed') : taskLabel(status, t))),
    h('p', { className: css.meta },
      t('lastChecked', { time: new Date(checkedAt).toLocaleString() }) +
      (status.running && status.pid > 0 ? ' · PID ' + status.pid : '')),
    stale && h('p', { className: css.warningText },
      t('stale') + ' · ' + t('previousState', { status: taskLabel(status, t) })),
    h(Workflow, { state: status.state, t }),
    status.state?.['current-milestone'] && h('p', null,
      t('milestone', { name: textValue(status.state['current-milestone']) })),
    packets.length > 0 && h('section', { className: css.details }, h('h4', null, t('packetsCount', { count: packets.length })), h('table', { className: css.packets, 'aria-label': t('packets') },
      h('caption', null, t('packetsCount', { count: packets.length })),
      h('thead', null, h('tr', null, ...['packet', 'state', 'route'].map(key => h('th', { key, scope: 'col' }, t(key))))),
      h('tbody', null, ...packets.map(([id, packet]) => h('tr', { key: id },
        h('td', null, id),
        h('td', null, packetLabels[packet?.status] ? t(packetLabels[packet.status]) : textValue(packet?.status, t('unknown'))),
        h('td', null, textValue(packet?.route))))))),
    h(Budget, { value: status.state?.budget, t }),
    status.state?.inbox?.length > 0 && h('section', { className: css.details },
      h('h4', null, t('inboxCount', { count: status.state.inbox.length })),
      h('pre', { className: css.raw }, JSON.stringify(status.state.inbox, null, 2))));
}

function Feedback({ control, selection, reconcile, t }) {
  const busy = control.pending !== null;
  const error = control.error &&
    (!mutates(control.error.action) || control.error.dispatch === 'not-dispatched') ? control.error : null;
  const mode = control.uncertainty ? 'uncertain' : error ? 'error' : busy ? 'loading' : 'idle';
  const otherSelection = busy && control.pending.key !== selectionKey(selectionOf(selection));
  const locked = busy && mutates(control.pending.action);
  const pendingLabel = busy ? t(otherSelection ? 'busyOther' : 'busy', {
    action: t(actionKey(control.pending)),
  }) : '';
  return h('div', { className: css.feedback, 'data-state': mode,
    role: 'region', 'aria-label': t('feedback'), tabIndex: mode === 'uncertain' || mode === 'error' ? 0 : undefined },
    control.status?.workspaceWarnings?.map((warning, index) => h('p', { key: index, role: 'alert', className: css.notice }, warning)),
    busy && h('div', { className: control.uncertainty ? css.loadingCompact : css.loading,
      role: 'status', 'aria-label': pendingLabel },
      h(Spinner),
      !control.uncertainty && (locked || otherSelection) && h('p', null, pendingLabel),
      locked && h('p', { className: css.meta }, t('mutationLock'))),
      control.uncertainty && h('section', { className: css.notice, role: 'alert' },
        h('h3', { className: css.heading }, t('unknownOutcome', { action: t(control.uncertainty.action) })),
        h('p', null, t('unknownHelp')),
        h('code', { className: css.path }, control.uncertainty.taskDir),
        h(Button, { variant: 'outline', size: 'sm', disabled: busy, onClick: reconcile }, t('reconcile')),
        h('section', { className: css.details },
          h('h4', null, t('errorDetails')),
          h('pre', { className: css.raw }, control.uncertainty.detail || t('associationUnconfirmed')))),
      error && h('section', { className: css.error, role: 'alert' },
        h('h3', { className: css.heading }, t(error.dispatch === 'not-dispatched' ? 'notDispatchedTitle' : 'errorTitle',
          { action: t(actionKey(error)) })),
        h('p', null, t(error.dispatch === 'not-dispatched' ? 'notDispatchedHelp' : 'errorHelp')),
        h('section', { className: css.details },
          h('h4', null, t('errorDetails')),
          h('pre', { className: css.raw }, error.detail))),
      mode === 'idle' && !control.status?.state && !control.status?.preparation && h('div', { className: css.idleFeedback },
        h('p', null, t('lifecycle')),
        !selection.taskDir.trim() && h('p', null, t('taskRequired'))));
}

// Plain-JS external clients supply real schema factories through the public
// Typert codec contract; src-json is not a valid generated Client input codec.
function consoleReadCodec(type, parse) {
  return { mode: 'strict', typeSymbol: '@ckanner/flywheel#' + type, create: () => ({ parse }) };
}
function consoleReadObject(value, keys, label) {
  if (!record(value) || Object.keys(value).some(key => !keys.includes(key))) throw new Error('Invalid ' + label);
  return value;
}
function consoleReadString(value, label) {
  if (typeof value !== 'string' || !value.trim()) throw new Error('Invalid ' + label);
  return value;
}
function consoleReadInput(value) {
  consoleReadObject(value, ['action', 'taskDir', 'cwd', 'probe', 'editor', 'packet', 'runId', 'sessionKey', 'delivery'], 'console read request');
  if (!['runs', 'status', 'config', 'config-edit', 'metrics', 'history', 'node', 'delivery', 'doctor'].includes(value.action)) throw new Error('Console Remote accepts reads only');
  const output = { action: value.action };
  for (const key of ['taskDir', 'cwd', 'packet', 'runId', 'sessionKey']) {
    if (value[key] !== undefined) output[key] = consoleReadString(value[key], key);
  }
  if (value.probe !== undefined) {
    if (value.action !== 'doctor' || typeof value.probe !== 'boolean') throw new Error('Invalid doctor probe');
    output.probe = value.probe;
  }
  if (value.action === 'config-edit') {
    consoleReadObject(value.editor, ['scope'], 'configuration scope');
    if (!['global', 'project'].includes(value.editor.scope)) throw new Error('Invalid configuration scope');
    output.editor = { scope: value.editor.scope };
  } else if (value.editor !== undefined) throw new Error('Editor requires config-edit');
  if (value.action === 'delivery') {
    consoleReadObject(value.delivery, ['action', 'headBranch'], 'delivery inspection');
    if (value.delivery.action !== 'inspect') throw new Error('Delivery Remote accepts inspection only');
    output.delivery = { action: 'inspect' };
    if (value.delivery.headBranch !== undefined) output.delivery.headBranch = consoleReadString(value.delivery.headBranch, 'headBranch');
  } else if (value.delivery !== undefined) throw new Error('Delivery requires inspection');
  if (value.action === 'node' ? !/^M\d+-T\d+$/.test(output.packet ?? '') : value.packet !== undefined) throw new Error('Invalid node packet');
  if (value.action === 'history' ? !output.sessionKey : value.sessionKey !== undefined) throw new Error('Invalid history session key');
  if (value.runId !== undefined && (value.action !== 'metrics' || value.runId.length > 128 || /[\0\r\n]/.test(value.runId))) throw new Error('Invalid metrics run ID');
  return output;
}
function consoleReadResult(value) {
  consoleReadObject(value, ['result'], 'console read result');
  const result = consoleReadObject(value.result, ['kind', 'text'], 'console result envelope');
  if (!['success', 'error'].includes(result.kind) || typeof result.text !== 'string') throw new Error('Invalid console result envelope');
  return { result: { kind: result.kind, text: result.text } };
}

exports.inject = ['slots', 'remote', 'remote.commands', 'locale'];
exports.apply = function apply(ctx) {
  ctx.effect(() => ctx.locale.register(NS, { zh, en }), 'flywheel: locales');
  const t = ctx.locale.bind(NS);
  // Drafts are slot-owned view state. No busy/status value is persisted or
  // restored as a live fact after a new Session binding or a plugin reload.
  const viewStore = createViewStore();
  const controls = new WeakMap();
  const pendingOperations = new Set();
  let disposed = false;
  ctx.on('dispose', () => {
    disposed = true;
    for (const release of pendingOperations) release();
    pendingOperations.clear();
  });
  const readsReady = ctx.remote.$mount({ package: '@ckanner/flywheel', descriptors: [{
    id: '@ckanner/flywheel#flywheelConsole/read', service: 'flywheelConsole', namespace: 'flywheelConsole', method: 'read',
    invocation: { kind: 'direct' }, parameters: [
      { name: 'agent', wire: 'sessionId', source: 'lookup', lookup: 'agent', codec: consoleReadCodec('SessionId', value => consoleReadString(value, 'sessionId')) },
      { name: 'input', wire: 'input', source: 'json', codec: consoleReadCodec('ConsoleReadRequest', consoleReadInput) },
    ], cancellation: { parameter: 'signal' }, result: { ...consoleReadCodec('ConsoleReadResult', consoleReadResult), decode: consoleReadResult },
  }] });
  // A missing read capability is a visible error, never a noisy command fallback.
  readsReady.catch(() => {});
  ctx.slots.inject('conversation.view', () => ctx.slots.register({
    // External views follow the built-in Chat, Trajectory and Statistics views.
    name: 'conversation.view', id: 'flywheel', order: 1000, label: () => t('view'),
    locale: NS, store: viewStore,
    inject: (sessionId, actions) => {
      // Bound actions identify one retained Session binding. A recycled
      // session id with a fresh binding must not adopt an old async result.
      let control = controls.get(actions);
      if (!control) {
        control = createControl(async request => {
          const value = readReply(await (mutates(request.action) || request.action === 'status' && control?.source.getSnapshot().uncertainty
            ? ctx.remote.commands.execute(sessionId, '/flywheel ' + JSON.stringify(request), [])
            : readsReady.then(() => ctx.remote.flywheelConsole.read(sessionId, request))), request);
          if (Array.isArray(value.runs)) value.runs = value.runs.filter(item => item.hostSessionId === sessionId);
          return value;
        }, pendingOperations, () => disposed);
        controls.set(actions, control);
      }
      return {
        hostSessionID: sessionId,
        hooks: { control: control.source },
        changeSelection: next => {
          if (disposed || control.isMutating()) return;
          actions.setSelection(next);
          control.select(next);
        },
        run: control.run,
        discover: control.discover,
        associate: value => control.associate(value, actions.setSelection),
        reconcile: () => {
          const outcome = control.source.getSnapshot().uncertainty;
          if (!outcome || control.source.getSnapshot().pending) return;
          const next = { taskDir: outcome.taskDir, cwd: outcome.cwd };
          actions.setSelection(next);
          return control.run('status', next);
        },
      };
    },
  }, TaskView));
};
