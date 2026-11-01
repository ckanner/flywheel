import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { BridgeError, MAX_FRAME_BYTES, normalizedError, record, SessionFold, validateStart } from './protocol.js';

export function makeOverlay(request, policyPath = fileURLToPath(new URL('./policy.js', import.meta.url))) {
  // Each patch replaces an entire config row. Pin the known rc.5 enforcement
  // implementations, not the user's mutable permission dropdown defaults.
  const disable = ['permission', 'tool-plugin-manager', 'tool-working-directory',
    'tool-subagent', 'tool-subagent-fork', 'tool-subagent-control', 'tool-subagent-list-agents',
    'subagent-spawn-in-process', 'subagent-fork-in-process', 'tool-workflow',
    'tool-goal', 'goal-round-driver', 'session-title-llm', 'tool-web', 'compaction-basic', 'command-compact', 'hmr'];
  return [
    ...disable.map(id => ({ id, disabled: true })),
    { id: 'sandbox', name: '@deepseek-ai/dsh-sandbox-local', disabled: false, config: {} },
    { id: 'sandbox-policy', name: '@deepseek-ai/dsh-sandbox-policy', disabled: false,
      config: { mode: request.permissionMode, workspaceRoot: request.cwd } },
    { id: 'bash-sandbox', name: '@deepseek-ai/dsh-bash-sandbox', disabled: false,
      config: { timeoutMs: 60000, graceMs: 100 } },
    { id: 'fs-sandbox', name: '@deepseek-ai/dsh-fs-sandbox', disabled: false, config: {} },
    { id: 'approval', name: '@deepseek-ai/dsh-user-approval', disabled: false, config: { policy: 'never' } },
    { id: 'sdk-jsonrpc-server', config: { maxTokensAsSuccess: false } },
    { insert: [{ id: 'flywheel-worker-policy', name: policyPath,
      config: { permissionMode: request.permissionMode, cwd: request.cwd } }] },
  ];
}

async function bounded(promise, ms) {
  let timer;
  try {
    return await Promise.race([promise, new Promise(resolve => { timer = setTimeout(resolve, ms); })]);
  } finally {
    clearTimeout(timer);
  }
}

async function abortable(promise, signal) {
  signal.throwIfAborted();
  let listener;
  try {
    return await Promise.race([promise, new Promise((_, reject) => {
      listener = () => reject(signal.reason);
      signal.addEventListener('abort', listener, { once: true });
    })]);
  } finally {
    signal.removeEventListener('abort', listener);
  }
}

/** A bounded JSON-RPC transport. It does not own a new process group. */
export class Runtime {
  constructor(bin, args, options, notify) {
    this.nextId = 0;
    this.pending = new Map();
    this.buffer = Buffer.alloc(0);
    this.failed = null;
    this.protocolFailure = null;
    this.shutdownAcknowledged = false;
    this.exit = null;
    this.closed = false;
    this.child = spawn(bin, args, { ...options, detached: false, stdio: ['pipe', 'pipe', 'pipe'] });
    this.exited = new Promise(resolve => { this.resolveExit = resolve; });
    this.child.on('error', error => {
      this.fail(new BridgeError('TRANSPORT', `cannot start DSH: ${error.message}`));
      if (this.child.pid === undefined) {
        this.exit = { code: null, signal: null };
        this.resolveExit(this.exit);
      }
    });
    this.child.on('exit', (code, signal) => {
      this.exit = { code, signal };
    });
    this.child.on('close', (code, signal) => {
      this.closed = true;
      if (this.buffer.length) this.fail(new BridgeError('TRANSPORT', 'DSH stdout ended in an incomplete frame'));
      this.fail(new BridgeError('TRANSPORT', `DSH transport closed (exit=${code}, signal=${signal ?? 'none'})`));
      this.resolveExit(this.exit ?? { code, signal });
    });
    this.child.stdin.on('error', error => this.fail(new BridgeError('TRANSPORT', `DSH stdin: ${error.message}`)));
    this.child.stderr.on('data', chunk => {
      // Logs stay separate from the engine's protocol. No prompt/credential dump.
      options.stderr?.(chunk);
    });
    this.child.stdout.on('data', chunk => {
      try {
        this.buffer = Buffer.concat([this.buffer, chunk]);
        let newline;
        while ((newline = this.buffer.indexOf(10)) !== -1) {
          if (newline > MAX_FRAME_BYTES) throw new BridgeError('TRANSPORT', 'DSH frame exceeds 8 MiB');
          const line = this.buffer.subarray(0, newline).toString('utf8');
          this.buffer = this.buffer.subarray(newline + 1);
          if (line.trim() === '') continue;
          const message = JSON.parse(line);
          if (!record(message) || message.jsonrpc !== '2.0') {
            throw new BridgeError('TRANSPORT', 'DSH stdout is not JSON-RPC 2.0');
          }
          if (Object.hasOwn(message, 'id')) {
            if (message.id === 'flywheel-shutdown') {
              this.shutdownAcknowledged = !message.error && Object.hasOwn(message, 'result');
            }
            const waiting = this.pending.get(message.id);
            if (!waiting) continue; // Includes the best-effort shutdown acknowledgement.
            this.pending.delete(message.id);
            if (message.error) {
              waiting.reject(new BridgeError('TRANSPORT', `${waiting.method}: ${message.error.message ?? 'JSON-RPC error'}`));
            } else if (Object.hasOwn(message, 'result')) waiting.resolve(message.result);
            else waiting.reject(new BridgeError('TRANSPORT', 'DSH response has no result or error'));
          } else if (typeof message.method === 'string') notify(message.method, message.params);
          else throw new BridgeError('TRANSPORT', 'malformed DSH notification');
        }
        if (this.buffer.length > MAX_FRAME_BYTES) throw new BridgeError('TRANSPORT', 'DSH frame exceeds 8 MiB');
      } catch (error) {
        this.protocolFailure = error instanceof BridgeError ? error : new BridgeError('TRANSPORT', `invalid DSH frame: ${error.message}`);
        this.buffer = Buffer.alloc(0);
        this.fail(this.protocolFailure);
      }
    });
  }

