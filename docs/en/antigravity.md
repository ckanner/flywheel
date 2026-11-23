# Antigravity and Gemini

[Documentation](../README.md) · English · [简体中文](../zh-CN/antigravity.md)

The default **Antigravity desktop connection is official CLI ACP** in the 0.39.0 source candidate. Flywheel bundles `@ckanner/dsh-subagent-antigravity@0.3.0` and automatically installs its human-operated ACP host and view. The official `agy` owns inference, tools, permissions and its internal loop. Flywheel does not send a replacement/append system prompt, Preset, PTC tools, managed agent or hidden control prefix through this path.

## Default interactive ACP

Install the official **agy 1.5.14**, then open the **Antigravity** tab in a DSH conversation attached to a committed Git repository. A blank conversation can use the human command `/flywheel-antigravity {"version":2,"action":"connect"}`. The page provides environment checks, native login, continuous messages, the actual runtime model/mode options, permission decisions, typed image attachments when advertised, Stop, Disconnect and exact Resume. It uses an authenticated Remote directly; the parent DSH model is not invoked. Prompts and login codes are not routed through command logs.

The runtime is admitted by exact official executable hashes for macOS/Linux x64/arm64, not by a version banner alone. Connections disable CLI self-update and fail visibly on binary/profile drift. A login action launches the same `agy acp --login`; Google and the native helper own credential exchange/storage. Helper exit 0 is not an entitlement check: reconnect explicitly to establish the session. The client does not read token files or fall back to a different account, API route, headless adapter or Registry distribution. Personal interactive prompts require a Gemini model selected from native options. Native user rules remain active, and native permission/sandbox settings are not overwritten by legacy plugin defaults.

Each new session uses a separate `antigravity/<record-id>` branch and Git worktree from the source **committed HEAD**. The path and starting commit are visible. Uncommitted source changes are not copied; the worktree and its changes remain after disconnect for deliberate review/integration. Flywheel's desktop mutation entry points reject these owned worktrees, including aliases and subdirectories. This guard is not a global filesystem lock on independently launched programs or the standalone Go CLI.

A prompt operation is written durably before dispatch. Only one unresolved prompt is admitted; native post-root tool/child/permission events continue on the same connection. An interrupted or unconfirmed operation remains unknown and is never replayed. Disconnect requires the negotiated native close acknowledgement and owned process-group cleanup; root exit alone is insufficient. Exact resume preserves session/workspace/runtime identity. The activity/stop hooks keep outstanding work visible to DSH workspace archival. No ACP result writes Go `state.json` or proves a Flywheel task/Review complete.

