import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import {
  fetchSourceCandidates, runIntake, parseTriageVerdict, listApprovedCandidates, fingerprint,
} from '../orchestrator/modules/intake.js';
import { loadConfig } from '../orchestrator/core/config.js';
import { createDrivers } from '../orchestrator/drivers/index.js';
import { makeFixture } from './helpers.js';

const noSleep = async () => {};
const noLog = () => {};

function fakeExec(responses) {
  const calls = [];
  const exec = async (cmd, args) => {
    calls.push([cmd, ...args].join(' '));
    for (const [pattern, stdout] of Object.entries(responses)) {
      if (calls[calls.length - 1].includes(pattern)) return { stdout, stderr: '' };
    }
    throw new Error(`unexpected exec: ${calls[calls.length - 1]}`);
  };
  return { calls, exec };
}

test('intake sources: github-issues parses gh JSON output', async () => {
  const { calls, exec } = fakeExec({
    'gh issue list': JSON.stringify([
      { number: 12, title: 'Add health endpoint', body: 'Needed for k8s probes', url: 'https://x/12', labels: [] },
      { number: 13, title: 'Flaky test in auth', body: '', url: 'https://x/13', labels: [] },
    ]),
  });
  const candidates = await fetchSourceCandidates(
    { name: 'repo-issues', type: 'github-issues', repo: 'o/r', labels: ['flywheel-candidate'], limit: 50 },
    { cwd: '/tmp', exec },
  );
  assert.equal(candidates.length, 2);
  assert.equal(candidates[0].title, 'Add health endpoint');
  assert.equal(candidates[0].source, 'repo-issues');
  assert.ok(calls[0].includes('--repo o/r'));
  assert.ok(calls[0].includes('--label flywheel-candidate'));
});

test('intake sources: command source parses JSONL and plain lines', async () => {
  const { exec } = fakeExec({
    '/bin/sh -c ./scan.sh': '{"title": "CI red", "summary": "auth failed", "url": "https://ci/1"}\nplain candidate line\n',
  });
  const candidates = await fetchSourceCandidates(
    { name: 'ci', type: 'command', cmd: './scan.sh' },
    { cwd: '/tmp', exec },
  );
  assert.equal(candidates.length, 2);
  assert.equal(candidates[0].title, 'CI red');
  assert.equal(candidates[0].url, 'https://ci/1');
  assert.equal(candidates[1].title, 'plain candidate line');
  assert.equal(candidates[1].url, null);
});

test('intake sources: unknown type throws', async () => {
  await assert.rejects(() => fetchSourceCandidates({ name: 'x', type: 'webhook' }, { cwd: '/tmp', exec: fakeExec({}).exec }), /unknown intake source type/);
});

test('parseTriageVerdict: recommend/skip/human, garbage tolerated', () => {
  assert.equal(parseTriageVerdict('analysis\n{"verdict":"recommend","reason":"clear oracle"}').verdict.verdict, 'recommend');
  assert.equal(parseTriageVerdict('{"verdict":"skip","reason":"vague"}').verdict.verdict, 'skip');
  assert.equal(parseTriageVerdict('{"verdict":"human","reason":"priority call"}').verdict.verdict, 'human');
  assert.equal(parseTriageVerdict('no json').ok, false);
  assert.equal(parseTriageVerdict('{"verdict":"pass"}').ok, false); // review enums don't leak
});

test('runIntake: scans, dedups by fingerprint, triages, and a second run adds nothing', async () => {
  const fx = makeFixture();
  const config = loadConfig(fx.taskDir, fx.root);
  const drivers = createDrivers(config);
  const opts = { taskDir: fx.taskDir, cwd: fx.root, config, drivers, log: noLog, sleep: noSleep };

  const first = await runIntake(opts);
  assert.equal(first.scanned, 2);
  assert.equal(first.new, 2);
  assert.equal(first.duplicates, 0);
  assert.equal(first.triaged.recommend, 2); // mock driver recommends everything
  const inbox = readFileSync(join(fx.taskDir, 'triage-inbox.md'), 'utf8');
  assert.match(inbox, /nightly CI red on main/);
  assert.match(inbox, /upgrade lodash to 5\.x/);
  assert.match(inbox, /- triage: recommend/);
  assert.match(inbox, /<!-- fp:[0-9a-f]{12} -->/);

  const second = await runIntake(opts);
  assert.equal(second.new, 0);
  assert.equal(second.duplicates, 2);
  const after = readFileSync(join(fx.taskDir, 'triage-inbox.md'), 'utf8');
  assert.equal(after, inbox); // untouched by the duplicate run

  // metrics landed (one spawn per new candidate)
  const metrics = readFileSync(join(fx.taskDir, 'metrics.jsonl'), 'utf8').trim().split('\n');
  assert.ok(metrics.some((l) => JSON.parse(l).role === 'intake'));
});

