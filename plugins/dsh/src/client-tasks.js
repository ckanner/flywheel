// Task discovery consumes Go's registered runs; no directory scanning or shell parsing.
Object.assign(zh, {
  taskSelected: '当前任务', taskNoSelection: '还没有关联任务',
  taskEmptyHelp: '此视图只显示当前会话关联的 Flywheel 任务。使用 /flywheel-task 准备并运行后，画布会自动出现；加载 Skill 本身不会启动引擎',
  taskDiscoveryError: '当前会话任务读取失败', taskRequired: '当前会话尚未关联任务', retry: '重试',
  taskWorkspace: '工作区', taskLocation: '任务位置（自动取得）', taskFolderLocation: '任务记录',
  emptyBody: '在当前会话中使用 /flywheel-task 准备并运行任务',
  stageFocus: '本次运行', stageFull: '完整流程', stageOnly: '仅运行{stage}阶段',
  stagePassed: '本次{stage}已通过', stageFailed: '本次{stage}未通过', stageCancelled: '本次{stage}已取消',
  stageRunning: '本次{stage}进行中', stageUnknown: '本次{stage}结果待核对',
  stageScopeHelp: '这里展示这次单阶段运行的回执。通过该阶段不代表完整任务已交付',
  stageContinue: '需要继续完整流程时', stageFullState: '整体交付：{status}', stageUndelivered: '尚未完成', stageDelivered: '已完成', stageRecordedScore: '记录评分', stageRecordedVerdict: '记录结论', stageMetadata: '运行记录与文件位置',
});
Object.assign(en, {
  taskSelected: 'Current task', taskNoSelection: 'No task associated yet',
  taskEmptyHelp: 'This view follows the current conversation only. Use /flywheel-task to prepare and run a task; its canvas appears automatically. Loading a skill alone does not start the engine',
  taskDiscoveryError: 'Could not read this conversation’s task', taskRequired: 'No task is associated with this conversation', retry: 'Retry',
  taskWorkspace: 'Workspace', taskLocation: 'Task location (automatic)', taskFolderLocation: 'Task records',
  emptyBody: 'Use /flywheel-task in this conversation to prepare and run a task',
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
    if (run?.hostSessionId !== sessionID || !sessionID || !record(run) || typeof run.taskDir !== 'string' || !run.taskDir.startsWith('/') || pathProblem(run.taskDir) || typeof run.cwd !== 'string' || !run.cwd.startsWith('/') || pathProblem(run.cwd)) continue;
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
  return [...tasks.values()].sort((a,b) => Number(b.belongs) - Number(a.belongs) || String(b.startedAt ?? '').localeCompare(String(a.startedAt ?? '')) || selectionKey(a).localeCompare(selectionKey(b)));
}
function TaskChooser({ control, selection, hostSessionID, association, changeSelection, run, discover, t }) {
  const refreshedCatalog = React.useRef(null);
  const catalogSelection = React.useRef(null);
  const tasks = taskCatalog(control.runs, association, hostSessionID).filter(task => task.belongs);
  const busy = control.pending !== null || control.uncertainty !== null || association && association.outcome !== 'confirmed';
  React.useEffect(() => {
    // An explicit session projection wins. CLI-only runs need an exact host ID;
    // choose the latest deterministically, never a global or directory guess.
    if (!association && !busy && tasks.length && (!selection.taskDir || catalogSelection.current === selectionKey(selection) && selectionKey(tasks[0]) !== catalogSelection.current)) {
      refreshedCatalog.current = control.runs;
      const next = { taskDir: tasks[0].taskDir, cwd: tasks[0].cwd };
      catalogSelection.current = selectionKey(next);
      changeSelection(next); void run('status', next);
    }
    const latest = tasks.find(task => task.taskDir === (control.status?.taskDir || selection.taskDir) && task.cwd === (control.status?.cwd || selection.cwd));
    if (latest?.id && !busy && refreshedCatalog.current !== control.runs && control.status?.state?.run?.id && latest.id !== control.status.state.run.id) {
      refreshedCatalog.current = control.runs;
      void run('status', selection);
    }
  }, [control.runs, busy, selection.taskDir, selection.cwd, association]);
  const current = control.status;
  const selected = tasks.find(task => task.taskDir === (current?.taskDir || selection.taskDir) && task.cwd === (current?.cwd || selection.cwd));
  const linked = association?.taskDir === selection.taskDir ? association : null;
  const taskPath = current?.taskDir || selected?.taskDir || linked?.taskDir || '';
  const project = current?.state?.project?.['workspace-cwd'] || selected?.workspaceCwd || current?.cwd || selected?.cwd || linked?.cwd || control.runsCwd || '';
  return h('section', { className: css.taskChooser, 'aria-label': t('taskSelected') },
    h('div', { className: css.taskContext },
      h('span', null, project.split('/').filter(Boolean).at(-1) || t('taskThisSession')),
      h('span', null, ' / ' + t('taskThisSession')),
      (current?.state?.run?.branch || selected?.branch) && h('code', null, current?.state?.run?.branch || selected.branch)),
    h('h3', { className: css.heading }, selection.taskDir ? current?.state?.slug || selected?.summary?.slug || selection.taskDir.split('/').filter(Boolean).at(-1) : 'Flywheel'),
    control.runsError && h('div', { className: css.actions, role: 'alert' },
      h('span', { className: css.errorText }, t('taskDiscoveryError')),
      h(Button, { disabled: control.runsLoading, onClick: discover }, t('retry'))));
}

function TaskLocations({ control, selection, association, t }) {
  const current = control.status;
  const taskPath = current?.taskDir || association?.taskDir || selection.taskDir;
  const project = current?.state?.project?.['workspace-cwd'] || current?.cwd || association?.cwd || control.runsCwd;
  if (!project && !taskPath) return null;
  return h('footer', { className: css.taskLocations, 'aria-label': t('taskLocation') },
    project && h('span', null, t('taskWorkspace'), ' ', h('code', { 'data-workspace-path': project }, project)),
    current?.cwd && current.cwd !== project && h('span', null, t('executionHome'), ' ', h('code', null, current.cwd)),
    taskPath && h('span', null, t('taskFolderLocation'), ' ', h('code', { 'data-task-path': taskPath }, project && taskPath.startsWith(project + '/') ? taskPath.slice(project.length + 1) : taskPath)));
}
