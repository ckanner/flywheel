# Design, discussion, review and implementation

[Documentation](../README.md) · English · [简体中文](../zh-CN/reviewed-planning.md)

New and Plan now give design work its own engine-owned lifecycle. New registers a
short brief, starts design and independent review, and stops for approval. Plan
changes or refines that design and stops after its review. The host conversation
provides the goal and evidence; it no longer has to write the entire architecture
and all work packets before Flywheel can start.

This guide applies to tasks with `state.planning.version: 1`. Existing Spec/packet
tasks retain their previous behavior until explicitly migrated. Use the
[usage handbook](usage-handbook.md) for incident and delivery examples, and
[configuration](configuration.md) for effective model bindings.

## Choose the entry

| Entry | What happens | Where it stops |
|---|---|---|
| `/flywheel-new <goal>` | Records a thin brief; Go runs design and independent Design Review | A reviewed exact design version awaits human approval |
| `/flywheel-plan <task-dir>` | Runs overall design or revision; default scope is `design` | After planning and review, even if implementation was authorized earlier |
| `/flywheel-plan <task-dir> scope=milestone id=M-01` | Refines one milestone within the approved root | After the child plan and independent Plan Review |
| `/flywheel-plan <task-dir> scope=packet id=M01-T01` | Refines one current work packet | After planning and review |
| `/flywheel-plan <task-dir> scope=replan` | Reconsiders planning against new evidence | Material changes return to root design review |
| `/flywheel-task <goal>` | Retains the user's existing implementation authority; design/review precede execution | Review-first requests stop for approval; delegated tasks continue within their authority |
| `/flywheel-run <task-dir>` | Resumes the original task and current approved scope | Stops at unresolved approval, input, budget or recovery boundaries |

Kimi uses the corresponding `/skill:flywheel-*` names. These are skills invoked in
the conversation. The native `/flywheel {…}` command is the plugin's typed control
boundary, not a natural-language planning command.

`replan` expresses revision intent and retains the existing review scope: an
omitted ID or `id=root` means design, `id=M-01` means milestone, and
`id=M01-T01` means packet. It shares that scope's version history and findings;
changing the command name cannot discard an unresolved review issue.

To refine a selected scope, add **Instructions for this planning scope** in the
view, or tell the Plan skill, for example: “Expand crash recovery for M-01 while
preserving the approved API.” The control request's optional `text` is retained
with that request and supplied to both planner and independent reviewer. It does
not change the global input revision or revoke root approval. Changes to goals,
fixed decisions or implementation scope belong in **Design feedback** and require
the root design to be reviewed again.

`New` is a useful default for an unfamiliar or substantial system. It does start
planning model calls. Only an explicit request to register a brief without running
planning uses `prepare` with `start:false`.

## A complete design discussion

For a complex system, begin in the intended repository:

```text
/flywheel-new Design a multi-tenant job scheduling system.
Preserve our current public API. Compare recovery, isolation, migration and rollback
options. Run independent design review, then stop so we can discuss the result.
mode=full
```

The skill verifies the project and task checkout, records the original request,
constraints and concrete evidence, and submits one `planning prepare` request.
The engine owns subsequent model calls, rounds, budgets and state writes.
A brief is not a completed design. Unresolved decisions remain visible in the
result; neither the host nor a Provider Team can declare them approved.

Open **Flywheel** in the same DSH conversation. The design section shows:

- The current design's version and full digest, document, requirements and decisions.
- The previous design when included in the snapshot, for comparison.
- Independent review verdict, findings, evidence, impact and recorded resolutions.
- The most recently approved root separately from current execution child plans.
- Planning rounds, stop reasons, applied feedback and the latest request receipt.

Discuss the findings in the conversation or write **Design feedback** in the view.
For example: “Keep the current API, use the database as the durable queue, and
explain recovery if acknowledgement succeeds but the caller times out.” A feedback
request binds the observed task revision and current design reference. **Queued**
means that the active owner still needs to adopt it; it does not mean the design
changed. After adoption, select **Run Plan**, or invoke the skill for the same task.
A new design gets its own version and independent review.

The review can pass, request revision, require a user decision, or block. A passing
review is evidence about the design. Human approval is a separate record.
The bounded loop can stop for a decision, exhausted rounds, budget, or an unknown
Provider outcome; it does not silently turn those results into a pass.

## Approve the version you actually read

After the current root design passes independent review, choose:

| Control | Result |
|---|---|
| **Approve this version** | Records approval of the displayed revision and digest; implementation remains stopped |
| **Approve this version and start** | Records that same approval and explicit implementation-start authority in one action |
| **Start approved design** | When already approved, records start authority and continues without asking for another design decision |

Refreshing, reopening the view, asking a model to “approve,” or calling ordinary
Run does not approve a design. The model tool does not expose `approve`. DSH's host
checks the original user command event and exact request before dispatching the
approval. A command from a model, an inherited fork event, a consumed command, or
a stale version cannot stand in for that action. The CLI's manual approval path
requires an interactive terminal and the exact design digest.

Large console snapshots may omit historical records or document content. The view
marks those omissions and points to the original task's `state.json`; empty
projected file lists do not mean the source files are empty. If the current design
or its review is incomplete in the snapshot, approval buttons are disabled.
Use `control status` without `--compact` to inspect the complete ledger, or use the
interactive CLI approval path, which presents the authoritative design and review.

