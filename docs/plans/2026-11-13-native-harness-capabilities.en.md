# Native harness capabilities within Flywheel loops

**Status: proposal for discussion; no runtime implementation or default change.**

**Research date:** 2026-11-13, America/New_York.

**Repository baseline:** Flywheel 0.32.1, commit [63d8781][fw-base].

**Evidence:** official documentation, pinned source, and current adapter inspection.
This is not installed-package, live-account, model-quality, performance, or macOS
acceptance evidence.

中文：[原生 Harness 能力在 Flywheel 各环节中的接入方案](2026-11-13-native-harness-capabilities.md) · [Plan index](README.md).

## 1. Recommendation and ownership

Use native capabilities where they improve a bounded invocation. Keep Flywheel
responsible for the workflow around that invocation. A backend's richer internal
runtime should not silently become a second owner of the task.

| Layer | Owns | Does not acquire through this proposal |
| --- | --- | --- |
| Flywheel outer loops | Frozen contracts, packet dependencies, global resources, workspace ownership, independent verification, retries, recovery, commits, delivery, and final completion | A model-controlled replacement for the deterministic state machine |
| Adapter for one admitted native invocation | An explicitly selected and checked collaboration profile, its bounded helpers or tool execution, parent/child attribution, collection of results, and native cleanup | Authority to create unrelated Flywheel packets, change the frozen acceptance, bypass Review/Judge, or merge the outer task |
| Native agents and tools | The delegated subproblem and capabilities actually granted to that actor | Additional account access, unreserved model concurrency, cross-packet writes, or independent delivery authority |

An Executor can ask two helpers to inspect an API and its tests while it prepares
a change. The helpers' reports are inputs to that Executor. They are not
Flywheel's independent Review verdict. Likewise, a native workflow may structure
one approved audit, but it cannot decide that the enclosing task has passed.

The first practical targets are efficient tool execution, structured outputs,
and small collaborations whose restrictions can be verified. Multiwriter Teams,
Kimi Tower, and remote review require separate admission and lifecycle work.

## 2. Terms, models, and the inspected versions

### 2.1 Four identities must remain separate

| Identity | Meaning | Example |
| --- | --- | --- |
| Model | Reasoning, coding, language, context, and modality capabilities | Kimi K3.1 Pro or a configured GPT model |
| Provider/account | The authenticated service, entitlement, quota, and billing route | Kimi Code subscription; DeepSeek Account; a ChatGPT subscription |
| Harness | The agent runtime, tool loop, context management, and native delegation | DSH, Kimi Code CLI, Codex app-server, Claude Code |
| Adapter | Flywheel's checked invocation, observation, cancellation, and receipt interface | DSH worker; Kimi print driver; Codex Pro machine; Claude Code Pro machine |

Using the same GPT model through DSH does not provide Codex's native subagent
controls. Conversely, changing a harness does not authorize a different account,
model, price tier, or permission policy. Preserve the existing subscription
routes, Kimi K3.1 Pro, and the DeepSeek Flash account default. Native capability
selection is an additional decision inside an eligible binding, not an implicit
model migration. [Current bindings and model policy][fw-models]

Cross-provider division of work remains an outer Flywheel decision. Do not add a
DSH parent invocation merely to delegate to a Codex or Claude machine interface
that Flywheel can already call directly.

Official Kimi documentation lists image/video input and context up to 1,048,576
tokens for K3.1 Pro, subject to account entitlement. It requires thinking and
supports low/high/max. These are capability and access facts, not proof that
1M context is enabled for this user or that the model wins a Flywheel benchmark.
Its official coding and visual-development examples justify testing screenshot
analysis and large-repository investigation; they do not establish a cross-model
ranking. [Model specification][kimi-models] · [Official examples][kimi-model-blog]

A model's vision support is useful only if the invocation actually receives
the artifact. Kimi has a native ReadMediaFile tool, while Flywheel's current Kimi
request is a text prompt. A future media-aware profile must bind artifact paths,
content identity, permitted readers, supported formats, and observed delivery.
A screenshot filename in prose is not evidence that the model inspected the
image. [Kimi tools][kimi-tools] · [Current Kimi driver][fw-kimi]

| Goal | Combination worth testing | What must be established |
| --- | --- | --- |
| Routine coding and tool-intensive investigation | DSH with the existing Flash binding and verified PTC | Fewer tool round trips and acceptable final quality; lower price alone does not prove lower total cost |
| Frontend comparison and long-context planning | K3.1 Pro with explicit artifacts and bounded explore/plan helpers | Actual media delivery, account access and task-specific improvement |
| Difficult coding and focused review | Codex's native runtime with the configured Sol binding | Controlled native behavior and independent candidate review, not a capability inferred from a model name |
| Repeated analysis or transformations | An allowed Claude workflow or Kimi Server Swarm | A fixed script/item contract, known model choices and bounded resources |

Compare the same model under different execution profiles first. Compare model
or effort changes separately. Multiple children using the same model provide
separate contexts, not independent model diversity by definition. Record cache
usage, compaction overhead and observable subscription quota separately from an
API-equivalent price estimate.

