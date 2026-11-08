# 会话中断恢复与上下文质量

本指南描述会话方案 P3/P4 的当前源码实现。P1/P2 的角色历史续用和已保存回执消费继续有效；新能力需要配套的 **Codex Pro 0.9.0** 或 **Claude Code Pro 0.4.0** 公开 machine API。源码、npm 发行、账号可用性和真实任务效果是不同的验收项。

## 1. 谁拥有哪些事实

| 层 | 责任 |
|---|---|
| Go 引擎 | 任务状态、角色与范围、调用意图、候选与契约检查、预算、计量、回执消费和交付 |
| Flywheel worker v3 | 传递有类型的生命周期帧；等待 Go 的持久化 ACK；转发取消；不作业务判断 |
| 上游 machine API | 独立 supervisor、私有 durable journal、原生进程 guardian、完整原始终态回执、只读 `recoverMachine` |
| 原生 Codex / Claude | 模型与工具执行、原生历史、实际配置与可观察 usage |

外层 worker 协议为 3，插件请求和回执仍是 machine protocol 1，生命周期单独为 version 1。DSH 和 Kimi 继续使用其已实现的正常历史续用及 Go 回执恢复；不能从这两个新插件的能力推断它们也有同样的 P3 接口。

## 2. 派发前必须有持久化确认

Go 准备任务的 `.sessions/` 父目录并保存调用意图，上游 API 再独占创建本次调用的恢复子目录。子目录以调用 ID 的摘要命名；不在用户的全局模型历史中搜索。

上游按实际可观察边界发送 `thread-ready`、`dispatch-intent`、`dispatch-possible`、`turn-accepted`、`terminal` 和 `cleanup-confirmed`。不同后端可观察到的事件不同，不能自行补齐：

- **Codex** 的 `thread-ready` 在 `turn/start` 之前返回实际 thread/configuration。Go 在确认 ACK 前核对完整权限策略、cwd、模型和身份。任一确认失败会阻止后续 turn。
- **Claude** 只有提交 prompt 后才返回原生 init。它提供派发前的 intent/possible ACK，但不宣称有 prompt 前的 thread-ready 或真实 turn-accepted；实际配置仍是终态接纳检查。

Go 将事件与原调用身份核对后，先原子保存并 `fsync` 检查点与状态，再回 ACK。缺失、错误、重复、过早或超时的 ACK 不能释放后续步骤。`persistent:true` 表示请求保留原生历史；不表示空 thread 已有可恢复 rollout。

上游 supervisor 与 worker 的生命周期分离，拥有原生 guardian。worker 断连时，它取消尚未确认的步骤并收束原生进程；终态回执在向 worker 通知终态之前已落盘。通知或最后 ACK 丢失不能把原本成功的完整回执改成“未执行”。

## 3. 恢复只读，不会重发 prompt

重新启动同一任务时，Go 根据原调用保存的 adapter 版本、request/run ID、任务目录与绑定，调用公开 `recoverMachine`。它只读取那个调用的 journal，不启动原生 CLI，不执行 `thread/resume`，不查询其他任务，不重新提交 prompt。

| 恢复返回 | 含义与引擎行为 |
|---|---|
| `terminal` | 找到同一调用的完整原始回执；继续核验身份、执行终态、清理、用量与原候选证据 |
| `pending` | 尚未找到已持久化终态；不证明原进程仍存活，保持阻断 |
| `unknown` / 损坏 / 错配 | 无法证明该次调用结果，保留证据并阻断；不能换 fresh、换后端或删账本来重放 |

只读查询可以重复。原生机器、历史文件或 journal 不可用时，不承诺跨机器恢复。supervisor 自身也被终止时可能只剩 pending；本实现没有 exactly-once 执行保证。

### 回执成功仍须有原候选证据

`cleanup-confirmed` 到达时，Go 在 ACK 前保存该次执行结束后的候选摘要、原角色 guard，以及 Execute 的报告。恢复得到成功回执后，必须同时有这份原调用的证据，才能应用业务结果。重启后重新拍摄的工作区、重新跑过的验收不能替代原检查点。

