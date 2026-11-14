import { isAbsolute } from 'node:path';

const LIFECYCLE_EVENTS = new Set(['thread-ready', 'dispatch-intent', 'dispatch-possible', 'turn-accepted', 'terminal', 'cleanup-confirmed']);
const ACK_BYTES = 4096;
const MAX_LIFECYCLE_EVENTS = 64;

function identifier(value) {
  return typeof value === 'string' && value.trim() !== '' && value.length <= 256 && !value.includes('\0');
}

/** Host-owned attribution, not provider arguments. External callers cannot supply leases. */
export function validateNativeAdmission(value, requestId, WorkerError) {
  if (value === undefined) return;
  const fields = ['version', 'runId', 'invocationId', 'role', 'scope', 'candidateId', 'channel', 'taskDir'];
  const text = value => typeof value === 'string' && value.trim() !== '' && value.length <= 4096 && !value.includes('\0');
  if (!value || typeof value !== 'object' || Array.isArray(value) || value.version !== 1 ||
      Object.keys(value).some(key => !fields.includes(key)) || value.invocationId !== requestId ||
      !['invocationId', 'role', 'scope', 'channel', 'taskDir'].every(key => text(value[key])) ||
      typeof value.runId !== 'string' || value.runId.length > 4096 || value.runId.includes('\0') ||
      (value.candidateId !== undefined && !text(value.candidateId)) || !isAbsolute(value.taskDir)) {
    throw new WorkerError('INVALID_REQUEST', 'The managed native identity must match this request and contain only host attribution fields');
  }
}

/** Recovery carries only the old non-secret binding, never a prompt. */
export function recoveryRequest(value, WorkerError) {
  const keys = new Set(['type', 'protocol', 'requestId', 'runId', 'recoveryDirectory', 'original']);
  const originalKeys = new Set(['cwd', 'model', 'reasoningEffort', 'permissionMode', 'threadMode', 'threadId', 'native', 'nativeOperation', 'managed']);
  const o = value?.original;
  if (!value || typeof value !== 'object' || Array.isArray(value) || value.type !== 'recover' || value.protocol !== 3 ||
      Object.keys(value).some(key => !keys.has(key)) || !identifier(value.requestId) || !identifier(value.runId) ||
      typeof value.recoveryDirectory !== 'string' || !isAbsolute(value.recoveryDirectory) || value.recoveryDirectory.includes('\0') || value.recoveryDirectory.length > 4096 ||
      !o || typeof o !== 'object' || Array.isArray(o) || Object.keys(o).some(key => !originalKeys.has(key)) ||
      typeof o.cwd !== 'string' || !isAbsolute(o.cwd) || o.cwd.includes('\0') || !identifier(o.model) ||
      typeof o.reasoningEffort !== 'string' || !['read-only', 'workspace-write', 'danger-full-access'].includes(o.permissionMode) ||
      !['ephemeral', 'persistent'].includes(o.threadMode) ||
      (o.threadId !== undefined && (o.threadMode !== 'persistent' || !identifier(o.threadId)))) {
    throw new WorkerError('INVALID_REQUEST', 'Expected a protocol 3 read-only recovery request with the original non-secret binding');
  }
  validateNativeAdmission(o.managed, value.requestId, WorkerError);
  // No cwd resolution, login, runMachine, thread/resume or model request here.
  return { protocol: 1, requestId: value.requestId, runId: value.runId, recoveryDirectory: value.recoveryDirectory };
}

