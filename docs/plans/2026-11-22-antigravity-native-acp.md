# Antigravity 官方 ACP 交互式接入方案

[English](2026-11-22-antigravity-native-acp.en.md) · 简体中文 · [方案索引](README.md) · [当前 Antigravity 指南](../zh-CN/antigravity.md)

- 日期：2026-11-22；按用户指定的官方独立 ACP 与个人交互用途修订。
- 状态：**待审方案，仅文档变更**。本文不交付 ACP 运行实现、配置字段、登录或图片能力，不更改现有默认值。
- 原始基线：Flywheel `291c0a02f9042d522d3e0f3c62977f63149465cd` / 0.38.0；`@ckanner/dsh-subagent-antigravity` 0.2.1，源码 `a20eb63bbfa2849df63043595475957bf4a4f04d`。
- 独立修复：§2 的 evidence-only 回执问题另行修复和验收；本文保留其基线事实，不把方案改写当成修复完成。

## 1. 决策、用途与阶段

**首期采用 ACP Registry 中 Google 官方、未经修改的 `antigravity-acp`，在 DSH/Flywheel UI 提供由用户直接操作的外部智能体会话。** 用户连接、走该 agent 自己的 Google 登录、发送开发任务、处理权限、审阅结果并中断工作。官方 agent 拥有模型调用、工具调用及其自身的 agent loop；桌面宿主承担 ACP client 与界面职责。

当前官方 headless stream-json 通道、默认值和既有任务保持原行为。首期 ACP 会话不作为 DSH 父智能体下的通用模型 provider，不自动进入 JEV、Plan/Execute/Review 或其他无人值守候选集合。用户在 UI 看见 ACP 会话，也不意味着 Flywheel task 已执行或通过独立 Review。

**修订说明。** 原提案只选择 `agy acp`，并把官方独立 `.par` 与第三方 wrapper 一起排除，未准确表达这次指定的方向。Registry manifest 和 Google 的 Android Studio 文档证明独立发行物也是官方编辑器接入路径 [S17、S18]。本稿据此调整主路径；原生 CLI `agy acp` 仍是官方选项，但须另行验证、显式选择，不能静默替代独立发行物。保留旧文件名与链接以兼容已有引用。

| 阶段 | 拟议范围与边界 |
| --- | --- |
| 首期：个人交互式外部 agent | macOS/Linux 上直接连接官方独立 ACP server，官方个人登录、文本对话、模型/effort 选择、权限、停止、精确历史恢复；图片与原生协作按实际能力分别验收。Registry 有 Windows 包不代表本项目已验收 Windows。 |
| 后续：其他官方 runtime | 可单独评估 `agy acp`；保留独立来源/版本/能力记录，不复用未经证明的 flags、认证或历史契约。 |
| 后续：Flywheel 自动化执行绑定 | 另立实现范围，先证明技术契约及所选用途、认证上下文受到支持，再讨论 machine API、角色准入和自动调度。首期会话不能据此自动升级为该绑定。 |

ACP 指 **Agent Client Protocol**，是客户端与完整 agent 的协议，不是新的模型名称。目标是可靠的交互、能力观察与经验证的原图输入；速度或质量收益需要测量。本方案不修改 Auto/Quick/Full、计划批准、JEV 单一开关、模型默认表、当前订阅规则或独立验收，也不对所有现有 Flywheel 使用方式作合规结论。

## 2. 当前架构与独立修复

当前已实现的路径是：

`Flywheel Go owner → Antigravity worker → 插件公开 machine API → 自有 headless agy 进程`。

插件拥有进程组和持久 recovery journal；Go owner 消费前核验终态、身份、配置及清理。DSH UI 不写任务状态。依据为 [Go driver][C1]、[worker bridge][C2]、[插件 runner][C3] 和 [machine 类型][C4]。新增交互会话另有 session record，不写 `state.json`，也不伪造 Go 调用回执。

| 基线事实 | 对本方案的约束 |
| --- | --- |
| headless 每次发送 prompt 后关闭 stdin。 | 保持原行为；历史续接、进程常驻和协议是不同维度。 |
| JEV/调查分析要求 evidence-only；普通执行要求已支持的显式授权。 | 首期 ACP 不加入这些角色；后续 machine 绑定逐项证明同等契约。 |
| 模型、effort、物理 cwd、订阅控制及 native mode 属于冻结身份。 | 后续 machine 身份还必须包含准确的 ACP 发行来源及能力，不只写 `acp`。 |
| packet 原图目前通过已验证 Claude 绑定准入。 | 交互 ACP 图片验收不自动改变 packet 的引擎准入。 |
| 当前 Swarm 的 Full、受审分工和输入检查已存在。 | 原有检查保留；未来 ACP machine 绑定继续遵守。交互会话不伪装成已审 packet。 |
| 未知调用不重放。 | ACP Session ID 或成功重新连接不证明丢失 prompt 未执行。 |

**原始基线缺陷。** 0.2.1 adapter 在受管 agent 与实际运行限制验证通过时允许 `init.tools` 有非空目录；Go owner 仍要求目录为空，导致成功回执被误判为 unknown，JEV 保持冻结基线，调查分析可能 blocked。独立修复应验证同一回执贯穿 adapter → worker → Go owner → 决策/分析消费与恢复，不能仅删除工具目录限制。

实现前核对该修复自己的源码与验收记录。此问题不靠 ACP 解决，也不要求首期交互会话实现 Go lifecycle ACK；后者只属于未来 machine 绑定。

