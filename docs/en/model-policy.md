# Reasoning effort and account selection

[中文](../model-policy.md) · English

Checked on 2026-11-05. This note separates documented capabilities, adapter behavior, engineering recommendations, and measurements. Official model documentation and request paths have been reviewed. **No representative Flywheel comparison of high, xhigh, and max has been run**, and no live user-account inference or quota check was performed. The defaults are editable starting points, not a measured optimum.

## Why most bindings use high

Seven of the eight stage/route bindings use `high`; Intake uses `low`. High matches the documented defaults for Kimi K3.1 Pro and DeepSeek Flash. GPT-6.1 Sol's API default is medium, so Flywheel explicitly requests a higher level. Astra's model page lists five levels without stating a universal API default. Sources: [Kimi models](https://www.kimi.com/code/docs/en/kimi-code/models), [DeepSeek thinking](https://api-docs.deepseek.com/guides/thinking_mode/), [Sol](https://developers.openai.com/api/docs/models/gpt-6.1-sol), and [Astra](https://developers.openai.com/api/docs/models/gpt-6-astra).

High provides an initial comparison baseline without starting every loop at maximum effort. More reasoning can help difficult tasks while adding latency and usage; it does not guarantee a better delivery on every task. OpenAI's [deployment checklist](https://developers.openai.com/api/docs/guides/deployment-checklist) recommends representative evaluation before adopting xhigh/max for their additional cost and latency. That supports the evaluation method, not a claim that this project's defaults have already been benchmarked.

## Effort values differ by backend

| Current path | Checked capability | Flywheel request |
|---|---|---|
| GPT-6.1 Sol / Codex Pro 0.8.0 | Model supports `low/medium/high/xhigh/max`; machine API forwards effort | hard: high |
| Sol and Astra / DSH ChatGPT provider | Models and checked provider catalog support `low/medium/high/xhigh/max`; serialized as `reasoning.effort` | Review and Judge: high |
| Kimi K3.1 Pro / Kimi CLI | Native `low/high/max`, default high; thinking cannot be disabled | Plan, frontend, Garden: high |
| DeepSeek Flash / DSH rc.5 | DSH accepts `off/low/high/max`, default high | ordinary execution: high; Intake: low |

GPT supports xhigh and max; their absence from the defaults is a policy choice, not an interface limitation. These names are not a shared scale across providers. Kimi compatibility aliases xhigh/max/ultra resolve to native max. DeepSeek's generic API maps xhigh to high, while the DSH adapter used here rejects xhigh before that mapping because it accepts only the four listed values. Use max when explicitly selecting Flash's higher supported level. See the native model documentation above and DSH's [configuration](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek/src/config.ts) and [serialization](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek/src/serialize.ts).

The ChatGPT provider snapshot is `388868f91bcdc05795fb94d21b868ded6d7219c9` (0.1.17): [dated model measurements](https://github.com/ckanner/dsh-plugin-chatgpt/blob/388868f91bcdc05795fb94d21b868ded6d7219c9/src/models/served.ts) and [request serialization](https://github.com/ckanner/dsh-plugin-chatgpt/blob/388868f91bcdc05795fb94d21b868ded6d7219c9/src/convert/request.ts). A catalog is not a live entitlement check for the user's account. The Codex Pro plugin name does not establish a separate server reasoning mode.

## Stages worth comparing first

These are **engineering recommendations inferred from stage responsibilities**, not measured performance results. This correction preserves effort choices; it adds no automatic escalation, effort-changing retry, or budget policy.

| Stage | Baseline | Candidate comparison |
|---|---|---|
| Intake | Flash low | high for ambiguous input or difficult merging/classification |
| Plan | Kimi high | max for architecture, dependency, or acceptance-design problems |
| Execute default | Flash high | max for difficult implementation or root-cause analysis; not xhigh |
| Execute frontend | Kimi high | max for complex interaction state or cross-component consistency |
| Execute hard | Sol high | prioritize xhigh comparisons; max for the hardest tasks |
| Review | Sol high | xhigh for high-impact concurrency, consistency, or permission reviews |
| Judge | Astra high | prioritize xhigh for high-impact milestone acceptance; max for the hardest cases |
| Garden | Kimi high | low for routine organization; max for difficult cross-task synthesis |

Hard execution tackles difficult implementation. Judge independently examines a whole milestone after L2/L3, making these two stages useful first comparisons. Changing every stage together makes the cause of any improvement harder to identify. OpenAI's [reasoning guide](https://developers.openai.com/api/docs/guides/reasoning) describes effort by task difficulty; applying that guidance to Flywheel's stages remains an inference.

Change `thinking` in the effective complete configuration, inspect it, and restart the engine. Use `routes.hard.thinking` for hard only and `loops.judge.thinking` for Judge only. Stage overrides avoid changing every consumer of a shared profile. A task file with only those two fields is not a complete configuration and does not inherit the rest of the project file.

For an actual comparison, hold the model, backend, context, task, and acceptance conditions fixed. Measure delivery success, first-pass acceptance, missed defects and false alarms, total time including rework, and input/output/reasoning/cache usage. Record observable subscription quota separately; unknown values remain unknown. API prices do not directly determine subscription task capacity.

## Request evidence is not quality evidence

Source and offline protocol tests establish field transmission and error behavior. SDK configuration acceptance or a successful machine-API request establishes acceptance at that interface. Independent effective-effort evidence and improved delivery quality require server evidence or actual task measurements.

DSH `initialize` validates configuration but returns only serverInfo. Flywheel's `ready.reasoningEffort` echoes the request after initialization; it is not independent model-server attestation. Codex Pro separates `requestedEffort`, `accepted`, and unknown `effectiveEffort:null`. See the [DSH SDK server](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sdk/server/src/server.ts) and [Codex Pro 0.8.0 protocol](https://github.com/ckanner/dsh-subagent-codex-pro/blob/04ecdb677735d84f3a0f5dba01a9d9b28c324c0e/docs/machine.md). A no-inference doctor cannot establish account balance, real inference, or higher-effort benefits.

## The funded DeepSeek account is the intended default

Current source selects `deepseek-account` for Flash in defaults and the subscription preset. Standard desktop and SDK profiles can reuse account authorization under the same DSH home; an additional API key is unnecessary. `deepseek-official` remains a separate, explicitly chosen API-key provider, not an automatic fallback.

Desktop login can initialize an account-model default, but independent SDK calls still need explicit provider/model values. Flywheel requests `deepseek-account` and `deepseek-flash`, preserving both the selected access method and the Flash-only choice. Authentication or quota failures are reported. The default preset has no cross-channel fallback; explicit user-defined fallback chains retain their existing policy.

**Release and migration are separate:** 0.28.3 changes the embedded default to account access; 0.28.2 still embeds the API default. Upgrades preserve user configuration, so adoption requires 0.28.3 or later followed by explicit migration. See [Configuration](configuration.md#deepseek-account-and-api-key-providers) for credential-sharing conditions and the preview/apply workflow. No user credentials or running tasks were modified by this source change.

## Optional Claude Code Pro

Claude remains unassigned in the default model policy. The optional `claude-code-pro` harness accepts an explicit native model and low/medium/high/xhigh/max request; an installed CLI or account can reject a choice. No new model ranking or effort benchmark is implied. See [the adapter guide](claude-code-pro.md) for the companion 0.3.2 public API, full-access boundary, and usage semantics.
