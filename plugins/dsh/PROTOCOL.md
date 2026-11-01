# Go ↔ DSH worker 协议 v1

此文件是 `internal/drivers/dsh.go` 与本目录 worker 的互操作契约。Go 负责状态机、验收、费用上限及最终进程组清扫；worker 负责一个独立的官方 DSH SDK 进程。两者之间只有本地 stdio，不开放 HTTP 端口。

## 启动与输入

Go 在工作目录启动 `flywheel-dsh-worker`，写入**一条** UTF-8 JSON 行并关闭 stdin。关闭 stdin 表示请求已完整发送，不表示取消任务。单条输入及 SDK 帧上限均为 8 MiB。stdout 只输出 JSONL，诊断走 stderr。

```json
{"type":"start","protocol":1,"requestId":"call-123","cwd":"/absolute/worktree","provider":"deepseek-official","model":"deepseek-flash","reasoningEffort":"high","permissionMode":"workspace-write","prompt":"执行冻结工作单元","timeoutMs":1800000,"maxTokens":32768}
```

必填字段：`type`、`protocol`、`requestId`、`cwd`、`provider`、`model`、`permissionMode`、`prompt`。`cwd` 必须是存在的绝对目录，worker 规范化为真实路径；Go 在比较前同样规范化。可选字段：

| 字段 | 语义 |
|---|---|
| `reasoningEffort` | 原样传给 DSH adapter，必须是该模型支持的非空值；省略使用 adapter 默认值 |
| `maxTokens` | 正整数，每次原生模型请求的输出上限，传入 SDK initialize |
| `timeoutMs` | 1–86,400,000，默认 1,800,000；涵盖权限准备、初始化、运行和关闭 |

`permissionMode` 仅允许 `read-only`、`workspace-write`。未知字段一律拒绝。v1 不接受 `sessionId` / resume、附件、任意额外 CLI 参数和额外写目录；不进行隐式冷启动替代恢复。

## 运行参数

| 环境变量 | 默认值 | 用途 |
|---|---|---|
| `FLYWHEEL_DSH_WORKER` | `flywheel-dsh-worker` | Go driver 使用的 worker 可执行文件路径 |
| `FLYWHEEL_DSH_BIN` | `dsh` | worker 使用的官方 CLI；支持绝对路径，也可明确依赖 PATH |
| `FLYWHEEL_DSH_PROFILE` | `sdk` | 已配置的 SDK profile；禁止 `desktop` |
| `DSH_HOME` | 官方 CLI 默认值 | DSH 自己的配置、授权引用及会话持久化根 |

worker 不搜索 `.app` 内部目录、不下载或更新 CLI、不复制桌面授权文件，也不读取或修改 Flywheel 的 `state.json`。Go 的凭证环境白名单仍是外层边界。

worker 用 `dsh --profile <profile> --patch <临时覆盖文件>` 启动 DSH。覆盖文件在临时目录内、权限为 `0600`，结束后移除。DSH 子进程继承 Go 建立的 worker 进程组，禁止再为 SDK 创建 detached 组。

## 输出帧

每帧均含 `protocol:1`、原 `requestId`。顺序是 `ready` → 零或多个 `event` → 恰好一个 `result`；启动失败允许直接 `result`。创建 Session 时的权限事件会缓冲至 ready 之后。

### ready：真实能力握手已通过

```json
{"protocol":1,"requestId":"call-123","type":"ready","sessionId":"flywheel-uuid","cwd":"/absolute/worktree","provider":"deepseek-official","model":"deepseek-flash","reasoningEffort":"high","permissionMode":"workspace-write","approvalPolicy":"never"}
```

发出 ready 前必须依次满足：

