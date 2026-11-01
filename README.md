# Flywheel（飞轮）

Flywheel 用确定性 Go 引擎驱动 Plan、Execute、Review 和分层验收，把已经明确的需求变成有边界、可验证、可持久化的软件交付。模型负责规划、实现和评审；引擎负责状态、重试、预算、工作区、验收、提交与停止。

本分支的 **v0.25 实施版**以 DSH Mac 桌面为入口：Kimi CLI 规划，DSH 原生 DeepSeek Flash 执行，DSH 原生 GPT 评审，较难的执行任务可走独立 Codex CLI。DSH 插件提供控制面板和工具，Go 仍是唯一状态所有者。设计依据见 [docs/design.md](docs/design.md)，仓库约束见 [AGENTS.md](AGENTS.md)。

**本实现与 npm 发行版本分开管理。请使用包含 DSH 集成的源码构建 Go 二进制和插件；PR 合并不代表已发布 npm 包，旧发行版可能没有 `control`、DSH worker 和新的完成门禁。** 旧版本的选型、维护机环境与演练记录保存在[历史设计](docs/design-history-v0.24.md)，不代表当前默认值或用户机器状态。

首次使用请按 [Mac 安装与首个任务教程](docs/getting-started.md) 完成安装、配置、无推理检查和小任务验收。本文保留各接口与设计边界的速查。

## 1. 从源码构建

需要 Go 1.27 或更高、Node.js 22 或更高，以及 Git。生产任务必须位于 Git 仓库；非 Git 目录可以用于部分步骤演练，但不能标记为 `completed`。

在包含本轮改动的 Flywheel 源码目录运行：

```sh
go version
node --version
mkdir -p bin
go build -o ./bin/flywheel ./cmd/flywheel
./bin/flywheel control --help

cd plugins/dsh
npm ci --ignore-scripts
npm run check
npm pack
```

Go 产物是 `bin/flywheel`；插件产物是 `plugins/dsh/ckanner-dsh-plugin-flywheel-0.1.0.tgz`。插件构建不需要额外打包器，不运行模型。后续示例中的 `flywheel` 指这个新二进制：可以加入自己的 PATH，也可以始终使用绝对路径。

## 2. 安装 DSH 入口与独立 worker

本实现针对已核对接口的 **DSH Mac 桌面 0.2.1-rc.5**。先核对 About、`dsh --version` 和命令实际来源。它不要求 alpha、OAR、Forge、Teams 或一直存活的桌面父智能体。

在自己的 Mac 上安装本地构建包：

```sh
npm install --global /absolute/path/ckanner-dsh-plugin-flywheel-0.1.0.tgz
dsh plugin --profile desktop add /absolute/path/ckanner-dsh-plugin-flywheel-0.1.0.tgz
```

第一条安装 `flywheel-dsh-worker`；第二条必须使用**桌面安装的 `dsh` 命令**，将控制插件加入桌面 profile。普通 npm CLI 不能管理保留的 `desktop` profile。也可以使用桌面的 Plugins 页面安装本地包，然后按宿主提示重新载入。不要猜测 `.app` 内部可执行文件路径。

原生 worker 使用独立的 SDK profile。桌面装有某个 provider，不代表 SDK profile 已经加载或授权。DeepSeek 使用实际 provider `deepseek-official`；GPT 原生路由要求在 SDK profile 中配置对应的 ChatGPT adapter，例如本轮核对的版本：

```sh
dsh plugin --profile sdk add dsh-plugin-chatgpt@0.1.17
```

按 provider 自身的正常流程完成授权，保持所选 `DSH_HOME` 与配置来源一致。安装 adapter 不会自动迁移桌面凭证，也不证明账号可以调用默认表里的所有模型。`subagent_codex_pro` 是外部 agent 工具，不是这个 SDK 入口的 LLM provider；本轮没有连接或修改用户另行加固的 Codex Pro adapter。

### worker 入口配置

| 环境变量 | 默认值与作用 |
|---|---|
| `FLYWHEEL_DSH_WORKER` | PATH 中的 `flywheel-dsh-worker`；可设为本包已构建的 `dist/worker.js` 绝对路径 |
| `FLYWHEEL_DSH_BIN` | PATH 中的 `dsh`；建议固定为已核对的官方命令入口 |
| `FLYWHEEL_DSH_PROFILE` | `sdk`；必须是 SDK profile，不能是 `desktop` |
| `DSH_HOME` | 交给 DSH 的自定义配置根；不设置时沿用 DSH 自己的默认行为 |
| `FLYWHEEL_KIMI_BIN` | PATH 中的 `kimi`；可指定 Kimi CLI 的绝对路径 |

