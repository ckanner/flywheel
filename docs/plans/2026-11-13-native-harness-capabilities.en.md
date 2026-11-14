# Native harness, PTC and LCU integration within Flywheel loops

[简体中文](2026-11-13-native-harness-capabilities.md) · [Plan index](README.md) · [System design](../design.md)

**Status:** discussion proposal; no runtime implementation or default change.
**Created:** 2026-11-13. **Fully revised:** 2026-11-14, America/New_York.
**Flywheel baseline:** source [ac1ed320][fw-base], package declaration **0.33.0**.
Evidence is pinned source and official documentation. Installed versions,
account access, target Mac behavior and performance still require acceptance.
The 2026-11-14 revision rechecks Flywheel, DSH release/master composition,
presets, LCU, Kimi source identity, both Pro plugin heads and current Claude
concurrency/restoration restrictions. Source review does not certify installed
binaries, npm publication or the user's machine.

## 1. Decisions proposed

Use each harness's useful capabilities inside a bounded Flywheel invocation.
Go continues to own contracts, packet scheduling, resources, verification,
recovery and delivery. Native tasks, boards and helper opinions remain inputs
to the enclosing role; they do not become formal packets or independent Review.

Start with a common capability contract and managed DSH PTC. Add small helpers
when their permissions, limits and cleanup can be verified. Treat LCU as a
separate desktop capability requiring explicit attachment, authorization,
exclusive GUI ownership and artifact evidence. Develop Team/Swarm/Workflows
through distinct adapter contracts; keep Tower and remote review separate.

Preserve existing models and subscription paths. A DSH route using GPT is a DSH
agent; Codex Pro runs Codex app-server, and Claude Pro runs Claude Code. LCU is
currently a DSH integration. Do not wrap the existing Kimi frontend route in a
DSH parent or change its model merely to obtain computer use. Visual work needs
an explicitly eligible DSH binding with verified image input. [Model policy][fw-models]

## 2. Source baseline and changed findings

| Component | Inspected identity | Meaning |
| --- | --- | --- |
| Flywheel | 0.33.0 / `ac1ed320c68f6a8a466005985264e65b5a72fad9` | One complete `@ckanner/flywheel` package; formal runtime contracts remain unchanged from 0.32.1 |
| Preset generator | 0.3.0 / `f692cb07abbb9786444c183d74eee018b1a2f0e8` | Both `daily` and `heavy` use PTC; their generated plugin lists are identical |
| LCU | 0.4.2 / `9efeded13d20b41a96621bb53d903954f68550e6` | Per-Agent `cua` bridge; Config and bundle defaults allow both presets |
| DSH release | 0.2.2-rc.3; source `38a6a5c965aa99ab525161efb7cc5cf1fe62fced` | Standard SDK composition and the older PTC output contract |
| DSH latest master | `39163dd30f703a79079e2d2446545cff3c453738` | New inspect/Image and private program-tool contracts despite the same PTC package version; see §5.C |
| Codex Pro | Latest main `aaf465901def5aa1870c1165b9714d32294337a4`, 0.9.0; bundled source `37da3d75de32b809c4fb1a631ccc15191f3d87f9` | Production source unchanged between these identities; later changes are release documentation and test cleanup |
| Claude Code Pro | Latest main `5eb637a9e82b317e67e50846005c407c54f1cea0`, 0.4.0; Flywheel source `ec0b99176b67512f3ca1838ec2a349dcb0189214` | Production source unchanged; later changes are release documentation and CI; still optional |
| Kimi | Flywheel driver documents 2.4.1; inspected upstream `f082f04a093a3e71a2263c5461d52507dc31f83a`; release API 2.6.2, web changelog 2.6.0 | Distinct source/documentation observations, not certification of the installed CLI |

Sources: [package][fw-package], [adapter provenance][fw-vendor],
[generator][user-preset-source], [LCU configuration][lcu-index],
[LCU bundle][lcu-bundle], [Kimi release][kimi-release], [changelog][kimi-changelog].
Plugin comparisons: [Codex][codex-latest-compare] and [Claude][claude-latest-compare].
Keep the exact pinned artifact; a matching version does not authorize replacing its bytes.

The generator now handles both the older `tool-presentation` row and concrete
executor rows. The earlier missing-row finding is superseded at this revision.
`daily` and `heavy` differ in identity, labels and metadata, not plugin contents.
LCU availability depends on actual Agent binding and plugin configuration,
not the word “heavy.” The generator reads the installed application's PTC base;
its source revision alone does not identify the user's materialized preset.
[Generator][user-preset-source]

Flywheel now ships Host, Client, workers and four platform engines together;
`plugins/dsh` is a private build/test directory. Worker exports use
`@ckanner/flywheel`. Package migration did not add preset, PTC or LCU controls,
change model/session defaults, or establish registry publication or live
acceptance. [Distribution contract][fw-design] · [Complete package][fw-root-package]

## 3. Desktop preparation and formal runtime

Keep four responsibilities distinct: a **model** performs inference;
**provider/account** selects access and credentials; a **harness** runs tools,
context and native collaboration; an **adapter** exposes them through Flywheel's
invocation and receipt contracts. GPT within DSH does not acquire Codex's agent
features, and PTC tool concurrency does not create additional model agents.

