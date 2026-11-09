# Flywheel workflow console

This guide describes the current source. Source, development candidates, local installation and npm publication are separate acceptance boundaries. Keep the CLI, platform packages and DSH plugin at one version.

## Configuration

In an existing DSH conversation view, Flywheel remains after Statistics; the blank new-session landing page does not yet show these conversation views. Its pages are Workflow canvas, Global/project configuration, and Runs and comparison. Global settings require no task directory. Project settings target the selected Git root. Discovery still selects one complete file in task → project → global → embedded order; it does not deep-merge files.

Read a draft, edit the resource/Harness, model, effort, permissions and history policy, then inspect the target and diff. Version defaults replace the complete configuration. Go checks the content revision, backs up the original and publishes atomically. A running engine retains its snapshot; subsequent starts load the new settings.

Resources include ChatGPT through DSH or Codex Pro, Kimi Code, DeepSeek Account (Flash by default), and optional Claude Code Pro. Defaults come from the embedded template. Use config inspect and doctor for actual backend capabilities. An absent peer or unsupported permission fails explicitly. Provider sign-in stays with its normal account flow.

`permissions.bindings` overrides `loops.<role>`, `routes.<route>` or `judge-panel`. Unspecified bindings use the global policy. History overrides are role-based: execution routes share the execute policy while each packet keeps its own session. Scoped continuation requires the same task, role, goal and eligible binding; fresh also retains invocation-safety records.

## Canvas and operations

The left-to-right canvas shows Plan/decomposition, scheduling, task branches, integration, final acceptance, PR and Merge. A packet expands into Execute, L1, Review and Git delivery. State colors have text labels. Details identify the DSH source conversation, project/worktree, branch, HEAD, run ID, actual invocation binding and failure evidence.

The canvas supports panning, locating the current node and packet paging. Large tasks use bounded diagnostic excerpts. Read full node evidence loads one packet with a matching fresh canvas revision; it never changes the original ledger.

- Run stage submits the observed revision for lease-bound admission.
- Continue once records a reason and adds one Execute/L1 attempt without resetting failures or billing.
- Reopening a completed packet archives old evidence and invalidates downstream acceptance.
- Git delivery requires current L1 and independent Review evidence.
- Final acceptance runs milestone gates and Judge without starting Plan or unfinished execution units.
- Continue workflow resumes the persisted Full or ultraWork/Quick policy.
- Stop waits for managed work to settle; closing a view is not a stop.

Explicitly independent packets with satisfied dependencies can be run ahead in isolated worktrees. Integration changes invalidate old acceptance. Each asynchronous node operation records running/passed/failed/cancelled and its reason. Quick retains deterministic checks and independent final Review; omitted Plan/Judge stages are not presented as passes.

## Parallel execution

Read `flywheel template scheduler-policy.schema.json` for the current fields and bounds. For example:

```json
{"scheduler":{"mode":"parallel","max-parallel":2,"providers":{"deepseek-account":1}}}
```

Provider-limit keys are account channel names. Eligibility comes from frozen packet contracts:

```yaml
schedule:
  after: [M01-T01]
  writes: [src/search/, tests/search/]
```

Dependencies reference earlier known packet IDs. Writes are normalized relative paths or directory prefixes, without globs or escapes. Include ordinary tests/configuration and account for shared test resources. Undeclared independence, overlapping scopes and live-credential units remain serial.

Only Execute/L1 run concurrently. Each unit has a worktree; one Go coordinator serializes CAS writes and accounting. Hard budgets admit only one model call at a time, retaining its slot through durable accounting. Other modes enforce total and channel concurrency. Cancellation awaits every started worker.

Review, integration and milestone gates remain serial. A combined candidate is revalidated after incorporating earlier work. Bounded execution failures retain their escalation policy. Integration conflicts and failed gates preserve the worktree and evidence. Write scopes are orchestration/acceptance contracts; full access is not an OS sandbox.

## Controlled Provider session interaction

Read latest response opens the exact digest-bound receipt, not an invented native history path or a claim of complete provider history.

A settled, consumed, eligible packet session can be claimed. The persisted handoff prevents automatic run/step from continuing. Both the desktop prompt form and managed terminal restore that exact thread through its adapter and keep budgets/accounting. Missing history, changed bindings or a required rebuild cannot silently become a cold start.

```sh
cd -- '/absolute/task-worktree'
flywheel session '/absolute/task-worktree/docs/task-slug' --key 'execute/M01-T01'
```

Each terminal line is a prompt. `/history` reads the latest response; `/reconcile` checks the original invocation; `/return` validates and returns; `/release` releases human control while staying paused; `/quit` retains the handoff. The desktop form supports multiline prompts.

Returning runs L1, applicable Quick gates and independent Review. Human text is not a passing verdict. Candidate-bound human-takeover evidence allows normal subsequent delivery. Release does not claim verification and cannot bypass unknown effects or cleanup. Milestone sessions currently expose read-only responses; interactive handoff is packet-scoped.

Ledger storage v2 is accepted by the new engine alongside legacy v1. Older engines reject v2 rather than ignoring human ownership. Session policy-version remains 1; worker/lifecycle versions are independent.

## Discovery and comparison

CLI and desktop starts register run IDs. A conversation retains multiple confirmed task associations. The index lives in runs/ beside the global configuration, optionally overridden by absolute FLYWHEEL_RUNS_DIR. It shows the latest 100 registrations and never scans the disk. Query Go again for liveness.

Compare operation kind, branch, HEAD, contract/configuration conditions, elapsed time and newly observed attempts, failures and deliveries. Select an exact run or all calls for the current task, including unregistered legacy observations. Invocation/receipt reconciliation precedes aggregation; late receipts keep their original attribution and are not double-billed.

Usage, cost and duration are known amounts, with missing/partial observations retained. Costs may include estimates from configured prices. Call failure rate is not code quality. Different conditions permit descriptive comparison only; quality conclusions require acceptance evidence and an independent oracle.

## PR and Merge

Inspect the project and remote first. Writes require normal GitHub CLI authentication, completed local acceptance, a still-valid candidate, and explicit repository/HEAD/base authorization. A changed registered repository identity is rejected. Fetch and push URLs must identify the same repository.

Push uses the approved commit and named task branch, without force, implicit tags or recursive submodule pushes. A separate remote task branch does not require switching the local checkout. Requery after push so an earlier Draft PR is updated instead of duplicated.

Merge rechecks the PR, head, base and checks, using `--match-head-commit`. It never uses admin bypass or enables auto-merge on its own. Merge-queue acceptance and actual MERGED state remain distinct; see the [GitHub CLI contract](https://cli.github.com/manual/gh_pr_merge).

Retries reconcile existing PR/merge results before mutations. Missing receipts remain unknown. Remote state is separate from local engine completion. Publication must bind the actual merge commit and artifacts for its own validation; Merge does not automatically publish npm packages or deploy.
