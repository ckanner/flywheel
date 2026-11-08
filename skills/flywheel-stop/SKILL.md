---
name: flywheel-stop
metadata:
  version: "1.7.0"
description: Stop the Flywheel orchestrator for a task and freeze its state. Use for "/flywheel-stop docs/<slug>" in DSH, "/skill:flywheel-stop docs/<slug>" in Kimi, or a request to halt a running flywheel task.
whenToUse: 当用户要停止正在运行的编排器（干预、预算控制、异常处理）时使用。
compatibility: "Requires the Flywheel CLI; stops the matching task through its managed control protocol."
arguments: slug
---

# flywheel-stop — 停止编排器

Button only: validate + invoke CLI + translate. No loop logic here.

DSH 在对话中使用 `/flywheel-stop <任务资料目录>`；Kimi 使用 `/skill:flywheel-stop <任务资料目录>`。

> 参数经 kimi 命名参数注入正文：`$slug`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 步骤

1. **定位原任务**：执行 `task_dir="$(cd "$slug" && pwd -P)"`，失败则报告路径问题。
   用户已要求停止时直接执行；不为停止创建 worktree、校验分支、要求干净工作区
   或切换 checkout。当前终端位于 `main`、detached HEAD 或仓库外都不妨碍停止
   原 `task_dir` 对应的运行，不重复索取停止授权（运行中的单元会被打断；
   进行中的单元会被标记 interrupted、产物保留，不会静默丢失——下次 run
   会先核对调用事实与证据，再决定是否能续跑）。
2. **调用**（使用绝对任务路径，不依赖当前工作目录）：

   ```bash
   flywheel stop "$task_dir"
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**：编排器进程是否被停掉（pid）、state.json 定格的状态与
   revision、当前里程碑进度。不得手写 state.json 或向 pid 文件中的进程直接发信号。
   超时无法确认停止时必须如实报告。`stopped` 只说明引擎生命周期结束；
   若记录了清理错误，不得声称所有子进程均已清理；`flywheel-run` skill
   可在问题处理后重新拉起继续。

停止不等于删除 packet worktree 或丢弃任务分支。保留现场，不复制／重置 state，
不借停止操作清理用户文件。恢复继续使用同一 `task_dir` 和任务集成 worktree；
`flywheel-run` 负责核对其分支与路径。清理未确认时先核实本次拥有的相关子进程
已结束；尤其不能只凭外层 worker 退出或固定等待几秒就启动下一轮。
子进程已结束也不证明已派发调用的结果已知；有未知调用时由 Go 核对可信终态回执，
保持阻断，不切换 fresh、另建任务或删除记录来绕过。
