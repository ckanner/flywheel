# 推理等级与账号选择

[English](en/model-policy.md) · 简体中文

2026-11-05 核对。本文区分官方能力、适配器实现、工程建议和实际测量。已查模型文档与请求链路，**尚未对 Flywheel 的代表性任务做 high／xhigh／max 对照评估**，也没有在用户账号上验证额度或真实推理。因此当前默认表是可编辑的初始基线，不是测得的最优组合。

## 1. 为什么当前大多使用 high

现有八个阶段／路由绑定中，七个使用 `high`，Intake 使用 `low`。Kimi K3.1 Pro 和 DeepSeek Flash 的官方默认都是 high；GPT-6.1 Sol 的 API 默认是 medium，Flywheel 显式选择的 high 已高于该默认。GPT-6 Astra 的模型参考列出了五档能力，但没有声明统一的 API 默认。依据见 [Kimi 模型文档](https://www.kimi.com/code/docs/en/kimi-code/models)、[DeepSeek thinking](https://api-docs.deepseek.com/guides/thinking_mode/)、[Sol 模型参考](https://developers.openai.com/api/docs/models/gpt-6.1-sol)及 [Astra 模型参考](https://developers.openai.com/api/docs/models/gpt-6-astra)。

以 high 开始是为了建立容易比较的基线，同时避免所有循环一开始都采用最高档。更高 effort 为复杂任务提供更多推理空间，也可能增加耗时与用量；并不保证每个任务都得到更好的交付。OpenAI 的[部署指南](https://developers.openai.com/api/docs/guides/deployment-checklist)建议用代表性评估判断 xhigh／max 的收益是否值得额外延迟和成本。这个方法支持先建立基线，但不能替代本项目尚未做的实验。

## 2. 不同后端的档位不能直接互换

| 当前调用路径 | 已核对的能力 | Flywheel 当前请求 |
|---|---|---|
| GPT-6.1 Sol／Codex Pro 0.8.0 | 模型支持 `low/medium/high/xhigh/max`；machine API 原样传递 effort | hard 为 high |
| GPT-6.1 Sol、GPT-6 Astra／DSH ChatGPT provider | 模型及已核对的 provider 表均列 `low/medium/high/xhigh/max`；请求写入 `reasoning.effort` | Review、Judge 为 high |
| Kimi K3.1 Pro／Kimi CLI | 模型原生 `low/high/max`，默认 high，不能关闭 thinking | Plan、frontend、Garden 为 high |
| DeepSeek Flash／DSH rc.5 | DSH 接受 `off/low/high/max`，默认 high | 普通执行 high，Intake low |

GPT 的 xhigh 和 max 已有支持，不能以“不支持更高档”解释选择 high。反过来，不能把 GPT 的五档照搬到每个 provider：Kimi 的兼容别名 xhigh／max／ultra 映射到同一原生 max；DeepSeek 通用 API 的 xhigh 兼容映射是 high，而本项目的 DSH adapter 会先拒绝 xhigh，因为它只接受表中四值。DeepSeek 的 max 才是这条路径可明确选择的更高档。相关原生别名以两家的模型文档为准，DSH 的限制见[参数配置](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek/src/config.ts)与[序列化校验](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek/src/serialize.ts)。

ChatGPT provider 的核对快照为 `388868f91bcdc05795fb94d21b868ded6d7219c9`（0.1.17）：[有日期的模型表](https://github.com/ckanner/dsh-plugin-chatgpt/blob/388868f91bcdc05795fb94d21b868ded6d7219c9/src/models/served.ts)、[请求序列化](https://github.com/ckanner/dsh-plugin-chatgpt/blob/388868f91bcdc05795fb94d21b868ded6d7219c9/src/convert/request.ts)。它描述可请求能力，不替代当前账号的实际可用模型目录。Codex Pro 是插件名称，不能由此推导已经启用另一种服务端推理模式。

## 3. 哪些阶段值得先比较更高档

以下是根据阶段职责提出的工程建议，**不是已实测的性能结论**。当前修复保留原有 effort，不自动升级、重试换档或改变预算策略。

| 阶段 | 保留的基线 | 更高档的评估场景 |
|---|---|---|
| Intake | Flash low | 输入含复杂歧义、合并或分类要求时比较 high |
| Plan | Kimi high | 跨模块架构、依赖关系或冻结验收难以明确时比较 max |
| Execute default | Flash high | 复杂根因分析或困难实现时比较 max；不填 xhigh |
| Execute frontend | Kimi high | 复杂交互状态、跨组件一致性问题时比较 max |
| Execute hard | Sol high | 优先比较 xhigh；极难实现再考虑 max |
| Review | Sol high | 并发、数据一致性、权限边界等高影响审查可比较 xhigh |
| Judge | Astra high | 高影响里程碑终验优先比较 xhigh；最难终验再考虑 max |
| Garden | Kimi high | 普通整理可比较 low，复杂跨任务归纳再比较 max |

hard 承担难实现，Judge 在 L2/L3 后独立核对整个里程碑；它们是较清晰的首批比较对象。对一切阶段一起提高档位，会更难知道改进来自哪里。OpenAI 的[推理指南](https://developers.openai.com/api/docs/guides/reasoning)也按任务难度区分 high、xhigh 和 max，但本表对 Flywheel 阶段的映射是工程判断。

要采用其他档位，在实际生效的完整配置里修改相应绑定的 `thinking`，然后 inspect 并重启引擎。只有 hard 改档时修改 `routes.hard.thinking`；只有 Judge 改档时修改 `loops.judge.thinking`。使用共享 profile 时，阶段级覆盖可以避免无意改变所有引用者。不要创建只有这两项的不完整任务配置来覆盖整个项目配置。

真正的比较应固定模型、后端、上下文、任务和验收条件，记录成功交付率、首轮通过、漏检与误报、返工后的总耗时，以及输入／输出／reasoning／缓存用量。能观察到的订阅额度变化另行记录，未知保持 unknown。API 单价不能直接换算成订阅可完成的任务数。

## 4. “参数传到了”与“模型表现更好”是两种证据

- 源码与离线协议测试能证明请求字段、环境变量和错误路径按契约传递。
- SDK 接受配置或 machine API 接受请求，只证明对应接口接纳，不是效果评测。
- 服务端实际采用的强度和质量收益，需要独立回执或实际任务测量；当前证据不能补造这些事实。

DSH `initialize` 调用配置校验，但回复只含 serverInfo。Flywheel 的 `ready.reasoningEffort` 是初始化成功后的请求回显，不是模型服务独立返回的 effort 证明。Codex Pro 回执明确区分 `requestedEffort`、`accepted` 和未知的 `effectiveEffort:null`。依据见 [DSH SDK server](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sdk/server/src/server.ts)及 [Codex Pro 0.8.0 协议](https://github.com/ckanner/dsh-subagent-codex-pro/blob/04ecdb677735d84f3a0f5dba01a9d9b28c324c0e/docs/machine.md)。无推理 doctor 也不能证明账号余额、真实调用或更高档位的收益。

## 5. DeepSeek 默认应使用已充值账号

当前源码已把 Flash 的默认与 subscriptions 预设改成 `deepseek-account`。它调用 DSH 账号服务，标准桌面与 SDK 在相同 DSH home 下可以复用已登录授权，不要求额外 API key。`deepseek-official` 是单独的 API-key provider，仅作为用户明确选择的配置保留，不作默认后备。

桌面能在登录后初始化账号默认模型，但独立 SDK 调用仍需明确 provider/model。Flywheel 固定请求 `deepseek-account`／`deepseek-flash`，既采用指定账号通路，也保持“只用 Flash”的选择。认证失败或额度不足会报告；默认预设没有跨渠道后备。用户显式配置的后备链仍按原有规则执行，本轮不新增后备路径。

**发布与迁移要分开：**0.28.3 将内嵌默认改为账号，0.28.2 仍内嵌 API 默认。安装升级保留已有用户配置，采用新账号预设需升级至 0.28.3 或以后版本，再预览并显式应用迁移。完整账号/API 区别、共享登录条件和迁移步骤见[配置指南](configuration.md#deepseek-账号与-api-key-是两个-provider)。本轮没有修改用户机器上的凭证或正在运行的任务。

## 可选 Claude Code Pro

默认模型策略尚未给 Claude 分配场景。可选 `claude-code-pro` harness 接收明确的原生模型与 low/medium/high/xhigh/max 请求，具体 CLI／账号可能拒绝某项选择；此接入不新增模型排名或 effort 实测结论。配套 0.3.2 公开接口、full-access 边界及用量口径见[接入指南](claude-code-pro.md)。
