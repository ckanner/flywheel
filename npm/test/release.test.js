"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const http = require("node:http");
const { execFileSync, execFile } = require("node:child_process");
const { promisify } = require("node:util");
const { PACKAGES, META, PLUGIN, validatePackages, preflight, parseArgs: parsePreflight } = require("../../scripts/release-preflight.js");
const { filenameFor, fileIntegrity, readPackedFile, verifyCandidate } = require("../../scripts/release-candidate.js");
const { parsePackOutput, snapshot } = require("../../scripts/release-build.js");
const { compareVersions, isNotFound, readValue, publish, promote, verifyLatest, waitForPackage, parseArgs } = require("../../scripts/release-publish.js");

const VERSION = "0.27.0";
const SHA = "a".repeat(40);

function temporary(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "flywheel-release-test-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  return root;
}

function manifests() { return require('./support/unified-package.js').manifests(VERSION); }

function writeManifests(root, values = manifests()) {
  for (const entry of PACKAGES) {
    const directory = path.join(root, entry.directory);
    fs.mkdirSync(directory, { recursive: true });
    fs.writeFileSync(path.join(directory, "package.json"), JSON.stringify(values[entry.name]));
  }
}

function git(root, ...args) {
  return execFileSync("git", ["-c", "user.name=release-test", "-c", "user.email=release-test@example.invalid", ...args],
    { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
}

function packFixture(directory, entry, manifest) {
  const staging = fs.mkdtempSync(path.join(directory, "tar-source-"));
  try {
    fs.mkdirSync(path.join(staging, "package"));
    require('./support/unified-package.js').writeContents(path.join(staging, 'package'), manifest);
    fs.writeFileSync(path.join(staging, "package/content.txt"), `verified content for ${entry.name}`);
    execFileSync("tar", ["-czf", path.join(directory, entry.filename), "-C", staging, "package"], { stdio: "pipe" });
  } finally { fs.rmSync(staging, { recursive: true, force: true }); }
  entry.integrity = fileIntegrity(path.join(directory, entry.filename));
  entry.bytes = fs.statSync(path.join(directory, entry.filename)).size;
}

function candidateFixture(t) {
  const directory = temporary(t);
  const values = manifests();
  const candidate = { format_version: 2, version: VERSION,
    source: { commit: SHA, tree: "b".repeat(40), dirty: false, snapshot_sha256: "c".repeat(64) },
    packages: PACKAGES.map((entry) => ({ name: entry.name, version: VERSION, filename: filenameFor(entry.name, VERSION) })) };
  for (const entry of candidate.packages) packFixture(directory, entry, values[entry.name]);
  const save = () => fs.writeFileSync(path.join(directory, "candidate.json"), JSON.stringify(candidate));
  save();
  return { directory, candidate, values, save };
}

const ok = (value) => ({ status: 0, stdout: value === undefined ? "" : JSON.stringify(value), stderr: "" });
const missing = () => ({ status: 1, stdout: JSON.stringify({ error: { code: "E404", summary: "Not found" } }), stderr: "" });

function registry(candidate, { populated = false } = {}) {
  const versions = new Map(populated ? candidate.packages.map((entry) => [entry.name, entry.integrity]) : []);
  const latest = new Map(candidate.packages.map((entry) => [entry.name, "0.22.1"]));
  const calls = [];
  const state = { versions, latest, calls, override: null, elapsed: 0 };
  state.runNpm = (args) => {
    calls.push(args);
    const overridden = state.override?.(args);
    if (overridden) return overridden;
    if (args[0] === "view") {
      if (args[2] === "dist-tags.latest") return ok(latest.get(args[1]) ?? null);
      const name = args[1].slice(0, args[1].lastIndexOf("@"));
      return versions.has(name) ? ok(versions.get(name)) : missing();
    }
    if (args[0] === "publish") {
      const entry = candidate.packages.find((entry) => entry.filename === path.basename(args[1]));
      assert.ok(entry, "publisher must use a known candidate file");
      versions.set(entry.name, entry.integrity);
      return ok();
    }
    if (args[0] === "dist-tag") {
      const split = args[2].lastIndexOf("@");
      latest.set(args[2].slice(0, split), args[2].slice(split + 1));
      return ok();
    }
    throw new Error(`Unexpected npm call: ${args.join(" ")}`);
  };
  state.options = { runNpm: state.runNpm, now: () => state.elapsed,
    wait: async (milliseconds) => { state.elapsed += milliseconds; }, log: () => {}, repositoryVisibility: "public" };
  return state;
}

test("one package must own the native platforms, plugin exports and pinned adapters", () => {
  assert.equal(validatePackages(manifests()), VERSION);
  for (const mutate of [
    (values) => { values[PLUGIN].version = "0.1.0"; },
    (values) => { values[META].dependencies[PLUGIN] = "^0.27.0"; },
    (values) => { values[PLUGIN].dependencies['dsh-subagent-codex-pro'] = '^0.10.2'; },
    (values) => { delete values[PLUGIN].bundledDependencies; },
    (values) => { values[PLUGIN].bundledDependencies.push('unreviewed-package'); },
    (values) => { delete values[PLUGIN].exports['./codex-pro-worker']; },
    (values) => { delete values[META].bin['flywheel-codex-pro-worker']; },
    (values) => { delete values[PLUGIN].exports['./model-catalog-worker']; },
    (values) => { delete values[META].bin['flywheel-model-catalog']; },
    (values) => { values[PLUGIN].peerDependencies['dsh-subagent-claude-code-pro'] = '^0.6.0'; },
    (values) => { values[PLUGIN].peerDependenciesMeta['dsh-subagent-claude-code-pro'].optional = false; },
    (values) => { values[PLUGIN].dependencies['dsh-subagent-claude-code-pro'] = '0.6.0'; },
    (values) => { delete values[PLUGIN].exports['./claude-code-pro-worker']; },
    (values) => { delete values[META].bin['flywheel-claude-code-pro-worker']; },
    (values) => { values[META].optionalDependencies = { "@ckanner/flywheel-darwin-arm64": "latest" }; },
    (values) => { values[PACKAGES[0].name].cpu = ["x64"]; },
    (values) => { delete values[PLUGIN].repository; },
    (values) => { delete values[PLUGIN]; },
  ]) { const values = manifests(); mutate(values); assert.throws(() => validatePackages(values, VERSION)); }
  for (const version of ["0.22.1", "01.2.3", "0.27.0-beta.1", "latest", "0.27.0;echo bad"]) {
    assert.throws(() => validatePackages(manifests(), version));
  }
});

test("preflight accepts only the exact clean HEAD tag, not an ancestor tag", (t) => {
  const root = temporary(t);
  writeManifests(root);
  git(root, "init"); git(root, "add", "."); git(root, "commit", "-m", "fixture"); git(root, "tag", `v${VERSION}`);
  assert.equal(preflight(root, { tag: `v${VERSION}` }).source_sha, git(root, "rev-parse", "HEAD"));
  fs.writeFileSync(path.join(root, "untracked.txt"), "not release source");
  assert.throws(() => preflight(root), /clean/);
  fs.unlinkSync(path.join(root, "untracked.txt"));
  git(root, "commit", "--allow-empty", "-m", "later HEAD");
  assert.throws(() => preflight(root, { tag: `v${VERSION}` }), /exact current HEAD/);
  assert.throws(() => preflight(root));
  assert.equal(preflight(root, { packagesOnly: true }).version, VERSION);
});

test("source snapshots preserve modes and symlinks without copying ignored files or editing source", (t) => {
  const root = temporary(t);
  git(root, "init");
  fs.writeFileSync(path.join(root, ".gitignore"), "ignored/\n");
  fs.mkdirSync(path.join(root, "ignored")); fs.writeFileSync(path.join(root, "ignored/token"), "must not be copied");
  fs.writeFileSync(path.join(root, "executable"), "#!/bin/sh\n", { mode: 0o755 });
  fs.symlinkSync("executable", path.join(root, "alias"));
  const destination = temporary(t);
  const first = snapshot(root, destination);
  assert.equal(snapshot(root), first);
  assert.equal(fs.readlinkSync(path.join(destination, "alias")), "executable");
  assert.ok(fs.statSync(path.join(destination, "executable")).mode & 0o111);
  assert.equal(fs.existsSync(path.join(destination, "ignored")), false);
  fs.writeFileSync(path.join(root, "executable"), "changed");
  assert.notEqual(snapshot(root), first);
  assert.equal(fs.readFileSync(path.join(destination, "executable"), "utf8"), "#!/bin/sh\n");
});

test("npm pack accepts the npm 11 array and npm 12 name-keyed single-package protocols", () => {
  const packed = { name: META, version: VERSION, filename: filenameFor(META, VERSION) };
  for (const result of [[packed], { [META]: packed }]) {
    assert.deepEqual(parsePackOutput(JSON.stringify(result), META, VERSION), packed);
  }
});

test("npm pack rejects malformed output, ambiguous package sets and wrong map keys", () => {
  const packed = { name: META, version: VERSION, filename: filenameFor(META, VERSION) };
  for (const result of [null, true, 1, "package.tgz", [], {}, packed, [null], [packed, packed],
    { wrong: packed }, { [META]: packed, extra: packed }, { [META]: null }, { [META]: [packed] }]) {
    assert.throws(() => parsePackOutput(JSON.stringify(result), META, VERSION), /Unexpected npm pack output/);
  }
  for (const raw of ["", "not JSON", JSON.stringify([packed]) + "\n{}"]) {
    assert.throws(() => parsePackOutput(raw, META, VERSION), /invalid JSON/);
  }
});

test("npm pack requires the exact name, version and basename in either protocol", () => {
  const filename = filenameFor(META, VERSION);
  for (const change of [{ name: "@fixture/wrong-package" }, { name: undefined }, { version: "0.1.0" }, { version: undefined },
    { filename: undefined }, { filename: `../${filename}` }, { filename: `./${filename}` },
    { filename: `/tmp/${filename}` }, { filename: `${filename}.extra` }]) {
    const packed = { name: META, version: VERSION, filename, ...change };
    for (const result of [[packed], { [META]: packed }]) {
      assert.throws(() => parsePackOutput(JSON.stringify(result), META, VERSION), /Unexpected npm pack output/);
    }
  }
});

test("real offline npm pack output identifies the artifact whose bytes are verified", (t) => {
  const root = temporary(t);
  const source = path.join(root, "source"), destination = path.join(root, "packages");
  fs.mkdirSync(source); fs.mkdirSync(destination);
  const name = "@flywheel-release-test/pack";
  fs.writeFileSync(path.join(source, "package.json"), JSON.stringify({ name, version: VERSION, files: ["content.txt"] }));
  fs.writeFileSync(path.join(source, "content.txt"), "local npm pack protocol fixture\n");
  const config = path.join(root, "npmrc"), globalConfig = path.join(root, "global-npmrc");
  fs.writeFileSync(config, ""); fs.writeFileSync(globalConfig, "");
  const raw = execFileSync("npm", ["pack", "--ignore-scripts", "--offline", "--json", "--pack-destination", destination,
    "--userconfig", config, "--globalconfig", globalConfig, "--cache", path.join(root, "cache")], {
    cwd: source, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], timeout: 15000,
  });
  const packed = parsePackOutput(raw, name, VERSION);
  const file = path.join(destination, packed.filename);
  assert.deepEqual(fs.readdirSync(destination), [filenameFor(name, VERSION)]);
  assert.equal(packed.size, fs.statSync(file).size);
  assert.equal(packed.integrity, fileIntegrity(file));
});

test("both package file lists ship the provenance records linked by their vendor index", (t) => {
  const root = temporary(t), repository = path.resolve(__dirname, '../..');
  const vendor = path.join(repository, 'plugins/dsh/vendor');
  const index = fs.readFileSync(path.join(vendor, 'README.md'), 'utf8');
  const records = [...index.matchAll(/\]\(([\w.-]+\.json)\)/g)].map(match => match[1]);
  assert.ok(records.includes('antigravity.json'));
  assert.ok(records.includes('registry-verification-2026-11-22.json'));
  for (const relative of ['', 'plugins/dsh']) {
    const source = path.join(root, relative || 'public'), destination = path.join(source, 'packed');
    const prefix = relative ? 'vendor' : 'plugins/dsh/vendor';
    fs.mkdirSync(path.join(source, prefix), { recursive: true });
    fs.mkdirSync(destination);
    const manifest = JSON.parse(fs.readFileSync(path.join(repository, relative, 'package.json'), 'utf8'));
    fs.writeFileSync(path.join(source, 'package.json'), JSON.stringify(manifest));
    for (const name of ['README.md', ...records]) fs.copyFileSync(path.join(vendor, name), path.join(source, prefix, name));
    const config = path.join(source, 'npmrc'), globalConfig = path.join(source, 'global-npmrc');
    fs.writeFileSync(config, ''); fs.writeFileSync(globalConfig, '');
    const raw = execFileSync('npm', ['pack', '--ignore-scripts', '--offline', '--json', '--pack-destination', destination,
      '--userconfig', config, '--globalconfig', globalConfig, '--cache', path.join(root, 'cache')], {
      cwd: source, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], timeout: 15000,
    });
    const packed = parsePackOutput(raw, manifest.name, manifest.version);
    for (const name of ['README.md', ...records]) {
      assert.deepEqual(readPackedFile(path.join(destination, packed.filename), `${prefix}/${name}`), fs.readFileSync(path.join(vendor, name)),
        `${manifest.name} must retain the exact linked provenance record ${name}`);
    }
  }
});

