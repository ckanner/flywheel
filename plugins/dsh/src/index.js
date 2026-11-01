import { runControl } from './control.js';

export const inject = ['commands', 'tools'];

export function apply(ctx, config = {}) {
  // Loading the desktop package accidentally into the SDK profile must not
  // give a worker a tool that launches another orchestrator.
  if (process.env.FLYWHEEL_DSH_NATIVE_WORKER === '1') return;
  const active = new Set();
  const control = async (input, agent, signal) => {
    const controller = new AbortController();
    const cancel = () => controller.abort();
    signal?.addEventListener('abort', cancel, { once: true });
    if (signal?.aborted) cancel();
    active.add(controller);
    try {
      return await runControl(input, {
        binary: config.flywheelBinary ?? 'flywheel',
        defaultCwd: agent?.session?.header?.cwd,
        timeoutMs: config.controlTimeoutMs ?? 60000,
        signal: controller.signal,
      });
    } finally {
      active.delete(controller);
      signal?.removeEventListener('abort', cancel);
    }
  };
  ctx.on('dispose', () => { for (const controller of active) controller.abort(); });
  ctx.commands.register({
    name: 'flywheel',
    description: '通过 Go 引擎启动、查看或停止 Flywheel 任务。',
    input: { hint: '{"action":"status","taskDir":"/absolute/docs/task"}' },
    async handler({ rawInput, agent, signal }) {
      try {
        const value = await control(JSON.parse(rawInput), agent, signal);
        return { kind: 'success', text: JSON.stringify(value, null, 2) };
      } catch (error) {
        return { kind: 'error', text: error.message };
      }
    },
  });
  // ToolDefinition is the public raw registry format. The same boundary
  // validates both this entry and the human slash command before spawning.
  ctx.tools.register({
    name: 'flywheel_control',
    description: 'Control an existing Flywheel task via its deterministic Go engine. ' +
      'Use status to inspect progress, start to launch in the background, stop to await cancellation. ' +
      'This tool never writes state.json. Do not use it to create a recursive Flywheel run.',
    parameters: {
      type: 'object', additionalProperties: false,
      properties: {
        action: { type: 'string', enum: ['start', 'status', 'stop'] },
        taskDir: { type: 'string', description: 'Absolute existing task ledger directory' },
        cwd: { type: 'string', description: 'Absolute target repository directory; defaults to this DSH session' },
      },
      required: ['action', 'taskDir'],
    },
    output: {
      schema: { type: 'string' },
      render: (_args, value) => [{ type: 'text', text: value }],
    },
    timeoutMs: config.controlTimeoutMs ?? 60000,
    isConcurrencySafe: args => args.action === 'status',
    async execute(args, execution) {
      return JSON.stringify(await control(args, execution.agent, execution.signal));
    },
  });
}
