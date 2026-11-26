# 会话中断恢复与上下文质量

[文档目录](../README.zh-CN.md) · [English](../en/session-recovery.md) · 简体中文

Flywheel 分别处理角色对话历史续用和中断调用恢复。fresh 与 scoped 都保留调用安全
检查。公开 journal 的持久化恢复要求 **Codex Pro 0.10.2** 或 **Claude Code Pro 0.7.0** 或 **Antigravity 0.1.0**；
DSH 与 Kimi 继续使用各自支持的恢复路径。

Antigravity 只读精确 owned journal，核对冻结的订阅/evidence-only绑定，不启动替代CLI，也不恢复全局latest。原生子任务完成与conversation级用量可能仍unknown。

## 恢复已有任务

使用原任务账本和集成目录，先查看状态，再通过正常引擎入口请求继续：

```sh
flywheel control status --cwd /repo --task-dir /repo/docs/my-task
flywheel control start --cwd /repo --task-dir /repo/docs/my-task
```

引擎会先核对先前的调用，再决定能否发起新请求。`pending` 或 `unknown` 可能使任务
保持阻断；应保留 journal 和诊断，不要删除状态或改成 fresh 会话来强行继续。
状态回复描述当前事实，不代表可以重放尚未确认的调用。

## 可观察的进展与调用时限

idle timeout 表示在配置的静默预算内没有收到可观察活动，不证明模型停止思考、
工具死锁或 Provider 拒绝请求。wall timeout 限制整次调用的总时长，即使持续有
进展也会生效。用户主动取消是另一种原因：子进程为了超时清理而确认 cancelled，
不会把原来的 idle／wall timeout 改成用户取消。

能观察到哪些进展，取决于实际 transport：

| 调用方式 | Flywheel 观察的进展 | 时限 |
|---|---|---|
| DSH SDK，包括一次性预热 | 已接纳根 Agent 和 owned local children 的公开 assistant-stream；已集成外部 Codex／Claude Provider 的有效回调 | 可配置的 idle 静默预算与 wall 总期限 |
| Codex Pro 0.10.2 / Claude Code Pro 0.7.0 | 本次 root 和 owned children 的白名单原生流式／工具活动，合并为有界元数据 | 可配置的 idle 静默预算与 wall 总期限 |
| Antigravity 0.1.0 | 原调用经校验的 init/step/result 元信息，不伪造心跳 | 可配置 idle 静默预算与 wall 总期限 |
| Kimi native | 归属于已接纳调用及 Agent 树的受支持原生事件 | 可配置的 idle 静默预算与 wall 总期限 |
| Kimi print / 旧 `codex exec` | 没有足以持续判断进展的输出契约 | 保留 wall 总期限，不因 print 静默执行 idle 终止 |

DSH 覆盖父 Agent 在 PTC 中等待、只有 owned native child 持续流式输出的情况。
外部 Provider、foreground PTC 和 background job 使用同一个版本化可选 Flywheel
活动服务；旧插件或未接入此回调的其他插件，在正常输出事件之间仍不可观察。
启动、首事件前等待、静默工具及不支持的 Provider 阶段仍可能耗尽配置的静默预算。
ready capability 如实列出这些边界，不承诺所有仍在运行的操作都会持续产生事件。

DSH 和 Pro 插件的活动从真实观察合并而来，只包含事件类型、次数和 root／child
范围，不带模型文本、隐藏推理或工具正文。计时器、ping 或人工心跳不会被用来维持调用。
活动是辅助观察，不是派发 ACK、业务结果、token 账单或清理证明。已校验的活动帧
不会累计挤掉有界 stdout 捕获中的终态回执；损坏或身份不匹配的帧仍使协议失败。

子任务归属必须匹配当前父 turn 和实际发送线程。旧轮次的 collaboration 事件、
外来线程都不能建立归属。Claude 的 ambient 后台监听任务不计入工作活性或完成
账本，后续 progress 即使省略 ambient 标记也保留这一分类。Claude init 之前的
启动 hook，只有 session ID 匹配调用方指定的新建／恢复历史时才计为活动。

传输流量与实际保留状态分别受限。Kimi native 的重复快照替换旧状态，不累计
消耗传输流量配额；Codex 保留的 native 归属与 usage 证据计入内存预算。单帧、
保留状态与最终结果的上限仍然有效。

