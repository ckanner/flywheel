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

## 可选 JEV

新 `decision` 配置与旧 `jev-shadow` 完全独立，省略时不发送请求。具体配置字段由
准备 schema 定义，包含固定 `jev-x.y.z` 模型、显式 endpoint、密钥环境变量名、超时、
策略版本、模式、数据允许集合、阈值与最大调用数。凭据值不写契约。

- Off／offline 不联网；shadow 只保存可审查快照；advisory 只留建议；adopt 才允许选择
  一个已批准的诊断动作。自动采用仍需独立实验政策，阈值不是本项目准确率保证。
- 至少两个合法选项与真实语义不确定性才值得调用；单个请求配对 Noul 适用性与 Choice，
  保留完整概率。没有适用选项、模型版本不符、证据字段未授权、预算不足或未知执行都弃权。
- 采用前重新检查任务／run／revision、契约、候选、观察及动作绑定；将回执消费和唯一
  action invocation 在同一 CAS 中保存后派发。JEV 不能生成命令、减少验收或宣布完成。
- 可选请求未知时不重发，保留未知用量，转合法确定性基线。实际动作回执未知则 blocked。
  结束、取消或 deadline 到期仍只读核对可选请求，不能留下无法结束的 pending 记录。
- 最大调用数包括 unknown。硬预算没有可实施的单次消耗上限时跳过 JEV；不会用字符数
  估计冒充可靠预留。shadow 研究与在线采用的费用和作用不同。

旧工作流影子开关仅捕获本地有界摘要，后续显式 `shadow run` 才请求服务；它不能采用
动作，也不授权新增源码／日志发送。新动作族的离线和影子评估不能证明采用后更快。
采用是否划算应比较既有基线、核心改进无 JEV、核心改进加单动作族三个版本的完整任务。

### 离线重放与显式 advisory 评估

通用决策的 `*.snapshot.json` 捕获文件包含冻结的 `snapshot`、`policy` 和来源
`target-hash`，与旧工作流 shadow 数据集不同。真实评估前先审阅事实与数据权限。

```bash
flywheel decision replay --input frozen.snapshot.json --response native-response.json \
  --endpoint "$JEV_ENDPOINT" --model jev-1.14.0 --expected-model jev-1.14.0
```

`replay` 接受已记录的原生 System One 响应或 fixture，强制 `offline`；不读取密钥、
不发送 HTTP、不写账本或任务状态。endpoint 和相同的固定模型版本只标明评估目标，
上面的版本是示例，必须与待评估响应相符。裸 Snapshot JSON 还须指定
`--policy reviewed-policy.json`。捕获文件也可用该参数完整替换策略；替换策略省略的
数据权限不会从原捕获策略继承。

明确授权的真实评估使用独立命令：

```bash
flywheel decision evaluate --input frozen.snapshot.json --policy reviewed-policy.json \
  --endpoint "$JEV_ENDPOINT" --model jev-1.14.0 --expected-model jev-1.14.0 \
  --directory ./decision-evaluation --key-env MEROUTER_API_KEY --max-calls 5
```

即使来源策略为 `adopt`，`evaluate` 也强制 `advisory`。密钥环境变量名、固定目标、
独立私有账本和累计调用上限都须明确指定。来源策略的数据类别权限必须覆盖快照；
不会借旧 shadow 开关扩大权限。`--hard-budget` 因原生 API 不能强制请求用量上限而
跳过调用。同一账本和快照只读取原回执；未知请求计入上限且不重发。命令不执行动作、
不改变任务状态。

两种命令都输出基线、实际策略、概率和回执，并明确 `adopted: false`。真实调用错误
或未知返回退出码 3 和回执；输入／配置错误为 1，发送前取消为 143。离线重放通过只
证明解析与 fixture 行为，不证明模型实际成功率。任务中有限采用仍须独立准备策略、
数据 opt-in、冻结的 Go 具体动作和上述当前状态校验。

## 度量与当前边界

`metrics.jsonl` 的调用记录以外新增 `phase-start` / `phase-end`。`control metrics`
的 timeline 同时给累计区间时长和已闭合区间的并集；并行区间不能相加冒充墙钟时间。
缺少结束、时钟倒退、无法关联的旧调用单列，Provider 内部排队／模型／工具细分保持
unknown。调查动作和调查运行使用同一阶段接口，宿主准备对话的不可观察时间不补零。

本实现没有基于这些离线测试宣布 token、更低成本或最终质量相同，也没有开启新默认
模型、JEV、预热或更高并发。ready queue、持久 worker、已有候选导入等继续按测量结果
决定；现有取消、共同候选复验和持久化规则保持有效。
