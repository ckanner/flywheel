# 原生 Harness、PTC 与 LCU 在 Flywheel 各环节中的接入方案

[English](2026-11-13-native-harness-capabilities.en.md) · [方案索引](README.md) · [当前架构](../design.md)

**状态：完整修订的设计提案，尚未实现。** 创建于 2026-11-13，依据最新源码于
2026-11-14（America/New_York）修订。Flywheel 基线为
`ac1ed320c68f6a8a466005985264e65b5a72fad9`，包声明版本 `0.33.0`。
本文中的能力配置、回执、资源租约和阶段是拟议契约，不能直接写入当前配置文件。
本 PR 更新方案及导航；后续另行实现实际接入，本 PR 不修改用户配置和默认模型。

## 1. 结论与本次调整

**复用已经做好的 PTC presets 和 LCU，把它们接入 Flywheel 的受管调用。**
当前 `daily` 和 `heavy` 都使用 PTC，LCU 已通过 `cua` bridge 支持 `run_code`。
上一版“重活需要切回 native 工具”“生成器无法处理新 executor 行”的判断已被新代码
消除。这两项不再是待解决问题。[U2][L2]

仍然需要完成的是：正式 SDK worker 的组合与准入、模型及 GUI 资源调度、子任务归属、
停止与恢复、媒体证据以及 UI 中的真实运行状态。推荐先走现有 SDK 的全局 composition
加官方 PTC overlay，并复用生成器的能力策略；直接选择 named Agent Preset 留作第二条
公开扩展路线。LCU 则需要支持受管 SDK root 的正式挂载条件，沿用现有 `cua` bridge。

| 已完成的基础 | 对新方案的影响 | 还需要补齐的内容 |
| --- | --- | --- |
| 生成器 0.3.0：`daily`、`heavy` 都明确选择 PTC | 保留用户熟悉的 presets；无须为 CUA 改用 native | 证明 SDK worker 的实际 composition 与 executor |
| LCU 0.4.2：结构化 `cua` 调用、对象代理、图片投影 | 复用现成的程序化 Computer/Browser 接口 | SDK 挂载、真实授权、跨调用 GUI 所有权、停止回执与 epoch |
| Flywheel 0.33.0：完整 npm 单包 | 继续使用 `@ckanner/flywheel` 的 host、client 和 workers | 扩展已有内部协议与 schema；无需另发一个 Flywheel UI 包 |
| 各 harness 的工具、子代理、Team/Swarm/Workflow | 在合适的 loop 内利用原生运行能力 | 每种能力分别证明权限、限额、观测和生命周期 |

Go 继续拥有正式 Spec/packet、跨 packet 调度、预算、同一候选验证、独立 Review、恢复
和 Git 交付。原生 task board、子代理的“完成”、LCU 的截图或 Team 内部 review，均作为
证据进入这些环节。近期优先级是 **DSH PTC → 少量有界 helpers；LCU 独立完成准入后
试点真实 UI 检查**。DSH Team、Kimi Server Swarm、Claude Workflows 可以分别推进，
不用等所有后端拥有同一组功能。

### 1.1 版本与证据

| 对象 | 本次核对基线 | 可据此得出的结论 |
| --- | --- | --- |
| Flywheel | `ac1ed320…` / `0.33.0` | 最新 main 的单包分发和当前 bridges；不代表 npm registry 或用户安装已经更新 |
| dsh-preset-generator | `f692cb07abbb9786444c183d74eee018b1a2f0e8` / `0.3.0` | 两个 presets 的生成策略；物化内容仍取决于本机 `app.asar` |
| dsh-plugin-lcu | `9efeded13d20b41a96621bb53d903954f68550e6` / `0.4.2` | 当前 bridge、挂载、授权、媒体和生命周期代码；release 已存在 |
| DSH release | `dsh-v0.2.2-rc.3`，`38a6a5c965aa99ab525161efb7cc5cf1fe62fced` | 已发布基线；标准 SDK 与 PTC 旧输出协议 |
| DSH 最新 master | `39163dd30f703a79079e2d2446545cff3c453738` | PTC 已增加 `inspect` / `Image` 和私有 program tool 工厂；同名 rc.3 包并不保证同一协议，见 §2.4 |
| Kimi Code | release `2.6.2`；源码 `f082f04a093a3e71a2263c5461d52507dc31f83a` | 延续 11-13 已核对研究；Flywheel driver 的既有 print 契约说明仍以 2.4.1 为检查基线 |
| Codex Pro | 最新 main `aaf465901def5aa1870c1165b9714d32294337a4` / `0.9.0`；Flywheel vendor 来源 `37da3d75de32b809c4fb1a631ccc15191f3d87f9` | 两者生产实现相同，后续变化为发布说明与测试收尾；不替换已锁定 artifact |
| Claude Code Pro | 最新 main `5eb637a9e82b317e67e50846005c407c54f1cea0` / `0.4.0`；Flywheel 来源 `ec0b99176b67512f3ca1838ec2a349dcb0189214` | 两者生产实现相同，后续变化为发布说明与 CI；仍是可选后端 |

源代码、发布包、有效配置和实际运行证据分别记录。本轮没有调用真实模型，没有验收
用户 Mac 的 TCC、浏览器扩展或 GUI 性能。生成器部分测试依赖安装的 Mac 应用；LCU 的
Ubuntu CI 与 release 成功也不能替代真实 ChatGPT.app runtime 验收。本轮复核了 Kimi
源码身份、两个 Pro 插件最新 main，以及 Claude 的并发/恢复/交互式限制；原生二进制、
npm registry 与用户安装未作验收。最新 main 与 vendor 来源的比较见 [P1][P2]。

## 2. 实际架构：桌面入口、SDK worker 与插件组合

本文把四种职责分开：**模型**提供推理能力；**provider/account** 决定模型接入与账号；
**harness** 拥有工具循环、上下文及原生协作；**adapter** 把它们接入 Flywheel 的调用
和回执。因此，DSH 中使用 GPT 并不会自动获得 Codex 的 subagent，PTC 也不等于增加
模型 Agent 数。能力必须跟随实际 harness，模型/effort 另行选择。

### 2.1 我们在 DSH 桌面里使用，也确实使用 DSH SDK

日常对话、需求准备和 `flywheel-task` 的准备工作由桌面当前 Agent 完成，用户可以继续
使用 `heavy`。正式 loop 由 host 插件调用 Go，再启动对应 worker；DSH 路径使用本地
`dsh --profile sdk --patch <临时覆盖>` 的 stdio JSON-RPC。它运行在本机 DSH runtime，
不需要改为另一种模型账户。[F2][F7]

当前桌面入口传递项目目录及 host Session 关联，没有把当前 Agent 的 preset、工具集
或私人历史传给正式 loop。`FLYWHEEL_DSH_PROFILE` 可选兼容的自定义 SDK profile，
bridge 明确拒绝 `desktop`。runtime profile、Agent Preset、Flywheel 模型 `profiles`
是三种不同配置；设置 `FLYWHEEL_DSH_PROFILE=heavy` 不等于选择 Agent Preset。
独立 worker 进程仍可以按现有协议恢复角色 Session，共用 `DSH_HOME` 也可能复用账户
授权；这些都不自动继承桌面插件组合。[F2][D9][D11]

