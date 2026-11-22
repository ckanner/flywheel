# Optional native ACP transport for Antigravity

English · [简体中文](2026-11-22-antigravity-native-acp.md) · [Proposal index](README.md) · [Current Antigravity guide](../en/antigravity.md)

- Date: 2026-11-22.
- Status: **proposal for review; documentation only**. No ACP runtime, configuration field, image capability, or default change ships with this document.
- Baseline: Flywheel `291c0a02f9042d522d3e0f3c62977f63149465cd` / 0.38.0 and `@ckanner/dsh-subagent-antigravity` 0.2.1, source `a20eb63bbfa2849df63043595475957bf4a4f04d`.
- Prerequisite: fix and independently validate the existing evidence-only receipt mismatch described in §2. This proposal is a separate change from that repair.

## 1. Decision and intended outcome

Keep the official headless stream-json transport as the default. Add an explicitly selected **native `agy acp`** transport to the standalone Antigravity plugin, then consume that public interface from Flywheel. Preserve the same deterministic orchestration, subscription policy, invocation journal, session ownership, and delivery gates.

The purpose is to make image input and structured bidirectional interaction available where verified, and to improve session/configuration observability. A speed or quality improvement is a hypothesis to measure, not a reason to silently change an existing task's transport.

ACP here means **Agent Client Protocol**. The target is the server inside the official `agy` executable, not the older standalone Python `.par` distribution, a third-party wrapper, a reverse-engineered `--hub` interface, or Gemini CLI under another name. Google documents both native ACP for editor integration and headless operation for scripts/CI [S1, S2]. Choosing headless for Flywheel was reasonable; neither interface is universally superior.

### Scope and non-goals

The first implementation covers macOS/Linux owned invocations, ordinary text execution, explicit native history resumption, and capability-gated additions. It must work through the standalone DSH plugin and the Flywheel worker without a second DSH parent model issuing the same task again.

This proposal does not change model defaults, Auto/Quick/Full, approved-plan semantics, JEV's single enable switch, permissions, subscription billing policy, or independent Review. It does not introduce remote agents, automatic login, credential copying, a shared background ACP daemon, a new provider named ACP, or automatic cross-transport recovery. Persistent process reuse and mid-turn steering require later gates below.

## 2. Existing system and prerequisite repair

The current path is:

`Flywheel Go owner → Antigravity worker → public plugin machine API → owned agy process`.

The plugin owns the native process group and durable recovery journal. The Go owner validates the result, terminal state, identity, configuration and cleanup before consuming it. The DSH UI projects engine facts rather than writing task state. Relevant sources are [the Go driver][C1], [the worker bridge][C2], [the plugin runner][C3], and [the machine types][C4].

| Current fact | Consequence for this proposal |
| --- | --- |
| The runner starts `agy --input-format stream-json --output-format stream-json`, sends one prompt and closes stdin. | There is process startup per invocation, even if native history is resumed. Native history and process residency are different properties. |
| JEV and investigation analysis require evidence-only; ordinary execution requires the currently supported explicit full-access binding. | ACP must meet the same authority contract before becoming eligible for that role. |
| Model, effort, physical cwd, subscription controls and native mode are frozen into invocation/session identity. | Transport and negotiated configuration must join that identity. |
| Images are currently admitted through a verified Claude execution binding. | Advertising ACP image support alone cannot change packet admission. |
| Swarm checks Full, reviewed independent work and input requirements; root completion is distinct from child observation. | These checks remain transport-independent. |
| Recovery reads a journal and never replays a possibly dispatched prompt. | An ACP session ID or a successful reconnect cannot weaken this rule. |

**Prerequisite defect.** At this baseline the 0.2.1 adapter accepts an evidence-only run whose CLI `init.tools` contains a nonempty catalog, provided its managed agent and runtime restrictions are verified. The Go owner still requires the catalog to be empty. A successful adapter result is therefore rejected as unknown; JEV keeps its frozen baseline and investigation analysis can block. Repair the Go proof checks and regression coverage first. Do not remove the restriction without validating the managed agent, and do not present a transport change as a fix for this cross-layer mismatch.

The repair must exercise adapter → worker → Go owner → decision/analysis consumption and recovery with the same nonempty-catalog receipt. Its acceptance evidence is a dependency of ACP work, not a result claimed by this proposal.

## 3. Evidence and alternatives

### 3.1 Official capability baseline

