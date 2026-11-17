// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
"use strict";

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const crypto = require("node:crypto");
const { spawnSync } = require("node:child_process");
const { parsePackOutput } = require("./release-build.js");
const { readPackedManifest } = require("./release-candidate.js");

const PACKAGE = "dsh-subagent-claude-code-pro";
const VERSION = "0.5.3";
const REGISTRY = "https://registry.npmjs.org/";
const VENDOR = "plugins/dsh/vendor";
const SOURCE_DIRECTORY = path.resolve(__dirname, "..");

function validateMetadata(metadata) {
  if (!metadata || metadata.package !== PACKAGE || metadata.version !== VERSION ||
      metadata.tarball !== `${PACKAGE}-${VERSION}.tgz` ||
      !/^[0-9a-f]{40}$/.test(metadata.sourceCommit ?? "") ||
      !/^[0-9a-f]{40}$/.test(metadata.sourceTree ?? "") ||
      !/^[0-9a-f]{64}$/.test(metadata.sha256 ?? "") ||
      !/^sha512-[A-Za-z0-9+/]{86}==$/.test(metadata.integrity ?? "")) {
    throw new Error(`Invalid reviewed Claude release metadata; expected ${PACKAGE}@${VERSION}`);
  }
  return metadata;
}

function readClaudeRelease(sourceDirectory = SOURCE_DIRECTORY) {
  return validateMetadata(JSON.parse(fs.readFileSync(path.join(sourceDirectory, VENDOR, "claude-code-pro.json"), "utf8")));
}

// Used by candidate CI as well as the release gate. This verifies source
// candidate bytes; it does not claim that those bytes exist on the registry.
function verifyArchive(file, metadata) {
  validateMetadata(metadata);
  const stat = fs.lstatSync(file);
  if (!stat.isFile() || stat.isSymbolicLink()) throw new Error("Claude archive must be a regular file");
  const bytes = fs.readFileSync(file);
  if (crypto.createHash("sha256").update(bytes).digest("hex") !== metadata.sha256 ||
      `sha512-${crypto.createHash("sha512").update(bytes).digest("base64")}` !== metadata.integrity) {
    throw new Error(`Claude archive SHA256/integrity differs from the reviewed candidate: ${PACKAGE}@${VERSION}`);
  }
  const manifest = readPackedManifest(file);
  if (manifest.name !== metadata.package || manifest.version !== metadata.version) {
    throw new Error(`Claude archive package identity differs from ${PACKAGE}@${VERSION}`);
  }
  return manifest;
}

function defaultRunNpm(args, options) {
  return spawnSync("npm", args, {
    encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], maxBuffer: 4 * 1024 * 1024, ...options,
  });
}

// Publication always fetches the exact official registry version. A private,
// initially empty npm cache prevents a previously cached candidate or an offline
// setting from standing in for a currently available registry release.
function prepareClaudeRegistry(sourceDirectory = SOURCE_DIRECTORY, { runNpm = defaultRunNpm } = {}) {
  const metadata = readClaudeRelease(sourceDirectory);
  verifyArchive(path.join(sourceDirectory, VENDOR, metadata.tarball), metadata);
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "flywheel-claude-registry-"));
  const dispose = () => fs.rmSync(directory, { recursive: true, force: true });
  try {
    const result = runNpm(["pack", `${PACKAGE}@${VERSION}`, "--ignore-scripts", "--json",
      "--prefer-online", "--offline=false", "--fetch-retries=0", "--scope=@ckanner",
      `--registry=${REGISTRY}`, `--@ckanner:registry=${REGISTRY}`,
      "--cache", path.join(directory, "cache"), "--pack-destination", directory], {
      timeout: 60000, killSignal: "SIGKILL",
    });
    if (result.error || result.signal || result.status !== 0) {
      const reason = result.error?.code || result.signal || `exit ${result.status}`;
      throw new Error(`Claude registry release is unavailable: ${PACKAGE}@${VERSION} (${reason}); publish the matching companion before Flywheel`);
    }
    const packed = parsePackOutput(result.stdout, PACKAGE, VERSION);
    if (packed.integrity !== metadata.integrity) {
      throw new Error(`Claude registry integrity differs from the reviewed candidate: ${PACKAGE}@${VERSION}`);
    }
    const tarball = path.join(directory, packed.filename);
    verifyArchive(tarball, metadata);
    // Never update registryReleaseVerified in the source record. This temporary
    // handle is fresh evidence for this release attempt only.
    return { metadata, tarball, dispose };
  } catch (error) {
    dispose();
    throw error;
  }
}

module.exports = { readClaudeRelease, verifyArchive, prepareClaudeRegistry };
