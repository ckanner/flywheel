Object.assign(zh, {
  investigate: '调查', investigationTitle: '有界调查', investigationHelp: '由 Go 运行已批准的取证动作与诊断实验。调查结论独立于代码验收和交付。',
  investigationRun: '继续调查', investigationReconcile: '核对原动作回执', investigationReport: '调查报告', investigationEvidence: '已收集证据', investigationBudget: '动作 {used}/{limit} · 累计 {ms} ms', investigationPartial: '有界片段 · 内容未完整展示', investigationDecision: '决策建议与实际采用',
  investigationState_ready: '等待调查', investigationState_running: '调查中', 'investigationState_awaiting-report': '等待证据报告', investigationState_blocked: '结果未知 · 需核对', investigationState_supported: '报告有证据支持', investigationState_inconclusive: '尚无定论', 'investigationState_access-blocked': '缺少访问', investigationState_cancelled: '调查已取消',
  investigationRepair: '已授权原范围内修复；形成合法 Spec 后由 Auto 选择 Quick / Full。', investigationReportOnly: '本目标以调查报告结束；没有启动代码实现。',
  investigationJEVObserved: '最近观察到的 JEV 状态', investigationJEV_enabled: 'JEV 已开启 · 在符合条件的决策点参与', investigationJEV_disabled: 'JEV 已关闭 · 后续动作沿用原路径', 'investigationJEV_config-unavailable': 'JEV 配置不可用 · 沿用原路径', 'investigationJEV_not-configured': '本次调查未配置 JEV · 沿用原路径', 'investigationJEV_policy-changed': 'JEV 配置已变更 · 本次调查沿用原路径',
  investigationAdopted: 'JEV 已采用', investigationBaseline: '原路径', investigationEffect_defer: '依据证据跳过后续动作', investigationEffect_finish: '提前结束取证 · 等待报告', investigationSkipped: '跳过动作', investigationCitations: '依据观察',
  investigationCommand: '隔离副本中的命令', investigationHTTP: '批准的 HTTP 查询', investigationExit: '退出码', investigationHTTPStatus: 'HTTP 状态', investigationSignal: '终止信号', investigationTimedOut: '动作超时', investigationCancelled: '动作已取消', investigationWorkspace: '实验副本', investigationCleanup_removed: '实验副本已清理', investigationCleanup_retained: '实验副本保留 · 需要核对收尾', 'investigationCleanup_not-created': '未创建实验副本',
});
Object.assign(en, {
  investigate: 'Investigate', investigationTitle: 'Bounded investigation', investigationHelp: 'Go runs approved evidence actions and diagnostic experiments. An investigation conclusion is separate from code acceptance and delivery.',
  investigationRun: 'Continue investigation', investigationReconcile: 'Reconcile original receipt', investigationReport: 'Investigation report', investigationEvidence: 'Collected evidence', investigationBudget: 'Actions {used}/{limit} · elapsed {ms} ms', investigationPartial: 'Bounded excerpt · incomplete content', investigationDecision: 'Advice and recorded adoption',
  investigationState_ready: 'Ready to investigate', investigationState_running: 'Investigating', 'investigationState_awaiting-report': 'Awaiting evidence report', investigationState_blocked: 'Unknown result · reconcile', investigationState_supported: 'Report supported by cited evidence', investigationState_inconclusive: 'Inconclusive', 'investigationState_access-blocked': 'Access blocked', investigationState_cancelled: 'Investigation cancelled',
  investigationRepair: 'Repair is authorized within the original scope. Prepare a valid Spec before Auto selects Quick or Full.', investigationReportOnly: 'The goal ends with an investigation report; implementation has not started.',
  investigationJEVObserved: 'Last observed JEV state', investigationJEV_enabled: 'JEV enabled · participates at eligible decision points', investigationJEV_disabled: 'JEV disabled · subsequent actions use the original path', 'investigationJEV_config-unavailable': 'JEV configuration unavailable · original path', 'investigationJEV_not-configured': 'JEV not configured for this investigation · original path', 'investigationJEV_policy-changed': 'JEV policy changed · this investigation uses the original path',
  investigationAdopted: 'JEV adopted', investigationBaseline: 'Original path', investigationEffect_defer: 'Evidence-backed actions skipped', investigationEffect_finish: 'Evidence collection ended early · awaiting report', investigationSkipped: 'Skipped actions', investigationCitations: 'Cited observations',
  investigationCommand: 'Command in an isolated copy', investigationHTTP: 'Approved HTTP query', investigationExit: 'Exit code', investigationHTTPStatus: 'HTTP status', investigationSignal: 'Termination signal', investigationTimedOut: 'Action timed out', investigationCancelled: 'Action cancelled', investigationWorkspace: 'Experiment copy', investigationCleanup_removed: 'Experiment copy cleaned up', investigationCleanup_retained: 'Experiment copy retained · reconcile cleanup', 'investigationCleanup_not-created': 'No experiment copy created',
});

function validInvestigationExecution(value) {
  return value == null || record(value) && ['run_command', 'http_read'].includes(value.kind) &&
    ['timed-out', 'cancelled'].every(key => typeof value[key] === 'boolean') &&
    ['removed', 'retained', 'not-created'].includes(value.cleanup) &&
    (value['exit-code'] == null || Number.isSafeInteger(value['exit-code']) && value['exit-code'] >= 0 && value['exit-code'] <= 255) &&
    (value['http-status'] == null || Number.isSafeInteger(value['http-status']) && value['http-status'] >= 100 && value['http-status'] <= 599) &&
    ['signal', 'workspace', 'input-hash'].every(key => value[key] == null || typeof value[key] === 'string');
}

function validPreparationSnapshot(p, archived = false) {
  const strings = (object, keys) => record(object) && keys.every(key => typeof object[key] === 'string');
  const stringArray = value => Array.isArray(value) && value.every(item => typeof item === 'string');
  return record(p) && p.version === 1 && Number.isSafeInteger(p.revision) && p.revision > 0 &&
    (!archived || p.report != null) &&
    ['ready', 'running', 'awaiting-report', 'blocked', 'supported', 'inconclusive', 'access-blocked', 'cancelled'].includes(p.status) &&
    typeof p.reason === 'string' &&
    strings(p.contract, ['id', 'goal', 'intent']) && ['investigate', 'investigate-and-fix'].includes(p.contract.intent) &&
    record(p.contract.limits) && Number.isSafeInteger(p.contract.limits['max-actions']) &&
    Array.isArray(p.observations) && p.observations.every(observation => record(observation) && strings(observation.invocation, ['id', 'action-id']) &&
      ['output', 'error'].every(key => observation[key] == null || typeof observation[key] === 'string') && validInvestigationExecution(observation.execution)) &&
    (p.history == null || !archived && Array.isArray(p.history) && p.history.length <= 100 && p.history.every(round => record(round) && Number.isSafeInteger(round.number) && typeof round.digest === 'string' && validPreparationSnapshot(round.snapshot, true))) &&
    (p.decisions == null || Array.isArray(p.decisions) && p.decisions.every(record)) &&
    (p['jev-status'] == null || typeof p['jev-status'] === 'string') &&
    (p['decision-effects'] == null || Array.isArray(p['decision-effects']) && p['decision-effects'].every(effect =>
      strings(effect, ['decision-id', 'action-id', 'kind']) && ['defer', 'finish'].includes(effect.kind) && stringArray(effect.actions) && stringArray(effect.evidence))) &&
    (p.report == null || strings(p.report, ['summary', 'counter-evidence', 'recommendation', 'regression', 'rollback']) && Array.isArray(p.report.findings) &&
      p.report.findings.every(finding => strings(finding, ['hypothesis', 'status', 'reason'])) &&
      (p.report.limitations == null || Array.isArray(p.report.limitations) && p.report.limitations.every(limit => typeof limit === 'string')));
}

function InvestigationExecution({ value, t }) {
  if (!value) return null;
  const property = (label, content) => h(React.Fragment, { key: label }, h('dt', null, label), h('dd', null, String(content)));
  return h('div', { 'data-investigation-execution': value.kind },
    h('p', { className: css.meta }, t(value.kind === 'run_command' ? 'investigationCommand' : 'investigationHTTP')),
    h('dl', { className: css.consoleProperties },
      value['exit-code'] != null && property(t('investigationExit'), value['exit-code']),
      value['http-status'] != null && property(t('investigationHTTPStatus'), value['http-status']),
      value.signal && property(t('investigationSignal'), value.signal)),
    value['timed-out'] && h('p', { className: css.warningText }, t('investigationTimedOut')),
    value.cancelled && h('p', { className: css.warningText }, t('investigationCancelled')),
    h('p', { className: value.cleanup === 'retained' ? css.warningText : css.meta }, t('investigationCleanup_' + value.cleanup)),
    value.workspace && h('section', { className: css.details }, h('h4', null, t('investigationWorkspace')), h('p', { className: css.meta }, value.workspace)));
}

function InvestigationPanel({ control, selection, run, t }) {
  const [roundEditor,setRoundEditor] = React.useState(null);
  const [roundError,setRoundError] = React.useState('');
  const preparation = control.status?.preparation;
  if (!preparation) return null;
  const busy = control.pending !== null || control.uncertainty !== null || control.stale;
  const ended = !!preparation.report || preparation.status === 'cancelled';
  const mutate = action => run('investigate', selection, false, { investigation: { action, revision: preparation.revision } });
  const pending = preparation.pending || preparation['decision-pending'] || preparation['analysis-pending'];
  const remaining = {...preparation.contract.limits,
    'max-actions':preparation.contract.limits['max-actions']-(preparation['actions-used']??0),
    'max-seconds':(preparation.contract.limits['max-seconds']??0)-Math.ceil((preparation['elapsed-ms']??0)/1000),
    'max-output-bytes':(preparation.contract.limits['max-output-bytes']??0)-(preparation['output-bytes']??0)};
  const openRound = () => { setRoundError('');setRoundEditor({revision:preparation.revision,text:JSON.stringify({...preparation.contract,decision:undefined,limits:remaining,'known-evidence':[]},null,2)}); };
  const saveRound = () => {
    let contract; try {contract=JSON.parse(roundEditor.text);if(!record(contract))throw Error();}catch {setRoundError(t('investigationRoundInvalid'));return;}
    void run('investigate',selection,false,{investigation:{action:'continue',revision:roundEditor.revision,contract}});
    setRoundEditor(null);setRoundError('');
  };

  return h('section', { className: css.section, 'aria-label': t('investigationTitle') },
    h('h3', { className: css.heading }, t('investigationTitle') + ' · ' + preparation.contract.id),
    h('p',{className:css.meta},t('investigationRound',{round:(preparation.history?.length??0)+1,remaining:remaining['max-actions']})),
    h('p', { className: css.meta }, t('investigationHelp')),
    h('p', null, preparation.contract.goal),
    h('p', { role: 'status' }, t('investigationState_' + preparation.status)),
    h('p', { className: css.meta }, t('investigationBudget', { used: preparation['actions-used'], limit: preparation.contract.limits['max-actions'], ms: preparation['elapsed-ms'] })),
    h('p', { className: css.meta }, preparation.reason),
    preparation['jev-status'] && h('p', { className: css.meta }, t('investigationJEVObserved') + ': ' + t('investigationJEV_' + preparation['jev-status'])),
    h('div', { className: css.actions },
      h(Button, { disabled: busy || control.status.running || ended, onClick: () => mutate('run') }, t(pending ? 'investigationReconcile' : preparation.status==='awaiting-report' ? 'investigationAnalyze' : 'investigationRun')),
      h(Button, { disabled: busy || ended || !control.status.running && (!!preparation.pending || !!preparation['analysis-pending']), onClick: () => control.status.running ? run('stop', selection) : mutate('cancel') }, t('stop')),
      h(Button,{disabled:busy || control.status.running || !preparation.report || !!pending || remaining['max-actions']<1,onClick:openRound},t('investigationNewRound')),
      h(Button, { disabled: control.pending !== null, onClick: () => run('status', selection) }, t('status'))),
    h('p', { className: css.notice }, t(preparation.contract.intent === 'investigate-and-fix' ? 'investigationRepair' : 'investigationReportOnly')),
    roundEditor && h('section',{className:css.details,'aria-label':t('investigationNewRound')},
      h('p',{className:css.meta},t('investigationRoundHelp')),
      h('textarea',{'aria-label':t('investigationRoundContract'),className:css.editorInput,rows:12,value:roundEditor.text,disabled:busy,onChange:e=>setRoundEditor({...roundEditor,text:e.target.value})}),
      roundError && h('p',{role:'alert'},roundError),
      roundEditor.revision!==preparation.revision && h('p',{role:'alert'},t('investigationRoundStale')),
      h(Button,{disabled:busy || roundEditor.revision!==preparation.revision,onClick:saveRound},t('investigationRoundSave')),
      h(Button,{disabled:busy,onClick:()=>setRoundEditor(null)},t('cancel'))),
    (preparation.history?.length>0) && h('section',{className:css.details},h('h4',null,t('investigationHistory')),
      ...preparation.history.map(round=>h('article',{key:round.number},h('h5',null,t('investigationRound',{round:round.number,remaining:round.snapshot.contract.limits['max-actions']-round.snapshot['actions-used']})),h('p',null,round.snapshot.report.summary),h('p',{className:css.meta},round.digest)))),
    preparation.report && h('article', null, h('h4', null, t('investigationReport')), h('p', null, preparation.report.summary),
      ...preparation.report.findings.map(finding => h('p', { key: finding.hypothesis }, finding.hypothesis + ' · ' + finding.status + ': ' + finding.reason)),
      h('p', null, preparation.report['counter-evidence']),
      ...(preparation.report.limitations ?? []).map((limit, i) => h('p', { key: i, className: css.warningText }, limit)),
      h('p', null, preparation.report.recommendation), h('p', null, preparation.report.regression), h('p', null, preparation.report.rollback)),
    h('section', { className: css.details }, h('h4', null, t('investigationEvidence')),
      ...preparation.observations.map(observation => h('article', { key: 'observation:' + observation.invocation.id },
        h('h4', null, observation.invocation['action-id'] + ' · ' + observation.status),
        h('p', { className: css.meta }, observation.invocation.id + ' · ' + observation['duration-ms'] + ' ms'),
        h(InvestigationExecution, { value: observation.execution, t }),
        observation.truncated && h('p', { className: css.warningText }, t('investigationPartial')),
        h('pre', { className: css.raw }, observation.output || observation.error)))),
    (preparation.decisions?.length > 0 || preparation['decision-effects']?.length > 0) && h('section', { className: css.details }, h('h4', null, t('investigationDecision')),
      ...(preparation.decisions ?? []).map(receipt => h('p', { key: 'receipt:' + receipt['decision-id'] }, receipt['decision-id'] + ' · ' + receipt.status + ' · ' + receipt.reason)),
      ...preparation.observations.map(observation => h('p', { key: 'observation:' + observation.invocation.id }, observation.invocation['action-id'] + ' · ' + t(observation.invocation.adopted === true ? 'investigationAdopted' : 'investigationBaseline') + ' · ' + observation.invocation.reason)),
      ...(preparation['decision-effects'] ?? []).map(effect => h('article', { key: 'effect:' + effect['decision-id'] },
        h('h4', null, t('investigationEffect_' + effect.kind)),
        h('p', { className: css.meta }, effect['decision-id']),
        effect.actions.length > 0 && h('p', null, t('investigationSkipped') + ': ' + effect.actions.join(', ')),
        h('p', { className: css.meta }, t('investigationCitations') + ': ' + effect.evidence.join(', '))))));
}