  fail(error) {
    this.failed ??= error;
    for (const waiter of this.pending.values()) waiter.reject(this.failed);
    this.pending.clear();
  }

  request(method, params) {
    if (this.failed) return Promise.reject(this.failed);
    if (this.exit) return Promise.reject(new BridgeError('TRANSPORT', 'DSH already exited'));
    const id = ++this.nextId;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject, method });
      this.child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
    });
  }

  async close() {
    // The Go watchdog owns the whole inherited process group. We request the
    // SDK's child-first drain and bound the leader wait; Go performs the final
    // group sweep even on normal exit. A child must not detach/setsid.
    let forced = false;
    if (!this.exit && !this.child.stdin.destroyed) {
      this.child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: 'flywheel-shutdown', method: 'shutdown', params: {} }) + '\n');
      await bounded(this.exited, 400);
    }
    if (!this.exit) {
      forced = true;
      this.child.stdin.end();
      this.child.kill('SIGTERM');
      await bounded(this.exited, 250);
    }
    if (!this.exit) {
      this.child.kill('SIGKILL');
      await bounded(this.exited, 350);
    }
    if (!this.exit) throw new BridgeError('TRANSPORT', 'DSH leader did not exit during bounded shutdown');
    if (!this.closed && this.child.pid !== undefined) {
      await bounded(this.exited, 350);
      if (!this.closed) throw new BridgeError('TRANSPORT', 'DSH exited but its protocol streams did not reach quiescence');
    }
    return { ...this.exit, forced, graceful: this.shutdownAcknowledged, spawned: this.child.pid !== undefined };
  }
}

