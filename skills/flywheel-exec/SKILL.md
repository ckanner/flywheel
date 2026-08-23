---
name: flywheel-exec
metadata:
  version: "1.0.0"
description: Trigger only the Execute Loop for a Flywheel task, optionally for one packet (single state transition). Use when the user says "/skill:flywheel-exec docs/<slug> [packet]" or wants to run just one execution unit.
whenToUse: 当用户只想手动跑某个执行单元（调试单个 packet、重跑失败单元）时使用。
compatibility: "Requires the flywheel CLI (single static binary from the GitHub release, or make build + make install in the flywheel repo) and the harness CLIs the loops use: pi >=0.84 and/or kimi CLI."
arguments:
  - name: slug
    description: 任务目录路径，如 docs/<slug>
    required: true
  - name: packet-id
    description: 可选，指定执行单元（如 M02-T03）；缺省取当前里程碑下一个未完成 packet
    required: false
---

# flywheel-exec — 只触发 Execute Loop

Button only: validate + invoke CLI + translate. No loop logic here.

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全。若给了 packet id（如 M02-T03），
   确认对应 `task-packet-*.md` 存在。
2. **调用**（在目标仓库根执行）：

   ```bash
   # 指定 packet
   flywheel run docs/<slug> --only execute --packet M02-T03
   # 或不指定：取当前里程碑下一个未完成 packet
   flywheel run docs/<slug> --only execute
   ```

   未 `npm link` 时：`node <flywheel-repo>/bin/flywheel.js run docs/<slug> --only execute [--packet M02-T03]`（<flywheel-repo> 定位：install-skills 在 skills 安装点写了锚文件，`cat ${KIMI_SKILL_DIR}/../flywheel-home.txt` 即仓库根；锚文件不存在说明 skill 直接来自仓库内，${KIMI_SKILL_DIR} 向上两级即仓库根；都不可用就问用户。建议优先 npm link）。

3. **翻译结果**：执行是否通过出口契约（进程终态/stderr/交付物三项独立判定）、
   done-report 是否合法、token 与成本增量、state 中该 packet 的新状态
   （executed / 失败与错误码）。提醒：execute 之后还有 L1 与 review，
   单元不算完成——完成由编排器与评审判定，不由执行者宣布。
