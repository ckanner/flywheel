"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const fs = require("node:fs/promises");
const { tmpdir } = require("node:os");
const path = require("node:path");
const { capture, readReceipt, writeLauncher, digest, installationLock } = require("../install.js");
const meta = require("../../package.json");

const repository = path.resolve(__dirname, "../..");
const platform = `${process.platform}-${process.arch === "x64" ? "amd64" : process.arch}`;
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

function alive(pid) {
  assert.ok(Number.isSafeInteger(pid) && pid > 0, "process checks require a complete positive PID");
  try { process.kill(pid, 0); return true; } catch (error) {
    if (error.code === "ESRCH") return false;
    throw error;
  }
}

async function waitFor(check, message) {
  const deadline = Date.now() + 8000;
  while (!await check()) {
    assert.ok(Date.now() < deadline, message);
    await delay(15);
  }
}

async function waitForPid(filename, message) {
  let pid;
  await waitFor(async () => {
    let content;
    try { content = await fs.readFile(filename, "utf8"); }
    catch (error) { if (error.code === "ENOENT") return false; throw error; }
    if (!/^[1-9]\d*$/.test(content) || !Number.isSafeInteger(Number(content))) return false;
    pid = Number(content);
    return true;
  }, message);
  return pid;
}

function publishPidSource(filename) {
  return `fs.writeFileSync(${JSON.stringify(filename + ".tmp")}, String(process.pid));
    fs.renameSync(${JSON.stringify(filename + ".tmp")}, ${JSON.stringify(filename)});`;
}

async function temporary(t, beforeRemove) {
  const root = await fs.realpath(await fs.mkdtemp(path.join(tmpdir(), "flywheel setup $() ")));
  t.after(async () => { await beforeRemove?.(); await fs.rm(root, { recursive: true, force: true }); });
  return root;
}

const nativeBody = `#!/usr/bin/env node
const fs = require("node:fs"); const path = require("node:path");
const args = process.argv.slice(2);
const record = value => fs.appendFileSync(process.env.TEST_INSTALL_LOG, JSON.stringify(value) + "\\n");
record({ kind: "native", args });
if (args[0] === "version") console.log("flywheel ${meta.version}");
else if (args[0] === "install-skills") {
  const index = args.indexOf("--project");
  const base = path.join(index < 0 ? process.env.HOME : args[index + 1], ".agents", "skills");
  const dest = path.join(base, "flywheel-run");
  if (process.env.TEST_SKILL_CONFLICT !== "1") {
    fs.mkdirSync(dest, { recursive: true });
    fs.writeFileSync(path.join(dest, "SKILL.md"), "fixture skill");
  }
  const configFailure = process.env.TEST_CONFIG_FAILURE === '1';
  const ok = process.env.TEST_SKILL_CONFLICT !== "1" && !configFailure;
  const configPath = process.env.FLYWHEEL_CONFIG || path.join(process.env.HOME, '.config', 'flywheel', 'flywheel.config.json');
  const existing = fs.existsSync(configPath);
  if (!existing && !configFailure) {
    fs.mkdirSync(path.dirname(configPath), { recursive: true });
    fs.writeFileSync(configPath, JSON.stringify({ version: 1, _note: 'user-owned fixture configuration' }));
  }
  console.log(JSON.stringify({ protocol: 1, ok, targetBase: base, config: { path: configPath, action: configFailure ? 'error' : existing ? 'existing' : 'created', error: configFailure ? 'config write denied' : undefined },
    results: [{ name: "flywheel-run", dest, action: process.env.TEST_SKILL_CONFLICT === "1" ? "conflict" : "copied" }] }));
  process.exitCode = ok ? 0 : 1;
} else if (args[0] === "doctor") {
  if (process.env.TEST_DOCTOR_HANG === "1") {
    let stopping = false;
    process.on("SIGTERM", () => { if (stopping) return; stopping = true;
      setTimeout(() => { record({ kind: "doctor-cleanup" }); process.exit(0); }, 100); });
    record({ kind: "doctor-ready" }); setInterval(() => {}, 1000);
  } else {
    const ok = process.env.TEST_DOCTOR_READY !== "0";
    console.log(JSON.stringify({ kind: "doctor", ok, inferenceTested: false,
      checks: ok ? [] : [{ id: "kimi-command", status: "fail", summary: "Kimi CLI is not installed", remedy: "install the CLI and rerun doctor" }] }));
    process.exitCode = ok ? 0 : 1;
  }
} else console.log(JSON.stringify({ args, dsh: process.env.FLYWHEEL_DSH_BIN, cwd: process.cwd() }));
`;