test("candidate verifies actual tar manifests, exact source and clean source policy", (t) => {
  const { directory, candidate, save } = candidateFixture(t);
  assert.equal(verifyCandidate(directory, { sourceSHA: SHA }).version, VERSION);
  assert.throws(() => verifyCandidate(directory, { sourceSHA: "d".repeat(40) }), /source SHA/);
  candidate.source.dirty = true; save();
  assert.throws(() => verifyCandidate(directory), /dirty source/);
  assert.equal(verifyCandidate(directory, { allowDirty: true }).version, VERSION);
  candidate.source.dirty = false; candidate.packages[1] = candidate.packages[0]; save();
  assert.throws(() => verifyCandidate(directory), /exactly one/);
});

test("changed tar bytes and symlink substitution cannot pass candidate verification", (t) => {
  const fixture = candidateFixture(t);
  const file = path.join(fixture.directory, fixture.candidate.packages[0].filename);
  const original = fs.readFileSync(file);
  fs.appendFileSync(file, "unexpected");
  assert.throws(() => verifyCandidate(fixture.directory), /bytes\/integrity/);
  fs.unlinkSync(file);
  const elsewhere = path.join(fixture.directory, "same-bytes"); fs.writeFileSync(elsewhere, original); fs.symlinkSync(elsewhere, file);
  assert.throws(() => verifyCandidate(fixture.directory), /bytes\/integrity/);
});

