---
name: flywheel-plan
metadata:
  version: "1.3.1"
description: Run only the bounded Plan Loop for a Flywheel task until contracts validate or the retry limit is reached. Use when the user says "/skill:flywheel-plan docs/<slug>" or wants to re-run just the planning step.
whenToUse: 当用户只想重跑某任务的 Plan Loop 单步（修订/确认 packet 序列）时使用。
compatibility: "Requires the Flywheel CLI and the configured Plan harness; default Kimi CLI with subscription login and high thinking, without ACP."
arguments: slug
---

# flywheel-plan — 只触发 Plan Loop

Button only: validate + invoke CLI + translate. No loop logic here.

> 参数经 kimi 命名参数注入正文：`$slug`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 步骤

1. **校验**：`docs/<slug>/` 存在且含 `spec.md` 和 task packets。确认所选运行配置来自任务、项目、全局或内嵌表，不强制每个任务都有自己的配置文件；状态由引擎初始化。
2. **调用**（在目标仓库根执行，前台运行该子环直至合法契约或有界失败）：

   ```bash
   flywheel run "$slug" --only plan
   ```

   若 `flywheel` 不在 PATH，使用已配置的绝对二进制路径；未安装时按项目 README 安装所需版本。DSH 功能须使用包含该实现的构建；版本与安装步骤见项目 README，不能假定旧 npm 发布包已支持。

3. **翻译结果**：把输出的 JSON 摘要讲成人话——当前里程碑、Plan Loop 确认/
   修订了哪些 packet、state.json 的新 revision。失败时原样报告错误信息并
   建议下一步（看 `orchestrator.log` / 检查契约）。

## 注意

- 全部里程碑都已关闭时 CLI 会报错 `no open milestone`——此时没有可规划的
  内容，直接告诉用户即可，不需要任何动作。
