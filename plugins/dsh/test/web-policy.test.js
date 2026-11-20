import test from 'node:test';
import assert from 'node:assert/strict';
import { admitChatGPTWeb, verifyChatGPTWebAdmission } from '../src/web-policy.js';
import { makeOverlay } from '../src/bridge.js';
import { modelErrorCode, normalizedError, requiresWebReconciliation, SessionFold } from '../src/protocol.js';

const request = { provider: 'chatgpt', model: 'gpt-6-pro-web', reasoningEffort: 'pro' };
const model = { provider: 'chatgpt', id: 'gpt-6-pro-web', reasoning: { defaultEffort: 'pro', efforts: [{ id: 'pro', name: 'Pro' }] } };
function context(config = {}) {
  const calls = [];
  const llm = { resolveModelInfo: async (provider, id) => {
    calls.push([provider, id]);
    return provider === 'chatgpt' && id === request.model ? structuredClone(model) : { provider, id };
  } };
  const compaction = { config: { auto: true, summarizationProvider: 'chatgpt', summarizationModel: 'gpt-6.2-sol', modelPolicies: [], ...config } };
  return { calls, llm, compaction, ctx: { get: name => ({ llm, compaction })[name] } };
}

test('Web admission checks the actual adapter and exact effective auxiliary route without inference', async () => {
  const f = context({ modelPolicies: [{ provider: 'chatgpt', model: request.model,
    summarizationProvider: 'explicit-aux', summarizationModel: 'summary-only' }] });
  const receipt = await admitChatGPTWeb(f.ctx, request);
  assert.deepEqual(f.calls, [['explicit-aux', 'summary-only'], ['chatgpt', 'gpt-6-pro-web']]);
  assert.deepEqual(receipt.summary, { provider: 'explicit-aux', model: 'summary-only' });
  assert.equal(receipt.usage, 'unknown');
  assert.equal(receipt.idle, 'wall-only');
  assert.equal(receipt.title, 'deterministic');
  verifyChatGPTWebAdmission({ web: receipt }, request);
  verifyChatGPTWebAdmission({}, { provider: 'chatgpt', model: 'gpt-6.2-sol' });
  assert.throws(() => verifyChatGPTWebAdmission({}, request), /prompt withheld/);
  assert.throws(() => verifyChatGPTWebAdmission({ web: { ...receipt, summary: { provider: 'chatgpt', model: request.model } } }, request), /prompt withheld/);
  assert.throws(() => verifyChatGPTWebAdmission({ web: { ...receipt, summary: { provider: 'renamed-provider', model: request.model } } }, request), /prompt withheld/);
});

test('Web readiness rejects missing services, implicit or Web summaries and stale model metadata', async () => {
  await assert.rejects(admitChatGPTWeb(context().ctx, { ...request, reasoningEffort: 'high' }), /exact/);
  await assert.rejects(admitChatGPTWeb({ get: () => undefined }, request), /public LLM/);
  for (const summary of [{ summarizationProvider: '' }, { summarizationModel: '' }, { summarizationModel: request.model },
    { summarizationProvider: 'renamed-provider', summarizationModel: request.model }]) {
    const f = context(summary);
    await assert.rejects(admitChatGPTWeb(f.ctx, request), /explicit supported/);
    assert.equal(f.calls.length, 0);
  }
  for (const malformed of [{ auto: false }, { auto: undefined }, { modelPolicies: [null] },
    { modelPolicies: [{ provider: 'chatgpt' }] }, { modelPolicies: {} }]) {
    const f = context(malformed);
    await assert.rejects(admitChatGPTWeb(f.ctx, request), /public LLM/);
    assert.equal(f.calls.length, 0, 'bad host policies fail before metadata or inference');
  }
  const duplicated = context({ modelPolicies: [request, request] });
  await assert.rejects(admitChatGPTWeb(duplicated.ctx, request), /duplicate compaction/);
  assert.equal(duplicated.calls.length, 0);
  const mismatch = context();
  mismatch.llm.resolveModelInfo = async () => ({ provider: 'other', id: 'other' });
  await assert.rejects(admitChatGPTWeb(mismatch.ctx, request), /auxiliary provider/);
  for (const info of [{ ...model, reasoning: undefined }, { ...model, defaultMaxTokens: 10 },
    { ...model, reasoning: { defaultEffort: 'high', efforts: [{ id: 'high' }] } }]) {
    const f = context();
    f.llm.resolveModelInfo = async (provider, id) => id === request.model ? info : { provider, id };
    await assert.rejects(admitChatGPTWeb(f.ctx, request), /Upgrade and configure/);
  }
  const unavailable = context();
  unavailable.llm.resolveModelInfo = async () => { throw Object.assign(new Error('verified bridge unavailable'), { code: 'WEB_SETUP_REQUIRED' }); };
  await assert.rejects(admitChatGPTWeb(unavailable.ctx, request), { code: 'WEB_SETUP_REQUIRED' });
  await assert.rejects(admitChatGPTWeb(context().ctx, request, AbortSignal.abort()), { name: 'AbortError' });
});

