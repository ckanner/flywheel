# JEV decisions

[Documentation](../README.md) · English · [简体中文](../zh-CN/jev.md)

Flywheel has one JEV switch: **Enable JEV** (`jev.enabled`). It is off by default.
Enabling it activates every supported JEV decision point automatically. There is
no separate shadow, advice, adoption, or per-feature switch in the task workflow.
The distributed configuration uses `backend: "subscription"` and the
`hydrogen-local` profile: Gemini 4 Hydrogen at medium effort through the native
Antigravity CLI. JEV remains the name of Flywheel's bounded decision mechanism;
this backend does not call the separately billed System One API.

## Enable once, use the usual task entries

In the desktop, open **Flywheel → Settings → JEV** and choose the intended global
or project scope. Turn on **Enable JEV**, confirm the subscription profile and its
model/effort, then select **Save changes**. The switch edits a draft; saving changes
that scope's file. The interface rereads the configuration actually selected for
the current task and displays its source, scope and switch separately. Editing
and saving do not call a model or prove account access. Controls bound timeout,
call count and thresholds, and select the five permitted-data categories.

The equivalent configuration section is:

```json
{
  "jev": {
    "enabled": true,
    "backend": "subscription",
    "profile": "hydrogen-local",
    "timeout-ms": 30000,
    "max-calls": 8,
    "min-applicability": 0.8,
    "min-choice": 0.6,
    "allowed-data": ["task-summary", "diagnostic-facts", "source"]
  }
}
```

`profile` resolves to a named model profile in the same configuration. Go freezes
its profile name, harness, channel, exact model slug and effort. Only an Antigravity
binding with no native child profile is accepted: the adapter must independently
verify the managed custom agent and its tool catalog before supplying the bounded
answer, and reject any tool or subagent step. It runs a
fresh conversation with evidence-only permissions, owned-process cleanup and the
subscription-only policy, even if implementation stages have full access.
The CLI's retained conversation history is not a promise of zero storage or an
OS sandbox. The request exposes no tools for file access, diagnostic commands or
execution of the chosen action.

Do not include `endpoint`, `model` or `key-env` in the subscription JEV object.
The model and effort belong to the named profile; subscription login and native
extra-usage controls are checked by the adapter. No API or credits fallback is
attempted. Proven non-dispatch keeps the deterministic baseline. A possible
dispatch without confirmed cleanup blocks further work until reconciliation.
Existing records preserve actual reasons; an implementation task can fail a
precondition before creating its first decision ledger.

Configuration selects **one complete file** in task → project → global → embedded
order. A project configuration that leaves JEV off overrides a globally enabled
configuration; likewise, globally disabling JEV does not turn off a task file
that keeps it enabled. Fields are not merged across files. The settings page
labels an effective-read failure as unconfirmed and offers an explicit retry;
it does not substitute the edited global value or retry an unknown save. Use `config inspect --cwd
<project> --task-dir <task> --json` to inspect the actual source. See
[configuration](configuration.md) for discovery and editing.

Use the normal entries after configuration:

- **Task** prepares and runs authorized investigation or implementation from your goal. Go resolves
  the JEV policy automatically; the host does not handwrite a decision mode.
- **New** registers an investigation contract, or starts implementation design/review and stops for exact-version approval. See [Reviewed planning](reviewed-planning.md).
- **Run** starts or resumes the prepared investigation or implementation task in its original directory.
- Auto, Quick and Full remain implementation workflow choices. Their eligibility,
  acceptance and review requirements continue to be enforced by Go.

## What the switch controls

| Decision point | JEV enabled | JEV disabled |
| --- | --- | --- |
| Next evidence action | Choose among approved reads, searches, directory lists, diagnostic commands and HTTP queries when the choice is uncertain | Original deterministic action order |
| Redundant evidence collection | May defer remaining actions for an already observed hypothesis through an evidence-bound control choice | Original action set remains |
| Enough evidence for a report | May end collection early once the engine's evidence prerequisites are met | Collection ends at the original action, time or output limit |
| Initial execution model and collaboration | Select once among eligible complete bindings before the first Execute dispatch | Use the route's initial or original fixed binding |
| Next step after a confirmed failure | Choose among legal local repair, feedback-check and replanning candidates | Existing bounded repair and escalation rules |
| Model / effort selection | Choose a complete approved execution binding and record the actual switch | Existing fixed binding and deterministic quality/channel fallback |
| Feedback-check priority | Select an approved check to collect facts before continuing bounded execution | Existing feedback and acceptance order |
| Explicit online evaluation | Available under the same enabled setting, target and configured limits | Refused before a request or evaluation journal is created |
| Recorded offline replay | Local parser/evaluation tool; no model call | Still readable; no model call |

