# Native harnesses, PTC and LCU: complete integration design

English · [简体中文](2026-11-13-native-harness-capabilities.md) · [Plans](README.md) · [User guide](../en/native-capabilities.md) · [Architecture](../design.md)

**Status: implementation candidates across six repositories, delivered as PRs.** Created on 2026-11-13; revised on 2026-11-14 for Flywheel `0.34.0`. This document describes the implemented contracts and their limits. The [validation record](../validation/2026-11-14-native-harness-implementation.md) distinguishes source tests, packed installation, publication and acceptance on the user's Mac.

The [previous proposal](https://github.com/ckanner/flywheel/blob/9c95a69490c7f38d39ede055a00adc56bdce087c/docs/plans/2026-11-13-native-harness-capabilities.en.md) remains available at its original commit. This revision replaces the staged, limited-helper rollout with complete entry points for each supported native harness. Each adapter reports the permissions, limits and lifecycle facts it can actually establish.

## 1. Decision and architecture

The user continues using the DSH desktop and the existing `heavy` or `daily` Agent Preset to prepare work and control Flywheel. Go runs formal loops. The DSH route uses a local SDK runtime, Codex and Claude use their Pro plugins' public machine APIs directly, and native Kimi uses an owned local Kimi Server. Model, account/provider and harness are separate choices; a model name does not confer another harness's tools or collaboration runtime.

Every Plan / Execute / Review / Judge / Intake / Garden binding can reference a `native-profile`. Static profiles work with JEV disabled. With JEV enabled, existing explicit allowed sets continue to bound selection; JEV cannot invent a new account, backend or permission.

| Component | Implemented integration | Evidence required for completion |
| --- | --- | --- |
| DSH | Shared PTC composition, behavioral executor probe, subagents, Team, PTC Workflow and explicit Web tools | Matching policy, root result, owned children, complete journal and SDK cleanup |
| Kimi | Agent / Swarm / Tower over local HTTP and WebSocket APIs; independent Tower Git candidate | Root and continuation completion, quiescent tasks/agents; checked candidate transfer |
| Codex Pro | Native app-server agent controls, child threads, accounting, recovery and independent review | Effective config/read, current turn identities, owned child completion and guardian cleanup |
| Claude Code Pro | Agent definitions, Workflow, Advisor, independent interactive Team and remote Review | Actual init tools, current task-run identity and the appropriate local/remote lifecycle |
| LCU | Desktop and managed SDK mounting, lineage, GUI leases, epochs/handles, media and cancellation | Current caller/lease, turn cleanup, physical runtime exit and durable media evidence |
| ChatGPT provider | Real DSH attachment projection to Responses images, ordering and abort cleanup | Actual stored bytes and API input structure; visual understanding requires live acceptance |

Go alone owns `state.json`, frozen contracts, budgets, packet scheduling, candidate validation, independent Review, commits and delivery. Native task boards, internal reviews, terminal text and screenshots are evidence; they do not directly advance the state machine.

## 2. The DSH desktop, SDK and presets

### 2.1 The SDK is already part of the local execution path

The desktop host is the control plane: commands, model tools, workbench and association with the user's current Session. The worker runs local `dsh --profile sdk --patch <temporary overlay>` and drives formal role sessions over stdio JSON-RPC. The same `DSH_HOME` can resolve the same local account authorization. SDK use does not require switching an account-backed subscription to an API-key provider.

The active desktop Agent's private history, tools and preset are not automatically inherited by formal loops. Independent Review never inherits Executor history. A DSH runtime profile (`sdk`), an Agent Preset (`heavy`) and a Flywheel model profile (`profiles`) are different objects. Setting `FLYWHEEL_DSH_PROFILE=heavy` does not select the Agent Preset.

The public SDK creates Agents from its global plugin composition. This implementation uses that seam and the generator's `createSdkPtcComposition()`. Receipts identify `sdk-global-overlay` and a composition digest, without fabricating a named-preset Session header.

### 2.2 Shared policy and actual PTC admission

`dsh-preset-generator@0.4.0` continues generating both presets from the installed application's resources, with PTC selected for both. Its public `/composition` entry exposes the same policy to Flywheel. Static inspection recognizes old and new executor registration forms and records bundle identity; static version information alone does not establish runtime behavior.

Before the first paid prompt, the worker verifies the selected TypeScript executor, a single model-facing `run_code` tool, canonical JSON and `__handle` preservation, multi-image projection, and the actual output contract: `deferred-context-v1` or `inspect-image-v1`. The newer contract also checks the real `inspect` and `Image` identities. Probes use internal no-effect tools and a known image, with no model request or desktop operation.

DSH release and master can declare the same `0.2.2-rc.3` version while containing different PTC contracts. Admission therefore combines public compatibility ranges with behavioral evidence. A failed probe stops dispatch and retains its diagnosis; changing a version string cannot make an incompatible executor pass.

### 2.3 Owned native trees

Policy admission uses public Agent lifecycle hooks and verifies the live Agent registry, real Session parent, model/provider/effort, cwd and permission. Requested tool switches are checked against actual raw schemas. Team uses the public agent-team service, and Workflow uses the public workflow tool.

Local SDK child events and the policy's durable public `subagent/start` / `subagent/end` observations establish ownership together. External Pro activations also have provider/run/parent identities. Missing starts, changing ownership, damaged journals or unobserved settlement cannot be interpreted as complete work.

## 3. Configuration and loop behavior

### 3.1 Explicit native intent

`native-profiles` maps a name to `{harness, options}`; `options.version` is 1. The field authority is `templates/native-profiles.schema.json`. References are supported in `loops`, `routes`, model `profiles`, `judge-panel` and `execution-policy.bindings`. LOAD rejects unknown fields/references and mismatched harnesses. Before dispatch, the adapter additionally checks the installed capability interface and effective settings.

Omitting `native-profile` retains the ordinary harness path. Existing model-profile inheritance applies; an explicit empty string disables an inherited native profile. Both the name and actual options participate in invocation/history identity. Unrelated definitions do not invalidate a frozen execution policy; changing referenced options prevents reuse under the old identity.

The following is a **fragment to incorporate into a complete configuration**, not an install-time migration or automatic change to model routing:

```json
{
  "native-profiles": {
    "dsh-heavy-team": {
      "harness": "dsh",
      "options": {
        "version": 1,
        "preset": "heavy",
        "agents": {"enabled": true, "maxConcurrent": 4, "maxDepth": 2},
        "team": {"enabled": true},
        "workflows": {"enabled": true}
      }
    },
    "kimi-swarm": {
      "harness": "kimi",
      "options": {"version": 1, "mode": "swarm", "agents": {"enabled": true}}
    },
    "kimi-tower": {
      "harness": "kimi",
      "options": {"version": 1, "mode": "tower", "agents": {"enabled": true}}
    },
    "codex-agents": {
      "harness": "codex-pro",
      "options": {"version": 1, "agents": {"enabled": true, "maxConcurrentThreads": 4, "backend": "v2"}}
    },
    "claude-workflow": {
      "harness": "claude-code-pro",
      "options": {
        "version": 1,
        "agents": {"enabled": true},
        "workflows": {"enabled": true, "maxConcurrent": 4},
        "advisor": {"enabled": true}
      }
    }
  }
}
```

Add the reference, such as `"native-profile":"dsh-heavy-team"`, to the intended binding. The workbench Models and sessions form selects profiles for the current harness. Advanced JSON edits their definitions. Revision checks, backups and atomic saves remain in force; new settings apply on the next engine start. There is no additional general intelligence switch.

### 3.2 Loop responsibilities remain explicit

| Loop or operation | Useful native capabilities | Authoritative output |
| --- | --- | --- |
| Intake / Garden | Parallel evidence collection, Web, PTC and native agents | Independent advisory result and native receipt |
| Plan | Team task board, Swarm analysis and code investigation | Root-produced Spec/packets, validated by Go |
| Execute | Agents, Workflows, Tower candidates and LCU operations | Actual candidate subject to verification, Review and Git delivery |
| Review | Its own isolated history and native children | Existing structured verdict contract |
| Judge / second opinion | Parallel evidence checks | Separate role/scope and candidate identity |
| Native Review / interactive Team | Harness-specific independent entry points | Native job receipt and recovery identity, not a Flywheel verdict |

The configured model table, subscription channels and explicit fallback policy remain authoritative. Selecting ChatGPT inside DSH does not silently change the harness to Codex, and requesting Kimi Swarm does not turn a DSH provider into Kimi Server.

## 4. Kimi Agent, Swarm and Tower

### 4.1 An owned local server

Without a native profile, Kimi retains the existing print CLI path. With one, Go starts the packaged native worker, which starts `kimi web --no-open --host 127.0.0.1 --port 0`. The worker uses and terminates only the server whose PID, registry entry, server ID and token belong to this invocation. Tokens are not emitted in receipts or logs.

The worker validates actual OpenAPI/AsyncAPI schemas, establishes the Session and WebSocket subscriptions, then checks model, effort, permission, mode, agent profile and local cwd before submitting the prompt. Kimi owns its model/tool loop and native scheduling.

This Server seam requires explicit `danger-full-access`; its checked public API does not establish an OS sandbox. A requested `agents.maxConcurrent` is rejected because the API cannot enforce that whole-tree hard cap. Agent/Swarm/Tower remain usable without claiming that unsupported guarantee.

### 4.2 Observation, continuation and usage

Native turn ordinals begin at zero, so a fresh baseline is -1. Prompt IDs, turn IDs, ordinals, task IDs and child-agent references delimit the invocation. A connection reset or replacement snapshot is not treated as continuous observation. Root output, subsequent continuations, running tasks/agents and pending interactions jointly determine quiescence.

Native Kimi does not import the legacy CLI PlanSession; it uses the role ledger. Scoped Agent/Swarm history binds task, role, scope, channel, physical paths, model, effort, permission and native options. Run/candidate IDs remain per-call evidence and do not invalidate every continuation. Tower always creates a fresh candidate and does not accept an old Session ID.

Only complete exclusive root/child step usage produces a family total. Old child history during resume, compaction, missing child identities or usage, and Tower's unavailable complete tree accounting remain unknown. Placeholder `session.usage` zeros are not evidence. An interrupted native Server call without a durable terminal stays unknown; resending the prompt is not recovery.

### 4.3 Independent Git candidate and transfer

Tower runs in a copied repository with its own `.git`, retaining HEAD, dirty and untracked files, binary changes, symlinks, mode changes and deletions. Admission resolves the execution directory to its physical path while retaining the caller's declared alias. Both exact directory prefixes in the original prompt are mapped with path boundaries; inserted candidate paths are never scanned again. Receipts record the declared, physical and candidate directories plus original and submitted prompt digests. URLs (including queries), similar path names and external contract paths are preserved.

This is an independent Git candidate directory, not an OS filesystem sandbox. A full-access process can still access other paths. Transfer therefore rechecks source HEAD, bytes/modes/links and the external task's Spec/packet contracts. Only named volatile runtime state and native evidence directories are excluded. Source or contract changes cause a conflict with the candidate retained.

Only Execute transfers a completed, fully cleaned-up candidate, using `git apply --check --binary` before the trusted patch application. Other roles retain candidate artifacts without applying them. Transfer does not commit or bypass verification, independent Review or Git delivery.

## 5. Codex and Claude entry points

### 5.1 Codex Pro

`dsh-subagent-codex-pro@0.10.0` retains machine protocol 1 and lifecycle v1 and adds native version 1. Native agents options map to public app-server controls, followed by actual `config/read` verification. Explicit child defaultModel/defaultEffort controls do not change the root's account or model.

Child progress belongs to its current turn. A late old terminal cannot settle new work. Usage consists of owned per-thread exclusive deltas, excluding root; absent historical baselines remain unknown. Success requires the root result, empty pending IDs, settled children and native/guardian cleanup.

Native `review/start` does not consume the complete Flywheel role prompt and verdict contract. Consequently role `native-profiles` reject a `review` field. The public adapter exposes native Review, and Flywheel supplies an independent command for it.

### 5.2 Claude Code Pro

`dsh-subagent-claude-code-pro@0.5.0` supports Agent definitions, Workflow and Advisor, recording invocation controls, actual init tools and native task observations. Init verifies available tools, not every CLI scheduler setting.

Task-run IDs are opaque. Lexical ordering cannot establish freshness, and a terminal without the current run identity cannot finish a new task. Queued Agent/Task tool requests do not count as active workers: the observed concurrency guard counts started local_agent task runs. Limits are reported as `resourceBounds.hard:false`; cancellation on an observed overrun is not a pre-dispatch whole-tree semaphore.

Root result usage, child task snapshots and Advisor work can overlap or be incomplete. Raw facts are retained. Child snapshots are not added as exclusive deltas, and an unprovable family total remains unknown.

### 5.3 Independent job API and CLI

```sh
flywheel native codex-review --request /absolute/codex-review.json
flywheel native claude-review --request /absolute/claude-review.json
flywheel native claude-team --request /absolute/claude-team.json
flywheel native claude-recover --request /absolute/recovery.json
flywheel native claude-control --request /absolute/control.json
```

Inputs use each pinned public package API plus an absolute private `recoveryDirectory`. The CLI accepts a regular JSON file up to 1 MiB, writes one native receipt to stdout and diagnostics to stderr, and waits for the public API's cleanup on cancellation.

Codex Review uses a fresh ephemeral thread and an explicit target. Claude remote Review requires `consent:true` and runs real `claude ultrareview --json --no-post`; it does not implicitly authorize a cloud job or publish a comment. Interactive Team uses an independently owned tmux server/pane with capture/send/resize/cancel/attach controls. Terminal text is not a formal verdict.

A local timeout or exit does not settle a remote Review job. Use the original requestId, runId and recoveryDirectory to read the original local journal and retrieve its durable receipt. This neither queries cloud job status nor resubmits Review. Unknown pane ownership likewise cannot be converted into confirmed cleanup.

Claude `recoverNativeJob` only reads the local durable journal. It does not start a process or query the cloud. Receipts explicitly retain `jobId:null`, `querySupported:false` and `cancelSupported:false`. A local recovery status of terminal does not establish remote completion: unknown remote state or unconfirmed native cleanup still forbids replay or a claim of cloud cancellation.

## 6. LCU, desktop ownership and media

### 6.1 Public managed mounting

`dsh-plugin-lcu@0.5.0` preserves normal desktop Preset mounting/revocation and exposes `attachManagedLcu()` for owned SDK Agents. The live registry and real parent lineage establish the caller; a supplied Session ID is insufficient.

Each Agent runtime has its own epoch, handles, turn lifecycle and MCP connection. The PTC tool is named `lcu_cua`, while programs still use `cua`. This avoids the newer executor's special coercion of the exact `cua` tool name, which would discard structured object handles. Existing programmatic Computer/Browser APIs and multiple images remain available.

App/command, Chrome/audio and allowedApps/allowedOrigins settings reach the actual runtime. macOS TCC, the browser extension, target permissions and real runtime questions remain owned by those components. The implementation does not fabricate approval answers or claim a complete new desktop question relay.

### 6.2 One lease per physical desktop

Before managed LCU initialization, Flywheel acquires an atomic `local-desktop` lease in a private resource directory. The lease records invocation, owner, ID and epoch. Each operation rechecks caller and lease; root and children share ownership and execute GUI actions sequentially.

A dead PID or PID reuse does not prove release, and unknown owners are not automatically displaced. Terminal lease records retain a digest and size of complete cleanup evidence; the full receipt remains available separately. This avoids oversized image evidence making the lock unreadable.

### 6.3 Cancellation and physical exit

Abort cancels MCP requests, waits for turn cleanup and then confirms native runtime exit. Normal native DSH shutdown has 15 seconds, with at least 20 seconds at the Go boundary. LCU receives 135 and 145 seconds respectively to cover its public turn-stop and native-close bounds.

Root turn-stopping awaits the policy journal. Final `native-cleanup.json` binds request ID, nonce and complete/events/bytes. The journal is limited to 4096 events and 2 MiB, with contiguous `journalSequence`. The worker checks identity, byte count, complete lines and regular NOFOLLOW reads. Storage errors, attribution errors, unfinished turns and uncertain physical exit prevent success and GUI release.

### 6.4 Durable media evidence

DSH AttachmentStore may normalize image encoding. The adapter reads and archives the actual stored bytes and distinguishes captured and archived digests. It never attributes the capture hash to different normalized bytes.

Media and a manifest persist under `<taskDir>/reports/native/<invocationId>/`, with regular-file, NOFOLLOW, size, hash and durable-write checks. Temporary worker deletion does not remove the evidence. `imageDeliveredToModel:'unverified'` distinguishes capture/storage/archive from confirmed model delivery. ChatGPT projection tests establish request construction; real model visual understanding requires a live test.

## 7. Receipts, budgets and recovery

Native receipts retain `version`, original `requested`, `effective`, `observed`, `children`, `usage`, `completion` and `cleanup`. Provider-specific identities and evidence remain visible instead of being compressed into an unsupported universal guarantee. Defaults belong in effective controls, without rewriting the caller's original request.

A native-event is observation, not a durable lifecycle ACK. Existing public lifecycle checkpoints alone authorize the corresponding execution boundary. **Completion is frozen before cleanup**: cancelling pending children during shutdown cannot turn unfinished work into success. Go requires consistent root, child, requested/effective, completion, cleanup and outer-process facts.

### 7.1 Concurrency and accounting

`scheduler.max-parallel` and `scheduler.providers` limit Flywheel root invocations. DSH's exact-live-root activation pool (shared by Team/Workflow and local/external activations), Codex controls, Claude's observed guard and Kimi's native scheduler keep their own meanings. They are not a uniform hard global token or whole-tree concurrency cap. The local DSH admission layer also enforces real lineage depth for Workflow-created Agents; it cannot control unobservable descendants inside an external provider.

| Harness | Usable accounting | Unknown cases |
| --- | --- | --- |
| DSH | Root plus complete exclusive committed local child Session events | External children without usage, missing events/journal |
| Codex | Root plus complete owned child per-call deltas | Missing child baselines or events |
| Claude | Raw root result and nonadditive child/Advisor observations | Unattributable family work |
| Kimi | Complete fresh root/child step usage | Compaction, old children, Tower or missing steps |

Unknown is not zero. The existing hard budget blocks additional calls when usage remains unknown; switching to fresh history or another adapter cannot bypass it. Soft/off retain their configured semantics. This implementation does not promise strict advance token reservations.

### 7.2 History and recovery

Go creates managed run/invocation/role/scope/candidate/channel/taskDir identity after durable invocation intent. Full native options are retained in Session configuration and frozen policy identity. Prewarm binds native intent but creates no Agent, GUI lease or prompt until formal admission.

Pro read-only recovery includes the original native options and validates the recovered receipt against the original request. Current configuration cannot reinterpret an old call. Kimi Server does not claim Pro's durable ACK/recovery capability. LCU handles do not survive runtime epochs; objects must be reacquired from the real runtime.

## 8. Repository delivery and packaging

| Repository / PR | Deliverable |
| --- | --- |
| [flywheel #44](https://github.com/ckanner/flywheel/pull/44) | Schema/configuration, drivers and history, DSH/Kimi adapters, job CLI, workbench, leases/media, single-package build and documentation |
| [dsh-preset-generator #1](https://github.com/ckanner/dsh-preset-generator/pull/1) | 0.4.0 shared composition, static identity and behavioral admission contract |
| [dsh-plugin-lcu #1](https://github.com/ckanner/dsh-plugin-lcu/pull/1) | 0.5.0 managed mounting, epochs/handles, physical cleanup, media and desktop lifecycle fixes |
| [dsh-subagent-codex-pro #6](https://github.com/ckanner/dsh-subagent-codex-pro/pull/6) | 0.10.0 controls/events/accounting/completion and independent native review |
| [dsh-subagent-claude-code-pro #2](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/2) | 0.5.0 agents/workflows/advisor, remote Review, interactive Team and recovery |
| [dsh-plugin-chatgpt #8](https://github.com/ckanner/dsh-plugin-chatgpt/pull/8) | 0.1.19 attachment/image projection, abort handling and public dependency compatibility |

Flywheel remains one `@ckanner/flywheel` package with CLI, DSH host/client, four workers, embedded skills and four-platform Go binaries. Codex 0.10.0 and generator 0.4.0 are pinned and bundled from real source tarballs. Claude 0.5.0 and LCU 0.5.0 are optional peers. ChatGPT remains a DSH provider chosen by the actual composition/account.

Five vendor records identify fetchable source commits/trees, tarball hashes, integrity, license and unverified registry status. CI reproduces the fixed Claude source candidate and verifies its bytes. Normal npm resolution is required; force/legacy-peer workarounds are not used. PR delivery does not publish packages or modify the user's installed configuration.

## 9. Verification and acceptance

Required gates include Go `make quality-gate` with 100% statement coverage for every measured function, full Node and legacy-host behavior, four real Go→Node worker boundaries, actual public Pro APIs, actual PTC/Team/LCU seams, Kimi local HTTP/WebSocket and real Git candidates, npm launcher tests, 36 real-engine session scenarios, and one packed installation with all four platform binaries.

Assertions cover rejection before dispatch, unchanged account/model routing, foreign-child attribution, late terminals, pending children at root completion, cancellation versus cleanup, failed journals and unreleased GUI leases, image order/hash, retained Tower conflicts, unknown usage and read-only recovery. Deterministic model/runtime fixtures are labeled as such.

Quality, repair count, elapsed time, usage and GUI success should later be compared on the same tasks and candidates. Multiple agents alone do not establish a speed or cost improvement. Mac TCC, Chrome extension, live account capabilities, model vision and remote Claude entitlement remain separate user-machine acceptance. Actual pass/fail/skipped/not-run results are in the [validation record](../validation/2026-11-14-native-harness-implementation.md).

## 10. Primary sources

Current behavior is owned by the [architecture](../design.md), [schema](https://github.com/ckanner/flywheel/blob/main/templates/native-profiles.schema.json), [worker protocol](../../plugins/dsh/PROTOCOL.md), [Kimi protocol](../../plugins/dsh/KIMI_NATIVE_PROTOCOL.en.md) and code/tests in this PR. The earlier proposal retains dated research; vendor records pin companion implementations.

- DSH: [release SDK](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/sdk/server/src/server.ts), [official PTC overlay](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/apps/cli/config/examples/ptc.yml), [master PTC](https://github.com/deepseek-ai/deepseek-harness/blob/39163dd30f703a79079e2d2446545cff3c453738/packages/core/tool-executor-ptc/src/ptc.ts), [Team](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/team/agent-team/README.md).
- Kimi: [Server API](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api.md), [conversation API](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-conversation.md), [agents](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/customization/agents.md).
- Codex: the [pinned public machine contract](https://github.com/ckanner/dsh-subagent-codex-pro/blob/c0045b06e6762b8dfc56f50b14cce7a53af2725b/docs/machine.md) and its fixed official app-server references.
- Claude: [Subagents](https://code.claude.com/docs/en/sub-agents), [Teams](https://code.claude.com/docs/en/agent-teams), [Workflows](https://code.claude.com/docs/en/workflows), [Ultrareview](https://code.claude.com/docs/en/ultrareview), [Advisor](https://code.claude.com/docs/en/advisor). The public adapter also preserves failures when the installed CLI does not expose the requested capability.
