# Flywheel for DSH

English · [简体中文](README.zh-CN.md) · [Project overview](https://github.com/ckanner/flywheel/blob/main/README.md) · [Documentation](https://github.com/ckanner/flywheel/blob/main/docs/README.md)

`@ckanner/flywheel` connects DeepSeek Harness (DSH) to Flywheel's
Go orchestration engine. It adds a native workflow view, command and model-tool
controls, and the workers used by selected backends. The engine owns task state,
verification, budgets, process lifecycles, and Git delivery.

Use the complete Flywheel installer for normal setup. The published
`@ckanner/flywheel` package includes the CLI, engine, workers, skills and desktop
plugin. Plain npm installation places those files; `install`/`setup` registers
the desktop plugin and configures the launcher.

## Install

Requirements are **Node.js 22.18.0 or later**, npm, and a compatible DSH desktop
with its desktop-supplied `dsh` command. This package declares DSH
**`>=0.2.1-rc.5 <0.4.0-0 || ^0.2.2-0 || ^0.2.3-0 || ^0.3.0-0`** and the corresponding Cordis 4.0/4.1 interfaces.
DSH includes prereleases when checking this range: later 0.2 rc/patch releases do
not need an exact-version exemption or a new Flywheel release. The next minor, 0.4 including its prereleases, remains outside this contract.
The selected host may have a higher Node requirement; use its supported runtime.

The range is an API compatibility policy, not evidence that every future host
has been tested. Locked integration fixtures cover the legacy 0.2.1-rc.5 host
and the current 0.2.2-rc.3 host/SDK. Required services, session identity,
permission receipts and process cleanup remain checked at runtime. A missing
service or invalid receipt still fails visibly before unconfirmed work is accepted.
DSH peers stay optional and are supplied by the host; their npm ranges also
include the two existing prerelease lines, without pinning individual rc releases.

```sh
npx -y @ckanner/flywheel@latest install
```

The installer installs one matching version of the CLI, platform binary,
workers, embedded skills, and desktop plugin. It registers the plugin through
the desktop's plugin management interface and creates a managed launcher.
Reload or restart DSH when requested by the host. Go packages support macOS and
Linux on arm64 and x64; Linux package availability does not establish Mac
desktop or Seatbelt behavior.

| Option or command | Use |
|---|---|
| `--dsh-bin /absolute/path/to/dsh` | Use the desktop CLI when it is not on PATH |
| `--prefix "$HOME/.local"` | Select a writable npm prefix; add its `bin` directory to PATH |
| `--no-dsh` | Explicitly skip desktop registration |
| `flywheel setup --no-global` | Finish or refresh setup for an existing persistent installation |
| `--json` | Return the installation report for automation |

The standalone npm DSH CLI cannot manage the reserved `desktop` profile.
Plain `npm install --global @ckanner/flywheel` installs package files only;
there is no postinstall hook that changes a profile, skills, or credentials.
In an installation report, `ok: true` means installation succeeded;
`ready: false` can still identify missing native CLIs or local prerequisites.
See [Getting started](https://github.com/ckanner/flywheel/blob/main/docs/en/getting-started.md) for setup and removal.

## Prepare the selected backends

The default uses native subscription clients; the DSH view is the control plane.
Installing a desktop plugin does not change the parent conversation's provider.

| Backend | Setup and execution path |
| --- | --- |
| Antigravity | Bundled `@ckanner/dsh-subagent-antigravity@0.3.1` machine API → official installed `agy` CLI (`transport: "acp"` default, native images, isolated `evidence-only`) |
| Claude Code Pro | Exact optional peer `0.7.1` in the worker dependency tree → official Agent SDK → local Claude Code subscription |
| Codex Pro | Bundled `0.10.2` machine API → official app-server with ChatGPT login |
| Kimi | Minimal owned native agent profile with managed Kimi Code OAuth (`WebSearch`/`FetchURL`/`Agent`/`AgentSwarm` enabled by default); strict subscription mode rejects print |

The strict preset checks native routing and supported subscription evidence before
admission; unknown quota remains unknown. Auth/quota failures never enable API keys,
credits or extra-usage fallback. See [model policy](https://github.com/ckanner/flywheel/blob/main/docs/en/model-policy.md),
[Antigravity](https://github.com/ckanner/flywheel/blob/main/docs/en/antigravity.md), and
[Claude](https://github.com/ckanner/flywheel/blob/main/docs/en/claude-code-pro.md).

Existing non-strict configurations can still explicitly use a DSH `sdk` profile,
DeepSeek account/API providers or DSH ChatGPT. They are not reachable alternatives
inside `subscription-only`. SDK and Desktop profiles are separate; native CLI
login remains the user's supported flow. Flywheel does not copy credentials or
install model-client executables.

### Executable and environment overrides

| Variable | Meaning |
|---|---|
| `FLYWHEEL_DSH_BIN` | DSH executable used by the worker |
| `FLYWHEEL_DSH_PROFILE` | Independent SDK profile; default `sdk` |
| `DSH_HOME` | DSH configuration, account, and session root |
| `FLYWHEEL_DSH_WORKER` | Explicit DSH worker override |
| `FLYWHEEL_CODEX_PRO_WORKER` | Explicit Codex Pro worker override |
| `FLYWHEEL_CLAUDE_CODE_PRO_WORKER` | Explicit optional Claude worker override |
| `FLYWHEEL_CODEX_BIN` / `CODEX_HOME` | Native Codex executable and existing configuration/login root |
| `FLYWHEEL_CLAUDE_BIN` / `CLAUDE_CONFIG_DIR` | Native Claude executable and existing configuration/login root |
| `FLYWHEEL_AGY_BIN` / `FLYWHEEL_ANTIGRAVITY_WORKER` | Official agy executable / explicit Antigravity worker override |
| `FLYWHEEL_KIMI_BIN` | Native Kimi executable; default `kimi` |

The npm launcher normally resolves workers from its own installed package.
Executables and environment settings must be visible to the process that starts
the engine; a shell export does not update an already running desktop.

## Start a task and use the workbench

In DSH's composer, enter `/flywheel-task <goal>`. The host records facts and a
thin brief, then uses `flywheel_control planning`. Go invokes the selected design
and independent review roles, using compact or system preparation. Review-first
stops for exact-version user approval; delegated work continues under existing
implementation authority. `/flywheel-new` defaults to review-first; `/flywheel-run`
continues the original task and ledger.

Open **Flywheel** alongside the built-in conversation views, or in their overflow
menu. The workbench has three pages:

| Page | What it provides |
|---|---|
| Workflow canvas | Current session task, current stage, evidence, stage run/retry, role-session history, and managed intervention |
| Run history | Registered runs, known usage/cost, missing metrics, and comparisons with their conditions |
| Configuration | Automatically loaded global/project settings, grouped editing, source display, and protected unsaved drafts |

The view automatically follows the current conversation's explicit task association,
then its latest registered host-session run. Multiple associated tasks appear in a conversation-only navigator; a single task opens directly. There is no cross-session selector or path form;
foreign and ownerless runs stay out of this view. Use `flywheel_control status` in the
intended conversation to associate a prepared task. Locations and action groups are
shown directly. A successful single-stage Review is not whole-task completion.

Automatic refresh and other reads use a session-scoped read-only Remote and do not
append JSON command records to chat. Explicit mutations and unknown-result recovery
keep durable command receipts. Task titles and IDs are distinct, with project/task paths in the header. Settings has continuous navigation and fixed save controls; stage assignments edit the same draft with one Save changes action, inline role-session overrides and an optional usage budget. Environment checks live in Diagnostics and config; raw diagnostic responses do not fill the normal workflow view.

Preparation-only tasks display a bounded investigation panel with observations,
budgets, original-receipt reconciliation, Stop, and the evidence report. Approved
diagnostic commands and HTTP queries show exit/status codes, cancellation and copy
cleanup. Commands use explicitly granted host-process authority in copied inputs;
the copy is not an OS sandbox. A report does not imply code delivery. See the
[investigation guide](https://github.com/ckanner/flywheel/blob/main/docs/en/investigation.md).

Under **Settings → JEV**, **Enable JEV** is the only JEV switch and is off by
default. Choose the global or project scope, turn it on to reveal the connection
fields, and fill or confirm the System One endpoint, pinned model and API-key
environment variable name. Select **Save changes** to apply the setting. The switch
edits a draft; editing and saving do not test service connectivity. When disabled,
existing execution paths run without JEV requests or shadow capture. New
investigations prepared through Task or New inherit the policy automatically;
Run continues that frozen policy. Enabled JEV can choose the next approved evidence action, defer
redundant actions on cited evidence, or end evidence collection early for a report.
During implementation it can choose a legal next step after confirmed failure:
local repair, an approved feedback check, bounded replanning or an allowed execution
binding. Investigation and implementation share the task's call allowance. A
feedback check never removes complete final acceptance.

Turning JEV off also blocks subsequent JEV calls and adoption in an existing
task, while retaining its receipts and budget. Turning it on does not
rewrite a preparation created while disabled. The panel distinguishes configured
JEV settings from the investigation's recorded status, calls, adopted actions and
evidence-backed effects. Task and node details distinguish applied control decisions,
settled feedback, unresolved actions and the original baseline. They show actual
feedback results and before/after model bindings with the switch reason; applying a
configuration does not claim a new session has already run or final acceptance passed.
Advanced request limits, confidence thresholds and data categories share the same
switch; there is no separate shadow or adoption toggle.
Legacy shadow configuration does not automatically opt into live decisions. See
[JEV decisions](https://github.com/ckanner/flywheel/blob/main/docs/en/jev.md).


Visible canvases refresh task discovery; running tasks refresh status. Opening
the view starts no task or diagnostic probe. Failed reads retain the previous
confirmed snapshot and its timestamp. A missing or timed-out launch/start/stop
reply is **unconfirmed**, so further mutations stay blocked until status for
that same task reconciles it. The view never automatically retries a mutation.
Hiding the view, closing DSH, or removing the plugin does not stop the independent
Go task; request Stop and await engine confirmation.

### Commands and model tools

| Entry | Scope |
|---|---|
| `/flywheel` | Human command accepting a JSON control request |
| `flywheel_launch` | Model tool to launch a prepared task, Auto by default |
| `flywheel_control` | Model tool for launch/start/status/stop/investigate/config/doctor |
| Workbench controls | Additional revision-bound stages, configuration editing, managed handoff, and explicit delivery operations |

A manual launch example is:

```text
/flywheel {"action":"launch","taskDir":"/repo/docs/my-task","cwd":"/repo","mode":"auto"}
```

Equivalent CLI operations are:

```sh
flywheel control launch --cwd /repo --task-dir /repo/docs/my-task --mode auto
flywheel control status --cwd /repo --task-dir /repo/docs/my-task
flywheel control stop --cwd /repo --task-dir /repo/docs/my-task
flywheel control start --cwd /repo --task-dir /repo/docs/my-task
```

Paths above are explicit command arguments, not UI fields. An omitted `cwd`
uses the session's public current-directory service. Launch/start/stop require
an explicit absolute `cwd` if the session has no recorded project directory.
Input is validated and passed as argv, not interpolated into a shell command.
`ok: true` reports the control operation; completion comes from task state.

The host prepares the outer task worktree. The engine independently manages
packet scheduling and execution directories. New templates enable declared
parallel scheduling and automatic worktree management; existing configurations
that omit the scheduler retain serial behavior. Ordinary run performs local
Git delivery. Remote PR creation and merge require explicit delivery operations
and their candidate/authorization checks. See the
[workflow guide](https://github.com/ckanner/flywheel/blob/main/docs/en/workflow.md) and
[console guide](https://github.com/ckanner/flywheel/blob/main/docs/en/flow-console.md) for these operations.

## Diagnose without model inference

```sh
flywheel config inspect --cwd /repo --json
flywheel doctor --cwd /repo --json
flywheel doctor --cwd /repo --probe --json
flywheel-codex-pro-worker --doctor
```

Add `--task-dir /repo/docs/my-task` to inspect one prepared task's configuration.
Ordinary doctor checks local configuration and prerequisites. `--probe`
initializes a temporary DSH SDK session, verifies directory and permission
behavior, then closes it. It uses temporary persistence for the probe; it sends
no inference prompt and runs no task acceptance commands. Provider
initialization can still query a model catalog.

Successful diagnostics do not prove live account authorization, quota, or model
responses. Preserve `warn`, `unknown`, and `skipped` as separate outcomes.
Diagnostics and installation do not write task state or start worktrees.

## Runtime boundaries

### Project membership and history

The stable project workspace identifies the repository. Task integration and
packet worktree directories identify where work actually happens. DSH creates a
session under the project and switches its execution directory through the
public working-directory service. Only the desktop host registers workspace
membership; independent SDK workers do not concurrently rewrite desktop
storage. Existing session headers and unresolved receipts retain their original
identities. Unsupported combinations of project, execution directory, and
restricted sandbox fail before dispatch; permissions are not silently widened.

Scoped history can continue within the same task, role, and scope. Executor,
Reviewer, Judge, and second-opinion histories stay separate. A Reviewer must
inspect the complete current candidate even when reusing its own history.
New templates explicitly select scoped mode; old configurations that omit it
retain fresh behavior. The explicit Plan `resident-session:false` opt-out
continues to apply.

Both modes retain invocation safety records. Unknown dispatch, outcome, or
cleanup blocks new execution. Codex/Claude public APIs add durable Go ACKs,
supervisor journals, and exact read-only receipt recovery; DSH/Kimi retain their
own supported paths. Quality rebuild thresholds default to disabled. See
[Recovery](https://github.com/ckanner/flywheel/blob/main/docs/en/session-recovery.md) and
[Paired evaluation](https://github.com/ckanner/flywheel/blob/main/docs/en/session-evaluation.md).

### Permissions and worker lifecycle

Optional `scheduler.prewarm:true` uses the DSH worker's two-phase `--prewarm`
protocol to initialize one SDK during verification, without creating a Session
or sending a prompt until the formal invocation is admitted. It defaults to
false; other backends retain their existing startup paths. The same bounded
shutdown and process-group proof apply to unused workers. See the
[worker protocol](PROTOCOL.md#dsh-sdk-一次性预热) for expiry, cancellation and recovery.

The DSH worker supports read-only, workspace-write, and explicit full access,
with approvals set to never. New subscription presets request full access;
existing files without a permission setting retain role-based behavior.
Actual policy is checked before prompting. Full access does not establish an
OS sandbox or remove host static denials, account restrictions, or OS permissions.
A Git worktree is also not a security sandbox.

The temporary SDK overlay applies only to the independent worker. An explicit
native-profile enables owned PTC subagents, Team, Workflow, Web and optional LCU;
without one, the existing tool policy remains in force. Automatic titles,
compaction, goals and scheduled input remain disabled. Full access alone does
not enable native capabilities. The overlay does not change desktop presets or
reminder storage. Use the [native guide](https://github.com/ckanner/flywheel/blob/main/docs/en/native-capabilities.md)
for configuration, receipts, Tower and independent native Review/Team jobs.

Main-task Codex calls check the complete actual sandbox policy at `thread-ready`
before Go acknowledges dispatch, then check the terminal receipt again.
Legacy/advisory paths retain their terminal checks. Claude's native init arrives
after its prompt and cannot provide the same pre-dispatch observation. Kimi
print mode does not provide an OS sandbox that this adapter can promise.

A successful result requires a trustworthy terminal receipt and confirmed
process cleanup. An outer worker exit does not prove SDK or guardian cleanup.
Graceful SDK shutdown allows two seconds for ordinary calls, 15 seconds for
native calls and 135 seconds for LCU, with at least 20/145 seconds at the Go
boundary, before bounded TERM/KILL cleanup. Lost outcomes preserve evidence and block automatic replay.
Missing usage and subscription dollar cost remain unknown, not zero. Complete
worker histories are not automatically merged into the parent's DSH trajectory;
public Pro inputs support explicit media/structured output, while formal Go
loops do not automatically inject archived images.

## Maintain and develop

The explicit plugin setting `flywheelBinary` takes precedence over the managed
installation record. Without it, the plugin uses the validated launcher recorded
under `~/.config/flywheel/`. Without a record, the installed package uses its own
`bin/flywheel.js`; only the private source harness retains the legacy PATH fallback.
A damaged record or changed launcher fails visibly. `controlTimeoutMs` defaults
to 60,000 and controls the desktop control request, not the task's total budget.

An existing managed launcher must match the installed package version. After updating
only the DSH copy, run setup for that version to refresh the managed installation;
a stale record is reported instead of mixing a newer view with an older engine.

After upgrading, moving the npm prefix, changing Node, or adding native CLIs,
run `flywheel setup --no-global` with the intended prefix and desktop CLI
options, then reload DSH as needed. Existing configuration and modified skills
are preserved. Adopt changed defaults through the explicit configuration
migration, not by deleting ownership records.

For a source checkout:

```sh
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check
```

See [Contributing](https://github.com/ckanner/flywheel/blob/main/docs/en/contributing.md) for bridge and release checks.
Offline renderer/SDK tests establish the exercised contracts, not physical Mac
window geometry, real-account inference, or measured quality gains.

| Reference | Purpose |
|---|---|
| [Usage handbook](https://github.com/ckanner/flywheel/blob/main/docs/en/usage-handbook.md) | Scenarios for incidents, issue fixes, and complex projects |
| [Worker protocol](PROTOCOL.md) | Detailed wire, permission, lifecycle, and error contracts (Chinese) |
| [Vendored package provenance](vendor/README.md) | Codex source identity, tarball hashes, and optional Claude source evidence |
| [JEV decisions](https://github.com/ckanner/flywheel/blob/main/docs/en/jev.md) | One master switch for actual bounded decisions, with offline replay and explicit evaluation |
| [Release guide](https://github.com/ckanner/flywheel/blob/main/docs/en/releasing.md) | Build, install, and publish the same checked artifacts |

## License

[MIT](LICENSE). Adapted DeepSeek UI portions and bundled dependencies retain
their own notices; see [Third-party notices](THIRD_PARTY_NOTICES.md).
