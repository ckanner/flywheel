# 模型、推理等级与账号

[文档目录](../README.zh-CN.md) · [English](../en/model-policy.md) · 简体中文

Flywheel 通过阶段或执行路由的绑定选择模型。[默认模板](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.config.example.json)
是新配置的依据；已有安装继续使用自己的配置。下面的默认值是可调整的初始基线，
仓库尚未证明它是代表性任务上的最优组合，也没有证明提高推理等级能改善每次交付。

## 默认绑定

| 阶段或路由 | Harness | 模型 | 访问方式 | 推理等级 |
|---|---|---|---|---|
| Plan | `kimi` | `kimi-code/k3.1-pro` | 原生 CLI 使用 Kimi Code 订阅 | `high` |
| Execute：default | `dsh` | `deepseek-flash` | DSH `deepseek-account` | `high` |
| Execute：frontend | `kimi` | `kimi-code/k3.1-pro` | 原生 CLI 使用 Kimi Code 订阅 | `high` |
| Execute：hard | `codex-pro` | `gpt-6.1-sol` | 公开 machine API 使用 Codex 登录 | `high` |
| Review | `dsh` | `gpt-6.1-sol` | DSH `chatgpt` provider | `high` |
| Judge | `dsh` | `gpt-6-astra` | DSH `chatgpt` provider | `high` |
| Intake | `dsh` | `deepseek-flash` | DSH `deepseek-account` | `low` |
| Garden | `kimi` | `kimi-code/k3.1-pro` | 原生 CLI 使用 Kimi Code 订阅 | `high` |

Harness 负责智能体与工具循环；DSH 的 `provider` 选择原生 LLM 适配器；`channel`
是配置中的账号或凭证渠道名称。`codex-pro` harness 不是名为 `subagent_codex_pro`
的 DSH provider。Kimi 使用原生 CLI，不使用 ACP。Auto、Quick、Full 不会自动改变
这些绑定或提高推理等级；具体配置方式见[配置指南](configuration.md)。

Claude Code Pro 是[可选后端](claude-code-pro.md)，默认未分配任何阶段或路由。
使用时明确指定原生模型与推理等级，不会隐式替换其他订阅渠道。

## 账号选择

默认配置和 `subscriptions` 预设使用 `deepseek-account` 与 `deepseek-flash`。
标准 DSH 桌面与 SDK 在同一 DSH home 下可以共享账号授权，无需额外 API key。
自定义 profile 仍须加载账号服务和适配器。独立 SDK 调用会明确传递 provider/model；
修改桌面对话模型不会更新 Flywheel 的阶段绑定。

`deepseek-official` 是单独的 API-key provider，需要明确配置。默认预设不会在
认证失败或额度不足后更换付费渠道；用户自定义后备链继续遵循其配置。
ChatGPT 与 Kimi 订阅同样需要相应适配器或 CLI，以及其支持的登录流程。

升级会保留已有用户文件。采用订阅预设时，先检查实际生效的完整配置、预览迁移，
再按[配置指南](configuration.md)显式应用。之后重新启动引擎；安装包或修改桌面
对话模型都不会迁移运行中的任务。

## 推理等级取决于后端

下表是 **2026-11-05** 对所列集成的能力核对，属于相应快照的兼容性依据，
不是当前服务端模型目录或账号权限检查。

| 已核对的集成 | 可请求的值 | 说明 |
|---|---|---|
| GPT-6.1 Sol／Codex Pro 0.10.0 | `low`、`medium`、`high`、`xhigh`、`max` | machine API 传递指定 effort |
| Sol、Astra／DSH ChatGPT provider | `low`、`medium`、`high`、`xhigh`、`max` | provider 序列化为 `reasoning.effort` |
| Kimi K3.1 Pro／原生 CLI | `low`、`high`、`max` | 不能关闭 thinking；兼容别名 `xhigh`、`max`、`ultra` 都映射到原生 `max` |
| DeepSeek Flash／已核对的 DSH adapter | `off`、`low`、`high`、`max` | 此适配器拒绝 `medium` 和 `xhigh` |

不同 provider 的同名档位不是统一量尺。通用 API 的兼容映射也不能绕过 DSH
适配器自身的参数校验。在这条路径比较 Flash 的更高档位时使用 `max`，不填
`xhigh`。后端不支持某项选择时应明确拒绝，不能静默替换模型或推理等级。