Google's CLI changelog dates the documented `agy acp` entry to 1.5.0 (2026-11-04), and broadens account-model availability to all ACP clients in 1.5.1 [S3]. This is a historical compatibility marker, **not** a recommendation to pin an old release. Select a specific current binary only after the conformance matrix in §11 passes.

The editor guide still contains a restricted-client model statement that conflicts with the later changelog. Resolve this by checking the pinned binary's actual session options; do not impersonate another editor to obtain models [S3, S4].

ACP negotiates protocol version and capabilities; protocol version 1 does not imply every optional feature exists [S5]. The current specification includes image content, configuration options, and optional subagent presentation [S6–S8]. Implementation support must be established separately. The first client speaks tested v1 features only, not v2 preview features inferred from current navigation.

Headless stdin supports multiple turns in one process, although this plugin currently closes stdin after each prompt. Its documented input content is text-only [S2]. Thus process reuse is an experiment available to both transports; image support is a separate capability question.

### 3.2 What the performance reports establish

| Evidence checked on 2026-11-22 | Observation | Limitation and design consequence |
| --- | --- | --- |
| T3 discussion #28315, 2026-10-31 [S9] | One Linux comparison reports cold initialization around 3.0–3.5 s for the old `.par` server versus 0.26–0.50 s for native ACP. | Both sides are ACP. This distinguishes implementations, not an ACP tax versus headless. |
| 2BAB's 24-run comparison, 2026-11-11, updated 11-17 [S10] | Old ACP versus hub medians: text 11.79/9.21 s; file workflow 29.65/32.23 s. | Different versions and default context/tool policies; same-build native ACP latency was not rerun. No universal speed ranking follows. |
| Theo's X post, 2026-11-04 [S11] | Reports problematic bindings and roughly 5% fewer false reports after a CLI rerun. | No complete controlled evaluation; 5% concerns false reports, not latency or a protocol-wide quality coefficient. The X body was available through a public mirror, not a successful direct X read. |
| T3 issue #28172 [S12] | Old-server reasoning/stream completion problem; maintainer could not reproduce with native agy 1.5.12 and closed it on 11-20. | Historical symptoms must not be presented as a current universal native-ACP defect. Displayed reasoning is not an independent quality measure. |
| Upstream cache issue #1533 [S13] | Users report cache drops across interactive, headless and ACP use. | Backend explanations are not confirmed root causes; keep cache telemetry and do not promise immunity on either path. |
| Upstream timeout issue #1402 [S14] | Includes a native-ACP 1.5.5 timeout/recovery report on Windows 11 with Vertex AI/ADC, alongside TUI/headless reports. | That platform/auth path is outside this proposal's macOS/Linux subscription-only baseline. It motivates lifecycle tests, not a claim that our exact path reproduced the defect. |
| DSH user feedback, 2026-11-13 [S15] | Reports repeated work and lost context in an ACP integration. | Missing exact client/runtime/configuration evidence; not identified as this plugin. Test compaction/history rather than attributing it to the protocol. |

No same-binary, same-account, same-model/effort/context/tools quality benchmark between current native ACP and this headless adapter was established in this research. Neither “ACP necessarily degrades quality” nor “the transports are proven equivalent” is justified.

### 3.3 Alternatives

| Choice | Decision |
| --- | --- |
| Retain headless only | Valid for today's text automation, but leaves the verified image/interactive capability gap. |
| Add native ACP behind explicit transport selection | **Proposed.** Reuses the official CLI while allowing isolated conformance and comparison. |
| Replace headless by default immediately | Rejected until correctness and comparative evidence support migration. |
| Add the old `.par` server or third-party wrapper | Out of scope; introduces another runtime and confounds lifecycle and performance evidence. |
| Adopt internal hub RPC | Out of scope; not needed for a client that already owns its UI, and no public contract is assumed. |

## 4. Ownership and protocol boundaries

The standalone plugin owns both transports. Shared modules retain input validation, subscription inspection, image manifests, session leases, journal durability, cancellation and receipt construction. Transport modules translate only the native wire protocol and its versioned observations. Flywheel imports this public API; it must not embed another ACP client or parse native ACP independently.

The first release keeps **one owned `agy acp` process and at most one active prompt per invocation**. A scoped invocation can load the previous native history in a new process, preserving current session reuse policy. Closing an invocation does not delete native history. This deliberately preserves the current process ownership boundary; warm reuse is a separate measured optimization, not a condition for basic ACP availability.

