import { isAbsolute } from 'node:path';
import { realpath } from 'node:fs/promises';
import { OwnedKimiRuntime, checkAbort, error } from './kimi-native-runtime.js';
import { validateObservationRequest } from './machine-worker.js';

const identifier = value => typeof value === 'string' && value.trim() && value.length <= 512 && !/[\0\r\n]/.test(value);
const workspacePath = value => typeof value === 'string' && value.trim() !== '' && value.length <= 4096 && !value.includes('\0') && isAbsolute(value);

// Kimi print currently has no title option and bypasses the TUI's title service.
// Prepare only a new named history through its public API, then fully close the
// owned server before print resumes it. No prompt or title-model call is sent.
export async function prepareKimiPrintSession(input, { env = process.env, signal } = {}) {
  let runtime, sessionId = '', title = '', failure, cleanupConfirmed = true;
  const controller = new AbortController();
  let abortCause;
  const abort = (cause, reason) => {
    if (controller.signal.aborted) return;
    abortCause = cause;
    controller.abort(reason);
  };
  const cancel = () => abort('cancelled', signal?.reason);
  if (signal?.aborted) cancel(); else signal?.addEventListener('abort', cancel, { once: true });
  let timer;
  try {
    if (!input || input.type !== 'prepare-print' || input.protocol !== 1 ||
        Object.keys(input).some(key => !['type', 'protocol', 'requestId', 'cwd', 'model', 'reasoningEffort', 'sessionTitle', 'timeoutMs'].includes(key)) ||
        !identifier(input.requestId) || !workspacePath(input.cwd) ||
        !identifier(input.model) || !identifier(input.reasoningEffort) || typeof input.sessionTitle !== 'string' ||
        !Number.isSafeInteger(input.timeoutMs) || input.timeoutMs <= 0 || input.timeoutMs > 2147483647) {
      throw error('INVALID_REQUEST', 'Expected a bounded new print-session preparation request without any prompt');
    }
    validateObservationRequest(input, class extends Error { constructor(code, message) { super(message); this.code = code; } });
    title = input.sessionTitle;
    timer = setTimeout(() => abort('timeout', error('TIMEOUT', 'Kimi session preparation exceeded the invocation deadline')), input.timeoutMs);
    checkAbort(controller.signal);
    const cwd = await realpath(input.cwd);
    runtime = new OwnedKimiRuntime(env, 'agent');
    cleanupConfirmed = false;
    await runtime.start(controller.signal);
    const openapi = await runtime.request('/openapi.json', undefined, { raw: true, signal: controller.signal });
    const creation = openapi.paths?.['/api/v1/sessions']?.post?.requestBody?.content?.['application/json']?.schema;
    const resolve = (schema, seen = new Set()) => {
      if (!schema || typeof schema !== 'object') return {};
      if (schema.$ref) {
        if (!schema.$ref.startsWith('#/') || seen.has(schema.$ref)) return {};
        seen.add(schema.$ref);
        return resolve(schema.$ref.slice(2).split('/').reduce((node, key) => node?.[key], openapi), seen);
      }
      return { ...schema.properties, ...Object.assign({}, ...(schema.allOf || []).map(part => resolve(part, new Set(seen)))) };
    };
    const fields = resolve(creation), config = resolve(fields.agent_config);
    if (!String(openapi.openapi).startsWith('3.') || !fields.title || !fields.metadata ||
        ['model', 'permission_mode', 'plan_mode'].some(key => !config[key])) {
      throw error('UNSUPPORTED_SCHEMA', 'The installed Kimi API cannot prepare a named print session with its original model and auto permission mode');
    }
    const session = await runtime.request('/api/v1/sessions', {
      title, metadata: { cwd, flywheel_print_invocation: input.requestId },
      // Print always uses auto permissions and skips default_plan_mode on a
      // fresh history. Preserve those semantics when the CLI sees --session.
      // Omit thinking too: KIMI_MODEL_THINKING_EFFORT preserves print's
      // conditional effort override without overriding a disabled-thinking profile.
      // Omitted environment/profile/native controls retain Kimi's own defaults.
      agent_config: { model: input.model, permission_mode: 'auto', plan_mode: false },
    }, { signal: controller.signal });
    if (!identifier(session.id)) throw error('RUNTIME_PROTOCOL', 'Kimi did not return a valid new print-session identity');
    sessionId = session.id;
    if (session.title !== title || session.metadata?.cwd !== cwd || session.metadata?.flywheel_print_invocation !== input.requestId ||
        session.busy !== false || session.main_turn_active !== false || session.pending_interaction !== 'none') {
      throw error('RUNTIME_PROTOCOL', 'Kimi did not confirm the requested title, workspace, ownership or idle new history');
    }
  } catch (cause) {
    failure = { code: typeof cause?.code === 'string' ? cause.code : 'PREPARE_FAILED',
      message: 'Kimi could not prepare the named print session; no task prompt was sent' };
  } finally {
    clearTimeout(timer);
    if (runtime) {
      try { const stopped = await runtime.stop(); cleanupConfirmed = stopped.exited && stopped.processesConfirmed; }
      catch { cleanupConfirmed = false; }
    }
    signal?.removeEventListener('abort', cancel);
  }
  // Cleanup certainty and the initiating stop reason are independent. A
  // deadline cannot become a user cancellation when cleanup also fails.
  if (abortCause === 'timeout') failure = { code: 'TIMEOUT', message: 'Kimi session preparation exceeded the invocation deadline; no task prompt was sent' };
  else if (!cleanupConfirmed) failure = { code: 'CLEANUP_UNCONFIRMED', message: 'The owned Kimi naming runtime did not confirm physical cleanup' };
  const status = abortCause === 'cancelled' ? 'cancelled' : failure ? 'error' : 'ok';
  return { protocol: 1, type: 'prepared-print', requestId: input?.requestId || 'invalid-request', status,
    sessionId, sessionTitle: title, cleanupConfirmed, inferenceDispatched: false, error: failure || null };
}
