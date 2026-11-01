# Flywheel 的 DSH 桌面入口与原生 worker

目标是 **DSH Mac 桌面 0.2.1-rc.5** 的已存在接口。Go 继续管理状态机、任务、worktree、验收和成本上限；本包只提供桌面操作入口和原生 SDK 执行通道。无需 alpha、OAR、Forge 或一直活着的父智能体。

完整的从源码安装、SDK 配置到首个任务流程见 [Mac 入门教程](../../docs/getting-started.md)。

## 提供什么

| 部分 | 行为 |
|---|---|
| 桌面面板 | 会话输入框底部的 **Flywheel** 按钮；配置查看、环境检查、SDK 无推理探测、启动／刷新／停止；显示全部工作单元、逐项诊断与未知计量 |
| `/flywheel` | 人工 slash command，通过已有 DSH commands Remote 调用 Go CLI；不会将控制指令交给模型执行 |
| `flywheel_control` | 模型工具，复用同一个校验与 CLI 控制路径；后台 start 很快返回，stop 等待引擎确认 |
| `flywheel-dsh-worker` | 独立 Node 进程，通过官方 `dsh --profile sdk` stdio JSON-RPC 执行一个原生模型会话 |
| worker policy | 发请求前验证 cwd、实际权限和完整沙箱，自动拒绝需要批准的扩权 |

插件没有自己的任务数据库，不直接读写 `state.json`。桌面退出不会让后台 Go 任务失去父智能体；Go 拥有 worker 生命周期。原生 SDK 入口只接收已注册的 LLM adapter provider；`subagent_codex_pro`、Kimi ACP、Teams 都不是此接口的 provider。独立 Codex / Kimi CLI 使用 Go 的对应 driver。

## 构建与测试

需要 Node.js 22 或更高。生产构建不打包 DSH、React 或第三方运行时；可选 peer 元数据约束桌面接口版本。React 在浏览器端从 DSH 的 ModuleLoader 获取。`npm ci` 会安装锁定的 React 18、ReactDOM 与 jsdom **开发依赖**，用于实际 DOM 交互测试，它们不进入发布包。

```sh
cd plugins/dsh
npm ci --ignore-scripts
npm run check
npm pack
```

产物为 `dist/` 和 `ckanner-dsh-plugin-flywheel-0.1.0.tgz`。包名为 `@ckanner/dsh-plugin-flywheel`，许可沿用仓库的 `UNLICENSED`，本轮没有发布到 npm。`npm run check` 运行真实 Node 子进程协议桩和 React DOM 行为测试，无模型和账号调用。仓库的 Go 闸门仍需单独运行 `make quality-gate`。

## 安装到自己的 Mac

下列步骤需要在实际 Mac 上执行；本实现没有替用户安装或更新桌面程序。

先确认桌面 About、桌面安装的 `dsh --version` 与实际运行入口。桌面附带的 `dsh` 命令能管理 `desktop` profile；单独从 npm 安装的 CLI 不能管理这个保留 profile。不要用猜测的 `.app/Contents/...` 内部路径。

安装 worker bin，并把同一个构建包交给桌面 profile：

```sh
npm install --global /absolute/path/ckanner-dsh-plugin-flywheel-0.1.0.tgz
dsh plugin --profile desktop add /absolute/path/ckanner-dsh-plugin-flywheel-0.1.0.tgz
```

后一条使用**桌面安装的 dsh 命令**。也可以通过桌面的 Plugins 页面选择本地包。包内 `cordis.patch.yml` 会启用 host 插件，`dsh.client` 声明 browser 模块。启用后根据宿主提示重启或重新载入；不声称所有 profile 都支持热更新。

Flywheel 可执行文件必须是本仓库构建、具有 `control` 命令的新版本。GUI 环境找不到 PATH 里的 `flywheel` 时，在桌面 profile 的 `cordis.patch.yml` 追加覆盖项，指定**绝对路径**：

```yaml
- id: flywheel
  config:
    flywheelBinary: /absolute/path/to/flywheel
    controlTimeoutMs: 60000
```

