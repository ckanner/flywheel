# Usage handbook: DSH and everyday tasks

[Documentation](../README.md) · English · [简体中文](../zh-CN/usage-handbook.md)

Use this handbook to investigate an incident, fix an issue, or build a larger
system from a DSH conversation. It describes the Flywheel 0.34 workflow; your
installed templates and effective configuration determine the actual settings.

Start with `/flywheel-task`: the host researches the repository and prepares the
contract; Go schedules work, verifies it and delivers accepted local Git changes.
Use New to review a proposal first, or Run for an existing task. Push, PR, merge
and publication follow their own authorization. Loading a skill is not a running task.

- [Start in DSH](#1-start-in-dsh)
- [Choose the entry and workflow mode](#2-choose-the-entry-and-workflow-mode)
- [Investigate an incident](#3-investigate-an-incident)
- [Fix issue #123 and open a PR](#4-fix-issue-123-and-open-a-pr)
- [Build a complex system with Full](#5-build-a-complex-system-with-full)
- [Models, PTC, native capabilities and JEV](#6-configure-models-ptc-native-capabilities-and-jev)
- [Status, stop, resume and manual control](#7-status-stop-resume-and-deliberate-manual-control)
- [Troubleshooting and the next action](#8-troubleshooting-and-the-next-action)

## 1. Start in DSH

### Check an existing installation

Open the intended repository in DSH and use an existing conversation. In a terminal,
replace the example path with that repository's absolute path:

```sh
flywheel version
flywheel skills list
flywheel config inspect --cwd /absolute/path/to/repo --json
flywheel doctor --cwd /absolute/path/to/repo --json
```

Check the selected configuration source, bindings and blockers after setup or changes.
Doctor sends no model prompt and does not establish quota or successful inference.
Optional `--probe` checks DSH SDK initialization, permissions and shutdown; provider
initialization may query catalogs or write caches. Probe sessions use temporary storage.

If Flywheel is not installed, use the supported installer from a terminal that can
find the desktop-supplied `dsh` command:

```sh
npx -y @ckanner/flywheel@latest install
```

Follow instructions about quitting/reopening DSH and preserve existing install options.
CLI, engines and desktop plugin must match. Installation preserves configuration;
`ok:true, ready:false` reports diagnostic blockers. Use an exact version when pinning.
See [Getting started](getting-started.md) for sign-in, prefixes and upgrades.

If the skills are missing from this project's discovery, inspect the installed
definition and install project snapshots:

```sh
flywheel skills read flywheel-task
flywheel install-skills --project /absolute/path/to/repo --json
```

Follow DSH's skill refresh/discovery behavior. Resolve reported ownership conflicts
by comparing customized files; installation does not overwrite them silently.

### Find the real task view

In an existing conversation, open **Flywheel** beside the conversation views,
after Statistics; check the overflow menu. The blank new-session landing page does
not show those views. Task/New/Run use the plugin's model tools to associate the
actual task with the conversation. The view follows that association, not a disk
scan or a task directory you type into a selector.

The panel shows real project, execution and task-file locations. The project remains
the original repository; execution may use an integration worktree. Preparation
reports both. A failed view association does not justify a duplicate launch.
For a terminal-created task, see section 7.

The current DSH model performs preparation and evidence analysis. `loops.plan`
controls the engine Plan stage, not this conversation's model. Choose the host
model for preparation; inspect engine bindings separately as described in section 6.

## 2. Choose the entry and workflow mode

| DSH entry | Purpose |
| --- | --- |
| `/flywheel-task <goal> [mode=<mode>]` | Prepare and launch authorized investigation or implementation; defaults to Auto. |
| `/flywheel-new <goal>` | Prepare a reviewable proposal and stop before execution. |
| `/flywheel-run <task-dir> [pause=review] [mode=<mode>]` | Start or resume the existing task; omitted mode preserves its selection. |
| `/flywheel-status <task-dir>` | Read progress, evidence and the next action. |
| `/flywheel-stop <task-dir>` | Stop the managed task and wait for its stop result. |
| `/flywheel-plan <task-dir>` | Run only the bounded Full Plan loop. |
| `/flywheel-exec <task-dir> [packet]` | Run a bounded Execute/L1 loop, optionally for one packet. |
| `/flywheel-review <task-dir> [unit]` | Run independent Review for an eligible current candidate. |
| `/flywheel-verify <task-dir> [packet]` | Run deterministic acceptance checks without a model. |
| `/flywheel-intake <task-dir> [source]` | Scan configured task sources into a triage inbox; candidates need approval. |
| `/flywheel-garden <task-dir>` | Produce evidence-based knowledge-maintenance proposals; do not apply them. |

`<task-dir>` is the existing path from preparation, such as `docs/fix-123` or its
absolute path; no extra `docs/` is prepended. Intake requires `intake.sources`;
the optional source is a configured name, and the default empty list scans nothing.
Garden's proposals need review before application. Kimi uses `/skill:flywheel-*`.
Exact definitions are in the
[bundled skills](https://github.com/ckanner/flywheel/tree/main/skills).

| Mode | What it means |
| --- | --- |
| `auto` | Go selects Quick only when the recorded qualification facts are sufficient; otherwise Full. |
| `quick` | One milestone and one packet: Execute, mandatory acceptance, independent final Review, then local delivery. The console also calls it ultraWork. |
| `full` | Plan, packet execution and review, integrated milestone checks, and Judge. |

Explicit Quick still needs clear scope, a known implementation, complete requirement
coverage, sufficient final review and reasons for any inapplicable L2/L3 checks.
Authentication, permission, payment, migration, concurrency or unresolved design
risks call for Full. File count alone does not establish eligibility.

Quick retains tests, applicable coverage and integration checks, budgets and cleanup.
Plan/Judge marked not applicable are not passing stages. New risks can cause a
one-way Quick-to-Full upgrade without resetting work or budget. Existing Full tasks
do not become Quick when resumed with Auto. See [Workflow](workflow.md).

## 3. Investigate an incident

### Ask for a report only

Give the host the symptom, time window and available evidence. For example:

```text
/flywheel-task Investigate intermittent redirects back to the login page after
sign-in. Inspect this repository's relevant code, available logs, recent changes
and tests. Form falsifiable hypotheses and run bounded local reproductions where
useful. This request is report-only: deliver confirmed facts and their evidence,
supported or rejected hypotheses, missing evidence, a proposed fix, regression
checks and rollback conditions.
```

Go executes approved file reads, searches and diagnostic actions from the host's
bounded contract. Reproduction commands or external GET/HEAD requests require
appropriate task authority. They do not authorize production changes; a diagnostic
input copy is not an operating-system sandbox.

When status becomes `awaiting-report`, the current host assistant must analyze the
observations and submit the evidence report through investigation control. Repeating
Run at this point does not replace the report or restart collection. The report
covers all hypotheses, uses real observation IDs, and separates facts, inference,
counter-evidence and unknowns.

Terminal report states include `supported`, `inconclusive` and `access-blocked`.
`supported` passes the evidence-reference contract; Go does not independently prove
semantic causality. Report-only work ends with the report. Its frozen intent cannot
be rewritten to authorize a repair; choose repair intent from the start when intended.

### Authorize investigation and repair together

```text
/flywheel-task Investigate and fix the intermittent redirect back to the login
page. You may change login/session-refresh implementation and its tests. Preserve
the evidence in one task; after a supported conclusion, implement within that
scope, add a regression, run required verification and independent review, and
submit a PR. Report the cause, changes, test results and rollback method. mode=auto
```

After a supported report and resolution of all action receipts, the host prepares
Spec/packets in the **same task directory and workspace**, within that authorized
implementation scope, and launches the normal workflow. Provenance, elapsed time
and applicable usage/budgets carry forward. Inconclusive evidence calls for an honest
report or a decision about new scope, not an unsupported repair. Login/session work
can carry permission or concurrency risks; Auto may choose Full even for a small diff.

The [investigation guide](investigation.md) explains evidence actions, reports and
the handoff. JEV can help select approved evidence actions when enabled; it cannot
turn a collection shortcut into proof of the root cause.

## 4. Fix issue #123 and open a PR

When the issue identifies a bounded implementation problem, start with Auto:

```text
/flywheel-task Fix issue #123 in this repository. Read the issue and relevant code,
preserve the documented behavior, add meaningful regression coverage and run the
repository's required checks. mode=auto. Reuse an existing task branch for this
issue if present. I authorize committing the task changes, pushing that branch and
opening or updating a PR against main. Do not merge the PR.
```

Replace the issue and base branch with the actual target. The host verifies scope,
existing work and acceptance, then starts the engine. It should not ask for another
mechanical implementation approval when the same work is already authorized.
Unresolved product choices or work outside that scope still need a decision.

In the Flywheel view, inspect the effective mode, active packet, verification and
independent Review. A failed acceptance remains a failure even when a model says
the patch is complete. Auto may choose Full when the issue has broader risks.

After local `completed`, inspect the actual task branch and delivered candidate.
The authorized host or console PR operation pushes and opens/updates the PR; a
pre-existing Draft PR should be reused. Packet completion and local integration
do not mean the PR was created, CI passed or the PR merged. Merge requires its own
authorization and current head/base/check evidence. See
[local delivery](workflow.md#6-understand-local-completion-before-merging-the-pr)
and [PR/Merge controls](flow-console.md#pr-and-merge).

## 5. Build a complex system with Full

Use Full for work that needs multiple contracts, dependencies and integration
checks. State the required behavior, exclusions and delivery authority. For example:

```text
/flywheel-task Implement a multi-tenant task scheduling system in this repository.
mode=full. Research the architecture, interfaces and deployment. Cover tenant
isolation, permissions, lifecycle, retries/idempotency, concurrency, observability,
migration and rollback. Prepare numbered requirements, milestones, packet
dependencies, write scopes and executable acceptance. Implement, verify, complete
independent review and Judge, and submit a PR. Proceed on the established scope;
ask about missing product choices that materially change behavior.
```

This authorizes preparation and implementation together. When you want to approve
the proposal first, use this **alternative** instead:

```text
/flywheel-new Prepare a Full proposal for the multi-tenant scheduling system above.
Show the architecture, migration/rollback approach, milestones, packet dependencies,
write scopes and executable acceptance. Do not start implementation.
```

After reviewing it, authorize Run using the **actual path returned by preparation**:

```text
/flywheel-run /absolute/task-worktree/docs/task-scheduling mode=full
```

Replace that path rather than inventing a second task with the same name. The host
commits the reviewed contracts to the chosen integration branch so packet worktrees
can obtain their exact baseline. Go alone creates and updates `state.json`.

An illustrative breakdown, to adapt to the real codebase:

| Milestone | Deliverable and acceptance |
| --- | --- |
| M1 | Domain model, interfaces and tenant/permission boundaries; contract and isolation tests. |
| M2 | Scheduler, worker, retries and idempotency; state-transition and recovery checks. |
| M3 | Administration UI and observability; end-to-end behavior and operation visibility. |
| M4 | Integration, migration, performance and recovery; overall requirements and Judge. |

Ready packets need explicit `schedule.after` and nonoverlapping `schedule.writes`
to run concurrently. Include shared interfaces, configuration and test resources
in the dependency analysis. The new template allows up to two execution lanes;
older complete files without scheduler settings stay serial. Review, integration
and final gates remain serialized; hard budgets can further restrict concurrency.
Full does not automatically enable Team, Swarm or Tower. Select those capabilities
for the intended stages as described below. See [scheduling](configuration.md#scheduling-and-worktrees).

## 6. Configure models, PTC, native capabilities and JEV

### Inspect the effective file before changing defaults

Flywheel selects one **complete** file: task → project → global → embedded defaults.
It does not merge these files. A task-specific file can hide a project/global edit.
Use `--task-dir` with `config inspect` for an existing task. In the desktop, open
**Flywheel → Settings**, select Global or Project, review the draft and save.
The next engine start reads changes; existing configuration is preserved on upgrade.

These are the embedded 0.34 installation bindings, not a claim about your setup:

| Work | Harness / account path | Model | Thinking |
| --- | --- | --- | --- |
| Plan, frontend Execute, Garden | Kimi CLI / subscription | `kimi-code/k3.1-pro` | `high` |
| Ordinary Execute | DSH / DeepSeek account | `deepseek-flash` | `high` |
| Hard Execute | Codex Pro machine / Codex login | `gpt-6.1-sol` | `high` |
| Review | DSH / ChatGPT provider | `gpt-6.1-sol` | `high` |
| Judge | DSH / ChatGPT provider | `gpt-6-astra` | `high` |
| Intake | DSH / DeepSeek account | `deepseek-flash` | `low` |

Read the installed template with `flywheel template flywheel.config.example.json`.
Quick uses its applicable Execute/Review bindings; it does not change their models.
The second Judge opinion is off; optional Claude has no default stage binding.
Selecting ChatGPT in DSH does not authenticate Codex CLI. To use the hard route,
ask preparation/planning to assign appropriate packets to it. See
[model/account configuration](configuration.md#fields-and-identities).

The new template selects scoped history, full access, automatic worktrees and
parallel scheduling with a maximum of two. Existing files that omit `sessions`,
`permissions` or `scheduler` retain fresh history, role-based permissions or serial
scheduling respectively. Batch dispatch and `prewarm:false` remain defaults.
Full access does not remove frozen scope, acceptance or independent-review checks;
it is separate from the conversation's permission dropdown. Worktrees are not
security sandboxes. Inspect these policies rather than assuming an upgrade changed them.

### Distinguish desktop delegation from Flywheel workers

DSH uses PTC to call `subagent_codex` and `subagent_claude_code`; inside, Codex/Claude
retain their native tools and children. An omitted `native` reads plugin defaults,
or preserves CLI behavior if none exist. An object replaces the entire default;
`null` explicitly turns ordinary native collaboration off. Independent Review and
interactive Team use dedicated operations.

Direct Flywheel Pro workers instead use the selected Flywheel `native-profile`;
they do not read another desktop profile's live defaults. DSH native workers share
the bundled Preset Generator's PTC composition policy and validate actual admission.
You do not need to regenerate desktop daily/heavy presets or separately install the
generator just to use that worker composition. Desktop preset customization remains
a separate operation.

The default template binds no `native-profile` to a stage. To enable Codex agents,
merge this **fragment into the effective complete configuration**, retaining its
other sections, profiles and bindings:

```json
{
  "native-profiles": {
    "codex-agents": {
      "harness": "codex-pro",
      "options": {
        "version": 1,
        "agents": {"enabled": true, "maxConcurrentThreads": 3}
      }
    }
  }
}
```

Do not save only this fragment as a task override. Under **Models and sessions**,
select `codex-agents` in **Native capabilities** for the hard route. Equivalently,
add this field to its existing `routes.hard` object, preserving the other fields:

```json
{"native-profile": "codex-agents"}
```

Only packets routed there use that selection.
Inspect the saved file before the next start. Codex needs its logged-in CLI;
optional Claude/LCU peers must resolve in the worker's own dependency tree. Claude's
direct worker requires full access. See [native capabilities](native-capabilities.md).

### Enable JEV only when you have its service configured

Static model and native profiles work with JEV off. The single `jev.enabled` switch
defaults to false. To use it, open **Settings → JEV**, enter an actual System One
`endpoint`, a fixed `model` such as `jev-1.14.0`, and the credential variable name in
`key-env`, then save. The Flywheel process must receive that variable; configuration
stores its name, not the secret. A terminal export does not update a running desktop.

JEV can choose among approved evidence actions and legal failure responses,
including configured execution bindings. It cannot invent an account, weaken final
acceptance or decide Quick eligibility in place of Go. Invalid/missing prerequisites
retain the documented baseline or fail validation; saving does not test connectivity.
Use the normal Task/New/Run entries afterward. See [JEV](jev.md) for frozen policies,
data scope, budgets and recovery; enabling it is not evidence of a measured speedup.

## 7. Status, stop, resume and deliberate manual control

CLI starts consume prepared contracts; they do not generate Spec from a goal or
create the outer task branch. Use the original integration checkout and its task
directory together. Replace both paths with the locations reported by preparation:

```sh
repo_dir="/absolute/task-worktree"
task_dir="$repo_dir/docs/fix-123"
```

For the first background launch of an authorized, prepared task:

```sh
flywheel control launch --cwd "$repo_dir" --task-dir "$task_dir" --mode auto
```

Choose the operation you need; this table is not a script to run sequentially:

| Operation | CLI command |
| --- | --- |
| Read status | `flywheel control status --cwd "$repo_dir" --task-dir "$task_dir"` |
| Stop and inspect the receipt | `flywheel control stop --cwd "$repo_dir" --task-dir "$task_dir"` |
| Resume after resolving/reconciling the blocker | `flywheel control start --cwd "$repo_dir" --task-dir "$task_dir"` |

`ok:true` acknowledges the operation. `running` describes the owner;
`state.status` describes implementation progress. An investigation can have only
`preparation`. A green stage does not mean whole-task completion.

Omitting mode preserves the saved strategy. Closing a view does not stop the engine.
Do not copy/reset state, delete receipts or switch to fresh history to bypass an
unknown invocation. Follow [session recovery](session-recovery.md).

For foreground/manual operations, first `cd "$repo_dir"`. To stop before Review,
use this **instead of** background start and keep the process attached until it pauses:

```sh
flywheel run "$task_dir" --pause-before review
```

`control start/launch` do not accept pause; `--pause-before` cannot combine with `--only`.

| Intention | CLI command |
| --- | --- |
| Full Plan only | `flywheel run "$task_dir" --only plan` |
| One Execute/L1 loop | `flywheel run "$task_dir" --only execute --packet M01-T01` |
| Deterministic verification | `flywheel run "$task_dir" --only verify --packet M01-T01` |
| Independent Review | `flywheel run "$task_dir" --only review --packet M01-T01` |

Use actual packet IDs; `--unit` is an alias for `--packet`. Plan-only requires Full;
an authorized upgrade can add `--mode full`. Stage successes do not replace Run/delivery.
There are no separate CLI `task`, `iterate`, `manual` or `resume` subcommands.

For an eligible, settled packet session, the console can hand control to you. The
equivalent terminal entry uses the actual role/scope key shown there:

```sh
flywheel session "$task_dir" --key execute/M01-T01
```

Each line is a prompt. `/history` reads the latest response; `/reconcile` checks an
uncertain invocation. `/return` revalidates L1, applicable Quick gates and independent
Review, then returns control with the task still paused; resume Run for delivery.
`/release` returns control without claiming verification and stays paused; `/quit`
retains the handoff. This does not import an arbitrary unrelated patch. See
[managed session interaction](flow-console.md#controlled-provider-session-interaction).

To associate a standalone CLI task with DSH, ask the assistant in the intended
conversation to call the **model tool `flywheel_control`** with these actual paths:

```json
{
  "action": "status",
  "taskDir": "/absolute/task-worktree/docs/fix-123",
  "cwd": "/absolute/task-worktree"
}
```

This read-only tool query establishes the association. Pasting a `/flywheel` JSON
slash command or merely writing task files does not establish that initial link.
If the model tool is unavailable, retain the paths and report the missing association
rather than launching another task. See the [console guide](flow-console.md).

## 8. Troubleshooting and the next action

| What you see | Next action |
| --- | --- |
| Authentication/quota failure | Check the binding's actual SDK provider or CLI login and account; do not silently change to another paid channel. |
| Terminal works, desktop fails | Check the managed launcher, `command`/PATH, SDK profile and environment of the desktop process; follow setup repair in [Getting started](getting-started.md). |
| A configuration change has no effect | Inspect with the actual cwd/task-dir; check higher-priority complete files and restart the engine when required. |
| `awaiting-report` | Ask the host to submit the evidence-grounded report; do not repeatedly start collection. |
| Unknown dispatch, result or cleanup | Preserve the original task and reconcile its trusted receipt; process exit alone is not a known result. |
| `budget-stopped` or unknown usage | Inspect actual accounting and the intended policy; new directories, fresh sessions and model switches do not reset the allowance. |
| Acceptance, Review or Git delivery fails | Read the current evidence, preserve worktrees, resolve the actual defect/conflict and resume; do not weaken frozen acceptance or force-reset work. |
| Skills absent or customized-file conflict | Inspect `skills list/read`, project discovery and the conflicting snapshot; retain custom work. |
| Flywheel view is blank | Use an existing conversation and a genuine model-tool task association; check plugin availability without starting a duplicate run. |

For deeper procedures, use [Workflow](workflow.md), [Configuration](configuration.md),
[the console](flow-console.md), [Investigation](investigation.md),
[native capabilities](native-capabilities.md), [JEV](jev.md), and
[Recovery](session-recovery.md). Keep the original task identity when following them.
