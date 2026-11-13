---
packet-id: M01-T01
contract-version: 1
goal: Implement the auth token parser so that `parseToken` returns claims for valid JWTs and null otherwise.
requirements: [R-01, R-02]
route: default
# 可选：研究真实依赖后，按当前 schema 声明独立执行范围；未知时省略、串行执行。
# schedule:
#   after: []
#   writes: [src/auth/, tests/auth/]
needs-live-credentials: false
context-files: [src/auth/*.ts, docs/spec.md#M-01]
cache-scope: [docs/spec.md]
boundaries:
  must-not-touch: [src/payments/, acceptance/auth/, e2e/auth-flow.ts]
  max-files: 6
# Optional approved short feedback; it never replaces any acceptance entry.
# feedback-checks:
#   - {cmd: "pnpm test auth -- --runInBand -t malformed", expect: "exit 0", purpose: "Distinguish malformed-token handling from other authentication failures"}
acceptance:
  L1:
    - {cmd: "pnpm test auth", expect: "exit 0"}
    - {cmd: "pnpm lint", expect: "exit 0"}
    - {cmd: "pnpm coverage --changed --min 100", expect: "exit 0"}
  L2:
    - {cmd: "pnpm test:integration auth", expect: "exit 0"}
  L3:
    - {script: "e2e/auth-flow.ts", expect: "exit 0"}
review-rubric: |
  Pass line: score >= 0.8 and no blocking issue.
  - Token parser rejects malformed input without throwing.
  - Tests contain real assertions on claims, not smoke calls.
  - No scope creep beyond R-01/R-02.
escalation: {max-retries: 3, on-fail: return-to-planner}
done-report-format:
  requirements-covered: []
  changed-files: []
  commands-run: []
  failures: []
---

# Work instructions

You are the executor for exactly one task packet. You only do this one thing.

1. Read the contract above and every file in `context-files` before writing code.
2. Implement the code together with ordinary unit and regression tests. Keep
   the 100% coverage gate and meaningful assertions; never weaken or bypass
   assertions or lower the coverage threshold to make a failing check green.
3. Frozen acceptance oracles are explicitly protected by
   `boundaries.must-not-touch` and `verify.protected-test-patterns` in the
   task configuration. Do not modify these files. If an oracle is wrong or
   the scope must change, return concrete evidence and a proposed revision
   to the Plan Loop. Revisions outside the task directory require an explicit
   human task; the planner cannot edit those files automatically.
4. Use targeted feedback checks while iterating, including approved
   `feedback-checks` when they distinguish the current failure. Report the
   commands actually run and their results. The orchestrator still runs the
   complete acceptance gates after you exit; do not claim unrun checks passed.
5. You NEVER declare the task "done". Completion is judged by the orchestrator
   (acceptance commands) and the Review Loop, not by you.
6. If reality disagrees with the design, or the work requires changing scope:
   STOP and escalate. Do not expand or shrink the feature on your own.
7. Do not touch anything under `boundaries.must-not-touch`, and never modify
   contract files (spec.md / task-packet-*.md).
8. When finished, write your done-report to
   `reports/<packet-id>.done.json` matching `done-report-format` exactly:
   what you did, which commands passed, which failed. Facts only.
