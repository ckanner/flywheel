import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { mkdtemp, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { BridgeError, MAX_FRAME_BYTES, normalizedError, readPolicyReceipt, record, SESSION_PROTOCOL, SessionFold, validateStart } from './protocol.js';

export function makeOverlay(request, policyPath = fileURLToPath(new URL('./policy.js', import.meta.url)), admission = {}, historyRoot) {
  // Each patch replaces an entire config row. Pin the checked rc.5/rc.1 enforcement
  // implementations, not the user's mutable permission dropdown defaults.
  const disable = ['permission', 'tool-plugin-manager', 'tool-working-directory',
    'tool-subagent', 'tool-subagent-fork', 'tool-subagent-control', 'tool-subagent-list-agents',
    'subagent-spawn-in-process', 'subagent-fork-in-process', 'tool-workflow',
    'tool-goal', 'goal-round-driver', 'schedule', 'tool-schedule',
    'session-title-llm', 'tool-web', 'compaction-basic', 'command-compact', 'hmr'];
  return [
    ...disable.map(id => ({ id, disabled: true })),
    ...(historyRoot ? [{ id: 'session-persistence-jsonl', config: { root: historyRoot } }] : []),
    { id: 'sandbox', name: '@deepseek-ai/dsh-sandbox-local', disabled: false, config: {} },
    { id: 'sandbox-policy', name: '@deepseek-ai/dsh-sandbox-policy', disabled: false,
      config: { mode: request.permissionMode, workspaceRoot: request.cwd } },
    { id: 'bash-sandbox', name: '@deepseek-ai/dsh-bash-sandbox', disabled: false,
      config: { timeoutMs: 60000, graceMs: 100 } },
    { id: 'fs-sandbox', name: '@deepseek-ai/dsh-fs-sandbox', disabled: false, config: {} },
    { id: 'approval', name: '@deepseek-ai/dsh-user-approval', disabled: false, config: { policy: 'never' } },
    { id: 'sdk-jsonrpc-server', config: { maxTokensAsSuccess: false } },
    { insert: [{ id: 'flywheel-worker-policy', name: policyPath,
      config: { permissionMode: request.permissionMode, cwd: request.cwd,
        ...(request.workspaceCwd ? { workspaceCwd: request.workspaceCwd } : {}),
        ...(!request.sessionId && request.sessionTitle ? { sessionTitle: request.sessionTitle } : {}), ...admission } }] },
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
      if (this.buffer.length) {
        this.protocolFailure ??= new BridgeError('TRANSPORT', 'DSH stdout ended in an incomplete frame');
        this.fail(this.protocolFailure);
      }
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
      // SDK shutdown includes persistent history flush and plugin disposal.
      // Keep this bounded below Go's default 5s cancellation grace, while
      // allowing a normal fsync to complete under concurrent local workloads.
      await bounded(this.exited, 2000);
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
  let resumeAccepted = false;
  let resumeUnconfirmed = false;
  let dispatch = 'not-dispatched';
  let promptId = '';
  let waited = false;
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
  const frame = value => emit({ protocol: request?.protocol ?? (input?.protocol === 2 ? 2 : 1), requestId: request?.requestId ?? input?.requestId ?? 'invalid-request', ...value });
  try {
    const budget = Number.isSafeInteger(input?.timeoutMs) && input.timeoutMs > 0 && input.timeoutMs <= 86400000
      ? input.timeoutMs : 30 * 60 * 1000;
    deadline = setTimeout(() => controller.abort(new BridgeError('TIMEOUT', 'DSH worker lifecycle deadline exceeded')), budget);
    if (signal?.aborted) cancel();
    request = await abortable(validateStart(input), controller.signal);
    if (doctor && request.protocol === 2) throw new BridgeError('INVALID_REQUEST', 'doctor does not create or resume persistent histories');
    controller.signal.throwIfAborted();
    const bin = env.FLYWHEEL_DSH_BIN || 'dsh';
    if (bin.includes('\0') || bin.startsWith('-')) throw new BridgeError('INVALID_REQUEST', 'invalid FLYWHEEL_DSH_BIN');
    const profile = env.FLYWHEEL_DSH_PROFILE || 'sdk';
    if (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(profile) || profile === 'desktop') {
      throw new BridgeError('INVALID_REQUEST', 'FLYWHEEL_DSH_PROFILE must name an SDK profile, not desktop');
    }
    temp = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-dsh-')));
    const patch = join(temp, 'worker.patch.json');
    sessionId = request.sessionId ?? `flywheel-${randomUUID()}`;
    const admission = { receiptPath: join(temp, 'policy.json'), requestId: request.requestId, sessionId, nonce: randomUUID() };
    await writeFile(patch, JSON.stringify(makeOverlay(request, undefined, admission, doctor ? join(temp, 'probe-history') : undefined)), { mode: 0o600 });
    controller.signal.throwIfAborted();
    fold = new SessionFold(sessionId, { strict: request.protocol === 2 });
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
      cwd: request.workspaceCwd ?? request.cwd, provider: request.provider, model: request.model,
      ...(request.reasoningEffort === undefined ? {} : { reasoningEffort: request.reasoningEffort }),
      ...(request.maxTokens === undefined ? {} : { maxTokens: request.maxTokens }),
    });
    if (initialized?.serverInfo?.name !== 'deepseek-harness-sdk-runtime') {
      throw new BridgeError('TRANSPORT', 'unexpected SDK runtime identity');
    }
    if (request.sessionId !== undefined) {
      // Public SDK resume restores exactly this identity, without a new input
      // and without a replacement on failure. A generic RPC error cannot tell
      // missing history from another live owner; never silently cold-start it.
      resumeUnconfirmed = true;
      const restored = await runtime.request('session/resume', { sessionId });
      if (!record(restored) || Object.keys(restored).length !== 0) {
        throw new BridgeError('TRANSPORT', 'SDK resume did not confirm restoration');
      }
      resumeAccepted = true;
      resumeUnconfirmed = false;
    }
    // This creates the fresh Session and its permission facts without an LLM turn.
    let working = await runtime.request('session/working-directory/get', { sessionId });
    if (request.workspaceCwd !== undefined && working?.cwd !== request.cwd) {
      working = await runtime.request('session/working-directory/set', { sessionId, path: request.cwd });
    }
    if (working?.cwd !== request.cwd) throw new BridgeError('INVALID_REQUEST', 'DSH effective cwd differs from the requested workspace');
    fold.policy = await abortable(readPolicyReceipt(admission.receiptPath,
      { requestId: admission.requestId, sessionId, nonce: admission.nonce }), controller.signal);
    if (request.workspaceCwd !== undefined && fold.policy.workspaceCwd !== request.workspaceCwd) {
      throw new BridgeError('INVALID_REQUEST', 'DSH session project workspace differs from the requested project');
    }
    fold.verifiedPolicy(request);
    frame({ type: 'ready', sessionId, cwd: request.cwd, ...(request.workspaceCwd ? { workspaceCwd: request.workspaceCwd } : {}), provider: request.provider, model: request.model,
      ...(request.reasoningEffort === undefined ? {} : { reasoningEffort: request.reasoningEffort }),
      permissionMode: request.permissionMode, approvalPolicy: 'never',
      policy: { protocol: fold.policy.protocol, cwd: fold.policy.cwd, permissionMode: fold.policy.permissionMode,
        approvalPolicy: fold.policy.approvalPolicy, confinedShell: fold.policy.confinedShell,
        ...(fold.policy.enforcement === undefined ? {} : { enforcement: fold.policy.enforcement }) },
      ...(request.protocol === 2 ? { persistent: true, resumed: resumeAccepted } : {}) });
    readySent = true;
    // v2 publishes only this invocation's events. Restoration/configuration
    // records are reflected in the checked ready receipt, never as new output.
    if (request.protocol === 1) for (const eventFrame of beforeReady) frame(eventFrame);
    beforeReady.length = 0;
    if (doctor) {
      outcome = { text: 'SDK route, cwd and permission handshake verified; no model prompt sent.', usage: null };
    } else {
      // The next prompt is the only invocation owned by this worker. Reset any
      // restoration facts before dispatch; historical usage must never be added.
      fold.beginCall();
      dispatch = 'unknown';
      const receipt = await runtime.request('session/prompt', {
        sessionId, contentBlocks: [{ type: 'text', text: request.prompt }],
      });
      if (typeof receipt?.messageId !== 'string' || receipt.messageId === '') {
        throw new BridgeError('TRANSPORT', 'SDK prompt returned no messageId');
      }
      promptId = receipt.messageId;
      dispatch = 'dispatched';
      const completed = await runtime.request('session/wait', { sessionId });
      if (!record(completed) || Object.keys(completed).length !== 0) {
        throw new BridgeError('TRANSPORT', 'SDK wait did not confirm a quiescent session');
      }
      waited = true;
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
        if (runtime.protocolFailure) {
          // A provider result cannot attest a damaged SDK stream. Keep its
          // diagnosis as context, while protocol and cleanup facts stay independent.
          const protocolFailure = normalizedError(runtime.protocolFailure);
          failure = failure && failure.message !== protocolFailure.message
            ? { ...protocolFailure, message: `${protocolFailure.message}; preceding failure: ${failure.message}` }
            : protocolFailure;
        }
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
  if (input?.protocol === 2) {
    // Cleanup, result certainty and output validity are independent. A complete
    // native turn with empty/model-invalid output can still be repaired in its
    // own context. Disconnects, damaged frames and ambiguous restore failures
    // never become a confirmed failure merely because the process has exited.
    const terminalKnown = waited && fold?.receipts.has(promptId) && fold?.terminal && !runtime?.protocolFailure;
    const terminal = terminalKnown ? fold.terminal.kind === 'completed' ? 'completed'
      : fold.terminal.kind === 'aborted' ? 'cancelled' : 'failed'
      : dispatch === 'not-dispatched' && !resumeUnconfirmed ? 'failed' : 'unknown';
    result.lifecycle = {
      protocol: SESSION_PROTOCOL, requestId: request?.requestId ?? input.requestId,
      sessionId: sessionId ?? '', dispatch, outcome: terminal,
      cleanupConfirmed: settled,
      resumeCandidate: Boolean(terminal === 'completed' && settled && readySent && !cancelled && !controller.signal.aborted),
      resumeAccepted, turnId: terminalKnown ? fold.turnId : '',
      usageStatus: result.usage === null ? 'unknown' : 'complete',
    };
  }
  frame(result);
  return result;
}
