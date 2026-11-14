# 使用手册：在 DSH 中处理日常任务

[文档首页](../README.zh-CN.md) · 简体中文 · [English](../en/usage-handbook.md)

本手册按实际目标说明如何使用 Flywheel：排查故障、修复 issue，以及完成复杂系统项目。
以下对应 0.34 工作流；实际设置以安装版本的模板和有效配置为准。
可以直接复制对话示例，并替换其中的问题、仓库和路径。安装细节见[快速开始](getting-started.md)，
完整流程和契约规则见[工作流](workflow.md)。

日常从 `/flywheel-task` 开始。DSH 助手研究需求并准备任务契约，Go 引擎负责执行、
验证、重试、预算、恢复和本地 Git 交付。需要先审方案时用 `/flywheel-new`；
已有任务用 `/flywheel-run`。推送、PR、合并和发布按各自明确的授权继续处理。

- [在 DSH 中开始](#1-在-dsh-中开始)
- [选择入口与模式](#2-选择入口与模式)
- [场景一：排查故障](#3-场景一排查故障)
- [场景二：修复 issue](#4-场景二修复-issue)
- [场景三：复杂系统项目](#5-场景三复杂系统项目)
- [模型、PTC、原生能力与 JEV](#6-模型ptc原生能力与-jev)
- [CLI 与人工接管](#7-cli-与人工接管)
- [遇到问题时如何继续](#8-遇到问题时如何继续)

## 1. 在 DSH 中开始

### 核对已有安装

在目标 Git 仓库的终端运行：

```bash
flywheel version
flywheel skills list
flywheel config inspect --cwd "$PWD" --json
flywheel doctor --cwd "$PWD" --json
```

`version` 显示实际安装版本，`skills list` 列出内嵌入口；当前共有 11 个 skills。
`config inspect` 显示配置来源及实际模型、后端、权限和会话策略。`doctor` 检查
本地依赖和配置，不证明账号额度或真实模型调用已经可用。

安装或配置改变后，需要进一步检查 SDK 时可运行：

```bash
flywheel doctor --cwd "$PWD" --probe --json
```

`--probe` 会初始化 DSH SDK 并核对目录、权限和关闭结果，不发送推理 prompt。
Provider 初始化可能查询模型目录或写缓存；探测会话使用临时存储。原生 Team、
Swarm、Tower、远端 Review 和 GUI 的实际准入仍以对应运行回执为准。

首次安装或需要升级时，在能找到桌面附带 `dsh` 命令的终端执行：

```bash
npx -y @ckanner/flywheel@latest install
```

已有完整安装不必为每个任务重复安装。按[升级说明](getting-started.md)退出和重载
DSH，并保留原 prefix、profile 和定制设置。CLI、内置引擎和桌面插件需要同版本；
普通 `npm install` 只放置包文件，完整设置由显式 `install`／`setup` 完成。
所选后端的登录仍由 DSH、Kimi、Codex 或 Claude 自己提供。

### 打开项目并发起任务

1. 在 DSH 打开目标仓库并进入对话。当前对话模型负责研究代码和准备契约；
   Flywheel 的 `loops.plan` 只控制引擎启动后的 Plan，不切换当前对话模型。
2. 在对话输入 `/flywheel-task` 加上目标和验收要求。先审方案时改用 `/flywheel-new`。
3. 助手选择或复用任务分支和执行目录，在其中准备 `docs/<任务名>/`，再调用启动接口。
4. 查看真实启动回执，并打开“统计”之后的 **Flywheel** 会话视图；窄窗口可检查更多菜单。
   新会话的空白首页尚不显示这排会话视图。

常规示例：

```text
/flywheel-task 修复搜索结果为空时页面报错的问题，补充回归测试并完成验证。mode=auto
```

Task／Run 的模型工具调用会关联当前会话的任务。画布跟随这个关联，无需填写路径表单。
加载 skill 或生成方案不等于引擎已启动；应看到启动回执和真实状态。
独立终端任务的关联方法见[CLI 与人工接管](#7-cli-与人工接管)。

如果 CLI 能列出 skills，但宿主尚未发现它们，可在目标仓库补装项目入口：

```bash
flywheel install-skills --project "$PWD" --json
```

检查安装结果并按宿主规则重新加载。安装器保留用户修改过的文件并报告冲突；
不要把“有冲突”误认为已经更新。PATH 缺失时可使用已安装的
`~/.config/flywheel/npm-launcher`。更多接入问题见[快速开始](getting-started.md)。

## 2. 选择入口与模式

### Skill 速查

下表中的 `<任务目录>` 是助手报告的真实资料目录，例如
`/absolute/task-worktree/docs/fix-search`。已有任务继续使用原路径，不再追加一层 `docs/`。
`M01-T01` 是示例 packet ID，需要替换成该任务中实际存在的 ID。

| DSH 入口 | 用途 |
| --- | --- |
| `/flywheel-task <目标> mode=auto` | 准备并启动新调查、实现或文档任务；省略 mode 也使用 Auto |
| `/flywheel-new <目标>` | 准备可审阅方案，不启动实施 |
| `/flywheel-run <任务目录>` | 启动或恢复已有任务，沿用已保存的流程策略 |
| `/flywheel-status <任务目录>` | 查看进度、用量、结果和阻断 |
| `/flywheel-stop <任务目录>` | 请求停止运行，保留任务成果和记录 |
| `/flywheel-plan <任务目录>` | 单独运行规划或重规划，要求 Full |
| `/flywheel-exec <任务目录> M01-T01` | 运行指定 packet 的有界 Execute／L1 循环 |
| `/flywheel-verify <任务目录> M01-T01` | 运行适用的确定性验收，不调用模型 |
| `/flywheel-review <任务目录> M01-T01` | 对满足前置条件的当前候选单独评审 |
| `/flywheel-intake <任务目录> [source]` | 扫描配置的来源并给出分诊建议；`source` 为可选来源名 |
| `/flywheel-garden <任务目录>` | 根据复盘提出规则或技能改进建议，供审阅 |

Exec／Verify／Review 可以省略 packet 参数，由当前任务状态决定适用对象。
单阶段通过不等于整个任务完成；常规任务让引擎继续完整流程。Run 可以明确附加
`mode=auto|quick|full`；`pause=review` 使用前台路径在评审前暂停。
这些是 skill 的对话参数，终端参数见[CLI 参考](#7-cli-与人工接管)。

Kimi 使用相应 `/skill:flywheel-task`、`/skill:flywheel-run` 等名称。
查看安装版本的完整说明：

```bash
flywheel skills read flywheel-task
flywheel skills read flywheel-run
```

### Auto、Quick 与 Full

| 模式 | 适用情况 | 流程 |
| --- | --- | --- |
| Auto | 日常默认；准备阶段已记录范围、风险和验收依据 | Go 根据契约选择 Quick 或 Full |
| Quick | 一个里程碑、一个可独立验收的 packet，设计与检查充分 | Execute → 必需验证 → 独立最终 Review → 本地 Git 交付 |
| Full | 多个 packet、未决设计，或权限、迁移、并发等风险 | Plan → 分项执行／验证／Review／交付 → 集成验证与 Judge |

Quick 保留适用的 L1／L2／L3 和仓库必需检查。首次成功路径有 Execute 与最终 Review
两次阶段模型调用；宿主准备、工具轮次、重试另计。显式 Quick 仍要满足资格；发现新风险
时可以单向升级为 Full。没有工作流元数据的旧任务继续使用 Full。

Auto 选择工作流，模型由执行路由和配置决定。`--only execute` 是单阶段操作，
不能代替 Quick。详细资格、契约和升级规则见[工作流](workflow.md)。

## 3. 场景一：排查故障

提供症状、预期行为、时间窗口、复现条件以及已知日志位置。文件和命令应来自真实
仓库；助手会据此准备具体取证动作，用户无需手填调查 JSON。

### 先得到原因报告

```text
/flywheel-task 排查这个仓库中“登录后偶发跳回登录页”的问题。

先检查相关源码、现有日志、最近改动和测试，提出可检验的假设，
运行与症状直接相关的本地复现或诊断检查。

本轮只排查，以原因报告和修复建议作为交付：
说明已确认事实及对应证据、支持或排除的假设、仍缺少的证据，
以及建议的修复方式、回归检查和回退条件。
```

这个目标对应调查，不启动业务实现。调查观察和报告保存在原任务记录中；
报告应区分事实、推断与未知。`supported` 表示报告包含结构合法的支持观察，
仍需要语义核查；它不表示代码已经修复，也不保证已经证明因果。

### 排查后直接修复

```text
/flywheel-task 排查并修复这个仓库中“登录后偶发跳回登录页”的问题。

允许修改登录、会话刷新相关实现及对应测试。
使用同一个任务保存调查证据；结论有支持后继续实现，
补充能够复现原问题的回归检查，完成仓库要求的验证和独立 review。

给出根因、修复内容、测试结果和回退方法，并提交 PR。mode=auto
```

原目标明确授权修复时，后续沿用该范围。登录和会话问题可能涉及权限或并发，Auto
可以选择 Full；不要仅凭“改动文件少”要求 Quick。新增范围或实质产品取舍另行确认。

### 读懂调查状态

`start` 保存调查契约，`run` 才执行 Go 受管取证。收集结束后可能显示
`awaiting-report`：当前 DSH 助手需要根据观察提交报告，而不是继续重复启动取证。
可以在原对话说：

```text
继续原任务：根据已经保存的观察完成并提交调查报告。
如果结论有支持且符合此前授权，就在原任务目录继续修复。
```

只有原目标为“调查并修复”、结论有支持且待核回执已解决时，才在同一任务执行目录
准备 Spec／packets 并启动实施，保留原范围、来源和用量。只调查的任务以报告结束；
结论不足或访问受阻时，应交付局限和缺失证据。JEV 默认关闭也能调查；JEV 不代写报告。

精确动作、预算与恢复规则见[有界调查](investigation.md)。

## 4. 场景二：修复 issue

将示例编号替换为真实 issue 编号或链接。已有明确目标时直接用 Task：

```text
/flywheel-task 修复当前仓库的 issue #123，mode=auto。

先阅读 issue、讨论和最新代码，确认复现条件与验收要求，
检查是否已有相关实现或未完成工作。

在独立任务分支/worktree 中完成修复，补充回归测试，
运行仓库要求的检查，并完成独立 review。

完成后推送任务分支并创建 PR，关联 #123，
说明根因、行为变化、测试结果和仍然存在的限制。
```

最后一段明确授权推送和创建 PR。如果只需要本地结果，改为“交付本地提交和验收报告”。
阅读远端 issue 和创建 PR 需要对应访问权限；工作台远端交付使用已登录的 GitHub CLI。

准备阶段应报告任务分支、执行根和 `docs/<任务名>/`。Spec 记录目标与验收，packet
是可独立执行和验证的工作单元。契约及冻结验收需要实际进入 Git，packet worktree
才能取得同一基线。目标清楚且符合资格时，Auto 采用 Quick；复杂风险使用 Full。

运行中按需输入以下入口，均使用助手报告的原任务目录：

| 需要的操作 | 对话入口 |
| --- | --- |
| 查看进度 | `/flywheel-status <任务目录>` |
| 停止并保留成果 | `/flywheel-stop <任务目录>` |
| 处理阻断后继续 | `/flywheel-run <任务目录>` |

不要为同一个 issue 重新生成任务来继续运行。正常返修由引擎负责；人工追加一次尝试
可以使用画布中的相应操作，保留累计失败和预算。关闭页面不等于停止引擎。

本地 `completed` 表示当前候选达到验收、独立评审和 Git 交付条件。远端 PR 是另一步：
由已获授权的宿主或工作台 PR 节点处理；合并还需符合目标仓库的检查与授权。
创建 PR 不等于已经合并，合并也不自动发布或部署。

想先从一批 issues 中找候选时，再使用 Intake。它要求完整有效配置中已有
`intake.sources`（默认空列表）和存在的资料目录；产物是 `triage-inbox.md` 与建议。
候选经审阅批准后再准备正式任务，扫描本身不授权实施。详见内嵌
`flywheel-intake` skill 和[工作流](workflow.md)。

## 5. 场景三：复杂系统项目

### 已决定完整实施

```text
/flywheel-task 在这个仓库中实现多租户任务调度系统，mode=full。

先研究现有架构、接口和部署方式，整理租户隔离、权限、任务生命周期、
重试与幂等、并发控制、监控、数据迁移和回退要求。

将需求编号，拆成明确的 milestones 和 task packets。
每个 packet 写清依赖、允许修改的范围和可执行验收。
对可独立开发的模块声明并行条件，遵守仓库已有门禁。

完成实现、回归测试、集成验证、独立 review 和最终 judge，并提交 PR。
已经明确的范围继续推进；会实质改变产品行为的缺失取舍，再集中向我确认。
```

可采用以下里程碑组织方式，实际拆分须依据仓库依赖和验收能力：

| 里程碑 | 主要内容 | 验收重点 |
| --- | --- | --- |
| M1 | 领域模型、接口、租户和权限边界 | 接口契约、隔离与权限测试 |
| M2 | 调度器、Worker、重试与幂等 | 状态转换、重复执行、失败恢复 |
| M3 | 管理界面、可观测性 | 端到端行为、指标和操作可见性 |
| M4 | 集成、迁移、性能与恢复验证 | 整体需求、迁移／回退、最终 Judge |

### 需要先审阅架构

```text
/flywheel-new 为这个仓库设计多租户任务调度系统。

准备 Full 工作流的需求、架构取舍、milestones、task packets、
验收方式、迁移与回退方案，先交付方案供我审阅。
```

审阅并授权实施后，用准备结果中的真实目录：

```text
/flywheel-run <实际任务目录> mode=full
```

从零研究需求用 New；已有契约需要重新规划时才用 Plan。一个任务可以包含多个
里程碑和 packets；packet 是内部执行单元，不会自动各建一个 PR。按可独立交付的
需求或里程碑组织 PR。跨多个仓库的系统可按仓库和交付边界拆任务，统一维护接口规范
与集成验收。

### 并行与原生能力

当前新安装模板采用并行调度，最大根调用并发为 2。依赖已满足、显式声明
`schedule.after` 与 `schedule.writes` 且写入范围不冲突的 packet 才能并行；
共享接口、配置、测试服务等真实依赖需要体现在任务拆分中。Review、集成和终验仍
由引擎协调。旧完整配置省略 scheduler 时保持兼容的串行行为。

每个 packet 和角色有自己的会话身份。符合条件的同角色返修可以续用历史；Review
不继承 Executor 上下文。Full 本身不自动打开 Team、Swarm 或 Tower，按下一节
为需要的阶段选择 native profile。详细边界见[工作台](flow-console.md)和
[会话恢复](session-recovery.md)。

## 6. 模型、PTC、原生能力与 JEV

### 当前模板的模型分工

| 阶段或执行路由 | 后端 | 模型 | 推理等级 |
| --- | --- | --- | --- |
| Plan、frontend 执行、Garden | Kimi CLI | `kimi-code/k3.1-pro` | `high` |
| 默认 Execute | DSH／`deepseek-account` | `deepseek-flash` | `high` |
| hard Execute | Codex Pro machine API | `gpt-6.1-sol` | `high` |
| Review | DSH／`chatgpt` | `gpt-6.1-sol` | `high` |
| Judge | DSH／`chatgpt` | `gpt-6-astra` | `high` |
| Intake | DSH／`deepseek-account` | `deepseek-flash` | `low` |

Claude Code Pro 是可选后端，默认未分配阶段。以上是可编辑的模板绑定，不是账号可用性
或模型排名的保证。DSH 的 ChatGPT provider 与 Codex CLI 的登录分别管理；桌面当前
模型不会决定独立 SDK worker 的模型。需要 hard 路由时，可在需求中明确要求对应
packet 使用该路由，由准备／规划阶段写入契约。详见[模型策略](model-policy.md)。

### 区分三层配置

| 位置 | 控制什么 |
| --- | --- |
| DSH 当前会话与 Agent Preset | 准备任务的模型、当前会话的 PTC 工具组合 |
| DSH Codex／Claude 插件配置页 | 普通插件工具调用使用的 native 默认值 |
| Flywheel 有效配置 | 各阶段后端、模型、native profile、权限、调度和会话策略 |

PTC 是 DSH 的工具编排方式；native 指实际 harness 内的 Agent、Team、Workflow 等
能力，二者可以同时使用。Flywheel 的 DSH native worker 使用共享的 PTC 组合并验证
真实执行入口；Codex／Claude worker 直接使用公开 machine API。

普通 DSH Pro 工具省略 `native` 时读取插件默认值；插件也未设置时保留 CLI 的原有
行为。单次对象完整替换默认值，`null` 明确关闭。Flywheel 的直接 worker 使用自身
配置快照，不读取另一份 desktop profile 的动态默认值。

Flywheel 内置 preset generator 的组合 API，使用它不要求重新生成桌面的 daily/heavy。
桌面、全局 CLI worker、SDK profile 的依赖树也分别管理；缺失某处依赖时按
[原生能力指南](native-capabilities.md)修复实际使用的位置。

### 为一个阶段启用原生 Agent

当前默认模板没有为阶段绑定 `native-profile`。安装配套包或仅添加 profile 定义
不会让阶段自动采用它，还需要为对应绑定选择这个 profile。

先用 `config inspect` 确认实际生效文件。下面是需要合入该**完整配置**的片段，
不是可以单独覆盖保存的配置文件；保留已有模型、渠道、其他 profile 和权限。

```json
{
  "native-profiles": {
    "codex-agents": {
      "harness": "codex-pro",
      "options": {
        "version": 1,
        "agents": {"enabled": true, "maxConcurrentThreads": 3}
      }
    }
  }
}
```

在 **Flywheel → 设置 → 模型与会话** 中，为 hard 执行路由选择 `codex-agents`。
等价操作是在已有 `routes.hard` 对象中增加以下字段，保留该对象的其他字段：

```json
{"native-profile": "codex-agents"}
```

定义在高级 JSON 中编辑；保存后下次启动采用。配置解析成功只证明字段和绑定合法，
实际 Agent 能力仍由运行准入与回执确认。DSH Team、Kimi Swarm／Tower、Claude
Workflow 和 LCU 的具体定义、依赖、权限及限制见[原生能力](native-capabilities.md)。
独立 `flywheel native` Review／Team 作业有自己的回执，不自动成为正式 Review／Judge verdict。

### JEV 与速度设置

JEV 默认关闭，endpoint 为空。需要时在 **Flywheel → 设置 → JEV** 填写真实
System One 地址、固定 `jev-x.y.z` 模型版本和密钥环境变量名称，开启并保存。
实际 Flywheel 进程需要能读取该变量；配置只存变量名。静态 native profiles 不依赖 JEV。

JEV 在已批准候选中选择取证或失败后的下一步，也可在允许的执行绑定中选择模型／effort。
它不代替报告、确定性验证和独立评审。既有调查的冻结策略不会因后来打开开关而被
重新创建；沿用原任务恢复规则。连接、数据与预算限制见[JEV 指南](jev.md)。

当前新安装模板还使用 scoped 会话、full-access、自动管理 worktree、parallel 最大
并发 2、batch 派发和关闭预热。已有配置升级时保留，任务／项目完整配置也可能覆盖
全局选择。完全权限不取消任务边界和验收要求；后端实际隔离能力见[配置参考](configuration.md)。

日常先用 Auto、针对性反馈检查、合格会话续用和有界并行。`ready-queue` 与 DSH Review
预热可按项目选择，使用条件见[配置参考](configuration.md)。离线检查不能证明某个
真实项目会节省多少时间或费用。普通返修复用可信 Plan 检查点，契约或验收改变时再重新规划。

## 7. CLI 与人工接管

### 从已有契约启动

CLI 消费已准备的契约，不从一句目标生成 Spec，也不自动创建外层任务分支。
以下示例假设 Task／New 已在真实任务 checkout 中准备并提交契约。
把两个路径替换为准备结果中的实际路径；它们必须属于同一个任务执行位置：

```bash
repo_dir="/absolute/task-worktree"
task_dir="$repo_dir/docs/fix-123"
```

首次后台启动：

```bash
flywheel control launch --cwd "$repo_dir" --task-dir "$task_dir" --mode auto
```

随后按需选择操作；下表不是需要依次执行的脚本：

| 操作 | 命令 |
| --- | --- |
| 查看状态 | `flywheel control status --cwd "$repo_dir" --task-dir "$task_dir"` |
| 停止 | `flywheel control stop --cwd "$repo_dir" --task-dir "$task_dir"` |
| 恢复原任务 | `flywheel control start --cwd "$repo_dir" --task-dir "$task_dir"` |

`ok:true` 说明控制请求成功。实现是否完成看 `state.status`；`running:false` 也可能是
暂停、`escalated`、`budget-stopped` 等状态。只有调查记录时返回 `preparation`，
不伪造实现 `state`。控制请求断连或就绪超时后先查询同一任务，不重复启动来猜结果。

### 单阶段与暂停

先 `cd "$repo_dir"`，再按需选择一条；这些不是普通任务必需的手工步骤：

| 操作 | 命令 |
| --- | --- |
| Full 重新规划 | `flywheel run "$task_dir" --only plan` |
| 实现一个 packet 并运行 L1 | `flywheel run "$task_dir" --only execute --packet M01-T01` |
| 运行确定性验收 | `flywheel run "$task_dir" --only verify --packet M01-T01` |
| 独立评审当前候选 | `flywheel run "$task_dir" --only review --packet M01-T01` |

`--unit` 是 `--packet` 的别名。阶段前置条件仍生效；只有 Exec 或 Review 通过不代表
完成交付。CLI 没有独立的 `task`、`iterate`、`manual` 或 `resume` 子命令。
正常返修由引擎处理，继续原任务使用 `control start`。

需要在 Review 前暂停时，在任务执行目录以前台方式运行：

```bash
flywheel run "$task_dir" --pause-before review
```

该命令运行到暂停点才返回。显式选择 Full 时可另加 `--mode full`；省略时沿用
既有策略。`control start` 不接受 pause，`--pause-before` 也不能与 `--only` 组合。

### 关联独立终端任务到 DSH

在目标 DSH 会话输入下面的请求，并替换路径：

```text
请调用 flywheel_control 查询下面的任务，并关联到当前会话：
action: status
taskDir: /absolute/task-worktree/docs/fix-123
cwd: /absolute/task-worktree
```

首次关联使用的是模型工具 `flywheel_control`。普通 CLI status，以及人工
`/flywheel` JSON status 命令，都不能代替这一首次关联步骤。
Task／Run 已包含对应工具调用。不要为了显示画布而重新 launch 一个任务。
详细行为见[流程工作台](flow-console.md)。

### 接管已有角色会话

通常使用工作台节点的“暂停并接管”。已有合格受管 Session 时，也可在任务执行目录运行：

```bash
flywheel session "$task_dir" --key "execute/M01-T01"
```

从实际节点记录取得 key；示例不能用于猜测任意 Session。运行中引擎需要先停稳，
接管和交回仍受原回执与状态检查约束。

| 交互命令 | 含义 |
| --- | --- |
| `/history` | 查看最近回复 |
| `/reconcile` | 核对原调用回执 |
| `/return` | 执行要求的复验和评审后交回，保持暂停 |
| `/release` | 交回并保持暂停 |
| `/quit` | 退出交互终端，保留接管状态 |

交回后使用 Run 或 `control start` 继续剩余流程与交付。接管要求、复验和恢复限制
见[会话恢复](session-recovery.md)与[工作台](flow-console.md)。

## 8. 遇到问题时如何继续

| 现象 | 下一步 |
| --- | --- |
| 提示找不到 skill 或 CLI | 用 `skills list` 核对内嵌入口、检查项目技能安装结果和 PATH；必要时使用已安装 launcher |
| DSH 看不到任务画布 | 先进入已有会话，再用模型工具 `flywheel_control status` 关联原任务 |
| 只生成方案，没有开始执行 | New 本来就只准备；已授权实施后对真实任务目录使用 Run |
| 调查停在 `awaiting-report` | 让当前助手根据已有观察提交报告；不重复取证来代替报告 |
| 模型、账号或权限与预期不同 | 用带 `--task-dir` 的 `config inspect` 查看真正命中的完整配置和实际 binding |
| desktop 可用，但 worker 缺少 provider／插件 | 核对独立 SDK profile 或全局 worker 的依赖树，按快速开始／原生能力指南修复实际位置 |
| 终端可用，桌面找不到命令 | 核对受管 launcher、显式 `flywheelBinary` 和安装时路径；在正确环境保留原参数重跑 `setup --no-global` 并重载 DSH |
| `escalated` 或 `budget-stopped` | 阅读具体原因、失败证据和预算；处理后从原任务继续，需要追加尝试时使用工作台操作 |
| 派发、结果或清理为 unknown／pending | 保留原调用和工作区，让引擎核对可信回执；进程退出不等于结果已知 |
| Git 集成失败或冲突 | 保留任务分支和 worktree，解决实际冲突后由引擎重新验证当前候选 |
| 本地 completed，但没有 PR | 按既有授权让宿主或工作台 PR 节点推送并创建；继续核对远端检查和源 HEAD |

恢复继续使用原 taskDir、cwd 和任务分支。不要手写 `state.json`、删除回执或切换 fresh
来绕过未知调用；它们不能证明原操作的结果。任务的 `orchestrator.log`、`retro.md`、
节点证据和会话记录用于定位具体阻断。[恢复指南](session-recovery.md)给出核对方式。

需要把反复出现的问题沉淀为规范时，对已有任务使用
`/flywheel-garden <任务目录>`，审阅生成的 `garden-proposals.md` 后再实施获批修改。

日常可从一个有明确复现条件的 issue 开始，用 Task Auto 完整走通准备、执行、验证、
独立评审和 Git 交付，再为复杂项目的具体阶段选择所需原生能力。
