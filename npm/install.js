// Copyright (c) 2026 ckanner. SPDX-License-Identifier: UNLICENSED
"use strict";

const fs = require("node:fs");
const fsp = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
const crypto = require("node:crypto");
const { spawn } = require("node:child_process");
const { createRequire } = require("node:module");
const { StringDecoder } = require("node:string_decoder");
const meta = require("../package.json");

const OWNER = "@ckanner/flywheel";
const PLUGIN = "@ckanner/dsh-plugin-flywheel";
const SIGNALS = ["SIGTERM", "SIGINT", "SIGHUP"];
const HELP = `Flywheel npm installation

  npx -y @ckanner/flywheel@latest install [options]
  flywheel setup [options]

Install this exact version of the CLI, worker, embedded skills and DSH plugin.
No model inference or account login is performed. Node >=22.18.0 and the existing
DSH desktop command are prerequisites. Kimi/Codex availability is reported by
the local preflight; this installer does not replace another harness.

Options:
  --json              Print one structured result (including partial failures)
  --prefix PATH       npm global installation prefix (default: npm prefix -g)
  --project PATH      Install skills for this project, instead of the user
  --dsh-bin PATH      Existing DSH desktop command (default: env/PATH)
  --profile NAME      DSH target profile (default: desktop)
  --no-dsh            Explicitly install only CLI, worker and shared skills
  --no-global         Configure this already installed package without npm -g
  --timeout-ms N      Deadline per external installation step (default: 600000)
  --help              Show this help
`;

function parseOptions(args) {
  const options = { json: false, dsh: true, global: true, profile: "desktop", timeoutMs: 600000 };
  const values = new Map([["--prefix", "prefix"], ["--project", "project"], ["--dsh-bin", "dshBin"], ["--profile", "profile"], ["--timeout-ms", "timeoutMs"]]);
  const seen = new Set();
  for (let index = 0; index < args.length; index++) {
    const argument = args[index];
    if (seen.has(argument)) throw new Error(`duplicate option: ${argument}`);
    seen.add(argument);
    if (argument === "--json") options.json = true;
    else if (argument === "--no-dsh") options.dsh = false;
    else if (argument === "--no-global") options.global = false;
    else if (argument === "--help" || argument === "-h") options.help = true;
    else if (values.has(argument)) {
      const value = args[++index];
      if (!value || value.startsWith("--") || value.includes("\0")) throw new Error(`${argument} requires a value`);
      options[values.get(argument)] = value;
    } else throw new Error(`unknown installation option: ${argument}`);
  }
  options.timeoutMs = Number(options.timeoutMs);
  if (!Number.isSafeInteger(options.timeoutMs) || options.timeoutMs < 100 || options.timeoutMs > 1800000) {
    throw new Error("--timeout-ms must be an integer between 100 and 1800000");
  }
  if (!/^[A-Za-z0-9][A-Za-z0-9_-]*$/.test(options.profile)) throw new Error("invalid DSH profile name");
  if (options.prefix) options.prefix = path.resolve(options.prefix);
  if (options.project) options.project = fs.realpathSync(options.project);
  return options;
}

function digest(data) {
  return crypto.createHash("sha256").update(data).digest("hex");
}

function shellQuote(value) {
  if (value.includes("\0") || value.includes("\n") || value.includes("\r")) throw new Error("installation paths cannot contain NUL or newline");
  return "'" + value.replaceAll("'", "'\\''") + "'";
}

function findExecutable(name, environment = process.env) {
  const candidates = name.includes(path.sep) ? [path.resolve(name)] : (environment.PATH || "").split(path.delimiter)
    .filter(directory => path.isAbsolute(directory)).map(directory => path.join(directory, name));
  for (const candidate of candidates) {
    try {
      fs.accessSync(candidate, fs.constants.X_OK);
      if (fs.statSync(candidate).isFile()) return fs.realpathSync(candidate);
    } catch { /* Try the next explicit PATH entry. */ }
  }
  return undefined;
}

function npmCommand(environment = process.env) {
  const script = environment.npm_execpath;
  if (script && path.isAbsolute(script) && fs.existsSync(script)) return [process.execPath, [script]];
  const binary = findExecutable("npm", environment);
  if (!binary) throw new Error("npm is required; install Node.js 22.18.0 or newer, then rerun this command");
  return [binary, []];
}

