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

const patch = JSON.parse(readFileSync(process.argv[process.argv.indexOf('--patch') + 1], 'utf8'));
const policy = patch.find(row => row.id === 'sandbox-policy').config;
const ctx = new Context();
await mountAgentLoopTestDependencies(ctx, { workingDirectory: true });
await ctx.plugin(Persistence, { root: patch.find(row => row.id === 'session-persistence-jsonl')?.config.root ?? process.env.TEST_DSH_HISTORY, compression: 'none' });
await ctx.plugin(SessionTitleService, { fallbackMaxWords: 8, fallbackMaxBytes: 96, maxTitleBytes: 120 });
await ctx.plugin(SandboxPolicy, policy);
await ctx.plugin(Approval, { policy: 'never' });
ctx.provide('shell', {
  sandboxMode: 'offline-fixture', resolve: request => request,
  execute: async request => ({ result: async () => ({ exitCode: 0, sandbox: {
    mode: request.sandboxPolicy.mode, denied: false,
    ...(request.sandboxPolicy.mode === 'danger-full-access' ? {} : { enforcement: 'full' }),
  } }) }),
});
await ctx.plugin(WorkerPolicy, patch.find(row => row.insert).insert[0].config);

class OfflineAdapter extends LlmAdapter {
  async resolveModel(provider, model) {
    return { id: model, provider, name: model, reasoning: {
      efforts: ['low', 'high', 'max'].map(id => ({ id, name: id })),
    } };
  }
  async *stream(request) {
    if (process.env.TEST_DSH_LLM_CALLS) appendFileSync(process.env.TEST_DSH_LLM_CALLS,
      JSON.stringify({ messages: request.messages, model: request.model, provider: request.provider,
        directories: ctx.sessions.list().map(session => ({ header: session.header.cwd, current: ctx.workingDirectory.get(session) })),
        titles: ctx.sessions.list().map(session => ctx.sessionTitle.get(session)?.title),
        reasoningEffort: request.reasoningEffort }) + '\n');
    yield { type: 'block-start', index: 0, blockType: 'text' };
    yield { type: 'text-delta', index: 0, text: 'published SDK fixture complete' };
    yield { type: 'block-end', index: 0, block: { type: 'text', text: 'published SDK fixture complete' } };
    yield { type: 'usage', usage: { inputTokens: 10, outputTokens: 2, totalTokens: 12 } };
    yield { type: 'finish', reason: { kind: 'completed' } };
  }
}
ctx.llm.registerAdapter(['offline-fixture'], new OfflineAdapter());
await mountAgentLoopTestHarness(ctx);
await ctx.plugin(Server, { maxTokensAsSuccess: false });
