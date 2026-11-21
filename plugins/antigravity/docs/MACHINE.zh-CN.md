# Machine API，协议 1

[English](MACHINE.md) · [插件指南](../README.zh-CN.md)

公开导入为 `dsh-subagent-antigravity/machine`，不依赖 DSH 运行时 peer。`lib/machine.d.ts` 定义导出的 TypeScript 契约。插件直接使用官方 `agy` NDJSON 协议；适配器自己的包装命令、lifecycle 和 recovery envelope 与原生协议分开。

## 导出和能力

导出 `MACHINE_PROTOCOL_VERSION`、`MACHINE_LIFECYCLE_VERSION`、`MACHINE_ACTIVITY_VERSION`、`MACHINE_CAPABILITIES`、`MACHINE_LIFECYCLE_CAPABILITIES`、`MACHINE_ACTIVITY_CAPABILITIES`、`MachineInputError`、`parseMachineRequest`、`runMachine`、`recoverMachine`、`doctor`。

支持文本任务、结构化输出、精确续跑、拥有的进程组监督、真实空工具准入、固定 Orchestrator 和显式原生协作。协作请求与子成员观察分别记录。不提供原生附件、预热、全账号子任务查询/取消、原生改名或整树硬预算。

## 请求字段

| 字段 | 含义 |
| --- | --- |
| `protocol`、`requestId` | 协议 1、调用方身份，ID 最多 256 bytes，可编码 task/stage/role/attempt。 |
| `cwd` | 已存在的绝对目录，经 realpath 规范化；所有进程使用此目录。 |
| `command` | 默认 `agy`，只能是程序名或可执行文件路径，不能是 shell 源码。 |
| `prompt` | 一条非空 UTF-8 文本，最多 1 MiB，没有附件传输。 |
| `model`、`effort` | 普通 Gemini 4 Hydrogen/Neon/Carbon/Argon 的精确 slug 与 effort 后缀匹配；Carbon/Argon 不允许 xhigh。固定 Orchestrator 为 `gemini-pro-orchestrator-neon`、`gemini-pro-orchestrator`（Carbon）、`gemini-pro-orchestrator-argon`，使用 `effort:""`；省略会规范为 `""`，不传原生 `--effort`。必须出现在实际 `agy models` 目录。 |
| `executionMode` | 显式 `default`、`accept-edits` 或 `plan`；default 传 `--mode ''`，不默默继承 auto。 |
| `permissionMode` | 显式 `request-review` 或 `always-proceed`；后者增加官方 `--dangerously-skip-permissions`，不是 OS 隔离证明。 |
| `sandbox` | 显式 boolean。true 请求 `--sandbox`；false 要求原生终端沙箱设置也为 false。init 不证明实际 OS 隔离。 |
| `toolPolicy` | 默认 `native`；`evidence-only` 要求受管 custom agent，以及派发前真实空 `init.tools`。 |
| `native` | 可选 `{version:1,mode:"native"\|"subagents"\|"swarm",maxObservedAgents?:1..8}`。上限统计已观察子成员总数，不是并发硬上限；省略保持旧请求结构。 |
| `sessionMode`、`sessionId` | `new` 不提供 ID；`resume` 提供已确认归适配器所有的精确 UUID。 |
| `sessionName` | 可选单行本地展示名称，不冒称原生标题已写入。 |
| `timeoutMs` | 总期限，默认 600000，0 不限；包括预检、启动、运行和退出，同时传原生 print timeout。没有首 token 空闲超时。 |
| `disposeGraceMs` | TERM 等待，默认 2000，范围 10–30000，必要时再 KILL。 |
| `maxFrameBytes` | 单个 NDJSON 帧/未完成帧缓冲上限，默认 4 MiB。 |
| `maxOutputBytes` | 最终 response 留存上限，默认 1 MiB；已消费的进度不占最终结果容量。 |
| `outputSchema` | 可选 JSON Schema 2020-12，在派发前和收到 structured_output 后分别验证。 |
| `subscriptionPolicy` | 必须是 `{version:1,mode:"subscription-only"}`，不提供 API/cloud/credits 回退。 |

