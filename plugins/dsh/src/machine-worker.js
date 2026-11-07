// Shared parent lifeline and bounded stream transport; adapters own backend semantics.
export async function runMachineWorker({ ADAPTER, LIMITS, WorkerError, loadAdapter, machineRequest, doctorResult,
  label, commandName, attestAdmission = false, runtimeOptions = () => ({}) }) {

  const controller = new AbortController();
  let cancelledBy, inputFailure, outputFailure, requestId = 'invalid-request', protocol = 1, finished = false, adapterInvoked = false;
  const cancel = signal => { cancelledBy ??= signal; controller.abort(); };
  const handlers = new Map(['SIGTERM', 'SIGINT', 'SIGHUP'].map(signal => [signal, () => cancel(signal)]));
  for (const [signal, handler] of handlers) process.on(signal, handler);
  process.stdout.on('error', () => {
    outputFailure ??= new WorkerError('OUTPUT_ERROR', 'The result consumer closed or failed');
    process.exitCode = 1;
    controller.abort();
  });

  function write(frame) {
    if (outputFailure) return;
    const line = JSON.stringify(frame) + '\n';
    if (Buffer.byteLength(line) > LIMITS.receiptFrameBytes) throw new WorkerError('OUTPUT_LIMIT', label + ' result exceeded the complete receipt frame limit');
    process.stdout.write(line);
  }

  /** Do not close stdin after the first line: parent death must cancel the run. */
  function input() {
    let bytes = 0, buffer = Buffer.alloc(0), admitted = false, resolveInput, rejectInput;
    const promise = new Promise((resolve, reject) => { resolveInput = resolve; rejectInput = reject; });
    const fail = (code, message) => {
      if (finished) return;
      inputFailure ??= new WorkerError(code, message);
      clearTimeout(timer);
      rejectInput(inputFailure);
      controller.abort();
    };
    const timer = setTimeout(() => fail('INPUT_TIMEOUT', 'No complete start request arrived within 10 seconds'), LIMITS.admissionMs);
    const onData = chunk => {
      bytes += chunk.length;
      if (bytes > LIMITS.requestBytes) { fail('INPUT_LIMIT', 'The request exceeded the 1 MiB input limit'); return; }
      if (admitted) {
        if (chunk.toString('utf8').trim() !== '') fail('INVALID_REQUEST', 'Only one start request is allowed');
        return;
      }
      buffer = Buffer.concat([buffer, chunk]);
      const newline = buffer.indexOf(10);
      if (newline < 0) return;
      admitted = true;
      clearTimeout(timer);
      if (buffer.subarray(newline + 1).toString('utf8').trim() !== '') { fail('INVALID_REQUEST', 'Only one start request is allowed'); return; }
      try { resolveInput(JSON.parse(buffer.subarray(0, newline).toString('utf8'))); }
      catch { fail('INVALID_REQUEST', 'The first line must contain one JSON request'); }
      buffer = Buffer.alloc(0);
    };
    const onEnd = () => fail('PARENT_DISCONNECTED', 'The parent stdin lifeline closed before completion');
    const onError = () => fail('INPUT_ERROR', 'The parent stdin lifeline failed');
    const onAbort = () => { clearTimeout(timer); rejectInput(new WorkerError('CANCELLED', label + ' worker was cancelled before admission')); };
    process.stdin.on('data', onData);
    process.stdin.once('end', onEnd);
    process.stdin.once('error', onError);
    controller.signal.addEventListener('abort', onAbort, { once: true });
    if (controller.signal.aborted) onAbort();
    return { promise, stop() {
      clearTimeout(timer);
      process.stdin.off('data', onData); process.stdin.off('end', onEnd); process.stdin.off('error', onError);
      controller.signal.removeEventListener('abort', onAbort);
      process.stdin.pause();
      process.stdin.destroy();
    } };
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
      protocol = value?.protocol === 2 ? 2 : 1;
      if (typeof value?.requestId === 'string' && value.requestId.trim() !== '' && value.requestId.length <= 256) requestId = value.requestId;
      const api = await loadAdapter();
      if (controller.signal.aborted) throw inputFailure || new WorkerError('CANCELLED', label + ' worker was cancelled before the run');
      const request = machineRequest(value, api, process.env);
      requestId = request.requestId;
      let progressBytes = 0;
      const options = { signal: controller.signal, env: process.env, ...runtimeOptions(process.env), onProgress(text) {
        if (controller.signal.aborted) return;
        const frame = { protocol, type: 'progress', requestId, text };
        progressBytes += Buffer.byteLength(JSON.stringify(frame) + '\n');
        if (progressBytes > LIMITS.progressBytes) {
          outputFailure ??= new WorkerError('OUTPUT_LIMIT', label + ' progress exceeded the 1 MiB stream limit');
          controller.abort();
        } else write(frame);
      } };
      adapterInvoked = true;
      const receipt = await api.runMachine(request, options);
      finished = true;
      lifeline.stop();
      const error = inputFailure || outputFailure;
      // An output-limit error still allows a bounded terminal receipt. A broken
      // stdout cannot, but the run has already settled and cleanup was awaited.
      if (outputFailure?.code === 'OUTPUT_LIMIT') outputFailure = undefined;
      write({ protocol, type: 'result', requestId, adapter: ADAPTER, receipt,
        error: error ? { code: error.code, message: error.message } : null });
      process.exitCode = receipt.ok && !error && !controller.signal.aborted ? 0 : 1;
    }
  } catch (error) {
    finished = true;
    lifeline?.stop();
    const known = error instanceof WorkerError ? error : new WorkerError('WORKER_ERROR', label + ' worker failed before a complete receipt was available');
    write({ protocol, type: 'result', requestId, adapter: ADAPTER, receipt: null,
      ...(attestAdmission && !adapterInvoked ? { admission: { dispatched: false, cleanupConfirmed: true } } : {}),
      error: { code: known.code, message: known.message } });
    process.exitCode = 1;
  } finally {
    finished = true;
    lifeline?.stop();
    if (cancelledBy) process.exitCode = { SIGINT: 130, SIGTERM: 143, SIGHUP: 129 }[cancelledBy];
    for (const [signal, handler] of handlers) process.off(signal, handler);
  }
}
