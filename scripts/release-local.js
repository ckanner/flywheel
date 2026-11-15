#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const { preflight } = require("./release-preflight.js");
const { verifyCandidate, readPackedManifest } = require("./release-candidate.js");
const { snapshot } = require("./release-build.js");
const publisher = require("./release-publish.js");
const { prepareClaudeRegistry, verifyArchive } = require("./claude-release-gate.js");

const REGISTRY = "https://registry.npmjs.org/";
const MAX_BUFFER = 4 * 1024 * 1024;
const capture = { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], maxBuffer: MAX_BUFFER };

function parseArgs(argv, cwd = process.cwd()) {
  const values = {};
  for (let index = 0; index < argv.length; index += 2) {
    const flag = argv[index];
    const value = argv[index + 1];
    if (!["--source-directory", "--directory", "--repository-visibility"].includes(flag) ||
        !value || value.startsWith("--") || Object.hasOwn(values, flag)) {
      throw new Error(`Unknown, duplicate or incomplete argument: ${flag}`);
    }
    values[flag] = value;
  }
  const sourceDirectory = values["--source-directory"]
    ? path.resolve(cwd, values["--source-directory"]) : path.resolve(__dirname, "..");
  if (!["private", "public"].includes(values["--repository-visibility"])) {
    throw new Error("--repository-visibility must explicitly be private or public");
  }
  return {
    sourceDirectory,
    directory: values["--directory"] ? path.resolve(cwd, values["--directory"]) : path.join(sourceDirectory, "dist-npm"),
    repositoryVisibility: values["--repository-visibility"],
  };
}

function checked(operation, result) {
  if (result.error || result.signal || result.status !== 0) {
    const reason = result.error?.code || result.signal || `exit ${result.status}`;
    throw new Error(`${operation} failed (${reason})`);
  }
  return result.stdout ?? "";
}

function isOfficialRegistry(value) {
  try { return new URL(value).href === REGISTRY; } catch (_) { return false; }
}

function verifySource(options, expectedManifest) {
  const manifestPath = path.join(options.directory, "candidate.json");
  const manifest = fs.readFileSync(manifestPath);
  if (expectedManifest && !manifest.equals(expectedManifest)) throw new Error("Candidate manifest changed during release");
  const candidate = verifyCandidate(options.directory, { readManifest(file) {
    const value = readPackedManifest(file);
    for (const key of ["registry", "@ckanner:registry"]) {
      if (Object.hasOwn(value.publishConfig ?? {}, key) && !isOfficialRegistry(value.publishConfig[key])) {
        throw new Error(`Candidate publishConfig.${key} must use the official npm registry`);
      }
    }
    return value;
  } });
  const assertHead = () => {
    const source = preflight(options.sourceDirectory, { version: candidate.version, tag: `v${candidate.version}` });
    if (source.source_sha !== candidate.source.commit) throw new Error("Candidate source does not match the clean release HEAD");
  };
  assertHead();
  const tree = checked("Read source tree", spawnSync("git", ["rev-parse", "HEAD^{tree}"], {
    ...capture, cwd: options.sourceDirectory, timeout: 30000, killSignal: "SIGKILL",
  })).trim();
  if (tree !== candidate.source.tree) throw new Error("Candidate tree does not match the release checkout");
  if (snapshot(options.sourceDirectory) !== candidate.source.snapshot_sha256) {
    throw new Error("Candidate source snapshot does not match the release checkout");
  }
  assertHead();
  if (!fs.readFileSync(manifestPath).equals(manifest)) throw new Error("Candidate manifest changed during verification");
  return { candidate, manifest };
}

function verifyRemoteTag(sourceDirectory, candidate, io = {}) {
  const ref = `refs/tags/v${candidate.version}`;
  const result = (io.spawnSync ?? spawnSync)("git", ["ls-remote", "--exit-code", "origin", ref, `${ref}^{}`], {
    ...capture, cwd: sourceDirectory, timeout: 30000, killSignal: "SIGKILL",
    env: { ...(io.env ?? process.env), GIT_TERMINAL_PROMPT: "0" },
  });
  const text = checked("Read origin release tag", result).trim();
  const refs = new Map();
  for (const line of text ? text.split(/\r?\n/) : []) {
    const fields = line.split("\t");
    if (fields.length !== 2 || !/^[0-9a-f]{40}$/.test(fields[0]) ||
        ![ref, `${ref}^{}`].includes(fields[1]) || refs.has(fields[1])) {
      throw new Error("Invalid or ambiguous origin release tag response");
    }
    refs.set(fields[1], fields[0]);
  }
  const objectSHA = refs.get(ref);
  const commitSHA = refs.get(`${ref}^{}`) ?? objectSHA;
  if (!objectSHA || commitSHA !== candidate.source.commit) {
    throw new Error("Origin release tag does not identify the candidate source");
  }
  return { objectSHA, commitSHA };
}

