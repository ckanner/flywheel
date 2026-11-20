# Machine API, protocol 1

[简体中文](MACHINE.zh-CN.md) · [Plugin guide](../README.md)

The public import is `dsh-subagent-antigravity/machine`. It has no DSH runtime-peer dependency. `lib/machine.d.ts` is the source of the exported TypeScript contract. This adapter speaks the documented official `agy` NDJSON protocol; its own wrapper and lifecycle envelopes are separate protocols.

## Exports

`MACHINE_PROTOCOL_VERSION`, `MACHINE_LIFECYCLE_VERSION`, `MACHINE_ACTIVITY_VERSION`, `MACHINE_CAPABILITIES`, `MACHINE_LIFECYCLE_CAPABILITIES`, `MACHINE_ACTIVITY_CAPABILITIES`, `MachineInputError`, `parseMachineRequest`, `runMachine`, `recoverMachine`, `doctor`.

Machine capabilities distinguish text input, schema output, precise resume, process-group supervision, observed empty-tools admission, fixed Orchestrator models and explicit native collaboration from unsupported native attachments, prewarm, provider-wide cancellation/query and native rename. Collaboration modes are requests with separate observed-child evidence, not claims of full native-tree control.

## Request

| Field | Meaning |
| --- | --- |
| `protocol`, `requestId` | Version 1 and caller-owned identity, at most 256 bytes. The caller may encode task/stage/role/attempt in this identity. |
| `cwd` | Existing absolute directory, normalized with realpath. Every process uses that cwd. |
| `command` | `agy` by default; program name or executable path, never shell source. |
| `prompt` | One nonempty UTF-8 text message, at most 1 MiB. No attachment transport. |
| `model`, `effort` | Exact Gemini 4 Hydrogen/Neon/Carbon/Argon slug with matching effort suffix; Carbon/Argon do not accept xhigh. Fixed models `gemini-pro-orchestrator-neon`, `gemini-pro-orchestrator` (Carbon), `gemini-pro-orchestrator-argon` use `effort:""` (omission normalizes to empty), with no `--effort` flag. Presence in the actual `agy models` catalog is checked before dispatch. |
| `executionMode` | Explicit `default`, `accept-edits` or `plan`. Default maps to `--mode ''`; the adapter does not inherit native auto mode silently. |
| `permissionMode` | Explicit `request-review` or `always-proceed`. The latter adds the official `--dangerously-skip-permissions`. Neither is an OS isolation claim. |
| `sandbox` | Explicit boolean. True requests `--sandbox`; false requires native terminal sandbox settings to be false. Effective OS containment is not observable in init. |
| `toolPolicy` | `native` by default; `evidence-only` requires a managed custom agent and real empty init.tools before prompt dispatch. |
| `native` | Optional `{version:1,mode:"native"\|"subagents"\|"swarm",maxObservedAgents?:1..8}`. The limit is an observed total-child guard, not a concurrency cap. Omission preserves the legacy request shape. |
| `sessionMode`, `sessionId` | `new` without ID, or `resume` with an exact UUID from a confirmed adapter-owned session. |
| `sessionName` | Optional one-line local human label. Native title remains unconfirmed. |
| `timeoutMs` | Total deadline, default 600000, zero unlimited. Includes preflight/startup/run/exit. No first-token inactivity timeout. Native print timeout is also passed. |
| `disposeGraceMs` | TERM grace period, default 2000, range 10–30000; KILL follows when necessary. |
| `maxFrameBytes` | Bound for each NDJSON frame and unfinished frame buffer, default 4 MiB. |
| `maxOutputBytes` | Retained final response bound, default 1 MiB. Consumed progress does not count toward final response capacity. |
| `outputSchema` | Optional JSON Schema 2020-12 object, validated before dispatch and again against native structured_output. |
| `subscriptionPolicy` | Exactly `{version:1,mode:"subscription-only"}`. API/cloud/credits fallback is not available. |

Unknown request fields, conflicting effort, invalid session IDs, unsupported permissions and invalid schema are rejected before provider invocation. Evidence-only rejects fixed Orchestrator models and explicit `subagents`/`swarm` collaboration before any CLI command.

