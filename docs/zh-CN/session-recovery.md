# 会话中断恢复与上下文质量

[文档目录](../README.zh-CN.md) · [English](../en/session-recovery.md) · 简体中文

Flywheel 分别处理角色对话历史续用和中断调用恢复。fresh 与 scoped 都保留调用安全
检查。公开 journal 的持久化恢复要求 **Codex Pro 0.10.0** 或 **Claude Code Pro 0.5.1**；
DSH 与 Kimi 继续使用各自支持的恢复路径。

## 恢复已有任务

使用原任务账本和集成目录，先查看状态，再通过正常引擎入口请求继续：

```sh
flywheel control status --cwd /repo --task-dir /repo/docs/my-task
flywheel control start --cwd /repo --task-dir /repo/docs/my-task
```

引擎会先核对先前的调用，再决定能否发起新请求。`pending` 或 `unknown` 可能使任务
保持阻断；应保留 journal 和诊断，不要删除状态或改成 fresh 会话来强行继续。
状态回复描述当前事实，不代表可以重放尚未确认的调用。

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
- **Claude** 只有提交 prompt 后才返回原生 init。它提供派发前的 intent/possible ACK，但不宣称有 prompt 前的 thread-ready 或真实 turn-accepted；实际配置仍是终态接纳检查。

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

规划角色（`design-plan`、`design-review`、`plan`、`plan-review`）的调用如果没有可信 dispatch/outcome，且不是 Kimi/DSH 那种可恢复 journal，可以明确退役，而不是让整个任务永久锁死。这个动作不是恢复或重放：它保留 unknown 事实，只结束该次调用的副作用范围。

```sh
flywheel session /absolute/repo/docs/my-task \
  --key design-plan/planning:engine-1-1 \
  --abandon --reason "idle watchdog killed a live model call"
```

要求：记录为 `blocked`、已计量、清理已确认，且 dispatch 或 outcome 至少一个仍为 unknown；角色必须是规划角色。命令在控制终端显示准确调用 ID 并要求输入 `abandon <invocation>`；Flywheel 视图的同一动作只接受来源为真实用户的 `/flywheel` 命令。证明绑定 taskDir、key、revision、reason 和准确 invocation，不能跨任务或重复使用；模型工具不暴露该动作。有完整 shell 的模型进程已能直接改写 state.json，不属于本宿主证明边界。

退役后记录变为 `abandoned`，保留原因、时间、来源和旧 thread 证据；旧 thread 永不续用。不应用任何阶段结果，也不再次调用模型。关联规划 request 标为 `cancelled`，任务回到 `paused`；之后显式运行 Plan 会创建新 request。`control session` 会返回 `session-control.applied/status/message`，不能用控制信封的 `ok:true` 推断退役成功。

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

显式 `sessions.context:"verified-incremental"` 允许 Execute 复用可信同角色的
稳定契约；Go 复核已消费回执明确证明 `dispatched` 且 `completed`，并检查提示词
版本、当前契约及可读 context/cache 文件摘要。安全的启动拒绝或失败／取消调用
可以保留原线程，但不证明本轮新准备的权威内容已送达；下一次仍须完整重发。
新输入携带反馈、候选身份、尝试和引用。权威内容变化或不可用、旧任务缺
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