### 2.2 Version audit

| Component | Inspected baseline | What that establishes |
| --- | --- | --- |
| Flywheel | 0.32.1 at 63d8781 | Current adapter behavior and project invariants |
| DSH SDK development dependencies | 0.2.2-rc.3; upstream source 38a6a5c965aa99ab525161efb7cc5cf1fe62fced; declared host range retains older supported versions | A development baseline, not identical capabilities for every supported desktop |
| Codex Pro | Bundled machine package 0.9.0 from source 37da3d75de32b809c4fb1a631ccc15191f3d87f9 | The adapter actually shipped by this repository; a newer upstream branch is not this artifact |
| Claude Code Pro | Optional machine peer 0.4.0; recorded source ec0b99176b67512f3ca1838ec2a349dcb0189214 | An optional integration that must be present and checked before selection |
| Kimi print | Driver documents the checked 2.4.1 contract; executable remains user-selected | It does not pin every user's installed Kimi to 2.4.1 |
| Kimi upstream | Release API reports 2.6.2; source inspection pins f082f04a093a3e71a2263c5461d52507dc31f83a; web changelog lists 2.6.0 | Source and documentation evidence with explicit version differences, not a 2.6.2 runtime certification |
| Codex/Claude native CLI | Official documentation reviewed on the research date | Available product interfaces; exact installed version and machine-adapter exposure still require probes |

Sources: [plugin package][fw-package], [bundled provenance][fw-vendor],
[Kimi release][kimi-release], [Kimi changelog][kimi-changelog].
The old Python [MoonshotAI/kimi-cli][kimi-legacy] is archived. Its Wire protocol
must not be presented as the current TypeScript Kimi Code integration.

## 3. Capability matrix and current integration boundaries

| Native capability | Useful work | Current Flywheel boundary | Proposed disposition |
| --- | --- | --- | --- |
| DSH programmatic tool calling (PTC) | Filter and combine tool results, run independent I/O efficiently, reduce repeated model turns | Do not assume every PTC operation is a model call or that existing tool scheduling is disabled; retain tool-policy enforcement | Early candidate after tracing and resource-class probes |
| DSH Agent Team | Parallel exploration, hypothesis comparison, bounded specialist contributions | Current policy rejects child agents; Team lifecycle and child receipts are not adapted. Shared checkout/write-scope coordination is not a hard filesystem boundary | Later, initially whole-call read-only or with verified per-child controls |
| Kimi Agent | Focused coder/explore/plan helpers, isolated context, native background work and continuation | Print may already delegate according to its runtime configuration; Flywheel does not expose a controlled collaboration profile or child ledger | Early bounded-helper candidate after profile and lifecycle work |
| Kimi Swarm | A template applied to multiple independent items with aggregated results | Current print interface has no supported mode-enabling control; official Server API has a programmable mode field | Separate Server adapter experiment |
| Kimi Tower | Missions with branches/worktrees, worker/reviewer roles, coordination and gated merges | Overlaps Flywheel's outer scheduling and delivery; no verified mode makes all those responsibilities external | Optional isolated work-unit experiment, not a normal Execute switch |
| Codex subagents | Parallel code reading, focused investigation, specialist context | Native configuration exists, but bundled machine 0.9.0 lacks a checked collaboration profile and complete child accounting | Extend the existing plugin interface before controlled enablement |
| Claude ordinary subagents | Focused research and review, tool-scoped custom roles | Public CLI definitions are usable design inputs; Flywheel's current Claude bridge accepts only full access and does not establish the requested child restrictions | Adapter/profile and permission work first |
| Claude dynamic workflows | Repeatable audits, independent-item migrations, cross-checked research | Official headless support does not itself provide Flywheel-owned budgets, child receipts, or complete stop/recovery | Later bounded workflow profile |
| Claude Agent Teams | Peer discussion and shared-task coordination | Experimental, with resume/shutdown limitations and more persistent actors | Later experiment after lifecycle probes |
| Native remote review | A separate review opinion or evidence source | Remote completion, account eligibility, exact candidate, results and cancellation need a separate protocol | Optional external evidence path; never replace local completion gates |

The current adapters must be inspected independently of native marketing terms.
DSH's [worker policy][fw-dsh-policy] and [bridge][fw-dsh-bridge], the
[Kimi driver][fw-kimi], [Codex Pro bridge][fw-codex-bridge], and
[Claude Code Pro bridge][fw-claude-bridge] are the baseline.

### 3.1 DSH: enable Team through an adapter contract

The rc.3 source provides several distinct capabilities:

