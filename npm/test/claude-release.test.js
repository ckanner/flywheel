"use strict";

const { test, before, after } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const crypto = require("node:crypto");
const { execFileSync } = require("node:child_process");
const { readClaudeRelease, verifyArchive, prepareClaudeRegistry } = require("../../scripts/claude-release-gate.js");
const { main } = require("../../scripts/release-publish.js");
const { fileIntegrity, filenameFor, verifyCandidate } = require("../../scripts/release-candidate.js");
const { META } = require("../../scripts/release-preflight.js");

const TOOL_ROOT = path.resolve(__dirname, "../..");
const REGISTRY = "https://registry.npmjs.org/";
const ok = (value) => ({ status: 0, stdout: JSON.stringify(value), stderr: "" });
const missing = () => ({ status: 1, stdout: JSON.stringify({ error: { code: "E404" } }), stderr: "" });

// A forgotten injection must fail locally, never contact a registry or publish.
let npmGuard;
const originalPath = process.env.PATH;
before(() => {
  npmGuard = fs.mkdtempSync(path.join(os.tmpdir(), "flywheel-claude-no-npm-"));
  fs.writeFileSync(path.join(npmGuard, "npm"),
    "#!/bin/sh\nprintf '%s\\n' 'Real npm is forbidden in Claude release tests' >&2\nexit 97\n", { mode: 0o755 });
  process.env.PATH = `${npmGuard}${path.delimiter}${originalPath ?? ""}`;
});
after(() => {
  if (originalPath === undefined) delete process.env.PATH;
  else process.env.PATH = originalPath;
  if (npmGuard) fs.rmSync(npmGuard, { recursive: true, force: true });
});

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "flywheel-claude-gate-test-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const metadata = { ...readClaudeRelease(TOOL_ROOT), registryReleaseVerified: false };
  delete metadata.registry;
  const vendor = path.join(root, "plugins/dsh/vendor");
  fs.mkdirSync(vendor, { recursive: true });
  const record = path.join(vendor, "claude-code-pro.json");
  const archive = path.join(vendor, metadata.tarball);
  fs.writeFileSync(record, JSON.stringify(metadata));
  fs.copyFileSync(path.join(TOOL_ROOT, "plugins/dsh/vendor", metadata.tarball), archive);
  return { root, vendor, metadata, record, archive };
}

function registry(source, { format = "npm12", mutate, result } = {}) {
  const calls = [];
  const runNpm = (args, options) => {
    calls.push({ args, options });
    assert.equal(args[0], "pack");
    assert.equal(args[1], `${source.metadata.package}@${source.metadata.version}`);
    for (const flag of ["--ignore-scripts", "--json", "--prefer-online", "--offline=false", "--fetch-retries=0",
      "--scope=@ckanner", `--registry=${REGISTRY}`, `--@ckanner:registry=${REGISTRY}`]) assert.ok(args.includes(flag), flag);
    const directory = args[args.indexOf("--pack-destination") + 1];
    const cache = args[args.indexOf("--cache") + 1];
    assert.equal(cache, path.join(directory, "cache"));
    assert.equal(fs.existsSync(cache), false, "no cached version can stand in for the registry release");
    assert.deepEqual(fs.readdirSync(directory), []);
    assert.ok(options.timeout > 0 && options.timeout <= 60000);
    assert.equal(options.killSignal, "SIGKILL");
    if (result) return result;
    const file = path.join(directory, source.metadata.tarball);
    fs.copyFileSync(source.archive, file);
    const packed = { name: source.metadata.package, version: source.metadata.version,
      filename: source.metadata.tarball, integrity: source.metadata.integrity };
    mutate?.({ packed, file, directory });
    return ok(format === "npm11" ? [packed] : { [source.metadata.package]: packed });
  };
  const directory = () => calls[0]?.args[calls[0].args.indexOf("--pack-destination") + 1];
  return { calls, runNpm, directory };
}

