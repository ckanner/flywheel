#!/usr/bin/env node
import * as adapter from './claude-code-pro-bridge.js';
import { runMachineWorker } from './machine-worker.js';

await runMachineWorker({ ...adapter, label: 'Claude Code Pro', commandName: 'flywheel-claude-code-pro-worker', attestAdmission: true });
