Object.assign(zh, { cfgWorktree: '自动管理任务 Worktree', cfgWorktreeHelp: '关闭后仍可并行，任务自行选择执行目录；保留已有目录' });
Object.assign(en, { cfgWorktree: 'Automatically manage task Worktrees', cfgWorktreeHelp: 'Off still allows parallel work and caller-selected execution directories; existing directories are kept' });
// The editor owns drafts only. File identity, validation, backup and publication stay in Go.
Object.assign(zh, {
  consoleResource: 'Provider',
  cfgRouteDefault: '普通执行',
  cfgRouteFrontend: '前端执行',
  cfgRouteHard: '复杂执行',
  cfgRouteMultimodal: '多模态执行',
  cfgEffortDefault: '后端默认',
  cfgEffortFixed: '原生固定',
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
  cfgDispatchHelp: '整批处理等待本批完成；就绪队列在 Execute／L1 完成后补充空位',
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
  cfgLoadDefaults: '载入默认配置',
  cfgResetPending: '待保存：完整替换为安装版本默认配置',
  cfgJson: '高级配置 JSON',
  cfgInvalid: '有无效字段，请修正后保存',
  cfgNumber: '请输入有效的非负数；换算后须为整数',
  cfgNumberRange: '请输入 {min} 到 {max} 之间的数值',
  cfgToggleOn: '开启',
  cfgToggleOff: '关闭',
  cfgNextStart: '运行方式、模型、会话与预算修改在下次启动生效',
  cfgAdvancedTiming: '常规设置在下次启动生效；JEV 后续请求与采用会核对当前任务实际选中的配置文件',
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
  cfgBindingDone: '返回模型列表',
  cfgBindingAdvanced: '后端与账号渠道',
  cfgModelHint: '使用此 Provider 支持的模型标识；预设不代表当前账号已授权',
  cfgJevHelp: '一个开关控制调查与执行中的 JEV 决策，包括取证、首轮执行选择及失败后的下一步。任务采用哪个配置文件，由任务 → 项目 → 全局 → 安装默认的优先级决定',
  cfgJevSend: '保存开启配置后，符合条件的决策会发送允许范围内的任务与证据片段，并采用通过校验的选择。预算、验收与权限仍由 Go 执行；不需要逐任务再次开启',
  cfgJevEndpoint: 'JEV API 地址',
  cfgJevEndpointHelp: '填写兼容 System One 协议的完整接口地址，以 /v1/systemone 结尾。',
  cfgJevModelHelp: '默认已填写，可修改为服务支持的固定版本。',
  cfgJevKeyHelp: '仅保存环境变量名，不填写 API Key。运行 Flywheel 的进程需要能读取该变量',
  cfgJevTimeout: '请求超时',
  cfgJevAdvanced: '模型版本与请求参数',
  cfgJevMaxCalls: '每项任务最多 JEV 请求次数',
  cfgJevApplicability: '最低适用概率',
  cfgJevChoice: '最低选择概率',
  cfgJevProbability: '请输入大于 0 且不超过 1 的概率',
  cfgJevData: '允许发送的数据类别',
  cfgJevDataHelp: '仅发送已勾选类别中符合范围与预算的事实；勾选不会扩大工具或执行权限',
  cfgJevNoData: '未授权任何数据类别',
  cfgJevModelInvalid: '请输入固定模型版本，例如 jev-1.14.0，最多 120 个字符',
  cfgJevKeyInvalid: '变量名须以英文字母或下划线开头，只能包含字母、数字和下划线，最多 120 个字符',
  cfgJevEndpointInvalid: '地址最多 2000 个字符',
  cfgJevDraftDiscarded: '已放弃无效的 JEV 输入，保留上次有效值',
  cfgJevChanges: '修改并保存当前范围的开关。只有当前任务实际采用的配置关闭，才会阻止其后续 JEV 请求与采用；全局修改不会覆盖项目或任务文件。已记录的策略、回执和预算保留',
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
  cfgEffortFixed: 'Native fixed',
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
  cfgDispatchHelp: 'Batch waits for the group; Ready queue refills slots after Execute/L1',
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
  cfgLoadDefaults: 'Load defaults',
  cfgResetPending: 'Pending: replace with the complete installed defaults',
  cfgJson: 'Advanced configuration JSON',
  cfgInvalid: 'Correct invalid fields before saving',
  cfgNumber: 'Enter a valid non-negative number that converts to an integer',
  cfgNumberRange: 'Enter a value from {min} to {max}',
  cfgToggleOn: 'On',
  cfgToggleOff: 'Off',
  cfgNextStart: 'Execution, model, session and budget changes apply to the next start',
  cfgAdvancedTiming: 'Regular settings apply on the next launch. Subsequent JEV requests and adoption check the configuration file selected for the current task',
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
  cfgBindingDone: 'Back to model list',
  cfgBindingAdvanced: 'Backend and account channel',
  cfgModelHint: 'Use a supported Provider model identifier. A preset does not prove account authorization',
  cfgJevHelp:
    'One switch controls JEV decisions during investigation and execution, including evidence collection, initial execution selection and next steps after failure. The selected file follows task → project → global → installed defaults precedence',
  cfgJevSend:
    'Saving an enabled configuration allows eligible decisions to send permitted task and evidence excerpts and adopt validated choices. Go retains budget, acceptance and permission checks; no per-task enable step is needed',
  cfgJevEndpoint: 'JEV API URL',
  cfgJevEndpointHelp: 'Enter a full System One-compatible endpoint URL ending in /v1/systemone.',
  cfgJevModelHelp: 'A default is provided; you can edit it to a pinned version supported by your service.',
  cfgJevKeyHelp: 'Store the variable name, not the API key. The Flywheel process must inherit this variable',
  cfgJevTimeout: 'Request timeout',
  cfgJevAdvanced: 'Model version and request options',
  cfgJevMaxCalls: 'Maximum JEV requests per task',
  cfgJevApplicability: 'Minimum applicability probability',
  cfgJevChoice: 'Minimum choice probability',
  cfgJevProbability: 'Enter a probability greater than 0 and at most 1',
  cfgJevData: 'Permitted data categories',
  cfgJevDataHelp: 'Only selected data categories within the approved scope and budget may be sent. Selecting a category does not grant tools or execution permission',
  cfgJevNoData: 'No data categories permitted',
  cfgJevModelInvalid: 'Enter a pinned model version such as jev-1.14.0, up to 120 characters',
  cfgJevKeyInvalid: 'Use up to 120 letters, digits or underscores, starting with a letter or underscore',
  cfgJevEndpointInvalid: 'The endpoint must be at most 2000 characters',
  cfgJevDraftDiscarded: 'Invalid JEV inputs were discarded; the last valid values remain',
  cfgJevChanges: 'Edit and save the switch in this scope. Subsequent JEV requests and adoption stop only when the file actually selected for the task is off. A global edit does not override project or task files. Recorded policies, receipts and budgets remain',
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
  if (values?.['native-profiles'] !== undefined) {
    if (!record(values['native-profiles'])) errors['native-profiles'] = 'cfgInvalid';
    else for (const [name, definition] of Object.entries(values['native-profiles']))
      if (!record(definition)) errors['native-profiles.' + name] = 'cfgInvalid';
  }
  if (values?.jev !== undefined) {
    if (!record(values.jev)) errors.jev = 'cfgInvalid';
    else {
      for (const key of Object.keys(values.jev))
        if (key !== '_note' && !key.startsWith('_note_') &&
            !['enabled', 'backend', 'profile', 'endpoint', 'model', 'key-env', 'timeout-ms', 'max-calls', 'min-applicability', 'min-choice', 'allowed-data'].includes(key))
          errors['jev.' + key] = 'cfgInvalid';
      for (const key of ['endpoint', 'model', 'key-env']) {
        if (values.jev.backend === 'subscription' || !Object.hasOwn(values.jev, key)) continue;
        const error = jevTextError(key, values.jev[key]);
        if (error) errors['jev.' + key] = error;
      }
      if (Object.hasOwn(values.jev, 'enabled') && typeof values.jev.enabled !== 'boolean') errors['jev.enabled'] = 'cfgInvalid';
      const data = values.jev['allowed-data'];
      if (data !== undefined && (!Array.isArray(data) || new Set(data).size !== data.length ||
          data.some(value => !['task-summary', 'diagnostic-facts', 'logs', 'source', 'diff'].includes(value))))
        errors['jev.allowed-data'] = 'cfgInvalid';
      if (values.jev.enabled === true && values.jev.backend === 'subscription') {
        const profile = values.profiles?.[values.jev.profile];
        if (!record(profile) || profile.harness !== 'antigravity' || profile.provider || profile['native-profile'] || fixedAntigravityEffort(profile))
          errors['jev.profile'] = 'cfgJevUnresolved';
      }
    }
  }
  for (const [role, binding] of Object.entries(values?.loops ?? {}))
    if (antigravitySwarmUnavailable(values, binding, role)) errors['loops.' + role + '.native-profile'] = 'cfgSwarmRole';
  if (antigravitySwarmUnavailable(values, values?.['judge-panel'], 'judge-panel')) errors['judge-panel.native-profile'] = 'cfgSwarmRole';
  for (const [role, profile] of Object.entries(values?.planning?.compact ?? {}))
    if (antigravitySwarmUnavailable(values, values?.profiles?.[profile], role)) errors['planning.compact.' + role] = 'cfgSwarmRole';
  for (const [route, policy] of Object.entries(values?.['execution-policy']?.routes ?? {})) {
    if (!record(policy)) { errors['execution-policy.routes.' + route] = 'cfgExecutionAdvanced'; continue; }
    if (!Number.isSafeInteger(policy['max-switches']) || policy['max-switches'] < 0 || policy['max-switches'] > 32)
      errors['execution-policy.routes.' + route + '.max-switches'] = 'cfgExecutionSwitchRange';
  }
  return errors;
}
function fixedAntigravityEffort(binding) {
  return binding?.harness === 'antigravity' && ['gemini-pro-orchestrator-neon', 'gemini-pro-orchestrator', 'gemini-pro-orchestrator-argon'].includes(binding.model);
}

function resolvedEditorBinding(values, binding) {
  if (!record(binding)) return null;
  return { ...values?.profiles?.[binding.profile], ...binding };
}

function antigravitySwarmUnavailable(values, binding, role) {
  const resolved = resolvedEditorBinding(values, binding);
  return role !== 'execute' && resolved?.harness === 'antigravity' &&
    values?.['native-profiles']?.[resolved['native-profile']]?.options?.mode === 'swarm';
}

function completeExecutionBinding(binding) {
  return record(binding) && ['dsh', 'kimi', 'codex-pro', 'claude-code-pro', 'antigravity'].includes(binding.harness) &&
    ['channel', 'model'].every(key => typeof binding[key] === 'string' && binding[key].trim()) &&
    typeof binding.provider === 'string' && typeof binding.thinking === 'string' &&
    (binding.thinking !== '' || fixedAntigravityEffort(binding)) &&
    ['read-only', 'workspace-write', 'danger-full-access'].includes(binding.permission) &&
    ['fresh', 'scoped'].includes(binding['session-mode']);
}

// Keep profiles shared by other roles/candidates immutable. A dedicated profile
// may be edited again only while every reference belongs to this same target.
function assignAntigravityOptions(values, group, key, binding, options) {
  const current = resolvedEditorBinding(values, binding)?.['native-profile'];
  const profiles = values['native-profiles'] ?? {};
  const prefix = ('ui-' + group + '-' + key + '-native').slice(0, 55);
  const initialID = group === 'routes' ? values['execution-policy']?.routes?.[key]?.initial : null;
  const sharedInitial = initialID && Object.entries(values['execution-policy']?.routes ?? {}).some(([name, route]) => name !== key && route.allowed?.includes(initialID));
  const references = [
    ...Object.entries(values.loops ?? {}).map(([name, value]) => ['loops.' + name, value]),
    ...Object.entries(values.routes ?? {}).map(([name, value]) => ['routes.' + name, value]),
    ...Object.entries(values.profiles ?? {}).map(([name, value]) => ['profiles.' + name, value]),
    ...Object.entries(values['execution-policy']?.bindings ?? {}).map(([name, value]) => ['candidate.' + name, value]),
    ['panel.judge-panel', values['judge-panel']],
  ];
  const own = new Set([group + '.' + key, ...(initialID && !sharedInitial ? ['candidate.' + initialID] : [])]);
  const shared = references.some(([location, value]) => !own.has(location) && value?.['native-profile'] === current);
  let name = current && (current === prefix || current.startsWith(prefix + '-')) && !shared ? current : prefix;
  if (name !== current || shared) {
    let suffix = 1;
    while (Object.hasOwn(profiles, name)) name = prefix + '-' + ++suffix;
  }
  values['native-profiles'] = { ...profiles, [name]: { harness: 'antigravity', options } };
  return { ...binding, 'native-profile': name };
}

function executionInitial(values, route) {
  const policy = values?.['execution-policy'];
  const binding = policy?.bindings?.[policy?.routes?.[route]?.initial];
  return record(binding) ? binding : null;
}

// A graphical route edit must change the actual initial executor, without
// redefining a shared candidate or granting it any additional permission.
function synchronizeExecutionInitial(values, route, binding) {
  const policy = values['execution-policy'], selection = policy?.routes?.[route];
  if (!selection) return true;
  const previous = executionInitial(values, route), old = selection.initial;
  if (!previous || !Array.isArray(selection.allowed) || !selection.allowed.includes(old) || !Array.isArray(selection['quality-order']) ||
      !['harness', 'channel', 'model'].every(key => typeof binding[key] === 'string' && binding[key].trim()) ||
      typeof binding.thinking !== 'string' || (!binding.thinking && !fixedAntigravityEffort(binding)) ||
      !['read-only', 'workspace-write', 'danger-full-access'].includes(previous.permission) || !['fresh', 'scoped'].includes(previous['session-mode'])) return false;
  const prefix = 'route-' + route + '-initial';
  let candidate = prefix, suffix = 1;
  const shared = id => Object.entries(policy.routes).some(([name, row]) => name !== route && row.allowed?.includes(id));
  while (policy.bindings[candidate] && (candidate !== old || shared(candidate))) candidate = prefix + '-' + ++suffix;
  if (!policy.bindings[candidate] && Object.keys(policy.bindings).length >= 32) return false;
  const next = { ...previous, harness: binding.harness, provider: binding.provider ?? '', channel: binding.channel,
    model: binding.model, thinking: binding.thinking };
  if (binding['native-profile']) next['native-profile'] = binding['native-profile'];
  else delete next['native-profile'];
  policy.bindings[candidate] = next;
  selection.initial = candidate;
  selection.allowed = selection.allowed.map(id => id === old ? candidate : id);
  selection['quality-order'] = selection['quality-order'].map(id => id === old ? candidate : id);
  if (selection['channel-fallbacks']) selection['channel-fallbacks'] = Object.fromEntries(Object.entries(selection['channel-fallbacks']).map(([from, to]) => [from === old ? candidate : from, to === old ? candidate : to]));
  return true;
}

function ConfigEditor({ control, selection, run, t, active = false, cache, contextKey, onClose, diagnostics }) {
  // Drafts survive task and view navigation in this live Session binding only.
  // A dirty cached draft is reused after remount; the engine's revision check
  // still prevents a stale draft from being saved as if it were current.
  const cached = React.useRef(cache?.get(contextKey));
  const initialScope = React.useRef(cached.current?.scope ?? 'global');
  const hadCachedDraft = React.useRef(cached.current?.dirty === true);

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
  // The baseline document identifies a confirmed save received while hidden.
  // An older applied document must not overwrite edits made after that save.
  const [baseDocument, setBaseDocument] = React.useState(cached.current?.document ?? null);
  const [reset, setReset] = React.useState(cached.current?.reset ?? false),
    [saved, setSaved] = React.useState(false),
    [dialog, setDialog] = React.useState(null),
    [bindingSelection, setBindingSelection] = React.useState(null);
  const [effectiveRead, setEffectiveRead] = React.useState({ status: 'unknown', value: null });
  const effectiveAttempt = React.useRef(null), effectiveRequest = React.useRef(0), editorMounted = React.useRef(true);
  const observedConfiguration = React.useRef(control.configuration);
  const currentEditor = React.useRef(null);
  // Preserve an attempted replacement read while the accepted draft is empty.
  // A clean loaded document still receives a fresh read on a later remount.
  const attempt = React.useRef(cached.current?.draft ? null : cached.current?.readAttempt ?? null),
    adopted = React.useRef(null),
    id = React.useId();
  const doc = control.editor,
    busy = control.pending !== null;
  currentEditor.current = { contextKey, scope, doc };
  React.useEffect(() => {
    editorMounted.current = true;
    return () => { editorMounted.current = false; };
  }, []);
  // Explicit configuration/doctor reads in Diagnostics use the same task-bound
  // controller. Adopt each new result without reviving the pre-save snapshot
  // when an unrelated action changes the controller's pending/error state.
  React.useEffect(() => {
    const changed = control.configuration !== observedConfiguration.current;
    observedConfiguration.current = control.configuration;
    if (control.configUncertainty || ['config', 'doctor'].includes(control.error?.action)) {
      setEffectiveRead({ status: 'unknown', value: null });
    } else if (changed) {
      setEffectiveRead(control.configuration
        ? { status: 'confirmed', value: control.configuration }
        : { status: 'unknown', value: null });
    }
  }, [control.configuration, control.error, control.configUncertainty]);
  const refreshEffective = () => {
    if (busy || !active || !doc || doc.scope !== scope) return;
    effectiveAttempt.current = doc;
    const request = ++effectiveRequest.current, target = currentEditor.current;
    setEffectiveRead({ status: 'loading', value: null });
    void run('config', selection).then(value => {
      const current = currentEditor.current;
      if (!editorMounted.current || request !== effectiveRequest.current || current.contextKey !== target.contextKey ||
          current.scope !== target.scope || current.doc !== target.doc) return;
      setEffectiveRead(value?.configuration
        ? { status: 'confirmed', value: value.configuration }
        : { status: 'unknown', value: null });
    });
  };
  const clearDraft = (nextScope = scope) => {
    // Persist the accepted discard and destination before an asynchronous read
    // can outlive this component; a missing draft is also meaningful state.
    cache?.set(contextKey, { scope: nextScope, section, dirty: false, document: doc });
    setDraft(null);
    setBase(null);
    setBaseDocument(doc);
    setRaw('');
    setInvalid(false);
    setNumbers({});
    setNumberErrors({});
    setTexts({});
    setTextErrors({});
    setDiscardedJevInputs(false);
    setReset(false);
    setSaved(false);
    setConflict(false);
    setRevision('');
    cached.current = null;
    adopted.current = null;
    hadCachedDraft.current = false;
  };
  const read = () => {
    attempt.current = scope;
    const entry = cache?.get(contextKey);
    if (entry) cache.set(contextKey, { ...entry, readAttempt: scope });
    void run('config-edit', selection, false, { editor: { scope } });
  };
  const load = () => {
    clearDraft();
    read();
  };
  React.useEffect(() => {
    // A task switch can invalidate an in-flight read. Only keep its cached
    // attempt when the controller still has the request, its reply, or error.
    if (cached.current && !cached.current.draft && cached.current.readAttempt === scope && !busy &&
        control.error?.action !== 'config-edit' && (doc?.scope !== scope || doc === cached.current.document))
      attempt.current = null;
    if (!active || busy || attempt.current === scope) return;
    // A remounted view starts with a dirty binding-scoped draft. Do not replace
    // it with a fresh read behind the user's back; the engine's revision check
    // still rejects a save if the file changed while the view was hidden.
    if (hadCachedDraft.current && scope === initialScope.current && doc?.scope === scope) {
      hadCachedDraft.current = false;
      attempt.current = scope;
      return;
    }
    read();
  }, [active, busy, scope, run, selection.taskDir, selection.cwd, doc, control.error]);
  React.useEffect(() => {
    if (!doc || doc.scope !== scope || adopted.current === doc) return;
    // A discarded document is not a reply to the replacement read. It may
    // remain in the controller while that read is pending or has failed.
    if (cached.current && !cached.current.draft && cached.current.document === doc) return;
    adopted.current = doc;
    const confirmedHiddenSave = doc.applied === true && cached.current?.document !== doc;
    if (cached.current?.dirty && !confirmedHiddenSave) {
      setConflict(cached.current.revision !== doc.revision);
      cached.current = null;
      return;
    }
    cached.current = null;
    setRevision(doc.revision);
    setConflict(false);
    setDraft(cloneConfig(doc.values));
    setBase(cloneConfig(doc.values));
    setBaseDocument(doc);
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
  // A confirmed editor read/save is separate from the effective task config.
  // Follow it through the existing read queue once; never retry an unknown save
  // or use an old global draft as evidence of the task's current JEV switch.
  React.useEffect(() => {
    if (!active || busy || control.configUncertainty || !draft || !doc || adopted.current !== doc || doc.scope !== scope || effectiveAttempt.current === doc) return;
    refreshEffective();
  }, [active, busy, doc, !!draft, scope, run, selection.taskDir, selection.cwd, control.configUncertainty]);
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
    if (cache)
      cache.set(contextKey, {
        scope,
        section,
        draft,
        base,
        document: baseDocument,
        readAttempt: attempt.current,
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
    baseDocument,
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
      setBaseDocument(doc);
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
      clearDraft(value);
      attempt.current = null;
    });
  };
  const save = () => {
    // The previous inspection predates this write. Until a confirmed save is
    // followed by a new read, it cannot establish the current effective switch.
    setEffectiveRead({ status: 'unknown', value: null });
    const values = cloneConfig(draft);
    // Register only the final channel of an edited binding. Typing a channel
    // must not leave a configuration entry for every partial input string.
    for (const [group, key, value] of rows) {
      if (group !== 'panel' && !Object.hasOwn(draft[group] ?? {}, key)) continue;
      const previous = group === 'panel' ? base?.['judge-panel'] : base?.[group]?.[key];
      if (JSON.stringify(value) !== JSON.stringify(previous) && value.channel && !values.channels?.[value.channel]) {
        const fallback = { 'env-key': null };
        values.channels = { ...values.channels, [value.channel]: doc.defaults.channels?.[value.channel] ?? fallback };
      }
    }
    if (values.subscription?.mode === 'subscription-only' && rows.some(([, , value]) => value['native-profile'] === 'kimi-subscription-agent') && !values['native-profiles']?.['kimi-subscription-agent']) {
      values['native-profiles'] = { ...values['native-profiles'], 'kimi-subscription-agent': { harness:'kimi',options:{version:1,mode:'agent',agents:{enabled:false}} } };
    }
    const sections = Object.keys({ ...base, ...values }).filter(key => JSON.stringify(values[key]) !== JSON.stringify(base?.[key]));
    void run('config-save', selection, false, {
      editor: { scope, revision, values: reset ? values : Object.fromEntries(sections.map(key => [key, values[key]])), reset },
    });
  };
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
      ...choices.map(([value, label, disabled]) => h('option', { key: value, value, disabled: disabled === true }, label)),
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
  const boundedInteger = (path, current, min, max, onChange, optional = false) => {
    const error = numberErrors[path] || fieldErrors[path];
    return h(React.Fragment, null, h('input', {
      id: inputID(path), className: css.editorInput, inputMode: 'numeric', disabled: busy,
      value: numbers[path] ?? (current == null ? '' : String(current)),
      'aria-invalid': !!error,
      'aria-describedby': inputID(path) + '-help' + (error ? ' ' + inputID(path) + '-error' : ''),
      onChange: event => {
        const text = event.target.value, value = Number(text);
        const invalidNumber = optional && !text.trim() ? null
          : !/^\d+$/.test(text.trim()) || !Number.isSafeInteger(value) ? 'cfgNumber'
          : value < min || value > max ? 'cfgNumberRange' : null;
        setNumbers(old => ({ ...old, [path]: text }));
        setNumberErrors(old => ({ ...old, [path]: invalidNumber }));
        setSaved(false);
        if (!invalidNumber) onChange(optional && !text.trim() ? undefined : value);
      },
    }), error && h('p', { id: inputID(path) + '-error', role: 'alert', className: css.errorText }, t(error, { min, max })));
  };
  const inheritedPlanningRoles = { 'design-plan': 'plan', 'design-review': 'review', 'plan-review': 'review' };
  const inheritedPlanningBinding = key => {
    const source = draft?.loops?.[inheritedPlanningRoles[key]];
    if (!record(source)) return undefined;
    const { 'resident-session': _legacyPlanHistory, ...binding } = source;
    return binding;
  };
  const effectiveBinding = (group, key) => {
    const initial = group === 'routes' && executionInitial(draft, key);
    if (initial) return Object.fromEntries(['harness', 'provider', 'channel', 'model', 'thinking', 'native-profile'].filter(field => Object.hasOwn(initial, field)).map(field => [field, initial[field]]));
    return draft?.[group]?.[key] ?? (group === 'loops' ? inheritedPlanningBinding(key) : undefined);
  };
  const bindingOrder = [
    'loops.design-plan', 'loops.design-review', 'loops.plan-review',
    'loops.plan',
    'routes.default', 'routes.complex', 'routes.backend', 'routes.batch',
    'routes.frontend',
    'routes.hard',
    'routes.multimodal',
    'loops.review',
    'loops.judge',
    'loops.intake',
    'loops.garden', 'loops.investigate',
  ];
  const rows = ['loops', 'routes']
    .flatMap((group) =>
      Object.entries(group === 'loops' ? { ...Object.fromEntries(Object.entries(inheritedPlanningRoles).filter(([, source]) => record(draft?.loops?.[source])).map(([role]) => [role, inheritedPlanningBinding(role)])), ...draft?.loops } : draft?.[group] ?? {})
        .filter(([, value]) => record(value))
        .map(([key, value]) => [group, key, group === 'routes' ? effectiveBinding(group, key) : value]),
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
      : ['kimi', 'codex-pro', 'claude-code-pro', 'antigravity'].includes(value.harness)
        ? value.harness
        : 'custom';
  const resourceNames = {
    ...(draft?.subscription?.mode === 'subscription-only' ? {} : {
      'deepseek-account': 'DeepSeek Account',
      chatgpt: 'ChatGPT · DSH',
    }),
    antigravity: 'Google · Antigravity',
    kimi: 'Kimi Code',
    'codex-pro': 'ChatGPT · Codex Pro',
    'claude-code-pro': 'Claude Code Pro',
    custom: t('consoleCustom'),
  };
  // Provider choices own their model definitions; changing a stage default
  // cannot silently redefine a different provider in the editor.
  const preset = resource => ({
    'deepseek-account': { harness:'dsh', provider:'deepseek-account', channel:'deepseek-account', model:'deepseek-flash', thinking:'high' },
    chatgpt: { harness:'dsh', provider:'chatgpt', channel:'chatgpt', model:'gpt-6.2-sol', thinking:'high' },
    kimi: { harness:'kimi', channel:'kimi-subscription', model:'kimi-code/k3.1-pro', thinking:'high', ...(draft?.subscription ? {'native-profile':'kimi-subscription-agent'} : {}) },
    'codex-pro': { harness:'codex-pro', channel:'chatgpt-subscription', model:'gpt-6.2-sol', thinking:'high' },
    'claude-code-pro': { harness:'claude-code-pro', channel:'claude-native', model:'claude-opus-5-5', thinking:'high' },
    antigravity: { harness:'antigravity', channel:'google-subscription', model:'gemini-4-hydrogen-high', thinking:'high' },
  })[resource];
  const roleName = (group, key) =>
    group === 'routes'
      ? {
          default: t('cfgRouteDefault'),
          complex:t('cfgRouteComplex'),backend:t('cfgRouteBackend'),batch:t('cfgRouteBatch'),
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
  const bindingEfforts = value => value.harness === 'antigravity'
    ? fixedAntigravityEffort(value) ? [['', t('cfgEffortFixed')]] : (/^gemini-4-(hydrogen|neon)-/.test(value.model ?? '') ? ['low','medium','high','xhigh'] : ['low','medium','high']).map(key => [key,efforts[key]])
    : Object.entries(efforts);
  const changeModelField = (binding, field, value) => {
    const next = {...binding,[field]:value};
    if(next.harness === 'antigravity') {
      if(fixedAntigravityEffort(next)) { next.thinking = ''; return next; }
      if(field === 'thinking' && ['low','medium','high','xhigh'].includes(value)) next.model = (next.model ?? '').replace(/-(low|medium|high|xhigh)$/, '-' + value);
      if(field === 'model') { const match=/^gemini-4-(hydrogen|neon|carbon|argon)-(low|medium|high|xhigh)$/.exec(value);if(match)next.thinking=match[2]; }
    }
    return next;
  };
  const openBinding = (group, key) => setBindingSelection({ group, key });
  const bindingEdit = draft && bindingSelection && (() => {
    const { group, key } = bindingSelection, role = group === 'routes' ? 'execute' : key;
    const value = group === 'panel' ? draft['judge-panel'] : effectiveBinding(group, key);
    const execution = group === 'routes' && executionInitial(draft, key);
    return value ? { group, key, role, value, execution, permission: draft.permissions?.bindings?.[bindingID(group, key)] ?? 'inherit', session: draft.sessions?.roles?.[role] ?? 'inherit' } : null;
  })();
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
    else if (group === 'panel') next['judge-panel']=changeModelField(next['judge-panel'],field,value);
    else { next[group] ??= {}; next[group][key] = changeModelField(effectiveBinding(group,key),field,value); }
    if (group === 'routes' && field !== 'session') syncRouteDraft(next, key);
    replace(next);
  };
  const syncRouteDraft = (next, key) => {
    const error = synchronizeExecutionInitial(next, key, next.routes[key]) ? null : 'cfgExecutionAdvanced';
    setTextErrors(previous => ({ ...previous, ['execution-policy.routes.' + key]: error }));
  };
  const nativeChoices = (value, role) => {
    const inherited = draft.profiles?.[value.profile]?.['native-profile'];
    const harness = value.harness || draft.profiles?.[value.profile]?.harness;
    const unavailable = name => antigravitySwarmUnavailable(draft, { harness, 'native-profile': name }, role);
    const choices = [['__inherit__', t('consoleInherit') + (inherited ? ' · ' + inherited : '') + (unavailable(inherited) ? ' · ' + t('cfgSwarmRole') : ''), unavailable(inherited)], ['', t('cfgNativeNone')],
      ...Object.entries(draft['native-profiles'] ?? {}).filter(([, definition]) => record(definition) && definition.harness === harness)
        .map(([name]) => [name, name + (unavailable(name) ? ' · ' + t('cfgSwarmRole') : ''), unavailable(name)])];
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
    const value = setNativeProfile(group === 'panel' ? next['judge-panel'] : effectiveBinding(group, key), name);
    if (antigravitySwarmUnavailable(next, value, group === 'routes' ? 'execute' : key)) return;
    clearNumberDrafts(path => path === bindingID(group, key) + '.maxObservedAgents');
    if (group === 'panel') next['judge-panel'] = value;
    else { next[group] ??= {}; next[group][key] = value; }
    if (group === 'routes') syncRouteDraft(next, key);
    replace(next);
  };
  const setBindingEdit = (update) => {
    if (update === null) { setBindingSelection(null); return; }
    const { group, key, role, value, permission, session } = update(bindingEdit),
      next = cloneConfig(draft);
    if (antigravitySwarmUnavailable(next, value, role)) return;
    if (value.harness !== bindingEdit.value.harness || value['native-profile'] !== bindingEdit.value['native-profile'])
      clearNumberDrafts(path => path === bindingID(group, key) + '.maxObservedAgents');
    if (group === 'panel') next['judge-panel'] = value;
    else { next[group] ??= {}; next[group][key] = value; }
    if (permission !== (next.permissions?.bindings?.[bindingID(group, key)] ?? 'inherit')) {
      const bindings = { ...next.permissions?.bindings };
      if (permission === 'inherit') delete bindings[bindingID(group, key)];
      else bindings[bindingID(group, key)] = permission;
      next.permissions = { ...next.permissions, bindings };
    }
    if (session !== (next.sessions?.roles?.[role] ?? 'inherit')) applySessionRole(next, role, session);
    if (group === 'routes') syncRouteDraft(next, key);
    replace(next);
  };
  const patchBinding = (key, value) =>
    setBindingEdit((old) => ({ ...old, value: changeModelField(old.value,key,value) }));
  const bindingSummary = binding => {
    if (!binding) return t('notConfigured');
    const native = draft?.['native-profiles']?.[binding['native-profile']]?.options;
    return [binding.model || t('notConfigured'), fixedAntigravityEffort(binding) ? t('cfgEffortFixed') : binding.thinking || t('cfgEffortDefault'),
      ...(native?.mode ? [native.mode] : []), ...(binding.permission ? [binding.permission] : []),
      ...(binding['session-mode'] ? [t(binding['session-mode'] === 'scoped' ? 'cfgReuse' : 'cfgFresh')] : [])].join(' · ');
  };
  const jevProfileChoices = () => {
    const choices = Object.entries(draft.profiles ?? {}).filter(([, p]) => record(p) && p.harness === 'antigravity' &&
      !p.provider && !p['native-profile'] && !fixedAntigravityEffort(p)).map(([name, profile]) => [name, name + ' · ' + bindingSummary(profile)]);
    const selected = draft.jev?.profile ?? 'hydrogen-local';
    if (!choices.some(([name]) => name === selected)) choices.push([selected, selected + ' · ' + t('cfgJevUnresolved'), true]);
    return choices;
  };
  const candidateUnavailable = (route, id, removing = false) => {
    const policy = draft['execution-policy']?.routes?.[route], binding = draft['execution-policy']?.bindings?.[id];
    const initial = executionInitial(draft, route);
    if (!Array.isArray(policy?.allowed) || !Array.isArray(policy?.['quality-order']) || !completeExecutionBinding(binding) || !completeExecutionBinding(initial)) return 'cfgExecutionAdvanced';
    if (binding.permission !== initial.permission || binding['session-mode'] !== initial['session-mode']) return 'cfgExecutionAuthorityMismatch';
    if (removing && id === policy.initial) return 'cfgExecutionKeepInitial';
    if (removing && (policy['quality-order']?.includes(id) || Object.entries(policy['channel-fallbacks'] ?? {}).some(([from, to]) => from === id || to === id)))
      return 'cfgExecutionRuleReference';
    return null;
  };
  const editRoutePolicy = (route, key, value) => {
    const next = cloneConfig(draft), policy = next['execution-policy']?.routes?.[route];
    if (!policy) return;
    if (key === 'initial' && (!policy.allowed?.includes(value) || candidateUnavailable(route, value))) return;
    policy[key] = value;
    replace(next);
  };
  const allowCandidate = (route, id, enabled) => {
    const policy = draft['execution-policy']?.routes?.[route];
    if (!Array.isArray(policy?.allowed) || candidateUnavailable(route, id, !enabled)) return;
    editRoutePolicy(route, 'allowed', enabled ? [...new Set([...policy.allowed, id])] : policy.allowed.filter(value => value !== id));
  };
  const nativeEdit = bindingEdit && (() => {
    const binding = resolvedEditorBinding(draft, bindingEdit.value);
    if (binding?.harness !== 'antigravity') return null;
    const profile = binding['native-profile'], definition = profile ? draft['native-profiles']?.[profile] : null;
    const options = definition?.options;
    const valid = !profile || definition?.harness === 'antigravity' && record(options) && options.version === 1 &&
      ['native', 'subagents', 'swarm'].includes(options.mode) && Object.keys(options).every(key => ['version', 'mode', 'maxObservedAgents'].includes(key)) &&
      (options.maxObservedAgents === undefined || Number.isSafeInteger(options.maxObservedAgents) && options.maxObservedAgents >= 1 && options.maxObservedAgents <= 8);
    return { binding, options, valid, path: bindingID(bindingEdit.group, bindingEdit.key) + '.maxObservedAgents' };
  })();
  const editNativeOptions = (key, value) => {
    if (!nativeEdit?.valid || key === 'mode' && value === 'swarm' && bindingEdit.role !== 'execute') return;
    const { group, key: target, value: binding } = bindingEdit, next = cloneConfig(draft);
    let updated;
    if (key === 'mode' && value === '') {
      updated = { ...binding, 'native-profile': '' };
      clearNumberDrafts(path => path === nativeEdit.path);
    } else {
      const options = { version: 1, mode: 'native', ...nativeEdit.options };
      if (value === undefined) delete options[key]; else options[key] = value;
      updated = assignAntigravityOptions(next, group, target, binding, options);
    }
    if (group === 'panel') next['judge-panel'] = updated;
    else { next[group] ??= {}; next[group][target] = updated; }
    if (group === 'routes') syncRouteDraft(next, target);
    replace(next);
  };
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
      h('p', { className: css.meta }, t('planningInheritedHelp')),
      draft?.subscription?.mode==='subscription-only' && h('p',{className:css.configCallout},t('cfgSubscriptionOnly')),
      Object.entries(textErrors).some(([key,error]) => key.startsWith('execution-policy.') && error) && h('p',{className:css.configCallout,role:'alert'},t('cfgExecutionAdvanced')),
      record(draft['execution-policy']?.routes) && h('section',{className:css.details,'data-execution-candidates':true},
        h('h4',null,t('cfgExecutionCandidates')), h('p',{className:css.meta},t('cfgExecutionCandidatesHelp')),
        ...Object.entries(draft['execution-policy'].routes).filter(([,policy]) => record(policy)).map(([route,policy])=>h('article',{key:route,'data-policy-route':route},
          h('h4',null,roleName('routes',route)),
          field('execution-policy.routes.' + route + '.initial', roleName('routes', route) + ' · ' + t('cfgExecutionInitial'), t('cfgExecutionInitialHelp'),
            select('execution-policy.routes.' + route + '.initial', policy.initial ?? '',
              (Array.isArray(policy.allowed) ? policy.allowed : []).map(id => [id, id + ' · ' + bindingSummary(draft['execution-policy'].bindings?.[id]), !!candidateUnavailable(route, id)]),
              value => editRoutePolicy(route, 'initial', value))),
          field('execution-policy.routes.' + route + '.max-switches', roleName('routes', route) + ' · ' + t('cfgExecutionSwitches'), t('cfgExecutionSwitchHelp'),
            boundedInteger('execution-policy.routes.' + route + '.max-switches', policy['max-switches'], 0, 32, value => editRoutePolicy(route, 'max-switches', value))),
          h('details', null, h('summary', null, t('cfgExecutionAllowed') + ' · ' + (policy.allowed?.length ?? 0)),
            ...Object.entries(draft['execution-policy'].bindings ?? {}).filter(([,binding]) => record(binding)).map(([id, binding]) => {
              const allowed = Array.isArray(policy.allowed) && policy.allowed.includes(id), reason = candidateUnavailable(route, id, allowed);
              const native = draft['native-profiles']?.[binding['native-profile']]?.options;
              return h('div', {key: id, className: css.configField},
                h('label', null, h('input', { type: 'checkbox', checked: allowed, disabled: busy || !!reason,
                  'aria-label': roleName('routes', route) + ' · ' + t('cfgExecutionAllowed') + ' · ' + id,
                  onChange: event => allowCandidate(route, id, event.target.checked) }),
                  ' ' + id + ' · ' + bindingSummary(binding)),
                reason && h('p', { className: css.settingsHelp }, t(reason)),
                binding.harness === 'antigravity' && native?.mode === 'swarm' && h('p', { className: css.settingsHelp }, t('cfgSwarmEligibility')));
            })),
          ((policy['quality-order']?.length ?? 0) > 0 || Object.keys(policy['channel-fallbacks'] ?? {}).length > 0) &&
            h('p', { className: css.settingsHelp }, t('cfgExecutionAdvancedRules')),
          draft['execution-policy'].bindings?.[policy.initial]?.harness === 'antigravity' &&
            draft['native-profiles']?.[draft['execution-policy'].bindings[policy.initial]['native-profile']]?.options?.mode === 'swarm' &&
            h('p', { className: css.configCallout }, t('cfgSwarmEligibility'))))),
      draft.planning?.compact && h('section',{className:css.details},h('h4',null,t('cfgCompactPlanning')),
        ...['design-plan','design-review'].map(role=>field('planning.compact.'+role,t(role==='design-plan'?'cfgCompactDesign':'cfgCompactReview'),null,
          select('planning.compact.'+role,draft.planning.compact[role],Object.entries(draft.profiles??{}).map(([name,p])=>[name,name+' · '+bindingSummary(p)+(antigravitySwarmUnavailable(draft,p,role)?' · '+t('cfgSwarmRole'):''),antigravitySwarmUnavailable(draft,p,role)]),name=>{
            if (!antigravitySwarmUnavailable(draft,draft.profiles?.[name],role)) replace({...draft,planning:{...draft.planning,compact:{...draft.planning.compact,[role]:name}}});
          })))),
      h('p', { className: css.meta }, t('cfgSessionDefault', { mode: t(draft.sessions?.mode === 'scoped' ? 'cfgReuse' : 'cfgFresh') })),
      h('div', { className: css.stageTableScroll }, h('table', { className: css.stageTable, 'aria-label': t('cfgModels') },
        h('thead', null, h('tr', null, ...['cfgModelRole', 'consoleResource', 'consoleModel', 'consoleThinking', 'cfgNative', 'consoleSession'].map(key => h('th', { key, scope: 'col' }, t(key))))),
        h('tbody', null, ...rows.map(([group, key, value]) => {
          const role = group === 'routes' ? 'execute' : key;
          const sessionChoices = [['inherit', roleSession(role)], ['scoped', t('cfgReuse')], ['fresh', t('cfgFresh')]];
          const policy = draft.sessions?.roles?.[role];
          const planConflict = role === 'plan' && policy && (policy === 'scoped' && value['resident-session'] === false || policy === 'fresh' && (value.harness || draft.profiles?.[value.profile]?.harness) === 'kimi' && value['resident-session'] === true);
          return h('tr', { key: bindingID(group, key), 'data-binding-id': bindingID(group, key) },
            h('th', { scope: 'row' }, roleName(group, key), group === 'loops' && inheritedPlanningRoles[key] && !draft.loops?.[key] && h('p', { className: css.settingsHelp }, t('planningInherited', { source: 'loops.' + inheritedPlanningRoles[key] }))),
            h('td', null, h(Button, { disabled: busy, onClick: () => openBinding(group, key, value), 'aria-label': t('cfgEdit') + ' ' + roleName(group, key) }, resourceNames[resourceOf(value)])),
            h('td', null, h('input', { className: css.editorInput, value: value.model ?? '', placeholder: value.profile || t('cfgEffortDefault'), disabled: busy, 'aria-label': roleName(group, key) + ' · ' + t('consoleModel'), onChange: e => inlineBinding(group, key, 'model', e.target.value) })),
            h('td', null, h('select', { className: css.modeSelect, value: value.thinking ?? '', disabled: busy, 'aria-label': roleName(group, key) + ' · ' + t('consoleThinking'), onChange: e => inlineBinding(group, key, 'thinking', e.target.value) }, ...bindingEfforts(value).map(([effort,label]) => h('option', { key: effort, value: effort },label)))),
            h('td', null, h('select', { className: css.modeSelect, disabled: busy, value: value['native-profile'] ?? '__inherit__', 'aria-label': roleName(group, key) + ' · ' + t('cfgNative'), onChange: e => inlineNativeProfile(group, key, e.target.value) }, ...nativeChoices(value, role).map(([name, label, unavailable]) => h('option', { key: name, value: name, disabled: unavailable === true }, label))),
              resolvedEditorBinding(draft,value)?.harness === 'antigravity' && h('p', { className: css.settingsHelp }, t(role === 'execute' ? 'cfgSwarmConditionsShort' : 'cfgSwarmRole'))),
            h('td', null, group === 'routes' && executionInitial(draft,key) ? t(executionInitial(draft,key)['session-mode']==='scoped'?'cfgReuse':'cfgFresh') : ['intake', 'garden'].includes(role) ? t('cfgIndependent') : h('select', { className: css.modeSelect, disabled: busy, value: draft.sessions?.roles?.[role] ?? 'inherit', 'aria-label': roleName(group, key) + ' · ' + t('consoleSession'), onChange: e => inlineBinding(group, key, 'session', e.target.value) }, ...sessionChoices.map(([v, label]) => h('option', { key: v, value: v }, label))), planConflict && h('div', { className: css.settingsHelp }, t('cfgPlanConflict'), h(Button, { disabled: busy, onClick: () => inlineBinding(group, key, 'session', policy) }, t('cfgApplyPolicy')))));
        })))),
      h('p', { className: css.settingsHelp }, t('consoleRouteSession')),
      h('p', { className: css.settingsHelp }, t('cfgNativeHelp')));
  if (draft && section === 'jev')
    fields = h(
      React.Fragment,
      null,
      h('p', { className: css.meta }, t('cfgJevHelp')),
      h('section', { className: css.details, 'data-jev-scope': true },
        h('h4', null, t('cfgEditingSource')),
        h('p', null, t(scope === 'global' ? 'consoleGlobal' : 'consoleProject') + ' · ' + t(draft.jev?.enabled === true ? 'cfgToggleOn' : 'cfgToggleOff')),
        h('code', { className: css.path }, doc?.target ?? t('cfgEffectiveUnknown')),
        h('h4', null, t('cfgEffectiveSource')),
        !control.configUncertainty && effectiveRead.status === 'confirmed' && typeof effectiveRead.value?.source === 'string' && ['task', 'project', 'global', 'embedded'].includes(effectiveRead.value?.scope)
          ? h(React.Fragment, null,
            h('p', null, t({ task: 'cfgScopeTask', project: 'consoleProject', global: 'consoleGlobal', embedded: 'cfgScopeEmbedded' }[effectiveRead.value.scope]) + ' · ' +
              (typeof effectiveRead.value.jev?.enabled === 'boolean' ? t(effectiveRead.value.jev.enabled ? 'cfgToggleOn' : 'cfgToggleOff') : t('cfgEffectiveUnknown'))),
            h('code', { className: css.path }, effectiveRead.value.source),
            h('p', { className: css.meta }, t('cfgJevSubscriptionProfile') + ' · ' +
              (effectiveRead.value.jev?.backend === 'subscription'
                ? effectiveRead.value.jevBinding ? bindingSummary(effectiveRead.value.jevBinding) : t('cfgJevUnresolved')
                : effectiveRead.value.jev?.model || t('cfgJevUnresolved'))),
            (effectiveRead.value.source !== doc?.target || effectiveRead.value.scope !== scope) && h('p', { className: css.configCallout }, t('cfgEffectiveOtherScope')))
          : h('p', { className: css.configCallout, role: 'status' }, t(!control.configUncertainty && effectiveRead.status === 'loading' ? 'cfgEffectiveLoading' : 'cfgEffectiveUnknown')),
        h(Button, { disabled: busy, onClick: refreshEffective }, t('cfgEffectiveReload'))),
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
          draft.jev.backend === 'subscription'
            ? field('jev.profile',t('cfgJevSubscriptionProfile'),t('cfgJevSubscriptionHelp'),
                select('jev.profile',draft.jev.profile ?? 'hydrogen-local',jevProfileChoices(),v=>update('jev','profile',v)))
            : h(React.Fragment,null,
          field(
            'jev.endpoint',
            t('cfgJevEndpoint'),
            t('cfgJevEndpointHelp'),
            input('jev.endpoint', draft.jev.endpoint, (v) => update('jev', 'endpoint', v), v => jevTextError('endpoint', v)),
          ),
          field(
            'jev.model',
            t('consoleJEVVersion'),
            t('cfgJevModelHelp'),
            input('jev.model', draft.jev.model ?? 'jev-1.14.0', (v) => update('jev', 'model', v), v => jevTextError('model', v)),
          ),
          field(
            'jev.key-env',
            t('consoleJEVEnv'),
            t('cfgJevKeyHelp'),
            input('jev.key-env', draft.jev['key-env'] ?? 'MEROUTER_API_KEY', (v) =>
              update('jev', 'key-env', v), v => jevTextError('key-env', v),
            ),
          )),
          h(
            'section',
            { className: css.details },
            h('h4', null, t('cfgJevAdvanced')),
            field('jev.timeout-ms', t('cfgJevTimeout'), null, number('jev', 'timeout-ms', 1000, t('cfgSeconds'))),
            field('jev.max-calls', t('cfgJevMaxCalls'), null, number('jev', 'max-calls', 1, t('cfgTimes'))),
            field('jev.min-applicability', t('cfgJevApplicability'), null, number('jev', 'min-applicability', 1, '', false, true)),
            field('jev.min-choice', t('cfgJevChoice'), null, number('jev', 'min-choice', 1, '', false, true)),
            (draft.jev['allowed-data'] === undefined || Array.isArray(draft.jev['allowed-data']) && draft.jev['allowed-data'].length === 0) && h('p', { className: css.meta }, t('cfgJevNoData')),
            h('fieldset', { className: css.details }, h('legend', null, t('cfgJevData')),
              ...['task-summary', 'diagnostic-facts', 'logs', 'source', 'diff'].map(category => h('label', { key: category },
                h('input', { type: 'checkbox', disabled: busy || draft.jev['allowed-data'] !== undefined && !Array.isArray(draft.jev['allowed-data']), checked: Array.isArray(draft.jev['allowed-data']) && draft.jev['allowed-data'].includes(category),
                  'aria-label': t('cfgJevData') + ' · ' + category,
                  onChange: event => update('jev', 'allowed-data', event.target.checked ? [...(draft.jev['allowed-data'] ?? []), category] : (draft.jev['allowed-data'] ?? []).filter(value => value !== category)) }), ' ' + category + ' '))),
            h('p', { className: css.meta }, t('cfgJevDataHelp')),
          ),
        ),
    );
  const bindingFields = bindingEdit && h('section', { className: css.bindingEditor, 'aria-label': t('cfgBinding') },
    h('div', { className: css.bindingHeader },
      h(Button, { disabled: busy, onClick: () => setBindingSelection(null) }, t('cfgBindingDone')),
      h('h4', { className: css.heading }, roleName(bindingEdit.group, bindingEdit.key))),
    h('p', { className: css.settingsHelp }, t('cfgBindingSaveHelp')),
    Object.entries(textErrors).some(([key,error]) => key.startsWith('execution-policy.') && error) && h('p',{className:css.configCallout,role:'alert'},t('cfgExecutionAdvanced')),
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
            bindingEfforts(bindingEdit.value).concat(
              bindingEdit.value.thinking && !Object.hasOwn(efforts, bindingEdit.value.thinking)
                ? [[bindingEdit.value.thinking, bindingEdit.value.thinking]]
                : [],
            ),
            (v) => {
              if (v || fixedAntigravityEffort(bindingEdit.value)) patchBinding('thinking', v);
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
          select('binding.native-profile', bindingEdit.value['native-profile'] ?? '__inherit__', nativeChoices(bindingEdit.value, bindingEdit.role),
            name => setBindingEdit(old => ({ ...old, value: setNativeProfile(old.value, name) })))),
        nativeEdit && h('section', { className: css.details, 'data-antigravity-options': true },
          h('h4', null, t('cfgAgyOptions')),
          h('p', { className: css.settingsHelp }, t('cfgAgyOptionsHelp')),
          h('p', { className: css.configCallout }, t(bindingEdit.role === 'execute' ? 'cfgSwarmEligibility' : 'cfgSwarmRole')),
          nativeEdit.valid ? h(React.Fragment, null,
            field('binding.native-mode', t('cfgAgyMode'), t('cfgAgyModeHelp'),
              select('binding.native-mode', nativeEdit.options?.mode ?? '', [
                ['', t('cfgNativeNone')], ['native', t('cfgAgyNative')], ['subagents', t('cfgAgySubagents')],
                ['swarm', t('cfgAgySwarm') + (bindingEdit.role !== 'execute' ? ' · ' + t('cfgSwarmRole') : ''), bindingEdit.role !== 'execute'],
              ], value => editNativeOptions('mode', value))),
            nativeEdit.options && field(nativeEdit.path, t('cfgAgyObservedLimit'), t('cfgAgyObservedHelp'),
              boundedInteger(nativeEdit.path, nativeEdit.options.maxObservedAgents, 1, 8, value => editNativeOptions('maxObservedAgents', value), true)))
            : h('p', { className: css.errorText }, t('cfgAgyAdvanced'))),
        field(
          'binding.permission',
          t('permission'),
          bindingEdit.execution ? t('cfgExecutionAuthority') : t('consolePermissionEffective'),
          bindingEdit.execution ? h('span',null,bindingEdit.execution.permission) : select(
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
            bindingEdit.execution ? t('cfgExecutionAuthority') : bindingEdit.group === 'routes' ? t('consoleRouteSession') : t('consoleScopeReason'),
            bindingEdit.execution ? h('span',null,t(bindingEdit.execution['session-mode']==='scoped'?'cfgReuse':'cfgFresh')) : select(
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
          { className: css.segmented + ' ' + css.scopeSegmented, 'aria-label': t('consoleScope') },
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
          { key, 'aria-pressed': section === key, onClick: () => { setSection(key); setBindingSelection(null); } },
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
            section !== 'advanced' && h('fieldset', { className: css.configFields, disabled: invalid }, section === 'models' && bindingEdit ? bindingFields : fields),
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

  );
}

Object.assign(zh, {
  consoleConfig: '设置', cfgStrategy: '运行方式', cfgModels: '模型与会话', cfgJev: 'JEV', cfgAdvanced: '诊断与配置文件', cfgBack: '← 返回流程',
  cfgSessionHelp: '各角色独立，可按阶段覆盖', consoleSession: '会话策略', cfgReuse: '按角色复用', cfgFresh: '每次新建', cfgInheritMode: '继承 · {mode}', cfgSessionDefault: '默认：{mode}', cfgIndependent: '独立调用',
  cfgScheduleHelp: '', cfgParallelHelp: '并发数为 1–8；Review 与集成仍串行', cfgPermissionHelp: '完全权限可访问工作区以外文件',
  cfgBudgetPolicy: '用量预算', cfgBudgetOff: '关闭', cfgBudgetSoft: '仅提醒', cfgBudgetHard: '达到后停止',
  cfgBudgetOffHelp: '不设 Token 门限，继续统计用量', cfgBudgetSoftHelp: '达到额度时提醒，任务继续运行', cfgBudgetHardHelp: '达到额度或用量无法确认时，暂停新调用', cfgBudgetDay: '按单个任务、UTC 日期统计；留空不设额度', consoleBudget: '单任务每日 Token 额度',
  cfgRunProtection: '运行保护', cfgRetryHelp: '传输故障重试，不影响任务返修次数', cfgTimeoutHelp: '', cfgTimeout: '单次运行时限',
  cfgClean: '没有未保存的修改', cfgDirty: '有未保存修改', cfgSaved: '配置已保存', cfgReset: '恢复默认', cfgDiscard: '撤销',
});
Object.assign(en, {
  consoleConfig: 'Settings', cfgStrategy: 'Execution', cfgModels: 'Models and sessions', cfgJev: 'JEV', cfgAdvanced: 'Diagnostics and config', cfgBack: '← Back to workflow',
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
  cfgExecutionCandidates: '执行候选', cfgExecutionInitial: '初始配置',
  cfgExecutionCandidatesHelp: '选择实际初始候选、允许集合和切换次数。模型列表显示实际初始配置；编辑路线会创建或更新该路线的独立初始候选。JEV 只在允许集合中经运行时资格筛选的候选之间选择。',
  cfgExecutionAuthority: '此路线的执行策略显式固定权限与会话。请在高级 JSON 中编辑完整候选，模型编辑不会扩大权限。',
  cfgExecutionAdvanced: '无法从当前编辑确定完整执行候选。请补全模型与思考等级，或在高级 JSON 中审查 execution-policy；尚未保存。',
});
Object.assign(en, {
  cfgNative: 'Native capabilities', cfgNativeNone: 'No native capability profile',
  cfgNativeHelp: 'Select a profile for this backend; edit native-profiles in advanced JSON. Profiles work with JEV off. Runtime receipts establish actual availability.',
  cfgExecutionCandidates: 'Execution candidates', cfgExecutionInitial: 'Initial configuration',
  cfgExecutionCandidatesHelp: 'Choose the actual initial candidate, allowed set and switch limit. The model table shows that initial binding; editing a route creates or updates its own initial candidate. JEV selects only allowed candidates that pass runtime eligibility checks.',
  cfgExecutionAuthority: 'This execution policy explicitly fixes permissions and sessions. Edit complete candidates in advanced JSON; changing a model does not expand permissions.',
  cfgExecutionAdvanced: 'This edit does not determine a complete execution candidate. Complete the model and effort or review execution-policy in advanced JSON. Nothing was saved.',
});

Object.assign(zh, { cfgBindingDone: '← 返回模型列表', cfgBindingSaveHelp: '修改后点击下方“保存修改”，统一保存到当前作用域。' });
Object.assign(en, { cfgBindingDone: '← Back to model list', cfgBindingSaveHelp: 'Use Save changes below to save all edits to the selected scope.' });

Object.assign(zh,{cfgRouteComplex:'复杂实现',cfgRouteBackend:'后端实现',cfgRouteBatch:'批量实现',cfgSubscriptionOnly:'仅使用订阅渠道；实际登录、额外计费开关与可用模型在调用前检查。',cfgCompactPlanning:'紧凑方案',cfgCompactDesign:'联合方案生成',cfgCompactReview:'联合方案审查',cfgJevSubscriptionProfile:'判断模型配置',cfgJevSubscriptionHelp:'使用订阅模型分析已收集的事实并从已有动作中选择；不授予文件或命令工具。'});
Object.assign(en,{cfgRouteComplex:'Complex implementation',cfgRouteBackend:'Backend implementation',cfgRouteBatch:'Batch implementation',cfgSubscriptionOnly:'Subscription channels only. Native login, extra billing controls and model availability are checked before dispatch.',cfgCompactPlanning:'Compact planning',cfgCompactDesign:'Combined proposal',cfgCompactReview:'Combined review',cfgJevSubscriptionProfile:'Decision model profile',cfgJevSubscriptionHelp:'A subscription model analyzes collected facts and chooses an existing action without file or command tools.'});

Object.assign(zh, {
  cfgExecutionInitialHelp: '选择已有的完整绑定；它的权限、会话和原生配置保持原值。当前任务已冻结的配置不会被改写。',
  cfgExecutionAllowed: '允许的候选', cfgExecutionSwitches: '最多切换次数',
  cfgExecutionSwitchHelp: '0–32 次；首轮改选与失败调整共用此路线的切换预算。设为 0 会保留初始候选。',
  cfgExecutionSwitchRange: '切换次数须为 0 到 32 的整数',
  cfgExecutionAuthorityMismatch: '此完整绑定的权限或会话与当前初始候选不同；请在高级 JSON 中明确审查配置。',
  cfgExecutionKeepInitial: '先选择另一个初始候选，才能从允许集合移除此项。',
  cfgExecutionRuleReference: '质量次序或渠道后备仍引用此项；请先在高级 JSON 中调整规则。',
  cfgExecutionAdvancedRules: '已有质量次序或跨渠道后备规则保留；在高级 JSON 中查看和编辑。',
  cfgSwarmRole: 'Antigravity Swarm 仅用于 Execute；此角色可选择 native 或 subagents。',
  cfgSwarmConditionsShort: 'Swarm：Full、已审分工、无原生图片；运行时复核资格。',
  cfgSwarmEligibility: 'Antigravity Swarm 仅适用于 Full，并需要已审批的 2–8 项独立写入分工及无原生图片输入的 packet。没有图片的任务可以使用；当前 Antigravity 没有 Flywheel 原生图片输入通道。此处配置不代表当前任务已满足条件。',
  cfgAgyOptions: 'Antigravity 原生选项',
  cfgAgyOptionsHelp: '编辑会为此阶段或路线保留专属原生配置；共享配置、其它候选、权限与会话保持原值。完整定义仍可在高级 JSON 中编辑。',
  cfgAgyMode: '原生协作方式', cfgAgyNative: 'native · 原生工具自主运行', cfgAgySubagents: 'subagents · 请求委派', cfgAgySwarm: 'swarm · 官方 /swarm',
  cfgAgyModeHelp: '这是请求的方式；实际子代理是否出现及完成证据，以运行回执为准。native 不代表强制单智能体。',
  cfgAgyObservedLimit: '观察到的子代理总数上限',
  cfgAgyObservedHelp: '留空不设置，或填写 1–8。包含观察到的 verifier/monitor；超过后请求停止，不是硬并发上限，也不能证明所有子代理已结束。',
  cfgAgyAdvanced: '当前原生定义无法安全地用这些控件编辑，请在高级 JSON 中修正。',
  cfgEditingSource: '正在编辑的文件', cfgEffectiveSource: '当前任务实际采用的配置',
  cfgScopeTask: '任务', cfgScopeEmbedded: '安装默认',
  cfgEffectiveOtherScope: '当前任务采用另一个范围的文件。保存此处的开关不会越过配置优先级；请以保存后重新读取的实际配置为准。',
  cfgEffectiveLoading: '正在重新读取当前任务的有效配置…',
  cfgEffectiveUnknown: '当前有效配置尚未确认；不能据此判断 JEV 已开启或关闭。',
  cfgEffectiveReload: '重新核对当前生效配置', cfgJevUnresolved: '判断模型未解析',
});
Object.assign(en, {
  cfgExecutionInitialHelp: 'Select an existing complete binding with its permissions, session and native options intact. Already frozen task configurations do not change.',
  cfgExecutionAllowed: 'Allowed candidates', cfgExecutionSwitches: 'Maximum switches',
  cfgExecutionSwitchHelp: '0–32 switches. Initial selection and failure adaptation share this route budget. Zero retains the initial candidate.',
  cfgExecutionSwitchRange: 'The switch limit must be an integer from 0 to 32',
  cfgExecutionAuthorityMismatch: 'This complete binding has different permissions or a different session policy. Review the change explicitly in advanced JSON.',
  cfgExecutionKeepInitial: 'Choose another initial candidate before removing this one from the allowed set.',
  cfgExecutionRuleReference: 'A quality-order or channel-fallback rule still references this candidate. Edit that rule in advanced JSON first.',
  cfgExecutionAdvancedRules: 'Existing quality-order and channel-fallback rules are preserved. Inspect and edit them in advanced JSON.',
  cfgSwarmRole: 'Antigravity Swarm is available only for Execute. This role can use native or subagents.',
  cfgSwarmConditionsShort: 'Swarm: Full, reviewed assignments, no native images; eligibility is checked at runtime.',
  cfgSwarmEligibility: 'Antigravity Swarm requires Full, an approved packet with 2–8 independent write assignments, and no native image inputs. Tasks without images can use it; Antigravity currently has no Flywheel native image-input channel. This configuration does not establish the current task’s eligibility.',
  cfgAgyOptions: 'Antigravity native options',
  cfgAgyOptionsHelp: 'Edits keep a dedicated profile for this stage or route. Shared profiles, other candidates, permissions and sessions remain intact. Advanced JSON keeps the complete definitions available.',
  cfgAgyMode: 'Native collaboration mode', cfgAgyNative: 'native · autonomous native tools', cfgAgySubagents: 'subagents · request delegation', cfgAgySwarm: 'swarm · official /swarm',
  cfgAgyModeHelp: 'This is the requested mode. Runtime receipts establish whether children appeared and what completion evidence exists. Native does not force a single agent.',
  cfgAgyObservedLimit: 'Observed total child limit',
  cfgAgyObservedHelp: 'Leave blank or enter 1–8. Includes observed verifier/monitor children; exceeding it requests a stop. It is not a hard concurrency limit or evidence that every child ended.',
  cfgAgyAdvanced: 'These controls cannot safely edit the current native definition. Correct it in advanced JSON.',
  cfgEditingSource: 'File being edited', cfgEffectiveSource: 'Configuration actually selected for this task',
  cfgScopeTask: 'Task', cfgScopeEmbedded: 'Installed defaults',
  cfgEffectiveOtherScope: 'The current task selects a file from another scope. Saving this switch does not bypass configuration precedence. Check the effective configuration reread after saving.',
  cfgEffectiveLoading: 'Rereading the effective task configuration…',
  cfgEffectiveUnknown: 'The effective configuration is unconfirmed. Its JEV switch cannot be reported as on or off.',
  cfgEffectiveReload: 'Recheck effective configuration', cfgJevUnresolved: 'Decision model unresolved',
});
