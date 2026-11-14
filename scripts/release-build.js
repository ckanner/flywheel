#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const os = require("node:os");
const crypto = require("node:crypto");
const { execFileSync } = require("node:child_process");
const { PACKAGES, PLATFORMS, preflight } = require("./release-preflight.js");
const { filenameFor, fileIntegrity, verifyCandidate } = require("./release-candidate.js");

function git(root, ...args) {
  return execFileSync("git", args, { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
}

function parsePackOutput(raw, name, version) {
  let result;
  try { result = JSON.parse(raw); }
  catch { throw new Error(`Unexpected npm pack output for ${name}: invalid JSON`); }
  let packed;
  // npm 11 returns an array; npm 12 keys each result by package name.
  if (Array.isArray(result) && result.length === 1) packed = result[0];
  else if (result && typeof result === "object" && !Array.isArray(result) &&
      Object.keys(result).length === 1 && Object.hasOwn(result, name)) packed = result[name];
  if (!packed || typeof packed !== "object" || Array.isArray(packed) ||
      packed.name !== name || packed.version !== version || packed.filename !== filenameFor(name, version)) {
    throw new Error(`Unexpected npm pack output for ${name}`);
  }
  return packed;
}

// Capture actual source, including new development files but excluding ignored
// build products. CI release candidates must additionally have a clean HEAD.
function snapshot(root, destination) {
  const paths = git(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z").split("\0").filter(Boolean);
  const hash = crypto.createHash("sha256");
  for (const name of [...new Set(paths)].sort()) {
    const source = path.join(root, name);
    const stat = fs.lstatSync(source, { throwIfNoEntry: false });
    if (!stat) continue;
    if (!stat.isFile() && !stat.isSymbolicLink()) throw new Error(`Unsupported source file: ${name}`);
    const bytes = stat.isSymbolicLink() ? Buffer.from(fs.readlinkSync(source)) : fs.readFileSync(source);
    const mode = stat.isSymbolicLink() ? "120000" : (stat.mode & 0o111) ? "100755" : "100644";
    hash.update(`${name}\0${mode}\0${bytes.length}\0`);
    hash.update(bytes);
    if (destination) {
      const target = path.join(destination, name);
      fs.mkdirSync(path.dirname(target), { recursive: true });
      if (stat.isSymbolicLink()) fs.symlinkSync(bytes.toString(), target);
      else fs.writeFileSync(target, bytes, { mode: stat.mode & 0o777 });
    }
  }
  return hash.digest("hex");
}

function build(root, version, output) {
  preflight(root, { version, packagesOnly: true });
  const statusBefore = git(root, "status", "--porcelain").trim();
  const before = { commit: git(root, "rev-parse", "HEAD").trim(), tree: git(root, "rev-parse", "HEAD^{tree}").trim(),
    dirty: statusBefore !== "" };
  const work = fs.mkdtempSync(path.join(os.tmpdir(), "flywheel-npm-build-"));
  try {
    const source = path.join(work, "source");
    const packages = path.join(work, "packages");
    fs.mkdirSync(source);
    fs.mkdirSync(packages);
    before.snapshot_sha256 = snapshot(root, source);
    // Install the locked build toolchain inside the snapshot. CSS Modules need
    // Lightning CSS; neither the caller's dist/ nor node_modules/ is a source.
    execFileSync("npm", ["ci", "--omit=dev", "--ignore-scripts", "--no-audit", "--no-fund"], { cwd: source, stdio: "inherit" });
    const pluginDirectory = path.join(source, "plugins/dsh");
    const helper = JSON.parse(fs.readFileSync(path.join(pluginDirectory, 'package.json'), 'utf8'));
    if (helper.private !== true || helper.name !== '@ckanner/flywheel' || helper.version !== version || helper.engines.dsh !== JSON.parse(fs.readFileSync(path.join(source, 'package.json'), 'utf8')).engines.dsh) throw new Error('DSH source package must stay private and match the unified identity');
    execFileSync("npm", ["ci", "--include=dev", "--include=optional", "--ignore-scripts", "--no-audit", "--no-fund"], {
      cwd: pluginDirectory, stdio: "inherit",
    });
    execFileSync(process.execPath, ["scripts/build.mjs", "--package-root", source], { cwd: pluginDirectory, stdio: "inherit" });
    const date = git(root, "show", "-s", "--format=%cs", "HEAD").trim();
    const native = { protocol: 1, version, platforms: {} };
    for (const platform of PLATFORMS) {
      const [goos, goarch] = platform.split("-");
      const binary = path.join(source, "native", platform, "flywheel");
      fs.mkdirSync(path.dirname(binary), { recursive: true });
      execFileSync("go", ["build", "-buildvcs=false", "-trimpath", "-ldflags",
        `-s -w -X github.com/ckanner/flywheel/internal/build.Version=${version} -X github.com/ckanner/flywheel/internal/build.Date=${date}`,
        "-o", binary, "./cmd/flywheel"], {
        cwd: source, stdio: "inherit", env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: "0" },
      });
      fs.chmodSync(binary, 0o755);
      const bytes = fs.readFileSync(binary);
      native.platforms[platform] = { bytes: bytes.length, sha256: crypto.createHash("sha256").update(bytes).digest("hex") };
    }
    fs.writeFileSync(path.join(source, "native/manifest.json"), JSON.stringify(native, null, 2) + "\n");
    const entries = [];
    for (const entry of PACKAGES) {
      const result = execFileSync("npm", ["pack", "--ignore-scripts", "--json", "--pack-destination", packages], {
        cwd: path.join(source, entry.directory), encoding: "utf8", stdio: ["ignore", "pipe", "inherit"],
      });
      const { filename } = parsePackOutput(result, entry.name, version);
      const file = path.join(packages, filename);
      entries.push({ name: entry.name, version, filename, bytes: fs.statSync(file).size, integrity: fileIntegrity(file) });
    }
    if (git(root, "rev-parse", "HEAD").trim() !== before.commit || git(root, "status", "--porcelain").trim() !== statusBefore ||
        snapshot(root) !== before.snapshot_sha256) {
      throw new Error("Source changed while building; discard this candidate and retry");
    }
    const candidate = { format_version: 2, version, source: before, packages: entries };
    fs.writeFileSync(path.join(packages, "candidate.json"), JSON.stringify(candidate, null, 2) + "\n");
    verifyCandidate(packages, { sourceSHA: before.commit, allowDirty: true });
    const destination = path.resolve(root, output);
    fs.mkdirSync(destination, { recursive: true });
    for (const filename of [...entries.map((entry) => entry.filename), "candidate.json"]) {
      fs.copyFileSync(path.join(packages, filename), path.join(destination, filename));
    }
    console.log(`Built ${entries.length} verified npm packages for ${version} at ${destination}${before.dirty ? " (development source; not publishable)" : ""}`);
    return candidate;
  } finally {
    fs.rmSync(work, { recursive: true, force: true });
  }
}

if (require.main === module) {
  try {
    if (process.argv.length < 3 || process.argv.length > 4) throw new Error("usage: release-build.js <version> [outdir]");
    build(path.resolve(__dirname, ".."), process.argv[2], process.argv[3] ?? "dist-npm");
  } catch (error) {
    console.error(`release:build: ${error.message}`);
    process.exitCode = 1;
  }
}

module.exports = { parsePackOutput, snapshot, build };