| Capability | Upstream contract | Adapter implication |
| --- | --- | --- |
| PTC `run_code` | Parallel-safe nested calls can overlap; exclusive calls form barriers; `maxParallelSubCalls` defaults to 10 | The current overlay does not specifically disable PTC. Probe the actual executor/profile before claiming enablement or token savings [DSH PTC][dsh-ptc] |
| Local SubAgent | Fresh/forked sessions, messages and continuation; the shared tree pool defaults to 8 active children | A candidate for small, constrained helpers once creation and lifecycle policies are extended [SubAgent][dsh-subagent] |
| Agent Team | Durable tasks, dependencies and compare-and-set claims over the subagent tree | Its internal board does not become Flywheel's formal packet DAG [Team][dsh-team] |
| Web search/fetch | Search and fetch are separately configurable; ordinary fetching need not call an auxiliary model | The current overlay disables `tool-web`; distinguish ordinary retrieval from model-backed tools when admitting it [Web tools][dsh-web] |
| Plan, Goal and compaction | Separate planning, continuation and context mechanisms | Do not enable them as an incidental consequence of enabling Team [Plan][dsh-plan] · [Goal rounds][dsh-goal] |

Headless Team has an official SDK path: the standard `sdk` profile plus an
`agent-team` patch setting `defaultEnabled` to true. `sdk-minimal` omits these
services. The default is saved for new roots, not applied to resumed sessions.
An internal `setEnabled` service does not establish a public `team/setEnabled`
JSON-RPC method. [SDK guide][dsh-sdk] · [Protocol][dsh-protocol]

Team members share one checkout. `writeScopes` warns about conflicting work;
it is neither a filesystem lock nor a worktree boundary. Turning Team off does
not cancel children or erase its board. `session/wait` joins progressing tree
work, while idle children with parked input can remain resident; final drain
and shutdown still matter. External Codex/Claude/ACP children at the ordinary
DSH seam are one execution and do not inherit local-child continuation
semantics. [Team][dsh-team] · [SubAgent][dsh-subagent] · [Protocol][dsh-protocol]

Current Flywheel policy admits only the original root at `agent/created`, the
bridge rejects child dispatch, and usage folding consumes root events.
Removing a disabled-tool entry would not establish managed child identity,
permissions, accounting or cleanup. Creation-time policy is a candidate for
enforcing narrower child permissions, subject to testing every creation path.
[Policy][fw-dsh-policy] · [Bridge][fw-dsh-bridge] · [Usage fold][fw-dsh-protocol]

Native Plan provides guidance, not tool isolation. Goal's round allowance is
not a cumulative spending budget. PTC tool/I/O concurrency and additional model
agents therefore need separate resource classes. [Plan][dsh-plan] ·
[Goal rounds][dsh-goal]

### 3.2 Kimi: ordinary agents first, a Server adapter for Swarm

Kimi supports custom agent tool and subagent allowlists, checked again before
execution. `--agent-file` is creation-only and cannot be combined with
`--session` or `--continue`. Resume restores the original profile; resuming an
existing native child is exempt from the subagent-type allowlist. Bind profile
identity to the scope and reject incompatible continuation.
[Agent profiles][kimi-agents] · [Print creation/resume][kimi-print]

Flywheel currently allocates a durable `call-*` request identity before its
first call, but leaves the native session ID empty. The Kimi driver adds
`--session` only for a nonempty native ID and learns a fresh ID from the
successful terminal `session.resume_hint`. Consequently, a future creation-only
agent profile does not conflict with a preallocated native session ID: there
is none. The adapter still needs a profile parameter, a persisted profile
digest in its invocation/session binding, and verified restoration on resume;
it must not append `--agent-file` to every resumed call.
[Invocation lifecycle][fw-sessions] · [Driver][fw-kimi]

Ordinary `Agent` is not explicitly disabled everywhere in the current print
path, so native configuration may already permit delegation. The gap is
controlled admission and evidence. Native `--plan` conflicts with `-p`; use
planning helpers as inputs to Flywheel's own Plan contract. Skills/MCP remain
native extensions, and any model/tool routes they add must be accounted for.
[CLI contract][kimi-cli]

The current print mode has no supported way to enable Swarm. The official
changelog says the model cannot enable it itself. This is different from saying
there is no programmable interface: the Beta Server API can set swarm_mode in
a session profile or prompt, and tower_mode after creating a session.
Explicit authorization for a collaborative profile must remain visible in
Flywheel; do not simulate a UI user or dispatch undocumented internal events.
[Changelog][kimi-changelog] · [Server endpoints][kimi-session-api]

The Server API is a separate optional route, not an ACP migration. It exposes
REST sessions/profile/prompt and WebSocket task events; compare its installed
`/openapi.json` and `/asyncapi.json` with the adapter contract. Profile changes
are applied field by field without rollback, so a failed update requires
inspection of the resulting mode. [Server API][kimi-api] ·
[Session endpoints][kimi-session-api]
The adapter must own the runtime/session lifecycle and use an authenticated
local endpoint. Do not attach broad stop/shutdown behavior to the user's shared
desktop service or assume unrelated sessions belong to Flywheel.

A restored interactive session may contain mode state. Lack of a print flag is
not proof that every resumed session is in a known off state. Probe mode
restoration and bind the actual mode to the session contract.

Tower initializes repository state and owns mission branches, worktrees,
reviews and merges. Disabling one named tool is not proof that Bash and every
descendant cannot make equivalent Git changes. Existing Tower safeguards
protect its protocol state; they do not implement Flywheel's external oracle.
No checked public mode delegates only workers while handing all Tower
coordination to Flywheel. [Tower guide][kimi-tower] ·
[Merge implementation][kimi-tower-merge] · [Bash guard][kimi-tower-guard]

