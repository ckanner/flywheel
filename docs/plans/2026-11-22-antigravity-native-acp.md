# Antigravity 可选原生 ACP 通道方案

[English](2026-11-22-antigravity-native-acp.en.md) · 简体中文 · [方案索引](README.md) · [当前 Antigravity 指南](../zh-CN/antigravity.md)

- 日期：2026-11-22。
- 状态：**待审方案，仅文档变更**。本文不交付 ACP 运行实现、配置字段、图片能力，也不更改默认值。
- 基线：Flywheel `291c0a02f9042d522d3e0f3c62977f63149465cd` / 0.38.0；`@ckanner/dsh-subagent-antigravity` 0.2.1，源码 `a20eb63bbfa2849df63043595475957bf4a4f04d`。
- 前置条件：先修复并独立验证 §2 的 evidence-only 回执不一致。本方案与该修复分别提交。

## 1. 决策与目标

保留官方 headless stream-json 为默认通道。在独立反重力插件中增加**显式选择的原生 `agy acp`**，再由 Flywheel 消费插件的公开接口。两条通道共用确定性编排、订阅策略、调用账本、会话所有权和交付门禁。

目标是接通经过验证的原图输入、结构化双向交互，并改善会话与配置的可观察性。速度或质量改善是需要测量的假设，不能据此静默改变已有任务的执行通道。

这里 ACP 指 **Agent Client Protocol**。目标是官方 `agy` 程序内置的 server，不是旧 Python `.par` 独立发行、第三方 wrapper、逆向 `--hub` 接口或换名后的 Gemini CLI。Google 同时为编辑器集成提供原生 ACP、为脚本和 CI 提供 headless [S1、S2]。Flywheel 原先选择 headless 合理；不存在适用于所有场景的唯一优选。

### 范围与非目标

第一阶段覆盖 macOS/Linux 上的受管调用、普通文本执行、明确的原生历史续接，以及分项准入的能力扩展。独立 DSH 插件和 Flywheel worker 使用同一公开入口，不增加一个 DSH 父模型再重复委派同一任务。

本方案不改变模型默认表、Auto/Quick/Full、已批准计划语义、JEV 单一开关、权限、订阅计费约束或独立 Review；不引入远程 agent、自动登录、凭证复制、共享后台 ACP daemon、名为 ACP 的新 provider，或自动跨通道恢复。常驻进程复用、运行中 steering 需经过后文的独立门禁。

## 2. 当前架构与前置修复

当前路径为：

`Flywheel Go owner → Antigravity worker → 插件公开 machine API → 自有 agy 进程`。

插件拥有原生进程组及持久恢复 journal；Go owner 在消费结果前核验终态、身份、配置和清理。DSH UI 投影引擎事实，不写任务状态。源码依据为 [Go driver][C1]、[worker bridge][C2]、[插件 runner][C3] 和 [machine 类型][C4]。

| 当前事实 | 对方案的约束 |
| --- | --- |
| runner 启动 `agy --input-format stream-json --output-format stream-json`，发送一个 prompt 后关闭 stdin。 | 即使恢复原生历史，每次调用仍有启动成本。历史复用与进程常驻是两件事。 |
| JEV 和调查分析要求 evidence-only；普通执行要求当前支持的显式 full-access 绑定。 | ACP 在符合相同授权契约后，才可用于对应角色。 |
| 模型、effort、物理 cwd、订阅控制及 native mode 进入冻结调用/会话身份。 | 通道及协商后的配置也必须进入该身份。 |
| 当前原图通过已验证的 Claude 执行绑定准入。 | ACP 宣称支持 image 不能直接放开 packet 准入。 |
| Swarm 检查 Full、已审独立分工与输入条件；根完成与子任务观察分开。 | 这些检查不随 transport 改变。 |
| 恢复只读 journal，不重放可能已派发的 prompt。 | ACP session ID 或成功重连不能弱化此规则。 |

