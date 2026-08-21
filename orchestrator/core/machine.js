// Core state machine (DESIGN.md §6 — deliberately thin):
//   for milestone in spec.milestones:
//     packet = plan_loop.issue(milestone)
//     for attempt in range(packet.escalation.max_retries):
//       result = execute_loop.run(packet)      # fresh session + watchdog
//       l1 = verify.run(packet.acceptance.L1)  # orchestrator runs it directly
//       verdict = review_loop.review(...) if l1.pass else fail(l1)
//       if verdict.pass and verify.milestone_gate(milestone): break
//     else:
//       escalate(result)  # back to Plan Loop / inbox a human
//
// The orchestrator is not an agent: every decision below is a mechanically
// checkable state transition (did state.json advance, was the exit code 0,
// are retries/budget exhausted). Anything requiring judgement is delegated
// to the Plan/Review agents or to a human (§2 rule 1).

import { appendFileSync, existsSync, readFileSync, rmSync, writeFileSync, mkdirSync } from 'node:fs';
import { join, relative } from 'node:path';
import { loadSpec, loadPackets, milestoneOfPacket } from './contracts.js';
import { loadState, saveState, initState, stateExists, markInterruptedUnits, recordUsage, ensurePacketStates } from './state.js';
import { evaluateExitContract } from './exit-contract.js';
import { parseDoneReport } from './done-report.js';
import { runGateCommands, getChangedFiles, checkBoundaries, getBaseRef, commitUnitChanges, getDiffText } from './verify.js';
import { LEDGER_FILES, LEDGER_DIRS, isLedgerTaskPath, COMMIT_VOLATILE_FILES } from './ledger.js';
import { classifyError, isRetryable, shouldFallback, computeBackoff, extractRetryAfterMs } from './errors.js';
import { checkBudget, failureSignature, detectSpin } from './circuit-breaker.js';
import { resolveBinding, nextChannel, modelForChannel, envKeyForChannel, missingChannelEnv, authFallbackAllowed } from './fallback.js';
import { appendMetrics, promptHash, estimateCostUsd, priceForModel } from './metrics.js';
import { shouldSample, runDefectInjection } from './defect-injection.js';
import { killActiveChildren } from './watchdog.js';
import { buildExecutePrompt, buildReviewPrompt, buildPlanPrompt, buildJudgePrompt, parseReviewVerdict } from './prompts.js';

const defaultSleep = (ms) => new Promise((r) => setTimeout(r, ms));
export function pidFile(taskDir) {
  return join(taskDir, 'orchestrator.pid');
}

function nowIso() {
  return new Date().toISOString();
}

/**
 * Single-instance guard: a pid file owned by a LIVE process means another
 * orchestrator runs this task — starting anyway would clobber the pid file
 * and make `flywheel stop` kill the wrong (dead) process, orphaning the real
 * one. Stale pid files (dead pid / unreadable) are safe to overwrite.
 */
export function assertNoRunningOrchestrator(taskDir) {
  const file = pidFile(taskDir);
  if (!existsSync(file)) return;
  let pid = null;
  try {
    pid = JSON.parse(readFileSync(file, 'utf8')).pid;
  } catch {
    return; // unreadable = stale, overwrite below
  }
  if (!Number.isInteger(pid)) return;
  try {
    process.kill(pid, 0); // signal 0: existence probe, no signal delivered
  } catch {
    return; // dead process = stale pid file
  }
  throw new Error(`another orchestrator is running this task (pid ${pid}) — stop it first: flywheel stop <slug>`);
}

/**
 * Load everything the machine needs and return the run context.
 */
export function createContext({ taskDir, cwd = process.cwd(), config, drivers, log = () => {}, sleep = defaultSleep }) {
  const spec = loadSpec(taskDir);
  const packetList = loadPackets(taskDir, spec);
  const packets = Object.fromEntries(packetList.map((p) => [p['packet-id'], p]));
  let state;
  let fresh = false;
  if (stateExists(taskDir)) {
    state = loadState(taskDir);
  } else {
    state = initState(spec.slug ?? taskDir.split('/').filter(Boolean).pop(), spec, packetList, config);
    fresh = true;
  }
  // A planner revision may have added packets to the contract layer since the
  // state was last written; every loaded packet must have a state entry.
  ensurePacketStates(state, packetList);
  return {
    taskDir,
    cwd,
    config,
    drivers,
    spec,
    packets,
    packetList,
    state,
    fresh,
    log,
    sleep,
    metricsFile: join(taskDir, 'metrics.jsonl'),
    revision: state.revision,
  };
}

/** CAS-save the working state; all mutations go through here. */
function save(ctx) {
  ctx.state = saveState(ctx.taskDir, ctx.state, { expectedRevision: ctx.revision });
  ctx.revision = ctx.state.revision;
}

function recordRetry(ctx, ps, entry) {
  ps['retry-history'].push({ at: nowIso(), 'planned-delay-ms': 0, ...entry });
}

function touchProgress(ctx) {
  ctx.state['last-progress-at'] = nowIso();
}

function recordSpawnMetrics(ctx, { role, packetId, model, harness, channel, prompt, result, errorCode, attempt }) {
  const usage = result.usage ?? null;
  const costUsd = result.costUsd ?? estimateCostUsd(usage, priceForModel(ctx.config.pricing, model));
  const cacheHitRate = usage && usage.input > 0 ? Math.round(((usage.cacheRead ?? 0) / usage.input) * 1000) / 1000 : null;
  appendMetrics(ctx.metricsFile, {
    role,
    'packet-id': packetId ?? null,
    milestone: packetId ? milestoneOfPacket(packetId) : null,
    model: model ?? null,
    harness: harness ?? result.harness ?? null,
    channel: channel ?? null,
    'prompt-hash': prompt ? promptHash(prompt) : null,
    usage,
    'cache-hit-rate': cacheHitRate,
    'reasoning-chars': result.reasoningChars ?? null,
    'cost-usd': costUsd,
    'duration-ms': result.durationMs ?? null,
    'exit-code': result.exitCode ?? null,
    'error-code': errorCode ?? null,
    attempt: attempt ?? null,
  });
  if (usage) {
    const provider = result.provider ?? channel ?? 'unknown';
    recordUsage(ctx.state, usage, { provider, costUsd });
    // §8-7: cache hit-rate below the per-channel threshold -> soft warning
    // (never stops the loop; a drop means the prompt prefix was disturbed).
    const bp = ctx.state.budget.cache['by-provider'][provider];
    const threshold = ctx.config.channels?.[channel]?.['cache-hit-rate-min'] ?? 0.3;
    if (bp && bp.input >= 1000 && bp['hit-rate'] < threshold) {
      const msg = `cache hit-rate ${(bp['hit-rate'] * 100).toFixed(1)}% below threshold ${Math.round(threshold * 100)}% for ${provider}`;
      if (!ctx.state.budget.warnings.includes(msg)) {
        ctx.state.budget.warnings.push(msg);
        ctx.log(msg);
      }
    }
  }
}

function apiKeysFor(ctx, channel) {
  const envKey = envKeyForChannel(ctx.config, channel);
  if (envKey && process.env[envKey]) return { [envKey]: process.env[envKey] };
  return {};
}

function watchdogOpts(ctx) {
  const w = ctx.config.watchdog ?? {};
  return {
    idleTimeoutMs: w['idle-timeout-ms'],
    wallTimeoutMs: w['wall-timeout-ms'],
    killGraceMs: w['kill-grace-ms'],
  };
}

/**
 * The unit diff judged for boundaries is the EXECUTOR's diff. Only the
 * orchestrator's own ledger files inside the task directory are excluded
 * (constants from core/ledger.js, single source of truth) — contract files
 * (spec.md / task-packet-*.md / config) stay IN: an executor tampering with
 * the contract is a violation.
 */
