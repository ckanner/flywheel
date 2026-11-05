---
name: flywheel-review
metadata:
  version: "1.6.0"
description: Trigger only the Review Loop for a Flywheel task, optionally for one unit (single state transition). Use when the user says "/skill:flywheel-review docs/<slug> [unit]" or wants to re-run just the review of a unit.
whenToUse: 当用户只想重跑某单元的 LLM 评审（L1 已绿之后）时使用。
compatibility: "Requires the Flywheel CLI and a Review harness supporting the configured permissions.mode; default DSH with the ChatGPT provider plugin."
arguments: slug unit

---

# flywheel-review — 只触发 Review Loop

Button only: validate + invoke CLI + translate. No loop logic here.

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
   验收全绿之后开口。若给了 unit id，确认 packet 存在。
2. **调用**（从任务集成 worktree 调用，按是否指定单元选择一条）：

   ```bash
   # 重跑某单元评审
   cd "$repo_dir" && flywheel run "$task_dir" --only review --unit "$unit"
   # 或不指定：当前里程碑第一个未完成 packet（L1 未绿会报错——先 execute/verify）
   cd "$repo_dir" && flywheel run "$task_dir" --only review
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**：verdict（pass / return / verification-gap）、分数、理由；
   return 时转述具体修改要求、失败证据、涉及位置、完成条件；
   verification-gap 时说明该单步只返回裁决，不会自动跨环修订；由用户运行
   Plan 修订允许范围内的契约，再 verify，或恢复完整 run 由引擎处理闭环。
   目录外冻结 oracle 的修改仍需明确任务；不得为了通过直接修改它。

Review 的实际 CWD 是引擎选定的 packet 候选工作区。核对 `permissions.mode`：
`role-based`（已有文件缺省）请求 `read-only`，`full-access` 通过所选后端正式接口
请求完全权限；Kimi print 为原生 auto，不提供 OS 级只读保证。两种模式都只做评审，
候选或契约在评审期间变化时，裁决不能通过身份检查。不要在 skill 中切进 packet 再启动，
也不要直接编辑候选。若此前执行使用 `in-place`，
或已完成单元的 worktree 已清理，引擎可能使用任务集成 checkout，不能声称总有
独立目录。Review pass 只记录裁决，本单步不提交／合并 packet，也不合并 GitHub PR。
完整运行的本地 `ff-only` 交付与外层获授权的 PR 合并分别处理。

清理未确认时保留现场并停止；确认相关子进程已结束后，使用原路径重新评审或
通过 `flywheel-run` 恢复，不在 skill 中自动重试或改 state。
