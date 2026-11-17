# Kimi native execution protocol

[中文](KIMI_NATIVE_PROTOCOL.md)

`kimi-native-worker.js` and `runKimiWorker()` implement native Kimi Agent, Swarm and Tower execution for the Go engine. The ordinary Kimi print inference transport remains separate; new readable-history preparation is described below. Go owns role loops, budgets, boundary checks, Review, commits and acceptance; native children belong to one role invocation.

## Entry and admission

The worker receives one UTF-8 JSONL `start` on stdin, which the parent keeps open. EOF, SIGINT, SIGTERM and SIGHUP request cancellation. stdout contains only protocol 1 `ready`, `event` and exactly one terminal `result`. The bridge emits ready/event and returns the final result; the CLI writes it once. Requests are bounded to 1 MiB and output frames to 2 MiB.

```js
{
  type: 'start', protocol: 1, requestId: 'invocation-id',
  cwd: '/physical/role-worktree', workspaceCwd: '/physical/project',
  model: 'configured-model-alias', reasoningEffort: 'high',
  permissionMode: 'danger-full-access', prompt: 'Role instructions', timeoutMs: 600000,
  idleTimeoutMs: 300000, // effective parent idle bound; optional, default 300000
  persistent: true, // sessionId is supplied only to resume Agent/Swarm history
  native: {
    version: 1, mode: 'agent', // agent | swarm | tower
    agents: { enabled: true },
    agentProfile: 'agent' // optional discovered Server profile name, not an agent-file path
    // tower: { branch: 'candidate-branch' } // tower only; branch is optional
  },
  managed: {
    version: 1, runId: '', invocationId: 'invocation-id',
    role: 'execute', scope: 'packet-scope', candidateId: 'candidate-id',
    channel: 'subscription', taskDir: '/physical/project/docs/task'
  }
}
```

`invocationId` must equal `requestId`. `runId` is a required string and may be empty. Unknown Start/native/managed fields, permission other than explicit full access, invalid directories, Tower sessionId and `agents.maxConcurrent` are rejected. The current public API cannot enforce maxConcurrent as a hard limit on the entire native task tree. Swarm/Tower reject `agents.enabled:false`.

Every invocation starts foreground `kimi web --no-open --host 127.0.0.1 --port 0`; `FLYWHEEL_KIMI_BIN` may select the CLI. Account login and model configuration remain in `KIMI_CODE_HOME`, defaulting to `~/.kimi-code`; the adapter does not rewrite user configuration. It matches the new child PID, start time and loopback port in the real instance registry, then authenticates with the existing `server.token` and verifies `/meta`, authentication, backend and WebSocket support. Kimi 2.5.3 may emit different registry `server_id` and `/meta` `server_id` values; the adapter accepts that difference only when the registry `host_version` and `/meta` `server_version` are both non-empty and equal, retaining both ids in diagnostics. Without matching versions it still rejects. CLI startup stdout/stderr may contain the bearer token; raw bytes are neither forwarded nor included in receipts.

Admission reads and hashes actual `/openapi.json` and `/asyncapi.json`, checking required creation/configuration/queue/environment/history/shutdown routes and subscription/task lifecycle messages. Creation must expose title, ownership metadata and local environment fields; the returned new history must confirm its requested title, binding and workspace before any task prompt. A mismatched owner is not targeted by session-specific cleanup. Upstream OpenAPI does not fully project the `:abort` action dispatcher, so an actual idempotent abort probe runs on the owned idle session before dispatch. Actual WS agent state must then match model, thinking, permission and plan/swarm/tower flags, and REST must confirm the local execution directory. Only matching facts set `native.effective.matched:true`.

`--doctor` checks only CLI availability and version, returning `inferenceTested:false`. It does not establish mode, account, model or API admission. This interface does not use the obsolete `kimi server` command, an SDK, a named-preset RPC, or arbitrary agent-file loading.

## Execution, completion and cancellation

The worker subscribes to `main` before submission and correlates `prompt_id` with actual `user_message.prompt_ids`; HTTP acceptance and WS events may arrive in either order. The first ordinal is **0**, so fresh history has baseline **-1**. Task/agent_refs observations discover and subscribe child agents. Completion considers native tasks, queues, interactions and session busy state together. The first root turn ending is insufficient; Tower's automatic root continuations must also end.

