# 工作流：从需求到 Pull Request

[文档首页](../README.zh-CN.md) · [English](../en/workflow.md) · 简体中文

Flywheel 将经过审阅的任务契约转化为经过验证的本地 Git 变更。宿主助手负责准备薄 brief，
Go 引擎负责设计、独立审查、调度、验证、恢复和交付。流程控制台还支持在明确授权后创建和合并远端 PR。

本文适用于已有仓库。安装及独立的首次运行示例见[快速开始](getting-started.md)。
故障调查、Issue 修复和复杂项目的操作示例见[使用手册](usage-handbook.md)。

## 从 Flywheel 视图创建任务

当前会话尚无关联任务时，画布提供“工程任务”“修复问题”和“排查原因”入口。输入目标即可；工作目录取自当前会话，引擎在该目录的 `docs` 下创建任务账本，不要求你预建目录。打开表单本身不调用模型。

工程与修复默认 **Auto**，授权默认为“方案审查后等我批准”。按钮通过 `planning prepare` 开始设计和独立审查，不会批准产出的方案。Quick／Full 放在高级选项；委托执行必须同时选择对应策略并明确勾选实施授权，执行资格仍由已审契约决定。

排障表单要求预期行为、待验证假设、反证条件，以及最多八个明确的文本文件路径（相对于当前会话工作目录）。它创建只生成报告的调查契约并启动 Go 调查引擎，最多 60 秒、每个文件 256 KiB，不授予命令执行、服务访问或实施范围。复杂排障仍通过 Task 技能准备有界调查契约，见[调查指南](investigation.md)。

工程或修复需要原始图片时，可填写最多四个仓库相对图片路径，点击“检查并预览选定图片”。只读检查实际 PNG/JPEG/GIF/WebP 字节，拒绝符号链接和仓库外路径，展示类型、尺寸、字节数及 SHA-256；每张最多 4 MiB，合计最多 8 MiB。修改路径会使旧选择失效。简报收到的是标为**未审**的选定引用；之后只有经过审查的 packet 才能成为执行输入。规划阶段收到路径和摘要，不会自动获得原图附件。Execute 仍须验证冻结原图并使用已支持的 Claude SDK 通道。聊天附件不会自动加入，没有原始图片的任务仍可正常使用 Gemini。

## 0. 常规入口：描述目标并启动

在 DSH 对话中使用任务技能描述需求：

```text
/flywheel-task 修复已定位的问题，补齐回归测试并完成验证。mode=auto
```

宿主核实代码与任务目录，记录原请求、约束和授权形成薄 brief，再调用
`flywheel_control` 的 `action:planning` / `planning.action:prepare`。Go 负责设计与
独立审查，再进入实施；review-first 等待批准，delegated 沿用已记录的原范围授权。插件将真实任务关联到当前会话。
加载技能或收到方案并不代表引擎已启动，应核对启动回执和任务状态。

| 模式 | 选择与执行 |
|---|---|
| Auto，默认 | Go 根据已记录的资格事实选择 Quick；依据不足则选择 Full |
| Quick，控制台也标为 ultraWork | 一个里程碑、一个 packet；Execute、必需验证、独立最终 Review、Git 交付 |
| Full | Plan、packet 执行与评审、集成 L2/L3、Judge |

Quick 要求范围明确、实现模式已知、全部需求有验收映射，最终评审可以判断整个任务。
缺失 L2/L3 时需要明确的不适用理由。鉴权、权限、支付、迁移、并发或未决设计风险应走
Full；显式选择 Quick 也不能跳过资格检查，文件数量不能代替风险判断。

正常首次成功的 Quick 实现阶段有 Execute 与最终 Review 两次 Agent 调用。
合格的新紧凑任务先一次生成设计与完整单 packet，再做一次联合独审；从 brief 到交付
通常共四次逻辑角色调用。已有有效契约可复用；调查、工具轮次、失败与升级另计。
必需测试、覆盖率、适用 L2/L3、缺陷抽检、预算及进程
收尾仍保留；Plan／Judge 标为不适用并记录原因。发现新风险或验收缺口后，Go 先保存
单向 Quick → Full 升级，再继续完整流程，保留成果和累计预算。