The client initially advertises no client-hosted filesystem, terminal or elicitation capability unless its handler is implemented, policy-bound and tested. Native CLI tools remain native. Send no additional `mcpServers` during session setup; the agent's `mcpCapabilities` describes supported MCP transports, not authority to introduce a server [S5]. Unexpected requests for unadvertised optional services fail visibly; they cannot cause ad-hoc file access or command execution in the UI process. Baseline permission requests still need a real handler.

Use the real plugin/client name and version in `clientInfo`. Record executable path, version and a non-secret executable identity, adapter source/package identity, selected transport and protocol, and negotiated capability digest. Resolve the same executable for preflight and execution; an in-place binary/configuration change invalidates admission. No token or credential content enters these identities.

### Proposed interface allocation

The following is a design target, **not accepted configuration today**:

```json
{
  "profiles": {
    "agy-acp": {
      "harness": "antigravity",
      "transport": "acp",
      "model": "gemini-4-hydrogen-medium",
      "thinking": "medium",
      "channel": "google-subscription"
    }
  }
}
```

| Surface | Proposed contract |
| --- | --- |
| Model/role/execution binding | Optional `transport` with values `"headless"` or `"acp"`, valid only for `harness: "antigravity"`; omission means headless. It follows existing profile expansion and enters every frozen binding. |
| Standalone plugin settings | The same transport selector, default headless; execution tool callers may not broaden authority by overriding it. |
| Public machine API | Allocate a new request/receipt protocol for ACP and explicit transport facts; retain the current protocol-1 headless reader. Old readers must reject ACP rather than silently ignore its field. |
| Flywheel worker/lifecycle/recovery | Negotiate the pinned adapter's supported versions, pass transport through every path, and require matching receipts. Allocate the next necessary outer protocol/ledger version before writing facts an old engine could misinterpret. |
| Capabilities | Report advertised, locally verified, live-verified and unsupported/unknown separately, keyed by binary, model/mode and adapter identity. A generic `acp: true` is insufficient. |
| Runtime configuration | Preserve requested, resolved and observed model/effort/permission values separately. A request echo is not an independent backend observation. |

The profile fragment assumes the complete configuration already defines the native `google-subscription` channel and existing subscription policy; it is not a standalone installation configuration.

The implementation PR must settle exact version numbers and schemas together across public plugin types, bridge, Go owner, state, recovery and UI. It must not merely add an optional field to strict readers. Existing headless journals keep their original identity and version.

## 5. Initialization, model selection and prompt admission

Perform the following before any business prompt:

1. Validate the request, physical directories, owned recovery location, supported platform and remaining deadline; apply existing subscription controls.
2. Start the owned ACP process, initialize with the supported protocol and honest client capabilities, and validate the response. Unknown required features fail before dispatch.
3. Create a session or load the exact owned session allowed by the frozen scope/binding. Do not choose “latest.” Treat history replay as replay, not new work or new usage.
4. Read returned configuration options and apply the exact model/effort and permission selection through the tested public mechanism. Prefer config options where supplied [S7]. Option IDs and values are runtime facts, not assumed spellings such as `thought_level` versus `thinking_level`.
5. Require a returned accepted configuration and record the strength/source of that evidence. A missing model, unsupported effort, mismatching cwd/mode or changed capability set stops admission. Orchestrator's fixed effort remains fixed; never substitute high for an empty effort.
6. Freeze the complete prompt and image manifest, verify the managed agent when required, persist dispatch-possible evidence and obtain the Go lifecycle ACK before writing `session/prompt`.

A handshake or catalog lookup performs no business inference and proves neither entitlement nor quota. If obtaining a session catalog creates native history or triggers native auxiliary work, label that side effect; do not call it a zero-side-effect probe. Start with the existing CLI model catalog for ordinary no-inference diagnostics. Any ACP-specific session probe uses an isolated owned workspace/history and no business prompt.

Configuration changes after dispatch are observations, not permission to reinterpret the request. Reject an unexplained model/authority change and retain uncertainty about already dispatched work. DSH/Flywheel may change the next invocation at a safe boundary; there is no silent mid-turn model switch.

## 6. Invocation, cancellation and recovery

Each journal binds Flywheel request ID, adapter run ID, native session ID, connection generation and the JSON-RPC prompt request ID. A client-generated RPC ID is **not** a provider idempotency key or a native turn ID. At most one business prompt is outstanding per owned session in the first version, making event attribution mechanically checkable [S16].