async function installation(t) {
  const active = [];
  const directory = await temporary(t, async () => {
    for (const run of active) {
      if (!run.finished) run.child.kill("SIGTERM");
      await Promise.race([run.exit, delay(1000)]);
      if (!run.finished) run.child.kill("SIGKILL");
      await run.exit;
    }
  });
  const root = path.join(directory, "installed ' $(touch SHELL_INJECTION) directory");
  const home = path.join(directory, "home");
  const prefix = path.join(directory, "prefix ' $(touch PREFIX_INJECTION)");
  const commands = path.join(directory, "commands");
  await fs.mkdir(root);
  await Promise.all([fs.mkdir(path.join(root, "bin")), fs.mkdir(path.join(root, "npm")),
    fs.mkdir(home), fs.mkdir(commands)]);
  const packageMeta = { ...meta };
  await fs.writeFile(path.join(root, "package.json"), JSON.stringify(packageMeta));
  await fs.copyFile(path.join(repository, "npm", "install.js"), path.join(root, "npm", "install.js"));
  for (const name of ["flywheel.js", "runtime.js", "flywheel-dsh-worker.js", "flywheel-codex-pro-worker.js", "flywheel-claude-code-pro-worker.js", "flywheel-kimi-native-worker.js"]) {
    await fs.copyFile(path.join(repository, "bin", name), path.join(root, "bin", name));
  }
  const native = path.join(root, "native", platform);
  const plugin = root;
  await Promise.all([fs.mkdir(native, { recursive: true }), fs.mkdir(path.join(plugin, "dist"), { recursive: true })]);
  await fs.writeFile(path.join(native, "flywheel"), nativeBody, { mode: 0o755 });
  await fs.writeFile(path.join(root,'native/manifest.json'),JSON.stringify({protocol:1,version:meta.version,platforms:{[platform]:{bytes:Buffer.byteLength(nativeBody),sha256:require('node:crypto').createHash('sha256').update(nativeBody).digest('hex')}}}));
  await fs.writeFile(path.join(plugin, "dist", "kimi-native-worker.mjs"), "// Unselected native Kimi worker.\n");
  await fs.writeFile(path.join(plugin, "dist", "worker.mjs"), "// No model worker runs during installation.\n");
  await fs.writeFile(path.join(plugin, "dist", "codex-pro-worker.mjs"), `
    if (process.argv[2] !== '--doctor') throw new Error('installation must never run a model');
    console.log(JSON.stringify({ protocol: 1, type: 'doctor', package: 'dsh-subagent-codex-pro',
      version: process.env.TEST_ADAPTER_VERSION || '0.10.1', machineProtocol: 1, available: true, inferenceTested: false }));
  `);
  await fs.writeFile(path.join(plugin, "dist", "claude-code-pro-worker.mjs"), "throw new Error(\"unselected optional adapter must not be invoked\");\n");
  const dsh = path.join(commands, "dsh ' $(touch DSH_INJECTION)");
  await fs.writeFile(dsh, `#!/usr/bin/env node
const fs = require("node:fs"); const args = process.argv.slice(2);
fs.appendFileSync(process.env.TEST_INSTALL_LOG, JSON.stringify({ kind: "dsh", args }) + "\\n");
if (args[0] === "--version") console.log("dsh 0.2.1-rc.5");
else if (args.includes("list")) console.log(JSON.stringify([{ dependencies: {
  "@ckanner/flywheel": { version: process.env.TEST_DSH_LIST_VERSION || ${JSON.stringify(meta.version)} }
} }]));
`, { mode: 0o755 });
  const npm = path.join(commands, "npm-fixture.js");
  await fs.writeFile(npm, 'throw new Error("--no-global with an explicit prefix must not invoke npm");\n');
  const log = path.join(directory, "commands.jsonl");
  const environment = { ...process.env, HOME: home, PATH: path.dirname(process.execPath), npm_execpath: npm,
    TEST_INSTALL_LOG: log, FLYWHEEL_DSH_BIN: dsh };
  delete environment.FLYWHEEL_DSH_WORKER;
  delete environment.FLYWHEEL_CODEX_PRO_WORKER;
  delete environment.FLYWHEEL_CLAUDE_CODE_PRO_WORKER;
  const cli = path.join(root, "bin", "flywheel.js");
  function launch(args = [], extraEnvironment = {}) {
    const child = spawn(process.execPath, [cli, "install", ...args], { cwd: directory,
      env: { ...environment, ...extraEnvironment }, stdio: ["ignore", "pipe", "pipe"] });
    const run = { child, stdout: "", stderr: "", finished: false };
    child.stdout.setEncoding("utf8"); child.stderr.setEncoding("utf8");
    child.stdout.on("data", data => { run.stdout += data; });
    child.stderr.on("data", data => { run.stderr += data; });
    run.exit = new Promise((resolve, reject) => {
      child.once("error", reject);
      child.once("close", (code, signal) => { run.finished = true; resolve({ code, signal, stdout: run.stdout, stderr: run.stderr }); });
    });
    active.push(run); return run;
  }
  async function records() {
    return (await fs.readFile(log, "utf8").catch(() => "")).trim().split("\n").filter(Boolean).map(line => JSON.parse(line));
  }
  const args = ["--json", "--no-global", "--prefix", prefix];
  return { directory, root, home, prefix, dsh, cli, environment, launch, records, args };
}