**前置缺陷。** 基线中 0.2.1 adapter 在受管 agent 与实际运行约束已验证时，允许 CLI `init.tools` 返回非空目录；Go owner 仍要求该目录为空。因此插件成功回执被误判为 unknown，JEV 退回冻结基线，调查分析可能 blocked。必须先修复 Go 证明校验和回归覆盖。不能只删除限制而不验证受管 agent，也不能用增加 transport 代替跨层契约修复。

修复需把同一份非空目录成功回执贯穿 adapter → worker → Go owner → 决策/分析消费及恢复。其验收证据是 ACP 工作的依赖，不是本文声称已完成的结果。

## 3. 依据与备选方案

### 3.1 官方能力基线

Google changelog 在 **1.5.0（2026-11-04）**正式列出 `agy acp`，在 **1.5.1（11-05）**把账号可用模型开放给所有 ACP 客户端 [S3]。这是历史兼容节点，**不是建议锁定旧版本**。应在 §11 的一致性矩阵通过后选择明确的当前二进制。

编辑器指南仍保留部分客户端模型限制，与后续 changelog 冲突。实现应读取固定二进制返回的实际会话选项，不冒用其他编辑器身份获取模型 [S3、S4]。

ACP 会协商协议版本与能力；协议 v1 不代表所有可选能力均可用 [S5]。当前规范描述了图片内容、配置选项及可选子会话展示 [S6–S8]，具体 server 的支持程度须另行验证。第一版只使用已验证的 v1 能力，不能因为文档目录出现 v2 preview 就假定支持。

headless stdin 本身支持同进程多轮，当前插件才是每次 prompt 后关闭 stdin；其文档明确输入内容仅支持文本 [S2]。所以进程复用可以在两条通道上比较，图片输入是另一个独立能力问题。

### 3.2 性能讨论能够证明什么

| 2026-11-22 核对的资料 | 观察 | 限制与设计含义 |
| --- | --- | --- |
| T3 discussion #28315，2026-10-31 [S9] | 一位 Linux 用户测得旧 `.par` 冷初始化约 3.0–3.5 秒，原生 ACP 约 0.26–0.50 秒。 | 两边都是 ACP；差异属于实现，不能当成 ACP 相对 headless 的固定损失。 |
| 2BAB 24 次比较，2026-11-11、11-17 更新 [S10] | 旧 ACP / hub 中位耗时：文本 11.79/9.21 秒，文件任务 29.65/32.23 秒。 | 版本、默认上下文和工具策略不同；未重跑同版本原生 ACP 的延迟测试，不能得出通用排名。 |
| Theo 的 X 帖，2026-11-04 [S11] | 报告 bindings 有问题，CLI 重跑后 false reports 约少 5%。 | 缺少完整受控评估；5% 指错误报告，不是延迟或协议能力系数。此轮通过公开镜像读到正文，未成功直接读取 X 原站正文。 |
| T3 issue #28172 [S12] | 旧 server 的 reasoning/streaming 问题；维护者在原生 agy 1.5.12 无法复现，11-20 关闭。 | 不能把历史症状当成当前原生 ACP 的普遍缺陷；是否显示 reasoning 也不是独立质量指标。 |
| 上游缓存 issue #1533 [S13] | 用户在 interactive、headless、ACP 均报告缓存突降。 | 后端解释尚非确认根因；保留缓存观测，不承诺某条通道天然免疫。 |
| 上游超时 issue #1402 [S14] | 除 TUI/headless 外，包含 Windows 11、Vertex AI/ADC 下原生 ACP 1.5.5 的超时/恢复报告。 | 该平台/认证路径不属于本文 macOS/Linux、subscription-only 基线。它推动生命周期测试，不证明我们的准确路径已复现。 |
| DSH 社区反馈，2026-11-13 [S15] | 报告 ACP 集成中重复工作、上下文丢失。 | 缺少准确客户端、运行时及配置，也未明确是本插件。应测压缩/历史，不把原因先归到协议。 |