“All supported decision points” does not mean every task makes a JEV request.
Single legal actions and deterministic choices execute directly. Missing data
permission, hard budgets without an enforceable request ceiling, exhausted calls,
low applicability, errors and invalid responses retain the baseline once native
cleanup is known. Any decision
or request records already created preserve their actual reasons; preconditions
that fail before recording do not imply a new skip receipt.
Auto still uses Go contract eligibility to choose Quick or Full. Entry routing does
not gain a mandatory JEV classification request.

Early finish and deferred actions are collection decisions, not proof of a root
cause. Go records the exact decision, observation and action bindings. The analyst
must still submit a report covering all hypotheses with actual observation
references and any uncertainty. An inconclusive report remains inconclusive. A
repair still needs its originally authorized scope and the ordinary implementation,
review and verification flow. See [investigation](investigation.md).

## Initial execution model and collaboration

The subscription defaults include `execution-policy` candidates for each route while preserving the original initial models in [model policy](model-policy.md). Defaults use `quality-order:[]` and `max-switches:2`: JEV off keeps the initial binding; JEV on can select another candidate before the first business invocation. Existing custom configurations without an execution policy retain fixed routes; the switch cannot invent models.

In **Settings → Models and sessions → Execution candidates**, select the initial
binding, allowed candidates and switch limit without editing JSON. Options keep
their complete model, effort, permission, session and native profile; the editor
does not alter shared definitions or broaden authority. Initial removal and
quality/fallback references are guarded. Advanced JSON remains available for
complete definitions and the less common switching rules. The configuration
page describes Swarm eligibility; only the runtime can establish whether an
approved packet meets it.

Go first reconciles unresolved invocations, authorization, budget, and the reviewed contract, then filters candidates. Image packets retain bindings with actual image transport. Swarm requires Full, no images, and a valid reviewed `collaboration` declaration; a local Quick task does not automatically expand. Orchestrator uses fixed `thinking:""`, without fictitious high/xhigh settings. Candidate descriptions include model, effort, native profile, collaboration mode, and known limits.

Only multiple legal choices justify a JEV call. Its inputs are the goal, route, approved scope, acceptance, collaboration, and budget facts, not live subscription balances or invented model success rates. It returns one action ID. Go rechecks configuration, candidates, and the switch before recording the actual binding change; effort or native-mode changes also cold-start. Initial choices use Snapshot v3, preserving investigation v1 and failure v2 compatibility.

Each packet records initial selection only once and shares the cumulative decision allowance with investigation and failure adaptation. Restarts, parallel packets, repairs, and binding switches do not reset it. Pending/unknown requests reconcile their original journal without redispatch. Failed decisions with confirmed cleanup retain the initial baseline; unknown cleanup blocks. Go starts any selected Swarm execution; the JEV evaluator itself remains tool-free and cannot spawn children.

## Next steps after failure and model selection

Before generating candidates, the engine confirms that the previous invocation has
ended, its result has been consumed, candidate/failure evidence can be reconciled,
and attempts and budget remain. JEV returns a concrete action ID. Go rechecks the
identity and current switch and persists selection/effect atomically. Existing
failure caps, one-way Quick → Full upgrades, full acceptance and independent Review
remain effective. JEV cannot treat an unknown executor outcome as permission to retry.

| Action | Effect | Prerequisite |
| --- | --- | --- |
| `repair` | Continue local repair within the same packet | Another execution attempt and budget remain legal |
| `feedback-N` | Run one short check from the contract's `feedback-checks` and supply its physical result to the next attempt | The check is explicitly approved and has a `purpose`; models cannot create or change its command |
| `recheck-N` | Rerun a failed L1 check with a complete prior observation to collect fresh feedback | The check belongs to frozen acceptance and its failure is confirmed |
| `replan` | Use the existing bounded planning / workflow-upgrade path | Planning is legal, with original scope, revision limits and history retained |
| `model-N` | Select a specific allowed execution configuration | `execution-policy` is configured and the complete binding and switch limits are valid |