/** A bounded installation command. Runtime model-process ownership stays in Go. */
function capture(command, args, { environment = process.env, cwd, signal, timeoutMs = 600000,
  cleanupGraceMs = 20000, cleanupTimeoutMs = 5000 } = {}) {
  return new Promise(resolve => {
    if (signal?.aborted) return resolve({ code: 1, stdout: "", stderr: "installation cancelled", cancelled: true });
    const child = spawn(command, args, { cwd, env: environment, shell: false, detached: true, stdio: ["ignore", "pipe", "pipe"] });
    const output = { stdout: "", stderr: "" };
    const decoders = { stdout: new StringDecoder("utf8"), stderr: new StringDecoder("utf8") };
    let bytes = 0;
    let stopped;
    let startError;
    let escalation;
    let cleanupDeadline;
    let probeTimer;
    let closed = false;
    let closedAt;
    let settled = false;
    let exitCode;
    let exitSignal;
    const groupAlive = () => {
      if (!child.pid) return false;
      try { process.kill(-child.pid, 0); return true; }
      catch (error) {
        if (error.code === 'ESRCH') return false;
        // macOS can briefly return EPERM while a terminating group is reaped.
        // Keep polling within the same deadline; absence is the success proof.
        if (error.code !== 'EPERM') startError ??= `cannot confirm installation process cleanup: ${error.code}`;
        return true;
      }
    };
    const kill = name => {
      if (!child.pid) return;
      try { process.kill(-child.pid, name); } catch (error) { if (error.code !== "ESRCH") startError ??= error.message; }
    };
    const stop = reason => {
      if (stopped) return;
      stopped = reason;
      kill("SIGTERM");
      // A leader's exit is not evidence that its group is gone. In particular,
      // npm/pnpm can have a child that closes stdio and ignores TERM.
      escalation = setTimeout(() => kill("SIGKILL"), cleanupGraceMs);
      cleanupDeadline = setTimeout(() => {
        if (groupAlive()) startError ??= "could not confirm installation process-group cleanup";
        if (!closed) startError ??= "installation command did not close its output streams";
        child.stdout.destroy();
        child.stderr.destroy();
        child.unref();
        finish();
      }, cleanupGraceMs + cleanupTimeoutMs);
    };
    const cancel = () => stop("cancelled");
    signal?.addEventListener("abort", cancel, { once: true });
    const timer = setTimeout(() => stop("timed out"), timeoutMs);
    for (const stream of ["stdout", "stderr"]) child[stream].on("data", chunk => {
      bytes += chunk.length;
      if (bytes > 1024 * 1024) return stop("output exceeded 1 MiB");
      output[stream] += decoders[stream].write(chunk);
    });
    const finish = () => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      clearTimeout(escalation);
      clearTimeout(cleanupDeadline);
      clearTimeout(probeTimer);
      signal?.removeEventListener("abort", cancel);
      for (const stream of ["stdout", "stderr"]) output[stream] += decoders[stream].end();
      resolve({ ...output, code: stopped || startError || exitSignal ? (exitCode || 1) : (exitCode ?? 1), signal: exitSignal,
        ...(stopped ? { cancelled: stopped === "cancelled", error: stopped } : {}), ...(startError ? { error: startError } : {}) });
    };
    const confirm = () => {
      if (settled) return;
      if (!groupAlive()) return finish();
      if (stopped || Date.now() - closedAt >= cleanupTimeoutMs) stop("installation command left descendants running");
      probeTimer = setTimeout(confirm, 25);
    };
    child.once("error", error => { startError = error.message; });
    child.once("close", (code, name) => {
      closed = true;
      closedAt = Date.now();
      exitCode = code;
      exitSignal = name;
      clearTimeout(timer);
      confirm();
    });
  });
}

function requireSuccess(result, label) {
  if (result.code !== 0 || result.error || result.signal) {
    const detail = result.error || result.stderr.trim().slice(-6000) || `exit ${result.code}`;
    throw new Error(`${label}: ${detail}`);
  }
  return result.stdout.trim();
}

function parseDocument(result, label) {
  try { return JSON.parse(result.stdout); } catch { throw new Error(`${label} did not return one valid JSON document`); }
}

async function readReceipt(filename) {
  try {
    const stat = await fsp.lstat(filename);
    if (!stat.isFile() || stat.isSymbolicLink()) throw new Error("installation record is not a regular file");
    const value = JSON.parse(await fsp.readFile(filename, "utf8"));
    if (value.protocol !== 1 || value.owner !== OWNER || typeof value.launcher?.sha256 !== "string") throw new Error("unrecognized installation record");
    return value;
  } catch (error) {
    if (error.code === "ENOENT") return undefined;
    throw new Error(`preserving installation record ${filename}: ${error.message}`);
  }
}