## 3. 依据与备选方案

### 3.1 两种官方发行物与支持边界

| 发行入口 | 2026-11-22 的来源事实 | 本方案处理 |
| --- | --- | --- |
| Registry `antigravity-acp` | manifest 标注 Google LLC，当前观察版本 1.5.2；Google 下载地址；Mac/Linux 为 `agy_acp_server.par`，Windows 为 `.exe`，各有 SHA-256 和平台参数 [S17]。 | **首期主路径。** 固定审核过的 manifest、包字节、平台命令和参数；1.5.2 是观察值，不是已验证默认值。 |
| CLI `agy acp` | Google 文档公开支持；CLI changelog 记录 1.5.0 的 ACP 入口及 1.5.1 的模型开放变更 [S1、S3、S4]。 | 后续独立选项。CLI flags/版本结论不可直接套用到 Registry server。 |
| 第三方 wrapper／内部 hub RPC | 与上述 Google 原版发行物不同。 | 不属于本次主路径，也不作为失败后的自动替代。 |

官方 Android Studio 指南通过 Registry 安装 agent，其他编辑器指南也公开描述 ACP [S4、S18]。这是采用官方 agent 作为交互式开发会话的产品依据。条款 §4 包含受监督和自主 agent；§6 与 FAQ 对第三方软件使用个人 Antigravity 登录有约束。FAQ 同时支持用户直接脚本/CI，不能据此写成“所有无人值守、MCP、custom agent 或原生 Swarm 都禁止” [S20、S21]。

Google-flair moderator chunduriv 在 2026-10-08 明确区分直接脚本/CI、ACP 编辑器和第三方 coding agent 包装个人登录，并把 ACP Registry 编辑器列为支持路径；该回复也声明不是法律解释 [S22]。本方案依此选择用户直接操作的官方 agent 会话；不能仅靠“使用 ACP”或另一个项目已实现，就为所有外层编排作合规背书。未来自动化绑定必须记录自己的用途与认证依据，不在本稿判断现有全部 Flywheel 流程。

编辑器指南的部分模型限制文字与后续 **CLI** changelog 不一致 [S3、S4]。使用真实 client identity，按所选发行物/版本返回的实际权限与配置选项准入；不能推断 CLI 1.5.1 同时改变独立 server 的模型策略。ACP v1 也不保证全部可选能力 [S5–S8]，v2 preview 不能自动进入第一版。

### 3.2 T3 与 Pi：可借鉴的实现和不能外推的部分

| 固定源码 | 实际实现 | 对本方案的意义 |
| --- | --- | --- |
| T3 main `f10e1eb3825a773cf702569d2258edfdbc8b77dc` | 内置 Antigravity provider 下载官方 CLI，运行 `acp`，个人登录启动同一程序的 `acp --login`；另有通用 Registry 入口 [S23、S24]。 | 证明当前 T3 内置实现已不同于历史 `.par` 比较。借鉴官方 runtime 管理登录，不把 CLI 当成 Registry 同一发行物。 |
| 同一 T3 源码的兼容/配额逻辑 | 旧版本 client identity 回退为 `zed`；quota probe 读取 token 并调用内部配额端点；还通过模拟 SSH 环境影响凭据存储 [S23、S25]。 | 本方案不复制这些策略。T3 整体不能描述为完全不读 token，也不能用它证明全部行为合规。 |
| 社区 `zacbemis/pi-antigravity-acp-provider`，`6d6135d888cb50e2cb1c18bc2520a94ac7d26392` | 使用官方独立 ACP 与 agent 自己的认证，同时注册 Pi provider，并通过 MCP bridge 接入 Pi 工具和外层循环 [S26]。 | 前半部分与发行/登录选择相符；嵌套 provider/tool loop 不属于本方案首期。该扩展不是 Pi core。 |
| 其他名为 Pi Antigravity 的扩展 | 存在直接 OAuth 接法，不能从项目昵称推断使用 ACP [S27]。 | 按准确仓库和 commit 讨论，不统称“Pi 都这样实现”。 |

### 3.3 性能讨论能够证明什么


| 2026-11-22 核对的资料 | 观察 | 限制与设计含义 |
| --- | --- | --- |
| T3 discussion #28315，2026-10-31 [S9] | 一位 Linux 用户测得旧 `.par` 冷初始化约 3.0–3.5 秒，原生 ACP 约 0.26–0.50 秒。 | 两边都是 ACP；差异属于实现，不能当成 ACP 相对 headless 的固定损失。 |
| 2BAB 24 次比较，2026-11-11、11-17 更新 [S10] | 旧 ACP / hub 中位耗时：文本 11.79/9.21 秒，文件任务 29.65/32.23 秒。 | 版本、默认上下文和工具策略不同；未重跑同版本原生 ACP 的延迟测试，不能得出通用排名。 |
| Theo 的 X 帖，2026-11-04 [S11] | 报告 bindings 有问题，CLI 重跑后 false reports 约少 5%。 | 缺少完整受控评估；5% 指错误报告，不是延迟或协议能力系数。此轮通过公开镜像读到正文，未成功直接读取 X 原站正文。 |
| T3 issue #28172 [S12] | 旧 server 的 reasoning/streaming 问题；维护者在原生 agy 1.5.12 无法复现，11-20 关闭。 | 不能把历史症状当成当前原生 ACP 的普遍缺陷；是否显示 reasoning 也不是独立质量指标。 |
| 上游缓存 issue #1533 [S13] | 用户在 interactive、headless、ACP 均报告缓存突降。 | 后端解释尚非确认根因；保留缓存观测，不承诺某条通道天然免疫。 |
| 上游超时 issue #1402 [S14] | 除 TUI/headless 外，包含 Windows 11、Vertex AI/ADC 下原生 ACP 1.5.5 的超时/恢复报告。 | 该平台/认证路径不属于首期 macOS/Linux、个人登录基线。它推动生命周期测试，不证明我们的准确路径已复现。 |
| DSH 社区反馈，2026-11-13 [S15] | 报告 ACP 集成中重复工作、上下文丢失。 | 缺少准确客户端、运行时及配置，也未明确是本插件。应测压缩/历史，不把原因先归到协议。 |