test("rehashed tarball with a wrong embedded package version is still rejected", (t) => {
  const { directory, candidate, values, save } = candidateFixture(t);
  const entry = candidate.packages[0];
  packFixture(directory, entry, { ...values[entry.name], version: "0.1.0" }); save();
  assert.throws(() => verifyCandidate(directory), /requested version/);
});

test("public candidate verification CLI returns failure for a mismatched source", (t) => {
  const { directory } = candidateFixture(t);
  const script = path.resolve(__dirname, "../../scripts/release-publish.js");
  assert.match(execFileSync(process.execPath, [script, "verify-candidate", "--directory", directory, "--source-sha", SHA], { encoding: "utf8" }), /verified 0\.27\.0/);
  assert.throws(() => execFileSync(process.execPath, [script, "verify-candidate", "--directory", directory, "--source-sha", "f".repeat(40)], { stdio: "pipe" }));
});

test("npm absence is only structured E404; authentication, network and cancellation fail closed", () => {
  assert.equal(isNotFound(missing()), true);
  for (const code of ["E401", "E403", "ETIMEDOUT", "ECONNRESET", "ETARGET"]) {
    const result = { status: 1, stdout: JSON.stringify({ error: { code } }), stderr: "registry URL contains 404" };
    assert.equal(isNotFound(result), false);
    assert.throws(() => readValue([META, "dist-tags.latest"], () => result));
  }
  assert.equal(isNotFound({ ...missing(), signal: "SIGTERM" }), false);
  assert.equal(isNotFound({ ...missing(), error: new Error("spawn failed") }), false);
  for (const status of [null, undefined, -1, 129, 130, 131, 137, 143]) {
    const result = { ...missing(), status };
    assert.equal(isNotFound(result), false);
    assert.throws(() => readValue([META], () => result));
  }
  assert.throws(() => readValue([META], () => ({ ...ok(null), error: new Error("spawn failed") })), /spawn failed/);
  assert.throws(() => readValue([META], () => ({ ...ok(null), signal: "SIGTERM" })), /SIGTERM/);
  assert.equal(isNotFound({ status: 1, stdout: "404 not found", stderr: "" }), false);
  assert.throws(() => readValue([META], () => ok({ error: { code: "E404" } })), /Unexpected response/);
});

test("npm view accepts legacy scalars and npm 12 single-scalar arrays", () => {
  for (const value of [VERSION, `sha512-${Buffer.alloc(64).toString("base64")}`]) {
    for (const response of [value, [value]]) assert.equal(readValue([META], () => ok(response)), value);
  }
});

test("npm view rejects ambiguous arrays, nesting and non-scalar fields", () => {
  for (const value of [[VERSION, VERSION], [[VERSION]], [true], [1], [{}], true, 1, {}]) {
    for (const allowMissingValue of [false, true]) {
      assert.throws(() => readValue([META], () => ok(value), { allowMissingValue }), /Unexpected response/);
    }
  }
});

