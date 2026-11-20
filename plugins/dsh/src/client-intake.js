Object.assign(zh, {
  images: '检查图片证据', intakeTitle: '创建任务', intakeGoal: '目标与问题现象', intakeKind: '这次要做什么',
  intakeEngineering: '工程任务', intakeRepair: '修复问题', intakeInvestigate: '排查原因（只生成报告）',
  intakeConstraints: '约束（每行一项，可选）', intakeEvidence: '已有证据（每行一项，可选）',
  intakePolicy: '方案授权', intakeReviewFirst: '方案审查后等我批准', intakeDelegated: '授权审查通过后继续实施',
  intakeDelegatedConfirm: '我授权在本次目标和约束内，方案独立审查通过后继续实施',
  intakeReviewHelp: '先生成方案并独立审查，然后停下来等待你批准这个版本。',
  intakeAdvanced: '高级选项', intakeDepth: '实施深度', intakeAuto: 'Auto · 根据已审契约选择',
  intakeQuick: 'Quick · 局部任务', intakeFull: 'Full · 完整工程流程',
  intakeCreate: '开始设计与审查', intakeCreateInvestigation: '创建并开始调查',
  intakeResume: '继续创建任务', intakeResumeHelp: '创建尚未完成。核对状态后，可在原任务继续；已输入的内容会保留。',
  intakeNoWorkspace: '正在取得当前会话的项目目录；取得后即可创建任务。',
  intakeLocation: '任务记录将由引擎在当前会话目录的 docs 下创建。',
  intakeExpected: '预期行为', intakeHypothesis: '待验证的假设', intakeFalsifier: '什么证据能推翻假设',
  intakeReadPaths: '允许读取的文件（相对会话工作目录，每行一个，最多 8 个）',
  intakeInvestigationHelp: '本入口只读取你列出的文件，最多 8 次、60 秒，每个文件最多 256 KiB；不运行命令或访问服务。报告完成后停止。更复杂的诊断可通过 /flywheel-task 准备有界调查契约。',
  intakeInvalid: '请补全目标和必填项，检查路径、长度和授权选项。',
  intakeImages: '原始图片证据（可选）', intakeImagePaths: '图片路径（仓库相对路径，每行一个，最多 4 张）',
  intakeInspectImages: '检查并预览选定图片', intakeImageRemove: '移除图片选择',
  intakeImageHelp: '没有图片也可以正常使用 Gemini。只有你明确选择的仓库图片才会被读取；聊天附件不会自动加入。支持 PNG/JPEG/GIF/WebP，每张最多 4 MiB，合计最多 8 MiB。',
  intakeImagePending: '这是选定图片的摘要，尚未成为已审执行契约。方案阶段收到路径和摘要；原图必须经后续 packet 审查并使用支持图片的 Claude SDK 通道。当前 Antigravity 自动化入口只接收文本。',
  intakeImageStale: '图片路径已改变或尚未检查，请先检查预览。', intakeImageFailed: '图片检查未完成；未加入任务。',
  intakeImagePreview: '选定图片的缩略图：{path}', intakeImageDimensions: '{width} × {height} · {bytes} bytes',
  intakeTextEvidence: '排障入口收集文本证据；需要原始图片时，请使用修复或工程任务入口准备方案。',
});
Object.assign(en, {
  images: 'Inspect image evidence', intakeTitle: 'Create a task', intakeGoal: 'Goal and observed problem', intakeKind: 'What do you want to do?',
  intakeEngineering: 'Engineering task', intakeRepair: 'Fix a problem', intakeInvestigate: 'Investigate the cause (report only)',
  intakeConstraints: 'Constraints (one per line, optional)', intakeEvidence: 'Existing evidence (one per line, optional)',
  intakePolicy: 'Design authorization', intakeReviewFirst: 'Wait for my approval after review', intakeDelegated: 'Authorize implementation after review',
  intakeDelegatedConfirm: 'I authorize implementation within this goal and its constraints after independent design review passes',
  intakeReviewHelp: 'Produce a design and independent review, then wait for you to approve that exact version.',
  intakeAdvanced: 'Advanced options', intakeDepth: 'Implementation depth', intakeAuto: 'Auto · select from reviewed contracts',
  intakeQuick: 'Quick · localized task', intakeFull: 'Full · complete engineering workflow',
  intakeCreate: 'Start design and review', intakeCreateInvestigation: 'Create and start investigation',
  intakeResume: 'Continue task creation', intakeResumeHelp: 'Creation has not completed. Reconcile status, then continue in the original task; your draft is retained.',
  intakeNoWorkspace: 'Reading this conversation’s workspace. Task creation becomes available when it is known.',
  intakeLocation: 'The engine will create task records under docs in this conversation’s working directory.',
  intakeExpected: 'Expected behavior', intakeHypothesis: 'Hypothesis to test', intakeFalsifier: 'Evidence that would disprove it',
  intakeReadPaths: 'Files to read (relative to the conversation’s working directory, one per line, up to 8)',
  intakeInvestigationHelp: 'Read only the files you list: up to 8 actions, 60 seconds, and 256 KiB per file. No commands or service access. Stop after the report. Use /flywheel-task to prepare a bounded contract for more complex diagnostics.',
  intakeInvalid: 'Complete the goal and required fields, and check paths, lengths, and authorization.',
  intakeImages: 'Original image evidence (optional)', intakeImagePaths: 'Image paths (repository-relative, one per line, up to 4)',
  intakeInspectImages: 'Inspect and preview selected images', intakeImageRemove: 'Remove image selection',
  intakeImageHelp: 'Gemini works for tasks without images. Only explicitly selected repository files are read; chat attachments are not added automatically. PNG/JPEG/GIF/WebP, up to 4 MiB each and 8 MiB combined.',
  intakeImagePending: 'These are selected image digests, not approved execution inputs. Planning receives paths and digests. Original images require a reviewed packet and the supported Claude SDK transport. The current automated Antigravity input accepts text only.',
  intakeImageStale: 'Image paths changed or have not been inspected. Inspect the preview first.', intakeImageFailed: 'Image inspection did not complete. No images were added to the task.',
  intakeImagePreview: 'Thumbnail of selected image: {path}', intakeImageDimensions: '{width} × {height} · {bytes} bytes',
  intakeTextEvidence: 'This investigation entry gathers text evidence. Use a repair or engineering brief when original images are required.',
});

