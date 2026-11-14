# 原生 Harness、PTC 与 LCU：完整接入设计与实现

[English](2026-11-13-native-harness-capabilities.en.md) · [方案索引](README.md) · [使用指南](../zh-CN/native-capabilities.md) · [当前架构](../design.md)

**状态：六仓库实现候选，随 PR 交付。** 创建于 2026-11-13；2026-11-14 修订为实现设计，Flywheel 候选版本 `0.34.0`。本文描述当前代码、接口和运行边界；验证结果独立记录在[验收记录](../validation/2026-11-14-native-harness-implementation.md)。源码候选、npm 发布、用户 Mac 上的实际账号和桌面验收是不同状态。

[先前提案](https://github.com/ckanner/flywheel/blob/9c95a69490c7f38d39ede055a00adc56bdce087c/docs/plans/2026-11-13-native-harness-capabilities.md)保留在原提交。本修订取消“先限制为少量 helpers，再逐阶段开放”的交付安排，直接提供各原生后端可支持的完整入口，同时保留各后端实际能证明的权限、并发、用量和终态。

## 1. 设计结论

用户继续在 DSH 桌面中使用 `heavy` 或 `daily` Agent Preset 准备需求和操作 Flywheel。正式 loop 由 Go 引擎运行：DSH 路径启动本地 SDK runtime；Codex、Claude 路径直接调用各 Pro 插件的公开 machine API；Kimi 原生模式使用本地 Kimi Server。模型、登录账号和 harness 是不同维度，能力跟随实际 harness，不因模型名称相同而转移。

本实现让同一套 Plan / Execute / Review / Judge / Intake / Garden 绑定通过 `native-profile` 使用能力。JEV 关闭时静态配置照常生效；JEV 开启后只能在既有显式允许集合内选择，不能创造新的后端、账号或原生权限。

| 组成 | 实现结果 | 正式完成依据 |
| --- | --- | --- |
| DSH | 共享生成器的 PTC composition、实际 executor probe、子 Agent、Team、PTC Workflow、显式 Web 工具 | 匹配的 policy 回执、root 结果、子任务终态、完整 journal、SDK 清理 |
| Kimi | Server 模式的 Agent / Swarm / Tower，HTTP + WebSocket 连续观测，Tower 独立 Git 候选 | 所有已归属任务和 continuation 停稳；Tower 另核对源指纹和导回 |
| Codex Pro | app-server 原生 agents controls、子线程事件、用量与恢复；独立原生 review | `config/read` 有效控制、当前 turn 的完成、owned thread 终态、guardian 清理 |
| Claude Code Pro | Agent 定义、Workflow、Advisor；独立交互 Team 和远端 Review | init 工具事实、当前 task-run 身份、完整本地或远端生命周期 |
| LCU | Desktop 与 SDK 两条挂载路径、受管 lineage、跨进程 GUI lease、PTC 对象与媒体、取消与物理退出 | 当前 lease/epoch、实际调用者、turn cleanup、runtime exit、证据归档 |
| ChatGPT provider | DSH attachment 到 Responses image 的真实投影、顺序与 call ID、取消收尾 | 实际存储图片与输入结构；不凭截图存在宣称模型已理解 |

Go 仍独占 `state.json`、冻结契约、预算、packet 调度、候选验证、独立 Review、提交与交付。Team task board、Swarm 内部评审、原生 Review 文本和 GUI 截图均不直接改变状态机。

## 2. DSH 桌面、SDK 和 Preset 的关系

### 2.1 当前确实使用本机 DSH SDK

桌面插件是控制面：接收命令和模型工具、展示工作台、关联用户当前 Session。Go 启动 worker，后者执行本机 `dsh --profile sdk --patch <临时覆盖>`，使用 stdio JSON-RPC 创建或恢复正式角色 Session。`DSH_HOME` 和账号配置可以指向同一份本机授权；无需为了运行 SDK 改用 API Key。

桌面当前 Agent 的工具集、Preset 和私人对话历史不会自动成为正式 loop 的上下文。特别是独立 Review 不继承 Executor，也不继承桌面父会话。`sdk` 是 runtime profile，`heavy` 是 Agent Preset，Flywheel 的 `profiles` 是模型绑定；把 `FLYWHEEL_DSH_PROFILE` 写成 `heavy` 不能选择那个 Agent Preset。

公开 SDK 创建接口仍以全局插件 composition 建立 Agent。本实现采用这个真实接缝，并调用生成器的 `createSdkPtcComposition()`；回执标明 `sdk-global-overlay` 及 composition digest，不伪造“已挂载 named preset”的 Session header。

### 2.2 两个 Preset 与 SDK 共享 PTC 策略

`dsh-preset-generator@0.4.0` 继续从实际 DSH 应用资源生成 `daily` 和 `heavy`。两者都选择 PTC；新增公开 `/composition` 入口供 worker 复用同一策略。静态检查识别旧、新 executor 注册形式并计算 bundle 身份，但不把静态版本或字符串命中作为运行证明。

正式 DSH 调用在首个模型请求前验证：唯一模型工具是 `run_code`；运行语言是 TypeScript；普通工具的 canonical JSON 与对象 `__handle` 保持；多张图片仍能投影；运行时输出协议是 `deferred-context-v1` 或 `inspect-image-v1`。新版还验证 `inspect` / `Image` 的实际对象身份。probe 使用内部无副作用工具和已知图片，不调用模型或操作桌面。

DSH release 与 master 可能同时声明 `0.2.2-rc.3`，PTC 输出协议却不同，因此采用“公开版本范围 + 实际行为准入”。probe 不通过时在派发前报错，保留具体原因；不能通过替换版本号或假工具声明绕过。

### 2.3 能力在 SDK 内按树归属

`native-policy.js` 在公开 Agent 生命周期中准入 root 与真实后代，核对实际 Agent registry、Session parent、provider、model、effort、工作目录和权限。工具开关按请求组合，Team 使用实际 `agent-team` service，Workflow 使用公开 workflow 工具；禁用的工具不能意外出现在 raw schema 中。

本地 wire 子会话事件与公开 `subagent/start`、`subagent/end` journal 共同建立归属。外部 Pro 子代理也记录 provider/run/parent 身份，不能仅因为没有本地 Session 而漏掉。没有归属、缺失开始事件、身份改变、journal 截断均使结果保持失败或未知。

## 3. 配置模型与各 loop

### 3.1 配置是显式能力意图

顶层 `native-profiles` 保存 `{harness, options}`，`options.version` 固定为 1；`templates/native-profiles.schema.json` 是字段信源。`loops`、`routes`、模型 `profiles`、`judge-panel` 与 `execution-policy.bindings` 可引用 `native-profile`。LOAD 阶段拒绝未知字段、未知引用和 harness 不匹配；后端在派发前进一步验证版本、实际工具与控制接口。

省略 `native-profile` 延续普通 harness 路径；继承模型 profile 时按现有显式字段规则展开，显式空字符串可关闭继承的原生配置。配置名称和完整 options 都进入实际调用身份。无关 profile 的修改不废弃已有执行策略；被引用 options 改变会使旧角色历史失去复用资格。

以下是合入完整配置的**片段**，不是完整配置文件，也不会自动修改用户默认路由：

```json
{
  "native-profiles": {
    "dsh-heavy-team": {
      "harness": "dsh",
      "options": {
        "version": 1,
        "preset": "heavy",
        "agents": {"enabled": true, "maxConcurrent": 4, "maxDepth": 2},
        "team": {"enabled": true},
        "workflows": {"enabled": true}
      }
    },
    "kimi-swarm": {
      "harness": "kimi",
      "options": {"version": 1, "mode": "swarm", "agents": {"enabled": true}}
    },
    "kimi-tower": {
      "harness": "kimi",
      "options": {"version": 1, "mode": "tower", "agents": {"enabled": true}}
    },
    "codex-agents": {
      "harness": "codex-pro",
      "options": {"version": 1, "agents": {"enabled": true, "maxConcurrentThreads": 4, "backend": "v2"}}
    },
    "claude-workflow": {
      "harness": "claude-code-pro",
      "options": {
        "version": 1,
        "agents": {"enabled": true},
        "workflows": {"enabled": true, "maxConcurrent": 4},
        "advisor": {"enabled": true}
      }
    }
  }
}
```

随后在实际绑定中加入 `"native-profile":"dsh-heavy-team"` 等引用。工作台“模型与会话”提供同 harness 的能力配置选择；高级 JSON 编辑定义本身；配置保存仍使用 revision、备份与原子写入，下次引擎启动生效。没有额外的“智能模式”总开关。

### 3.2 loop 的职责不因原生协作改变

| loop / 操作 | 可利用的能力 | 结果如何进入 Flywheel |
| --- | --- | --- |
| Intake / Garden | 并行收集代码证据、Web、PTC、原生 agents | 一次独立调用的建议与 native receipt；不写状态 |
| Plan | Team task board、Swarm 分析、并行代码调查 | root 提交 Spec/packet；Go 继续校验结构、依赖和范围 |
| Execute | agents 分工、Workflow、Tower 候选、LCU 操作 | 实际候选进入 L1/L2/L3、Review 和 Git 交付 |
| Review | 新建或续用本角色独立历史；可使用其 own agents | 仍必须给出既有 verdict 契约；子 Agent 的意见只是证据 |
| Judge / 第二意见 | 原生协作辅助核查 | 独立 scope 和候选身份，不能继承 Executor |
| 原生 Review / 交互 Team | harness 专属的独立入口 | 返回原生任务回执与恢复身份，不直接输出 Flywheel verdict |

默认模型表、订阅 channel 和显式 fallback 规则保持原配置语义。启用 DSH 中的 ChatGPT provider 不会自动切换为 Codex harness；同样，Kimi Swarm 不会在 DSH provider 上凭名称出现。

## 4. Kimi Agent、Swarm 与 Tower

### 4.1 独立受管 Server

未配置 native profile 时保留原 Kimi CLI print 路径。启用后 Go 调用打包的 Kimi native worker，后者启动 `kimi web --no-open --host 127.0.0.1 --port 0`。只有本次启动、PID/server ID/registry/token 匹配的本地服务可以被使用和关闭；日志与回执不暴露 token。

worker 核对实际 OpenAPI/AsyncAPI schema，建立 session 与 WebSocket 订阅，再核对模型、effort、permission、mode、agent profile 和本地执行目录。能力配置和工作目录有证据后才提交 prompt。模型的工具循环、Agent/Swarm/Tower 编排仍由 Kimi 执行。

当前 Server 接缝不提供可证明的 OS 沙箱，故要求显式 `danger-full-access`；不把普通 Kimi CLI 的权限设置冒充 Server 的能力。`agents.maxConcurrent` 不是可强制的整树限额；若用户明确要求这个硬上限，worker 拒绝该组合并说明原因，普通 Agent/Swarm/Tower 可以直接运行。

### 4.2 连续观测、续接与计量

首个原生 turn ordinal 可以为 0；新调用的 baseline 为 -1，不能漏掉首轮。按 prompt ID、turn ID、ordinal、task ID 和 child agent 引用建立本次调用的观测。重连或 snapshot reset 不被视为无损历史；root 结果、后续 continuation、running tasks、agents 和待交互事项共同决定停稳。

native Kimi 不导入旧 CLI PlanSession，统一使用角色账本。普通 scoped Agent/Swarm 续接绑定 task、role、scope、channel、真实目录、模型、effort、权限及 native options；run ID 与 candidate ID 只属于单次回执，不能因为每次调用改变就破坏正常续接。Tower 总是 fresh 候选，不接受旧 session 恢复。

仅在所有本次 root/child step 的 exclusive usage 都完整时生成总量。恢复历史中的子 Agent、compaction、缺失 child identity 或计量，以及 Tower 未公开的整树消耗保留 unknown。`session.usage` 中的占位零不能替代真实 step 计量。中断后没有 durable terminal 的 Kimi Server 调用保持未知，不通过重新发 prompt“恢复”。

### 4.3 Tower 独立候选与可信导回

Tower 在具有独立 `.git` 的临时副本中运行，复制源 HEAD、工作区修改、未跟踪文件、二进制、symlink、权限位和删除。它不共享源 `.git` 管理数据。入口先将执行目录解析为物理路径，并保留调用者声明的目录别名；提示词中这两种精确目录前缀都按路径边界映射到 candidate。映射只扫描原 prompt，不再次处理插入的候选路径；回执记录声明路径、物理路径、候选路径及原始／提交摘要。URL（包括 query）、相似路径和外部 task 路径保持原样。

这提供独立 Git 候选工作目录，**不构成 OS 文件系统沙箱**。源目录可能仍被完全权限的原生进程访问。因此导回必须重新检查源 HEAD、文件内容/模式/link 和外部 task Spec/packet 契约，排除的仅是明确的 runtime 账本与原生证据目录。source 有并发变化或契约变化时保留 candidate 并报冲突。

只有 Execute 在 root/children 完成且所有 owned 进程清理确认后，才通过 `git apply --check --binary` 和可信 patch 导回。其他角色保留候选产物供审核，不自动应用。导回不提交 Git，也不跳过原有候选验证、Review 和交付。

## 5. Codex 与 Claude 的完整原生入口

### 5.1 Codex Pro

`dsh-subagent-codex-pro@0.10.0` 保留 public machine protocol 1 与 lifecycle v1，增加 native version 1。agents options 映射到 app-server 的公开 controls，读取实际 `config/read` 后核对。defaultModel/defaultEffort 只在显式给出时改变子线程默认，不修改 root 账号或模型。

子线程记录当前 turn 的单调进度；旧 turn 的晚到终态不能覆盖新工作。子用量按 owned thread 的 exclusive delta 归属，排除 root，防止重复累加；缺失历史 baseline 的 child 保留 unknown。成功必须同时证明 root 完成、pending IDs 为空、owned children 已结束、native 与 guardian 清理完成。

Codex 原生 `review/start` 不消费 Flywheel 的完整角色 prompt 与 verdict 契约，因此正式 `native-profiles` 不接受 `review` 字段。公开包提供完整原生 Review API，Flywheel 通过独立命令暴露，不将其包装成一次普通 Review turn。

### 5.2 Claude Code Pro

`dsh-subagent-claude-code-pro@0.5.0` 支持 Agent 定义、Workflow 和 Advisor，并输出有效命令设置、init 可见工具和 native 子任务事件。init 只证明工具是否出现，不宣称已验证 CLI 内部所有调度配置。

task-run ID 是不透明身份，不能按字符串大小判断先后；缺少当前 run 身份的旧 terminal 不能结束新 task。Agent/Task tool-use 可能仍排队，observed concurrency guard 只统计已 `task_started` 的 local_agent。配置限额标记 `resourceBounds.hard:false`；超限可取消已观测的工作，但不是派发前的整树 semaphore。

root result usage、child task snapshot 和 Advisor 消耗可能重叠或不完整。保留 raw root result 与 child observations；不能把快照当独占 delta 相加。family total 不可证明时统一为 unknown。

### 5.3 独立任务 API 与 CLI

```sh
flywheel native codex-review --request /absolute/codex-review.json
flywheel native claude-review --request /absolute/claude-review.json
flywheel native claude-team --request /absolute/claude-team.json
flywheel native claude-recover --request /absolute/recovery.json
flywheel native claude-control --request /absolute/control.json
```

请求使用对应公开包 API，并增加私有绝对 `recoveryDirectory`。CLI 限制为 regular JSON 文件，最多 1 MiB；stdout 返回一次完整原生 receipt，stderr 承载诊断。信号触发取消并等待公开 API 收尾。

Codex Review 要求 fresh ephemeral thread 与显式 target。Claude 远端 Review 要求 `consent:true`，执行真实 `claude ultrareview --json --no-post`；不隐式同意云任务或发布 review comment。交互 Team 使用拥有独立身份的 tmux server/pane，支持 capture/send/resize/cancel/attach 控制；文本输出不是结构化验收结论。

远端 Review 与本地进程有不同生命周期：本地 timeout/exit 不证明远端 job 结束。使用原 requestId、runId 和 recoveryDirectory 只读原本地 journal，取回已持久化回执；不会查询云端状态，也不会重新提交 Review。pane ownership 无法证明时也不能把清理标为成功。

Claude `recoverNativeJob` 只读本地 durable journal，不启动进程或向云端查询。当前回执明确 `jobId:null`、`querySupported:false`、`cancelSupported:false`。本地恢复 status 为 terminal 也不等于 remote 完成：若远端 state 仍为 unknown 或 native.cleanup.confirmed 为 false，不能自动重放或声称云任务已取消。

## 6. LCU 挂载、GUI 资源与媒体

### 6.1 沿用公开桥接并区分生命周期

`dsh-plugin-lcu@0.5.0` 保留用户桌面 Preset 的正常挂载与撤销；新增公开 `attachManagedLcu()` 供 owned SDK Agent 使用。真实 Agent registry 和 parent lineage 决定工具调用权，config 中出现一个 Session ID 不等于它是真实 caller。

每个 Agent runtime 有独立 epoch、handle registry、turn lifecycle 和 MCP connection。`lcu_cua` 是 PTC 工具名，程序内仍使用 `cua`；避免新版 executor 对精确 `cua` 名称的特殊 image coercion 把结构化对象 handle 丢失。已有 `run_code`、对象代理、Computer/Browser 操作和多图输出都保留。

app、command、Chrome、audio、allowedApps、allowedOrigins 从显式配置传入实际 runtime。Mac TCC、浏览器扩展、目标白名单和 runtime 的真实用户交互依旧由实际组件负责；本实现不伪造审批回答，也不宣称新增了完整的跨桌面问答 relay。

### 6.2 一台桌面一个 lease

Flywheel 在受管 LCU 创建前取得私有资源目录中的 `local-desktop` lease，记录 invocation、lease ID、epoch 和 owner；每次操作重查 lease 与 caller。root 和后代共享 GUI 所有权，操作顺序执行，不能因为模型 Agent 并发就并发移动同一鼠标。

lease 使用跨进程原子文件约束。进程死掉或 PID 复用不等于资源已释放；无法证明清理的 owner 不被自动接管。终态 lease 保存完整 cleanup 证据的摘要与字节数，完整证据保留在回执中，避免大图把锁文件变成无法读取的超大文件。

### 6.3 完成、取消和物理停止分别记录

abort 首先取消 MCP 请求，再等待 turn 清理，最后关闭 native runtime 并确认实际退出。普通 native DSH shutdown 允许 15 秒，Go 外层不少于 20 秒；LCU 路径分别为 135 秒与 145 秒，覆盖公开 API 的 turn-stop 与 native close 上限。超时不是成功的快捷方式。

policy 在 root turn-stopping 阶段等待 journal flush；最终关闭时写出 `native-cleanup.json`，绑定 request ID、nonce 与 journal complete/events/bytes。journal 最多 4096 条、2 MiB，每条有连续 `journalSequence`；读取核对每条 root 身份、大小与完整换行。写盘失败、超界、归属错误、未完成 turn 或物理退出不确定都会阻止成功和 lease 释放。

### 6.4 媒体是可核验产物

截图在 DSH AttachmentStore 中的最终编码可能与采集编码不同。adapter 从实际 store 读取并归档最终图片，分别记录 capture digest 与 archive digest；不得用采集 hash 去冒充经过正规化后的存储字节。

媒体与 manifest 保存在 `<taskDir>/reports/native/<invocationId>/`。验证 regular file、NOFOLLOW、大小、摘要与 durable 写入；worker 临时目录删除后证据仍存在。`imageDeliveredToModel:'unverified'` 明确区分“生成/存储/归档图片”和“模型确实收到并使用该图片”。ChatGPT provider 的结构投影测试能够证明输入形成，真实模型视觉理解仍需实际账号验收。

## 7. 统一回执、用量、预算和恢复

### 7.1 三种事实分开保存

```json
{
  "version": 1,
  "requested": {"version": 1},
  "effective": {"matched": true},
  "observed": {},
  "children": [],
  "usage": {},
  "completion": {"confirmed": true, "pendingIds": []},
  "cleanup": {"confirmed": true}
}
```

这是结构示意；各 harness 保留自己的有效配置、计量策略、owned identities 和原始事实。`requested` 必须与传入 options 等价；adapter 采用的默认值进入 effective，不回填或改写请求。native-event 是观察帧，不是 lifecycle ACK；只有原 durable lifecycle checkpoint 才允许推进有副作用的执行阶段。

**completion 在清理前冻结。** root 停止时仍有 child 或 continuation 的 invocation 不能因为 shutdown 把它取消了，就倒推出任务已经完成。Go 只在 requested/effective、root、children、completion、cleanup 和外层进程终态相互一致时接受成功。

### 7.2 并发按真实控制粒度解释

`scheduler.max-parallel` 与 `scheduler.providers` 继续限制 Flywheel 根调用，不声称覆盖 harness 内部所有模型请求。DSH 每个 exact live root tree 共享的 activation 池（Team／Workflow、本地／外部 activation 共用）、本地 Agent 深度准入、Codex app-server controls、Claude 的 observed guard、Kimi 原生 scheduler 分别记录，不能统一描述成已强制的全局 token 或整树 concurrency semaphore。 DSH 本地准入也约束 Workflow 创建的真实后代深度，但不宣称控制外部 provider 内部不可观测的后代。

| 后端 | 可以证明的用量 | 不完整时 |
| --- | --- | --- |
| DSH | root 与完整本地 child 的 exclusive committed Session events | external child 无计量、journal 丢失或事件不完整 → unknown |
| Codex | root result + 完整 owned child thread 的本次 exclusive delta | resumed child baseline 不足或丢事件 → unknown |
| Claude | raw root result 与非可加的 child/Advisor observations | 有无法归属的 family 消耗 → total unknown |
| Kimi | fresh root/child 的完整 step usage | compaction、旧 child、Tower 或缺失步骤 → unknown |

未知不是零。原有 hard budget 对未知用量继续阻断新的调用，不能用切换 fresh、降级 adapter 或只保留 root usage 绕过；soft/off 的既有语义由用户配置决定。当前实现不提供事前可预测的严格 token reservation。

### 7.3 会话身份与恢复

Go 在持久化 invocation intent 后生成 managed 身份：run、invocation、role、scope、candidate、channel 和绝对 taskDir。完整 native options 存入 session configuration，并进入历史与执行策略冻结身份；prewarm 同样绑定 composition intent，准备阶段不创建 Agent、不取得 GUI lease、不发送 prompt。

Pro 的原调用 native options 随 durable journal recovery 请求一同传入，恢复回执按原请求核对；更改当前配置不能重解释旧调用。Kimi Server 缺失 durable terminal 时明确保留 unknown，不假装具备 Pro 的 ACK/recovery 能力。LCU 句柄跨 runtime epoch 无效，恢复后必须从真实 runtime 重新取得对象。

## 8. 仓库分工与分发

| 仓库 / PR | 本次交付 |
| --- | --- |
| [flywheel #44](https://github.com/ckanner/flywheel/pull/44) | 配置与 schema、Go drivers/state/history、DSH/Kimi adapters、native job CLI、工作台选择、资源/媒体回执、单包构建与完整文档 |
| [dsh-preset-generator #1](https://github.com/ckanner/dsh-preset-generator/pull/1) | 0.4.0 composition API、PTC 静态身份与真实准入契约、daily/heavy 兼容 |
| [dsh-plugin-lcu #1](https://github.com/ckanner/dsh-plugin-lcu/pull/1) | 0.5.0 受管 Agent API、epoch/handle、物理收尾、媒体和桌面生命周期修复 |
| [dsh-subagent-codex-pro #6](https://github.com/ckanner/dsh-subagent-codex-pro/pull/6) | 0.10.0 原生 controls/events/accounting/completion 与独立 review |
| [dsh-subagent-claude-code-pro #2](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/2) | 0.5.0 Agent/Workflow/Advisor、远端 Review、交互 Team、恢复和实际并发观察 |
| [dsh-plugin-chatgpt #8](https://github.com/ckanner/dsh-plugin-chatgpt/pull/8) | 0.1.19 attachment/image 投影、取消协议、公开依赖兼容与真实安装 |

Flywheel 仍只发布一个 `@ckanner/flywheel` 包，包含 CLI、DSH host/client、四个 workers、内嵌 skills 和四个平台 Go 二进制。Codex 0.10.0 与 generator 0.4.0 以真实 tarball 锁定并 bundle；Claude 0.5.0 和 LCU 0.5.0 为 optional peer。ChatGPT 是 DSH provider，是否使用由已选 DSH composition/account 决定。

五个配套候选都在 `plugins/dsh/vendor/*.json` 记录可取得的 source commit/tree、tarball hash、integrity、许可证和未验证 registry 发布状态。CI 从固定 Claude commit 复现并核对同一候选；正常 npm resolver 必须通过，不使用 force/legacy-peer 绕过兼容。提交 PR 不等于执行 npm publish，也不修改用户安装。

## 9. 验证策略与实施后的判断标准

必须通过 Go `make quality-gate`（每个计量函数 100% statement coverage）、DSH 完整 Node 行为与旧宿主兼容、四种 Go→Node 实际进程桥接、实际公开 Pro API、真实 PTC/Team/LCU 接缝、Kimi 本地 HTTP/WS 和真实 Git 候选、npm 启动器、36 项实际引擎 session 场景，以及包含四平台二进制的单包安装。测试使用确定性模型/runtime fixture 时明确标识，不称为真实模型验收。

运行证据至少覆盖：错误 native config 不派发、模型/账号不静默改变、foreign child 不入树、旧 terminal 不覆盖新 epoch、root 结束但 child 仍活跃不成功、取消与清理分离、journal 写失败不释放 GUI、图片多块顺序与 hash、候选冲突保留、unknown usage 不变零、恢复不重放副作用。

实际收益另行用相同任务与候选比较质量、返修次数、耗时、用量与 GUI 成功率；不从“有多 Agent”推断更快或更省。Mac TCC、Chrome 扩展、真实账号能力、实时模型视觉与 Claude 云端权益仍需用户机器的独立验收，不能由离线绿灯代替。最终已运行项目、失败/跳过和未运行范围见[验收记录](../validation/2026-11-14-native-harness-implementation.md)。

## 10. 主要源码与研究依据

当前行为以[架构](../design.md)、[native schema](https://github.com/ckanner/flywheel/blob/main/templates/native-profiles.schema.json)、[worker 协议](../../plugins/dsh/PROTOCOL.md)、[Kimi 协议](../../plugins/dsh/KIMI_NATIVE_PROTOCOL.md)及同 PR 的源码/测试为准。历史研究保留在前述原提案，本次适配器来源固定在 vendor 记录。

| 上游 | 已核对的一手来源 |
| --- | --- |
| DSH SDK / PTC | [release SDK](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/sdk/server/src/server.ts)、[官方 PTC overlay](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/apps/cli/config/examples/ptc.yml)、[master PTC](https://github.com/deepseek-ai/deepseek-harness/blob/39163dd30f703a79079e2d2446545cff3c453738/packages/core/tool-executor-ptc/src/ptc.ts) |
| DSH Team / Agent | [Team service](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/team/agent-team/README.md)、[Subagent service](https://github.com/deepseek-ai/deepseek-harness/blob/38a6a5c965aa99ab525161efb7cc5cf1fe62fced/packages/subagent/subagent/README.md) |
| Kimi Server / Agent | [Server API](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api.md)、[conversation API](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/reference/server-api-conversation.md)、[agents](https://github.com/MoonshotAI/kimi-code/blob/f082f04a093a3e71a2263c5461d52507dc31f83a/docs/en/customization/agents.md) |
| Codex | [公开 machine 契约](https://github.com/ckanner/dsh-subagent-codex-pro/blob/c0045b06e6762b8dfc56f50b14cce7a53af2725b/docs/machine.md)及该 PR 中固定的官方 app-server 来源 |
| Claude | [Subagents](https://code.claude.com/docs/en/sub-agents)、[Teams](https://code.claude.com/docs/en/agent-teams)、[Workflows](https://code.claude.com/docs/en/workflows)、[Ultrareview](https://code.claude.com/docs/en/ultrareview)、[Advisor](https://code.claude.com/docs/en/advisor)，公开包同时保留实际版本/能力不足的失败回执 |
