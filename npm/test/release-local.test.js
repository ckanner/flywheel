"use strict";

const { test, before, after } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { execFileSync, spawnSync } = require("node:child_process");
const { PACKAGES, META, PLUGIN } = require("../../scripts/release-preflight.js");
const { snapshot } = require("../../scripts/release-build.js");
const { filenameFor, fileIntegrity, verifyCandidate } = require("../../scripts/release-candidate.js");
const { parseArgs, verifyRemoteTag, createNpmRunner, releaseLocal, main } = require("../../scripts/release-local.js");

const VERSION = "0.28.0";
const REGISTRY = "https://registry.npmjs.org/";
const TOOL_ROOT = path.resolve(__dirname, "../..");
const ok = (stdout = "") => ({ status: 0, stdout, stderr: "" });
const json = (value) => ok(JSON.stringify(value));
const missing = () => ({ status: 1, stdout: JSON.stringify({ error: { code: "E404" } }), stderr: "" });

// An injection regression must fail locally, never reach a real npm publish.
let npmGuard;
const originalPath = process.env.PATH;
before(() => {
  npmGuard = fs.mkdtempSync(path.join(os.tmpdir(), "flywheel-no-real-npm-"));
  fs.writeFileSync(path.join(npmGuard, "npm"),
    "#!/bin/sh\nprintf '%s\\n' 'Real npm is forbidden in release-local tests' >&2\nexit 97\n", { mode: 0o755 });
  process.env.PATH = `${npmGuard}${path.delimiter}${originalPath ?? ""}`;
});
after(() => {
  if (originalPath === undefined) delete process.env.PATH;
  else process.env.PATH = originalPath;
  if (npmGuard) fs.rmSync(npmGuard, { recursive: true, force: true });
});

function testEnvironment() {
  return { PATH: process.env.PATH, GIT_CONFIG_NOSYSTEM: "1", GIT_CONFIG_GLOBAL: os.devNull,
    GIT_TERMINAL_PROMPT: "0" };
}

function temporary(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "flywheel-local-release-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  return root;
}

function git(root, ...args) {
  return execFileSync("git", ["-c", "user.name=release-test", "-c", "user.email=release-test@example.invalid",
    "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", ...args], {
    cwd: root, env: testEnvironment(), encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], timeout: 10000,
  }).trim();
}

function manifests() {
  const values = {};
  for (const entry of PACKAGES) {
    const value = { name: entry.name, version: VERSION, license: "UNLICENSED",
      repository: { type: "git", url: "git+https://github.com/ckanner/flywheel.git", directory: entry.directory } };
    if (entry.platform) {
      const [os, arch] = entry.platform.split("-");
      Object.assign(value, { os: [os], cpu: [arch === "amd64" ? "x64" : arch] });
    }
    values[entry.name] = value;
  }
  values[META].optionalDependencies = Object.fromEntries(PACKAGES.filter((entry) => entry.platform).map((entry) => [entry.name, VERSION]));
  values[META].dependencies = { [PLUGIN]: VERSION };
  values[META].bin = { "flywheel-codex-pro-worker": "bin/flywheel-codex-pro-worker.js" };
  values[PLUGIN].dependencies = { "dsh-subagent-codex-pro": "0.8.0" };
  values[PLUGIN].exports = { "./codex-pro-worker": "./dist/codex-pro-worker.js" };
  values[PLUGIN].bin = { "flywheel-codex-pro-worker": "./dist/codex-pro-worker.js" };
  values[META].bin['flywheel-claude-code-pro-worker'] = 'bin/flywheel-claude-code-pro-worker.js';
  values[PLUGIN].exports['./claude-code-pro-worker'] = './dist/claude-code-pro-worker.js';
  values[PLUGIN].bin['flywheel-claude-code-pro-worker'] = './dist/claude-code-pro-worker.js';
  values[PLUGIN].peerDependencies = { 'dsh-subagent-claude-code-pro': '0.3.2' };
  values[PLUGIN].peerDependenciesMeta = { 'dsh-subagent-claude-code-pro': { optional: true } };
  return values;
}

