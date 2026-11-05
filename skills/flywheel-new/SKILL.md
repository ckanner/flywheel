---
name: flywheel-new
metadata:
  version: "1.7.0"
description: Generate a Flywheel task from a requirement — conversational intake that researches the repo, asks intent questions, generates docs/<slug>/ (spec.md + task packets + configuration), then STOPS for human review. Use for "/flywheel-new" in DSH, "/skill:flywheel-new" in Kimi, or a request to create a new flywheel task.
whenToUse: 当用户想把一个需求冻结成 flywheel 任务契约（"把 XX 做成 flywheel 任务"）时使用；也可从 triage-inbox 的 approved 候选生成。
compatibility: "Requires the Flywheel CLI with embedded contract templates, write access, and Git in the target repository."
arguments: requirement
---

# flywheel-new — 任务生成器

You are the task generator. You turn a **frozen-intent** requirement into Flywheel
contract files. You contain no loop logic — the orchestrator runs the loops later.

DSH 在对话中使用 `/flywheel-new <需求>`，也可从输入框的 `/` 菜单选择；
Kimi 使用 `/skill:flywheel-new <需求>`。正常准备无需先填写 Flywheel 视图：
任务资料目录由本 skill 在核实的任务 worktree 中创建，完成后报告其真实绝对路径。

> 参数经 kimi 命名参数注入正文：`$requirement`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

## 问答纪律（硬规则）

- **意图类的问人，事实类的自己查证。** Never ask the user about technical facts:
  read the code, run commands, verify — and write the evidence into the spec.
  Never guess about intent: if the requirement is ambiguous, STOP and ask before
  generating anything.
- 技术事实不许问人；意图歧义不许猜。

## 流程

先确认 `flywheel template --help` 可用。PATH 中没有命令时，使用已安装的
`~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径；首次安装使用
`npx -y @ckanner/flywheel@latest install --json`，检查 `ok` 与逐步结果。
`ready:false` 表示模型依赖或配置尚待处理，不阻止先编写契约，也不表示账号已授权。

准备契约使用当前 DSH 会话模型，`loops.plan` 只决定引擎运行后的规划／修订，
不会替本 skill 切换模型。复杂需求可建议用户在已授权目录中选择 `gpt-6-astra`、
`high`，简单明确的任务可沿用 Flash；不要因此自动发起另一个付费模型调用。

### 先固定任务工作区

默认在本需求的**任务集成分支与独立 worktree**中准备契约。先核实目标仓库、
当前分支、`git status --short` 和 `git worktree list --porcelain`。已明确选择了
本任务工作区就直接复用，不重复确认；不要仅凭目录名判断它属于本任务。
若尚无任务工作区，从已核实的基线创建，不切换原 checkout：

```bash
git -C "$source_repo" worktree add -b "$task_branch" "$task_worktree" "$base_ref"
```

执行前把变量设为本次任务的实际值：`source_repo` 是目标仓库，`task_branch`
是未占用的任务分支，`task_worktree` 是新的绝对路径，`base_ref` 是已核实的基线。
不盲用 `origin/main`，不使用 `-f`、`-B` 或 reset 覆盖既有分支／文件。
若路径或分支已存在，先核对归属再复用；有运行状态的任务不得复制账本迁入新目录。

将 `task_worktree` 设为刚创建或已核实可复用的任务目录，逐条执行，失败即停止：

```bash
repo_dir="$(git -C "$task_worktree" rev-parse --show-toplevel)"
repo_dir="$(cd "$repo_dir" && pwd -P)"
task_branch="$(git -C "$repo_dir" branch --show-current)"
```

核对实际分支。`main`／`master`、仓库已知默认分支
或 detached HEAD 都不能在用户不知情时成为运行位置；没有明确的特殊选择时，
先建立任务工作区。已有明确选择与 Git 授权继续沿用；仅目标或归属无法判断时才问。
以下调研、验收试跑和契约保存都在 `repo_dir` 中完成，`task_dir` 必须是其内部的
`docs/<slug>`。在结果中报告这两个绝对路径及实际分支，供后续启动／恢复复用。
仅在 shell 中 `cd` 或使用 `git -C` 不代表已改变 DSH 会话的当前工作目录；
后续 skill 继续使用所报告的绝对任务路径，不依赖视图自动关联本次任务。

### 生成与冻结

1. **调研仓库**：读相关代码/文档，确认需求对应的真实位置与约束；把关键事实
   （含你验证过的命令与输出）记下来。若任务来自 Intake：可读
   `docs/<slug>/triage-inbox.md`，**只从标记为 approved 的候选生成契约**
   （未 approved 的候选不是需求，提醒用户先确认）。
2. **问意图**：只问意图类问题（范围、取舍、优先级、验收标准的主观部分）。
   一次问清，不要挤牙膏。
3. **生成契约**：在所选任务 worktree 的 `repo_dir` 内创建 `docs/<slug>/`：
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
     新模板的 hard 路由使用 `codex-pro` 插件 machine API；保留用户已有显式配置，
     不把外部 subagent 工具名当 provider，也不在 skill 中直接调用旧 CLI 代替引擎。
   - `.gitignore` — 任务目录级，逐行忽略 `orchestrator.log`、
     `orchestrator.pid`、`metrics.jsonl`、`.state.lock`、`.run.lock`、
     `.stop-request`、`.control/`（运行时易变产物；
     state.json/retro.md/inbox.md/reports/ 属于契约层，**不要**忽略）。
   - **分清测试权限**：为关键验收提供独立 oracle，并把冻结路径写入
     `boundaries.must-not-touch` 或配置的 `protected-test-patterns`。executor 可以
     编写普通单元测试、回归测试并做必要重构；不能削弱断言、改覆盖率阈值或冻结 oracle。
     Plan 运行中只修订任务目录内 Spec/packet；目录外 oracle 的修订需要显式人工任务。
   契约目录创建后，将 `task_name` 设为本次采用的目录短名，执行
   `task_dir="$(cd "$repo_dir/docs/$task_name" && pwd -P)"`，之后所有命令复用该绝对路径。
4. **试跑每条验收命令（硬规则）**：提交前，在 `repo_dir` 逐条实际执行你写下的
   每条 L1 命令（含覆盖率闸门命令）。允许失败（目标代码还没实现），但必须把
   每条命令的「命令原文 + 退出码 + 输出摘要（≤10 行）」记入 spec 附表
   `## Acceptance dry-run evidence`。无法执行的命令（环境缺失等）必须显式标注
   `unverified: <原因>` 并在人审时提醒用户重点关注——**未经试跑的验收命令是
   契约质量的最大盲区**。
   `flywheel doctor --cwd <仓库绝对路径> --task-dir <任务绝对路径> --json`
   可补充只读安装／契约预检；它不执行验收命令，也不能替代上述 dry-run evidence。
