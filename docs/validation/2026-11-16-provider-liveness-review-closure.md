# Provider 活性与会话名称：交叉复审闭环（2026-11-16）

[验证索引](README.md) · [原始 F1–F8 审查](../reviews/2026-11-16-provider-liveness.md) · [首轮实现验收](2026-11-16-provider-liveness-and-titles.md) · [PR #53](https://github.com/ckanner/flywheel/pull/53)

## 范围与源码身份

本轮先同步 PR #53 的 `186251d`，确认其中 `e58e106` 已实现跨 Provider 活动转发、
超时首因、DSH 恢复事实与 Session 命名的主要修复，再对真实调用链、打包依赖和
旧 review thread 做交叉复审。下文记录本轮继续发现、复现并补正的问题；不改写
原始审查和首轮候选的历史身份。

最终受验源码为 [`fabd8eba00fd785d5366d6b15642378ae1628651`](https://github.com/ckanner/flywheel/commit/fabd8eba00fd785d5366d6b15642378ae1628651)，
tree `aceeb2193b0834bb4d8a9fb807f4acb4b2f9f972`。生产补修在其父提交
[`4f12a45ecae575ff5db1641c7678ba3c5ed98400`](https://github.com/ckanner/flywheel/commit/4f12a45ecae575ff5db1641c7678ba3c5ed98400)，
最终提交只补完整 before 下的 no-op release 回归。远端 tree 与本地受验 tree
精确相同；完整 npm 候选直接从该干净远端提交隔离构建。本记录及 evidence 的
后续提交仅记录验收，不改变运行时代码。

环境：Linux、Go 1.27.4、Node 24.22.1、npm 11.9.0。Go driver/watchdog、Node
worker、实际安装的公开插件 API、supervisor/guardian 和 DSH SDK 均使用真实实现；
模型、原生 CLI、HTTP/WS 对端由离线 fixture 提供。没有调用真实模型账号或修改
用户桌面 profile。

### 精确 companion 候选

| 包 | 远端受验提交 / tree | 实际 tarball SHA-256 |
| --- | --- | --- |
| Codex Pro 0.10.1 | [49099acd4944](https://github.com/ckanner/dsh-subagent-codex-pro/commit/49099acd49447ddf556b0e49f4d8e87b187d90c2) / `ff08276190fa4b5ca40332aaebd0105c8eca48a9` | `d760e599cf19af54a93d77e1e2e4249e416b252467fe6d57d2174854507a6f6e` |
| Claude Code Pro 0.5.2 | [34291ff600bb](https://github.com/ckanner/dsh-subagent-claude-code-pro/commit/34291ff600bb70fa7dd628435fa10046c11b21d1) / `ff056d7052706101543c50a9eea701c701271d7d` | `d9f5d764d14a6cd1ead1563c6bd461193132ee1bba04675536ee9d8d2c22884a` |

配套 PR 为 [Codex #8](https://github.com/ckanner/dsh-subagent-codex-pro/pull/8) 和
[Claude #8](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/8)。实际 SHA-512
integrity、源码身份和打包工具链在 [vendor 记录](../../plugins/dsh/vendor/README.md)。
两份 lock 和实际 archive 已同步；`npm ci --offline` 后逐文件核对，Codex 的 110 个、
Claude 的 57 个归档文件与安装内容完全相同。版本号相同不代替字节身份。

两者仍是未在本轮发布的源码候选。Codex 随 Flywheel 内置；Claude 保持 optional
peer，正式发行的既有 registry 精确字节 gate 没有放宽。

## 继续发现并修复的问题

### 1. Codex：旧父轮次与错误 sender 可以认领子线程

旧 parent turn 的 collaboration 事件，或 foreign thread 伪装的 sender，可以把
不属于当前调用的 child 纳入活动。现在认领必须同时匹配当前父线程、当前 turn 和
实际 sender，仍允许真实的 provisional turn 响应时序。通过公开 runMachine、
supervisor 和 app-server fixture 的两个反例先失败后通过。

对另一个人为构造的 `thread/started` 历史子线程疑点，已核查上游固定版本与当前
生产发送点，未找到当前部署路径可达的反例，因此没有把任意 fixture 注入当作已
确认的生产缺陷扩展修改。

### 2. Codex：保留的 native/usage 证据也必须纳入容量预算

不仅最终文本需要有界。现在只保留归属、状态和数值 usage 的必要投影，不缓存完整
tool/thread message；native 和 usage 证据计入 retained-output 预算。默认 native
工具保留真实归属观察，不生成未请求的 native 回执账本；新 child 确认后释放用于
关联的旧观察。逐行传输使用单个有界、按需增长的缓冲，避免按碎片积累 Buffer。

单帧、真正保留的数据和最终答案仍受限；累计经过但未保留的 delta 不再耗尽预算。
启动阶段的保留证据超限可以明确证明未派发并确认本地清理，不伪造成功。

### 3. Claude：ambient、运行轮次与远端所有权要分别记录

ambient watcher 的后续 progress 可能不再带 ambient 字段。新的有界 task/run
分类同时服务活动和 native 账本，避免把监听任务当作工作进展或永不结束的 child。
同一个 task 重跑时，旧 run、legacy 无 run_id 状态和新 run 不能相互覆盖。

最后一轮复审又补正首次晚到旧 start/snapshot 的情况：同一 task 的 current 只按
run_id 的普通字符串时序前进；实时无 run_id 事件使用 current，已保存 child 使用
exactRun。依据是 [Claude 官方 task 事件契约](https://code.claude.com/docs/en/agent-sdk/typescript#sdktaskstartedmessage)：
run_id 可用于判断同一任务的启动先后，而后台快照与单条任务事件通常不保证到达
顺序。五个新反例先全部失败、修复后通过；违反该契约的旧测试 ID 时序也已纠正。

身份数量、身份字节或 child ownership 账本超限时，如果新出现的 remote child
未能入账，不能因为列表中“没有远端任务”就确认 native cleanup。现在保留独立的
本地进程组 cleanup，远端清理保持未确认。仅 event-history 截断、但归属仍完整时，
不会无故抹掉可信清理事实。

另外，调用方已指定新建／恢复 session ID 时，匹配的 pre-init hook 可以提供活动；
没有明确身份的 ephemeral 初始化阶段仍不猜测归属。timeout/cancel 的首因在进入
清理前冻结，后到的停止信号不会覆盖它。

### 4. Go：不可信传输与合法的 worker 错误报告要区别处理

DSH fresh 和 persistent 路径现在都要求完整身份匹配的回执、正常物理退出、未截断
传输和成功写入请求，才继承 provider cleanup。Go 自己观察到的 timeout/cancel
独立保留；不可信回执不能提供一个新的 provider timeout 首因。

Pro 回执先核对内层 run/thread/ownership，再继承停止事实；错 adapter、坏尾部、
截断、ACK 失败或缺少必需 durable lifecycle，不能让“看起来完整”的前缀充当证明。

交叉复审发现过度保守也会丢事实：worker 对无效活动、native metadata 或观察配额
报错后，会取消同一次调用并等待完整清理回执，最终发送的 JSONL 本身仍可合法。
因此仅对明确的 `INVALID_ACTIVITY`、`INVALID_NATIVE_EVENT`、`OUTPUT_LIMIT`
报告保留已独立核验的身份、usage 与 cleanup；业务 outcome 仍为 unknown，不能
得到 resume checkpoint，清理取消也不能覆盖 worker 首因。未知 worker 错误继续
保守处理。缺 durable lifecycle 的成功回执即使包上一层观测 error，也不能绕过
证据校验。正向保留与损坏对照都经过回归验证。

### 5. Kimi native：写入标题还必须验证标题与归属

原先创建请求带了 title，但没有确认服务器实际接受它。现在准入 schema 必须支持
title、metadata 与环境字段；新历史返回的 ID、binding、cwd 和 title 必须匹配，
才发送业务 prompt。先确认归属，再允许会话级清理；错 owner 的历史不会收到
targeted abort/profile/prompt，自己启动的 runtime 仍会停止。

缺 title schema、忽略 title、返回错误 owner 的三个反例先失败后通过。已有会话
resume 不会重新命名。

### 6. Kimi native：重复快照仍会触发累计 64 MiB 限制

同一 agent 状态被反复替换时，实际只保留一份，旧计数仍会累加所有传输字节并终止
健康调用。现在按当前保留的 agent/task/pending/turn JSON payload 计量；替换与
删除释放旧配额，WS 与最终 HTTP mergeTurn 共用预算，后者不能绕过上限。

delta 只量新增片段，通过缓存更新 item/array/turn 尺寸；不反复序列化累计答案，
也正确处理跨片段的 Unicode surrogate pair。超限先检查，再改投影；拒绝增长时
保留旧值。九个回归先全部失败，修复后通过，包括超过 64 MiB 的重复状态、真正
保留超限、回收、HTTP 绕过、增量复杂度和 Unicode 边界。原有数量、单帧、HTTP
响应和最终答案上限保留。

### 7. Session control：缺 before 不能把旧终态当作本次成功

旧 review thread 的正常 before/after 转换已修，但首次 snapshot 失败、后续读取
恢复时，已有 consumed 或 claimed 状态仍会误报 applied。公共 startSessionControl
入口的真实子进程 fixture 已复现：命令没有改变状态，旧实现却报告本次成功。

现在缺少目标 key 的有效 before record/Last/invocation ID 时，保留 after 的
revision/status/ID，返回 applied:false，并明确说明效果无法验证、命令可能已经
执行，应先刷新状态。命令执行流程保持原样，不能把这个 false 当作未执行证明。
正常 send/return/claim/release，以及已 consumed 但 pending handoff 刚清除的
reconcile 仍按真实转换判断。

新增六种操作乘五类缺证据用例、公共入口回归，以及完整 before 下 no-op release
对照。最后一项补足严格逐函数覆盖门禁，而不是放宽门禁。独立复审通过后，已回复
并关闭 [原 review thread](https://github.com/ckanner/flywheel/pull/53#discussion_r4583288641)。

### 8. 已处理远端 macOS 的测试可移植性失败

远端 `186251d` 的 macOS job 在“删除当前目录后 filepath.Abs 必须失败”这一测试
假设处失败。Linux 普通目录和模拟 `/var`/`/private/var` 别名均不能复现生产路径
故障，实际 Node 准备仍会检查目录，因此没有借此改变生产路径语义。

按仓库已有模式注入最窄的 Abs 错误边界，保留真实相对路径／符号链接成功和不存在
目录拒绝测试，fixture 写入失败也必须退出，避免伪成功。默认 Abs 行为不变；新
macOS 执行结果须以新 head 的远端 CI 为准。

## 名称行为与原始 F1–F8 的闭环

可读名来自 task title、规划 goal 或 slug，结合 role/scope，例如
`Flywheel · design-plan · 排查告警重复发送`。DSH 使用公开 title 服务并回读，
Codex 用 thread/name/set，Claude 用受支持的 CLI naming。Kimi print 绕过 TUI
自动命名且没有 title flag，因此先通过公开本地 API 创建具名空历史，确认关闭后
原 print 命令续用该 ID；不发送额外业务 prompt 或命名模型请求。首次准备多一次
SessionStart/SessionEnd，包括用户 hooks；恢复不重命名，Agent/Swarm 不多加准备。
详细上游依据与成本见 [首轮名称说明](2026-11-16-provider-liveness-and-titles.md#session-名称)。

F1–F4 的 root/tool/owned-child 活动经实际 SDK/插件/worker 送到 Go；F5 的容量
边界覆盖保留状态；F6 保留独立的可信恢复事实；F7 区分停止首因与清理取消；F8 的
旧 Codex exec 与 Kimi print 保持有限 wall-only。native 和 PTC 的活动桥接均受
同一真实归属约束。没有给所有 Provider 换一个更大的统一 idle，也没有生成假心跳。

## 已执行验证

| 层 | 最终结果 |
| --- | --- |
| Codex standalone | typecheck；250 tests 中245 pass、5个原有live/account skip；实际pack/install public API/CLI/supervisor/guardian/recovery通过 |
| Claude standalone | 174/174 tests；实际pack/install public machine smoke；8个schema/PTC host组合通过 |
| Flywheel Node | 485/485主JS（含4个嵌套subtests）+10/10 legacy host；均0 fail/skip/cancel |
| 实际 Go→Node→公开插件 | 8个顶层测试+93个subtests，共101条PASS；0 FAIL、0 SKIP；103.988s |
| Go完整门禁 | 最终源码 `make quality-gate` 通过，vet/fmt/全包测试及每个受检函数100% |
| Race | watchdog 6.789s、drivers142.064s通过，无race；此后两包源码未变 |
| 根npm | 166/166通过，0失败、0跳过 |
| 完整候选构建 | 干净远端提交隔离snapshot、精确依赖、四平台native与完整单包通过 |
| 实际候选安装 | packed Host/Client 10/10，安装/迁移15/15；0 skip，实际安装并执行optional Claude候选 |
| 精确候选引擎场景评估 | tarball实际引擎36/36质量通过、96次调用、六场景各6；40个新持久会话prompt前命名，10次resume不改名；0提前停止 |

### 完整候选身份

- source：`fabd8eba00fd785d5366d6b15642378ae1628651`，tree `aceeb2193b0834bb4d8a9fb807f4acb4b2f9f972`，dirty:false。
- source snapshot SHA-256：`e350f21e252589ceb0eebdb3bc66d87e103e2eb1e40921abf23edc19434d1450`。
- archive：`ckanner-flywheel-0.35.0.tgz`，20,022,042 bytes。
- archive SHA-256：`d4df9130583c908233501018a57e9e83d51c915b1ef0ea8f3c9d944dd363c98b`。
- integrity：`sha512-HQ1W6AxA3S0pIIhvauC5Q3/UqFqiFPQB298NbHpIFQdB7lVs7b/DREFDPTMiR/fQt5KFfsaOTzICwlzoXlKCOA==`。
- 实际候选 Linux amd64 engine SHA-256：`d44ee50732c126f664a8724cd8e2eac3dc98bd309d9e21175e6454895e228101`，与包内 native manifest 一致。
- 机器可读记录：[review closure evidence](evidence/2026-11-16-provider-liveness-review-closure.json)。

仍沿用开发分支0.35.0构建未发布候选，没有覆盖任何 registry 包。一次构建因验证
过程中新增测试改变源码快照而被构建器正确拒绝；上述最终候选从冻结后的干净提交
重新构建，不使用被拒绝的产物。

## 远端 CI 与验收边界

本地全部通过不等于远端全部通过。远端矩阵结果在 PR 的最新复审评论中按提交记录；
尚未启动、排队或执行前取消的任务不记为通过。记录时的精确结果：

- Codex [run 43894473187](https://github.com/ckanner/dsh-subagent-codex-pro/actions/runs/43894473187)：Linux/macOS × Node22/24 四组全部通过；曾在执行前取消的 macOS Node24 已重跑通过。
- Claude [run 43896639421](https://github.com/ckanner/dsh-subagent-claude-code-pro/actions/runs/43896639421)：Linux/macOS × Node22/24 四组全部通过。
- Flywheel 源码 `fabd8eba` 的 [run 43897783989](https://github.com/ckanner/flywheel/actions/runs/43897783989)：完整候选构建通过，Linux 进行中、macOS 排队。后续验收文档 head 需要自己的检查，不能把该 run 冒充最终 head 的全绿证明。

本轮未合并 PR、未发布 npm、未进行真实模型／账号、桌面 UI 或用户机器验收。
旧历史名称与原始告警任务没有批量修改。未提供已验证 title API 的旧兼容后端不
冒称具备原生重命名能力。

activity 只说明观测到进展。首 token 前、队列等待和无中间消息的远端工具，仍可能
消耗适用的 idle 策略预算；这不是“模型已挂死”的证明，也不授权自动重放。unknown
dispatch/outcome、未确认的清理仍受原有阻断与人工退役规则约束。真实模型性能、
最佳超时值与模型稳定性排名不由这些离线测试推出。
