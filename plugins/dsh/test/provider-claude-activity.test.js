import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';
import { chmod, copyFile, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { basename, join } from 'node:path';
import { createInterface } from 'node:readline';
import { fileURLToPath } from 'node:url';
import { ADAPTER, LIMITS } from '../src/claude-code-pro-bridge.js';
import { validProviderActivity } from '../src/machine-worker.js';

const require = createRequire(import.meta.url);
let installed = false;
try { require.resolve('dsh-subagent-claude-code-pro/package.json'); installed = true; }
catch (error) { if (error.code !== 'MODULE_NOT_FOUND') throw error; }
const live = { skip: installed ? false : 'install the pinned public Claude Code Pro 0.5.3 candidate', timeout: 20000 };
const worker = fileURLToPath(new URL('../dist/claude-code-pro-worker.js', import.meta.url));
const fixture = fileURLToPath(new URL('./fixtures/fake-claude-code-pro.js', import.meta.url));
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

async function setup(t, scenario = 'ok', durationMs = 1400) {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-claude-activity-')));
  const executable = join(cwd, 'claude-offline.mjs');
  await copyFile(fixture, executable); await chmod(executable, 0o755);
  await writeFile(join(cwd, 'claude-fixture.json'), JSON.stringify({ scenario, durationMs }));
  const env = { ...process.env, FLYWHEEL_CLAUDE_BIN: executable, CLAUDE_CONFIG_DIR: join(cwd, 'claude-config') };
  for (const key of ['NODE_OPTIONS', 'NODE_PATH', 'ANTHROPIC_API_KEY', 'ANTHROPIC_AUTH_TOKEN', 'ANTHROPIC_BASE_URL', 'CLAUDE_CODE_OAUTH_TOKEN']) delete env[key];
  const request = { type: 'start', protocol: 3, requestId: 'claude-activity', cwd, model: 'fixture-model',
    reasoningEffort: 'high', permissionMode: 'danger-full-access', prompt: 'Offline activity validation',
    threadMode: 'ephemeral', recoveryDirectory: join(cwd, 'journal'), timeoutMs: 10000, idleTimeoutMs: 800 };
  const runs = [];
  const launch = (input = request) => {
    const child = spawn(process.execPath, [worker], { cwd, env, stdio: ['pipe', 'pipe', 'pipe'] });
    const run = { child, frames: [], stderr: '', closed: false };
    child.stderr.setEncoding('utf8'); child.stderr.on('data', text => { run.stderr += text; });
    child.stdin.on('error', () => {});
    createInterface({ input: child.stdout }).on('line', line => {
      const frame = JSON.parse(line); run.frames.push(frame);
      if (frame.type === 'lifecycle') child.stdin.write(JSON.stringify({ protocol: 3, type: 'ack',
        requestId: input.requestId, runId: frame.lifecycle.runId, sequence: frame.lifecycle.sequence }) + '\n');
    });
    run.done = new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('close', (code, signal) => { run.closed = true; resolve({ ...run, code, signal }); });
    });
    child.stdin.write(JSON.stringify(input) + '\n'); runs.push(run);
    return run;
  };
  const records = async (name = 'claude-fixture.jsonl') => (await readFile(join(cwd, name), 'utf8').catch(() => '')).trim().split('\n').filter(Boolean).map(JSON.parse);
  t.after(async () => {
    for (const run of runs) {
      if (!run.closed) run.child.kill('SIGTERM');
      let timer;
      await Promise.race([run.done, new Promise(resolve => { timer = setTimeout(resolve, 10000); })]); clearTimeout(timer);
      if (!run.closed) run.child.kill('SIGKILL');
      await run.done;
    }
    await rm(cwd, { recursive: true, force: true });
  });
  return { cwd, request, launch, records };
}

function completed(run) {
  assert.equal(run.code, 0, run.stderr + JSON.stringify(run.frames.at(-1)));
  assert.equal(run.signal, null);
  assert.equal(run.frames.filter(f => f.type === 'result').length, 1);
  const frame = run.frames.at(-1);
  assert.equal(frame.type, 'result'); assert.deepEqual(frame.adapter, ADAPTER);
  assert.equal(frame.error, null); assert.equal(frame.receipt.ok, true);
  assert.equal(frame.receipt.cleanup.confirmed, true);
  assert.equal(frame.receipt.execution.truncated, false);
  assert.equal(frame.receipt.execution.outcome, 'completed');
  return frame.receipt;
}

