---
name: flywheel-review
metadata:
  version: "1.8.2"
description: Trigger only the Review Loop for a Flywheel task, optionally for one unit (single state transition). Use for "/flywheel-review docs/<slug> [unit]" in DSH, "/skill:flywheel-review docs/<slug> [unit]" in Kimi, or a request to re-run just the review of a unit.
whenToUse: 当用户只想重跑某单元的 LLM 评审（L1 已绿之后）时使用。
compatibility: "Requires the Flywheel CLI and a Review harness supporting the configured permissions.mode; default DSH with the ChatGPT provider plugin."
arguments: slug unit

---

# flywheel-review — 只触发 Review Loop

Button only: validate + invoke CLI + translate. No loop logic here.

DSH 在对话中使用 `/flywheel-review <任务资料目录> [unit]`；Kimi 使用 `/skill:flywheel-review <任务资料目录> [unit]`。

> 参数经 kimi 命名参数注入正文：`$slug`、`$unit`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 固定任务工作区

逐条解析，任一步失败就停止；不要从原终端目录推断评审的仓库：

```bash
task_dir="$(cd "$slug" && pwd -P)"
repo_dir="$(git -C "$task_dir" rev-parse --show-toplevel)"
repo_dir="$(cd "$repo_dir" && pwd -P)"
task_branch="$(git -C "$repo_dir" branch --show-current)"
git -C "$repo_dir" worktree list --porcelain
```

复用本任务的集成 worktree／分支和原 `task_dir`；已有明确选择与授权不重复询问。
未明确选择的 `main`／`master`、已知默认分支或 detached HEAD 先核对任务归属，
不直接开跑，不为评审另建工作区、切分支或迁移 state。新任务工作区在
`flywheel-new` 准备，已有 packet worktree 由引擎管理。

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全；目标单元的 **L1 已通过**
   （引擎会同时核对 `evidence.l1.pass` 和当前候选/契约/配置身份）——评审只在确定性
   验收全绿之后开口。Quick 还必须取得同一候选的适用 L2/L3；不能使用旧集成
   checkout 的结果。若给了 unit id，确认 packet 存在。
2. **调用**（从任务集成 worktree 调用，按是否指定单元选择一条）：

   ```bash
   # 重跑某单元评审
   cd "$repo_dir" && flywheel run "$task_dir" --only review --unit "$unit"
   # 或不指定：当前里程碑第一个未完成 packet（L1 未绿会报错——先 execute/verify）
   cd "$repo_dir" && flywheel run "$task_dir" --only review
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**：verdict（pass / return / verification-gap，Quick 另可为 upgrade）、分数、理由；
   return 时转述具体修改要求、失败证据、涉及位置、完成条件；
   verification-gap 时说明该单步不自动跨环修订；Quick 的 upgrade、
   verification-gap 或 `quick-eligible:false` 会由 Go 立即保存单向 Full 升级，
   下一次完整 run 才进入 Full。也可按明确范围单步 Plan 后 verify。
   目录外冻结 oracle 的修改仍需明确任务；不得为了通过直接修改它。

Quick 的本步骤是独立最终 Review：结构化 `requirements` 必须把全部 Spec 需求
各列一次，每项含 `id`、`pass`、非空 `evidence`；`quick-eligible` 表示资格是否仍成立。
仅通过代码风格评审不能交付。pass 还需全部需求成立且资格成立；upgrade 报告不再
适合 Quick 的事实。单环返回实际裁决并由 Go 保存必要的升级 checkpoint，
不在 skill 中自行重试、改策略或跨环规划；恢复完整 `flywheel-run` 后由 Go
处理返修、Full 阶段和 Git 交付，不能通过重跑评审将已升级任务改回 Quick。

Review 的实际 CWD 是引擎选定的 packet 候选工作区。scoped 模式可以复用同范围
Reviewer 自己的上下文，不继承 Executor 历史；每次仍完整检查当前候选、契约与适用
需求，不能仅关闭上轮 findings 或沿用旧 pass。会话选择和回执恢复由 Go 负责。
核对 `permissions.mode`：
`role-based`（已有文件缺省）请求 `read-only`，`full-access` 通过所选后端正式接口
请求完全权限；Kimi print 为原生 auto，不提供 OS 级只读保证。两种模式都只做评审，
候选或契约在评审期间变化时，裁决不能通过身份检查。不要在 skill 中切进 packet 再启动，
也不要直接编辑候选。若此前执行使用 `in-place`，
或已完成单元的 worktree 已清理，引擎可能使用任务集成 checkout，不能声称总有
独立目录。Review pass 只记录裁决，本单步不提交／合并 packet，也不合并 GitHub PR。
完整运行的本地 `ff-only` 交付与外层获授权的 PR 合并分别处理。

清理、派发或结果未知时保留现场并停止；子进程退出不证明裁决已知。通过原任务
路径让 Go 核对可信回执，不切 fresh、另建任务、自动重试或修改 state 来绕过阻断。
评审期间发生的候选修改不能在重启或重新验证后被解释成有效 pass。

## DSH 画布关联

从 DSH 终端运行时，引擎使用宿主的会话标识登记运行；不要手写、复制或伪造这个标识。
首次运行、阶段结束或结果待核对时，再通过 `flywheel_control` 查询一次原任务：

```json
{"action":"status","taskDir":"<已核实的 task_dir 绝对路径>","cwd":"<已核实的 repo_dir 绝对路径>"}
```

这是只读查询，会将真实任务关联到当前会话画布；关联失败不重跑阶段，分别报告引擎
回执和关联结果。没有该工具时，明确给出原任务路径，
不得声称 UI 已关联，也不要让用户寻找任务选择器。单阶段通过只表示本次阶段通过，
不能说完整流程已完成；加载 Skill、创建任务资料与引擎实际运行必须分别报告。
