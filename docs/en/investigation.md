# Bounded investigation and feedback-driven workflows

[Documentation](../README.md) · English · [简体中文](../zh-CN/investigation.md) · [Workflow](workflow.md)

Task is the recommended goal entry. It identifies the requested deliverable and
prepares the appropriate contract. Auto chooses Quick or Full only after a valid
implementation contract exists. An investigation can deliver a report on its own;
it is not a third implementation completion mode.

| Daily request | Entry | Engine behavior and endpoint |
| --- | --- | --- |
| Diagnose and explain | `/flywheel-task` | Managed evidence collection, approved diagnostic experiments and a report; no implementation |
| Diagnose and repair within scope | `/flywheel-task` | Supported report, then Spec and Auto; retain provenance, usage and scope |
| Repair an identified issue | Task Auto or explicit Quick | Qualification, mandatory checks, independent final Review and Git delivery |
| Propose a complex change for review | `/flywheel-new` | Stop after preparation; Run Full after implementation authorization |
| Continue an existing task | `/flywheel-run` | Resume its original preparation or implementation ledger |
| Run a particular stage | Plan / Exec / Verify / Review | Direct controls remain; stage success is not whole-task completion |

Kimi uses `/skill:flywheel-*`. Status and Stop work for investigations and
implementation. Existing repair authorization is retained; new scope or an
unresolved product decision needs a separate decision.

## Preparation contract

The host prepares an input from the user's goal and repository facts. Go validates
it and owns `preparation.json`. Users do not need to fill another large form, and
the host never edits preparation state or `state.json` directly. The installed CLI
exposes `preparation.schema.json`, `preparation-state.schema.json` and
`preparation.example.json` through `flywheel template`.

Version 1 records the goal, expected result, environment, time window, known
evidence, falsifiable hypotheses, exact read paths, concrete actions and budgets.
`intent` is `investigate` or `investigate-and-fix`. Repair intent requires a nonempty
`implementation-scope`; report-only intent grants no implementation scope. A Spec
is not required while only preparation exists.

Go executes the evidence actions. Models choose only among candidates already
approved in the contract:

| kind | Capability | Boundaries |
| --- | --- | --- |
| `read_file` | Read one approved UTF-8 file | Reject symlinks, special files and excessive input |
| `search_text` | Literal search in one approved file, with line numbers | No regular expressions or command interpolation |
| `list_files` | List names in one approved directory | No recursive content grant; at most 4096 names |
| `run_command` | Run exact argv against a temporary copy of approved inputs for reproduction or diagnosis | Explicit `host-process` authority, action timeout, controlled environment, process-group cancellation and cleanup |
| `http_read` | GET or HEAD against an exact approved URL | HTTPS (loopback HTTP for tests), no redirects/environment proxy, bounded response and output |

File paths must be clean repository-relative paths, and file actions reference an
approved path. Every action references existing hypotheses. The opened inode must
match the inspected inode; `os.Root` confines traversal. A directory grant permits
names only, not its descendants' contents. Command and HTTP arguments belong to the
frozen contract. JEV cannot generate another command, URL or credential variable.

`limits` freezes action count, overall deadline, per-file bytes and total output
bytes. Resume does not reset them. The deadline starts at preparation, including
paused time. Bounded UTF-8 excerpts retain a content digest; `truncated` means the
display is incomplete, not that an entire log or test inventory was observed.

### Reproduction tests and diagnostic commands

`run_command` uses `command.argv` without an implicit shell. `command.inputs`
lists exact approved regular files from the contract's `read-paths`. Each invocation
copies these inputs with their relative directory structure and runs in that copy;
ordinary writes to the copy are not transferred back to the source workspace. This
is a bounded reproduction input set: at most 32 UTF-8 files, subject to the contract's
file and output limits, with executable bits preserved. A directory entry does not
copy its descendants automatically. The following
action fragment illustrates the fields. Replace paths and the hypothesis ID with
those actually approved in the current contract:

```json
{
  "id": "reproduce-timeout",
  "kind": "run_command",
  "purpose": "Reproduce the observed timeout with isolated inputs",
  "hypotheses": ["timeout"],
  "command": {
    "argv": ["/usr/bin/python3", "tests/reproduce.py"],
    "inputs": ["tests/reproduce.py", "src/settings.py"],
    "env": [],
    "timeout-ms": 30000,
    "expect-exit": 1,
    "execution-boundary": "host-process"
  }
}
```

