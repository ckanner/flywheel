# Optional Claude Code Pro backend

[中文](../claude-code-pro.md) · [Configuration](configuration.md)

Select `harness: "claude-code-pro"` to call the public machine API of
[`dsh-subagent-claude-code-pro`](https://github.com/ckanner/dsh-subagent-claude-code-pro).
The plugin runs the user's installed Claude Code CLI, with its native tool loop
and login environment. Go retains scheduling, role sessions, verification,
budgets, and delivery.

**Source and release boundary:** the integration requires plugin **0.3.2,
machine protocol 1**. Version 0.3.1 only exposes the DSH tool interface and lacks
the required machine receipt. The companion source version does not establish
an npm publication. Flywheel source remains version 0.28.5; the new source does
not update an already installed artifact. No default stage or route selects Claude.

## 1. Prepare the optional dependency

Use Node.js 22.18.0 or later on macOS or Linux, with the user's installed and
authenticated `claude` CLI accessible to the process starting the engine.
Flywheel does not copy credentials or sign in on the user's behalf.

Flywheel ships `flywheel-claude-code-pro-worker` and declares the exact plugin as
an **optional peer**. Other backends work without it. To select Claude, install
`dsh-subagent-claude-code-pro@0.3.2` in the same Node dependency tree or global npm
prefix as Flywheel. Installing it only in a DSH desktop profile does not establish
that the independent worker can resolve it.

After publication, install that exact release. Before publication, run
`npm pack --ignore-scripts` in the reviewed plugin checkout and install the actual
resulting `.tgz` into the same prefix, alongside a newly built Flywheel candidate.
Do not relabel 0.3.1 or substitute it for the missing machine interface. This
source-candidate validation does not promote npm latest.

| Variable | Purpose |
|---|---|
| `FLYWHEEL_CLAUDE_CODE_PRO_WORKER` | Explicit worker override; npm normally resolves the matching worker |
| `FLYWHEEL_CLAUDE_BIN` | Claude CLI command or absolute path; defaults to `claude` |
| `CLAUDE_CONFIG_DIR` | Optional native configuration/login directory, interpreted by Claude |

Go forwards only its base environment allowlist and credentials explicitly
selected by the channel. A subscription channel with no env-key does not inherit
an ambient Anthropic API key. Supported explicit credentials are passed through
the machine runtime. The bridge does not serialize environment credential values
into requests or diagnostics; text returned by the CLI is preserved as received.

## 2. Define a profile, then choose its stages

The complete [profile template](../../templates/flywheel.profiles.example.json)
includes an unreferenced `claude-sonnet` profile. This fragment belongs in your
existing complete configuration; it is not a replacement file:

```json
{
  "profiles": {
    "claude-sonnet": {
      "harness": "claude-code-pro",
      "model": "sonnet",
      "channel": "claude-subscription",
      "thinking": "high"
    }
  },
  "channels": {
    "claude-subscription": { "env-key": null }
  },
  "permissions": { "mode": "full-access" },
  "sessions": { "mode": "scoped", "policy-version": 1 }
}
```

Use native `sonnet`, `opus`, or `haiku` aliases, or a full model ID available to
your account. Full IDs must match the observed CLI initialization value; the
three aliases must resolve to the corresponding family. Flywheel does not pin a
changing model catalog. Supported request efforts are low, medium, high, xhigh,
and max, with high as the default. A particular CLI or model can still reject a
choice; Flywheel never silently substitutes another model or effort. Where the
CLI cannot attest effective reasoning effort, only the requested value is recorded.

When you decide the use case, reference the profile from the selected
`loops.plan/review/judge/intake/garden` or Execute `routes.*` binding, for example
`{"profile":"claude-sonnet"}`. This is a configuration capability, not a new default
allocation. Clear any explicit inherited `provider`: that field identifies a DSH
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
doctor check the worker, exact public API, and local CLI. These commands do not
make model requests or prove account availability, quota, or live inference.

## 3. Permissions and role history

The initial adapter requires explicit `permissions.mode:full-access`, mapped to
native `bypassPermissions` and noninteractive invocation. Native plan/acceptEdits
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

Before dispatching Claude, the plugin establishes an independent guardian and
an IPC lifeline to the worker. Worker death or disconnect makes the guardian
stop and clean the native process group before exiting. Normal calls also await
guardian exit. This provides physical cleanup; a lost receipt remains unknown.

This delivers normal terminal-backed continuation and recovery of receipts already
saved by Flywheel. It does not provide early durable acknowledgments, read-only
live recovery, or arbitrary crash recovery. No live quality, cache, or performance
improvement is inferred from offline tests.

## 4. Usage and subscription cost

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