本轮调研没有建立当前独立 ACP、原生 CLI ACP 与本 headless adapter 在可比账号、模型/effort、上下文、工具下的严格质量基准。不同发行物不能声称使用相同二进制。因此既不能断言 ACP 必然降智，也不能宣称两条接法已完全等价。

### 3.4 备选方案

| 方案 | 决定 |
| --- | --- |
| 保留现有 headless | 继续服务已实现的路径，不因本提案改变运行时或默认值。 |
| 官方独立 ACP + 用户直接操作 | **首期建议。** 固定 Registry 原版，并完成登录、权限、生命周期和 UI 验收。 |
| 官方 CLI `agy acp` | 另行验证后的显式选项，不静默替换主路径。 |
| 立刻加入所有 Flywheel 自动化候选 | 不在首期；技术门禁和用途/认证支持均需另行明确。 |
| 第三方 wrapper 或内部 hub RPC | 本次不采用，不与官方独立发行物混为一类。 |

## 4. 所有权与拟议接口

首期路径为 `用户 → DSH/Flywheel 外部 agent 会话 UI → 独立插件 ACP client → 官方 antigravity-acp`。插件拥有连接、进程组、事件关联、session record 和取消；Google agent 拥有模型、工具与内部编排。UI 发出用户操作并展示事实，不运行一个父 agent 来重新决定或委派同一任务。

**每个交互会话拥有一个独立进程/连接，至多一个 pending prompt。** 多轮用户交互可以复用该进程；这不是跨用户/任务的共享 daemon。本轮 prompt 完成、已知子工作收敛和会话进程退出是不同事实。关闭会话才执行有界停止与清理；不能要求每轮都有退出码，也不能用 root response 宣称整树完成。

写入前必须遵守物理 workspace 的所有权与 lease。交互 UI 不得与活动 Flywheel writer 重叠，也不能绕过人工接管门禁；使用明确可用的工作区，或完成现有接管流程。后续若将产物交给 task，仍走已审产物与正常验证交接；交互完成不改任务状态。

客户端仅宣告已实现、受用户授权约束并通过测试的能力。初版不自动把 DSH 工具封装成 MCP 给 agent，也不凭 `mcpCapabilities` 注入服务器。官方 runtime 本身的 MCP、skills、自定义 agent 和内部协作按其支持范围及用户授权处理；这里是客户端实现范围，不是对这些官方功能的普遍禁止。未实现的宿主 fs/terminal/elicitation 请求应明确失败；permission handler 必须真实存在。

预检与连接解析到同一个官方程序，记录发行入口、manifest/二进制 hash、版本、平台参数、adapter 包身份、协议及能力摘要。`clientInfo` 使用真实名称和版本；不冒用编辑器。配置或二进制变化使原能力验证失效，不记录凭证内容。

### 首期接口示意

以下是**待设计的客户端设置示例，当前配置不接受**；不是 machine profile，也不是可直接安装的完整配置：

```json
{
  "externalAgentSessions": {
    "antigravity": {
      "mode": "personal-interactive",
      "runtime": {
        "source": "official-acp-registry",
        "agentId": "antigravity-acp",
        "release": "<explicitly-validated-release>"
      },
      "authentication": "oauth-personal",
      "modelSelection": "from-session-config"
    }
  }
}
```

| 界面 | 首期契约 |
| --- | --- |
| 外部 agent 会话 | 显式连接/登录、选择工作目录、选择 runtime 提供的模型/effort、发任务、处理权限、审阅和停止。 |
| 插件公开交互 API | 独立版本化 connect/authenticate/new-or-load/prompt/cancel/close 与观察事件；UI 不再嵌入另一套 ACP parser。 |
| 会话记录 | 自有 session/turn/connection 身份、输入 manifest、dispatch/terminal/活动/清理事实；不写 Flywheel task state，不构造 machine 成功回执。 |
| 能力与运行配置 | 区分 advertised/local-verified/live-verified/unknown，以及 requested/accepted/observed；回显不等于独立后端观察。 |
| 既有 machine API、模型/角色绑定 | 首期不新增 ACP 候选。现有 headless 协议及省略值行为保持不变。 |

未来 machine 绑定须另定 schema、transport 与发行来源字段、machine/worker/lifecycle/ledger 版本及迁移；旧 reader 必须明确拒绝不认识的 ACP 事实。交互 API 可复用经过验证的底层模块，但不因此获得 machine 资格。现有 Go 成功要求其终态、exitCode 0 与 cleanup 契约；持续存活的交互事件不能伪装成该回执。

