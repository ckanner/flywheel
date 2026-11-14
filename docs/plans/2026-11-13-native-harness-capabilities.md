# 原生 Harness 能力在 Flywheel 各环节中的接入方案

[English](2026-11-13-native-harness-capabilities.en.md) · [方案索引](README.md) · [当前架构](../design.md)

**状态：调研与待讨论方案，尚未实现。** 研究日期为 2026-11-13（America/New_York）；
Flywheel 基线为 `63d8781ee1ece0cce443fa2e1ead62d1d6288817`，包声明版本 `0.32.1`。
本次修改只增加方案及导航，不启用原生协作、不修改模型默认值、用户配置或运行协议。
下文的配置概念、回执字段和阶段均是提案，不能直接写入当前配置文件。

## 1. 建议结论

应该利用各个 harness 的原生能力，并按它们实际提供的接口接入。近期最值得做的是
**工具并行、结构化结果、少量有明确边界的子代理**；随后分别引入 DSH Team、Kimi
Server Swarm 和 Claude Workflows。Kimi Tower、Claude Agent Teams、远端 ultrareview
需要单独的生命周期与所有权设计。

推荐保留三层责任：

| 层级 | 负责什么 | 对下一层的交付要求 |
| --- | --- | --- |
| Flywheel | 正式 Spec/packet、跨任务调度、账户与预算、工作目录、验证、独立评审、恢复、Git 交付 | 冻结一次调用的范围、候选、权限和资源额度 |
| 一次调用的适配器 | 选择并核验原生能力配置，关联父子运行，等待结果和清理，归集用量 | 原生运行不能在外层调用结束后留下无法解释的责任 |
| 原生 harness | 工具使用、内部探索、子代理通信、限定范围的并行或工作流 | 返回可归属的结果、实际执行事实、用量和终态 |

原生协作的价值是减少单个模型上下文里的杂乱信息、并行处理独立问题、使用后端已经
维护的工具和运行机制。这些是采用它们的理由；是否缩短交付时间、降低返工或增加
费用，仍需同条件实验。原生 task 的 `completed`、Team 的内部 review 或自然语言
“完成”，都不能直接替代 Flywheel 对同一候选的验证和验收。

## 2. 先区分模型、Provider、Harness 与适配器

模型决定推理、编码、视觉等能力；Provider 负责模型接口和账户接入；harness 负责
工具循环、上下文、子代理及运行方式；Flywheel adapter 决定这些能力中哪些已经进入
我们的可控制协议。相同模型经不同 harness 调用，能使用的运行能力可能不同。

| 当前路径 | 实际运行边界 | 不能据此推断的能力 |
| --- | --- | --- |
| `harness=dsh`，`provider=deepseek-account` 或 `chatgpt` | Go → DSH SDK worker → DSH 原生 agent/tool loop → 模型 Provider | DSH 使用 GPT 模型，不会自动获得 Codex 原生子代理或 app-server 功能 |
| `harness=kimi` | Go → 原生 Kimi Code CLI `-p` / `stream-json` | 网页产品、交互式命令、Server API 的能力不自动进入 print 路径 |
| `harness=codex-pro` | Go → 用户的 `dsh-subagent-codex-pro/machine` → Codex app-server | 插件名称包含 subagent，不表示 Flywheel 已管理 Codex 内部的子代理树 |
| `harness=claude-code-pro` | Go → 用户的 `dsh-subagent-claude-code-pro/machine` → 本地 `claude -p` | CLI/SDK 支持某功能，不表示当前 machine 请求和回执已经暴露它 |

当前实现依据见 [F1]–[F6]。跨 Provider 的任务分工仍适合由 Flywheel 调度；不增加一个
DSH 父模型，只为再次委派给已经可以直接调用的 Codex/Claude machine API。

### 2.1 版本与证据边界

| 对象 | 本次核对的基线 | 证据限制 |
| --- | --- | --- |
| Flywheel | `63d8781` / `0.32.1` | 源码和包声明，不代替用户机器的实际配置 |
| DSH | 官方 `dsh-v0.2.2-rc.3`，源码 `38a6a5c965aa99ab525161efb7cc5cf1fe62fced` | Flywheel devDependencies 为 rc.3；宿主兼容范围还包含更早版本，不能一律判为支持新 Team |
| Kimi Code | 官方 release `@moonshot-ai/kimi-code@2.6.2`；检查源码 `f082f04a093a3e71a2263c5461d52507dc31f83a` | 源码快照不是 2.6.2 安装包验收；Flywheel driver 的既有契约说明仍以 2.4.1 print 为检查基线 |
| Codex Pro | 固定 machine `0.9.0`；vendor 记录源码 `37da3d75de32b809c4fb1a631ccc15191f3d87f9` | 原生 Codex 二进制单独安装；插件版本不能证明 CLI 能力版本 |
| Claude Code Pro | 可选 machine `0.4.0`；仓库记录 source candidate 为 `ec0b99176b67512f3ca1838ec2a349dcb0189214` | 原生 Claude CLI、登录权益和实验功能另需探测；上游其他提交不代替该 artifact 的来源 |
| 官方在线文档 | 2026-11-13 检查 | 文档更新、实验功能、实际安装版本和账户可用性分别记录 |

