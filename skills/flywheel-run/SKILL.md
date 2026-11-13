---
name: flywheel-run
metadata:
  version: "1.12.0"
description: Start or resume an authorized, prepared Flywheel task through the Go control interface, retaining its persisted Quick or Full strategy; optionally choose a mode or pause before review in the foreground. Use for "/flywheel-run docs/<slug>" in DSH, "/skill:flywheel-run docs/<slug>" in Kimi, or a request to resume an existing task. Use flywheel-task to prepare a new goal and launch it.
whenToUse: 用户要启动或恢复已准备且范围已获授权的 Flywheel 任务时使用；保留原任务身份与策略，可选在 Review 前暂停。
compatibility: "Requires the Flywheel CLI with control, config inspect and doctor commands, plus the harnesses selected by the task configuration."
arguments: slug pause mode
---

# flywheel-run — 启动或恢复编排器

只校验参数、调用 Go CLI、翻译结果。所有规划、执行、重试、验收和状态转移由引擎负责。

DSH 在对话中使用 `/flywheel-run <任务资料目录>`；Kimi 使用
`/skill:flywheel-run <任务资料目录>`。直接复用准备阶段给出的真实任务路径，
无需先填写或打开 Flywheel 视图。

> 参数经 kimi 命名参数注入正文：`$slug`、`$pause`、`$mode`；其他 harness 按调用文本提取。
> `slug` 是实际任务目录路径，例如 `docs/my-task` 或绝对路径，不再追加 `docs/`。
> `pause` 可选，当前只接受 `review`。
> `mode` 可选，只接受 `auto`、`quick`、`full`；未提供时省略该参数，保留已有选择。

## 已有调查准备记录

先用 `control status` 查询原目录。响应只有 `preparation` 而没有实现 `state` 时，
保留原目录与预算，通过 `flywheel_control` 的 `action: investigate`、
`investigation: {action: run, revision: <实际版本>}` 启动或核对原调查回执。
非 DSH 使用同一 `control investigate` JSON stdin。该运行不接受 Quick / Full 或
pause=review；这些参数属于已有合法 Spec 的实现阶段。不要补造 Spec 来掩盖缺失根因。
调查终态 supported / inconclusive / access-blocked / cancelled 不等于代码 completed。
若用户要求仅看方案而尚未授权运行，继续保留原停止点。JEV 服从有效配置的唯一
`jev.enabled` 开关与原准备策略；关闭会停止后续请求及采用，不重置原回执／预算。
`awaiting-report` 后形成报告，不重复启动取证，也不手工改准备状态来开启 JEV。
复现命令与 HTTP 查询同样使用原动作回执；超时、取消、截断或清理未确认不能
翻译为成功。引擎记录保留副本或未知动作时先核对现场，不另起命令代替恢复。

## 步骤

1. **校验**：任务目录含 `spec.md` 和 `task-packet-*.md`，需求和运行已获授权。
   用户要求先审阅时沿用该停止点；已明确授权同一需求则不重复索取审批。
   配置可来自任务、项目、全局或内嵌表；不强制任务目录另存配置。
   `state.json` 可以不存在，由引擎初始化；不得手工创建或修改它。
   将任务目录和 Git 仓库根解析为绝对路径：

   ```bash
   task_dir="$(cd "$slug" && pwd -P)"
   repo_dir="$(git -C "$task_dir" rev-parse --show-toplevel)"
   repo_dir="$(cd "$repo_dir" && pwd -P)"
   task_branch="$(git -C "$repo_dir" branch --show-current)"
   git -C "$repo_dir" worktree list --porcelain
   git -C "$repo_dir" status --short
   ```

   逐条执行，任一命令失败就停止启动并报告路径问题。`repo_dir` 应是准备时选定的
   任务集成 checkout；`task_dir` 必须来自同一目录，不能混用另一个 checkout 的账本。
   核对实际分支与既有任务选择。没有明确特殊授权时，不在 `main`／`master`、
   已知默认分支或空分支名（detached HEAD）直接开跑；新任务先按 `flywheel-new`
   创建或选用任务工作区。已选择工作区或明确授权的运行位置不重复询问。
   已有状态的任务使用原路径恢复，不切分支、复制／重置 state 或替换 packet worktree。

   若 `flywheel` 不在 PATH，使用
   已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。
   首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对 `ok`
   和逐步结果；`ready:false` 表示尚有模型依赖或配置问题。安装不证明账号已授权。
   所用版本必须支持 `flywheel control --help`。

   首次启动、升级或修改模型配置后，查看有效绑定并做只读本地预检：

   ```bash
   flywheel config inspect --cwd "$repo_dir" --task-dir "$task_dir" --json
   flywheel doctor --cwd "$repo_dir" --task-dir "$task_dir" --json
   ```

   阻断项先处理；不要在 skill 内静默换 profile、provider 或付费渠道。
   hard 的新默认 `codex-pro` 由引擎直接调用插件公开 API；不在 skill 中另发
   `codex exec` 或父 agent 委派。旧配置保留原后端，要迁移先让用户审阅实际配置差异。
   用户要求 SDK 检查时可加 `--probe`，只初始化并验证目录／权限／关闭，不推理。
   Codex Pro 只做本地包／命令检查；其完整策略事后检查警告不能翻译成运行前隔离保证。
   可选 `claude-code-pro` 同样由独立 worker 调用公开 machine API；先核对配套
   0.4.0 插件与 CLI。首版只支持明确 full-access，不把原生审批模式当作 OS 沙箱。
   不主动改变 loop 绑定，也不在 skill 内扫描或选择 Claude session。
   `ok:true` 不等于账号、额度或模型能力已验证，unknown 不得翻译为通过。

