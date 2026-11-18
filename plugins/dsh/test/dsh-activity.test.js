import test from 'node:test';
import assert from 'node:assert/strict';
import { createDshActivityPublisher, acceptDshActivity, validDshActivity, dshActivityInterval, DSH_ACTIVITY_METHOD } from '../src/dsh-activity.js';

function fixture({ children = true, idleTimeoutMs = 1000, web = false } = {}) {
  const handlers = new Map(), services = new Map(), agents = new Map(), frames = [];
  let at = 0, dispose;
  const ctx = { get: key => key === 'agents' ? { get: id => agents.get(id) } : services.get(key),
    provide: (key, value) => services.set(key, value), on: (key, fn) => handlers.set(key, fn),
    effect: fn => { dispose = fn(); } };
  const identity = { requestId: 'request-one', nonce: 'private-nonce', rootSessionId: 'root' };
  const publisher = createDshActivityPublisher(ctx, { identity: () => identity, idleTimeoutMs, children, ...(web ? { webIdentity: () => ({ hostInvocationId: 'owned-call', executionDomainId: 'owned-domain' }) } : {}) },
    { now: () => at, notify: (method, params) => frames.push({ method, params }) });
  const agent = (id, parentId) => {
    const value = { id, session: { id, header: { origin: parentId ? 'subagent' : 'sdk', ...(parentId ? { parentSession: parentId } : {}) } } };
    agents.set(id, value); return value;
  };
  const root = agent('root'); publisher.admit(root, true);
  const stream = (subject, revision, chunk = { type: 'text-delta', text: 'PRIVATE_MODEL_BODY' }) =>
    handlers.get('agent/assistant-stream')({ agent: subject, frame: { type: 'chunk', attemptId: subject.id + ':1', revision, chunk } });
  return { publisher, identity, frames, root, agent, agents, handlers, stream,
    time: value => { at = value; }, dispose: () => dispose() };
}

test('DSH coalesces actual root/child observations globally without copying content or emitting timer heartbeats', async () => {
  const f = fixture();
  const child = f.agent('child', 'root'); f.publisher.admit(child, false);
  for (let i = 1; i <= 100000; i++) {
    f.time(i / 100);
    f.stream(i % 2 ? f.root : child, i);
  }
  assert.equal(f.frames.length, 4);
  assert.equal(f.frames[0].params.activity.scope, 'root');
  assert.equal(f.frames.at(-1).params.activity.scope, 'mixed');
  assert.ok(f.frames.every(row => row.method === DSH_ACTIVITY_METHOD && validDshActivity(row.params.activity)));
  assert.ok(Buffer.byteLength(JSON.stringify(f.frames)) < 4096);
  assert.ok(!JSON.stringify(f.frames).includes('PRIVATE_MODEL_BODY'));
  const before = f.frames.length;
  f.time(100000);
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(f.frames.length, before, 'elapsed time alone cannot create progress');
  f.dispose(); f.stream(f.root, 100001);
  assert.equal(f.frames.length, before, 'disposed policy cannot keep a task alive');
});

test('only live admitted lineage counts; foreign, replaced, replayed and detached children do not refresh activity', () => {
  const f = fixture();
  const foreign = f.agent('foreign');
  f.stream(foreign, 1);
  assert.equal(f.frames.length, 0);
  assert.throws(() => f.publisher.admit(foreign, true), /lineage/);
  const child = f.agent('child', 'root'); f.publisher.admit(child, false);
  const grandchild = f.agent('grandchild', 'child'); f.publisher.admit(grandchild, false);
  f.stream(grandchild, 1);
  assert.equal(f.frames[0].params.activity.scope, 'child');
  f.time(1000);
  f.stream(grandchild, 1);
  assert.equal(f.frames.length, 1, 'replayed revisions are not activity');
  child.session.header.parentSession = 'foreign';
  f.stream(grandchild, 2);
  assert.equal(f.frames.length, 1);
  child.session.header.parentSession = 'root';
  f.agents.set('root', { ...f.root });
  f.stream(grandchild, 3);
  assert.equal(f.frames.length, 1, 'a matching string ID does not establish the same live Agent');
  f.agents.set('root', f.root);
  f.handlers.get('agent/disposed')({ agent: child });
  f.stream(grandchild, 4);
  assert.equal(f.frames.length, 1);
  const single = fixture({ children: false });
  assert.throws(() => single.publisher.admit(single.agent('child', 'root'), false), /lineage/);
  assert.ok(single.publisher.capabilities.opaquePhases.includes('silent-tool'));
  assert.ok(single.publisher.capabilities.opaquePhases.includes('external-provider-without-activity'));
});

