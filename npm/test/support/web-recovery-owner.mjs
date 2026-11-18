// This original owner must exit before the installed read-only reader runs.
import { readFileSync } from 'node:fs';
import { runWorker } from '../../../plugins/dsh/src/bridge.js';

const { request, env } = JSON.parse(readFileSync(0, 'utf8'));
const frames = [];
let stderr = '';
const result = await runWorker(request, { env, emit: frame => frames.push(frame), stderr: text => { stderr += text; } });
process.stdout.write(JSON.stringify({ result, frames, stderr }));