function repack(fixture, entry, contents = "verified local candidate", manifest = fixture.values[entry.name]) {
  const staging = fs.mkdtempSync(path.join(fixture.root, "pack-"));
  const file = path.join(fixture.directory, entry.filename);
  try {
    fs.mkdirSync(path.join(staging, "package"));
    fs.writeFileSync(path.join(staging, "package/package.json"), JSON.stringify(manifest));
    fs.writeFileSync(path.join(staging, "package/content.txt"), `${contents}: ${entry.name}\n`);
    execFileSync("tar", ["-czf", file, "-C", staging, "package"], { stdio: "pipe", timeout: 10000 });
  } finally { fs.rmSync(staging, { recursive: true, force: true }); }
  entry.bytes = fs.statSync(file).size;
  entry.integrity = fileIntegrity(file);
}

function fixture(t, { annotated = false, externalDirectory = false, changeManifests } = {}) {
  const root = temporary(t);
  const sourceDirectory = path.join(root, "source checkout");
  const remote = path.join(root, "origin.git");
  const directory = externalDirectory ? path.join(root, "candidate archives") : path.join(sourceDirectory, "dist-npm");
  fs.mkdirSync(sourceDirectory);
  const values = manifests();
  changeManifests?.(values);
  for (const entry of PACKAGES) {
    const location = path.join(sourceDirectory, entry.directory);
    fs.mkdirSync(location, { recursive: true });
    fs.writeFileSync(path.join(location, "package.json"), JSON.stringify(values[entry.name]));
  }
  fs.writeFileSync(path.join(sourceDirectory, ".gitignore"), "dist-npm/\n");
  fs.writeFileSync(path.join(sourceDirectory, "source.txt"), "reviewed release source\n");
  git(sourceDirectory, "init", "-b", "main");
  git(sourceDirectory, "add", ".");
  git(sourceDirectory, "commit", "-m", "release fixture");
  git(root, "init", "--bare", remote);
  git(sourceDirectory, "remote", "add", "origin", remote);
  if (annotated) git(sourceDirectory, "tag", "-a", `v${VERSION}`, "-m", "original release annotation");
  else git(sourceDirectory, "tag", `v${VERSION}`);
  git(sourceDirectory, "push", "origin", `refs/tags/v${VERSION}`);
  const candidate = { format_version: 1, version: VERSION, source: {
    commit: git(sourceDirectory, "rev-parse", "HEAD"),
    tree: git(sourceDirectory, "rev-parse", "HEAD^{tree}"),
    dirty: false, snapshot_sha256: snapshot(sourceDirectory),
  }, packages: PACKAGES.map((entry) => ({ name: entry.name, version: VERSION, filename: filenameFor(entry.name, VERSION) })) };
  fs.mkdirSync(directory, { recursive: true });
  const result = { root, sourceDirectory, directory, remote, candidate, values,
    options: { sourceDirectory, directory, repositoryVisibility: "private" } };
  result.save = () => fs.writeFileSync(path.join(directory, "candidate.json"), JSON.stringify(candidate, null, 2) + "\n");
  for (const entry of candidate.packages) repack(result, entry);
  result.save();
  return result;
}

