---
name: flywheel-verify
metadata:
  version: "1.7.2"
description: Trigger only the verification gates (L1/L2/L3) for a Flywheel task — no model involved, pure deterministic commands. Use for "/flywheel-verify docs/<slug> [packet]" in DSH, "/skill:flywheel-verify docs/<slug> [packet]" in Kimi, or a request to re-run acceptance checks.
whenToUse: 当用户只想重跑确定性验收（L1/L2/L3 命令，无模型参与）时使用。
compatibility: "Requires the Flywheel CLI. No model call; verification commands run under the engine watchdog."
arguments: slug packet

---

# flywheel-verify — 只触发验证闸门

Button only: validate + invoke CLI + translate. No loop logic
here — and no model either: verification is the orchestrator running commands.

DSH 在对话中使用 `/flywheel-verify <任务资料目录> [packet]`；Kimi 使用 `/skill:flywheel-verify <任务资料目录> [packet]`。

> 参数经 kimi 命名参数注入正文：`$slug`、`$packet`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 固定任务工作区

验收命令会实际执行代码，先逐条解析任务路径；任一步失败即停止：

```bash
task_dir="$(cd "$slug" && pwd -P)"
repo_dir="$(git -C "$task_dir" rev-parse --show-toplevel)"
repo_dir="$(cd "$repo_dir" && pwd -P)"
task_branch="$(git -C "$repo_dir" branch --show-current)"
git -C "$repo_dir" worktree list --porcelain
```

复用本任务原有的集成 worktree／分支，不根据原终端 CWD 另选 checkout。
已有明确选择与授权不重复询问；未明确选择的 `main`／`master`、已知默认分支或
detached HEAD 先核对，不直接执行验收。新任务工作区由 `flywheel-new` 准备；
已有 state／候选成果不迁移、不重置，packet worktree 由引擎管理。

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全。
2. **调用**（从任务集成 worktree 调用，按是否指定 packet 选择一条）：

   ```bash
   # 指定 packet；Full 验 L1，Quick 同时收集候选上的适用 L2/L3
   cd "$repo_dir" && flywheel run "$task_dir" --only verify --packet "$packet"
   # 或不指定：未完成 packet 的 L1 + 当前策略要求的 L2/L3
   cd "$repo_dir" && flywheel run "$task_dir" --only verify
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**：每条命令的退出码与通过与否、git diff 边界判定
   （must-not-touch / protected-test-patterns / max-files）、里程碑闸门结论。任何红色都
   原样报告命令与输出摘要——命令是 ground truth，不替它找理由。

## 注意

- Full 不带 packet 的 verify 覆盖**所有里程碑**里未完成 packet 的 L1，不只当前
  里程碑；L2/L3 闸门仍只跑当前里程碑。Quick 收齐唯一 packet 的适用 L2/L3，
  不把选中 packet 当成跳过这些检查的理由。
- L1 在引擎选定的 packet 候选工作区运行；Full 的里程碑 L2/L3 在 `repo_dir`，
  Quick 的 L2/L3 在实际候选工作区，且证据绑定最终 Review 将读取的候选。本命令
  不创建 packet worktree；若没有可用 worktree 会使用任务集成 checkout，
  严格隔离任务应先核实候选位置，不能把 verify 当作隔离创建步骤。
- 本单步只验收并记录证据，不运行 Review／Judge，不完成 packet 本地合并，
  也不推送、创建或合并 PR；这些操作分别由完整引擎流程和获授权宿主承担。
- 超时／清理未确认必须如实报告；确认相关验收子进程已结束后再从原路径重跑，
  不用 sleep 代替确认，不手写 state 或自动删除工作区。

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