const CONTRACT_FILE_RE = /^(spec\.md|task-packet-.*\.md|flywheel\.config\.json)$/;

function taskDirPrefix(ctx) {
  return `${relative(ctx.cwd, ctx.taskDir)}/`;
}

function ledgerExcludes(ctx) {
  const prefix = taskDirPrefix(ctx);
  return [...LEDGER_FILES].map((f) => `${prefix}${f}`).concat(LEDGER_DIRS.map((d) => `${prefix}${d}`));
}

/** Volatile runtime files excluded from the unit commit (P1: never commit
 * the live orchestrator.log or append-only metrics). */
function commitExcludes(ctx) {
  const prefix = taskDirPrefix(ctx);
  return [...COMMIT_VOLATILE_FILES].map((f) => `${prefix}${f}`);
}

function partitionChangedFiles(ctx, changedFiles) {
  if (changedFiles === null) return { executorFiles: null, contractViolations: [] };
  const prefix = taskDirPrefix(ctx);
  const executorFiles = [];
  const contractViolations = [];
  for (const f of changedFiles) {
    if (!f.startsWith(prefix)) {
      executorFiles.push(f);
      continue;
    }
    const inner = f.slice(prefix.length);
    if (isLedgerTaskPath(inner)) continue;
    if (CONTRACT_FILE_RE.test(inner)) contractViolations.push({ type: 'contract-file', path: f });
    else executorFiles.push(f);
  }
  return { executorFiles, contractViolations };
}

/** Raw channel API key value (for pi --api-key); NEVER from credentialEnv. */
function channelApiKey(ctx, channel) {
  const envKey = envKeyForChannel(ctx.config, channel);
  return envKey && process.env[envKey] ? process.env[envKey] : null;
}

/**
 * Fail-loud guard (P1-12②): a channel declaring an env-key that is unset must
 * NOT spawn — pi would silently fall back to its global auth.json otherwise.
 * Only enforced for real drivers (mock/fixture drivers need no keys).
 * Returns the missing env var name, or null.
 */
function missingKeyForRealDriver(ctx, role, channel) {
  if ((ctx.config.drivers?.[role] ?? 'real') === 'mock') return null;
  return missingChannelEnv(ctx.config, channel);
}

/**
 * Budget gate before every spawn. Returns 'ok' | 'stop'.
 * Token/cost recording itself never turns off, in any enforce mode.
 */
function budgetGate(ctx) {
  const verdict = checkBudget(ctx.state, ctx.config);
  if (verdict.action === 'stop') {
    ctx.log(`budget HARD stop: ${verdict.reason}`);
    ctx.state.status = 'budget-stopped';
    save(ctx);
    return 'stop';
  }
  if (verdict.action === 'warn') {
    // P2: dedupe by day — a volatile counter in the message would add a new
    // warning per spawn and bloat state.json.
    const msg = `budget soft warning: daily token budget exceeded (${new Date().toISOString().slice(0, 10)})`;
    if (!ctx.state.budget.warnings.includes(msg)) ctx.state.budget.warnings.push(msg);
    ctx.log(msg);
  }
  return 'ok';
}

// ---------------------------------------------------------------------------
// Steps
// ---------------------------------------------------------------------------

/**
 * Live-credentials human checkpoint (§5.1 needs-live-credentials, §9):
 * before the unit executes, pause and wait for a human approval file
 * (approvals/<packet-id>.json, {"approved-env": ["KEY", ...]}). Only the
 * approved keys are injected from the orchestrator's environment into the
 * unit's child processes, and the injection is recorded as evidence.
 * Timeout escalates to the inbox.
 */
async function awaitCredentialsApproval(ctx, packet) {
  const id = packet['packet-id'];
  const ps = ctx.state.packets[id];
  const approvalFile = join(ctx.taskDir, 'approvals', `${id}.json`);
  const pollMs = ctx.config.credentials?.['poll-interval-ms'] ?? 30_000;
  const timeoutMs = ctx.config.credentials?.['timeout-ms'] ?? 86_400_000;
  const deadline = Date.now() + timeoutMs;
  let announced = ps.status === 'awaiting-credentials';

  for (;;) {
    if (existsSync(approvalFile)) {
      let parsed;
      try {
        parsed = JSON.parse(readFileSync(approvalFile, 'utf8'));
      } catch {
        return { ok: false, reason: `approval file ${approvalFile} is not valid JSON` };
      }
      const approvedEnv = parsed['approved-env'];
      if (!Array.isArray(approvedEnv) || !approvedEnv.every((k) => typeof k === 'string')) {
        return { ok: false, reason: `approval file ${approvalFile} needs {"approved-env": ["KEY", ...]}` };
      }
      const injected = {};
      const missing = [];
      for (const key of approvedEnv) {
        if (process.env[key] !== undefined) injected[key] = process.env[key];
        else missing.push(key);
      }
      ps.evidence.credentials = {
        'approved-env': approvedEnv,
        'injected-env': approvedEnv.filter((k) => !missing.includes(k)),
        'missing-env': missing,
        source: `approvals/${id}.json`,
        at: nowIso(),
      };
      ps.status = 'pending';
      ps['updated-at'] = nowIso();
      ctx.log(`${id}: credentials approved by human (${approvedEnv.length} keys${missing.length ? `, missing in env: ${missing.join(',')}` : ''})`);
      save(ctx);
      return { ok: true, injected };
    }

    if (!announced) {
      ps.status = 'awaiting-credentials';
      ps['updated-at'] = nowIso();
      const entry = {
        at: nowIso(),
        'packet-id': id,
        reason: `packet ${id} declares needs-live-credentials: approve by writing docs task approvals/${id}.json {"approved-env": ["KEY", ...]}; the listed keys will be injected for this unit only.`,
      };
      ctx.state.inbox.push(entry);
      appendInbox(ctx, entry);
      ctx.log(`${id}: awaiting live-credentials approval (${approvalFile})`);
      save(ctx);
      announced = true;
    }

    if (Date.now() >= deadline) {
      ps.status = 'inbox';
      ps['updated-at'] = nowIso();
      const entry = {
        at: nowIso(),
        'packet-id': id,
        reason: `credentials approval for ${id} timed out after ${timeoutMs}ms. Human needed.`,
      };
      ctx.state.inbox.push(entry);
      ctx.state.status = 'escalated';
      appendInbox(ctx, entry);
      save(ctx);
      return { ok: false, reason: 'credentials approval timed out' };
    }

    await ctx.sleep(pollMs);
  }
}

/**
 * Execute step with transport retries + channel fallback (§6).
 * On success the packet state is 'executed' with executor evidence recorded.
 */