function resultDocument(result, expectedCode) {
  assert.equal(result.code, expectedCode, result.stderr || result.stdout);
  assert.equal(result.signal, null);
  const document = JSON.parse(result.stdout);
  assert.equal(document.protocol, 1);
  return document;
}

test("invalid installer arguments return one JSON error without creating installation state", { timeout: 10000 }, async t => {
  const f = await installation(t);
  for (const args of [["--unknown", "--json"], ["--json", "--prefix"], ["--json", "--timeout-ms", "not-a-number"],
    ["--json", "--profile", "$(profile)"], ["--json", "--no-dsh", "--no-dsh"]]) {
    const report = resultDocument(await f.launch(args).exit, 1);
    assert.equal(report.ok, false);
    assert.equal(report.steps.length, 0);
    assert.ok(report.error);
  }
  await assert.rejects(fs.access(path.join(f.home, ".config")), { code: "ENOENT" });
});

test('unsupported Node reports the required floor before changing installation state', async t => {
  const f = await installation(t);
  const preload = path.join(f.directory, 'unsupported-node.cjs');
  await fs.writeFile(preload, `Object.defineProperty(process.versions, 'node', { value: '22.17.9' });\n`);
  const report = resultDocument(await f.launch(f.args, { NODE_OPTIONS: `--require=${JSON.stringify(preload)}` }).exit, 1);
  assert.equal(report.ok, false);
  assert.equal(report.steps.length, 0);
  assert.match(report.error, /Node\.js 22\.18\.0/);
  await assert.rejects(fs.access(path.join(f.home, '.config')), { code: 'ENOENT' });
});

