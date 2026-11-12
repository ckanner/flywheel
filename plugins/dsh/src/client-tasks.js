// Task discovery consumes Go's registered runs; no directory scanning or shell parsing.
Object.assign(zh, {
  taskChooser: '选择任务', taskSelected: '当前任务', taskSwitch: '切换任务', taskRecent: '本机最近任务', taskThisSession: '本会话任务',
  taskRefresh: '刷新任务列表', taskPick: '选择一个任务以打开画布', taskNoSelection: '还没有关联任务',
  taskEmptyHelp: '助手准备并运行任务后会显示在这里。已运行过的任务可直接从列表打开，无需填写路径',
  taskNone: '暂无已登记的运行；加载 Skill 不代表引擎已经启动', taskDiscoveryError: '任务列表读取失败，可重试；已打开的画布仍保留',
  taskMultiple: '本会话有多个任务，请选择要查看的任务', taskWorkspace: '工作区', taskLocation: '任务位置（自动取得）', taskFolderLocation: '任务记录',
  taskRequired: '请先从任务列表选择一个任务', emptyBody: '准备任务后调用 flywheel_launch，或从最近任务中打开已有运行',
  stageFocus: '本次运行', stageFull: '完整流程', stageOnly: '仅运行{stage}阶段',
  stagePassed: '本次{stage}已通过', stageFailed: '本次{stage}未通过', stageCancelled: '本次{stage}已取消',
  stageRunning: '本次{stage}进行中', stageUnknown: '本次{stage}结果待核对',
  stageScopeHelp: '这里展示这次单阶段运行的回执。通过该阶段不代表完整任务已交付',
  stageContinue: '需要继续完整流程时', stageFullState: '整体交付：{status}', stageUndelivered: '尚未完成', stageDelivered: '已完成', stageRecordedScore: '记录评分', stageRecordedVerdict: '记录结论', stageMetadata: '运行记录与文件位置',
});
Object.assign(en, {
  taskChooser: 'Choose task', taskSelected: 'Current task', taskSwitch: 'Switch task', taskRecent: 'Recent tasks on this computer', taskThisSession: 'Tasks in this conversation',
  taskRefresh: 'Refresh task list', taskPick: 'Choose a task to open its canvas', taskNoSelection: 'No task associated yet',
  taskEmptyHelp: 'Tasks appear here after the assistant prepares and runs them. Open an existing run from the list without entering paths',
  taskNone: 'No registered runs. Loading a skill does not start the engine', taskDiscoveryError: 'Could not read tasks. Retry; the existing canvas is retained',
  taskMultiple: 'This conversation has several tasks. Choose one to inspect', taskWorkspace: 'Workspace', taskLocation: 'Task location (automatic)', taskFolderLocation: 'Task records',
  taskRequired: 'Choose a task from the list first', emptyBody: 'Prepare a task and call flywheel_launch, or open an existing run from recent tasks',
  stageFocus: 'This run', stageFull: 'Full workflow', stageOnly: '{stage} stage only',
  stagePassed: 'This {stage} passed', stageFailed: 'This {stage} failed', stageCancelled: 'This {stage} was cancelled',
  stageRunning: 'This {stage} is running', stageUnknown: 'This {stage} is unconfirmed',
  stageScopeHelp: 'This view shows the recorded outcome of one stage. Passing it does not mean the entire task was delivered',
  stageContinue: 'To continue the full workflow', stageFullState: 'Overall delivery: {status}', stageUndelivered: 'Not completed', stageDelivered: 'Completed', stageRecordedScore: 'Recorded score', stageRecordedVerdict: 'Recorded verdict', stageMetadata: 'Run details and file locations',
});