function intakeLines(value) { return value.split('\n').map(line => line.trim()).filter(Boolean); }
function intakeLocalPaths(paths, maximum) {
  return paths.length > 0 && paths.length <= maximum && new Set(paths).size === paths.length &&
    paths.every(path => path.length <= 512 && !path.startsWith('/') && !/[\\:\p{C}]/u.test(path) && path.split('/').every(part => part && !['.', '..', '.git'].includes(part)));
}
function TaskIntake({ control, selection, intakeStore, changeSelection, run, t }) {
  const project = selection.cwd || control.runsCwd || '';
  const createDraft = () => ({ id: 'task-' + planningRequestID(), kind: 'engineering', goal: '', constraints: '', evidence: '', policy: 'review-first', delegated: false, mode: 'auto', expected: '', hypothesis: '', falsifier: '', readPaths: '', imagePaths: '' });
  const [draft, setDraft] = React.useState(() => intakeStore.cwd === project && intakeStore.draft || createDraft());
  const [images, setImages] = React.useState(null), [error, setError] = React.useState('');
  const [inspectingImages, setInspectingImages] = React.useState(false);
  const generation = React.useRef(0);
  React.useEffect(() => () => { generation.current++; }, []);
  React.useEffect(() => {
    generation.current++; setImages(null); setInspectingImages(false);
    if (intakeStore.cwd !== project) {
      intakeStore.cwd = project; intakeStore.target = null; intakeStore.draft = createDraft(); setDraft(intakeStore.draft);
    }
  }, [project]);
  const confirmedEmpty = control.status?.taskDir === selection.taskDir && control.status?.running === false && control.status.state === null && !control.status.preparation;
  const notDispatched = ['planning', 'investigate'].includes(control.error?.action) && control.error.dispatch === 'not-dispatched';
  const busy = !!control.pending || !!control.uncertainty || !!control.configUncertainty || !!control.stale || inspectingImages || !!selection.taskDir && !confirmedEmpty && !notDispatched;
  const workspaceReady = typeof project === 'string' && project.startsWith('/') && !pathProblem(project);
  const update = (key, value) => { setError(''); if (key === 'imagePaths' || key === 'kind') { generation.current++; setImages(null); } setDraft(previous => { const next = { ...previous, [key]: value }; intakeStore.draft = next; return next; }); };
  const field = (label, key, maximum = 4096, rows = 2) => h('label', { className: css.intakeField }, h('span', null, t(label)),
    h('textarea', { 'aria-label': t(label), value: draft[key], rows, maxLength: maximum, disabled: busy, onChange: event => update(key, event.target.value) }));
  const select = (label, key, options) => h('label', { className: css.intakeField }, h('span', null, t(label)),
    h('select', { 'aria-label': t(label), value: draft[key], disabled: busy, onChange: event => update(key, event.target.value) }, ...options.map(([value, label]) => h('option', { key: value, value }, t(label)))));
  const paths = intakeLines(draft.imagePaths), isInvestigation = draft.kind === 'investigate';
  const imagesCurrent = paths.length > 0 && images && validImageInspection(images, paths);
  const inspect = async () => {
    if (busy || !workspaceReady || !validImageSelection({ paths })) return;
    const token = ++generation.current;
    setError(''); setImages(null); setInspectingImages(true);
    try {
      const result = await run('images', { taskDir: selection.taskDir || '', cwd: project }, false, { images: { paths } });
      if (token !== generation.current) return;
      if (result && validImageInspection(result.inputImages, paths)) setImages(result.inputImages);
      else setError('intakeImageFailed');
    } finally { if (token === generation.current) setInspectingImages(false); }
  };
  const submit = async event => {
    event.preventDefault();
    if (busy || !workspaceReady) return;
    const evidence = [...new Set(intakeLines(draft.evidence))], constraints = intakeLines(draft.constraints), readPaths = intakeLines(draft.readPaths);
    if (!draft.goal.trim() || evidence.length > 32 || evidence.some(line => line.length > 2048) || !isInvestigation && constraints.length > 32 ||
        isInvestigation && (!draft.expected.trim() || !draft.hypothesis.trim() || !draft.falsifier.trim() || !intakeLocalPaths(readPaths, 8)) ||
        !isInvestigation && (draft.policy === 'delegated' && !draft.delegated || paths.length && !imagesCurrent)) {
      setError(!isInvestigation && paths.length && !imagesCurrent ? 'intakeImageStale' : 'intakeInvalid'); return;
    }
    const next = intakeStore.target || { taskDir: project.replace(/\/$/, '') + '/docs/' + draft.id, cwd: project };
    intakeStore.target = next; intakeStore.draft = draft;
    let result;
    if (isInvestigation) {
      const contract = { version: 1, id: draft.id, goal: draft.goal.trim(), expected: draft.expected.trim(), intent: 'investigate',
        environment: 'Local repository only; no commands or service access.', window: 'The current checked-out version.',
        'known-evidence': evidence, 'read-paths': readPaths, 'implementation-scope': [],
        hypotheses: [{ id: 'hypothesis-1', claim: draft.hypothesis.trim(), falsifier: draft.falsifier.trim() }],
        actions: readPaths.map((path, index) => ({ id: 'read-' + (index + 1), kind: 'read_file', path, purpose: 'Test the user-provided hypothesis against this explicitly selected file.', hypotheses: ['hypothesis-1'] })),
        limits: { 'max-actions': readPaths.length, 'max-seconds': 60, 'max-file-bytes': 262144, 'max-output-bytes': 16384 } };
      result = await run('investigate', next, false, { investigation: { action: 'start', contract, run: true } });
    } else {
      const chosen = imagesCurrent ? [JSON.stringify({ 'selected-input-images': images.references, status: 'unreviewed-selection', 'planning-consumption': 'paths-and-digests-only' })] : [];
      const brief = { version: 1, id: draft.id, goal: draft.goal.trim(), 'original-request': draft.goal.trim(),
        constraints: ['Task intent: ' + draft.kind, ...constraints, ...(chosen.length ? ['Carry the explicitly selected input-images references into the relevant reviewed packet. Original images require the supported Claude SDK transport. Do not claim planning or review consumed these images from this metadata.'] : [])],
        evidence: [...evidence, ...chosen], policy: draft.policy, mode: draft.mode,
        'implementation-authorized': draft.policy === 'delegated',
        ...(draft.policy === 'delegated' ? { authorization: 'User selected and confirmed implementation after independent review in the Flywheel task form. Goal: ' + draft.goal.trim() } : {}) };
      result = await run('planning', next, false, { planning: { action: 'prepare', 'request-id': planningRequestID(), brief, start: true } });
    }
    // Never claim that a new task exists before a confirmed Go receipt. Unknown
    // outcomes retain the controller's original-task reconciliation barrier.
    if (result) {
      intakeStore.target = null; intakeStore.draft = null;
      changeSelection({ taskDir: result.taskDir, cwd: result.cwd || next.cwd });
    }
  };
  return h('form', { className: css.intake, onSubmit: submit, 'aria-label': t('intakeTitle') },
    h('h3', { className: css.heading }, t('intakeTitle')),
    select('intakeKind', 'kind', [['engineering', 'intakeEngineering'], ['repair', 'intakeRepair'], ['investigate', 'intakeInvestigate']]),
    field('intakeGoal', 'goal', 4096, 3),
    isInvestigation ? h(React.Fragment, null, field('intakeExpected', 'expected'), field('intakeHypothesis', 'hypothesis', 2048), field('intakeFalsifier', 'falsifier', 2048), field('intakeReadPaths', 'readPaths', 4096),
      h('p', { className: css.settingsHelp }, t('intakeInvestigationHelp')), h('p', { className: css.settingsHelp }, t('intakeTextEvidence')))
      : h(React.Fragment, null, field('intakeConstraints', 'constraints', 8192),
        select('intakePolicy', 'policy', [['review-first', 'intakeReviewFirst'], ['delegated', 'intakeDelegated']]),
        draft.policy === 'review-first' ? h('p', { className: css.settingsHelp }, t('intakeReviewHelp')) : h('label', { className: css.intakeCheck },
          h('input', { type: 'checkbox', checked: draft.delegated, disabled: busy, onChange: event => update('delegated', event.target.checked) }), t('intakeDelegatedConfirm')),
        h('details', { className: css.intakeAdvanced }, h('summary', null, t('intakeAdvanced')), select('intakeDepth', 'mode', [['auto', 'intakeAuto'], ['quick', 'intakeQuick'], ['full', 'intakeFull']]))),
    field('intakeEvidence', 'evidence', 8192),
    !isInvestigation && h('section', { className: css.intakeImages, 'aria-label': t('intakeImages') },
      h('h4', null, t('intakeImages')), h('p', { className: css.settingsHelp }, t('intakeImageHelp')),
      field('intakeImagePaths', 'imagePaths', 2048),
      h('div', { className: css.actions }, h(Button, { disabled: busy || !workspaceReady || !validImageSelection({ paths }), onClick: inspect }, t('intakeInspectImages')),
        paths.length > 0 && h(Button, { disabled: busy, onClick: () => update('imagePaths', '') }, t('intakeImageRemove'))),
      imagesCurrent && h(React.Fragment, null, h('p', { className: css.notice }, t('intakeImagePending')),
        h('div', { className: css.intakePreviews }, ...images.previews.map(item => h('figure', { key: item.path, className: css.intakePreview },
          h('img', { src: item.dataUrl, alt: t('intakeImagePreview', { path: item.path }) }),
          h('figcaption', null, h('code', null, item.path), h('span', null, item.mimeType + ' · ' + t('intakeImageDimensions', item)), h('code', null, 'SHA-256 ' + item.sha256))))))),
    error && h('p', { role: 'alert', className: css.errorText }, t(error)),
    !workspaceReady && h('p', { role: 'status', className: css.settingsHelp }, t('intakeNoWorkspace')),
    h('p', { className: css.settingsHelp }, t('intakeLocation')),
    h(Button, { type: 'submit', variant: 'primary', disabled: busy || !workspaceReady || !draft.goal.trim() || !isInvestigation && draft.policy === 'delegated' && !draft.delegated }, t(isInvestigation ? 'intakeCreateInvestigation' : 'intakeCreate')));
}