| Event or interruption | Required behavior |
| --- | --- |
| Failure before prompt bytes can be written | Record proven not-dispatched only if the dispatch gate and cleanup support that statement. |
| Write attempted, connection lost, malformed/oversized frame or timeout | Preserve possible/accepted dispatch and unknown outcome unless a trusted correlated terminal result exists; never resend automatically. |
| Normal prompt response | Validate the correlated stop reason, output and configuration. Drain supported post-prompt work to a verified completion/shutdown boundary, then confirm process-group cleanup before successful consumption. The prompt response alone is not the final adapter receipt. |
| `session/cancel` sent | Cancellation is a notification, not a receipt proving work stopped. Reply to every pending `session/request_permission` with the protocol's `cancelled` outcome; while the root prompt is pending, keep draining updates until its response [S16]. Confirm owned-work cleanup, using bounded TERM/KILL as needed. |
| Partial text followed by cancellation, refusal, resource cutoff or error | Preserve text as partial evidence; never promote it to a completed task. Map errors into the existing taxonomy while retaining raw native reasons. |
| Late progress or history replay | Correlate session and generation; do not append prior-session content, invent a new invocation or count usage twice. Distinguish legal post-prompt activity from work contradicting an already claimed final quiescence/cleanup proof. |
| Engine/worker restart | Read the original durable journal and consume a valid terminal receipt once. A successful session load cannot prove whether the lost prompt ran. |
| Missing terminal or cleanup proof | Block under the existing unknown-outcome rules. Do not cold-start, switch transport, reset a budget or infer safety from an empty UI. |

ACP turn cancellation and local process cleanup are different from cancellation of any unobserved remote work. Do not turn the existing `providerWork.cancelSupported`/`querySupported` facts into true merely because ACP has `session/cancel`/`session/load`. Record the narrower transport capability separately.

The optional child-session contract permits child and parent activity after the root prompt response [S8]. Continue handling owned updates and permissions during a bounded drain; a final adapter receipt must not erase known unfinished required work. If it cannot settle within the deadline, retain partial output/evidence and an unknown invocation outcome, along with cancellation evidence. A parent cancellation after that response has no pending prompt response to acknowledge it; require tested shutdown/quiescence and confirmed cleanup instead of inventing a cancelled terminal. Do not call legal post-prompt updates a protocol violation or infer successful child completion from killing the process.

The normalized receipt keeps execution outcome, terminal receipt, local cleanup, requested/observed configuration, native collaboration, images, usage and provider-work uncertainty distinct. With one process per invocation, successful completion still requires a clean owned process boundary; a later persistent-process design must introduce an explicit quiescence proof rather than pretend every turn has an exit code.

Session reuse remains scoped to task, role, goal, physical workspace, transport, account/config references, model/effort, authority, native mode and input contract. Independent reviewers never receive executor history. Cross-transport history reuse is disabled until an explicit compatibility test and migration contract exist. Changing transport cannot retire a pending unknown invocation.

For compaction, preserve the current authoritative contracts and existing full/verified-incremental policy. Native compaction metadata is observed only if supported; absence stays unknown. Replayed history or a summary must not overwrite approved plans, write scopes or acceptance criteria. Test task constraints before and after compaction, rather than judging continuity by transcript length.

## 7. Permissions, subscription controls and evidence-only

### Subscription and configuration

Reuse the same environment filtering and local controls as headless: reject API/ADC/GCP/custom-provider routing under subscription-only, keep credits fallback disabled, and recheck relevant configuration before dispatch and after execution. ACP's available authentication methods do not authorize switching billing channels [S4].

Use the user's existing native sign-in. On authentication failure, return AUTH and direct the user to the official login flow outside the task. Do not read/copy OAuth tokens, invoke login during unattended execution, change an account or select a key-based fallback. Keep `verified-local-controls`, account entitlement and actual billing/usage knowledge separate.

### Tool permissions

Bind every permission request and response to the owned session, invocation and request identity. Apply existing explicit grants and native deny rules. A `selected` outcome must use an `optionId` the server actually offered; cancellation instead uses the protocol's `cancelled` outcome, never an invented option or an unanswered pending request. Do not persist an “allow always” choice or widen directories without the corresponding user authority. A native plan/accept-edits mode label alone does not prove OS isolation.