最新 master 仍明确从全局组合创建 SDK Agent，没有增加公开的 named-preset 选择字段；
这项边界并未随本次 PTC 更新自动消失。[D20]

| 责任层 | 所有权 | 向外提供的事实 |
| --- | --- | --- |
| DSH 桌面 host | 用户交互、准备材料、设置与状态展示 | 正式任务输入与 host 关联；准备消耗单独说明 |
| Flywheel Go | run/packet、候选、调度、权限、预算、验收、恢复 | 冻结一次 invocation 的范围、资源和完成条件 |
| worker / machine adapter | composition、原生请求、子任务归属、用量、清理 | 实际能力、原生身份、结果及停止回执 |
| 原生 harness | 模型工具循环、上下文、内部协作 | 真实执行事件、结构化结果和终态 |
| LCU / 本机 GUI | app/origin 授权、Computer/Browser 操作、媒体 | 目标和 runtime 身份、操作及图片交付事实 |

0.33.0 将 CLI、DSH host/client、三个 workers 与四个平台 Go 二进制放入一个完整 npm
包。`plugins/dsh` 仍是源码和构建测试目录，现为私有 harness；后续实现沿用根包的安装、
manifest/version/size/SHA 校验及桌面迁移机制，避免额外的运行时下载或平行迁移入口。
这次 main 更新没有为 Go driver 增加 preset/PTC/LCU 的正式能力字段。[F8]

### 2.2 两个 presets 已经统一为 PTC

生成器从已安装应用的 PTC 基底生成两个 presets。当前二者都设置 `presentation:'ptc'`，
`enable` 与 `append` 都为空；生成的插件列表相同，区别主要是 ID、名称、说明和顺序。
`setPresentation()` 同时识别旧 `tool-presentation` 和新 `tool-executor-native/ptc`，
并替换为具体 executor 行。旧的兼容障碍已经解决。[U2]

生成器只更新自己管理的 preset 插入项，保留其他桌面配置和默认选择；它不负责安装
runtime、配置账户或让所有 SDK worker 自动选中某个 preset。实际 ASAR 基底、输出
摘要、唯一有效 executor 仍需记录，不能仅看名称。也不应建立两份手工维护的完整
插件列表，分别给桌面与 SDK 使用。

LCU 的当前默认 preset allowlist **同时包含 `daily` 与 `heavy`**。因此两个名称不能
充当“无 GUI / 有 GUI”的权限边界。“重活”的说明表达使用定位；实际 CUA 可用性取决于
安装、挂载、有效配置、授权和目标状态。[L1][L8]

### 2.3 PTC、模型协作与 GUI 是三类资源

PTC 使用 `run_code` 编排工具；当前作用域选择最近的 executor。同一个程序里，明确
通过 `isConcurrencySafe(args)` 明确返回 `true` 的工具调用才允许并行；省略、抛错或
其他返回值均为排他调用。默认
`maxParallelSubCalls=10` 是工具调用上限，不是模型 Agent 上限。[D4][D14]

LCU 的 `cua` 未声明并行安全，已有 PTC 调度会约束单个程序内的调用；其他 Agent、
其他 worker 或另一条 Flywheel run 仍可同时操作同一桌面。全局 GUI 资源必须由独立
所有权契约协调。普通文件/HTTP I/O、额外模型推理和物理桌面输入分别计量、分别调度。

### 2.4 最新 PTC 协议变化必须纳入兼容矩阵

rc.3 tag 与最新 master 的 `tool-executor-ptc/package.json` 都写 `0.2.2-rc.3`，但
master 新增 `/run-code` export，并改变了输出与图片契约。因此准入不能只按版本号
判断，应记录实际 bundle/模块摘要、exports、工具 schema 和无副作用行为探测。[D16][D17]

| 组合 | 已核对事实 | 接入条件 |
| --- | --- | --- |
| rc.3 发布基线 | 旧 PTC 程序输出与图片 deferred context | 使用匹配的程序模板及工具结果规则 |
| 最新 master | TypeScript；选择性输出使用 `inspect(...)`；函数 return/最终表达式不成为结果；`read_image`/`cua` 图片适配为 `Image`，由 inspect 呈现 | 验证文本、图片、错误和取消的实际输出，不能沿用旧输出假设 |
| master + LCU 0.4.2 | 存在下述源码组合风险，未执行真实组合验证 | 先修订图片/结构化值契约并通过夹具，再准入 GUI |

具体风险是：master 的 transport 对名为 `cua` 的结果作图片特殊处理，恰好一张图片时
返回 `PtcImage`，替代原结构化 value；多张图片会报错。LCU 的初次 app/tab 调用可以
同时有 `__handle` 和图片，其 proxy 又依赖该 handle。两者直接组合可能丢失句柄；
proxy 对普通对象的复制也需要验证是否保留 Image 身份。**这是源码推断，尚非真实
Mac 复现结论**；已有 PTC 支持不能据此宣称最新组合已兼容。[D18][L2][L3]

新增 `createRunCodeTool` 可把私有工具集合封装成有界 program tool，值得用作后续局部
能力入口；它复用同一 transport，不能绕开上述问题，也不代替模型预算或 GUI 租约。
首期仍优先已核对源码的全局 PTC overlay；工厂接口成为独立可选能力，避免强制升级全部
用户。兼容检查至少覆盖结构化值/handle 与图片共存、零/单/多图、输出策略移除图片、
`inspect` 的成功/失败输出，以及取消后已发生的工具副作用。[D19]

## 3. 各后端现有能力与接入价值

### 3.1 DSH：PTC、helpers 与 Team 分开接入

| 能力 | 已核对事实 | 推荐接入 |
| --- | --- | --- |
| PTC | 标准 SDK 默认 native；官方 overlay 关闭 native、开启 PTC，并调整文件工具配置 | 复用 preset 策略生成受管 SDK composition，核验实际 executor [D9][D10] |
| 本地 SubAgent | fresh/fork、消息、继续执行；每树默认最多 8 个活跃 children；delegation tool 继承的 Host 默认深度为 1，可覆写 | 先接少量探索助手，逐 child 强制权限、cap 和归属 [D2] |
| Agent Team | 在 subagent tree 上增加持久化任务板、依赖与 CAS 认领 | 作为 invocation 内部协作，正式 packet DAG 留在 Go [D1] |
| Web search/fetch | 可分别配置；普通 fetch 不必使用辅助模型 | 按实际工具效果开放检索和模型调用 [D5] |
| Plan、Goal、压缩 | 各有不同交互和生命周期 | 作为后续独立能力；Plan 提示不构成权限隔离，Goal 轮数不构成累计费用预算 [D6][D7] |

Team 有正式 headless 组合：标准 `sdk` 的 patch 可以配置
`agent-team.config.defaultEnabled=true`，`sdk-minimal` 不包含 Team。默认值仅作用于
新 root，公开 RPC 没有可直接假设的 `team/setEnabled`。关闭 Team 也不取消 children
或清空任务板。`session/wait` 等待 root 与正在推进的后代，parked child 仍需
drain/shutdown。[D1][D3][D8]

