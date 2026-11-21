#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: MIT
"use strict";

const { loadModelCatalogWorkerPath, assertSupportedNode, runChild } = require("./runtime.js");

async function main() {
  assertSupportedNode();
  return runChild(process.execPath, [loadModelCatalogWorkerPath(), ...process.argv.slice(2)], {
    label: "flywheel-model-catalog",
  });
}

main().then(code => { process.exitCode = code; }).catch(error => {
  console.error(`flywheel-model-catalog: ${error.message}`);
  process.exitCode = 1;
});