| Layer | Current responsibility | Proposed extension |
| --- | --- | --- |
| DSH host conversation | Discuss the goal and prepare contracts with its current model, tools and preset | Record relevant evidence and provenance; do not claim all prior host consumption was engine-metered |
| Go engine | Choose configured bindings, admit invocations, validate results and deliver the candidate | Capability eligibility, resource reservations, GUI ownership and recovery |
| Native worker/adapter | Run a bounded native session and report observed outcome/cleanup | Checked composition, child and tool facts, effective capability receipts |
| Native agents/tools | Execute the admitted subproblem | Only the capabilities and resources granted to their actor |

The Task skill uses the current conversation for preparation; `loops.plan`
controls the later engine stage. The desktop plugin passes cwd and a host
Session association to Go. Formal DSH loops start a separate local
`dsh --profile sdk --patch …` and use SDK stdio JSON-RPC. This can reuse the
existing account store and scoped role history without inheriting the desktop
Agent's preset or conversation. [Task skill][fw-task-skill] ·
[Host entry][fw-host-entry] · [Worker][fw-dsh-bridge]

Three names must remain distinct: a desktop **Agent Preset** is an Agent plugin
composition; a DSH **runtime profile** such as `sdk` loads services; Flywheel
**model profiles** reuse harness/provider/model/channel/effort bindings.
`FLYWHEEL_DSH_PROFILE` selects an SDK-compatible runtime and rejects `desktop`.
It does not select `heavy` as an Agent Preset. [Worker][fw-dsh-bridge]

Standard rc.3 SDK uses a native executor and global composition without preset
mounting. The official PTC overlay switches executors; current Flywheel does
not apply that switch. Its request schema has no preset/PTC/LCU fields.
Customized profiles require inspection of effective state.
[SDK defaults][dsh-sdk-app] · [SDK creation][dsh-sdk-server] ·
[PTC overlay][dsh-ptc-overlay] · [Worker protocol][fw-dsh-protocol]

The latest inspected master still creates SDK Agents without preset composition;
its public session/initialize protocol adds no named-preset selector.
[Master SDK creation][dsh-sdk-master]

## 4. Common capability contract

The following are proposed facts, not accepted configuration fields or CLI flags.

| Area | Required distinction |
| --- | --- |
| Identity | Installed harness/adapter/version; runtime profile; preset/composition source and digest; actual tool executor, exports and output/image contract |
| Request and result | Requested policy, eligible policy, effective capabilities and observed use |
| Model/account | Allowed parent/child bindings, effort, subscription route and actual model; modality verified separately |
| Resources | Model agents, tool/I/O concurrency, GUI ownership and remote jobs are different resource classes |
| Authority | Role, directories, permitted tools/methods, applications/origins, enforcement level and authorization source |
| Evidence | Invocation/root/child identities where available; dispatch, outcome, usage completeness, cleanup and artifact identity |
| Restoration | Profile revision/digest, native mode/board, connection epoch and compatibility of resumed actors |

A trustworthy aggregate may satisfy an aggregate-only policy, but cannot prove
per-child permissions or exact child recovery. Missing facts remain unknown.
Current session identities hash runtime references such as a profile name;
new effective composition, capability and GUI identities must enter session
reuse and prewarm bindings. [Session lifecycle][fw-sessions] ·
[Runtime identity][fw-runtime-identity]

## 5. DSH composition: two implementation paths

### A. Managed SDK global composition first

Use the official PTC overlay and a managed SDK composition derived from the
existing generator policy and identified application base. Preserve SDK RPC,
permissions, accounting and lifecycle services. Record the resulting digest and
verify its executor/tools before dispatch. Keep desktop configuration unchanged.
This is an implementation path using existing composition mechanisms, not a
capability already selected by Flywheel. [PTC overlay][dsh-ptc-overlay] ·
[Generator][user-preset-source]

Initially enable efficient tools without incidental Goal, workflow, delegation
or computer use. Ordinary retrieval and model-backed tools need separate
admission. Current Flywheel rejects unrelated Agents and child dispatch and
folds root events; helpers require deliberate extensions to those contracts.
[Policy][fw-dsh-policy] · [Bridge][fw-dsh-bridge]

Formal LCU also requires a new **managed attachment gate in the LCU plugin**.
It must bind a real engine invocation, native root, capability decision and GUI
lease, while retaining the existing desktop preset guard. Loading LCU into a
global SDK composition alone does not make its preset gate pass. Do not forge
`session.header.agentPreset`, weaken the desktop guard, or pretend a preset-free
SDK Agent is a desktop heavy Agent. [LCU attachment][lcu-index]

### B. Public preset composition when the SDK supports it

DSH exposes `agentPresets.mount(ctx, id)` for an unpublished Agent during setup;
children can join the exact revision retained by their parent. The inspected
SDK does not expose that creation seam. A future supported SDK integration
could mount a named preset and replace part of path A; it should use public
interfaces without a private upstream fork. [Preset registry][dsh-preset-registry]

Test creation, child composition, disposal and resume independently. A live
Agent retains a revision, but restoration by preset ID can select its current
definition after restart. Reject resume when the recorded digest does not match. A new composition
requires a separately admitted rebuild after the old responsibility is settled.
An ID alone is insufficient. Neither A nor B currently
provides Flywheel with a ready-made `preset=heavy` request.

### C. Probe the PTC contract, not just its package version

The release tag and latest master both declare PTC package version 0.2.2-rc.3,
but only master exports `/run-code`, and their output/image contracts differ.
Fingerprint actual bundle/module content, exports and schemas, then use
non-mutating protocol fixtures; semver alone cannot select a compatible path.
[Release package][dsh-ptc-release-package] · [Master package][dsh-ptc-master-package]

