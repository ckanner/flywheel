# Flywheel DSH 插件

[English](README.md) · 简体中文 · [项目介绍](https://github.com/ckanner/flywheel/blob/main/README.zh-CN.md) · [文档目录](https://github.com/ckanner/flywheel/blob/main/docs/README.zh-CN.md)

`@ckanner/dsh-plugin-flywheel` 将 DeepSeek Harness（DSH）连接到 Flywheel 的
Go 编排引擎，提供原生工作流视图、命令与模型工具入口，以及所选后端需要的 worker。
任务状态、验收、预算、进程生命周期和 Git 交付由引擎管理。

日常使用应通过完整 Flywheel 安装器安装。单独安装本插件只能得到插件和 worker，
不包含 Go 引擎和 skills。

## 安装

需要 **Node.js 22.18.0 或更高版本**、npm，以及兼容的 DSH 桌面和桌面提供的
`dsh` 命令。此包声明支持 **DSH 0.2.1-rc.5、0.2.2-rc.1 或 0.2.2-rc.2** 及相应 Cordis
4.0/4.1 接口。宿主可能有更高的 Node 要求，请使用其支持的运行时；其他宿主版本
需要单独验证兼容性。

```sh
npx -y @ckanner/flywheel@latest install
```

安装器安装同一版本的 CLI、平台二进制、worker、内嵌 skills 和桌面插件，通过
桌面插件管理接口完成注册并创建受管启动器。根据宿主提示重新加载或重启 DSH。
Go 包提供 macOS／Linux 的 arm64 与 x64 版本；Linux 包可用不代表已经验证
Mac 桌面或 Seatbelt 行为。

| 选项或命令 | 用途 |
|---|---|
| `--dsh-bin /absolute/path/to/dsh` | 桌面 CLI 不在 PATH 时明确指定 |
| `--prefix "$HOME/.local"` | 选择可写 npm 前缀，并将其 `bin` 加入 PATH |
| `--no-dsh` | 明确跳过桌面插件注册 |
| `flywheel setup --no-global` | 为已有持久安装补全或刷新配置 |
| `--json` | 返回可供自动化处理的安装报告 |

独立 npm DSH CLI 不能管理保留的 `desktop` profile。普通
`npm install --global @ckanner/flywheel` 只放置包文件，没有修改 profile、skills
或凭证的 postinstall。安装报告中 `ok: true` 只表示安装成功，`ready: false`
仍可能报告缺少原生 CLI 等本地前置条件。完整安装与卸载说明见[快速开始](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/getting-started.md)。

## 准备所选后端

DSH worker 默认使用独立 `sdk` profile。标准桌面与 SDK 在同一 `DSH_HOME` 下
可以复用 DeepSeek 账号授权存储；自定义 profile 需要加载所选账号服务、provider
和工具。保留的 `desktop` profile 不能作为 worker profile。

| 后端 | 准备与执行方式 |
|---|---|
| DeepSeek Flash | DSH `deepseek-account`，明确指定 `deepseek-flash`；标准共享账号存储不需要额外 API key |
| DSH ChatGPT | 在 SDK profile 安装并授权所选 ChatGPT 适配器；桌面对话模型不决定 worker 配置 |
| Codex Pro | worker 导入固定 0.9.0 包的 `dsh-subagent-codex-pro/machine`，使用已安装 Codex CLI 和登录 |
| Kimi | Go 调用原生 Kimi CLI 及其登录；不使用 ACP 或父模型二次委派 |
| Claude Code Pro | 可选 0.4.0 peer 和已安装 Claude CLI／登录；默认无阶段选它，详见[接入指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/claude-code-pro.md) |

一个已经记录的 ChatGPT 适配器兼容示例：

```sh
dsh plugin --profile sdk add dsh-plugin-chatgpt@0.1.17
```

此固定版本示例不代表当前账号拥有某模型权限。认证和真实模型可用性需要通过
各后端支持的流程检查。Flywheel 不安装原生模型 CLI、不复制凭证，也不提供订阅额度。

默认 Flash 路由不会在账号失败后回退到 `deepseek-official`；后者是需要明确选择
的 API-key provider。默认 hard 使用 `codex-pro`，不是名为 `subagent_codex_pro`
的 DSH provider，也不静默回退到 `codex exec`。详细说明见
[配置指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/configuration.md)和[模型与推理等级](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/model-policy.md)。

### 可执行文件与环境变量

| 环境变量 | 含义 |
|---|---|
| `FLYWHEEL_DSH_BIN` | worker 使用的 DSH 可执行文件 |
| `FLYWHEEL_DSH_PROFILE` | 独立 SDK profile，默认 `sdk` |
| `DSH_HOME` | DSH 配置、账号与会话根目录 |
| `FLYWHEEL_DSH_WORKER` | 显式覆盖 DSH worker |
| `FLYWHEEL_CODEX_PRO_WORKER` | 显式覆盖 Codex Pro worker |
| `FLYWHEEL_CLAUDE_CODE_PRO_WORKER` | 显式覆盖可选 Claude worker |
| `FLYWHEEL_CODEX_BIN` / `CODEX_HOME` | 原生 Codex 可执行文件及已有配置／登录根目录 |
| `FLYWHEEL_CLAUDE_BIN` / `CLAUDE_CONFIG_DIR` | 原生 Claude 可执行文件及已有配置／登录根目录 |
| `FLYWHEEL_KIMI_BIN` | 原生 Kimi 可执行文件，默认 `kimi` |

npm 启动器通常会从同版本依赖中解析 worker。启动引擎的进程必须能看到相关
可执行文件与环境设置；终端 export 不会更新已经启动的桌面进程。

## 启动任务与使用工作台

在 DSH 输入框输入 `/flywheel-task <目标>`。宿主助手先准备并检查任务契约，
再调用 `flywheel_launch`。准备阶段使用当前对话模型，执行阶段使用 Flywheel
配置中的绑定。`/flywheel-new` 只准备方案，`/flywheel-run` 继续已有任务。
用户明确要求先审阅方案时，这仍是任务授权的一部分。

在内置会话视图旁或其溢出菜单中打开 **Flywheel**。工作台分为三个页面：

| 页面 | 内容 |
|---|---|
| 流程画布 | 任务选择、当前阶段、证据、阶段执行／重试、角色历史与受管人工介入 |
| 运行记录 | 已登记运行、已知用量／费用、缺失指标及带条件的运行比较 |
| 配置 | 自动读取全局／项目配置，分组编辑、显示来源并保护未保存草稿 |

界面没有可编辑的任务／项目路径框。当前会话仅有一个关联任务时可以自动打开；
多个或历史任务需要明确选择。视图读取已登记运行与会话关联，不扫描仓库。
已知位置只作为只读信息展示；单阶段 Review 成功不代表整个任务完成。

只有准备记录的任务显示有界调查面板，包含观察、预算、原回执核对、停止与证据报告。
调查报告不代表代码交付。详见[调查指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/investigation.md)。

可见画布会刷新任务发现，运行中的任务会刷新状态。打开视图不启动任务或诊断。
读取失败时保留最后确认的快照及时间。launch/start/stop 回复丢失或超时表示
**结果未确认**；同一任务状态完成核对前，后续修改操作保持阻断，界面不会自动重试。
隐藏视图、关闭 DSH 或移除插件都不会停止独立 Go 任务；应明确停止并等待引擎确认。

### 命令与模型工具

| 入口 | 范围 |
|---|---|
| `/flywheel` | 接收 JSON 控制请求的人工命令 |
| `flywheel_launch` | 启动已准备任务的模型工具，默认 Auto |
| `flywheel_control` | launch/start/status/stop/investigate/config/doctor 模型工具 |
| 工作台控件 | 另外提供绑定 revision 的单阶段操作、配置编辑、受管接管及显式交付操作 |

人工启动示例：

```text
/flywheel {"action":"launch","taskDir":"/repo/docs/my-task","cwd":"/repo","mode":"auto"}
```

对应 CLI 操作：

```sh
flywheel control launch --cwd /repo --task-dir /repo/docs/my-task --mode auto
flywheel control status --cwd /repo --task-dir /repo/docs/my-task
flywheel control stop --cwd /repo --task-dir /repo/docs/my-task
flywheel control start --cwd /repo --task-dir /repo/docs/my-task
```

上述路径是明确的命令参数，不是界面输入框。省略 `cwd` 时使用会话公开的当前目录
服务；若会话没有记录项目目录，launch/start/stop 必须明确提供绝对 `cwd`。
输入经过校验后作为 argv 传递，不拼接成 shell 字符串。`ok: true` 表示控制操作
成功，任务是否完成以状态为准。

宿主准备外层任务 worktree，引擎独立管理 packet 调度和执行目录。新模板开启
基于声明的并行调度及自动 worktree 管理；已有完整配置省略 scheduler 时仍串行。
普通 run 负责本地 Git 交付；远端创建和合并 PR 需要显式交付操作，以及候选和授权
检查。完整说明见[工作流指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/workflow.md)和
[控制台指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/flow-console.md)。

## 无模型推理的诊断

```sh
flywheel config inspect --cwd /repo --json
flywheel doctor --cwd /repo --json
flywheel doctor --cwd /repo --probe --json
flywheel-codex-pro-worker --doctor
```

加上 `--task-dir /repo/docs/my-task` 可检查具体任务的配置。普通 doctor 检查本地
配置和前置条件；`--probe` 初始化临时 DSH SDK 会话，核对目录与权限后关闭。
探测使用临时持久化目录，不发送推理 prompt，也不运行任务验收命令；provider
初始化仍可能查询模型目录。

诊断成功不证明真实账号授权、额度或模型响应。`warn`、`unknown`、`skipped`
应分别保留，不能视为 pass。诊断和安装不写任务状态，也不创建任务 worktree。

## 运行边界

### 项目归属与历史

稳定的项目工作区用于标识仓库；任务集成目录与 packet worktree 用于实际执行。
DSH 在项目下建立 Session，再通过公开工作目录服务切换执行目录。只有桌面 Host
登记工作区归属；独立 SDK worker 不并发改写桌面存储。已有 Session header 与
未解决回执保留原身份。项目、执行目录和受限沙箱的组合不受支持时，派发前明确
失败，不静默扩大权限。

scoped 历史可以在同任务、同角色、同范围内续用。Executor、Reviewer、Judge
与第二意见的历史保持隔离；Reviewer 即使续用自己历史，也必须检查完整当前候选。
新模板明确选择 scoped，旧配置省略时保留 fresh；Plan 显式
`resident-session:false` 继续禁止该角色续用。

两种模式都保存调用安全记录。派发、结果或清理未知时阻断新执行。
Codex／Claude 的公开接口另外提供 Go 持久化 ACK、supervisor journal 和精确
只读回执恢复；DSH／Kimi 保留自身支持的路径。质量重建阈值默认关闭。
详见[恢复指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/session-recovery.md)及
[配对评估](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/session-evaluation.md)。

### 权限与 worker 生命周期

DSH worker 支持 read-only、workspace-write 和显式完全权限，审批设为 never。
新订阅预设请求完全权限；已有文件缺少权限设置时保留按角色权限。实际策略在
prompt 前检查。完全权限不代表 OS 沙箱，也不取消宿主静态拒绝、账号限制或
操作系统权限；Git worktree 同样不是安全沙箱。

临时 SDK overlay 只作用于独立 worker，关闭递归委派、workflow 驱动、自动标题／
压缩、定时输入及整个 web-tool 包（搜索和抓取）。完全权限不会恢复这些额外模型／
工具能力；恢复它们需要另外支持计量、取消和目录归属。overlay 不移除桌面工具，
也不改写提醒存储。

主任务 Codex 调用在 `thread-ready` 时核对完整实际沙箱策略，Go 确认后才继续
派发；终态回执再次检查。旧版／建议调用保留终态检查。Claude 的原生 init 在
prompt 之后，不能提供相同的预派发观察；Kimi print 模式也没有本适配器可承诺
的 OS 沙箱。

成功结果需要可信终态回执和清理确认。外层 worker 退出不能证明 SDK 或 guardian
已清理。SDK 正常关闭保留两秒用于历史落盘和释放，再进入有界 TERM/KILL 收尾。
结果丢失时保留证据并阻断自动重放。缺失用量与订阅美元成本保持 unknown，不填零。
完整 worker 历史不会自动并入父会话轨迹；这些 worker 输入不支持图片和附件。

## 维护与开发

插件显式 `flywheelBinary` 优先于受管安装记录。未设置时，使用
`~/.config/flywheel/` 中经过验证的启动器；只有记录不存在时才允许旧 PATH 查找。
记录损坏或启动器改变会明确失败。`controlTimeoutMs` 默认 60,000，限制的是桌面
控制请求，不是整个任务预算。

升级、移动 npm 前缀、更换 Node 或补充原生 CLI 后，携带所需前缀和桌面 CLI
选项运行 `flywheel setup --no-global`，再按需重载 DSH。已有配置与修改过的
skills 会保留。采用新默认值应走显式配置迁移，不通过删除归属记录强行覆盖。

在源码 checkout 中：

```sh
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check
```

跨进程与发布检查见[贡献指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/contributing.md)。离线 renderer／SDK
测试证明被测协议，不证明真实 Mac 窗口几何、账号推理或模型质量收益。

| 参考资料 | 用途 |
|---|---|
| [Worker 协议](PROTOCOL.md) | 详细通信、权限、生命周期与错误契约 |
| [依赖来源](vendor/README.md) | Codex 源码身份、tarball 摘要及可选 Claude 的源码依据 |
| [JEV 影子评估](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/jev-shadow.md) | 默认关闭的建议实验，不改变实际路由 |
| [发布指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/releasing.md) | 构建、安装和发布同一批已验证产物 |

## 许可证

[MIT](LICENSE)。借鉴的 DeepSeek UI 部分及打包依赖保留原声明，见
[第三方声明](THIRD_PARTY_NOTICES.md)。
