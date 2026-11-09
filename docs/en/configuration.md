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

The result includes `source`, `scope`, resolved physical paths, `sessions`, and stage `bindings`. Each binding's `session` reports selected mode, adapter support, a reason, and `historyRecoveryTested:false`. Without `--task-dir`, discovery starts at the selected project and lower levels. Inspection does not create global configuration or task state, read account tokens, or invoke a model.

## Apply the subscription preset to existing configuration

The current source-level `subscriptions` preset uses ChatGPT subscriptions, Kimi Code subscriptions, and the signed-in, funded DeepSeek account in DSH, with Flash and `provider:deepseek-account`. Normal installation and upgrades preserve existing configuration, including old Z.AI/GLM routes, Kimi K3 bindings, API providers, and effort choices.

**Release boundary:** migration and `permissions.mode` shipped in 0.28.2, but that published preset still selects the `deepseek-official` API-key route. The account-default correction is available from 0.28.3. Check the registry version before installing. First check that `flywheel template flywheel.config.example.json` from your binary contains `deepseek-account`; rerunning migration with the published 0.28.2 binary does not select the new account default.

From the actual project, preview and apply the selected configuration. Add `--task-dir /repo/docs/my-task` when the task has its own configuration:

```sh
flywheel config migrate --preset subscriptions --cwd /repo --json
flywheel config migrate --preset subscriptions --cwd /repo --apply --json
flywheel config inspect --cwd /repo --json
```

The first command only previews. `--apply` backs up the original bytes and updates the effective task/project/global file; with no file, it creates global configuration. Optional `--expect-sha256` binds application to a preview. Changed source files, invalid input, unsupported file identities, and extra custom routes/stages are rejected. Budgets, verification, worktrees, retries, pricing, notes, an explicit Plan `resident-session:false`, and the existing `sessions` policy or its omission are preserved. Model migration does not opt an existing file into scoped history.

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

## Session policy, history reuse, and invocation safety

```json
{"sessions":{"mode":"scoped","policy-version":1}}
```

Both new-installation templates explicitly select `scoped`. An existing file without `sessions` normalizes to `fresh`, policy version 1; upgrades do not rewrite it. An explicit policy must contain both fields. Unknown modes, future versions, nulls, typos, and incomplete objects fail before a model starts. The [session policy schema](../../templates/session-policy.schema.json) is the field contract.

`sessions.mode` controls history reuse; it does not disable invocation safety records. Main-task Plan, Execute, Review, and Judge calls to Codex Pro, Claude Code Pro, DSH, and Kimi journal intent and trusted terminal receipts in both modes. `fresh` disables general role history reuse, retains each backend's native new-call behavior, and preserves existing Kimi Plan `resident-session` behavior. `scoped` can continue compatible history after a trusted normal terminal outcome. Other adapters and mock retain their existing invocation path. Intake and Garden are outside this first-phase role session policy.

| Role | History scope and responsibility |
|---|---|
| Plan | Same task and milestone; only permitted task-contract revisions |
| Execute | Same task and packet; ordinary repairs may continue executor history |
| Review | Same task, packet, and review responsibility; isolated from the executor and still inspecting the complete current candidate |
| Judge / second opinion | Same task and milestone, with separate histories for primary and panel judgments |

Quick final Review and Full packet Review have different completion requirements and cannot exchange old histories or verdicts. Quick's first-success path still has two stage-agent calls, Execute and final Review. Go owns all session selection, continuation, rebuilding, and accounting. Skills, models, and desktop controls do not scan backend history to guess a session.

An explicit Plan `resident-session:false` still disables that role's history reuse under scoped mode; omission is different from an explicit opt-out. `resident-session:true` remains the legacy Kimi Plan option, not a cross-harness switch. Codex Pro still rejects it; select scoped Codex Pro history through `sessions`.