先运行设计与审查、停下等你批准时用 `/flywheel-new <需求>`；运行已有任务时用
`/flywheel-run <绝对任务目录>`。Kimi 对应使用 `/skill:flywheel-task` 等 `/skill:` 名称。

## 0.1 调查与专用入口

Task 可以直接接收“先排查，告诉我原因”，自动准备并进入 Go 受管调查；无需先手动 New。
只排查到报告结束；已授权“排查并修好”时，证据支持结论后在原范围内继续实现。
新[调查指南](investigation.md)说明契约、证据、JEV 与恢复。原 New → 审阅 → Run 仍适合
明确要求先审方案的复杂任务，Plan / Exec / Verify / Review / Status / Stop 均保留。

Full 复用已确认且持久化的有效 Plan 检查点；显式 Plan、verification-gap、契约变化或
不可信基线仍重规划。执行者先跑针对性检查，Go 完整门禁与独立 Review 继续负责完成。
执行允许集合和可信增量上下文是独立设置，见[配置](configuration.md)。

已授权调查可使用输入副本中的确切复现命令及明确 GET／HEAD 请求，执行权限边界、
deadline、输出上限和回执写入调查契约。开启 JEV 后，确认结束的实施失败也可在
返修、已批准反馈检查、重规划或已有允许执行绑定中选择；流程资格和最终完整门禁
仍由 Go 负责。一次反馈实验成功不等于实现完成。

## 0.2 何时使用原生协作

Auto 仍决定实现深度；唯一可选的 JEV 开关只在合法执行候选中做选择。JEV 关闭时运行
配置的初始模型，开启后可在首次 Execute 前选择一次，并在确认失败后再次决策。
决策共用既有账本和预算。更换模型、effort 或原生策略会冷启动执行器，不能跳过方案
审批、验证、独立 Review 或 unknown 调用对账。

Swarm 适合已经拆成独立组件的工作。已审查 packet 可附带以下可选契约：

```yaml
schedule:
  after: []
  writes: ["src/parser/", "src/formatter/"]
collaboration:
  version: 1
  tasks:
    - goal: "实现 parser 并测试"
      writes: ["src/parser/"]
    - goal: "实现 formatter 并测试"
      writes: ["src/formatter/"]
```

声明要求 2–8 个子任务，写入范围互不重叠且位于 `schedule.writes` 内，与 packet 一起
审查，并使该任务不符合 Quick 资格。即使静态指定 Antigravity Swarm，也必须 Full、无
图片且已有合法声明。只选 complex／batch 路由或列出多个文件不会触发 Swarm。共享
可变文件或顺序设计依赖应先在方案中处理。适配器负责请求原生协作，只有真实原生事件
才能证明协作确实发生。

只读前台调查可以使用显式子代理，不要求执行拆分；evidence-only 故障分析仍无工具且
不能生成子代理，之后获准的修复再进入普通的已审查执行流程。复杂顺序任务可以选择
Orchestrator 的固定原生模型交接，无需为了它制造 Swarm 拆分。配置、观察数量限制与
恢复见 [Antigravity](antigravity.md) 和 [JEV](jev.md)。

## 1. 分开选择对话模型与规划模型

当前 DSH 对话整理 brief 和用户反馈，不必在 New 之前完成整套架构。整体设计使用
`loops.design-plan`，没有显式配置时继承原 `loops.plan` 后端；独立方案审查和子计划
审查分别用 `loops.design-review`、`loops.plan-review`，默认继承 `loops.review`
后端。角色与历史仍独立。具有明确 compact 粒度、理由和事实依据的 brief，使用
`planning.compact` 引用的设计与联合审查 profile；system 设计使用普通 loops。Auto
只负责确定性选择实施深度，不是第三条链路，也不额外调用分类模型。

用 config inspect 查看 `bindingSource`、实际后端、模型与思考等级。配置视图显示
继承的规划角色，编辑一个角色即可建立独立覆盖。Provider 原生能力继续按原准入
规则使用。完整讨论/审批流程见[方案设计与审查](reviewed-planning.md)，选择依据与
实测限制见[模型策略](model-policy.md)。

