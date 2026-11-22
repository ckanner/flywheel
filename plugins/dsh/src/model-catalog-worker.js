#!/usr/bin/env node
import { modelCatalog } from './model-catalog.js';

const controller = new AbortController();
for (const signal of ['SIGTERM', 'SIGINT', 'SIGHUP']) process.once(signal, () => controller.abort());
process.stdout.on('error', () => controller.abort());
const timer = setTimeout(() => { controller.abort(); process.stdin.destroy(); }, 12000);
try {
  if (process.argv.length !== 2) throw new Error('invalid arguments');
  const chunks = [];
  let size = 0;
  for await (const chunk of process.stdin) {
    size += chunk.length;
    if (size > 8192) throw new Error('input limit');
    chunks.push(chunk);
  }
  const input = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(Buffer.concat(chunks)));
  const result = await modelCatalog(input, { signal: controller.signal });
  process.stdout.write(JSON.stringify(result) + '\n');
} catch {
  // No untrusted input, native stderr, credentials or exception details escape.
  process.stdout.write(JSON.stringify({ error: { code: 'INVALID_REQUEST', message: 'Expected one bounded model catalog request.' } }) + '\n');
  process.exitCode = 1;
} finally { clearTimeout(timer); }
