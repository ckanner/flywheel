---
name: flywheel-review
metadata:
  version: "1.7.0"
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
