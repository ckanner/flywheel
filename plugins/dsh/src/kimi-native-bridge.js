import { isAbsolute } from 'node:path';
import { realpath } from 'node:fs/promises';
import { OwnedKimiRuntime, KimiSocket, inspectKimiSchemas, initializeKimiCreatedSession, digest, error, pause, checkAbort } from './kimi-native-runtime.js';
import { createTowerCandidate, freezeTowerCandidate, transferTowerCandidate, towerCandidateReceipt, towerPrompt } from './kimi-tower-candidate.js';
import { validateObservationRequest } from './machine-worker.js';

const object = value => value && typeof value === 'object' && !Array.isArray(value);
const identifier = value => typeof value === 'string' && value.trim() && value.length <= 512 && !/[\0\r\n]/.test(value);
const encoded = value => encodeURIComponent(value);
// Only real session events refresh the parent's output watchdog. Forward the
// identity/type, never model reasoning, streamed text or tool argument/results.
// A transport ping is not model progress and is deliberately not included.
const activityEvents = new Set(['agent', 'task', 'turn.started', 'turn.updated', 'turn.ended',
  'item.started', 'item.updated', 'item.delta', 'item.ended', 'item.removed', 'turns.removed', 'interaction']);
const progressEvents = new Set(['turn.updated', 'item.updated', 'item.delta']);

// Liveness needs evidence of recent work, not one log line per token. Bound
// progress notifications across the entire tree so a long streamed answer
// cannot exhaust the parent's output buffer. Only a real event can emit one;
// no timer, ping, or locally synthesized heartbeat extends the idle deadline.
export function createKimiActivityReporter(idleTimeoutMs, emit, now = () => performance.now()) {
  const interval = Math.min(1000, Math.max(1, Math.floor(idleTimeoutMs / 4)));
  const pendingTypes = new Set();
  let lastProgress = -Infinity;
  return frame => {
    if (!activityEvents.has(frame.type) || !identifier(frame.agent_id)) return;
    const event = { type: frame.type, agentId: frame.agent_id, taskId: frame.task_id,
      turnId: frame.turn_id ?? frame.turn?.turn_id, itemId: frame.item_id ?? frame.item?.item_id,
      status: frame.status ?? frame.turn?.status, outcome: frame.outcome ?? frame.turn?.outcome };
    if (progressEvents.has(frame.type)) {
      pendingTypes.add(frame.type);
      const at = now();
      if (at - lastProgress < interval) return;
      lastProgress = at;
      event.coalescedTypes = [...pendingTypes];
      pendingTypes.clear();
    }
    emit(event);
  };
}

export function validateKimiNativeStart(input) {
  if (!object(input) || input.type !== 'start' || input.protocol !== 1 || !identifier(input.requestId)
      || Object.keys(input).some(key => !['type', 'protocol', 'requestId', 'cwd', 'workspaceCwd', 'model', 'reasoningEffort', 'permissionMode', 'prompt', 'timeoutMs', 'idleTimeoutMs', 'sessionId', 'sessionTitle', 'persistent', 'native', 'managed'].includes(key))
      || !identifier(input.cwd) || !isAbsolute(input.cwd) || !identifier(input.model) || !identifier(input.reasoningEffort)
      || (input.workspaceCwd !== undefined && (!identifier(input.workspaceCwd) || !isAbsolute(input.workspaceCwd)))
      || (input.persistent !== undefined && typeof input.persistent !== 'boolean')
      || input.permissionMode !== 'danger-full-access' || typeof input.prompt !== 'string' || !input.prompt.trim()
      || Buffer.byteLength(JSON.stringify(input)) > 1024 * 1024 || !Number.isSafeInteger(input.timeoutMs) || input.timeoutMs <= 0
      || (input.idleTimeoutMs !== undefined && (!Number.isSafeInteger(input.idleTimeoutMs) || input.idleTimeoutMs <= 0))
      || (input.sessionId !== undefined && (!identifier(input.sessionId) || input.persistent !== true))) {
    throw error('INVALID_REQUEST', 'Expected a bounded protocol 1 Kimi start request with explicit full-access permission');
  }
  validateObservationRequest(input, class extends Error { constructor(code, message) { super(message); this.code = code; } });
  const native = input.native, managed = input.managed;
  if (!object(native) || native.version !== 1 || !['agent', 'swarm', 'tower'].includes(native.mode)
      || Object.keys(native).some(key => !['version', 'mode', 'agents', 'agentProfile', 'tower'].includes(key))
      || (native.agentProfile !== undefined && !identifier(native.agentProfile))
      || (native.agents !== undefined && (!object(native.agents) || typeof native.agents.enabled !== 'boolean'
        || Object.keys(native.agents).some(key => !['enabled', 'maxConcurrent'].includes(key))))
      || (native.tower !== undefined && (!object(native.tower) || native.mode !== 'tower'
        || Object.keys(native.tower).some(key => key !== 'branch') || (native.tower.branch !== undefined && !identifier(native.tower.branch))))) {
    throw error('INVALID_NATIVE_PROFILE', 'The requested Kimi native profile is invalid or unsupported');
  }
  if (native.agents?.maxConcurrent !== undefined) throw error('UNSUPPORTED_HARD_CAP', 'The current Kimi API has no enforceable whole-tree agents.maxConcurrent; native batch/Tower limits are not that guarantee');
  if (native.agents?.enabled === false && native.mode !== 'agent') throw error('INVALID_NATIVE_PROFILE', 'Swarm and Tower require native agents to be enabled');
  if (native.mode === 'tower' && input.sessionId !== undefined) throw error('TOWER_RESUME_UNSUPPORTED', 'Tower uses a fresh isolated repository and cannot resume an earlier source candidate');
  if (!object(managed) || managed.version !== 1 || typeof managed.runId !== 'string' || managed.runId.length > 512 || /[\0\r\n]/.test(managed.runId)
      || ['invocationId', 'role', 'scope', 'channel', 'taskDir'].some(key => !identifier(managed[key])) || managed.invocationId !== input.requestId
      || Object.keys(managed).some(key => !['version', 'runId', 'invocationId', 'role', 'scope', 'candidateId', 'channel', 'taskDir'].includes(key))
      || !isAbsolute(managed.taskDir) || (managed.candidateId !== undefined && !identifier(managed.candidateId))) {
    throw error('INVALID_REQUEST', 'Kimi native execution requires an orchestrator-owned role and task binding');
  }
  return input;
}

