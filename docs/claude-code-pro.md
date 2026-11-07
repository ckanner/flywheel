# 可选 Claude Code Pro 后端

[English](en/claude-code-pro.md) · [配置指南](configuration.md)

Flywheel 现在可通过 `harness: "claude-code-pro"` 调用
[`dsh-subagent-claude-code-pro`](https://github.com/ckanner/dsh-subagent-claude-code-pro)
的公开 machine API。插件运行用户已安装的 Claude Code CLI，使用该 CLI 的原生工具
循环与登录环境。Go 仍负责阶段调度、角色会话、验证、预算和交付。

**源码与发行边界：**此接入要求插件 **0.3.2 / machine protocol 1**；0.3.1 只有
DSH 工具入口，不满足机器回执契约。0.3.2 是本次配套源码版本，不能据此推断 npm 已
发布。Flywheel 当前源码版本仍为 0.28.5，新增代码也不等于旧同版本安装已更新。
合并、发布及用户安装各有独立状态。现有默认 loop／route 没有切换到 Claude。

## 1. 准备依赖

需要 Node.js 22.18.0 或更高、macOS 或 Linux，以及可被启动引擎的进程找到的
`claude` CLI。用户按 CLI 自身流程完成登录；Flywheel 不复制登录数据。

Flywheel 包含 `flywheel-claude-code-pro-worker`，把 Claude 插件作为精确版本的
**optional peer**。没有选择这个后端时无需安装插件，也不会启动 Claude。选择后必须
让 worker 的 Node 依赖解析能找到 `dsh-subagent-claude-code-pro@0.3.2`。仅在 DSH
desktop profile 安装插件，不保证独立 worker 能访问该目录。

正式发布后，将该精确版本装到 Flywheel 所在的同一全局 npm prefix 或同一项目依赖树。
发布前可从已审查的插件源码运行 `npm pack --ignore-scripts`，把产生的真实 `.tgz`
安装到同一 prefix，再用新构建的 Flywheel 验证。不要用 0.3.1 替代，也不要编辑包版本
伪装兼容。此开发候选流程不提升 npm latest。

| 环境变量 | 用途 |
|---|---|
| `FLYWHEEL_CLAUDE_CODE_PRO_WORKER` | 显式 worker 覆盖；npm 启动器通常自动解析同版本 worker |
| `FLYWHEEL_CLAUDE_BIN` | Claude CLI 命令或绝对路径，缺省为 `claude` |
| `CLAUDE_CONFIG_DIR` | 可选的原生 CLI 配置／登录目录，由 Claude 解释 |

Go 只传递白名单基础环境和所选 channel 明确注入的凭证；没有配置 env-key 的订阅
渠道不自动继承外部 Anthropic API key。显式凭证只通过受支持的环境名进入插件，
环境凭证值不主动序列化到请求或 diagnostics；CLI 返回的文本按实际结果保留。

## 2. 先定义模型，按需绑定阶段

完整的 [profiles 模板](../templates/flywheel.profiles.example.json) 具有未引用的
`claude-sonnet`。以下是加入自己现有完整配置的片段，不是替换全部配置的文件：

```json
{
  "profiles": {
    "claude-sonnet": {
      "harness": "claude-code-pro",
      "model": "sonnet",
      "channel": "claude-subscription",
      "thinking": "high"
    }
  },
  "channels": {
    "claude-subscription": { "env-key": null }
  },
  "permissions": { "mode": "full-access" },
  "sessions": { "mode": "scoped", "policy-version": 1 }
}
```

`sonnet`、`opus`、`haiku` 使用 CLI 原生别名，也可写实际账号支持的完整模型 ID。
完整 ID 必须与 CLI init 观察值一致，三个别名核对对应模型 family；具体版本由 CLI
目录决定。支持传递 low、medium、high、xhigh、max，省略时 high。能传递档位不代表
每个模型／CLI 版本都支持它；拒绝会原样保留，不静默更换模型或档位。CLI 没有回显
有效 effort 时只记录请求值。

未来决定用途时，让所需的 `loops.plan/review/judge/intake/garden` 或 Execute 的
`routes.*` 引用该 profile。例如，把某个绑定设为 `{"profile":"claude-sonnet"}`。
这只是配置能力，不是默认阶段分配建议。现有绑定有显式 `provider` 时需同时清掉；
`provider` 是 DSH 原生 LLM adapter 字段，不能填 `subagent_claude_code` 或
`claude-code-pro` 来代替 harness。
从 Kimi Plan 切换时也移除原来的 `resident-session` 字段，让历史策略由
`sessions.mode` 控制。`resident-session:true` 对 Claude 无效；为保留旧配置的明确
选择，Plan 的显式 `resident-session:false` 仍会关闭 scoped 历史复用。

新配置在下次启动引擎时生效。检查实际命中的文件和展开结果：

```sh
flywheel config inspect --cwd /repo --json
flywheel doctor --cwd /repo --json
flywheel-claude-code-pro-worker --doctor
```

未绑定的 profile 不要求安装 Claude；一旦绑定，doctor 会检查 worker、精确插件接口
和 CLI。上述检查不发模型请求，不能证明账号额度或真实推理已通过。

## 3. 权限与会话

首版要求显式 `permissions.mode:full-access`，映射为 Claude 原生
`bypassPermissions` 与非交互调用。plan／acceptEdits 等原生审批模式不等于 OS 的
read-only／workspace-write 沙箱，因此 role-based 请求会在启动前拒绝。
评审、规划的角色职责及候选前后检查仍有效；完全权限不允许通过修改验收取得 pass。

主任务的 Plan／Execute／Review／Judge 使用现有 fresh/scoped 策略。scoped 普通
返修只续接同任务、同范围、同角色的已确认历史；Review 与 Execute 分开，第二意见
也独立。session ID 精确传入原生 resume，不猜最近会话，恢复失败不改为新建重放。
Intake／Garden 可选此后端，但继续执行独立建议调用，不拥有主任务角色会话账本。

一个可应用的成功结果必须同时具备合法完整流、请求／会话／配置匹配、正常物理退出和清理
确认。父连接断开、取消、超时、坏帧、重复终态、异常退出或未知清理都不能取得成功。
确定发生在 machine 调用前的依赖缺失／参数拒绝被记录为未派发与零消耗；修好环境后
可继续。已经进入调用却没有可信结果时，即使进程已清理，仍须停止而不能自动重放。

插件在派发 Claude 前建立独立 guardian 与 worker 的 IPC lifeline。worker 被强制
终止或连接丢失后，guardian 负责停止并清理原生进程组，再退出。正常调用也等待
guardian 退出；这只补齐物理清理，丢失的调用回执仍然是 unknown。

这里只实现终态确认后的续接和 Flywheel 已保存回执的恢复。没有 early durable ack、
实时查询或任意崩溃点恢复承诺；也没有真实账号质量、缓存率或性能收益结论。

## 4. 用量与成本

完整 CLI usage 中 input_tokens 不含缓存；Flywheel 总输入加上 cache read 和 cache
creation，再加 output 得到总 token。缺失计数保持 partial/unknown，不能补零。
每次续接只计入该次调用，不把历史累计重复入账。

原始回执保留 CLI 的 `total_cost_usd` 为 `reportedCostUSD`。它是 API 等价估算，
不是订阅实际账单；`costUSD` 保持 null、`costStatus` 保持 unknown。已有 soft/hard
预算纪律继续适用。

若用户显式提供 `pricing`，metrics／预算可另记配置价表估算：从包含缓存的总输入中
扣除 cache read／creation，再对各桶计价，避免重复计算。该估算不读取
`reportedCostUSD`，也不改写原始回执或变成订阅实际账单。