Codex CLI 通过 PATH 中的 `codex` 启动。所有选定 CLI、Node 与账户配置必须对**启动 Go 引擎的进程环境**可见。

终端里的 `export` 不会改变已经运行的桌面程序环境。如果 GUI 的 PATH 不完整，可以把桌面插件的 `flywheelBinary` 指向自己的启动脚本，例如：

```sh
#!/bin/sh
export PATH="/absolute/path/to/your-cli-bin:$PATH"
export FLYWHEEL_DSH_WORKER="/absolute/path/to/flywheel/plugins/dsh/dist/worker.js"
export FLYWHEEL_DSH_BIN="/absolute/path/to/dsh"
export FLYWHEEL_DSH_PROFILE="sdk"
export FLYWHEEL_KIMI_BIN="/absolute/path/to/kimi"
exec /absolute/path/to/flywheel/bin/flywheel "$@"
```

将脚本设为可执行，在桌面 profile 的 `cordis.patch.yml` 中覆盖配置：

```yaml
- id: flywheel
  config:
    flywheelBinary: /absolute/path/to/your-flywheel-launcher
    controlTimeoutMs: 60000
```

也可直接指定新 Go 二进制的绝对路径。这里配置的是 DSH 启动入口，不是 Flywheel 的 `state.json`。

### 不调用模型的检查

在实际目标仓库目录、使用与引擎 worker 相同的环境变量执行：

```sh
node --input-type=module -e 'console.log(JSON.stringify({type:"start",protocol:1,requestId:"local-doctor",cwd:process.cwd(),provider:"deepseek-official",model:"deepseek-flash",reasoningEffort:"high",permissionMode:"read-only",prompt:"Not sent in doctor mode",timeoutMs:20000}))' \
  | flywheel-dsh-worker --doctor
```

doctor 启动本地 SDK、校验模型配置、创建会话并运行沙箱中的无副作用探针，**不发送推理 prompt**；provider 初始化可能查询模型目录。成功应有 `ready` 与 `status:ok` 的 `result`，usage/cost 为 `null`。它不能证明远端凭证、订阅额度或真实模型响应可用。缺少完整沙箱、cwd 不符或权限无法实施时会在模型请求前拒绝；不要用 full access 绕过。

完整安装、权限、生命周期说明见 [DSH 插件 README](plugins/dsh/README.md)，线协议见 [PROTOCOL.md](plugins/dsh/PROTOCOL.md)。

## 3. 当前默认路由

配置模板是 [templates/flywheel.config.example.json](templates/flywheel.config.example.json)。`harness` 指执行后端；DSH 的 `channel` 必须是 SDK 实际注册的 provider ID；`model` 与 `thinking` 逐调用传入。

| 阶段或执行路由 | Harness | Provider／渠道 | 模型 | Thinking |
|---|---|---|---|---|
| Plan | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high`，匹配身份后可恢复会话 |
| Execute `default` | DSH SDK | `deepseek-official` | `deepseek-flash` | `high` |
| Execute `frontend` | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high` |
| Execute `hard` | Codex CLI | `chatgpt-subscription` | `gpt-6.1-sol` | `high` |
| Review | DSH SDK | `chatgpt` | `gpt-6.1-sol` | `high`，只读 |
| Judge | DSH SDK | `chatgpt` | `gpt-6-astra` | `high`，里程碑终验 |
| Intake | DSH SDK | `deepseek-official` | `deepseek-flash` | `low` |
| Garden | Kimi CLI | `kimi-subscription` | `kimi-code/k3.1-pro` | `high` |

这些是可编辑的初始绑定，并非模型排名或性能承诺。DeepSeek 只用 Flash；当前 Mac rc.5 的合法 effort 为 `off/low/high/max`，没有 `medium`。双 judge 默认关闭，也不默认跨渠道付费降级。`multimodal` 暂无默认绑定：当前 bridge 只有文本契约，模型名称或目录里的 vision 标记不能证明附件已正确传递。

