# Kimi 原生运行协议

[English](KIMI_NATIVE_PROTOCOL.en.md)

本协议由 `kimi-native-worker.js` / `runKimiWorker()` 实现，供 Go 引擎选择 Kimi 原生 Agent、Swarm 或 Tower。普通 Kimi CLI 路径不受影响。Go 继续拥有角色循环、预算、边界检查、Review、提交和验收；原生子任务属于本次角色调用。

## 入口与能力准入

Worker 从 stdin 接收一个 UTF-8 JSONL `start`，父进程继续保持 stdin 打开。EOF、SIGINT、SIGTERM、SIGHUP 都请求取消。stdout 仅有 protocol 1 的 `ready` / `event` / 最后一个 `result`。桥接函数只 emit ready/event，返回 result，由 CLI 输出一次。单个请求上限 1 MiB，单个输出帧上限 2 MiB。

```js
{
  type: 'start', protocol: 1, requestId: 'invocation-id',
  cwd: '/physical/role-worktree', workspaceCwd: '/physical/project',
  model: 'configured-model-alias', reasoningEffort: 'high',
  permissionMode: 'danger-full-access', prompt: 'Role instructions', timeoutMs: 600000,
  persistent: true, // sessionId 仅在恢复 Agent/Swarm 历史时传入
  native: {
    version: 1, mode: 'agent', // agent | swarm | tower
    agents: { enabled: true },
    agentProfile: 'agent' // 可选；Server 已发现的 profile 名称，不是 agent-file 路径
    // tower: { branch: 'candidate-branch' } // 仅 tower 可用，branch 可省略
  },
  managed: {
    version: 1, runId: '', invocationId: 'invocation-id',
    role: 'execute', scope: 'packet-scope', candidateId: 'candidate-id',
    channel: 'subscription', taskDir: '/physical/project/docs/task'
  }
}
```

`invocationId` 必须等于 `requestId`；`runId` 是必需的字符串，可以为空。未知 Start/native/managed 字段、非显式完整权限、无效目录、Tower sessionId，以及 `agents.maxConcurrent` 均拒绝。`maxConcurrent` 拒绝的原因是当前公开 API 不能保证整棵原生任务树的硬上限。Swarm/Tower 不接受 `agents.enabled:false`。

每次调用启动前台 `kimi web --no-open --host 127.0.0.1 --port 0`。`FLYWHEEL_KIMI_BIN` 可以指定 CLI。登录和模型配置沿用 `KIMI_CODE_HOME`（否则 `~/.kimi-code`），不重写用户配置。Worker 从真实实例注册表核对本次 child PID、启动时间、loopback 端口，再使用已有 `server.token` 验证 `/meta` 的 server_id、认证、backend 和 WebSocket 支持。带 bearer token 的 CLI banner/stdout/stderr 不转发、不写入 receipt。

准入读取并哈希实际 `/openapi.json` 和 `/asyncapi.json`，核对所需创建/配置/队列/环境/历史/关闭路由和订阅/任务生命周期消息。`:abort` 在上游 OpenAPI 中未完整投影，因此对尚未 dispatch 的本次空闲会话做实际幂等探测。最终还必须由实际 WS agent 状态确认 model、thinking、permission、plan/swarm/tower 标志，以及 REST local environment/cwd。只有这些事实吻合，`native.effective.matched` 才为 true。

`--doctor` 仅检查 CLI 可执行性及版本，返回 `inferenceTested:false`。它不能证明模式、账号、模型或 API 准入。旧的 `kimi server` 命令、SDK、named preset RPC 和任意 agent-file 加载均不是此接口。

## 执行、完成、取消

通过 WS 先订阅 `main`，以 `prompt_id` 关联实际 `user_message.prompt_ids` 和 HTTP 接受结果；两者到达顺序不限。首次 ordinal 是 **0**，新会话的历史基线是 **-1**。随后从 task/agent_refs 发现并订阅子 agent。子任务、队列、交互和整个会话 busy 状态共同决定完成；首个主 turn 结束不足以成功。Tower 的自动主 agent continuation 也必须结束。

`native.completion={confirmed,pendingIds}` 在 cleanup **之前**记录。成功必须先看到全部主 turn 完成、已观察子任务和 agent 空闲、队列为空、没有待回答交互，且最后的 REST/WS 对账一致。关闭阶段取消了子任务，不能将未完成的结果改成成功。receipt 保留实际 task/child ID、状态和模型。

未预期的 approval/question 返回 `INTERACTION_REQUIRED`，不编造回答。断线、重新投影 conversation、未知 dispatch 不重发 prompt。取消先撤回 queued prompts，再中止主 turn、停止已观察任务并等待空闲；关闭 remote control 和本次 owned runtime 后，核对进程退出及已观察后代进程身份。所有清理受独立、有界 grace 约束。无法确认队列/子任务/进程已收束时，`settled` 与 `cleanup.confirmed` 都为 false。

Worker 继承 Go 的进程组，不创建逃逸的独立进程组。Go watchdog 继续拥有最终进程组清扫。receipt 的 `settled` 表示清理已确认；它不等于 role 成功，error/cancelled 也可 settled=true。