async function executeStep(ctx, packet) {
  const id = packet['packet-id'];
  const ps = ctx.state.packets[id];

  // Live credentials gate: pause for human approval before any spawn (§9).
  let credentialEnv = {};
  if (packet['needs-live-credentials'] === true) {
    if (ps.evidence.credentials) {
      // Previously approved (e.g. resume): re-inject the approved keys only.
      for (const key of ps.evidence.credentials['injected-env'] ?? []) {
        if (process.env[key] !== undefined) credentialEnv[key] = process.env[key];
      }
    } else {
      const approval = await awaitCredentialsApproval(ctx, packet);
      if (!approval.ok) return { success: false, stopped: true, awaiting: 'credentials' };
      credentialEnv = approval.injected;
    }
  }

  ps.status = 'executing';
  ps.attempts += 1;
  // Boundary judgement anchor (§4.2/§4.4): sampled on the FIRST attempt only —
  // retries keep the original base ref so an executor cannot escape judgement
  // by committing its own earlier attempts (audit fix #5).
  if (ps['base-ref'] == null) {
    ps['base-ref'] = await getBaseRef(ctx.cwd);
  }
  ps['updated-at'] = nowIso();
  save(ctx);

  const binding = resolveBinding(ctx.config, 'execute', packet.route);
  let channel = binding.channel;
  let currentBinding = binding;
  const visitedChannels = new Set();
  const maxTransport = ctx.config.retry?.['max-transport-retries'] ?? 5;
  let lastCode = 'TRANSPORT';

  // The exit contract must judge THIS run's deliverable — a stale done-report
  // from an earlier attempt would turn a no-op retry into a false success.
  const reportRel = `reports/${id}.done.json`;
  rmSync(join(ctx.taskDir, reportRel), { force: true });

  for (let t = 0; t <= maxTransport; t++) {
    if (budgetGate(ctx) === 'stop') return { success: false, stopped: true };
    // Fail loud instead of spawning keyless into pi's auth.json trap (P1-12②).
    const missingKey = missingKeyForRealDriver(ctx, 'execute', channel);
    if (missingKey) {
      recordRetry(ctx, ps, { scope: 'transport', 'error-code': 'AUTH', policy: `fail-loud: channel ${channel} requires env ${missingKey} (not set)`, 'planned-delay-ms': 0 });
      save(ctx);
      return { success: false, errorCode: 'AUTH' };
    }
    const prompt = buildExecutePrompt({ packet, spec: ctx.spec, taskDir: ctx.taskDir, cwd: ctx.cwd, attempt: ps.attempts });
    visitedChannels.add(channel);
    const result = await ctx.drivers.execute.execute({
      packet,
      taskDir: ctx.taskDir,
      cwd: ctx.cwd,
      prompt,
      model: currentBinding.model,
      binding: currentBinding,
      channel,
      apiKey: channelApiKey(ctx, channel),
      apiKeys: { ...apiKeysFor(ctx, channel), ...credentialEnv },
      watchdogOpts: watchdogOpts(ctx),
    });

    // Deliverable = the done-report, present and schema-valid.
    const reportAbs = join(ctx.taskDir, reportRel);
    const reportExists = existsSync(reportAbs);
    let reportParsed = { ok: false, errors: ['missing'] };
    if (reportExists) {
      reportParsed = parseDoneReport(readFileSync(reportAbs, 'utf8'), packet['done-report-format']);
    }
    // A provider error inside the event stream (pi exits 0 even on 401/403)
    // outranks the exit contract.
    const streamCode = result.errorMessage ? classifyError({ exitCode: 1, stderr: result.errorMessage }) : null;
    const contract = streamCode
      ? { ok: false, errorCode: streamCode, reasons: [`stream error: ${result.errorMessage}`] }
      : evaluateExitContract({
          result,
          deliverables: [{ path: reportRel, exists: reportExists, valid: reportParsed.ok, detail: reportParsed.errors?.join(',') }],
        });

    ps.exit = {
      exitCode: result.exitCode,
      signal: result.signal,
      timedOut: result.timedOut,
      idleTimedOut: result.idleTimedOut ?? false,
      wallTimedOut: result.wallTimedOut ?? false,
    };
    const code = contract.ok ? null : contract.errorCode ?? classifyError(result) ?? 'TRANSPORT';
    lastCode = code ?? lastCode;
    recordSpawnMetrics(ctx, { role: 'execute', packetId: id, model: currentBinding.model, harness: result.harness, channel, prompt, result, errorCode: code, attempt: ps.attempts });

    if (contract.ok) {
      ps.status = 'executed';
      ps.interrupted = false;
      ps.evidence.executor = {
        'done-report': reportRel,
        'requirements-covered': reportParsed.report['requirements-covered'],
        'changed-files': reportParsed.report['changed-files'],
        failures: reportParsed.report.failures,
        at: nowIso(),
      };
      if (!ps.artifacts.includes(reportRel)) ps.artifacts.push(reportRel);
      ps['updated-at'] = nowIso();
      touchProgress(ctx);
      save(ctx);
      return { success: true };
    }

    ctx.log(`execute ${id} attempt ${ps.attempts} transport ${t}: ${code} (${contract.reasons.join('; ') || 'spawn error'})`);

    // Channel fallback (降级): rate limit / quota (403) -> next configured
    // channel; AUTH only for subscription channels with a keyed fallback
    // waiting (P0-7).
    if (shouldFallback(code) || authFallbackAllowed(ctx.config, channel, code)) {
      const next = nextChannel(ctx.config, channel);
      const mapped = next ? modelForChannel(ctx.config, next, 'execute', packet.route) : null;
      if (next && !visitedChannels.has(next) && mapped) {
        recordRetry(ctx, ps, { scope: 'fallback', 'error-code': code, policy: `channel ${channel} -> ${next}` });
        ctx.log(`fallback: ${channel} -> ${next} (${code})`);
        channel = next;
        currentBinding = { ...binding, model: mapped.model, harness: mapped.harness ?? binding.harness };
        t -= 1; // fallback does not consume a transport retry
        save(ctx);
        continue;
      }
      if (next && !mapped) {
        ctx.log(`fallback chain ends: no model mapping for execute/${packet.route} on ${next} (P1-12①)`);
      }
    }

    if (!isRetryable(code)) {
      recordRetry(ctx, ps, { scope: 'transport', 'error-code': code, policy: 'no-retry' });
      save(ctx);
      return { success: false, errorCode: code };
    }

    const delay = computeBackoff(t, {
      baseDelayMs: ctx.config.retry?.['base-delay-ms'],
      maxDelayMs: ctx.config.retry?.['max-delay-ms'],
      jitter: ctx.config.retry?.jitter,
      retryAfterMs: extractRetryAfterMs(`${result.stderr}\n${result.stdout}`),
    });
    recordRetry(ctx, ps, { scope: 'transport', 'error-code': code, policy: 'backoff', 'planned-delay-ms': delay });
    save(ctx);
    await ctx.sleep(delay);
  }

  return { success: false, errorCode: lastCode };
}

/**
 * L1 step (§4.4): orchestrator runs the acceptance commands directly, then
 * judges test/boundary integrity from the git diff. Coverage gate = one L1
 * command in the packet; nothing special here.
 */
async function l1Step(ctx, packet) {
  const id = packet['packet-id'];
  const ps = ctx.state.packets[id];
  const timeoutMs = ctx.config.verify?.['command-timeout-ms'];
  const envAllowlist = ctx.config.verify?.['env-allowlist'] ?? [];
  const gate = await runGateCommands(packet.acceptance.L1 ?? [], { cwd: ctx.cwd, timeoutMs, envAllowlist });
  const { executorFiles, contractViolations } = partitionChangedFiles(ctx, await getChangedFiles(ctx.cwd, ps['base-ref']));
  const boundary = checkBoundaries(executorFiles, packet.boundaries, ctx.config.verify?.['test-file-patterns']);
  if (contractViolations.length > 0) {
    // Executor touched the contract layer itself (spec/packets/config) — fail.
    boundary.violations.push(...contractViolations);
    boundary.pass = false;
  }
  const pass = gate.pass && boundary.pass;
  ps.evidence.l1 = { pass, commands: gate.results, boundary, at: nowIso() };
  ps.status = pass ? 'l1-passed' : 'l1-failed';
  ps['updated-at'] = nowIso();
  if (pass) touchProgress(ctx);
  save(ctx);
  return { pass, gate, boundary };
}

/**
 * Shared verdict-loop for LLM judge-style roles (review, judge): transport
 * retries + channel fallback, final-JSON verdict parsing. A provider error
 * inside the event stream (pi exits 0 even on 401/403) outranks parsing.
 * `ps` (packet state) is optional — when present, retries are recorded in its
 * retry-history; otherwise metrics carry the audit trail (milestone-level judge).
 */
