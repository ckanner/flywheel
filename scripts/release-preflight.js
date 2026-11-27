#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
// One published package owns the CLI, all native binaries, plugin and workers.
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { execFileSync } = require("node:child_process");

const PLATFORMS = ["darwin-arm64", "darwin-amd64", "linux-arm64", "linux-amd64"];
const META = "@ckanner/flywheel";
const PLUGIN = META; // CLI, host plugin and machine workers share one published identity.
const VERSION = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const PACKAGES = [{ name: META, directory: "." }];

function validatePackages(manifests, requestedVersion) {
  const meta = manifests[META];
  const version = requestedVersion ?? meta?.version;
  if (!VERSION.test(version ?? "")) throw new Error("Release version must be a stable X.Y.Z without leading zeroes");
  if (Object.keys(manifests).length !== 1 || !meta || meta.name !== META || meta.version !== version || meta.private === true) {
    throw new Error("Release must contain only the public @ckanner/flywheel package at the requested version");
  }
  if (meta.repository?.type !== 'git' || meta.repository?.url !== 'git+https://github.com/ckanner/flywheel.git' ||
      (meta.repository.directory ?? '.') !== '.') throw new Error('Flywheel must identify its source repository');
  if (JSON.stringify(meta.os) !== JSON.stringify(['darwin', 'linux']) || JSON.stringify(meta.cpu) !== JSON.stringify(['x64', 'arm64'])) {
    throw new Error('The unified package must support darwin/linux on x64/arm64');
  }
  if (Object.keys(meta.optionalDependencies ?? {}).length || Object.keys(meta.dependencies ?? {}).some(name => name.startsWith('@ckanner/flywheel') || name === '@ckanner/dsh-plugin-flywheel')) {
    throw new Error('Flywheel must carry its own native binaries and plugin, not depend on legacy split packages');
  }
  if (meta.dependencies?.['dsh-subagent-codex-pro'] !== '0.10.2' || JSON.stringify(meta.bundledDependencies) !== JSON.stringify(['dsh-subagent-codex-pro', 'dsh-preset-generator', '@ckanner/dsh-subagent-antigravity'])) {
    throw new Error('The reviewed Codex Pro source candidate must be pinned and bundled');
  }
  if (meta.dependencies?.['dsh-preset-generator'] !== '0.4.0') throw new Error('The shared PTC composition source must be pinned and bundled');
  if (!['0.5.0', '0.5.0 || 0.5.1'].includes(meta.peerDependencies?.['dsh-plugin-lcu']) || meta.peerDependenciesMeta?.['dsh-plugin-lcu']?.optional !== true) throw new Error('Managed LCU must remain an explicit optional peer');
  if (meta.peerDependencies?.['dsh-subagent-claude-code-pro'] !== '0.7.1' || meta.peerDependenciesMeta?.['dsh-subagent-claude-code-pro']?.optional !== true || meta.dependencies?.['dsh-subagent-claude-code-pro'] !== undefined) {
    throw new Error('Claude Code Pro must remain an optional peer pinned to subscription-capable 0.7.1');
  }
  if (meta.dependencies?.['@ckanner/dsh-subagent-antigravity'] !== 'file:plugins/dsh/vendor/ckanner-dsh-subagent-antigravity-0.3.2.tgz') throw new Error('The reviewed Antigravity source candidate must be pinned and bundled');
  const exports = { '.': './dist/index.mjs', './client': './dist/client.js', './worker-policy': './dist/policy.mjs', './worker': './dist/worker.mjs', './codex-pro-worker': './dist/codex-pro-worker.mjs', './claude-code-pro-worker': './dist/claude-code-pro-worker.mjs', './kimi-native-worker': './dist/kimi-native-worker.mjs', './antigravity-worker': './dist/antigravity-worker.mjs', './model-catalog-worker': './dist/model-catalog-worker.mjs', './native-jobs': './dist/native-jobs.mjs', './package.json': './package.json' };
  for (const [key, value] of Object.entries(exports)) if (meta.exports?.[key] !== value) throw new Error(`Missing unified package export ${key}`);
  for (const name of ['flywheel', 'flywheel-dsh-worker', 'flywheel-codex-pro-worker', 'flywheel-claude-code-pro-worker', 'flywheel-kimi-native-worker', 'flywheel-antigravity-worker', 'flywheel-model-catalog']) {
    if (meta.bin?.[name] !== `bin/${name}.js`) throw new Error(`Missing CLI entry ${name}`);
  }
  if (meta.dsh?.bundle?.patch !== './cordis.patch.yml' || meta.dsh?.client?.platform !== 'web' || !meta.engines?.dsh) throw new Error('Missing DSH host metadata');
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
    console.log(options.json ? JSON.stringify(result) : `release:check: ok (${result.version}; one complete package${result.tag ? "; exact tag and clean HEAD" : ""})`);
  } catch (error) {
    console.error(`release:check: ${error.message}`);
    process.exitCode = 1;
  }
}

module.exports = { META, PLUGIN, PLATFORMS, PACKAGES, VERSION, readPackages, validatePackages, validateTag, preflight, parseArgs };