async function atomicWrite(filename, data, mode) {
  const temporary = filename + ".tmp-" + crypto.randomUUID();
  try {
    await fsp.writeFile(temporary, data, { mode, flag: "wx" });
    await fsp.rename(temporary, filename);
  } finally {
    await fsp.rm(temporary, { force: true });
  }
}

function launcherContent({ cli, prefix, dshBin, environment = process.env }) {
  const dirs = [path.dirname(process.execPath), path.join(prefix, "bin"), dshBin && path.dirname(dshBin),
    ...(environment.PATH || "").split(path.delimiter)].filter(directory => directory && path.isAbsolute(directory)
      && !directory.includes(`${path.sep}node_modules${path.sep}.bin`) && !directory.includes(`${path.sep}_npx${path.sep}`));
  const lines = ["#!/bin/sh", "# Managed by @ckanner/flywheel. User edits are preserved on setup.",
    `export PATH=${shellQuote([...new Set(dirs)].join(path.delimiter))}`];
  if (dshBin) lines.push('if [ "${FLYWHEEL_DSH_BIN+x}" != x ]; then', `  export FLYWHEEL_DSH_BIN=${shellQuote(dshBin)}`, "fi");
  lines.push(`exec ${shellQuote(process.execPath)} ${shellQuote(cli)} "$@"`, "");
  return lines.join("\n");
}

async function writeLauncher({ directory, previous, cli, prefix, worker, codexProWorker, adapter, dshBin, environment }) {
  await fsp.mkdir(directory, { recursive: true });
  const launcher = path.join(directory, "npm-launcher");
  const content = launcherContent({ cli, prefix, dshBin, environment });
  try {
    const stat = await fsp.lstat(launcher);
    if (!stat.isFile() || stat.isSymbolicLink()) throw new Error("launcher is not a regular managed file");
    const current = await fsp.readFile(launcher);
    if (digest(current) !== digest(content) && (previous?.launcher?.path !== launcher || previous.launcher.sha256 !== digest(current))) {
      throw new Error("launcher has user changes; preserve it and configure flywheelBinary explicitly, or move it aside before retrying");
    }
  } catch (error) { if (error.code !== "ENOENT") throw error; }
  await atomicWrite(launcher, content, 0o755);
  const receipt = { protocol: 1, owner: OWNER, version: meta.version, prefix, cli, worker, codexProWorker, adapter,
    node: process.execPath, launcher: { path: launcher, sha256: digest(content) } };
  await atomicWrite(path.join(directory, "installation.json"), JSON.stringify(receipt, null, 2) + "\n", 0o600);
  return receipt;
}

function hasPluginVersion(document, version) {
  return Array.isArray(document) && document.some(project => project.dependencies?.[PLUGIN]?.version === version);
}

async function installationLock(directory) {
  await fsp.mkdir(directory, { recursive: true });
  const filename = path.join(directory, 'npm-install.lock');
  const token = crypto.randomUUID();
  let handle;
  try { handle = await fsp.open(filename, 'wx', 0o600); }
  catch (error) {
    if (error.code === 'EEXIST') throw new Error(`installation lock exists: ${filename}; wait for the other installer, or remove this lock only after confirming a previous crashed installer has exited`);
    throw error;
  }
  try { await handle.writeFile(JSON.stringify({ pid: process.pid, token })); }
  catch (error) { await fsp.rm(filename, { force: true }); throw error; }
  finally { await handle.close(); }
  return async () => {
    const value = JSON.parse(await fsp.readFile(filename, 'utf8'));
    if (value.token !== token) throw new Error('installation lock ownership changed; preserving the lock');
    await fsp.unlink(filename);
  };
}