function singleRunKind(control) {
  const state = control.status?.state;
  const kind = state?.run?.kind;
  return ['plan', 'execute', 'verify', 'review', 'integrate', 'final'].includes(kind) && state.control?.kind === kind ? kind : null;
}
function stageName(kind, t) {
  return ({ plan: t('consolePlanSplit'), execute: t('executeRole'), verify: 'L1', review: t('reviewRole'), integrate: t('consoleAggregate'), final: 'L2 / L3 · Judge' })[kind];
}
function singleRunLabel(control, t) {
  const kind = singleRunKind(control), op = control.status?.state?.control;
  const key = control.stale || op?.status === 'running' && !control.status.running ? 'stageUnknown' :
    ({ passed: 'stagePassed', failed: 'stageFailed', cancelled: 'stageCancelled', running: 'stageRunning' }[op?.status] ?? 'stageUnknown');
  return t(key, { stage: stageName(kind, t) });
}
function taskCatalog(runs, association, sessionID) {
  const tasks = new Map();
  for (const run of (runs ?? []).slice(0,100)) {
    if (!record(run) || typeof run.taskDir !== 'string' || !run.taskDir.startsWith('/') || pathProblem(run.taskDir) || typeof run.cwd !== 'string' || !run.cwd.startsWith('/') || pathProblem(run.cwd)) continue;
    const key = selectionKey(run), old = tasks.get(key);
    const belongs = run.hostSessionId === sessionID;
    if (!old || String(run.startedAt ?? '') > String(old.startedAt ?? '')) tasks.set(key, { ...run, belongs: !!(belongs || old?.belongs) });
    else if (belongs) old.belongs = true;
  }
  for (const task of Array.isArray(association?.tasks) ? association.tasks : []) {
    if (task?.outcome !== 'confirmed' || typeof task.taskDir !== 'string' || !task.taskDir.startsWith('/') || pathProblem(task.taskDir) || typeof task.cwd !== 'string' || !task.cwd.startsWith('/') || pathProblem(task.cwd)) continue;
    const key = selectionKey(task), old = tasks.get(key);
    if (old) old.belongs = true;
    else tasks.set(key, { taskDir: task.taskDir, cwd: task.cwd, belongs: true, startedAt: '', summary: { slug: task.taskDir.split('/').at(-1) } });
  }
  return [...tasks.values()].sort((a,b) => Number(b.belongs) - Number(a.belongs) || String(b.startedAt ?? '').localeCompare(String(a.startedAt ?? '')));
}
function TaskChooser({ control, selection, hostSessionID, association, changeSelection, run, discover, t }) {
  const [open, setOpen] = React.useState(false);
  const refreshedCatalog = React.useRef(null);
  const tasks = taskCatalog(control.runs, association, hostSessionID);
  const local = tasks.filter(task => task.belongs);
  const busy = control.pending !== null || control.uncertainty !== null || association && association.outcome !== 'confirmed';
  const choose = task => {
    const next = { taskDir: task.taskDir, cwd: task.cwd };
    changeSelection(next); setOpen(false); void run('status', next);
  };
  React.useEffect(() => {
    if (!selection.taskDir && !busy && local.length === 1) { refreshedCatalog.current = control.runs; choose(local[0]); }
    // A matching run can start while the canvas is open. Read its state instead
    // of interpreting the index itself as a live or completed engine outcome.
    const latest = local.find(task => task.taskDir === (control.status?.taskDir || selection.taskDir) && task.cwd === (control.status?.cwd || selection.cwd));
    if (latest?.id && !busy && refreshedCatalog.current !== control.runs && control.status?.state?.run?.id && latest.id !== control.status.state.run.id) {
      // An index may precede state publication or survive a failed CAS. At most
      // one refresh per catalog response; busy -> idle must not form a loop.
      refreshedCatalog.current = control.runs;
      void run('status', selection);
    }
  }, [control.runs, busy, selection.taskDir, selection.cwd ]);
  // The controller already binds status to the selected request epoch; use
  // its canonical paths rather than comparing a user alias lexically.
  const current = control.status;
  const selected = tasks.find(task => task.taskDir === (current?.taskDir || selection.taskDir) && (!(current?.cwd || selection.cwd) || task.cwd === (current?.cwd || selection.cwd)));
  const linked = association?.taskDir === selection.taskDir ? association : null;
  const taskPath = current?.taskDir || selected?.taskDir || linked?.taskDir || '';
  const project = current?.state?.project?.['workspace-cwd'] || selected?.workspaceCwd || current?.cwd || selected?.cwd || linked?.cwd || control.runsCwd || '';
  const show = open || !selection.taskDir;
  return h('section', { className: css.taskChooser, 'aria-label': t('taskChooser') },
    h('div', { className: css.consoleToolbar },
      h('div', null,
        h('h3', { className: css.heading }, selection.taskDir ? current?.state?.slug || selected?.summary?.slug || selection.taskDir.split('/').filter(Boolean).at(-1) : t('taskNoSelection'))),
      h('div', { className: css.actions },
        selection.taskDir && h(Button, { disabled: busy, onClick: () => setOpen(!open), 'aria-expanded': show }, t('taskSwitch')),
        h(Button, { disabled: control.runsLoading, onClick: discover }, t('taskRefresh')))),
    !selection.taskDir && h('p', { className: css.meta }, t(local.length > 1 ? 'taskMultiple' : 'taskEmptyHelp')),
    control.runsError && h('p', { className: css.errorText, role: 'alert' }, t('taskDiscoveryError')),
    show && tasks.length > 0 && h('label', { className: css.editorField }, t('taskChooser'),
      h('select', { className: css.taskSelect, value: selected ? selectionKey(selected) : '', disabled: busy,
        onChange: event => { const task = tasks.find(task => selectionKey(task) === event.target.value); if (task) choose(task); } },
        h('option', { value: '' }, t('taskPick')),
        ...[true,false].map(belongs => h('optgroup', { key: String(belongs), label: t(belongs ? 'taskThisSession' : 'taskRecent') },
          ...tasks.filter(task => task.belongs === belongs).map(task => h('option', { key: selectionKey(task), value: selectionKey(task) },
            (task.summary?.slug || task.taskDir.split('/').at(-1)) + ' · ' + (task.workspaceCwd || task.cwd).split('/').at(-1) + (task.branch ? ' · ' + task.branch : '') + (stageName(task.kind,t) ? ' · ' + stageName(task.kind,t) : ''))))))),
    show && !control.runsLoading && control.runs !== null && tasks.length === 0 && h('p', { className: css.meta }, t('taskNone')),
    (project || taskPath) && h('details', null,
      h('summary', { className: css.taskLocationSummary }, [project.split('/').filter(Boolean).at(-1), current?.state?.run?.branch || selected?.branch].filter(Boolean).join(' · ') || t('wbPaths')),
      h('dl', { className: css.taskLocations, 'aria-label': t('taskLocation') },
      project && h(React.Fragment, null, h('dt', null, t('taskWorkspace')), h('dd', { 'data-workspace-path': project }, project)),
      current?.cwd && current.cwd !== project && h(React.Fragment, null, h('dt', null, t('executionHome')), h('dd', null, current.cwd)),
      taskPath && h(React.Fragment, null, h('dt', null, t('taskFolderLocation')), h('dd', { 'data-task-path': taskPath },
        project && taskPath.startsWith(project + '/') ? taskPath.slice(project.length + 1) : taskPath)),
      (current?.state?.run?.branch || selected?.branch) && h(React.Fragment, null, h('dt', null, t('consoleBranch')), h('dd', null, current?.state?.run?.branch || selected.branch)))));
}