test("installer executes special paths literally and repeat setup keeps a stable desktop launcher", { timeout: 15000 }, async t => {
  const f = await installation(t);
  const first = resultDocument(await f.launch(f.args).exit, 0);
  assert.equal(first.ok, true);
  assert.equal(first.ready, true);
  assert.equal(first.authentication, "unknown");
  assert.equal(first.configuration.action, 'created');
  assert.equal(first.configuration.automaticMigration, false);
  assert.deepEqual(first.adapter, { package: 'dsh-subagent-codex-pro', version: '0.10.1', protocol: 1, inferenceTested: false });
  const record = await readReceipt(path.join(f.home, ".config", "flywheel", "installation.json"));
  assert.equal(record.launcher.path, first.paths.launcher);
  assert.equal(record.codexProWorker, first.paths.codexProWorker);
  assert.equal(record.claudeCodeProWorker, first.paths.claudeCodeProWorker);
  assert.deepEqual(record.adapter, first.adapter);
  assert.equal(digest(await fs.readFile(record.launcher.path)), record.launcher.sha256);
  assert.equal((await fs.stat(record.launcher.path)).mode & 0o777, 0o755);
  assert.equal((await fs.stat(path.join(f.home, ".config", "flywheel", "installation.json"))).mode & 0o777, 0o600);
  const desktopEnvironment = { ...f.environment, PATH: "" };
  delete desktopEnvironment.FLYWHEEL_DSH_BIN;
  const echo = await capture(first.paths.launcher, ["echo", "literal ' $(touch ARG_INJECTION)"], { environment: desktopEnvironment, cwd: f.directory });
  assert.equal(echo.code, 0, echo.stderr);
  assert.deepEqual(JSON.parse(echo.stdout), { args: ["echo", "literal ' $(touch ARG_INJECTION)"], dsh: f.dsh, cwd: f.directory });
  const second = resultDocument(await f.launch(f.args).exit, 0);
  assert.equal(second.configuration.preserved, true);
  assert.match(second.configuration.message, /升级不会替换旧模型、渠道或思考等级/);
  assert.deepEqual(second.configuration.migration, { command: 'flywheel', args: ['config', 'migrate', '--preset', 'subscriptions', '--cwd', f.directory] });
  assert.equal(second.paths.launcher, first.paths.launcher);
  assert.deepEqual(await readReceipt(path.join(f.home, ".config", "flywheel", "installation.json")), record);
  const records = await f.records();
  assert.equal(records.some(entry => entry.kind === 'native' && entry.args[0] === 'config'), false, 'setup must not implicitly migrate model selections');
  const add = records.find(entry => entry.kind === "dsh" && entry.args.includes("add"));
  assert.ok(add.args.includes(`@ckanner/flywheel@${meta.version}`));
  assert.ok(add.args.includes("--save-exact"));
  assert.ok(add.args.includes("--ignore-scripts"));
  for (const name of ["SHELL_INJECTION", "PREFIX_INJECTION", "DSH_INJECTION", "ARG_INJECTION"]) {
    await assert.rejects(fs.access(path.join(f.directory, name)), { code: "ENOENT" });
  }
});

test("explicit no-dsh is reported as skipped while missing CLI prerequisites remain recoverable", { timeout: 15000 }, async t => {
  const f = await installation(t);
  const installed = resultDocument(await f.launch([...f.args, "--no-dsh"], { TEST_DOCTOR_READY: "0" }).exit, 0);
  assert.equal(installed.ok, true);
  assert.equal(installed.ready, false);
  assert.equal(installed.steps.find(step => step.name === "dsh-plugin").status, "skipped");
  assert.equal(installed.steps.find(step => step.name === "preflight").status, "warning");
  assert.equal(installed.diagnostic.checks[0].id, "kimi-command");
  assert.ok(!(await f.records()).some(entry => entry.kind === "dsh"));
  const ready = resultDocument(await f.launch([...f.args, "--no-dsh"]).exit, 0);
  assert.equal(ready.ready, true);
  assert.equal(ready.paths.launcher, installed.paths.launcher);
});

test("human setup output retains diagnostic remedies and does not announce an uninstalled DSH plugin", { timeout: 10000 }, async t => {
  const f = await installation(t);
  const result = await f.launch([...f.args.filter(value => value !== "--json"), "--no-dsh"], { TEST_DOCTOR_READY: "0" }).exit;
  assert.equal(result.code, 0, result.stderr);
  assert.match(result.stdout, /Kimi CLI is not installed/);
  assert.match(result.stdout, /install the CLI and rerun doctor/);
  assert.doesNotMatch(result.stdout, /重载插件/);
  assert.match(result.stdout, /迁移预览:/);
  assert.match(result.stdout, /subscriptions/);
});

test('configuration initialization errors never claim that defaults were created or migrated', async t => {
  const f = await installation(t);
  const filename = path.join(f.home, '.config', 'flywheel', 'flywheel.config.json');
  const original = '{"_note":"user configuration","routes":{"default":{"harness":"zcode","model":"zai/glm-5.3"}}}\n';
  await fs.mkdir(path.dirname(filename), { recursive: true });
  await fs.writeFile(filename, original);
  const failed = resultDocument(await f.launch(f.args, { TEST_CONFIG_FAILURE: '1' }).exit, 1);
  assert.equal(failed.configuration.action, 'error');
  assert.equal(failed.configuration.automaticMigration, false);
  assert.match(failed.configuration.message, /初始化失败/);
  assert.doesNotMatch(failed.configuration.message, /新建全局默认配置/);
  assert.equal(await fs.readFile(filename, 'utf8'), original);
  const human = await f.launch(f.args.filter(value => value !== '--json'), { TEST_CONFIG_FAILURE: '1' }).exit;
  assert.equal(human.code, 1);
  assert.match(human.stdout, /配置初始化失败/);
  assert.doesNotMatch(human.stdout, /新建全局默认配置/);
});

