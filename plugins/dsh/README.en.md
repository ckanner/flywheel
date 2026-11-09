# Flywheel for DSH

[简体中文](README.md) · [Project overview](https://github.com/ckanner/flywheel/blob/main/README.en.md) · [Getting started](https://github.com/ckanner/flywheel/blob/main/docs/en/getting-started.md)

This package connects the DSH desktop to Flywheel's deterministic Go engine and
provides its DSH SDK and Codex Pro workers. The engine owns task state, budgets,
verification, Git delivery, and process lifecycles. The desktop is a control and
observation interface; it does not need a parent model session to keep a task alive.

This guide describes the **Flywheel 0.28.5** interfaces. Source availability does
not mean that npm `latest` already contains them. Check the registry version and
the [release guide](https://github.com/ckanner/flywheel/blob/main/docs/en/releasing.md) before installing. The checked desktop runtimes are **DSH Mac 0.2.1-rc.5 and 0.2.2-rc.1**. The 0.28.4 engine range declares only those two versions. DSH 0.2.2 itself requires Node 24.9 or later; use its desktop-supplied CLI. Other builds and provider adapters need their own checks.

**Flywheel 0.28.1 introduces the native conversation view.** Version `0.28.0`
uses the composer popup. Upgrade the CLI and desktop plugin to the same new
version; merging source does not update an installed plugin.

Subscription preset migration and `permissions.mode` are available from **0.28.2**. Upgrades preserve existing configuration; only an explicit migration backs it up and updates model bindings and permissions.

**Changes in 0.28.3:** account defaults, account-error handling, and the current session directory. Version 0.28.2 still uses API-key defaults and the original directory. Reinstalling that version does not include these fixes. Upgrades preserve configuration; adopting the account preset requires explicit migration.

Version 0.28.4 aligns Cordis 4.0/4.1 and host peer declarations, testing the same built plugin against both published renderers with locked dependencies. Configuration, account selection and permissions are preserved. The SDK writes permission events during the public agent/created phase, after the read-only Session creation announcement and before model work; it reads the explicit never override through public `approval.overrideOf(session)`.

The package also ships an optional Claude Code Pro worker that calls the companion 0.4.0 public machine API without a second parent-model delegation. Unselected configurations do not require the optional peer. See [integration and release prerequisites](../../docs/en/claude-code-pro.md); default loops remain unchanged.

## Components

| Component | Responsibility |
|---|---|
| Flywheel conversation view | A native view alongside Chat and Trajectory for task progress, intervention, configuration, checks, and explicit start/refresh/stop |
| `/flywheel` command | Invoke the Go control interface from an explicit desktop command |
| `flywheel_control` tool | Expose the same control operations to the host model |
| `flywheel-dsh-worker` | Run one native model session through an independent DSH SDK profile |
| `flywheel-codex-pro-worker` | Call the public machine API of the pinned `dsh-subagent-codex-pro@0.9.0` package |

The plugin has no separate task database and does not write `state.json`.
Kimi CLI, Codex Pro, and the explicit legacy Codex CLI path have separate Go
drivers. A DSH LLM provider and an external agent tool are different interfaces.

The view answers which stage is running, what needs attention, which models are
configured, and how to stop or recover the task. Requirement discussion, contract
preparation, and ordinary conversation stay in DSH's chat and skills. Flywheel
does not replace the host's composer, model selector, trajectory, or right sidebar.

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
platform binary and workers, and that version's embedded skills (eleven in current source). It creates a desktop
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

The native worker uses `sdk` by default. Its standard base already includes the
DeepSeek account service and both account/API-key adapters. With the same
`DSH_HOME`, standard desktop and SDK profiles share the account credential store;
the account route needs no additional API key or token copy. Check custom homes,
credential stores, and plugin sets separately. `FLYWHEEL_DSH_PROFILE` can select
another SDK profile, but cannot select the reserved `desktop` profile.

Current source explicitly selects `deepseek-account` and `deepseek-flash`.
`deepseek-official` is the separate API-key provider and remains an explicit
alternative. Desktop default-model selection does not control an independent
SDK session. The default preset has no cross-provider fallback for authentication or quota failures; explicit user-defined fallback chains keep their existing policy.
At the checked rc.5 baseline Flash accepts `off`, `low`, `high`, and `max`,
defaulting to high; it accepts neither medium nor xhigh. Other providers have
their own values; see [Reasoning effort](https://github.com/ckanner/flywheel/blob/main/docs/en/model-policy.md).
For native GPT routes, install and authorize the selected ChatGPT adapter in
the SDK profile. One checked compatibility example is:

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

From 0.28.5, the external Flywheel view follows the built-in Chat, Trajectory and Statistics views. The host remains responsible for rendering and overflow navigation.


The normal entry is `/flywheel-task <goal>` in DSH's composer, defaulting to Auto.
The assistant prepares contracts, researched facts, and checks in the actual task
worktree, then calls `flywheel_launch` to start and associate that concrete task
with the current session's view. No directory fields need filling first. Use
`/flywheel-new` for a proposal without starting and `/flywheel-run` for an existing
task. Preserve prior authorization and any explicit request to review a proposal
first. Preparation uses the current conversation model; runtime stages use their
configured bindings. Workflow mode does not switch models, effort, channels, or permissions.

The unified entry and Auto/Quick/Full require a matching CLI/plugin release containing
these changes. Check `flywheel control launch --help`; source merges do not update
an already installed npm package.

To inspect or control an existing task, choose **Flywheel** beside Chat and
Trajectory, or in that view bar's More menu when necessary.

| Field | Purpose | When to fill it |
|---|---|---|
| Task files directory | Prepared Spec, packets, acceptance description, and task records in `docs/<slug>/`, e.g. `/Users/me/project/docs/fix-login` | Required for panel start/refresh/stop; optional for configuration and diagnostics. Use the path reported by the preparation Skill. |
| Project directory (optional) | Source project or task integration worktree, e.g. `/Users/me/project` | Current source defaults to the session's effective working directory; override for another project/worktree. |

From 0.28.3, Flywheel reads the host's `workingDirectory.get(session)`, following
directory changes. An explicit project path wins. Getter failures stop before
dispatch without falling back to the original `header.cwd`. Version 0.28.2
still uses the original directory, so explicitly fill the project path after
switching worktrees. DSH tool launch/start now associates the exact task. Preparing
files alone or starting in a terminal does not create a desktop session association;
manual opening and refresh remain available. These are project paths, not plugin
installation paths. The direct `/flywheel {…}` command is distinct from skill entries.

The panel, `/flywheel`, `flywheel_control`, and `flywheel_launch` share these Go operations:

```sh
flywheel control launch --cwd "$PWD" --task-dir "$PWD/docs/my-task" --mode auto
flywheel control start --cwd "$PWD" --task-dir "$PWD/docs/my-task"
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/my-task"
flywheel control stop --cwd "$PWD" --task-dir "$PWD/docs/my-task"
flywheel control config --cwd "$PWD"
flywheel control doctor --cwd "$PWD" --task-dir "$PWD/docs/my-task" --probe
```

`flywheel_launch` accepts `taskDir`, optional `cwd`, and optional `mode` (`auto|quick|full`,
default `auto`). `flywheel_control` also accepts action `launch`; only launch/start
accept mode, and only doctor accepts probe. The manual equivalent is
`/flywheel {"action":"launch","taskDir":"/repo/docs/my-task","cwd":"/repo","mode":"auto"}`.
If neither the session header nor a directory-change event records the project directory, launch/start/stop require an explicit absolute `cwd` and otherwise return `not-dispatched` before starting Go; read-only status can still use the host's current-directory getter.
Input is validated and passed as argv, not a shell string.
Launch consumes prepared contracts and defaults to Auto; start without mode retains
the contract/persisted choice, with legacy tasks staying Full. The host prepares
requirements and the outer task worktree; the CLI does not generate them from a goal.

Starting a real task calls the selected models. Launch/start create a background
engine; closing the panel or desktop is not a stop request. Refresh status to
observe it, and use `stop` to request shutdown. `ok: true` describes a successful
control operation; task completion is recorded in `state.status`.

Opening the view starts no task or probe. A confirmed association triggers one
fresh status query; subsequent refresh is explicit, without polling or directory scanning.
The view displays requested/effective policies, reasons, Plan/Judge applicability,
and upgrade history. Not-applicable stages are not shown as passing checks.
To observe a task started in a terminal, select its task directory and refresh. Configuration comes
from the engine's selected file; installing the plugin or changing DSH's chat
model does not migrate that configuration.

**Check installation and configuration** performs local checks without starting
the SDK. **Check DSH connection** additionally starts a temporary SDK process,
checks its configuration, directory and permissions, and closes it. Use these
when installing, upgrading or troubleshooting; they are not required before
every task. Neither sends an inference prompt or proves account access, quota
or actual model response capability.

### View, request, and task lifetimes

The public `sessionProjections` service folds the session's existing command,
tool, and PTC call/result records; the client reads `useProjection('flywheelTask')`.
The exact association can be reconstructed when that session is reloaded while
its records remain available. A fork does not inherit the parent's association.
This adds no fictional worker events or second engine state machine; restored
associations still query Go for current facts.

Manual path drafts and fetched snapshots remain session view caches, without a
durability guarantee across plugin reload. Only one control request is pending
per session. Start/stop locks directory fields. Read-only requests allow edits,
but reject late replies for an old selection. A pending or unconfirmed launch
retains its original paths and blocks repeat mutations until status of that task
succeeds. Hiding a view does not complete a pending operation.

Task state is the last successful snapshot with its refresh time, not a live
monitor. Failed reads retain valid prior content and show the failure. A lost,
timed-out, or disconnected start/stop response means the outcome is **unconfirmed**;
it does not prove that nothing started or that a task stopped. Refresh the same
task first and inspect its log when needed before deciding how to recover.
Until a successful status refresh reconciles that task, the view keeps the
uncertainty visible and disables further starts and stops. It never automatically
retries a start or stop.

A missing directory or a control process that cannot be created is recoverable
after correcting the input or environment only when the host supplies structured
evidence that the request was not dispatched. An ordinary error, connection loss,
or error-message text cannot establish that fact or dismiss an unconfirmed outcome.
The `/flywheel` command's error `text` uses the JSON discriminator
`type:"flywheel-control-error"`; its fields and compatibility behavior are defined
in the [command failure protocol](PROTOCOL.md#dsh-命令失败与是否派发) (Chinese).

Switching views, closing the session page, removing the plugin, or exiting DSH
does not stop the independent Go task. Request an explicit stop and await engine
confirmation. DSH's trajectory retains its own parent-session command and tool
events. Complete SDK, Kimi, and Codex Pro worker histories are not automatically
merged into that trajectory or presented as native DSH background tasks.

### Host UI contract

The client uses `ctx.slots.inject()` and a distinct `id: flywheel` in the
`conversation.view` list. The host owns session scope and view navigation. Chat,
Trajectory, the composer, and the right sidebar keep their existing entries;
Flywheel neither shadows them nor draws a separate floating sidebar or composer
popup. Long paths, model details, and errors wrap or scroll inside the view.

As an external plain-JS plugin, the browser module imports only shared React at
runtime. It does not import `ui-primitives`, `client-store`, or another Harness
Client package. The plugin owns its buttons, inputs, viewing-state store handle,
and observable; the host manages registered session instances and binds their
read hooks. Locale comes through the injected service. Controls follow the
checked host markup and interactions, consume semantic color aliases, inherit
the host font, and mount compiled CSS Modules through the React view.

The baseline is the exact source snapshot `b830ec950faba0a46dda564be3e2d854afed94f1`,
which declares version `0.2.1-rc.5`: [external plugin practices](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/preset/agent-preset/skills/cordis-plugin-development/references/practices.md),
[UI guidance](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/.agents/skills/dsh-client-ui-ux/SKILL.md),
[layout](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/client/ui-layout/README.md),
[conversation slots](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/client/ui-conversation/src/client/contract/slots.ts),
and [slot lifecycle](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/docs/subsystems/slots.md).
The view sits within Conversation's existing header and content geometry; it
does not change global window-chrome clearance. Source-contract inspection does
not establish a live Mac desktop pass.

Offline UI checks execute the built plugin's own controls, store handle, and
observable with React and jsdom. Component fixtures adapt slot bindings and
Remote; a separate check uses the published rc.5 renderer, slots, and Cordis to
exercise registration, session generations, declaration collapse/restoration,
and disposal. These checks do not establish real browser geometry or Mac
window behavior.

Use one task integration worktree for preparation, planning, milestone
verification, and the eventual PR. The engine manages packet worktrees for
execution, L1, and review. A successful packet is merged into the branch of the
checkout from which the engine was started. Ordinary run does not push or publish. Explicit console/CLI delivery operations can create and merge PRs after candidate and authorization checks. See the
[worktree and PR workflow](https://github.com/ckanner/flywheel/blob/main/docs/en/workflow.md).

## Role history and invocation safety records

`sessions.mode=scoped` permits continuation for the same task, role, and scope. Ordinary repairs can continue Executor history, while a Reviewer only continues its own history and checks the complete current candidate again. Primary Judge and second opinion remain separate. New templates explicitly select scoped mode; old configurations default to fresh, and an explicit Plan `resident-session:false` still opts that role out. See [Session policy](../../docs/en/configuration.md#session-policy-history-reuse-and-invocation-safety).

Go journals supported main-task Codex Pro, Claude Code Pro, DSH, and Kimi calls in both fresh and scoped modes. Fresh disables general role history reuse while preserving the existing Kimi Plan resident behavior. Other adapters, mock, and Intake/Garden retain their existing paths. Unknown dispatch, outcome, or cleanup blocks further execution; creating a new session, switching modes, or scanning history cannot bypass it. `status` and `control status` expose compact role, scope, action/reason, and latest invocation facts without reading conversation history. A resume candidate is not proof of durable backend history.

DSH v2 uses public `session/resume` with the exact requested session ID and a new SDK process for each call. Restored events do not enter the current output, usage, or activity callback; directory and permission checks run again. P3 adds durable Go ACKs, independent supervisor journals, and exact read-only receipt recovery for Codex/Claude. DSH/Kimi retain their own existing capabilities. P4 adds role overrides, bounded quality rebuilds, and paired evaluation tools; real quality, cache, and performance require live evidence. See [recovery](../../docs/en/session-recovery.md).

## Permissions and lifecycle boundaries

Desktop integration and worker policy have different scopes. The worker's
temporary overlay applies only to its independent **SDK process**; it does not
edit the desktop profile or remove the desktop's plugins, skills, tools, or UI.
Conversely, desktop installation and sign-in do not prove that the SDK profile
loads and authorizes the same capabilities. Verify each selected harness through
its own configuration and supported authorization flow.

The DSH worker supports `read-only`, `workspace-write`, and explicit
`danger-full-access`, with approvals fixed to `never`. New subscription presets
select `permissions.mode:full-access` for every stage; Kimi print retains native
auto behavior. Existing files without this setting retain role-based access
until explicitly migrated. The actual SDK policy is checked before prompting.
Restricted modes use the checked Mac Seatbelt runner and filesystem fence;
workspace writes include DSH's permitted temporary area. Explicit full access
is recorded without claiming OS confinement. Host static denials, account
restrictions, and operating-system permissions remain in effect.

The SDK overlay disables worker-owned delegation, workflow/goal drivers, model
titles, automatic model compaction, and `schedule` / `tool-schedule` so auxiliary
calls or due reminders cannot independently append input to a Go-owned session.
It does not change user reminder storage. It also disables the whole `tool-web` package,
including both `web_search` and `web_fetch`. This is a conservative policy,
not a claim that every DSH web operation requires a model: the official
[web tool](https://github.com/deepseek-ai/deepseek-harness/blob/fcc0ad462172522aaf5c93a7b2e01f6b9f0b0159/packages/web/tool-web/README.md)
configures search and fetch independently, while the
[DeepSeek search backend](https://github.com/deepseek-ai/deepseek-harness/blob/fcc0ad462172522aaf5c93a7b2e01f6b9f0b0159/packages/web/web-search-deepseek/README.md)
does use an auxiliary model request. Full access changes the host access policy;
these tool and auxiliary-model restrictions still apply.
Restoring retrieval or [automatic compaction](https://github.com/deepseek-ai/deepseek-harness/blob/fcc0ad462172522aaf5c93a7b2e01f6b9f0b0159/packages/compaction/compaction-basic/README.md)
requires a separate check of accounting, budgets, cancellation, and directory
ownership; the view's location does not establish those guarantees.

Main-task Codex calls use worker v3 for both ephemeral and persistent history, retaining public machine protocol 1 and lifecycle version 1. Legacy v1/v2 callers remain supported. Go checks the complete actual sandboxPolicy at pre-dispatch thread-ready before ACK, then checks the terminal receipt again. Restricted modes reject unexpected writable roots and network access; full access explicitly records dangerFullAccess without claiming OS confinement. Legacy and advisory calls without lifecycle retain terminal policy checks. Kimi print mode likewise provides no OS sandbox this adapter can promise.

The reviewed Codex 0.9.0 source candidate is bundled as a real public npm package, with [provenance and hashes](vendor/README.md). Claude 0.4.0 remains an optional peer. Neither change installs native CLIs or copies credentials.

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
DSH's `ready.reasoningEffort` echoes the request after successful initialization.
Known account sign-in/token errors and the exact account-signed-out hook map to
AUTH; account quota errors remain QUOTA. Original diagnostics and account
recovery guidance are preserved. The default preset has no API-key fallback.
Unreported usage and dollar cost remain unknown; subscriptions are not recorded
as free calls. DSH v1 creates a fresh session; v2 supports public session resume.
Missing, busy, or generic restore failures do not silently create a replacement.
Image and attachment inputs remain unsupported. Protocol details are documented
in [PROTOCOL.md](PROTOCOL.md) (Chinese).

From 0.28.3, a started DSH worker without a trustworthy terminal receipt, including a missing result or malformed stream, records unconfirmed SDK cleanup and ends the run without automatic retry or fallback. Reaping the outer worker does not replace an SDK shutdown receipt. Process exit also does not establish a known model outcome. The Go ledger requires trusted dispatch/outcome facts and the original candidate, contract, and role guards; rerunning verification cannot rehabilitate a verdict after a forbidden mutation.

Permission proof uses a private per-invocation nonce receipt. Backend history contains only native `sandbox/mode` and `approval/policy` events, with no custom `flywheel/policy` event that public SDK history parsing would reject.

The offline persisted create → shutdown → new-process resume test uses public SDK, JSONL persistence, and AgentLoop packages at `0.2.2-rc.1`. It checks restored context and current-call output/usage only. The `0.2.1-rc.5` host/UI/plugin compatibility tests do not establish persisted-history restoration for that version. Runtime serverInfo is not treated as proof of the npm package version. Real-account inference, physical macOS validation, and performance experiments remain separate and unperformed here.

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

## Horizontal workflow console

The new view provides an expandable horizontal canvas, stage run/retry, isolated session handoff, serial or declared parallel execution, global/project configuration, run comparison, and explicitly authorized PR/Merge operations at a reviewed HEAD. Ordinary run does not acquire remote-delivery permission.

See the [workflow console guide](../../docs/en/flow-console.md) for commands, configuration and acceptance boundaries. The engine reads old state; new invocation/handoff records use ledger v2 so an old engine cannot ignore human ownership. Real account readiness remains a separate check.

Graceful SDK shutdown allows two seconds for history flushing and plugin disposal before bounded TERM/KILL cleanup. Success requires both acknowledgement and physical exit.
