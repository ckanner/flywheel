#!/usr/bin/env node
import { spawn } from 'node:child_process';
import { runKimiWorker } from './kimi-native-bridge.js';
import { prepareKimiPrintSession } from './kimi-print-session.js';

const controller = new AbortController();
let finished = false, inputError, request = { requestId: 'invalid-request' }, outputError;
const cancel = () => { if (!finished) controller.abort(); };
for (const name of ['SIGTERM', 'SIGINT', 'SIGHUP']) process.on(name, cancel);
process.stdout.on('error', () => { outputError = true; cancel(); });

function write(frame) {
  const line = JSON.stringify(frame) + '\n';
  if (Buffer.byteLength(line) > 2 * 1024 * 1024) throw new Error('Kimi native receipt exceeds the output frame limit');
  if (!outputError) process.stdout.write(line);
}

async function doctor() {
  const child = spawn(process.env.FLYWHEEL_KIMI_BIN || 'kimi', ['--version'], { stdio: ['ignore', 'pipe', 'pipe'], detached: false });
  let output = '', bytes = 0;
  child.stdout.on('data', chunk => { bytes += chunk.length; if (bytes < 4096) output += chunk; });
  child.stderr.on('data', () => {});
  const timer = setTimeout(() => child.kill('SIGKILL'), 5000);
  const ok = await new Promise(resolve => { child.once('error', () => resolve(false)); child.once('exit', code => resolve(code === 0)); });
  clearTimeout(timer);
  const version = output.match(/\b\d+\.\d+\.\d+(?:[-+][\w.-]+)?\b/)?.[0] || null;
  write({ type: 'doctor', protocol: 1, available: ok && version !== null, cliVersion: version,
    backendProtocol: 'kimi-server/v1', inferenceTested: false, nativeAdmission: 'requires-actual-server-schema-and-effective-session-probe', recoverySupported: false });
  process.exitCode = ok && version ? 0 : 1;
}

async function input() {
  return new Promise((resolve, reject) => {
    let buffer = Buffer.alloc(0), admitted = false;
    const fail = message => { inputError ||= message; clearTimeout(timer); controller.abort(); reject(new Error(message)); };
    const timer = setTimeout(() => fail('No complete Kimi native request arrived within 10 seconds'), 10_000);
    process.stdin.on('data', chunk => {
      if (finished) return;
      if (admitted) { if (chunk.toString('utf8').trim()) fail('Only one start request is accepted'); return; }
      buffer = Buffer.concat([buffer, chunk]);
      if (buffer.length > 1024 * 1024) { fail('Kimi native request exceeds the 1 MiB limit'); return; }
      const end = buffer.indexOf(10);
      if (end < 0) return;
      try {
        request = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(buffer.subarray(0, end)));
        if (buffer.subarray(end + 1).toString('utf8').trim()) throw new Error();
        admitted = true; clearTimeout(timer); resolve(request);
      } catch { fail('Expected one valid UTF-8 JSON start request'); }
    });
    process.stdin.on('end', () => { if (!admitted) fail('Parent input closed before a start request'); else cancel(); });
    process.stdin.on('error', () => fail('Parent input failed'));
  });
}

try {
  if (process.argv.includes('--doctor')) await doctor();
  else {
    let value;
    try { value = await input(); } catch { value = request; }
    const run = value?.type === 'prepare-print' ? prepareKimiPrintSession : runKimiWorker;
    const result = await run(value, { emit: write, signal: controller.signal,
      stderr: message => process.stderr.write('[Kimi native] ' + message) });
    if (inputError) {
      result.status = 'error'; result.error = { code: 'INPUT_ERROR', message: inputError };
      if (result.lifecycle) result.lifecycle.resumeCandidate = false;
    }
    finished = true;
    write(result);
    process.exitCode = result.status === 'ok' ? 0 : result.status === 'cancelled' ? 130 : 1;
  }
} catch {
  // A broken consumer/oversized receipt must fail the outer transport gate;
  // never emit a second plausible final after partial output.
  process.exitCode = 1;
} finally {
  finished = true;
  process.stdin.pause();
  for (const name of ['SIGTERM', 'SIGINT', 'SIGHUP']) process.removeListener(name, cancel);
}