## 5. 连接、官方登录与模型选择

首期由明确的用户操作触发以下流程：

1. 选择工作目录与官方 Registry 发行物，验证所有权/lease、完整性、平台命令/参数和本地配置。完整包不等于模型权限或账号额度。
2. 启动自有进程，以真实 `clientInfo` 和已实现能力发送 `initialize`，读取 `authMethods` 和能力。所需能力未知时在任务派发前报告。
3. 显示“连接／使用 Google 登录”。已有官方登录可由 agent 复用；需要登录且用户启动该操作时，仅使用 runtime 实际支持的 `authenticate`，个人路径选择 `oauth-personal`。浏览器/terminal 辅助仅在该发行物及客户端已支持时启用；不能直接给 `.par` 添加 CLI 的 `--login` 或假定其支持 `terminal-login`。
4. Google agent 生成认证请求、完成交换并保存/刷新凭据；UI 只承载官方提供的交互。凭据不进入 Flywheel 配置、日志、journal 或 provider adapter。取消/失败回到可解释状态，不改账号、API key 或付费渠道继续。
5. 完成登录后重验连接和会话。新建 Session，或加载用户选择且身份匹配的自有 Session，不选择 latest，不重放丢失的 prompt。
6. 读取该 Session 的可用模型/effort/权限选项并确认用户选择的接受结果 [S7]。不硬编码 `thought_level`/`thinking_level`；Orchestrator 的固定 effort 不补成 high。会话目录或握手成功不证明全部模型可用。
7. 用户发送任务时冻结完整 prompt 和附件身份，先持久化可能派发记录再写 `session/prompt`。首期由客户端本地记录负责；未来 machine 路径另需 Go durable lifecycle ACK。

Google Authenticate 文档说明本地界面可能共享认证 [S19]。**不承诺每个 provider instance 都有独立登录缓存**；独立设置目录只有官方明确支持并验证后才采用，也不能据此推断账号隔离。不得伪造 SSH 环境、复制 token、读取 keyring、注册自己的 OAuth client、代理私人配额端点或替换 client identity。

诊断只使用公开、已验证的接口。需创建会话或产生辅助工作的 probe 应注明副作用，在自有隔离目录执行，不发送业务 prompt。模型/effort/权限改动只在已确认安全边界采用；活动 turn 保留原请求与之后的观察，不静默换模型或把新配置倒填为旧事实。

## 6. 生命周期、取消与恢复

### 6.1 交互会话

记录 client session/turn ID、adapter run ID、native Session ID、connection generation 和 JSON-RPC request ID。客户端 RPC ID 不是 provider 幂等键。一个会话最多一个未终结 prompt，后续用户输入先排队。下一个 prompt 必须等待相关原生工作已验证收敛；已知活动 child 或更早工作的未知结果继续阻断。并发 steering 等独立能力验证后再开放 [S16]。

| 事件 | 必须行为 |
| --- | --- |
| prompt 可能写出前失败 | 只有持久记录证明该业务 prompt 尚未写出、相关待决请求已收敛时，才标记 proven not-dispatched；不要求可继续使用的会话进程退出，也不能据此清除更早的未知请求。 |
| 写入尝试后失联、超时、坏帧/超限 | 保留 dispatch-possible/accepted 与 unknown outcome；不自动重发、不换发行物或通道。 |
| root prompt response | 展示关联 stop reason 与输出，继续观察已知后续/子工作。root 完成、全部已知工作收敛和进程关闭分别显示，不自动产生 Flywheel completed verdict。 |
| 用户停止 | 发送 `session/cancel`，对待决 permission 返回协议 `cancelled`；root request 仍 pending 时继续处理至其 response。通知发出不证明工作停止；必要时有界 TERM/KILL，并记录清理及未知远程工作。 |
| root 结束后仍有 child | 继续处理已归属的事件和权限；此时没有 pending root response 可以证明取消，须依经验证的 quiescence/shutdown 观察。杀进程不等于 child 成功。 |
| 关闭会话／宿主退出 | 有界结束自有进程组并保留历史及未决记录。不能在关闭 UI 时丢弃已派发状态。 |
| 重新连接或加载历史 | 只使用精确自有 Session；区分历史 replay、当前活动和新增用量。能 load 不证明丢失 prompt 的结果。未知请求保留，不能自动补发。 |
| 部分输出后拒绝、取消、资源截断或错误 | 保存部分证据并显示原生原因，不提升为成功或可安全重试。 |

每轮可以有终态和已知活动状态而进程仍存活；会话关闭才有进程退出/清理证明。协议允许的 root 后活动不是自动违规 [S8]，但也不能推翻已宣布的最终 quiescence 而被忽略。没有子观测时，不声称全树静止。ACP cancel、进程清理与未知远程工作取消分开记录；不据此把现有 `providerWork.cancelSupported`/`querySupported` 改成 true。

用户会话按物理 workspace、runtime 来源/版本、账号/配置引用、权限、模型和输入身份关联。共享认证不等于共享对话。跨发行物/通道的历史迁移必须显式验证，不能用新会话退役 pending unknown。压缩和 replay 不覆盖用户已确认的要求；用任务约束保持测试验证连续性，缺失计量或 compaction 元数据保持 unknown。

### 6.2 未来 machine 绑定的额外契约