本轮调研没有建立当前原生 ACP 与本 headless adapter 在同二进制、账号、模型/effort、上下文、工具下的严格质量基准。因此既不能断言 ACP 必然降智，也不能宣称两条接法已完全等价。

### 3.3 备选方案

| 方案 | 决定 |
| --- | --- |
| 只保留 headless | 对当前文本自动化有效，但原图和交互能力缺口仍在。 |
| 显式增加原生 ACP | **建议方案。** 复用官方 CLI，能够独立验收和比较。 |
| 立即把默认全部改成 ACP | 在正确性及对照证据完成前不采用。 |
| 接旧 `.par` 或第三方 wrapper | 超出范围；会引入额外运行时并混淆生命周期和性能依据。 |
| 接内部 hub RPC | 超出范围；已有 UI 无需复制该链路，也不假定内部 RPC 有公开稳定契约。 |

## 4. 所有权与协议边界

独立插件拥有两种 transport。公共模块继续负责输入校验、订阅检查、图片 manifest、session lease、journal 持久化、取消和回执构造；transport 模块仅转换原生 wire 及版本化观察。Flywheel 导入此公开 API，不再嵌入另一套 ACP client 或独立解析 ACP。

第一版坚持**一次 invocation 拥有一个 `agy acp` 进程，至多一个活动 prompt**。scoped 调用可以在新进程中加载同一原生历史，保留现有会话复用政策；关闭调用不删除原生历史。这保留现有进程所有权边界。热复用属于需要测量的独立优化，不是基础 ACP 可用的前提。

除非 handler 已实现、受策略约束并通过测试，初始 client 不宣告宿主文件系统、terminal 或 elicitation 能力；原生工具仍由 CLI 运行。会话建立时不传入额外 `mcpServers`；agent 的 `mcpCapabilities` 表示支持的 MCP transport，不代表获准引入 server [S5]。对未宣告的可选服务请求明确失败，不能临时在 UI 进程中读文件或执行命令。基础 permission request 仍必须有真实 handler。

`clientInfo` 使用真实插件/client 名称及版本。记录可执行文件路径、版本、非秘密的二进制身份、adapter 源码/包身份、transport、协议版本及能力摘要。预检与执行解析到同一程序；原位替换二进制或配置使准入失效。摘要中不包含 token 或凭证内容。

### 拟议接口分配

以下是**未来设计示例，当前配置不接受**：

```json
{
  "profiles": {
    "agy-acp": {
      "harness": "antigravity",
      "transport": "acp",
      "model": "gemini-4-hydrogen-medium",
      "thinking": "medium",
      "channel": "google-subscription"
    }
  }
}
```

示例假定完整配置已定义 `google-subscription` 原生账号 channel 和现有订阅策略；不是可独立安装的完整配置。

| 界面 | 拟议契约 |
| --- | --- |
| 模型/角色/执行绑定 | 增加可选 `transport`，值为 `"headless"` 或 `"acp"`，仅对 `harness: "antigravity"` 合法；省略为 headless。沿用 profile 展开，并进入全部冻结绑定。 |
| 独立插件设置 | 相同选择器，默认 headless；执行工具调用者不能用覆盖此字段扩大授权。 |
| 公开 machine API | 为 ACP 及明确 transport 事实分配新请求/回执协议，保留当前 protocol-1 headless 读取。旧 reader 必须拒绝 ACP，不能静默忽略字段。 |
| Flywheel worker/lifecycle/recovery | 协商精确 adapter 的支持版本，所有路径透传 transport 并验证回执一致。写入旧引擎可能误解的事实前，分配所需的新 outer protocol/ledger 版本。 |
| 能力 | 按二进制、模型/mode、adapter 身份区分 advertised、local-verified、live-verified、unsupported/unknown；单一 `acp: true` 不够。 |
| 运行配置 | 分开保存 requested、resolved、observed 模型/effort/权限。请求回显不等于后端独立观察。 |

实现 PR 必须一起确定公开类型、bridge、Go owner、state、recovery 和 UI 的版本及 schema，不能仅向严格 reader 塞一个可选字段。原 headless journal 保留原身份和版本。

