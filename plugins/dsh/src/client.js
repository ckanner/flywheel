// The build wraps this file in rc.5's browser ModuleLoader factory.
const React = require('react');
const h = React.createElement;

const taskLabels = {
  pending: '待开始', paused: '已暂停', stopped: '已停止', completed: '已完成',
  escalated: '需要人工处理', 'budget-stopped': '预算已停止',
};
const packetLabels = {
  pending: '待执行', 'awaiting-credentials': '等待凭证', executing: '执行中', executed: '待验收',
  'l1-passed': '验收已通过', 'l1-failed': '验收未通过', 'review-failed': '评审退回',
  completed: '已完成', interrupted: '已中断', escalated: '需要人工处理', inbox: '等待处理',
};
const checkLabels = { pass: '通过', fail: '失败', warn: '注意', unknown: '未验证', skipped: '跳过' };
const roleLabels = { plan: '规划', execute: '执行', review: '评审', judge: '终验',
  intake: '需求收集', garden: '知识维护', 'judge-panel': '第二意见' };
const buttonStyle = { border: '1px solid #8888', padding: '4px 10px', borderRadius: 5 };
const tableStyle = { width: '100%', textAlign: 'left', fontSize: 11, borderCollapse: 'collapse' };
const cellStyle = { padding: '5px 8px 5px 0', verticalAlign: 'top', overflowWrap: 'anywhere' };
const rawStyle = { fontSize: 11, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 240, overflow: 'auto' };

function taskLabel(value) {
  if (value.running) return '运行中';
  if (!value.state) return '尚未初始化';
  if (value.state.status === 'running') return '引擎未运行，状态待核对';
  return taskLabels[value.state.status] ?? `未知状态（${value.state.status ?? '未报告'}）`;
}

function Configuration({ value }) {
  if (!value) return null;
  return h('section', { 'aria-label': '实际模型配置' },
    h('h4', { style: { marginBottom: 6 } }, '实际模型配置'),
    h('p', { style: { overflowWrap: 'anywhere' } }, `来源：${value.source || '内嵌模板'} · 范围：${value.scope}`),
    h('div', { style: { overflowX: 'auto' } }, h('table', { style: tableStyle },
      h('thead', null, h('tr', null, ...['阶段 / 路由', '模型 / Profile', 'Harness / Provider / 渠道', '推理 / 阶段权限要求 / 会话']
        .map(title => h('th', { key: title, scope: 'col', style: cellStyle }, title)))),
      h('tbody', null, ...value.bindings.map(binding => h('tr', { key: binding.id },
        h('td', { style: cellStyle }, h('strong', null, roleLabels[binding.role] ?? binding.role),
          h('div', null, binding.route || binding.id),
          binding.inScope === false && h('div', null, '未用于本次检查')),
        h('td', { style: cellStyle }, h('code', null, binding.model || '未配置'),
          binding.profile && h('div', null, `Profile：${binding.profile}`)),
        h('td', { style: cellStyle }, h('div', null, binding.harness),
          h('div', null, `Provider：${binding.provider || (binding.harness === 'dsh' ? '未配置' : '由原生 CLI 管理')}`),
          h('div', null, `渠道：${binding.channel || '未配置'}`)),
        h('td', { style: cellStyle }, h('div', null, `推理：${binding.thinking || '默认'}`),
          h('div', null, `阶段权限要求：${binding.permissionMode || '未声明'}`),
          h('div', null, binding.residentSession ? '会话恢复：启用' : '会话恢复：关闭'))))))));
}

function Diagnostic({ value }) {
  if (!value) return null;
  return h('section', { 'aria-label': '环境检查结果' },
    h('h4', { style: { marginBottom: 6 } }, value.ok ? '检查结束：未发现阻断项' : '检查结束：存在阻断项'),
    h('p', null, value.probeRequested ? '已请求 SDK 无推理检查。' : '本次只检查本地环境，未启动 SDK 探测。'),
    h('p', null, '未发送推理请求；账号授权、额度和真实模型能力未通过本次检查验证。'),
    h('ul', { style: { paddingLeft: 20 } }, ...value.checks.map((check, index) =>
      h('li', { key: `${check.id}:${index}`, style: { marginBottom: 9, overflowWrap: 'anywhere' } },
        h('strong', null, `${checkLabels[check.status]} · ${check.id}`),
        h('div', null, check.summary),
        check.code && h('div', null, `错误码：${check.code}`),
        check.remedy && h('div', null, `处理建议：${check.remedy}`)))));
}

