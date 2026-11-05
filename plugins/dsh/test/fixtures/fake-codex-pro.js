#!/usr/bin/env node
// An offline app-server peer; no model, network or account is contacted.
import { appendFileSync } from 'node:fs';
import { createInterface } from 'node:readline';
import { spawn } from 'node:child_process';

const scenario = process.env.TEST_CODEX_PRO_SCENARIO || 'ok';
const log = value => { if (process.env.TEST_CODEX_PRO_LOG) appendFileSync(process.env.TEST_CODEX_PRO_LOG, JSON.stringify(value) + '\n'); };
log({ event: 'spawn', pid: process.pid, supervisorPid: process.ppid, argv: process.argv.slice(2), cwd: process.cwd() });
if (process.argv.length !== 4 || process.argv[2] !== 'app-server' || process.argv[3] !== '--stdio') process.exit(91);
const send = value => process.stdout.write(JSON.stringify(value) + '\n');
const notify = (method, params) => send({ jsonrpc: '2.0', method, params });
const reply = (id, result) => send({ jsonrpc: '2.0', id, result });
const threadId = 'fixture-thread', turnId = 'fixture-turn';
let stopping = false;
// Keep the process alive even when its protocol input closes, so this case
// really requires escalation rather than falling out of the Node event loop.
if (scenario === 'stubborn') setInterval(() => {}, 1000);
let descendantExit;
if (scenario === 'descendant') {
  const descendant = spawn(process.execPath, ['-e', `
    process.on('SIGTERM', () => setTimeout(() => process.exit(0), 20));
    process.stdout.write('ready'); setInterval(() => {}, 1000);
  `], { stdio: ['ignore', 'pipe', 'ignore'] });
  descendant.stdout.once('data', () => log({ event: 'descendant', pid: descendant.pid }));
  descendantExit = new Promise(resolve => descendant.once('close', resolve));
}
process.on('SIGTERM', () => {
  log({ event: 'SIGTERM' });
  if (scenario === 'stubborn' || stopping) return;
  stopping = true;
  Promise.resolve(descendantExit).then(() => setTimeout(() => { log({ event: 'cleanup' }); process.exit(0); }, 50));
});
process.stdout.on('error', () => process.exit(1));
createInterface({ input: process.stdin }).on('line', line => {
  const request = JSON.parse(line);
  log(request);
  if (request.method === 'initialize') {
    if (scenario !== 'initialize-hang') reply(request.id, { userAgent: 'offline-fixture' });
  } else if (request.method === 'thread/start') {
    const { cwd, model, sandbox, approvalPolicy, ephemeral } = request.params;
    const policy = sandbox === 'danger-full-access' ? { type: 'dangerFullAccess' } : sandbox === 'read-only'
      ? { type: 'readOnly', networkAccess: false }
      : { type: 'workspaceWrite', writableRoots: [cwd], networkAccess: false, excludeTmpdirEnvVar: true, excludeSlashTmp: true };
    if (scenario === 'broad-policy') policy.writableRoots = ['/'];
    reply(request.id, { thread: { id: threadId, ephemeral }, cwd: scenario === 'wrong-cwd' ? '/' : cwd,
      model: scenario === 'wrong-model' ? 'unexpected-model' : model,
      approvalPolicy: scenario === 'wrong-approval' ? 'on-request' : approvalPolicy,
      sandbox: scenario === 'wrong-sandbox'
        ? (sandbox === 'danger-full-access' ? { type: 'workspaceWrite', writableRoots: [cwd] } : { type: 'dangerFullAccess' })
        : policy });
  } else if (request.method === 'turn/start') {
    reply(request.id, { turn: { id: turnId } });
    notify('turn/started', { threadId, turn: { id: turnId } });
    notify('item/completed', { threadId, turnId, item: { type: 'agentMessage', phase: 'commentary', text: 'fixture working' } });
    if (scenario === 'progress-hang') {
      setInterval(() => notify('item/completed', { threadId, turnId,
        item: { type: 'agentMessage', phase: 'commentary', text: 'fixture active' } }), 20);
      return;
    }
    if (scenario === 'progress-limit') {
      for (let count = 0; count < 5000; count++) notify('item/completed', { threadId, turnId,
        item: { type: 'agentMessage', phase: 'commentary', text: 'x' } });
      return;
    }
    if (['hang', 'stubborn', 'descendant'].includes(scenario)) return;
    if (scenario === 'output-limit') { process.stdout.write('x'.repeat(1100000)); return; }
    if (scenario === 'malformed') { process.stdout.write('{bad-json\n'); return; }
    if (scenario !== 'unknown-usage') {
      const total = { totalTokens: 108, inputTokens: 100, cachedInputTokens: 25, outputTokens: 8, reasoningOutputTokens: 3 };
      notify('thread/tokenUsage/updated', { threadId, turnId, tokenUsage: { total, last: total } });
      if (scenario === 'partial-usage') notify('thread/tokenUsage/updated', { threadId, turnId, tokenUsage: { total: { ...total, outputTokens: 'invalid' } } });
    }
    notify('account/rateLimits/updated', { rateLimits: { fixture: { usedPercent: 12 } } });
    notify('item/completed', { threadId, turnId, item: { type: 'agentMessage', phase: 'final_answer', text: 'fixture complete\n' } });
    notify('turn/completed', { threadId, turn: { id: turnId, status: 'completed' } });
  } else if (request.method === 'turn/interrupt') {
    reply(request.id, {});
    notify('turn/completed', { threadId, turn: { id: turnId, status: 'interrupted' } });
  }
});
