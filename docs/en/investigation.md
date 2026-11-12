# Bounded investigation and feedback-driven workflows

[Documentation](../README.md) · English · [简体中文](../zh-CN/investigation.md) · [Workflow](workflow.md)

Task is the recommended goal entry. It identifies the requested deliverable and
prepares the appropriate contract. Auto chooses Quick or Full only after a valid
implementation contract exists. An investigation can deliver a report on its own;
it is not a third implementation completion mode.

| Daily request | Entry | Engine behavior and endpoint |
| --- | --- | --- |
| Diagnose and explain | `/flywheel-task` | Managed reads/searches and an evidence report; no implementation |
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

The initial action family is implemented directly by Go:

| kind | Capability | Boundaries |
| --- | --- | --- |
| `read_file` | Read one approved UTF-8 file | Reject symlinks, special files and excessive input |
| `search_text` | Literal search in one approved file, with line numbers | No regular expressions or command interpolation |
| `list_files` | List names in one approved directory | No recursive content grant; at most 4096 names |

Paths must be clean repository-relative paths. Each action references an approved
path and existing hypotheses. The opened inode must match the inspected inode;
`os.Root` confines traversal. A directory grant permits names only. This version
has no arbitrary shell, external query, production restart, cache flush, data
repair or writable investigator Agent action. Existing full-access configuration
does not add capabilities to these Go handlers. Broader experiments require
additional handlers, contracts and validation.

`limits` freezes action count, overall deadline, per-file bytes and total output
bytes. Resume does not reset them. The deadline starts at preparation, including
paused time. Bounded UTF-8 excerpts retain a content digest; `truncated` means the
display is incomplete, not that an entire log or test inventory was observed.

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
shows budgets, observations, excerpts, decisions and reports, with continue,
reconcile, stop and refresh controls.

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

## Optional JEV decisions

Preparation `decision` is independent of legacy `jev-shadow` and absent by default.
Its schema defines an explicit endpoint, fixed `jev-x.y.z` model, credential
environment-variable name, timeout, policy version, mode, allowed data categories,
thresholds and maximum calls. Credential values never enter the contract.

- Off/offline do not contact the service; shadow captures a local snapshot;
  advisory records advice; adopt can select one approved diagnostic action.
  Thresholds are explicit experiment parameters, not measured deployment accuracy.
- At least two legal choices and actual semantic uncertainty are required. One
  request pairs applicability Noul with Choice and retains full probabilities.
  Missing applicability, version mismatch, unauthorized fields, insufficient budget
  or unknown execution cause abstention.
- Before adoption, recheck task/run/revision, contracts, candidate, observations and
  action binding. Consume the decision and bind a unique action invocation in one
  CAS before dispatch. JEV cannot invent commands, skip acceptance or declare success.
- An unknown optional request is not resent. Usage remains unknown and the task can
  take its legal deterministic baseline. An unknown action outcome blocks. Ending,
  cancellation and deadline expiration still reconcile the optional request read-only.
- Maximum calls include unknowns. Hard budgets skip optional JEV when the API cannot
  enforce a provable request ceiling. Character estimates are not reservations.

Legacy workflow shadow captures only a local bounded summary; explicit `shadow run`
evaluates it later. That switch cannot adopt actions or authorize new source/log
sharing. Offline or shadow results do not establish a causal speedup. Compare full
tasks using the existing baseline, core improvements without JEV, and those same
improvements plus one action family.

### Offline replay and explicit advisory evaluation

The general decision capture is a `*.snapshot.json` document containing the frozen
`snapshot`, `policy` and source `target-hash`. It is distinct from the legacy workflow
shadow dataset. Review its facts and data permissions before any live evaluation.

```bash
flywheel decision replay --input frozen.snapshot.json --response native-response.json \
  --endpoint "$JEV_ENDPOINT" --model jev-1.14.0 --expected-model jev-1.14.0
```

Replay accepts a recorded native System One response or a fixture, forces `offline`,
does not read a credential, sends no HTTP and writes no journal or task state. The
endpoint and equal fixed model IDs identify the intended target; the version above
is an example and must match the response under evaluation. A bare Snapshot JSON
requires `--policy reviewed-policy.json`. That flag also replaces the entire captured
policy when used with a wrapper; omitted data permissions are not inherited.

For an explicitly authorized live experiment:

```bash
flywheel decision evaluate --input frozen.snapshot.json --policy reviewed-policy.json \
  --endpoint "$JEV_ENDPOINT" --model jev-1.14.0 --expected-model jev-1.14.0 \
  --directory ./decision-evaluation --key-env MEROUTER_API_KEY --max-calls 5
```

`evaluate` always forces `advisory`, even if the source policy says `adopt`. The
credential environment-variable name, fixed target, separate private journal and
cumulative call cap are explicit. It preserves source data-category permissions and
requires them to cover the snapshot. `--hard-budget` skips the optional call because
the native API cannot enforce a request usage ceiling. Reusing the same journal and
snapshot reads the original receipt; unknown requests consume the cap and are never
resent. The command does not execute an action or change task state.

Both commands print the baseline, effective policy, probabilities and receipt with
`adopted: false`. Live errors or unknown outcomes return exit code 3 with that receipt;
input/configuration errors return 1, cancellation before dispatch returns 143. A
successful offline replay is parser/fixture evidence, not a measured model success.
Limited task adoption still requires the separate preparation policy, data opt-in,
frozen concrete Go action and current-state checks described above.

## Measurement and remaining boundaries

Alongside invocation records, `metrics.jsonl` now contains `phase-start` and
`phase-end`. `control metrics` reports cumulative span duration separately from the
union of completed intervals. Parallel spans cannot be summed as wall time. Missing
ends, clock regressions and unmatched legacy calls remain explicit. Provider queue,
model and internal tool breakdown remains unknown. Investigation actions/runs use
the same phase API; unobserved host-preparation time is never filled with zero.

Offline checks do not establish fewer tokens, lower cost or equal final quality.
This change does not enable a new default model, JEV, prewarming or higher
concurrency. Ready queues, persistent workers and importing existing candidates
remain conditional on measurement and their own recovery/acceptance evidence.