async function agentVerdictStep(ctx, { role, binding, prompt, packetId = null, attemptNo = null, ps = null }) {
  let channel = binding.channel;
  let currentBinding = binding;
  const visitedChannels = new Set();
  const maxTransport = ctx.config.retry?.['max-transport-retries'] ?? 5;
  let lastCode = 'TRANSPORT';

  for (let t = 0; t <= maxTransport; t++) {
    if (budgetGate(ctx) === 'stop') return { ok: false, stopped: true };
    // Fail loud instead of spawning keyless into pi's auth.json trap (P1-12②).
    const missingKey = missingKeyForRealDriver(ctx, role, channel);
    if (missingKey) {
      if (ps) {
        recordRetry(ctx, ps, { scope: 'transport', 'error-code': 'AUTH', policy: `fail-loud: channel ${channel} requires env ${missingKey} (not set)` });
        save(ctx);
      }
      ctx.log(`${role}: fail-loud — channel ${channel} requires env ${missingKey} (not set)`);
      return { ok: false, errorCode: 'AUTH' };
    }
    visitedChannels.add(channel);
    const result = await ctx.drivers[role][role]({
      taskDir: ctx.taskDir,
      cwd: ctx.cwd,
      prompt,
      model: currentBinding.model,
      binding: currentBinding,
      channel,
      apiKey: channelApiKey(ctx, channel),
      apiKeys: apiKeysFor(ctx, channel),
      watchdogOpts: watchdogOpts(ctx),
    });

    const streamCode = result.errorMessage ? classifyError({ exitCode: 1, stderr: result.errorMessage }) : null;
    const verdict = !streamCode && result.exitCode === 0 && !result.timedOut
      ? parseReviewVerdict(result.text || result.stdout)
      : { ok: false };
    const code = verdict.ok ? null : streamCode ?? classifyError(result) ?? 'EMPTY_RESPONSE';
    lastCode = code ?? lastCode;
    recordSpawnMetrics(ctx, { role, packetId, model: currentBinding.model, harness: result.harness, channel, prompt, result, errorCode: code, attempt: attemptNo });

    if (verdict.ok) return { ok: true, verdict: verdict.verdict };

    ctx.log(`${role}${packetId ? ` ${packetId}` : ''} transport ${t}: failed/unparseable (${code})`);

    if (shouldFallback(code) || authFallbackAllowed(ctx.config, channel, code)) {
      const next = nextChannel(ctx.config, channel);
      const mapped = next ? modelForChannel(ctx.config, next, role) : null;
      if (next && !visitedChannels.has(next) && mapped) {
        if (ps) recordRetry(ctx, ps, { scope: 'fallback', 'error-code': code, policy: `channel ${channel} -> ${next}` });
        channel = next;
        currentBinding = { ...binding, model: mapped.model, harness: mapped.harness ?? binding.harness };
        t -= 1;
        if (ps) save(ctx);
        continue;
      }
      if (next && !mapped) {
        ctx.log(`${role} fallback chain ends: no model mapping for ${role} on ${next} (P1-12①)`);
      }
    }
    if (!isRetryable(code)) {
      if (ps) {
        recordRetry(ctx, ps, { scope: 'transport', 'error-code': code, policy: 'no-retry' });
        save(ctx);
      }
      return { ok: false, errorCode: code };
    }
    const delay = computeBackoff(t, {
      baseDelayMs: ctx.config.retry?.['base-delay-ms'],
      maxDelayMs: ctx.config.retry?.['max-delay-ms'],
      jitter: ctx.config.retry?.jitter,
      retryAfterMs: extractRetryAfterMs(`${result.stderr}\n${result.stdout}`),
    });
    if (ps) {
      recordRetry(ctx, ps, { scope: 'transport', 'error-code': code, policy: 'backoff', 'planned-delay-ms': delay });
      save(ctx);
    }
    await ctx.sleep(delay);
  }
  return { ok: false, errorCode: lastCode };
}

/**
 * Review step (§4.3): only runs after L1 is green (§2 rule 4). The reviewer
 * sees packet + diff + L1 results — never the executor transcript.
 */
async function reviewStep(ctx, packet, { defectInjection = null } = {}) {
  const id = packet['packet-id'];
  const ps = ctx.state.packets[id];

  const { executorFiles } = partitionChangedFiles(ctx, await getChangedFiles(ctx.cwd, ps['base-ref']));
  const diffSummary = executorFiles
    ? `changed files (git diff ${ps['base-ref'] ?? 'HEAD'}, ledger excluded):\n${executorFiles.map((f) => `- ${f}`).join('\n')}`
    : `changed files (executor-reported):\n${(ps.evidence.executor?.['changed-files'] ?? []).map((f) => `- ${f}`).join('\n')}`;
  // §4.3 promises the reviewer the real git diff, not a file list (#11).
  const diffText = await getDiffText(ctx.cwd, ps['base-ref'], { exclude: ledgerExcludes(ctx) });
  const prompt = buildReviewPrompt({ packet, l1Evidence: ps.evidence.l1, diffSummary, diffText, defectInjection });

  const r = await agentVerdictStep(ctx, {
    role: 'review',
    binding: resolveBinding(ctx.config, 'review'),
    prompt,
    packetId: id,
    attemptNo: ps.attempts,
    ps,
  });
  if (r.stopped) return { pass: false, stopped: true };
  if (!r.ok) return { pass: false, errorCode: r.errorCode };

  ps.evidence.review = { ...r.verdict, at: nowIso() };
  ps['updated-at'] = nowIso();
  if (r.verdict.verdict === 'pass') touchProgress(ctx);
  save(ctx);
  return { pass: r.verdict.verdict === 'pass', verdict: r.verdict };
}

/**
 * Judge step (§7 matrix 里程碑终验): after the L2/L3 gate passes and before
 * the milestone closes, a FRESH session re-verifies the whole milestone
 * against the spec's requirement matrix. Fail -> escalate to inbox.
 */
async function judgeStep(ctx, milestone) {
  const ms = ctx.state.milestones[milestone.id];
  const packets = ctx.packetList.filter((p) => milestoneOfPacket(p['packet-id']) === milestone.id);
  const prompt = buildJudgePrompt({ spec: ctx.spec, milestone, packets, state: ctx.state, gate: ms.gate });
  const r = await agentVerdictStep(ctx, { role: 'judge', binding: resolveBinding(ctx.config, 'judge'), prompt });
  if (r.stopped) return { pass: false, stopped: true };
  ms.gate ??= { pass: true, levels: {}, at: nowIso() };
  if (!r.ok) {
    ms.gate.judge = { verdict: 'error', error: r.errorCode, at: nowIso() };
    save(ctx);
    return { pass: false, errorCode: r.errorCode };
  }
  ms.gate.judge = { ...r.verdict, at: nowIso() };
  save(ctx);
  return { pass: r.verdict.verdict === 'pass', verdict: r.verdict };
}

/** Milestone gate (§4.4): union of the milestone's packets' L2/L3 commands. */
async function milestoneGateStep(ctx, milestone) {
  const packets = ctx.packetList.filter((p) => milestoneOfPacket(p['packet-id']) === milestone.id);
  const timeoutMs = ctx.config.verify?.['command-timeout-ms'];
  const envAllowlist = ctx.config.verify?.['env-allowlist'] ?? [];
  const levels = {};
  let pass = true;
  for (const level of ['L2', 'L3']) {
    const commands = packets.flatMap((p) => (p.acceptance?.[level] ?? []).map((c) => ({ ...c, _packet: p['packet-id'] })));
    if (commands.length === 0) {
      levels[level] = { pass: true, results: [], skipped: true };
      continue;
    }
    const gate = await runGateCommands(commands, { cwd: ctx.cwd, timeoutMs, envAllowlist });
    levels[level] = gate;
    if (!gate.pass) pass = false;
  }
  ctx.state.milestones[milestone.id].gate = { pass, levels, at: nowIso() };
  save(ctx);
  return { pass, levels };
}

