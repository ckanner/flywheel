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

## 规划角色与当前对话模型

New/Task 登记薄 brief，宿主对话整理事实并讨论，真正设计由引擎执行。未显式配置时，
`loops.design-plan` 继承 `loops.plan`，`loops.design-review` 和 `loops.plan-review`
继承 `loops.review`。可以给新角色独立设置支持的模型、思考等级和 native profile，
不改变来源角色。config inspect 的 `bindingSource` 显示实际来源；配置视图展示这些
继承行，只有编辑后才新增独立配置。角色作用域、历史和权限覆盖继续独立。

复杂设计的模型选择应落在 design/review 绑定，不要求每次 New 前先切换宿主模型。
现有模型和思考等级默认值保持不变，选择仍是工程起点而非已测最优。完整交互见
[方案设计与审查](reviewed-planning.md)。

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
| GPT-6.1 Sol／Codex Pro 0.10.2 | `low`、`medium`、`high`、`xhigh`、`max` | machine API 传递指定 effort |
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

## 通过 DSH Provider 使用 ChatGPT Web Pro

Web 路径明确选择 `harness:"dsh"`、`provider:"chatgpt"`、
`model:"gpt-6-pro-web"`、`thinking:"pro"`，可以配置到任意 Loop、执行 Route
或显式允许的 execution-policy binding。既有模型默认值与其他 Provider 的档位范围
保持原样。在当前生效的完整配置中编辑对应 binding，例如：

```json
{"harness":"dsh","provider":"chatgpt","channel":"chatgpt","model":"gpt-6-pro-web","thinking":"pro"}
```