2. **按所选运行方式调用一次**：

   DSH 优先调用 `flywheel_control`，传入 `action:"start"`、实际绝对 `taskDir`
   和 `cwd`，用户明确选择工作流时再带 `mode`。插件将任务关联到当前会话视图。
   其他宿主使用 Go 自己的启动与就绪协议：

   ```bash
   flywheel control start --task-dir "$task_dir" --cwd "$repo_dir"
   ```

   明确选择工作流时可追加 `--mode "$mode"`。新任务可根据契约选择 Quick；
   旧任务没有字段时仍为 Full。恢复沿用已持久化策略，不因传 auto 重新分类，
   也不把已有 Full 降为 Quick。显式 full 或新风险的升级由引擎记录。

   用户明确选择 `pause=review` 时，在仓库根以前台方式运行并保留该进程：

   ```bash
   # 未指定 mode：恢复已保存策略
   cd "$repo_dir" && flywheel run "$task_dir" --pause-before review
   # 用户同时指定 mode：在同一前台调用中透传（与上一条二选一）
   cd "$repo_dir" && flywheel run "$task_dir" --pause-before review --mode "$mode"
   ```

   `pause=review` 不覆盖用户明确选择的 mode；例如 `mode=full` 必须在这个前台
   命令中传入，省略 mode 时不添加该参数。该模式运行到评审前暂停才返回。
   `control start` 当前不接受 pause 参数，
   不得假装桌面启动实现了这个冻结点；`--pause-before` 也不能和 `--only` 组合。

3. **读取真实状态**：默认后台启动返回 protocol 1 JSON。`ok` 是控制操作结果，
   `running` 是引擎运行状态，任务结果看 `state.status`；不能把 `ok: true`
   当成任务已完成。任务也可能很快结束，或停在 escalated / budget-stopped。
   就绪超时或断连不代表子进程未启动，先查询同一任务并读 `orchestrator.log`，不要重复拉起：

   ```bash
   flywheel control status --task-dir "$task_dir" --cwd "$repo_dir"
   ```

4. **翻译结果**：说明当前状态、工作单元、日志路径与后续入口。
   后台模式可通过 `flywheel-status` 或桌面面板观察，通过 `flywheel-stop`
   停止；关闭当前会话不等于停止后台引擎。监督模式已经暂停时明确说明尚未评审
   或完成交付。失败或升级时转述实际原因，不在 skill 内重试，不改 `state.json`。
   说明请求模式、生效策略与原因；Quick 的 Plan／Judge 不适用不等于已通过。
   有 JEV 执行决策时，翻译真实采用的修复、补证据、重规划或模型绑定及原因；
   区分建议、实际效果与未知。模型选择仍受原允许集合、切换次数和预算约束。
   关闭总开关沿用原路径，不在 skill 中补一套决策或重试循环。
   DSH 工具调用会明确关联当前会话视图。CLI 运行登记可带 DSH 提供的会话 ID，
   无归属的独立终端运行可从“本机最近任务”选择同一个任务；不要求用户填写路径字段。
   真实任务位置由引擎提供，只读展示；CLI 参数继续使用原 `task_dir` 与 `repo_dir`。

## 工作区与恢复