function Budget({ value }) {
  if (!value) return null;
  const cost = value.totals?.['cost-usd'];
  const unknownCost = value['unknown-cost-calls'] ?? 0;
  const unknownUsage = value['unknown-usage-calls'] ?? 0;
  let costText = Number.isFinite(cost) ? `已报告成本合计：$${cost.toFixed(4)}` : '成本：未知';
  if (unknownCost > 0) {
    costText = Number.isFinite(cost) && cost > 0 ?
      `已知成本合计：$${cost.toFixed(4)}；另有 ${unknownCost} 次调用成本未知，合计不完整` :
      `成本：未知（${unknownCost} 次调用未报告结算成本）`;
  }
  return h('section', { 'aria-label': '用量与成本' },
    h('p', null, costText),
    h('p', null, `已报告 Token：${value.totals?.tokens ?? '未知'}` +
      (unknownUsage > 0 ? `；${unknownUsage} 次调用用量未知，合计不完整` : '')),
    value['daily-token-limit'] != null && h('p', null,
      `每日 Token 限额：${value['daily-token-limit']} · 预算策略：${value.enforce}`),
    ...(value.warnings ?? []).map((warning, index) => h('p', { key: index }, `预算提示：${warning}`)));
}

function Panel({ execute }) {
  const [open, setOpen] = React.useState(false);
  const [taskDir, setTaskDir] = React.useState('');
  const [cwd, setCwd] = React.useState('');
  const [busy, setBusy] = React.useState(false);
  const [status, setStatus] = React.useState(null);
  const [configuration, setConfiguration] = React.useState(null);
  const [diagnostic, setDiagnostic] = React.useState(null);
  const [raw, setRaw] = React.useState(null);
  const [error, setError] = React.useState('');
  const requestVersion = React.useRef(0);
  const panelId = React.useId();
  React.useEffect(() => () => { requestVersion.current += 1; }, []);
  const changePath = (setPath, value) => {
    // Editing a path changes the view's identity, even while a request is in
    // flight. It never implies stopping an independently running Go task.
    requestVersion.current += 1;
    setPath(value);
    setBusy(false);
    setStatus(null);
    setConfiguration(null);
    setDiagnostic(null);
    setRaw(null);
    setError('');
  };
  const packets = Object.entries(status?.state?.packets ?? {});
  const run = async (action, probe = false) => {
    const version = ++requestVersion.current;
    setBusy(true);
    setError('');
    try {
      const request = { action, ...(taskDir.trim() ? { taskDir: taskDir.trim() } : {}),
        ...(cwd.trim() ? { cwd: cwd.trim() } : {}), ...(probe ? { probe: true } : {}) };
      const value = await execute(request);
      if (version !== requestVersion.current) return;
      if (['start', 'status', 'stop'].includes(action)) setStatus(value);
      if (['config', 'doctor'].includes(action)) {
        setConfiguration(value.configuration ?? value.diagnostic?.configuration ?? null);
        setDiagnostic(value.diagnostic ?? null);
      }
      setRaw(value);
    } catch (error) {
      if (version === requestVersion.current) setError(error instanceof Error ? error.message : String(error));
    } finally {
      if (version === requestVersion.current) setBusy(false);
    }
  };
  const button = (action, label, needsTask, probe = false) => h('button', {
    key: label, type: 'button', disabled: busy || (needsTask && !taskDir.trim()),
    onClick: () => run(action, probe), style: buttonStyle,
  }, label);
  return h('div', { style: { position: 'relative', fontSize: 12 } },
    h('button', { type: 'button', onClick: () => setOpen(!open), 'aria-expanded': open, 'aria-controls': panelId,
      style: { border: '1px solid currentColor', borderRadius: 6, padding: '2px 8px' } },
    `Flywheel${status ? ` · ${taskLabel(status)}` : ''}`),
    open && h('section', { id: panelId, 'aria-label': 'Flywheel 任务控制', style: {
      position: 'absolute', bottom: 'calc(100% + 8px)', left: 0, width: 'min(720px, 88vw)',
      maxHeight: '70vh', overflow: 'auto', padding: 14, border: '1px solid #8888', borderRadius: 10,
      background: 'var(--color-background, Canvas)', color: 'var(--color-foreground, CanvasText)',
      boxShadow: '0 8px 32px #0003', zIndex: 40,
    } },
    h('strong', null, 'Flywheel · Go 引擎'),
    h('p', { style: { margin: '8px 0' } }, '任务独立于桌面会话运行。切换下方路径只切换查看对象；停止任务请使用“停止”。'),
    h('label', { style: { display: 'block' } }, '任务目录（绝对路径，配置与环境检查可留空）',
      h('input', { value: taskDir, onChange: e => changePath(setTaskDir, e.target.value),
        placeholder: '/path/to/repo/docs/task',
        style: { width: '100%', boxSizing: 'border-box', padding: 6, border: '1px solid #8888', borderRadius: 4, margin: '4px 0 8px' } })),
    h('label', { style: { display: 'block' } }, '仓库目录（留空使用当前 DSH 会话目录）',
      h('input', { value: cwd, onChange: e => changePath(setCwd, e.target.value), placeholder: '/path/to/repo',
        style: { width: '100%', boxSizing: 'border-box', padding: 6, border: '1px solid #8888', borderRadius: 4, margin: '4px 0 8px' } })),
    h('div', { style: { display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 8 } },
      button('status', '刷新状态', true), button('start', '启动', true), button('stop', '停止', true)),
    h('div', { style: { display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 8 } },
      button('config', '查看模型配置', false), button('doctor', '检查环境', false),
      button('doctor', 'SDK 无推理检查', false, true)),
    busy && h('p', { role: 'status' }, '正在等待引擎确认…'),
    error && h('p', { role: 'alert', style: { color: '#d34b4b', whiteSpace: 'pre-wrap' } }, error),
    status && h('section', { 'aria-label': '任务状态' },
      h('h4', { style: { marginBottom: 6 } }, '任务状态（上次刷新）'),
      h('p', null, `${status.state?.slug ?? '任务'} · ${taskLabel(status)}` + (status.running && status.pid > 0 ? ` · PID ${status.pid}` : '')),
      status.state?.['current-milestone'] && h('p', null, `当前里程碑：${status.state['current-milestone']}`),
      packets.length > 0 && h('div', { style: { maxHeight: 260, overflow: 'auto' } },
        h('table', { 'aria-label': '工作单元', style: tableStyle },
          h('caption', { style: { textAlign: 'left' } }, `工作单元（共 ${packets.length} 个）`),
          h('thead', null, h('tr', null, ...['工作单元', '状态', '路由'].map(title => h('th', { key: title, scope: 'col', style: cellStyle }, title)))),
          h('tbody', null, ...packets.map(([id, packet]) => h('tr', { key: id },
            h('td', { style: cellStyle }, id),
            h('td', { style: cellStyle }, packetLabels[packet?.status] ?? packet?.status ?? '未知'),
            h('td', { style: cellStyle }, packet?.route ?? '—')))))),
      h(Budget, { value: status.state?.budget }),
      status.state?.inbox?.length > 0 && h('details', null,
        h('summary', null, `待人工处理记录：${status.state.inbox.length} 条`),
        h('pre', { style: rawStyle }, JSON.stringify(status.state.inbox, null, 2)))),
    h(Diagnostic, { value: diagnostic }),
    h(Configuration, { value: configuration }),
    raw && h('details', null, h('summary', null, '引擎原始结果（JSON）'),
      h('pre', { style: rawStyle }, JSON.stringify(raw, null, 2))),
    ));
}

exports.inject = ['slots', 'remote', 'remote.commands'];
exports.apply = function apply(ctx) {
  ctx.slots.inject('conversation.composer.dock', () => ctx.slots.register({
    name: 'conversation.composer.dock', id: 'flywheel', order: 50,
    inject: sessionId => ({
      execute: async request => {
        const reply = await ctx.remote.commands.execute(sessionId, `/flywheel ${JSON.stringify(request)}`, []);
        if (!reply?.ok) throw new Error(reply?.error?.message ?? 'DSH command transport failed');
        const result = reply.value?.result;
        if (result?.kind !== 'success') throw new Error(result?.text ?? 'Flywheel command unavailable');
        const value = JSON.parse(result.text);
        if (value?.protocol !== 1) throw new Error('Flywheel control protocol mismatch');
        return value;
      },
    }),
  }, Panel));
};