Kimi 使用原生 CLI print/stream-json，不使用 ACP；恢复的是会话历史，不是常驻后台进程。该模式不提供本适配器可宣称的 OS 级只读／工作区沙箱，worktree 只是代码隔离。DSH/Codex 使用明确的 workspace-write 或 read-only 与禁止扩权策略。具体账号的可用模型、额度和使用授权需要由所选服务确认。

usage 或美元成本未上报时保留 unknown，不能把订阅调用记成免费。默认 soft token budget 会报告未知量；hard budget 无法证明未知用量未超额时停止。汇总数字是已知量下限。

### 迁移已有配置

配置按**第一个存在的完整文件**选择，不逐字段合并：任务目录 → 项目根 → 全局 `~/.config/flywheel/flywheel.config.json` → 二进制内嵌模板；`FLYWHEEL_CONFIG` 可指定全局配置路径。已有文件不会因升级自动改写。

可用新二进制查看模板，再审阅并迁移自己的配置：

```sh
flywheel template flywheel.config.example.json
```

`drivers` 选择 `real/mock`，不是模型或 provider 名称。模型在 `loops` 和 `routes` 配置。修改运行配置后重启引擎；Plan 不允许悄悄改模型、权限或其他运行配置。

## 4. 创建、运行和观察任务

先在目标 Git 仓库准备并审阅 `docs/<slug>/` 中的 Spec、packet 与配置：需求编号、L1/L2/L3 命令、禁止修改的验收路径和失败出口都应明确。模板可通过 `flywheel template` 查看；可选的 `flywheel-new` skill 用于辅助生成，最终仍由人确认契约。不要把 DSH 面板当成需求生成器。

桌面面板位于会话输入区底部的 **Flywheel** 按钮。输入绝对任务路径和仓库路径后，可以启动、刷新和停止。也可使用人工命令：

```text
/flywheel {"action":"status","taskDir":"/repo/docs/my-task","cwd":"/repo"}
```

面板、人工命令和 `flywheel_control` 模型工具复用同一组 Go 接口：

```sh
flywheel control start --task-dir /repo/docs/my-task --cwd /repo
flywheel control status --task-dir /repo/docs/my-task --cwd /repo
flywheel control stop --task-dir /repo/docs/my-task --cwd /repo
```

`start` 派生独立后台引擎，日志进入任务目录的 `orchestrator.log`；关闭面板或桌面不等于停止任务。`status` 只读。`stop` 向本次运行提交带随机 token 的停止请求，等待本次引擎退出受管调用栈、释放任务 lease 后才保存停止状态；不按文件里的 PID 盲目发送信号。引擎只管理本次拥有的进程组；已观测的 `CleanupError` 仍保留在证据和指标中、阻止交付，需要按错误记录处理，`stopped` 不代表任意逃逸进程已清理。停止等待最长 45 秒，桌面入口预留 60 秒。

控制接口始终返回一个 protocol 1 JSON；`ok` 表示控制操作成功，任务是否完成应看 `state.status`。若启动就绪超时，后台引擎可能仍在加载：先刷新状态和查看日志，不要另起重复任务。

原有前台／单步 CLI 仍可使用；这些命令应在目标仓库根目录运行：

```sh
flywheel run docs/my-task
flywheel run docs/my-task --pause-before review
flywheel run docs/my-task --only execute --packet M01-T01
flywheel run docs/my-task --only verify --packet M01-T01
flywheel status docs/my-task
flywheel stop docs/my-task
```

`--only` 与 `--pause-before` 不能同时使用。前台 `run` 的退出码为 0 成功、1 用法／运行错误、3 升级／预算／单步失败、143 中断；控制命令的操作失败返回 1。

### 可选 skills

```sh
flywheel install-skills
flywheel install-skills --project /path/to/project
flywheel skills list
flywheel skills read flywheel-new
```

默认把内嵌快照安装到 `~/.agents/skills`，`--project` 改为项目的 `.agents/skills`。重跑会刷新带 `.flywheel-snapshot` 标记的自有目录，并替换旧链接／文件；没有该标记的真实目录会拒绝覆盖，需要先审阅并自行备份处理。`--force` 只用于源码 `--link` 模式，不会让默认模式删除用户目录。手工改过的自有快照也会被刷新，升级前先保存自定义内容。

安装同时会在全局配置不存在时生成默认配置，已有配置保留。源码开发者才使用 `--link`。skills 只是 CLI 操作按钮，宿主的发现规则和调用语法按其配置使用，不能承担重试或状态机逻辑；安装成功不等于所有宿主已经发现它们。可用按钮涵盖 new、run、plan、exec、review、verify、status、stop、intake、garden。

