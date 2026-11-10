# From requirements to a merged PR

[English README](../../README.en.md) · [中文](../workflow.md) · [Getting started](getting-started.md)

This guide describes the **current source implementation** and a recommended operating workflow. It does not assert that a particular npm version has been published. Check the registry when installing. The engine's local delivery and the host's GitHub operations are separate responsibilities.

## 0. Normal entry: describe a goal and launch

With a matching CLI/plugin release containing the adaptive workflow, use:

```text
/flywheel-task Fix the identified bug, add regression coverage, and complete verification. mode=auto
```

The current host conversation researches the code, creates or reuses the task integration worktree, prepares contracts and workflow facts, and calls `flywheel_launch`. Go validates eligibility and starts the engine; DSH associates that exact task with the initiating session's Flywheel view. There is normally no need to copy paths or repeat an authorization already given. Check `flywheel control launch --help` before relying on this new entry; older installation commands do not prove its availability.

| Choice | Selection and loop |
|---|---|
| Auto, the new entry's default | Select Quick from supported contract facts; use Full when facts are insufficient or risks remain |
| Quick | One milestone and packet: Execute, all required checks, one independent final Review, Git delivery |
| Full | Plan, packet execution/review, integrated L2/L3, Judge |

Quick requires clear scope, a known implementation pattern, complete verification mappings, and a final reviewer able to assess the whole task. Missing L2/L3 needs an explicit inapplicability reason. File count does not establish safety: authentication, permissions, payments, migrations, concurrency, and unresolved design use Full. An explicit Quick request still has to qualify. A verification gap or new risk causes a persisted upgrade to Full while preserving task identity, work, budget, and history.

The first-success Quick path has two stage-agent calls, Execute and final Review. Host preparation, internal model/tool turns, and retries are separate. Required coverage, integration/user-path tests, and applicable defect sampling still run against the final candidate; Plan/Judge are recorded as not applicable with reasons, never as fabricated passes.

Use `/flywheel-new` when the user only wants a proposal and `/flywheel-run <actual task path>` to start or resume an existing task. Preserve prior implementation and delivery authorization; honor an explicit request to review a proposal first. The sections below explain the individual checkpoints, not mandatory approval stops for the unified entry.

## 1. Choose a model for preparation

Preparation turns a requirement into a Spec, task packets, acceptance commands, boundaries, and recorded dry-run evidence. `flywheel-task` and `flywheel-new` use the host's current conversation model. They do not switch it based on `loops.plan`; that binding controls planning and revisions after the Go engine starts.

For accounts that expose the named models, these are practical starting points:

| Requirement | Preparation model and effort | Reason for the recommendation |
|---|---|---|
| Ambiguous requirements, architecture, protocol, or process lifecycle changes | `chatgpt/gpt-6-astra`, `high` | Spend more reasoning on constraints, failure cases, and testable acceptance before execution |
| A clear repository feature with known boundaries | `chatgpt/gpt-6.1-sol`, `high` | A practical default for making a concrete implementation plan |
| A small, explicit fix or documentation update | `deepseek-account/deepseek-flash`, `high` | Keep preparation proportional to a narrow task |

These are engineering recommendations, not benchmark results or evidence that one setting is universally best. Confirm the exact model ID and supported effort in the selected account and adapter. The checked DSH Flash adapter accepts `off/low/high/max`, not `medium`. A catalog entry or accepted configuration does not prove that real inference has succeeded.

DeepSeek uses the existing signed-in, funded DSH account. `deepseek-official` API access is an explicit alternative; the default preset has no fallback to it. User-defined fallback policies remain explicitly configured. `high` remains an engineering baseline without project A/B evidence; do not automatically raise every stage to `xhigh` or `max`.

Use the preparation model to identify risks and improve the contract. Increasing effort cannot repair missing requirements, an invalid test oracle, or an incorrectly chosen working directory. Later execution routes and the independent Review/Judge stages remain explicit configuration; see [Configuration](configuration.md).

## 2. Create the task integration worktree first

For an existing repository, create or deliberately reuse a task branch in its own worktree before invoking `flywheel-new`. The host or user performs this step. Choose unused names and substitute the repository's actual base branch:

```sh
git status --short
git fetch origin
git worktree add -b task/my-feature ../repo-my-feature origin/main
cd ../repo-my-feature
git branch --show-current
```

Inspect the initial status before proceeding. These commands create a new task branch without moving the original checkout's `main`. Do not use forced branch replacement to resolve a naming collision. If a task branch/worktree already exists, inspect its state and reuse it deliberately.

