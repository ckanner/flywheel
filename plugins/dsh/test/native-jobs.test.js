import test from 'node:test';
import assert from 'node:assert/strict';
import { runNativeJob, main } from '../src/native-jobs.js';

test('independent operations use the exact public request and separate journal without a role verdict', async () => {
  const calls = [];
  const api = Object.fromEntries(['recoverNativeJob', 'controlInteractiveTeam', 'runMachine', 'runNativeReview', 'runInteractiveTeam'].map(name => [name, async (...args) => { calls.push({ name, args }); return { ok: true, operation: name }; }]));
  api.parseMachineRequest = value => value;
  api.validateNativeReviewRequest = () => {};
  api.validateInteractiveTeamRequest = () => {};
  const signal = new AbortController().signal;
  const options = { signal, loaders: { codex: async () => api, claude: async () => api } };
  for (const operation of ['codex-review', 'claude-review', 'claude-team', 'claude-recover', 'claude-control']) {
    const input = { protocol: 1, requestId: 'job-1', cwd: '/repo', prompt: 'original', recoveryDirectory: '/private/jobs/one',
      ...(operation === 'codex-review' ? { threadMode: 'ephemeral', native: { version: 1, review: { target: { type: 'uncommittedChanges' } } } } : {}),
      ...(operation === 'claude-review' ? { consent: true } : {}) };
    const receipt = await runNativeJob(operation, input, options);
    assert.equal(receipt.ok, true); assert.equal(receipt.verdict, undefined);
    const call = calls.at(-1);
    if (['claude-recover', 'claude-control'].includes(operation)) assert.deepEqual(call.args, [input]);
    else {
      const { recoveryDirectory, ...request } = input;
      assert.deepEqual(call.args, [request, { recoveryDirectory, signal }]);
    }
  }
  await assert.rejects(runNativeJob('codex-review', { recoveryDirectory: '/job', threadMode: 'persistent' }, options), /fresh/);
  for (const [operation, input] of [['unknown', {}], ['claude-team', null], ['claude-team', { recoveryDirectory: 'relative' }]]) {
    await assert.rejects(runNativeJob(operation, input, options));
  }
  const aborted = AbortSignal.abort(new Error('stop before dispatch'));
  const count = calls.length;
  await assert.rejects(runNativeJob('claude-team', { recoveryDirectory: '/job' }, { ...options, signal: aborted }), /stop before/);
  assert.equal(calls.length, count);
});

test('native CLI describes job semantics and rejects malformed paths before loading a provider', async () => {
  let output = '', errors = '';
  const streams = { stdout: { write: value => { output += value; } }, stderr: { write: value => { errors += value; } } };
  assert.equal(await main(['--help'], streams), 0);
  assert.match(output, /never a formal Flywheel verdict/);
  assert.equal(await main(['claude-review', '--request', 'relative.json'], streams), 1);
  assert.match(errors, /request-file interface/);
});


test('pending recovery and unconfirmed remote cleanup do not report successful job completion', async () => {
  const { nativeJobExitCode } = await import('../src/native-jobs.js');
  assert.equal(nativeJobExitCode({ status: 'pending', receipt: null }), 3);
  assert.equal(nativeJobExitCode({ status: 'settled', receipt: { ok: false, cleanup: { confirmed: true } } }), 3);
  assert.equal(nativeJobExitCode({ ok: true, cleanup: { confirmed: true }, native: { cleanup: { confirmed: false } } }), 3);
  assert.equal(nativeJobExitCode({ status: 'settled', receipt: { ok: true, cleanup: { confirmed: true }, native: { cleanup: { confirmed: true } } } }), 0);
});
