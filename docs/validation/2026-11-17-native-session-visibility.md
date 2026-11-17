# 2026-11-17：Provider 原生会话可见性与 Kimi 兼容复审

[验证目录](README.md) · [Kimi 协议](../../plugins/dsh/KIMI_NATIVE_PROTOCOL.md) · [PR #55](https://github.com/ckanner/flywheel/pull/55)

## 复审范围与基线

- Flywheel PR #55 的复审起点：`cb1edcd90cd081e3e457407d9234f577acaaed87`，分支 `fix/kimi-253-identity`；比较基线为 main `d9018eaf0f7d462795c0a8e5f76c478e424c4c95`。
- 原 PR 修正 Kimi 2.5.3 注册表与 `/meta` 分别生成 server ID 导致的拒绝，并保留 PID、启动时间、loopback、token 和能力校验。
- 以下 Kimi 部分同时检查原 PR、实际发布包、较新上游源码、真实本地进程及离线协议 fixture。没有连接真实账号，没有发送模型 prompt，没有执行标题生成，也没有进行桌面 UI 或付费推理验收。
- 本文中 Provider 历史可见、API 能力准入、模型推理成功、桌面显示和 Flywheel 可安全恢复是分别验证的事实，不能相互替代。

## Kimi：精确来源

| 对象 | 身份与用途 |
| --- | --- |
| 已发布包 | [`@moonshot-ai/kimi-code@2.5.3` tarball][kimi253]；真实安装后 `kimi --version` 返回 `2.5.3` |
| tarball SHA-1 | `c653b04968bcfd6136943aa7afd12bc358dbd5b2` |
| tarball SHA-256 | `7c6a44fb54265f9b0d10c35e32c538678a655598f49eba434780c60077fa6c7a` |
| tarball integrity | `sha512-NdBttZiktZhKdH1tu/25A+Il1MxO1RTE1gAdZDmogoCT4g8L7k3pcLeJpwy7PNJvHo9CF8fDJiaZ9nsIPn+Gag==` |
| 发布包 `dist/main.mjs` SHA-256 | `07019fce6f568b642d4422e1daa81f975de6a21c578c82a924925c2e667aa11a`；以其中源模块注释和实际行为核对 2.5.3，而非用最新 main 冒充已发布代码 |
| 较新官方源码 | [`a692ff9af2c68157297a324b013a8960ceae66bb`][kimi-current]，其 `apps/kimi-code/package.json` 标为 `2.6.2`；本轮对此身份做源码核验，未将其视为已安装包或模型验收 |

2.5.3 发布包中直接相关模块为
`packages/kap-server/src/routes/sessions.ts`、`sessionProfile.ts`、
`sessionAgentConfig.ts`、`protocol/ws-control.ts`、
`transport/ws/v1/agentState.ts` 及 `transport/ws/v1/connection.ts`。
发布 bundle 中保留了这些模块的源码注释；运行时实际发出的 OpenAPI／AsyncAPI
与调用结果是本轮兼容判断的依据。

## Kimi：确认问题、根因与修复

### 1. 身份拒绝后仍通过 HTTP 清理被拒绝的端点

原逻辑在 `/meta` 准入前保存 `base`／token。即使随后因版本／身份不一致拒绝，
`stop()` 仍会向该地址 POST `/api/v1/shutdown`；native 的 finally 路径也会
读写 remote-control。复现请求序列为 `GET /meta` 后紧接 `POST /shutdown`，
而结果已经是 `RUNTIME_IDENTITY`、未派发。

根因是混淆了“拥有刚启动的 OS 子进程”和“已认可该 HTTP 服务的身份／能力”。
修复后只有通过 `/meta` 准入的 runtime 才能执行 HTTP 清理；未准入时直接终止并
核对本次 OS 进程及已观察后代。native 回执的 `remoteControlOff` 保持 `null`，
不虚构远程状态；未派发调用的物理清理仍可确认。

回归同时覆盖版本不符、缺版本、remote-control 处于 on 的拒绝端点，以及损坏的
meta。拒绝场景仅收到 `GET /api/v1/meta`，没有远程状态修改、shutdown、会话创建
或 prompt；本地进程退出已确认。

### 2. 损坏的 `/meta` 被误分为可重试传输失败

`data:null`、缺失 data 或空 JSON envelope 会在字段读取处抛出 JavaScript
TypeError。原 print 准备把它归为 `PREPARE_FAILED`，Go 将该通用失败映射为
可重试 `TRANSPORT`，掩盖了确定的协议错误。

修复在字段读取前校验响应 envelope 和身份文档，损坏结构返回
`RUNTIME_PROTOCOL`，沿既有分类进入不可重试的 `INVALID_REQUEST`。这没有把所有
TypeError 一概视为协议错误：模拟真实连接失败的 TypeError 仍返回
`PREPARE_FAILED`，保留此前 PR 评论要求的可重试传输行为。拒绝的端点同样不接收
HTTP 清理动作。

### 3. 实际 2.5.3 创建接口没有应用 schema 中声明的归属与配置

原 PR 的 server ID 修正使实际 2.5.3 通过身份准入，但 named print 准备仍失败。
实际创建结果有正确的可读 title，却只有 `metadata.cwd`，没有
`flywheel_print_invocation`，也没有应用请求中的 model／permission／plan。
Flywheel 随后正确拒绝未确认的归属，因此“ID 已兼容”不能证明调用已恢复。

发布包的 create schema 声明了 `metadata` 和 `agent_config`；create handler
实际上只使用 cwd、title 和 environment。公开 POST `/sessions/{id}/profile`
才会应用 custom metadata 与这些配置。之前的 fixture 直接回显全部 create
字段，未覆盖真实发布包的遗漏。

修复使用以下可核验步骤：

1. 只接受本次 create 返回的 ID，并确认原始 title、物理 cwd、idle，以及没有
   既有 custom metadata／归属。
2. 重新读取同一 ID，检查真实 history 和 active／queued prompt 均为空。
   2.5.3 的 `message_count` 和 session usage 恒为占位零，不作为空历史证明。
3. 确认实际 profile schema 支持所需字段，才对这个新历史写入归属与请求配置。
   非本次归属、旧内容、变更后的 ID 或 unsupported profile 不会收到该修改。
4. 重新读取并核对 ID、title、cwd、归属、idle 和 model；print 再通过实际声明的
   agent-state 订阅确认 model、`permission_mode:auto`、`plan_mode:false`。
5. 关闭并确认整个准备 runtime 的物理退出后，原 print 路径才可使用 `--session`
   发送正式任务。

prepare 不接受 resume ID，不重写未知历史，不额外调用标题模型。native 仅在原有
完整生命周期能力已准入时共用新会话初始化；resume 分支继续要求已有绑定吻合，
不会尝试认领缺归属的历史。被 create 忽略的非默认 `agentProfile` 不能由无法
建立该 profile 的 setter 悄悄替代，直接返回 `UNSUPPORTED_SCHEMA`。

## Kimi：真实 2.5.3 无推理验证

新增手动验证入口：
[`plugins/dsh/scripts/verify-kimi-print-history.mjs`](../../plugins/dsh/scripts/verify-kimi-print-history.mjs)。
它接收一个已安装 Kimi CLI 的绝对路径，默认仅创建临时 home 和临时项目，不继承
账号凭据、模型环境覆盖或已有配置。唯一模型 provider 指向脚本自己启动的
loopback 拒绝服务器，并计数所有请求；初始 permission 故意设为 manual、plan
故意设为 true，以检查真正生效和跨进程保留。脚本没有 prompt 字段或推理入口，
结束后移除自己创建的临时 home。

可重现命令（安装精确包不修改工作仓库或用户 profile）：

```bash
kimi_probe_dir=$(mktemp -d)
npm install --prefix "$kimi_probe_dir" --ignore-scripts --omit=optional --no-audit --no-fund @moonshot-ai/kimi-code@2.5.3
node plugins/dsh/scripts/verify-kimi-print-history.mjs "$kimi_probe_dir/node_modules/.bin/kimi"
```

本轮对上述精确 2.5.3 包运行该入口，进程退出码为 0，JSON `verified:true`。

| 验证事实 | 实际结果 |
| --- | --- |
| 生产 `prepareKimiPrintSession` | `status:ok`，可读 title，原生 session ID，`cleanupConfirmed:true`，`inferenceDispatched:false` |
| 准备 runtime 关闭后启动全新 runtime | `/sessions` 可列出同一 ID、title、cwd 与 `flywheel_print_invocation` |
| 重新加载 main 后实际 WS 配置 | model 为 `fixture-model`，permission 为 `auto`，plan 为 false，status 为 idle；请求前默认是 manual／plan=true |
| 重新加载前的 REST model | 空字符串，来自未加载 agent 的摘要占位；WS 恢复 main 后同一 REST 返回 `fixture-model`，不能把摘要占位误认为配置丢失 |
| 实际历史和队列 | messages 为空、`has_more:false`，active 为 null、queued 为空 |
| 模型／标题请求 | 本地模型服务器请求数为 0；没有 POST prompts 或 title/generate |
| 第二次 runtime 清理 | exited 与 processesConfirmed 均为 true；没有依赖仅收到 shutdown ACK 的假成功 |

因此这轮证明了真实发布包的具名原生历史创建、关闭后列表可见、配置持久化及无推理
恢复加载。它不证明真实模型推理完成、桌面 UI 已手工验收，或任意用户客户端使用了
同一数据目录。

## Kimi：原生界面可见性与 native 模式边界

[官方会话说明][kimi-sessions] 与 [数据目录说明][kimi-data] 指明历史存储于
`KIMI_CODE_HOME`（默认 `~/.kimi-code`），CLI 的 session picker、`/sessions`、
`/resume` 和 `kimi --session <id>` 使用原生历史。Flywheel 的公开 create／resume
沿用这个 home，停止自有 runtime 不删除或归档历史。上述真实进程测试已确认新
runtime 可以从该存储列出准备好的会话。

客户端必须使用同一 home／机器，工作区筛选也必须匹配 session 的物理 cwd。
native 角色 worktree 或 Tower candidate 可能归入独立工作区；原生子代理属于根
会话内的 agents，而不是必须单列成新的顶层 session。这些是可见性范围，不应把
另一个 home 中不存在记录误报为 Flywheel 没有持久化。

**实际 2.5.3 的 native Agent／Swarm／Tower 仍不满足所需执行协议。** 它没有
`GET /api/v1/sessions/{session_id}/agents/{agent_id}/turns`，也没有 native
适配器依赖的扁平 `subscribe`／`response`、turn／item 实时流；其 AsyncAPI 为
`client_hello`、`subscribe_v2`、ack 和 transcript。实际 capability 检查因此返回
`UNSUPPORTED_SCHEMA`，对应 fixture 验证在会话创建／prompt 前失败。仅为 print
读取一次 v2 main-agent 状态，并未宣称实现完整 v2 native 执行、计量与清理协议。

较新官方固定源码 `a692ff9af2c68157297a324b013a8960ceae66bb` 中的
[conversation route][kimi-conversation]、[WS contract][kimi-ws] 和
[session create][kimi-create] 已包含 native 所需的接口及创建配置应用。
本轮仅核验该源码能力；不能将它等同于某个用户已安装的 2.6.2 包，更不能由版本号
跳过安装现场的 schema／实际 effective-state 准入。

## Kimi：回归与独立 review

| 检查 | 结果与边界 |
| --- | --- |
| `node --test plugins/dsh/test/kimi-native.test.js plugins/dsh/test/kimi-session-title.test.js plugins/dsh/test/kimi-conversation-retention.test.js` | 64 个测试全部通过，0 失败／跳过；真实本地 HTTP／WS 子进程，模型对端为 fixture |
| 最后一次订阅 ACK 数组校验后，重跑 print initialization／发布语义／传输失败专项 | 10 个相关测试通过，0 失败／跳过；与前项重叠，不合计为 74 个不同测试 |
| 精确发布包的手动验证脚本 | 实际 Kimi 2.5.3 执行成功，0 模型请求，关闭后可见且配置恢复正确 |
| 独立代码 review | 另一审查者检查 fresh-only 修改边界、外来／resume owner 拒绝、未准入端点清理、actual 2.5.3 证据和 native 能力拒绝；没有剩余阻塞发现 |
| 本节没有覆盖 | 真实账号／付费推理、桌面 UI、最新源码构建包的 native 模型调用、其他 Provider 的验收和整仓门禁 |

## Flywheel：持久化、复用与展示分别保存

原来的单个 `persistent` 判断同时决定 native 是否保存历史、是否请求旧上下文，以及是否
保留可见 ID。它使 Pro 的 fresh 变成 ephemeral，并把 DSH／Kimi 已保存历史的 ID 从角色
记录中丢掉。修复后：

- Pro 主任务 fresh／scoped 都请求 native persistent；fresh 每次不传旧 ID，明确保存
  `configuration.sessionMode:fresh`，不能成为自动或受管人工复用候选。
- 只读 recovery 使用原调用保存的 `Last.Persistent`、requested thread 与 lifecycle 身份。
  旧 ephemeral 记录不被重新解释；unknown／未确认 cleanup 仍先阻断。
- DSH／Kimi 不改变原保存协议，保留 requestId 匹配的实际 ID 供查看。出现 ID 不证明任务完成。
- 最近回复先要求 state 已确认 receipt digest，再核验实际回执。旧 fresh 记录可从同一 invocation
  的回执补充只读 ID，不回写 state、不使用无归属的顶层 legacy ID。
- UI 同时核对 session key 与 invocation ID，拒绝同 scope 上一次调用的缓存内容。原生命令优先
  使用 `workspace-cwd`，旧记录回退到原 cwd；UUID、引号、运行状态、终态与 cleanup 均有约束。
  仅显示命令，不自动打开客户端或发送提示词。
- Intake／Garden 原来继承 `PersistentSession:false` 零值，并非用户选择 ephemeral。Pro 辅助
  调用现在保留各自独立的新历史；返回 ID 不进入下一次调用或安全重试，不新增角色账本。
  DSH／Kimi 的保存协议不变，四个支持命名的后端都获得角色／任务目录标题，标题不含建议 prompt。

主任务回归包括两种策略下的两次调用、distinct fresh ID／scoped reuse、精确 lifecycle flags、
当前与旧 ephemeral 的中断恢复、foreign invocation、缺失旧 configuration、manual claim 拒绝及
unknown replay guard。辅助模块覆盖两个角色、六类 harness、独立重试、未知／取消／清理失败与
一次计量。独立审查者分别复审 Go、模块、客户端和双语契约，没有剩余阻塞发现。

## DSH SDK：真实公开包保存与 Host 重启读取

`plugins/dsh/test/workspaces.test.js` 使用精确已发布的 DSH `0.2.2-rc.3` SDK／AgentLoop／
JSONL persistence／Workspace／title 包。仅模型与 shell 对端为离线 fixture：

1. 独立 SDK 进程创建项目 header 与实际 worktree 分离的会话，写入可读双语 title。
2. 正常结束并确认 cleanup；通过公开 `sessionPersistence.open(id, 'read')`／`read()` 和
   `foldSessionTitle` 读回同一 title 与 user 来源。
3. Host 通过公开 Workspace 服务附加会话，不为执行 worktree 建立新项目。
4. dispose 并重建 Host persistence 服务，仍读回同一 title 和项目 membership。

这证明被测共享存储链路可供 desktop Host 发现并读取；不等于真实 DSH 图形界面手工验收。
相同 `DSH_HOME`、实际 persistence root 与项目归属仍是前提。

## Codex Pro 0.10.2：修复 Host 委派丢失持久化配置

[配套 PR #9](https://github.com/ckanner/dsh-subagent-codex-pro/pull/9) 的生产根因是
`CodexExecProvider.start` 没有把插件已经选择的 `threadMode` 放进 native run spec。
因此配置为 persistent 或 session 的通用 Host 委派仍启动 ephemeral thread。新回归在原实现
观察到两次 `ephemeral:true`，修复后两次分别新建具名 persistent thread，并转发已有 originator
配置。Host 没有 resume 协议，不推断旧 ID；独立工具的 session 复用、逐次 ephemeral 覆盖和
专用 read-only review 的 ephemeral 行为均保留。

| 身份／检查 | 结果 |
| --- | --- |
| 精确源码 | `18079d8b8c751f5b50375473a96960eeb68b61be`；tree `b7c2e312c66dffb9b4aff6ce8c1b523cf5a3e7fe` |
| 候选 archive SHA-256 | `bd66d165ff314c3259af12aaba85e97bec407bff16efcc1109c0b49bd4baa04a` |
| typecheck／build／diff check | 全部通过 |
| 全量插件测试 | 300 passed、0 failed；5 项显式 live-model tests 未执行 |
| 真实 npm pack/install | 通过；安装后的公开 Host export 覆盖四种 lifetime 配置、两次不同 ID、命名与 originator；独立 machine 无 optional peer 检查先执行 |
| 独立 review／远端 CI | 无阻断发现；[CI #49](https://github.com/ckanner/dsh-subagent-codex-pro/actions/runs/44037605646) 成功 |

另据固定上游 `e5839cbc591df02408d5009e287887c7d57a9167`，实际 `codex app-server`
入口使用 VSCode SessionSource，满足默认交互式来源过滤；originator 是独立字段，不是所有
客户端都通用的显示保证。本地 originators 过滤在该版本不受支持。使用相同 CODEX_HOME 与
匹配项目仍可能受 Provider、账号和客户端筛选影响；空 thread 也可能尚无 resumable rollout。
本轮真实 native Codex 无推理探测被受管环境在派发前拒绝，未绕过限制，不能算 native／Desktop
验收通过。完整源码依据与发现规则见配套 PR 的双语 README。

## Claude Code Pro 0.5.3：拒绝已知的历史关闭冲突

[配套 PR #9](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/9) 修复实际转发的
`CLAUDE_CODE_SKIP_PROMPT_HISTORY` 与 persistent 承诺冲突。官方 Claude Code 2.1.325
二进制 SHA-256 为 `0dc4f4a9a165fc60746d7ae8489712e5fc352f1e7ad4eb22e7141bd0812eb0cf`。
使用隔离 home／项目与仅 loopback 的 Anthropic SSE fixture，两次实际 CLI 运行都成功退出，
但只有未启用 opt-out 的运行保存了 UUID transcript；没有真实账号或付费模型请求。

新建、显式 resume、DSH query 与交互 Team 现在在 CLI probe／任务／tmux 启动前拒绝这个
冲突，保留 not-dispatched／no-process cleanup。判断精确遵循 native 的 `1/true/yes/on` 真值，
显式 ephemeral、环境原值不变，诊断不包含变量值或凭证。内部 sessionMode 与 argv 分离，
合法 title 等于 `--no-session-persistence` 也不能绕过 guard。

| 身份／检查 | 结果 |
| --- | --- |
| 精确源码 | `bd748d8a4dae93f2ed090e2a4edff7dd6fda431e`；tree `8361ec80821219acb01bc3744b98e24aedb9626c` |
| 候选 archive SHA-256 | `57bf53268324ba6d81e2a44e72cd56043ebe8076b96ada1bd134ae18f459eaa9` |
| 全量插件测试 | 181 passed，0 failed／skipped |
| 真实 npm pack/install／Host 契约 | 均通过；8 组 DSH schema／PTC 组合通过 |
| 独立 review／远端 CI | 无阻断发现；[CI #34](https://github.com/ckanner/dsh-subagent-claude-code-pro/actions/runs/44035891678) 成功 |

Flywheel 的 Go allowlist 原本不转发该变量；配套修复也覆盖直接 machine／DSH 调用者。
它不证明任何可能的存储失败或 CLI 内部随后加载的设置。官方 [session rules][claude-sessions]
排除普通 picker 中的 print／SDK 历史；使用根 UUID 显式 resume，或在满足版本、平台、订阅、
交互终端及停稳条件时 [handoff 到 Desktop][claude-desktop]。不修改 transcript 类型以伪造
交互会话，也不声称本轮完成真实 Desktop UI 验收。

## 整仓回归与交付

环境：Linux，Go 1.27.4，Node 24.22.1，npm 11.9.0。Flywheel 没有变更发布版本，
Codex／Claude 都使用上述实际候选 archive；各摘要与 integrity 记录在 vendor JSON 和 lockfile。
源代码候选不等于 registry 已发布，旧 ephemeral 历史不能事后重建；旧未知调用仍需使用
原 adapter 身份对账。没有合并、发布或改动用户真实 desktop／SDK profile。

| 本地检查 | 已完成结果 |
| --- | --- |
| `go build ./...`／`make build` | 通过 |
| `make quality-gate` | vet、fmt、全部 Go 测试通过；总覆盖率与每个函数均为 100% |
| 七个核心包 `go test -race` | machine、verify、watchdog、drivers、cli、modules、diagnostic 全部通过 |
| `npm --prefix plugins/dsh run check` | 537 plugin tests + 10 legacy-host tests 全部通过，0 failed／skipped |
| 根目录 `npm test` | 166 tests 全部通过，0 failed／skipped |
| `test:sessions` | 36 条真实引擎／公开 adapter 配对 trial，96 次离线 provider 调用；六场景全部观察到，独立 oracle 全部通过 |
| 完整 Go → Node → SDK／Pro 包离线桥接 | 9 个顶层测试组、100 个子用例全部通过，含 Kimi identity 不可重试、命名与四 Provider worker／活动链；101.153 秒 |
| `npm run build:npm` | 同一个 0.36.1 候选包含四个平台引擎，构建成功 |
| 实际候选 `npm run test:install` | 10 个 packed-host tests + 15 个安装 tests 全部通过，0 failed／skipped；显式提供 Claude 0.5.3 实际 tarball，optional 后端用例实际执行 |

完整桥接使用 `FLYWHEEL_TEST_NODE_BRIDGE=1`，测试正则包括四个
`NodeWorkerOfflineBridge`、三个 `ProviderActivityOfflineBridge`，以及
`KimiPrintSessionTitleOfflineBridge` 和 `KimiPrintIdentityFailureIsNotRetryableOfflineBridge`。
实际安装验证覆盖打包后的公开 DSH Host、Go watchdog 取消、DSH SDK、Codex app-server
离线协议对端、Claude source tarball、重复安装及已有 split 安装迁移。

本地候选身份：

- 文件：`ckanner-flywheel-0.36.1.tgz`，20,070,559 bytes。
- integrity：`sha512-mfxvoxkPQ3nwdMnU+XyALx/lp89K4TD7lMqzZr2tcrG8rCTmlr/5rBVYJqOCwnB9EZAjF1tyYD3zOhTKgNEU6A==`。
- 隔离源码快照 SHA-256：`76d4b83e2bfd5b0e61513b1e8593a4644868939d762fa05b5e6464f10a2fefcc`。
- 构建时父提交为 `cb1edcd90cd081e3e457407d9234f577acaaed87`，候选清单明确记录
  `dirty:true`；快照包含本轮最终生产代码、测试、候选 Provider 包与当时的文档。
- 构建与安装使用同一 archive；冻结后仅在本验证文档补充最终结果，不修改生产代码。
  这是开发验证候选，未执行 clean release、打 tag、registry 发布或客户端升级。

最终提交的远端 CI 结果另回复到 [Flywheel PR #55](https://github.com/ckanner/flywheel/pull/55)，
明确关联实际 head；旧提交 CI 不作为新 head 的通过证据。

[claude-sessions]: https://code.claude.com/docs/en/sessions#where-the-session-picker-looks
[claude-desktop]: https://code.claude.com/docs/en/desktop#coming-from-the-cli

[kimi253]: https://registry.npmjs.org/@moonshot-ai/kimi-code/-/kimi-code-2.5.3.tgz
[kimi-current]: https://github.com/MoonshotAI/kimi-code/tree/a692ff9af2c68157297a324b013a8960ceae66bb
[kimi-sessions]: https://github.com/MoonshotAI/kimi-code/blob/a692ff9af2c68157297a324b013a8960ceae66bb/docs/en/guides/sessions.md
[kimi-data]: https://github.com/MoonshotAI/kimi-code/blob/a692ff9af2c68157297a324b013a8960ceae66bb/docs/en/configuration/data-locations.md
[kimi-conversation]: https://github.com/MoonshotAI/kimi-code/blob/a692ff9af2c68157297a324b013a8960ceae66bb/packages/kap-server/src/routes/conversation.ts
[kimi-ws]: https://github.com/MoonshotAI/kimi-code/blob/a692ff9af2c68157297a324b013a8960ceae66bb/packages/kap-server/src/protocol/ws-control.ts
[kimi-create]: https://github.com/MoonshotAI/kimi-code/blob/a692ff9af2c68157297a324b013a8960ceae66bb/packages/kap-server/src/routes/sessions.ts
