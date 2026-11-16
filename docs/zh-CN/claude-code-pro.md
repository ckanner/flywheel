# 可选 Claude Code Pro 后端

[文档目录](../README.zh-CN.md) · [English](../en/claude-code-pro.md) · 简体中文

Flywheel 通过 `harness: "claude-code-pro"` 调用
[`dsh-subagent-claude-code-pro`](https://github.com/ckanner/dsh-subagent-claude-code-pro)
的公开 machine API。插件运行用户已安装的 Claude Code CLI，使用该 CLI 的原生工具
循环与登录环境。Go 仍负责阶段调度、角色会话、验证、预算和交付。

适配器要求 **`dsh-subagent-claude-code-pro@0.5.2`**、machine protocol 1
及其公开 lifecycle/recovery 接口。旧桌面工具接口和旧 machine 契约不能替代这些
接口。默认阶段和路由均未选择 Claude。源码兼容性与包可用性需要分别核对；
发行流程见[发布指南](releasing.md)。

## 准备依赖

需要 Node.js 22.18.0 或更高、macOS 或 Linux，以及可被启动引擎的进程找到的
`claude` CLI。用户按 CLI 自身流程完成登录；Flywheel 不复制登录数据。

Flywheel 包含 `flywheel-claude-code-pro-worker`，把 Claude 插件作为精确版本的
**optional peer**。没有选择这个后端时无需安装插件，也不会启动 Claude。选择后必须
让 worker 的 Node 依赖解析能找到 `dsh-subagent-claude-code-pro@0.5.2`。仅在 DSH
desktop profile 安装插件，不保证独立 worker 能访问该目录。

registry 已有该精确版本时，将它装到 Flywheel 所在的同一全局 npm prefix 或同一项目依赖树。
验证明确审查过的源码候选时，可从插件源码运行 `npm pack --ignore-scripts`，把产生的真实 `.tgz`
安装到同一 prefix，再用新构建的 Flywheel 验证。不要用 0.3.1 替代，也不要编辑包版本
伪装兼容。此开发候选流程不提升 npm latest。

已审查的配套 0.5.2 精确产物见
[vendor 记录](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/claude-code-pro.json)。
CI 验证这些候选字节；发布 Flywheel 前，必须从公共 registry 取得精确插件，
并核对包身份、SHA-256 和 integrity 与该记录一致。源码验证本身不证明 registry 可用性。
此 worker 会拒绝旧配套版本，不会悄悄丢弃活动或命名能力。同步升级 worker 和 peer；旧版本的
未决调用保留原适配器与恢复身份，须先使用原版本核对，不能改写为新版本。

| 环境变量 | 用途 |
|---|---|
| `FLYWHEEL_CLAUDE_CODE_PRO_WORKER` | 显式 worker 覆盖；npm 启动器通常自动解析同版本 worker |
| `FLYWHEEL_CLAUDE_BIN` | Claude CLI 命令或绝对路径，缺省为 `claude` |
| `CLAUDE_CONFIG_DIR` | 可选的原生 CLI 配置／登录目录，由 Claude 解释 |

Go 只传递白名单基础环境和所选 channel 明确注入的凭证；没有配置 env-key 的订阅
渠道不自动继承外部 Anthropic API key。显式凭证只通过受支持的环境名进入插件，
环境凭证值不主动序列化到请求或 diagnostics；CLI 返回的文本按实际结果保留。

## CLI 流式能力与可读会话名称

查询路径在发送业务 prompt 前，使用无模型请求的 `claude --help` 检查
`--include-partial-messages`、`--forward-subagent-text`（上游 2.1.211 引入），
新建具名持久会话还要求 `--name`。以已安装 CLI 实际提供的 flags 为准，版本号本身
不能替代能力检查；缺少能力时，在业务文本送入 CLI 前明确失败。检查与正式查询
共用原 wall 预算和取消信号。

真实根／子任务的 partial、tool、task 活动刷新 Flywheel 的 idle 预算，不复制私有
思考正文，也不通过定时心跳伪造进展；native 状态去重不抹掉活动。长期小增量不会
耗尽累计原始流量预算，单帧和保留结果仍有上限。排队或不可见的工具仍可能静默；
停止后如何处理见[活动、超时与恢复](session-recovery.md)，不能只因超时自动重试。

新持久历史通过原生 `-n` 获得 Flywheel 的任务／角色标题；恢复时保留已有标题，
包括用户手动改名。脱离 Flywheel 使用时，插件优先采用明确委派标签，缺省使用可读
工作区名称；命名不额外调用模型。

## 先定义模型，按需绑定阶段

完整的 [profiles 模板](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.profiles.example.json) 具有未引用的
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

让所需的 `loops.plan/review/judge/intake/garden` 或 Execute 的
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

### 为具名原生 Agent 设置 effort

每个 `native-profiles.<name>.options.agents.definitions.<agent>.effort` 接受
`low`、`medium`、`high`、`xhigh` 或 `max`。例如，把以下片段加入同一个完整配置，
再在已有 `claude-sonnet` profile 中加入 `"native-profile":"claude-agents"`：

```json
{
  "native-profiles": {
    "claude-agents": {
      "harness": "claude-code-pro",
      "options": {
        "version": 1,
        "agents": {
          "enabled": true,
          "definitions": {
            "scout": {
              "description": "检查相关代码",
              "prompt": "读取目标代码，报告有具体依据的发现。",
              "model": "sonnet",
              "effort": "medium"
            }
          }
        }
      }
    }
  }
}
```

父绑定的 `thinking` 仍单独传给父调用的 `--effort`；子定义原样进入 Claude 的
`--agents` JSON。省略子 effort 时保持字段缺省，让 CLI 使用原生继承行为；Flywheel
不会填入父值或自行设定子默认值。非法档位在配置加载时拒绝，不会派发 worker。

同一字段也可用于 0.5.2 桌面插件的 `native` 设置、PTC 逐次覆盖及独立交互 Team
请求。直接 Flywheel worker 仍读取选中的 native profile，不读取桌面设置。显式
传递 effort 只能证明 CLI 请求参数；tools-only init 证据不能证明子模型实际采用的档位。

## 权限与会话

适配器要求所选绑定明确采用完全权限，映射为 Claude 原生 `bypassPermissions`
与非交互调用。可以全局设置 `permissions.mode:"full-access"`，也可以使用支持的
阶段覆盖，具体见[配置指南](configuration.md)。plan／acceptEdits 等原生审批模式不等于 OS 的
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

项目归属、执行目录选择与 worktree 约定见[工作流指南](workflow.md)。这些位置仍属于
调用身份；修改目录不会授权重放尚未确认的调用。

### 持久化恢复

插件在派发 Claude 前建立独立 supervisor、durable journal 和 native guardian。
Go 保存并确认 intent/possible 检查点后才允许提交 prompt。worker 被强制终止时，
supervisor 取消未确认步骤并收束原生进程，将完整终态落盘；正常调用也等待 guardian
退出。公开 `recoverMachine` 只读取这次调用，不启动 CLI 或重发 prompt。

Claude 的 init 只能在 prompt 后观察，不提供预派发 thread-ready 或真实 turn-accepted；
终态仍须检查实际模型、目录与权限。supervisor 自身丢失、journal 损坏或缺少原候选
检查点时仍可阻断，不能承诺任意崩溃都继续交付。详见[恢复与质量](session-recovery.md)。
真实账号质量、缓存率和性能收益须另行[配对评估](session-evaluation.md)，离线成功不证明这些结果。

## 用量与成本

完整 CLI usage 中 input_tokens 不含缓存；Flywheel 总输入加上 cache read 和 cache
creation，再加 output 得到总 token。缺失计数保持 partial/unknown，不能补零。
每次续接只计入该次调用，不把历史累计重复入账。

原始回执保留 CLI 的 `total_cost_usd` 为 `reportedCostUSD`。它是 API 等价估算，
不是订阅实际账单；`costUSD` 保持 null、`costStatus` 保持 unknown。已有 soft/hard
预算纪律继续适用。

若用户显式提供 `pricing`，metrics／预算可另记配置价表估算：从包含缓存的总输入中
扣除 cache read／creation，再对各桶计价，避免重复计算。该估算不读取
`reportedCostUSD`，也不改写原始回执或变成订阅实际账单。
