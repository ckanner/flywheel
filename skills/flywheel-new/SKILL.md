---
name: flywheel-new
metadata:
  version: "1.4.0"
description: Generate a Flywheel task from a requirement — conversational intake that researches the repo, asks intent questions, generates docs/<slug>/ (spec.md + task packets + configuration), then STOPS for human review. Use when the user says "把 XX 需求做成 flywheel 任务" or wants to create a new flywheel task.
whenToUse: 当用户想把一个需求冻结成 flywheel 任务契约（"把 XX 做成 flywheel 任务"）时使用；也可从 triage-inbox 的 approved 候选生成。
compatibility: "Requires the Flywheel CLI with embedded contract templates, write access, and Git in the target repository."
arguments: requirement
---

# flywheel-new — 任务生成器

You are the task generator. You turn a **frozen-intent** requirement into Flywheel
contract files. You contain no loop logic — the orchestrator runs the loops later.

> 参数经 kimi 命名参数注入正文：`$requirement`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

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
   - `spec.md` — 用 `flywheel template spec.example.md` 输出的内嵌模板格式（二进制自带，无需源码仓库）：编号需求
     R-01…R-n（每项可独立验证）+ 需求→里程碑映射 + 里程碑表。
   - `task-packet-<id>.md` — 每个执行单元一个，frontmatter 遵守
     `flywheel template task-packet.schema.json` 输出的 schema：packet-id（M<NN>-T<NN>）、
     contract-version、goal、requirements（引用 R 编号）、route
     （frontend|default|hard）、needs-live-credentials（默认 false；需要 live
     凭证的任务标 true 并提醒用户需人工确认）、context-files、cache-scope、
     boundaries、acceptance.L1/L2/L3（验收必须是**可执行命令**，覆盖率闸门也
     是一条 L1 命令）、review-rubric、escalation、done-report-format。
   - 不创建或编辑 `state.json`。首次运行由 Go 引擎从契约初始化状态；之后所有状态变更也必须通过引擎。
   - `flywheel.config.json` — 只有任务需要独立配置时，使用
     `flywheel template flywheel.config.example.json` 导出完整模板并调整。
     否则使用已确认的项目或全局配置。四级发现按首个完整文件选择，
     不逐字段合并；任务文件会遮住项目和全局配置。
     需要复用模型定义时可查看 `flywheel template flywheel.profiles.example.json`。
     profile 只引用本文件内定义；DSH provider 是实际 adapter，channel 是配置渠道，
     别名不会自动切账号。用 `flywheel config inspect` 确认展开后的实际绑定。
   - `.gitignore` — 任务目录级，逐行忽略 `orchestrator.log`、
     `orchestrator.pid`、`metrics.jsonl`、`.state.lock`、`.run.lock`、
     `.stop-request`、`.control/`（运行时易变产物；
     state.json/retro.md/inbox.md/reports/ 属于契约层，**不要**忽略）。
   - **分清测试权限**：为关键验收提供独立 oracle，并把冻结路径写入
     `boundaries.must-not-touch` 或配置的 `protected-test-patterns`。executor 可以
     编写普通单元测试、回归测试并做必要重构；不能削弱断言、改覆盖率阈值或冻结 oracle。
     Plan 运行中只修订任务目录内 Spec/packet；目录外 oracle 的修订需要显式人工任务。
4. **试跑每条验收命令（硬规则）**：提交前，在目标仓库逐条实际执行你写下的
   每条 L1 命令（含覆盖率闸门命令）。允许失败（目标代码还没实现），但必须把
   每条命令的「命令原文 + 退出码 + 输出摘要（≤10 行）」记入 spec 附表
   `## Acceptance dry-run evidence`。无法执行的命令（环境缺失等）必须显式标注
   `unverified: <原因>` 并在人审时提醒用户重点关注——**未经试跑的验收命令是
   契约质量的最大盲区**。
   `flywheel doctor --cwd <仓库绝对路径> --task-dir <任务绝对路径> --json`
   可补充只读安装／契约预检；它不执行验收命令，也不能替代上述 dry-run evidence。
5. **保存契约文件**：遵守目标仓库 Git 授权规则；已有提交授权时在目标仓库执行
   `git add docs/<slug>` 并 `git commit`（conventional 风格 message，如
   `docs(<slug>): add flywheel task contracts`）。契约层文件全部是已提交
   内容；未获提交授权时保留可审查的文件并说明尚未提交，勿提前启动。
6. **停下来等人审（关键 checkpoint，不可省）**：草稿生成并按授权保存后 STOP。明确告诉用户：
   请审 spec 与 packet 的需求编号、验收命令、边界与试跑证据。**冻结点 = 人审契约**——
   不要求源素材预先定稿；用户审过契约即冻结，此后范围变更只能走 Plan Loop。
   不要主动启动 `/skill:flywheel-run`。
