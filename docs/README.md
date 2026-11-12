# Flywheel documentation

English · [简体中文](README.zh-CN.md) · [Project overview](../README.md)

Start with installation, follow the workflow for a first task, and use the
reference guides when configuring or investigating a run. User and maintainer
guides are maintained in English and Simplified Chinese.

## Use Flywheel

| Guide | English | 简体中文 |
| --- | --- | --- |
| Install, configure providers, and complete a first task | [Getting started](en/getting-started.md) | [快速开始](zh-CN/getting-started.md) |
| Prepare contracts, choose Auto/Quick/Full, and deliver changes | [Workflow](en/workflow.md) | [工作流](zh-CN/workflow.md) |
| Use the canvas, stage controls, sessions, and run history | [Workbench](en/flow-console.md) | [流程工作台](zh-CN/flow-console.md) |
| Resolve configuration, profiles, permissions, and scheduling | [Configuration](en/configuration.md) | [配置参考](zh-CN/configuration.md) |
| Understand the default models and reasoning settings | [Model policy](en/model-policy.md) | [模型策略](zh-CN/model-policy.md) |
| Enable the optional Claude machine backend | [Claude Code Pro](en/claude-code-pro.md) | [Claude Code Pro](zh-CN/claude-code-pro.md) |
| Resume roles and reconcile interrupted calls | [Recovery and context quality](en/session-recovery.md) | [恢复与上下文质量](zh-CN/session-recovery.md) |
| Compare session policies with fixed tasks and evidence | [Session evaluation](en/session-evaluation.md) | [会话评估](zh-CN/session-evaluation.md) |
| Run the optional, disabled-by-default JEV experiment | [JEV shadow evaluation](en/jev-shadow.md) | [JEV 影子评估](zh-CN/jev-shadow.md) |

The [first-task script](examples/create-first-task.sh) creates an isolated
tutorial repository. The getting started guide explains preparation and
execution separately.

## Develop and maintain Flywheel

| Reference | English | 简体中文 |
| --- | --- | --- |
| Development setup, quality gates, and pull requests | [Contributing](en/contributing.md) | [贡献指南](zh-CN/contributing.md) |
| Documentation ownership, bilingual updates, and review checklist | [Maintenance rules](en/contributing.md#keep-documentation-organized) | [文档维护规范](zh-CN/contributing.md#保持文档结构清晰) |
| Build, verify, publish, and recover a six-package release | [Release runbook](en/releasing.md) | [发行手册](zh-CN/releasing.md) |
| DSH package, host compatibility, and worker setup | [Plugin README](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.md) | [插件说明](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.zh-CN.md) |

The [architecture specification](design.md) is the maintained design contract
and is written in Chinese. The [worker protocol](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md),
[schemas and templates](https://github.com/ckanner/flywheel/tree/main/templates), and [repository instructions](https://github.com/ckanner/flywheel/blob/main/AGENTS.md)
define the implementation interfaces. For changes over time, read the
[changelog](../CHANGELOG.md).

## Design decisions and evidence

These records preserve their original date and language. A proposal is not a
claim that every proposed feature shipped; a validation record applies only
to its named source, platform, and checks.

| Collection | Contents |
| --- | --- |
| [Plans](plans/README.md) | Dated design proposals and implementation decisions |
| [Validation](validation/README.md) | Test, review, release, and desktop acceptance records |
| [Reviews](reviews/README.md) | Historical code reviews and their findings |
| [Evaluation data](evaluation/README.md) | Fixed experiment inputs and recorded results |
| [Archive](archive/README.md) | Superseded designs and integration handoffs |
| [Drills](https://github.com/ckanner/flywheel/blob/main/drills/README.md) | Reproducible failure and verification exercises |

## Documentation structure

| Location | Purpose |
| --- | --- |
| `README.md` / `README.zh-CN.md` | Concise project entry points; English is the default |
| `docs/en/` / `docs/zh-CN/` | Paired user and maintainer guides |
| `docs/design.md` | Current architecture and design invariants |
| `docs/plans/`, `docs/validation/`, `docs/reviews/` | Dated decisions and evidence |
| `docs/archive/` | Historical material that must not set current defaults |
| `plugins/dsh/` | Plugin-specific documentation and protocol reference |

Update the section that owns a behavior. Keep release history in the
changelog and dated evidence in validation records, rather than adding
unrelated paragraphs to guide introductions or endings. The old
`README.en.md` files remain compatibility links, not duplicate manuals.
Use the [maintenance rules](en/contributing.md#keep-documentation-organized) to
map each change to its affected documents and check the final PR. AI agents also
follow the [repository operating rules](https://github.com/ckanner/flywheel/blob/main/AGENTS.md).

This repository's documentation is distinct from the `docs/<task>/` contract
and runtime ledger that Flywheel creates in a **target repository**.