## 2. 选择任务执行目录

区分以下位置：

| 位置 | 用途 |
|---|---|
| 项目工作区 | Provider Session 的稳定仓库归属，通常为原始 checkout |
| 任务集成目录 | 准备契约、集成已验证 packet 的 checkout 和分支 |
| 任务文件目录 | 该 checkout 内的 `docs/<slug>`，保存 Spec、packet、状态与报告 |
| packet 执行目录 | Execute、验证、Review 使用的候选目录，通常是引擎管理的 worktree |

项目工作区可以与实际执行目录不同。Session 归属项目不代表文件操作在项目根执行；
提示词和证据明确绑定实际执行目录。后端不能表示选定的分离式 `workspace-write`
沙箱时，会在派发前拒绝该组合。

使用默认 worktree 策略时，准备前先创建或明确复用任务分支和独立 worktree。先检查
已有改动，使用未占用的名称，并替换成仓库真实的基分支：

```sh
git status --short
git fetch origin
git worktree add -b task/my-feature ../repo-my-feature origin/main
cd ../repo-my-feature
git branch --show-current
```

任务文件放在所选 checkout 的 `docs/my-feature`。集成分支就是传入 `cwd` 中当前
检出的分支，并非硬编码的 `main`。不要把一个 checkout 的任务目录与另一个 checkout
的 `cwd` 配对，也不要在运行中切换该分支。

