---
name: flywheel-review
version: 1.0.0
description: Trigger only the Review Loop for a Flywheel task, optionally for one unit (single state transition). Use when the user says "/skill:flywheel-review docs/<slug> [unit]" or wants to re-run just the review of a unit.
compatibility: Requires the flywheel CLI (Node.js >=20; run "npm link" in the flywheel repo, or fall back to node <flywheel-repo>/bin/flywheel.js) and the harness CLIs the loops use: pi >=0.84 and/or kimi CLI.
arguments:
  - name: slug
    description: 任务目录路径，如 docs/<slug>
    required: true
  - name: unit-id
    description: 可选，指定评审单元（如 M02-T03）；缺省取当前里程碑第一个未完成 packet（其 L1 未绿时 CLI 会报错，建议明确给 unit id）
    required: false
---

# flywheel-review — 只触发 Review Loop

Button only: validate + invoke CLI + translate. No loop logic here.

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全；目标单元的 **L1 已通过**
   （`state.json` 中该 packet 的 `evidence.l1.pass === true`）——评审只在确定性
   验收全绿之后开口。若给了 unit id，确认 packet 存在。
2. **调用**（在目标仓库根执行）：

   ```bash
   # 重跑某单元评审
   flywheel run docs/<slug> --only review --unit M02-T03
   # 或不指定：当前里程碑第一个未完成 packet（L1 未绿会报错——先 execute/verify）
   flywheel run docs/<slug> --only review
   ```

   未 `npm link` 时：`node <flywheel-repo>/bin/flywheel.js run docs/<slug> --only review [--unit M02-T03]`（<flywheel-repo> 定位：install-skills 在 skills 安装点写了锚文件，`cat ${KIMI_SKILL_DIR}/../flywheel-home.txt` 即仓库根；锚文件不存在说明 skill 直接来自仓库内，${KIMI_SKILL_DIR} 向上两级即仓库根；都不可用就问用户。建议优先 npm link）。

3. **翻译结果**：verdict（pass / return / verification-gap）、分数、理由；
   return 时转述具体修改要求、失败证据、涉及位置、完成条件；
   verification-gap 时说明编排器会加强验收后复验而不是退回执行。
