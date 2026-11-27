import { reconcileWorkspaces, watchWorkspaces } from './workspaces.js';
import { ControlNotDispatchedError, runControl } from './control.js';
import { resolveFlywheelBinary } from './installation.js';
import { mutatesTask, taskAssociationProjection, taskProjectionKey } from './task-association.js';
import { planningUserAction, planningUserProjection, planningUserProjectionKey } from './planning-control.js';
import { sessionUserAction, sessionUserProjection, sessionUserProjectionKey } from './session-control.js';
import { packetUserAction, packetUserProjection, packetUserProjectionKey } from './packet-control.js';
import { milestoneUserAction, milestoneUserProjection, milestoneUserProjectionKey } from './milestone-control.js';

export const inject = ['commands', 'tools', 'workingDirectory', 'sessionProjections'];

export function apply(ctx, config = {}) {
  // Loading the desktop package accidentally into the SDK profile must not
  // give a worker a tool that launches another orchestrator.
  if (process.env.FLYWHEEL_DSH_NATIVE_WORKER === '1') return;
  ctx.sessionProjections.register(taskAssociationProjection);
  ctx.sessionProjections.register(planningUserProjection);
  ctx.sessionProjections.register(sessionUserProjection);
  ctx.sessionProjections.register(packetUserProjection);
  ctx.sessionProjections.register(milestoneUserProjection);
  watchWorkspaces(ctx);
  let binary = null;
  let binaryError = null;
  try {
    binary = resolveFlywheelBinary(config);
  } catch (error) {
    binaryError = error instanceof Error ? error : new Error(String(error));
  }
  const active = new Set();
  const sessionMutations = new WeakMap();
  const failure = (input, error) => ({
    protocol: 1, type: 'flywheel-control-error',
    action: typeof input?.action === 'string' ? input.action : null,
    dispatch: error instanceof ControlNotDispatchedError ? 'not-dispatched' : 'unknown',
    message: error instanceof Error ? error.message : String(error),
  });
  const control = async (input, agent, signal, operationId, humanPlanningAction, humanSessionAction, overrideHostSessionID, humanPacketAction, humanMilestoneAction) => {
    if (!binary) {
      const detail = binaryError?.message || 'Flywheel engine binary could not be resolved.';
      throw new ControlNotDispatchedError(
        `${detail} Run "npx -y @ckanner/flywheel@latest install" to synchronize the DSH plugin and installation record, then reopen DSH Desktop.`,
      );
    }
    const session = agent?.session;
    const planningRead = input?.action === 'planning' && input.planning?.action === 'status';
    const mutation = mutatesTask(input?.action) && !planningRead;
    if (session && (mutation || input?.action === 'status' || planningRead)) {
      try {
        const projection = ctx.sessionProjections.stateOf(session, taskProjectionKey);
        if (!projection) throw new Error('Flywheel task association is unavailable for this session');
        const unresolved = projection.view;
        if (sessionMutations.has(session) || (mutation && unresolved &&
            unresolved.outcome !== 'confirmed' && unresolved.operationId !== operationId)) {
          throw new Error('A Flywheel mutation is still unconfirmed. Wait for it to finish, then query status for its original task before another launch/start/stop.');
        }
        // The projection can recover an omitted cwd only from the immutable
        // header or a logged directory change. An external getter fallback
        // cannot bind a missing response to the original project after reload.
        if (mutation && input?.cwd == null && (typeof projection.cwd !== 'string' ||
            !projection.cwd.startsWith('/') || /[\0\r\n]/.test(projection.cwd))) {
          throw new Error('This session has no recorded project directory. Supply an explicit absolute cwd for launch/start/stop so the original task can be associated and reconciled.');
        }
      } catch (error) {
        throw new ControlNotDispatchedError(error);
      }
    }
    const controller = new AbortController();
    if (session && mutation) sessionMutations.set(session, controller);
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
      const response = await runControl(input, {
        binary,
        defaultCwd,
        hostSessionID: session?.id ?? overrideHostSessionID,
        planningUserAction: humanPlanningAction,
        sessionUserAction: humanSessionAction,
        packetUserAction: humanPacketAction,
        milestoneUserAction: humanMilestoneAction,
        timeoutMs: config.controlTimeoutMs ?? 60000,
        signal: controller.signal,
      });
      try { await reconcileWorkspaces(ctx, response); }
      catch (error) {
        // Association failure does not undo a successful engine mutation.
        response.workspaceWarnings = ['工作区归属未同步：' + error.message];
      }
      return response;
    } finally {
      if (session && sessionMutations.get(session) === controller) sessionMutations.delete(session);
      active.delete(controller);
      signal?.removeEventListener('abort', cancel);
    }
  };
  ctx.on('dispose', () => { for (const controller of active) controller.abort(); });
  // Optional injection keeps command-only SDK/legacy fixtures independent.
  ctx.inject?.(['typert'], async scope => {
    // Host peers remain optional for CLI-only imports and native workers.
    const { ConsoleReadService } = await import('./console-read.js');
    new ConsoleReadService(scope, control, failure, config.defaultCwd);
  });
  ctx.commands.register({
    name: 'flywheel',
    description: '通过 Go 引擎控制 Flywheel 任务、查看实际模型配置或执行无推理环境检查。',
    input: { hint: '{"action":"launch","taskDir":"/absolute/docs/task","mode":"auto"} 或 {"action":"status","taskDir":"/absolute/docs/task"}' },
    async handler({ rawInput, agent, signal, commandId }) {
      let input;
      try {
        try {
          input = JSON.parse(rawInput);
        } catch (error) {
          throw new ControlNotDispatchedError(error);
        }
        let humanPlanningAction, humanSessionAction, humanPacketAction, humanMilestoneAction;
        try { humanPlanningAction = planningUserAction(input, agent?.session, commandId, rawInput,
          input?.action === 'planning' && input.planning?.action === 'approve' && agent?.session
            ? ctx.sessionProjections.stateOf(agent.session, planningUserProjectionKey) : undefined); }
        catch (error) { throw new ControlNotDispatchedError(error); }
        try { humanSessionAction = sessionUserAction(input, agent?.session, commandId, rawInput,
          input?.action === 'session' && input.session?.action === 'abandon' && agent?.session
            ? ctx.sessionProjections.stateOf(agent.session, sessionUserProjectionKey) : undefined); }
        catch (error) { throw new ControlNotDispatchedError(error); }
        try { humanPacketAction = packetUserAction(input, agent?.session, commandId, rawInput,
          input?.action === 'packet' && agent?.session
            ? ctx.sessionProjections.stateOf(agent.session, packetUserProjectionKey) : undefined); }
        catch (error) { throw new ControlNotDispatchedError(error); }
        try { humanMilestoneAction = milestoneUserAction(input, agent?.session, commandId, rawInput,
          input?.action === 'milestone' && agent?.session
            ? ctx.sessionProjections.stateOf(agent.session, milestoneUserProjectionKey) : undefined); }
        catch (error) { throw new ControlNotDispatchedError(error); }
        const value = await control(input, agent, signal, 'command:' + commandId, humanPlanningAction, humanSessionAction, undefined, humanPacketAction, humanMilestoneAction);
        return { kind: 'success', text: JSON.stringify(value, null, 2) };
      } catch (error) {
        // CommandResult preserves only kind/text. Wrap every failure, including
        // unknown outcomes, so a child error's text cannot impersonate host proof.
        return { kind: 'error', text: JSON.stringify(failure(input, error), null, 2) };
      }
    },
  });
  const toolFailures = new WeakMap();
  const executeTool = async (args, execution) => {
    try {
      if (args?.action === 'planning' && args.planning?.action === 'approve') throw new ControlNotDispatchedError('Use the human design approval control; model tools cannot approve a design');
      if (args?.action === 'session' && args.session?.action === 'abandon') throw new ControlNotDispatchedError('Use the human session abandonment control; model tools cannot retire an unknown invocation');
      if (args?.action === 'packet') throw new ControlNotDispatchedError('Use the human packet triage control; model tools cannot triage escalated inbox packets');
      if (args?.action === 'milestone') throw new ControlNotDispatchedError('Use the human milestone triage control; model tools cannot triage escalated milestones');
      return JSON.stringify(await control(args, execution.agent, execution.signal, 'tool:' + execution.callId));
    } catch (error) {
      toolFailures.set(execution, failure(args, error));
      throw error;
    }
  };
  // finalizeContent is the public last-mile ToolDefinition hook. Framework
  // failures remain isError outcomes, with host-owned dispatch proof available
  // to the durable projection instead of brittle matching of error prose.
  const finalizeContent = (execution, result) => {
    const known = toolFailures.get(execution);
    toolFailures.delete(execution);
    if (!result.isError) return;
    const action = execution.name === 'flywheel_launch' ? 'launch' : execution.arguments?.action;
    const value = known ?? failure({ action }, new Error(result.error?.message ?? 'Flywheel tool did not confirm its result'));
    if (result.error?.info?.code === 'ABORTED_BEFORE_DISPATCH') value.dispatch = 'not-dispatched';
    return [{ type: 'text', text: JSON.stringify(value) }];
  };
  const taskProperties = {
    taskDir: { type: 'string', description: 'Absolute existing prepared task ledger directory; never infer a task by scanning directories' },
    cwd: { type: 'string', description: 'Absolute target repository directory; defaults to the current working directory of this DSH session. Launch/start/stop require it explicitly if the session has no recorded directory.' },
    mode: { type: 'string', enum: ['auto', 'quick', 'full'], description: 'Launch/start only. Launch defaults to auto; omitted on legacy start preserves its existing behavior. The Go engine persists and explains the effective mode.' },
  };
  ctx.tools.register({
    name: 'flywheel_launch',
    description: 'Launch one prepared Flywheel task with its deterministic Go engine and associate its exact directory with this DSH conversation’s Flywheel view. ' +
      'Prepare and validate the frozen contract first; this tool does not invent requirements or write state.json. ' +
      'Default mode auto lets the engine choose Quick or Full and explain why. Explicit quick/full selects that request. ' +
      'If the result is unconfirmed, query flywheel_control status for the same task before repeating any mutation. Do not launch recursively from a Flywheel worker.',
    parameters: { type: 'object', additionalProperties: false, properties: taskProperties, required: ['taskDir'] },
    output: { schema: { type: 'string' }, render: (_args, value) => [{ type: 'text', text: value }] },
    timeoutMs: config.controlTimeoutMs ?? 60000,
    isConcurrencySafe: () => false,
    execute: (args, execution) => executeTool({ ...args, action: 'launch' }, execution),
    finalizeContent,
  });
  // ToolDefinition is the public raw registry format. The same boundary
  // validates both this entry and the human slash command before spawning.
  ctx.tools.register({
    name: 'flywheel_control',
    description: 'Control an existing Flywheel task via its deterministic Go engine. ' +
      'Use planning prepare with a thin brief to start design and independent review; planning plan requests design or scoped refinement only, with optional text retained as instructions for that request. New uses review-first. Global feedback references the observed state revision. Human design approval is available only through the Flywheel view or interactive CLI, never this model tool. ' +
      'Use investigate with a versioned preparation contract to gather bounded evidence; run starts the Go investigation owner in the background. Report-only investigation never authorizes implementation. Use launch for a prepared implementation task (Auto by default), status to inspect progress, start to resume, stop to await cancellation. ' +
      'Launch/start associates the exact task with the current conversation’s Flywheel view. After an unconfirmed mutation, query the same task before another mutation. ' +
      'Use config to inspect effective model bindings or doctor for a local preflight; these may omit taskDir. ' +
      'Only doctor accepts probe=true to check the SDK without sending an inference prompt. ' +
      'A successful doctor does not prove account authorization, quota, or model inference. ' +
      'This tool never writes state.json. Do not use it to create a recursive Flywheel run.',
    parameters: {
      type: 'object', additionalProperties: false,
      properties: {
        action: { type: 'string', enum: ['planning', 'investigate', 'launch', 'start', 'status', 'stop', 'config', 'doctor'] },
        ...taskProperties,
        planning: { type: 'object', additionalProperties: false, properties: {
          action: { type: 'string', enum: ['prepare', 'plan', 'feedback', 'status', 'migrate'] },
          revision: { type: 'integer', minimum: 1 },
          'request-id': { type: 'string', description: 'Stable request identity; reuse it when reconciling the same action.' },
          brief: { type: 'object', description: 'Installed planning brief contract, retaining the original user request and existing authority. New defaults to review-first.' },
          design: { type: 'object', additionalProperties: false, properties: { revision: { type: 'integer', minimum: 1 }, digest: { type: 'string' } }, required: ['revision', 'digest'] },
          scope: { type: 'string', enum: ['design', 'milestone', 'packet', 'replan'] },
          'scope-id': { type: 'string' },
          text: { type: 'string', description: 'Plan: scope-local instructions retained for planner and reviewer without invalidating root approval. Feedback: changed global goals or constraints. Migrate: reason for an explicitly requested migration.' },
          policy: { type: 'string', enum: ['review-first', 'delegated'] },
          start: { type: 'boolean', description: 'Start the requested planning work; this does not approve a design.' },
        }, required: ['action'] },
        investigation: { type: 'object', additionalProperties: false, properties: {
          action: { type: 'string', enum: ['start', 'run', 'status', 'report', 'cancel'] },
          run: { type: 'boolean', description: 'Start only: register the bounded contract and start its Go investigation owner; defaults to false.' },
          revision: { type: 'integer', minimum: 1 },
          contract: { type: 'object', description: 'Exact version-1 preparation.schema.json contract; use flywheel template to read the installed schema. Store no credentials.' },
          goal: { type: 'string', description: 'Start only (alternative to contract): one-sentence investigation goal synthesized into a bounded read-only contract by the Go engine.' },
          intent: { type: 'string', enum: ['investigate'] },
          'read-paths': { type: 'array', items: { type: 'string' }, description: 'Optional clean repository-relative paths paired with goal.' },
          report: { type: 'object', description: 'Evidence-cited investigation report; references must name recorded invocation IDs. This never marks code completed.' },
        }, required: ['action'] },
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
    execute: executeTool,
    finalizeContent,
  });
}