Observed root/child item, step and lifecycle activity is forwarded as `event` metadata to Go's idle watchdog; reasoning text and tool arguments/results are not copied. High-frequency item/step progress is coalesced across the task tree, with an interval no greater than one second and derived from the effective parent `idleTimeoutMs` (a positive integer, default 300000). `event.coalescedTypes` identifies the progress kinds observed since the previous notification. Lifecycle boundaries and the terminal result are not throttled. Only actual events trigger progress notifications; timers and WebSocket ping/pong do not manufacture model activity. The wall deadline and cancellation/cleanup remain independent bounds.

`native.completion={confirmed,pendingIds}` is captured **before** cleanup. Success requires completed root turns, idle observed children/agents, an empty queue, no pending interaction and consistent final REST/WS reconciliation. Cancelling children during shutdown cannot convert incomplete work to success. Receipts retain actual task/child IDs, states and models.

Conversation observation bounds the retained JSON payload to 64 MiB, not cumulative protocol traffic. Replaced or deleted agent/task/turn state releases its previous allowance; incremental text accounts only for the new fragment. WebSocket updates and final HTTP turn reconciliation share this budget and the existing agent/turn/task count limits. Single-frame, HTTP-response and final-text limits remain separate. Repeated snapshots of the same bounded state do not exhaust an ever-growing traffic counter.

Unexpected approvals/questions return `INTERACTION_REQUIRED`; the adapter fabricates no answer. A lost socket, replaced conversation projection or unknown dispatch never triggers prompt replay. Cancellation withdraws queued prompts before aborting the root turn, stopping observed tasks and awaiting idle. It then disables remote control, shuts down only the owned runtime, and checks process exit and identities of observed descendants. Cleanup has a separate bounded grace. Unconfirmed queue, task or process cleanup leaves both `settled` and `cleanup.confirmed` false.

The worker inherits Go's process group and does not create a detached group. The Go watchdog retains final group cleanup ownership. `settled` means cleanup was confirmed, rather than role success: an error/cancelled result may also be settled=true.

## Results, usage and history

Terminal fields are `protocol/requestId/type/status/settled/sessionId/text/usage/costUsd/native/lifecycle`, plus `error.code/message` on failure. Status is ok/error/cancelled. `native.requested` preserves Start.native exactly; effective/observed contain actual admission facts, schema digests, history binding, current managed identity, children, completion, cleanup and any Tower candidate.

`lifecycle.protocol` is `kimi-server/v1`, with requestId/sessionId, dispatch (not-dispatched/dispatched/unknown), outcome (completed/failed/cancelled/unknown), cleanupConfirmed, resumeCandidate, resumeAccepted, usageStatus and turnId.

Usage comes only from four actual ended model-step counters in the public conversation projection. Complete accounting returns `{input,output,cacheRead,cacheWrite,totalTokens}`; input includes input_other and both cache input categories. Placeholder zeros in `session.usage` and WS context_tokens are not invocation billing. Missing steps, unidentified children, compaction or unscoped resumed child history yield usage=null, retaining attributable partial step facts. Tower whole-tree usage is currently always unknown; costUsd is always null. Go's existing unknown-budget policy remains authoritative.

Persistent Agent/Swarm history binds taskDir, role, scope, channel, physical cwd/workspace, model, effort, permission and native profile. runId/invocationId/candidateId belong to the current receipt and do not prevent same-role history reuse after restart or revision. Resumed history counts only current root turns; old child history is never charged wholesale again. Another role or configuration cannot reuse the history.

Tower is always fresh: explicit sessionId is rejected; persistent=true without sessionId may run, but resumeCandidate remains false. This interface has no durable recovery journal/ACK protocol: `recover` is unsupported, and a new start must not replay an invocation whose dispatch is unknown.

## Tower candidate and transfer

Tower receives a repository with independent `.git` administration, including when the source is a linked worktree. Source HEAD, all tracked/unignored content, existing dirty/untracked files, deletion, binary data, executable bits and symlinks are snapshotted. A taskDir outside the role worktree gets a separate external contract fingerprint; that external directory accepts regular files/directories only. Source and external contract together are bounded to 256 MiB; submodules and special files are rejected.