Object.assign(zh, {
  runtimeDecisionTitle: 'JEV 执行决策', runtimeDecisionHelp: '展示引擎已记录的选择与执行结果；建议被接受不代表动作已经完成。', runtimeDecisionUsage: '任务请求 {used}/{limit} · 其中调查移交 {imported}', runtimeDecisionOmitted: '显示最近 {shown} 条，共 {total} 条；完整记录保留在任务账本',
  runtimeDecisionPending: '决策请求待核对', runtimeDecisionApplied: '控制决策已应用 · 后续执行见节点状态', runtimeDecisionCompleted: '控制动作已收尾 · 任务结果见节点状态', runtimeDecisionUnknown: '结果未知 · 核对原回执', runtimeDecisionStale: '输入已变化 · 未采用', runtimeDecisionBaseline: '沿用基线', runtimeDecisionAdopted: '采用 JEV 选择', runtimeDecisionRepair: '继续局部修复', runtimeDecisionReplan: '请求重新规划', runtimeDecisionModel: '切换批准的模型配置', runtimeDecisionFeedback: '补充短反馈检查', runtimeDecisionRecheck: '复查失败的验收项', runtimeDecisionFeedbackRunning: '反馈检查未收尾 · 恢复前核对原执行', runtimeDecisionFeedbackCompleted: '反馈检查已收尾', runtimeDecisionFeedbackUnknown: '反馈结果未知 · 核对原执行，禁止重放', runtimeDecisionFeedbackStale: '反馈改变了候选或验证输入 · 已阻断', runtimeDecisionFeedbackHelp: '这是诊断反馈；最终仍执行完整验收。', runtimeDecisionCall: '决策请求', runtimeDecisionReason: '原因', runtimeDecisionAttempt: '执行尝试', runtimeDecisionStatus: '最近观察状态', runtimeDecisionResultsOmitted: '省略了 {count} 项较早的反馈结果', runtimeDecisionIncomplete: '反馈结果不完整',
  runtimeDecisionReplanRunning: '重规划未收尾 · 恢复前核对原会话', runtimeDecisionUnapplied: 'JEV 选择未应用', runtimeModelHistory: '执行配置切换记录', runtimeModelHistoryHelp: '这里记录已应用的配置切换。实际启动的模型与会话以节点会话记录为准。', runtimeModelFrom: '切换前', runtimeModelTo: '切换后', runtimeModelJev: '采用 JEV 模型选择', runtimeModelInitialJev: '采用 JEV 首轮配置选择',
});
Object.assign(en, {
  runtimeDecisionTitle: 'JEV execution decisions', runtimeDecisionHelp: 'Recorded choices and effects from the engine. Accepting advice does not mean its action has completed.', runtimeDecisionUsage: 'Task requests {used}/{limit} · {imported} imported from investigation', runtimeDecisionOmitted: 'Showing {shown} of {total} records; the complete history remains in the task ledger',
  runtimeDecisionPending: 'Decision request needs reconciliation', runtimeDecisionApplied: 'Control decision applied · see node for subsequent execution', runtimeDecisionCompleted: 'Control action settled · see node for task outcome', runtimeDecisionUnknown: 'Unknown result · reconcile original receipt', runtimeDecisionStale: 'Inputs changed · not adopted', runtimeDecisionBaseline: 'Original baseline', runtimeDecisionAdopted: 'JEV choice accepted', runtimeDecisionRepair: 'Continue local repair', runtimeDecisionReplan: 'Request replanning', runtimeDecisionModel: 'Switch approved model configuration', runtimeDecisionFeedback: 'Collect a short feedback check', runtimeDecisionRecheck: 'Recheck failed acceptance', runtimeDecisionFeedbackRunning: 'Feedback action unsettled · reconcile before resuming', runtimeDecisionFeedbackCompleted: 'Feedback action settled', runtimeDecisionFeedbackUnknown: 'Unknown feedback result · reconcile original action, no replay', runtimeDecisionFeedbackStale: 'Feedback changed candidate or verification inputs · blocked', runtimeDecisionFeedbackHelp: 'This is diagnostic feedback. Final acceptance still runs in full.', runtimeDecisionCall: 'Decision request', runtimeDecisionReason: 'Reason', runtimeDecisionAttempt: 'Execution attempt', runtimeDecisionStatus: 'Last observed status', runtimeDecisionResultsOmitted: '{count} earlier feedback results omitted', runtimeDecisionIncomplete: 'Feedback observation is incomplete',
  runtimeDecisionReplanRunning: 'Replanning unsettled · reconcile original session before resuming', runtimeDecisionUnapplied: 'JEV choice not applied', runtimeModelHistory: 'Execution configuration changes', runtimeModelHistoryHelp: 'These are applied configuration changes. Actual model dispatch and sessions remain in the node session records.', runtimeModelFrom: 'Before', runtimeModelTo: 'After', runtimeModelJev: 'JEV model selection applied', runtimeModelInitialJev: 'JEV initial configuration applied',
});

Object.assign(zh, {
  runtimeDecisionInitial: '首次执行前的选择', runtimeDecisionFailure: '失败后的调整', runtimeDecisionPhaseUnknown: '决策阶段未记录',
  runtimeDecisionSuggested: 'JEV 建议', runtimeDecisionChosen: '引擎已选动作', runtimeDecisionInitialBaseline: '初始基线',
  runtimeDecisionRecordedAction: '记录的控制动作 · 尚未应用', runtimeDecisionBaselineChoice: '本次决策基线',
  runtimeDecisionUnknownAction: '已记录其他动作，详见原始证据', runtimeModelQuality: '按已配置规则升级模型', runtimeModelChannel: '切换已授权账号渠道', runtimeModelOther: '已记录配置变更',
  runtimeDecisionNoSuggestion: '没有可用的模型建议', runtimeDecisionBindingUnknown: '此历史记录没有保存完整模型配置',
  runtimeDecisionBindingsOmitted: '本视图省略了 {count} 项候选配置；完整记录保留在任务账本。', runtimeDecisionBindingUnshown: '本视图未包含该候选的完整配置。',
  runtimeDecisionKeptBaseline: '保留初始基线 · 没有切换模型', runtimeDecisionNotDispatched: '配置选择不代表模型已经启动',
  runtimeBindingID: '候选配置', runtimeDecisionEvidence: '展开原始决策证据', runtimeBindingEvidence: '展开原始配置证据',
  runtimeDecisionReasonOther: '引擎记录了其他原因，详见原始证据',
  decisionReason_masterDisabled: 'JEV 总开关已关闭', decisionReason_baseline: '沿用确定的原路径',
  decisionReason_model: '建议满足模型选择的采用条件', decisionReason_repair: '建议继续原范围内修复',
  decisionReason_replan: '建议在原目标内重新规划', decisionReason_diagnostic: '建议执行已批准的反馈检查',
  decisionReason_stale: '决策所依据的输入已变化', decisionReason_ineligible: '本次不满足建议采用条件',
  decisionReason_abstained: '决策没有返回可采用的建议', decisionReason_lowConfidence: '建议未达到适用性或选择阈值',
  decisionReason_family: '该类动作未获授权', decisionReason_unknown: '原执行结果尚未确认',
  decisionReason_evaluated: '已收到决策结果', decisionReason_cancelled: '请求已取消，原结果需核对',
  decisionReason_notSent: '请求在派发前取消', decisionReason_budget: '已达到决策请求上限',
  decisionReason_data: '证据类别未获授权', decisionReason_hardBudget: '当前预算约束无法证明满足',
  decisionReason_oneAction: '只有一项合法动作，无需模型选择', decisionReason_deterministic: '可直接确定后续动作',
  decisionReason_offline: '仅记录离线证据', decisionReason_missingAdapter: '订阅决策适配器不可用',
  decisionReason_missingKey: '所配置的 API 凭据不可用',
  decisionReason_progress: '当前执行进展不足',
  runtimeLedger_enabled: '已开启', runtimeLedger_disabled: '已关闭', 'runtimeLedger_not-configured': '未配置',
  'runtimeLedger_config-unavailable': '配置暂不可用', 'runtimeLedger_policy-changed': '配置已变化，沿用原路径',
  'runtimeLedger_native-cleanup-unconfirmed': '订阅调用收尾尚未确认', runtimeLedger_unknown: '状态未识别，详见原始证据',
  wbBindingInitial: '运行初始配置', wbBindingSelected: '下一次已选配置', wbBindingActual: '最近实际调用',
  wbBindingRecorded: '最近调用记录 · 派发未确认', wbBindingNotSent: '最近调用记录 · 未派发',
  wbBindingInitialUnknown: '此运行没有保存可确认的初始模型配置', wbBindingSelectedUnknown: '尚无可确认的后续模型选择',
  wbBindingNoCall: '尚无已记录的模型调用', wbBindingMissing: '此记录没有保存完整模型配置',
  wbBindingModelUnspecified: '未指定具体模型',
  wbBindingNativeMode: '请求的协作方式', wbBindingNativeModeUnknown: '此记录未保留可确认的协作方式',
  wbBindingModeNative: '普通原生编排（native）', wbBindingModeSubagents: '子智能体编排（subagents）', wbBindingModeSwarm: '蜂群协作（swarm）',
  wbBindingNativeOmitted: '原生配置正文已从本视图省略；原生配置仍保留在任务账本。', wbBindingNativeHash: '原生配置 SHA256',
  wbBindingsOmitted: '本视图省略了 {count} 项初始配置；完整记录保留在任务账本。', wbBindingUnshown: '本视图未包含当前阶段的完整配置。',
  wbBindingHelp: '初始配置、后续选择与调用事实分别记录。设置修改不会重写旧会话。',
  wbBindingDeclared: '旧版运行声明 · 不作为实际模型证明', wbConfigurationSource: '本次运行的配置来源',
  wbBindingDispatch: '派发状态', wbBindingOutcome: '调用结果', wbBindingInvocation: '调用标识',
  wbDispatchAccepted: '已确认派发', wbDispatchNotSent: '未派发', wbDispatchUnknown: '派发未确认',
  wbOutcomeCompleted: '已完成', wbOutcomeFailed: '失败', wbOutcomeCancelled: '已取消', wbOutcomeUnknown: '结果未知',
  wbCollaboration: '已审协作分工', wbCollaborationHelp: '这是已批准的任务和写入范围；实际子智能体观察另列。',
  wbCollaborationWrites: '允许写入范围', wbCollaborationUnknown: '协作契约身份尚未验证',
  wbCollaborationUnshown: '协作分工超出本视图的显示范围；此处无法核对完整的已审分工。',
  wbCollaborationNone: '此节点没有已审协作分工', wbCollaborationRead: '读取节点协作证据',
  wbCollaborationEvidence: '展开原始协作契约证据',
  wbNativeRead: '读取本次调用原生观察', wbNativeMissing: '本次调用尚无可用的原生观察，不能据此推断没有子智能体。',
  wbNativeUnrequested: '尚无可关联的 Antigravity 调用', wbNativeOlder: '当前指标没有包含这次调用；较早记录可能超出显示窗口。',
});
Object.assign(en, {
  runtimeDecisionInitial: 'Choice before first execution', runtimeDecisionFailure: 'Adjustment after a failure', runtimeDecisionPhaseUnknown: 'Decision phase not recorded',
  runtimeDecisionSuggested: 'JEV recommendation', runtimeDecisionChosen: 'Action selected by the engine', runtimeDecisionInitialBaseline: 'Initial baseline',
  runtimeDecisionRecordedAction: 'Recorded control action · not applied', runtimeDecisionBaselineChoice: 'Baseline for this decision',
  runtimeDecisionUnknownAction: 'Another action was recorded; see original evidence', runtimeModelQuality: 'Model upgraded by configured policy', runtimeModelChannel: 'Authorized account channel changed', runtimeModelOther: 'Recorded binding change',
  runtimeDecisionNoSuggestion: 'No usable model recommendation', runtimeDecisionBindingUnknown: 'This history did not retain a complete model binding',
  runtimeDecisionBindingsOmitted: '{count} candidate bindings were omitted from this view; complete records remain in the task ledger.', runtimeDecisionBindingUnshown: 'This view does not include the complete binding for this candidate.',
  runtimeDecisionKeptBaseline: 'Initial baseline retained · no model switch', runtimeDecisionNotDispatched: 'Selecting a binding does not establish model dispatch',
  runtimeBindingID: 'Candidate binding', runtimeDecisionEvidence: 'Expand original decision evidence', runtimeBindingEvidence: 'Expand original binding evidence',
  runtimeDecisionReasonOther: 'The engine recorded another reason; see original evidence',
  decisionReason_masterDisabled: 'The JEV master switch is off', decisionReason_baseline: 'The deterministic original path was retained',
  decisionReason_model: 'The recommendation met model-selection adoption checks', decisionReason_repair: 'Continue repair within the approved scope',
  decisionReason_replan: 'Replan within the original goal', decisionReason_diagnostic: 'Run an approved feedback check',
  decisionReason_stale: 'The inputs used by this decision changed', decisionReason_ineligible: 'Adoption conditions were not met',
  decisionReason_abstained: 'No usable recommendation was returned', decisionReason_lowConfidence: 'Applicability or choice thresholds were not met',
  decisionReason_family: 'This action family is not authorized', decisionReason_unknown: 'The original execution outcome is unconfirmed',
  decisionReason_evaluated: 'A decision result was received', decisionReason_cancelled: 'The request was cancelled; reconcile its original outcome',
  decisionReason_notSent: 'The request was cancelled before dispatch', decisionReason_budget: 'The decision request limit was reached',
  decisionReason_data: 'The evidence category is not authorized', decisionReason_hardBudget: 'The current budget bound cannot be established',
  decisionReason_oneAction: 'Only one legal action remains; no model choice is needed', decisionReason_deterministic: 'The next action can be determined directly',
  decisionReason_offline: 'Offline evidence recorded only', decisionReason_missingAdapter: 'The subscription decision adapter is unavailable',
  decisionReason_missingKey: 'The configured API credential is unavailable',
  decisionReason_progress: 'The current execution made insufficient progress',
  runtimeLedger_enabled: 'Enabled', runtimeLedger_disabled: 'Disabled', 'runtimeLedger_not-configured': 'Not configured',
  'runtimeLedger_config-unavailable': 'Configuration unavailable', 'runtimeLedger_policy-changed': 'Configuration changed; original path retained',
  'runtimeLedger_native-cleanup-unconfirmed': 'Subscription invocation cleanup is unconfirmed', runtimeLedger_unknown: 'Unrecognized status; see original evidence',
  wbBindingInitial: 'Initial run binding', wbBindingSelected: 'Binding selected for the next execution', wbBindingActual: 'Latest confirmed dispatch',
  wbBindingRecorded: 'Latest invocation record · dispatch unconfirmed', wbBindingNotSent: 'Latest invocation record · not dispatched',
  wbBindingInitialUnknown: 'This run did not retain a confirmed initial model binding', wbBindingSelectedUnknown: 'No confirmed next binding has been recorded',
  wbBindingNoCall: 'No model invocation has been recorded', wbBindingMissing: 'This record did not retain a complete model binding',
  wbBindingModelUnspecified: 'No specific model was selected',
  wbBindingNativeMode: 'Requested collaboration mode', wbBindingNativeModeUnknown: 'This record did not retain a confirmed collaboration mode',
  wbBindingModeNative: 'Native orchestration (native)', wbBindingModeSubagents: 'Subagent orchestration (subagents)', wbBindingModeSwarm: 'Swarm collaboration (swarm)',
  wbBindingNativeOmitted: 'The native options body was omitted from this view; the native configuration remains in the task ledger.', wbBindingNativeHash: 'Native options SHA256',
  wbBindingsOmitted: '{count} initial bindings were omitted from this view; complete records remain in the task ledger.', wbBindingUnshown: 'This view does not include the complete binding for this stage.',
  wbBindingHelp: 'Initial bindings, subsequent choices and invocation facts are separate. Settings changes do not rewrite old sessions.',
  wbBindingDeclared: 'Legacy run declaration · not proof of the actual model', wbConfigurationSource: 'Configuration source for this run',
  wbBindingDispatch: 'Dispatch', wbBindingOutcome: 'Invocation outcome', wbBindingInvocation: 'Invocation ID',
  wbDispatchAccepted: 'Dispatch confirmed', wbDispatchNotSent: 'Not dispatched', wbDispatchUnknown: 'Dispatch unconfirmed',
  wbOutcomeCompleted: 'Completed', wbOutcomeFailed: 'Failed', wbOutcomeCancelled: 'Cancelled', wbOutcomeUnknown: 'Unknown outcome',
  wbCollaboration: 'Reviewed collaboration tasks', wbCollaborationHelp: 'These are approved tasks and write scopes. Actual child observations are shown separately.',
  wbCollaborationWrites: 'Allowed write scope', wbCollaborationUnknown: 'The collaboration contract identity has not been verified',
  wbCollaborationUnshown: 'The collaboration tasks exceed this view\'s display limit; the complete reviewed tasks cannot be verified here.',
  wbCollaborationNone: 'No reviewed collaboration tasks are recorded for this node', wbCollaborationRead: 'Read node collaboration evidence',
  wbCollaborationEvidence: 'Expand original collaboration contract evidence',
  wbNativeRead: 'Read native observations for this invocation', wbNativeMissing: 'No native observation is available for this invocation. This does not prove that it had no children.',
  wbNativeUnrequested: 'No Antigravity invocation is available to match', wbNativeOlder: 'The current metrics do not include this invocation. Older records may be outside the displayed window.',
});