`host-process` explicitly means execution with the current user's real host
permissions. The copy keeps ordinary relative writes outside the original workspace;
**it is not an operating-system sandbox or a network boundary**. Approve the concrete
program and arguments within the task's existing authority. Full-access settings
and the JEV switch do not grant that action authority on their own. `env` contains
environment-variable names, never secret values. An expected nonzero exit can
establish a reproduced failure. Timeout, cancellation, signals, output truncation
and cleanup errors cannot stand in for a normal expected exit.
An expected nonzero exit can be valid reproduction evidence, but does not by itself
qualify the remaining collection for deferral or early finish.

An observation's `execution` records physical exit/signal, timeout, cancellation,
input digest and copy cleanup. `removed` means cleanup completed, `retained` keeps
the experiment for reconciliation, and `not-created` means no copy was created.
Unknown process or cleanup outcomes leave the original invocation pending and block
further actions. Recovery reads its original receipt rather than executing again
to guess the result.

### Approved external queries

The `http_read` action's `http` object contains `url`, `method` (GET/HEAD),
`headers-env`, `timeout-ms`, `max-bytes` and `expect-status`. An approved health
query might use `headers-env: {"Authorization": "INCIDENT_STATUS_AUTH"}`. The
variable supplies the complete header value; the contract and receipt do not store
that secret value. The URL is an exact authorization: no redirects, environment
proxy or embedded credentials. Go bounds request time and bytes and records the
actual HTTP status and output digest. A GET/HEAD grant does not authorize arbitrary
services, production restarts, cache flushes or data repair.

Structured command/HTTP outcomes, such as exit code, status and completeness, use
`diagnostic-facts` and may participate under the default data policy. Raw output is
included as a bounded excerpt only when `logs` is explicitly allowed. Otherwise
the snapshot records that missing content, and JEV can still choose from structured
facts. File evidence uses `source`. Action approval does not add raw-log sharing
permission, and a permitted category does not remove the need to protect secrets.

## CLI and desktop controls

Create the actual task directory, then submit a complete preparation request:

```bash
flywheel control investigate --task-dir "$task_dir" --cwd "$repo_dir" < start-request.json
```

The outer request is `{ "action": "start", "contract": <complete-contract> }`;
read the installed example and replace its paths, goals and permissions. `start`
saves ready preparation without executing actions. To run, submit
`{ "action": "run", "revision": <observed-revision> }` through the same command.

Go starts a background owner under the existing task lifetime lease and token-bound
stop protocol. In DSH, `flywheel_control` accepts `action: investigate` and the nested
`investigation` request, associating the exact task with the conversation. The panel
shows budgets, observations, excerpts, process exits/HTTP status, cancellation and
copy cleanup, decisions and reports, with continue, reconcile, stop and refresh
controls.

```bash
flywheel control status --task-dir "$task_dir" --cwd "$repo_dir"
flywheel status "$task_dir"
flywheel stop "$task_dir"
```

`ok:true` acknowledges a control request. `running` comes from the lifetime lease.
An investigation-only response contains `preparation`, not a fabricated code-task
`state`. Reconcile an uncertain result in the original directory before mutation.

## Report and repair handoff

`awaiting-report` means observations are available. The current host model submits
one evidence-based report; Go checks fields, hypothesis coverage and references.
Separate facts from inference, classify each hypothesis as supported, contradicted
or unknown, and include counter-evidence, limitations, recommendation, regression
checks and rollback conditions. References are real observation `invocation.id`
values; missing or failed observations cannot be cited as successful evidence.

Submit `action: report`, the observed `revision` and `report` through
`control investigate`. Terminal report states are `supported`, `inconclusive` and
`access-blocked`; cancellation produces `cancelled`. Supported means the analyst's
conclusion has structurally valid evidence references. Go does not independently
prove semantic causality. Uncertainty produces a report with limitations, not an
automatic expansion of investigation or implementation authority.

When the original request authorized repair and the investigation is supported
with all receipts resolved, prepare Spec/packets in the same workspace and task
directory, then launch normally. `preparation-source` records provenance, the report
digest, authorized implementation scope, actions and elapsed time. Known JEV usage
is imported once on its original dispatch day; missing usage and cost remain
unknown. Resume never double-counts. Actual implementation changes are checked
against the original scope, including after a Quick-to-Full or model upgrade.

