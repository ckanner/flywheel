# Configuration

[Documentation](../README.md) · English · [简体中文](../zh-CN/configuration.md)

Flywheel loads one complete configuration, expands model profiles, and resolves
stage bindings before starting. Model allocation, workflow selection, permissions,
scheduling, and session history are separate policies. Restart the engine after
changing configuration; installation preserves existing files.

Use `config inspect` to find the effective file before editing. The examples below
that show selected fields are **fragments** to place in a complete configuration.

- [Configuration sources](#configuration-precedence) and [templates](#start-from-a-complete-template)
- [Model fields](#fields-and-identities) and [subscription migration](#apply-the-subscription-preset-to-existing-configuration)
- [Subscription-only routing](#subscription-only-routing) and [default model allocation](#default-model-allocation)
- [Permissions](#full-access-without-per-action-approval) and [scheduling](#scheduling-and-worktrees)
- [Session history](#session-policy-history-reuse-and-invocation-safety) and [workflow policy](#workflow-policy-and-recovery)
- [Backend integration](#codex-pro-and-migration), [budgets](#fallback-and-budgets), and [diagnostics](#diagnose-without-inference)


## Reviewed-planning bindings

New tasks have separate `design-plan`, `design-review` and `plan-review` roles.
Missing `loops.design-plan` inherits the `loops.plan` backend; both new review roles
inherit `loops.review`. Explicit `loops.<role>` / `drivers.<role>` bindings override
that source. Profile, model, effort and native profile resolve through the source;
`config inspect` includes `bindingSource` so inheritance is visible. Setting an
explicit binding's `native-profile` to an empty string disables inherited native
capabilities. The desktop settings view shows inherited rows; editing one saves
only that role's override.

Role identity, sessions and permissions do not inherit the source role's overrides.
`sessions.roles` accepts all three new roles independently. Under `role-based`, new
reviewers default to read-only; a Claude Code Pro backend requires an already
authorized full-access policy or explicit `permissions.bindings` for
`loops.design-review` and `loops.plan-review` with `danger-full-access`. Doctor
reports unsupported restricted combinations rather than broadening them.
Cross-channel fallback still requires `channels.<channel>.models.<new-role>`;
inheritance does not extend previously authorized fallback models or spending.
See [Reviewed planning](reviewed-planning.md) for design approval and scoped Plan.

The final preset explicitly binds all these roles. It also supplies a compact
design pair:

```json
{"planning":{"compact":{"design-plan":"hydrogen-local","design-review":"sol-plan"}}}
```

The values are names in `profiles`, not inline bindings. A reviewed-planning
brief with `design-granularity:"compact"` selects this pair for design drafting
and design review. System design uses `loops.design-plan` and
`loops.design-review`. Compact design still produces a reviewable design and
passes the same approval gate; it does not automatically choose Quick, skip
scoped Plan, or authorize implementation. Old configurations without
`planning.compact` retain their normal role bindings. Design granularity,
Auto/Quick/Full execution and review-first/delegated approval are independent.


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

### Editing configuration in the workflow console

The Global / project configuration view first reads an explicit scope and file revision. Edit model bindings, role history policies, budget, timeouts, retries or the global permission policy, then review changes and save beside the form. Concurrent edits reject replacement; reread an unconfirmed save before proceeding. New project files use the current version's complete default template. A higher-priority task file can still hide the project/global file being edited. Files do not merge.

**Models and sessions → Complete execution bindings** creates, copies, edits and
removes complete candidates, including Provider, exact model, effort, native
profile, permission and session policy. The editor lists every referencing route;
editing a shared candidate affects those routes, while copying creates an unused
candidate until it is explicitly added to a route. Referenced candidates cannot
be deleted. The maximum remains 32 bindings.

**Execution candidates** edits the actual
`execution-policy.routes.<route>.initial`, allowed set, route authorization and
0–32 switch limit. Selecting an initial candidate preserves its complete binding.
Editing the initial model in the stage table isolates a candidate shared by other
routes. Changing route authorization explicitly updates all its allowed candidates,
copying candidates shared with other routes before remapping this route's references.
Other routes retain their permissions. A direct candidate permission edit that
conflicts with a referencing route blocks saving until authorization is explicitly
reconciled; session policies may differ between allowed candidates. A legacy route
can opt into a policy using its existing complete binding and zero switches.

The **Quality adaptation order** controls add, remove and reorder allowed
candidates without assigning a model quality ranking. Version 1 acts after the
first confirmed stalled comparison. **Channel fallback rules** select a different
channel's allowed candidate and show the required shared channel chain, backend
and route model mapping. Applying that mapping is a separate explicit draft edit;
unmatched mappings, cycles and conflicting rules block saving. Removing a rule
retains the shared channel link until it is explicitly removed after all rule
references are gone. Review the shared effects in the diff. Initial, quality and
fallback references must be removed before a candidate leaves the allowed set.

Inside a stage or candidate editor, **Read model catalog** requests exact IDs
from the selected native client on demand. Supported sources are `agy models`,
Codex app-server `model/list`, and Claude SDK initialization metadata. Unsupported
backends or unavailable clients leave manual model entry usable. The view shows
the source, native-client scope, limitations and account entitlement as unknown;
the read performs no inference. Catalog effort choices/defaults are used only
when explicitly reported. A current supported effort is retained. An unlisted
model remains an exact manual ID, and changing Provider, channel, task or scope
cannot apply a stale catalog response to the new selection.

All these controls use the same revision-checked draft and Save changes action.
Already frozen task configurations remain unchanged. Advanced JSON retains the
complete document and unsupported provider-specific native schemas.

**JEV** displays the file being edited separately from the file actually selected
for the current task, including its scope, enabled state and resolved subscription
model/effort. A read-only configuration inspection follows each confirmed editor
read or save. If that inspection fails, the effective configuration is shown as
unconfirmed; a confirmed save remains a confirmed save. Saving a global switch
does not override a task or project file, and an unknown save is not retried by
the interface. The five permitted-data categories are also selectable in this
section; they do not grant tools or execution permission.

Advanced callers send JSON on stdin to `flywheel control config-edit --cwd /absolute/repo`, with `{"scope":"project"}`. Save through `control config-save` using scope, the returned revision and values containing changed sections only. With reset=true, replacement starts from the full embedded default template. Saving creates a backup and publishes atomically; the next engine start loads the file.

## Fields and identities

| Field | Meaning | Example |
|---|---|---|
| `harness` | Process and tool-loop backend | `antigravity`, `kimi`, `codex-pro`, `claude-code-pro`; legacy configurations can also select `dsh`, `codex`, or other supported drivers |
| `provider` | Registered DSH SDK LLM adapter ID; applies to DSH bindings | `deepseek-account`, `chatgpt`; omitted on the native subscription bindings |
| `channel` | A configuration label used for credentials, fallback, and accounting | `google-subscription`, `claude-native`, `chatgpt-subscription`, `kimi-subscription` |
| `model` | Exact model identifier passed to the selected backend | `gemini-4-hydrogen-high`, `claude-opus-5-5`, `gpt-6.2-sol`, `kimi-code/k3.1-pro` |
| `thinking` | Requested reasoning effort | `high` |
| `profile` | Reusable model fields inside this Flywheel configuration | `hydrogen`, `opus-plan`, `sol`, `kimi` |

A Flywheel model profile is different from a DSH `sdk` or `desktop` profile. The latter chooses the host's plugins and authorization configuration. A channel alias alone does not switch DSH accounts or create a separate subscription or quota pool.

For legacy DSH bindings, the provider resolves from `binding.provider`, then `channels[channel].provider`, then the channel name. Explicit binding and channel provider values must agree. Native subscription harnesses use their own login state; a DSH provider cannot be attached to them and silently ignored.

Only supported effort values should be used. Antigravity requires a matching model slug and effort, such as `gemini-4-hydrogen-medium` with `thinking:"medium"`. A configuration value names an intended binding, not evidence of account entitlement or measured model performance.

## Subscription-only routing

New complete templates include:

```json
{"subscription":{"version":1,"mode":"subscription-only"}}
```

This policy validates every reachable loop, route, enabled second opinion,
compact-planning profile, execution-policy binding and channel fallback. Only
`antigravity`, `kimi`, `codex-pro` and `claude-code-pro` are admitted. Their
bindings and reachable channels must not specify a DSH provider or nonempty
`env-key`. Cycles, undeclared channels and conflicting harnesses fail during
configuration loading. An enabled JEV must use its subscription backend.
Unused custom definitions may remain in the file, but do not become fallbacks.

The workers repeat their native routing and configuration checks before task
input. An unsupported contract, conflicting API route or unavailable selected
model fails explicitly. No worker adds an API-key, credit, alternate-model or
provider fallback. This policy does not log in, change account settings, attest
remaining quota, or lock billing outside the invocation. Evidence differs by
adapter: Claude requires the same-process usage response to show extra usage
disabled; Antigravity verifies its local credit-fallback control; Codex and
Kimi retain unknown account-wide extra-usage status where their official
interfaces do not expose it. See [Claude admission](claude-code-pro.md#native-subscription-admission),
[Antigravity controls](antigravity.md#native-subscription-controls) and
[Codex/Kimi admission](native-capabilities.md#subscription-only-codex-and-kimi-admission).

Existing complete files that omit `subscription` retain their explicit legacy
DSH/API/CLI behavior. DSH's ChatGPT provider is supported on that compatibility
path; it cannot substitute for the native Codex Pro admission contract under
`subscription-only`. Installing an upgrade does not silently enable this policy.

### DeepSeek account and API-key providers

| Access method | Provider | Credentials | Selection |
|---|---|---|---|
| Signed-in, funded DeepSeek account in DSH | `deepseek-account` | DSH's `deepseekAccount` service | Explicit legacy configuration without `subscription-only`; no `DEEPSEEK_API_KEY` required |
| DeepSeek API key | `deepseek-official` | DSH credentials or launch environment; default reference `DEEPSEEK_API_KEY` | Explicit legacy API configuration without `subscription-only` |

Neither access method is part of the current default allocation. This section
documents compatibility with existing files and the older `subscriptions`
migration preset; an account balance is not a subscription allowance.

The standard SDK profile layers over a [base](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/bundle/base/cordis.patch.yml) that includes the account service and both adapters. The standard desktop and SDK share the [account credential store](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/credentials/credentials-local/src/index.ts) when they use the same `DSH_HOME`. Profiles isolate plugin configuration; no token copy or switch to the reserved `desktop` profile is needed. Custom homes, credential stores, or SDK plugin sets must resolve the intended authorization source.

After login, the desktop can choose the first account model when no provider API key exists. That host selection neither controls an independent SDK session nor guarantees Flash is the first model. An explicitly configured Flywheel DSH call sends its selected provider and model. The older preset selected `deepseek-account` and `deepseek-flash`, without an API-key fallback. Explicit legacy fallback chains retain their configured policy. The checked sources are the [account adapter](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek-account/src/index.ts), [API adapter](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek-api-key/src/index.ts), and [host default selection](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/api/session-controller/src/index.ts).

Explicit API-key configurations remain supported. Select `deepseek-official` consistently in the relevant binding and its channel, and configure credentials through DSH's API-provider flow. Renaming a channel does not override a conflicting explicit provider. Account access does not need an additional API key.

## Native capability profiles

`native-profiles` defines versioned harness options; a binding selects one with
`native-profile`. Model profiles, loops, routes, judge-panel and allowed
execution-policy bindings support this reference. The harness must match.
Omission inherits model-profile settings; an explicit empty string disables an
inherited capability profile. Static profiles work with JEV off. The workbench
selects profiles for the current harness and actual role. Antigravity Swarm is
disabled for Plan, Review, Judge and compact-planning roles; configuration loading
rejects those invalid references as well. Execute displays the remaining runtime
requirements: Full, an approved 2–8-task collaboration declaration with independent
write scopes, and no native image inputs. Configuring a Swarm candidate does not
establish a particular packet's eligibility. Tasks without images can use
Antigravity; its Flywheel integration currently has no native image-input channel.

The stage binding editor exposes Antigravity `native`, `subagents`, and `swarm`
options, plus optional `maxObservedAgents` from 1 to 8. Edits create a dedicated
profile when the current definition is shared, preserving other roles, candidates,
permissions and sessions. The limit counts observed child IDs, including verifier
or monitor agents, and requests a stop when exceeded; it is not hard concurrency
enforcement or evidence that every child finished. Requested mode and observed
collaboration remain separate runtime facts. Other detailed native definitions
are editable in advanced JSON.
Native options participate in history and frozen policy identity, and actual
admission/usage/cleanup remain runtime facts. See [Native capabilities](native-capabilities.md)
for complete examples, LCU, Tower, independent Review/Team commands and limits.

The default `kimi` model profile selects `kimi-subscription-agent`:

```json
{"native-profiles":{"kimi-subscription-agent":{"harness":"kimi","options":{"version":1,"mode":"agent","agents":{"enabled":false}}}}}
```

It uses the native Agent path with child agents disabled. Under subscription
policy, the worker checks native routing and disables service tools whose
credential source cannot be distinguished; the old print path, Swarm, Tower and
custom child-agent configurations are not substitutes. This profile applies to
Garden and batch execution. Existing configurations without the policy retain
their legacy paths. Antigravity accepts version 1 `native`, `subagents` and `swarm`
profiles with optional `maxObservedAgents`; its evidence-only analyst contract
rejects native collaboration and is selected by the engine. See
[Antigravity](antigravity.md) for fixed-effort Orchestrator and collaboration.

## Start from a complete template

Inspect either bundled example:

```sh
flywheel template flywheel.config.example.json
flywheel template flywheel.profiles.example.json
```

Both names provide the complete current subscription CLI configuration with reusable profiles. If you choose to create a project configuration, first confirm that no file needs preserving; then export one template from the target Git root. For example, Bash's noclobber option prevents replacing an existing regular file:

```sh
(set -C; flywheel template flywheel.profiles.example.json > flywheel.config.json)
```

Do not create a task file containing only a few profiles and expect it to inherit the rest from the project file. An illustrative **fragment**, to incorporate into a complete file, is:

```json
{
  "profiles": {
    "hydrogen": {
      "harness": "antigravity",
      "channel": "google-subscription",
      "model": "gemini-4-hydrogen-high",
      "thinking": "high"
    }
  },
  "routes": {
    "default": { "profile": "hydrogen" }
  },
  "loops": {
    "intake": { "profile": "hydrogen", "model": "gemini-4-hydrogen-low", "thinking": "low" }
  },
  "channels": {
    "google-subscription": {
      "harness": "antigravity",
      "env-key": null
    }
  }
}
```

`loops` binds reviewed planning, Plan, Review, Judge, Intake, Garden and investigation analysis. `routes` selects the Execute backend for each packet. `drivers` selects real or mock drivers; it is not a provider-name mapping.

### Default model allocation

| Stage or route | Profile | Exact model | Effort | Harness |
|---|---|---|---|---|
| System design | `carbon` | `gemini-4-carbon-high` | high | `antigravity` |
| System design review | `astra` | `gpt-6.2-astra` | high | `codex-pro` |
| Compact design | `hydrogen-local` | `gemini-4-hydrogen-medium` | medium | `antigravity` |
| Compact design review | `sol-plan` | `gpt-6.2-sol` | medium | `codex-pro` |
| Scoped Plan | `opus-plan` | `claude-opus-5-5` | high | `claude-code-pro` |
| Plan review | `sol-plan` | `gpt-6.2-sol` | medium | `codex-pro` |
| Execute `default` | `hydrogen` | `gemini-4-hydrogen-high` | high | `antigravity` |
| Execute `complex` | `neon` | `gemini-4-neon-high` | high | `antigravity` |
| Execute `frontend` / `multimodal` | `opus-ui` | `claude-opus-5-5` | medium | `claude-code-pro` |
| Execute `backend` | `sol` | `gpt-6.2-sol` | high | `codex-pro` |
| Execute `batch` | `kimi` | `kimi-code/k3.1-pro` | high | `kimi` native Agent |
| Execute `hard` | `astra` | `gpt-6.2-astra` | high | `codex-pro` |
| Review | `sol` | `gpt-6.2-sol` | high | `codex-pro` |
| Judge | `astra` | `gpt-6.2-astra` | high | `codex-pro` |
| Intake | `hydrogen-summary` | `gemini-4-hydrogen-low` | low | `antigravity` |
| Garden | `kimi` | `kimi-code/k3.1-pro` | high | `kimi` native Agent |
| Investigation evidence analysis | `neon` | `gemini-4-neon-high` | high | `antigravity`, evidence-only |
| Optional JEV selector, disabled by default | `hydrogen-local` | `gemini-4-hydrogen-medium` | medium | `antigravity`, evidence-only |

`fable` (`claude-fable-5-2`, high) and `carbon-explore`
(`gemini-4-carbon-medium`, medium) are available for explicit overrides. The
template has no second opinion, channel fallback or paid API binding. All four
native channels use `env-key:null` and no `provider`. This table describes the
configured allocation, not measured quality or confirmed account access; see the
[model policy](model-policy.md) for its rationale and acceptance limits.

### Profile override and validation rules

| Input | Effect |
|---|---|
| Stage omits a profile field | Inherit that field from the referenced profile |
| Stage explicitly supplies `model`, `thinking`, `channel`, etc. | Override only that stage |
| `thinking: ""` | Use the harness's default only where that adapter allows omission; Antigravity requires an explicit effort matching its slug |
| Plan `resident-session: false` | Explicitly disable Plan history reuse; loading, profile expansion, and effective config export preserve its presence |
| `judge-panel: null` | Disable the optional second opinion |
| Missing/empty profile `harness`, `model`, or `channel` | Configuration error |
| Unknown profile, field, stage name, or harness | Configuration error |
| A profile references another profile | Unsupported; configuration error |
| `_note` or `_note_*` | Permitted explanatory metadata, excluded from execution bindings |

`resident-session` is a legacy stage option, not a profile field. It continues to control the existing Kimi Plan behavior, and an explicit false also opts Plan out of scoped history. Other roles use the top-level `sessions` policy. Restoring history does not require a permanently running model process.

Legal existing zero values, including `env-key: null`, `retry.jitter: 0`, and `worktree.enabled: false`, remain meaningful. When changing a harness or channel, also inspect any inherited provider. Use `provider: ""` when necessary to clear an explicit value before resolving the newly selected DSH channel.

## Apply the subscription preset to existing configuration

Use `subscription-cli` for the current Gemini, Claude, Codex and Kimi allocation.
Normal installation and upgrades preserve existing files. The older preset named
`subscriptions` remains a legacy compatibility operation with its original
DeepSeek account/DSH mappings; it does not enable the new native subscription policy.

Inspect the preset shipped with your installed binary using `flywheel template flywheel.config.example.json`; an older binary can contain older defaults. Release history belongs to the [changelog](../../CHANGELOG.md).

From the actual project, preview and apply the selected configuration. Add `--task-dir /repo/docs/my-task` when the task has its own configuration:

```sh
flywheel config migrate --preset subscription-cli --cwd /repo --json
flywheel config migrate --preset subscription-cli --cwd /repo --apply --expect-sha256 PREVIEW_SHA256 --json
flywheel config inspect --cwd /repo --json
```

The first command only previews. Replace `PREVIEW_SHA256` with its returned
digest; `--expect-sha256` is optional but binds application to that exact source.
`--apply` backs up the original bytes and updates the effective
task/project/global file; with no file, it creates global configuration. Changed
source files, invalid input, unsupported file identities and extra custom
routes/stages are rejected. Budget, verification, worktree, retries, pricing,
notes and existing permission/session policies are preserved. If no `sessions`
policy exists, this explicit preset adds the current scoped default.

The preset replaces its named loops, routes, profiles, channels and native
profiles with the complete bindings above, including the Claude multimodal route;
it preserves explanatory notes but clears superseded binding overrides and
selected-channel fallbacks. It installs `subscription` and `planning.compact`.
Unreferenced custom definitions remain intact. Existing enabled second opinions,
execution policies and other reachable routes must pass the subscription graph
check; incompatible paid bindings block the candidate rather than disappearing.

A disabled legacy HTTP JEV is converted to
`{"enabled":false,"backend":"subscription","profile":"hydrogen-local"}`.
Its timeout, call budget, thresholds and allowed data are retained; the report
lists removal of HTTP-only `endpoint`, `key-env` and `model` fields. An enabled
HTTP JEV blocks migration before any write. Disable it deliberately, migrate,
review the new target and then enable JEV if wanted. An existing subscription
JEV is preserved; an absent JEV receives the new disabled default. Reapplying an
unchanged migration is a no-op.

The migration does not broaden existing `role-based` permissions. Run `doctor`
afterward; ordinary Antigravity execution and Claude require an explicitly
authorized full-access binding. Review and change those permissions separately
when intended. Permission conflicts are not solved by silently switching
provider, dropping the subscription policy or elevating a worker.
Only when `loops.investigate` has no explicit permission override does migration
add `read-only` for the new analyst, reporting that restriction as a field change.
The existing global mode and other role overrides remain intact. An explicit
analyst override is also preserved; if it is not read-only, the report asks for
manual review before investigation analysis can run.

Do not edit the target file while migrating. The lock coordinates Flywheel migration processes; the source is checked again before a complete file is atomically replaced. This is not an atomic compare-and-set against arbitrary external editors. Reports distinguish proposed configuration (`configurationStatus: proposed`), re-read effective configuration (`effective`), and an unconfirmed post-write result (`unconfirmed`). A failed backup or write must not present the proposed models as already effective.

Migration does not sign in or invoke models. Running tasks retain their startup configuration; the next engine start uses the migrated file. Review the resulting source, scope, and bindings before stopping/resuming an existing task as needed.

## Full access without per-action approval

```json
{"permissions":{"mode":"full-access"}}
```

New installation templates select this global mode and explicitly bind
`loops.investigate` to `read-only`. The `subscription-cli` migration preserves
an existing permission policy rather than elevating it. Explicit per-binding
overrides take precedence. Codex Pro maps full access through its public machine
API; Claude uses its authorized bypass mode; Antigravity uses native
`accept-edits`/`always-proceed` without a sandbox; the Kimi native Agent profile
uses native `yolo`. Legacy DSH/Codex and Kimi print retain their existing mappings.
Full access grants worker access within the current operating-system user's
permissions, independently of the desktop conversation's permission dropdown.

Existing files without this setting retain `role-based`: Plan/Execute request workspace-write, and Review/Judge request read-only. Neither policy waits for interactive approval. In restricted modes, `never` can reject an action; it does not itself grant full access. Invalid policy names and types fail before execution.

Full access preserves task boundaries, frozen acceptance, verification, and candidate identity checks. A reviewer still cannot change the candidate or acceptance criteria and obtain a valid verdict. Select `role-based` explicitly when confinement is intended; a worktree is not an OS sandbox.

### Per-binding permission overrides

`permissions.bindings` overrides the global policy for `loops.<role>`,
`routes.<route>`, or `judge-panel`. Values are `read-only`, `workspace-write`, or
`danger-full-access`; omitted bindings retain the global policy. For example:

```json
{
  "permissions": {
    "mode": "full-access",
    "bindings": {"loops.review": "read-only"}
  }
}
```

The [permission schema](https://github.com/ckanner/flywheel/blob/main/templates/permission-policy.schema.json) defines the
accepted fields. Each backend must support the selected policy. Claude Code Pro
requires full access; native approval settings do not provide an OS read-only sandbox.
Changing effective permissions also changes session-reuse eligibility. The
investigation analyst and subscription JEV use a separately verified empty-tool
Antigravity agent; that evidence-only contract does not claim an OS sandbox.

## Scheduling and worktrees

The embedded installation default selects parallel scheduling with a maximum of two packets.
An existing complete file without `scheduler` normalizes to **serial**, and an
explicit serial choice remains unchanged. Installing a new release does not replace
that file. The [scheduler schema](https://github.com/ckanner/flywheel/blob/main/templates/scheduler-policy.schema.json)
bounds total and per-channel concurrency from 1 to 8.
Both new and existing configurations default to `dispatch: "batch"` and
`prewarm: false`. These settings do not change the Quick/Full workflow or enable JEV.

```json
{
  "scheduler": {
    "mode": "parallel",
    "max-parallel": 2,
    "providers": {"google-subscription": 1},
    "dispatch": "batch",
    "prewarm": false
  },
  "worktree": {"enabled": true}
}
```

Despite the field name, `providers` keys are the **actual configuration channel
names**, not DSH provider IDs. The native Gemini bindings use
`google-subscription`; a custom channel alias must use its actual name here.

Parallel eligibility comes from a packet's frozen declaration:

```yaml
schedule:
  after: [M01-T01]
  writes: [src/search/, tests/search/]
```

Dependencies must name earlier known packets. Write scopes are normalized relative
paths or directory prefixes, without globs or escapes. Only ready packets with
nonoverlapping scopes qualify; missing declarations, conflicting scopes, and
live-credential units remain serial. Hard budgets admit one model call at a time
until its accounting is durable. Review, integration, and final gates remain serial.

`worktree.enabled` defaults to true and controls automatic packet directory
management, independently of scheduling. Parallel automatic management requires
isolated worktrees and fails if creation fails. The legacy serial path can record
an in-place fallback. With automatic management off, executors can share the task
checkout or register a qualifying external worktree before implementation. Shared
batches verify the complete combined candidate after workers finish and deliver
only when every member passes; external worktrees are retained.

Project workspace identity and execution location are recorded separately. Native
backends that cannot separate a `workspace-write` sandbox root reject unsupported
split-directory requests before dispatch, without expanding permissions. See the
[console guide](flow-console.md#parallel-execution) for registration and batch rules.

### Ready queues and runtime preparation

`scheduler.dispatch` accepts `batch` or `ready-queue`. Batch dispatch waits for the
current bounded group before selecting more work. Ready queues admit the next
already independent packet when an Execute/L1 slot becomes free. They apply to
Full with parallel scheduling, a maximum above one, and automatic Worktree
management. Quick, serial scheduling, a maximum of one, and automatic management
off retain their existing path. Shared-checkout execution retains its batch rules,
including when executors may register external Worktrees later.

The first ready-queue window must have every Worktree prepared and saved before
dispatch. Later packets prepare their directory before taking a freed slot. A
preparation failure stops admission and cancels and awaits started workers; queued
packets that have not been admitted create no directory or model invocation.
Dependencies become ready only after delivery. Review, replanning and integration
wait for all admitted workers to settle, and every combined candidate is rechecked.
Existing total/channel limits, hard budgets, attempts and unknown-result recovery
remain in force.

A replacement packet does not recover or cancel a sibling's still-running model
call. The coordinator retains those calls and their shared accounting. An unknown
result or failed receipt persistence stops new admission immediately and must be
reconciled before resuming.

`--pause-before review` also applies to independent parallel Worktrees. Once all
Execute/L1 work in the current group passes and its workers settle, the task pauses
before any Review or integration, retaining the verified candidates. Batch mode
finishes its admitted group; ready-queue mode may refill the fixed independent
queue before reaching that boundary. Resuming the same task reuses valid execution
evidence instead of repeating Execute. A candidate changed by later integration
still requires validation again.

`scheduler.prewarm: true` allows a serial packet to initialize one DSH Review
runtime while its post-Execute deterministic checks run. Initialization sends no
model prompt or Session request. Review inference still waits for its checks and
normal admission. Parallel Execute/L1 lanes do not prepare runtimes: after their
barrier, preparation can begin only in the serial Review path. Shared-candidate
batches do not prewarm. The main overlap opportunity is therefore a serial
Execute's L1 and applicable Quick checks, not every parallel lane's validation.

Unused or incompatible runtimes must finish bounded cleanup before further model
work. If an unused runtime reaches its 60-second lifetime before a formal start
is attempted, one cold invocation may use the same persisted request identity and
remaining time allowance. This requires an exact, durable no-dispatch receipt,
confirmed SDK and process-group cleanup, and no cancellation or expired invocation
deadline. It does not grant another attempt or reset a budget. Unknown cleanup
remains blocking. Other harnesses start normally. Session history reuse is separate
from runtime preparation and retains role isolation.

Confirmed initialization failures preserve their reported error classification and
the normal retry/fallback rules. An authentication or invalid-request failure does
not become a transport retry or qualify as a harmless prepared-runtime expiration.

In Settings → Execution, dispatch is a Batch/Ready queue selector and prewarm is
an independent switch. The form shows unmet Ready queue configuration conditions
without resetting the selected value; task eligibility is checked by the engine.
Both options also remain available in advanced JSON. Saving
applies to the next start; the current process retains its loaded policy. A resumed
run still reconciles existing invocations and shared batches before new dispatch.
An outstanding prepared worker also requires its original no-prompt cleanup
receipt, synchronized file and directory, and confirmed worker/process-group exit,
even after `prewarm` is disabled; see
[prewarm recovery](session-recovery.md#dsh-runtime-preparation-after-a-restart).
Both optimizations remain optional while live latency, resource use and adoption
defaults are evaluated separately. Offline checks establish behavior and recovery,
not a guaranteed speed or token improvement. See the
[P5 implementation plan](../plans/2026-11-13-p5-ready-queue-prewarm.md) (Chinese).

## Session policy, history reuse, and invocation safety

```json
{"sessions":{"mode":"scoped","policy-version":1}}
```

Both new-installation templates explicitly select `scoped`. An existing file without `sessions` normalizes to `fresh`, policy version 1; upgrades do not rewrite it. An explicit policy must contain both fields. Unknown modes, future versions, nulls, typos, and incomplete objects fail before a model starts. The [session policy schema](https://github.com/ckanner/flywheel/blob/main/templates/session-policy.schema.json) is the field contract.

`sessions.mode` controls history reuse; it does not disable invocation safety records. Main-task Plan, Execute, Review, and Judge calls to Antigravity, Codex Pro, Claude Code Pro, DSH, and Kimi journal intent and trusted terminal receipts in both modes. `fresh` disables general role history reuse and retains each backend's new-call behavior. It preserves Kimi Plan `resident-session` behavior only on the legacy CLI path without a `native-profile`. Native Agent/Swarm history uses the scoped role ledger; Tower always starts fresh. `scoped` can continue compatible history after a trusted normal terminal outcome. Other adapters and mock retain their existing invocation path. Intake and Garden are outside this role session policy. Investigation analysis uses its own round-bound ledger, and JEV has an independent decision journal; neither borrows executor history.

| Role | History scope and responsibility |
|---|---|
| Plan | Same task and milestone; only permitted task-contract revisions |
| Execute | Same task and packet; ordinary repairs may continue executor history |
| Review | Same task, packet, and review responsibility; isolated from the executor and still inspecting the complete current candidate |
| Judge / second opinion | Same task and milestone, with separate histories for primary and panel judgments |

Quick final Review and Full packet Review have different completion requirements and cannot exchange old histories or verdicts. After reviewed planning and approval, Quick execution's first-success path has two stage-agent calls, Execute and final Review. Go owns all session selection, continuation, rebuilding, and accounting. Skills, models, and desktop controls do not scan backend history to guess a session.

An explicit Plan `resident-session:false` still disables that role's history reuse under scoped mode; omission is different from an explicit opt-out. Without a `native-profile`, `resident-session:true` retains the legacy Kimi CLI Plan behavior. For native Agent/Swarm, that explicit resident intent under global `fresh` instead selects the scoped role ledger, unless `sessions.roles.plan:"fresh"` explicitly disables reuse. It never imports old CLI sessions. Tower always stays fresh. This option does not enable history reuse across harnesses; Codex Pro still rejects it, so select scoped Codex Pro history through `sessions`.

Reuse identity includes the actual task and working directories, role and scope, harness, provider, channel, model, thinking, permissions, and backend runtime references. Backend home/profile, worker, and command references reflect the child configuration actually used; only the names of injected credentials are included, never their values. Renaming a profile without changing its resolved binding does not invalidate history. A changed backend, account channel, model, permission, or working directory selects a new history. Changes to the Spec goal, non-goals, requirement identity, or packet goal, body, boundaries, and requirement mapping also invalidate the previous task semantics. Milestone-scoped Plan, Judge, and second opinions also check the milestone title and the identities and goal fields of its constituent packets. Compatible acceptance revisions can retain already-consumed history, with the current contract and evidence supplied again.

These non-secret references do not attest an account's identity or the contents of the referenced files. Replacing configuration, account authorization, or an executable in place under the same path is not guaranteed to change this policy's binding. Credential values are not read to compare accounts. Accepted configuration and effective permissions remain limited to what the current adapter receipt can establish.

Completed but unconsumed receipts have stricter rules: Review and Judge must match the original contract, candidate, and verification evidence. Running verification again cannot replace that evidence. Plan may update its permitted contract files, but replay remains bound to the complete post-call candidate and durable original before/after guards. A forbidden reviewer or planner mutation cannot become an accepted result after restart.

`state.sessions` records each scope's generation, action/reason, and latest invocation facts. One atomically replaced `.sessions` receipt slot per scope bounds recovery storage; raw receipts are excluded from Git. Recovery verifies the saved intent, receipt digest, and identity before consuming the same result, without another model request or duplicate billing. Budgets constrain new dispatches; consuming an already completed trusted receipt is not a new call. Metrics retain earlier audit facts.

The single-slot limit applies only to Flywheel's recovery receipt. Native thread and Session histories follow the backend's retention rules. Rebuilding a role history or shutting down its worker does not automatically delete earlier backend histories.

An unknown dispatch, outcome, or cleanup state blocks further execution. Changing to fresh mode, changing adapters, or restarting cannot bypass it. Process exit alone does not establish a known model outcome. `resume-candidate` only permits an adapter to attempt normal continuation; it does not prove recovery after an arbitrary crash. Main-task calls through Codex Pro 0.10.2, Claude Code Pro 0.7.0 and Antigravity 0.2.0 use outer worker v3: public typed lifecycle events, ACKs that wait for Go's durable checkpoint, and a read-only recovery entry that never sends a new turn. Codex gates dispatch after verified `thread-ready`. Claude checks SDK initialize/version and its required subscription evidence before task input, while native `system/init` still follows prompt dispatch and cannot provide an equivalent pre-dispatch permission checkpoint. Antigravity waits for native init before task input for evidence-only and explicit subagents/Swarm calls. Its ordinary native path (including Orchestrator) sends input after spawn and validates init afterward; it does not provide a universal pre-dispatch thread-ready gate. Each adapter retains uncertainty when its evidence is insufficient. See [interrupted invocation reconciliation](session-recovery.md).

`status` and `control status` expose role, scope, generation, action/reason, latest status, dispatch/outcome/cleanup, history-reuse selection, and usage status. The compact projection does not read receipt contents or conversation history and never changes the ledger. Legacy tasks without session records are not assumed to have recovery eligibility.

### Role overrides and context quality

Optional `sessions.roles` selects `fresh` or `scoped` separately for `design-plan`, `design-review`, `plan-review`, `plan`, `execute`, `review`, `judge`, and `judge-panel`. Omitted roles inherit `mode`. An explicit `roles.plan:"fresh"` also disables the legacy Kimi Plan reuse shortcut; an explicit `resident-session:false` still prevents Plan reuse. Intake and Garden do not accept these overrides. This example changes history policy without changing model or account allocation:

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

### Verified incremental executor context

`sessions.context` is optional: omission or `"full"` retains the complete prompt;
`"verified-incremental"` enables an Execute-only optimization. First calls and
rebuilds receive full authority. Later same-role resumes may omit the stable
contract only after Go rechecks the consumed terminal receipt, exact session
binding, prompt version, contract and all context/cache file content digests.
References must be readable regular files within the current execution directory,
bounded to 1 MiB per file. The update includes feedback, current/prior candidate
identities, attempt and authoritative references.

Missing or revised references, contract changes, absent legacy checkpoints and
invalid context receipts select full input. A changed model, effort, permission
or explicit session mode rebuilds history. Plan, Review and Judge keep full
inputs; Review always examines the complete current candidate and requirements.
`state.sessions.records[*].context` records mode, identity and reason. This is
protocol behavior, not measured evidence of token savings or equal quality.

## Bounded execution policy (opt-in)

`execution-policy` version 1 is independent of Auto/Quick/Full. Omission preserves
fixed route bindings and channel fallback behavior. Its
[schema](https://github.com/ckanner/flywheel/blob/main/templates/execution-policy.schema.json)
requires complete bindings and an explicit allowed set. Add this section to the
effective **complete** configuration after checking channels and permissions;
do not install the fragment as a partial override file:

```json
{
  "execution-policy": {
    "version": 1,
    "bindings": {
      "hydrogen-medium": {
        "harness": "antigravity", "provider": "",
        "channel": "google-subscription", "model": "gemini-4-hydrogen-medium",
        "thinking": "medium", "permission": "danger-full-access",
        "session-mode": "scoped"
      },
      "hydrogen-high": {
        "harness": "antigravity", "provider": "",
        "channel": "google-subscription", "model": "gemini-4-hydrogen-high",
        "thinking": "high", "permission": "danger-full-access",
        "session-mode": "scoped"
      }
    },
    "routes": {
      "default": {
        "initial": "hydrogen-medium", "allowed": ["hydrogen-medium", "hydrogen-high"],
        "quality-order": ["hydrogen-medium", "hydrogen-high"],
        "after-stalled-attempts": 1, "max-switches": 1
      }
    }
  }
}
```

Each binding explicitly declares harness/provider/channel/model/thinking,
permission and session mode; non-DSH bindings use `provider:""`. Version 1 supports
the durable Antigravity, Kimi, Codex Pro, Claude Code Pro and legacy DSH adapters.
The global subscription policy excludes DSH and any paid API route. Every binding's
permission must equal the route's existing authorized permission, so this example
assumes it already uses `danger-full-access`. Backend admission still validates
actual model availability and public configuration/permission capabilities.

`quality-order` is a user-declared experiment order, not a model ranking. Go may
advance one entry when complete repeated L1 observations show the same candidate,
check set, recorded environment and failures. Version 1 requires
`after-stalled-attempts:1`, before the existing spin stop. AUTH, QUOTA,
transport/protocol errors and unknown observations do not trigger quality
upgrades. Original packet `max-retries` and task token budgets remain in force;
switching never resets failures, adds attempts or weakens final gates/Review.

Optional `channel-fallbacks` maps one allowed binding ID to another. Its target
must match the configured channel fallback chain, target harness and route model
mapping. A channel chain alone cannot escape the allowed set. `max-switches`
bounds both kinds of switch per packet. Go freezes the policy identity before
dispatch; `state.packets[*].execution-policy.history` records separate
`quality-upgrade` and `channel-fallback` events with full before/after bindings,
attempt and reason. Parallel packets retain independent records.

With `jev.enabled`, JEV may select a legal binding before the first Execute and
after confirmed failures. These switches are recorded as `jev-initial-selection`
and `jev-model-selection`, use the same `max-switches` and cold-start rules, and
do not alter channel-error handling. The first selection runs before business
dispatch and shares the existing task decision ledger/budget with investigation
and failure decisions. It is recorded once per packet; resume never reruns it to
obtain a different choice. Unknown dispatch/cleanup must reconcile first.
The allowed bindings remain an explicit resource/permission declaration; the
JEV switch does not add a provider, account or model to that set.

The subscription defaults include candidates for all seven routes, preserving
their original initial models and using `quality-order:[]` and `max-switches:2`.
JEV remains off by default, so those defaults keep fixed initial execution.
The table is in [model policy](model-policy.md#initial-selection-and-failure-adaptation).
Go filters candidates before asking JEV: images require the actual Claude image
transport; Swarm requires Full and a valid reviewed collaboration declaration.
No eligible alternative, a disabled selector or exhausted decision budget uses
the current baseline. Fixed-effort Orchestrator uses `thinking:""`.

Migration preserves an existing execution policy. When none exists, it adds
default candidates only to routes already authorized for full access, and records
notes for routes left fixed. It does not broaden preserved permissions.

`config inspect --json` exposes the complete declaration under
`execution-policy`; the ordinary bindings table remains the configured baseline.
Current choices belong to packet/session state. Doctor's existing binding probes
do not establish admission of every allowed experimental binding; actual dispatch
still requires the adapter's configuration and permission checks.

The model editor shows each route's actual initial candidate and its alternatives.
The structured controls above edit complete candidates, explicit authorization,
quality order and channel fallbacks. Editing a route's model preserves shared
bindings and does not change permission or session policy. Incomplete bindings or
unmatched route authorization cannot be saved as if they changed the executor.
Advanced JSON retains the complete execution policy for inspection.

Every switch cold-starts Execute, even an effort-only change; effort remains in
the session binding. Unknown or unconsumed invocations reconcile first. Removing
or changing a frozen policy is refused on resume: restore it, or use a new task
for a changed experiment. Requested settings and available admission facts remain
in session/adapter receipts; unavailable effective effort, usage and subscription
cost stay unknown. Workflow, Review/Judge and delivery authorization are separate.

## Watchdog and planning liveness

`watchdog` provides an output-idle bound where the transport exposes progress, an absolute `wall-timeout-ms` bound, and a `kill-grace-ms` SIGTERM-to-SIGKILL grace. Defaults are 300 seconds idle, 3600 seconds wall, and 5 seconds grace; the kill grace is never shortened per role. Kimi print has no reliable output-idle signal, so its driver uses the wall bound and explicit cancellation without arming the idle timer.

`watchdog.roles.<role>` overrides idle/wall for one role. Where output-idle detection applies, planning roles default to 900 seconds while execute and review keep 300 seconds. These are policy bounds, not proof that a silent model is hung. Example:

```json
{
  "watchdog": {
    "idle-timeout-ms": 300000,
    "wall-timeout-ms": 3600000,
    "kill-grace-ms": 5000,
    "roles": {
      "design-plan": { "idle-timeout-ms": 900000 },
      "plan": { "idle-timeout-ms": 900000 }
    }
  }
}
```

Kimi print buffers assistant deltas, omits thinking deltas and does not expose child progress. Even foreground work can therefore be silent; neither a longer idle value nor a prompt instruction repairs that observation gap. All print roles use their effective wall deadline (including a role override); their idle values do not apply. Use `wall-timeout-ms` to bound a long print call. Explicit stop/cancel and process-group cleanup still apply; zero does not mean an unlimited wall deadline.

The Kimi native worker forwards real root/child item, step and lifecycle progress as metadata to the parent watchdog. It does not forward reasoning or tool payloads and does not manufacture heartbeat events from timers or WebSocket ping/pong. Idle callbacks recheck the latest activity deadline, so a stale queued callback cannot kill work that has already produced new activity. Foreground investigation remains a result-collection constraint for non-native planning, not a liveness guarantee.

Unknown planning dispatch/outcome cannot be fixed by switching bindings, fresh history, deleting the ledger or replay. Use the explicit retirement action in [session recovery](session-recovery.md) only after cleanup is confirmed; unconfirmed cleanup remains blocked. Unknown is not zero and is never silently rewritten as known.

### Defect-injection rollback deadlines

Defect-injection baseline and test commands use `verify.command-timeout-ms`. After injection, `git apply -R` has an independent cancellation context and a bounded rollback deadline: a positive command budget is clamped to 1–10 seconds; a missing effective budget uses 10 seconds. The context and the command watchdog receive the same rollback deadline, followed by the existing cleanup grace. This does not extend the test itself or turn a timed-out test into evidence that the defect was detected. Unconfirmed rollback stops the run and preserves the workspace.

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

Quick reuses existing Execute and Review bindings without changing effort, accounts, fallbacks, or full-access settings. All required checks remain, and applicable L2/L3 run in the actual candidate before final Review. The reviewer checks every requirement; the execution segment's first-success path uses two stage-agent calls, Execute and final Review. Reviewed planning and its approval gate precede this segment. See the [Spec schema](https://github.com/ckanner/flywheel/blob/main/templates/spec.schema.json), [state schema](https://github.com/ckanner/flywheel/blob/main/templates/state.schema.json), and [workflow guide](workflow.md).

## Codex Pro and migration

The embedded hard route uses this binding. This is a **fragment**, not a complete replacement configuration:

```json
{
  "routes": {
    "hard": {
      "harness": "codex-pro",
      "model": "gpt-6.2-astra",
      "channel": "chatgpt-subscription",
      "thinking": "high"
    }
  }
}
```

The complete profile template references `profiles.astra` from `routes.hard` and
uses `sol`/`sol-plan` for its other Codex responsibilities. The worker consumes
the exact public `dsh-subagent-codex-pro@0.10.2` package and its
`dsh-subagent-codex-pro/machine` entry. The dependency is pinned by the plugin
package and lockfile; [vendor provenance](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md)
records the reviewed source artifact. The plugin runs Codex app-server and uses
the existing native login. Claude is a separate exact `0.7.0` optional peer:
it is required for the default Claude stages, but unnecessary when no selected
binding uses that backend. Package capability checks do not establish registry
publication or real-account acceptance.

The external tool name `subagent_codex_pro` is not a DSH provider. This route does not create a parent DSH model to make a second delegation. Missing dependencies or invalid receipts fail; they do not trigger an implicit fallback to the older CLI driver.

To migrate one legacy binding, use `config inspect` to locate the effective file.
Change its inline harness or referenced profile from `codex` to `codex-pro`,
retain the intended model/channel/thinking, inspect the result, and restart.
Keeping `harness:"codex"` retains the older `codex exec` behavior only outside
the new subscription policy. Use `--preset subscription-cli` for the full
allocation and graph validation described above.

`codex-pro` requires nonempty model and channel values. It does not inherit the old Codex driver's implicit model. Omitted thinking defaults to `high`; `resident-session: true` is rejected.

Main-task fresh and scoped calls both use outer worker protocol 3 and retain native history. Fresh omits a prior thread ID on every call, creating a separate named history each time. Scoped can supply the exact confirmed thread ID for continuation. Native persistence does not grant reuse eligibility. Dispatch waits for the relevant lifecycle checkpoints and Go's durable ACK. Public machine requests/terminal receipts remain protocol 1, with separately versioned lifecycle contracts. The legacy outer v1/v2 entries and explicit ephemeral requests remain for low-level compatibility and do not acquire the main-task durable recovery guarantees. Resume must accept the same thread; busy, missing-history, and generic failures cannot be classified as safe cold-start opportunities from error text. See [native client visibility and invocation reconciliation](session-recovery.md#find-the-session-in-its-native-client).

### Worker environment

| Variable | Purpose |
|---|---|
| `FLYWHEEL_DSH_WORKER` | Optional DSH worker override; the npm launcher normally resolves the worker inside its own package |
| `FLYWHEEL_DSH_BIN` | DSH command, otherwise `dsh` on PATH |
| `FLYWHEEL_DSH_PROFILE` | SDK profile, default `sdk`; it must not be `desktop` |
| `DSH_HOME` | Optional DSH configuration root, interpreted by DSH |
| `FLYWHEEL_KIMI_BIN` | Kimi CLI override, otherwise `kimi` on PATH |
| `FLYWHEEL_CODEX_PRO_WORKER` | Optional Codex Pro worker override; normally resolved by npm |
| `FLYWHEEL_CODEX_BIN` | Codex executable used by the plugin, otherwise `codex` on PATH |
| `CODEX_HOME` | Existing Codex configuration/login directory, interpreted by Codex |
| `FLYWHEEL_CLAUDE_CODE_PRO_WORKER` | Optional matching Claude Code Pro worker override |
| `FLYWHEEL_CLAUDE_BIN` | Native Claude executable, otherwise `claude` on PATH |
| `CLAUDE_CONFIG_DIR` | Existing Claude configuration/login directory, interpreted by Claude |
| `FLYWHEEL_ANTIGRAVITY_WORKER` | Optional matching Antigravity worker override |
| `FLYWHEEL_AGY_BIN` | Official native CLI executable, otherwise `agy` on PATH |

These settings must be visible to the process starting the engine. The installer does not copy authentication data. The managed GUI launcher records executable paths and the installation-time tool PATH; a terminal export alone does not change an already running desktop process.

### Permissions, receipts, and cleanup

The Codex plugin checks the requested model, working directory, approval policy, and sandbox mode before a turn. Its public `thread-ready` checkpoint exposes the full effective policy; Flywheel checks it before durably acknowledging permission to continue toward dispatch. In role-based mode, workspace-write permits the workspace and standard Codex temporary directories; extra external write roots, enabled network, or unconfirmed policy are rejected before the new turn. The terminal receipt is checked again. Explicit full access verifies the matching `danger-full-access` sandbox and `dangerFullAccess` policy without claiming confinement.

Claude checks SDK initialize/version and, under subscription policy, same-process
account/usage evidence before task input. Native `system/init` follows prompt
submission, so the adapter does not claim a pre-dispatch `thread-ready`
permission guarantee. Claude requires explicitly configured full access; native
approval modes cannot stand in for an OS sandbox. Kimi's native Agent path also
provides no OS-level sandbox guarantee. Antigravity ordinary execution requires
full access; the separate evidence-only path verifies its generated agent and
tool catalog, then rejects every tool or subagent step. The catalog can be
nonempty because it is the native registry; the managed custom agent file carries
the empty allowlist. Adapter capabilities and actual lifecycle evidence must be
evaluated separately.

The `codex-pro-receipt` metric preserves the complete machine receipt, including requested and actual configuration, thread/turn IDs, process outcomes, cleanup, usage, and failures. An accepted effort request is not proof of effective server reasoning effort; `effectiveEffort` remains null.

An abnormal worker exit does not prove the plugin's separately managed processes stopped. If cleanup cannot be confirmed after startup, the engine records the evidence, escalates, and ends the run instead of automatically retrying or falling back. Before resuming, confirm the owned app-server/guardian processes have terminated. See [recovery](workflow.md#7-stop-and-recover-without-losing-evidence).

## Optional Claude Code Pro

The default `opus-plan` and `opus-ui` profiles use `claude-code-pro` for scoped
Plan and frontend/multimodal execution. The `fable` profile is available as an
explicit alternative; its canonical model is `claude-fable-5-2`. The plugin uses
official Agent SDK 0.3.326 with the selected local Claude CLI and native login.
Under subscription policy, a same-process usage query with
`skipBehaviors:true` must explicitly show extra usage disabled before the task
prompt is released. Unsupported, timed-out or unknown evidence blocks dispatch;
there is no API fallback. See the [Claude Code Pro guide](claude-code-pro.md)
for exact version requirements, aliases, effort conflicts, optional-peer setup,
full-access permissions and recovery. Cumulative `modelUsage` is observation of
the whole native run/history; it does not independently prove a root-model
fallback, current-turn tokens or a subscription charge.

## Antigravity and Gemini

The `antigravity` harness invokes the official local `agy` CLI through the
`@ckanner/dsh-subagent-antigravity@0.2.0` public machine API. The same companion supports
direct use in DSH. No ACP bridge or separate Gemini API credential is involved.
The native catalog must contain the exact selected model, and `thinking` must
match the model slug. The plugin verifies its local subscription/credit controls,
native lifecycle, requested configuration and cleanup; unavailable account
access or models produce explicit failures. It does not silently change the
model, turn on credits or select a cloud API. See the [Antigravity guide](antigravity.md)
for installation, supported native CLI versions, environment checks, DSH entry,
evidence-only analysis and the distinction between local controls and account
billing evidence.

## Fallback and budgets

Default profiles have no channel fallbacks. An explicitly configured
`channels[channel].fallback` is a bounded chain used only when error
classification and retry policy permit it. Every reachable fallback must also
pass `subscription-only` validation. Switching uses the target channel's
explicit role/route model and harness, then resolves its provider again; missing
model mappings stop instead of guessing. An omitted harness inherits the
current harness.

Ordinary channel fallback does not translate the existing thinking level or
native profile for another backend. A cross-harness mapping with incompatible
effort, capabilities or permissions is rejected by admission. Use complete,
explicit `execution-policy` bindings when different effort/native options are
intended. No fallback may bypass unknown dispatch or unconfirmed cleanup.

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

DSH's `ready.reasoningEffort` echoes the requested value after SDK configuration acceptance; it is not an independent server attestation of effective reasoning effort. A no-inference probe cannot demonstrate quality gains from a higher effort. Gemini, Claude, GPT and Kimi have different native effort contracts; see [Reasoning effort](model-policy.md).

Native worker checks import their public APIs and verify the pinned companion
contracts: Codex Pro 0.10.2, Claude Code Pro 0.7.0 and Antigravity 0.2.0, including
their required lifecycle/recovery exports, Node compatibility and native command.
Binding fields `durableJournal`, `readOnlyRecovery` and
`threadReadyBeforeDispatch` describe adapter capabilities; Claude's last field
is false. `historyRecoveryTested` remains false. `--probe` sends no model task
and does not turn a successful import into evidence of account access,
effective permissions or recovery of a saved history. Runtime subscription
admission remains necessary; unknown quota and billing evidence stay unknown.

| Status | Meaning |
|---|---|
| `pass` | Evidence supports this individual check only |
| `fail` | A blocker; overall `ok:false` and exit code 1 |
| `warn` | A limitation or preparation state requiring interpretation |
| `unknown` | Not verified, such as entitlement, quota, or real inference |
| `skipped` | Not required, mock mode, duplicate probe, or an unmet prerequisite |

`sessions.policy` confirms only the configured mode and version. Each applicable real binding's `.session` check reports selection, reason, and adapter support, while history recovery remains unknown. Doctor does not restore user histories, send a test turn, or treat a successful worker import as proof of durability.

No probe sends an inference prompt, runs L1/L2/L3, creates a worktree, or writes Flywheel task state. SDK initialization may contact a provider's catalog and write its own cache files and temporary probe-session persistence; read-only here is scoped to Flywheel state and configuration. Recheck in the desktop panel because its PATH/profile/environment may differ from the terminal.


## JEV decisions

`jev.enabled` is the only JEV activation setting and defaults to false. The desktop
editor exposes one switch; enabled investigation preparations automatically freeze
an adoption policy for action selection, evidence-backed deferral and early report
handoff. Confirmed implementation failures can also select bounded repair, an
approved feedback check, replanning or an allowed execution binding. Task/New/Run
share the same engine-owned policy. Disabling stops subsequent
requests and adoption, while existing receipts and budgets are retained.

The complete default is:

```json
{"jev":{"enabled":false,"backend":"subscription","profile":"hydrogen-local"}}
```

Enabling this policy invokes Gemini Hydrogen/medium through the native
Antigravity subscription path, in a fresh, verified empty-tool agent. JEV
receives bounded approved evidence and chooses an already enumerated action;
it cannot introduce commands, URLs, tools or model bindings. Default bounds are
30 seconds per request, eight cumulative calls, minimum applicability 0.8 and
minimum choice confidence 0.6. The full resolved profile is frozen into the
decision target. Later profile edits invalidate old decisions. Intent and
lifecycle checkpoints are durable before dispatch; unknown cleanup preserves
the pending invocation and blocks further model work until read-only recovery
can establish cleanup. Recovery never resubmits a prompt or adopts a late answer.

There is no independent task shadow/advisory switch. Legacy `jev-shadow` is read
only to migrate connection hints into a disabled unified configuration; editor saves
remove the old key. Explicit new `jev` wins, and configuration still selects one
complete task/project/global/embedded file without merging. Existing files with
an omitted backend retain the legacy HTTP contract; explicit `backend:"http"`
remains supported outside subscription-only policy. Its endpoint/model/key
fields are not accepted on a subscription backend. The dedicated
[JEV guide](jev.md) covers both contracts, data consent, shared investigation and
execution budgets, abstention and recovery. Investigation analysis through
`loops.investigate` is a separate stage and does not require JEV to be enabled.

Packets may declare up to eight optional `feedback-checks`, using the same
`cmd`/`script` and `expect` shape as an acceptance command. They are short,
approved diagnostic candidates, not replacements for `acceptance.L1/L2/L3`.
Add a concise `purpose` (up to 160 characters) explaining what each check can
distinguish. JEV receives that approved purpose, not the command. A check without
`purpose` remains executor guidance and is excluded from JEV's targeted candidates.
Keep credentials, raw logs and command text out of the purpose. JEV may also use
an eligible previously failed L1 check. Execution uses the existing
verification timeout, environment allowlist and process cleanup rules. Changing
these checks changes the contract identity and invalidates stale decisions.

## Reference files

- [Complete default configuration](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.config.example.json)
- [Complete profile example](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.profiles.example.json)
- [System design](../design.md), in Chinese
- [Worker protocol](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md)
