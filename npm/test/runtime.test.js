"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const { mkdtemp, mkdir, copyFile, chmod, writeFile, readFile, realpath, rename, rm } = require("node:fs/promises");
const { tmpdir, constants } = require("node:os");
const { join, resolve } = require("node:path");

const source = resolve(__dirname, "../..");
const platform = `${process.platform}-${process.arch === "x64" ? "amd64" : process.arch}`;
const version = "1.2.3";
const workerManifest = {
  name: "@ckanner/dsh-plugin-flywheel", version, type: "module",
  exports: { "./worker": "./dist/worker.js", "./package.json": "./package.json" },
};
const nativePreamble = `#!/usr/bin/env node
const fs = require("node:fs");
fs.appendFileSync(process.env.TEST_OWNED_PIDS, process.pid + "\\n");
`;
const echoNative = `let input = "";
process.stdin.setEncoding("utf8");
process.stdin.on("data", chunk => { input += chunk; });
process.stdin.on("end", () => {
  console.log(JSON.stringify({ argv: process.argv.slice(2), cwd: process.cwd(),
    marker: process.env.TEST_MARKER, worker: process.env.FLYWHEEL_DSH_WORKER, input }));
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
    dependencies: { "@ckanner/dsh-plugin-flywheel": version },
    optionalDependencies: { [`@ckanner/flywheel-${platform}`]: version },
  }));
  for (const name of ["flywheel.js", "flywheel-dsh-worker.js", "runtime.js"]) {
    await copyFile(join(source, "bin", name), join(root, "bin", name));
  }
  const nativeDir = join(root, "node_modules", "@ckanner", `flywheel-${platform}`);
  const workerDir = join(root, "node_modules", "@ckanner", "dsh-plugin-flywheel");
  await mkdir(nativeDir, { recursive: true });
  await mkdir(join(workerDir, "dist"), { recursive: true });
  const nativePath = join(nativeDir, "flywheel");
  const workerPath = join(workerDir, "dist", "worker.js");
  await writeFile(join(nativeDir, "package.json"), JSON.stringify({ name: `@ckanner/flywheel-${platform}`, version }));
  await writeFile(nativePath, nativePreamble + native, { mode: 0o755 });
  await writeFile(join(workerDir, "package.json"), JSON.stringify(workerManifest));
  await writeFile(workerPath, worker);

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
  return { root, container, nativePath, nativeDir, workerPath, workerDir, launch };
}

test("npm CLI preserves literal argv, stdin, cwd, environment, stderr and native exit status", async t => {
  const f = await fixture(t);
  const args = ["control", "config", "--cwd", "a path $(touch should-not-exist)", "中文"];
  const result = await f.launch(args, { env: { TEST_MARKER: "kept", TEST_NATIVE_EXIT: "7" }, input: "one\ntwo\n" }).exit;
  assert.equal(result.code, 7);
  assert.equal(result.signal, null);
  assert.deepEqual(JSON.parse(result.stdout), { argv: args, cwd: f.root, marker: "kept", worker: f.workerPath, input: "one\ntwo\n" });
  assert.equal(result.stderr, "native stderr\n");
});

test("an explicit worker override is preserved, even when the bundled worker is unavailable", async t => {
  const f = await fixture(t);
  await rm(f.workerDir, { recursive: true });
  for (const override of ["/custom worker", ""]) {
    const result = await f.launch(["version"], { env: { FLYWHEEL_DSH_WORKER: override } }).exit;
    assert.equal(result.code, 0, result.stderr);
    assert.equal(JSON.parse(result.stdout).worker, override);
  }
});

test("missing platform or worker packages fail visibly instead of selecting another installation", async t => {
  const f = await fixture(t);
  await rm(f.nativeDir, { recursive: true });
  const native = await f.launch(["version"]).exit;
  assert.equal(native.code, 1);
  assert.match(native.stderr, /platform package .* is missing/);
  await rm(f.workerDir, { recursive: true });
  const worker = await f.launch([], { executable: "flywheel-dsh-worker.js" }).exit;
  assert.equal(worker.code, 1);
  assert.match(worker.stderr, /matching .* worker is missing/);
});

test("a non-executable native binary reports a launch failure", async t => {
  const f = await fixture(t);
  await chmod(f.nativePath, 0o644);
  const result = await f.launch(["version"]).exit;
  assert.equal(result.code, 1);
  assert.match(result.stderr, /could not start .*EACCES/);
});

test("resolved native and worker versions must match the meta package's exact pins", async t => {
  const f = await fixture(t);
  await writeFile(join(f.workerDir, "package.json"), JSON.stringify({ ...workerManifest, version: "1.2.2" }));
  const wrongWorker = await f.launch(["version"]).exit;
  assert.equal(wrongWorker.code, 1);
  assert.match(wrongWorker.stderr, /version mismatch for @ckanner\/dsh-plugin-flywheel: expected 1\.2\.3, found 1\.2\.2/);
  // Explicit user-managed workers do not pretend to be verified package peers.
  const override = await f.launch(["version"], { env: { FLYWHEEL_DSH_WORKER: "/custom/worker" } }).exit;
  assert.equal(override.code, 0, override.stderr);
  await writeFile(join(f.workerDir, "package.json"), JSON.stringify(workerManifest));
  await writeFile(join(f.nativeDir, "package.json"), JSON.stringify({ name: `@ckanner/flywheel-${platform}`, version: "1.2.2" }));
  const wrongNative = await f.launch(["version"]).exit;
  assert.equal(wrongNative.code, 1);
  assert.match(wrongNative.stderr, /version mismatch for @ckanner\/flywheel-.*: expected 1\.2\.3, found 1\.2\.2/);
});

test("a stale parent dependency is rejected while a correctly hoisted package is accepted", async t => {
  const f = await fixture(t);
  const parentPackage = join(f.container, "node_modules", "@ckanner", "dsh-plugin-flywheel");
  await mkdir(join(parentPackage, ".."), { recursive: true });
  await rename(f.workerDir, parentPackage);
  await writeFile(join(parentPackage, "package.json"), JSON.stringify({ ...workerManifest, version: "1.2.2" }));
  const staleParent = await f.launch(["version"]).exit;
  assert.equal(staleParent.code, 1);
  assert.match(staleParent.stderr, /version mismatch/);
  await writeFile(join(parentPackage, "package.json"), JSON.stringify(workerManifest));
  const correctParent = await f.launch(["version"]).exit;
  assert.equal(correctParent.code, 0, correctParent.stderr);
  assert.equal(JSON.parse(correctParent.stdout).worker, join(parentPackage, "dist", "worker.js"));
  // A local matching dependency wins over an unrelated old parent installation.
  await mkdir(join(f.workerDir, "dist"), { recursive: true });
  await writeFile(join(f.workerDir, "package.json"), JSON.stringify(workerManifest));
  await writeFile(f.workerPath, echoWorker);
  await writeFile(join(parentPackage, "package.json"), JSON.stringify({ ...workerManifest, version: "1.2.2" }));
  const correctLocal = await f.launch(["version"]).exit;
  assert.equal(correctLocal.code, 0, correctLocal.stderr);
  assert.equal(JSON.parse(correctLocal.stdout).worker, f.workerPath);
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

test("worker shim uses its current Node executable and the package's public worker export", async t => {
  const f = await fixture(t);
  const result = await f.launch(["--literal", "$(argument)"], { executable: "flywheel-dsh-worker.js", input: '{"protocol":1}\n' }).exit;
  assert.equal(result.code, 0, result.stderr);
  assert.deepEqual(JSON.parse(result.stdout), { argv: ["--literal", "$(argument)"], node: process.execPath, cwd: f.root, input: '{"protocol":1}\n' });
});

const ownedWorker = `
const fs = require("node:fs");
fs.appendFileSync(process.env.TEST_OWNED_PIDS, process.pid + "\\n");
process.on("SIGTERM", () => setTimeout(() => process.exit(0), 80));
process.stdout.write("ready\\n");
setInterval(() => {}, 1000);
`;
const nativeOwner = `
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
