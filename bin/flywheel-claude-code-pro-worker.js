#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
"use strict";

const { loadClaudeCodeProWorkerPath, runChild } = require("./runtime.js");

async function main() {
  return runChild(process.execPath, [loadClaudeCodeProWorkerPath(), ...process.argv.slice(2)], {
    label: "flywheel-claude-code-pro-worker",
  });
}

main().then(code => { process.exitCode = code; }).catch(error => {
  console.error(`flywheel-claude-code-pro-worker: ${error.message}`);
  process.exitCode = 1;
});
