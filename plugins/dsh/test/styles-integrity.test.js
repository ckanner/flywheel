import test from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';

const CLIENT_ROOT = resolve(import.meta.dirname, '../src/client');
const CSS_PATH = join(CLIENT_ROOT, 'styles/flywheel.module.css');

function walkSourceFiles(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      out.push(...walkSourceFiles(full));
    } else if (full.endsWith('.ts') || full.endsWith('.tsx')) {
      out.push(full);
    }
  }
  return out;
}

test('CSS Module Static Integrity Guard (PILLAR-5.3): every styles.<className> in src/client/** is defined in flywheel.module.css', () => {
  const cssText = readFileSync(CSS_PATH, 'utf8');
  const defined = new Set();
  for (const match of cssText.matchAll(/\.([A-Za-z_][A-Za-z0-9_-]*)/g)) {
    defined.add(match[1]);
  }

  const missing = [];
  for (const file of walkSourceFiles(CLIENT_ROOT)) {
    const content = readFileSync(file, 'utf8');
    for (const match of content.matchAll(/\bstyles\.([A-Za-z_][A-Za-z0-9_-]*)/g)) {
      const cls = match[1];
      if (!defined.has(cls)) {
        missing.push(`${cls} (${file.replace(`${CLIENT_ROOT}/`, '')})`);
      }
    }
  }

  assert.deepEqual(
    missing,
    [],
    `Undefined CSS classes referenced in client TSX: ${missing.join(', ')}`,
  );
});
