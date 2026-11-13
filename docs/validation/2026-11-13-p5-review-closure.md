# PR #37 后续复核：P5 恢复边界与 JEV 凭据名称

[验证索引](README.md) · [P5 实施计划](../plans/2026-11-13-p5-ready-queue-prewarm.md) · [前次 P5 工程验证](2026-11-13-p5-ready-queue-prewarm.md)

## 源码与范围

本次从 PR #37 的 `f75fd1f59773f9083e55ea4bbe0a7f0b0203d159` 继续。
该基线已包含 ready queue、Execute／L1 流水化和一次性 DSH 初始化预热。
本轮在独立 checkout 复核并补齐暂停、迟到计时器和 JEV 配置冻结边界；最终源码
以承载本记录的提交为准。前次测试和六包候选身份继续只对应前次记录中的源码。

默认保持 `scheduler.dispatch=batch`、`scheduler.prewarm=false`；不改变现有
Provider、模型、并发、JEV 总开关、依赖锁或用户配置。真实账号、Mac 桌面、净性能
收益及默认启用决定按用户要求后置。通用外部 patch 导入和长期 worker 池不属于
本轮原有 P5 工程范围。

## 修复与失败复现

### 并行任务在 Review 前暂停

当前轮次 Execute／L1 全部通过后，原实现将这些新候选当作恢复候选，绕过
`--pause-before review`。基线回归中 batch 已提前调用两次 Review，ready queue
直接把整个任务完成。本轮在已派发 worker 停稳后保存 `paused`，保留 `l1-passed`
候选及工作区，不评审、不集成。恢复后每个 packet 的 Execute 仍只发生一次；
后续集成改变候选时照常重新验证。

batch 的暂停点在已接纳的有界批次后；ready queue 可先补完当前固定独立候选集。
未接纳项没有新目录、调用意图或尝试次数。既有失败、取消和预算出口保持。

### 预热回调重新核对当前事实

原 `AfterFunc` 回调已经排队后，`Stop`／`Reset` 不能撤回它。新增回归在基线上
实际复现：刷新 activity 后仍被旧 idle 回调取消、进程结束后被改记超时，以及
初始化失败被迟到准备 TTL 改成一次冷模型调用。

修复让 idle 回调核对最新活动期限，准备与正式调用回调核对物理进程退出。
进一步的受管进程回归发现，仅核对 `processDone` 还不够：worker 已发出
`warm-result(error=SERVER)`、但尚未退出时，TTL 仍可能改写结果并触发冷调用。
因此准备回调也尊重已收到的终态。真实 `TIMEOUT` 对照仍沿同一 RequestID 冷启动，
保留全部未派发、清理和剩余期限要求；初始化失败不取得该权限。

独立 review 又发现，直接在 `warmTerminal` 时返回会同时跳过关闭计时器，导致
仍活着的 worker 不在原宽限内退出；未手动释放 stdin 的真实进程回归复现该失败。
最终实现只抑制终态重分类，对仍存活的未交接 worker 继续安排原有界关闭，不重置
已经开始的关闭宽限。收到终态与完成物理清理始终分别确认。

初次定向覆盖检查的 `expirePrepare` 为 88.9%，其余 20 个函数为 100%。通过真实
“私有未派发收据存在、没有 stdout 终态帧、进程已经退出”的 fixture 补齐物理退出
分支，最终 21 个预热函数均为 100%。没有人为改写终态标志来填充覆盖率。

### JEV 凭据名称可冻结并恢复

PR review 指出的配置／契约不一致仍存在于基线：配置允许小写和混合大小写，
`preparation.schema.json` 的 `decision.key-env` 却只允许大写。真实配置加载到
`Start`／`SavePreparation` 的回归复现四个失败：小写、混合大小写、下划线开头
及 120 位混合名称。

本轮只放宽准备契约的字符集，保留原 128 位上限，保持原名称和契约哈希。
新配置仍限制为 120 位；旧 121／128 位大写冻结契约继续可读，129 位及非法名字
仍拒绝。测试覆盖再次 CAS 保存和读取，并确认非法配置不创建 preparation。

