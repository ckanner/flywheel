Object.assign(zh, { cfgWorktree: '自动管理任务 Worktree', cfgWorktreeHelp: '默认开启。关闭后不自动创建 Worktree；仍可并行，模型或用户可选择并登记执行目录。已有目录不会被删除' });
Object.assign(en, { cfgWorktree: 'Automatically manage task Worktrees', cfgWorktreeHelp: 'On by default. Off keeps existing directories and still allows parallel tasks; callers may choose and register their own execution checkouts' });
// The editor owns drafts only. File identity, validation, backup and publication stay in Go.
Object.assign(zh, {
  consoleResource: 'Provider',
  cfgRouteDefault: '普通执行',
  cfgRouteFrontend: '前端执行',
  cfgRouteHard: '复杂执行',
  cfgRouteMultimodal: '多模态执行',
  cfgEffortDefault: '后端默认',
  cfgEffortOff: '关闭',
  cfgEffortMinimal: '最低',
  cfgEffortLow: '低',
  cfgEffortMedium: '中',
  cfgEffortHigh: '高',
  cfgEffortXhigh: '更高',
  cfgEffortMax: '最高',
  consoleInvalid: '请输入合法的完整配置 JSON，并保留已有顶层字段',
  cfgStrategy: '工作方式',
  cfgModels: '模型分配',
  cfgLimits: '预算与重试',
  cfgJev: 'JEV 决策',
  cfgMore: '更多配置操作',
  cfgSessionHelp: '同任务、同角色续接；执行与独立 Review 不共享历史',
  cfgScheduleHelp: '仅对契约声明独立、依赖已满足的任务并发执行',
  cfgDispatch: '并行任务派发方式',
  cfgDispatchHelp: '批次处理等待本批 Execute／L1 收尾；就绪队列在执行与验证完成后补充空位。评审和集成仍串行',
  cfgDispatchEligibility: '就绪队列仅适用于 Full 任务，且需要并行、并发数大于 1、自动管理独立 Worktree',
  cfgDispatchBatchState: '当前配置为批次处理',
  cfgDispatchReadyState: '配置条件已满足；运行时仍须核对 Full 任务的独立单元与依赖',
  cfgDispatchUnavailable: '当前不使用就绪队列：{reasons}。保留已选策略',
  cfgDispatchSerial: '调度策略为串行',
  cfgDispatchSingle: '并发上限为 1',
  cfgDispatchUnmanaged: '未开启自动 Worktree 管理',
  cfgDispatchInvalid: '并发数量需要修正',
  cfgReasonSeparator: '；',
  cfgDispatchBatch: '整批处理 · 默认',
  cfgDispatchReady: '就绪队列 · 空位立即补充',
  cfgPrewarm: '提前启动评审运行环境',
  cfgPrewarmHelp: '默认关闭。在串行任务验证期间初始化 DSH 评审 SDK；验证和调用准入通过后才创建或恢复会话、发送 Prompt。并行执行槽位和共享候选批次不预热，其他后端保持冷启动',
  cfgParallelHelp: 'Review 与集成仍串行推进',
  cfgPermissionHelp: '后端不支持的权限会明确拒绝',
  cfgScopeHelp: '优先级：任务 → 项目 → 全局 → 安装默认；选择完整配置文件，不逐字段叠加',
  cfgTemplateSource: '该范围尚无文件，将以安装版本默认配置创建',
  cfgFileSource: '正在编辑此范围的配置文件',
  cfgSaved: '配置已保存，下次启动生效',
  cfgClean: '配置已读取；修改后可保存',
  cfgDirty: '下次启动生效，当前运行保持原配置',
  cfgDiscard: '放弃修改',
  cfgSave: '保存修改',
  cfgReload: '重新读取配置',
  cfgReadError: '配置读取失败，可重新读取；不会创建或修改文件',
  cfgUnsaved: '当前有未保存的修改',
  cfgDiscardHelp: '继续将放弃此范围的草稿；本机文件不会被修改',
  cfgDiscardContinue: '放弃草稿并继续',
  cfgReset: '恢复安装版本默认配置',
  cfgResetHelp: '将完整默认配置载入草稿，保存时先备份原文件。当前任务的配置快照保持不变',
  cfgLoadDefaults: '载入待保存修改',
  cfgResetPending: '待保存：完整替换为安装版本默认配置',
  cfgJson: '高级配置 JSON',
  cfgInvalid: '有无效字段，请修正后保存',
  cfgNumber: '请输入有效的非负数；换算后须为整数',
  cfgNumberRange: '请输入 {min} 到 {max} 之间的数值',
  cfgToggleOn: '开启',
  cfgToggleOff: '关闭',
  cfgNextStart: '运行方式、模型、会话与预算修改在下次启动生效',
  cfgAdvancedTiming: '常规设置在下次启动生效；JEV 总开关的关闭也会阻止已有任务的后续请求与采用',
  cfgStructuredInvalid: '高级配置中有无效字段，请修正后保存',
  cfgFixJSON: '修正高级配置',
  cfgRequiredNumber: '此项需要填写数值',
  cfgMinutes: '分钟',
  cfgSeconds: '秒',
  cfgTimes: '次',
  cfgTasks: '个任务',
  cfgBudgetPolicy: '达到预算时',
  cfgBudgetOff: '不限制',
  cfgBudgetSoft: '仅告警',
  cfgBudgetHard: '阻止新的模型调用',
  cfgBudgetHelp: '缺失用量不当作 0；硬预算会限制并发并核对用量',
  cfgRetryHelp: '仅限传输故障；任务返修次数由任务契约控制',
  cfgTimeout: '单次运行超时',
  cfgTimeoutHelp: '达到时限后请求停止，等待子任务收尾',
  cfgBindingsHelp: '按真实配置编辑 Provider、模型、推理与权限；模型标识在运行时由后端核对',
  cfgEdit: '编辑',
  cfgBinding: '编辑阶段模型',
  cfgApplyBinding: '加入待保存修改',
  cfgBindingAdvanced: '后端与账号渠道',
  cfgModelHint: '使用此 Provider 支持的模型标识；预设不代表当前账号已授权',
  cfgJevHelp: '一个开关控制调查与执行中的 JEV 决策，包括取证、失败后下一步及批准的模型选择。关闭后停止后续 JEV 请求与采用，沿用原路径，也不采集影子快照',
  cfgJevSend: '保存开启配置后，符合条件的决策会发送允许范围内的任务与证据片段，并采用通过校验的选择。预算、验收与权限仍由 Go 执行；不需要逐任务再次开启',
  cfgJevEndpoint: 'MeRouter System One API 地址',
  cfgJevKeyHelp: '仅保存环境变量名，不填写 API Key。运行 Flywheel 的进程需要能读取该变量',
  cfgJevTimeout: '请求超时',
  cfgJevAdvanced: '模型版本与请求参数',
  cfgJevMaxCalls: '每项任务最多 JEV 请求次数',
  cfgJevApplicability: '最低适用概率',
  cfgJevChoice: '最低选择概率',
  cfgJevProbability: '请输入大于 0 且不超过 1 的概率',
  cfgJevData: '允许发送的数据类别',
  cfgJevDataHelp: '可在高级配置 JSON 中调整；类别不包含的数据不会获得发送许可',
  cfgJevNoData: '未授权任何数据类别',
  cfgJevModelInvalid: '请输入固定模型版本，例如 jev-1.14.0，最多 120 个字符',
  cfgJevKeyInvalid: '变量名须以英文字母或下划线开头，只能包含字母、数字和下划线，最多 120 个字符',
  cfgJevEndpointInvalid: '地址最多 2000 个字符',
  cfgJevDraftDiscarded: '已放弃无效的 JEV 输入，保留上次有效值',
  cfgJevChanges: '关闭也会禁止已有任务的后续 JEV 请求与采用；已记录的策略、回执和预算保留。调查创建时冻结策略，开启不会改写此前关闭时创建的调查',
  cfgLocation: '配置文件与来源',
  cfgRawDiff: '查看保存差异',
  cfgModelRole: '阶段',
  cfgProviderID: 'Provider ID',
  cfgConflict: '文件已被外部修改；草稿已保留。重新读取并核对后才能保存',
});
Object.assign(en, {
  consoleResource: 'Provider',
  cfgRouteDefault: 'General execution',
  cfgRouteFrontend: 'Frontend execution',
  cfgRouteHard: 'Complex execution',
  cfgRouteMultimodal: 'Multimodal execution',
  cfgEffortDefault: 'Backend default',
  cfgEffortOff: 'Off',
  cfgEffortMinimal: 'Minimal',
  cfgEffortLow: 'Low',
  cfgEffortMedium: 'Medium',
  cfgEffortHigh: 'High',
  cfgEffortXhigh: 'Extra high',
  cfgEffortMax: 'Maximum',
  consoleInvalid: 'Enter a valid complete configuration JSON object and retain existing top-level fields',
  cfgStrategy: 'Workflow policy',
  cfgModels: 'Model assignments',
  cfgLimits: 'Budget and retries',
  cfgJev: 'JEV decisions',
  cfgMore: 'More configuration actions',
  cfgSessionHelp:
    'Reuse within the same task and role; execution and independent Review do not share history',
  cfgScheduleHelp: 'Parallel execution requires declared independence and satisfied dependencies',
  cfgDispatch: 'Parallel dispatch',
  cfgDispatchHelp: 'Batch waits for every Execute/L1 in the group. Ready queue refills slots after execution and verification. Review and integration stay serial',
  cfgDispatchEligibility: 'Ready queue requires a Full task, parallel scheduling, a limit above 1 and automatically managed independent Worktrees',
  cfgDispatchBatchState: 'Configured for batch processing',
  cfgDispatchReadyState: 'Configuration conditions are met; the run must still have a Full task with eligible independent units and dependencies',
  cfgDispatchUnavailable: 'Ready queue is inactive: {reasons}. The selected policy is retained',
  cfgDispatchSerial: 'scheduling is serial',
  cfgDispatchSingle: 'the concurrency limit is 1',
  cfgDispatchUnmanaged: 'automatic Worktree management is off',
  cfgDispatchInvalid: 'correct the concurrency limit',
  cfgReasonSeparator: '; ',
  cfgDispatchBatch: 'Batch · default',
  cfgDispatchReady: 'Ready queue · refill available lanes',
  cfgPrewarm: 'Prepare the review runtime early',
  cfgPrewarmHelp: 'Off by default. Initialize the DSH review SDK during serial task checks. Create or resume a session and send the prompt only after validation and call admission. Parallel execution slots and shared-candidate batches do not prewarm; other backends start cold',
  cfgParallelHelp: 'Review and integration remain serial',
  cfgPermissionHelp: 'Unsupported backend permissions are rejected explicitly',
  cfgScopeHelp:
    'Priority: task → project → global → installed defaults. A complete file is selected, not a per-field overlay',
  cfgTemplateSource: 'No file exists for this scope; a new file starts from installed defaults',
  cfgFileSource: 'Editing the configuration file for this scope',
  cfgSaved: 'Saved; applies to the next start',
  cfgClean: 'Configuration loaded; edit to save changes',
  cfgDirty: 'Applies to the next start; the current run keeps its snapshot',
  cfgDiscard: 'Discard changes',
  cfgSave: 'Save changes',
  cfgReload: 'Reload configuration',
  cfgReadError: 'Could not read configuration. Retry without creating or changing a file',
  cfgUnsaved: 'There are unsaved changes',
  cfgDiscardHelp: 'Continuing discards this scope’s draft without changing the file',
  cfgDiscardContinue: 'Discard draft and continue',
  cfgReset: 'Restore installed version defaults',
  cfgResetHelp:
    'Load the complete defaults into the draft. Saving backs up the old file; the current run retains its snapshot',
  cfgLoadDefaults: 'Load pending changes',
  cfgResetPending: 'Pending: replace with the complete installed defaults',
  cfgJson: 'Advanced configuration JSON',
  cfgInvalid: 'Correct invalid fields before saving',
  cfgNumber: 'Enter a valid non-negative number that converts to an integer',
  cfgNumberRange: 'Enter a value from {min} to {max}',
  cfgToggleOn: 'On',
  cfgToggleOff: 'Off',
  cfgNextStart: 'Execution, model, session and budget changes apply to the next start',
  cfgAdvancedTiming: 'Regular settings apply to the next start. Turning JEV off also blocks subsequent requests and adoption in existing tasks',
  cfgStructuredInvalid: 'Advanced configuration contains invalid fields. Correct them before saving',
  cfgFixJSON: 'Fix advanced configuration',
  cfgRequiredNumber: 'A numeric value is required',
  cfgMinutes: 'minutes',
  cfgSeconds: 'seconds',
  cfgTimes: 'attempts',
  cfgTasks: 'tasks',
  cfgBudgetPolicy: 'When the budget is reached',
  cfgBudgetOff: 'No limit',
  cfgBudgetSoft: 'Warn only',
  cfgBudgetHard: 'Block new model calls',
  cfgBudgetHelp: 'Missing usage is not zero; hard budgets constrain concurrency and reconcile usage',
  cfgRetryHelp: 'Transport faults only. Repair limits belong to the task contract',
  cfgTimeout: 'Run timeout',
  cfgTimeoutHelp: 'Request cancellation at the limit and wait for child cleanup',
  cfgBindingsHelp:
    'Edit actual Provider, model, reasoning and permissions. The backend checks model identifiers at runtime',
  cfgEdit: 'Edit',
  cfgBinding: 'Edit stage model',
  cfgApplyBinding: 'Add to pending changes',
  cfgBindingAdvanced: 'Backend and account channel',
  cfgModelHint: 'Use a supported Provider model identifier. A preset does not prove account authorization',
  cfgJevHelp:
    'One switch controls JEV decisions during investigation and execution, including evidence collection, next steps after failure and approved model selection. Turning it off stops subsequent requests and adoption, keeps the original paths and disables shadow capture',
  cfgJevSend:
    'Saving an enabled configuration allows eligible decisions to send permitted task and evidence excerpts and adopt validated choices. Go retains budget, acceptance and permission checks; no per-task enable step is needed',
  cfgJevEndpoint: 'MeRouter System One API endpoint',
  cfgJevKeyHelp: 'Store the variable name, not the API key. The Flywheel process must inherit this variable',
  cfgJevTimeout: 'Request timeout',
  cfgJevAdvanced: 'Model version and request options',
  cfgJevMaxCalls: 'Maximum JEV requests per task',
  cfgJevApplicability: 'Minimum applicability probability',
  cfgJevChoice: 'Minimum choice probability',
  cfgJevProbability: 'Enter a probability greater than 0 and at most 1',
  cfgJevData: 'Permitted data categories',
  cfgJevDataHelp: 'Edit in Advanced configuration JSON. Other categories do not gain permission to be sent',
  cfgJevNoData: 'No data categories permitted',
  cfgJevModelInvalid: 'Enter a pinned model version such as jev-1.14.0, up to 120 characters',
  cfgJevKeyInvalid: 'Use up to 120 letters, digits or underscores, starting with a letter or underscore',
  cfgJevEndpointInvalid: 'The endpoint must be at most 2000 characters',
  cfgJevDraftDiscarded: 'Invalid JEV inputs were discarded; the last valid values remain',
  cfgJevChanges: 'Turning it off also blocks further JEV requests and adoption in existing tasks; recorded policies, receipts and budgets remain. Investigations freeze their policy at creation; enabling does not rewrite preparations created while off',
  cfgLocation: 'Configuration file and source',
  cfgRawDiff: 'Review changes',
  cfgModelRole: 'Stage',
  cfgProviderID: 'Provider ID',
  cfgConflict: 'The file changed externally. Your draft is retained; reload and review before saving',
});

