// The build wraps this file in rc.5's browser ModuleLoader factory.
const React = require('react');
const h = React.createElement;

function Panel({ execute }) {
  const [open, setOpen] = React.useState(false);
  const [taskDir, setTaskDir] = React.useState('');
  const [cwd, setCwd] = React.useState('');
  const [busy, setBusy] = React.useState(false);
  const [status, setStatus] = React.useState(null);
  const [error, setError] = React.useState('');
  const packets = Object.entries(status?.state?.packets ?? {});
  const run = async action => {
    setBusy(true);
    setError('');
    try {
      const request = { action, taskDir: taskDir.trim(), ...(cwd.trim() ? { cwd: cwd.trim() } : {}) };
      const value = await execute(request);
      setStatus(value);
    } catch (error) {
      setError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  };
  return h('div', { style: { position: 'relative', fontSize: 12 } },
    h('button', { type: 'button', onClick: () => setOpen(!open), 'aria-expanded': open,
      style: { border: '1px solid currentColor', borderRadius: 6, padding: '2px 8px' } },
    `Flywheel${status ? status.running ? ' · 运行中' : ' · 已停止' : ''}`),
    open && h('section', { 'aria-label': 'Flywheel 任务控制', style: {
      position: 'absolute', bottom: 'calc(100% + 8px)', left: 0, width: 'min(440px, 80vw)',
      maxHeight: '65vh', overflow: 'auto', padding: 14, border: '1px solid #8888', borderRadius: 10,
      background: 'var(--color-background, Canvas)', color: 'var(--color-foreground, CanvasText)',
      boxShadow: '0 8px 32px #0003', zIndex: 40,
    } },
    h('strong', null, 'Flywheel · Go 引擎'),
    h('p', { style: { margin: '8px 0' } }, '任务独立于桌面会话运行。模型与工作单元以引擎状态为准。'),
    h('label', { style: { display: 'block' } }, '任务目录（绝对路径）',
      h('input', { value: taskDir, onChange: e => setTaskDir(e.target.value),
        placeholder: '/path/to/repo/docs/task', disabled: busy,
        style: { width: '100%', padding: 6, border: '1px solid #8888', borderRadius: 4, margin: '4px 0 8px' } })),
    h('label', { style: { display: 'block' } }, '仓库目录（留空使用当前 DSH 会话目录）',
      h('input', { value: cwd, onChange: e => setCwd(e.target.value), placeholder: '/path/to/repo', disabled: busy,
        style: { width: '100%', padding: 6, border: '1px solid #8888', borderRadius: 4, margin: '4px 0 8px' } })),
    h('div', { style: { display: 'flex', gap: 8, marginBottom: 8 } },
      ...[['status', '刷新状态'], ['start', '启动'], ['stop', '停止']].map(([action, label]) =>
        h('button', { key: action, type: 'button', disabled: busy || !taskDir.trim(),
          onClick: () => run(action), style: { border: '1px solid #8888', padding: '4px 10px', borderRadius: 5 } }, label))),
    busy && h('p', { role: 'status' }, '正在等待引擎确认…'),
    error && h('p', { role: 'alert', style: { color: '#d34b4b', whiteSpace: 'pre-wrap' } }, error),
    status && h('p', null, `任务：${status.state?.slug ?? '未初始化'} · ${status.state?.status ?? '未知'} · PID ${status.pid ?? 0}`),
    status?.state?.['current-milestone'] && h('p', null, `当前里程碑：${status.state['current-milestone']}`),
    packets.length > 0 && h('table', { style: { width: '100%', textAlign: 'left', fontSize: 11, marginBottom: 8 } },
      h('thead', null, h('tr', null, ...['工作单元', '状态', '路由'].map(title => h('th', { key: title }, title)))),
      h('tbody', null, ...packets.slice(0, 12).map(([id, packet]) => h('tr', { key: id },
        h('td', null, id), h('td', null, packet.status ?? '未知'), h('td', null, packet.route ?? '—'))))),
    status?.state?.budget?.['unknown-cost-calls'] > 0 && h('p', null,
      `尚无结算成本的调用：${status.state.budget['unknown-cost-calls']} 次`),
    status && h('pre', { style: { fontSize: 11, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 240, overflow: 'auto' } },
      JSON.stringify(status, null, 2)),
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
