#!/usr/bin/env node
import { runWorker } from './bridge.js';
import { BridgeError, MAX_FRAME_BYTES, normalizedError } from './protocol.js';

const controller = new AbortController();
process.once('SIGTERM', () => controller.abort());
process.once('SIGINT', () => controller.abort());
// The engine writes exactly one request then closes stdin. EOF after admission
// is normal and must not cancel an in-flight model turn.
async function readInput() {
  const chunks = [];
  let bytes = 0;
  const admission = setTimeout(() => process.stdin.destroy(new BridgeError('TIMEOUT', 'no complete worker request within 10 seconds')), 10000);
  try {
    for await (const chunk of process.stdin) {
      bytes += chunk.length;
      if (bytes > MAX_FRAME_BYTES) throw new BridgeError('INVALID_REQUEST', 'worker request exceeds 8 MiB');
      chunks.push(chunk);
    }
    const lines = Buffer.concat(chunks).toString('utf8').trim().split('\n');
    if (lines.length !== 1 || !lines[0]) throw new BridgeError('INVALID_REQUEST', 'expected exactly one JSON line then EOF');
    try { return JSON.parse(lines[0]); } catch {
      throw new BridgeError('INVALID_REQUEST', 'worker request is not valid JSON');
    }
  } finally {
    clearTimeout(admission);
  }
}

const emit = value => process.stdout.write(JSON.stringify(value) + '\n');
try {
  if (process.argv.slice(2).some(arg => arg !== '--doctor') || process.argv.length > 3) {
    throw new BridgeError('INVALID_REQUEST', 'worker accepts only optional --doctor');
  }
  const request = await readInput();
  const result = await runWorker(request, {
    emit, signal: controller.signal, doctor: process.argv.includes('--doctor'),
    stderr: chunk => process.stderr.write(chunk),
  });
  process.exitCode = result.status === 'ok' ? 0 : result.status === 'cancelled' ? 143 : 1;
} catch (error) {
  emit({ protocol: 1, requestId: 'invalid-request', type: 'result', status: 'error', settled: true,
    text: '', usage: null, costUsd: null, error: normalizedError(error) });
  process.exitCode = 1;
}
