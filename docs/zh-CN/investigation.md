# 有界调查与反馈驱动工作流

[文档首页](../README.zh-CN.md) · [English](../en/investigation.md) · [工作流](workflow.md)

Task 是推荐的目标入口。它先区分交付终点，再准备适合的契约；Auto 在合法实现契约
上选择 Quick 或 Full。调查是实现前可独立交付的工作，不是第三种完成模式。

| 日常需求 | 推荐入口 | 引擎行为与终点 |
| --- | --- | --- |
| “先排查，告诉我原因” | `/flywheel-task` | 受管读取／搜索、证据报告；不实施修改 |
| “排查并在原范围内修好” | `/flywheel-task` | 调查有支持结论后准备 Spec，继承来源、用量和范围，再 Auto |
| “问题已定位，快速修复” | Task Auto 或显式 Quick | 资格成立才走紧凑流程；必需验证、独立最终 Review、Git 交付保留 |
| “复杂任务先出方案，我们审阅” | `/flywheel-new` | 准备方案后停止；获得后续实施授权再 Run Full |
| “继续这个已有任务” | `/flywheel-run` | 恢复原准备或实现记录；不新建任务、不重置预算 |
| “只运行一个阶段” | Plan / Exec / Verify / Review | 保留直接控制；阶段通过不代表任务完成 |

Kimi 使用对应 `/skill:flywheel-*` 名称。Status、Stop 均可用于调查和实现。用户已明确
要求“排查并修好”时，不重复要求同一授权；新范围或未解决的产品取舍仍需单独决定。

## 调查契约

宿主助手从当前需求与仓库事实生成输入，Go 校验并写 `preparation.json`。不要求用户
手工填写表单，不允许宿主直接修改准备状态或 `state.json`。可读取已安装二进制的
`preparation.schema.json`、`preparation-state.schema.json` 和 `preparation.example.json`。

契约 v1 包含目标、预期、环境、时间窗口、已知证据、可证伪假设、精确读取路径、具体
动作与预算。`intent` 为 `investigate` 或 `investigate-and-fix`；后者需要非空的
`implementation-scope`，前者不允许带实施授权。只存在准备记录时，尚不需要 Spec。

首个可采用动作族由 Go 直接实现：

| kind | 参数与能力 | 限制 |
| --- | --- | --- |
| `read_file` | 读取一个已授权 UTF-8 文件 | 拒绝符号链接、特殊文件与超限输入 |
| `search_text` | 在一个已授权文件中查找字面量，返回行号 | 非正则，无命令拼接 |
| `list_files` | 列出一个已授权目录的名字 | 不递归读取内容；最多 4096 个名字 |

路径必须是干净的仓库相对路径，动作必须引用允许路径和已有假设。打开的文件还须与
检查时的 inode 一致；`os.Root` 限制目录穿越。目录授权只授予名字读取。首版没有
任意 shell、外部查询、生产重启、清缓存、数据回填或可写 Agent 调查动作。现有
full-access 配置不改变这些 Go 动作的能力。更广实验需要独立增加处理器、契约和验证。

`limits` 冻结动作次数、总时限、单文件字节和总输出字节；恢复不重置。总时限以首次
准备的 deadline 为准，暂停时间也会消耗这个时限。只保留有界 UTF-8 输出，另存内容
摘要；`truncated` 表示展示不完整，不能把片段当成完整日志或完整测试集合。

## CLI 与桌面控制

准备实际目录后，提交一个包含完整契约的请求文件：

```bash
flywheel control investigate --task-dir "$task_dir" --cwd "$repo_dir" < start-request.json
```

```json
{"action":"start","contract":{"version":1,"id":"replace-with-a-complete-contract"}}
```

上面的 JSON 仅展示请求外层；`contract` 必须替换为完整 schema 合法输入，不能直接
执行这个缩写示例。`start` 保存 ready 准备记录，不执行动作。需要运行时提交：

```json
{"action":"run","revision":1}
```

`revision` 必须取实际回执。Go 按原任务运行 lease 启动后台调查，复用已有就绪与
token 绑定的停止协议。DSH 使用 `flywheel_control` 的 `action: investigate` 和
嵌套 `investigation` 请求，会关联当前会话视图；不需要手填路径。视图显示预算、
观察、截断、决策回执及报告，提供继续、核对回执、停止和刷新。

```bash
flywheel control status --task-dir "$task_dir" --cwd "$repo_dir"
flywheel status "$task_dir"
flywheel stop "$task_dir"
```

控制响应 `ok:true` 只确认控制请求；`running` 根据运行锁判断。只有调查时响应是
`preparation`，不会伪造实现 `state`。未知结果须从原目录核对，不能重复启动来试运气。

