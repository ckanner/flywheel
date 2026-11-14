# Getting started

[Documentation](../README.md) · English · [简体中文](../zh-CN/getting-started.md)

Install Flywheel, connect the backends you intend to use, then run a small task.
This guide follows the current repository. Check the installed version and the npm
version you choose; a merged change does not update an existing installation.
Keep the CLI, bundled engines, and DSH plugin at one version.

For the complete path through task branches and PR review, see [Workflow](workflow.md).

From 0.33, `@ckanner/flywheel` is one complete package: CLI, DSH view/workers,
embedded skills and four platform engines. No separate platform package or first-run
engine download is needed. When upgrading from 0.32.x, quit DSH before running the
installer; it replaces the old desktop package registration and preserves your
configuration and histories. Reload DSH after setup. Existing old npm versions
remain available for historical installations.

## 1. Prepare the environment

| Requirement | Purpose |
|---|---|
| Git with a configured commit identity | Worktrees and local delivery commits |
| Node.js 22.18.0 or later and npm | Installer, launchers, and JavaScript workers |
| DSH Mac and its desktop-supplied `dsh` command | Desktop plugin management and DSH SDK execution |
| Authenticated Kimi CLI | Default Full Plan, frontend execution, and optional Garden |
| Codex executable and existing login | The hard execution route uses Codex Pro and Codex app-server |
| DeepSeek and ChatGPT providers in the DSH SDK profile | Default execution, independent Review, and Judge |
| Go 1.27 or later | Only the disposable Go example and source builds; not npm binary installation |

The plugin admits DSH `>=0.2.1-rc.5 <0.3.0-0`, including later prereleases in
the 0.2 series. Routine rc/patch upgrades do not require adding a version to an allowlist.
The 0.3 series, including its alpha/beta/rc versions, needs a new compatibility review.
Range admission does not bypass required host services or SDK permission/cleanup checks. DSH 0.2.2 has
its own Node 24.9 minimum; use the official desktop-supplied CLI. Check actual command
locations and versions before setup:

```sh
git --version
node --version
npm --version
command -v dsh
dsh --version
kimi --version
codex --version
```

Use each backend's normal installation and login flow. Flywheel does not sign in,
copy credentials, or replace the desktop application. Full needs its configured
Plan/Execute/Review/Judge backends. A qualified Quick task needs its actual execution
route and independent final Review; an upgrade to Full requires the additional stages.
Unused execution routes do not become task runtime requirements.

Claude is optional. Selecting `claude-code-pro` additionally requires the matching
0.5.0 companion package in the worker's dependency tree and an installed,
authenticated Claude CLI. See [Claude Code Pro](claude-code-pro.md).

## 2. Install

From a terminal that can find the desktop's `dsh` command:

```sh
npm view @ckanner/flywheel version
npx -y @ckanner/flywheel@latest install
flywheel version
```

Use a published exact version in place of `@latest` when pinning an installation.
Add `--json` to the install command when an agent needs one structured result.
The installer:

1. Installs its exact version globally and resolves the matching platform binary and workers.
2. Installs the bundled skills and creates global configuration only when absent.
3. Records an absolute GUI launcher and the installation-time tool paths.
4. Registers the same Flywheel plugin version with the selected DSH profile.
5. Runs local diagnostics and reports completed steps and blockers.

`ok:true` means installation completed; `ready:true` means that local diagnostics
found no blocker. Missing tools can produce `ok:true, ready:false`; inspect
`diagnostic.checks`. Neither value proves account access or real model inference.
An installation failure preserves partial results for diagnosis and retry.

### Installation options

| Option | Effect |
|---|---|
| `--prefix PATH` | Choose the npm global prefix |
| `--project PATH` | Install project skills in `.agents/skills` and diagnose that existing project; CLI installation remains global |
| `--dsh-bin PATH` | Select the verified desktop DSH command |
| `--profile NAME` | Desktop plugin target profile, default `desktop`; separate from the worker's SDK profile |
| `--no-dsh` | Skip desktop plugin registration |
| `--no-global` | Set up an existing persistent installation without another global install |
| `--timeout-ms N` | Per-step timeout, default 600000 ms, range 100–1800000 |
| `--json` | Emit one structured installation result |

For a user-owned prefix:

```sh
npx -y @ckanner/flywheel@latest install --prefix "$HOME/.local"
export PATH="$HOME/.local/bin:$PATH"
flywheel version
```