test("missing DSH reports partial installation and recovers on a later explicit command", { timeout: 15000 }, async t => {
  const f = await installation(t);
  const missing = resultDocument(await f.launch([...f.args, "--dsh-bin", path.join(f.directory, "missing-dsh")]).exit, 1);
  assert.equal(missing.ok, false);
  assert.equal(missing.steps.find(step => step.name === "global-cli").status, "ok");
  assert.equal(missing.steps.find(step => step.name === "skills").status, "ok");
  assert.match(missing.steps.find(step => step.name === "dsh-plugin").error, /DSH command not found.*--dsh-bin/);
  const recovered = resultDocument(await f.launch([...f.args, "--dsh-bin", f.dsh]).exit, 0);
  assert.equal(recovered.ok, true);
  assert.equal(recovered.paths.launcher, missing.paths.launcher);
});

test("DSH reporting a different installed version cannot complete setup", { timeout: 10000 }, async t => {
  const f = await installation(t);
  const report = resultDocument(await f.launch(f.args, { TEST_DSH_LIST_VERSION: "0.0.1" }).exit, 1);
  assert.equal(report.ok, false);
  assert.match(report.error, /exact installed plugin version/);
});

test('setup rejects an adapter whose installed public API reports another version', async t => {
  const f = await installation(t);
  const report = resultDocument(await f.launch(f.args, { TEST_ADAPTER_VERSION: '0.7.0' }).exit, 1);
  assert.equal(report.ok, false);
  assert.equal(report.steps.at(-1).name, 'global-cli');
  assert.match(report.error, /exact public machine API/);
  assert.ok(!(await f.records()).some(entry => entry.kind === 'dsh'));
});

test("a skills conflict remains visible and prevents later desktop changes", { timeout: 10000 }, async t => {
  const f = await installation(t);
  const report = resultDocument(await f.launch(f.args, { TEST_SKILL_CONFLICT: "1" }).exit, 1);
  assert.equal(report.ok, false);
  assert.equal(report.steps.find(step => step.name === "skills").summary.results[0].action, "conflict");
  assert.ok(!report.steps.some(step => step.name === "desktop-launcher"));
});

test("setup cancellation waits for a trapping CLI and cannot publish success", { timeout: 15000 }, async t => {
  const f = await installation(t);
  const run = f.launch([...f.args, "--no-dsh"], { TEST_DOCTOR_HANG: "1" });
  await waitFor(async () => (await f.records()).some(entry => entry.kind === "doctor-ready"), "doctor fixture never started");
  run.child.kill("SIGTERM");
  const report = resultDocument(await run.exit, 143);
  assert.equal(report.ok, false);
  assert.equal(report.cancelledBy, "SIGTERM");
  assert.match(report.error, /cancelled/);
  assert.ok((await f.records()).some(entry => entry.kind === "doctor-cleanup"));
});

test("step timeout rejects a CLI that traps termination as exit zero", { timeout: 15000 }, async t => {
  const f = await installation(t);
  // Admit the Node-based version/adapter fixtures before timing out the hanging
  // doctor. This checks timeout/cleanup semantics, not sub-second process startup.
  const report = resultDocument(await f.launch([...f.args, "--no-dsh", "--timeout-ms", "3000"], { TEST_DOCTOR_HANG: "1" }).exit, 1);
  assert.equal(report.ok, false);
  assert.equal(report.steps.at(-1).name, "preflight");
  assert.match(report.error, /timed out/);
  assert.ok((await f.records()).some(entry => entry.kind === "doctor-cleanup"));
});