Its headless path requires the experimental Tower gate, a created session, then
`POST /api/v1/sessions/{id}/profile` with the Tower mode and base in
`agent_config`; creation itself refuses `tower_mode=true` because initialization
can mutate the repository. This is an integration target to isolate, not a
print flag that is safe to add to an existing Execute call.
[Session endpoints][kimi-session-api]

### 3.3 Codex: preserve the subscribed machine path

Official subagent configuration has `agents.enabled` defaulting to true, with
`agents.max_concurrent_threads_per_session` as its cap and `agents.max_threads`
as a legacy alias. Children inherit parent sandbox/approval settings; a custom
agent file cannot override them. A role named explorer or a custom file does
not establish a stricter child sandbox than the parent. If the main invocation
can write, read-only helper claims require a separately verified enforcement
mechanism. A whole-call read-only worker or separate Flywheel-owned read-only
invocations are alternatives. [Codex subagents][codex-subagents]

Extend the bundled Codex Pro machine contract instead of replacing it with an
unrelated paid API or silently selecting the legacy Codex CLI path. Current
root-focused lifecycle and usage observations are not complete proof of
descendant execution and cost. Capability probing must also establish the
baseline off profile, because upstream defaults may permit subagents.

App-server exposes start/resume/fork, turn steering/interruption, native
`review/start`, collaboration events and version-specific schema generation.
Review can be adapted in a fresh independent thread. Forking a frozen planning
context may help experimentation; forking the Executor's private history is
not the independent final Reviewer. Existing plugin image/structured-output
support is also worth exposing through separate Flywheel contracts.
[App-server][codex-app-server] · [Bundled plugin][codex-plugin]

### 3.4 Claude: distinguish subagents, Workflows, Teams, and remote review

Ordinary subagents and custom agent definitions are the smallest integration
unit. A parent running with bypassed permissions cannot be made safe merely by
placing a more restrictive permissionMode on a child. Flywheel's current Claude
bridge contract needs an enforceable tool/profile design before any
mixed writer/read-only claim. [Claude subagents][claude-subagents]

The current machine reads native user/project/local settings without exposing
all resulting agents, tools, Team/Workflow policy or child facts as a checked
contract. It must not silently fill a role that requires read-only execution
or expand the user's permissions to make a new feature work.
[Current bridge][fw-claude-bridge] · [Optional plugin][claude-plugin]

Dynamic workflows move a bounded orchestration into a script. Official docs
support headless execution and describe default workflow concurrency up to 16,
potentially fewer with fewer available CPUs; there is also a 1,000-agent total
per run. Versioned native controls must be
checked, including `CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS` from 2.1.269.
Ordinary Agent concurrency and workflow concurrency are different limits.
Enable them together only when their combined maximum is reserved and
enforced, or make the profiles mutually exclusive. [Workflows][claude-workflows]

Probe `ultracode` as well: official documentation says it bypasses the session's
concurrent subagent limit for the ordinary Agent tool. That does not establish
that the workflow runtime's own cap is bypassed. A bounded profile must exclude
modes that defeat its required limit. When an organization's `availableModels`
blocks a requested workflow-agent model, native substitution may also occur;
rule out unauthorized substitution before dispatch and reject fixed-model
profiles when that cannot be proved. Record the actual binding instead of
treating the requested name as execution evidence.
[Workflows][claude-workflows]

Teams are experimental peer sessions; teammates cannot be assumed to resume
with their parent. [Agent Teams][claude-teams]

Remote ultrareview needs a separate job protocol. `claude -p '/code-review ultra'`
does not wait for findings; the dedicated `claude ultrareview` command does.
Ending the local wait may leave cloud work running, and rerunning starts a new
review. Any adapter must track account eligibility, remote identity, candidate,
status and cancellation. Posting is off by default and requires its own
authorization; account usage-credit conditions are not implied by a local
subscription. [Ultrareview][claude-ultrareview]

Advisor is another optional, account/model-dependent consultation mechanism.
Investigate its attribution and relationship to JEV/second opinions separately;
do not enable it implicitly with a collaboration profile.
[Advisor][claude-advisor]

## 4. Positive uses by loop