function registry(fixture) {
  const entries = structuredClone(fixture.candidate.packages);
  const state = { events: [], versions: new Map(), latest: new Map(entries.map((entry) => [entry.name, "0.27.0"])),
    scope: "undefined", installResult: ok(), registryResult: ok(), failPublish: null,
    afterInstall: null, afterRegistry: null, afterWrite: null, elapsed: 0 };
  state.spawnSync = (command, args, options) => {
    if (command === "git") {
      assert.equal(options.cwd, fixture.sourceDirectory);
      // Only a local fixture remote is reachable through this runner.
      assert.equal(git(fixture.sourceDirectory, "remote", "get-url", "origin"), fixture.remote);
      state.events.push({ kind: "remote", args, options });
      return spawnSync(command, args, options);
    }
    assert.equal(command, "npm", "unexpected process is never delegated to the real host");
    assert.equal(options.cwd, fixture.sourceDirectory);
    assert.equal(options.env.npm_config_registry, REGISTRY);
    if (args[0] === "config") {
      state.events.push({ kind: "scope", args, options });
      assert.deepEqual(args.slice(0, 3), ["config", "get", "@ckanner:registry"]);
      return ok(`${state.scope}\n`);
    }
    if (args[0] === "run") {
      const kind = args[1] === "test:install" ? "install" : "registry";
      assert.ok(["test:install", "test:registry"].includes(args[1]));
      state.events.push({ kind, args, options });
      if (kind === "install") { state.afterInstall?.(); return state.installResult; }
      state.afterRegistry?.();
      return state.registryResult;
    }
    assert.ok(args.includes(`--registry=${REGISTRY}`));
    assert.equal(args.filter((arg) => arg.startsWith("--scope=")).at(-1), "--scope=@ckanner",
      "registry reads and writes must select the scope whose registry was checked");
    if (args[0] === "view") {
      state.events.push({ kind: "read", args, options });
      if (args[2] === "dist-tags.latest") return json(state.latest.get(args[1]) ?? null);
      assert.equal(args[2], "dist.integrity");
      const entry = entries.find((entry) => `${entry.name}@${entry.version}` === args[1]);
      assert.ok(entry, "must read an exact known version");
      return state.versions.has(entry.name) ? json(state.versions.get(entry.name)) : missing();
    }
    if (args[0] === "publish") {
      const entry = entries.find((entry) => entry.filename === path.basename(args[1]));
      assert.ok(entry, "must upload a known candidate tarball");
      assert.equal(args[1], path.join(fixture.directory, entry.filename));
      state.events.push({ kind: "publish", args, options, name: entry.name });
      if (state.failPublish === entry.name) return { status: 1, stdout: "", stderr: "fixture publish failed" };
      assert.equal(fileIntegrity(args[1]), entry.integrity, "published bytes must be the initially tested candidate");
      state.versions.set(entry.name, entry.integrity);
      state.afterWrite?.("publish", entry);
      return ok();
    }
    if (args[0] === "dist-tag") {
      assert.equal(args[1], "add");
      assert.equal(args[3], "latest");
      const entry = entries.find((entry) => `${entry.name}@${entry.version}` === args[2]);
      assert.ok(entry);
      state.events.push({ kind: "promote", args, options, name: entry.name });
      state.latest.set(entry.name, entry.version);
      state.afterWrite?.("promote", entry);
      return ok();
    }
    throw new Error(`Unexpected npm call: ${args.join(" ")}`);
  };
  state.io = { spawnSync: state.spawnSync, env: testEnvironment(), stdin: { isTTY: true }, stdout: { isTTY: true },
    log: () => {}, now: () => state.elapsed, wait: async (milliseconds) => { state.elapsed += milliseconds; } };
  state.writes = () => state.events.filter((event) => ["publish", "promote"].includes(event.kind));
  return state;
}

function changeRemoteTarget(fixture) {
  const other = git(fixture.sourceDirectory, "commit-tree", fixture.candidate.source.tree, "-m", "a different release");
  git(fixture.sourceDirectory, "push", "--force", "origin", `${other}:refs/tags/v${VERSION}`);
}

function replaceRemoteAnnotation(fixture) {
  git(fixture.sourceDirectory, "tag", "-a", "replacement-annotation", fixture.candidate.source.commit, "-m", "changed release annotation");
  git(fixture.sourceDirectory, "push", "--force", "origin", `refs/tags/replacement-annotation:refs/tags/v${VERSION}`);
}

function replaceCandidate(fixture) {
  repack(fixture, fixture.candidate.packages[0], "different but internally valid candidate");
  fixture.save();
  assert.equal(verifyCandidate(fixture.directory).source.commit, fixture.candidate.source.commit);
}

