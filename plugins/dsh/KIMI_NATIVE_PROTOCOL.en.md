# Kimi native execution protocol

[中文](KIMI_NATIVE_PROTOCOL.md)

`kimi-native-worker.js` and `runKimiWorker()` implement native Kimi Agent, Swarm and Tower execution for the Go engine. The ordinary Kimi CLI path is unchanged. Go owns role loops, budgets, boundary checks, Review, commits and acceptance; native children belong to one role invocation.

## Entry and admission

The worker receives one UTF-8 JSONL `start` on stdin, which the parent keeps open. EOF, SIGINT, SIGTERM and SIGHUP request cancellation. stdout contains only protocol 1 `ready`, `event` and exactly one terminal `result`. The bridge emits ready/event and returns the final result; the CLI writes it once. Requests are bounded to 1 MiB and output frames to 2 MiB.

```js
{
  type: 'start', protocol: 1, requestId: 'invocation-id',
  cwd: '/physical/role-worktree', workspaceCwd: '/physical/project',
  model: 'configured-model-alias', reasoningEffort: 'high',
  permissionMode: 'danger-full-access', prompt: 'Role instructions', timeoutMs: 600000,
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

Every invocation starts foreground `kimi web --no-open --host 127.0.0.1 --port 0`; `FLYWHEEL_KIMI_BIN` may select the CLI. Account login and model configuration remain in `KIMI_CODE_HOME`, defaulting to `~/.kimi-code`; the adapter does not rewrite user configuration. It matches the new child PID, start time and loopback port in the real instance registry, then authenticates with the existing `server.token` and verifies `/meta` server_id, authentication, backend and WebSocket support. CLI startup stdout/stderr may contain the bearer token; raw bytes are neither forwarded nor included in receipts.

Admission reads and hashes actual `/openapi.json` and `/asyncapi.json`, checking required creation/configuration/queue/environment/history/shutdown routes and subscription/task lifecycle messages. Upstream OpenAPI does not fully project the `:abort` action dispatcher, so an actual idempotent abort probe runs on the owned idle session before dispatch. Actual WS agent state must then match model, thinking, permission and plan/swarm/tower flags, and REST must confirm the local execution directory. Only matching facts set `native.effective.matched:true`.

`--doctor` checks only CLI availability and version, returning `inferenceTested:false`. It does not establish mode, account, model or API admission. This interface does not use the obsolete `kimi server` command, an SDK, a named-preset RPC, or arbitrary agent-file loading.

## Execution, completion and cancellation

The worker subscribes to `main` before submission and correlates `prompt_id` with actual `user_message.prompt_ids`; HTTP acceptance and WS events may arrive in either order. The first ordinal is **0**, so fresh history has baseline **-1**. Task/agent_refs observations discover and subscribe child agents. Completion considers native tasks, queues, interactions and session busy state together. The first root turn ending is insufficient; Tower's automatic root continuations must also end.

`native.completion={confirmed,pendingIds}` is captured **before** cleanup. Success requires completed root turns, idle observed children/agents, an empty queue, no pending interaction and consistent final REST/WS reconciliation. Cancelling children during shutdown cannot convert incomplete work to success. Receipts retain actual task/child IDs, states and models.

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

The submitted prompt maps bounded source cwd path prefixes to candidate cwd and adds candidate-directory instructions. Similar paths, URLs and external contract paths are preserved. `observed.promptMapping` records the rule, replacement count, original/effective prompt digests and both directories. **This is an independent Git candidate directory, not an OS filesystem sandbox**: explicit full-access permission remains unchanged. Source content or HEAD changes prevent transfer.

Only successful native completion, confirmed cleanup and `managed.role==='execute'` permit trusted adapter transfer. It freezes a binary patch, checks the exact source/external contract again, verifies the patch digest, runs `git apply --check`, repeats source admission, applies and checks resulting content. Other roles return the candidate without applying it. Conflicts, changed contract/HEAD, modified patches or a failed postcondition fail the role while preserving the private repository, source snapshot, patch and receipt. There is no system-wide lock over unrelated editors and no claim that multi-file apply is an OS atomic transaction. Go still performs boundary checks, acceptance and commits.

## Validation evidence

`test/kimi-native.test.js` uses actual local HTTP/WS child processes to test all three modes, cross-channel ordering, native children, unknown usage, admission refusals, cancellation, history, CLI lifeline, one final result, token suppression and Tower failure gates. `test/kimi-tower-candidate.test.js` uses actual Git for dirty/untracked/binary/link/mode/deletion cases, linked worktrees, external contracts, candidate conflicts and prompt mapping. These tests contact neither a user account nor a model. CLI availability, real account admission and paid inference are separate evidence.

The implementation was checked against `MoonshotAI/kimi-code` commit `f082f04a093a3e71a2263c5461d52507dc31f83a`: [Server API](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api.md), [sessions/configuration](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-sessions.md), [conversation/task lifecycle](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-conversation.md), [Tower](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/guides/tower.md), [instance registry](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/kap-server/src/instanceRegistry.ts), [per-batch Swarm concurrency](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/agent-core-v2/src/features/swarm/session/agentRunBatch.ts), and [Tower rate limiting](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/agent-core-v2/src/features/tower/towerRateLimitService.ts). Actual installed runtime probes remain authoritative.