test('only explicit Web keeps host compaction; the deterministic title path never needs a title model', () => {
  const common = { cwd: '/repo', permissionMode: 'read-only' };
  const web = makeOverlay({ ...common, ...request });
  const normal = makeOverlay({ ...common, provider: 'chatgpt', model: 'gpt-6.2-sol' });
  assert.deepEqual(web.find(row => row.id === 'compaction-basic'), { id: 'compaction-basic', disabled: false },
    'omitting config preserves the explicit SDK summary settings');
  assert.equal(normal.find(row => row.id === 'compaction-basic').disabled, true);
  assert.equal(web.find(row => row.id === 'session-title-llm').disabled, true);
  assert.deepEqual(web.find(row => row.insert).insert[0].config.web, { version: 1, ...request });
});

test('typed Web errors remain distinct and absent usage never becomes zero', () => {
  for (const code of ['WEB_UNCERTAIN', 'WEB_DELIVERY_CLAIMED', 'WEB_CONNECTION_LOST', 'WEB_PROTOCOL_ERROR']) {
    assert.equal(modelErrorCode(code), code);
    assert.equal(requiresWebReconciliation(code), true);
    const fold = new SessionFold('s');
    fold.receipts.add('p');
    const requestId = 'turn-' + 'a'.repeat(64);
    fold.terminal = { kind: 'error', error: { code, message: 'reconcile original browser request', requestId } };
    assert.throws(() => fold.finish('p'), error => {
      assert.equal(error.code, code);
      assert.equal(normalizedError(error).requestId, requestId);
      return true;
    });
  }
  assert.equal(requiresWebReconciliation('SERVER'), false);
  const failedJournal = new SessionFold('s');
  failedJournal.receipts.add('p');
  failedJournal.terminal = { kind: 'error', error: { code: 'WEB_JOURNAL_WRITE', message: 'durability failed after Send', requestId: 'turn-' + 'b'.repeat(64) } };
  assert.throws(() => failedJournal.finish('p'), error => {
    assert.equal(error.code, 'SERVER');
    assert.equal(normalizedError(error).requestId, failedJournal.terminal.error.requestId);
    return true;
  });
  for (const code of ['WEB_SETUP_REQUIRED', 'WEB_BINDING_MISMATCH', 'WEB_UNSUPPORTED_CONTROL']) assert.equal(modelErrorCode(code), 'INVALID_REQUEST');
  const fold = new SessionFold('s');
  fold.accept({ sessionId: 's', event: { seq: 0, type: 'agent/inbox/spliced', data: { inserted: [{ id: 'p' }] } } });
  fold.accept({ sessionId: 's', event: { seq: 1, type: 'assistant/message', data: { message: { content: [{ type: 'text', text: 'complete answer' }] } } } });
  fold.accept({ sessionId: 's', event: { seq: 2, type: 'turn/end', data: { turn: 1, reason: { kind: 'completed' } } } });
  assert.equal(fold.finish('p').usage, null);
});