test("npm view missing values are allowed only for explicitly optional fields", () => {
  for (const value of [undefined, null, [], [null]]) {
    assert.throws(() => readValue([META], () => ok(value)), /Missing registry value/);
    assert.equal(readValue([META], () => ok(value), { allowMissingValue: true }), null);
  }
});

test("a successful empty integrity response never permits publication, while a missing latest tag is allowed", async (t) => {
  const { directory, candidate } = candidateFixture(t);
  for (const value of [undefined, null, "", [], [null], [""]]) {
    const remote = registry(candidate);
    remote.override = (args) => args[0] === "view" && args[2] === "dist.integrity" ? ok(value) : undefined;
    await assert.rejects(publish(candidate, directory, remote.options), /Missing registry value|Invalid registry integrity/);
    assert.equal(remote.calls.filter((args) => args[0] !== "view").length, 0);
  }
  for (const value of [undefined, null, [], [null]]) {
    const remote = registry(candidate);
    remote.override = (args) => args[0] === "view" && args[2] === "dist-tags.latest" ? ok(value) : undefined;
    await publish(candidate, directory, remote.options);
    assert.equal(remote.calls.filter((args) => args[0] === "publish").length, 1);
  }
});

test("real npm view preserves scalar, missing-field and registry-error semantics", async (t) => {
  const root = temporary(t);
  const config = path.join(root, "npmrc"); fs.writeFileSync(config, "");
  const globalConfig = path.join(root, "global-npmrc"); fs.writeFileSync(globalConfig, "");
  const integrity = `sha512-${Buffer.alloc(64).toString("base64")}`;
  const requests = [];
  const server = http.createServer((req, res) => {
    requests.push(req.method);
    const name = decodeURIComponent(req.url.slice(1));
    const forbidden = name.endsWith("/forbidden"), missing = name.endsWith("/missing-package");
    if (forbidden || missing) {
      res.writeHead(forbidden ? 403 : 404, { "content-type": "application/json" });
      res.end(JSON.stringify({ error: forbidden ? "forbidden" : "not found" }));
      return;
    }
    const tags = name.endsWith("/missing-latest") ? { "candidate-0-27-0": VERSION } : { latest: VERSION };
    const dist = name.endsWith("/missing-integrity") ? {} : { integrity };
    res.writeHead(200, { "content-type": "application/json" });
    res.end(JSON.stringify({ name, "dist-tags": tags, versions: { [VERSION]: { name, version: VERSION, dist } } }));
  });
  await new Promise((resolve, reject) => { server.once("error", reject); server.listen(0, "127.0.0.1", resolve); });
  t.after(() => new Promise((resolve) => server.close(resolve)));
  for (const [name, field, expected, error, absent] of [
    ["complete", "dist-tags.latest", VERSION],
    ["complete", "dist.integrity", integrity],
    ["missing-latest", "dist-tags.latest", null],
    ["missing-integrity", "dist.integrity", undefined, /Missing registry value/],
    ["missing-package", "dist.integrity", null, undefined, true],
    ["forbidden", "dist.integrity", undefined, /npm view/],
  ]) {
    const spec = `@flywheel-release-test/${name}${field === "dist.integrity" ? `@${VERSION}` : ""}`;
    let result;
    try {
      const output = await promisify(execFile)("npm", ["view", spec, field, "--json", "--prefer-online",
        "--registry", `http://127.0.0.1:${server.address().port}`, "--userconfig", config, "--globalconfig", globalConfig,
        "--cache", path.join(root, "cache"), "--fetch-retries", "0"], { cwd: root, timeout: 15000 });
      result = { status: 0, ...output };
    } catch (error) { result = { status: error.code, stdout: error.stdout, stderr: error.stderr, signal: error.signal }; }
    assert.equal(isNotFound(result), absent === true, JSON.stringify(result));
    const read = () => readValue([spec, field], () => result, { allowMissingValue: field === "dist-tags.latest" });
    if (error) assert.throws(read, error);
    else assert.equal(read(), expected);
  }
  assert.ok(requests.length >= 6);
  assert.ok(requests.every((method) => method === "GET"), "registry protocol tests are read-only");
});

test("publishing uses exact dependencies first and never advances latest; retries verify existing bytes", async (t) => {
  const { directory, candidate } = candidateFixture(t);
  const remote = registry(candidate);
  await publish(candidate, directory, remote.options);
  const writes = remote.calls.filter((args) => args[0] === "publish");
  assert.deepEqual(writes.map((args) => path.basename(args[1])), candidate.packages.map((entry) => entry.filename));
  for (const args of writes) {
    assert.ok(args.includes("--ignore-scripts")); assert.ok(args.includes("--provenance"));
    assert.equal(args[args.indexOf("--tag") + 1], "candidate-0-27-0");
  }
  assert.ok([...remote.latest.values()].every((value) => value === "0.22.1"));
  remote.calls.length = 0;
  await publish(candidate, directory, remote.options);
  assert.equal(remote.calls.filter((args) => args[0] !== "view").length, 0);
});

test("npm 12 scalar arrays preserve exact-version verification and latest promotion", async (t) => {
  const { directory, candidate } = candidateFixture(t);
  const remote = registry(candidate);
  const runNpm = remote.runNpm;
  remote.options.runNpm = (args) => {
    const result = runNpm(args);
    return args[0] === "view" && result.status === 0 ? ok([JSON.parse(result.stdout)]) : result;
  };
  await publish(candidate, directory, remote.options);
  assert.ok([...remote.latest.values()].every((value) => value === "0.22.1"));
  await promote(candidate, remote.options);
  await verifyLatest(candidate, remote.options);
  assert.deepEqual(remote.calls.filter((args) => args[0] === "dist-tag").map((args) => args[2]),
    candidate.packages.map((entry) => `${entry.name}@${VERSION}`));
});

test("a conflicting existing version or newer latest blocks every publish before mutation", async (t) => {
  const { directory, candidate } = candidateFixture(t);
  for (const conflict of ["integrity", "newer"]) {
    const remote = registry(candidate);
    if (conflict === "integrity") remote.versions.set(META, `sha512-${Buffer.alloc(64, 1).toString("base64")}`);
    else remote.latest.set(META, "0.28.0");
    await assert.rejects(publish(candidate, directory, remote.options), conflict === "integrity" ? /integrity differs/ : /backwards/);
    assert.equal(remote.calls.filter((args) => args[0] !== "view").length, 0);
  }
});

