#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
// One release owns the engine, four native packages and the DSH plugin.
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { execFileSync } = require("node:child_process");

const PLATFORMS = ["darwin-arm64", "darwin-amd64", "linux-arm64", "linux-amd64"];
const META = "@ckanner/flywheel";
const PLUGIN = "@ckanner/dsh-plugin-flywheel";
const VERSION = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const PACKAGES = [
  ...PLATFORMS.map((platform) => ({ name: `${META}-${platform}`, directory: `npm/platforms/${platform}`, platform })),
  { name: PLUGIN, directory: "plugins/dsh" },
  { name: META, directory: "." },
];

function validatePackages(manifests, requestedVersion) {
  const meta = manifests[META];
  const version = requestedVersion ?? meta?.version;
  if (!VERSION.test(version ?? "")) throw new Error("Release version must be a stable X.Y.Z without leading zeroes");
  for (const entry of PACKAGES) {
    const manifest = manifests[entry.name];
    if (!manifest || manifest.name !== entry.name) throw new Error(`Missing or incorrect manifest for ${entry.name}`);
    if (manifest.version !== version) throw new Error(`${entry.name} version ${manifest.version} != requested ${version}`);
    if (manifest.repository?.type !== "git" || manifest.repository?.url !== "git+https://github.com/ckanner/flywheel.git" ||
        (manifest.repository.directory ?? ".") !== entry.directory) {
      throw new Error(`${entry.name} must identify its Flywheel source repository and directory for provenance`);
    }
    if (entry.platform) {
      const [os, arch] = entry.platform.split("-");
      const cpu = arch === "amd64" ? "x64" : arch;
      if (JSON.stringify(manifest.os) !== JSON.stringify([os]) || JSON.stringify(manifest.cpu) !== JSON.stringify([cpu])) {
        throw new Error(`Incorrect os/cpu selector for ${entry.name}`);
      }
      if (meta.optionalDependencies?.[entry.name] !== version) throw new Error(`Meta optional dependency ${entry.name} must be exactly ${version}`);
    }
  }
  if (meta.dependencies?.[PLUGIN] !== version) throw new Error(`Meta dependency ${PLUGIN} must be exactly ${version}`);
  if (manifests[PLUGIN].dependencies?.['dsh-subagent-codex-pro'] !== '0.9.0') {
    throw new Error('The plugin dependency dsh-subagent-codex-pro must be exactly 0.9.0');
  }
  if (JSON.stringify(manifests[PLUGIN].bundledDependencies) !== JSON.stringify(['dsh-subagent-codex-pro'])) {
    throw new Error('The reviewed Codex Pro source candidate must be bundled; installation must not depend on an unpublished registry version');
  }
  if (manifests[PLUGIN].exports?.['./codex-pro-worker'] !== './dist/codex-pro-worker.js' ||
      manifests[PLUGIN].bin?.['flywheel-codex-pro-worker'] !== './dist/codex-pro-worker.js' ||
      meta.bin?.['flywheel-codex-pro-worker'] !== 'bin/flywheel-codex-pro-worker.js') {
    throw new Error('The release must expose the installed Codex Pro worker');
  }
  if (manifests[PLUGIN].peerDependencies?.['dsh-subagent-claude-code-pro'] !== '0.4.0' ||
      manifests[PLUGIN].peerDependenciesMeta?.['dsh-subagent-claude-code-pro']?.optional !== true ||
      manifests[PLUGIN].dependencies?.['dsh-subagent-claude-code-pro'] !== undefined) {
    throw new Error('Claude Code Pro must remain an optional peer pinned to machine-compatible 0.4.0');
  }
  if (manifests[PLUGIN].exports?.['./claude-code-pro-worker'] !== './dist/claude-code-pro-worker.js' ||
      manifests[PLUGIN].bin?.['flywheel-claude-code-pro-worker'] !== './dist/claude-code-pro-worker.js' ||
      meta.bin?.['flywheel-claude-code-pro-worker'] !== 'bin/flywheel-claude-code-pro-worker.js') {
    throw new Error('The release must expose the installed Claude Code Pro worker');
  }
  return version;
}

function readPackages(root) {
  return Object.fromEntries(PACKAGES.map((entry) => [entry.name,
    JSON.parse(fs.readFileSync(path.join(root, entry.directory, "package.json"), "utf8"))]));
}

function git(root, args) {
  return execFileSync("git", args, { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
}

function validateTag({ version, tag, head, target, dirty }) {
  if (tag !== `v${version}`) throw new Error(`Release tag must be v${version}, got ${tag}`);
  if (!/^[0-9a-f]{40}$/.test(head) || target !== head) throw new Error("Release tag must point to the exact current HEAD");
  if (dirty) throw new Error("Release checkout must be clean, including untracked source files");
  return { version, tag, source_sha: head };
}

function preflight(root, { version: requestedVersion, tag, packagesOnly = false } = {}) {
  const version = validatePackages(readPackages(root), requestedVersion);
  if (packagesOnly) return { version, packages: PACKAGES.map((entry) => entry.name) };
  const head = git(root, ["rev-parse", "HEAD"]);
  const exactTag = tag ?? git(root, ["describe", "--tags", "--exact-match", "HEAD"]);
  if (exactTag !== `v${version}`) throw new Error(`Release tag must be v${version}, got ${exactTag}`);
  const target = git(root, ["rev-parse", `refs/tags/${exactTag}^{commit}`]);
  return validateTag({ version, tag: exactTag, head, target, dirty: git(root, ["status", "--porcelain"]) !== "" });
}

function parseArgs(argv) {
  const opts = {};
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--packages-only") opts.packagesOnly = true;
    else if (argv[i] === "--json") opts.json = true;
    else if (["--version", "--tag"].includes(argv[i]) && argv[i + 1] && !argv[i + 1].startsWith("--")) opts[argv[i].slice(2)] = argv[++i];
    else throw new Error(`Unknown or incomplete argument: ${argv[i]}`);
  }
  return opts;
}

if (require.main === module) {
  try {
    const options = parseArgs(process.argv.slice(2));
    const result = preflight(path.resolve(__dirname, ".."), options);
    console.log(options.json ? JSON.stringify(result) : `release:check: ok (${result.version}; all six packages${result.tag ? "; exact tag and clean HEAD" : ""})`);
  } catch (error) {
    console.error(`release:check: ${error.message}`);
    process.exitCode = 1;
  }
}

module.exports = { META, PLUGIN, PLATFORMS, PACKAGES, VERSION, readPackages, validatePackages, validateTag, preflight, parseArgs };