1. 官方 SDK `initialize` 返回 `deepseek-harness-sdk-runtime`，adapter 接受指定 provider/model/effort。
2. worker 策略插件经真实 `ctx.shell` 执行本地无副作用的 `:` 探针；退出码为 0 且报告 `sandbox.enforcement:full`。不可用或部分实施的沙箱拒绝通过。
3. `session/working-directory/get` 创建全新的 Session，返回实际 cwd。
4. Session 的 `sandbox/mode`、`approval/policy` 被固定，`ctx.sandboxPolicy.resolve()` 与 `ctx.approval.effectivePolicy()` 的实际读值形成 `flywheel/policy` 事件。
5. worker 比对 policy 事件的 cwd、mode、`never` 和 full confinement。

任一项缺失，不发送 `session/prompt`。`never` 在 rc.5 的含义是自动拒绝需要批准的提权操作；它不会授予无限权限。

### event：可审计的原始持久事件

```json
{"protocol":1,"requestId":"call-123","type":"event","sessionId":"flywheel-uuid","event":{"seq":12,"type":"turn/end","data":{"turn":0,"reason":{"kind":"completed"}}}}
```

只转发该根 Session 的持久事件，按 `seq` 去重。SDK streaming usage 不作重复累加。意外出现根 Session 的子智能体启动时，立即失败并关闭 SDK；编排与委派由 Go 管理。

### result：关闭后的唯一结果

```json
{"protocol":1,"requestId":"call-123","type":"result","status":"ok","settled":true,"sessionId":"flywheel-uuid","text":"完成说明","usage":{"input_tokens":120,"output_tokens":8,"reasoning_tokens":3,"cache_read_tokens":20,"cache_write_tokens":0,"total_tokens":128},"costUsd":null}
```

成功必须同时具有：对应 messageId 的持久 inbox receipt、`session/wait` 返回、根 `turn/end.reason.kind:completed`、非空已提交 assistant text、SDK shutdown 确认及退出码 0。idle、超出输出 token 上限、半截文本或仅成功退出都不足以证明成功。

`status` 是 `ok`、`error` 或 `cancelled`。失败提供 `error:{code,message}`；code 仅使用 Go 已有分类。`settled` 只描述 SDK 关闭证据；Go watchdog 对整个拥有的进程组做最终判定。强制终止 SDK 或未获得正常 shutdown 确认时，不能返回成功，也不能声称其原生托管进程已全部清理。

使用量来自**每条唯一 `assistant/message` 的已提交 per-call usage**，跨该次 worker 调用累加：

- DSH 的 `inputTokens` 是未命中缓存的输入；bridge 的 `input_tokens` 是它加已报告 cache read/write 的总输入。Go 不再重复加缓存。
- `reasoning_tokens` 属于输出的细分项，不再次计入 `total_tokens`。
- 官方有 `totalTokens` 时保留；没有时按总输入加输出求和。可选细分计数未报告时不额外计入，原始事件保留其缺省事实。
- 任一已提交 assistant message 缺少有效 usage，整个 `usage` 返回 `null`，不会把不完整的计数当成完整账单。
- `costUsd` 为 `null`。此桥接没有供应商结算账单，也不把订阅调用计作已知零成本。

## 取消、终止及恢复

SIGTERM/SIGINT 请求取消。worker 先发送 SDK `shutdown`，再按有界窗口升级至 SIGTERM/SIGKILL 并等待 SDK leader 退出。Go watchdog 在正常退出、取消和超时路径都清扫自己拥有的进程组。DSH 还有自己托管的命令生命周期，因此正常 SDK dispose/exit 是成功收据的一部分；强杀时不能据此宣称所有独立进程组已清理。主动 `setsid` 的进程不在普通进程组保证内。

v1 每次调用使用全新 session。Go 重试时重新提交冻结任务输入，已有 worktree 与外部验收证据由 Go 决定是否保留；不会恢复旧 Session 后悄悄更换模型或权限。桌面关闭、窗口隐藏、插件卸载均不会主动停止已经由引擎后台启动的任务；显式 `control stop` 才请求引擎停止并等待清理。

`flywheel-dsh-worker --doctor` 接收同一请求格式，只执行初始化与权限握手，然后关闭 SDK，绝不发送模型 prompt。它不能证明凭证、远端推理或终端 UI 已通过真实使用验证。
