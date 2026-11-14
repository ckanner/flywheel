import { createHash } from 'node:crypto';
import { BridgeError, record, nonempty, SessionFold } from './protocol.js';

export function nativeDigest(value) {
  const canonical = v => Array.isArray(v) ? v.map(canonical) : record(v)
    ? Object.fromEntries(Object.keys(v).sort().map(k => [k, canonical(v[k])])) : v;
  return createHash('sha256').update(JSON.stringify(canonical(value))).digest('hex');
}

export function validateNative(value, harness = 'dsh') {
  if (value === undefined) return;
  if (!record(value) || value.version !== 1) throw new BridgeError('INVALID_REQUEST', 'native requires version 1');
  if (harness !== 'dsh') return; // Public machine adapters own their native vocabularies.
  const keys = ['version', 'preset', 'agents', 'team', 'workflows', 'web', 'lcu'];
  if (Object.keys(value).some(k => !keys.includes(k)) || !['heavy', 'daily'].includes(value.preset)) {
    throw new BridgeError('INVALID_REQUEST', 'DSH native requires a daily/heavy PTC composition');
  }
  for (const [name, fields] of Object.entries({ agents: ['maxConcurrent', 'maxDepth'], team: [], workflows: [], lcu: ['config'] })) {
    const option = value[name];
    if (option === undefined) continue;
    if (!record(option) || typeof option.enabled !== 'boolean' || Object.keys(option).some(k => !['enabled', ...fields].includes(k))) {
      throw new BridgeError('INVALID_REQUEST', `invalid native.${name}`);
    }
  }
  for (const [key, limit] of [['maxConcurrent', 64], ['maxDepth', 16]]) {
    const limitValue = value.agents?.[key];
    if (limitValue !== undefined && (!Number.isSafeInteger(limitValue) || limitValue < 1 || limitValue > limit)) {
      throw new BridgeError('INVALID_REQUEST', `invalid native.agents.${key}`);
    }
  }
  if ((value.team?.enabled || value.workflows?.enabled) && value.agents?.enabled !== true) {
    throw new BridgeError('INVALID_REQUEST', 'Team and Workflows require enabled native agents');
  }
  if (value.web !== undefined && (!record(value.web) || Object.entries(value.web).some(([k, v]) => !['search', 'fetch'].includes(k) || typeof v !== 'boolean'))) {
    throw new BridgeError('INVALID_REQUEST', 'invalid native.web');
  }
  if (value.lcu?.config !== undefined && (!record(value.lcu.config) || Object.entries(value.lcu.config).some(([k, v]) =>
    ['app', 'command'].includes(k) ? typeof v !== 'string' || !v.trim() || v.includes('\0') :
      ['chrome', 'audio'].includes(k) ? typeof v !== 'boolean' :
        ['allowedApps', 'allowedOrigins'].includes(k) ? !Array.isArray(v) || v.some(s => typeof s !== 'string' || !s.trim()) : true))) {
    throw new BridgeError('INVALID_REQUEST', 'invalid native.lcu.config');
  }
}

export function validateManaged(value, requestId) {
  if (value === undefined) return;
  if (!record(value) || value.version !== 1 || value.invocationId !== requestId ||
      Object.keys(value).some(k => !['version', 'runId', 'invocationId', 'role', 'scope', 'candidateId', 'channel', 'taskDir'].includes(k))) {
    throw new BridgeError('INVALID_REQUEST', 'native admission identity does not match its invocation');
  }
  for (const key of ['invocationId', 'role', 'scope', 'channel', 'taskDir']) nonempty(value[key], `managed.${key}`, 4096);
  if (typeof value.runId !== 'string' || (value.candidateId !== undefined && typeof value.candidateId !== 'string')) {
    throw new BridgeError('INVALID_REQUEST', 'invalid native run/candidate identity');
  }
}

const MAX_NATIVE_EVENTS = 4096;
const MAX_NATIVE_EVENT_BYTES = 2 * 1024 * 1024;
const usageKeys = ['input_tokens', 'output_tokens', 'reasoning_tokens', 'cache_read_tokens', 'cache_write_tokens', 'total_tokens'];
const validUsage = usage => record(usage) && usageKeys.every(key => Number.isSafeInteger(usage[key]) && usage[key] >= 0);
const identityString = value => typeof value === 'string' && value.length > 0 && value.length <= 4096 && !value.includes('\0');

