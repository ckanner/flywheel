import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, existsSync, mkdtempSync, mkdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { extractFailurePatterns, validateGardenProposals, runGarden } from '../orchestrator/modules/garden.js';
import { loadConfig } from '../orchestrator/core/config.js';
import { createDrivers } from '../orchestrator/drivers/index.js';
import { makeFixture } from './helpers.js';

const noSleep = async () => {};
const noLog = () => {};

const RETRO = `
- 2026-08-19T01:00:00Z packet M01-T01: completed (attempts=1, failed=0, commit abc12345)
- 2026-08-19T02:00:00Z packet M01-T02: INBOX — packet M01-T02 exhausted retries (3 per round, plan revisions: 1). Human needed.
- 2026-08-19T03:00:00Z packet M01-T03: INBOX — packet M01-T03 exhausted retries (3 per round, plan revisions: 1). Human needed.
- 2026-08-19T04:00:00Z milestone M-01: closed (gate pass, judge 1)
- 2026-08-19T05:00:00Z packet M02-T01: escalated to planner, revision 1
`;

test('extractFailurePatterns: normalized patterns repeating >= 2 times', () => {
  const patterns = extractFailurePatterns(RETRO);
  assert.equal(patterns.length, 1);
  assert.equal(patterns[0].count, 2);
  assert.match(patterns[0].pattern, /INBOX.*exhausted retries/);
  assert.equal(patterns[0].lines.length, 2); // evidence lines preserved
  assert.equal(extractFailurePatterns('- all good here\n').length, 0);
});

test('validateGardenProposals: evidence citation is mandatory', () => {
  const good = [
    '## GP-01 — Harden retries',
    '- type: failure-pattern',
    '- evidence: retro.md:"packet M01-T02: INBOX — transport" (x2)',
    '- proposal: add a retry-hardening skill',
  ].join('\n');
  assert.equal(validateGardenProposals(good).ok, true);

  const noEvidence = '## GP-01 — Vague\n- type: decay\n- proposal: clean something\n';
  const r1 = validateGardenProposals(noEvidence);
  assert.equal(r1.ok, false);
  assert.match(r1.errors[0], /missing evidence/);

  const badCitation = '## GP-01 — Weak cite\n- type: decay\n- evidence: "something I remember"\n- proposal: x\n';
  assert.equal(validateGardenProposals(badCitation).ok, false);

  assert.equal(validateGardenProposals('no proposals at all').ok, false);
});

test('runGarden (mock kimi): proposals file written with evidence-carrying proposals', async () => {
  const fx = makeFixture();
  writeFileSync(join(fx.taskDir, 'retro.md'), RETRO);
  const config = loadConfig(fx.taskDir, fx.root);
  const drivers = createDrivers(config);
  const result = await runGarden({ taskDir: fx.taskDir, cwd: fx.root, config, drivers, log: noLog, sleep: noSleep });
  assert.equal(result.ok, true);
  assert.equal(result.proposals, 1);
  const content = readFileSync(result.file, 'utf8');
  assert.match(content, /Garden proposals/);
  assert.match(content, /HUMAN reviews and applies/); // proposals-only banner
  assert.match(content, /- evidence: retro\.md/);
  // Garden must never edit knowledge assets itself.
  assert.equal(existsSync(join(fx.root, 'AGENTS.md')), false);
});

test('runGarden: proposals failing evidence validation still land raw, but flagged not-ok', async () => {
  const fx = makeFixture();
  const config = loadConfig(fx.taskDir, fx.root);
  const drivers = createDrivers(config);
  drivers.garden = { garden: async () => ({ exitCode: 0, signal: null, timedOut: false, stdout: '', stderr: '', durationMs: 1, harness: 'mock', provider: 'mock', model: 'm', usage: null, costUsd: 0, settled: true, text: '## GP-01 — Evidence-free\n- type: decay\n- proposal: trust me\n' }) };
  const result = await runGarden({ taskDir: fx.taskDir, cwd: fx.root, config, drivers, log: noLog, sleep: noSleep });
  assert.equal(result.ok, false);
  assert.ok(result.errors.length > 0);
  assert.match(readFileSync(result.file, 'utf8'), /Evidence-free/); // raw kept for the human
});

test('runGarden: agent failure does not write a proposals file', async () => {
  const fx = makeFixture();
  const config = loadConfig(fx.taskDir, fx.root);
  const drivers = createDrivers(config);
  drivers.garden = { garden: async () => ({ exitCode: 1, signal: null, timedOut: false, stdout: '', stderr: '401: invalid api key', durationMs: 1, harness: 'mock', provider: 'mock', model: 'm', usage: null, costUsd: 0, settled: false, text: '', errorMessage: '401: invalid api key' }) };
  const result = await runGarden({ taskDir: fx.taskDir, cwd: fx.root, config, drivers, log: noLog, sleep: noSleep });
  assert.equal(result.ok, false);
  assert.equal(result.file, null);
  assert.match(result.errors[0], /AUTH|failed/);
});

const HAS_KIMI = (() => {
  try {
    execFileSync('kimi', ['--version'], { stdio: 'pipe' });
    return true;
  } catch {
    return false;
  }
})();

test('garden smoke (real kimi): tiny garden prompt returns proposal-shaped text', { timeout: 300_000, skip: !HAS_KIMI && 'kimi CLI unavailable' }, async () => {
  const { runKimi } = await import('../orchestrator/drivers/kimi.js');
  const cwd = mkdtempSync(join(tmpdir(), 'flywheel-garden-smoke-'));
  const r = await runKimi({
    prompt: 'Reply with exactly this markdown block and nothing else:\n## GP-01 — test\n- type: decay\n- evidence: retro.md:"x"\n- proposal: test',
    cwd,
    watchdogOpts: { idleTimeoutMs: 120_000, wallTimeoutMs: 280_000, killGraceMs: 3000 },
  });
  console.log('[garden-smoke]', JSON.stringify({ exitCode: r.exitCode, sessionId: r.sessionId, text: r.text.slice(0, 150) }));
  assert.equal(r.exitCode, 0, r.stderr.slice(0, 300));
  const v = validateGardenProposals(r.text);
  assert.ok(v.ok, `validation failed: ${v.errors.join('; ')} — text: ${r.text.slice(0, 300)}`);
});
