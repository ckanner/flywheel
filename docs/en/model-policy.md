# Models, reasoning effort, and subscriptions

[Documentation](../README.md) · English · [简体中文](../zh-CN/model-policy.md)

The [default template](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.config.example.json) uses Gemini, Claude, Codex and Kimi through native subscription clients. It is an editable engineering baseline, not a measured ranking or an account-entitlement guarantee. Existing configurations retain their bindings until explicit migration. The [approved proposal](../plans/2026-11-19-subscription-cli-workflows.en.md) records the research and rationale; this guide describes the implementation.

## Default bindings

| Stage or route | Profile | Harness | Exact model | Effort |
| --- | --- | --- | --- | --- |
| System design | `carbon` | `antigravity` | `gemini-4-carbon-high` | `high` |
| System design review | `astra` | `codex-pro` | `gpt-6.2-astra` | `high` |
| Milestone / implementation planning | `opus-plan` | `claude-code-pro` | `claude-opus-5-5` | `high` |
| Plan review | `sol-plan` | `codex-pro` | `gpt-6.2-sol` | `medium` |
| Compact design and single-packet plan | `hydrogen-local` | `antigravity` | `gemini-4-hydrogen-medium` | `medium` |
| Independent compact joint review | `sol-plan` | `codex-pro` | `gpt-6.2-sol` | `medium` |
| Execute: default | `hydrogen` | `antigravity` | `gemini-4-hydrogen-high` | `high` |
| Execute: complex | `neon` | `antigravity` | `gemini-4-neon-high` | `high` |
| Execute: frontend / multimodal | `opus-ui` | `claude-code-pro` | `claude-opus-5-5` | `medium` |
| Execute: backend | `sol` | `codex-pro` | `gpt-6.2-sol` | `high` |
| Execute: batch | `kimi` | `kimi` native agent | `kimi-code/k3.1-pro` | `high` |
| Execute: hard | `astra` | `codex-pro` | `gpt-6.2-astra` | `high` |
| Code Review | `sol` | `codex-pro` | `gpt-6.2-sol` | `high` |
| Milestone Judge | `astra` | `codex-pro` | `gpt-6.2-astra` | `high` |
| Intake | `hydrogen-summary` | `antigravity` | `gemini-4-hydrogen-low` | `low` |
| Investigation analysis | `neon` | `antigravity`, evidence-only | `gemini-4-neon-high` | `high` |
| Garden | `kimi` | `kimi` native agent | `kimi-code/k3.1-pro` | `high` |
| Optional JEV decisions | `hydrogen-local` | `antigravity`, evidence-only | `gemini-4-hydrogen-medium` | `medium` |

JEV is **off by default**. The additional `fable` profile provides `claude-fable-5-2 / high`, and `carbon-explore` provides `gemini-4-carbon-medium / medium`. Neither is an automatic fallback. Select them through an explicit binding. Native model unavailability fails visibly rather than selecting another paid route.

The approved proposal initially assigned JEV to Sol medium. This implementation
uses Hydrogen medium because the Antigravity adapter can verify an empty native
tool list before dispatch. The current Codex adapter does not expose an equivalent
evidence-only admission contract. This is a capability constraint, not a claim
that Hydrogen makes better decisions; JEV remains off unless explicitly enabled.

Routine implementation and short decisions use a smaller Gemini profile, Carbon handles system design, and Neon handles difficult implementation and evidence analysis. Independent review uses Codex; Claude handles plan refinement and interface work; Kimi handles declared batch work and task synthesis. These stage-specific choices have not been proven optimal in live Flywheel A/B tests.

`multimodal` selects the Claude route. A packet's versioned `input-images` contract supplies actual PNG/JPEG/GIF/WebP bytes through Execute → worker → Claude's public machine API → Agent SDK. Each repository-relative reference binds its MIME type and SHA-256; the engine validates the file, decoding and bounds before dispatch and retains its identity for verification and recovery. Up to four images are allowed, at most 4 MiB each and 8 MiB combined. Text filenames and DSH chat attachments are not imported automatically. The independent default Review checks code, acceptance and frozen references; it does not claim to see those images. Antigravity's first adapter accepts text only. See [image inputs](image-inputs.md), [Claude](claude-code-pro.md) and [Antigravity](antigravity.md).

## Initial selection and failure adaptation

The defaults also provide complete execution candidates while retaining the initial bindings above. `jev.enabled` remains off. Each route uses an empty `quality-order` and `max-switches:2`, so JEV-off defaults do not automatically walk the candidate list. With JEV enabled, the first Execute and confirmed failure boundaries can select among candidates admitted by Go; this is not reclassification before every stage.

| Route | Initial binding and allowed alternatives |
| --- | --- |
| default | Hydrogen high / Hydrogen medium / Orchestrator Neon |
| complex | Neon high / Orchestrator Neon / Neon high Swarm |
| frontend, multimodal | Opus medium / Opus high |
| backend | Sol high / Sol medium / Astra high |
| batch | Kimi high / Kimi max / Neon high Swarm |
| hard | Astra high / Carbon high / Orchestrator Carbon |

Swarm is eligible only with Full, no images, and a reviewed packet `collaboration` split whose child write scopes are disjoint and remain within approved scope. Ordinary subagents, fixed two-model Orchestrator, and Swarm are different strategies. All three Orchestrator options are manually selectable; see [Antigravity](antigravity.md) for exact identifiers. Their fixed `thinking:""` omits `--effort`; a selector cannot add high/xhigh. Image candidates retain only Claude bindings with actual image transport.

