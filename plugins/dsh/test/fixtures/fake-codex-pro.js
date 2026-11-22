#!/usr/bin/env node
// An offline app-server peer; no model, network or account is contacted.
import { appendFileSync, existsSync, readFileSync, writeFileSync } from 'node:fs';
import { createInterface } from 'node:readline';
import { spawn } from 'node:child_process';
import { once } from 'node:events';

const scenario = process.env.TEST_CODEX_PRO_SCENARIO || 'ok';
const log = value => { if (process.env.TEST_CODEX_PRO_LOG) appendFileSync(process.env.TEST_CODEX_PRO_LOG, JSON.stringify(value) + '\n'); };
log({ event: 'spawn', pid: process.pid, supervisorPid: process.ppid, argv: process.argv.slice(2), cwd: process.cwd() });
const argv = process.argv.slice(2), config = {};
while (argv[0] === '-c') {
  argv.shift();
  const pair = argv.shift(); const equals = pair.indexOf('=');
  const value = pair.slice(equals + 1).replace(/([A-Za-z_][A-Za-z0-9_]*)\s*=/g, '"$1":');
  config[pair.slice(0, equals)] = JSON.parse(value);
}
if (argv.length !== 2 || argv[0] !== 'app-server' || argv[1] !== '--stdio') process.exit(91);
const send = value => process.stdout.write(JSON.stringify(value) + '\n');
const notify = (method, params) => send({ jsonrpc: '2.0', method, params });
const reply = (id, result) => send({ jsonrpc: '2.0', id, result });
const threadId = 'fixture-thread', turnId = `fixture-turn-${process.pid}`;
const historyPath = process.env.TEST_CODEX_PRO_HISTORY;
let history = historyPath && existsSync(historyPath) ? JSON.parse(readFileSync(historyPath, 'utf8')) : null;
let persistent = false;
const saveHistory = () => { if (persistent && historyPath) writeFileSync(historyPath, JSON.stringify(history)); };
let stopping = false;
let subscriptionAccountReads = 0;
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
  if (scenario === 'subscription-stop-exit-nonzero') process.exit(23);
  if (scenario === 'subscription-stop-signal' || scenario === 'subscription-stop-unexpected-signal') {
    const signal = scenario === 'subscription-stop-signal' ? 'SIGTERM' : 'SIGHUP';
    process.removeAllListeners(signal); process.kill(process.pid, signal); return;
  }
  Promise.resolve(descendantExit).then(() => setTimeout(() => { log({ event: 'cleanup' }); process.exit(0); }, 50));
});
process.stdout.on('error', () => process.exit(1));

// Real owned notifications are deliberately separated from display progress.
// These scenarios exercise every installed adapter/worker/watchdog layer.
async function observedTurn() {
  if (scenario === 'activity-quiet') return;
  const child = ['activity-child', 'activity-default-child', 'activity-completed-child'].includes(scenario);
  const activeThread = child ? 'activity-child-thread' : scenario === 'activity-foreign' ? 'foreign-thread' : threadId;
  const activeTurn = child ? 'activity-child-turn' : scenario === 'activity-stale-root' ? 'preceding-root-turn' : turnId;
  if (child) {
    notify('item/completed', { threadId, turnId, item: { type: 'collabAgentToolCall', id: 'activity-spawn',
      tool: 'spawnAgent', senderThreadId: threadId, receiverThreadIds: [activeThread],
      agentsStates: { [activeThread]: { status: 'running' } } } });
    notify('turn/started', { threadId: activeThread, turn: { id: activeTurn } });
  }
  const completeChild = () => {
    notify('thread/tokenUsage/updated', { threadId: activeThread, turnId: activeTurn,
      tokenUsage: { total: { totalTokens: 15, inputTokens: 10, outputTokens: 5, cachedInputTokens: 0, reasoningOutputTokens: 0 } } });
    notify('turn/completed', { threadId: activeThread, turn: { id: activeTurn, status: 'completed' } });
  };
  if (scenario === 'activity-completed-child') completeChild();
  const burst = scenario === 'activity-raw-volume' || scenario === 'activity-display-volume';
  const count = burst ? 420 : 70;
  const kinds = ['item/reasoning/summaryTextDelta', 'item/reasoning/textDelta', 'item/agentMessage/delta', 'item/commandExecution/outputDelta'];
  let rawBytes = 0;
  for (let i = 0; i < count; i++) {
    if (stopping) return;
    const delta = burst ? 'x'.repeat(4096) : 'PRIVATE_REASONING_AND_TOOL_CONTENT';
    rawBytes += Buffer.byteLength(delta);
    const writable = scenario === 'activity-display-volume'
      ? notify('item/completed', { threadId, turnId, item: { type: 'agentMessage', phase: 'commentary', text: delta } })
      : notify(kinds[i % kinds.length], { threadId: activeThread, turnId: activeTurn, itemId: 'activity-item', delta });
    if (!writable) await once(process.stdout, 'drain');
    if (!burst) await new Promise(resolve => setTimeout(resolve, 50));
  }
  log({ event: 'activity-complete', count, rawBytes, scope: child ? 'child' : 'root' });
  if (child && scenario !== 'activity-completed-child') completeChild();
  const delta = { totalTokens: 108, inputTokens: 100, cachedInputTokens: 25, outputTokens: 8, reasoningOutputTokens: 3 };
  const total = Object.fromEntries(Object.entries(delta).map(([key, value]) => [key, value + (history?.total?.[key] ?? 0)]));
  notify('thread/tokenUsage/updated', { threadId, turnId, tokenUsage: { total, last: delta } });
  history = { ...history, threadId, turnId, total }; saveHistory();
  notify('item/completed', { threadId, turnId, item: { type: 'agentMessage', phase: 'final_answer', text: 'fixture complete\n' } });
  notify('turn/completed', { threadId, turn: { id: turnId, status: 'completed' } });
}

