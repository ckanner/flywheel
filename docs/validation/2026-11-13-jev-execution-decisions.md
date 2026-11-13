# JEV 执行决策与诊断扩展：工程验证

[验证索引](README.md) · [本轮方案](../plans/2026-11-13-jev-execution-decisions.md) · [JEV 使用指南](../zh-CN/jev.md) · [调查指南](../zh-CN/investigation.md)

## 源码与范围

本轮接在 PR #37 的 `4ea69cc54176d7afa36d235e6b7dc8728005a671` 后；PR 的 main
基线仍为 `e035b5b5736c2d26da7ada2bf1ca97ee020fd5a2`。本记录与所验证的实现一同
提交，精确源码身份以承载本记录的 Git commit 为准。源码包版本仍为 0.31.1，
没有修改六包版本、Provider 默认绑定、依赖锁、vendor 或用户实际配置。

用户明确要求先补完剩余工程，真实账号、模型效果与桌面验收后置。因此这里记录
离线工程验证；不把本地协议 fixture 的正确执行称为真实 JEV 正确率或速度收益。

本轮补齐：

| 能力 | 实际实现 |
| --- | --- |
| 有界复现与实验 | `run_command` 在批准输入的私有副本中执行精确 argv，保存物理结果、输入身份及清理状态 |
| 外部查询 | `http_read` 执行批准的 HTTPS GET／HEAD，限制时长与输出，拒绝重定向与环境代理 |
| 失败后的策略选择 | 在合法 `repair`、`feedback-N`、`recheck-N` 和 `replan` 中实际采用 JEV 选择 |
| 模型／effort | `model-N` 只能选择已有 `execution-policy` 完整批准绑定，共用切换预算并冷启动 |
| 定向短检查 | 可选 `feedback-checks`，最多八项；用 `purpose` 提供语义用途，不向 JEV 发送原命令 |
| 恢复与并发 | 持久请求意图、任务共享上限、真实动作状态、Plan 回执和 checkpoint 恢复 |
| 桌面与使用入口 | 展示选择、实际效果、未知状态、实验结果和具体模型切换；保留 Task／New／Run 与 Auto／Quick／Full |

所有在线 JEV 路径沿用唯一 `jev.enabled`，没有恢复影子执行或增加动作族开关。
最终 L1／适用 L2/L3、独立 Review、权限、已有尝试与规划次数仍由 Go 检查。

## 环境与构建身份

- Linux x86_64，Go 1.27.4，Node 24.22.1，npm 11.9.0。
- DSH 测试依赖使用未修改的锁文件与 `0.2.2-rc.2`；旧宿主 fixture 为 `0.2.1-rc.5`。
- Codex Pro 继续使用原锁定的 `0.9.0` vendor 包；Claude Code Pro 使用已核对的
  `0.4.0` 源码 tarball，SHA256 为
  `125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`。
- `make build` 的本地 Linux 二进制 SHA256 为
  `9a0df53755ceef4db05af025c1bb3e7f433ca00d24b368e7c5987621a22cb65b`。
  它在提交前构建，用于下面的真实引擎场景；不是 npm 发行产物。

## 已执行检查

| 检查 | 结果 | 验证边界 |
| --- | --- | --- |
| `make quality-gate` | PASS | vet、gofmt、全部 Go 行为测试和逐函数 100% statement coverage；既有单行 cmd 入口例外不变 |
| investigation／contract／state／decision 的 race 测试 | PASS | 实际子进程、loopback HTTP、状态校验、journal、预算及恢复；最后日桶修复后另重跑 state 全包 race |
| `go test -race ./internal/machine -run 'TestRuntime' -count=1` | PASS | 五类实际执行分支、并行共享预算、取消、动态关闭、反馈与 Plan 恢复；最终冻结版本 18.51 秒 |
| `npm test` | 142 PASS，0 skipped | 启动器、安装器、发行门禁与协议回归 |
| `npm --prefix plugins/dsh run check` | 263 PASS + 9 旧宿主测试 PASS，0 skipped | 插件构建、配置、桌面渲染、控制、worker 与宿主兼容 |
| 三个 Go → Node offline bridge | PASS，0 skipped | DSH、Codex Pro、Claude Code Pro 的真实跨进程协议 |
| `make build` 与 `FLYWHEEL_EVAL_BINARY=… npm run test:sessions` | 36 场景 PASS | 六类场景、三种会话策略、两轮配对；96 次离线 adapter 调用全部有实际观察 |
| 当前文档的相对链接、锚点、围栏与 `git diff --check` | PASS | 双语指南、方案、技能及本记录的导航与格式 |

36 条场景覆盖 first-pass、L1 repair、Review return、long-context、stop-resume 与
configuration-change。全部 `oraclePass=true`；没有 unobserved／unknown 场景。
这些场景使用真实本地引擎和公开 adapter 的离线 fixture，不证明真实模型缓存、
质量或 token 成本改善。

