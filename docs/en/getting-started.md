# Install and run your first Flywheel task

[English README](../../README.en.md) · [中文](../getting-started.md) · [Workflow](workflow.md) · [Configuration](configuration.md)

This guide covers **Flywheel 0.28.5**: npm installation, the DSH desktop and workers, configuration, preflight, a small task, and recovery. The checked desktop interfaces are **DSH Mac 0.2.1-rc.5 and 0.2.2-rc.1**, both declared from Flywheel 0.28.4. DSH 0.2.2 itself requires Node 24.9 or later; use its official desktop-supplied CLI. Real desktop UI, macOS permissions, authentication, and model inference still need testing on the intended machine.

Before installing, check `npm view @ckanner/flywheel version`. Continue with these instructions once the registry has the version you intend to use. A merged source change or GitHub Release does not establish npm publication, and an older installed release may not include these interfaces.

**Adaptive workflow in current source:** `/flywheel-task`, `control launch`, Auto/Quick/Full, and automatic DSH task association require a CLI, plugin, and skills containing this implementation. Check `flywheel control launch --help`; the earlier 0.28.5 version label alone does not establish that these additions are installed.

The top-level **Flywheel conversation view** below is available from `0.28.1`. Version `0.28.0` uses the composer popup; upgrade the CLI and desktop plugin to the same new version. Merging source or reloading the old version does not change that layout. CLI and task-directory interfaces remain the same.

Subscription preset migration and `permissions.mode` are available from **0.28.2**. Upgrades preserve existing configuration; preview `flywheel config migrate --preset subscriptions`, review the model bindings and full-access policy, then explicitly apply it with `--apply` and a backup.

**Ordering from 0.28.5:** the external Flywheel conversation view follows the built-in Statistics view. DSH still owns view selection and overflow navigation.

**Fixes in 0.28.3:** the `deepseek-account` default and the current session directory when the project path is omitted. Version 0.28.2 still uses `deepseek-official` and the original directory. Check the registry version before installing. Upgrades preserve configuration; adopting the account preset requires explicit migration.

**Optional Claude backend:** the default installation does not require Claude. Selecting `claude-code-pro` requires the companion 0.4.0 plugin in the worker's dependency tree and an installed/authenticated Claude CLI. See [Claude Code Pro](claude-code-pro.md) for configuration and source-candidate boundaries.

The reviewed Codex Pro 0.9.0 source package is bundled with this Flywheel candidate; installation does not assume an unpublished registry version exists. See [provenance](../../plugins/dsh/vendor/README.md), [recovery](session-recovery.md), and [paired evaluation](session-evaluation.md).

## 1. Prepare the environment

| Requirement | Why it is needed |
|---|---|
| Git with your commit identity | Worktrees and local delivery commits |
| Node.js 22.18.0 or later and npm | Installer, launchers, and JavaScript workers |
| DSH Mac and its desktop-supplied `dsh` command | Desktop plugin management and DSH SDK execution |
| Authenticated Kimi CLI | Default Plan, frontend execution, and optional Garden |
| Codex executable and existing login | The hard route's Codex Pro plugin runs Codex app-server |
| DeepSeek and ChatGPT providers in the DSH SDK profile | Default execution, independent Review, and Judge |
| Go 1.27 or later | Only the Go example below and source builds; not npm binary installation |

Check command locations and versions with `git --version`, `node --version`, `npm --version`, `command -v dsh`, `dsh --version`, `kimi --version`, and `codex --version`. Check the desktop version in About as well. Use the respective tools' documented installation and login flows.

The defaults reuse the Kimi subscription, ChatGPT subscription, and existing signed-in, funded DeepSeek account. DeepSeek API access is an explicitly configured alternative. Flywheel does not log in, copy credentials, or replace the desktop application. Full needs its configured Plan, Execute, Review, and Judge backends. A qualified Quick task needs its actual execution route and independent Review; Plan and Judge are recorded as inapplicable. Unused execution routes do not become runtime requirements.

## 2. Install

From a terminal that can find the desktop's `dsh` command:

```sh
npx -y @ckanner/flywheel@latest install
```

For an agent consuming a single JSON result, use the same command with `--json`. The installer installs its exact version globally, resolves the platform binary and workers, installs that version's bundled skills (eleven in current source), creates global configuration only if absent, records a GUI launcher, registers the matching DSH plugin, and runs local diagnostics.

`ok:true` means the installation steps completed. `ready:true` means that local doctor found no blocker. For example, missing Kimi can produce `ok:true, ready:false` with the reason in `diagnostic.checks`. Neither result proves real authentication or inference. A failed step retains its partial results so the actual cause can be corrected before retrying.

