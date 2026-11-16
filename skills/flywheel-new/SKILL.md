---
name: flywheel-new
metadata:
  version: "2.0.0"
description: >-
  Register a thin Flywheel brief and start bounded design plus independent design review,
  then stop for human approval of the exact version. Use for "/flywheel-new <goal>" in DSH,
  "/skill:flywheel-new <goal>" in Kimi, or a request to discuss and review a design before implementation.
  The Go engine owns the design loop; the host does not prewrite a complete design or execution plan.
whenToUse: 用户希望先设计、讨论并审查方案，审查完成后自己确认再实施时使用；已有任务的设计修订用 flywheel-plan。
compatibility: "Requires matching Flywheel CLI/plugin with control planning and planning.schema.json, Git, and the configured design/review harnesses."
arguments: requirement mode
---

# flywheel-new — 建立 brief，启动方案循环

Button only：核实目标与工作区、登记简短 brief、调用一次 Go 控制入口、解释真实回执。
New 默认 `review-first`：后台做方案设计和独立 Design Review，结束后停在等待用户批准。
New 会启动**规划**，不会因此获得实施授权。技能不做设计／审查循环，不写 `state.json`。

DSH 使用 `/flywheel-new <目标> [mode=auto|quick|full]`，Kimi 使用
`/skill:flywheel-new`。`requirement` 是用户原始目标；mode 未给时是 `auto`。
已有本需求任务时复用其目录，改用 Plan 处理新反馈，不复制账本或再次 prepare。

## 1. 核实输入与工作区

- 读取仓库指令和足以定位目标的代码证据。技术事实自己查；只有影响范围、取舍或验收的
  意图确实缺失时才询问。保留用户原始请求，不把尚未解决的设计问题猜成已批准决定。
- Intake 来源只接受已经标为 approved 的候选。用户当前的直接请求本身是输入，
  不要求再次批准同一需求。New 的设计停止点是用户选择的工作方式。
- 用 `flywheel config inspect --cwd <原仓库绝对路径> --json` 查看现有配置。
  当前 DSH 对话只整理 brief；实际设计模型来自 `loops.design-plan`，未配置时
  继承 `loops.plan` 后端。复杂设计不要求宿主先完成整套架构或所有 packet。
- 检查 `git status --short`、分支、基线和 `git worktree list --porcelain`。
  复用明确属于本需求的任务集成 checkout。自动 worktree 管理开启而没有任务 checkout
  时，从核实基线创建独立任务分支/worktree；关闭时沿用所选目录，不自动另建。
  不混入用户 WIP，不覆盖已有分支，不因目录名相似就认定归属。

需要新 worktree 时先把变量设成已核实值，执行一次：

```bash
git -C "$source_repo" worktree add -b "$task_branch" "$task_worktree" "$base_ref"
```

然后逐条解析，失败就报告，不继续启动：

```bash
repo_dir="$(git -C "$task_worktree" rev-parse --show-toplevel)"
repo_dir="$(cd "$repo_dir" && pwd -P)"
task_branch="$(git -C "$repo_dir" branch --show-current)"
```

未明确选择的默认分支或 detached HEAD 不直接承载实施；已有明确选择与授权继续沿用。
在此 checkout 下创建 `docs/<slug>`，解析为真实绝对 `task_dir`。它必须位于 `repo_dir`
内。Provider 项目归属保留原仓库，不把 packet 或任务 worktree 注册成新项目。
只读核实与本入口准备均不新增 push、PR、合并、发布、账号或权限授权。

## 2. 准备薄 brief

读取 `flywheel template planning.schema.json` 的 `$defs.brief`。记录目标、原话、
已知约束和证据即可；暂不生成完整 Spec、全部 packets、审查通过或审批记录。
框架中的固定决定、可委派细节和未决问题由实际方案输出区分。

DSH 的 `flywheel_control` 参数示例（路径与内容替换为本任务事实）：

```json
{
  "action": "planning",
  "taskDir": "/absolute/task-worktree/docs/task-slug",
  "cwd": "/absolute/task-worktree",
  "planning": {
    "action": "prepare",
    "request-id": "brief-task-slug-1",
    "start": true,
    "brief": {
      "version": 1,
      "id": "task-slug",
      "goal": "为可靠队列设计可验证的恢复流程",
      "original-request": "用户原始请求；不要改写成已经批准方案",
      "constraints": ["先讨论并审阅方案，再决定是否实施"],
      "evidence": ["已读取的真实代码位置与约束；没有证据时用空数组"],
      "policy": "review-first",
      "implementation-authorized": false,
      "mode": "auto"
    }
  }
}
```

只调用一次；`start:true` 在这里启动规划。用户明确只登记输入、连设计模型也先不运行时，
可用 `start:false`，清楚说明尚未开始设计。不能把这个选择默认加到普通 New 上。
没有 DSH 工具时，将 `planning` 内层对象写入私有 JSON 文件，再交给 CLI：

```bash
flywheel control planning --task-dir "$task_dir" --cwd "$repo_dir" < "$request_file"
```

`request_file` 必须是本次已核实的输入文件；不得包含凭证。DSH 有工具时优先使用工具，
它会将原任务关联到当前 Flywheel 会话视图。缺少新接口时说明需要同版升级，
不要静默退回由宿主伪造完整方案的旧行为。PATH 缺失使用用户显式配置的 launcher；
首次安装按 `npx -y @ckanner/flywheel@latest install --json` 的真实结果处理。

## 3. 解释结果与停止点

- `ok:true` 只表示控制操作完成，`running` 表示后台引擎是否运行；方案进度看
  `state.planning.phase`。等待批准不是任务完成，也不是启动失败。
- 汇报原 task_dir、repo_dir、实际分支、当前设计 revision/digest、审查 verdict、
  未解决 findings、轮次和下一步。正文与审查可能仍在生成，不补造。
- `review-first` 通过独立审查后停下。用户可在同一个 Flywheel 视图提交反馈、比较上一版，
  然后用 Plan 修订。批准必须来自视图的真实用户按钮或 CLI 的交互终端。
  模型工具不暴露 approve；不能自报 `actor:user`、设置审批环境变量或代输入批准 digest。
- “批准此版本”只批准；“批准此版本并开始”同时授权并启动后续。仅批准后要开跑，
  用户点“开始已批准方案”。普通 Run 不把暂停实施改成授权。
- 超时、断连或结果未知，先对原目录调用 `flywheel_control status`，不要重复 prepare、
  新建任务、重置预算或删除状态。Go 独占重试、审查、审批和激活事务。
  规划角色的 `blocked` unknown 只能由用户在 Flywheel 视图或
  `flywheel session ... --abandon` 显式退役；模型工具不暴露它，不能代输确认，
  也不能把退役描述成成功、未派发或可重放。退役后仍需用户显式 Plan 生成新版本。

## 调查专用路径

用户只想先看故障取证计划时，仍可按当前 `preparation.schema.json` 准备有界调查，
用 `control investigate` 的 `action:start` 登记而不执行取证；后续明确授权后 Run。
不要为了套用设计协议，制造尚无根因的实施 Spec。直接“排查并修好”使用 Task 的
调查入口，保留原修复授权与证据边界。
