---
name: flywheel-status
description: Read-only progress report for a Flywheel task — reads state.json/retro.md/metrics and explains progress in plain language. Use when the user says "/skill:flywheel-status docs/<slug>" or asks how a flywheel task is doing.
compatibility: Requires the flywheel CLI (Node.js >=20; run "npm link" in the flywheel repo, or fall back to node <flywheel-repo>/bin/flywheel.js).
whenToUse: 当用户询问任务进度、成本、缓存命中率、是否停滞或有 inbox 待办时使用（只读，不改状态）。
arguments:
  - name: slug
    description: 任务目录路径，如 docs/<slug>
    required: true
---

# flywheel-status — 看进度（只读）

Button only: read state + translate. Writes nothing.

## 步骤

1. **校验**：`docs/<slug>/state.json` 存在（不存在 = 任务未生成或未启动）。
2. **调用**（在目标仓库根执行）：

   ```bash
   flywheel status docs/<slug>
   ```

   未 `npm link` 时：`node <flywheel-repo>/bin/flywheel.js status docs/<slug>`（<flywheel-repo> = 本 skill 所在仓库根，可由 ${KIMI_SKILL_DIR} 向上两级定位；注意 ${KIMI_SKILL_DIR} 可能指向软链路径本身，建议优先 npm link）。

3. **翻译结果**，用人话讲：
   - 任务状态、编排器是否存活（pid）、当前里程碑、上次推进时间——
     推进时间久未更新 = 可能停滞/断链，提醒用户。
   - 每个 packet：状态、尝试/失败次数、三级证据（executor 自报 < L1 确定性
     通过 < Review  verdict——证据等级不同，讲进度时区分）。
   - token/成本累计、缓存命中率（突降 = 前缀被破坏，提醒）、预算档与告警。
   - inbox 有待处理条目时**必须显著提醒**：这需要人。
   - 用户要更多细节时，可读 `docs/<slug>/retro.md`、`metrics.jsonl`、
     `orchestrator.log` 再讲。
