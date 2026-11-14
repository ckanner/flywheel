# Models, reasoning effort, and accounts

[Documentation](../README.md) · English · [简体中文](../zh-CN/model-policy.md)

Flywheel selects a model through a stage or execution-route binding. The
[default template](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.config.example.json) is the source
of truth for new configurations; an existing installation keeps its own
configuration. These defaults are an editable baseline. The repository does not
establish that they are the optimum for representative tasks, or that increasing
reasoning effort improves every delivery.

## Default bindings

| Stage or route | Harness | Model | Access path | Effort |
|---|---|---|---|---|
| Plan | `kimi` | `kimi-code/k3.1-pro` | Kimi Code subscription through the native CLI | `high` |
| Execute: default | `dsh` | `deepseek-flash` | DSH `deepseek-account` | `high` |
| Execute: frontend | `kimi` | `kimi-code/k3.1-pro` | Kimi Code subscription through the native CLI | `high` |
| Execute: hard | `codex-pro` | `gpt-6.1-sol` | Codex login through the public machine API | `high` |
| Review | `dsh` | `gpt-6.1-sol` | DSH `chatgpt` provider | `high` |
| Judge | `dsh` | `gpt-6-astra` | DSH `chatgpt` provider | `high` |
| Intake | `dsh` | `deepseek-flash` | DSH `deepseek-account` | `low` |
| Garden | `kimi` | `kimi-code/k3.1-pro` | Kimi Code subscription through the native CLI | `high` |

The harness owns the agent/tool loop. A DSH `provider` selects its native LLM
adapter, while a `channel` names the configured account/credential path. A
`codex-pro` harness is not a DSH provider called `subagent_codex_pro`. Kimi uses
its native CLI, not ACP. Auto, Quick, and Full do not change these bindings or
increase effort automatically; see [Configuration](configuration.md).

Claude Code Pro is available as an [optional backend](claude-code-pro.md), with
no default stage or route assigned. Its profile uses an explicit native model
and effort rather than an implicit replacement for another subscription.

## Account selection

The default and `subscriptions` preset use `deepseek-account` with
`deepseek-flash`. Standard DSH desktop and SDK profiles under the same DSH home
can share account authorization without an additional API key. Custom profiles
must load the account service and adapter. Independent SDK calls still supply
an explicit provider/model; changing the desktop chat model does not update
Flywheel's bindings.

`deepseek-official` is a separate API-key provider, selected only by explicit
configuration. The default preset does not switch payment channels after an
authentication or quota failure. User-configured fallback chains retain their
own policy. ChatGPT and Kimi subscriptions likewise require the corresponding
installed adapter or CLI and its supported login flow.

Upgrades preserve existing user files. To adopt the subscription preset, inspect
the effective complete configuration, preview the migration, then explicitly
apply it as described in [Configuration](configuration.md). Restart the engine
afterward; neither installing packages nor changing a desktop chat model
migrates a running task.

## Reasoning effort is backend-specific

The following capabilities were checked for the documented integrations on
**2026-11-05**. They are compatibility evidence for those snapshots, not a live
catalog or an account-entitlement check.

| Checked integration | Accepted request values | Notes |
|---|---|---|
| GPT-6.1 Sol / Codex Pro 0.10.0 | `low`, `medium`, `high`, `xhigh`, `max` | The machine API forwards the selected effort |
| Sol and Astra / DSH ChatGPT provider | `low`, `medium`, `high`, `xhigh`, `max` | The provider serializes `reasoning.effort` |
| Kimi K3.1 Pro / native CLI | `low`, `high`, `max` | Thinking cannot be disabled; compatibility aliases `xhigh`, `max`, and `ultra` resolve to native `max` |
| DeepSeek Flash / checked DSH adapter | `off`, `low`, `high`, `max` | This adapter rejects `medium` and `xhigh` |

Names do not form a common scale across providers. In particular, a generic
API's compatibility mapping does not override the DSH adapter's own input
validation. Use `max`, not `xhigh`, when evaluating Flash's higher supported
level on this path. The adapter must reject an unsupported selection rather
than silently choose another model or effort.

