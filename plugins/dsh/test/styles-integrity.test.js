import test from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';

const CLIENT_ROOT = resolve(import.meta.dirname, '../src/client');
const CSS_PATH = join(CLIENT_ROOT, 'styles/flywheel.module.css');
const COMPILED_CLIENT_PATH = resolve(import.meta.dirname, '../dist/client.js');

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

test('Running Orbiting Light-Ring & Unified Packet Shell Guard: compiled CSS preserves @property/@keyframes and forbids animation-killing !important', () => {
  const cssSource = readFileSync(CSS_PATH, 'utf8');

  // 1. Source CSS must not use !important on background/background-position inside .runningFlowCard
  //    (in CSS Cascade, author !important overrides @keyframes animations and freezes them).
  const runningBlockMatch = cssSource.match(/\.runningFlowCard[\s\S]*?\}/);
  assert.ok(runningBlockMatch, 'expected .runningFlowCard rule in flywheel.module.css');
  assert.doesNotMatch(
    runningBlockMatch[0],
    /background(?:-[a-z-]+)?\s*:[^;]*!important/i,
    'runningFlowCard must never use !important on background properties (kills CSS animations)',
  );

  // 2. .packetTimelineRow must live inside .packetCardShell without its own outer border
  //    (prevents top-half-blue / bottom-half-gray split borders on running packet cards).
  const timelineBlockMatch = cssSource.match(/\.packetTimelineRow\s*\{[\s\S]*?\}/);
  assert.ok(timelineBlockMatch, 'expected .packetTimelineRow rule in flywheel.module.css');
  assert.doesNotMatch(
    timelineBlockMatch[0],
    /(?:^|\n)\s*border\s*:/,
    'packetTimelineRow must not define an independent outer border; .packetCardShell owns the unified frame',
  );

  // 3. Compiled dist/client.js must retain @property --fw-ring-angle, @keyframes fwRingSpin, and conic-gradient.
  const compiled = readFileSync(COMPILED_CLIENT_PATH, 'utf8');
  assert.match(compiled, /@property --fw-ring-angle\{/, 'compiled bundle must include @property --fw-ring-angle');
  assert.match(compiled, /@keyframes flywheel_fwRingSpin_/, 'compiled bundle must include scoped @keyframes fwRingSpin');
  assert.match(compiled, /conic-gradient\(from var\(--fw-ring-angle\)/, 'compiled bundle must animate conic-gradient from --fw-ring-angle');
  assert.doesNotMatch(
    compiled,
    /flywheel_runningFlowCard_[A-Za-z0-9_-]+\{[^}]*background[^;]*!important/,
    'compiled runningFlowCard rule must not contain background !important',
  );
});
