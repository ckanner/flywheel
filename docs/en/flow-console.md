# Workflow console

[Documentation](../README.md) · English · [简体中文](../zh-CN/flow-console.md)

The DSH Flywheel view brings task selection, workflow stages, configuration, session
interaction, and delivery into one place. Keep the CLI, platform packages, and DSH
plugin at the same version. For installation, see [Getting started](getting-started.md).

## Find a task without entering paths

The canvas starts with task selection. One registered task belonging to the current DSH conversation opens automatically; multiple tasks require a choice. Terminal runs record DSH's host session ID when it is available. This is discovery metadata, never authorization. The UI reads Go state after finding an index entry; the index is not proof of liveness or completion.

Older records without a host ID remain selectable under Recent tasks on this computer. Selection performs a read-only status query and associates the task with the conversation. Refreshing the list does not start, retry or replace a manually selected task. Read failures retain the existing canvas; unresolved operations still require reconciliation against their original task.

There are no editable task/project path fields. The project workspace remains the original repository; the execution directory may be a task checkout or independent Worktree. These locations are displayed separately when known. The task folder contains the specification, acceptance checks and progress. A prepared task can be associated using read-only status and then selected from the list.

DSH keeps the original repository in the Session header and switches the current execution directory through its public API. The desktop Host periodically attaches Flywheel SDK sessions to matching existing projects; task status also ensures the original project is registered. It does not create a desktop workspace for every packet Worktree or rewrite old Session headers. Probes use temporary persistence, leaving no new user sessions. Native Providers start sessions in the project; task prompts name the actual execution directory for every shell/file operation. Backends unable to separate a `workspace-write` sandbox root reject that combination before dispatch instead of widening permissions. Full access and supported read-only modes retain their actual semantics.

Loading a skill does not start the engine. `flywheel-task` prepares and calls `flywheel_launch`; a research/proposal-only response must explicitly say the engine was not started. Single-stage skills query `flywheel_control status` afterward to associate their canvas, or disclose the exact path when that tool is absent rather than claiming association.

| Conversation entry | Does it start the engine? | How the UI finds the task |
|---|---|---|
| `/flywheel-new <goal>` | Prepares contracts for review only | Before handing over the proposal, the assistant registers a candidate through read-only `status`; an uninitialized task shows its identity and location, not execution progress |
| `/flywheel-run <task directory>` | Starts or resumes an existing task | Start and query the original task path to associate it with the conversation; select the intended task when there are multiple candidates |
| `/flywheel-task <goal>` | Prepares, then starts | Launch and associate through `flywheel_launch`, then check status |

`new → run` and `task` use the same engine; the canvas is not exclusive to one
skill. Writing task files alone does not register a view, and a failed association
query must not be replaced with launch merely to populate the canvas. Report an
unavailable association tool explicitly. Tasks actually run from a standalone
terminal remain selectable under Recent tasks on this computer.

While the canvas is visible, registered-task discovery refreshes every five seconds.
An observed running task refreshes status every two seconds, backing off to ten
seconds after an unsuccessful response. Hidden documents skip those requests.
Refresh is read-only and does not replay uncertain operations.

## This stage versus the full workflow

When `run.kind` and `control.kind` identify the same single-stage operation, This run shows its recorded outcome. A review-only run defaults to **L1 → Review**, including the recorded verdict and score, while stating that overall delivery is not complete. Switch to Full workflow for the larger graph.

Continue workflow is behind an explicit disclosure. A passed stage does not start subsequent stages or mark the task completed. Failed, cancelled and unknown operations cannot inherit an older green verdict. Run ID, HEAD and digests are collapsed under Run details and file locations.

## Configuration

In an existing DSH conversation view, Flywheel remains after Statistics; the blank new-session landing page does not yet show these conversation views. Its navigation tabs are Workflow, Run history, and Configuration. Global settings require no task directory. Project settings target the selected Git root. Discovery still selects one complete file in task → project → global → embedded order; it does not deep-merge files.

Opening Configuration automatically reads the selected scope. Workflow policy, model assignments, budget/retries and JEV shadow advice have separate sections. Save and Discard appear beside the form only after edits. Ordinary saving takes one click; first enabling JEV confirms the task-summary transfer boundary. Serial mode omits concurrency controls. Minutes and seconds are converted precisely to engine millisecond fields.