// Child notifications and the parent's spawn result use independent listeners.
// The parent's status snapshot can therefore arrive after a real child terminal
// event, or after the same child has already begun its next turn.
function reorderedNativeChild() {
  const childId = 'child-thread', firstTurn = 'child-turn', nextTurn = 'child-turn-2';
  const hasNextTurn = scenario !== 'native-stale-spawn';
  notify('thread/started', { thread: { id: childId,
    source: { subAgent: { thread_spawn: { parent_thread_id: threadId } } } } });
  notify('turn/started', { threadId: childId, turn: { id: firstTurn } });
  notify('thread/tokenUsage/updated', { threadId: childId, turnId: firstTurn,
    tokenUsage: { total: { totalTokens: 15, inputTokens: 10, outputTokens: 5, cachedInputTokens: 0, reasoningOutputTokens: 0 } } });
  notify('turn/completed', { threadId: childId, turn: { id: firstTurn, status: 'completed' } });
  if (hasNextTurn) notify('turn/started', { threadId: childId, turn: { id: nextTurn } });
  notify('item/completed', { threadId, turnId, item: { type: 'collabAgentToolCall', id: 'spawn-late',
    tool: 'spawnAgent', senderThreadId: threadId, receiverThreadIds: [childId], model: 'child-model',
    agentsStates: { [childId]: { status: hasNextTurn ? 'completed' : 'running' } } } });
  if (scenario === 'native-new-turn-completed') {
    notify('thread/tokenUsage/updated', { threadId: childId, turnId: nextTurn,
      tokenUsage: { total: { totalTokens: 30, inputTokens: 20, outputTokens: 10, cachedInputTokens: 0, reasoningOutputTokens: 0 } } });
    notify('turn/completed', { threadId: childId, turn: { id: nextTurn, status: 'completed' } });
  }
}

// A parent-only terminal snapshot must survive replay of an older creation
// snapshot, even when no child listener supplied a turn ID or usage evidence.
function replayedParentSnapshot() {
  const snapshot = (tool, id, status) => notify('item/completed', { threadId, turnId,
    item: { type: 'collabAgentToolCall', id, tool, senderThreadId: threadId,
      receiverThreadIds: ['child-thread'], model: 'child-model', agentsStates: { 'child-thread': { status } } } });
  snapshot('spawnAgent', 'spawn-replayed', 'completed');
  const queued = scenario === 'native-parent-terminal-queued';
  if (queued) snapshot('sendInput', 'input-new-work', 'running');
  snapshot('spawnAgent', 'spawn-replayed', queued ? 'completed' : 'running');
}

