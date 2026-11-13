# JEV decisions

[Documentation](../README.md) · English · [简体中文](../zh-CN/jev.md)

Flywheel has one JEV switch: **Enable JEV** (`jev.enabled`). It is off by default.
Enabling it activates every supported JEV decision point automatically. There is
no separate shadow, advice, adoption, or per-feature switch in the task workflow.

## Enable once, use the usual task entries

In the desktop, open **Flywheel → Configuration → JEV decisions** in the intended
global or project scope. Configure the System One endpoint, fixed model version,
and credential environment-variable name, enable JEV, and save. The process running
Flywheel must receive that variable. Configuration stores its name, never its value.
Saving this setting is the activation action; tasks do not ask for the same approval
again. Advanced fields bound timeout, call count, thresholds and shared data.

The equivalent configuration section is:

```json
{
  "jev": {
    "enabled": true,
    "endpoint": "https://your-service.example/v1/systemone",
    "model": "jev-1.14.0",
    "key-env": "MEROUTER_API_KEY",
    "timeout-ms": 5000,
    "max-calls": 8,
    "min-applicability": 0.8,
    "min-choice": 0.6,
    "allowed-data": ["task-summary", "diagnostic-facts", "source"]
  }
}
```

Replace the example endpoint with your service. The model must be a fixed
`jev-x.y.z` version matching the response. The distributed template leaves the
endpoint empty and JEV disabled. Enabled configurations with an invalid target
fail validation. Missing credentials at execution are recorded as a skip, and the
task continues through its deterministic baseline.

Configuration selects **one complete file** in task → project → global → embedded
order. A project configuration that leaves JEV off overrides a globally enabled
configuration; fields are not merged across files. Use `config inspect --cwd
<project> --task-dir <task> --json` to inspect the actual source. See
[configuration](configuration.md) for discovery and editing.

Use the normal entries after configuration:

- **Task** prepares and runs an authorized investigation from your goal. Go resolves
  the JEV policy automatically; the host does not handwrite a decision mode.
- **New** prepares the same investigation but stops before running it.
- **Run** starts or resumes that prepared investigation in its original directory.
- Auto, Quick and Full remain implementation workflow choices. Their eligibility,
  acceptance and review requirements continue to be enforced by Go.

## What the switch controls

| Decision point | JEV enabled | JEV disabled |
| --- | --- | --- |
| Next evidence action | Choose among the approved read, search and directory-list actions when the choice is uncertain | Original deterministic action order |
| Redundant evidence collection | May defer remaining actions for an already observed hypothesis through an evidence-bound control choice | Original action set remains |
| Enough evidence for a report | May end collection early once the engine's evidence prerequisites are met | Collection ends at the original action, time or output limit |
| Explicit online evaluation | Available under the same enabled setting, target and configured limits | Refused before a request or evaluation journal is created |
| Recorded offline replay | Local parser/evaluation tool; no model call | Still readable; no model call |

“All supported decision points” does not mean every task makes a JEV request.
Single legal actions and deterministic choices execute directly. Missing data
permission, hard budgets without an enforceable request ceiling, exhausted calls,
low applicability, errors and invalid responses retain a recorded baseline reason.
The current runtime adoption family is investigation; repair strategy, replan and
provider/model switching are not silently enabled by names in the protocol enum.

Early finish and deferred actions are collection decisions, not proof of a root
cause. Go records the exact decision, observation and action bindings. The analyst
must still submit a report covering all hypotheses with actual observation
references and any uncertainty. An inconclusive report remains inconclusive. A
repair still needs its originally authorized scope and the ordinary implementation,
review and verification flow. See [investigation](investigation.md).

## Data, budgets and recovery

At preparation start, Go freezes the effective JEV target, data categories,
thresholds and call budget. Task-provided historical `decision` modes cannot turn
JEV on while the effective master setting is off. The default enabled data set
allows the task summary, diagnostic hypotheses, and bounded excerpts of approved
evidence. `allowed-data` can narrow this set. Omitted permission does not mean
automatic redaction: a snapshot containing an unauthorized category is skipped.

Disabling the setting stops subsequent JEV dispatches and adoption in existing
runs, including a result returned after the switch was disabled. Already dispatched
requests are reconciled using their frozen identity, and their known or unknown
usage is retained. Unknown requests are not retried. Unknown **action** outcomes
still block execution until reconciled.

Re-enabling an originally enabled run preserves its target, journal and remaining
budget. A preparation created while JEV was off remains a baseline run; a new
preparation receives the newly enabled policy. Changes that invalidate an existing
run's frozen target or tighten its permissions/limits disable further optional
decisions for that run rather than silently rewriting its contract. Check the
reported JEV status and reason; do not copy ledgers or reset budgets to bypass it.

`max-calls` includes unknown dispatched requests. It is a call cap, not a guaranteed
token or currency ceiling. A configured hard budget skips JEV when the service
cannot enforce a provable per-request bound. Every successful request records its
actual usage when available; missing usage remains unknown.

## Replay and explicit evaluation

Normal tasks save frozen `*.input.json` documents and receipts in their private
decision journal. Offline replay also accepts historical `*.snapshot.json` captures:

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

The endpoint, fixed model and credential variable come from the same configuration.
Legacy explicit target flags must match it. Evaluation forces advisory behavior;
it never dispatches an action or changes task state. Its call cap, timeout,
thresholds and data categories cannot loosen the configured bounds. Reusing an
input and journal reads the original receipt; it does not resend an unknown call.

Both tools return `adopted: false`. Live failures and unknown outcomes return exit
code 3, input/configuration errors return 1, and cancellation returns 143. Offline
fixture success is not evidence of live model accuracy or lower task latency.

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