test("private repository publication disables automatic provenance without changing candidate bytes or latest", async (t) => {
  const { directory, candidate } = candidateFixture(t);
  const remote = registry(candidate);
  remote.override = (args) => {
    if (args[0] === "publish" && !args.includes("--provenance=false")) {
      return { status: 1, stdout: "", stderr: "provenance requires a public source repository" };
    }
  };
  await publish(candidate, directory, { ...remote.options, repositoryVisibility: "private" });
  const writes = remote.calls.filter((args) => args[0] === "publish");
  assert.equal(writes.length, PACKAGES.length);
  for (const entry of candidate.packages) assert.equal(remote.versions.get(entry.name), entry.integrity);
  assert.ok([...remote.latest.values()].every((value) => value === "0.22.1"));
  // The explicit command-line flag must override even an enabled npm default.
  const flags = writes[0].filter((arg) => arg.startsWith("--provenance"));
  const actual = execFileSync("npm", ["config", "get", "provenance", ...flags], {
    encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], env: { ...process.env, NPM_CONFIG_PROVENANCE: "true" },
  }).trim();
  assert.equal(actual, "false");
});

test("unknown repository visibility fails before any registry request", async (t) => {
  const { directory, candidate } = candidateFixture(t);
  for (const visibility of [undefined, "", "unknown", "PRIVATE", true]) {
    const remote = registry(candidate);
    await assert.rejects(publish(candidate, directory, { ...remote.options, repositoryVisibility: visibility }), /explicit repository visibility/);
    assert.equal(remote.calls.length, 0);
  }
});

test("an uncertain publish response succeeds only when registry bytes match", async (t) => {
  const { directory, candidate } = candidateFixture(t);
  const remote = registry(candidate);
  remote.override = (args) => {
    if (args[0] !== "publish") return;
    const entry = candidate.packages.find((entry) => entry.filename === path.basename(args[1]));
    remote.versions.set(entry.name, entry.integrity);
    return { status: 1, stdout: "", stderr: "connection reset after response" };
  };
  await publish(candidate, directory, remote.options);
  assert.equal(remote.versions.size, 1);
});

test("failed publish with no registry version stops before later packages", async (t) => {
  const { directory, candidate } = candidateFixture(t);
  const remote = registry(candidate);
  remote.override = (args) => args[0] === "publish" ? { status: 1, stdout: "", stderr: "authentication failed" } : undefined;
  await assert.rejects(publish(candidate, directory, remote.options), /authentication failed/);
  assert.equal(remote.calls.filter((args) => args[0] === "publish").length, 1);
  assert.equal(remote.versions.size, 0);
});

test("publication cancellation is a failure even if registry accepted the bytes", async (t) => {
  const { directory, candidate } = candidateFixture(t);
  for (const termination of [{ signal: "SIGTERM" }, { error: new Error("spawn failed") },
    { status: null }, { status: 130 }, { status: 143 }]) {
    const remote = registry(candidate);
    remote.override = (args) => {
      if (args[0] !== "publish") return;
      const entry = candidate.packages.find((entry) => entry.filename === path.basename(args[1]));
      remote.versions.set(entry.name, entry.integrity);
      return { ...ok(), ...termination };
    };
    await assert.rejects(publish(candidate, directory, remote.options), /SIGTERM|spawn failed|exit null|exit 130|exit 143/);
    assert.equal(remote.calls.filter((args) => args[0] === "publish").length, 1);
  }
});

test("registry visibility accepts a matching package that appears after the former ten-second window", async (t) => {
  const { candidate } = candidateFixture(t);
  const entry = candidate.packages[0];
  let elapsed = 0; let reads = 0;
  const waits = [];
  await waitForPackage(entry, {
    now: () => elapsed,
    runNpm: () => { reads++; elapsed += 300; return elapsed >= 90000 ? ok(entry.integrity) : missing(); },
    wait: async (milliseconds) => { waits.push(milliseconds); elapsed += milliseconds; },
  });
  assert.ok(elapsed >= 90000 && elapsed < 180000);
  assert.ok(reads > 6, "a successful upload must survive more than the former six reads");
  assert.ok(waits.every((milliseconds) => milliseconds > 0 && milliseconds <= 15000));
});

test("registry visibility has one deadline including query time and never starts an unbounded query", async (t) => {
  const { candidate } = candidateFixture(t);
  let elapsed = 0; let queryTime = 0; let waitTime = 0;
  const queries = [];
  await assert.rejects(waitForPackage(candidate.packages[0], {
    now: () => elapsed,
    runNpm: (args, options) => {
      queries.push({ at: elapsed, ...options });
      assert.equal(args[0], "view");
      assert.ok(Number.isInteger(options.timeout) && options.timeout > 0);
      assert.ok(options.timeout <= Math.min(30000, 360000 - elapsed));
      const duration = Math.min(24000, options.timeout);
      queryTime += duration; elapsed += duration;
      return missing();
    },
    wait: async (milliseconds) => {
      assert.ok(milliseconds > 0 && milliseconds <= Math.min(15000, 360000 - elapsed));
      waitTime += milliseconds; elapsed += milliseconds;
    },
  }), /not visible within 360s/);
  assert.equal(elapsed, 360000);
  assert.equal(queryTime + waitTime, 360000);
  assert.ok(queryTime > 0 && waitTime > 0);
  assert.ok(queries.at(-1).timeout < 30000, "the last query is limited by the remaining budget");
  assert.ok(queries.every((query) => query.at < 360000 && query.killSignal === "SIGKILL"));
});

