#!/usr/bin/env node
"use strict";

// Paired P4 experiments run the real engine in disposable, pinned Git clones.
// Only the engine writes task state. The independent, pinned oracle is copied
// outside the candidate; no model output is promoted to an evaluation score.
const fs = require("node:fs/promises");
const { constants } = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { spawn, spawnSync } = require("node:child_process");

const VARIANTS = Object.freeze(["fresh", "executor-scoped", "role-scoped"]);
const SCENARIOS = Object.freeze(["first-pass", "l1-repair", "review-return", "long-context", "stop-resume", "configuration-change"]);
const HASH = /^[0-9a-f]{40}$/;
const MAX_FILE = 32 * 1024 * 1024;

function fail(message) { throw new Error(message); }
function object(value, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)) fail(`${label} must be an object`);
  return value;
}
function keys(value, names, label) {
  object(value, label);
  for (const key of Object.keys(value)) if (!names.includes(key)) fail(`${label}: unknown field ${key}`);
}
function integer(value, min, max, label) {
  if (!Number.isSafeInteger(value) || value < min || value > max) fail(`${label} must be an integer in ${min}..${max}`);
}
function relative(value, label) {
  if (typeof value !== "string" || !value || path.isAbsolute(value) || value.includes("\\") || value.split("/").some(p => p === ".." || p === "." || !p)) fail(`${label} must be a normalized relative path`);
  return value;
}
function text(value, label) {
  if (typeof value !== "string" || !value.trim() || value.includes("\0")) fail(`${label} must be a nonempty string`);
}
function digest(value) { return crypto.createHash("sha256").update(value).digest("hex"); }
function canonical(value) {
  if (Array.isArray(value)) return value.map(canonical);
  if (value && typeof value === "object") return Object.fromEntries(Object.keys(value).sort().map(k => [k, canonical(value[k])]));
  return value;
}

function validateManifest(manifest) {
  keys(manifest, ["version", "execution", "source", "config", "repetitions", "limits", "cases"], "manifest");
  if (manifest.version !== 1) fail("manifest version must be 1");
  if (!["live", "offline-fixture"].includes(manifest.execution)) fail("execution must be live or offline-fixture");
  keys(manifest.source, ["repository", "commit"], "source");
  text(manifest.source.repository, "source.repository");
  if (!path.isAbsolute(manifest.source.repository) || !HASH.test(manifest.source.commit)) fail("source requires an absolute local repository and an exact 40-character commit");
  relative(manifest.config, "config");
  integer(manifest.repetitions, 2, 30, "repetitions");
  keys(manifest.limits, ["maxCalls", "runTimeoutMs"], "limits");
  integer(manifest.limits.maxCalls, 1, 10000, "limits.maxCalls");
  integer(manifest.limits.runTimeoutMs, 100, 3600000, "limits.runTimeoutMs");
  if (!Array.isArray(manifest.cases) || manifest.cases.length < 1 || manifest.cases.length > 50) fail("cases must contain 1..50 cases");
  const ids = new Set();
  for (const c of manifest.cases) {
    keys(c, ["id", "scenario", "taskDir", "oracle", "steps", "minRunInputTokens"], "case");
    if (typeof c.id !== "string" || !/^[a-z0-9][a-z0-9_-]{0,63}$/.test(c.id) || ids.has(c.id)) fail("case ids must be unique safe names");
    ids.add(c.id);
    if (!SCENARIOS.includes(c.scenario)) fail(`case ${c.id}: unsupported scenario`);
    relative(c.taskDir, "taskDir");
    relative(c.oracle, "oracle");
    if (c.scenario === "long-context") integer(c.minRunInputTokens, 1, Number.MAX_SAFE_INTEGER, "minRunInputTokens");
    else if (c.minRunInputTokens !== undefined) fail("minRunInputTokens only belongs to long-context cases");
    if (!Array.isArray(c.steps) || c.steps.length < 1 || c.steps.length > 20) fail("case steps must contain 1..20 steps");
    for (const step of c.steps) {
      if (step.kind === "run") {
        keys(step, ["kind", "only", "packet", "pauseBeforeReview", "stopAfterMs"], "run step");
        if (step.only !== undefined && !["plan", "execute", "verify", "review"].includes(step.only)) fail("unsupported single step");
        if (step.packet !== undefined && !/^M\d+-T\d+$/.test(step.packet)) fail("invalid step packet");
        if (step.pauseBeforeReview !== undefined && typeof step.pauseBeforeReview !== "boolean") fail("pauseBeforeReview must be boolean");
        if (step.only && step.pauseBeforeReview) fail("single step and pauseBeforeReview are exclusive");
        if (step.stopAfterMs !== undefined) integer(step.stopAfterMs, 1, manifest.limits.runTimeoutMs - 1, "stopAfterMs");
      } else if (step.kind === "configure") {
        keys(step, ["kind", "section", "name", "model", "thinking"], "configure step");
        if (!["loops", "routes"].includes(step.section)) fail("configure section must be loops or routes");
        text(step.name, "binding name");
        if (step.model === undefined && step.thinking === undefined) fail("configure requires model or thinking");
        if (step.model !== undefined) text(step.model, "model");
        if (step.thinking !== undefined) text(step.thinking, "thinking");
      } else if (step.kind === "inject") {
        keys(step, ["kind", "script", "reason"], "injection step");
        relative(step.script, "injection script");
        text(step.reason, "injection reason");
      } else fail("unsupported case step");
    }
    if (!c.steps.some(s => s.kind === "run")) fail("each case needs an engine run");
  }
  return manifest;
}