Reuse identity includes the actual task and working directories, role and scope, harness, provider, channel, model, thinking, permissions, and backend runtime references. Backend home/profile, worker, and command references reflect the child configuration actually used; only the names of injected credentials are included, never their values. Renaming a profile without changing its resolved binding does not invalidate history. A changed backend, account channel, model, permission, or working directory selects a new history. Changes to the Spec goal, non-goals, requirement identity, or packet goal, body, boundaries, and requirement mapping also invalidate the previous task semantics. Milestone-scoped Plan, Judge, and second opinions also check the milestone title and the identities and goal fields of its constituent packets. Compatible acceptance revisions can retain already-consumed history, with the current contract and evidence supplied again.

These non-secret references do not attest an account's identity or the contents of the referenced files. Replacing configuration, account authorization, or an executable in place under the same path is not guaranteed to change this policy's binding. Credential values are not read to compare accounts. Accepted configuration and effective permissions remain limited to what the current adapter receipt can establish.

Completed but unconsumed receipts have stricter rules: Review and Judge must match the original contract, candidate, and verification evidence. Running verification again cannot replace that evidence. Plan may update its permitted contract files, but replay remains bound to the complete post-call candidate and durable original before/after guards. A forbidden reviewer or planner mutation cannot become an accepted result after restart.

`state.sessions` records each scope's generation, action/reason, and latest invocation facts. One atomically replaced `.sessions` receipt slot per scope bounds recovery storage; raw receipts are excluded from Git. Recovery verifies the saved intent, receipt digest, and identity before consuming the same result, without another model request or duplicate billing. Budgets constrain new dispatches; consuming an already completed trusted receipt is not a new call. Metrics retain earlier audit facts.

The single-slot limit applies only to Flywheel's recovery receipt. Native thread and Session histories follow the backend's retention rules. Rebuilding a role history or shutting down its worker does not automatically delete earlier backend histories.

An unknown dispatch, outcome, or cleanup state blocks further execution. Changing to fresh mode, changing adapters, or restarting cannot bypass it. Process exit alone does not establish a known model outcome. `resume-candidate` only permits an adapter to attempt normal continuation; it does not prove recovery after an arbitrary crash. Main-task calls through Codex Pro 0.9.0 and Claude Code Pro 0.4.0 use outer worker v3: public typed lifecycle events, ACKs that wait for Go's durable checkpoint, and a read-only recovery entry that never sends a new turn. Codex can gate dispatch after verified `thread-ready`; Claude's native init follows prompt dispatch and cannot provide equivalent pre-dispatch configuration evidence. Both retain uncertainty when evidence is insufficient. See [interrupted invocation reconciliation](session-recovery.md).

`status` and `control status` expose role, scope, generation, action/reason, latest status, dispatch/outcome/cleanup, history-reuse selection, and usage status. The compact projection does not read receipt contents or conversation history and never changes the ledger. Legacy tasks without session records are not assumed to have recovery eligibility.

### Role overrides and context quality

Optional `sessions.roles` selects `fresh` or `scoped` separately for `plan`, `execute`, `review`, `judge`, and `judge-panel`. Omitted roles inherit `mode`. An explicit `roles.plan:"fresh"` also disables the legacy Kimi Plan reuse shortcut; an explicit `resident-session:false` still prevents Plan reuse. Intake and Garden do not accept these overrides. This example changes history policy without changing model or account allocation:

```json
{
  "sessions": {
    "mode": "scoped",
    "policy-version": 1,
    "roles": {"review": "fresh"},
    "quality": {
      "max-confirmed-turns": 0,
      "max-unproductive-turns": 0,
      "max-run-input-tokens": 0
    }
  }
}
```

All quality thresholds default to `0`, meaning disabled. Deployment defaults are not tuned from unmeasured benefits. A positive value selects these mechanically checked limits:

| Field | Observation that can trigger rebuilding |
|---|---|
| `max-confirmed-turns` | Safely ended, dispatched invocations in this role history generation reach the limit |
| `max-unproductive-turns` | Consecutive Execute calls leave the authoritative before/after candidate fingerprint unchanged; this is a candidate observation, not semantic judgement |
| `max-run-input-tokens` | The latest complete per-run input usage reaches the limit; aggregate run input is a workload measure, not native context-window occupancy |