未知字段、模型/effort 冲突、非法 sessionId、权限或 schema 在调用前拒绝。evidence-only 在任何 CLI 命令前拒绝 Orchestrator 和 `subagents` / `swarm`。

`runtime.env` 若提供，就是完整环境，不借用 process.env 中缺失的值；否则使用过滤后的 process.env。HOME 应绑定同一个原生登录/配置环境。适配器只读配置 JSON，不打开 token profiles 或 keyring。API key、token/secret、替代 provider 路由与 Node 注入变量被过滤；显式 ADC 认证被拒绝。

默认 journal 在 `$HOME/.local/state/dsh-subagent-antigravity/runs/<runId>` 私有目录。也可指定 `runtime.recoveryDirectory`，必须是已存在、规范化、归当前用户所有、0700 父目录下的未使用路径。每个 receipt 带有可直接用于恢复的 `recovery` 请求。

## 原生调用与准入

同一 cwd/环境中检查 `agy --version`、`--help`、`models` 和本地配置。普通调用要求 1.5.7+；Orchestrator 和显式协作以 1.5.12+ 为支持基线。普通可调模型命令为：

```text
agy --input-format stream-json --output-format stream-json
    --model <exact slug> --effort <effort> --mode <mode>
    --print-timeout <duration> --disable-slash-commands
```

续跑、沙箱、预批准工具、JSON schema 和 evidence-only agent 采用对应官方参数。prompt 不放 argv 或 shell 源码，只在 stdin 写一行：

```json
{"event":"user","message":{"content":"the supplied prompt"}}
```

之后关闭 stdin，并持续消费 stdout/stderr，等待根结果、物理退出和本地清理。stdin 关闭或 exit 0 不代表任务成功。

原生事件为 `init`、`step_update`、`result`。init 必须匹配真实 cwd、model、permission_mode；续跑还须匹配 UUID。原生错误可能在 init 前用空 conversation ID 返回。SUCCESS、ERROR、CANCELED、INTERRUPTED、INVALID 是终态；WAITING/RUNNING 不能算成功。非空 `result.finished_reason` 一律 `native-cutoff`，包括未来未知字符串，保存在 `execution.finishedReason`。重复结果、非法/半截 JSON、身份不符、超量帧/输出、soft denial、schema 失败或非零退出都失败。

`configuration.matched` 只证明可观察字段一致。effort、executionMode、sandbox 未独立回显时为 null；`session.nativeNameStatus` 为 unknown。

evidence-only 为本调用独占创建 custom agent，用官方 `--agent` 选择；实际 init 必须报告此 agent 和空的全部可用工具，才能写 prompt。没有这些证据就等待到期限并失败，不回退到较弱策略。受管 YAML 只在 inode/内容仍匹配时删除，父进程死亡也沿用该清理路径；无法确认删除会失败。工作区 custom agent 不等于账号设置，也不构成 OS 沙箱。

## 显式协作

原生模式与模型独立。选择 Orchestrator 表示其固定规划/实施交接，不自动推断创建了子智能体或 Swarm。

| 模式 | 派发和准入 |
| --- | --- |
| `native` 或省略 | 原文传 prompt，保留 `--disable-slash-commands`。原生仍可自主调用工具/子智能体，不承诺单智能体。 |
| `subagents` | 等待匹配 init，且可用工具包含 `invoke_subagent`，才发送委派/汇总指令和 JSON 任务。继续禁用 slash 展开。 |
| `swarm` | 等待匹配 init，由适配器生成唯一 `/swarm` 前缀，再附汇总指令和 JSON 任务。只有此模式省略 `--disable-slash-commands`。不编造 Swarm 专用工具名，也不假设必须有普通 `invoke_subagent`。 |