Keep the task at `docs/my-feature` **inside this worktree**. Run preparation and later CLI commands from this root. If using the desktop view, choose this task directory; leave its optional project directory blank only when the DSH session's current directory is already this worktree. Do not pair the original checkout's task directory with the new worktree's `cwd`, or change the task worktree's branch during a run.

The engine's integration checkout is the supplied `cwd`, not a hardcoded `main` branch. Its packet branches normally use `flywheel/<slug>/<packet-id>` and start from that checkout's HEAD. Successful packet delivery fast-forwards into the branch currently checked out there. This is why choosing a task branch before starting matters.

### Isolation has a fallback

Worktrees isolate working directories and indexes; they are not security sandboxes. The engine normally creates a packet worktree, but a creation failure can record the reason and fall back to **in-place** execution. There is currently no switch that requires successful isolation in every case.

If strict task isolation is required and the log reports `worktree unavailable`, stop and inspect the workspace and recorded mode. Correcting a directory and restarting does not automatically convert an already persisted in-place packet into a new worktree. Preserve its work and resolve the recorded situation explicitly; do not edit `state.json`, delete results, or force-reset a branch to hide it.

## 3. Prepare and review the contracts

In the task worktree, ask the host to run `flywheel-new`, or inspect the bundled templates and prepare the contract files manually:

```sh
flywheel template --list
flywheel skills read flywheel-new
```

In DSH's conversation input, select the skill from the `/` menu or send:

```text
/flywheel-new Prepare docs/my-feature for this requirement. Research the actual code, define packets, routes, boundaries and executable acceptance checks, run L1 to record the baseline, then present the contracts for review.
```

**No initial Flywheel-view form is required.** The assistant creates or reuses the task worktree, prepares `spec.md` and task packets, and reports two absolute paths: `repo_dir` for the project worktree and `task_dir` for its `docs/my-feature` task files. A project can contain several task directories; its root alone does not identify a Flywheel task.

