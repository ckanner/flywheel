---
name: flywheel-plan
metadata:
  version: "1.0.0"
description: Trigger only the Plan Loop for a Flywheel task (single state transition). Use when the user says "/skill:flywheel-plan docs/<slug>" or wants to re-run just the planning step.
whenToUse: 当用户只想重跑某任务的 Plan Loop 单步（修订/确认 packet 序列）时使用。
compatibility: "Requires the flywheel CLI (single static binary from the GitHub release, or make build + make install in the flywheel repo) and the harness CLIs the loops use: pi >=0.84 and/or kimi CLI."
arguments:
  - name: slug
    description: 任务目录路径，如 docs/<slug>
    required: true
---

# flywheel-plan — 只触发 Plan Loop

Button only: validate + invoke CLI + translate. No loop logic here.

## 步骤

1. **校验**：`docs/<slug>/` 存在且含 `spec.md`、`state.json`、`flywheel.config.json`。
2. **调用**（在目标仓库根执行，前台一步即返回）：

   ```bash
   flywheel run docs/<slug> --only plan
   ```

   若 flywheel 不在 PATH：`flywheel run docs/<slug> --only plan`（<flywheel-repo> 定位：install-skills 在 skills 安装点写了锚文件，`cat ${KIMI_SKILL_DIR}/../flywheel-home.txt` 即仓库根；锚文件不存在说明 skill 直接来自仓库内，${KIMI_SKILL_DIR} 向上两级即仓库根；都不适用就直接问用户要仓库路径）。
   （Go 版为单二进制：`make build && make install` 或经 npm 安装后 `flywheel` 即在 PATH。）

3. **翻译结果**：把输出的 JSON 摘要讲成人话——当前里程碑、Plan Loop 确认/
   修订了哪些 packet、state.json 的新 revision。失败时原样报告错误信息并
   建议下一步（看 `orchestrator.log` / 检查契约）。

## 注意

- 全部里程碑都已关闭时 CLI 会报错 `no open milestone`——此时没有可规划的
  内容，直接告诉用户即可，不需要任何动作。