for (const scenario of ['activity-root-tools', 'activity-root-thinking', 'activity-child-thinking', 'activity-native-progress']) {
  test(`installed Claude worker forwards ${scenario} through both owned supervisors`, live, async t => {
    const f = await setup(t, scenario);
    const input = { ...f.request, ...(scenario === 'activity-native-progress' ? { native: { version: 1, agents: { enabled: true } } } : {}) };
    const run = await f.launch(input).done, receipt = completed(run);
    const activity = run.frames.filter(frame => frame.type === 'activity');
    assert.ok(activity.length >= 4, JSON.stringify(activity));
    assert.ok(activity.length < 25, 'metadata is coalesced across the entire owned tree');
    for (let i = 0; i < activity.length; i++) {
      const frame = activity[i];
      assert.equal(frame.protocol, 3); assert.equal(frame.requestId, input.requestId); assert.deepEqual(frame.adapter, ADAPTER);
      assert.ok(validProviderActivity(frame.activity, 'claude-code-pro', i ? activity[i - 1].activity.sequence : 0));
    }
    const kind = scenario === 'activity-root-tools' ? 'tool-result' : scenario === 'activity-native-progress' ? 'task-progress' : 'stream-delta';
    assert.ok(activity.filter(frame => frame.activity.eventTypes.includes(kind)).length >= 3);
    if (scenario.includes('child') || scenario.includes('native')) assert.ok(activity.some(frame => frame.activity.scope === 'child'));
    assert.equal(JSON.stringify(activity).includes('PRIVATE_PROVIDER_BODY'), false);
    assert.equal(JSON.stringify(activity).includes('tool_use_id'), false);
    assert.ok((await f.records('claude-fixture-activity.jsonl')).length >= 10);
    const [record] = await f.records();
    for (const flag of ['--include-partial-messages', '--forward-subagent-text']) assert.ok(record.argv.includes(flag));
    assert.ok(run.frames.some(frame => frame.type === 'lifecycle' && frame.lifecycle.event === 'cleanup-confirmed'));
    if (scenario === 'activity-native-progress') {
      assert.equal(receipt.native.observed.events.filter(event => event.kind === 'task-progress').length, 1,
        'the durable state ledger still deduplicates unchanged child state');
    } else assert.equal(receipt.native, undefined, 'activity does not require opting into native policy or fabricated child receipts');
    if (scenario === 'activity-root-tools') {
      const recovered = await f.launch({ protocol: 3, type: 'recover', requestId: input.requestId, runId: receipt.runId,
        recoveryDirectory: input.recoveryDirectory, original: { cwd: input.cwd, model: input.model,
          reasoningEffort: input.reasoningEffort, permissionMode: input.permissionMode, threadMode: input.threadMode } }).done;
      assert.equal(recovered.code, 0, recovered.stderr + JSON.stringify(recovered.frames));
      assert.deepEqual(recovered.frames.at(-1).recovery.receipt, receipt);
      assert.equal((await f.records()).length, 1, 'read-only recovery must not dispatch another CLI query');
    }
  });
}

test('Claude display exceeding 1 MiB is omitted without cancelling useful work', live, async t => {
  const f = await setup(t, 'activity-display-overflow'), run = await f.launch().done;
  completed(run);
  const progress = run.frames.filter(frame => frame.type === 'progress');
  assert.ok(progress.some(frame => frame.text.includes('Further display progress omitted')));
  assert.ok(progress.reduce((sum, frame) => sum + Buffer.byteLength(JSON.stringify(frame) + '\n'), 0) <= LIMITS.progressBytes + 1024);
  assert.ok(run.frames.filter(frame => frame.type === 'activity').length >= 4);
});

