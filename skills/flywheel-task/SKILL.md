---
name: flywheel-task
metadata:
  version: "1.2.1"
description: Prepare and launch an authorized Flywheel implementation or documentation task from a goal, with Auto, Quick or Full workflow selection and automatic DSH task-view association. Use for "/flywheel-task <goal>" in DSH, "/skill:flywheel-task <goal>" in Kimi, or a request to let Flywheel handle a task through verified delivery. Use flywheel-new when the user only wants a proposal, and flywheel-run to resume an existing task.
whenToUse: 用户希望描述一次目标后由 Flywheel 准备并启动任务时使用；默认 Auto，可明确选择 Quick 或 Full。只要方案时用 flywheel-new，已有任务恢复用 flywheel-run。
compatibility: "Requires the Flywheel CLI with control launch and workflow schema v1, Git, and the configured harnesses. DSH task-view association requires the matching plugin with flywheel_launch."
arguments: requirement mode
---

# flywheel-task — 从目标准备并启动

在当前宿主对话中研究需求、准备契约，再调用一次 Go 启动接口。模式选择、重试、
预算、升级、验收、提交与恢复均由引擎负责；不在 skill 中重新实现循环，永不写
`state.json`。本入口默认 Auto，不自动更换当前对话模型或 Flywheel 模型配置。

DSH 使用 `/flywheel-task <目标>`，Kimi 使用 `/skill:flywheel-task <目标>`。
`/flywheel {…}` 是插件原有的人工 JSON 控制命令，与这个 skill 不同。
可在请求中明确 `mode=quick` 或 `mode=full`；没有指定时使用 `auto`。

## 1. 确认目标与已有任务

用户明确调用本入口时，HTML、文档或代码任务都可准备可验收契约，不因“只是文档”
静默省略流程。若用户要求仅调研、只看方案或暂不运行，遵守范围，并明确说明
“已加载技能／已完成准备，尚未启动引擎”。加载正文不等于执行成功。


- 沿用用户已经授权的需求、工作区、权限和交付范围。明确要求先看方案、暂不运行时，
  改用 `flywheel-new` 准备后交付审阅；不要用本入口越过该停止点。
- 事实自己查代码、配置与仓库指令；仅在缺失的产品取舍或目标身份会改变实现时询问。
  已明确授权实施的需求不追加机械的人审步骤。
- 当前需求已有实际任务目录时，核对它的 Spec、分支和状态；调用 `flywheel-run`
  恢复同一任务，不重新生成同名任务、复制状态、重置预算或再分配 packet 工作区。
- 确认 `flywheel control launch --help` 与 `flywheel template spec.schema.json` 可用。
  PATH 缺失时使用已安装的 `~/.config/flywheel/npm-launcher` 或用户明确配置的绝对路径。
  老版本没有 launch 时说明需要同版升级，不把 `--only execute` 当作 Quick。

## 2. 准备真实工作区与契约

1. 核对目标 Git 仓库、`git status --short`、实际分支、基线与
   `git worktree list --porcelain`。复用已明确属于本需求的任务集成 worktree；
   否则从核实的基线创建新分支与独立 worktree，不切换原 checkout，不强制覆盖。
   没有明确特殊选择时不从默认分支或 detached HEAD 直接运行开发任务。
2. 在该 worktree 中研究代码、仓库门禁与既有实现模式，明确目标、非目标、
   成功标准、风险和验收入口。准备使用当前对话模型；`loops.plan` 只约束引擎阶段。
   不固定再调用一个分类 Agent，不为小任务默认组建完整团队。
3. 读取二进制内嵌的 `spec.schema.json`、`task-packet.schema.json` 和相应 example，
   在 worktree 内生成 `docs/<slug>/spec.md` 与实际需要的 task packets。
   编号需求必须映射到 packet，验收用可执行命令；沿用仓库的覆盖率、集成和 UI
   门禁，不因 Quick 降低要求。详情与工作流字段示例见
   [契约准备](references/contract.md)。
4. 将所选 Git worktree 根解析为真实绝对路径 `repo_dir`，将其中的
   `docs/<slug>` 解析为真实绝对路径 `task_dir`。两者必须属于同一任务工作区。
   仅在 shell 中 `cd` 不会改变 DSH 会话目录，调用工具时显式传入这两个路径。
