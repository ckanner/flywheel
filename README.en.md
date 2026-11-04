# Flywheel

English | [简体中文](README.md)

Flywheel turns reviewed software requirements into bounded coding tasks with explicit acceptance criteria. A deterministic Go engine runs Plan, Execute, Review, and verification loops. Models propose plans, implement changes, and review candidates; the engine owns state, retries, budgets, workspaces, verification, local Git delivery, and stopping.

This documentation describes the **0.28.0 implementation**. The desktop integration targets the interfaces checked against **DSH Mac 0.2.1-rc.5**. Source changes and GitHub releases do not establish npm availability: check the registry before installing a version. Offline test results also do not establish that a user's Mac, desktop permissions, account, or selected model works.

## Install

You need **Node.js 22.18.0 or later**, npm, Git with a commit identity, and DSH Mac with its desktop-supplied `dsh` command. The default configuration also needs an authenticated Kimi CLI, a Codex installation and login for the hard route, and configured DeepSeek and ChatGPT providers in the DSH SDK profile. Go is needed for source builds and the Go example, but not for npm binary installation.

First check the published version:

```sh
npm view @ckanner/flywheel version
```

Once the version you intend to use is available:

```sh
npx -y @ckanner/flywheel@latest install
```

Add `--json` for a structured installation result. The installer selects one exact version for the CLI, platform binary, workers, and DSH plugin; installs the ten bundled skills; preserves existing configuration and modified skills; and records a managed desktop launcher. Binary packages cover macOS and Linux on arm64 and amd64.

`ok:true` means installation succeeded. `ready:false` means local diagnostics still found a blocker; these values can occur together. The installer does not install or log in to Kimi/Codex, configure API credentials, or prove model access. Plain `npm install --global @ckanner/flywheel` only installs package files; run `flywheel setup --no-global` afterward to perform explicit user setup.

Follow [Getting started](docs/en/getting-started.md) for prerequisites, desktop versus SDK setup, a runnable first task, and recovery. Use `--prefix "$HOME/.local"` if you need a user-owned npm prefix.

## Default model routes

These bindings are editable starting points, not a model ranking or a performance guarantee. Availability depends on the actual account, adapter, and model catalog.