test('a single oversized Claude frame fails explicitly and preserves confirmed owned cleanup', live, async t => {
  const f = await setup(t, 'oversized-frame'), run = await f.launch().done;
  assert.equal(run.code, 1);
  const receipt = run.frames.at(-1).receipt;
  assert.equal(receipt.ok, false); assert.equal(receipt.execution.truncated, true);
  assert.equal(receipt.execution.outcome, 'unknown'); assert.equal(receipt.cleanup.confirmed, true);
  assert.equal(receipt.failure.category, 'protocol');
  assert.throws(() => process.kill(-receipt.execution.processGroupId, 0), { code: 'ESRCH' });
});

test('silent Claude emits no timer heartbeat and its timeout receipt retains cleanup', live, async t => {
  const f = await setup(t, 'hang');
  const run = await f.launch({ ...f.request, timeoutMs: 1800 }).done;
  assert.equal(run.code, 1);
  const activity = run.frames.filter(frame => frame.type === 'activity');
  assert.equal(activity.length, 1);
  assert.deepEqual(activity[0].activity.eventTypes, ['session-init']);
  const receipt = run.frames.at(-1).receipt;
  assert.equal(receipt.ok, false); assert.equal(receipt.execution.timedOut, true);
  assert.equal(receipt.execution.outcome, 'unknown'); assert.equal(receipt.cleanup.confirmed, true);
});

test('cancellation during child-only Claude activity stops the real owned CLI', live, async t => {
  const f = await setup(t, 'activity-child-thinking', 5000), running = f.launch();
  const deadline = Date.now() + 5000;
  while (running.frames.filter(frame => frame.type === 'activity' && frame.activity.scope === 'child').length < 2) {
    assert.ok(Date.now() < deadline, 'child observations did not reach the real worker');
    await sleep(20);
  }
  running.child.kill('SIGTERM');
  const run = await running.done, receipt = run.frames.at(-1).receipt;
  assert.equal(run.code, 143);
  assert.equal(receipt.execution.cancelled, true); assert.equal(receipt.execution.timedOut, false);
  assert.equal(receipt.execution.outcome, 'unknown'); assert.equal(receipt.cleanup.confirmed, true);
  assert.throws(() => process.kill(-receipt.execution.processGroupId, 0), { code: 'ESRCH' });
});

test('unsupported child streaming fails before inference rather than running with false liveness', live, async t => {
  const f = await setup(t, 'unsupported-child-stream'), run = await f.launch().done;
  assert.equal(run.code, 1);
  assert.equal(run.frames.at(-1).receipt.execution.dispatch, 'not-dispatched');
  assert.equal(run.frames.at(-1).receipt.cleanup.confirmed, true);
  assert.deepEqual(await f.records(), []);
  assert.equal(run.frames.filter(frame => frame.type === 'activity').length, 0);
});

test('fresh Claude titles are readable, private prompts stay out, and resume keeps a user name', live, async t => {
  const f = await setup(t);
  const input = { ...f.request, protocol: 2, threadMode: 'persistent', sessionTitle: 'Flywheel · Alerts · Design review' };
  delete input.recoveryDirectory;
  const first = completed(await f.launch(input).done);
  const history = join(f.cwd, 'claude-history-' + first.session.id + '.json');
  const stored = JSON.parse(await readFile(history, 'utf8')); stored.name = 'User named investigation';
  await writeFile(history, JSON.stringify(stored));
  completed(await f.launch({ ...input, requestId: 'resume', threadId: first.session.id, sessionTitle: 'Must not replace user title' }).done);
  assert.equal(JSON.parse(await readFile(history, 'utf8')).name, stored.name);
  completed(await f.launch({ ...f.request, requestId: 'ephemeral', sessionTitle: 'Ephemeral stays ephemeral' }).done);
  const fallback = { ...input, requestId: 'fallback', prompt: 'PRIVATE_PROMPT_CONTENT must not be a title' }; delete fallback.sessionTitle;
  completed(await f.launch(fallback).done);
  const records = await f.records();
  const name = record => record.argv.includes('-n') ? record.argv[record.argv.indexOf('-n') + 1] : undefined;
  assert.deepEqual(records.map(name), [input.sessionTitle, undefined, undefined, 'Claude · ' + basename(f.cwd)]);
  assert.ok(records[2].argv.includes('--no-session-persistence'));
});
