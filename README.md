# Flywheel

English · [简体中文](README.zh-CN.md)

**Turn a defined coding task into a verified software change.**

Flywheel is a deterministic Go orchestrator for coding agents. Models plan,
implement, and review; the engine manages task contracts, verification,
retries, budgets, workspaces, and delivery. Use it from the DeepSeek Harness
(DSH) desktop or the command line.

[Getting started](docs/en/getting-started.md) ·
[Usage handbook](docs/en/usage-handbook.md) ·
[Documentation](docs/README.md) ·
[Contributing](CONTRIBUTING.md) ·
[MIT license](LICENSE)

## What Flywheel does

- **Chooses a workflow for the task.** Auto selects Quick for an eligible,
  well-defined task and Full when planning or broader verification is needed.
- **Verifies the actual candidate.** Deterministic checks precede independent
  review. Completion requires evidence for the same code that is committed
  and integrated.
- **Keeps roles separate.** Planning, execution, and review use separate
  contexts. Eligible repairs can resume the same role's session without
  discarding task history.
- **Coordinates bounded parallel execution.** Packets with explicit
  dependencies and non-overlapping write scopes can execute concurrently;
  review and integration remain coordinated by the engine.
- **Uses native harness capabilities.** Select PTC Agents/Teams, Kimi Swarm/Tower, Codex agents, Claude Workflows or managed LCU per role; see [native capabilities](docs/en/native-capabilities.md).
- **Provides a desktop workbench.** Inspect task progress, evidence, sessions,
  run history, and configuration from DSH's native Flywheel view.
- **Makes failures visible.** Unknown execution, usage, or cleanup stays
  unknown. The engine preserves work and stops when it cannot safely proceed.

From 0.33, `@ckanner/flywheel` is one complete package: CLI, DSH view/workers,
embedded skills and four platform engines. No separate platform package or first-run
engine download is needed. When upgrading from 0.32.x, quit DSH before running the
installer; it replaces the old desktop package registration and preserves your
configuration and histories. Reload DSH after setup. Existing old npm versions
remain available for historical installations.

## Install

For the desktop path, install Node.js **22.18.0 or later**, npm, Git, and a
supported DSH desktop with its `dsh` command available. The plugin currently
accepts the DSH **0.2 through 0.3 series**, starting at **0.2.1-rc.5** and ending before
**0.4.0** (including the tested 0.2.2/0.2.3 and 0.3.0 prerelease lines). Routine upgrades within
that range do not require a Flywheel release. This is a best-effort compatibility window: required host services and
SDK checks still fail closed if a release is incompatible, and the failure is fixed in code. Prepare the accounts
and CLI tools required by the model routes you choose. DSH itself may require
a newer Node version; the DSH 0.2.2 SDK requires Node 24.9 or later.

```sh
npx -y @ckanner/flywheel@latest install
```

The installer sets up matching CLI, native binary, workers, embedded skills,
and the DSH plugin. Native packages cover macOS and Linux on arm64 and x64;
installing from npm does not require Go. Account login and provider setup
remain separate steps. Existing configuration and modified skills are
preserved.

If a user-owned npm prefix is needed:

```sh
npx -y @ckanner/flywheel@latest install --prefix "$HOME/.local"
```

Add that prefix's `bin` directory to your shell PATH to use `flywheel`
directly. The [installation guide](docs/en/getting-started.md) covers desktop
registration, SDK providers, diagnostics, upgrades, and a complete first task.

This README describes the current source. Check the published version with
`npm view @ckanner/flywheel version`; merging source does not update an
existing installation. Version changes are recorded in the [changelog](CHANGELOG.md).

## Run your first task

Open the target repository in DSH, then describe a bounded change:

```text
/flywheel-task Fix the list's empty state, add regression tests, and complete the change. mode=auto
```

The skill studies the repository and prepares a task contract under
`docs/<task>/`. Go validates the contract and owns execution from that point.
Use `/flywheel-new` to run design and independent review, then stop for exact-version approval, or
`/flywheel-run` to start or resume an existing task. In Kimi, use the
`/skill:flywheel-task` form of the command.

Open the **Flywheel** conversation view to inspect the associated task's
canvas, run history, or configuration. Task locations come from the session
and recorded runs; the view does not require manually entered paths.

For a prepared task, the equivalent CLI entry is:

```sh
flywheel control launch --cwd /path/to/repo --task-dir /path/to/repo/docs/my-task --mode auto
flywheel control status --cwd /path/to/repo --task-dir /path/to/repo/docs/my-task
```