本次源码集成要求所选 SDK profile 实际加载匹配的 ChatGPT Web host、公开 DSH
host-policy 扩展和唯一 CompactionEngine。包版本、桌面登录状态、静态模型目录都
不能单独证明这些能力；缺失时 worker 在 prompt 前明确拒绝。
插件要求 `summaryRoute: 2`；`replayVeto`、`durableResponseAck`、
`durableSummaryAck`、`pressurePolicy` 仍为版本 1。旧 `summaryRoute: 1` 宿主在
派发前拒绝。worker Web extension、`chatgptWebHost` 和 binding 的
`hostContractVersion` 仍为版本 1，不能用这些外层版本号代替插件逐项宿主能力校验。
通过 [ChatGPT 插件](https://github.com/ckanner/dsh-plugin-chatgpt) 配置专用浏览器
连接及 project-only 上下文策略；稳定 0.1.20 安装示例不包含这个新路径。
源码 harness 以 [vendor/chatgpt.json](../../plugins/dsh/vendor/chatgpt.json) 的摘要
锁定已审查 `0.2.0-alpha.1` archive，并将其公开只读恢复 reader 与 schema 验证器
构建为 Flywheel 自有相对模块，保留来源/输出摘要和原许可证。安装后的恢复 worker
无需在自身模块路径查找 provider，也不能启动 SDK、浏览器或模型。
此构建依赖不会把 provider 装入用户 SDK profile。
实际使用必须把该精确候选安装到同一个 SDK profile，并运行匹配的精确源码
patched DSH 宿主；不宣称 alpha 已在 registry 发行或原版 SDK 已支持。

在桌面模型编辑器选择 DSH 模型后，点击**读取此模型的推理档位**，从精确的
provider/model 公开 metadata 读取合法 efforts 和默认值。目录不支持的旧值会保留并
标为不支持；更换模型后须重新核对。实际执行与预热仍在所选 SDK profile 独立验证，
不会因为桌面目录通过就跳过 SDK 准入，也不会把 pro 偷换为 high/max。

DSH 拥有已提交 Session 事件、derived history、工具执行及提交历史变更的唯一
CompactionEngine。插件协调 Web 会话轮换、输入预算、摘要路由、工具协议适配和恢复；
网页投影和日志不是第二份权威历史。Flywheel 管理角色隔离、预算与
调用回执。Web 是 opaque transport：保留总 wall 时限，输出静默本身不触发 idle
终止。用量和费用为 unknown，因此硬用量预算可能阻止后续调用。远端结果 unknown
会阻止跨角色、fresh 或更换 Provider 自动重放，必须先对账原调用；续接还要求
connection、account、policy 完全相同。Web Session 账本使用 v5，旧 Flywheel 不能
继续读取写回。离线契约测试不等于真实账号／浏览器验收已通过。

压缩时，用户显式配置的摘要 provider/model 优先于本次主调用的 proposed model。
宿主用 `configuredTarget` 单独传递该显式选择，与 `proposedTarget` 和历史默认值
区分。插件仅在最终选中的摘要目标为此 Web 模型时应用 Pro 摘要策略；显式选择
其他摘要路由时，保留其模型、effort 与计量。没有显式摘要路由时，本次
proposed target 为 Web 则使用 Web Pro，即使最后回答的模型仍是 SIWC。
Flywheel 不覆盖宿主的摘要选择。

手动压缩与自动压缩一样，宿主在整个维护操作中保留真实发起 Agent 的上下文。
因此摘要恢复和执行域归属绑定真实 Session 与物理 workspace，即使插件状态目录
位于另一处也不改变归属。摘要中断且未对账时，同一 workspace 的新工作继续阻断，
包括 fresh 非 Web Agent。插件不通过 sessionId 猜测发起 Agent；显式采用摘要仍
绑定原宿主事务，不能重复应用同一摘要。

### 对账中断的 Web 调用

继续任务前，在 ChatGPT 插件中检查原请求。只读观察、采用已捕获回答与显式丢弃
已拒绝回答是三个独立动作。Flywheel 下次对账只读取原私有回执，不启动 DSH、
浏览器、模型或工具。必须核对原 connection/account/policy、execution domain、
SDK home/profile 选择、原 owner 已退出，以及原候选和角色边界。
只读 worker 在原物理执行目录启动，使相对 worker 路径和 wrapper 与原派发保持
相同的目录语义。
profile 管理设置以旧调用准入时冻结的配置为准；下一次新 SDK 调用仍独立验证
当前配置，读取旧回执不表示当前 profile 文件未改变。

| 原调用证据 | 对账结果 |
|---|---|
| 正面证明 SDK prompt 未派发，当前 candidate/role guard 仍等于原 before | 已知失败；不伪造成功回答 |
| 浏览器未提交，但 SDK 已接受 prompt；原 SDK 失败终态、turn identity、正常关闭和 Go candidate/guard 回执齐全 | 已知 SDK 失败；保留 dispatched 事实，不复用该失败历史 |
| 原 SDK prompt/wait/完整 terminal/正常关闭，同一已提交主回答，以及原 Go terminal candidate/guard 回执 | 恢复该原 completed 结果 |
| 显式按 digest 丢弃已拒绝回答，原 SDK 正常关闭、有实际观察的 turn identity、协议完整、工具已结算，保留原 Go terminal candidate/guard 回执 | 已知失败；无成功文本，不提供历史复用 |
| 原 SDK 断线后才 adopt 纯 final，缺少原业务完成终态 | 继续阻断 |
| adopt 工具回答或摘要，缺少原 SDK 后续完整业务操作 | 继续阻断 |
| 工具、远端、清理仍未知，owner 活跃，绑定/域/候选变化，或已派发调用缺少原 candidate 回执 | 继续阻断 |

只有新 managed Web 调用会记录所需恢复检查点；旧调用不能事后补造。
已知失败可解除不确定性栅栏，后续是否继续由正常任务策略决定。对账保持用量
unknown，不补发 prompt。
浏览器未提交不证明 SDK 历史未变化。SDK 准入或已接受 prompt 后中断，缺少所需
原终态证明时，即使浏览器未发送也继续阻断。

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