test("cancellation during final unlock cannot return a successful report or hide cleanup errors", { timeout: 15000 }, async t => {
  for (const failUnlock of [false, true]) {
    const f = await installation(t);
    const preload = path.join(f.directory, "cancel-at-final-unlock.cjs");
    await fs.writeFile(preload, `
const fs = require("node:fs/promises");
const path = require("node:path");
const unlink = fs.unlink;
fs.unlink = async function(filename) {
  if (path.basename(filename) === "npm-install.lock") {
    await new Promise(resolve => {
      const pending = setInterval(() => {}, 1000);
      process.once("SIGTERM", () => { clearInterval(pending); resolve(); });
      process.kill(process.pid, "SIGTERM");
    });
    if (process.env.TEST_UNLOCK_FAILURE === "1") throw new Error("fixture final unlock failed");
  }
  return unlink.call(this, filename);
};
`);
    const report = resultDocument(await f.launch([...f.args, "--no-dsh"], {
      NODE_OPTIONS: `--require=${JSON.stringify(preload)}`,
      TEST_UNLOCK_FAILURE: failUnlock ? "1" : "0",
    }).exit, 143);
    assert.equal(report.ok, false);
    assert.equal(report.cancelledBy, "SIGTERM");
    assert.match(report.error, /installation cancelled by SIGTERM/);
    assert.ok(report.steps.every(step => ["ok", "skipped"].includes(step.status)), "the signal must arrive after all setup steps finish");
    if (failUnlock) assert.match(report.error, /fixture final unlock failed/);
    else await assert.rejects(fs.access(path.join(f.home, ".config", "flywheel", "npm-install.lock")), { code: "ENOENT" });
  }
});

test("managed launcher updates preserve user edits and invalid installation records", { timeout: 10000 }, async t => {
  const directory = await temporary(t);
  const opts = { directory, prefix: path.join(directory, "prefix"), cli: path.join(directory, "cli.js"),
    worker: path.join(directory, "worker.mjs"), environment: { PATH: path.dirname(process.execPath) } };
  const first = await writeLauncher(opts);
  const filename = path.join(directory, "installation.json");
  const second = await writeLauncher({ ...opts, previous: await readReceipt(filename), cli: path.join(directory, "updated cli.js") });
  assert.equal(second.launcher.path, first.launcher.path);
  assert.notEqual(second.launcher.sha256, first.launcher.sha256);
  const ownedRecord = await fs.readFile(filename, "utf8");
  await fs.writeFile(second.launcher.path, "#!/bin/sh\necho my-custom-launcher\n");
  await assert.rejects(writeLauncher({ ...opts, previous: second }), /launcher has user changes/);
  assert.equal(await fs.readFile(second.launcher.path, "utf8"), "#!/bin/sh\necho my-custom-launcher\n");
  assert.equal(await fs.readFile(filename, "utf8"), ownedRecord);
  await fs.writeFile(filename, "user-owned configuration, not an installation receipt");
  await assert.rejects(readReceipt(filename), /preserving installation record/);
  assert.equal(await fs.readFile(filename, "utf8"), "user-owned configuration, not an installation receipt");
});

test("setup preserves a modified launcher and exposes the failed step in its JSON result", { timeout: 15000 }, async t => {
  const f = await installation(t);
  const first = resultDocument(await f.launch([...f.args, "--no-dsh"]).exit, 0);
  const edited = "#!/bin/sh\necho user-controlled-entry\n";
  await fs.writeFile(first.paths.launcher, edited);
  const failed = resultDocument(await f.launch([...f.args, "--no-dsh"]).exit, 1);
  assert.equal(failed.ok, false);
  assert.equal(failed.steps.at(-1).name, "desktop-launcher");
  assert.match(failed.error, /user changes/);
  assert.equal(await fs.readFile(first.paths.launcher, "utf8"), edited);
});

test("capture cancels a whole installation process group after its leader exits", { timeout: 10000 }, async t => {
  const directory = await temporary(t);
  const pidFile = path.join(directory, "worker.pid");
  // A reader can observe file creation before PID bytes are written. Empty is
  // not readiness: Number("") is 0, and kill(0, 0) probes our own process group.
  await fs.writeFile(pidFile, "");
  const parent = path.join(directory, "leader.cjs");
  const workerSource = `const fs = require('node:fs'); process.on('SIGTERM', () => {});
    ${publishPidSource(pidFile)} setInterval(() => {}, 1000);`;
  await fs.writeFile(parent, `const { spawn } = require('node:child_process');
    spawn(process.execPath, ['-e', ${JSON.stringify(workerSource)}], { stdio: 'ignore' });
    process.on('SIGTERM', () => process.exit(0)); setInterval(() => {}, 1000);`);
  let worker;
  t.after(() => { if (worker && alive(worker)) process.kill(worker, "SIGKILL"); });
  const controller = new AbortController();
  const resultPromise = capture(process.execPath, [parent], { signal: controller.signal, timeoutMs: 5000,
    cleanupGraceMs: 50, cleanupTimeoutMs: 1000 });
  worker = await waitForPid(pidFile, "worker never started");
  controller.abort();
  const result = await resultPromise;
  assert.notEqual(result.code, 0);
  assert.equal(result.cancelled, true);
  assert.equal(alive(worker), false, `leader close must not cancel escalation while PID ${worker} is still alive; ${result.error}`);
});

