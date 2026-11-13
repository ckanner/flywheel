Object.assign(zh, { cfgWorktree: '自动管理任务 Worktree', cfgWorktreeHelp: '默认开启。关闭后不自动创建 Worktree；仍可并行，模型或用户可选择并登记执行目录。已有目录不会被删除', cfgWorktreeOn: '开启 · 自动创建 Worktree', cfgWorktreeOff: '关闭 · 自行管理执行目录' });
Object.assign(en, { cfgWorktree: 'Automatically manage task Worktrees', cfgWorktreeHelp: 'On by default. Off does not create Worktrees and still allows parallel tasks; callers may register their own execution checkouts. Existing directories are retained', cfgWorktreeOn: 'On · automatic Worktrees', cfgWorktreeOff: 'Off · caller-managed execution' });
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
  cfgJev: 'JEV 影子建议',
  cfgMore: '更多配置操作',
  cfgSessionHelp: '同任务、同角色续接；执行与独立 Review 不共享历史',
  cfgScheduleHelp: '仅对契约声明独立、依赖已满足的任务并发执行',
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
  cfgJevHelp: '可选，默认关闭。先捕获本地摘要，后续显式评估；不改变 Quick / Full 或模型路由',
  cfgJevSend: '开启后，新任务首次普通运行仅保存任务目标与验收摘要，不发 HTTP。审阅后显式 shadow run 才发送并占用独立实验预算',
  cfgJevEndpoint: 'MeRouter System One API 地址',
  cfgJevKeyHelp: '仅保存环境变量名，不填写 API Key；显式 shadow run 进程需要能读取该变量，本地捕获不需要密钥',
  cfgJevTimeout: '请求超时',
  cfgJevAdvanced: '模型版本与请求参数',
  cfgJevConfirm: '开启 JEV 影子建议',
  cfgJevEnable: '开启并保存',
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
  cfgJev: 'JEV shadow advice',
  cfgMore: 'More configuration actions',
  cfgSessionHelp:
    'Reuse within the same task and role; execution and independent Review do not share history',
  cfgScheduleHelp: 'Parallel execution requires declared independence and satisfied dependencies',
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
    'Optional and off by default. Capture locally for later explicit evaluation; Quick / Full and model routing stay unchanged',
  cfgJevSend:
    'Enabling captures the goal and acceptance summary locally on a new task’s first run, without HTTP. Review it before explicit shadow run uses a separate experiment budget',
  cfgJevEndpoint: 'MeRouter System One API endpoint',
  cfgJevKeyHelp: 'Store the variable name, not the API key. The explicit shadow run process must inherit it; local capture needs no key',
  cfgJevTimeout: 'Request timeout',
  cfgJevAdvanced: 'Model version and request options',
  cfgJevConfirm: 'Enable JEV shadow advice',
  cfgJevEnable: 'Enable and save',
  cfgLocation: 'Configuration file and source',
  cfgRawDiff: 'Review changes',
  cfgModelRole: 'Stage',
  cfgProviderID: 'Provider ID',
  cfgConflict: 'The file changed externally. Your draft is retained; reload and review before saving',
});

