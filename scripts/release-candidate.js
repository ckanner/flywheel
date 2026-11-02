// Copyright (c) 2026 ckanner. SPDX-License-Identifier: UNLICENSED
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { execFileSync } = require("node:child_process");
const { PACKAGES, VERSION } = require("./release-preflight.js");

function filenameFor(name, version) {
  return `${name.replace(/^@/, "").replace("/", "-")}-${version}.tgz`;
}

function fileIntegrity(file) {
  return `sha512-${crypto.createHash("sha512").update(fs.readFileSync(file)).digest("base64")}`;
}

function readPackedManifest(file) {
  return JSON.parse(execFileSync("tar", ["-xOf", file, "package/package.json"], {
    encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], timeout: 15000,
  }));
}

function verifyCandidate(directory, { sourceSHA, allowDirty = false, readManifest = readPackedManifest } = {}) {
  const candidate = JSON.parse(fs.readFileSync(path.join(directory, "candidate.json"), "utf8"));
  if (candidate.format_version !== 1 || !VERSION.test(candidate.version ?? "")) throw new Error("Invalid release candidate format/version");
  if (!/^[0-9a-f]{40}$/.test(candidate.source?.commit ?? "") || !/^[0-9a-f]{40}$/.test(candidate.source?.tree ?? "") ||
      !/^[0-9a-f]{64}$/.test(candidate.source?.snapshot_sha256 ?? "") || typeof candidate.source?.dirty !== "boolean") {
    throw new Error("Invalid release source identity");
  }
  if (sourceSHA && candidate.source.commit !== sourceSHA) throw new Error("Candidate source SHA does not match the verified source");
  if (candidate.source.dirty && !allowDirty) throw new Error("Refusing to publish a dirty source candidate");
  if (!Array.isArray(candidate.packages) || candidate.packages.length !== PACKAGES.length) throw new Error("Candidate must contain exactly six packages");
  const expected = new Set(PACKAGES.map((entry) => entry.name));
  const manifests = {};
  for (const entry of candidate.packages) {
    if (!expected.delete(entry.name) || entry.version !== candidate.version || entry.filename !== filenameFor(entry.name, candidate.version)) {
      throw new Error("Candidate package set, version or filename mismatch");
    }
    const file = path.join(directory, entry.filename);
    const stat = fs.lstatSync(file);
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size !== entry.bytes || fileIntegrity(file) !== entry.integrity) {
      throw new Error(`Candidate bytes/integrity mismatch: ${entry.name}`);
    }
    manifests[entry.name] = readManifest(file);
  }
  require("./release-preflight.js").validatePackages(manifests, candidate.version);
  return candidate;
}

module.exports = { filenameFor, fileIntegrity, readPackedManifest, verifyCandidate };
