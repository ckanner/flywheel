# Workflow: from a goal to a pull request

[Documentation](../README.md) · English · [简体中文](../zh-CN/workflow.md)

Flywheel turns a reviewed task contract into verified local Git changes. The host
assistant prepares the task; the Go engine owns scheduling, verification, recovery,
and delivery. The workflow console can also create and merge a PR when those remote
operations are explicitly authorized.

Use this guide for an existing repository. For installation and a disposable first
run, start with [Getting started](getting-started.md).
For incidents, issue fixes, and complex projects, use the [Usage handbook](usage-handbook.md).

## 0. Normal entry: describe a goal and launch

In DSH Chat, describe the requirement with the task skill:

```text
/flywheel-task Fix the identified issue, add regression coverage, and complete verification. mode=auto
```

The host researches the code, selects or reuses the task execution directory,
prepares the Spec, packets, executable acceptance checks, and workflow evidence,
then calls `flywheel_launch` within the authorized scope. The plugin associates the
actual task with the conversation. Loading a skill or receiving a proposal alone
does not mean the engine started; check the launch result and task status.

| Mode | Selection and execution |
|---|---|
| Auto, the default | Go selects Quick when recorded qualification facts are sufficient; otherwise Full |
| Quick, also labelled ultraWork in the console | One milestone and packet; Execute, required checks, independent final Review, and Git delivery |
| Full | Plan, packet execution and review, integrated L2/L3 checks, and Judge |

Quick needs clear scope, a known implementation approach, complete requirement
coverage, and a final review that can assess the whole task. Missing L2/L3 checks
need explicit reasons. Authentication, permissions, payments, migrations,
concurrency, or unresolved design risks select Full. An explicit Quick request
still has to qualify; file count alone is not a risk assessment.

A first-success Quick run has two stage-agent calls: Execute and final Review.
Preparation, internal tool turns, retries, and upgrades are additional work. Required
tests, coverage, applicable L2/L3, defect sampling, budgets, and cleanup remain in
force. Plan/Judge are marked not applicable with reasons. A newly discovered risk or
verification gap saves a one-way Quick → Full upgrade, preserving work and budgets.

Use `/flywheel-new <goal>` for preparation without launching, and
`/flywheel-run <absolute-task-directory>` for an existing task. Kimi uses
`/skill:flywheel-task` and the corresponding `/skill:` names.

## 0.1 Investigation and direct controls

Task can accept “diagnose this and explain the cause” directly, prepare its contract
and enter the Go-owned investigation. Calling New first is not required. Report-only
goals stop at the report; authorized repair continues within the original scope
when the evidence supports a conclusion. See [Investigation](investigation.md) for
contracts, evidence, JEV and recovery. New → review → Run remains appropriate when
proposal review is requested. Plan / Exec / Verify / Review / Status / Stop remain.

Full reuses a confirmed, persisted Plan checkpoint when its inputs and baseline stay
valid. Explicit Plan, verification gaps, changed contracts or untrusted baselines
still replan. Targeted executor checks complement the complete Go gate and independent
Review. Default model bindings remain; execution allowlists and verified incremental
context are independent opt-ins described in [Configuration](configuration.md).

Authorized investigations can include exact reproduction commands in private
input copies and explicit GET/HEAD requests. Their real execution boundary,
deadline, output limits and receipts belong to the investigation contract.
Enabling JEV also permits bounded choices after confirmed implementation failures:
repair, an approved feedback check, replanning, or an already allowed execution
binding. Go retains workflow eligibility and the complete final gates; a successful
feedback experiment does not complete the implementation.

## 1. Choose a model for preparation

`flywheel-task` and `flywheel-new` run in the host's current conversation. The
`loops.plan` binding controls planning **after the engine starts**; it does not
change that host conversation's model.