`N` indexes the frozen candidate set; it is not a model name or arbitrary command.
A model candidate resolves to its actual binding ID, harness, Provider, account channel,
model, effort, permission and session policy. Missing approved candidates do not
authorize guessing a model name or adding a paid channel. Model/effort changes
retain the existing cold-start rules and never pass Executor history to Reviewer.
AUTH/QUOTA and other channel failures keep their corresponding fallback rules;
they are not evidence of insufficient reasoning ability. See
[configuration](configuration.md) for the allowed binding set.

A short check's `purpose` is a nonblank description of 1–160 characters. The JEV
candidate sends this description without appending the `cmd`/`script` text. Keep
secrets, raw logs and commands out of the description itself.
Entries without `purpose` remain available to Executor but do not become targeted
JEV candidates. This field belongs only to `feedback-checks`; it does not extend
acceptance fields or task authority.

A feedback check supplies evidence for the next action. Even if it passes, the
engine still runs every required acceptance check against the final candidate;
the choice cannot remove checks. Task and node views show the request, adoption or
baseline reason, feedback check and recorded effect. `applied` means the control
decision was applied; subsequent repair, planning and model sessions remain in
the node records. `completed` means this control action settled, not that the whole
task was delivered. `feedback-completed` likewise means only that feedback settled.
`pending` requires request reconciliation. `feedback-running` and `feedback-unknown`
require the original action to be reconciled and must not be replayed automatically.
`feedback-stale` blocks execution because feedback changed candidate or verification
inputs. `replan-running` means replanning has not settled; recovery must reconcile
the original Plan session rather than dispatching again to guess its outcome.
Applied model changes show the actual before/after bindings, attempt,
reason and time. The UI never guesses a model from `model-N` or treats a configuration
change as proof that a new session ran successfully.

## Data, budgets and recovery

For an investigation, Go freezes the effective JEV backend and complete resolved
target, data categories, thresholds and call budget when each authorized round
is created. Task-provided historical `decision` modes cannot turn
JEV on while the effective master setting is off. The default enabled data set
allows the task summary, diagnostic hypotheses, structured command/HTTP outcomes,
and bounded excerpts of approved file evidence. `allowed-data` can narrow this set.
Raw command/HTTP output is included only with explicit `logs` permission; otherwise
the snapshot records that missing content and uses structured facts. Permission is
not automatic redaction: any remaining unauthorized category skips the complete
request, and summarization cannot bypass the original data policy.

Disabling the setting in the file actually selected for the task stops subsequent JEV dispatches and adoption in existing
runs, including a result returned after the switch was disabled. Already dispatched
requests are reconciled using their frozen identity, and their known or unknown
usage is retained. Unknown requests are not retried. Unknown **action** outcomes
still block execution until reconciled.

An investigation round created while JEV was off stays on its baseline; enabling
JEV does not rewrite it. An explicitly authorized new round can freeze the new
policy, within the same remaining cumulative allowance.

Implementation freezes its own runtime decision policy at the first eligible
initial choice or failure in an enabled run. If the current process started with JEV off and has no
runtime decision ledger, saving an enabled setting does not activate that process
immediately: start or resume the task so it reads the enabled configuration. Once a
runtime decision ledger exists, turning JEV off and back on retains its frozen
target, records and remaining allowance, including imported investigation calls.
Changes that invalidate a frozen target or tighten its permissions/limits disable
further optional decisions rather than silently rewriting the policy. Check the
reported JEV status and reason; do not copy ledgers or reset budgets to bypass it.

`max-calls` is a task allowance that includes unknown dispatched requests. Calls
used in every investigation round are imported into implementation's allowance. Multiple
packets, model switches, replanning and resume cannot reset it. This is a call cap,
not a guaranteed token or currency ceiling. A configured hard budget skips JEV when
the service cannot enforce a provable per-request bound. Requests record actual
usage when available; missing usage remains unknown. Native conversation totals
are not per-call deltas and are not charged repeatedly. Subscription use is not
recorded as zero cost. The returned probabilities and applicability are
uncalibrated model estimates, not measured accuracy.

