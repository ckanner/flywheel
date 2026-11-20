# Workflow: from a goal to a pull request

[Documentation](../README.md) · English · [简体中文](../zh-CN/workflow.md)

Flywheel turns a reviewed task contract into verified local Git changes. The host
assistant prepares a thin brief; the Go engine owns design, independent review,
scheduling, verification, recovery and delivery. The workflow console can also create and merge a PR when those remote
operations are explicitly authorized.

Use this guide for an existing repository. For installation and a disposable first
run, start with [Getting started](getting-started.md).
For incidents, issue fixes, and complex projects, use the [Usage handbook](usage-handbook.md).

## 0. Normal entry: describe a goal and launch

In DSH Chat, describe the requirement with the task skill:

```text
/flywheel-task Fix the identified issue, add regression coverage, and complete verification. mode=auto
```

The host checks the code and task execution directory, records a thin brief with
the original request and authority, then calls `flywheel_control` with
`action:planning` / `planning.action:prepare`. Go owns design and independent review
before implementation. Review-first requests stop for approval; delegated tasks
continue within their recorded authority. The plugin associates the
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

A first-success Quick implementation has two stage-agent calls: Execute and final
Review. New qualifying compact tasks first generate the design and complete single
packet together, then receive one independent joint review. This normally makes
four logical role calls from brief to delivery; internal tool turns, investigation,
retries and upgrades are additional work. Existing valid reviewed contracts remain
reusable. Required
tests, coverage, applicable L2/L3, defect sampling, budgets, and cleanup remain in
force. Plan/Judge are marked not applicable with reasons. A newly discovered risk or
verification gap saves a one-way Quick → Full upgrade, preserving work and budgets.

Use `/flywheel-new <goal>` to start design/review and stop for approval, and
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
Review. Execution allowlists and verified incremental
context are independent opt-ins described in [Configuration](configuration.md).

Authorized investigations can include exact reproduction commands in private
input copies and explicit GET/HEAD requests. Their real execution boundary,
deadline, output limits and receipts belong to the investigation contract.
Enabling JEV also permits bounded choices after confirmed implementation failures:
repair, an approved feedback check, replanning, or an already allowed execution
binding. Go retains workflow eligibility and the complete final gates; a successful
feedback experiment does not complete the implementation.

## 0.2 When native collaboration is used

Auto still selects implementation depth; the single optional JEV switch selects
only legal execution candidates. With JEV off, the configured initial model runs.
With JEV on, Go may ask once before the first Execute and again after confirmed
failure. Decisions share the existing ledger and budgets. A model, effort or
native-strategy change cold-starts the executor; it cannot bypass plan approval,
verification, independent Review or unknown-invocation reconciliation.

Swarm is useful for work that has already been split into independent components.
The reviewed packet can declare the following optional contract:

```yaml
schedule:
  after: []
  writes: ["src/parser/", "src/formatter/"]
collaboration:
  version: 1
  tasks:
    - goal: "Implement and test the parser"
      writes: ["src/parser/"]
    - goal: "Implement and test the formatter"
      writes: ["src/formatter/"]
```

The declaration requires 2–8 tasks with disjoint scopes inside `schedule.writes`,
is reviewed with the packet and makes Quick ineligible. Full, no image inputs and
this valid declaration are required even for a statically selected Antigravity
Swarm binding. Merely choosing `complex`/`batch` or listing multiple files does not
trigger Swarm. Shared mutable files or a sequential design dependency should be
resolved in the plan before admitting parallel work. The adapter requests native
collaboration; only observed native events establish that it occurred.

Read-only foreground research may use explicit subagents without the execution
split. Evidence-only incident analysis remains tool-free and cannot spawn agents;
an authorized repair later enters the ordinary reviewed execution workflow.
Orchestrator can handle a complex sequential task through its fixed native model
handoff without a Swarm split. See [Antigravity](antigravity.md) and [JEV](jev.md)
for configuration, observed-member limits and recovery.

## 1. Choose conversation and planning models separately

The current DSH conversation organizes the brief and user feedback. It does not need
to produce the complete architecture before New starts. Actual overall design uses
`loops.design-plan`; absent an override, it inherits the existing `loops.plan`
backend. Independent Design Review and Plan Review use `loops.design-review` and
`loops.plan-review`, defaulting to the `loops.review` backend. Roles and history stay
independent. A brief with explicit compact granularity, reason and evidence uses
`planning.compact` profile references for design and its joint review; otherwise
system design uses the ordinary loops. Auto is the deterministic implementation
depth selector, not a third flow or a separate classifier call.

Use `config inspect` to see `bindingSource` and the resolved backend, model and
effort. The configuration view displays inherited planning roles and lets you make
an explicit override. Existing Provider native capabilities remain available under
the same admission rules. See [Reviewed planning](reviewed-planning.md) for the
complete discussion/approval flow and [Model policy](model-policy.md) for measured
limits and model-selection considerations.

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
/flywheel-new Design this feature from the repository evidence. Compare the relevant options, run independent design review, and stop so I can discuss and approve the design.
```

No initial console form is required. Preparation should report the actual task
execution root and task files path, while retaining the original project identity.
A repository can contain several tasks; a repository path alone does not select one.

For new tasks, review the current design and findings in the Flywheel view. Submit
feedback and run Plan to revise. **Approve this version** stops before implementation;
**Approve this version and start** also authorizes continuation. Go then refines and
reviews the current child plan and activates its execution contracts. You do not
have to prepare every distant packet first. See [Reviewed planning](reviewed-planning.md).

### Legacy or explicitly prepared execution contracts

The following describes the existing manual path and the contracts activated by Go.
A runnable implementation needs a reviewed `spec.md` and at least one legal
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

For a new reviewed-planning task, first use the exact-version controls described
above. Plain Run preserves a review-first stop and does not authorize implementation
after approval-only. Use **Start approved design** to grant start authority.

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