async function install(options, { environment = process.env, cwd = process.cwd() } = {}) {
  const report = { protocol: 1, ok: false, version: meta.version, steps: [], ready: false, authentication: "unknown" };
  const controller = new AbortController();
  let cancelledBy;
  let unlock;
  const handlers = SIGNALS.map(name => {
    const handler = () => { cancelledBy ??= name; controller.abort(); };
    process.on(name, handler);
    return [name, handler];
  });
  const run = (command, args, extra = {}) => capture(command, args, { environment, cwd, timeoutMs: options.timeoutMs, signal: controller.signal, ...extra });
  const step = async (name, action) => {
    if (controller.signal.aborted) throw new Error("installation cancelled");
    const item = { name, status: "running" };
    report.steps.push(item);
    try { await action(item); if (item.status === "running") item.status = "ok"; }
    catch (error) { item.status = "failed"; item.error = error.message; throw error; }
  };
  try {
    if (!['darwin', 'linux'].includes(process.platform) || !['arm64', 'x64'].includes(process.arch)) throw new Error("unsupported installation platform");
    require('../bin/runtime.js').assertSupportedNode();
    const home = environment.HOME;
    if (!home || !path.isAbsolute(home)) throw new Error("HOME must identify the user's absolute home directory");
    const directory = path.join(home, ".config", "flywheel");
    unlock = await installationLock(directory);
    const previous = await readReceipt(path.join(directory, "installation.json"));
    const [npm, npmArgs] = npmCommand(environment);
    let prefix = options.prefix;
    if (!prefix) prefix = requireSuccess(await run(npm, [...npmArgs, "prefix", "--global"]), "npm global prefix");
    if (!path.isAbsolute(prefix)) throw new Error("npm did not return an absolute global prefix");
    report.prefix = prefix;
    let installedRoot = path.resolve(__dirname, "..");
    await step("global-cli", async item => {
      if (options.global) {
        requireSuccess(await run(npm, [...npmArgs, "install", "--global", "--prefix", prefix, "--ignore-scripts", "--include=optional",
          "--no-audit", "--no-fund", `${OWNER}@${meta.version}`]), "npm global installation");
        const globalRoot = requireSuccess(await run(npm, [...npmArgs, "root", "--global", "--prefix", prefix]), "npm global package directory");
        if (!path.isAbsolute(globalRoot)) throw new Error("npm did not return an absolute global package directory");
        installedRoot = path.join(globalRoot, "@ckanner", "flywheel");
      } else item.action = "existing-package";
      const installed = JSON.parse(await fsp.readFile(path.join(installedRoot, "package.json"), "utf8"));
      if (installed.name !== OWNER || installed.version !== meta.version) throw new Error("installed package does not match this installer version");
      const installedRequire = createRequire(path.join(installedRoot, "package.json"));
      const runtime = installedRequire("./bin/runtime.js");
      report.paths = { cli: path.join(installedRoot, "bin", "flywheel.js"), native: runtime.loadNativePath(), worker: runtime.loadWorkerPath(),
        codexProWorker: runtime.loadCodexProWorkerPath() };
      const probe = await run(process.execPath, [report.paths.codexProWorker, '--doctor']);
      requireSuccess(probe, 'installed Codex Pro dependency');
      const adapter = parseDocument(probe, 'installed Codex Pro dependency');
      if (adapter.protocol !== 1 || adapter.type !== 'doctor' || adapter.package !== 'dsh-subagent-codex-pro' ||
          adapter.version !== '0.8.0' || adapter.machineProtocol !== 1 || adapter.available !== true || adapter.inferenceTested !== false) {
        throw new Error('installed Codex Pro dependency did not verify the exact public machine API');
      }
      report.adapter = { package: adapter.package, version: adapter.version, protocol: adapter.machineProtocol, inferenceTested: false };
      const version = requireSuccess(await run(process.execPath, [report.paths.cli, "version"]), "installed CLI version");
      if (!version.split(/\s+/).includes(meta.version)) throw new Error("native binary version does not match the npm package");
      item.version = version;
    });
    await step("skills", async item => {
      const args = [report.paths.cli, "install-skills", "--json", ...(options.project ? ["--project", options.project] : [])];
      const result = await run(process.execPath, args);
      if (result.error || result.signal || ![0, 1].includes(result.code)) requireSuccess(result, 'skills installation');
      const summary = parseDocument(result, "skills installation");
      item.summary = summary;
      if (summary.protocol !== 1 || !Array.isArray(summary.results) || summary.results.length === 0) throw new Error("invalid skills installation result");
      requireSuccess(result, "skills installation");
      if (summary.ok !== true || summary.results.some(skill => ["error", "stale", "conflict"].includes(skill.action))) throw new Error("some skills were preserved due to conflicts; inspect the results and retry");
      report.skills = { count: summary.results.length, directory: summary.targetBase };
    });
    const requestedDsh = options.dshBin ?? environment.FLYWHEEL_DSH_BIN ?? "dsh";
    const dshBin = findExecutable(requestedDsh, environment);
    await step("desktop-launcher", async item => {
      const receipt = await writeLauncher({ directory, previous, cli: report.paths.cli, prefix, worker: report.paths.worker,
        codexProWorker: report.paths.codexProWorker, adapter: report.adapter, dshBin, environment });
      report.paths.launcher = receipt.launcher.path;
      item.path = receipt.launcher.path;
    });
    await step("dsh-plugin", async item => {
      if (!options.dsh) { item.status = "skipped"; item.reason = "explicit --no-dsh"; return; }
      if (!dshBin) throw new Error("DSH command not found. Enable the command in the existing desktop app, then rerun with --dsh-bin /absolute/path/to/dsh");
      item.version = requireSuccess(await run(dshBin, ["--version"]), "DSH version");
      requireSuccess(await run(dshBin, ["plugin", "--profile", options.profile, "add", "--save-exact", "--ignore-scripts", `${PLUGIN}@${meta.version}`]), "DSH plugin registration");
      const listing = await run(dshBin, ["plugin", "--profile", options.profile, "list", PLUGIN, "--depth", "0", "--json"]);
      requireSuccess(listing, "DSH plugin verification");
      if (!hasPluginVersion(parseDocument(listing, "DSH plugin list"), meta.version)) throw new Error("DSH profile did not report the exact installed plugin version");
      item.profile = options.profile;
      item.reloadRequired = true;
    });
    await step("preflight", async item => {
      const result = await run(report.paths.launcher, ["doctor", "--cwd", options.project || cwd, "--json"]);
      if (result.error || result.signal || ![0, 1].includes(result.code)) requireSuccess(result, 'installed CLI preflight');
      const diagnostic = parseDocument(result, "installed CLI preflight");
      if (result.error || result.signal || ![0, 1].includes(result.code) || typeof diagnostic.ok !== "boolean" || !Array.isArray(diagnostic.checks)
        || diagnostic.ok !== (result.code === 0)) throw new Error("installed CLI returned an invalid preflight result");
      report.diagnostic = diagnostic;
      report.ready = diagnostic.ok;
      if (!diagnostic.ok) { item.status = "warning"; item.reason = "components installed; resolve the reported harness/configuration prerequisites before running a task"; }
    });
    const finalVersion = requireSuccess(await run(process.execPath, [report.paths.cli, 'version']), 'final installed version');
    const finalPackage = JSON.parse(await fsp.readFile(path.join(installedRoot, 'package.json'), 'utf8'));
    if (finalPackage.version !== meta.version || !finalVersion.split(/\s+/).includes(meta.version)) throw new Error('installed version changed during setup; rerun the intended exact version');
    report.ok = !controller.signal.aborted;
    if (!report.ok) throw new Error("installation cancelled");
  } catch (error) {
    report.error = error.message;
  } finally {
    if (unlock) {
      try { await unlock(); }
      catch (error) { report.ok = false; report.error = error.message; }
    }
    for (const [name, handler] of handlers) process.removeListener(name, handler);
  }
  if (cancelledBy) {
    report.cancelledBy = cancelledBy;
    report.ok = false;
    const cancellation = `installation cancelled by ${cancelledBy}`;
    report.error = report.error ? `${report.error}; ${cancellation}` : cancellation;
  }
  return report;
}

