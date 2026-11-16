#!/usr/bin/env node
"use strict";

// Deterministic native app-server fixture for the *actual* Go engine, worker v3
// and public Codex Pro machine API. It edits only the corpus implementation and
// normal Executor report. It NEVER writes state.json, receipts or metrics. The
// real engine owns those files and runs the frozen L1/Review/commit gates.
// Reported usage is synthetic protocol data, not a model performance result.
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { createInterface } = require("node:readline");
const { spawnSync } = require("node:child_process");

if (process.argv[2] === "--version") { console.log("codex-cli 0.167.0-offline-fixture"); process.exit(0); }
if (process.argv[2] !== "app-server" || process.argv[3] !== "--stdio") throw new Error("expected the native app-server entry point");
if (!process.env.CODEX_HOME || !fs.existsSync(path.join(process.env.CODEX_HOME, ".flywheel-fixture-only"))) throw new Error("fixture requires a separate marked CODEX_HOME");
if (fs.readFileSync(path.join(process.cwd(), "eval", "CORPUS"), "utf8") !== "flywheel-session-evaluation-v1\n") throw new Error("fixture only runs inside the explicitly prepared corpus");

const home = process.env.CODEX_HOME;
const log = record => fs.appendFileSync(path.join(home, "native-calls.jsonl"), JSON.stringify({ pid: process.pid, cwd: process.cwd(), ...record }) + "\n");
const send = value => process.stdout.write(JSON.stringify(value) + "\n");
const reply = (id, result) => send({ jsonrpc: "2.0", id, result });
const notify = (method, params) => send({ jsonrpc: "2.0", method, params });
let history, persistent = false, threadId, turnId;
const historyFile = () => path.join(home, `thread-${threadId}.json`);
const save = () => { if (persistent) fs.writeFileSync(historyFile(), JSON.stringify(history)); };
const expected = values => {
  const sorted = [...new Set(values)].sort((a,b) => a-b), output = [];
  for (const n of sorted) { const tail=output[output.length-1]; if(tail && n===tail[1]+1)tail[1]=n; else output.push([n,n]); }
  return output;
};
const implementation = `module.exports = function compactRanges(values) {
  const sorted = [...new Set(values)].sort((a,b) => a-b), output = [];
  for (const n of sorted) { const tail=output[output.length-1]; if(tail && n===tail[1]+1)tail[1]=n; else output.push([n,n]); }
  return output;
};
`;

function turn(prompt) {
  const role = prompt.includes("You are the executor for exactly one task packet") ? "execute"
    : prompt.includes("You are the reviewer for one executed unit") ? "review" : null;
  const id = prompt.match(/lib\/([a-z][a-z0-9-]+)\.cjs/)?.[1];
  if (!role || !id) throw new Error("fixture received an unsupported role or task");
  if (history.role && history.role !== role) throw new Error("role history was shared across Executor and Reviewer");
  history.role = role;
  const file = path.join(process.cwd(), "lib", `${id}.cjs`);
  const cmd = ["eval/acceptance.cjs", id];
  if (role === "execute") fs.writeFileSync(file, implementation);
  const check = spawnSync(process.execPath, cmd, { cwd: process.cwd(), encoding: "utf8" });
  if (role === "execute") {
    const taskDir = path.join(process.cwd(), "docs", id);
    fs.mkdirSync(path.join(taskDir, "reports"), { recursive: true });
    fs.writeFileSync(path.join(taskDir, "reports", "M01-T01.done.json"), JSON.stringify({
      "requirements-covered": ["R-01"], "changed-files": [`lib/${id}.cjs`],
      "commands-run": [`node ${cmd.join(" ")} (exit ${check.status})`], "failures": check.status === 0 ? [] : ["frozen acceptance failed"],
    }));
    return "Implementation and normal done-report written; frozen acceptance exit " + check.status;
  }
  // The reviewer inspects current code and behavior; retained history never
  // turns the previous pass into evidence for this candidate.
  const fn = require(file);
  const sample = [3, 1, 2, 2, -4], before = JSON.stringify(sample), actual = fn(sample);
  const pass = check.status === 0 && before === JSON.stringify(sample) && JSON.stringify(actual) === JSON.stringify(expected([3,1,2,2,-4]));
  const reason = pass ? `lib/${id}.cjs:1 preserves caller input; node eval/acceptance.cjs ${id} exit 0`
    : `lib/${id}.cjs:1 mutates the caller array; independently observed [3,1,2,2,-4] changed`;
  return JSON.stringify({ verdict: pass ? "pass" : "return", score: pass ? 1 : 0.4, reasons: [reason], "quick-eligible": true,
    requirements: [{ id: "R-01", pass, evidence: [reason] }] });
}