test("local release CLI resolves a selected source and defaults to that source's candidate directory", (t) => {
  const cwd = temporary(t);
  assert.deepEqual(parseArgs(["--source-directory", "../release source", "--repository-visibility", "private"], cwd), {
    sourceDirectory: path.resolve(cwd, "../release source"), directory: path.resolve(cwd, "../release source/dist-npm"),
    repositoryVisibility: "private",
  });
  assert.deepEqual(parseArgs(["--source-directory", "source", "--directory", "../saved candidate", "--repository-visibility", "public"], cwd), {
    sourceDirectory: path.join(cwd, "source"), directory: path.resolve(cwd, "../saved candidate"), repositoryVisibility: "public",
  });
  assert.equal(parseArgs(["--repository-visibility", "private"], cwd).sourceDirectory, TOOL_ROOT);
  for (const args of [[], ["--repository-visibility", "unknown"], ["--repository-visibility"],
    ["--source-directory"], ["--directory"], ["--allow-dirty"],
    ["--repository-visibility", "private", "--repository-visibility", "public"],
    ["--source-directory", "--repository-visibility", "private"]]) assert.throws(() => parseArgs(args, cwd));
});

test("clean lightweight and annotated source tags complete installation, candidate publication, registry smoke and promotion in order", async (t) => {
  for (const annotated of [false, true]) await t.test(annotated ? "annotated" : "lightweight", async (t) => {
    const source = fixture(t, { annotated });
    const remote = registry(source);
    const original = fs.readFileSync(path.join(source.directory, "candidate.json"));
    const delivered = await releaseLocal(source.options, remote.io);
    assert.equal(delivered.source.commit, source.candidate.source.commit);
    const operations = remote.events.filter((event) => ["install", "publish", "registry", "promote"].includes(event.kind));
    assert.deepEqual(operations.map((event) => event.kind), ["install", ...Array(6).fill("publish"), "registry", ...Array(6).fill("promote")]);
    for (const kind of ["publish", "promote"]) {
      assert.deepEqual(operations.filter((event) => event.kind === kind).map((event) => event.name), PACKAGES.map((entry) => entry.name));
    }
    for (const event of operations.filter((event) => event.kind === "publish")) {
      assert.ok(event.args.includes("--provenance=false"));
      assert.ok(event.args.includes("--ignore-scripts"));
      assert.equal(event.args[event.args.indexOf("--tag") + 1], "candidate-0-28-0");
    }
    assert.equal(operations[0].options.env.FLYWHEEL_NPM_PACKAGES_DIR, source.directory);
    const smoke = operations.find((event) => event.kind === "registry");
    assert.equal(smoke.options.env.FLYWHEEL_NPM_VERSION, VERSION);
    assert.equal(smoke.options.env.FLYWHEEL_NPM_REGISTRY, REGISTRY);
    for (const event of [operations[0], smoke]) {
      assert.ok(event.options.timeout > 0 && event.options.timeout <= 300000);
      assert.deepEqual(event.options.stdio, ["ignore", "inherit", "inherit"]);
    }
    assert.ok([...remote.latest.values()].every((value) => value === VERSION));
    assert.deepEqual(fs.readFileSync(path.join(source.directory, "candidate.json")), original);
    assert.equal(git(source.sourceDirectory, "status", "--porcelain"), "");
  });
});

test("the CLI can release an explicitly saved candidate outside the source checkout", async (t) => {
  const source = fixture(t, { externalDirectory: true });
  const remote = registry(source);
  await main(["--source-directory", source.sourceDirectory, "--directory", source.directory,
    "--repository-visibility", "private"], remote.io);
  assert.equal(remote.writes().length, 12);
  assert.equal(remote.events.find((event) => event.kind === "install").options.env.FLYWHEEL_NPM_PACKAGES_DIR, source.directory);
});