## 5. 初始化、模型选择与派发准入

业务 prompt 前按顺序执行：

1. 校验请求、物理目录、自有恢复位置、平台、剩余期限，并执行现有订阅约束。
2. 启动自有 ACP 进程，使用已支持协议和真实 client capabilities 初始化并核验响应。必要能力未知时，在派发前失败。
3. 新建 Session，或加载冻结 scope/binding 允许的精确自有 Session，不取 latest。历史 replay 不计为新工作或新增用量。
4. 读取返回的配置选项，使用已验证公开接口设置准确模型/effort 和权限；有 config options 时优先使用 [S7]。选项 ID/value 来自运行时，不能假定固定为 `thought_level` 或 `thinking_level`。
5. 要求返回已接受配置，记录该证据的来源和强度。模型缺失、effort 不支持、cwd/mode 不符、能力变化均阻止准入。Orchestrator 固定 effort 仍保持固定，不能把空值补成 high。
6. 冻结完整 prompt 和图片 manifest；需要时核对受管 agent；持久化 dispatch-possible 并收到 Go lifecycle ACK 后，才写入 `session/prompt`。

握手或目录查询不执行业务推理，也不证明账号权益或额度。若读取会话目录需要创建原生历史或触发原生辅助工作，须标明副作用，不能称为零副作用探测。普通无推理诊断先沿用现有 CLI 目录；ACP 专用 session probe 在隔离、自有的 workspace/history 中执行，不发送业务 prompt。

派发后的配置变化只是观察，不能据此重新解释原请求。无法解释的模型/权限改变应失败，并保留已派发工作的未知状态。DSH/Flywheel 只在安全边界选择下一次调用，不静默进行本轮模型切换。

## 6. 调用、取消与恢复

journal 同时绑定 Flywheel request ID、adapter run ID、原生 Session ID、connection generation、JSON-RPC prompt request ID。client 生成的 RPC ID **不是** provider 幂等键或原生 turn ID。第一版每个自有会话至多一个未完成业务 prompt，以保证事件归属可以机械核对 [S16]。

| 事件或中断 | 必须行为 |
| --- | --- |
| prompt 字节可能写出前失败 | 只有派发门和清理证据允许，才能记录 proven not-dispatched。 |
| 尝试写入后失联、坏帧/超限或超时 | 无可信关联终态时，保留 possible/accepted dispatch 与 unknown outcome，不自动重发。 |
| 正常 prompt response | 核对关联 stop reason、输出与配置。持续处理受支持的 prompt 后续工作，达到已验证完成/shutdown 边界，再确认进程组清理后才消费成功。prompt response 本身不是 adapter 最终回执。 |
| 已发送 `session/cancel` | cancel 是 notification，不是工作已停止的证明。对所有 pending `session/request_permission` 回复协议的 `cancelled` outcome；root prompt 仍 pending 时，持续接收更新直到其 response [S16]。确认自有工作清理，必要时有界 TERM/KILL。 |
| 部分文本后取消、拒绝、资源截断或错误 | 保留部分输出作为证据，不提升为任务完成；映射现有码表，并保留原生原因。 |
| 迟到进度或历史 replay | 核对 Session/generation，不串入前一会话内容，不伪造新调用或重复计费。区分合法的 prompt 后续活动与推翻已声明最终 quiescence/清理证明的工作。 |
| 引擎/worker 重启 | 只读原 durable journal，对合法终态消费一次；能加载 Session 不证明丢失 prompt 是否已执行。 |
| 缺少终态或清理证明 | 按现有规则 blocked；不冷启动、不换 transport、不重置预算，不根据空 UI 猜测安全。 |

ACP turn cancellation、本地进程清理、未知远程工作的取消是不同事实。不能仅因有 `session/cancel`/`session/load` 就把既有 `providerWork.cancelSupported`/`querySupported` 设为 true；较窄的 transport 能力应单独记录。