Preparation state, raw observations and the decision journal are private local
runtime records excluded from automatic Git delivery. Do not publish credentials
or source excerpts from logs automatically. Write a separately reviewed report when
publication is part of the user's goal. Once admitted, deleting or changing the
preparation source cannot bypass scope or accounting.

## Feedback, progress and Plan

Executor prompts recommend targeted checks first. Go still runs the complete frozen
acceptance against the current candidate before independent Review. This removes a
fixed requirement to run the full suite twice; it does not cache or remove the gate.
Packets can declare up to eight approved short `feedback-checks` with the same
`cmd`/`expect` shape as acceptance commands. They support targeted reproduction
without changing final acceptance. With JEV enabled, the engine can choose one of
these checks or recheck a failed L1 command after its prior result is confirmed.
Short checks offered to JEV also require a nonblank `purpose` of 1–160 characters;
entries without it remain available to Executor. JEV candidates send the description
without appending `cmd`/`script`; the description itself must omit secrets and raw logs.

Feedback carries stable check IDs, expected and actual physical outcomes,
candidate/check-set/environment identities, a full failed-output digest and bounded
head/tail excerpts. `observationComplete` describes command-outcome completeness,
not a complete test inventory. A repeated identical failure is stalled only for
comparable candidate, checks and environment. A changed candidate without a trusted
oracle is unknown and retains the total attempt cap. File names alone do not prove
stagnation, and free text cannot establish a trustworthy “10 failures to 1” metric.

A Full Plan is reusable only after a confirmed terminal result, valid reloaded
contracts, Git persistence and worktree synchronization. Normal repairs can retain
that proof. Contract, verification or scheduling changes, an unknown external HEAD,
legacy missing proof, explicit Plan or a verification gap require planning again.
Speed changes the economics of feedback; it does not erase planning or quality gates.

## JEV decisions

Use the single `jev.enabled` setting described in the [JEV guide](jev.md). Start
resolves configuration and freezes the policy automatically; Task, New and Run do
not maintain separate decision modes. The default disabled path retains the
original action order and collection limits. Enabled decisions may choose the
next action, defer redundant evidence collection, or stop collection for a report
when Go's evidence prerequisites are met.

Deferred actions and early completion of collection are persisted with decision
and observation references. They do not classify a hypothesis as proven or
contradicted. The analyst still writes the evidence-grounded report, and repair
still uses the original scope plus the ordinary verification and review flow.
`awaiting-report` does not restart collection when Run is repeated.

After handoff to implementation, the same switch also controls the next step after
a failure: continue repair, collect an approved feedback check, request replanning,
or select a model/effort from allowed execution bindings. These choices share the
task decision allowance, including calls already used during investigation. Changing
packet, replanning or resuming does not reset that allowance. Feedback checks collect
facts and never replace complete final acceptance. See the [JEV guide](jev.md) for
candidate and state semantics.

The effective target, model, data categories, thresholds and budget are frozen at
preparation time. Current OFF, incompatible target changes or tightened policy
stop further optional decisions. In-flight results are rechecked before adoption;
known/unknown receipts are reconciled without resending. A preparation initially
created with JEV off keeps its baseline policy; a new preparation receives a newly
enabled policy. The desktop labels its stored status as last observed.

The [JEV guide](jev.md#replay-and-explicit-evaluation) documents local replay and
explicit online evaluation. Replay stays available without JEV. Online evaluation
requires the same enabled master and configured target; its data/call limits cannot
loosen configuration. Neither tool executes actions or changes task state.

## Measurement and remaining boundaries

Alongside invocation records, `metrics.jsonl` now contains `phase-start` and
`phase-end`. `control metrics` reports cumulative span duration separately from the
union of completed intervals. Parallel spans cannot be summed as wall time. Missing
ends, clock regressions and unmatched legacy calls remain explicit. Provider queue,
model and internal tool breakdown remains unknown. Investigation actions/runs use
the same phase API; unobserved host-preparation time is never filled with zero.

Offline checks do not establish fewer tokens, lower cost or equal final quality.
JEV remains off by default; enabling the master switch activates the supported
decisions. Default models and concurrency are unchanged. The
[ready queue and bounded DSH runtime preparation](configuration.md#ready-queues-and-runtime-preparation)
are now available as explicit scheduler settings, with batch dispatch and no
prewarming remaining the defaults. Real performance and any default change still
need measurement. Long-lived workers and general external-patch import remain
separate work; the existing managed Session claim/return path keeps its original
recovery and acceptance requirements.