test("dirty source and mismatched HEAD, tree or physical snapshot cannot reach publication", async (t) => {
  const cases = [
    ["tracked edit", (source) => fs.appendFileSync(path.join(source.sourceDirectory, "source.txt"), "unreviewed edit\n")],
    ["untracked file", (source) => fs.writeFileSync(path.join(source.sourceDirectory, "new-source.txt"), "not in the release")],
    ["staged edit", (source) => { fs.appendFileSync(path.join(source.sourceDirectory, "source.txt"), "staged edit\n"); git(source.sourceDirectory, "add", "source.txt"); }],
    ["new clean HEAD", (source) => {
      git(source.sourceDirectory, "commit", "--allow-empty", "-m", "later source HEAD");
      git(source.sourceDirectory, "tag", "-f", `v${VERSION}`);
      git(source.sourceDirectory, "push", "--force", "origin", `refs/tags/v${VERSION}`);
    }],
    ["different tree", (source) => { source.candidate.source.tree = "a".repeat(40); source.save(); }],
    ["different snapshot", (source) => { source.candidate.source.snapshot_sha256 = "a".repeat(64); source.save(); }],
    ["dirty build receipt", (source) => { source.candidate.source.dirty = true; source.save(); }],
    ["working bytes hidden from Git status", (source) => {
      git(source.sourceDirectory, "update-index", "--assume-unchanged", "source.txt");
      fs.appendFileSync(path.join(source.sourceDirectory, "source.txt"), "hidden physical change\n");
      assert.equal(git(source.sourceDirectory, "status", "--porcelain"), "");
    }],
  ];
  for (const [name, mutate] of cases) await t.test(name, async (t) => {
    const source = fixture(t);
    mutate(source);
    const remote = registry(source);
    await assert.rejects(releaseLocal(source.options, remote.io), /clean|HEAD|tree|snapshot|source/i);
    assert.equal(remote.writes().length, 0);
    assert.equal(remote.events.some((event) => event.kind === "install"), false);
  });
});

test("the exact remote release tag must exist and identify the candidate source", async (t) => {
  for (const [name, mutate] of [
    ["missing", (source) => git(source.remote, "update-ref", "-d", `refs/tags/v${VERSION}`)],
    ["different target", changeRemoteTarget],
  ]) await t.test(name, async (t) => {
    const source = fixture(t);
    mutate(source);
    const remote = registry(source);
    await assert.rejects(releaseLocal(source.options, remote.io), /origin|tag|source/i);
    assert.equal(remote.writes().length, 0);
  });
});

test("remote tag parsing rejects ambiguous, unexpected and abnormal responses", () => {
  const head = "1".repeat(40), object = "2".repeat(40), other = "3".repeat(40);
  const ref = `refs/tags/v${VERSION}`;
  const candidate = { version: VERSION, source: { commit: head } };
  const read = (result) => verifyRemoteTag("/fixture/source", candidate, { spawnSync: () => result, env: {} });
  assert.deepEqual(read(ok(`${head}\t${ref}\n`)), { objectSHA: head, commitSHA: head });
  assert.deepEqual(read(ok(`${object}\t${ref}\n${head}\t${ref}^{}\n`)), { objectSHA: object, commitSHA: head });
  for (const text of ["", "not a ref\n", `${head} ${ref}\n`, `${head}\t${ref}\textra\n`,
    `${head}\t${ref}\n${head}\t${ref}\n`, `${head}\t${ref}^{}\n`,
    `${head}\t${ref}-unexpected\n`, `${"x".repeat(40)}\t${ref}\n`,
    `${object}\t${ref}\n${other}\t${ref}^{}\n`, `${head}\t${ref}\n\n${head}\t${ref}^{}\n`,
    `${head}\t${ref}\n${other}\trefs/heads/main\n`]) assert.throws(() => read(ok(text)), /origin|tag/i);
  for (const result of [
    { ...ok(`${head}\t${ref}\n`), status: 2 },
    { ...ok(`${head}\t${ref}\n`), status: null },
    { ...ok(`${head}\t${ref}\n`), signal: "SIGTERM" },
    { ...ok(`${head}\t${ref}\n`), error: Object.assign(new Error("timeout"), { code: "ETIMEDOUT" }) },
  ]) assert.throws(() => read(result), /origin|tag/i);
});