/** Plan step: issue/confirm the packet sequence for a milestone (§4.1). */
/**
 * Plan step (§4.1): issue/confirm the packet sequence for a milestone through
 * the resident kimi code session. Session discipline:
 * - first call creates a session; its id is captured from the stream and kept
 *   in state.json (plan-session); later calls resume it with `kimi -r <id>`;
 * - session death (dead resume, hang killed by the watchdog) triggers the
 *   cold-restart protocol: a FRESH session booted from the contract layer
 *   (spec + state + retro tail) — the contract layer is the only memory;
 * - provider failure follows the configured channel fallback chain;
 * - if every channel fails, the task escalates to the inbox (Plan is the
 *   direction-setter; running on without it is pointless).
 */
async function planStep(ctx, milestone) {
  const packets = ctx.packetList.filter((p) => milestoneOfPacket(p['packet-id']) === milestone.id);
  if (packets.length === 0) throw new Error(`milestone ${milestone.id} has no packets`);
  const binding = resolveBinding(ctx.config, 'plan');

  const retroTail = () => {
    try {
      return readFileSync(join(ctx.taskDir, 'retro.md'), 'utf8').split('\n').slice(-40).join('\n');
    } catch {
      return null;
    }
  };

  const attempt = async ({ sessionId, coldStart, channel, currentBinding }) => {
    const prompt = buildPlanPrompt({
      spec: ctx.spec, state: ctx.state, milestone, packets, coldStart, retroTail: coldStart ? retroTail() : null,
    });
    const result = await ctx.drivers.plan.plan({
      milestone,
      packets,
      prompt,
      sessionId,
      model: currentBinding.model,
      binding: currentBinding,
      channel,
      apiKey: channelApiKey(ctx, channel),
      apiKeys: apiKeysFor(ctx, channel),
      cwd: ctx.cwd,
      taskDir: ctx.taskDir,
      watchdogOpts: watchdogOpts(ctx),
    });
    recordSpawnMetrics(ctx, { role: 'plan', packetId: null, model: currentBinding.model, harness: result.harness, channel, prompt, result });
    const code = result.errorMessage
      ? classifyError({ exitCode: 1, stderr: result.errorMessage })
      : result.exitCode === 0 && !result.timedOut && !result.spawnError
        ? null
        : classifyError(result);
    return { result, code };
  };

  let channel = binding.channel;
  let currentBinding = binding;
  // 铁律 7: a resident session is only resumed when it was born on the SAME
  // model+channel as the current binding; anything else cold-starts (#15).
  const stored = ctx.state['plan-session'];
  let sessionId = stored
    && stored.channel === channel
    && stored.model === currentBinding.model
    ? stored['session-id']
    : null;
  if (stored && !sessionId) {
    ctx.log(`stored plan session (${stored.channel}/${stored.model}) mismatches current binding (${channel}/${currentBinding.model}) — cold start`);
  }
  const visited = new Set([channel]);
  const maxTransport = ctx.config.retry?.['max-transport-retries'] ?? 5;

  let result;
  let code;
  let transportRetries = 0;
  for (;;) {
    if (budgetGate(ctx) === 'stop') return { ok: false, stopped: true, packets };
    // Fail loud instead of spawning keyless (P1-12②).
    const missingKey = missingKeyForRealDriver(ctx, 'plan', channel);
    if (missingKey) {
      ctx.log(`plan: fail-loud — channel ${channel} requires env ${missingKey} (not set)`);
      return { ok: false, errorCode: 'AUTH', packets };
    }
    ({ result, code } = await attempt({ sessionId, coldStart: !sessionId, channel, currentBinding }));
    if (!code) break;

    // Session death -> cold-restart protocol (§4.1): fresh session, contract-only memory.
    if (sessionId) {
      ctx.log(`plan session ${sessionId} is dead (${code}) — cold restart with a fresh session`);
      sessionId = null;
      continue;
    }
    // Channel fallback (降级): quota/rate-limit, plus AUTH on subscription
    // channels with a keyed fallback waiting (P0-7).
    if (shouldFallback(code) || authFallbackAllowed(ctx.config, channel, code)) {
      const next = nextChannel(ctx.config, channel);
      const mapped = next ? modelForChannel(ctx.config, next, 'plan') : null;
      if (next && !visited.has(next) && mapped) {
        visited.add(next);
        ctx.log(`plan fallback: ${channel} -> ${next} (${code})`);
        channel = next;
        currentBinding = { ...binding, model: mapped.model, harness: mapped.harness ?? binding.harness };
        sessionId = null; // a new channel starts a new session
        continue;
      }
      if (next && !mapped) {
        ctx.log(`plan fallback chain ends: no model mapping for plan on ${next} (P1-12①)`);
      }
    }
    // Transient failures back off and retry instead of going straight to inbox.
    if (isRetryable(code) && transportRetries < maxTransport) {
      const delay = computeBackoff(transportRetries, {
        baseDelayMs: ctx.config.retry?.['base-delay-ms'],
        maxDelayMs: ctx.config.retry?.['max-delay-ms'],
        jitter: ctx.config.retry?.jitter,
        retryAfterMs: extractRetryAfterMs(`${result.stderr}\n${result.stdout}`),
      });
      transportRetries += 1;
      ctx.log(`plan transport retry ${transportRetries}/${maxTransport} after ${code} in ${delay}ms`);
      await ctx.sleep(delay);
      continue;
    }
    break;
  }

  if (code) {
    ctx.log(`plan step failed on every channel (last: ${code})`);
    return { ok: false, errorCode: code, packets };
  }

  // Capture/refresh the resident session handle (model+channel pinned, 铁律 7).
  if (result.sessionId) {
    const prev = ctx.state['plan-session'];
    ctx.state['plan-session'] = {
      'session-id': result.sessionId,
      channel,
      model: currentBinding.model,
      'created-at': prev?.['session-id'] === result.sessionId ? prev['created-at'] : nowIso(),
      'updated-at': nowIso(),
      resumes: (prev?.['session-id'] === result.sessionId ? prev.resumes ?? 0 : 0) + (sessionId ? 1 : 0),
    };
    save(ctx);
  }
  return { ok: true, packets, result };
}

// ---------------------------------------------------------------------------
// Failure / escalation
// ---------------------------------------------------------------------------

function failAttempt(ctx, packet, errorCode, detail = {}) {
  const id = packet['packet-id'];
  const ps = ctx.state.packets[id];
  ps['failed-attempts'] += 1;
  recordRetry(ctx, ps, { scope: 'attempt', 'error-code': errorCode, policy: 'requeue' });

  const signature = failureSignature({
    errorCodes: [errorCode],
    failedCommands: (detail.failedCommands ?? []),
    changedFiles: detail.changedFiles ?? ps.evidence.executor?.['changed-files'] ?? [],
  });
  const spin = detectSpin(ps['last-failure-signature'], signature, ps['failed-attempts']);
  ps['last-failure-signature'] = signature;
  ps['updated-at'] = nowIso();
  save(ctx);
  if (spin) {
    ctx.log(`spin detected on ${id}: consecutive identical failure signatures — escalating (§6)`);
    return 'spin';
  }
  return 'failed';
}

function appendInbox(ctx, entry) {
  const file = join(ctx.taskDir, 'inbox.md');
  appendFileSync(file, `\n## ${entry.at} — ${entry['packet-id'] ?? 'task'}\n\n${entry.reason}\n`, 'utf8');
}

function appendRetro(ctx, line) {
  appendFileSync(join(ctx.taskDir, 'retro.md'), `${line}\n`, 'utf8');
}

/**
 * Escalation (§4.2/§6): retries exhausted -> return-to-planner revises the
 * packet (one more full round); still failing -> inbox a human and stop.
 */