这一节不作为首期交互 UI 的已实现能力。后续方案若进入 Flywheel 自动化，仍须在 prompt 前冻结调用身份、持久化 dispatch-possible 并取得 Go lifecycle ACK；绑定 Flywheel request ID、角色、目标、任务/packet 与预算。journal 恢复只消费合法终态一次，丢失结果或清理证明时 blocked，不冷启动、不重置预算。

需选择并验证每 invocation 独立进程，或具有足够 quiescence 证明的复用策略；不能把首期会话驻留直接当成 machine 终态保证。统一回执分开保存结果、终态、清理、模型/权限、协作、图片、usage 与 provider work 不确定性；独立 reviewer 不继承 executor 历史，full/verified-incremental 政策保持。未来 steering 需要单独版本化授权和 input revision，不能绕过冻结输入。

## 7. 权限、用途与 evidence-only

### 首期用户权限与个人登录

用户直接操作官方 agent，浏览器/terminal 登录由其本人完成。权限请求与选择绑定正确 Session、turn 和 request identity；`selected` 只能使用 server 提供的 `optionId`，取消回复 `cancelled`。明确展示访问范围和持久授权含义，不能替用户授予 allow-always 或静默扩大目录。缺少已验证 approval bridge 时，不派发需要它的交互任务；plan/accept-edits 标签不等于 OS 沙箱。

遵守用户选定的个人认证和适用订阅约束，拒绝未授权的 API/ADC/GCP/custom-provider/credits fallback。是否能验证某一控制是该 runtime 的能力事实，不能从 headless 配置照搬为已生效。认证失败需要用户明确恢复登录，不自动轮换账号。账号权益、可验证本地控制、配额和实际账单是不同事实；缺少公开配额接口就显示未知。

这里选择个人交互范围，不推导为所有自动化均禁止。官方原生 tools、MCP、定制 agent、Orchestrator/Swarm 可有各自支持场景；外层第三方 agent 调度个人登录则需单独核对官方支持边界 [S20–S22]。改变为 API/enterprise 认证同样是独立用途与计费选择，不能自动代替用户的个人登录。

### 未来 machine 授权与 evidence-only

自动化 ACP 绑定只有在用途/认证支持与技术契约均得到记录后才能讨论角色准入；新的 approval 超出授权时应明确报告，不自动同意或无限等待。现有 headless 的环境过滤、credits 禁用及前后配置复核，应按所选 runtime 实现等价、可验证控制，不能只复用名称。

JEV 继续是单一 `jev.enabled` 下 fresh、无工具的决策，不得因 ACP 增加委派或 Orchestrator/Swarm。只有 runtime 能证明活动 Session 使用同等受管 agent、不可变配置与 run ID/digest，工具/MCP/skills/plugins 和命令执行被禁用且无实际工具或 child 步骤，才可验收 evidence-only。非空工具目录不等于实际执行工具。

ACP 是否提供该活动 agent 证据仍待验证；传入 `--agent` 或检查配置文件不够。在获得该证据前，ACP 不进入 JEV/调查分析；独立修复后的已验证 headless 路径继续使用。不得删除 `evidenceOnly` 来过门禁。这是该角色的专属限制，不是对普通交互会话或官方 custom-agent 功能的普遍禁止。

## 8. 图片与原生协作

### 原图输入

交互会话先验证 image prompt capability 以及实际 adapter/model 路径 [S5、S6]，让用户选择已授权文件并预览。复用允许格式、大小/数量、普通文件与授权根检查；发送前冻结 MIME、bytes、SHA-256。传送原图字节，不把路径、资源 URI 或聊天中已有附件当作已送达。

记录 manifest 和送达证据，不把 base64/凭据倒入进度日志；分别报告序列化、prompt 接受和真实视觉验收。live 测试需要仅凭文件名、文字或元数据无法答出的图。根输入验证不证明 child 收到图。

**未来 packet 准入另行验收。** 只有引擎/UI 共享的能力判断通过后，才可改变当前 Claude 要求；否则保持原有规则。不支持时派发前报错，不删图片声明、不降级纯文本。交互附件记录不自动进入 task packet。

### 官方 agent 内部的 Orchestrator、subagents 与 Swarm

这些是 runtime 自身的能力，与外部父 agent 把 Antigravity 当作模型/工具后端不同。首期只展示并开放**所选独立发行物实际支持且已验证**的模型/mode/命令；不能根据 CLI 文档或 UI 标签宣称已接通。普通 native 模式也不保证无 children。

| 能力 | 交互验收 | 未来 Flywheel machine 额外条件 |
| --- | --- | --- |
| Orchestrator | 精确模型、固定 effort、实际接受值；如实显示其内部协作。 | 保持角色/模型资格，evidence-only 不能委派。 |
| 显式 subagents | 验证真实委派及归属事件；无观测时明确未知，不画全部成功的树。 | 必要工作须满足归集与回执契约。 |
| Swarm | 验证官方支持的启动方式及用户授权，不能把任意 slash 文本当成成功启动。 | 仍要求 Execute + Full + 已审独立 collaboration/writes + 输入条件；首期交互会话不自动具备该资格。 |

ACP child Session 展示是可选能力，agent 可能委派却不暴露 child [S8]。请求模式、实际 child ID、root 完成和 child outcome 分别记录；未知模型、usage、状态和远程清理保持 unknown。观察数量 guard 不是原生并发硬上限。锁定 schema/SDK；只有对应 handler 已实现才宣告 `subagents`，draft 变体单独准入，事件先验证 child 属于本次自有 root。

