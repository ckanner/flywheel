# Interrupted calls and context quality

[Documentation](../README.md) · English · [简体中文](../zh-CN/session-recovery.md)

Flywheel separates reuse of a role’s conversation history from recovery of an
interrupted invocation. Fresh and scoped histories both retain invocation safety
checks. Durable public-journal recovery requires **Codex Pro 0.10.2** or
**Claude Code Pro 0.7.0** or **Antigravity 0.1.0**; DSH and Kimi retain their own supported recovery paths.

Antigravity recovery reads the exact owned journal and frozen subscription/evidence-only binding. It never launches a replacement CLI or resumes a global latest conversation. Native child completion and conversation-level usage can remain unknown.

## Resume an existing task

Use the original task ledger and integration directory. Read its status, then
request continuation through the normal engine entry point:

```sh
flywheel control status --cwd /repo --task-dir /repo/docs/my-task
flywheel control start --cwd /repo --task-dir /repo/docs/my-task
```

The engine reconciles earlier calls before making another model request. A
`pending` or `unknown` outcome can keep the task blocked; preserve its journals
and diagnostics rather than deleting state or selecting a fresh session to
force progress. A status response describes current facts, not permission to
replay an unresolved call.

## Observable progress and time limits

An idle timeout means that no observable activity arrived within the configured
silence budget. It does not prove that the model stopped reasoning, that a tool
deadlocked, or that the provider rejected the request. A wall timeout bounds the
whole invocation even when progress continues. User cancellation is a third,
separate cause: a child acknowledging cancellation during timeout cleanup does
not change the original idle/wall timeout into a user cancellation.

The transport determines which progress can be observed:

| Transport | Progress used by Flywheel | Time limits |
|---|---|---|
| DSH SDK, including one-use prewarming | Public assistant-stream events from the admitted root and owned local children; validated callbacks from integrated external Codex/Claude providers | Configurable idle silence budget and wall deadline |
| Codex Pro 0.10.2 / Claude Code Pro 0.7.0 | Allowlisted native streaming/tool activity from the current root and owned children, summarized as bounded metadata | Configurable idle silence budget and wall deadline |
| Antigravity 0.1.0 | Validated native init/step/result metadata for the original invocation; no fabricated heartbeats | Configurable idle silence budget and wall deadline |
| Kimi native | Supported native events associated with the admitted invocation and agent tree | Configurable idle silence budget and wall deadline |
| Kimi print / legacy `codex exec` | No continuous progress contract that can support an idle inference | Wall deadline; no idle termination based on print silence |

DSH also covers a parent waiting in PTC while an owned native child streams.
External provider, foreground PTC and background-job calls share a versioned
optional Flywheel activity service; an older or unrelated plugin without that
service's callback remains unobservable between its normal output events.
Startup, waiting for a first event, silent tools and unsupported provider phases
can still consume the configured silence budget. The ready capabilities describe
these limits; they do not promise that every live operation produces events.

DSH and the Pro plugins coalesce activity from real observations. Their metadata
contains event types, counts and root/child scope, without model text, hidden
reasoning or tool bodies. No timer,
ping or artificial heartbeat is used to keep a call alive. Activity is advisory:
it is neither a durable dispatch ACK nor a result, token bill or cleanup proof.
Validated advisory activity frames do not displace a terminal receipt from bounded output
capture; malformed or uncorrelated frames still fail the protocol checks.

Child attribution must match the active parent turn and the actual sender.
Stale collaboration events and foreign threads do not establish ownership.
Claude's ambient background watchers are excluded from both work activity and
completion tracking, including later progress that omits the ambient flag.
Startup hook activity is accepted before Claude's init frame only when its
session ID matches the caller-selected new or resumed history.

Stream traffic and retained state have different limits. Repeated Kimi native
snapshots replace the previous state instead of consuming a cumulative traffic
quota; native and usage evidence retained by Codex is included in its memory
budget. Frame, retained-state and final-result bounds remain enforced.

