# Getting started

[Documentation](../README.md) · English · [简体中文](../zh-CN/getting-started.md)

Install Flywheel, connect the backends you intend to use, then run a small task.
This guide follows the current repository. Check the installed version and the npm
version you choose; a merged change does not update an existing installation.
Keep the CLI, bundled engines, and DSH plugin at one version.

For the complete path through task branches and PR review, see [Workflow](workflow.md).
For incidents, issue fixes, and complex projects, follow the [Usage handbook](usage-handbook.md).

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
| Antigravity `agy` CLI and native account login | Gemini design, ordinary/complex execution, investigation analysis and Intake |
| Claude CLI and native subscription login | Implementation planning and frontend/multimodal routes through Claude Code Pro |
| Codex executable and ChatGPT account login | Backend/hard execution, independent reviews and Judge through Codex app-server |
| Authenticated Kimi CLI | Native-agent batch execution and optional Garden |
| Go 1.27 or later | Only the disposable Go example and source builds; not npm binary installation |

The plugin admits DSH `>=0.2.1-rc.5 <0.4.0-0 || ^0.2.2-0 || ^0.2.3-0 || ^0.3.0-0`, including the 0.3 series and its
prereleases. Routine upgrades do not require adding a version to an allowlist. This is a
best-effort window: required host services and SDK permission/cleanup checks still fail
closed on a genuinely incompatible release and are fixed in code. DSH 0.2.2 has
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
claude --version
agy --version
```

Use each backend's normal installation and login flow. Flywheel does not sign in,
copy credentials, or replace the desktop application. Full needs its configured
design/planning, execution, review and Judge backends. A new compact task also needs
its combined-design and joint-review bindings before Quick execution and final Review;
an upgrade to Full requires the additional stages.
Unused execution routes do not become task runtime requirements.

Claude remains an optional package peer, but the default planning and frontend
bindings use it. Those stages require the pinned 0.7.0 companion in the worker's
dependency tree and a compatible Claude CLI with a supported native subscription.
See [Claude Code Pro](claude-code-pro.md). Gemini uses the pinned Antigravity adapter
and official `agy` CLI, without ACP; see [Antigravity](antigravity.md).

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
An explicitly selected DSH worker runs in a separate **`sdk` profile** and does not
inherit the conversation's selected model or every desktop plugin. The default
subscription workers invoke their native clients directly.

### Account selection

The default declares `subscription-only` and uses native Gemini, Claude, Codex and
Kimi clients. It has no DeepSeek route, API-key fallback or automatic paid-credit
fallback. Each native client keeps its own normal login; selecting a model in a DSH
conversation does not authenticate the independent worker. The exact stage/profile
table and admission limits are maintained in [Model policy](model-policy.md).

Codex Pro 0.10.2 directly uses the official app-server and checks the ChatGPT account
route. Claude Code Pro 0.7.0 checks a supported native subscription with extra usage
disabled before sending the task. Antigravity rejects API/custom-provider routes and
enabled credits fallback; Kimi uses managed native agent mode with Kimi Code OAuth.
These controls do not prove remaining quota or promise account-wide zero extra costs.
Keep credentials out of task files and Git. Older explicit DSH configurations remain
separate compatibility choices until migration; they do not become fallbacks for the
new preset.

Without a selected `native-profile`, the DSH worker's temporary policy disables
delegation and the Web tool. An explicit profile can enable owned PTC Agents, Team,
Workflow, Web and optional LCU; auxiliary model compaction remains disabled.
Desktop views and presets do not configure the independent worker or merge its full
history into the parent trajectory. See [Native capabilities](native-capabilities.md)
for configuration and the [plugin documentation](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.md)
for lifecycle boundaries.

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
then apply if it matches the intended accounts and retained permissions:

```sh
flywheel config migrate --preset subscription-cli --cwd /absolute/path/to/repo --json
flywheel config migrate --preset subscription-cli --cwd /absolute/path/to/repo --apply --json
```

Add `--task-dir` for a task-level override. Migration backs up the selected file and
preserves unrelated settings and permissions. The historical `subscriptions` preset
does not select this new four-client policy. Migration does not automatically opt an old configuration
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
`/flywheel-task <goal>` to prepare and launch, `/flywheel-new <goal>` for design/review followed by approval,
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

Antigravity diagnostics check the installed CLI and subscription controls without
sending a task. Claude diagnostics check the matching package and local CLI;
the pre-prompt subscription check remains a runtime boundary. Neither replaces a
separately authorized live account test.

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
directory under the effective worktree policy, records a thin brief and launches
Go-owned design and independent review. Task retains existing implementation
authority; `/flywheel-new` stops after review for your approval. Use the Flywheel
view to give feedback, run Plan, approve only, or approve and start. No initial
console form is required. See [Reviewed planning](reviewed-planning.md).

Keep the **project workspace**, **task execution root**, and **task files directory**
separate. The project is the stable repository identity; task execution may use a
linked worktree, with its contracts at `docs/<slug>`. Preparation reports actual
paths. The engine later manages or registers packet execution directories, and owns
`state.json`. See [Workflow](workflow.md#2-choose-the-task-execution-directory).

### Investigate or continue an existing incident

Use Task with an explicit report-only or investigate-and-fix goal. Go performs bounded
evidence collection; the configured Gemini analysis role prepares the report.
`awaiting-report` resumes analysis, while an unknown pending call only reconciles its
original receipt. After a supported report, an already authorized repair enters thin
brief preparation in the same task and retains its scope and cumulative accounting.

When new logs arrive after a report, ask to continue the original task with that
evidence. The host submits `control investigate` with `action:continue`, the current
revision and a complete next-round contract, then runs it once. Goal, intent and repair
scope remain unchanged; earlier reports and spent budgets remain recorded. See
[Investigation](investigation.md#continue-with-new-evidence-in-the-same-task).

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
the corresponding skills. The canvas follows the current conversation's explicit
tool association, or its latest registered run with an exactly matching host session
ID. To associate a prepared or standalone terminal task, have the host assistant call
`flywheel_control` with `action: "status"` and the actual task and execution paths in
that conversation. A terminal `flywheel control status` command alone does not create
that association. From 0.34.2, multiple associated tasks appear in a conversation-only navigator;
a single task opens directly. There is no editable path form. Runs from other
conversations and unassociated runs remain hidden. See [Workflow console](flow-console.md).

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
