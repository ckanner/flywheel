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
  cfgJevHelp: '可选，默认关闭。只记录流程建议，不改变 Quick / Full，也不接管模型路由',
  cfgJevSend: '开启后，新任务首次普通运行最多发送一次任务目标与验收摘要；调用失败不阻断任务，硬预算任务跳过',
  cfgJevEndpoint: 'MeRouter System One API 地址',
  cfgJevKeyHelp: '仅保存环境变量名，不填写 API Key；DSH 桌面启动的引擎需要能读取该变量',
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
    'Optional and off by default. Advice is recorded without changing Quick / Full or model routing',
  cfgJevSend:
    'Enabling sends at most one goal and acceptance summary on a new task’s first ordinary run. Failures do not block the workflow; hard-budget tasks skip it',
  cfgJevEndpoint: 'MeRouter System One API endpoint',
  cfgJevKeyHelp: 'Store the environment variable name, not the API key. The desktop engine must inherit it',
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
function ConfigEditor({ control, selection, run, t, active = false, cache, contextKey }) {
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
        help && h('p', { className: css.meta, id: inputID(key) + '-help' }, help),
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
      permission: draft.permissions?.bindings?.[group + '.' + key] ?? 'inherit',
      session: draft.sessions?.roles?.[role] ?? 'inherit',
    });
  };
  const saveBinding = () => {
    const { group, key, role, value, permission, session } = bindingEdit,
      next = cloneConfig(draft);
    next[group][key] = value;
    if (value.channel && !next.channels?.[value.channel])
      next.channels = {
        ...next.channels,
        [value.channel]: doc.defaults.channels?.[value.channel] ?? { 'env-key': null },
      };
    if (permission !== (next.permissions?.bindings?.[group + '.' + key] ?? 'inherit')) {
      const bindings = { ...next.permissions?.bindings };
      if (permission === 'inherit') delete bindings[group + '.' + key];
      else bindings[group + '.' + key] = permission;
      next.permissions = { ...next.permissions, bindings };
    }
    if (session !== (next.sessions?.roles?.[role] ?? 'inherit')) {
      const roles = { ...next.sessions?.roles };
      if (session === 'inherit') delete roles[role];
      else roles[role] = session;
      next.sessions = { ...next.sessions, roles };
    }
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
      field(
        'budget.daily-token-limit',
        t('consoleBudget'),
        t('cfgBudgetHelp'),
        number('budget', 'daily-token-limit', 1, 'tokens', true),
      ),
      field(
        'budget.enforce',
        t('cfgBudgetPolicy'),
        null,
        select(
          'budget.enforce',
          draft.budget?.enforce ?? 'soft',
          [
            ['off', t('cfgBudgetOff')],
            ['soft', t('cfgBudgetSoft')],
            ['hard', t('cfgBudgetHard')],
          ],
          (v) => update('budget', 'enforce', v),
        ),
      ),
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
    fields = h(
      'div',
      { className: css.modelRows },
      h('p', { className: css.meta }, t('cfgBindingsHelp')),
      ...rows.map(([group, key, value]) =>
        h(
          'article',
          { key: group + '.' + key, className: css.modelRow, 'data-binding-id': group + '.' + key },
          h('h4', { className: css.heading }, roleName(group, key)),
          h(
            'div',
            null,
            h('p', null, resourceNames[resourceOf(value)]),
            h(
              'p',
              { className: css.meta },
              [value.model, efforts[value.thinking ?? ''] ?? value.thinking].filter(Boolean).join(' · '),
            ),
          ),
          h(
            Button,
            {
              disabled: busy,
              onClick: () => openBinding(group, key, value),
              'aria-label': t('cfgEdit') + ' ' + roleName(group, key),
            },
            t('cfgEdit'),
          ),
        ),
      ),
    );
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
            'details',
            { className: css.details },
            h('summary', null, t('cfgJevAdvanced')),
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
      'nav',
      { className: css.configNav, 'aria-label': t('consoleConfig') },
      ...['strategy', 'models', 'limits', 'jev'].map((key) =>
        h(
          Button,
          { key, 'aria-pressed': section === key, onClick: () => setSection(key) },
          t({ strategy: 'cfgStrategy', models: 'cfgModels', limits: 'cfgLimits', jev: 'cfgJev' }[key]),
        ),
      ),
    ),
    h(
      'div',
      { className: css.configMain },
      h(
        'div',
        { className: css.configHeader },
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
        h(
          'details',
          { className: css.configMore },
          h('summary', null, t('cfgMore')),
          h(
            'div',
            { className: css.actions },
            h(Button, { disabled: busy, onClick: () => deferDiscard(load) }, t('cfgReload')),
            h(
              Button,
              { disabled: busy || !doc || doc.scope !== scope, onClick: () => setDialog({ kind: 'reset' }) },
              t('cfgReset'),
            ),
          ),
        ),
      ),
      h(
        'div',
        { className: css.configContent },
        h(
          'h3',
          { className: css.configTitle },
          t({ strategy: 'cfgStrategy', models: 'cfgModels', limits: 'cfgLimits', jev: 'cfgJev' }[section]),
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
                'details',
                { className: css.details },
                h('summary', null, t('errorDetails')),
                h('pre', { className: css.raw }, control.error.detail),
              ),
          ),
        draft &&
          doc?.scope === scope &&
          h(
            React.Fragment,
            null,
            fields,
            h(
              'details',
              { className: css.configSource },
              h('summary', null, t('cfgLocation')),
              h('p', { className: css.meta }, t('cfgScopeHelp')),
              h('p', null, t(doc.source === doc.target ? 'cfgFileSource' : 'cfgTemplateSource')),
              h('code', { className: css.path }, doc.target),
            ),
            h(
              'details',
              { className: css.configSource },
              h('summary', null, t('cfgJson')),
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
            (changed.length > 0 || reset) &&
              h(
                'details',
                { className: css.configSource },
                h('summary', null, t('cfgRawDiff')),
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
      draft &&
        doc?.scope === scope &&
        h(
          'footer',
          { className: css.configSaveBar, 'data-dirty': dirty },
          dirty
            ? h(
                React.Fragment,
                null,
                h(
                  Button,
                  {
                    variant: 'primary',
                    disabled: !ready || busy || control.configUncertainty,
                    onClick: submit,
                  },
                  t('cfgSave'),
                ),
                h(Button, { disabled: busy, onClick: discard }, t('cfgDiscard')),
                h('span', { className: css.meta }, t('cfgDirty')),
              )
            : h(
                'p',
                { className: css.meta, role: saved ? 'status' : undefined },
                t(saved ? 'cfgSaved' : 'cfgClean'),
              ),
        ),
    ),
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
          'details',
          { className: css.details },
          h('summary', null, t('cfgBindingAdvanced')),
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
