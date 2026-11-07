import { z } from 'zod';

// This read model folds existing public command/tool events. Third-party DSH
// plugins must not invent Session.append event types or write engine state.
export const taskProjectionKey = 'flywheelTask';
const modes = ['auto', 'quick', 'full'];
const taskActions = ['launch', 'start', 'status', 'stop'];
export const mutatesTask = action => ['launch', 'start', 'stop'].includes(action);
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const path = value => typeof value === 'string' && value.startsWith('/') && !/[\0\r\n]/.test(value);
const parse = value => { try { return JSON.parse(value); } catch { return null; } };
const sameTask = (a, b) => a.taskDir === b.taskDir && a.cwd === b.cwd;
const operationSchema = z.object({
  operationId: z.string(), sequence: z.number().int().nonnegative(),
  action: z.enum(taskActions), taskDir: z.string(), cwd: z.string(),
});
const viewSchema = operationSchema.extend({
  outcome: z.enum(['pending', 'confirmed', 'unknown']), message: z.string(),
}).nullable();
const stateSchema = z.object({
  cwd: z.string(), inheritedEventCount: z.number().int().nonnegative(),
  calls: z.record(z.string(), operationSchema), view: viewSchema,
});

function request(input, cwd, operationId, sequence, launchOnly = false) {
  if (!record(input)) return null;
  const value = launchOnly ? { ...input, action: 'launch' } : input;
  if (!taskActions.includes(value.action) || !path(value.taskDir) || !path(value.cwd ?? cwd) ||
      Object.keys(input).some(key => !(launchOnly ? ['taskDir', 'cwd', 'mode'] : ['action', 'taskDir', 'cwd', 'mode']).includes(key)) ||
      ('mode' in value && (!['start', 'launch'].includes(value.action) || !modes.includes(value.mode)))) return null;
  return { operationId, sequence, action: value.action, taskDir: value.taskDir, cwd: value.cwd ?? cwd };
}

function begin(state, input, id, event, launchOnly = false) {
  const operation = request(input, state.cwd, id, event.seq, launchOnly);
  if (!operation) return state;
  const unresolved = state.view && state.view.outcome !== 'confirmed';
  // A second attempted mutation cannot erase the first uncertain target, even
  // when a model calls a new tool before the earlier response reaches its view.
  const binds = mutatesTask(operation.action) && !unresolved;
  return { ...state, calls: { ...state.calls, [id]: operation },
    view: binds ? { ...operation, outcome: 'pending', message: '' } : state.view };
}

function settle(state, id, value, failed, error) {
  const operation = state.calls[id];
  if (!operation) return state;
  const calls = { ...state.calls };
  delete calls[id];
  let view = state.view;
  const owns = view?.operationId === id;
  const successful = !failed && record(value) && value.protocol === 1 && value.ok === true &&
    path(value.taskDir) && (value.cwd === undefined || path(value.cwd)) && typeof value.running === 'boolean';
  if (successful) {
    // Compare the logged request, not lexical input against a realpath result.
    // A later successful status explicitly reconciles the original target.
    const reconciles = operation.action === 'status' && view && sameTask(operation, view) && operation.sequence > view.sequence;
    if (owns || reconciles || (!view && operation.action === 'status') ||
        (view?.outcome === 'confirmed' && operation.sequence > view.sequence && ['launch', 'start'].includes(operation.action))) {
      view = { ...operation, taskDir: value.taskDir, cwd: value.cwd ?? operation.cwd, outcome: 'confirmed', message: '' };
    }
  } else if (owns) {
    const typed = record(value) && value.protocol === 1 && value.type === 'flywheel-control-error' &&
      value.action === operation.action && ['not-dispatched', 'unknown'].includes(value.dispatch);
    // Only host-produced structured proof can release an ambiguous mutation.
    const notDispatched = typed && value.dispatch === 'not-dispatched' || error?.code === 'ABORTED_BEFORE_DISPATCH';
    view = { ...view, outcome: notDispatched ? 'confirmed' : 'unknown',
      message: typed && typeof value.message === 'string' ? value.message : '' };
  }
  return { ...state, calls, view };
}

function contentValue(content) {
  if (!Array.isArray(content)) return null;
  const text = content.filter(block => block?.type === 'text').map(block => block.text).join('\n');
  return parse(text);
}

export const taskAssociationProjection = {
  key: taskProjectionKey,
  stateVersion: 1,
  stateSchema,
  init: (header, inheritedEventCount) => ({
    cwd: typeof header.cwd === 'string' ? header.cwd : '', inheritedEventCount,
    calls: {}, view: null,
  }),
  apply(state, event) {
    if (event.type === 'working-directory/change') return { ...state, cwd: event.data.cwd };
    // A fork is a new conversation: inherited Flywheel tools are context, not
    // permission to control the parent conversation's background task.
    if (event.seq < state.inheritedEventCount) return state;
    const data = event.data;
    if (event.type === 'command/run' && data.name === 'flywheel') {
      return begin(state, parse(data.args), 'command:' + data.commandId, event);
    }
    if (event.type === 'command/done') {
      return settle(state, 'command:' + data.commandId, parse(data.text), data.kind !== 'success');
    }
    if (event.type === 'tool/call' && ['flywheel_launch', 'flywheel_control'].includes(data.name)) {
      return begin(state, parse(data.arguments), 'tool:' + data.callId, event, data.name === 'flywheel_launch');
    }
    if (event.type === 'tool/result') {
      return settle(state, 'tool:' + data.message.toolCallId, contentValue(data.message.content), data.message.isError, data.error);
    }
    if (event.type === 'tool/ptc-dispatch-start' && ['flywheel_launch', 'flywheel_control'].includes(data.name)) {
      return begin(state, data.arguments, 'tool:' + data.subCallId, event, data.name === 'flywheel_launch');
    }
    if (event.type === 'tool/ptc-dispatch') {
      return settle(state, 'tool:' + data.subCallId, contentValue(data.content), data.isError, data.error);
    }
    return state;
  },
  wire: { viewSchema, view: state => state.view },
};
