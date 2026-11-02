#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: UNLICENSED
"use strict";

const { loadWorkerPath, runChild } = require("./runtime.js");

async function main() {
  return runChild(process.execPath, [loadWorkerPath(), ...process.argv.slice(2)], {
    label: "flywheel-dsh-worker",
  });
}

main().then(code => { process.exitCode = code; }).catch(error => {
  console.error(`flywheel-dsh-worker: ${error.message}`);
  process.exitCode = 1;
});
