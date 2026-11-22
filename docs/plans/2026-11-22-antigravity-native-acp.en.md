# Official Antigravity ACP interactive integration proposal

English · [简体中文](2026-11-22-antigravity-native-acp.md) · [Plans index](README.md) · [Current Antigravity guide](../en/antigravity.md)

- Date: 2026-11-22; revised for the requested official standalone ACP and personal interactive use.
- Status: **proposed, documentation only**. This document ships no ACP runtime, configuration field, login or image capability and changes no existing default.
- Original baseline: Flywheel `291c0a02f9042d522d3e0f3c62977f63149465cd` / 0.38.0; `@ckanner/dsh-subagent-antigravity` 0.2.1, source `a20eb63bbfa2849df63043595475957bf4a4f04d`.
- Separate repair: the evidence-only receipt mismatch in §2 has its own fix and acceptance. Preserving that baseline here does not claim this proposal completes the repair.

## 1. Decision, use case and stages

**The initial implementation uses Google's official, unmodified `antigravity-acp` from ACP Registry as a directly operated external-agent session in the DSH/Flywheel UI.** The user connects, signs in through the agent's own Google flow, sends development tasks, handles permissions, reviews results and interrupts work. The official agent owns its model calls, tools and internal agent loop; the desktop host provides the ACP client and interface.

Keep the existing official headless stream-json runtime, defaults and tasks unchanged. The initial ACP session is not a generic model provider beneath a DSH parent agent and is not automatically added to JEV, Plan/Execute/Review or other unattended candidate sets. An ACP session appearing in the UI does not mean a Flywheel task executed or passed independent Review.

**Correction to the original proposal.** The earlier native-only choice excluded Google's standalone `.par` alongside third-party wrappers and did not match the requested direction. Registry metadata and Google's Android Studio guide establish the standalone distribution as an official editor integration [S17, S18]. This revision makes it primary. Native CLI `agy acp` remains an official option requiring independent validation and explicit selection; it cannot silently replace the standalone distribution. Keep the existing filenames and links for compatibility.

| Stage | Proposed scope and boundary |
| --- | --- |
| Initial: personal interactive external agent | Direct official standalone ACP on macOS/Linux: personal sign-in, text turns, model/effort selection, permissions, stop and exact history loading. Images and native collaboration receive separate capability acceptance. Registry Windows artifacts do not establish this project's Windows acceptance. |
| Later: another official runtime | Evaluate `agy acp` independently, with distinct source/version/capability records; do not assume interchangeable flags, authentication or history. |
| Later: automated Flywheel binding | Establish a separate implementation scope with technical proof and support for the selected use/authentication context before machine APIs, role admission or automatic scheduling. An interactive session cannot upgrade itself into this binding. |

ACP means **Agent Client Protocol**, connecting a client to a complete agent; it is not a model name. Goals are reliable interaction, observable capabilities and verified original-image input. Speed and quality gains require measurement. This proposal does not change Auto/Quick/Full, plan approval, JEV's single switch, model defaults, current subscription rules or independent acceptance, and does not pronounce on every existing Flywheel use case.

## 2. Existing system and separate repair

The implemented path remains:

`Flywheel Go owner → Antigravity worker → public plugin machine API → owned headless agy process`.

The plugin owns process groups and durable recovery journals; Go validates terminal state, identity, configuration and cleanup before consumption. The DSH UI does not write task state. Sources are [the Go driver][C1], [worker bridge][C2], [plugin runner][C3] and [machine types][C4]. New interactive sessions have separate session records; they neither write `state.json` nor fabricate Go invocation receipts.

| Baseline fact | Constraint on this proposal |
| --- | --- |
| Headless closes stdin after each prompt. | Preserve that behavior; history reuse, process residency and protocol are separate dimensions. |
| JEV/investigation require evidence-only; ordinary execution requires supported explicit authority. | Initial ACP joins none of these roles; a later machine binding proves each equivalent contract. |
| Model, effort, physical cwd, subscription controls and native mode are frozen identity. | A later machine identity also needs the precise ACP distribution and capabilities, not just `acp`. |
| Packet images currently require a verified Claude binding. | Interactive ACP image acceptance does not automatically change engine packet admission. |
| Swarm already requires Full, reviewed work and input checks. | Preserve them for existing and future machine bindings. An interactive session is not a reviewed packet. |
| Unknown invocations are not replayed. | An ACP Session ID or successful reconnect does not prove a lost prompt never executed. |

**Original baseline defect.** Adapter 0.2.1 accepts a nonempty `init.tools` catalog after verifying the managed agent and runtime restrictions, while Go still requires an empty catalog. Successful receipts can be rejected as unknown, keeping JEV's frozen baseline and blocking investigation. The separate repair must exercise the same receipt through adapter → worker → Go owner → decision/analysis consumption and recovery, not merely remove the catalog restriction.