| Stage or route | Harness | Provider or channel | Model | Requested thinking |
|---|---|---|---|---|
| Plan | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high` |
| Execute: `default` | DSH SDK | `deepseek-official` | `deepseek-flash` | `high` |
| Execute: `frontend` | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high` |
| Execute: `hard` | Codex Pro machine API | `chatgpt-subscription` | `gpt-6.1-sol` | `high` |
| Review | DSH SDK | `chatgpt` | `gpt-6.1-sol` | `high`, read-only |
| Judge | DSH SDK | `chatgpt` | `gpt-6-astra` | `high`, read-only |
| Intake, optional | DSH SDK | `deepseek-official` | `deepseek-flash` | `low` |
| Garden, optional | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high` |

Initial requirement preparation is separate: `flywheel-new` runs in the host's current conversation and uses its selected model. Changing `loops.plan` does not change that conversation. The [workflow guide](docs/en/workflow.md) explains preparation model choices and the later engine loops.

The default configuration has no automatic paid fallback across channels and no second judge. Flash's checked DSH effort values are `off`, `low`, `high`, and `max`; there is no `medium` in that adapter baseline. There is no default multimodal route: the current bridge has a text contract, so a model's vision capability alone does not establish attachment support.

## Codex Pro uses the actual plugin runtime

The hard route uses `harness: "codex-pro"`. Its worker directly imports the public `dsh-subagent-codex-pro/machine` API from the exact dependency **`dsh-subagent-codex-pro@0.8.0`**, using protocol 1. The plugin runs Codex app-server with the user's existing Codex installation and login. No parent DSH model is required to delegate the task again.

`subagent_codex_pro` is an external tool name, not a DSH LLM provider. Native DSH GPT calls use the `chatgpt` provider and have a different execution backend. The older `harness: "codex"` driver remains an explicit compatibility choice; the new route does not silently fall back to `codex exec`.

Existing configuration is preserved during upgrades. To adopt the new route, inspect the effective configuration and update the hard route or its model profile explicitly. See [Configuration](docs/en/configuration.md#codex-pro-and-migration).

## From a requirement to a pull request

Use one task integration branch and worktree for a deliverable. Create it **before** preparing contracts with `flywheel-new`; keep the task directory inside that same worktree. The host or user creates this outer worktree. Flywheel then manages the inner packet workspaces.

A normal workflow is:

1. Prepare and review the Spec, task packets, frozen acceptance tests, and actual dry-run evidence in the task worktree.
2. Commit the substantive contract files. Push the task branch and open a task-level Draft PR through the host or GitHub CLI.
3. Start Flywheel with the task worktree as `cwd` and its own task directory as `taskDir`.
4. Let the engine run bounded execution, L1 acceptance, and independent review for each packet; then run milestone L2/L3 and Judge.
5. Push the completed result, verify the current PR head, review it, and merge under the repository's chosen policy.

For example, from an already prepared task worktree:

```sh
flywheel config inspect --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --probe --json
flywheel control start --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
```

Packet delivery is a **local Git operation**. After evidence matches the current candidate, the engine commits the packet and fast-forwards it into the branch checked out at `cwd`. With the recommended layout, that is the task integration branch. Flywheel does not itself push, create GitHub PRs, wait for remote CI, merge remote PRs, or publish releases. Those operations belong to the authorized host or user. See [Workflow](docs/en/workflow.md) for commands, directory ownership, and merge timing.

### Desktop task controls

The current source adds a native **Flywheel** conversation view alongside Chat and Trajectory, preserving DSH's composer, model selector, and right sidebar. Published `0.28.0` still uses the old composer popup; the native view requires a build containing this fix. Select a prepared task directory and its repository to inspect progress, intervention, effective models, and diagnostics, or explicitly start, refresh, or stop it.

The view retains session-scoped drafts while the host retains the session binding and shows snapshots with their refresh time. It does not discover tasks or poll automatically. An unconfirmed start/stop outcome requires a successful refresh of the same task before further mutations; closing or switching the view does not stop the independent engine. DSH's trajectory keeps its parent-session records, without automatically merging independent worker history. Desktop capabilities and SDK worker policy have separate scopes; see the [plugin guide](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.en.md#use-the-desktop-controls).

## Verification and operating boundaries

L1 validates a packet; L2/L3 validate integration and user paths at milestone scope. Review examines the actual candidate and acceptance evidence independently of the executor transcript. Candidate or contract changes invalidate prior evidence. Completion requires verified Git delivery and persisted state; an agent's claim that it finished is insufficient.

Keep these boundaries in mind:

- **Worktrees isolate Git work, not process permissions.** Packet worktree creation can currently fall back to in-place execution. There is no engine option requiring isolation in every case. Skills check the recommended task location; direct CLI/GUI use does not execute those skill checks.
- **Permissions depend on the harness.** DSH and Codex paths request explicit permissions. Kimi's current CLI integration does not provide an OS-level read-only or workspace sandbox guarantee.
- **Codex Pro's complete policy check is after the run.** The plugin checks model, directory, approval, and sandbox mode before a turn. Flywheel validates the full returned policy afterward, allowing the workspace and standard Codex temporary directories. Additional external write roots, enabled network access, or unconfirmed policy prevent acceptance. This check cannot prevent access already allowed by custom Codex configuration.
- **Cleanup needs evidence.** A worker exiting does not prove that the plugin's separate app-server or guardian processes stopped. An unconfirmed cleanup result escalates and stops the run; inspect and resolve it before resuming.
- **Unknown usage remains unknown.** Missing usage or monetary cost is not zero. Soft budgets report uncertainty; hard budgets stop when unknown usage makes the limit unverifiable.
- **Preflight is not a model smoke test.** `doctor --probe` initializes supported SDK sessions without an inference prompt. Codex Pro checks import/version/protocol and local commands, leaving actual model, authorization, and full runtime policy unverified.

The engine alone writes `state.json`, atomically with revision checks. Do not edit it to force progress or delete retained work to make a retry look clean.

## Documentation

| Guide | Contents |
|---|---|
| [Getting started](docs/en/getting-started.md) | Install, configure, diagnose, run a small task, stop, and recover |
| [Workflow](docs/en/workflow.md) | Preparation models, task and packet worktrees, Draft PRs, completion, and merge policy |
| [Configuration](docs/en/configuration.md) | File precedence, profiles, providers, channels, Codex Pro, and diagnostics |
| [Chinese documentation](README.md) | Default language and additional implementation detail |
| [System design](https://github.com/ckanner/flywheel/blob/main/docs/design.md) (Chinese) | Authoritative design and invariants |
| [DSH plugin](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.en.md) / [protocol](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md) (protocol in Chinese) | Desktop control surface, workers, permissions, and lifecycle |
| [Release guide](https://github.com/ckanner/flywheel/blob/main/docs/en/releasing.md) | Candidate artifacts, registry verification, and release recovery |
| [Contributing](https://github.com/ckanner/flywheel/blob/main/docs/contributing.md) | Development workflow, validation, and contribution boundaries |

The English guides cover current operation. Historical design and validation ledgers remain in their original language. Source and design links are intended for browsing the repository; the npm package contains selected runtime and user documentation rather than the entire repository.

## Development

Read [AGENTS.md](https://github.com/ckanner/flywheel/blob/main/AGENTS.md) and the [design](https://github.com/ckanner/flywheel/blob/main/docs/design.md) before changing the engine. Source development needs Go 1.27 or later and Node.js 22.18.0 or later. Relevant local gates include:

```sh
make quality-gate
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(DSH|CodexPro)NodeWorkerOfflineBridge$' -count=1
npm test
```

These commands cover local code and protocol behavior. Real desktop UI, macOS permissions, account access, and model inference require separate, recorded testing in the intended environment.

Maintainers can build the six packages from a tagged source checkout and use the repository's `npm run release:local` command for interactive publication. No prebuilt archive is required. The command runs candidate and registry installation checks before promoting `latest`; see [local publication from source](https://github.com/ckanner/flywheel/blob/main/docs/en/releasing.md#local-build-and-publication-from-source) for the exact worktree and authentication steps.

## License status

The package currently declares **`UNLICENSED`**. An open-source license has not been selected; these English documents do not change the repository's visibility or licensing status.
