# 模型、思考等级与订阅

[文档首页](../README.zh-CN.md) · [English](../en/model-policy.md) · 简体中文

[默认模板](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.config.example.json)通过原生订阅客户端使用 Gemini、Claude、Codex 与 Kimi。这是可编辑的工程起点，不是实测能力排名或账号权益保证。已有配置只在显式迁移后采用新绑定。[已批准方案](../plans/2026-11-19-subscription-cli-workflows.md)记录调研与分工依据，本页说明当前实现。

## 默认模型分工

| 阶段或路由 | Profile | Harness | 完整模型 ID | 思考等级 |
| --- | --- | --- | --- | --- |
| 系统设计 | `carbon` | `antigravity` | `gemini-4-carbon-high` | `high` |
| 系统设计审查 | `astra` | `codex-pro` | `gpt-6.2-astra` | `high` |
| 里程碑／实施计划 | `opus-plan` | `claude-code-pro` | `claude-opus-5-5` | `high` |
| 计划审查 | `sol-plan` | `codex-pro` | `gpt-6.2-sol` | `medium` |
| 紧凑设计与单 packet 计划 | `hydrogen-local` | `antigravity` | `gemini-4-hydrogen-medium` | `medium` |
| 紧凑产物独立联合审查 | `sol-plan` | `codex-pro` | `gpt-6.2-sol` | `medium` |
| Execute：default | `hydrogen` | `antigravity` | `gemini-4-hydrogen-high` | `high` |
| Execute：complex | `neon` | `antigravity` | `gemini-4-neon-high` | `high` |
| Execute：frontend / multimodal | `opus-ui` | `claude-code-pro` | `claude-opus-5-5` | `medium` |
| Execute：backend | `sol` | `codex-pro` | `gpt-6.2-sol` | `high` |
| Execute：batch | `kimi` | `kimi` native agent | `kimi-code/k3.1-pro` | `high` |
| Execute：hard | `astra` | `codex-pro` | `gpt-6.2-astra` | `high` |
| 代码 Review | `sol` | `codex-pro` | `gpt-6.2-sol` | `high` |
| 里程碑 Judge | `astra` | `codex-pro` | `gpt-6.2-astra` | `high` |
| Intake | `hydrogen-summary` | `antigravity` | `gemini-4-hydrogen-low` | `low` |
| 调查分析 | `neon` | `antigravity`，仅分析证据 | `gemini-4-neon-high` | `high` |
| Garden | `kimi` | `kimi` native agent | `kimi-code/k3.1-pro` | `high` |
| 可选 JEV 决策 | `hydrogen-local` | `antigravity`，仅分析证据 | `gemini-4-hydrogen-medium` | `medium` |

JEV **默认关闭**。额外的 `fable` profile 为 `claude-fable-5-2 / high`，`carbon-explore` 为 `gemini-4-carbon-medium / medium`；它们不是自动后备，需要显式改绑定。原生模型不可用时失败，不会换到另一条付费路径。

已批准方案最初把 JEV 分给 Sol medium。本实现改用 Hydrogen medium，因为 Antigravity
适配器能在派发前核验原生空工具列表，当前 Codex 适配器尚无等价的 evidence-only
准入契约。这是能力约束，不是声称 Hydrogen 决策更好；JEV 仍须显式开启。

常规实现和短决策用较轻的 Gemini profile，Carbon 承担系统设计，Neon 承担复杂实现和证据分析。独立审查用 Codex，Claude 承担计划细化和界面工作，Kimi 承担声明为 batch 的批量任务及整理。这些分工尚未经过真实 Flywheel 任务的最优性 A/B 验证。

`multimodal` 选择 Claude 路由。packet 的版本化 `input-images` 契约将真实 PNG/JPEG/GIF/WebP 字节经 Execute → worker → Claude 公开 machine API → Agent SDK 送入原生请求。每条仓库相对路径同时绑定 MIME 类型和 SHA-256；引擎派发前校验文件、解码和大小，并把输入身份保留到验证与恢复。最多四张，每张 4 MiB、合计 8 MiB。提示词中的文件名及 DSH 聊天附件不会自动导入。默认独立 Review 检查代码、验收与冻结引用，不声称已看到这些图片。Antigravity 首版接受文本。见[图片输入](image-inputs.md)、[Claude](claude-code-pro.md)与 [Antigravity](antigravity.md)。

## 规划角色与当前对话

New/Task 登记带代码事实的 brief，设计和审查由引擎调用实际角色。`design-granularity:compact` 必须有具体证据、理由，且没有显式 Full 要求；一次产生设计与单 packet，独立角色对两个精确版本联合审查。范围不明、架构、迁移及多部分工程选 `system`；引擎可从 compact 升级，保留原授权和预算。

`planning.compact` 选择两个紧凑准备绑定；系统准备使用 `loops.design-plan`、`loops.design-review`、`loops.plan` 与 `loops.plan-review`。旧配置省略角色时保留已有继承规则，`config inspect` 展示实际绑定来源。原角色仍拥有自己的权限与会话作用域。