Check that repair's own source and acceptance record before implementation. ACP does not fix it, and the initial interactive session does not require Go lifecycle ACK machinery; that belongs to a later machine binding.

## 3. Evidence and alternatives

### 3.1 Two official distributions and supported use

| Distribution | Source facts checked on 2026-11-22 | Treatment |
| --- | --- | --- |
| Registry `antigravity-acp` | Manifest identifies Google LLC; observed version 1.5.2; Google download URLs; Mac/Linux `agy_acp_server.par`, Windows `.exe`, with platform arguments and SHA-256 [S17]. | **Initial primary path.** Pin reviewed manifest, artifact bytes, platform command and arguments. 1.5.2 is an observation, not an accepted default. |
| CLI `agy acp` | Publicly documented by Google; CLI changelog records ACP in 1.5.0 and expanded model access in 1.5.1 [S1, S3, S4]. | Independently validated later option. CLI flags/version findings do not automatically apply to the Registry server. |
| Third-party wrappers/internal hub RPC | Different from either official distribution. | Outside this primary path; never automatic fallbacks. |

Google documents Registry installation for Android Studio and ACP for other editors [S4, S18], supporting the product direction of an official agent in an interactive development client. Terms §4 covers supervised and autonomous agents; §6 and the FAQ restrict third-party use of personal Antigravity sign-in. The FAQ also supports direct scripts/CI. This does not justify blanket bans on unattended operation, MCP, custom agents or native Swarm [S20, S21].

On 2026-10-08, Google-flair moderator chunduriv distinguished direct scripts/CI and ACP editors from third-party coding agents wrapping consumer sign-in, listing Registry editors as supported and disclaiming formal legal interpretation [S22]. This proposal adopts a directly operated official-agent session; ACP or another project's implementation cannot certify every outer orchestration. A future automated binding needs its own use/authentication basis, without judging all existing Flywheel workflows here.

Parts of the editor model-restriction text conflict with the later **CLI** changelog [S3, S4]. Use honest client identity and actual access/configuration returned by the chosen distribution/version. Do not infer that CLI 1.5.1 changed standalone-server policy. ACP v1 does not guarantee optional capabilities [S5–S8]; v2 preview is not implicitly part of the first client.

### 3.2 T3 and Pi: useful examples and limits

| Pinned source | Actual implementation | Consequence |
| --- | --- | --- |
| T3 main `f10e1eb3825a773cf702569d2258edfdbc8b77dc` | Its built-in provider downloads the official CLI, runs `acp`, and uses the same program's `acp --login` for personal sign-in; a generic Registry provider also exists [S23, S24]. | Current built-in behavior differs from historical `.par` comparisons. Borrow runtime-owned login without treating the CLI as the Registry artifact. |
| Compatibility/quota code at the same T3 commit | Older runtimes receive `zed` client identity; quota probes read tokens and call internal endpoints; a synthetic SSH environment affects credential storage [S23, S25]. | Do not copy these choices. T3 as a whole is not token-opaque or proof of blanket compliance. |
| Community `zacbemis/pi-antigravity-acp-provider`, `6d6135d888cb50e2cb1c18bc2520a94ac7d26392` | Uses the official standalone agent and its authentication, while registering a Pi provider and bridging Pi tools/outer loop through MCP [S26]. | Artifact/login choices are relevant; the nested provider/tool loop is outside our initial scope. This extension is not Pi core. |
| Other Pi Antigravity extensions | Direct OAuth implementations also exist; the Pi name does not establish ACP use [S27]. | Identify the repository/commit rather than saying all Pi integrations work alike. |

### 3.3 What the performance reports establish


| Evidence checked on 2026-11-22 | Observation | Limitation and design consequence |
| --- | --- | --- |
| T3 discussion #28315, 2026-10-31 [S9] | One Linux comparison reports cold initialization around 3.0–3.5 s for the old `.par` server versus 0.26–0.50 s for native ACP. | Both sides are ACP. This distinguishes implementations, not an ACP tax versus headless. |
| 2BAB's 24-run comparison, 2026-11-11, updated 11-17 [S10] | Old ACP versus hub medians: text 11.79/9.21 s; file workflow 29.65/32.23 s. | Different versions and default context/tool policies; same-build native ACP latency was not rerun. No universal speed ranking follows. |
| Theo's X post, 2026-11-04 [S11] | Reports problematic bindings and roughly 5% fewer false reports after a CLI rerun. | No complete controlled evaluation; 5% concerns false reports, not latency or a protocol-wide quality coefficient. The X body was available through a public mirror, not a successful direct X read. |
| T3 issue #28172 [S12] | Old-server reasoning/stream completion problem; maintainer could not reproduce with native agy 1.5.12 and closed it on 11-20. | Historical symptoms must not be presented as a current universal native-ACP defect. Displayed reasoning is not an independent quality measure. |
| Upstream cache issue #1533 [S13] | Users report cache drops across interactive, headless and ACP use. | Backend explanations are not confirmed root causes; keep cache telemetry and do not promise immunity on either path. |
| Upstream timeout issue #1402 [S14] | Includes a native-ACP 1.5.5 timeout/recovery report on Windows 11 with Vertex AI/ADC, alongside TUI/headless reports. | That platform/auth path is outside the initial macOS/Linux personal-login scope. It motivates lifecycle tests, not a claim that our exact path reproduced the defect. |
| DSH user feedback, 2026-11-13 [S15] | Reports repeated work and lost context in an ACP integration. | Missing exact client/runtime/configuration evidence; not identified as this plugin. Test compaction/history rather than attributing it to the protocol. |