## 结果、用量与恢复

最终字段为 `protocol/requestId/type/status/settled/sessionId/text/usage/costUsd/native/lifecycle`，失败另含 `error.code/message`。`status` 为 ok/error/cancelled；`native.requested` 原样保留 Start.native，effective/observed 则记录实际准入事实、schema 摘要、会话绑定、当前 managed 身份、子任务、completion、cleanup 和 Tower candidate。

`lifecycle.protocol` 为 `kimi-server/v1`，携带 requestId/sessionId、`dispatch`（not-dispatched/dispatched/unknown）、`outcome`（completed/failed/cancelled/unknown）、cleanupConfirmed、resumeCandidate、resumeAccepted、usageStatus 和 turnId。

用量只从公开 conversation 中实际结束的 model step 四项计数得到，完整时输出 `{input,output,cacheRead,cacheWrite,totalTokens}`；input 包含 input_other 与两种 cache input。`session.usage` 的占位零和 WS `context_tokens` 不作为本次账单。缺失步骤、未识别 child、compaction 或无法划定恢复 child 的历史边界时，顶层 usage=null；保留可归属的部分步骤事实。Tower 整树用量目前始终 unknown，costUsd 始终 null。未知值交给 Go 的既有预算停机策略。

Agent/Swarm 的持久历史绑定 taskDir、role、scope、channel、物理 cwd/workspace、model、effort、permission 和 native profile。runId/invocationId/candidateId 记入本次 receipt，不阻止同一角色在重启或返修后恢复。恢复历史只统计本次主 turn；复用旧 child 历史时不再次计入全部旧费用。不同角色或配置拒绝复用。

Tower 总是 fresh：显式 sessionId 拒绝；persistent=true 且无 sessionId 可以执行，但 resumeCandidate=false。本接口没有 durable recovery journal/ACK：`recover` 不受支持，不能用新 start 重跑一个 unknown invocation。

## Tower 候选与导回

Tower 收到独立 `.git` 的仓库副本，不复用主仓 Git 管理目录；linked worktree 同样先转成独立仓库。源 HEAD、所有 tracked/unignored 内容、已有 dirty/untracked、删除、二进制、可执行位与符号链接都纳入快照。taskDir 在角色 worktree 外部时，另行哈希外部任务契约；该外部目录只接受普通文件与目录。源和外部契约合计最多 256 MiB；submodule/特殊文件拒绝。

只排除真正该任务的运行文件：state.json 及其原子临时文件、metrics.jsonl、orchestrator.log/pid、锁、stop-request、`.control/`、`.sessions/`、`reports/native/`；spec、packet 等保留在指纹中。Tower 自己的 `.tower/` 管理数据不导回。

入口将 cwd 解析为物理路径，同时保留调用者声明的目录别名。提交给模型的 prompt 会把这两种有路径边界的 source cwd 前缀映射到 candidate cwd，并添加候选目录说明；只扫描原始文本，不再次替换插入的候选路径。相似路径、URL（包括 query）、外部契约路径不替换。receipt 的 `observed.promptMapping` 记录规则、替换次数、原始／提交 prompt 摘要，以及声明、物理和候选目录。**这是独立 Git 候选工作目录，不是 OS 文件系统沙箱**：完整访问权限不因此变小；源内容或 HEAD 变化会使导回拒绝。

只有 native completion 成功、cleanup confirmed 且 `managed.role==='execute'`，可信 adapter 才冻结 binary patch，再次精确核对源和外部契约、检查 patch 摘要、执行 `git apply --check`、二次快照检查、实际 apply 和结果一致性检查。非 Execute 仅交回候选。冲突、契约/HEAD 变化、patch 被改或后置条件不符会使 role 失败，并保留私有候选、源快照、patch 和 receipt。没有针对无关编辑器的全系统文件锁，也不宣称多文件 apply 是 OS 原子事务；Go 仍负责后续边界检查、验收和提交。

## 已验证证据

`test/kimi-native.test.js` 使用真实本地 HTTP/WS 子进程验证三种模式、事件顺序、原生子任务、用量未知值、准入拒绝、取消、历史恢复、CLI lifeline、一次 final、token 不泄漏及 Tower 失败门禁；`test/kimi-tower-candidate.test.js` 使用实际 Git 验证 dirty/untracked/binary/link/mode/deletion、linked worktree、外部契约、候选冲突和 prompt 路径映射。测试不联系用户账号或模型；CLI 可执行性、真实账号准入和付费推理验证是不同的证据。

实现依据是上游 `MoonshotAI/kimi-code` 提交 `f082f04a093a3e71a2263c5461d52507dc31f83a` 的 [Server API](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api.md)、[会话配置](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-sessions.md)、[conversation/task 生命周期](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-conversation.md)、[Tower](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/guides/tower.md)、[实例注册表](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/kap-server/src/instanceRegistry.ts)、[逐 batch Swarm 并发](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/agent-core-v2/src/features/swarm/session/agentRunBatch.ts) 和 [Tower 限流服务](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/packages/agent-core-v2/src/features/tower/towerRateLimitService.ts)。安装现场仍以实际 API 探测为准。