`worktree.enabled` 只控制引擎对 **packet** worktree 的自动管理，与并行调度独立。
关闭后可共享所选任务 checkout，也可在实现前登记外部 worktree。外部目录仍须通过
基线、分支、干净状态和归属校验，详见[串行与并行](flow-console.md#串行与并行)。

### 隔离与失败行为

Worktree 隔离工作目录和索引，不是安全沙箱。旧串行路径在 worktree 创建失败时可以
记录原因并退回原地执行；开启自动 worktree 管理的并行路径要求独立目录，创建失败
就停止。显式关闭自动管理后允许共享 checkout，但采用相应的批次验证和交付规则。

需要独立目录时，应检查实际记录的工作区模式，在意外回退时停止。保留已有成果并
解决失败原因，不要通过修改 `state.json` 或强制重置分支伪造隔离状态。

## 3. 准备并审阅契约

在任务集成目录调用准备技能，或查看内嵌模板：

```sh
flywheel template --list
flywheel skills read flywheel-new
```

```text
/flywheel-new 根据仓库证据设计这个功能，比较相关方案并做独立审查，停下来让我讨论和批准。
```

无需先填写控制台表单。准备结果应给出真实任务执行根和任务文件路径，同时保留原
项目归属。一个仓库可包含多个任务，仓库路径本身不能唯一指定任务。

新任务在 Flywheel 视图审阅方案和 findings，提交反馈后用 Plan 修订。“批准此版本”
仍停在实施前；“批准此版本并开始”同时授权继续。随后 Go 细化并审查当前子计划，
激活执行契约，不必提前写全所有远期 packet。详见[方案设计与审查](reviewed-planning.md)。

### 旧任务或明确手工准备的执行契约

以下说明兼容的手工路径，以及 Go 激活后使用的契约。
可运行的实现任务需要已审阅的 `spec.md` 和至少一个合法 `task-packet-*.md`。应明确需求 ID、
排除范围、路由、可写与保护路径、L1 命令、里程碑 L2/L3 命令和实际基线结果。
冻结验收与普通实现测试分开：执行者可补普通单测和回归测试，不能削弱受保护验收来
换取通过。

Spec 的 `workflow` 保存资格事实、风险和检查不适用理由。并行 packet 还须显式声明
`schedule.after` 与 `schedule.writes`，字段示例见[工作流策略](configuration.md#工作流策略与恢复)
和[调度与-worktree](configuration.md#调度与-worktree)。缺少工作流字段的契约继续按
Full 处理；未声明独立性的 packet 保持串行。

审阅后提交真实契约，使 packet worktree 能从 Git 取得它们。先检查索引中没有无关
暂存改动：

```sh
git diff -- docs/my-feature
git diff --cached --stat
git add -- docs/my-feature
git diff --cached
git commit -m "docs: prepare my-feature contracts"
```

额外验收文件也须分别审阅再暂存。`state.json` 由引擎创建和维护，准备技能与模型不写它。

## 4. 按需建立任务级 Draft PR

初始契约提交后，Draft PR 可以作为固定的审阅入口。已获授权的宿主或用户可在执行前
创建：

```sh
git push -u origin task/my-feature
gh pr create --draft --base main --head task/my-feature
```

描述问题、预期结果、验收与未完成工作。通常一个可独立交付的需求或里程碑对应一个
PR；packet 是内部执行单元，不会自动各建一个 PR。提前创建 Draft PR 不代表批准合并。
控制台后续 PR 操作会在 push 后重新查询已有 PR，避免重复创建。

## 5. 从同一个任务执行目录启动

新规划任务先使用上面的精确版本控制。普通 Run 保留 review-first 停止点；
“只批准”之后不会自动获得启动授权，需要用户点“开始已批准方案”。

已有任务准备完成后，在其集成 checkout 运行：

```sh
flywheel config inspect --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --probe --json
flywheel control launch --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --mode auto
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
```

预检不发送推理 prompt，也不运行验收命令。正式启动才调用所配置的模型。
控制回执的 `ok` 表示本次操作结果，整体完成看 `state.status`。

`launch` 默认 Auto；`start` 与前台 `run` 接受 `--mode auto|quick|full`，省略时沿用
契约或持久化选择。status／stop 不切换模式，重启或请求 Auto 不能将已确立的 Full 降级。

DSH 对应使用 `/flywheel-run <绝对任务目录>` 和 `/flywheel-status <同一路径>`。
控制台跟随当前会话的显式工具关联；没有关联时，读取 DSH host session ID 精确匹配的
最新运行。准备中的任务或独立终端任务需要在目标会话成功调用 `flywheel_control` 的
`action: "status"`，携带实际任务与执行路径。单独运行终端 status 命令不会建立该关联。
从 0.34.2 起，多个关联任务显示“本会话任务”导航，单任务直接展示画布；不提供可编辑路径表单，其他会话和无归属运行不显示，详见[流程控制台](flow-console.md)。

### 各阶段的执行目录

| 阶段 | 实际执行目录 |
|---|---|
| 准备、Full Plan、获准的契约修订 | 任务集成 checkout |
| Execute、L1、独立 Review | 当前 packet 候选目录 |
| Quick 适用的 L2/L3 | 最终 Review 前的同一 packet 候选目录 |
| Full L2/L3、Judge | packet 交付后的任务集成 checkout |
| Intake、Garden | 所选任务执行根 |
| Status、Stop | 原任务文件与执行根，无需新分支或 worktree |

上表是执行位置，Provider Session 仍保留单独记录的项目工作区。任务 checkout 跨越整个
交付过程，packet 目录跨越实现、检查、评审和交付；每个 loop 不需要新建 worktree，
也不需要一个持续运行的父模型监督。

### 调度与角色历史

新安装默认配置使用 `scheduler.mode=parallel`，最大并发为 2。已有完整配置没有 `scheduler`
时仍为串行，显式 serial 也保持不变。只有依赖已满足、显式声明依赖和互不重叠写入范围
的 packet 才有并行资格。独立 worktree 可并发 Execute／L1；共享 checkout 批次并发
Execute，再验证和评审完整共同候选。Review、集成与终验保持串行。hard 预算在前一调用
完成持久化计量前只允许一个模型调用在飞，详见[控制台指南](flow-console.md#串行与并行)。

可选 `scheduler.dispatch="ready-queue"` 在自动管理 Worktree 的 Full 任务中，为
已就绪的独立 packet 补充空闲 Execute／L1 槽位；Review、Plan 修订和集成仍等待
已启动 worker 停稳。`scheduler.prewarm=true` 可以在串行 packet 的确定性检查
期间准备一个 DSH Review runtime，不提前发送模型 prompt。并行执行 lane 和共享
候选不预热；独立并行组停稳后，串行 Review 路径才可开始准备。主要重叠机会是
串行 Execute 后的 L1 和适用 Quick 检查。默认仍为 batch 调度且不预热；
Task／New／Run 和 Auto／Quick／Full 的职责保持，详见
[配置与限制](configuration.md#就绪队列与运行时准备)。

`scoped` 历史可在资格满足时复用同 packet Executor Session 返修。Review 使用独立
历史，每次仍检查完整当前候选；Plan 按任务／里程碑划分，Judge 和第二意见分别独立。
旧配置省略 `sessions` 时继续 fresh。两种模式都有调用安全记录，派发、结果或清理未知
都会阻断。[恢复指南](session-recovery.md)说明可信回执、持久化检查点和有界上下文重建。

### 单阶段操作与权限

`flywheel-verify` 只运行确定性验收，不调用 Review／Judge 或执行 Git 交付。
`--only execute` 是有界 Execute／L1 循环。Quick 的 `--only review` 执行最终评审但不
交付，必要时保存 Full 升级而不跨阶段启动 Plan；`--only plan` 要求 Full。继续完整流程
才会处理剩余阶段与交付。控制台的受管阶段操作还会检查状态 revision。

模型权限由有效全局策略及逐绑定覆盖决定。完全权限不会取消冻结边界或评审候选检查，
Kimi print 不提供 OS 沙箱保证。技能还会在开发前检查任务位置；直接 CLI／GUI 调用
不会执行 `SKILL.md`，也不会创建外层任务分支。只读状态与停止保持独立可用。

## 6. 理解本地完成，再合并 PR

本地交付要求当前候选具备有效验证、独立 Review 和可核实的 Git 持久化。packet
worktree 的成果集成到任务 checkout；交付失败保留成果。共享 checkout 批次只有在
全部成员对共同候选通过且整批状态持久化后才完成。Full 还需集成 L2/L3 与 Judge；
Quick 的最终 Review 须覆盖全部需求、验收充分性及持续资格。

远端交付是独立操作。可以手动检查并推送任务分支：

```sh
git status --short
git log -5 --oneline
git push origin task/my-feature
gh pr ready
gh pr checks
```

也可明确授权仓库、源 HEAD、任务分支和目标分支后使用控制台 PR／Merge 节点。
这些操作需要正常的 GitHub CLI 登录和仍有效的本地验收。合并会重新检查 PR、head、
base 与 checks，并绑定获准的源提交，不使用 admin 绕过，也不擅自开启 auto-merge。
合并队列接收请求不等于已经合并，详见[远端 PR 与 Merge](flow-console.md#远端-pr-与-merge)。

合并前检查当前差异、必需 CI 和最新基分支。绿灯后的变更可能要求重验；无法运行的
检查如实记录，并按仓库授权流程处理。发布或部署有独立验证，不因点击 Merge 自动开始。

## 7. 停止与恢复，保留原有证据

```sh
flywheel control stop --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
```

核对停止回执和真实失败原因后，再从原路径使用 `control start` 或 `/flywheel-run`
恢复。启动就绪超时或请求断连需要核对，不能据此重复启动；关闭页面不会停止引擎。

两种会话模式都会在派发、模型结果或清理未知时阻断。进程退出本身不能证明模型结果
已知。Go 必须将可信回执与原身份、原证据核对；切 fresh、换后端或重置状态不能绕过
未解决调用。

交付冲突时保留 worktree 和报告，处理实际 Git 冲突后由引擎重验。配置变更需重启
引擎。对已有合格会话进行受管人工交互，见[同一 Provider Session 接管](flow-console.md#在同一-provider-session-中接管)。

## 相关指南

- [配置](configuration.md)：模型绑定、权限、预算与策略。
- [流程控制台](flow-console.md)：任务、阶段、会话、运行对比与远端交付。
- [恢复与上下文质量](session-recovery.md)：中断核对与安全续用。
- [JEV 决策](jev.md)：一个默认关闭的总开关控制调查及有界失败决策，Go 保留工作流资格与验收规则。
- [系统设计](../design.md)：权威实现契约。
