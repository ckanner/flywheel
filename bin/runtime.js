// Copyright (c) 2026 ckanner. SPDX-License-Identifier: UNLICENSED
"use strict";

const { spawn } = require("node:child_process");
const { constants } = require("node:os");

const platforms = { darwin: "darwin", linux: "linux" };
const architectures = { arm64: "arm64", x64: "amd64" };
const forwardedSignals = ["SIGTERM", "SIGINT", "SIGHUP"];
const ownPackage = require("../package.json");

function packageEntry(name, entry, expectedVersion, missingMessage) {
  let installed;
  let entryPath;
  try {
    installed = require(`${name}/package.json`);
    entryPath = require.resolve(`${name}/${entry}`);
  } catch {
    throw new Error(missingMessage);
  }
  // npm may legitimately hoist dependencies. Check the resolved package's
  // identity, rather than assuming a particular machine's node_modules path.
  // A partial installation must not pick up an incompatible parent package.
  if (installed.name !== name) {
    throw new Error(`package identity mismatch for ${name}: found ${installed.name}; reinstall @ckanner/flywheel`);
  }
  if (typeof expectedVersion !== "string" || installed.version !== expectedVersion) {
    throw new Error(`version mismatch for ${name}: expected ${expectedVersion}, found ${installed.version}; reinstall @ckanner/flywheel`);
  }
  return entryPath;
}

function loadNativePath() {
  const platform = platforms[process.platform];
  const arch = architectures[process.arch];
  if (!platform || !arch) {
    throw new Error(`unsupported platform ${process.platform}/${process.arch} (supported: darwin/linux × amd64/arm64)`);
  }
  const name = `@ckanner/flywheel-${platform}-${arch}`;
  return packageEntry(name, "flywheel", ownPackage.optionalDependencies?.[name],
    `platform package ${name} is missing; reinstall @ckanner/flywheel with optional dependencies enabled`);
}

function loadWorkerPath() {
  const name = "@ckanner/dsh-plugin-flywheel";
  return packageEntry(name, "worker", ownPackage.dependencies?.[name],
    "the matching @ckanner/dsh-plugin-flywheel worker is missing; reinstall @ckanner/flywheel");
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

module.exports = { loadNativePath, loadWorkerPath, runChild };