// Parent lifeline and bounded transport; adapters own backend semantics.
export async function runMachineWorker({ ADAPTER, LIMITS, WorkerError, loadAdapter, machineRequest, doctorResult,
  label, commandName, attestAdmission = false, runtimeOptions = () => ({}),
  invokeAdapter = (api, request, options) => api.runMachine(request, options),
  recoverAdapter = (api, request) => api.recoverMachine(request) }) {
  const controller = new AbortController();
  let cancelledBy, inputFailure, outputFailure, requestId = 'invalid-request', protocol = 1, finished = false, adapterInvoked = false;
  let recovery = false, pendingAck, lifecycleSequence = 0, lifecycleRunId, lifecycleBytes = 0;
  const cancel = signal => { cancelledBy ??= signal; controller.abort(); };
  const handlers = new Map(['SIGTERM', 'SIGINT', 'SIGHUP'].map(signal => [signal, () => cancel(signal)]));
  for (const [signal, handler] of handlers) process.on(signal, handler);
  process.stdout.on('error', () => {
    outputFailure ??= new WorkerError('OUTPUT_ERROR', 'The result consumer closed or failed');
    process.exitCode = 1;
    controller.abort();
  });

  function write(frame) {
    if (outputFailure) return false;
    const line = JSON.stringify(frame) + '\n';
    if (Buffer.byteLength(line) > LIMITS.receiptFrameBytes) throw new WorkerError('OUTPUT_LIMIT', label + ' result exceeded the complete receipt frame limit');
    process.stdout.write(line);
    return true;
  }

  function settleAck(error) {
    if (!pendingAck) return;
    const pending = pendingAck;
    pendingAck = undefined;
    clearTimeout(pending.timer);
    if (error) pending.reject(error); else pending.resolve();
  }

  function acceptAck(value) {
    const keys = new Set(['type', 'protocol', 'requestId', 'runId', 'sequence']);
    if (protocol !== 3 || !pendingAck || !value || typeof value !== 'object' || Array.isArray(value) ||
        Object.keys(value).some(key => !keys.has(key)) || value.type !== 'ack' || value.protocol !== 3 ||
        value.requestId !== requestId || value.runId !== lifecycleRunId || value.sequence !== lifecycleSequence) {
      throw new WorkerError('INVALID_ACK', 'A lifecycle acknowledgment must match the pending request, run and sequence');
    }
    settleAck();
  }

  /** Keep stdin open: protocol 3 uses durable ACKs, all versions use EOF as a lifeline. */
  function input() {
    let buffer = Buffer.alloc(0), admitted = false, inputProtocol = 1, ackBytes = 0, resolveInput, rejectInput;
    const promise = new Promise((resolve, reject) => { resolveInput = resolve; rejectInput = reject; });
    const fail = (code, message) => {
      if (finished) return;
      inputFailure ??= new WorkerError(code, message);
      clearTimeout(timer);
      rejectInput(inputFailure);
      settleAck(inputFailure);
      controller.abort();
    };
    const timer = setTimeout(() => fail('INPUT_TIMEOUT', 'No complete start request arrived within 10 seconds'), LIMITS.admissionMs);
    const onData = chunk => {
      if (finished || inputFailure) return;
      buffer = Buffer.concat([buffer, chunk]);
      while (buffer.length) {
        if (admitted && buffer.toString('utf8').trim() !== '' && (inputProtocol !== 3 || !pendingAck)) {
          fail(inputProtocol === 3 ? 'INVALID_ACK' : 'INVALID_REQUEST', 'Only the pending lifecycle checkpoint may be acknowledged');
          return;
        }
        const newline = buffer.indexOf(10);
        const limit = admitted ? ACK_BYTES : LIMITS.requestBytes;
        if ((newline < 0 ? buffer.length : newline + 1) > limit) { fail('INPUT_LIMIT', 'A request or acknowledgment exceeded its frame limit'); return; }
        if (newline < 0) return;
        const bytes = buffer.subarray(0, newline);
        buffer = buffer.subarray(newline + 1);
        try {
          const line = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
          if (admitted) {
            ackBytes += bytes.length + 1;
            if (ackBytes > MAX_LIFECYCLE_EVENTS * ACK_BYTES) throw new WorkerError('INPUT_LIMIT', 'Lifecycle acknowledgments exceeded their total limit');
            if (line.trim() === '') continue;
          }
          const value = JSON.parse(line);
          if (!admitted) {
            admitted = true;
            inputProtocol = value?.protocol;
            clearTimeout(timer);
            resolveInput(value);
          } else {
            acceptAck(value);
          }
        } catch (error) {
          fail(error instanceof WorkerError ? error.code : 'INVALID_REQUEST',
            error instanceof WorkerError ? error.message : 'Every frame must be one valid UTF-8 JSON document');
          return;
        }
      }
    };
    const onEnd = () => fail('PARENT_DISCONNECTED', 'The parent stdin lifeline closed before completion');
    const onError = () => fail('INPUT_ERROR', 'The parent stdin lifeline failed');
    const onAbort = () => {
      clearTimeout(timer);
      const error = inputFailure || outputFailure || new WorkerError('CANCELLED', label + ' worker was cancelled');
      rejectInput(error);
      settleAck(error);
    };
    process.stdin.on('data', onData);
    process.stdin.once('end', onEnd);
    process.stdin.once('error', onError);
    controller.signal.addEventListener('abort', onAbort, { once: true });
    if (controller.signal.aborted) onAbort();
    return { promise, stop() {
      clearTimeout(timer);
      settleAck(new WorkerError('CANCELLED', 'The worker has settled'));
      process.stdin.off('data', onData); process.stdin.off('end', onEnd); process.stdin.off('error', onError);
      controller.signal.removeEventListener('abort', onAbort);
      process.stdin.pause();
      process.stdin.destroy();
    } };
  }

  async function onLifecycle(event) {
    if (controller.signal.aborted) throw inputFailure || outputFailure || new WorkerError('CANCELLED', 'Lifecycle consumer was cancelled');
    if (pendingAck || !event || typeof event !== 'object' || event.type !== 'lifecycle' || event.lifecycleVersion !== 1 ||
        event.requestId !== requestId || !identifier(event.runId) || (lifecycleRunId && lifecycleRunId !== event.runId) ||
        !Number.isSafeInteger(event.sequence) || event.sequence !== lifecycleSequence + 1 || event.sequence > MAX_LIFECYCLE_EVENTS ||
        !LIFECYCLE_EVENTS.has(event.event)) {
      throw new WorkerError('INVALID_LIFECYCLE', 'The adapter emitted an uncorrelated or out-of-order lifecycle event');
    }
    lifecycleRunId = event.runId;
    lifecycleSequence = event.sequence;
    const frame = { protocol: 3, type: 'lifecycle', requestId, adapter: ADAPTER, lifecycle: event };
    lifecycleBytes += Buffer.byteLength(JSON.stringify(frame) + '\n');
    if (lifecycleBytes > LIMITS.progressBytes) throw new WorkerError('OUTPUT_LIMIT', 'Lifecycle evidence exceeded its stream limit');
    // Install the waiter before writing so a fast durable consumer cannot race it.
    await new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        inputFailure ??= new WorkerError('ACK_TIMEOUT', 'The parent did not durably acknowledge the lifecycle checkpoint in time');
        settleAck(inputFailure);
        controller.abort();
      }, LIMITS.lifecycleAckMs ?? 10000);
      pendingAck = { resolve, reject, timer };
      try {
        if (!write(frame)) settleAck(outputFailure);
      } catch (error) { settleAck(error); controller.abort(); }
    });
  }

  let lifeline;
  try {
    const args = process.argv.slice(2);
    if (args.length > 1 || (args.length === 1 && args[0] !== '--doctor')) throw new WorkerError('INVALID_REQUEST', 'Use ' + commandName + ' [--doctor]');
    if (args[0] === '--doctor') {
      await loadAdapter();
      write(doctorResult());
      process.exitCode = 0;
    } else {
      lifeline = input();
      const value = await lifeline.promise;
      protocol = [2, 3].includes(value?.protocol) ? value.protocol : 1;
      recovery = protocol === 3 && value?.type === 'recover';
      if (identifier(value?.requestId)) requestId = value.requestId;
      const api = await loadAdapter();
      if (controller.signal.aborted) throw inputFailure || new WorkerError('CANCELLED', label + ' worker was cancelled before the run');
      if (protocol === 3 && (api.MACHINE_LIFECYCLE_VERSION !== 1 || typeof api.recoverMachine !== 'function' ||
          api.MACHINE_LIFECYCLE_CAPABILITIES?.durableJournal !== true || api.MACHINE_LIFECYCLE_CAPABILITIES?.readOnlyRecovery !== true)) {
        throw new WorkerError('ADAPTER_MISMATCH', 'The selected adapter does not expose lifecycle 1 and read-only durable recovery');
      }
      if (recovery) {
        const query = recoveryRequest(value, WorkerError);
        const evidence = await recoverAdapter(api, query, value.original);
        finished = true;
        lifeline.stop();
        const error = inputFailure || outputFailure;
        write({ protocol: 3, type: 'recovery', requestId, adapter: ADAPTER, recovery: evidence,
          error: error ? { code: error.code, message: error.message } : null });
        process.exitCode = error || controller.signal.aborted ? 1 : 0;
      } else {
        const request = machineRequest(value, api, process.env);
        requestId = request.requestId;
        let progressBytes = 0;
        const options = { signal: controller.signal, env: process.env, ...runtimeOptions(process.env),
          ...(protocol === 3 ? { recoveryDirectory: value.recoveryDirectory, onLifecycle } : {}),
          onProgress(text) {
            if (controller.signal.aborted) return;
            const frame = { protocol, type: 'progress', requestId, text };
            progressBytes += Buffer.byteLength(JSON.stringify(frame) + '\n');
            if (progressBytes > LIMITS.progressBytes) {
              outputFailure ??= new WorkerError('OUTPUT_LIMIT', label + ' progress exceeded the 1 MiB stream limit');
              controller.abort();
            } else write(frame);
          },
          onNativeEvent(event) {
            if (controller.signal.aborted) return;
            if (!event || typeof event !== 'object' || Array.isArray(event) || event.version !== 1 ||
                !Number.isSafeInteger(event.sequence) || event.sequence < 1 || !identifier(event.id) ||
                !identifier(event.kind) || typeof event.state !== 'string' ||
                (event.parentId !== null && !identifier(event.parentId))) {
              outputFailure ??= new WorkerError('INVALID_NATIVE_EVENT', 'The adapter emitted invalid native child metadata');
              controller.abort(); return;
            }
            const frame = { protocol, type: 'native-event', requestId, adapter: ADAPTER, native: event,
              ...(value.managed === undefined ? {} : { managed: value.managed }) };
            progressBytes += Buffer.byteLength(JSON.stringify(frame) + '\n');
            if (progressBytes > LIMITS.progressBytes) {
              outputFailure ??= new WorkerError('OUTPUT_LIMIT', label + ' native evidence exceeded the 1 MiB stream limit');
              controller.abort();
            } else write(frame);
          } };
        adapterInvoked = true;
        const receipt = await invokeAdapter(api, request, options);
        finished = true;
        lifeline.stop();
        const error = inputFailure || outputFailure;
        if (['OUTPUT_LIMIT', 'INVALID_NATIVE_EVENT'].includes(outputFailure?.code)) outputFailure = undefined;
        write({ protocol, type: 'result', requestId, adapter: ADAPTER, receipt,
          error: error ? { code: error.code, message: error.message } : null });
        process.exitCode = receipt.ok && !error && !controller.signal.aborted ? 0 : 1;
      }
    }
  } catch (error) {
    finished = true;
    lifeline?.stop();
    const known = error instanceof WorkerError ? error : new WorkerError('WORKER_ERROR', label + ' worker failed before a complete receipt was available');
    write({ protocol, type: recovery ? 'recovery' : 'result', requestId, adapter: ADAPTER,
      ...(recovery ? { recovery: null } : { receipt: null }),
      ...(attestAdmission && !adapterInvoked && !recovery ? { admission: { dispatched: false, cleanupConfirmed: true } } : {}),
      error: { code: known.code, message: known.message } });
    process.exitCode = 1;
  } finally {
    finished = true;
    lifeline?.stop();
    if (cancelledBy) process.exitCode = { SIGINT: 130, SIGTERM: 143, SIGHUP: 129 }[cancelledBy];
    for (const [signal, handler] of handlers) process.off(signal, handler);
  }
}