可选子会话契约允许 root prompt response 之后继续发生 child 和 parent 活动 [S8]。有界等待期间仍处理自有更新及权限请求，adapter 最终回执不能抹去已知未完成的必要工作。期限内无法收敛时，保留部分输出/证据、unknown 调用结果及取消证据。该 response 之后取消 parent，不再有 pending prompt response 可以确认取消；应要求已测 shutdown/quiescence 和明确清理证明，不能伪造 cancelled 终态。不能把合法的 prompt 后续更新误判为协议违规，也不能从杀进程推断 child 成功。

统一回执仍区分执行结果、终态、清理、requested/observed 配置、原生协作、图片、用量与 provider work 不确定性。一次调用一个进程时，成功仍要求正常自有进程边界；未来常驻进程必须引入明确 quiescence 证明，不能伪造每一轮都有退出码。

Session 复用继续按任务、角色、目标、物理 workspace、transport、账号/配置引用、模型/effort、权限、native mode 和输入契约隔离。独立 reviewer 不继承 executor 历史。跨通道复用在明确兼容测试/迁移契约前关闭；换通道不能退役 pending unknown 调用。

压缩继续以权威契约和 full/verified-incremental 政策为依据。原生 compaction 元数据只在支持时记录，缺失保持 unknown；历史 replay 或 summary 不能覆盖已批准计划、写范围或验收。测试压缩前后的任务约束保留，而不以 transcript 长度判断连续性。

## 7. 权限、订阅与 evidence-only

### 订阅和配置

复用 headless 环境过滤及本地约束：subscription-only 下拒绝 API/ADC/GCP/custom-provider 路由，保持 credits fallback 关闭，派发前后复核有关配置。ACP 返回哪些认证方式，不代表用户授权切换计费渠道 [S4]。

使用既有原生登录。认证失败返回 AUTH，提示用户在任务外走官方登录流程；不读/复制 OAuth token，不在无人值守执行中启动登录，不切账号，不改用 API key。`verified-local-controls`、账号权益和实际账单/用量仍是不同事实。

### 工具权限

每个权限请求/响应都绑定自有 Session、invocation 和 request identity。应用现有显式授权与原生 deny。`selected` outcome 只能使用 server 实际提供的 `optionId`；取消则返回协议的 `cancelled` outcome，不能编造 option 或留下未回答的 pending request。没有相应授权时，不写持久 allow-always、不扩大目录。原生 plan/accept-edits 标签本身不能证明 OS 隔离。

初始无人值守 Flywheel 只接纳 adapter 已证明支持的权限组合。超出已有授权的新 approval 明确拒绝/报告，不无限等待，也不自动同意。独立交互式 DSH 可使用已验证宿主 approval bridge；缺少桥时在发任务前拒绝交互模式。client 侧 fs/terminal 和外部 MCP 在单独实现及测试前保持关闭。

### Evidence-only

JEV 继续使用同一个 `jev.enabled` 政策作 fresh、无工具决策；换 transport 不得让它委派、使用 Orchestrator/Swarm 或获得工具。

只有固定运行时能够证明同等受管 agent 身份和不可变配置，ACP evidence-only 才可准入：禁用 tools/MCP/skills/plugins，command execution 关闭，agent 绑定 run ID/digest，派发前及清理时验证，没有实际工具或子代理步骤。非空的工具目录本身不是违规，实际工具请求/步骤才是。

原生 ACP 是否提供足够的 active-agent 证据，是**待验证项**。传入 `--agent` 或检查文件本身不证明活动 Session 已选择它。在 server/adapter 能提供同等证据前，明确禁止 ACP 用于 JEV 和调查分析；前置修复后的 headless 继续可用。不得删除 `evidenceOnly` 来通过准入。

## 8. 图片与原生协作

### 原图送达

准入要求协商的 image prompt capability，并验证 adapter/model 路径 [S5、S6]。复用当前授权图片和冻结 manifest 规则：仅从授权根中的允许普通文件读取，限制格式/大小/数量，在派发前绑定 MIME、bytes、SHA-256，保存/准入计划时重检身份。路径或 resource URI 不是原始像素已送达模型的证明。

