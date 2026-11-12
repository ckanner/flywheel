# Flywheel（飞轮）

项目工作区与任务执行目录分开：Session 归属原仓库，任务可以在独立 Worktree 执行。配置页提供默认开启的“自动管理任务 Worktree”，与并行调度独立；自行选择目录须由引擎核对登记。详见[流程控制台](docs/flow-console.md)。

[English](README.en.md) · 简体中文

Flywheel 用确定性 Go 引擎驱动 Plan、Execute、Review 和分层验收，把已经明确的需求变成有边界、可验证、可持久化的软件交付。模型负责规划、实现和评审；引擎负责状态、重试、预算、工作区、验收、提交与停止。

当前源码支持 **Auto／Quick／Full**：描述目标后由 `flywheel-task` 准备并启动；合格小任务走 Execute、必要验证、一次独立最终 Review 和持久化交付，复杂任务保留完整流程。旧任务继续按 Full 恢复。模式选择不改变已有模型、权限和交付授权。

会话策略支持 **fresh／scoped**：新模板显式 scoped，旧配置缺省 fresh。scoped 可在可信
正常终态后延续同任务、同角色、同作用域的历史；独立复审仍检查完整当前候选。
Codex Pro、Claude Code Pro、DSH、Kimi 的主任务调用在两种模式下都保存安全记录，未知派发、结果或清理
会阻止继续执行，不能靠换模式绕过。P3 已接通 Codex／Claude 公开 typed lifecycle、
持久化 ACK 与只读回执恢复；P4 提供按角色策略、有界质量重建和三组对照评估工具。
真实账号的质量、缓存与性能结果仍待完成，不能由离线实验推断。详见
[会话配置](docs/configuration.md#会话策略历史复用与调用安全)及[中断恢复与上下文质量](docs/session-recovery.md)。

当前源码的统一发行目标为 **v0.30.1**，使用 DSH Mac 桌面为入口：Kimi CLI 规划，DSH 原生 DeepSeek Flash 执行，DSH 原生 GPT 评审，较难的执行任务默认走 Codex Pro 插件的公开 machine API。0.28.3 包含 DeepSeek 账号默认、账号错误分类与会话当前目录修复；安装前核对 npm registry，源码合并不会更新已安装的包。DSH 插件提供控制面板、配置查看和安装预检；具名 model profiles 可复用于不同阶段，Go 仍是唯一状态所有者。设计依据见 [docs/design.md](docs/design.md)，仓库约束见 [AGENTS.md](AGENTS.md)。

**基础安装命令适用于 0.28.0 及以后的完整 npm 发行版；Auto／Quick／Full 与 `flywheel-task` 需要包含本轮变更的同版本 CLI／插件。** 用 `flywheel control launch --help` 检查新增入口是否存在，并在安装前用 `npm view @ckanner/flywheel version` 核对 registry；源码合并或 GitHub Release 不代表 npm 已发行。旧 0.22.1 没有本教程使用的安装与 DSH 接口，当前发行状态见[发行记录](docs/releasing.md#3-失败恢复)。

本轮插件接入测试与实机边界见[验证记录](docs/validation/2026-11-03-codex-pro-default.md)，既有 npm 发行证据见[安装验证](docs/validation/2026-11-01-npm-installation.md)。旧版本的选型、维护机环境与演练记录保存在[历史设计](docs/design-history-v0.24.md)，不代表当前默认值或用户机器状态。

首次使用请按 [Mac 安装与首个任务教程](docs/getting-started.md) 完成安装、配置、无推理检查和小任务验收。本文保留各接口与设计边界的速查。

当前源码另支持可选的 [Claude Code Pro 后端](docs/claude-code-pro.md)：模型与 thinking 可配置，按角色续用已确认会话，尚未分配任何默认 loop。它需要配套插件 0.4.0 的公开 machine API 与显式 full-access；0.3.1 桌面工具不等于已满足此接口。

## 1. 一条命令安装

已有 **Node.js 22.18.0 或更高、npm 和 DSH Mac 桌面及其 `dsh` 命令**后，在终端运行：

```sh
npx -y @ckanner/flywheel@latest install
```

Agent 可在同一命令后加 `--json`，读取一个结构化结果。安装器把本次解析到的**精确版本**的 CLI、对应平台 Go 二进制、DSH worker 和 Codex Pro worker（含精确版本的插件依赖）装到 npm 全局目录，安装该版本的内嵌 skills（当前源码为 11 个），再将同版本 DSH 插件注册到桌面 profile 并核对版本。无需下载压缩包、克隆源码或自行编译 Go。

安装完成会显示每个步骤和本地预检结果。`ok:true` 表示软件安装成功；`ready:false` 表示当前配置仍有运行阻断项，例如缺少 Kimi／Codex CLI，二者可以同时出现。安装器不替换现有 DSH，也不安装或代登录 Kimi／Codex。当前源码的 DeepSeek 默认复用 DSH 已登录账号；用户明确选择 API Key 时仍自行配置该凭证。运行真实任务前需完成所选渠道的准备。

默认使用 npm 的全局 prefix 和用户级 skills 目录。没有全局目录写权限时，可显式选择用户目录：

```sh
npx -y @ckanner/flywheel@latest install --prefix "$HOME/.local"
```

自定义 prefix 后，如需在终端直接输入 `flywheel`，将该 prefix 的 `bin` 加入自己的 shell PATH。`--project`、`--dsh-bin`、`--profile`、`--no-dsh`、`--no-global` 和超时参数见[完整教程](docs/getting-started.md#2-一条命令安装)。

普通 `npm install --global @ckanner/flywheel` 只安装包文件，不通过 postinstall 修改用户的 skills、配置或 DSH profile。已经用它安装的用户可显式运行 `flywheel setup --no-global` 完成配置；`install` 与 `setup` 共用安装流程，默认都会确保本版全局包已安装。

## 2. DSH 入口与独立 worker

本实现覆盖已核对接口的 **DSH Mac 桌面 0.2.1-rc.5 与 0.2.2-rc.1**；0.28.4 的声明只允许这两个版本。先核对 About、`dsh --version` 和命令实际来源。它不要求 alpha、OAR、Forge、Teams 或一直存活的桌面父智能体。

上一节的安装器使用**桌面安装的 `dsh` 命令**执行正式插件管理接口：`dsh plugin --profile desktop add --save-exact --ignore-scripts @ckanner/dsh-plugin-flywheel@<本次版本>`，然后通过 `plugin list` 核对版本。普通 npm DSH CLI 不能管理保留的 `desktop` profile；命令不在 PATH 时用 `--dsh-bin` 指定已核对的桌面命令入口。安装后按宿主提示重新载入。不要猜测 `.app` 内部可执行文件路径。

原生 worker 使用独立的 SDK profile。标准 SDK 已包含 `deepseek-account` provider 和账号服务；标准桌面与 SDK 使用同一 `DSH_HOME` 时默认共用账号授权，无需为账号路径额外配置 API Key 或复制 token。`deepseek-official` 是单独的 API-key provider，仅作为显式选择保留。第三方 provider 则须核对 SDK 实际加载与授权，例如 GPT 原生路由使用本轮核对的 ChatGPT adapter：

```sh
dsh plugin --profile sdk add dsh-plugin-chatgpt@0.1.17
```

这是已核对的 adapter 版本示例；采用其他版本前按其文档检查兼容性。按 provider 自身的正常流程完成授权，保持所选 `DSH_HOME` 与配置来源一致。安装 adapter 不会复制凭证，也不证明账号可以调用默认表里的所有模型。DSH 桌面能在登录后选择账号默认模型，但该选择不控制独立 SDK 会话；Flywheel 显式请求 `deepseek-account`／`deepseek-flash`，详见[账号与 API 的区别](docs/configuration.md#deepseek-账号与-api-key-是两个-provider)。`subagent_codex_pro` 是外部 agent 工具，不是这个 SDK 入口的 LLM provider。Flywheel 的 `hard` 路由通过独立 worker 直接导入 `dsh-subagent-codex-pro@0.9.0` 的公开 `./machine` 入口，复用插件运行时；该精确源码候选随插件 bundle，来源与摘要见 [vendor 记录](plugins/dsh/vendor/README.md)。它不创建 DSH 父模型再次委派。插件内部仍运行 Codex app-server，并使用用户已有的 Codex 登录。

### worker 入口配置

| 环境变量 | 默认值与作用 |
|---|---|
| `FLYWHEEL_DSH_WORKER` | npm 启动器自动解析同版本依赖中的 worker 绝对路径；显式环境变量优先。直接运行 Go 二进制时默认从 PATH 查找 `flywheel-dsh-worker` |
| `FLYWHEEL_DSH_BIN` | PATH 中的 `dsh`；建议固定为已核对的官方命令入口 |
| `FLYWHEEL_DSH_PROFILE` | `sdk`；必须是 SDK profile，不能是 `desktop` |
| `DSH_HOME` | 交给 DSH 的自定义配置根；不设置时沿用 DSH 自己的默认行为 |
| `FLYWHEEL_KIMI_BIN` | PATH 中的 `kimi`；可指定 Kimi CLI 的绝对路径 |
| `FLYWHEEL_CODEX_PRO_WORKER` | npm 启动器解析同版本包的 Codex Pro worker；独立 Go 二进制默认查找 `flywheel-codex-pro-worker` |
| `FLYWHEEL_CODEX_BIN` | 插件内部使用的 Codex 可执行路径；默认 `codex` |
| `CODEX_HOME` | 可选的 Codex 配置／登录目录，交给插件使用；不复制账号凭证 |

旧 `harness: "codex"` 仍是显式兼容路径，使用 PATH 中的 `codex exec`；新默认不会自动退回它。所有选定 CLI、Node 与账户配置必须对**启动 Go 引擎的进程环境**可见。

安装器自动生成 `~/.config/flywheel/npm-launcher` 和 `installation.json`：记录绝对 Node／CLI 路径、已核对的 DSH 入口及安装时可见的工具 PATH。桌面插件未显式配置 `flywheelBinary` 时，读取这份安装记录，校验 launcher 是普通可执行文件且 SHA-256 匹配后使用。**无需手写 GUI 启动脚本或修改 profile 配置。**

用户原有的显式 `flywheelBinary` 优先，安装器不会替用户覆盖它。安装记录不存在时保留 PATH 查找兼容行为；记录或 launcher 已损坏／被修改时明确失败，不悄悄使用另一份引擎。升级 Node、移动全局 prefix 或新增 CLI 后，可在工具均可见的终端运行 `flywheel setup --no-global` 更新安装记录；先保存自己对 launcher 的修改，冲突不会被覆盖。终端里的 `export` 本身不会改变已运行的桌面进程，仍需在面板内复验。

### 不调用模型的检查

在实际目标仓库目录、使用与引擎 worker 相同的环境变量执行：

```sh
flywheel config inspect --cwd "$PWD" --json
flywheel doctor --cwd "$PWD" --json
flywheel doctor --cwd "$PWD" --probe --json
```

`config inspect` 只读展开有效配置；`doctor` 检查目录、配置、所需命令与声明的环境变量。加 `--task-dir docs/my-task` 会校验 Spec/packet，并把工具检查缩小到任务所需阶段与路由；不指定任务时检查全部配置绑定。没有 packet 时按规划前状态检查默认路由，不会擅自生成契约。

`--probe` 进一步启动本地 DSH SDK、创建会话、核对 cwd／权限并关闭，**不发送推理 prompt、不运行验收命令**；provider 初始化可能查询模型目录。输出保留 pass/fail/warn/unknown/skipped，阻断项使退出码为 1；没有阻断项也不能证明账号授权、订阅额度或真实模型推理可用。相同账号／模型／目录／effort／权限的 SDK 检查去重，检查预算 40 秒，进程各收尾阶段有 5 秒宽限，保守预留另 15 秒清理。不会物化全局配置、写任务状态或创建 worktree。

完整安装、权限、生命周期说明见 [DSH 插件 README](plugins/dsh/README.md)，线协议见 [PROTOCOL.md](plugins/dsh/PROTOCOL.md)。

## 3. 当前默认路由

默认内联模板是 [templates/flywheel.config.example.json](templates/flywheel.config.example.json)，可复用配置见 [profiles 示例](templates/flywheel.profiles.example.json) 与[配置指南](docs/configuration.md)。`harness` 指执行后端，`provider` 是 DSH SDK 实际注册的 LLM adapter ID，`channel` 是本配置的账号／计量渠道名；`model` 与 `thinking` 逐调用传入。旧配置未写 provider 时继续把 DSH channel 解释为 provider ID。

| 阶段或执行路由 | Harness | Provider／渠道 | 模型 | Thinking |
|---|---|---|---|---|
| Plan | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high`，匹配身份后可恢复会话 |
| Execute `default` | DSH SDK | `deepseek-account` | `deepseek-flash` | `high` |
| Execute `frontend` | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high` |
| Execute `hard` | Codex Pro machine API | `chatgpt-subscription` | `gpt-6.1-sol` | `high` |
| Review | DSH SDK | `chatgpt` | `gpt-6.1-sol` | `high` |
| Judge | DSH SDK | `chatgpt` | `gpt-6-astra` | `high`，里程碑终验 |
| Intake | DSH SDK | `deepseek-account` | `deepseek-flash` | `low` |
| Garden | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high` |

这些是可编辑的初始绑定，并非模型排名或性能承诺。DeepSeek 只用 Flash；当前 Mac rc.5 的合法 effort 为 `off/low/high/max`，没有 `medium`。双 judge 默认关闭，也不默认跨渠道付费降级。`multimodal` 暂无默认绑定：当前 bridge 只有文本契约，模型名称或目录里的 vision 标记不能证明附件已正确传递。

七个 high 与 Intake low 是工程基线，尚未做本项目的 high／xhigh／max 对照评估。GPT 支持 xhigh/max，Kimi K3.1 Pro 原生支持 low/high/max；hard 与高影响 Judge 可优先比较 xhigh，不把所有后端的档位名当作同一尺度。官方依据、适配器限制和建议见[推理等级说明](docs/model-policy.md)。0.28.2 默认仍使用 DeepSeek API；采用上表账号默认须升级至 0.28.3 或以后版本，并[显式迁移配置](docs/configuration.md#将旧配置切换到已有的三种资源)。

Kimi 使用原生 CLI print/stream-json，不使用 ACP；恢复的是会话历史，不是常驻后台进程。该模式不提供本适配器可宣称的 OS 级只读／工作区沙箱，worktree 只是代码隔离。新模板及显式 subscriptions 迁移使用 `permissions.mode:full-access`，DSH/Codex 全阶段 danger-full-access＋never，Kimi print 为 auto；已有文件未声明时保持 role-based 的 workspace-write／read-only。Codex Pro 在运行前核对模型、目录和沙箱模式，Flywheel 在结果返回后检查完整策略；在受限模式，工作区写入允许 Codex 标准临时目录，额外工作区外 roots 或启用网络的策略不能通过交付资格检查；明确 full-access 则核对完全权限回执，不声称 OS 隔离。这是事后校验，不能阻止自定义 Codex 配置已允许的访问；要求运行前完整策略阻止的环境需等待上游公开 guard。具体账号的可用模型、额度和使用授权需要由所选服务确认。

usage 或美元成本未上报时保留 unknown，不能把订阅调用记成免费。默认 soft token budget 会报告未知量；hard budget 无法证明未知用量未超额时停止。汇总数字是已知量下限。

### 迁移已有配置

配置按**第一个存在的完整文件**选择，不逐字段合并：任务目录 → 项目根 → 全局 `~/.config/flywheel/flywheel.config.json` → 二进制内嵌模板；`FLYWHEEL_CONFIG` 可指定全局配置路径。已有文件不会因升级自动改写。

从 0.27.x 升级要采用插件路径时，在实际生效配置中将 `routes.hard.harness` 从 `codex` 改为 `codex-pro`；具名 profile 则修改对应定义。保留 `model`、`thinking` 和 `channel`，不要把 `subagent_codex_pro` 填入 provider。安装依赖不自动迁移已有配置，也不会默默替换旧后端。

可用新二进制查看模板，再审阅并迁移自己的配置：

```sh
flywheel template flywheel.config.example.json
flywheel template flywheel.profiles.example.json
flywheel config inspect --cwd /repo --task-dir /repo/docs/my-task --json
```

`drivers` 选择 `real/mock`，不是模型或 provider 名称。模型在 `loops` 和 `routes` 配置，可直接写字段，也可引用 `profiles` 后按需覆盖。未知 profile、拼错字段、模型字段的 null 和 provider 冲突会拒绝加载；允许 `_note` 注释，原有 `env-key:null`、`judge-panel:null` 仍合法。渠道别名不会自动切换 DSH 账号，授权仍由选定 SDK profile/provider 管理。修改运行配置后重启引擎；Plan 不允许悄悄改模型、权限或其他运行配置。

## 4. 创建、运行和观察任务

在 DSH 对话中描述需求即可准备并启动，通常无需手工填写目录：

```text
/flywheel-task 修复已定位的列表空状态问题，补齐回归测试并完成交付。mode=auto
```

| 选择 | 何时使用 | 实际行为 |
|---|---|---|
| Auto | 让引擎根据已研究的契约事实选择 | 资格充分时 Quick，否则 Full |
| Quick | 一个里程碑、一个 packet、范围明确且验证充分 | 全部必需验证后作一次独立最终 Review；新风险或验收缺口升级 Full |
| Full | 多工作单元、设计未定、影响较大或明确要求完整流程 | Plan → Execute／Review → 集成验证／Judge |

Quick 不省略仓库门禁、适用 L2/L3、缺陷注入或 Git 持久化检查。正常成功路径有两次
阶段 Agent 调用（Execute、最终 Review），不含宿主准备、内部工具轮次与重试。
旧任务缺少新字段仍为 Full；恢复沿用持久化策略，Full 不因重启自动降为 Quick。

业务仓库建议先建立任务集成分支与独立 worktree；准备契约、Draft PR 和人工合并的完整流程见[从需求准备到 PR 合并](docs/workflow.md)。引擎的单元合并目标是启动 checkout 当前所在分支，GitHub PR 操作由使用者或获授权的宿主执行，也可使用[流程控制台](docs/flow-console.md)的明确授权入口。

准备阶段由当前 DSH 对话模型生成需求契约，`loops.plan` 只控制引擎启动后的规划／修订。复杂需求建议先用已授权的 `gpt-6-astra`、`high` 生成可审阅草稿；简单明确的任务可沿用 Flash，避免每个 packet 都重复调用昂贵规划模型。

`flywheel-task` 使用当前对话研究代码，在任务 worktree 准备 `docs/<slug>/` 的 Spec、packet、验收与工作流事实，再调用 Go `control launch`。DSH 通过 `flywheel_launch` 把该具体任务关联到当前会话的视图。只想先看方案时用 `/flywheel-new`；启动或恢复已有任务用 `/flywheel-run`。Kimi 对应使用 `/skill:flywheel-task`、`/skill:flywheel-new` 和 `/skill:flywheel-run`。宿主准备与 Go 调度的分工、授权和目录规则见[工作流](docs/workflow.md)。

从 `0.28.1` 开始，桌面控制面接入顶部原生 **Flywheel** 会话视图（从 0.28.5 起排在内置“统计”之后），保留 DSH 输入区、模型选择器和右侧栏。`0.28.0` 仍是旧底部弹层，需把 CLI 与桌面插件同版升级。“任务资料目录”选择已准备的 `docs/<slug>/`；“项目目录”是任务所在项目／集成 worktree，可留空。从 0.28.3 开始通过 DSH `workingDirectory.get(session)` 读取会话实际目录，跟随目录切换；0.28.2 仍取最初的 header 目录，切换 worktree 后需显式填写项目路径。查看配置或检查接入可不填任务目录；启动、刷新和停止必须选择已有任务。视图展示最近刷新的结果、完整 packet 列表与未知用量，模型配置和诊断按需查看，原始 JSON 默认折叠。也可使用人工命令：

```text
/flywheel {"action":"status","taskDir":"/repo/docs/my-task","cwd":"/repo"}
```

面板、人工命令、`flywheel_control` 与 `flywheel_launch` 模型工具复用同一组 Go 接口：

```sh
flywheel control launch --task-dir /repo/docs/my-task --cwd /repo --mode auto
flywheel control start --task-dir /repo/docs/my-task --cwd /repo
flywheel control status --task-dir /repo/docs/my-task --cwd /repo
flywheel control stop --task-dir /repo/docs/my-task --cwd /repo
flywheel control config --cwd /repo
flywheel control doctor --task-dir /repo/docs/my-task --cwd /repo --probe
```

`launch` 对准备好的契约默认请求 Auto；`start`／前台 `run` 未提供 `--mode` 时沿用契约与持久化策略，旧任务保持 Full。三者可显式请求 `auto|quick|full`，资格和升级仍由 Go 决定。CLI 消费契约，不自行调用模型生成任务或创建外层任务分支。

`launch`／`start` 派生独立后台引擎，日志进入任务目录的 `orchestrator.log`；关闭面板或桌面不等于停止任务。`status` 只读。`stop` 向本次运行提交带随机 token 的停止请求，等待本次引擎退出受管调用栈、释放任务 lease 后才保存停止状态；不按文件里的 PID 盲目发送信号。引擎只管理本次拥有的进程组；已观测的 `CleanupError` 仍保留在证据和指标中、阻止交付，需要按错误记录处理，`stopped` 不代表任意逃逸进程已清理。停止等待最长 45 秒，桌面入口预留 60 秒。

控制接口始终返回一个 protocol 1 JSON；`ok` 表示控制操作成功，任务是否完成应看 `state.status`。`config` 返回 `configuration`，`doctor` 返回 `diagnostic`；诊断有阻断项时保留完整报告并返回退出码 1，不能当成空白传输错误。若启动就绪超时，后台引擎可能仍在加载：先刷新状态和查看日志，不要另起重复任务。

DSH 工具的 launch／start 将明确的任务路径关联到发起会话；视图展示请求模式、生效策略、原因、阶段适用性与升级记录。它不扫描目录猜任务，也不轮询终端另行启动的任务；手动打开仍可用。启动／停止结果未确认时，先刷新原任务再恢复操作，迟到响应不能覆盖另一个会话或新绑定。原生轨迹保留父会话已有记录，独立 worker 历史不会自动合并。关联的恢复范围见[桌面视图与运行边界](plugins/dsh/README.md)。

原有前台／单步 CLI 仍可使用；这些命令应在目标仓库根目录运行：

```sh
flywheel run docs/my-task
flywheel run docs/my-task --mode full
flywheel run docs/my-task --pause-before review
flywheel run docs/my-task --only execute --packet M01-T01
flywheel run docs/my-task --only verify --packet M01-T01
flywheel status docs/my-task
flywheel stop docs/my-task
```

`--only` 与 `--pause-before` 不能同时使用，`--only execute` 不等于 Quick 或完整交付。前台 `run` 的退出码为 0 成功、1 用法／运行错误、3 升级／预算／单步失败、143 中断；控制命令的操作失败返回 1。

### 已安装的 skills

```sh
flywheel install-skills
flywheel install-skills --project /path/to/project
flywheel skills list
flywheel skills read flywheel-task
```

一条命令安装包含所选版本的全部 skills，当前源码为 11 个。默认把 Go 内嵌快照安装到 `~/.agents/skills`，`--project` 改为项目的 `.agents/skills`；上述命令可用于补装、检查或显式安装项目范围的副本。

每个快照保存带文件内容 hash 的 `.flywheel-snapshot` 清单。重跑先校验当前树：未改过的自有快照可更新，相同内容跳过；用户修改、新增／删除文件、链接或未标记目录都保留并报告冲突。更新按每个 skill 暂存、rename 替换，失败时回滚，不会因为存在一个标记就覆盖用户内容。旧纯文本标记仅在整棵目录与当前内嵌内容一致时迁移。冲突恢复见[升级教程](docs/getting-started.md#8-升级排错与卸载)；`--force` 只用于源码 `--link` 模式，不能绕过默认快照保护。

安装同时会在全局配置不存在时生成默认配置，已有配置保留。源码开发者才使用 `--link`。skills 是宿主准备／CLI 操作入口，宿主的发现规则和调用语法按其配置使用，不能承担重试或状态机逻辑；安装成功不等于所有宿主已经发现它们。可用入口涵盖 task、new、run、plan、exec、review、verify、status、stop、intake、garden。

## 5. 什么才算完成

执行者可以一起修改实现、单测和回归测试；冻结验收通过 packet 的 `boundaries.must-not-touch` 或配置的 `protected-test-patterns` 显式保护。100% 覆盖率仍是本项目的门禁，断言是否有效由验收和独立评审共同检查。

L1 在命令前后检查候选身份。验收过程中代码或契约变了，即使命令 exit 0 也不能继续交付。Quick 在实际候选上先收齐适用 L2/L3，再作覆盖所有需求的独立最终 Review；Full 仍在集成 checkout 运行里程碑 L2/L3 与 Judge。工作流策略也进入证据身份，模式变化不能复用旧绿灯。正常完成需要代码实际提交、合并到本地目标 checkout，并确认提交后的 Git tree 与已验证候选一致；commit hook 修改 index 或文件也不能绕过这一步。任务最终完成前再次检查持久化状态，重启不会把上次验收留下的未提交副作用当成交付。

Plan 只能直接修改任务目录内的 Spec 和 packet。它若修改外部验收、实现或运行配置，引擎保留现场并停止，不自动提交或丢弃。修订后的契约 fingerprint 会跨重启保存：旧 completed 单元的契约或验证策略改变时重新打开，早先已关闭的里程碑也会被重新调度。同一 run 对一个里程碑最多访问 3 次，再反复重开就升级给人。最终要求所有当前契约单元完成、里程碑关闭。

提交或合并失败保留工作区。交付已持久化而仅工作区清理失败时，可以完成，但必须保留 worktree 引用和 `cleanup-error`，供后续处理。非 Git 演练不能完成。Flywheel 任务引擎本身不会自动 push、发布或合并远端 PR；这些仓库管理操作按使用者的授权另行执行。

任务目录保存契约、状态、报告、复盘、升级记录和指标。`state.json` 只有 Go 可以写；执行报告先写在单元 worktree 内，再由 Go 校验并归档。日志、PID、运行锁、停止请求和其他易变运行文件排除在自动提交之外。

## 6. 验证与当前边界

```sh
make test
make coverage
make quality-gate
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check
npm test
npm run build:npm
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

Go 的交付门禁是 vet、格式检查与**逐函数 100% statement coverage**；statement coverage 不等于分支覆盖或真实模型质量。测试使用真实 Git、hook、index、worktree、子进程与协议桩，默认不需要账号或模型调用。

DSH 检查包括离线 worker 生命周期、事件与 usage 折叠、控制入口一致性、真实 React DOM 交互、Go → Node → SDK 协议桩，以及前轮官方 npm rc.5 的真实 Host 注册和 SDK 初始化／关闭。npm 检查覆盖安装与取消、同版包解析、GUI 安装记录及发行门禁；`test:install` 使用构建出的六个真实 npm 包和隔离环境验证安装，DSH 命令由协议桩替代，不使用用户 profile 或模型账号。

React DOM 与安装协议桩不代替 Mac 桌面验证。当前 Linux 环境没有可用的完整原生沙箱，SDK probe 在 prompt 前正确拒绝；这项结果证明拒绝路径，不能冒充 Mac Seatbelt 成功。

**尚需在用户 Mac 验证**：桌面安装与 UI 渲染、Seatbelt 探针、GUI 环境中的 CLI／凭证可见性、真实 Flash/GPT/Kimi/Codex 响应，以及真实机器重启后的恢复。当前不支持 DSH worker resume、附件传输或自动多 agent Teams 调度。Codex Pro 已通过公开 API 接入，完整策略的事后检查边界见配置指南。

## 7. 开发与备用构建

普通用户使用上面的 npm 安装流程。开发者需要 Go 1.27 或更高、Node.js 22.18.0 或更高及 Git，在仓库根构建：

```sh
go build -o ./bin/flywheel ./cmd/flywheel
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check
npm run build:npm
```

`build:npm` 先构建 worker，再在隔离目录打包 meta、四个平台包和 DSH 插件，输出到 `dist-npm/`。六个包版本必须一致；本地 tarball 和 Actions artifact 用于开发、验证或备用分发，不是日常用户的安装入口，也不等于已签名的 Mac 安装程序。源码构建及开发模式 skills 说明见[教程末节](docs/getting-started.md#9-开发与备用构建)。

维护者可以直接拉取源码，在本机构建后运行仓库内的 `npm run release:local` 完成交互发布，无需下载预构建归档。工具会运行候选和 registry 安装测试，再按六包依赖顺序提升 `latest`；已存在 release tag 的具体步骤见[从源码本地发布](https://github.com/ckanner/flywheel/blob/main/docs/releasing.md#5-从源码本地构建并发布)。

## 文档与代码入口

英文用户文档包括 [入门](docs/en/getting-started.md)、[配置](docs/en/configuration.md)、[工作流](docs/en/workflow.md) 和 [DSH 插件说明](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.en.md)。贡献者从 [Contributing](https://github.com/ckanner/flywheel/blob/main/docs/contributing.md) 开始，发行维护者见 [Releasing](https://github.com/ckanner/flywheel/blob/main/docs/en/releasing.md)。

| 路径 | 内容 |
|---|---|
| [docs/getting-started.md](docs/getting-started.md) | Mac 安装、首个任务、配置迁移与排错 |
| [docs/design.md](docs/design.md) | 权威设计、不变量、闭环与边界 |
| [会话生命周期方案](docs/plans/2026-11-07-session-lifecycle.md) | P1/P2 正常终态续接与回执恢复；P3 公开实时恢复 API、P4 真实实验仍待完成 |
| [docs/configuration.md](docs/configuration.md) | profiles、阶段覆盖、provider／channel 与预检语义 |
| [docs/plans/2026-11-03-codex-pro-default.md](docs/plans/2026-11-03-codex-pro-default.md) | 默认插件接入与验收计划 |
| [docs/plans/2026-11-01-dsh-integration.md](docs/plans/2026-11-01-dsh-integration.md) | 本轮实施和验收记录 |
| [docs/plans/2026-11-01-model-profiles-preflight.md](docs/plans/2026-11-01-model-profiles-preflight.md) | 配置、预检与面板后续实施计划 |
| [docs/plans/2026-11-01-npm-installation.md](docs/plans/2026-11-01-npm-installation.md) | npm 安装、GUI 入口与发行验证计划 |
| [plugins/dsh/README.md](plugins/dsh/README.md) | DSH 安装、独立 SDK profile 与本机检查 |
| [plugins/dsh/PROTOCOL.md](plugins/dsh/PROTOCOL.md) | worker v1 请求、事件、终态和 usage 契约 |
| [templates/](templates/) | 契约、配置模板与 JSON Schema |
| [internal/machine/](internal/machine/) | 状态机、契约修订、证据、生命周期 |
| [internal/drivers/](internal/drivers/) | DSH／Kimi／Codex 与保留的兼容 driver |
| [internal/verify/](internal/verify/) | 确定性验收、边界、Git 持久化与工作区 |
| [docs/design-history-v0.24.md](docs/design-history-v0.24.md) | 历史设计与当时的研究、环境记录 |

## 许可证

当前源码与 Flywheel npm 包标记为 `UNLICENSED`，尚未授予开源许可证。英文文档和公开 npm 分发不改变这一状态；正式开源时需由维护者选择许可证，并同步源码声明与全部包的 metadata。第三方依赖遵循各自许可证。

### 流程控制台（当前源码）

DSH 会话视图提供从左到右的任务画布、节点运行／重跑、按阶段配置、受管 Session 交互、声明式并行与串行集成、运行指标和 PR／Merge 回执。完整操作与当前能力边界见[控制台指南](docs/flow-console.md)。源码状态不等于 npm 已发布或本机已升级。

工作台默认以大画布展示任务总览，节点详情按需打开，任务步骤在独立子图中展开。“配置”自动读取并分组编辑，修改后才出现相邻的保存操作；新配置只影响后续启动。

可选、默认关闭的 JEV 影子评估源码实现见[使用说明](docs/jev-shadow.md)。开启仅记录建议，故障不阻断原流程；需要同版本 CLI／插件。
