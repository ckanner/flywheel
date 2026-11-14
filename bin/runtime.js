// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
"use strict";

const { spawn } = require("node:child_process");
const { constants } = require("node:os");

const platforms = { darwin: "darwin", linux: "linux" };
const architectures = { arm64: "arm64", x64: "amd64" };
const forwardedSignals = ["SIGTERM", "SIGINT", "SIGHUP"];
const ownPackage = require("../package.json");
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");

function packageFile(relative) {
  if (ownPackage.name !== '@ckanner/flywheel') throw new Error('Flywheel package identity mismatch');
  const filename = path.resolve(__dirname, '..', relative);
  const stat = fs.lstatSync(filename, { throwIfNoEntry: false });
  if (!stat?.isFile() || stat.isSymbolicLink()) throw new Error(`Bundled ${relative} is missing or invalid; reinstall @ckanner/flywheel`);
  return filename;
}

function loadNativePath() {
  const platform = platforms[process.platform];
  const arch = architectures[process.arch];
  if (!platform || !arch) {
    throw new Error(`unsupported platform ${process.platform}/${process.arch} (supported: darwin/linux × amd64/arm64)`);
  }
  const key = `${platform}-${arch}`;
  const manifest = JSON.parse(fs.readFileSync(packageFile('native/manifest.json'), 'utf8'));
  if (manifest.protocol !== 1 || manifest.version !== ownPackage.version) {
    throw new Error('Bundled engine version mismatch; reinstall @ckanner/flywheel');
  }
  const entry = manifest.platforms?.[key];
  const filename = packageFile(`native/${key}/flywheel`);
  const bytes = fs.readFileSync(filename);
  if (!entry || entry.bytes !== bytes.length || !/^[a-f0-9]{64}$/.test(entry.sha256 ?? '') ||
      crypto.createHash('sha256').update(bytes).digest('hex') !== entry.sha256) {
    throw new Error(`Bundled engine integrity mismatch for ${key}; reinstall @ckanner/flywheel`);
  }
  return filename;
}

function loadWorkerPath() {
  return packageFile('dist/worker.mjs');
}

function loadCodexProWorkerPath() {
  return packageFile('dist/codex-pro-worker.mjs');
}

function loadClaudeCodeProWorkerPath() {
  return packageFile('dist/claude-code-pro-worker.mjs');
}

function loadKimiNativeWorkerPath() {
  return packageFile('dist/kimi-native-worker.mjs');
}

function assertSupportedNode(version = process.versions.node) {
  const match = /^(\d+)\.(\d+)\.(\d+)(?:$|-)/.exec(version);
  if (!match || Number(match[1]) < 22 || (Number(match[1]) === 22 && Number(match[2]) < 18)) {
    throw new Error("Node.js 22.18.0 or newer is required");
  }
}

function signalExitCode(signal) {
  return constants.signals[signal] ? 128 + constants.signals[signal] : 1;
}

/** Preserve the native owner's cancellation and cleanup lifetime through npm. */
function runChild(command, args, { env = process.env, cwd, label = "flywheel" } = {}) {
  return new Promise(resolve => {
    const child = spawn(command, args, { env, cwd, stdio: "inherit", shell: false, detached: false });
    let cancelledBy;
    let spawnFailed = false;
    const listeners = new Map();
    for (const signal of forwardedSignals) {
      const listener = () => {
        cancelledBy ??= signal;
        // Go owns its worker groups. Keep this wrapper alive until Go confirms
        // its own exit; killing only this PID would orphan that cleanup stack.
        child.kill(signal);
      };
      listeners.set(signal, listener);
      process.on(signal, listener);
    }
    child.once("error", error => {
      spawnFailed = true;
      console.error(`${label}: could not start ${command}: ${error.message}`);
    });
    child.once("close", (code, signal) => {
      for (const [name, listener] of listeners) process.removeListener(name, listener);
      if (spawnFailed) return resolve(1);
      if (signal) return resolve(signalExitCode(signal));
      // A child trapping termination as exit 0 still cannot turn cancellation
      // into success. Preserve the native program's nonzero failure otherwise.
      if (cancelledBy && code === 0) return resolve(signalExitCode(cancelledBy));
      resolve(code ?? 1);
    });
  });
}

module.exports = { loadNativePath, loadWorkerPath, loadCodexProWorkerPath, loadClaudeCodeProWorkerPath, loadKimiNativeWorkerPath, assertSupportedNode, runChild };