| Composition | Source observation | Admission requirement |
| --- | --- | --- |
| rc.3 release | Older program output and deferred image context | Match its runtime instructions and result semantics |
| Latest master | TypeScript; inspect selects output, while function return/final expression values are omitted; read_image/cua images become Image values shown with inspect | Verify text, images, errors and cancellation against actual runtime behavior |
| Master with LCU 0.4.2 | The source interaction below is a compatibility risk, not a tested failure | Resolve value/image preservation before admitting GUI use |

Master's transport treats a tool named `cua` specially: one projected image
replaces its structured result with PtcImage; multiple images fail. LCU can
return a handle with initial UI imagery, and its proxy needs that handle to
construct methods. This combination may lose the handle. Copying Image as an
ordinary object in the proxy also needs verification. This is a **source-derived
risk, not a reproduced Mac failure**. Existing PTC support alone does not prove
compatibility. [Transport][dsh-ptc-master-source] · [LCU value/projection][lcu-ptc] ·
[Proxy decoding][lcu-bridge]

Master also offers `createRunCodeTool` over a private tool list. It is a useful
optional extension for a bounded program interface, but shares the same
transport and does not solve the image issue or replace budgets/GUI leases.
Keep the source-inspected global overlay as the first path, without requiring every
user to upgrade. Test handle/scalar plus images, zero/one/multiple images,
policy-removed images, inspect on success/failure, and completed effects after
cancellation. [Factory][dsh-run-code-factory] · [Current contract][dsh-ptc-master]

## 6. LCU: tool path, authority and physical resource

### 6.1 The PTC bridge and screenshots

LCU registers one model-facing tool, `cua`. In `run_code`, `connectCua` builds
Proxies whose method calls pass through generated `tools.cua` bindings and the
guarded tool pipeline; LCU translates them into its internal MCP `js` runtime.
The internal `js` tool is not the model-facing API. Each Agent owns a connection
and REPL. Current `hN` handles identify REPL-local objects; they carry no
generation token or enforced epoch. Epoch binding is part of this proposal.
[PTC tool][lcu-ptc] · [Proxy][lcu-bridge] · [Attachment lifecycle][lcu-index]

The current runtime requires Apple Silicon macOS, local ChatGPT.app with its
bundled Node/CUA runtime, and the Sky helper. Browser control also needs the
official extension and desktop native messaging host. LCU does not add a
Codex/ChatGPT model login or separate Python-wrapper requirement; model
authentication and the local GUI runtime are separate dependencies. Verify
Screen Recording/Accessibility permissions for the actual responsible process
and test SDK launches on the user's Mac. [Runtime requirements][lcu-readme]

Screenshots already have an image projection: LCU checks the routed model's
image modality and the attachment service before returning DSH image blocks.
A text-only outer worker request does not mean the Agent cannot receive images
from its own tools. A missing modality/store produces diagnostics, not proof
that the image was seen. Track capture success, image delivery to the actual
model, and archival as run evidence as three separate facts; these are proposed
receipt fields. [Image projection][lcu-tool]

Also distinguish the release's deferred image path from master's Image/inspect
path in §5.C. Producing an LCU image block does not prove that downstream
transport and proxy processing delivered it intact to the model.

Cross-loop transfer needs a separate frozen artifact manifest: candidate or
build identity, content hash, media type, permitted path/store, capture time,
app/origin/target and viewport, connection epoch, actual model route and delivery
evidence. Read/copy external host paths only within permitted scope and verify
the hash; do not trust a model-supplied path as artifact provenance. Another
Reviewer cannot reuse an Executor's live handle or private context. Start by
letting an independent Reviewer inspect frozen artifacts through a verified
image-capable route.

### 6.2 Admission and permission

LCU checks preset eligibility when attaching; changing the preset is not a
per-call revocation mechanism. Both Config and bundle defaults include `daily`
and `heavy`. Attachment failure currently logs a warning and lets the Agent
continue. Required GUI therefore needs actual tool, connection and approval
capability receipts before task dispatch. Managed use needs explicit revocation
and invocation-bound checks, with cleanup confirmed before revocation completes.
[Attachment/configuration][lcu-index] · [Bundle][lcu-bundle]

The current approval path cancels immediately without `userQuestions`, even
when app/origin allowlists exist. With that service, an exact permitted app or
origin may be accepted automatically; other requests need the actual answer.
The native-app branch checks the agent-host guard before its app allowlist;
browser origins follow their separate exact-origin checks.
[Approvals][lcu-index] · [Host guard][lcu-host-guard]

The standard rc.3 base already mounts `userQuestions`. SDK question delivery
requires `initialize.capabilities.userQuestions=true`; current Flywheel sends
no such capability and implements no host question/answer relay. Exact
preauthorized targets can still pass LCU's check when the service is present;
requests needing a person require the missing forwarding path. Service
availability and working interactive delivery are separate facts.
[Base composition][dsh-base] · [SDK server][dsh-sdk-server] · [Bridge][fw-dsh-bridge]

Preserve precise existing authorization without asking again each turn. A host
relay or explicitly designed preauthorization channel must retain real answers;
models cannot answer for the user, and unknown/cancel cannot become allow.
Flywheel full access, `approval=never` and repository sandbox settings grant no
independent GUI access. Preserve the agent-host guard; never enable its testing
override to satisfy a model request.

Bind each question/answer to run, invocation, root, lease/runtime epoch, exact
app/origin and native question ID. After cancellation, owner/runtime replacement
or target changes, late answers cannot authorize new work. Reuse existing grants
within their original scope and record authorization separately from execution.