| Loop or workflow | Useful native work | Required ownership and evidence |
| --- | --- | --- |
| Host preparation | Native exploration of the request, constraints and available evidence | Record provenance and preparation activity; do not pretend all prior host consumption was measured by the engine |
| Intake / ordinary Quick | Structured output, efficient tools, concise context | Prefer one agent; collaboration startup and aggregation must earn their cost. Quick still has independent final Review |
| Full Plan | Read-only helpers map architecture, requirements and alternatives in parallel | Parent produces the proposed contract; Flywheel validates and freezes it. Native Plan mode is not the outer Plan loop |
| Execute | Read-only helpers locate references, check interfaces and suggest tests while one writer implements | Call it single-writer only after per-child restrictions are enforced; otherwise use separate read-only calls or leave helpers off |
| Review | Independent lenses inspect security, compatibility, tests and requirements | Reviewer context stays separate from Executor context. A helper inside Execute is not the independent Review |
| Judge / second opinion | Evidence gathering and alternative interpretations of the candidate | Preserve distinct role/session identity; judge the exact verified candidate, not a native summary alone |
| Investigation | Parallel hypotheses over code and saved diagnostics | Distinguish observations from hypotheses; real diagnostic operations retain their existing admission and receipts |
| Garden | Repository surveys, duplicate-pattern checks, documentation and architecture inspection | Treat discovered work as proposals; do not let a native coordinator create and merge unrelated implementation tasks |
| Large homogeneous audit | A bounded Swarm or workflow with one item schema | Enforce item, concurrency and scope limits; aggregate failures and unknown results explicitly |

Model capabilities can inform experiments within each row. For example, visual
input may improve a frontend comparison, and long context may help reconcile
interfaces across modules. Test actual artifact delivery and access first, and
hold model/account/effort fixed when measuring the collaboration benefit.
No role assignment in this proposal asserts a universal model ranking.

## 5. Proposed capability and invocation contracts

These tables are conceptual design fields. They are not accepted configuration,
CLI flags, or a runnable JSON example.

### 5.1 Separate requested policy from observed facts

| Contract area | Proposed facts |
| --- | --- |
| Runtime identity | Harness, adapter, installed binary/package identity, native interface version and capability fingerprint |
| Collaboration selection | Baseline off, small helpers, Team, Swarm or workflow; selected role/profile and content hash |
| Admission support | Verified total/running-child caps, depth control, profile restrictions, native pre-dispatch hooks if any |
| Model and account binding | Parent and permitted child models, effort, provider/account route, and actual bindings |
| Workspace and tools | Root cwd, execution root, actor-specific writable scope, tool allowlist, MCP/tool provenance and enforcement level |
| Observation quality | Full child events; trustworthy aggregate only; or unknown |
| Lifecycle | Dispatch boundary, active actor/task identities, cancellation scope, terminal result, physical/native cleanup, and recovery support |
| Usage | Per-request/per-child identity, aggregation semantics, completeness, deduplication and unknown components |
| Optional media | Artifact hash/path/type, supported input route, actual delivery evidence and retention scope |

P0 must not claim every backend already supports a complete child ledger.
A trustworthy aggregate may be sufficient for a policy that only requires
aggregate cleanup. It cannot satisfy a policy requiring per-child permissions,
per-child model attribution, or exact child recovery. Unknown support fails
admission when the selected policy requires that evidence.

### 5.2 Reserve work before dispatch

For one collaborative invocation, Go should reserve the root plus the maximum
permitted active descendants before starting it. This is valid only when the
native cap is verified for the exact runtime and all enabled spawn paths.
A guessed constant or a prompt asking for two helpers is not a hard cap.

If ordinary subagents and workflows can run simultaneously, add their possible
concurrency or make those profile choices mutually exclusive. Nested spawn
paths and dynamically added tools must not evade the reservation. A backend
without a per-child admission hook may use an upfront bounded reservation;
do not describe that as Go authorizing each child immediately before spawn.
Reserve the whole group before starting its root, avoiding roots that each hold
a slot while waiting indefinitely for helper capacity. For illustration, a
root plus at most two active children reserves three slots; a global six-slot
limit admits at most two such groups. These are examples, not proposed defaults.

PTC/tool I/O slots are separate from model-agent slots. A tool that calls another
model consumes model capacity even if its public name looks like ordinary I/O.
Provider/account limits apply to the combined workload, not each packet alone.

Concurrency limits do not bound cumulative tokens or money. Observed usage
plus cancellation gives a soft threshold with possible in-flight overshoot.
A policy requiring a hard token or monetary ceiling is inadmissible unless
the selected route actually enforces it. Unknown usage remains unknown and
must not become zero or a fabricated cost saving.

### 5.3 Child lifecycle, cancellation, and recovery

Persist the admitted invocation and reserved capacity before dispatch. An engine
root invocation identity is mandatory; associate the native root ID when the
protocol establishes it and otherwise preserve the uncertainty. Where supported,
associate native child IDs with that root call, task, packet, role, profile and
exact workspace; never invent identifiers for aggregate-only observations.
Record actual creation, dispatch, terminal outcome
and cleanup separately; a summary such as “all done” is not the ledger.

Stop must cover the root, queued work, running children, native background jobs,
and owned processes. A parent's successful exit cannot prove its independent
children have stopped. Release reserved capacity only at the proved boundary.
An unknown result or cleanup state blocks retries, fallback and replacement
workers until reconciliation resolves the original responsibility.

Kimi Server illustrates why adapter detail matters: prompt idempotency is
in-memory, limited to 256 records per session and lost on restart; aborting the
main turn does not empty the queue; task.stop acts on specific asynchronous work. Session
usage placeholders and live context-size events are not billable usage.
Use exact turn/step identities and verified child aggregation, and test
reconnect/replay behavior. [Session endpoints][kimi-session-api] ·
[Conversation and task protocol][kimi-conversation]