async function escalatePacket(ctx, packet) {
  const id = packet['packet-id'];
  const ps = ctx.state.packets[id];
  const onFail = packet.escalation['on-fail'];

  if (onFail === 'return-to-planner' && ps['plan-revisions'] < 1) {
    const milestone = ctx.spec.milestones.find((m) => m.id === ps.milestone);
    const plan = await planStep(ctx, milestone); // Plan Loop revises the packet (§4.1)
    if (!plan.ok) {
      ctx.log(`planner unavailable (${plan.errorCode}) — skipping revision, straight to inbox`);
    } else {
      // P0-2: the planner may have rewritten contract files — reload them, or
      // the next round would run on the stale in-memory packet. The planner
      // may also have ADDED packets; sync state entries for them.
      const fresh = loadPackets(ctx.taskDir, ctx.spec);
      ctx.packetList = fresh;
      ctx.packets = Object.fromEntries(fresh.map((p) => [p['packet-id'], p]));
      ensurePacketStates(ctx.state, fresh);
      ps['plan-revisions'] += 1;
      ps['failed-attempts'] = 0;
      ps['last-failure-signature'] = null;
      ps.status = 'pending';
      ps['updated-at'] = nowIso();
      const lastCode = ps['retry-history'].at(-1)?.['error-code'] ?? 'REVIEW_RETURNED';
      recordRetry(ctx, ps, { scope: 'attempt', 'error-code': lastCode, policy: 'return-to-planner' });
      appendRetro(ctx, `- ${nowIso()} packet ${id}: escalated to planner, revision ${ps['plan-revisions']}`);
      save(ctx);
      return 'revised';
    }
  }

  ps.status = 'inbox';
  ps['updated-at'] = nowIso();
  const entry = {
    at: nowIso(),
    'packet-id': id,
    reason: `packet ${id} exhausted retries (${packet.escalation['max-retries']} per round, plan revisions: ${ps['plan-revisions']}). Human needed.`,
  };
  ctx.state.inbox.push(entry);
  ctx.state.status = 'escalated';
  appendInbox(ctx, entry);
  appendRetro(ctx, `- ${entry.at} packet ${id}: INBOX — ${entry.reason}`);
  save(ctx);
  return 'inbox';
}

async function completeUnit(ctx, packet, note = null) {
  const id = packet['packet-id'];
  const ps = ctx.state.packets[id];
  ps.status = 'completed';
  ps['updated-at'] = nowIso();
  touchProgress(ctx);
  save(ctx); // state.json says "completed" before the commit captures it
  // Unit-granularity commit, owned by the orchestrator (§4.2), SCOPED to the
  // unit's changes since the recorded base ref; volatile ledger files (live
  // log, metrics, pid) excluded.
  ps.commit = await commitUnitChanges(ctx.cwd, packet, { excludePaths: commitExcludes(ctx), baseRef: ps['base-ref'] });
  const commitNote = ps.commit.sha
    ? `commit ${ps.commit.sha.slice(0, 8)}`
    : `commit skipped (${ps.commit.skipped ?? ps.commit.error ?? 'unknown'})`;
  appendRetro(ctx, `- ${nowIso()} packet ${id}: completed (attempts=${ps.attempts}, failed=${ps['failed-attempts']}, ${commitNote}${note ? `, ${note}` : ''})`);
  save(ctx);
}

/**
 * Defect-injection spot check (§4.3): deterministic, ALWAYS ON (user decision
 * 2026-08-20 — no switch), runs BETWEEN L1 and Review; the outcome feeds the
 * reviewer. Returns the outcome object when sampled, null otherwise.
 * Case handling lives here (P0-2): only `fake-tests` and `restore-failed`
 * stop the unit; `uninjectable` / `baseline-red` / missing-config inbox a
 * human and let the unit continue.
 */
async function maybeDefectInjection(ctx, packet) {
  const di = ctx.config['defect-injection'] ?? {};
  const ps = ctx.state.packets[packet['packet-id']];
  const completedCount = Object.values(ctx.state.packets).filter((p) => p.status === 'completed').length;
  // Test-file ratio from the AUTHORITATIVE diff (l1 boundary), shared patterns
  // with verify — not the executor's self-report (P2).
  const patterns = (ctx.config.verify?.['test-file-patterns'] ?? []).map((p) => new RegExp(p));
  const changed = ps.evidence.l1?.boundary?.changedFiles ?? ps.evidence.executor?.['changed-files'] ?? [];
  const testRatio = changed.length === 0 ? 0 : changed.filter((f) => patterns.some((re) => re.test(f))).length / changed.length;
  if (!shouldSample(completedCount, { ...di, testFileRatio: testRatio })) return null;

  const inboxContinue = (reason, evidence) => {
    ps.evidence['defect-injection'] = { ...evidence, at: nowIso() };
    const entry = { at: nowIso(), 'packet-id': packet['packet-id'], reason };
    ctx.state.inbox.push(entry);
    appendInbox(ctx, entry);
    save(ctx);
    return null;
  };

  if (!di.patch || !di['test-cmd']) {
    return inboxContinue(
      `defect injection sampled for ${packet['packet-id']} but defect-injection.patch/test-cmd are not configured in flywheel.config.json — a human should add them for this task.`,
      { skipped: 'missing-config', detail: 'sampled but defect-injection.patch/test-cmd not configured for this task' },
    );
  }

  const outcome = await runDefectInjection({
    patchFile: di.patch,
    testCmd: di['test-cmd'],
    cwd: ctx.cwd,
    timeoutMs: ctx.config.verify?.['command-timeout-ms'],
    envAllowlist: ctx.config.verify?.['env-allowlist'] ?? [],
    forbidPaths: ledgerExcludes(ctx),
  });
  ps.evidence['defect-injection'] = { ...outcome, at: nowIso() };
  save(ctx);

  if (outcome.outcome === 'pass') return outcome;
  if (outcome.outcome === 'fake-tests' || outcome.outcome === 'restore-failed') return outcome; // caller stops (DC3)
  // uninjectable / baseline-red: inbox a human, never accuse fake tests (P0-2).
  return inboxContinue(
    `defect injection for ${packet['packet-id']}: ${outcome.outcome} — ${outcome.detail ?? ''} (human should refresh the patch / fix the suite)`,
    outcome,
  );
}

/** Valid done-report still on disk (pause/resume and interrupt recovery). */
function doneReportValid(ctx, packet) {
  const reportAbs = join(ctx.taskDir, 'reports', `${packet['packet-id']}.done.json`);
  if (!existsSync(reportAbs)) return false;
  return parseDoneReport(readFileSync(reportAbs, 'utf8'), packet['done-report-format']).ok;
}

// ---------------------------------------------------------------------------
// Packet processing (the attempts loop)
// ---------------------------------------------------------------------------