The initial unattended Flywheel path supports only permission combinations already proven by the adapter. An unexpected approval beyond those grants is denied/reported, not left waiting indefinitely or automatically approved. Standalone interactive DSH prompts may use a verified host approval bridge; if that bridge is unavailable, reject the interactive mode before sending the task. Client-hosted fs/terminal and external MCP remain disabled until separately implemented and tested.

### Evidence-only

JEV continues to make fresh, tool-free decisions using the same single `jev.enabled` policy. It cannot delegate, use Orchestrator/Swarm, or acquire tools because the transport changed.

ACP evidence-only is eligible only after the pinned runtime can prove the same managed agent identity and immutable configuration: tools/MCP/skills/plugins disabled, command execution off, agent bound to run ID and digest, verified before dispatch and cleanup, no actual tool or child steps. A nonempty advertised tool catalog is not itself a violation; a real tool request/step is.

Whether native ACP exposes enough active-agent evidence is an **open verification item**. Passing `--agent` or inspecting the file alone is not proof the active session selected it. Until the tested server/adapter can establish equivalent evidence, ACP is explicitly ineligible for JEV and investigation analysis; headless remains available after the prerequisite repair. Never erase `evidenceOnly` to pass admission.

## 8. Images and native collaboration

### Original image delivery

Admission requires the server's negotiated image prompt capability and a validated adapter/model path [S5, S6]. Reuse the repository's authorized-image and frozen-manifest rules: read allowed ordinary files within the authorized root, enforce format/size/count limits, bind MIME, byte count and SHA-256 before dispatch, and recheck identity when saving/admitting the plan. A file path or resource URI is not evidence that original pixels reached the model.

The transport encodes the frozen bytes in image content blocks. The journal stores the input manifest and durable delivery facts without dumping credentials or base64 into progress logs. Distinguish local serialization, prompt acceptance and live end-to-end image validation; none alone proves what an unobserved child received. Use an image whose answer cannot be obtained from its filename, accompanying text or repository metadata for live acceptance.

Replace the current backend-specific image eligibility test only with a shared verified capability predicate, consumed by both engine and UI. Before that capability is validated, keep the present Claude requirement. Unsupported images fail before dispatch; there is no text-only fallback and no removal of the packet image declaration.

### Orchestrator, explicit subagents and Swarm

| Mode | ACP implementation requirement |
| --- | --- |
| Ordinary native / Orchestrator | Preserve exact model selection and fixed effort semantics; distinguish autonomous child use from observed child facts. |
| Explicit subagents | Require the pinned runtime's verified delegation mechanism and sufficient actual child observations for the current contract. No observation means it cannot certify this mode. |
| Swarm | Admit only Execute + Full + reviewed independent collaboration/writes + satisfied inputs. Verify the supported native command/mechanism; do not assume an arbitrary slash-looking prompt invokes it. |

ACP subagent presentation is optional; the protocol permits delegation without exposed child sessions [S8]. Record requested mode, actual child IDs, root completion and child outcomes separately. Keep unknown child model/usage/state unknown. An observed-child count guard is not a native hard concurrency limit, and root prompt completion is not independent verification that every child succeeded.

Pin the ACP schema/SDK as well as the CLI. Advertise the documented v1 client `subagents` capability only with a matching, implemented child-session handler; any draft-only variant in the selected versions requires a separate explicit capability gate. Accept a child update only after a validated ownership event connects that child to the invocation's owned root. The capability requests presentation, not native delegation or a guaranteed child stream.

Images introduce an additional distinction: verified root image input versus verified delivery to image-dependent workers. Retain the existing Swarm image exclusion until worker input requirements are expressible in the reviewed contract and propagation has been validated. Text-only downstream work may eventually use a reviewed derivative specification, but deleting an original-image requirement is not a migration strategy. ACP alone does not change any of these rules.

## 9. Configuration, UI and diagnostics

Transport is an advanced property of an Antigravity execution binding, not a provider or a model. Keep it independent from `native-profile`, reasoning level and the JEV enable switch.

The UI must:

- preserve headless when a user edits an existing binding; expose ACP only when the installed adapter supports it;
- show initial configuration, next selected binding and the last actual call with their own transport/configuration facts;
- show native protocol/CLI/adapter versions and specific eligibility failures without claiming directory discovery is entitlement;
- use the engine's shared eligibility result for role, Full/collaboration, permissions, image and evidence-only requirements;
- keep editing scope distinct from the task's effective scope, and reread effective configuration after save;
- label advertised, tested and unknown capabilities distinctly, including root versus worker image support;
- show ordinary model labels and decision reasons, with raw protocol IDs in expandable detail;
- disable unsafe edits while the corresponding invocation is pending and preserve stale/unknown request state.