Keep the actual prefix's `bin` in your shell configuration. If needed, pass
`--dsh-bin /absolute/path/to/dsh`; verify that path rather than guessing an app-bundle
location. Plain `npm install --global @ckanner/flywheel` only places files. Complete
setup explicitly afterward with `flywheel setup --no-global`. Do not use
`--no-global` to make a temporary npx cache the permanent GUI entry.

## 3. Connect desktop and SDK backends

Reload the desktop as directed by DSH, open an existing conversation, and select
**Flywheel** beside the built-in views. It follows Statistics; check the overflow
menu if necessary. The blank new-session landing page does not show conversation
views. The console provides a canvas, configuration, and run history while DSH keeps
its conversation, trajectory, composer, and sidebar.

Desktop plugin registration uses the desktop-supplied command and the `desktop`
profile by default. An independent npm DSH CLI cannot manage that reserved profile.
The worker normally runs in a separate **`sdk` profile** and does not inherit the
conversation's selected model or every desktop plugin.

### Account selection

The default DeepSeek route explicitly selects `deepseek-account` / `deepseek-flash`.
The standard SDK `base` profile includes the account adapter. With the same
`DSH_HOME`, standard desktop and SDK setups share the existing account credential
store. Custom homes, profiles, or credential stores need separate checking.
`deepseek-official` is an explicitly configured API-key alternative, with no automatic
paid fallback from the account preset.

Native GPT Review/Judge need the ChatGPT adapter in the SDK profile. The repository's
checked adapter baseline can be installed with:

```sh
dsh plugin --profile sdk add dsh-plugin-chatgpt@0.1.17
```

This is a compatibility baseline, not a latest-version claim. Follow that adapter's
normal authorization flow; keep `DSH_HOME` consistent and credentials out of task
files and Git.

The hard route directly imports the pinned Codex Pro 0.10.0 public machine API.
It needs the local Codex command/login, not a parent model delegating through the
`subagent_codex_pro` tool. Optional Claude has its own dependency and login.

The DSH worker's child-only policy disables delegation, auxiliary model compaction,
and the current web tool. A native conversation view does not restore those worker
capabilities or merge full worker histories into the parent trajectory. See the
[plugin documentation](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.md) for lifecycle and capability boundaries.

### Desktop launcher

Setup writes `~/.config/flywheel/npm-launcher` and an installation record containing
absolute Node/CLI paths and the checked DSH entry. An explicit `flywheelBinary`
overrides the managed launcher. Without a record, the installed package uses its bundled CLI;
invalid records or modified launchers fail visibly.

After moving Node/the npm prefix or installing missing tools, rerun
`flywheel setup --no-global` from a terminal that can find them, preserving original
options. Save custom launcher changes before resolving a conflict. A terminal export
does not update an already running desktop process.

## 4. Select configuration and skills

Configuration discovery selects the first complete file in this order: task,
project, global, embedded defaults. Files do not merge. Inspect the selected source:

```sh
flywheel config inspect --cwd /absolute/path/to/repo --json
flywheel template flywheel.config.example.json
flywheel template flywheel.profiles.example.json
```

Upgrades preserve existing configuration, including old model, permission, history,
and scheduling choices. To adopt the subscription model preset, preview first and
then apply if it matches the intended accounts and full-access policy:

```sh
flywheel config migrate --preset subscriptions --cwd /absolute/path/to/repo --json
flywheel config migrate --preset subscriptions --cwd /absolute/path/to/repo --apply --json
```

Add `--task-dir` for a task-level override. Migration backs up the selected file and
preserves unrelated settings. It does not automatically opt an old configuration
into scoped history or parallel scheduling. To adopt a full installed template, use
the console's global/project configuration editor, load defaults, review, and save.
[Configuration](configuration.md) explains precedence, permissions, and migration.

The current source bundles eleven skills. They are optional host entry points; the
CLI and console work independently. Inspect or install project skills with:

```sh
flywheel skills list
flywheel skills read flywheel-task
flywheel install-skills --project /absolute/path/to/repo --json
```

Follow the host's discovery rules. Installer-owned unchanged snapshots can update;
modified or unowned files are preserved and reported as conflicts. In DSH, use
`/flywheel-task <goal>` to prepare and launch, `/flywheel-new <goal>` for a proposal,
and `/flywheel-run <task-path>` for an existing task. Kimi uses the `/skill:` prefix.