## 证据报告与修复交接

动作结束后状态为 `awaiting-report`。当前宿主模型根据观察提交一次分析报告，Go
校验字段、假设覆盖和观察引用。报告必须区分事实与推断，逐项说明 supported /
contradicted / unknown，包含反例检查、局限、推荐方案、回归检查和回退条件。
引用使用真实 `invocation.id`；失败或不存在的观察不能作为成功取证。

通过 `control investigate` 提交 `action: report`、实际 `revision` 与 `report`。
终态为 `supported`、`inconclusive`、`access-blocked`，或取消产生的 `cancelled`。
`supported` 表示分析者给出的结论有结构合法的观察引用；Go 不替代语义核查，也不
声称这个状态证明因果。没有支持结论时交付局限，不自动扩大调查或实现授权。

原请求授权修复、调查已 supported 且全部回执已解决时，可在同一工作区与任务目录
准备 Spec／packets，再走常规 launch。Go 把来源身份、报告摘要、原实施范围、动作
次数与耗时写入 `preparation-source`；JEV 已知用量按原派发日一次计入任务总账，
缺失用量与费用仍记 unknown。恢复不会重复记账。实际实现 diff 仍受原范围约束，
Quick → Full 或允许模型升级不会增加权限。

准备状态、原始观察与决策 journal 属于本地私有运行账本，自动 Git 交付排除这些文件。
不能把日志中的凭据或源码片段自动带进 PR；如需发布调查结论，按用户目标另写经过
检查的文档。已进入实现后不得删除或改写原准备来源来绕过范围和预算。

## 快反馈、Plan 与进展

执行提示词鼓励先跑相关的小检查；Go 仍在当前候选上执行冻结的完整验收，之后才
独立 Review。这样去掉固定重复的全量验收要求，而不是缓存或省略最终门禁。

反馈保留固定 check ID、期望和实际终态、候选／检查集／环境摘要、完整失败输出摘要，
以及有界的头尾片段。`observationComplete` 只说明命令结果完整，不说明测试枚举完整。
同一候选、同一检查与环境反复得到相同失败才可判 stalled；候选改变而缺乏可信 oracle
时记 unknown，仍受总尝试次数约束。不会仅凭同一文件名判空转，或从文本推断“10→1”。

Full 首次 Plan 后，只有已确认终态、合法重载契约、成功持久化并同步的检查点才可复用。
普通返修保留该检查点；契约／验证／调度策略改变、未知外部 HEAD、旧任务没有证明、
显式 Plan 或 verification-gap 都重新规划。Plan 的价值是消除真实不确定性，速度快
不构成删除规划或降低质量门禁的理由。

## JEV 决策

使用 [JEV 指南](jev.md) 中唯一的 `jev.enabled` 设置。Start 自动解析配置并冻结
策略，Task／New／Run 不再各自维护决策模式。默认关闭时沿用原动作顺序和收集
限制；开启后可选择下一项动作、跳过冗余取证，或在满足 Go 证据前提后提前转入报告。

跳过动作和提前结束收集均保存决策及观察引用，不会把假设自动标成已证明或已反驳。
分析者仍要形成有证据的报告；修复仍须遵守原范围及普通验证、review 流程。
`awaiting-report` 后重复 Run 不会重新开始收集。

目标、模型、数据类别、阈值和预算在创建准备记录时冻结。当前关闭、目标不再相容
或收紧策略会停止后续可选决策；请求返回后、采用前仍要复核。已知／未知回执继续
核对，不重发。创建时关闭的准备记录保留原基线路径，新准备记录才采用新开启的
策略；桌面对持久状态明确标注“最近观察到的状态”。

离线回放和显式在线评估见 [JEV 指南](jev.md#离线回放与显式评估)。关闭 JEV 仍可
本地回放；在线评估必须服从同一已启用的总开关和配置目标，其数据与次数限制不能
放宽配置。两种工具均不执行动作、不修改任务状态。

## 度量与当前边界

`metrics.jsonl` 的调用记录以外新增 `phase-start` / `phase-end`。`control metrics`
的 timeline 同时给累计区间时长和已闭合区间的并集；并行区间不能相加冒充墙钟时间。
缺少结束、时钟倒退、无法关联的旧调用单列，Provider 内部排队／模型／工具细分保持
unknown。调查动作和调查运行使用同一阶段接口，宿主准备对话的不可观察时间不补零。

本实现没有基于这些离线测试宣布更少 token、更低成本或最终质量相同。JEV 仍默认
关闭，明确开启总开关才参与已支持的决策；没有改变默认模型、预热或并发设置。ready queue、持久 worker、已有候选导入等继续按测量结果
决定；现有取消、共同候选复验和持久化规则保持有效。