Team 的 `writeScopes` 是冲突提示；成员共用 checkout，原生没有因此获得文件锁或
独立 worktree。外部 SDK/ACP/Codex/Claude children 是一次执行，不具备本地 teammate
同样的继续语义。[D1][D2]

当前 Flywheel 的临时策略禁用委派、workflow/goal、schedule、web 及部分辅助模型功能；
`agent/created` 只允许 root，bridge 拒绝子派发，usage fold 只消费 root。因此实现
helpers 必须同时扩展策略、事件、身份树和清理，单独删除禁用列表不够。[F2][F9]

### 3.2 Kimi：普通 Agent、Swarm、Tower 各有入口

| 能力 | 当前公开入口 | 建议 |
| --- | --- | --- |
| 普通 `Agent`，含 coder/explore/plan | print 可以使用；新 Session 可通过 `--agent-file` 配置 | 作为小规模协作候选，补有效配置、子账本与 cap |
| `AgentSwarm` | 显式 swarm mode；Server profile/prompt API 有字段 | 新增可选 Server adapter，不能给现有 print 虚构开启参数 |
| Tower | 实验开关 + Server profile；先创建 Session 再启用 | 后期独立仓库工作单元，交回候选 patch/commit |
| 原生 Plan / Hooks | `--plan` 与 `-p` 冲突；hooks 错误/超时 fail-open | 保留 Flywheel Plan 契约，hooks 不作唯一门禁 |

当前维护项目是 TypeScript 的 `MoonshotAI/kimi-code`，旧 Python `kimi-cli` 已归档。
Kimi print 不等于已禁用普通 Agent；准确状态是尚未受管和证明。[K1]–[K4]
`--agent-file` 仅用于创建，不能与 `--session`/`--continue` 同用；child resume 还有
委派白名单豁免。Flywheel 首调原生 Session ID 为空，预分配的是自己的 Request ID，
因此创建时接 profile 不与首调身份冲突；恢复仍需绑定并核验配置摘要。[F5][K3]

Server API 仍为 Beta，需对实际安装版本的 OpenAPI/AsyncAPI 核对。adapter 应拥有
经过认证的本地 runtime/Session，停止只作用于本次工作。profile 更新不是原子事务；
prompt 幂等记录仅存内存、每 Session 最多 256 条且重启清空；abort 不清空排队 prompt；
Session usage 的占位零不能代表免费或实际零用量。这些都需要外层持久化意图、逐 task
停止及用量核对。[K5][K6]

停止时先撤销排队 prompt，再 abort main，并通过 WebSocket `task.stop` 停止可见的
child/Swarm task，等待各自终态。当前 side-chat Agent turn 没有同等公开 stop 接口，
受管 profile 应禁用它，或先实现并验证对整个隔离 runtime 的所有权与关闭；不能将
`:abort` 宣称为“全部子任务已停止”。[K8]

Tower 已承担 mission、worker/reviewer、分支、worktree、review 和 merge 职责。本次
没有查到可直接关闭全部编排只借 workers 的公开模式。禁用某个 merge 工具仍不足以
限制 Bash/Git；普通 worktree 还共享 Git 元数据。未来试点应在独立仓库副本中运行，
限制正式状态与交付凭据的访问，由 Flywheel 在外部验证候选。[K7]

### 3.3 Codex：扩展已有 machine 的 controls 与 events

Codex 当前文档中的 `agents.enabled` 默认 true，并发项为
`agents.max_concurrent_threads_per_session`，`agents.max_threads` 是兼容别名。
child 继承父 sandbox/approval，自定义角色文件不能另设它们；explorer 名称不证明
只读。app-server 有 thread start/resume/fork、turn interrupt、native review 和协作
事件，应核对实际 CLI schema。[O1][O2]

当前 Codex Pro machine 0.9.0 以 root thread 为中心，过滤其他 thread 的部分通知与
用量，初始化 `experimentalApi:false`；Flywheel 尚未接入完整 child 事实。machine
已有的图片与结构化结果能力也未成为 Flywheel 的通用输入契约。[F3][F12]
优先扩展用户已有插件，保留订阅登录，再提供受管只读探索、`review/start` 和媒体/结果
透传。正式 Reviewer 保持独立上下文；从 Executor 私人历史 fork 出来的 child 不能
直接替代它。没有传 agents 参数也不能记录为已关闭所有协作。

### 3.4 Claude：小型子代理、Workflows 与远端任务分别处理

| 能力 | 用途 | 必须独立处理的边界 |
| --- | --- | --- |
| Subagents | 专项调查、假设检查、小范围审查 | tools/agents、实际模型、子事件和父权限继承 |
| Dynamic Workflows | 同构批量分析、迁移、交叉检查；支持 `claude -p` | 受管脚本、并发及累计任务上限、所有派发机制 |
| Agent Teams | 多 Session 共享任务/消息 | 实验状态、teammates 恢复与关闭 |
| Ultrareview | 云端多代理代码审查 | 独立 job ID、查询/取消、账户条件与发帖授权 |
| Advisor | 满足模型/账户条件时的第二意见 | 单独的模型与费用授权，避免和 JEV 重叠 |

Workflows 默认最多 16 个并发、累计 1,000 个 agents；CLI `2.1.269` 起支持
`CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS`。普通 Agent 的 cap 与 Workflow cap
分开。`ultracode` 会绕过普通 Agent 的会话子代理并发限制；需要严格上限的 profile
应排除该模式；`/subtask` fork 和已完成 child 的 resume 也可占槽而绕过普通 cap，
须限制这些入口或证明其额外上界。组织 `availableModels` 还可能把受阻的请求模型替换成其他模型，必须
在派发前排除未授权替换，回执再核验实际绑定。[C1][C3]

当前 Flywheel Claude bridge 只接受 `danger-full-access`；这是本项目 bridge 的约束，
不能泛化为原生插件只支持该权限。原生 user/project/local settings 会加载，但当前
machine 协议未受管 agents、Team/Workflow 与子树。父 bypassPermissions 不能靠
child 的 `permissionMode:plan` 收窄，工具白名单也不等于 OS 只读隔离。[F4][C1]

Workflow 的恢复会按启动顺序匹配旧调用；失败或提示变更后，后续已完成 Agent 也可能
重新运行。因此新增 child attempt/generation、脚本与输入摘要、候选和副作用记录；
初期优先只读分析，写入任务不能直接依赖原生 resume 自动重放。Teams 的 teammates
要求 interactive session，当前 `claude -p` machine 通道不支持它；后期必须单独实现
可观测的交互式 adapter，设置一个环境变量不足以完成接入。[C2][C3]

`claude -p '/code-review ultra'` 不等待完整审查，应使用专用 `claude ultrareview`
接口并处理远端责任；本地进程结束不能代表云端取消，重启调用可能再创建一份审查。
默认不发 PR 评论；订阅登录路径保持现有方式，不用要求 API key 的启动选项替换它。
Claude 继续是显式选用的可选后端。[C2]–[C6]

## 4. 统一能力意图与运行事实