JSON 任务中的所有 `/` 转成 `\u002f`，用户文本不能在命令前缀中增加 slash command 或 skill。已有 `disableSlashCommands:true` 是 Swarm 的派发前冲突，该设置进入配置指纹；中途漂移会失败。适配器不修改设置，不开放任意 command 字符串。

显式协作要求真实 `step_update.subagent_info.subagents`。根调用正常结束却没有子成员时，返回 `collaboration-not-observed`；同时 `native.completion.state` 仍可如实写当前 CLI invocation 已完成。Swarm 的 `dispatch.method:"slash-command"` 记录请求的传输方式，不证明命令展开、board、完整 roster 或 worker 模型。

子 ID 必须为 UUID、不能等于根会话，忽略大小写去重。`maxObservedAgents` 按整个 invocation 累积唯一子成员，包含 worker/verifier/monitor；超过后停止拥有的 CLI 进程组，失败类别为 `observed-agent-limit`。固定 `native.limit.hard:false`，不保证原生并发、未报告的成员、嵌套层级或总额度。

记录 `id`、`kind`、`role`、`logUri`、`workspaceUris`；URI 只作为元数据保存，不打开或拉取。父 tool step 的 DONE 或根 SUCCESS 不改变子成员 `state:"unknown"`、`model:null`。稀疏更新可补元数据，不重复计数。跨帧/更新累计 child JSON 留存预算为 1 MiB，并在超过 1000 个记录时拒绝。超量立即失败：保留已验证的所有 ID（包括触发上限的 ID），降为 unknown/null/[] 最小记录并写 `native.metadataTruncated:true`。只有该 failed/truncated 回执可保留 1001 个 ID；后续帧只消费，不再记录工作。`observedAgents` 始终等于 `children.length`，不把截断数据返回成功或压低已观察人数。

回执相关字段示例：

```json
{
  "mode": "swarm",
  "dispatch": { "method": "slash-command", "command": "/swarm" },
  "observedAgents": 2,
  "collaborationObserved": true,
  "limit": { "maxObservedAgents": 4, "hard": false, "exceeded": false },
  "completion": {
    "scope": "cli-invocation", "state": "completed",
    "source": "normal-result-and-cli-exit", "children": "unknown"
  },
  "metadataTruncated": false
}
```

正常 SUCCESS、无非空 finished_reason、有效流和 exit 0 才得到 `normal-result-and-cli-exit`。范围只包括当前 CLI invocation，子成员终态与整树用量仍独立未知。不读原生 board，不另起后台 watcher，不取消其他会话。原生 verifier 属于本根调用，不能替代外层流程独立正式 Review。

## 生命周期、所有权和恢复

`runMachine` 创建与父调用方有 IPC 联系的 detached supervisor；它拥有一个 detached CLI 进程组、私有 journal 和输出流。调用方消失后取消本组，不按进程名清理。先 TERM、有界等待、必要时 KILL；仍存在或僵尸组不能冒称已消失。

lifecycle 1 含单调序号、requestId/runId、已知 threadId、`turnId:null`、配置和派发/结果/清理事实：

1. `dispatch-intent`：本地准入和准备完成，任务尚不能发送。
2. `dispatch-possible`：stdin 可接受任务之前持久化；必须等待调用方 `onLifecycle` 完成 ACK。
3. `terminal`：本地 invocation 到达适配器最终检查点；原生终态/结果须另查 receipt。
4. `cleanup-confirmed`：本地拥有资源清理确认，可在缺少原生终态时出现。

每个 lifecycle 都原子写入并 fsync 后才通知调用方。ACK 错误/超时阻断派发；父 IPC 断开仍取消并写原 journal。进度/活动回调是观察性的，不能把失败变成功，也不制造心跳。

`execution.dispatch` 表示 prompt 是否释放；`turnDispatch` 分 not-dispatched、possible、accepted。持久化边界崩溃按可能已派发处理；无原生终态即使本地清理确认，providerWork 仍 unknown。child 活动携带可选 native 快照，DSH status 可在运行中显示成员。根结束或本地组退出不证明整棵远端/daemon/native 树结束。

