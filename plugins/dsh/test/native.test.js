import test from 'node:test';
import assert from 'node:assert/strict';
import { NativeTree, nativeDigest, validateNative, validateManaged } from '../src/native.js';

const native = { version: 1, preset: 'heavy', agents: { enabled: true } };
const rootUsage = { input_tokens: 10, output_tokens: 2, reasoning_tokens: 1, cache_read_tokens: 0, cache_write_tokens: 0, total_tokens: 12 };
const start = (tree, id = 'child', parentSessionId = 'root') => tree.notify('subagent.started', { childSessionId: id, parentSessionId });
const finish = (tree, id = 'child', stopReason = 'completed', parentSessionId = 'root') => tree.notify('subagent.finished', { childSessionId: id, parentSessionId, stopReason });
const event = (tree, seq, type, data, sessionId = 'child') => tree.notify('session.event', { sessionId, event: { seq, type, data } });
function answer(tree, id = 'child', seq = 1, usage = { inputTokens: 3, outputTokens: 4, cacheReadTokens: 2 }) {
  event(tree, seq, 'turn/start', { turn: seq }, id);
  event(tree, seq + 1, 'assistant/message', { message: { content: [{ type: 'text', text: 'a child answer must not become the root result' }] }, usage }, id);
  event(tree, seq + 2, 'turn/end', { turn: seq, reason: { kind: 'completed' } }, id);
}
function runtime(sequence, type, { id = 'external', runId = 'epoch-1', local = false, parentSessionId = 'root', stopReason = 'completed' } = {}) {
  return { version: 1, journalSequence: sequence, rootSessionId: 'root', type,
    child: { id, runId, local, parentSessionId, provider: local ? 'spawn' : 'codex-pro', ...(type === 'subagent.finished' ? { stopReason } : {}) } };
}

test('native options validate composition, capability dependencies and immutable invocation identity', () => {
  assert.equal(nativeDigest({ b: [1, 2], a: 3 }), nativeDigest({ a: 3, b: [1, 2] }));
  validateNative(native);
  for (const invalid of [null, { ...native, team: { enabled: true }, agents: { enabled: false } },
    { ...native, agents: { enabled: true, maxDepth: 0 } }, { ...native, lcu: { enabled: true, config: { command: '' } } },
    { ...native, web: { fetch: 'yes' } }, { ...native, unknown: true }]) assert.throws(() => validateNative(invalid));
  const managed = { version: 1, runId: 'run', invocationId: 'invocation', role: 'executor', scope: 'scope', channel: 'test', taskDir: '/task' };
  validateManaged(managed, 'invocation');
  assert.throws(() => validateManaged(managed, 'other'));
  assert.throws(() => validateManaged({ ...managed, lease: {} }, 'invocation'));
});

test('local child usage is committed once and telemetry cannot leak child answers into the root', () => {
  const frames = [], tree = new NativeTree('root', native, value => frames.push(value));
  start(tree); start(tree); answer(tree); answer(tree); finish(tree); finish(tree);
  assert.deepEqual(tree.completion(), { confirmed: true, pendingIds: [] });
  assert.deepEqual(tree.addUsage(rootUsage), { input_tokens: 15, output_tokens: 6, reasoning_tokens: 1,
    cache_read_tokens: 2, cache_write_tokens: 0, total_tokens: 21 });
  const receipt = tree.receipt({ matched: true }, { status: 'confirmed' });
  assert.equal(receipt.children.length, 1);
  assert.equal(receipt.usage.status, 'complete');
  assert.ok(!JSON.stringify(receipt).includes('a child answer'));
  assert.ok(!JSON.stringify(frames).includes('a child answer'));
  assert.equal(rootUsage.input_tokens, 10);
});

test('root completion remains pending until the child residency epoch settles; later shutdown cannot rewrite a snapshot', () => {
  const tree = new NativeTree('root', native);
  start(tree); answer(tree);
  const completionAtRootEnd = tree.completion();
  assert.deepEqual(completionAtRootEnd, { confirmed: false, pendingIds: ['child'] });
  assert.equal(tree.addUsage(rootUsage), null);
  finish(tree, 'child', 'aborted');
  assert.deepEqual(tree.completion(), { confirmed: true, pendingIds: [] });
  assert.equal(completionAtRootEnd.confirmed, false);
});

test('a settled child failure is attributable and does not veto a root that handled it', () => {
  const tree = new NativeTree('root', native);
  start(tree);
  event(tree, 1, 'assistant/message', { message: { content: [] }, usage: { inputTokens: 2, outputTokens: 1 } });
  event(tree, 2, 'turn/end', { turn: 1, reason: { kind: 'error' } });
  finish(tree, 'child', 'error');
  assert.equal(tree.completion().confirmed, true);
  assert.equal(tree.addUsage(rootUsage).total_tokens, 15);
});

test('unreported or malformed usage remains unknown, never a fabricated zero', () => {
  for (const usage of [undefined, null, { inputTokens: -1, outputTokens: 3 }, { inputTokens: 1, outputTokens: Infinity }]) {
    const tree = new NativeTree('root', native); start(tree);
    event(tree, 1, 'assistant/message', { message: { content: [] }, usage });
    event(tree, 2, 'turn/end', { turn: 1, reason: { kind: 'completed' } }); finish(tree);
    assert.equal(tree.addUsage(rootUsage), null);
    assert.equal(tree.receipt({}, {}).usage.status, 'partial');
  }
  const empty = new NativeTree('root', native); start(empty); finish(empty);
  assert.equal(empty.addUsage(rootUsage), null);
  assert.equal(new NativeTree('root', native).addUsage(null), null);
  assert.equal(new NativeTree('root', native).addUsage({}), null);
});

