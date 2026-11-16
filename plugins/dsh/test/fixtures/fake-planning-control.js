#!/usr/bin/env node
import { appendFile } from 'node:fs/promises';
import { planningState, planningTask } from '../support/planning-fixture.js';
const args = process.argv.slice(2), action = args[1];
const option = key => args[args.indexOf(key) + 1];
let input = '';
if (action === 'planning') for await (const chunk of process.stdin) input += chunk;
const command = input ? JSON.parse(input) : undefined;
if (process.env.TEST_PLANNING_LOG) await appendFile(process.env.TEST_PLANNING_LOG, JSON.stringify({ args, command, proof: process.env.FLYWHEEL_PLANNING_USER_ACTION }) + '\n');
process.stdout.write(JSON.stringify({ protocol: 1, ok: true, taskDir: option('--task-dir'), cwd: option('--cwd'), running: false, state: planningTask(planningState()), ...(command ? { 'planning-control': { 'request-id': command['request-id'] ?? '', status: 'applied' } } : {}) }));