5. 按现有授权检查并提交本任务契约与冻结验收；不混入无关暂存文件或凭证。
   契约必须实际进入 Git，packet worktree 才能取得相同基线。若仓库要求独立提交授权
   且会话中没有该授权，先完成可审查契约，说明具体规则后再取得所需授权。

`flywheel.config.json` 仅在任务需要独立配置时创建；它是完整配置覆盖，并非局部合并。
保留已选的模型、provider、渠道和 `permissions.mode`。Quick／Full 不改变权限，
也不提供新的远端 push、PR、合并或发布授权。不要静默新增付费渠道。

## 3. 校验并启动一次

首次使用、升级或修改配置后可执行 `config inspect` 和本地 `doctor`，确认实际绑定。
预检不执行模型推理或本任务验收；通过不证明账号额度充足。真实阻断先处理，
不要把 unknown 翻译成通过，也不要在 skill 内静默切换模型。

在 DSH 优先调用模型工具，参数使用上一步解析的实际绝对路径：

```json
{
  "taskDir": "/absolute/task-worktree/docs/task-slug",
  "cwd": "/absolute/task-worktree",
  "mode": "auto"
}
```

把该对象传给 **`flywheel_launch`**。插件将具体任务关联到当前会话的 Flywheel 视图；
用户无需复制目录。DSH 已提供该工具时不要绕过它改走 Bash。没有 DSH 工具的宿主使用 CLI：

```bash
flywheel control launch --task-dir "$task_dir" --cwd "$repo_dir" --mode "$mode"
```

`mode` 只接受 `auto`、`quick`、`full`。Go 校验契约、作资格判断并保存生效策略；
证据不足时选择 Full，显式 Quick 也不能绕过资格。不要提前把宿主的建议说成已生效。
CLI 本身消费已准备的契约，不会用一句目标自动生成需求或创建外层任务分支；
这些工作由本 skill 的宿主准备阶段完成。

## 4. 解释回执与继续交付

启动后通过 `flywheel_control` 的 `status` 核对原 task_dir／repo_dir，确认画布关联。
若仅运行过 `--only review`／`verify` 等单阶段，明确报告该阶段和回执；不能把它当成
本入口已经完成全流程。启动或关联失败要给出具体回执，不用“任务已经创建”替代。


- 读取协议中的 `ok`、`running`、`state.status` 和工作流记录。`ok:true` 只表示
  控制请求成功；说明请求模式、生效策略及原因，不能把启动成功说成任务完成。
- 结果未确认、超时或断连时，先对**原 task_dir 与 repo_dir**调用
  `flywheel_control` 的 `status` 或 `control status`，查看原任务日志；不要重复 launch、
  换目录启动，或声称已停止。只在事实确认后继续操作。
- Quick 正常路径为 Execute、全部必需验证、一次独立最终 Review、Git 持久化；
  Plan／Judge 显示“不适用”及原因，不显示假通过。Full 保留完整阶段。
  引擎升级 Full 时转述具体原因；已有授权继续沿用，新需求取舍才交用户决定。
- Go 按冻结依赖和写入范围决定串行／并行调度及会话选择。scoped 可延续同角色、同作用域的正常终态历史，
  Execute／Review 始终隔离，复审仍检查完整当前候选；fresh 不启用通用历史复用。
  支持的主任务适配器在两种模式下均记录调用安全事实，未知调用不能靠换模式绕过。
  重试保留工作区成果；
  不需要一个长期父模型会话监督后台任务。关闭视图或准备对话不等于停止任务。
- 状态为 completed 后，核对同一候选的需求证据、测试、独立评审与实际 Git 交付，
  再按已有授权完成外层 push、PR、检查、合并或发布。控制台提供按明确仓库与 HEAD 授权的推送／PR／合并入口；引擎不会自动推断远端交付授权。
  原有仓库门禁仍适用；源码合并、npm 发布、Mac 上更新与真实账号验收分别说明。

后台观察用 `flywheel-status`，停止用 `flywheel-stop`，恢复用 `flywheel-run`。
保留失败成果；清理、派发或结果未知时报告阻断。子进程退出不代表结果已知；
从原路径交由 Go 核对回执，不用固定等待、新建任务或重置状态代替证明。
