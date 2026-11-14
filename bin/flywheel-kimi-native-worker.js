#!/usr/bin/env node
"use strict";
const { loadKimiNativeWorkerPath, runChild } = require('./runtime.js');
runChild(process.execPath, [loadKimiNativeWorkerPath(), ...process.argv.slice(2)], { label: 'flywheel-kimi-native-worker' })
  .then(code => { process.exitCode = code; }).catch(error => { console.error(`flywheel-kimi-native-worker: ${error.message}`); process.exitCode = 1; });
