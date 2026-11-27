// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { execFileSync } = require("node:child_process");
const { PACKAGES, PLATFORMS, VERSION } = require("./release-preflight.js");

function filenameFor(name, version) {
  return `${name.replace(/^@/, "").replace("/", "-")}-${version}.tgz`;
}

function fileIntegrity(file) {
  return `sha512-${crypto.createHash("sha512").update(fs.readFileSync(file)).digest("base64")}`;
}

function readPackedManifest(file) {
  return JSON.parse(readPackedFile(file, 'package.json'));
}

function readPackedFile(file, relative) {
  return execFileSync('tar', ['-xOf', file, 'package/' + relative], {
    stdio: ['ignore', 'pipe', 'pipe'], timeout: 15000, maxBuffer: 64 * 1024 * 1024,
  });
}

function verifyPackedContents(file, manifest) {
  const native = JSON.parse(readPackedFile(file, 'native/manifest.json'));
  if (native.protocol !== 1 || native.version !== manifest.version ||
      JSON.stringify(Object.keys(native.platforms ?? {}).sort()) !== JSON.stringify([...PLATFORMS].sort())) {
    throw new Error('Packed native manifest must identify all four platforms at the package version');
  }
  for (const platform of PLATFORMS) {
    const bytes = readPackedFile(file, `native/${platform}/flywheel`);
    const expected = native.platforms[platform];
    if (!bytes.length || expected.bytes !== bytes.length ||
        crypto.createHash('sha256').update(bytes).digest('hex') !== expected.sha256) {
      throw new Error(`Packed native binary integrity mismatch: ${platform}`);
    }
  }
  for (const relative of [...Object.values(manifest.bin), ...Object.values(manifest.exports), 'cordis.patch.yml', 'LICENSE', 'THIRD_PARTY_NOTICES.md']) {
    if (!readPackedFile(file, relative.replace(/^\.\//, '')).length) throw new Error(`Packed entry is empty: ${relative}`);
  }
  const composition = JSON.parse(readPackedFile(file, 'node_modules/dsh-preset-generator/package.json'));
  if (composition.name !== 'dsh-preset-generator' || composition.version !== '0.4.0' || Object.keys(composition.dependencies ?? {}).length) throw new Error('Packed PTC composition must match the reviewed 0.4.0 source');
  const antigravity = JSON.parse(readPackedFile(file, 'node_modules/@ckanner/dsh-subagent-antigravity/package.json'));
  if (antigravity.name !== '@ckanner/dsh-subagent-antigravity' || antigravity.version !== '0.3.2' || JSON.stringify(antigravity.dependencies) !== JSON.stringify({ '@agentclientprotocol/sdk': '2.9.1', ajv: '8.17.1', zod: '4.3.6' })) throw new Error('Packed Antigravity must match the reviewed 0.3.2 source and pinned ACP/schema dependencies');
  const adapter = JSON.parse(readPackedFile(file, 'node_modules/dsh-subagent-codex-pro/package.json'));
  if (adapter.name !== 'dsh-subagent-codex-pro' || adapter.version !== '0.10.2' || Object.keys(adapter.dependencies ?? {}).length) {
    throw new Error('Packed Codex Pro must be the reviewed dependency-free 0.10.2 adapter');
  }
}

function verifyCandidate(directory, { sourceSHA, allowDirty = false, readManifest = readPackedManifest } = {}) {
  const candidate = JSON.parse(fs.readFileSync(path.join(directory, "candidate.json"), "utf8"));
  if (candidate.format_version !== 2 || !VERSION.test(candidate.version ?? "")) throw new Error("Invalid release candidate format/version");
  if (!/^[0-9a-f]{40}$/.test(candidate.source?.commit ?? "") || !/^[0-9a-f]{40}$/.test(candidate.source?.tree ?? "") ||
      !/^[0-9a-f]{64}$/.test(candidate.source?.snapshot_sha256 ?? "") || typeof candidate.source?.dirty !== "boolean") {
    throw new Error("Invalid release source identity");
  }
  if (sourceSHA && candidate.source.commit !== sourceSHA) throw new Error("Candidate source SHA does not match the verified source");
  if (candidate.source.dirty && !allowDirty) throw new Error("Refusing to publish a dirty source candidate");
  if (!Array.isArray(candidate.packages) || candidate.packages.length !== PACKAGES.length) throw new Error("Candidate must contain exactly one complete package");
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
  for (const entry of candidate.packages) verifyPackedContents(path.join(directory, entry.filename), manifests[entry.name]);
  return candidate;
}

module.exports = { filenameFor, fileIntegrity, readPackedManifest, readPackedFile, verifyPackedContents, verifyCandidate };
