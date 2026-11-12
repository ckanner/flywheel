# Codex Pro adapter 0.7.0：固定版本审计与下一 session 交接

> 历史归档：本文保留当时的设计和交接背景，不代表当前默认行为。
> 当前规范见[系统设计](../design.md)，使用指南见[文档索引](../README.zh-CN.md)。

> **历史审计，已由 0.8.0 接入替代。** 2026-11-03 已核验 [v0.8.0](https://github.com/ckanner/dsh-subagent-codex-pro/releases/tag/v0.8.0) 发布包及公开 machine API；Flywheel 0.28.0 默认 hard 路由改为 `codex-pro`。当前实现、迁移、完整策略事后检查限制见 [配置指南](../zh-CN/configuration.md#codex-pro-与旧配置迁移) 与 [本轮计划](../plans/2026-11-03-codex-pro-default.md)。下文保留 0.7.0 的当时证据，不能据此断言新版仍存在同样阻断。

核对日期：2026-11-01。审计对象为 [`ckanner/dsh-subagent-codex-pro@21fd7acccf1f5989b14fecdc1957559eea6914f8`](https://github.com/ckanner/dsh-subagent-codex-pro/commit/21fd7acccf1f5989b14fecdc1957559eea6914f8)，[package version `0.7.0`](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/package.json)。**启动 deadline、resume 参数传递和内部运行收据已有实质进展；公开结果、权限、用量完整性和跨宿主进程所有权仍有接入阻断。Flywheel 尚未消费这个 external adapter。**

本文替换旧版本审计。不要继续按“启动 RPC 不受内部 deadline 控制”“resume 只传 threadId”“完全没有 usage 实现”这些旧结论安排工作。本轮只读检查外仓、运行离线探针、更新 Flywheel 内的交接文档；没有修改、发布或安装外仓代码，没有调用真实模型。

追加复核：2026-11-02 01:57:41 UTC，GitHub main、独立 Git 拉取及 npm latest 仍为上述 SHA／0.7.0；重新执行 typecheck 与测试得到 106 pass、6 skip、0 fail，跳过项不算 live 通过。新探针及证据边界见 [npm 安装轮验证记录](../validation/2026-11-01-npm-installation.md#5-codex-pro-最新复核)。如果后续提交已更新，应先核对新的 SHA 再逐项验收，不按旧版问题列表直接判断新版。

## 1. 用户目标、现有通路与所有权

用户使用 DSH Mac 桌面，组合 Kimi 订阅、ChatGPT 订阅与 DeepSeek Flash API。Kimi 保持原生 CLI；DeepSeek 默认只用 Flash。Codex Pro 的加固由用户安排在另一个 session、另一个仓库，不能通过本轮偷偷修改外仓来宣称接入完成。

| 通路 | 当前含义 | 本文与它的关系 |
|---|---|---|
| Flywheel `harness: codex` | Go 直接管理 `codex exec --json`，已有完整 harness 通路 | 继续可用，不需要等 external adapter 完成 |
| Flywheel `harness: dsh` | 独立 SDK worker 调用注册的原生 LLM provider，每次新 session | 当前用于 Flash／GPT 原生 agent，不把 external subagent 当成 LLM provider |
| `dsh-subagent-codex-pro` | DSH external subagent provider 与 `subagent_codex` 工具，底层运行 Codex app-server | 先完成公开契约，再单独增加消费者 |

Flywheel 的 `provider` 表示 DSH LLM adapter，`channel` 表示本配置的账号／凭证／计费渠道。`codex-pro` 是 external subagent provider；仅把 `provider` 或 `channel` 改成该名字不能接通它。相关入口：[driver 接口](https://github.com/ckanner/flywheel/blob/main/internal/drivers/drivers.go)、[独立 Codex driver](https://github.com/ckanner/flywheel/blob/main/internal/drivers/codex.go)、[原生 DSH driver](https://github.com/ckanner/flywheel/blob/main/internal/drivers/dsh.go)、[worker 协议](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md)。

**Go 继续拥有契约、状态、运行 lease、deadline、重试、预算、worktree、验证、提交、合并和停止。** Adapter 只执行一次有边界的委派并返回可验证结果；不写 Flywheel 的 `state.json`，不自行重试整个工作单元，不批准自己的实现通过验收，也不替 Go 提交或合并。

## 2. 固定基线与证据等级

固定提交的 [CI run](https://github.com/ckanner/dsh-subagent-codex-pro/actions/runs/41312145457) 与 [release run](https://github.com/ckanner/dsh-subagent-codex-pro/actions/runs/41312145445) 已在本轮审计中核对为成功。[CI workflow](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/.github/workflows/ci.yml) 的测试矩阵是 Ubuntu、Node 22／24，执行 typecheck 与离线测试；它不证明用户 Mac 桌面行为。

证据分为三层：

- **本轮源码复核与离线复现：**下文四条探针使用脚本化协议响应和本地 fake app-server，没有网络、账号或模型推理。清理探针实际传递终止信号，只对进程组存活检查注入 `EPERM`；它证明失败结算问题，不声称当时留有真实孤儿进程。
- **上游自己的验证记录：**固定版 [README](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/README.md) 与 [changelog](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/CHANGELOG.md) 记录 Codex 0.165.0 app-server live 用例 4/4 通过，另有 `codex exec` live 用例通过；也明确 DSH desktop 端到端尚未运行。本轮没有独立复跑这些 live 用例。
- **尚无本轮证据：**用户 Mac、实际桌面／SDK profile、真实账号授权、订阅额度、远端模型输出、沙箱中的真实工具执行、宿主强杀后的清理。不能把源码、Ubuntu CI 或 fake-process 结果改写为这些项目已通过。

本轮复现材料如下；临时路径不是安装依赖，另一 session 不可假定它们仍存在：

| 材料 | 本轮路径 |
|---|---|
| 固定源码只读副本 | `/tmp/flywheel-codex-pro-audit-21fd7ac` |
| 审计记录 | `/tmp/flywheel-codex-pro-v070-audit.md` |
| 离线探针 | `/tmp/flywheel-codex-adapter-audit-probe.mjs` |
| 探针输出 | `/tmp/flywheel-codex-adapter-audit-probe.log` |

下一 session 应在真实 checkout 记录 `git status`、HEAD、package version、Node 版本和 `codex --version`，保留用户已有修改。源码副本只能用于核对，不能当成具备完整 Git 历史的工作仓库。

## 3. 0.7.0 已有进展：保留并增加回归

| 已有进展 | 固定源码依据 | 后续工作重点 |
|---|---|---|
| initialize、thread/start、thread/resume、thread/name/set 与 turn 使用同一内部取消信号／deadline；自动 resume 重试沿用原绝对期限 | [run.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/run.ts#L379-L510)、[lifecycle 测试](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/tests/lifecycle.spec.ts) | 保留真实子进程回归，不重新引入启动等待只受外部 abort 控制的问题 |
| resume 发送明确 cwd、model 和权限 overrides；effort 在 turn/start 传入 | [wire.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/wire.ts#L560-L656) | 补“有效字段缺失时拒绝”，不是再次只加请求字段 |
| 内部 `CodexRunOutcome` 已包含 usage、requested/effective configuration、cleanup、structured/schemaError | [run.ts outcome](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/run.ts#L126-L156) | 形成稳定公开出口，补齐物理终态及关联字段 |
| usage 使用累计快照与基线求 delta，rate limits 单独记录 | [usage.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/usage.ts) | 修复完整性状态与异常、turn 关联、早到事件的关系 |
| POSIX 自成进程组、teardown 幂等、TERM／KILL 后检查存活；`delegateOnce` 拒绝未确认清理 | [进程管理](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/run.ts#L260-L356)、[delegateOnce](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/run.ts#L780-L810) | 对齐公开入口失败语义，并接通外层 Go 与独立 PGID 的所有权 |
| schema 验证、结构化结果和展示文本已有独立处理 | [structured.ts](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/structured.ts)、[测试](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/tests/structured.spec.ts) | 保留 persistent marker 不污染机器 JSON 的回归 |

“内部收据中有字段”与“公开消费者能取得该字段”必须分开验收。

## 4. 仍阻止直接接入的六个缺口

### A. 有效配置缺字段只记录未匹配，仍允许继续

[`appliedConfiguration`](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/wire.ts#L383-L428) 遇到缺失字段会返回 `matched: false`，但不加入 `mismatches`；start/resume 仅在 `mismatches.length > 0` 时抛错。因此“字段存在但不同”会拒绝，“根本没证明字段生效”仍可进入后续 turn。

离线探针让 thread/start 只返回 thread id 与 ephemeral，省略全部有效配置，得到：

```json
{"accepted":true,"configuration":{"requested":{"cwd":"/tmp","model":"gpt-audit","approvalPolicy":"never","sandbox":"read-only"},"effective":{},"matched":false}}
```

必需的 cwd、model、approval policy、sandbox 回执缺失、类型错误、不支持或冲突，都应在发送用户任务前失败。requested、effective、缺失字段和失败阶段分别保留；不能把 requested 复制成 effective，或把“请求没有被拒绝”当作权限已实施。

Effort 是 per-turn 字段，不能拿 thread 的历史 reasoningEffort 冒充本次 turn 的证据。按实际 CLI 版本明确哪些事实由服务端回执证明、哪些只能证明参数已传入；消费者所需证据能力不可提供时明确报不支持。显式 thread 无效时继续保持不静默换新。

### B. 默认 full access，且缺少 workspace-write + never 模式

[`index.ts`](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/index.ts#L320-L326) 的默认值仍为 `dangerously-bypass-approvals-and-sandbox`；[bundle](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/cordis.patch.yml) 没有覆盖。离线默认 `apply` 探针也读取到这个模式。

[`THREAD_PERMISSION_PARAMS`](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/wire.ts#L38-L56) 的 `read-only` 有 `never + read-only`；`never` 只设置审批策略、不设置 sandbox；`approve-for-me` 则为 `on-request + auto_review + workspace-write`。后两者都不等于 Flywheel 要求的 `never + workspace-write`。

新增可显式选择、可核对回执的 `workspace-write + never`。Full access 只能明确选择，不能作权限能力不足或无人审批时的回退。默认值变更须说明兼容迁移；不能悄悄把一种权限解释成另一种。cwd 是工作目录，不是权限边界。

### C. 公开 provider 在 cleanup 未确认时仍可返回 completed

[`finish`](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/run.ts#L518-L570) 对 `cleanup.confirmed: false` 追加 diagnostic，但不必改变 stopReason。工具侧 `delegateOnce` 会拒绝；公开 [`CodexExecProvider.start`](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/index.ts#L433-L459) 仍可透传 completed，`dispose()` 也丢弃清理收据。

本轮清理探针实际发送终止信号，仅把负 PID 的 signal 0 检查设为 `EPERM`，公开 provider 返回：

```json
{"output":[{"type":"text","text":"AUDIT_OK"}],"stopReason":"completed","diagnostic":"process cleanup could not be confirmed: the owned process group was still alive after SIGKILL"}
```

所有公开入口都应把“无法确认收尾”作为不能交付成功的独立事实。宿主 enum 没有 timeout 时可映射合法的 aborted／error，但公共收据仍保留 timeout、cancel、cleanup 的各自原因。正常文本、turn/completed、leader exit、job completed 不能相互替代。

### D. 完整 receipt 没有稳定公开出口

公开结果主要是 output、stopReason、可选 structured／diagnostic；内部 usage、configuration、cleanup 没有贯穿。[`SubagentResultLike`](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/dsh-types.ts#L54-L74) 也反映了限制。[package exports](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/package.json) 只有根入口与 package.json，`src/run.ts` 不属于公开子路径。

需要版本化、机器可读、按单次运行关联的公共 receipt：可以是正式导出的 machine API、受控 RPC，或明确约定的单次收据出口。宿主结果结构受限时另设稳定出口；只改本地镜像类型、解析人类日志或 deep import 内部文件都不构成完整接入。

### E. 独立 PGID 不受当前 Go 进程组兜底覆盖

[`spawnAppServer`](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/run.ts#L260-L278) 在 POSIX 使用 `detached: true`。正常 SDK drain 可以调用 dispose，但 Flywheel 杀死自己拥有的 Node／DSH 继承进程组时，不能据此证明独立 Codex 组也已退出。

这里的缺口是两个所有者的边界没有接通。机器入口必须明确谁创建、登记、取消、等待回收，以及宿主突退后谁仍负责清理。保留独立组需要可证明的外层监督与注册协议；采用继承模式也不能让 adapter 误杀祖先组。单纯返回 PID，或先 detached 再事后登记，不能自动消除启动窗口和 PID 复用问题。

只清理本次实际拥有的范围，不能按名称扫杀其他 Codex。任意主动逃离管理范围的进程不在普通进程组保证内，不能把“信号已发送”写成“全系统无残留”。

### F. 坏 usage 通知仍可能被标成 complete

[`usage.ts`](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/src/usage.ts) 已有累计 delta。但 `observe`／`observeInvalid` 发现坏通知时主要设置 reason；`report` 的 complete／partial 条件没有完整纳入这些异常。

探针先提交合法的 100-token 快照，再提交负数／缺字段的快照，仍取得 `status: "complete"`、`run.totalTokens: 100`，同时 reason 写着 malformed token-usage notification。这个组合不足以证明本次完整用量。

完整性状态必须与坏通知、计数回退、基线来源、thread／turn 关联一致。保留已知量时说明它是部分量；无法可靠归属时使用 unknown，不补零。补测 turn/start 响应前到达的 usage、resume replay、其他 turn 和取消后的迟到事件；不能因一项已复现就假设其他时序正确。

累计规则继续保留：可靠 baseline=1000，随后 1100、1150、1150，本次可归属值为 150；不能相加 total，也不能把最后请求的 last 当整个 turn。cached input 与 reasoning output 已含在对应大类中，不能重复加。rate limits、上下文窗口、美元 cost 各有独立含义；没有真实 cost 或适用定价时保持 unknown，订阅不自动等于 0 美元。

## 5. 公共 machine receipt 的最低契约

以下是后续验收要求，**不是 0.7.0 已公开提供的字段清单**。实际协议不能证明的数据保持 null／unknown，并说明原因。

| 类别 | 必须可取得的事实 |
|---|---|
| 版本与关联 | receipt/protocol version、调用方 request ID、run ID、thread ID、turn ID；区分重试、迟到结果及另一委派 |
| 请求与有效配置 | requested／effective 分开，cwd、model、per-turn effort、approval policy、sandbox；缺失、冲突、匹配状态和证据来源 |
| 时间与错误阶段 | 起止时间、当前阶段、timeout、caller cancellation、spawn／protocol／model／schema failure |
| 物理终态 | 实际 exit code／signal、输出完整或截断；未知值不能由 exit 0 代替 |
| 所有权与清理 | 受管范围及所有者、取消／dispose 是否完成、cleanup confirmed 与原因；不能只有 leader 退出 |
| 用量 | complete／partial／unknown、本次 run usage、thread total、baseline、事件范围、异常原因；rate limits 分开 |
| 内容与产物 | 原始最终文本、通过 schema 验证的 structured、thread metadata、展示文本、产物／图片分别保留 |
| 成本 | 可知 cost 及来源；无来源为 null，不从认证或订阅推导价格 |

公开 provider、工具前台、后台 job、取消与失败路径应能取得同一套事实。通过公开入口测试，不只检查内部 `CodexRunOutcome`。收据需要可靠关联和有界生命周期；不能让 job 混用文件、无限积累全局记录或解析用户日志当数据协议。

Flywheel 分别保留 `ExitCode/Signal/TimedOut/Cancelled/Truncated/SpawnError/CleanupError/Usage/CostUSD`。可靠终态与收尾成立后仍须由 Go 验证 done-report、L1、独立评审、里程碑 gate，以及同一候选的持久化 Git tree。错误分类保持 `AUTH/QUOTA/RATE_LIMIT/SERVER/TRANSPORT/TIMEOUT/EMPTY_RESPONSE/INVALID_REQUEST/CONTEXT_WINDOW_EXCEEDED`；取消另行记录，不由 adapter 任意改变 wire contract。

## 6. DSH rc.5 接入边界与后续消费者

本轮只读检查了 Mac `0.2.1-rc.5` 分发包的接口，并参照固定版 [SDK protocol](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sdk/protocol/README.md) 与 [SDK server](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/sdk/server/README.md) 文档：

- 现有 stdio SDK RPC 有 initialize、session prompt/resume/wait、工作目录 get/set、shutdown，没有 external subagent/start RPC。
- initialize 解析 LLM call configuration；`codex-pro` 的 subagent provider 身份不等于 LLM adapter 注册。
- SDK 的 `subagent.finished` 转发限制为 local agent，不能靠该通知取得 external Codex 的结果。
- Host `subagents.startActivation({provider, request, parent, signal, delivery: 'caller'})` 可启动 external provider，但要求精确的 live parent agent；受管 drain 能覆盖 external 生命周期，不自动补足丢失的 receipt。

后续应选择明确公开的 machine 入口，或增加受控 Host plugin RPC，再实现独立 external-subagent driver／worker。Host 路线必须实际管理所需 parent 生命周期，不能虚构 parent ID。无需让额外父模型先推理一次再选择工具，也不依赖 Teams／OAR／Forge。

Go 传入 request ID、cwd、模型、effort、权限和 deadline，取得完整 receipt。Flywheel Execute／Review 每次使用新 thread／leaf；adapter 可保留交互用户的 persistent thread 功能，但不能自动带入该新会话契约。权限、receipt、所有权、无推理 doctor 与跨进程生命周期回归全部通过后，才可声明消费者可用。当前独立 Codex CLI 和原生 DSH 通路继续保留。

## 7. 验收矩阵：默认离线，真实设备单列

使用脚本化 fake app-server 和真实 Node 子进程；每个挂起测试有独立外层 watchdog，避免 runner 永久等待。保留 `npm run typecheck`、`npm run build`、`npm test` 门禁，不通过删除失败分支或扩大 timeout 冒充修复。

| 场景 | 必须断言 |
|---|---|
| initialize／start／resume／name／turn 各阶段不响应 | 同一内部 deadline 持续有效；未发布 handle 也有界清理，阶段和原因明确 |
| 启动前／执行中取消、timeout=0、自动 resume 冷启动 | 取消与超时区分；关闭运行 deadline 不关闭清理预算；重试不获得新总时限 |
| cwd/model/approval/sandbox 缺失、错类型、冲突 | 用户任务发送前失败；公共收据保留 requested、effective 与缺口 |
| read-only、workspace-write+never、显式 full access | 不支持即失败，不静默扩权；参数和实际工具权限分别验证 |
| 显式 thread 无效、恢复时变更目录／模型／权限／effort | 不把 spawn cwd 当 thread cwd；显式 thread 不换新；per-turn effort 不与历史配置混淆 |
| 已有正常文本，但 cleanup 未确认 | provider、tool、job 都不能交付 completed；dispose 结果可追踪 |
| 忽略 TERM、leader 先退、后代占用 stdio | 有界升级与回收，只处理本次范围；未知存活状态保持失败 |
| Go／Node／DSH 父进程在 startup、turn、teardown 被强杀 | 证明独立 Codex 范围的外层所有权已接通，不只测正常 dispose |
| 公开包入口／正式 machine 导出／Host RPC | 取得完整 receipt，不 deep import／解析日志；版本不支持明确失败 |
| 多次模型请求、重复累计快照、历史 baseline | 求可靠 run delta，不相加 total／只取 last；无基线不伪造完整量 |
| 有效 usage 后坏通知、回退、其他 thread／turn、早到事件 | 不串单、不双算；complete/partial/unknown 符合证据 |
| schema、persistent marker、图片及部分产物 | structured 真正验证，marker 不污染 JSON；失败保留产物，不以占位文本冒充图片 |
| 失败、取消、截断、未知 usage／cost | 统一公开终态；未知不是零，认证／额度失败不成为空回答成功 |
| macOS fake-process CI | 覆盖组清理、目录别名与宿主取消；Ubuntu Node 22／24 不能替代 |

真实设备仍须单列：用户 Mac 的实际 DSH／Codex 版本、provider／工具注册、SDK 与桌面 profile 授权、真实 read-only／workspace sandbox、模型和 effort、persistent thread、远端 usage／额度、图片及宿主退出后的进程状态。上游 Codex 0.165 live 记录可以参考，不能标为本 session 或用户机器已验。认证、ChatGPT 订阅、Codex 额度、独立 API credit 分别确认，不复制 token 或靠显示标签推断授权。

## 8. 可直接粘贴到另一个 session 的任务

```text
请继续加固 ckanner/dsh-subagent-codex-pro 独立仓库。固定审计基线为 21fd7acccf1f5989b14fecdc1957559eea6914f8、package v0.7.0（2026-11-01）。先读真实 checkout 的 AGENTS.md、git status/HEAD、package、Node 与 codex --version，保留已有修改；HEAD 已更新时逐项复核，不重做已修部分。

背景：我用 DSH Mac 桌面，已核对宿主基线为 0.2.1-rc.5。Flywheel 由 Go 拥有状态、lease、deadline、重试、预算、worktree、验证及提交；现有 GPT 通路是独立 codex exec 和 DSH 原生 LLM worker，尚未接入本 external adapter。这次先修 adapter 的公开契约，不修改 Flywheel、ChatGPT 插件或 Kimi，不自动部署到我的 Mac 或调用收费模型。提交、PR、发布按本会话已有授权执行。

保留 0.7.0 进展：startup 与 turn 共用内部 deadline/abort；resume 已传 cwd/model/权限，effort 在 turn/start；内部 outcome 有 usage/configuration/cleanup；POSIX owned-group 清理与 delegateOnce cleanup 拒绝；schema 和展示分离。不要继续按旧版“startup 不可终止／完全无 usage／resume 只传 threadId”来修。

优先完成：
1. wire.appliedConfiguration 缺有效字段时只有 matched=false，start/resume 仍继续。必需 cwd/model/approval/sandbox 缺失、错类型、不支持、不匹配都须在发任务前失败；保留 requested/effective/缺口。effort 按真实 per-turn 协议处理，不伪造回执。
2. 默认仍 full access；never 无 sandbox，approve-for-me 不是 never。提供明确 workspace-write+never 与read-only，full access 须显式选择，写清兼容迁移和实际工具权限验收。
3. public CodexExecProvider 在 cleanup.confirmed=false 时仍可返回 completed，虽然 delegateOnce 会拒绝。统一 provider、tool、前台、job、dispose 终态，未知清理不能交付成功。
4. 提供公开、版本化、按request/run关联的 machine receipt。内部 outcome 有部分字段，但公开结果丢 usage/config/cleanup；补实际 exit/signal、turn ID、起止时间、截断、关联和错误阶段。package 目前只 exports 根与 package.json，不能要求消费者 deep import run.ts 或解析日志。
5. app-server detached:true 自成PGID，Flywheel杀Node/DSH父组覆盖不到。明确对外所有权/监督协议，覆盖宿主突退、启动窗口和PID复用；不能只返回PID就声称受控，也不能扫杀别的Codex。
6. TokenUsageTracker 在有效快照后收到malformed通知仍可能complete。让complete/partial/unknown与异常、baseline、thread/turn、早到事件一致。累计值只求可靠delta，不相加；last不是整轮；未知usage/cost不补零。

默认验证全部离线：真实fake app-server子进程，分别挂住startup/turn，覆盖取消、缺配置回执、workspace-write+never、cleanup未确认的public provider结果、parent强杀、公开receipt、多次/重复/坏usage、resume基线、早到turn事件、schema与persistent展示、tool/provider/job一致性。保留typecheck/build/test门禁，增加macOS fake-process覆盖。真实Mac桌面和账号模型验证单列，不把上游README live记录当本会话已验。

完整上下文见 Flywheel 的 docs/integrations/codex-pro-adapter-handoff.md；看不到时请让我提供。完成后交付实际改动、公共接口、回归证据、兼容迁移和未验证项。Adapter内部修好不等于Flywheel已经接入：rc.5 stdio SDK无external-subagent/start RPC，后续仍需独立消费者与受控Host/machine入口，Go的状态与交付所有权不变。
```