Kimi 当前维护项目是 `MoonshotAI/kimi-code`；旧 `MoonshotAI/kimi-cli` 已归档。
不能拿旧 Python CLI 的 Wire 文档直接设计新 TypeScript CLI 的 adapter。[K1][K2]
本轮没有启动真实模型、检查用户订阅权益或验收 Mac 桌面；旧工作区的 rc.2
`node_modules` 也不作为当前 rc.3 发行或用户宿主的运行证据。

## 3. 各后端值得利用的能力

### 3.1 DSH：工具并行、本地子代理与持久化 Team

| 能力 | 上游事实 | Flywheel 当前状态与建议 |
| --- | --- | --- |
| PTC `run_code` | 独立的并行安全工具可在一个程序内并发；排他调用形成屏障，`maxParallelSubCalls` 默认 10 | 当前 overlay 没有专门禁用 PTC，但实际 executor 取决于 profile。优先探测并验证工具并行，不能声称默认已经启用或必然节省 tokens |
| 本地 SubAgent | fresh/fork、独立 Session、消息和继续执行；tree pool 默认最多 8 个活跃 children | 当前 worker 明确限制委派。适合首先扩展少量受限的探索助手 |
| Agent Team | 在现有 subagent tree 上增加持久化任务板、依赖和 CAS 认领 | 适合一次调用里的协作，不替代正式 packet DAG |
| Web search/fetch | 可分别开启；普通抓取不必调用辅助模型 | 当前整个 `tool-web` 被关闭。应按实际后端拆分普通检索与额外模型调用 |
| 原生 Plan、Goal、压缩 | 分别提供规划交互、自动多轮继续、上下文压缩 | 当前策略限制这些路径；后续按独立能力处理，不能随 Team 一并打开 |

上述能力来自 rc.3 发布源码 [D1]–[D8]。PTC 管的是工具执行；SubAgent/Team 管的是额外
模型运行，二者不能使用同一个并发或费用计数。

**Team 有 headless 接入方式。** 官方 SDK guide 使用标准 `sdk` profile，并以 patch
设置 `agent-team.config.defaultEnabled=true`；`sdk-minimal` 不包含 Team。该默认值只
作用于新 root Session，恢复旧 Session 不会自动应用。服务内部有 `setEnabled`，
公开 JSON-RPC 方法表没有可以直接假设存在的 `team/setEnabled`。[D1][D3][D8]

Team 共用 checkout；`writeScopes` 是冲突提示，不是文件锁，也不创建 worktree。
关闭 Team 不会自动取消 children 或清空任务板。原生 `session/wait` 等到 root idle
与正在推进的后代工作完成，但 parked child 仍可能驻留；最终还需 drain/shutdown。
标准 DSH seam 的外部 Codex/Claude/ACP children 是一次执行，不具备本地 children
相同的继续语义。[D1][D2][D8]

当前 Flywheel 不仅禁用了委派工具：`policy.js` 的 `agent/created` 只允许原 root
Session，bridge 遇到子派发事件会拒绝，usage fold 也只消费 root 事件。[F2]
因此需要扩展受管身份树、每个 child 的权限检查、事件归集与清理证明，不能只删除
禁用列表。原生 Plan 是指导而非工具权限隔离；Goal 的轮数限制也不是累计费用预算。[D6][D7]

### 3.2 Kimi：普通 Agent、Swarm 与 Tower 是三种不同能力

**Tower 确有其名。** 它会把大型工作拆成 mission，为 worker/reviewer 管理独立分支、
worktree、消息、review 和合并。它已经承担一部分完整编排器职责，不能当成普通
子代理开关放入现有 Execute。[K7]

| 能力 | 可编程入口 | 对当前 print 路径的判断 |
| --- | --- | --- |
| `Agent`，含 `coder` / `explore` / `plan` | print 可使用普通工具；新 Session 可传 `--agent-file` | 可作为第一批候选，但当前 driver 没有显式协作 profile、子账本或限额保证 |
| `AgentSwarm` | 需要显式 swarm mode；Server profile/prompt API 有正式字段 | print 没有正式的开启入口；不能只写提示词要求它自动开启 |
| Tower | 实验开关 + Server profile API；须先创建 Session 再开启 | 需要独立 adapter 和所有权设计，不能直接加入 print 参数 |
| 原生 Plan | `--plan` 用于交互路径，与 `-p` 冲突 | Flywheel Plan 继续产出自己的契约；可利用 `plan` 子代理协助探索 |
| Skills、MCP、Hooks | CLI 配置及启动参数 | 可以利用原生扩展，但 hooks 的错误/超时 fail-open，不能作为唯一准入门禁 |