### Useful options

| Option | Effect |
|---|---|
| `--prefix PATH` | Choose the npm global prefix |
| `--project PATH` | Install skills into an existing project's `.agents/skills` and diagnose that project; CLI installation remains global |
| `--dsh-bin PATH` | Use the verified desktop DSH command at an explicit path |
| `--profile NAME` | Desktop plugin target profile, default `desktop`; unrelated to the worker's SDK profile |
| `--no-dsh` | Skip desktop plugin registration |
| `--no-global` | Set up the current persistent installation without another global install |
| `--timeout-ms N` | External install-step timeout; default 600000 ms, range 100–1800000 |
| `--json` | Emit one structured installation result |

For a user-owned global prefix:

```sh
npx -y @ckanner/flywheel@latest install --prefix "$HOME/.local"
export PATH="$HOME/.local/bin:$PATH"
flywheel version
```

Add the actual prefix's `bin` directory to your shell configuration for later terminals. Supply `--dsh-bin /absolute/path/to/dsh` if necessary. Do not guess an executable path inside a macOS app bundle.

Plain `npm install --global @ckanner/flywheel` only places package files. Complete user setup explicitly afterward:

```sh
flywheel setup --no-global
```

`install` and `setup` share options. Do not use `--no-global` to turn a temporary npx cache into the permanent GUI entry.

## 3. Distinguish desktop and SDK setup

The installer uses the official desktop plugin interface with exact version selection:

```text
dsh plugin --profile desktop add --save-exact --ignore-scripts @ckanner/dsh-plugin-flywheel@<installed-version>
```

This is the command shape used internally, not a placeholder to paste literally. An independent npm DSH CLI cannot manage the reserved `desktop` profile. Reload according to the host's instructions and select **Flywheel** in the native view bar alongside Chat and Trajectory. Check that bar's More menu if the entry is not directly visible.

This view controls and observes an already prepared task: progress, intervention, effective models, environment checks, and explicit start/refresh/stop. DSH keeps its chat, trajectory, composer, model selector, and right sidebar. Discuss requirements and prepare contracts through the original chat and skills. A launch or start through the DSH tools associates the exact task with that conversation. The view does not scan for tasks started elsewhere; select the same task directory to observe one.

The worker defaults to a separate **`sdk` profile**. The standard SDK layers on `base`, which already includes DeepSeek's account and API adapters. Current source explicitly selects `provider: "deepseek-account"` and `model: "deepseek-flash"` to reuse the signed-in, funded account. `deepseek-official` is only for an explicitly chosen API-key configuration; there is no automatic paid-channel fallback.

With the same `DSH_HOME`, the standard desktop and SDK can share the existing login in the default `<DSH_HOME>/.credentials.yaml`, independently of the profile. No token copying or switch to the `desktop` profile is needed. Check custom home, profile, or credential-store choices separately. The SDK does not inherit the desktop's selected conversation model; Flywheel uses its own effective bindings, visible through `config inspect` or the view's model-configuration action. Native GPT routing needs the ChatGPT adapter in the SDK profile. The checked adapter baseline can be installed with:

```sh
dsh plugin --profile sdk add dsh-plugin-chatgpt@0.1.17
```

That is a checked compatibility version, not a claim about the latest adapter. Follow its authorization documentation for the chosen profile and keep `DSH_HOME` consistent. Do not copy tokens into task files or Git.

