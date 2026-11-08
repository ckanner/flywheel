# Go ↔ DSH / Codex Pro / Claude Code Pro worker 协议

此文件记录两种独立 worker 协议。前半部分对应 `internal/drivers/dsh.go` 与官方 DSH SDK worker；[Codex Pro 部分](#codex-pro-worker协议)对应 `internal/drivers/codex_pro.go` 和插件 machine API worker。Go 负责状态机、验收、费用上限及最终进程判断；worker 仅执行一次有边界的调用。两者之间只有本地 stdio，不开放 HTTP 端口。

## 启动与输入

Go 在工作目录启动 `flywheel-dsh-worker`，写入**一条** UTF-8 JSON 行并关闭 stdin。关闭 stdin 表示请求已完整发送，不表示取消任务。单条输入及 SDK 帧上限均为 8 MiB。stdout 只输出 JSONL，诊断走 stderr。

```json
{"type":"start","protocol":1,"requestId":"call-123","cwd":"/absolute/worktree","provider":"deepseek-account","model":"deepseek-flash","reasoningEffort":"high","permissionMode":"workspace-write","prompt":"执行冻结工作单元","timeoutMs":1800000,"maxTokens":32768}
```

必填字段：`type`、`protocol`、`requestId`、`cwd`、`provider`、`model`、`permissionMode`、`prompt`。`cwd` 必须是存在的绝对目录，worker 规范化为真实路径；Go 在比较前同样规范化。可选字段：

| 字段 | 语义 |
|---|---|
| `reasoningEffort` | 原样传给 DSH adapter，必须是该模型支持的非空值；省略使用 adapter 默认值 |
| `maxTokens` | 正整数，每次原生模型请求的输出上限，传入 SDK initialize |
| `timeoutMs` | 1–86,400,000，默认 1,800,000；涵盖权限准备、初始化、运行和关闭 |

`permissionMode` 允许 `read-only`、`workspace-write` 和显式 `danger-full-access`。未知字段一律拒绝。v1 不接受 `sessionId` / resume、附件、任意额外 CLI 参数和额外写目录；不进行隐式冷启动替代恢复。

scoped 调用显式使用 v2：原字段保持不变，增加必填 `persistent:true` 和可选的非空
`sessionId`。省略 sessionId 创建新的持久会话；提供时只能恢复该精确 ID。

```json
{"type":"start","protocol":2,"requestId":"call-124","cwd":"/absolute/worktree","provider":"deepseek-account","model":"deepseek-flash","reasoningEffort":"high","permissionMode":"workspace-write","prompt":"根据当前契约和失败证据返修","timeoutMs":1800000,"persistent":true,"sessionId":"flywheel-uuid"}
```

v2 拒绝缺失或 false 的 persistent；v1 不接受这些 v2 字段。版本选择由 Go 的调用策略
决定，不从用户提示词或后端错误正文猜测。SDK 初始化后、目录检查和 prompt 前通过
公开 `session/resume` 恢复；missing、busy 或通用恢复错误均不自动冷启动替代。

`provider` 明确选择认证路径：`deepseek-account` 使用 DSH 账号服务，`deepseek-official` 使用 API Key；不省略 provider 来猜桌面默认，worker 不在账号失败时自行换用 API。默认预设没有后备渠道，Go 仍可按用户显式配置的后备链执行原有策略。标准 SDK 与桌面在相同 DSH home 下默认共用账号授权存储，无需复制凭证。

## 运行参数

| 环境变量 | 默认值 | 用途 |
|---|---|---|
| `FLYWHEEL_DSH_WORKER` | npm 启动器解析同版本 worker 的绝对路径；独立 Go 二进制为 `flywheel-dsh-worker` | Go driver 使用的 worker 可执行文件路径；用户显式变量优先 |
| `FLYWHEEL_DSH_BIN` | `dsh` | worker 使用的官方 CLI；支持绝对路径，也可明确依赖 PATH |
| `FLYWHEEL_DSH_PROFILE` | `sdk` | 已配置的 SDK profile；禁止 `desktop` |
| `DSH_HOME` | 官方 CLI 默认值 | DSH 自己的配置、授权引用及会话持久化根 |

worker 不搜索 `.app` 内部目录、不下载或更新 CLI、不复制桌面授权文件，也不读取或修改 Flywheel 的 `state.json`。Go 的凭证环境白名单仍是外层边界。

worker 用 `dsh --profile <profile> --patch <临时覆盖文件>` 启动 DSH。覆盖文件在临时目录内、权限为 `0600`，结束后移除。DSH 子进程继承 Go 建立的 worker 进程组，禁止再为 SDK 创建 detached 组。覆盖禁用自主 goal／workflow／delegation、辅助模型调用及 `schedule`／`tool-schedule`；不修改用户 profile 或提醒存储。

## 输出帧

每帧均回显请求的 `protocol`（1 或 2）及原 `requestId`。顺序是 `ready` → 零或多个
`event` → 恰好一个 `result`；启动失败允许直接 `result`。v1 创建 Session 时的权限
事件缓冲至 ready 之后；v2 admission 前的恢复事件不转发为本轮活动。

### ready：SDK 配置与权限握手已通过

```json
{"protocol":1,"requestId":"call-123","type":"ready","sessionId":"flywheel-uuid","cwd":"/absolute/worktree","provider":"deepseek-account","model":"deepseek-flash","reasoningEffort":"high","permissionMode":"workspace-write","approvalPolicy":"never"}
```

发出 ready 前必须依次满足：

1. 官方 SDK `initialize` 返回 `deepseek-harness-sdk-runtime`，adapter 接受指定 provider/model/effort。
2. worker 策略插件经真实 `ctx.shell` 执行本地无副作用的 `:` 探针；退出码为 0、模式匹配且未被拒绝。受限模式还须报告 `sandbox.enforcement:full`，不可用或部分实施的沙箱拒绝通过；明确完全权限时记录 `confinedShell:false` 并省略 enforcement，不伪造 OS 强制。
3. v2 指定历史时先调用公开 `session/resume`；随后 `session/working-directory/get` 返回所选 Session 的实际 cwd。无历史请求则创建新 Session。
4. 在公开 `agent/created` 阶段固定原生 `sandbox/mode`、`approval/policy`；通过公开 `sandboxPolicy.resolve()` 与 `approval.overrideOf(session)` 读取实际策略，等待写入本次私有权限回执。不得调用私有 effectivePolicy，也不向持久历史追加自定义 `flywheel/policy` 事件。
5. worker 限定大小读取普通非链接的回执文件，核对 nonce、requestId、sessionId、完整 cwd／mode／never 和权限证据。目录／文件权限为 0700／0600；非阻塞读取拒绝 FIFO，回执在本次运行后移除。

任一项缺失，不发送 `session/prompt`。`never` 控制交互审批，不单独扩大访问范围；必须同时显式选择 `danger-full-access` 才请求完全权限。既有静态拒绝或外部权限不足仍按真实失败处理。

`initialize` 返回 serverInfo，并通过 SDK 配置校验。ready 中的 provider/model/reasoningEffort 是配置被接受后的 worker 请求回显；尤其 reasoningEffort 不是模型服务独立返回的实际强度证明，也不证明账号额度或回答质量。

v2 的 ready 另含 `persistent:true`、`resumed:boolean`；恢复时 sessionId 必须与请求
精确匹配，不能返回一个新 ID 冒充恢复成功。serverInfo 不足以确认 SDK npm 包版本。

### event：可审计的原始持久事件

```json
{"protocol":1,"requestId":"call-123","type":"event","sessionId":"flywheel-uuid","event":{"seq":12,"type":"turn/end","data":{"turn":0,"reason":{"kind":"completed"}}}}
```

只转发该根 Session 的持久事件，按 `seq` 去重。SDK streaming usage 不作重复累加。意外出现根 Session 的子智能体启动时，立即失败并关闭 SDK；编排与委派由 Go 管理。

恢复过程中重放的旧事件不进入当前 output／usage fold，也不触发 Go 的 OnEvent。
worker 在 admission 后、发送本次 prompt 前重置 fold，只计当前调用新增的输出与用量。

### result：关闭后的唯一结果

```json
{"protocol":1,"requestId":"call-123","type":"result","status":"ok","settled":true,"sessionId":"flywheel-uuid","text":"完成说明","usage":{"input_tokens":120,"output_tokens":8,"reasoning_tokens":3,"cache_read_tokens":20,"cache_write_tokens":0,"total_tokens":128},"costUsd":null}
```

成功必须同时具有：对应 messageId 的持久 inbox receipt、`session/wait` 返回、根 `turn/end.reason.kind:completed`、非空已提交 assistant text、SDK shutdown 确认及退出码 0。idle、超出输出 token 上限、半截文本或仅成功退出都不足以证明成功。

`status` 是 `ok`、`error` 或 `cancelled`。失败提供 `error:{code,message}`；code 仅使用 Go 已有分类。`settled` 只描述 SDK 关闭证据；Go watchdog 对整个拥有的进程组做最终判定。强制终止 SDK 或未获得正常 shutdown 确认时，不能返回成功，也不能声称其原生托管进程已全部清理。

v2 result 增加 `lifecycle`，不替代物理终态或原有字段。例如正常恢复后的结果包含：

```json
{"protocol":"dsh-sdk/session-resume/worker-v2","requestId":"call-124","sessionId":"flywheel-uuid","dispatch":"dispatched","outcome":"completed","cleanupConfirmed":true,"resumeCandidate":true,"resumeAccepted":true,"turnId":"1","usageStatus":"complete"}
```

dispatch 为 not-dispatched／dispatched／unknown，outcome 为
completed／failed／cancelled／unknown；cleanup 独立判断。DSH 不暴露独立 runId。
未知原生 turn kind、无效 resume/wait 响应和无法确认的失败保持 unknown，不能靠
settled 或错误文本补出可信 outcome。恢复失败即使确认本轮未派发，也不证明旧结果。
v1 wire 不增加 lifecycle；Go 在收到完整、身份匹配、物理成功的普通 v1 调用时生成
`dsh-sdk/worker-v1` 标准化事实并回显引擎 requestId，resumeCandidate 恒 false；
任意通用失败保持 unknown。

DeepSeek 账号的 `ACCOUNT_SIGN_IN_REQUIRED`／`ACCOUNT_TOKEN_INVALID` 精确映射到 AUTH，`ACCOUNT_QUOTA` 映射到 QUOTA。`turn/end` 的 aborted 只在其 reason 为 `{kind:"hook",reason:"deepseek-account/signed-out"}` 时同归 AUTH。保留原始诊断与恢复同一账号的提示；未知 ACCOUNT 前缀、其他 hook 和普通 JSON-RPC 传输失败沿用原分类。worker 自身取消继续优先输出 cancelled，不把取消冒充认证失败。

Go 在验证协议身份、终态、已知 code 及 error 对应物理退出码 1 后独立保留结构化错误码，编排与 doctor 不从原始消息重新猜测该码。矛盾的退出码、进程超时、spawn/signal、截断、取消和未确认清理优先处理；这些事实不因收到 AUTH/QUOTA 而消失。合法 error 帧的 settled:false 记录 SDK 清理未确认并阻止自动重试；已启动 worker 缺少可信终态回执（包括身份损坏、缺失终态或坏尾帧）同样记录 CleanupError 并停止本次运行，外层进程已退出不代替 SDK shutdown 证明；worker 启动前拒绝且 settled:true 不等于清理未知。未知码或坏协议仍属于传输／协议失败，不能借错误正文冒充可信 code。Node 也要把 SDK 关闭期间的坏帧及 EOF 未换行残帧记为 protocolFailure，并覆盖先前账户错误；外部取消继续保留 cancelled 状态。

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

Go 的 `DSHArgs.Doctor` 使用这个入口；为保持 v1 必填 `prompt` 的 admission schema，它会用固定的探针说明替代调用者文本，worker 不将其交给 `session/prompt`。doctor 仍必须具备匹配的 ready、可靠 result、正常 SDK shutdown 与物理退出，不能因为没有推理而放松清理或权限检查。

## Codex Pro worker协议

公开入口为 `@ckanner/dsh-plugin-flywheel/codex-pro-worker`，可执行入口为 `flywheel-codex-pro-worker`。`FLYWHEEL_CODEX_PRO_WORKER` 允许显式覆盖；npm 启动器解析同版本依赖，不依靠碰巧可用的全局旧 worker。worker 实际导入 bundle 的已审查源码包 `dsh-subagent-codex-pro@0.9.0` 的 `./machine` API，严格检查包名、版本、machine protocol 1 及 lifecycle/recovery capabilities；源码来源见 `vendor/README.md`，不宣称该版本已在 registry 发布。Node 最低 22.18.0，仅支持 macOS／Linux。

### 请求与父进程连接

```json
{"type":"start","protocol":1,"requestId":"call-123","cwd":"/absolute/worktree","model":"gpt-6.1-sol","reasoningEffort":"high","permissionMode":"workspace-write","prompt":"执行冻结工作单元","timeoutMs":1800000}
```

v1 只接受这些字段；不接受 provider、resume、threadMode、threadId、额外写目录、任意 CLI 参数或未知权限模式。requestId 非空且最多 256 字符；cwd 为绝对路径，Go 先规范化成物理目录。model／reasoningEffort／prompt 必须是非空文本，permissionMode 为 read-only／workspace-write／danger-full-access；最后一项在 Node adapter 边界映射为 machine API 的 `dangerously-bypass-approvals-and-sandbox`，Go 同时核对映射后的 requested 字段和 canonical 生效模式。timeoutMs 显式值必须为 1–2,147,483,647 的安全整数，省略使用上游 600,000 毫秒；worker 不接受通过 0 关闭期限。Go 传递有效 wall deadline 和 high 默认 thinking；worker 将 reasoningEffort 映射到 machine 的 effort。v1 固定 ephemeral，command 为 `FLYWHEEL_CODEX_BIN` 或 `codex`。

旧低层 scoped 入口使用外层 v2，增加必填 `threadMode:"persistent"` 和可选的精确 threadId：

```json
{"type":"start","protocol":2,"requestId":"call-125","cwd":"/absolute/worktree","model":"gpt-6.1-sol","reasoningEffort":"high","permissionMode":"workspace-write","prompt":"按当前契约返修","timeoutMs":1800000,"threadMode":"persistent","threadId":"existing-thread"}
```

v2 拒绝 ephemeral、缺失模式和空／非法句柄；所有 progress/result 回显外层版本。
嵌套 adapter、machine 请求与原始 receipt 仍为协议 1，精确包版本仍为 0.9.0。
v1/v2 不启用有 ACK 的生命周期；主任务使用下面的 v3 接通公开 lifecycle/recovery。

**这里与 DSH worker 的 EOF 语义不同：Go 写入一行后保持 stdin 开放。** stdin 是父进程的存活连接；未完成前 EOF、读取错误或非法后续输入触发 AbortSignal，并等待插件有界清理。不能用 `echo … | worker` 的关闭 stdin 行为冒充正常调用。运行请求首行等待上限 10 秒；请求最多 1 MiB，machine 原始输出上限 1 MiB，序列化 progress 累计最多 1 MiB，完整终态帧最多 8 MiB。超限明确失败，不能裁掉回执后判成功。

### 进度与完整结果

零或多个进度帧：

```json
{"protocol":1,"type":"progress","requestId":"call-123","text":"插件报告的活动"}
```

最后恰好一个 result，字段结构如下，`receipt` 为插件实际完整对象：

```json
{"protocol":1,"type":"result","requestId":"call-123","adapter":{"package":"dsh-subagent-codex-pro","version":"0.9.0","protocol":1},"receipt":null,"error":{"code":"INVALID_REQUEST","message":"启动请求未通过校验"}}
```

启动前不能获得 machine receipt 时为 null。运行已开始则即使失败也保留完整 receipt；`error` 非 null、重复／丢失 result、尾部垃圾、请求身份或 adapter 版本不符都不能成功。本通道没有 DSH `ready` 帧；v3 使用有类型的 `thread-ready` lifecycle 与 Go ACK。不能从普通 progress 文本虚构沙箱或实际 effort 证据。

Go 同时校验外层 worker 物理终态和插件 receipt：请求及 run/thread/turn 身份、模型、cwd、approval=never、实际沙箱模式、完整 sandboxPolicy、stopReason、cleanup 与执行事实。受限模式的 workspaceWrite 显式 writableRoots 必须处于工作区内，networkAccess 必须明确 false；显式完全权限要求 `sandbox:danger-full-access` 与精确的 `{type:"dangerFullAccess"}` policy，不要求不存在的受限模式字段；Codex 标准临时目录标记保留，不宣称只能写 cwd。主任务 v3 在 `thread-ready` ACK 前也核对这份完整策略，不符合时阻止 turn/start。旧协议或未请求 lifecycle 的辅助调用仍只在返回后完整检查；这些路径不能宣称预执行完整策略 guard。

effort 的证据只可为 turn/start 请求接受，保留 `effectiveEffort:null`。usage 使用 `run` 增量，不能把 threadTotal 当成本次消耗；只有 complete 才记入可知预算，partial/unknown 原样存入 receipt 并增加未知调用计数。cached input／reasoning output 是总 input／output 的子项，不重复累加。订阅 costUSD 保持 null、costStatus 保持 unknown。receipt 没有实际 provider 时保持未知，不根据 channel 发明 provider。

Go 由可信 receipt 生成 `codex-pro/0.9.0/worker-v1`、`worker-v2` 或 `worker-v3` 的 SessionOutcome。
只有正常 completed、cleanup 已确认且 persistent 的结果才具备 resumeCandidate；
这是适配器证据支持的尝试资格，不是 rollout 已持久化的证明。显式恢复必须接受同一
thread，requested／outcome／execution 的 thread 身份和 thread/start、thread/resume
来源均须一致。缺失／坏 receipt、流错误或 worker 异常保持 unknown。只有可信的
request／initialize／新 thread-start 等派发前阶段可证明 not-dispatched；thread-resume
拒绝仍未知，不从 busy／no-rollout 文本猜测安全冷启动。

### 取消与进程终态

worker 捕获 SIGINT／SIGTERM／SIGHUP，先取消并等待插件返回清理结果，再分别退出 130／143／129；父连接断开为失败。Go 为 Codex Pro worker 保留至少 10 秒终止宽限，全局 kill switch 不提前缩短该子进程的宽限。插件 guardian 拥有 app-server 进程组；worker 意外死亡由插件自己的连接与 guardian 负责终止其拥有的进程。Go 未收到完整回执时不能把内部清理记为已确认，主动脱离拥有进程组的外部程序不属于此保证。

已启动 worker 后缺少可信 `cleanup.confirmed` 会记录 `CleanupError`，即使外层进程已退出。引擎和建议模块保存诊断后停止本次运行，不执行传输重试、渠道降级、Plan 冷启动或下一任务。完整回执中确认清理的普通服务错误仍使用原重试策略。恢复前先确认该次调用遗留的进程已停止；等待固定秒数不构成清理确认。

正常完成后 app-server 仍可能需要 TERM/KILL 收尾。只有 completed、cleanup.confirmed、无取消／超时／截断／失败事实且外层 worker 正常退出时，才允许 receipt 中正常 terminationRequested 导致的信号或未知 Codex exit code。不能将 supervisor exit code 冒充模型进程的物理退出码，也不能把文本答案当终态证明。

### 无推理诊断

`flywheel-codex-pro-worker --doctor` 不读取 stdin，不调用 runMachine，不启动 app-server，仅导入并核验公开接口。成功返回单个 JSON：

```json
{"protocol":1,"type":"doctor","package":"dsh-subagent-codex-pro","version":"0.9.0","machineProtocol":1,"available":true,"inferenceTested":false}
```

Go doctor 同时检查 Node 与 Codex 命令。包导入成功不证明账号、模型、完整策略或真实推理能力，`--probe` 也不会对 Codex Pro 发模型任务。

## Codex／Claude 共有的 worker v3 生命周期

主任务的 fresh/scoped 都使用 v3。保留 v1/v2 字段，增加显式历史模式和该调用独占的恢复目录：

```json
{"type":"start","protocol":3,"requestId":"call-126","cwd":"/absolute/worktree","model":"gpt-6.1-sol","reasoningEffort":"high","permissionMode":"workspace-write","prompt":"执行冻结工作单元","timeoutMs":1800000,"threadMode":"persistent","recoveryDirectory":"/absolute/task/.sessions/recovery-digest"}
```

`threadMode` 必须为 ephemeral 或 persistent；只有 persistent 允许非空 threadId。Go 准备 `.sessions` 父目录并持久化 intent；上游独占建立 recovery leaf，已有目录不允许重发。machine 请求与原始 receipt 仍为 protocol 1，生命周期为 version 1。

上游公开导出 `MACHINE_LIFECYCLE_VERSION=1`、`MACHINE_LIFECYCLE_CAPABILITIES`、`recoverMachine`。capabilities 必须声明 durableJournal/readOnlyRecovery；Codex 的 threadReadyBeforeDispatch=true，Claude=false。worker 原样传递有类型的事件，不解析文本补 ID：

```json
{"type":"lifecycle","protocol":3,"requestId":"call-126","adapter":{"package":"dsh-subagent-codex-pro","version":"0.9.0","protocol":1},"lifecycle":{"lifecycleVersion":1,"type":"lifecycle","requestId":"call-126","runId":"native-run","sequence":2,"event":"dispatch-intent","threadId":"thread-id","turnId":null,"persistent":true,"configuration":null,"dispatch":"not-dispatched","outcome":"unknown","cleanupConfirmed":false}}
```

事件枚举为 thread-ready、dispatch-intent、dispatch-possible、turn-accepted、terminal、cleanup-confirmed；字段记录实际事实，示例中的 null 不是已确认配置。Go 校验 backend 能力、实际身份和合法状态演进。Codex thread-ready 携带完整实际 configuration，ACK 前核对；Claude init 只在 prompt 后出现，不能发明预派发 thread-ready 或真实 turn-accepted。

Go 原子保存、fsync 检查点和 state 后，才向仍开放的 stdin 写一行：

```json
{"type":"ack","protocol":3,"requestId":"call-126","runId":"native-run","sequence":2}
```

仅接受正在等待的精确 ACK。早到、重复、错误 ID/sequence、额外字段、坏 UTF-8、断连或 10 秒 ACK 超时都取消调用。每个 ACK 至多 4 KiB，至多 64 个事件，ACK 累计至多 256 KiB，lifecycle 流至多 1 MiB。worker 安装等待器后才输出事件，不接受提前缓存的“未来 ACK”。Go ACK 不是模型进程终态，也不是 exactly-once 的证明。

独立 supervisor 拥有原生 guardian，并在向调用者发送 terminal/cleanup-confirmed 前持久化完整 receipt。worker 消失时取消未确认步骤、收束原生进程；已保存成功回执不会因通知失败变成未执行。supervisor 自身消失可能只留下 pending，不得盲目重放。

### 精确只读恢复

```json
{"type":"recover","protocol":3,"requestId":"call-126","runId":"native-run","recoveryDirectory":"/absolute/task/.sessions/recovery-digest","original":{"cwd":"/absolute/worktree","model":"gpt-6.1-sol","reasoningEffort":"high","permissionMode":"workspace-write","threadMode":"persistent"}}
```

`original` 仅为旧调用的非秘密绑定，不含 prompt、凭证或环境；不会解析旧 cwd、认证、启动 CLI 或调用 thread/resume。worker 只调用公开 `recoverMachine({protocol:1,requestId,runId,recoveryDirectory})`，返回：

```json
{"type":"recovery","protocol":3,"requestId":"call-126","adapter":{"package":"dsh-subagent-codex-pro","version":"0.9.0","protocol":1},"recovery":{"protocol":1,"type":"recovery","requestId":"call-126","runId":"native-run","status":"pending","receipt":null,"lifecycle":null,"reason":"No durable terminal receipt is available"},"error":null}
```

status 为 terminal/pending/unknown。只有 exact invocation 的完整 terminal 可继续核验；pending 不证明 process alive，错误不附带安全重放的 admission 证明。只读路径可重复且不产生新的模型调用。Go 还核对原 adapter 版本与 checkpoint 摘要；升级后不拿新版本解释不兼容的旧 journal。

cleanup-confirmed 的 Go ACK 前保存原候选、role guard 和 Execute report；恢复到 receipt 不自动获得该证明。缺少它时可信用量可对账，业务保持阻断。状态、metrics 与候选消费语义见[恢复指南](../../docs/session-recovery.md)。

## Claude Code Pro worker

`flywheel-claude-code-pro-worker` 与 Codex 共用父 stdin lifeline、单请求、大小上限、
信号转发与等待收尾的 transport，适配器及回执保持独立。optional peer 精确为
`dsh-subagent-claude-code-pro@0.4.0`，公开导出 `MACHINE_PROTOCOL_VERSION=1`、
`parseMachineRequest`、`MachineInputError`、`runMachine`、`recoverMachine` 和 lifecycle version/capabilities。缺 peer 不影响未选择 Claude
的流程；选中时在 machine 调用前报错，绝不改调 DSH、Codex 或另一版 CLI wrapper。

start 字段与 Codex 外层 v1/v2/v3 一致：type、protocol、requestId、cwd、model、
reasoningEffort、permissionMode、prompt、timeoutMs；v2 额外允许
`threadMode:persistent` 和可选 threadId。v1 映射原生 sessionMode=ephemeral，v2
映射 persistent／显式 sessionId。主任务 v3 额外使用上面的 durable ACK/recovery；只接受 danger-full-access，映射 bypassPermissions；
原生审批模式不等价于 OS read-only/workspace-write。命令来自
FLYWHEEL_CLAUDE_BIN，权限、session、命令和输出上限不能通过额外 start 字段覆盖。

terminal envelope 为 `{protocol,type:"result",requestId,adapter,receipt,error}`。
adapter 是精确 package/version/protocol；receipt 原样保存 Claude machine v1 的
requested、execution、cleanup、session、configuration、output、usage、
reportedCostUSD、costUSD、costStatus、failure。CLI 没有远端 turnId 时保持缺失，
不借本地 runId 伪造远端证据。模型完整 ID、可观察 cwd／权限及会话必须匹配；请求
effort 不冒充已观察的实际 effort。只有成功终态、物理 exit 0、清理确认和外层正常
退出能产生可继续的结果。

插件先建立独立 supervisor 与 guardian 的 IPC lifeline，再派发原生 CLI。worker 被强杀或连接
丢失时收束 native group，并由 supervisor 保存完整 receipt；正常 API 返回等待 guardian 退出。
公开回执仍记录 native PID／PGID。Go 通过原 journal 核对结果，不能从后台清理本身
推断任务成功或恢复资格。

**预派发证明：**只在尚未进入 `runMachine` 的 catch 中，null receipt 的错误
envelope 可额外带 `admission:{dispatched:false,cleanupConfirmed:true}`。Go 必须
同时校验 adapter、请求 ID、协议、null receipt 和错误；成功结果、进入 machine
后的 throw 或坏帧不能使用该证明。它表示本次没有调用后端，可记录已知零消耗。
进入调用后的缺失结果则保持 unknown，即使工作进程随后退出也不允许自动重放。

`--doctor` 仅核验精确 peer 与导出，返回
`{protocol:1,type:"doctor",package:"dsh-subagent-claude-code-pro",version:"0.4.0",machineProtocol:1,available:true,inferenceTested:false}`。
不启动 Claude，不测试账号。完整用户入口见[中文](../../docs/claude-code-pro.md)／
[English](../../docs/en/claude-code-pro.md)。

## Go 角色账本与回执恢复

`Result.Session` 是 Go 的标准化调用事实，不是给所有旧 wire 格式新增字段。
Codex Pro、Claude Code Pro、DSH、Kimi 的主任务 Plan／Execute／Review／Judge 在 fresh/scoped 下都由
Go 保存 intent，再执行一次调用并写入限定大小的原子回执。`last.persistent` 独立控制
角色历史复用；fresh 的原始会话 ID 可保留作审计，但不成为通用 scoped 句柄。既有
Kimi Plan resident 选择保留，Intake／Garden、其他适配器及 mock 保持原路径。

Kimi 的标准化协议名为 `kimi-print/v1`。被引擎跟踪的调用严格检查完整 JSONL 与最终
resume_hint，恢复 ID 必须匹配；没有远端 runId／turnId 或可归属 usage，保持 unknown
用量。CLI 正常成功只支持正常续接资格，不构成任意中断后的持久化或清理证明。

待消费结果须绑定原任务、物理 cwd、运行绑定、候选和原始角色保护证据。
`guard-before`／`guard-after` 持久化，Review／Judge 还绑定原契约及原验证证据；
重启后不得重新采样“调用前”状态或重跑验收来使旧裁决有效。Plan 获准的契约修订仍
由完整调用后候选与非契约保护检查约束。回执消费与计量通过状态 CAS 一起推进；
消费已完成结果不重复派发或计费，预算继续约束新的调用。

未知 dispatch／outcome／cleanup 会阻止继续执行，包括变更模式或适配器后。
Codex／Claude 的 v3 另提供精确 journal 的只读恢复，不扫描后端历史。未知结果不会因质量阈值、角色覆盖或新 generation 被解除。

## 桌面控制响应（独立于 worker JSONL）

桌面控制层调用 Go 的 `control` 命令，只接收一个 JSON 文档，不把它与上述 worker 流混用。原 `start/status/stop` 继续支持，下列入口复用同一协议：

| action | 输入与输出 |
|---|---|
| `launch` | 必需 taskDir／cwd；可选 `--mode`（auto、quick、full），缺省 auto；校验已准备契约并启动 |
| `start` | 必需 taskDir／cwd；可选 `--mode`，省略时保留契约或持久化策略；旧任务仍 Full |
| `config` | 必需 cwd，可省 taskDir；返回 `configuration` |
| `doctor` | 必需 cwd，可省 taskDir；可选 `--probe`；返回 `diagnostic` |

共同 envelope 为 `protocol:1, ok, running, pid, taskDir, state`，可选 `cwd`、`error`。成功解析目录后 `cwd` 是请求目录的规范化物理路径，供宿主关联校验；旧回复未带该字段仍按原任务身份机制处理。省略任务时 `taskDir:""`；配置／诊断不启动或修改任务，通常 `running:false, pid:0, state:null`，不能用这些字段覆盖面板已经显示的任务运行状态。doctor 可只读已有工作流状态以判断必需阶段。

工作流事实来自 Spec，最终策略由 Go 保存到 `state.workflow`：version、requested、effective、reason、evidence、risks、selected-at、stages 和 upgrades。`stages.plan/judge` 各有 applicable 与 reason，不使用伪造 pass。launch 默认 Auto 只是请求，不是承诺一定 Quick；显式 Quick 也必须通过资格校验。未知版本或非法模式在模型启动前失败。已有策略恢复时不重分类，Full 不降为 Quick；upgrade 先持久化再进入 Full。status／stop／config／doctor 不接受 `--mode`。

`configuration` 含 source、scope、cwd、taskDir、sessions、bindings。sessions 含 mode／policy-version、可选 roles 与 quality 配置；每个 binding 包括 id、role、route、profile、harness、provider、channel、model、thinking、residentSession、permissionMode、inScope 及 session。session 含 mode、supported、reason、historyRecoveryTested:false，仅报告选择与适配器能力，不证明具体历史存在。账号渠道与实际 adapter 分开；未给出的旧配置值不得由 UI 猜成已验证。

有会话记录的状态快照另含紧凑 `sessions` 数组：role、scope、generation、action、reason、
resumeCandidate，以及 last 的 status、persistent、dispatch、outcome、cleanupConfirmed、
usageStatus、accounted。投影只读取已校验的 state，不打开后端历史或原始回执；旧的
完整 state 字段继续保留。状态查询不会核销回执、修改计量或解除未知调用的阻断。

`diagnostic` 含 kind=`doctor`、ok、cwd、taskDir、probeRequested、inferenceTested=`false`、可选 configuration，以及 checks 数组。check 为 id/status/summary，可附 code/remedy；status 只允许 pass/fail/warn/unknown/skipped。配置本身无效时允许没有 configuration，仍返回完整失败检查项。

合法检查失败同时满足 exit 1、envelope.ok=false、diagnostic.ok=false；控制层保留并展示报告。其他非零退出、超时、信号、坏 JSON、错误 protocol 或任务身份仍是传输错误。普通 config/doctor 不接受 shell 参数拼接，`--probe` 不能用于其他 action。Go 检查预算 40 秒，TERM 升级、stdio 排空、进程组确认各有 5 秒宽限，保守另留 15 秒；桌面默认控制窗口 60 秒。

config/doctor 的 Go CLI 将 SIGTERM/SIGINT 转为 context 取消，等待 watchdog 收尾后才退出。控制调用取消或超过窗口时，host 先发 SIGTERM，诊断动作再留 20 秒让 Go 完成分阶段清理，仍未退出才 SIGKILL；取消始终报告控制失败，即使迟到结果成功也不覆盖它。launch/start/status/stop 保持原控制进程的 500ms 收尾，后台引擎停止仍走独立 stop 协议。

### DSH 启动工具与任务关联

`flywheel_launch({taskDir,cwd?,mode?})` 使用 action=launch，mode 缺省 auto。
`flywheel_control({action,taskDir?,cwd?,mode?,probe?})` 与人工 `/flywheel` JSON 命令
使用同一校验与控制层；mode 仅限 launch/start，probe 仅限 doctor。
省略 cwd 时采用当前会话公开 workingDirectory 服务，跨 worktree 时显式传入。
若 `header.cwd` 和 `working-directory/change` 均未记录当前目录，launch/start/stop 必须显式传入绝对 cwd；宿主在派发 Go 前返回 `not-dispatched`，即使 getter 有外部默认目录也不据此猜测关联，status 仍可使用 getter。
CLI 只消费已准备契约，宿主的 `flywheel-task` skill 负责研究与生成契约。

`sessionProjections` 的 `flywheelTask` 投影从宿主已有的 command/tool/PTC
调用与结果记录解析具体任务。客户端只通过公开 `useProjection('flywheelTask')`
读取，按会话隔离，不扫描任务目录，不自行写入新的会话事件或 state。
会话重载可从保留记录重建关联；fork 不继承父会话关联。投影记录是控制交接信息，
恢复后仍查询 Go 取得最新状态，不把历史 running 值当成实时事实。

launch/start 的在途或未知结果保留原 taskDir/cwd 并阻止重复变更，直到 status
成功核对原任务。确认新关联时可自动查询一次状态，不自动启动或周期轮询。
旧会话、旧绑定的迟到回复不覆盖当前选择。手动路径草稿与控制快照仍是查看缓存，
不能替代可重放的明确任务关联。

### DSH 命令失败与是否派发

`/flywheel` 的 DSH command 结果仍使用 `kind:"success"` 或 `kind:"error"`。成功结果的 `text` 是上述 Go control JSON，包括具有合法诊断报告的 doctor 失败。新的 host 将所有 command 失败包装为以下独立 JSON，放在 `kind:"error"` 的 `text` 中；这不是 Go stdout envelope，也不改变 worker JSONL 或模型工具的异常接口：

```json
{"protocol":1,"type":"flywheel-control-error","action":"start","dispatch":"not-dispatched","message":"taskDir must be an existing directory"}
```

| 字段 | 语义 |
|---|---|
| `protocol`、`type` | 固定为 `1` 和 `flywheel-control-error`，用于区分命令失败与引擎结果 |
| `action` | 可读取的请求 action 字符串；解析失败或没有字符串 action 时为 `null`；界面必须与自己的在途请求匹配 |
| `dispatch` | `not-dispatched` 表示宿主证明本次请求没有启动 Go 控制进程；否则为 `unknown` |
| `message` | 给用户诊断的错误文本，本身不证明进程是否启动，也不能作为另一个 envelope 递归解析 |

只有参数／目录等派发前校验失败、派发前取消，以及进程创建失败且没有成功启动的证据，才能返回 `not-dispatched`。省略 cwd 时，host 读取 `workingDirectory.get(agent.session)` 的当前目录；getter 失败、缺会话或缺目录属于派发前失败，不回退 header.cwd。显式 cwd 不读取会话目录。进入控制进程后出现的非零退出、超时、取消、坏 JSON 或任务身份不符都不能取得这项证明，即使错误文本包含相同 JSON。host 对普通错误同样增加外层 envelope，只把原始错误放入 `message`，避免进程返回的文字冒充宿主确认。

界面只有在版本、类型、action、dispatch 和 message 都符合契约时才使用宿主证明。未知协议、旧 host 的纯文本错误、DSH 传输失败或丢失响应均保持 `unknown`。启动／停止的 `unknown` 结果必须先成功查询原任务状态，再决定后续变更；不得自动重试。已确认未派发的拒绝会显示原因，用户可以修正输入或环境后重新提交。一次未派发的状态查询也不能解除更早启动／停止留下的未确认结果。

host 在调用 Go 前把任务和仓库目录规范化为真实路径，并校验引擎返回的任务身份。界面不能把用户输入的符号链接、`..`、尾部斜杠或 Mac `/var` 与 `/private/var` 的文本差异当作引擎身份错误；响应接纳依赖 host 的路径校验和当前请求关联。

### 浏览器查看状态的边界

浏览器端遵循[外部 plain-JS 插件实践](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/preset/agent-preset/skills/cordis-plugin-development/references/practices.md)，不运行时导入 Harness Client 包。插件自有的 store handle 只通过公开的 `spec`／`create()` 结构交给槽位注册；每个实例提供 `actions`、`getSnapshot()`、`subscribe()` 和 `clearPersisted()`，由宿主负责会话作用域、实例缓存与 hooks 绑定。当前没有持久化配置，`clearPersisted()` 不删除任何任务文件。

异步控制层另有插件自有 observable，快照在未变化时保持同一身份，经 `inject.hooks` 交给宿主绑定；React 组件不自行订阅，不读取 ctx 或原始服务对象。路径草稿与控制快照只是查看缓存，不成为任务账本，也不写入新的 DSH 会话事件。模型可见的命令和工具结果仍由宿主原有记录机制处理。此查看契约不改变 Go control 协议、worker JSONL 或后台任务的停止语义。

## npm 安装报告与桌面入口

以下是 Node 安装器的独立契约，不与 worker JSONL、Go control envelope 或任务 `state.json` 混用。入口为 `npx -y @ckanner/flywheel@latest install` 或已安装包的 `flywheel setup`；二者共用参数与行为。普通 npm 包安装没有修改用户目录的 postinstall，只有显式 install／setup 执行 skills、安装记录和 DSH profile 配置。

### 安装结果

加 `--json` 时 stdout 只包含一个 JSON 文档；安装中的外部命令输出由安装器捕获，不混入此文档。正常进入安装流程后的字段如下：

| 字段 | 含义 |
|---|---|
| `protocol` | 固定为 `1` |
| `ok` | 全部请求的软件安装步骤和最终版本核对是否完成；不是账号或推理成功 |
| `version` | 本次安装器的精确包版本，决定 CLI、平台包、worker 与插件版本 |
| `steps` | 顺序记录 `global-cli`、`skills`、`desktop-launcher`、`dsh-plugin`、`preflight`；失败时只含已开始的步骤 |
| `ready` | 本地 doctor 是否无阻断项；诊断未完成时为 `false` |
| `authentication` | 固定 `unknown`，没有自动登录或真实模型请求 |
| `prefix`、`paths` | 得到后才记录；paths 包含 CLI、原生二进制、DSH worker、codexProWorker 与生成的 launcher 的绝对路径 |
| `adapter` | 实际导入通过后记录 Codex Pro 包名、精确 0.9.0、protocol 1 与 inferenceTested=false；不代表账号已就绪 |
| `skills` | 安装成功时的数量及准确目录；每项结果保存在 skills 步骤的 `summary` |
| `diagnostic` | 合法的完整本地 doctor 报告，包括失败检查项；不执行 `--probe` |
| `error`、`cancelledBy` | 失败原因、可选取消信号；不是成功结果的一部分 |

每个 step 具有 `name` 和 `status`，结束时为 `ok`、`warning`、`skipped` 或 `failed`，按步骤附带 `error`、`reason`、`version`、`summary`、`path` 或 `profile`。成功的桌面插件步骤包含 `reloadRequired:true`；宿主仍需按自身流程重新载入。参数解析失败也返回 protocol 1 的单个失败文档，但尚无 `ready`、`authentication` 等运行结果。

缺少 Kimi／Codex CLI 或配置未准备好时，合法 doctor 失败报告使安装结果成为 **`ok:true, ready:false`**，preflight 步骤为 warning，安装退出码仍为 0。doctor 必须满足退出码 0／1 与 `diagnostic.ok` 一致；坏 JSON、超时、进程信号或其他退出码都使安装失败，不能伪装成“工具尚未准备好”。

安装失败退出码为 1；SIGINT／SIGTERM／SIGHUP 取消按 `128 + signal` 返回，分别为 130／143／129。外部安装步骤有独立时限与进程组收尾窗口，取消不因迟到的成功结果转绿。已完成的 npm／DSH／skills 步骤不自动整体回退，调用者应保留报告、修复具体原因并重跑。

`--prefix`、`--project`、`--dsh-bin`、`--profile`、`--no-dsh`、`--no-global`、`--timeout-ms`、`--json` 的完整语义见[教程](../../docs/getting-started.md#2-一条命令安装)。`--no-global` 指当前已经持久安装的包；默认 install 与 setup 都会确保本次版本的全局安装。`--no-dsh` 仅显式跳过插件注册，不把可能失败的本地 doctor 当成已通过。

### GUI 安装记录

安装器生成 `~/.config/flywheel/installation.json`（0600）与 `npm-launcher`（0755）。记录包含 `protocol:1`、`owner:"@ckanner/flywheel"`、`version`、`prefix`、`cli`、`worker`、`codexProWorker`、`adapter`、`node` 和 `launcher:{path,sha256}`。它只描述软件入口，不保存账号凭证，不构成第二份任务状态。旧记录仍按原字段读取，升级 setup 生成新字段；用户修改的 launcher 继续受保护。

launcher 使用绝对 Node 与 CLI 路径，设置安装时可见的绝对工具 PATH 目录，并在用户没有显式设置 `FLYWHEEL_DSH_BIN` 时使用已核对的桌面命令。npm CLI 启动器解析版本一致的原生包和 worker，缺失或版本不匹配时拒绝继续，不隐式下载另一份二进制。临时 npx 缓存与 `node_modules/.bin` 不被记录成持久工具 PATH。

桌面插件按以下顺序选择入口：

1. 用户显式 `flywheelBinary`，必须是非空字符串；它优先于安装记录。
2. 存在安装记录时，验证其协议、owner 和绝对 launcher 路径，再验证 launcher 是普通非链接的可执行文件且 SHA-256 匹配。
3. 仅当安装记录不存在时，兼容旧配置，从 PATH 查找 `flywheel`。

损坏记录、符号链接、缺失或被修改的 launcher 都明确失败，不静默退回 PATH。SHA-256 核对的是 launcher 内容，不是账号状态或所有外部 CLI 的真实性。已有用户修改会在 setup 时保留并报告冲突；修复方法见[升级教程](../../docs/getting-started.md#8-升级排错与卸载)。普通 npm 卸载不会擅自删除安装记录、配置、skills 或任务数据。

## DSH 宿主兼容性（0.28.4）

包声明只接受 `0.2.1-rc.5` 与 `0.2.2-rc.1`。新 SDK 会按策略文件所属包的 engines.dsh 校验其准入；策略插件未加载就没有本次私有权限回执，worker 必须拒绝派发模型 prompt。权限插件在公开 agent/created 阶段（创建公告结束后、首个模型请求之前）写入显式 approval/policy，不在 session/created 只读公告中追加事件，再只读公开 approval.overrideOf(session)，不访问私有 effectivePolicy。证明通过本次 nonce/request/session 关联的私有文件返回，不追加自定义持久事件；未知／缺失回执仍不放行。

持久化 create／新进程 resume 的公开 SDK 验证针对 `0.2.2-rc.1` SDK、JSONL persistence
与 AgentLoop。`0.2.1-rc.5` 的 Host／UI／插件兼容测试不代表历史恢复已验证。无真实
账号、模型或 Mac 实机的测试不能证明这些环境通过，也不构成性能／缓存收益实验。