| Preparation work | Suggested DSH provider / model | Effort |
|---|---|---|
| Architecture, protocols, lifecycle, or uncertain acceptance | `chatgpt` / `gpt-6-astra` | `high` |
| A bounded feature with established implementation patterns | `chatgpt` / `gpt-6.1-sol` | `high` |
| An explicit small fix or documentation update | `deepseek-account` / `deepseek-flash` | `high` |

These are engineering starting points, not project benchmark results. Check exact
model IDs and supported effort in the selected account. Raising effort does not
repair missing requirements or an invalid test oracle. Runtime defaults and the
separate account channels are described in [Model policy](model-policy.md) and
[Configuration](configuration.md).

## 2. Choose the task execution directory

Keep these locations distinct:

| Location | Purpose |
|---|---|
| Project workspace | Stable repository identity for Provider sessions; normally the original checkout |
| Task integration directory | Checkout and branch where contracts are prepared and verified packets are integrated |
| Task files directory | `docs/<slug>` inside that checkout; contains Spec, packets, state, and reports |
| Packet execution directory | Candidate checkout used by Execute, verification, and Review; usually an engine-managed worktree |

The project workspace and execution directory may differ. Provider project
membership does not redirect file operations: prompts and evidence identify the
actual execution directory. A backend that cannot represent the selected split
`workspace-write` sandbox rejects that combination before dispatch.

For a repository using the default worktree policy, create or deliberately reuse a
task branch and worktree before preparation. Inspect existing changes first, use
unused names, and substitute the repository's actual base branch:

```sh
git status --short
git fetch origin
git worktree add -b task/my-feature ../repo-my-feature origin/main
cd ../repo-my-feature
git branch --show-current
```

Keep this task's files at `docs/my-feature` inside the selected checkout. The
integration branch is the branch checked out in the supplied `cwd`; it is not
hardcoded to `main`. Do not pair task files from one checkout with another
checkout's `cwd`, or switch its branch during a run.