createInterface({ input: process.stdin }).on('line', line => {
  const request = JSON.parse(line);
  log(request);
  if (request.method === 'initialize') {
    if (scenario !== 'initialize-hang') reply(request.id, { userAgent: 'offline-fixture' });
  } else if (request.method === 'account/read') {
    subscriptionAccountReads++;
    if (scenario === 'subscription-hang') return;
    if (scenario === 'subscription-no-account-api') { send({ jsonrpc: '2.0', id: request.id, error: { code: -32601, message: 'method unavailable' } }); return; }
    const api = scenario === 'subscription-api' || (scenario === 'subscription-drift' && subscriptionAccountReads > 1);
    reply(request.id, { account: scenario === 'subscription-logged-out' ? null : api ? { type: 'apiKey' } : { type: 'chatgpt', email: 'never-copy-account@example.invalid', planType: 'pro' }, requiresOpenaiAuth: true });
  } else if (request.method === 'config/read') {
    const observed = scenario === 'native-mismatch' ? { ...config, 'agents.enabled': !config['agents.enabled'] } : { ...config };
    if (String(request.id).startsWith('flywheel-subscription-')) {
      observed.model_provider = scenario === 'subscription-custom-provider' ? 'pay-api' : 'openai';
      if (scenario === 'subscription-custom-endpoint') observed.chatgpt_base_url = 'https://api.example.invalid/v1';
    }
    reply(request.id, { config: observed });
  } else if (request.method === 'thread/start' || request.method === 'thread/resume') {
    const { cwd, model, sandbox, approvalPolicy, ephemeral } = request.params;
    const resumed = request.method === 'thread/resume';
    if (resumed && (scenario === 'resume-busy' || scenario === 'resume-no-rollout' || !history || history.threadId !== request.params.threadId)) {
      send({ jsonrpc: '2.0', id: request.id, error: { code: -32001,
        message: scenario === 'resume-busy' ? 'thread is busy' : 'no rollout exists for this thread' } });
      return;
    }
    persistent = resumed || ephemeral === false;
    if (!resumed) history = { threadId, total: null, turnId: null };
    if (resumed && history.total && scenario !== 'resume-no-baseline') {
      notify('thread/tokenUsage/updated', { threadId, turnId: history.turnId,
        tokenUsage: { total: history.total, last: history.total } });
    }
    saveHistory();
    const policy = sandbox === 'danger-full-access' ? { type: 'dangerFullAccess' } : sandbox === 'read-only'
      ? { type: 'readOnly', networkAccess: false }
      : { type: 'workspaceWrite', writableRoots: [cwd], networkAccess: false, excludeTmpdirEnvVar: true, excludeSlashTmp: true };
    if (scenario === 'broad-policy') policy.writableRoots = ['/'];
    const modelProvider = scenario === 'subscription-thread-provider' ? 'pay-api' : 'openai';
    reply(request.id, { thread: { id: scenario === 'wrong-resume-thread' ? 'another-thread' : threadId, ephemeral: !persistent, modelProvider }, modelProvider, cwd: scenario === 'wrong-cwd' ? '/' : cwd,
      model: scenario === 'wrong-model' ? 'unexpected-model' : model,
      approvalPolicy: scenario === 'wrong-approval' ? 'on-request' : approvalPolicy,
      sandbox: scenario === 'wrong-sandbox'
        ? (sandbox === 'danger-full-access' ? { type: 'workspaceWrite', writableRoots: [cwd] } : { type: 'dangerFullAccess' })
        : policy });
  } else if (request.method === 'thread/name/set') {
    if (scenario === 'name-rejected') {
      send({ jsonrpc: '2.0', id: request.id, error: { code: -32603, message: 'offline fixture rejects the thread name' } });
      return;
    }
    history = { ...history, name: request.params.name };
    saveHistory();
    reply(request.id, {});
  } else if (request.method === 'turn/start' || request.method === 'review/start') {
    reply(request.id, { turn: { id: turnId }, reviewThreadId: threadId });
    notify('turn/started', { threadId, turn: { id: turnId } });
    if (scenario.startsWith('activity-')) {
      observedTurn().catch(() => process.exit(92));
      return;
    }
    if (scenario === 'native-child') {
      notify('item/completed', { threadId, turnId, item: { type: 'collabAgentToolCall', id: 'spawn-1', tool: 'spawnAgent', senderThreadId: threadId,
        receiverThreadIds: ['child-thread'], model: 'child-model', agentsStates: { 'child-thread': { status: 'running' } } } });
      notify('turn/started', { threadId: 'child-thread', turn: { id: 'child-turn' } });
      for (let i = 0; i < 2; i++) notify('thread/tokenUsage/updated', { threadId: 'child-thread', turnId: 'child-turn',
        tokenUsage: { total: { totalTokens: 15, inputTokens: 10, outputTokens: 5, cachedInputTokens: 0, reasoningOutputTokens: 0 } } });
      notify('turn/completed', { threadId: 'child-thread', turn: { id: 'child-turn', status: 'completed' } });
    }
    if (['native-stale-spawn', 'native-new-turn-pending', 'native-new-turn-completed'].includes(scenario)) reorderedNativeChild();
    if (['native-parent-terminal-replay', 'native-parent-terminal-queued'].includes(scenario)) replayedParentSnapshot();
    notify('item/completed', { threadId, turnId, item: { type: 'agentMessage', phase: 'commentary', text: 'fixture working' } });
    if (scenario === 'progress-hang') {
      setInterval(() => notify('item/completed', { threadId, turnId,
        item: { type: 'agentMessage', phase: 'commentary', text: 'fixture active' } }), 20);
      return;
    }
    if (scenario === 'progress-limit') {
      // The public supervisor caps each progress message at 16 KiB. A bounded
      // 2 MiB burst crosses the worker's display budget without thousands of
      // IPC notifications competing with the actual task deadline. Let both
      // pipes drain between small batches: the worker may omit optional text
      // while its stdout is backpressured, without charging its display budget.
      void (async () => {
        const count = 128, text = 'x'.repeat(16 * 1024);
        for (let offset = 0; offset < count; offset += 4) {
          for (let item = 0; item < 4; item++) {
            if (stopping) return;
            if (!notify('item/completed', { threadId, turnId,
              item: { type: 'agentMessage', phase: 'commentary', text } })) await once(process.stdout, 'drain');
          }
          await new Promise(resolve => setTimeout(resolve, 4));
        }
        log({ event: 'progress-burst-complete', count, bytes: count * Buffer.byteLength(text) });
        // No turn completion: only the real adapter deadline ends this task.
      })().catch(() => process.exit(92));
      return;
    }
    if (['hang', 'stubborn', 'descendant'].includes(scenario)) return;
    if (scenario === 'output-limit') { process.stdout.write('x'.repeat(1100000)); return; }
    if (scenario === 'malformed') { process.stdout.write('{bad-json\n'); return; }
    if (scenario !== 'unknown-usage') {
      const delta = { totalTokens: 108, inputTokens: 100, cachedInputTokens: 25, outputTokens: 8, reasoningOutputTokens: 3 };
      const total = Object.fromEntries(Object.entries(delta).map(([key, value]) => [key, value + (history?.total?.[key] ?? 0)]));
      notify('thread/tokenUsage/updated', { threadId, turnId, tokenUsage: { total, last: delta } });
      history = { ...history, threadId, turnId, total };
      saveHistory();
      if (scenario === 'partial-usage') notify('thread/tokenUsage/updated', { threadId, turnId, tokenUsage: { total: { ...total, outputTokens: 'invalid' } } });
    }
    notify('account/rateLimits/updated', { rateLimits: { fixture: { usedPercent: 12 } } });
    notify('item/completed', { threadId, turnId, item: request.method === 'review/start' ? { type: 'exitedReviewMode', review: 'fixture native review' }
      : { type: 'agentMessage', phase: 'final_answer', text: 'fixture complete\n' } });
    notify('turn/completed', { threadId, turn: { id: turnId, status: 'completed' } });
    if (scenario === 'subscription-truncated-json-tail') process.stdout.write('{"jsonrpc":"2.0"');
    if (scenario === 'subscription-truncated-utf8-tail') process.stdout.write(Buffer.from([0xe4, 0xb8]));
  } else if (request.method === 'turn/interrupt') {
    reply(request.id, {});
    notify('turn/completed', { threadId: request.params.threadId, turn: { id: request.params.turnId, status: 'interrupted' } });
  }
});