After an interrupted DSH call, an intact correlated receipt and normal process
exit can independently preserve the actual Session ID, dispatch fact and
confirmed SDK cleanup. The business outcome stays unknown and automatic replay
stays blocked. A truncated/corrupt stream, physical termination signal, missing
exit evidence or unconfirmed cleanup cannot supply that proof. Use the existing
reconciliation and explicit planning-retirement controls below; changing a
timeout does not resolve an earlier unknown invocation.
The same rule applies to Pro receipts: an invalid inner ownership binding,
missing lifecycle evidence or damaged outer stream cannot supply a trusted
timeout, cancellation or cleanup fact. The original receipt remains available
for inspection; a timeout or cancellation observed by Go itself remains valid.
A valid worker report of rejected activity/native metadata or an observation
quota is different from a damaged stream. Its complete, correlated cleanup
receipt can preserve independently verified identity, usage and cleanup, while
the business outcome remains unknown and no continuation checkpoint is granted.
Cleanup cancellation cannot replace that initiating worker error.

## Readable native session names

Fresh DSH, Codex Pro, Claude Code Pro and Kimi sessions receive a readable label
derived from the task title or goal, role and applicable work scope. Opaque
request IDs remain in the ledger and receipts. Continuing an existing session
keeps its original name, including a name the user edited. Naming does not make
a separate model request or change the selected model, thinking effort, task
permissions or recovery identity.

DSH uses its public title service and verifies the accepted snapshot; its native
UTF-8 byte limit controls truncation. An explicitly requested fresh title with a
missing service or failed read-back is rejected before the task prompt. Codex
and Claude Pro use their supported native naming interfaces. Codex requires the
new persistent thread's name write to be confirmed before `thread-ready` and
task dispatch. A failed write returns a `thread-name` failure with
`not-dispatched`, the known thread ID and independently verified cleanup;
read-only recovery preserves these facts without retrying. An empty thread's
existence does not establish that it can be resumed. The legacy
`codex exec` transport does not acquire this naming capability merely by sharing
the new timeout policy.

Kimi print has no corresponding print title flag. For a new named call Flywheel
first creates an idle session through the public Kimi session API, verifies the
title and ownership, and confirms that the owned preparation runtime exited.
It then runs print against that exact session. Preparation sends no task prompt
and performs no title-generation inference; preparation and execution share the
same wall budget. The original print model, automatic permission behavior and
conditional thinking-effort behavior remain in force. An unsupported API or
unconfirmed preparation cleanup prevents dispatch. Resume keeps its existing
session and does not perform this preparation again.

The public Kimi API opens and closes a native session during this preparation.
For a first named print call, configured `SessionStart` and `SessionEnd` hooks
therefore run for an extra startup/shutdown before print resumes. The business
prompt is still sent once. Agent/Swarm set the name during their existing
creation flow and do not need this extra lifecycle.

These contracts are covered by deterministic tests of the published SDK and
public plugin interfaces with offline model/CLI fixtures. They do not establish
live account availability, provider performance or desktop UI behavior.

## Find the session in its native client

For managed main-task calls, `fresh` controls which conversation is supplied as
input; it does not request deletion of the completed conversation. Codex Pro
and Claude Code Pro now request persistent native history in both fresh and
scoped modes. Fresh always starts without a prior ID and gets a new readable
name. Scoped can reuse a compatible, confirmed history. DSH and Kimi keep their
existing native persistence, and Flywheel now retains their observed fresh IDs
for inspection. Merely knowing an ID never grants managed continuation.

Native storage, a row in a client's list, and permission to continue are separate:

| Backend | Where retained root history can be found | Discovery limits |
| --- | --- | --- |
| DSH SDK | DSH desktop using the same `DSH_HOME`, SDK persistence store and original project workspace | The Flywheel desktop plugin attaches matching out-of-process SDK histories through the public Workspace service. A separate home/store is separate history. The execution Worktree is not an extra project. |
| Kimi print / supported native API | Kimi Code using the same `KIMI_CODE_HOME` and session project | A new named print call first creates and verifies an empty history, then print uses that exact ID. Native Agent/Swarm still require their full public conversation API; admitting a server identity alone does not establish those capabilities. |
| Codex Pro | The local Codex store under the same `CODEX_HOME`; matching native project/history search or `codex resume <thread-id>` | The app-server records an interactive-compatible source, but clients may additionally filter by project, provider, account or other fields. A newly created empty thread can lack a resumable rollout. Local history is not automatic cross-device cloud sync. |
| Claude Code Pro | Exact saved root UUID through `claude --resume <UUID>`; supported Desktop handoff through `claude --desktop --resume <UUID>` | Official Claude rules exclude print/Agent SDK sessions from the ordinary picker and latest-session continuation. A readable title does not remove that filter or automatically register a Desktop sidebar entry. |

Use the same local user, configuration directory and project environment as the
original provider call. In Flywheel's session detail, the native ID and provider
guidance are read-only. Native terminal commands appear only after the task is
stopped, the latest invocation is consumed and cleanup is confirmed. They use
the original project directory, falling back to the historical cwd for older
records, and open the session without sending a prompt. Continue managed work
through Flywheel's takeover and validation flow; a native client is not a
replacement for task reconciliation.