推荐统一“需要什么”和“实际发生什么”，让各 adapter 保留真实差异。以下是拟议
profile/receipt 的内容，不是已经支持的配置 schema。

| 内容 | 必须表达的事实 |
| --- | --- |
| 身份与来源 | harness/adapter/CLI/schema 版本；模型、provider、账户渠道；cwd 与 candidate |
| 组合 | runtime profile、preset ID/修订、生成输入与输出摘要、有效 executor、模块 exports、输出/图片协议、插件版本及实际工具注册 |
| 使用意图 | PTC、普通检索、结构化结果、受限 helpers、内部 task board、GUI、隔离 mission 或远端 review |
| 权限与资源 | root/child 实际权限、允许模型/工具/目标、模型槽位与工具并发、GUI lease、剩余时间 |
| 观测 | requested / effective / observed、支持或拒绝原因、证据来源与观测时间 |
| 生命周期 | root 和可见 child/task 身份、可能派发、取消、终态、清理与恢复能力 |
| 用量 | 可信 aggregate 或去重后的 children、未知项、在途用量、硬预算与软停止的实际能力 |
| 媒体 | 截图成功、图片交付实际模型、证据进入 run 三个独立事实 |

能力显示“可用”之前要核对实际生效的配置与工具注册，不能只看包已安装或 schema 中
出现 `cua`。静态 probe 只能证明静态事实；实际目标授权、runtime 连接、模型视觉路由
及 GUI lease 属于调用准入，并有自己的有效期。

拟议选择语义：

- `off`：核验该能力的原生入口实际关闭；缺少显式开启参数不是关闭证明。
- `prefer`：仅在确认没有任务相关模型/工具派发、清理已完成，且普通路径满足相同
  约束时回退。已完成的无推理 probe 单独记录；未知探测责任仍阻断回退。
- `required`：必要能力缺失就在首次可能派发前拒绝，说明原因，不隐式更换权限、
  模型、账号或付费渠道。

任何可能已派发、旧 child 活跃、工具副作用未知或清理未知的情况，都保留原 invocation
用于核对。Go、SDK、adapter 不能各自重试一次制造重复任务。启动 hook 和恢复 Session
也可能发生活动，启动异常不能自动证明“尚未派发”。

## 5. DSH composition 的推荐落地路线

### 5.1 路线 A：受管 SDK 全局组合，优先实施

沿用当前标准 SDK 入口，由 Flywheel 生成版本化的临时 overlay。复用官方 PTC overlay
和生成器的能力策略，核验 native/PTC executor 的最终选择及工具 schema/执行一致性。
共享的生成逻辑应是纯粹的策略/组合转换，输入包括固定的 DSH 基底与插件版本，输出
可审查且有摘要；避免复制两份完整 preset 列表。[U2][D10][D14]

此路线不依赖 named preset，适合尽快启用正式 loop 的 PTC。当前 LCU 却按 Agent 的
preset 判断是否挂载：先查 registry 的 composed preset，再查 session header；两者
都没有就跳过。SDK server 目前从全局组合创建 Agent，不挂载 preset roster。[L1][D11]

因此，**LCU 的受管 SDK 挂载需要新增公开入口**：由可信 host/adapter 传入已经核验的
root、invocation、能力配置和 GUI 所有权，LCU 验证后注册 `cua` 并返回实际挂载回执。
继续保留桌面 preset allowlist 的现有行为；不伪造 `header.agentPreset`，不借测试
override 绕过 host guard。这个入口还应明确 revoke/dispose 与清理结果，不能只加一个
全局“允许所有 Agent”布尔值。

### 5.2 路线 B：公开的 Agent Preset 创建接缝，后续可选

rc.3 已有公开 registry API：`agentPresets.mount(ctx, id)` 在尚未发布 Agent 的 creation
`setup` 中绑定当前 preset 修订；`composeFrom(ctx, parent)` 使 child 继承父的确切
修订。但标准 SDK server 没有暴露该 preset 创建接缝，也不存在可直接填的
`preset=heavy` RPC 参数。[D11][D13]

如果需要正式 loop 直接选择 daily/heavy，可通过受维护的 SDK 创建扩展或上游公开
协议实现，再验证 preset mount、LCU 初始化顺序、root/child 策略和会话恢复。避免
私自 patch SDK 内部实现。存活 Agent 绑定的修订可以稳定保持；重启或恢复时按相同
preset ID 选择的是当前定义，不能声称代码被永久冻结。恢复必须比较实际组合摘要。

两条路线共享能力意图、证据与测试。先用 A 打通受管 PTC，再根据 named preset 的
维护收益决定是否做 B；不把完成 B 作为全部原生能力的前提。

## 6. LCU：复用现有桥接，补齐 GUI 契约

### 6.1 已有接口与实际依赖

LCU 0.4.2 已注册结构化 `cua` 工具，并提供
`connectCua(request => tools.cua(request), …)` 形式的代理，使 `run_code` 中的
`cua.getApp/getTab` 和 handle 方法转成 `{target?, method, args}` 调用。内部仍调用
ChatGPT CUA MCP 的持久 REPL `js`：先执行并保存结果，再用短调用读取结果标记；图片
走单独的内容投影。现成 bridge 已完成程序化接入，无需重新让模型拼嵌套 JS。[L2][L3]

实际 runtime 依赖 Apple Silicon macOS、本机 ChatGPT.app、其随附 Node/CUA runtime
和 Sky helper；浏览器路径还依赖官方扩展与 desktop native host。LCU 本身不要求
Codex/ChatGPT 模型登录，也不需要 Python/amontlabs LCU 包；模型账户和 GUI runtime
是不同依赖。Screen Recording/Accessibility 等 TCC 权限需要核验实际负责进程，
SDK 启动方式必须在用户 Mac 上单独验收。[L7]

### 6.2 授权与挂载：保留真实的人和目标边界

当前 LCU 先检查 `userQuestions` 服务；缺失时即使配置了目标 allowlist 也会取消。
服务存在时，native app 先通过 host guard，再按精确 bundle ID 预授权；浏览器按精确
origin 预授权；其他目标需要真实用户回答。[L1]

标准 rc.3 SDK 的 base 已挂载 `userQuestions` 服务。当前 Flywheel initialize 没有
开启 `capabilities.userQuestions`，也没有回答请求处理，所以缺的是正式 host 问答转发。
源码允许已精确预授权的目标在服务存在时自动接受；需要新增人类回答的目标，还须
接通 SDK capability 与真实回答通道。服务存在、已预授权、可以互动回答是三个不同
事实，均需在实际组合中核验。[D11][D15][F2]

方案应保留用户已有的准确授权，避免每轮重复确认；同时提供可用的 host approval
转发，或经明确设计验证的预授权通道。模型不能代答，未知/错误/cancel 不能转为
allow。`full-access`、`approval=never`、文件沙箱和 worktree 都不授予 GUI app/origin
权限。host guard 对宿主/Agent host/终端的保护继续有效。

问答关联 run、invocation、root、lease/runtime epoch、精确 app/origin 和原生 question
ID。取消、换 owner、换 runtime 或目标变化后的迟到回答不能批准新动作。有效既有
授权按其原范围复用；授权事实与操作实际执行分别记录。