test('nested children require their actual admitted parent and cannot change identity', () => {
  const tree = new NativeTree('root', native); start(tree);
  start(tree, 'grandchild', 'child');
  assert.deepEqual(tree.completion().pendingIds, ['child', 'grandchild']);
  for (const invoke of [() => start(tree, 'foreign', 'other'), () => start(tree, 'root'), () => start(tree, 'grandchild', 'root'),
    () => finish(tree, 'child', 'completed', 'other'), () => finish(tree, 'missing')]) assert.throws(invoke);
  assert.throws(() => start(new NativeTree('root', { agents: { enabled: false } })));
});

test('changed duplicate and late committed child records cannot alter terminal state or usage', () => {
  const duplicate = new NativeTree('root', native); start(duplicate); answer(duplicate); finish(duplicate);
  assert.throws(() => event(duplicate, 2, 'assistant/message', { message: { content: [] }, usage: { inputTokens: 99, outputTokens: 2 } }), /changed/);
  const late = new NativeTree('root', native); start(late); answer(late); finish(late);
  event(late, 4, 'turn/start', { turn: 2 });
  assert.equal(late.completion().confirmed, false);
  assert.throws(() => event(late, 5, 'turn/end', { turn: 1, reason: { kind: 'completed' } }), /earlier turn/);
  const reordered = new NativeTree('root', native); start(reordered);
  event(reordered, 3, 'turn/start', { turn: 1 });
  assert.throws(() => event(reordered, 2, 'diagnostic', {}), /out of order/);
});

test('public remote provider lifecycle is idempotent, owns completion, and keeps total usage unknown', () => {
  const tree = new NativeTree('root', native);
  const started = runtime(1, 'subagent.started');
  const ended = runtime(2, 'subagent.finished');
  tree.observeRuntime([started]); tree.observeRuntime([started]);
  assert.deepEqual(tree.completion(), { confirmed: false, pendingIds: ['external'] });
  tree.observeRuntime([started, ended]); tree.observeRuntime([started, ended]);
  assert.deepEqual(tree.completion(), { confirmed: true, pendingIds: [] });
  assert.equal(tree.addUsage(rootUsage), null);
  assert.equal(tree.receipt({}, {}).activations.length, 1);
  assert.equal(tree.receipt({}, {}).children[0].usage, null);
  assert.throws(() => tree.observeRuntime([{ ...ended, child: { ...ended.child, stopReason: 'aborted' } }]), /changed/);
});

test('an active later residency epoch cannot be closed by a previous terminal or a duplicate start', () => {
  const tree = new NativeTree('root', native); start(tree); answer(tree); finish(tree);
  const first = [runtime(1, 'subagent.started', { id: 'child', local: true }), runtime(2, 'subagent.finished', { id: 'child', local: true })];
  tree.observeRuntime(first);
  assert.equal(tree.completion().confirmed, true);
  tree.observeRuntime([...first, runtime(3, 'subagent.started', { id: 'child', local: true, runId: 'epoch-2' })]);
  finish(tree); tree.observeRuntime(first);
  assert.deepEqual(tree.completion(), { confirmed: false, pendingIds: ['child'] });
});

test('runtime observations from an unrelated root or an unpaired end fail attribution', () => {
  for (const row of [{ ...runtime(1, 'subagent.started'), rootSessionId: 'other' }, runtime(1, 'subagent.finished'),
    runtime(1, 'subagent.started', { parentSessionId: 'unadmitted' })]) {
    assert.throws(() => new NativeTree('root', native).observeRuntime([row]));
  }
});

test('pre-dispatch rejection preserves earlier paid turns and cannot settle an active epoch or an unowned parent', () => {
  const rejection = (journalSequence, parentLive = true) => ({ version: 1, journalSequence, rootSessionId: 'root',
    type: 'agent.admission-rejected', id: 'child', parentId: 'root', matched: false, beforeModelDispatch: true, parentLive });
  const prior = new NativeTree('root', native); start(prior); answer(prior); finish(prior);
  const usage = prior.addUsage(rootUsage);
  prior.observeRuntime([rejection(1)]);
  assert.deepEqual(prior.addUsage(rootUsage), usage);
  assert.equal(prior.receipt({}, {}).children[0].state, 'completed');
  assert.equal(prior.receipt({}, {}).children[0].usage.total_tokens, 9);
  const active = new NativeTree('root', native); start(active);
  active.observeRuntime([runtime(1, 'subagent.started', { id: 'child', local: true }), rejection(2)]);
  assert.deepEqual(active.completion(), { confirmed: false, pendingIds: ['child'] });
  assert.equal(active.addUsage(rootUsage), null);
  const unknown = new NativeTree('root', native); start(unknown); unknown.observeRuntime([rejection(1, false)]);
  assert.equal(unknown.completion().confirmed, false);
  assert.equal(unknown.receipt({}, {}).children[0].usage, null);
});

test('native telemetry bounds and token overflow fail explicitly', () => {
  const tree = new NativeTree('root', native);
  for (let i = 0; i < 4096; i++) tree.event('bounded', { id: String(i) });
  assert.throws(() => tree.event('overflow', {}), /budget/);
  assert.equal(tree.receipt({}, {}).observed.complete, false);
  assert.equal(tree.addUsage(rootUsage), null);
  const bytes = new NativeTree('root', native);
  assert.throws(() => bytes.event('too-large', { value: 'x'.repeat(2 * 1024 * 1024) }), /budget/);
  const tokens = new NativeTree('root', native); start(tokens); answer(tokens); finish(tokens);
  assert.equal(tokens.addUsage({ ...rootUsage, input_tokens: Number.MAX_SAFE_INTEGER }), null);
});