const cloneConfig = (value) => JSON.parse(JSON.stringify(value));
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
  const dirty = changed.length > 0 || reset || invalid || Object.values(numberErrors).some(Boolean);
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
    !Object.values(numberErrors).some(Boolean);
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
  const submit = () => {
    if (draft['jev-shadow']?.enabled === true && base['jev-shadow']?.enabled !== true)
      setDialog({ kind: 'jev' });
    else save();
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
  const input = (key, value, onChange) =>
    h('input', {
      id: inputID(key),
      className: css.editorInput,
      value: value ?? '',
      disabled: busy,
      'aria-describedby': inputID(key) + '-help',
      onChange: (e) => onChange(e.target.value),
    });
  const number = (group, key, factor, unit, nullable = false) => {
    const path = group + '.' + key,
      current = draft[group]?.[key];
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
          'aria-invalid': !!numberErrors[path],
          onChange: (e) => {
            const value = e.target.value;
            setNumbers((old) => ({ ...old, [path]: value }));
            if (nullable && !value.trim()) {
              setNumberErrors((old) => ({ ...old, [path]: null }));
              update(group, key, null);
              return;
            }
            const scaled = Number(value) * factor,
              rounded = Math.round(scaled);
            const error = !value.trim()
              ? 'cfgRequiredNumber'
              : !/^\d+(?:\.\d+)?$/.test(value.trim()) ||
                  !Number.isSafeInteger(rounded) ||
                  scaled < 0 ||
                  Math.abs(scaled - rounded) > 0.000001
                ? 'cfgNumber'
                : null;
            setNumberErrors((old) => ({ ...old, [path]: error }));
            setSaved(false);
            if (!error) update(group, key, rounded);
          },
        }),
        h('span', null, unit),
      ),
      numberErrors[path] && h('p', { role: 'alert', className: css.errorText }, t(numberErrors[path])),
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
        select('worktree.enabled', String(draft.worktree?.enabled !== false),
          [['true', t('cfgWorktreeOn')], ['false', t('cfgWorktreeOff')]],
          (v) => replace({ ...draft, worktree: { ...draft.worktree, enabled: v === 'true' } }))),
      field(
        'scheduler.mode',
        t('consoleScheduling'),
        t('cfgScheduleHelp'),
        select(
          'scheduler.mode',
          draft.scheduler?.mode ?? 'parallel',
          [
            ['serial', t('consoleSerial')],
            ['parallel', t('consoleParallel')],
          ],
          (v) => replace({ ...draft, scheduler: { 'max-parallel': 2, ...draft.scheduler, mode: v } }),
        ),
      ),
      draft.scheduler?.mode === 'parallel' &&
        field(
          'scheduler.max-parallel',
          t('consoleConcurrency'),
          t('cfgParallelHelp'),
          number('scheduler', 'max-parallel', 1, t('cfgTasks')),
        ),
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
              setNumbers(old => { const next = { ...old }; delete next['budget.daily-token-limit']; return next; });
              setNumberErrors(old => { const next = { ...old }; delete next['budget.daily-token-limit']; return next; });
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
        h('thead', null, h('tr', null, ...['cfgModelRole', 'consoleResource', 'consoleModel', 'consoleThinking', 'consoleSession'].map(key => h('th', { key, scope: 'col' }, t(key))))),
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
            h('td', null, ['intake', 'garden'].includes(role) ? t('cfgIndependent') : h('select', { className: css.modeSelect, disabled: busy, value: draft.sessions?.roles?.[role] ?? 'inherit', 'aria-label': roleName(group, key) + ' · ' + t('consoleSession'), onChange: e => inlineBinding(group, key, 'session', e.target.value) }, ...sessionChoices.map(([v, label]) => h('option', { key: v, value: v }, label))), planConflict && h('div', { className: css.settingsHelp }, t('cfgPlanConflict'), h(Button, { disabled: busy, onClick: () => inlineBinding(group, key, 'session', policy) }, t('cfgApplyPolicy')))));
        })))),
      h('p', { className: css.settingsHelp }, t('consoleRouteSession')));
  if (draft && section === 'jev')
    fields = h(
      React.Fragment,
      null,
      h('p', { className: css.meta }, t('cfgJevHelp')),
      field(
        'jev-shadow.enabled',
        t('consoleJEV'),
        null,
        select(
          'jev-shadow.enabled',
          String(draft['jev-shadow']?.enabled ?? false),
          [
            ['false', t('consoleJEVOff')],
            ['true', t('consoleJEVOn')],
          ],
          (v) => update('jev-shadow', 'enabled', v === 'true'),
        ),
      ),
      draft['jev-shadow']?.enabled &&
        h(
          React.Fragment,
          null,
          h('p', { className: css.configCallout }, t('cfgJevSend')),
          field(
            'jev-shadow.endpoint',
            t('cfgJevEndpoint'),
            null,
            input('jev-shadow.endpoint', draft['jev-shadow'].endpoint, (v) =>
              update('jev-shadow', 'endpoint', v),
            ),
          ),
          field(
            'jev-shadow.key-env',
            t('consoleJEVEnv'),
            t('cfgJevKeyHelp'),
            input('jev-shadow.key-env', draft['jev-shadow']['key-env'] ?? 'MEROUTER_API_KEY', (v) =>
              update('jev-shadow', 'key-env', v),
            ),
          ),
          h(
            'section',
            { className: css.details },
            h('h4', null, t('cfgJevAdvanced')),
            field(
              'jev-shadow.model',
              t('consoleModel'),
              null,
              input('jev-shadow.model', draft['jev-shadow'].model ?? 'jev', (v) =>
                update('jev-shadow', 'model', v),
              ),
            ),
            field(
              'jev-shadow.expected-model',
              t('consoleJEVVersion'),
              null,
              input('jev-shadow.expected-model', draft['jev-shadow']['expected-model'] ?? 'jev-1.14.0', (v) =>
                update('jev-shadow', 'expected-model', v),
              ),
            ),
            field(
              'jev-shadow.timeout-ms',
              t('cfgJevTimeout'),
              null,
              number('jev-shadow', 'timeout-ms', 1000, t('cfgSeconds')),
            ),
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
            section !== 'advanced' && fields,
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
                'aria-invalid': invalid,
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
        invalid && h('p', { role: 'alert', className: css.errorText }, t('consoleInvalid')),
        Object.values(numberErrors).some(Boolean) &&
          h('p', { role: 'alert', className: css.errorText }, t('cfgInvalid')),
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
      h('span', { className: css.meta, role: saved ? 'status' : undefined }, t(saved ? 'cfgSaved' : dirty ? 'cfgDirty' : 'cfgClean')),
      h('div', { className: css.actions },
        h(Button, { disabled: busy || !doc || doc.scope !== scope, onClick: () => setDialog({ kind: 'reset' }) }, t('cfgReset')),
        h(Button, { disabled: busy || !dirty || !base, onClick: discard }, t('cfgDiscard')),
        h(Button, { variant: 'primary', disabled: !ready || !dirty || busy || control.configUncertainty || !!control.uncertainty, onClick: submit }, t('cfgSave')))),
    dialog &&
      h(
        WorkbenchDialog,
        {
          t,
          title: t(
            dialog.kind === 'reset' ? 'cfgReset' : dialog.kind === 'jev' ? 'cfgJevConfirm' : 'cfgUnsaved',
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
                    setReset(true);
                  } else if (next.kind === 'jev') save();
                  else next.next();
                },
              },
              t(
                dialog.kind === 'reset'
                  ? 'cfgLoadDefaults'
                  : dialog.kind === 'jev'
                    ? 'cfgJevEnable'
                    : 'cfgDiscardContinue',
              ),
            ),
          ),
        },
        h(
          'p',
          null,
          t(
            dialog.kind === 'reset'
              ? 'cfgResetHelp'
              : dialog.kind === 'jev'
                ? 'cfgJevSend'
                : 'cfgDiscardHelp',
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
  cfgWorktree: '独立 Worktree', cfgWorktreeHelp: '', cfgScheduleHelp: '', cfgParallelHelp: '', cfgPermissionHelp: '完全权限可访问工作区以外文件',
  cfgBudgetPolicy: '用量预算', cfgBudgetOff: '关闭', cfgBudgetSoft: '仅提醒', cfgBudgetHard: '达到后停止',
  cfgBudgetOffHelp: '不设 Token 门限，继续统计用量', cfgBudgetSoftHelp: '达到额度时提醒，任务继续运行', cfgBudgetHardHelp: '达到额度或用量无法确认时，暂停新调用', cfgBudgetDay: '按单个任务、UTC 日期统计；留空不设额度', consoleBudget: '单任务每日 Token 额度',
  cfgRunProtection: '运行保护', cfgRetryHelp: '传输故障重试，不影响任务返修次数', cfgTimeoutHelp: '', cfgTimeout: '单次运行时限',
  cfgClean: '修改用于下次启动', cfgDirty: '有未保存修改 · 下次启动生效', cfgReset: '恢复默认', cfgDiscard: '撤销', cfgJevHelp: '只记录建议，不改变实际流程或模型选择', consoleJEV: '影子评估',
});
Object.assign(en, {
  consoleConfig: 'Settings', cfgStrategy: 'Execution', cfgModels: 'Models and sessions', cfgJev: 'JEV', cfgAdvanced: 'Advanced and diagnostics', cfgBack: '← Back to workflow',
  cfgSessionHelp: 'Isolated roles; stages can override the default', cfgReuse: 'Reuse within role', cfgFresh: 'New each time', cfgInheritMode: 'Inherit · {mode}', cfgSessionDefault: 'Default: {mode}', cfgIndependent: 'Independent call',
  cfgWorktree: 'Independent Worktrees', cfgWorktreeHelp: '', cfgScheduleHelp: '', cfgParallelHelp: '', cfgPermissionHelp: 'Full access includes files outside the workspace',
  cfgBudgetPolicy: 'Usage budget', cfgBudgetOff: 'Off', cfgBudgetSoft: 'Warn only', cfgBudgetHard: 'Stop at limit', cfgBudgetOffHelp: 'No token limit; usage is still recorded', cfgBudgetSoftHelp: 'Warn at the limit and keep running', cfgBudgetHardHelp: 'Pause new calls at the limit or when usage is unknown', cfgBudgetDay: 'Per task and UTC day; blank means no limit', consoleBudget: 'Daily tokens per task',
  cfgRunProtection: 'Run protection', cfgRetryHelp: 'Transport faults only; task repair limits stay unchanged', cfgTimeoutHelp: '', cfgTimeout: 'Run time limit',
  cfgClean: 'Changes apply to the next start', cfgDirty: 'Unsaved changes · next start', cfgReset: 'Restore defaults', cfgDiscard: 'Discard', cfgJevHelp: 'Capture advice only; no workflow or model routing changes', consoleJEV: 'Shadow evaluation',
});

Object.assign(zh, { cfgPlanConflict: 'Plan 旧历史开关与此策略不同', cfgApplyPolicy: '同步此策略' });
Object.assign(en, { cfgPlanConflict: 'The legacy Plan history switch differs', cfgApplyPolicy: 'Align policy' });