More configuration actions provides reload and installed-default restoration. Scope changes or reloads first disclose unsaved drafts; restoring defaults loads a pending full replacement before saving. Failures retain the draft and an uncertain save requires reconciliation. Go checks file identity, backs up the original and publishes atomically. A running engine retains its snapshot. The displayed source is file-level: task → project → global → installed defaults selects one file, not a per-field overlay. New files start from the installed template.

Resources include ChatGPT through DSH or Codex Pro, Kimi Code, DeepSeek Account (Flash by default), and optional Claude Code Pro. Defaults come from the embedded template. Use config inspect and doctor for actual backend capabilities. An absent peer or unsupported permission fails explicitly. Provider sign-in stays with its normal account flow.

`permissions.bindings` overrides `loops.<role>`, `routes.<route>` or `judge-panel`. Unspecified bindings use the global policy. History overrides are role-based: execution routes share the execute policy while each packet keeps its own session. Scoped continuation requires the same task, role, goal and eligible binding; fresh also retains invocation-safety records.

The optional [JEV shadow evaluator](jev-shadow.md) is disabled by default. Enabling
it discloses the transmitted task summary; advice does not select workflow policy,
and failures do not block the main task.

## Canvas and operations

The left-to-right overview groups Plan/decomposition, scheduling, task branches, integration/final checks and PR/Merge. Selecting a node opens view-local Progress, Sessions and Run configuration details. Open task workflow drills into Execute, L1, Review and Git delivery; integration and remote delivery groups also expand. Colors have text labels. Identity, project/worktree, branch, HEAD, run ID, bindings and evidence come from actual receipts.

The canvas uses the available plugin width, with two-axis panning, zoom, fit width, current-step location, paging and a list alternative. New snapshots preserve the viewport and selection; location can switch to the running packet's page. Large tasks use bounded excerpts. Read full node evidence loads one packet with a matching fresh revision; it never changes the ledger.

Focus canvas expands the view in a plugin-owned native dialog. Exit or Escape returns without starting or stopping the task. It does not change the DSH global layout or composer. Unknown operation outcomes exit focus mode so the original-invocation reconciliation action remains visible.

Node operation confirmation freezes the observed revision; a changed snapshot invalidates it. Retry once requires a reason, while reopening a completed packet explains evidence invalidation. Review cannot run with a red current L1. Unqueried remote state, closed PR and merged PR remain distinct.

- Run stage submits the observed revision for lease-bound admission.
- Continue once records a reason and adds one Execute/L1 attempt without resetting failures or billing.
- Reopening a completed packet archives old evidence and invalidates downstream acceptance.
- Git delivery requires current L1 and independent Review evidence.
- Final acceptance runs milestone gates and Judge without starting Plan or unfinished execution units.
- Continue workflow resumes the persisted Full or ultraWork/Quick policy.
- Stop waits for managed work to settle; closing a view is not a stop.

Explicitly independent packets with satisfied dependencies can be run ahead in their registered execution directories, subject to the selected worktree policy. Integration changes invalidate old acceptance. Each asynchronous node operation records running/passed/failed/cancelled and its reason. Quick retains deterministic checks and independent final Review; omitted Plan/Judge stages are not presented as passes.

## Parallel execution

### Eligibility and limits

Read `flywheel template scheduler-policy.schema.json` for the current fields and bounds. For example:

```json
{"scheduler":{"mode":"parallel","max-parallel":2,"providers":{"deepseek-account":1}}}
```

Provider-limit keys are actual configuration channel names. The example uses the inline template's `deepseek-account`; a binding using `deepseek-account-sdk` needs that key instead. Eligibility comes from frozen packet contracts:

```yaml
schedule:
  after: [M01-T01]
  writes: [src/search/, tests/search/]
```

Dependencies reference earlier known packet IDs. Writes are normalized relative paths or directory prefixes, without globs or escapes. Include ordinary tests/configuration and account for shared test resources. Undeclared independence, overlapping scopes and live-credential units remain serial.

### Execution directories

