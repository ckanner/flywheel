# Using native Agents, Teams, Swarm and LCU

[Documentation](../README.md) · English · [简体中文](../zh-CN/native-capabilities.md) · [Complete design](../plans/2026-11-13-native-harness-capabilities.en.md)

The Flywheel 0.34.0 implementation candidate can use each harness's native collaboration inside formal loops. Define `native-profiles`, then select a profile for the intended stage. Static profiles work with JEV disabled; existing bindings still determine model, account channel, permissions and role history.

This guide describes source candidates. [Vendor records](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md) identify the actual companion artifacts. Version numbers here do not imply registry publication or installation on the user's machine.

## 1. Choose the actual harness

| Harness | Capabilities inside loops | Requirements |
| --- | --- | --- |
| `dsh` | PTC, subagents, Team, Workflow, explicit Web and LCU | Local DSH and an actual PTC probe; LCU additionally needs the 0.5.0 peer and full access |
| `kimi` | Server Agent / Swarm / Tower | Logged-in Kimi, the current Server API and explicit full access |
| `codex-pro` | Native agents and child threads | Bundled Codex Pro 0.10.0, logged-in Codex CLI and matching sandbox/approval receipts |
| `claude-code-pro` | Agent definitions, Workflow and Advisor | Optional Claude Code Pro 0.5.0, logged-in Claude CLI and explicit full access |

ChatGPT inside DSH still uses DSH's tool loop. It does not automatically acquire Codex app-server capabilities. The desktop `heavy` Agent Preset is not automatically inherited by an SDK worker; the worker uses the generator's shared PTC overlay policy and checks its actual runtime behavior.

### Desktop plugin defaults and PTC calls

Codex Pro and Claude Code Pro provide their own native capability settings pages. Open the corresponding plugin's configuration in the plugin manager to set its `native` defaults. The DSH PTC tools `subagent_codex` and `subagent_claude_code` read those defaults at the start of each call. A running call keeps its captured configuration; later saves affect subsequent calls.

| Ordinary tool's `native` argument | Meaning |
| --- | --- |
| Omitted | Read the plugin's current default; if no default exists, preserve the CLI's existing behavior |
| Object | Replace the complete native default for this call, without a field-by-field merge |
| `null` | Explicitly disable native collaboration for this ordinary call, rather than fall back to implicit CLI enablement |

Codex settings cover Agent enablement, concurrent child threads, child model/effort defaults and an optional V2 backend. Claude settings cover Agent definitions, concurrency/depth, Workflow and Advisor; enabling Advisor requires its model. The tools expose corresponding per-call overrides. Native settings remain subject to the existing permission and model-selection policies.

Here, `native` describes capabilities inside Codex/Claude. DSH can still orchestrate these tools through PTC. Independent native Review and actual interactive Teams use dedicated operations; saving ordinary Agent defaults does not launch those jobs.

These desktop defaults belong to the desktop plugins. A direct Flywheel `codex-pro` / `claude-code-pro` worker uses the fixed selected `native-profile`, without reading another DSH profile's live settings. Independent `/machine` imports do not load desktop UI or DSH services. Normal daily/heavy preset regeneration preserves separately saved plugin settings.

### Align independent worker dependencies

Desktop, global CLI workers and the selected SDK profile are separate Node resolution trees. Updating only the desktop does not update the others. Check the managed installation record (`~/.config/flywheel/installation.json`) for the actual Flywheel prefix; when using the optional Claude or LCU backend, install its exact peer in that same prefix:

```sh
npm install --global --prefix /absolute/flywheel-node-prefix --ignore-scripts dsh-subagent-claude-code-pro@0.5.0 dsh-plugin-lcu@0.5.0
dsh plugin --profile sdk add --save-exact --ignore-scripts dsh-plugin-chatgpt@0.1.20
```

Replace `sdk` with the chosen `FLYWHEEL_DSH_PROFILE` if customized. Keep it separate from `desktop`. These commands align software dependencies, not model credentials, permissions or native-profile enablement. Run `flywheel doctor --json` afterward; import availability does not prove live native admission or model vision.

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

Add a `native-profile` reference to the selected stage while keeping its other settings. For example, the default Execute route can retain DeepSeek account access through DSH:

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

Concurrency and budgets retain their actual scope: Flywheel scheduling limits root invocations; Claude limits are an observed guard with `hard:false`; Kimi lacks that whole-tree hard cap. Unattributable external-agent, Advisor, compaction or Tower usage remains unknown, and hard budgets do not count it as zero. See [validation](../validation/2026-11-14-native-harness-implementation.md) for tested and untested runtime boundaries.

See the [desktop configuration follow-up](../validation/2026-11-14-desktop-native-configuration.md) for plugin settings, typed PTC arguments and this candidate's package validation.
