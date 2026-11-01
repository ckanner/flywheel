---
name: flywheel-stop
metadata:
  version: "1.3.1"
description: Stop the Flywheel orchestrator for a task and freeze its state. Use when the user says "/skill:flywheel-stop docs/<slug>" or wants to halt a running flywheel task.
whenToUse: 当用户要停止正在运行的编排器（干预、预算控制、异常处理）时使用。
compatibility: "Requires the Flywheel CLI; stops the matching task through its managed control protocol."
arguments: slug
---

# flywheel-stop — 停止编排器

Button only: validate + invoke CLI + translate. No loop logic here.

> 参数经 kimi 命名参数注入正文：`$slug`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 步骤

1. **校验**：`docs/<slug>/` 存在。用户已要求停止时直接执行（运行中的单元会被打断；
   进行中的单元会被标记 interrupted、产物保留，不会静默丢失——下次 run
   会续跑）。
2. **调用**（在目标仓库根执行）：

   ```bash
   flywheel stop "$slug"
   ```

   若 `flywheel` 不在 PATH，使用已配置的绝对二进制路径；未安装时按项目 README 安装所需版本。DSH 功能须使用包含该实现的构建；版本与安装步骤见项目 README，不能假定旧 npm 发布包已支持。

3. **翻译结果**：编排器进程是否被停掉（pid）、state.json 定格的状态与
   revision、当前里程碑进度。不得手写 state.json 或向 pid 文件中的进程直接发信号。
   超时无法确认停止时必须如实报告。`stopped` 只说明引擎生命周期结束；
   若记录了清理错误，不得声称所有子进程均已清理；`/skill:flywheel-run docs/<slug>`
   可在问题处理后重新拉起继续。
