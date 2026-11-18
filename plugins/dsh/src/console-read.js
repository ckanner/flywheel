import { Service } from '@deepseek-ai/cordis';
import { Remote, bindTypertRemote } from '@deepseek-ai/dsh-typert-protocol';
import { realpath } from 'node:fs/promises';
import { ControlNotDispatchedError } from './control.js';
import { taskProjectionKey } from './task-association.js';
import { validPlanningCommand } from './planning-control.js';

// A read-only Remote, separate from logged human commands. Never add mutations
// here: their durable command/tool receipts own replay and uncertain outcomes.
export const consoleReadActions = new Set(['runs', 'status', 'config', 'config-edit', 'metrics', 'history', 'node', 'delivery', 'doctor', 'planning', 'model-info']);
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
        if (input.action === 'model-info') {
          const model = input.model;
          if (Object.keys(input).some(key => !['action', 'taskDir', 'cwd', 'model'].includes(key)) || !model || Object.keys(model).some(key => !['provider', 'id'].includes(key)) ||
              !['provider', 'id'].every(key => typeof model[key] === 'string' && model[key].trim() && model[key].length <= 512 && !/[\0\r\n]/.test(model[key]))) {
            throw new ControlNotDispatchedError('Exact provider and model identifiers are required');
          }
          const llm = this.ctx.get('llm');
          if (typeof llm?.resolveModelInfo !== 'function') throw new ControlNotDispatchedError('This host does not expose public model metadata');
          const info = await llm.resolveModelInfo(model.provider, model.id, signal);
          const reasoning = info?.reasoning;
          if (!info || !Array.isArray(reasoning?.efforts) || reasoning.efforts.length > 128 ||
              !reasoning.efforts.every(e => typeof e.id === 'string' && e.id.trim() && e.id.length <= 128 && !/[\0\r\n]/.test(e.id) && typeof e.name === 'string' && e.name.trim() && e.name.length <= 512) ||
              new Set(reasoning.efforts.map(e => e.id)).size !== reasoning.efforts.length ||
              reasoning.defaultEffort !== undefined && !reasoning.efforts.some(e => e.id === reasoning.defaultEffort)) {
            throw new ControlNotDispatchedError('The model does not publish a valid reasoning-effort catalog');
          }
          value = { protocol: 1, ok: true, taskDir: input.taskDir ? await realpath(input.taskDir) : '', modelInfo: {
            provider: model.provider, id: model.id, source: 'desktop-host',
            efforts: reasoning.efforts.map(({ id, name }) => ({ id, name })),
            ...(reasoning.defaultEffort !== undefined ? { defaultEffort: reasoning.defaultEffort } : {}),
          } };
        } else value = await this.control(input, agent, signal);
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