`flywheel-run` 同时是恢复入口：继续使用原 `task_dir`、`repo_dir` 和任务分支，
由引擎恢复 packet 状态并重新核验证据，不重新生成任务或为同一 packet 另建目录。
若上次记录了清理、派发或结果未知，先查看原任务的调用记录与日志并保留现场。
确认 worker／guardian／app-server 已结束只解决清理问题，不证明调用结果已知；
Go 仍须核对可信终态回执。不要切换 fresh、换任务目录、删除回执或重置 state 来绕过阻断。
外层退出或等待几秒不算证明，skill 不自行选择会话 ID 或实现恢复循环。
配套 Codex 0.9.0／Claude 0.4.0 支持由 Go 只读核对原调用 journal；pending 不证明
进程仍活着，找到成功回执也须有原候选检查点。只从原路径调用一次正常恢复入口，
不删除 `.sessions`，不直接调用插件重发 prompt。按角色覆盖与质量阈值由 Go 解释，
阈值默认关闭，重建不会重置预算或解除 unknown；不在 skill 中另做压缩。

| 阶段 | 实际工作目录 |
|---|---|
| Full Plan／升级后修订 | `repo_dir`，只允许修订任务内契约 |
| Execute、L1、Review | 引擎记录的 packet 实际候选目录 |
| Quick 适用 L2/L3 | 最终 Review 前，在同一 packet 候选目录 |
| Full 里程碑 L2/L3、Judge | 合入后的 `repo_dir` |

Quick 的最终 Review 同时检查全部需求、验收充分性和 Quick 资格。验收缺口或
新风险可升级到 Full；策略变更保留预算、工作区与历史，并使不匹配的证据失效。
Go 按有效 `scheduler`、冻结的依赖和写入范围选择串行／并行；不在 skill 中另起调度循环。
独立 Worktree 的 Execute／L1 可并发；共享 checkout 批次先收齐 Execute，再串行验收完整
共同候选。Review、集成与终验仍串行。并行资格不明确的旧契约保留串行行为。
引擎按 `sessions.mode` 决定同角色历史是否可复用；普通返修可
延续 Executor，复审可延续 Reviewer 自己的上下文，二者始终隔离。复审仍检查完整
当前候选与全部适用要求。支持的主任务适配器在 fresh/scoped 下均记录调用安全事实，
会话选择和回执消费由 Go 完成，skill 不接管。重试保留磁盘成果。
当前宿主对话无需保持模型运行；Go 的 packet 并发不等于 DSH Teams 或父模型自主委派。

核对实际配置的 `permissions.mode`：`role-based`（已有文件缺省）下 Plan／Execute
请求 `workspace-write`，Review／Judge 请求 `read-only`；`full-access` 下全部模型
阶段通过所选后端正式接口请求完全权限。Kimi print 保持原生 auto，不宣称 OS
只读或工作区沙箱。完全权限不改变评审职责；候选或契约在评审期间变化时，裁决不能通过身份检查。

`worktree.enabled` 只控制自动创建／管理 packet Worktree，与调度独立。关闭时可共享
任务 checkout，或由用户／模型在实现前选择并登记合格的外部 Worktree；不强制原地执行。
自动管理开启的并行批次创建 Worktree 失败就停止；串行路径仍可能回退 `in-place`。
实际位置和模式以引擎回执为准，严格隔离要求下遇到原地执行应停止并处理。
保留原成果和状态，不把重启当成隔离已恢复。独立 Worktree 的合格单元集成到
`repo_dir` 当前任务分支；共享目录批次全部通过后统一提交，不逐个提前交付。

GitHub PR 在外层管理：按已有授权推送任务分支／建立 Draft PR，完整终验及当前
head 的检查与审阅满足仓库要求后再合并；引擎不自动执行这些远端操作。

## 单环入口

需要人工逐步控制时使用 `flywheel-plan`、`flywheel-exec`、`flywheel-review`、
`flywheel-verify`。它们的返回条件以引擎子环为准；尤其 Execute 会执行 L1，
不是只有一次模型调用。

这些单环入口不是 Quick 的替代品。要完成快速闭环，应恢复同一任务的完整 `run`，
由引擎统一处理必需验收、最终 Review、持久化与升级，不能拼接子环结果声称完成。

## DSH 画布关联

从 DSH 终端运行时，引擎使用宿主的会话标识登记运行；不要手写、复制或伪造这个标识。
首次运行、阶段结束或结果待核对时，再通过 `flywheel_control` 查询一次原任务：

```json
{"action":"status","taskDir":"<已核实的 task_dir 绝对路径>","cwd":"<已核实的 repo_dir 绝对路径>"}
```

这是只读查询，会将真实任务关联到当前会话画布；关联失败不重跑阶段，分别报告引擎
回执和关联结果。没有该工具时，明确给出原任务路径，
用户可从“最近任务”选择；不要声称 UI 已关联。单阶段通过只表示本次阶段通过，
不能说完整流程已完成；加载 Skill、创建任务资料与引擎实际运行必须分别报告。