未来 Swarm packet 的图片排除应保留到受审契约能表达必须看图的 worker，且传递通过验收。下游文本步骤可以使用经过评审的衍生规格，不能删除真实的原图需求来通过检查。

## 9. UI、配置与诊断

首期增加“Antigravity 外部会话”的明确入口，选择官方 Registry runtime、工作目录和个人登录；**不在现有执行 provider/model 下拉框中自动增加 ACP 候选**。ACP 是连接协议，Antigravity 是 agent 身份，模型来自已建立 Session 的真实选项。

UI 必须：

- 显示连接、需要登录、正在登录、可用、任务进行中、等待权限、取消中和结果未知，并提供显式官方登录入口及取消。
- 告知原生登录可能与其他本地 Antigravity 界面共享，不把“断开会话”写成“已退出 Google”。仅在 runtime 支持且用户明确操作时退出登录。
- 展示发行来源、runtime/协议/adapter 版本与完整性、可用模型/effort，以及 advertised/tested/unknown 能力；目录不等于权益。
- 将用户选择、agent 已接受配置、最近实际调用分开；用普通名称解释，原始 ID 放在详情，固定 effort 不补默认值。
- 展示权限、实际工具和已观察 children，允许用户审阅/中断；不把原生自评映射成 Flywheel Review 通过。
- 提供附件预览、冻结身份及根/worker 送达区别；说明当前会话附件是否进入了任何 task packet。
- 让异步登录/模型结果绑定准确 session、runtime、目标和 revision；pending/unknown 请求禁止不安全修改或隐式重试，并执行 workspace 所有权/接管检查。

未来若提供 machine 配置，再加入 transport/发行来源与任务有效配置、来源作用域、JEV 决策理由及引擎共享准入；保存后重读实际生效值。它与 `native-profile`、模型、effort 和 JEV 开关分开。该后续设计不修改现有任务、批准或初始默认值。

## 10. 实施、发行与回退

1. 核对独立回执/打包修复与 headless 验收，保留原源码、registry 和 live 证据；不在本 PR 重写历史结论。
2. 在独立插件实现官方 Registry ACP client 及显式登录流程，固定发行物与公开交互 API；首期不修改现有 machine 协议或自动化候选。
3. 完成文本会话、权限、停止/恢复和官方登录的离线/必要 live 验收，再由 DSH/Flywheel UI 消费同一交互 API，记录真实能力和未知项。
4. 另行授权发布已测插件版本并锁定精确包身份，验证同候选安装与配套文档。配置未启用此会话时仍使用原路径；不支持的 runtime/adapter 明确报错，不静默替代。
5. 图片与原生协作分别验收开放；native CLI 选项单独验证；未来 machine 绑定另提用途/认证与技术方案，再更新必要 schema、bridge、Go owner、ledger、UI 和迁移。
6. 关闭/卸载交互入口前处理自有连接并保存未决记录；断开不等于退出账号或删除共享缓存。保留旧 journal reader，未知 ACP turn 不通过 headless/其他发行物重放。

首期会话内多轮使用同一连接；跨会话共享 server、同 server 并行 Session 以及运行中 steering 仍需独立测试。未来自动化反馈继续遵守冻结输入与安全采用边界，不能借交互按钮绕过 Go 状态机。

## 11. 验证与对照评估

### 11.1 分阶段正确性门禁

| 范围 | 首期必须验证 | 未来 machine 的附加验证 |
| --- | --- | --- |
| 来源/协议 | 原版 manifest/hash、平台参数、准确发行来源；协议/选项拒绝、有界 Unicode/JSON frame、能力差异。 | 身份进入冻结绑定、回执、恢复；旧 reader 明确拒绝。 |
| 登录/权限 | 实际 `authMethods`、显式 `oauth-personal`、官方交互成功/取消/过期/失败、无凭据提取、共享缓存提示；错会话/迟到/拒绝 approval。 | 认证/用途受支持，已有授权不足时不无限等待；订阅控制可验证。 |
| 生命周期 | turn 与会话退出分离；工具/权限/child 中取消、失联、超时、宿主死亡、SIGKILL、坏/重复/迟到终态、进程组清理，以及收敛前不启动下一轮。 | Go ACK 各崩溃点、合法回执一次消费、unknown blocked、预算不重置、完整 required-work 证明。 |
| 历史/计量 | 精确 session load、无自动 replay、压缩保留要求、累计量不当逐轮相加、缺失/负值/不一致差值 unknown。 | 角色隔离、full/incremental、独立 reviewer、任务/packet/目标身份。 |
| 图片/协作 | 冻结字节、源变化拒绝、真实视觉测试、child 归属、Orchestrator effort、Swarm 实际机制与未知状态。 | 引擎/UI 共享图片准入、Full/审查分工/写范围、必须 worker 的图片送达。 |
| Evidence-only | 不作为首期角色宣称。 | 活动受管 agent、禁工具/委派、不可变身份、实际步骤与非空目录区别。 |
| 分发/UI | 干净公开包导入、版本拒绝、Linux/macOS 同候选安装、会话状态、权限、模型/登录异步关联、workspace lease/接管与文档。 | machine/ledger 迁移、有效配置来源、任务/角色准入一致。 |

实现改动沿用仓库对应门禁：Go 改动需逐函数 100% measured statement coverage，插件/bridge 行为检查，发行需真实候选安装。文档 PR 只做静态检查；离线 fixture 不证明账号权限、登录成功、视觉质量或性能，live 验收另记准确 runtime、平台、账号上下文和范围。

