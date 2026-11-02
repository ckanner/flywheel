#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: UNLICENSED
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const { performance } = require("node:perf_hooks");
const { PACKAGES, VERSION } = require("./release-preflight.js");
const { verifyCandidate, fileIntegrity } = require("./release-candidate.js");

function compareVersions(a, b) {
  if (!VERSION.test(a ?? "") || !VERSION.test(b ?? "")) throw new Error("Cannot compare an invalid stable npm version");
  const left = a.split(".").map(BigInt);
  const right = b.split(".").map(BigInt);
  for (let i = 0; i < left.length; i++) if (left[i] !== right[i]) return left[i] < right[i] ? -1 : 1;
  return 0;
}

function abnormalExit(result) {
  return Boolean(result.error || result.signal || !Number.isInteger(result.status) || result.status < 0 || result.status >= 128);
}

function isNotFound(result) {
  if (result.status === 0 || abnormalExit(result)) return false;
  for (const text of [result.stdout, result.stderr]) {
    try { if (JSON.parse(text).error?.code === "E404") return true; } catch (_) { /* npm may emit diagnostics on stderr. */ }
  }
  return false;
}

function defaultRunNpm(args, { timeout = 180000, killSignal = "SIGTERM" } = {}) {
  return spawnSync("npm", args, { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], timeout, killSignal, maxBuffer: 4 * 1024 * 1024 });
}

function failure(operation, result) {
  const detail = result.error?.message || result.signal || result.stderr?.trim() || result.stdout?.trim() || `exit ${result.status}`;
  return new Error(`${operation} (exit ${result.status}): ${String(detail).slice(0, 4000)}`);
}

function readValue(args, runNpm = defaultRunNpm, { allowMissingValue = false } = {}) {
  const result = runNpm(["view", ...args, "--json", "--prefer-online"]);
  if (result.error || result.signal || result.status !== 0) {
    if (isNotFound(result)) return null;
    throw failure(`npm view ${args.join(" ")}`, result);
  }
  const raw = result.stdout?.trim();
  if (!raw) {
    if (allowMissingValue) return null;
    throw new Error(`Missing registry value from npm view ${args.join(" ")}`);
  }
  let value;
  try { value = JSON.parse(raw); } catch (_) { throw new Error(`Invalid JSON from npm view ${args.join(" ")}`); }
  // npm 12 wraps a selected scalar in an array. Never pick one value out of an
  // ambiguous response, and keep an empty result subject to the missing-value policy.
  if (Array.isArray(value)) {
    if (value.length > 1) throw new Error(`Unexpected response from npm view ${args.join(" ")}`);
    value = value.length === 0 ? null : value[0];
  }
  if (value !== null && typeof value !== "string") throw new Error(`Unexpected response from npm view ${args.join(" ")}`);
  if (value === null && !allowMissingValue) throw new Error(`Missing registry value from npm view ${args.join(" ")}`);
  return value;
}

function remoteIntegrity(entry, runNpm) {
  const value = readValue([`${entry.name}@${entry.version}`, "dist.integrity"], runNpm);
  if (value !== null && !/^sha512-[A-Za-z0-9+/]{86}==$/.test(value)) throw new Error(`Invalid registry integrity for ${entry.name}`);
  return value;
}

function checkIntegrity(entry, actual, required = false) {
  if (actual === null) {
    if (required) throw new Error(`Registry version is not visible: ${entry.name}@${entry.version}`);
  } else if (actual !== entry.integrity) {
    throw new Error(`Published integrity differs from the verified candidate: ${entry.name}@${entry.version}`);
  }
}

function checkLatest(entry, runNpm) {
  const current = readValue([entry.name, "dist-tags.latest"], runNpm, { allowMissingValue: true });
  if (current !== null && compareVersions(current, entry.version) > 0) throw new Error(`Refusing to move ${entry.name} latest backwards from ${current} to ${entry.version}`);
  return current;
}

function orderedPackages(candidate) {
  return PACKAGES.map((expected) => candidate.packages.find((entry) => entry.name === expected.name));
}

function assertLocalBytes(directory, entry) {
  const file = path.join(directory, entry.filename);
  const stat = fs.lstatSync(file);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size !== entry.bytes || fileIntegrity(file) !== entry.integrity) {
    throw new Error(`Candidate changed before publication: ${entry.name}`);
  }
  return file;
}

const defaultWait = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds));
const defaultNow = () => performance.now();
const VISIBILITY_TIMEOUT_MS = 180000;

async function waitForPackage(entry, { runNpm = defaultRunNpm, wait = defaultWait, now = defaultNow } = {}) {
  const deadline = now() + VISIBILITY_TIMEOUT_MS;
  let delay = 2000;
  for (;;) {
    const remaining = Math.floor(deadline - now());
    if (remaining <= 0) break;
    // Only metadata queries use the shortened, hard timeout. Include query
    // time in the deadline; npm's own fetch retries must not extend the window.
    const actual = remoteIntegrity(entry, (args) => runNpm(args, { timeout: Math.min(30000, remaining), killSignal: "SIGKILL" }));
    checkIntegrity(entry, actual);
    const afterQuery = now();
    if (afterQuery >= deadline) break;
    if (actual !== null) return;
    await wait(Math.min(delay, deadline - afterQuery));
    delay = Math.min(delay * 2, 15000);
  }
  throw new Error(`Registry version is not visible within ${VISIBILITY_TIMEOUT_MS / 1000}s after publication: ${entry.name}@${entry.version}`);
}