The full [ACP client contract](https://github.com/ckanner/dsh-subagent-antigravity/blob/7d5056c81688cc9408f3be32fc38ac7388b97377/docs/ACP.md) describes storage, authentication, recovery and remaining limits. This source candidate has offline client/host/package evidence and unauthenticated Linux x64 CLI probes; personal-login, inference, image delivery and native detached-work cleanup require separate live acceptance. See the [implementation record](../validation/2026-11-22-antigravity-acp.md).

## Explicit legacy machine transport

The rest of this guide describes the existing `harness:"antigravity"` **headless stream-json machine interface**, pinned to the companion 0.3.0 while retaining machine/lifecycle/activity protocol 1. Go still owns automated Plan/Execute/Review, JEV, gates and delivery. Those workflows are not silently converted to personal ACP and are not validated as permitted third-party personal-login uses by this change. The standalone old task panel and `subagent_antigravity` registration now require explicit `transport:"headless"`.

## Installation and capability checks

Use Node.js 22.18+ on macOS or Linux, with official `agy` **1.5.7+** already
installed and signed in through its normal native login. Orchestrator and explicit
subagents/Swarm require **1.5.12+** and the needed public capabilities. The plugin never
downloads the executable, implements login, copies account tokens or changes
global settings. This release supervises POSIX process groups and rejects Windows.

Flywheel includes `flywheel-antigravity-worker` and the exact reviewed companion
archive recorded in
[vendor metadata](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/antigravity-acp.json).
The source is reviewable in
[`dsh-subagent-antigravity`](https://github.com/ckanner/dsh-subagent-antigravity)
as a standalone npm package. Package identity, installed candidate bytes, public
registry availability and real-account acceptance are separate checks. The
[2026-11-22 registry verification](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/registry-verification-2026-11-22.json)
confirms that the published 0.2.1 archive matches the reviewed bytes. An older
package with another machine contract cannot substitute for 0.3.0.

| Environment variable | Purpose |
| --- | --- |
| `FLYWHEEL_ANTIGRAVITY_WORKER` | Explicit worker executable override |
| `FLYWHEEL_AGY_BIN` | Official native CLI command or absolute path; default `agy` |
| `HOME` | Native account and settings environment; credentials remain owned by the CLI |

```sh
flywheel config inspect --cwd /repo --json
flywheel doctor --cwd /repo --json
flywheel-antigravity-worker --doctor
```

The worker doctor verifies the exact installed companion and public machine,
lifecycle and evidence-only contracts. The legacy companion's `doctor()` and headless panel
**Check environment** additionally run bounded `--version`, `--help` and `models`
probes using the execution environment. These checks do not submit model prompts
and do not establish remaining quota, entitlement or model answer quality.

## Model profiles and stage allocation

The [complete template](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.profiles.example.json)
has one subscription policy and native account channel. This is a fragment to
merge into an existing complete configuration:

```json
{
  "subscription": { "version": 1, "mode": "subscription-only" },
  "channels": {
    "google-subscription": { "harness": "antigravity", "env-key": null }
  },
  "profiles": {
    "hydrogen": {
      "harness": "antigravity",
      "model": "gemini-4-hydrogen-high",
      "channel": "google-subscription",
      "thinking": "high"
    },
    "neon": {
      "harness": "antigravity",
      "model": "gemini-4-neon-high",
      "channel": "google-subscription",
      "thinking": "high"
    },
    "carbon": {
      "harness": "antigravity",
      "model": "gemini-4-carbon-high",
      "channel": "google-subscription",
      "thinking": "high"
    }
  }
}
```

| Profile | Exact model / effort | Default responsibility |
| --- | --- | --- |
| `hydrogen` | `gemini-4-hydrogen-high` / high | Routine implementation |
| `hydrogen-local` | `gemini-4-hydrogen-medium` / medium | Compact design planning; optional subscription evaluator |
| `hydrogen-summary` | `gemini-4-hydrogen-low` / low | Intake summaries |
| `neon` | `gemini-4-neon-high` / high | Complex implementation and evidence-only investigation analysis |
| `carbon` | `gemini-4-carbon-high` / high | Full design planning |
| `carbon-explore` | `gemini-4-carbon-medium` / medium | Explicit exploration override |

For ordinary Gemini models the suffix and `thinking` must agree, and the selected model must appear in
the actual CLI catalog. An unavailable model or incompatible effort fails;
Flywheel does not silently choose another provider. The requested effort remains
separate from observed native configuration: an init that omits effort cannot
attest its effective value. See [model allocation](model-policy.md) and
[workflow](workflow.md) for the complete cross-provider plan.

Ordinary Antigravity execution requires an explicitly authorized full-access
binding. Flywheel maps it to native `accept-edits` plus `always-proceed` and
`sandbox:false`. The opt-in subscription migration preserves existing user
permissions. An incompatible preserved permission produces a visible admission
error; the adapter does not silently expand it. Native image attachments are not
supported by this backend contract. Multimodal implementation uses the configured
Claude route.

### Images and the automated input boundary

**An image is not required to use Gemini or Swarm.** The restriction applies when the packet explicitly requires original image bytes. The official [interactive CLI](https://antigravity.google/docs/cli/prompting/) accepts pasted media, while the [headless stream-json input](https://antigravity.google/docs/cli/headless/) currently accepts text content. Interactive media support does not establish automated image delivery or child-agent consumption in this adapter.

The Flywheel task form can inspect and preview explicitly selected repository images, retaining paths and SHA-256 as unreviewed brief evidence. It does not turn a text path into an attachment or give every stage the original image. Reviewed original-image packets continue to use the verified Claude SDK transport; text-only engineering and investigation tasks can use Gemini, including eligible native collaboration.

## Orchestrator, subagents and Swarm

Orchestrator is a model choice. The official CLI hands work between its named
planner and Hydrogen within a conversation. All three fixed-effort choices use
`thinking:""`; the adapter omits `--effort` and rejects a nonempty effort:

| Model | Native planner | Effort |
| --- | --- | --- |
| `gemini-pro-orchestrator-neon` | Neon | Fixed |
| `gemini-pro-orchestrator` | Carbon | Fixed |
| `gemini-pro-orchestrator-argon` | Argon | Fixed |

Subagents and Swarm are native execution strategies, configured separately from
the model. Add this fragment to a complete Flywheel configuration and select the
profile through a stage or candidate binding's `native-profile`:

```json
{
  "native-profiles": {
    "antigravity-subagents": {
      "harness": "antigravity",
      "options": {"version":1,"mode":"subagents","maxObservedAgents":4}
    },
    "antigravity-swarm": {
      "harness": "antigravity",
      "options": {"version":1,"mode":"swarm","maxObservedAgents":4}
    }
  }
}
```

`native` preserves implicit CLI behavior and does not mean “children disabled.”
`subagents` requests delegation with results collected in the current invocation;
admission checks the native delegation tool. `swarm` uses the official `/swarm`
entry. Only the adapter supplies that command; user content is encoded as a JSON
payload with slash characters escaped. Other modes keep slash commands disabled.
Explicit `subagents` succeeds only when the stream actually exposes child
conversation metadata. `swarm` is accepted when the adapter-owned `/swarm` root
invocation returns a normal successful terminal even if `agy` 1.5.14 exposes no
child metadata; the receipt records `collaborationObserved:false` and
`children:"unknown"`. A request, an Orchestrator model, or a successful root
terminal alone is not child evidence.

`maxObservedAgents` accepts 1–8. It stops the invocation after more distinct
observed child conversations than permitted, including observed verifier/monitor
members. It is **not** a native concurrency or whole-tree cap; receipts report
`hard:false`. Child roles, models, states and usage remain unknown when the CLI
does not expose them. The UI distinguishes requested collaboration from observed
members. The plugin does not fabricate a complete Swarm task board. Retained
child metadata has a cumulative size bound; overflow keeps verified member IDs,
marks `metadataTruncated:true` and fails the invocation rather than silently
reporting an incomplete successful result.

Flywheel Execute requires Full, no declared model image inputs, and a reviewed `collaboration` contract
before admitting a Swarm binding, whether selected by JEV or static configuration.
The contract contains 2–8 independent tasks with disjoint write scopes inside the
packet's approved `schedule.writes`. Planning may use foreground subagents to
collect read-only research; it does not run execution Swarm. Native verification
does not replace independent Flywheel Review or exact-version plan approval.
See [workflow](workflow.md) and [model selection](model-policy.md).

Sources: [official models](https://antigravity.google/docs/models),
[CLI subagents](https://antigravity.google/docs/subagents?tab=cli) and
[Swarm](https://antigravity.google/docs/swarm).

## Investigation uses approved evidence only

The investigation workflow separates deterministic evidence collection from
model analysis. Go collects the authorized command/URL evidence and supplies it
to Neon. The analyst does not receive authority to run another command or fetch
another URL just because its prompt requests more evidence. Missing evidence is
reported for a later authorized collection step.

For this phase Flywheel passes `toolPolicy:"evidence-only"`, native `plan`,
`request-review` and the explicit `loops.investigate` read-only binding. The
plugin creates a uniquely owned temporary workspace custom agent with empty
tool/MCP/skill/plugin lists and no subagent capability. It selects that agent
through the official `--agent` option and waits for real native init before
writing the task:

- `init.agent` must match the exact generated agent identity.
- `init.tools` must exist as an array containing the native tool catalog.
- The catalog may be nonempty; the managed custom agent file carries the empty
  allowlist, and every observed tool or subagent step rejects the invocation.
- Orchestrator models and native collaboration profiles are rejected for this phase.
- Missing agent identity, absent catalog, malformed init or later tool/subagent events reject the invocation.

The [official headless contract](https://antigravity.google/docs/cli/headless/)
defines the tool catalog and the agent/model fields when their flags are selected.
The [custom-agent contract](https://antigravity.google/docs/subagents/) defines the
managed allowlist and workspace discovery. A CLI that cannot identify the managed
agent or catalog is not admitted. Plain plan mode or a prompt instruction is
insufficient. A live `agy` 1.5.14 check confirmed the managed empty allowlist refuses
a requested shell command; that recorded check is separate from the offline
regressions added for Flywheel's receipt acceptance.

Flywheel accepts the result only when `observed.agent` and the adapter's
`native.managedAgent` record match this invocation's run ID and exact workspace
path, contain a valid content digest, and confirm managed-file cleanup. The
pinned adapter verifies the policy file's content and ownership before dispatch
and removal. Go verifies that attestation's binding, without independently
reconstructing the policy from its digest or reopening the already removed file.
Read-only recovery uses the same checks. Subscription-backed JEV decisions use
this evidence-only contract as well.

After a successful Antigravity receipt and clean process/session completion have
been verified, Flywheel does not reclassify its raw stdout by diagnostic words.
For example, `extraBilling:"unknown"` is subscription metadata, not a quota
failure. The original receipt stays intact; typed failures, stderr checks and
physical timeout/cancellation/cleanup guards still apply. Report and verdict
contents must independently satisfy their schemas and evidence requirements.

Creation uses an exclusive path, invocation ownership and content identity;
cleanup removes only that unchanged invocation-owned file. Existing user agents
and global settings are never overwritten. This is native tool exclusion, not
an OS filesystem sandbox: native initialization/customization discovery still
occurs. The transcript and receipt keep that distinction explicit.

## Native subscription controls

Every Antigravity call requires the versioned subscription policy. The adapter
rejects API/provider routing, ADC, GCP and custom-model configurations, filters
alternate credential/endpoint environments and checks native `useG1Credits:false`
(including its documented default). That setting disables using AI credits after
plan quota runs out. Relevant settings are fingerprinted and rechecked around
dispatch and completion; drift prevents successful acceptance.

These checks produce `subscription.status:"verified-local-controls"`,
`creditsFallback:false` and a configuration fingerprint. Actual account identity,
entitlement, remaining quota and extra billing remain unknown when not exposed by
the official interface. A catalog entry is not quota evidence. Login or quota
failure stops the call without an API, credits or alternative-model fallback.
Hooks and external tools authorized by the native runtime are not covered by an
account-wide billing lock.

Sources: [native login and API/ADC routing](https://antigravity.google/docs/install/),
[CLI settings](https://antigravity.google/docs/cli/reference/) and
[credit fallback](https://antigravity.google/docs/cli/commands/credits).

## Explicit legacy headless use inside DSH

For legacy delegation, explicitly set the standalone plugin row to `transport:"headless"`. Its default is the interactive ACP view described above. Install the reviewed candidate into the intended DSH profile:

```sh
dsh plugin --profile <profile> add /absolute/path/ckanner-dsh-subagent-antigravity-0.3.0.tgz
```

Open the **Antigravity** conversation view, select the workspace, label, complete
task, model, effort and native strategy, then **Start task**. Fixed-effort
Orchestrator choices use an empty effort. **Refresh tasks**, **View result** and
**Cancel** operate on the session's owned invocation. **Check environment** and
saved defaults are available on the plugin's settings/panel. The view calls the
public DSH command directly; it does not first ask a parent model to delegate.

The equivalent command supports versioned `start`, `list`, `status`, `result`,
`cancel`, `doctor` and `defaults` actions:

```text
/antigravity {"version":1,"action":"start","label":"Investigate timeout","prompt":"Analyze this collected evidence…","model":"gemini-4-neon-high","effort":"high","toolPolicy":"evidence-only"}
/antigravity {"version":1,"action":"start","label":"Independent modules","prompt":"Implement the two modules with separate write scopes, collect both results and verify them.","model":"gemini-4-neon-high","effort":"high","native":{"version":1,"mode":"swarm","maxObservedAgents":4}}
/antigravity {"version":1,"action":"list"}
/antigravity {"version":1,"action":"result","invocationId":"<returned UUID>"}
```

Agent delegation requires the preset to explicitly register
`subagent_antigravity` and pass the plugin's preset allowlist. Background
delegation uses DSH jobs with `job_output` and `job_kill`; direct panel tasks use
their owner-bound task store. Neither route can widen native permissions through
a tool argument. Config changes affect future invocations.

Calling a subagent from an existing DSH conversation still leaves that parent
conversation on its configured provider. The direct panel/command avoids a
parent model call. This package is an Agent transport, so do not register it as a
conventional DSH LLM provider. See the standalone
[plugin guide](https://github.com/ckanner/dsh-subagent-antigravity/blob/main/README.md)
for the DSH schemas, machine API and wrapper.

## Legacy machine lifecycle, history and usage

Each call owns one CLI process and one text turn. Native conversations are
retained: `fresh` starts a new conversation, while scoped resume requires the
exact owned, clean, terminal conversation with matching directory, model, effort,
permissions, tool policy, complete native options and native settings. Concurrent and unresolved resumes
are rejected. There is no “latest session” discovery, prewarm pool or implicit
replay. The panel preserves a readable local label and native conversation ID;
it does not claim to rename the native conversation.

Before task input, the supervisor writes durable intent/possible-dispatch
checkpoints and waits for Go's acknowledgement. Nonterminal events never claim
cleanup. Every local invocation end records a terminal checkpoint; native
`terminalReceived`, outcome and provider-work evidence independently determine
whether the provider completed. A subsequent cleanup checkpoint proves only
local process-group cleanup. Parent loss, cancellation and timeout terminate
owned local work and preserve uncertainty about unobserved provider work.
`recoverMachine` reads the original journal and never runs the CLI or resends input.

Success requires a native successful terminal without a nonempty `finished_reason`, valid output/schema, matching
observable configuration and confirmed cleanup. Exit zero, a soft permission
denial or a timed-out connection cannot establish success. Tokens retain the
native conversation/partial/unknown scope; actual cost remains `null`, not zero.
CLI invocation completion and observed child metadata do not prove completion or
cleanup of the entire remote child tree. Unobserved child usage, remote completion, image/PDF input and artifact return
are not claimed by this release.

Offline protocol, cancellation, parent-death, evidence-only, public API and actual
candidate-install tests are separate from a live account or macOS acceptance.
See [recovery](session-recovery.md), [release validation](releasing.md) and the
plugin's [validation record](https://github.com/ckanner/dsh-subagent-antigravity/blob/main/docs/VALIDATION.md).
