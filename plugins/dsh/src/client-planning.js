Object.assign(zh, {
  planning: '规划', planningTitle: '方案与审查', designPlanRole: '方案设计', designReviewRole: '方案审查', planReviewRole: '执行计划审查', bindingSource: '绑定来源',
  planningHelp: '先设计、独立审查，再批准当前版本。批准方案与启动实现是两个动作；子计划必须保持在已批准方案的范围内。',
  planningWorkflowPending: '实现流程尚未激活；当前进行方案设计与审查。',
  planningPolicy: '授权策略', 'planningPolicy_review-first': '审查后由用户批准', planningPolicy_delegated: '沿用已记录的实施授权',
  planningPhase_drafting: '正在准备方案', planningPhase_reviewing: '独立审查', 'planningPhase_awaiting-design-approval': '等待批准方案', 'planningPhase_awaiting-input': '等待讨论与决定', planningPhase_approved: '方案已批准', planningPhase_active: '执行计划已激活', planningPhase_blocked: '已阻断 · 查看原因', 'planningPhase_budget-stopped': '规划预算已停止', planningPhase_executing: '执行中', planningPhase_activating: '正在激活执行计划', planningPhase_ready: '执行计划就绪', planningPhase_cancelled: '规划已取消',
  planningRound: '累计规划轮次 {rounds} · 单次请求最多 {max} 轮', planningDraft: '当前方案', planningPrevious: '上一版本（对照）', planningApproved: '最近批准的根方案', planningApprovedDocument: '已批准方案正文', planningParent: '所属根方案', planningRequestRound: '当前请求：{scope} {id} · {rounds}/{max} 轮 · {status}', planningNoApproval: '还没有批准的根方案', planningNoDraft: '方案尚未生成。设计与审查由后台引擎完成。', planningNoDocument: '本快照未包含正文；原文保存在任务账本中。', planningReference: '版本 {revision} · {digest}',
  planningReview: '审查记录', planningNoReview: '本版本尚无独立审查回执', planningReview_pass: '通过', planningReview_revise: '需修订', 'planningReview_needs-user-decision': '需用户决定', planningReview_blocked: '阻断', planningEvidence: '证据', planningImpact: '影响', planningResolution: '处理结果', planningTarget: '目标',
  planningStartApproved: '开始已批准方案', planningApprove: '批准此版本', planningApproveStart: '批准此版本并开始', planningApprovalHelp: '按钮只批准上方显示的版本与摘要；版本变更后须重新审阅。模型和 Team 的意见不会代替你的批准。', planningApprovedBy: '批准记录：{kind} · {source}', planningStartAuthorized: '实现启动已授权', planningStartSeparate: '方案已批准；尚未授权启动实现', planningActivationPending: '执行计划激活事务未完成；实现保持阻断。',
  planningFeedback: '方案反馈', planningFeedbackHelp: '写明需要调整的目标、约束或取舍。反馈保留当前方案引用；采用后再运行 Plan 形成新版本。', planningSendFeedback: '提交反馈', planningFeedbackApplied: '已采用的反馈', planningReceipt_queued: '已排队 · 等待引擎采用', planningReceipt_applied: '已应用', planningReceipt_rejected: '未采用', planningReceipt_unchanged: '未变更', planningReceipt: '本次请求',
  planningBlockedInvocation: '未解开的规划调用', planningBlockedInvocationHelp: '引擎无法证明这次调用是否已派发或已结束。退役只结束这次未知调用，不应用结果、不重放、不复用旧线程；之后请显式运行 Plan 产生新版本。', planningAbandonReason: '退役原因', planningAbandonConfirm: '我确认退役这次调用：不应用结果、不重放、不复用旧线程', planningAbandon: '退役未知调用', planningAbandonApplied: '已退役；没有应用结果', planningAbandonRejected: '退役未生效：', planningAbandonInvocation: '调用', planningAbandonStatus: '派发/结果/清理',
  planningPlan: '运行 Plan', planningScope: '规划范围', planningScope_design: '整体方案', planningScope_milestone: '里程碑', planningScope_packet: '工作单元', planningScope_replan: '重新规划', planningScopeID: '范围 ID', planningPlanHelp: 'Plan 只运行设计或细化及其审查，结束后停下。不会自动开始实现。', planningChild: '当前执行子计划', planningNoChild: '尚无执行子计划；根方案可以先独立批准。', planningActive: '当前激活版本', planningNotActive: '尚未激活', planningFiles: '契约文件', planningTrace: '需求与里程碑', planningDecision: '设计决定', planningStateRevision: '任务状态版本 {revision}', planningInherited: '继承 {source}；修改后保存为独立角色配置', planningInheritedHelp: '新增规划角色默认继承现有 Plan / Review 的后端绑定，保持独立会话和权限。只修改一个角色时不会改动其来源角色。',
});
Object.assign(en, {
  planning: 'Planning', planningTitle: 'Design and review', designPlanRole: 'Design planning', designReviewRole: 'Design review', planReviewRole: 'Execution plan review', bindingSource: 'Binding source',
  planningHelp: 'Design and independent review precede approval of this exact version. Approval and implementation start are separate actions. Child plans must stay within the approved design.',
  planningWorkflowPending: 'Implementation workflow is not active yet. Design and independent review are in progress.',
  planningPolicy: 'Authorization policy', 'planningPolicy_review-first': 'Human approval after review', planningPolicy_delegated: 'Recorded implementation authorization',
  planningPhase_drafting: 'Drafting', planningPhase_reviewing: 'Independent review', 'planningPhase_awaiting-design-approval': 'Awaiting design approval', 'planningPhase_awaiting-input': 'Awaiting discussion and decisions', planningPhase_approved: 'Design approved', planningPhase_active: 'Execution plan active', planningPhase_blocked: 'Blocked · inspect reason', 'planningPhase_budget-stopped': 'Planning budget stopped', planningPhase_executing: 'Executing', planningPhase_activating: 'Activating execution plan', planningPhase_ready: 'Execution plan ready', planningPhase_cancelled: 'Planning cancelled',
  planningRound: '{rounds} cumulative planning rounds · up to {max} per request', planningDraft: 'Current design', planningPrevious: 'Previous version for comparison', planningApproved: 'Most recently approved root design', planningApprovedDocument: 'Approved design document', planningParent: 'Parent design', planningRequestRound: 'Current request: {scope} {id} · {rounds}/{max} rounds · {status}', planningNoApproval: 'No root design has been approved', planningNoDraft: 'No design has been produced yet. The background engine owns design and review.', planningNoDocument: 'This snapshot does not include the document. The original is retained in the task ledger.', planningReference: 'Version {revision} · {digest}',
  planningReview: 'Review record', planningNoReview: 'No independent review receipt for this version', planningReview_pass: 'Pass', planningReview_revise: 'Revise', 'planningReview_needs-user-decision': 'Needs user decision', planningReview_blocked: 'Blocked', planningEvidence: 'Evidence', planningImpact: 'Impact', planningResolution: 'Resolution', planningTarget: 'Target',
  planningStartApproved: 'Start approved design', planningApprove: 'Approve this version', planningApproveStart: 'Approve this version and start', planningApprovalHelp: 'Approval binds the displayed version and digest. A changed version needs another review. Model and Team opinions do not replace your approval.', planningApprovedBy: 'Approval: {kind} · {source}', planningStartAuthorized: 'Implementation start authorized', planningStartSeparate: 'Design approved; implementation start is not authorized yet', planningActivationPending: 'Execution plan activation is incomplete; implementation remains blocked.',
  planningFeedback: 'Design feedback', planningFeedbackHelp: 'Describe changes to goals, constraints or decisions. Feedback retains this design reference. Run Plan after adoption to produce the next version.', planningSendFeedback: 'Submit feedback', planningFeedbackApplied: 'Applied feedback', planningReceipt_queued: 'Queued · awaiting engine adoption', planningReceipt_applied: 'Applied', planningReceipt_rejected: 'Not adopted', planningReceipt_unchanged: 'Unchanged', planningReceipt: 'Request receipt',
  planningBlockedInvocation: 'Unresolved planning call', planningBlockedInvocationHelp: 'The engine cannot prove whether this call was dispatched or finished. Abandoning retires only this unknown invocation: no result is applied, it is not replayed, and the old thread is not resumed. Run Plan explicitly afterwards to produce a new version.', planningAbandonReason: 'Reason for abandonment', planningAbandonConfirm: 'I confirm this retires the invocation: no result is applied, no replay, and the old thread is not resumed', planningAbandon: 'Abandon unknown invocation', planningAbandonApplied: 'Abandoned; no result was applied', planningAbandonRejected: 'Abandonment did not apply: ', planningAbandonInvocation: 'Invocation', planningAbandonStatus: 'Dispatch/outcome/cleanup',
  planningPlan: 'Run Plan', planningScope: 'Planning scope', planningScope_design: 'Overall design', planningScope_milestone: 'Milestone', planningScope_packet: 'Work packet', planningScope_replan: 'Replan', planningScopeID: 'Scope ID', planningPlanHelp: 'Plan runs design or refinement and its review, then stops. It does not automatically start implementation.', planningChild: 'Current execution child plans', planningNoChild: 'No execution child plan yet. The root design can be approved independently.', planningActive: 'Active version', planningNotActive: 'Not active yet', planningFiles: 'Contract files', planningTrace: 'Requirements and milestones', planningDecision: 'Design decisions', planningStateRevision: 'Task state revision {revision}', planningInherited: 'Inherits {source}; editing saves an independent role binding', planningInheritedHelp: 'The new planning roles inherit existing Plan / Review backend bindings with separate sessions and permissions. Editing one role leaves its source role unchanged.',
});
Object.assign(zh, { planningInstructions: '本次规划说明（可选）', planningInstructionsHelp: '只补充当前范围的细化要求。目标、固定决定或实施范围有变化时，请使用上方的方案反馈。', planningApprovalIncomplete: '审批需要完整方案与审查记录；当前快照省略了内容，按钮审批已停用。请在交互式 CLI 中审阅并批准确切版本。' });
Object.assign(en, { planningInstructions: 'Instructions for this planning scope (optional)', planningInstructionsHelp: 'Add refinement instructions for this scope. Use Design feedback above to change goals, fixed decisions or implementation scope.', planningApprovalIncomplete: 'Approval requires the complete design and review. This snapshot omits content, so approval buttons are disabled. Review and approve the exact version through the interactive CLI.' });
Object.assign(zh, { planningExcerpt: '为满足视图大小上限，此快照省略了部分历史或正文。空的文件列表不表示原文件为空；完整记录保留在原任务账本中，可用不带 --compact 的 control status 读取。' });
Object.assign(en, { planningExcerpt: 'This snapshot omits some history or content to fit the view size limit. An empty file list does not mean the original files are empty. Complete records remain in the original task ledger; read them with control status without --compact.' });

