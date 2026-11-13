# P5 继续复核：就绪队列会话与预热错误处理

[验证索引](README.md) · [P5 实施方案](../plans/2026-11-13-p5-ready-queue-prewarm.md) · [前次复核](2026-11-13-p5-review-closure.md)

## 源码与范围

本次从 PR #37 的 `89a936d0cc0e74d49f0a1f9edcd6d7d249bf7db4` 继续，
最终源码以承载本记录的提交为准。该基线已经包含原先暂缓的 ready queue、
Execute／L1 槽位流水化和一次性 DSH Review 运行时预热。本轮对照原方案及最新
实现，补齐实际会话与子进程路径中的两个问题及其相邻竞争条件。

调度默认仍为 `dispatch=batch`、`prewarm=false`，唯一 JEV 开关仍为
`jev.enabled`。真实账号、Mac 桌面、质量／耗时／token／费用配对和默认启用
决定按用户要求后置。任意外部补丁导入、常驻 worker 池与没有公开初始化接口的
其他 harness 预热，不属于当前 P5 承诺范围；原整体方案 §14 允许候选接纳
独立排期，P5 实施方案 §5 已评估现有受管 Session 的接管与交回路径。

## 修复与失败复现

### 1. 就绪队列补位不恢复正在运行的兄弟调用

原 `newWorker` 直接复制协调器当前完整状态。首窗中的慢任务仍在运行时，它的
Session `intent` 已经持久化；随后补入的 worker 复制到该记录，在自己的
`invokeScoped` 中把它当作遗留调用恢复。由于真实终态尚不存在，新任务在派发前
就报缺少回执，并取消整组。已有 mock harness 补位测试未经过这条调用跟踪路径。

本轮使用实际 `invokeScoped`、会话账本、回执保存和真实 Git worktree，配合
离线 adapter fixture 复现。fresh 与 scoped 两种方式在原实现均只能派发两项，
第三项不能在慢任务结束前补位。fixture 不代表真实模型服务。

修复为每一轮保存完整的派发前会话快照，所有 worker 从该快照开始维护自己的
调用。最新 packet 和预算仍取自协调器；协调器持续合并本 packet 会话、检查
重复历史并记账。历史遗留未核对调用仍保留在快照和启动恢复中，没有全局跳过
`reconcileSessionReceipts`。

独立复核进一步发现同一边界的反向风险：真实 unknown 终态已保存，但 worker
还未从 Execute 循环返回时，补位不能继续。终态文件保存失败时，记录则仍为
`intent`，并通过 `Action=wait` 表达阻断。两个回归均在修复前实际观察到 peer
多派发一次。现在协调器一旦合并到 `blocked` 或 `wait` 就停止新接纳并取消，
已有可信回执继续记账；无法保存的回执保持未确认、未记账。

新增回归覆盖 fresh／scoped 补位、跨 packet 历史冲突、真实 unknown 在切换
调度／会话方式后继续阻断、批次前未完成回执保留，以及上述两个提前停止准入场景。

### 2. 预热初始化失败保留原分类

原实现过滤预热控制帧后，只从私有回执恢复未派发与清理事实，丢失
`warm-result.error`。真实受管子进程回归确认 `INVALID_REQUEST`、`AUTH`、
`QUOTA`、`SERVER`、`RATE_LIMIT` 均变为 `TRANSPORT`，改变既有重试或后备
判断。真实 Go → Node worker 使用不支持的 desktop profile，也复现该错误，
且确认初始化失败前没有启动 SDK。

现在只接受既有错误枚举；完整协议流、精确未派发回执、SDK／进程组清理、正常
错误退出码及无取消／超时等条件全部满足后，保留原错误码和诊断消息。未知／空
错误码、损坏尾帧、矛盾退出码等不取得可信结构化分类，未知清理继续阻断。

另外，初始化可能先失败，再等待 SDK 清理，期间 Go TTL 已触发。原
`prepareExpired=true` 会使随后到达的 AUTH／SERVER 被当作正常过期，实际发起
一次冷调用。新增延迟终态回归在修复前观察到 `cold-model-call`，修复后明确
拒绝非 TIMEOUT 的已知初始化失败获得冷回退。真正 TIMEOUT 的精确回执、同一
RequestID、剩余时限和清理要求保持。

## 最终工程验证

环境：Linux x86_64、Go 1.27.4、Node v24.22.1、npm 11.9.0。Go 使用既有
精确版本模块缓存与 `GOPROXY=off`、`GOTOOLCHAIN=local`；依赖声明和锁未变。

| 检查 | 本轮结果 |
| --- | --- |
| 新增行为在旧实现失败、修复后通过 | 已确认；调度使用原 HEAD 的 Go overlay 复现，预热包含真实子进程及公开 Node worker |
| `make quality-gate` | 通过：vet、格式、全包测试及每个被测 Go 函数 100% statement coverage；原单行 cmd 入口例外不变 |
| 调度相关 `-race` | 通过；最终 `parallel.go` 两个变更函数均 100% statement coverage |
| 全部 `TestDSHPrewarm*` 的 `-race` | 通过；21 个预热函数均 100% statement coverage |
| 根 `npm test` | 142 通过，0 失败、0 跳过 |
| `npm --prefix plugins/dsh run check` | 274 主测试与 9 旧宿主兼容测试通过，0 失败、0 跳过 |
| DSH／Codex Pro／Claude Code Pro Go → Node `-race` bridge | 全部通过，0 跳过，包含本轮新增初始化错误回归 |
| 最终 native build 与 session evaluation | 通过：36／36 场景、96 次实际离线 adapter 调用；0 未观察场景、0 unknown outcome |
| 文档链接、锚点、围栏与 diff | 6 个变更 Markdown、165 个相对链接、43 个锚点、36 个围栏通过；`git diff --check` 通过 |
| 独立代码复核 | 调度会话隔离、未知／回执保存失败准入与预热分类／过期顺序复核无剩余阻塞项 |

定向命令如下；定向包覆盖率只代表过滤后的测试范围，不能替代完整逐函数门禁：

```bash
go test -race ./internal/machine \
  -run '^(TestReadyQueue|TestParallel|TestFullScheduler|TestDeclaredWriteScope)' \
  -count=1
go test -race ./internal/drivers -run '^TestDSHPrewarm' -count=1
FLYWHEEL_TEST_NODE_BRIDGE=1 go test -race ./internal/drivers \
  -run '^Test(DSH|CodexPro|ClaudeCodePro)NodeWorkerOfflineBridge$' -count=1 -v
make quality-gate
make build
FLYWHEEL_EVAL_BINARY="$PWD/flywheel" npm run test:sessions
```

插件依赖沿用前次已验证环境。可选 Claude Code Pro 0.4.0 使用前次已审阅源码
tarball，并非本轮从 registry 获取；详见[前次依赖身份记录](2026-11-13-p5-review-closure.md#本轮工程验证)。
没有修改 npm 发行逻辑，也没有重建六包候选；历史候选安装证据仍只覆盖其记录的
源码，不作为本次源码的六包发行验收。

## 远端检查与交付

基线提交对应的 [CI run 43418248409](https://github.com/ckanner/flywheel/actions/runs/43418248409)
仍为 failure，`npm-candidate` 失败、下游 test skipped。本轮提交的精确远端状态
单独记录在 PR；本地门禁通过不等于远端 CI 通过，也不证明实际模型性能或桌面效果。

本轮交付继续更新 PR #37；不将源码提交称为已经合并、打 tag、发布 npm 或更新
用户安装。当前约定的工程功能，包括 P5 可选优化，均已有实现；真实效果验收仍
属于后续工作。
