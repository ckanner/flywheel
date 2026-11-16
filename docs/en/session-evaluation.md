# Session quality policy and paired evaluation

[Documentation](../README.md) · English · [简体中文](../zh-CN/session-evaluation.md)

Flywheel supports bounded context rebuilding based on confirmed facts and paired experiments using three history policies on the same tasks. Rebuild thresholds default to disabled. The evaluator preserves original conditions and results, keeps missing usage unknown, and assumes no quality, cache, or speed benefit.

See [configuration](configuration.md) for the policy and [recovery](session-recovery.md) for lifecycle and ledger semantics. Implementation entry points are the [quality policy](https://github.com/ckanner/flywheel/blob/main/internal/machine/session_quality.go), [evaluator](https://github.com/ckanner/flywheel/blob/main/scripts/evaluate-sessions.js), and [corpus preparer](https://github.com/ckanner/flywheel/blob/main/scripts/prepare-session-evaluation.js).

## Bounded rebuilding

`sessions.roles` accepts `fresh` or `scoped` for `plan`, `execute`, `review`, `judge`, and `judge-panel`. Unspecified roles inherit `sessions.mode`. Plan's explicit `resident-session:false` still prevents history reuse; explicit `sessions.roles.plan:"fresh"` also disables the legacy Kimi Plan continuation path.

All `sessions.quality` thresholds default to `0`, meaning disabled. Existing default behavior is unchanged.

| Setting | Trigger | What it establishes |
|---|---|---|
| `max-confirmed-turns` | Confirmed, safely attributed turns in the current generation reach the threshold | Reuse count, not demonstrated quality loss |
| `max-unproductive-turns` | Consecutive Execute completions leave the candidate fingerprint unchanged | No candidate progress on disk, not an inference about private reasoning |
| `max-run-input-tokens` | The last complete, attributable per-run input reaches the threshold | That run's measured input, not native history size or remaining context capacity |

These thresholds permit rebuilding only after the previous invocation is consumed and its accounting state persisted, dispatch and outcome are known, cleanup is confirmed, and the candidate is identifiable. Unknown dispatch/outcome, unfinished lifecycle checkpoints, and unconfirmed cleanup first use P3 reconciliation or the existing escalation path. Partial/unknown usage cannot trigger the input threshold; confirmed-turn and unchanged-candidate thresholds remain independently usable. Usage uncertainty is preserved, and the existing hard budget can separately block another invocation. Consuming an invocation again cannot increment its counters again.

Rebuilding records the reason, generation, and replaced invocation reference. The handoff uses the current Spec, packet, disk candidate, valid reports, verification evidence, and structured feedback. It does not transfer Executor private history to Reviewer. The new generation resets its quality counters while task attempts, failures, elapsed work, and budget accounting remain. This policy uses verifiable rebuilding, without an extra unmetered model call to compress history.

## Three fixed policies

Experiments hold the Git commit, contracts, models, effort, permissions, acceptance, and quality thresholds constant. Only Execute and Review history policies differ.

| Report variant | Execute | Review | Plan / Judge / judge-panel |
|---|---|---|---|
| `fresh` | Fresh each time | Fresh each time | Explicitly fresh |
| `executor-scoped` | Scoped to the same packet | Fresh each time | Explicitly fresh |
| `role-scoped` | Scoped to the same packet | Scoped to the Reviewer's own review scope | Explicitly fresh |

`role-scoped` implements the design's separate Executor and Reviewer histories; it does not share one thread across roles. Each case has at least two paired repetitions, each containing all three policies. Order rotates by case and repetition. Every trial uses a new isolated Git clone. The original repository is unchanged, and one trial's candidate or history is not given to another trial.

## Prepare an executable corpus

Prepare a complete Flywheel configuration with explicit `routes.default`, `loops.review`, and their channels. Use a backend and account configuration that can already execute tasks. The preparer reads this file without running a model or reading/copying account credentials.

```sh
node scripts/prepare-session-evaluation.js \
  --config /absolute/path/flywheel.config.json \
  --output /absolute/path/new-session-corpus \
  --repetitions 2
```

The preparer creates a new local Git repository, exact commit, and `manifest.json`. It disables additional worktree creation equally for every trial because the evaluator already isolates each clone, and protects the frozen `eval/` checks. It does not overwrite the supplied configuration file.

The generated configuration-change case uses `medium` / `high`. The selected model must accept both. For a backend with different effort values, edit the manifest's `configure` step before running to use another explicitly supported value. All three policies use that same edited manifest; no trial guesses or substitutes its own value.

The built-in task is an independently testable code change: implement pure `compactRanges`, converting safe integer arrays into sorted maximal inclusive intervals, removing duplicates, handling negatives and empty input, and preserving the caller's array. Frozen L1 checks output. Final Review independently checks input preservation. A separate pinned oracle checks both. The model's done-report does not supply the evaluation score.

| Scenario | Experiment | Required observation |
|---|---|---|
| `first-pass` | Run normally from an unimplemented function | Completed task with one Execute and one Review |
| `l1-repair` | After the first execution, a fixed injector breaks output; real L1 rejects it and the engine repairs it | Failed L1 evidence and more than one Execute |
| `review-return` | Inject an in-place input sort that preserves all output checks; L1 passes and Review should return it | Review return evidence and more than one Execute |
| `long-context` | Put stable reference material in the actual Execute prompt's `cache-scope` | Measured per-run input reaches the manifest threshold; absent usage is unknown |
| `stop-resume` | Pause normally before Review, then explicitly run again | Saved paused state followed by continuation; this is a stage-boundary pause |
| `configuration-change` | Change the same binding's thinking after the first Execute, then execute again | A different effective binding fingerprint is observed |

Controlled injectors modify candidate code only. The evaluator verifies that Go state, contracts, and configuration remain unchanged across injection. It does not fabricate L1, Review, receipts, metrics, or state. The stage-boundary pause complements the separate P3 tests for active-turn termination, ACK failure windows, and read-only recovery.

Effective configuration lives outside the trial candidate and is selected with `FLYWHEEL_CONFIG`. Before the engine establishes its BaseRef, the evaluator removes overriding root/task configuration files in the disposable clone and commits this setup. Configuration changes therefore do not impersonate model edits to protected contracts. The source configuration and before/after bindings remain recorded.

## Run real backend evaluation

Build the current Go CLI and verify that the selected public adapter, native CLI, and account can execute tasks, then run:

```sh
node scripts/evaluate-sessions.js \
  --manifest /absolute/path/new-session-corpus/manifest.json \
  --flywheel /absolute/path/flywheel \
  --output /absolute/path/new-session-results \
  --live
```

`execution:"live"` requires `--live` and rejects mock driver bindings. The evaluator uses the supplied models and permissions without automatically choosing another model, channel, or broader permission mode. The output directory must not exist; saved experiments are never overwritten.

Every invocation retains the engine's normal budget, stopping, and recovery rules. The manifest's `maxCalls` is an additional observed-call limit: every 50 ms, the runner checks persisted metrics and requests cancellation when the threshold is reached. In-flight calls and delayed metric writes can exceed it; it is not a transactional reservation before model dispatch. `runTimeoutMs` bounds each step, with up to 30 seconds allowed for cancellation cleanup. Forced termination, unknown dispatch/outcome, or unconfirmed cleanup stops the entire experiment and preserves available evidence without replay.

## Interpret the report

`report.json` records source commit/tree, manifest/configuration/engine/oracle SHA-256 hashes, timestamps, trial conditions, order, policy, step outcomes, and paired comparisons. Each trial also retains stdout/stderr, state snapshots, and original metrics. The independent oracle is copied from the fixed source commit outside the candidate and checked by hash before and after execution. Candidate tampering with that oracle stops the experiment.

| Field | Meaning and limits |
|---|---|
| `quality.pass`, `omissions`, `regressions` | Independent oracle results; unfinished checks remain null. Engine completion is separate from quality. |
| `repairRounds` | Total packet attempts beyond the first, from actual Go state |
| `repeatedFindings` | Exactly repeated Review return reasons after whitespace normalization, not semantic equivalence |
| `elapsedMs` | Engine run time; excludes clone, oracle, and injection overhead |
| `usage.*.known` | Known complete per-run input/output/cacheRead/cacheWrite/totalTokens totals |
| `observedCalls` / `missingCalls` | Calls with and without each metric; known totals are lower bounds when anything is missing |
| `cost.knownUSD` | Known monetary cost, with missing costs counted separately; subscription billing is not assumed zero |
| `actions`, `rebuildReasons` | Actual create/resume/rebuild counts and attributable reasons |
| `recovery` | Resumes, failed or unknown resumes, unknown outcomes, unresolved invocations, and reconciliation count |
| `scenarioCoverage` | Planned, observed, unobserved, and unknown separately; a scenario label alone is not evidence |

P3 `record-kind:"reconciliation"` rows replace the original invocation's later-known facts using `reconciles-invocation`. They are audit corrections, not new model calls, and do not add the same usage or cost twice. Original audit rows remain preserved.

Deltas require all three paired policies, matching fixed conditions, and equal observed role/harness/provider/channel/model sets. Missing usage makes that metric's delta null. The report does not automatically produce savings percentages. Establish quality and actual scenario coverage before interpreting performance changes.

## Separate offline gates from live evidence

The repository provides three distinct evidence paths:

1. `node --test npm/test/session-evaluation.test.js` tests manifest validation, isolation, unknown usage, pairing, and stopping through an explicitly labeled runner protocol fixture.
2. The command below runs the real Go CLI → worker 3 → actual public Codex Pro 0.10.1 API from the reviewed source candidate → native CLI fixture for 36 trials. The fixture implements native protocol and controlled code work, never Go state/metrics/receipts. The real engine evaluates L1, Review, Git boundaries, and history decisions.
3. `--live` performs actual backend inference and supplies model quality, cache, time, and cost evidence.

```sh
FLYWHEEL_EVAL_BINARY=/absolute/path/freshly-built-flywheel \
FLYWHEEL_EVAL_OUTPUT=/absolute/path/new-native-evaluation \
node scripts/test-session-evaluation.js
```

From the repository root, `npm run test:sessions` invokes the same gate with the same environment variables.

The second path requires built `plugins/dsh/dist` files and the installed exact public Codex package. Missing binaries or dependencies fail rather than skip. It freezes worker and public package bytes in a separate directory. The native fixture requires an explicitly marked experimental CODEX_HOME and corpus, without actual account access. Its usage is synthetic protocol evidence, not measured model savings.

Passing offline gates does not establish real-account or macOS lifecycle acceptance. If a live environment rejects permissions or times out during thread/start, retain that blocker as evidence. Keep default thresholds disabled until paired real results justify a specific change.
