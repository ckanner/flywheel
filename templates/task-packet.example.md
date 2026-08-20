---
packet-id: M01-T01
contract-version: 1
goal: Implement the auth token parser so that `parseToken` returns claims for valid JWTs and null otherwise.
requirements: [R-01, R-02]
route: default
needs-live-credentials: false
context-files: [src/auth/*.ts, docs/spec.md#M-01]
cache-scope: [docs/spec.md]
boundaries:
  must-not-touch: [src/payments/, tests/]
  max-files: 6
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

# Work instructions (DESIGN.md §4.2 执行纪律)

You are the executor for exactly one task packet. You only do this one thing.

1. Read the contract above and every file in `context-files` before writing code.
2. **Implement only — never write or modify tests** (v0.14 DC1). Acceptance
   tests are provided with this packet by the Plan Loop, or already exist in
   the repo. Creating or modifying ANY test file fails this unit: the
   orchestrator checks the git diff after you finish.
3. Code that is hard to test is a design defect: STOP and escalate so the Plan
   Loop can adjust the design — never "fix" it by touching tests yourself.
4. Run the acceptance commands yourself. Report what passed and what failed.
5. You NEVER declare the task "done". Completion is judged by the orchestrator
   (acceptance commands) and the Review Loop, not by you.
6. If reality disagrees with the design, or the work requires changing scope:
   STOP and escalate. Do not expand or shrink the feature on your own.
7. Do not touch anything under `boundaries.must-not-touch`, and never modify
   contract files (spec.md / task-packet-*.md).
8. When finished, write your done-report to
   `reports/<packet-id>.done.json` matching `done-report-format` exactly:
   what you did, which commands passed, which failed. Facts only.