async function verifyRemote(candidate, options = {}) {
  for (const entry of orderedPackages(candidate)) await waitForPackage(entry, options);
}

function provenanceFlag(visibility) {
  if (visibility === "public") return "--provenance";
  if (visibility === "private") return "--provenance=false";
  throw new Error("Publishing requires explicit repository visibility: public or private");
}

async function publish(candidate, directory, { runNpm = defaultRunNpm, wait = defaultWait, now = defaultNow, log = console.log, repositoryVisibility } = {}) {
  const provenance = provenanceFlag(repositoryVisibility);
  log(`Repository visibility: ${repositoryVisibility}; npm provenance: ${repositoryVisibility === "public" ? "required" : "unsupported for private repositories, explicitly disabled"}`);
  const entries = orderedPackages(candidate);
  // Validate every existing version and channel before the first mutation.
  const existing = new Map();
  for (const entry of entries) {
    assertLocalBytes(directory, entry);
    const actual = remoteIntegrity(entry, runNpm);
    checkIntegrity(entry, actual);
    existing.set(entry.name, actual);
    checkLatest(entry, runNpm);
  }
  const tag = `candidate-${candidate.version.replaceAll(".", "-")}`;
  for (const entry of entries) {
    if (existing.get(entry.name) !== null) {
      log(`Verified existing ${entry.name}@${entry.version}`);
      continue;
    }
    checkLatest(entry, runNpm);
    const file = assertLocalBytes(directory, entry);
    const result = runNpm(["publish", file, "--access", "public", "--ignore-scripts", provenance, "--tag", tag]);
    if (abnormalExit(result)) throw failure(`npm publish ${entry.name}`, result);
    if (result.status !== 0) {
      // A network failure may happen after npm accepted the package. Only its
      // exact registry integrity can turn an uncertain response into success.
      const actual = remoteIntegrity(entry, runNpm);
      if (actual === null) throw failure(`npm publish ${entry.name}`, result);
      checkIntegrity(entry, actual, true);
    }
    await waitForPackage(entry, { runNpm, wait, now });
    log(`Published and verified ${entry.name}@${entry.version} under ${tag}`);
  }
  await verifyRemote(candidate, { runNpm, wait, now });
  log("All six exact versions are verified; latest is unchanged. Run registry installation tests before promotion.");
}

async function promote(candidate, { runNpm = defaultRunNpm, wait = defaultWait, now = defaultNow, log = console.log } = {}) {
  await verifyRemote(candidate, { runNpm, wait, now });
  const entries = orderedPackages(candidate);
  for (const entry of entries) checkLatest(entry, runNpm);
  // Dependencies first, public entry last. npm has no multi-package dist-tag
  // transaction, so retries verify progress and never roll a newer tag back.
  for (const entry of entries) {
    if (checkLatest(entry, runNpm) === entry.version) continue;
    const result = runNpm(["dist-tag", "add", `${entry.name}@${entry.version}`, "latest"]);
    if (result.error || result.signal || result.status !== 0) throw failure(`npm dist-tag ${entry.name}`, result);
    log(`Promoted ${entry.name}@${entry.version} to latest`);
  }
  verifyLatest(candidate, { runNpm });
}

function verifyLatest(candidate, { runNpm = defaultRunNpm } = {}) {
  for (const entry of orderedPackages(candidate)) {
    checkIntegrity(entry, remoteIntegrity(entry, runNpm), true);
    if (readValue([entry.name, "dist-tags.latest"], runNpm, { allowMissingValue: true }) !== candidate.version) throw new Error(`latest does not identify the verified version: ${entry.name}`);
  }
}

function parseArgs(argv) {
  const [operation, ...args] = argv;
  if (!["verify-candidate", "publish", "promote", "verify-latest"].includes(operation)) throw new Error("Expected verify-candidate, publish, promote, or verify-latest");
  const options = { operation };
  for (let i = 0; i < args.length; i++) {
    if (["--directory", "--source-sha"].includes(args[i]) && args[i + 1] && !args[i + 1].startsWith("--")) options[args[i].slice(2)] = args[++i];
    else if (args[i] === "--repository-visibility" && operation === "publish" && args[i + 1]) options["repository-visibility"] = args[++i];
    else if (args[i] === "--allow-dirty" && operation === "verify-candidate") options.allowDirty = true;
    else throw new Error(`Unknown or incomplete argument: ${args[i]}`);
  }
  if (!options.directory || !/^[0-9a-f]{40}$/.test(options["source-sha"] ?? "")) throw new Error("--directory and exact --source-sha are required");
  if (operation === "publish") provenanceFlag(options["repository-visibility"]);
  return options;
}

async function main(argv) {
  const options = parseArgs(argv);
  const directory = path.resolve(options.directory);
  const candidate = verifyCandidate(directory, { sourceSHA: options["source-sha"], allowDirty: options.allowDirty });
  if (options.operation === "publish") await publish(candidate, directory, { repositoryVisibility: options["repository-visibility"] });
  else if (options.operation === "promote") await promote(candidate);
  else if (options.operation === "verify-latest") verifyLatest(candidate);
  console.log(`${options.operation}: verified ${candidate.version}, source ${candidate.source.commit}`);
}

if (require.main === module) main(process.argv.slice(2)).catch((error) => {
  console.error(`release:publish: ${error.message}`);
  process.exitCode = 1;
});

module.exports = { compareVersions, isNotFound, readValue, remoteIntegrity, checkIntegrity, checkLatest, assertLocalBytes,
  waitForPackage, verifyRemote, publish, promote, verifyLatest, parseArgs, main };
