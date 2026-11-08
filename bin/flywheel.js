#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: UNLICENSED
"use strict";

const { loadNativePath, loadWorkerPath, loadCodexProWorkerPath, loadClaudeCodeProWorkerPath, assertSupportedNode, runChild } = require("./runtime.js");

async function main() {
  const args = process.argv.slice(2);
  if (args[0] === "install" || args[0] === "setup") {
    return require("../npm/install.js").main(args.slice(1));
  }
  assertSupportedNode();

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
  return runChild(loadNativePath(), args, { env });
}

main().then(code => { process.exitCode = code; }).catch(error => {
  console.error(`flywheel: ${error.message}`);
  process.exitCode = 1;
});