test("normal exit zero cannot hide a surviving installation subprocess", { timeout: 10000 }, async t => {
  const directory = await temporary(t);
  const pidFile = path.join(directory, "worker.pid");
  const parent = path.join(directory, "leader.cjs");
  const workerSource = `const fs = require('node:fs'); process.on('SIGTERM', () => {});
    ${publishPidSource(pidFile)} process.stdout.write('ready'); setInterval(() => {}, 1000);`;
  await fs.writeFile(parent, `const { spawn } = require('node:child_process');
    const worker = spawn(process.execPath, ['-e', ${JSON.stringify(workerSource)}], { stdio: ['ignore', 'pipe', 'ignore'] });
    worker.stdout.once('data', () => process.exit(0));`);
  let worker;
  t.after(() => { if (worker && alive(worker)) process.kill(worker, "SIGKILL"); });
  const pending = capture(process.execPath, [parent], { timeoutMs: 5000, cleanupGraceMs: 50, cleanupTimeoutMs: 1000 });
  worker = await waitForPid(pidFile, "worker never started");
  const result = await pending;
  assert.notEqual(result.code, 0);
  assert.ok(result.error);
  assert.equal(alive(worker), false);
});

test("inherited output handles outside the owned group produce a bounded failure", { timeout: 10000 }, async t => {
  const directory = await temporary(t);
  const pidFile = path.join(directory, "escaped.pid");
  const parent = path.join(directory, "leader.cjs");
  const workerSource = `const fs = require('node:fs');
    ${publishPidSource(pidFile)} setInterval(() => {}, 1000);`;
  await fs.writeFile(parent, `const { spawn } = require('node:child_process');
    const worker = spawn(process.execPath, ['-e', ${JSON.stringify(workerSource)}], { detached: true, stdio: 'inherit' });
    worker.unref();`);
  let worker;
  t.after(async () => {
    if (worker && alive(worker)) process.kill(worker, "SIGKILL");
    if (worker) await waitFor(() => !alive(worker), "escaped fixture did not exit during test cleanup");
  });
  const started = Date.now();
  const pending = capture(process.execPath, [parent], { timeoutMs: 500, cleanupGraceMs: 50, cleanupTimeoutMs: 200 });
  worker = await waitForPid(pidFile, "escaped fixture never started");
  const result = await pending;
  assert.notEqual(result.code, 0);
  assert.ok(result.error);
  assert.ok(Date.now() - started < 5000, "output descriptors must not make setup wait indefinitely");
});

test("concurrent setup preserves the active install lock and can retry after its owner exits", { timeout: 15000 }, async t => {
  const f = await installation(t);
  const first = f.launch([...f.args, "--no-dsh"], { TEST_DOCTOR_HANG: "1" });
  await waitFor(async () => (await f.records()).some(entry => entry.kind === "doctor-ready"), "first installation never reached preflight");
  for (let attempt = 0; attempt < 2; attempt++) {
    const blocked = resultDocument(await f.launch([...f.args, "--no-dsh"]).exit, 1);
    assert.equal(blocked.ok, false);
    assert.match(blocked.error, /installation lock exists/);
  }
  first.child.kill("SIGTERM");
  assert.equal(resultDocument(await first.exit, 143).ok, false);
  const retry = resultDocument(await f.launch([...f.args, "--no-dsh"]).exit, 0);
  assert.equal(retry.ok, true);
});

test("an installer never removes a lock whose ownership changed", async t => {
  const directory = await temporary(t);
  const unlock = await installationLock(directory);
  const filename = path.join(directory, "npm-install.lock");
  const replacement = JSON.stringify({ pid: process.pid, token: "different-owner" });
  await fs.writeFile(filename, replacement);
  await assert.rejects(unlock(), /ownership changed.*preserving/);
  assert.equal(await fs.readFile(filename, "utf8"), replacement);
  await assert.rejects(installationLock(directory), /installation lock exists/);
  assert.equal(await fs.readFile(filename, "utf8"), replacement);
});