This research established no controlled quality benchmark across the current standalone ACP server, native CLI ACP and this headless adapter with comparable account, model/effort, context and tools. Different distributions cannot be described as identical executable bytes. Neither “ACP necessarily degrades quality” nor “the transports are proven equivalent” is justified.

### 3.4 Alternatives

| Choice | Decision |
| --- | --- |
| Preserve existing headless | Continue implemented paths without changing runtimes/defaults through this proposal. |
| Official standalone ACP, directly operated by the user | **Initial recommendation.** Pin the original Registry artifact and accept login, permissions, lifecycle and UI. |
| Official CLI `agy acp` | Independently validated explicit option later; no silent substitution. |
| Immediately add all automated Flywheel candidates | Outside the initial release; technical gates and supported use/authentication must both be settled separately. |
| Third-party wrapper/internal hub RPC | Not adopted here; do not group them with the official standalone artifact. |

## 4. Ownership and proposed interfaces

The initial path is `user → DSH/Flywheel external-agent session UI → standalone plugin ACP client → official antigravity-acp`. The plugin owns the connection, process group, event correlation, session record and cancellation. Google's agent owns models, tools and internal orchestration. The UI sends user actions and displays facts, without a parent agent deciding or delegating the same work again.

**Each interactive session owns one process/connection, with at most one pending prompt.** Human turns can reuse it; this is not a shared daemon across users/tasks. Root prompt completion, settlement of known child work and process exit are separate facts. Closing a session performs bounded shutdown/cleanup; each turn need not have an exit code, and a root response cannot certify the whole tree.

Honor physical workspace ownership and leases before allowing writes. A direct UI session cannot overlap an active Flywheel writer or bypass its human-takeover gate. Use an explicitly available workspace or the existing takeover process. Any later transfer of edits into a task uses normal reviewed-artifact and verification handoff; interactive completion never changes task state.

Advertise only client capabilities whose handlers are implemented, authorized and tested. The initial client does not automatically expose DSH tools as MCP or inject servers merely because `mcpCapabilities` exists. The runtime's own MCP, skills, custom agents and internal collaboration follow supported capabilities and user authority; this is a client implementation scope, not a general ban on official features. Unsupported client-hosted fs/terminal/elicitation requests fail visibly; permissions require a real handler.

Resolve the same official executable at preflight and connection. Record distribution, manifest/binary hash, version, platform arguments, adapter package identity, protocol and capability digest. Use real `clientInfo`; never impersonate an editor. Binary/configuration changes invalidate prior capability evidence. Identities contain no credentials.

### Initial interface example

This is a **future client-settings design, not accepted configuration today**. It is neither a machine profile nor a complete installation configuration:

```json
{
  "externalAgentSessions": {
    "antigravity": {
      "mode": "personal-interactive",
      "runtime": {
        "source": "official-acp-registry",
        "agentId": "antigravity-acp",
        "release": "<explicitly-validated-release>"
      },
      "authentication": "oauth-personal",
      "modelSelection": "from-session-config"
    }
  }
}
```

| Surface | Initial contract |
| --- | --- |
| External-agent session | Explicit connect/login, workspace selection, runtime-provided model/effort, tasks, permissions, review and stop. |
| Public interactive plugin API | Separately versioned connect/authenticate/new-or-load/prompt/cancel/close and observations; no second native ACP parser in the UI. |
| Session record | Owned session/turn/connection identities, input manifest, dispatch/terminal/activity/cleanup facts; no task-state writes or machine-success receipts. |
| Capabilities/configuration | Distinct advertised/local-verified/live-verified/unknown and requested/accepted/observed facts; an echo is not independent backend observation. |
| Existing machine API and role/model bindings | No initial ACP candidates. Preserve headless protocols and omitted-value behavior. |

A later machine binding needs its own schema, transport/distribution fields, machine/worker/lifecycle/ledger versions and migration; old readers must reject unknown ACP facts. The interactive API can reuse verified lower-level modules without acquiring machine eligibility. Existing Go success requires its terminal, exitCode 0 and cleanup contract; persistent interactive events must never masquerade as such a receipt.

## 5. Connection, official sign-in and model selection

The initial flow begins with explicit user actions:

1. Choose workspace and official Registry release; check ownership/lease, integrity, platform command/arguments and local configuration. A valid artifact proves neither model entitlement nor quota.
2. Start the owned process, send `initialize` with real `clientInfo` and implemented capabilities, and inspect `authMethods` and capabilities. Report missing required support before dispatch.
3. Present Connect / Sign in with Google. The agent may reuse official cached sign-in. If login is needed and the user initiates it, use the runtime-supported `authenticate` method with `oauth-personal`. Browser/terminal assistance is enabled only when supported and tested for this distribution/client; never append CLI `--login` to `.par` or assume `terminal-login` exists.
4. The Google agent creates the auth request, exchanges and stores/refreshes credentials. The UI carries only the official interaction. Credentials never enter Flywheel configuration, logs, journals or a model-provider adapter. Cancellation/failure returns to an explained state, without switching accounts, API keys or billing paths.
5. Revalidate connection/session after login. Create a session or load the exact owned, matching session the user selected; never choose latest or resend a lost prompt.
6. Read the session's model/effort/permission options and confirm acceptance of the user's selection [S7]. Do not hard-code `thought_level`/`thinking_level`; retain fixed Orchestrator effort instead of substituting high. Session discovery/handshake success does not make every model available.
7. When the user sends a task, freeze prompt/attachment identity and persist dispatch-possible before writing `session/prompt`. The interactive client's record owns this boundary; a future machine path additionally needs Go durable lifecycle ACK.

Google documents shared authentication across local surfaces [S19]. **Do not promise independent login caches per provider instance.** Separate settings directories require official support and validation and do not prove account isolation. Do not simulate SSH, copy tokens, read keyrings, register our own OAuth client, proxy private quota endpoints or substitute client identity.

Use verified public interfaces for diagnostics. A probe creating history or auxiliary work must declare its effects and use owned isolated directories without business prompts. Adopt model/effort/authority changes only at a confirmed safe boundary; retain the active turn's original request and later observations rather than switching silently or rewriting earlier facts.

## 6. Lifecycle, cancellation and recovery

### 6.1 Interactive sessions

Record client session/turn ID, adapter run ID, native Session ID, connection generation and JSON-RPC request ID. A client RPC ID is not a provider idempotency key. Permit at most one unresolved prompt; queue further user input. The next prompt requires a verified settled boundary for relevant native work. Known active children or unknown prior work keep it blocked; steering is a separately verified later capability [S16].

| Event | Required behavior |
| --- | --- |
| Failure before prompt bytes can leave | Mark proven not-dispatched only when durable records establish that this business prompt was never written and relevant pending requests have settled. A usable session process need not exit; this cannot clear earlier unknown requests. |
| Write attempted, disconnect, timeout, malformed/oversized frame | Preserve possible/accepted dispatch and unknown outcome; no automatic resend, runtime substitution or transport change. |
| Root prompt response | Show correlated stop reason/output and continue observing known post-prompt/child work. Root completion, known-work settlement and process closure are separate; no automatic Flywheel completed verdict. |
| User stops | Send `session/cancel`, reply `cancelled` to pending permissions, and drain a still-pending root request to its response. Sending the notification does not prove work stopped; use bounded TERM/KILL when needed and record cleanup/unknown remote work. |
| Children remain after root response | Continue owned events/permissions. There is no pending root response to prove cancellation now; use tested quiescence/shutdown observations. Killing a process does not prove child success. |
| Session close/host exit | Bounded shutdown of the owned process group, preserving history and unresolved records. Closing UI cannot discard dispatch facts. |
| Reconnect/history load | Exact owned session only; distinguish replay, current activity and new usage. Loading history cannot prove the lost prompt's outcome. Preserve unknown requests; never resend automatically. |
| Partial output then refusal/cancel/resource cutoff/error | Retain partial evidence and native reason, without promoting it to success or safe retry. |

A turn may have a terminal/activity state while its process lives; process exit/cleanup belongs to session closure. Protocol-permitted post-root activity is not itself a violation [S8], but cannot be ignored when contradicting previously claimed final quiescence. Missing child observation cannot prove a quiet whole tree. ACP cancellation, local cleanup and cancellation of unknown remote work remain distinct; they do not set existing `providerWork.cancelSupported`/`querySupported` to true.

Associate sessions with physical workspace, runtime source/version, account/configuration references, authority, model and input identity. Shared authentication is not shared conversation. Cross-runtime/transport history migration needs explicit validation and cannot retire unknown requests. Compaction/replay cannot override confirmed user requirements; test constraint retention, and preserve unknown accounting/compaction metadata.

### 6.2 Additional contracts for a future machine binding

This is not an initial interactive-UI capability. A later automated Flywheel proposal still freezes invocation identity, persists dispatch-possible and obtains Go lifecycle ACK before the prompt, binding Flywheel request ID, role, target, task/packet and budget. Recovery consumes a valid durable terminal once; missing outcome/cleanup blocks cold starts and budget resets.