普通子代理的工具与委派白名单会在执行前检查。`--agent-file` 只用于新 Session，不能
与 `--session`/`--continue` 合用；原生 child resume 还有白名单豁免。应记录有效
profile、工具、模式和模型绑定，改变约束后不得续用不兼容的旧 child。[K3][K4]
当前 Flywheel 首调只预分配自己的 Request ID，原生 Session ID 为空，成功收尾后才
接收 resume hint。因此新增创建时 `--agent-file` 参数不与现有首调身份协议冲突；
仍需把有效 profile 摘要绑定到调用与 Session，并在 resume 时重新核验。[F5]
Flywheel 当前没有显式禁用所有普通 Agent；原生配置可能已经让 CLI 内部委派。
准确现状是“尚未受管和证明”，不是“子代理一定关闭”。

Server API 是值得设计的新接入路线，无需改回 ACP。官方提供 session/profile/prompt
及 WebSocket task 事件；可以明确设置 `swarm_mode`，也能进入实验 Tower。
它仍是 Beta，应以安装版本的 `/openapi.json` 与 `/asyncapi.json` 核对契约。[K5]
新 adapter 应拥有自己的 runtime/Session 生命周期，使用经过认证的本地服务入口；
取消和退出只作用于所属工作，不能停止用户共享桌面服务或其他任务。

这里有四个必须处理的具体差异：[K6]

1. profile 更新逐字段生效而非原子事务；失败后须核对实际模式，不能认为全部回滚。
2. prompt 幂等记录仅在内存、每 Session 最多 256 项，重启清空；不能代替 Flywheel
   的持久化调用意图，也不能据此在断线后自动重发。
3. abort 主 turn 不会自动清空排队 prompt。还需逐 task 停止并确认后台工作收敛。
4. Session `usage` 目前是占位零，WebSocket context usage 也不是实际花费。
   应核对 turn/step 用量及父子去重，缺失部分保留 unknown。

Tower 的 `disabled_tools` 或禁止 `TowerMerge` 不足以证明所有 children、Bash 和
Git 操作都被外部编排器约束。本次未查到可直接把 Tower 降级为“只借 worker，完全
关闭其分派/worktree/merge”的文档化模式。未来可研究让它在独立仓库副本中承包
一个完整工作单元，只交回候选 patch/commit；Flywheel 在外部重新验证并接纳。
普通 worktree 共用 Git 元数据，不能作为阻止 Tower 改动正式分支的隔离证明。[K7]

### 3.3 Codex：保留原生工具与子代理，扩展已有 machine 契约

官方当前支持内置/自定义子代理。`agents.enabled` 默认 true，并发配置为
`agents.max_concurrent_threads_per_session`；旧 `agents.max_threads` 为兼容别名。
不应继续照搬早期实验开关说明。child 继承父 sandbox/approval，custom agent
文件不能另设这些权限；仅命名为 explorer 并不能建立独立只读沙箱。[O1]

app-server 提供 thread start/resume/fork、turn start/steer/interrupt、native review
和协作事件；安装的 CLI 还可以生成对应版本的协议 schema。[O2] 最小路径是在
**现有 Codex Pro 插件**中扩展公开 machine controls/events，再由 Flywheel 消费。
保留用户已有 Codex 登录，不为使用多代理而换成新的 API 计费渠道。

当前 machine 0.9.0 及 Flywheel bridge 以一个 root invocation/thread 为中心，没有
原生协作请求与完整 child 事实契约；现有 wire 会过滤其他 thread 的多种通知与用量。
也没有明确关闭所有原生子代理，因此不能把“没传 agents 参数”当成单代理证明。[F3]

建议先做只读探索/审查、native `review/start` 的结果适配，再评估 fork。
`thread/fork` 可从冻结的规划上下文分出实验，但正式 Reviewer 应保持独立上下文，
不直接 fork Executor 的私人过程来替代独立评审。原插件已有的结构化输出/图片能力
也值得透传；当前 Flywheel 请求尚未暴露这些字段，应单独做输入与结果契约。[F3]

### 3.4 Claude Code：子代理、Teams、Workflows、远端审查分开接入

