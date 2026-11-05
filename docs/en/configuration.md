# Model profiles, stage bindings, and diagnostics

[English README](../../README.en.md) · [中文](../configuration.md) · [Workflow](workflow.md)

Flywheel loads one complete configuration, expands its model profiles, and resolves each stage's binding before starting. Model selection is explicit configuration. Restart the engine after changing it. This guide follows the current source; existing user configuration is not automatically migrated during installation. The account-provider correction below is separate from the published 0.28.2 release.

## Configuration precedence

The discovery order is:

1. Task directory: `flywheel.config.json`.
2. Project Git root: `flywheel.config.json`.
3. Global file: `~/.config/flywheel/flywheel.config.json`.
4. Embedded defaults.

Flywheel selects the **first complete file**. It does not merge fields, profiles, loops, routes, or channels across those locations. `FLYWHEEL_CONFIG` replaces the global file path only; it does not override an existing task or project file. `XDG_CONFIG_HOME` does not change the current default path.

An unreadable higher-priority file, invalid JSON, or broken symlink is an error, not permission to use a different account's configuration. Inspect the selected source before editing:

```sh
flywheel config inspect --cwd /absolute/path/to/repo --task-dir /absolute/path/to/repo/docs/my-feature --json
```

The result includes `source`, `scope`, resolved physical paths, and stage `bindings`. Without `--task-dir`, discovery starts at the selected project and lower levels. Inspection does not create global configuration or task state, read account tokens, or invoke a model.

## Apply the subscription preset to existing configuration

The current source-level `subscriptions` preset uses ChatGPT subscriptions, Kimi Code subscriptions, and the signed-in, funded DeepSeek account in DSH, with Flash and `provider:deepseek-account`. Normal installation and upgrades preserve existing configuration, including old Z.AI/GLM routes, Kimi K3 bindings, API providers, and effort choices.

**Release boundary:** migration and `permissions.mode` shipped in 0.28.2, but that published preset still selects the `deepseek-official` API-key route. The account-default correction is post-0.28.2 source awaiting the next npm release. First check that `flywheel template flywheel.config.example.json` from your binary contains `deepseek-account`; rerunning migration with the published 0.28.2 binary does not select the new account default.

From the actual project, preview and apply the selected configuration. Add `--task-dir /repo/docs/my-task` when the task has its own configuration:

```sh
flywheel config migrate --preset subscriptions --cwd /repo --json
flywheel config migrate --preset subscriptions --cwd /repo --apply --json
flywheel config inspect --cwd /repo --json
```

The first command only previews. `--apply` backs up the original bytes and updates the effective task/project/global file; with no file, it creates global configuration. Optional `--expect-sha256` binds application to a preview. Changed source files, invalid input, unsupported file identities, and extra custom routes/stages are rejected. Budgets, verification, worktrees, retries, pricing, notes, and an explicit Plan `resident-session:false` are preserved.

The preset replaces the model bindings, removes their old profile references, and clears fallback/model mappings on the selected channels so that old paid routes are no longer automatically reachable. Unreferenced custom profiles and channels remain intact, including the old 0.28.2 API channel; it is not an automatic fallback for the account route. The preset disables the optional second opinion and reports removal of the old unverified multimodal route. Kimi K3.1 Pro/high handles Plan, frontend execution, and Garden; DeepSeek Flash/high handles ordinary execution and Flash/low handles Intake; GPT-6.1 Sol/high handles hard execution and Review; GPT-6 Astra/high handles Judge. Hard execution uses Codex Pro; Review/Judge use the DSH ChatGPT provider. Kimi's omitted-model default is `kimi-code/k3.1-pro`, without overriding an explicitly selected model. See [Reasoning effort and account selection](model-policy.md) for the rationale and the project comparison that remains unperformed.

Do not edit the target file while migrating. The lock coordinates Flywheel migration processes; the source is checked again before a complete file is atomically replaced. This is not an atomic compare-and-set against arbitrary external editors. Reports distinguish proposed configuration (`configurationStatus: proposed`), re-read effective configuration (`effective`), and an unconfirmed post-write result (`unconfirmed`). A failed backup or write must not present the proposed models as already effective.

Migration does not sign in or invoke models. Running tasks retain their startup configuration; the next engine start uses the migrated file. Review the resulting source, scope, and bindings before stopping/resuming an existing task as needed.

## Full access without per-action approval

```json
{"permissions":{"mode":"full-access"}}
```

New installation templates and the explicit subscription migration select this mode for every stage. DSH/Codex request `danger-full-access` and `approval=never`; Codex Pro maps through its public machine API; Kimi print retains native auto behavior. This grants worker access within the current operating-system user's permissions. It is independent of the desktop conversation's permission dropdown.

