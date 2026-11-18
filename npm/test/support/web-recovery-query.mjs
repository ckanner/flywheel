// Copied into the isolated consumer directory; imports no source fixture or SDK.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';

const { modulePath, query } = JSON.parse(readFileSync(0, 'utf8'));
const consumerRequire = createRequire(modulePath);
for (const specifier of ['dsh-plugin-chatgpt', 'dsh-plugin-chatgpt/web-recovery', '@deepseek-ai/dsh-sdk-jsonrpc-server']) {
  assert.throws(() => consumerRequire.resolve(specifier), { code: 'MODULE_NOT_FOUND' },
    specifier + ' must not be available in the installed consumer');
}
try {
  const { recoverWebWorker } = await import(pathToFileURL(modulePath));
  const result = await recoverWebWorker(query);
  process.stdout.write(JSON.stringify({ ok: true, result }));
} catch (error) {
  process.stdout.write(JSON.stringify({ ok: false, name: error.name, code: error.code, message: error.message }));
}