function validDisplayBinding(value) {
  return record(value) && typeof value.harness === 'string' && value.harness.length > 0 && typeof value.model === 'string' &&
    ['provider', 'channel', 'thinking', 'nativeProfile', 'permission', 'permissionMode', 'sessionMode', 'profile']
      .every(key => value[key] == null || typeof value[key] === 'string');
}

function validBindingChoices(value, omitted) {
  return (omitted == null || Number.isSafeInteger(omitted) && omitted >= 0) &&
    (value == null || record(value) && Object.values(value).every(choice => record(choice) &&
      (choice.bindingId == null || typeof choice.bindingId === 'string') && validDisplayBinding(choice.binding) &&
      (choice.nativeOptionsOmitted == null && choice.nativeOptionsHash == null || choice.nativeOptionsOmitted === true &&
        typeof choice.nativeOptionsHash === 'string' && /^[0-9a-f]{64}$/.test(choice.nativeOptionsHash) && choice.binding.nativeOptions == null)));
}

function bindingEffortLabel(binding, t) {
  if (binding?.thinking == null) return t('unknown');
  if (fixedAntigravityEffort(binding) && binding.thinking === '') return t('cfgEffortFixed');
  const key = { off: 'cfgEffortOff', minimal: 'cfgEffortMinimal', low: 'cfgEffortLow', medium: 'cfgEffortMedium', high: 'cfgEffortHigh', xhigh: 'cfgEffortXhigh', max: 'cfgEffortMax' }[binding?.thinking];
  return key ? t(key) : binding?.thinking || t('default');
}

function BindingProperties({ binding, bindingId, nativeOptionsOmitted, nativeOptionsHash, t }) {
  if (!binding) return h('p', { className: css.meta }, t('wbBindingMissing'));
  const property = (label, value) => h(React.Fragment, { key: label }, h('dt', null, label), h('dd', null, textValue(value)));
  const permission = binding.permission ?? binding.permissionMode;
  const nativeMode = binding.nativeOptions?.mode;
  const nativeModeLabel = ['native', 'subagents', 'swarm'].includes(nativeMode)
    ? { native: 'wbBindingModeNative', subagents: 'wbBindingModeSubagents', swarm: 'wbBindingModeSwarm' }[nativeMode]
    : 'wbBindingNativeModeUnknown';
  return h('dl', { className: css.consoleProperties },
    bindingId && property(t('runtimeBindingID'), bindingId),
    property(t('consoleProvider'), bindingCaption({ ...binding, model: '' })),
    property(t('consoleModel'), binding.model === '' ? t('wbBindingModelUnspecified') : binding.model),
    binding.profile && property(t('profile'), binding.profile),
    binding.channel && property(t('channel'), binding.channel),
    property(t('consoleThinking'), bindingEffortLabel(binding, t)),
    (binding.nativeProfile || binding['native-profile']) && property(t('cfgNative'), binding.nativeProfile || binding['native-profile']),
    binding.harness === 'antigravity' && property(t('wbBindingNativeMode'), t(nativeModeLabel)),
    nativeOptionsOmitted && h(React.Fragment, null,
      h('dt', null, t('cfgNative')), h('dd', { 'data-native-options-omitted': true }, t('wbBindingNativeOmitted')),
      h('dt', null, t('wbBindingNativeHash')), h('dd', { className: css.path }, nativeOptionsHash)),
    permission && property(t('permission'), permissionLabels[permission] ? t(permissionLabels[permission]) : permission),
    (binding.sessionMode || binding.session?.mode) && property(t('session'), sessionModeLabel({ session: { mode: binding.sessionMode || binding.session?.mode } }, t)));
}

function RawEvidence({ value, label, t }) {
  return h('details', { className: css.details + ' ' + css.evidenceToggle, 'data-raw-evidence': true },
    h('summary', null, t(label)), h('pre', { className: css.raw }, JSON.stringify(value, null, 2)));
}

function runtimeDecisionReason(reason, t) {
  const code = typeof reason === 'string' ? reason.split('; decision ')[0] : '';
  const key = {
    'master-disabled': 'masterDisabled', 'deterministic-baseline': 'baseline', 'approved-model-choice': 'model',
    'approved-repair-choice': 'repair', 'approved-replan-choice': 'replan', 'approved-diagnostic-choice': 'diagnostic',
    'stale-decision': 'stale', 'adoption-not-eligible': 'ineligible', 'decision-abstained': 'abstained',
    'decision-not-applicable': 'lowConfidence', 'action-family-not-enabled': 'family',
    'invalid-or-unknown-execution': 'unknown', 'executor-outcome-unknown': 'unknown', 'reserved-without-terminal': 'unknown',
    evaluated: 'evaluated', 'cancelled-after-dispatch': 'cancelled', 'cancelled-before-dispatch': 'notSent',
    'decision-call-budget': 'budget', 'data-not-authorized': 'data', 'hard-budget-without-enforceable-bound': 'hardBudget',
    'one-legal-action': 'oneAction', 'deterministic-choice': 'deterministic', 'offline-no-network': 'offline',
    'snapshot-captured': 'offline', 'offline-recorded-response': 'evaluated', 'missing-subscription-adapter': 'missingAdapter', 'missing-api-key': 'missingKey', 'insufficient-progress': 'progress',
  }[code];
  return key ? t('decisionReason_' + key) : /^[a-z][a-z0-9]*(?:-[a-z0-9]+)+$/.test(code) ? t('runtimeDecisionReasonOther') : textValue(reason);
}

Object.assign(zh, {
  runtimeExecutionTitle: '对应执行的状态', runtimeExecution_selected: '已选择，尚未启动执行调用',
  'runtimeExecution_not-selected': '此选择尚未采用', 'runtimeExecution_not-started': '已确认未派发，执行调用未启动',
  runtimeExecution_started: '已启动，执行结果或收尾仍待确认', runtimeExecution_completed: '对应执行调用已完成',
  runtimeExecution_failed: '对应执行调用失败', runtimeExecution_cancelled: '对应执行调用已取消',
  runtimeExecution_unknown: '执行状态未知，缺少可核对的调用关联或回执',
  runtimeExecutionBoundary: '这是关联调用的状态；任务验收和实际子智能体结果另行记录。',
});
Object.assign(en, {
  runtimeExecutionTitle: 'Linked execution status', runtimeExecution_selected: 'Selected; execution has not started',
  'runtimeExecution_not-selected': 'This selection has not been applied', 'runtimeExecution_not-started': 'Confirmed not dispatched; execution did not start',
  runtimeExecution_started: 'Started; execution outcome or cleanup is unconfirmed', runtimeExecution_completed: 'Linked execution call completed',
  runtimeExecution_failed: 'Linked execution call failed', runtimeExecution_cancelled: 'Linked execution call cancelled',
  runtimeExecution_unknown: 'Execution status unknown; a matching call or receipt is unavailable',
  runtimeExecutionBoundary: 'This is the linked call status. Task acceptance and observed child-agent outcomes are recorded separately.',
});

function validDecisionExecutionView(value) {
  if (value == null) return true;
  if (!record(value) || !['not-selected', 'selected', 'not-started', 'started', 'completed', 'failed', 'cancelled', 'unknown'].includes(value.status) ||
      typeof value.reason !== 'string' || value.reason.length > 256) return false;
  const call = value.invocation;
  if (call != null && (!record(call) || typeof call.id !== 'string' || !/^call-[A-Za-z0-9]+$/.test(call.id) || call.id.length > 160 ||
      typeof call['started-at'] !== 'string' || !Number.isSafeInteger(call.attempt) || call.attempt < 1 ||
      !['intent', 'received', 'consumed', 'blocked', 'abandoned'].includes(call.status) ||
      !['not-dispatched', 'dispatched', 'unknown'].includes(call.dispatch) || !['completed', 'failed', 'cancelled', 'unknown'].includes(call.outcome) ||
      typeof call['cleanup-confirmed'] !== 'boolean' || call['dispatch-observed'] != null && typeof call['dispatch-observed'] !== 'boolean' ||
      call['engine-run-id'] != null && typeof call['engine-run-id'] !== 'string')) return false;
  if (['completed', 'failed', 'cancelled'].includes(value.status)) return call?.dispatch === 'dispatched' && call.outcome === value.status && call['cleanup-confirmed'] && ['received', 'consumed'].includes(call.status);
  if (value.status === 'not-started') return call?.dispatch === 'not-dispatched' && call['dispatch-observed'] !== true;
  if (value.status === 'started') return call?.dispatch === 'dispatched' || call?.dispatch === 'unknown' && call['dispatch-observed'] === true;
  return value.status === 'unknown' || call == null;
}

function DecisionExecutionStatus({ entry, t }) {
  if (!/^model-\d+$/.test(entry.action)) return null;
  const view = entry['execution-view'];
  const status = view?.status ?? 'unknown';
  const call = view?.invocation;
  return h('section', { className: css.observationBlock, 'data-decision-execution': status },
    h('h5', null, t('runtimeExecutionTitle')), h('p', null, t('runtimeExecution_' + status)),
    call && h('p', { className: css.path }, t('wbBindingInvocation') + ': ' + call.id + ' · ' + t('runtimeDecisionAttempt') + ' ' + call.attempt),
    h('p', { className: css.meta }, t('runtimeExecutionBoundary')));
}

function validRuntimeDecisions(value) {
  const strings = (object, keys) => record(object) && keys.every(key => typeof object[key] === 'string');
  const optionalStrings = (object, keys) => keys.every(key => object[key] == null || typeof object[key] === 'string');
  const count = number => Number.isSafeInteger(number) && number >= 0;
  const feedback = report => report == null || record(report) && typeof report.pass === 'boolean' && Array.isArray(report.results) && report.results.every(result =>
    record(result) && count(result['omitted-items']) && Object.keys(result).length === 1 ||
    strings(result, ['cmd']) && typeof result.pass === 'boolean' && Number.isSafeInteger(result.exitCode) &&
    ['timedOut', 'cancelled', 'truncated', 'outputTruncated', 'observationComplete'].every(key => result[key] == null || typeof result[key] === 'boolean') &&
    optionalStrings(result, ['output', 'signal', 'cleanupError', 'spawnError', 'checkId']));
  return value == null || record(value) && value.version === 1 && count(value.used) && count(value['imported-calls']) && typeof value.status === 'string' &&
    (value['max-calls'] == null || count(value['max-calls'])) && (value['omitted-records'] == null || count(value['omitted-records'])) &&
    Array.isArray(value.records) && value.records.every(entry =>
      strings(entry, ['id', 'packet', 'action', 'status', 'started-at']) && count(entry.attempt) &&
      (entry['plan-revisions'] == null || count(entry['plan-revisions'])) && (entry.accounted == null || typeof entry.accounted === 'boolean') &&
      (entry.selection == null || strings(entry.selection, ['action-id', 'reason']) && typeof entry.selection.adopted === 'boolean') &&
      (entry.receipt == null || strings(entry.receipt, ['status', 'reason']) && optionalStrings(entry.receipt, ['model', 'action-id'])) &&
      (entry.phase == null || ['initial-execution', 'after-failure'].includes(entry.phase)) &&
      optionalStrings(entry, ['baseline-action-id']) && validBindingChoices(entry['model-bindings'], entry['model-bindings-omitted']) && validDecisionExecutionView(entry['execution-view']) &&
      (entry.actions == null || Array.isArray(entry.actions) && entry.actions.length <= 16 && entry.actions.every(action => strings(action, ['id', 'kind', 'description']))) && feedback(entry.feedback));
}

function runtimeDecisionAction(action, t) {
  const kind = action.match(/^(model|feedback|recheck)-\d+$/)?.[1] ?? action;
  const label = { repair: 'runtimeDecisionRepair', replan: 'runtimeDecisionReplan', model: 'runtimeDecisionModel', feedback: 'runtimeDecisionFeedback', recheck: 'runtimeDecisionRecheck' }[kind];
  return label ? t(label) : t('runtimeDecisionUnknownAction');
}

function DecisionActionDetails({ entry, action, title, t }) {
  const choice = action && entry['model-bindings']?.[action];
  return h('section', { className: css.observationBlock, 'data-decision-choice': title },
    h('h5', null, t(title)),
    !action ? h('p', { className: css.meta }, t('runtimeDecisionNoSuggestion')) : choice
      ? h(BindingProperties, { ...choice, t })
      : h(React.Fragment, null, h('p', null, runtimeDecisionAction(action, t)),
        /^model-\d+$/.test(action) && h('p', { className: css.meta }, t(entry['model-bindings-omitted'] > 0 ? 'runtimeDecisionBindingUnshown' : 'runtimeDecisionBindingUnknown'))));
}

function RuntimeDecisionPanel({ ledger, packetID, t }) {
  if (!ledger) return null;
  const entries = ledger.records.filter(entry => !packetID || entry.packet === packetID);
  if (packetID && entries.length === 0) return null;
  const statuses = { pending: 'runtimeDecisionPending', baseline: 'runtimeDecisionBaseline', applied: 'runtimeDecisionApplied', completed: 'runtimeDecisionCompleted', stale: 'runtimeDecisionStale', 'replan-running': 'runtimeDecisionReplanRunning', 'feedback-running': 'runtimeDecisionFeedbackRunning', 'feedback-completed': 'runtimeDecisionFeedbackCompleted', 'feedback-unknown': 'runtimeDecisionFeedbackUnknown', 'feedback-stale': 'runtimeDecisionFeedbackStale' };
  const total = ledger.records.length + (ledger['omitted-records'] ?? 0);
  return h('section', { className: css.details, 'data-runtime-decisions': packetID ?? 'task' },
    h('h4', null, t('runtimeDecisionTitle')),
    h('p', { className: css.meta }, t('runtimeDecisionHelp')),
    h('p', { className: css.meta }, t('runtimeDecisionUsage', { used: ledger.used, limit: ledger['max-calls'] ?? '—', imported: ledger['imported-calls'] })),
    h('p', { className: css.meta }, t('runtimeDecisionStatus') + ': ' + t(['enabled', 'disabled', 'not-configured', 'config-unavailable', 'policy-changed', 'native-cleanup-unconfirmed'].includes(ledger.status) ? 'runtimeLedger_' + ledger.status : 'runtimeLedger_unknown')),
    !packetID && total > entries.length && h('p', { className: css.meta }, t('runtimeDecisionOmitted', { shown: entries.length, total })),
    ...entries.map(entry => h('article', { key: entry.id, 'data-decision-id': entry.id },
      h('h4', null, entry.packet + ' · ' + t(entry.phase === 'initial-execution' ? 'runtimeDecisionInitial' : entry.phase === 'after-failure' ? 'runtimeDecisionFailure' : 'runtimeDecisionPhaseUnknown')),
      h('p', { className: css.meta }, entry.id + ' · ' + t('runtimeDecisionAttempt') + ' ' + entry.attempt + ' · ' + entry['started-at']),
      h('p', { className: ['pending', 'replan-running', 'feedback-running', 'feedback-unknown', 'feedback-stale'].includes(entry.status) ? css.warningText : css.meta }, statuses[entry.status] ? t(statuses[entry.status]) : t('runtimeDecisionUnknown')),
      entry.selection && h('p', { 'data-decision-adoption': true }, t(!entry.selection.adopted ? 'runtimeDecisionBaseline' : ['stale', 'baseline', 'pending'].includes(entry.status) ? 'runtimeDecisionUnapplied' : 'runtimeDecisionAdopted') + ' · ' + runtimeDecisionReason(entry.selection.reason, t)),
      entry.receipt && h('p', { className: css.meta }, t('runtimeDecisionCall') + ': ' + runtimeDecisionReason(entry.receipt.reason, t)),
      entry.receipt?.status === 'unknown' && h('p', { className: css.warningText }, t('runtimeDecisionUnknown')),
      entry['model-bindings-omitted'] > 0 && h('p', { className: css.meta, 'data-model-bindings-omitted': true }, t('runtimeDecisionBindingsOmitted', { count: entry['model-bindings-omitted'] })),
      entry['baseline-action-id'] && h(DecisionActionDetails, { entry, action: entry['baseline-action-id'], title: entry.phase === 'initial-execution' ? 'runtimeDecisionInitialBaseline' : 'runtimeDecisionBaselineChoice', t }),
      h(DecisionActionDetails, { entry, action: entry.receipt?.['action-id'], title: 'runtimeDecisionSuggested', t }),
      h(DecisionActionDetails, { entry, action: entry.action, title: ['pending', 'stale'].includes(entry.status) ? 'runtimeDecisionRecordedAction' : 'runtimeDecisionChosen', t }),
      entry.phase === 'initial-execution' && entry.action === entry['baseline-action-id'] && ['baseline', 'applied'].includes(entry.status) && h('p', { className: css.meta, 'data-baseline-retained': true }, t('runtimeDecisionKeptBaseline')),
      h(DecisionExecutionStatus, { entry, t }),
      h('p', { className: css.meta }, t('runtimeDecisionNotDispatched')),
      h(RawEvidence, { value: entry, label: 'runtimeDecisionEvidence', t }),
      entry.feedback && h('div', { 'data-decision-feedback': entry.id },
        h('p', { className: css.meta }, t('runtimeDecisionFeedbackHelp')),
        ...entry.feedback.results.map((result, index) => result['omitted-items'] != null
          ? h('p', { key: index, className: css.meta }, t('runtimeDecisionResultsOmitted', { count: result['omitted-items'] }))
          : h('article', { key: index },
          h('p', null, result.cmd),
          h('p', { className: css.meta }, t('investigationExit') + ': ' + result.exitCode + ' · ' + (result.pass ? t('pass') : t('fail'))),
          result.timedOut && h('p', { className: css.warningText }, t('investigationTimedOut')),
          result.cancelled && h('p', { className: css.warningText }, t('investigationCancelled')),
          result.observationComplete === false && h('p', { className: css.warningText }, t('runtimeDecisionIncomplete')),
          (result.truncated || result.outputTruncated) && h('p', { className: css.warningText }, t('investigationPartial')),
          result.signal && h('p', { className: css.warningText }, t('investigationSignal') + ': ' + result.signal),
          (result.cleanupError || result.spawnError) && h('p', { className: css.warningText }, result.cleanupError || result.spawnError),
          result.output && h('pre', { className: css.raw }, result.output)))))));
}

