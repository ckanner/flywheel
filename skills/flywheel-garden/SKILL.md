---
name: flywheel-garden
metadata:
  version: "1.6.0"
description: Analyze retro.md failure patterns and knowledge-asset decay (AGENTS.md/skills), producing garden-proposals.md for human review — proposals only, never auto-applied. Use for "/flywheel-garden" in DSH, "/skill:flywheel-garden" in Kimi, or a request to distill loop learnings into skill proposals.
whenToUse: 当用户想把 retro 里反复出现的失败模式提炼成 skill/验收规则提案、或清理 AGENTS.md/skills 腐烂内容时使用。
compatibility: "Requires the Flywheel CLI and configured Garden harness; default Kimi CLI with subscription login."
arguments: slug
---

# flywheel-garden — 知识资产维护提案（外挂模块）

Button only: validate + invoke CLI + translate. No loop logic
here. Garden **只产提案，绝不自动应用**——改的是 loop 自己的知识资产，
应用权在人。

DSH 在对话中使用 `/flywheel-garden <任务资料目录>`；Kimi 使用 `/skill:flywheel-garden <任务资料目录>`。

> 参数经 kimi 命名参数注入正文：`$slug`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 固定提案工作区

逐条解析任务所在仓库，任一步失败即停止；不要从原终端 CWD 读取另一份知识资产：

```bash
task_dir="$(cd "$slug" && pwd -P)"
repo_dir="$(git -C "$task_dir" rev-parse --show-toplevel)"
repo_dir="$(cd "$repo_dir" && pwd -P)"
task_branch="$(git -C "$repo_dir" branch --show-current)"
git -C "$repo_dir" worktree list --porcelain
```

默认复用本任务的集成 worktree／分支，已有明确选择与授权不重复询问。
若 `main`／`master`、已知默认分支或 detached HEAD 的归属未确认，先核对提案对象；
不要在用户不知情时选用它。Garden 只分析项目知识资产并产出建议，不为每条建议
另建 packet worktree，也不迁移既有 state、切分支或自动应用提案。

## 步骤

1. **校验**：`docs/<slug>/` 存在。`retro.md` 为空时提醒用户：提案原料
   主要是 retro 积累，空 retro 也能跑（只做腐烂检查）但价值有限。
2. **调用**（从解析出的任务 `repo_dir` 执行）：

   ```bash
   cd "$repo_dir" && flywheel garden "$task_dir"
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**，用人话讲：
   - retro 中反复出现（≥2 次）的失败模式有哪些、各几次；
   - `docs/<slug>/garden-proposals.md` 里每条提案：类型（失败模式/腐烂清理）、
     证据引用、建议动作；
   - 提案校验未过（缺证据引用）时明确指出：原始输出仍在文件里，但证据不足
     的提案不该应用；
   - 提醒：审完提案后，由用户或获授权宿主在选定任务分支实施 AGENTS.md/skills/契约模板的修改；改完可考虑
     把高频教训沉淀进 `flywheel-new` 的生成习惯。

模型与知识资产读取的 CWD 是 `repo_dir`，提案由 Go 写入 `task_dir`。这不是
Execute 或 Review，不运行 packet 验收／本地 `ff-only` 合并，不自动推送或合并 PR。
获批修改按正式任务的 worktree、验收和 PR 流程处理，沿用已有授权。
核对 `permissions.mode`：`role-based`（已有文件缺省）下 DSH／Codex（含 Codex Pro）
请求 `read-only`；`full-access` 通过所选后端正式接口请求完全权限，仍只产提案、
不自动应用。默认 Kimi print 保持原生 auto，不提供可宣称的 OS 级只读沙箱；
不要把提案规则或 worktree 当成统一的安全隔离。清理未确认时保留记录并停止，
确认相关子进程结束后再从原任务路径继续，不用固定等待或手改 state 绕过。