const planningRef = value => record(value) && Number.isSafeInteger(value.revision) && value.revision > 0 && typeof value.digest === 'string' && /^[a-f0-9]{64}$/.test(value.digest);
const samePlanningRef = (a, b) => planningRef(a) && planningRef(b) && a.revision === b.revision && a.digest === b.digest;
const planningItems = (value, check) => value == null || Array.isArray(value) && value.every(check);
function validPlanningSnapshot(value) {
  return record(value) && value.version === 1 && typeof value.phase === 'string' &&
    record(value.brief) && typeof value.brief.goal === 'string' && ['review-first', 'delegated'].includes(value.policy) &&
    Number.isSafeInteger(value.rounds) && value.rounds >= 0 && Number.isSafeInteger(value['max-rounds']) && value['max-rounds'] > 0 &&
    ['current-design', 'current-plan'].every(key => value[key] == null || planningRef(value[key])) &&
    (value['current-plans'] == null || record(value['current-plans']) && Object.values(value['current-plans']).every(planningRef)) &&
    planningItems(value.designs, d => record(d) && planningRef(d.ref) && typeof d.document === 'string' &&
      ['requirements', 'decisions', 'milestones'].every(key => planningItems(d[key], record))) &&
    planningItems(value.plans, p => record(p) && planningRef(p.ref) && planningRef(p.design) && typeof p.scope === 'string' && typeof p['scope-id'] === 'string' &&
      (p.files == null || record(p.files) && Object.values(p.files).every(content => typeof content === 'string'))) &&
    planningItems(value.reviews, r => record(r) && ['design', 'plan'].includes(r.kind) && planningRef(r.target) &&
      typeof r.id === 'string' && typeof r.summary === 'string' && ['pass', 'revise', 'needs-user-decision', 'blocked'].includes(r.verdict) && planningItems(r.findings, record)) &&
    planningItems(value.approvals, a => record(a) && planningRef(a.design) && typeof a['review-id'] === 'string' && ['human', 'delegated'].includes(a.kind)) &&
    planningItems(value.feedback, f => record(f) && typeof f.text === 'string' && typeof f['request-id'] === 'string') &&
    planningItems(value.requests, record) && typeof value['start-authorized'] === 'boolean' &&
    (value.projection == null || record(value.projection) && value.projection.kind === 'console-excerpt' && typeof value.projection['approval-complete'] === 'boolean' && value.projection.ledger === 'state.json' &&
      (value.projection.omitted == null || record(value.projection.omitted) && Object.values(value.projection.omitted).every(count => Number.isSafeInteger(count) && count >= 0)));
}
function validPlanningReceipt(value) {
  return record(value) && (value['request-id'] === undefined || typeof value['request-id'] === 'string') && ['queued', 'applied', 'rejected', 'unchanged'].includes(value.status) &&
    (value.message === undefined || typeof value.message === 'string');
}
function currentPlanningReview(planning, kind, ref) {
  return [...(planning.reviews ?? [])].reverse().find(review => review.kind === kind && samePlanningRef(review.target, ref));
}
function currentPlanningApproval(planning) {
  const ref = planning['current-design'], review = currentPlanningReview(planning, 'design', ref);
  const design = (planning.designs ?? []).find(value => samePlanningRef(value.ref, ref));
  return design?.['input-revision'] === planning['input-revision'] && review?.verdict === 'pass' && [...(planning.approvals ?? [])].reverse().find(approval =>
    samePlanningRef(approval.design, ref) && approval['review-id'] === review.id && (planning.policy === 'delegated' || approval.kind === 'human'));
}
function currentPlanningStartAllowed(planning) {
  const ref = planning['current-design'];
  return planning['start-authorized'] && planningRef(ref) && (planning.brief['implementation-authorized'] === true ||
    (planning.approvals ?? []).some(approval => samePlanningRef(approval.design, ref) && approval.kind === 'human' && approval.start === true));
}
let planningRequestSequence = 0;
function planningRequestID() { return 'dsh-' + Date.now().toString(36) + '-' + (++planningRequestSequence).toString(36); }
function PlanningReference({ value, t }) {
  return planningRef(value) && h('code', { className: css.planningRef }, t('planningReference', { revision: value.revision, digest: value.digest }));
}
function PlanningReview({ value, t }) {
  if (!value) return h('p', { className: css.meta }, t('planningNoReview'));
  return h('section', { className: css.planningReview, 'aria-label': t('planningReview'), 'data-review-id': value.id },
    h('h4', null, t('planningReview') + ' · ' + t('planningReview_' + value.verdict)),
    h('p', null, value.summary),
    h('p', { className: css.meta }, [value.role, value.binding, value.invocation].filter(Boolean).join(' · ')),
    ...(value.findings ?? []).map((finding, index) => h('article', { key: finding.id ?? index, className: css.binding },
      h('strong', null, [finding.id, finding.severity, finding.status].filter(Boolean).join(' · ')),
      ...[['planningTarget', 'target'], ['planningEvidence', 'evidence'], ['planningImpact', 'impact'], ['planningResolution', 'resolution']].map(([label, field]) =>
        finding[field] && h('p', { key: field }, h('strong', null, t(label) + ': '), textValue(finding[field]))))));
}
function PlanningDocument({ value, t }) {
  return h('article', { className: css.planningDocument },
    h(PlanningReference, { value: value.ref, t }),
    value.document ? h('pre', { className: css.planningProse }, value.document) : h('p', { className: css.meta }, t('planningNoDocument')),
    value.requirements?.length > 0 && h('details', null, h('summary', null, t('planningTrace')),
      h('ul', null, ...value.requirements.map((requirement, index) => h('li', { key: requirement.id ?? index },
        [requirement.id, requirement.title, requirement.milestone, requirement.verification].filter(Boolean).join(' · '))))),
    value.decisions?.length > 0 && h('details', null, h('summary', null, t('planningDecision')),
      h('ul', null, ...value.decisions.map((decision, index) => h('li', { key: decision.id ?? index },
        [decision.id, decision.kind, decision.statement, decision.rationale].filter(Boolean).join(' · '))))));
}
const blockedPlanningRoles = new Set(['design-plan', 'design-review', 'plan', 'plan-review']);
function BlockedInvocation({ recordKey, record, revision, busy, run, selection, t }) {
  const [reason, setReason] = React.useState('');
  const [confirmed, setConfirmed] = React.useState(false);
  const [receipt, setReceipt] = React.useState(null);
  const last = record.last ?? {};
  const submit = async () => {
    const result = await run('session', selection, false, {
      session: { action: 'abandon', key: recordKey, revision, reason: reason.trim() },
    });
    const next = result?.['session-control'] ?? null;
    setReceipt(next);
    if (next?.applied) {
      setReason('');
      setConfirmed(false);
    }
  };
  return h('article', { className: css.binding, 'data-session-key': recordKey },
    h('h4', null, t('planningBlockedInvocation')),
    h('code', { className: css.planningRef }, recordKey),
    h('p', { className: css.meta }, t('planningAbandonInvocation') + ': ' + (last.id ?? '—') + ' · ' + t('planningAbandonStatus') + ': ' + [last.dispatch, last.outcome, last['cleanup-confirmed'] ? 'cleanup-confirmed' : 'cleanup-unknown'].filter(Boolean).join(' / ')),
    record.reason && h('p', { role: 'alert' }, record.reason),
    h('p', { className: css.meta }, t('planningBlockedInvocationHelp')),
    h('textarea', { className: css.planningInput, 'aria-label': t('planningAbandonReason'), maxLength: 1000, value: reason, disabled: busy, onChange: event => setReason(event.target.value) }),
    h('label', null, h('input', { type: 'checkbox', checked: confirmed, disabled: busy, onChange: event => setConfirmed(event.target.checked) }), ' ', t('planningAbandonConfirm')),
    h('div', { className: css.actions },
      h(Button, { variant: 'primary', disabled: busy || !reason.trim() || !confirmed, onClick: submit }, t('planningAbandon'))),
    receipt && h('p', { role: receipt.applied ? 'status' : 'alert', 'data-session-control': receipt.applied ? 'applied' : 'rejected' },
      receipt.applied ? t('planningAbandonApplied') : t('planningAbandonRejected') + (receipt.message ? ' ' + receipt.message : '')));
}