DSH 调用中断后，完整且身份匹配的回执与正常进程退出，可以独立保留实际
Session ID、dispatch 事实及 SDK cleanupConfirmed；业务 outcome 仍为 unknown，
自动重放仍被阻止。截断／损坏传输、物理终止信号、缺少退出证据或清理未知，
不能提供这种证明。应使用下文既有的核对和人工规划退役入口；修改 timeout
不会解除先前未知调用的责任。
Pro 回执遵循相同边界：内层 ownership 不匹配、缺少 lifecycle 证据或外层流损坏，
都不能提供可信的 timeout、cancelled 或 cleanup 事实。原始回执仍保留供检查；
Go 自身观察到的超时和调用方取消不因此失效。
合法的 worker 进度／native 元数据校验错误或观察配额错误，与传输损坏不同。
其完整且身份匹配的清理回执可以保留独立核验的身份、usage 和 cleanup；业务结果
仍为 unknown，不能据此授予续跑 checkpoint，也不把清理所用的取消当作首因。

## 可读的原生会话名称

新建 DSH、Codex Pro、Claude Code Pro 和 Kimi 会话时，Flywheel 根据任务标题或
目标、角色及适用的工作范围生成可读名称。request ID 等技术标识仍保存在账本和
回执中。续接已有会话保留原名称，包括用户手动修改的名称。命名不额外调用模型，
不改变选定模型、思考级别、任务权限或恢复身份。

DSH 使用公开 title 服务并验证已接受的 snapshot，由原生 UTF-8 字节限制处理
截断。显式请求新标题而服务缺失或回读失败时，会在任务 prompt 前拒绝派发。
Codex／Claude Pro 使用各自受支持的原生命名接口。Codex 新建持久线程必须先
确认名称写入，才会发送 `thread-ready` 和任务。写入失败返回 `thread-name`
失败，保留 `not-dispatched`、已知 thread ID 与独立核验的清理事实；只读恢复
不重试命名或任务。仅存在一个空线程不代表它已经可以续接。旧 `codex exec`
不会因为共享新的超时策略就自动获得这种命名能力。

Kimi print 没有对应的 print title 参数。新建带名称的调用时，Flywheel 先通过
公开 Kimi session API 创建 idle 会话，验证名称与归属，并确认其拥有的准备
运行时已退出，再让 print 续接这个准确的会话。准备阶段不发送任务 prompt，
不进行标题生成推理，和正式执行共用同一个 wall 预算；原 print 的模型、auto
权限及有条件的 thinking effort 行为保持一致。API 不支持或准备清理未确认时
不派发；恢复已有会话不重新做这一步。

这次准备使用的公开 Kimi API 会打开并关闭原生会话。因此首次具名 print 若配置
了 `SessionStart` / `SessionEnd` hook，会在正式 print 续接之前多执行一轮
启动／关闭 hook；业务 prompt 仍只发送一次。Agent／Swarm 在原有创建流程中
直接设置名称，不需要这次额外生命周期。

这些契约由真实已发布 SDK、公开插件接口及离线模型／CLI fixture 的确定性
测试覆盖，不代表已经验证真实账号可用性、Provider 性能或桌面 UI 行为。

## 在原生客户端中找到会话

受管主任务的 `fresh` 控制下一次调用使用哪段输入历史，不要求丢弃已完成会话。
Codex Pro／Claude Code Pro 在 fresh 与 scoped 下都请求原生 persistent 历史。
fresh 每次不传旧 ID，创建独立、具名的会话；scoped 只有在兼容且证据确认后才续用。
DSH／Kimi 保持原有原生持久化，Flywheel 同时保留 fresh 调用观察到的真实 ID，便于查看。
知道一个 ID 不会单独赋予受管续接资格。

原生文件是否保存、列表是否显示，以及是否允许继续，是三个不同问题：

