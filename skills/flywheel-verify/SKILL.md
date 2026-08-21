---
name: flywheel-verify
description: Trigger only the verification gates (L1/L2/L3) for a Flywheel task — no model involved, pure deterministic commands. Use when the user says "/skill:flywheel-verify docs/<slug> [packet]" or wants to re-run acceptance checks.
compatibility: Requires the flywheel CLI (Node.js >=20; run "npm link" in the flywheel repo, or fall back to node <flywheel-repo>/bin/flywheel.js). No harness CLI needed — verification is model-free.
whenToUse: 当用户只想重跑确定性验收（L1/L2/L3 命令，无模型参与）时使用。
arguments:
  - name: slug
    description: 任务目录路径，如 docs/<slug>
    required: true
  - name: packet-id
    description: 可选，只验该单元的 L1（如 M02-T03）；缺省跑全部未完成单元的 L1 + 当前里程碑闸门
    required: false
---

# flywheel-verify — 只触发验证闸门

Button only: validate + invoke CLI + translate. No loop logic
here — and no model either: verification is the orchestrator running commands.

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全。
2. **调用**（在目标仓库根执行）：

   ```bash
   # 只验某个 packet 的 L1
   flywheel run docs/<slug> --only verify --packet M02-T03
   # 或不指定：全部未完成 packet 的 L1 + 当前里程碑 L2/L3 闸门
   flywheel run docs/<slug> --only verify
   ```

   未 `npm link` 时：`node <flywheel-repo>/bin/flywheel.js run docs/<slug> --only verify [--packet M02-T03]`（<flywheel-repo> = 本 skill 所在仓库根，可由 ${KIMI_SKILL_DIR} 向上两级定位；注意 ${KIMI_SKILL_DIR} 可能指向软链路径本身，建议优先 npm link）。

3. **翻译结果**：每条命令的退出码与通过与否、git diff 边界判定
   （must-not-touch / 测试文件 / max-files）、里程碑闸门结论。任何红色都
   原样报告命令与输出摘要——命令是 ground truth，不替它找理由。
