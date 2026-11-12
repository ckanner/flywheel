---
name: flywheel-exec
metadata:
  version: "1.9.4"
description: Run the bounded Execute Loop for a Flywheel task, optionally for one packet, until L1 passes or a failure limit is reached. Use for "/flywheel-exec docs/<slug> [packet]" in DSH, "/skill:flywheel-exec docs/<slug> [packet]" in Kimi, or a request to run just one execution unit.
whenToUse: 当用户只想手动跑某个执行单元（调试单个 packet、重跑失败单元）时使用。
compatibility: "Requires Flywheel >=0.28.0 and configured route harnesses: DSH Flash for default, Kimi CLI for frontend, the bundled Codex Pro machine worker for hard tasks."
arguments: slug packet

---

# flywheel-exec — 只触发 Execute Loop

Button only: validate + invoke CLI + translate. No loop logic here.

DSH 在对话中使用 `/flywheel-exec <任务资料目录> [packet]`；Kimi 使用 `/skill:flywheel-exec <任务资料目录> [packet]`。

> 参数经 kimi 命名参数注入正文：`$slug`、`$packet`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 固定任务工作区

逐条解析，任一步失败就停止；不凭原终端或 DSH 会话目录启动：

```bash
task_dir="$(cd "$slug" && pwd -P)"
repo_dir="$(git -C "$task_dir" rev-parse --show-toplevel)"
repo_dir="$(cd "$repo_dir" && pwd -P)"
task_branch="$(git -C "$repo_dir" branch --show-current)"
git -C "$repo_dir" worktree list --porcelain
```

默认复用准备时选定的任务集成 checkout／分支，核对路径和实际分支；已有选择与
授权不重复询问。未明确选择的 `main`／`master`、已知默认分支或 detached HEAD
不能直接开跑。新任务先按 `flywheel-new` 准备工作区；已有状态使用原路径，
不切分支、复制／重置 state 或替换已有 packet worktree。

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全。若给了 packet id（如 M02-T03），
   确认对应 `task-packet-*.md` 存在。
   首次运行或升级后用 `flywheel config inspect --cwd "$repo_dir" --task-dir "$task_dir" --json`
   查看有效路由。0.28.0 的 hard 默认是 `codex-pro`，直接使用已安装插件的 machine API；
   旧配置不自动迁移。不要在 skill 中改用 `codex exec` 或启动父模型再次委派，
   也不要把 `subagent_codex_pro` 写成 DSH provider。
   显式选择 `claude-code-pro` 时核对 doctor 的配套 0.4.0 machine API 与 full-access
   前提；它也是独立 harness，不能在 skill 中改用 `subagent_claude_code` 再委派。
   未决定使用场景时保留原 routes，不因为存在可选 profile 自动切换模型。
2. **调用**（按是否指定 packet 选择一条，不进入 packet 目录另启动引擎）：

   ```bash
   # 指定 packet
   cd "$repo_dir" && flywheel run "$task_dir" --only execute --packet "$packet"
   # 或不指定：取当前里程碑下一个未完成 packet
   cd "$repo_dir" && flywheel run "$task_dir" --only execute
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**：该子环内的执行尝试、done-report、L1 验收与失败反馈；
   成功会报告 `L1 green (execute loop closed)`，不是只生成代码就返回。
   失败区分预算／凭证停止、尝试次数用尽、相同失败空转和实际错误。
   token／成本缺失要说明未知，不能记成免费。提醒：还需要独立 Review
   和引擎持久化交付；该命令不会自动运行 Review 或把单元标为 completed。
   达到失败出口后由用户选择修订 Plan 或重试，不在 skill 中另建重试循环。

`--only execute` 不是 Quick。Quick 的完整运行还必须在同一候选上收齐适用的
L2/L3，完成独立最终 Review 与 Git 持久化；继续闭环使用 `flywheel-run`，
新需求的自动准备启动使用 `flywheel-task`。

Execute 与 L1 使用引擎记录的 packet 实际候选目录；重试复用成果。
`sessions.mode=scoped` 时，Go 可在可信正常终态后延续同 packet 的 Executor 历史；
fresh 不启用通用历史复用，两种模式均保留支持适配器的调用安全记录。不要自行选择
会话 ID 或在 skill 中重试。done-report 先写实际候选目录，再由引擎校验、归档。
自动管理开启时由引擎创建／复用 packet Worktree，不手工替换或重置它。关闭时可使用
任务 checkout，也允许用户／模型在实现前选择并登记合格的外部 Worktree；
不把关闭开关解释为禁止 Worktree 或禁止并行，不混用不同 checkout 的账本。
本入口只执行一个 packet，不会因为配置允许并行就启动其他单元。
串行路径创建失败可能回退 `in-place`；完整流程中自动管理开启的并行批次则停止派发。
按引擎回执说明实际位置，严格隔离任务遇到原地执行应停止并处理，重启不证明隔离已恢复。

清理、派发或结果未知时保留成果和审计并报告阻断。确认子进程结束不证明调用结果
已知；从原任务路径交由 Go 核对回执，不切 fresh、换目录或重置 state 来绕过。
不用固定等待代替确认。P3 的只读原调用恢复和 P4 的有界重建均由 Go 负责；
只有原候选证据完整的回执能推进执行，质量阈值不能解除未知调用阻断。不要删除
`.sessions`、直接重发 prompt 或在 skill 中自行压缩历史。
完整运行通过 Review 等门禁后，引擎才交付到任务集成分支；共享目录批次还须全部成员
通过同一候选的验收。PR 的推送、创建和最终合并由外层按已有授权处理，
本单环不自动执行这些远端操作。

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
