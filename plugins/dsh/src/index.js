import { ControlNotDispatchedError, runControl } from './control.js';
import { resolveFlywheelBinary } from './installation.js';

export const inject = ['commands', 'tools', 'workingDirectory'];

export function apply(ctx, config = {}) {
  // Loading the desktop package accidentally into the SDK profile must not
  // give a worker a tool that launches another orchestrator.
  if (process.env.FLYWHEEL_DSH_NATIVE_WORKER === '1') return;
  const binary = resolveFlywheelBinary(config);
  const active = new Set();
  const control = async (input, agent, signal) => {
    const controller = new AbortController();
    const cancel = () => controller.abort();
    signal?.addEventListener('abort', cancel, { once: true });
    if (signal?.aborted) cancel();
    active.add(controller);
    try {
      let defaultCwd;
      try {
        // The immutable session header records its original project. Read the
        // public current-directory projection without ensure() recovery effects.
        if (input?.cwd == null && agent?.session) {
          defaultCwd = ctx.workingDirectory.get(agent.session);
        }
      } catch (error) {
        // No control process has been launched if resolving the default fails.
        throw new ControlNotDispatchedError(error);
      }
      return await runControl(input, {
        binary,
        defaultCwd,
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
    description: '通过 Go 引擎控制 Flywheel 任务、查看实际模型配置或执行无推理环境检查。',
    input: { hint: '{"action":"doctor"} 或 {"action":"status","taskDir":"/absolute/docs/task"}' },
    async handler({ rawInput, agent, signal }) {
      let input;
      try {
        try {
          input = JSON.parse(rawInput);
        } catch (error) {
          throw new ControlNotDispatchedError(error);
        }
        const value = await control(input, agent, signal);
        return { kind: 'success', text: JSON.stringify(value, null, 2) };
      } catch (error) {
        // CommandResult preserves only kind/text. Wrap every failure, including
        // unknown outcomes, so a child error's text cannot impersonate host proof.
        return { kind: 'error', text: JSON.stringify({
          protocol: 1,
          type: 'flywheel-control-error',
          action: typeof input?.action === 'string' ? input.action : null,
          dispatch: error instanceof ControlNotDispatchedError ? 'not-dispatched' : 'unknown',
          message: error instanceof Error ? error.message : String(error),
        }, null, 2) };
      }
    },
  });
  // ToolDefinition is the public raw registry format. The same boundary
  // validates both this entry and the human slash command before spawning.
  ctx.tools.register({
    name: 'flywheel_control',
    description: 'Control an existing Flywheel task via its deterministic Go engine. ' +
      'Use status to inspect progress, start to launch in the background, stop to await cancellation. ' +
      'Use config to inspect effective model bindings or doctor for a local preflight; these may omit taskDir. ' +
      'Only doctor accepts probe=true to check the SDK without sending an inference prompt. ' +
      'A successful doctor does not prove account authorization, quota, or model inference. ' +
      'This tool never writes state.json. Do not use it to create a recursive Flywheel run.',
    parameters: {
      type: 'object', additionalProperties: false,
      properties: {
        action: { type: 'string', enum: ['start', 'status', 'stop', 'config', 'doctor'] },
        taskDir: { type: 'string', description: 'Absolute existing task ledger directory; required for start/status/stop' },
        cwd: { type: 'string', description: 'Absolute target repository directory; defaults to the current working directory of this DSH session' },
        probe: { type: 'boolean', description: 'Doctor only: explicitly enable SDK checks without inference; defaults to false' },
      },
      required: ['action'],
    },
    output: {
      schema: { type: 'string' },
      render: (_args, value) => [{ type: 'text', text: value }],
    },
    timeoutMs: config.controlTimeoutMs ?? 60000,
    isConcurrencySafe: args => ['status', 'config', 'doctor'].includes(args.action),
    async execute(args, execution) {
      return JSON.stringify(await control(args, execution.agent, execution.signal));
    },
  });
}
