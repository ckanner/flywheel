#!/usr/bin/env node
import { appendFileSync } from 'node:fs';
const args = process.argv.slice(2);
if (process.env.TEST_CONTROL_LOG) appendFileSync(process.env.TEST_CONTROL_LOG, JSON.stringify(args) + '\n');
const taskDir = args[args.indexOf('--task-dir') + 1];
process.stdout.write(JSON.stringify({ protocol: 1, ok: true, running: args[1] === 'start', pid: 42,
  taskDir, state: { phase: 'execute', model: 'deepseek-flash', unit: 'u1' } }) + '\n');