async function processPacket(ctx, packet0, { pauseBefore } = {}) {
  let packet = packet0;
  const id = packet['packet-id'];
  const ps = ctx.state.packets[id];

  // P2: a packet already escalated to the inbox must not re-spawn or re-append
  // inbox entries on re-run — the human re-arms by editing state.json (§12).
  // The credentials flow is the exception: there the human's fix IS the
  // approval file, so its appearance after an approval timeout re-arms the
  // unit.
  if (ps.status === 'inbox') {
    const approvalFile = join(ctx.taskDir, 'approvals', `${id}.json`);
    if (packet['needs-live-credentials'] === true && !ps.evidence.credentials && existsSync(approvalFile)) {
      ctx.log(`${id}: approval file appeared after inbox escalation — re-arming`);
      ps.status = 'pending';
      ps['updated-at'] = nowIso();
      save(ctx);
    } else {
      ctx.log(`${id}: already escalated to inbox — waiting for human (edit state.json to re-arm)`);
      // The task is still escalated; a re-run must not flip it to "running".
      ctx.state.status = 'escalated';
      save(ctx);
      return 'stop';
    }
  }

  if (ps.status === 'interrupted') {
    ctx.log(`${id}: resuming interrupted unit (artifacts preserved, no penalty)`);
    ps.status = 'pending';
    save(ctx);
  }

  // Resume after --pause-before review (or a stop between L1 and review):
  // L1 is green and the done-report is intact — continue AT REVIEW without
  // re-executing or wiping the report (#6).
  let stage = 'execute';
  if (ps.status === 'l1-passed' && ps.evidence.l1?.pass === true && doneReportValid(ctx, packet)) {
    ctx.log(`${id}: resuming at review (L1 green + done-report intact)`);
    stage = 'review';
  }

  let pendingInjection = null;
  for (;;) {
    const maxRetries = packet.escalation['max-retries']; // re-read every round: the planner may revise it
    if (ps['failed-attempts'] >= maxRetries) {
      const escalation = await escalatePacket(ctx, packet);
      if (escalation === 'inbox') return 'stop';
      packet = ctx.packets[id] ?? packet; // P0-2: run the revised contract
      stage = 'execute';
    }

    if (stage === 'execute') {
      const exec = await executeStep(ctx, packet);
      if (exec.stopped) return 'stop';
      if (!exec.success) {
        const outcome = failAttempt(ctx, packet, exec.errorCode ?? 'TRANSPORT');
        if (outcome === 'spin') {
          const esc = await escalatePacket(ctx, packet);
          if (esc === 'inbox') return 'stop';
          packet = ctx.packets[id] ?? packet;
        }
        continue;
      }

      const l1 = await l1Step(ctx, packet);
      if (!l1.pass) {
        const failedCommands = l1.gate.results.filter((r) => !r.pass).map((r) => r.cmd);
        const boundaryViolations = l1.boundary.violations.map((v) => `${v.type}:${v.path ?? v.count}`);
        const outcome = failAttempt(ctx, packet, 'L1_FAILED', { failedCommands: [...failedCommands, ...boundaryViolations] });
        if (outcome === 'spin') {
          const esc = await escalatePacket(ctx, packet);
          if (esc === 'inbox') return 'stop';
          packet = ctx.packets[id] ?? packet;
        }
        continue;
      }

      // Defect-injection spot check BETWEEN L1 and Review (§4.3); the outcome
      // feeds the reviewer. fake-tests / restore-failed -> inbox + stop
      // (v0.14 DC3); other non-pass cases already inboxed-and-continued (P0-2).
      pendingInjection = await maybeDefectInjection(ctx, packet);
      if (pendingInjection && (pendingInjection.outcome === 'fake-tests' || pendingInjection.outcome === 'restore-failed')) {
        ps.status = 'inbox';
        ps['updated-at'] = nowIso();
        const entry = {
          at: nowIso(),
          'packet-id': id,
          reason: pendingInjection.outcome === 'fake-tests'
            ? `defect injection FAILED for ${id}: injected defect did not turn the tests red (fake/weak tests proven). Human needed.`
            : `defect injection for ${id}: restore via git apply -R FAILED — the workspace may still hold the injected defect. Human needed.`,
        };
        ctx.state.inbox.push(entry);
        ctx.state.status = 'escalated';
        appendInbox(ctx, entry);
        appendRetro(ctx, `- ${entry.at} packet ${id}: INBOX — defect injection ${pendingInjection.outcome}`);
        save(ctx);
        return 'stop';
      }

      if (pauseBefore === 'review') {
        ctx.state.status = 'paused';
        save(ctx);
        ctx.log(`${id}: paused before review (--pause-before review)`);
        return 'pause';
      }
      stage = 'review';
    }

    const review = await reviewStep(ctx, packet, { defectInjection: pendingInjection });
    pendingInjection = null;
    if (review.stopped) return 'stop';
    if (review.pass) {
      await completeUnit(ctx, packet);
      return 'ok';
    }

    const verdict = review.verdict;
    if (verdict?.verdict === 'verification-gap') {
      // v0.14 DC2: a verification gap goes BACK TO PLAN to strengthen
      // acceptance (consuming the revision budget) and is then re-verified —
      // the unit is NEVER completed on a gap.
      ctx.log(`${id}: verification-gap — returning to planner to strengthen acceptance`);
      const esc = await escalatePacket(ctx, packet);
      if (esc === 'inbox') return 'stop';
      packet = ctx.packets[id] ?? packet; // revised contract (P0-2)
      const recheck = await l1Step(ctx, packet); // re-verify with strengthened acceptance
      if (!recheck.pass) {
        failAttempt(ctx, packet, 'L1_FAILED', { failedCommands: recheck.gate.results.filter((r) => !r.pass).map((r) => r.cmd) });
        stage = 'execute';
        continue;
      }
      stage = 'review'; // re-review against the strengthened acceptance
      continue;
    }

    ps.status = 'review-failed';
    stage = 'execute';
    const outcome = failAttempt(ctx, packet, review.errorCode ?? 'REVIEW_RETURNED');
    if (outcome === 'spin') {
      const esc = await escalatePacket(ctx, packet);
      if (esc === 'inbox') return 'stop';
      packet = ctx.packets[id] ?? packet;
    }
  }
}

// ---------------------------------------------------------------------------
// Entry points
// ---------------------------------------------------------------------------

function nextActionablePacket(ctx, milestoneId) {
  return ctx.packetList.find((p) => {
    const id = p['packet-id'];
    if (milestoneOfPacket(id) !== milestoneId) return false;
    return ctx.state.packets[id].status !== 'completed';
  });
}

function currentMilestone(ctx) {
  return ctx.spec.milestones.find((m) => ctx.state.milestones[m.id].status !== 'closed') ?? null;
}

function writePid(ctx) {
  mkdirSync(ctx.taskDir, { recursive: true });
  writeFileSync(pidFile(ctx.taskDir), JSON.stringify({ pid: process.pid, startedAt: nowIso() }), 'utf8');
}

function clearPid(ctx) {
  try {
    rmSync(pidFile(ctx.taskDir), { force: true });
  } catch { /* best effort */ }
}

/**
 * Full automatic run: milestone -> packet -> attempts (execute -> L1 -> review)
 * -> milestone gate. Returns a summary; state.json carries the truth.
 */