No changes to the initial model defaults or existing tasks are included. A future catalog refresh must bind its asynchronous response to the exact transport/target/configuration revision, just as current provider/model results are correlated.

## 10. Migration, release and rollback

1. Land the receipt/packaging corrections independently and validate headless behavior. Keep their historical source/registry/live evidence separate.
2. Implement native ACP in the standalone plugin, with explicit public protocol/capability versions, documentation and isolated package-consumption tests. Keep protocol-1 headless and old recovery records readable.
3. Publish a separately authorized tested plugin release, then pin its exact package identity in Flywheel. Update schemas, driver/bridge/recovery/session identity, diagnostics and UI in one coherent implementation PR.
4. Preserve omitted-transport → headless behavior. Reject ACP on an older adapter/engine; do not silently run headless. Use a ledger compatibility marker before writing ACP state that an old reader could misinterpret.
5. Enable text ACP explicitly for admitted configurations; enable images and evidence-only independently as their gates pass. Existing task inputs, approvals and historical receipts are not rewritten.
6. Reverting an idle configuration to headless is allowed only at a safe boundary. Retain original journals/readers to reconcile ACP invocations. An unresolved ACP turn prevents replay through headless; rollback is not cancellation or proof of non-dispatch.

A shared warm ACP process, parallel sessions in one server, and automatic steering are later proposals/gates. Flywheel feedback remains queued at the existing safe adoption boundary. Allowing a concurrent `session/prompt` as steering would change the frozen input and invocation attribution; it requires its own versioned authorization/input-revision record, not merely a button that forwards text.

## 11. Verification and comparative evaluation

### 11.1 Correctness gates

| Area | Required cases before the capability is eligible |
| --- | --- |
| Wire/configuration | Unsupported protocol/options, exact model and effort, changed binary/config, capability mismatch, bounded Unicode/JSON framing, unknown optional versus invalid required messages. |
| Authority | Every supported permission mode, denied request, wrong-session/late approval, out-of-root operation, prohibited auth route, settings changed during a run, evidence-only wrong/mutated agent and real tool/child steps. |
| Lifecycle | Cancellation before/after dispatch, during tools/permissions and after the root response with active children; timeout then next prompt, transport loss, parent death, SIGKILL, missing/duplicate/contradictory terminal events, late updates and confirmed group cleanup. |
| Recovery | Crashes around each durable ACK, receipt-before-consumption, exactly-once local consumption, unknown stays blocked, exact session restoration, no implicit replay or cross-transport cold start. |
| Context/accounting | Replayed history not duplicated, role isolation, full/incremental input, compaction retaining task constraints, cumulative counters not summed as per-call values, missing/negative/inconsistent deltas stay unknown. |
| Images | Actual frozen bytes, changed source rejection, MIME/size bounds, unsupported capability, root visual task, required worker propagation and unavailable propagation rejected. |
| Native modes | Orchestrator fixed effort, explicit subagents absent/present observations, Swarm command acceptance, Full/approved scopes, missing child telemetry, observed-count cutoff and root/child completion distinction. |
| Distribution/UI | Clean public-package import, version rejection, same candidate install on Linux/macOS, packaged docs/provenance, shared eligibility, config scope and asynchronous model/transport selector identity. |

Use the repository's existing gates, including 100% measured Go statement coverage per function when Go changes, plugin behavior/bridge checks, and actual candidate installation for release changes. Do not lower a gate or replace the public adapter with a fake package claiming the same version. Offline fixtures prove contracts; a separate live record is needed for the selected CLI, account, model, image and native-mode capabilities.

### 11.2 A/B design

Before trials, freeze the task set, scoring and acceptance margins. Compare the **same** official executable bytes, account/auth path, model/effort, settings/rules/plugins, tool authority, workspace snapshot, inputs and history/compaction policy. Randomize/alternate path order; run each case at least ten paired repetitions for an exploratory report, recording all failures. Use fresh matched sessions for cold trials and equivalent prior turns for warm trials. Do not call uncontrolled backend cache state “held constant.”

Cover short evidence decisions, a deterministic edit with tests, a multi-file repair, long-context continuation/compaction, an image task and approved collaboration. Cases unsupported by one path are capability results, not numerical performance wins. Keep live jobs in authorized disposable workspaces; no publishing or user-profile mutation.