## 5. Run preflight without inference

From the target repository:

```sh
flywheel doctor --cwd "$PWD" --json
flywheel doctor --cwd "$PWD" --probe --json
```

Local checks cover configuration, directories, commands/versions, and declared
environment variables. `--probe` initializes DSH SDK sessions and checks accepted
configuration, directories, permissions, and shutdown. It sends no inference prompt
and runs no acceptance command. Provider initialization may query catalogs or write
caches; probe sessions use temporary persistence rather than the user session list.

Codex Pro diagnostics import the pinned API and check its lifecycle/recovery exports
and local commands. They do not prove model entitlement, a particular saved thread's
recovery, or effective reasoning effort. Runtime Codex calls separately validate the
full policy at `thread-ready` before Go acknowledges dispatch. This does not turn a
no-inference diagnostic into real account validation.

Results are `pass`, `fail`, `warn`, `unknown`, or `skipped`; a fail gives exit code 1.
A task-specific preflight checks the Spec or persisted policy and only applicable
bindings. It does not apply a later runtime `--mode` override:

```sh
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --probe --json
```

No preflight creates Flywheel state or worktrees. Recheck from the desktop if its
PATH/profile/environment differs. Diagnostics are useful after setup or changes;
they are not a mandatory sequence of buttons for every task.

## 6. Prepare a task

### Existing repository

In DSH Chat:

```text
/flywheel-task Fix the identified issue, add regression coverage, and verify the result. mode=auto
```

The assistant researches the repository, chooses or reuses the task integration
directory under the effective worktree policy, prepares contracts and baseline
evidence, then launches within the authorized scope. Use `/flywheel-new` to stop at
preparation. No initial console form is required.

