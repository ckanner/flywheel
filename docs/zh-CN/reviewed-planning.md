# 方案设计、讨论、审查与实施

[文档目录](../README.zh-CN.md) · [English](../en/reviewed-planning.md) · 简体中文

New 和 Plan 把真正的方案设计交给引擎管理。New 登记简短 brief，自动开始设计和独立
审查，然后停下来等你批准；Plan 用于设计修订或具体任务的细化，审查结束后也停下。
宿主对话提供目标和事实依据，不需要先写出完整架构和全部工作单元才启动 Flywheel。

本指南适用于 `state.planning.version:1` 的任务。已有 Spec/packet 任务保持旧行为，
只有明确迁移才进入新协议。故障与交付场景见[使用手册](usage-handbook.md)，实际模型
绑定见[配置参考](configuration.md)。

## 桌面目标入口

空白 Flywheel 画布可以直接创建简短需求。工程与修复默认 Auto 和 review-first。“开始设计与审查”调用 Go 规划引擎，与 Task 技能使用相同的精确版本批准门禁；必须等你批准才能进入实现。表单只选择 delegated 还不够，必须再明确勾选实施授权。Quick／Full 是高级深度选项，不改变批准方式。

可选图片检查把仓库相对路径、类型和字节摘要作为未审证据加入简报，不会把原图自动交给规划模型，也不修改已批准 packet。批准前核对产出的 `input-images` 与兼容执行绑定；真正执行前仍重新验证图片字节。参见[任务入口](workflow.md#从-flywheel-视图创建任务)。

## 选哪个入口

| 入口 | 做什么 | 停在哪里 |
|---|---|---|
| `/flywheel-new <目标>` | 登记薄 brief；Go 运行方案设计与独立 Design Review | 等待用户批准审查通过的精确方案版本 |
| `/flywheel-plan <任务目录>` | 整体方案设计或修订，默认 scope=design | 规划与审查结束；即使之前授权了实施也停下 |
| `/flywheel-plan <任务目录> scope=milestone id=M-01` | 在已批准根方案内细化一个里程碑 | 子计划及独立 Plan Review 完成后 |
| `/flywheel-plan <任务目录> scope=packet id=M01-T01` | 细化当前某个工作单元 | 规划与审查完成后 |
| `/flywheel-plan <任务目录> scope=replan` | 根据新证据重新规划 | 实质变更返回根方案审查 |
| `/flywheel-task <目标>` | 沿用用户已有实施授权，先设计和审查再执行 | review-first 等批准；delegated 在原授权内继续 |
| `/flywheel-run <任务目录>` | 恢复原任务和当前获批范围 | 遇到未解决审批、输入、预算或恢复问题时停下 |

Kimi 使用相应的 `/skill:flywheel-*`。这些是在对话中调用的 skill；原生
`/flywheel {…}` 是插件的结构化控制命令，不是自由文本规划入口。

`replan` 表达修订意图，沿用原来的审查范围：省略 id 或 `id=root` 表示整体设计，
`id=M-01` 表示里程碑，`id=M01-T01` 表示 packet。版本历史和 findings 都留在
对应范围内，改换命令名称不会清掉尚未解决的审查问题。

只补充当前范围的细化要求时，可以填写视图中的“本次规划说明”，或对 Plan skill
说“展开 M-01 的崩溃恢复步骤，保留已批准的 API”。控制请求的可选 `text` 会作为
本次 request 的不可变说明，同时交给 Planner 和独立 Reviewer；它不改变全局输入
版本，也不撤销根方案批准。整体目标、固定决定或实施范围变化时，应使用“方案反馈”，
重新审查根方案。

不熟悉或比较复杂的系统适合从 New 开始。New 会调用设计模型；只有你明确要求
“先登记，设计也暂时不跑”时，才使用 prepare 的 `start:false`。

## 一次完整的方案讨论

在目标仓库的 DSH 对话里输入：

```text
/flywheel-new 为这个仓库设计多租户任务调度系统。
保留现有公共 API，比较恢复、隔离、迁移和回退方案。
完成独立设计审查后停下来，我要讨论方案再决定是否实施。
mode=full
```

技能核实项目和任务 checkout，记录原始请求、约束及已查证的事实，提交一次
`planning prepare`。后续模型调用、轮次、预算和状态保存都由 Go 管理。
brief 不是完整方案；尚未解决的决定继续留在方案中，宿主和 Provider Team 都不能
把它们自称为已获批准。

在同一会话打开 **Flywheel**，方案区域显示：

- 当前方案的版本、完整摘要、正文、需求和设计决定。
- 快照中保留的上一版本，可展开对照。
- 独立审查 verdict、findings、证据、影响及处理结果。
- 最近批准的根方案，与当前各作用域的执行子计划分别展示。
- 规划轮次、停止原因、已采用反馈和最近控制请求的回执。

你可以在对话里讨论，也可以在视图的**方案反馈**里写：“保留当前 API，用数据库做
持久队列，并解释确认成功但调用者超时时如何恢复。”反馈请求绑定当时的任务状态
版本和方案引用。**已排队**只说明等待现有 owner 采用，不表示方案已经修改。
反馈采用后，在视图选择**运行 Plan**，或对同一个目录调用 Plan。新方案有新的版本，
还会经过独立审查。

审查可以通过、要求修订、需要用户决定或阻断。审查通过是对方案的评价，人工批准
是另一份记录。有界循环可能因待决定问题、轮次、预算或 Provider 结果未知而停止，
这些状态不会自动变成通过。

## 批准你真正看过的版本

当前根方案通过独立审查后，可以选择：

| 操作 | 结果 |
|---|---|
| **批准此版本** | 记录对上方 revision 和 digest 的批准，实施继续停止 |
| **批准此版本并开始** | 同一次动作批准该版本，并授权开始实施 |
| **开始已批准方案** | 已批准后补充启动授权并继续，不要求重新做一次设计决定 |

刷新、重新打开视图、让模型说“已批准”或调用普通 Run 都不会批准方案。模型工具不提供
approve。DSH host 在派发前核对原始真实用户 command 事件和精确参数；模型来源、
fork 继承事件、已消费命令或过期版本都不能代替这次动作。CLI 的人工批准需要交互终端
确认该版本的完整 digest。

较大的控制台快照可能省略历史记录或正文，视图会明确标注，并给出原任务的
`state.json` 路径；空的投影文件列表不表示原文件为空。当前设计或独立审查显示
不完整时，按钮审批停用。可以用不带 `--compact` 的 `control status` 读取完整
账本，或使用交互式 CLI 批准入口；该入口会展示权威的完整设计与审查。

根方案可以先批准，不必等所有子计划都生成。之后的细化也不能改变已经批准的意图：
根与子计划各有精确引用，独立 Plan Review 和激活检查分别生效。激活事务没有完成时，
实现保持阻断，由 Go 从原事务恢复。

## 合格局部任务的紧凑准备

brief 可以用 `design-granularity:compact` 选择紧凑设计，同时记录具体的
`granularity-reason` 与仓库依据。仅写 `mode:quick` 不会切换设计模型，也不证明资格。
规划者一次返回根设计、完整 `spec.md` 与一个 packet，采用版本化 `compact` 输出。
Go 先验证需求映射、写范围、可执行检查和确定性 Quick 资格，再整体采用两份产物。
不能成立时升级 system 设计，保留已经使用的轮次、用量和历史。

独立 `design-review` 对两份精确产物做联合审查；一条 `compact-v1` 记录同时绑定
设计和计划引用，分别确认设计有效、覆盖完整、实施在设计范围内。它不会被复制成
假的 Plan Review。合格任务取得授权后即可激活该单 packet 契约，不再额外固定调用
一组里程碑 Planner／Reviewer。全新合格局部任务首次正常完成通常是紧凑设计、联合
独审、Execute、task-final Review 四次逻辑角色调用；调查、内部工具、失败与返修另计。

review-first 仍等待批准精确设计及所审查的投影；批准与开始实施是两份不同的授权。
New、显式 Plan 保持原停止点，delegated Task 沿用原始实施委托。激活重新核对输入
版本、产物摘要、独立审查身份、仓库／依赖身份和范围；契约字节变化或审查覆盖缺失
不能沿用旧联合审查完成激活。控制台摘录省略紧凑计划文件时，审批停用，须读取完整权威产物。

已有有效计划与原审查仍可复用；旧 Quick 任务不需要为了恢复而重新生成紧凑方案。

## 执行中的逐步细化

复杂项目不必第一天就冻结每个实现细节。先批准目标、约束、固定决定、里程碑结构
和验收方向，之后由引擎在这个范围内细化下一个相关里程碑或 packet。
每个子计划携带获批根方案引用，并有独立审查。

范围内的可委派细节无需反复批准整个根方案；改变需求、固定决定、约束或验收承诺时，
必须回到根方案修订。不能把工作改名为“小 packet”就避开边界。想先看某个任务的
执行方案，可以显式使用 `scope=milestone` 或 `scope=packet`。

显式 Plan 始终是 planning-only。即使原任务已经有启动授权，这次 Plan 结束也停下；
随后按需要 Run 继续。Quick 的实现快路径仍是 Execute、必需验证和独立最终 Review；
新规划协议还会先做紧凑设计与 Design Review。“两次实现阶段模型调用”不表示新任务
从 brief 到交付只需要两次模型调用。

## 模型和 Provider 分工

| 角色 | 未显式配置时的绑定来源 | 职责 |
|---|---|---|
| `loops.design-plan` | `loops.plan` | 整体方案设计与修订 |
| `loops.design-review` | `loops.review` | 独立审查精确根方案版本 |
| `loops.plan` | 原来的配置 | 执行子计划细化 |
| `loops.plan-review` | `loops.review` | 独立审查子计划质量与范围 |

当前 DSH 对话负责整理 brief 和你的反馈，换宿主模型不会改变引擎绑定。用 config
inspect 查看实际配置，`bindingSource` 说明继承来源；配置视图也展示继承角色，编辑
一个角色才为它建立独立覆盖。`planning.compact` 引用两条紧凑设计 profile，system 设计使用普通 loops。没有声称
某模型或思考等级对所有设计都最优。

继承的是后端/profile/native 设置，角色会话与权限覆盖仍独立。显式的
`sessions.roles.review` 不会变成 `design-review` 的历史。`role-based` 下新审查角色
默认请求只读；若继承了 Claude Code Pro，需要已有授权的全局 full-access，或给
`loops.design-review`、`loops.plan-review` 分别配置权限，否则 doctor 会报告能力冲突。
不能静默扩大权限。跨渠道 fallback 也需要新角色自己的显式 model 映射，后端继承
不代表自动获得新的付费授权。

规划复用有效的原生能力配置。DSH Team 和 PTC 按实际准入及预算使用，本地 DSH 子树
保留 root 的 provider、model 和 effort。Codex 原生 review、Claude Team 的结果
可以作为证据，正式 verdict 仍由独立 design/plan reviewer 产生。Kimi Tower 候选
transfer 保持 Execute 专属，普通设计和审查角色不会获得转移权限。详见
[原生能力](native-capabilities.md)。

## CLI 与恢复

无模型调用的只读检查：

```bash
flywheel control status --task-dir /absolute/task/docs/my-task --cwd /absolute/task
flywheel template planning.schema.json
```

`control planning` 从 stdin 读取一个 JSON 请求；DSH 模型工具把相同对象放在
`planning` 字段内。只做规划的请求示例：

```json
{
  "action": "plan",
  "revision": 7,
  "request-id": "plan-my-task-2",
  "scope": "design",
  "start": true
}
```

把示例 revision 换成当前 `state.revision`，同一次动作保留同一个 request-id；
里程碑或 packet scope 还要有实际 scope-id。保存到私有 JSON 文件后调用：

```bash
flywheel control planning --task-dir /absolute/task/docs/my-task --cwd /absolute/task < /absolute/private/plan-request.json
```

人工批准请求用 `action:"approve"`、当前 state revision、新 request-id、精确
`design:{revision,digest}`，以及 `start:false` 或 true。在自己的交互终端执行并审阅
显示的完整方案与审查，然后输入 `approve <digest>`，其中 `<digest>` 替换为该版本的
确切摘要。不要注入审批来源环境变量，也不要让模型代输入批准。

超时或回复丢失时，先查看**原任务**再做下一次变更。控制成功只是请求回执，不等于
任务完成。结果未知、清理未确认和用量缺失保持真实可见；不能复制 state、清空回执、
切 fresh/scoped 或另建任务来隐藏旧结果。规划角色确认清理和计量后，可以使用
[人工退役未知调用](session-recovery.md#人工退役未知的规划调用)；退役只结束该次
未知副作用，之后仍需显式 Plan 创建新 request，不能把它当作重放或成功。

如果原生会话的人工接管改动了契约，导致实施被阻断，先释放同一次 handoff。释放
要求精确匹配原 session key/thread、已记录调用均已消费且清理已确认；它不会批准
改动后的契约，也不会恢复实施。其他未知调用仍会阻断释放。之后可以在原任务上
提交反馈并运行 Plan。

反馈排队时，刷新按原 request-id 读取引擎最终的采用或拒绝回执。被拒绝的请求保留
原输入与原因，不会悄悄重发，也不会显示成已经采用。

任务根目录和 `planning/` 中的 `.planning-tmp-*` 普通文件保留给 Go 原子写入。
准备或恢复时，引擎持有任务 lease，清除中断遗留的这些临时文件，再由权威 state
重新生成导出。同名前缀若是符号链接、目录或特殊文件则停止并要求检查；普通用户
文件不会被清理。

旧 `run --only plan` 保留原 Full 里程碑规划语义，升级不会重新解释已有账本。
明确 `planning migrate` 时才绑定当前 revision、request-id 和选择的 policy 进入
新生命周期。保留原任务与证据；继续旧工作不以迁移为前提。