插件中间一轮出现过一次既有控制 fixture 的 ready 文件等待超时：尚未进入进程
收尾断言。未修改代码门禁或扩大超时；同用例单独完整重跑通过，最终 263 项主
套件及 9 项旧宿主套件也全部通过。保留这次事实，不把首次失败隐去。

## 关键行为证据

### 调查确实执行命令和 HTTP

真实子进程测试核对复制输入、精确 argv、显式环境、执行权限位、源文件变动、
超时／取消／信号、输出截断与清理结果。未知清理保留工作目录和 pending，恢复
不重跑。输出含凭据时脱敏；截断边界切出的凭据前缀也不写入回执。

loopback HTTP 测试核对 GET／HEAD、精确地址、命名 header 环境变量、状态码、
响应截断和超时，验证不会跟随重定向或环境代理。错误地址、头、凭据和响应不能
伪造成功观察。执行结果绑定原批准动作，不能把另一动作的回执用于当前观察。

真实 Go 调查循环接本地 JEV 协议 fixture，分别选择 command／HTTP，再消费真实
结构化结果并进入 finish。默认不授予 `logs` 时仍可发送批准的状态、退出码和
HTTP 结果等结构化事实，原始输出不外发；失败、截断、预期非零退出等不授权
提前结束取证。已有调查回执恢复与报告门禁继续生效。

### 执行策略有实际效果

`TestRuntimeJEVActuallySelectsRepairFeedbackModelAndReplan` 在真实 Execute/L1
失败后逐一选择 repair、feedback、recheck、model 和 replan，核对后续实际行为。
短检查只提供下一轮反馈，完整验收仍独立存在。用途缺失的短检查留作 Executor
指导，不作为 JEV 定向选择候选；模型候选包含当前与目标的实际完整绑定。

`TestRuntimeQuickReplanUsesTheExistingOneWayFullUpgrade` 验证 Quick 的重规划走
原有单向 Full 升级。渠道错误与未结束的调用不会被改写成模型质量问题；新模型
不能绕过 unknown 会话。Review 空转的确定性基线不会误触发原本仅用于 L1 的
质量升级。

### 中断不能制造第二次执行

请求预留 CAS 冲突时，零 JEV HTTP、零请求 journal，磁盘上的其他写入保持不变。
请求已派发后未知只读恢复，不重复请求或记账；坏 journal 仍保留冻结的回执身份，
不会制造无法再次加载的零值回执。

实际短检查修改候选或验证输入时，保存 `feedback-stale` 并阻断。清理未知保存
`feedback-unknown`；终态保存失败保留原 `feedback-running`，恢复不重放。保存
错误和候选错误并发出现时，保留最先发生的持久化错误。

Plan 派发前保存 `replan-running`、决策原因与旧 invocation 边界。真实中断测试
验证：已确认的新 Plan 回执与 checkpoint 可复用，第二次 Plan 调用数为零；只有
旧 consumed 标志、回执缺失、候选变化或检查点不匹配时阻断，不猜测原结果。

### 总预算与关闭语义

两 packet 的真实并发测试验证共同调用上限，调查移交的已用调用也占同一额度。
模型切换、恢复和 packet 切换不能重置它；未派发的 skipped 不虚构付费调用，
记录数另有 100 条上限。已知／未知用量只核算一次，按原派发日归属；过期日桶
不会因迟到回执复活，累计总量仍保留。

配置关闭、目标变化、数据授权收紧、候选变化等使可选选择弃权。并行选择使用
原 worker 的取消域，并把模型切换后的会话退役记录同步回 worker，后续保存不会
重新启用旧历史。关闭 JEV 不会抹去未知动作、原回执或预算。

### 桌面与文档

渲染测试区分“建议被接受”“控制效果已应用”“动作已收尾”和“任务已完成”，
展示真实 before／after 模型绑定、原因、尝试次数与时间。pending、反馈未知和
重规划未收尾显示核对提示；坏快照保留原有效视图并禁止基于它再次启动。
投影只保留有限诊断摘要，不向画布暴露原始决策快照或目标配置。

Task 保留统一目标准备与启动入口，New 保留只准备后停下，Run 保留已有任务
启动／恢复；没有把循环、重试或模型选择搬入 skill。设计、schema、示例、双语
指南及技能随代码更新，历史验收记录不改写。

## 本轮没有证明的内容

真实 JEV／模型账号、实际决策质量、耗时／token／费用配对以及用户 Mac 桌面
操作，按用户要求留到后续。PR 的远端 CI 状态独立于上述本地检查，以新提交的
GitHub Checks 为准；没有把本次源码提交称为合并、npm 发布或用户机器升级。

`run_command` 的 `host-process` 只是批准输入的私有副本及受管进程，不是 OS
沙箱，不能阻止主机用户能力内的其他路径或网络访问。需要强隔离的任务必须批准
真实容器／沙箱入口。没有配置额外允许绑定时，JEV 继续使用原模型，不能生成
新的账号、权限或付费渠道。

原方案 P5 的 ready queue、预热和进一步流水化继续以实际瓶颈与收益为前提。
它们不属于本轮以缺省配置强行加入的功能，后续效果验收再决定是否实施相应优化。
