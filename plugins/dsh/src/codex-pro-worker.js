#!/usr/bin/env node
import * as adapter from './codex-pro-bridge.js';
import { runMachineWorker } from './machine-worker.js';

await runMachineWorker({ ...adapter, label: 'Codex Pro', commandName: 'flywheel-codex-pro-worker' });