当前 LCU 按 Agent/preset 挂载，并未因 child 身份硬性禁用全部工具。首期方案主动
只给受管 root CUA 能力，helpers 分析代码或冻结的媒体产物。DSH 子代理的人类
交互本身受 root 存活等条件限制；先不扩散 GUI 审批与操作权。若提供“观察 / 交互”
配置，必须在方法和目标层验证读取/动作限制；当前通用 bridge 没有完整的只读策略，
不能以提示词“只观察”宣传只读隔离。[L1][L2]

当前挂载失败会记录警告并让 Agent 继续运行，preset gate 主要在挂载时判断；切换到
不允许的 preset 不足以证明旧连接即时撤销。对于 `required` GUI，adapter 必须在
派发前拿到真实工具/连接/授权能力回执；对于 revoke，要等待动作停止与清理回执。

### 6.3 一台桌面只有一个受管操作者

新增由 Go 管理的 GUI resource lease，首期粒度为实际 host/display/desktop session，
覆盖整个 GUI invocation。每个 Agent 独立连接、独立 REPL，仍然面对共享的焦点、
鼠标、键盘和浏览器状态。当前 packet `schedule` 仅有 `after`/`writes`，现有 model
slots 按 channel 计数；它们都没有表达桌面资源。[F10][F13][L2]

| 设计点 | 推荐规则 |
| --- | --- |
| 覆盖范围 | 所有 serial/parallel、Quick、手动 stage 路径共用准入；跨 run/进程共享 owner/broker |
| owner | 持久化 run、role/packet、invocation、runtime epoch、目标与资源身份 |
| 取得顺序 | 先取得 GUI 再消费模型槽，或原子取得整组资源；若没有模型容量且尚未派发，释放临时 GUI 预约并重新排队 |
| 释放条件 | 停止新动作、确认 turn cleanup 与实际 runtime 收束后释放；PID/超时/TTL 不能单独证明清理 |
| 首期范围 | GUI root 独占；代码/文件任务可继续并行；冻结截图的离线分析可并行 |
| 可见状态 | 等待哪个资源、当前 owner、取消中、清理未知、可恢复的原因 |

租约只能协调遵守它的参与者，无法阻止人或其他未受管程序操作桌面。动作前后要检查
目标、页面/窗口和状态变化；遇到外部干扰暂停并重新观察。未来按 app/window 分锁
需要先证明没有焦点、输入或 native runtime 的交叉影响。初期整调用独占较保守，
但规则清晰且可验证；后续再用真实等待时间决定是否细化。

### 6.4 epoch 与对象句柄：应用升级后重新取得目标

CUA 代理对象存于每个 native REPL 的内存 map，句柄形如 `h1`，没有跨 runtime 的
持久对象恢复；`tab.id/title/browserId` 等属性也是取得对象时的快照。[L2][L3]
当前 `ManagedSession` 检测 ChatGPT.app generation 变化后会先创建新连接、迁移
session ID，再关闭旧连接；对象 map 不会随之转移。旧 `h1` 可能失效，也可能在新
runtime 中碰撞到另一个对象。[L4]

受管路径必须给 handle/proxy 加上 worker/session/runtime epoch 归属，跨代拒绝旧
句柄并清空代理缓存。需要正式 generation 事件或受管模式下关闭透明替换；新 runtime
取得 GUI 操作权前确认旧 runtime 已停止。重新取得 app/tab、捕获新状态后才继续，
不透明重试旧指令。仅名称相同、Session ID 相同或进程重启成功，都不能恢复旧对象身份。

### 6.5 取消与清理：区分请求、确认和物理停止

LCU 已使用真实 session ID、每 turn UUID 和 `turn_ended` Stop/Interrupt；wrapper
经 ChatGPT 的 signed client IPC 执行清理。一类清理超时会设置 pending gate，在下一次
CUA 调用前重试，这是可复用的基础。[L1][L5]

但当前 request abort 只拒绝本地 pending promise，没有向远端发送取消；部分 cleanup
失败只记警告；helper 重试有限；dispose 的 close 未提供等待完成的外层回执。
`close()` 的 stdin→TERM→KILL 路径最后一次退出等待也没有检查布尔结果。因此，
“Promise 被取消”“root 返回”“调用了 close”不能分别充当 GUI 已停止的证明。[L1][L5][L11]

建议对每次调用记录：取消请求已接受、新动作已封锁、排队输入已处理、turn cleanup
已确认、受管 runtime/后代已退出或可证明静止、lease 已释放。失败或未知状态继续
占有清理责任，并阻断下一位 GUI owner；提供核对/恢复入口，避免静默重试。
`cua.stopApp` 是当前 turn 内某个 app 的停止手段，不等于全局 cleanup 或撤销用户授权，
也不需要在每次普通成功后随意调用它。

### 6.6 截图：内部视觉与正式证据分别核对

LCU 当前会校验图片 MIME/base64，并按 Agent 的实际 provider/model route 调用
`resolveModelInfo` 验证 image input；附件存储成功后返回 DSH image content，可用时
还附 host path。无法解析路由、模型无图像输入、无附件 store 或保存失败时，图片会
变成诊断文本。[L6]

所以 Flywheel 外层 prompt 只有文本，并不意味着内部 DSH Agent 看不到 LCU 截图。
需要分别记录 **captureSucceeded、imageDeliveredToModel、evidenceAttachedToRun**。
以上名称只是拟议事实字段。不能把“截图成功”的文字当图片交付，也不能把内部模型
看过图片当成独立 Reviewer 已经取得证据。

还须按 §2.4 区分旧 deferred context 与新 Image/inspect 通路，确认图像确实进入模型
输入；LCU 生成 image block 不足以证明后续 PTC transport 与 proxy 没有丢失它。

拟议媒体 manifest 绑定候选 commit/build、target/tab、viewport、runtime epoch、
观测时间、实际模型路由、图像 hash 和 artifact reference。外部 host path 必须在允许
范围内读取、复制并核验 hash，不能盲信模型返回的路径。独立 Review 可读取同一候选
的冻结图片；需要再操作 UI 时，取得新的 GUI lease 并重新确认目标与状态。

LCU 当前接在 DSH。Kimi 前端模型可以评估单独的结构化图片输入，但不能因此自动把
Kimi loop 改经 DSH，或偷偷替换为有视觉能力的另一模型。GUI 与媒体 profile 必须验证
实际选定路由满足需求，能力缺失时按 §4 的选择语义处理。

## 7. 各 loop 如何使用

以下是推荐试点，模型默认保持当前[模型策略](../zh-CN/model-policy.md)：DeepSeek Flash、
Kimi K3.1 Pro、既有 ChatGPT/Codex 订阅路径；Claude 显式可选，不加入智谱或付费后备。
能力名称不构成模型排名；同一模型下先比较执行方式，再单独比较模型/effort。

### 7.1 保留当前绑定，在其实际 harness 内选能力

