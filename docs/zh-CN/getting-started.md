# 快速开始

[文档首页](../README.zh-CN.md) · [English](../en/getting-started.md) · 简体中文

先安装 Flywheel，配置实际要使用的后端，再运行一个小任务。本文对应当前仓库，安装时
应核对所选 npm 版本与本机版本；源码合并不会更新已有安装。CLI、内置引擎和 DSH 插件
须保持同一版本。

0.39.0 源码候选通过配套包 0.3.0 增加默认的个人 Antigravity ACP 视图。这份源码说明
不表示 npm `latest` 已经包含该功能；候选安装与发行核验见[发行手册](releasing.md)。

任务分支、契约与 PR 审阅的完整流程见[工作流指南](workflow.md)。
故障调查、Issue 修复和复杂项目的操作示例见[使用手册](usage-handbook.md)。

从 0.33 起，`@ckanner/flywheel` 一个包包含 CLI、DSH 界面／worker、内嵌技能和
四个平台引擎，不再需要独立平台包或首次运行下载引擎。从 0.32.x 升级前先退出 DSH，
再运行原来的安装命令；安装器替换旧桌面包登记，保留配置和任务历史，完成后重载 DSH。
历史 npm 版本继续保留。

## 1. 准备运行环境

| 依赖 | 用途 |
|---|---|
| Git，已配置提交身份 | Worktree 与本地交付提交 |
| Node.js 22.18.0 或更高，npm | 安装器、启动器及 JavaScript worker |
| DSH Mac 及桌面附带的 `dsh` 命令 | 桌面插件管理与 DSH SDK 执行 |
| 官方 Antigravity `agy` CLI 及原生账号登录 | 个人 ACP 使用 CLI 1.5.14；显式 machine harness 另外负责 Gemini 任务阶段 |
| Claude CLI 及原生订阅登录 | 通过 Claude Code Pro 细化实施计划、执行 frontend／multimodal 路由 |
| Codex 程序及 ChatGPT 账号登录 | 通过 Codex app-server 执行 backend／hard 路由、独立审查和 Judge |
| 已登录的 Kimi CLI | native agent 批量执行及可选 Garden |
| Go 1.27 或更高 | 仅本文 Go 示例与源码构建需要，npm 二进制安装不需要 |

插件使用 DSH `>=0.2.1-rc.5 <0.4.0-0 || ^0.2.2-0 || ^0.2.3-0 || ^0.3.0-0` 范围，包含 0.3 系列及其预发布版本。
常规升级不再逐版本追加白名单。这是尽力兼容窗口：如果某个版本确实存在不兼容变更，
必需的宿主服务或 SDK 权限／清理检查仍会失败关闭，并通过代码修复，不伪造宿主版本放行。DSH 0.2.2 自身要求 Node 24.9
或更高，应使用官方桌面附带的 CLI。安装前核对实际命令位置与版本：

```sh
git --version
node --version
npm --version
command -v dsh
dsh --version
kimi --version
codex --version
claude --version
agy --version
```

各后端沿用自己的安装和登录流程。个人 ACP 视图可以启动 Antigravity 官方登录助手，
由用户完成认证；Flywheel 不复制凭证或替换桌面程序。
Full 需要所配置的设计／规划、执行、审查与 Judge 后端；新 compact 任务还要先运行
联合设计与独立联合审查，再进入 Quick 执行和最终 Review，升级为 Full 后才需要
额外阶段。未使用的执行路由不成为该任务的运行前提。

Claude 在打包上仍是 optional peer，但默认实施规划和 frontend 绑定使用它；这些阶段
需要 worker 依赖树中的精确 0.7.0 配套包、兼容的 Claude CLI 和受支持原生订阅。
详见[Claude Code Pro](claude-code-pro.md)。正式 Gemini 任务阶段使用锁定的 Antigravity
machine adapter 与官方 `agy` stream-json；独立的个人桌面入口默认使用 ACP，详见
[Antigravity](antigravity.md)。

