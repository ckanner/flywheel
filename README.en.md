# Flywheel

English | [简体中文](README.md)

Flywheel turns reviewed software requirements into bounded coding tasks with explicit acceptance criteria. A deterministic Go engine runs Plan, Execute, Review, and verification loops. Models propose plans, implement changes, and review candidates; the engine owns state, retries, budgets, workspaces, verification, local Git delivery, and stopping.

Current source supports **Auto, Quick, and Full** workflows. `flywheel-task` prepares and launches an authorized goal. A qualified small task uses Execute, all required checks, one independent final Review, and persisted Git delivery; larger or uncertain tasks retain the full loop. Existing tasks remain Full. Workflow selection does not change model bindings, permissions, or delivery authorization.

Session policy supports **fresh/scoped**: new templates explicitly choose scoped, while old configurations default to fresh. Scoped mode can continue compatible history for the same task, role, and scope after a trusted normal terminal outcome; independent re-review still checks the complete current candidate. Both modes journal main-task Codex Pro, Claude Code Pro, DSH, and Kimi calls. Unknown dispatch, outcome, or cleanup blocks execution and cannot be bypassed by switching modes. P3 adds public typed lifecycle events, durable acknowledgements, and read-only Codex/Claude receipt recovery. P4 adds role overrides, bounded quality rebuilds, and paired evaluation tools. Real-account quality, cache, and performance results remain unverified; offline tests cannot establish them. See [Interrupted calls and context quality](docs/en/session-recovery.md). See [Session policy](docs/en/configuration.md#session-policy-history-reuse-and-invocation-safety).

This source targets **0.29.1**, including account defaults, account-error handling, and the current session directory. Check the npm registry before installing; merging source does not update an installed package. The desktop integration covers the checked **DSH Mac 0.2.1-rc.5 and 0.2.2-rc.1** interfaces. Version 0.28.4 declares only these two runtime versions. Check the registry before installing a version. Offline tests do not establish a live user Mac, account, or model pass.

The adaptive workflow and `flywheel-task` require a matching CLI/plugin release containing these changes. Check `flywheel control launch --help`; the older installation commands alone do not establish that this new interface is available.

Current source also supports an optional [Claude Code Pro backend](docs/en/claude-code-pro.md), with explicit models/efforts and scoped role history. No default stage selects it. It requires the companion 0.4.0 machine API and explicit full access; the 0.3.1 desktop tool is not a compatible substitute.

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

Add `--json` for a structured installation result. The installer selects one exact version for the CLI, platform binary, workers, and DSH plugin; installs that version's bundled skills (eleven in current source); preserves existing configuration and modified skills; and records a managed desktop launcher. Binary packages cover macOS and Linux on arm64 and amd64.

`ok:true` means installation succeeded. `ready:false` means local diagnostics still found a blocker; these values can occur together. The installer does not install or log in to Kimi/Codex or prove model access. Current source defaults reuse the signed-in DeepSeek account in DSH; API-key credentials are needed only for an explicitly selected API provider. Plain `npm install --global @ckanner/flywheel` only installs package files; run `flywheel setup --no-global` afterward to perform explicit user setup.

Follow [Getting started](docs/en/getting-started.md) for prerequisites, desktop versus SDK setup, a runnable first task, and recovery. Use `--prefix "$HOME/.local"` if you need a user-owned npm prefix.

## Default model routes

These bindings are editable starting points, not a model ranking or a performance guarantee. Availability depends on the actual account, adapter, and model catalog.

| Stage or route | Harness | Provider or channel | Model | Requested thinking |
|---|---|---|---|---|
| Plan | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high` |
| Execute: `default` | DSH SDK | `deepseek-account` | `deepseek-flash` | `high` |
| Execute: `frontend` | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high` |
| Execute: `hard` | Codex Pro machine API | `chatgpt-subscription` | `gpt-6.1-sol` | `high` |
| Review | DSH SDK | `chatgpt` | `gpt-6.1-sol` | `high` |
| Judge | DSH SDK | `chatgpt` | `gpt-6-astra` | `high` |
| Intake, optional | DSH SDK | `deepseek-account` | `deepseek-flash` | `low` |
| Garden, optional | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high` |

Initial requirement preparation is separate: `flywheel-task` and the preparation-only `flywheel-new` use the host's current conversation model. Changing `loops.plan` does not change that conversation. Quick reuses the configured Execute route and Review binding; it does not change effort or add a paid fallback. The [workflow guide](docs/en/workflow.md) explains preparation and the later engine loops.

The default configuration has no automatic paid fallback across channels and no second judge. Flash's checked DSH effort values are `off`, `low`, `high`, and `max`; there is no `medium` in that adapter baseline. There is no default multimodal route: the current bridge has a text contract, so a model's vision capability alone does not establish attachment support.

Seven high bindings and Intake low are an engineering baseline, without a project high/xhigh/max comparison. GPT supports xhigh/max; Kimi K3.1 Pro natively supports low/high/max. Hard execution and high-impact Judge are useful first xhigh evaluation candidates. See [Reasoning effort](docs/en/model-policy.md) for evidence and limits.

The standard SDK already includes the DeepSeek account provider and account service. Under the same `DSH_HOME`, it shares the standard desktop account credential store without copying tokens or adding an API key. Desktop default-model selection does not control an independent SDK session, so Flywheel explicitly requests `deepseek-account` and `deepseek-flash`. Version 0.28.2 still selects the API provider; upgrade to 0.28.3 or later and use [explicit migration](docs/en/configuration.md#apply-the-subscription-preset-to-existing-configuration) to adopt the account default. Custom API-key configuration remains supported.

## Codex Pro uses the actual plugin runtime

The hard route uses `harness: "codex-pro"`. Its worker directly imports the public `dsh-subagent-codex-pro/machine` API from the exact dependency **`dsh-subagent-codex-pro@0.9.0`**, using machine protocol 1 and lifecycle version 1. The reviewed source candidate is bundled with Flywheel; [provenance and hashes](plugins/dsh/vendor/README.md) identify its exact bytes. The plugin runs Codex app-server with the user's existing Codex installation and login. No parent DSH model is required to delegate the task again.

`subagent_codex_pro` is an external tool name, not a DSH LLM provider. Native DSH GPT calls use the `chatgpt` provider and have a different execution backend. The older `harness: "codex"` driver remains an explicit compatibility choice; the new route does not silently fall back to `codex exec`.

Existing configuration is preserved during upgrades. To adopt the new route, inspect the effective configuration and update the hard route or its model profile explicitly. See [Configuration](docs/en/configuration.md#codex-pro-and-migration).

## From a requirement to a pull request

Describe an authorized goal in DSH without filling directory fields first:

```text
/flywheel-task Fix the identified empty-list behavior, add regression coverage, and deliver the verified change. mode=auto
```

| Choice | Use | Engine behavior |
|---|---|---|
| Auto | Let the engine select from researched contract facts | Quick when eligibility is established; otherwise Full |
| Quick | One milestone and packet, known implementation, complete checks | Execute, required checks, independent final Review, Git delivery; upgrade on a gap or new risk |
| Full | Multiple units, unresolved design, significant impact, or explicit choice | Plan, packet Execute/Review, integrated L2/L3, Judge |

Quick retains repository gates, applicable L2/L3, defect sampling, and persistence checks. Its first-success path has two stage-agent calls, excluding host preparation, internal tool turns, and retries. A legacy task remains Full; restarting does not reclassify or downgrade an existing Full task.

Use one task integration branch and worktree for a deliverable. The host creates or deliberately reuses it before preparing contracts and keeps the task directory inside it. Flywheel manages the inner packet workspaces. `/flywheel-task` prepares and launches, `/flywheel-new` prepares a proposal without starting, and `/flywheel-run` starts or resumes an existing task. Kimi uses the `/skill:` prefix for the same skill names. Existing authorizations remain valid; an explicit request to review a proposal first remains a stopping point.

A normal workflow is:

1. Prepare the Spec, task packets, workflow facts, frozen acceptance tests, and actual dry-run evidence in the task worktree; honor any requested proposal review.
2. Commit the substantive contract files. Push the task branch and open a task-level Draft PR through the host or GitHub CLI.
3. Launch with the task worktree as `cwd`, its own task directory as `taskDir`, and the requested mode. DSH's `flywheel_launch` associates this concrete task with the initiating session's view.
4. Let the Go engine select and run Quick or Full, including all applicable checks and independent review against the same candidate.
5. Push the completed result, verify the current PR head, review it, and merge under the repository's chosen policy.

For example, from an already prepared task worktree:

```sh
flywheel config inspect --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --probe --json
flywheel control launch --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --mode auto
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
```

`control launch` defaults to Auto and consumes prepared contracts; it does not invent requirements or create the outer task branch. `control start` and foreground `run` preserve the contract/persisted selection when `--mode` is omitted. Their optional mode is `auto|quick|full`; Go still enforces eligibility and one-way upgrade rules. `--only execute` is a partial loop, not Quick or complete delivery.

Packet delivery is a **local Git operation**. After evidence matches the current candidate, the engine commits the packet and fast-forwards it into the branch checked out at `cwd`. With the recommended layout, that is the task integration branch. Flywheel does not itself push, create GitHub PRs, wait for remote CI, merge remote PRs, or publish releases. Those operations belong to the authorized host or user. See [Workflow](docs/en/workflow.md) for commands, directory ownership, and merge timing.

### Desktop task controls

From `0.28.1`, the native **Flywheel** conversation view appears alongside the built-in views (after Statistics from 0.28.5), preserving DSH's composer, model selector, and right sidebar. Version `0.28.0` uses the old composer popup; upgrade the CLI and desktop plugin together. **Task files directory** selects the prepared `docs/<slug>/`; **Project directory** is its project/integration worktree and may stay blank. From 0.28.3, Flywheel reads DSH's `workingDirectory.get(session)`, following session directory changes. Version 0.28.2 still uses the original header directory: explicitly fill the project path after switching worktrees. Configuration and diagnostics can omit the task; start, refresh, and stop require an existing task directory.

DSH tool launch/start associates the exact task with the initiating session. The view shows requested and effective modes, reasons, stage applicability, and upgrades. It does not scan directories to guess tasks or poll tasks started separately in a terminal; manual opening remains supported. An unconfirmed start/stop outcome requires a successful refresh of the original task before further mutations; late responses cannot overwrite a different session or newer binding. Closing the view does not stop the independent engine. DSH's trajectory keeps its parent-session records, without automatically merging worker history. See the [plugin guide](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.en.md#use-the-desktop-controls) for association lifetime and runtime boundaries.

## Verification and operating boundaries

L1 validates a packet; L2/L3 validate integration and user paths. Quick runs applicable L2/L3 in the actual packet candidate before its final Review, which covers every requirement and acceptance sufficiency. Full keeps the integrated milestone gates and Judge. Review is independent of the executor transcript. Candidate, contract, verification configuration, and effective workflow policy changes invalidate incompatible evidence. Completion requires verified Git delivery and persisted state; an agent's claim that it finished is insufficient.

Keep these boundaries in mind:

- **Worktrees isolate Git work, not process permissions.** Packet worktree creation can currently fall back to in-place execution. There is no engine option requiring isolation in every case. Skills check the recommended task location; direct CLI/GUI use does not execute those skill checks.
- **Permissions are explicit.** New templates and the subscription migration select `permissions.mode:full-access` for every stage, using DSH/Codex full access with approval never and native Kimi print auto. Existing configurations without this field retain role-based permissions. Kimi does not provide an OS confinement guarantee; task and acceptance identity checks remain enforced.
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
| [Session lifecycle proposal](docs/en/plans/2026-11-07-session-lifecycle.md) | Original P1–P4 proposal; implemented recovery and quality policy are documented in the current guides |
| [Configuration](docs/en/configuration.md) | File precedence, profiles, providers, channels, Codex Pro, and diagnostics |
| [Reasoning effort](docs/en/model-policy.md) | Supported levels, high baseline, evaluation candidates, and account-provider choice |
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