这是 DSH 的 profile 配置，不是 Flywheel 的任务状态。`controlTimeoutMs` 保持 60 秒可覆盖引擎 stop 的清理与恢复窗口。

## 准备原生 SDK profile

引擎默认调用 PATH 中的 `flywheel-dsh-worker`。也可显式配置 `FLYWHEEL_DSH_WORKER=/absolute/path/to/dist/worker.js`。worker 默认调用 PATH 中的 `dsh`，建议通过 `FLYWHEEL_DSH_BIN` 固定已经核验的官方入口。`FLYWHEEL_DSH_PROFILE` 默认 `sdk`；可用另一个明确基于 SDK 的独立 profile，但不能是 `desktop`。

桌面有 adapter 不表示 SDK profile 也加载了它。SDK 的 DeepSeek 官方路由可自动挂载，其余 provider 必须在 **SDK profile** 中预先安装并完成该 provider 的授权配置。例如需原生 ChatGPT 路由时，先按该插件的文档检查并安装到对应 profile：

```sh
dsh plugin --profile sdk add dsh-plugin-chatgpt@0.1.17
```

这条命令是配置示例，不会迁移桌面凭证，也不保证一个第三方插件版本自动适配所有宿主。保持用户选择的 `DSH_HOME` 与授权来源一致，按 provider 的正常登录流程配置；不要复制或打印 token。`initialize` 将严格检查请求的 provider、model、effort，未注册的 provider 会失败。

实际 Mac rc.5 的 DeepSeek 原生路由是：

```text
provider = deepseek-official
model = deepseek-flash
reasoningEffort = off | low | high | max
```

`high` 是默认平衡档，**没有 `medium`**。其他 provider 使用自身的合法 effort。SDK README 的历史示例可能仍写 `deepseek-v4-flash`；以所用发行包与 adapter 的实际 catalog 为准。本桥接不替换模型 slug、不跨 provider 降级。

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
node --input-type=module -e 'console.log(JSON.stringify({type:"start",protocol:1,requestId:"local-doctor",cwd:process.cwd(),provider:"deepseek-official",model:"deepseek-flash",reasoningEffort:"high",permissionMode:"read-only",prompt:"This text is not sent in doctor mode",timeoutMs:20000}))' \
  | flywheel-dsh-worker --doctor