Configuration → Workflow policy → Automatically manage task Worktrees is on by default (`worktree.enabled`). It controls automatic directory creation/management independently from parallel scheduling. The installation default allows parallel execution with a limit of two; existing complete files without `scheduler` remain serial, as do explicit serial settings. Parallel automatic management requires isolated worktrees and stops if creation fails; the legacy serial path can record an in-place fallback. With automatic management off, tasks may run concurrently in the task checkout or choose their own Worktrees. Shared directories do not provide file isolation: write-scope conflicts, candidate drift and failed acceptance still stop progress.

To select another execution Worktree, the executor first prepares a clean checkout of the same repository and task baseline, unclaimed by another unit in this workflow and with a branch not shared by multiple Worktrees, then returns only `{"flywheelExecutionDirectory":{"cwd":"/absolute/worktree","branch":"actual-branch"}}` before implementation. Go validates and records it; the next invocation, tests, Review and delivery follow that directory. Registration is not task completion and its model usage is still billed. External Worktrees are retained. A prompt-only directory claim or edited state file cannot replace this registration.

Independent Worktrees permit concurrent Execute/L1. Shared checkouts run Execute concurrently, then verify and review serially after all workers finish. One Go coordinator serializes CAS writes and accounting. Hard budgets admit only one model call at a time, retaining its slot through durable accounting. Other modes enforce total and channel concurrency. Cancellation awaits every started worker.

Review, integration and milestone gates remain serial. A combined candidate is revalidated after incorporating earlier work. Bounded execution failures retain their escalation policy. Integration conflicts and failed gates preserve the worktree and evidence. Write scopes are orchestration/acceptance contracts; full access is not an OS sandbox.

### Shared-checkout batches

When a batch shares one checkout, all executor calls finish before L1 and independent Review validate the complete combined candidate. Changes are projected onto declared scopes; undeclared and forbidden paths still fail. This does not attribute filesystem writes to individual processes. Repairs invalidate earlier proofs. Only after every peer passes can code be committed and all completions published in one state CAS; failed publication never optimistically completes a subset. Mixed batches deliver the shared checkout first, then revalidate and integrate external Worktrees. Manual integration/final checks cannot bypass the batch barrier; resume the complete workflow. The canvas shows the cohort and previous error.

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

CLI and desktop starts register run IDs. A conversation retains multiple confirmed task associations. The index lives in `runs/` beside the global configuration, optionally overridden by absolute `FLYWHEEL_RUNS_DIR`. It shows the latest 100 registrations and never scans the disk. Query Go again for liveness.

Run history uses a selection list with adjacent details for operation kind, branch, HEAD, contract/configuration conditions, elapsed time and newly observed attempts, failures and deliveries. Selecting a record does not launch it; opening its canvas refreshes current Go state. Metrics scope chooses the selected run or all calls for its task, including legacy observations. The reference compares conditions without changing the metrics target. Invocation/receipt reconciliation precedes aggregation; late receipts keep their original attribution and are not double-billed. A group with entirely missing cost shows Not reported, not $0.

Usage, cost and duration are known amounts, with missing/partial observations retained. Costs may include estimates from configured prices. Call failure rate is not code quality. Different conditions permit descriptive comparison only; quality conclusions require acceptance evidence and an independent oracle.

## PR and Merge

Inspect the project and remote first. Writes require normal GitHub CLI authentication, completed local acceptance, a still-valid candidate, and explicit repository/HEAD/base authorization. A changed registered repository identity is rejected. Fetch and push URLs must identify the same repository.

Push uses the approved commit and named task branch, without force, implicit tags or recursive submodule pushes. A separate remote task branch does not require switching the local checkout. Requery after push so an earlier Draft PR is updated instead of duplicated.

Merge rechecks the PR, head, base and checks, using `--match-head-commit`. It never uses admin bypass or enables auto-merge on its own. Merge-queue acceptance and actual MERGED state remain distinct; see the [GitHub CLI contract](https://cli.github.com/manual/gh_pr_merge).

Retries reconcile existing PR/merge results before mutations. Missing receipts remain unknown. Remote state is separate from local engine completion. Publication must bind the actual merge commit and artifacts for its own validation; Merge does not automatically publish npm packages or deploy.

## Related guides

- [Configuration](configuration.md): complete files, model bindings, permissions, and scheduling.
- [Workflow](workflow.md): preparation, execution, and PR review checkpoints.
- [Recovery](session-recovery.md): durable receipts and unresolved invocation handling.