Choose and validate either a process per invocation or reuse with sufficient quiescence proof; initial interactive residency is no machine terminal guarantee. Receipts distinguish outcome, terminal, cleanup, model/authority, collaboration, images, usage and provider-work uncertainty. Independent reviewers do not inherit executor history; full/verified-incremental policy remains. Future steering needs separately versioned authorization/input revisions, not a bypass around frozen input.

## 7. Authority, intended use and evidence-only

### Initial personal sign-in and permissions

The user directly operates the official agent and completes its browser/terminal login. Bind permission requests and responses to the right session, turn and request. A `selected` outcome uses an offered `optionId`; cancellation returns `cancelled`. Explain scope/persistence instead of granting allow-always or widening directories for the user. Without a verified approval bridge, do not dispatch tasks needing one. Plan/accept-edits labels do not prove OS isolation.

Honor the chosen personal auth and applicable subscription controls; reject unauthorized API/ADC/GCP/custom-provider/credits fallback. Verifying a control is a property of this runtime, not an assumption copied from headless configuration. Auth failure requires explicit user recovery, not account rotation. Entitlement, verified local controls, quota and actual billing remain distinct; absent public quota evidence stays unknown.

Choosing personal interactive scope does not prohibit all automation. Official native tools, MCP, custom agents and Orchestrator/Swarm have their supported contexts; an outer third-party agent driving consumer sign-in needs separate support review [S20–S22]. API/enterprise auth is also a separate use and billing choice, never an automatic replacement for personal login.

### Future machine authority and evidence-only

Automated ACP role admission requires both supported use/authentication and technical evidence. New approvals beyond granted authority are reported, not automatically accepted or left waiting indefinitely. Reimplement equivalent, verifiable environment filtering, credits controls and configuration checks for the selected runtime; matching setting names are insufficient.

JEV remains a fresh, tool-free decision under the single `jev.enabled` policy. ACP cannot give it delegation or Orchestrator/Swarm. Evidence-only needs proof that the active session uses the managed agent, immutable configuration and run ID/digest, with tools/MCP/skills/plugins/commands disabled and no actual tool/child steps. A nonempty catalog is not an executed tool.

Whether ACP supplies that active-agent proof remains unresolved; `--agent` or checking a file is insufficient. Until accepted, ACP is ineligible for JEV/investigation; the separately repaired, verified headless path remains available. Never remove `evidenceOnly` to pass admission. This is a role-specific restriction, not a ban on normal interactive sessions or official custom-agent features.

## 8. Images and native collaboration

### Original-image input

Verify image prompt capability and the actual adapter/model path before interactive image input [S5, S6]; let the user select and preview authorized files. Preserve format, size/count, ordinary-file and authorized-root checks; freeze MIME, bytes and SHA-256 before dispatch. Send image bytes rather than treating a path, resource URI or existing chat attachment as delivery.

Record manifests/delivery evidence without base64 or credentials in progress logs. Distinguish serialization, prompt acceptance and live visual acceptance; use images not answerable from filename, text or metadata. Verified root delivery does not establish child delivery.

**Packet admission is a later, separate gate.** Change the current Claude requirement only after a shared engine/UI capability check is accepted. Otherwise preserve existing rules: fail unsupported input before dispatch, without deleting image declarations or degrading to text. Interactive attachments do not automatically enter task packets.

### Orchestrator, subagents and Swarm inside the official agent

These are native runtime capabilities, distinct from an external parent agent using Antigravity as its model/tool backend. Initially expose only models/modes/commands supported and verified for the selected standalone distribution; CLI docs or UI labels do not establish implementation. Ordinary native mode does not promise zero children.

| Capability | Interactive acceptance | Additional future Flywheel machine requirements |
| --- | --- | --- |
| Orchestrator | Exact model, fixed effort, accepted configuration and honest internal-collaboration observations. | Preserve role/model eligibility; evidence-only cannot delegate. |
| Explicit subagents | Verify real delegation and ownership; missing observation stays unknown, not an all-successful tree. | Required work meets collection/receipt contracts. |
| Swarm | Verify the officially supported initiation mechanism and user authority; slash-looking text alone is insufficient. | Execute + Full + reviewed independent collaboration/writes + input eligibility. An initial interactive session does not acquire these qualifications. |

Child-session presentation is optional; an agent may delegate without exposing children [S8]. Record requested mode, actual child IDs, root completion and child outcomes separately. Unknown models, usage, state and remote cleanup stay unknown. An observed-count guard is not a hard native concurrency limit. Pin schema/SDK; advertise `subagents` only with an implemented handler, separately gate draft variants, and accept child events only after verified ownership links them to this root.

Retain the future Swarm packet image exclusion until reviewed contracts express which workers need the original and delivery is accepted. Text-only downstream steps may use reviewed derivative specifications; deleting a real image requirement is not a migration.

## 9. UI, configuration and diagnostics

Add an explicit Antigravity external-session entry for official Registry runtime, workspace and personal login. **Do not automatically add ACP to existing execution provider/model candidates.** ACP is the connection protocol, Antigravity the agent identity, and model options come from the established session.

