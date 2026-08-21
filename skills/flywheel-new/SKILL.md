---
name: flywheel-new
version: 1.0.0
description: Generate a Flywheel task from a requirement — conversational intake that researches the repo, asks intent questions, generates docs/<slug>/ (spec.md + task packets + state.json), then STOPS for human review. Use when the user says "把 XX 需求做成 flywheel 任务" or wants to create a new flywheel task.
compatibility: Requires write access + git in the target repository; generated contracts follow the schemas in the flywheel repo templates/.
arguments:
  - name: requirement
    description: 一句话需求描述（自然语言），或 triage-inbox 中 approved 候选的指纹/标题
    required: true
---

# flywheel-new — 任务生成器

You are the task generator. You turn a **frozen-intent** requirement into Flywheel
contract files. You contain no loop logic — the orchestrator runs the loops later.

## 问答纪律（硬规则）

- **意图类的问人，事实类的自己查证。** Never ask the user about technical facts:
  read the code, run commands, verify — and write the evidence into the spec.
  Never guess about intent: if the requirement is ambiguous, STOP and ask before
  generating anything.
- 技术事实不许问人；意图歧义不许猜。

## 流程

1. **调研仓库**：读相关代码/文档，确认需求对应的真实位置与约束；把关键事实
   （含你验证过的命令与输出）记下来。若任务来自 Intake：可读
   `docs/<slug>/triage-inbox.md`，**只从标记为 approved 的候选生成契约**
   （未 approved 的候选不是需求，提醒用户先确认）。
2. **问意图**：只问意图类问题（范围、取舍、优先级、验收标准的主观部分）。
   一次问清，不要挤牙膏。
3. **生成契约**：在目标仓库创建 `docs/<slug>/`：
   - `spec.md` — 用 `flywheel 仓库/templates/spec.example.md` 的格式（flywheel 仓库根定位：`cat ${KIMI_SKILL_DIR}/../flywheel-home.txt`，或 skill 直接来自仓库内时向上两级）：编号需求
     R-01…R-n（每项可独立验证）+ 需求→里程碑映射 + 里程碑表。
   - `task-packet-<id>.md` — 每个执行单元一个，frontmatter 遵守
     `templates/task-packet.schema.json`：packet-id（M<NN>-T<NN>）、
     contract-version、goal、requirements（引用 R 编号）、route
     （frontend|default|hard）、needs-live-credentials（默认 false；需要 live
     凭证的任务标 true 并提醒用户需人工确认）、context-files、cache-scope、
     boundaries、acceptance.L1/L2/L3（验收必须是**可执行命令**，覆盖率闸门也
     是一条 L1 命令）、review-rubric、escalation、done-report-format。
   - `state.json` — 初始状态，遵守 `templates/state.schema.json`
     （revision 0，status pending，全部 packet pending）。
   - `flywheel.config.json` — 从 `templates/flywheel.config.example.json`
     复制并按需调整。
   - `.gitignore` — 任务目录级，内容三行：`orchestrator.log`、
     `orchestrator.pid`、`metrics.jsonl`（运行时易变产物；state.json/
     retro.md/inbox.md/reports/ 属于契约层，**不要**忽略）。
   - **随 packet 提供验收测试**：executor 不写/不改任何测试文件，
     验收测试由你（任务生成者，之后是 Plan Loop）编写并随契约提交。
4. **提交契约文件（硬规则）**：在目标仓库执行
   `git add docs/<slug>` 并 `git commit`（conventional 风格 message，如
   `docs(<slug>): add flywheel task contracts`）。契约层文件全部是已提交
   内容——未提交的契约对编排器与 worktree 不可见。
5. **停下来等人审（关键 checkpoint，不可省）**：草稿生成并提交后 STOP。明确告诉用户：
   请审 spec 与 packet 的需求编号、验收命令、边界。**冻结点 = 人审契约**——
   不要求源素材预先定稿；用户审过契约即冻结，此后范围变更只能走 Plan Loop。
   不要主动启动 `/skill:flywheel-run`。