test("remote tag reads are captured, bounded and noninteractive even in a TTY release", () => {
  const candidate = { version: VERSION, source: { commit: "1".repeat(40) } };
  let observed;
  verifyRemoteTag("/fixture/source", candidate, { env: { GIT_TERMINAL_PROMPT: "1" }, spawnSync(command, args, options) {
    observed = { command, args, options };
    return ok(`${candidate.source.commit}\trefs/tags/v${VERSION}\n`);
  } });
  assert.equal(observed.command, "git");
  assert.deepEqual(observed.args.slice(-2), [`refs/tags/v${VERSION}`, `refs/tags/v${VERSION}^{}`]);
  assert.deepEqual(observed.options.stdio, ["ignore", "pipe", "pipe"]);
  assert.equal(observed.options.env.GIT_TERMINAL_PROMPT, "0");
  assert.ok(observed.options.timeout > 0 && observed.options.timeout <= 30000);
  assert.equal(observed.options.killSignal, "SIGKILL");
});

test("failed or cancelled local installation never uploads a package", async (t) => {
  const source = fixture(t);
  for (const failure of [{ status: 1 }, { status: null }, { signal: "SIGTERM" },
    { error: Object.assign(new Error("timeout"), { code: "ETIMEDOUT" }) }]) {
    const remote = registry(source);
    remote.installResult = { ...ok(), ...failure };
    await assert.rejects(releaseLocal(source.options, remote.io), /test:install/);
    assert.equal(remote.writes().length, 0);
    assert.equal(remote.events.some((event) => event.kind === "registry"), false);
  }
});

test("a failed registry installation leaves latest unchanged, and a retry reuses all six uploaded candidates", async (t) => {
  const source = fixture(t);
  const remote = registry(source);
  const original = fs.readFileSync(path.join(source.directory, "candidate.json"));
  remote.registryResult = { status: 1, stdout: "", stderr: "fixture registry installation failed" };
  await assert.rejects(releaseLocal(source.options, remote.io), /test:registry/);
  assert.equal(remote.events.filter((event) => event.kind === "publish").length, 6);
  assert.equal(remote.events.filter((event) => event.kind === "promote").length, 0);
  assert.ok([...remote.latest.values()].every((value) => value === "0.27.0"));
  remote.events.length = 0;
  remote.registryResult = ok();
  await releaseLocal(source.options, remote.io);
  assert.equal(remote.events.filter((event) => event.kind === "publish").length, 0);
  assert.equal(remote.events.filter((event) => event.kind === "promote").length, 6);
  assert.deepEqual(fs.readFileSync(path.join(source.directory, "candidate.json")), original);
});

test("a partial publication retries only missing immutable versions using the same saved candidate", async (t) => {
  const source = fixture(t);
  const remote = registry(source);
  const original = fs.readFileSync(path.join(source.directory, "candidate.json"));
  remote.failPublish = PACKAGES[2].name;
  await assert.rejects(releaseLocal(source.options, remote.io), /publish/);
  assert.equal(remote.versions.size, 2);
  assert.equal(remote.events.some((event) => event.kind === "registry" || event.kind === "promote"), false);
  remote.events.length = 0;
  remote.failPublish = null;
  await releaseLocal(source.options, remote.io);
  assert.deepEqual(remote.events.filter((event) => event.kind === "publish").map((event) => event.name), PACKAGES.slice(2).map((entry) => entry.name));
  assert.equal(remote.versions.size, 6);
  assert.ok([...remote.latest.values()].every((value) => value === VERSION));
  assert.deepEqual(fs.readFileSync(path.join(source.directory, "candidate.json")), original);
});

test("source, candidate and remote tag changes during installation block the first upload", async (t) => {
  for (const [name, mutate] of [
    ["source", (source) => fs.appendFileSync(path.join(source.sourceDirectory, "source.txt"), "changed during install\n")],
    ["valid replacement candidate", replaceCandidate],
    ["remote deletion", (source) => git(source.remote, "update-ref", "-d", `refs/tags/v${VERSION}`)],
    ["remote target", changeRemoteTarget],
    ["same commit, new annotation", replaceRemoteAnnotation],
  ]) await t.test(name, async (t) => {
    const source = fixture(t, { annotated: true });
    const remote = registry(source);
    remote.afterInstall = () => mutate(source);
    await assert.rejects(releaseLocal(source.options, remote.io), /clean|source|candidate|manifest|origin|tag/i);
    assert.equal(remote.writes().length, 0);
  });
});

