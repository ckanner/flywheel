# Flywheel for DSH

[简体中文](README.md) · [Project overview](https://github.com/ckanner/flywheel/blob/main/README.en.md) · [Getting started](https://github.com/ckanner/flywheel/blob/main/docs/en/getting-started.md)

This package connects the DSH desktop to Flywheel's deterministic Go engine and
provides its DSH SDK and Codex Pro workers. The engine owns task state, budgets,
verification, Git delivery, and process lifecycles. The desktop is a control and
observation interface; it does not need a parent model session to keep a task alive.

This guide describes the **Flywheel 0.28.0** interfaces. Source availability does
not mean that npm `latest` already contains them. Check the registry version and
the [release guide](https://github.com/ckanner/flywheel/blob/main/docs/en/releasing.md) before installing. The desktop
compatibility baseline is **DSH Mac 0.2.1-rc.5**, not a claim that every DSH build
or provider adapter is compatible.

## Components

| Component | Responsibility |
|---|---|
| Flywheel panel | Inspect configuration, run local checks, and start, observe, or stop a task |
| `/flywheel` command | Invoke the Go control interface from an explicit desktop command |
| `flywheel_control` tool | Expose the same control operations to the host model |
| `flywheel-dsh-worker` | Run one native model session through an independent DSH SDK profile |
| `flywheel-codex-pro-worker` | Call the public machine API of the pinned `dsh-subagent-codex-pro@0.8.0` package |

The plugin has no separate task database and does not write `state.json`.
Kimi CLI, Codex Pro, and the explicit legacy Codex CLI path have separate Go
drivers. A DSH LLM provider and an external agent tool are different interfaces.

## Install the complete application

You need **Node.js 22.18.0 or later**, npm, and the existing DSH Mac desktop with
its desktop-installed `dsh` command. Verify the desktop version and the command
you are actually running. Do not guess an executable path inside an application
bundle.

```sh
npm view @ckanner/flywheel version
npx -y @ckanner/flywheel@latest install
```

The explicit installer installs the resolved Flywheel version, the matching Go
platform binary and workers, and ten embedded skills. It creates a desktop
launcher, registers the same plugin version with the desktop profile, and checks
the installed versions. It uses the desktop's plugin management interface:

```text
dsh plugin --profile desktop add --save-exact --ignore-scripts @ckanner/dsh-plugin-flywheel@<resolved-version>
```

The standalone npm DSH CLI cannot manage the reserved `desktop` profile. If the
desktop command is not on PATH, pass `--dsh-bin /absolute/path/to/dsh` to the
Flywheel installer. Reload or restart as instructed by the host.

Use `--prefix "$HOME/.local"` if you need a writable installation prefix, and add
its `bin` directory to your shell PATH. `--no-dsh` explicitly skips desktop
registration. Go CLI packages are provided for macOS and Linux, on arm64 and
x64; Linux package availability does not establish support for the Mac desktop
or its sandbox. See the [installation guide](https://github.com/ckanner/flywheel/blob/main/docs/en/getting-started.md)
for the full options and supported workflow.

Software installation and model readiness are separate. With `--json`,
`ok: true` means installation succeeded; `ready: false` can still report missing
Kimi or Codex executables or other local prerequisites. Authentication and actual
inference remain unverified. Flywheel does not install or update DSH, sign in to
model services, copy credentials, or provide subscription access.

Ordinary `npm install --global @ckanner/flywheel` only installs package files.
It has no postinstall hook that changes your profile or skills. To finish setup
for an existing persistent installation, run:

```sh
flywheel setup --no-global
```

Installing `@ckanner/dsh-plugin-flywheel` alone gives you this plugin and its
workers, not the complete Flywheel application.

## Prepare the SDK profile and selected models

The native DSH worker uses `sdk` by default. A provider installed or authorized
in the desktop profile is not automatically present in the SDK profile.
`FLYWHEEL_DSH_PROFILE` can select another SDK profile, but cannot select
`desktop`. Keep `DSH_HOME` consistent with the configuration you intend to use.

The native DeepSeek provider is `deepseek-official`, with `deepseek-flash` in
the default configuration. At the checked rc.5 baseline its effort values are
`off`, `low`, `high`, and `max`; there is no `medium`. Other providers have their
own accepted values. For native GPT routes, install and authorize the selected
ChatGPT adapter in the SDK profile. One checked compatibility example is:

```sh
dsh plugin --profile sdk add dsh-plugin-chatgpt@0.1.17
```

This example does not migrate desktop credentials or prove that your account
can use every model in Flywheel's configuration. Confirm the exact model IDs
and effort values against the selected provider. The
[configuration guide](https://github.com/ckanner/flywheel/blob/main/docs/en/configuration.md) explains profiles,
provider IDs, account channels, and existing-configuration migration.

The default `hard` route uses `harness: "codex-pro"`. Its worker directly imports
`dsh-subagent-codex-pro/machine`; the package runs Codex app-server using your
existing Codex installation and login. It does not ask a DSH parent model to
delegate again. `subagent_codex_pro` is not an LLM provider ID, and this route does
not silently fall back to `codex exec`.

| Setting | Meaning |
|---|---|
| `FLYWHEEL_DSH_BIN` | Explicit DSH executable; otherwise use the configured launcher environment or PATH |
| `FLYWHEEL_DSH_PROFILE` | Independent SDK profile, default `sdk` |
| `DSH_HOME` | Optional DSH configuration root |
| `FLYWHEEL_DSH_WORKER` | Explicit DSH worker override; the npm launcher normally resolves the matching dependency |
| `FLYWHEEL_CODEX_PRO_WORKER` | Explicit Codex Pro worker override; normally resolved by the npm launcher |
| `FLYWHEEL_CODEX_BIN` | Codex executable used by the plugin, default `codex` |
| `CODEX_HOME` | Optional existing Codex configuration and login directory |
| `FLYWHEEL_KIMI_BIN` | Kimi executable for routes that use its CLI, default `kimi` |

The selected executables and configuration must be visible to the process that
starts the engine. A shell export does not update an already running desktop.

## Check the installation without inference

Run these from the task's integration worktree, after preparing and reviewing
its contracts. Replace the task name with your own:

```sh
flywheel config inspect --cwd "$PWD" --task-dir "$PWD/docs/my-task" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-task" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-task" --probe --json
```

Omit `--task-dir` to inspect the general configuration. Ordinary doctor checks
local configuration and commands. `--probe` additionally initializes the DSH
SDK, checks its working directory and permission enforcement, and closes the
session. Provider initialization may query a model catalog or create SDK
session/cache files, but it does not send an inference prompt or run task
acceptance commands. It does not write Flywheel state or create worktrees.

For Codex Pro, doctor imports the pinned machine API and checks local prerequisites;
it does not start an inference task. The worker's narrower package check is:

```sh
flywheel-codex-pro-worker --doctor
```

Its successful response reports `available: true` and `inferenceTested: false`.
Neither check proves account authorization, quota, model availability, or an
actual inference result. Keep `warn`, `unknown`, and `skipped` distinct from
`pass`. A blocking diagnostic returns exit code 1 with the report intact.

## Use the desktop controls

Open the **Flywheel** panel below the DSH session input. Select the absolute
task worktree path as the repository directory and the task directory inside
that same worktree. Prepare and review the contracts first; the panel does not
turn an empty directory into a complete task plan.

The panel, `/flywheel`, and `flywheel_control` share these Go operations:

```sh
flywheel control start --cwd "$PWD" --task-dir "$PWD/docs/my-task"
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/my-task"
flywheel control stop --cwd "$PWD" --task-dir "$PWD/docs/my-task"
flywheel control config --cwd "$PWD"
flywheel control doctor --cwd "$PWD" --task-dir "$PWD/docs/my-task" --probe
```

Starting a real task calls the selected models. `start` launches a background
engine; closing the panel or desktop is not a stop request. Refresh status to
observe it, and use `stop` to request shutdown. `ok: true` describes a successful
control operation; task completion is recorded in `state.status`.

Use one task integration worktree for preparation, planning, milestone
verification, and the eventual PR. The engine manages packet worktrees for
execution, L1, and review. A successful packet is merged into the branch of the
checkout from which the engine was started. The engine does not push, open
GitHub PRs, merge remote PRs, or publish packages. See the
[worktree and PR workflow](https://github.com/ckanner/flywheel/blob/main/docs/en/workflow.md).

## Permissions and lifecycle boundaries

The DSH worker requests `read-only` or `workspace-write`, fixes approvals to
`never`, and verifies the effective SDK policy before sending the task prompt.
On the checked Mac host, the DSH sandbox uses its Seatbelt runner and filesystem
fence. Workspace writes include DSH's permitted temporary area; this is not a
network-isolation guarantee. Missing or incomplete enforcement is a failure,
not a reason to switch to unrestricted access.

Codex Pro uses fresh ephemeral calls and checks the model, directory, approval
policy, and sandbox mode before execution. In version 0.8.0, the full
`sandboxPolicy` reaches Flywheel only in the returned receipt. Flywheel rejects
delivery when that policy permits unexpected writable roots or network access,
but **this check happens after execution and cannot undo access already
permitted by custom Codex configuration**. It is not a complete pre-execution
policy guard. Kimi CLI print mode likewise does not provide an OS sandbox that
this adapter can promise.

A worktree separates Git working files and index state; it is not a security
sandbox. The current engine can fall back to `in-place` if packet worktree
creation fails. If the task requires strict workspace separation, stop and
resolve that condition before continuing. Restarting alone does not convert
saved `in-place` state into a new isolated worktree.

Go owns worker shutdown; Codex Pro also owns a separate guardian/app-server
lifecycle. Text output or the outer worker exiting does not prove that every
owned process has stopped. When cleanup cannot be confirmed, Flywheel preserves
the workspace and audit evidence and stops automatic retries, fallback, cold
restarts, and later work in that run. Confirm the owned processes have ended
before recovery. A successful stop operation is not proof that an arbitrary
escaped process has been cleaned up.

Complete receipts preserve configuration, usage, and termination evidence.
Accepted effort is not proof of the server's effective reasoning effort.
Unreported usage and dollar cost remain unknown; subscriptions are not recorded
as free calls. The current DSH bridge does not support session resume or image
and attachment inputs. Its protocol and implementation details are documented
in [PROTOCOL.md](PROTOCOL.md) (Chinese).

## Upgrade and maintain the desktop entry

The installer records an absolute Node/CLI launcher under
`~/.config/flywheel/`, including its content hash and the checked tool paths.
An explicit plugin `flywheelBinary` setting takes precedence. Without an
explicit setting, the plugin uses a valid installation record; only a missing
record permits legacy PATH lookup. A damaged record or changed launcher fails
visibly instead of selecting a different engine.

After upgrading Flywheel, moving the npm prefix, changing Node, or adding a
required CLI, run `flywheel setup --no-global` from a terminal where those tools
are available. Retain your chosen prefix and DSH executable options. Reload the
desktop as needed and run its checks again. Custom configuration and modified
skills are preserved and reported as conflicts; save your changes and resolve
the conflict rather than deleting their ownership records.

Existing Flywheel configuration is not automatically migrated. To adopt the new
plugin path from 0.27.x, change the effective hard-route harness from `codex` to
`codex-pro`, including any referenced profile, and inspect the expanded result.
The full upgrade and removal procedure is in
[Getting started](https://github.com/ckanner/flywheel/blob/main/docs/en/getting-started.md).

For source changes and offline verification, read the
[contribution guide](https://github.com/ckanner/flywheel/blob/main/docs/contributing.md). The repository currently uses
`UNLICENSED`; an npm release or a future public repository does not by itself
establish an open-source license.
