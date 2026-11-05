---
name: flywheel-intake
metadata:
  version: "1.6.0"
description: Scan task sources (GitHub issues / custom commands) into the triage inbox and get LLM triage advice — candidates never become tasks without human approval. Use for "/flywheel-intake" in DSH, "/skill:flywheel-intake" in Kimi, or a request to discover new flywheel task candidates.
whenToUse: 当用户想扫描任务源（issue/CI 失败/依赖升级）发现候选任务、或刷新 triage 分诊建议时使用。
compatibility: "Requires the Flywheel CLI and configured Intake harness; default DSH DeepSeek Flash. GitHub issue sources also need gh."
arguments: slug source

---

# flywheel-intake — 扫任务源进 triage（外挂模块）

Button only: validate + invoke CLI + translate. No loop logic
here — and **no freezing**: Intake 只产候选与分诊建议，冻结需求永远走
`flywheel-new` + 人审。

DSH 在对话中使用 `/flywheel-intake <任务资料目录> [source]`；Kimi 使用 `/skill:flywheel-intake <任务资料目录> [source]`。

> 参数经 kimi 命名参数注入正文：`$slug`、`$source`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 固定建议来源与工作区

逐条解析实际任务路径，任一步失败就停止；扫描命令与建议模型都应看到同一仓库：

```bash
task_dir="$(cd "$slug" && pwd -P)"
repo_dir="$(git -C "$task_dir" rev-parse --show-toplevel)"
repo_dir="$(cd "$repo_dir" && pwd -P)"
task_branch="$(git -C "$repo_dir" branch --show-current)"
git -C "$repo_dir" worktree list --porcelain
```

已有任务默认复用其集成 worktree／分支；项目级候选扫描可沿用用户明确选定的仓库。
已有选择与授权不重复询问。若位于 `main`／`master`、已知默认分支或 detached HEAD
且归属尚不明确，先核对扫描目标，不凭原终端 CWD 启动。Intake 只给建议，不为
每个候选创建 packet worktree；候选获批成为正式 coding 任务时，再由 `flywheel-new`
创建或复用任务分支／worktree。不要为扫描迁移已有 state、切换分支或复制账本。

## 步骤

1. **校验**：`docs/<slug>/` 存在且 `flywheel.config.json` 里有
   `intake.sources` 配置。如果用户给了源名，确认它在 sources 里。
2. **调用**（从解析出的 `repo_dir` 运行，按源参数选择一条）：

   ```bash
   # 扫全部源
   cd "$repo_dir" && flywheel intake "$task_dir"
   # 只扫一个源
   cd "$repo_dir" && flywheel intake "$task_dir" --source "$source"
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**，用人话讲：
   - 扫到多少候选、多少是新的、多少因指纹重复被跳过；
   - 每条新候选的分诊建议：建议接 / 建议不接 / 需人判断 + 一句话理由；
   - 明确告诉用户：**候选不是任务**。要接的候选，到
     `docs/<slug>/triage-inbox.md` 把该条的 `- approval:` 改成 `approved`，
     然后用 `flywheel-new` 从 approved 候选生成契约（仍需人审冻结）。

本模块的 CWD 是 `repo_dir`，不会启动 Execute／L1／Review 或本地 packet 合并，
也不自动创建／合并 PR。核对 `permissions.mode`：`role-based`（已有文件缺省）下
DSH／Codex（含 Codex Pro）请求 `read-only`；`full-access` 通过所选后端正式接口
请求完全权限。Intake 仍只产候选与建议，不能因权限扩大自动实施。Kimi print 为原生
auto，不提供 OS 级只读保证；不能把“建议模块”或 worktree 当成安全沙箱。
PR 操作留给获授权的外层流程。
清理未确认会停止本轮后续候选；保留 inbox／metrics，确认相关子进程结束后再用
原路径处理后续任务，不在 skill 中重试、手写 state 或删除工作区。