| 后端 | 已保存根会话的查看位置 | 列表与环境限制 |
| --- | --- | --- |
| DSH SDK | 使用相同 `DSH_HOME`、SDK 持久化存储和原项目工作区的 DSH 桌面端 | Flywheel 桌面插件通过公开 Workspace 服务附加匹配的独立 SDK 历史。不同 home／存储是不同历史；执行 Worktree 不成为额外桌面项目。 |
| Kimi print／受支持的 native API | 同一 `KIMI_CODE_HOME` 与项目下的 Kimi Code 会话 | 首次具名 print 先建立并核对空历史，再使用准确 ID。native Agent／Swarm 仍须完整公开 conversation API；通过服务器身份校验不代表具备这些能力。 |
| Codex Pro | 同一 `CODEX_HOME` 的本地历史、对应项目的原生历史／搜索，或 `codex resume <thread-id>` | app-server 记录的来源满足默认交互式来源筛选；客户端还可能按项目、模型 Provider、账号等过滤。只有 ID 的空线程可能尚无可续接 rollout。本地保存不等于跨设备云同步。 |
| Claude Code Pro | `claude --resume <UUID>`；支持的桌面环境可用 `claude --desktop --resume <UUID>` | 官方规则将 print／Agent SDK 会话排除在普通 picker 与最新会话续接之外。可读标题不会移除该筛选，也不会自动添加 Desktop 侧栏条目。 |

使用与原调用相同的本地用户、配置目录及项目环境。Flywheel 会话详情只读显示
原生 ID 和各 Provider 的入口说明；只有任务已停止、最新调用已消费且清理确认后，
才显示原生终端命令。命令使用原项目目录，旧记录回退到历史 cwd；只打开会话，
不会自动发送 prompt。需要继续受管任务时，使用 Flywheel 的人工接管与验证流程；
原生客户端不能代替调用对账。