test('optional native-provider service validates actual Agent, per-call sequence and a content-free envelope', () => {
  const f = fixture({ idleTimeoutMs: 100 });
  const service = f.publisher.service;
  assert.equal(service.version, 1);
  assert.equal(service.intervalMs, 25);
  const progress = { version: 1, sequence: 1, kind: 'provider-progress', source: 'codex-pro', scope: 'child',
    eventTypes: ['item/reasoning/textDelta'], observedEvents: 42, invocationId: 'native-call-one' };
  assert.equal(service.report(f.root, progress), true);
  assert.equal(f.frames[0].params.activity.observedEvents, 42);
  f.time(100);
  assert.equal(service.report(f.root, progress), false);
  for (const bad of [{ ...progress, sequence: 0 }, { ...progress, source: 'foreign' },
    { ...progress, invocationId: '' }, { ...progress, text: 'PRIVATE_MODEL_BODY' },
    { ...progress, eventTypes: ['tool call body with spaces'] }, { ...progress, observedEvents: 1.1 }]) {
    assert.equal(service.report(f.root, bad), false);
  }
  assert.equal(service.report(f.agent('foreign'), { ...progress, sequence: 2 }), false);
  assert.equal(service.report(f.root, { ...progress, sequence: 2 }), true);
  assert.equal(f.frames.length, 2);
  assert.ok(!JSON.stringify(f.frames).includes('PRIVATE_MODEL_BODY'));
  f.publisher.close();
  assert.equal(service.report(f.root, { ...progress, sequence: 3 }), false);
});

test('SDK activity receiver rejects damaged owned metadata and ignores foreign and repeated observations', () => {
  const f = fixture(); f.stream(f.root, 1);
  const value = f.frames[0].params;
  assert.deepEqual(acceptDshActivity(value, f.identity, 0), value.activity);
  assert.equal(acceptDshActivity(value, f.identity, 1), null);
  for (const key of ['requestId', 'nonce', 'rootSessionId']) {
    assert.equal(acceptDshActivity({ ...value, [key]: 'foreign' }, f.identity, 0), null);
  }
  assert.equal(acceptDshActivity(null, f.identity, 0), null);
  for (const bad of [{ ...value, version: 2 }, { ...value, text: 'secret' },
    { ...value, activity: { ...value.activity, content: 'secret' } },
    { ...value, activity: { ...value.activity, eventTypes: [] } },
    { ...value, activity: { ...value.activity, eventTypes: ['assistant/start', 'assistant/start'] } }]) {
    assert.throws(() => acceptDshActivity(bad, f.identity, 0), /malformed owned/);
  }
  assert.equal(dshActivityInterval(), 1000);
  assert.equal(dshActivityInterval(1), 1);
  assert.equal(dshActivityInterval(1200), 300);
});


test('Web activity belongs to its exact host invocation, domain and admitted Session and contains no output', () => {
  const f = fixture({ web: true, idleTimeoutMs: 4 });
  const send = value => f.handlers.get('chatgpt-web/activity')(value);
  const progress = { hostInvocationId: 'owned-call', executionDomainId: 'owned-domain', sessionId: 'root', requestId: 'web-request', generation: 1, sequence: 1, kind: 'prompt-accepted' };
  for (const bad of [{ ...progress, hostInvocationId: 'old-call' }, { ...progress, executionDomainId: 'foreign-domain' },
    { ...progress, sessionId: 'foreign-session' }, { ...progress, requestId: '' }, { ...progress, sequence: 0 },
    { ...progress, generation: 0 }, { ...progress, kind: 'heartbeat' }, { ...progress, text: 'secret' }]) send(bad);
  assert.equal(f.frames.length, 0);
  send({ ...progress, bindingDigest: 'digest', at: Date.now(), observedBytes: 5 });
  assert.deepEqual(f.frames[0].params.activity.eventTypes, ['provider/chatgpt-web']);
  f.time(100); send(progress);
  assert.equal(f.frames.length, 1, 'replayed sequence cannot report progress');
  send({ ...progress, sequence: 2, kind: 'content-delta' });
  assert.equal(f.frames.length, 2);
  assert.ok(!JSON.stringify(f.frames).includes('web-request'), 'transport IDs stay inside the host validation boundary');
  f.dispose(); f.time(200); send({ ...progress, sequence: 3 });
  assert.equal(f.frames.length, 2);
});
