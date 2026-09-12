---
name: flywheel-plan
metadata:
  version: "1.2.0"
description: Trigger only the Plan Loop for a Flywheel task (single state transition). Use when the user says "/skill:flywheel-plan docs/<slug>" or wants to re-run just the planning step.
whenToUse: 当用户只想重跑某任务的 Plan Loop 单步（修订/确认 packet 序列）时使用。
compatibility: "Requires the flywheel CLI (npm) and the kimi CLI with subscription login (Plan runs Kimi K3, max thinking)."
arguments: slug
---

# flywheel-plan — 只触发 Plan Loop

Button only: validate + invoke CLI + translate. No loop logic here.

> 参数经 kimi 命名参数注入正文：`$slug`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

## 步骤

1. **校验**：`docs/<slug>/` 存在且含 `spec.md`、`state.json`、`flywheel.config.json`。
2. **调用**（在目标仓库根执行，前台一步即返回）：

   ```bash
   flywheel run "$slug" --only plan
   ```

   若 flywheel 不在 PATH：`flywheel run docs/<slug> --only plan`（flywheel 应已在 PATH 上——npm 全局安装或开发机 go install；不在则先安装，不要绕道源码仓库）。
   （单二进制：`npm i -g @ckanner/flywheel` 后 `flywheel` 即在 PATH；不在则先安装，不要从源码仓库构建。）

3. **翻译结果**：把输出的 JSON 摘要讲成人话——当前里程碑、Plan Loop 确认/
   修订了哪些 packet、state.json 的新 revision。失败时原样报告错误信息并
   建议下一步（看 `orchestrator.log` / 检查契约）。

## 注意

- 全部里程碑都已关闭时 CLI 会报错 `no open milestone`——此时没有可规划的
  内容，直接告诉用户即可，不需要任何动作。