function validExecutionPolicyHistory(packet) {
  const policy = packet?.['execution-policy'];
  const binding = value => record(value) && ['harness', 'provider', 'channel', 'model', 'thinking', 'nativeProfile', 'permission', 'sessionMode']
    .every(key => value[key] == null || typeof value[key] === 'string');
  return record(packet) && (policy == null || record(policy) &&
    (policy.currentBindingOmitted == null || typeof policy.currentBindingOmitted === 'boolean' &&
      (!policy.currentBindingOmitted || typeof policy.current === 'string' && policy.current.length > 0)) &&
    (policy.currentBinding == null || policy.currentBindingOmitted !== true &&
      validBindingChoices({ current: policy.currentBinding }) && typeof policy.current === 'string' && policy.current.length > 0 && policy.currentBinding.bindingId === policy.current) &&
    (policy.history == null || Array.isArray(policy.history) && policy.history.every(change =>
      record(change) && ['kind', 'from', 'to', 'reason', 'at'].every(key => typeof change[key] === 'string') &&
      Number.isSafeInteger(change.attempt) && change.attempt >= 0 && binding(change.before) && binding(change.after))));
}

function ExecutionPolicyChanges({ packets, packetID, t }) {
  const changes = Object.entries(packets ?? {}).flatMap(([id, packet]) => !packetID || id === packetID
    ? (packet['execution-policy']?.history ?? []).map(change => ({ id, change })) : []);
  if (!changes.length) return null;
  const bindingDetails = (binding, label) => h('div', null,
    h('p', null, t(label)),
    h(BindingProperties, { binding, t }));
  return h('section', { className: css.details, 'data-execution-switches': packetID ?? 'task' },
    h('h4', null, t('runtimeModelHistory')),
    h('p', { className: css.meta }, t('runtimeModelHistoryHelp')),
    ...changes.map(({ id, change }, index) => h('article', { key: index },
      h('h4', null, id + ' · ' + change.from + ' → ' + change.to),
      h('p', { className: css.meta }, t({ 'jev-model-selection': 'runtimeModelJev', 'jev-initial-selection': 'runtimeModelInitialJev', 'quality-upgrade': 'runtimeModelQuality', 'channel-fallback': 'runtimeModelChannel' }[change.kind] || 'runtimeModelOther') + ' · ' + t('runtimeDecisionAttempt') + ' ' + change.attempt + ' · ' + change.at),
      h('p', null, t('runtimeDecisionReason') + ': ' + runtimeDecisionReason(change.reason, t)),
      bindingDetails(change.before, 'runtimeModelFrom'),
      bindingDetails(change.after, 'runtimeModelTo'),
      h(RawEvidence, { value: change, label: 'runtimeBindingEvidence', t }))));
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
  wbExpand: '查看任务流程',
  wbExpandGroup: '查看阶段流程',
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
    add('delivery', 'PR', remoteStatus, 808, center);
    add('merge', 'Merge', control.deliveryStale ? 'unknown' : pull?.state === 'MERGED' ? 'completed' : 'pending', 1004, center);
    edges.push(['aggregate', 'delivery'], ['delivery', 'merge']);
    width = 1192;
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

function nodeBindingFacts(node, state, quick) {
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
    .filter((s) => record(s) && s.role === role && (!scope || s.scope === scope || s.scope === 'final:' + scope))
    .sort((a, b) => (b.last?.['started-at'] || '').localeCompare(a.last?.['started-at'] || ''))[0];
  const stageID = role === 'execute' ? 'routes.' + node.packet.route : 'loops.' + role;
  // Legacy `bindings` recorded declarations, not resolved policy candidates.
  // Never fill a missing historical fact from the currently edited settings.
  const initial = state.run?.initialBindings?.[stageID] ?? null;
  const policy = role === 'execute' ? node.packet?.['execution-policy'] : null;
  let selected = initial;
  if (policy) {
    // New records freeze the exact selected configuration independently of a
    // later run's defaults. A display omission must not be filled from history.
    if (policy.currentBinding) selected = policy.currentBinding;
    else if (policy.currentBindingOmitted) selected = null;
    else {
      const last = policy.history?.at(-1);
      selected = last?.to === policy.current ? { bindingId: last.to, binding: last.after } : null;
      if (!selected) {
        for (const decision of [...(state['runtime-decisions']?.records ?? [])].reverse()) {
          const choice = decision['model-bindings']?.[decision.action];
          if (decision.packet === id && ['applied', 'baseline'].includes(decision.status) && choice?.bindingId === policy.current) {
            selected = choice;
            break;
          }
        }
      }
    }
  }
  return { role, initial, selected, selectedId: policy?.current, selectedOmitted: policy?.currentBindingOmitted === true, actual,
    declared: state.run?.bindings?.[stageID], mock: state.run?.drivers?.[role] === 'mock' };
}

function invocationDispatched(value) {
  return value?.last?.dispatch === 'dispatched' || value?.last?.dispatch === 'unknown' && value.last['dispatch-observed'] === true;
}

function recordedInvocationLabel(value) {
  return invocationDispatched(value) ? 'wbBindingActual' : value?.last?.dispatch === 'not-dispatched' && value.last['dispatch-observed'] !== true ? 'wbBindingNotSent' : 'wbBindingRecorded';
}

function bindingFactCaption(facts, t) {
  if (!facts) return null;
  if (facts.actual?.configuration) return t(recordedInvocationLabel(facts.actual)) + ' · ' + bindingCaption(facts.actual.configuration);
  if (facts.mock) return t('consoleMockModel');
  if (facts.selected) return t('wbBindingSelected') + ' · ' + bindingCaption(facts.selected.binding);
  if (facts.initial) return t('wbBindingInitial') + ' · ' + bindingCaption(facts.initial.binding);
  if (facts.declared) return t('wbBindingDeclared') + ' · ' + bindingCaption(facts.declared);
  return null;
}

function bindingCaption(binding) {
  const provider = binding.provider || binding.harness;
  const label =
    {
      'deepseek-account': 'DeepSeek',
      chatgpt: 'ChatGPT',
      kimi: 'Kimi Code',
      antigravity: 'Antigravity',
      'codex-pro': 'Codex Pro',
      'claude-code-pro': 'Claude Code Pro',
    }[provider] || provider;
  const model =
    { 'deepseek-flash': 'Flash', 'kimi-code/k3.1-pro': 'K3.1 Pro' }[binding.model] || binding.model;
  return [label, model].filter(Boolean).join(' · ');
}

Object.assign(zh, { wbInputImagesSent: '已派发调用的图片身份', wbInputImagesRecorded: '记录的图片输入配置 · 未确认派发', wbInputImagesClaude: '图片按这次调用的冻结身份传给 Claude SDK；此处不重新读取文件。', wbInputImageSize: '字节数', wbInputImageDigest: 'SHA-256' });
Object.assign(en, { wbInputImagesSent: 'Image identities for the confirmed dispatch', wbInputImagesRecorded: 'Recorded image input configuration · dispatch unconfirmed', wbInputImagesClaude: 'Images use this invocation’s frozen identity in Claude SDK. This view does not reread files.', wbInputImageSize: 'Bytes', wbInputImageDigest: 'SHA-256' });

function validInputImageManifest(value) {
  return value == null || record(value) && value.version === 1 && Array.isArray(value.images) && value.images.length > 0 && value.images.length <= 4 &&
    value.images.every(item => record(item) && ['image/png', 'image/jpeg', 'image/gif', 'image/webp'].includes(item.mimeType) &&
      typeof item.sha256 === 'string' && /^[0-9a-f]{64}$/.test(item.sha256) && Number.isSafeInteger(item.bytes) && item.bytes > 0);
}

function InvocationImages({ value, t }) {
  const manifest = value?.configuration?.inputImages;
  if (!manifest || !validInputImageManifest(manifest)) return null;
  const dispatched = invocationDispatched(value);
  return h('section', { className: css.inspectorSection, 'data-invocation-images': value.last?.id || '' },
    h('h5', null, t(dispatched ? 'wbInputImagesSent' : 'wbInputImagesRecorded')),
    dispatched && value.configuration.harness === 'claude-code-pro' && h('p', { className: css.meta }, t('wbInputImagesClaude')),
    ...manifest.images.map((item, index) => h('dl', { key: index, className: css.consoleProperties },
      h('dt', null, 'MIME'), h('dd', null, item.mimeType), h('dt', null, t('wbInputImageDigest')), h('dd', { className: css.path }, item.sha256),
      h('dt', null, t('wbInputImageSize')), h('dd', null, String(item.bytes)))));
}

function NodeBindingHistory({ facts, state, t }) {
  if (!facts) return null;
  const actual = facts.actual;
  return h('section', { className: css.inspectorSection + ' ' + css.observationFacts, 'data-node-bindings': true },
    h('h4', null, t('wbConfiguration')), h('p', { className: css.meta }, t('wbBindingHelp')),
    state.run?.configurationSource != null && h('p', { className: css.path }, t('wbConfigurationSource') + ': ' + (state.run.configurationSource || t('embedded'))),
    state.run?.initialBindingsOmitted > 0 && h('p', { className: css.meta, 'data-initial-bindings-omitted': true }, t('wbBindingsOmitted', { count: state.run.initialBindingsOmitted })),
    h('article', { className: css.observationBlock, 'data-binding-fact': 'initial' }, h('h5', null, t('wbBindingInitial')),
      facts.initial ? h(BindingProperties, { ...facts.initial, t }) : h('p', { className: css.meta }, t(state.run?.initialBindingsOmitted > 0 ? 'wbBindingUnshown' : 'wbBindingInitialUnknown'))),
    facts.role === 'execute' && h('article', { className: css.observationBlock, 'data-binding-fact': 'selected' }, h('h5', null, t('wbBindingSelected')),
      facts.selected ? h(BindingProperties, { ...facts.selected, t }) : h('p', { className: css.meta },
        (facts.selectedId ? t('runtimeBindingID') + ': ' + facts.selectedId + ' · ' : '') + t(facts.selectedOmitted || state.run?.initialBindingsOmitted > 0 ? 'wbBindingUnshown' : 'wbBindingSelectedUnknown'))),
    h('article', { className: css.observationBlock, 'data-binding-fact': 'actual' }, h('h5', null, t(actual ? recordedInvocationLabel(actual) : 'wbBindingActual')),
      actual ? h(React.Fragment, null, h(BindingProperties, { binding: actual.configuration, t }),
        h('dl', { className: css.consoleProperties },
          h('dt', null, t('wbBindingInvocation')), h('dd', { className: css.path }, textValue(actual.last?.id)),
          h('dt', null, t('wbBindingDispatch')), h('dd', null, t(invocationDispatched(actual) ? 'wbDispatchAccepted' : actual.last?.dispatch === 'not-dispatched' && actual.last['dispatch-observed'] !== true ? 'wbDispatchNotSent' : 'wbDispatchUnknown')),
          h('dt', null, t('wbBindingOutcome')), h('dd', null, t({ completed: 'wbOutcomeCompleted', failed: 'wbOutcomeFailed', cancelled: 'wbOutcomeCancelled' }[actual.last?.outcome] || 'wbOutcomeUnknown'))),
        h(InvocationImages, { value: actual, t })) : h('p', { className: css.meta }, t('wbBindingNoCall'))),
    !facts.initial && facts.declared && h(RawEvidence, { value: facts.declared, label: 'wbBindingDeclared', t }));
}

function validCollaborationView(value) {
  return value == null || record(value) && ['verified', 'unverified', 'absent'].includes(value.status) && typeof value.reason === 'string' &&
    (value.contractFingerprint == null || typeof value.contractFingerprint === 'string') && (value.status === 'verified'
      ? typeof value.contractFingerprint === 'string' && /^[0-9a-f]{64}$/.test(value.contractFingerprint) &&
        record(value.value) && value.value.version === 1 && Array.isArray(value.value.tasks) && value.value.tasks.length >= 2 && value.value.tasks.length <= 8 &&
        value.value.tasks.every(task => record(task) && typeof task.goal === 'string' && task.goal.trim() && Array.isArray(task.writes) && task.writes.length > 0 && task.writes.length <= 64 && task.writes.every(path => typeof path === 'string' && path.length > 0))
      : value.value == null);
}

Object.assign(zh, {
  wbEligibility: '当前执行资格', wbEligibilityHelp: '由引擎按当前 packet、执行深度和配置检查。资格检查不会启动模型调用；状态变化后需要重新检查。',
  wbEligibilityRead: '检查执行资格', wbEligibilityUnrequested: '尚未检查当前执行候选', wbEligibilityEligible: '满足当前输入与协作要求',
  wbEligibilityIneligible: '当前不可用', wbEligibilityUnknown: '无法确认当前执行资格', wbEligibilityStale: '检查结果已过期，请重新检查',
  wbEligibilityNoCandidates: '引擎未返回可检查的执行候选', wbEligibilityEvidence: '展开执行资格原始依据',
  wbEligibilityChecking: '正在检查当前执行资格',
  wbEligibilityFrozen: '检查依据：当前运行已冻结的配置', wbEligibilityConfiguration: '检查依据：当前生效配置',
  wbEligibilityImagePass: '当前通道满足这个步骤声明的图片输入要求', wbEligibilityImageFail: '这个步骤需要原图；所选通道尚未接通图片传输',
  wbEligibilityNativePass: '原生配置与执行后端匹配', wbEligibilityNativeFail: '原生配置无效或不匹配当前执行后端',
  wbEligibilityFullPass: '当前执行深度为 Full', wbEligibilityFullFail: 'Swarm 需要 Full 执行深度',
  wbEligibilitySplitPass: '已声明协作分工', wbEligibilitySplitFail: '尚未准备可并行的独立分工',
  wbEligibilityWritesPass: '独立分工通过写入范围检查', wbEligibilityWritesFail: '分工未通过写入范围检查',
  wbCollaborationEdit: '编辑分工草案', wbCollaborationEditor: '分工草案',
  wbCollaborationEditHelp: '填写 2–8 个独立子任务。每项写入范围必须位于父任务允许范围内，且不能与其他子任务重叠。',
  wbCollaborationReviewHelp: '保存会生成新的计划版本并重新进行独立审查，原版本的审查不适用于新草案。原设计批准只在目标与范围不变时保留。此操作不会启动实施。',
  wbCollaborationParentWrites: '父任务允许写入范围', wbCollaborationGoal: '子任务目标', wbCollaborationWriteLines: '写入范围（每行一项）',
  wbCollaborationTask: '子任务 {index}', wbCollaborationAdd: '添加子任务', wbCollaborationRemove: '移除此子任务',
  wbCollaborationDraftInvalid: '需要 2–8 个子任务；每项目标为 1–512 字，每项写入范围为 1–64 条。',
  wbCollaborationDraftTooLarge: '分工草案超过 32 KiB，请缩短目标或写入范围后再保存。',
  wbCollaborationSave: '保存草案并重新审查', wbCollaborationReload: '重新读取并替换草案', wbCollaborationClose: '关闭分工草案',
  wbCollaborationRebase: '刷新版本并保留草案', wbCollaborationRebaseFailed: '契约、计划或父范围已变化，或当前阶段不可编辑；原草案已保留，不能自动移到新版本。',
  wbCollaborationSnapshotStale: '草案基于旧版本，不能保存。当前内容已保留；重新读取会替换这些未保存的内容。',
  wbCollaborationLoadFailed: '未能读取可编辑的分工快照，当前草案没有改变。', wbCollaborationSaveFailed: '没有收到可确认的保存结果。草案已保留，请刷新任务状态并核对结果后再操作。',
  wbCollaborationSaved: '新的分工计划已保存并请求独立审查；只有新计划通过审查后才能执行。', wbCollaborationNotEditable: '当前阶段不可编辑分工',
  wbCollaborationRejected: '分工草案未被采用', wbCollaborationReconcile: '刷新任务并核对结果',
  wbCollaborationQueued: '分工草案请求已排队，尚未确认采用；请核对这一请求的回执。',
  wbCollaborationSnapshot: '展开分工草案原始快照', wbCollaborationPending: '正在读取或保存分工草案',
});
Object.assign(en, {
  wbEligibility: 'Current execution eligibility', wbEligibilityHelp: 'The engine checks this packet, execution depth and configuration. This check does not start a model invocation; check again after the state changes.',
  wbEligibilityRead: 'Check execution eligibility', wbEligibilityUnrequested: 'Current execution candidates have not been checked', wbEligibilityEligible: 'Meets current input and collaboration requirements',
  wbEligibilityIneligible: 'Currently unavailable', wbEligibilityUnknown: 'Current execution eligibility could not be confirmed', wbEligibilityStale: 'This check is out of date; check again',
  wbEligibilityNoCandidates: 'The engine returned no execution candidates to check', wbEligibilityEvidence: 'Expand original execution eligibility evidence',
  wbEligibilityChecking: 'Checking current execution eligibility',
  wbEligibilityFrozen: 'Checked against the frozen configuration for this run', wbEligibilityConfiguration: 'Checked against the current effective configuration',
  wbEligibilityImagePass: 'The channel meets this step’s declared image input requirements', wbEligibilityImageFail: 'This step requires original images; the selected channel has no connected image transport',
  wbEligibilityNativePass: 'The native profile matches the execution backend', wbEligibilityNativeFail: 'The native profile is invalid or does not match the execution backend',
  wbEligibilityFullPass: 'The execution depth is Full', wbEligibilityFullFail: 'Swarm requires Full execution depth',
  wbEligibilitySplitPass: 'A collaboration task split is declared', wbEligibilitySplitFail: 'An independent parallel task split has not been prepared',
  wbEligibilityWritesPass: 'Independent task write scopes passed validation', wbEligibilityWritesFail: 'The collaboration write scopes failed validation',
  wbCollaborationEdit: 'Edit collaboration draft', wbCollaborationEditor: 'Collaboration draft',
  wbCollaborationEditHelp: 'Define 2–8 independent tasks. Each write scope must stay within the parent task scope and must not overlap another task.',
  wbCollaborationReviewHelp: 'Saving creates a new plan revision for independent review; the old plan review does not cover this draft. Design approval is retained only while its goal and scope remain unchanged. This action does not start implementation.',
  wbCollaborationParentWrites: 'Parent task write scope', wbCollaborationGoal: 'Task goal', wbCollaborationWriteLines: 'Write scopes (one per line)',
  wbCollaborationTask: 'Task {index}', wbCollaborationAdd: 'Add task', wbCollaborationRemove: 'Remove this task',
  wbCollaborationDraftInvalid: 'Define 2–8 tasks, each with a 1–512 character goal and 1–64 write scopes.',
  wbCollaborationDraftTooLarge: 'The collaboration draft exceeds 32 KiB. Shorten the goals or write scopes before saving.',
  wbCollaborationSave: 'Save draft for review', wbCollaborationReload: 'Reread and replace draft', wbCollaborationClose: 'Close collaboration draft',
  wbCollaborationRebase: 'Refresh revision and keep draft', wbCollaborationRebaseFailed: 'The contract, plan or parent scope changed, or this stage is not editable. Your original draft is retained and cannot be moved to the new revision automatically.',
  wbCollaborationSnapshotStale: 'This draft is based on an older revision and cannot be saved. Your changes are retained; rereading will replace these unsaved changes.',
  wbCollaborationLoadFailed: 'An editable collaboration snapshot could not be read. Your draft is unchanged.', wbCollaborationSaveFailed: 'A confirmed save result was not received. Your draft is retained; refresh the task and reconcile the outcome before continuing.',
  wbCollaborationSaved: 'The new collaboration plan was saved for independent review. It must pass review before execution.', wbCollaborationNotEditable: 'Collaboration cannot be edited at this stage',
  wbCollaborationRejected: 'The collaboration draft was not adopted', wbCollaborationReconcile: 'Refresh task and reconcile outcome',
  wbCollaborationQueued: 'The collaboration request is queued and adoption is unconfirmed. Reconcile the receipt for this request.',
  wbCollaborationSnapshot: 'Expand original collaboration draft snapshot', wbCollaborationPending: 'Reading or saving the collaboration draft',
});

function collaborationDraftValue(rows) {
  return { version: 1, tasks: rows.map(row => ({ goal: row.goal, writes: row.writes.split(/\r?\n/).map(path => path.trim()).filter(Boolean) })) };
}

function collaborationDraftValid(rows) {
  const value = collaborationDraftValue(rows);
  return value.tasks.length >= 2 && value.tasks.length <= 8 && value.tasks.every(task => task.goal.trim() && !task.goal.includes('\0') && Array.from(task.goal).length <= 512 &&
    task.writes.length >= 1 && task.writes.length <= 64 && task.writes.every(path => path.length <= 8192 && !path.includes('\0')));
}

function collaborationEditBytes(value) {
  let bytes = 0;
  for (const character of JSON.stringify(value)) {
    const point = character.codePointAt(0);
    bytes += point < 0x80 ? 1 : point < 0x800 ? 2 : point < 0x10000 ? 3 : 4;
    if (bytes > (32 << 10)) break;
  }
  return bytes;
}

function CollaborationTaskFields({ rows, disabled, change, add, remove, t }) {
  return h(React.Fragment, null,
    ...rows.map((row, index) => h('fieldset', { key: row.key, className: css.editorBinding, disabled, 'data-collaboration-task': index },
      h('legend', null, t('wbCollaborationTask', { index: index + 1 })),
      h('label', { className: css.editorField }, t('wbCollaborationGoal'),
        h('textarea', { className: css.planningInput, value: row.goal, 'aria-label': t('wbCollaborationTask', { index: index + 1 }) + ' · ' + t('wbCollaborationGoal'),
          onInput: event => change(index, 'goal', event.target.value) })),
      h('label', { className: css.editorField }, t('wbCollaborationWriteLines'),
        h('textarea', { className: css.planningInput, value: row.writes, 'aria-label': t('wbCollaborationTask', { index: index + 1 }) + ' · ' + t('wbCollaborationWriteLines'),
          onInput: event => change(index, 'writes', event.target.value) })),
      h(Button, { disabled: disabled || rows.length <= 2, onClick: () => remove(index) }, t('wbCollaborationRemove')))),
    h(Button, { disabled: disabled || rows.length >= 8, onClick: add }, t('wbCollaborationAdd')));
}

function validCollaborationEditor(value) {
  const fingerprint = digest => typeof digest === 'string' && /^[0-9a-f]{64}$/.test(digest);
  return value == null || record(value) && typeof value.packet === 'string' && /^M\d+-T\d+$/.test(value.packet) &&
    Number.isSafeInteger(value.revision) && value.revision > 0 && typeof value.editable === 'boolean' && typeof value.reason === 'string' &&
    Array.isArray(value.writes) && value.writes.every(path => typeof path === 'string' && path.length > 0) &&
    (value.editable ? fingerprint(value.contractFingerprint) && planningRef(value.plan) :
      (value.contractFingerprint == null || value.contractFingerprint === '' || fingerprint(value.contractFingerprint)) && (value.plan == null || planningRef(value.plan))) &&
    (value.value == null || record(value.value) && value.value.version === 1 && Array.isArray(value.value.tasks) &&
      value.value.tasks.length >= 2 && value.value.tasks.length <= 8 && value.value.tasks.every(task => record(task) && typeof task.goal === 'string' &&
        task.goal.trim() && Array.from(task.goal).length <= 512 && Array.isArray(task.writes) && task.writes.length > 0 && task.writes.length <= 64 &&
        task.writes.every(path => typeof path === 'string' && path.length > 0)));
}

function validNodeEligibility(value) {
  return value == null || record(value) && typeof value.packet === 'string' && /^M\d+-T\d+$/.test(value.packet) &&
    Number.isSafeInteger(value.revision) && value.revision > 0 && ['verified', 'unknown'].includes(value.status) && typeof value.reason === 'string' &&
    ['frozen', 'configuration'].includes(value.source) && (value.route == null || typeof value.route === 'string') &&
    Array.isArray(value.candidates) && value.candidates.every(candidate => record(candidate) && typeof candidate.id === 'string' &&
      record(candidate.binding) && typeof candidate.binding.harness === 'string' && candidate.binding.harness.length > 0 &&
      (candidate.binding.model == null || typeof candidate.binding.model === 'string') &&
      ['provider', 'channel', 'thinking', 'native-profile', 'permission', 'session-mode'].every(key => candidate.binding[key] == null || typeof candidate.binding[key] === 'string') &&
      typeof candidate.eligible === 'boolean' && Array.isArray(candidate.conditions) && candidate.conditions.every(condition => record(condition) &&
        typeof condition.code === 'string' && typeof condition.passed === 'boolean' && typeof condition.message === 'string'));
}

function eligibilityConditionLabel(condition, t) {
  const labels = { 'image-input': 'wbEligibilityImage', 'native-profile': 'wbEligibilityNative', 'full-workflow': 'wbEligibilityFull',
    'collaboration-declared': 'wbEligibilitySplit', 'independent-writes': 'wbEligibilityWrites' };
  if (!Object.hasOwn(labels, condition.code)) return condition.message;
  return t(labels[condition.code] + (condition.passed ? 'Pass' : 'Fail')) +
    (condition.code === 'independent-writes' && !condition.passed && condition.message ? ': ' + condition.message : '');
}

function NodeExecutionEligibility({ packetID, state, control, selection, run, t }) {
  const [failed, setFailed] = React.useState(false);
  if (!packetID) return null;
  const view = control.node?.id === packetID ? control.node.eligibility : null;
  const valid = view && validNodeEligibility(view) && view.packet === packetID;
  const stale = valid && (control.stale || control.node.revision !== state.revision || view.revision !== state.revision);
  const checking = control.pending?.action === 'node', readFailed = failed || control.error?.action === 'node';
  const verified = valid && !stale && !readFailed && !checking && view.status === 'verified';
  const inspect = async () => {
    setFailed(false);
    const result = await run('node', selection, false, { packet: packetID });
    if (!result?.node?.eligibility) setFailed(true);
  };
  return h('section', { className: css.inspectorSection, 'data-node-eligibility': packetID },
    h('h4', null, t('wbEligibility')), h('p', { className: css.meta }, t('wbEligibilityHelp')),
    h(Button, { disabled: !!control.pending, onClick: inspect }, t('wbEligibilityRead')),
    !verified && h('p', { className: css.meta, 'data-eligibility-status': checking ? 'pending' : stale ? 'stale' : readFailed || view ? 'unknown' : 'unrequested' },
      t(checking ? 'wbEligibilityChecking' : stale ? 'wbEligibilityStale' : readFailed || view ? 'wbEligibilityUnknown' : 'wbEligibilityUnrequested')),
    valid && !stale && !readFailed && !checking && h('p', { className: css.meta }, t(view.source === 'frozen' ? 'wbEligibilityFrozen' : 'wbEligibilityConfiguration')),
    valid && !stale && !readFailed && !checking && view.reason && h('p', { className: css.meta }, view.reason),
    verified && (view.candidates.length ? view.candidates.map((candidate, index) => {
      const binding = candidate.binding;
      const property = (label, content) => h(React.Fragment, { key: label }, h('dt', null, label), h('dd', null, content));
      return h('article', { key: candidate.id + ':' + index, className: css.observationBlock, 'data-eligibility-candidate': candidate.id, 'data-eligible': String(candidate.eligible) },
        h('h5', null, bindingCaption(binding) || candidate.id),
        h('p', { className: candidate.eligible ? css.meta : css.warningText }, t(candidate.eligible ? 'wbEligibilityEligible' : 'wbEligibilityIneligible')),
        h('dl', { className: css.consoleProperties },
          property(t('runtimeBindingID'), candidate.id), property(t('consoleProvider'), bindingCaption({ ...binding, model: '' })),
          property(t('consoleModel'), binding.model || t('wbBindingModelUnspecified')), property(t('consoleThinking'), bindingEffortLabel(binding, t)),
          binding.channel && property(t('channel'), binding.channel), binding['native-profile'] && property(t('cfgNative'), binding['native-profile'])),
        h('ul', null, ...candidate.conditions.map((condition, conditionIndex) => h('li', { key: conditionIndex, 'data-eligibility-condition': condition.code, 'data-passed': String(condition.passed), className: condition.passed ? css.meta : css.warningText }, eligibilityConditionLabel(condition, t)))));
    }) : h('p', { className: css.meta }, t('wbEligibilityNoCandidates'))),
    valid && h(RawEvidence, { value: view, label: 'wbEligibilityEvidence', t }));
}

function NodeCollaborationEditor({ packetID, state, control, selection, run, packetDrafts, t }) {
  const cacheKey = JSON.stringify([selection.cwd ?? '', selection.taskDir ?? '', packetID]);
  const draft = React.useMemo(() => {
    let value = packetDrafts?.get(cacheKey);
    if (!value) {
      value = createSnapshot({ open: false, snapshot: null, rows: [], notice: null, pending: false, sequence: 0 });
      packetDrafts?.set(cacheKey, value);
    }
    return value;
  }, [packetDrafts, cacheKey]);
  // The Session binding owns this observable, so closing a node or remounting a
  // view retains both edits and the exact receipt needed after a lost reply.
  const { open, snapshot, rows, notice, pending: localPending } = React.useSyncExternalStore(draft.subscribe, draft.getSnapshot, draft.getSnapshot);
  React.useEffect(() => {
    const error = control.error;
    if (notice?.kind !== 'unknown' || error?.action !== 'planning' || error.dispatch !== 'not-dispatched' ||
        error.planningAction !== 'collaboration' || error.planningRequestID !== notice.requestID) return;
    draft.update(next => {
      if (next.notice?.kind === 'unknown' && next.notice.requestID === error.planningRequestID)
        next.notice = { kind: 'rejected', message: error.detail };
    });
  }, [control.error, notice, draft]);
  const busy = !!control.pending || localPending;
  const latest = control.node?.id === packetID ? control.node.collaborationEditor : null;
  const stale = !!snapshot && (snapshot.revision !== state.revision || control.stale || !latest ||
    latest.contractFingerprint !== snapshot.contractFingerprint || !(latest.plan == null && snapshot.plan == null || samePlanningRef(latest.plan, snapshot.plan)));
  const unresolved = ['pending', 'unknown', 'queued'].includes(notice?.kind);
  const blocked = busy || stale || unresolved || ['read-failed', 'rebase-failed'].includes(notice?.kind) || control.uncertainty || control.status?.running || state.handoff || !snapshot?.editable || !latest?.editable;
  const value = collaborationDraftValue(rows);
  const edit = snapshot && { packet: packetID, contractFingerprint: snapshot.contractFingerprint, plan: snapshot.plan, value };
  const dirty = !!snapshot && JSON.stringify(value) !== JSON.stringify(snapshot.value ?? null);
  const fieldsValid = collaborationDraftValid(rows), bounded = !!edit && collaborationEditBytes(edit) <= (32 << 10);
  const validDraft = fieldsValid && bounded;
  const read = async (preserve = false) => {
    if (busy || unresolved) return;
    draft.update(next => { next.pending = true; next.notice = null; next.open = true; });
    try {
      const result = await run('node', selection, false, { packet: packetID });
      const next = result?.node?.collaborationEditor;
      if (!next || !validCollaborationEditor(next) || result.node.id !== packetID || next.packet !== packetID ||
          result.node.revision !== next.revision || result.state?.revision !== next.revision) {
        draft.update(value => { value.notice = { kind: 'read-failed' }; }); return;
      }
      if (preserve && (!snapshot || !next.editable || next.contractFingerprint !== snapshot.contractFingerprint || !samePlanningRef(next.plan, snapshot.plan) ||
          JSON.stringify(next.writes) !== JSON.stringify(snapshot.writes) || JSON.stringify(next.value ?? null) !== JSON.stringify(snapshot.value ?? null))) {
        draft.update(value => { value.notice = { kind: 'rebase-failed' }; }); return;
      }
      draft.update(value => {
        value.snapshot = next;
        if (!preserve) value.rows = (next.value?.tasks ?? [{ goal: '', writes: [] }, { goal: '', writes: [] }]).map(task =>
          ({ key: ++value.sequence, goal: task.goal, writes: task.writes.join('\n') }));
      });
    } finally { draft.update(value => { value.pending = false; }); }
  };
  const acceptReceipt = (result, requestID) => {
    const receipt = result?.['planning-control'];
    const failure = result?.controlFailure;
    draft.update(next => {
      if (next.notice?.requestID !== requestID) return;
      if (result?.ok === undefined && failure?.dispatch === 'not-dispatched' && failure.action === 'planning' &&
          failure.planningAction === 'collaboration' && failure.planningRequestID === requestID &&
          failure.key === selectionKey(selectionOf(selection))) {
        next.notice = { kind: 'rejected', message: failure.detail }; return;
      }
      if (receipt?.action !== 'collaboration' || receipt['request-id'] !== requestID || !['applied', 'unchanged', 'rejected', 'queued'].includes(receipt.status)) {
        next.notice = { kind: 'unknown', requestID }; return;
      }
      if (receipt.status === 'queued') { next.notice = { kind: 'queued', requestID }; return; }
      if (receipt.status === 'rejected') { next.notice = { kind: 'rejected', message: receipt.message }; return; }
      next.notice = { kind: 'saved', message: receipt.message }; next.open = false; next.snapshot = null; next.rows = [];
    });
  };
  React.useEffect(() => {
    const receipt = control.planningReceipt;
    if (!localPending && ['unknown', 'queued'].includes(notice?.kind) && receipt?.action === 'collaboration' && receipt['request-id'] === notice.requestID &&
        ['applied', 'unchanged', 'rejected'].includes(receipt.status)) acceptReceipt({ 'planning-control': receipt }, notice.requestID);
  }, [control.planningReceipt, localPending, notice, draft]);
  const save = async () => {
    if (blocked || !dirty || !validDraft) return;
    const requestID = planningRequestID();
    // Persist this identity before dispatch. The same cached draft can settle
    // after its component unmounts without changing another packet's editor.
    draft.update(next => { next.pending = true; next.notice = { kind: 'pending', requestID }; });
    try {
      const result = await run('planning', selection, false, { planning: {
        action: 'collaboration', revision: snapshot.revision, 'request-id': requestID, start: true,
        collaboration: edit,
      } });
      acceptReceipt(result, requestID);
    } finally { draft.update(next => { next.pending = false; }); }
  };
  const reconcile = async () => {
    if (busy || !unresolved) return;
    draft.update(next => { next.pending = true; });
    try {
      const result = await run('planning', selection, false, { planning: { action: 'status', 'request-id': notice.requestID } });
      acceptReceipt(result, notice.requestID);
    } finally { draft.update(next => { next.pending = false; }); }
  };
  const change = (index, field, value) => draft.update(next => { next.rows = next.rows.map((row, n) => n === index ? { ...row, [field]: value } : row); });
  return h('section', { className: css.inspectorSection, 'data-collaboration-editor': packetID },
    !open && h(Button, { disabled: busy, onClick: () => read() }, t('wbCollaborationEdit')),
    open && h(React.Fragment, null,
      h('h4', null, t('wbCollaborationEditor')), h('p', { className: css.meta }, t('wbCollaborationEditHelp')),
      h('p', { className: css.meta }, t('wbCollaborationReviewHelp')),
      snapshot && h(React.Fragment, null,
        h('p', { className: css.path }, t('wbCollaborationParentWrites') + ': ' + (snapshot.writes.join(', ') || '—')),
        h(PlanningReference, { value: snapshot.plan, t }),
        !snapshot.editable && h('p', { className: css.warningText, 'data-collaboration-uneditable': true }, t('wbCollaborationNotEditable') + (snapshot.reason ? ': ' + snapshot.reason : '')),
        snapshot.editable && h(CollaborationTaskFields, { rows, disabled: !!blocked, change,
          add: () => draft.update(next => { next.rows = [...next.rows, { key: ++next.sequence, goal: '', writes: '' }]; }),
          remove: index => draft.update(next => { next.rows = next.rows.filter((_, n) => n !== index); }), t }),
        snapshot.editable && !fieldsValid && h('p', { className: css.meta }, t('wbCollaborationDraftInvalid')),
        snapshot.editable && !bounded && h('p', { className: css.meta, 'data-collaboration-too-large': true }, t('wbCollaborationDraftTooLarge')),
        stale && h('p', { role: 'alert', className: css.warningText, 'data-collaboration-stale': true }, t('wbCollaborationSnapshotStale')),
        !stale && (control.uncertainty || control.status?.running || state.handoff) && h('p', { className: css.warningText },
          t(control.uncertainty ? 'wbUnknown' : control.status?.running ? 'wbRunning' : 'wbHeld')),
        h(Button, { variant: 'primary', disabled: !!blocked || !dirty || !validDraft, onClick: save }, t('wbCollaborationSave')),
        h(RawEvidence, { value: snapshot, label: 'wbCollaborationSnapshot', t })),
      h('div', { className: css.actions },
        snapshot && (stale || ['rejected', 'read-failed', 'rebase-failed'].includes(notice?.kind)) && h(Button, { disabled: busy || unresolved, onClick: () => read(true) }, t('wbCollaborationRebase')),
        h(Button, { disabled: busy || unresolved, onClick: () => read() }, t('wbCollaborationReload')),
        h(Button, { disabled: busy || unresolved, onClick: () => draft.update(next => { next.open = false; next.snapshot = null; next.rows = []; next.notice = null; }) }, t('wbCollaborationClose')))),
    localPending && h('p', { role: 'status', className: css.meta }, t('wbCollaborationPending')),
    notice && notice.kind !== 'pending' && h('p', { role: notice.kind === 'saved' ? 'status' : 'alert', 'data-collaboration-result': notice.kind, className: notice.kind === 'saved' ? css.meta : css.warningText },
      t({ saved: 'wbCollaborationSaved', rejected: 'wbCollaborationRejected', queued: 'wbCollaborationQueued', unknown: 'wbCollaborationSaveFailed', 'read-failed': 'wbCollaborationLoadFailed', 'rebase-failed': 'wbCollaborationRebaseFailed' }[notice.kind]) + (notice.message ? ' ' + notice.message : '')),
    unresolved && h(Button, { disabled: busy, onClick: reconcile }, t('wbCollaborationReconcile')));
}

Object.assign(zh, {
  packetImages: '图片输入', packetImagesHelp: '这里显示已读取契约中固定的路径、格式与摘要；读取声明不会读取原图，也不表示模型已经收到图片。',
  packetImagesCurrent: '当前计划的图片声明', packetImagesRead: '已读取的图片声明', packetImagesNone: '此契约没有声明图片输入', packetImagesUnknown: '图片声明尚未确认，请读取准确快照。',
  packetImagesEdit: '编辑图片输入', packetImagesDraft: '图片输入草案', packetImagesPath: '仓库图片路径 {index}',
  packetImagesAdd: '添加图片路径', packetImagesRemove: '移除图片 {index}', packetImagesPreview: '检查并预览图片草案',
  packetImagesPreviewHelp: '只读取你明确填写的仓库相对路径。最多 4 张 PNG/JPEG/GIF/WebP，每张 4 MiB、合计 8 MiB；不自动导入聊天附件。',
  packetImagesNeedPreview: '路径和摘要可以保留；预览不会缓存。修改路径或重新打开页面后，请重新检查预览再保存。',
  packetImagesPreviewFailed: '图片检查未确认；草案已保留，不能据此冻结图片。',
  packetImagesReview: '保存会校验原始字节、生成新的不可变计划并请求独立审查。旧计划的审查不覆盖此草案；不会修改活动契约或授予实施授权。原图执行仍需支持图片的 Claude SDK 通道。',
  packetImagesClear: '明确清空全部图片声明', packetImagesClearConfirm: '我确认此步骤不再需要这些原图；清空也必须经过独立计划审查，不能用于绕过图片输入要求。',
  packetImagesKeep: '返回图片路径编辑', packetImagesSave: '保存图片草案并重新审查', packetImagesReload: '重新读取并替换图片草案',
  packetImagesRebase: '刷新版本并保留图片草案', packetImagesRebaseFailed: '契约、准确计划或原图片声明已变化，或当前阶段不可编辑。草案已保留，不能自动移到新版本。',
  packetImagesStale: '图片草案基于旧快照，当前不能保存。', packetImagesReadFailed: '未能读取准确的图片编辑快照；原草案已保留。',
  packetImagesClose: '放弃并关闭图片草案', packetImagesUneditable: '当前阶段不可编辑图片输入', packetImagesPending: '正在读取或保存图片输入',
  packetImagesSaved: '图片修订已保存并请求独立审查；实施仍受原有批准与启动条件约束。', packetImagesRejected: '图片修订未被采用',
  packetImagesQueued: '图片修订已排队，尚未确认采用，请核对原请求。', packetImagesUnconfirmed: '图片修订结果未知，草案及请求编号已保留；请核对原请求，勿重复保存。',
  packetImagesReconcile: '核对图片修订结果', packetImagesSnapshot: '展开图片声明原始快照', packetImagesPreviewAlt: '图片草案缩略图：{path}',
});
Object.assign(en, {
  packetImages: 'Image inputs', packetImagesHelp: 'These are fixed paths, formats and digests from the contract that was read. Reading references does not read originals or establish that a model received them.',
  packetImagesCurrent: 'Image references in the current plan', packetImagesRead: 'Image references read from the contract', packetImagesNone: 'This contract declares no image inputs', packetImagesUnknown: 'Image references are unconfirmed. Read the exact snapshot.',
  packetImagesEdit: 'Edit image inputs', packetImagesDraft: 'Image input draft', packetImagesPath: 'Repository image path {index}',
  packetImagesAdd: 'Add image path', packetImagesRemove: 'Remove image {index}', packetImagesPreview: 'Inspect and preview image draft',
  packetImagesPreviewHelp: 'Read only the repository-relative paths you explicitly enter. Up to 4 PNG/JPEG/GIF/WebP images, 4 MiB each and 8 MiB combined. Chat attachments are not imported automatically.',
  packetImagesNeedPreview: 'Paths and digests can be retained; previews are not cached. Inspect again after changing paths or reopening the view before saving.',
  packetImagesPreviewFailed: 'Image inspection is unconfirmed. Your draft is retained and cannot freeze inputs from this result.',
  packetImagesReview: 'Saving verifies original bytes, creates a new immutable plan and requests independent review. The old plan review does not cover this draft. It does not change active contracts or grant implementation authority. Original image execution still requires the supported Claude SDK transport.',
  packetImagesClear: 'Explicitly clear all image references', packetImagesClearConfirm: 'I confirm this step no longer requires these originals. Clearing still requires independent plan review and must not bypass image input requirements.',
  packetImagesKeep: 'Return to image paths', packetImagesSave: 'Save image draft for review', packetImagesReload: 'Reread and replace image draft',
  packetImagesRebase: 'Refresh revision and keep image draft', packetImagesRebaseFailed: 'The contract, exact plan or original image references changed, or editing is unavailable. Your draft is retained and cannot move automatically to the new revision.',
  packetImagesStale: 'This image draft uses an older snapshot and cannot be saved.', packetImagesReadFailed: 'The exact image editor snapshot could not be read. Your original draft is retained.',
  packetImagesClose: 'Discard and close image draft', packetImagesUneditable: 'Image inputs cannot be edited at this stage', packetImagesPending: 'Reading or saving image inputs',
  packetImagesSaved: 'The image revision was saved for independent review. Existing approval and start requirements still govern implementation.', packetImagesRejected: 'The image revision was not adopted',
  packetImagesQueued: 'The image revision is queued and adoption is unconfirmed. Reconcile the original request.', packetImagesUnconfirmed: 'The image revision outcome is unknown. Your draft and request ID are retained; reconcile the original request before another save.',
  packetImagesReconcile: 'Reconcile image revision', packetImagesSnapshot: 'Expand original image reference snapshot', packetImagesPreviewAlt: 'Image draft thumbnail: {path}',
});

function PacketImageReferences({ value, t }) {
  return value ? h('ol', { 'data-packet-image-references': true }, ...value.images.map((ref, index) =>
    h('li', { key: index }, h('p', { className: css.path }, ref.path), h('p', { className: css.meta }, ref['media-type']), h('code', { className: css.path }, 'SHA-256 ' + ref.sha256))))
    : h('p', { className: css.meta }, t('packetImagesNone'));
}

function NodeImageInputEditor({ packetID, state, control, selection, run, packetDrafts, t }) {
  const cacheKey = JSON.stringify([selection.cwd ?? '', selection.taskDir ?? '', packetID, 'input-images']);
  const draft = React.useMemo(() => {
    let value = packetDrafts?.get(cacheKey);
    if (!value) {
      value = createSnapshot({ open: false, snapshot: null, rows: [], references: null, mode: 'replace', clearConfirmed: false, notice: null, pending: false, sequence: 0 });
      packetDrafts?.set(cacheKey, value);
    }
    return value;
  }, [packetDrafts, cacheKey]);
  const { open, snapshot, rows, references, mode, clearConfirmed, notice, pending: localPending } = React.useSyncExternalStore(draft.subscribe, draft.getSnapshot, draft.getSnapshot);
  const [preview, setPreview] = React.useState(null);
  const generation = React.useRef(0), live = React.useRef(true);
  React.useEffect(() => {
    live.current = true; setPreview(null);
    return () => { live.current = false; generation.current++; };
  }, [draft]);
  React.useEffect(() => { if (!open) setPreview(null); }, [open]);
  const latest = control.node?.id === packetID ? control.node.imageInputEditor : null;
  const current = latest && validImageInputEditor(latest) && latest.revision === state.revision && !control.stale ? latest : null;
  const stale = !!snapshot && (snapshot.revision !== state.revision || control.stale || !current ||
    current.contractFingerprint !== snapshot.contractFingerprint || !(current.plan == null && snapshot.plan == null || samePlanningRef(current.plan, snapshot.plan)));
  const busy = !!control.pending || !!localPending, unresolved = ['pending', 'unknown', 'queued'].includes(notice?.kind);
  const blocked = busy || stale || unresolved || ['read-failed', 'rebase-failed'].includes(notice?.kind) || control.uncertainty || control.status?.running || state.handoff || !snapshot?.editable || !current?.editable;
  const paths = rows.map(row => row.path);
  const previewCurrent = mode === 'replace' && preview?.owner === draft && validImageInspection(preview.value, paths) && JSON.stringify(references) === JSON.stringify(preview.value.references);
  const value = mode === 'clear' ? null : references;
  const edit = snapshot && { packet: packetID, contractFingerprint: snapshot.contractFingerprint, plan: snapshot.plan, value };
  const validDraft = mode === 'clear' ? clearConfirmed && snapshot?.value !== null : previewCurrent;
  const dirty = !!snapshot && (mode === 'clear' || references !== null) && JSON.stringify(value) !== JSON.stringify(snapshot.value);
  const invalidatePreview = () => { generation.current++; setPreview(null); };
  const read = async (preserve = false) => {
    if (busy || unresolved || draft.getSnapshot().pending) return;
    invalidatePreview();
    draft.update(next => { next.pending = 'read'; next.notice = null; next.open = true; });
    try {
      const result = await run('node', selection, false, { packet: packetID }), next = result?.node?.imageInputEditor;
      if (!next || !validImageInputEditor(next) || result.node.id !== packetID || next.packet !== packetID || result.node.revision !== next.revision || result.state?.revision !== next.revision) {
        draft.update(value => { value.notice = { kind: 'read-failed' }; }); return;
      }
      if (preserve && (!snapshot || !next.editable || next.contractFingerprint !== snapshot.contractFingerprint || !samePlanningRef(next.plan, snapshot.plan) || JSON.stringify(next.value) !== JSON.stringify(snapshot.value))) {
        draft.update(value => { value.notice = { kind: 'rebase-failed' }; }); return;
      }
      draft.update(value => {
        value.snapshot = next;
        if (!preserve) {
          value.rows = (next.value?.images.map(image => image.path) ?? ['']).map(path => ({ key: ++value.sequence, path }));
          value.references = null; value.mode = 'replace'; value.clearConfirmed = false;
        }
      });
    } finally { draft.update(next => { next.pending = false; }); }
  };
  const inspect = async () => {
    if (blocked || mode !== 'replace' || !validImageSelection({ paths }) || draft.getSnapshot().pending) return;
    invalidatePreview(); const token = generation.current;
    draft.update(next => { next.pending = 'preview'; next.references = null; next.notice = null; });
    try {
      const result = await run('images', selection, false, { images: { paths } });
      if (!live.current || token !== generation.current) return;
      if (!result || !validImageInspection(result.inputImages, paths)) { draft.update(next => { next.notice = { kind: 'preview-failed' }; }); return; }
      // Only references enter the retained store. Thumbnail data stays local
      // and a remounted view must inspect again before saving a replacement.
      draft.update(next => { next.references = result.inputImages.references; });
      setPreview({ owner: draft, value: result.inputImages });
    } finally { draft.update(next => { next.pending = false; }); }
  };
  const acceptReceipt = (result, requestID) => {
    const receipt = result?.['planning-control'], failure = result?.controlFailure;
    draft.update(next => {
      if (next.notice?.requestID !== requestID) return;
      if (result?.ok === undefined && failure?.dispatch === 'not-dispatched' && failure.action === 'planning' && failure.planningAction === 'input-images' &&
          failure.planningRequestID === requestID && failure.key === selectionKey(selectionOf(selection))) {
        next.notice = { kind: 'rejected', message: failure.detail }; return;
      }
      if (receipt?.action !== 'input-images' || receipt['request-id'] !== requestID || !['applied', 'unchanged', 'rejected', 'queued'].includes(receipt.status)) {
        next.notice = { kind: 'unknown', requestID }; return;
      }
      if (receipt.status === 'queued') { next.notice = { kind: 'queued', requestID }; return; }
      if (receipt.status === 'rejected') { next.notice = { kind: 'rejected', message: receipt.message }; return; }
      next.notice = { kind: 'saved', message: receipt.message }; next.open = false; next.snapshot = null; next.rows = []; next.references = null; next.clearConfirmed = false;
    });
  };
  React.useEffect(() => {
    const receipt = control.planningReceipt;
    if (!localPending && ['unknown', 'queued'].includes(notice?.kind) && receipt?.action === 'input-images' && receipt['request-id'] === notice.requestID &&
        ['applied', 'unchanged', 'rejected'].includes(receipt.status)) acceptReceipt({ 'planning-control': receipt }, notice.requestID);
  }, [control.planningReceipt, localPending, notice, draft]);
  const save = async () => {
    if (blocked || !validDraft || !dirty || !edit || collaborationEditBytes(edit) > (32 << 10) || draft.getSnapshot().pending) return;
    const requestID = planningRequestID();
    draft.update(next => { next.pending = 'save'; next.notice = { kind: 'pending', requestID }; });
    try {
      const result = await run('planning', selection, false, { planning: { action: 'input-images', revision: snapshot.revision, 'request-id': requestID, start: true, 'input-images': edit } });
      acceptReceipt(result, requestID);
    } finally { draft.update(next => { next.pending = false; }); }
  };
  const reconcile = async () => {
    if (busy || !unresolved || draft.getSnapshot().pending) return;
    draft.update(next => { next.pending = 'reconcile'; });
    try { acceptReceipt(await run('planning', selection, false, { planning: { action: 'status', 'request-id': notice.requestID } }), notice.requestID); }
    finally { draft.update(next => { next.pending = false; }); }
  };
  const changePaths = change => {
    if (blocked) return;
    invalidatePreview(); draft.update(next => { change(next); next.references = null; next.notice = null; });
  };
  return h('section', { className: css.inspectorSection, 'data-packet-image-editor': packetID },
    h('h4', null, t('packetImages')), h('p', { className: css.meta }, t('packetImagesHelp')),
    current ? h('section', { 'data-packet-image-current': true, 'data-image-reference-status': current.status },
      current.status === 'known' ? h(React.Fragment, null, h('h5', null, t(current.plan ? 'packetImagesCurrent' : 'packetImagesRead')),
        h(PlanningReference, { value: current.plan, t }), h(PacketImageReferences, { value: current.value, t }))
        : h('p', { className: css.meta }, t('packetImagesUnknown')),
      !current.editable && h('p', { className: css.meta }, current.reason))
      : h('p', { className: css.meta }, t('packetImagesUnknown')),
    !open && h(Button, { disabled: busy, onClick: () => read() }, t('packetImagesEdit')),
    open && h(React.Fragment, null, h('h5', null, t('packetImagesDraft')), h('p', { className: css.meta }, t('packetImagesReview')),
      snapshot && h(React.Fragment, null,
        !snapshot.editable && h('p', { className: css.warningText, 'data-image-uneditable': true }, t('packetImagesUneditable') + ': ' + snapshot.reason),
        snapshot.editable && (mode === 'clear' ? h('section', { className: css.configCallout, 'data-image-clear': true },
          h('label', { className: css.intakeCheck }, h('input', { type: 'checkbox', disabled: !!blocked, checked: clearConfirmed,
            onChange: event => draft.update(next => { next.clearConfirmed = event.target.checked; }) }), t('packetImagesClearConfirm')),
          h(Button, { disabled: !!blocked, onClick: () => changePaths(next => { next.mode = 'replace'; next.clearConfirmed = false; }) }, t('packetImagesKeep')))
          : h(React.Fragment, null, h('p', { className: css.meta }, t('packetImagesPreviewHelp')),
            ...rows.map((row, index) => h('div', { key: row.key, className: css.editorField, 'data-image-path-row': index },
              h('label', null, t('packetImagesPath', { index: index + 1 }), h('input', { className: css.editorInput, value: row.path, maxLength: 512, disabled: !!blocked,
                'aria-label': t('packetImagesPath', { index: index + 1 }), onChange: event => changePaths(next => { next.rows = next.rows.map((value, n) => n === index ? { ...value, path: event.target.value } : value); }) })),
              h(Button, { disabled: !!blocked || rows.length <= 1, onClick: () => changePaths(next => { next.rows = next.rows.filter((_, n) => n !== index); }) }, t('packetImagesRemove', { index: index + 1 })))),
            h(Button, { disabled: !!blocked || rows.length >= 4, onClick: () => changePaths(next => { next.rows = [...next.rows, { key: ++next.sequence, path: '' }]; }) }, t('packetImagesAdd')),
            h(Button, { disabled: !!blocked || !validImageSelection({ paths }), onClick: inspect }, t('packetImagesPreview')),
            !previewCurrent && h('p', { className: css.meta, 'data-image-preview-needed': true }, t('packetImagesNeedPreview')),
            previewCurrent && h('div', { className: css.intakePreviews, 'data-image-draft-preview': true }, ...preview.value.previews.map(item => h('figure', { key: item.path, className: css.intakePreview },
              h('img', { src: item.dataUrl, alt: t('packetImagesPreviewAlt', { path: item.path }) }),
              h('figcaption', null, h('code', null, item.path), h('span', null, item.mimeType + ' · ' + t('intakeImageDimensions', item)), h('code', null, 'SHA-256 ' + item.sha256))))),
            snapshot.value !== null && h(Button, { disabled: !!blocked, onClick: () => changePaths(next => { next.mode = 'clear'; next.clearConfirmed = false; }) }, t('packetImagesClear')))),
        stale && h('p', { role: 'alert', className: css.warningText, 'data-image-stale': true }, t('packetImagesStale')),
        !stale && (control.uncertainty || control.status?.running || state.handoff) && h('p', { className: css.warningText }, t(control.uncertainty ? 'wbUnknown' : control.status?.running ? 'wbRunning' : 'wbHeld')),
        h(Button, { variant: 'primary', disabled: !!blocked || !validDraft || !dirty, onClick: save }, t('packetImagesSave')),
        h(RawEvidence, { value: snapshot, label: 'packetImagesSnapshot', t })),
      h('div', { className: css.actions },
        snapshot && (stale || ['rejected', 'read-failed', 'rebase-failed'].includes(notice?.kind)) && h(Button, { disabled: busy || unresolved, onClick: () => read(true) }, t('packetImagesRebase')),
        h(Button, { disabled: busy || unresolved, onClick: () => read() }, t('packetImagesReload')),
        h(Button, { disabled: busy || unresolved, onClick: () => { invalidatePreview(); draft.update(next => { next.open = false; next.snapshot = null; next.rows = []; next.references = null; next.notice = null; }); } }, t('packetImagesClose')))),
    localPending && h('p', { role: 'status', className: css.meta }, t('packetImagesPending')),
    notice && notice.kind !== 'pending' && h('p', { role: notice.kind === 'saved' ? 'status' : 'alert', 'data-image-edit-result': notice.kind, className: notice.kind === 'saved' ? css.meta : css.warningText },
      t({ saved: 'packetImagesSaved', rejected: 'packetImagesRejected', queued: 'packetImagesQueued', unknown: 'packetImagesUnconfirmed', 'read-failed': 'packetImagesReadFailed', 'rebase-failed': 'packetImagesRebaseFailed', 'preview-failed': 'packetImagesPreviewFailed' }[notice.kind]) + (notice.message ? ' ' + notice.message : '')),
    unresolved && h(Button, { disabled: busy, onClick: reconcile }, t('packetImagesReconcile')));
}

function NodeCollaboration({ packetID, state, control, selection, run, packetDrafts, t }) {
  if (!packetID) return null;
  const view = control.node?.id === packetID && control.node.revision === state.revision ? control.node.collaboration : null;
  return h('section', { className: css.inspectorSection, 'data-node-collaboration': packetID },
    h('h4', null, t('wbCollaboration')),
    h('p', { className: css.meta }, t('wbCollaborationHelp')),
    view?.status === 'verified' ? h('ol', null, ...view.value.tasks.map((task, index) => h('li', { key: index },
      h('p', null, task.goal), h('p', { className: css.meta }, t('wbCollaborationWrites') + ': ' + task.writes.join(', ')))))
      : h('p', { className: css.meta }, t(view?.status === 'absent' ? 'wbCollaborationNone' : view?.reason === 'display-limit-exceeded' ? 'wbCollaborationUnshown' : 'wbCollaborationUnknown')),
    h(Button, { disabled: !!control.pending, onClick: () => run('node', selection, false, { packet: packetID }) }, t('wbCollaborationRead')),
    view && h(RawEvidence, { value: view, label: 'wbCollaborationEvidence', t }),
    h(NodeCollaborationEditor, { key: selection.taskDir + '/' + packetID, packetID, state, control, selection, run, packetDrafts, t }));
}

function NodeNativeObservations({ facts, control, selection, run, t }) {
  const actual = facts?.actual;
  const [requested, setRequested] = React.useState('');
  const id = actual?.last?.id;
  if (!id || actual.configuration?.harness !== 'antigravity') return null;
  const calls = (control.metrics?.calls ?? []).filter(call => call['invocation-id'] === id && call.harness === 'antigravity');
  const observed = calls.filter(call => validNativeObservation(call.native));
  return h('section', { className: css.inspectorSection, 'data-node-native': id },
    h('h4', null, t('runsNative')),
    h('p', { className: css.path }, t('wbBindingInvocation') + ': ' + id),
    h(Button, { disabled: !!control.pending, onClick: async () => {
      setRequested('');
      const value = await run('metrics', selection, false, { runId: actual.last['engine-run-id'] });
      if (value?.metrics) setRequested(id);
    } }, t('wbNativeRead')),
    observed.length ? h(NativeObservations, { calls: observed, t, compact: true }) : h('p', { className: css.meta }, t(requested === id && calls.length === 0 ? 'wbNativeOlder' : 'wbNativeMissing')));
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

function ConsoleCanvas({ control, selection, run, packetDrafts, t, focusRequest = 0, onInspect }) {
  const [scope, setScope] = React.useState(null),
    [selected, setSelected] = React.useState(null);
  const [page, setPage] = React.useState(0),
    [whole, setWhole] = React.useState(true),
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
  React.useEffect(() => { onInspect?.(!!selected); return () => onInspect?.(false); }, [selected, onInspect]);
  const manualView = React.useRef(false);
  React.useEffect(() => {
    const container = viewport.current?.parentElement;
    if (!container) return;
    const resize = () => {
      const width = container.clientWidth;
      if (width > 0 && !manualView.current) setView(width < 520 ? 'list' : 'canvas');
      if (width > 520 && viewport.current) setZoom(Math.min(1, Math.max(.9, Math.min((viewport.current.clientWidth - 40) / model.width, (viewport.current.clientHeight - 40) / model.height))));
    };
    resize();
    if (typeof ResizeObserver !== 'function') return;
    const observer = new ResizeObserver(resize);
    observer.observe(container);
    return () => observer.disconnect();
  }, [!!model, focusMode, scope]);
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
    if (stageBlockReason(control, node, state)) return;
    const next = { only: node.stage || 'execute', packet: node.packet ? node.id.split('/')[0] : undefined, revision: state.revision, retry, reopen: node.packet?.status === 'completed' };
    if (next.reopen) { setReason(''); setRequest(next); return; }
    // The explicit click authorizes exactly one revision-bound step. The audit
    // reason describes the action, not a fabricated explanation for the failure.
    void run('step', selection, false, { only: next.only, packet: next.packet, revision: next.revision,
      ...(retry ? { retryReason: 'Manual retry requested from the Flywheel console' } : {}) });
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
    const caption = bindingFactCaption(nodeBindingFacts(node, state, quick), t);
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
        caption
          ? caption
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
    h(
      focusMode ? 'dialog' : 'div',
      {
        ref: canvasDialog,
        className: [css.canvasWorkbench, focusMode ? css.focusCanvas : ''].join(' '),
        'data-inspecting': !!chosen,
        'data-single-run': focused,
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
            single && h('div', { className: css.segmented, 'aria-label': t('stageScopeHelp') },
              h(Button, { 'aria-pressed': whole, onClick: () => { setWhole(true); back(); } }, t('stageFull')),
              h(Button, { 'aria-pressed': !whole, onClick: () => { setWhole(false); setSelected(null); } }, t('stageFocus'))),
            scope && !focused
              ? h(Button, { onClick: back }, t('wbBack'))
              : !single && h('span', null, t('wbOverview')),
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
            { className: css.segmented, hidden: focused },
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
            hidden: view !== 'canvas' || focused,
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
        (view === 'list' || focused) &&
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
            view === 'canvas' && !focused &&
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
          packetDrafts,
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
  packetDrafts,
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
  const bindingFacts = nodeBindingFacts({ ...chosen, packet }, state, quick),
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
  const action = primaryStage && h(Button, {
    variant: 'primary', disabled: !!blocked || (packet?.status === 'completed' && !retry),
    onClick: () => planRequest({ ...chosen, stage: primaryStage }, retry),
  }, t(retry ? packet.status === 'completed' ? 'wbReopen' : 'wbRetry' : chosen.stage ? 'consoleStageRun' : 'wbRunTask'));
  const feedback = typeof packet?.['last-feedback'] === 'string' ? packet['last-feedback'] : packet?.['last-feedback']?.reason || packet?.['last-feedback']?.['parallel-error'] || packet?.commit?.error;
  return h('section', { className: css.nodeInspector, 'aria-label': t('consoleDetails') },
    h('header', { className: css.inspectorHeader }, h('div', null, packetID && h('span', { className: css.meta }, packetID), h('h3', { className: css.heading }, chosen.title)), h(Button, { onClick: onClose, 'aria-label': t('wbClose') }, '×')),
    h('div', { className: css.inspectorBody },
      tab !== 'progress' && h(Button, { onClick: () => setTab('progress') }, t('wbBackToNode')),
      tab === 'progress' && h(React.Fragment, null,
        h('p', { className: css.nodeOutcome, 'data-tone': consoleTone(chosen.status) }, canvasStatus(chosen.status, t)),
        feedback && h('p', { className: css.nodeIssue }, textValue(feedback).slice(0, 800)),
        h('div', { className: css.inspectorActions }, action, sessions.length > 0 && h(Button, { onClick: () => setTab('session') }, t('wbTakeover'))),
        blocked && h('p', { className: css.settingsHelp }, t(blocked)),
        chosen.packet && !chosen.stage && h('ol', { className: css.stageTimeline }, ...packetStages(packet, quick, t).map(([stage, title, status]) => h('li', { key: stage, 'data-tone': consoleTone(status) }, h('span', null, title), h('span', { className: css.meta }, canvasStatus(status, t))))),
        ((chosen.packet && !chosen.stage) || chosen.group) && h(Button, { onClick: () => expand(chosen) }, t(chosen.packet ? 'wbExpand' : 'wbExpandGroup')),
        ['delivery', 'merge'].includes(chosen.id) && h(DeliveryPanel, { control, selection, run, t }),
        h(NodeBindingHistory, { facts: bindingFacts, state, t }),
        h(NodeExecutionEligibility, { key: selection.taskDir + '/' + packetID, packetID, state, control, selection, run, t }),
        h(NodeCollaboration, { packetID, state, control, selection, run, packetDrafts, t }),
        packetID && h(NodeImageInputEditor, { key: 'images:' + selection.taskDir + '/' + packetID, packetID, state, control, selection, run, packetDrafts, t }),
        h(NodeNativeObservations, { facts: bindingFacts, control, selection, run, t }),
        h('section', { className: css.inspectorSection }, h('h4', null, t('wbSession')),
          sessions.length ? sessions.map(([key, value]) => h('article', { key, className: css.sessionSummary },
            h('h4', null, (roleLabels[value.role] ? t(roleLabels[value.role]) : value.role) + ' · ' + value.scope),
            h('dl', { className: css.consoleProperties },
              property(t('consoleAction'), ['pending', 'intent', 'blocked'].includes(value.last?.status) || ['unknown', 'blocked'].includes(value.last?.outcome) ? t('consoleUnconfirmed') : sessionActionLabel(value.action, t)),
              property('Session', value['thread-id']), value.reason && property(t('consoleReason'), value.reason)))) : h('p', { className: css.meta }, t('wbNoSessions'))),
        chosen.stage === 'review' && packet?.evidence?.review && h('dl', { className: css.consoleProperties }, property(t('stageRecordedVerdict'), packet.evidence.review.verdict), property(t('stageRecordedScore'), packet.evidence.review.score)),
        packet && h('dl', { className: css.consoleProperties }, property(t('consoleAttempts'), packet.attempts), property(t('consoleFailures'), packet['failed-attempts'])),
        h('section', { className: css.inspectorSection }, h('h4', null, t('stageMetadata')),
          h('dl', { className: css.consoleProperties }, property('Run', state.run?.id), property(t('executionHome'), packet?.worktree?.path ?? control.status.cwd), property(t('consoleBranch'), packet?.worktree?.branch || state.run?.branch))),
        packetID && h(RuntimeDecisionPanel, { ledger: state['runtime-decisions'], packetID, t }),
        packetID && h(ExecutionPolicyChanges, { packets: { [packetID]: packet }, packetID, t }),
        h(Button, { onClick: () => setTab('evidence') }, t('wbEvidence'))),
      tab === 'session' && h(React.Fragment, null, ...sessions.map(([key, value]) => h(SessionInspector, { key, sessionKey: key, value, control, selection, run, t }))),
      tab === 'evidence' && h(React.Fragment, null,
        packetID && h(Button, { disabled: !!control.pending, onClick: () => run('node', selection, false, { packet: packetID }) }, t('consoleFullNode')),
        h('h4', { className: css.heading }, t('wbEvidence')), h('pre', { className: css.raw }, JSON.stringify(evidence, null, 2)),
        h('dl', { className: css.consoleProperties }, property('HEAD', state.run?.head), property(t('consoleTask'), control.status.taskDir), property(t('projectHome'), state.project?.['workspace-cwd']), property(t('consoleRevision'), state.revision), property('after', packet?.schedule?.after?.join(', ')), property('writes', packet?.schedule?.writes?.join(', '))))));
}

function TaskView({
  hostSessionID,
  configDrafts,
  packetDrafts,
  intakeStore,
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
  const page = useStore((value) => value.page ?? 'canvas');
  const setPage = actions.setPage;
  const [focusRequest, setFocusRequest] = React.useState(0);
  const [inspecting, setInspecting] = React.useState(false);
  const [, refreshIntake] = React.useState(0);
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
    if (page !== 'canvas' || !selection.taskDir || !control.status?.running && control.planningReceipt?.status !== 'queued') return;
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
  }, [page, control.status?.running, control.planningReceipt?.status, control.planningReceipt?.['request-id'], selection.taskDir, selection.cwd, run]);
  const busy = control.pending !== null,
    locked = busy && mutates(control.pending.action),
    state = control.status?.state,
    preparation = control.status?.preparation;
  const invalid = !!pathProblem(selection.taskDir) || !!pathProblem(selection.cwd),
    hasTask = !!selection.taskDir.trim();
  const recoveringIntake = hasTask && intakeStore?.target?.taskDir === selection.taskDir && intakeStore.target.cwd === selection.cwd;
  const newIntake = hasTask && !!intakeStore?.open;
  const intakeLocked = busy || !!control.uncertainty || !!control.configUncertainty || !!control.stale || control.planningReceipt?.status === 'queued';
  const intakeWorkspaceReady = typeof control.runsCwd === 'string' && control.runsCwd.startsWith('/') && !pathProblem(control.runsCwd) && !control.runsError;
  const openIntake = () => {
    if (!hasTask || recoveringIntake || intakeLocked || !intakeWorkspaceReady || association && association.outcome !== 'confirmed') return;
    // Discovery identifies the conversation workspace; the selected task can
    // belong to another worktree. A later catalog refresh cannot retarget an
    // already opened draft or erase its user-provided goal and authorization.
    intakeStore.draftWorkspace ||= control.runsCwd;
    intakeStore.origin = selectionOf(selection); intakeStore.open = true;
    refreshIntake(value => value + 1); setPage('canvas');
  };
  const closeIntake = discard => {
    if (intakeLocked) return;
    intakeStore.open = false;
    if (discard) { intakeStore.draft = null; intakeStore.target = null; intakeStore.origin = null; intakeStore.draftWorkspace = null; }
    refreshIntake(value => value + 1);
  };
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
          (['launch', 'start'].includes(action) && control.stale) ||
          (['launch', 'start'].includes(action) && !!state?.handoff) ||
          (['launch', 'start'].includes(action) && !!state?.planning && (!currentPlanningApproval(state.planning) || !currentPlanningStartAllowed(state.planning) || !!state.planning['pending-activation'])),
        onClick: () => run(action, selection, probe),
      },
      t(label),
    );
  const blocked = Object.values(state?.packets ?? {}).some((p) =>
    ['l1-failed', 'review-failed', 'escalated', 'blocked'].includes(p.status),
  );
  const diagnostics = h('section', { className: css.advanced, 'aria-label': t('consoleDiagnostics') },
    h('h3', { className: css.heading }, t('consoleDiagnostics')),
    h('div', { className: css.actions }, button('config', 'config', false), button('doctor', 'doctor', false), button('doctor', 'probe', false, true)),
    h('p', { className: css.settingsHelp }, t('inferenceBoundary')),
    h(Diagnostic, { value: control.diagnostic, t }), h(Configuration, { value: control.configuration, t }),
    control.raw && h('section', { className: css.details }, h('h4', null, t('raw')), h('pre', { className: css.raw }, JSON.stringify(control.raw, null, 2))));
  // The workbench is not a chat target: tell the host to give this view the
  // full content area, and mark it so the stylesheet can hide the composer
  // seat only while this view is mounted. Switching back to Chat restores it.
  return h('section', { className: css.page, 'aria-label': t('consoleTitle'), 'data-page': page, 'data-conversation-composer-overlay': '', 'data-flywheel-page': '' },
    h('style', { 'data-plugin': '@ckanner/flywheel', 'data-plugin-css': '@ckanner/flywheel/client.module.css' }, stylesheet),
    h('header', { className: css.workbenchHeader, hidden: page === 'config' },
      h('div', { className: css.taskIdentity }, h(TaskChooser, { control, selection, hostSessionID, association, changeSelection, run, discover, t }), h(TaskLocations, { control, selection, association, t }),
        hasTask && !recoveringIntake && h(React.Fragment, null,
          h('div', { className: css.actions }, h(Button, { disabled: intakeLocked || !intakeWorkspaceReady || newIntake || !!association && association.outcome !== 'confirmed', 'aria-expanded': newIntake, onClick: openIntake }, t('intakeAnother'))),
          !newIntake && !intakeWorkspaceReady && h('p', { className: css.settingsHelp, role: 'status' },
            t(control.runsError ? 'intakeWorkspaceFailed' : 'intakeNoWorkspace') + (control.runsError ? ' ' + control.runsError : '')))),
      h('div', { className: css.repositoryLinks },
        state?.project?.url && h('a', { href: state.project.url, target: '_blank', rel: 'noopener noreferrer' }, state.project.repo),
        h('a', { href: 'https://github.com/ckanner/flywheel', target: '_blank', rel: 'noopener noreferrer' }, 'Flywheel · GitHub ↗'),
        page !== 'config' && h(Button, { onClick: () => setPage('config') }, t('consoleConfig')))),
    h('div', { className: css.workbenchNav, hidden: page === 'config' },
      h('nav', { className: css.consoleTabs, 'aria-label': t('view') },
        ...['canvas', 'runs'].map(key => h(Button, { key, 'aria-pressed': page === key, onClick: () => setPage(key) }, t(key === 'canvas' ? 'consoleCanvas' : 'consoleRuns')))),
      page === 'canvas' && hasTask && !preparation && h('div', { className: css.workbenchStatus },
        state && h('span', { 'data-tone': consoleTone(control.stale ? 'unknown' : control.status.running ? 'running' : state.status) }, singleRunKind(control) ? singleRunLabel(control, t) + ' · ' + t('stageFullState', { status: t(state.status === 'completed' ? 'stageDelivered' : 'stageUndelivered') }) : taskLabel(control.status, t)),
        button('status', 'status', true),
        blocked && !inspecting && !control.status?.running && h(Button, { variant: 'primary', disabled: busy, onClick: () => setFocusRequest(v => v + 1) }, t('wbBlocked')),
        !control.status?.running && state?.status !== 'completed' && (state || !control.status) && !(blocked && inspecting) && button('start', singleRunKind(control) ? 'consoleWhole' : 'start', true, false, blocked ? 'outline' : 'primary'),
        (control.status?.running || state?.status === 'running' || locked || control.uncertainty) && button('stop', 'stop', true))),
    (control.error || control.uncertainty || control.status?.workspaceWarnings?.length || control.pending && !(control.pending.action === 'status' && control.status)) &&
      !(page === 'config' && (control.pending?.action === 'config-edit' || control.error?.action === 'config-edit') && !control.uncertainty) && h(Feedback, { control, selection, reconcile, t }),
    h('div', { className: css.workflowPage, hidden: page !== 'canvas' },
      newIntake && h('section', { className: css.intake, 'aria-label': t('intakeAnother') },
        h('p', { className: css.settingsHelp }, t('intakeAnotherHelp')),
        h(TaskIntake, { key: hostSessionID, control, selection, intakeStore, changeSelection, run, newTask: true, t }),
        h('p', { className: css.settingsHelp }, t('intakeDraftHelp')),
        h('div', { className: css.actions },
          h(Button, { disabled: intakeLocked, onClick: () => closeIntake(false) }, t('intakeHide')),
          h(Button, { disabled: intakeLocked, onClick: () => closeIntake(true) }, t('intakeDiscard')))),
      state?.['shared-execution'] && h('section', { className: css.notice, role: 'status' }, h('strong', null, t('sharedBatch')), h('p', null, state['shared-execution'].packets.join(' · ')), state['shared-execution']['last-error'] && h('p', { role: 'alert' }, state['shared-execution']['last-error'])),
      hasTask && !recoveringIntake && control.status && !state && !preparation && !busy && !control.uncertainty && h('div', { className: css.launchOptionsBody },
        h('label', { className: css.editorField }, t('launchMode'), h('select', { className: css.modeSelect, value: selection.mode, onChange: e => actions.setMode(e.target.value) }, ...['auto','quick','full'].map(mode => h('option', { key: mode, value: mode }, mode === 'auto' ? 'Auto' : mode === 'quick' ? 'ultraWork / Quick' : 'Full')))),
        button('launch', 'launch', true, false, 'primary')),
      preparation && h(InvestigationPanel, { control, selection, run, t }),
      state?.planning && h(PlanningPanel, { key: 'planning:' + selectionKey(selectionOf(selection)), control, selection, run, t }),
      state && h(RuntimeDecisionPanel, { ledger: state['runtime-decisions'], t }),
      state && h(ExecutionPolicyChanges, { packets: state.packets, t }),
      !state && !preparation && !newIntake && h('div', { className: css.workbenchEmpty }, h('h2', { className: css.title }, t(recoveringIntake ? 'intakeResume' : hasTask ? 'wbReadFirst' : 'taskNoSelection')), h('p', null, t(recoveringIntake ? 'intakeResumeHelp' : hasTask ? 'wbStartHelp' : 'taskEmptyHelp')), (!hasTask || recoveringIntake) && h(TaskIntake, { key: hostSessionID, control, selection, intakeStore, changeSelection, run, t })),
      !preparation && (!state?.planning || state.planning.active) && h(ConsoleCanvas, { key: 'canvas:' + selectionKey(selectionOf(selection)), control, selection, run, packetDrafts, t, focusRequest, onInspect: setInspecting })),
    h('div', { className: css.settingsPage, hidden: page !== 'config' },
      h(ConfigEditor, { key: selectionKey(selectionOf(selection)), contextKey: selectionKey(selectionOf(selection)), cache: configDrafts, control, selection, run, t, active: page === 'config', onClose: () => setPage('canvas'), diagnostics })),
    h('div', { className: css.historyPage, hidden: page !== 'runs' },
      h(RunExplorer, { control, selection, run, changeSelection, showCanvas: () => setPage('canvas'), t, active: page === 'runs' }),
      hasTask && !preparation && h('section', { className: css.legacySummary }, h(TaskStatus, { control, selection, t }))));
}

Object.assign(zh, { consoleDiagnostics: '环境检查' });
Object.assign(en, { consoleDiagnostics: 'Environment checks' });
Object.assign(zh, { wbBackToNode: '← 返回节点', wbTakeover: '进入会话', wbRetry: '修复并重试', wbBlocked: '处理失败任务', taskThisSession: '当前会话', consoleRuns: '记录', status: '刷新', start: '继续流程', wbEvidence: '查看证据', wbNoSessions: '尚无会话记录' });
Object.assign(en, { wbBackToNode: '← Back to node', wbTakeover: 'Open session', wbRetry: 'Repair and retry', wbBlocked: 'Resolve failure', taskThisSession: 'Current conversation', consoleRuns: 'History', status: 'Refresh', start: 'Continue workflow', wbEvidence: 'Inspect evidence', wbNoSessions: 'No session recorded yet' });

Object.assign(zh,{investigationNewRound:'追加调查轮',investigationRound:'第 {round} 轮 · 剩余动作 {remaining}',investigationRoundHelp:'填写新增证据和下一轮取证动作。既有报告保留，下一轮上限不得超过剩余预算。保存后可启动本轮调查。',investigationRoundContract:'新调查轮契约 JSON',investigationRoundSave:'保存新调查轮',investigationRoundInvalid:'请输入有效的调查契约 JSON 对象。',investigationRoundStale:'调查版本已变化，请重新打开追加调查轮。',investigationHistory:'此前调查轮'});
Object.assign(en,{investigationNewRound:'Add investigation round',investigationRound:'Round {round} · {remaining} actions remaining',investigationRoundHelp:'Add new evidence and the next bounded actions. Existing reports remain intact; limits cannot exceed the remaining budget. Start the round after saving.',investigationRoundContract:'New investigation contract JSON',investigationRoundSave:'Save new round',investigationRoundInvalid:'Enter a valid investigation contract JSON object.',investigationRoundStale:'The investigation revision changed. Reopen the new-round editor.',investigationHistory:'Previous investigation rounds'});

Object.assign(zh,{investigationAnalyze:'生成调查报告'});
Object.assign(en,{investigationAnalyze:'Analyze investigation evidence'});
