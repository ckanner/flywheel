# Using native Agents, Teams, Swarm and LCU

[Documentation](../README.md) · English · [简体中文](../zh-CN/native-capabilities.md) · [Complete design](../plans/2026-11-13-native-harness-capabilities.en.md)

Flywheel can use each harness's native collaboration inside formal loops. Define `native-profiles`, then select a profile for the intended stage. Static profiles work with JEV disabled; existing bindings still determine model, account channel, permissions and role history.

This guide describes the implemented version. [Vendor records](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md) identify the actual companion artifacts. Check user-machine installation separately; the [0.34.0 release record](../validation/2026-11-14-release-0.34.0.md) identifies the published artifact and completed checks.

## 1. Choose the actual harness

| Harness | Capabilities inside loops | Requirements |
| --- | --- | --- |
| `dsh` | PTC, subagents, Team, Workflow, explicit Web and LCU | Local DSH and an actual PTC probe; LCU additionally needs the 0.5.0 peer and full access |
| `kimi` | Server Agent / Swarm / Tower | Logged-in Kimi, the current Server API and explicit full access |
| `codex-pro` | Native agents and child threads | Bundled Codex Pro 0.10.2, logged-in Codex CLI and matching sandbox/approval receipts |
| `claude-code-pro` | Agent definitions, Workflow and Advisor | Optional Claude Code Pro 0.7.0, compatible local Claude runtime, authorized authentication and explicit full access |

ChatGPT inside DSH still uses DSH's tool loop. It does not automatically acquire Codex app-server capabilities. The desktop `heavy` Agent Preset is not automatically inherited by an SDK worker; the worker uses the generator's shared PTC overlay policy and checks its actual runtime behavior.

### Desktop plugin defaults and PTC calls

Codex Pro and Claude Code Pro provide their own native capability settings pages. Open the corresponding plugin's configuration in the plugin manager to set its `native` defaults. The DSH PTC tools `subagent_codex` and `subagent_claude_code` read those defaults at the start of each call. A running call keeps its captured configuration; later saves affect subsequent calls.

| Ordinary tool's `native` argument | Meaning |
| --- | --- |
| Omitted | Read the plugin's current default; if no default exists, preserve the CLI's existing behavior |
| Object | Replace the complete native default for this call, without a field-by-field merge |
| `null` | Explicitly disable native collaboration for this ordinary call, rather than fall back to implicit CLI enablement |

