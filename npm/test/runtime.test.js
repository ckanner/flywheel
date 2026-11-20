"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { createHash } = require('node:crypto');
const { spawn } = require("node:child_process");
const { mkdtemp, mkdir, copyFile, chmod, writeFile, readFile, realpath, rename, rm } = require("node:fs/promises");
const { tmpdir, constants } = require("node:os");
const { join, resolve } = require("node:path");

const source = resolve(__dirname, "../..");
const platform = `${process.platform}-${process.arch === "x64" ? "amd64" : process.arch}`;
const version = "1.2.3";
const nativePreamble = `#!/usr/bin/env node
const fs = require("node:fs");
fs.appendFileSync(process.env.TEST_OWNED_PIDS, process.pid + "\\n");
`;
const echoNative = `let input = "";
process.stdin.setEncoding("utf8");
process.stdin.on("data", chunk => { input += chunk; });
process.stdin.on("end", () => {
  console.log(JSON.stringify({ argv: process.argv.slice(2), cwd: process.cwd(),
    marker: process.env.TEST_MARKER, worker: process.env.FLYWHEEL_DSH_WORKER, codexProWorker: process.env.FLYWHEEL_CODEX_PRO_WORKER, claudeCodeProWorker: process.env.FLYWHEEL_CLAUDE_CODE_PRO_WORKER, kimiNativeWorker: process.env.FLYWHEEL_KIMI_NATIVE_WORKER, antigravityWorker: process.env.FLYWHEEL_ANTIGRAVITY_WORKER, input }));
  console.error("native stderr");
  process.exitCode = Number(process.env.TEST_NATIVE_EXIT || 0);
});
`;
const echoWorker = `let input = "";
process.stdin.setEncoding("utf8");
process.stdin.on("data", chunk => { input += chunk; });
process.stdin.on("end", () => console.log(JSON.stringify({ argv: process.argv.slice(2),
  node: process.execPath, cwd: process.cwd(), input })));
`;

function alive(pid) {
  try { process.kill(pid, 0); return true; } catch (error) {
    if (error.code === "ESRCH") return false;
    throw error;
  }
}

async function eventually(check, message) {
  const deadline = Date.now() + 5000;
  while (!check()) {
    assert.ok(Date.now() < deadline, message);
    await new Promise(resolve => setTimeout(resolve, 10));
  }
}

