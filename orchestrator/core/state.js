// state.json management (DESIGN.md §5.2): the single source of truth, written
// ONLY by the orchestrator — the supervised never writes the supervisor's
// ledger. Every write is compare-and-set on `revision` and atomic (tmp+rename).
// On restart, mid-flight units are explicitly marked interrupted with their
// artifacts preserved — never silently reset to pending.

import { readFileSync, writeFileSync, renameSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { milestoneOfPacket } from './contracts.js';

export class CasConflictError extends Error {
  constructor(file, expected, found) {
    super(`CAS conflict on ${file}: expected revision ${expected}, found ${found}`);
    this.code = 'CAS_CONFLICT';
  }
}

export class StateError extends Error {}

export function stateFile(taskDir) {
  return join(taskDir, 'state.json');
}

export function stateExists(taskDir) {
  return existsSync(stateFile(taskDir));
}

export function loadState(taskDir) {
  const file = stateFile(taskDir);
  if (!existsSync(file)) throw new StateError(`state.json not found in ${taskDir}`);
  let state;
  try {
    state = JSON.parse(readFileSync(file, 'utf8'));
  } catch (err) {
    throw new StateError(`${file}: invalid JSON: ${err.message}`);
  }
  if (!Number.isInteger(state.revision)) throw new StateError(`${file}: missing integer revision`);
  return state;
}

/**
 * Atomic CAS write: re-read the on-disk revision, refuse to overwrite a state
 * newer than the one the caller based its decisions on.
 */
export function saveState(taskDir, state, { expectedRevision } = {}) {
  const file = stateFile(taskDir);
  if (existsSync(file)) {
    let onDisk;
    try {
      onDisk = JSON.parse(readFileSync(file, 'utf8'));
    } catch (err) {
      throw new StateError(`${file}: unreadable during CAS check: ${err.message}`);
    }
    const expected = expectedRevision ?? state.revision;
    if (onDisk.revision !== expected) {
      throw new CasConflictError(file, expected, onDisk.revision);
    }
  }
  const next = { ...state, revision: (state.revision ?? 0) + 1 };
  const tmp = `${file}.tmp-${process.pid}`;
  writeFileSync(tmp, `${JSON.stringify(next, null, 2)}\n`, 'utf8');
  renameSync(tmp, file);
  return next;
}

function emptyPacketState(packet) {
  return {
    milestone: milestoneOfPacket(packet['packet-id']),
    status: 'pending',
    route: packet.route,
    attempts: 0,
    'failed-attempts': 0,
    'plan-revisions': 0,
    interrupted: false,
    'needs-live-credentials': packet['needs-live-credentials'] ?? false,
    artifacts: [],
    exit: null,
    'base-ref': null,
    commit: null,
    worktree: null,
    evidence: { executor: null, l1: null, review: null, 'defect-injection': null },
    'last-failure-signature': null,
    'retry-history': [],
    'updated-at': null,
  };
}

/**
 * Ensure every loaded packet has a state entry. The Plan Loop may ADD packets
 * mid-run (scope revisions); a packet without a state entry would crash
 * processPacket. Called after every contract reload and at context creation.
 */
export function ensurePacketStates(state, packets) {
  for (const p of packets) {
    if (!(p['packet-id'] in state.packets)) {
      state.packets[p['packet-id']] = emptyPacketState(p);
    }
  }
}

/** Build the initial state.json content for a task (what $flywheel-new emits). */
export function initState(slug, spec, packets, config) {
  const milestones = {};
  for (const m of spec.milestones) {
    milestones[m.id] = { status: 'pending', gate: null, 'closed-at': null };
  }
  const packetStates = {};
  for (const p of packets) {
    packetStates[p['packet-id']] = emptyPacketState(p);
  }
  return {
    'schema-version': 1,
    slug,
    revision: 0,
    status: 'pending',
    'current-milestone': spec.milestones[0]?.id ?? null,
    'plan-session': null,
    milestones,
    packets: packetStates,
    budget: {
      enforce: config?.budget?.enforce ?? 'soft',
      'daily-token-limit': config?.budget?.['daily-token-limit'] ?? null,
      daily: {},
      totals: { input: 0, output: 0, reasoning: 0, 'cache-read': 0, 'cache-write': 0, tokens: 0, 'cost-usd': 0 },
      cache: { 'hit-rate': 0, 'by-provider': {} },
      warnings: [],
    },
    'last-progress-at': null,
    inbox: [],
  };
}

/**
 * Crash-recovery pass (§5.2 v0.8): units found mid-flight after a restart are
 * marked interrupted, artifacts preserved. Returns the list of affected ids.
 */
export function markInterruptedUnits(state, now = new Date().toISOString()) {
  const affected = [];
  for (const [id, p] of Object.entries(state.packets ?? {})) {
    if (p.status === 'executing') {
      p.status = 'interrupted';
      p.interrupted = true;
      p['updated-at'] = now;
      affected.push(id);
    }
  }
  return affected;
}

/** Accumulate usage + cost into budget totals, daily bucket, and per-provider cache stats. */
export function recordUsage(state, usage, { provider = 'unknown', costUsd = null, day }) {
  const b = state.budget;
  const d = day ?? todayKeyOf(new Date());
  b.daily[d] ??= { tokens: 0, 'cost-usd': 0 };
  // Prune the daily ledger to the last 31 days (P2: state.json must not grow
  // unboundedly across long-running tasks).
  const days = Object.keys(b.daily).sort();
  while (days.length > 31) {
    delete b.daily[days.shift()];
  }
  const input = usage?.input ?? 0;
  const output = usage?.output ?? 0;
  const reasoning = usage?.reasoning ?? 0;
  const cacheRead = usage?.cacheRead ?? 0;
  const cacheWrite = usage?.cacheWrite ?? 0;
  const tokens = usage?.totalTokens ?? input + output;
  const cost = costUsd ?? 0;

  for (const t of [b.totals, b.daily[d]]) {
    t.tokens += tokens;
    t['cost-usd'] = Math.round((t['cost-usd'] + cost) * 1e8) / 1e8;
  }
  b.totals.input += input;
  b.totals.output += output;
  b.totals.reasoning += reasoning;
  b.totals['cache-read'] += cacheRead;
  b.totals['cache-write'] += cacheWrite;

  const bp = (b.cache['by-provider'][provider] ??= { input: 0, 'cache-read': 0, 'cache-write': 0, 'hit-rate': 0 });
  bp.input += input;
  bp['cache-read'] += cacheRead;
  bp['cache-write'] += cacheWrite;
  bp['hit-rate'] = bp.input > 0 ? Math.round((bp['cache-read'] / bp.input) * 1000) / 1000 : 0;
  b.cache['hit-rate'] = b.totals.input > 0 ? Math.round((b.totals['cache-read'] / b.totals.input) * 1000) / 1000 : 0;
}

function todayKeyOf(d) {
  return d.toISOString().slice(0, 10);
}