Deduplicate by native request identity. Use either a trustworthy root aggregate
that includes children or deduplicated per-child totals, never both added
together. Historical totals, current context size, cost estimates and actual
billing are different facts. For cancellation, close new admission, withdraw
queued input, stop known native work, confirm terminal state and actual cleanup,
then save the receipt. A local process-group stop cannot prove a remote job has
ended.

Session reuse binds role, directory, model/account, permissions and profile.
An old board, persisted mode or restored child does not acquire a new call's
authorization automatically. A changed restriction needs compatible restoration
evidence or a new scope; resume must not bypass it.

### 5.4 Model, account, and permission boundaries

Keep children on the parent's approved model/account by default. A native
secondary-model pool or role alias is not permission to choose another provider,
paid route, or entitlement. An explicitly allowed alternative requires actual
binding evidence and remains inside the same task budget.

A tool allowlist and an OS sandbox are distinct controls. For read-only helpers,
verify every enabled execution route, including shell, filesystem APIs, MCP,
plugins and resumed children. If the runtime cannot enforce the required
boundary, select a whole-call read-only profile or a separate outer worker.
Do not label a writable inherited sandbox read-only because of a role name.

Hooks may provide valuable observations, but Kimi hooks explicitly fail open
on errors and timeouts. Such hooks cannot be the sole barrier for budgets,
merge control or authorization. [Hooks][kimi-hooks]

## 6. Selection, fallback, JEV, and user controls

| Policy | Proposed meaning |
| --- | --- |
| Off | Request the tested baseline with native collaboration explicitly disabled; verify effective state rather than inherit an unknown user/runtime default |
| Prefer | Use a capability when all required facts are established; otherwise use the declared baseline only before any possible task-related dispatch and after cleanup is confirmed |
| Required | Refuse before dispatch when the requested capability, enforcement or evidence is unavailable |

A startup error is not automatically a safe fallback point. Startup hooks,
session restoration, initialization and resumed background work can have side
effects before the first visible model answer. Prefer fallback requires positive
evidence of no task-related dispatch and cleanup confirmation. After possible
task-related dispatch or an unknown state, reconcile the original invocation;
do not cold-start another.
The baseline must satisfy the same required constraints. Record the selection
and fallback decision once in the invocation journal; SDK, adapter and Go must
not each independently retry the work.

Completed managed probes that are proven to perform no inference are recorded
separately; their existence is not itself task dispatch. Their completion and
lack of task effects must be established, not inferred from a startup error.

JEV may recommend among the eligible profiles after deterministic admission has
filtered them. It cannot increase the tool permissions, spawn limit, provider
allowlist, budget or workspace scope. With JEV disabled, a static role profile
can still use a supported native capability. Native capability controls must
not silently depend on turning the entire JEV route on.

Keep the existing single JEV switch; do not add a second shadow switch.
`scheduler.dispatch` still controls formal packet scheduling and
`scheduler.prewarm` controls eligible DSH initialization. Neither is a native
child switch. Existing L1/L2/L3 checks, protected paths, candidate binding,
independent Review and Git delivery remain the engine's responsibility.

The UI should distinguish requested profile, effective profile and observed
execution. Show “unavailable”, “baseline selected”, “running with helpers” and
“cleanup unresolved” from facts. Avoid presenting provider brand names as a
generic speed switch or showing an unchecked “single writer” badge.
Use role/stage profiles with advanced limits kept in the chosen profile, rather
than duplicating semantically different booleans for every provider. Retrieval,
PTC, media and remote services each retain their own admission conditions.

## 7. Staged implementation proposal

| Stage | Work | Exit condition |
| --- | --- | --- |
| P0: contracts and probes | Capability facts, invocation reservations, explicit baseline off, observation-quality levels, version/scope binding, known-before-dispatch fallback; extend existing machine interfaces | Offline probes demonstrate supported and unsupported cases without pretending a complete child ledger exists everywhere |
| P1a: low-coordination capabilities | DSH PTC/ordinary retrieval and structured results; a separate input manifest for media | Tool scope, failure, result and usage contracts pass; model results still satisfy the existing schema |
| P1b: small collaborations | Restricted DSH local helpers first, then Kimi Agent, Codex and Claude as their contracts allow | Child caps, permissions, results, accounting and cancellation are provable; combinations that cannot enforce read-only scope are not admitted |
| P2: bounded native collaboration | DSH Team, Kimi Server Swarm, Claude Workflows as separate adapter/profile increments | Each route proves its own caps, lifecycle, actual model/account bindings and complete required observations |
| P3: optional advanced work | Native Teams, additional compaction policy, multiwriter isolation, isolated Tower, and remote review | Dedicated acceptance proves ownership, crash recovery, artifact/candidate identity and a benefit over a simpler profile |

P0 capability discovery should inspect interfaces and perform non-inference
checks. A prewarm or capability probe must not create a formal task Session,
spawn children, or submit a model prompt merely to determine availability.
Start with the shared fact contract and DSH route; P0 does not require rewriting
all providers before any increment is useful. Unsupported combinations stay
explicitly unsupported.