export async function runTask(opts) {
  const ctx = createContext(opts);
  const { pauseBefore = null } = opts;
  assertNoRunningOrchestrator(ctx.taskDir); // P1: single instance per task
  writePid(ctx);
  // Graceful stop (§9 kill switch): kill in-flight child process groups and
  // await quiescence before exiting, so no orphan writes to the workspace.
  const onSigterm = () => {
    ctx.log('SIGTERM received — killing in-flight children, then exiting');
    killActiveChildren().finally(() => process.exit(143));
  };
  process.once('SIGTERM', onSigterm);
  try {
    // Crash recovery: never silently reset in-flight units (§5.2).
    const interrupted = markInterruptedUnits(ctx.state);
    if (interrupted.length > 0) {
      ctx.log(`recovered interrupted units: ${interrupted.join(', ')}`);
    }
    ctx.state.status = 'running'; // a paused/stopped task flips back to running on resume (#6)
    save(ctx);

    for (const milestone of ctx.spec.milestones) {
      const ms = ctx.state.milestones[milestone.id];
      if (ms.status === 'closed') continue;

      ctx.state['current-milestone'] = milestone.id;
      ms.status = 'in-progress';
      const budgetStart = { tokens: ctx.state.budget.totals.tokens, cost: ctx.state.budget.totals['cost-usd'] };
      save(ctx);

      const plan = await planStep(ctx, milestone);
      if (plan.stopped) return summary(ctx);
      if (!plan.ok) {
        ctx.state.status = 'escalated';
        const entry = { at: nowIso(), 'packet-id': null, milestone: milestone.id, reason: `plan step failed on every channel (last: ${plan.errorCode}). Human needed.` };
        ctx.state.inbox.push(entry);
        appendInbox(ctx, entry);
        save(ctx);
        return summary(ctx);
      }
      // P1: the planner may have revised contract files during this routine
      // step — reload them, exactly like the escalation path (P0-2 symmetry).
      // The plan output is otherwise only observed by the next restart.
      const freshPackets = loadPackets(ctx.taskDir, ctx.spec);
      ctx.packetList = freshPackets;
      ctx.packets = Object.fromEntries(freshPackets.map((p) => [p['packet-id'], p]));
      ensurePacketStates(ctx.state, freshPackets);
      save(ctx);

      for (;;) {
        const packet = nextActionablePacket(ctx, milestone.id);
        if (!packet) break;
        const outcome = await processPacket(ctx, packet, { pauseBefore });
        if (outcome === 'stop' || outcome === 'pause') return summary(ctx);
      }

      const gate = await milestoneGateStep(ctx, milestone);
      if (!gate.pass) {
        ctx.state.status = 'escalated';
        const entry = { at: nowIso(), 'packet-id': null, milestone: milestone.id, reason: `milestone ${milestone.id} gate (L2/L3) failed` };
        ctx.state.inbox.push(entry);
        appendInbox(ctx, entry);
        save(ctx);
        return summary(ctx);
      }

      // Judge (§7 里程碑终验): fresh-context final verification before close.
      const judge = await judgeStep(ctx, milestone);
      if (judge.stopped) return summary(ctx);
      if (!judge.pass) {
        ctx.state.status = 'escalated';
        const entry = {
          at: nowIso(),
          'packet-id': null,
          milestone: milestone.id,
          reason: `milestone ${milestone.id} judge verdict: ${judge.verdict?.verdict ?? judge.errorCode} — ${(judge.verdict?.reasons ?? []).join('; ') || 'see metrics'}`,
        };
        ctx.state.inbox.push(entry);
        appendInbox(ctx, entry);
        save(ctx);
        return summary(ctx);
      }

      ms.status = 'closed';
      ms['closed-at'] = nowIso();
      touchProgress(ctx);
      appendRetro(ctx, `- ${nowIso()} milestone ${milestone.id}: closed (gate pass, judge ${judge.verdict?.score ?? 'n/a'})`);
      appendRetro(ctx, milestoneSummaryLine(ctx, milestone, budgetStart));
      save(ctx);
    }

    ctx.state.status = 'completed';
    ctx.state['current-milestone'] = null;
    touchProgress(ctx);
    save(ctx);
    return summary(ctx);
  } finally {
    process.removeListener('SIGTERM', onSigterm);
    clearPid(ctx);
  }
}

/** retro.md milestone summary (§5.3 fields): checkable facts only. */
function milestoneSummaryLine(ctx, milestone, budgetStart) {
  const packets = ctx.packetList.filter((p) => milestoneOfPacket(p['packet-id']) === milestone.id);
  const states = packets.map((p) => ctx.state.packets[p['packet-id']]).filter(Boolean);
  const attempts = states.reduce((n, p) => n + p.attempts, 0);
  const reviewFailed = states.reduce((n, p) => n + p['retry-history'].filter((r) => r['error-code'] === 'REVIEW_RETURNED').length, 0);
  const reopens = states.reduce((n, p) => n + p['plan-revisions'], 0);
  const scopeChanges = states.reduce((n, p) => n + p['retry-history'].filter((r) => r.policy === 'return-to-planner').length, 0);
  const tokens = ctx.state.budget.totals.tokens - budgetStart.tokens;
  const cost = Math.round((ctx.state.budget.totals['cost-usd'] - budgetStart.cost) * 1e8) / 1e8;
  // P2: interventions counted for THIS milestone only (entry.milestone, or the
  // entry's packet id resolving to this milestone) — never the whole-task
  // inbox, which grows with every later escalation.
  const humanInterventions = ctx.state.inbox.filter((e) => {
    if (e.milestone === milestone.id) return true;
    if (e['packet-id']) {
      try { return milestoneOfPacket(e['packet-id']) === milestone.id; } catch { return false; }
    }
    return false;
  }).length;
  return [
    `  summary: rounds=${attempts}, defects(review-failed)=${reviewFailed}, reopens=${reopens}, ` +
    `scope-changes=${scopeChanges} (return-to-planner revisions), budget=+${tokens} tokens / $${cost}, ` +
    `human-interventions=${humanInterventions}`,
    '  improvements (<=3, placeholder — filled by Plan/human): -',
  ].join('\n');
}

/**
 * Manual single-step mode (§12): --only performs exactly ONE state transition.
 * The step's outcome lands in the summary (`step`), and a failed step exits
 * non-zero via the CLI (#9).
 */
export async function runSingleStep(opts) {
  const ctx = createContext(opts);
  const { only, packetId = null } = opts;
  assertNoRunningOrchestrator(ctx.taskDir); // P1: single instance per task
  writePid(ctx);
  const onSigterm = () => {
    ctx.log('SIGTERM received — killing in-flight children, then exiting');
    killActiveChildren().finally(() => process.exit(143));
  };
  process.once('SIGTERM', onSigterm);
  try {
    const interrupted = markInterruptedUnits(ctx.state);
    if (interrupted.length > 0) ctx.log(`recovered interrupted units: ${interrupted.join(', ')}`);
    save(ctx);

    const pickPacket = () => {
      if (packetId) {
        const p = ctx.packets[packetId];
        if (!p) throw new Error(`unknown packet/unit: ${packetId}`);
        return p;
      }
      const m = currentMilestone(ctx);
      if (!m) throw new Error('no open milestone');
      const p = nextActionablePacket(ctx, m.id);
      if (!p) throw new Error(`no actionable packet in milestone ${m.id}`);
      return p;
    };

    let step = null;
    switch (only) {
      case 'plan': {
        const m = currentMilestone(ctx);
        if (!m) throw new Error('no open milestone');
        const r = await planStep(ctx, m);
        step = { name: 'plan', ok: r.ok === true, detail: r.errorCode ?? null };
        break;
      }
      case 'execute': {
        const r = await executeStep(ctx, pickPacket());
        step = { name: 'execute', ok: r.success === true, detail: r.errorCode ?? null };
        break;
      }
      case 'review': {
        const packet = pickPacket();
        const ps = ctx.state.packets[packet['packet-id']];
        // Review speaks only after deterministic acceptance is green (铁律 4).
        if (!ps.evidence.l1 || ps.evidence.l1.pass !== true) {
          throw new Error(`unit ${packet['packet-id']} has no PASSING L1 evidence — run execute/verify first`);
        }
        const r = await reviewStep(ctx, packet);
        step = { name: 'review', ok: r.pass === true, verdict: r.verdict ?? null, detail: r.errorCode ?? null };
        break;
      }
      case 'verify': {
        if (packetId) {
          const r = await l1Step(ctx, pickPacket());
          step = { name: 'verify', ok: r.pass === true, detail: null };
        } else {
          let ok = true;
          for (const p of ctx.packetList) {
            if (ctx.state.packets[p['packet-id']].status !== 'completed') {
              const r = await l1Step(ctx, p);
              if (!r.pass) ok = false;
            }
          }
          const m = currentMilestone(ctx);
          if (m) {
            const g = await milestoneGateStep(ctx, m);
            if (!g.pass) ok = false;
          }
          step = { name: 'verify', ok, detail: null };
        }
        break;
      }
      default:
        throw new Error(`unknown --only step: ${only}`);
    }
    return { ...summary(ctx), step };
  } finally {
    process.removeListener('SIGTERM', onSigterm);
    clearPid(ctx);
  }
}

function summary(ctx) {
  const done = Object.values(ctx.state.packets).filter((p) => p.status === 'completed').length;
  const total = Object.keys(ctx.state.packets).length;
  return {
    slug: ctx.state.slug,
    status: ctx.state.status,
    'current-milestone': ctx.state['current-milestone'],
    'packets-completed': `${done}/${total}`,
    revision: ctx.state.revision,
    'last-progress-at': ctx.state['last-progress-at'],
    tokens: ctx.state.budget.totals.tokens,
    'cost-usd': ctx.state.budget.totals['cost-usd'],
  };
}
