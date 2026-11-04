# From requirements to a merged PR

[English README](../../README.en.md) · [中文](../workflow.md) · [Getting started](getting-started.md)

This guide describes the **0.28.0 implementation** and a recommended operating workflow. It does not assert that a particular npm version has been published. Check the registry when installing. The engine's local delivery and the host's GitHub operations are separate responsibilities.

## 1. Choose a model for preparation

Preparation turns a requirement into a Spec, task packets, acceptance commands, boundaries, and recorded dry-run evidence. `flywheel-new` uses the model selected in the host's current conversation. It does not switch that model based on `loops.plan`; that binding controls planning and revisions after the Go engine starts.

For accounts that expose the named models, these are practical starting points:

| Requirement | Preparation model and effort | Reason for the recommendation |
|---|---|---|
| Ambiguous requirements, architecture, protocol, or process lifecycle changes | `chatgpt/gpt-6-astra`, `high` | Spend more reasoning on constraints, failure cases, and testable acceptance before execution |
| A clear repository feature with known boundaries | `chatgpt/gpt-6.1-sol`, `high` | A practical default for making a concrete implementation plan |
| A small, explicit fix or documentation update | `deepseek-official/deepseek-flash`, `high` | Keep preparation proportional to a narrow task |

These are engineering recommendations, not benchmark results or evidence that one setting is universally best. Confirm the exact model ID and supported effort in the selected account and adapter. The checked DSH Flash adapter accepts `off/low/high/max`, not `medium`. A catalog entry or accepted configuration does not prove that real inference has succeeded.

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

Keep the task at `docs/my-feature` **inside this worktree**. Run preparation and later CLI commands from this root; enter these same absolute paths in the desktop panel. Do not pair the original checkout's task directory with the new worktree's `cwd`, or change the task worktree's branch during a run.

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

A runnable task needs a reviewed `spec.md` and at least one legal `task-packet-*.md`, using the bundled schemas. The preparation should establish:

- Requirement IDs, expected behavior, and scope exclusions.
- Packet dependencies, execution routes, editable paths, and forbidden paths.
- Real L1 commands and milestone L2/L3 commands with expected exit codes.
- Frozen acceptance tests or other oracles, separately protected from implementation tests.
- Actual dry-run results, with failure output and exit codes where applicable.

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

With the current directory still at the task worktree root:

```sh
flywheel config inspect --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --probe --json
flywheel control start --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
```

Preflight does not infer with a model or execute acceptance commands. Its unknown and warning results still need interpretation. Starting the real task invokes the configured models. A control response's `ok` describes the operation; final task completion is in `state.status`.

### Working directory by stage

| Stage or entry | Working directory | Responsibility |
|---|---|---|
| `flywheel-new` preparation | Task integration worktree | Host creates or reuses it according to the skill |
| Plan | Task integration worktree | Engine uses the startup root to revise the task contracts |
| Execute, L1, Review | Current packet candidate workspace, normally its packet worktree | Engine creates, records, and reuses that workspace |
| L2/L3, Judge | Task integration worktree | Engine checks the integrated milestone |
| Intake, Garden | Selected task's worktree | Host selects the root; Go writes proposals. DSH/Codex request read-only; default Kimi has no OS-level read-only guarantee |
| Status, Stop | Original task directory and execution root | Query or stop the existing task without requiring a new worktree, branch switch, or commit |

Every loop does not create its own worktree. The outer task worktree spans the deliverable; a packet workspace spans its execution, acceptance, review, and delivery. Plan commits valid contract revisions in the integration worktree and attempts to fast-forward existing packet worktrees to that checkpoint while preserving implementation work. Conflicts stop for resolution.

`flywheel-verify` runs deterministic acceptance only. With `--packet`, it runs that packet's L1. Without a packet, it checks unfinished packets' L1 and the current milestone's L2/L3. It does **not** call Review or Judge, create worktrees, or perform local delivery. It uses the available candidate workspace and may use the integration checkout when no packet worktree is available.

The bundled skills check task location before preparation, run, resume, and manual loop actions. By default, development starts outside `main`, `master`, a known default branch, or detached HEAD unless the user has explicitly chosen and authorized that location. Direct CLI and GUI actions do not read or execute `SKILL.md`, create the outer task branch, or inherit those host checks. Read-only status and stopping must remain available even when development-entry checks would fail.

## 6. Understand local completion before merging the PR

A packet is delivered only after L1 and independent Review match its current candidate, and Git delivery is confirmed. The engine commits implementation changes, performs `git merge --ff-only` into the integration checkout, persists delivery state, and cleans up only when safe. Failed commits or merges preserve the workspace. Milestone completion additionally requires L2/L3 and Judge.

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

Codex Pro retains a machine receipt with configuration, process, cleanup, and usage facts. If cleanup is unconfirmed after the worker started, the engine escalates and stops the current run. It does not automatically try another channel, a transport retry, or a fresh Plan call. Confirm the task-owned plugin/app-server/guardian processes have ended before attempting recovery; a fixed delay is not evidence. Avoid broad process killing that could terminate unrelated sessions.

For a delivery conflict, preserve the packet worktree, inspect Git state, resolve the conflict deliberately, then resume and let the engine revalidate evidence. Do not use forced cleanup or manual state edits as a recovery shortcut. Configuration changes require restarting the engine.

## Reference boundaries

The engine owns local state, bounded loops, verification, and Git delivery. The authorized host/user owns task-branch setup, GitHub pushes and PRs, remote CI policy, PR merge, and publication. Repository visibility and licensing are separate decisions.

Implementation references: [engine](https://github.com/ckanner/flywheel/blob/main/internal/machine/run.go), [Git/worktree operations](https://github.com/ckanner/flywheel/blob/main/internal/verify/verify.go), [design](https://github.com/ckanner/flywheel/blob/main/docs/design.md), and [bundled skills](https://github.com/ckanner/flywheel/tree/main/skills). Additional documentation: [Git worktree](https://git-scm.com/docs/git-worktree) and [GitHub auto-merge](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/incorporating-changes-from-a-pull-request/automatically-merging-a-pull-request).
