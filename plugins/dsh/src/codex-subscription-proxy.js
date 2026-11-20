#!/usr/bin/env node
// Official app-server JSON-RPC transport guard. The public adapter still owns
// turn semantics, persistent journals and the whole native process group.
import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { inspectCodexSubscription, inspectSubscriptionEnvironment, SubscriptionError } from './subscription-controls.js';

const prefix = 'flywheel-subscription-' + randomUUID() + '-';
const pending = new Map(), threads = new Map(), methods = new Map();
let child, closed = false, failed = false, stopping = false, sequence = 0, input = '', output = '', queue = Promise.resolve();
let queuedBytes = 0;
const decoders = { parent: new TextDecoder('utf-8', { fatal: true }), native: new TextDecoder('utf-8', { fatal: true }) };
const ended = { parent: false, native: false };
const maxFrame = 8 * 1024 * 1024;
const code = cause => cause instanceof SubscriptionError ? cause : new SubscriptionError('INVALID_REQUEST', 'Codex subscription admission could not verify the native account and effective provider');
const line = value => JSON.stringify(value) + '\n';
function send(value) {
  if (stopping || closed) return;
  if (child?.stdin.writableLength > maxFrame) { fail(new Error('native input queue limit')); return; }
  if (!child.stdin.destroyed) child.stdin.write(line(value));
}
function emit(value) {
  if (!process.stdout.destroyed && !process.stdout.write(line(value))) {
    child?.stdout.pause(); process.stdout.once('drain', () => child?.stdout.resume());
  }
}
function rejectRPC(id, cause) {
  const failure = code(cause);
  emit({ jsonrpc: '2.0', id, error: { code: -32602, message: failure.code + ': ' + failure.message } });
}
function stop() {
  if (!child || closed || stopping) return;
  stopping = true;
  process.stdin.pause();
  child.stdin.end(); child.kill('SIGTERM');
  const timer = setTimeout(() => { if (!closed) child.kill('SIGKILL'); }, 1500);
  timer.unref();
}
function fail(cause) {
  if (failed) return;
  failed = true;
  process.exitCode = 1;
  for (const item of pending.values()) { clearTimeout(item.timer); item.reject(code(cause)); }
  pending.clear();
  stop();
}
function rpc(method, params) {
  return new Promise((resolve, reject) => {
    if (failed || closed || stopping) { reject(new SubscriptionError('AUTH', 'Native subscription admission is no longer available')); return; }
    const id = prefix + ++sequence;
    const timer = setTimeout(() => { pending.delete(id); reject(new SubscriptionError('TIMEOUT', 'Native subscription account/config inspection exceeded five seconds')); }, 5000);
    pending.set(id, { resolve, reject, timer });
    send({ jsonrpc: '2.0', id, method, params });
  });
}
async function admit(cwd) {
  inspectSubscriptionEnvironment(process.env, 'codex-pro');
  const [account, config] = await Promise.all([
    rpc('account/read', { refreshToken: false }),
    rpc('config/read', { includeLayers: false, cwd }),
  ]);
  return inspectCodexSubscription(account, config);
}

async function fromParent(message) {
  if (failed || closed || stopping) return;
  if (typeof message.id === 'string' && message.id.startsWith(prefix)) throw new SubscriptionError('INVALID_REQUEST', 'RPC identity conflicts with subscription admission');
  const guarded = ['thread/start', 'thread/resume', 'thread/fork', 'turn/start', 'review/start', 'thread/compact/start'];
  if (guarded.includes(message.method)) {
    const cwd = message.params?.cwd ?? threads.get(message.params?.threadId) ?? process.cwd();
    try {
      if (presentProvider(message.params?.modelProvider)) throw new SubscriptionError('AUTH', 'Subscription-only refuses a per-call model provider override');
      await admit(cwd);
    } catch (cause) { if (!stopping) { rejectRPC(message.id, cause); fail(cause); } return; }
    if (stopping || closed) return;
    methods.set(message.id, { method: message.method, cwd });
  }
  send(message);
}
const presentProvider = value => value !== undefined && value !== null && value !== 'openai';

function fromNative(message) {
  const item = pending.get(message.id);
  if (item) {
    pending.delete(message.id); clearTimeout(item.timer);
    if (message.error || !Object.hasOwn(message, 'result')) item.reject(new SubscriptionError('INVALID_REQUEST', 'Installed Codex does not support the required read-only subscription inspection'));
    else item.resolve(message.result);
    return;
  }
  const tracked = methods.get(message.id);
  if (tracked) {
    methods.delete(message.id);
    if (!message.error && ['thread/start', 'thread/resume', 'thread/fork'].includes(tracked.method)) {
      if (message.result?.modelProvider !== 'openai' || message.result?.thread?.modelProvider !== 'openai') {
        const cause = new SubscriptionError('AUTH', 'Native thread did not confirm the built-in OpenAI subscription provider; no task turn was admitted');
        rejectRPC(message.id, cause); fail(cause); return;
      }
      threads.set(message.result.thread.id, message.result.cwd ?? tracked.cwd);
    }
  }
  if (message.method === 'account/updated' && message.params?.authMode !== 'chatgpt') {
    fail(new SubscriptionError('AUTH', 'The native account changed away from managed ChatGPT authentication')); return;
  }
  emit(message);
}