| 能力 | 原生能力 | 建议用途与边界 |
| --- | --- | --- |
| Subagents | 自定义角色、工具、模型和独立上下文；headless stream 可关联子任务 | 小规模探索、假设验证、专项审查；沿现有 `claude -p` machine 扩展 |
| Agent Teams | 多 Session 的共享任务和消息，仍为 experimental | 有交互价值的调查实验；不能假定 teammates 能随 Session resume 恢复 |
| Dynamic Workflows | 脚本使用 `agent`、`parallel`、`pipeline` 编排；支持 `claude -p` | 同构批量分析、迁移和交叉检查；脚本内容、版本和结果范围应受管 |
| Ultrareview | Anthropic 云端多代理审查，有独立 CLI 子命令 | 后续独立 remote-review job，不能混入当前本地一次调用的完成语义 |
| Advisor | 在部分模型/账户条件下咨询另一模型 | 后续候选；与 JEV/第二意见的职责和计费分别说明，不默认开放 |

官方文档见 [C1]–[C6]。Workflows 的默认并发可达 16，累计 agents 上限 1,000；
CLI `2.1.269` 起支持 `CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS`。普通 Agent 的深度/并发限制
不是 Workflow 的限制，不能只管其中一种就声称控制了全部原生派发。[C3]

当前 Claude Pro 0.4.0 bridge 只接受 `danger-full-access`；默认读取原生 user/project/local
settings，但没有把 agents、tools、Team/Workflow 策略和子任务树暴露为受管契约。[F4]
父会话在 bypassPermissions 时，child 不能通过自己的 `permissionMode:plan` 收窄权限。
工具白名单可减少工具面，但不等于 OS 只读隔离。[C1] 所以不能自动将此后端填入
要求只读的 Review/Judge，也不应为接入新能力偷偷扩大或更改用户权限。

Ultrareview 尤其需要独立处理：`claude -p '/code-review ultra'` 不等待最终 findings；
脚本应使用专用 `claude ultrareview` 接口。本地等待结束后云端工作可能仍在继续，
重新启动又会创建一次新审查。未来 adapter 需要远端 job ID、查询/取消/核对与真实
账户条件；默认不发 PR 评论，使用、计费和发帖均遵守相应授权。[C4]

## 4. 模型优势如何用，而不把能力名称当成路由依据

本方案保持现有基线：DeepSeek Flash、Kimi K3.1 Pro、ChatGPT/Codex 订阅路径，Claude
仍为显式选用的可选后端。具体模型与 effort 见当前[模型策略](../zh-CN/model-policy.md)。
不引入未配置的模型、账号或付费后备渠道。

| 目标 | 值得验证的组合 | 当前可做出的结论 |
| --- | --- | --- |
| 常规代码定位、实现与工具密集工作 | DSH + Flash + 已核验 PTC；有需要时加少量 helpers | 可以研究减少工具往返；不能仅据价格或宣传断言总成本更低 |
| 前端、视觉输入、长任务规划 | Kimi K3.1 Pro + explore/plan；明确的图片/产物输入 | 官方模型提供图像/视频等能力，但当前 Flywheel 没有通用结构化媒体 manifest，需单独接入和验收 [M1] |
| 困难编码、代码导航和专门审查 | Codex 原生运行能力 + 已配置 Sol 模型 | 原生 app-server/subagents 可成为执行方式，不能由模型名称推出已受管 |
| 多视角最终判断 | 保持独立的 Review/Judge，按允许模型选择 | 同一个模型分出多个 agents 不自动获得异模型独立性，也不保证更高质量 |
| 大量相似分析、转换与检查 | 允许的 Claude 模型 + 已保存 Workflow；或 Kimi Server Swarm | 先固定任务、模型与脚本比较；不默认用更高价模型承担所有 helpers |

子代理默认模型、secondary model pool、effort、工具和账户继承都须进入实际绑定。
同一任务先比较“相同模型、不同原生执行方式”，再单独比较模型/effort；否则无法
判断收益来自协作、模型变化还是更多消耗。缓存命中、压缩开销、子调用 tokens 与
可观察订阅额度分别记录，不把 API 等价价格当订阅账单。

## 5. 各 loop 的具体使用方式

以下是推荐试点，不是当前已实现的自动选择规则。

