Object.assign(zh, { projectHome: '项目工作区', executionHome: '实际执行目录', externalWorktree: '自行管理的 Worktree（保留目录）' });
Object.assign(en, { projectHome: 'Project workspace', executionHome: 'Execution directory', externalWorktree: 'Caller-managed Worktree (retained)' });
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
  if (['return', 'review-failed', 'paused', 'stopped', 'unknown', 'cancelled'].includes(status)) return 'warning';
  if (['blocked', 'interrupted', 'escalated', 'budget-stopped', 'l1-failed', 'failed'].includes(status)) return 'error';
  return 'idle';
}

Object.assign(zh, { runsObserved: '本轮新增交付 {completed} · 执行尝试 {attempts} · 失败尝试 {failed} · 总耗时 {seconds} s', runsPartial: '部分数据 {count} 次', runsBounded: '每个已显示分组均汇总其全部调用；明细最多 200 条、分组最多 500 个。按 run 筛选可查看其完整汇总。', runsAll: '当前任务全部调用（含早期记录）', runsRefresh: '刷新最近运行', runsOpen: '在画布中打开', runsMetrics: '读取所选运行指标', runsCompare: '比较条件', runsEqual: '相同契约与配置', runsDifferent: '条件不同，仅并列展示', runsLegacy: '早期调用（未登记 run ID）', runsSamples: '调用 / 失败', runsLatency: '已知调用耗时累计', runsUsage: '已知 Token / 缺失调用', runsCost: '已知费用 / 缺失调用', runsQuality: '验收质量见各节点证据；模型质量需要独立 oracle', runsEmpty: '暂无已登记运行。CLI 启动或桌面启动后会自动登记。', runsReference: '对照运行', runsLive: '尚无退出回执，打开画布核对', runsReadOnly: '登记记录用于发现；当前运行状态以刷新后的引擎回执为准。' });
Object.assign(en, { runsObserved: 'New deliveries {completed} · attempts {attempts} · failed attempts {failed} · elapsed {seconds} s', runsPartial: 'partial {count}', runsBounded: 'Each displayed group aggregates all its calls. Details are limited to 200 calls and 500 groups; filter by run for its full summary.', runsAll: 'All calls for this task, including legacy records', runsRefresh: 'Refresh recent runs', runsOpen: 'Open canvas', runsMetrics: 'Read selected run metrics', runsCompare: 'Comparison conditions', runsEqual: 'Same contracts and configuration', runsDifferent: 'Different conditions; descriptive comparison only', runsLegacy: 'Legacy call without run ID', runsSamples: 'Calls / failed', runsLatency: 'Sum of known call durations', runsUsage: 'Known tokens / missing calls', runsCost: 'Known cost / missing calls', runsQuality: 'Validation quality is recorded in node evidence; model quality needs an independent oracle', runsEmpty: 'No registered runs. CLI and desktop launches register automatically.', runsReference: 'Reference run', runsLive: 'No exit receipt; open canvas to reconcile', runsReadOnly: 'The index is for discovery; refresh engine status to establish liveness.' });

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
    value['workspace-cwd'] && h('p', { className: css.path }, t('projectHome') + ': ' + value['workspace-cwd']),
    value.cwd && h('p', { className: css.path }, t('executionHome') + ': ' + value.cwd),
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
      h('code', { className: css.path }, 'cd -- ' + shellQuote(value.cwd || control.status.cwd || selection.cwd) + ' && flywheel session ' + shellQuote(control.status.taskDir) + ' --key ' + shellQuote(sessionKey))));
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

Object.assign(zh, { consoleJEV: '启用 JEV', consoleJEVConfiguredOn: '配置已开启 · 在符合条件的决策点调用', consoleJEVConfiguredOff: '配置已关闭 · 沿用原路径', consoleJEVOff: '关闭 · 沿用原流程', consoleJEVOn: '开启 · 参与决策', consoleJEVVersion: '固定模型版本', consoleJEVEnv: '密钥环境变量名（不要填密钥）' });
Object.assign(en, { consoleJEV: 'Enable JEV', consoleJEVConfiguredOn: 'Configured on · calls at eligible decision points', consoleJEVConfiguredOff: 'Configured off · original paths', consoleJEVOff: 'Off · existing workflow', consoleJEVOn: 'On · participate in decisions', consoleJEVVersion: 'Pinned model version', consoleJEVEnv: 'API key environment variable name (not the key)' });