async function fixture(t, { native = echoNative, worker = echoWorker } = {}) {
  // Physical paths keep assertions meaningful on macOS /var aliases too.
  const container = await realpath(await mkdtemp(join(tmpdir(), "flywheel npm $() ")));
  const root = join(container, "package directory");
  await mkdir(root);
  const runs = [];
  await mkdir(join(root, "bin"));
  await writeFile(join(root, "package.json"), JSON.stringify({
    name: "@ckanner/flywheel", version,

  }));
  for (const name of ["flywheel.js", "flywheel-dsh-worker.js", "flywheel-codex-pro-worker.js", "flywheel-claude-code-pro-worker.js", "flywheel-kimi-native-worker.js", "flywheel-antigravity-worker.js", "runtime.js"]) {
    await copyFile(join(source, "bin", name), join(root, "bin", name));
  }
  const nativeDir = join(root, "native", platform);
  const workerDir = join(root, "dist");
  await mkdir(nativeDir, { recursive: true });
  await mkdir(workerDir, { recursive: true });
  const nativePath = join(nativeDir, "flywheel");
  const workerPath = join(workerDir, "worker.mjs");
  const codexProWorkerPath = join(workerDir, "codex-pro-worker.mjs");
  const claudeCodeProWorkerPath = join(workerDir, "claude-code-pro-worker.mjs");
  const kimiNativeWorkerPath = join(workerDir, "kimi-native-worker.mjs");
  const antigravityWorkerPath = join(workerDir, "antigravity-worker.mjs");

  await writeFile(nativePath, nativePreamble + native, { mode: 0o755 });
  await writeFile(join(root, 'native/manifest.json'), JSON.stringify({protocol:1,version,platforms:{[platform]:{bytes:Buffer.byteLength(nativePreamble+native),sha256:createHash('sha256').update(nativePreamble+native).digest('hex')}}}));
  await writeFile(workerPath, worker);
  await writeFile(codexProWorkerPath, worker);
  await writeFile(claudeCodeProWorkerPath, worker);
  await writeFile(kimiNativeWorkerPath, worker);
  await writeFile(antigravityWorkerPath, worker);

  t.after(async () => {
    for (const run of runs) {
      if (!run.finished) run.child.kill("SIGTERM");
      // A failing shim regression must not leave its fixture owner alive.
      const pids = (await readFile(run.pids, "utf8").catch(() => "")).trim().split("\n").filter(Boolean).map(Number);
      if (!run.finished) {
        for (const pid of pids) if (alive(pid)) process.kill(pid, "SIGTERM");
        await Promise.race([run.exit, new Promise(resolve => setTimeout(resolve, 500))]);
        if (!run.finished) run.child.kill("SIGKILL");
      }
      // Native fixtures record only their own processes, never external PIDs.
      for (const pid of pids) if (alive(pid)) process.kill(pid, "SIGKILL");
      await run.exit;
    }
    await rm(container, { recursive: true, force: true });
  });

  function launch(args = [], { env = {}, executable = "flywheel.js", input = "" } = {}) {
    const pids = join(root, `owned-${runs.length}.txt`);
    const childEnv = { ...process.env, TEST_OWNED_PIDS: pids, ...env };
    // The test must not accidentally inherit a user's existing worker override.
    if (!Object.hasOwn(env, "FLYWHEEL_DSH_WORKER")) delete childEnv.FLYWHEEL_DSH_WORKER;
    if (!Object.hasOwn(env, "FLYWHEEL_CODEX_PRO_WORKER")) delete childEnv.FLYWHEEL_CODEX_PRO_WORKER;
    if (!Object.hasOwn(env, "FLYWHEEL_CLAUDE_CODE_PRO_WORKER")) delete childEnv.FLYWHEEL_CLAUDE_CODE_PRO_WORKER;
    if (!Object.hasOwn(env, "FLYWHEEL_KIMI_NATIVE_WORKER")) delete childEnv.FLYWHEEL_KIMI_NATIVE_WORKER;
    if (!Object.hasOwn(env, "FLYWHEEL_ANTIGRAVITY_WORKER")) delete childEnv.FLYWHEEL_ANTIGRAVITY_WORKER;
    const child = spawn(process.execPath, [join(root, "bin", executable), ...args], {
      cwd: root, env: childEnv, stdio: ["pipe", "pipe", "pipe"],
    });
    const run = { child, pids, stdout: "", stderr: "", finished: false };
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", chunk => { run.stdout += chunk; });
    child.stderr.on("data", chunk => { run.stderr += chunk; });
    run.exit = new Promise((resolve, reject) => {
      child.once("error", reject);
      child.once("close", (code, signal) => {
        run.finished = true;
        resolve({ code, signal, stdout: run.stdout, stderr: run.stderr });
      });
    });
    child.stdin.on("error", () => {}); // Early load errors may close stdin first.
    child.stdin.end(input);
    runs.push(run);
    return run;
  }
  return { root, container, nativePath, nativeDir, workerPath, codexProWorkerPath, claudeCodeProWorkerPath, kimiNativeWorkerPath, antigravityWorkerPath, workerDir, launch };
}

test("npm CLI preserves literal argv, stdin, cwd, environment, stderr and native exit status", async t => {
  const f = await fixture(t);
  const args = ["control", "config", "--cwd", "a path $(touch should-not-exist)", "中文"];
  const result = await f.launch(args, { env: { TEST_MARKER: "kept", TEST_NATIVE_EXIT: "7" }, input: "one\ntwo\n" }).exit;
  assert.equal(result.code, 7);
  assert.equal(result.signal, null);
  assert.deepEqual(JSON.parse(result.stdout), { argv: args, cwd: f.root, marker: "kept", worker: f.workerPath, codexProWorker: f.codexProWorkerPath, claudeCodeProWorker: f.claudeCodeProWorkerPath, kimiNativeWorker: f.kimiNativeWorkerPath, antigravityWorker: f.antigravityWorkerPath, input: "one\ntwo\n" });
  assert.equal(result.stderr, "native stderr\n");
});