The root can be approved before execution child plans exist. Later detail is not
an invitation to change the approved intent: root and child references, independent
Plan Review and activation checks remain separate. A pending activation keeps
implementation blocked until Go resolves the original transaction.

## Progressive planning during execution

Do not try to freeze every detail of a large project on day one. Approve the root's
goals, constraints, fixed decisions, milestone structure and verification approach.
The engine can then plan the next relevant milestone or packet in that envelope.
Each child plan carries the approved root reference and has independent review.

A narrower plan can choose delegated details without demanding another root
approval. A changed requirement, fixed decision, constraint, or acceptance promise
needs a root design revision. Naming such work a “small packet” does not avoid that
boundary. Explicit `scope=milestone` / `scope=packet` is useful when you want to
inspect a particular execution plan before continuing.

Explicit Plan always runs with planning-only semantics. Even a task with earlier
start authority stops after that planning request. Resume with Run when appropriate.
The short Quick implementation path still uses Execute, required verification and
independent final Review; a new reviewed-planning task also has initial compact
design and Design Review. “Two implementation-stage calls” is not a claim that the
entire new task uses only two model calls.

## Model and Provider responsibilities

| Role | Missing binding inherits | Responsibility |
|---|---|---|
| `loops.design-plan` | `loops.plan` | Overall design and revision |
| `loops.design-review` | `loops.review` | Independent review of the exact root design |
| `loops.plan` | Its existing binding | Execution-plan refinement |
| `loops.plan-review` | `loops.review` | Independent scope and execution-plan review |

The current DSH conversation organizes the brief and your feedback. Choosing a
stronger host model does not change these engine bindings. Inspect the effective
configuration; `bindingSource` explains an inherited binding. The configuration
view displays inherited planning roles, and editing one creates its own override.
Existing model/effort defaults stay in place. There is no benchmark claim that
one model or thinking level is best for every design.

Backend/profile/native settings inherit; role session identities and permission
overrides remain independent. An explicit `sessions.roles.review` does not become
`design-review` history. Under `role-based`, new reviewers request read-only access.
If their inherited backend is Claude Code Pro, use an already authorized global
full-access setting or explicit permission bindings for `loops.design-review` and
`loops.plan-review`; otherwise doctor reports the capability conflict. Do not
silently broaden permissions. Cross-channel fallback also needs the new role's
explicit channel model mapping; backend inheritance does not authorize new spend.

Planning reuses the configured native capability layer. DSH Team and PTC remain
available according to admission and budget; local DSH children keep their root's
provider, model and effort. Codex native review and Claude Team results contribute
evidence, while the separate design/plan reviewer produces the formal verdict.
Kimi Tower candidate transfer remains an Execute capability. Ordinary design and
review roles do not gain transfer authority. See [Native capabilities](native-capabilities.md).

## CLI and recovery

Read status without a model call:

```sh
flywheel control status --task-dir /absolute/task/docs/my-task --cwd /absolute/task
flywheel template planning.schema.json
```

`control planning` reads one JSON request from stdin. The DSH model tool wraps this
same object in its `planning` property. A planning-only request is:

```json
{
  "action": "plan",
  "revision": 7,
  "request-id": "plan-my-task-2",
  "scope": "design",
  "start": true
}
```

Replace the sample revision with the current `state.revision`, retain one request
identity for one action, and provide a `scope-id` for milestone/packet scope. Save
the request in a private file and invoke:

```sh
flywheel control planning --task-dir /absolute/task/docs/my-task --cwd /absolute/task < /absolute/private/plan-request.json
```

A manual approval request uses `action:"approve"`, that same current state revision,
a new request ID, the exact `design:{revision,digest}`, and `start:false` or `true`.
Run it yourself in an interactive terminal and review the displayed design before
entering `approve <digest>` with that exact digest. Do not inject an approval-origin environment variable or ask a
model to supply terminal approval on your behalf.

After a timeout or lost reply, inspect the **original task** before another mutation.
A successful control envelope is an acknowledgement, not task completion. Unknown
outcome, cleanup uncertainty and incomplete usage remain visible and block unsafe
continuation. Do not copy state, clear receipts, switch fresh/scoped modes, or create
a new task to hide the original result.

If a manual native-session handoff changed a contract and implementation is now
blocked, release that same handoff first. Release only succeeds for its exact
session key and thread after recorded calls are consumed and cleanup is confirmed;
it does not approve the edited contract or resume implementation. Other unknown
calls still block release. You can then submit feedback and run Plan on the
original task.

While feedback is queued, refresh reads its original request ID and displays the
engine's applied or rejected receipt. Rejection retains the original input and
reason; it does not silently resubmit the feedback or mark it adopted.

The task root and `planning/` reserve `.planning-tmp-*` regular files for Go's
atomic writes. Preparation or recovery holds the task lease, removes those
interrupted temporary files, and regenerates exports from authoritative state.
A symlink, directory or special file using that prefix stops recovery for
inspection; ordinary user files are preserved.

Legacy `run --only plan` remains the previous Full milestone planning interface.
Upgrading alone does not reinterpret existing records. Explicit `planning migrate`
binds the current revision, request ID and chosen policy before entering the new
lifecycle. Preserve the original task and its evidence; migration is a deliberate
operation, not a prerequisite for continuing existing work.