A quality rebuild requires the previous result to be consumed, accounted, and known safe in dispatch, outcome, and cleanup, with history otherwise eligible for continuation. Proven non-dispatch does not increment confirmed turns. Recovery deduplicates observations by invocation ID. Partial/unknown usage is not filled with zero and cannot trigger the input threshold by itself. Original budgets, attempts, stagnation limits, work, and independent acceptance remain intact. A rebuild hands over this role's generation/invocation references, reason, and current authoritative materials; it does not copy another role's private transcript or start an additional model compression call.

`state.sessions.records[*].quality` and corresponding metrics expose confirmed turns, unchanged-candidate observations, latest input measurement and completeness, generation rebuild count, and reason. The same schema bounds values and rejects unknown fields before dispatch. See [paired session evaluation](session-evaluation.md) for executable real-task comparisons, threshold selection, and unresolved-result handling.

## Workflow policy and recovery

Workflow policy chooses stages. Model profiles, loops, routes, and `permissions.mode` independently choose each stage's backend and permissions. **Put workflow in the Spec's YAML frontmatter, not in `flywheel.config.json`.** The unified `flywheel-task` entry defaults to Auto and records researched facts before Go validates them. This example shows the fields; replace the content with evidence from the actual task:

```yaml
workflow:
  version: 1
  mode: auto
  reason: "The failure is located and follows an existing implementation and regression pattern"
  scope-clear: true
  implementation-known: true
  verification-complete: true
  review-sufficient: true
  evidence:
    - "Inspected the implementation, callers, and a regression test reproducing this failure"
  risks: []
  not-applicable:
    L2: "No cross-module protocol changes; L1 covers the calling boundary"
    L3: "No separate UI or external-service path; the local function contract directly verifies behavior"
```

Only `version` and `mode` are required fields; missing assessment data does not qualify a task for Quick. Quick needs one milestone and packet, all four fact flags true, nonempty reason/evidence, no risks, every Spec requirement referenced by the packet with a nonempty verification description, at least one L1 command, and reasons for absent L2/L3. Go validates structure and declarations; a model's low-risk label is not proof of semantic safety. Insufficient evidence or declared risks selects Full, and execution/review can still discover new risks.

```sh
flywheel control launch --task-dir /repo/docs/task --cwd /repo --mode auto
flywheel control start --task-dir /repo/docs/task --cwd /repo --mode full
flywheel run /repo/docs/task --mode full
flywheel control status --task-dir /repo/docs/task --cwd /repo
```

`launch` defaults to Auto. `start` and `run` preserve the contract or persisted choice when `--mode` is omitted. Status/stop do not switch modes. Legacy contracts and existing state without workflow remain Full. Recovery retains the selected policy: Auto does not reclassify, and Full never downgrades to Quick. A new risk, verification gap, or explicit Full request upgrades Quick through a persisted checkpoint before the full loop continues. Unknown workflow versions fail explicitly. Never edit state or create a replacement task to reset budget or evidence.

`state.workflow` stores requested/effective policy, reason, evidence, risks, version, selection time, Plan/Judge applicability, and upgrades. A stage marked not applicable is not a passing check. Effective policy and completion requirements participate in evidence identity, so a Quick final Review cannot stand in for Full's Judge after upgrade. New packets can declare `workflow-risks: []` in `done-report-format`; the executor reports facts through that string array rather than editing policy or state.

Quick reuses existing Execute and Review bindings without changing effort, accounts, fallbacks, or full-access settings. All required checks remain, and applicable L2/L3 run in the actual candidate before final Review. The reviewer checks every requirement; the first-success path uses two stage-agent calls, Execute and final Review. See the [Spec schema](../../templates/spec.schema.json), [state schema](../../templates/state.schema.json), and [workflow guide](workflow.md).

## Optional Claude Code Pro

The `claude-code-pro` harness and unreferenced `claude-sonnet` profile can be selected for any configured loop or Execute route. See the [Claude Code Pro guide](claude-code-pro.md) for model/effort choices, optional-peer installation, full-access-only permissions, continuation, and durable receipt recovery. The provider field remains specific to DSH native adapters; default stage allocation is unchanged.

## Fields and identities

