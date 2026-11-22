# Flywheel 文档

[English](README.md) · 简体中文 · [项目介绍](../README.zh-CN.md)

先完成安装，再按工作流指南运行第一个任务；配置或排查具体运行时，查阅相应参考。用户指南和维护指南均提供英文与简体中文版本。

## 使用 Flywheel

| 主题 | 简体中文 | English |
| --- | --- | --- |
| 安装、配置 provider、完成首个任务 | [快速开始](zh-CN/getting-started.md) | [Getting started](en/getting-started.md) |
| 处理故障、修复 Issue、推进复杂项目 | [使用手册](zh-CN/usage-handbook.md) | [Usage handbook](en/usage-handbook.md) |
| 方案讨论、独立审查、精确批准与逐步细化 | [方案设计与审查](zh-CN/reviewed-planning.md) | [Reviewed planning](en/reviewed-planning.md) |
| 准备契约、选择 Auto/Quick/Full、交付变更 | [工作流](zh-CN/workflow.md) | [Workflow](en/workflow.md) |
| 故障调查、修复授权与有界语义决策 | [有界调查](zh-CN/investigation.md) | [Investigation](en/investigation.md) |
| 使用画布、阶段操作、会话和运行记录 | [流程工作台](zh-CN/flow-console.md) | [Workbench](en/flow-console.md) |
| 配置优先级、profiles、权限和调度 | [配置参考](zh-CN/configuration.md) | [Configuration](en/configuration.md) |
| 默认模型与推理等级的依据 | [模型策略](zh-CN/model-policy.md) | [Model policy](en/model-policy.md) |
| 使用原生 Agent、Team、Swarm、Tower 与 LCU | [原生能力](zh-CN/native-capabilities.md) | [Native capabilities](en/native-capabilities.md) |
| 使用 Claude 原生订阅后端 | [Claude Code Pro](zh-CN/claude-code-pro.md) | [Claude Code Pro](en/claude-code-pro.md) |
| 在任务执行中传入冻结的参考图片 | [图片输入](zh-CN/image-inputs.md) | [Image inputs](en/image-inputs.md) |
| 官方 CLI 的 Gemini 与独立 DSH 插件 | [Antigravity](zh-CN/antigravity.md) | [Antigravity](en/antigravity.md) |
| 角色续接、调用中断与对账 | [恢复与上下文质量](zh-CN/session-recovery.md) | [Recovery and context quality](en/session-recovery.md) |
| 使用固定任务和证据比较会话策略 | [会话评估](zh-CN/session-evaluation.md) | [Session evaluation](en/session-evaluation.md) |
| 通过一个开关启用有界 JEV 决策 | [JEV 决策](zh-CN/jev.md) | [JEV decisions](en/jev.md) |

[首个任务脚本](examples/create-first-task.sh)用于创建隔离的教程仓库；准备和执行的区别见快速开始指南。

## 开发与维护