Proposed GUI modes are **disabled**, **observe** and **interact**. Current `cua`
can invoke mutating methods and is not intrinsically read-only. “Observe” needs
real method-level restrictions through every handle and dynamic call path;
otherwise the policy is unsupported. “Interact” additionally binds authorized
applications/origins and allowed effects. Neither mode expands delivery or
account authorization.

### 6.3 Exclusive GUI ownership

Current packet scheduling checks `schedule.after`, nonoverlapping `writes`
and channel/model slots. Worktrees do not isolate a display. LCU's `cua` lacks
`isConcurrencySafe`, so its calls are exclusive within each `run_code` program;
that is not a lock across programs, Agents, processes or runs.
[Scheduler][fw-parallel] · [Packet schema][fw-packet-schema] · [LCU tool][lcu-ptc]

For the first implementation, give one root GUI invocation an exclusive lease
on the real host/display/desktop session. Disable GUI for its children; keep
ordinary file tasks eligible for parallel execution. Current LCU attaches by
Agent/preset rather than imposing a blanket child ban; root-only GUI is our
proposed managed policy. All serial/parallel, Quick and manual-stage paths use
the same admission gate and a shared coordinator across participating
runs/processes, not only `parallelGroup`'s local mutex. Obtain GUI capacity
before holding model slots, or atomically reserve the whole resource group.
If model capacity is unavailable and nothing has been dispatched, release the
temporary GUI reservation and requeue instead of holding the desktop idle.

Persist owner, task/role/invocation, target and lease epoch before GUI dispatch.
Check ownership at the tool boundary; release only after confirmed input stop
and cleanup. TTL expiry or a returned process is not proof that control ended.
The lease coordinates participating clients; it does not isolate the user or
unrelated software. Detect changed UI state, pause or re-observe, and invalidate
stale coordinates and handles before further actions.

### 6.4 Stop, generation changes and restoration

Current request abort removes local pending work and rejects its promise without
sending a remote cancellation notification. Agent disposal invokes
`void connection.close()`; `close()` waits after escalation to SIGKILL but does
not validate that final exit result. Existing per-turn/application cleanup is
useful, but these paths do not prove complete physical cleanup for Flywheel.
Add awaitable stop/drain and correlated cleanup receipts before eligibility.
`cua.stopApp` stops a held application for the current turn; it is not global
cleanup or revocation of user authorization.
[Connection][lcu-connection] · [Agent disposal/turn cleanup][lcu-index]

LCU automatically rebuilds after application-generation changes, connecting
the replacement before closing the old connection. REPL objects are not
restored; an old `hN` can be invalid or reused in a new epoch. A managed route
must disable transparent replacement or enforce epoch checks, retire and drain
the old owner before admitting the replacement, reacquire state, and reject
old handles. Unknown cleanup keeps the lease blocked. Reconnecting alone does
not restore screenshot validity, UI state or authorization.
[Managed session][lcu-session]

## 7. Other native capabilities and adapter gaps

| Capability | Useful work | Current gap / smallest credible path |
| --- | --- | --- |
| DSH PTC | Filtering results and parallel-safe I/O | Managed executor composition and tool/resource verification |
| DSH SubAgent / Team | Exploration, hypotheses, specialist task contributions | Extend root-only policy and receipts; Team board stays inside one invocation |
| Kimi Agent | Coder/explore/plan helpers with separate contexts | Explicit agent profile, restoration identity and child accounting |
| Kimi Server Swarm | One template over independent items | Separate owned/authenticated Server adapter; no supported print enabling flag |
| Codex subagents / native review | Parallel investigation and focused review | Extend existing Codex Pro machine contract; verify inherited permissions and child facts |
| Claude subagents / Workflows | Specialist contexts and repeatable scripted audits | Extend existing optional machine contract, enforce caps and actual bindings |
| Claude Teams | Peer discussion and shared task coordination | Interactive teammates; unavailable through the current print machine, requiring a separate observable adapter |
| Tower / remote review | Isolated work-unit experiment / additional opinion | Dedicated repository or remote-job ownership and independent acceptance |

### DSH Team and helpers

rc.3 supports fresh/forked local subagents, messaging and continuation; its
shared tree pool defaults to eight active children. The Host delegation-depth
default is one, but explicit tool depths and direct service requests can
override it; admission must cover those paths. PTC's parallel-safe tool cap
defaults to ten. These are distinct resource classes, not Flywheel defaults.
SDK Team can use the standard `sdk` profile with
`agent-team.config.defaultEnabled=true`; `sdk-minimal` omits it. That default
is saved for new roots, not retroactively applied on resume. An internal service
method is not a public `team/setEnabled` RPC. [SubAgent][dsh-subagent] ·
[PTC][dsh-ptc] · [SDK Team][dsh-sdk] · [Protocol][dsh-protocol]

Team members share a checkout; `writeScopes` warns rather than enforcing a
filesystem boundary. Turning Team off does not cancel children or erase its
board. `session/wait` joins progressing work, while parked children can remain;
final drain is still required. External Codex/Claude/ACP delegates do not inherit
local continuation semantics. Native Plan guides behavior, and Goal round
allowances do not establish isolation or cumulative spend limits.
[Team][dsh-team] · [Plan][dsh-plan] · [Goal][dsh-goal]

### Kimi: ordinary Agent, then Server Swarm