Keep the **project workspace**, **task execution root**, and **task files directory**
separate. The project is the stable repository identity; task execution may use a
linked worktree, with its contracts at `docs/<slug>`. Preparation reports actual
paths. The engine later manages or registers packet execution directories, and owns
`state.json`. See [Workflow](workflow.md#2-choose-the-task-execution-directory).

### Disposable first-run example

The packaged scaffold creates a separate Go addition task with an intentionally
wrong implementation and protected acceptance test. It needs Go 1.27 or later and
an unused destination. Its contracts have no workflow assessment and therefore use
Full; a mode flag alone does not qualify them for Quick.

```sh
FLYWHEEL_PACKAGE="$(npm root --global)/@ckanner/flywheel"
bash "$FLYWHEEL_PACKAGE/docs/examples/create-first-task.sh" "$HOME/flywheel-demo"
cd "$HOME/flywheel-demo"
git init -b main
```

For a custom prefix, resolve the package using `npm root --global --prefix
"$HOME/.local"`. If the CLI is not on PATH, set `FLYWHEEL_BIN` to its absolute path
when running the scaffold. It writes files and reads templates; it does not call
models or commit.

Run every acceptance command from the generated packet. The supplied checks are:

```sh
go test ./...
go test -coverprofile=coverage.out . &&
  go tool cover -func=coverage.out |
  awk '$NF != "100.0%" && $NF != "100%" { bad=1 } END { exit bad }'
```

Initial failures are expected. Record actual results in the Spec, review the frozen
boundaries, then commit the scaffold and create the task branch:

```sh
git add go.mod sum.go acceptance .gitignore flywheel.config.json docs/add
git diff --cached
git commit -m "docs: prepare first flywheel task"
git worktree add -b task/add ../flywheel-demo-add
cd ../flywheel-demo-add
```

This demonstration generates contracts before creating its task worktree. In an
existing repository, follow the preparation order in [Workflow](workflow.md).

## 7. Start, observe, stop, and recover

These commands invoke the configured real backends. Run them from the demo task
checkout, or substitute your actual task name:

```sh
flywheel config inspect --cwd "$PWD" --task-dir "$PWD/docs/add" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/add" --probe --json
flywheel control start --cwd "$PWD" --task-dir "$PWD/docs/add"
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/add"
```

In DSH, `/flywheel-run <absolute-task-path>` and `/flywheel-status <same-path>` are
equivalent entry points. Launch/start and read-only status association make the task
selectable in the console. It reads registered tasks rather than scanning folders;
known project/execution/file locations are read-only details. One current-session
task can open automatically; multiple or unassociated tasks require selection.

The UI distinguishes this stage's result from overall delivery. Running state and
registered discovery refresh automatically. A timeout or disconnect can leave an
operation unconfirmed: keep its original paths and reconcile that task before
another mutation. Old snapshots, page closure, or session switching cannot prove
it stopped. Structured host evidence of rejection before control-process start is
different from an unknown outcome.

For a foreground run that pauses before Review, use this **instead of** background
start:

```sh
flywheel run docs/add --pause-before review
```

`control launch`/`start` do not accept `--pause-before`, which also cannot combine
with `--only`. Partial-loop commands do not imply whole-task completion; see
[Partial stages](workflow.md#partial-stages-and-permissions).

Inspect and stop with:

```sh
flywheel status docs/add
tail -n 80 docs/add/orchestrator.log
flywheel control stop --cwd "$PWD" --task-dir "$PWD/docs/add"
```

Resume from the same paths only after reconciling the previous run and addressing
the actual failure. Keep state, reports, and worktrees. Unknown dispatch, outcome,
or cleanup cannot be bypassed by switching to fresh history or another backend.
See [Recovery](session-recovery.md).

For the first real run, inspect `completed`, requirement coverage, unchanged frozen
acceptance, independent review, reports, and local Git delivery. Real model identity,
latency, and usage are separate observations from preflight. PR creation/merge is a
separately authorized [console operation](flow-console.md#pr-and-merge).

## 8. Upgrade, troubleshoot, or uninstall

### Upgrade and repair

Stop active tasks and reconcile their state. Preserve configuration/customizations,
then repeat installation with the original prefix/project/DSH options. Confirm
`flywheel version`, the installation report, and the desktop plugin version, then
reload DSH as directed. `flywheel setup --no-global` repairs the installed version.

Run npx installation outside Flywheel's own source checkout so npm does not satisfy
the request with that local package. If needed, explicitly install the chosen
published version globally, then run setup from that exact prefix. Existing
configuration stays in place; inspect higher-priority task files before assuming a
project/global edit took effect.

### Troubleshooting

| Symptom | First check |
|---|---|
| A command or new layout is missing | Installed CLI/plugin versions, PATH, and explicit `flywheelBinary` overrides |
| Flywheel is absent from the view bar | Existing conversation, overflow menu, plugin enablement, and host reload |
| `ok:true`, `ready:false` after install | Specific blockers in `diagnostic.checks` |
| Global prefix is unwritable | Use a user-owned `--prefix` and its `bin` directory |
| The desktop profile is rejected | Use the desktop-supplied DSH command, not an independent npm CLI |
| Terminal works but desktop does not | Managed launcher, tool paths, SDK profile, and `DSH_HOME`; rerun setup after tool changes |
| Provider is unregistered or unauthorized | The worker's SDK profile and that provider's normal login flow |
| Model changes have no effect | Effective configuration source and engine restart |
| A channel rename does not change accounts | Channel is a configuration/accounting label; actual authorization belongs to the backend |
| Doctor shows unknown or skipped | Read the individual reason; neither means real inference passed |
| Cost is missing | Usage/cost was not reported; unknown is not zero |
| Acceptance fails despite exit 0 | Candidate drift, timeout, cancellation, truncation, or cleanup failure |
| Isolation is unavailable | Recorded execution mode; parallel automatic worktree management fails instead of falling back |
| Start/stop or cleanup is unconfirmed | Original task status, logs, owned processes, and trusted receipts before retrying |
| Git delivery fails | Preserved worktree, hooks, and diff; resolve the cause without forced cleanup |
| Skill or launcher ownership conflicts | Back up the specific customization and compare it; do not rewrite integrity manifests |

Install is not one atomic transaction across npm, DSH, and skills. Correct the
reported cause and retry without deleting unrelated user files. Acceptance commands
may need additional named variables in `verify.env-allowlist`; do not inject all
model credentials into tests.

### Uninstall

Stop and reconcile active tasks, remove the desktop Flywheel plugin through DSH's
supported plugin manager, then uninstall the global CLI using the same prefix:

```sh
npm uninstall --global @ckanner/flywheel
```

Configuration, task ledgers, skills, installation records, and worktrees are retained
for deliberate separate handling. This does not uninstall DSH/Kimi/Codex or remove
account credentials. Source builds and contribution checks are in
[Contributing](contributing.md); maintainer publication is in [Releasing](releasing.md).
