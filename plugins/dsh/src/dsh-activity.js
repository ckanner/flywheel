// Transient activity is deliberately separate from durable Session events.
// The published SDK transports durable records but does not forward the public
// agent/assistant-stream hook. This policy extension owns only notifications;
// the SDK remains the sole stdin reader and request/response server.
import { validProviderActivity } from './machine-worker.js';

export const DSH_ACTIVITY_METHOD = 'flywheel.activity';
export const DSH_ACTIVITY_VERSION = 1;
export const DSH_ACTIVITY_CAPABILITIES = Object.freeze({
  version: 1,
  observation: 'events-only',
  root: 'agent/assistant-stream',
  children: 'admitted-local-agents',
  externalProviders: 'flywheelProviderActivity/v1-when-installed',
  opaquePhases: ['startup', 'before-first-event', 'silent-tool', 'external-provider-without-activity'],
});

const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const identity = value => typeof value === 'string' && value.length > 0 && value.length <= 512 && !value.includes('\0');
const positive = value => Number.isSafeInteger(value) && value > 0;
const scopes = new Set(['root', 'child', 'mixed']);
const activityKeys = new Set(['version', 'sequence', 'kind', 'source', 'scope', 'eventTypes', 'observedEvents']);
const streamKinds = new Set(['block-start', 'block-end', 'text-delta', 'reasoning-delta', 'tool-call-delta', 'usage', 'finish']);
const eventTypes = new Set(['assistant/start', 'assistant/chunk', 'assistant/end',
  ...[...streamKinds].map(type => 'assistant/' + type), 'provider/codex-pro', 'provider/claude-code-pro', 'provider/chatgpt-web']);

export function dshActivityInterval(idleTimeoutMs = 300000) {
  return Math.max(1, Math.min(1000, Math.floor(idleTimeoutMs / 4)));
}

/** Check the small, content-free progress contract before it can refresh idle. */
export function validDshActivity(value, { external = false } = {}) {
  if (!object(value)) return false;
  if (external) {
    const { invocationId, ...activity } = value;
    return identity(invocationId) && invocationId.length <= 128 && validProviderActivity(activity, activity.source);
  }
  return Object.keys(value).every(key => activityKeys.has(key)) &&
    value.version === 1 && positive(value.sequence) && value.kind === 'provider-progress' &&
    value.source === 'dsh' &&
    scopes.has(value.scope) && positive(value.observedEvents) && Array.isArray(value.eventTypes) &&
    value.eventTypes.length > 0 && value.eventTypes.length <= 16 && value.eventTypes.every(type => eventTypes.has(type)) &&
    new Set(value.eventTypes).size === value.eventTypes.length;
}

/**
 * Bind a policy-owned set of real Agents to observed activity. Calls after
 * disposal, from foreign/replaced Agents, or with changed lineage are ignored.
 * No timer emits progress: a new valid observation is necessary for every send.
 *
 * The optional Cordis service is a Flywheel extension, not a DSH built-in event:
 * report(agent, { ...machineActivity, invocationId }). Native providers keep
 * their own streaming/UI channels and use this only when the service exists.
 */