async function main(args) {
  let options;
  try { options = parseOptions(args); }
  catch (error) {
    if (args.includes("--json")) console.log(JSON.stringify({ protocol: 1, ok: false, version: meta.version, steps: [], error: error.message }));
    else console.error(`flywheel install: ${error.message}`);
    return 1;
  }
  if (options.help) { process.stdout.write(HELP); return 0; }
  const report = await install(options);
  if (options.json) console.log(JSON.stringify(report));
  else {
    console.log(`Flywheel ${report.version}: ${report.ok ? "安装完成" : "安装尚未完成"}`);
    for (const item of report.steps) console.log(`  ${item.status.padEnd(7)} ${item.name}${item.error ? ": " + item.error : ""}`);
    if (report.paths?.launcher) console.log(`桌面启动入口: ${report.paths.launcher}`);
    if (report.skills) console.log(`Skills: ${report.skills.count} 个 → ${report.skills.directory}`);
    if (report.ok && report.steps.some(step => step.name === 'dsh-plugin' && step.status === 'ok')) console.log("请按 DSH 提示重载插件。");
    if (report.ok) console.log("账号授权与真实模型任务尚未验证；先查看 doctor 的检查项。");
    if (report.diagnostic?.checks) for (const check of report.diagnostic.checks.filter(value => value.status === "fail")) {
      console.log(`  预检: ${check.name || check.id}: ${check.summary || check.message || "检查失败"}`);
      if (check.remedy) console.log(`    ${check.remedy}`);
    }
    if (report.error) console.error(report.error);
  }
  if (report.cancelledBy) return 128 + os.constants.signals[report.cancelledBy];
  return report.ok ? 0 : 1;
}

module.exports = { main, install, parseOptions, findExecutable, npmCommand, capture, shellQuote, digest, readReceipt, writeLauncher, launcherContent, hasPluginVersion, installationLock };