Custom profiles have tool/subagent allowlists, checked again before execution.
`--agent-file` is creation-only and incompatible with `--session/--continue`;
resume restores the original profile, and an existing child can be resumed
outside the subagent-type allowlist. Current Flywheel preallocates a `call-*`
invocation ID, not a native session ID: it learns the latter from terminal
`session.resume_hint`. Add profile selection at creation and bind its digest
on resume. Ordinary Agent may already be available through native settings;
absence of managed controls does not prove it is disabled. `--plan` conflicts
with print `-p`. [Profiles][kimi-agents] · [Print][kimi-print] · [Driver][fw-kimi]

Swarm cannot be enabled by the model or a supported current print flag. Beta
Server exposes `swarm_mode` in session profiles/prompts and WebSocket task
events. It is an optional route alongside print, not ACP. Probe installed
OpenAPI/AsyncAPI contracts and own only the intended runtime/sessions. Profile
updates are fieldwise without rollback; inspect partial failure and restored
mode. [Server][kimi-api] · [Endpoints][kimi-session-api] · [Changelog][kimi-changelog]

Prompt idempotency is in-memory, limited to 256 records per session and lost
on restart. Aborting a turn does not empty its queue; asynchronous tasks need
specific stops. Usage placeholders and context-size events are not billable
usage. Hooks fail open on errors/timeouts and cannot be authorization barriers.
[Task protocol][kimi-conversation] · [Endpoints][kimi-session-api] · [Hooks][kimi-hooks]

Concretely, withdraw queued prompts, abort the main turn, stop visible child/
Swarm tasks with WebSocket `task.stop`, and await their terminal state. Side-chat
Agent turns currently lack an equivalent public stop endpoint: exclude them
from managed profiles unless an independently owned runtime can prove complete
shutdown. Main `:abort` alone is not a subtree stop receipt.

Tower owns mission branches, worktrees, reviewers and merges. Its experimental
headless path creates a session, then enables Tower through the profile API;
creation rejects `tower_mode=true` because initialization can mutate Git state.
There is no checked public external-oracle mode. Experiment only in an isolated
repository copy with a fixed input commit and returned candidate; a worktree
shares Git metadata and is insufficient isolation. [Tower][kimi-tower] ·
[Endpoints][kimi-session-api] · [Merge][kimi-tower-merge]

### Codex: preserve the subscribed machine route

Official configuration defaults `agents.enabled` to true and uses
`agents.max_concurrent_threads_per_session` (`agents.max_threads` is legacy).
Children inherit parent sandbox/approval; custom agents cannot override that
boundary. A writable root plus an “explorer” name is not a read-only helper.
Verify a baseline off policy and enforceable permissions before claiming either.
[Subagents][codex-subagents]

Bundled machine 0.9.0 supplies root lifecycle observations, not a complete child
ledger. Extend this plugin rather than changing billing routes or silently
falling back to `codex exec`. App-server offers start/resume/fork, turn
steer/interrupt, collaboration events and `review/start`; generate schemas from
the installed binary. Adapt independent native Review and existing plugin
image/structured-output support separately. An Executor fork is not the
independent final Reviewer. [App-server][codex-app-server] · [Plugin][codex-plugin]

### Claude: separate native mechanisms

The optional 0.4.0 bridge accepts only `danger-full-access`, maps it to native
`bypassPermissions`, and does not expose complete child/tool/workflow policy.
Native user/project/local settings may participate. A child permissionMode
cannot narrow a bypassed parent into an enforced read-only worker; require a
verified profile/tool boundary or separate restricted invocation.
[Bridge][fw-claude-bridge] · [Subagents][claude-subagents] · [Plugin][claude-plugin]

**Workflows** support headless scripted orchestration. Their default concurrency
is up to 16, potentially lower with available CPUs, with 1,000 total agents per
run. Check versioned controls, including
`CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS` from 2.1.269. Ordinary Agent and
Workflow caps are separate; reserve their combined maximum or make profiles
mutually exclusive. `/effort ultracode` bypasses the ordinary Agent concurrent
subagent limit, not demonstrably the Workflow cap. `availableModels` can cause
native model substitution: exclude unauthorized alternatives before dispatch
and record actual models. A requested name is not execution evidence.
[Workflows][claude-workflows]

The ordinary cap also does not block in-session `/subtask` forks or resuming
finished children. Restrict those entry points or prove their additional bound;
setting one environment variable is not a global concurrency guarantee.
Workflow restoration matches calls in launch order: a failure or changed prompt
can replay later completed agents. Bind script/input digests, child attempts/
generations and candidate/effect records; begin with read-only analysis rather
than automatically replaying completed writes. [Subagents][claude-subagents] ·
[Workflow restoration][claude-workflows]

**Agent Teams** require an interactive session to spawn teammates. The current
`claude -p` machine cannot gain support from an environment toggle alone; a
later observable interactive adapter must also handle teammate restoration and
shutdown. **Ultrareview** is remote work: `claude -p '/code-review ultra'` does not await findings, while the dedicated `claude ultrareview` command
does. Ending the local wait may leave cloud work running; retrying creates a
new review. Require remote job identity, candidate, eligibility, usage and
cancel/reconcile semantics; posting needs separate authorization. **Advisor**
is another account/model-dependent mechanism, not an implicit JEV replacement.
[Teams][claude-teams] · [Ultrareview][claude-ultrareview] · [Advisor][claude-advisor]

## 8. Applying capabilities to loops

### Current bindings and initial capability choices

