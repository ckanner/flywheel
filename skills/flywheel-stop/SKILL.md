---
name: flywheel-stop
metadata:
  version: "1.2.0"
description: Stop the Flywheel orchestrator for a task and freeze its state. Use when the user says "/skill:flywheel-stop docs/<slug>" or wants to halt a running flywheel task.
whenToUse: 当用户要停止正在运行的编排器（干预、预算控制、异常处理）时使用。
compatibility: "Requires the flywheel CLI (npm)."
arguments: slug
---

# flywheel-stop — 停止编排器

Button only: validate + invoke CLI + translate. No loop logic here.

> 参数经 kimi 命名参数注入正文：`$slug`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

## 步骤

1. **校验**：`docs/<slug>/` 存在。与用户确认要停止（运行中的单元会被打断；
   进行中的单元会被标记 interrupted、产物保留，不会静默丢失——下次 run
   会续跑）。
2. **调用**（在目标仓库根执行）：

   ```bash
   flywheel stop "$slug"
   ```

   若 flywheel 不在 PATH：`flywheel stop docs/<slug>`（flywheel 应已在 PATH 上——npm 全局安装或开发机 go install；不在则先安装，不要绕道源码仓库）。
   （单二进制：`npm i -g @ckanner/flywheel` 后 `flywheel` 即在 PATH；不在则先安装，不要从源码仓库构建。）

3. **翻译结果**：编排器进程是否被停掉（pid）、state.json 定格的状态与
   revision、当前里程碑进度。告诉用户：干预还可以通过改 state.json /
   写 inbox.md 完成——全是文件操作；`/skill:flywheel-run docs/<slug>` 可随时
   重新拉起继续。
