# Antigravity for DeepSeek Harness

[简体中文](README.zh-CN.md) · [Machine contract](docs/MACHINE.md) · [Compatibility and release checks](docs/VALIDATION.md)

Run Gemini tasks through the **official local `agy` CLI**, using its existing native account login. One supervised runner serves the Desktop task panel, the `subagent_antigravity` tool, and the public ESM machine API. It does not use ACP or call a separate model API.

This is the **0.2.0 release candidate**. The repository does not imply that an npm release, a real-account smoke test, or installation on your Mac has occurred. Offline protocol, process lifecycle, DSH contract and installed-package checks are reproducible below.

## Requirements and installation

- Node.js 22.18+ on macOS or Linux. Windows execution is rejected because this release supervises POSIX process groups.
- Official `agy` 1.5.7+ already installed and signed in by the user; Orchestrator and explicit subagents/Swarm require 1.5.12+. The adapter never downloads or replaces the CLI, copies tokens, or implements its own login.
- DSH with public commands, workingDirectory, conversation-view/settings slots and Agent tools. The published schema/PTC contract is checked against `0.2.2-rc.3`; host capabilities are checked when the plugin loads. Jobs are needed for background tool calls.
- Native account routing, no ADC/GCP/custom API models, and AI credits fallback disabled in the official CLI. See [subscription controls](#subscription-controls).

Build and check a candidate from this repository:

```sh
npm ci --ignore-scripts
npm run check
```

`npm run test:package` prints the exact candidate `.tgz` and integrity. The supported DSH installation entry is:

```sh
dsh plugin --profile <profile> add /absolute/path/dsh-subagent-antigravity-0.2.0.tgz
```

Use a disposable profile for candidate acceptance first. Registry installation by package name becomes appropriate only after that exact version is published. There are no install scripts and no automatic inference tests.

The bundle creates one `antigravity` plugin row. Agent delegation also requires the chosen preset to **explicitly register `subagent_antigravity`**; the plugin's `presets` setting is a second allowlist, not a way around preset tool selection. The direct panel/command works without asking a parent model to call a tool. Do not register this package as a conventional DSH LLM provider: `agy` already owns the Agent loop.

## Desktop use

Open the **Antigravity** conversation view. Enter the complete task, optional project directory, task name and exact model. Adjustable models expose effort; Orchestrator shows **Fixed by model**. Choose the native collaboration, execution and approval settings, then **Start task**. The view uses DSH's public command transport directly; it does not generate an assistant message or invoke the parent model.

Use **Refresh tasks**, **View result** and **Cancel** to manage the session's work. Task ownership includes the Desktop session, canonical workspace and invocation ID. Switching views keeps tasks running; disposing the plugin cancels owned live invocations. After a host restart, the same owner can read persisted receipts. A task without conclusive evidence remains `unknown`; a status read never starts a replacement run. An unknown task has no “retry automatically” behavior.

The panel shows the local label, conversation ID, requested model/effort/collaboration, observed binding, progress/result, errors and cleanup/provider-work evidence. **View result** on a running task shows its current observed child IDs and roles, with unknown child states retained. Native titles are not modified. **Check environment** runs only `--version`, `--help` and `models`; it cannot establish entitlement, remaining quota or answer quality.

Settings are available on the plugin row. Each invocation freezes its configuration. Saving command/default directory/model/effort/native collaboration/permissions/deadline/preset settings affects later calls. An explicit selection in a task takes precedence over saved defaults. Resume restores the original task's binding in the form; omitted native options on resume come from that owner's prior task, including an originally absent field, rather than a later saved collaboration default. Changing the binding is rejected by the runner.

The same direct interface is available as a DSH command:

```text
/antigravity {"version":1,"action":"start","label":"Investigate timeout","prompt":"Analyze the evidence below…","model":"gemini-4-neon-high","effort":"high","toolPolicy":"evidence-only"}
/antigravity {"version":1,"action":"list"}
/antigravity {"version":1,"action":"result","invocationId":"<exact returned UUID>"}
/antigravity {"version":1,"action":"cancel","invocationId":"<exact returned UUID>"}
```

`status`, `doctor` and `defaults` are also version-1 actions. Manual tasks use the plugin's owner-bound store; they do not fabricate an Agent-owned DSH job.

## Agent and PTC delegation

```js
subagent_antigravity({
  label: "Fix timeout regression",
  prompt: "Reproduce and fix the approved timeout defect, then run the regression test. …",
  model: "gemini-4-hydrogen-high",
  effort: "high",
  run_in_background: true
})
```

Background calls return `{kind:"background", jobId}` and use the host's `job_output` and `job_kill`. Foreground calls return text content containing a bounded task result, native observations and evidence identifiers. Tool calls cannot widen native permissions or choose a different owner workspace. They can choose a model/effort, `native` mode, exact prior session ID, deadline, JSON output schema, and `toolPolicy`.

Tool registration is scoped to allowed committed presets and rechecked at execution, including after preset changes. Both its input and output schemas are tested with DSH's published validator and PTC SDK generator.

## Orchestrator, subagents and Swarm

Model selection and collaboration are independent. The official [model guide](https://antigravity.google/docs/models) describes Orchestrator as a planner/implementer handoff within one agent; [Swarm](https://antigravity.google/docs/swarm/) requests parallel native members. Selecting Orchestrator alone does not assert a spawned team.

| Model | Effort value |
| --- | --- |
| `gemini-4-hydrogen-*`, `gemini-4-neon-*` | Matching `low`, `medium`, `high` or `xhigh` suffix |
| `gemini-4-carbon-*`, `gemini-4-argon-*` | Matching `low`, `medium` or `high` suffix |
| `gemini-pro-orchestrator-neon` | `""` — fixed; no `--effort` flag |
| `gemini-pro-orchestrator` (Carbon) | `""` — fixed; no `--effort` flag |
| `gemini-pro-orchestrator-argon` | `""` — fixed; no `--effort` flag |

An omitted `native` keeps the CLI's ordinary behavior. Explicit options have the same shape in DSH defaults, direct actions, Agent/PTC calls and the machine API:

```js
subagent_antigravity({
  prompt: "Implement the approved parser and UI changes in their separate file scopes; gather results and run the integration checks.",
  model: "gemini-pro-orchestrator",
  effort: "",
  native: { version: 1, mode: "swarm", maxObservedAgents: 4 }
})
```

| `native.mode` | Requested behavior and admission |
| --- | --- |
| `native` | Preserve autonomous CLI behavior; it can still use native children. Slash expansion remains disabled. |
| `subagents` | Ask the coordinator to delegate and collect results. `init.tools` must advertise `invoke_subagent` before any prompt is sent. Slash expansion remains disabled. |
| `swarm` | Send the adapter-owned `/swarm` command with user task text encoded as JSON data. Existing `disableSlashCommands:true` settings block the request; the adapter does not change them. |

Both explicit collaboration modes require observed child metadata. A normal root result without it fails with `collaboration-not-observed`. The receipt distinguishes the requested command from observed children; it does not invent a Swarm board, selected worker models or child terminal states. Any nonempty native `finished_reason`, including `print_timeout`, fails even when status is SUCCESS and exit is zero, following the official [headless/changelog contract](https://antigravity.google/docs/changelog?tab=cli).

`maxObservedAgents` is an optional integer from 1 to 8. It counts **all unique observed child IDs**, including verifier and monitor. Exceeding it stops this owned CLI invocation. Its receipt always says `hard:false`: this is an observation guard, not a native concurrency cap, a complete remote-tree limit or a quota guarantee. Whole-tree usage and child models/states remain unknown when the protocol does not report them. URI metadata is retained but never opened automatically.

Completion is scoped to **this CLI invocation**. Local cancellation supervises its owned process group; recovery reads its original journal. Neither performs a provider-wide child query or cancellation. Use clear independent tasks for collaboration, and retain a separate formal review where the outer workflow requires one; a native verifier is evidence from the same root invocation.

## Public machine API

```js
import { runMachine } from "dsh-subagent-antigravity/machine";

const receipt = await runMachine({
  protocol: 1,
  requestId: "task/design/attempt-1",
  cwd: "/absolute/project",
  prompt: "Design the approved system from this specification…",
  command: "agy",
  model: "gemini-4-carbon-high",
  effort: "high",
  executionMode: "plan",
  permissionMode: "request-review",
  sandbox: false,
  toolPolicy: "evidence-only",
  sessionMode: "new",
  sessionName: "Task design",
  timeoutMs: 600000,
  subscriptionPolicy: { version: 1, mode: "subscription-only" }
}, {
  signal: controller.signal,
  // An unused path below an existing private, owned, canonical directory:
  recoveryDirectory: "/absolute/private-runs/attempt-1",
  onLifecycle: async event => { /* persist caller acknowledgement */ }
});
```

`runtime.env`, when supplied, is the complete environment, not a merge with the current process. Supply `HOME` and `PATH` appropriate for the same native CLI account environment. Credentials and alternate API/cloud routing variables are filtered; explicit ADC selection is rejected. No global credentials file is read to construct this environment.

Use the [`./machine` contract](docs/MACHINE.md) for lifecycle acknowledgement, recovery, strict model binding, output schema and failure semantics. The package also exports `./client` and a wrapper:

```sh
antigravity-machine --capabilities
antigravity-machine --doctor < doctor-request.json
antigravity-machine --json < request.json
antigravity-machine --recover < recovery-request.json
```

These are **adapter flags**, not native `agy` flags. The wrapper emits one JSON envelope on stdout; progress does not pollute that channel.

## Evidence-only analysis

`toolPolicy:"evidence-only"` creates one uniquely named, owned workspace custom agent with `tools: []`, `mcpServers: []`, `subagent: false`, and `commandExecutionPolicy: off`, selected with official `--agent`. Before sending any user prompt, the adapter requires the real `init` to identify that agent and expose **an empty available-tools list**. Missing init, inherited/nonempty tools, absent or mismatched agent identity all fail closed with no prompt written. Subsequent tool/subagent events also fail the run.

Evidence-only rejects Orchestrator and `subagents`/`swarm` modes. It always disables slash expansion and spawning, including when used for an outer JEV decision. A prompt requesting tools cannot override the verified empty tool list.

This uses native capabilities plus observed admission; it does not assume that an empty YAML list will be interpreted consistently by every CLI version. The official [headless documentation](https://antigravity.google/docs/cli/headless/) defines `init.tools` as all available tools and states that `model`/`agent` are included when those flags are set. The [custom-agent documentation](https://antigravity.google/docs/subagents/) defines the permitted-tools allowlist and workspace agent discovery. A version that cannot provide these facts is not admitted for evidence-only execution.

Only the exact invocation-created file is cleaned up, after checking inode and content, including on cancellation/parent death. Existing agent files are never overwritten and global settings are never changed. Native automatic initialization, customization discovery and OS-level access are outside this tool allowlist; this is **not an OS sandbox**. For strict host isolation use an independently managed execution environment. Plain `plan` mode by itself is not evidence-only enforcement.

## Subscription controls

The adapter verifies **local billing-route controls**, not the commercial entitlement of the account. It rejects an API provider, ADC, GCP and custom API-model settings, filters alternate endpoint/key environments, checks `useG1Credits:false` (the documented default), and verifies that relevant settings have not changed before/after dispatch. It never falls back to an API or credits when auth/quota fails.

Official sources: [authentication and API/ADC routing](https://antigravity.google/docs/install/), [CLI settings and credit fallback](https://antigravity.google/docs/cli/reference/), and [AI credits](https://antigravity.google/docs/cli/commands/credits). `useG1Credits` means using credits after plan quota runs out; disabling it is the local fallback control. The adapter does not enable or edit it.

The receipt says `verified-local-controls` when those checks pass. Account identity/entitlement, remaining quota, external tools' billing and actual extra charges remain **unknown** where the official protocol does not expose them. A model catalog is not quota evidence. A native login failure or quota exhaustion becomes a failed invocation, with no alternative paid route. Token counters and estimated commercial cost are separate: missing cost is `null`, never a claim of zero dollars.

## Boundaries

- One invocation, one CLI process, one text turn; exact conversation UUID for resume. No global “latest” session, no persistent pool or prewarm claim.
- Resume is permitted only for adapter-owned, terminal, clean conversations with identical directory/model/effort/mode/permission/tool policy/native options/native settings. Concurrent or unresolved resumes are blocked across processes.
- Success requires a normal native successful terminal result without `finished_reason`, valid schema, matching observable configuration and confirmed local cleanup. Explicit collaboration also needs observed children. Exit zero alone is insufficient; native soft permission denials are failures.
- `--sandbox` can be unavailable on a machine. Requested sandbox, execution mode and effort are not falsely reported as observed when init omits them. Skipping tool approvals does not prove sandbox is off or every external path is writable.
- Native child tasks/models/usage can be incomplete; whole-tree usage and remote completion remain unknown. Observed total-child guards do not establish a hard tree budget.
- Input is text. No native image/PDF attachment transport or verified artifact-return channel is claimed.

See [validation](docs/VALIDATION.md) for what was actually tested and the remaining Mac/account release gates.