export function createDshActivityPublisher(ctx, { identity: getIdentity, idleTimeoutMs = 300000, children = false, webIdentity }, {
  now = () => performance.now(),
  notify = (method, params) => process.stdout.write(JSON.stringify({ jsonrpc: '2.0', method, params }) + '\n'),
} = {}) {
  const intervalMs = dshActivityInterval(idleTimeoutMs);
  const admitted = new Map();
  let closed = false, sequence = 0, lastSent = -Infinity, observedEvents = 0;
  const pendingTypes = new Set(), pendingScopes = new Set();
  const live = agent => agent && String(agent.id) === String(agent.session?.id) && ctx.get('agents')?.get(String(agent.id)) === agent;
  const owns = agent => {
    const rootId = getIdentity().rootSessionId;
    const visited = new Set();
    for (let member = agent; member;) {
      const id = String(member.id), record = admitted.get(id);
      if (!live(member) || record?.agent !== member || visited.has(id) || record.parentId !== String(member.session.header.parentSession ?? '')) return false;
      if (id === rootId) return record.root;
      if (!children || record.root || member.session.header.origin !== 'subagent') return false;
      visited.add(id);
      member = admitted.get(record.parentId)?.agent;
    }
    return false;
  };
  const observe = (agent, types, count = 1) => {
    if (closed || !owns(agent)) return false;
    for (const type of types) if (pendingTypes.size < 16) pendingTypes.add(type);
    pendingScopes.add(String(agent.id) === getIdentity().rootSessionId ? 'root' : 'child');
    observedEvents = Math.min(Number.MAX_SAFE_INTEGER, observedEvents + count);
    const at = now();
    if (at - lastSent < intervalMs) return true;
    const activity = { version: 1, sequence: ++sequence, kind: 'provider-progress', source: 'dsh',
      scope: pendingScopes.size > 1 ? 'mixed' : [...pendingScopes][0],
      eventTypes: [...pendingTypes], observedEvents };
    pendingTypes.clear(); pendingScopes.clear(); observedEvents = 0; lastSent = at;
    notify(DSH_ACTIVITY_METHOD, { version: 1, ...getIdentity(), activity });
    return true;
  };
  const service = Object.freeze({ version: 1, intervalMs,
    report(agent, activity) {
      if (closed || !owns(agent) || !validDshActivity(activity, { external: true })) return false;
      const entries = admitted.get(String(agent.id)).external;
      const key = activity.source + ':' + activity.invocationId;
      if (activity.sequence <= (entries.get(key) ?? 0) || !entries.has(key) && entries.size >= 4096) return false;
      entries.set(key, activity.sequence);
      return observe(agent, ['provider/' + activity.source], activity.observedEvents);
    },
  });
  ctx.provide('flywheelProviderActivity', service);
  ctx.on('agent/assistant-stream', ({ agent, frame }) => {
    if (closed || !owns(agent) || !object(frame) || !identity(frame.attemptId) || !positive(frame.revision) ||
        !['start', 'chunk', 'end'].includes(frame.type)) return;
    const entry = admitted.get(String(agent.id));
    if (frame.revision <= entry.revision) return;
    entry.revision = frame.revision;
    const type = frame.type === 'chunk' && streamKinds.has(frame.chunk?.type) ? frame.chunk.type : frame.type;
    observe(agent, ['assistant/' + type]);
  });
  if (webIdentity) ctx.on('chatgpt-web/activity', value => {
    const expected = webIdentity();
    if (!object(value) || !expected || Object.keys(value).some(key => !['hostInvocationId','executionDomainId','sessionId','requestId','generation','kind','sequence','bindingDigest','at','observedBytes'].includes(key)) ||
        value.hostInvocationId !== expected.hostInvocationId || value.executionDomainId !== expected.executionDomainId ||
        value.bindingDigest !== undefined && !identity(value.bindingDigest) || value.at !== undefined && !positive(value.at) ||
        value.observedBytes !== undefined && (!Number.isSafeInteger(value.observedBytes) || value.observedBytes < 0) ||
        !identity(value.requestId) || !positive(value.generation) || !positive(value.sequence) ||
        !['prompt-accepted','content-delta','phase-change','response-completed'].includes(value.kind)) return;
    const entry = admitted.get(value.sessionId);
    if (!entry || !owns(entry.agent)) return;
    const key = 'web:' + value.requestId + ':' + value.generation;
    if (value.sequence <= (entry.external.get(key) ?? 0) || !entry.external.has(key) && entry.external.size >= 4096) return;
    entry.external.set(key, value.sequence);
    observe(entry.agent, ['provider/chatgpt-web']);
  });
  ctx.on('agent/disposed', ({ agent }) => {
    if (admitted.get(String(agent.id))?.agent === agent) admitted.delete(String(agent.id));
  });
  const publisher = {
    capabilities: Object.freeze({ ...DSH_ACTIVITY_CAPABILITIES, children: children ? 'admitted-local-agents' : 'disabled' }),
    service,
    admit(agent, root) {
      const id = String(agent.id), parentId = String(agent.session?.header.parentSession ?? '');
      if (closed || !live(agent) || (root ? id !== getIdentity().rootSessionId :
        !children || agent.session.header.origin !== 'subagent' || !owns(admitted.get(parentId)?.agent))) {
        throw new Error('DSH activity admission requires the current owned Agent lineage');
      }
      admitted.set(id, { agent, parentId, root, revision: 0, external: new Map() });
    },
    close() { closed = true; admitted.clear(); pendingTypes.clear(); pendingScopes.clear(); },
  };
  ctx.effect(() => () => publisher.close());
  return publisher;
}

/** Drop foreign/replayed notifications; malformed owned activity is an error. */
export function acceptDshActivity(params, expected, lastSequence) {
  if (!object(params) || params.requestId !== expected.requestId || params.nonce !== expected.nonce ||
      params.rootSessionId !== expected.rootSessionId) return null;
  if (params.version !== 1 || Object.keys(params).some(key => !['version', 'requestId', 'nonce', 'rootSessionId', 'activity'].includes(key)) ||
      !validDshActivity(params.activity)) throw new Error('malformed owned DSH activity notification');
  return params.activity.sequence > lastSequence ? params.activity : null;
}