function npmEnvironment(io) {
  return { ...(io.env ?? process.env), npm_config_registry: REGISTRY };
}

function verifyRegistry(sourceDirectory, io = {}) {
  const result = (io.spawnSync ?? spawnSync)("npm", ["config", "get", "@ckanner:registry", `--registry=${REGISTRY}`], {
    ...capture, cwd: sourceDirectory, env: npmEnvironment(io), timeout: 30000, killSignal: "SIGKILL",
  });
  const scope = checked("Read npm scope registry", result).trim();
  if (!["undefined", "null"].includes(scope) && !isOfficialRegistry(scope)) {
    throw new Error("The @ckanner scope must use the official npm registry");
  }
}

function requireTTY(io) {
  if (!(io.stdin ?? process.stdin).isTTY || !(io.stdout ?? process.stdout).isTTY) {
    throw new Error("Local publication requires a real TTY for npm browser/2FA authentication");
  }
}

function createNpmRunner(sourceDirectory, io = {}, beforeWrite = () => {}) {
  return (args, { timeout = 180000, killSignal = "SIGTERM" } = {}) => {
    const mutation = args[0] === "publish" || (args[0] === "dist-tag" && args[1] === "add");
    if (!mutation && !["view", "pack"].includes(args[0])) throw new Error(`Unexpected npm operation: ${args[0]}`);
    if (mutation) {
      requireTTY(io);
      beforeWrite();
    }
    // npm can fall back to a configured default scope before its registry flag.
    return (io.spawnSync ?? spawnSync)("npm", [...args, "--scope=@ckanner", `--registry=${REGISTRY}`], {
      ...capture, cwd: sourceDirectory, env: npmEnvironment(io),
      stdio: mutation ? "inherit" : capture.stdio, timeout, killSignal,
    });
  };
}

function runInstallation(name, options, candidate, io, claudeTarball) {
  const env = npmEnvironment(io);
  if (name === "test:install") {
    env.FLYWHEEL_NPM_PACKAGES_DIR = options.directory;
    // The optional Claude installation smoke must run against the exact bytes
    // fetched by the release gate, regardless of the caller's environment.
    env.FLYWHEEL_CLAUDE_CODE_PRO_TARBALL = claudeTarball;
  }
  else {
    env.FLYWHEEL_NPM_VERSION = candidate.version;
    env.FLYWHEEL_NPM_REGISTRY = REGISTRY;
  }
  const result = (io.spawnSync ?? spawnSync)("npm", ["run", name], {
    ...capture, cwd: options.sourceDirectory, env, stdio: ["ignore", "inherit", "inherit"],
    timeout: 300000, killSignal: "SIGTERM",
  });
  checked(name, result);
}

async function releaseLocal(options, io = {}) {
  requireTTY(io);
  const { candidate, manifest } = verifySource(options);
  const remoteTag = verifyRemoteTag(options.sourceDirectory, candidate, io);
  let companion;
  const checkpoint = () => {
    verifySource(options, manifest);
    const current = verifyRemoteTag(options.sourceDirectory, candidate, io);
    if (current.objectSHA !== remoteTag.objectSHA || current.commitSHA !== remoteTag.commitSHA) {
      throw new Error("Origin release tag changed during release");
    }
    verifyRegistry(options.sourceDirectory, io);
    if (companion) verifyArchive(companion.tarball, companion.metadata);
  };
  verifyRegistry(options.sourceDirectory, io);
  const runNpm = createNpmRunner(options.sourceDirectory, io, checkpoint);
  const log = io.log ?? console.log;
  const publishOptions = { runNpm, wait: io.wait, now: io.now, log, repositoryVisibility: options.repositoryVisibility };
  try {
    companion = prepareClaudeRegistry(options.sourceDirectory, { runNpm });
    runInstallation("test:install", options, candidate, io, companion.tarball);
    checkpoint();
    await (io.publish ?? publisher.publish)(candidate, options.directory, publishOptions);
    checkpoint();
    runInstallation("test:registry", options, candidate, io);
    checkpoint();
    await (io.promote ?? publisher.promote)(candidate, publishOptions);
    await (io.verifyLatest ?? publisher.verifyLatest)(candidate, publishOptions);
    checkpoint();
    log(`release:local: completed ${candidate.version}; source ${candidate.source.commit}; the complete package verified and promoted to latest`);
    return candidate;
  } finally { companion?.dispose(); }
}

async function main(argv, io = {}) {
  return releaseLocal(parseArgs(argv), io);
}

if (require.main === module) main(process.argv.slice(2)).catch((error) => {
  console.error(`release:local: ${error.message}`);
  process.exitCode = 1;
});

module.exports = { parseArgs, verifySource, verifyRemoteTag, verifyRegistry, createNpmRunner, releaseLocal, main };
