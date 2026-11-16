import { Service } from '@deepseek-ai/cordis';
import { Remote, bindTypertRemote } from '@deepseek-ai/dsh-typert-protocol';
import { realpath } from 'node:fs/promises';
import { ControlNotDispatchedError } from './control.js';
import { taskProjectionKey } from './task-association.js';
import { validPlanningCommand } from './planning-control.js';

// A read-only Remote, separate from logged human commands. Never add mutations
// here: their durable command/tool receipts own replay and uncertain outcomes.
export const consoleReadActions = new Set(['runs', 'status', 'config', 'config-edit', 'metrics', 'history', 'node', 'delivery', 'doctor', 'planning']);
const initializers = [];
export class ConsoleReadService extends Service {
  constructor(ctx, control, failure) {
    super(ctx, 'flywheelConsole');
    this.typertRemote = bindTypertRemote(this, 'flywheelConsole');
    this.control = control;
    this.failure = failure;
    for (const initialize of initializers) initialize.call(this);
  }
  async read(agent, input, signal) {
    try {
      if (!agent?.session || !consoleReadActions.has(input?.action)) throw new ControlNotDispatchedError('Only session-scoped console reads are supported');
      if (input.action === 'planning' && (input.planning?.action !== 'status' || !validPlanningCommand(input.planning))) throw new ControlNotDispatchedError('Console planning reads accept status only');
      // Taskless configuration uses the real session directory. The client may
      // not turn a read endpoint into arbitrary cross-session task discovery.
      const projection = this.ctx.sessionProjections.stateOf(agent.session, taskProjectionKey);
      const catalog = async () => {
        const value = await this.control({ action: 'runs' }, agent, signal);
        return { ...value, runs: (value.runs ?? []).filter(run => run.hostSessionId === agent.session.id) };
      };
      let value;
      if (input.action === 'runs') {
        value = await catalog();
      } else {
        if (input.taskDir) {
          const taskDir = await realpath(input.taskDir);
          const cwd = await realpath(input.cwd ?? this.ctx.workingDirectory.get(agent.session));
          const matches = task => task?.taskDir === taskDir && task.cwd === cwd;
          const historical = Object.values(projection?.tasks ?? {}).some(task => task?.outcome === 'confirmed' && matches(task));
          if (!matches(projection?.view) && !historical && !(await catalog()).runs.some(matches)) throw new ControlNotDispatchedError('This task is not associated with the current conversation');
        } else if (input.cwd != null && await realpath(input.cwd) !== await realpath(this.ctx.workingDirectory.get(agent.session))) {
          throw new ControlNotDispatchedError('Configuration must belong to the current conversation workspace');
        }
        value = await this.control(input, agent, signal);
      }
      return { result: { kind: 'success', text: JSON.stringify(value) } };
    } catch (error) {
      return { result: { kind: 'error', text: JSON.stringify(this.failure(input, error)) } };
    }
  }
}
// Invoke the public standard decorator in plain JS (no compiler/private symbols).
Remote(ConsoleReadService.prototype.read, { kind: 'method', name: 'read', static: false, private: false,
  addInitializer: initialize => initializers.push(initialize) });
