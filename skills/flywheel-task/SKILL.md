---
name: flywheel-task
metadata:
  version: "2.2.0"
description: >-
  Register a thin brief and launch an authorized Flywheel task through Go-owned design,
  independent review, progressive planning and verified implementation. Use for
  "/flywheel-task <goal>" in DSH or "/skill:flywheel-task <goal>" in Kimi.
  Retain existing implementation authority as delegated policy; a review-first request
  stops for human design approval. Use flywheel-run for an existing task.
whenToUse: 用户希望描述一次目标后由 Flywheel 完成设计、审查和授权范围内的交付时使用；调查目标沿用有界调查，先审方案用 flywheel-new。
compatibility: "Requires matching Flywheel CLI/plugin with control planning, planning.schema.json, Git, and the configured Provider backends."
arguments: requirement mode
---

# flywheel-task — 从目标推进授权任务

技能是按钮：检查目标和工作区、登记 brief、调用一次控制入口、翻译结果。
设计、独立审查、细化、重试、验收与状态变更由 Go 负责。永不写 `state.json`，
不在宿主对话中提前生成完整系统方案，不自行循环调用规划或审查 Agent。

DSH 使用 `/flywheel-task <目标> [mode=auto|quick|full]`，Kimi 使用 `/skill:flywheel-task`。
默认 Auto 根据已经审查的契约选择 Quick／Full。准备粒度另由 brief 的
`design-granularity` 明确选择；授权由 `policy` 明确选择，三者不能互相代替。
模式不自动更换 provider、渠道或权限；Quick 的实现快路径仍需全部必需验收。

## 1. 目标、既有任务与授权

- 复用用户已经明确的需求、工作区、权限和交付范围，不机械增加重复确认。
  当前需求已有任务时，查询其原目录与 `state.planning`，使用 Run / Plan 继续；
  不重建同名任务、复制 state 或重置预算。
- 用户明确要求先讨论／审方案时用 `review-first`（也可直接用 New），不能因为
  skill 名称是 Task 就自动执行。用户已要求“实现、修复、完成交付”等原范围实施时，
  用 `delegated` 并保存具体原授权文字；新的实质范围变化仍须提出讨论。
- 事实自行读代码和仓库指令；只有未给出的用户取舍会改变任务时才问。
  图表、文档、HTML 和代码任务都保留真实验收，不能因“只是文档”省略流程。
- 核对 `flywheel control planning --help` 与 `flywheel template planning.schema.json`。
  老版本需同版升级，不把 `--only execute` 当 Quick，不自行模拟新协议。

## 2. 调查目标

“先排查，告诉我为什么”或“排查并修好”读取[调查准备与交付](references/investigation.md)，
使用现有 preparation / `control investigate` 路径。report-only 停在证据报告；已授权
修复只在有证据的结论和原范围内继续。调查动作、复现命令、外部读取、JEV 选择、
预算和恢复由 Go 执行，技能不循环。不要用空 Spec 掩盖缺失根因。
新调查在报告支持结论、回执已解决后，在同一 taskDir/cwd 登记下面的薄 brief；Go
绑定该轮报告、累计用量与原修复范围，再执行方案准备和独立审查。已经存在完整旧
Spec/packet 的任务保留原兼容交接；不能未经明确迁移就改写既有实施账本。
以下步骤用于新实现目标，以及尚未进入实施的新调查交接。

## 3. 工作区与薄 brief

1. 检查 Git 仓库、当前分支、`git status --short`、基线和
   `git worktree list --porcelain`。用 `flywheel config inspect --cwd <原仓库绝对路径> --json`
   核对 worktree 策略。已有任务 checkout 直接复用；开启自动管理且没有本任务工作区时，
   从核实基线建立任务分支/worktree；关闭时不自动新建。无明确特殊选择不在默认分支
   或 detached HEAD 实施。保护用户 WIP，不用强制 reset 或覆盖已有目录。
2. 把所选 checkout 根解析为真实绝对 `repo_dir`，在其中创建 `docs/<slug>`，
   解析为 `task_dir`。必须属于同一任务位置。保留原仓库的 Provider 项目身份；
   shell 的 cd 不等于变更 DSH 当前目录，控制调用始终显式传这两个路径。
3. 读足以定位目标的代码、指令和验收入口，记录真实证据、约束、成功标准与未决问题。
   当前宿主模型只准备这些输入。system 设计使用 `loops.design-plan`（默认继承
   `loops.plan`），独立审查使用 `loops.design-review`（默认继承 `loops.review`）；
   compact 使用 `planning.compact` 中对应角色的具名 profile，未配置才回退普通绑定。
   不要求宿主先写完整 Spec、所有 packets 或自报审查结果。
