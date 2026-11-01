# Codex Pro adapter 加固：交给另一 session 的上下文

核对日期：2026-11-01。**本文是独立仓库的后续实施任务，不是“adapter 已修复／已接入 Flywheel”的完成报告。** 本 session 只读检查了该 adapter，并在 Flywheel 仓库保存这份交接文档。

## 1. 用户目标与明确范围

用户常用 DSH 的 Mac 桌面，希望组合 Kimi 订阅、ChatGPT 订阅与 DeepSeek Flash API，兼顾质量、成本、速度和可控性。Flywheel 继续由 Go 管理契约、状态、验证、worktree、重试、预算、提交及停止；模型不接管这些决定。

用户将 `ckanner/dsh-subagent-codex-pro` 的加固安排在**另一个 session、另一个仓库**。本任务仅加固这个 adapter 已有的 Codex app-server 委派通路及其测试、文档，不重写所有 DSH 插件，不修改 `dsh-plugin-chatgpt`、LCU、Kimi，不发布或安装到用户机器，不自动调用收费模型。

DSH 宿主基线是已审阅接口的 Mac `0.2.1-rc.5`。不把 alpha 的 Teams/OAR/Forge 能力当成这次实现的前提。Codex app-server 是由 adapter 配置的 `command` 或 PATH 中的用户 CLI 启动；**实际 Codex CLI 版本必须单独核对**，不能从 DSH 桌面版本推导。

### 当前 Flywheel 已实现什么

| 通路 | 本轮状态 | 所有权与适用场景 |
|---|---|---|
| `harness: codex` | 已有独立 `codex exec --json` driver，默认 `hard` 路由采用 `gpt-6.1-sol / high` | Go 直接管理进程、deadline、工作目录与验收；适用于 Flywheel 的有界工作单元 |
| `harness: dsh` | 独立 DSH SDK worker 调用已注册的原生 LLM provider | Flash／GPT 原生工具循环；每次新 SDK session，不依赖桌面活父 agent |
| `dsh-subagent-codex-pro` | 本轮**未消费、未修复、未集成** | DSH 交互会话中的 Codex 委派、持久 thread、后台 job；需先完成本文加固 |

`codex-pro`／`subagent_codex` 是 subagent provider／工具，不是 Flywheel DSH worker 的原生 LLM provider。把配置的 `channel` 改成它的名字不会完成接线。未来接入必须另做明确的消费者与能力握手；当前不改变默认 `hard` 路由。

相关 Flywheel 入口：[driver 接口](../../internal/drivers/drivers.go)、[Codex CLI driver](../../internal/drivers/codex.go)、[DSH driver](../../internal/drivers/dsh.go)、[原生 worker 协议](../../plugins/dsh/PROTOCOL.md)、[默认配置](../../templates/flywheel.config.example.json)。

## 2. 审计基线与证据等级