const cloneConfig = (value) => JSON.parse(JSON.stringify(value));
// These bounds protect the visible controls; Go remains the save authority.
const configNumberRules = {
  'scheduler.max-parallel': { min: 1, max: 8 },
  'jev.timeout-ms': { min: 1, max: 30000 },
  'jev.max-calls': { min: 1, max: 100 },
  'jev.min-applicability': { min: 0, max: 1, probability: true },
  'jev.min-choice': { min: 0, max: 1, probability: true },
};
function boundedConfigError(path, value) {
  const rule = configNumberRules[path];
  if (!rule) return null;
  if (rule.probability)
    return typeof value !== 'number' || !Number.isFinite(value) || value <= 0 || value > 1 ? 'cfgJevProbability' : null;
  if (!Number.isSafeInteger(value)) return 'cfgNumber';
  return value < rule.min || value > rule.max ? 'cfgNumberRange' : null;
}
function jevTextError(key, value) {
  if (key === 'endpoint') return typeof value !== 'string' || [...value].length > 2000 ? 'cfgJevEndpointInvalid' : null;
  if (key === 'model') return typeof value !== 'string' || value.length > 120 || !/^jev-\d+\.\d+\.\d+$/.test(value) ? 'cfgJevModelInvalid' : null;
  return typeof value !== 'string' || !/^[A-Za-z_][A-Za-z0-9_]{0,119}$/.test(value) ? 'cfgJevKeyInvalid' : null;
}
function configFieldErrors(values) {
  const errors = {};
  for (const path of Object.keys(configNumberRules)) {
    const [group, key] = path.split('.');
    if (record(values?.[group]) && Object.hasOwn(values[group], key)) {
      const error = boundedConfigError(path, values[group][key]);
      if (error) errors[path] = error;
    }
  }
  if (values?.scheduler !== undefined && !record(values.scheduler)) errors.scheduler = 'cfgInvalid';
  if (values?.jev !== undefined) {
    if (!record(values.jev)) errors.jev = 'cfgInvalid';
    else {
      for (const key of Object.keys(values.jev))
        if (key !== '_note' && !key.startsWith('_note_') &&
            !['enabled', 'endpoint', 'model', 'key-env', 'timeout-ms', 'max-calls', 'min-applicability', 'min-choice', 'allowed-data'].includes(key))
          errors['jev.' + key] = 'cfgInvalid';
      for (const key of ['endpoint', 'model', 'key-env']) {
        if (!Object.hasOwn(values.jev, key)) continue;
        const error = jevTextError(key, values.jev[key]);
        if (error) errors['jev.' + key] = error;
      }
      if (Object.hasOwn(values.jev, 'enabled') && typeof values.jev.enabled !== 'boolean') errors['jev.enabled'] = 'cfgInvalid';
      const data = values.jev['allowed-data'];
      if (data !== undefined && (!Array.isArray(data) || new Set(data).size !== data.length ||
          data.some(value => !['task-summary', 'diagnostic-facts', 'logs', 'source', 'diff'].includes(value))))
        errors['jev.allowed-data'] = 'cfgInvalid';
    }
  }
  return errors;
}
function ConfigEditor({ control, selection, run, t, active = false, cache, contextKey, onClose, diagnostics }) {
  // Drafts survive task navigation in this live Session binding only. Restoring
  // one always rereads the file; cached data never grants a revision to save.
  const cached = React.useRef(cache?.get(contextKey));
  const [scope, setScope] = React.useState(cached.current?.scope ?? 'global'),
    [section, setSection] = React.useState(cached.current?.section ?? 'strategy');
  const [draft, setDraft] = React.useState(cached.current?.draft ?? null),
    [base, setBase] = React.useState(cached.current?.base ?? null),
    [raw, setRaw] = React.useState(cached.current?.raw ?? '');
  const [invalid, setInvalid] = React.useState(cached.current?.invalid ?? false),
    [numbers, setNumbers] = React.useState(cached.current?.numbers ?? {}),
    [numberErrors, setNumberErrors] = React.useState(cached.current?.numberErrors ?? {});
  const [texts, setTexts] = React.useState(cached.current?.texts ?? {}),
    [textErrors, setTextErrors] = React.useState(cached.current?.textErrors ?? {}),
    [discardedJevInputs, setDiscardedJevInputs] = React.useState(cached.current?.discardedJevInputs ?? false);
  const [revision, setRevision] = React.useState(cached.current?.revision ?? ''),
    [conflict, setConflict] = React.useState(false);
  const [reset, setReset] = React.useState(cached.current?.reset ?? false),
    [saved, setSaved] = React.useState(false),
    [dialog, setDialog] = React.useState(null),
    [bindingEdit, setBindingEdit] = React.useState(null);
  const attempt = React.useRef(null),
    adopted = React.useRef(null),
    id = React.useId();
  const doc = control.editor,
    busy = control.pending !== null;
  const load = () => {
    cached.current = null;
    adopted.current = null;
    attempt.current = scope;
    void run('config-edit', selection, false, { editor: { scope } });
  };
  React.useEffect(() => {
    if (!active || busy || attempt.current === scope) return;
    attempt.current = scope;
    void run('config-edit', selection, false, { editor: { scope } });
  }, [active, busy, scope, run, selection.taskDir, selection.cwd]);
  React.useEffect(() => {
    if (!doc || doc.scope !== scope || adopted.current === doc) return;
    adopted.current = doc;
    if (cached.current?.dirty) {
      setConflict(cached.current.revision !== doc.revision);
      cached.current = null;
      return;
    }
    cached.current = null;
    setRevision(doc.revision);
    setConflict(false);
    setDraft(cloneConfig(doc.values));
    setBase(cloneConfig(doc.values));
    setRaw(JSON.stringify(doc.values, null, 2));
    setInvalid(false);
    setNumbers({});
    setNumberErrors({});
    setTexts({});
    setTextErrors({});
    setDiscardedJevInputs(false);
    setReset(false);
    setSaved(doc.applied === true);
  }, [doc, scope]);
  const replace = (values) => {
    setDraft(values);
    setRaw(JSON.stringify(values, null, 2));
    setInvalid(false);
    setSaved(false);
  };
  const changed = draft
    ? Object.keys({ ...base, ...draft }).filter(
        (key) => JSON.stringify(draft[key]) !== JSON.stringify(base?.[key]),
      )
    : [];
  const fieldErrors = configFieldErrors(draft);
  const dirty = changed.length > 0 || reset || invalid || Object.values(numberErrors).some(Boolean) || Object.values(textErrors).some(Boolean);
  React.useEffect(() => {
    if (cache && draft)
      cache.set(contextKey, {
        scope,
        section,
        draft,
        base,
        raw,
        invalid,
        numbers,
        numberErrors,
        texts,
        textErrors,
        discardedJevInputs,
        reset,
        revision,
        dirty,
      });
  }, [
    scope,
    section,
    draft,
    base,
    raw,
    invalid,
    numbers,
    numberErrors,
    texts,
    textErrors,
    discardedJevInputs,
    reset,
    revision,
    dirty,
    cache,
    contextKey,
  ]);
  const ready =
    !!draft &&
    doc?.scope === scope &&
    doc.revision === revision &&
    !conflict &&
    !invalid &&
    !Object.values(numberErrors).some(Boolean) &&
    !Object.values(textErrors).some(Boolean) &&
    Object.keys(fieldErrors).length === 0;
  const update = (group, key, value) => replace({ ...draft, [group]: { ...draft[group], [key]: value } });
  const discard = () => {
    const latest = conflict ? doc.values : base;
    replace(cloneConfig(latest));
    if (conflict) {
      setBase(cloneConfig(latest));
      setRevision(doc.revision);
      setConflict(false);
    }
    setNumbers({});
    setNumberErrors({});
    setTexts({});
    setTextErrors({});
    setDiscardedJevInputs(false);
    setReset(false);
  };
  const deferDiscard = (next) => {
    if (dirty) setDialog({ kind: 'discard', next });
    else next();
  };
  const switchScope = (value) => {
    if (scope === value) return;
    deferDiscard(() => {
      setScope(value);
      setDraft(null);
      setBase(null);
      setRaw('');
      setNumbers({});
      setNumberErrors({});
      setTexts({});
      setTextErrors({});
      setDiscardedJevInputs(false);
      setInvalid(false);
      setReset(false);
      setSaved(false);
      setConflict(false);
      setRevision('');
      cached.current = null;
      attempt.current = null;
      adopted.current = null;
    });
  };
  const save = () =>
    void run('config-save', selection, false, {
      editor: {
        scope,
        revision,
        values: reset ? draft : Object.fromEntries(changed.map((key) => [key, draft[key]])),
        reset,
      },
    });
  const clearNumberDrafts = (matches) => {
    const keep = ([path]) => !matches(path);
    setNumbers(old => Object.fromEntries(Object.entries(old).filter(keep)));
    setNumberErrors(old => Object.fromEntries(Object.entries(old).filter(keep)));
  };
  const changeJev = (enabled) => {
    const jevField = path => path.startsWith('jev.');
    const discarded = !enabled && Object.entries({ ...numberErrors, ...textErrors })
      .some(([path, error]) => jevField(path) && error);
    if (!enabled) {
      clearNumberDrafts(jevField);
      setTexts(old => Object.fromEntries(Object.entries(old).filter(([path]) => !jevField(path))));
      setTextErrors(old => Object.fromEntries(Object.entries(old).filter(([path]) => !jevField(path))));
    }
    setDiscardedJevInputs(discarded);
    update('jev', 'enabled', enabled);
  };
  const inputID = (key) => id + '-' + key;
  const field = (key, label, help, controlNode) =>
    h(
      'div',
      { className: css.configField, key },
      h(
        'div',
        { className: css.configFieldLabel },
        h('label', { htmlFor: inputID(key) }, label),
        h('p', { className: css.meta, id: inputID(key) + '-help', hidden: !help }, help || ''),
      ),
      h('div', { className: css.configFieldControl }, controlNode),
    );
  const select = (key, value, choices, onChange) =>
    h(
      'select',
      {
        id: inputID(key),
        className: css.modeSelect,
        value,
        disabled: busy,
        'aria-describedby': inputID(key) + '-help',
        onChange: (e) => onChange(e.target.value),
      },
      ...choices.map(([value, label]) => h('option', { key: value, value }, label)),
    );
  const toggle = (key, checked, onChange, unavailable = false) =>
    h('label', { className: css.configSwitch, htmlFor: inputID(key) },
      h('input', {
        id: inputID(key), type: 'checkbox', role: 'switch', checked, disabled: busy || unavailable,
        'aria-describedby': inputID(key) + '-help',
        onChange: event => onChange(event.target.checked),
      }),
      h('span', { className: css.configSwitchTrack, 'aria-hidden': true }),
      h('span', { className: css.configSwitchState, 'aria-hidden': true }, t(checked ? 'cfgToggleOn' : 'cfgToggleOff')));
  const input = (key, value, onChange, validate) => {
    const error = textErrors[key] || fieldErrors[key];
    return h(React.Fragment, null, h('input', {
      id: inputID(key),
      className: css.editorInput,
      value: texts[key] ?? value ?? '',
      disabled: busy,
      'aria-invalid': !!error,
      'aria-describedby': inputID(key) + '-help' + (error ? ' ' + inputID(key) + '-error' : ''),
      onChange: (e) => {
        const next = e.target.value;
        if (!validate) { onChange(next); return; }
        const invalidText = validate(next);
        setTexts(old => ({ ...old, [key]: next }));
        setTextErrors(old => ({ ...old, [key]: invalidText }));
        setSaved(false);
        setDiscardedJevInputs(false);
        if (!invalidText) onChange(next);
      },
    }), error && h('p', { id: inputID(key) + '-error', role: 'alert', className: css.errorText }, t(error)));
  };
  const number = (group, key, factor, unit, nullable = false, probability = false) => {
    const path = group + '.' + key,
      current = draft[group]?.[key],
      error = numberErrors[path] || fieldErrors[path],
      bounds = configNumberRules[path];
    return h(
      React.Fragment,
      null,
      h(
        'div',
        { className: css.inputUnit },
        h('input', {
          id: inputID(path),
          inputMode: 'decimal',
          value: numbers[path] ?? (current == null ? '' : String(current / factor)),
          disabled: busy,
          'aria-invalid': !!error,
          'aria-describedby': inputID(path) + '-help' + (error ? ' ' + inputID(path) + '-error' : ''),
          onChange: (e) => {
            const value = e.target.value;
            setNumbers((old) => ({ ...old, [path]: value }));
            if (nullable && !value.trim()) {
              setNumberErrors((old) => ({ ...old, [path]: null }));
              update(group, key, null);
              return;
            }
            const scaled = Number(value) * factor,
              rounded = probability ? scaled : Math.round(scaled);
            const invalidNumber = !value.trim()
              ? 'cfgRequiredNumber'
              : probability
                ? !/^\d+(?:\.\d+)?$/.test(value.trim()) || !Number.isFinite(scaled) || scaled <= 0 || scaled > 1
                  ? 'cfgJevProbability'
                  : null
              : !/^\d+(?:\.\d+)?$/.test(value.trim()) ||
                  !Number.isSafeInteger(rounded) ||
                  scaled < 0 ||
                  Math.abs(scaled - rounded) > 0.000001
                ? 'cfgNumber'
                : boundedConfigError(path, rounded);
            setNumberErrors((old) => ({ ...old, [path]: invalidNumber }));
            setSaved(false);
            if (!invalidNumber) update(group, key, rounded);
          },
        }),
        h('span', null, unit),
      ),
      error && h('p', { id: inputID(path) + '-error', role: 'alert', className: css.errorText },
        t(error, bounds ? { min: bounds.min / factor, max: bounds.max / factor } : undefined)),
    );
  };
  const bindingOrder = [
    'loops.plan',
    'routes.default',
    'routes.frontend',
    'routes.hard',
    'routes.multimodal',
    'loops.review',
    'loops.judge',
    'loops.intake',
    'loops.garden',
  ];
  const rows = ['loops', 'routes']
    .flatMap((group) =>
      Object.entries(draft?.[group] ?? {})
        .filter(([, value]) => record(value))
        .map(([key, value]) => [group, key, value]),
    )
    .sort(
      (a, b) =>
        (bindingOrder.indexOf(a[0] + '.' + a[1]) + 1 || 999) -
        (bindingOrder.indexOf(b[0] + '.' + b[1]) + 1 || 999),
    );
  if (record(draft?.['judge-panel'])) rows.push(['panel', 'judge-panel', draft['judge-panel']]);
  const bindingID = (group, key) => group === 'panel' ? 'judge-panel' : group + '.' + key;
  const resourceOf = (value) =>
    value.harness === 'dsh'
      ? ['chatgpt', 'deepseek-account'].includes(value.provider || value.channel)
        ? value.provider || value.channel
        : 'custom'
      : ['kimi', 'codex-pro', 'claude-code-pro'].includes(value.harness)
        ? value.harness
        : 'custom';
  const resourceNames = {
    'deepseek-account': 'DeepSeek Account',
    chatgpt: 'ChatGPT · DSH',
    kimi: 'Kimi Code',
    'codex-pro': 'ChatGPT · Codex Pro',
    'claude-code-pro': 'Claude Code Pro',
    custom: t('consoleCustom'),
  };
  const preset = (resource) =>
    ({
      'deepseek-account': doc.defaults.routes?.default,
      chatgpt: doc.defaults.loops?.[bindingEdit?.key === 'judge' ? 'judge' : 'review'],
      kimi: doc.defaults.loops?.plan,
      'codex-pro': doc.defaults.routes?.hard,
      'claude-code-pro': {
        harness: 'claude-code-pro',
        model: 'sonnet',
        channel: 'claude-subscription',
        thinking: 'high',
      },
    })[resource];
  const roleName = (group, key) =>
    group === 'routes'
      ? {
          default: t('cfgRouteDefault'),
          frontend: t('cfgRouteFrontend'),
          hard: t('cfgRouteHard'),
          multimodal: t('cfgRouteMultimodal'),
        }[key] || t('executeRole') + ' · ' + key
      : t(roleLabels[key] || key);
  const efforts = {
    '': t('cfgEffortDefault'),
    off: t('cfgEffortOff'),
    minimal: t('cfgEffortMinimal'),
    low: t('cfgEffortLow'),
    medium: t('cfgEffortMedium'),
    high: t('cfgEffortHigh'),
    xhigh: t('cfgEffortXhigh'),
    max: t('cfgEffortMax'),
  };
  const openBinding = (group, key, value) => {
    const role = group === 'routes' ? 'execute' : key;
    setBindingEdit({
      group,
      key,
      role,
      value: cloneConfig(value),
      permission: draft.permissions?.bindings?.[bindingID(group, key)] ?? 'inherit',
      session: draft.sessions?.roles?.[role] ?? 'inherit',
    });
  };
  const applySessionRole = (next, role, session) => {
    const roles = { ...next.sessions?.roles };
    if (session === 'inherit') delete roles[role];
    else roles[role] = session;
    next.sessions = { ...next.sessions, roles };
    // Kimi's legacy Plan switch also controls history when mode=fresh.
    // Explicit role choices must not advertise a different actual behavior.
    if (role === 'plan' && session !== 'inherit' && next.loops?.plan) {
      const plan = next.loops.plan;
      if (plan.harness === 'kimi') plan['resident-session'] = session === 'scoped';
      else delete plan['resident-session'];
    }
  };
  const roleSession = (role) => {
    let mode = draft.sessions?.mode ?? 'fresh';
    if (role === 'plan') {
      const plan = draft.loops?.plan;
      if (plan?.['resident-session'] === false) mode = 'fresh';
      else if ((plan?.harness || draft.profiles?.[plan?.profile]?.harness) === 'kimi' && plan['resident-session'] === true) mode = 'scoped';
    }
    return t('cfgInheritMode', { mode: t(mode === 'scoped' ? 'cfgReuse' : 'cfgFresh') });
  };
  const inlineBinding = (group, key, field, value) => {
    const next = cloneConfig(draft);
    if (field === 'session') applySessionRole(next, group === 'routes' ? 'execute' : key, value);
    else if (group === 'panel') next['judge-panel'][field] = value;
    else next[group][key][field] = value;
    replace(next);
  };
  const nativeChoices = value => {
    const inherited = draft.profiles?.[value.profile]?.['native-profile'];
    const harness = value.harness || draft.profiles?.[value.profile]?.harness;
    const choices = [['__inherit__', t('consoleInherit') + (inherited ? ' · ' + inherited : '')], ['', t('cfgNativeNone')],
      ...Object.entries(draft['native-profiles'] ?? {}).filter(([, definition]) => definition.harness === harness).map(([name]) => [name, name])];
    const selected = value['native-profile'];
    if (selected && !choices.some(([name]) => name === selected)) choices.push([selected, selected]);
    return choices;
  };
  const setNativeProfile = (value, name) => {
    const next = { ...value };
    if (name === '__inherit__') delete next['native-profile'];
    else next['native-profile'] = name;
    return next;
  };
  const inlineNativeProfile = (group, key, name) => {
    const next = cloneConfig(draft);
    if (group === 'panel') next['judge-panel'] = setNativeProfile(next['judge-panel'], name);
    else next[group][key] = setNativeProfile(next[group][key], name);
    replace(next);
  };
  const saveBinding = () => {
    const { group, key, role, value, permission, session } = bindingEdit,
      next = cloneConfig(draft);
    if (group === 'panel') next['judge-panel'] = value;
    else next[group][key] = value;
    if (value.channel && !next.channels?.[value.channel])
      next.channels = {
        ...next.channels,
        [value.channel]: doc.defaults.channels?.[value.channel] ?? { 'env-key': null },
      };
    if (permission !== (next.permissions?.bindings?.[bindingID(group, key)] ?? 'inherit')) {
      const bindings = { ...next.permissions?.bindings };
      if (permission === 'inherit') delete bindings[bindingID(group, key)];
      else bindings[bindingID(group, key)] = permission;
      next.permissions = { ...next.permissions, bindings };
    }
    if (session !== (next.sessions?.roles?.[role] ?? 'inherit')) applySessionRole(next, role, session);
    replace(next);
    setBindingEdit(null);
  };
  const patchBinding = (key, value) =>
    setBindingEdit((old) => ({ ...old, value: { ...old.value, [key]: value } }));
  const dispatchReasons = [];
  if ((draft?.scheduler?.mode ?? 'serial') !== 'parallel') dispatchReasons.push(t('cfgDispatchSerial'));
  if (numberErrors['scheduler.max-parallel'] || fieldErrors['scheduler.max-parallel']) dispatchReasons.push(t('cfgDispatchInvalid'));
  else if ((draft?.scheduler?.['max-parallel'] ?? 2) <= 1) dispatchReasons.push(t('cfgDispatchSingle'));
  if (draft?.worktree?.enabled === false) dispatchReasons.push(t('cfgDispatchUnmanaged'));
  const readyQueue = draft?.scheduler?.dispatch === 'ready-queue';
  const dispatchStatus = !readyQueue ? 'batch' : dispatchReasons.length ? 'unavailable' : 'conditional';
  let fields = null;
  if (draft && section === 'strategy')
    fields = h(
      React.Fragment,
      null,
      field(
        'sessions.mode',
        t('consoleSession'),
        t('cfgSessionHelp'),
        select(
          'sessions.mode',
          draft.sessions?.mode ?? 'fresh',
          [
            ['scoped', t('consoleScoped')],
            ['fresh', t('consoleFresh')],
          ],
          (v) => replace({ ...draft, sessions: { ...draft.sessions, mode: v, 'policy-version': 1 } }),
        ),
      ),
      field('worktree.enabled', t('cfgWorktree'), t('cfgWorktreeHelp'),
        toggle('worktree.enabled', draft.worktree?.enabled !== false,
          (enabled) => update('worktree', 'enabled', enabled))),
      field(
        'scheduler.mode',
        t('consoleScheduling'),
        t('cfgScheduleHelp'),
        select(
          'scheduler.mode',
          draft.scheduler?.mode ?? 'serial',
          [
            ['serial', t('consoleSerial')],
            ['parallel', t('consoleParallel')],
          ],
          (v) => {
            if (v === 'serial') clearNumberDrafts(path => path === 'scheduler.max-parallel');
            replace({ ...draft, scheduler: { 'max-parallel': 2, ...draft.scheduler, mode: v } });
          },
        ),
      ),
      draft.scheduler?.mode === 'parallel' &&
        field(
          'scheduler.max-parallel',
          t('consoleConcurrency'),
          t('cfgParallelHelp'),
          number('scheduler', 'max-parallel', 1, t('cfgTasks')),
        ),
      field('scheduler.dispatch', t('cfgDispatch'),
        h(React.Fragment, null, t('cfgDispatchHelp'),
          h('span', { className: css.configHintLine }, t('cfgDispatchEligibility')),
          h('span', { className: css.configHintLine, role: 'status', 'data-dispatch-status': dispatchStatus },
            t(dispatchStatus === 'batch' ? 'cfgDispatchBatchState' : dispatchStatus === 'conditional' ? 'cfgDispatchReadyState' : 'cfgDispatchUnavailable',
              { reasons: dispatchReasons.join(t('cfgReasonSeparator')) }))),
        select('scheduler.dispatch', draft.scheduler?.dispatch ?? 'batch',
          [['batch', t('cfgDispatchBatch')], ['ready-queue', t('cfgDispatchReady')]],
          (v) => update('scheduler', 'dispatch', v))),
      field('scheduler.prewarm', t('cfgPrewarm'), t('cfgPrewarmHelp'),
        toggle('scheduler.prewarm', draft.scheduler?.prewarm === true,
          (enabled) => update('scheduler', 'prewarm', enabled))),
      field(
        'permissions.mode',
        t('permission'),
        t('cfgPermissionHelp'),
        select(
          'permissions.mode',
          draft.permissions?.mode ?? 'role-based',
          [
            ['role-based', t('consoleRolePermissions')],
            ['full-access', t('permissionFull')],
          ],
          (v) => update('permissions', 'mode', v),
        ),
      ),
    );
  if (draft && section === 'limits')
    fields = h(
      React.Fragment,
      null,
      field('budget.enforce', t('cfgBudgetPolicy'),
        t((draft.budget?.enforce ?? 'soft') === 'off' ? 'cfgBudgetOffHelp' : (draft.budget?.enforce ?? 'soft') === 'hard' ? 'cfgBudgetHardHelp' : 'cfgBudgetSoftHelp'),
        select('budget.enforce', draft.budget?.enforce ?? 'soft',
          [['off', t('cfgBudgetOff')], ['soft', t('cfgBudgetSoft')], ['hard', t('cfgBudgetHard')]],
          value => {
            if (value === 'off') {
              clearNumberDrafts(path => path === 'budget.daily-token-limit');
            }
            update('budget', 'enforce', value);
          })),
      draft.budget?.enforce !== 'off' && field('budget.daily-token-limit', t('consoleBudget'),
        t('cfgBudgetDay'), number('budget', 'daily-token-limit', 1, 'Token', true)),
      h('h4', { className: css.settingsSubheading }, t('cfgRunProtection')),
      field(
        'watchdog.wall-timeout-ms',
        t('cfgTimeout'),
        t('cfgTimeoutHelp'),
        number('watchdog', 'wall-timeout-ms', 60000, t('cfgMinutes')),
      ),
      field(
        'retry.max-transport-retries',
        t('consoleRetries'),
        t('cfgRetryHelp'),
        number('retry', 'max-transport-retries', 1, t('cfgTimes')),
      ),
    );
  if (draft && section === 'models')
    fields = h(React.Fragment, null,
      h('p', { className: css.meta }, t('cfgSessionDefault', { mode: t(draft.sessions?.mode === 'scoped' ? 'cfgReuse' : 'cfgFresh') })),
      h('div', { className: css.stageTableScroll }, h('table', { className: css.stageTable, 'aria-label': t('cfgModels') },
        h('thead', null, h('tr', null, ...['cfgModelRole', 'consoleResource', 'consoleModel', 'consoleThinking', 'cfgNative', 'consoleSession'].map(key => h('th', { key, scope: 'col' }, t(key))))),
        h('tbody', null, ...rows.map(([group, key, value]) => {
          const role = group === 'routes' ? 'execute' : key;
          const sessionChoices = [['inherit', roleSession(role)], ['scoped', t('cfgReuse')], ['fresh', t('cfgFresh')]];
          const policy = draft.sessions?.roles?.[role];
          const planConflict = role === 'plan' && policy && (policy === 'scoped' && value['resident-session'] === false || policy === 'fresh' && (value.harness || draft.profiles?.[value.profile]?.harness) === 'kimi' && value['resident-session'] === true);
          return h('tr', { key: bindingID(group, key), 'data-binding-id': bindingID(group, key) },
            h('th', { scope: 'row' }, roleName(group, key)),
            h('td', null, h(Button, { disabled: busy, onClick: () => openBinding(group, key, value), 'aria-label': t('cfgEdit') + ' ' + roleName(group, key) }, resourceNames[resourceOf(value)])),
            h('td', null, h('input', { className: css.editorInput, value: value.model ?? '', placeholder: value.profile || t('cfgEffortDefault'), disabled: busy, 'aria-label': roleName(group, key) + ' · ' + t('consoleModel'), onChange: e => inlineBinding(group, key, 'model', e.target.value) })),
            h('td', null, h('select', { className: css.modeSelect, value: value.thinking ?? '', disabled: busy, 'aria-label': roleName(group, key) + ' · ' + t('consoleThinking'), onChange: e => inlineBinding(group, key, 'thinking', e.target.value) }, ...[...new Set([...Object.keys(efforts), value.thinking ?? ''])].map(effort => h('option', { key: effort, value: effort }, efforts[effort] ?? effort)))),
            h('td', null, h('select', { className: css.modeSelect, disabled: busy, value: value['native-profile'] ?? '__inherit__', 'aria-label': roleName(group, key) + ' · ' + t('cfgNative'), onChange: e => inlineNativeProfile(group, key, e.target.value) }, ...nativeChoices(value).map(([name, label]) => h('option', { key: name, value: name }, label)))),
            h('td', null, ['intake', 'garden'].includes(role) ? t('cfgIndependent') : h('select', { className: css.modeSelect, disabled: busy, value: draft.sessions?.roles?.[role] ?? 'inherit', 'aria-label': roleName(group, key) + ' · ' + t('consoleSession'), onChange: e => inlineBinding(group, key, 'session', e.target.value) }, ...sessionChoices.map(([v, label]) => h('option', { key: v, value: v }, label))), planConflict && h('div', { className: css.settingsHelp }, t('cfgPlanConflict'), h(Button, { disabled: busy, onClick: () => inlineBinding(group, key, 'session', policy) }, t('cfgApplyPolicy')))));
        })))),
      h('p', { className: css.settingsHelp }, t('consoleRouteSession')),
      h('p', { className: css.settingsHelp }, t('cfgNativeHelp')));
  if (draft && section === 'jev')
    fields = h(
      React.Fragment,
      null,
      h('p', { className: css.meta }, t('cfgJevHelp')),
      field(
        'jev.enabled',
        t('consoleJEV'),
        t('cfgJevChanges'),
        toggle('jev.enabled', draft.jev?.enabled === true, changeJev, draft.jev !== undefined && !record(draft.jev)),
      ),
      discardedJevInputs && h('p', { className: css.configCallout, role: 'status' }, t('cfgJevDraftDiscarded')),
      draft.jev?.enabled &&
        h(
          React.Fragment,
          null,
          h('p', { className: css.configCallout }, t('cfgJevSend')),
          field(
            'jev.endpoint',
            t('cfgJevEndpoint'),
            null,
            input('jev.endpoint', draft.jev.endpoint, (v) => update('jev', 'endpoint', v), v => jevTextError('endpoint', v)),
          ),
          field(
            'jev.model',
            t('consoleJEVVersion'),
            null,
            input('jev.model', draft.jev.model ?? 'jev-1.14.0', (v) => update('jev', 'model', v), v => jevTextError('model', v)),
          ),
          field(
            'jev.key-env',
            t('consoleJEVEnv'),
            t('cfgJevKeyHelp'),
            input('jev.key-env', draft.jev['key-env'] ?? 'MEROUTER_API_KEY', (v) =>
              update('jev', 'key-env', v), v => jevTextError('key-env', v),
            ),
          ),
          h(
            'section',
            { className: css.details },
            h('h4', null, t('cfgJevAdvanced')),
            field('jev.timeout-ms', t('cfgJevTimeout'), null, number('jev', 'timeout-ms', 1000, t('cfgSeconds'))),
            field('jev.max-calls', t('cfgJevMaxCalls'), null, number('jev', 'max-calls', 1, t('cfgTimes'))),
            field('jev.min-applicability', t('cfgJevApplicability'), null, number('jev', 'min-applicability', 1, '', false, true)),
            field('jev.min-choice', t('cfgJevChoice'), null, number('jev', 'min-choice', 1, '', false, true)),
            h('p', { className: css.meta }, t('cfgJevData') + ': ' + (Array.isArray(draft.jev['allowed-data']) ? draft.jev['allowed-data'].length ? draft.jev['allowed-data'].join(', ') : t('cfgJevNoData') : t('notConfigured'))),
            h('p', { className: css.meta }, t('cfgJevDataHelp')),
          ),
        ),
    );
  return h(
    'section',
    { className: css.editor, 'aria-label': t('consoleConfig') },
      h(
        'div',
        { className: css.configHeader },
        onClose && h(Button, { onClick: () => deferDiscard(() => { discard(); onClose(); }), disabled: busy }, t('cfgBack')),
        h('h2', { className: css.configTitle }, t('consoleConfig')),
        h(
          'div',
          { className: css.segmented, 'aria-label': t('consoleScope') },
          ...['global', 'project'].map((key) =>
            h(
              Button,
              { key, disabled: busy, 'aria-pressed': scope === key, onClick: () => switchScope(key) },
              t(key === 'global' ? 'consoleGlobal' : 'consoleProject'),
            ),
          ),
        ),
      ),
    h(
      'nav',
      { className: css.configNav, 'aria-label': t('consoleConfig') },
      ...['strategy', 'models', 'limits', 'jev', 'advanced'].map((key) =>
        h(
          Button,
          { key, 'aria-pressed': section === key, onClick: () => setSection(key) },
          t({ strategy: 'cfgStrategy', models: 'cfgModels', limits: 'cfgLimits', jev: 'cfgJev', advanced: 'cfgAdvanced' }[key]),
        ),
      ),
    ),
    h(
      'div',
      { className: css.configMain },
      h(
        'div',
        { className: css.configContent, 'data-section': section },
        h(
          'h3',
          { className: css.configTitle },
          t({ strategy: 'cfgStrategy', models: 'cfgModels', limits: 'cfgLimits', jev: 'cfgJev', advanced: 'cfgAdvanced' }[section]),
        ),
        (!draft || doc?.scope !== scope) &&
          busy &&
          h(
            'div',
            { className: css.configLoading, role: 'status', 'aria-label': t('wbLoading') },
            h(Spinner),
          ),
        !busy &&
          (!draft || control.error?.action === 'config-edit') &&
          attempt.current === scope &&
          h(
            'div',
            { role: 'alert', className: css.configCallout },
            h('p', null, t('cfgReadError')),
            h(Button, { onClick: () => deferDiscard(load) }, t('cfgReload')),
            control.error?.detail &&
              h(
                'section',
                { className: css.details },
                h('h4', null, t('errorDetails')),
                h('pre', { className: css.raw }, control.error.detail),
              ),
          ),
        draft &&
          doc?.scope === scope &&
          h(
            React.Fragment,
            null,
            section !== 'jev' && h('p', { className: css.configTiming }, t(section === 'advanced' ? 'cfgAdvancedTiming' : 'cfgNextStart')),
            section !== 'advanced' && h('fieldset', { className: css.configFields, disabled: invalid }, fields),
            h(
              'section',
              { className: css.configSource, hidden: section !== 'advanced' },
              h('h4', null, t('cfgLocation')),
              h('p', { className: css.meta }, t('cfgScopeHelp')),
              h('p', null, t(doc.source === doc.target ? 'cfgFileSource' : 'cfgTemplateSource')),
              h('code', { className: css.path }, doc.target),
            ),
            h(
              'section',
              { className: css.configSource, hidden: section !== 'advanced' },
              h('h4', null, t('cfgJson')),
              h('textarea', {
                className: css.editorJSON,
                value: raw,
                disabled: busy,
                'aria-label': t('consoleValues'),
                'aria-invalid': invalid || Object.keys(fieldErrors).length > 0,
                onChange: (e) => {
                  setRaw(e.target.value);
                  try {
                    const value = JSON.parse(e.target.value);
                    if (!record(value) || Object.keys(base ?? {}).some((key) => !Object.hasOwn(value, key)))
                      throw new Error();
                    setDraft(value);
                    setInvalid(false);
                    setNumbers({});
                    setNumberErrors({});
                    setTexts({});
                    setTextErrors({});
                    setDiscardedJevInputs(false);
                    setSaved(false);
                  } catch {
                    setInvalid(true);
                  }
                },
              }),
            ),
            section === 'advanced' && (changed.length > 0 || reset) &&
              h(
                'section',
                { className: css.configSource },
                h('h4', null, t('cfgRawDiff')),
                reset && h('p', { className: css.warningText }, t('cfgResetPending')),
                ...changed.map((key) =>
                  h(
                    'div',
                    { key },
                    h('h4', { className: css.heading }, key),
                    h(
                      'pre',
                      { className: css.raw },
                      JSON.stringify({ before: base?.[key], after: draft[key] }, null, 2),
                    ),
                  ),
                ),
              ),
          ),
        section === 'advanced' && h('div', { className: css.actions }, h(Button, { disabled: busy, onClick: () => deferDiscard(load) }, t('cfgReload'))),
        diagnostics && h('div', { hidden: section !== 'advanced' }, diagnostics),
        (Object.values(numberErrors).some(Boolean) || Object.values(textErrors).some(Boolean)) &&
          h('p', { role: 'alert', className: css.errorText }, t('cfgInvalid')),
        (invalid || Object.keys(fieldErrors).length > 0) && h('div', { className: css.configCallout, role: 'alert' },
          h('p', null, t(invalid ? 'consoleInvalid' : 'cfgStructuredInvalid')),
          Object.keys(fieldErrors).length > 0 && h('p', { className: css.meta }, Object.keys(fieldErrors).join(', ')),
          section !== 'advanced' && h(Button, { onClick: () => setSection('advanced') }, t('cfgFixJSON'))),
        (control.configUncertainty || conflict) &&
          h(
            'div',
            { className: css.configCallout, role: 'alert' },
            h('p', null, t(conflict ? 'cfgConflict' : 'consoleReload')),
            h(Button, { disabled: busy, onClick: () => deferDiscard(load) }, t('cfgReload')),
          ),
      ),
    ),
    h('footer', { className: css.configSaveBar, 'data-dirty': dirty },
      h('span', { className: css.meta, role: 'status' }, t(saved ? 'cfgSaved' : dirty ? 'cfgDirty' : 'cfgClean')),
      h('div', { className: css.actions },
        h(Button, { disabled: busy || !doc || doc.scope !== scope, onClick: () => setDialog({ kind: 'reset' }) }, t('cfgReset')),
        h(Button, { disabled: busy || !dirty || !base, onClick: discard }, t('cfgDiscard')),
        h(Button, { variant: 'primary', disabled: !ready || !dirty || busy || control.configUncertainty || !!control.uncertainty, onClick: save }, t('cfgSave')))),
    dialog &&
      h(
        WorkbenchDialog,
        {
          t,
          title: t(
            dialog.kind === 'reset' ? 'cfgReset' : 'cfgUnsaved',
          ),
          onClose: () => setDialog(null),
          actions: h(
            React.Fragment,
            null,
            h(Button, { onClick: () => setDialog(null) }, t('wbCancel')),
            h(
              Button,
              {
                variant: 'primary',
                disabled: busy,
                onClick: () => {
                  const next = dialog;
                  setDialog(null);
                  if (next.kind === 'reset') {
                    replace(cloneConfig(doc.defaults));
                    setNumbers({});
                    setNumberErrors({});
                    setTexts({});
                    setTextErrors({});
                    setDiscardedJevInputs(false);
                    setReset(true);
                  } else next.next();
                },
              },
              t(
                dialog.kind === 'reset' ? 'cfgLoadDefaults' : 'cfgDiscardContinue',
              ),
            ),
          ),
        },
        h(
          'p',
          null,
          t(
            dialog.kind === 'reset' ? 'cfgResetHelp' : 'cfgDiscardHelp',
          ),
        ),
        dialog.kind === 'discard' && h('pre', { className: css.raw }, raw),
      ),
    bindingEdit &&
      h(
        WorkbenchDialog,
        {
          t,
          title: t('cfgBinding') + ' · ' + roleName(bindingEdit.group, bindingEdit.key),
          onClose: () => setBindingEdit(null),
          actions: h(
            React.Fragment,
            null,
            h(Button, { onClick: () => setBindingEdit(null) }, t('wbCancel')),
            h(Button, { variant: 'primary', disabled: busy, onClick: saveBinding }, t('cfgApplyBinding')),
          ),
        },
        field(
          'binding.resource',
          t('consoleResource'),
          null,
          select(
            'binding.resource',
            resourceOf(bindingEdit.value),
            Object.entries(resourceNames),
            (value) => {
              const next = preset(value);
              if (next)
                setBindingEdit((old) => {
                  const value = cloneConfig(next);
                  if (old.group !== 'loops' || old.key !== 'plan') delete value['resident-session'];
                  return { ...old, value };
                });
            },
          ),
        ),
        field(
          'binding.model',
          t('consoleModel'),
          t('cfgModelHint'),
          input('binding.model', bindingEdit.value.model, (v) => patchBinding('model', v)),
        ),
        field(
          'binding.thinking',
          t('consoleThinking'),
          null,
          select(
            'binding.thinking',
            bindingEdit.value.thinking ?? '',
            Object.entries(efforts).concat(
              bindingEdit.value.thinking && !Object.hasOwn(efforts, bindingEdit.value.thinking)
                ? [[bindingEdit.value.thinking, bindingEdit.value.thinking]]
                : [],
            ),
            (v) => {
              if (v) patchBinding('thinking', v);
              else
                setBindingEdit((old) => {
                  const value = { ...old.value };
                  delete value.thinking;
                  return { ...old, value };
                });
            },
          ),
        ),
        field('binding.native-profile', t('cfgNative'), t('cfgNativeHelp'),
          select('binding.native-profile', bindingEdit.value['native-profile'] ?? '__inherit__', nativeChoices(bindingEdit.value),
            name => setBindingEdit(old => ({ ...old, value: setNativeProfile(old.value, name) })))),
        field(
          'binding.permission',
          t('permission'),
          t('consolePermissionEffective'),
          select(
            'binding.permission',
            bindingEdit.permission,
            [
              ['inherit', t('consoleInherit')],
              ['read-only', t('permissionReadOnly')],
              ['workspace-write', t('permissionWorkspace')],
              ['danger-full-access', t('permissionFull')],
            ],
            (v) => setBindingEdit((old) => ({ ...old, permission: v })),
          ),
        ),
        !['intake', 'garden'].includes(bindingEdit.key) &&
          field(
            'binding.session',
            t('consoleSession'),
            bindingEdit.group === 'routes' ? t('consoleRouteSession') : t('consoleScopeReason'),
            select(
              'binding.session',
              bindingEdit.session,
              [
                ['inherit', t('consoleInherit')],
                ['scoped', t('consoleScoped')],
                ['fresh', t('consoleFresh')],
              ],
              (v) => setBindingEdit((old) => ({ ...old, session: v })),
            ),
          ),
        h(
          'section',
          { className: css.details },
          h('h4', null, t('cfgBindingAdvanced')),
          field(
            'binding.harness',
            t('consoleAdapter'),
            null,
            input('binding.harness', bindingEdit.value.harness, (v) => patchBinding('harness', v)),
          ),
          field(
            'binding.provider',
            t('cfgProviderID'),
            null,
            input('binding.provider', bindingEdit.value.provider, (v) => patchBinding('provider', v)),
          ),
          field(
            'binding.channel',
            t('consoleChannel'),
            null,
            input('binding.channel', bindingEdit.value.channel, (v) => patchBinding('channel', v)),
          ),
        ),
      ),
  );
}

