---
name: flywheel-plan
metadata:
  version: "1.8.2"
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

默认复用准备时选定的任务集成 checkout／分支。已有明确选择或授权不重复询问；
未明确选择的 `main`／`master`、已知默认分支或 detached HEAD 不能直接开跑。
新任务先按 `flywheel-new` 准备工作区；已有状态先核对原任务路径，不切分支、
复制／重置 state 或迁移已有成果。`task_dir` 与 `repo_dir` 必须属于同一 worktree。

## 步骤

1. **校验**：`docs/<slug>/` 存在且含 `spec.md` 和 task packets。确认所选运行配置来自任务、项目、全局或内嵌表，不强制每个任务都有自己的配置文件；状态由引擎初始化。
2. **调用**（在解析出的任务集成 checkout 运行，直到合法契约或有界失败）：

   ```bash
   cd "$repo_dir" && flywheel run "$task_dir" --only plan
   ```

   该入口要求 Full。原任务是 Quick 且用户明确要求改走规划时，追加
   `--mode full`，让 Go 持久化升级后运行；不要手改 Spec 或 state 来跳过策略检查。
   仅想完成现有 Quick 时使用 `flywheel-run`，合格任务不需要再调用本步骤。

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**：把输出的 JSON 摘要讲成人话——当前里程碑、Plan Loop 确认/
   修订了哪些 packet、state.json 的新 revision。失败时原样报告错误信息并
   建议下一步（看 `orchestrator.log` / 检查契约）。

Plan 的模型 CWD 是 `repo_dir`，可修订任务内 Spec／packet，不直接改实现或配置。
合法契约由引擎提交并尝试快进同步到已有 packet worktree；冲突时保留现场，
不得手工 reset、重建 packet 工作区或修改 `state.json`。这一步不执行实现／Review，
不创建或合并远端 PR；任务分支的 PR 操作由外层按已有授权执行。

Plan 历史由 Go 按任务／里程碑和 `sessions.mode` 管理；显式设置的
`resident-session:false` 继续关闭 Plan 历史复用，不在 skill 中改写它。
fresh 保留既有 Kimi Plan resident 行为，但不将它声明为通用角色会话。
出现清理、派发或结果未知时停止，不在 skill 中重试；确认子进程退出不能代替
可信结果回执。从原任务路径交由 Go 核对，不切模式或重置 state 绕过阻断。
Plan 不负责派发 packet 或决定执行目录隔离。后续由有效 `scheduler` 与
`worktree.enabled` 决定：自动管理开启的并行批次必须准备独立 Worktree，失败停止；
串行路径仍可能回退 `in-place`。关闭自动管理可使用任务 checkout 或已登记的外部
Worktree，不禁止并行；Plan 成功不证明后续执行一定隔离。

## 注意

- 全部里程碑都已关闭时 CLI 会报错 `no open milestone`——此时没有可规划的
  内容，直接告诉用户即可，不需要任何动作。

## DSH 画布关联

从 DSH 终端运行时，引擎使用宿主的会话标识登记运行；不要手写、复制或伪造这个标识。
首次运行、阶段结束或结果待核对时，再通过 `flywheel_control` 查询一次原任务：

```json
{"action":"status","taskDir":"<已核实的 task_dir 绝对路径>","cwd":"<已核实的 repo_dir 绝对路径>"}
```

这是只读查询，会将真实任务关联到当前会话画布；关联失败不重跑阶段，分别报告引擎
回执和关联结果。没有该工具时，明确给出原任务路径，
用户可从“最近任务”选择；不要声称 UI 已关联。单阶段通过只表示本次阶段通过，
不能说完整流程已完成；加载 Skill、创建任务资料与引擎实际运行必须分别报告。
