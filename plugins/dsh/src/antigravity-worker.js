#!/usr/bin/env node
import * as adapter from './antigravity-bridge.js';
import { runMachineWorker } from './machine-worker.js';
await runMachineWorker({ ...adapter, label: 'Antigravity', commandName: 'flywheel-antigravity-worker', attestAdmission: true });