Prefer extending the user's existing subscribed plugin/CLI routes. Kimi Server
is a proposed optional route alongside print, not a covert ACP migration.
Codex and Claude changes should first improve their public machine adapters.
A Team-capable DSH desktop does not make every SDK profile and older supported
version compatible.

For Tower, investigate an isolated repository copy with a fixed input commit
and a returned candidate commit/patch. Its internal coordination would be
contained in that work unit, while Flywheel independently validates and accepts
the result. This needs actual isolation and recovery engineering; a prompt
saying “do not merge the main repository” is not that boundary.
A normal worktree shares Git metadata and is not proof that Tower cannot change
formal branches. Keep the experiment away from Flywheel's authoritative state,
acceptance files and delivery credentials. If upstream later adds an enforceable
external merge gate, assess that as a distinct integration.

## 8. Validation and research limits

### 8.1 Required failure acceptance

| Area | Minimum cases before a capability is eligible |
| --- | --- |
| Availability | Missing adapter, older binary, mismatched capability fingerprint, unknown config field, and incompatible resumed profile |
| Dispatch and fallback | Failure before dispatch; initialization side effect; lost dispatch acknowledgement; startup hook; native child created before root progress |
| Limits | Extra child, recursive child, ordinary/workflow combined fan-out, runtime-ignored cap, model-calling tool, and account-level saturation |
| Scope | Child writes outside scope, permission inheritance, MCP escape route, late-created child and resumed child with an old allowlist |
| Outcome and cleanup | Partial child failure, root exit while a child runs, stop during fan-out, queued work after abort, process kill and uncertain physical cleanup |
| Recovery | Reconnect, duplicate/reordered events, server restart, forgotten idempotency record, changed profile and unresolved previous work |
| Usage | Root/child double counting, cumulative versus delta usage, retry attempts, missing child usage and placeholder zero values |
| Verification | Native self-review disagrees with independent Review; candidate changes after evidence; no acceptance bypass through native merge |
| Media | Unsupported modality, missing file, changed artifact hash, mistaken reference path and output that claims an unseen artifact was inspected |

Use real SDK/CLI protocol fixtures and fault injection, followed by target
macOS and subscription-route acceptance. Implementation changes retain the
repository's 100% statement coverage requirement for each measured Go function
and the applicable Node, bridge and race gates. These are future implementation
requirements; this documentation research did not execute them.

### 8.2 Paired live experiments after engineering acceptance

Compare the same repository HEAD, task contract/oracle, model, account route,
effort, permissions, artifacts, versions and total resource conditions with a
verified single-agent baseline, tool parallelism, small helpers and native
workflow/Swarm profiles. Include small Quick tasks, repository exploration,
frontend repair with screenshots, repeated audits, planted-defect reviews,
multi-hypothesis investigation and interrupted long sessions. Interleave runs
and record all failures/retries, not just successful examples.

Measure final acceptance and first-pass rates, missed/spurious findings,
end-to-end p50/p95 time, time to first useful evidence, model/tool concurrency,
known token/cache usage, unknown usage, quota pressure, repair rounds,
independent review defects, cancellation latency and leftover actors.
Include startup and aggregation time; do not report only the fastest child.
Record failures and quality changes alongside speed.

Run on the user's target macOS desktop as well as the relevant offline
environment. Do not claim that long context, a larger team, a more expensive
model, or the presence of a native workflow makes a task faster or better before
these comparisons. Native product examples guide the experiment; they are not
Flywheel measurements.

### 8.3 What this research has and has not established

The source review establishes that several useful mechanisms exist, identifies
current adapter gaps, and proposes verifiable admission rules. It does not
certify the user's installed native versions, account entitlements, specific
model access, native child cleanup under process failure, or performance gains.
No runtime configuration, model default, user profile, subscription route, or
release package is changed by this document.

## 9. Decisions for review and source catalogue

Recommended decisions for the implementation discussion:

1. Accept the three-layer ownership boundary and start with explicitly bounded
   invocation capabilities.
2. Prioritize P0 and P1; keep native collaboration off until its adapter policy
   is verifiable for the selected runtime.
3. Preserve independent Review/Judge and the existing provider/model defaults.
4. Treat Kimi Server, DSH Team and Claude Workflows as distinct integrations,
   each with its own evidence requirements.
5. Defer normal-loop Tower and mixed writable/read-only claims until enforceable
   ownership and actor-level restrictions exist.
6. Decide default enablement only after paired live results, with a usable
   static profile when JEV is off.

### Current repository sources

- [Flywheel baseline][fw-base], [design][fw-design], [model policy][fw-models].
- [Plugin dependency declarations][fw-package] and [vendored source identities][fw-vendor].
- [DSH policy][fw-dsh-policy], [DSH bridge][fw-dsh-bridge], [Kimi driver][fw-kimi],
  [Codex Pro bridge][fw-codex-bridge], [Claude Code Pro bridge][fw-claude-bridge].