test("registry visibility cannot succeed or start another query after the deadline", async (t) => {
  const { candidate } = candidateFixture(t);
  const entry = candidate.packages[0];
  for (const overrun of ["query", "wait"]) {
    let elapsed = 0; let reads = 0;
    await assert.rejects(waitForPackage(entry, {
      now: () => elapsed,
      runNpm: () => {
        reads++;
        if (overrun === "query") { elapsed = 360001; return ok(entry.integrity); }
        return missing();
      },
      wait: async () => { elapsed = 360001; },
    }), /not visible within 360s/);
    assert.equal(reads, 1, overrun);
  }
});

test("registry visibility retries only structured E404 and stops immediately on other errors", async (t) => {
  const { candidate } = candidateFixture(t);
  for (const [result, error] of [
    [{ status: 1, stdout: JSON.stringify({ error: { code: "E403" } }), stderr: "access denied" }, /access denied/],
    [{ status: 1, stdout: "", stderr: "ECONNRESET" }, /ECONNRESET/],
    [{ ...missing(), signal: "SIGTERM" }, /SIGTERM/],
    [{ ...missing(), error: new Error("ETIMEDOUT") }, /ETIMEDOUT/],
    [{ status: 130, stdout: "", stderr: "" }, /exit 130/],
    [{ status: 143, stdout: "", stderr: "" }, /exit 143/],
    [{ status: null, stdout: "", stderr: "" }, /exit null/],
    [{ status: 0, stdout: "{", stderr: "" }, /Invalid JSON/],
    [ok(null), /Missing registry value/],
    [ok(`sha512-${Buffer.alloc(64, 1).toString("base64")}`), /integrity differs/],
  ]) {
    let elapsed = 0; let reads = 0; let waits = 0;
    await assert.rejects(waitForPackage(candidate.packages[0], {
      now: () => elapsed,
      runNpm: () => ++reads === 1 ? missing() : result,
      wait: async (milliseconds) => { waits++; elapsed += milliseconds; },
    }), error);
    assert.equal(reads, 2);
    assert.equal(waits, 1);
  }
});

test("registry query timeout terminates the real npm process even when it ignores SIGTERM", async (t) => {
  const root = temporary(t);
  const pidFile = path.join(root, "npm.pid");
  const termFile = path.join(root, "received-term");
  const expiredFile = path.join(root, "query-survived");
  // The query has a one-second deadline. Use a native single-process fixture
  // so Node's cold startup under concurrent builds cannot consume that deadline
  // before the readiness marker and signal handler have been installed.
  fs.writeFileSync(path.join(root, "npm"), `#!/bin/bash
trap 'printf ignored > "$FLYWHEEL_QUERY_TERM"' TERM
printf '%s' "$$" > "$FLYWHEEL_QUERY_PID"
until (( SECONDS >= 3 )); do :; done
printf survived > "$FLYWHEEL_QUERY_EXPIRED"
exit 91
`, { mode: 0o755 });
  const script = path.resolve(__dirname, "../../scripts/release-publish.js");
  const source = `const { waitForPackage } = require(${JSON.stringify(script)});
let ticks = 0;
waitForPackage({ name: "@fixture/query", version: "1.0.0" }, {
  now: () => ticks++ === 0 ? 0 : 359000,
}).then(() => { process.exitCode = 1; }, (error) => console.log(JSON.stringify({ error: error.message })));
`;
  const { stdout } = await promisify(execFile)(process.execPath, ["-e", source], {
    env: { ...process.env, PATH: `${root}${path.delimiter}${process.env.PATH}`,
      FLYWHEEL_QUERY_PID: pidFile, FLYWHEEL_QUERY_TERM: termFile, FLYWHEEL_QUERY_EXPIRED: expiredFile }, timeout: 6000,
  });
  assert.match(JSON.parse(stdout).error, /ETIMEDOUT/);
  assert.equal(fs.existsSync(termFile), false, "the query must not rely on cooperative SIGTERM cleanup");
  assert.equal(fs.existsSync(expiredFile), false, "the query must be terminated before its own exit timer");
  const pid = Number(fs.readFileSync(pidFile, "utf8"));
  assert.ok(Number.isInteger(pid) && pid > 0);
  assert.throws(() => process.kill(pid, 0), { code: "ESRCH" });
});

test("a visibility timeout keeps the same candidate reusable without republishing accepted bytes", async (t) => {
  const { directory, candidate } = candidateFixture(t);
  const candidateBytes = fs.readFileSync(path.join(directory, "candidate.json"));
  const remote = registry(candidate);
  const first = candidate.packages[0];
  remote.override = (args) => args[0] === "view" && args[1] === `${first.name}@${first.version}` ? missing() : undefined;
  await assert.rejects(publish(candidate, directory, remote.options), /not visible within 360s/);
  assert.equal(remote.elapsed, 360000);
  assert.deepEqual(remote.calls.filter((args) => args[0] === "publish").map((args) => path.basename(args[1])), [first.filename]);
  assert.equal(remote.versions.get(first.name), first.integrity, "upload succeeded even though the view endpoint lagged");
  assert.ok([...remote.latest.values()].every((value) => value === "0.22.1"));

  remote.override = null;
  remote.calls.length = 0;
  await publish(candidate, directory, remote.options);
  assert.deepEqual(remote.calls.filter((args) => args[0] === "publish").map((args) => args[1]),
    candidate.packages.slice(1).map((entry) => path.join(directory, entry.filename)));
  assert.equal(remote.calls.some((args) => args[0] === "dist-tag"), false);
  assert.deepEqual(fs.readFileSync(path.join(directory, "candidate.json")), candidateBytes);
  for (const entry of candidate.packages) assert.equal(remote.versions.get(entry.name), fileIntegrity(path.join(directory, entry.filename)));
});

test("a local candidate changed after registry preflight is not published", async (t) => {
  const { directory, candidate } = candidateFixture(t);
  const remote = registry(candidate); let latestReads = 0;
  remote.override = (args) => {
    if (args[0] === "view" && args[2] === "dist-tags.latest" && ++latestReads === PACKAGES.length + 1) fs.appendFileSync(path.join(directory, candidate.packages[0].filename), "changed");
  };
  await assert.rejects(publish(candidate, directory, remote.options), /Candidate changed/);
  assert.equal(remote.calls.filter((args) => args[0] === "publish").length, 0);
});

