# Flywheel 的 DSH 桌面入口与原生 worker

[English](README.en.md) · 简体中文

目标是 **DSH Mac 桌面 0.2.1-rc.5** 的已存在接口。Go 继续管理状态机、任务、worktree、验收和成本上限；本包提供桌面操作入口、原生 SDK 执行通道和 Codex Pro machine API worker。无需 alpha、OAR、Forge 或一直活着的父智能体。

完整的 npm 安装、SDK 配置到首个任务流程见 [Mac 入门教程](../../docs/getting-started.md)，适用于 Flywheel **0.28.0 及以后的完整 npm 发行版**。安装前用 `npm view @ckanner/flywheel version` 确认 registry 的 latest 已更新到所需版本；源码合并或 GitHub Release 不代表 npm 已发行。旧 0.22.1 没有本教程使用的安装与 DSH 接口，当前发行状态见[发行记录](../../docs/releasing.md#3-失败恢复)。

**Flywheel 0.28.1 引入原生会话视图**；0.28.0 仍使用输入区底部弹层。需要升级 CLI 与桌面插件到同一新版本，源码合并不会自动更新已安装的插件。

订阅预设迁移命令和 `permissions.mode` 从 **0.28.2** 开始提供。升级保留已有配置；只有用户明确应用迁移时才备份并更新模型与权限。

**0.28.3 的变化：**新增 `deepseek-account` 默认、账号错误处理与会话当前工作目录修复。0.28.2 仍使用 API 默认和会话最初目录，重装同版不会获得修复；升级保留已有配置，采用账号预设需要显式迁移。

## 提供什么

| 部分 | 行为 |
|---|---|
| 桌面会话视图 | 与“对话／轨迹”并列的 **Flywheel** 视图；查看任务进度和待处理事项、检查模型配置与环境、启动／刷新／停止已有任务 |
| `/flywheel` | 人工 slash command，通过已有 DSH commands Remote 调用 Go CLI；不会将控制指令交给模型执行 |
| `flywheel_control` | 模型工具，复用同一个校验与 CLI 控制路径；后台 start 很快返回，stop 等待引擎确认 |
| `flywheel-dsh-worker` | 独立 Node 进程，通过官方 `dsh --profile sdk` stdio JSON-RPC 执行一个原生模型会话 |
| DSH worker policy | 发请求前验证 cwd、实际权限和完整沙箱，自动拒绝需要批准的扩权 |
| `flywheel-codex-pro-worker` | 直接导入 `dsh-subagent-codex-pro@0.8.0` 的公开 `./machine` 入口，传递任务与取消并返回完整插件回执 |

插件没有自己的任务数据库，不直接读写 `state.json`。桌面退出不会让后台 Go 任务失去父智能体；Go 拥有 worker 生命周期。原生 SDK 入口只接收已注册的 LLM adapter provider；`subagent_codex_pro`、Kimi ACP、Teams 都不是此接口的 provider。Kimi CLI、Codex Pro machine API 和旧显式 Codex CLI 使用各自 Go driver；Codex Pro 复用同一插件 runner，不经过 DSH 父模型或工具名转发。

这个视图用于回答“任务运行到了哪一步、哪里需要处理、使用哪些模型，以及如何停止或恢复”。需求讨论、编写契约和一般对话继续使用 DSH 的对话与 skills；Flywheel 不提供另一套聊天界面，也不接管 DSH 的模型选择器、输入框、轨迹或右侧栏。

## 一条命令安装到自己的 Mac

已有 Node.js 22.18.0 或更高、npm 和 DSH Mac 桌面后，先确认 About、桌面安装的 `dsh --version` 与实际运行入口，再运行：

```sh
npx -y @ckanner/flywheel@latest install
```

Agent 可以加 `--json` 获取单个结构化结果。该命令全局安装本次精确版本的 CLI、平台 Go 二进制与 worker，安装 10 个内嵌 skills，生成 GUI 启动器，并用现有桌面命令执行 `dsh plugin --profile desktop add --save-exact --ignore-scripts @ckanner/dsh-plugin-flywheel@<本次版本>`，再通过 `plugin list` 验证版本。无需另行安装 worker 或下载压缩包。安装器不替用户安装／更新 DSH，不代登录模型渠道。

桌面附带的 `dsh` 命令能管理 `desktop` profile；单独从 npm 安装的 DSH CLI 不能管理这个保留 profile。命令不在 PATH 时加 `--dsh-bin /absolute/path/to/dsh`；不要猜测 `.app/Contents/...` 内部路径。包内 `cordis.patch.yml` 启用 host 插件，`dsh.client` 声明 browser 模块。完成后根据宿主提示重启或重新载入，不假定所有 profile 都支持热更新。

`ok:true` 表示软件安装成功。安装后的本地 doctor 仍有阻断项时返回 `ready:false` 和具体检查项，例如 Kimi／Codex 未安装；这不会将已经完成的软件安装伪报为失败，也不证明账号已就绪。安装过程不发送模型 prompt。参数、部分失败恢复与 JSON 语义见[入门教程](../../docs/getting-started.md#2-一条命令安装)和[协议说明](PROTOCOL.md#npm-安装报告与桌面入口)。

普通 `npm install --global @ckanner/flywheel` 只安装包文件；随后可显式执行 `flywheel setup --no-global` 配置用户目录和 DSH。没有隐式修改用户配置的 postinstall。单独安装 `@ckanner/dsh-plugin-flywheel` 只得到插件／worker，不是整个 Flywheel 的完整安装。

### 自动解析 GUI 入口

安装器生成 `~/.config/flywheel/npm-launcher` 及 `installation.json`，记录绝对 Node／CLI 路径、同版 worker、prefix 和 launcher 的 SHA-256。launcher 使用已核对的 DSH 入口与安装时的工具 PATH。默认配置无需手写脚本或修改 profile；桌面插件验证安装记录、普通可执行 launcher 与 hash 后使用。

解析顺序是：用户显式 `flywheelBinary` → 有效安装记录 → 仅当记录不存在时从 PATH 查找 `flywheel`。已有记录损坏、launcher 缺失或内容变更时明确失败，不会自动退回另一份引擎。用户自定义配置优先，安装器不会覆盖它。升级 Node、变更安装位置或补齐 CLI 后，可在工具均可见的终端运行 `flywheel setup --no-global`，保留自定义 prefix／DSH 入口参数，再在桌面面板复验。

源码开发者或有意使用自定义入口的用户，才需要在桌面 profile 的 `cordis.patch.yml` 显式覆盖，例如：

```yaml
- id: flywheel
  config:
    flywheelBinary: /absolute/path/to/flywheel
    controlTimeoutMs: 60000
```

这是 DSH 的 profile 配置，不是 Flywheel 的任务状态。使用独立 Go 二进制时应自行保证 worker 与模型 CLI 可见；该高级配置会优先于自动安装记录。`controlTimeoutMs` 保持 60 秒可覆盖引擎 stop 的清理与恢复窗口。

## 准备原生 SDK profile

npm CLI 启动器自动解析同版本依赖中的 worker 绝对路径，并在用户未设置时注入 `FLYWHEEL_DSH_WORKER`。直接运行 Go 二进制时默认从 PATH 查找 `flywheel-dsh-worker`；也可显式设置 `FLYWHEEL_DSH_WORKER=/absolute/path/to/dist/worker.js`。自动 launcher 已记录安装时的桌面 `dsh` 入口，用户显式 `FLYWHEEL_DSH_BIN` 优先。`FLYWHEEL_DSH_PROFILE` 默认 `sdk`；可用另一个明确基于 SDK 的独立 profile，但不能是 `desktop`。

标准 SDK profile 叠加 base，已经包含 DeepSeek 账号服务、账号 adapter 和 API-key adapter。标准桌面与 SDK 在同一 `DSH_HOME` 下默认共用账号授权存储，账号路径不要求复制 token 或额外 API key。自定义 home、凭证存储和 SDK 插件集需核对配置。第三方 provider 则要在 **SDK profile** 中加载并按各自流程授权，例如原生 ChatGPT 路由：

```sh
dsh plugin --profile sdk add dsh-plugin-chatgpt@0.1.17
```

这条命令是配置示例，不会复制凭证，也不保证一个第三方插件版本自动适配所有宿主。保持用户选择的 `DSH_HOME` 与授权来源一致，按 provider 的正常登录流程配置；不要复制或打印 token。`initialize` 检查请求的 provider、model、effort，未注册的 provider 会失败。DSH 桌面的账号默认模型选择不自动传给 SDK；Flywheel 必须明确请求所选 provider 与模型。

当前源码默认选择的 Mac rc.5 DeepSeek 账号路由是：

```text
provider = deepseek-account
model = deepseek-flash
reasoningEffort = off | low | high | max
```

本项目核对的 DSH rc.5 adapter 只接受 `off/low/high/max`，默认 high，**不接受 `medium` 或 `xhigh`**；这不同于 DeepSeek 通用 API 的兼容别名。`deepseek-official` 是另一个 API-key provider，只在用户明确选择 API 接入时配置，不复用账号 token。默认预设没有 API 后备，因此未登录、登录失效或额度不足不会自动转用 API；用户显式配置的后备链按原规则执行。其他 provider 使用各自合法 effort，GPT 支持 xhigh/max，Kimi K3.1 Pro 原生支持 low/high/max；理由与评估边界见[推理等级说明](https://github.com/ckanner/flywheel/blob/main/docs/model-policy.md)。SDK README 的历史示例可能仍写 `deepseek-v4-flash`；以所用发行包与 adapter 的实际 catalog 为准。本桥接不替换模型 slug、不跨 provider 降级。

## Codex Pro 插件执行通道

默认 `hard` 路由使用 `harness: "codex-pro"`。本包精确依赖 `dsh-subagent-codex-pro@0.8.0`，独立 worker 直接导入公开的 `dsh-subagent-codex-pro/machine`，将模型、effort、工作目录和权限传给插件。DSH 控制面、Go 状态机、插件 runner 各自保持原职责；不需要桌面父模型二次委派，也不会自动回退旧 `codex exec`。

npm 启动器自动解析并注入 `FLYWHEEL_CODEX_PRO_WORKER`。独立 Go 二进制默认从 PATH 查找 `flywheel-codex-pro-worker`，源码开发可显式设为构建后的 `dist/codex-pro-worker.js`。`FLYWHEEL_CODEX_BIN` 可指定底层 Codex 可执行程序，默认 `codex`；`CODEX_HOME` 沿用用户选择的配置／登录目录。Go 会将这些运行必需项交给插件，不把凭证复制到配置、契约或 Git。

新调用使用 ephemeral 会话，权限由 `permissions.mode` 决定，approval 为 never。role-based 选择 `read-only`／`workspace-write`；full-access 请求 `danger-full-access`。插件运行前核对模型、cwd 和权限模式，完整 sandboxPolicy 由 Flywheel 在返回后审查。受限模式允许 Codex 标准临时目录、拒绝工作区外额外 roots 与网络启用；完全权限则核对 `dangerFullAccess` 策略，不声称 OS 隔离。0.8.0 尚无公开的运行前完整策略 guard，事后拒绝交付不能阻止先前的访问；可信 Codex 配置仍是本机运行前提。

完整 machine receipt 写入指标，保留 accepted effort、未知 effectiveEffort、usage 完整性与进程终态。插件正常收尾可能主动 TERM/KILL app-server，其物理 exit code 可以为空；Flywheel 只有在完整终态证明任务完成、清理已确认且外层 worker 干净退出时才能继续。取消、超时、截断、启动或清理失败均阻止成功。Go 为此 worker 预留至少 10 秒终止宽限，避免截断插件内部清理。

仅检查包入口、不调用模型可运行：

```sh
flywheel-codex-pro-worker --doctor
```

成功输出单行协议 1 JSON，精确版本 0.8.0，`available:true`、`inferenceTested:false`。它不启动 Codex app-server，不读取登录凭证，不证明账号或实际沙箱可用。日常仍应使用下面的 Go doctor，以核对完整配置来源与所需阶段。

## 无模型请求的 doctor

日常使用 Go 命令或面板；Go 会解析实际配置，避免手工输入与任务绑定不一致：

```sh
flywheel config inspect --cwd /repo --task-dir /repo/docs/my-task --json
flywheel doctor --cwd /repo --task-dir /repo/docs/my-task --json
flywheel doctor --cwd /repo --task-dir /repo/docs/my-task --probe --json
```

`--task-dir` 可省略：无任务时检查全部绑定，有任务时只检查必要阶段与实际 packet 路由。无 packet 为规划前状态，使用 default 路由并提示规划后复查。`--probe` 才初始化 DSH SDK；普通检查只读取配置、版本和所声明环境变量是否存在。它们不写 Flywheel 状态、全局配置或 worktree，不运行验收，不调用模型。阻断项返回退出码 1 并保留报告；认证／额度／真实模型能力始终未验证。

只调试单个 worker 时，仍可在已有且可访问的目标目录生成一条 probe 请求：

```sh
node --input-type=module -e 'console.log(JSON.stringify({type:"start",protocol:1,requestId:"local-doctor",cwd:process.cwd(),provider:"deepseek-account",model:"deepseek-flash",reasoningEffort:"high",permissionMode:"read-only",prompt:"This text is not sent in doctor mode",timeoutMs:20000}))' \
  | flywheel-dsh-worker --doctor
```

预期先看到 ready，再看到 `status:ok` 的 result；doctor 的 `usage` 和 `costUsd` 都是 `null`。该操作会启动本地 SDK、读取 provider 配置、创建本地会话，并运行沙箱中的 `:` 探针；**不会提交推理 prompt**。provider 的模型目录初始化可能联网，不等于发起推理。

如返回 `INVALID_REQUEST` 且没有权限确认，先查看 stderr 的真实 SDK 启动诊断。缺少沙箱、部分权限实施、错误 cwd 或插件加载失败都会被拒绝。不要通过 full access 绕过检查。doctor 通过后，再按用户自己的调用授权做一个小型真实任务，验证凭证和远端响应；真实付费模型检查不属于离线默认测试。

## 桌面使用

正常入口是在 DSH 对话输入框调用 `/flywheel-new`，让助手准备任务说明与验收清单并给出实际路径；审阅后通过 `/flywheel-run` 开始工作。无需先填写面板。初始准备使用当前对话选择的模型，引擎启动后才使用 Flywheel 配置中的各阶段模型；两者不自动同步。

要查看或直接控制已有任务，再在顶部“对话／轨迹”所在的视图栏选择 **Flywheel**；未直接显示时检查该栏的更多视图入口。

| 字段 | 用途 | 是否需要填写 |
|---|---|---|
| 任务资料目录 | Spec、packet、验收说明和任务记录所在的 `docs/<slug>/`，例如 `/Users/me/project/docs/fix-login` | 面板启动／刷新／停止需指定；配置与诊断可以留空。由准备 Skill 给出，无需自行编写这些文件才能使用入口 |
| 项目目录（可选） | 对应源码项目／任务集成 worktree，例如 `/Users/me/project` | 当前源码默认读取会话实际工作目录；只在选择另一项目或 worktree 时覆盖 |

从 0.28.3 开始调用宿主 `workingDirectory.get(session)`，跟随会话中的目录切换；显式项目目录优先，getter 失败在派发前报错，不回退到会话最初的 `header.cwd`。0.28.2 仍取最初目录，切换 worktree 后请显式填写项目路径。面板不自动绑定 Skill 创建的任务；查看它时粘贴 Skill 返回的任务目录，再刷新。两个字段都是项目路径，不是插件安装目录。按钮只执行以下 Go JSON 控制接口：

```sh
flywheel control status --task-dir /repo/docs/my-task --cwd /repo
flywheel control start --task-dir /repo/docs/my-task --cwd /repo
flywheel control stop --task-dir /repo/docs/my-task --cwd /repo
flywheel control config --cwd /repo
flywheel control doctor --task-dir /repo/docs/my-task --cwd /repo --probe
```

等价的人工命令：

```text
/flywheel {"action":"status","taskDir":"/repo/docs/my-task","cwd":"/repo"}
```

模型工具 `flywheel_control` 接受 `action`、`taskDir`、`cwd`，以及仅用于 doctor 的可选 `probe` 布尔值。config/doctor 可省 taskDir，cwd 可沿用会话当前工作目录；start/status/stop 必须指定任务。`/flywheel {…}` 是直接人工控制命令，与 `/flywheel-new` 等 Skill 入口不同。输入统一校验，使用 argv 调用，不拼接 shell 命令。打开视图不会启动任务或探测；视图只在点击时读取状态，不自动轮询，也不自动发现其他入口启动的任务。

“查看模型配置”展示实际配置来源、展开后的阶段绑定及 provider／channel／权限。“检查安装与配置”仅做本地预检，不启动 SDK；“检查 DSH 接入”额外临时启动 SDK、核对配置／目录／权限后退出。首次安装、升级或排障时按需使用，平时不必每次点击；两项都不向模型发送消息，也不验证账号、额度和实际回答能力。doctor 的合法退出码 1 会显示逐项诊断，超时、坏协议或不匹配的任务身份仍视为控制错误。`ok:true` 不代表账号、额度和真实推理通过。

状态区区分未初始化、运行、已完成、暂停、停止、升级和预算停止；全部 packet 可见，不会只展示前几个。未知 usage／cost 明确保留，原始 JSON 默认折叠。配置显示引擎实际选择的文件及阶段绑定；已有用户配置不会因安装或切换 DSH 对话模型而自动迁移。

### 视图、请求与任务的生命周期

任务／仓库路径草稿和已取得的快照按 DSH 会话分别保留，在宿主仍保留该会话绑定时，切换“对话／轨迹／Flywheel”后可以继续使用；这不是任务数据库，也不保证在会话绑定释放、插件重载或桌面重启后继续保留。每个会话同一时刻只等待一个控制请求。启动／停止等待期间锁定目录输入；只读请求等待期间可以编辑路径，但仍等待已有请求返回，并忽略旧选择的迟到响应。切换视图不会把等待中的请求当成已经完成。

任务状态是最近一次成功读取的快照，带有刷新时间，不能当作实时监控。读取失败时保留已有有效内容并显示失败；启动或停止的响应丢失、超时或断连表示**结果未确认**，不能推断任务没有启动或已经停止。结果未确认时，界面保留提示并禁止继续启动／停止，直到“刷新状态”成功核对该任务；必要时查看任务日志，再决定恢复。界面不会自动重试启动或停止。

目录不存在或控制进程无法创建等情况，只有宿主结构化确认请求尚未执行，界面才允许修正输入或环境后再次提交。普通错误、断连或错误文字本身不构成这项确认，不能据此解除结果未确认状态。`/flywheel` 的失败 `text` 使用 `type:"flywheel-control-error"` 的 JSON 包装，字段和兼容行为见[命令失败协议](PROTOCOL.md#dsh-命令失败与是否派发)。

切换视图、关闭会话页面、卸载插件或退出桌面，都不等于向后台 Go 任务发出停止请求。需要停止时显式点击“停止”并等待引擎确认；重新打开后刷新同一任务。DSH 的原生轨迹继续展示父会话已有的命令与工具记录；独立 SDK、Kimi、Codex Pro worker 的完整执行历史不会自动合并到父会话轨迹，也不会被伪装成 DSH 原生后台任务。

### 宿主 UI 接入

客户端通过 `ctx.slots.inject()` 和独立 `id: flywheel` 注册到 `conversation.view` 列表，使用宿主的会话作用域和视图切换。保留“对话／轨迹”原有条目、输入区和右侧栏；不注册覆盖它们的同名条目，不另外绘制悬浮侧栏或底部大弹层。长路径、模型信息和错误在视图内容区换行或局部滚动，不通过遮挡宿主控件来增加空间。

作为外部 plain-JS 插件，浏览器模块只运行时导入共享 React，不导入 `ui-primitives`、`client-store` 或其他 Harness Client 包。插件自有按钮、输入框、查看状态 store handle 和 observable；宿主根据注册契约管理会话实例并提供读取 hooks，locale 通过注入服务取得。控件参照宿主结构和交互，颜色使用语义别名，字体继承宿主，编译后的 CSS Modules 样式随 React 视图挂载和卸载。这样匹配宿主界面，也避免绑定不稳定的 Client 模块值接口。

依据固定为声明版本 `0.2.1-rc.5` 的源码快照 `b830ec950faba0a46dda564be3e2d854afed94f1`：[外部插件实践](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/preset/agent-preset/skills/cordis-plugin-development/references/practices.md)、[UI 规范](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/.agents/skills/dsh-client-ui-ux/SKILL.md)、[布局说明](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/client/ui-layout/README.md)，以及[会话插槽契约](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/client/ui-conversation/src/client/contract/slots.ts)和[插槽生命周期](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/docs/subsystems/slots.md)。视图使用 Conversation 已有标题栏和内容区，遵循其窗口边距，不修改全局窗口留白。源码契约核对不代替目标 Mac 桌面的实机验证。

## 权限、上下文与报告归档

每个调用使用新的 SDK Session；provider/model/effort 在该进程中固定。v1 不支持 resume、图像/附件输入或 SDK 原生 output schema。文本与报告文件由 Go 的冻结合同和 JSON Schema 验收，不能把 SDK idle 当成验收完成。

SDK 接受模型与 effort 配置不代表远端服务证明了实际推理强度。`ready.reasoningEffort` 是初始化成功后的请求回显；只有实际任务评估才能判断更高 effort 是否改善交付。账号未登录和 token 失效精确归 AUTH，退出账号导致的已知取消事件同样归 AUTH；账号额度不足归 QUOTA。保留原始原因并提示恢复同一账号；默认预设不提供 API Key 后备。

worker 支持 `read-only`、`workspace-write` 和显式 `danger-full-access`。新订阅预设通过 `permissions.mode:full-access` 为全部阶段选择完全权限，approval 固定为 `never`；Kimi print 使用原生 auto，不逐步请求用户点击允许。已有文件缺省仍保留 role-based，需通过[配置迁移](../../docs/configuration.md#将旧配置切换到已有的三种资源)显式采用新方案。受限模式由 Mac 上 DSH 的 Seatbelt bash runner 和 native fs fence 执行，workspace-write 范围是**实际工作目录加 DSH 规定的系统临时区**。full-access 如实记录无 OS 沙箱限制；宿主静态拒绝、账号与操作系统权限不被绕过。

执行报告仍先写到 worktree 内的任务报告路径，由 Go 校验后归档到主任务目录。权限模式来自明确配置，不能因为报告写入失败而暗中扩大它；任务边界、冻结验收和候选身份检查保持有效。

默认覆盖会关闭 DSH 自主委派、workflow、goal 驱动、模型生成标题及自动模型压缩，避免在一个 Go 工作单元之外再启动辅助模型调用。需要压缩/继续执行时由 Go 显式决定下一次任务；超出上下文或输出 token 限制不会伪装成成功。SDK profile 应只装所需的可信 provider /工具，第三方 Host 插件本身是进程内代码，不能由本包替它提供任意代码隔离。

这份临时覆盖只用于每次独立启动的 **SDK worker 进程**，不修改桌面 profile，不卸载或隐藏桌面已有的插件、skills、对话工具及 UI。反过来，桌面已安装插件或已登录账号，也不证明 SDK profile 已加载并授权相同能力；应按对应 harness 的配置和正式授权流程核对。

当前覆盖禁用整个 `tool-web`，因此 SDK worker 的 `web_search` 和 `web_fetch` 都不可用。这是一项保守的运行策略，不是 DSH 所有网页操作必然调用模型：官方 [web 工具](https://github.com/deepseek-ai/deepseek-harness/blob/fcc0ad462172522aaf5c93a7b2e01f6b9f0b0159/packages/web/tool-web/README.md)支持分别配置搜索与抓取，[DeepSeek 搜索后端](https://github.com/deepseek-ai/deepseek-harness/blob/fcc0ad462172522aaf5c93a7b2e01f6b9f0b0159/packages/web/web-search-deepseek/README.md)才明确包含辅助模型调用。此次 UI 修复不变更 worker 策略；恢复检索或[自动压缩](https://github.com/deepseek-ai/deepseek-harness/blob/fcc0ad462172522aaf5c93a7b2e01f6b9f0b0159/packages/compaction/compaction-basic/README.md)等能力，需要另行验证调用计量、预算、取消和工作目录边界。

## 生命周期与可验证范围

官方 rc.5 SDK 没有 per-turn cancel，所以使用每调用一个 SDK 进程。worker 请求 `shutdown` 并等待退出；SDK 自己排空其托管命令。Go 继续负责外层 deadline、取消及它拥有的进程组清扫。正常 SDK 关闭未被证明时结果失败，不能进入成功验收。worker 已启动但终态回执缺失或协议损坏时会记录清理未确认并停止本次运行，不自动重试或降级；恢复前先确认该次调用的 SDK 托管进程已停止。Go 默认 5 秒 kill grace 应保留，以给 worker 清理窗口；不要配置短于桥接清理预算的值。

测试覆盖离线 worker 全生命周期与事件折叠；桌面 command/tool 入口一致；构建后的浏览器模块在 React 与 jsdom 中运行插件自有控件、store handle 和 observable。组件夹具适配槽位桥接和 Remote，另用已发布的 rc.5 renderer、slots 与 Cordis 检验真实注册、会话绑定、声明收起／恢复及卸载；这两类测试都不验证真实宿主几何布局。还覆盖 Go 与 Node 的配置、诊断和失败契约，以及安装记录校验、用户配置优先和损坏入口拒绝。根目录的 npm 安装测试使用实际六包 tarball、隔离目录和 DSH 协议桩，验证完整安装流程。前轮还核对官方 npm rc.5 的真实 Host commands/tools 注册、SDK 初始化／关闭，以及当前 Linux 无可用沙箱时在 prompt 前 fail closed。前轮运行结果见[验证记录](../../docs/validation/2026-11-01-results.md)。

尚未在用户 Mac 实测：桌面安装和 UI 渲染、Seatbelt 探针、实际 Flash/GPT 推理、授权可见性及断电恢复。Linux 上的 CLI 加载成功不代替这些验证。不会将协议桩写成付费模型成功的证据。

完整输入、事件、usage 与关闭语义见 [PROTOCOL.md](PROTOCOL.md)。

## 升级、卸载与开发构建

升级前停止活动任务、保存配置与自定义内容，再运行同一个 `npx -y @ckanner/flywheel@latest install`，保留原来的 prefix／profile 等参数。skills 使用内容 hash 验证所有权，用户修改会报告冲突并保留；launcher 也不能被静默覆盖。按[冲突恢复步骤](../../docs/getting-started.md#8-升级排错与卸载)处理后重跑，不删除任务账本或修改 hash 绕过校验。

卸载先停止活动任务，在 DSH 的所用 profile 移除该插件并重新载入，再执行 `npm uninstall --global @ckanner/flywheel`，自定义 prefix 时加同一参数。npm 卸载不自动删除用户配置、skills、任务文件或安装记录，也不删除模型工具及凭证。

开发构建需要 Node.js 22.18.0 或更高；Go 引擎还需要 Go 1.27 或更高。生产插件精确依赖 `dsh-subagent-codex-pro@0.8.0`；不打包 DSH、React 或 Codex 可执行程序；可选 peer 元数据约束桌面接口版本。React 在浏览器端从 DSH 的 ModuleLoader 获取。`npm ci` 安装锁定的 React 18、ReactDOM 与 jsdom **开发依赖**，用于真实 DOM 测试，它们不进入发布包。在仓库根运行：

```sh
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check
make quality-gate
npm test
npm run build:npm
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

`build:npm` 在隔离源码副本中构建 worker 和四个平台 Go 二进制，输出六个同版本 npm 包与候选校验记录，包含 `ckanner-dsh-plugin-flywheel-0.28.0.tgz`。所有包的许可沿用仓库 `UNLICENSED`。tarball／Actions artifact 用于开发或备用验证；正式发布必须使用已经过门禁与真实 npm 安装测试的同一候选，不能在测试后重新编译再发布。上述测试不调用真实模型，也不使用用户账号或桌面 profile。

## 实现依据

宿主代码按公开 tag `dsh-v0.2.1-rc.5` 固定；另对实际 Mac rc.5 的编译模块核对了同名 API。关键一手来源：

- [SDK server 方法、生命周期与限制](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sdk/server/README.md)
- [SDK TypeScript 客户端、usage 事件和关闭语义](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sdk/client/README.md)
- [SDK bundle 与共享 base 配置](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/bundle/sdk-app/cordis.patch.yml)
- [官方 CLI profile 与桌面命令边界](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/apps/cli/README.md)
- [commands 服务](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/interaction/commands/README.md) 与 [tools 服务](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/core/tools/README.md)
- [沙箱的单 workspaceRoot、临时区与完整性语义](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sandbox/sandbox/README.md)
- [ChatGPT adapter 本轮参考版本](https://github.com/ckanner/dsh-plugin-chatgpt/tree/388868f91bcdc05795fb94d21b868ded6d7219c9)
