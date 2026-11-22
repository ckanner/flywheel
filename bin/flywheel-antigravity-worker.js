#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
"use strict";

const { loadAntigravityWorkerPath, runChild } = require("./runtime.js");

async function main() {
  return runChild(process.execPath, [loadAntigravityWorkerPath(), ...process.argv.slice(2)], {
    label: "flywheel-antigravity-worker",
  });
}

main().then(code => { process.exitCode = code; }).catch(error => {
  console.error(`flywheel-antigravity-worker: ${error.message}`);
  process.exitCode = 1;
});
