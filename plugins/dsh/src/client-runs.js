Object.assign(zh, {
  runsDetails: '运行详情',
  runsNoReference: '不设置对照',
  runsMetricScope: '指标范围',
  runsSelected: '仅此运行',
  runsStart: '开始',
  runsFinish: '结束',
  runsSnapshot: '配置与契约条件',
  runsRead: '查看指标',
  runsNoSelection: '选择一条运行记录',
  runsFailure: '运行失败原因',
  runsCurrent: '当前任务',
  runsMissing: '未报告',
  runsMissingCount: '缺失 {count} 次',
  runsNoMetrics: '读取所选运行指标后显示用量与耗时；缺失费用不记作 0',
  runsTimeline: '阶段时间记录',
  runsTimelineWall: '已观测经过时间',
  runsTimelineSum: '阶段耗时累计',
  runsTimelineSpans: '已闭合 / 未知区间',
  runsTimelineUnmatched: '未关联时间区间的调用',
  runsTimelineHelp: '经过时间取已观测区间的并集；并行和嵌套阶段不能直接相加。未闭合区间保持未知，Provider 内部耗时尚不可见。',
});
Object.assign(en, {
  runsDetails: 'Run details',
  runsNoReference: 'No reference',
  runsMetricScope: 'Metrics scope',
  runsSelected: 'This run only',
  runsStart: 'Started',
  runsFinish: 'Finished',
  runsSnapshot: 'Configuration and contract conditions',
  runsRead: 'Inspect metrics',
  runsNoSelection: 'Select a run',
  runsFailure: 'Run failure',
  runsCurrent: 'Current task',
  runsMissing: 'Not reported',
  runsMissingCount: 'Missing {count}',
  runsNoMetrics: 'Read selected-run metrics to inspect usage and duration. Missing cost is not zero',
  runsTimeline: 'Phase timing',
  runsTimelineWall: 'Observed elapsed time',
  runsTimelineSum: 'Sum of phase durations',
  runsTimelineSpans: 'Closed / unknown intervals',
  runsTimelineUnmatched: 'Calls without a matched interval',
  runsTimelineHelp: 'Elapsed time is the union of observed intervals; parallel and nested phases cannot be added together. Open intervals remain unknown. Provider-internal timing is not observed.',
});