Changing model, effort, or native configuration freezes the complete binding, cold-starts, and records the reason while sharing existing attempt, switch, and decision budgets. JEV does not inspect live plan balances or assume measured candidate quality or speed. These alternatives are not a global model ranking. A sole legal option uses the initial baseline; unknown execution or cleanup reconciles first. See [JEV](jev.md).

## Planning roles and the current conversation

New/Task record a brief with repository facts. The engine owns design and review. `design-granularity:compact` requires concrete evidence, a reason and no explicit Full request. It creates one design plus one packet together, followed by one independent joint review of their exact versions. Uncertain scope, architecture, migrations and multi-part work use `system`; the engine may upgrade compact preparation without resetting authority or budgets.

`planning.compact` selects its two preparation bindings. System preparation uses `loops.design-plan`, `loops.design-review`, `loops.plan` and `loops.plan-review`. Legacy omissions retain the documented inheritance; `config inspect` reports the binding source. Source roles retain their own permissions and session scopes.

Auto chooses Quick or Full implementation from the reviewed contract. It is not a third execution chain or an approval policy. `review-first` stops for exact-version approval; `delegated` continues under retained implementation authority within its envelope. Compact tasks can also be review-first. See [Reviewed planning](reviewed-planning.md) and [Workflow](workflow.md).

## Account selection

The preset declares `subscription:{version:1,mode:"subscription-only"}`. Reachable loops, routes, compact bindings, Judge panel, execution alternatives and fallback channels must use the four admitted native harnesses. Unused custom definitions can remain in a file without becoming fallbacks. The default has no fallback and no DeepSeek API or account route.

| Harness | Admission evidence | Evidence limit |
| --- | --- | --- |
| Antigravity | Native account route; API/ADC/GCP/custom models rejected; credits fallback disabled; observed model and local controls checked | Does not attest entitlement, quota, external-tool charges or a whole-run billing lock |
| Claude Code Pro 0.7.0 | Same official CLI process confirms a supported first-party subscription and extra usage disabled before the prompt; alternate API/gateway routes rejected | Pre-prompt snapshot, not an atomic billing lock; success also requires the observed root model |
| Codex Pro | Same official app-server checked with `account/read` and `config/read`; ChatGPT login and native OpenAI routing required | Does not establish remaining quota or actual subscription charges |
| Kimi native agent | Official redacted providers/config APIs confirm managed Kimi Code OAuth and no API-key route; separate search/fetch and subagents disabled | Unexposed fields, including custom headers, are not attested; no whole-account billing guarantee |

Kimi uses `kimi-subscription-agent`: native agent mode, Agents disabled. Strict calls disable `WebSearch` and `FetchURL`, whose separate credential billing cannot be verified with the public account API. Kimi print is rejected before strict subscription dispatch; configurations without this policy retain legacy behavior.

Adapters use installed native clients and supported login. They do not copy tokens or create API accounts. Auth/quota errors stop the call without enabling credits, extra usage or API fallback. Doctor checks local compatibility without inference; live account access remains separate acceptance.

Preview and explicitly apply `flywheel config migrate --preset subscription-cli` as described in [Configuration](configuration.md). Migration preserves permissions, verification, budgets and scheduling. A retained role-based policy may need an explicit authorized change before full-access native bindings can run. The historical `subscriptions` preset remains a compatibility migration; it does not select this new table.

## Reasoning effort is backend-specific

| Integration | Adapter selections | Interpretation |
| --- | --- | --- |
| Gemini Hydrogen / Neon | `low`, `medium`, `high`, `xhigh` | Exact model suffix must agree with `thinking` |
| Gemini Carbon / Argon | `low`, `medium`, `high` | `xhigh` is rejected, not remapped |
| Orchestrator Neon / Carbon / Argon | Fixed, `thinking:""` | Named planner and Hydrogen hand off in one conversation; no `--effort` |
| Codex Pro | `low`, `medium`, `high`, `xhigh`, `max` | Specific model and native-client support still govern acceptance |
| Claude Code Pro | `low`, `medium`, `high`, `xhigh`, `max` | Explicit native model/effort; no automatic model fallback |
| Kimi K3.1 Pro | Native `low`, `high`, `max` | Compatibility aliases do not form a cross-provider scale |

Gemini choices follow the approved research, Google's [model-selection guide](https://aistudio.google.com/docs/choosing-a-model) and [Antigravity CLI reference](https://antigravity.google/docs/cli/reference/). The installed catalog establishes current invocation availability. Parameter acceptance is separate from independent server evidence of effective reasoning strength; receipts preserve that distinction.

## Choose and evaluate a baseline

Hold model, harness, task, context, permission and acceptance fixed when comparing effort. Measure first-pass acceptance, missed defects, false alarms, total time including rework and known/unknown usage. Subscription prices do not determine per-task capacity; API-equivalent estimates are not subscription charges. Antigravity conversation counters cannot safely be summed as per-call usage, so normalized totals remain unknown.

Change a stage binding to affect only that stage; a shared profile affects all consumers. Configuration selects one complete file without deep merging. Do not create a partial task file to change one effort field.

## Bounded model and effort experiments

The opt-in [execution policy](configuration.md#bounded-execution-policy-opt-in) declares complete allowed bindings per route and a finite switch budget. Confirmed repeated L1 stalls may advance the declared order within original attempt/token limits. JEV, when enabled, selects only from that same frozen allowlist. Neither mechanism creates permissions, expands scope, resets counters or enables undeclared accounts. Unknown calls must settle before switching.

## Interpret configuration and quality evidence

Source tests establish field transmission and error behavior; candidate installation establishes the packaged boundary. Neither proves live model quality, quota or account access. CLI acceptance, independently observed settings and improved delivery quality are separate evidence. Missing usage or price never means zero. The [session evaluation guide](session-evaluation.md) describes paired offline engine experiments and their limits.