| 当前阶段 / 路由 | 新安装模板的实际绑定 | 首期能力选择方向 |
| --- | --- | --- |
| Plan | Kimi CLI / K3.1 Pro / high | 受管普通 Agent；需要 Swarm 才显式选 Server 路径 |
| Execute default | DSH / DeepSeek Flash / high / deepseek-account | 受管 PTC，随后少量受限 helpers |
| Execute frontend | Kimi CLI / K3.1 Pro / high | 原生工具与媒体读取；LCU 需单独配置有资格的 DSH 调用 |
| Execute hard | Codex Pro machine / GPT-6.1 Sol / high | 扩展当前插件的 controls/events 后使用原生助手 |
| Review | DSH / GPT-6.1 Sol / high / chatgpt | 独立 DSH PTC 评审，可读取冻结媒体 |
| Judge | DSH / GPT-6 Astra / high / chatgpt | 以既有证据为主，按需补独立分析 |
| Intake | DSH / DeepSeek Flash / low / deepseek-account | 单代理、低固定开销与结构化结果 |
| Garden | Kimi CLI / K3.1 Pro / high | 原生检索，后续按任务选择有界协作 |

上表来自当前模板，不覆盖用户配置；Claude 仍为显式可选。桌面 Codex/Claude 插件注册
的工具可以由 heavy 的 PTC executor 调用，正式 machine API 则不经过这层桌面工具；
目前两个 Pro 仓库的新提交也没有新增受管 child controls。不能因工具能从 `run_code`
调用，就推断 Flywheel 已取得子代理计量与恢复能力。[F1][P1][P2]

### 7.2 各环节的工作与验收

| 环节 | 推荐原生能力 | LCU / 媒体用途 | 最终责任 |
| --- | --- | --- | --- |
| 宿主准备 | 用户重活 preset 的 PTC、现有工具 | 观察现状、收集参考与明确授权 | 形成可校验输入；准备消耗与正式引擎分开 |
| Intake | 工具检索、结构化输出，通常单代理 | 读取已有截图附件 | 原 Intake 契约和预算 |
| Full Plan | helpers 分查接口、测试、兼容性，主 planner 汇总 | 必要时受管观察现有 UI，再冻结证据 | 单一正式 planner 写 Spec/packet |
| Investigation | helpers 分别检查假设和证据 | GUI root 复现界面路径，记录前后状态与操作 | Go 管理诊断、修复交接与真实执行回执 |
| Execute | PTC；主执行者写、助手研究；有结构时使用 Team/Workflow | 受管 DSH 路线对当前 build 作交互检查 | 原 packet、保护路径、验证和交付 |
| Review | 独立上下文，分角度审查同一候选 | 分析冻结媒体；复测时重新取得桌面 | 独立 Reviewer 汇总原 verdict |
| Judge | 按需要聚合额外证据 | 比对已冻结验收和对应候选证据 | 原 Judge/oracle；一致意见不能覆盖失败 |
| Garden | 跨任务模式检索与归纳 | 一般使用既有产物 | 原整理范围，不重写冻结状态 |
| Quick | 低固定开销，明确可并行时才加少量 helpers | 有 UI 验收要求才单独取得 GUI | 保留最终独立 Review 与候选验证 |

单 writer 必须有执行层约束。DSH 可研究逐 child 注入更窄策略；对于无法收窄 child
权限的后端，可将整次调用设为真正只读，或由 Flywheel 启动独立受限 worker。
full-access child 的“不要修改”提示不算只读证明。跨 packet 并行继续由 Go 调度，
不会把全部写入者塞进一个共享 checkout 的原生 Team。

前端实例：Execute 产出候选 A，受管 GUI root 在对应 build 打开页面、操作并保存证据；
它停止且释放 lease 后，独立 Reviewer 查看 A 与冻结图片。若需要复测，Reviewer
作为新 owner 重新观察并操作。发现问题则返回原修复流程，新的候选 B 使用新证据。
这使 LCU 成为可验收能力，而不是独立于候选版本的一组截图。

## 8. 并发、计量、会话与恢复

### 8.1 模型并发采用可证明的控制粒度

首期采用整组预留：root 加最大允许 children 一起取得额度，再由原生 runtime 强制
真实 cap。例如 root + 最多 2 个 children 占 3 槽；全局 6 槽最多接纳两组，数字仅为
试点示例。这样不要求每次 spawn 都存在一个原生尚未提供的 Go RPC，也避免多个 root
各自占槽后等待 child 额度。

多种派发机制可并存时，预留合计上界或显式互斥；普通 Agent、Workflow、Team、
`ultracode` 等绕过路径必须纳入探测。没有可验证 cap 的模式不能承诺严格全局并发。
停止中或清理未知的组继续承担责任。工具内部若调用模型，也进入相应模型预算，
不能借 PTC 工具并发绕过。GUI 的资源取得顺序按 §6.3 统一执行。

现有 scheduler 同时约束全局 slots 与 channel。P1b 首期限定 helpers 在同一个已批准
channel，仍核验实际模型；若开放跨渠道，启动前原子预留全局上界及每个 channel/account
的上界向量。同一真实账号的配置别名不建立独立额度池；无法证明 child 路由或上界时
拒绝该多渠道 profile，不能将所有消耗归到 root channel。[F10]

### 8.2 用量与预算

并发限制不等于累计 token/金额硬限额。事后 usage、阈值告警和异步取消提供的是
对应的观测与软控制，可能仍有在途消耗。严格累计预算只准入确实支持且验证过的路径。
缺失 usage 保留 unknown，不能将占位零、context size 或订阅解释为零费用/无限额度。

按原生请求身份去重，采用“已包含 child 的可信父汇总”或“去重后的逐 child 汇总”
之一，不能两者相加。历史累计、当前调用、cache、模型 token、API 等价估价和真实
订阅账单分别记录。源码证明的是机制，费用和速度收益需要同条件实验。

### 8.3 归属、恢复与预热

扩展现有 invocation journal：必须有引擎 root invocation、run、role/packet、Request
ID、候选、配置摘要与结果/清理状态。原生 root ID 在协议确认后绑定；只有真实存在的
child/task/job ID 才记录，不给 aggregate-only 后端捏造树。无法满足所需权限或恢复
粒度的 profile 不准入。

取消顺序为停止新派发、处理队列、取消已知工作、等待终态和清理、保存回执。
本地进程组收束不能代替远端 job 终止证据，GUI 还须满足物理停止契约。网络断开或
parent 被杀时先核对原调用，不自动重新创建 Swarm、Workflow 或云端 review。

当前 Session/预热 binding 覆盖模型、权限及运行环境引用，但 profile 路径/名称不能
捕获文件内容改变。应加入实际 preset/SDK overlay、executor、工具策略、LCU/target
policy 的摘要，以及会影响授权的 runtime 身份。[F11][F14][F15] 同名 preset 已变化、旧 board、
旧 mode 或恢复 children 不自动获得新调用权限；必须重新核验。

预热只做允许的无推理运行时准备，不创建正式 Session、child、模型调用或 GUI 操作，
不提前取得长期 GUI lease。composition probe 与正式准入分开，预热项须按扩展后的
binding 复用或失效。当前新安装模板的 scoped/parallel/batch/prewarm=false/full-access
与旧配置缺省归一化不同；实施基于实际有效配置，不借本方案重置用户设置。[F1][F14][F15]