| 环节 | 推荐做法 | 谁负责最终产物 |
| --- | --- | --- |
| 宿主准备 | 调研入口、约束与资料可借宿主原生能力；记录材料来源和已发生的准备工作 | 宿主准备契约，随后由 Flywheel 校验；准备消耗不能假称已全部计入引擎 |
| Intake | 简单分类/归并继续单代理；先利用工具检索与结构化输出 | 既有 Intake 契约和预算 |
| Full Plan | helpers 分查接口、调用关系、测试与兼容性，父 planner 汇总后写 Spec/packet | 单一正式 planner 写受限契约；native task 不直接变成 packet |
| Investigation | 不同 helpers 检查不同假设，交回证据和未证实项 | Flywheel 决定受管诊断与修复交接；研究结论不是实际诊断执行回执 |
| Execute | 先做主执行者写代码、受限助手研究；跨 packet 并行仍使用现有调度 | 原 packet 的 Execute/验证/交付路径 |
| Review | 正确性、恢复边界、兼容、测试充分性分角度审查同一候选 | 独立父 Reviewer 汇总原有 verdict；helpers 不修改实现或验收 |
| Judge | 必要时聚合额外证据，不机械地再开一个完整 team | 原 Judge 与冻结的里程碑验收；内部一致意见不能覆盖失败的 oracle |
| Garden | 将跨任务归纳、重复模式检查拆为独立检索 | 原有整理范围；不能借此重写已冻结任务状态 |
| Quick | 默认保持低固定开销；只在明确存在独立研究面时选少量 helpers | 仍保留独立最终 Review 和同一候选验证 |

**单 writer 是需要实现的边界。** 对无法单独约束 child 的后端，只在整次调用已是
只读时采用原生只读协作，或由 Flywheel 启动独立的受限 worker。不能把 full-access
child 的提示词“不要修改”当成只读证明。DSH 可研究在 `agent/created` 中逐 child
设置并核对更窄策略；Codex/Claude 的继承限制则需分别处理。

一个具体故障例子是：主调查者冻结当前代码与失败证据，两个 helpers 分别追踪错误
传播和检查相关测试；主调查者汇总可验证的假设；真正的复现命令由 Flywheel 受管
执行。若进入修复，原 executor 修改代码，独立 Reviewer 检查新候选。这样能够借用
原生并行和上下文隔离，同时保留原来“证据 → 修复 → 验证 → 独立评审”的顺序。

## 6. 统一请求意图，保留各后端真实的能力差异

### 6.1 能力声明与配置

统一的是需求与运行事实，不是把所有后端压成一个 `supportsTeam=true`。
未来 `NativeProfile` 应描述下列信息；名称和存储格式待实现时定义 schema：

| 内容 | 需要记录的事实 |
| --- | --- |
| 身份 | harness、adapter、binary/schema、profile/工具配置摘要、模型、provider、账户渠道、cwd |
| 请求方式 | 普通工具、只读协作、单 writer、同构 fan-out、内部 task board、隔离 mission、远端 review |
| 执行限制 | 实际 child 上限、深度、总任务数、全局预留、剩余 wall time、允许的模型和工具 |
| 权限 | root 与 child 的实际能力；OS sandbox、工具白名单与纯提示约定分别标记 |
| 观测范围 | 完整 child 事件、可信 aggregate、或 unknown；不能捏造逐 child 数据 |
| 用量与资源 | 可观测口径、父子聚合规则、是否有真实硬限额、缺失和在途用量 |
| 生命周期 | 创建/可能派发/终态/清理/恢复支持，后台任务和远端任务的责任边界 |

配置按角色或执行路线引用这些 profile，避免给每个 Provider 复制一组语义不一致的
布尔开关。UI 可以提供“关闭 / 按阶段使用”和阶段方案选择，并显示“请求、可用、
实际采用、拒绝原因”。高级限制放在对应方案里。PTC、媒体、检索、远端服务等能力
各有独立准入条件，不被一个“协作开关”全部无条件打开。

`off`、`prefer`、`required` 是拟议的选择语义，不是现有配置字段：

- `off` 要核验原生 delegation/workflow 等实际关闭；没传开启参数不能代替证明。
- `prefer` 只有在证明未发生任务相关的模型/工具派发、清理已确认且原基线满足相同
  约束时，才可选回普通路径。受管、已确认完成的无推理探测单独记录；未知探测责任
  仍会阻断回退。启动钩子、Session resume 也可能有活动，启动失败不自动满足条件。
- `required` 在首次可能派发前发现缺少必要能力就拒绝，不静默换模型、权限或账号。

可能已经派发、已有 child、未知工具效果或未知清理时，不自动换一个实现重新做。
能力选择与回退理由进入原调用记录；不能让 SDK、adapter 和 Go 各自重试一轮。

### 6.2 并发和预算：采用可以证明的控制粒度

第一阶段可采用**整组预留**：在启动一个 invocation 之前，Go 为 root 与允许的最大
children 一次取得额度，native runtime 同时落实可验证的 child cap。例：root + 最多
2 个 children 占 3 个槽位，全局 6 个槽位最多接纳两个这样的调用。数字只是试点示例。
先预留整组再启动，避免多个 root 已占槽后都等待 child 额度。

