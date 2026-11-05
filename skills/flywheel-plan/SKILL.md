---
name: flywheel-plan
metadata:
  version: "1.6.0"
description: Run only the bounded Plan Loop for a Flywheel task until contracts validate or the retry limit is reached. Use for "/flywheel-plan docs/<slug>" in DSH, "/skill:flywheel-plan docs/<slug>" in Kimi, or a request to re-run just the planning step.
whenToUse: 当用户只想重跑某任务的 Plan Loop 单步（修订/确认 packet 序列）时使用。
compatibility: "Requires the Flywheel CLI and the configured Plan harness; default Kimi CLI with subscription login and high thinking, without ACP."
arguments: slug
---

# flywheel-plan — 只触发 Plan Loop

Button only: validate + invoke CLI + translate. No loop logic here.

DSH 在对话中使用 `/flywheel-plan <任务资料目录>`；Kimi 使用 `/skill:flywheel-plan <任务资料目录>`。

> 参数经 kimi 命名参数注入正文：`$slug`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 固定任务工作区

逐条解析，任一步失败就停止；不能用原终端或 DSH 会话目录代替任务所在仓库：

```bash
task_dir="$(cd "$slug" && pwd -P)"
repo_dir="$(git -C "$task_dir" rev-parse --show-toplevel)"
repo_dir="$(cd "$repo_dir" && pwd -P)"
task_branch="$(git -C "$repo_dir" branch --show-current)"
git -C "$repo_dir" worktree list --porcelain
```

默认复用准备时选定的任务集成 worktree／分支。已有明确选择或授权不重复询问；
未明确选择的 `main`／`master`、已知默认分支或 detached HEAD 不能直接开跑。
新任务先按 `flywheel-new` 准备工作区；已有状态先核对原任务路径，不切分支、
复制／重置 state 或迁移已有成果。`task_dir` 与 `repo_dir` 必须属于同一 worktree。

## 步骤

1. **校验**：`docs/<slug>/` 存在且含 `spec.md` 和 task packets。确认所选运行配置来自任务、项目、全局或内嵌表，不强制每个任务都有自己的配置文件；状态由引擎初始化。
2. **调用**（在解析出的任务集成 worktree 运行，直到合法契约或有界失败）：

   ```bash
   cd "$repo_dir" && flywheel run "$task_dir" --only plan
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**：把输出的 JSON 摘要讲成人话——当前里程碑、Plan Loop 确认/
   修订了哪些 packet、state.json 的新 revision。失败时原样报告错误信息并
   建议下一步（看 `orchestrator.log` / 检查契约）。

Plan 的模型 CWD 是 `repo_dir`，可修订任务内 Spec／packet，不直接改实现或配置。
合法契约由引擎提交并尝试快进同步到已有 packet worktree；冲突时保留现场，
不得手工 reset、重建 packet 工作区或修改 `state.json`。这一步不执行实现／Review，
不创建或合并远端 PR；任务分支的 PR 操作由外层按已有授权执行。

出现清理未确认时停止，不在 skill 中重试；确认所报告的子进程已结束后，仍从
原任务路径执行单环或使用 `flywheel-run` 恢复。默认 packet worktree 创建有
`in-place` 回退，不应把 Plan 成功解读成后续执行必然有隔离。

## 注意

- 全部里程碑都已关闭时 CLI 会报错 `no open milestone`——此时没有可规划的
  内容，直接告诉用户即可，不需要任何动作。