| Stage / route | New-installation template binding | Initial direction |
| --- | --- | --- |
| Plan | Kimi CLI / K3.1 Pro / high | Managed ordinary Agent; explicit Server path only when Swarm is needed |
| Execute default | DSH / DeepSeek Flash / high / deepseek-account | Managed PTC, then small bounded helpers |
| Execute frontend | Kimi CLI / K3.1 Pro / high | Native tools/media; LCU needs a separately configured eligible DSH invocation |
| Execute hard | Codex Pro machine / GPT-6.1 Sol / high | Native helpers after existing controls/events are extended |
| Review | DSH / GPT-6.1 Sol / high / chatgpt | Independent PTC review and frozen media |
| Judge | DSH / GPT-6 Astra / high / chatgpt | Existing evidence first, extra analysis where useful |
| Intake | DSH / DeepSeek Flash / low / deepseek-account | One agent, structured output and low fixed overhead |
| Garden | Kimi CLI / K3.1 Pro / high | Native retrieval, later bounded collaboration where useful |

These are template facts, not overrides of user configuration. Claude remains
explicitly optional. The desktop Codex/Claude tools can run through heavy's
PTC executor; their public machine APIs bypass that desktop tool layer. The
latest plugin heads add no managed child controls, so PTC-callable tools do not
establish child accounting or recovery. [Bindings][fw-defaults] ·
[Codex comparison][codex-latest-compare] · [Claude comparison][claude-latest-compare]

### Role work and acceptance

| Stage | Initial useful capability | Acceptance boundary |
| --- | --- | --- |
| Host preparation | Current preset exploration; saved UI/code evidence | Prepare validated inputs and provenance; do not claim host spend was fully metered |
| Formal Investigation | Bounded hypothesis helpers and a leased GUI root for reproduction | Go owns diagnostics, observation receipts and repair handoff; GUI remains subject to resource admission |
| Intake / ordinary Quick | One agent, structured output, efficient tools | Collaboration must justify overhead; Quick retains independent final Review |
| Full Plan | Small exploration helpers | One planner produces contracts; Go validates and freezes them |
| Execute | PTC, bounded helpers, optional controlled LCU root | Verify single-writer restrictions; GUI actions remain within admitted scope |
| Review / Judge | Independent analysis, frozen screenshots, bounded review lenses | Exact verified candidate; no Executor history or live GUI handles |
| Garden / repeated audits | Repository survey, later Swarm/Workflow | Findings remain proposals; native coordination does not authorize unrelated delivery |

Kimi's native media tools and LCU's DSH image projection are different input
paths. Model capability alone does not prove adapter delivery. Verify actual
artifact delivery, then compare profiles while holding
model/account/effort fixed. Test model changes separately; same-model helpers
provide separate contexts without guaranteeing independent model diversity.
[Model specification][kimi-models] · [Tools][kimi-tools]

For a frontend change, Executor produces candidate A; a leased GUI root tests
that build and archives observations. Once it stops and releases ownership,
an independent Reviewer reads A and the frozen evidence. Interactive retesting
requires a new lease and fresh target observation. Repairs produce candidate B
and new evidence. A single-writer claim requires enforceable helper restrictions;
otherwise use an entirely read-only group or separate constrained workers.
Cross-packet writers remain under Go scheduling, not a shared native checkout.

## 9. Reservations, usage and recovery

Reserve a root plus its maximum permitted active descendants before starting
it. The native cap must cover all enabled spawn paths, including workflows,
nesting, restored children and model-calling tools. Without per-child admission
hooks, a verified upfront group reservation is acceptable; do not claim Go
checks every spawn. Acquire groups without roots holding partial capacity while
waiting indefinitely for helpers. GUI leases and tool/I/O caps remain separate.

The existing scheduler tracks global and per-channel capacity. P1b initially
keeps helpers on one approved channel while checking actual models. Multi-channel
profiles must reserve a global bound plus bounds for each channel/account as one
group; aliases for the same account are not separate quota pools. Reject a
profile whose child routes or bounds cannot be established rather than charging
all children to the root channel. [Scheduler][fw-parallel]

Concurrency does not bound cumulative spend. Usage-triggered cancellation is
a soft threshold with in-flight overshoot; a hard token/monetary policy requires
actual enforcement by the chosen route. Deduplicate either a trustworthy root
aggregate including children or per-request child totals. Never add both,
count historical totals as current work, or turn missing subscription usage
into zero cost. [Existing usage contract][fw-design]

Persist intent before dispatch. Record creation, dispatch, outcome and cleanup
separately, with native identities only where observed. Stop admission, withdraw
queued work, stop native actors/jobs, prove cleanup and save the receipt before
releasing capacity. Unknown responsibility blocks retries, fallback and
replacement; a local process exit does not settle remote or GUI work.

Restoration binds role, goal, directories, model/account, permissions and
capability digest. Old boards, modes, preset IDs and handles gain no new
authorization automatically. Prewarm may initialize an eligible runtime but
must not create formal Sessions, spawn children, interact with GUI or acquire
GUI ownership speculatively. [Lifecycle][fw-sessions]

The current new-installation template explicitly selects scoped history,
parallel scheduling with two packet slots, batch dispatch, prewarm=false and
full-access permissions. Existing complete configurations omitting the relevant
policies normalize to fresh history, serial scheduling and role-based
permissions. Use the actual effective configuration; this proposal does not
reset user settings or turn full access into GUI authorization.
[Installation defaults][fw-defaults] · [Configuration loading][fw-config]

## 10. UI, deterministic selection and JEV

Expose requested policy, effective composition/capabilities and observed use.
Use role profiles with advanced limits and explicit GUI enablement/targets.
Offer an observe mode only after method/target restrictions actually enforce it.
Show availability reasons, actual image support, GUI owner,
waiting state, authorization status, epoch changes and unresolved cleanup.
Do not infer capabilities from preset labels or display unverified permission
badges.