Run these read-only configuration checks from the target repository:

```sh
flywheel config inspect --cwd "$PWD" --json
flywheel doctor --cwd "$PWD" --json
```

Doctor checks local setup; it does not establish live model access. See the
[workbench guide](docs/en/flow-console.md) for stage operations, takeover,
run comparisons, and explicit GitHub delivery actions.

## Choose a workflow

| Mode | Use it for | Execution |
| --- | --- | --- |
| **Auto** | A prepared task whose scope and checks are known | Go selects Quick when the contract meets the eligibility checks; otherwise Full. |
| **Quick** | One milestone and one bounded packet with sufficient verification | Execute → required checks → independent final Review → local delivery. New risks can upgrade it to Full. |
| **Full** | Multiple packets, design work, or a broader change | Plan → packet execution, checks, review, and local delivery → integration checks and Judge. |

Quick retains required repository checks and delivery checks. Its first-pass
success path has two stage-agent calls, excluding preparation, retries, and
the agent's internal tool calls. Existing tasks without workflow metadata
continue as Full. See [workflow and delivery](docs/en/workflow.md).

When the cause is still unknown, Task can first run a [bounded investigation](docs/en/investigation.md).
Report-only goals stop at an evidence report; authorized repairs retain the original
scope before entering Auto. Compact/system preparation, Quick/Full implementation and review-first/delegated authorization are separate choices. New and direct stage controls remain available.

## Models and configuration

The default is a subscription-only configuration using four native clients:

| Work | Default models |
| --- | --- |
| System design and difficult analysis | Gemini Carbon high / Neon high through official `agy` |
| Compact preparation, routine implementation and Intake | Gemini Hydrogen medium / high / low |
| Plan refinement and interface work | Claude Opus 5.5 high / medium through Claude Code |
| Backend implementation and independent reviews | Codex GPT-6.2 Sol; Astra for hard work and Judge |
| Batch work and Garden | Kimi K3.1 Pro high through its native agent |

The [model policy](docs/en/model-policy.md) gives exact stage, route, profile and
effort mappings. [Antigravity](docs/en/antigravity.md) is included as a bundled
machine adapter and also has an independently installable DSH plugin. Claude
requires the reviewed [0.7.1 companion](docs/en/claude-code-pro.md) in the worker's
resolution tree. Model access still depends on native login and the actual account.

The preset has no API-key or credits fallback. JEV is off by default; when enabled
it uses Gemini Hydrogen medium over the same evidence-only subscription path.
Scoped sessions, full-access permissions, automatic worktrees and a maximum of two
parallel packets are explicit defaults. Investigation analysis has a separate
read-only/evidence-only binding. Required dependencies and write scopes still
constrain parallel execution.

Upgrades preserve existing configuration. Preview `flywheel config migrate
--preset subscription-cli`, inspect its effective file and changes, then apply it
explicitly. Migration retains permissions, budgets, verification and scheduling.
See [Configuration](docs/en/configuration.md) for the complete procedure.

## Workspaces and recovery

The **project workspace** is the repository a session belongs to. The
**execution directory** is where a task or packet changes and verifies code.
They may differ. Automatic worktree management and scheduling are independent
settings; disabling worktrees does not itself disable parallelism. Shared
directory batches require checks against the final common candidate.

Session reuse is separate from call recovery. Unknown dispatch, result, or
cleanup blocks continuation even if a fresh session is requested. Backend
capabilities differ; [recovery and context quality](docs/en/session-recovery.md)
documents those boundaries. Offline evaluations establish protocol behavior,
not live model quality, cache savings, or performance.

## Documentation and development

Start with the [documentation index](docs/README.md) for paired English and
Chinese guides, the architecture specification, and historical records.

- [Configuration reference](docs/en/configuration.md)
- [DSH plugin](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.md) and [worker protocol](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md)
- [Architecture specification](docs/design.md) — maintained in Chinese
- [Contribution guide](CONTRIBUTING.md) and [release runbook](docs/en/releasing.md)

Developers need Go **1.27 or later** and the Node/npm toolchain described in
the contribution guide. The Go quality gate includes formatting, vet, and
100% statement coverage for every measured function. Package and adapter
changes also have dedicated build, bridge, and installation checks.

## License

Flywheel is licensed under the [MIT License](LICENSE).
Third-party code retains its own notices; see the
[DSH plugin notices](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/THIRD_PARTY_NOTICES.md) and
[bundled adapter provenance](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md).
