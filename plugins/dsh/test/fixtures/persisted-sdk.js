#!/usr/bin/env node
// The published SDK server, AgentLoop and JSONL persistence run unmodified.
// Only the model adapter and shell probe are offline fixtures; no credentials
// or provider endpoint are read, and the adapter makes no network request.
import { appendFileSync, readFileSync } from 'node:fs';
import { Context } from '@deepseek-ai/cordis';
import { LlmAdapter } from '@deepseek-ai/dsh-llm';
import { mountAgentLoopTestDependencies, mountAgentLoopTestHarness } from '@deepseek-ai/dsh-agent-loop-testkit';
import * as Persistence from '@deepseek-ai/dsh-session-persistence-jsonl';
import * as Server from '@deepseek-ai/dsh-sdk-jsonrpc-server';
import SandboxPolicy from '@deepseek-ai/dsh-sandbox-policy';
import Approval from '@deepseek-ai/dsh-user-approval';
import * as WorkerPolicy from '../../src/policy.js';
import { SessionTitleService } from '@deepseek-ai/dsh-session-title';
import { setTimeout as delay } from 'node:timers/promises';

const patch = JSON.parse(readFileSync(process.argv[process.argv.indexOf('--patch') + 1], 'utf8'));
const policy = patch.find(row => row.id === 'sandbox-policy').config;
const ctx = new Context();
await mountAgentLoopTestDependencies(ctx, { workingDirectory: true });
await ctx.plugin(Persistence, { root: patch.find(row => row.id === 'session-persistence-jsonl')?.config.root ?? process.env.TEST_DSH_HISTORY, compression: 'none' });
if (process.env.TEST_DSH_NO_TITLE_SERVICE !== '1') await ctx.plugin(SessionTitleService, { fallbackMaxWords: 8, fallbackMaxBytes: 96, maxTitleBytes: 120 });
await ctx.plugin(SandboxPolicy, policy);
await ctx.plugin(Approval, { policy: 'never' });
ctx.provide('shell', {
  sandboxMode: 'offline-fixture', resolve: request => request,
  execute: async request => ({ result: async () => ({ exitCode: 0, sandbox: {
    mode: request.sandboxPolicy.mode, denied: false,
    ...(request.sandboxPolicy.mode === 'danger-full-access' ? {} : { enforcement: 'full' }),
  } }) }),
});
const webScenario = process.env.TEST_DSH_WEB_SCENARIO;
let webIdentity, webDispatched = false, webCompleted = false;
const webBinding = { protocol: 1, transport: 'chatgpt-web', provider: 'offline-fixture', model: 'gpt-6-pro-web', reasoningEffort: 'pro',
  connectionId: 'offline-connection', accountFingerprint: 'offline-account', policyVersion: 'fixture-policy/1', contextPolicy: 'project-only',
  observation: 'opaque', usage: 'unknown', hostContractVersion: 1, runtimeVersion: 'web-runtime/1' };
if (webScenario && webScenario !== 'missing-host') {
  // Offline contract fixture only: the production host is implemented by the
  // ChatGPT plugin, with its own durable transport/commit journal and account.
  ctx.provide('compaction', { compactIfNeeded: async () => {} });
  ctx.provide('chatgptWebHost', {
    version: 1, admitBinding: async () => ({ ...webBinding, ...(webScenario === 'account-change' ? { accountFingerprint: 'different-account' } : {}) }),
    beginInvocation: async input => { webIdentity = { sessionId: input.sessionId, executionDomainId: input.executionDomainId, hostInvocationId: input.hostInvocationId }; },
    recoveryCheckpoint: async input => ({ version: 1, contract: 'chatgpt-web/read-only-recovery/1', identity: input, ownerPid: process.pid, fixture: true }),
    invocationFacts: async input => {
      if (JSON.stringify(input) !== JSON.stringify(webIdentity)) throw new Error('foreign fixture invocation');
      return { version: 1, ...input, dispatch: webDispatched ? 'dispatched' : 'not-dispatched',
        outcome: webScenario === 'unknown' && webDispatched ? 'unknown' : webCompleted ? 'completed' : 'failed',
        remoteGeneration: webScenario === 'unknown' && webDispatched ? 'unknown' : 'idle',
        localCleanupConfirmed: webScenario !== 'unclean', usageStatus: 'unknown' };
    },
  });
}
await ctx.plugin(WorkerPolicy, patch.find(row => row.insert).insert[0].config);

class OfflineAdapter extends LlmAdapter {
  async resolveModel(provider, model) {
    return { id: model, provider, name: model, reasoning: {
      efforts: (webScenario && webScenario !== 'unsupported-effort' ? ['pro'] : ['low', 'high', 'max']).map(id => ({ id, name: id })),
    } };
  }
  async *stream(request) {
    if (process.env.TEST_DSH_LLM_CALLS) appendFileSync(process.env.TEST_DSH_LLM_CALLS,
      JSON.stringify({ messages: request.messages, model: request.model, provider: request.provider,
        directories: ctx.sessions.list().map(session => ({ header: session.header.cwd, current: ctx.workingDirectory.get(session) })),
        titles: ctx.sessions.list().map(session => ctx.get('sessionTitle')?.get(session)?.title),
        reasoningEffort: request.reasoningEffort }) + '\n');
    if (webScenario) {
      await delay(15);
      webDispatched = true;
      ctx.emit('chatgpt-web/activity', { ...webIdentity, requestId: 'offline-browser-request', generation: 1, sequence: 1, kind: 'prompt-accepted' });
      if (webScenario === 'rejected-response') throw new Error('captured Web response rejected by the original protocol');
    }
    yield { type: 'block-start', index: 0, blockType: 'text' };
    const answer = 'published SDK fixture complete';
    const chunks = Number(process.env.TEST_DSH_STREAM_CHUNKS ?? 1);
    for (let i = 0; i < chunks; i++) {
      if (process.env.TEST_DSH_STREAM_DELAY_MS) await delay(Number(process.env.TEST_DSH_STREAM_DELAY_MS), undefined, { signal: request.signal });
      yield { type: 'text-delta', index: 0, text: answer.slice(Math.floor(i * answer.length / chunks), Math.floor((i + 1) * answer.length / chunks)) };
    }
    yield { type: 'block-end', index: 0, block: { type: 'text', text: 'published SDK fixture complete' } };
    yield { type: 'usage', usage: { inputTokens: 10, outputTokens: 2, totalTokens: 12 } };
    if (webScenario) webCompleted = true;
    yield { type: 'finish', reason: { kind: 'completed' } };
  }
}
ctx.llm.registerAdapter(['offline-fixture'], new OfflineAdapter());
await mountAgentLoopTestHarness(ctx);
await ctx.plugin(Server, { maxTokensAsSuccess: false });
