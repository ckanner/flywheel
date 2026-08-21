---
name: flywheel-stop
version: 1.0.0
description: Stop the Flywheel orchestrator for a task and freeze its state. Use when the user says "/skill:flywheel-stop docs/<slug>" or wants to halt a running flywheel task.
compatibility: Requires the flywheel CLI (Node.js >=20; run "npm link" in the flywheel repo, or fall back to node <flywheel-repo>/bin/flywheel.js).
arguments:
  - name: slug
    description: 任务目录路径，如 docs/<slug>
    required: true
---

# flywheel-stop — 停止编排器

Button only: validate + invoke CLI + translate. No loop logic here.

## 步骤

1. **校验**：`docs/<slug>/` 存在。与用户确认要停止（运行中的单元会被打断；
   进行中的单元会被标记 interrupted、产物保留，不会静默丢失——下次 run
   会续跑）。
2. **调用**（在目标仓库根执行）：

   ```bash
   flywheel stop docs/<slug>
   ```

   未 `npm link` 时：`node <flywheel-repo>/bin/flywheel.js stop docs/<slug>`（<flywheel-repo> 定位：install-skills 在 skills 安装点写了锚文件，`cat ${KIMI_SKILL_DIR}/../flywheel-home.txt` 即仓库根；锚文件不存在说明 skill 直接来自仓库内，${KIMI_SKILL_DIR} 向上两级即仓库根；都不可用就问用户。建议优先 npm link）。

3. **翻译结果**：编排器进程是否被停掉（pid）、state.json 定格的状态与
   revision、当前里程碑进度。告诉用户：干预还可以通过改 state.json /
   写 inbox.md 完成——全是文件操作；`/skill:flywheel-run docs/<slug>` 可随时
   重新拉起继续。
