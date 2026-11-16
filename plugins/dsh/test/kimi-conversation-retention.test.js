import test from 'node:test';
import assert from 'node:assert/strict';
import { KimiConversation } from '../src/kimi-native-bridge.js';

const MiB = 1024 * 1024;
const payload = 'x'.repeat(MiB);
const event = (type, fields = {}) => ({ type, agent_id: 'main', ...fields });
const turn = (id, text = '') => ({ turn_id: id, ordinal: 1, kind: 'agent', status: 'running',
  items: [{ item_id: 'answer', type: 'assistant_message', text }] });
const retainedBytes = conversation => [...conversation.agents.values(), ...conversation.tasks.values(),
  ...conversation.pending.values(), ...[...conversation.turns.values()].flatMap(turns => [...turns.values()])]
  .reduce((total, value) => total + Buffer.byteLength(JSON.stringify(value)), 0);
const assertAccounting = conversation => assert.equal(conversation.bytes, retainedBytes(conversation));

test('replacing one agent snapshot can exceed 64 MiB of traffic without exhausting retained space', () => {
  const conversation = new KimiConversation();
  for (let index = 0; index < 70; index++) {
    conversation.fold(event('agent', { status: 'running', index, description: payload }));
  }
  assert.equal(conversation.agents.size, 1);
  assert.equal(conversation.agents.get('main').index, 69);
  assert.ok(conversation.bytes < 2 * MiB);
  assertAccounting(conversation);
});

test('unretained protocol frames cannot consume the retained budget or allocate agent histories', () => {
  const conversation = new KimiConversation();
  const before = conversation.lastChange;
  for (let index = 0; index < 70; index++) conversation.fold(event('usage', {
    agent_id: `unretained-${index}`, context_tokens: index, detail: payload,
  }));
  assert.equal(conversation.bytes, 0);
  assert.equal(conversation.turns.size, 0);
  assert.equal(conversation.lastChange, before);
});

test('HTTP reconciliation shares the retained budget and rejects overflow before retaining the extra turn', () => {
  const conversation = new KimiConversation();
  for (let index = 0; index < 63; index++) conversation.mergeTurn('main', turn(`turn-${index}`, payload));
  const before = conversation.bytes;
  assert.throws(() => conversation.mergeTurn('main', turn('over-limit', payload)), { code: 'OBSERVATION_LIMIT' });
  assert.equal(conversation.agentTurns('main').size, 63);
  assert.equal(conversation.bytes, before);
  assertAccounting(conversation);

  conversation.fold(event('turns.removed', { turn_ids: ['turn-0', 'turn-1'] }));
  conversation.mergeTurn('main', turn('replacement', payload));
  assert.equal(conversation.agentTurns('main').has('turn-0'), false);
  assertAccounting(conversation);
});

test('both HTTP and live turn replacement release their previous retained payload', () => {
  const conversation = new KimiConversation();
  for (let index = 0; index < 70; index++) conversation.mergeTurn('main', turn('current', payload));
  for (let index = 0; index < 70; index++) conversation.fold(event('turn.ended', { turn: turn('current', payload) }));
  assert.equal(conversation.agentTurns('main').size, 1);
  assert.ok(conversation.bytes < 2 * MiB);
  conversation.mergeTurn('main', { turn_id: 'current', ordinal: 1, status: 'ended', items: [] });
  assert.ok(conversation.bytes < 256);
  assertAccounting(conversation);
});

test('item, step, task and interaction replacement or removal reclaim retained bytes', () => {
  const conversation = new KimiConversation();
  conversation.mergeTurn('main', turn('current', payload));
  conversation.fold(event('turn.updated', { turn_id: 'current', step: { index: 0, detail: payload } }));
  conversation.fold(event('task', { task_id: 'child', status: 'running', detail: payload }));
  conversation.fold(event('interaction', { interaction_id: 'permission', status: 'pending', detail: payload }));
  assertAccounting(conversation);
  assert.ok(conversation.bytes > 4 * MiB);
  conversation.fold(event('item.updated', { turn_id: 'current', item_id: 'answer', set: { text: 'small' } }));
  conversation.fold(event('turn.updated', { turn_id: 'current', step: { index: 0, usage: { output: 2 } } }));
  conversation.fold(event('task.list', { tasks: [{ task_id: 'child', status: 'running' }] }));
  conversation.fold(event('interaction', { interaction_id: 'permission', status: 'answered' }));
  assert.ok(conversation.bytes < 1024);
  assertAccounting(conversation);
  conversation.fold(event('task.list', { tasks: [] }));
  assert.equal(conversation.tasks.get('main/child').outcome, 'unknown', 'absence retains the child identity and uncertain outcome');
  conversation.fold(event('item.started', { turn_id: 'current', item: { item_id: 'second', text: payload } }));
  conversation.fold(event('item.removed', { turn_id: 'current', item_id: 'second' }));
  assert.ok(conversation.bytes < 1024);
  assertAccounting(conversation);
});