Object.assign(zh, {
  consoleConfig: '设置', cfgStrategy: '运行方式', cfgModels: '模型与会话', cfgJev: 'JEV', cfgAdvanced: '高级与诊断', cfgBack: '← 返回流程',
  cfgSessionHelp: '各角色独立，可按阶段覆盖', consoleSession: '会话策略', cfgReuse: '按角色复用', cfgFresh: '每次新建', cfgInheritMode: '继承 · {mode}', cfgSessionDefault: '默认：{mode}', cfgIndependent: '独立调用',
  cfgScheduleHelp: '', cfgParallelHelp: '并发数为 1–8；Review 与集成仍串行', cfgPermissionHelp: '完全权限可访问工作区以外文件',
  cfgBudgetPolicy: '用量预算', cfgBudgetOff: '关闭', cfgBudgetSoft: '仅提醒', cfgBudgetHard: '达到后停止',
  cfgBudgetOffHelp: '不设 Token 门限，继续统计用量', cfgBudgetSoftHelp: '达到额度时提醒，任务继续运行', cfgBudgetHardHelp: '达到额度或用量无法确认时，暂停新调用', cfgBudgetDay: '按单个任务、UTC 日期统计；留空不设额度', consoleBudget: '单任务每日 Token 额度',
  cfgRunProtection: '运行保护', cfgRetryHelp: '传输故障重试，不影响任务返修次数', cfgTimeoutHelp: '', cfgTimeout: '单次运行时限',
  cfgClean: '没有待保存修改', cfgDirty: '有未保存修改', cfgSaved: '配置已保存', cfgReset: '恢复默认', cfgDiscard: '撤销',
});
Object.assign(en, {
  consoleConfig: 'Settings', cfgStrategy: 'Execution', cfgModels: 'Models and sessions', cfgJev: 'JEV', cfgAdvanced: 'Advanced and diagnostics', cfgBack: '← Back to workflow',
  cfgSessionHelp: 'Isolated roles; stages can override the default', cfgReuse: 'Reuse within role', cfgFresh: 'New each time', cfgInheritMode: 'Inherit · {mode}', cfgSessionDefault: 'Default: {mode}', cfgIndependent: 'Independent call',
  cfgScheduleHelp: '', cfgParallelHelp: 'Limit: 1–8. Review and integration remain serial', cfgPermissionHelp: 'Full access includes files outside the workspace',
  cfgBudgetPolicy: 'Usage budget', cfgBudgetOff: 'Off', cfgBudgetSoft: 'Warn only', cfgBudgetHard: 'Stop at limit', cfgBudgetOffHelp: 'No token limit; usage is still recorded', cfgBudgetSoftHelp: 'Warn at the limit and keep running', cfgBudgetHardHelp: 'Pause new calls at the limit or when usage is unknown', cfgBudgetDay: 'Per task and UTC day; blank means no limit', consoleBudget: 'Daily tokens per task',
  cfgRunProtection: 'Run protection', cfgRetryHelp: 'Transport faults only; task repair limits stay unchanged', cfgTimeoutHelp: '', cfgTimeout: 'Run time limit',
  cfgClean: 'No pending changes', cfgDirty: 'Unsaved changes', cfgSaved: 'Configuration saved', cfgReset: 'Restore defaults', cfgDiscard: 'Discard',
});

Object.assign(zh, { cfgPlanConflict: 'Plan 旧历史开关与此策略不同', cfgApplyPolicy: '同步此策略' });
Object.assign(en, { cfgPlanConflict: 'The legacy Plan history switch differs', cfgApplyPolicy: 'Align policy' });

Object.assign(zh, {
  cfgNative: '原生能力', cfgNativeNone: '不启用原生能力配置',
  cfgNativeHelp: '选择与当前后端匹配的能力配置；在高级 JSON 中编辑 native-profiles。JEV 关闭时仍生效，实际可用性以运行回执为准。',
});
Object.assign(en, {
  cfgNative: 'Native capabilities', cfgNativeNone: 'No native capability profile',
  cfgNativeHelp: 'Select a profile for this backend; edit native-profiles in advanced JSON. Profiles work with JEV off. Runtime receipts establish actual availability.',
});
