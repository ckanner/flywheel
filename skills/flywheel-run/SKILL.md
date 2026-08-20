---
name: flywheel-run
description: Start the Flywheel orchestrator on a task (docs/<slug>) as an independent process — triggers ALL loops fully automatically. Use when the user says "/skill:flywheel-run docs/<slug>" or wants to start a flywheel task.
compatibility: Requires the flywheel CLI (Node.js >=20; run "npm link" in the flywheel repo, or fall back to node <flywheel-repo>/bin/flywheel.js) and the harness CLIs the loops use: pi >=0.84 and/or kimi CLI.
whenToUse: 当用户已人审契约、要全自动启动某个 flywheel 任务（"启动/跑这个任务"）时使用。
arguments:
  - name: slug
    description: 任务目录路径，如 docs/<slug>
    required: true
---

# flywheel-run — 拉起编排器（触发全部 loop）

This skill is only a button (DESIGN.md §13): validate params, launch the
orchestrator as an **independent process**, confirm it is alive, report the
status entry point. You do NOT do any loop work yourself — the real Plan
session is spawned by the orchestrator as a separate harness session.

## 步骤

1. **参数校验**：确认 `docs/<slug>/` 存在且含 `spec.md`、`task-packet-*.md`、
   `state.json`、`flywheel.config.json`。缺任何一样 → 停下，提示用户先走
   `/skill:flywheel-new` 或检查路径。确认用户已**人审契约**（冻结点）。
2. **以独立进程拉起**（不依赖当前会话存活）：

   ```bash
   cd <目标仓库根> && nohup flywheel run docs/<slug> \
     > docs/<slug>/orchestrator.log 2>&1 &
   ```

   未 `npm link` 时：`cd <目标仓库根> && nohup node <flywheel-repo>/bin/flywheel.js run docs/<slug> > docs/<slug>/orchestrator.log 2>&1 &`（<flywheel-repo> = 本 skill 所在仓库根，可由 ${KIMI_SKILL_DIR} 向上两级定位；注意 ${KIMI_SKILL_DIR} 可能指向软链路径本身，建议优先 npm link）。
3. **确认存活**：等 2 秒，检查进程仍在、`docs/<slug>/orchestrator.pid` 已写入、
   `state.json` 的 status 变为 running。拉起失败 → 读
   `docs/<slug>/orchestrator.log` 报告原因。
4. **翻译结果**，告诉用户：编排器已在后台运行（pid）、如何看进度
   （`/skill:flywheel-status docs/<slug>`）、如何停（`/skill:flywheel-stop docs/<slug>`）、
   日志位置。提醒：关掉本会话窗口不影响运行。

## 注意

- 编排器是外面的 runner，不住在本会话里；你只负责"拉起并确认"。
- 手动单步触发是别的按钮：`/skill:flywheel-plan / -exec / -review / -verify`。
