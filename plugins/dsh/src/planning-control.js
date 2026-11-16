import { z } from 'zod';

// The engine owns planning semantics. This boundary rejects malformed requests
// and separates a recorded human command from model-provided JSON.
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const text = (value, max = 8192) => typeof value === 'string' && value.trim() !== '' && value.length <= max && !value.includes('\0');
const ref = value => record(value) && Object.keys(value).every(key => ['revision', 'digest'].includes(key)) &&
  Number.isSafeInteger(value.revision) && value.revision > 0 && typeof value.digest === 'string' && /^[a-f0-9]{64}$/.test(value.digest);
const fields = {
  prepare: ['action', 'request-id', 'brief', 'start'],
  plan: ['action', 'revision', 'request-id', 'scope', 'scope-id', 'text', 'start'],
  feedback: ['action', 'revision', 'request-id', 'design', 'text', 'policy', 'start'],
  approve: ['action', 'revision', 'request-id', 'design', 'start'],
  status: ['action', 'request-id'],
  migrate: ['action', 'revision', 'request-id', 'policy', 'text', 'start'],
};

export function validPlanningCommand(value) {
  if (!record(value) || !Object.hasOwn(fields, value.action) || Object.keys(value).some(key => !fields[value.action].includes(key))) return false;
  if ('start' in value && typeof value.start !== 'boolean') return false;
  if ('request-id' in value && (!text(value['request-id'], 120) || !/^[A-Za-z0-9][A-Za-z0-9_.-]*$/.test(value['request-id']))) return false;
  if (!['prepare', 'status'].includes(value.action) && (!Number.isSafeInteger(value.revision) || value.revision < 1 || !value['request-id'])) return false;
  if ('design' in value && !ref(value.design)) return false;
  if ('policy' in value && !['review-first', 'delegated'].includes(value.policy)) return false;
  if ('text' in value && (!text(value.text) || Buffer.byteLength(value.text) > 8192)) return false;
  if (value.action === 'prepare' && !record(value.brief)) return false;
  if (value.action === 'approve' && !ref(value.design)) return false;
  if (value.action === 'feedback' && !text(value.text)) return false;
  if (value.action === 'plan' && (!['design', 'milestone', 'packet', 'replan'].includes(value.scope) ||
      ('scope-id' in value && (!text(value['scope-id'], 120) || !/^[A-Za-z0-9][A-Za-z0-9_.-]*$/.test(value['scope-id']))) ||
      (['milestone', 'packet'].includes(value.scope) && !value['scope-id']))) return false;
  return true;
}

// Public projections fold the original command events without synchronous log reads.
export const planningUserProjectionKey = 'flywheelPlanningUser';
export const planningUserProjection = {
  key: planningUserProjectionKey,
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
    if (input?.action !== 'planning' || input.planning?.action !== 'approve') return state;
    const allowed = !Object.hasOwn(state.commands, data.commandId) && data.source?.kind === 'user';
    return { ...state, commands: { ...state.commands, [data.commandId]: allowed ? data.args : null } };
  },
};

export function planningUserAction(input, session, commandId, rawInput, projection) {
  if (input?.action !== 'planning' || input.planning?.action !== 'approve') return undefined;
  if (!validPlanningCommand(input.planning) || !text(session?.id, 200) || !text(commandId, 200)) {
    throw new Error('Design approval requires a recorded human Flywheel command');
  }
  if (projection?.commands?.[commandId] !== rawInput || typeof rawInput !== 'string') {
    throw new Error('Design approval requires this command’s exact, unconsumed user event');
  }
  return { kind: 'dsh', sessionId: session.id, commandId, 'request-id': input.planning['request-id'],
    revision: input.planning.revision, design: { ...input.planning.design }, start: input.planning.start ?? false };
}

export function validPlanningUserAction(value, command) {
  return record(value) && value.kind === 'dsh' && text(value.sessionId, 200) && text(value.commandId, 200) &&
    command?.action === 'approve' && value['request-id'] === command['request-id'] &&
    value.revision === command.revision && ref(value.design) && value.design.revision === command.design?.revision &&
    value.design.digest === command.design?.digest && value.start === (command.start ?? false);
}