这不要求每次 native spawn 都有一个不存在的 Go RPC。只有后端实际提供可信准入
接口时才优化为动态借还。不同原生派发机制可同时存在时，应预留它们的合计上界，
或明确配置为互斥；没有可验证内部 cap 的 Swarm/Workflow 不能承诺严格全局并发。
停止中或清理未知的组不因 root 已返回就释放责任。

工具/I/O 并发单独计数。文件读取和普通 HTTP fetch 不占额外模型槽；工具内部调用
模型、启动子代理或工作流时必须进入相应模型预算，不能通过 PTC 绕过。

并发 cap 不等于累计 token/金额硬上限。事后 usage、停止阈值和异步取消只能给出
相应的观测与软控制，可能有在途消耗。要求严格累计预算时，只接纳确实提供该能力
且已验证的 profile；缺失 usage 保持 unknown，不记为零，不以订阅等同无限额度。

### 6.3 运行账本、会话与取消

沿现有 invocation journal 扩展归属，必须记录引擎的 root invocation 身份，并关联
run、角色/packet、Request ID、profile 摘要、候选与结果/清理状态。原生 root ID 在
协议确认后绑定；未确认时保留不确定性。原生确实提供 child/task/activation ID 和
父子关系时一并记录，不为只有 aggregate 的后端补造身份。
有完整 child 事件就记录完整树；只有可信 aggregate 就如实记录 aggregate，不能
满足要求逐 child 权限或恢复的策略时拒绝该 profile。

用量以原生请求身份去重。必须选择“已包含 child 的可信父汇总”或“去重后的逐 child
汇总”，不能两者相加。历史累计、上下文大小、API 等价估价、真实账单分别保留。
child 返回内容、root 返回、后台任务停止和整个调用可安全结束是不同事实。

取消顺序应为：关闭新派发 → 停止/清空排队输入 → 取消所有已知原生工作 → 等待协议
终态及实际清理 → 保存回执。未知责任继续阻断；原生取消能力不足时走已有进程组
收束，但进程退出也不能替代远端 job 终止证据。

会话复用绑定角色、目录、模型、账号、权限和 native profile。旧 board、旧 mode、
恢复的 children 不自动继承新调用的授权；改变限制时不得靠 resume 绕过。预热仍
不创建正式 Session、child 或模型调用：无推理 composition probe 与正式权限准入分开。

### 6.4 与 JEV、scheduler 和已有验证的关系

JEV 只对 Go 已按真实能力、权限和剩余额度过滤后的候选提出建议。JEV 关闭时，静态
角色策略照常使用已经获准的原生能力；开启 JEV 不会自动开启 Team/Swarm/付费服务。
配置上保留现有单一 JEV 开关，不增加另一个影子开关。

`scheduler.dispatch` 继续控制正式 packet 的调度，`scheduler.prewarm` 继续控制
符合条件的 DSH 初始化准备。它们都不是原生子代理开关。L1/L2/L3、独立 Review、
保护路径、相同候选绑定与 Git 交付由原有引擎负责。

## 7. 建议实施顺序与完成条件

| 阶段 | 交付 | 可以进入下一阶段的条件 |
| --- | --- | --- |
| P0：能力与控制契约 | 各 adapter 的 probe、有效配置、观测粒度、明确 off、资源预留、原调用事实；扩展用户现有两个 machine 插件 | 未支持/未知场景不会误报 available；继承配置与子派发都有明确处理；不要求所有后端虚构相同 API |
| P1a：低协调开销能力 | DSH PTC/普通检索的受管验证，按后端透传结构化结果；媒体另做输入 manifest | 工具边界、失败、输出和用量口径验证；模型结果仍通过原有 schema |
| P1b：少量子代理 | 先 DSH 的受限本地 helpers，再按实际契约接入 Kimi Agent、Codex、Claude | child cap、权限、结果、用量与取消可证明；不能满足只读要求的组合不进入试点 |
| P2：有明确任务结构的协作 | DSH Team task board、Kimi Server Swarm、Claude 保存的 Workflow，分别交付 adapter/profile | 公开 API、模式/恢复、全部派发机制、后台任务、去重与全局额度矩阵通过 |
| P3：更复杂的能力 | 交互式 Teams、显式压缩控制、隔离多 writer、Tower 工作单元、远端 review | 每种能力单独定义所有权和生命周期，完成真实场景对比后再考虑默认采用 |

P0 不是把所有 Provider 同时重写完才有价值。先建立公共事实结构和 DSH 路径，再以
相同准入问题扩展其他 adapters；不支持的组合继续明确显示不支持。Codex 与 Claude
优先扩展现有公开 machine API，Kimi Server 是新增可选路径，不强制替换 print 或使用 ACP。