test("mutating source, candidate or tag after an upload blocks the next write", async (t) => {
  for (const [name, mutate] of [
    ["source", (source) => fs.appendFileSync(path.join(source.sourceDirectory, "source.txt"), "changed after upload\n")],
    ["candidate", replaceCandidate],
    ["tag target", changeRemoteTarget],
    ["tag annotation", replaceRemoteAnnotation],
  ]) await t.test(name, async (t) => {
    const source = fixture(t, { annotated: true });
    const remote = registry(source);
    remote.afterWrite = () => { remote.afterWrite = null; mutate(source); };
    await assert.rejects(releaseLocal(source.options, remote.io), /clean|source|candidate|manifest|origin|tag/i);
    assert.equal(remote.writes().length, 1);
    assert.equal(remote.events.some((event) => event.kind === "registry" || event.kind === "promote"), false);
  });
});

test("source, candidate and tag changes during registry smoke block promotion", async (t) => {
  for (const [name, mutate] of [
    ["source", (source) => fs.appendFileSync(path.join(source.sourceDirectory, "source.txt"), "changed during registry smoke\n")],
    ["candidate", replaceCandidate],
    ["tag", changeRemoteTarget],
  ]) await t.test(name, async (t) => {
    const source = fixture(t);
    const remote = registry(source);
    remote.afterRegistry = () => mutate(source);
    await assert.rejects(releaseLocal(source.options, remote.io), /clean|source|candidate|manifest|origin|tag/i);
    assert.equal(remote.events.filter((event) => event.kind === "publish").length, 6);
    assert.equal(remote.events.filter((event) => event.kind === "promote").length, 0);
    assert.ok([...remote.latest.values()].every((value) => value === "0.27.0"));
  });
});

test("a source change during partial latest promotion stops remaining tags", async (t) => {
  const source = fixture(t);
  const remote = registry(source);
  remote.afterWrite = (kind) => {
    if (kind === "promote") {
      remote.afterWrite = null;
      fs.appendFileSync(path.join(source.sourceDirectory, "source.txt"), "changed during promotion\n");
    }
  };
  await assert.rejects(releaseLocal(source.options, remote.io), /clean|source/i);
  assert.equal(remote.events.filter((event) => event.kind === "promote").length, 1);
  assert.equal(remote.latest.get(META), "0.27.0", "the public entry must not be advanced after a failed dependency promotion");
});

test("scope registry and tarball publishConfig cannot redirect uploads to another registry", async (t) => {
  const source = fixture(t);
  const remote = registry(source);
  remote.scope = "https://registry.example.invalid/";
  await assert.rejects(releaseLocal(source.options, remote.io), /official npm registry/);
  assert.equal(remote.writes().length, 0);
  for (const key of ["registry", "@ckanner:registry"]) await t.test(key, async (t) => {
    const source = fixture(t, { changeManifests(values) {
      values[PLUGIN].publishConfig = { [key]: "https://registry.example.invalid/" };
    } });
    const remote = registry(source);
    await assert.rejects(releaseLocal(source.options, remote.io), /publishConfig.*official npm registry/);
    assert.equal(remote.writes().length, 0);
  });
});

test("a scope registry change after the first upload cannot redirect the next package", async (t) => {
  const source = fixture(t);
  const remote = registry(source);
  remote.afterWrite = () => { remote.scope = "https://registry.example.invalid/"; };
  await assert.rejects(releaseLocal(source.options, remote.io), /official npm registry/);
  assert.equal(remote.writes().length, 1);
});

