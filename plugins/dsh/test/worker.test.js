import test from 'node:test';
import assert from 'node:assert/strict';
import { chmod, mkdtemp, readFile, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { makeOverlay, runWorker } from '../src/bridge.js';

const fixture = fileURLToPath(new URL('./fixtures/fake-dsh.js', import.meta.url));
await chmod(fixture, 0o755);

async function setup(t, scenario = 'ok') {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-worker-test-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const request = { type: 'start', protocol: 1, requestId: 'request-1', cwd,
    provider: 'deepseek-official', model: 'deepseek-flash', reasoningEffort: 'high',
    permissionMode: 'workspace-write', prompt: 'Implement the frozen unit.', timeoutMs: 4000 };
  const log = join(cwd, 'requests.jsonl');
  const frames = [];
  const options = { env: { PATH: process.env.PATH, FLYWHEEL_DSH_BIN: fixture,
    TEST_DSH_SCENARIO: scenario, TEST_DSH_LOG: log }, emit: frame => frames.push(frame) };
  const methods = async () => (await readFile(log, 'utf8')).trim().split('\n').map(line => JSON.parse(line).method);
  return { request, options, frames, methods };
}

test('full worker handshake, durable result, usage and SDK shutdown agree with the Go contract', async t => {
  const { request, options, frames, methods } = await setup(t, 'duplicate');
  const result = await runWorker(request, options);
  assert.equal(result.status, 'ok');
  assert.equal(result.settled, true);
  assert.equal(result.text, 'fixture complete');
  assert.equal(result.costUsd, null);
  assert.deepEqual(result.usage, { input_tokens: 120, output_tokens: 8, reasoning_tokens: 3,
    cache_read_tokens: 20, cache_write_tokens: 0, total_tokens: 128 });
  assert.equal(frames[0].type, 'ready');
  assert.equal(frames[0].cwd, await realpath(request.cwd));
  assert.equal(frames[0].approvalPolicy, 'never');
  assert.ok(frames.every(frame => frame.protocol === 1 && frame.requestId === request.requestId));
  assert.equal(frames.at(-1).type, 'result');
  assert.deepEqual(await methods(), ['initialize', 'session/working-directory/get', 'session/prompt', 'session/wait', 'shutdown']);
});

test('an explicit full-access request reaches the SDK with truthful policy and normal shutdown', async t => {
  const { request, options, frames, methods } = await setup(t);
  const result = await runWorker({ ...request, permissionMode: 'danger-full-access' }, options);
  assert.equal(result.status, 'ok');
  assert.equal(result.settled, true);
  assert.equal(frames[0].permissionMode, 'danger-full-access');
  const acknowledgement = frames[0].policy;
  assert.deepEqual(acknowledgement, { protocol: 1, cwd: await realpath(request.cwd),
    permissionMode: 'danger-full-access', approvalPolicy: 'never', confinedShell: false });
  assert.deepEqual(await methods(), ['initialize', 'session/working-directory/get', 'session/prompt', 'session/wait', 'shutdown']);
});

for (const scenario of ['wrong-policy', 'wrong-cwd', 'wrong-approval', 'missing-policy', 'forged-confinement', 'unexpected-enforcement']) {
  test(`full access ${scenario}: no prompt before a matching explicit policy`, async t => {
    const { request, options, methods } = await setup(t, scenario);
    const result = await runWorker({ ...request, permissionMode: 'danger-full-access' }, options);
    assert.equal(result.status, 'error');
    assert.ok(!(await methods()).includes('session/prompt'));
    assert.ok((await methods()).includes('shutdown'));
  });
}

for (const scenario of ['wrong-policy', 'wrong-cwd', 'missing-policy', 'malformed']) {
  test(`${scenario}: fail before submitting a paid prompt`, async t => {
    const { request, options, methods } = await setup(t, scenario);
    const result = await runWorker(request, options);
    assert.equal(result.status, 'error');
    assert.ok(!(await methods()).includes('session/prompt'));
  });
}

for (const scenario of ['empty', 'missing-terminal', 'max-tokens', 'corrupt-tail']) {
  test(`${scenario}: session/wait alone never establishes success`, async t => {
    const { request, options } = await setup(t, scenario);
    const result = await runWorker(request, options);
    assert.equal(result.status, 'error');
  });
}

for (const [scenario, code, evidence, guidance] of [
  ['account-sign-in-required', 'AUTH', 'ACCOUNT_SIGN_IN_REQUIRED', /Sign in to DeepSeek in DSH/],
  ['account-token-invalid', 'AUTH', 'ACCOUNT_TOKEN_INVALID', /Sign in again in DSH/],
  ['account-signed-out', 'AUTH', 'deepseek-account/signed-out', /Sign in again in DSH/],
  ['account-quota', 'QUOTA', 'ACCOUNT_QUOTA', /Check the DeepSeek account balance and usage/],
]) {
  test(`${scenario}: durable SDK failure reaches the worker without rerouting or retrying`, async t => {
    const { request, options, frames, methods } = await setup(t, scenario);
    const result = await runWorker({ ...request, provider: 'deepseek-account' }, options);
    assert.equal(result.status, 'error');
    assert.equal(result.settled, true);
    assert.equal(result.error.code, code);
    assert.ok(result.error.message.includes(evidence));
    assert.match(result.error.message, guidance);
    assert.equal(result.usage, null);
    assert.equal(result.costUsd, null);
    assert.equal(result.text, '');
    assert.equal(frames[0].provider, 'deepseek-account');
    assert.equal(frames.at(-1).error.code, code);
    assert.deepEqual(await methods(), ['initialize', 'session/working-directory/get', 'session/prompt', 'session/wait', 'shutdown']);
  });
}

test('account-like JSON-RPC text without a durable native code remains a transport error', async t => {
  const { request, options, methods } = await setup(t, 'account-code-in-rpc-message');
  const result = await runWorker({ ...request, provider: 'deepseek-account' }, options);
  assert.equal(result.status, 'error');
  assert.equal(result.error.code, 'TRANSPORT');
  assert.equal(result.error.message, 'initialize: ACCOUNT_TOKEN_INVALID: native detail without a durable error code');
  assert.deepEqual(await methods(), ['initialize', 'shutdown']);
});

for (const tail of ['corrupt', 'incomplete']) {
  for (const scenario of ['shutdown', 'account-token-invalid', 'account-quota']) {
    test(`${scenario}: ${tail} SDK shutdown tail overrides the model outcome`, async t => {
      const { request, options, frames, methods } = await setup(t, `${scenario}-${tail}-tail`);
      const result = await runWorker({ ...request, provider: 'deepseek-account' }, options);
      assert.equal(result.status, 'error');
      assert.equal(result.settled, true, 'graceful cleanup remains independent of protocol validity');
      assert.equal(result.error.code, 'TRANSPORT');
      assert.match(result.error.message, tail === 'corrupt' ? /invalid DSH frame/ : /incomplete frame/);
      if (scenario !== 'shutdown') {
        assert.match(result.error.message, /preceding failure: ACCOUNT_(?:TOKEN_INVALID|QUOTA)/);
      }
      assert.equal(frames.at(-1).error.code, 'TRANSPORT');
      assert.deepEqual(await methods(), ['initialize', 'session/working-directory/get', 'session/prompt', 'session/wait', 'shutdown']);
    });
  }

  test(`${tail} SDK shutdown tail does not erase unconfirmed cleanup`, async t => {
    const { request, options } = await setup(t, `account-quota-${tail}-tail-unclean-exit`);
    const result = await runWorker({ ...request, provider: 'deepseek-account' }, options);
    assert.equal(result.status, 'error');
    assert.equal(result.error.code, 'TRANSPORT');
    assert.equal(result.settled, false);
    assert.match(result.error.message, /preceding failure: ACCOUNT_QUOTA/);
  });

  test(`${tail} SDK shutdown tail cannot replace explicit worker cancellation`, async t => {
    const { request, options, methods } = await setup(t, `account-token-invalid-${tail}-tail`);
    const controller = new AbortController();
    const original = options.emit;
    options.emit = frame => {
      original(frame);
      if (frame.event?.type === 'turn/end') controller.abort();
    };
    const result = await runWorker({ ...request, provider: 'deepseek-account' }, { ...options, signal: controller.signal });
    assert.equal(result.status, 'cancelled');
    assert.equal(result.settled, true);
    assert.equal(result.error.code, 'TRANSPORT');
    assert.match(result.error.message, /preceding failure: worker cancelled/);
    assert.deepEqual(await methods(), ['initialize', 'session/working-directory/get', 'session/prompt', 'shutdown']);
  });
}

test('explicit worker cancellation remains cancelled when an account failure arrives', async t => {
  for (const scenario of ['account-token-invalid', 'account-signed-out']) {
    const { request, options, methods } = await setup(t, scenario);
    const controller = new AbortController();
    const original = options.emit;
    let terminal;
    options.emit = frame => {
      original(frame);
      if (frame.event?.type === 'turn/end') {
        terminal = frame.event.data.reason;
        controller.abort();
      }
    };
    const result = await runWorker({ ...request, provider: 'deepseek-account' }, { ...options, signal: controller.signal });
    assert.ok(terminal);
    assert.equal(result.status, 'cancelled');
    assert.equal(result.settled, true);
    assert.equal(result.error.message, 'worker cancelled');
    const requests = await methods();
    assert.equal(requests.filter(method => method === 'session/prompt').length, 1);
    assert.equal(requests.at(-1), 'shutdown');
  }
});

test('unknown provider usage survives a successful call as null', async t => {
  const { request, options } = await setup(t, 'unknown-usage');
  const result = await runWorker(request, options);
  assert.equal(result.status, 'ok');
  assert.equal(result.usage, null);
});

test('scoped SDK sessions continue the exact history with one new prompt and a new usage delta', async t => {
  const { request, options, frames, methods } = await setup(t, 'resume-replay');
  const first = await runWorker({ ...request, protocol: 2, persistent: true }, options);
  assert.equal(first.status, 'ok');
  assert.equal(first.lifecycle.dispatch, 'dispatched');
  assert.equal(first.lifecycle.outcome, 'completed');
  assert.equal(first.lifecycle.resumeCandidate, true);
  assert.equal(first.lifecycle.resumeAccepted, false);
  assert.equal(first.lifecycle.cleanupConfirmed, true);
  assert.equal(first.lifecycle.turnId, '1');
  const second = await runWorker({ ...request, protocol: 2, persistent: true,
    requestId: 'request-2', sessionId: first.sessionId, prompt: 'Repair using the new frozen evidence.' }, options);
  assert.equal(second.status, 'ok');
  assert.equal(second.sessionId, first.sessionId);
  assert.equal(second.lifecycle.requestId, 'request-2');
  assert.equal(second.lifecycle.resumeAccepted, true);
  assert.equal(second.lifecycle.resumeCandidate, true);
  assert.equal(second.text, 'fixture complete');
  assert.equal(second.usage.total_tokens, 128, 'restored assistant calls are not added to this invocation');
  assert.equal(second.lifecycle.usageStatus, 'complete');
  assert.ok(frames.every(frame => frame.protocol === 2));
  assert.equal(frames.filter(frame => frame.event?.type === 'assistant/message').length, 2,
    'historical assistant output is never forwarded as a new invocation event');
  assert.ok(!JSON.stringify(frames).includes('old answer'));
  assert.deepEqual(await methods(), [
    'initialize', 'session/working-directory/get', 'session/prompt', 'session/wait', 'shutdown',
    'initialize', 'session/resume', 'session/working-directory/get', 'session/prompt', 'session/wait', 'shutdown',
  ]);
});

for (const scenario of ['resume-missing', 'resume-busy', 'resume-invalid-result']) {
  test(`${scenario}: exact history rejection never falls back to a fresh session`, async t => {
    const { request, options, methods } = await setup(t, scenario);
    const result = await runWorker({ ...request, protocol: 2, persistent: true, sessionId: 'owned-session' }, options);
    assert.equal(result.status, 'error');
    assert.equal(result.lifecycle.dispatch, 'not-dispatched');
    assert.equal(result.lifecycle.outcome, 'unknown', 'the public generic RPC error cannot distinguish missing history from an occupied owner');
    assert.equal(result.lifecycle.resumeCandidate, false);
    assert.equal(result.lifecycle.resumeAccepted, false);
    assert.equal(result.lifecycle.cleanupConfirmed, true);
    assert.deepEqual(await methods(), ['initialize', 'session/resume', 'shutdown']);
  });
}

test('scoped result certainty stays separate from output validity, usage and cleanup', async t => {
  for (const [scenario, outcome, candidate] of [
    ['empty', 'completed', true], ['account-quota', 'failed', false],
    ['missing-terminal', 'unknown', false], ['invalid-turn', 'unknown', false],
    ['unknown-turn', 'unknown', false], ['invalid-wait', 'unknown', false],
    ['corrupt-tail', 'unknown', false], ['unknown-usage', 'completed', true],
  ]) {
    const { request, options } = await setup(t, scenario);
    const result = await runWorker({ ...request, protocol: 2, persistent: true }, options);
    assert.equal(result.lifecycle.outcome, outcome, scenario);
    assert.equal(result.lifecycle.resumeCandidate, candidate, scenario);
    if (scenario === 'empty' || scenario === 'unknown-usage') assert.equal(result.lifecycle.usageStatus, 'unknown');
  }
  const { request, options, methods } = await setup(t);
  const doctor = await runWorker({ ...request, protocol: 2, persistent: true }, { ...options, doctor: true });
  assert.equal(doctor.status, 'error');
  assert.equal(doctor.lifecycle.dispatch, 'not-dispatched');
  await assert.rejects(methods(), { code: 'ENOENT' });
});

test('full lifecycle deadline includes a stalled initialize; no model prompt is sent', async t => {
  const { request, options, methods } = await setup(t, 'initialize-hang');
  const started = Date.now();
  const result = await runWorker({ ...request, timeoutMs: 150 }, options);
  assert.equal(result.error.code, 'TIMEOUT');
  assert.ok(Date.now() - started < 2000);
  assert.ok(!(await methods()).includes('session/prompt'));
});

test('external cancellation settles a stalled turn through SDK shutdown', async t => {
  const { request, options, methods } = await setup(t, 'wait-hang');
  const controller = new AbortController();
  const original = options.emit;
  options.emit = frame => {
    original(frame);
    if (frame.type === 'ready') setTimeout(() => controller.abort(), 75);
  };
  const result = await runWorker(request, { ...options, signal: controller.signal });
  assert.equal(result.status, 'cancelled');
  assert.ok((await methods()).includes('shutdown'));
});

test('forced SDK shutdown cannot claim successful settlement of native managed ranges', async t => {
  const { request, options } = await setup(t, 'forced-shutdown');
  const result = await runWorker(request, options);
  assert.equal(result.status, 'error');
  assert.equal(result.settled, false);
});

test('doctor exercises runtime and permission handshake but never prompts a model', async t => {
  for (const permissionMode of ['workspace-write', 'danger-full-access']) {
    const { request, options, methods } = await setup(t);
    const result = await runWorker({ ...request, permissionMode }, { ...options, doctor: true });
    assert.equal(result.status, 'ok');
    assert.equal(result.usage, null);
    assert.deepEqual(await methods(), ['initialize', 'session/working-directory/get', 'shutdown']);
  }
});

test('overlay pins real native confinement and disables model-owned delegation and auxiliary model calls', () => {
  const patch = makeOverlay({ permissionMode: 'read-only', cwd: '/workspace' }, '/plugin/policy.js');
  const row = id => patch.find(item => item.id === id);
  assert.deepEqual(row('sandbox-policy').config, { mode: 'read-only', workspaceRoot: '/workspace' });
  assert.equal(row('approval').config.policy, 'never');
  assert.equal(row('permission').disabled, true);
  assert.equal(row('tool-subagent').disabled, true);
  assert.equal(row('schedule').disabled, true, 'the public SDK composition must not deliver a reminder into this invocation');
  assert.equal(row('tool-schedule').disabled, true);
  assert.equal(row('compaction-basic').disabled, true);
  assert.equal(row('sdk-jsonrpc-server').config.maxTokensAsSuccess, false);
});

test('full access changes the explicit permission policy while retaining engine ownership', () => {
  const patch = makeOverlay({ permissionMode: 'danger-full-access', cwd: '/workspace' }, '/plugin/policy.js');
  const row = id => patch.find(item => item.id === id);
  assert.deepEqual(row('sandbox-policy').config, { mode: 'danger-full-access', workspaceRoot: '/workspace' });
  assert.equal(row('approval').config.policy, 'never');
  assert.equal(row('permission').disabled, true);
  assert.equal(row('tool-subagent').disabled, true);
  assert.equal(row('schedule').disabled, true);
  assert.equal(row('tool-schedule').disabled, true);
  assert.equal(row('compaction-basic').disabled, true);
  assert.deepEqual(patch.find(item => item.insert).insert[0].config,
    { permissionMode: 'danger-full-access', cwd: '/workspace' });
});
