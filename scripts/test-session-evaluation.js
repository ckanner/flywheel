#!/usr/bin/env node
"use strict";

// Explicit integration gate: no skips and no real inference. Build the Go CLI
// and the Node worker, install the actual pinned public Codex package first.
const fs = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
const assert = require("node:assert/strict");
const { prepare } = require("./prepare-session-evaluation.js");
const { evaluate } = require("./evaluate-sessions.js");

async function main() {
  const engine = process.env.FLYWHEEL_EVAL_BINARY;
  if (!engine || !path.isAbsolute(engine)) throw new Error("set FLYWHEEL_EVAL_BINARY to the freshly built absolute Go CLI path");
  const root = path.resolve(__dirname, "..");
  const fixtureSource = path.join(root, "npm/test/support/session-eval-codex.cjs");
  const output = process.env.FLYWHEEL_EVAL_OUTPUT || await fs.mkdtemp(path.join(os.tmpdir(), "flywheel-native-eval-"));
  await fs.mkdir(output, { recursive: true, mode: 0o700 });
  const home = path.join(output, "fixture-codex-home"); await fs.mkdir(home, { mode: 0o700 });
  // Freeze executable bytes outside the actively edited checkout, so an
  // unrelated checkout refresh cannot alter permissions between trials.
  const fixture = path.join(home, "native-codex.cjs");
  await fs.copyFile(fixtureSource, fixture); await fs.chmod(fixture, 0o755);
  const workerRoot = path.join(output, "frozen-worker");
  await fs.mkdir(workerRoot, { mode: 0o700 });
  await fs.writeFile(path.join(workerRoot, "package.json"), JSON.stringify({ type: "module", private: true }));
  for (const file of ["codex-pro-worker.js", "codex-pro-bridge.js", "machine-worker.js"]) await fs.copyFile(path.join(root, "plugins/dsh/dist", file), path.join(workerRoot, file));
  const worker = path.join(workerRoot, "codex-pro-worker.js"); await fs.chmod(worker, 0o755);
  await fs.mkdir(path.join(workerRoot, "node_modules"));
  await fs.cp(path.join(root, "plugins/dsh/node_modules/dsh-subagent-codex-pro"), path.join(workerRoot, "node_modules/dsh-subagent-codex-pro"), { recursive: true, dereference: true });
  await fs.writeFile(path.join(home, ".flywheel-fixture-only"), "no account or model service\n");
  const bind = { harness: "codex-pro", model: "fixture-model", channel: "offline-subscription", thinking: "high" };
  const config = { version: 1, drivers: Object.fromEntries(["plan", "execute", "review", "judge", "intake", "garden"].map(r => [r, "real"])),
    loops: Object.fromEntries(["plan", "review", "judge", "intake", "garden"].map(r => [r, bind])), routes: { default: bind },
    channels: { "offline-subscription": { "env-key": null } }, permissions: { mode: "role-based" },
    sessions: { mode: "scoped", "policy-version": 1 }, budget: { enforce: "soft", "daily-token-limit": 2000000 },
    watchdog: { "idle-timeout-ms": 30000, "wall-timeout-ms": 60000, "kill-grace-ms": 1000 },
    verify: { "command-timeout-ms": 10000 } };
  const prepared = await prepare(config, path.join(output, "corpus"));
  const manifest = JSON.parse(await fs.readFile(prepared.manifest, "utf8"));
  manifest.execution = "offline-fixture";
  const report = await evaluate(manifest, { flywheel: engine, output: path.join(output, "results"),
    env: { ...process.env, CODEX_HOME: home, FLYWHEEL_CODEX_BIN: fixture,
      FLYWHEEL_CODEX_PRO_WORKER: worker },
    onTrial: trial => process.stdout.write(JSON.stringify({ trial }) + "\n") });
  process.stdout.write(JSON.stringify({ complete: report.complete, calls: report.calls, trials: report.trials.length,
    scenarioCoverage: report.scenarioCoverage, stopReason: report.stopReason, report: path.join(output, "results/report.json") }) + "\n");
  assert.equal(report.complete, true, report.stopReason);
  assert.equal(report.trials.length, 36);
  assert.ok(report.trials.every(t => t.summary.quality.pass === true));
  for (const coverage of Object.values(report.scenarioCoverage)) assert.equal(coverage.observed, 6);
  for (const trial of report.trials.filter(t => t.case === "review-return")) {
    assert.ok(trial.summary.callsByRole.execute >= 2);
    assert.ok(trial.summary.callsByRole.review >= 2);
    const resumes = trial.summary.recovery.resumes;
    if (trial.variant === "fresh") assert.equal(resumes, 0);
    else if (trial.variant === "executor-scoped") assert.ok(resumes >= 1);
    else assert.ok(resumes >= 2);
  }
  const native = (await fs.readFile(path.join(home, "native-calls.jsonl"), "utf8")).trim().split("\n").map(x => JSON.parse(x));
  assert.equal(native.filter(e => e.event === "turn/start").length, report.calls);
  assert.ok(native.some(e => e.event === "thread/resume"));
}
if (require.main === module) main().catch(error => { process.stderr.write((error.stack || error.message) + "\n"); process.exitCode = 1; });
