---
name: flywheel-plan
description: Trigger only the Plan Loop for a Flywheel task (single state transition). Use when the user says "/skill:flywheel-plan docs/<slug>" or wants to re-run just the planning step.
compatibility: Requires the flywheel CLI (Node.js >=20; run "npm link" in the flywheel repo, or fall back to node <flywheel-repo>/bin/flywheel.js) and the harness CLIs the loops use: pi >=0.84 and/or kimi CLI.
whenToUse: 当用户只想重跑某任务的 Plan Loop 单步（修订/确认 packet 序列）时使用。
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

   未 `npm link` 时：`node <flywheel-repo>/bin/flywheel.js run docs/<slug> --only plan`（<flywheel-repo> = 本 skill 所在仓库根，可由 ${KIMI_SKILL_DIR} 向上两级定位；注意 ${KIMI_SKILL_DIR} 可能指向软链路径本身，建议优先 npm link）。

3. **翻译结果**：把输出的 JSON 摘要讲成人话——当前里程碑、Plan Loop 确认/
   修订了哪些 packet、state.json 的新 revision。失败时原样报告错误信息并
   建议下一步（看 `orchestrator.log` / 检查契约）。
