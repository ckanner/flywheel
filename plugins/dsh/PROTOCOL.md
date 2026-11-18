# Go ↔ DSH / Kimi / Codex Pro / Claude Code Pro worker 协议

此文件记录独立 worker 协议。前半部分对应 `internal/drivers/dsh.go` 与官方 DSH SDK worker；[Codex Pro 部分](#codex-pro-worker协议)对应 `internal/drivers/codex_pro.go` 和插件 machine API worker。Go 负责状态机、验收、费用上限及最终进程判断；worker 仅执行一次有边界的调用。Go与worker之间使用本地stdio；Kimi native worker内部使用其拥有的loopback Server，见本文件原生扩展和Kimi专属协议。

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
| `idleTimeoutMs` | 可选的 1–86,400,000 毫秒，默认 300,000；策略插件据此合并真实活动通知，外层 Go watchdog 执行静默预算。Go 始终显式传递本轮有效的 idle 和 wall 值，零配置分别使用 300,000 和 3,600,000 毫秒 |
| `workspaceCwd` | 可选的稳定项目目录，必须存在并规范化。SDK 在该目录初始化 Session header，当前执行目录仍为 `cwd`；ready 必须回显匹配的项目目录。省略保留旧行为 |
| `sessionTitle` | 可选的人类可读任务／角色标题；只给新 Session 设置，续接不覆盖现有标题。完整标题交给公开 `sessionTitle.rename`，再用 `get` 验证已接受的原生 snapshot；由 DSH 的 `maxTitleBytes` 处理 UTF-8 长度，不额外截成 35 字符。显式请求新标题而服务缺失或回读失败时，在 prompt 前失败 |
| `web` | 可选的严格 Web extension v1；仅精确 Web model 使用，详见下文 |
| `native` / `managed` | 版本化能力选项与Go生成的实际调用身份，见原生扩展 |

项目归属与执行目录分离时，worker 在发送 prompt 前使用公开 working-directory 服务切换并核对当前目录；权限根仍如实记录 DSH 的 Session header 语义。当前 DSH 不支持将 `workspace-write` 沙箱根与项目目录独立设置，因此这类不同目录组合在派发前拒绝，不把权限改成 full access。`read-only` 与显式 full access 保留其实际权限。doctor 使用临时持久化根，清理后不留下新的用户会话。桌面 Workspace 登记只通过 Host 的公开注册表执行。

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

## ChatGPT Web extension v1

`model:gpt-6-pro-web` 必须显式使用 `reasoningEffort:pro` 并携带：

```json
{"web":{"version":1,"executionDomainId":"flywheel-stable-task-digest","requireIsolation":true}}
```

可选 `expectedBinding` 是上一已确认历史的完整 binding；可选 `probe:true` 仅匹配
`--doctor`，不能放入普通 start 或 prepare。未知字段拒绝。旧 worker 不认识 `web`
字段会在模型派发前失败，不能靠回显 model 名冒充支持。

正式 protocol 2 persistent start 可额外携带 `web.recoveryDirectory`，必须为
Go 已准备的原任务 `.sessions/recovery-<原 requestId 的 SHA-256>` 绝对路径。
worker 独占创建 0700 子目录和 0600 `worker.json`；prepare/probe 拒绝该字段。
host `recoveryCheckpoint(identity)` 必须在 beginInvocation 后、prompt 前提供
`chatgpt-web/read-only-recovery/1` 原身份锚点；缺少时正式调用在 prompt 前失败。

Web 的 prepare/start 都经实际 SDK 精确模型 metadata 校验，并要求 ChatGPT 插件
公开 `chatgptWebHost` v1 与已有唯一 `compaction.compactIfNeeded` 服务。
插件宿主准入要求 `summaryRoute:2`，其他必需 capability
`replayVeto`、`durableResponseAck`、`durableSummaryAck`、`pressurePolicy` 均为 1；
旧 `summaryRoute:1` 不能放行。该独立能力升级不改变 worker Web extension v1、
`chatgptWebHost` v1、binding 的 `hostContractVersion:1` 或 recovery 契约版本。
DSH 持久事件、derived history 和 compaction checkpoint 是权威，唯一 CompactionEngine
提交历史变更，工具仍由 DSH 执行。插件提供 Web 投影、预算/摘要路由、迁移与恢复
协调；其同步游标、映射和证据不构成第二份权威历史。
`compaction/summary-route` 用 `configuredTarget` 传递用户显式摘要 provider/model，
优先于本次 `proposedTarget`。只有选中的摘要目标仍是该 Web 模型时，插件才应用
Pro 与 Web 输出预算；显式其他路由保留其 effort 与计量。未显式配置时才依据
proposed target 选择 Web 默认摘要，不能让旧历史默认模型覆盖本次模型选择。
宿主的手动压缩维护操作必须保留真实发起 Agent 的上下文，使辅助摘要的恢复元数据
和执行域绑定原 Session 与物理 workspace。插件状态目录不能替代 workspace，
也不能由插件按 sessionId 猜测 Agent。摘要捕获后中断仍阻断同一 workspace 的新
Agent（包括 fresh 非 Web 路由）；原摘要只经显式、幂等的宿主恢复事务采用。
`admitBinding` 返回 protocol 1、transport `chatgpt-web`、精确 provider/model/effort、
connectionId、accountFingerprint、policyVersion、contextPolicy `project-only`、
observation `opaque`、usage `unknown`、hostContractVersion 1、runtimeVersion
`web-runtime/1`。prepare 的 `warm-ready.web` 是这个静态 binding，不能证明浏览器活着
或额度可用。正式 `ready.web` 再核对同一 binding；resume 还与 expectedBinding 完全一致。

正式 agent 创建时先调用 `beginInvocation`，参数为本轮 requestId（hostInvocationId）、
稳定 executionDomainId、真实 sessionId、物理 workspace、requireIsolation 和从本轮起点计算的绝对 deadlineMs。
预热从不调用 beginInvocation；同 invocation 的重复准入由插件幂等处理，未决旧域不可覆盖。
公开 `chatgpt-web/activity` 只接收身份匹配、单调 sequence 的实际事件，聚合为既有
DSH `activity` 帧中的 `provider/chatgpt-web`，不转发原文或私有 nonce。
Go 对该 transport 采用 wall-only，保留取消及清理预算。

SDK 正常关闭会通过真实 Cordis effect disposer 发布私有、原子、已 fsync 的终态。
worker 读取并校验后在 `result.web` 返回：

```json
{
  "version":1,
  "binding":{"protocol":1,"transport":"chatgpt-web","provider":"chatgpt","model":"gpt-6-pro-web","reasoningEffort":"pro","connectionId":"opaque-connection","accountFingerprint":"opaque-account","policyVersion":"policy/1","contextPolicy":"project-only","observation":"opaque","usage":"unknown","hostContractVersion":1,"runtimeVersion":"web-runtime/1"},
  "facts":{"version":1,"hostInvocationId":"call-123","executionDomainId":"flywheel-stable-task-digest","sessionId":"flywheel-session","dispatch":"dispatched","outcome":"completed","remoteGeneration":"idle","localCleanupConfirmed":true,"usageStatus":"unknown"}
}
```

facts 聚合主调用、辅助模型和子调用，invocations 详细 journal 留在插件；Flywheel 只存
有界摘要。dispatch 为 not-dispatched/dispatched/unknown；outcome 为
completed/failed/cancelled/unknown；remoteGeneration 为 idle/running/unknown。
`web.facts.dispatch` 表示浏览器提交，`lifecycle.dispatch` 表示 SDK prompt 派发；
SDK 已接受 prompt 而浏览器未提交时两者不同，不得抹掉已变化的 SDK 历史。
remote 非 idle 或 dispatch unknown 不能同时宣称已知 outcome；本地关闭不证明远端结束。
普通成功必须 dispatched/completed/idle 且 localCleanupConfirmed。doctor 成功只表示探针通过，
facts 保持 not-dispatched/failed/idle。result.usage 与 costUsd 必须 null，lifecycle
usageStatus 必须 unknown；缺失回执、身份冲突、伪造 0 用量均拒绝。

managed Web 的 ledger 为 v5，`web-binding` 保存精确动态绑定；unknown 调用仍由既有
全任务恢复栅栏阻止跨角色、fresh 或更换 provider 自动重放。物理 worker 退出与
remote generation certainty 独立判断。Web SDK 关闭宽限 15 秒，外层至少 20 秒。

### 原调用只读恢复协议

发布包将精确候选的公开 `dsh-plugin-chatgpt/web-recovery` reader 与 schema 验证器
编译为自有 `dist/web-recovery-reader.mjs`，恢复 worker 以相对路径加载。
其 manifest 与原许可证同包交付，候选验证核对来源及输出摘要；该模块仅允许
Node filesystem/crypto/path/os 外部导入，不初始化 SDK、provider 或浏览器。
用户另一个 SDK profile 安装的 provider 不被当作 worker 的解析依赖。

Go 对旧 unknown 调用发送单个 JSON 行并关闭 stdin：

```json
{"type":"web-recover","protocol":1,"requestId":"original-call","recoveryDirectory":"/task/.sessions/recovery-<sha256>","original":{"cwd":"/repo","workspaceCwd":"","provider":"chatgpt","model":"gpt-6-pro-web","reasoningEffort":"pro","permissionMode":"workspace-write","persistent":true,"requestedSessionId":"","executionDomainId":"original-task-domain","native":null}}
```

可选 `expectedBinding` 是原已保存的完整 Web binding。该查询不能混用 doctor 或
prewarm。短期进程只加载 `dsh-plugin-chatgpt/web-recovery` 公开文件读取 helper，
不创建 SDK、浏览器会话、模型、工具或恢复采用事务。必须核对原 worker/SDK owner
已退出、私有文件/目录无 symlink、原 home/profile/bin、identity/settings 摘要、
同域 invocation 版本、原 connection/account/policy、所有调用和本地工具已结算。
对于 profile 管理配置，helper 证明冻结原配置与原调用一致；不重新加载当前 profile。
只读恢复 worker 在原调用的物理 `cwd` 启动，保留相对 worker 路径及依赖工作目录的
wrapper 行为；不能改用引擎本次启动目录来解释原调用。

成功查询返回 `{protocol:1,type:"web-recovery",requestId,revision,frames:[ready,result]}`。
revision 是所读原证据快照的摘要；ready/result 仍通过正常 protocol 2 严格校验。
插件 disposition 不是业务成功：`not-dispatched` 只能生成 failed；`retired` 只能
生成 failed 且要求原 SDK 正常关闭与实际观察过的原 turnId；缺 turnId 仍阻断，不得构造。
`committed` 还须存在相同 prompt 的原
wait/terminal/completed、原关闭记录与相同主回答，才允许 completed。
纯 final/tool/summary adopt 缺少这些业务证据继续阻断，未知 remote/tool/cleanup
也不能解除。原已知 SDK failed 加同一已结算插件失败可恢复为 failed。

插件 `not-dispatched` 不单独证明 SDK 未派发。只有原 `admitted` 阶段，或已结算
原记录明确说明 SDK 未派发且没有 prompt receipt，才保留 lifecycle not-dispatched。
SDK 已接受 prompt 时必须有其原失败终态、真实 turnId、prompt receipt 与正常关闭
证据，恢复为 lifecycle dispatched/failed，同时保留 Web facts not-dispatched。
派发意图或接收后的 SDK 终态仍未知则继续阻断，不复用已追加失败 prompt 的历史。

worker 查询不写任何原证据。Go 另行核对原 terminal receipt 的 candidate/role
guard，并原子更新自身 receipt/state；已派发调用缺失该原检查点继续阻断。
仅正面证明 SDK 未派发的调用可在当前 candidate/guard 等于原 before 时结算失败。
usage/cost 保持 null/unknown；完成对账不新增模型调用，也不重复预算记账。

原 Go candidate/guard 在覆盖主结果前写入独立 `role/scope/web-original` receipt
槽，其摘要先通过 `recovery.checkpoint-digest` 的 state CAS 确认。重启后严格按此
digest 重读原证据，即使主结果已替换而最终 CAS 未完成也能幂等重做对账。

## 运行参数

| 环境变量 | 默认值 | 用途 |
|---|---|---|
| `FLYWHEEL_DSH_WORKER` | npm 启动器解析同版本 worker 的绝对路径；独立 Go 二进制为 `flywheel-dsh-worker` | Go driver 使用的 worker 可执行文件路径；用户显式变量优先 |
| `FLYWHEEL_DSH_BIN` | `dsh` | worker 使用的官方 CLI；支持绝对路径，也可明确依赖 PATH |
| `FLYWHEEL_DSH_PROFILE` | `sdk` | 已配置的 SDK profile；禁止 `desktop` |
| `DSH_HOME` | 官方 CLI 默认值 | DSH 自己的配置、授权引用及会话持久化根 |

worker 不搜索 `.app` 内部目录、不下载或更新 CLI、不复制桌面授权文件，也不读取或修改 Flywheel 的 `state.json`。Go 的凭证环境白名单仍是外层边界。

worker 用 `dsh --profile <profile> --patch <临时覆盖文件>` 启动 DSH。覆盖文件在临时目录内、权限为 `0600`，结束后移除。DSH 子进程继承 Go 建立的 worker 进程组，禁止再为 SDK 创建 detached 组。未配置native时覆盖禁用自主goal／workflow／delegation、辅助模型调用及schedule；显式native按下文启用受管agents/Team/Workflow/Web/LCU，自主goal/schedule仍关闭。不修改用户profile或提醒存储。

## 输出帧

每帧均回显请求的 `protocol`（1 或 2）及原 `requestId`。顺序是 `ready` → 零或多个
`event`／`native-event`／`activity` → 恰好一个 `result`；启动失败允许直接 `result`。v1 创建 Session 时的权限
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

ready 的 `activity` 说明本次策略插件实际安装的观察通道：`version:1`、
`observation:"events-only"`、`root:"agent/assistant-stream"`、
`children:"admitted-local-agents"`（未启用原生子 Agent 时为 `"disabled"`），以及
`externalProviders:"flywheelProviderActivity/v1-when-installed"`。同时明确
`opaquePhases` 包括 startup、before-first-event、silent-tool、external-provider-without-activity。
旧 SDK fixture／策略未提供此证明时只报告 `durable-events-only`，不声称可观察整棵调用树。

### event：可审计的原始持久事件

```json
{"protocol":1,"requestId":"call-123","type":"event","sessionId":"flywheel-uuid","event":{"seq":12,"type":"turn/end","data":{"turn":0,"reason":{"kind":"completed"}}}}
```

普通 `event` 只转发该根 Session 的持久事件，按 `seq` 去重。SDK streaming usage 不作重复累加。未配置 `native` 时，根 Session 出现子智能体启动会立即失败并关闭 SDK。显式 `native` 的 owned children 由 `NativeTree` 归属和核对，通过 `native-event` 与终态回执报告；Go 继续管理角色调用与外层编排。

恢复过程中重放的旧事件不进入当前 output／usage fold，也不触发 Go 的 OnEvent。
worker 在 admission 后、发送本次 prompt 前重置 fold，只计当前调用新增的输出与用量。

### activity：不含正文的实时进展

```json
{"protocol":2,"requestId":"call-124","type":"activity","sessionId":"flywheel-uuid","activity":{"version":1,"sequence":7,"kind":"provider-progress","source":"dsh","scope":"child","eventTypes":["assistant/reasoning-delta"],"observedEvents":38}}
```

公开 DSH SDK 的 `session.event` 只转发持久事件。策略插件另订阅公开
`agent/assistant-stream`，观察真实 `start`／`chunk`／`end`；metadata 不含文本、
推理正文、工具参数、命令输出或凭据。仅接受已通过本轮权限／原生能力 admission 的
真实 Agent 对象，核对注册表、稳定父链与根 Session；外来 Agent、旧对象、已释放父链
和重放的 stream revision 不构成本轮活动。子 Agent 独自流式输出时同样可刷新静默预算。

所有 root／child 通道共享一个合并窗口 `max(1, min(1000, floor(idleTimeoutMs/4)))` 毫秒。
只有新的真实观察才能发送通知，计时器本身不发送心跳。`sequence` 严格递增；
`observedEvents` 计观察次数，`scope` 为 root／child／mixed。类型仅允许
`assistant/` 前缀的 start、chunk、end、block-start、block-end、text-delta、reasoning-delta、
tool-call-delta、usage、finish，以及 provider/codex-pro、provider/claude-code-pro，
后两项代表已校验的外部 Provider 回调，不表示它创建了一个 DSH local Agent。
每帧最多 16 个去重类型，Go 验证完整活动帧最多 4096 字节，拒绝多余字段、错误身份、
错误类型及重复序号。活动计数不进入 token usage、费用、持久历史、完成判定或清理证明。

冷调用与预热调用的 Go stdout capture 都只保留最近一条已经校验的活动诊断，
原始活动仍逐条送达 OnEvent 并刷新 watchdog。活动先绑定完整 matching ready，再核对
序号；累计活动超过 16 MiB 也不会挤掉终态回执。普通输出和终态不被删除，仍受原有
捕获上限约束。错误活动、终态后的帧或 EOF 未换行残帧保留明确的解析失败证据；
过滤早前活动不能使重复／身份错误的帧重新变得合法，也不能产生 cleanup 证明。

策略插件向同一个 SDK stdout 写一个 namespaced `flywheel.activity` JSON-RPC notification；
原 SDK 仍是唯一 stdin reader 和请求服务器。私有 envelope 绑定 requestId、nonce 和
rootSessionId；worker 只有在 matching admission 与本轮 prompt 派发后才接收，再输出上面的
外层活动帧。外来身份和重复私有通知被忽略；已匹配身份但格式损坏会使协议失败。

Codex Pro／Claude Code Pro 等外部 Provider 在 foreground、PTC 和 background jobs 中可能
没有 DSH local Agent，因此使用**可选的 Flywheel host 扩展**，而不是伪造 DSH stream：

```js
const activity = ctx.get('flywheelProviderActivity');
// activity.version === 1; activity.intervalMs is the host's coalescing interval.
activity.report(owningAgent, { ...machineActivity, invocationId });
```

`machineActivity` 采用公开 machine API v1 的 version、sequence、kind、source、scope、
eventTypes、observedEvents 元数据；`invocationId` 是插件为本次调用生成的唯一标识。
服务验证 source 为 codex-pro／claude-code-pro、真实 owning Agent 及每次调用的递增序号，
再合并为 DSH 活动。插件在取消或结算后停止报告；普通 DSH 没有这个服务时保留原有行为。
未接入扩展的 Provider、首事件前等待、静默工具和启动阶段仍不可连续观察。
DSH 的 idle 是可配置的静默资源预算，不是模型死锁证明；独立 wall deadline、取消和
进程清理仍然生效。不能靠增大发送频率、重复元数据或人工心跳掩盖这些阶段。

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

外层 idle／wall 超时保留 TIMEOUT 及各自诊断；worker 为配合清理而返回 cancelled 不改变
发起终止的原因。用户主动取消仍为 Cancelled。持久调用被中断后业务 outcome 保持 unknown，
不自动续接或重放；但完整、身份匹配的回执及正常进程退出可以独立保留实际 Session ID、
dispatch、resumeAccepted 与 cleanupConfirmed。退出 143 本身不抹掉已经验证的 SDK shutdown。
损坏／截断的传输、物理信号、缺少退出证据或清理错误仍保留 cleanup 未确认。

v1 每次调用使用全新 session。Go 重试时重新提交冻结任务输入，已有 worktree 与外部验收证据由 Go 决定是否保留；不会恢复旧 Session 后悄悄更换模型或权限。桌面关闭、窗口隐藏、插件卸载均不会主动停止已经由引擎后台启动的任务；显式 `control stop` 才请求引擎停止并等待清理。

`flywheel-dsh-worker --doctor` 接收同一请求格式，只执行初始化与权限握手，然后关闭 SDK，绝不发送模型 prompt。它不能证明凭证、远端推理或终端 UI 已通过真实使用验证。

Go 的 `DSHArgs.Doctor` 使用这个入口；为保持 v1 必填 `prompt` 的 admission schema，它会用固定的探针说明替代调用者文本，worker 不将其交给 `session/prompt`。doctor 仍必须具备匹配的 ready、可靠 result、正常 SDK shutdown 与物理退出，不能因为没有推理而放松清理或权限检查。

## DSH SDK 一次性预热

`scheduler.prewarm:true` 允许引擎在确定性检查期间预启动即将使用的 DSH Review
worker；默认关闭，是否改善实际任务耗时仍须配对验收。只有 DSH 的公开 SDK 支持这里
的无 prompt 初始化。Codex Pro、Claude Code Pro 和 Kimi 继续原冷启动路径；历史复用
与模块导入不称为原生预热。

Go 使用同一个 `flywheel-dsh-worker --prewarm` 可执行入口。此模式的 stdio 是独立的
两阶段 envelope protocol 1，**从 prepare 到最终退出始终保持 stdin 开放；EOF 即父进程
失联，不能当作派发许可**。旧 v1/v2 单请求入口的 EOF 语义不变。prepare 最多等待
10 秒，每条完整 UTF-8 JSON 帧不超过 8 MiB，不允许重复、未知字段或提前 start。

```json
{"type":"prepare","protocol":1,"warmId":"warm-owned-123","binding":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","receiptDirectory":"/absolute/task/.control/prewarm/warm-owned-123","ttlMs":60000,"cwd":"/absolute/worktree","provider":"deepseek-account","model":"deepseek-flash","reasoningEffort":"high","permissionMode":"read-only"}
```

prepare 不含 prompt、Session ID 或正式 Request ID；可选 `workspaceCwd` 与普通请求
同义，`idleTimeoutMs` 与正式请求绑定，确保预先安装的活动观察使用同一静默预算。
Go 的 binding SHA-256 绑定实际 command、SDK bin/profile/home、完整白名单子进程
环境、目录、provider/model/effort、权限和调用时限；只保存摘要，不输出凭据。目录必须
是新建空的绝对物理路径和 0700 私有目录。TTL 上限 60 秒，Go 与 worker 双方执行。
Go 到期先原子禁止正式派发，再允许最多三秒正常清理，随后由 watchdog 收束；准备
准入期限与清理宽限分别计量，60 秒不表示必须在该时刻强杀整个进程组。初始化期间沿用原覆盖策略与
进程所有权，只调用 SDK `initialize`，不创建或恢复 Session，不发 `session/prompt`。

```json
{"protocol":1,"type":"warm-ready","warmId":"warm-owned-123","binding":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","level":"sdk-initialized","dispatched":false}
```

`warm-ready` 只证明 SDK 初始化和路由配置接受，不是 Session 权限收据、模型完成或
计费证明。Go 等正式 invocation intent 已持久化、检查及预算允许后，才发送：

```json
{"type":"start","protocol":1,"warmId":"warm-owned-123","request":{"type":"start","protocol":2,"requestId":"formal-review","persistent":true,"sessionId":"existing-review","cwd":"/absolute/worktree","provider":"deepseek-account","model":"deepseek-flash","reasoningEffort":"high","permissionMode":"read-only","prompt":"评审通过确定性检查的候选","timeoutMs":1800000}}
```

嵌套请求遵循原 v1/v2 schema，但本模式明确拒绝新增 `maxTokens`，因为初始化未配置
该值；冷调用原有支持不变。绑定与 prepare 必须一致。同一个 SDK 只允许消费
一次。正式 start 后才选择或恢复 Session。worker 在自己的临时私有目录独占写入延迟
admission 元数据，公开 `agent/created` hook 非阻塞读取有界普通文件，核对 warm nonce
及实际 Session 身份后发布原权限收据；其中无 prompt 或凭据。权限、cwd、终态和 SDK
清理检查与冷调用相同，正式 ready/event/result 仍使用正式 Request ID 和原协议版本。
Go 从正式 `Run` 开始独立执行调用 idle/wall 上限，预热不会延长模型时限；结果
`durationMs` 不包含此前与验证重叠的等待，预热窗口另计。

不再需要时，Go 发送唯一的 `close`：

```json
{"type":"close","protocol":1,"warmId":"warm-owned-123"}
```

close、TTL、EOF、TERM/INT/HUP 均触发有界 shutdown。Go 发 close 后最多给外层 worker
三秒协议收尾窗口，再取消现有 watchdog 并等待其 TERM/KILL。SDK 未配置 `native` 时
正常关闭宽限为两秒；显式 native／LCU 分别为 15／135 秒，Go 外层分别至少 20／145 秒，
不等剩余模型 wall-time。尚未收到正式 start 的 worker
在 SDK 关闭后，将 `terminal.json` 以 0600、独占、原子发布到指定证据目录，fsync 文件及目录后再输出
`warm-result`。文件包含 protocol、warmId、binding、真实 workerPid、`dispatched:false`
和 `cleanupConfirmed`；协议损坏或 SDK 清理未知不得为 true。发布的完整 JSON 文件
最多 8192 字节，按序列化后的 UTF-8 计数，包含 JSON 转义与末尾换行。可选 `error`
诊断过长时仅截断该文本并标明截断，正常短错误完整保留；身份、派发和清理证明字段
不得删改。Go 保持相同大小上限与全部文件、身份和退出校验，诊断截断不构成清理证明。
只有无错误且确认清理
的正常 close 才返回 `status:closed`／退出 0，否则返回 error 或 cancelled 和非零退出。
正式 start 已接收则
不写未派发证明，恢复归属于正式 Session 调用账本。

Go 只在精确身份匹配、receipt 为有界私有普通文件、无重复终态或损坏传输、外层
watchdog 已证明静止后清除当前 warm intent。崩溃恢复重新 fsync 已核对的文件及目录，并额外要求该 worker PID 和同号进程组
（负 PID）的只读探测均明确为 ESRCH；leader 消失但孙进程仍存活、PID／进程组被复用
或探测错误均阻断，不自行杀死或重放。确知尚未启动
进程的 Go 路径可原子写 `noProcess:true,workerPid:0` 的独立本地事实。缺少收据、清理不明
或输入写入未知都不能仅凭等待时间自动冷启动。预热句柄不跨任务、角色、目录或引擎
进程复用，也不绕过硬预算或增加模型在飞数量。

确定性检查超过 TTL 时，已过期预热视为优化未命中：只有精确未派发收据、SDK 和整个
进程组清理已确认、正式 start 从未尝试发送、父上下文未取消且流完整时，才在同一个
已持久化 Request ID 内冷调用一次。该路径不消耗 transport retry、不虚增模型调用，
只使用从正式 Run 入口计算的剩余 wall／idle 时间；冷调用的清理由其自己的结果证明，
不能借旧预热收据掩盖。正式移交会原子退休 prepare timer，已排队的 TTL 回调也不能
取消正式调用。初始化错误、未知派发或不完整收据不会触发这一替代路径。

## Codex Pro worker协议

公开入口为 `@ckanner/flywheel/codex-pro-worker`，可执行入口为 `flywheel-codex-pro-worker`。`FLYWHEEL_CODEX_PRO_WORKER` 允许显式覆盖；npm 启动器解析同一安装包内的 worker，不依靠碰巧可用的全局旧 worker。worker 实际导入 bundle 的已审查源码包 `dsh-subagent-codex-pro@0.10.2` 的 `./machine` API，严格检查包名、版本、machine protocol 1 及 lifecycle/recovery capabilities；源码来源见 `vendor/README.md`，不宣称该版本已在 registry 发布。Node 最低 22.18.0，仅支持 macOS／Linux。

### 请求与父进程连接

```json
{"type":"start","protocol":1,"requestId":"call-123","cwd":"/absolute/worktree","model":"gpt-6.1-sol","reasoningEffort":"high","permissionMode":"workspace-write","prompt":"执行冻结工作单元","timeoutMs":1800000}
```

v1 还允许可选的 `idleTimeoutMs` 与 `sessionTitle`；不接受 provider、resume、threadMode、threadId、额外写目录、任意 CLI 参数或未知权限模式。requestId 非空且最多 256 字符；cwd 为绝对路径，Go 先规范化成物理目录。model／reasoningEffort／prompt 必须是非空文本，permissionMode 为 read-only／workspace-write／danger-full-access；最后一项在 Node adapter 边界映射为 machine API 的 `dangerously-bypass-approvals-and-sandbox`，Go 同时核对映射后的 requested 字段和 canonical 生效模式。timeoutMs 显式值必须为 1–2,147,483,647 的安全整数，省略使用上游 600,000 毫秒；worker 不接受通过 0 关闭期限。Go 传递有效 wall deadline 和 high 默认 thinking；worker 将 reasoningEffort 映射到 machine 的 effort。v1 固定 ephemeral，command 为 `FLYWHEEL_CODEX_BIN` 或 `codex`。

旧低层 scoped 入口使用外层 v2，增加必填 `threadMode:"persistent"` 和可选的精确 threadId：

```json
{"type":"start","protocol":2,"requestId":"call-125","cwd":"/absolute/worktree","model":"gpt-6.1-sol","reasoningEffort":"high","permissionMode":"workspace-write","prompt":"按当前契约返修","timeoutMs":1800000,"threadMode":"persistent","threadId":"existing-thread"}
```

v2 拒绝 ephemeral、缺失模式和空／非法句柄；所有 progress/result 回显外层版本。
嵌套 adapter、machine 请求与原始 receipt 仍为协议 1，精确包版本仍为 0.10.2。
v1/v2 不启用有 ACK 的生命周期；主任务使用下面的 v3 接通公开 lifecycle/recovery。

**这里与 DSH 旧单请求入口的 EOF 语义不同：Go 写入一行后保持 stdin 开放。** stdin 是父进程的存活连接；未完成前 EOF、读取错误或非法后续输入触发 AbortSignal，并等待插件有界清理。不能用 `echo … | worker` 的关闭 stdin 行为冒充正常调用。运行请求首行等待上限 10 秒；请求最多 1 MiB，Codex machine 单帧与保留结果各受 1 MiB 预算约束，原始累计流量不作为保留内存上限。序列化 progress 展示累计最多 1 MiB；到限只停止追加展示正文并输出一次省略说明，活动与终态继续。完整终态帧最多 8 MiB，native 与 durable lifecycle 另有独立边界；真实单帧／结果超限明确失败，不能裁掉回执后判成功。

### 进度与完整结果

零或多个进度帧：

```json
{"protocol":1,"type":"progress","requestId":"call-123","text":"插件报告的活动"}
```

最后恰好一个 result，字段结构如下，`receipt` 为插件实际完整对象：

```json
{"protocol":1,"type":"result","requestId":"call-123","adapter":{"package":"dsh-subagent-codex-pro","version":"0.10.2","protocol":1},"receipt":null,"error":{"code":"INVALID_REQUEST","message":"启动请求未通过校验"}}
```

启动前不能获得 machine receipt 时为 null。运行已开始则即使失败也保留完整 receipt；`error` 非 null、重复／丢失 result、尾部垃圾、请求身份或 adapter 版本不符都不能成功。本通道没有 DSH `ready` 帧；v3 使用有类型的 `thread-ready` lifecycle 与 Go ACK。不能从普通 progress 文本虚构沙箱或实际 effort 证据。

合法终态中的 `INVALID_ACTIVITY`、`INVALID_NATIVE_EVENT` 或 `OUTPUT_LIMIT` 表示 worker 拒绝了观察数据并等待同一次 adapter 调用清理，不等同于 JSONL／身份损坏。完整、独立核验通过的 receipt 仍可保留 session/run、usage 和 cleanup；业务 outcome 为 unknown，不能成为 resume checkpoint，内层为清理而产生的 cancelled/timedOut 不替代 worker 首因。坏帧、错误身份、缺失必需 durable lifecycle、ACK 失败及未知 worker 错误继续使对应证据不可信。这一边界也适用于 Claude worker。

Go 同时校验外层 worker 物理终态和插件 receipt：请求及 run/thread/turn 身份、模型、cwd、approval=never、实际沙箱模式、完整 sandboxPolicy、stopReason、cleanup 与执行事实。受限模式的 workspaceWrite 显式 writableRoots 必须处于工作区内，networkAccess 必须明确 false；显式完全权限要求 `sandbox:danger-full-access` 与精确的 `{type:"dangerFullAccess"}` policy，不要求不存在的受限模式字段；Codex 标准临时目录标记保留，不宣称只能写 cwd。主任务 v3 在 `thread-ready` ACK 前也核对这份完整策略，不符合时阻止 turn/start。旧协议或未请求 lifecycle 的辅助调用仍只在返回后完整检查；这些路径不能宣称预执行完整策略 guard。

effort 的证据只可为 turn/start 请求接受，保留 `effectiveEffort:null`。usage 使用 `run` 增量，不能把 threadTotal 当成本次消耗；只有 complete 才记入可知预算，partial/unknown 原样存入 receipt 并增加未知调用计数。cached input／reasoning output 是总 input／output 的子项，不重复累加。订阅 costUSD 保持 null、costStatus 保持 unknown。receipt 没有实际 provider 时保持未知，不根据 channel 发明 provider。

Go 由可信 receipt 生成 `codex-pro/0.10.2/worker-v1`、`worker-v2` 或 `worker-v3` 的 SessionOutcome。
只有正常 completed、cleanup 已确认且 persistent 的结果才具备 resumeCandidate；
这是适配器证据支持的尝试资格，不是 rollout 已持久化的证明。显式恢复必须接受同一
thread，requested／outcome／execution 的 thread 身份和 thread/start、thread/resume
来源均须一致。缺失／坏 receipt、流错误或 worker 异常保持 unknown。只有可信的
request／initialize／新 thread-start 等派发前阶段可证明 not-dispatched；thread-resume
拒绝仍未知，不从 busy／no-rollout 文本猜测安全冷启动。

### 共有的活动与名称扩展

Codex Pro 0.10.2 和 Claude Code Pro 0.6.0 必须公开 `MACHINE_ACTIVITY_VERSION=1` 与
`MACHINE_ACTIVITY_CAPABILITIES={version:1,metadataOnly:true,coalesced:true,rootAndChildren:true}`。
外层请求的可选 `idleTimeoutMs` 是 1–2,147,483,647 正整数；省略按 300000 计算
观察合并间隔 `min(1000, max(1, floor(idleTimeoutMs/4)))`，不改变有效 wall 预算。
公开 API 通过 `onActivity` 上报；worker 再核对字段集合、来源、递增序号与事件类型。

```json
{"protocol":3,"type":"activity","requestId":"call-123","adapter":{"package":"dsh-subagent-codex-pro","version":"0.10.2","protocol":1},"activity":{"version":1,"sequence":1,"kind":"provider-progress","source":"codex-pro","scope":"child","eventTypes":["item/reasoning/textDelta"],"observedEvents":3}}
```

`source` 为 codex-pro 或 claude-code-pro，`scope` 为 root/child/mixed，类型集合有界
且只能来自相应 adapter allowlist；counts/sequence 是正安全整数。不得加入原始文本、
思考、工具参数或结果、session/thread ID。实际事件驱动限频，背压时省略 advisory
发送；没有定时心跳。根与真实 owned 后代都适用，不依赖用户是否显式设置 native。
Claude 的 partial、tool/task progress 独立于 native durable ledger 的状态去重。

活动不是派发、终态或清理证据，协议 3 不给它 ACK，也不推进状态机。Go 实时检查后
在捕获流中仅保留有界活动摘要，防止长期活动填满结果缓冲；原始 OnEvent 仍收到实时
事件。坏帧、重复序号、身份错配、终态后帧与非空 EOF 残帧不能因合并而被洗白。

`sessionTitle` 为非空单行、最多 200 个 Unicode 字符、不含控制／格式控制的标签。
新 persistent 历史映射为 Codex 的 `threadName` 或 Claude 的 `sessionName`，由各自
公开 API 使用原生命名能力。resume 和 ephemeral 调用不设置标题，不覆盖用户重命名。
独立 Claude interactive-team 可以接收同一明确标题。Go 默认从任务 title/brief/slug
与 role/scope 构造；provider ID 和 session ID 不代替展示名称。

外层超时引发的 child cancel 保留原始 receipt，但顶层继续报告 TIMEOUT 与 idle/wall
来源。真实用户 context cancel 仍为 Cancelled。清理确认与业务 outcome 分开判断：
可信物理退出不能凭空证明模型完成，也不能授权未知调用的重试。

### 取消与进程终态

worker 捕获 SIGINT／SIGTERM／SIGHUP，先取消并等待插件返回清理结果，再分别退出 130／143／129；父连接断开为失败。Go 为 Codex Pro worker 保留至少 10 秒终止宽限，全局 kill switch 不提前缩短该子进程的宽限。插件 guardian 拥有 app-server 进程组；worker 意外死亡由插件自己的连接与 guardian 负责终止其拥有的进程。Go 未收到完整回执时不能把内部清理记为已确认，主动脱离拥有进程组的外部程序不属于此保证。

已启动 worker 后缺少可信 `cleanup.confirmed` 会记录 `CleanupError`，即使外层进程已退出。引擎和建议模块保存诊断后停止本次运行，不执行传输重试、渠道降级、Plan 冷启动或下一任务。完整回执中确认清理的普通服务错误仍使用原重试策略。恢复前先确认该次调用遗留的进程已停止；等待固定秒数不构成清理确认。

正常完成后 app-server 仍可能需要 TERM/KILL 收尾。只有 completed、cleanup.confirmed、无取消／超时／截断／失败事实且外层 worker 正常退出时，才允许 receipt 中正常 terminationRequested 导致的信号或未知 Codex exit code。不能将 supervisor exit code 冒充模型进程的物理退出码，也不能把文本答案当终态证明。

### 无推理诊断

`flywheel-codex-pro-worker --doctor` 不读取 stdin，不调用 runMachine，不启动 app-server，仅导入并核验公开接口。成功返回单个 JSON：

```json
{"protocol":1,"type":"doctor","package":"dsh-subagent-codex-pro","version":"0.10.2","machineProtocol":1,"available":true,"inferenceTested":false}
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
{"type":"lifecycle","protocol":3,"requestId":"call-126","adapter":{"package":"dsh-subagent-codex-pro","version":"0.10.2","protocol":1},"lifecycle":{"lifecycleVersion":1,"type":"lifecycle","requestId":"call-126","runId":"native-run","sequence":2,"event":"dispatch-intent","threadId":"thread-id","turnId":null,"persistent":true,"configuration":null,"dispatch":"not-dispatched","outcome":"unknown","cleanupConfirmed":false}}
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

`original` 仅为旧调用的非秘密绑定，不含 prompt、凭证或环境。Go 在原物理 `cwd`
启动只读 worker，保留相对 worker 路径及 wrapper 的目录语义；reader 不重新选择
工作区或认证，不启动 CLI 或调用 thread/resume。worker 只调用公开
`recoverMachine({protocol:1,requestId,runId,recoveryDirectory})`，返回：

```json
{"type":"recovery","protocol":3,"requestId":"call-126","adapter":{"package":"dsh-subagent-codex-pro","version":"0.10.2","protocol":1},"recovery":{"protocol":1,"type":"recovery","requestId":"call-126","runId":"native-run","status":"pending","receipt":null,"lifecycle":null,"reason":"No durable terminal receipt is available"},"error":null}
```

status 为 terminal/pending/unknown。只有 exact invocation 的完整 terminal 可继续核验；pending 不证明 process alive，错误不附带安全重放的 admission 证明。只读路径可重复且不产生新的模型调用。Go 还核对原 adapter 版本与 checkpoint 摘要；升级后不拿新版本解释不兼容的旧 journal。

cleanup-confirmed 的 Go ACK 前保存原候选、role guard 和 Execute report；恢复到 receipt 不自动获得该证明。缺少它时可信用量可对账，业务保持阻断。状态、metrics 与候选消费语义见[恢复指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/session-recovery.md)。

## Claude Code Pro worker

`flywheel-claude-code-pro-worker` 与 Codex 共用父 stdin lifeline、单请求、大小上限、
信号转发与等待收尾的 transport，适配器及回执保持独立。optional peer 精确为
`dsh-subagent-claude-code-pro@0.6.0`，公开导出 `MACHINE_PROTOCOL_VERSION=1`、
`parseMachineRequest`、`MachineInputError`、`runMachine`、`recoverMachine` 和 lifecycle version/capabilities。缺 peer 不影响未选择 Claude
的流程；选中时在 machine 调用前报错，绝不改调 DSH、Codex 或另一版 CLI wrapper。

0.6.0 普通调用通过官方 Agent SDK 0.3.326 的 `query()`，由既有 guardian 的公开
spawn hook 启动 `FLYWHEEL_CLAUDE_BIN` 指定的本地程序。SDK 负责请求／控制协议，
插件保留逐帧 UTF-8／JSON／大小校验、真实活动、结果身份、物理退出和进程组清理。
SDK 迭代结束不证明清理完成。machine／lifecycle version 仍为 1，真实原生
`system/init` 仍标记 `configuration.source:cli-init`。原生 Review 与交互 Team 保留
专用 CLI；普通调用失败不回退到旧 print runner。

start 字段与 Codex 外层 v1/v2/v3 一致：type、protocol、requestId、cwd、model、
reasoningEffort、permissionMode、prompt、timeoutMs，以及可选 idleTimeoutMs/sessionTitle；v2 额外允许
`threadMode:persistent` 和可选 threadId。v1 映射原生 sessionMode=ephemeral，v2
映射 persistent／显式 sessionId。主任务 v3 额外使用上面的 durable ACK/recovery；只接受 danger-full-access，映射 bypassPermissions；
原生审批模式不等价于 OS read-only/workspace-write。命令来自
FLYWHEEL_CLAUDE_BIN，权限、session、命令和输出上限不能通过额外 start 字段覆盖。

Claude query 单帧上限为 16 MiB，最终答案按请求的 1 MiB 保留预算核验，图片数量／字节另有有界上限；中间文本只保留展示 tail，不按累计 raw 流量终止任务。

Claude query 在发送业务 prompt 前，通过受管、无推理的 `claude --help` 检查
`--include-partial-messages`、`--forward-subagent-text` 与新会话命名所需的 `--name`。
实际 flags 支持是准入依据；forward-subagent-text 由上游 2.1.211 引入，嵌套/特定
工具的实时行为仍依赖所装 CLI 的具体实现。检查和正式调用共用原 wall 预算，缺失
能力不自动试发 prompt，也不切换后端。短暂 partial 事件不进入最终答案文本。
SDK 同一进程的 initialize 响应还须提供稳定的 `claude_code_version >= 2.1.325`，
之后才放行用户任务。这是最低已验证版本；缺失、过旧或预发布版拒绝。该拒绝保留
已经观察到的原生 PID／dispatch 与独立 cleanup，当前契约保守记录 unknown／possible，
不因尚未发送 prompt 而伪造进程从未启动。新会话标题、Agent 定义与子级文本转发
通过 SDK initialize 发送；根 effort、权限与显式持久化通过 SDK 正式选项传递。

native v1 的 `agents.definitions.<name>.effort` 可选，枚举为
low/medium/high/xhigh/max。Go 配置 schema 在加载时检查，worker 将 native 对象完整
交给公开 parser，插件原样传入 SDK `agents`（交互 Team 使用 `--agents`）；省略时不注入子 effort。父调用的
reasoningEffort 仍独立映射到 `--effort`。此扩展不升级 machine/native protocol，
但需要精确 0.6.0；0.5.0 的 parser 不接受该字段，不能改写包身份放行。回执中的
requested 证明请求参数，tools-only effective 不证明子模型实际 effort。

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
`{protocol:1,type:"doctor",package:"dsh-subagent-claude-code-pro",version:"0.6.0",machineProtocol:1,available:true,inferenceTested:false}`。
不启动 Claude，不测试账号。完整用户入口见[中文](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/claude-code-pro.md)／
[English](https://github.com/ckanner/flywheel/blob/main/docs/en/claude-code-pro.md)。

## Go 角色账本与回执恢复

`Result.Session` 是 Go 的标准化调用事实，不是给所有旧 wire 格式新增字段。
Codex Pro、Claude Code Pro、DSH、Kimi 的主任务 Plan／Execute／Review／Judge 在 fresh/scoped 下都由
Go 保存 intent，再执行一次调用并写入限定大小的原子回执。`last.persistent` 记录该调用原生持久化请求，
与 lifecycle 和只读 recovery 精确匹配；受管复用还必须满足保存的角色策略与原有证据检查。
Codex／Claude Pro 的 fresh 也请求 persistent，但每次省略旧 ID；显式 `configuration.sessionMode:fresh`
不授予 resume-candidate。DSH／Kimi 保持原有保存方式。两种策略的匹配原生 ID 均保留作展示。
旧调用按原 persistent 值恢复，缺少 configuration 的旧记录不重新解释。既有
Kimi Plan resident 选择保留。Intake／Garden 不进入角色账本，Pro 每次保留独立具名历史但不续用返回的 ID；
其他适配器及 mock 保持原路径。

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
| `planning` | 必需 taskDir／cwd；stdin 接收版本化规划命令，返回 state.planning 与 planning-control 回执，见下节 |
| `launch` | 必需 taskDir／cwd；可选 `--mode`（auto、quick、full），缺省 auto；校验已准备契约并启动 |
| `start` | 必需 taskDir／cwd；可选 `--mode`，省略时保留契约或持久化策略；旧任务仍 Full |
| `config` | 必需 cwd，可省 taskDir；返回 `configuration` |
| `doctor` | 必需 cwd，可省 taskDir；可选 `--probe`；返回 `diagnostic` |
| `session` | 必需 taskDir／cwd；stdin 接收 revision-bound session 命令；可选 `abandon` 只用于规划角色未知调用，要求人工证明；返回 `session-control` |

共同 envelope 为 `protocol:1, ok, running, pid, taskDir, state`，可选 `cwd`、`error`、`taskTitle`、`session-control`。`taskTitle` 仅来自通过契约校验的 Spec.title，是显示信息，不作为任务或会话身份；缺失或无效 Spec 时省略，仍保留状态／停止回执。成功解析目录后 `cwd` 是请求目录的规范化物理路径，供宿主关联校验；旧回复未带该字段仍按原任务身份机制处理。省略任务时 `taskDir:""`；配置／诊断不启动或修改任务，通常 `running:false, pid:0, state:null`，不能用这些字段覆盖面板已经显示的任务运行状态。doctor 可只读已有工作流状态以判断必需阶段。

工作流事实来自 Spec，最终策略由 Go 保存到 `state.workflow`：version、requested、effective、reason、evidence、risks、selected-at、stages 和 upgrades。`stages.plan/judge` 各有 applicable 与 reason，不使用伪造 pass。launch 默认 Auto 只是请求，不是承诺一定 Quick；显式 Quick 也必须通过资格校验。未知版本或非法模式在模型启动前失败。已有策略恢复时不重分类，Full 不降为 Quick；upgrade 先持久化再进入 Full。status／stop／config／doctor 不接受 `--mode`。

`configuration` 含 source、scope、cwd、taskDir、sessions、bindings 和 jev。jev 使用统一配置字段，enabled 是唯一总开关；它表示配置状态，不代表已请求或采用。sessions 含 mode／policy-version、可选 roles 与 quality 配置；每个 binding 包括 id、role、route、profile、bindingSource、harness、provider、channel、model、thinking、residentSession、permissionMode、inScope 及 session。session 含 mode、supported、reason、historyRecoveryTested:false，仅报告选择与适配器能力，不证明具体历史存在。账号渠道与实际 adapter 分开；未给出的旧配置值不得由 UI 猜成已验证。

有会话记录的状态快照另含紧凑 `sessions` 数组：role、scope、generation、action、reason、
resumeCandidate，以及 last 的 status、persistent、dispatch、outcome、cleanupConfirmed、
usageStatus、accounted。投影只读取已校验的 state，不打开后端历史或原始回执；旧的
完整 state 字段继续保留。状态查询不会核销回执、修改计量或解除未知调用的阻断。

`session-control` 为异步 session 命令的语义回执：key、action、applied、revision、
status、invocation、message。它不改变共同 envelope 的 `ok` 含义；`ok:true` 只说明
控制请求完成，`applied:false` 表示本次尚未确认对应动作生效，须结合 message、running
和 status 区分拒绝、未变化、仍在处理及无法核实效果。缺少操作前的 invocation
证据时，保留 after-state 并明确提示命令可能已执行；`applied:false` 不是未执行证明，
下一次修改前须刷新状态。`session abandon`
仅接受规划角色、已计量且清理确认的 blocked 调用；host 证明绑定 taskDir、key、
revision、reason 和准确 invocation，并要求 `FLYWHEEL_SESSION_USER_ACTION` 与
`DSH_SESSION_ID` 精确匹配；模型工具不暴露该动作。人类 CLI 走控制终端的
`abandon <invocation>` 确认。Host 先核对原始用户命令，再将证明和引擎请求同时绑定
规范化任务路径，允许同一目录的符号链接和尾斜线，仍拒绝跨任务重放。Reconcile 对
同 key、thread、invocation 的 pending 接管 true→false 转换报告 applied，即使回执
已在派发前取消路径中 consumed；重复核对不算新动作。

`diagnostic` 含 kind=`doctor`、ok、cwd、taskDir、probeRequested、inferenceTested=`false`、可选 configuration，以及 checks 数组。check 为 id/status/summary，可附 code/remedy；status 只允许 pass/fail/warn/unknown/skipped。配置本身无效时允许没有 configuration，仍返回完整失败检查项。

合法检查失败同时满足 exit 1、envelope.ok=false、diagnostic.ok=false；控制层保留并展示报告。其他非零退出、超时、信号、坏 JSON、错误 protocol 或任务身份仍是传输错误。普通 config/doctor 不接受 shell 参数拼接，`--probe` 不能用于其他 action。Go 检查预算 40 秒，TERM 升级、stdio 排空、进程组确认各有 5 秒宽限，保守另留 15 秒；桌面默认控制窗口 60 秒。

config/doctor 的 Go CLI 将 SIGTERM/SIGINT 转为 context 取消，等待 watchdog 收尾后才退出。控制调用取消或超过窗口时，host 先发 SIGTERM，诊断动作再留 20 秒让 Go 完成分阶段清理，仍未退出才 SIGKILL；取消始终报告控制失败，即使迟到结果成功也不覆盖它。launch/start/status/stop 保持原控制进程的 500ms 收尾，后台引擎停止仍走独立 stop 协议。

### Reviewed planning 控制与可信批准

新增 `control planning --task-dir <绝对目录> --cwd <绝对目录> --compact`。stdin 是一个
JSON 对象，host 外层工具/command 请求为 `{action:"planning",taskDir,cwd,planning:{…}}`。
未知字段拒绝，不接受模型自报 actor、source 或 approval proof。

| 内层 action | 字段与含义 |
|---|---|
| prepare | brief、可选 request-id/start；brief 服从 planning.schema.json 的 $defs.brief，缺省启动设计；start:false 只登记 |
| plan | revision、request-id、scope、可选 scope-id/text/start；text 是该 request 不可变的局部规划说明，供 planner/reviewer 使用，不增加全局 input-revision；design / milestone / packet / replan；显式 Plan 运行 planning-only，已有启动授权也不顺带执行 |
| feedback | revision、request-id、text，可选 design/policy/start；输入绑定观测版本，运行中的 owner 可排队采用 |
| approve | revision、request-id、精确 design:{revision,digest}、可选 start；仅可信用户来源，start 缺省 false |
| status | 只读规划状态，可选 request-id 查询排队请求的已核对终态；不接受变更字段 |
| migrate | revision、request-id、可选 policy/text/start；明确迁移已有 legacy 任务，不能由升级隐式触发 |

响应继续 protocol 1，包含原 `state.revision` 与 `state.planning`。可选
`planning-control:{request-id,action,status,revision,message}` 的 status 是
queued / applied / rejected / unchanged。queued 不修改已采用 feedback，也不代表审查、
批准或启动；状态快照和控制回执分别解释。当前/上一设计正文、当前scope子计划与近期
审查按响应上限投影；历史引用仍用于追踪，不将省略正文当作空设计。

只读投影可带 `state.planning.projection:{kind:"console-excerpt",approval-complete,ledger:"state.json",omitted:{类别:数量}}`。
权威 planning schema 不接受该显示标记，不能回写投影。省略当前完整设计或 review 时，
`approval-complete:false` 禁用 UI 批准；不能把截断正文当完整已审方案。无 `--compact`
的 CLI 状态保留完整记录；交互式 CLI 批准展示权威设计与审查。

`scope=replan` 是重新规划的意图，不新建审查范围：无 id 或 root 归为 design，
M-01 这类 id 归为 milestone，M01-T01 归为 packet；沿用该范围的版本和 findings。
控制台已有 queued 回执时，刷新调用 `planning/status` 加原 request-id；同一个只读
Remote 核对关联任务，不追加会话命令事件，也不启动规划或实施。

模型工具 `flywheel_control` 的 planning action 不列出 approve，并在执行边界再次拒绝。
人工 `/flywheel` command 使用公开 `sessionProjections` 的 `flywheelPlanningUser` 投影，
从本 Session 自有 command/run 的 `source.kind:user`、commandId 和原始 args 核对
精确输入。继承的 fork 事件、已 command/done 消费的事件、重复 commandId、模型来源或
不匹配 args 不产生批准凭据。投影不新增 Session 事件，不同步扫描旧事件日志。

仅核对成功后，host 为本次子进程注入 `FLYWHEEL_PLANNING_USER_ACTION` JSON：
`{kind:"dsh",sessionId,commandId,"request-id",revision,design,start}`。
runControl 始终剥离父环境继承的同名变量，只有独立 host option 可设置；用户工具 JSON
不能提供该 option。Go 必须精确比对 request/ref/revision/start，记录 dsh 用户来源。
普通 model worker 环境不传播该凭据。无 host 用户凭据时，CLI 人工批准独立打开
交互终端显示确切版本并要求输入 `approve <digest>`，不从模型 JSON 接受 actor。

approve-only 不获得 StartAuthorized；之后普通 Run 不补权限。已经批准的根方案可通过
一次新的真实用户 approve(start:true) 动作授权开始，不需要重复一次设计讨论。
根批准可以先于任何子计划；Plan Review、scope 校验、冻结验收和激活事务由 Go 管理。
`pending-activation`、过期设计或未知调用仍阻断实施，不由 UI 刷新消除。

### DSH 启动工具与任务关联

`flywheel_launch({taskDir,cwd?,mode?})` 使用 action=launch，mode 缺省 auto。
`flywheel_control({action,taskDir?,cwd?,mode?,probe?})` 与人工 `/flywheel` JSON 命令
使用同一校验与控制层；mode 仅限 launch/start，probe 仅限 doctor。
省略 cwd 时采用当前会话公开 workingDirectory 服务，跨 worktree 时显式传入。
若 `header.cwd` 和 `working-directory/change` 均未记录当前目录，launch/start/stop 必须显式传入绝对 cwd；宿主在派发 Go 前返回 `not-dispatched`，即使 getter 有外部默认目录也不据此猜测关联，status 仍可使用 getter。
旧 launch/start 消费已准备契约；新 `control planning prepare` 接收宿主提供的薄 brief，由 Go 负责设计与独立审查，宿主不提前生成完整设计。

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

异步控制层另有插件自有 observable，快照在未变化时保持同一身份，经 `inject.hooks` 交给宿主绑定；React 组件不自行订阅，不读取 ctx 或原始服务对象。会话关联与控制快照只是查看缓存，不成为任务账本，也不写入新的 DSH 会话事件。模型可见的命令和工具结果仍由宿主原有记录机制处理。此查看契约不改变 Go control 协议、worker JSONL 或后台任务的停止语义。

### 会话控制台的静默读取

客户端参数与结果使用公开 `strict` codec（`typeSymbol`、`create().parse`）；结果还提供同一校验的 `decode`。Agent lookup 使用宿主规定的 wire 字段 `agentId`，其值仍为当前 Session ID；不能自行改名为 `sessionId`。Session ID 必须为非空字符串，请求只允许具名读取动作及相应字段，返回值保持 `{ result: { kind, text } }`。严格 codec 不替代 Host 的 Session／任务归属检查及 Go 控制验证；客户端仍只导入共享 React。`$mount` 成功后，通过父插件拥有的 `ctx.inject(['remote.flywheelConsole'])` 子作用域访问动态 namespace，随父插件撤销；初始外层 context 不直接访问未注入的服务。

`flywheelConsole/read` 是公开 Typert Remote，参数为宿主解析的 `agent`、控制请求 `input`
和取消信号 `signal`。客户端显式挂载描述符，只有 React 是运行时共享模块。
宿主只接受 runs/status/config/config-edit/metrics/history/node/delivery/doctor；其他 action
在派发前拒绝。runs 仅返回当前会话的 hostSessionId；带任务的查询必须匹配当前投影或
精确会话归属的运行记录，taskless 配置限定当前会话目录。返回 CommandResult 形状的
`result`，复用成功 JSON 与结构化失败验证，但不经过 Commands execute、不追加会话事件。

UI 写操作仍经过 `/flywheel`；结果未知时的显式 status 也保留日志以解除持久投影的锁。
投影 stateVersion=3 会重放日志，排除旧版 UI status 对无关联或其他任务的绑定，工具的
显式 status 仍可关联准备任务。fork 仍忽略继承的任务事件。旧聊天记录不删除。

## npm 安装报告与桌面入口

以下是 Node 安装器的独立契约，不与 worker JSONL、Go control envelope 或任务 `state.json` 混用。入口为 `npx -y @ckanner/flywheel@latest install` 或已安装包的 `flywheel setup`；二者共用参数与行为。普通 npm 包安装没有修改用户目录的 postinstall，只有显式 install／setup 执行 skills、安装记录和 DSH profile 配置。

### 安装结果

加 `--json` 时 stdout 只包含一个 JSON 文档；安装中的外部命令输出由安装器捕获，不混入此文档。正常进入安装流程后的字段如下：

| 字段 | 含义 |
|---|---|
| `protocol` | 固定为 `1` |
| `ok` | 全部请求的软件安装步骤和最终版本核对是否完成；不是账号或推理成功 |
| `version` | 本次安装器的精确包版本，决定 CLI、内置引擎、worker 与插件版本 |
| `steps` | 顺序记录 `global-cli`、`skills`、`desktop-launcher`、`dsh-plugin`、`preflight`；失败时只含已开始的步骤 |
| `ready` | 本地 doctor 是否无阻断项；诊断未完成时为 `false` |
| `authentication` | 固定 `unknown`，没有自动登录或真实模型请求 |
| `prefix`、`paths` | 得到后才记录；paths 包含 CLI、原生二进制、DSH worker、codexProWorker 与生成的 launcher 的绝对路径 |
| `adapter` | 实际导入通过后记录 Codex Pro 包名、精确 0.10.2、protocol 1 与 inferenceTested=false；不代表账号已就绪 |
| `skills` | 安装成功时的数量及准确目录；每项结果保存在 skills 步骤的 `summary` |
| `diagnostic` | 合法的完整本地 doctor 报告，包括失败检查项；不执行 `--probe` |
| `error`、`cancelledBy` | 失败原因、可选取消信号；不是成功结果的一部分 |

每个 step 具有 `name` 和 `status`，结束时为 `ok`、`warning`、`skipped` 或 `failed`，按步骤附带 `error`、`reason`、`version`、`summary`、`path` 或 `profile`。成功的桌面插件步骤包含 `reloadRequired:true`；宿主仍需按自身流程重新载入。参数解析失败也返回 protocol 1 的单个失败文档，但尚无 `ready`、`authentication` 等运行结果。

缺少 Kimi／Codex CLI 或配置未准备好时，合法 doctor 失败报告使安装结果成为 **`ok:true, ready:false`**，preflight 步骤为 warning，安装退出码仍为 0。doctor 必须满足退出码 0／1 与 `diagnostic.ok` 一致；坏 JSON、超时、进程信号或其他退出码都使安装失败，不能伪装成“工具尚未准备好”。

安装失败退出码为 1；SIGINT／SIGTERM／SIGHUP 取消按 `128 + signal` 返回，分别为 130／143／129。外部安装步骤有独立时限与进程组收尾窗口，取消不因迟到的成功结果转绿。已完成的 npm／DSH／skills 步骤不自动整体回退，调用者应保留报告、修复具体原因并重跑。

`--prefix`、`--project`、`--dsh-bin`、`--profile`、`--no-dsh`、`--no-global`、`--timeout-ms`、`--json` 的完整语义见[教程](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/getting-started.md#2-安装)。`--no-global` 指当前已经持久安装的包；默认 install 与 setup 都会确保本次版本的全局安装。`--no-dsh` 仅显式跳过插件注册，不把可能失败的本地 doctor 当成已通过。

### GUI 安装记录

安装器生成 `~/.config/flywheel/installation.json`（0600）与 `npm-launcher`（0755）。记录包含 `protocol:1`、`owner:"@ckanner/flywheel"`、`version`、`prefix`、`cli`、`worker`、`codexProWorker`、`adapter`、`node` 和 `launcher:{path,sha256}`。它只描述软件入口，不保存账号凭证，不构成第二份任务状态。旧记录仍按原字段读取，升级 setup 生成新字段；用户修改的 launcher 继续受保护。

launcher 使用绝对 Node 与 CLI 路径，设置安装时可见的绝对工具 PATH 目录，并在用户没有显式设置 `FLYWHEEL_DSH_BIN` 时使用已核对的桌面命令。npm CLI 启动器解析版本一致的原生包和 worker，缺失或版本不匹配时拒绝继续，不隐式下载另一份二进制。临时 npx 缓存与 `node_modules/.bin` 不被记录成持久工具 PATH。

桌面插件按以下顺序选择入口：

1. 用户显式 `flywheelBinary`，必须是非空字符串；它优先于安装记录。
2. 存在安装记录时，验证其协议、owner 和绝对 launcher 路径，再验证 launcher 是普通非链接的可执行文件且 SHA-256 匹配。已安装完整包还要求记录版本与自身版本一致；不一致时运行该版本的 setup 刷新，不能混用旧引擎。
3. 安装记录不存在时，使用完整包内的 `bin/flywheel.js`；宿主必须能找到 Node。仅 private 源码测试区可使用旧 PATH 查找。

损坏记录、符号链接、缺失或被修改的 launcher 都明确失败，不静默退回 PATH。SHA-256 核对的是 launcher 内容，不是账号状态或所有外部 CLI 的真实性。已有用户修改会在 setup 时保留并报告冲突；修复方法见[升级教程](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/getting-started.md#8-升级排错与卸载)。普通 npm 卸载不会擅自删除安装记录、配置、skills 或任务数据。

## DSH 宿主兼容性（0.28.4）

包声明采用 `>=0.2.1-rc.5 <0.3.0-0` 的兼容系列范围，DSH 的范围判断包含预发布版本。0.2 系列后续 rc／补丁不需要逐版本追加白名单；0.3 系列及其预发布版保持拒绝。版本范围只决定准入，不代替以下公开接口与权限／清理证明。新 SDK 会按策略文件所属包的 engines.dsh 校验其准入；策略插件未加载就没有本次私有权限回执，worker 必须拒绝派发模型 prompt。权限插件在公开 agent/created 阶段（创建公告结束后、首个模型请求之前）写入显式 approval/policy，不在 session/created 只读公告中追加事件，再只读公开 approval.overrideOf(session)，不访问私有 effectivePolicy。证明通过本次 nonce/request/session 关联的私有文件返回，不追加自定义持久事件；未知／缺失回执仍不放行。

当前持久化 create／新进程 resume 的公开 SDK 验证锁定 `0.2.2-rc.3` SDK、JSONL persistence
与 AgentLoop。`0.2.1-rc.5` 的 Host／UI／插件兼容测试不代表历史恢复已验证。无真实
账号、模型或 Mac 实机的测试不能证明这些环境通过，也不构成性能／缓存收益实验。

## Console extensions

Protocol 1 adds command-only `config-edit`, `config-save` and `step`. Existing model tools retain their original action schema.

- Config actions accept `editor` in the host command, serialized to Go stdin as at most 1 MiB JSON. Its fields are `scope` (global/project), `revision`, `values`, and `reset`. Go disallows unknown request fields. Config-edit returns target, scope, source, revision, editable values, and embedded defaults in `editor`. Config-save requires the revision and returns the reread document with applied=true. An error after publication remains an error and requires rereading. Config documents contain credential environment-variable names only.
- Step accepts only=plan/execute/review/verify, a positive state revision and an optional packet ID. Host translation uses literal argv: `control step --only ... --expect-revision ... --packet ...`. The detached engine receives `run --only` and checks the revision inside its task lease. Completed or out-of-order packet execution is rejected. Responses use the ordinary control state envelope; acknowledgement does not imply stage success.
- Task projections recognize step command intents/results, preserving unconfirmed mutation ownership across reload. Configuration writes use file revision checking and a separate UI unconfirmed-save state.

Step optionally accepts `retryReason` for Execute only, translated to `--retry-reason`. It must contain 1–1000 UTF-8 bytes. The engine requires a revision-bound request, persists a human-continue-once audit entry before dispatch and permits one business attempt without resetting failed-attempts.

## 控制台增量接口

控制层协议仍为 1。所有动作均由 Go 读取／写入账本，UI 不写 state.json。

| action | 额外输入 | 结果 |
|---|---|---|
| investigate | task-dir、cwd 与 stdin JSON：start/run/status/report/cancel 及所需 contract/revision | 由 Go 拥有的准备记录与有界调查结果 |
| runs | cwd | 最近 100 条登记，运行状态另查 status |
| metrics | task-dir，可选 run-id | 对账后的分组与有界调用明细；未知／部分数据不置零 |
| node | task-dir、packet | 当前 packet 完整证据及相同 revision 的状态 |
| history | task-dir、session-key | 摘要绑定的最近回复，不等同完整后端历史 |
| session | stdin JSON：action、key、revision；claim 的 reason 或 send 的 prompt | 受管 claim/send/reconcile/return/release；后台结果写入 control 回执 |
| delivery | stdin JSON：action=inspect，可选 headBranch | 仓库、当前 HEAD、PR 与检查状态 |
| deliver | stdin JSON：create/merge、revision、expectedHead、expectedRepo、confirmed=true、base 等 | 明确授权的远端动作与回执 |

调查响应的 `preparation` 保持 version 1。可选 `jev-status` 是最近一次准备／运行
记录的有效状态（enabled、disabled、config-unavailable、not-configured、policy-changed），UI 不从
冻结的 contract.decision 猜测当前总开关。observations[].invocation.adopted 标记
实际采用的取证动作；decisions 保留调用／跳过与原因。可选 `decision-effects` 保存
已采用的 defer／finish：decision-id、action-id、kind、actions、evidence、observations、
binding-hash、candidate-hash、at。其中 actions 是跳过的原动作 ID，evidence 是已成功
观察的 invocation ID。finish 只进入 awaiting-report，不产生报告或代码完成证据。
UI 等待报告时禁用继续取证，仍允许停止和状态查询；所有决策与状态变更由 Go 执行。

调查 action 新增 `run_command` 和 `http_read`。`command` 冻结 argv、精确 inputs、
env 变量名、timeout-ms、expect-exit 与 `execution-boundary:host-process`；HTTP
冻结精确 url、GET／HEAD、headers-env 变量名、timeout-ms、max-bytes 和 expect-status。
这些输入继续放在 bounded stdin 中，不能拼接进 CLI argv。普通文件动作仍使用 path；
新动作不能因缺少 path 被客户端误判为不合法，完整契约资格由 Go schema 校验。

`observations[].execution` 是可选的实际运行结果：kind、可选 exit-code／http-status／
signal、timed-out、cancelled、可选 workspace／input-hash，以及 cleanup
（removed／retained／not-created）。UI 不把预期非零退出、HTTP 失败、取消或输出
截断显示成代码验收通过。副本隔离不宣称 OS 沙箱。unknown 实际动作回执不作为普通
observation 消费；Go 保留 pending 与 blocked，UI 只能核对原回执，不能另派动作。

实现状态的可选 `runtime-decisions` 版本为 1，汇总任务共享的 used、imported-calls、
max-calls 与最近观察状态。records 的显示字段为 id、packet、attempt、started-at、
action、status，可附 selection（action-id／adopted／reason）、去敏 receipt 摘要
及 feedback GateReport。`repair`、`replan`、`model-N`、`feedback-N`、`recheck-N`
由 Go 生成，客户端只显示；N 分别索引冻结的允许绑定、短反馈检查、失败 L1 检查。
applied 是实际应用的控制决策，completed 是控制动作收尾，两者都不代表任务验收成功。
feedback-completed 表示反馈已收尾；feedback-running／feedback-unknown 需要核对原执行，
feedback-stale 表示反馈改变了候选或验证输入并阻断流程。请求 receipt 的 unknown 与
stale 不能显示成成功采用。调用次数包括调查移交的已派发与未知请求。
replan-running 表示重规划未收尾，恢复须核对原 Plan 会话，不允许自动重复派发。
画布投影移除原始 snapshot／facts 和目标配置，最多保留末 10 条记录，
`omitted-records` 是更早省略的条数。feedback.results 的 `omitted-items` 标记也会明确展示。
实际模型切换来自 packet.execution-policy.history 的 from／to、before／after、kind、
reason、attempt 与 at；jev-model-selection 标记已采用的 JEV 模型选择。UI 不从 model-N
推测模型，不把配置变更覆盖成已启动的会话，节点模型仍来自实际会话或原始运行绑定。
节点详情可查看所属 packet 的记录；客户端对新增结构做类型校验，畸形回执保留上次
有效视图并要求刷新，禁止基于未确认状态直接 launch／start。

配置编辑只展示并保存 `jev.enabled` 一个总开关，保存本身是明确启用操作，没有第二个
影子／采用开关或逐任务确认。Go 规范化旧配置，旧 jev-shadow.enabled 不授予在线调用
或采用权限。UI 不保存密钥，只保存环境变量名；变更仍受文件 revision 与未确认保存
协议保护。

step 新增 integrate 和 final。阶段请求仍绑定 expect-revision；单节点操作不等同继续整个流程。

插件使用 `--compact` 获取画布摘要，响应标记 `projection:console-excerpt`。身份、revision、计数和裁决不变；诊断明细被明确标记省略，node 按需返回完整单元。原始账本不被裁剪。

session 请求经 `.control/session-request-*` 私有文件和内容摘要传给子进程，prompt 不进入 argv。人工接管期间普通 run/step 拒绝继续。结果未知或未消费时必须先核对原调用；release 只恢复暂停状态，不宣称验收通过。

会话账本存储 v2 是旧引擎拒绝接管状态的兼容性栅栏。session policy-version 1、DSH worker v2、Pro worker v3 与 lifecycle/recovery 版本保持各自独立。

远端写入绑定仓库、源 HEAD 和 base，检查项目记录与实际 fetch/push 目标。推送禁用隐式 tags、mirror 和 submodule 推送；合并不使用 admin 绕过。已创建／合并的结果先核对，已接收但未完成的合并请求保持 pending。GitHub CLI 的 [匹配 HEAD 语义](https://cli.github.com/manual/gh_pr_merge)与 [mergeStateStatus](https://docs.github.com/en/graphql/reference/pulls#mergestatestatus)为外部接口依据。

## Native capability extension（0.34.0）

本节是已实现的可选扩展，不替换既有 worker/lifecycle 版本。配置字段信源为
`templates/native-profiles.schema.json`，使用方法见[原生能力指南](https://github.com/ckanner/flywheel/blob/main/docs/zh-CN/native-capabilities.md)。

### 请求与所有权

DSH v1/v2、Pro v1/v2/v3 Start 可带 `native` version 1 options；Kimi native 是单独
protocol 1 worker。Go 从实际绑定的 native-profile 取得 options，在保存 invocation
intent 后传入 managed：

```json
{
  "version": 1,
  "runId": "run-001",
  "invocationId": "call-001",
  "role": "execute",
  "scope": "M01-T01",
  "candidateId": "candidate-digest",
  "channel": "deepseek-account-sdk",
  "taskDir": "/absolute/repo/docs/task"
}
```

`invocationId` 必须等于外层 requestId，taskDir 是绝对目录；candidateId 可省略，
空 runId 仅保留尚无 main-run 身份的既有调用语义。managed 不授予权限，不允许从
Start 注入 GUI lease，也不会被当成 model prompt 或 public Pro machine 字段。
DSH LCU 的 lease 由 worker 成功取得后内部加入。完整 native options 同时保存到
SessionConfiguration.nativeOptions；state schema 同步接受 nativeProfile/nativeOptions，
Pro recover.original.native 必须使用原调用内容。

### DSH composition 与实际准入

`native:{version:1,preset:"daily"|"heavy",agents?,team?,workflows?,web?,lcu?}`。
Team/Workflow 要求 agents.enabled。模型仍来自实际 Start provider/model/effort，
原生选项不改变账号。生成器固定 0.4.0，公开 `/composition` 产生 SDK global overlay；
回执不宣称 named preset 已被公开 SDK 挂载。

正式 prompt 前执行实际 TypeScript PTC probe，确认唯一模型工具 run_code、canonical
JSON/handles、多图与 deferred-context-v1 或 inspect-image-v1 输出。工具准入使用
当前 Agent/model 的真实可用集合。旧未配置 native 的 Start 保留原禁止 delegation
路径；显式 native 只开启对应受管树的工具，独立 goal/schedule/辅助标题与自动压缩
不因此开启。

本地 session.event/subagent 通知与公开 service 的 durable lifecycle journal共同
确定 owned children，包含外部 provider activation。journal 每行含 version、
journalSequence、requestId、nonce、rootSessionId、invocationId/runId，最多4096条／2MiB。
root turn-stopping等待flush，关闭回执包含：

```json
{
  "version": 1,
  "requestId": "call-001",
  "nonce": "owned-random-nonce",
  "cleanup": [],
  "lcu": null,
  "journal": {"complete": true, "events": 0, "bytes": 0}
}
```

bytes包含每行末尾换行。worker核对连续序号、每行身份与最终完整性，拒绝symlink／
非regular文件／超界与截断。journal不完整使调用失败并保持资源未知，不能只因SDK退出
而释放GUI。普通native SDK关闭宽限15秒，LCU135秒；Go外层至少20／145秒。

LCU使用0.5.0公开 managed API，PTC工具名lcu_cua，程序对象cua。跨进程local-desktop
lease限制同时操作整台桌面；每次操作检查真实caller和epoch。媒体从实际AttachmentStore
归档到task reports/native/invocation，保留captured和archive不同摘要，不能把图片存在
当作模型送达。原生runtime的目标权限和用户交互仍保持其真实约束。

### 观察、完成与清理

DSH native-event 使用外层 request/session 与 `event`；Pro native-event 使用严格
adapter 身份、原 managed 和 `native` 观察对象。所有事件只作观察，不是 lifecycle
ACK，不绕过 durable dispatch checkpoints。Kimi保留其本地server事件与连续订阅事实。

各 native terminal 保留以下共同字段，具体有效配置和用量策略由harness定义：

| 字段 | 必须保持的语义 |
| --- | --- |
| version | 1 |
| requested | 与原始 native options JSON 等价；默认值不回填原请求 |
| effective | matched及实际来源；DSH行为probe、Codex config/read、Claude tools-only等不同事实 |
| observed / children | bounded、去重、可归属的实际事件和子任务身份 |
| usage | 计量方法、完整性及不能直接相加的原始证据 |
| completion | confirmed与pendingIds，在cleanup之前冻结 |
| cleanup | 本地process group／owned SDK tree／remote job等真实作用域与确认 |

root完成但child仍在运行时completion=false；清理取消child不能修改已冻结的完成事实。
只有全部事实与外层进程终态相符才成功。未知用量保留null。Codex仅完整owned child
exclusive delta可加到root；Claude task snapshot不叠加；DSH external无计量、Kimi
compaction／旧child／Tower等保持未知。

### Kimi native worker

公开入口 `@ckanner/flywheel/kimi-native-worker`，可执行入口
`flywheel-kimi-native-worker`；Go覆盖变量 `FLYWHEEL_KIMI_NATIVE_WORKER`，CLI覆盖
`FLYWHEEL_KIMI_BIN`。账号环境使用真实 `KIMI_CODE_HOME`，不复制或更换授权。

Start protocol1带native/managed，permissionMode必须danger-full-access。worker
拥有loopback Kimi Server的PID/registry/serverID，验证HTTP/WS schemas与有效配置。
SessionOutcome.protocol为kimi-server/v1；ready/event/result仍经过本地stdio。原始
HTTP/WS内部端口不向外开放。首turn ordinal=0合法；fresh baseline=-1。

mode为agent/swarm/tower，agentProfile可选。无法强制的agents.maxConcurrent明确拒绝。
Tower独立Git候选只允许Execute在完成／清理与source及外部契约检查后导回；不能作为OS
沙箱，也不恢复旧Session。没有durable terminal的调用保持unknown，不声称支持Pro的
read-only journal recovery。详细字段、协议校验和fixtures见
[Kimi native protocol](KIMI_NATIVE_PROTOCOL.md)（[English](KIMI_NATIVE_PROTOCOL.en.md)）。

### 独立 native jobs

npm入口 `flywheel native <codex-review|claude-review|claude-team|claude-recover|claude-control>
--request /absolute/request.json`，使用实际固定public API加绝对recoveryDirectory。
输入为最多1MiB的regular NOFOLLOW JSON文件。正常返回一个原生JSON receipt，usage或
runtime错误保留既有公开结构；diagnostics到stderr。已取消返回130，失败／未知终态
返回非零；这不是Go role verdict协议。

Codex native review要求fresh ephemeral thread且不进入角色native-profiles，因为
review/start不消费角色prompt。Claude远端review要求显式consent，使用no-post；
本地进程退出不证明云端完成或取消。交互Team拥有独立tmuxserver/pane，控制及恢复按原
requestId/runId核对。恢复只使用已公开且可证明的事实，不重新派发未知副作用。

### 无推理诊断

DSH `--native-doctor`只import固定generator composition与可选LCU managed API，输出
nativeVersion1、实际依赖可用性、inferenceTested:false、admissionTested:false；不创建
SDK/Agent/lease。启用native的DSH binding不复用legacy SDK probe作为原生准入结论。
Kimi `--doctor`检查本地worker接口；Pro要求公开manifest nativeVersion1。Go doctor
按binding分别列native.manifest与native.admission，后者在实际worker调用前保持unknown。