function RunExplorer({ control, selection, run, changeSelection, showCanvas, t, active = false }) {
  const [selected, setSelected] = React.useState(''),
    [reference, setReference] = React.useState('');
  const [metricScope, setMetricScope] = React.useState('run');
  const runs = control.runs ?? [],
    busy = control.pending !== null;
  const item =
    runs.find((r) => r.id === selected) ??
    runs.find((r) => r.id === control.status?.state?.run?.id) ??
    runs[0];
  const baseline = runs.find((r) => r.id === reference);
  const attempted = React.useRef(false);
  React.useEffect(() => {
    if (active && !busy && control.runs === null && !attempted.current) {
      attempted.current = true;
      void run('runs', selection);
    }
  }, [active, busy, control.runs, run]);
  const open = async () => {
    const next = { taskDir: item.taskDir, cwd: item.cwd };
    changeSelection(next);
    showCanvas();
    await run('status', next);
  };
  const metrics = () => {
    const source = item,
      next = source ? { taskDir: source.taskDir, cwd: source.cwd } : selection;
    if (source) changeSelection(next);
    return run('metrics', next, false, { runId: metricScope === 'task' ? undefined : source?.id });
  };
  const property = (label, value) =>
    h(
      React.Fragment,
      { key: label },
      h('dt', null, label),
      h('dd', null, textValue(value, t('runsMissing'))),
    );
  const timeline = control.metrics?.timeline;
  const measuredTime = (value) => timeline?.completeSpans > 0 && Number.isFinite(value)
    ? (value / 1000).toFixed(3) + ' s'
    : t('runsMissing');
  return h(
    'section',
    { className: css.runExplorer, 'aria-label': t('consoleRuns') },
    h(
      'div',
      { className: css.runToolbar },
      h(Button, { disabled: busy, onClick: () => run('runs', selection) }, t('runsRefresh')),
      h('p', { className: css.meta }, t('runsReadOnly')),
    ),
    runs.length
      ? h(
          'div',
          { className: css.runSplit },
          h(
            'nav',
            { className: css.runList, 'aria-label': t('consoleRuns') },
            ...runs.slice(0, 100).map((row) =>
              h(
                'button',
                {
                  key: row.id,
                  type: 'button',
                  className: css.runRow,
                  'aria-pressed': item?.id === row.id,
                  onClick: () => setSelected(row.id),
                },
                h('strong', null, row.summary?.slug ?? row.taskDir),
                h('span', { className: css.meta }, row.startedAt),
                h(
                  'span',
                  { className: css.meta },
                  [
                    row.kind,
                    row.finishedAt
                      ? row.error
                        ? t('fail')
                        : canvasStatus(row.summary?.status, t)
                      : t('runsLive'),
                  ]
                    .filter(Boolean)
                    .join(' · '),
                ),
              ),
            ),
          ),
          item &&
            h(
              'article',
              { className: css.runDetail, 'aria-label': t('runsDetails') },
              h(
                'header',
                { className: css.runTitle },
                h('h3', { className: css.configTitle }, item.summary?.slug ?? item.taskDir),
                h(
                  'span',
                  { className: css.meta },
                  [item.kind, item.branch, item.head?.slice(0, 12)].filter(Boolean).join(' · '),
                ),
              ),
              h(
                'div',
                { className: css.actions },
                h(Button, { disabled: busy || !!control.uncertainty, onClick: open }, t('runsOpen')),
                h(Button, { disabled: busy, onClick: metrics }, t('runsMetrics')),
                h(
                  'label',
                  { className: css.metricScope },
                  t('runsMetricScope'),
                  h(
                    'select',
                    {
                      className: css.modeSelect,
                      value: metricScope,
                      disabled: busy,
                      onChange: (e) => setMetricScope(e.target.value),
                    },
                    h('option', { value: 'run' }, t('runsSelected')),
                    h('option', { value: 'task' }, t('runsAll')),
                  ),
                ),
              ),
              item.error &&
                h(
                  'div',
                  { className: css.configCallout, role: 'note' },
                  h('h4', { className: css.heading }, t('runsFailure')),
                  h(
                    'pre',
                    { className: css.raw },
                    typeof item.error === 'string' ? item.error : JSON.stringify(item.error, null, 2),
                  ),
                ),
              h(
                'dl',
                { className: css.consoleProperties },
                property(t('runsStart'), item.startedAt),
                property(t('runsFinish'), item.finishedAt || t('runsLive')),
                property(t('consoleBranch'), item.branch),
                property(t('projectHome'), item.workspaceCwd),
                property(t('executionHome'), item.cwd),
              ),
              item.quality &&
                item.finishedAt &&
                !item.error &&
                h(
                  'p',
                  null,
                  t('runsObserved', {
                    completed: item.quality.newCompletions,
                    attempts: item.quality.newAttempts,
                    failed: item.quality.newFailedAttempts,
                    seconds: item.elapsedMs == null ? t('runsMissing') : (item.elapsedMs / 1000).toFixed(1),
                  }),
                ),
              h(
                'label',
                { className: css.editorField },
                t('runsReference'),
                h(
                  'select',
                  {
                    className: css.modeSelect,
                    value: baseline?.id ?? '',
                    onChange: (e) => setReference(e.target.value),
                  },
                  h('option', { value: '' }, t('runsNoReference')),
                  ...runs.map((row) =>
                    h(
                      'option',
                      { key: row.id, value: row.id },
                      (row.summary?.slug ?? row.id) + ' · ' + row.startedAt,
                    ),
                  ),
                ),
              ),
              h(
                'p',
                { className: css.meta },
                t('runsCompare') +
                  ' · ' +
                  (baseline
                    ? !item.conditions || !baseline.conditions
                      ? t('runsMissing')
                      : t(item.conditions === baseline.conditions ? 'runsEqual' : 'runsDifferent')
                    : textValue(item.conditions).slice(0, 12)),
              ),
              h(
                'details',
                { className: css.details },
                h('summary', null, 'Run / ' + item.id),
                h('pre', { className: css.raw }, JSON.stringify(item, null, 2)),
              ),
            ),
        )
      : h('div', { className: css.workbenchEmpty }, h('p', null, t('runsEmpty'))),
    control.metrics
      ? h(
          'section',
          { className: css.runMetrics },
          h('p', { className: css.meta }, t('consoleCompareHelp')),
          h('p', { className: css.meta }, t('runsQuality')),
          timeline && h(
            'article',
            { className: css.metricRow },
            h('h4', { className: css.heading }, t('runsTimeline')),
            h('p', { className: css.meta }, t('runsTimelineHelp')),
            h('dl', { className: css.consoleProperties },
              property(t('runsTimelineWall'), measuredTime(timeline.observedWallMs)),
              property(t('runsTimelineSum'), measuredTime(timeline.knownSpanMs)),
              property(t('runsTimelineSpans'), timeline.completeSpans + ' / ' + timeline.unknownSpans),
              property(t('runsTimelineUnmatched'), timeline.unmatchedCalls),
            ),
          ),
          !!(control.metrics.omittedGroups || control.metrics.omittedCalls) &&
            h('p', { className: css.meta }, t('runsBounded')),
          ...(control.metrics.groups ?? []).map((group, index) =>
            h(
              'article',
              { key: index, className: css.metricRow },
              h(
                'h4',
                { className: css.heading },
                [group.role, group.harness, group.model].filter(Boolean).join(' · '),
              ),
              h('p', { className: css.meta }, group.runId || t('runsLegacy')),
              h(
                'dl',
                { className: css.consoleProperties },
                property(t('runsSamples'), group.calls + ' / ' + group.failedCalls),
                property(
                  t('runsLatency'),
                  (group.knownDurationMs / 1000).toFixed(1) +
                    ' s · ' +
                    t('runsMissingCount', { count: group.missingDuration }),
                ),
                property(
                  t('runsUsage'),
                  group.knownTokens +
                    ' / ' +
                    group.missingUsage +
                    ' · ' +
                    t('runsPartial', { count: group.partialUsage ?? 0 }),
                ),
                property(
                  t('runsCost'),
                  (group.missingCost === group.calls
                    ? t('runsMissing')
                    : '$' + group.knownCostUSD.toFixed(4)) +
                    ' / ' +
                    group.missingCost,
                ),
              ),
            ),
          ),
        )
      : h('p', { className: css.runMetricsHint }, t('runsNoMetrics')),
  );
}
