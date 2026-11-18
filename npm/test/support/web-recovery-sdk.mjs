#!/usr/bin/env node
// Produce original evidence through the published SDK, AgentLoop and JSONL
// backend plus the actual vendor ChatGptWebHost. This single-turn fixture uses
// an offline page and sandbox probe, no-op compaction, and the durable ACK
// adapter below. It does not claim to exercise the complete patched host.
// Recovery never imports or starts this process.
import assert from 'node:assert/strict';
import { appendFileSync, readFileSync } from 'node:fs';
import { mkdir } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import * as WorkerPolicy from '../../../plugins/dsh/src/policy.js';

const sourceRequire = createRequire(new URL('../../../plugins/dsh/package.json', import.meta.url));
const load = name => import(pathToFileURL(sourceRequire.resolve(name)));
const { Context } = await load('@deepseek-ai/cordis');
const { LlmAdapter } = await load('@deepseek-ai/dsh-llm');
const { mountAgentLoopTestDependencies, mountAgentLoopTestHarness } = await load('@deepseek-ai/dsh-agent-loop-testkit');
const Persistence = await load('@deepseek-ai/dsh-session-persistence-jsonl');
const Server = await load('@deepseek-ai/dsh-sdk-jsonrpc-server');
const { default: SandboxPolicy } = await load('@deepseek-ai/dsh-sandbox-policy');
const { default: Approval } = await load('@deepseek-ai/dsh-user-approval');
const { ChatGptWebHost, webHostCapabilities } = await load('dsh-plugin-chatgpt/web-host');

const patch = JSON.parse(readFileSync(process.argv[process.argv.indexOf('--patch') + 1], 'utf8'));
const config = patch.find(row => row.insert).insert[0].config;
const stateDir = process.env.FLYWHEEL_TEST_WEB_STATE;
const log = value => appendFileSync(process.env.FLYWHEEL_TEST_WEB_CALLS, JSON.stringify(value) + '\n');
log({ kind: 'sdk-start' });
await mkdir(stateDir, { recursive: true, mode: 0o700 });
const ctx = new Context();
await mountAgentLoopTestDependencies(ctx, { workingDirectory: true });
await ctx.plugin(Persistence, { root: patch.find(row => row.id === 'session-persistence-jsonl')?.config.root ?? process.env.FLYWHEEL_TEST_WEB_HISTORY, compression: 'none' });
await ctx.plugin(SandboxPolicy, patch.find(row => row.id === 'sandbox-policy').config);
await ctx.plugin(Approval, { policy: 'never' });
ctx.provide('shell', {
  sandboxMode: 'offline-fixture', resolve: request => request,
  execute: async request => {
    assert.equal(request.command, ':', 'the fixture permits only the readiness probe');
    log({ kind: 'shell-probe' });
    return { result: async () => ({ exitCode: 0, sandbox: { mode: request.sandboxPolicy.mode, denied: false } }) };
  },
});
ctx.provide('compaction', { compactIfNeeded: async () => {} });
ctx.on('tools/execute', () => { log({ kind: 'tool' }); throw new Error('this final-answer fixture must not execute a tool'); });