/** A replacement-based projection; HTTP acceptance and live turns may race. */
export class KimiConversation {
  static #events = new Set(['agent', 'turns', 'turns.failed', 'turn.started', 'turn.ended', 'turn.updated',
    'item.started', 'item.ended', 'item.updated', 'item.delta', 'item.removed', 'turns.removed', 'task', 'task.list', 'interaction']);
  #sizes = new WeakMap();
  constructor() { this.agents = new Map(); this.turns = new Map(); this.tasks = new Map(); this.snapshots = new Set(); this.pending = new Map(); this.lastChange = Date.now(); this.bytes = 0; this.unmeteredWork = false; }
  // Budget the JSON payload actually retained by the projection. Transport
  // frame limits are separate: repeated snapshots and discarded notifications
  // do not consume a lifetime allowance. Cached field/array sizes let a delta
  // update its item and turn without serializing the accumulated answer again.
  #measure(value) {
    if (value === undefined) return undefined;
    if (value === null || typeof value !== 'object') return { bytes: Buffer.byteLength(JSON.stringify(value)),
      tail: typeof value === 'string' && value.length ? value.charCodeAt(value.length - 1) : undefined };
    const cached = this.#sizes.get(value);
    if (cached) return cached;
    let info;
    if (Array.isArray(value)) {
      info = { bytes: 2 + Math.max(0, value.length - 1) + value.reduce((sum, item) => sum + (this.#measure(item)?.bytes ?? 4), 0) };
    } else {
      info = this.#fields({ bytes: 2, fields: new Map() }, Object.entries(value).map(([key, item]) => [key, this.#measure(item)]));
    }
    this.#sizes.set(value, info);
    return info;
  }
  #fields(prior, changes) {
    const fields = new Map(prior.fields);
    let bytes = prior.bytes - 2 - Math.max(0, fields.size - 1);
    for (const [key, value] of changes) {
      const old = fields.get(key);
      if (old) bytes -= old.keyBytes + 1 + old.bytes;
      if (value) {
        const keyBytes = old?.keyBytes ?? Buffer.byteLength(JSON.stringify(key));
        fields.set(key, { bytes: value.bytes, tail: value.tail, keyBytes });
        bytes += keyBytes + 1 + value.bytes;
      } else fields.delete(key);
    }
    return { bytes: bytes + 2 + Math.max(0, fields.size - 1), fields };
  }
  #patch(prior, updates) {
    const next = { ...prior, ...updates };
    this.#sizes.set(next, this.#fields(this.#measure(prior), Object.entries(updates).map(([key, value]) => [key, this.#measure(value)])));
    return next;
  }
  #reserve(delta) {
    if (this.bytes + delta > 64 * 1024 * 1024) throw error('OBSERVATION_LIMIT', 'Kimi exceeded the bounded retained observation limit');
    this.bytes += delta;
  }
  #replace(map, key, next, limit = Infinity) {
    if (!map.has(key) && map.size >= limit) throw error('OBSERVATION_LIMIT', 'Kimi exceeded the bounded observation count');
    this.#reserve(this.#measure(next).bytes - (this.#measure(map.get(key))?.bytes ?? 0));
    map.set(key, next);
  }
  #remove(map, key) {
    const prior = map.get(key);
    if (prior !== undefined) { this.#reserve(-this.#measure(prior).bytes); map.delete(key); }
  }
  #setArray(turn, field, array, info = this.#measure(array), mutate = () => {}) {
    const prior = this.#measure(turn), next = this.#fields(prior, [[field, info]]);
    this.#reserve(next.bytes - prior.bytes);
    mutate();
    turn[field] = array;
    this.#sizes.set(array, info);
    this.#sizes.set(turn, next);
  }
  #setItem(turn, index, item) {
    const items = turn.items, before = this.#measure(items).bytes;
    const bytes = before + this.#measure(item).bytes - (index < 0 ? 0 : this.#measure(items[index]).bytes)
      + (index < 0 && items.length ? 1 : 0);
    this.#setArray(turn, 'items', items, { bytes }, () => { if (index < 0) items.push(item); else items[index] = item; });
  }
  agentTurns(id) {
    if (!this.turns.has(id)) {
      if (this.turns.size >= 512) throw error('OBSERVATION_LIMIT', 'Kimi exceeded the bounded child observation limit');
      this.turns.set(id, new Map());
    }
    return this.turns.get(id);
  }
  mergeTurn(agentId, turn) {
    if (!identifier(agentId) || !object(turn) || !identifier(turn.turn_id) || !Number.isSafeInteger(turn.ordinal) || turn.ordinal < 0
        || (turn.items != null && (!Array.isArray(turn.items) || turn.items.some(item => !object(item))))
        || (turn.steps != null && (!Array.isArray(turn.steps) || turn.steps.some(step => !object(step))))) throw error('WS_PROTOCOL', 'Kimi turn identity, ordinal or content is invalid');
    const turns = this.agentTurns(agentId), prior = turns.get(turn.turn_id);
    const updates = { ...turn, items: turn.items?.map(item => ({ ...item })) ?? prior?.items ?? [] };
    if (Array.isArray(turn.steps)) updates.steps = turn.steps.map(step => ({ ...step }));
    this.#replace(turns, turn.turn_id, prior ? this.#patch(prior, updates) : updates, 4096);
  }
  fold(frame) {
    if (!identifier(frame?.agent_id) || !KimiConversation.#events.has(frame.type)) return;
    this.lastChange = Date.now();
    const agentId = frame.agent_id, turns = this.agentTurns(agentId);
    switch (frame.type) {
      case 'agent': this.#replace(this.agents, agentId, { ...frame }); if (frame.compacting) this.unmeteredWork = true; break;
      case 'turns':
        if (!Array.isArray(frame.turns)) throw error('WS_PROTOCOL', 'Kimi conversation snapshot is invalid');
        if (this.snapshots.has(agentId)) throw error('CONVERSATION_RESET', 'Kimi replaced a subscribed conversation; this invocation cannot certify continuous observation');
        for (const turn of frame.turns) this.mergeTurn(agentId, turn);
        this.snapshots.add(agentId); break;
      case 'turns.failed': throw error('CONVERSATION_UNAVAILABLE', 'Kimi could not load the subscribed conversation');
      case 'turn.started': case 'turn.ended': this.mergeTurn(agentId, frame.turn); break;
      case 'turn.updated': {
        const turn = turns.get(frame.turn_id);
        if (turn && frame.step) {
          if (!object(frame.step)) throw error('WS_PROTOCOL', 'Kimi step update is invalid');
          this.#setArray(turn, 'steps', [...(turn.steps || []).filter(step => step.index !== frame.step.index), { ...frame.step }]);
        }
        break;
      }
      case 'item.started': case 'item.ended': {
        const turn = turns.get(frame.turn_id);
        if (!turn || !object(frame.item)) throw error('WS_PROTOCOL', 'Kimi item has no matching turn');
        const index = turn.items.findIndex(item => item.item_id === frame.item.item_id);
        this.#setItem(turn, index, { ...frame.item });
        break;
      }
      case 'item.updated': case 'item.delta': {
        const turn = turns.get(frame.turn_id), index = turn?.items.findIndex(item => item.item_id === frame.item_id);
        const item = index >= 0 ? turn.items[index] : undefined;
        if (!item) throw error('WS_PROTOCOL', 'Kimi item update has no matching item');
        if (frame.type === 'item.updated') {
          if (!object(frame.set)) throw error('WS_PROTOCOL', 'Kimi item replacement fields are invalid');
          this.#setItem(turn, index, this.#patch(item, frame.set));
        } else if (['text', 'input_text', 'output'].includes(frame.field) && typeof frame.delta === 'string') {
          const text = item[frame.field] ?? '';
          if (typeof text !== 'string') throw error('WS_PROTOCOL', 'Kimi text delta targets a non-text field');
          const prior = this.#measure(item), before = typeof item[frame.field] === 'string' ? prior.fields.get(frame.field) : { bytes: 2 };
          const added = this.#measure(frame.delta);
          // JSON escapes lone surrogates (6 bytes each), but joining a high/low
          // pair across delta boundaries encodes one 4-byte UTF-8 character.
          const paired = before.tail >= 0xd800 && before.tail <= 0xdbff
            && frame.delta.charCodeAt(0) >= 0xdc00 && frame.delta.charCodeAt(0) <= 0xdfff;
          const info = { bytes: before.bytes + added.bytes - 2 - (paired ? 8 : 0), tail: frame.delta.length ? added.tail : before.tail };
          const next = { ...item, [frame.field]: text + frame.delta };
          this.#sizes.set(next, this.#fields(prior, [[frame.field, info]]));
          this.#setItem(turn, index, next);
        }
        break;
      }
      case 'item.removed': {
        const turn = turns.get(frame.turn_id); if (turn) this.#setArray(turn, 'items', turn.items.filter(item => item.item_id !== frame.item_id)); break;
      }
      case 'turns.removed': for (const id of frame.turn_ids || []) this.#remove(turns, id); break;
      case 'task': this.#replace(this.tasks, agentId + '/' + frame.task_id, { ...frame }, 4096); break;
      case 'task.list':
        // An absent running task is no longer active, but retain its observation
        // for accounting rather than silently deleting the child's identity.
        for (const [key, task] of this.tasks) if (task.agent_id === agentId && task.status === 'running'
            && !(frame.tasks || []).some(next => next.task_id === task.task_id)) this.#replace(this.tasks, key, this.#patch(task, { status: 'ended', outcome: task.outcome || 'unknown' }));
        for (const task of frame.tasks || []) this.#replace(this.tasks, agentId + '/' + task.task_id, { ...task, agent_id: agentId }, 4096);
        break;
      case 'interaction': {
        const key = agentId + '/' + (frame.interaction_id || frame.id);
        if (frame.status === 'pending') this.#replace(this.pending, key, { ...frame }); else this.#remove(this.pending, key);
        break;
      }
    }
  }
  newTurns(agentId, baseline = -1) { return [...this.agentTurns(agentId).values()].filter(turn => turn.ordinal > baseline && turn.kind === 'agent').sort((a, b) => a.ordinal - b.ordinal); }
  matchingPrompt(promptId) { return this.newTurns('main').find(turn => turn.items.some(item => item.type === 'user_message' && item.prompt_ids?.includes(promptId))); }
  childIds(baseline = -1) {
    const ids = new Set([...this.tasks.values()].map(task => task.child_agent_id).filter(Boolean));
    for (const [agentId] of this.turns) for (const turn of this.newTurns(agentId, agentId === 'main' ? baseline : -1)) {
      for (const item of turn.items) for (const ref of item.agent_refs || []) if (identifier(ref.agent_id)) ids.add(ref.agent_id);
    }
    ids.delete('main');
    return ids;
  }
  quiescent() {
    return !this.pending.size && ![...this.tasks.values()].some(task => task.status === 'running')
      && ![...this.agents.values()].some(agent => agent.status === 'running' || agent.compacting);
  }
}