function frames(chunk, source) {
  if (source === 'parent' && stopping) return;
  let buffer = (source === 'parent' ? input : output) + decoders[source].decode(chunk, { stream: true });
  if (Buffer.byteLength(buffer) > maxFrame && !buffer.includes('\n')) throw new Error('frame too large');
  let at;
  while ((at = buffer.indexOf('\n')) >= 0) {
    const raw = buffer.slice(0, at); buffer = buffer.slice(at + 1);
    if (!raw.trim()) continue;
    if (Buffer.byteLength(raw) > maxFrame) throw new Error('frame too large');
    const message = JSON.parse(raw);
    if (!message || typeof message !== 'object' || Array.isArray(message)) throw new Error('invalid frame');
    if (source === 'parent') {
      const bytes = Buffer.byteLength(raw);
      queuedBytes += bytes;
      if (queuedBytes > maxFrame) throw new Error('parent queue too large');
      queue = queue.then(() => fromParent(message)).catch(fail).finally(() => { queuedBytes -= bytes; });
    }
    else fromNative(message);
  }
  if (source === 'parent') input = buffer; else output = buffer;
}

function endFrames(source) {
  if (ended[source]) return;
  ended[source] = true;
  const tail = (source === 'parent' ? input : output) + decoders[source].decode();
  if (tail.trim()) throw new Error('Incomplete JSON-RPC frame at EOF');
}

try {
  const argv = process.argv.slice(2);
  if (argv.at(-2) !== 'app-server' || argv.at(-1) !== '--stdio') throw new SubscriptionError('INVALID_REQUEST', 'Subscription proxy only accepts the official app-server stdio command');
  inspectSubscriptionEnvironment(process.env, 'codex-pro');
  const command = process.env.FLYWHEEL_CODEX_SUBSCRIPTION_COMMAND;
  if (!command || command.includes('\0')) throw new SubscriptionError('INVALID_REQUEST', 'Native Codex command was not bound');
  const env = { ...process.env }; delete env.FLYWHEEL_CODEX_SUBSCRIPTION_COMMAND;
  child = spawn(command, argv, { cwd: process.cwd(), env, stdio: ['pipe', 'pipe', 'pipe'], detached: false });
  child.stdin.on('error', cause => {
    // The owner can close stdin and signal the whole group concurrently.
    // A write already in flight may then settle after our idempotent stop.
    if (stopping && ['EPIPE', 'ERR_STREAM_WRITE_AFTER_END', 'ERR_STREAM_DESTROYED'].includes(cause.code)) return;
    fail(cause);
  });
  child.stdout.on('data', chunk => { try { frames(chunk, 'native'); } catch (cause) { fail(cause); } });
  child.stdout.on('end', () => { try { endFrames('native'); } catch (cause) { fail(cause); } });
  // Native stderr may contain account URLs or diagnostic credentials. It is
  // neither admission evidence nor provider activity, and is never copied.
  child.stderr.on('data', () => {});
  child.once('error', fail);
  child.once('close', (nativeCode, nativeSignal) => {
    try { endFrames('native'); endFrames('parent'); } catch (cause) { fail(cause); }
    closed = true;
    for (const item of pending.values()) { clearTimeout(item.timer); item.reject(new SubscriptionError('AUTH', 'Codex exited before native subscription inspection completed')); }
    pending.clear();
    process.stdin.pause();
    if (failed || nativeSignal === null) {
      process.exitCode = failed ? 1 : nativeCode ?? 1;
      return;
    }
    // Node ignores SIGPIPE and repurposes SIGUSR1 for its inspector. Those and
    // other unexpected signals must remain failures, never a synthetic exit 0.
    process.exitCode = 1;
    if (!['SIGTERM', 'SIGKILL', 'SIGINT', 'SIGHUP'].includes(nativeSignal)) return;
    // Preserve ordinary termination signals for the owning public adapter to
    // interpret alongside completion/cancellation. Drain forwarded frames first.
    process.stdout.write('', error => {
      if (error || failed) { process.exitCode = 1; return; }
      process.removeListener(nativeSignal, stop);
      process.kill(process.pid, nativeSignal);
    });
  });
  process.stdin.on('data', chunk => { try { frames(chunk, 'parent'); } catch (cause) { fail(cause); } });
  process.stdin.on('end', () => { try { endFrames('parent'); } catch (cause) { fail(cause); } stop(); });
  process.stdin.on('error', fail); process.stdout.on('error', fail);
  for (const signal of ['SIGTERM', 'SIGINT', 'SIGHUP']) process.on(signal, stop);
} catch {
  // This line deliberately contains no account identifiers or raw native text.
  process.stderr.write('Subscription-only Codex startup could not verify its native route; no prompt was forwarded.\n');
  process.exitCode = 1;
}