- [DSH usage fold][fw-dsh-protocol] and [durable invocation/session lifecycle][fw-sessions].

### Official native sources

- DSH rc.3, pinned to 38a6a5c965aa99ab525161efb7cc5cf1fe62fced:
  [Agent Team][dsh-team], [SubAgent lifecycle][dsh-subagent],
  [SDK Team configuration][dsh-sdk], [PTC executor][dsh-ptc],
  [Web tools][dsh-web], [Plan mode][dsh-plan], [Goal rounds][dsh-goal]
  and [SDK protocol][dsh-protocol].
- Kimi: [release][kimi-release], [changelog][kimi-changelog], [CLI][kimi-cli],
  [Agent profiles][kimi-agents], [tools][kimi-tools], [Tower][kimi-tower],
  [Server API][kimi-api], [session endpoints][kimi-session-api],
  [conversation/task protocol][kimi-conversation], [hooks][kimi-hooks],
  [model specifications][kimi-models] and [official model examples][kimi-model-blog].
- Pinned Kimi source: [print defaults][kimi-print-defaults],
  [print lifecycle][kimi-print], [Swarm mode check][kimi-swarm],
  [Tower merge][kimi-tower-merge], [Tower guard][kimi-tower-guard].
- Codex: [official subagent configuration][codex-subagents],
  [app-server][codex-app-server] and
  [the bundled plugin source][codex-plugin].
- Claude: [ordinary subagents][claude-subagents],
  [dynamic workflows][claude-workflows], [Agent Teams][claude-teams],
  [ultrareview][claude-ultrareview], [Advisor][claude-advisor] and
  [the optional plugin source][claude-plugin].

[fw-base]: https://github.com/ckanner/flywheel/commit/63d8781ee1ece0cce443fa2e1ead62d1d6288817
[fw-design]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/docs/design.md
[fw-models]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/docs/en/model-policy.md
[fw-package]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/package.json
[fw-vendor]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/vendor/README.md
[fw-dsh-policy]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/policy.js
[fw-dsh-bridge]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/bridge.js
[fw-dsh-protocol]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/protocol.js
[fw-kimi]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/internal/drivers/kimi.go
[fw-sessions]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/internal/machine/sessions.go
[fw-codex-bridge]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/codex-pro-bridge.js
[fw-claude-bridge]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/claude-code-pro-bridge.js
[dsh-team]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/team/agent-team/README.md
[dsh-subagent]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/subagent/subagent/README.md
[dsh-sdk]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/docs/user/guide/python-sdk.md#enable-team-mode
[dsh-ptc]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/core/tool-executor-ptc/README.md
[dsh-web]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/web/tool-web/README.md
[dsh-plan]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/plan/plan-mode/README.md
[dsh-goal]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/goal/goal-round-driver/README.md
[dsh-protocol]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/sdk/protocol/README.md
[kimi-release]: https://github.com/MoonshotAI/kimi-code/releases/tag/%40moonshot-ai/kimi-code%402.6.2
[kimi-legacy]: https://github.com/MoonshotAI/kimi-cli
[kimi-changelog]: https://www.kimi.com/code/docs/en/kimi-code-cli/changelog.html
[kimi-cli]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/kimi-command.md
[kimi-agents]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/customization/agents.md
[kimi-tools]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/tools.md
[kimi-tower]: https://www.kimi.com/code/docs/en/kimi-code-cli/guides/tower.html
[kimi-api]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api.md
[kimi-session-api]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-sessions.md
[kimi-conversation]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-conversation.md
[kimi-hooks]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/customization/hooks.md
[kimi-models]: https://www.kimi.com/code/docs/en/kimi-code/models.html
[kimi-model-blog]: https://www.kimi.com/en/blog/kimi-k3-1-pro
[kimi-print-defaults]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/agent-core-v2/src/agent/task/printDefaults.ts
[kimi-print]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/apps/kimi-code/src/cli/v2/run-v2-print.ts
[kimi-swarm]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/agent-core-v2/src/features/swarm/tools/agent-swarm/agentSwarmTool.ts
[kimi-tower-merge]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/agent-core-v2/src/features/tower/tools/merge/mergeTool.ts
[kimi-tower-guard]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/agent-core-v2/src/features/tower/bashGuard.ts
[codex-subagents]: https://learn.chatgpt.com/docs/agent-configuration/subagents
[codex-app-server]: https://learn.chatgpt.com/docs/app-server
[codex-plugin]: https://github.com/ckanner/dsh-subagent-codex-pro/tree/37da3d75de32b809c4fb1a631ccc15191f3d87f9
[claude-subagents]: https://code.claude.com/docs/en/sub-agents
[claude-workflows]: https://code.claude.com/docs/en/workflows
[claude-teams]: https://code.claude.com/docs/en/agent-teams
[claude-ultrareview]: https://code.claude.com/docs/en/ultrareview
[claude-advisor]: https://code.claude.com/docs/en/advisor
[claude-plugin]: https://github.com/ckanner/dsh-subagent-claude-code-pro/tree/ec0b99176b67512f3ca1838ec2a349dcb0189214