## 2. 安装

在能找到桌面 `dsh` 的终端执行：

```sh
npm view @ckanner/flywheel version
npx -y @ckanner/flywheel@latest install
flywheel version
```

固定安装时，把 `@latest` 换成已发布的精确版本。Agent 需要单个结构化结果时，在安装
命令后加 `--json`。安装器依次完成：

1. 全局安装本次精确版本，解析同版本平台二进制与 workers。
2. 安装内嵌 skills；仅在全局配置不存在时创建它。
3. 保存绝对路径 GUI 启动器与安装时工具路径。
4. 在所选 DSH profile 中注册同版本 Flywheel 插件。
5. 运行本地诊断，报告已完成步骤和阻断项。

`ok:true` 表示安装完成，`ready:true` 表示本次本地诊断没有阻断。缺少工具可以产生
`ok:true, ready:false`，具体原因见 `diagnostic.checks`；两者都不能证明账号或真实
推理通过。安装失败会保留部分结果，供定位原因后重试。

### 安装参数

| 参数 | 作用 |
|---|---|
| `--prefix PATH` | 选择 npm 全局安装 prefix |
| `--project PATH` | 将项目 skills 安装到已有项目的 `.agents/skills` 并诊断该项目；CLI 仍全局安装 |
| `--dsh-bin PATH` | 选择已核实的桌面 DSH 命令 |
| `--profile NAME` | 桌面插件目标 profile，默认 `desktop`，与 worker SDK profile 分开 |
| `--no-dsh` | 跳过桌面插件注册 |
| `--no-global` | 配置已有持久安装，不再次全局安装 |
| `--timeout-ms N` | 单步骤超时，默认 600000 毫秒，范围 100–1800000 |
| `--json` | 输出一个结构化安装结果 |

使用用户可写的 prefix：

```sh
npx -y @ckanner/flywheel@latest install --prefix "$HOME/.local"
export PATH="$HOME/.local/bin:$PATH"
flywheel version
```

把实际 prefix 的 `bin` 写入 shell 配置。必要时传入
`--dsh-bin /absolute/path/to/dsh`，先核实路径，不猜测 app bundle 内的位置。
普通 `npm install --global @ckanner/flywheel` 只放置包文件，之后须显式运行
`flywheel setup --no-global`。不要用 `--no-global` 把临时 npx 缓存当成永久 GUI 入口。

## 3. 配置桌面与 SDK 后端

按 DSH 要求重新载入，进入已有会话，在内置视图旁选择 **Flywheel**。它排在“统计”
之后，必要时检查更多菜单；新会话的空白首页不展示会话视图。控制台提供画布、配置
和运行记录，DSH 继续保留对话、轨迹、输入区与侧栏。

桌面插件注册默认使用桌面附带的命令和 `desktop` profile。独立 npm 安装的 DSH CLI
不能管理该保留 profile。显式选择的 DSH worker 运行在独立 **`sdk` profile**，不会
继承对话当前模型或全部桌面插件。默认订阅 workers 直接调用各自原生客户端。

### 开始个人 Antigravity 会话

在已有提交的 Git 仓库中打开对话，选择独立的 **Antigravity** 视图。它使用官方 agent
自己的循环，不启动 DSH 父模型或 Flywheel 自动任务。空白会话可以先用这个人工
命令建立入口：

```text
/flywheel-antigravity
```

1. 安装官方 CLI **1.5.14**，在视图中执行 **检查 CLI**。未知版本、字节改变和包装
   程序都会被拒绝，不自动替换。插件不下载 CLI。
2. 选择 **新建独立会话**；空命令本身不启动 CLI。每次新建都根据来源仓库已提交的 HEAD
   建立独立 `antigravity/<id>` 分支和 worktree；不复制未提交修改，不接受非 Git
   目录或没有提交的仓库。