兼容性参考：[Kimi 模型文档](https://www.kimi.com/code/docs/en/kimi-code/models)、
[DeepSeek thinking](https://api-docs.deepseek.com/guides/thinking_mode/)、
[Sol 模型参考](https://developers.openai.com/api/docs/models/gpt-6.1-sol)、
[Astra 模型参考](https://developers.openai.com/api/docs/models/gpt-6-astra)、
[DSH DeepSeek 配置](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek/src/config.ts)
与[序列化校验](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek/src/serialize.ts)。
ChatGPT provider 的核对快照为 0.1.17、`388868f91bcdc05795fb94d21b868ded6d7219c9`：
[模型目录](https://github.com/ckanner/dsh-plugin-chatgpt/blob/388868f91bcdc05795fb94d21b868ded6d7219c9/src/models/served.ts)
与[请求序列化](https://github.com/ckanner/dsh-plugin-chatgpt/blob/388868f91bcdc05795fb94d21b868ded6d7219c9/src/convert/request.ts)。

## 选择并评估基线

八个默认阶段／路由中七个使用 `high`，Intake 使用 `low`。稳定的初始设置方便
把比较结果归因到具体阶段。以下比较方向来自阶段职责的工程判断，**不是已实测的
Flywheel 质量结论**。

| 阶段 | 可比较的设置 |
|---|---|
| Intake | 对有歧义的输入、合并或分类比较 `high` |
| Plan | 对困难架构、依赖关系或验收设计比较 `max` |
| Execute：default | 对困难实现或根因分析比较 Flash `max` |
| Execute：frontend | 对复杂交互状态或跨组件一致性比较 Kimi `max` |
| Execute：hard | 先比较 Sol `xhigh`，再在最难任务上评估 `max` |
| Review | 对并发、一致性或权限审查比较 `xhigh` |
| Judge | 对高影响里程碑终验比较 Astra `xhigh`，有依据时再评估 `max` |
| Garden | 常规整理比较 `low`，困难跨任务归纳比较 `max` |

在实际生效的完整配置中修改 `thinking`。例如，`routes.hard.thinking` 只影响
hard 执行，`loops.judge.thinking` 只影响 Judge。阶段覆盖可以避免改变共享 profile
的所有使用者。不要创建只包含这两个字段的不完整任务配置：配置发现选择一个文件，
不会跨文件合并。

固定模型、harness、上下文、任务、权限和验收条件，再比较交付成功率、首轮通过、
漏检与误报、包含返工的总耗时，以及输入／输出／reasoning／缓存用量。可观察的
订阅额度另记，缺失值保留 unknown。API 单价不能直接换算成订阅可完成任务数。
[会话评估指南](session-evaluation.md)介绍仓库的配对实验工具；推理等级比较也需要
明确自己的固定条件。

## 有界模型与 effort 实验

可选[执行策略](configuration.md#有界执行策略显式启用)逐路由声明完整允许绑定，
包括账号、effort、权限和历史模式。Auto／Quick／Full 不会启用它。确认重复 L1
空转后，可在原尝试／token 预算及显式切换上限内前进实验顺序；这条确定性规则
不证明后一个模型更好。渠道错误使用单独声明的后备，两种机制都不能选择未声明
的模型或费用渠道。

开启唯一 [JEV 总开关](jev.md) 后，确认结束的失败也可把已有允许绑定作为决策
候选。采用前核对冻结允许集、权限、剩余尝试与切换预算，不再设置 JEV 模型路由
开关。没有合法备选时继续当前绑定。语义选择只记录为实际选择，不代表已经测得
该模型更快或更好。

effort 改变会冷启动角色，并记录完整前后绑定。对照必须计入重读、启动及实际
已知／未知用量与成本。离线测试以受控 driver 结果运行真实引擎返修和恢复路径，
不能证明实际模型质量或速度；改变默认值或宣称节省前仍须使用配对真实任务与
冻结 oracle。

## 解释配置证据与质量证据

源码和离线协议测试能够证明字段传递与错误处理。SDK 或 machine API 接受请求，
能够证明接口接纳。服务端实际采用的 effort 和交付质量提升，则需要后端证据或
真实任务测量。

DSH `initialize` 校验配置，但回复只包含 server 信息。Flywheel 的
`ready.reasoningEffort` 是初始化成功后的请求回显，不是模型服务独立证明。
Codex Pro 区分 `requestedEffort`、`accepted` 和未知的 `effectiveEffort:null`。
依据见 [DSH SDK server](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sdk/server/src/server.ts)
与 [Codex Pro machine 协议](https://github.com/ckanner/dsh-subagent-codex-pro/blob/9c7d724cb19dc29457a56f4a4269c24278595d2f/docs/machine.md)。
无推理 doctor 不能证明账号余额、真实调用或更高档位的收益。