Only the actual task's changing runtime files are excluded: state.json and its atomic temporary files, metrics.jsonl, orchestrator.log/pid, locks, stop-request, `.control/`, `.sessions/` and `reports/native/`. Spec and packet files remain fingerprinted. Tower's own `.tower/` administration is not transferred.

Admission resolves cwd to its physical path while retaining the caller's declared directory alias. The submitted prompt maps bounded prefixes of both source cwd spellings to candidate cwd and adds candidate-directory instructions. Only original text is scanned; inserted candidate paths are never replaced again. Similar paths, URLs (including queries) and external contract paths are preserved. `observed.promptMapping` records the rule, replacement count, original/submitted prompt digests and the declared, physical and candidate directories. **This is an independent Git candidate directory, not an OS filesystem sandbox**: explicit full-access permission remains unchanged. Source content or HEAD changes prevent transfer.

Only successful native completion, confirmed cleanup and `managed.role==='execute'` permit trusted adapter transfer. It freezes a binary patch, checks the exact source/external contract again, verifies the patch digest, runs `git apply --check`, repeats source admission, applies and checks resulting content. Other roles return the candidate without applying it. Conflicts, changed contract/HEAD, modified patches or a failed postcondition fail the role while preserving the private repository, source snapshot, patch and receipt. There is no system-wide lock over unrelated editors and no claim that multi-file apply is an OS atomic transaction. Go still performs boundary checks, acceptance and commits.

## Validation evidence

`test/kimi-native.test.js` uses actual local HTTP/WS child processes to test all three modes, cross-channel ordering, native children, unknown usage, admission refusals, cancellation, history, CLI lifeline, one final result, token suppression and Tower failure gates. `test/kimi-tower-candidate.test.js` uses actual Git for dirty/untracked/binary/link/mode/deletion cases, linked worktrees, external contracts, candidate conflicts and prompt mapping. These tests contact neither a user account nor a model. CLI availability, real account admission and paid inference are separate evidence.

The implementation was checked against `MoonshotAI/kimi-code` commit `f082f04a093a3e71a2263c5461d52507dc31f83a`: [Server API](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api.md), [sessions/configuration](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-sessions.md), [conversation/task lifecycle](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-conversation.md), [Tower](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/guides/tower.md), [instance registry](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/kap-server/src/instanceRegistry.ts), [per-batch Swarm concurrency](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/agent-core-v2/src/features/swarm/session/agentRunBatch.ts), and [Tower rate limiting](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/agent-core-v2/src/features/tower/towerRateLimitService.ts). Actual installed runtime probes remain authoritative.

## Named print-session preparation

The same packaged worker accepts a separate `prepare-print` request with protocol,
requestId, cwd, model, reasoningEffort, sessionTitle and timeoutMs. It rejects prompt
and sessionId fields. The checked upstream print path submits turns below the TUI
first-prompt/title service and offers no title CLI option, so native auto-titling
is not assumed.

An owned authenticated local API runtime verifies the session-creation schema,
creates an empty named history with the caller's physical workspace, and confirms
its title, ownership metadata and idle state. It sets only the requested model,
`permission_mode:auto` and `plan_mode:false` to match fresh print behavior.
Thinking/profile/environment remain provider defaults; the same existing effort
environment override is passed to both preparation and print. Both use the same
KIMI_CODE_HOME. After confirmed physical cleanup, the original print command uses
`--session` with that new identity and the remaining original wall budget.

The single `prepared-print` receipt has requestId/status/sessionId/sessionTitle,
cleanupConfirmed, `inferenceDispatched:false` and error. Missing, mismatched or
unclean preparation prevents task dispatch. No title model is called, no business
prompt is sent during preparation, and failure does not retry an unknown task.
Existing histories skip preparation and retain their title. Native `start` also
accepts sessionTitle, setting it only during new session creation. The display
title never replaces the opaque session handle used for identity/recovery.

The public create/resume operations open a native session, and close ends it.
The first named print call therefore adds one preparation session startup and
shutdown before the ordinary print invocation. Configured `SessionStart` and
`SessionEnd` hooks run for that extra lifecycle as well. The business prompt is
still sent exactly once; an existing-history resume skips preparation.
Agent/Swarm set the title in their existing creation runtime and do not add this
preparation lifecycle. The adapter does not bypass upstream hooks or treat
naming as a side-effect-free metadata write.
