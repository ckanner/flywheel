# Interrupted calls and context quality

[Documentation](../README.md) · English · [简体中文](../zh-CN/session-recovery.md)

Flywheel separates reuse of a role’s conversation history from recovery of an
interrupted invocation. Fresh and scoped histories both retain invocation safety
checks. Durable public-journal recovery requires **Codex Pro 0.10.0** or
**Claude Code Pro 0.5.1**; DSH and Kimi retain their own supported recovery paths.

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
- **Claude** exposes native init only after prompt submission. It supports pre-dispatch intent/possible acknowledgements, but does not invent a pre-prompt thread-ready or native turn-accepted event. Its observed configuration remains a terminal admission check.

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

Explicit `sessions.context:"verified-incremental"` permits Execute to reuse a
confirmed same-role stable contract after Go verifies that the consumed receipt
proves `dispatched` and `completed`, together with the prompt version, current
contract and readable context/cache file digests. A safe admission rejection or
failed/cancelled turn may preserve the old thread, but cannot prove delivery of
that turn's newly prepared authority; the next call resends full input. New
input carries feedback, candidate identities, attempt and references. Changed or
unavailable authority, legacy missing checkpoints and invalid context receipts
select full input. The decision and reason are saved on the role record. Review
and Judge keep full current inputs and never inherit Executor private history.

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

There is currently no automatic journal-retention timer. Archive evidence through the project's task lifecycle after completion or explicit resolution of an unknown result. Deleting a journal is not a recovery procedure. `control status` and doctor are observation surfaces; models and skills never write `state.json`. Resume through the normal Go entry point from the original task directory so the engine can reconcile before making a new call.
