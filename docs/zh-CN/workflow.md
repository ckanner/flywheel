# 工作流：从需求到 Pull Request

[文档首页](../README.zh-CN.md) · [English](../en/workflow.md) · 简体中文

Flywheel 将经过审阅的任务契约转化为经过验证的本地 Git 变更。宿主助手负责准备任务，
Go 引擎负责调度、验证、恢复和交付。流程控制台还支持在明确授权后创建和合并远端 PR。

本文适用于已有仓库。安装及独立的首次运行示例见[快速开始](getting-started.md)。

## 0. 常规入口：描述目标并启动

在 DSH 对话中使用任务技能描述需求：

```text
/flywheel-task 修复已定位的问题，补齐回归测试并完成验证。mode=auto
```

宿主研究代码，选择或复用任务执行目录，准备 Spec、packet、可执行验收与工作流依据，
再在已授权范围内调用 `flywheel_launch`。插件将真实任务关联到当前会话。
加载技能或收到方案并不代表引擎已启动，应核对启动回执和任务状态。

| 模式 | 选择与执行 |
|---|---|
| Auto，默认 | Go 根据已记录的资格事实选择 Quick；依据不足则选择 Full |
| Quick，控制台也标为 ultraWork | 一个里程碑、一个 packet；Execute、必需验证、独立最终 Review、Git 交付 |
| Full | Plan、packet 执行与评审、集成 L2/L3、Judge |

Quick 要求范围明确、实现模式已知、全部需求有验收映射，最终评审可以判断整个任务。
缺失 L2/L3 时需要明确的不适用理由。鉴权、权限、支付、迁移、并发或未决设计风险应走
Full；显式选择 Quick 也不能跳过资格检查，文件数量不能代替风险判断。

正常首次成功的 Quick 有 Execute 与最终 Review 两次阶段 Agent 调用。宿主准备、
内部工具轮次、重试和升级另计。必需测试、覆盖率、适用 L2/L3、缺陷抽检、预算及进程
收尾仍保留；Plan／Judge 标为不适用并记录原因。发现新风险或验收缺口后，Go 先保存
单向 Quick → Full 升级，再继续完整流程，保留成果和累计预算。

只准备方案时用 `/flywheel-new <需求>`；运行已有任务时用
`/flywheel-run <绝对任务目录>`。Kimi 对应使用 `/skill:flywheel-task` 等 `/skill:` 名称。

## 1. 选择准备阶段的模型

`flywheel-task` 与 `flywheel-new` 使用宿主当前对话的模型。`loops.plan` 只控制
**引擎启动之后**的规划，不会切换宿主对话模型。

| 准备任务 | 建议的 DSH provider／模型 | 思考等级 |
|---|---|---|
| 架构、协议、生命周期或不明确的验收 | `chatgpt`／`gpt-6-astra` | `high` |
| 有既定实现模式的边界明确功能 | `chatgpt`／`gpt-6.1-sol` | `high` |
| 明确的小修复或文档调整 | `deepseek-account`／`deepseek-flash` | `high` |

这些是工程起点，不是本项目的 benchmark 结论。先在实际账号中核对精确模型 ID 和
支持的 effort。提高思考等级不能修复缺失需求或错误验收依据。运行期默认分配与账号
渠道见[模型策略](model-policy.md)和[配置指南](configuration.md)。

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
/flywheel-new 为这个需求准备 docs/my-feature。研究实际代码，定义有边界的 packet 与可执行验收，运行基线检查，再展示契约供审阅。
```

无需先填写控制台表单。准备结果应给出真实任务执行根和任务文件路径，同时保留原
项目归属。一个仓库可包含多个任务，仓库路径本身不能唯一指定任务。

可运行任务需要已审阅的 `spec.md` 和至少一个合法 `task-packet-*.md`。应明确需求 ID、
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
启动与只读状态关联会使任务出现在控制台。控制台选择已登记任务，不再提供可编辑的
任务／项目路径框。带 DSH host ID 的终端运行可归属对应会话，其他运行仍能从最近任务
选择；发现过程不扫描目录。

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
- [JEV 影子评估](jev-shadow.md)：可选且默认关闭，不改变实际工作流选择。
- [系统设计](../design.md)：权威实现契约。