/** Native child histories contribute usage, never the root's answer or verdict. */
export class NativeTree {
  constructor(rootSessionId, request, emit = () => {}) {
    this.rootSessionId = rootSessionId;
    this.request = request;
    this.emit = emit;
    this.children = new Map();
    this.activations = new Map();
    this.runtimeSeen = new Map();
    this.events = [];
    this.sequence = 0;
    this.eventBytes = 0;
    this.observationError = null;
  }
  fail(message) {
    this.observationError ??= message;
    throw new BridgeError('TRANSPORT', message);
  }
  event(kind, facts) {
    const event = { version: 1, sequence: this.sequence + 1, kind, ...facts };
    const bytes = Buffer.byteLength(JSON.stringify(event));
    if (this.events.length >= MAX_NATIVE_EVENTS || this.eventBytes + bytes > MAX_NATIVE_EVENT_BYTES) {
      this.fail('Native observation journal exceeded its bounded event budget');
    }
    this.sequence += 1;
    this.eventBytes += bytes;
    this.events.push(event);
    this.emit(event);
  }
  ownsParent(id) { return id === this.rootSessionId || this.children.has(id); }
  child(id, parentId, local = true) {
    if (!this.request.agents?.enabled || !identityString(id) || id === this.rootSessionId || !this.ownsParent(parentId)) {
      this.fail('Native child dispatch has no admitted parent lineage');
    }
    const existing = this.children.get(id);
    if (existing && (existing.parentId !== parentId || existing.local !== local)) this.fail('Native child changed parent or ownership kind');
    if (existing) return existing;
    const child = { id, parentId, kind: 'agent', local, state: 'created', terminal: false,
      fold: local ? new SessionFold(id, { strict: true }) : null, fingerprints: new Map(), lastEventSequence: -1, currentTurn: -1 };
    this.children.set(id, child);
    this.event('child.created', { id, parentId, local, state: 'created' });
    return child;
  }
  notify(method, params) {
    if (method === 'subagent.started') {
      if (!record(params)) this.fail('Malformed native child publication');
      this.child(params.childSessionId, params.parentSessionId);
      return;
    }
    const child = this.children.get(params?.sessionId ?? params?.childSessionId);
    if (method === 'session.event' && child?.local) {
      const event = params.event;
      if (!record(event) || !Number.isSafeInteger(event.seq)) this.fail('Malformed native child event');
      const digest = nativeDigest(event);
      if (child.fingerprints.has(event.seq)) {
        if (child.fingerprints.get(event.seq) !== digest) this.fail('Native child replay changed a committed event');
        return;
      }
      if (event.seq < child.lastEventSequence) this.fail('Native child events arrived out of order');
      if (child.fold.accept(params)) {
        child.fingerprints.set(event.seq, digest);
        child.lastEventSequence = event.seq;
        const turn = event.data.turn;
        if (event.type === 'turn/start') {
          if (!Number.isSafeInteger(turn) || turn < 0 || turn < child.currentTurn) this.fail('Native child turn identity regressed');
          child.currentTurn = turn;
          child.terminal = false;
          child.state = 'active';
          child.fold.terminal = null;
        } else if (event.type === 'turn/end') {
          if (turn < child.currentTurn) this.fail('A late native terminal belongs to an earlier turn');
          child.currentTurn = turn;
          child.state = event.data.reason.kind;
          // A committed turn ending does not establish residency-epoch settlement.
          // subagent.finished / public subagent/end owns that separate fact.
        }
        this.event('child.event', { id: child.id, parentId: child.parentId, state: child.state,
          eventType: event.type, eventSequence: event.seq, turnId: Number.isSafeInteger(turn) ? String(turn) : child.fold.turnId || null });
      }
    }
    if (method === 'subagent.finished') {
      if (!child) {
        if (this.ownsParent(params?.parentSessionId)) this.fail('Native child settled without a published start');
        return;
      }
      if (params.parentSessionId !== child.parentId || !identityString(params.stopReason)) this.fail('Native child terminal changed parent or lacks a stop reason');
      if (child.terminal && child.state === params.stopReason) return;
      child.state = params.stopReason;
      child.terminal = true;
      this.event('child.finished', { id: child.id, parentId: child.parentId, state: child.state });
    }
  }
  /** Fold the SDK policy's durable public lifecycle observations, idempotently. */
  observeRuntime(events) {
    if (!Array.isArray(events) || events.length > MAX_NATIVE_EVENTS) this.fail('Malformed native runtime journal');
    for (const event of events) {
      if (!['subagent.started', 'subagent.finished', 'agent.admission-rejected'].includes(event?.type)) continue;
      if (event.version !== 1 || !Number.isSafeInteger(event.journalSequence) || event.journalSequence < 1 ||
          event.rootSessionId !== this.rootSessionId) this.fail('Unowned native runtime lifecycle observation');
      const fingerprint = nativeDigest(event);
      if (this.runtimeSeen.has(event.journalSequence)) {
        if (this.runtimeSeen.get(event.journalSequence) !== fingerprint) this.fail('Native runtime journal changed an observed event');
        continue;
      }
      if (this.runtimeSeen.size >= MAX_NATIVE_EVENTS) this.fail('Native runtime journal exceeded its bounded event budget');
      if (event.type === 'agent.admission-rejected') {
        const child = this.children.get(event.id);
        if (!child || event.id === this.rootSessionId) continue;
        if (!child.local || child.parentId !== event.parentId || event.matched !== false) this.fail('Native child rejection changed its owned identity');
        const active = [...this.activations.values()].some(value => value.id === child.id && !value.terminal);
        const proven = event.beforeModelDispatch === true && event.parentLive === true && !active;
        child.rejectedAdmissions ??= [];
        child.rejectedAdmissions.push({ journalSequence: event.journalSequence, beforeModelDispatch: proven });
        // Rejecting a new attempt never resets previous paid turns. Only a
        // fresh, wholly undispatched child can acquire a proved-zero usage.
        if (proven && child.fold.assistantCount === 0 && child.fold.terminal === null) {
          child.state = 'admission-rejected';
          child.terminal = true;
          child.admissionRejected = true;
        } else if (!proven) child.terminal = false;
        this.runtimeSeen.set(event.journalSequence, fingerprint);
        this.event('child.admission-rejected', { id: child.id, parentId: child.parentId, beforeModelDispatch: proven });
        continue;
      }
      const info = event.child;
      if (!record(info) || !identityString(info.runId) || !identityString(info.provider) || typeof info.local !== 'boolean') {
        this.fail('Malformed native runtime child identity');
      }
      const child = this.child(info.id, info.parentSessionId, info.local);
      let activation = this.activations.get(info.runId);
      if (activation && (activation.id !== info.id || activation.parentId !== info.parentSessionId ||
          activation.provider !== info.provider || activation.local !== info.local)) this.fail('Native runtime activation changed identity');
      if (event.type === 'subagent.started') {
        if (!activation) {
          activation = { runId: info.runId, id: info.id, parentId: info.parentSessionId, provider: info.provider,
            local: info.local, state: 'active', terminal: false };
          this.activations.set(info.runId, activation);
          child.admissionRejected = false;
          // Local wire events may already have reported settlement before this
          // journal is read. An active epoch still appears in completion().
          if (!info.local) { child.state = 'active'; child.terminal = false; }
        }
      } else {
        if (!activation || !identityString(info.stopReason)) this.fail('Native runtime terminal has no attributable start');
        if (activation.terminal && activation.state !== info.stopReason) this.fail('Native runtime terminal outcome changed');
        activation.state = info.stopReason;
        activation.terminal = true;
        child.state = info.stopReason;
        child.terminal = ![...this.activations.values()].some(value => value.id === child.id && !value.terminal);
      }
      this.runtimeSeen.set(event.journalSequence, fingerprint);
      this.event(event.type, { id: child.id, parentId: child.parentId, runId: info.runId,
        provider: info.provider, local: info.local, state: activation.state });
    }
  }
  /** Capture this at root completion, before shutdown can cancel pending work. */
  completion() {
    const pending = new Set([...this.children.values()].filter(child => !child.terminal).map(child => child.id));
    for (const activation of this.activations.values()) if (!activation.terminal) pending.add(activation.id);
    const pendingIds = [...pending].sort();
    return { confirmed: this.observationError === null && pendingIds.length === 0, pendingIds };
  }
  receipt(effective, cleanup) {
    return { version: 1, requested: this.request, effective,
      observed: { events: this.events, truncated: false, complete: this.observationError === null,
        ...(this.observationError ? { error: this.observationError } : {}) },
      children: [...this.children.values()].map(({ fold, fingerprints, lastEventSequence, currentTurn, ...child }) => ({ ...child,
        turnId: fold?.turnId || null, usage: this.knownUsage(child, fold) ? { ...fold.usage } : null })),
      activations: [...this.activations.values()].map(value => ({ ...value })), cleanup,
      usage: { strategy: 'exclusive-committed-session-events', status: this.usageComplete() ? 'complete' : 'partial' } };
  }
  knownUsage(child, fold = child.fold) {
    return child.local && child.terminal && fold?.usageComplete && validUsage(fold.usage) &&
      (fold.assistantCount > 0 && fold.terminal !== null || child.admissionRejected === true && fold.assistantCount === 0);
  }
  usageComplete() {
    return this.observationError === null && this.completion().confirmed && [...this.children.values()].every(child => this.knownUsage(child));
  }
  addUsage(rootUsage) {
    if (!validUsage(rootUsage) || !this.usageComplete()) return null;
    const result = { ...rootUsage };
    for (const { fold } of this.children.values()) for (const key of usageKeys) {
      const sum = result[key] + fold.usage[key];
      if (!Number.isSafeInteger(sum)) return null;
      result[key] = sum;
    }
    return result;
  }
}