export function kimiObservedUsage(conversation, baseline, mode, { resumed = false } = {}) {
  const partial = [], sum = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0 };
  let complete = mode !== 'tower' && !conversation.unmeteredWork, stepsSeen = 0;
  if ([...conversation.tasks.values()].some(task => task.kind === 'subagent' && !task.child_agent_id)) complete = false;
  const children = conversation.childIds(baseline);
  // A resumed root can resume an old child. The API has no invocation ledger
  // for that child's preceding turns, so do not re-bill its whole history.
  const unscopedChildren = resumed && children.size > 0;
  if (unscopedChildren) complete = false;
  for (const childId of children) if (!conversation.snapshots.has(childId) || !conversation.newTurns(childId).length) complete = false;
  for (const [agentId] of conversation.turns) {
    if (resumed && agentId !== 'main') continue;
    const floor = agentId === 'main' ? baseline : -1;
    if ([...conversation.agentTurns(agentId).values()].some(turn => turn.ordinal > floor
        && turn.items.some(item => item.type === 'system' && item.kind === 'compaction'))) complete = false;
    const turns = conversation.newTurns(agentId, floor);
    for (const turn of turns) {
      if (turn.status !== 'ended' || !turn.steps?.length) complete = false;
      for (const item of turn.items) if (item.type === 'tool_call' && /^(?:Agent|AgentSwarm|TowerSpawn)$/.test(item.name || '') && !item.agent_refs?.length) complete = false;
      for (const step of turn.steps || []) {
        const usage = step.usage;
        if (!usage || ['input_other', 'output', 'input_cache_read', 'input_cache_creation'].some(key => !Number.isSafeInteger(usage[key]) || usage[key] < 0)) { complete = false; continue; }
        stepsSeen++;
        partial.push({ agentId, turnId: turn.turn_id, step: step.index, usage });
        sum.input += usage.input_other + usage.input_cache_read + usage.input_cache_creation;
        sum.output += usage.output; sum.cacheRead += usage.input_cache_read; sum.cacheWrite += usage.input_cache_creation;
      }
    }
  }
  sum.totalTokens = sum.input + sum.output;
  return { usage: complete && stepsSeen && Object.values(sum).every(Number.isSafeInteger) ? sum : null, observedSteps: partial,
    scope: mode === 'tower' ? 'observed-agents-only; Tower has no complete tree usage API' : unscopedChildren ? 'root-only; resumed-child-history-is-not-invocation-scoped' : complete ? 'all-observed-agent-steps' : 'partial-or-unknown' };
}

