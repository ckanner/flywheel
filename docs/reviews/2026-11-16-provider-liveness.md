# Provider 活性、超时与恢复复审（2026-11-16）

[评审索引](README.md) · [设计契约](../design.md) · [PR #53](https://github.com/ckanner/flywheel/pull/53)

## 结论与适用范围

Kimi 事故暴露的是跨进程调用的观测问题。复查确认，DSH、Codex Pro、Claude Code Pro 也存在“底层持续工作，但外层没有收到活动，因此触发输出 idle”的缺陷。还确认了 Codex 原始协议流容量过小，以及 DSH 超时后丢失已验证清理事实的问题。

**本文是指定源码的审查记录，不是这些新发现已经修复或发布的声明。** PR #53 的 Kimi 专项修复不能被当作所有 Provider 已通过活性验收。同轮另补正 PR #53 CI 相关的测试时序及 Git 还原命令超时传递；下列跨 Provider 生产代码发现仍需后续实现与验收。

后续实现与验证另见 [Provider 修复验收记录](../validation/2026-11-16-provider-liveness-and-titles.md)。该记录对应后续代码与精确 companion 候选，不追溯改变下述审查基线和复现结果。

### 审查身份

| 对象 | 实际检查的版本 / 源码 |
| --- | --- |
| Flywheel main | `85c1b2dcad6e28c44438c5f5e3cf71a890d17c2c`，0.35.0 |
| Flywheel 审查及复现基线 | PR #53 `4ca33427231fb1d3d5844d8c146c704189a8ae55`，包含已完成的 Kimi liveness 修复；当时尚未合并 |
| Codex Pro | 最新 main `29b388a286748740b8bbbcbcdf0e5bf02038befa`，0.10.0；Flywheel 实际 bundle 与该版本一致 |
| Claude Code Pro | 最新 main `f560162d3bca6de8a76072729b0bffb57c543bfa`，0.5.1；Flywheel pin 为 `412904d3120895c30c2d75fd47216e2d663a64c6`，相关 `lib/` 生产代码逐字节相同 |
| ChatGPT | 最新 main `40d8f885e7a386af5f441b356b9c68496a47b0e3`，0.1.20；与 Flywheel vendor 记录一致 |
| LCU | 最新 main `505e516cbc909556a7b9855a454202cc28b583a8` |
| DSH SDK / AgentLoop | 实际发布包 0.2.2-rc.3；另核查 0.2.1-rc.5 发布包源码 |

测试使用 Linux、Go 1.27.4、Node 24.22.1，以及真实 driver、worker、公开插件 API、监督进程和 SDK。模型服务或 CLI 对端由离线 fixture 提供。短 idle 是为了快速触发同一状态转换，不能解释成真实模型延迟测量。未调用付费模型，未操作用户桌面或告警仓库。

## 第一性原理

应分别回答五个问题：调用是否已经派发、是否观察到任务进展、结果是否确定、拥有的进程是否已清理、哪些用量已知。它们不能互相替代。

- 进程还存在、连接还存在、输出了普通日志，都不能单独证明模型正在推进任务。
- 没有 stdout 也不能证明模型停止工作。协议可能缓存文本、隐藏思考、忽略子代理，工具也可能只有开始和完成事件。
- 实际进展经过 SDK、插件、supervisor、worker 中的任意一层过滤后，父进程就无法再凭原始流静默区分正常运行和挂起。
- 取消是终止机制；idle、wall、用户操作、父进程断开是不同的终止原因。
- 清理确认只能证明拥有的工作已停止，不能证明业务成功；业务结果 unknown 也不应抹掉独立可信的清理证据。

所以 300 秒改成 900 秒只能推迟误判。模型 effort 可能改变静默时长，但换模型、降低 effort 或改用 PTC 本身不能修复事件传递契约。

## 发现

### F1 / P1：DSH 丢失真实 streaming 活动，DeepSeek、ChatGPT 和本地 Agent Team 均受影响

DSH AgentLoop 0.2.2-rc.3 在 `src/agent.ts:486–503` 通过 `agent/assistant-stream` 和 `live.push` 发布逐 chunk 活动，完成后才在 `580–588` 写 durable `assistant/message`。SDK JSON-RPC Server 的 `src/server.ts:164–179` 只桥接 `session/event`、`agent/status`，不桥接这个 transient hook。[Flywheel bridge](https://github.com/ckanner/flywheel/blob/4ca33427231fb1d3d5844d8c146c704189a8ae55/plugins/dsh/src/bridge.js#L290-L305) 接收的 durable session event 因而缺少生成过程。

真实 rc.3 SDK 复现中，离线 adapter 每 100ms yield 一个 chunk，共 12 个。外层在约 1.2 秒内没有进展事件，最后才一次性出现 assistant/message。相同请求施加 300ms 输出 idle 后，在两个 chunk 后取消，结果为 `dispatch=dispatched`、`outcome=unknown`、`cleanupConfirmed=true`。

真实 Agent Team + PTC + spawn 子代理复现也一样：child 产生 19 个 transient frame，SDK 发出 0 个 streaming notification；主线等待 child 的连续无输出窗口为 1192ms。[native tree 转发](https://github.com/ckanner/flywheel/blob/4ca33427231fb1d3d5844d8c146c704189a8ae55/plugins/dsh/src/native.js#L106-L153) 观察到的仍主要是 durable 事件及生命周期，不能弥补这一缺口。

这不是 rc.3 独有的升级退化：0.2.1-rc.5 的两个精确 npm 发布包也有同一 hook / 桥接缺口。rc.3 做过实际 SDK 执行，rc.5 仅做源码核验。[冷启动](https://github.com/ckanner/flywheel/blob/4ca33427231fb1d3d5844d8c146c704189a8ae55/internal/drivers/dsh.go#L372-L390) 与[预热调用](https://github.com/ckanner/flywheel/blob/4ca33427231fb1d3d5844d8c146c704189a8ae55/internal/drivers/dsh_prewarm.go#L295-L300) 都受相同输出 idle 策略影响。

修复需要通过公开接口桥接真实 root / 已验证 lineage 的 child 活动，并声明当前 SDK 能观察到哪些阶段。只改 ChatGPT adapter 或换 DeepSeek 模型不能解决。

### F2 / P1：Codex Pro 丢弃 root 和 child 的过程 delta

[Codex wire](https://github.com/ckanner/dsh-subagent-codex-pro/blob/29b388a286748740b8bbbcbcdf0e5bf02038befa/src/wire.ts#L1112-L1191) 只将 root 已完成的工具 item、已完成 commentary 等转换为 `onProgress`，不转发 `item/started`、reasoning delta、agent message delta、command output delta。[NativeTracker](https://github.com/ckanner/dsh-subagent-codex-pro/blob/29b388a286748740b8bbbcbcdf0e5bf02038befa/src/native.ts#L218-L239) 主要转发子线程状态变化。Flywheel [Go driver](https://github.com/ckanner/flywheel/blob/4ca33427231fb1d3d5844d8c146c704189a8ae55/internal/drivers/codex_pro.go#L714-L719) 仍对这个过滤后的流启用 idle。

实际 Go → worker → bundled 0.10.0 machine API → supervisor / guardian → app-server fixture，每 40ms 发一个合法 reasoning delta，约 2 秒后正常完成：

| 场景 | idle | 结果 |
| --- | --- | --- |
| root 连续生成 | 500ms | 约 849ms 被终止，已产生 13 条 delta |
| 同一 root 对照 | 5000ms | 约 2401ms 完成，49 条 delta |
| native child 连续生成 | 500ms | 约 898ms 被终止，13 条 delta |
| 同一 child 对照 | 5000ms | 约 2342ms 完成，49 条 delta |

修复应增加与展示文本分离的 activity 通道，核对 root / turn / 已拥有 child，再节流输出小体积元信息。不能逐 token 转发思考或工具正文充当心跳。

### F3 / P1：Claude Code Pro 只转发主会话文本，普通工具与 partial stream 活动不可见

[run-local.js](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/f560162d3bca6de8a76072729b0bffb57c543bfa/lib/run-local.js#L123-L139) 只有主会话 assistant text block 才触发可见进度。普通 `tool_use` / `tool_result`、thinking、`stream_event`、子代理消息、`system/api_retry` 均不能维持该通道的活性。

[CLI argv](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/f560162d3bca6de8a76072729b0bffb57c543bfa/lib/cli.js#L141-L159) 使用 stream-json，但未启用 `--include-partial-messages`。Anthropic 的[官方流式文档](https://code.claude.com/docs/en/headless#stream-responses) 明确区分该选项与普通 stream-json；子代理 text/thinking 还涉及 `--forward-subagent-text` 及版本支持。只补 argv 仍不足以修复，因为当前 handler 也丢弃相应事件。

真实 Go → worker → 已安装 0.5.1 API → supervisors → CLI fixture，每 100ms 输出一轮合法 Read 工具调用及结果：

- idle 800ms：已输出 15 个协议帧、7 轮工具活动，仍 `timedOut=true`、`settled=false`；结果 unknown，清理已确认。
- 同一 fixture，idle 5s：完成 20 轮、41 个协议帧，`settled=true`、`outcome=completed`。

最新 main 与 Flywheel pinned 0.5.1 的相关生产代码相同，因此仅升级到当前 main 不能消除问题。

### F4 / P1：Claude native child 的新进展被当成重复状态永久丢弃

[NativeTracker.#emit](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/f560162d3bca6de8a76072729b0bffb57c543bfa/lib/native.js#L112-L121) 的去重键只有 kind、id、parentId、state、turnId、model。[task_progress](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/f560162d3bca6de8a76072729b0bffb57c543bfa/lib/native.js#L234-L249) 虽然更新了 usage，但同一 running child 后续产生相同去重键，因而一直不再上报。

确定性复现：输入 100 次 token / tool_uses / duration 均递增的 task_progress，内部 usage 正确到达 100 / 100 / 10000，外部只收到 **1 次** task-progress。这不是间隔合并，而是首次之后永久抑制直到身份或状态变化。

应分离状态账本压缩与实时活性上报：相同状态可以去重，新观察到的有效进展仍需更新活动记录。

### F5 / P2：Codex 正常原始协议流超过 1 MiB 就被终止，最终答案大小无关

Flywheel [bridge 限制](https://github.com/ckanner/flywheel/blob/4ca33427231fb1d3d5844d8c146c704189a8ae55/plugins/dsh/src/codex-pro-bridge.js#L7-L8) 将 `maxOutputBytes` 固定为 1 MiB。[插件 run.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/29b388a286748740b8bbbcbcdf0e5bf02038befa/src/run.ts#L705-L716) 统计整个 app-server stdout/stderr 累计流量，包含已解析并丢弃的 delta；它不是最终结果或保留内存的上限。插件自身缺省为 32 MiB。

复现用 5000 条、每条正文 64 字节的合法 delta，总正文 312.5 KiB，最终答案只有 17 字节。经 Flywheel，即使 idle 放宽到 5 秒，仍因原始 JSON 封装流量超过 1 MiB 而 `truncated=true`。直接调用同一 bundled 插件的 public machine API、采用其原有默认上限，则正常完成并确认清理。

因此补活动转发时，还要区分帧大小、协议累计流量、结果保留容量和活动通知容量。共享 [machine-worker](https://github.com/ckanner/flywheel/blob/4ca33427231fb1d3d5844d8c146c704189a8ae55/plugins/dsh/src/machine-worker.js#L221-L254) 的 progress / native-event 又有 1 MiB 累计上限，Go capture 另有 16 MiB 上限；不能把原始 token 全部继续转发。

### F6 / P1：persistent DSH 超时后丢失已确认清理和新会话身份，影响恢复

[finishDSH](https://github.com/ckanner/flywheel/blob/4ca33427231fb1d3d5844d8c146c704189a8ae55/internal/drivers/dsh.go#L456-L469) 在 timeout / cancellation 分支重建 SessionOutcome，使用请求前的 `a.SessionID` 并令其他事实回到 unknown / 默认 false。新建 persistent 会话的实际 ID、已知 dispatched、已验证 cleanup 因而丢失。

真实 Go watchdog 黑盒 fixture 在 SIGTERM 后发出完整、身份匹配且 `cleanupConfirmed=true` 的回执，然后 exit 143，没有子进程、截断或 CleanupError：

| 模式 | Go 返回 sessionId | Go 返回 cleanupConfirmed |
| --- | --- | --- |
| v1 fresh | `fresh-observed-session` | true |
| v2 persistent fresh | 空 | **false** |

两者均为真实 timeout。预热后的正式调用也使用相同 finishDSH。未知结果继续阻断自动重放是正确的，但不应抹去独立成立的清理与身份事实。当前 [abandon 准入](https://github.com/ckanner/flywheel/blob/4ca33427231fb1d3d5844d8c146c704189a8ae55/internal/machine/session_abandon.go#L89-L98) 要求 cleanup 已确认，所以这个信息丢失会进一步阻止合法人工退役。

### F7 / P2：timeout 的终止原因与底层 cancellation 混合，丢失主诊断

Codex Pro、Claude Code Pro、DSH 以及 Kimi native 的 driver 都存在将下层 cancelled 合并进外层结果的路径。已复现的 Codex 和 DSH timeout 同时返回 `TimedOut=true`、`Cancelled=true`；Codex 的 `ErrorMessage=nil`。原因是 [Codex driver](https://github.com/ckanner/flywheel/blob/4ca33427231fb1d3d5844d8c146c704189a8ae55/internal/drivers/codex_pro.go#L732-L771) 合并 flags 后，又在 Cancelled 时跳过错误消息生成。

这里需要保留发起终止的原因及执行取消的事实，不能让被动收到 SIGTERM 的回执覆盖 idle/wall。**不能据此断言规划 UI 必定显示“用户取消”**：unknown scoped 调用会先在 invokeScoped 阻断，未必经过后续 cancelled 分支。本项确认的是 driver 原因区分与诊断丢失。

### F8 / P2：旧 Codex CLI 的 JSONL 也不具备连续活性保证

显式 `harness:codex` 保留旧 exec 路径。[Flywheel 注释及配置](https://github.com/ckanner/flywheel/blob/4ca33427231fb1d3d5844d8c146c704189a8ae55/internal/drivers/codex.go) 把 JSONL 流视为适用通用 idle 的依据，但核查 OpenAI Codex 固定源码 `662a634e88fcc809d77da393d75572c3d814cb27` 的 [JSONL event processor](https://github.com/openai/codex/blob/662a634e88fcc809d77da393d75572c3d814cb27/codex-rs/exec/src/event_processor_with_jsonl_output.rs#L344-L603)，assistant / reasoning started 不输出，相关 delta 也不转换为 exec JSONL。

长期思考、长工具的正常静默因而仍可能触发通用 idle。这里是协议源码证据，没有在用户实际 Codex 二进制或真实模型上测量；旧 CLI 不是默认 Codex Pro，也不能通过名称相同推断能力相同。

## 嵌套工具、PTC 与其他边界

### DSH 内调用外部 Codex / Claude

root 或本地 child 的 assistant-stream 修好后，外部子代理仍需单独接活动桥。当前 Codex 的 [host provider / PTC 工具](https://github.com/ckanner/dsh-subagent-codex-pro/blob/29b388a286748740b8bbbcbcdf0e5bf02038befa/src/index.ts) 将部分 progress 写到诊断日志，前台 PTC 调用未提供进度回调，后台只 append 到 job handle。Claude 的 [前台委派](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/f560162d3bca6de8a76072729b0bffb57c543bfa/lib/index.js#L447-L449) 也未传 runtime.onEvent，后台记录不自动成为 root 活动。

这是源码已确认的缺失路径，尚未额外做完整 DSH→外部插件嵌套误超时实验。PTC 解决工具组合，不自动提供跨进程活性与清理证明；native 能力也需要被实际接入。

### ChatGPT 与 LCU

ChatGPT 0.1.20 本身能[接收文本 / reasoning delta](https://github.com/ckanner/dsh-plugin-chatgpt/blob/40d8f885e7a386af5f441b356b9c68496a47b0e3/src/api/events.ts#L220-L229)并[实时 yield 给 DSH](https://github.com/ckanner/dsh-plugin-chatgpt/blob/40d8f885e7a386af5f441b356b9c68496a47b0e3/src/adapter.ts#L586-L623)。本轮没有证据将 F1 归为 ChatGPT adapter 自身不 streaming。

LCU 的[默认 120 秒操作期限](https://github.com/ckanner/dsh-plugin-lcu/blob/505e516cbc909556a7b9855a454202cc28b583a8/src/connection.ts#L104-L109) 是具体操作 wall deadline，callTool 可显式覆盖 timeoutMs。[受管取消](https://github.com/ckanner/dsh-plugin-lcu/blob/505e516cbc909556a7b9855a454202cc28b583a8/src/managed.ts#L222-L254) 要求 turn cleanup 和 runtime exit，未将发出 cancel 等同于清理成功。PTC 嵌套工具有 durable dispatch 开始 / 结束事件；native journal 本身则不是实时 watchdog 通道。本轮未确认 LCU 新的同类缺陷，也未运行真实 macOS CUA。

### 保留已有正确机制

Codex / Claude 的 guardian、父进程 lifeline、私有 journal、精确调用恢复，以及 unknown 阻断自动重放，应继续保留。Claude 针对清理、恢复与独立 native jobs 的 8 个现有测试通过。独立 `claude-review` / `claude-team` 操作并不经过同一 Go idle watchdog，不能泛化为 F3；Flywheel 显式 wall 也确实传到了 Claude 插件，未发现被插件默认十分钟覆盖。

pi / omp / ZCode 的旧路径做了当前 driver 源码检查，本轮没有复现新的同等级缺陷；没有进行其真实模型或最新 CLI 协议的完整验收，不能给出“全部安全”的结论。

## 后续实现与验收建议

1. **先建立活动契约。** 每个 adapter 明示 root、子代理、工具、长思考分别有什么真实信号；缺少观测能力的阶段不能把输出静默当成挂死证明。保留总 wall、预算和明确取消。
2. **贯通真实活动元信息。** 绑定 invocation / root / turn / child lineage，按实际事件进行有界合并；不输出思考正文，不使用周期性假心跳。展示文字、状态账本、活动和 durable ACK 是不同通道。
3. **拆开容量限制。** 结果大小、单帧大小、保留缓冲、累计协议流量分别约束，不让丰富 native 事件挤掉终态和清理回执。
4. **保留已验证的恢复事实与停止原因。** unknown 结果继续阻断自动重放；证据仍可信且已确认物理退出时，不能把 cleanup=true 降为 false，或把 timeout 原因变成不明 cancellation。损坏流、身份不匹配与未知物理状态仍应保持保守处理。
5. **做跨层验收。** 至少包含长思考、仅工具活动、仅子代理活动、真实无进展、海量 delta、输出上限、idle/wall/用户取消、清理成功但结果 unknown、冷启动与预热、恢复后不重复派发。

在这些修复与验收完成前，增大 idle 只能作为显式的临时策略，降低误杀概率同时延迟真正停滞的发现；不应宣称某个统一阈值是所有模型的“最佳值”。
