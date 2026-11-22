# Antigravity 与 Gemini

[文档目录](../README.zh-CN.md) · [English](../en/antigravity.md) · 简体中文

通过 `harness:"antigravity"` 使用官方本地 **`agy` CLI** 运行 Gemini。
Flywheel 调用 `@ckanner/dsh-subagent-antigravity@0.2.0` 的公开 machine API；插件负责原生
Agent 循环、进程组和持久化回执，Go 负责阶段调度、方案审批、证据收集、验证和交付。
同一个插件也提供 DSH 直接任务面板和 `subagent_antigravity`。不使用 ACP，也不另接模型 API。

## 安装与能力检查

要求 macOS 或 Linux、Node.js 22.18+，以及已安装并通过原生流程登录的官方
`agy` **1.5.7+**；Orchestrator 和显式 subagents／Swarm 要求 **1.5.12+** 及相应公开能力。
插件不下载原生程序、不实现登录、不复制账户 token，也不修改
全局设置。本版本使用 POSIX 进程组守护，不支持 Windows 执行。

Flywheel 包含 `flywheel-antigravity-worker` 与
[vendor 记录](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/antigravity.json)
中的精确配套候选。完整源码位于
[`dsh-subagent-antigravity`](https://github.com/ckanner/dsh-subagent-antigravity)，
保留独立 npm 包结构，便于直接审查。源码身份、候选字节安装、公共 registry 可用性
和真实账号验收分别核对；不能拿其他 machine 契约的旧版本替代 0.2.0。

| 环境变量 | 用途 |
| --- | --- |
| `FLYWHEEL_ANTIGRAVITY_WORKER` | 显式覆盖 worker 可执行程序 |
| `FLYWHEEL_AGY_BIN` | 官方 CLI 命令或绝对路径，默认 `agy` |
| `HOME` | 原生账号与设置所在环境；凭据仍由 CLI 管理 |

```sh
flywheel config inspect --cwd /repo --json
flywheel doctor --cwd /repo --json
flywheel-antigravity-worker --doctor
```

worker doctor 检查精确插件及公开 machine、lifecycle、evidence-only 契约。插件的
`doctor()` 和 DSH **检查环境**还在执行所用环境中，对 `--version`、`--help`、`models`
进行有界探测。这些检查不发模型 prompt，不证明剩余额度、账户权益或真实回答质量。

## 模型配置与阶段分工

[完整模板](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.profiles.example.json)
使用统一订阅策略和原生账号通道。下面是合并到既有完整配置中的片段：

```json
{
  "subscription": { "version": 1, "mode": "subscription-only" },
  "channels": {
    "google-subscription": { "harness": "antigravity", "env-key": null }
  },
  "profiles": {
    "hydrogen": {
      "harness": "antigravity",
      "model": "gemini-4-hydrogen-high",
      "channel": "google-subscription",
      "thinking": "high"
    },
    "neon": {
      "harness": "antigravity",
      "model": "gemini-4-neon-high",
      "channel": "google-subscription",
      "thinking": "high"
    },
    "carbon": {
      "harness": "antigravity",
      "model": "gemini-4-carbon-high",
      "channel": "google-subscription",
      "thinking": "high"
    }
  }
}
```

| Profile | 精确模型／effort | 默认职责 |
| --- | --- | --- |
| `hydrogen` | `gemini-4-hydrogen-high`／high | 常规实现 |
| `hydrogen-local` | `gemini-4-hydrogen-medium`／medium | 精简设计规划；可选订阅评估器 |
| `hydrogen-summary` | `gemini-4-hydrogen-low`／low | Intake 摘要 |
| `neon` | `gemini-4-neon-high`／high | 复杂实现，以及仅分析获准证据的排障分析 |
| `carbon` | `gemini-4-carbon-high`／high | 完整设计规划 |
| `carbon-explore` | `gemini-4-carbon-medium`／medium | 显式探索覆盖 |

普通 Gemini 模型的 slug 后缀与 `thinking` 必须一致，而且模型必须出现在实际 CLI 目录中。模型不可用或
档位不兼容时明确失败，不悄悄切换 provider。请求 effort 与原生观察值分别记录；init
没有回显 effort 时，不能证明其实际生效档位。完整跨 provider 分工见[模型策略](model-policy.md)
和[工作流](workflow.md)。

普通 Antigravity 执行要求该绑定明确授权 full-access，映射为原生 `accept-edits`、
`always-proceed` 与 `sandbox:false`。显式订阅迁移保留既有用户权限；保留的权限不兼容
时给出准入错误，不静默扩大权限。该后端契约不接受原生图片附件；多模态实现走配置中的 Claude 路由。

### 图片与自动化输入边界

**使用 Gemini 或 Swarm 不要求有图片。** 只有 packet 明确要求输入原始图片字节时，才受当前通道能力限制。官方[交互式 CLI](https://antigravity.google/docs/cli/prompting/)支持粘贴媒体，但当前[headless stream-json 输入](https://antigravity.google/docs/cli/headless/)只接受文本。交互式支持不能证明本适配器已实现自动化原图传递或子代理消费。

Flywheel 任务表单可检查并预览用户明确选择的仓库图片，把路径与 SHA-256 作为未审证据加入简报；文本路径不会自动变成附件，各阶段也不会因此获得原图。经审查的原始图片 packet 继续使用已验证的 Claude SDK 通道；文本工程任务与排障任务可正常使用 Gemini 及符合资格的原生协作。

## Orchestrator、子代理和 Swarm

Orchestrator 是模型选择，由官方 CLI 在同一个会话中把工作交给指定规划模型和 Hydrogen。
三个固定档位都使用 `thinking:""`；适配器不传 `--effort`，非空 effort 会被拒绝：

| 模型 | 原生规划模型 | effort |
| --- | --- | --- |
| `gemini-pro-orchestrator-neon` | Neon | 固定 |
| `gemini-pro-orchestrator` | Carbon | 固定 |
| `gemini-pro-orchestrator-argon` | Argon | 固定 |

子代理和 Swarm 是与模型分开配置的原生执行方式。将下面片段加入完整 Flywheel 配置，
在阶段或候选绑定中通过 `native-profile` 选择：

```json
{
  "native-profiles": {
    "antigravity-subagents": {
      "harness": "antigravity",
      "options": {"version":1,"mode":"subagents","maxObservedAgents":4}
    },
    "antigravity-swarm": {
      "harness": "antigravity",
      "options": {"version":1,"mode":"swarm","maxObservedAgents":4}
    }
  }
}
```

`native` 保留 CLI 隐式行为，不表示关闭子代理。`subagents` 明确要求委派并在当前调用
归集结果，准入时检查原生委派工具；`swarm` 使用官方 `/swarm` 入口。命令前缀只由
适配器生成，用户内容置于转义斜杠的 JSON 负载，其他模式仍禁用 slash commands。
显式协作只有实际流中出现子会话元数据才能成功；已发请求、选了 Orchestrator 或根调用
成功终态，都不能充当观察到子代理的证明。

`maxObservedAgents` 为 1–8，观察到超量的不同子会话后停止调用，计入已观察的
verifier／monitor。它不是原生并发上限或整棵子树硬上限，回执明确 `hard:false`。
未公开的成员角色、模型、状态和用量保留 unknown。UI 分开展示请求策略和实际观察成员，
不会生成缺少事实依据的完整 Swarm task board。子成员元数据另有累计保留字节上限；
越限保留已验证成员 ID，标记 `metadataTruncated:true` 并使调用失败，不将缩减过的记录
静默报告为成功。

Flywheel Execute 的 Swarm 候选，无论 JEV 选择还是静态指定，都必须满足 Full、无图片、
已有审查过的 `collaboration` 契约。契约包含 2–8 个独立子任务，写入范围互不重叠且处于
packet 获准的 `schedule.writes` 内。规划阶段可以用前台子代理收集只读调查，不运行执行
Swarm。原生 verifier 不能代替独立 Flywheel Review 或方案精确版本审批。
参见[工作流](workflow.md)和[模型策略](model-policy.md)。

依据：[官方模型](https://antigravity.google/docs/models)、
[CLI 子代理](https://antigravity.google/docs/subagents?tab=cli)及
[Swarm](https://antigravity.google/docs/swarm)。

## 排障分析只消费获准证据

排障工作流把确定性的证据收集与模型分析分开：Go 执行已经批准的命令／URL 收集，
把证据交给 Neon。分析模型提出需要更多证据，不会因此获得自行执行新命令或访问新 URL
的权限；缺少什么会进入报告，供下一次获准收集使用。

这一阶段由 Flywheel 传入 `toolPolicy:"evidence-only"`、原生 `plan`、`request-review`，
并使用明确的 `loops.investigate` read-only 绑定。插件创建本次调用独占的临时工作区
自定义 agent，工具、MCP、skill、plugin 列表为空且不具备 subagent 能力，通过官方
`--agent` 选择它，在写入任务前等待真实 init：

- `init.agent` 必须等于本次生成的精确 agent 身份。
- `init.tools` 必须存在且为空数组。
- 此阶段拒绝 Orchestrator 模型和原生协作 profile。
- 身份缺失、继承了工具、init 异常，以及之后出现工具／子代理事件，都会拒绝本次调用。

[官方 headless 契约](https://antigravity.google/docs/cli/headless/)定义完整可用工具列表，
以及指定 model／agent 参数后的对应字段；[自定义 agent 契约](https://antigravity.google/docs/subagents/)
定义工具允许列表。CLI 无法提供这些证据就不准入，不能只靠 plan mode 或提示词限制。

临时文件通过独占创建、调用 owner 和内容身份管理，只清理本次创建且未被更改的文件；
用户已有 agent 与全局设置都不会覆盖。这是原生工具排除，不是 OS 文件系统沙箱；
原生初始化和自定义配置发现仍会发生，记录与回执保留这一边界。

## 原生订阅控制

所有 Antigravity 调用都要求版本化订阅策略。适配器拒绝 API／provider 路由、ADC、GCP、
自定义模型配置，过滤替代凭证／endpoint 环境，并检查原生 `useG1Credits:false`
（包括文档规定的默认值）。这一设置禁止计划额度耗尽后使用 AI credits。相关配置生成
指纹，并在派发与完成前后复查；配置漂移会阻止结果被接受为成功。

检查通过后回执给出 `subscription.status:"verified-local-controls"`、
`creditsFallback:false` 和配置指纹。官方接口未暴露的真实账号身份、权益、剩余额度
和额外计费仍为 unknown；模型目录不是额度证明。登录或额度失败直接返回，不走 API、
credits 或替代模型 fallback。原生运行时已授权的 hooks 与外部工具不受账户级计费锁隔离。

依据：[原生登录与 API／ADC 路由](https://antigravity.google/docs/install/)、
[CLI 设置](https://antigravity.google/docs/cli/reference/)和
[credits fallback](https://antigravity.google/docs/cli/commands/credits)。

## 在 DSH 中直接使用

将精确审查过的插件产物安装到目标 DSH profile：

```sh
dsh plugin --profile <profile> add /absolute/path/ckanner-dsh-subagent-antigravity-0.2.0.tgz
```

打开 **Antigravity** 会话视图，选择工作区、名称、完整任务、模型、effort 和原生策略，然后启动任务。
固定档位 Orchestrator 使用空 effort。
**刷新任务**、**查看结果**、**取消**作用于本会话拥有的调用；插件设置／面板中可以检查
环境、保存默认配置。视图直接调用公开 DSH command，无需先让父模型调用委派工具。

对应命令提供版本化的 `start`、`list`、`status`、`result`、`cancel`、`doctor`、`defaults`：

```text
/antigravity {"version":1,"action":"start","label":"排查超时","prompt":"分析以下已收集证据…","model":"gemini-4-neon-high","effort":"high","toolPolicy":"evidence-only"}
/antigravity {"version":1,"action":"start","label":"独立模块","prompt":"按两个互不重叠的写入范围实现模块，归集两个结果并验证。","model":"gemini-4-neon-high","effort":"high","native":{"version":1,"mode":"swarm","maxObservedAgents":4}}
/antigravity {"version":1,"action":"list"}
/antigravity {"version":1,"action":"result","invocationId":"<返回的 UUID>"}
```

Agent 委派需要 preset 明确注册 `subagent_antigravity`，并满足插件 preset 允许列表。
后台委派使用 DSH jobs，配套 `job_output`、`job_kill`；直接面板任务使用按 owner 绑定的
任务存储。工具参数不能扩大原生权限，保存设置只影响之后的调用。

在已有 DSH 对话中调用 subagent，父对话仍使用自己的 provider；直接面板／命令不会
先调用父模型。本包承担完整 Agent 传输，不要注册成普通 DSH LLM provider。
DSH schema、公开 machine API 和 CLI 包装用法见独立
[插件指南](https://github.com/ckanner/dsh-subagent-antigravity/blob/main/README.zh-CN.md)。

## 生命周期、历史与用量

每次调用拥有一个 CLI 进程和一个文本轮次。原生会话保留历史：`fresh` 创建新会话，
scoped 恢复只接受精确自有、已终态、已清理，且目录、模型、effort、权限、工具策略、完整 native 选项和
原生设置一致的 conversation。并发恢复和未决恢复会拒绝，不猜“最近会话”、不预热池、
不隐式重放。面板保留可读本地名称和原生 conversation ID，不声称改写原生会话名称。

提交任务输入前，supervisor 写入持久化 intent／possible-dispatch 检查点并等待 Go
确认。非终态事件不宣称清理完成；每次本地调用结束先写 terminal 检查点，原生
`terminalReceived`、outcome 和 provider-work 证据分别决定 provider 是否完成。
之后的 cleanup 检查点只证明本地进程组已清理。父进程退出、取消和超时会收束自有本地
工作，但不会把未观察到的 provider 工作写成已完成。`recoverMachine` 只读原 journal，
不启动 CLI 或重发输入。

成功必须同时满足原生成功终态、没有非空 `finished_reason`、输出／schema 有效、可观察配置一致以及清理确认。
exit zero、软权限拒绝或连接超时均不能证明成功。token 保留原生会话／部分／未知范围，
实际成本为 `null` 而非零。CLI 调用结束和已观察成员元数据不证明整棵远程子树都完成或
已清理。本版本不宣称完整子任务用量、远程完成证明、图片／PDF 输入
或产物返回通道。

离线协议、取消、父进程退出、evidence-only、公开 API 和真实候选安装检查，与真实账号
或 macOS 验收分别记录。详见[恢复](session-recovery.md)、[发布验证](releasing.md)以及
插件的[验证记录](https://github.com/ckanner/dsh-subagent-antigravity/blob/main/docs/VALIDATION.md)。