Tower 试验必须对正式仓库具有清晰隔离，禁止直接接触 Flywheel 的状态/验收文件及
交付权限，只输出可审查候选；普通 checkout 隔离不自动解决预算、权限、后台清理。
若上游以后提供可验证的外部 merge gate，可另行评估更深的整合。

## 8. 验证与评估计划

### 8.1 必须覆盖的行为

| 场景 | 验收要求 |
| --- | --- |
| 版本/profile/模型不支持 | 首次可能派发前拒绝；无静默换付费渠道 |
| 原生默认启用协作 | 能识别实际配置；不能把缺少显式参数记录为关闭 |
| root 提前完成、child 挂起、深层后代、队列仍有 prompt | 不提前成功、不释放未知责任、不遗留可继续派发工作 |
| 外层并行 × 内层并行、Agent + Workflow 同时存在 | 预留与有效上限吻合，不靠 prompt 限数 |
| helper 越界写、Bash/PTC/MCP 绕过、用户自定义 Agent 覆盖 | 所称权限真实成立；发现不能强制的组合就不声明受限能力 |
| usage 重复、丢失、历史累计、占位零、父已包含 child | 去重且保留 unknown，不重复收费或虚报免费 |
| 断线、父进程被杀、原生模式恢复、远端任务仍运行 | 原调用可核对；未知状态不自动重新创建任务 |
| Team 完成但 oracle 失败、Reviewer 继承 executor 历史 | 不通过正式 Gate；独立评审与同一候选绑定仍成立 |
| 预热、取消、配置关闭时有既存原生责任 | 不在预热中派发；关闭能力不抹除已存在的清理责任 |

需要真实 SDK/CLI 协议夹具及故障注入，也需要用户 Mac 与订阅路径的端到端验收。
实现涉及 Go 时维持每个被测函数 100% 语句覆盖率，运行对应 Node/桥接/race 门禁。
这些是未来实现的验证要求，本次文档研究没有执行它们。

### 8.2 如何判断是否值得默认开启

选取小型 Quick、复杂故障调查、跨模块功能、前端截图任务、同构批量修改、含植入
缺陷的 review、长会话与中断恢复任务。固定仓库 HEAD、契约/oracle、模型与 effort、
账户、权限、输入、版本和总资源条件，比较明确单代理、工具并行、小规模 helpers、
原生 Workflow/Swarm。顺序交错，记录全部失败与重试，避免只展示成功案例。

记录最终验收率、首次通过率、漏报/误报、返工、总耗时 p50/p95、实际 token/cache
usage、可观察额度、峰值并发、限流及取消后残余工作。子代理不是越多越好；默认值
只有在质量、完成时间和资源开销的取舍被代表性任务验证后再调整。

## 9. 待讨论的决策

1. **推荐先落地 DSH PTC + 受限 helpers，并同步建立其他 adapter 的能力探测。**
   这样能利用现有结构，较快得到可测收益，再逐项扩展。
2. **推荐原生协作按阶段配置，初期 opt-in。** Quick/Intake 默认保持简洁；Plan、
   Investigation、Review 优先评估，Execute 先满足单 writer 边界。
3. **推荐 Tower 作为独立实验工作单元，暂不接管现有正式仓库。** 保留后续采用它
   完整能力的空间，但需要隔离候选和外部验收接口。
4. **推荐 JEV 做已准入能力的可选选择者。** 能力可用性、授权、预算与完成规则由代码
   判断；没有 JEV 也能按静态 profile 使用原生能力。

## 10. 来源与本次研究范围

以下优先使用官方文档、发布源码和本项目实际适配器。固定 commit 链接用于避免
上游 master 漂移；在线页面仅代表研究日的文档。产品能力介绍是试验方向，不代替
安装包、真实模型、性能、订阅权益或 Mac 桌面验证。

### Flywheel 当前代码