本次重新读取 GitHub `main`，仍为 **`fc6a417888335507e24a67d4dd5653f4894cab3e`，package version `0.6.4`**。源码固定入口：[该提交](https://github.com/ckanner/dsh-subagent-codex-pro/commit/fc6a417888335507e24a67d4dd5653f4894cab3e)。

本地只读快照位置为 `/workspace/scratch/fac5a5a9409b/dsh-plugin-review/dsh-subagent-codex-pro`。它是提取的源码目录，**没有 `.git`**；另一 session 应使用真实 checkout，先记录 `git status`、HEAD、package version、Node 版本、`codex --version`，避免覆盖用户已有修改或把本快照当成可直接提交的仓库。

证据区分：

- **前轮已用协议桩复现，本轮源码复核：**启动阶段 deadline 无法解除等待。前轮设置 50 ms，约 358 ms 时仍 pending 且子进程存活；100 ms 时约 353 ms 仍如此。测试另有 2 秒外层保护，外部 `AbortController.abort()` 后才进入 settlement／cleanup。没有真实模型调用。原临时桩已删除，需在 adapter 仓库补成可保留的回归测试；这些观测时间不是建议的 CI 超时阈值。
- **本轮静态确认：**resume 参数与回执校验不足、默认权限过宽、usage 未贯通、两入口结果不一致、persistent 文本 marker 与 JSON 结果冲突、leader 退出不能单独证明后代清理。
- **未在本 session 实测：**用户 Mac 的真实 app-server／DSH 生命周期、具体账号和模型、远端 usage、订阅限额、实际沙箱和退出后的子进程。上游 commit／README 自述的 live 结果属于上游记录，不当作本 session 的验证。

## 3. 关键文件与当前行为

以下链接固定在 adapter 的审计提交，避免另一 session 读到更新后混淆问题是否已修。

| 文件 | 实际职责／需要关注的部位 |
|---|---|
| [src/run.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/fc6a417888335507e24a67d4dd5653f4894cab3e/src/run.ts) | `startCodexRun`、`spawnAppServer`、`requestCancel`、deadline、`teardown`、`delegateOnce`；启动等待与运行取消信号不同 |
| [src/wire.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/fc6a417888335507e24a67d4dd5653f4894cab3e/src/wire.ts) | initialize、thread start/resume、turn start/completed、事件关联、审批处理、最终文本选择；尚未处理 token usage 通知 |
| [src/jsonrpc.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/fc6a417888335507e24a67d4dd5653f4894cab3e/src/jsonrpc.ts) | pending RPC、signal／timeout／dispose；传输层已有取消机制，需要生命周期层真正传入同一控制信号 |
| [src/index.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/fc6a417888335507e24a67d4dd5653f4894cab3e/src/index.ts) | `CodexExecProvider.start`、自定义工具 `runCall`、session thread 复用、background jobs、`structuredOf`、`withThreadMarker`、默认权限 |
| [src/tool.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/fc6a417888335507e24a67d4dd5653f4894cab3e/src/tool.ts) | `subagent_codex` 的输入 schema、前台／后台分支、model/effort/timeout/cwd/schema/images/thread 控制 |
| [src/jobs.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/fc6a417888335507e24a67d4dd5653f4894cab3e/src/jobs.ts) | job settlement；当前主要留下 status、文本 result 或 detail，不是完整运行收据 |
| [src/dsh-types.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/fc6a417888335507e24a67d4dd5653f4894cab3e/src/dsh-types.ts) | 本地镜像的宿主结构类型；扩展结果前须核对真实 rc.5 契约，不能只改这个文件就宣称宿主支持 |
| [tests/](https://github.com/ckanner/dsh-subagent-codex-pro/tree/fc6a417888335507e24a67d4dd5653f4894cab3e/tests) | 已有 wire、JSON-RPC、thread choice、spawn failure、tool 与 live 测试；必须增加跨真实子进程的生命周期回归 |

`src/codex-exec.ts` 是另一个 CLI helper／测试入口；adapter 的主要生产委派已走 app-server。只修改该 helper 的 argv，不能修复 `startCodexRun` 的问题。

## 4. 必须完成的加固

### P0：让 deadline 覆盖完整生命周期

当前 timer 调用 `requestCancel()`，取消的是内部 `runAbort.signal`；但 initialize、thread/start、thread/resume、thread/name/set 等启动等待使用外部 `signal`。因此 deadline 触发后，启动 RPC 仍可能无限等待。尚无 turn ID 时，`turn/interrupt` 也不能帮助结束握手。只有 published run 才存在的 handle／dispose，不能补救 publication 之前的卡住。

需要一个覆盖本次调用的取消／deadline 所有者：外部取消、内部 deadline、进程失败都应使当前阶段的 RPC 退出，继而执行同一套幂等 teardown。区分 timeout、用户取消、启动失败和模型失败，不能把启动超时笼统写成“request aborted”。显式 `timeout=0` 如继续支持，应保留为用户主动关闭运行 deadline 的选择；清理窗口仍须有界。

运行 deadline 到期后，在**明确的有界清理预算**内结算；不要声称 timeout 恰好到毫秒时所有进程已退出。初始化、建／续 thread、命名、turn start、执行、等待终态和关闭，都要有可中断路径。终态完成后清除 timer、abort listener、pending RPC 与进程监听器，不再接受迟到事件把失败改成成功。

### P0：证明 cwd、模型和权限实际生效

当前新 thread 会发送 model、reasoningEffort、cwd 和权限参数，但主要只检查 thread id 与 ephemeral 属性。resume 只发送 `{threadId}`，只验证返回 thread id；显式 `thread_id` 与新的 cwd 同时出现时，spawn 的目录并不证明 thread 的工作目录改变。

同时，`turn/start` 当前仍传 `effort`，所以“resume 时 model 与 reasoning_effort 都完全忽略”的 README 说法也不精确。必须区分 thread 配置、per-turn effort 和本次请求的有效配置。

本次核对的 OpenAI 官方源码已存在 resume／turn 的覆盖字段及回执：

- [ThreadResumeParams](https://github.com/openai/codex/blob/67f0994b532b8740f14b5b738af7b7b2a915b1d6/codex-rs/app-server-protocol/schema/typescript/v2/ThreadResumeParams.ts) 包含 model、cwd、approvalPolicy、sandbox 等。
- [TurnStartParams](https://github.com/openai/codex/blob/67f0994b532b8740f14b5b738af7b7b2a915b1d6/codex-rs/app-server-protocol/schema/typescript/v2/TurnStartParams.ts) 包含 cwd、model、effort、approvalPolicy、sandboxPolicy、outputSchema。
- [ThreadStartResponse](https://github.com/openai/codex/blob/67f0994b532b8740f14b5b738af7b7b2a915b1d6/codex-rs/app-server-protocol/schema/typescript/v2/ThreadStartResponse.ts) 与 [ThreadResumeResponse](https://github.com/openai/codex/blob/67f0994b532b8740f14b5b738af7b7b2a915b1d6/codex-rs/app-server-protocol/schema/typescript/v2/ThreadResumeResponse.ts) 提供有效 model、cwd、approvalPolicy、sandbox、reasoningEffort。

这是固定源码提交的协议证据，**不表示用户已安装的 CLI 自动具有相同能力**。实现前按实际 CLI 对应 schema 核对：支持时明确传参并校验回执；不支持、回执缺失或与请求冲突时明确拒绝。不能把字段被服务端接受当作已生效，也不能静默换目录、模型或权限。显式指定旧 thread 失败时，不得悄悄改成新 thread；自动记住的 session thread 的冷启动策略应有明确、有限、可观察的范围。

当前默认权限是 `dangerously-bypass-approvals-and-sandbox`；`never` 只发送 approvalPolicy，不显式给出 read-only sandbox。**禁止批准不等于只读，cwd 也不是权限边界。** 为 future Flywheel 消费提供明确的 read-only／workspace-write 策略及回执校验；full access 只能是明确选择，不能作权限失败的回退。兼容既有配置时写清迁移，不能悄悄改变其语义。DSH provider 的输入能力与自定义工具的字段不同，不能捏造宿主会传一个不存在的 permission 参数。

### P0：补齐 usage，正确区分累计与最近请求

当前 wire 没有消费 `thread/tokenUsage/updated`，`CodexRunOutcome`、provider result、工具和 job 也没有完整 usage 贯通。这里首先要修数据契约，再考虑显示或成本估算。

官方 [ThreadTokenUsageUpdatedNotification](https://github.com/openai/codex/blob/67f0994b532b8740f14b5b738af7b7b2a915b1d6/codex-rs/app-server-protocol/schema/typescript/v2/ThreadTokenUsageUpdatedNotification.ts) 包含 threadId、turnId 与 tokenUsage；[ThreadTokenUsage](https://github.com/openai/codex/blob/67f0994b532b8740f14b5b738af7b7b2a915b1d6/codex-rs/app-server-protocol/schema/typescript/v2/ThreadTokenUsage.ts) 含 `total` 和 `last`。更关键的是官方 [TokenUsageInfo 实现](https://github.com/openai/codex/blob/67f0994b532b8740f14b5b738af7b7b2a915b1d6/codex-rs/protocol/src/protocol.rs)：追加一次用量时将它加入累计 total，并用它替换 last。

因此：

- **`last` 不是整次 delegation／整个用户 turn 的保证值。** 同一 turn 可能经过多次模型响应和工具循环，只取最终 last 会漏计。
- **每条 total 通知不能相加。** 这些是累积快照，会把先前请求重复计算。
- 新 thread 只有在确认历史基线为零时，才可把最终有效累计值当成本次用量。resume 必须取得同一 thread、当前 turn 之前的可靠基线，再用最终累计值减基线；不能拿当前 turn 的第一条中途通知冒充起点。
- 例：恢复前 total=1000，本轮通知为 1100、1150、1150。可证明的本轮用量是 150，不是 3400，也不是最近请求的 50。若无法证明 1000 这个基线，返回 unknown；不能把 1150 算成本轮，也不能靠猜测补零。
- 按 thread／turn 过滤并处理重复、乱序、计数回退或重置；保留原始 usage 来源和有效范围。不同 thread 的事件不能污染本次记录。
- cached input 是 input 的细分，reasoning output 是 output 的细分；不要在总量里重复加。cache-write 只在实际版本报告时保留。优先使用供应商的有效 total，不能把上下文大小、估计值或额度数字伪装成结算账单。
- 没有 usage、基线未知、字段无效、事件范围不完整时保留 unknown/null。美元成本也保持 unknown，除非存在真实 cost 或明确的适用定价；订阅调用不能自动写成 0 美元。

六个字段的来源形状见 [TokenUsageBreakdown](https://github.com/openai/codex/blob/67f0994b532b8740f14b5b738af7b7b2a915b1d6/codex-rs/app-server-protocol/schema/typescript/v2/TokenUsageBreakdown.ts)。如果采用 per-response 明细计量，也必须按实际协议可用的唯一标识去重，证明覆盖本次完整调用；不能从文本长度估 token。

### P0：终态、关闭与进程所有权分开记录

目前 `CodexRunOutcome` 主要携带 output、stopReason、diagnostic、threadId、images。它不能直接表示实际 exit code／signal、是否超时、是否取消、流是否截断、是否完成清理，以及请求值与实际生效值。

建立共享的内部运行收据，至少保存请求／运行关联 ID、threadId／turnId、请求和有效配置、起止时间、语义 stopReason、物理退出、timeout/cancel、cleanup 状态、usage 及其范围、错误阶段与原因。正常文本、turn/completed、子进程退出、DSH job completed 是不同证据，不得互相替代。失败或取消也保留已观察的 thread、产物与已知 usage。

现有 teardown 主要向 app-server leader 发 SIGTERM，再 SIGKILL；`waitForExit` 的有界等待返回本身不证明所有后代已退出。应明确本次拥有的进程范围和平台策略，走 interrupt／关闭协议，再有界终止并确认。覆盖正常完成也残留持有 stdio 的子进程情形；清理失败必须可观察并阻止成功。只清理本次拥有的进程组，不能按名称扫描杀死其他 Codex／用户任务，也不能保证主动逃离进程组的任意进程。

宿主关闭、父 agent dispose、job cancel 与外部 AbortSignal 都应进入同一生命周期。不要把“后台 job”解释为脱离 DSH 宿主仍能可靠存活；需要脱离桌面的任务应由独立 Go／CLI 所有者启动。

### P1：两入口一致，结构化结果不被显示文本破坏

保留 `CodexExecProvider.start` 与 `subagent_codex` 工具两个入口，但让它们消费同一内部运行收据和结果归一化逻辑。宿主目前的 stopReason 没有 timeout 时可按原契约映射 aborted，同时用结构化诊断保留 timeout 事实；不能擅自扩展宿主 enum。

当前 provider 的 `structuredOf` 只是尽力 `JSON.parse`，失败时丢掉 structured；自定义工具在 persistent 模式下还会把 thread marker 追加到正文，因此原本合法的 JSON 结果不再是完整 JSON。修复时将 **raw final text／parsed structured result／thread metadata／人类展示文本**分开。请求 schema 时，JSON 无效、根类型或约束不符应明确失败；不要把 `JSON.parse` 成功当作完整 schema 校验。使用清楚声明的 schema 支持范围，未支持的形状拒绝，不“尽力成功”。

thread id 通过宿主实际支持的结构、单独 metadata 或可读取运行收据暴露；文本 marker 可保留给人看，但不可污染机器消费的 JSON。background job 也应可取得相同的终态、thread 和 usage 信息。若宿主字段受限，设计一个明确的兼容出口并测试，不仅修改本地 `dsh-types.ts` 就声称生效。

## 5. 与 Flywheel 的未来映射约束

这部分是**后续消费者的验收条件**，本次不在 Flywheel 自动增加 driver。

| Flywheel 字段／要求 | adapter 应能证明或保留的事实 |
|---|---|
| `CallArgs.Cwd` | 实际 thread／turn cwd 与规范化请求一致；resume 冲突不能忽略 |
| `Model`、`Binding.Model/Thinking` | 请求模型、effort 与实际生效配置；不静默沿用历史配置 |
| `PermissionMode` | read-only／workspace-write 的真实策略，审批 never；不可用就失败 |
| `Watchdog`、`OnEvent` | 可取消的完整生命周期和有界事件流；Go 仍可施加外层 deadline |
| `SessionID` | 明确的 thread／resume 句柄，不从自由文本猜；Flywheel Execute/Review 当前默认新会话 |
| `Result.ExitCode/Signal/TimedOut/Cancelled/Truncated/SpawnError/CleanupError` | 分别保留，不能用一个 completed 标记抹去失败原因 |
| `Result.Settled` | 只有可靠成功终态且关闭／清理条件成立才能用于成功判定；取消即使 exit 0 也不能成功 |
| `Result.Usage` | 当前调用范围的已知用量；未知是 nil，不是全零 |
| `Result.CostUSD` | 真实可知值；没有来源时 nil |
| `Result.Text/ErrorMessage/SessionID` | 最终正文、明确错误、可追踪 thread；结构化数据另有不污染正文的出口 |

Flywheel 的错误分类仍是 `AUTH/QUOTA/RATE_LIMIT/SERVER/TRANSPORT/TIMEOUT/EMPTY_RESPONSE/INVALID_REQUEST/CONTEXT_WINDOW_EXCEEDED`。取消另外体现在 `Cancelled`，不要凭本 adapter 的内部 stopReason 擅自增改这套线契约。保持 adapter 内部原始错误和阶段，以便未来稳定映射。

Flywheel 不信任模型宣称完成：即使 adapter 返回可靠成功，还必须由 Go 验证 done-report、运行 L1／评审／里程碑 gate、确认同一候选与持久化 Git tree。adapter 不写 Flywheel state、不自建工作单元重试循环、不自动提交或合并 Go 管理的交付。

**认证、额度与账单必须分开。** ChatGPT 订阅／Codex OAuth 登录成功，不等于有通用 API 额度，也不等于无限调用或零边际成本。独立 API credit、订阅限额和当前路由的计费来源必须分别确认。本文不要求复制 token、伪装客户端来换取不同授权，也不把 originator 的桌面显示标签当成授权或计费证明。

## 6. 最低真实验收矩阵

优先用本地 fake app-server 与真实 Node 子进程，所有默认测试不需要账号、不触发模型。每个卡住场景都设独立外层 watchdog，避免测试 runner 自己永久挂住。

| 场景 | 必须断言 |
|---|---|
| 可执行文件缺失／spawn error | 一次调用失败、无未处理 rejection、不拖垮 DSH host |
| initialize 不响应 | 内部 deadline 能解除 startup 等待；在清理预算内 settle，子进程确已处理 |
| thread/start、thread/resume、thread/name/set 分别不响应 | 与 initialize 同样受控；记录具体阶段，不能只等外部 abort |
| turn/start／turn/completed 不响应 | timeout 与用户 cancel 可区分；迟到 completed 不能覆盖失败 |
| 启动前／启动中／执行中取消 | 所有 pending 被释放，teardown 幂等；未发布 handle 也能清理 |
| TERM 被忽略、后代持有 stdio、正常 leader 退出但后代仍活 | 有界升级与本次进程范围清理；失败记录 cleanup 未确认，不伪装成功 |
| 模型／cwd／sandbox 回执与请求不符 | prompt 前拒绝；不静默采用历史值或扩大权限 |
| resume 到 A，却请求 B；切换 model／effort／权限 | 版本支持则真正应用并验证，否则明确失败；检查 thread 实际执行目录，不只看 spawn cwd |
| 显式 thread 无效、自动记忆 thread 无效、并发复用 | 显式指定不静默冷启动；自动恢复有界且可见；同一 thread 不被错误并发写入 |
| 一次 turn 内多次模型请求 | 本次 usage 包含全部有效请求；最终 last 不冒充完整 delegation |
| 重复／乱序／错误 thread/turn usage | 不双算、不串单；计数回退、缺字段、基线缺失保持 unknown |
| 已有历史 thread 的累计用量 | 例如 1000→1100→1150→1150，本次正确为 150；无法证明 baseline 时不得伪造 150 |
| partial/final/commentary、终态缺失、流提前关闭或截断 | 只有符合协议的最终内容与终态能成功；不能选上一轮的旧 final answer |
| schema 合法／不合法；persistent／ephemeral | 机器结果可直接解析并满足约束；thread marker 不污染 JSON |
| 原生 provider／自定义工具；前台／background job | 相同配置与终态语义；usage/thread/error 不因入口改变而丢失 |
| image 参数与产物回归 | 保留已有支持；缺失文件／不支持能力明确失败，不以文字占位冒充图片已传递 |
| 无 usage／无 cost／认证或额度失败 | unknown 与 0 区分；AUTH、QUOTA、RATE_LIMIT 不混为正常空回答 |

运行现有 `npm run typecheck`、`npm run build`、`npm test`，并加入上述行为测试。不要只断言 argv 字符串、只 mock promise 成功，或通过扩大 timeout／删除防御分支让测试变绿。已有 live 测试保持显式 opt-in，需用户另行授权后才运行。

## 7. 可直接粘贴到另一个 session 的任务

```text
请在 ckanner/dsh-subagent-codex-pro 独立仓库完成一次有界加固，不修改 Flywheel、dsh-plugin-chatgpt、LCU 或 Kimi，不自动安装到我的 Mac、不发布、不 commit/push、不运行付费模型。

背景：我使用 DSH 最新 Mac 桌面；本次已核对的接口基线是 0.2.1-rc.5。Flywheel v0.25 由 Go 拥有编排、状态、验证、worktree 和提交；当前 hard 路由走独立 Codex CLI，还没有消费本 adapter。你要先修 adapter 自己，而不是假装集成已完成。

审计基线：2026-11-01 远端 main 为 fc6a417888335507e24a67d4dd5653f4894cab3e，package v0.6.4。先看真实 checkout 的 AGENTS.md、git status/HEAD、Node 与 codex --version；保留我已有修改。读 src/run.ts、wire.ts、jsonrpc.ts、index.ts、tool.ts、jobs.ts、dsh-types.ts 和现有 tests。不要只改 codex-exec.ts，因为生产委派主要走 app-server。

优先修复：
1. deadline 必须覆盖 initialize、thread start/resume/name、turn、关闭。现在内部 timeout abort 的是 runAbort，启动却等待外部 signal，启动会挂住。用真实 fake app-server 子进程复现并补回归，不只改超时数字。timeout/cancel/进程错误分开保留，publication 前也能幂等清理。
2. 每次新建／续接都确认有效 cwd/model/effort/权限。现有 resume 只发送 threadId；never 也不等于 read-only，默认仍是 full access。按实际 CLI 的官方 schema 实施覆盖并检查回执，不支持就明确失败；显式 thread 不得静默换成新 thread。
3. 补 thread/tokenUsage/updated 与终态运行收据。total 是累计快照，不能逐条相加；last 只代表最近请求，不能当整个 delegation。恢复 thread 要有本次 turn 前的可靠 baseline；不可靠就 unknown。缓存和 reasoning 不重复加；无 cost 不是 0。
4. 原生 provider、自定义 subagent_codex 工具、前台、后台 job 共用结果归一化。保留 thread/turn、有效配置、usage、真实退出、timeout/cancel、cleanupError 和错误阶段。
5. raw text、parsed structured、thread metadata、展示文本分开；persistent thread marker 不得破坏 output_schema 的 JSON。坏 JSON／坏 schema 不能静默只返回文本成功。
6. interrupt + 有界 TERM/KILL + 等待退出，处理启动失败、后代持有 stdio、宿主 dispose 和迟到事件。只清理本次拥有的进程范围，不能扫杀其他 Codex；无法证明清理时明确失败。

默认测试全部离线：分别卡住每个启动/运行阶段，覆盖外部取消、回执不匹配、resume 目录/权限变更、多请求 usage、历史累计基线、重复/乱序事件、structured/persistent 组合、两入口与 background job 一致性、真实子进程退出。沿用现有 typecheck/build/test 门禁，不删除防御逻辑来刷绿。

请先读完整交接文档 docs/integrations/codex-pro-adapter-handoff.md（位于 Flywheel 仓库；如该文件不可见，可让我提供），然后实施。完成后交付修改清单、证据、兼容迁移说明，以及尚未 live 验证的项目。不要把认证成功、ChatGPT 订阅、Codex 可用额度和独立 API credit 混成一个东西。此轮也不要改 Flywheel 默认路由或声称它已接上这个 adapter。
```

## 8. 仍未验证的 live 项目

当前文档与源码复核没有运行真实模型。下一 session 即使离线测试通过，也仍须明确标记：用户 Mac 的 DSH 安装／provider 与工具注册、实际 CLI 版本的 schema 与回执、真实只读／workspace 沙箱、不同模型和 effort、持久 thread 续接、远端 usage／限额、真实账号授权来源、后台 job 在宿主关闭时的行为、真实图片输入和产物，以及宿主退出后的进程清理。

这些检查需要实际机器与对应调用授权。上游 README 的历史实测、源码存在字段、fake app-server 通过，均不能替代它们；本文不声称任何一项已经在用户账号上验证。