test("an explicit worker override is preserved, even when the bundled worker is unavailable", async t => {
  const f = await fixture(t);
  await rm(f.workerDir, { recursive: true });
  for (const override of ["/custom worker", ""]) {
    const result = await f.launch(["version"], { env: { FLYWHEEL_DSH_WORKER: override, FLYWHEEL_CODEX_PRO_WORKER: override, FLYWHEEL_CLAUDE_CODE_PRO_WORKER: override, FLYWHEEL_KIMI_NATIVE_WORKER: override, FLYWHEEL_ANTIGRAVITY_WORKER: override } }).exit;
    assert.equal(result.code, 0, result.stderr);
    assert.equal(JSON.parse(result.stdout).worker, override);
    assert.equal(JSON.parse(result.stdout).codexProWorker, override);
    assert.equal(JSON.parse(result.stdout).claudeCodeProWorker, override);
    assert.equal(JSON.parse(result.stdout).antigravityWorker, override);
  }
});

test("missing platform or worker packages fail visibly instead of selecting another installation", async t => {
  const f = await fixture(t);
  await rm(f.nativeDir, { recursive: true });
  const native = await f.launch(["version"]).exit;
  assert.equal(native.code, 1);
  assert.match(native.stderr, /Bundled native.* is missing/);
  await rm(f.workerDir, { recursive: true });
  const worker = await f.launch([], { executable: "flywheel-dsh-worker.js" }).exit;
  assert.equal(worker.code, 1);
  assert.match(worker.stderr, /Bundled dist.* is missing/);
  const codexPro = await f.launch([], { executable: "flywheel-codex-pro-worker.js" }).exit;
  assert.equal(codexPro.code, 1);
  assert.match(codexPro.stderr, /Bundled dist.* is missing/);
  const claude = await f.launch([], { executable: "flywheel-claude-code-pro-worker.js" }).exit;
  assert.equal(claude.code, 1);
  assert.match(claude.stderr, /Bundled dist.* is missing/);
});

test("a non-executable native binary reports a launch failure", async t => {
  const f = await fixture(t);
  await chmod(f.nativePath, 0o644);
  const result = await f.launch(["version"]).exit;
  assert.equal(result.code, 1);
  assert.match(result.stderr, /could not start .*EACCES/);
});

test("a mismatched native manifest or corrupted bundled engine is rejected", async t => {
  const f = await fixture(t);
  const manifestPath=join(f.root,'native/manifest.json');
  const original=await readFile(manifestPath,'utf8');
  const changed=JSON.parse(original);changed.version='1.2.2';await writeFile(manifestPath,JSON.stringify(changed));
  assert.match((await f.launch(['version']).exit).stderr,/engine version mismatch/);
  await writeFile(manifestPath,original);
  await writeFile(f.nativePath,'wrong engine bytes');
  assert.match((await f.launch(['version']).exit).stderr,/engine integrity mismatch/);
});

test("missing bundled workers never resolve from an old parent package", async t => {
  const f = await fixture(t);
  const parentPackage=join(f.container,'node_modules/@ckanner/dsh-plugin-flywheel');
  await mkdir(parentPackage,{recursive:true});await rename(f.workerDir,join(parentPackage,'dist'));
  await writeFile(join(parentPackage,'package.json'),JSON.stringify({name:'@ckanner/dsh-plugin-flywheel',version,exports:{'./worker':'./dist/worker.mjs','./package.json':'./package.json'}}));
  const result=await f.launch(['version']).exit;
  assert.equal(result.code,1);assert.match(result.stderr,/Bundled dist.* is missing/);
});

test("install and setup dispatch to the bundled installer with untouched arguments", async t => {
  const f = await fixture(t);
  await mkdir(join(f.root, "npm"));
  await writeFile(join(f.root, "npm", "install.js"), `exports.main = async args => {
    if (args[0] === "throw") throw new Error("setup fixture failure");
    console.log(JSON.stringify(args)); return 17;
  };`);
  for (const command of ["install", "setup"]) {
    const result = await f.launch([command, "--project", "literal $(project)"]).exit;
    assert.equal(result.code, 17);
    assert.deepEqual(JSON.parse(result.stdout), ["--project", "literal $(project)"]);
  }
  const failed = await f.launch(["setup", "throw"]).exit;
  assert.equal(failed.code, 1);
  assert.match(failed.stderr, /setup fixture failure/);
});

for (const executable of ['flywheel-dsh-worker.js', 'flywheel-codex-pro-worker.js', 'flywheel-claude-code-pro-worker.js', 'flywheel-kimi-native-worker.js', 'flywheel-antigravity-worker.js']) {
  test(`${executable} uses its current Node executable and the package's public worker export`, async t => {
    const f = await fixture(t);
    const result = await f.launch(["--literal", "$(argument)"], { executable, input: '{"protocol":1}\n' }).exit;
    assert.equal(result.code, 0, result.stderr);
    assert.deepEqual(JSON.parse(result.stdout), { argv: ["--literal", "$(argument)"], node: process.execPath, cwd: f.root, input: '{"protocol":1}\n' });
  });
}