The UI must:

- Show disconnected/connecting/auth-required/signing-in/ready/running/waiting-permission/cancelling/unknown states, with explicit official login and cancellation.
- Explain that native sign-in may be shared across local Antigravity surfaces. Disconnect is not Google logout; perform logout only when supported and explicitly requested.
- Show distribution, runtime/protocol/adapter versions and integrity, available models/effort and advertised/tested/unknown capabilities; discovery is not entitlement.
- Separate user selection, agent-accepted configuration and last actual call. Use readable names and expandable raw IDs; do not invent effort defaults.
- Present permissions, actual tools and observed children for review/interruption; native self-assessment never becomes a Flywheel Review pass.
- Preview attachments, frozen identity and root/worker delivery distinctions; state whether any attachment entered a task packet.
- Correlate asynchronous auth/model results with exact session, runtime, target and revision. Prevent unsafe edits or implicit retry around pending/unknown work and enforce workspace ownership/takeover.

A later machine configuration can add transport/distribution, effective task scope/source, JEV explanations and engine-shared eligibility, rereading effective values after save. Keep these distinct from `native-profile`, model, effort and JEV's switch. This later design does not alter existing tasks, approvals or initial defaults.

## 10. Implementation, release and rollback

1. Check separate receipt/packaging repairs and headless acceptance, preserving original source/registry/live evidence; this PR does not rewrite historical outcomes.
2. Implement official Registry ACP and explicit login in the standalone plugin, pinning artifacts and the public interactive API; do not initially change machine protocols or automated candidates.
3. Accept text sessions, permissions, stop/recovery and official login with offline and necessary live checks, then connect DSH/Flywheel UI through that API. Record real capabilities and unknowns.
4. Publish a tested plugin only under separate release authorization, pin exact package identity, and verify the same candidate installation/documentation. Existing paths remain when this session is not enabled. Reject unsupported runtime/adapter combinations instead of substituting silently.
5. Accept images/native collaboration independently; validate CLI ACP as a separate option. Propose later machine use/authentication and technical contracts before changing schemas, bridge, Go owner, ledger, UI and migration.
6. Before closing/removing the interactive entry, handle owned connections and retain unresolved records. Disconnect does not log out or delete shared credentials. Keep old journal readers; do not replay unknown ACP turns through headless or another distribution.

Initial human turns share their session connection. Cross-session server sharing, parallel sessions in one server and mid-turn steering still require independent tests. Future automated feedback respects frozen inputs and adoption boundaries; interactive buttons cannot bypass the Go state machine.

## 11. Verification and comparative evaluation

### 11.1 Staged correctness gates

| Area | Initial acceptance | Additional future machine acceptance |
| --- | --- | --- |
| Source/protocol | Original manifest/hash, platform arguments, exact distribution; unsupported protocol/options, bounded Unicode/JSON framing and capability differences. | Identity in frozen bindings/receipts/recovery; strict old-reader rejection. |
| Auth/permissions | Actual `authMethods`, explicit `oauth-personal`, official success/cancel/expiry/failure, no token extraction, shared-cache disclosure; wrong-session/late/denied approval. | Supported auth/use context, bounded handling of insufficient authority, verified subscription controls. |
| Lifecycle | Turn versus process exit; cancel during tools/permissions/children, disconnect, timeout, host death, SIGKILL, malformed/duplicate/late terminals, owned-group cleanup and no next turn before settlement. | Go ACK crash windows, one-time valid consumption, unknown blocked, preserved budgets and required-work proof. |
| History/accounting | Exact session load, no automatic replay, compaction retaining requirements, cumulative counters not summed per turn, unknown missing/negative/inconsistent deltas. | Role isolation, full/incremental input, independent reviewer and task/packet/target identity. |
| Images/collaboration | Frozen bytes, changed-source rejection, real visual task, child ownership, Orchestrator effort, actual Swarm mechanism and unknowns. | Shared image admission, Full/reviewed work/write scopes and required worker image delivery. |
| Evidence-only | No initial claim to these roles. | Active managed agent, no tools/delegation, immutable identity, actual steps versus catalog. |
| Distribution/UI | Clean public import, version rejection, same Linux/macOS candidate, session/permission states, asynchronous auth/model correlation, workspace lease/takeover and docs. | Machine/ledger migration, effective-config sources and consistent task/role eligibility. |

Implementation follows applicable repository gates: 100% measured Go statement coverage per function for Go changes, plugin/bridge behavior checks and actual candidate installation for release changes. This documentation PR runs static checks only. Offline fixtures prove neither account access, login, visual quality nor performance; live records must name runtime, platform, account context and tested scope.

### 11.2 Comparison design and attribution