| Field | Meaning | Example |
|---|---|---|
| `harness` | Process and tool-loop backend | `dsh`, `kimi`, `codex-pro`, `claude-code-pro`; `codex` is the explicit older CLI path |
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
| Plan `resident-session: false` | Explicitly disable Plan history reuse; loading, profile expansion, and effective config export preserve its presence |
| `judge-panel: null` | Disable the optional second opinion |
| Missing/empty profile `harness`, `model`, or `channel` | Configuration error |
| Unknown profile, field, stage name, or harness | Configuration error |
| A profile references another profile | Unsupported; configuration error |
| `_note` or `_note_*` | Permitted explanatory metadata, excluded from execution bindings |

`resident-session` is a legacy stage option, not a profile field. It continues to control the existing Kimi Plan behavior, and an explicit false also opts Plan out of scoped history. Other roles use the top-level `sessions` policy. Restoring history does not require a permanently running model process.

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

The complete profile template uses `profiles.codex-pro` and references it from `routes.hard`. The worker consumes the exact public `dsh-subagent-codex-pro@0.9.0` package and its `dsh-subagent-codex-pro/machine` entry. The current candidate bundles the real package built from reviewed upstream source; this does not claim that 0.9.0 has been published to the registry. The plugin runs Codex app-server and uses the existing Codex login. Claude remains a separate exact `0.4.0` optional peer and is unnecessary when that backend is not selected.

The external tool name `subagent_codex_pro` is not a DSH provider. This route does not create a parent DSH model to make a second delegation. Missing dependencies or invalid receipts fail; they do not trigger an implicit fallback to the older CLI driver.

To migrate an existing configuration, use `config inspect` to locate the effective file. Change the hard route's inline harness or referenced profile from `codex` to `codex-pro`, retain the intended model/channel/thinking, inspect the result, and restart. Keeping `harness: "codex"` explicitly retains the older `codex exec` behavior.

`codex-pro` requires nonempty model and channel values. It does not inherit the old Codex driver's implicit model. Omitted thinking defaults to `high`; `resident-session: true` is rejected.

Main-task fresh and scoped calls both use outer worker protocol 3. Fresh selects ephemeral history; scoped selects persistent history and an exact thread ID for continuation. Dispatch waits for the relevant lifecycle checkpoints and Go's durable ACK. Public machine requests/terminal receipts remain protocol 1, with separately versioned lifecycle contracts. The legacy outer v1/v2 entries remain for low-level compatibility and do not acquire the main-task durable recovery guarantees. Resume must accept the same thread; busy, missing-history, and generic failures cannot be classified as safe cold-start opportunities from error text. See [interrupted invocation reconciliation](session-recovery.md).

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

The Codex plugin checks the requested model, working directory, approval policy, and sandbox mode before a turn. Its public `thread-ready` checkpoint exposes the full effective policy; Flywheel checks it before durably acknowledging permission to continue toward dispatch. In role-based mode, workspace-write permits the workspace and standard Codex temporary directories; extra external write roots, enabled network, or unconfirmed policy are rejected before the new turn. The terminal receipt is checked again. Explicit full access verifies the matching `danger-full-access` sandbox and `dangerFullAccess` policy without claiming confinement.

Claude's native initialization follows prompt submission, so its adapter does not claim a pre-dispatch `thread-ready` guarantee. Claude requires explicitly configured full access; native approval modes cannot stand in for an OS sandbox. Kimi's current CLI integration also provides no OS-level sandbox guarantee. Adapter capabilities and actual lifecycle evidence must be evaluated separately.

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

Without a task, doctor checks all configured bindings. With a task, it validates the Spec/packets and uses the engine's policy resolver: fresh tasks follow the Spec, existing tasks retain their saved policy, and legacy state remains Full. Full requires Plan, Review, Judge, enabled second opinions, and the used execution routes. Qualified Quick requires Review and the actual routes, marking Plan/Judge/second opinion skipped. Malformed workflow state blocks SDK probes without mutating or reclassifying the task. With no packets yet, it checks the default route and reports incomplete preparation. A real run still needs at least one valid packet for an authorized scope.