transport 将冻结字节编码为 image content blocks。journal 保存输入 manifest 和持久送达事实，不把凭证或 base64 倒入进度日志。区分本地序列化、prompt 接受、真实端到端视觉验收；这些都不能单独证明未知子代理收到图片。live 验收使用无法仅从文件名、附带文字或仓库元数据得出答案的图片。

只有共享的已验证能力谓词，才能替换当前按后端限定的图片准入，并同时供引擎和 UI 使用。在完成验证前保持 Claude 要求。图片不支持时在派发前失败，不退化成纯文本，也不移除 packet 图片声明。

### Orchestrator、显式 subagents 与 Swarm

| 模式 | ACP 实现要求 |
| --- | --- |
| 普通 native / Orchestrator | 保持精确模型选择和固定 effort，区分自主使用 children 与实际观察到的 children。 |
| 显式 subagents | 固定运行时必须有已验证委派机制，且获得符合现行契约的实际子会话观察；无观察时不能认证此模式。 |
| Swarm | 只允许 Execute + Full + 已审独立 collaboration/writes + 输入条件成立。验证原生命令/机制，不能把任意形似 slash 的文本当成已调用。 |

ACP 的子会话展示可选；规范允许 agent 委派但不暴露 child Session [S8]。分别记录请求模式、实际 child ID、根完成和 child outcome；未观察的模型、用量、状态保持 unknown。观察数量 guard 不是原生并发硬上限，根 prompt 完成也不是所有 child 成功的独立证明。

除 CLI 外，还需锁定 ACP schema/SDK。只有对应子会话 handler 已实现，才宣告文档中的 v1 client `subagents` 能力；所选版本中仅有 draft 的变体须独立显式准入。必须先由合法归属事件把 child 连接到本 invocation 自有 root，才接受它的更新。client capability 请求的是展示，不负责开启原生委派，也不保证收到子事件。

图片还需区分根输入已验证、必须看图的 worker 送达已验证。现有 Swarm 图片排除应保留到受审契约可以表达 worker 输入需求，且传递验证通过。下游纯文本工作未来可使用经过评审的衍生规格，但删除原图需求不是迁移方法。ACP 本身不改变这些规则。

## 9. 配置、UI 与诊断

transport 是 Antigravity 执行绑定的高级属性，不是新的 provider 或模型；与 `native-profile`、思考等级及 JEV 开关独立。

UI 必须：

- 编辑既有绑定时保持 headless；只有已安装 adapter 支持时才提供 ACP。
- 对初始配置、下一次已选绑定、最近实际调用分别展示各自 transport/configuration 事实。
- 展示原生协议、CLI/adapter 版本及具体不合格原因，不把目录发现解释为账号权益。
- 对角色、Full/collaboration、权限、图片及 evidence-only 使用引擎共享资格结果。
- 区分当前编辑作用域与任务有效作用域，保存后重读有效配置。
- 区分 advertised、tested、unknown，包括根与 worker 的图片支持。
- 保持可读模型名称和决策理由，原始协议 ID 放在展开详情中。
- 对 pending invocation 禁止不安全修改，保留 stale/unknown 请求状态。

本方案不修改初始模型默认值或既有任务。未来目录刷新需把异步结果绑定到准确 transport/编辑目标/配置 revision，沿用已有 provider/model 结果的关联方式。

## 10. 迁移、发行与回退

1. 独立完成回执/打包修正并验证 headless；历史源码、registry、live 证据分别保留。
2. 独立插件实现原生 ACP，明确公开协议/能力版本、文档及隔离包消费测试；保持 protocol-1 headless 和原恢复记录可读。
3. 经单独授权发布已测插件版本，Flywheel 再锁定精确包身份。schema、driver/bridge/recovery/session identity、诊断和 UI 在一个完整实现 PR 中更新。
4. 省略 transport 继续 headless；旧 adapter/engine 遇到 ACP 明确拒绝，不静默退回。写 ACP state 前增加兼容标记，阻止旧 reader 误解。
5. 明确开启已准入的文本 ACP；图片、evidence-only 分别按门禁开启。不改写既有输入、批准或历史回执。
6. 只有安全边界上的空闲配置才能回到 headless；保留原 journal/reader 供 ACP 调用对账。未决 ACP turn 阻止通过 headless 重放；回退不等于取消或未派发证明。