| 主题 | 简体中文 | English |
| --- | --- | --- |
| 开发环境、质量门禁和 PR 流程 | [贡献指南](zh-CN/contributing.md) | [Contributing](en/contributing.md) |
| 文档归属、双语更新和审阅清单 | [文档维护规范](zh-CN/contributing.md#保持文档结构清晰) | [Maintenance rules](en/contributing.md#keep-documentation-organized) |
| 单包构建、验证、发布和恢复 | [发行手册](zh-CN/releasing.md) | [Release runbook](en/releasing.md) |
| DSH 包、宿主兼容和 worker 接入 | [插件说明](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.zh-CN.md) | [Plugin README](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.md) |

[架构规范](design.md)是当前维护的设计契约，以中文编写。[Worker 协议](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md)、[schema 与模板](https://github.com/ckanner/flywheel/tree/main/templates)及[仓库约定](https://github.com/ckanner/flywheel/blob/main/AGENTS.md)定义实现接口；版本变化见 [CHANGELOG](../CHANGELOG.md)。

## 设计决策与证据

以下记录保留原日期和语言。提案不代表其中所有能力都已交付；验收记录只对文中明确的源码、平台和检查负责。

[订阅模型、CLI 插件与工作流方案](plans/2026-11-19-subscription-cli-workflows.md)
（[English](plans/2026-11-19-subscription-cli-workflows.en.md)）保留已批准设计。
当前源码实现四订阅默认表、官方 Antigravity CLI 插件、紧凑准备、明确批准策略及累计
调查轮次。当前行为见[模型策略](zh-CN/model-policy.md)、[方案设计与审查](zh-CN/reviewed-planning.md)
和[有界调查](zh-CN/investigation.md)。源码及候选验收与 registry 发行、真实账号权益分别记录。
源码检查、候选身份和验证边界见[实施及复审记录](validation/2026-11-20-subscription-cli-workflows.md)。
[编排与首轮选择增补](validation/2026-11-20-orchestration-adaptive-selection.md)记录
Antigravity 0.2.1 的 Orchestrator／子代理／Swarm、受审查协作范围、JEV 首轮选择、
共享恢复预算和独立复核。

[真实设计、独立评审与分层规划方案](plans/2026-11-16-reviewed-planning.md)（中文，原始提案）记录 Plan 设计、独立审查、精确版本批准与逐步细化的方案。当前控制入口与兼容边界见[方案设计与审查](zh-CN/reviewed-planning.md)；真实 Provider 验收与源码测试分别记录。

[反馈驱动工作流与 JEV 方案](plans/2026-11-12-feedback-driven-workflows.md)（中文，历史提案）系统讨论 Auto／Quick／Full、故障调查、短反馈闭环、模型策略和有界语义决策；当前支持的行为见[调查指南](zh-CN/investigation.md)与 [JEV 指南](zh-CN/jev.md)。[P5 实施计划](plans/2026-11-13-p5-ready-queue-prewarm.md)记录可选调度与运行时准备，真实性能和默认采用策略另需证据。

[原生 Harness、PTC 与 LCU 完整设计](plans/2026-11-13-native-harness-capabilities.md)（[English](plans/2026-11-13-native-harness-capabilities.en.md)）于 2026-11-14 修订为六仓库实现设计，涵盖共享 SDK/PTC 组合、原生 Agent/Team/Swarm/Workflow、Tower 候选、GUI lease、媒体、用量和恢复。配置方法见[原生能力指南](zh-CN/native-capabilities.md)，已测源码和安装边界见[验收记录](validation/2026-11-14-native-harness-implementation.md)。

| 分类 | 内容 |
| --- | --- |
| [方案](plans/README.md) | 按日期保存的设计提案与实施决策 |
| [验收记录](validation/README.md) | 测试、审查、发行和桌面验收证据 |
| [历史评审](reviews/README.md) | 代码评审及当时的问题清单 |
| [评估数据](evaluation/README.md) | 固定实验输入和已记录结果 |
| [归档](archive/README.md) | 已被替代的设计及集成交接文档 |
| [演练](https://github.com/ckanner/flywheel/blob/main/drills/README.md) | 可复现的故障与验证演练 |

## 文档组织约定

| 位置 | 用途 |
| --- | --- |
| `README.md` / `README.zh-CN.md` | 简明项目入口，默认英文 |
| `docs/en/` / `docs/zh-CN/` | 对应的用户指南和维护指南 |
| `docs/design.md` | 当前架构与设计不变量 |
| `docs/plans/`、`docs/validation/`、`docs/reviews/` | 带日期的决策与证据 |
| `docs/archive/` | 不再决定当前默认行为的历史材料 |
| `plugins/dsh/` | 插件专属说明和协议参考 |

行为变化应更新所属章节；版本变化放入 CHANGELOG，验收证据放入对应记录，避免在指南开头或末尾追加无关段落。旧 `README.en.md` 只保留兼容跳转，不再维护重复正文。
按[文档维护规范](zh-CN/contributing.md#保持文档结构清晰)确定本次改动需要同步的文档，并在 PR 前核对清单。AI agent 还须遵循 [AGENTS.md 操作规则](https://github.com/ckanner/flywheel/blob/main/AGENTS.md)。

本仓库的项目文档与 Flywheel 在**目标仓库**中生成的 `docs/<task>/` 任务契约、运行账本是两类内容。