`runtime.env` is either the complete provided environment or a filtered copy of process.env. When provided, it does not borrow missing keys from process.env. HOME must identify the native login/configuration environment. The adapter opens configuration JSON, never token profiles or secure keyring data. API keys, token/secret variables, alternate provider routes and Node injection variables are removed. Explicit ADC activation is rejected.

The default recovery location is a private directory under `$HOME/.local/state/dsh-subagent-antigravity/runs/<runId>`. A caller may supply `runtime.recoveryDirectory`: an unused path below an existing, canonical, owned, mode-0700 parent. Existing paths are never silently reused. Every receipt includes a ready-to-use `recovery` request.

## Native invocation and observations

The runner checks `agy --version`, `agy --help`, `agy models`, and local configuration in the exact same cwd/environment used for the turn. Ordinary calls require 1.5.7+; Orchestrator and explicit collaboration use 1.5.12+ as the supported baseline. The ordinary adjustable-model invocation is:

```text
agy --input-format stream-json --output-format stream-json
    --model <exact slug> --effort <effort> --mode <mode>
    --print-timeout <duration> --disable-slash-commands
```

Additional documented flags express resume, sandbox, preapproved tool permissions, JSON schema and evidence-only custom-agent selection. Prompt content is never in argv or shell commands. Stdin receives one line:

```json
{"event":"user","message":{"content":"the supplied prompt"}}
```

Stdin then closes. The adapter continues draining stdout/stderr until a native terminal result and local process exit/cleanup. It does not treat stdin close or exit zero as success.

Native events are `init`, `step_update` and `result`. Init must supply the expected real cwd, model and permission mode; resume must match the requested UUID. Native errors can precede init with an empty conversation ID. Terminal statuses include SUCCESS, ERROR, CANCELED, INTERRUPTED and INVALID. WAITING/RUNNING are not terminal success. Any nonempty `result.finished_reason` fails as `native-cutoff`, including unknown future values; `execution.finishedReason` retains it. A duplicate result, malformed/truncated JSON, wrong identity, oversized frame/output, soft tool denial, failed schema or nonzero exit also fails the invocation.

Observed effort, executionMode and sandbox are `null`: passing flags is not evidence that the CLI independently echoed them. `configuration.matched` means the observable fields agreed, not that every requested feature has independent attestation. `session.nativeNameStatus` remains `unknown`.

For evidence-only policy, the official docs guarantee `agent` when `--agent` is set and define `tools` as all available tools. The adapter waits for an init with the pinned agent name and an empty tools array before writing any prompt. An incompatible CLI can wait until deadline and fail without prompt dispatch. There is no fallback to a weaker policy. A managed YAML file is created with O_EXCL and cleaned only while its inode/content still match. Parent death uses the same cleanup path. Custom agent files are local workspace files, not account settings; a missing cleanup proof is a failure.

## Explicit collaboration contract

`native.mode` and model selection are independent. An Orchestrator model uses its fixed planner/implementer configuration; the adapter does not infer that it spawned a child or exercised Swarm.

| Mode | Dispatch and admission |
| --- | --- |
| `native` or omitted | Pass the prompt as text and retain `--disable-slash-commands`. Autonomous native tool/child behavior remains available; this mode does not promise a solo agent. |
| `subagents` | Wait for matching init with `invoke_subagent` in its available tools. Then send a delegation/collection instruction and JSON-encoded task, retaining `--disable-slash-commands`. |
| `swarm` | Wait for matching init and send one adapter-owned `/swarm` prefix, followed by a collection instruction and JSON-encoded task. Omit `--disable-slash-commands` only here. Do not assume an undocumented Swarm-specific tool name or require the ordinary `invoke_subagent` tool. |

Every slash in the encoded task is escaped as the JSON sequence `\u002f`. Caller text cannot introduce another slash command or a skill invocation into the adapter's command prefix. The native `disableSlashCommands:true` setting is a pre-dispatch conflict for Swarm. This setting enters the native configuration fingerprint; changing it during a call fails. The adapter neither edits settings nor supports arbitrary command strings.

Both explicit collaboration modes require actual `step_update.subagent_info.subagents` evidence. A normal root terminal without child metadata yields `collaboration-not-observed`, even though `native.completion.state` can truthfully say the CLI invocation completed. Swarm's `dispatch.method:"slash-command"` records the command the adapter requested. It does not attest command expansion, board existence, a complete member roster, or worker model selection.