```

预期先看到 ready，再看到 `status:ok` 的 result；doctor 的 `usage` 和 `costUsd` 都是 `null`。该操作会启动本地 SDK、读取 provider 配置、创建本地会话，并运行沙箱中的 `:` 探针；**不会提交推理 prompt**。provider 的模型目录初始化可能联网，不等于发起推理。

如返回 `INVALID_REQUEST` 且没有权限确认，先查看 stderr 的真实 SDK 启动诊断。缺少沙箱、部分权限实施、错误 cwd 或插件加载失败都会被拒绝。不要通过 full access 绕过检查。doctor 通过后，再按用户自己的调用授权做一个小型真实任务，验证凭证和远端响应；真实付费模型检查不属于离线默认测试。

## 桌面使用

打开一个 DSH 会话，点击输入区底部的 **Flywheel**，填写已准备并审阅契约的任务目录，例如 `/repo/docs/my-task`。仓库目录默认取当前 DSH 会话的起始目录，也可填明确的绝对路径。按钮只执行以下 Go JSON 控制接口：

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

模型工具 `flywheel_control` 接受 `action`、`taskDir`、`cwd`，以及仅用于 doctor 的可选 `probe` 布尔值。config/doctor 可省 taskDir，cwd 可沿用会话目录；start/status/stop 必须指定任务。输入统一校验，使用 argv 调用，不拼接 shell 命令。面板只在点击时读取状态，不自动轮询，也不重新决定模型路由。

“查看模型配置”展示配置来源、展开后的阶段绑定及 provider／channel；“检查环境”不启动 SDK，“SDK 无推理检查”显式启用 probe。doctor 的合法退出码 1 会显示逐项诊断，超时、坏协议或不匹配的任务身份仍视为控制错误。`ok:true` 不代表账号、额度和真实推理通过。

状态区区分未初始化、运行、已完成、暂停、停止、升级和预算停止；全部 packet 可见，不会只展示前几个。未知 usage／cost 明确保留，原始 JSON 默认折叠。改任务／仓库路径会清空旧视图并忽略旧请求的迟到结果。关闭面板、卸载插件或退出桌面，不等于停止任务；重新打开后按“刷新状态”恢复观察，需停止时点“停止”。

## 权限、上下文与报告归档

每个调用使用新的 SDK Session；provider/model/effort 在该进程中固定。v1 不支持 resume、图像/附件输入或 SDK 原生 output schema。文本与报告文件由 Go 的冻结合同和 JSON Schema 验收，不能把 SDK idle 当成验收完成。

worker 只支持 `read-only` 与 `workspace-write`。Mac 上由 DSH 的 Seatbelt bash runner 和 native fs fence 执行权限。workspace-write 的写入范围是**实际工作目录加 DSH 规定的系统临时区**，并非额外目录列表，也不是网络隔离。策略插件将 approval 固定为 `never`，即拒绝提权。

执行报告先写到 worktree 内的任务报告路径，由 Go 校验后归档到主任务目录。不得为报告写入而把整个任务目录或用户目录设成 full access。

默认覆盖会关闭 DSH 自主委派、workflow、goal 驱动、模型生成标题、web search 工具及自动模型压缩，避免在一个 Go 工作单元之外再启动辅助模型调用。需要压缩/继续执行时由 Go 显式决定下一次任务；超出上下文或输出 token 限制不会伪装成成功。SDK profile 应只装所需的可信 provider /工具，第三方 Host 插件本身是进程内代码，不能由本包替它提供任意代码隔离。

## 生命周期与可验证范围

官方 rc.5 SDK 没有 per-turn cancel，所以使用每调用一个 SDK 进程。worker 请求 `shutdown` 并等待退出；SDK 自己排空其托管命令。Go 继续负责外层 deadline、取消及它拥有的进程组清扫。正常 SDK 关闭未被证明时结果失败，不能进入成功验收。Go 默认 5 秒 kill grace 应保留，以给 worker 清理窗口；不要配置短于桥接清理预算的值。

当前已验证：离线 worker 全生命周期与事件折叠；桌面 command/tool 入口一致；构建后的浏览器模块经 ModuleLoader／宿主 React／真实 DOM／Remote 的交互；Go 与 Node 的配置、诊断和失败契约。前轮还核对官方 npm rc.5 的真实 Host commands/tools 注册、SDK 初始化/关闭，以及当前 Linux 无可用沙箱时在 prompt 前 fail closed。

尚未在用户 Mac 实测：桌面安装和 UI 渲染、Seatbelt 探针、实际 Flash/GPT 推理、授权可见性及断电恢复。Linux 上的 CLI 加载成功不代替这些验证。不会将协议桩写成付费模型成功的证据。

完整输入、事件、usage 与关闭语义见 [PROTOCOL.md](PROTOCOL.md)。

## 实现依据

宿主代码按公开 tag `dsh-v0.2.1-rc.5` 固定；另对实际 Mac rc.5 的编译模块核对了同名 API。关键一手来源：

- [SDK server 方法、生命周期与限制](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sdk/server/README.md)
- [SDK TypeScript 客户端、usage 事件和关闭语义](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sdk/client/README.md)
- [SDK bundle 与共享 base 配置](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/bundle/sdk-app/cordis.patch.yml)
- [官方 CLI profile 与桌面命令边界](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/apps/cli/README.md)
- [commands 服务](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/interaction/commands/README.md) 与 [tools 服务](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/core/tools/README.md)
- [沙箱的单 workspaceRoot、临时区与完整性语义](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sandbox/sandbox/README.md)
- [ChatGPT adapter 本轮参考版本](https://github.com/ckanner/dsh-plugin-chatgpt/tree/388868f91bcdc05795fb94d21b868ded6d7219c9)
