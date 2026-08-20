// Exit contract (DESIGN.md §6): unit success is judged by THREE independent
// checks — clean process termination, stderr free of errors, and deliverables
// present. timedOut / signal / exitCode are recorded orthogonally; a timeout
// trapped into "exit 0" must never read as green. Empty output (exit 0 but no
// deliverable) maps to the retryable EMPTY_RESPONSE, not silent success.

import { classifyError } from './errors.js';

/**
 * @param {object} args
 * @param {{exitCode:number|null, signal:string|null, timedOut:boolean, stderr:string, spawnError?:Error|null}} args.result
 * @param {Array<{path:string, exists:boolean, valid:boolean, detail?:string}>} args.deliverables
 */
export function evaluateExitContract({ result, deliverables = [] }) {
  const reasons = [];

  // 1. Process termination
  const processOk = !result.timedOut && !result.signal && !result.spawnError && result.exitCode === 0;
  if (!processOk) {
    reasons.push(`process: exitCode=${result.exitCode} signal=${result.signal} timedOut=${result.timedOut}`);
  }

  // 2. stderr hygiene: no recognizable error patterns even when exit code is 0
  const stderrError = result.stderr ? classifyError({ exitCode: 0, stderr: result.stderr }) : null;
  const stderrOk = !stderrError;
  if (!stderrOk) reasons.push(`stderr: looks like ${stderrError}`);

  // 3. Deliverables exist and are valid
  const missing = deliverables.filter((d) => !d.exists);
  const invalid = deliverables.filter((d) => d.exists && !d.valid);
  const deliverablesOk = missing.length === 0 && invalid.length === 0;
  for (const d of missing) reasons.push(`deliverable missing: ${d.path}`);
  for (const d of invalid) reasons.push(`deliverable invalid: ${d.path} (${d.detail || 'schema'})`);

  let errorCode = null;
  if (!processOk) {
    errorCode = classifyError(result);
  } else if (!deliverablesOk) {
    // exit 0 but no usable deliverable = EMPTY_RESPONSE (retryable), not success
    errorCode = 'EMPTY_RESPONSE';
  } else if (!stderrOk) {
    errorCode = stderrError;
  }

  return {
    ok: processOk && stderrOk && deliverablesOk,
    processOk,
    stderrOk,
    deliverablesOk,
    errorCode,
    reasons,
  };
}
