# Codex 后续复审：子线程状态与命名准入修复

日期：2026-11-16。配套[机器可读证据](evidence/2026-11-16-codex-review-fixes.json)保存实际候选、依赖、验收计数和远端 CI 快照。

本记录针对三仓后续 review 确认的两项问题：迟到的 Codex 创建快照覆盖真实 child turn 状态，以及 machine 持久会话命名失败后仍发送任务。此前的跨 Provider 活性、容量、恢复与 Kimi 命名实现保留在[上一轮验证记录](2026-11-16-provider-liveness-review-closure.md)中；其历史归档和测试结果不改写为本次结果。

## 1. 来源与交付范围

| 仓库 | 本次复审基线 | 本次生产修复／依赖提交 |
| --- | --- | --- |
| Codex Pro | `71f67c469336cde65745d5253a2dabb4c2afd197` | [f31659536ed0](https://github.com/ckanner/dsh-subagent-codex-pro/commit/f31659536ed085f86c31f28e56a48ebed9d6b85b) |
| Flywheel | `294b6db985d9da62f2f33a58307a29bb90919022` | [01d3244bc809](https://github.com/ckanner/flywheel/commit/01d3244bc809620506e0611f7f46c74c67c700eb) |
| Claude Code Pro | `2b41b87a62d77dc9404a6f10773c086ba01a53ee` | 无新增生产修改；精确旧候选继续参与 worker、bridge 和安装回归 |

Codex 的源码树为 `5135f6d4b00be228e2f8a4c211f6f081406e993e`。Flywheel 候选从干净的 `01d3244`、源码树 `93e2afe124a9ba0c8eee68c95a13549a821f1241` 构建。GitHub 提交的 tree 与本地检查的 tree 精确相同；分支更新保留已有提交，没有强推。

本记录及索引由后续文档提交保存，没有重新构建并替换下面已验收的候选字节。Flywheel 本次修改依赖归档、锁文件、回归和中英文说明，没有新增 Go 生产逻辑。

## 2. 创建快照不能覆盖已观察到的 child turn

原实现无条件把父线程 `spawnAgent` 的 `agentsStates` 写入 child 状态。子线程 listener 与父线程 listener 独立，child 的开始／完成可能先到；建立父子归属时重放这些事件后，较旧的 running 创建快照又把 completed 覆盖了。反方向也存在：child 已进入第二轮，迟到的 completed 创建快照会把仍在运行的第二轮判成完成。

[修复后的 native tracker](https://github.com/ckanner/dsh-subagent-codex-pro/blob/f31659536ed085f86c31f28e56a48ebed9d6b85b/src/native.ts)只在尚无 child turn 身份、且 child 未关闭时，允许创建快照设置状态。规则同时适用于 `spawnAgent` 和 V2 `subAgentActivity: started`。固定上游 [Codex V2 spawn 源码](https://github.com/openai/codex/blob/360319d8dee62d87e2d59fc0c21805d79519cfed/codex-rs/core/src/tools/handlers/multi_agents_v2/spawn.rs)确认，后者在初始 spawn 完成后发出，属于创建标记。

| 真实事件顺序 | 修复后的验收结果 |
| --- | --- |
| 父创建先到，child 随后完成 | 正常成功 |
| child 完成后，旧 running 创建快照才到 | 保留 completed，正常成功 |
| child 第二轮已开始，旧 completed 创建快照才到 | 保留第二轮 running，completion 未确认，不能成功 |
| 第二轮随后真正完成 | 允许成功，按实际 turn 归属 usage |
| V2 child 先完成，父 started 创建标记后到 | 保留完成事实，正常成功 |

状态优先级没有永久锁定 child：真正的新 `turn/started` 仍启动新工作；后续 `sendInput`／`followupTask` 仍可在新 turn 到达前表示排队工作。failed、interrupted、closed、默认 native ownership、迟到旧 activity 和 usage 重放也有控制测试。ownership、保留字节预算和事件数量上限沿用既有检查。

对应[19 项专项回归](https://github.com/ckanner/dsh-subagent-codex-pro/blob/f31659536ed085f86c31f28e56a48ebed9d6b85b/tests/native-snapshot-order.spec.ts)包含真实 public machine → supervisor → guardian → app-server fixture 的正反顺序、独立清理和持久恢复。该创建快照缺陷在此前实现中已存在，不能归因于后来的名称长度测试提交。

## 3. 新持久 machine 会话必须确认命名

原 wire 的 `setThreadName` 在普通 RPC 失败时返回 false。machine 调用虽然提供了可读名称，run 层却忽略了这个返回值，最终仍发送业务 prompt 并报告成功。

[机器入口](https://github.com/ckanner/dsh-subagent-codex-pro/blob/f31659536ed085f86c31f28e56a48ebed9d6b85b/src/machine-local.ts)现在启用内部命名约束。[run 层](https://github.com/ckanner/dsh-subagent-codex-pro/blob/f31659536ed085f86c31f28e56a48ebed9d6b85b/src/run.ts)必须先确认写入，才发布 `thread-ready` 并发送任务；显式标题与已有工作目录备用标题都适用。内部调用只设置约束、未传显式标题时，也会使用备用标题并执行同样的准入检查。

本轮命名 RPC 拒绝、且物理清理已独立确认的回归用例，关键回执事实为：

- `ok:false`、`outcome:null`，失败阶段／类别为 `thread-name/protocol`。
- 已观察到的 thread ID 保留，turn ID 为 null，`execution.dispatch` 为 `not-dispatched`。
- 已核验的 `cleanup.confirmed:true` 保留，不伪造 cancelled 或 timedOut。
- 生命周期只有 `terminal` 和 `cleanup-confirmed`；只读恢复返回同一失败，不重新发命名或任务请求。

清理未确认时，仍保留真实的未确认状态，不发出 `cleanup-confirmed`，也不会
把已创建 thread 或收到命名 RPC 回复作为物理退出证据。

诊断使用 “name was not confirmed”，同时涵盖 RPC 拒绝和响应丢失，不能把未确认写入等同于已证明从未写入。真正的取消和总期限仍走原有 aborted／timeout 分类。已存在空 thread 也不证明它已可续接。

resume 保留原名称，ephemeral 不因命名变为 persistent；历史 desktop 直接调用继续使用既有 best-effort 约定。名称仍来自任务／工作区信息，没有追加模型生成请求。对应[8 项准入回归](https://github.com/ckanner/dsh-subagent-codex-pro/blob/f31659536ed085f86c31f28e56a48ebed9d6b85b/tests/thread-name-admission.spec.ts)覆盖拒绝、成功顺序、恢复、取消、resume、ephemeral、旧调用兼容和内部 fallback。

## 4. 先复现，再验证修复

旧安装包的真实跨进程 RED 结果：

| 路径 | 用例 | 正常控制通过 | 对应缺陷失败 |
| --- | ---: | ---: | ---: |
| Node worker → 安装后的 Codex adapter | 6 | 2 | 4 |
| Go driver → worker → 安装后的 adapter | 5 | 2 | 3 |

这里只替换 app-server 对端。Go driver/watchdog、worker、实际 npm adapter、supervisor 与 guardian 均执行生产代码。修复后这些用例全部通过；新命名失败的 Go 回归进一步验证原始 receipt、session 身份、未派发状态、独立清理、无自动替代进程，以及 read-only recovery 的一致性。

实现者与独立 reviewer 交叉检查了两个修复、失败分类、状态优先级和打包测试。另一次独立检查核对 Flywheel 的回执与依赖边界。复审没有发现新的阻断项；完成门禁、归属检查和失败证据没有被削弱。

## 5. 本次实际运行的验证

工具链：Go 1.27.4、Node 24.22.1、npm 11.9.0，Linux 本地环境。

| 验证 | 实测结果 |
| --- | --- |
| Codex TypeScript typecheck | 通过 |
| Codex 全量测试 | 278 项：273 通过，5 项既有 live/account 条件跳过，0 失败 |
| Codex 实际 pack／隔离安装 | 公开 API、CLI、guardian、supervisor、activity、durable recovery，以及两项新缺陷的安装包回归通过 |
| Flywheel `make quality-gate` | vet、fmt 和全部 Go 包通过；总覆盖率及每个受测函数均 100% |
| DSH `npm --prefix plugins/dsh run check` | 491 项行为测试 + 10 项宿主兼容测试，0 失败／跳过 |
| AGENTS 指定的完整 Go → Node bridges | 8 组顶层测试 + 98 个子测试，0 失败／跳过 |
| 根目录 `npm test` | 166／166 通过 |
| 完整 `npm run build:npm` | 干净源码快照，四平台 native binary，manifest 大小及 hash 全部匹配 |
| 精确候选 `npm run test:install` | 10 项 packed-host + 15 项安装／迁移测试通过，0 跳过；实际安装并执行可选 Claude 归档 |
| 同一安装候选的会话评估 | 36／36 配对试验与独立 oracle 通过，96 次真实离线 adapter 调用 |

本次会话评估同时使用候选包安装后的 engine 和公开 `bin/flywheel-codex-pro-worker.js`。保留原 prepare/evaluate 代码与全部断言，只将 harness 的源码 worker 复制块替换为已安装 worker 路径。候选中和安装后的 Codex 110 个文件分别逐一匹配 vendor tarball。隔离 npm cache 缺少 zod 元数据时，使用现有真实 zod 4.3.6 的本地 tarball 完成离线解析，未改变候选字节。

六类场景各 6 条，fresh／executor-scoped／role-scoped 各 12 条。原生日志及最终历史确认：40 个 fresh persistent 会话都在 prompt 前命名，10 次 resume 都没有重命名并保留原名称，ephemeral 命名次数为 0。示例名称为 `Flywheel · execute/M01-T01 · Immutable integer interval compaction (first-pass)`。

## 6. 精确归档

| 归档 | 来源 | 大小 | SHA-256 |
| --- | --- | ---: | --- |
| `dsh-subagent-codex-pro-0.10.1.tgz` | `f316595` / tree `5135f6d4` | 221,516 bytes，110 files | `73a156d24e739e45a35419c3fa6cf6f2ed2ceb12cda2f76c798e75fc2a093473` |
| `ckanner-flywheel-0.35.0.tgz` | `01d3244` / tree `93e2afe1`，clean | 20,031,297 bytes | `884880a06a39aef143cb4f4943955e83b82313c7620935e692ac71376a4ecef0` |

Claude 0.5.2 归档保持原身份，SHA-256 为 `d9f5d764d14a6cd1ead1563c6bd461193132ee1bba04675536ee9d8d2c22884a`，57 个安装文件与归档逐一相等。[vendor 记录](https://github.com/ckanner/flywheel/blob/01d3244bc809620506e0611f7f46c74c67c700eb/plugins/dsh/vendor/README.md)和两份锁文件指向实际验收字节；同一个源码候选版本号不代表 registry 已发布。

## 7. CI 与验证边界

Codex `f316595` 的 [Linux/macOS × Node 22/24 CI](https://github.com/ckanner/dsh-subagent-codex-pro/actions/runs/43913351959)全部通过。未改动的 Claude `2b41b87` 的[同矩阵 CI](https://github.com/ckanner/dsh-subagent-claude-code-pro/actions/runs/43902455228)也通过。Flywheel `01d3244` 的[源码候选 CI](https://github.com/ckanner/flywheel/actions/runs/43914039570)快照及后续文档 head 的区别在配套 JSON 中明确记录；最终 PR head 的状态以该 head 的 Actions 为准。

本次没有重新执行本地 Go race，Go 生产源码未变；远端 CI 继续执行其既有 race 门禁。离线协议与实际安装验收不等于真实模型、账号或桌面 UI 验收。5 项 Codex live 测试的跳过没有计作通过。本次交付是两个现有 PR 的修复与验证记录，未合并或发布；已有用户会话名称、用户 profile 和原未知调用没有被修改，unknown outcome 仍不能自动重放。
