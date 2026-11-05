#!/usr/bin/env node
// Protocol fixture only. This file never imports a model SDK or reads a key.
import { appendFileSync, readFileSync } from 'node:fs';
import { createInterface } from 'node:readline';

const scenario = process.env.TEST_DSH_SCENARIO ?? 'ok';
const patchPath = process.argv[process.argv.indexOf('--patch') + 1];
const patch = JSON.parse(readFileSync(patchPath, 'utf8'));
const policy = patch.find(row => row.id === 'sandbox-policy').config;
const log = row => { if (process.env.TEST_DSH_LOG) appendFileSync(process.env.TEST_DSH_LOG, JSON.stringify(row) + '\n'); };
const send = value => process.stdout.write(JSON.stringify({ jsonrpc: '2.0', ...value }) + '\n');
const reply = (id, result) => send({ id, result });
let seq = 0;
const event = (sessionId, type, data) => {
  const value = { method: 'session.event', params: { sessionId, event: { seq: seq++, type, data } } };
  send(value);
  return value;
};
let initialized;
if (scenario === 'forced-shutdown') process.on('SIGTERM', () => {});
for await (const line of createInterface({ input: process.stdin })) {
  const request = JSON.parse(line);
  log(request);
  const { id, method, params } = request;
  if (method === 'initialize') {
    initialized = params;
    if (scenario === 'initialize-hang') continue;
    if (scenario === 'malformed') { process.stdout.write('not JSON\n'); continue; }
    reply(id, { serverInfo: { name: 'deepseek-harness-sdk-runtime', version: '0.0.1' } });
  } else if (method === 'session/working-directory/get') {
    const fullAccess = policy.mode === 'danger-full-access';
    if (scenario !== 'missing-policy') event(params.sessionId, 'flywheel/policy', {
      protocol: 1, cwd: policy.workspaceRoot,
      permissionMode: scenario === 'wrong-policy' ? (fullAccess ? 'workspace-write' : 'danger-full-access') : policy.mode,
      approvalPolicy: scenario === 'wrong-approval' ? 'ask' : 'never', confinedShell: !fullAccess,
      ...(fullAccess ? {} : { enforcement: 'full' }),
      ...(scenario === 'forged-confinement' ? { confinedShell: true, enforcement: 'full' } : {}),
      ...(scenario === 'unexpected-enforcement' ? { enforcement: 'partial' } : {}),
    });
    reply(id, { cwd: scenario === 'wrong-cwd' ? '/' : initialized.cwd });
  } else if (method === 'session/prompt') {
    event(params.sessionId, 'agent/inbox/spliced', { inserted: [{ id: 'message-1' }] });
    if (scenario !== 'empty') {
      const value = event(params.sessionId, 'assistant/message', {
        message: { id: 'assistant-1', content: [{ type: 'text', text: 'fixture complete' }] },
        ...(scenario === 'unknown-usage' ? {} : { usage: {
          inputTokens: 100, cacheReadTokens: 20, outputTokens: 8, reasoningTokens: 3, totalTokens: 128,
        } }),
      });
      if (scenario === 'duplicate') send(value);
    }
    if (scenario !== 'missing-terminal') event(params.sessionId, 'turn/end', {
      turn: 0, reason: { kind: scenario === 'max-tokens' ? 'max-tokens' : 'completed' },
    });
    reply(id, { messageId: 'message-1' });
  } else if (method === 'session/wait') {
    if (scenario !== 'wait-hang') reply(id, {});
    if (scenario === 'corrupt-tail') process.stdout.write('corrupted-tail\n');
  } else if (method === 'shutdown') {
    if (scenario === 'forced-shutdown') continue;
    reply(id, {});
    process.exit(0);
  } else {
    send({ id, error: { code: -32601, message: 'unknown method' } });
  }
}
if (scenario === 'forced-shutdown') setInterval(() => {}, 1000);