4. 依据当前 `planning.schema.json` 的 `$defs.brief` 生成下述输入。
   `flywheel.config.json` 只在任务确实需要独立完整覆盖时创建；不要用少数字段
   意外遮住项目/全局配置，也不自动新增账号、费用渠道或 full-access。
5. 选择并填写 `design-granularity` 和 `granularity-reason`。已有确切位置、局部边界、
   成熟实现模式和可运行验收，且用户未指定 Full 时可选 `compact`；必须在 evidence
   中记录支撑事实，不能填“应该很简单”。跨模块架构、迁移、未决接口、风险或范围不清
   选 `system`，用户明确 Full 也选 system。Quick 本身不是紧凑准备的证据。
   compact 由引擎一次生成设计与单 packet，再由独立角色联合审查两个精确版本；
   引擎发现复杂性时可升级 system，保留原任务、授权与累计预算。

## 4. 调用一次

将以下对象的路径与目标替换为本次事实，通过 DSH **`flywheel_control`** 调用：

```json
{
  "action": "planning",
  "taskDir": "/absolute/task-worktree/docs/task-slug",
  "cwd": "/absolute/task-worktree",
  "planning": {
    "action": "prepare",
    "request-id": "brief-task-slug-1",
    "start": true,
    "brief": {
      "version": 1,
      "id": "task-slug",
      "goal": "修复已经定位的失败并完成回归验收",
      "original-request": "用户原始请求",
      "constraints": ["保留现有公共接口"],
      "evidence": ["已核实的代码位置和复现证据；没有时用空数组"],
      "policy": "delegated",
      "authorization": "用户确实已给出的原范围实施授权，不是模型自己的批准声明",
      "implementation-authorized": true,
      "mode": "auto",
      "design-granularity": "system",
      "granularity-reason": "需要明确跨模块边界、固定决策与分阶段验收"
    }
  }
}
```

只请求先审方案时，改为 `policy:review-first`、`implementation-authorized:false`，
省略 authorization；沿用明确的停止点。delegated 必须有实际已有授权，
不能因为模板有该字段就编造。可把 mode 换成用户明确指定的 quick/full。
上例是系统设计；符合第 3 节局部条件时改为 compact，并用实际证据说明选择理由。
省略粒度的兼容请求采用 system，不会仅凭 mode=quick 跳过完整准备。

没有 DSH 工具时，把 `planning` 内层 JSON 存入私有 `request_file` 后调用：

```bash
flywheel control planning --task-dir "$task_dir" --cwd "$repo_dir" < "$request_file"
```

DSH 提供工具时不绕过其关联机制。缺少 PATH 可使用用户指定 launcher；首次安装
使用 `npx -y @ckanner/flywheel@latest install --json`，按真实结果处理。
首次使用或配置变更后可做 config inspect / doctor；它们不证明账号、额度或推理成功。

## 5. 翻译与恢复

- 说明实际任务、执行目录与分支、`ok`、`running`、`state.planning.phase`，以及
  规划或实现当前阶段。控制请求完成、审查通过、方案获批、整个任务完成分别报告。
- `review-first` 停在精确版本的人工批准；反馈后用 Plan 再跑设计与审查。
  delegated 仍须独立 Design Review，只有有效原授权与无实质越界时才继续。
  不代点批准按钮、不设置审批来源变量、不自报 `actor:user`。
- Plan / 子计划审查使用独立角色与作用域。已批准根方案内细化由引擎继续；发现固定
  决策、需求、约束或验收改变时返回根方案讨论，不用小任务名绕过批准。
- compact 准备包含一次联合产物与一次独立联合审查。Quick 实现仍包含 Execute、
  必需验收与独立最终 Review；system 准备与 Full 实现保留逐步细化和完整门禁。
  不要宣称整个新任务只有两次模型调用，也不把 review-first 当作模型自审。
- 结果未知、超时或断连时，先对原 taskDir/cwd 调用 `status` 核对，不能重复 prepare
  或 launch。后续通过 Run 继续原任务；预算、工作区、恢复和计量都保留。
  规划角色的 blocked unknown 只能由用户在视图或 `flywheel session ... --abandon`
  显式退役；模型不能代输确认或声称已退役。
- 原生 Team/PTC、Codex review、Claude team 都按有效 native 配置准入；工具输出只是
  证据，不能代替正式审查／批准。新 role 的 backend 继承不表示权限随同继承。
- 本入口不新增远端 push、PR、合并或发布授权；相关交付按已给出的实际范围执行。

已有旧版完整 Spec/packet 的手工兼容路径见[契约参考](references/contract.md)。
已有用户不重新创建任务；新目标与新调查修复交接使用上面的 brief。
