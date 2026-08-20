import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { readFileSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { makeFixture } from './helpers.js';

const BIN = join(dirname(fileURLToPath(import.meta.url)), '..', 'bin', 'flywheel.js');

function cli(args, cwd) {
  return new Promise((resolve) => {
    const child = spawn('node', [BIN, ...args], { cwd, stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (d) => { stdout += d; });
    child.stderr.on('data', (d) => { stderr += d; });
    child.on('exit', (exitCode) => resolve({ exitCode, stdout, stderr }));
  });
}

test('e2e: flywheel intake scans the command source, dedups, triages into triage-inbox.md', async () => {
  const fx = makeFixture();
  const run = await cli(['intake', `docs/${fx.slug}`], fx.root);
  assert.equal(run.exitCode, 0, run.stderr);
  const summary = JSON.parse(run.stdout.slice(run.stdout.indexOf('{')));
  assert.equal(summary.new, 2);
  assert.equal(summary.triaged.recommend, 2);

  const inbox = readFileSync(join(fx.taskDir, 'triage-inbox.md'), 'utf8');
  assert.match(inbox, /nightly CI red on main/);
  assert.match(inbox, /- triage: recommend/);
  assert.match(inbox, /- approval: pending/);

  // second pass: everything deduped
  const again = await cli(['intake', `docs/${fx.slug}`], fx.root);
  const second = JSON.parse(again.stdout.slice(again.stdout.indexOf('{')));
  assert.equal(second.new, 0);
  assert.equal(second.duplicates, 2);

  // single-source flag works
  const single = await cli(['intake', `docs/${fx.slug}`, '--source', 'ci-failures'], fx.root);
  assert.equal(single.exitCode, 0, single.stderr);
});

test('e2e: flywheel garden writes garden-proposals.md with evidence citations', async () => {
  const fx = makeFixture();
  const run = await cli(['garden', `docs/${fx.slug}`], fx.root);
  assert.equal(run.exitCode, 0, run.stderr);
  const summary = JSON.parse(run.stdout.slice(run.stdout.indexOf('{')));
  assert.equal(summary.ok, true);
  assert.ok(summary.proposals >= 1);
  const content = readFileSync(join(fx.taskDir, 'garden-proposals.md'), 'utf8');
  assert.match(content, /Garden proposals/);
  assert.match(content, /- evidence: retro\.md/);
  assert.ok(existsSync(join(fx.taskDir, 'metrics.jsonl'))); // spawn recorded
});