const answer = 'Original durable answer from the installed recovery fixture';
const host = new ChatGptWebHost(stateDir, config.provider, {
  capabilities: () => webHostCapabilities.requiredHost,
  activity(event) {
    const kinds = { accepted: 'prompt-accepted', content: 'content-delta', phase: 'phase-change', completed: 'response-completed' };
    ctx.emit('chatgpt-web/activity', { ...event, kind: kinds[event.kind] ?? event.kind });
  },
  pageFactory: {
    async connect() { throw new Error('the original fixture cannot connect to a browser'); },
    async open(connection, request) {
      log({ kind: 'page-open', requestId: request.requestId });
      const conversation = { id: request.requestId, url: 'https://chatgpt.com/c/' + request.requestId,
        projectUrl: 'https://chatgpt.com/g/g-packed-fixture/project' };
      const evidence = { adapter: 'chatgpt-dom/1', model: 'GPT-6', effort: 'Pro',
        accountFingerprint: connection.accountFingerprint, contextPolicy: request.contextPolicy,
        conversationUrl: conversation.url, projectUrl: conversation.projectUrl,
        bindingDigest: request.bindingDigest, capturedAt: Date.now() };
      return {
        targetId: 'packed-original-page', prepare: async () => {
          if (process.env.FLYWHEEL_TEST_WEB_FAIL_BEFORE_SUBMIT === '1') throw new Error('original browser admission failed before submit');
          return { conversation, evidence };
        },
        inspect: async () => ({ conversation, evidence }), async submit() { log({ kind: 'page-submit' }); },
        async close() {}, async stop() { return false; },
        async sample() {
          return { conversation, userMessageId: 'user-' + request.requestId, acceptedPrompt: request.prompt,
            responseId: 'answer-' + request.requestId,
            text: JSON.stringify({ protocol: 'dsh-chatgpt-web/1', requestId: JSON.parse(request.prompt).requestId,
              generation: request.generation, kind: 'final', text: answer }),
            stopVisible: false, thinking: false, finishedActions: true };
        },
      };
    },
  },
});
const settings = await host.settings.read();
settings.enabled = true;
settings.connections.default.accountFingerprint = 'a'.repeat(64);
await host.settings.save(settings);

// The published SDK exposes the real persistence barrier. Emulate only the
// patched-host ACK hook, after checking the stored assistant event itself;
// never manufacture a receipt or acknowledge merely streamed text.
let commits = Promise.resolve();
ctx.on('session/event', (session, event) => {
  if (event.type !== 'assistant/message') return;
  const message = event.data.message;
  const reference = message.source.replayState?.response;
  if (!reference) throw new Error('the original assistant event lost its Web reference');
  commits = commits.then(async () => {
    await ctx.sessionPersistence.flush();
    const reader = await ctx.sessionPersistence.open(session.id, 'read');
    try {
      const stored = (await reader.read()).events.find(value => value.seq === event.seq);
      assert.deepEqual(stored, event, 'the ACK must identify the actual persisted SDK event');
    } finally { await reader.close(); }
    const hostCommitId = `assistant:${session.id}:${event.seq}:${message.id}`;
    await host.committed(reference, message.content, hostCommitId, true);
    log({ kind: 'durable-ack', hostCommitId, sessionId: session.id, eventSeq: event.seq, text: answer });
  });
  // Preserve failure for terminal facts without creating an unhandled rejection.
  void commits.catch(() => {});
});
ctx.provide('chatgptWebHost', {
  version: host.version,
  admitBinding: input => host.admitBinding(input),
  beginInvocation: input => host.beginInvocation(input),
  recoveryCheckpoint: input => host.recoveryCheckpoint(input),
  async invocationFacts(input) {
    await commits;
    await host.agentIdle(input.sessionId);
    return await host.invocationFacts(input);
  },
});
await ctx.plugin(WorkerPolicy, config);

class OriginalWebAdapter extends LlmAdapter {
  async resolveModel(provider, model) {
    return { id: model, provider, name: model, reasoning: { efforts: [{ id: 'pro', name: 'Pro' }] } };
  }
  async *stream(request) {
    log({ kind: 'model', sessionId: request.sessionId });
    const sessionId = String(request.sessionId);
    const context = await host.callContext(request, 'main', { sessionId, rootSessionId: sessionId, workspace: config.cwd });
    yield* host.wrap(context, async function* () {
      const delivery = await host.generate();
      assert.deepEqual(delivery.blocks, [{ type: 'text', text: answer }]);
      yield { type: 'block-start', index: 0, blockType: 'text' };
      yield { type: 'text-delta', index: 0, text: answer };
      yield { type: 'block-end', index: 0, block: delivery.blocks[0] };
      yield { type: 'finish', reason: { kind: 'stop' }, replayState: { response: delivery.reference } };
    });
  }
}
ctx.llm.registerAdapter([config.provider], new OriginalWebAdapter());
await mountAgentLoopTestHarness(ctx);
await ctx.plugin(Server, { maxTokensAsSuccess: false });
