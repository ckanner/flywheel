---
name: flywheel-plan
metadata:
  version: "2.0.0"
description: >-
  Run Go-owned design and independent review for a reviewed-planning task, or refine an
  explicit milestone or packet within its approved root design. Plan stops after planning;
  it never automatically starts implementation. Existing legacy tasks retain their milestone
  Plan semantics. Use for "/flywheel-plan <task-dir> [scope=design|milestone|packet|replan] [id=<scope-id>]"
  in DSH or the corresponding Kimi skill.
whenToUse: 用户想进行整体方案设计、根据反馈修订方案，或细化具体里程碑/工作单元时使用；只运行规划和审查，完成后停下。
compatibility: "Requires matching Flywheel CLI/plugin with control planning for new tasks; legacy run --only plan remains supported."
arguments: slug scope id
---

# flywheel-plan — 设计、审查与逐步细化

Button only：读取原任务、核对目标范围、调用一次 Go 入口、翻译结果。
技能不写方案循环、不写 `state.json`，不自行选择 Provider session，不代替用户批准。
DSH 使用 `/flywheel-plan <任务资料目录> [scope=…] [id=…]`；Kimi 使用 `/skill:flywheel-plan`。
`slug` 是已经存在的路径，不自动再拼接 `docs/`。

## 1. 定位原任务

逐条解析，失败就停止；不借用其他 checkout 的账本：

```bash
task_dir="$(cd "$slug" && pwd -P)"
repo_dir="$(git -C "$task_dir" rev-parse --show-toplevel)"
repo_dir="$(cd "$repo_dir" && pwd -P)"
task_branch="$(git -C "$repo_dir" branch --show-current)"
git -C "$repo_dir" worktree list --porcelain
flywheel control status --task-dir "$task_dir" --cwd "$repo_dir"
```

DSH 优先用 `flywheel_control({action:"status",taskDir,cwd})` 取得同一回执并关联视图。
复用准备时选定的任务分支；不切分支、迁移状态、重建 packet 目录或重置预算。
没有明确特殊选择时，默认分支或 detached HEAD 不作为新的实施任务位置。

## 2. 新规划协议：默认整体方案

有 `state.planning.version:1` 时，读取实际 `state.revision`、当前设计引用和未决 findings。
不要求初始已有 spec.md 或 packets；它们由引擎在批准与子计划审查后激活。

| scope | 用途 | id |
|---|---|---|
| `design`（缺省） | 真正的整体设计、修订和独立 Design Review | 省略 |
| `milestone` | 在已批准根方案内细化一个里程碑 | 实际里程碑 ID，例如 M-01 |
| `packet` | 在已批准根方案内细化一个工作单元 | 实际 packet ID，例如 M01-T01 |
| `replan` | 根据新证据重新规划；仍沿用原 scope 的版本和审查历史 | 省略或 root → design；M-01 → milestone；M01-T01 → packet |

用户只补充本次细化要求（例如“展开 M-01 的崩溃恢复步骤”）时，把原意放进
`action:plan` 的可选 `text`；Go 将其保留为当前 request 的不可变说明，同时提供给
Planner 和 Reviewer。它不改变全局输入版本，也不撤销根方案批准；子计划仍须通过
原范围审查。不要将局部说明误发为全局反馈。

用户改变整体目标、固定决定或实施范围时，先提交一个有 `revision`、稳定 `request-id`、`text` 和当前 `design`
引用的 `action:feedback`，`start:false`。反馈只有回执 `applied` 才算采用；`queued`
表示等待现有 owner 处理。排队时报告真实状态，不在技能里轮询或叠加 Plan。
已应用后使用最新状态版本触发一次 Plan；不能把反馈文字直接当作新批准。

DSH 的模型工具参数示例：

```json
{
  "action": "planning",
  "taskDir": "/absolute/task-worktree/docs/task-slug",
  "cwd": "/absolute/task-worktree",
  "planning": {
    "action": "plan",
    "revision": 7,
    "request-id": "plan-task-slug-2",
    "scope": "design",
    "start": true
  }
}
```

`revision:7` 只是格式示例，必须换成刚读取的实际版本；scope / scope-id 来自任务事实。
没有 DSH 工具时将内层 JSON 写入私有文件并调用：

```bash
flywheel control planning --task-dir "$task_dir" --cwd "$repo_dir" < "$request_file"
```

此 action 以 planning-only 运行：即使已有实施授权，显式 Plan 结束后也停止，
不会顺带开始 Execute。Go 负责有界设计/审查循环、独立角色调用、用量、scope checks
与状态保存。人讨论通过反馈进入下一轮，审查 pass 不等于人已批准。

## 3. 旧任务兼容

状态没有 planning 字段、只有旧 Spec/packets 时，继续旧里程碑 Plan：

```bash
cd "$repo_dir" && flywheel run "$task_dir" --only plan
```

旧路径要求 Full。已有 Quick 且用户明确改走完整规划时才加 `--mode full`；
不要手改 workflow/state。所有里程碑已关闭的 `no open milestone` 如实报告。
不能仅因升级二进制就自动迁移旧账本。用户明确要求迁移时，用新 `control planning`
的 `action:migrate`，绑定当前 revision、稳定 request-id 和明确 policy；
解释迁移将进入新设计审查。迁移前后的旧实施证据与预算由 Go 保留。

## 4. 翻译回执

汇报整体设计或子计划的精确 revision/digest、独立 review verdict、findings、
轮次及停止原因。根方案与子计划分别说明，不能把子计划审查冒充批准新的根方案。
`awaiting-design-approval` 时让用户在 Flywheel 视图选择“批准此版本”或“批准此版本并开始”；
CLI 由真实交互终端确认。模型工具不能 approve，也不能通过环境变量伪造人源。
`needs-user-decision` 说明需要哪项取舍；unknown、清理未确认、预算停止保留原回执，
先查询原任务，禁止在技能内重试或切 fresh 绕过。Plan 成功不代表代码验收或 PR 完成。
继续既有实施用 Run；已批准但未授权启动时用视图“开始已批准方案”。
