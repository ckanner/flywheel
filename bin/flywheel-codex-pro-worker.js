#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
"use strict";

const { loadCodexProWorkerPath, runChild } = require("./runtime.js");

async function main() {
  return runChild(process.execPath, [loadCodexProWorkerPath(), ...process.argv.slice(2)], {
    label: "flywheel-codex-pro-worker",
  });
}

main().then(code => { process.exitCode = code; }).catch(error => {
  console.error(`flywheel-codex-pro-worker: ${error.message}`);
  process.exitCode = 1;
});