Existing files without this setting retain `role-based`: Plan/Execute request workspace-write, and Review/Judge request read-only. Neither policy waits for interactive approval. In restricted modes, `never` can reject an action; it does not itself grant full access. Invalid policy names and types fail before execution.

Full access preserves task boundaries, frozen acceptance, verification, and candidate identity checks. A reviewer still cannot change the candidate or acceptance criteria and obtain a valid verdict. Select `role-based` explicitly when confinement is intended; a worktree is not an OS sandbox.

## Fields and identities

| Field | Meaning | Example |
|---|---|---|
| `harness` | Process and tool-loop backend | `dsh`, `kimi`, `codex-pro`; `codex` is the explicit older CLI path |
| `provider` | Registered DSH SDK LLM adapter ID; applies to DSH bindings | `deepseek-account`, `chatgpt` |
| `channel` | A configuration label used for credentials, fallback, and accounting | `deepseek-account-sdk`, `chatgpt-sdk`, `kimi-subscription` |
| `model` | Exact model identifier passed to the selected backend | `deepseek-flash`, `kimi-code/k3.1-pro`, `gpt-6.1-sol` |
| `thinking` | Requested reasoning effort | `high` |
| `profile` | Reusable model fields inside this Flywheel configuration | `flash`, `gpt-review`, `codex-pro` |

A Flywheel model profile is different from a DSH `sdk` or `desktop` profile. The latter chooses the host's plugins and authorization configuration. A channel alias alone does not switch DSH accounts or create a separate subscription or quota pool.

For DSH, the provider resolves from `binding.provider`, then `channels[channel].provider`, then the legacy channel name. Explicit binding and channel provider values must agree. Kimi and Codex use their own login state; a DSH provider cannot be attached to those harnesses and silently ignored.

Only supported effort values should be used. The checked DSH Flash adapter accepts `off/low/high/max`; it has no `medium`. A configuration value names an intended binding, not evidence of account entitlement or measured model performance.

### DeepSeek account and API-key providers

| Access method | Provider | Credentials | Selection |
|---|---|---|---|
| Signed-in, funded DeepSeek account in DSH | `deepseek-account` | DSH's `deepseekAccount` service | Current source default; no `DEEPSEEK_API_KEY` required |
| DeepSeek API key | `deepseek-official` | DSH credentials or launch environment; default reference `DEEPSEEK_API_KEY` | An explicitly selected alternative |

The standard SDK profile layers over a [base](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/bundle/base/cordis.patch.yml) that includes the account service and both adapters. The standard desktop and SDK share the [account credential store](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/credentials/credentials-local/src/index.ts) when they use the same `DSH_HOME`. Profiles isolate plugin configuration; no token copy or switch to the reserved `desktop` profile is needed. Custom homes, credential stores, or SDK plugin sets must resolve the intended authorization source.

After login, the desktop can choose the first account model when no provider API key exists. That host selection neither controls an independent SDK session nor guarantees Flash is the first model. Flywheel explicitly sends `deepseek-account` and `deepseek-flash`; the default preset has no API-key fallback for sign-in failures, invalid tokens, or insufficient quota. Explicit user-defined fallback chains retain their existing policy. The checked sources are the [account adapter](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek-account/src/index.ts), [API adapter](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek-api-key/src/index.ts), and [host default selection](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/api/session-controller/src/index.ts).

Explicit API-key configurations remain supported. Select `deepseek-official` consistently in the relevant binding and its channel, and configure credentials through DSH's API-provider flow. Renaming a channel does not override a conflicting explicit provider. Account access does not need an additional API key.

## Start from a complete template

Inspect either bundled example:

```sh
flywheel template flywheel.config.example.json
flywheel template flywheel.profiles.example.json
```

The first uses inline bindings. The second is a complete configuration with reusable profiles. If you choose to create a project configuration, first confirm that no file needs preserving; then export one template from the target Git root. For example, Bash's noclobber option prevents replacing an existing regular file:

```sh
(set -C; flywheel template flywheel.profiles.example.json > flywheel.config.json)
```

Do not create a task file containing only a few profiles and expect it to inherit the rest from the project file. An illustrative **fragment**, to incorporate into a complete file, is:

```json
{
  "profiles": {
    "flash": {
      "harness": "dsh",
      "provider": "deepseek-account",
      "channel": "deepseek-account-sdk",
      "model": "deepseek-flash",
      "thinking": "high"
    }
  },
  "routes": {
    "default": { "profile": "flash" }
  },
  "loops": {
    "intake": { "profile": "flash", "thinking": "low" }
  },
  "channels": {
    "deepseek-account-sdk": {
      "harness": "dsh",
      "provider": "deepseek-account",
      "env-key": null
    }
  }
}
```