共享常驻 ACP 进程、同 server 并行 Session、自动 steering 属于后续提案/门禁。Flywheel 反馈继续在现有安全边界排队采用。并发 `session/prompt` steering 会改变冻结输入及调用归属，必须先有版本化授权/input revision 记录，不能仅加一个转发文本的按钮。

## 11. 验证与对照评估

### 11.1 正确性门禁

| 范围 | 能力准入前必须覆盖 |
| --- | --- |
| Wire/配置 | 不支持协议/选项，准确模型与 effort，二进制/配置变化，能力不符，有界 Unicode/JSON frame，未知可选消息与非法必要消息。 |
| 权限 | 每种支持模式、拒绝请求、错会话/迟到 approval、根外操作、禁止认证路由、运行中设置改变、evidence-only 错/被改 agent 与实际工具/child 步骤。 |
| 生命周期 | 派发前后、工具/权限中、root response 后仍有活动 child 时取消；超时后下一次 prompt、transport 丢失、父进程死亡、SIGKILL、缺失/重复/矛盾终态、迟到事件和进程组清理。 |
| 恢复 | 每个 durable ACK 周围崩溃、收到回执但未消费、一次本地消费、unknown 仍阻断、精确历史恢复、不隐式重放或跨 transport 冷启动。 |
| 上下文/计量 | 历史不重复注入、角色隔离、full/incremental、压缩保留任务约束、累计量不当逐调用相加、缺失/负值/不一致差值保持未知。 |
| 图片 | 实际冻结字节、源变更拒绝、MIME/体积限制、不支持能力、根视觉任务、必须的 worker 传递及不可用时拒绝。 |
| 原生模式 | Orchestrator 固定 effort、显式 subagents 有/无观察、Swarm 命令接受、Full/批准范围、无 child telemetry、数量 guard 与根/child 完成区分。 |
| 分发/UI | 干净公开包导入、版本拒绝、同候选 Linux/macOS 安装、随包文档/溯源、共享准入、作用域与异步模型/transport 选择器身份。 |

沿用当前仓库门禁：Go 改动要求逐函数 100% measured statement coverage；插件行为/跨进程 bridge 检查；发行改动要求真实候选安装。不降低门禁，也不用冒称同版本的假包替换公开 adapter。离线 fixture 只证明契约；所选 CLI、账号、模型、图片和原生模式仍需独立 live 记录。

### 11.2 A/B 设计

实验前冻结任务、评分和接受界限。对比**相同**官方二进制字节、账号/认证、模型/effort、设置/rules/plugins、工具授权、workspace snapshot、输入和历史/压缩策略。随机或交替路径顺序，每个场景至少做十次配对重复作为探索报告，记录全部失败。冷试验用新建的匹配 Session；热试验用等价前序轮次。不把无法控制的后端缓存状态写成已控制。

覆盖短证据判断、带确定性测试的修改、多文件修复、长上下文续接/压缩、图片任务和已审协作。某路径不支持的场景属于能力结论，不是数值性能胜出。live 工作只在已授权的可丢弃 workspace 运行，不发布内容、不修改用户 profile。

分别记录：

- 进程启动、ACP initialize/headless ready、Session create/load、prompt dispatch、首事件、首可见文字、完整结束/清理耗时。
- 实际接受的模型/effort、可观察的模型/工具调用数、带 scope 的 input/output/thinking/cache tokens、压缩观察和 quota 错误。
- 确定性测试、正确性/rubric、约束保持、重复工作、遗漏工具及失败/unknown 结果。
- 终态/取消/恢复正确性、进程数量和峰值 RSS；缺失 telemetry 仍标缺失。