test("latest promotion requires the verified complete package", async (t) => {
  const { candidate } = candidateFixture(t);
  const incomplete = registry(candidate);
  await assert.rejects(promote(candidate, incomplete.options), /not visible/);
  assert.equal(incomplete.calls.filter((args) => args[0] === "dist-tag").length, 0);
  const remote = registry(candidate, { populated: true });
  await promote(candidate, remote.options);
  assert.deepEqual(remote.calls.filter((args) => args[0] === "dist-tag").map((args) => args[2]), candidate.packages.map((entry) => `${entry.name}@${VERSION}`));
  await verifyLatest(candidate, remote.options);
  remote.calls.length = 0; await promote(candidate, remote.options);
  assert.equal(remote.calls.filter((args) => args[0] === "dist-tag").length, 0);
});

test("promotion waits for old latest responses to converge without repeating dist-tag mutations", async (t) => {
  const { candidate } = candidateFixture(t);
  const remote = registry(candidate, { populated: true });
  remote.override = (args) => args[0] === "view" && args[2] === "dist-tags.latest" &&
    remote.latest.get(args[1]) === VERSION && remote.elapsed < 90000 ? ok("0.22.1") : undefined;
  await promote(candidate, remote.options);
  assert.ok(remote.elapsed >= 90000 && remote.elapsed < 180000);
  assert.deepEqual(remote.calls.filter((args) => args[0] === "dist-tag").map((args) => args[2]),
    candidate.packages.map((entry) => `${entry.name}@${VERSION}`));
  assert.equal(remote.calls.some((args) => args[0] === "publish"), false);
  assert.ok([...remote.latest.values()].every((value) => value === VERSION));
});

test("latest verification tolerates a missing tag or temporarily absent target metadata", async (t) => {
  const { candidate } = candidateFixture(t);
  const first = candidate.packages[0];
  for (const [field, response] of [["dist-tags.latest", ok(null)], ["dist-tags.latest", missing()], ["dist.integrity", missing()]]) {
    const remote = registry(candidate, { populated: true });
    for (const entry of candidate.packages) remote.latest.set(entry.name, VERSION);
    remote.override = (args) => args[0] === "view" && args[2] === field &&
      [first.name, `${first.name}@${VERSION}`].includes(args[1]) && remote.elapsed < 5000 ? response : undefined;
    await verifyLatest(candidate, remote.options);
    assert.ok(remote.elapsed >= 5000 && remote.elapsed < 180000);
    assert.ok(remote.calls.every((args) => args[0] === "view"));
  }
});

test("latest verification immediately rejects a newer tag even while target metadata is missing", async (t) => {
  const { candidate } = candidateFixture(t);
  const first = candidate.packages[0];
  const remote = registry(candidate, { populated: true });
  remote.versions.delete(first.name);
  let reads = 0;
  remote.override = (args) => args[0] === "view" && args[1] === first.name && args[2] === "dist-tags.latest"
    ? ok(++reads === 1 ? "0.22.1" : "0.28.0") : undefined;
  await assert.rejects(verifyLatest(candidate, remote.options), /backwards/);
  assert.equal(reads, 2);
  assert.equal(remote.elapsed, 2000);
  assert.ok(remote.calls.every((args) => args[0] === "view" && [first.name, `${first.name}@${VERSION}`].includes(args[1])));
});

test("latest verification never retries invalid tags, integrity conflicts or query failures", async (t) => {
  const { candidate } = candidateFixture(t);
  const first = candidate.packages[0];
  for (const [field, response, error] of [
    ["dist-tags.latest", ok("0.27.0-beta.1"), /invalid stable npm version/],
    ["dist-tags.latest", ok(""), /invalid stable npm version/],
    ["dist-tags.latest", { status: 1, stdout: "", stderr: "E403 access denied" }, /E403/],
    ["dist-tags.latest", { status: 1, stdout: "", stderr: "ECONNRESET" }, /ECONNRESET/],
    ["dist-tags.latest", { ...missing(), signal: "SIGTERM" }, /SIGTERM/],
    ["dist-tags.latest", { ...missing(), status: 143 }, /exit 143/],
    ["dist.integrity", { ...missing(), error: new Error("ETIMEDOUT") }, /ETIMEDOUT/],
    ["dist.integrity", { status: 0, stdout: "{", stderr: "" }, /Invalid JSON/],
    ["dist.integrity", ok(null), /Missing registry value/],
    ["dist.integrity", ok(`sha512-${Buffer.alloc(64, 1).toString("base64")}`), /integrity differs/],
  ]) {
    const remote = registry(candidate, { populated: true });
    // A stale tag does not excuse malformed or mismatched target integrity.
    remote.override = (args) => args[0] === "view" && args[2] === field &&
      [first.name, `${first.name}@${VERSION}`].includes(args[1]) ? response : undefined;
    await assert.rejects(verifyLatest(candidate, remote.options), error);
    assert.equal(remote.elapsed, 0);
    assert.ok(remote.calls.length <= 2);
    assert.ok(remote.calls.every((args) => args[0] === "view"));
  }
});

test("latest and integrity queries share one deadline and stop before starting a query past it", async (t) => {
  const { candidate } = candidateFixture(t);
  let elapsed = 0;
  const queries = [];
  await assert.rejects(verifyLatest(candidate, {
    now: () => elapsed,
    runNpm: (args, options) => {
      queries.push({ at: elapsed, field: args[2], ...options });
      assert.ok(elapsed < 360000, "do not start the second view after the first consumes the budget");
      assert.ok(Number.isInteger(options.timeout) && options.timeout > 0);
      assert.ok(options.timeout <= Math.min(30000, 360000 - elapsed));
      elapsed += Math.min(24000, options.timeout);
      return args[2] === "dist-tags.latest" ? ok("0.22.1") : ok(candidate.packages[0].integrity);
    },
    wait: async (milliseconds) => {
      assert.ok(milliseconds > 0 && milliseconds <= Math.min(15000, 360000 - elapsed));
      elapsed += milliseconds;
    },
  }), /latest does not identify the verified version within 360s/);
  assert.equal(elapsed, 360000);
  assert.ok(queries.some((query) => query.field === "dist.integrity"));
  assert.ok(queries.at(-1).timeout < 30000);
  assert.ok(queries.every((query) => query.killSignal === "SIGKILL"));
});

