# Flywheel DSH 插件

[English](README.md) · 简体中文 · [项目介绍](https://github.com/ckanner/flywheel/blob/main/README.zh-CN.md) · [文档目录](https://github.com/ckanner/flywheel/blob/main/docs/README.zh-CN.md)

`@ckanner/flywheel` 将 DeepSeek Harness（DSH）连接到 Flywheel 的
Go 编排引擎，提供原生工作流视图、命令与模型工具入口，以及所选后端需要的 worker。
任务状态、验收、预算、进程生命周期和 Git 交付由引擎管理。

日常使用应通过完整 Flywheel 安装器安装。正式发布的 `@ckanner/flywheel` 包含 CLI、
引擎、worker、skills 和桌面插件。普通 npm 安装放置这些文件；`install`／`setup`
负责登记桌面插件并配置启动器。

## 安装

需要 **Node.js 22.18.0 或更高版本**、npm，以及兼容的 DSH 桌面和桌面提供的
`dsh` 命令。此包声明支持 **`>=0.2.1-rc.5 <0.4.0-0 || ^0.2.2-0 || ^0.2.3-0 || ^0.3.0-0`** 及相应 Cordis 4.0/4.1 接口。
DSH 的范围判断包含预发布版本：0.2 系列内后续 rc／补丁升级无需精确版本豁免或
重新发布 Flywheel；0.3 系列及其 alpha／beta／rc 仍在边界之外。宿主可能有更高的
Node 要求，请使用其支持的运行时。

范围是公开 API 兼容策略，不代表所有未来宿主均已实测。锁定的集成夹具覆盖旧版
0.2.1-rc.5 Host 和当前 0.2.2-rc.3 Host／SDK；运行时继续检查必需服务、会话身份、
权限回执与进程清理，服务缺失或回执无效仍明确失败。DSH peer 依赖由宿主提供且
保持 optional，npm 范围覆盖两个既有预发布系列，不再逐个锁定 rc 版本。

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

默认通过原生订阅客户端执行，DSH 视图是控制面。安装桌面插件不会改变父对话 provider。

| 后端 | 安装与执行路径 |
| --- | --- |
| Antigravity | 内置 `@ckanner/dsh-subagent-antigravity@0.3.2` machine API → 官方本机 `agy` CLI（默认 `transport: "acp"`、单轮用量计量、原生图片输入、独立 `evidence-only` 隔离） |
| Claude Code Pro | worker 依赖树中的精确 optional peer `0.7.1` → 官方 Agent SDK → 本地 Claude Code 订阅 |
| Codex Pro | 内置 `0.10.2` machine API → 官方 app-server 与 ChatGPT 登录 |
| Kimi | 最小 owned native agent profile，managed Kimi Code OAuth（默认放开 `WebSearch`/`FetchURL`/`Agent`/`AgentSwarm`）；严格订阅模式拒绝 print |

严格预设在准入时核对原生路径与受支持订阅证据，额度未知仍保留未知。认证/额度失败不
开启 API Key、credits 或 extra usage 后备。见[模型策略](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/model-policy.md)、
[Antigravity](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/antigravity.md)和
[Claude](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/claude-code-pro.md)。

旧的非严格配置仍可显式使用 DSH sdk profile、DeepSeek 账号/API 或 DSH ChatGPT；
它们不能作为 subscription-only 的可达替代。SDK 和 Desktop profile 分开，登录仍由
原生客户端支持的流程完成；Flywheel 不复制凭据或安装模型客户端程序。

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
| `FLYWHEEL_AGY_BIN` / `FLYWHEEL_ANTIGRAVITY_WORKER` | 官方 agy 程序／显式 Antigravity worker 覆盖 |
| `FLYWHEEL_KIMI_BIN` | 原生 Kimi 可执行文件，默认 `kimi` |

npm 启动器直接解析当前完整包内的 worker。启动引擎的进程必须能看到相关
可执行文件与环境设置；终端 export 不会更新已经启动的桌面进程。

## 启动任务与使用工作台

在 DSH 输入框输入 `/flywheel-task <目标>`。宿主登记事实和薄 brief，经
`flywheel_control planning` 交给 Go，实际设计和独立审查采用 compact 或 system 准备。
review-first 停在精确版本人工批准；delegated 保留原实施授权继续。`/flywheel-new`
默认先审方案，`/flywheel-run` 继续原任务和原账本。

在内置会话视图旁或其溢出菜单中打开 **Flywheel**。工作台分为三个页面：

| 页面 | 内容 |
|---|---|
| 流程画布 | 当前会话任务、当前阶段、证据、阶段执行／重试、角色历史与受管人工介入 |
| 运行记录 | 已登记运行、已知用量／费用、缺失指标及带条件的运行比较 |
| 配置 | 自动读取全局／项目配置，分组编辑、显示来源并保护未保存草稿 |

视图自动跟随当前会话的显式任务关联；没有投影时读取精确匹配会话 ID 的最新运行。
多个关联任务自动显示“本会话任务”导航，单个任务直接展示画布；没有跨会话选择器或路径输入框，其他会话与无归属记录不可见。准备任务使用当前会话的
`flywheel_control status` 显式关联。位置和操作分组直接显示；单阶段 Review 成功不代表整个任务完成。

自动刷新和其他读取使用会话作用域的只读 Remote，不向对话追加 JSON 命令记录。
显式修改与未知结果核对仍保留持久命令回执。任务标题与任务 ID 分开展示，项目／任务路径位于顶部。设置栏目连续排列，阶段模型直接编辑同一份草稿，统一点击“保存修改”；直接展示角色会话覆盖和可选用量预算；环境检查归入诊断与配置文件，原始诊断回执
不占用日常流程画布。

只有准备记录的任务显示有界调查面板，包含观察、预算、原回执核对、停止与证据报告。
批准的诊断命令和 HTTP 查询显示实际退出码／状态码、取消与副本收尾。命令需要明确
host-process 授权，在批准输入的副本运行；副本不是操作系统沙箱。调查报告不代表
代码交付。详见[调查指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/investigation.md)。

在**设置 → JEV**中，**启用 JEV** 是唯一总开关，默认关闭。选择全局或项目范围，
打开开关以显示连接字段，填写或确认 System One 地址、固定模型版本与密钥环境
变量名，再点击**保存修改**。开关先修改草稿，保存后才应用；编辑和保存都不测试
服务连通性。关闭时沿用原路径，不请求 JEV，也不采集影子快照。
Task、New 创建的新调查自动继承策略，Run 继续该冻结策略。开启后，JEV 可以选择
下一项已批准的取证动作、依据已引用的证据跳过
冗余动作，或提前结束取证进入报告。实施阶段在确认失败后，可以选择合法的局部
修复、反馈检查、受限重规划或批准的执行绑定。调查与实施共享任务调用额度；反馈
检查不能代替完整最终验收。

关闭也会禁止已有任务的后续 JEV 调用与采用，同时保留回执和预算；重新开启不会
改写此前关闭时创建的准备记录。面板分别展示配置状态、本次调查记录的状态、请求、
已采用动作与有证据依据的效果。任务与节点详情区分已应用的控制决策、已收尾反馈、
未确认动作和原有基线，保留实际反馈结果、模型绑定前后值和切换原因；配置应用不等于
新会话已经运行或最终验收通过。高级调用上限、概率阈值与数据类别共用这个总开关，
没有独立的影子或采用开关。旧影子配置不会自动开启在线决策。
详见[JEV 决策](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/jev.md)。


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

可选 `scheduler.prewarm:true` 使用 DSH worker 的两阶段 `--prewarm` 协议，在验证期间
初始化一个 SDK；正式调用获准前不创建 Session、不发送 prompt。默认关闭，其他后端
沿用原启动路径。未使用的 worker 同样必须有界关闭并证明进程组已清理；过期、取消和
恢复规则见[worker 协议](PROTOCOL.md#dsh-sdk-一次性预热)。

DSH worker 支持 read-only、workspace-write 和显式完全权限，审批设为 never。
新订阅预设请求完全权限；已有文件缺少权限设置时保留按角色权限。实际策略在
prompt 前检查。完全权限不代表 OS 沙箱，也不取消宿主静态拒绝、账号限制或
操作系统权限；Git worktree 同样不是安全沙箱。

临时 SDK overlay 只作用于独立 worker。显式 native-profile 可启用受管 PTC 子 Agent、
Team、Workflow、Web 与可选 LCU；未选择时保留原工具策略。自动标题、压缩、goal与
定时输入仍关闭，完全权限本身不会启用原生能力。overlay 不修改桌面 preset 和提醒。
配置、回执、Tower 与独立 Review/Team 入口见[原生能力指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/native-capabilities.md)。

主任务 Codex 调用在 `thread-ready` 时核对完整实际沙箱策略，Go 确认后才继续
派发；终态回执再次检查。旧版／建议调用保留终态检查。Claude 的原生 init 在
prompt 之后，不能提供相同的预派发观察；Kimi print 模式也没有本适配器可承诺
的 OS 沙箱。

成功结果需要可信终态回执和清理确认。外层 worker 退出不能证明 SDK 或 guardian
已清理。普通SDK关闭保留两秒；native为15秒，LCU为135秒，Go外层相应不少于20／145秒，再进入有界TERM/KILL收尾。
结果丢失时保留证据并阻断自动重放。缺失用量与订阅美元成本保持 unknown，不填零。
完整worker历史不会自动并入父会话轨迹；公开Pro machine支持显式图片／结构化输出，正式Go loop不会自动注入所有归档截图。

## 维护与开发

插件显式 `flywheelBinary` 优先于受管安装记录。未设置时，使用
`~/.config/flywheel/` 中经过验证的启动器；记录不存在时使用本包 `bin/flywheel.js`，
仅 private 源码测试区保留旧 PATH 后备。
记录损坏或启动器改变会明确失败。`controlTimeoutMs` 默认 60,000，限制的是桌面
控制请求，不是整个任务预算。

已有受管启动记录必须与当前包同版。单独更新 DSH 副本后，应运行该版本的 setup
刷新受管安装；旧记录会明确报错，不把新版界面与旧引擎混用。

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
| [使用手册](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/usage-handbook.md) | 故障调查、Issue 修复和复杂项目的操作场景 |
| [Worker 协议](PROTOCOL.md) | 详细通信、权限、生命周期与错误契约 |
| [依赖来源](vendor/README.md) | Codex 源码身份、tarball 摘要及可选 Claude 的源码依据 |
| [JEV 决策](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/jev.md) | 一个总开关控制实际有界决策，保留离线回放与显式评估 |
| [发布指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/releasing.md) | 构建、安装和发布同一批已验证产物 |

## 许可证

[MIT](LICENSE)。借鉴的 DeepSeek UI 部分及打包依赖保留原声明，见
[第三方声明](THIRD_PARTY_NOTICES.md)。