Freeze tasks, scoring and acceptance margins first. Separate **the actual standalone Registry ACP versus current headless product paths** from a later **same-CLI-build `agy acp` versus headless protocol experiment**. The former necessarily includes runtime differences; do not call it same-binary or attribute all outcomes to ACP. Record both artifact identities, account/auth, model/effort, settings/rules/plugins, authority, workspace, input and history/compaction policy.

Randomize/alternate order; use at least ten paired repetitions per case for an exploratory report and retain every failure. Cold trials use fresh matched sessions; warm trials use equivalent histories. Label uncontrolled backend caches honestly. Run only in authorized disposable workspaces within supported use/auth boundaries. Initial evaluation is user-triggered and does not enable an outer unattended agent loop for benchmarking.

Cover text edits, deterministic tests, multi-file repair, long-context continuation/compaction, images and verified native collaboration. Evaluate evidence-only and automated packets only after later admission. Unsupported cases are capability findings, not numerical victories. Record separately:

- Process start, initialization, session create/load, dispatch, first event/visible text, turn terminal/post-turn activity and close/cleanup latency; login latency separately.
- Accepted model/effort, observed model/tool counts, scoped tokens/cache/compaction/quota. Absent public telemetry remains unknown; do not probe with extracted tokens.
- Deterministic tests, correctness, constraint retention, repeated work, missed tools and failed/unknown outcomes.
- Cancellation/recovery, process count, peak RSS, cold/resident costs; missing displayed reasoning does not establish model ability.

Report medians, tails, paired differences, uncertainty, raw distributions and failure counts. Initial adoption and expansion require predeclared correctness/resource margins; this proposal runs no benchmark and recommends no existing-default change. Residency experiments must include warm headless stdin, warm ACP and the current per-invocation baseline, avoiding attribution of all residency gains to the protocol.

## 12. Milestones and open decisions

| Milestone | Deliverable and exit condition |
| --- | --- |
| M0 — verify separate repairs | Independent evidence for baseline receipt/packaging corrections and cross-layer regression; headless behavior/defaults unchanged. |
| M1 — official standalone ACP client | Pinned Registry original/platform arguments, honest identity, explicit official personal login, text session, permissions, lifecycle and recovery acceptance. |
| M2 — interactive UI and delivery | Direct DSH/Flywheel external session, models/effort/permissions/stop/unknown states, public interactive API, exact package and candidate installation; no new automated candidates. |
| M3 — individual capabilities | Separate root-image and native-collaboration acceptance; preserve limitations on worker images/whole-child-tree unknowns. Evaluate CLI runtime independently. |
| M4 — evaluation and later decisions | Distinguish runtime/protocol/residency costs. Review future machine use/auth support and complete technical contracts separately; M2 does not automatically enable it. |

Resolve before implementation:

1. Exact standalone release, manifest/hash, platform arguments, optional methods and actual account models/effort; do not derive its capabilities from CLI versions.
2. Actual official `oauth-personal` interaction, shared-cache/logout effects and the browser/terminal bridge the client can support and validate.
3. Interactive API/session-record versions, root/child activity and per-turn quiescence, session-close cleanup and unknown remote-work limits.
4. Original-image delivery to root and required workers, and which requirements belong only to a later packet contract.
5. For proposed automation, separately prove supported use/authentication, active-agent evidence-only, Go durable ACK/recovery and role/Full/collaboration/input eligibility, then allocate machine/ledger versions.
6. Preregistered quality/latency/resource measures and whether practical benefits justify another runtime or broader use.

Approval reviews this initial interactive direction and later verification boundaries. It does not publish a package, ship implementation, guarantee account compliance or enable ACP for existing tasks.

## Sources and verification boundary

Sources were checked on 2026-11-22. Official docs/Registry establish public paths; pinned source establishes implementations; issues/community posts establish particular reports. A project's existence does not establish permission for every use. Google-flair moderators and ordinary forum accounts are distinct; unverified T3/OpenBot/OMP replies are not treated as official rulings here. This documentation PR ran no live ACP inference, login, quality benchmark, image propagation test or user-machine installation.