Codex settings cover Agent enablement, concurrent child threads, child model/effort defaults and an optional V2 backend. Claude settings cover Agent definitions with optional per-agent model/effort, concurrency/depth, Workflow and Advisor; enabling Advisor requires its model. The tools expose corresponding per-call overrides. Native settings remain subject to the existing permission and model-selection policies. See [named Claude agent effort](claude-code-pro.md#set-effort-for-named-native-agents) for accepted values and the distinction from the parent stage's effort.

Here, `native` describes capabilities inside Codex/Claude. DSH can still orchestrate these tools through PTC. Independent native Review and actual interactive Teams use dedicated operations; saving ordinary Agent defaults does not launch those jobs.

These desktop defaults belong to the desktop plugins. A direct Flywheel `codex-pro` / `claude-code-pro` worker uses the fixed selected `native-profile`, without reading another DSH profile's live settings. Independent `/machine` imports do not load desktop UI or DSH services. Normal daily/heavy preset regeneration preserves separately saved plugin settings.

### Align independent worker dependencies

Desktop, global CLI workers and the selected SDK profile are separate Node resolution trees. Updating only the desktop does not update the others. Check the managed installation record (`~/.config/flywheel/installation.json`) for the actual Flywheel prefix; when using the optional Claude or LCU backend, install its exact peer in that same prefix. The following registry command requires Claude 0.7.0 to be published; until then, validate only the [reviewed source candidate](claude-code-pro.md#prepare-the-optional-dependency):

```sh
npm install --global --prefix /absolute/flywheel-node-prefix --ignore-scripts dsh-subagent-claude-code-pro@0.7.0 dsh-plugin-lcu@0.5.0
dsh plugin --profile sdk add --save-exact --ignore-scripts dsh-plugin-chatgpt@0.1.20
```

Replace `sdk` with the chosen `FLYWHEEL_DSH_PROFILE` if customized. Keep it separate from `desktop`. These commands align software dependencies, not model credentials, permissions or native-profile enablement. Run `flywheel doctor --json` afterward; import availability does not prove live native admission or model vision.

### Subscription-only Codex and Kimi admission

When `subscription-policy` is `{"version":1,"mode":"subscription-only"}`, the formal Flywheel workers inspect the native route before sending a task prompt. This is a local routing policy: it prevents Flywheel from selecting API-key/cloud providers or adding a paid fallback. It does not lock the account's billing settings, guarantee a remaining balance, or disable charges configured independently at the provider. Entitlement, quota and extra usage remain `unknown` unless an actual native observation establishes them.

**Codex Pro** keeps the published machine adapter as the owner of turns, journals and cleanup. A packaged CLI transport guard uses the official app-server JSON-RPC protocol in that same native process. Before thread start/resume and before a task turn, it calls `account/read` with `refreshToken:false` and `config/read` with the actual workspace. The account must report `type:"chatgpt"`, the effective provider must be built-in `openai`, and the new/resumed thread must confirm that provider. API login, custom/cloud providers, route/header/credential overrides, unknown inspection capabilities, and separately configured child-agent provider files are refused. Account changes away from ChatGPT terminate the owned transport. No login, logout, config write, credit purchase or API fallback is issued. `forced_login_method` is inspected, never set; the native CLI may log an incompatible account out when that setting is changed.

**Kimi** uses its owned local Server API and the following minimal profile:

```json
{
  "native-profiles": {
    "kimi-subscription-agent": {
      "harness": "kimi",
      "options": {"version":1,"mode":"agent","agents":{"enabled":false}}
    }
  }
}
```

Reference `"native-profile":"kimi-subscription-agent"` from the selected Kimi model profile or stage. The existing native permission contract still requires explicit `danger-full-access` and uses native `yolo`; it is not a read-only sandbox or the legacy print command's `auto` mode. Garden, batch and other managed roles can use the same profile. Under an explicit subscription policy, legacy print fails before session preparation or prompt dispatch and asks for a native profile; existing configurations without the policy keep their original behavior.

Kimi admission reads only the official redacted `GET /api/v1/providers` and `GET /api/v1/config` interfaces. It requires the connected `managed:kimi-code` OAuth provider, `has_api_key:false`, an official coding endpoint, and known model aliases without provider, endpoint or credential overrides; configured secondary aliases must also resolve to admitted models. It rechecks the same runtime before dispatch and before accepting completion. The provider catalog's key flag includes configured provider environment keys. The receipt states exactly these observed facts.

Kimi's redacted service configuration combines OAuth and API keys in one credential flag. Therefore every subscription task explicitly disables `WebSearch` and `FetchURL`, in addition to the native `Agent`/`AgentSwarm` and scheduled-work restrictions. Swarm, Tower, custom agent profiles and enabled child agents are refused for this policy because inheritance of these service restrictions is not exposed by the native contract. The redacted API does not expose every manually configured provider header; those hidden fields are not claimed as verified. No account credential file or raw provider-key export is read. The receipt preserves the service limitation and `extraUsage:"unknown"`.

These guards apply to **Flywheel's formal workers**. Calling another plugin's ordinary `subagent_codex`/`subagent_claude_code` tool from an independently running DSH parent does not inherit Flywheel's subscription policy, and an existing DSH parent may already have consumed its own model. Use the direct Flywheel task/workbench entry for this policy and configure any independent desktop parent separately.

Native contract references, checked 2026-11-20: [Codex app-server authentication and configuration](https://developers.openai.com/codex/app-server/), [Codex authentication](https://developers.openai.com/codex/auth/), [Kimi provider catalog and credential flags](https://github.com/MoonshotAI/kimi-code/blob/1ef139e6c32a01c7ac51a89fea6ae279c91bc276/packages/agent-core-v2/src/llm-adapter/model/catalog.ts), [Kimi credential resolution](https://github.com/MoonshotAI/kimi-code/blob/1ef139e6c32a01c7ac51a89fea6ae279c91bc276/packages/agent-core-v2/src/llm-adapter/model/catalog-service.ts), and [Kimi redacted configuration](https://github.com/MoonshotAI/kimi-code/blob/1ef139e6c32a01c7ac51a89fea6ae279c91bc276/packages/kap-server/src/routes/config.ts). Offline native-protocol fixtures establish guard behavior and cleanup; they are not live account, billing or model-quality acceptance.

## 2. Define profiles and bind them

Use `flywheel config inspect --cwd /absolute/repo --json` to locate the effective complete file. Flywheel selects one file from task, project, global or defaults; it does not merge fields between them.

Incorporate this **fragment** into that complete file, choosing the definitions you need:

```json
{
  "native-profiles": {
    "heavy-team": {
      "harness": "dsh",
      "options": {
        "version": 1, "preset": "heavy",
        "agents": {"enabled": true, "maxConcurrent": 4, "maxDepth": 2},
        "team": {"enabled": true},
        "workflows": {"enabled": true},
        "web": {"search": true, "fetch": true}
      }
    },
    "swarm": {
      "harness": "kimi",
      "options": {"version": 1, "mode": "swarm", "agents": {"enabled": true}}
    },
    "tower": {
      "harness": "kimi",
      "options": {"version": 1, "mode": "tower", "agents": {"enabled": true}}
    },
    "codex-team": {
      "harness": "codex-pro",
      "options": {"version": 1, "agents": {"enabled": true, "maxConcurrentThreads": 4, "backend": "v2"}}
    },
    "claude-flow": {
      "harness": "claude-code-pro",
      "options": {"version": 1, "agents": {"enabled": true}, "workflows": {"enabled": true}, "advisor": {"enabled": true, "model": "opus"}}
    }
  }
}
```

Add a `native-profile` reference to the selected stage while keeping its other settings. The following compatibility example retains an explicitly configured DeepSeek account route through DSH. It applies only without `subscription-only`; the current subscription default uses Antigravity for `routes.default` and rejects this paid DSH route:

```json
{
  "routes": {
    "default": {
      "harness": "dsh", "provider": "deepseek-account",
      "channel": "deepseek-account-sdk", "model": "deepseek-flash", "thinking": "high",
      "native-profile": "heavy-team"
    }
  }
}
```

References work in model `profiles`, `loops`, `routes`, `judge-panel` and `execution-policy.bindings`. The harness must match. Omission inherits a model profile's field; explicit `"native-profile":""` disables inheritance. Changing effective native options prevents incompatible role-history reuse.

The desktop Models and sessions form includes a Native capabilities selector, showing profiles for the current harness. Edit definitions in advanced JSON. Saves use the current file revision and apply on the next engine start. Selecting a profile expresses intent; runtime receipts establish admission.

## 3. Apply capabilities to a stage

Plan can use Team or Swarm to investigate, while its root produces formal Spec and packets. Execute can use agents, Workflow or Tower, with results subject to the existing verification and independent Review. Review and Judge use their own role histories and children, never Executor history. Intake/Garden can use the same capabilities for advice without writing state.

Native scheduling does not replace Go's decisions about retries, budgets, accepted candidates and delivery. A completed native task board is not a completed Flywheel task.

A native planning profile lets the worker observe child tasks and WS lifecycle; root/child item and step progress is forwarded to Go as metadata without reasoning or tool payloads. Plain Kimi print buffers assistant output, omits thinking deltas and does not expose background child progress. Its driver therefore uses the finite wall deadline and explicit cancellation without an output-idle timer. Foreground investigation keeps results within the non-native planning call but cannot prove liveness. Prefer native profiles when child lifecycle observation is needed; on either path, an unknown planning invocation can only be retired after cleanup is confirmed, not by switching backends or replaying. Transport ping/pong is not reported as model progress.

## 4. LCU and screenshots

Add this field to a DSH profile's options and explicitly select full access for the affected role:

```json
{
  "lcu": {
    "enabled": true,
    "config": {"chrome": true, "audio": false}
  }
}
```

`config` also accepts the runtime's `app`, `command`, `allowedApps` and `allowedOrigins`. Use the actual local runtime and authorized targets. The peer must resolve from the SDK worker's real Node dependency tree; installing it in a separate desktop profile is not proof that an independent worker can import it.

The worker acquires a desktop lease before mounting LCU. Root and child GUI operations are serialized. The PTC tool name is `lcu_cua`; program objects still use `cua`. Handles belong to the current runtime epoch and must be reacquired after restart. Unknown cleanup is not automatically displaced by another invocation; inspect the original owner and actual runtime rather than deleting a lock to manufacture success.

Images and a manifest persist under `<taskDir>/reports/native/<invocationId>/`. Archive hashes identify actual final AttachmentStore bytes; capture hashes are separate. An archived image does not prove model delivery, which may remain `unverified`. ChatGPT provider 0.1.20 fixes real image projection into requests, but this work has not performed live visual acceptance on the user's account.

Public Pro machine APIs support explicit media and structured-output inputs. Formal Go loops do not automatically inject every archived image into the next turn or treat an external schema/native review text as their verdict contract.

## 5. Tower candidate directories

Tower works in an independent Git copy including current dirty/untracked/binary/symlink/mode/deletion state. It is not an OS sandbox and currently requires full access. Exact execution-directory prefixes are mapped in the prompt with boundaries and recorded digests.

Only Execute attempts transfer after completion and cleanup. Transfer rechecks source HEAD, bytes/modes, external task contracts and binary patch applicability. Concurrent source or Spec/packet changes retain the candidate and return a conflict; other roles retain artifacts without applying them. Normal verification, Review and Git delivery still follow.

Native Kimi never reads or writes the legacy `plan-session` cache. Agent/Swarm
use role-ledger history; an explicit legacy `resident-session:true` can retain
its reuse intent unless `sessions.roles.plan:"fresh"` explicitly overrides it.
Tower always uses fresh history and does not resume an old Session. Kimi Server has no proven whole-tree `agents.maxConcurrent` hard cap; omit that field. An explicit request for it is rejected before dispatch. See the [Kimi native protocol](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/KIMI_NATIVE_PROTOCOL.en.md).

## 6. Independent native Review and interactive Team

These operations return their own native job receipts rather than formal loop verdicts:

```sh
flywheel native codex-review --request /absolute/codex-review.json
flywheel native claude-review --request /absolute/claude-review.json
flywheel native claude-team --request /absolute/claude-team.json
flywheel native claude-recover --request /absolute/recovery.json
flywheel native claude-control --request /absolute/control.json
```

Requests use the pinned public package API plus a private absolute `recoveryDirectory`. Input must be a regular JSON file of at most 1 MiB. A minimal Codex example is:

```json
{
  "protocol": 1, "requestId": "review-001", "cwd": "/absolute/repo",
  "model": "gpt-6.1-sol", "effort": "high",
  "prompt": "Review the selected changes.", "permissionMode": "read-only", "threadMode": "ephemeral",
  "native": {"version": 1, "review": {"target": {"type": "uncommittedChanges"}}},
  "recoveryDirectory": "/absolute/private/native-review-001"
}
```

A remote Claude Review request needs `protocol:1`, a unique requestId, absolute cwd, `consent:true` and recoveryDirectory. It runs `ultrareview --json --no-post`. Local timeout does not settle the remote job; use the original requestId, runId and recoveryDirectory to read the original local journal and retrieve its durable receipt. This neither queries cloud status nor resubmits Review.

Interactive Team accepts the public `runInteractiveTeam` prompt and permissionMode and requires tmux. `claude-control` supports capture/send/resize/cancel/attach with the original requestId/runId and recoveryDirectory. Full fields are in the [Claude native API](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/d14ff8d8334fefd039c3d48ee2a969747d1e88c8/docs/native-capabilities.md).

Claude `recoverNativeJob` only reads the local durable journal. It does not start a process or query the cloud. Receipts explicitly retain `jobId:null`, `querySupported:false` and `cancelSupported:false`. A local recovery status of terminal does not establish remote completion: unknown remote state or unconfirmed native cleanup still forbids replay or a claim of cloud cancellation.

## 7. Inspect availability and failures

```sh
flywheel doctor --cwd /absolute/repo --json
flywheel doctor --cwd /absolute/repo --probe --json
```

Native diagnostics inspect local workers, exact public manifests and capability versions. They do not invoke a model, start a Team/cloud job or operate the GUI. A DSH native profile does not use a legacy SDK probe as native admission evidence. Formal worker admission must prove actual PTC, model and target behavior. `native.admission` remains unknown; `inferenceTested:false` and `admissionTested:false` do not mean a failed login attempt.

`native-receipt` retains intent, effective configuration, observations, children, usage, completion and cleanup. Native events are not durable lifecycle ACKs. Active children at root completion cannot become success merely because shutdown later cancels them. Unknown cleanup, damaged receipts and uncertain recovery continue to block progress.

For Codex, observed child turns and thread closure take precedence over delayed
creation snapshots (`spawnAgent` and V2 started markers). A completed child is
not reopened by an older running snapshot, and a newer running turn is not
completed by an older terminal snapshot. A genuine new turn or later queued
follow-up still needs its own completion evidence.

This also covers a child whose terminal state was observed only in an owned
parent snapshot, before any child turn ID. A repeated running creation snapshot
cannot undo that terminal evidence. Missing turn identity and usage remain
unknown; completion does not supply either.
Later queued work does not restore that old snapshot's authority to finish the
new work.

Concurrency and budgets retain their actual scope: Flywheel scheduling limits root invocations; Claude limits are an observed guard with `hard:false`; Kimi lacks that whole-tree hard cap. Unattributable external-agent, Advisor, compaction or Tower usage remains unknown, and hard budgets do not count it as zero. See [validation](../validation/2026-11-14-native-harness-implementation.md) for tested and untested runtime boundaries.

See the [desktop configuration follow-up](../validation/2026-11-14-desktop-native-configuration.md) for plugin settings, typed PTC arguments and this candidate's package validation.
