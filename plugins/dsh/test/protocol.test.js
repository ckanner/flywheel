import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, realpath, rm, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { SessionFold, validateStart } from '../src/protocol.js';

const request = cwd => ({ type: 'start', protocol: 1, requestId: 'test', cwd,
  provider: 'deepseek-official', model: 'deepseek-flash', reasoningEffort: 'high',
  permissionMode: 'read-only', prompt: 'Inspect only.' });

test('start normalizes cwd and rejects resume, extra write roots, implicit permission, and empty route', async t => {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel-protocol-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  const link = join(cwd, 'alias');
  await symlink(cwd, link);
  assert.equal((await validateStart(request(link))).cwd, await realpath(cwd));
  for (const permissionMode of ['read-only', 'workspace-write', 'danger-full-access']) {
    assert.equal((await validateStart({ ...request(cwd), permissionMode })).permissionMode, permissionMode);
  }
  for (const extra of [{ sessionId: 'old' }, { writableRoots: ['/'] }, { permissionMode: undefined },
    { permissionMode: 'dangerously-bypass-approvals-and-sandbox' }, { permissionMode: 'full' },
    { provider: '' }, { model: '' }, { reasoningEffort: '' }, { timeoutMs: 0 }, { maxTokens: -1 }, { cwd: '.' }]) {
    await assert.rejects(validateStart({ ...request(cwd), ...extra }));
  }
});

function completedFold(usage) {
  const fold = new SessionFold('session');
  const event = (seq, type, data) => fold.accept({ sessionId: 'session', event: { seq, type, data } });
  event(0, 'agent/inbox/spliced', { inserted: [{ id: 'prompt' }] });
  event(1, 'assistant/message', { message: { content: [{ type: 'text', text: 'first' }] }, usage });
  event(1, 'assistant/message', { message: { content: [{ type: 'text', text: 'duplicate' }] }, usage });
  event(2, 'turn/end', { reason: { kind: 'completed' } });
  return { fold, event };
}

test('only unique committed model calls count; cache is included in aggregate input, reasoning is a subset', () => {
  const { fold, event } = completedFold({ inputTokens: 10, outputTokens: 5, cacheReadTokens: 8,
    cacheWriteTokens: 2, reasoningTokens: 3, totalTokens: 25 });
  event(3, 'assistant/message', { message: { content: [{ type: 'text', text: 'last' }] },
    usage: { inputTokens: 7, outputTokens: 2 } });
  assert.deepEqual(fold.finish('prompt'), { text: 'last', usage: {
    input_tokens: 27, output_tokens: 7, reasoning_tokens: 3,
    cache_read_tokens: 8, cache_write_tokens: 2, total_tokens: 34,
  } });
});

test('missing or malformed usage remains unknown, not free or partial cost', () => {
  for (const usage of [undefined, { inputTokens: -1, outputTokens: 1 }, { inputTokens: 1 },
    { inputTokens: 1, outputTokens: 1, reasoningTokens: 0.5 }]) {
    assert.equal(completedFold(usage).fold.finish('prompt').usage, null);
  }
});

test('an idle or max-token interval is not successful completion', () => {
  const fold = new SessionFold('s');
  assert.throws(() => fold.finish('missing'), /receipt/);
  fold.receipts.add('p');
  assert.throws(() => fold.finish('p'), /turn\/end/);
  fold.terminal = { kind: 'max-tokens' };
  assert.throws(() => fold.finish('p'), /max-tokens/);
  fold.terminal = { kind: 'error', error: { code: 'RATE_LIMIT', message: 'slow down' } };
  assert.throws(() => fold.finish('p'), error => error.code === 'RATE_LIMIT');
  fold.terminal = { kind: 'error', error: { code: 'MISSING_CREDENTIAL', message: 'credential unavailable' } };
  assert.throws(() => fold.finish('p'), error => error.code === 'AUTH');
  fold.terminal = { kind: 'completed' };
  assert.throws(() => fold.finish('p'), error => error.code === 'EMPTY_RESPONSE');
});

test('permission acknowledgement must match actual cwd, policy, approval and full confinement', () => {
  const fold = new SessionFold('s');
  const req = request('/tmp/workspace');
  const valid = { protocol: 1, cwd: req.cwd, permissionMode: 'read-only', approvalPolicy: 'never', confinedShell: true, enforcement: 'full' };
  for (const extra of [{ cwd: '/' }, { permissionMode: 'workspace-write' }, { approvalPolicy: 'ask' },
    { confinedShell: false }, { enforcement: 'partial' }]) {
    fold.policy = { ...valid, ...extra };
    assert.throws(() => fold.verifiedPolicy(req));
  }
  fold.policy = valid;
  assert.doesNotThrow(() => fold.verifiedPolicy(req));
});

test('full access acknowledgement is explicit and never claims confinement', () => {
  const fold = new SessionFold('s');
  const req = { ...request('/tmp/workspace'), permissionMode: 'danger-full-access' };
  const valid = { protocol: 1, cwd: req.cwd, permissionMode: req.permissionMode,
    approvalPolicy: 'never', confinedShell: false };
  for (const extra of [{ cwd: '/' }, { permissionMode: 'workspace-write' }, { approvalPolicy: 'ask' },
    { confinedShell: true }, { confinedShell: undefined }, { enforcement: 'full' }, { enforcement: 'partial' }]) {
    fold.policy = { ...valid, ...extra };
    assert.throws(() => fold.verifiedPolicy(req));
  }
  fold.policy = valid;
  assert.doesNotThrow(() => fold.verifiedPolicy(req));
  assert.throws(() => fold.verifiedPolicy({ ...req, permissionMode: 'read-only' }),
    'a full-access report must never satisfy a constrained request');
});
