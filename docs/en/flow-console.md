# Workflow console

[Documentation](../README.md) · English · [简体中文](../zh-CN/flow-console.md)

The DSH Flywheel view brings the current task, workflow stages, configuration, session
interaction, and delivery into one place. Keep the CLI, bundled engines, and DSH
plugin at the same version. For installation, see [Getting started](getting-started.md).

## Find a task without entering paths

The view follows the current conversation automatically, with no editable path fields. A conversation task navigator appears only when more than one task is associated; a single task opens directly. Its explicit tool association takes priority. Without one, the latest registered run with an exact matching host session ID opens. Foreign and ownerless runs are not shown. The index supplies identity only; Go status establishes progress. Unconfirmed operations retain their original target.


The list uses retained conversation associations and recent run registrations; it does not scan directories or provide an unlimited archive. Multiple runs of the same task share one navigation entry. Selecting a task reads its current state without starting, stopping or rebinding it. Ordinary catalog refreshes retain a manual choice; a new explicit task association still follows the conversation. Other tasks show their last recorded status, not a live-running claim. Preparation-only entries say “Select to read status”. Failed reads retain the selected identity and error rather than another task's canvas.

Switching is disabled during pending reads or mutations, unknown task outcomes and unconfirmed configuration saves. Confirmed preparation history remains readable without starting a run. Settings hides task navigation and retains draft protection. History remains the selected task's runs, not all runs in the conversation.

The project, task folder, execution directory and branch appear directly when known. Use `flywheel_control status` in the intended conversation to associate a prepared or standalone terminal task. Old UI-generated status command records do not establish a new association.

DSH keeps the original repository in the Session header and switches the current execution directory through its public API. The desktop Host periodically attaches Flywheel SDK sessions to matching existing projects; task status also ensures the original project is registered. It does not create a desktop workspace for every packet Worktree or rewrite old Session headers. Probes use temporary persistence, leaving no new user sessions. Native Providers start sessions in the project; task prompts name the actual execution directory for every shell/file operation. Backends unable to separate a `workspace-write` sandbox root reject that combination before dispatch instead of widening permissions. Full access and supported read-only modes retain their actual semantics.

Loading a skill does not start the engine. `flywheel-task` prepares a thin brief and invokes `planning prepare` for design/review and authorized implementation; investigation goals retain their bounded investigation control. Existing prepared-contract tasks still use launch/start. A preparation/proposal-only response must explicitly say execution was not started. Single-stage skills query `flywheel_control status` afterward to associate their canvas, or disclose the exact path when that tool is absent rather than claiming association.

| Conversation entry | Does it start the engine? | How the UI finds the task |
|---|---|---|
| `/flywheel-new <goal>` | Starts design and independent review; implementation stops for approval | The planning tool associates the exact task and shows its design phase |
| `/flywheel-run <task directory>` | Starts or resumes an existing task | Start and query the original task path to associate it with the conversation; the view follows the explicit association |
| `/flywheel-task <goal>` | Starts planning; continues according to recorded authority | Use investigation control or planning prepare, then check the original task status |

`new → run` and `task` use the same engine; the canvas is not exclusive to one
skill. Writing task files alone does not register a view, and a failed association
query must not be replaced with launch merely to populate the canvas. Report an
unavailable association tool explicitly. Standalone terminal tasks without a host ID require an explicit association in the intended conversation.

While the canvas is visible, registered-task discovery refreshes every five seconds.
An observed running task refreshes status every two seconds, backing off to ten
seconds after an unsuccessful response. Hidden documents skip those requests.
Refresh uses a read-only Remote instead of a slash command, so it does not append chat events or JSON. Explicit mutations and uncertain-result reconciliation retain durable command receipts. Old chat records are preserved. Session identity and read requests use strict Remote codecs; an unavailable or malformed read remains a visible error rather than falling back to chat commands.

Task titles are separate from DSH conversation names. The title comes from validated Spec `title`; otherwise the heading says “Associated task”, with the slug separately labeled “Task ID”. Project and task-record paths appear automatically in the header; node details show execution Worktrees. Settings hides the task header while retaining association updates. Its five navigation items remain together at the top, with the form beside them and the save bar above the DSH composer.

Navigation uses compact rounded hover targets with a separate selected underline. Hover and selection do not resize the controls; keyboard focus remains visible and touch targets are larger.


## Design and approval in the same view

