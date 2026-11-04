# Install and run your first Flywheel task

[English README](../../README.en.md) · [中文](../getting-started.md) · [Workflow](workflow.md) · [Configuration](configuration.md)

This guide covers the **0.28.0 implementation**: npm installation, the DSH desktop and workers, configuration, preflight, a small task, and recovery. The checked desktop interface baseline is **DSH Mac 0.2.1-rc.5**. Real desktop UI, macOS permissions, authentication, and model inference still need testing on the intended machine.

Before installing, check `npm view @ckanner/flywheel version`. Continue with these instructions once the registry has the version you intend to use. A merged source change or GitHub Release does not establish npm publication, and an older installed release may not include these interfaces.

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

Kimi subscription, ChatGPT subscription, and DeepSeek API access are separate channels. Flywheel does not log in, copy credentials, or replace the desktop application. A task using only the default execution route can skip the hard route at runtime, but it still needs its configured Plan, Execute, Review, and Judge backends.

## 2. Install

From a terminal that can find the desktop's `dsh` command:

```sh
npx -y @ckanner/flywheel@latest install
```

For an agent consuming a single JSON result, use the same command with `--json`. The installer installs its exact version globally, resolves the platform binary and workers, installs the ten bundled skills, creates global configuration only if absent, records a GUI launcher, registers the matching DSH plugin, and runs local diagnostics.

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

This is the command shape used internally, not a placeholder to paste literally. An independent npm DSH CLI cannot manage the reserved `desktop` profile. Reload according to the host's instructions; the Flywheel button appears in the desktop input area.

The worker defaults to a separate **`sdk` profile**. A desktop provider installation or login does not prove that the SDK profile has the same configuration or authorization. DeepSeek uses `deepseek-official`; native GPT routing uses the ChatGPT adapter. The checked adapter baseline can be installed with:

```sh
dsh plugin --profile sdk add dsh-plugin-chatgpt@0.1.17
```

That is a checked compatibility version, not a claim about the latest adapter. Follow its authorization documentation for the chosen profile and keep `DSH_HOME` consistent. Do not copy tokens into task files or Git.

The hard route uses the installed `dsh-subagent-codex-pro@0.8.0` dependency's public machine API directly. It does not use `subagent_codex_pro` as a native LLM provider or require a parent model to delegate. Its underlying Codex app-server still needs the local Codex command and login.

### Desktop launcher

Setup creates `~/.config/flywheel/npm-launcher` and an installation record with absolute Node/CLI paths, the checked DSH entry, and the installation-time tool PATH. The desktop plugin verifies the managed launcher before using it. A user-specified `flywheelBinary` takes precedence. Missing records can use the legacy PATH lookup; damaged records or modified managed launchers fail visibly.

After moving Node or the npm prefix, or installing previously missing tools, run `flywheel setup --no-global` in a terminal where all required tools are available, preserving your original options. Save custom launcher changes before resolving a conflict. A shell export alone does not update an already running desktop process; recheck from the panel.

## 4. Select configuration and skills

Configuration precedence is task directory, project root, global file, then embedded defaults. The first complete file wins; files do not merge. Existing configuration is preserved during upgrades. Inspect the actual source:

```sh
flywheel config inspect --cwd /absolute/path/to/repo --json
flywheel template flywheel.config.example.json
flywheel template flywheel.profiles.example.json
```

Use either complete template as a starting point and review it before writing a configuration file. The [configuration guide](configuration.md) covers safe export, fields, profile overrides, channel identity, and migration to `codex-pro`. `loops.plan` controls engine planning; the host's selected conversation model controls initial `flywheel-new` preparation.

The installer includes ten skills. Skills are optional host entry points; the desktop panel and CLI also work independently. To inspect or install them for an existing project:

```sh
flywheel skills list
flywheel skills read flywheel-run
flywheel install-skills --project /absolute/path/to/repo --json
```

Follow the host's skill-discovery rules. Copying files does not prove the host loaded them. Installer-owned unchanged snapshots can update; modified files, unowned directories, and invalid manifests are preserved and reported as conflicts.

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

A task without packets can be inspected during preparation, but a real start still needs at least one reviewed legal packet. Fix schema, directory, provider, or permission failures at their source.

## 6. Prepare a task

### Existing repository

Create or deliberately reuse a task integration branch and independent worktree **before** running `flywheel-new`. Prepare contracts and launch the engine from that same root, using a task path inside it. The [workflow guide](workflow.md) gives the branch commands, model recommendations, draft-PR timing, and exact working directory for each loop.

Use the bundled templates to prepare a real Spec and packets with actual acceptance commands, protected oracles, and recorded dry-run results. Commit the reviewed contracts before starting so packet worktrees can read them from Git. Do not create `state.json` manually.

### Disposable first-run example

For a first installation check, the packaged scaffold creates a separate Go addition task. The initial implementation is intentionally wrong; the executor must fix it and add ordinary tests without changing the frozen acceptance test. This example needs Go 1.27 or later and an unused destination directory.

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

The desktop panel accepts the same two absolute paths. A control response's `ok` means the operation succeeded; task completion is determined by `state.status`. After a startup readiness timeout, query status and read logs before attempting another start.

For a foreground run that pauses before Review, use this instead of background start:

```sh
flywheel run docs/add --pause-before review
```

`control start` does not accept `--pause-before`, and pause mode cannot be combined with `--only`. `--only execute` runs the bounded execution/L1 loop; it is not a single model call. `--only review` returns a review result without continuing into Plan or local merge. `--only verify` runs deterministic acceptance without Review/Judge or worktree creation.

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

Existing hard-route configuration remains unchanged after upgrade. To adopt the new plugin route, change the effective hard binding or profile to `harness: "codex-pro"`, preserve model/channel/thinking, inspect it, and restart. Do not use `subagent_codex_pro` as a provider.

| Symptom | First check |
|---|---|
| Missing `install` or `control` command | Published version, `flywheel version`, PATH, and any explicit desktop `flywheelBinary` |
| Install succeeds but `ready:false` | Resolve the specific `diagnostic.checks` blockers |
| Terminal works but the desktop does not | Managed launcher, explicit override, GUI PATH/profile/environment; rerun setup and check the panel |
| A configuration edit has no effect | `config inspect` source and higher-priority task/project files; restart the engine |
| Skill or launcher conflict | Preserve customizations, compare the specific files, and resolve the reported conflict without altering ownership hashes |
| Worktree creation falls back in-place | Stop if isolation is required; inspect persisted mode and preserve work before recovery |
| Cleanup is unconfirmed | Inspect task-owned processes and receipts before retrying; elapsed time alone is not proof |

Installation is not one atomic transaction across npm, DSH, and all skills. Correct the reported cause and retry; preserve completed steps and user files. Do not delete whole configuration directories or modify integrity manifests to bypass conflicts.

To uninstall, first stop running tasks, remove the desktop Flywheel plugin through the desktop's supported plugin-management interface, then uninstall the global CLI using the same prefix:

```sh
npm uninstall --global @ckanner/flywheel
```

Configuration, task ledgers, modified skills, and worktrees require deliberate separate handling; do not remove them as an incidental cleanup step. Maintainer build and publication procedures are in the [release guide](https://github.com/ckanner/flywheel/blob/main/docs/en/releasing.md). Normal users do not need npm publishing credentials or GitHub OIDC to install Flywheel.
