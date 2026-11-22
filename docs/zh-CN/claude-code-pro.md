# Claude Code Pro 订阅后端

[文档目录](../README.zh-CN.md) · [English](../en/claude-code-pro.md) · 简体中文

Flywheel 通过 `harness: "claude-code-pro"` 调用
[`dsh-subagent-claude-code-pro`](https://github.com/ckanner/dsh-subagent-claude-code-pro)
的公开 machine API。插件通过官方 **Agent SDK 0.3.326** 运行配置指定的本地
Claude Code 程序，保留原生工具循环。Go 仍负责阶段调度、角色会话、验证、预算和交付。

适配器要求 **`dsh-subagent-claude-code-pro@0.7.0`**、machine protocol 1
及其公开 lifecycle/recovery 接口。旧桌面工具接口和旧 machine 契约不能替代这些
接口。订阅预设让 Claude 负责实施规划和前端／多模态实现，Fable 保留为显式选择的
备用 profile，详见[模型分工](model-policy.md)。源码兼容性与包可用性需要分别核对；
发行流程见[发布指南](releasing.md)。

## 准备依赖

需要 Node.js 22.18.0 或更高、macOS 或 Linux，以及可被启动引擎的进程找到的
兼容的 `claude` 程序及获准使用的认证；Flywheel 不复制登录数据或代办登录。

Flywheel 包含 `flywheel-claude-code-pro-worker`，把 Claude 插件作为精确版本的
**optional peer**。没有选择这个后端时无需安装插件，也不会启动 Claude。选择后必须
让 worker 的 Node 依赖解析能找到 `dsh-subagent-claude-code-pro@0.7.0`。仅在 DSH
desktop profile 安装插件，不保证独立 worker 能访问该目录。

registry 已有该精确版本时，将它装到 Flywheel 所在的同一全局 npm prefix 或同一项目依赖树。
验证明确审查过的源码候选时，可从插件源码运行 `npm pack --ignore-scripts`，把产生的真实 `.tgz`
安装到同一 prefix，再用新构建的 Flywheel 验证。不要用 0.3.1 替代，也不要编辑包版本
伪装兼容。此开发候选流程不提升 npm latest。

已审查的配套 0.7.0 精确产物见
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

Go 只传递白名单基础环境和所选 channel 明确注入的凭证。订阅预设使用
`claude-native` 与 `env-key:null`，不继承外部 Anthropic API key；订阅策略还会检查
原生运行时的冲突凭证／provider 设置。环境凭证值不主动序列化到请求或 diagnostics。

## 官方 SDK、本地程序与认证

普通调用进入配套插件独立 guardian 内的 SDK `query()`。SDK 负责请求与控制协议，
适配器继续负责原生进程组、有界流校验、活动、用量和持久化回执。SDK 结束本身
不能证明物理清理完成。配置的订阅策略同样适用于前台、PTC 和后台委派。

`FLYWHEEL_CLAUDE_BIN` 保持权威，默认 `claude`；本地程序缺失或不兼容时，不会
静默切换到 SDK 自带程序。安装 SDK 可能同时安装可选平台程序包，但不会改变实际
选用的程序、配置目录或账号。

本地 CLI 在自己的 `HOME`／`CLAUDE_CONFIG_DIR` 中解析既有登录。插件不实现 OAuth、
复制 token、切换账号或修改账户设置。通道名不能证明认证方式；下述证据必须由同一个
已初始化的原生进程提供。刻意不配置订阅策略的旧 API 配置仍可使用，但它们不会成为
订阅调用失败后的自动备选。

### 原生订阅准入

配置 `subscription:{"version":1,"mode":"subscription-only"}` 后，0.7.0 适配器在
以下检查全部通过前，不向 SDK 用户消息生成器放行任务输入：

1. 拒绝已知 API 凭证、外部 provider／endpoint、备用模型、fast mode 和冲突 effort
   设置，冻结明确的第一方模型 ID。只读选中的原生 settings，不读凭据文件或导出 OAuth token。
2. 通过精确的 **SDK 0.3.326** 初始化指定 CLI，检查真实 CLI 版本及其提供的 effort 能力信息。
3. 在**同一 query／进程**调用 `accountInfo()` 和公开实验方法
   `usage_EXPERIMENTAL_MAY_CHANGE_DO_NOT_RELY_ON_THIS_API_YET({skipBehaviors:true})`；
   不发送任务 prompt，也不通过 `/usage` 模型请求探测。
4. 要求两份响应都给出一致的第一方 `pro`、`max`、`team` 或 `enterprise` 订阅类型、
   可用的原生计划额度信息，以及固定响应契约中的明确
   `rate_limits.extra_usage.is_enabled === false`。满足后才放行任务。

控制接口缺失、响应形状未知、计费证据不可用或 extra usage 已开启都会阻止任务输入。
预检自身有 10 秒上限，并服从更短的任务截止时间和取消。失败时不更换模型、provider、
API 账号或 credits 路由。调用设置 `forceLoginMethod:"claudeai"`、`fallbackModel:[]`、
`fastMode:false` 和原生禁用 fast mode 控制，不改写用户配置文件。

回执保存 `status:"verified"`、`source:"sdk-usage-0.3.326"`、观察到的订阅类型、
`extraUsageEnabled:false`、解析后的模型以及 `scope:"pre-prompt-snapshot"`。
这证明任务输入前的准入快照，不是对之后账户变更的原子锁，也不保证模型权益。
Fable 权限和剩余额度取决于实际账号，原生拒绝仍作为失败返回。回执不含邮箱、token、
余额或原始账户／用量响应。

独立原生远程 Review、交互 Team 创建及 Team send／attach 无法完成相同的任务前
检查，因此在订阅策略下拒绝。既有自有任务的 recovery、capture、resize、cancel 仍可用。
普通 SDK 委派仍能使用明确启用的原生 Agent／Workflow／Advisor 能力。已授权的 hooks
和外部工具不受账户计费检查隔离。详见插件的
[完整订阅契约](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/main/docs/subscription.md)
与[原生能力指南](native-capabilities.md)。

## 流式能力与可读会话名称

查询路径在发送业务 prompt 前，使用无模型请求的 `claude --help` 检查
`--include-partial-messages`、`--forward-subagent-text`，
新建具名持久会话还要求 `--name`。以已安装 CLI 实际提供的 flags 为准，版本号本身
不能替代能力检查；缺少能力时，在业务文本送入 CLI 前明确失败。检查与正式查询
共用原 wall 预算和取消信号。
SDK 随后检查同一进程 initialize 响应中的 `claude_code_version`，只有稳定的
**2.1.325 或更新版本**才放行任务输入。这是本适配器最低已验证版本，不表示每项
上游功能首次出现的版本。版本缺失、过旧或为预发布版时，在任务输入前失败。
此时原生进程已经启动，回执保留实际 PID 与独立清理事实。订阅准入拒绝时，
`execution.turnDispatch` 保持 `not-dispatched`；启动检查进程与发送任务分别记录。

真实根／子任务的 partial、tool、task 活动刷新 Flywheel 的 idle 预算，不复制私有
思考正文，也不通过定时心跳伪造进展；native 状态去重不抹掉活动。长期小增量不会
耗尽累计原始流量预算，单帧和保留结果仍有上限。排队或不可见的工具仍可能静默；
停止后如何处理见[活动、超时与恢复](session-recovery.md)，不能只因超时自动重试。

新持久历史通过 SDK `title` 选项获得 Flywheel 的任务／角色标题；恢复时保留已有标题，
包括用户手动改名。脱离 Flywheel 使用时，插件优先采用明确委派标签，缺省使用可读
工作区名称；命名不额外调用模型。

受管 fresh 调用现在每次保存一条新的具名原生历史，不传上一条 Session ID。
scoped 可续用已经确认的 ID，保存历史本身不授权复用。Claude 官方 picker 排除
print／SDK 会话；请通过真实根 UUID 使用受支持的 CLI／Desktop 入口，见
[原生历史可见性](session-recovery.md#在原生客户端中找到会话)。

0.7.0 配套插件会在 CLI 探测或任务启动前，拒绝有效环境中启用了
`CLAUDE_CODE_SKIP_PROMPT_HISTORY` 的 persistent 新建／续接请求。
显式 ephemeral 仍可用，用户设置不会被改写。原生真值为 `1`、`true`、`yes`、`on`，
忽略大小写与首尾空格。Flywheel 的 Go 环境白名单原本就不转发此变量；新增检查
同时保护直接 machine、DSH 委派和交互 Team 调用。它不证明任意存储故障，
也不核验 CLI 内部随后加载的所有设置。

## 先定义模型，按需绑定阶段

完整的 [profiles 模板](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.profiles.example.json)
包含 `opus-plan`、`opus-ui` 和未引用的 `fable`。以下是加入自己现有完整配置的片段，
不是替换全部配置的文件：

```json
{
  "profiles": {
    "opus-plan": {
      "harness": "claude-code-pro",
      "model": "claude-opus-5-5",
      "channel": "claude-native",
      "thinking": "high"
    }
  },
  "channels": {
    "claude-native": { "harness": "claude-code-pro", "env-key": null }
  },
  "subscription": { "version": 1, "mode": "subscription-only" },
  "permissions": { "mode": "full-access" },
  "sessions": { "mode": "scoped", "policy-version": 1 }
}
```

预设直接使用完整模型 ID。订阅策略下，配套插件把 `opus`／`best` 冻结到
`claude-opus-5-5`，`sonnet` 到 `claude-sonnet-5-5`，`haiku` 到 `claude-haiku-5-5`，
`fable` 到 `claude-fable-5-2`；受支持的原生第一方 family 环境覆盖单独记录。
动态 `default`／`opusplan` 不予准入。完整 ID 必须与原生初始化及整个当前流内观察到的
根 assistant 模型一致；换成别的模型后再换回来也会失败。
`result.modelUsage` 可能含历史、辅助请求和子代理，只标为累计证据，不当作当前轮 fallback 轨迹。

支持请求 low、medium、high、xhigh、max，省略时 high。显式环境覆盖、已知原生 effort
上限冲突及 CLI 模型目录明确报告不支持的档位均被拒绝；没有回显的有效 effort 保持未知，
不能把请求被接受写成实际采用该档位的证明。

让所需的 `loops.plan/review/judge/intake/garden` 或 Execute 的
`routes.*` 引用该 profile，例如 `{"profile":"opus-plan"}`。预设为 Plan 选择
`opus-plan`，为前端／多模态执行选择 `opus-ui`。现有绑定有显式 `provider` 时需同时清掉；
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
和 CLI。上述检查不发模型请求，不能证明账号额度或真实推理已通过。需要不发送任务地
核查真实订阅准入时，调用插件公开的 `inspectSubscription()`，传入实际 command、cwd、
model、effort 和环境；它区分 `verified`、`blocked`、`unknown`。正式执行会在自己的
原生进程中重新完成检查，不沿用 doctor 的旧结果。

### 为具名原生 Agent 设置 effort

每个 `native-profiles.<name>.options.agents.definitions.<agent>.effort` 接受
`low`、`medium`、`high`、`xhigh` 或 `max`。例如，把以下片段加入同一个完整配置，
再在已有 `opus-plan` profile 中加入 `"native-profile":"claude-agents"`：

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
SDK `agents`，经 initialize 传递；交互 Team 保留 `--agents`。省略子 effort 时保持
字段缺省，让 CLI 使用原生继承行为；Flywheel
不会填入父值或自行设定子默认值。非法档位在配置加载时拒绝，不会派发 worker。

同一字段也可用于 0.7.0 桌面插件的 `native` 设置和 PTC 逐次覆盖；订阅策略下不允许
独立交互 Team。直接 Flywheel worker 仍读取选中的 native profile，不读取桌面设置。显式
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

普通消息流的 Claude init 在 prompt 后观察；SDK initialize 和订阅检查在任务输入前
完成。不会伪造预派发 thread-ready 或真实 turn-accepted。终态仍须检查实际模型、目录
与权限。supervisor 自身丢失、journal 损坏或缺少原候选
检查点时仍可阻断，不能承诺任意崩溃都继续交付。详见[恢复与质量](session-recovery.md)。
真实账号质量、缓存率和性能收益须另行[配对评估](session-evaluation.md)，离线成功不证明这些结果。

## 冻结图片输入

工程 packet 可以声明 `input-images:{"version":1,"images":[...]}`。每一项包含
仓库相对 `path`、小写 `sha256` 摘要与 `media-type`。Execute 从任务仓库解析这些
引用，验证已冻结字节，再经公开 machine API 与官方 SDK 发送真实 base64 附件。
此类 packet 必须选择 Claude 多模态 Execute 绑定；其他后端在派发前拒绝。
Plan／Review／Judge 不会隐式收到图片字节。

Flywheel 接受 1 至 4 张 PNG、JPEG、GIF 或 WebP；单张最多 4 MiB，合计最多
8 MiB。每张图片须完整解码并匹配声明格式，最大边长 8,192 像素、总计最多
1,600 万像素。路径须指向仓库内普通文件，各级路径不得使用符号链接。
packet 冻结文件路径、MIME 类型与 SHA-256；参考图变化必须更新已批准的 packet，
不能悄悄换成下一次模型输入。

Claude worker 的 start 帧上限为 16 MiB，包含换行，用于容纳编码附件；prompt
仍有独立的 1 MiB 上限。其他 worker 保持原有 1 MiB 输入帧限制。
图片在原生消息中成为文本 prompt 后的 image 内容块，不是文件路径描述。

companion 计算 `requested.inputImages`，按输入顺序记录 MIME、解码字节数和
SHA-256；durable journal 在首个派发检查点前将同份 manifest 写入
`context.inputImages`。Flywheel 将这些证据与调用冻结的 manifest 比较，只读恢复也
执行相同核对。调用 state 与输入 manifest 不保存图片内容；原生 CLI 可以保留其
已授权会话历史。尚未确认的调用须先完成恢复核对，才能读取或发送另一张图片。
恢复不会替换变更后的附件，也不会授权自动重试。

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