function variantConfig(base, variant) {
  if (!VARIANTS.includes(variant)) fail("unknown experiment variant");
  const cfg = structuredClone(base);
  // Keep Plan/Judge policies equal across all three arms. These experiments
  // isolate the proposed Executor and Reviewer change; they do not add stages.
  cfg.sessions = { ...cfg.sessions, mode: "fresh", "policy-version": 1,
    roles: { plan: "fresh", execute: variant === "fresh" ? "fresh" : "scoped",
      review: variant === "role-scoped" ? "scoped" : "fresh", judge: "fresh", "judge-panel": "fresh" } };
  return cfg;
}

function git(cwd, args, trim = true) {
  const result = spawnSync("git", ["-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-C", cwd, ...args], { encoding: "utf8", timeout: 30000, maxBuffer: MAX_FILE,
    env: { ...process.env, GIT_TERMINAL_PROMPT: "0", GIT_CONFIG_NOSYSTEM: "1", GIT_CONFIG_GLOBAL: "/dev/null" } });
  if (result.error || result.status !== 0) fail(`git ${args[0]} failed: ${result.error?.message || result.stderr.trim()}`);
  return trim ? result.stdout.trim() : result.stdout;
}
async function jsonFile(file, missing = undefined) {
  try {
    const info = await fs.lstat(file);
    if (!info.isFile() || info.size > MAX_FILE) fail(`invalid evidence file: ${file}`);
    return JSON.parse(await fs.readFile(file, "utf8"));
  } catch (error) { if (error.code === "ENOENT" && missing !== undefined) return missing; throw error; }
}
async function metricRows(file, complete = true) {
  try {
    const info = await fs.lstat(file);
    if (!info.isFile() || info.size > MAX_FILE) fail("invalid metrics file");
    const raw = await fs.readFile(file, "utf8");
    const lines = raw.split("\n");
    if (!complete && !raw.endsWith("\n")) lines.pop(); // A currently appending frame is not a corrupt terminal ledger.
    return lines.filter(Boolean).map(line => JSON.parse(line));
  } catch (error) { if (error.code === "ENOENT") return []; throw error; }
}
async function saveJSON(file, value) { await fs.writeFile(file, JSON.stringify(value, null, 2) + "\n", { mode: 0o600 }); }