export async function runWorker(input, { emit, env = process.env, signal, doctor = false, stderr } = {}) {
  let request;
  let runtime;
  let temp;
  let sessionId;
  let fold;
  let outcome;
  let failure;
  let cancelled = false;
  let settled = true;
  let readySent = false;
  const beforeReady = [];
  let deadline;
  const started = Date.now();
  const controller = new AbortController();
  const cancel = () => {
    cancelled = true;
    controller.abort(new BridgeError('TRANSPORT', 'worker cancelled'));
  };
  const abort = () => runtime?.fail(controller.signal.reason);
  controller.signal.addEventListener('abort', abort);
  signal?.addEventListener('abort', cancel, { once: true });
  const frame = value => emit({ protocol: 1, requestId: request?.requestId ?? input?.requestId ?? 'invalid-request', ...value });
  try {
    const budget = Number.isSafeInteger(input?.timeoutMs) && input.timeoutMs > 0 && input.timeoutMs <= 86400000
      ? input.timeoutMs : 30 * 60 * 1000;
    deadline = setTimeout(() => controller.abort(new BridgeError('TIMEOUT', 'DSH worker lifecycle deadline exceeded')), budget);
    if (signal?.aborted) cancel();
    request = await abortable(validateStart(input), controller.signal);
    controller.signal.throwIfAborted();
    const bin = env.FLYWHEEL_DSH_BIN || 'dsh';
    if (bin.includes('\0') || bin.startsWith('-')) throw new BridgeError('INVALID_REQUEST', 'invalid FLYWHEEL_DSH_BIN');
    const profile = env.FLYWHEEL_DSH_PROFILE || 'sdk';
    if (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(profile) || profile === 'desktop') {
      throw new BridgeError('INVALID_REQUEST', 'FLYWHEEL_DSH_PROFILE must name an SDK profile, not desktop');
    }
    temp = await mkdtemp(join(tmpdir(), 'flywheel-dsh-'));
    const patch = join(temp, 'worker.patch.json');
    await writeFile(patch, JSON.stringify(makeOverlay(request)), { mode: 0o600 });
    controller.signal.throwIfAborted();
    sessionId = `flywheel-${randomUUID()}`;
    fold = new SessionFold(sessionId);
    runtime = new Runtime(bin, ['--profile', profile, '--patch', patch], {
      cwd: request.cwd,
      env: { ...env, DSH_PERMISSION_MODE: request.permissionMode,
        DSH_MAX_TOKENS_AS_SUCCESS: 'false', DSH_TELEMETRY_DISABLED: '1', FLYWHEEL_DSH_NATIVE_WORKER: '1' },
      stderr,
    }, (method, params) => {
      if (method === 'session.event' && fold.accept(params)) {
        const eventFrame = { type: 'event', sessionId, event: params.event };
        if (readySent) frame(eventFrame);
        else {
          beforeReady.push(eventFrame);
          if (beforeReady.length > 256) throw new BridgeError('TRANSPORT', 'excessive SDK events before readiness');
        }
      }
      if (method === 'subagent.started' && params?.parentSessionId === sessionId) {
        throw new BridgeError('INVALID_REQUEST', 'DSH child delegation is disabled: the Go engine owns workers');
      }
    });
    if (controller.signal.aborted) runtime.fail(controller.signal.reason);
    const initialized = await runtime.request('initialize', {
      cwd: request.cwd, provider: request.provider, model: request.model,
      ...(request.reasoningEffort === undefined ? {} : { reasoningEffort: request.reasoningEffort }),
      ...(request.maxTokens === undefined ? {} : { maxTokens: request.maxTokens }),
    });
    if (initialized?.serverInfo?.name !== 'deepseek-harness-sdk-runtime') {
      throw new BridgeError('TRANSPORT', 'unexpected SDK runtime identity');
    }
    // This creates the fresh Session and its permission facts without an LLM turn.
    const working = await runtime.request('session/working-directory/get', { sessionId });
    if (working?.cwd !== request.cwd) throw new BridgeError('INVALID_REQUEST', 'DSH effective cwd differs from the requested workspace');
    fold.verifiedPolicy(request);
    frame({ type: 'ready', sessionId, cwd: request.cwd, provider: request.provider, model: request.model,
      ...(request.reasoningEffort === undefined ? {} : { reasoningEffort: request.reasoningEffort }),
      permissionMode: request.permissionMode, approvalPolicy: 'never' });
    readySent = true;
    for (const eventFrame of beforeReady) frame(eventFrame);
    beforeReady.length = 0;
    if (doctor) {
      outcome = { text: 'SDK route, cwd and permission handshake verified; no model prompt sent.', usage: null };
    } else {
      const receipt = await runtime.request('session/prompt', {
        sessionId, contentBlocks: [{ type: 'text', text: request.prompt }],
      });
      if (typeof receipt?.messageId !== 'string' || receipt.messageId === '') {
        throw new BridgeError('TRANSPORT', 'SDK prompt returned no messageId');
      }
      await runtime.request('session/wait', { sessionId });
      outcome = fold.finish(receipt.messageId);
    }
  } catch (error) {
    failure = normalizedError(error);
  } finally {
    if (runtime) {
      try {
        const exit = await runtime.close();
        if (exit.spawned && (exit.forced || exit.code !== 0 || exit.signal || !exit.graceful)) {
          settled = false;
          failure ??= { code: 'TRANSPORT', message: 'DSH runtime did not prove graceful shutdown of its managed processes' };
        }
        if (runtime.protocolFailure) failure ??= normalizedError(runtime.protocolFailure);
      } catch (error) {
        settled = false;
        failure = normalizedError(error);
      }
    }
    clearTimeout(deadline);
    if (controller.signal.aborted) failure ??= normalizedError(controller.signal.reason);
    if (request && Date.now() - started > request.timeoutMs && !cancelled) {
      failure = { code: 'TIMEOUT', message: 'DSH worker lifecycle deadline exceeded (including cleanup)' };
    }
    if (temp) await rm(temp, { recursive: true, force: true }).catch(error => {
      failure ??= { code: 'TRANSPORT', message: `could not remove worker overlay: ${error.message}` };
    });
    signal?.removeEventListener('abort', cancel);
    controller.signal.removeEventListener('abort', abort);
  }
  const result = {
    type: 'result', status: cancelled ? 'cancelled' : failure ? 'error' : 'ok', settled,
    ...(sessionId ? { sessionId } : {}),
    text: outcome?.text ?? fold?.text ?? '',
    usage: outcome?.usage ?? null, costUsd: null,
    ...(failure ? { error: failure } : {}),
  };
  frame(result);
  return result;
}