## 9. UI 与 JEV 的设计

设置按用户要完成的行为组织，避免为每个品牌复制一组看似相同的开关。保留现有唯一
JEV 总开关；静态角色 profile 在 JEV 关闭时照常工作。JEV 只从 Go 已核验能力、权限、
资源和预算的候选里提出选择，开启 JEV 不自动授权 GUI、Team 或新模型。

| UI 区域 | 用户需要决定或看见的内容 |
| --- | --- |
| 原生执行 | 是否按阶段使用；每个 loop 引用的能力方案；默认保留现有绑定 |
| 工具与协作 | PTC/结构化结果、helpers、Team/Swarm/Workflow 的实际可用性和限制 |
| Computer / Browser | 单独启用与目标授权；请求/有效/实际 CUA 状态；观察模式仅在强制策略存在后提供 |
| 资源 | 模型额度、GUI 等待原因与当前 owner；不把 PTC 并发显示为模型数 |
| 运行记录 | 实际 executor、preset/配置修订、模型路由、能力拒绝原因、图片交付与证据状态 |
| 停止与恢复 | 正在停止、清理已确认或仍未知；下一步核对动作 |

基础页只呈现高频决定，高级上限和版本证据放入详情。不可用项展示具体原因，避免
看起来可以开启但从未生效的“影子开关”。设置变更对下一次合法准入生效；正在运行的
调用需要完成关闭和清理，不通过隐藏开关抹除责任。UI 显示 Go 的事实，不在浏览器端
再实现一套调度器。

现有 `scheduler.dispatch` 管正式 packet 调度，`scheduler.prewarm` 管受限初始化准备，
两者继续各司其职。L1/L2/L3、保护路径、独立 Review、同一候选与 Git 交付规则保持由
引擎执行。UI 的新增配置需与 schema、CLI、diagnostic、docs 一起交付。

## 10. 实施阶段与仓库分工

| 阶段 | 交付 | 完成条件 |
| --- | --- | --- |
| P0：事实和准入契约 | 有效能力 probe、组合摘要、requested/effective/observed、明确 off、资源与回执设计 | 同版本不同 PTC 行为可区分；恢复/预热 binding、媒体和 GUI 资源字段有版本化契约 |
| P1a：受管 PTC 与结果 | 路线 A、共享生成策略、DSH PTC/普通检索、结构化结果适配 | executor/工具一致；错误与结果不绕过原 schema；无需 GUI 即可验收 |
| P1b：少量 helpers | 先 DSH 本地受限 helpers，再扩展已有 Kimi/Codex/Claude 入口 | 实际 cap、权限、父子计量、停止和恢复均可证明 |
| P2a：受管 LCU | 正式 SDK 挂载、root CUA、审批、跨 run GUI lease、epoch、媒体与停止回执 | 用户 Mac 端到端通过，未知清理阻断下一个 owner；图片与候选证据闭环 |
| P2b：有结构的协作 | DSH Team、可选 Kimi Server Swarm、受管 Claude Workflow | 各自模式/队列/派发/后台责任/计量矩阵通过，不要求同日交付 |
| P3：复杂运行单元 | Tower 独立仓库、交互式 Teams、远端 review、隔离多 writer、显式压缩控制 | 每种能力独立所有权、权限与生命周期，完成代表性对比 |

P2a 与 P2b 可在各自依赖满足后并行；LCU 不必等待所有 Team/Swarm 能力完成。桌面 PTC
已经存在，新增的正式引擎能力初期 opt-in，等收益和失效行为验证后再讨论默认开启。

| 仓库 / 模块 | 后续实现责任 |
| --- | --- |
| `dsh-preset-generator` | 提取/复用版本化纯生成策略、输出 provenance；继续保护用户桌面配置 |
| `dsh-plugin-lcu` | 新旧 PTC 图片/handle 兼容、公开受管挂载/撤销接口、授权通道、epoch-aware handle、可等待清理回执、媒体事实 |
| DSH SDK / composition 扩展 | 路线 A 使用公开组合；路线 B 单独提供正式 creation seam，并验证修订/child 继承 |
| Flywheel `plugins/dsh` | bridge/policy/protocol、实际能力 probe、事件与媒体投影、client/host 状态展示 |
| Flywheel `internal` 与 contracts | 资源准入、journal/session/prewarm、driver 请求回执、schema/migration、恢复和验证 |
| 现有 Codex/Claude machine 插件 | 公开 controls/events、有效配置、子运行与用量，继续复用登录 |
| Kimi adapter | 保留 print；Server 为可选新增路径，独立拥有认证 runtime 与任务生命周期 |

新字段先进入 `docs/design.md` 与相应协议/schema 的正式设计，再实现兼容解析、状态迁移
和测试；此方案目前只在架构文档加入未来方向链接。根 npm 单包继续负责发布和安装，
不把源码目录当成需要用户另装的第二个 Flywheel 产品。

## 11. 验证与收益评估

### 11.1 行为验收矩阵

| 场景 | 验收要求 |
| --- | --- |
| 新旧 executor 基底、同名 preset 改动、恢复/预热 | 两种生成输入受支持；实际 executor 唯一；摘要变化使不兼容状态失效 |
| 同版本不同 PTC 协议、handle+image、零/单/多图、Image 身份 | 按实际 bundle/export/行为区分；结构化值与图片不互相覆盖；inspect/错误/取消符合真实运行契约 |
| SDK 未挂 preset、LCU attach 失败、无 `userQuestions`、未知 image route | 状态准确；required 在任务派发前拒绝，既有授权不被伪造或重复索取 |
| 两个 GUI run、serial/Quick/manual、无模型槽位 | 全部经过同一资源准入；无交叉输入和资源占用死锁 |
| 人或未受管应用改变焦点/页面 | 检测目标变化，暂停并重新观察；不声称租约提供物理桌面隔离 |
| ChatGPT.app 更新、REPL reset、旧 `h1`、连接替换 | epoch 不混用；先确认旧操作停止，再取得新对象和状态 |
| request abort、cleanup timeout、helper 退出失败、root 被杀 | 不能凭本地 Promise 或最终超时释放 GUI；责任可持久核对 |
| 截图成功但模型不支持图片、附件失败、host path 越界 | 区分三项媒体事实；诊断文字不伪装成图像证据 |
| 原生默认开协作、Agent 与 Workflow 并存、超深/挂起 children | 明确 off、有效 cap、整组额度、队列与清理都成立 |
| Claude fork/resume 绕过、Workflow 重放、Kimi side-chat 不可停止 | 未覆盖入口拒绝准入；按 attempt 区分重放，已完成写入不静默重做 |
| 多 channel children、账号别名、迟到 GUI 回答 | 按真实账号整组预留；迟到回答不能授权已取消或不同 owner 的调用 |
| helper 越界写、Bash/PTC/MCP 绕过、用户 agents 覆盖 | 逐权限路径验证；不能强制的限制不宣称支持 |
| usage 重复/丢失/占位零、父包含 child、服务重启 | 去重且保留 unknown；断线不自动重复创建任务 |
| Team 完成但 oracle 失败、Review 继承 Executor 历史、候选已变化 | 原验收/独立性继续生效；媒体/结果绑定确切候选 |