Doctor does not accept the runtime `--mode` override; it reports the Spec/saved state. An explicit launch override is resolved and persisted by the engine when starting. Configuration inspection lists bindings without claiming every configured stage will be called.

Local checks cover commands, versions, directories, configuration, and declared nonempty environment variables, without showing their values. `--probe` additionally initializes DSH SDK sessions, verifies accepted configuration, working directory, permissions, and normal shutdown. Identical probes are deduplicated only when their relevant identity and permissions match. The probe budget is 40 seconds plus bounded process cleanup time.

DSH's `ready.reasoningEffort` echoes the requested value after SDK configuration acceptance; it is not an independent server attestation of effective reasoning effort. A no-inference probe cannot demonstrate quality gains from a higher effort. GPT, Kimi, and Flash also have different native effort enums; see [Reasoning effort](model-policy.md).

Codex Pro checks actually import the public API and verify exact version 0.9.0/protocol 1, typed lifecycle/recovery exports, Node compatibility, and the Codex executable. Binding fields `durableJournal`, `readOnlyRecovery`, and `threadReadyBeforeDispatch` describe pinned adapter capabilities; Claude's last field is false. `historyRecoveryTested` remains false. `--probe` sends no model task and does not turn a successful import into evidence of account access, effective permissions, or recovery of a particular saved history.

| Status | Meaning |
|---|---|
| `pass` | Evidence supports this individual check only |
| `fail` | A blocker; overall `ok:false` and exit code 1 |
| `warn` | A limitation or preparation state requiring interpretation |
| `unknown` | Not verified, such as entitlement, quota, or real inference |
| `skipped` | Not required, mock mode, duplicate probe, or an unmet prerequisite |

`sessions.policy` confirms only the configured mode and version. Each applicable real binding's `.session` check reports selection, reason, and adapter support, while history recovery remains unknown. Doctor does not restore user histories, send a test turn, or treat a successful worker import as proof of durability.

No probe sends an inference prompt, runs L1/L2/L3, creates a worktree, or writes Flywheel task state. SDK initialization may contact a provider's catalog and write its own cache/session files; read-only here is scoped to Flywheel state and configuration. Recheck in the desktop panel because its PATH/profile/environment may differ from the terminal.

Repository references: [inline defaults](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.config.example.json), [complete profiles](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.profiles.example.json), [design](https://github.com/ckanner/flywheel/blob/main/docs/design.md), and [worker protocol](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md).

### Editing configuration in the workflow console

The Global / project configuration view first reads an explicit scope and file revision. Edit model bindings, role history policies, budget, timeouts, retries or the global permission policy, then review section changes before saving. Concurrent edits reject replacement; reread an unconfirmed save before proceeding. New project files use the current version's complete default template. Task files override project files, which override global configuration.

Advanced callers send JSON on stdin to `flywheel control config-edit --cwd /absolute/repo`, with `{"scope":"project"}`. Save through `control config-save` using scope, the returned revision and values containing changed sections only. With reset=true, replacement starts from the full embedded default template. Saving creates a backup and publishes atomically; the next engine start loads the file.

### Console permissions and scheduling

`permissions.bindings` overrides the global mode by `loops.<role>`, `routes.<route>` or `judge-panel`, using read-only, workspace-write or danger-full-access. Read `permission-policy.schema.json` for the contract. Adapters must still prove the requested capability; native Claude approval modes are not advertised as an OS read-only sandbox.

`scheduler` is defined by `scheduler-policy.schema.json`. Serial is the default. Parallel only dispatches independent units declared by frozen schedule.after/writes; max-parallel limits units and providers limits actual account channels. Hard budgets admit one model call until its accounting is durable. Missing declarations and overlapping write scopes are not independence.

For example:

```json
{
  "permissions": {
    "mode": "full-access",
    "bindings": {"loops.review":"read-only","routes.hard":"workspace-write"}
  },
  "scheduler": {"mode":"parallel","max-parallel":2,"providers":{"deepseek-account":1}}
}
```

This is a field excerpt, not a complete configuration replacement. The console reads and validates the complete selected file. See the [console guide](flow-console.md) for run indexing, controlled session interaction, observation completeness and remote-delivery authority.