test("only npm writes inherit the terminal; reads preserve captured output and deadline options", () => {
  const events = [];
  const io = { stdin: { isTTY: true }, stdout: { isTTY: true }, env: { NODE_AUTH_TOKEN: "fixture-only-token" },
    spawnSync(command, args, options) {
      events.push({ kind: "spawn", command, args, options });
      return json("captured-registry-value");
    } };
  const run = createNpmRunner("/fixture/source", io, () => events.push({ kind: "check" }));
  const result = run(["view", `${META}@${VERSION}`, "dist.integrity", "--json"], { timeout: 73, killSignal: "SIGKILL" });
  assert.equal(result.stdout, JSON.stringify("captured-registry-value"));
  assert.equal(events.length, 1);
  assert.deepEqual(events[0].options.stdio, ["ignore", "pipe", "pipe"]);
  assert.equal(events[0].options.timeout, 73);
  assert.equal(events[0].options.killSignal, "SIGKILL");
  assert.equal(events[0].options.env.NODE_AUTH_TOKEN, "fixture-only-token");
  assert.equal(events[0].options.env.npm_config_registry, REGISTRY);
  for (const args of [["publish", "/fixture/package.tgz"], ["dist-tag", "add", `${META}@${VERSION}`, "latest"]]) {
    run(args, { timeout: 91, killSignal: "SIGTERM" });
    assert.equal(events.at(-2).kind, "check");
    const call = events.at(-1);
    assert.equal(call.options.stdio, "inherit");
    assert.equal(call.options.timeout, 91);
    assert.equal(call.options.killSignal, "SIGTERM");
    assert.equal(call.options.cwd, "/fixture/source");
    assert.equal(call.args.at(-1), `--registry=${REGISTRY}`);
  }
});

test("npm release reads and writes override an unrelated default scope without discarding authentication", () => {
  const calls = [];
  const run = createNpmRunner("/fixture/source", {
    stdin: { isTTY: true }, stdout: { isTTY: true },
    env: { npm_config_scope: "@other", NODE_AUTH_TOKEN: "fixture-only-token" },
    spawnSync(command, args, options) {
      calls.push({ command, args, options });
      return ok();
    },
  });
  for (const args of [["view", `${META}@${VERSION}`, "dist.integrity"],
    ["publish", "/fixture/package.tgz"], ["dist-tag", "add", `${META}@${VERSION}`, "latest"]]) {
    run([...args, "--scope=@another"]);
    const call = calls.at(-1);
    assert.equal(call.command, "npm");
    assert.equal(call.args.filter((arg) => arg.startsWith("--scope=")).at(-1), "--scope=@ckanner",
      `${args[0]} must not fall back to another scope's registry`);
    assert.equal(call.args.at(-1), `--registry=${REGISTRY}`);
    assert.equal(call.options.env.NODE_AUTH_TOKEN, "fixture-only-token");
    assert.equal(call.options.env.npm_config_registry, REGISTRY);
  }
  assert.equal(calls.length, 3);
});

test("a failed pre-write checkpoint prevents invoking npm even with a real terminal", () => {
  let spawns = 0;
  const run = createNpmRunner("/fixture/source", {
    stdin: { isTTY: true }, stdout: { isTTY: true }, spawnSync: () => { spawns++; return ok(); },
  }, () => { throw new Error("source changed"); });
  assert.throws(() => run(["publish", "/fixture/package.tgz"]), /source changed/);
  assert.equal(spawns, 0);
});

test("non-TTY local publication stops before installation or any npm request", async (t) => {
  const source = fixture(t);
  for (const [stdin, stdout] of [[false, true], [true, false], [false, false]]) {
    const remote = registry(source);
    await assert.rejects(releaseLocal(source.options, { ...remote.io,
      stdin: { isTTY: stdin }, stdout: { isTTY: stdout } }), /TTY/);
    assert.equal(remote.events.some((event) => event.kind !== "remote"), false);
  }
});

test("the public command reports non-TTY failure without invoking real npm", (t) => {
  const source = fixture(t);
  const result = spawnSync(process.execPath, [path.join(TOOL_ROOT, "scripts/release-local.js"),
    "--source-directory", source.sourceDirectory, "--repository-visibility", "private"], {
    cwd: source.root, env: testEnvironment(), encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], timeout: 15000,
  });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /TTY/);
  assert.doesNotMatch(result.stderr, /Real npm is forbidden/);
});