A task with planning v1 displays **Design and review** before its implementation
canvas. Read the current document, exact revision/digest, requirements, decisions
and independent review findings; expand the previous version for comparison.
Approved root designs and current child plans appear separately, including their
reviews, contract files and activation state. Missing projected history is labelled
as unavailable in this snapshot, rather than invented or treated as empty evidence.

**Approve this version** records approval only. **Approve this version and start**
adds implementation authority in the same user action; after approval-only the
button becomes **Start approved design**. The request binds the observed state
revision and displayed design digest. Stale, unreviewed, changed-input, pending
activation or running states cannot be approved. Refreshing or remounting does not
approve or start work. Ordinary Continue preserves the approval/start boundary.

Submit feedback with a design reference. The request receipt distinguishes queued,
applied and rejected input; only adopted feedback appears in the applied list.
Use **Run Plan** to request overall design, replan, milestone or packet scope.
Explicit Plan runs planning/review only and stops. Scope IDs use the actual task's
milestone or packet identifiers. See [Reviewed planning](reviewed-planning.md).

When a planning role has a `blocked` unknown invocation, the panel shows the
unresolved call, its exact invocation and reason, and an abandonment form. A reason
and an explicit confirmation (no result applied, no replay, old thread not resumed)
are required; the action is a real user command and is revalidated against the
current state. Model tools do not expose it. Abandonment ends only this effect
boundary; an explicit Run Plan is still required to produce a new version.
“Refresh control state” reconciles only the plugin’s unconfirmed control action; it
cannot turn an engine session’s unknown into a known result and is not a substitute
for abandonment.


## This stage versus the full workflow

The task overview is shown by default so that subsequent steps remain visible. When `run.kind` and `control.kind` identify the same single-stage operation, the toolbar distinguishes its result from overall delivery. This run shows a compact result list; review-only results include **L1 → Review**, with verdict and score available in node details. Passing one stage does not mean the task is delivered.

Continue workflow is directly visible when applicable. New-task launch options appear only before an engine state exists. A passed stage does not start subsequent stages or mark the task completed. Failed, cancelled and unknown operations cannot inherit an older green verdict. Run ID, HEAD and digests are directly visible in node details.

## Investigation before implementation

A task with a preparation record and no implementation state opens the investigation
panel. It shows the goal, action budget, collected observations, truncated excerpts,
decision receipts, and the evidence report. Continue investigation submits the current
revision to Go; a pending invocation instead offers reconciliation of the original
receipt. Unknown control results require status reconciliation before another mutation.
Stop waits for the running owner to settle; it cannot turn a missing action receipt
into a confirmed cancellation.

The panel does not offer ordinary code-workflow launch while the task is in preparation.
`awaiting-report` asks the host assistant to submit an evidence-based analysis; it is
not implementation completion. A report-only goal ends with that report. An already
authorized repair can proceed after a supported report and a valid Spec are prepared,
retaining the original scope and usage. See [bounded investigation](investigation.md)
for the supported read/search actions, command protocol, and handoff requirements.

## Configuration

In an existing DSH conversation view, Flywheel remains after Statistics; the blank new-session landing page does not yet show these conversation views. Its primary navigation is Workflow and History; Settings opens a separate workspace. Global settings require no task directory. Project settings target the selected Git root. Discovery still selects one complete file in task → project → global → embedded order; it does not deep-merge files.

Opening Settings automatically reads the selected scope. Execution, Models and sessions, Budget and retries, JEV, and Diagnostics and config have separate sections. Save, Discard and Restore defaults stay in a fixed footer; Save is disabled without changes. Saving takes one click, including the single JEV master switch; the form explains supported decisions and the configured data scope. Serial mode omits concurrency controls. Minutes and seconds are converted precisely to engine millisecond fields.