Child IDs are UUID-validated, distinct from the root, deduplicated case-insensitively, and counted across the whole invocation. The optional `maxObservedAgents` is a **total observed unique child limit**, including workers, verifier and monitor. When observed count exceeds it the owned CLI process group is stopped and the receipt fails with `observed-agent-limit`. It always reports `native.limit.hard:false`; launch timing, unreported agents, concurrent work, nesting and total quota use are not hard-bounded by this guard.

Child records preserve `id`, `kind`, `role`, `logUri` and `workspaceUris` when present. URIs are metadata only and are never opened or fetched. `state:"unknown"` and `model:null` remain unchanged when a parent step says DONE or the root succeeds. Sparse updates can add metadata without duplicating a member. Metadata retention is limited to 1 MiB cumulatively across frames and updates, with a 1000-record admission bound. Exceeding either fails immediately: verified IDs remain as minimal unknown records (including the ID that crossed the bound), other member metadata is removed, and `native.metadataTruncated:true` records the loss. Only this failed/truncated receipt can retain 1001 IDs; subsequent frames are drained without recording further work. `observedAgents` continues to equal `children.length`; no successful truncated result or artificially reduced member count is returned.

The relevant receipt fields are:

```json
{
  "mode": "swarm",
  "dispatch": { "method": "slash-command", "command": "/swarm" },
  "observedAgents": 2,
  "collaborationObserved": true,
  "limit": { "maxObservedAgents": 4, "hard": false, "exceeded": false },
  "completion": {
    "scope": "cli-invocation", "state": "completed",
    "source": "normal-result-and-cli-exit", "children": "unknown"
  },
  "metadataTruncated": false
}
```

`native.completion` covers the current CLI invocation. Only an accepted normal SUCCESS result with no nonempty finished reason, a valid stream and exit zero supplies `normal-result-and-cli-exit`. Child terminal proof and aggregate native usage are separate unknown facts. The adapter does not read a native board, launch a background watcher, or cancel unrelated conversations. A native verifier is part of the same root invocation and does not replace an outer workflow's independent formal review.

## Lifecycle, ownership and acknowledgement

`runMachine` creates a detached supervisor worker with an IPC parent relationship. The supervisor owns one detached native CLI process group, writes the private journal, drains the streams and cancels the group if the caller disappears. It never kills by process name. TERM is followed by a bounded wait, then KILL if needed; retained/zombie groups are not falsely described as gone.

Lifecycle version 1 events have monotonic sequence, requestId/runId, native threadId when known, `turnId:null`, observable configuration, dispatch/outcome, and cleanup evidence:

1. `dispatch-intent`: local admission and setup, before work can be sent.
2. `dispatch-possible`: persisted before stdin can accept the task. `runtime.onLifecycle` must resolve before dispatch proceeds.
3. `terminal`: the local invocation reached its final adapter checkpoint; inspect the receipt's native terminal/outcome fields separately.
4. `cleanup-confirmed`: local ownership cleanup is confirmed; it can occur without a native terminal.

Each is written atomically with fsync before caller acknowledgement. An acknowledgement failure/deadline blocks prompt dispatch at the boundary. Parent disconnect cancels and still writes the receipt locally. Advisory progress/activity callbacks cannot turn a native failure into success. Only real events generate activity; no invented heartbeats.

`execution.dispatch` records whether the prompt was actually released; `turnDispatch` distinguishes not-dispatched, possible and accepted. A crash at the durable boundary is treated as possibly dispatched. A receipt with no native terminal retains `providerWork.state:"unknown"` even if local cleanup succeeds.

Native children are metadata-only observations with unknown model/state when the stream does not supply proof. Child activity events include an optional `native` snapshot so a DSH status read can display observed members while the invocation is running. Root terminal and local group exit do not prove all remote/daemon/native child work is stopped. `native.aggregateUsageStatus` stays unknown. No whole-tree hard budget is advertised.

