# Claude Code Pro subscription backend

[Documentation](../README.md) · English · [简体中文](../zh-CN/claude-code-pro.md)

Select `harness: "claude-code-pro"` to call the public machine API of
[`dsh-subagent-claude-code-pro`](https://github.com/ckanner/dsh-subagent-claude-code-pro).
The plugin uses official **Agent SDK 0.3.326** to run the configured local Claude
Code executable with its native tool loop. Go retains scheduling, role sessions, verification,
budgets, and delivery.

The adapter requires **`dsh-subagent-claude-code-pro@0.7.0`**, machine protocol
1 and the public lifecycle/recovery interfaces. The older desktop-tool interface
and older machine contracts are not substitutes. The subscription preset uses
Claude for execution planning and frontend/multimodal implementation; Fable is an
explicit alternative profile. See [model allocation](model-policy.md). Package
availability must be checked separately from source compatibility; see the
[release guide](releasing.md).

## Prepare the optional dependency

Use Node.js 22.18.0 or later on macOS or Linux, with a compatible local `claude`
executable and authorized authentication available to the process starting the
engine. Flywheel does not copy credentials or sign in on the user's behalf.

Flywheel ships `flywheel-claude-code-pro-worker` and declares the exact plugin as
an **optional peer**. Other backends work without it. To select Claude, install
`dsh-subagent-claude-code-pro@0.7.0` in the same Node dependency tree or global npm
prefix as Flywheel. Installing it only in a DSH desktop profile does not establish
that the independent worker can resolve it.

When that exact version is available in the registry, install it. For an
explicitly reviewed source candidate, run
`npm pack --ignore-scripts` in the reviewed plugin checkout and install the actual
resulting `.tgz` into the same prefix, alongside a newly built Flywheel candidate.
Do not relabel 0.3.1 or substitute it for the missing machine interface. This
source-candidate validation does not promote npm latest.

The reviewed 0.7.0 companion artifact is recorded in
[vendor metadata](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/claude-code-pro.json).
CI validates those candidate bytes. Before publishing Flywheel, obtain the exact
companion from the public registry and verify its package identity, SHA-256 and
integrity against that record. Source validation alone does not establish registry
availability. This worker rejects older companions rather than silently dropping activity or naming capabilities.
Upgrade the worker and peer together;
unresolved older invocations retain their original adapter/recovery identity and
must be reconciled with their original version first.

| Variable | Purpose |
|---|---|
| `FLYWHEEL_CLAUDE_CODE_PRO_WORKER` | Explicit worker override; npm normally resolves the matching worker |
| `FLYWHEEL_CLAUDE_BIN` | Claude CLI command or absolute path; defaults to `claude` |
| `CLAUDE_CONFIG_DIR` | Optional native configuration/login directory, interpreted by Claude |

Go forwards its base environment allowlist and the selected channel's explicit
credentials. The subscription preset uses `claude-native` with `env-key:null`;
it does not inherit an ambient Anthropic API key. Subscription policy also
rejects conflicting credential/provider settings in the native runtime. The
bridge does not serialize credential values into requests or diagnostics.

## Official SDK, local runtime and authentication

Ordinary calls enter the companion's SDK `query()` inside its independent
guardian. The SDK owns the request/control protocol; the adapter still owns
the native process group, bounded stream validation, activity, usage and durable
receipts. SDK completion alone cannot establish physical cleanup. The configured
subscription policy also applies to foreground, PTC and background delegation.

`FLYWHEEL_CLAUDE_BIN` remains authoritative; its default is `claude`. A missing or
incompatible local executable does not fall back to the SDK's bundled runtime.
Installing the SDK may include an optional platform binary, but does not change
the selected executable, settings directory or account.

The local CLI resolves its own existing login in `HOME` / `CLAUDE_CONFIG_DIR`.
The plugin does not implement OAuth, copy tokens, select a different account or
modify account settings. A channel label does not establish authentication: the
same initialized native process must supply the evidence below. Existing API
configurations remain usable when they deliberately omit subscription policy;
they are not a fallback for a subscription invocation.

### Native subscription admission

With `subscription:{"version":1,"mode":"subscription-only"}`, the 0.7.0 adapter
keeps the SDK user-message generator closed until all these checks pass:

1. Reject known API credentials, external providers/endpoints, configured
   fallback models, fast mode and conflicting effort settings. Freeze an explicit
   first-party model ID. Read only selected native settings, never credential
   files or exported OAuth tokens.
2. Initialize the configured CLI through the exact **SDK 0.3.326** and check the
   actual CLI version and available effort-capability information.
3. Call `accountInfo()` and the public experimental
   `usage_EXPERIMENTAL_MAY_CHANGE_DO_NOT_RELY_ON_THIS_API_YET({skipBehaviors:true})`
   on that **same query/process**, without a task prompt or a `/usage` model request.
4. Require matching first-party `pro`, `max`, `team` or `enterprise` evidence,
   available native plan limits and an explicit
   `rate_limits.extra_usage.is_enabled === false` in the pinned response shape.
   Only then release the task message.

Missing controls, unknown response shapes, unavailable billing evidence and
enabled extra usage all block task input. The preflight has its own 10-second
bound and shares cancellation and any shorter task deadline. It never retries
through another model, provider, API account or credits path. The invocation
passes `forceLoginMethod:"claudeai"`, `fallbackModel:[]`, `fastMode:false` and the
native fast-mode disable control; it does not rewrite configuration files.

The receipt records `status:"verified"`, `source:"sdk-usage-0.3.326"`, the observed
subscription type, `extraUsageEnabled:false`, the resolved model and
`scope:"pre-prompt-snapshot"`. This is an observed admission snapshot, not an
atomic lock on subsequent account changes or a guarantee of model entitlement.
Fable access and quota depend on the actual account; a refusal remains a failure.
No email, token, balance or raw account/usage response is returned.

Separate native remote Review and interactive Team creation, Team send and Team
attach cannot perform this same pre-prompt check and are blocked under the
policy. Recovery, capture, resize and cancellation of existing owned jobs remain
available. Ordinary SDK delegation can still use its explicitly enabled native
Agent/Workflow/Advisor controls. Authorized hooks and external tools are outside
the account billing checks. See the companion's
[full subscription contract](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/main/docs/subscription.md)
and the [native capabilities guide](native-capabilities.md).

## Streaming and readable session names

The query path checks `claude --help` without a model prompt before dispatch.
It requires `--include-partial-messages` and `--forward-subagent-text`, plus
`--name` when creating a named persistent session.
The installed CLI's actual capabilities determine admission; a version label
alone is insufficient. An unsupported CLI fails before receiving task text.
The capability probe and query share the original wall budget and cancellation.
The SDK then checks `claude_code_version` in the same process's initialize
response and only releases task input for a stable **2.1.325 or newer** runtime.
This is the minimum validated release for this adapter, not a claim about when
each upstream feature first appeared. Missing, old or prerelease versions fail
before task input. Since that native process has already started, its actual PID
and cleanup facts remain in the receipt. Subscription rejection keeps `execution.turnDispatch` at
`not-dispatched`; starting the CLI for inspection is separate from sending a task.

Real root/child partial, tool and task activity refreshes Flywheel's idle budget
without forwarding private reasoning or manufacturing timer heartbeats. Native
state deduplication does not suppress activity. Long streams of small deltas do
not consume a cumulative raw-output budget; individual frames and retained
results remain bounded. Queueing and opaque tools can still be silent. See
[activity, timeouts and recovery](session-recovery.md) before retrying a stopped call.

New persistent histories receive the Flywheel task/role title through the SDK's
`title` option. Resume keeps the existing title, including a user's rename. Outside
Flywheel, the plugin uses the explicit delegation label or a readable workspace
fallback. Naming does not send a separate model request.

Fresh managed calls now retain a new named native history on every invocation,
without supplying an old session ID. Scoped calls can continue a confirmed ID;
retention alone does not authorize reuse. The official Claude picker excludes
print/SDK histories, so use the saved root UUID and the supported CLI/Desktop
entry points in [native history visibility](session-recovery.md#find-the-session-in-its-native-client).

The 0.7.0 companion rejects persistent creation/resume when its effective
environment enables `CLAUDE_CODE_SKIP_PROMPT_HISTORY`, before a CLI probe or
task starts. It preserves explicit ephemeral calls and never changes that user
preference. Native true values are `1`, `true`, `yes` and `on`, ignoring case and
surrounding spaces. Flywheel's Go environment allowlist already excludes this
variable; the guard also protects direct machine, DSH delegation and interactive
team callers. It does not attest arbitrary storage failures or settings loaded
later inside the CLI.

## Define a profile, then choose its stages

The complete [profile template](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.profiles.example.json)
includes `opus-plan`, `opus-ui` and an unreferenced `fable` profile. This fragment belongs in your
existing complete configuration; it is not a replacement file:

```json
{
  "profiles": {
    "opus-plan": {
      "harness": "claude-code-pro",
      "model": "claude-opus-5-5",
      "channel": "claude-native",
      "thinking": "high"
    }
  },
  "channels": {
    "claude-native": { "harness": "claude-code-pro", "env-key": null }
  },
  "subscription": { "version": 1, "mode": "subscription-only" },
  "permissions": { "mode": "full-access" },
  "sessions": { "mode": "scoped", "policy-version": 1 }
}
```

The preset supplies canonical IDs. Under subscription policy the companion
freezes `opus`/`best` to `claude-opus-5-5`, `sonnet` to `claude-sonnet-5-5`, `haiku`
to `claude-haiku-5-5` and `fable` to `claude-fable-5-2`; a supported explicit
first-party family override in the native environment is recorded separately.
Dynamic `default`/`opusplan` selection is not admitted. Full IDs must match native
initialization and the observed root assistant model throughout the stream. A
switch away from the frozen model fails even if the model later switches back.
`result.modelUsage` can include history, auxiliary calls and subagents; it is
labeled cumulative evidence and is not a current-turn fallback trace.

Request efforts are low, medium, high, xhigh and max, with high as the default.
The adapter rejects explicit environment overrides, known native caps and
unsupported levels reported by the CLI catalog. Missing effective-effort evidence
stays unknown; an accepted input is not invented proof of the actual effort.

Reference the profile from the selected
`loops.plan/review/judge/intake/garden` or Execute `routes.*` binding, for example
`{"profile":"opus-plan"}`. The preset selects `opus-plan` for Plan and `opus-ui`
for frontend/multimodal execution. Clear any explicit inherited `provider`: that field identifies a DSH
native LLM adapter. Neither `subagent_claude_code` nor `claude-code-pro` belongs
there as a substitute for the harness field.
When switching from Kimi Plan, remove its `resident-session` field to use
`sessions.mode`. Claude rejects `resident-session:true`; an explicit Plan
`resident-session:false` still disables scoped history reuse, preserving that
existing opt-out.

Restart the engine after changing the configuration. Inspect the effective file
and expanded bindings:

```sh
flywheel config inspect --cwd /repo --json
flywheel doctor --cwd /repo --json
flywheel-claude-code-pro-worker --doctor
```

An unused profile does not require the optional adapter. A selected binding makes
doctor check the worker, exact public API and local CLI. These checks do not make
model requests or prove account availability, quota or live inference. To inspect
the actual subscription admission without submitting a task, use the companion's
public `inspectSubscription()` with the intended command, cwd, model, effort and
environment; its result distinguishes `verified`, `blocked` and `unknown`.
Execution independently repeats that inspection in its own process.

### Set effort for named native agents

Each `native-profiles.<name>.options.agents.definitions.<agent>.effort` accepts
`low`, `medium`, `high`, `xhigh`, or `max`. For example, add this fragment to the
same complete configuration, then add `"native-profile":"claude-agents"` to the
existing `opus-plan` profile:

```json
{
  "native-profiles": {
    "claude-agents": {
      "harness": "claude-code-pro",
      "options": {
        "version": 1,
        "agents": {
          "enabled": true,
          "definitions": {
            "scout": {
              "description": "Inspect the relevant code",
              "prompt": "Read the target and report concrete findings.",
              "model": "sonnet",
              "effort": "medium"
            }
          }
        }
      }
    }
  }
}
```

The parent binding's `thinking` remains its own `--effort`; the child definition
travels unchanged through SDK `agents` in initialize; interactive Team retains native `--agents`. An omitted child effort stays
absent so the CLI keeps its native inheritance behavior. Flywheel does not fill
it from the parent's value or invent a child default. Invalid values fail during
configuration loading, before a worker is dispatched.

The same definition field is available in the 0.7.0 desktop plugin's `native`
settings and PTC override. Independent interactive Team is unavailable under
subscription policy. Direct
Flywheel workers still read their selected native profile, not desktop settings.
Passing an explicit effort proves the requested CLI input; tools-only init
evidence does not attest the effort actually used inside a child model.

## Permissions and role history

The adapter requires explicit full access for its selected binding, mapped to
native `bypassPermissions` and noninteractive invocation. Configure it globally
with `permissions.mode:"full-access"` or use a supported binding override; see
[Configuration](configuration.md). Native plan/acceptEdits
approval policies are not OS read-only/workspace-write sandboxes; role-based
requests therefore fail before dispatch. Reviewer and planner responsibilities,
contract boundaries, and before/after candidate guards remain enforced by Go.

Main-task Plan, Execute, Review, and Judge use the existing fresh/scoped policy.
Scoped repairs resume only a confirmed session owned by the same task, scope,
and role. Review never shares the executor's session; a second opinion is also
independent. Resume supplies the exact session ID, without discovering the latest
history or silently starting a fresh task after a failed resume. Intake and Garden
can select the backend, but remain independent advisory calls without the main
task's role-session ledger.

Success requires a complete valid stream, correlated request/session/configuration,
normal physical process exit, and confirmed cleanup. Disconnects, cancellation,
timeouts, malformed frames, duplicate terminal results, abnormal exit, and unknown
cleanup never qualify. Dependency and input failures proven to occur before
machine invocation are not-dispatched with known zero consumption, so fixing the
environment can permit a later attempt. After invocation, a missing trustworthy
outcome must stop replay even if cleanup is confirmed.

For project membership, execution-directory selection, and worktrees, see
[Workflow](workflow.md). These locations remain part of the recorded invocation
identity; changing them does not authorize replay of an unresolved call.

### Durable recovery

Before dispatching Claude, an independent supervisor owns a durable journal and native guardian. Go persists and acknowledges intent/possible checkpoints before prompt submission. Worker loss cancels unacknowledged steps and initiates native cleanup; complete terminal receipts are durable before notification. Public `recoverMachine` reads only that invocation and never starts the CLI or resubmits a prompt.

The ordinary stream's Claude init arrives after the prompt; SDK initialize and
subscription inspection occur before it. There is no fabricated pre-dispatch
thread-ready or native turn-accepted event. Actual model, cwd and permission
checks still apply at terminal admission. Supervisor loss, corruption or missing
original candidate proof can leave execution blocked. See [recovery and context
quality](session-recovery.md). Real-account quality, cache and performance require
separate [paired evidence](session-evaluation.md).

## Frozen image inputs

An engineering packet can declare `input-images:{"version":1,"images":[...]}`.
Each entry supplies a repository-relative `path`, a lower-case `sha256` digest,
and `media-type`. Execute resolves these references from the task repository,
validates the frozen bytes, and passes actual base64 attachments through the
public machine API and official SDK. Select a Claude multimodal Execute binding
for such a packet; other backends are refused before dispatch. Plan, Review and
Judge do not implicitly receive the image bytes.

Flywheel accepts one to four PNG, JPEG, GIF or WebP images, at most 4 MiB each
and 8 MiB combined. Each raster is decoded and checked against the declared
format, a maximum edge of 8,192 pixels and 16 million pixels in total. Paths must
refer to ordinary files within the repository without symlink components.
The packet freezes the file paths, MIME types and SHA-256 digests. Changing a
referenced file requires updating the approved packet instead of silently
changing the next model input.

The Claude worker permits a 16 MiB start frame, including its newline, to carry
these encoded attachments; prompt text retains its separate 1 MiB limit. The
other workers retain their existing 1 MiB input frame. Images become native
image content blocks after the text prompt, not file-path descriptions.

The companion computes `requested.inputImages` as ordered MIME type, decoded
byte count and SHA-256 metadata. Its durable journal records the same manifest
under `context.inputImages` before the first dispatch checkpoint. Flywheel
compares that evidence with the invocation's frozen manifest, including during
read-only recovery. The invocation state and input manifest contain no image
contents; the native CLI may retain its own authorized conversation history.
An unresolved invocation is reconciled before reading or sending another image.
Recovery never substitutes a modified attachment or authorizes an automatic retry.

## Usage and subscription cost

CLI input_tokens excludes cache. For complete reports, Flywheel adds cache read
and cache creation to input, then adds output to calculate total tokens. Missing
counters remain partial/unknown instead of becoming zero. Each resumed invocation
is accounted once, without rebilling historical totals.

The raw receipt retains CLI total_cost_usd as reportedCostUSD, an API-equivalent
estimate rather than a subscription bill. costUSD stays null and costStatus stays
unknown. Existing soft/hard budget behavior remains in effect.

An explicit user `pricing` table can separately produce an estimate in metrics
and budgets. The estimator subtracts cache read/creation from inclusive input
before pricing disjoint buckets, avoiding duplicate charges. This estimate does
not use reportedCostUSD, rewrite the raw receipt, or establish a subscription bill.