session lease 绑定规范 cwd、command、model、effort、执行/审批/sandbox/toolPolicy、精确可选 native 对象和原生配置指纹。只允许已终态且清理确认的本插件会话续跑；绑定变化在派发前拒绝。活动/未知 lease 不因进程消失而删除，不允许盲目 resume。调用方还须阻止换 requestId/model/session 对同一未知任务做语义重放；Desktop 以相同 UUID 和请求摘要保证重试为只读。

```js
const recovered = await recoverMachine(receipt.recovery);
```

恢复核对目录/文件所有权、无 symlink、精确 request/run 身份，然后只读原 journal；不会启动 CLI、发 prompt、续跑、查远端任务或 kill。`status:"terminal"` 仅表示最终适配器回执已存在，providerWork 仍可能 unknown。缺失/不可核验证据保持 unknown，恢复本身不授权重放。

## 成功、用量和订阅边界

`ok:true` 同时要求正常原生 SUCCESS、无 finished_reason、有效 schema、可观察配置匹配、无 soft denial/进程失败、本地清理确认。显式协作还要求已观察成员且未超观察上限。回执分别记录请求、观察、本地执行、providerWork、会话、内容、原生元数据、订阅控制、usage 和 cost。

原生 result usage/num_turns/duration 累计整个会话，只有 response 是当前轮。`usage.scope:"conversation"` 不猜增量；续跑时不要把各轮累计计数直接相加当费用。缺失字段为 null，`costUSD` / `reportedCostUSD` 为 null，`costStatus:"unknown"`。

failure 含 stage/category 和有界脱敏 detail。auth/quota/rate-limit 需要对应原生证据，不把一般错误叫额度不足。配置冲突或不支持模型/effort 是 invalid-request；没有终态时保留 unknown 和 replay-unsafe。

本地预检拒绝 API provider、ADC、GCP、自定义 API model、credits 回退，过滤替代路由环境并检查漂移。`verified-local-controls` 只证明这些检查通过，不是账号/商业权益核验。官方 CLI 拥有 keyring 凭据；插件不读取或导出。

官方 `useG1Credits` 默认 false，控制套餐额度耗尽后的 credits 使用。账号权益、余额、外部工具收费无可靠 headless 字段时保持 unknown。unknown 余额不会触发额外付费路由，也不会单独阻断本地控制符合要求的调用。认证/额度失败就终止同一请求，runner 不隐藏重试、新建会话或替代 provider。

## 官方依据与验证范围

2026-11-20 核对的官方来源：

- [Headless](https://antigravity.google/docs/cli/headless/)：stdin/events/init/子元数据、schema、累计计数和权限拒绝。
- [CLI reference](https://antigravity.google/docs/cli/reference/)：参数、slash 展开、权限、沙箱和 credits 设置。
- [Authentication](https://antigravity.google/docs/install/)：官方登录与 API/ADC/GCP 路由。
- [Custom agents](https://antigravity.google/docs/subagents/) 与 [Hooks](https://antigravity.google/docs/hooks/)：自定义工具列表、发现路径和 `invoke_subagent`。
- [Permissions](https://antigravity.google/docs/permissions/)：规则顺序与执行模式/审批/隔离的区别。
- [Models](https://antigravity.google/docs/models) 与 [Swarm](https://antigravity.google/docs/swarm/)：固定模型交接和多成员协作的区别；安装可用性仍查本机 catalog。
- [CLI changelog](https://antigravity.google/docs/changelog?tab=cli)：后台等待、finished_reason 截断、1.5.5 Swarm 与 1.5.6 headless 修正；插件新功能以 1.5.12 为基线。

这些依据和离线测试证明适配器协议实现，不等于真实账号、用户 Mac 或模型质量验收。实测记录与边界见 [VALIDATION.md](VALIDATION.md)。