Session leases are private, cross-process and bound to canonical cwd, command, model, effort, execution/approval/sandbox/tool policy, the exact optional native options and native configuration fingerprint. A resumed session must have a prior confirmed terminal and cleanup. Model/permission/cwd/native-option drift fails before dispatch. Active or unresolved leases are not removed just because a process disappeared. A caller cannot repair an unknown run by blind resume.

The adapter cannot infer two differently identified prompts describe the same task. The orchestrator remains responsible for retaining unresolved invocation IDs and preventing semantic replay under a new task ID/model/session. Desktop actions provide idempotency for the same invocation UUID and request digest.

## Receipt, recovery and usage

`ok:true` requires a normal successful native terminal without a finished reason, locally valid schema, matching observable configuration, no soft denials/process failure, and confirmed local cleanup. Explicit collaboration also needs child evidence and must remain within the observed guard. The result separates:

- requested binding and native observations;
- local execution, turn dispatch and native provider-work state;
- root session identity and local name;
- final text/structured output and native child metadata;
- local subscription-control evidence from account entitlement/billing unknowns;
- usage counters from monetary cost.

Official `result.usage`, `num_turns`, and duration are cumulative over the native conversation; only response text is per-turn. The adapter returns `usage.scope:"conversation"` and never guesses a delta. A caller resuming a session must not sum cumulative counters as independent turn charges. Missing fields remain null. `costUSD`/`reportedCostUSD` remain null, `costStatus:"unknown"`.

The receipt failure includes stage, category and bounded redacted detail. Auth, quota and rate-limit classifications require corresponding native evidence; generic errors are not automatically called quota failures. Settings conflicts and unsupported model/effort are invalid-request. Results without native terminal remain unknown and replay-unsafe.

```js
const recovered = await recoverMachine(receipt.recovery);
```

Recovery validates private directory/file ownership, no symlink files and exact request/run identity, then reads the original journal. It never launches `agy`, resumes a session, sends a prompt, queries a remote task, or kills a process. Recovery status `terminal` means a final **adapter receipt exists**; inspect that receipt because its provider-work status can still be unknown. Missing or unverifiable evidence remains unknown. Recovery does not authorize replay.

## Subscription controls versus entitlement

Local preflight rejects configured API provider, ADC, GCP, custom API models and credits fallback, filters alternate routing environments and checks drift. `verified-local-controls` means those specific checks passed. It is not account verification. Official keyring credentials are owned by the CLI; this adapter neither opens nor exports them.

The official settings reference says `useG1Credits` defaults false and controls use of credits after plan quota runs out. Native account entitlement, exact quota balance and external tool charges do not have a documented structured headless signal, so they remain unknown. Unknown quota balance alone does not trigger a paid fallback or block a locally controlled native-account request. Auth/quota errors fail the same request. No retry, new conversation or alternate provider is hidden inside the runner.

## Sources and version scope

Verified against official documents on 2026-11-20:

- [Headless mode](https://antigravity.google/docs/cli/headless/): native events, init identity/tool observations, stdin grammar, terminal states, schema, cumulative counters and soft denials.
- [CLI reference](https://antigravity.google/docs/cli/reference/): flags, mode defaults, toolPermission, sandbox and useG1Credits.
- [Authentication](https://antigravity.google/docs/install/): native keyring login, API provider routing, custom endpoint and ADC/GCP paths.
- [Custom agents](https://antigravity.google/docs/subagents/): workspace discovery, mainAgent/subagent/tools and command policy.
- [Permissions](https://antigravity.google/docs/permissions/): native rule precedence and the difference between mode, approval and containment.
- [Models](https://antigravity.google/docs/models): model families and effort. Runtime catalog remains authoritative for installed availability.
- [Swarm](https://antigravity.google/docs/swarm/): native coordination and the distinction from single-agent Orchestrator handoff.
- [Hooks](https://antigravity.google/docs/hooks/): the canonical `invoke_subagent` tool and structured child input.
- [CLI changelog](https://antigravity.google/docs/changelog?tab=cli): headless background-task waiting, nonempty `finished_reason` cutoffs, Swarm in 1.5.5 and the 1.5.6 headless fix. The 0.2.0 adapter uses 1.5.12 as its new-feature baseline.

These establish a protocol implementation, not real-account compatibility or a benchmark. See VALIDATION.md for release gates.