Compatibility references: [Kimi model documentation](https://www.kimi.com/code/docs/en/kimi-code/models),
[DeepSeek thinking](https://api-docs.deepseek.com/guides/thinking_mode/),
[Sol model reference](https://developers.openai.com/api/docs/models/gpt-6.1-sol),
[Astra model reference](https://developers.openai.com/api/docs/models/gpt-6-astra),
[DSH DeepSeek configuration](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek/src/config.ts),
and [serialization](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek/src/serialize.ts).
The checked ChatGPT provider is 0.1.17 at `388868f91bcdc05795fb94d21b868ded6d7219c9`:
[model catalog](https://github.com/ckanner/dsh-plugin-chatgpt/blob/388868f91bcdc05795fb94d21b868ded6d7219c9/src/models/served.ts)
and [request serialization](https://github.com/ckanner/dsh-plugin-chatgpt/blob/388868f91bcdc05795fb94d21b868ded6d7219c9/src/convert/request.ts).

## Choose and evaluate a baseline

Seven of the eight default bindings use `high`; Intake uses `low`. Keeping a
stable starting point makes comparisons attributable to a particular stage.
The following candidates are engineering judgments based on stage
responsibilities, not measured Flywheel quality results.

| Stage | Candidate comparison |
|---|---|
| Intake | Compare `high` for ambiguous input, merging, or classification |
| Plan | Compare `max` for difficult architecture, dependencies, or acceptance design |
| Execute: default | Compare Flash `max` for difficult implementation or root-cause analysis |
| Execute: frontend | Compare Kimi `max` for complex interaction state or cross-component consistency |
| Execute: hard | Compare Sol `xhigh` first; evaluate `max` on the hardest tasks |
| Review | Compare `xhigh` for concurrency, consistency, or permission reviews |
| Judge | Compare Astra `xhigh` for high-impact milestone acceptance, then `max` where justified |
| Garden | Compare `low` for routine organization and `max` for difficult cross-task synthesis |

Change `thinking` in the effective complete configuration. For example, change
`routes.hard.thinking` to affect only hard execution, or
`loops.judge.thinking` to affect only Judge. A stage override avoids changing all
consumers of a shared profile. Do not create an incomplete task configuration
containing only these fields: configuration discovery selects one file rather
than merging files.

Hold the model, harness, context, task, permissions, and acceptance conditions
fixed. Measure delivery success, first-pass acceptance, missed defects and false
alarms, total time including rework, and input/output/reasoning/cache usage.
Record observable subscription quota separately and preserve unknown values.
API prices do not directly determine subscription task capacity. The
[session evaluation guide](session-evaluation.md) describes the repository's
paired experiment tooling; effort comparisons need their own fixed conditions.

## Bounded model and effort experiments

The optional [execution policy](configuration.md#bounded-execution-policy-opt-in)
declares complete allowed bindings per route, including account, effort,
permission and history mode. Auto/Quick/Full do not enable it. A confirmed
repeated L1 stall may advance the declared experiment order within existing
attempt/token budgets and an explicit switch limit. This deterministic rule
does not prove the next model is better. Channel errors use separately declared
fallbacks; neither mechanism may select an undeclared model or payment channel.

When the single [JEV switch](jev.md) is enabled, a confirmed failure can also
offer the already allowed bindings as decision candidates. The engine checks
the frozen allowlist, permission, remaining attempts and switch budget before
adoption. No extra JEV model-routing switch is required. Without an allowed
alternative, the configured binding stays in use. A semantic choice is recorded
as a choice, not as measured proof that the selected model is faster or better.

Effort changes cold-start the role and record complete before/after bindings.
Comparisons must include rereading, startup and actual known/unknown usage and
cost. Offline tests run real engine repair and recovery paths with controlled
driver results; they do not establish live model quality or speed. Use matched
real tasks with frozen oracles before changing defaults or claiming savings.

## Interpret configuration and quality evidence

Source and offline protocol tests establish field transmission and error
behavior. SDK or machine-API acceptance establishes acceptance at that interface.
Independent effective-effort evidence and better delivery quality require
backend evidence or actual task measurements.

DSH `initialize` validates configuration but returns only server information.
Flywheel's `ready.reasoningEffort` echoes the request after initialization;
it is not independent model-server attestation. Codex Pro separates
`requestedEffort`, `accepted`, and an unknown `effectiveEffort:null`.
See the [DSH SDK server](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sdk/server/src/server.ts)
and [Codex Pro machine protocol](https://github.com/ckanner/dsh-subagent-codex-pro/blob/9c7d724cb19dc29457a56f4a4269c24278595d2f/docs/machine.md).
A no-inference doctor cannot prove account balance, live inference, or the
benefit of a higher effort setting.