Proposed selection semantics are **off**, **prefer** and **required**. Off must
establish that the selected capability is actually off; prefer may choose the
tested baseline only before possible related dispatch, after confirmed cleanup,
and when that baseline meets the same constraints. Required rejects unavailable
capabilities without changing model, account or permission. Initialization hooks and restoration can have effects, so a
startup error alone cannot justify retry/fallback. Keep one engine-owned
selection record rather than independent SDK/adapter/Go retries.

JEV may recommend among deterministically eligible profiles. It cannot enlarge
model/account choices, resource caps, GUI grants, permissions or budget. A
static profile works with JEV disabled. Preserve the single JEV switch;
`scheduler.dispatch` and `scheduler.prewarm` retain their existing meanings.
Native work remains subject to L1/L2/L3, protected paths, independent Review,
candidate identity and Git delivery. [Design][fw-design]

## 11. Implementation sequence

| Phase | Deliverable | Exit evidence |
| --- | --- | --- |
| P0: inventory and common policy | Installed/effective facts, capability digests, selection, resource/lifecycle contracts | Supported and unsupported cases distinguished without inference or GUI side effects |
| P1a: managed PTC | Path A composition, bounded tools/retrieval and structured results | Executor/tool policy and effective receipts; desktop configuration preserved |
| P1b: bounded helpers | DSH local helpers, then eligible Kimi/Codex/Claude profiles | Enforced caps, scope, usage and complete required cleanup |
| P2a: controlled LCU | Managed attachment gate, real approval path, GUI lease, epochs, artifacts and awaited cleanup | Mac end-to-end and fault acceptance, root-only GUI; no unsupported observe claim |
| P2b: native collaboration | Separate DSH Team, Kimi Server Swarm and Claude Workflow increments | Each adapter proves its own limits, model bindings, restoration and stop behavior |
| P3: advanced work | Isolated Tower, interactive Teams, remote review, multiwriter isolation and explicit compaction controls | Dedicated isolation, remote/restart recovery and benefit over simpler profiles |

P2a and P2b can proceed independently once their prerequisites are met. Path B
is an optional SDK extension assessed separately, not a prerequisite for path A
or all other capabilities. No phase requires rewriting every backend first.
Keep existing subscribed plugin/CLI routes and unsupported combinations visible.
New formal-engine capabilities start opt-in; decide default enablement after
engineering acceptance and paired live measurements.

| Repository / module | Implementation owner |
| --- | --- |
| dsh-preset-generator | Shared versioned composition policy and provenance; preserve user desktop settings |
| dsh-plugin-lcu | Old/new PTC value-image compatibility, managed attachment/revocation, approvals, epoch handles, awaited cleanup and media facts |
| DSH SDK/composition extension | Public composition for path A; a separate supported creation seam for named presets in path B |
| Flywheel plugins/dsh | Bridge/policy/protocol, effective probes, events/media, Host/Client status |
| Flywheel internal/contracts | Admission, global/channel/GUI resources, journal/session/prewarm, schemas and recovery |
| Existing Codex/Claude machine plugins | Public controls/events, effective configuration, child facts and usage without changing login routes |
| Kimi adapter | Keep print; optional authenticated Server runtime with its own lifecycle contract |

Promote accepted fields into the architecture/protocol/schema before code and
migrations. This PR only adds a future-direction link to current architecture.
The root npm package remains the single installation/release unit.

## 12. Validation and evidence

Before eligibility, test unavailable versions, changed composition, incomplete
approval services, unsupported image models, attempted method-level bypass,
child fan-out, combined workflow caps, partial results and missing usage.
Include same-version/different-bundle PTC, handle/image coexistence, Image identity,
zero/one/multiple images, inspect output and image-policy rejection. Exercise
Claude fork/resume cap exceptions and Workflow replay, Kimi side-chat rejection,
cross-channel reservations, account aliases and late GUI answers.
For LCU specifically cover two runs contending for one display, nonparticipating
UI interference, stop during input, lost acknowledgements, application upgrade,
reused handles, failed final exit, attachment failure and mismatched candidate
or artifact hashes. Unknown cases must not silently release leases or retry.

Use real protocol fixtures and failure injection, then the user's Mac and
subscription routes. Future code retains repository Go coverage and applicable
Node/bridge/race gates. For packaged behavior, test the complete 0.33 package,
not only source-directory imports. These are implementation exit criteria;
this proposal does not claim those tests ran for the new capabilities.

Paired experiments hold repository HEAD, contract/oracle, model/account/effort,
permissions, artifact identity, versions and resource limits fixed. Compare
single-agent, PTC, helpers and collaboration on Quick repairs, exploration,
visual frontend work, repeated audits and interrupted tasks. Measure final
acceptance, missed/spurious findings, full p50/p95 time, startup/aggregation,
known and unknown usage, quota pressure, retries, stop latency and leftover
actors. Keep failed runs. Native examples guide experiments; they do not prove
Flywheel speed, cost or quality improvements.

## Sources

Repository facts use the fixed Flywheel baseline; native sources retain their
own identities. Current web documentation can evolve and must be checked
against installed versions before implementation.

- Flywheel: [design][fw-design], [package][fw-root-package], [provenance][fw-vendor],
  [DSH bridge][fw-dsh-bridge], [policy][fw-dsh-policy], [Kimi][fw-kimi],
  [Codex bridge][fw-codex-bridge], [Claude bridge][fw-claude-bridge],
  [sessions][fw-sessions], [scheduler][fw-parallel].
