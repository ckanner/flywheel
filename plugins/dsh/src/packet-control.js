import { z } from 'zod';

// The Go engine owns inbox packet triage semantics (retry / replan / accept).
// This boundary records the exact human Flywheel command and turns it into a
// host proof; model tools never receive it.
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const text = (value, max = 200) => typeof value === 'string' && value.trim() !== '' && value.length <= max && !value.includes('\0');
const packetActions = new Set(['retry', 'replan', 'accept']);

export function validPacketTriageCommand(value) {
  if (!record(value)) return false;
  if (!Object.keys(value).every(key => ['action', 'packet-id', 'revision', 'reason', 'guidance'].includes(key))) return false;
  if (!packetActions.has(value.action)) return false;
  if (!text(value['packet-id'], 100) || /[\r\n]/.test(value['packet-id'])) return false;
  if (!Number.isSafeInteger(value.revision) || value.revision < 1) return false;
  if (!text(value.reason, 1000) || Buffer.byteLength(value.reason) > 1000) return false;
  if ('guidance' in value) {
    if (typeof value.guidance !== 'string' || value.guidance.includes('\0') || Buffer.byteLength(value.guidance) > 2000) return false;
  }
  return true;
}

export const packetUserProjectionKey = 'flywheelPacketUser';
export const packetUserProjection = {
  key: packetUserProjectionKey,
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
    if (input?.action !== 'packet') return state;
    const allowed = !Object.hasOwn(state.commands, data.commandId) && data.source?.kind === 'user';
    return { ...state, commands: { ...state.commands, [data.commandId]: allowed ? data.args : null } };
  },
};

export function packetUserAction(input, session, commandId, rawInput, projection) {
  if (input?.action !== 'packet') return undefined;
  if (!validPacketTriageCommand(input.packet) || !text(session?.id, 200) || !text(commandId, 200)) {
    throw new Error('Packet triage requires a recorded human Flywheel command');
  }
  if (projection?.commands?.[commandId] !== rawInput || typeof rawInput !== 'string') {
    throw new Error('Packet triage requires this command’s exact, unconsumed user event');
  }
  return {
    kind: 'dsh',
    sessionId: session.id,
    commandId,
    taskDir: input.taskDir,
    packetId: input.packet['packet-id'],
    revision: input.packet.revision,
    action: input.packet.action,
    reason: input.packet.reason,
    ...(input.packet.guidance ? { guidance: input.packet.guidance } : {}),
  };
}

export function validPacketUserAction(value, command, taskDir) {
  return record(value) && value.kind === 'dsh' && text(value.sessionId) && text(value.commandId) &&
    validPacketTriageCommand(command) && value.action === command.action && value.taskDir === taskDir &&
    value.packetId === command['packet-id'] && value.revision === command.revision &&
    value.reason === command.reason && (value.guidance ?? '') === (command.guidance ?? '');
}
