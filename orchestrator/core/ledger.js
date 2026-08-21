// The orchestrator's ledger zone inside a task directory (docs/<slug>) —
// single source of truth. Boundary judgement excludes these paths from the
// executor's diff; defect-injection patches touching them are refused (P0-3).

export const LEDGER_FILES = new Set(['state.json', 'retro.md', 'metrics.jsonl', 'inbox.md', 'orchestrator.pid', 'orchestrator.log']);

export const LEDGER_DIRS = ['reports/', 'approvals/'];

// Ledger files that are pure runtime artifacts: excluded from unit commits
// (a live log must never land in git history; metrics are append-only
// telemetry). Everything else in the ledger zone — state.json, retro.md,
// inbox.md, reports/, approvals/ — stays in the committed contract layer.
export const COMMIT_VOLATILE_FILES = new Set(['metrics.jsonl', 'orchestrator.log', 'orchestrator.pid']);

/** True when a task-dir-relative path (e.g. "reports/x.json") is ledger. */
export function isLedgerTaskPath(inner) {
  return LEDGER_FILES.has(inner) || LEDGER_DIRS.some((d) => inner.startsWith(d));
}