DSH uses `/flywheel-new`. Kimi uses `/skill:flywheel-new`; the prefixes are host-specific. See the pinned [DSH skill entry contract](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/client/ui-skill/README.md) and [Kimi slash-command reference](https://moonshotai.github.io/kimi-code/en/reference/slash-commands.html).

A runnable task needs a reviewed `spec.md` and at least one legal `task-packet-*.md`, using the bundled schemas. The preparation should establish:

- Requirement IDs, expected behavior, and scope exclusions.
- Packet dependencies, execution routes, editable paths, and forbidden paths.
- Real L1 commands and milestone L2/L3 commands with expected exit codes.
- Frozen acceptance tests or other oracles, separately protected from implementation tests.
- Actual dry-run results, with failure output and exit codes where applicable.

The unified entry also records `workflow` in the Spec: version, requested mode, qualification facts, code evidence, risks, and reasons for inapplicable checks. See [Workflow policy and recovery](configuration.md#workflow-policy-and-recovery). A model's low-risk label alone is insufficient. Old contracts without this field remain Full.

Run the stated commands before recording their results. An intentionally failing baseline can be valid evidence; a guessed passing result cannot. Ordinary implementation and regression tests can be added by the executor. Frozen acceptance and configuration must not be rewritten to make an implementation pass.

Review the files, then commit the substantive initial contracts so packet worktrees can obtain them from Git. The following assumes the index has been checked for unrelated staged work:

```sh
git diff -- docs/my-feature
git diff --cached --stat
git add -- docs/my-feature
git diff --cached
git commit -m "docs: prepare my-feature contracts"
```

Stage any additional acceptance fixtures separately after review. Do not create `state.json`; the engine initializes and maintains it. An empty commit does not replace committing the actual contracts.

## 4. Open a task-level Draft PR

Once the reviewed contracts are committed, a Draft PR gives the task a durable review location. The user or authorized host runs the GitHub steps:

```sh
git push -u origin task/my-feature
gh pr create --draft --base main --head task/my-feature
```

Describe the problem, expected outcome, acceptance criteria, and remaining work. Usually one independently deliverable requirement or milestone belongs in one PR. Packets are the engine's internal execution units; the engine does not automatically create a PR for each packet.

Keep the PR in draft while implementation and acceptance are still changing. Opening it early does not imply that it should be merged early.

## 5. Run from the same task worktree

After reviewing and saving the contracts, send these skill invocations separately in DSH Chat, substituting the actual `task_dir` returned during preparation. Query after starting; use the last entry when stopping is needed:

```text
/flywheel-run /absolute/path/to/repo-my-feature/docs/my-feature
/flywheel-status /absolute/path/to/repo-my-feature/docs/my-feature
/flywheel-stop /absolute/path/to/repo-my-feature/docs/my-feature
```

The skills resolve the project root from that existing task directory and invoke the CLI. They do not require opening or filling in the view. For direct terminal use, keep the current directory at the same task worktree root:

```sh
flywheel config inspect --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --probe --json
flywheel control start --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
```

Preflight does not infer with a model or execute acceptance commands. Its unknown and warning results still need interpretation. Starting the real task invokes the configured models. A control response's `ok` describes the operation; final task completion is in `state.status`.

For the new unified entry, use `control launch ... --mode auto` in place of `start`. It consumes prepared contracts and defaults to Auto. `start` and foreground `run` accept optional `--mode auto|quick|full`; omission retains the contract or persisted choice. Existing legacy state remains Full. Restarting or requesting Auto does not reclassify or downgrade an established Full task.

DSH's `flywheel_launch` and `flywheel_control` start associate the supplied `taskDir` and `cwd` with the initiating session. Calling the CLI from a shell alone does not establish a desktop association. For terminal-started tasks or manual opening, enter the actual task directory and project root. The project field may be blank only if the DSH session directory already matches; it is read through public `workingDirectory.get(session)`. A shell `cd` alone does not change that directory. The plugin does not scan or guess tasks.

### Working directory by stage

| Stage or entry | Working directory | Responsibility |
|---|---|---|
| `flywheel-task` / `flywheel-new` preparation | Task integration worktree | Host creates or reuses it according to the skill |
| Full Plan / revision after upgrade | Task integration worktree | Engine uses the startup root to revise the task contracts |
| Execute, L1, Review | Current packet candidate workspace, normally its packet worktree | Engine creates, records, and reuses that workspace |
| Quick's applicable L2/L3 | The same packet candidate, before final Review | The engine must not validate a stale integration checkout |
| Full L2/L3, Judge | Task integration worktree | Engine checks the integrated milestone |
| Intake, Garden | Selected task's worktree | Host selects the root; Go writes proposals |
| Status, Stop | Original task directory and execution root | Query or stop the existing task without requiring a new worktree, branch switch, or commit |

Model-call permissions follow `permissions.mode`. Under `role-based`, the default for existing files that omit this setting, Plan/Execute request `workspace-write`; Review/Judge and DSH/Codex (including Codex Pro) Intake/Garden request `read-only`. Under `full-access`, every stage requests full access through the selected backend's public interface. Kimi print retains native auto behavior and provides no claimed OS-level read-only or workspace confinement. Stage duties remain unchanged: reviewers evaluate candidates, auxiliary modules produce advice, and candidate identity checks still run before and after review. See [Configuration](configuration.md) for configuration and migration details.

Every loop does not create its own worktree. The outer task worktree spans the deliverable; a packet workspace spans its execution, acceptance, review, and delivery. Plan commits valid contract revisions in the integration worktree and attempts to fast-forward existing packet worktrees to that checkpoint while preserving implementation work. Conflicts stop for resolution.

Go schedules packets serially and does not need a persistent parent model session to supervise them. `sessions.mode=scoped` permits ordinary repairs to continue the packet's Executor history, and a Reviewer may continue its own independent history for the same review scope. Every re-review still checks the complete current candidate, contract, and applicable requirements; closing old findings is insufficient. Plan history is task/milestone scoped, primary Judge and second opinion are separate, and an explicit Plan `resident-session:false` remains effective. Parallel DAG scheduling and team communication remain outside this implementation.

New configuration templates explicitly select scoped mode; old files without the setting retain fresh mode. Fresh disables general role history reuse while preserving the legacy Kimi Plan resident option. Both modes journal supported main-task Codex Pro, DSH, and Kimi calls. Other adapters, mock, and Intake/Garden retain their existing paths. History reuse does not remove verification requirements or add stages to Quick's two-call first-success path. See [Session policy](configuration.md#session-policy-history-reuse-and-invocation-safety).

Saved receipt recovery retains the original candidate, contract, and role guard evidence. Re-running sampling or verification cannot replace evidence bound to a pending receipt. Consuming an already completed result makes no new model request and does not bill twice; budgets constrain new dispatches. P3 uses public Codex/Claude lifecycle events, durable Go ACKs, and read-only invocation journals; recovered success still requires the original post-call candidate and role proof. P4 adds role overrides, bounded quality rebuilds, and paired evaluation, with thresholds disabled by default and live effects separately measured. See [recovery and quality](session-recovery.md) and [paired evaluation](session-evaluation.md).

`flywheel-verify` runs deterministic acceptance only. Full retains the existing packet L1 / milestone L2/L3 behavior; Quick also collects applicable L2/L3 in the actual candidate workspace. It does **not** call Review or Judge, create worktrees, or perform local delivery. `--only execute` remains a partial Execute/L1 loop. A Quick task's `--only review` performs final Review without delivery; a verification gap, upgrade verdict, or lost Quick eligibility immediately saves the one-way Full checkpoint. The next complete run enters Full. `--only plan` requires Full, so explicitly request `--mode full` when a Quick task needs planning. Resume the complete run to let Go handle the remaining stages and delivery.

The bundled skills check task location before preparation, run, resume, and manual loop actions. By default, development starts outside `main`, `master`, a known default branch, or detached HEAD unless the user has explicitly chosen and authorized that location. Direct CLI and GUI actions do not read or execute `SKILL.md`, create the outer task branch, or inherit those host checks. Read-only status and stopping must remain available even when development-entry checks would fail.

## 6. Understand local completion before merging the PR

A packet is delivered only after required checks and independent Review match its current candidate, and Git delivery is confirmed. The engine commits implementation changes, performs `git merge --ff-only` into the integration checkout, persists delivery state, and cleans up only when safe. Failed commits or merges preserve the workspace. Full additionally runs integrated L2/L3 and Judge. Quick runs applicable L2/L3 before a final Review that covers every requirement, acceptance sufficiency, and continued eligibility; it then persists that same candidate.

After the complete task passes, inspect the integrated changes and push them:

```sh
git status --short
git log -5 --oneline
git push origin task/my-feature
gh pr ready
gh pr checks
```

Before merging, review the current PR diff, relevant acceptance evidence, required CI results for the current head, and the effect of the current base branch. Changes after a green run or review may require renewed evidence. If a required CI check cannot run, record the blocker and use the repository's explicitly authorized exception process; do not describe an unavailable check as passed.

Manual merge after those gates is the recommended default. Auto-merge can be appropriate for a repository with an explicit policy, required checks/reviews, protected branches, and a defined class of eligible changes. Enabling it is a host/user decision; Flywheel does not infer that permission from an early Draft PR or a local packet pass.

## 7. Stop and recover without losing evidence

```sh
flywheel control stop --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
```

Check the stop result and the actual failure before resuming from the **same** root and task path with `control start` or `flywheel-run`. A startup readiness timeout is a reason to query status and logs, not to launch a duplicate run. A stopped engine does not prove that every separately managed process is gone.

Supported main-task adapters retain invocation facts and terminal receipts. Unknown dispatch, outcome, or cleanup blocks further execution in both session modes, including after a backend change. Confirm the task-owned plugin/app-server/guardian processes have ended when cleanup is uncertain; a fixed delay is not evidence, and process exit alone does not establish a known model outcome. Go must reconcile a trusted receipt against the original identity and evidence. Changing to fresh, switching task directories, resetting state, or retrying through another channel cannot bypass an unresolved invocation. Avoid broad process killing that could terminate unrelated sessions.

For a delivery conflict, preserve the packet worktree, inspect Git state, resolve the conflict deliberately, then resume and let the engine revalidate evidence. Do not use forced cleanup or manual state edits as a recovery shortcut. Configuration changes require restarting the engine.

## Reference boundaries

The engine owns local state, bounded loops, verification, and Git delivery. The authorized host/user owns task-branch setup, GitHub pushes and PRs, remote CI policy, PR merge, and publication. Repository visibility and licensing are separate decisions.

Implementation references: [engine](https://github.com/ckanner/flywheel/blob/main/internal/machine/run.go), [Git/worktree operations](https://github.com/ckanner/flywheel/blob/main/internal/verify/verify.go), [design](https://github.com/ckanner/flywheel/blob/main/docs/design.md), and [bundled skills](https://github.com/ckanner/flywheel/tree/main/skills). Additional documentation: [Git worktree](https://git-scm.com/docs/git-worktree) and [GitHub auto-merge](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/incorporating-changes-from-a-pull-request/automatically-merging-a-pull-request).

## Interactive console delivery

The [workflow console](flow-console.md) adds stage controls, handoff, declared parallelism and explicitly authorized PR/Merge operations. Local completion does not authorize remote writes. An earlier Draft PR is reconciled after push; publication validates the actual merge commit separately.

Default-off [JEV shadow evaluation](jev-shadow.md) can independently observe new tasks without blocking execution on failure. Advice does not participate in Quick/Full selection or downgrade existing Full tasks. This feature requires the 0.30.1 CLI/plugin.
