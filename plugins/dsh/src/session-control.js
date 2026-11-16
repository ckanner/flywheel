import { z } from 'zod';

// The engine owns abandonment semantics. This boundary records the exact human
// Flywheel command and turns it into a proof; model tools never receive it.
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const text = (value, max = 200) => typeof value === 'string' && value.trim() !== '' && value.length <= max && !value.includes('\0');

export function validSessionAbandonCommand(value) {
  return record(value) && Object.keys(value).every(key => ['action', 'key', 'revision', 'reason', 'invocation'].includes(key)) &&
    value.action === 'abandon' && text(value.key, 256) && text(value.invocation, 200) &&
    Number.isSafeInteger(value.revision) && value.revision > 0 &&
    text(value.reason, 1000) && Buffer.byteLength(value.reason) <= 1000;
}

export const sessionUserProjectionKey = 'flywheelSessionUser';
export const sessionUserProjection = {
  key: sessionUserProjectionKey,
  stateVersion: 1,
  stateSchema: z.object({ inheritedEventCount: z.number().int().nonnegative(), commands: z.record(z.string(), z.string().nullable()) }),
  init: (_header, inheritedEventCount) => ({ inheritedEventCount, commands: {} }),
  apply(state, event) {
    if (event.seq < state.inheritedEventCount) return state;
    const data = event.data;
    if (event.type === 'command/done' && Object.hasOwn(state.commands, data.commandId)) {
      return { ...state, commands: { ...state.commands, [data.commandId]: null } };
    }
    if (event.type !== 'command/run' || data.name !== 'flywheel') return state;
    let input;
    try { input = JSON.parse(data.args); } catch { return state; }
    if (input?.action !== 'session' || input.session?.action !== 'abandon') return state;
    const allowed = !Object.hasOwn(state.commands, data.commandId) && data.source?.kind === 'user';
    return { ...state, commands: { ...state.commands, [data.commandId]: allowed ? data.args : null } };
  },
};

export function sessionUserAction(input, session, commandId, rawInput, projection) {
  if (input?.action !== 'session' || input.session?.action !== 'abandon') return undefined;
  if (!validSessionAbandonCommand(input.session) || !text(session?.id, 200) || !text(commandId, 200)) {
    throw new Error('Session abandonment requires a recorded human Flywheel command');
  }
  if (projection?.commands?.[commandId] !== rawInput || typeof rawInput !== 'string') {
    throw new Error('Session abandonment requires this command’s exact, unconsumed user event');
  }
  return {
    kind: 'dsh', sessionId: session.id, commandId, taskDir: input.taskDir,
    key: input.session.key, invocation: input.session.invocation, revision: input.session.revision,
    action: 'abandon', reason: input.session.reason,
  };
}

export function validSessionUserAction(value, command, taskDir) {
  return record(value) && value.kind === 'dsh' && text(value.sessionId) && text(value.commandId) &&
    command?.action === 'abandon' && value.action === 'abandon' && value.taskDir === taskDir &&
    value.key === command.key && value.invocation === command.invocation &&
    value.revision === command.revision && value.reason === command.reason;
}