function PlanningPanel({ control, selection, run, t }) {
  const [feedback, setFeedback] = React.useState(''), [scope, setScope] = React.useState('design'), [scopeID, setScopeID] = React.useState(''), [instructions, setInstructions] = React.useState('');
  const p = control.status?.state?.planning, stateRevision = control.status?.state?.revision;
  const sessions = control.status?.state?.sessions?.records ?? {};
  const blocked = Object.entries(sessions).filter(([, record]) => record && blockedPlanningRoles.has(record.role) && record.last?.status === 'blocked');
  if (!p) return null;
  const current = (p.designs ?? []).find(design => samePlanningRef(design.ref, p['current-design']));
  const previous = [...(p.designs ?? [])].reverse().find(design => current && design.ref.revision < current.ref.revision);
  const review = currentPlanningReview(p, 'design', p['current-design']);
  const approval = currentPlanningApproval(p), lastApproval = [...(p.approvals ?? [])].reverse()[0];
  const startAllowed = currentPlanningStartAllowed(p);
  const approvedDesign = lastApproval && (p.designs ?? []).find(design => samePlanningRef(design.ref, lastApproval.design));
  const latestRequest = (p.requests ?? []).at(-1);
  const busy = !!control.pending || !!control.uncertainty || control.stale || !Number.isSafeInteger(stateRevision);
  const approvalComplete = !!current?.document.trim() && p.projection?.['approval-complete'] !== false;
  const canApprove = !busy && !control.status.running && !p['pending-activation'] && approvalComplete && review?.verdict === 'pass' && current['input-revision'] === p['input-revision'];
  const childPlans = (p.plans ?? []).filter(plan => Object.values(p['current-plans'] ?? {}).some(ref => samePlanningRef(ref, plan.ref)));
  const receipt = control.planningReceipt;
  const receiptStatus = receipt && (p.feedback ?? []).some(item => item['request-id'] === receipt['request-id']) ? 'applied' : receipt?.status;
  const dispatch = fields => run('planning', selection, false, { planning: { revision: stateRevision, 'request-id': planningRequestID(), ...fields } });
  const submitFeedback = async () => {
    const result = await dispatch({ action: 'feedback', ...(p['current-design'] ? { design: p['current-design'] } : {}), text: feedback.trim(), start: false });
    if (result?.['planning-control']?.status !== 'rejected' && result) setFeedback('');
  };
  const submitPlan = async () => {
    const result = await dispatch({ action: 'plan', scope,
      ...(['milestone', 'packet', 'replan'].includes(scope) && scopeID.trim() ? { 'scope-id': scopeID.trim() } : {}),
      ...(instructions.trim() ? { text: instructions.trim() } : {}), start: true });
    if (result?.['planning-control']?.status !== 'rejected' && result) setInstructions('');
  };
  return h('section', { className: css.planningPanel, 'aria-label': t('planningTitle') },
    h('h2', { className: css.heading }, t('planningTitle')),
    h('p', null, p.brief.goal), h('p', { className: css.meta }, t('planningHelp')),
    p.projection && Object.values(p.projection.omitted ?? {}).some(count => count > 0) && h('aside', { className: css.meta, 'data-planning-excerpt': true },
      h('p', null, t('planningExcerpt')), h('code', null, selection.taskDir.replace(/\/$/, '') + '/state.json')),
    h('div', { className: css.actions }, h('strong', { role: 'status' }, t('planningPhase_' + p.phase)),
      h('span', { className: css.badge }, t('planningPolicy') + ': ' + t('planningPolicy_' + p.policy))),
    h('p', { className: css.meta }, t('planningRound', { rounds: p.rounds, max: p['max-rounds'] }) + ' · ' + t('planningStateRevision', { revision: stateRevision })),
    latestRequest && h('p', { className: css.meta }, t('planningRequestRound', { scope: t('planningScope_' + latestRequest.scope), id: latestRequest['scope-id'] || '', rounds: latestRequest.rounds ?? 0, max: latestRequest['max-rounds'] ?? p['max-rounds'], status: textValue(latestRequest.status) })),
    p.reason && h('p', null, p.reason),
    p['pending-activation'] && h('p', { role: 'alert', className: css.warningText }, t('planningActivationPending')),
    blocked.length > 0 && h('section', { 'aria-label': t('planningBlockedInvocation') },
      h('h3', null, t('planningBlockedInvocation')),
      ...blocked.map(([key, record]) => h(BlockedInvocation, { key, recordKey: key, record, revision: stateRevision, busy, run, selection, t }))),
    h('section', { 'aria-label': t('planningDraft') }, h('h3', null, t('planningDraft')),
      current ? h(PlanningDocument, { value: current, t }) : h('p', null, t('planningNoDraft')),
      h(PlanningReview, { value: review, t }),
      previous && h('details', { className: css.planningPrevious }, h('summary', null, t('planningPrevious')), h(PlanningDocument, { value: previous, t }), h(PlanningReview, { value: currentPlanningReview(p, 'design', previous.ref), t }))),
    h('section', { 'aria-label': t('planningApproved') }, h('h3', null, t('planningApproved')),
      lastApproval ? h(React.Fragment, null, h(PlanningReference, { value: lastApproval.design, t }), h('p', { className: css.meta }, t('planningApprovedBy', { kind: lastApproval.kind, source: textValue(lastApproval.source) }))) : h('p', null, t('planningNoApproval')),
      approvedDesign && !samePlanningRef(approvedDesign.ref, current?.ref) && h('details', null, h('summary', null, t('planningApprovedDocument')), h(PlanningDocument, { value: approvedDesign, t })),
      approval && h('p', null, t(startAllowed ? 'planningStartAuthorized' : 'planningStartSeparate')),
      h('p', { className: css.meta }, t('planningApprovalHelp')),
      current && !approvalComplete && h('p', { role: 'alert' }, t('planningApprovalIncomplete')),
      h('div', { className: css.actions },
        h(Button, { disabled: !canApprove || !!approval, onClick: () => dispatch({ action: 'approve', design: current.ref, start: false }) }, t('planningApprove')),
        h(Button, { variant: 'primary', disabled: !canApprove || !!approval && startAllowed, onClick: () => dispatch({ action: 'approve', design: current.ref, start: true }) }, t(approval ? 'planningStartApproved' : 'planningApproveStart')))),
    h('section', { 'aria-label': t('planningChild') }, h('h3', null, t('planningChild')),
      childPlans.length === 0 && h('p', null, t('planningNoChild')),
      ...childPlans.map(plan => h('article', { key: plan.ref.digest, className: css.binding },
        h('h4', null, t('planningScope_' + plan.scope) + ' · ' + plan['scope-id']), h(PlanningReference, { value: plan.ref, t }),
        h('p', { className: css.meta }, t('planningParent')), h(PlanningReference, { value: plan.design, t }),
        h('p', { className: css.meta }, t((p.active?.plans ?? []).some(ref => samePlanningRef(ref, plan.ref)) ? 'planningActive' : 'planningNotActive')),
        h(PlanningReview, { value: currentPlanningReview(p, 'plan', plan.ref), t }),
        h('details', null, h('summary', null, t('planningFiles')), ...Object.entries(plan.files ?? {}).map(([path, content]) => h('details', { key: path }, h('summary', null, path), h('pre', { className: css.planningProse }, content))))))),
    h('section', { 'aria-label': t('planningFeedback') }, h('h3', null, t('planningFeedback')),
      h('p', { className: css.meta }, t('planningFeedbackHelp')),
      h('textarea', { className: css.planningInput, 'aria-label': t('planningFeedback'), maxLength: 8192, value: feedback, disabled: busy, onChange: event => setFeedback(event.target.value) }),
      h(Button, { disabled: busy || !feedback.trim(), onClick: submitFeedback }, t('planningSendFeedback')),
      receipt && h('p', { role: 'status', 'data-planning-receipt': receiptStatus }, t('planningReceipt') + ': ' + t('planningReceipt_' + receiptStatus) + (receipt['request-id'] ? ' · ' + receipt['request-id'] : '') + (receipt.message ? ' · ' + receipt.message : '')),
      (p.feedback ?? []).length > 0 && h('details', null, h('summary', null, t('planningFeedbackApplied')), ...(p.feedback ?? []).map(item => h('article', { key: item['request-id'] }, h('p', null, item.text), item.target && h(PlanningReference, { value: item.target, t }), h('p', { className: css.meta }, [item.source, item.at].filter(Boolean).join(' · ')))))),
    h('section', { 'aria-label': t('planningPlan') }, h('h3', null, t('planningPlan')), h('p', { className: css.meta }, t('planningPlanHelp')),
      h('label', null, t('planningInstructions'), h('textarea', { className: css.planningInput, 'aria-label': t('planningInstructions'), maxLength: 8192, value: instructions, disabled: busy || control.status.running, onChange: event => setInstructions(event.target.value) })),
      h('p', { className: css.meta }, t('planningInstructionsHelp')),
      h('div', { className: css.actions },
        h('label', null, t('planningScope'), h('select', { className: css.modeSelect, value: scope, disabled: busy || control.status.running, onChange: event => setScope(event.target.value) }, ...['design', 'milestone', 'packet', 'replan'].map(value => h('option', { key: value, value }, t('planningScope_' + value))))),
        ['milestone', 'packet', 'replan'].includes(scope) && h('label', null, t('planningScopeID'), h('input', { className: css.editorInput, value: scopeID, maxLength: 120, disabled: busy || control.status.running, onChange: event => setScopeID(event.target.value) })),
        h(Button, { disabled: busy || control.status.running || !!p['pending-activation'] || (['milestone', 'packet'].includes(scope) && !scopeID.trim()), onClick: submitPlan }, t('planningPlan')))));
}