Claude Desktop 接管要求交互终端、CLI 2.1.285 或以上、受支持的 macOS 或 x64 Windows、
已安装的 Desktop 应用和订阅登录。自定义 `CLAUDE_CONFIG_DIR` 必须一致，目标会话
不能仍被其他终端或后台会话占用。不能使用管道、print／SDK 模式或额外 prompt。
具体以官方[会话筛选规则](https://code.claude.com/docs/en/sessions#where-the-session-picker-looks)
与 [Desktop 接管条件](https://code.claude.com/docs/en/desktop#coming-from-the-cli)为准。
Codex 来源和列表契约见[原生历史说明](https://github.com/ckanner/dsh-subagent-codex-pro#thread-lifetime)。

旧 Pro ephemeral 调用没有可重建的原生 transcript，仍可查看 Flywheel 保存的最近回复。
旧 DSH／Kimi fresh 记录可从摘要已验证且 invocation 匹配的回执中取回只读展示 ID，
不回写账本、不开放复用；尚未由状态确认的回执必须先对账再显示。低层显式 ephemeral
请求保持不保存历史。Intake／Garden 的 Pro 调用也保留独立具名历史，安全重试仍新建 ID，
不参与角色账本、不复用返回 ID。DSH／Kimi 辅助调用获得可读标题，不改变原保存协议；
Kimi 具名 print 使用上述准备生命周期。外部 DSH 委派继续遵循显式配置／逐次持久化策略。
原生子任务 ID 也不自动等于可恢复的根 Session ID。

## 谁拥有哪些事实

| 层 | 责任 |
|---|---|
| Go 引擎 | 任务状态、角色与范围、调用意图、候选与契约检查、预算、计量、回执消费和交付 |
| Flywheel worker v3 | 传递有类型的生命周期帧；等待 Go 的持久化 ACK；转发取消；不作业务判断 |
| 上游 machine API | 独立 supervisor、私有 durable journal、原生进程 guardian、完整原始终态回执、只读 `recoverMachine` |
| 原生 Codex / Claude | 模型与工具执行、原生历史、实际配置与可观察 usage |

外层 worker 协议为 3，插件请求和回执仍是 machine protocol 1，生命周期单独为 version 1。DSH 和 Kimi 继续使用其已实现的正常历史续用及 Go 回执恢复；不能从这两个新插件的能力推断它们也有同样的 P3 接口。

## 派发前必须有持久化确认

Go 准备任务的 `.sessions/` 父目录并保存调用意图，上游 API 再独占创建本次调用的恢复子目录。子目录以调用 ID 的摘要命名；不在用户的全局模型历史中搜索。

上游按实际可观察边界发送 `thread-ready`、`dispatch-intent`、`dispatch-possible`、`turn-accepted`、`terminal` 和 `cleanup-confirmed`。不同后端可观察到的事件不同，不能自行补齐：

- **Codex** 的 `thread-ready` 在 `turn/start` 之前返回实际 thread/configuration。Go 在确认 ACK 前核对完整权限策略、cwd、模型和身份。任一确认失败会阻止后续 turn。
- **Claude** 在任务输入前核对 SDK initialize／版本握手，但原生 `system/init` 配置消息仍在提交 prompt 后返回。它提供派发前的 intent/possible ACK，但不宣称有 prompt 前的 thread-ready 或真实 turn-accepted；实际配置仍是终态接纳检查。

Go 将事件与原调用身份核对后，先原子保存并 `fsync` 检查点与状态，再回 ACK。缺失、错误、重复、过早或超时的 ACK 不能释放后续步骤。`persistent:true` 表示请求保留原生历史；不表示空 thread 已有可恢复 rollout。

上游 supervisor 与 worker 的生命周期分离，拥有原生 guardian。worker 断连时，它取消尚未确认的步骤并收束原生进程；终态回执在向 worker 通知终态之前已落盘。通知或最后 ACK 丢失不能把原本成功的完整回执改成“未执行”。

## 恢复只读，不会重发 prompt

重新启动同一任务时，Go 根据原调用保存的 adapter 版本、request/run ID、任务目录与绑定，调用公开 `recoverMachine`。它只读取那个调用的 journal，不启动原生 CLI，不执行 `thread/resume`，不查询其他任务，不重新提交 prompt。

| 恢复返回 | 含义与引擎行为 |
|---|---|
| `terminal` | 找到同一调用的完整原始回执；继续核验身份、执行终态、清理、用量与原候选证据 |
| `pending` | 尚未找到已持久化终态；不证明原进程仍存活，保持阻断 |
| `unknown` / 损坏 / 错配 | 无法证明该次调用结果，保留证据并阻断；不能换 fresh、换后端或删账本来重放 |

只读查询可以重复。原生机器、历史文件或 journal 不可用时，不承诺跨机器恢复。supervisor 自身也被终止时可能只剩 pending；本实现没有 exactly-once 执行保证。

### 人工退役未知的规划调用

规划角色（`design-plan`、`design-review`、`plan`、`plan-review`）的调用如果没有可信 dispatch/outcome，且没有可恢复的 journal，可以明确退役，而不是让整个任务永久锁死。这个动作不是恢复或重放：它保留 unknown 事实，只结束该次调用的副作用范围。

```sh
flywheel session /absolute/repo/docs/my-task \
  --key design-plan/planning:engine-1-1 \
  --abandon --reason "idle watchdog killed a live model call"
```

要求：记录为 `blocked`、已计量、清理已确认，且 dispatch 或 outcome 至少一个仍为 unknown；角色必须是规划角色。命令在控制终端显示准确调用 ID 并要求输入 `abandon <invocation>`；Flywheel 视图的同一动作只接受来源为真实用户的 `/flywheel` 命令。证明绑定 taskDir、key、revision、reason 和准确 invocation，不能跨任务或重复使用；模型工具不暴露该动作。有完整 shell 的模型进程已能直接改写 state.json，不属于本宿主证明边界。

退役后记录变为 `abandoned`，保留原因、时间、来源和旧 thread 证据；旧 thread 永不续用。不应用任何阶段结果，也不再次调用模型。关联规划 request 标为 `cancelled`，任务回到 `paused`；之后显式运行 Plan 会创建新 request。`control session` 会返回 `session-control.applied/status/message`，不能用控制信封的 `ok:true` 推断退役成功。

旧 Kimi resident Plan 的精确关联续会话缓存，会在同一次状态保存中同步退役；其他线程和未知缓存仍保持阻断。原线程和 unknown dispatch/outcome 另保留在审计历史中，后续旧式调用覆盖 session 最新记录时不会丢失。DSH 先核对原始用户命令，再把引擎证明绑定到规范化任务路径；符号链接别名、尾斜线仍指向同一任务，但证明不能跨任务复用。

人工 send 在派发前取消时，可能留下已 consumed 的回执和 pending 接管状态。Reconcile 清除 pending 后，会按这次真实转换报告 applied；重复核对已完成状态不算新成功动作。
如果无法读取操作前的 invocation，回执保留已观察到的操作后状态，但不能证明
这是本次命令造成的变化。此时 `applied:false` 的 message 会明确说明无法验证、
命令可能已经执行；应先刷新状态再进行下一次修改，不能把它当作未执行的证明。

这不是“删除账本来重放”：原账本、receipt 和 unknown 证据仍保留。不要手改 `state.json` 或搬走 `.sessions/` 来伪造终态。

### 回执成功仍须有原候选证据

`cleanup-confirmed` 到达时，Go 在 ACK 前保存该次执行结束后的候选摘要、原角色 guard，以及 Execute 的报告。恢复得到成功回执后，必须同时有这份原调用的证据，才能应用业务结果。重启后重新拍摄的工作区、重新跑过的验收不能替代原检查点。

若 supervisor 保存了回执，但 Go 尚未来得及记录原候选，则可恢复可信用量，业务仍保持阻断。只有物理事实证明未派发、清理完成且候选与原 guard 完全未变的拒绝，才允许按已知未执行处理。独立 Review/Judge 的原契约与候选检查继续适用。

### 共享目录并行

共享目录并行的 Execute 回执使用已声明写入范围的候选指纹，并把该范围绑定到会话身份；其他任务的独立变更不会导致本调用无故丢失归属。自己的范围变化仍拒绝旧回执。L1、独立 Review 与批次提交始终核对完整共同候选；范围回执不能证明整组完成，也不能绕过未知派发、结果或清理。

### DSH 预热运行时的重启核对

可选 `scheduler.prewarm` 在确定性检查期间准备 DSH Review runtime，不发送
模型 prompt。Go 在启动前保存单个 `worker-prewarm` 清理责任，绑定准备 ID、
角色／范围、配置身份和 run；终态回执私有保存在 `.control/prewarm/` 下。
这份回执不是模型调用或 Review 通过证据。

完整终态 JSON 在 UTF-8 序列化后最多 8 KiB，包含转义字符和末尾换行。过长的错误
诊断会保留带截断标记的前缀，正常短错误以及全部身份、派发和清理字段保持完整。
诊断截断不会放宽任何恢复检查，也不会把初始化失败变成允许冷启动回退的安全过期。

引擎重启不会续用这个待用 worker。只有原身份匹配的回执证明没有提交 prompt、
SDK 清理已确认，且记录的 worker 与其原进程组都已退出，才可解除原清理责任。
发布回执时同步文件与目录；恢复时在清除状态前重新确认这两项持久化。实际从未
启动进程时，可以使用明确记录的本地 no-process 回执；不能仅凭经过时间或缺失
PID 推断。回执缺失、超限、损坏、身份不匹配、进程或进程组仍存活或无法确认退出、
清理未知都会保持阻断；关闭预热也不能跳过核对。安全收束后，后续工作才按当前
配置和正常调用检查继续。

在同一个引擎运行中，待用实例安全过期、且尚未尝试任何正式派发时，可在同一
已持久化 invocation 身份和剩余时限内冷启动一次。这不属于重启续用，不增加一次
模型计费尝试，也不能绕过未知效果。取消、正式调用时限耗尽、清理不完整或输出
不可信时都不能这样回退。

首次创建预热记录会将 `state.sessions.version` 提升为 **3**，后续操作保持该
版本。这个存储兼容标记使旧引擎拒绝无法理解的清理责任；新引擎仍支持版本 1、2。
配置的 `sessions.policy-version` 仍为 1，worker 和 lifecycle 各自维持独立的
协议版本。不要手改存储版本或删回执来强行降级。

## 计量与预算

已消费的调用不再次计数，不重发模型请求。原调用曾因丢失回执计为 unknown、后又找回可信 usage 时，Go 按同一调用记一条可审计 correction；保留原 provider、原日桶和 lifetime 归属。已过保留窗口的日桶不会因为恢复而重新出现。

状态采用 CAS，receipt、状态和 metrics 都在成功前持久化。metric 已追加但状态 CAS 失败时，重试不会追加第二份相同调用；已存在行也必须再次确认文件与目录已同步，不能将“可读”当作持久化完成。

未知仍不是零。恢复中的质量重建、Quick → Full、角色模式变化和新 worker 都不重置预算、尝试次数或未知调用阻断。预算只约束新的派发，不妨碍消费已经执行完成且证据齐全的结果。

## 可解释的上下文重建

`sessions.roles` 可分别选择 Plan、Execute、Review、Judge、judge-panel 的 fresh/scoped；未覆盖的角色沿用总模式。具体配置见[会话策略](configuration.md#会话策略历史复用与调用安全)。

可选 `sessions.quality` 接受三个有界指标：

| 字段 | 观测事实 |
|---|---|
| `max-confirmed-turns` | 当前角色历史已确认且去重后的调用次数 |
| `max-unproductive-turns` | Execute 连续完成调用但候选摘要未变化的次数 |
| `max-run-input-tokens` | 最近一次可完整归属的实际输入 token 数；不是猜测的上下文窗口大小 |

所有阈值默认关闭。只有原调用已安全终止、计量并消费后，阈值才可让下一次调用重建。unknown 或待消费结果优先处理，不能借重建绕过。

重建保留明确原因与 generation，并用当前 Spec、packet、工作区、合法报告、验证与结构化反馈交接。它不读取另一个角色的私有 transcript，不以“压缩成功”伪造模型事实，也不另发未计量的模型压缩调用。Review 即使续用自己历史，仍须检查完整当前候选。

### 可信的增量输入

默认情况下（`sessions.context` 省略或设为 `"verified-incremental"`），Execute
与规划角色（`design-plan`、`design-review`、`plan`、`plan-review`）均可复用可信同角色的
稳定契约（或规划 Layer 1/2）；Go 复核已消费回执明确证明 `dispatched` 且 `completed`，并检查提示词
版本、当前契约（或规划稳定权威与 `project-context.md`）及可读 context/cache 文件摘要。如需每轮完整重发，可显式配置
`sessions.context: "full"`。安全的启动拒绝或失败／取消调用
可以保留原线程，但不证明本轮新准备的权威内容已送达；下一次仍须完整重发。
新输入携带反馈、候选身份、尝试和引用（或规划 Layer 3 易变增量）。权威内容变化或不可用、旧任务缺
检查点、上下文回执无效时回到完整输入，角色记录保存选择与原因。Review、Judge
保持完整当前输入，不继承 Executor 私有历史。

可选[执行策略](configuration.md#有界执行策略显式启用)可选择另一完整允许绑定。
每次切换均新建历史代次，仅改变 effort 也不例外；显式会话模式同样进入绑定
身份。unknown／pending 调用先对账，新上下文不能授权重放未知副作用。切换记录
区分质量升级和渠道后备，保留原任务及 packet 预算。

## 对照评估与证据边界

仓库提供固定源码与独立 oracle 的评估工具，比较 fresh、仅 Execute scoped、各角色 scoped。覆盖一次成功、L1 返修、Review return、长上下文、停止恢复、配置改变；轮换策略顺序并要求重复运行。工具记录完成质量、漏项、回归、返修轮数、重复 findings、耗时、已知/未知 token 与缓存、重建原因和恢复失败。场景是否发生由实际状态证据判断，不靠名称。

离线 native fixture 经过真实 Go、worker v3 和公开插件接口，验证评估工具及安全边界；它不是实际模型质量、缓存率或节省比例。真实账号的对照结果需要可用的原生 CLI、账号、权限与模型环境。未获得该结果前不自动启用阈值，不改变任何默认模型或 loop。操作步骤与实验产物说明见[配对评估指南](session-evaluation.md)。

## 保存和排障

上游恢复目录与文件使用私有权限、独占创建、身份/摘要检查和原子持久化；已存在目录不会触发二次执行。它们属于任务审计证据，可能含模型输出与诊断。按任务一起保存、备份和控制访问，不复制凭证，也不因安装升级而删除未知调用的证据。

当前没有自动删除这些恢复证据的保留计时器。确认任务完成，或对规划角色使用上面的显式退役动作之后，才能按项目的任务归档流程处理；删除 journal 不是解除阻断的恢复方法。`control status`/doctor 是查看入口，模型与 skills 不修改 `state.json`。需要恢复时仍从原任务目录使用正常 Go 启动入口，由引擎核对并决定下一步。