### 11.2 比较设计与归因

先冻结任务、评分和接受界限，再区分两个问题：**独立 Registry ACP 与当前 headless 的整体产品比较**，以及未来 **同一 CLI 二进制的 `agy acp` 与 headless 的 transport 比较**。第一种必然含 runtime 差异，不能标成同二进制或把全部结果归到 ACP。记录包字节、账号/认证、模型/effort、settings/rules/plugins、工具授权、workspace、输入及历史/压缩策略。

随机/交替执行顺序；每个场景至少十次配对作为探索报告并保留全部失败。冷试验用新匹配 Session；热试验用等价历史。后端 cache 不可控就说明不可控。仅在已授权的可丢弃 workspace、符合该用途/认证边界的条件下运行；首期评估由用户直接触发，不借基准启用外层无人值守 agent loop。

覆盖文本修改、确定性测试、多文件修复、长上下文/压缩、图片及已验证的原生协作。evidence-only 与自动化 packet 场景只在后续范围准入后测。不支持属于能力结果，不计数值胜出。分别记录：

- 进程启动、初始化、Session create/load、prompt dispatch、首事件/可见文字、本轮终态/后续活动及关闭清理耗时；登录耗时单列。
- 实际接受的模型/effort、可观察模型/工具次数、带 scope 的 tokens/cache/compaction/quota；无公开数据保持未知，不读 token 探测。
- 确定性测试、正确性、约束保持、重复工作、遗漏工具及失败/unknown 结果。
- 取消/恢复、进程数、峰值 RSS、冷启动与驻留成本；不以未显示 reasoning 推断模型能力。

报告中位数、尾部、配对差异、不确定性、原始分布和失败数量。初次采用与后续扩展均需满足预先声明的正确性及资源界限；本文没有跑 benchmark，也不提出改变现有默认。常驻实验应包含热 headless stdin、热 ACP 与现有每次启动基线，避免把驻留效果全算成协议收益。

## 12. 里程碑与待决项

| 里程碑 | 交付与退出条件 |
| --- | --- |
| M0 — 核对独立修复 | 基线回执/打包修复与跨层回归的独立证据，headless 行为/默认不变。 |
| M1 — 官方独立 ACP client | 固定 Registry 原版及平台参数，真实 client identity、显式官方个人登录、文本会话、权限、生命周期与恢复验证。 |
| M2 — 交互 UI 与交付 | DSH/Flywheel 直接外部会话、模型/effort/权限/停止/未知状态、公开交互 API、精确包与候选安装。无新增自动化候选。 |
| M3 — 分项能力 | 根图片及 runtime 内原生协作分别验收；worker 图像和完整子树状态未知时保留限制。CLI runtime 作为独立选项评估。 |
| M4 — 评估与后续决策 | 报告区分 runtime/协议/驻留成本；未来 machine 绑定另审用途/认证支持与完整技术契约，不能从 M2 自动启用。 |

实现前必须明确：

1. 独立 runtime 的准确版本、manifest/hash、平台参数、可选方法、真实账号模型与 effort；其能力不能从 CLI 版本推导。
2. `oauth-personal` 的实际官方交互、共享缓存与退出影响，以及客户端可以提供且已验证的 browser/terminal bridge。
3. 交互公开 API/session record 的版本、root/child 活动和本轮 quiescence 判据；闭会话清理与未知远程工作的限制。
4. 根与必须看图 worker 的送达能力，以及哪些信息只在后来 packet 契约中表达。
5. 若拟自动化，单独证明用途/认证受支持、active-agent evidence-only、Go durable ACK/恢复、角色/Full/分工/输入准入，再确定 machine/ledger 版本。
6. 预登记的质量与延迟/资源指标，以及实际收益是否值得支持第二种 runtime 或更广用途。

批准本文是对上述首期交互方向及后续验证边界的评审，不是发布授权、运行实现、账号合规保证，或为现有任务自动开启 ACP。

## 来源与验证边界

资料核对日期为 2026-11-22。官方文档与 Registry 说明公开路径，固定源码说明具体实现，issue/社区帖子只证明特定反馈；项目存在不等于全部用途获官方许可。Google-flair moderator 与普通论坛账号不可混为一谈；本稿不把身份未验证的 T3/OpenBot/OMP 回复作为官方裁定。本次文档 PR 未运行真实 ACP 推理、登录、质量基准、图片传递或用户机器安装。