function candidateFixture(source) {
  const directory = path.join(source.root, "dist-npm");
  const staging = path.join(source.root, "staging");
  fs.mkdirSync(directory);
  fs.mkdirSync(path.join(staging, "package"), { recursive: true });
  const manifest = require("./support/unified-package.js").manifests("0.34.2")[META];
  require("./support/unified-package.js").writeContents(path.join(staging, "package"), manifest);
  const filename = filenameFor(META, manifest.version);
  const file = path.join(directory, filename);
  execFileSync("tar", ["-czf", file, "-C", staging, "package"], { stdio: "pipe", timeout: 10000 });
  const candidate = { format_version: 2, version: manifest.version, source: {
    commit: "1".repeat(40), tree: "2".repeat(40), dirty: false, snapshot_sha256: "3".repeat(64),
  }, packages: [{ name: META, version: manifest.version, filename,
    bytes: fs.statSync(file).size, integrity: fileIntegrity(file) }] };
  fs.writeFileSync(path.join(directory, "candidate.json"), JSON.stringify(candidate));
  verifyCandidate(directory);
  return { directory, candidate };
}

test("source candidate verification checks both hashes and identity without claiming a registry release", (t) => {
  const source = fixture(t);
  const before = fs.readFileSync(source.record);
  assert.equal(source.metadata.registryReleaseVerified, false);
  assert.equal(verifyArchive(source.archive, source.metadata).version, "0.7.1");
  assert.deepEqual(fs.readFileSync(source.record), before);
  for (const changes of [{ version: "0.5.0" }, { package: "another-package" }, { tarball: "../other.tgz" },
    { sourceCommit: "not-a-commit" }, { sourceTree: "not-a-tree" }, { sha256: "bad" }, { integrity: "sha1-bad" }]) {
    fs.writeFileSync(source.record, JSON.stringify({ ...source.metadata, ...changes }));
    assert.throws(() => readClaudeRelease(source.root), /Invalid reviewed Claude release metadata/);
  }
});

test("a matching source archive alone is not enough to publish; exact registry bytes are fetched and disposed", (t) => {
  for (const format of ["npm11", "npm12"]) {
    const source = fixture(t);
    const original = fs.readFileSync(source.record);
    const remote = registry(source, { format });
    const handle = prepareClaudeRegistry(source.root, remote);
    assert.equal(remote.calls.length, 1);
    assert.notEqual(handle.tarball, source.archive);
    assert.equal(verifyArchive(handle.tarball, source.metadata).name, source.metadata.package);
    assert.equal(handle.metadata.registryReleaseVerified, false);
    assert.deepEqual(fs.readFileSync(source.record), original);
    handle.dispose();
    handle.dispose();
    assert.equal(fs.existsSync(remote.directory()), false);
  }
});

test("registry absence, network failures, cancellation and malformed output fail closed and clean temporary files", (t) => {
  const source = fixture(t);
  for (const result of [missing(), { ...missing(), stderr: "ECONNRESET" },
    { ...ok([]), status: null }, { ...ok([]), signal: "SIGTERM" },
    { ...ok([]), error: Object.assign(new Error("timeout"), { code: "ETIMEDOUT" }) },
    ok([]), ok({}), { ...ok([]), stdout: "not JSON" }]) {
    const remote = registry(source, { result });
    assert.throws(() => prepareClaudeRegistry(source.root, remote), /Claude registry release is unavailable|Unexpected npm pack output/);
    assert.equal(remote.calls.length, 1);
    assert.equal(fs.existsSync(remote.directory()), false);
  }
});

test("registry byte substitution, mismatched integrity and identity cannot pass the gate", (t) => {
  const source = fixture(t);
  for (const mutate of [
    ({ file }) => fs.appendFileSync(file, "unreviewed bytes"),
    ({ packed }) => { packed.integrity = `sha512-${Buffer.alloc(64).toString("base64")}`; },
    ({ packed }) => { packed.version = "0.5.0"; },
    ({ packed }) => { packed.filename = "../substitute.tgz"; },
    ({ file }) => { fs.unlinkSync(file); fs.symlinkSync(source.archive, file); },
  ]) {
    const remote = registry(source, { mutate });
    assert.throws(() => prepareClaudeRegistry(source.root, remote), /SHA256\/integrity|registry integrity|Unexpected npm pack output|regular file/);
    assert.equal(fs.existsSync(remote.directory()), false);
  }
});