test('the runtime enforces the adapter minimum of Node 22.18.0', () => {
  const { assertSupportedNode } = require('../../bin/runtime.js');
  for (const version of ['22.18.0', '22.19.1', '23.0.0', '24.22.1']) assert.doesNotThrow(() => assertSupportedNode(version));
  for (const version of ['18.20.0', '21.10.0', '22.0.0', '22.17.9', 'invalid']) assert.throws(() => assertSupportedNode(version), /22\.18\.0/);
});

const ownedWorker = `
const fs = require("node:fs");
fs.appendFileSync(process.env.TEST_OWNED_PIDS, process.pid + "\\n");
process.on("SIGTERM", () => setTimeout(() => process.exit(0), 80));
process.stdout.write("ready\\n");
setInterval(() => {}, 1000);
`;
const nativeOwner = `
const { createHash } = require('node:crypto');
const { spawn } = require("node:child_process");
const worker = spawn(process.execPath, ["-e", ${JSON.stringify(ownedWorker)}], {
  detached: true, stdio: ["ignore", "pipe", "inherit"], env: process.env,
});
let stopping = false;
for (const signal of ["SIGTERM", "SIGINT", "SIGHUP"]) process.on(signal, () => {
  if (stopping) return;
  stopping = true;
  process.kill(-worker.pid, "SIGTERM");
});
worker.stdout.once("data", () => console.log(JSON.stringify({ ready: true, native: process.pid, worker: worker.pid })));
worker.once("close", () => {
  console.log("worker-closed");
  setTimeout(() => {
    console.log("native-cleanup-complete");
    process.exit(Number(process.env.TEST_NATIVE_EXIT || 0));
  }, 150);
});
`;

for (const signal of ["SIGTERM", "SIGINT", "SIGHUP"]) {
  test(`${signal} reaches the native owner and npm waits for its worker and delayed cleanup`, { timeout: 10000 }, async t => {
    const f = await fixture(t, { native: nativeOwner });
    const run = f.launch(["doctor"]);
    await eventually(() => run.stdout.includes('"ready":true'), "native owner did not become ready");
    const identity = JSON.parse(run.stdout.trim().split("\n")[0]);
    run.child.kill(signal); // Only the npm shim PID, as DSH control does.
    const result = await run.exit;
    assert.equal(result.code, 128 + constants.signals[signal]);
    assert.equal(result.signal, null);
    assert.match(result.stdout, /worker-closed\nnative-cleanup-complete\n$/);
    assert.equal(alive(identity.native), false, "native owner must be reaped before the npm wrapper exits");
    assert.equal(alive(identity.worker), false, "the native owner's worker must be gone too");
  });
}

test("native signal death maps to its conventional exit code", async t => {
  const f = await fixture(t, { native: 'process.kill(process.pid, "SIGTERM");' });
  const result = await f.launch().exit;
  assert.equal(result.code, 143);
  assert.equal(result.signal, null);
});

test("cancellation preserves a native diagnostic failure after cleanup", async t => {
  const f = await fixture(t, { native: nativeOwner });
  const run = f.launch(["doctor"], { env: { TEST_NATIVE_EXIT: "1" } });
  await eventually(() => run.stdout.includes('"ready":true'), "native owner did not become ready");
  run.child.kill("SIGTERM");
  const result = await run.exit;
  assert.equal(result.code, 1);
  assert.match(result.stdout, /native-cleanup-complete/);
});

test("worker shim also waits for cancellation cleanup and rejects a late exit zero", async t => {
  const f = await fixture(t, { worker: `
    process.on("SIGTERM", () => setTimeout(() => { console.log("worker-cleanup-complete"); process.exit(0); }, 100));
    console.log("ready"); setInterval(() => {}, 1000);
  ` });
  const run = f.launch([], { executable: "flywheel-dsh-worker.js" });
  await eventually(() => run.stdout.includes("ready"), "worker did not become ready");
  run.child.kill("SIGTERM");
  const result = await run.exit;
  assert.equal(result.code, 143);
  assert.match(result.stdout, /worker-cleanup-complete/);
});
