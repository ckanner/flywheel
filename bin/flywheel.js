#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
"use strict";

const { loadModelCatalogWorkerPath, loadNativePath, loadWorkerPath, loadCodexProWorkerPath, loadClaudeCodeProWorkerPath, loadKimiNativeWorkerPath, loadAntigravityWorkerPath, assertSupportedNode, runChild } = require("./runtime.js");

async function main() {
  const args = process.argv.slice(2);
  if (args[0] === "install" || args[0] === "setup") {
    return require("../npm/install.js").main(args.slice(1));
  }
  assertSupportedNode();

  if (args[0] === 'native') {
    return (await import('../dist/native-jobs.mjs')).main(args.slice(1));
  }

  const env = { ...process.env };
  // An npm dependency's bin is not necessarily on a desktop application's
  // PATH. Resolve this installation's worker without changing an explicit
  // user override or relying on the shell's global npm prefix.
  if (env.FLYWHEEL_DSH_WORKER === undefined) {
    env.FLYWHEEL_DSH_WORKER = loadWorkerPath();
  }
  if (env.FLYWHEEL_CODEX_PRO_WORKER === undefined) {
    env.FLYWHEEL_CODEX_PRO_WORKER = loadCodexProWorkerPath();
  }
  if (env.FLYWHEEL_CLAUDE_CODE_PRO_WORKER === undefined) {
    env.FLYWHEEL_CLAUDE_CODE_PRO_WORKER = loadClaudeCodeProWorkerPath();
  }
  if (env.FLYWHEEL_KIMI_NATIVE_WORKER === undefined) {
    env.FLYWHEEL_KIMI_NATIVE_WORKER = loadKimiNativeWorkerPath();
  }
  if (env.FLYWHEEL_ANTIGRAVITY_WORKER === undefined) {
    env.FLYWHEEL_ANTIGRAVITY_WORKER = loadAntigravityWorkerPath();
  }
  if (env.FLYWHEEL_MODEL_CATALOG_WORKER === undefined) {
    env.FLYWHEEL_MODEL_CATALOG_WORKER = loadModelCatalogWorkerPath();
  }
  return runChild(loadNativePath(), args, { env });
}

main().then(code => { process.exitCode = code; }).catch(error => {
  console.error(`flywheel: ${error.message}`);
  process.exitCode = 1;
});
