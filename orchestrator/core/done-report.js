// Done-report parsing + format validation (DESIGN.md §5.2, §6).
// The agent reports facts only ("what was done, which commands passed/failed");
// it never declares completion. A malformed report is treated as a retryable
// error by the caller (EMPTY_RESPONSE), not silently accepted.

export const DEFAULT_FORMAT_KEYS = ['requirements-covered', 'changed-files', 'commands-run', 'failures'];

/**
 * @param {string} content raw file content
 * @param {object} [format] the packet's done-report-format; keys define the required shape
 * @returns {{ok: boolean, report: object|null, errors: string[]}}
 */
export function parseDoneReport(content, format = null) {
  const errors = [];
  let report = null;

  try {
    report = JSON.parse(content);
  } catch {
    return { ok: false, report: null, errors: ['invalid-json'] };
  }

  if (report === null || typeof report !== 'object' || Array.isArray(report)) {
    return { ok: false, report: null, errors: ['not-an-object'] };
  }

  const keys = format && typeof format === 'object' ? Object.keys(format) : DEFAULT_FORMAT_KEYS;
  for (const key of keys) {
    if (!(key in report)) {
      errors.push(`missing-key: ${key}`);
    } else if (!Array.isArray(report[key])) {
      errors.push(`not-an-array: ${key}`);
    } else if (!report[key].every((el) => typeof el === 'string')) {
      // Non-string elements (nested arrays/objects) silently broke spin
      // detection's set comparisons downstream (P0-8, user-verified).
      errors.push(`non-string-element: ${key}`);
    }
  }

  return { ok: errors.length === 0, report, errors };
}