test('runIntake: --source filters to one configured source', async () => {
  const fx = makeFixture();
  const config = loadConfig(fx.taskDir, fx.root);
  const drivers = createDrivers(config);
  const result = await runIntake({ taskDir: fx.taskDir, cwd: fx.root, config, drivers, source: 'ci-failures', log: noLog, sleep: noSleep });
  assert.equal(result.new, 2);
  await assert.rejects(
    () => runIntake({ taskDir: fx.taskDir, cwd: fx.root, config, drivers, source: 'nope', log: noLog, sleep: noSleep }),
    /unknown intake source/,
  );
});

test('runIntake: a failing source never blocks the others', async () => {
  const fx = makeFixture();
  const cfg = JSON.parse(readFileSync(join(fx.taskDir, 'flywheel.config.json'), 'utf8'));
  cfg.intake.sources.push({ name: 'broken', type: 'command', cmd: 'exit 1' });
  writeFileSync(join(fx.taskDir, 'flywheel.config.json'), JSON.stringify(cfg));
  const config = loadConfig(fx.taskDir, fx.root);
  const drivers = createDrivers(config);
  const result = await runIntake({ taskDir: fx.taskDir, cwd: fx.root, config, drivers, log: noLog, sleep: noSleep });
  assert.equal(result.new, 2); // the good source still produced
});

test('approved flow: listApprovedCandidates returns only human-approved entries', async () => {
  const fx = makeFixture();
  const config = loadConfig(fx.taskDir, fx.root);
  const drivers = createDrivers(config);
  await runIntake({ taskDir: fx.taskDir, cwd: fx.root, config, drivers, log: noLog, sleep: noSleep });

  assert.deepEqual(listApprovedCandidates(fx.taskDir), []);
  const file = join(fx.taskDir, 'triage-inbox.md');
  const content = readFileSync(file, 'utf8');
  const fp = fingerprint('ci-failures', 'nightly CI red on main');
  writeFileSync(file, content.replace(new RegExp(`(## ${fp} —[\\s\\S]*?)- approval: pending`), '$1- approval: approved'));
  const approved = listApprovedCandidates(fx.taskDir);
  assert.equal(approved.length, 1);
  assert.equal(approved[0].fingerprint, fp);
  assert.match(approved[0].title, /nightly CI red/);
});

const HAS_PI = (() => {
  try {
    execFileSync('pi', ['--version'], { stdio: 'pipe' });
    return true;
  } catch {
    return false;
  }
})();
const HAS_KEY = Boolean(process.env.OPENCODE_API_KEY);

test('intake smoke (real pi): one tiny triage prompt parses to a verdict', { timeout: 300_000, skip: (!HAS_PI || !HAS_KEY) && 'pi or OPENCODE_API_KEY unavailable' }, async () => {
  const { runPi } = await import('../orchestrator/drivers/pi.js');
  const { mkdtempSync } = await import('node:fs');
  const { tmpdir } = await import('node:os');
  const cwd = mkdtempSync(join(tmpdir(), 'flywheel-intake-smoke-'));
  const r = await runPi({
    prompt: 'Triage this task candidate. End with ONLY this JSON: {"verdict":"recommend"|"skip"|"human","reason":"..."}\nCandidate: "Add a /health endpoint returning 200 when the DB is reachable, verified by curl exit code."',
    model: 'opencode-go/deepseek-v4-flash',
    thinking: 'max',
    cwd,
    apiKey: process.env.OPENCODE_API_KEY,
    watchdogOpts: { idleTimeoutMs: 120_000, wallTimeoutMs: 280_000, killGraceMs: 3000 },
  });
  console.log('[intake-smoke]', JSON.stringify({ exitCode: r.exitCode, text: r.text.slice(0, 200) }));
  assert.equal(r.exitCode, 0);
  const v = parseTriageVerdict(r.text);
  assert.ok(v.ok, `no verdict parsed from: ${r.text.slice(0, 300)}`);
  assert.ok(['recommend', 'skip', 'human'].includes(v.verdict.verdict));
});
