# Optional JEV shadow evaluation

This describes 0.30.1 functionality. Version 0.30.0 does not include the switch; verify registry availability before upgrading.

## Enable or disable

JEV is **off by default**. Missing `jev-shadow` in older configuration also means off: no credential lookup, network requests or shadow records. Existing Go Quick/Full selection stays in charge.

In the desktop, open **Flywheel → Global/project configuration → Read draft → JEV shadow evaluation (optional)**, configure the endpoint and save. Alternatively add this section to the effective `flywheel.config.json`:

```json
"jev-shadow": {
  "enabled": false,
  "endpoint": "https://merouter.play.one2x.ai/v1/systemone",
  "model": "jev",
  "expected-model": "jev-1.14.0",
  "key-env": "MEROUTER_API_KEY",
  "timeout-ms": 5000
}
```

The shared template leaves the endpoint empty. Configuration stores the environment variable **name**, never the key. A desktop engine must inherit that variable; a terminal export alone does not establish this. Never put credentials in endpoint or model fields. Discovery selects one complete file in task → project → global → embedded order, without merging. Inspect the source with `config inspect --json`; saves affect subsequent starts. Disabling preserves previous records.

Enabling authorizes sending a bounded summary of task goals, requirement/acceptance descriptions and planning evidence, also retained privately for later labeling. No source files, diffs, execution logs or provider conversations are read. URL/home-prefix removal and common credential-pattern rejection are best effort, not universal anonymization. Decide whether these business summaries may be sent to your configured endpoint.

Only a new task's first ordinary run is observed, after existing workflow state is saved: at most one request, up to five seconds, without retries. Resume, retry and single-step commands do not backfill observations. Missing keys, invalid endpoints, timeouts, HTTP errors and record failures log a warning and leave the primary workflow running. Explicit Full or structural Quick exclusions skip inference. Hard-budget tasks skip automatic shadow usage, because experimental usage is metered separately. User cancellation still stops the whole task.

Advice never changes Quick/Full, routing, permissions, sessions, acceptance or PR/Merge gates. Independent usage and latency records do not enter the task budget. Unknown usage stays unknown; no billing savings are implied.

## Explicit experiments

Automatic records live under `~/.config/flywheel/shadow/<target-digest>/<input-digest>/` (0700 directories, 0600 files). Logs identify the directory. `cases.json` holds the bounded summary; `target.json` pins endpoint, model, questions and policy. Call records retain probabilities, suggestions, actual baseline, response model and usage. An intent without a terminal remains unknown and is not replayed. A different directory represents an explicit new experiment.

With a binary built from the new source:

```bash
flywheel shadow prepare --task-dir docs/my-task > /tmp/jev-cases.json
# Review the summary before sending it; supply MEROUTER_API_KEY in your environment.
flywheel shadow run --input /tmp/jev-cases.json \
  --directory /tmp/my-jev-experiment \
  --endpoint https://merouter.play.one2x.ai/v1/systemone \
  --max-calls 20 --timeout 5s
flywheel shadow report --input /tmp/jev-cases.json \
  --directory /tmp/my-jev-experiment
```

These explicit commands remain usable with automatic observation disabled. They do not start or mutate a task. Preparation requires a prepared or quiescent task. Reporting is offline and needs no key. For an automatic capture, use its `cases.json` as input. Copy it elsewhere before adding independent labels (`mode`, `status`, `author`, `reason`); labels are excluded from both requests and prediction cache identity. Preserve original captures.

Datasets are limited to 100 cases and 1 MiB, requests to 64 KiB. Explicit `--max-calls` is 1–100; timeout is at most 30 seconds. A batch exceeding the new-call limit is rejected before dispatch. There are no HTTP redirects or retries; error response bodies are not logged. Commands print one JSON document on stdout, errors on stderr. Exit codes: 0 report success, 1 invalid input/runtime failure, 3 failed/unknown/not-run cases, 143 cancellation.

## Interpretation

Eight atomic Noul questions use fixed experimental thresholds: every positive condition ≥0.8 and every risk ≤0.2 yields a Quick candidate; any risk ≥0.8 yields Full; otherwise abstain. The manifest pins questions, thresholds and versions. Individual probabilities are not an overall accuracy score. Unexpected response models abstain until an operator reviews and updates the pin.

This policy is **not production calibrated**. Reports separate decision-time, prepared, replay, historical and synthetic cases, and provisional versus confirmed labels. Current historical state need not be its initial Auto decision. Missing/provisional labels do not establish a false-Quick rate. Auto already uses preparation-time contract evidence; this experiment adds usage rather than removing a classifier call that never existed.

The [six historical summaries](../evaluation/jev-historical-provisional.json) reference completed commits, with unknown actual modes and provisional labels. They test the interface and expose counterexamples, not production benefits. Real automatic routing would require independently confirmed decision-time samples and measured false-Quick/abstention rates. This version has no production routing switch.