The SDK worker's temporary permission and tool policy applies only to its child process. It does not edit the desktop profile or remove desktop capabilities. Worker-owned delegation, auxiliary model compaction, and currently the entire web tool are disabled. A native view does not make the worker inherit desktop plugins or merge its history. See the [plugin's lifecycle boundaries](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.en.md#permissions-and-lifecycle-boundaries) for the capability limits and separate work needed to restore retrieval safely.

The hard route uses the installed `dsh-subagent-codex-pro@0.9.0` dependency's public machine API directly. It does not use `subagent_codex_pro` as a native LLM provider or require a parent model to delegate. Its underlying Codex app-server still needs the local Codex command and login.

### Desktop launcher

Setup creates `~/.config/flywheel/npm-launcher` and an installation record with absolute Node/CLI paths, the checked DSH entry, and the installation-time tool PATH. The desktop plugin verifies the managed launcher before using it. A user-specified `flywheelBinary` takes precedence. Missing records can use the legacy PATH lookup; damaged records or modified managed launchers fail visibly.

After moving Node or the npm prefix, or installing previously missing tools, run `flywheel setup --no-global` in a terminal where all required tools are available, preserving your original options. Save custom launcher changes before resolving a conflict. A shell export alone does not update an already running desktop process; recheck from the panel.

## 4. Select configuration and skills

If the view still shows Z.AI/GLM or Kimi K3, inspect its configuration source: upgrades preserve that file. A version containing this change can back up and migrate it with `flywheel config migrate --preset subscriptions --cwd /repo --apply --json`; omit `--apply` to preview, and add `--task-dir` for task overrides. This selects ChatGPT/Kimi Code/DeepSeek Flash, the agreed efforts, and full access for every stage. See [migration and permissions](configuration.md#apply-the-subscription-preset-to-existing-configuration). Published 0.28.1 does not contain this command.

Configuration precedence is task directory, project root, global file, then embedded defaults. The first complete file wins; files do not merge. Existing configuration is preserved during upgrades. Inspect the actual source:

```sh
flywheel config inspect --cwd /absolute/path/to/repo --json
flywheel template flywheel.config.example.json
flywheel template flywheel.profiles.example.json
```

Use either complete template as a starting point and review it before writing a configuration file. The [configuration guide](configuration.md) covers safe export, fields, profile overrides, channel identity, and migration to `codex-pro`. `loops.plan` controls Full engine planning; the host's selected conversation model controls initial preparation through `flywheel-task` or `flywheel-new`.

Current source includes eleven skills. Skills are optional host entry points; the desktop panel and CLI also work independently. To inspect or install them for an existing project:

```sh
flywheel skills list
flywheel skills read flywheel-task
flywheel install-skills --project /absolute/path/to/repo --json
```

Follow the host's skill-discovery rules. Copying files does not prove the host loaded them. Installer-owned unchanged snapshots can update; modified files, unowned directories, and invalid manifests are preserved and reported as conflicts.

In DSH Chat, use `/flywheel-task <goal>` for research, preparation, and validated launch; Auto is the default. Use `/flywheel-new` for a proposal without launching, `/flywheel-run` for an existing task, and `/flywheel-status` for its state. Kimi uses `/skill:flywheel-task` and corresponding prefixed names. **No initial Flywheel-view form is required.** The view is an alternative way to open an already prepared task. See the [copyable workflow](workflow.md#0-normal-entry-describe-a-goal-and-launch).

## 5. Run preflight without inference

From the target repository:

```sh
flywheel doctor --cwd "$PWD" --json
flywheel doctor --cwd "$PWD" --probe --json
```

Local checks cover configuration, directories, command availability/version, and declared environment variables. `--probe` also initializes DSH SDK sessions, checks accepted configuration, working directory, permissions, and shutdown. It sends no inference prompt and runs no acceptance command. Provider initialization may query a model catalog and create SDK-owned cache/session files.

Codex Pro additionally verifies that the exact public machine API can be imported and that Node/Codex commands are available. It does not run a model task, prove account entitlement, or pre-check the complete policy. Its full-policy limitation remains a warning; model/effort/authorization remain unknown.

Results use `pass`, `fail`, `warn`, `unknown`, and `skipped`. A fail produces exit code 1. A successful aggregate result only covers the checks performed. No preflight creates Flywheel state, worktrees, or global configuration. Check again in the desktop panel because GUI environment and profile selection may differ.

After contracts exist, narrow the checks to the selected task:

```sh
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --probe --json
```

Task preflight uses a fresh Spec's qualification or the saved workflow policy. For qualified Quick it skips Plan, Judge, and the Judge panel; legacy tasks remain Full. It does not consume a later launch/run `--mode` override. A malformed saved policy blocks probes rather than being silently reclassified. Configuration inspection still lists all configured bindings.

A task without packets can be inspected during preparation, but a real launch still needs at least one legal packet within the authorized scope. Fix schema, directory, provider, or permission failures at their source.

The view's optional configuration and troubleshooting section offers “Inspect model configuration,” “Check installation and configuration,” and “Check DSH integration.” Use these for setup, configuration changes, or troubleshooting; they are not buttons that must be pressed for every task. Neither diagnostic performs model inference.

## 6. Prepare a task

### Existing repository

The normal DSH entry is `/flywheel-task` followed by the actual requirement. The assistant researches the repository, creates or deliberately reuses a task integration branch and independent worktree, prepares compact contracts and real verification evidence, then launches within the existing authorization. Auto chooses Quick only when the contracts qualify; uncertain or complex tasks use Full. The [workflow guide](workflow.md) gives the branch commands, model bindings, delivery boundaries, and working directory for each stage.

Use `/flywheel-new` when you want to review a proposal before execution. Both preparation entries report two actual absolute paths: the **project directory** is the code worktree; the **task files directory** is its `docs/<slug>` holding this task's contracts and later state/reports. You do not need to invent an empty directory or fill in the UI first. Keep both paths for observation and recovery.

Use the bundled templates to prepare a real Spec and packets with actual acceptance commands, protected oracles, and recorded dry-run results. Persist authorized contracts before starting so packet worktrees can read them from Git. Preparation and loop scheduling are separate: skills never write `state.json` or orchestrate stage retries themselves.

### Disposable first-run example

For a first installation check, the packaged scaffold creates a separate Go addition task. The initial implementation is intentionally wrong; the executor must fix it and add ordinary tests without changing the frozen acceptance test. This example needs Go 1.27 or later and an unused destination directory.

The scaffold intentionally has no `workflow` assessment and therefore follows legacy Full behavior. To try Quick, ask `flywheel-task` to inspect and qualify an actual small task; changing a mode flag alone does not supply the missing evidence.

```sh
FLYWHEEL_PACKAGE="$(npm root --global)/@ckanner/flywheel"
bash "$FLYWHEEL_PACKAGE/docs/examples/create-first-task.sh" "$HOME/flywheel-demo"
cd "$HOME/flywheel-demo"
git init -b main
```

For a custom prefix, resolve the package with `npm root --global --prefix "$HOME/.local"` instead. If the CLI is not on PATH, set `FLYWHEEL_BIN` to its absolute path when invoking the script. The scaffold writes files and reads installed templates; it does not invoke models or commit anything.

The scaffold includes `sum.go`, frozen `acceptance/sum_test.go`, `docs/add/spec.md`, a packet, a complete project configuration, and ignore rules for generated/volatile files. Run every L1 command from the packet. For the shipped example:

```sh
go test ./...
go test -coverprofile=coverage.out . &&
  go tool cover -func=coverage.out |
  awk '$NF != "100.0%" && $NF != "100%" { bad=1 } END { exit bad }'
```

The initial failures are expected. Record the actual exit codes and summaries in the Spec's dry-run evidence. Review the contracts and frozen boundaries, then commit the scaffold in this new demonstration repository:

```sh
git add go.mod sum.go acceptance .gitignore flywheel.config.json docs/add
git diff --cached
git commit -m "docs: prepare first flywheel task"
git worktree add -b task/add ../flywheel-demo-add
cd ../flywheel-demo-add
```

This disposable scaffold generates contracts before the task worktree is created; for an existing repository, use the preparation order in the preceding section. The real run below uses the new task worktree and its own `docs/add`, so local packet delivery stays on `task/add`.

## 7. Start, observe, stop, and recover

The following commands invoke the real configured backends. Run them from the demo task worktree, or substitute your actual task name:

```sh
flywheel config inspect --cwd "$PWD" --task-dir "$PWD/docs/add" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/add" --probe --json
flywheel control start --cwd "$PWD" --task-dir "$PWD/docs/add"
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/add"
```

In DSH Chat, the equivalent entries are `/flywheel-run <absolute-task-path>` and `/flywheel-status <same-path>`, with `/flywheel-stop <same-path>` when stopping is needed. Replace the placeholders with the real path returned during preparation. These skills resolve the project root without requiring an initial view selection.

Launch/start through flywheel_launch or flywheel_control associates the task with this DSH conversation. Terminal runs record the host session ID; older runs are selectable from recent tasks. There are no editable path fields: known locations supplied by the assistant/engine appear read-only. Prepared tasks can also be associated through a read-only status query.

The UI separates the last stage from overall delivery and refreshes running state plus registered discovery. It does not scan directories. Public session projections restore explicit associations; forks do not inherit the parent task. Selection and status snapshots are separate, late replies cannot replace another task, and unconfirmed operations require reconciliation against their original target.

After a launch/start/stop timeout or disconnect, the background task's outcome may be unconfirmed. Keep the original task and project paths, refresh that task's status, and inspect logs before deciding how to recover. The view disables further mutations until that task is successfully reconciled and never retries them automatically. A prior snapshot, closing a page, or switching sessions cannot prove the engine stopped. The native trajectory retains its parent-session records; full worker histories are not merged into it.

If the host explicitly confirms rejection before the control process started, such as a missing directory, correct the path or environment and submit again. This requires structured host evidence; error-message text alone cannot establish that a lost request was never executed.

For a foreground run that pauses before Review, use this instead of background start:

```sh
flywheel run docs/add --pause-before review
```

`control launch` and `control start` do not accept `--pause-before`, and pause mode cannot be combined with `--only`. `--only execute` runs the bounded execution/L1 loop; it is neither Quick nor a single model call. For Quick, `--only review` uses the task-final review contract and saves any required one-way Full upgrade without continuing into Plan or local merge; `--only verify` includes applicable candidate L2/L3, even with a selected packet. Running Plan on Quick requires an explicit `--mode full` upgrade. See the individual skills for partial-loop recovery.

Inspect progress:

```sh
flywheel status docs/add
tail -n 80 docs/add/orchestrator.log
git log -5 --oneline
```

Stop with:

```sh
flywheel control stop --cwd "$PWD" --task-dir "$PWD/docs/add"
```

Only resume with `control start` after confirming the previous run ended and addressing its actual error. Retain the original task/worktree paths, reports, and state. A stop result concerns engine lifetime; recorded cleanup uncertainty still needs investigation, especially for Codex Pro's separately managed processes. See [recovery](workflow.md#7-stop-and-recover-without-losing-evidence).

For the first real run, record whether the task reached `completed`, all requirements were covered, frozen acceptance remained unchanged, independent review and reports exist, and the implementation was delivered in local Git. Record actual model identity, latency, and usage separately from preflight. No check in this guide claims those results on your behalf.

## 8. Upgrade and troubleshoot

Stop active tasks and confirm their state, preserve configuration/customizations, then rerun the installation command with the original prefix/project/DSH options. To pin 0.28.0, first confirm that exact version exists in the registry and replace `@latest` with `@0.28.0`. `flywheel setup --no-global` repairs setup for the installed version without selecting a different CLI release.

Inside Flywheel's own source checkout, npm may consider the matching local package satisfied and then resolve an older global executable from PATH. If the installation report shows a different version, retry outside the checkout, or run `npm install --global --ignore-scripts --include=optional @ckanner/flywheel@0.30.0` followed by `flywheel setup --no-global` from that exact prefix. Preserve a custom prefix in both steps. Check the CLI version, installation report, and desktop-profile plugin version. Reload the running desktop as directed by the host so it loads the newly installed modules.

From 0.30.0, adopting the complete packaged preset is explicit: choose the global or project scope in the configuration editor, read it, load this version's defaults, review, and save. Saving backs up and replaces the selected configuration. Higher-priority task/project files still take precedence.

Existing hard-route configuration remains unchanged after upgrade. To adopt the new plugin route, change the effective hard binding or profile to `harness: "codex-pro"`, preserve model/channel/thinking, inspect it, and restart. Do not use `subagent_codex_pro` as a provider.

| Symptom | First check |
|---|---|
| Missing `install` or `control` command | Published version, `flywheel version`, PATH, and any explicit desktop `flywheelBinary` |
| The large composer popup is still present | Published `0.28.0` has the old layout; verify a build containing the conversation-view fix is installed, then reload as directed by DSH |
| Flywheel is not directly visible in the top bar | Check More, plugin enablement, and the actual installed version; `/flywheel` and CLI remain diagnostic entry points |
| Install succeeds but `ready:false` | Resolve the specific `diagnostic.checks` blockers |
| Terminal works but the desktop does not | Managed launcher, explicit override, GUI PATH/profile/environment; rerun setup and check the panel |
| A configuration edit has no effect | `config inspect` source and higher-priority task/project files; restart the engine |
| Skill or launcher conflict | Preserve customizations, compare the specific files, and resolve the reported conflict without altering ownership hashes |
| Worktree creation falls back in-place | Stop if isolation is required; inspect persisted mode and preserve work before recovery |
| Cleanup is unconfirmed | Inspect task-owned processes and receipts before retrying; elapsed time alone is not proof |
| Start/stop outcome is unconfirmed | Refresh the same task and inspect logs; timeout/disconnection proves neither no start nor a completed stop, and mutations are not retried automatically |

Installation is not one atomic transaction across npm, DSH, and all skills. Correct the reported cause and retry; preserve completed steps and user files. Do not delete whole configuration directories or modify integrity manifests to bypass conflicts.

To uninstall, first stop running tasks, remove the desktop Flywheel plugin through the desktop's supported plugin-management interface, then uninstall the global CLI using the same prefix:

```sh
npm uninstall --global @ckanner/flywheel
```

Configuration, task ledgers, modified skills, and worktrees require deliberate separate handling; do not remove them as an incidental cleanup step. Maintainer build and publication procedures are in the [release guide](https://github.com/ckanner/flywheel/blob/main/docs/en/releasing.md). Normal users do not need npm publishing credentials or GitHub OIDC to install Flywheel.