| ID | 来源 | 用途 |
| --- | --- | --- |
| S1 | [Google：CLI ACP](https://antigravity.google/docs/cli/acp) | 官方原生入口与编辑器集成。 |
| S2 | [Google：headless mode](https://antigravity.google/docs/cli/headless) | 自动化、持续 stdin、纯文本输入。 |
| S3 | [Google：changelog](https://antigravity.google/docs/changelog) | CLI 1.5.0/1.5.1 历史及生命周期修复。 |
| S4 | [Google：ACP 编辑器集成](https://antigravity.google/docs/ide/extensions/acp) | CLI ACP 认证、flags、会话能力；不能将其全部移植为独立 server 的能力。 |
| S5 | [ACP v1 initialization](https://agentclientprotocol.com/protocol/v1/initialization) | 版本/能力协商。 |
| S6 | [ACP v1 content](https://agentclientprotocol.com/protocol/v1/content) | 图片及其他类型化 prompt 内容。 |
| S7 | [ACP v1 session configuration](https://agentclientprotocol.com/protocol/v1/session-config-options) | 运行时 option ID 和接受值。 |
| S8 | [ACP v1 subagents](https://agentclientprotocol.com/protocol/v1/subagents) | 可选 child Session 暴露与所有权。 |
| S9 | [T3 discussion #28315](https://github.com/pingdotgg/t3code/discussions/28315) | 单作者旧/原生 ACP 启动比较。 |
| S10 | [2BAB 比较](https://2bab.me/en/blog/2026-11-11-antigravity-hub-vs-acp/?source=bokelianmeng.com) | 随任务变化的旧 ACP/hub 耗时及限制。 |
| S11 | [Theo X 原帖](https://x.com/theo/status/2118107369448276371)、[可读镜像](https://nitter.gq/theo/status/2118107369448276371) | bindings/false report 第一手说法；本次无法直接读取 X 正文。 |
| S12 | [T3 issue #28172](https://github.com/pingdotgg/t3code/issues/28172) | 历史 streaming bug 和原生新版本复测。 |
| S13 | [Antigravity issue #1533](https://github.com/google-antigravity/antigravity-cli/issues/1533) | 跨模式缓存反馈，根因尚未确认。 |
| S14 | [Antigravity issue #1402](https://github.com/google-antigravity/antigravity-cli/issues/1402) | 带版本的超时/恢复反馈。 |
| S15 | [DSH 社区反馈](https://linux.do/t/topic/3173768) | 非受控的上下文丢失体验。 |
| S16 | [ACP v1 prompt turn](https://agentclientprotocol.com/protocol/v1/prompt-turn) | prompt、事件、replay 区分及终态/cancel 语义。 |
| S17 | [ACP Registry：antigravity-acp manifest](https://raw.githubusercontent.com/agentclientprotocol/registry/main/antigravity-acp/agent.json) | 2026-11-22 观察到独立 1.5.2、Google 下载地址、平台命令/参数与 SHA-256；不是已验收默认版本。 |
| S18 | [Google：Android Studio](https://antigravity.google/docs/ide/extensions/android-studio/) | 从 ACP Registry 安装官方 agent 的编辑器路径。 |
| S19 | [Google：Authenticate](https://antigravity.google/docs/install) | 官方登录与跨本地界面的共享认证；不承诺独立凭据缓存。 |
| S20 | [Google：Terms of Service](https://antigravity.google/terms) | §4 的自主/受监督 agent 与责任；§6 的第三方使用限制。 |
| S21 | [Google：FAQ](https://antigravity.google/docs/faq) | 直接 headless 脚本/CI 与第三方 agent 驱动个人登录的边界。 |
| S22 | [Google-flair moderator chunduriv，2026-10-08](https://discuss.ai.google.dev/t/is-google-ai-pro-antigravity-cli-automation-with-codex-permitted/187241/2) | 明确列出 ACP 编辑器为支持路径，并区分第三方 coding agent 包装；不是法律裁定。 |
| S23 | [T3 官方 CLI 下载源码](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/antigravityCliRelease.ts)、[启动参数](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/antigravityAuthSupport.ts)、[通用 Registry 解析](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/packages/provider-acp-registry/src/server/AcpRegistrySupport.ts) | 固定 main 源码；内置 provider 使用官方 CLI，不能与通用 Registry 入口混为一谈。 |
| S24 | [T3 登录源码](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/Drivers/AntigravityDriver.ts) | 官方 `acp --login` 进程接收用户 code。 |
| S25 | [T3 client identity](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/acp/AntigravityAcpSupport.ts)、[quota probe](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/antigravityUsageLimits.ts) | 旧版本 `zed` fallback 和 token/私有配额查询；本方案不复制。 |
| S26 | [zacbemis Pi ACP 社区扩展](https://github.com/zacbemis/pi-antigravity-acp-provider/tree/6d6135d888cb50e2cb1c18bc2520a94ac7d26392)、[runtime.ts](https://github.com/zacbemis/pi-antigravity-acp-provider/blob/6d6135d888cb50e2cb1c18bc2520a94ac7d26392/src/runtime.ts)、[MCP bridge](https://github.com/zacbemis/pi-antigravity-acp-provider/blob/6d6135d888cb50e2cb1c18bc2520a94ac7d26392/src/mcp/bridge.ts) | 独立官方 agent/自己的登录，也嵌入 Pi provider/tool loop；不等于 Pi core。 |
| S27 | [另一 Pi Antigravity 扩展](https://github.com/Rahularya01/pi-antigravity) | 区别直接 OAuth 接法与 ACP 接法，不能只凭 Pi 名称归类。 |

[C1]: https://github.com/ckanner/flywheel/blob/291c0a02f9042d522d3e0f3c62977f63149465cd/internal/drivers/antigravity.go
[C2]: https://github.com/ckanner/flywheel/blob/291c0a02f9042d522d3e0f3c62977f63149465cd/plugins/dsh/src/antigravity-bridge.js
[C3]: https://github.com/ckanner/dsh-subagent-antigravity/blob/a20eb63bbfa2849df63043595475957bf4a4f04d/lib/runner.js
[C4]: https://github.com/ckanner/dsh-subagent-antigravity/blob/a20eb63bbfa2849df63043595475957bf4a4f04d/lib/machine.d.ts