`loops` binds Plan, Review, Judge, Intake, and Garden. `routes` selects the Execute backend for default/frontend/hard packets. `drivers` selects real or mock drivers; it is not a provider-name mapping. The full default routing table is in the [README](../../README.en.md#default-model-routes).

### Profile override and validation rules

| Input | Effect |
|---|---|
| Stage omits a profile field | Inherit that field from the referenced profile |
| Stage explicitly supplies `model`, `thinking`, `channel`, etc. | Override only that stage |
| `thinking: ""` | Use the harness's default effort instead of re-inheriting the profile |
| `resident-session: false` | Explicitly disable the stage's resume request |
| `judge-panel: null` | Disable the optional second opinion |
| Missing/empty profile `harness`, `model`, or `channel` | Configuration error |
| Unknown profile, field, stage name, or harness | Configuration error |
| A profile references another profile | Unsupported; configuration error |
| `_note` or `_note_*` | Permitted explanatory metadata, excluded from execution bindings |

`resident-session` belongs on a stage, not in a profile. Currently only Kimi Plan can resume, and only when harness, model, channel, working directory, thinking, and permission mode match. This restores session history, not a permanently running process. Execution and review calls start fresh sessions.

Legal existing zero values, including `env-key: null`, `retry.jitter: 0`, and `worktree.enabled: false`, remain meaningful. When changing a harness or channel, also inspect any inherited provider. Use `provider: ""` when necessary to clear an explicit value before resolving the newly selected DSH channel.

## Codex Pro and migration

The 0.28.0 embedded hard route uses this binding. This is a **fragment**, not a complete replacement configuration:

```json
{
  "routes": {
    "hard": {
      "harness": "codex-pro",
      "model": "gpt-6.1-sol",
      "channel": "chatgpt-subscription",
      "thinking": "high"
    }
  }
}
```

The complete profile template uses `profiles.codex-pro` and references it from `routes.hard`. The npm dependency is exactly `dsh-subagent-codex-pro@0.8.0`. The worker imports its public `dsh-subagent-codex-pro/machine` entry and calls `runMachine` using protocol 1. The plugin itself runs Codex app-server and uses the existing Codex login.

The external tool name `subagent_codex_pro` is not a DSH provider. This route does not create a parent DSH model to make a second delegation. Missing dependencies or invalid receipts fail; they do not trigger an implicit fallback to the older CLI driver.

To migrate an existing configuration, use `config inspect` to locate the effective file. Change the hard route's inline harness or referenced profile from `codex` to `codex-pro`, retain the intended model/channel/thinking, inspect the result, and restart. Keeping `harness: "codex"` explicitly retains the older `codex exec` behavior.

`codex-pro` requires nonempty model and channel values. It does not inherit the old Codex driver's implicit model. Omitted thinking defaults to `high`; `resident-session: true` is rejected. Each execute/review call uses a new ephemeral session.

### Worker environment

| Variable | Purpose |
|---|---|
| `FLYWHEEL_DSH_WORKER` | Optional DSH worker override; the npm launcher normally resolves the matching dependency |
| `FLYWHEEL_DSH_BIN` | DSH command, otherwise `dsh` on PATH |
| `FLYWHEEL_DSH_PROFILE` | SDK profile, default `sdk`; it must not be `desktop` |
| `DSH_HOME` | Optional DSH configuration root, interpreted by DSH |
| `FLYWHEEL_KIMI_BIN` | Kimi CLI override, otherwise `kimi` on PATH |
| `FLYWHEEL_CODEX_PRO_WORKER` | Optional Codex Pro worker override; normally resolved by npm |
| `FLYWHEEL_CODEX_BIN` | Codex executable used by the plugin, otherwise `codex` on PATH |
| `CODEX_HOME` | Existing Codex configuration/login directory, interpreted by Codex |

These settings must be visible to the process starting the engine. The installer does not copy authentication data. The managed GUI launcher records executable paths and the installation-time tool PATH; a terminal export alone does not change an already running desktop process.

### Permissions, receipts, and cleanup

The plugin checks the requested model, working directory, approval policy, and sandbox mode before a turn. Flywheel examines the complete returned policy afterward. In role-based mode, workspace-write permits the workspace and standard Codex temporary directories; extra external write roots, enabled network, or unconfirmed policy make the result ineligible for successful delivery. Explicit full access verifies the matching `danger-full-access` sandbox and `dangerFullAccess` policy without claiming confinement.

**The complete policy check happens after execution. It cannot prevent access already allowed by custom Codex configuration.** An environment requiring a full policy rejection before any turn needs an upstream public pre-turn guard. Kimi's current CLI integration also provides no OS-level sandbox guarantee; a configured permission requirement should not be read as proof of its enforcement.

The `codex-pro-receipt` metric preserves the complete machine receipt, including requested and actual configuration, thread/turn IDs, process outcomes, cleanup, usage, and failures. An accepted effort request is not proof of effective server reasoning effort; `effectiveEffort` remains null.

An abnormal worker exit does not prove the plugin's separately managed processes stopped. If cleanup cannot be confirmed after startup, the engine records the evidence, escalates, and ends the run instead of automatically retrying or falling back. Before resuming, confirm the owned app-server/guardian processes have terminated. See [recovery](workflow.md#7-stop-and-recover-without-losing-evidence).

## Fallback and budgets

Default profiles do not enable automatic paid fallback across channels. An explicitly configured `channels[channel].fallback` is a bounded chain and is used only when error classification and retry policy permit it. A fallback resolves provider and model for the newly selected channel; an omitted harness inherits the current harness.

Metrics retain harness, provider, channel, and model separately. Provider remains null if the driver cannot confirm it. Missing usage and dollar cost remain unknown, including subscription calls. Numeric aggregates are lower bounds of known usage, not a complete bill.

For Codex Pro, only complete usage for the current run enters known token accounting; partial/unknown usage remains in the receipt and counts as unknown calls. The plugin does not provide actual dollar billing. Explicit user pricing plus complete usage can produce an estimate, which remains distinct from the subscription bill.

The default token budget is soft, with a daily limit of 2,000,000 tokens. A soft budget reports uncertainty. A hard budget stops when unknown usage makes compliance unverifiable. Disabling enforcement does not turn off accounting or convert unknown values to zero.

Acceptance commands use a restricted environment. If the project requires additional tool variables such as `GOCACHE` or `GOPATH`, add their names to `verify.env-allowlist` and ensure they exist in the engine's environment. Do not put secret values into task contracts or committed configuration.

## Diagnose without inference

```sh
flywheel doctor --cwd /absolute/path/to/repo --json
flywheel doctor --cwd /absolute/path/to/repo --task-dir /absolute/path/to/repo/docs/my-feature --json
flywheel doctor --cwd /absolute/path/to/repo --task-dir /absolute/path/to/repo/docs/my-feature --probe --json
```

Without a task, doctor checks all configured bindings. With a task, it validates the Spec/packets and requires Plan, Review, Judge, enabled second opinions, and routes used by the packets; other bindings can be skipped. With no packets yet, it checks the default execution route and reports that preparation must finish. A real run still needs at least one reviewed legal packet.

Local checks cover commands, versions, directories, configuration, and declared nonempty environment variables, without showing their values. `--probe` additionally initializes DSH SDK sessions, verifies accepted configuration, working directory, permissions, and normal shutdown. Identical probes are deduplicated only when their relevant identity and permissions match. The probe budget is 40 seconds plus bounded process cleanup time.

DSH's `ready.reasoningEffort` echoes the requested value after SDK configuration acceptance; it is not an independent server attestation of effective reasoning effort. A no-inference probe cannot demonstrate quality gains from a higher effort. GPT, Kimi, and Flash also have different native effort enums; see [Reasoning effort](model-policy.md).

Codex Pro checks actually import the public API and verify exact version 0.8.0/protocol 1, Node compatibility, and the Codex executable. `--probe` does not send a task through this path: account/model/effort and full runtime policy remain unverified. A successful import is not a successful model call.

| Status | Meaning |
|---|---|
| `pass` | Evidence supports this individual check only |
| `fail` | A blocker; overall `ok:false` and exit code 1 |
| `warn` | A limitation or preparation state requiring interpretation |
| `unknown` | Not verified, such as entitlement, quota, or real inference |
| `skipped` | Not required, mock mode, duplicate probe, or an unmet prerequisite |

No probe sends an inference prompt, runs L1/L2/L3, creates a worktree, or writes Flywheel task state. SDK initialization may contact a provider's catalog and write its own cache/session files; read-only here is scoped to Flywheel state and configuration. Recheck in the desktop panel because its PATH/profile/environment may differ from the terminal.

Repository references: [inline defaults](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.config.example.json), [complete profiles](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.profiles.example.json), [design](https://github.com/ckanner/flywheel/blob/main/docs/design.md), and [worker protocol](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md).