`worktree.enabled` controls automatic **packet** worktree management, independently
of scheduling. With it disabled, the selected task checkout can be shared or an
external worktree can be registered before implementation. External directories
must satisfy the engine's baseline, branch, cleanliness, and ownership checks; see
[Parallel execution](flow-console.md#parallel-execution).

### Isolation and failure behavior

Worktrees separate working directories and indexes; they are not security sandboxes.
The legacy serial path can record a worktree-creation failure and fall back to
in-place execution. Parallel execution with automatic worktree management requires
isolated directories and fails if they cannot be created. Disabling automatic
management explicitly permits shared-checkout execution, with different validation
and batch-delivery rules.

If isolation is required, inspect the recorded workspace mode and stop on an
unexpected fallback. Preserve existing work and resolve the failure; do not edit
`state.json` or force-reset a branch to manufacture an isolated state.

## 3. Prepare and review the contracts

From the task integration directory, use the preparation skill or inspect the
bundled templates:

```sh
flywheel template --list
flywheel skills read flywheel-new
```

```text
/flywheel-new Prepare docs/my-feature for this requirement. Inspect the code, define bounded packets and executable acceptance checks, run the baseline checks, and present the contracts for review.
```

No initial console form is required. Preparation should report the actual task
execution root and task files path, while retaining the original project identity.
A repository can contain several tasks; a repository path alone does not select one.

A runnable task needs a reviewed `spec.md` and at least one legal
`task-packet-*.md`. Establish requirement IDs, exclusions, routes, editable and
protected paths, L1 commands, milestone L2/L3 commands, and actual baseline results.
Keep frozen acceptance separate from implementation tests. Executors may add normal
unit/regression tests; they may not weaken protected acceptance to obtain a pass.

The Spec's `workflow` records qualification facts, risks, and reasons for inapplicable
checks. Parallel packets also need explicit `schedule.after` and `schedule.writes`.
See [Workflow policy](configuration.md#workflow-policy-and-recovery) and
[Scheduling and worktrees](configuration.md#scheduling-and-worktrees) for field
examples. Contracts without workflow remain Full; packets without scheduling
independence remain serial.

Commit the reviewed contracts so packet worktrees can obtain them from Git. Inspect
the index for unrelated changes first:

```sh
git diff -- docs/my-feature
git diff --cached --stat
git add -- docs/my-feature
git diff --cached
git commit -m "docs: prepare my-feature contracts"
```

Stage any additional acceptance fixtures separately after review. The engine creates
and maintains `state.json`; neither preparation skills nor models write it.

## 4. Use a task-level Draft PR when useful

A Draft PR can provide a review location once the initial contracts are committed.
The authorized host or user can open it before execution:

```sh
git push -u origin task/my-feature
gh pr create --draft --base main --head task/my-feature
```

Describe the problem, expected outcome, acceptance, and remaining work. Usually one
independently deliverable requirement or milestone belongs in one PR. Packets are
internal execution units, not automatically separate PRs. An early Draft PR does not
authorize merge. The console's later PR operation re-queries an existing PR after
push, avoiding duplicate creation.

## 5. Run from the same task execution directory

For an existing prepared task, run these commands from its integration checkout:

```sh
flywheel config inspect --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --probe --json
flywheel control launch --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --mode auto
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
```

Preflight sends no inference prompt and runs no acceptance command. Starting the
real task invokes the configured models. A control response's `ok` describes that
operation; overall completion is reported by `state.status`.

`launch` defaults to Auto. `start` and foreground `run` accept
`--mode auto|quick|full`; omission preserves the contract or persisted selection.
Status/stop never switch modes. Restarting or selecting Auto cannot downgrade an
established Full task.

DSH equivalents are `/flywheel-run <absolute-task-directory>` and
`/flywheel-status <same-path>`. The console follows the current conversation's explicit
tool association, falling back to the latest registered run with an exactly matching
DSH host session ID. A prepared or standalone terminal task needs a successful
`flywheel_control` call with `action: "status"` and its actual task and execution paths
in the intended conversation. A terminal status command alone does not establish
that association. From 0.34.2, multiple associated tasks appear in a conversation-only navigator;
a single task opens directly. There is no editable path form. Runs from other
conversations and unassociated runs remain hidden. See [Workflow console](flow-console.md).

### Working directory by stage

| Stage | Execution directory |
|---|---|
| Preparation; Full Plan and permitted contract revision | Task integration checkout |
| Execute, L1, independent Review | Current packet candidate directory |
| Quick's applicable L2/L3 | The same packet candidate, before final Review |
| Full L2/L3 and Judge | Task integration checkout after packet delivery |
| Intake and Garden | Selected task execution root |
| Status and Stop | Original task files and execution root; no new branch or worktree required |

These are execution locations. Provider sessions retain the separately recorded
project workspace. The task checkout spans the deliverable; each packet directory
spans implementation, checks, review, and delivery. A new loop does not imply a new
worktree or a permanently running parent model.

### Scheduling and role history

The embedded installation default selects `scheduler.mode=parallel` with a maximum of two.
Existing complete configuration files that omit `scheduler` remain serial, as do
explicit serial settings. Only ready packets with explicit dependencies and
nonoverlapping declared write scopes qualify. Independent worktrees permit
concurrent Execute/L1. Shared-checkout batches run Execute concurrently, then
validate and review the combined candidate. Review, integration, and final gates
remain serial. Hard budgets allow one model call in flight until its accounting is
durable. See the [console guide](flow-console.md#parallel-execution).

The optional `scheduler.dispatch="ready-queue"` fills free Execute/L1 slots among
already independent Full packets with automatic Worktrees. Review, Plan revisions
and integration still wait for started workers to settle. `scheduler.prewarm=true`
can prepare one DSH Review runtime alongside a serial packet's deterministic
checks, without sending a model prompt early. Parallel execution lanes and shared
candidates do not prewarm; after an independent cohort settles, its serial Review
path may start preparation. The useful overlap is mainly serial post-Execute L1
and applicable Quick checks. Defaults remain batch dispatch and no prewarming;
these settings do not change Task/New/Run or Auto/Quick/Full. Details and limits are
in [Configuration](configuration.md#ready-queues-and-runtime-preparation).

`scoped` history can reuse an eligible packet Executor session for repairs. Review
uses its own independent history and always evaluates the complete current candidate.
Plan is task/milestone scoped; Judge and second opinions remain separate. Old
configuration without `sessions` retains fresh behavior. Both modes preserve
invocation-safety records and block on unknown dispatch, outcome, or cleanup.
[Recovery](session-recovery.md) explains trusted receipts, durable checkpoints, and
bounded context rebuilding.

### Partial stages and permissions

`flywheel-verify` runs deterministic acceptance without Review/Judge or Git delivery.
`--only execute` is a bounded Execute/L1 loop. Quick's `--only review` performs final
Review without delivery and saves a required Full upgrade without starting Plan.
`--only plan` requires Full. Resume the complete run for remaining stages and delivery.
The console offers the same managed stage operations with revision checks.

Model permissions follow the effective global policy and per-binding overrides.
Full access does not remove frozen boundaries or reviewer candidate checks. Kimi
print does not provide an OS sandbox guarantee. Skills also check the chosen task
location before development; direct CLI/GUI calls do not execute `SKILL.md` or create
the outer task branch. Read-only status and stopping remain available independently.

## 6. Understand local completion before merging the PR

Local delivery requires current checks, independent Review, and verified Git
persistence of the same candidate. Packet worktrees are integrated into the task
checkout; failed delivery preserves work. Shared-checkout batches complete only
after every member passes on the combined candidate and the batch is durably saved.
Full also requires integrated L2/L3 and Judge. Quick requires a final Review covering
all requirements, acceptance sufficiency, and continued eligibility.

Remote delivery is a separate operation. You can inspect and push the task branch
manually:

```sh
git status --short
git log -5 --oneline
git push origin task/my-feature
gh pr ready
gh pr checks
```

Or use the console's PR/Merge nodes after explicitly authorizing the repository,
source HEAD, task branch, and target branch. These operations require normal GitHub
CLI authentication and current local acceptance. Merge rechecks PR/head/base/checks
and binds the request to the approved head; it does not use admin bypass or enable
auto-merge implicitly. An accepted merge-queue request is not a confirmed merge.
See [PR and Merge](flow-console.md#pr-and-merge).

Review the current diff, required CI, and current base branch before merging. Changes
after a green run may require renewed evidence. An unavailable check must be recorded
as unavailable and handled through the repository's authorized process. Publication
or deployment has its own validation and is not triggered merely by clicking Merge.

## 7. Stop and recover without losing evidence

```sh
flywheel control stop --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
```

Check the stop receipt and actual failure before resuming from the same paths with
`control start` or `/flywheel-run`. A readiness timeout or disconnected request calls
for reconciliation, not a duplicate launch. Closing a view does not stop the engine.

Unknown dispatch, model outcome, or cleanup blocks further execution in both session
modes. Process exit alone does not prove a known model result. Go must reconcile a
trusted receipt against its original identity and evidence; switching to fresh,
changing backends, or resetting state cannot bypass an unresolved invocation.

Preserve worktrees and reports when delivery conflicts. Resolve the actual Git
conflict and let the engine revalidate. Configuration changes require an engine
restart. Controlled human interaction with an eligible existing session is described
in [Provider session interaction](flow-console.md#controlled-provider-session-interaction).

## Related guides

- [Configuration](configuration.md): model bindings, permissions, budgets, and policies.
- [Workflow console](flow-console.md): tasks, stages, sessions, run comparison, and remote delivery.
- [Recovery and context quality](session-recovery.md): interrupted calls and safe continuation.
- [JEV decisions](jev.md): one default-off switch for investigation and bounded failure decisions; Go retains workflow eligibility and acceptance.
- [System design](../design.md): authoritative implementation contracts, in Chinese.