Record separately:

- process start, ACP initialize / headless ready, session create/load, prompt dispatch, first event, first visible text and full completion/cleanup time;
- actual accepted model/effort, model/tool call counts when available, input/output/thinking/cache tokens with scope, compaction observations and quota errors;
- deterministic tests, correctness/rubric results, constraint retention, duplicate work, missed tools and failed/unknown outcomes;
- terminal/cancel/recovery correctness, process count and peak RSS; absent telemetry remains absent.

Report medians, tails, paired differences and uncertainty; show raw distributions and failure counts rather than one percentage. A default change needs no unresolved correctness/authority regressions and evidence of useful task outcomes within the predeclared latency/resource margins. Select those margins before observing results; this proposal claims neither a benchmark result nor a safe default version.

A persistent-process experiment must compare warm ACP with warm headless stdin as well as today's one-process-per-invocation baseline. Otherwise it would measure process residency together with the protocol and overstate an ACP-specific benefit.

## 12. Milestones and decisions to resolve

| Milestone | Deliverable and exit condition |
| --- | --- |
| M0 — repair current behavior | Managed-agent receipt fix, nonempty-catalog cross-layer regression, package/provenance corrections; headless remains default. |
| M1 — native ACP conformance | Pinned CLI, protocol/options/auth/permissions probe record, owned text invocation, journal/cancel/recovery tests; only explicitly admitted text configurations. |
| M2 — public integration | Standalone plugin release plus Flywheel pinned integration, strict protocol/ledger migration, UI transport/effective-config controls and candidate installation. |
| M3 — independent capabilities | Root images, evidence-only and native modes each pass their own offline/live gates; worker image propagation remains separately conditional. |
| M4 — evaluation | Reproducible paired task report; accept, limit or decline wider/default use based on results. Warm reuse/steering remain separately reviewed changes. |

Before implementation completes, resolve and record:

1. The exact CLI build and optional ACP methods actually supported; model/effort/mode mapping and non-Gemini account access without spoofing.
2. Whether ACP exposes sufficient active-agent evidence for evidence-only. If not, keep that role on verified headless and state the limitation.
3. Version allocation for machine/worker/lifecycle/state facts, including unambiguous loading/replay versus active-prompt attribution.
4. Whether original images can reach every worker that needs them, and how the reviewed contract represents that requirement.
5. Native shutdown/quiescence behavior, context/usage accounting scope, and whether session-level resume provides any additional **proven** recovery beyond history access.
6. Preregistered evaluation tasks, quality criteria and latency/resource margins; which practical gains justify the maintenance cost.

The recommended next action is M0, followed by an explicit M1/M2 implementation proposal. Approval of this document does not itself publish packages, enable ACP for existing tasks or certify a user's installed environment.

## Sources and verification boundary

Sources were checked on 2026-11-22. Official documentation establishes a supported interface; issue reports establish reported behavior in named conditions. They do not replace local conformance or live acceptance. No live ACP inference, quality benchmark, image propagation test or user-machine installation was performed for this documentation PR.

| ID | Source | Use |
| --- | --- | --- |
| S1 | [Google: CLI ACP](https://antigravity.google/docs/cli/acp) | Official native entry and editor integration. |
| S2 | [Google: headless mode](https://antigravity.google/docs/cli/headless) | Automation, persistent stdin, text-only input. |
| S3 | [Google: changelog](https://antigravity.google/docs/changelog) | CLI 1.5.0/1.5.1 history and lifecycle fixes. |
| S4 | [Google: ACP editor integration](https://antigravity.google/docs/ide/extensions/acp) | Authentication, flags and session-facing features; model restriction text conflicts with S3. |
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

[C1]: https://github.com/ckanner/flywheel/blob/291c0a02f9042d522d3e0f3c62977f63149465cd/internal/drivers/antigravity.go
[C2]: https://github.com/ckanner/flywheel/blob/291c0a02f9042d522d3e0f3c62977f63149465cd/plugins/dsh/src/antigravity-bridge.js
[C3]: https://github.com/ckanner/dsh-subagent-antigravity/blob/a20eb63bbfa2849df63043595475957bf4a4f04d/lib/runner.js
[C4]: https://github.com/ckanner/dsh-subagent-antigravity/blob/a20eb63bbfa2849df63043595475957bf4a4f04d/lib/machine.d.ts