3. 需要认证时选择 **官方 Google 登录**，完成已核验 CLI 自己的 `agy acp --login`
   流程，将其授权码或回调 URL 输入视图的密码字段。助手退出后再连接或恢复会话
   核对认证；助手本身成功不证明模型访问通过。
4. 从原生会话报告的选项中选择 Gemini，在视图发送任务并回答原生工具权限请求。
   只有会话声明支持图片输入时，才能选择或粘贴图片。
5. **停止工作** 请求原生取消；**断开会话** 请求关闭会话并停止所拥有的进程组。
   切换视图仅断开界面订阅，断开会话保留 worktree 修改，也不退出 Google 账号。

任务、图片、登录码和权限答复只通过认证视图发送。人工命令仅接受 `doctor`、
`connect`、`list`、`status` 和带精确会话 `id` 的 `cancel`，不要把代码或凭据放进去。
发送或清理结果不明时保留原始记录，不重放。使用 Git 主动审阅并整合 worktree
修改；Host 拒绝指向这些 ACP 所属工作区的 machine 变更，但不全局锁定其他编辑器或
直接 Go CLI。

视图、恢复和配置详见[交互视图](flow-console.md#交互式-antigravity-视图)及
[独立插件设置](configuration.md#antigravity-与-gemini)。下面的自动任务示例继续使用
正式 machine 后端；ACP 完成不会推进其 Plan、Execute、Review 或 JEV 状态。

### 正式任务的账号选择

默认配置声明 `subscription-only`，使用 Gemini、Claude、Codex 与 Kimi 原生客户端，
没有 DeepSeek 路由、API Key 后备或自动付费点数后备。各客户端保留自己的正常登录；
DSH 对话选择模型不会替独立 worker 登录。完整阶段／profile 表与准入限制统一维护在
[模型策略](model-policy.md)。

Codex Pro 0.10.2 直接使用官方 app-server 并核对 ChatGPT 账号路由；Claude Code Pro
0.7.0 在任务发送前核对原生订阅及 extra usage 关闭。Antigravity 拒绝 API／自定义
provider 和启用 credits fallback 的配置；Kimi 使用 Kimi Code OAuth 的受管原生
agent 模式。这些控制不证明剩余额度或整个账号绝无额外费用。不把凭证写入任务文件
或 Git。旧的显式 DSH 配置在迁移前仍是独立兼容选择，不成为新预设的后备渠道。

未选择 `native-profile` 时，DSH worker 的临时策略禁用委派与 Web 工具。显式 profile
可启用受管 PTC Agent、Team、Workflow、Web 与可选 LCU；辅助模型压缩仍关闭。
桌面视图和 preset 不会自动配置独立 worker，也不会把完整 worker 历史合入父会话轨迹。
配置方法见[原生能力](native-capabilities.md)，生命周期边界见[插件文档](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.zh-CN.md)。

### 桌面启动器

Setup 写入 `~/.config/flywheel/npm-launcher` 和安装记录，保存绝对 Node／CLI 路径
及已核实的 DSH 入口。显式 `flywheelBinary` 优先于托管启动器；记录缺失时使用本包 CLI，记录损坏或 launcher 被改动则明确失败。

移动 Node／npm prefix，或安装原先缺失的工具后，在能找到它们的终端中保留原参数，
重跑 `flywheel setup --no-global`。处理冲突前先保留定制 launcher。终端 export 不会
更新已经运行的桌面进程。

## 4. 选择配置与 skills

配置按任务、项目、全局、内嵌默认的顺序选择首个完整文件，不逐字段合并。先查看真实
来源：

```sh
flywheel config inspect --cwd /absolute/path/to/repo --json
flywheel template flywheel.config.example.json
flywheel template flywheel.profiles.example.json
```

升级保留已有模型、权限、历史和调度设置。采用新订阅 CLI 预设时先预览，确认账号与
保留的权限符合预期后再应用：

```sh
flywheel config migrate --preset subscription-cli --cwd /absolute/path/to/repo --json
flywheel config migrate --preset subscription-cli --cwd /absolute/path/to/repo --apply --json
```

任务级配置需加 `--task-dir`。迁移先备份所选文件，保留无关设置与权限；历史
`subscriptions` 预设不选择这份新的四客户端策略。迁移不会自动让旧配置
启用 scoped 历史或并行调度。完整采用安装版本模板时，在全局／项目配置编辑器载入
默认值、审阅并保存。优先级、权限和迁移规则见[配置指南](configuration.md)。

当前源码内嵌 11 个 skills，作为可选宿主入口；CLI 和控制台也能独立使用。查看或补装
项目 skills：

```sh
flywheel skills list
flywheel skills read flywheel-task
flywheel install-skills --project /absolute/path/to/repo --json
```

沿用宿主的技能发现规则。安装器拥有且未修改的快照可更新，修改过或不属于安装器的
文件会保留并报告冲突。DSH 使用 `/flywheel-task <目标>` 准备并启动，
`/flywheel-new <目标>` 运行设计与审查后等你批准，`/flywheel-run <任务路径>` 运行已有任务；Kimi
对应使用 `/skill:` 前缀。

## 5. 运行无推理预检

在目标仓库中执行：

```sh
flywheel doctor --cwd "$PWD" --json
flywheel doctor --cwd "$PWD" --probe --json
```

本地检查覆盖配置、目录、命令／版本及声明的环境变量。`--probe` 会初始化 DSH SDK
会话，核对接受的配置、目录、权限和关闭结果，不发送推理 prompt，也不运行验收命令。
Provider 初始化可能查询目录或写缓存，探测会话使用临时存储，不进入用户会话列表。

Codex Pro 诊断导入锁定 API，检查 lifecycle／recovery 导出及本机命令，不证明模型
授权、某条已保存历史的恢复或实际推理强度。正式 Codex 调用另外在 `thread-ready`
核对完整策略，再由 Go ACK 放行；这不使无推理诊断等同于真实账号验收。

Antigravity 诊断检查已安装 CLI 和订阅控制，不发送业务任务；Claude 诊断核对配套包
与本地 CLI，发送 prompt 前的订阅检查仍由实际运行执行。两者都不能替代另行授权的
真实账号验收。

结果分为 `pass`、`fail`、`warn`、`unknown`、`skipped`，fail 退出码为 1。
按任务预检会读取 Spec 或持久化策略，仅检查适用绑定，不提前应用随后启动时的
`--mode` 覆盖：

```sh
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --probe --json
```

预检不创建 Flywheel 状态或 worktree。桌面 PATH／profile／环境不同时，再从桌面核对。
这些诊断适合安装后或配置变化时使用，不是每个任务必须逐项点击的按钮。

## 6. 准备任务

### 已有仓库

在 DSH 对话中输入：

```text
/flywheel-task 修复已定位的问题，补齐回归测试并验证结果。mode=auto
```

助手研究仓库，按照有效 worktree 策略选择或复用任务集成目录，记录薄 brief，
启动 Go 管理的设计与独立审查。Task 沿用已有实施授权；`/flywheel-new` 在审查后停下
等待你批准。通过 Flywheel 视图反馈、Plan 修订、仅批准或批准并开始，无需先填写路径
表单。详见[方案设计与审查](reviewed-planning.md)。

区分**项目工作区**、**任务执行根**与**任务文件目录**：项目是稳定仓库归属，执行可
位于 linked worktree，其契约放在 `docs/<slug>`。准备结果应报告真实路径。后续
packet 执行目录由引擎管理或登记，`state.json` 也由引擎维护，详见
[选择任务执行目录](workflow.md#2-选择任务执行目录)。

### 调查故障或补充证据

在 Task 中明确只要报告，还是排查并修好。Go 执行有界取证，由配置的 Gemini 分析角色
形成报告。`awaiting-report` 接续分析；未知 pending 调用只核对原回执。报告有支持结论
后，已有修复授权的任务在原目录进入薄 brief 准备，保留原范围和累计用量。

报告后收到新日志时，在原对话要求使用新证据继续原任务。宿主通过 `control investigate`
提交 `action:continue`、当前 revision 与完整下一轮契约，再运行一次。目标、意图和
修复范围不变；旧报告与已消耗预算继续保留。详见[调查续轮](investigation.md#同一任务补充证据继续新一轮)。

### 独立的首次运行示例

内嵌脚本创建一个独立 Go 加法任务，故意提供错误实现和受保护的验收测试。需要
Go 1.27 或更高及尚未使用的目标目录。该示例没有 workflow 评估字段，因此走 Full；
仅改模式参数不能使它具备 Quick 资格。

```sh
FLYWHEEL_PACKAGE="$(npm root --global)/@ckanner/flywheel"
bash "$FLYWHEEL_PACKAGE/docs/examples/create-first-task.sh" "$HOME/flywheel-demo"
cd "$HOME/flywheel-demo"
git init -b main
```

使用自定义 prefix 时，通过 `npm root --global --prefix "$HOME/.local"` 找到包。
CLI 不在 PATH 时，调用脚本前以 `FLYWHEEL_BIN` 指定其绝对路径。脚本仅写文件并读取
模板，不调用模型或提交。

运行生成 packet 中的全部验收命令，示例自带检查为：

```sh
go test ./...
go test -coverprofile=coverage.out . &&
  go tool cover -func=coverage.out |
  awk '$NF != "100.0%" && $NF != "100%" { bad=1 } END { exit bad }'
```

首次失败符合预期。把实际结果写入 Spec，审阅冻结边界，再提交示例并创建任务分支：

```sh
git add go.mod sum.go acceptance .gitignore flywheel.config.json docs/add
git diff --cached
git commit -m "docs: prepare first flywheel task"
git worktree add -b task/add ../flywheel-demo-add
cd ../flywheel-demo-add
```

此独立示例先生成契约再创建任务 worktree；已有仓库按[工作流指南](workflow.md)的顺序
准备。

## 7. 启动、观察、停止与恢复

以下命令调用已配置的真实后端。在示例任务 checkout 中运行，或替换实际任务名称：

```sh
flywheel config inspect --cwd "$PWD" --task-dir "$PWD/docs/add" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/add" --probe --json
flywheel control start --cwd "$PWD" --task-dir "$PWD/docs/add"
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/add"
```

DSH 对应入口为 `/flywheel-run <绝对任务路径>` 和 `/flywheel-status <同一路径>`。
画布跟随当前会话的显式工具关联；没有关联时，读取 host session ID 精确匹配的最新运行。
准备中的任务或独立终端任务，需要宿主助手在目标会话通过 `flywheel_control` 的
`action: "status"`，携带实际任务与执行路径建立关联。单独运行终端
`flywheel control status` 不会建立该会话关联。从 0.34.2 起，多个关联任务显示“本会话任务”导航，单任务直接展示画布；不提供可编辑路径表单，
其他会话和无归属运行不显示，详见[流程控制台](flow-console.md)。

界面区分本阶段结果和整体交付，自动刷新运行状态与登记任务。超时或断连可能留下
未确认操作：保留原路径，先核对原任务再提交新变更。旧快照、关闭页面或切换会话
不能证明引擎已停止。宿主明确证明控制进程启动前拒绝，与结果未知是不同状态。

需要在 Review 前暂停的前台运行时，使用下面命令**替代**后台启动：

```sh
flywheel run docs/add --pause-before review
```

`control launch`／`start` 不接受 `--pause-before`，该参数也不能与 `--only` 合用。
单环命令不等于整任务完成，详见[单阶段操作](workflow.md#单阶段操作与权限)。

查看与停止：

```sh
flywheel status docs/add
tail -n 80 docs/add/orchestrator.log
flywheel control stop --cwd "$PWD" --task-dir "$PWD/docs/add"
```

核对先前运行并解决真实错误后，才从原路径恢复，保留状态、报告和 worktree。
派发、结果或清理未知时，切换 fresh 历史或另一个后端不能绕过，详见[恢复指南](session-recovery.md)。

首次实机运行应检查 completed、全部需求覆盖、冻结验收未变、独立 Review、报告及
本地 Git 交付。真实模型、延迟与用量是独立于预检的观测。创建／合并 PR 是另行授权的
[控制台操作](flow-console.md#远端-pr-与-merge)。

## 8. 升级、排错与卸载

### 升级与修复

先停止活动任务并核对状态，保留配置和定制内容，再用原有 prefix／项目／DSH 参数
重复安装。核对 `flywheel version`、安装报告和桌面插件版本，并按 DSH 要求重载。
`flywheel setup --no-global` 用于修复当前已安装版本。

在 Flywheel 自身源码目录外执行 npx，避免 npm 用本地同名包满足请求。必要时显式
全局安装所选已发布版本，再从同一 prefix 运行 setup。已有配置继续保留，判断项目／
全局修改是否生效前，先检查更高优先级任务文件。

### 常见问题

| 现象 | 先检查什么 |
|---|---|
| 缺少命令或新布局 | 已安装 CLI／插件版本、PATH 和显式 `flywheelBinary` |
| 视图栏没有 Flywheel | 已有会话、更多菜单、插件启用与宿主重载 |
| 安装后 `ok:true, ready:false` | `diagnostic.checks` 中具体阻断 |
| 全局 prefix 不可写 | 使用用户可写的 `--prefix` 及其 `bin` |
| desktop profile 被拒绝 | 使用桌面附带的 DSH 命令，而非独立 npm CLI |
| 终端可用、桌面不可用 | 托管 launcher、工具路径、SDK profile 与 `DSH_HOME`；工具变化后重跑 setup |
| Provider 未注册或未授权 | Worker 的 SDK profile 和对应 provider 正常登录流程 |
| 模型修改未生效 | 实际配置来源及引擎重启 |
| 改渠道名没切账号 | Channel 是配置／计量标签，真实授权由后端决定 |
| Doctor 显示 unknown 或 skipped | 阅读具体原因，两者都不代表真实推理通过 |
| 费用缺失 | 渠道未报告 usage／cost，unknown 不是零 |
| 验收 exit 0 仍失败 | 候选漂移、超时、取消、输出截断或清理错误 |
| 隔离目录不可用 | 实际执行模式；并行自动管理 worktree 时创建失败即停止，不回退 |
| 启动／停止／清理未确认 | 重试前核对原任务状态、日志、所属进程与可信回执 |
| Git 交付失败 | 保留的 worktree、hook 与差异，处理原因而不强制清理 |
| Skill 或 launcher 归属冲突 | 备份具体定制并对照，不改写完整性清单 |

安装不是跨 npm、DSH 和 skills 的单个事务。修正报告中的原因后重试，不删除无关
用户文件。验收命令需要额外工具变量时，用 `verify.env-allowlist` 显式列出变量名，
不要把全部模型凭证注入测试。

### 卸载

停止并核对活动任务，通过 DSH 正式插件管理入口移除桌面 Flywheel 插件，然后使用
同一个 prefix 卸载全局 CLI：

```sh
npm uninstall --global @ckanner/flywheel
```

配置、任务账本、skills、安装记录和 worktree 保留，需另行决定如何处理。卸载不会移除
DSH／Kimi／Codex 或账号凭证。源码构建与贡献检查见[贡献指南](contributing.md)，
维护者发布见[发行指南](releasing.md)。