- [F1：模型默认与角色绑定](https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/templates/flywheel.config.example.json)。
- [F2：DSH overlay/通知处理](https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/bridge.js)、[root policy](https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/policy.js)、[root usage fold](https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/protocol.js)。
- [F3：Codex Pro bridge](https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/codex-pro-bridge.js)、[vendor 身份](https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/vendor/codex-pro.json)、[固定插件 machine](https://github.com/ckanner/dsh-subagent-codex-pro/blob/37da3d75de32b809c4fb1a631ccc15191f3d87f9/src/machine.ts)、[wire](https://github.com/ckanner/dsh-subagent-codex-pro/blob/37da3d75de32b809c4fb1a631ccc15191f3d87f9/src/wire.ts)。
- [F4：Claude Code Pro bridge](https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/claude-code-pro-bridge.js)、[可选 peer 的 source candidate 记录](https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/vendor/README.md)、[固定插件 machine](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/ec0b99176b67512f3ca1838ec2a349dcb0189214/lib/machine.js)与 [CLI](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/ec0b99176b67512f3ca1838ec2a349dcb0189214/lib/cli.js)。
- [F5：Kimi print driver](https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/internal/drivers/kimi.go)。
- [F6：插件依赖与兼容范围](https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/package.json)。

### DSH 官方 rc.3

- [D1：Agent Team](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/team/agent-team/README.md)。
- [D2：SubAgent 服务与生命周期](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/subagent/subagent/README.md)。
- [D3：SDK Team 配置](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/docs/user/guide/python-sdk.md#enable-team-mode)。
- [D4：PTC 工具执行](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/core/tool-executor-ptc/README.md)。
- [D5：Web 工具](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/web/tool-web/README.md)。
- [D6：Plan mode](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/plan/plan-mode/README.md)。
- [D7：Goal rounds](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/goal/goal-round-driver/README.md)。
- [D8：SDK protocol](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/sdk/protocol/README.md)。

### Kimi 官方资料

- [K1：维护中的 kimi-code](https://github.com/MoonshotAI/kimi-code)、[旧 kimi-cli](https://github.com/MoonshotAI/kimi-cli)。
- [K2：2.6.2 release](https://github.com/MoonshotAI/kimi-code/releases/tag/%40moonshot-ai/kimi-code%402.6.2)。
- [K3：Agent profile 与约束](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/customization/agents.md)。
- [K4：CLI 参数与 print](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/kimi-command.md)、[Swarm 开启变更](https://www.kimi.com/code/docs/en/kimi-code-cli/changelog.html)、[Hooks](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/customization/hooks.md)。
- [K5：Server API](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api.md)。
- [K6：Session/profile/prompt](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-sessions.md)、[任务事件与控制](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-conversation.md)。
- [K7：Tower 官方指南](https://www.kimi.com/code/docs/en/kimi-code-cli/guides/tower.html)、[Tower merge 实现](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/agent-core-v2/src/features/tower/tools/merge/mergeTool.ts)。
- [M1：Kimi Code 模型能力与档位限制](https://www.kimi.com/code/docs/en/kimi-code/models.html)。

### OpenAI 与 Claude 官方文档

- [O1：Codex Subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents)。
- [O2：Codex App Server](https://learn.chatgpt.com/docs/app-server)。
- [C1：Claude Subagents](https://code.claude.com/docs/en/sub-agents)、[SDK 的子代理限制与计量](https://code.claude.com/docs/en/agent-sdk/subagents)。
- [C2：Agent Teams](https://code.claude.com/docs/en/agent-teams)。
- [C3：Dynamic Workflows](https://code.claude.com/docs/en/workflows)。
- [C4：Ultrareview](https://code.claude.com/docs/en/ultrareview)。
- [C5：Headless](https://code.claude.com/docs/en/headless)。
- [C6：Advisor](https://code.claude.com/docs/en/advisor)。

[F1]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/templates/flywheel.config.example.json
[F2]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/bridge.js
[F3]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/codex-pro-bridge.js
[F4]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/src/claude-code-pro-bridge.js
[F5]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/internal/drivers/kimi.go
[F6]: https://github.com/ckanner/flywheel/blob/63d8781ee1ece0cce443fa2e1ead62d1d6288817/plugins/dsh/package.json
[D1]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/team/agent-team/README.md
[D2]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/subagent/subagent/README.md
[D3]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/docs/user/guide/python-sdk.md#enable-team-mode
[D4]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/core/tool-executor-ptc/README.md
[D5]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/web/tool-web/README.md
[D6]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/plan/plan-mode/README.md
[D7]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/goal/goal-round-driver/README.md
[D8]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/sdk/protocol/README.md
[K1]: https://github.com/MoonshotAI/kimi-code
[K2]: https://github.com/MoonshotAI/kimi-code/releases/tag/%40moonshot-ai/kimi-code%402.6.2
[K3]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/customization/agents.md
[K4]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/kimi-command.md
[K5]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api.md
[K6]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-sessions.md
[K7]: https://www.kimi.com/code/docs/en/kimi-code-cli/guides/tower.html
[M1]: https://www.kimi.com/code/docs/en/kimi-code/models.html
[O1]: https://learn.chatgpt.com/docs/agent-configuration/subagents
[O2]: https://learn.chatgpt.com/docs/app-server
[C1]: https://code.claude.com/docs/en/sub-agents
[C2]: https://code.claude.com/docs/en/agent-teams
[C3]: https://code.claude.com/docs/en/workflows
[C4]: https://code.claude.com/docs/en/ultrareview
[C5]: https://code.claude.com/docs/en/headless
[C6]: https://code.claude.com/docs/en/advisor