test("a latest visibility timeout resumes the same candidate without repeating completed mutations", async (t) => {
  const { candidate } = candidateFixture(t);
  const remote = registry(candidate, { populated: true });
  const delayed = candidate.packages[0];
  remote.override = (args) => args[0] === "view" && args[1] === delayed.name && args[2] === "dist-tags.latest"
    ? ok("0.22.1") : undefined;
  await assert.rejects(promote(candidate, remote.options), /latest does not identify the verified version within 360s/);
  assert.equal(remote.elapsed, 360000);
  assert.deepEqual(remote.calls.filter((args) => args[0] === "dist-tag").map((args) => args[2]),
    candidate.packages.map((entry) => `${entry.name}@${VERSION}`));
  assert.ok([...remote.latest.values()].every((value) => value === VERSION), "mutations succeeded before delayed verification failed");

  remote.override = null;
  remote.calls.length = 0;
  await promote(candidate, remote.options);
  assert.ok(remote.calls.every((args) => args[0] === "view"));
  for (const entry of candidate.packages) assert.equal(remote.versions.get(entry.name), entry.integrity);
});

test("the verify-latest CLI awaits asynchronous verification before reporting success", async (t) => {
  const { directory } = candidateFixture(t);
  const commands = temporary(t);
  fs.writeFileSync(path.join(commands, "npm"), `#!/usr/bin/env node
console.log(JSON.stringify({ error: { code: "E403", summary: "access denied" } }));
process.exitCode = 1;
`, { mode: 0o755 });
  const script = path.resolve(__dirname, "../../scripts/release-publish.js");
  await assert.rejects(promisify(execFile)(process.execPath,
    [script, "verify-latest", "--directory", directory, "--source-sha", SHA],
    { env: { ...process.env, PATH: `${commands}${path.delimiter}${process.env.PATH}` }, timeout: 15000 }), (error) => {
    assert.equal(error.code, 1);
    assert.equal(error.stdout, "", "a rejected verification must never print the final verified summary");
    assert.match(error.stderr, /release:publish: npm view.*E403/);
    return true;
  });
});

test("a concurrent newer entry tag cannot be rolled back during promotion", async (t) => {
  const { candidate } = candidateFixture(t);
  const remote = registry(candidate, { populated: true }); let metaReads = 0;
  remote.override = (args) => {
    if (args[0] === "view" && args[1] === META && args[2] === "dist-tags.latest" && ++metaReads === 2) remote.latest.set(META, "0.28.0");
  };
  await assert.rejects(promote(candidate, remote.options), /backwards/);
  assert.equal(remote.latest.get(META), "0.28.0");
  assert.equal(remote.calls.some((args) => args[0] === "dist-tag" && args[2] === `${META}@${VERSION}`), false);
});

test("latest verification checks all versions and rejects unknown version ordering", async (t) => {
  const { candidate } = candidateFixture(t);
  const remote = registry(candidate, { populated: true });
  await assert.rejects(verifyLatest(candidate, remote.options), /latest/);
  assert.equal(compareVersions("0.27.0", "0.27.0"), 0);
  assert.equal(compareVersions("0.27.0", "0.9.99"), 1);
  assert.equal(compareVersions("1.0.0", "2.0.0"), -1);
  assert.throws(() => compareVersions("0.27.0-beta.1", "0.27.0"));
});

test("release commands reject incomplete or unsafe options", () => {
  assert.deepEqual(parsePreflight(["--packages-only", "--version", VERSION, "--json"]), { packagesOnly: true, version: VERSION, json: true });
  assert.throws(() => parsePreflight(["--tag"]));
  assert.throws(() => parsePreflight(["--force"]));
  assert.equal(parseArgs(["verify-candidate", "--directory", "some dir", "--source-sha", SHA, "--allow-dirty"]).allowDirty, true);
  const publishArgs = ["publish", "--directory", "some dir", "--source-sha", SHA];
  for (const visibility of ["public", "private"]) assert.equal(parseArgs([...publishArgs, "--repository-visibility", visibility])["repository-visibility"], visibility);
  assert.throws(() => parseArgs(publishArgs), /explicit repository visibility/);
  assert.throws(() => parseArgs([...publishArgs, "--repository-visibility", "unknown"]), /explicit repository visibility/);
  assert.throws(() => parseArgs(["promote", "--directory", "some dir", "--source-sha", SHA, "--repository-visibility", "private"]));
  for (const args of [[], ["publish"], ["publish", "--directory", "some dir", "--source-sha", SHA, "--allow-dirty"], ["promote", "--directory", "--source-sha", SHA]]) assert.throws(() => parseArgs(args));
});

test('a rehashed tarball cannot omit a foreign-platform engine or corrupt its native manifest', t => {
  for (const fault of ['missing-platform', 'wrong-digest', 'wrong-version']) {
    const {directory,candidate,save}=candidateFixture(t),entry=candidate.packages[0];
    const staging=temporary(t),file=path.join(directory,entry.filename);
    execFileSync('tar',['-xzf',file,'-C',staging]);
    const manifestFile=path.join(staging,'package/native/manifest.json');
    const native=JSON.parse(fs.readFileSync(manifestFile));
    if(fault==='missing-platform')delete native.platforms['linux-amd64'];
    else if(fault==='wrong-digest')native.platforms['linux-amd64'].sha256='0'.repeat(64);
    else native.version='0.0.1';
    fs.writeFileSync(manifestFile,JSON.stringify(native));
    execFileSync('tar',['-czf',file,'-C',staging,'package']);
    entry.bytes=fs.statSync(file).size;entry.integrity=fileIntegrity(file);save();
    assert.throws(()=>verifyCandidate(directory),/native manifest|native binary integrity/);
  }
});