function sessionBinding(input) {
  const { role, scope, channel, taskDir } = input.managed;
  // Go's sessionHash treats JSON objects canonically. Formatting a config or
  // saving it through the editor must not invalidate equivalent Kimi history.
  const canonical = value => Array.isArray(value) ? value.map(canonical) : object(value)
    ? Object.fromEntries(Object.keys(value).sort().map(key => [key, canonical(value[key])])) : value;
  return digest(canonical({ version: 1, cwd: input.cwd, workspaceCwd: input.workspaceCwd || input.cwd,
    model: input.model, reasoningEffort: input.reasoningEffort, permissionMode: input.permissionMode,
    native: input.native, role, scope, channel, taskDir }));
}

/** Bridge emits only ready/event frames; the caller writes the returned final once. */
export async function runKimiWorker(raw, { emit = () => {}, env = process.env, signal, stderr = () => {} } = {}) {
  let input = raw, runtime, socket, sessionId = '', ready = false, dispatch = 'not-dispatched', baseline = -1,
    resumed = false, ownsSession = false, candidate, transfer, failure, matched = false, schema, environment, effective = {}, observedUsage,
    cleanup = { confirmed: true, queueDrained: true, tasksStopped: true, remoteControlOff: true }, binding,
    completion = { confirmed: false, pendingIds: [] }, lastQueue, promptMapping;
  let completed = false, promptId, requestedTurnId = '', acceptingEvents = false;
  const conversation = new KimiConversation(), subscribed = new Set(), subscribing = new Map();
  const controller = new AbortController();
  const cancel = () => controller.abort(signal?.reason);
  if (signal?.aborted) cancel(); else signal?.addEventListener('abort', cancel, { once: true });
  let deadlineTimer, reportActivity;
  const send = (type, content) => emit({ protocol: 1, requestId: input?.requestId || 'invalid-request', type, sessionId, ...content });
  const sessionPath = suffix => `/api/v1/sessions/${encoded(sessionId)}${suffix}`;
  const observe = frame => {
    if (frame.session_id !== sessionId || !identifier(frame.agent_id)) return;
    conversation.fold(frame);
    if (ready) reportActivity(frame);
    const turn = promptId && conversation.matchingPrompt(promptId);
    if (turn) { requestedTurnId = turn.turn_id; dispatch = 'dispatched'; }
  };
  const subscribe = async (agentId, timeout = 3000) => {
    if (subscribed.has(agentId)) return;
    if (subscribing.has(agentId)) return subscribing.get(agentId);
    const promise = socket.command('subscribe', { session_id: sessionId, agent_ids: [agentId] }, timeout).then(() => { subscribed.add(agentId); });
    subscribing.set(agentId, promise);
    try { await promise; } finally { subscribing.delete(agentId); }
  };
  const syncChildren = async (deadline = Infinity) => {
    if (!acceptingEvents) return;
    for (const id of conversation.childIds(baseline)) {
      if (Date.now() >= deadline) throw error('CLEANUP_TIMEOUT', 'Kimi child cleanup exceeded its grace');
      await subscribe(id, Math.min(3000, deadline - Date.now()));
    }
  };
  const getQueue = async options => { lastQueue = await runtime.request(sessionPath('/prompts'), undefined, options); return lastQueue; };
  const idle = async options => {
    const [session, queue] = await Promise.all([runtime.request(sessionPath(''), undefined, options), getQueue(options)]);
    if (!object(queue) || !Array.isArray(queue.queued) || typeof session.busy !== 'boolean') throw error('RUNTIME_PROTOCOL', 'Kimi returned an invalid queue or session status');
    return { idle: session.busy === false && session.main_turn_active === false && session.pending_interaction === 'none'
      && !queue.active && queue.queued.length === 0 && conversation.quiescent(), session, queue };
  };
  const readTurns = async (agentId, options = { signal: controller.signal }) => {
    let before, pages = 0;
    const floor = agentId === 'main' ? baseline : -1;
    while (++pages <= 64) {
      checkAbort(options.signal);
      const page = await runtime.request(sessionPath(`/agents/${encoded(agentId)}/turns?page_size=200${before !== undefined ? '&before_ordinal=' + before : ''}`), undefined, options);
      if (!Array.isArray(page.turns)) throw error('RUNTIME_PROTOCOL', 'Kimi turn page is invalid');
      for (const turn of page.turns) if (turn.ordinal > floor) conversation.mergeTurn(agentId, turn);
      if (!page.has_more || !page.turns.length || Math.min(...page.turns.map(turn => turn.ordinal)) <= floor) return;
      const next = Math.min(...page.turns.map(turn => turn.ordinal));
      if (before !== undefined && next >= before) throw error('RUNTIME_PROTOCOL', 'Kimi turn pagination failed to advance');
      before = next;
    }
    throw error('OBSERVATION_LIMIT', 'Kimi turn history exceeds the bounded reconciliation limit');
  };
  try {
    input = validateKimiNativeStart(raw);
    reportActivity = createKimiActivityReporter(input.idleTimeoutMs ?? 300_000, event => send('event', { event }));
    const declaredCwd = input.cwd;
    input = { ...input, cwd: await realpath(input.cwd), managed: { ...input.managed, taskDir: await realpath(input.managed.taskDir) } };
    deadlineTimer = setTimeout(() => controller.abort(error('TIMEOUT', 'The Kimi invocation exceeded its wall timeout')), input.timeoutMs);
    checkAbort(controller.signal);
    if (input.native.mode === 'tower') candidate = await createTowerCandidate(input);
    runtime = new OwnedKimiRuntime(env, input.native.mode);
    stderr('Starting an owned authenticated Kimi web runtime\n');
    await runtime.start(controller.signal);
    const [openapi, asyncapi, remote] = await Promise.all([
      runtime.request('/openapi.json', undefined, { raw: true, signal: controller.signal }),
      runtime.request('/asyncapi.json', undefined, { raw: true, signal: controller.signal }),
      runtime.request('/api/v1/remote-control', undefined, { signal: controller.signal }),
    ]);
    schema = inspectKimiSchemas(openapi, asyncapi, input.native.mode);
    if (remote.state !== 'off') throw error('REMOTE_CONTROL_ACTIVE', 'The owned Kimi runtime has remote control enabled');
    binding = sessionBinding(input);
    const cwd = candidate?.path || input.cwd;
    const agentConfig = { ...(input.native.agentProfile ? { profile: input.native.agentProfile } : {}), model: input.model,
      thinking: input.reasoningEffort, permission_mode: 'yolo', plan_mode: false, swarm_mode: input.native.mode === 'swarm' };
    let session;
    if (input.sessionId) {
      sessionId = input.sessionId;
      session = await runtime.request(sessionPath(''), undefined, { signal: controller.signal });
      if (session.metadata?.flywheel_native_binding !== binding || session.metadata?.cwd !== cwd) throw error('RESUME_BINDING_MISMATCH', 'Kimi history belongs to a different task, role, candidate or native contract');
      resumed = true; ownsSession = true;
    } else {
      const title = input.sessionTitle || `Flywheel · ${input.managed.role}/${input.managed.scope}`;
      const creationRequest = { metadata: { cwd, flywheel_native_binding: binding },
        title, agent_config: agentConfig, environment_id: 'local', environment_cwd: cwd };
      session = await runtime.request('/api/v1/sessions', creationRequest, { signal: controller.signal });
      sessionId = identifier(session?.id) ? session.id : '';
      session = await initializeKimiCreatedSession(runtime, openapi, session, creationRequest, 'flywheel_native_binding', { signal: controller.signal });
      // No session-specific cleanup targets an unverified or foreign owner.
      ownsSession = true;
    }
    if (!identifier(sessionId) || session.id !== sessionId) throw error('RUNTIME_PROTOCOL', 'Kimi returned an invalid session identity');
    if (candidate) await runtime.request(sessionPath('/profile'), { agent_config: { tower_mode: true, tower_base: candidate.branch } }, { signal: controller.signal });
    environment = await runtime.request(sessionPath('/environment'), undefined, { signal: controller.signal });
    if (environment.environment_id !== 'local' || typeof environment.cwd !== 'string' || await realpath(environment.cwd) !== cwd) throw error('ENVIRONMENT_MISMATCH', 'Kimi did not bind the requested local execution directory');
    socket = new KimiSocket(runtime, observe);
    await socket.open(controller.signal);
    await subscribe('main');
    let waitUntil = Date.now() + 5000;
    while (!conversation.agents.has('main') || !conversation.snapshots.has('main')) {
      checkAbort(controller.signal); socket.check();
      if (Date.now() > waitUntil) throw error('ADMISSION_TIMEOUT', 'Kimi did not provide effective agent state and initial history');
      await pause(20);
    }
    const main = conversation.agents.get('main');
    if (main.model !== input.model || main.thinking !== input.reasoningEffort || main.permission_mode !== 'yolo'
        || main.plan_mode !== false || main.swarm_mode !== (input.native.mode === 'swarm') || main.tower_mode !== (input.native.mode === 'tower')) {
      throw error('EFFECTIVE_CONFIG_MISMATCH', 'Kimi effective model, effort, permission or native mode differs from the request');
    }
    if (!(await idle()).idle) throw error('SESSION_BUSY', 'Kimi history has active work, queued input or a pending interaction');
    // OpenAPI intentionally omits the session action dispatcher. Probe actual
    // idempotent abort on an idle, owned session instead of inventing a schema.
    const abortProbe = await runtime.request(sessionPath(':abort'), {});
    if (abortProbe.aborted !== true) throw error('ABORT_UNSUPPORTED', 'Kimi did not acknowledge the owned-session abort capability');
    baseline = Math.max(-1, ...[...conversation.agentTurns('main').values()].map(turn => turn.ordinal));
    acceptingEvents = true;
    effective = { matched: true, mode: input.native.mode, model: main.model, reasoningEffort: main.thinking,
      permissionMode: input.permissionMode, permission: main.permission_mode, plan: main.plan_mode,
      swarm: main.swarm_mode, tower: main.tower_mode, agentProfile: input.native.agentProfile || 'agent',
      agents: input.native.agents?.enabled !== false, concurrency: { enforcement: 'native', scope: 'native-batch-or-subsystem', upperBound: null, hardTreeCap: false } };
    matched = true; ready = true;
    send('ready', { cwd: input.cwd, workspaceCwd: input.workspaceCwd || input.cwd, model: input.model,
      reasoningEffort: input.reasoningEffort, permissionMode: input.permissionMode, persistent: Boolean(input.persistent), resumed });
    promptId = 'flywheel-' + digest({ requestId: input.requestId, invocationId: input.managed.invocationId }).slice(0, 40);
    checkAbort(controller.signal);
    let submittedPrompt = input.prompt;
    if (candidate) { const mapped = towerPrompt(input, candidate, declaredCwd); submittedPrompt = mapped.prompt; promptMapping = mapped.mapping; }
    dispatch = 'unknown';
    const prompt = await runtime.request(sessionPath('/prompts'), { content: [{ type: 'text', text: submittedPrompt }], prompt_id: promptId,
      model: input.model, thinking: input.reasoningEffort, permission_mode: 'yolo', swarm_mode: input.native.mode === 'swarm',
      disabled_tools: ['CronCreate', ...(input.native.agents?.enabled === false ? ['Agent', 'AgentSwarm'] : [])],
      metadata: { flywheel_invocation_id: input.managed.invocationId } }, { signal: controller.signal, timeout: 30_000 });
    if (prompt.prompt_id !== promptId || !['running', 'queued'].includes(prompt.status)) {
      throw error('PROMPT_NOT_RUNNING', 'Kimi did not accept the correlated invocation as running or queued');
    }
    let stableSince = 0;
    while (true) {
      checkAbort(controller.signal); socket.check();
      if (runtime.closed) throw error('RUNTIME_EXITED', 'Kimi runtime exited during the invocation');
      if (conversation.pending.size) throw error('INTERACTION_REQUIRED', 'Kimi requires an interactive approval or answer; no response was fabricated');
      await syncChildren();
      const turn = conversation.matchingPrompt(promptId);
      if (turn) { requestedTurnId = turn.turn_id; dispatch = 'dispatched'; }
      if (turn?.status === 'ended') {
        const state = await idle();
        if (state.idle && [...conversation.newTurns('main', baseline)].every(item => item.status === 'ended')
            && [...conversation.childIds(baseline)].every(id => conversation.snapshots.has(id))) {
          stableSince ||= Date.now();
          if (Date.now() - stableSince >= 350 && Date.now() - conversation.lastChange >= 200) break;
        } else stableSince = 0;
      }
      await pause(70);
    }
    for (const agentId of subscribed) await readTurns(agentId);
    await syncChildren();
    if (!(await idle()).idle || [...conversation.childIds(baseline)].some(id => !conversation.snapshots.has(id))
        || [...conversation.turns].some(([id]) => conversation.newTurns(id, id === 'main' ? baseline : -1).some(turn => turn.status !== 'ended'))) {
      throw error('NATIVE_WORK_ACTIVE', 'Kimi revealed additional native work during final reconciliation');
    }
    const turns = conversation.newTurns('main', baseline);
    if (!turns.length || turns.some(turn => turn.status !== 'ended' || turn.outcome !== 'completed')) throw error('TURN_FAILED', 'Kimi did not complete every root continuation successfully');
    completed = true;
  } catch (cause) {
    failure = controller.signal.aborted && typeof controller.signal.reason?.code === 'string' ? controller.signal.reason : cause;
  } finally {
    // Capture completion before cleanup; stopping unfinished work must never
    // turn an incomplete native result into an accepted role result.
    completion = { confirmed: completed, pendingIds: [
      ...[...conversation.tasks.values()].filter(task => task.status === 'running').map(task => 'task:' + task.agent_id + '/' + task.task_id),
      ...[...conversation.agents].filter(([, agent]) => agent.status === 'running' || agent.compacting).map(([id]) => 'agent:' + id),
      ...[...conversation.pending.keys()].map(id => 'interaction:' + id),
      ...(lastQueue?.queued || []).map(prompt => 'prompt:' + prompt.prompt_id),
      ...(lastQueue?.active ? ['prompt:' + lastQueue.active.prompt_id] : []),
    ] };
    if (completion.pendingIds.length) completion.confirmed = false;
    clearTimeout(deadlineTimer);
    signal?.removeEventListener('abort', cancel);
    // Cleanup has a separate bounded grace. An aborted model request must not
    // cancel queue withdrawal, child stop or owned-server shutdown requests.
    if (runtime) {
      const admittedRuntime = Boolean(runtime.meta);
      cleanup = { confirmed: false, queueDrained: !ownsSession, tasksStopped: !ownsSession, remoteControlOff: admittedRuntime ? false : null };
      const cleanupDeadline = Date.now() + 6000;
      const cleanupSignal = AbortSignal.timeout(6000);
      const options = { signal: cleanupSignal, timeout: 1000 };
      try {
        if (ownsSession && sessionId && runtime.base && !runtime.closed) {
          do {
            checkAbort(cleanupSignal);
            const queue = await getQueue(options);
            for (const prompt of queue.queued || []) {
              try { await runtime.request(sessionPath(`/prompts/${encoded(prompt.prompt_id)}:abort`), {}, options); }
              catch (cause) { if (cause.code !== 'KIMI_API_40402') throw cause; }
            }
            if (queue.active || !conversation.quiescent()) await runtime.request(sessionPath(':abort'), {}, options);
            if (socket && !socket.closed) {
              await syncChildren(cleanupDeadline);
              for (const task of conversation.tasks.values()) if (task.status === 'running') {
                checkAbort(cleanupSignal);
                try {
                  const reply = await socket.command('task.stop', { session_id: sessionId, agent_id: task.agent_id, task_id: task.task_id }, Math.max(1, Math.min(1000, cleanupDeadline - Date.now())));
                  if (reply.task) conversation.fold({ ...reply.task, type: 'task', agent_id: task.agent_id });
                } catch (cause) { if (cause.code !== 'KIMI_WS_40406') throw cause; }
              }
            }
            const state = await idle(options);
            cleanup.queueDrained = !state.queue.active && state.queue.queued.length === 0;
            cleanup.tasksStopped = state.idle;
            if (state.idle) break;
            await pause(75);
          } while (Date.now() < cleanupDeadline);
          if (socket && !socket.closed && cleanup.tasksStopped) {
            for (const agentId of subscribed) { try { await readTurns(agentId, options); } catch { /* Missing final usage stays unknown. */ } }
          }
        }
        if (admittedRuntime && runtime.base && !runtime.closed) {
          let remote = await runtime.request('/api/v1/remote-control', undefined, options);
          if (remote.state !== 'off') {
            await runtime.request('/api/v1/remote-control', { enabled: false }, options);
            remote = await runtime.request('/api/v1/remote-control', undefined, options);
          }
          cleanup.remoteControlOff = remote.state === 'off';
        }
      } catch (cause) { cleanup.error = cause.code || 'CLEANUP_FAILED'; }
      socket?.close();
      const process = await runtime.stop();
      cleanup.process = process;
      cleanup.confirmed = cleanup.queueDrained && cleanup.tasksStopped && (!admittedRuntime || cleanup.remoteControlOff) && process.exited && process.processesConfirmed;
      if (!cleanup.confirmed && !failure) failure = error('CLEANUP_UNKNOWN', 'Kimi work or process cleanup could not be confirmed');
    }
  }
  if (candidate && cleanup.confirmed) {
    try {
      candidate = await freezeTowerCandidate(candidate, input);
      if (completed && !failure) transfer = await transferTowerCandidate(candidate, input, true);
    } catch (cause) { failure ||= error('CANDIDATE_TRANSFER_FAILED', cause.message); transfer = { applied: false, reason: cause.message }; }
  }
  observedUsage = kimiObservedUsage(conversation, baseline, input?.native?.mode, { resumed });
  let text = conversation.newTurns('main', baseline).flatMap(turn => turn.items.filter(item => item.type === 'assistant_message').map(item => item.text || '')).join('\n');
  if (Buffer.byteLength(text) > 512 * 1024) { failure ||= error('OUTPUT_LIMIT', 'Kimi output exceeds the complete receipt limit'); text = ''; }
  if (completed && !failure && !text.trim()) failure = error('EMPTY_OUTPUT', 'Kimi completed without a final assistant result');
  const cancelled = controller.signal.aborted && controller.signal.reason?.code !== 'TIMEOUT';
  const status = cancelled ? 'cancelled' : failure || !completed ? 'error' : 'ok';
  const turns = conversation.newTurns('main', baseline), terminal = turns.at(-1);
  const outcome = dispatch === 'not-dispatched' ? 'failed' : dispatch === 'unknown' || !terminal || terminal.status !== 'ended' ? 'unknown'
    : terminal.outcome === 'completed' ? 'completed' : terminal.outcome === 'interrupted' ? 'cancelled' : 'failed';
  const native = { version: 1, requested: raw?.native ?? null, matched, effective: { ...effective, matched },
    observed: { server: runtime?.meta ? { id: runtime.meta.server_id, version: runtime.meta.server_version, backend: runtime.meta.backend,
      ownedPid: runtime.child.pid, authenticated: true, local: true, identity: runtime.identity ?? null } : null, schema: schema ?? null, environment: environment ?? null,
      binding: binding ?? null, managed: input?.managed ?? null, promptMapping: promptMapping ?? null, abortProbe: matched, usage: observedUsage, nativeLimits: { wholeTreeUpperBound: null, admission: 'native-runtime', hardMaxConcurrentSupported: false },
      disabledTools: ready ? ['CronCreate', ...(input.native.agents?.enabled === false ? ['Agent', 'AgentSwarm'] : [])] : [], inferenceTested: dispatch === 'dispatched' },
    children: [...conversation.tasks.values()].map(task => ({ taskId: task.task_id, parentAgentId: task.agent_id, agentId: task.child_agent_id ?? null,
      kind: task.kind, status: task.status, outcome: task.outcome ?? null, model: task.model ?? null, thinking: task.thinking ?? null, swarmIndex: task.swarm_index ?? null })),
    completion, cleanup, ...(candidate ? { candidate: towerCandidateReceipt(candidate, transfer) } : {}) };
  return { protocol: 1, requestId: raw?.requestId || 'invalid-request', type: 'result', status, settled: cleanup.confirmed,
    sessionId, text, usage: dispatch === 'dispatched' ? observedUsage.usage : null, costUsd: null, native,
    ...(failure ? { error: { code: typeof failure.code === 'string' ? failure.code : 'KIMI_NATIVE_FAILED', message: failure.message || 'Kimi native invocation failed' } } : {}),
    lifecycle: { protocol: 'kimi-server/v1', requestId: raw?.requestId || 'invalid-request', sessionId, dispatch, outcome,
      cleanupConfirmed: cleanup.confirmed, resumeCandidate: status === 'ok' && Boolean(input.persistent) && input.native.mode !== 'tower',
      resumeAccepted: resumed && ready, usageStatus: dispatch === 'dispatched' && observedUsage.usage !== null ? 'complete' : 'unknown', turnId: requestedTurnId } };
}