process.on("SIGTERM", () => process.exit(0));
process.stdout.on("error", () => process.exit(1));
log({ event: "spawn" });
createInterface({ input: process.stdin }).on("line", line => {
  const message = JSON.parse(line), p = message.params || {};
  if (message.method === "initialize") reply(message.id, { userAgent: "flywheel-offline-corpus-fixture" });
  else if (["thread/start", "thread/resume"].includes(message.method)) {
    persistent = message.method === "thread/resume" || p.ephemeral === false;
    threadId = p.threadId || crypto.randomUUID();
    if (!/^[a-f0-9-]{36}$/.test(threadId)) throw new Error("invalid fixture history handle");
    if (message.method === "thread/resume") {
      if (!fs.existsSync(historyFile())) return send({ jsonrpc: "2.0", id: message.id, error: { code: -32001, message: "no rollout exists for this thread" } });
      history = JSON.parse(fs.readFileSync(historyFile(), "utf8"));
      if (history.cwd !== p.cwd || !history.total) throw new Error("resume did not refer to a completed same-workspace history");
      notify("thread/tokenUsage/updated", { threadId, turnId: history.turnId, tokenUsage: { total: history.total, last: history.total } });
    } else history = { threadId, cwd: p.cwd, total: null, role: null };
    save();
    const sandbox = p.sandbox === "read-only" ? { type: "readOnly", networkAccess: false }
      : p.sandbox === "danger-full-access" ? { type: "dangerFullAccess" }
        : { type: "workspaceWrite", writableRoots: [p.cwd], networkAccess: false, excludeTmpdirEnvVar: true, excludeSlashTmp: true };
    log({ event: message.method, threadId, persistent, model: p.model, sandbox: p.sandbox });
    reply(message.id, { thread: { id: threadId, ephemeral: !persistent }, cwd: p.cwd, model: p.model,
      approvalPolicy: p.approvalPolicy, approvalsReviewer: p.approvalsReviewer || "user", sandbox, reasoningEffort: null });
  } else if (message.method === "thread/name/set") {
    const requestId = (Number.isSafeInteger(message.id) && message.id >= 0) ||
      (typeof message.id === "string" && message.id.length > 0);
    if (!requestId || !history || p.threadId !== threadId || history.threadId !== threadId ||
        typeof p.name !== "string" || !p.name.trim() || Buffer.byteLength(p.name) > 1024 ||
        /[\u0000-\u001f\u007f]/.test(p.name)) {
      return send({ jsonrpc: "2.0", id: message.id ?? null, error: { code: -32602, message: "invalid fixture thread name request" } });
    }
    history = { ...history, name: p.name }; save();
    log({ event: "thread/name/set", threadId, name: p.name });
    reply(message.id, {});
  } else if (message.method === "turn/start") {
    turnId = crypto.randomUUID();
    const prompt = (p.input || []).filter(x => x.type === "text").map(x => x.text).join("\n");
    log({ event: "turn/start", threadId, turnId, effort: p.effort, promptSHA256: crypto.createHash("sha256").update(prompt).digest("hex") });
    reply(message.id, { turn: { id: turnId } });
    notify("turn/started", { threadId, turn: { id: turnId } });
    const output = turn(prompt);
    const input = Math.max(1, Math.ceil(Buffer.byteLength(prompt) / 4));
    const delta = { inputTokens: input, outputTokens: 40, totalTokens: input + 40, cachedInputTokens: 0, cacheWriteInputTokens: 0, reasoningOutputTokens: 0 };
    const total = Object.fromEntries(Object.entries(delta).map(([key, value]) => [key, value + (history.total?.[key] || 0)]));
    history = { ...history, turnId, total }; save();
    notify("thread/tokenUsage/updated", { threadId, turnId, tokenUsage: { total, last: delta } });
    notify("item/completed", { threadId, turnId, item: { type: "agentMessage", phase: "final_answer", text: output } });
    notify("turn/completed", { threadId, turn: { id: turnId, status: "completed" } });
  } else if (message.method === "turn/interrupt") {
    reply(message.id, {}); notify("turn/completed", { threadId, turn: { id: turnId, status: "interrupted" } });
  }
});