Reload, complete JSON editing, diagnostics and raw responses live in Diagnostics and config. The narrow navigation row is fixed at 54px with 36px buttons (44px for coarse pointers); shorter content cannot stretch it. Leaving Settings, scope changes or reloads first disclose unsaved drafts; restoring defaults loads a pending full replacement before saving. Stage assignments edit the same draft inside Settings. Back to model list keeps edits; Save changes at the bottom is the only write action. Discard removes all unsaved changes in the current scope. Failures retain the draft and an uncertain save requires reconciliation. Go checks file identity, backs up the original and publishes atomically. Implementation runs retain their configuration snapshot. JEV additionally rechecks current activation and tighter permissions at decision boundaries; see [JEV recovery](jev.md#data-budgets-and-recovery). The displayed source is file-level: task → project → global → installed defaults selects one file, not a per-field overlay. New files start from the installed template.

Resources include ChatGPT through DSH or Codex Pro, Kimi Code, DeepSeek Account (Flash by default), and optional Claude Code Pro. Defaults come from the embedded template. Use config inspect and doctor for actual backend capabilities. An absent peer or unsupported permission fails explicitly. Provider sign-in stays with its normal account flow.

`permissions.bindings` overrides `loops.<role>`, `routes.<route>` or `judge-panel`. Unspecified bindings use the global policy. History overrides are role-based: execution routes share the execute policy while each packet keeps its own session. Scoped continuation requires the same task, role, goal and eligible binding; fresh also retains invocation-safety records.

[JEV decisions](jev.md) use one default-off master switch. Enabling it activates
supported investigation and failure-action decisions automatically, including approved
execution-model choices. Configuration inspection is read
only; it is not another switch. The investigation view shows the last observed JEV
state, adopted versus baseline actions, and evidence-referenced deferral or early
report effects. A stored state label is not a live re-read of configuration. These
collection effects never mean the root cause is proven or implementation completed.

### Switches and scheduling choices

Settings uses one JEV master switch. Automatic task Worktree management and early
DSH Review runtime preparation have their own switches in Execution. Each switch
has an associated label, a visible On/Off state, and native checkbox keyboard
behavior. Toggling edits the draft; Save changes writes the selected scope. No
second JEV confirmation or task-specific enable switch is required.

Parallel dispatch remains a two-option policy selector: Batch waits for the
current batch, while Ready queue refills an available Execute/L1 slot with another
eligible independent packet. The form preserves the selected policy and explains
unmet configuration conditions; it cannot establish whether a future task will
use Full or have independent work. Ready queue requires Full, parallel scheduling,
a maximum above one and automatic Worktrees. Early runtime preparation is
independent of JEV and only initializes a supported DSH Review SDK; no Session or
model prompt is created before normal admission. Defaults remain Batch and
preparation Off. See [runtime scheduling](configuration.md#scheduling-and-worktrees).

Automatic Worktree management controls engine-owned directories. Turning it off
does not delete existing directories or prohibit externally managed Worktrees.
The JEV section explains the allowed data boundary and current-task disable
semantics beside the fields. Disabling JEV or switching to serial discards the
invalid input drafts hidden by that change and retains the last valid values;
unrelated errors still require correction. Detected invalid fields entered through advanced JSON remain editable there and
block Save until corrected; Go still validates the complete configuration.
A valid connection, provider quota, explicit empty allowed-data list, role policy
or budget is not reset by an unrelated switch. Invalid drafts and stale/unknown
saves retain the existing protections.

### Session defaults and stage overrides

Execution sets the default fresh/scoped policy. Models and sessions lets each role inherit or override it. Execute routes share a role policy while packets retain independent histories. An explicit Plan choice reconciles the legacy Kimi history switch; inheritance preserves it. Nodes show actual session IDs, actions and reasons, not a resume inferred from configuration.

### Optional usage budget

Choose Off, Warn only, or Stop at limit. Off hides the threshold while retaining accounting. Warn only does not stop execution; a hard budget blocks new calls at the limit or when usage is unknown. Thresholds apply per task and UTC day; blank means no limit. Timeouts and transport retries remain independent, as do Provider quotas. The form reads actual settings and never silently changes an existing soft budget to off.

## Canvas and operations

The left-to-right overview groups Plan/decomposition, scheduling, task branches, integration/final checks and PR/Merge. Selecting a node directly shows its outcome, actions, configuration snapshot and actual role-session facts. Evidence and managed session interaction have separate explicit destinations. Open task workflow drills into Execute, L1, Review and Git delivery; integration can be inspected separately, while PR and Merge are separate nodes. Colors have text labels. Identity, project/worktree, branch, HEAD, run ID, bindings and evidence come from actual receipts.

The canvas uses the available plugin width, with two-axis panning, zoom, fit width, current-step location, paging and a list alternative. New snapshots preserve the viewport and selection; location can switch to the running packet's page. Large tasks use bounded excerpts. Read full node evidence loads one packet with a matching fresh revision; it never changes the ledger.

Focus canvas expands the view in a plugin-owned native dialog. Exit or Escape returns without starting or stopping the task. It does not change the DSH global layout or composer. Unknown operation outcomes exit focus mode so the original-invocation reconciliation action remains visible.

Ordinary node operations dispatch once with the observed revision. Repair and retry records the explicit console action and adds one Execute/L1 attempt, without inventing an explanation for the failure. Reopening a completed packet still requires a reason and confirmation of downstream evidence invalidation; a changed revision invalidates that confirmation. Review cannot run with a red current L1. Unqueried remote state, closed PR and merged PR remain distinct.

- Run stage submits the observed revision for lease-bound admission.
- Repair and retry records the explicit console action and adds one Execute/L1 attempt without resetting failures or billing.
- Reopening a completed packet archives old evidence and invalidates downstream acceptance.
- Git delivery requires current L1 and independent Review evidence.
- Final acceptance runs milestone gates and Judge without starting Plan or unfinished execution units.
- Continue workflow resumes the persisted Full or ultraWork/Quick policy.
- Stop waits for managed work to settle; closing a view is not a stop.

Explicitly independent packets with satisfied dependencies can be run ahead in their registered execution directories, subject to the selected worktree policy. Integration changes invalidate old acceptance. Each asynchronous node operation records running/passed/failed/cancelled and its reason. Quick retains deterministic checks and independent final Review; omitted Plan/Judge stages are not presented as passes.

## Parallel execution

### Eligibility and limits

Read `flywheel template scheduler-policy.schema.json` for the current fields and bounds. For example:

```json
{"scheduler":{"mode":"parallel","max-parallel":2,"providers":{"deepseek-account":1},"dispatch":"batch","prewarm":false}}
```

Provider-limit keys are actual configuration channel names. The example uses the inline template's `deepseek-account`; a binding using `deepseek-account-sdk` needs that key instead. Eligibility comes from frozen packet contracts:

```yaml
schedule:
  after: [M01-T01]
  writes: [src/search/, tests/search/]
```

Dependencies reference earlier known packet IDs. Writes are normalized relative paths or directory prefixes, without globs or escapes. Include ordinary tests/configuration and account for shared test resources. Undeclared independence, overlapping scopes and live-credential units remain serial.

### Dispatch and runtime preparation

The configuration editor also offers `scheduler.dispatch` and `scheduler.prewarm`.
The defaults are `batch` and false. Selecting `ready-queue` fills a free Execute/L1
slot from the remaining independent Full packets when automatic Worktree management
is enabled and the parallel limit exceeds one. Review, replanning and integration
retain the all-workers-settled barrier. Shared-checkout runs keep batch dispatch.

Prewarming initializes one DSH Review runtime while a serial packet performs its
post-Execute deterministic checks. It sends no model prompt until normal Review
admission. Parallel Execute/L1 lanes and shared candidates do not prewarm; an
independent cohort must settle before its serial Review path may prepare a runtime.
Other harnesses use the cold path. Unused runtimes must finish bounded cleanup.
Neither setting changes JEV activation, task qualification or completion gates.
Saving takes effect on the next start. See [Configuration](configuration.md#ready-queues-and-runtime-preparation)
for eligibility, recovery and the separate live-performance evaluation.

### Execution directories

Configuration → Workflow policy → Automatically manage task Worktrees is on by default (`worktree.enabled`). It controls automatic directory creation/management independently from parallel scheduling. The installation default allows parallel execution with a limit of two; existing complete files without `scheduler` remain serial, as do explicit serial settings. Parallel automatic management requires isolated worktrees and stops if creation fails; the legacy serial path can record an in-place fallback. With automatic management off, tasks may run concurrently in the task checkout or choose their own Worktrees. Shared directories do not provide file isolation: write-scope conflicts, candidate drift and failed acceptance still stop progress.

To select another execution Worktree, the executor first prepares a clean checkout of the same repository and task baseline, unclaimed by another unit in this workflow and with a branch not shared by multiple Worktrees, then returns only `{"flywheelExecutionDirectory":{"cwd":"/absolute/worktree","branch":"actual-branch"}}` before implementation. Go validates and records it; the next invocation, tests, Review and delivery follow that directory. Registration is not task completion and its model usage is still billed. External Worktrees are retained. A prompt-only directory claim or edited state file cannot replace this registration.

Independent Worktrees permit concurrent Execute/L1. Shared checkouts run Execute concurrently, then verify and review serially after all workers finish. One Go coordinator serializes CAS writes and accounting. Hard budgets admit only one model call at a time, retaining its slot through durable accounting. Other modes enforce total and channel concurrency. Cancellation awaits every started worker.

Review, integration and milestone gates remain serial. A combined candidate is revalidated after incorporating earlier work. Bounded execution failures retain their escalation policy. Integration conflicts and failed gates preserve the worktree and evidence. Write scopes are orchestration/acceptance contracts; full access is not an OS sandbox.

### Shared-checkout batches

When a batch shares one checkout, all executor calls finish before L1 and independent Review validate the complete combined candidate. Changes are projected onto declared scopes; undeclared and forbidden paths still fail. This does not attribute filesystem writes to individual processes. Repairs invalidate earlier proofs. Only after every peer passes can code be committed and all completions published in one state CAS; failed publication never optimistically completes a subset. Mixed batches deliver the shared checkout first, then revalidate and integrate external Worktrees. Manual integration/final checks cannot bypass the batch barrier; resume the complete workflow. The canvas shows the cohort and previous error.

## Controlled Provider session interaction

Read latest response opens the exact digest-bound receipt, not an invented native history path or a claim of complete provider history.

A settled, consumed, eligible packet session can be claimed. The persisted handoff prevents automatic run/step from continuing. Both the desktop prompt form and managed terminal restore that exact thread through its adapter and keep budgets/accounting. Missing history, changed bindings or a required rebuild cannot silently become a cold start.

```sh
cd -- '/absolute/task-worktree'
flywheel session '/absolute/task-worktree/docs/task-slug' --key 'execute/M01-T01'
```

Each terminal line is a prompt. `/history` reads the latest response; `/reconcile` checks the original invocation; `/return` validates and returns; `/release` releases human control while staying paused; `/quit` retains the handoff. The desktop form supports multiline prompts.

Returning runs L1, applicable Quick gates and independent Review. Human text is not a passing verdict. Candidate-bound human-takeover evidence allows normal subsequent delivery. Release does not claim verification and cannot bypass unknown effects or cleanup. Milestone sessions currently expose read-only responses; interactive handoff is packet-scoped.

This return path requires an existing eligible managed session. It does not import
an arbitrary patch prepared before the task, and running Verify followed by Review
does not create that provenance. A general existing-candidate admission interface
remains separate work.

The engine accepts session ledger storage versions 1, 2 and 3. Human handoff
requires at least v2; the first runtime prewarm records v3 so older engines reject
cleanup responsibilities they cannot interpret. Later handoffs retain v3.
Session policy-version remains 1; worker/lifecycle versions are independent.
[Recovery](session-recovery.md#dsh-runtime-preparation-after-a-restart) explains the
original cleanup receipt required after an interrupted preparation.

## Discovery and comparison

CLI and desktop starts register run IDs. A conversation retains multiple confirmed task associations. The index lives in `runs/` beside the global configuration, optionally overridden by absolute `FLYWHEEL_RUNS_DIR`. It shows the latest 100 registrations and never scans the disk. Query Go again for liveness.

Run history is limited to the currently associated task and uses a selection list with adjacent details for operation kind, branch, HEAD, contract/configuration conditions, elapsed time and newly observed attempts, failures and deliveries. Selecting a record does not launch it; opening its canvas refreshes current Go state. Metrics scope chooses the selected run or all calls for its task, including legacy observations. The reference compares conditions without changing the metrics target. Invocation/receipt reconciliation precedes aggregation; late receipts keep their original attribution and are not double-billed. A group with entirely missing cost shows Not reported, not $0.

Usage, cost and duration are known amounts, with missing/partial observations retained. Costs may include estimates from configured prices. Call failure rate is not code quality. Different conditions permit descriptive comparison only; quality conclusions require acceptance evidence and an independent oracle.

## PR and Merge

Inspect the project and remote first. Writes require normal GitHub CLI authentication, completed local acceptance, a still-valid candidate, and explicit repository/HEAD/base authorization. A changed registered repository identity is rejected. Fetch and push URLs must identify the same repository.

Push uses the approved commit and named task branch, without force, implicit tags or recursive submodule pushes. A separate remote task branch does not require switching the local checkout. Requery after push so an earlier Draft PR is updated instead of duplicated.

Merge rechecks the PR, head, base and checks, using `--match-head-commit`. It never uses admin bypass or enables auto-merge on its own. Merge-queue acceptance and actual MERGED state remain distinct; see the [GitHub CLI contract](https://cli.github.com/manual/gh_pr_merge).

Retries reconcile existing PR/merge results before mutations. Missing receipts remain unknown. Remote state is separate from local engine completion. Publication must bind the actual merge commit and artifacts for its own validation; Merge does not automatically publish npm packages or deploy.

## Related guides

- [Configuration](configuration.md): complete files, model bindings, permissions, and scheduling.
- [Workflow](workflow.md): preparation, execution, and PR review checkpoints.
- [Investigation](investigation.md): bounded evidence collection, reports, and optional JEV decisions.
- [Recovery](session-recovery.md): durable receipts and unresolved invocation handling.