async function taskSnapshot(directory, prefix = "", result = {}) {
  for (const entry of (await fs.readdir(path.join(directory, prefix), { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name))) {
    const name = prefix ? `${prefix}/${entry.name}` : entry.name;
    if (entry.isDirectory()) await taskSnapshot(directory, name, result);
    else {
      const file = path.join(directory, name), info = await fs.lstat(file);
      if (!info.isFile() || info.size > MAX_FILE || Object.keys(result).length >= 10000) fail("invalid task evidence during controlled injection");
      result[name] = digest(await fs.readFile(file));
    }
  }
  return result;
}

function processRun(command, args, options) {
  return new Promise((resolve, reject) => {
    const started = performance.now();
    const child = spawn(command, args, { cwd: options.cwd, env: options.env || process.env, stdio: ["ignore", "pipe", "pipe"], detached: true });
    let stdout = "", stderr = "", stopReason = null, forcedKill = false, tickBusy = false;
    let grace, stopping = false, closed = false;
    const stop = reason => {
      if (stopping || closed) return;
      stopping = true; stopReason = reason;
      try { process.kill(-child.pid, "SIGTERM"); } catch (error) { if (error.code !== "ESRCH") child.kill("SIGTERM"); }
      // The engine must finish its watchdog/guardian cleanup before another
      // invocation. Forced termination always stops the whole experiment.
      grace = setTimeout(() => {
        forcedKill = true;
        try { process.kill(-child.pid, "SIGKILL"); } catch (error) { if (error.code !== "ESRCH") child.kill("SIGKILL"); }
      }, 30000);
    };
    const collect = field => chunk => {
      if (field === "stdout") stdout += chunk; else stderr += chunk;
      if (Buffer.byteLength(stdout) + Buffer.byteLength(stderr) > MAX_FILE) {
        stdout = stdout.slice(0, MAX_FILE / 2); stderr = stderr.slice(0, MAX_FILE / 2);
        stop("output-limit");
      }
    };
    child.stdout.setEncoding("utf8"); child.stderr.setEncoding("utf8");
    child.stdout.on("data", collect("stdout")); child.stderr.on("data", collect("stderr"));
    const timeout = setTimeout(() => stop("timeout"), options.timeoutMs);
    const planned = options.stopAfterMs ? setTimeout(() => stop("planned-stop"), options.stopAfterMs) : null;
    const interval = options.shouldStop ? setInterval(async () => {
      if (tickBusy || stopping) return;
      tickBusy = true;
      try { if (await options.shouldStop()) stop("call-limit"); }
      catch { stop("evidence-unreadable"); }
      finally { tickBusy = false; }
    }, 50) : null;
    const cancel = () => stop("caller-cancelled");
    process.once("SIGTERM", cancel); process.once("SIGINT", cancel);
    const clear = () => {
      clearTimeout(timeout); clearTimeout(planned); clearTimeout(grace); clearInterval(interval);
      process.removeListener("SIGTERM", cancel); process.removeListener("SIGINT", cancel);
    };
    child.once("error", error => { closed = true; clear(); reject(error); });
    child.once("close", (exitCode, signal) => {
      closed = true; clear();
      resolve({ exitCode, signal, stopReason, forcedKill, durationMs: Math.round(performance.now() - started), stdout, stderr });
    });
  });
}

function addMeasure(measure, value) {
  if (Number.isSafeInteger(value) && value >= 0) {
    if (!Number.isSafeInteger(measure.known + value)) fail("measured usage exceeds exact numeric aggregation range");
    measure.known += value; measure.observedCalls++;
  }
  else measure.missingCalls++;
}
function resolveMetricRows(rows) {
  const calls = [], byID = new Map(), auditIDs = new Set();
  let corrections = 0, duplicates = 0;
  for (const row of rows) {
    const id = row["invocation-id"] || row.session?.last?.id;
    if (id && auditIDs.has(id)) { duplicates++; continue; }
    if (id) auditIDs.add(id);
    if (row["record-kind"] === "reconciliation") {
      const original = byID.get(row["reconciles-invocation"]);
      if (!original) fail("reconciliation has no original metric invocation");
      original.usage = row.usage;
      original["cost-usd"] = row["cost-usd"];
      if (row.result) original.result = structuredClone(row.result);
      // Preserve the original selection action; reconciliation is an audit
      // correction, not another model call or another create/resume decision.
      if (row.session) original.session = { ...row.session, action: original.session?.action, reason: original.session?.reason };
      corrections++;
      continue;
    }
    if (row["record-kind"] && row["record-kind"] !== "invocation") fail("unsupported metric record kind");
    const copy = structuredClone(row);
    calls.push(copy);
    if (id) byID.set(id, copy);
  }
  return { calls, corrections, duplicates };
}
function summarizeTrial(state, rows, steps, quality, c) {
  const names = ["input", "output", "cacheRead", "cacheWrite", "totalTokens"];
  const usage = Object.fromEntries(names.map(name => [name, { known: 0, observedCalls: 0, missingCalls: 0 }]));
  const roles = {}, actions = {}, rebuildReasons = [];
  const resolved = resolveMetricRows(rows);
  const cost = { knownUSD: 0, observedCalls: 0, missingCalls: 0 };
  let unknownOutcomes = 0, resumes = 0, resumeFailures = 0, maxRunInput = null;
  for (const row of resolved.calls) {
    roles[row.role] = (roles[row.role] || 0) + 1;
    if (row.session) {
      const s = row.session;
      actions[s.action] = (actions[s.action] || 0) + 1;
      if (s.action === "rebuild") rebuildReasons.push({ role: s.role, scope: s.scope, reason: s.reason, generation: s.generation });
      if (s.last?.outcome === "unknown" || s.last?.dispatch === "unknown" || s.last?.["cleanup-confirmed"] !== true) unknownOutcomes++;
      if (s.last?.["requested-thread"]) {
        resumes++;
        if (s.last.outcome !== "completed") resumeFailures++;
      }
    }
    // Normalized per-run usage is trusted only as recorded by Go. Never add
    // threadTotal or convert an absent field into zero. Raw partial receipts
    // remain in the saved rows for separate inspection, not guessed totals.
    for (const name of names) addMeasure(usage[name], row.usage?.[name]);
    if (typeof row["cost-usd"] === "number" && Number.isFinite(row["cost-usd"]) && row["cost-usd"] >= 0) {
      cost.knownUSD += row["cost-usd"]; cost.observedCalls++;
    } else cost.missingCalls++;
    if (Number.isSafeInteger(row.usage?.input) && row.usage.input >= 0) maxRunInput = Math.max(maxRunInput ?? 0, row.usage.input);
  }
  const packets = Object.values(state.packets || {});
  const evidence = packets.flatMap(p => [...(p["evidence-history"] || []).map(h => h.evidence), p.evidence]).filter(Boolean);
  const reviewReasons = evidence.filter(e => e.review?.verdict === "return").flatMap(e => e.review.reasons || []);
  const reasons = reviewReasons.map(x => String(x).trim().replace(/\s+/g, " "));
  const repeatedFindings = reasons.length - new Set(reasons).size;
  const runSteps = steps.filter(s => s.kind === "run");
  const observations = {
    "first-pass": state.status === "completed" && roles.execute === 1 && roles.review === 1,
    "l1-repair": evidence.some(e => e.l1?.pass === false) && (roles.execute || 0) > 1,
    "review-return": evidence.some(e => e.review?.verdict === "return") && (roles.execute || 0) > 1,
    "long-context": maxRunInput === null ? null : maxRunInput >= (c.minRunInputTokens || Number.MAX_SAFE_INTEGER),
    "stop-resume": runSteps.some((s, i) => i < runSteps.length - 1 &&
      ((s.process.stopReason === "planned-stop" && s.process.exitCode === 143) || (s.pauseBeforeReview && s.stateStatus === "paused"))),
    "configuration-change": steps.some(s => s.kind === "configure") && new Set(rows.map(r => r.session?.binding).filter(Boolean)).size > 1,
  };
  const pending = Object.values(state.sessions?.records || {}).filter(r => ["intent", "blocked"].includes(r.last?.status) || r.last?.dispatch === "unknown" || r.last?.outcome === "unknown" || r.last?.["cleanup-confirmed"] !== true);
  const observedBindings = [...new Set(resolved.calls.map(r => JSON.stringify(canonical({ role: r.role, harness: r.harness ?? null,
    provider: r.provider ?? null, channel: r.channel ?? null, model: r.model ?? null }))))].sort().map(x => JSON.parse(x));
  return { status: state.status || "unavailable", calls: resolved.calls.length, callsByRole: roles, usage, cost, observedBindings,
    repairRounds: packets.reduce((sum, p) => sum + Math.max(0, (p.attempts || 0) - 1), 0),
    repeatedFindings: { value: repeatedFindings, method: "exact normalized repeated return reasons; not semantic equivalence" },
    elapsedMs: runSteps.reduce((sum, s) => sum + s.process.durationMs, 0), maxRunInput,
    actions, rebuildReasons, recovery: { resumes, failedOrUnknownResumes: resumeFailures, unknownOutcomes, unresolved: pending.length, reconciliations: resolved.corrections },
    duplicateMetricRows: resolved.duplicates, quality, scenario: { requested: c.scenario, observed: observations[c.scenario] } };
}

function compareTrials(trials) {
  const groups = new Map();
  for (const trial of trials) {
    const key = `${trial.case}/${trial.repetition}`;
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(trial);
  }
  return [...groups].map(([pair, items]) => {
    const baseline = items.find(t => t.variant === "fresh");
    const complete = items.length === VARIANTS.length && new Set(items.map(t => t.variant)).size === VARIANTS.length && items.every(t => t.condition === items[0].condition);
    const observedBindingsEqual = complete && items.every(t => JSON.stringify(t.summary?.observedBindings) === JSON.stringify(items[0].summary?.observedBindings));
    const deltas = items.filter(t => t !== baseline).map(t => {
      const usable = complete && observedBindingsEqual && baseline?.summary && t.summary;
      const usage = Object.fromEntries(["input", "output", "cacheRead", "cacheWrite", "totalTokens"].map(name => {
        const a = baseline?.summary?.usage[name], b = t.summary?.usage[name];
        return [name, usable && a.missingCalls === 0 && b.missingCalls === 0 ? b.known - a.known : null];
      }));
      return { variant: t.variant, elapsedMs: usable ? t.summary.elapsedMs - baseline.summary.elapsedMs : null,
        repairRounds: usable ? t.summary.repairRounds - baseline.summary.repairRounds : null,
        oraclePass: t.summary?.quality.pass ?? null, baselineOraclePass: baseline?.summary?.quality.pass ?? null, usage };
    });
    return { pair, complete, observedBindingsEqual, deltas };
  });
}

async function readOracle(oracleFile, oracleHash, workspace, taskDir, timeoutMs, env) {
  if (digest(await fs.readFile(oracleFile)) !== oracleHash) fail("pinned independent oracle changed before evaluation");
  const result = await processRun(process.execPath, [oracleFile, workspace, taskDir], { cwd: workspace, timeoutMs, env });
  if (digest(await fs.readFile(oracleFile)) !== oracleHash) fail("pinned independent oracle changed during evaluation");
  if (result.exitCode !== 0 || result.stopReason) return { pass: null, omissions: null, regressions: null, error: "independent oracle did not complete normally", process: result };
  const q = JSON.parse(result.stdout);
  keys(q, ["version", "pass", "omissions", "regressions", "evidence"], "oracle result");
  if (q.version !== 1 || (q.pass !== null && typeof q.pass !== "boolean")) fail("invalid oracle version or pass");
  for (const name of ["omissions", "regressions"]) if (q[name] !== null && (!Array.isArray(q[name]) || q[name].some(v => typeof v !== "string" || !v))) fail(`oracle ${name} must be null or named findings`);
  if (!Array.isArray(q.evidence) || q.evidence.length === 0 || q.evidence.some(v => typeof v !== "string" || !v)) fail("oracle requires independent evidence");
  return { ...q, omissionsCount: q.omissions?.length ?? null, regressionsCount: q.regressions?.length ?? null, oracleSHA256: oracleHash };
}

async function evaluate(manifest, options) {
  validateManifest(manifest);
  if (manifest.execution === "live" && !options.live) fail("live experiments require --live; they use the configured model accounts");
  if (!path.isAbsolute(options.flywheel)) fail("--flywheel must name an absolute executable path");
  await fs.access(options.flywheel, constants.X_OK);
  const source = await fs.realpath(manifest.source.repository);
  const commit = git(source, ["rev-parse", `${manifest.source.commit}^{commit}`]);
  if (commit !== manifest.source.commit) fail("source commit does not match manifest");
  const tree = git(source, ["rev-parse", `${commit}^{tree}`]);
  const base = JSON.parse(git(source, ["show", `${commit}:${manifest.config}`]));
  if (manifest.execution === "live" && Object.values(base.drivers || {}).includes("mock")) fail("live experiments cannot contain mock driver bindings");
  const out = path.resolve(options.output);
  await fs.mkdir(out, { mode: 0o700 }); // An existing destination is never overwritten.
  const report = { version: 1, execution: manifest.execution, source: { repository: source, commit, tree },
    startedAt: new Date().toISOString(), engine: options.flywheel, engineSHA256: digest(await fs.readFile(options.flywheel)),
    manifestSHA256: digest(JSON.stringify(canonical(manifest))), configurationSHA256: digest(JSON.stringify(canonical(base))),
    conditions: "same pinned task, binding, effort, permission and oracle; only Execute/Review history modes differ; Plan/Judge explicitly fresh in every arm",
    trials: [], comparisons: [], complete: false, stopReason: null };
  await saveJSON(path.join(out, "manifest.json"), manifest);
  const persist = async () => { report.comparisons = compareTrials(report.trials); await saveJSON(path.join(out, "report.json"), report); };
  let usedCalls = 0;
  try {
    for (let rep = 0; rep < manifest.repetitions; rep++) {
      for (let caseIndex = 0; caseIndex < manifest.cases.length; caseIndex++) {
        const c = manifest.cases[caseIndex];
        const offset = (rep + caseIndex) % VARIANTS.length;
        const order = VARIANTS.slice(offset).concat(VARIANTS.slice(0, offset));
        for (const variant of order) {
          if (usedCalls >= manifest.limits.maxCalls) fail("experiment call limit reached before next trial");
          if (digest(await fs.readFile(options.flywheel)) !== report.engineSHA256) fail("engine changed after experiment conditions were frozen");
          const trialID = `${c.id}-${rep + 1}-${variant}`;
          const dir = path.join(out, trialID);
          await fs.mkdir(dir, { mode: 0o700 });
          const workspace = path.join(dir, "workspace");
          git(source, ["clone", "--quiet", "--no-hardlinks", "--no-checkout", "--", source, workspace]);
          git(workspace, ["checkout", "--quiet", "--detach", commit]);
          git(workspace, ["config", "user.name", "Flywheel Session Evaluation"]);
          git(workspace, ["config", "user.email", "session-evaluation@localhost"]);
          const taskDir = path.join(workspace, c.taskDir);
          const physicalTask = await fs.realpath(taskDir);
          if (!physicalTask.startsWith(workspace + path.sep)) fail("task directory escapes the isolated workspace");
          try { await fs.lstat(path.join(taskDir, "state.json")); fail("evaluation source already contains task state"); }
          catch (error) { if (error.code !== "ENOENT") throw error; }
          const oracleBytes = git(source, ["show", `${commit}:${c.oracle}`], false);
          const oracleFile = path.join(dir, "oracle.cjs");
          await fs.writeFile(oracleFile, oracleBytes, { mode: 0o400 });
          const oracleHash = digest(oracleBytes);
          // A configuration-change experiment must not edit the candidate's
          // frozen task contracts. Use the engine's explicit global config
          // entry point and remove overriding root/task configs in this
          // disposable clone before the engine chooses its initial BaseRef.
          for (const localConfig of [path.join(workspace, "flywheel.config.json"), path.join(taskDir, "flywheel.config.json")]) {
            try {
              if (!(await fs.lstat(localConfig)).isFile()) fail("source configuration must be a regular file");
              await fs.unlink(localConfig);
            } catch (error) { if (error.code !== "ENOENT") throw error; }
          }
          const configPath = path.join(dir, "flywheel.config.json");
          const trialEnv = { ...(options.env || process.env), FLYWHEEL_CONFIG: configPath };
          let cfg = variantConfig(base, variant);
          await saveJSON(configPath, cfg);
          git(workspace, ["add", "--all"]);
          git(workspace, ["commit", "--quiet", "--allow-empty", "-m", "chore: isolate paired evaluation configuration"]);
          const trial = { id: trialID, case: c.id, repetition: rep + 1, variant, order,
            condition: digest(JSON.stringify(canonical({ commit, base, case: c }))), workspace,
            oracleSHA256: oracleHash, configuration: configPath, policy: cfg.sessions, steps: [] };
          report.trials.push(trial);
          let beforeCalls = 0;
          for (const step of c.steps) {
            if (step.kind === "inject") {
              // Controlled perturbations model L1/Review regressions. Freeze
              // the injector too and verify it leaves Go state and contracts
              // byte-for-byte intact; this is not an alternate state writer.
              const before = await taskSnapshot(taskDir);
              const configHash = digest(await fs.readFile(configPath));
              const scriptBytes = git(source, ["show", `${commit}:${step.script}`], false);
              const injector = path.join(dir, `inject-${trial.steps.length + 1}.cjs`);
              await fs.writeFile(injector, scriptBytes, { mode: 0o400 });
              const result = await processRun(process.execPath, [injector, workspace, taskDir], { cwd: workspace, env: trialEnv, timeoutMs: manifest.limits.runTimeoutMs });
              if (JSON.stringify(await taskSnapshot(taskDir)) !== JSON.stringify(before)) fail("injection changed Go-owned evidence or frozen contracts");
              if (digest(await fs.readFile(configPath)) !== configHash) fail("injection changed frozen configuration");
              if (result.exitCode !== 0 || result.stopReason || digest(await fs.readFile(injector)) !== digest(scriptBytes)) fail("controlled injection did not complete with its pinned source");
              trial.steps.push({ ...step, scriptSHA256: digest(scriptBytes), durationMs: result.durationMs });
              continue;
            }
            if (step.kind === "configure") {
              const binding = Object.hasOwn(cfg[step.section] || {}, step.name) ? cfg[step.section][step.name] : null;
              if (!binding) fail("configure step refers to a missing binding");
              const before = structuredClone(binding);
              if (step.model !== undefined) binding.model = step.model;
              if (step.thinking !== undefined) binding.thinking = step.thinking;
              await saveJSON(configPath, cfg);
              trial.steps.push({ ...step, before, after: structuredClone(binding) });
              continue;
            }
            if (usedCalls >= manifest.limits.maxCalls) fail("experiment call limit reached before next step");
            if (JSON.stringify(canonical(await jsonFile(configPath))) !== JSON.stringify(canonical(cfg))) fail("frozen trial configuration changed outside a declared configure step");
            const argv = ["run", taskDir];
            if (step.only) argv.push("--only", step.only);
            if (step.packet) argv.push("--packet", step.packet);
            if (step.pauseBeforeReview) argv.push("--pause-before", "review");
            const remaining = manifest.limits.maxCalls - usedCalls;
            const metricsPath = path.join(taskDir, "metrics.jsonl");
            const result = await processRun(options.flywheel, argv, { cwd: workspace, env: trialEnv,
              timeoutMs: manifest.limits.runTimeoutMs, stopAfterMs: step.stopAfterMs,
              shouldStop: async () => resolveMetricRows(await metricRows(metricsPath, false)).calls.length - beforeCalls >= remaining });
            const row = { ...step, process: { ...result } };
            delete row.process.stdout; delete row.process.stderr;
            const stepIndex = trial.steps.length + 1;
            await fs.writeFile(path.join(dir, `step-${stepIndex}.stdout.txt`), result.stdout, { mode: 0o600 });
            await fs.writeFile(path.join(dir, `step-${stepIndex}.stderr.txt`), result.stderr, { mode: 0o600 });
            trial.steps.push(row);
            const rows = await metricRows(metricsPath);
            const callCount = resolveMetricRows(rows).calls.length;
            usedCalls += callCount - beforeCalls; beforeCalls = callCount;
            const state = await jsonFile(path.join(taskDir, "state.json"), {});
            row.stateStatus = state.status || "unavailable";
            await saveJSON(path.join(dir, `step-${stepIndex}.state.json`), state);
            trial.summary = summarizeTrial(state, rows, trial.steps, { pass: null, omissions: null, regressions: null }, c);
            await persist();
            if (JSON.stringify(canonical(await jsonFile(configPath))) !== JSON.stringify(canonical(cfg))) fail("frozen trial configuration changed during model execution");
            if (result.forcedKill || trial.summary.recovery.unresolved || (result.stopReason && result.stopReason !== "planned-stop")) fail("execution or cleanup is unresolved; experiment stopped without replay");
            if (![0, 3, 143].includes(result.exitCode)) fail("engine admission or runtime failure; inspect the saved trial");
          }
          const rows = await metricRows(path.join(taskDir, "metrics.jsonl"));
          const state = await jsonFile(path.join(taskDir, "state.json"), {});
          const quality = await readOracle(oracleFile, oracleHash, workspace, taskDir, manifest.limits.runTimeoutMs, trialEnv);
          trial.summary = summarizeTrial(state, rows, trial.steps, quality, c);
          await saveJSON(path.join(dir, "metrics.json"), rows);
          await persist();
          options.onTrial?.({ id: trial.id, calls: trial.summary.calls, status: trial.summary.status, oraclePass: quality.pass });
        }
      }
    }
    report.complete = true;
  } catch (error) { report.stopReason = error.message; }
  report.finishedAt = new Date().toISOString();
  report.calls = usedCalls;
  report.scenarioCoverage = Object.fromEntries(SCENARIOS.map(name => {
    const trials = report.trials.filter(t => t.summary?.scenario.requested === name);
    return [name, { planned: manifest.cases.some(c => c.scenario === name), observed: trials.filter(t => t.summary.scenario.observed === true).length,
      unobserved: trials.filter(t => t.summary.scenario.observed === false).length, unknown: trials.filter(t => t.summary.scenario.observed === null).length }];
  }));
  await persist();
  return report;
}

async function main(argv) {
  const options = {};
  for (let i = 0; i < argv.length; i++) {
    const name = argv[i];
    if (name === "--live") options.live = true;
    else if (["--manifest", "--flywheel", "--output"].includes(name) && argv[i + 1]) options[name.slice(2)] = argv[++i];
    else fail("usage: node scripts/evaluate-sessions.js --manifest FILE --flywheel /absolute/flywheel --output NEW_DIRECTORY [--live]");
  }
  if (!options.manifest || !options.flywheel || !options.output) fail("--manifest, --flywheel and --output are required");
  const manifest = await jsonFile(options.manifest);
  const report = await evaluate(manifest, options);
  process.stdout.write(JSON.stringify({ complete: report.complete, execution: report.execution, calls: report.calls,
    trials: report.trials.length, stopReason: report.stopReason, report: path.resolve(options.output, "report.json") }) + "\n");
  return report.complete ? 0 : 1;
}

module.exports = { VARIANTS, SCENARIOS, validateManifest, variantConfig, resolveMetricRows, summarizeTrial, compareTrials, evaluate, main };
if (require.main === module) main(process.argv.slice(2)).then(code => { process.exitCode = code; }, error => { process.stderr.write(error.message + "\n"); process.exitCode = 1; });