test("archive manifest identity is checked even when both hashes match the metadata", (t) => {
  const source = fixture(t);
  const staging = path.join(source.root, "wrong-manifest");
  fs.mkdirSync(path.join(staging, "package"), { recursive: true });
  fs.writeFileSync(path.join(staging, "package/package.json"), JSON.stringify({ name: source.metadata.package, version: "0.5.0" }));
  execFileSync("tar", ["-czf", source.archive, "-C", staging, "package"], { stdio: "pipe", timeout: 10000 });
  const bytes = fs.readFileSync(source.archive);
  const metadata = { ...source.metadata, sha256: crypto.createHash("sha256").update(bytes).digest("hex"), integrity: fileIntegrity(source.archive) };
  assert.throws(() => verifyArchive(source.archive, metadata), /package identity/);
});

test("both real publication command paths stop before writes when the companion is absent or substituted", async (t) => {
  for (const operation of ["publish", "promote"]) for (const mismatch of [false, true]) {
    const source = fixture(t);
    const { directory, candidate } = candidateFixture(source);
    const remote = registry(source, mismatch
      ? { mutate: ({ file }) => fs.appendFileSync(file, "substituted") } : { result: missing() });
    const args = [operation, "--directory", directory, "--source-sha", candidate.source.commit];
    if (operation === "publish") args.push("--repository-visibility", "private");
    await assert.rejects(main(args, { sourceDirectory: source.root, runNpm: remote.runNpm, log: () => {} }),
      /Claude registry release is unavailable|Claude archive SHA256\/integrity/);
    assert.deepEqual(remote.calls.map((call) => call.args[0]), ["pack"], "no registry mutation or Flywheel publication query may precede the companion gate");
    assert.equal(fs.existsSync(remote.directory()), false);
  }
});

test("publication and promotion proceed only after a matching registry fetch and never rewrite source evidence", async (t) => {
  for (const operation of ["publish", "promote"]) {
    const source = fixture(t);
    const before = fs.readFileSync(source.record);
    const { directory, candidate } = candidateFixture(source);
    const remote = registry(source);
    const entry = candidate.packages[0];
    const calls = [];
    let integrity = operation === "promote" ? entry.integrity : null;
    let latest = "0.34.1";
    const runNpm = (args, options) => {
      calls.push(args[0]);
      if (args[0] === "pack") return remote.runNpm(args, options);
      assert.equal(calls[0], "pack");
      assert.equal(remote.calls.length, 1);
      if (args[0] === "view") {
        if (args[2] === "dist-tags.latest") return ok(latest);
        assert.equal(args[2], "dist.integrity");
        return integrity === null ? missing() : ok(integrity);
      }
      if (args[0] === "publish") { integrity = entry.integrity; return ok(null); }
      assert.deepEqual(args.slice(0, 2), ["dist-tag", "add"]);
      latest = entry.version;
      return ok(null);
    };
    const args = [operation, "--directory", directory, "--source-sha", candidate.source.commit];
    if (operation === "publish") args.push("--repository-visibility", "private");
    await main(args, { sourceDirectory: source.root, runNpm, log: () => {} });
    assert.ok(calls.includes(operation === "publish" ? "publish" : "dist-tag"));
    assert.deepEqual(fs.readFileSync(source.record), before);
    assert.equal(fs.existsSync(remote.directory()), false);
  }
});

test("candidate validation and latest verification do not require a companion registry release", async (t) => {
  const source = fixture(t);
  const { directory, candidate } = candidateFixture(source);
  for (const operation of ["verify-candidate", "verify-latest"]) {
    await main([operation, "--directory", directory, "--source-sha", candidate.source.commit], {
      sourceDirectory: source.root, runNpm(args) {
        assert.equal(operation, "verify-latest", "candidate CI must not query the registry");
        assert.equal(args[0], "view", "read-only commands must not fetch or require the companion release");
        return ok(args[2] === "dist-tags.latest" ? candidate.version : candidate.packages[0].integrity);
      }, log: () => {},
    });
  }
  assert.equal(readClaudeRelease(source.root).registryReleaseVerified, false);
});