Claude Desktop handoff requires an interactive terminal, CLI 2.1.285 or later,
supported macOS or x64 Windows, an installed Desktop app and subscription
sign-in. Keep any custom `CLAUDE_CONFIG_DIR` consistent. The target cannot still
be owned by a running terminal/background session. Do not put the command in a
pipe or add print/SDK mode or a new prompt. See the official
[session filtering rules](https://code.claude.com/docs/en/sessions#where-the-session-picker-looks)
and [Desktop handoff requirements](https://code.claude.com/docs/en/desktop#coming-from-the-cli).
Codex source/list details are recorded in its
[native history guide](https://github.com/ckanner/dsh-subagent-codex-pro#thread-lifetime).

Old Pro ephemeral calls have no native transcript to reconstruct; their saved
Flywheel response remains available. Old DSH/Kimi fresh records can recover a
display ID from the exact, digest-verified invocation receipt without changing
the ledger or reuse eligibility. An unacknowledged receipt must be reconciled
before it is displayed. Low-level explicit ephemeral requests remain ephemeral.
Intake/Garden Pro calls also retain independent named histories, including a new
ID on any safe retry; they do not enter the main-task role ledger or reuse a
returned ID. DSH/Kimi advice calls receive readable titles without changing their
persistence protocol; named Kimi print uses the preparation lifecycle described
above. External DSH delegations still honor their configured/per-call persistence
policy. A native child/task ID is not automatically a resumable root-session ID.

## Ownership

| Layer | Responsibilities |
|---|---|
| Go engine | Task state, role/scope isolation, intent, candidate and contract guards, budgets, accounting, receipt consumption, delivery |
| Flywheel worker v3 | Typed lifecycle frames, Go durable acknowledgements, cancellation; no task decisions |
| Public machine API | Independent supervisor, private durable journal, native guardian, complete raw receipts, read-only `recoverMachine` |
| Native Codex / Claude | Model/tool execution, native history, observed configuration and usage |

The outer worker protocol is 3. The plugin request/receipt protocol remains 1, with lifecycle version 1. DSH and Kimi retain their existing normal-history and Go saved-receipt paths; the new plugin capabilities do not imply identical P3 support for those backends.

## Durable acknowledgement before dispatch

Go prepares the task's `.sessions/` parent and saves intent before an invocation. The upstream API exclusively creates the invocation's recovery subdirectory, named from a digest of the invocation ID. Recovery never scans global native history.

Typed events include `thread-ready`, `dispatch-intent`, `dispatch-possible`, `turn-accepted`, `terminal`, and `cleanup-confirmed`. Events must reflect actual backend observations:

- **Codex** reports its actual thread/configuration before `turn/start`. Go checks the complete permission policy, cwd, model, and identity before acknowledging. A failed acknowledgement prevents the next turn boundary.
- **Claude** checks the SDK initialize/version handshake before task input, but the native `system/init` configuration message follows prompt submission. It supports pre-dispatch intent/possible acknowledgements, but does not invent a pre-prompt thread-ready or native turn-accepted event. Its observed configuration remains a terminal admission check.

Go correlates each event, atomically saves and `fsync`s its checkpoint and state, then sends the matching ACK. Missing, early, duplicate, mismatched, or expired ACKs cannot release execution. `persistent:true` describes requested history preservation; it does not prove that an empty thread has a resumable rollout.

An independent supervisor owns the native guardian. Worker loss cancels unacknowledged steps and initiates native cleanup. The complete terminal receipt is durable before the terminal notification. A lost notification or final ACK does not rewrite a completed receipt as unexecuted.

## Read-only recovery

Restarting the same task lets Go use the original adapter version, request/run IDs, directory, and binding to call public `recoverMachine`. It reads exactly one invocation journal: no CLI startup, `thread/resume`, other-history search, or prompt resubmission.

| Result | Interpretation |
|---|---|
| `terminal` | A complete original receipt is available; validate identity, outcome, cleanup, accounting, and original candidate evidence |
| `pending` | No durable terminal receipt is available; this is not proof of a live process, and execution stays blocked |
| `unknown`, corruption, or mismatch | The outcome is unproven; preserve evidence and block replay, including after switching modes or backends |

Queries may be repeated. Cross-machine recovery requires the original data and environment and is not promised. Killing the supervisor itself can leave pending evidence. This is not an exactly-once execution guarantee.

### Explicitly retiring an unknown planning invocation

A planning-role invocation (`design-plan`, `design-review`, `plan`, `plan-review`) whose dispatch or outcome is unproven and has no recoverable journal can be explicitly retired instead of blocking the task forever. This is not recovery or replay: it preserves the unknown facts and only ends the invocation's effect boundary.

```sh
flywheel session /absolute/repo/docs/my-task \
  --key design-plan/planning:engine-1-1 \
  --abandon --reason "idle watchdog killed a live model call"
```

The record must be `blocked`, accounted, cleanup-confirmed, and have an unknown dispatch or outcome; the role must be a planning role. The command shows the exact invocation on a controlling terminal and requires `abandon <invocation>`. The proof is bound to the task directory, key, revision, reason, and exact invocation and cannot be reused across tasks. The Flywheel view uses the same action only for an exact user-sourced `/flywheel` command. Model tools do not expose it. A model process with an unrestricted shell can already rewrite `state.json` and is outside this host-proof boundary.

The record becomes `abandoned` with its reason, time, source, and original thread evidence; the old thread is never resumed. No stage result is applied and no model is called again. The associated planning request becomes `cancelled`, the task returns to `paused`, and a later explicit Plan creates a new request. `control session` returns `session-control.applied/status/message`; the control envelope's `ok:true` does not prove abandonment succeeded.

For a legacy Kimi resident Plan, the exact matching resume cache is retired in the same state write. Other threads and unknown caches remain blocked. The invocation's original thread and unknown dispatch/outcome are also retained in the audit history when a later legacy call replaces the session's latest record. DSH verifies the original user command before binding the engine proof to the canonical task path, so symlink aliases and trailing slashes identify the same task without allowing cross-task proof reuse.

A manual send cancelled before dispatch may leave a consumed receipt with a pending handoff. Reconcile clears that pending state and reports the actual transition as applied; repeating an already-settled reconcile is not a new successful operation.
If the pre-command invocation could not be read, the receipt keeps the observed
after-state but cannot confirm that this command caused it. It returns
`applied:false` with an explicit verification message; the command may already
have executed. Refresh status before another mutation rather than treating that
value as proof of non-execution.

This is not "delete the ledger and replay": the original ledger, receipt, and unknown evidence remain. Do not hand-edit `state.json` or move `.sessions/` to fabricate a terminal result.

### A successful receipt also needs original candidate evidence

At `cleanup-confirmed`, before acknowledging, Go captures the post-call candidate digest, original role guards, and Execute report. A recovered success may advance the task only with that original proof. A new workspace snapshot or rerun verification after restart cannot substitute for it.

A durable receipt without the Go candidate checkpoint may reconcile trustworthy usage while leaving business execution blocked. A proven pre-dispatch rejection may be treated as unexecuted only with confirmed cleanup and an unchanged original candidate and guard. Review/Judge contract and candidate checks remain mandatory.

### Shared-checkout execution

For shared-checkout execution, an Execute receipt fingerprints its declared write scope and binds that scope into the session identity. Independent peer changes do not discard a known invocation; changes within its own scope still reject stale receipts. L1, independent Review and collective delivery validate the entire combined candidate. Scoped receipts neither prove batch completion nor bypass unknown dispatch, outcome or cleanup.

### DSH runtime preparation after a restart

Optional `scheduler.prewarm` prepares a DSH Review runtime without a model prompt
while deterministic checks run. Before starting it, Go saves a single
`worker-prewarm` responsibility in task state, including the preparation identity,
role/scope, binding and run. Its terminal receipt is private under
`.control/prewarm/`; it is separate from a model invocation or passing Review.

The complete terminal JSON is limited to 8 KiB after UTF-8 serialization, including
escaped characters and the trailing newline. Long error diagnostics are shortened
with a truncation marker; ordinary short errors and all identity, dispatch and
cleanup fields remain intact. Diagnostic truncation does not relax any recovery
check or turn an initialization failure into a safe-expiry fallback.

A restarted engine never resumes that prepared worker. It may discharge the old
responsibility only when the matching receipt proves that no prompt was submitted,
SDK cleanup was confirmed, and both the recorded worker and its original process
group have exited. Receipt publication synchronizes the file and its directory;
recovery repeats those durability checks before clearing state. If no process was
ever started, an explicit local no-process receipt can establish that fact;
elapsed time or a missing PID cannot. Missing, oversized, corrupt or mismatched receipts, a
live or unconfirmed process/group, or uncertain cleanup keep the task blocked.
Turning prewarming off does not bypass reconciliation. After safe resolution,
subsequent work follows the current configuration and its normal invocation checks.

Within the same engine run, a safely expired unused runtime may fall back once to
a cold invocation only before any formal start was attempted, with the same
persisted invocation identity and its remaining deadlines. This is not restart
reuse, a second billed model attempt, or a way around unknown effects. Cancellation,
expired invocation deadlines, incomplete cleanup, or untrusted output prevent that
fallback.

Creating the first preparation record advances `state.sessions.version` to **3**
and later operations retain that version. This storage compatibility marker makes
older engines reject a task whose cleanup responsibilities they cannot interpret.
The new engine still reads versions 1 and 2. `sessions.policy-version` remains 1;
worker and lifecycle protocols retain their separate version contracts. Do not
edit the storage version or delete receipts to force a downgrade.

## Accounting and budgets

Consumed invocations do not create another model request or call count. If an unknown invocation later yields trusted usage, Go appends an auditable correction tied to the same call, provider, original day, and lifetime totals. Recovery does not recreate an expired daily bucket.

State uses CAS. Receipts, state, and metrics must be durable before success. A metrics append followed by failed state CAS does not create a duplicate row on retry; an existing row must still pass file and directory synchronization. Readable bytes alone do not prove durability.

Unknown is not zero. Rebuilds, role overrides, new workers, and Quick-to-Full transitions never reset budgets, attempts, or unknown-call blocking. Budgets constrain new dispatches, while a completed result with complete evidence can still be consumed.

## Bounded context rebuilds

`sessions.roles` can override fresh/scoped independently for Plan, Execute, Review, Judge, and judge-panel. Other roles inherit the global mode. See [session configuration](configuration.md#session-policy-history-reuse-and-invocation-safety).

Optional `sessions.quality` supports:

| Field | Measured fact |
|---|---|
| `max-confirmed-turns` | Confirmed, deduplicated calls in the current role history |
| `max-unproductive-turns` | Consecutive Execute calls leaving the candidate digest unchanged |
| `max-run-input-tokens` | The last complete, attributable input token count, not an inferred context-window size |

All thresholds default to disabled. They may rebuild only after the previous call has safely ended, been accounted, and been consumed. Unknown or pending results take precedence.

A rebuild records its reason and generation and hands over current contracts, workspace, valid reports, verification, and structured feedback. It never imports another role's private transcript or issues an unmetered model-compression call. A returning Reviewer must still inspect the entire current candidate.

### Verified incremental inputs

By default (when `sessions.context` is omitted or `"verified-incremental"`),
Execute and planning roles (`design-plan`, `design-review`, `plan`, `plan-review`)
may reuse a confirmed same-role stable contract (or planning Layer 1/2) after Go
verifies that the consumed receipt proves `dispatched` and `completed`, together
with the prompt version, current contract (or stable planning authority and
`project-context.md`) and readable context/cache file digests. Set
`sessions.context: "full"` to always resend full prompts. A safe admission
rejection or failed/cancelled turn may preserve the old thread, but cannot prove
delivery of that turn's newly prepared authority; the next call resends full
input. New input carries feedback, candidate identities, attempt and references
(or Layer 3 planning delta). Changed or unavailable authority, legacy missing
checkpoints and invalid context receipts select full input. The decision and
reason are saved on the role record. Review and Judge keep full current inputs
and never inherit Executor private history.

An opt-in [execution policy](configuration.md#bounded-execution-policy-opt-in)
can select another complete allowed binding. Every switch starts a new history
generation, including effort-only changes. Explicit session mode is also part
of the binding. Unknown/pending invocations reconcile first; fresh context cannot
authorize replaying uncertain effects. Switch records separate quality upgrades
from channel fallback and preserve the original task and packet budgets.

## Paired evaluation

The repository tools pin source and use an independent frozen oracle to compare fresh, Execute-only scoped, and role-scoped policies. The corpus covers first-pass success, L1 repair, Review return, long context, stop/resume, and configuration changes, with repeated trials and rotated strategy order. Reports retain quality, omissions, regressions, repair rounds, repeated findings, elapsed time, known/unknown tokens and cache use, rebuild reasons, and recovery failures. Actual state evidence determines whether a scenario occurred.

The offline native fixture exercises the real Go engine, worker v3, and public plugin API. It validates the evaluator and safety behavior, not model quality, cache rates, or savings. Real paired results require an available native CLI, account, permissions, and model. Defaults remain unchanged until that evidence exists. See the [paired evaluation guide](session-evaluation.md) for commands and report interpretation.

## Retention and operation

Recovery journals have private permissions, exclusive creation, identity/digest checks, and atomic persistence. Existing directories never authorize redispatch. They are task audit data and may contain model output and diagnostics; keep their access and backups with the task. Do not copy credentials or delete uncertain evidence during upgrades.

There is currently no automatic journal-retention timer. Archive evidence through the project's task lifecycle after completion or explicit retirement of an unknown planning invocation. Deleting a journal is not a recovery procedure. `control status` and doctor are observation surfaces; models and skills never write `state.json`. Resume through the normal Go entry point from the original task directory so the engine can reconcile before making a new call.