5. **保存契约文件**：遵守目标仓库 Git 授权规则；已有提交授权时在 `repo_dir` 执行
   `git add docs/<slug>` 并 `git commit`（conventional 风格 message，如
   `docs(<slug>): add flywheel task contracts`）。契约层文件全部是已提交
   内容；未获提交授权时保留可审查的文件并说明尚未提交，勿提前启动。
6. **提交人审**：本次草稿尚未经审阅时，生成并按授权保存后 STOP。明确告诉用户：
   请审 spec 与 packet 的需求编号、验收命令、边界与试跑证据。**冻结点 = 人审契约**——
   不要求源素材预先定稿；用户审过契约即冻结，此后范围变更只能走 Plan Loop。
   已批准的同一契约沿用已有审批，不重复索取；本 skill 不自行启动运行。

获批后在 DSH 对话使用 `/flywheel-run <上述 task_dir>`，查看进度用
`/flywheel-status <同一路径>`。若用户另选 Flywheel 视图，把 `task_dir` 填入
“任务资料目录”；只有 DSH 当前工作目录就是 `repo_dir` 时，“项目目录”才可留空。
视图的目录选择不会随 skill 调用自动更新。

## 交接边界

引擎负责 packet worktree 的创建／复用、Execute 与 L1、独立 Review，合格单元
本地 `ff-only` 合回 `repo_dir` 当前所在的任务分支；不需要为每个 Loop 再手工建
worktree。Plan 在任务集成 worktree 修订契约，里程碑 L2/L3 与 Judge 在合入后的
任务集成 worktree 验证。worktree 只隔离 Git 工作目录，不是 OS 安全沙箱。

当前默认尝试 packet worktree；创建失败可能记录 `worktree unavailable` 并回退
`in-place`，不是强制隔离保证。严格隔离任务遇到该情况应停止并处理，不能手改
`state.json` 或搬走已有成果。契约获批并提交后，可按已有授权建立任务级 Draft PR；
完整验收和对应 head 的检查／审阅满足仓库要求后，才由用户或获授权宿主合并 PR。
引擎不自动 push、创建 PR 或合并远端 PR；本地 packet 合并不代表 PR 已获批准。