- Presets/LCU: [generator][user-preset-source], [LCU attachment/approval][lcu-index],
  [PTC registration][lcu-ptc], [Proxy][lcu-bridge], [images][lcu-tool],
  [connection][lcu-connection], [generation handling][lcu-session], [guard][lcu-host-guard].
- DSH: [SDK][dsh-sdk-app], [SDK creation][dsh-sdk-server], [preset registry][dsh-preset-registry],
  [PTC][dsh-ptc], [SubAgent][dsh-subagent], [Team][dsh-team], [protocol][dsh-protocol].
- Kimi: [release][kimi-release], [CLI][kimi-cli], [profiles][kimi-agents],
  [Server API][kimi-api], [session endpoints][kimi-session-api], [Tower][kimi-tower].
  The archived [Python kimi-cli][kimi-legacy] is not the current TypeScript integration.
- Codex: [subagents][codex-subagents], [app-server][codex-app-server], [bundled source][codex-plugin].
- Claude: [subagents][claude-subagents], [Workflows][claude-workflows], [Teams][claude-teams],
  [ultrareview][claude-ultrareview], [Advisor][claude-advisor], [optional source][claude-plugin].

[fw-base]: https://github.com/ckanner/flywheel/commit/ac1ed320c68f6a8a466005985264e65b5a72fad9
[dsh-ptc-release-package]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/core/tool-executor-ptc/package.json
[dsh-ptc-master-package]: https://github.com/deepseek-ai/deepseek-harness/blob/39163dd30f703a79079e2d2446545cff3c453738/packages/core/tool-executor-ptc/package.json
[dsh-ptc-master-source]: https://github.com/deepseek-ai/deepseek-harness/blob/39163dd30f703a79079e2d2446545cff3c453738/packages/core/tool-executor-ptc/src/ptc.ts
[dsh-ptc-master]: https://github.com/deepseek-ai/deepseek-harness/blob/39163dd30f703a79079e2d2446545cff3c453738/packages/core/tool-executor-ptc/README.md
[dsh-run-code-factory]: https://github.com/deepseek-ai/deepseek-harness/blob/39163dd30f703a79079e2d2446545cff3c453738/packages/core/tool-executor-ptc/src/run-code.ts
[dsh-sdk-master]: https://github.com/deepseek-ai/deepseek-harness/blob/39163dd30f703a79079e2d2446545cff3c453738/packages/sdk/server/src/server.ts
[codex-latest-compare]: https://github.com/ckanner/dsh-subagent-codex-pro/compare/37da3d75de32b809c4fb1a631ccc15191f3d87f9...aaf465901def5aa1870c1165b9714d32294337a4
[claude-latest-compare]: https://github.com/ckanner/dsh-subagent-claude-code-pro/compare/ec0b99176b67512f3ca1838ec2a349dcb0189214...5eb637a9e82b317e67e50846005c407c54f1cea0
[fw-design]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/docs/design.md
[fw-models]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/docs/en/model-policy.md
[fw-package]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/package.json
[fw-vendor]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/vendor/README.md
[fw-dsh-policy]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/policy.js
[fw-dsh-bridge]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/bridge.js
[fw-dsh-protocol]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/protocol.js
[fw-kimi]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/internal/drivers/kimi.go
[fw-sessions]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/internal/machine/sessions.go
[fw-codex-bridge]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/codex-pro-bridge.js
[fw-claude-bridge]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/claude-code-pro-bridge.js
[fw-host-entry]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/index.js
[fw-host-control]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/control.js
[fw-task-skill]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/skills/flywheel-task/SKILL.md
[user-preset-readme]: https://github.com/ckanner/dsh-preset-generator/blob/f692cb07abbb9786444c183d74eee018b1a2f0e8/README.md
[user-preset-source]: https://github.com/ckanner/dsh-preset-generator/blob/f692cb07abbb9786444c183d74eee018b1a2f0e8/src/gen-presets.mjs
[dsh-team]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/team/agent-team/README.md
[dsh-subagent]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/subagent/subagent/README.md
[dsh-sdk]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/docs/user/guide/python-sdk.md#enable-team-mode
[dsh-ptc]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/core/tool-executor-ptc/README.md
[dsh-web]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/web/tool-web/README.md
[dsh-plan]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/plan/plan-mode/README.md
[dsh-goal]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/goal/goal-round-driver/README.md
[dsh-protocol]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/sdk/protocol/README.md
[dsh-sdk-app]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/bundle/sdk-app/README.md
[dsh-sdk-server]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/sdk/server/src/server.ts
[dsh-ptc-overlay]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/apps/cli/config/examples/ptc.yml
[dsh-web-ptc]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/bundle/web-app/presets/ptc.patch.yml
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

[fw-root-package]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/package.json
[fw-runtime-identity]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/internal/machine/session_runtime.go
[fw-parallel]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/internal/machine/parallel.go
[fw-packet-schema]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/templates/task-packet.schema.json
[dsh-preset-registry]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/preset/agent-preset-registry/src/index.ts
[lcu-index]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/index.ts
[lcu-bundle]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/cordis.patch.yml
[lcu-ptc]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/ptc.ts
[lcu-bridge]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/bridge.ts
[lcu-tool]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/tool.ts
[lcu-connection]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/connection.ts
[lcu-session]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/session.ts
[lcu-host-guard]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/host-guard.ts

[fw-defaults]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/templates/flywheel.config.example.json
[fw-config]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/internal/config/config.go
[lcu-readme]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/README.md

[dsh-base]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/bundle/base/cordis.patch.yml