test('live item growth enforces retained capacity and preserves the prior projection on overflow', () => {
  const conversation = new KimiConversation();
  for (let index = 0; index < 63; index++) conversation.mergeTurn('main', turn(`turn-${index}`, payload));
  const before = conversation.bytes;
  assert.throws(() => conversation.fold(event('item.delta', {
    turn_id: 'turn-0', item_id: 'answer', field: 'text', delta: payload,
  })), { code: 'OBSERVATION_LIMIT' });
  assert.equal(conversation.bytes, before);
  assert.equal(conversation.agentTurns('main').get('turn-0').items[0].text, payload);
  conversation.fold(event('item.removed', { turn_id: 'turn-1', item_id: 'answer' }));
  conversation.fold(event('item.delta', { turn_id: 'turn-0', item_id: 'answer', field: 'text', delta: payload }));
  assert.equal(conversation.agentTurns('main').get('turn-0').items[0].text.length, 2 * MiB);
  assertAccounting(conversation);
});

test('incremental bytes match JSON encoding for escaped text and Unicode split across deltas', () => {
  const conversation = new KimiConversation();
  conversation.mergeTurn('main', turn('current'));
  for (const delta of ['\"\\\n\t', '你好', '\ud800', '\udc00', '😀', '\ud800', '', 'x', '\udc00']) {
    conversation.fold(event('item.delta', { turn_id: 'current', item_id: 'answer', field: 'text', delta }));
    assertAccounting(conversation);
  }
  conversation.fold(event('item.updated', { turn_id: 'current', item_id: 'answer', set: { text: '\ud800', metadata: { escaped: '\\' } } }));
  conversation.fold(event('item.delta', { turn_id: 'current', item_id: 'answer', field: 'text', delta: '\udc00' }));
  conversation.fold(event('item.delta', { turn_id: 'current', item_id: 'answer', field: 'input_text', delta: '\n😀' }));
  assertAccounting(conversation);
});

test('delta accounting serializes new fragments without repeatedly scanning accumulated answer text', t => {
  const conversation = new KimiConversation();
  conversation.mergeTurn('main', turn('current', payload));
  const stringify = JSON.stringify;
  let measuredBytes = 0;
  const mocked = t.mock.method(JSON, 'stringify', (...args) => {
    const encoded = stringify(...args);
    measuredBytes += Buffer.byteLength(encoded ?? '');
    return encoded;
  });
  try {
    for (let index = 0; index < 128; index++) conversation.fold(event('item.delta', {
      turn_id: 'current', item_id: 'answer', field: 'text', delta: 'y'.repeat(256),
    }));
  } finally { mocked.mock.restore(); }
  assert.ok(measuredBytes < 256 * 1024, `incremental accounting measured ${measuredBytes} bytes for 32 KiB of new text`);
  assertAccounting(conversation);
});

test('HTTP and live paths preserve turn, task and observed-agent count limits', () => {
  const turns = new KimiConversation();
  for (let index = 0; index < 4096; index++) turns.mergeTurn('main', turn(`turn-${index}`));
  assert.throws(() => turns.mergeTurn('main', turn('extra')), { code: 'OBSERVATION_LIMIT' });
  assert.equal(turns.agentTurns('main').size, 4096);
  const tasks = new KimiConversation();
  for (let index = 0; index < 4096; index++) tasks.fold(event('task', { task_id: `task-${index}`, status: 'ended' }));
  assert.throws(() => tasks.fold(event('task', { task_id: 'extra', status: 'running' })), { code: 'OBSERVATION_LIMIT' });
  assert.equal(tasks.tasks.size, 4096);
  const agents = new KimiConversation();
  for (let index = 0; index < 512; index++) agents.mergeTurn(`agent-${index}`, turn('current'));
  assert.throws(() => agents.mergeTurn('extra', turn('current')), { code: 'OBSERVATION_LIMIT' });
  assert.equal(agents.turns.size, 512);
});
