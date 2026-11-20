#!/usr/bin/env node
// Protocol fixture only. This file never imports a model SDK or reads a key.
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import { createInterface } from 'node:readline';

const scenario = process.env.TEST_DSH_SCENARIO ?? 'ok';
const failureScenario = scenario.replace(/-(?:corrupt|incomplete)-tail(?:-unclean-exit)?$/, '');
const accountFailure = {
  'account-sign-in-required': { code: 'ACCOUNT_SIGN_IN_REQUIRED', message: 'Account route requires sign-in or an eligible request destination.' },
  'account-token-invalid': { code: 'ACCOUNT_TOKEN_INVALID', message: 'Native provider rejected the request.' },
  'account-quota': { code: 'ACCOUNT_QUOTA', message: 'Native provider quota is exhausted.' },
  'web-uncertain': { code: 'WEB_UNCERTAIN', message: 'Reconcile the original browser submission.' },
  'web-delivery-claimed': { code: 'WEB_DELIVERY_CLAIMED', message: 'The tool response was already claimed.' },
  'web-connection-lost': { code: 'WEB_CONNECTION_LOST', message: 'The original delivery connection was lost.' },
  'web-protocol-error': { code: 'WEB_PROTOCOL_ERROR', message: 'The complete response could not be verified.' },
  'web-journal-write': { code: 'WEB_JOURNAL_WRITE', message: 'The journal could not establish post-Send durability.' },
}[failureScenario];
const corruptShutdownTail = scenario !== 'corrupt-tail' && scenario.includes('-corrupt-tail');
const incompleteShutdownTail = scenario.includes('-incomplete-tail');
const uncleanShutdownExit = scenario.endsWith('-unclean-exit');
const patchPath = process.argv[process.argv.indexOf('--patch') + 1];
const patch = JSON.parse(readFileSync(patchPath, 'utf8'));
const policy = patch.find(row => row.id === 'sandbox-policy').config;
const admission = patch.find(row => row.insert).insert[0].config;
const log = row => { if (process.env.TEST_DSH_LOG) appendFileSync(process.env.TEST_DSH_LOG, JSON.stringify(row) + '\n'); };
const send = value => process.stdout.write(JSON.stringify({ jsonrpc: '2.0', ...value }) + '\n');
const reply = (id, result) => send({ id, result });
const activity = sessionId => {
  const params = { version: 1, requestId: admission.requestId, nonce: admission.nonce, rootSessionId: sessionId,
    activity: { version: 1, sequence: 1, kind: 'provider-progress', source: 'dsh', scope: 'root',
      eventTypes: ['assistant/reasoning-delta'], observedEvents: 50 } };
  if (scenario === 'activity-foreign') params.rootSessionId = 'foreign-session';
  if (scenario === 'activity-malformed') params.activity.text = 'private text must not become activity';
  send({ method: 'flywheel.activity', params });
};
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
    if (scenario === 'account-code-in-rpc-message') {
      send({ id, error: { code: -32603, message: 'ACCOUNT_TOKEN_INVALID: native detail without a durable error code' } });
      continue;
    }
    if (scenario.startsWith('initialize-long-')) {
      const text = { ascii: 'x'.repeat(9000), multibyte: '初始失败😀'.repeat(2500),
        escaped: '"\\\n\t\u0000'.repeat(2000) }[scenario.slice('initialize-long-'.length)];
      send({ id, error: { code: -32603, message: 'initialization-error:' + text } });
      continue;
    }
    reply(id, { serverInfo: { name: 'deepseek-harness-sdk-runtime', version: '0.0.1' } });
  } else if (method === 'session/resume') {
    if (scenario === 'resume-missing' || scenario === 'resume-busy') {
      send({ id, error: { code: -32603, message: scenario === 'resume-busy' ? 'SDK session is already open or opening' : 'Session history is missing' } });
      continue;
    }
    if (scenario === 'resume-hang') continue;
    // Old records are a deliberately adversarial replay; the real public SDK
    // does not notify them. They must not supply this invocation's usage/text.
    if (scenario === 'resume-replay') {
      event(params.sessionId, 'assistant/message', { message: { content: [{ type: 'text', text: 'old answer' }] },
        usage: { inputTokens: 99999, outputTokens: 99999 } });
      event(params.sessionId, 'turn/end', { turn: 0, reason: { kind: 'completed' } });
    }
    reply(id, scenario === 'resume-invalid-result' ? { sessionId: 'replacement' } : {});
  } else if (method === 'session/working-directory/get') {
    const fullAccess = policy.mode === 'danger-full-access';
    if (scenario !== 'missing-policy') writeFileSync(admission.receiptPath, JSON.stringify({
      protocol: 1, cwd: policy.workspaceRoot, requestId: admission.requestId, sessionId: params.sessionId, nonce: admission.nonce,
      permissionMode: scenario === 'wrong-policy' ? (fullAccess ? 'workspace-write' : 'danger-full-access') : policy.mode,
      approvalPolicy: scenario === 'wrong-approval' ? 'ask' : 'never', confinedShell: !fullAccess,
      ...(fullAccess ? {} : { enforcement: 'full' }),
      ...(scenario === 'forged-confinement' ? { confinedShell: true, enforcement: 'full' } : {}),
      ...(scenario === 'unexpected-enforcement' ? { enforcement: 'partial' } : {}),
      ...(scenario.startsWith('activity-') && scenario !== 'activity-unattested' ? { activity: { version: 1, observation: 'events-only' } } : {}),
      ...(admission.web && scenario !== 'web-not-ready' ? { web: { ...admission.web, ready: true, usage: 'unknown', idle: 'wall-only',
        summary: { provider: 'chatgpt', model: 'gpt-6.2-sol' }, title: 'deterministic' } } : {}),
    }), { mode: 0o600, flag: 'wx' });
    if (scenario === 'activity-before-ready') activity(params.sessionId);
    reply(id, { cwd: scenario === 'wrong-cwd' ? '/' : initialized.cwd });
  } else if (method === 'session/prompt') {
    if (scenario.startsWith('activity-')) activity(params.sessionId);
    if (scenario === 'activity-duplicate') activity(params.sessionId);
    event(params.sessionId, 'agent/inbox/spliced', { inserted: [{ id: 'message-1' }] });
    if (scenario !== 'empty' && !accountFailure && scenario !== 'account-signed-out') {
      const value = event(params.sessionId, 'assistant/message', {
        message: { id: 'assistant-1', content: [{ type: 'text', text: 'fixture complete' }] },
        ...(scenario === 'unknown-usage' ? {} : { usage: {
          inputTokens: 100, cacheReadTokens: 20, outputTokens: 8, reasoningTokens: 3, totalTokens: 128,
        } }),
      });
      if (scenario === 'duplicate') send(value);
    }
    if (scenario !== 'missing-terminal') event(params.sessionId, 'turn/end', {
      turn: scenario === 'invalid-turn' ? 'not-a-turn' : 1, reason: accountFailure ? { kind: 'error', error: accountFailure }
        : scenario === 'account-signed-out'
          ? { kind: 'aborted', reason: { kind: 'hook', reason: 'deepseek-account/signed-out' } }
          : { kind: scenario === 'unknown-turn' ? 'future-unknown-terminal' : scenario === 'max-tokens' ? 'max-tokens' : 'completed' },
    });
    reply(id, { messageId: 'message-1' });
  } else if (method === 'session/wait') {
    if (scenario !== 'wait-hang') reply(id, scenario === 'invalid-wait' ? null : {});
    if (scenario === 'corrupt-tail') process.stdout.write('corrupted-tail\n');
  } else if (method === 'shutdown') {
    if (scenario === 'forced-shutdown') continue;
    if (scenario === 'slow-shutdown') await new Promise(resolve => setTimeout(resolve, 700));
    reply(id, {});
    if (corruptShutdownTail || incompleteShutdownTail) {
      // Flush the injected tail before exit so the pipe, not scheduling, decides
      // whether the worker observes a malformed line or an incomplete EOF frame.
      await new Promise(resolve => process.stdout.write(corruptShutdownTail ? 'corrupted-tail\n' : '{"jsonrpc":', resolve));
      process.exit(uncleanShutdownExit ? 1 : 0);
    }
    process.exit(0);
  } else {
    send({ id, error: { code: -32601, message: 'unknown method' } });
  }
}
if (scenario === 'forced-shutdown') setInterval(() => {}, 1000);