若 supervisor 保存了回执，但 Go 尚未来得及记录原候选，则可恢复可信用量，业务仍保持阻断。只有物理事实证明未派发、清理完成且候选与原 guard 完全未变的拒绝，才允许按已知未执行处理。独立 Review/Judge 的原契约与候选检查继续适用。

## 4. 计量与预算

已消费的调用不再次计数，不重发模型请求。原调用曾因丢失回执计为 unknown、后又找回可信 usage 时，Go 按同一调用记一条可审计 correction；保留原 provider、原日桶和 lifetime 归属。已过保留窗口的日桶不会因为恢复而重新出现。

状态采用 CAS，receipt、状态和 metrics 都在成功前持久化。metric 已追加但状态 CAS 失败时，重试不会追加第二份相同调用；已存在行也必须再次确认文件与目录已同步，不能将“可读”当作持久化完成。

未知仍不是零。恢复中的质量重建、Quick → Full、角色模式变化和新 worker 都不重置预算、尝试次数或未知调用阻断。预算只约束新的派发，不妨碍消费已经执行完成且证据齐全的结果。

## 5. 可解释的上下文重建

`sessions.roles` 可分别选择 Plan、Execute、Review、Judge 的 fresh/scoped；未覆盖的角色沿用总模式。具体配置见[会话策略](configuration.md#会话策略历史复用与调用安全)。

可选 `sessions.quality` 接受三个有界指标：

| 字段 | 观测事实 |
|---|---|
| `max-confirmed-turns` | 当前角色历史已确认且去重后的调用次数 |
| `max-unproductive-turns` | Execute 连续完成调用但候选摘要未变化的次数 |
| `max-run-input-tokens` | 最近一次可完整归属的实际输入 token 数；不是猜测的上下文窗口大小 |

所有阈值默认关闭。只有原调用已安全终止、计量并消费后，阈值才可让下一次调用重建。unknown 或待消费结果优先处理，不能借重建绕过。

重建保留明确原因与 generation，并用当前 Spec、packet、工作区、合法报告、验证与结构化反馈交接。它不读取另一个角色的私有 transcript，不以“压缩成功”伪造模型事实，也不另发未计量的模型压缩调用。Review 即使续用自己历史，仍须检查完整当前候选。

## 6. P4 对照评估与证据边界

仓库提供固定源码与独立 oracle 的评估工具，比较 fresh、仅 Execute scoped、各角色 scoped。覆盖一次成功、L1 返修、Review return、长上下文、停止恢复、配置改变；轮换策略顺序并要求重复运行。工具记录完成质量、漏项、回归、返修轮数、重复 findings、耗时、已知/未知 token 与缓存、重建原因和恢复失败。场景是否发生由实际状态证据判断，不靠名称。

离线 native fixture 经过真实 Go、worker v3 和公开插件接口，验证评估工具及安全边界；它不是实际模型质量、缓存率或节省比例。真实账号的对照结果需要可用的原生 CLI、账号、权限与模型环境。未获得该结果前不自动启用阈值，不改变任何默认模型或 loop。操作步骤与实验产物说明见[配置指南](configuration.md)及 `scripts/prepare-session-evaluation.js`、`scripts/evaluate-sessions.js` 的帮助。

## 7. 保存和排障

上游恢复目录与文件使用私有权限、独占创建、身份/摘要检查和原子持久化；已存在目录不会触发二次执行。它们属于任务审计证据，可能含模型输出与诊断。按任务一起保存、备份和控制访问，不复制凭证，也不因安装升级而删除未知调用的证据。

当前没有自动删除这些恢复证据的保留计时器。确认任务完成或由人解决未知结果之后，才能按项目的任务归档流程处理；删除 journal 不是解除阻断的恢复方法。`control status`/doctor 是查看入口，模型与 skills 不修改 `state.json`。需要恢复时仍从原任务目录使用正常 Go 启动入口，由引擎核对并决定下一步。