The subscription adapter persists correlated lifecycle checkpoints before
acknowledging dispatch. A crash, interruption or missing terminal cannot cause a
new prompt. Recovery queries only the original native supervisor journal. If it
proves cleanup, a separate `*.reconciled.json` record adds that fact while keeping
the original unknown receipt, unknown usage and deterministic baseline; a late
answer is not adopted. Without this proof, turning JEV off, submitting a report,
starting another round or changing the model cannot bypass the cleanup block.

Implementation's `runtime-decisions` record lives in `state.json` and retains the
frozen policy, used/imported call counts, requests, selections and effects. It
follows the existing state commit and delivery rules. The request journal under
`.investigation/runtime-decisions` is private and excluded from automatic delivery;
that exclusion does not apply to the copy of decision information in `state.json`.

The desktop displays bounded summaries and omits raw snapshots and target
configuration from its projection. This display projection does not redact Git
delivery. As with existing failure feedback, approved goals, facts, bindings and
feedback output must follow the existing rule against recording credentials.
Pending requests/actions are reconciled in their original task, without a new
journal or model switch bypassing an unknown outcome.

## Replay and explicit evaluation

Normal tasks save frozen `*.input.json` documents and receipts in their private
decision journal. The existing offline System One response parser also accepts
historical HTTP `*.snapshot.json` captures:

```bash
flywheel decision replay --input frozen.input.json --response native-response.json \
  --endpoint "$JEV_ENDPOINT" --model jev-1.14.0 --expected-model jev-1.14.0
```

Replay reads no credential, makes no HTTP request and writes no journal or task
state. A bare Snapshot requires `--policy reviewed-policy.json`; an explicit policy
replaces the complete captured policy rather than inheriting missing fields.

For a deliberate online evaluation, enable JEV in the relevant scope first:

```bash
flywheel decision evaluate --cwd "$repo_dir" --input frozen.input.json \
  --directory ./decision-evaluation --max-calls 5
```

The backend and complete target come from the same configuration. The subscription
backend resolves the named profile without an API key; explicit model checks must
match, and API endpoint/key flags are rejected. Legacy HTTP target flags must match
their configured target. Evaluation forces advisory behavior;
it never dispatches an action or changes task state. Its call cap, timeout,
thresholds and data categories cannot loosen the configured bounds. Reusing an
input and journal reads the original receipt; it does not resend an unknown call.

Both tools return `adopted: false`. Live failures and unknown outcomes return exit
code 3, input/configuration errors return 1, and cancellation returns 143. Offline
fixture success is not evidence of live model accuracy or lower task latency.

## Explicit legacy HTTP configurations

Existing files without a backend retain the historical HTTP contract. An explicit
`backend: "http"` is also supported for configurations outside subscription-only
mode. Use a credential-free HTTPS `/v1/systemone` endpoint, a fixed `jev-x.y.z`
request/response version and a `key-env` variable name. The historical default
timeout is 5000 ms; HTTP is permitted only on loopback for tests. `key-env` accepts
1–120 ASCII letters, digits and underscores, starting with a letter or underscore,
and retains case. The process receives the credential; configuration stores only
its name. This is a separately billed integration, never an automatic fallback
from a subscription. A global `subscription-only` policy rejects enabling it.
Existing HTTP target hashes and frozen receipts remain readable.

## Migration from shadow evaluation

The old automatic observer and separate shadow network client have been removed.
Old `jev-shadow` configuration remains readable for migration: connection hints are
retained, but the old enabled flag never activates online decisions. An explicit
new `jev` section takes precedence. Saving through the editor writes the unified
section and removes the legacy key while preserving the normal backup/revision
checks. The retired `shadow` command reports the migration path without running
an experiment. Historical records and their schema remain available as evidence.

To assess value, compare complete tasks with JEV off and on: time to useful evidence,
actions performed/deferred, total latency, actual/unknown usage, and final quality.
A different action order or a passing fixture alone does not establish a speedup.