实现时需要真实 SDK/CLI 协议夹具、故障注入和用户 Mac/订阅路径端到端验收。涉及 Go
时遵守仓库逐函数 100% 语句覆盖要求，并运行相关 Node/bridge/race 门禁。本次文档
修改只验证文档范围、链接/锚点、引用、双语一致和独立源码评审，不以无关运行套件
替代这些未来验收。

### 11.2 是否值得默认开启

选取 Quick、复杂调查、跨模块功能、真实 UI 回归、批量迁移、植入缺陷 review、长会话
与中断恢复。固定 HEAD、候选/验收、模型/effort、账户、权限、输入、版本和资源条件，
顺序交错比较明确单代理、PTC、小型 helpers、GUI 证据、Team/Swarm/Workflow。

记录最终/首次验收率、漏报误报、返工、总耗时 p50/p95、token/cache、可观察订阅额度、
峰值模型并发、GUI 等待/占用时间、限流、取消后残余工作和证据完整率；包括失败与重试。
LCU 对交付质量的收益应体现为发现实际界面问题和形成可复验的证据，不能只统计截图
数量。默认值由代表性任务的质量、耗时和资源取舍决定。

## 12. 来源与研究范围

固定 commit 链接锚定源码；在线文档代表各自研究日期。下列事实支持本方案的接入
选择，不代替真实安装、模型运行、账户权益、Mac 授权或性能验收。

| 范围 | 主要来源 |
| --- | --- |
| Flywheel 模型与入口 | [模型模板][F1]、[DSH bridge][F2]、[宿主入口][F7]、[单包声明][F8] |
| 当前限制与资源 | [root policy][F9]、[scheduler][F10]、[session runtime binding][F11] |
| 已有 adapters | [Codex bridge][F3]、[Claude bridge][F4]、[Kimi driver][F5]、[Codex wire][F12] |
| 用户 presets | [README][U1]、[生成代码][U2]、[版本兼容测试][U3] |
| DSH composition/PTC | [SDK 默认组合][D9]、[官方 overlay][D10]、[server 创建][D11]、[PTC 基底][D12]、[registry][D13]、[tool contracts][D14] |
| DSH 协作 | [Team][D1]、[SubAgent][D2]、[SDK guide][D3]、[PTC][D4]、[Web][D5]、[Plan][D6]、[Goal][D7]、[protocol][D8] |
| LCU 挂载与 PTC | [注册/授权][L1]、[PTC 工具][L2]、[对象代理][L3]、[bundle][L8] |
| LCU 生命周期与媒体 | [generation][L4]、[connection][L5]、[图片投影][L6]、[runtime 检测][L7]、[changelog][L9]、[host guard][L10]、[helper cleanup][L11] |
| Kimi | [项目][K1]、[release][K2]、[agents][K3]、[CLI][K4]、[Server][K5]、[sessions][K6]、[Tower][K7]、[模型能力][M1] |
| Codex / Claude | [Codex agents][O1]、[app-server][O2]、[Claude subagents][C1]、[Teams][C2]、[Workflows][C3]、[ultrareview][C4]、[headless][C5]、[Advisor][C6] |

[F1]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/templates/flywheel.config.example.json
[F2]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/bridge.js
[F3]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/codex-pro-bridge.js
[F4]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/claude-code-pro-bridge.js
[F5]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/internal/drivers/kimi.go
[F6]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/package.json
[F7]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/index.js
[U1]: https://github.com/ckanner/dsh-preset-generator/blob/f692cb07abbb9786444c183d74eee018b1a2f0e8/README.md
[U2]: https://github.com/ckanner/dsh-preset-generator/blob/f692cb07abbb9786444c183d74eee018b1a2f0e8/src/gen-presets.mjs
[D1]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/team/agent-team/README.md
[D2]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/subagent/subagent/README.md
[D3]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/docs/user/guide/python-sdk.md#enable-team-mode
[D4]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/core/tool-executor-ptc/README.md
[D5]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/web/tool-web/README.md
[D6]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/plan/plan-mode/README.md
[D7]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/goal/goal-round-driver/README.md
[D8]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/sdk/protocol/README.md
[D9]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/bundle/sdk-app/README.md
[D10]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/apps/cli/config/examples/ptc.yml
[D11]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/sdk/server/src/server.ts
[D12]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/bundle/web-app/presets/ptc.patch.yml
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
[F8]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/package.json
[F9]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/plugins/dsh/src/policy.js
[F10]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/internal/machine/parallel.go
[F11]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/internal/machine/session_runtime.go
[F12]: https://github.com/ckanner/dsh-subagent-codex-pro/blob/37da3d75de32b809c4fb1a631ccc15191f3d87f9/src/wire.ts
[U3]: https://github.com/ckanner/dsh-preset-generator/blob/f692cb07abbb9786444c183d74eee018b1a2f0e8/tests/generate.spec.mjs
[D13]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/preset/agent-preset-registry/src/index.ts
[D14]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/core/tools/src/contracts.ts
[L1]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/index.ts
[L2]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/ptc.ts
[L3]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/bridge.ts
[L4]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/session.ts
[L5]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/connection.ts
[L6]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/tool.ts
[L7]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/app.ts
[L8]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/cordis.patch.yml
[L9]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/CHANGELOG.md
[L10]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/src/host-guard.ts
[L11]: https://github.com/ckanner/dsh-plugin-lcu/blob/9efeded13d20b41a96621bb53d903954f68550e6/helper/sky-service.mjs
[F13]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/templates/task-packet.schema.json
[F14]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/internal/drivers/dsh_prewarm.go
[F15]: https://github.com/ckanner/flywheel/blob/ac1ed320c68f6a8a466005985264e65b5a72fad9/internal/machine/sessions.go
[D15]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/bundle/base/cordis.patch.yml
[D16]: https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/core/tool-executor-ptc/package.json
[D17]: https://github.com/deepseek-ai/deepseek-harness/blob/39163dd30f703a79079e2d2446545cff3c453738/packages/core/tool-executor-ptc/package.json
[D18]: https://github.com/deepseek-ai/deepseek-harness/blob/39163dd30f703a79079e2d2446545cff3c453738/packages/core/tool-executor-ptc/src/ptc.ts
[D19]: https://github.com/deepseek-ai/deepseek-harness/blob/39163dd30f703a79079e2d2446545cff3c453738/packages/core/tool-executor-ptc/README.md
[D20]: https://github.com/deepseek-ai/deepseek-harness/blob/39163dd30f703a79079e2d2446545cff3c453738/packages/sdk/server/src/server.ts
[P1]: https://github.com/ckanner/dsh-subagent-codex-pro/compare/37da3d75de32b809c4fb1a631ccc15191f3d87f9...aaf465901def5aa1870c1165b9714d32294337a4
[P2]: https://github.com/ckanner/dsh-subagent-claude-code-pro/compare/ec0b99176b67512f3ca1838ec2a349dcb0189214...5eb637a9e82b317e67e50846005c407c54f1cea0
[K8]: https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-conversation.md