报告中位数、尾部、配对差异和不确定性，提供原始分布及失败数量，不只报一个百分比。改变默认须无未解决的正确性/权限回归，并在预先声明的延迟/资源界限内产生有价值的任务结果。界限应在看结果前决定；本文不声称有实测结果或已选定安全默认版本。

常驻进程实验必须同时比较热 ACP、热 headless stdin 和当前每次启动的基线；否则测得的是进程复用与协议的混合效果，会夸大 ACP 独有收益。

## 12. 里程碑与待决项

| 里程碑 | 交付与退出条件 |
| --- | --- |
| M0 — 修复当前行为 | 受管 agent 回执修复、非空目录跨层回归、打包/溯源修正；保留 headless 默认。 |
| M1 — 原生 ACP 一致性 | 固定 CLI、协议/options/auth/权限记录、自有文本调用及 journal/cancel/recovery 测试；仅显式准入的文本配置。 |
| M2 — 公开集成 | 独立插件发行、Flywheel 精确锁定、严格协议/ledger 迁移、UI transport/有效配置控件及候选安装。 |
| M3 — 独立能力 | 根图片、evidence-only、native mode 各通过离线/live 门禁；worker 图片传递仍单独条件化。 |
| M4 — 评估 | 可复现配对任务报告，据结果采用、限制或拒绝扩大/默认使用；热复用/steering 仍需单独 review。 |

实现完成前必须解决并记录：

1. 精确 CLI build 及真正支持的可选 ACP 方法；模型/effort/mode 映射，以及不用冒名即可访问的账号模型。
2. ACP 是否提供足够的 active-agent 证据；不足时该角色继续使用已验证 headless，并明确限制。
3. machine/worker/lifecycle/state 的版本分配，以及 load/replay 与当前 prompt 的明确归属。
4. 原图是否能到达每个需要看图的 worker，以及受审契约如何表达该要求。
5. 原生 shutdown/quiescence、上下文/计量 scope，以及 Session resume 是否在历史访问以外提供了额外的**已证明**恢复能力。
6. 预登记的评估任务、质量标准、延迟/资源界限，以及哪些实际收益值得维护额外通道。

建议先完成 M0，再推进明确的 M1/M2 实现方案。批准本文本身不发布包、不为已有任务开启 ACP，也不认证用户已安装环境。

## 来源与验证边界

资料核对日期为 2026-11-22。官方文档说明接口受支持；issue 说明特定条件下的反馈，不能替代本地一致性或真实验收。本次文档 PR 未运行真实 ACP 推理、质量基准、图片传递测试或用户机器安装。

| ID | 来源 | 用途 |
| --- | --- | --- |
| S1 | [Google：CLI ACP](https://antigravity.google/docs/cli/acp) | 官方原生入口与编辑器集成。 |
| S2 | [Google：headless mode](https://antigravity.google/docs/cli/headless) | 自动化、持续 stdin、纯文本输入。 |
| S3 | [Google：changelog](https://antigravity.google/docs/changelog) | CLI 1.5.0/1.5.1 历史及生命周期修复。 |
| S4 | [Google：ACP 编辑器集成](https://antigravity.google/docs/ide/extensions/acp) | 认证、flags、会话能力；模型限制文字与 S3 冲突。 |
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

[C1]: https://github.com/ckanner/flywheel/blob/291c0a02f9042d522d3e0f3c62977f63149465cd/internal/drivers/antigravity.go
[C2]: https://github.com/ckanner/flywheel/blob/291c0a02f9042d522d3e0f3c62977f63149465cd/plugins/dsh/src/antigravity-bridge.js
[C3]: https://github.com/ckanner/dsh-subagent-antigravity/blob/a20eb63bbfa2849df63043595475957bf4a4f04d/lib/runner.js
[C4]: https://github.com/ckanner/dsh-subagent-antigravity/blob/a20eb63bbfa2849df63043595475957bf4a4f04d/lib/machine.d.ts