Auto 从已审契约选择 Quick／Full 实施，不是第三条执行链或批准策略。`review-first` 停在精确已审方案的人工批准点；`delegated` 在保留的实施授权范围内继续。紧凑任务也可先审方案。见[方案设计与审查](reviewed-planning.md)和[工作流](workflow.md)。

## 账号选择

预设声明 `subscription:{version:1,mode:"subscription-only"}`。可达阶段、路由、紧凑绑定、Judge panel、执行候选与后备渠道只能使用四种已准入原生 harness。用户文件可保留未使用的自定义定义，它们不会成为后备。默认没有 fallback，也没有 DeepSeek API 或充值账号绑定。

| Harness | 准入证据 | 证据边界 |
| --- | --- | --- |
| Antigravity | 原生账号；拒绝 API/ADC/GCP/自定义模型；credits 后备关闭；核对实际模型和本地控制 | 不证明账号权益、额度、外部工具费用或全过程计费锁 |
| Claude Code Pro 0.7.0 | 同一官方 CLI 进程在提示词前确认支持的 first-party 订阅及 extra usage 关闭；拒绝 API/网关路径 | 提示词前快照，不是原子计费锁；成功还需实际主模型证据 |
| Codex Pro | 同一官方 app-server 的 `account/read`、`config/read`；要求 ChatGPT 登录与原生 OpenAI 路由 | 不证明剩余额度或实际订阅费用 |
| Kimi native agent | 官方脱敏 providers/config API 确认 managed Kimi Code OAuth，无 API Key 路由；关闭独立搜索/抓取和子代理 | 未暴露的字段（包括 custom headers）不作核验声明；不保证整个账号的计费状态 |

Kimi 使用 `kimi-subscription-agent`：native agent 模式、Agents 关闭。严格订阅调用禁用 `WebSearch` 与 `FetchURL`，因为公开账号 API 无法核对它们独立凭据的计费。Kimi print 会在严格订阅派发前拒绝；未声明该策略的旧配置保留原行为。

适配器使用本机原生程序及其支持的登录，不复制 token、不建立 API 账号。认证或额度错误会停止，不自动开启 credits、extra usage 或 API 后备。Doctor 无推理检查本地兼容性，真实账号可用性须独立验收。

按[配置指南](configuration.md)预览并显式应用 `flywheel config migrate --preset subscription-cli`。迁移保留权限、验证、预算和调度；保留的 role-based 策略可能需要明确授权后修改，原生 full-access 绑定才能运行。历史 `subscriptions` 预设仅为兼容迁移，不代表这张新表。

## 思考等级按后端解释

| 集成 | 适配器可选档位 | 解释 |
| --- | --- | --- |
| Gemini Hydrogen / Neon | `low`、`medium`、`high`、`xhigh` | 模型后缀必须与 thinking 一致 |
| Gemini Carbon / Argon | `low`、`medium`、`high` | 拒绝 xhigh，不静默映射 |
| Codex Pro | `low`、`medium`、`high`、`xhigh`、`max` | 仍受具体模型与本机客户端准入限制 |
| Claude Code Pro | `low`、`medium`、`high`、`xhigh`、`max` | 显式原生模型与 effort，不自动换模型 |
| Kimi K3.1 Pro | 原生 `low`、`high`、`max` | 兼容别名不代表跨 provider 同一尺度 |

Gemini 选择参考已批准调研、Google 的[模型选择指南](https://aistudio.google.com/docs/choosing-a-model)与 [Antigravity CLI reference](https://antigravity.google/docs/cli/reference/)。具体调用以已安装模型目录为准。参数被接受与服务端实际推理强度的独立证据分别记录，不混为一谈。

## 基线与评估

比较 effort 时固定模型、harness、任务、上下文、权限与验收。测量首次通过、漏检、误报、含返工总耗时及已知／未知用量。订阅价格不能推导逐任务容量，API 等价估算不是实际订阅账单。Antigravity conversation 计数不能安全累加为逐调用用量，因此标准化总量保留 unknown。

只改变一个阶段时改阶段绑定，共享 profile 影响所有引用者。配置选择首个完整文件，不深度合并；不要为一个 effort 字段创建不完整任务配置。

## 有界模型与 effort 实验

可选[执行策略](configuration.md#有界执行策略显式启用)声明每个 route 的完整候选与有限切换预算。确认的重复 L1 空转可按声明顺序推进，仍受原尝试次数和 token 限额约束。JEV 开启时也只在同一个冻结允许集中选择。两者都不生成权限、扩大范围、重置计数或启用未声明账号；结果未知须先对账。

## 如何解释证据

源码测试证明字段传递和错误行为，候选安装证明真实分发边界；二者不证明真实模型质量、额度或账号可用。CLI 接受参数、独立观察到的设置和交付质量改善分别记录。缺少 usage 或价格不填零。[会话评估](session-evaluation.md)说明成对离线引擎实验及其局限。
