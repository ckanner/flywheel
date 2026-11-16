# 2026-11-16：会话活性与未知规划调用退役实现验证

[验证目录](README.md) · [设计契约](../design.md) · [会话恢复](../zh-CN/session-recovery.md) · [工作台](../zh-CN/flow-console.md)

## 来源与交付范围

- 基线：`ckanner/flywheel` main，`85c1b2dcad6e28c44438c5f5e3cf71a890d17c2c`。
- 分支：`fix-session-liveness-recovery`；[PR #53](https://github.com/ckanner/flywheel/pull/53)。
- 环境：macOS arm64、Go `1.27.0`、Node `26.12.1`。未修改真实 DSH profile、账号、凭据或用户配置。
- 本记录对应代码与离线验证，不声称 npm 发布、真实桌面 UI 或真实模型调用已验收。

以下初次实现、review 和检查记录保留原 PR 的证据；`3b8e3fc` 之后的根因复审及修正见末节。当前行为以设计契约与中英文使用指南为准。

## 实现内容

| 层 | 代码与行为 |
| --- | --- |
| 看门狗 | `watchdog.roles.<role>` 覆盖 idle/wall；规划角色默认 900 秒，执行/评审保持 300 秒；收尾宽限不变 |
| 规划提示词 | 非 native harness 明确禁止后台/异步子代理，调查必须前台完成；Kimi native/profile 仍可观察子任务 |
| 状态 | session ledger 增加 `abandoned` 状态和 `abandon` 对象；保留 unknown dispatch/outcome、原因、来源和旧 thread 证据 |
| 引擎 | 仅规划角色的 blocked、已计量、清理确认调用可退役；不应用结果、不重放、不复用旧 thread；关联 request 标为 cancelled |
| 人工边界 | 模型工具不暴露退役；DSH 用户命令证明或控制终端确认精确 invocation；证明绑定 taskDir、key、revision、reason、invocation，不能跨任务或重复使用；CLI 提供 `session --abandon` |
| 控制回执 | `session-control.applied/status/message/invocation` 由命令前后状态转移计算，send/return/claim/release 不再用上一次状态冒充成功；信封 `ok` 不再被误读为退役成功 |
| 收尾门 | `abandoned` 与 `consumed` 一样视为已结算：重开已交付节点、启动 investigation、规划收据门都接受它；其他执行/评审路径仍要求真实 consumed |
| 请求校验 | 退役会要求关联 planning request 存在且为 pending/running、input revision 与 role 匹配；终止请求、缺失请求、pending activation 都拒绝，不再任意改写 |
| 状态不变量 | schema 要求 abandoned 至少保留一个 unknown 事实并禁止非 abandoned 记录携带 abandon；Save 重新校验 sessions；audit 记录准确 invocation |
| 视图 | 规划面板显示未解开调用、准确 invocation 与原因，提供原因和确认框；退役后显示会话结果并允许显式 Plan |
| 文档与技能 | 设计、恢复、配置、工作台、使用手册、原生能力、协议和中英技能同步说明 |

## 关键验证情形

1. **误杀防护**：配置解析为规划角色选择 900 秒 idle，执行角色保持全局值；部分覆盖只声明 wall 时仍继承角色默认 idle；未知角色/非法值在派发前拒绝。
2. **提示词边界**：非 native 规划提示词包含前台调查约束；native profile 选项不注入该约束；现有规划/审查 JSON 协议不变。
3. **状态与 schema**：合法 abandoned 记录可原子保存/重载；缺 proof、非 human、空原因、未计量、清理未知、已知 dispatch+outcome、可恢复 thread、未知字段均在派发前拒绝。
4. **引擎退役**：成功路径保留 unknown 事实、清空可恢复 thread、取消关联 request、任务回到 paused、写 inbox/orchestrator log；无 proof、错误状态、未计量、清理未知、非规划角色、人工接管、缺失 session、错误 key 均拒绝。
5. **CLI/DSH 人工证明**：控制路径只接受匹配 key/revision/action/reason 的 host 证明；缺失、伪造、过期或 env 继承均失败；控制终端要求准确 invocation 确认；模型工具不可用。
6. **控制回执**：abandon/reconcile 返回 applied/status/message；`ok:true` 但记录仍 blocked 时 applied=false；快速 session 命令等待子进程退出后才生成回执。
7. **视图**：blocked 规划调用显示 invocation 和原因；原因/确认缺一不可；真实用户命令提交后响应被采用，退役记录从阻断面板消失。
8. **回归**：原有 planning、session、状态、CLI、插件客户端和 legacy host 测试保持通过；新增测试覆盖所有新分支。

## 独立 review 处理

- **证明重放/跨任务复用**：host 证明现在绑定规范化 taskDir、key、revision、reason 和 invocation；跨任务、错误 invocation、篡改 reason 都拒绝。共享 shell 仍可伪造 env，这已记录为“完整 shell 不属于模型工具边界”，与既能直接改写 state.json 的能力一致；实现不宣称 OS 级防伪。
- **回执假阳性**：回执由命令前后快照的差异计算；send 要求新 invocation，claim/release 要求 handoff 前后变化，return/reconcile 要求 consumed+cleanup。未结算的 send/return 返回 applied=false 和“刷新状态”消息。
- **abandoned 收尾门**：console reopen、investigation 和规划收据门统一使用 `TerminalSessionStatus`（consumed 或 abandoned）。
- **任意 request 改写**：退役要求 request 存在且 pending/running、input revision/role 匹配；终止请求、缺失请求和 pending activation 都拒绝。
- **未生效配置**：从 `watchdog.roles` 白名单移除从未接线的 intake/garden，避免静默忽略。
- **覆盖缺口**：新增 dsh proof、task/invocation 绑定、receipt 转移、收尾门、schema/Save 不变量和 request 拒绝测试。

## 检查记录

| 检查 | 结果 |
| --- | --- |
| `go build ./...` | 通过 |
| `go test ./...` | 通过（全部包；machine 包约 751 秒） |
| `go vet ./...` / `gofmt -l` | 通过；无诊断或未格式化文件 |
| `make coverage` | 通过：total 100.0%，每个 measured function 100%（含 machine 809.8s） |
| `npm --prefix plugins/dsh test` | 428 个插件测试与 10 个 rc5/legacy host 测试全部通过，0 失败/跳过；review 修复后又跑了 43 个直接相关测试，0 失败 |
| GitHub Actions CI | 通过（Linux + macOS，run 43861692624） |
| 真实模型/桌面/发布验收 | 未运行；离线 fixture 不等于真实账号或桌面验证 |

## 限制

- 本次没有真实 Kimi/DSH 设计调用、真实桌面按钮或 npm 安装包验收；真实模型仍可能长时间无输出，role idle 只是策略上限，不是活性证明。
- 快速 session 命令的等待测试使用本地 fake orchestrator；真实引擎启动延迟和状态保存仍需桌面场景复核。
- 退役只适用于规划角色；packet 角色继续使用人工接管/原调用对账路径，不因本次变更获得新的通用放弃动作。

## PR #53 二次复审与修复（2026-11-16）

### 基线与根因证据

复审基线为 `3b8e3fc4a83e240ab62b59b940bfb509de7d85f8`，环境为 Linux amd64、Go 1.27.4、Node 24.22.1。本节没有访问用户的告警仓库运行目录或真实 Kimi 账号；事故时间线依据 PR 描述，协议判断依据源码，修复验证使用真实本地进程和离线协议 fixture。

上游 Kimi 固定源码 `f538d534cc9f4e96f4bacfdf02e28df682a4f611` 提供了比“后台子代理没有输出”更基础的证据：

- [`PromptJsonWriter`](https://github.com/MoonshotAI/kimi-code/blob/f538d534cc9f4e96f4bacfdf02e28df682a4f611/apps/kimi-code/src/cli/prompt-render.ts#L149-L205) 缓存 assistant delta、忽略 thinking delta、缓存工具参数；因此前台模型调用也可能持续静默。
- [`runNativeTurn`](https://github.com/MoonshotAI/kimi-code/blob/f538d534cc9f4e96f4bacfdf02e28df682a4f611/apps/kimi-code/src/cli/v2/run-v2-print.ts#L576-L600) 订阅当前主代理的事件总线，不提供完整子任务进展流。

所以 stdout 静默不等于模型停止工作；300→900 秒只改变触发点，前台提示词不能补造观测。进程已清理、调用结果已知、任务结果可采用也是三件不同的事。

### 发现与处理

| 发现 | 处理与回归 |
| --- | --- |
| P1：print 仍按不可观察的输出 idle 杀死正常调用 | print 不启动 idle timer，仍保留有效角色 wall、显式取消与进程组清理。静默后合法完成的真实 CLI fixture 修复前失败、修复后通过；真正 wall 超时仍保留 unknown 结果，不接受成功 hint 掩盖超时 |
| P1：native 已观察进展但未传给 Go | 转发真实主／子代理 item、step 和生命周期元信息，不复制思考、工具正文或账号 banner；Go→Node→HTTP/WS 长进展测试超过 idle 窗口仍完整完成 |
| P2：已经排队的旧 idle 回调误杀新活动 | 同一锁内重核最新 deadline；确定性测试让旧回调在新输出之后执行，修复前误杀、修复后继续，真实新期限到达仍终止 |
| P2：退役后旧 resident Plan cache 仍挡住重试 | 同一 CAS 清退精确关联旧缓存，保留其他未知缓存；实际 Plan unknown→公开 abandon→重启 fresh Plan 测试通过，原线程与 unknown 事实跨代次留在审计中 |
| P2：合法目录别名被用户证明拒绝 | 先验证原始用户事件，再同步规范化请求与证明路径；符号链接、尾斜线、`/./` 通过，跨任务和篡改原始命令仍拒绝 |
| P2：reconcile 清除 pending 后误报未应用 | 实际 send 在持久化 intent 后、派发前取消可留下 consumed+pending；恢复后只对同 key/thread/invocation 的真实 pending 转换报告 applied，重复操作与改变身份仍拒绝 |

另一次独立复审发现逐 token 转发会放大 Go 的累计输出缓冲。现以有效 `idleTimeoutMs` 推导最多一秒的高频进展通知间隔，只在实际事件到来时合并通知，不用定时器或 ping 续命；生命周期与终态不节流。确定性测试覆盖 100000 次短 delta，真实桥接测试把 stdout 上限缩到 64 KiB，在大量 delta 后仍保留完整结果和清理回执。

新协议端到端回归还验证：普通 Run 不重放 unknown 调用；人工退役后普通 Run 不自动新建替代请求；显式 Plan 才创建新请求，重新设计与独立审查后停在待批准，没有自动进入实施。

### 本次最终检查

| 检查 | 本次修订结果 |
| --- | --- |
| `go build ./...` | 通过 |
| `make quality-gate` | 通过：vet、fmt-check、全部覆盖率测试；总计与每个 measured function 均 100.0% |
| `go test -race ./internal/watchdog ./internal/drivers -count=1` | 通过，无 race 报告 |
| `npm --prefix plugins/dsh run check` | 432 个插件测试和 10 个 rc5/legacy host 测试通过，0 失败、0 跳过，包含打包 host 检查 |
| `FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(DSH\|CodexPro\|ClaudeCodePro\|KimiNative)NodeWorkerOfflineBridge$' -count=1` | 四个真实 Node worker 离线桥接通过；Kimi 含主代理、子代理、密集进展及 64 KiB 输出预算场景 |
| `git diff --check` | 通过 |

前述 macOS/CI 记录不自动代表本次修订；本节记录本地 Linux 离线结果，修订后的远端 CI 以对应 commit 的运行记录为准。

真实模型、用户桌面按钮、npm 发布和告警仓库复测均不属于本节离线证据。