## 5. 什么才算完成

执行者可以一起修改实现、单测和回归测试；冻结验收通过 packet 的 `boundaries.must-not-touch` 或配置的 `protected-test-patterns` 显式保护。100% 覆盖率仍是本项目的门禁，断言是否有效由验收和独立评审共同检查。

L1 在命令前后检查候选身份。验收过程中代码或契约变了，即使命令 exit 0 也不能继续交付。Review 同样必须针对当前候选；最终 L2/L3 与 Judge 还会核对主 checkout 的前后身份。正常完成需要代码实际提交、合并到本地目标 checkout，并确认提交后的 Git tree 与已验证候选一致；commit hook 修改 index 或文件也不能绕过这一步。任务最终完成前再次检查持久化状态，重启不会把上次验收留下的未提交副作用当成交付。

Plan 只能直接修改任务目录内的 Spec 和 packet。它若修改外部验收、实现或运行配置，引擎保留现场并停止，不自动提交或丢弃。修订后的契约 fingerprint 会跨重启保存：旧 completed 单元的契约或验证策略改变时重新打开，早先已关闭的里程碑也会被重新调度。同一 run 对一个里程碑最多访问 3 次，再反复重开就升级给人。最终要求所有当前契约单元完成、里程碑关闭。

提交或合并失败保留工作区。交付已持久化而仅工作区清理失败时，可以完成，但必须保留 worktree 引用和 `cleanup-error`，供后续处理。非 Git 演练不能完成。Flywheel 任务引擎本身不会自动 push、发布或合并远端 PR；这些仓库管理操作按使用者的授权另行执行。

任务目录保存契约、状态、报告、复盘、升级记录和指标。`state.json` 只有 Go 可以写；执行报告先写在单元 worktree 内，再由 Go 校验并归档。日志、PID、运行锁、停止请求和其他易变运行文件排除在自动提交之外。

## 6. 验证与当前边界

```sh
make test
make coverage
make quality-gate

cd plugins/dsh
npm run check
```

Go 的交付门禁是 vet、格式检查与**逐函数 100% statement coverage**；statement coverage 不等于分支覆盖或真实模型质量。测试使用真实 Git、hook、index、worktree、子进程与协议桩，默认不需要账号或模型调用。

本轮已做的 DSH 检查包括离线 worker 生命周期、事件与 usage 折叠、控制入口一致性、Go → Node → SDK 协议桩，以及官方 npm rc.5 的真实 Host 注册和 SDK 初始化／关闭。当前 Linux 环境没有可用的完整原生沙箱，doctor 在 prompt 前正确拒绝；这项结果证明拒绝路径，不能冒充 Mac Seatbelt 成功。

**尚需在用户 Mac 验证**：桌面安装与 UI 渲染、Seatbelt 探针、GUI 环境中的 CLI／凭证可见性、真实 Flash/GPT/Kimi/Codex 响应，以及真实机器重启后的恢复。当前不支持 DSH worker resume、附件传输、外部 Codex Pro adapter 消费或自动多 agent Teams 调度。

## 文档与代码入口

| 路径 | 内容 |
|---|---|
| [docs/getting-started.md](docs/getting-started.md) | Mac 安装、首个任务、配置迁移与排错 |
| [docs/design.md](docs/design.md) | v0.25 权威设计、不变量、闭环与边界 |
| [docs/plans/2026-11-01-dsh-integration.md](docs/plans/2026-11-01-dsh-integration.md) | 本轮实施和验收记录 |
| [plugins/dsh/README.md](plugins/dsh/README.md) | DSH 安装、独立 SDK profile 与本机检查 |
| [plugins/dsh/PROTOCOL.md](plugins/dsh/PROTOCOL.md) | worker v1 请求、事件、终态和 usage 契约 |
| [templates/](templates/) | 契约、配置模板与 JSON Schema |
| [internal/machine/](internal/machine/) | 状态机、契约修订、证据、生命周期 |
| [internal/drivers/](internal/drivers/) | DSH／Kimi／Codex 与保留的兼容 driver |
| [internal/verify/](internal/verify/) | 确定性验收、边界、Git 持久化与工作区 |
| [docs/design-history-v0.24.md](docs/design-history-v0.24.md) | 历史设计与当时的研究、环境记录 |
