# 会话质量策略与配对评估

[文档目录](../README.zh-CN.md) · [English](../en/session-evaluation.md) · 简体中文

Flywheel 支持按已确认事实进行有界上下文重建，以及对相同任务运行三种历史策略的配对评估。重建阈值默认关闭；评估程序保存原始条件和结果，不把缺失用量写成零，也不预设质量、缓存率或速度收益。

配置入口见 [配置文档](configuration.md)，中断恢复的生命周期和账本语义见 [恢复文档](session-recovery.md)。实现入口为 [质量策略](https://github.com/ckanner/flywheel/blob/main/internal/machine/session_quality.go)、[评估器](https://github.com/ckanner/flywheel/blob/main/scripts/evaluate-sessions.js) 与 [任务集生成器](https://github.com/ckanner/flywheel/blob/main/scripts/prepare-session-evaluation.js)。

## 有界重建

`sessions.roles` 可分别指定 `plan`、`execute`、`review`、`judge` 和 `judge-panel` 的 `fresh` / `scoped`，未配置的角色继承 `sessions.mode`。Plan 的显式 `resident-session:false` 仍然禁止续用；显式 `sessions.roles.plan:"fresh"` 也关闭旧 Kimi Plan 的续用入口。

`sessions.quality` 支持以下阈值；`0` 表示关闭，对既有默认行为没有改变。

| 配置 | 触发依据 | 能证明的内容 |
|---|---|---|
| `max-confirmed-turns` | 本代已确认、安全归属的 turn 数达到阈值 | 对话被重用的次数；不等同于质量已经下降 |
| `max-unproductive-turns` | Execute 连续完成后，候选成果 fingerprint 没有变化 | 磁盘候选没有进展；不推断模型思考过程 |
| `max-run-input-tokens` | 最近一次完整、可归属的单次运行 input 达到阈值 | 该次输入用量；不等同于原生历史占用或剩余上下文容量 |

只有上一调用已经消费、记账状态已保存、派发与终态确定、清理已确认、候选成果可核对时，才按这些阈值重建。未知派发/终态、未完成的生命周期和未确认清理先进入 P3 的恢复或现有升级路径。partial/unknown usage 不能触发输入用量阈值；但已确认 turn 数和候选无进展仍可独立触发各自阈值。未知用量继续保留，原有 hard budget 可另行阻断后续调用。重复消费同一 invocation 不重复增加计数。

重建记录原因、代数、被替代调用引用，并通过当前 Spec、packet、磁盘成果、合法报告、验证证据和结构化反馈交接。Executor 的私有历史不会复制给 Reviewer。新代重置本代质量计数，但任务 attempts、失败、耗时及预算账本继续保留。这里使用可验证的重建，不调用额外模型做未计量的历史压缩。

## 三种固定策略

评估器使用同一固定 Git commit、任务契约、模型、effort、权限、验收与质量阈值。它只改变 Execute / Review 历史策略。

| 报告中的 variant | Execute | Review | Plan / Judge / judge-panel |
|---|---|---|---|
| `fresh` | 每次 fresh | 每次 fresh | 显式 fresh |
| `executor-scoped` | 同 packet 范围 scoped | 每次 fresh | 显式 fresh |
| `role-scoped` | 同 packet 范围 scoped | Reviewer 自己的评审范围 scoped | 显式 fresh |

`role-scoped` 是设计中的“Executor 和 Reviewer 各自在自己的范围续用”，不表示所有角色共享一个 thread。每个 case 至少运行两次配对重复，每一组含三种策略；运行顺序按 case 和重复编号轮换。每个 trial 使用新的隔离 Git clone，不修改原始仓库，不把前一个 trial 的工作树或历史交给下一个 trial。

## 准备可执行任务集

先准备完整的 Flywheel 配置，显式指定 `routes.default` 和 `loops.review`，以及它们引用的渠道。使用已经能正常运行的账号与后端配置。生成器只读取这份配置，不运行模型，也不读取或复制账号凭据。

```sh
node scripts/prepare-session-evaluation.js \
  --config /absolute/path/flywheel.config.json \
  --output /absolute/path/new-session-corpus \
  --repetitions 2
```

生成器输出新的本地 Git 仓库、精确 commit 和 `manifest.json`。它为所有 trial 统一关闭额外 worktree 创建，因为评估器本身已经逐 trial 隔离；并把冻结检查 `eval/` 加入保护范围。原配置文件不被覆盖。

生成的配置变化场景使用 `medium` / `high`。所选模型必须接受这两个值；不同 effort 枚举的后端应在运行前把 manifest 的 `configure` 步骤改为该后端明确支持的另一个值。三种策略使用同一份修改后的 manifest，不在某一个 trial 中临时猜测或替换。

内置任务是一个可独立验收的真实代码修改：实现纯函数 `compactRanges`，把安全整数数组变成排序后的最大闭区间，去重、支持负数和空数组，并保持输入数组不变。冻结 L1 检查输出，最终 Review 独立检查输入不变性；进程外的固定 oracle 同时检查两者。模型自己的 done-report 不是评估评分依据。

| 场景 | 实验步骤 | 观测条件 |
|---|---|---|
| `first-pass` | 从未实现代码开始正常执行 | 完成任务，且一次 Execute、一次 Review |
| `l1-repair` | 首次执行后，固定脚本注入错误输出；真实 L1 拒绝，再由引擎修复 | 存在 L1 失败且 Execute 多于一次 |
| `review-return` | 注入输出仍正确、但原地排序输入的回归；L1 通过，Review 应退回 | 存在 Review return 且 Execute 多于一次 |
| `long-context` | 在实际 Execute prompt 的 `cache-scope` 中加入稳定参考材料 | 实测单次 input 达到 manifest 阈值；缺失用量记 unknown |
| `stop-resume` | 在 Review 前由引擎正常暂停，再显式运行 | 已保存 paused 状态后继续执行；这是阶段边界恢复 |
| `configuration-change` | 首次 Execute 后显式改变同一 binding 的 thinking，再执行 | 观察到有效 binding fingerprint 改变 |

受控注入只修改候选代码；评估器核对注入前后的 Go state、任务契约和配置不变。它不伪造 L1、Review、receipt、metrics 或 state。阶段边界暂停不替代 P3 对活跃 turn 杀进程、ACK 故障窗口和只读恢复的独立测试。

有效配置写在隔离 trial 的候选目录外，通过 `FLYWHEEL_CONFIG` 指定。开始前，评估器在该 clone 中移除会覆盖它的根目录和任务目录配置并提交这个实验基线。因此配置变化不会伪装成模型修改受保护契约。源配置和变更前后 binding 均保留在条件记录中。

## 运行真实后端评估

构建当前 Go CLI，确认所选公开 adapter、原生 CLI 与账号能够执行任务后运行：

```sh
node scripts/evaluate-sessions.js \
  --manifest /absolute/path/new-session-corpus/manifest.json \
  --flywheel /absolute/path/flywheel \
  --output /absolute/path/new-session-results \
  --live
```

`execution:"live"` 必须显式传 `--live`，且不能使用 mock driver。评估器使用提供的模型与权限，不自动切换到其他模型、渠道或更宽权限。输出目录必须不存在，已经保存的结果不会被覆盖。

所有调用仍走真实引擎的预算、停止和恢复规则。manifest 的 `maxCalls` 是额外的观测调用上限：运行中每 50 ms 检查落盘 metrics，达到上限发送取消；已在途调用和 metrics 落盘延迟可能造成超出，因此它不是模型调用前的事务额度预留。`runTimeoutMs` 控制每个步骤，取消后保留最多 30 秒清理时间。强制杀进程、未知派发/终态或未确认清理会停止整个实验，保存已知结果，不自动重发。

## 报告如何解释

`report.json` 包含源 commit/tree、manifest/config/engine/oracle 的 SHA-256、开始结束时间、每个 trial 的固定条件、顺序、策略、原始步骤结果和配对比较。每个 trial 另存 stdout/stderr、状态快照和原始 metrics。独立 oracle 从固定 commit 复制到候选目录外，执行前后核对 hash；候选篡改 oracle 会终止实验。

| 字段 | 含义与边界 |
|---|---|
| `quality.pass`、`omissions`、`regressions` | 独立 oracle 的结果；未完成检查保留 null，不拿引擎 completed 代替质量证据 |
| `repairRounds` | packet attempts 减首次执行的总数；使用真实 Go 状态 |
| `repeatedFindings` | 规范空白后完全相同的 Review return 原因重复数；不声称语义相同 |
| `elapsedMs` | 引擎运行步骤耗时；不包含 clone、oracle 与注入开销 |
| `usage.*.known` | 完整单次 input/output/cacheRead/cacheWrite/totalTokens 的已知合计 |
| `observedCalls` / `missingCalls` | 各指标实际可计数和缺失的调用数；有缺失时 known 仅为下限 |
| `cost.knownUSD` | 已知费用合计；缺失费用单列，不把订阅账单当零 |
| `actions`、`rebuildReasons` | 实际创建、续用、重建次数及其可核对原因 |
| `recovery` | 续用数、失败或未知续用数、未知终态、未解决调用、只读核对次数 |
| `scenarioCoverage` | 分别记录已计划、实际观测到、未观测到和 unknown；场景名称不是成功证据 |

P3 的 `record-kind:"reconciliation"` 行按 `reconciles-invocation` 替换原调用的后验事实；不把审计行当成新模型调用，也不重复累加用量或费用。原始审计行仍完整保存。

只有三个策略都完成配对、固定条件相同且实际 role/harness/provider/channel/model 集合一致时，才计算差值。某个用量字段缺失则该差值为 null。报告没有自动生成收益百分比；需要先判断验收质量和场景是否成立，再讨论性能变化。

## 离线门禁与真实证据分开

仓库提供三层不同的证据入口：

1. `node --test npm/test/session-evaluation.test.js` 验证 manifest、隔离、未知用量、配对与停止规则，使用明确标记的 runner 协议 fixture。
2. 下列命令运行真实 Go CLI → worker 3 → 实际公开 Codex 0.9 包 → 原生 CLI fixture，共 36 个 trial。fixture 只实现原生协议和受控代码工作，不写 Go state/metrics/receipts；真实引擎执行 L1、Review 判定、Git 边界与恢复选择。
3. `--live` 使用真实后端推理，才能提供模型质量、缓存、耗时与费用方面的实际证据。

```sh
FLYWHEEL_EVAL_BINARY=/absolute/path/freshly-built-flywheel \
FLYWHEEL_EVAL_OUTPUT=/absolute/path/new-native-evaluation \
node scripts/test-session-evaluation.js
```

仓库根目录的 `npm run test:sessions` 是同一门禁入口，接受相同环境变量。

第二层要求已构建 `plugins/dsh/dist`、已安装实际固定版本 Codex 包；缺少二进制或依赖会失败，不会自动 skip。它在独立目录冻结 worker 和公开包，原生 fixture 只能访问带实验标记的 CODEX_HOME 与任务集，不接触实际账号。用量由 fixture 合成，只验证计量链路，不能作为模型节省数据。

当前实现不以离线通过宣称真实账号或 macOS 生命周期验收已完成。真实环境若拒绝权限或在 thread/start 超时，应保存阻断证据；默认阈值继续关闭，直到配对真实结果支持具体调整。