| ID | Source | Use |
| --- | --- | --- |
| S1 | [Google: CLI ACP](https://antigravity.google/docs/cli/acp) | Official native entry and editor integration. |
| S2 | [Google: headless mode](https://antigravity.google/docs/cli/headless) | Automation, persistent stdin, text-only input. |
| S3 | [Google: changelog](https://antigravity.google/docs/changelog) | CLI 1.5.0/1.5.1 history and lifecycle fixes. |
| S4 | [Google: ACP editor integration](https://antigravity.google/docs/ide/extensions/acp) | CLI ACP authentication, flags and session features; these cannot all be assumed for the standalone server. |
| S5 | [ACP v1 initialization](https://agentclientprotocol.com/protocol/v1/initialization) | Version/capability negotiation. |
| S6 | [ACP v1 content](https://agentclientprotocol.com/protocol/v1/content) | Image and other typed prompt content. |
| S7 | [ACP v1 session configuration](https://agentclientprotocol.com/protocol/v1/session-config-options) | Runtime option IDs and accepted values. |
| S8 | [ACP v1 subagents](https://agentclientprotocol.com/protocol/v1/subagents) | Optional child-session exposure and ownership. |
| S9 | [T3 discussion #28315](https://github.com/pingdotgg/t3code/discussions/28315) | Single-author old/native ACP startup comparison. |
| S10 | [2BAB comparison](https://2bab.me/en/blog/2026-11-11-antigravity-hub-vs-acp/?source=bokelianmeng.com) | Task-dependent old-ACP/hub timings and limits. |
| S11 | [Theo original X post](https://x.com/theo/status/2118107369448276371), [readable mirror](https://nitter.gq/theo/status/2118107369448276371) | Firsthand bindings/false-report claim; direct X body unavailable in this review. |
| S12 | [T3 issue #28172](https://github.com/pingdotgg/t3code/issues/28172) | Historical streaming bug and later native-runtime retest. |
| S13 | [Antigravity issue #1533](https://github.com/google-antigravity/antigravity-cli/issues/1533) | Cross-mode cache reports, not confirmed root cause. |
| S14 | [Antigravity issue #1402](https://github.com/google-antigravity/antigravity-cli/issues/1402) | Versioned timeout/session recovery reports. |
| S15 | [DSH community feedback](https://linux.do/t/topic/3173768) | Uncontrolled context-loss experience report. |
| S16 | [ACP v1 prompt turn](https://agentclientprotocol.com/protocol/v1/prompt-turn) | Prompt, updates, replay distinction and terminal/cancel semantics. |
| S17 | [ACP Registry: antigravity-acp manifest](https://raw.githubusercontent.com/agentclientprotocol/registry/main/antigravity-acp/agent.json) | Observed standalone 1.5.2 on 2026-11-22, Google URLs, platform commands/arguments and SHA-256; not an accepted default version. |
| S18 | [Google: Android Studio](https://antigravity.google/docs/ide/extensions/android-studio/) | Official agent installation through ACP Registry. |
| S19 | [Google: Authenticate](https://antigravity.google/docs/install) | Official sign-in and shared local authentication; no promise of independent credential caches. |
| S20 | [Google: Terms of Service](https://antigravity.google/terms) | Section 4's supervised/autonomous agents and responsibility; section 6's third-party restrictions. |
| S21 | [Google: FAQ](https://antigravity.google/docs/faq) | Direct headless scripting/CI versus third-party agents using personal sign-in. |
| S22 | [Google-flair moderator chunduriv, 2026-10-08](https://discuss.ai.google.dev/t/is-google-ai-pro-antigravity-cli-automation-with-codex-permitted/187241/2) | Lists ACP editors as supported and distinguishes third-party coding-agent wrapping; not a legal ruling. |
| S23 | [T3 official CLI download code](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/antigravityCliRelease.ts), [spawn configuration](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/antigravityAuthSupport.ts), [generic Registry resolution](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/packages/provider-acp-registry/src/server/AcpRegistrySupport.ts) | Pinned main source; built-in provider uses the CLI, distinct from the generic Registry provider. |
| S24 | [T3 sign-in implementation](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/Drivers/AntigravityDriver.ts) | Official `acp --login` process receives the user's code. |
| S25 | [T3 client identity](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/acp/AntigravityAcpSupport.ts), [quota probe](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/antigravityUsageLimits.ts) | Legacy `zed` fallback and token/private-quota access; excluded from this design. |
| S26 | [zacbemis Pi ACP community extension](https://github.com/zacbemis/pi-antigravity-acp-provider/tree/6d6135d888cb50e2cb1c18bc2520a94ac7d26392), [runtime.ts](https://github.com/zacbemis/pi-antigravity-acp-provider/blob/6d6135d888cb50e2cb1c18bc2520a94ac7d26392/src/runtime.ts), [MCP bridge](https://github.com/zacbemis/pi-antigravity-acp-provider/blob/6d6135d888cb50e2cb1c18bc2520a94ac7d26392/src/mcp/bridge.ts) | Official standalone agent/own auth plus an outer Pi provider/tool loop; not Pi core. |
| S27 | [Another Pi Antigravity extension](https://github.com/Rahularya01/pi-antigravity) | Direct OAuth and ACP integrations must be distinguished by repository. |

[C1]: https://github.com/ckanner/flywheel/blob/291c0a02f9042d522d3e0f3c62977f63149465cd/internal/drivers/antigravity.go
[C2]: https://github.com/ckanner/flywheel/blob/291c0a02f9042d522d3e0f3c62977f63149465cd/plugins/dsh/src/antigravity-bridge.js
[C3]: https://github.com/ckanner/dsh-subagent-antigravity/blob/a20eb63bbfa2849df63043595475957bf4a4f04d/lib/runner.js
[C4]: https://github.com/ckanner/dsh-subagent-antigravity/blob/a20eb63bbfa2849df63043595475957bf4a4f04d/lib/machine.d.ts