## 本轮工程验证

环境：Linux x86_64、Go 1.27.4、Node v24.22.1、npm 11.9.0。
Go 使用已有精确版本模块缓存，`GOPROXY=off`、`GOTOOLCHAIN=local`；没有修改依赖。

| 检查 | 本轮结果 |
| --- | --- |
| 新增回归：原实现失败、修复后通过 | 已确认上述暂停、计时和凭据名称行为；真实过期对照继续通过 |
| `make quality-gate` | 最终修复后重新执行通过；vet、格式、全包测试及每个被测函数 100% statement coverage，原单行 cmd 入口例外不变 |
| 受影响调度、预热与 JEV 凭据路径 `-race` | 通过；最后的预热分类／清理修复再次跑完整 `TestDSHPrewarm*` race，21 个函数均 100% |
| `npm test` | 142 通过，0 失败、0 跳过 |
| `npm --prefix plugins/dsh run check` | 主套件 274 通过、旧宿主 9 通过，均 0 失败、0 跳过 |
| 三个公开 adapter 的 Go → Node `-race` bridge | DSH、Codex Pro、Claude Code Pro 均通过，0 跳过 |
| 最终 native build 与 session evaluation | 构建通过；36／36 场景、96 次实际离线 adapter 调用全部通过，0 未观察、0 unknown outcome |
| 文档链接、锚点、围栏及 diff | 8 个变更 Markdown、173 个相对链接、42 个锚点和 42 个围栏通过；`git diff --check` 通过 |

定向验证使用以下命令；完整门禁仍单独覆盖全部 Go 包及逐函数覆盖率：

```bash
go test -race ./internal/drivers ./internal/machine \
  ./internal/investigation ./internal/contract ./internal/config ./internal/state \
  -run '^(TestDSHPrewarm|TestReadyQueue|TestParallel|TestFullScheduler|TestDeclaredWriteScope|TestConfiguredJEVKeyNames|TestPreparationJEVKeyNames)' \
  -count=1
FLYWHEEL_TEST_NODE_BRIDGE=1 go test -race ./internal/drivers \
  -run '^Test(DSH|CodexPro|ClaudeCodePro)NodeWorkerOfflineBridge$' -count=1 -v
make build
FLYWHEEL_EVAL_BINARY="$PWD/flywheel" npm run test:sessions
```

上述定向 race 的名称过滤器没有选中 config／state 包中的测试，因此不将那两包
描述为定向 race 通过；它们的实际完整测试由 quality gate 覆盖。最后新增的
有界收尾回归另以 `go test -race ./internal/drivers -run '^TestDSHPrewarm' -count=1`
执行，覆盖率单独核对。开发中的较早完整门禁不代替最终修复后的重新执行。

插件依赖沿用基线锁定的环境；本轮逐文件核对 Codex Pro 的 79 个文件和 Claude
Code Pro 的 45 个文件，与实际 tarball 完全相同。可选 Claude peer 使用已审阅
源码生成的 0.4.0 tarball（不宣称本轮从 registry 安装），其 SHA-256 为
`125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`；
Codex Pro 0.9.0 的仓库 vendor tarball SHA-256 为
`4a86380fdde0d7f2673c4b1f2d5c8682cf61d6a4b83deb65eab8fe76f6032198`。

本轮未重建六包发行候选；前次候选安装结果不能代表此次源码的发行验收。
离线 adapter 和进程测试不等同真实模型调用或桌面验收。

## 远端检查与交付边界

基线 [CI run 43386131427](https://github.com/ckanner/flywheel/actions/runs/43386131427)
仍为 failure：`npm-candidate` 的 `runner_id=0`、`steps=[]`，下游 test skipped，
日志下载返回 404。check 显示有两条 annotation，但当前连接的 endpoint 允许范围
不能读取其内容；现有证据不足以确认具体平台原因。

后续提交的远端 CI 状态单独记录在 PR；本地通过不把远端检查变成通过。
本次提交不合并 PR、不打 tag、不发布 npm，也不升级用户安装。
