# 配置指南

[文档首页](../README.zh-CN.md) · [English](../en/configuration.md) · 简体中文

Flywheel 启动时选择一份完整配置，展开模型 profiles，再解析阶段绑定。模型分配、
工作流选择、权限、调度和会话历史是独立策略。安装保留已有文件，修改配置后需重启引擎。

这些引擎设置管理正式 Flywheel 任务。默认的个人 Antigravity ACP 视图使用独立的 DSH
插件设置 `config.antigravity`，见[下文](#antigravity-与-gemini)；它不读取这里的角色绑定。

编辑前先用 `config inspect` 查明生效文件。下文只展示部分字段的示例是需要并入完整
配置的**片段**。

- [配置来源](#配置来源)与[完整模板](#从完整模板开始)
- [模型字段](#字段与身份)与[订阅预设迁移](#将已有配置迁移到订阅预设)
- [仅订阅渠道策略](#仅订阅渠道策略)与[默认模型分工](#默认模型分工)
- [权限](#完全权限与无需逐步确认)与[调度](#调度与-worktree)
- [会话历史](#会话策略历史复用与调用安全)与[工作流策略](#工作流策略与恢复)
- [后端接入](#codex-pro-与旧配置迁移)、[预算](#后备渠道与预算)和[预检](#无推理诊断)
- [个人 Antigravity ACP 与独立的 machine harness](#antigravity-与-gemini)


## 方案规划角色的绑定

新任务使用独立 `design-plan`、`design-review`、`plan-review` 角色。缺省
`loops.design-plan` 继承 `loops.plan` 后端，两个新审查角色继承 `loops.review`。
显式 `loops.<role>` / `drivers.<role>` 覆盖来源绑定；profile、model、effort 与 native
profile 随来源解析，config inspect 的 `bindingSource` 明确展示继承。显式绑定中的
`native-profile:""` 可关闭继承的原生能力。桌面配置页展示继承行，修改后只保存该
角色的独立覆盖。

角色身份、历史及权限不会继承来源角色的覆盖。`sessions.roles` 可以独立设置这三个
新角色。`role-based` 下新审查角色默认只读；Claude Code Pro 后端需要已有授权的
full-access，或分别给 `permissions.bindings` 中 `loops.design-review` 和
`loops.plan-review` 配置 `danger-full-access`。doctor 会报告不支持的权限组合，
不会自动扩大权限。跨渠道 fallback 仍须显式 `channels.<channel>.models.<新角色>`，
继承不扩大原来的付费渠道和模型授权。审批与作用域 Plan 见
[方案设计与审查](reviewed-planning.md)。

最终预设显式绑定这些角色，并提供紧凑方案的两项 profile 引用：

```json
{"planning":{"compact":{"design-plan":"hydrogen-local","design-review":"sol-plan"}}}
```

值必须是 `profiles` 中的名称，不能写内联绑定。方案 brief 的
`design-granularity:"compact"` 选择这组方案起草／方案审查绑定；system 仍使用
`loops.design-plan` 与 `loops.design-review`。紧凑方案仍产出可审阅的设计，经过相同
审批门，不会因此自动选择 Quick、跳过作用域 Plan 或取得实现授权。旧配置没有
`planning.compact` 时继续使用原角色绑定。方案粒度、Auto／Quick／Full 执行流程、
review-first／delegated 审批策略是三个独立维度。


## 配置来源

发现顺序为任务目录、项目 Git 根目录、全局文件、内嵌默认表，选择第一个存在的完整文件。全局默认路径为 `~/.config/flywheel/flywheel.config.json`；`FLYWHEEL_CONFIG` 可覆盖全局文件路径，但不能越过已经存在的任务／项目配置。当前不通过 `XDG_CONFIG_HOME` 改变该默认路径。

高优先级文件不可读、JSON 无效或符号链接失效时直接报错，不悄悄使用其他账号或模型配置。不同文件的 profiles、loops、routes、channels 不逐字段合并。

```sh
flywheel config inspect --cwd /repo --task-dir /repo/docs/task --json
```

输出 `source`、`scope`、物理 `cwd/taskDir`、`sessions` 策略和按阶段排列的 `bindings`。每个绑定的 `session` 包含所选模式、适配器支持情况、原因以及恒为 false 的 `historyRecoveryTested`。`scope` 为 task／project／global／embedded。它只解析配置，不物化全局文件，不读取账号 token，不启动模型，也不创建任务状态。省略任务时只从项目及更低层发现配置，不会误读运行命令所在的另一个目录。

展示值包含 Flywheel 已确定的 driver 默认：DSH／Codex／Kimi 的空 thinking 为 high，pi／omp／zcode 为 max；Kimi／Codex／Zcode 的空 model 使用各自 driver 常量。pi／omp 未指定模型时仍留空，表示 Flywheel 不能从配置确定它。`residentSession` 只有 Plan＋Kimi＋明确启用时为 true，不把其他阶段忽略的请求显示为已生效。

### 在控制台编辑配置

DSH 的「全局 / 项目配置」先读取显式作用域和文件 revision，再编辑模型绑定、角色会话策略、预算、超时、重试或全局权限。在表单旁审阅改动并保存。目标文件被其他编辑器修改时拒绝覆盖，保存结果未确认时先重新读取核对。新项目文件以当前版本默认模板创建完整配置，项目文件优先于全局，任务文件优先于项目，不按字段合并。

**模型与会话 → 完整执行绑定**可以新建、复制、编辑和移除候选，包括 Provider、精确
模型、effort、原生能力、权限和会话策略。界面列出所有引用路线；直接编辑共享候选
影响这些路线，复制出的新候选则要明确加入路线允许集合后才参与执行。有引用的候选
不能直接删除，完整绑定总数仍最多 32 项。

**执行候选**可以编辑实际的 `execution-policy.routes.<route>.initial`、允许集合、
路线授权和 0–32 次切换上限。选择初始项保留完整绑定；阶段表中编辑路线模型会隔离
其它路线共享的初始候选。明确更改路线授权会更新该路线全部允许候选，先复制其它
路线共享的候选，再同步本路线引用；其它路线权限不变。直接编辑候选权限后如果与
引用路线授权不符，必须明确同步授权或恢复候选权限后才能保存；允许候选可以分别
选择会话策略。旧路线可使用当前完整配置启用执行策略，初始切换次数为 0。

**质量调整次序**提供加入、移除和上移／下移，不把用户指定顺序当作模型能力排名。
版本 1 在首次确认停滞比较后调整。**渠道后备规则**选择另一个渠道的允许候选，并
展示必须配套的共享渠道链、后端和路线模型映射。应用映射是单独的显式草稿操作；
映射未匹配、循环或其它规则冲突会阻止保存。移除规则会保留共享渠道后备链，所有
规则引用解除后可再显式移除渠道链。保存前审阅差异中的共享影响。初始、质量和后备
引用都需先移除，才能从允许集合中移除相应候选。

阶段或候选编辑器中的 **读取模型目录** 按需读取当前原生客户端的精确模型 ID。
现有来源包括 `agy models`、Codex app-server 的 `model/list` 和 Claude SDK 初始化
元数据。不支持的后端或不可用的客户端仍保留手动输入。界面显示来源、原生客户端
范围、局限和未知账号资格；读取不启动推理。只有目录明确返回时才使用其 effort
选项／默认值，当前受支持的 effort 保留；目录未列出的模型仍按精确手动 ID 保存。
Provider、渠道、任务或配置范围改变后，过时返回不会应用到新的选择。

上述控件共用有 revision 检查的草稿和「保存修改」，不改写当前任务的冻结配置。
高级 JSON 保留完整配置及尚不支持图形编辑的后端专属原生 schema。

**JEV** 分别显示「正在编辑的文件」和「当前任务实际采用的配置」，包括作用域、
开关与解析后的订阅模型／effort。每次确认读取编辑文件或保存成功后，界面会通过
独立只读请求重新核对有效配置；核对失败显示「尚未确认」，不把已确认的保存改报为
失败。修改全局开关不会越过项目或任务配置，结果未知的保存也不会自动重试。
五类允许发送的数据可以在此勾选，勾选本身不授予工具或执行权限。

高级调用通过 stdin 传入 JSON。读取示例使用 `flywheel control config-edit --cwd /absolute/repo`，输入为 `{"scope":"project"}`。保存使用 `control config-save`，输入包含 scope、读取返回的 revision，以及仅包含改动章节的 values。reset=true 从内嵌默认完整模板开始替换。配置保存创建备份并原子发布，下一次引擎启动时加载。

## 字段与身份

| 字段 | 含义 | 示例 |
|---|---|---|
| `harness` | 实际工具循环与进程后端 | `antigravity`、`kimi`、`codex-pro`、`claude-code-pro`；旧配置仍可显式选择 `dsh`、`codex` 等兼容 driver |
| `provider` | DSH SDK 已注册的 LLM adapter ID；仅 DSH 使用 | `deepseek-account`、`chatgpt`；原生订阅绑定省略此字段 |
| `channel` | 配置中的凭证／后备／计量渠道名称 | `google-subscription`、`claude-native`、`chatgpt-subscription`、`kimi-subscription` |
| `model` | 传给后端的精确模型标识 | `gemini-4-hydrogen-high`、`claude-opus-5-5`、`gpt-6.2-sol`、`kimi-code/k3.1-pro` |
| `thinking` | 请求的推理等级 | `high` |
| `profile` | 当前 Flywheel 配置内的一组模型字段引用 | `hydrogen`、`opus-plan`、`sol`、`kimi` |

Flywheel 的 model profile 与 DSH 的 `sdk`／`desktop` profile 是两件事。前者复用模型定义，后者决定宿主实际加载的插件和授权配置。channel 别名本身不会切换 DSH 账号；给同一个 provider 起两个 channel 名称不等于拥有两份独立订阅或额度。

旧 DSH 绑定的实际 provider 按 `binding.provider` → `channels[channel].provider` → `channel` 名解析。前两者都明确配置而不相等时拒绝加载。原生订阅后端使用各自登录状态，不接受一个会被静默忽略的 DSH provider。

thinking 必须符合后端实际枚举。Antigravity 的 slug 和 effort 必须一致，例如
`gemini-4-hydrogen-medium` 配 `thinking:"medium"`。配置名称表示请求的绑定，
不证明账号权益，也不证明该模型已经通过项目质量评估。

## 仅订阅渠道策略

新完整模板包含：

```json
{"subscription":{"version":1,"mode":"subscription-only"}}
```

此策略检查所有可达 loops、routes、已启用第二意见、紧凑方案 profiles、
execution-policy 绑定和后备渠道。仅允许 `antigravity`、`kimi`、`codex-pro`、
`claude-code-pro`，绑定及可达 channel 均不得设置 DSH provider 或非空 `env-key`。
循环后备链、未声明渠道和冲突的 harness 在加载时拒绝。启用的 JEV 必须选择
subscription 后端。未引用的自定义定义可以保留，但不会因此成为后备渠道。

worker 在提交任务前继续检查实际原生路由和配置。不支持的接口、冲突的 API 路径、
不可用的选定模型都会明确失败，不自动增加 API Key、credits、其他模型或 provider
后备。这项策略不会登录账号、修改账号设置、证明剩余额度或锁定调用之外的计费。
各后端可验证的证据不同：Claude 要求同一 CLI 进程的 usage 回应明确额外用量已关闭；
Antigravity 检查本机 credits 后备控制；Codex、Kimi 在官方接口未暴露账号级额外
用量状态时保留 unknown。具体边界见 [Claude 准入](claude-code-pro.md)、
[Antigravity 控制](antigravity.md)和 [Codex／Kimi 准入](native-capabilities.md)。

已有完整文件省略 `subscription` 时保留显式选择的旧 DSH／API／CLI 行为。DSH
ChatGPT provider 仍在这条兼容路径上受支持，但不能在 `subscription-only` 下
替代 Codex Pro 的原生准入契约。普通升级不会偷偷开启这项策略。

### DeepSeek 账号与 API Key 是两个 provider

| 方式 | Provider | 凭证来源 | 选择方式 |
|---|---|---|---|
| DSH 已登录、已充值的 DeepSeek 账号 | `deepseek-account` | DSH 的 `deepseekAccount` 服务 | 未启用 `subscription-only` 的显式旧配置；不要求 `DEEPSEEK_API_KEY` |
| DeepSeek API Key | `deepseek-official` | DSH credentials／启动环境中的 API key，默认引用 `DEEPSEEK_API_KEY` | 未启用 `subscription-only` 的显式 API 配置 |

这两条路径都不属于当前默认分工。本节说明已有配置及旧 `subscriptions` 迁移预设
的兼容背景；账号充值余额不等于订阅额度。

标准 SDK profile 叠加的 [base](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/bundle/base/cordis.patch.yml) 包含账号服务和这两种 adapter。标准桌面与 SDK 使用同一 `DSH_HOME` 时，默认共享该 home 下的[账号授权存储](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/credentials/credentials-local/src/index.ts)；profile 隔离插件配置，不要求把 token 复制到 `sdk` 或改用 `desktop` profile。自定义 home、凭证存储或 SDK 插件集时，须核对它们指向同一授权来源。这里不读取或展示凭证值。

DSH 桌面在登录后且没有任何 provider API key 时，可以自动选择账号目录里的第一个模型。该桌面默认不决定独立 SDK 会话的 provider/model，且“第一个模型”不保证是 Flash。显式配置的 Flywheel DSH 调用会传入所选 provider 和 model；旧预设曾选择 `deepseek-account`／`deepseek-flash`，没有 API Key 后备。用户显式配置的旧后备链仍按其原有策略处理。已核对的[官方账号 adapter](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek-account/src/index.ts)、[API adapter](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek-api-key/src/index.ts)及[默认选择逻辑](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/api/session-controller/src/index.ts)是这一边界的依据。

需要继续用 API Key 的自定义配置仍受支持：在完整配置的相关绑定及其 channel 中一致选择 `deepseek-official`，按 DSH API provider 的正常方式配置凭证。不要仅改 channel 名却保留冲突的显式 provider；也不必为账号路径额外添加 API key。

## 原生能力配置

`native-profiles` 定义版本化 harness options，绑定使用 `native-profile` 引用。
模型 profiles、loops、routes、judge-panel 和 execution-policy 的允许绑定均支持，
引用必须与实际 harness 匹配。省略可继承模型 profile，显式空字符串关闭继承。
静态能力不依赖 JEV 开关；工作台按实际 harness 和逻辑角色选择配置。Antigravity
Swarm 在 Plan、Review、Judge 和紧凑方案角色中禁选，配置加载也拒绝这些不合法的
引用。Execute 显示运行时仍须核对的条件：Full、已审批且有 2–8 项独立写入分工的
collaboration 契约，以及没有原生图片输入。配置 Swarm 候选不代表当前 packet 已
满足资格。没有图片的任务可以使用 Antigravity；当前正式 machine harness 尚无它的
原生图片输入通道。独立的人工 ACP 视图可在原生会话声明支持时发送图片；这不会给
machine packet 或 Swarm profile 增加图片能力。

阶段绑定详情提供 Antigravity 的 `native`、`subagents`、`swarm` 和可选的
`maxObservedAgents`（1–8）控件。原定义被共享时，编辑会创建此目标专属的 profile，
保留其它角色、候选、权限和会话。该上限统计观察到的 child ID，包含 verifier／
monitor；超过后请求停止，不是硬并发限制，也不证明全部子代理已经结束。请求的方式
与实际观察到的协作仍是两种运行事实。其它复杂原生定义继续通过高级 JSON 编辑。
实际 options 进入历史及冻结策略身份，准入、用量和清理仍取决于运行回执。
完整示例、LCU、Tower 和独立 Review/Team 入口见[原生能力](native-capabilities.md)。

默认 `kimi` 模型 profile 引用 `kimi-subscription-agent`：

```json
{"native-profiles":{"kimi-subscription-agent":{"harness":"kimi","options":{"version":1,"mode":"agent","agents":{"enabled":false}}}}}
```

它使用原生 Agent 路径，并关闭子 agent。订阅策略下，worker 检查实际路由，并关闭
无法区分凭证来源的服务工具；旧 print、Swarm、Tower 和自定义子 agent 配置不能
替代这条路径。Garden 和 batch 执行共用该 profile。无订阅策略的旧配置保留原路径。
Antigravity 支持 version 1 的 `native`、`subagents`、`swarm` profile，以及可选
`maxObservedAgents`；仅分析证据的契约由引擎选择并拒绝原生协作。固定档位 Orchestrator
和协作设置见 [Antigravity](antigravity.md)。

## 从完整模板开始

两个模板名称都提供包含可复用 profiles 的当前完整订阅 CLI 配置：

```sh
flywheel template flywheel.config.example.json
flywheel template flywheel.profiles.example.json
```

确认目标没有需要保留的配置后，选择其中一份导出到项目根。已有配置先备份并审阅差异，避免 shell 重定向覆盖它。可以使用 Bash 的 noclobber 避免覆盖已有普通文件：

```sh
(set -C; flywheel template flywheel.profiles.example.json > flywheel.config.json)
```

不要只写几个 profiles 后期待继承另一个文件的内容。下面只是解释结构的片段，需并入完整配置；两份模板均可作为完整起点：

```json
{
  "profiles": {
    "hydrogen": {
      "harness": "antigravity",
      "channel": "google-subscription",
      "model": "gemini-4-hydrogen-high",
      "thinking": "high"
    }
  },
  "loops": {
    "intake": { "profile": "hydrogen", "model": "gemini-4-hydrogen-low", "thinking": "low" }
  },
  "routes": {
    "default": { "profile": "hydrogen" }
  },
  "channels": {
    "google-subscription": { "harness": "antigravity", "env-key": null }
  }
}
```

`loops` 绑定方案规划、Plan、Review、Judge、Intake、Garden 和调查分析；`routes`
选择 packet 的 Execute 后端。`drivers` 选择 real／mock driver，不是 provider 映射。

### 默认模型分工

| 阶段或路由 | Profile | 精确模型 | Effort | Harness |
|---|---|---|---|---|
| 系统方案 | `carbon` | `gemini-4-carbon-high` | high | `antigravity` |
| 系统方案审查 | `astra` | `gpt-6.2-astra` | high | `codex-pro` |
| 紧凑方案 | `hydrogen-local` | `gemini-4-hydrogen-medium` | medium | `antigravity` |
| 紧凑方案审查 | `sol-plan` | `gpt-6.2-sol` | medium | `codex-pro` |
| 作用域 Plan | `opus-plan` | `claude-opus-5-5` | high | `claude-code-pro` |
| Plan 审查 | `sol-plan` | `gpt-6.2-sol` | medium | `codex-pro` |
| Execute `default` | `hydrogen` | `gemini-4-hydrogen-high` | high | `antigravity` |
| Execute `complex` | `neon` | `gemini-4-neon-high` | high | `antigravity` |
| Execute `frontend`／`multimodal` | `opus-ui` | `claude-opus-5-5` | medium | `claude-code-pro` |
| Execute `backend` | `sol` | `gpt-6.2-sol` | high | `codex-pro` |
| Execute `batch` | `kimi` | `kimi-code/k3.1-pro` | high | `kimi` 原生 Agent |
| Execute `hard` | `astra` | `gpt-6.2-astra` | high | `codex-pro` |
| Review | `sol` | `gpt-6.2-sol` | high | `codex-pro` |
| Judge | `astra` | `gpt-6.2-astra` | high | `codex-pro` |
| Intake | `hydrogen-summary` | `gemini-4-hydrogen-low` | low | `antigravity` |
| Garden | `kimi` | `kimi-code/k3.1-pro` | high | `kimi` 原生 Agent |
| 调查证据分析 | `neon` | `gemini-4-neon-high` | high | `antigravity`，仅分析证据 |
| 可选 JEV 选择器，默认关闭 | `hydrogen-local` | `gemini-4-hydrogen-medium` | medium | `antigravity`，仅分析证据 |

`fable`（`claude-fable-5-2`，high）和 `carbon-explore`
（`gemini-4-carbon-medium`，medium）供显式覆盖使用。模板不启用第二意见、渠道后备
或付费 API 绑定。四个原生渠道均为 `env-key:null`，不设置 `provider`。此表描述配置
分工，不代表项目质量已测或账号权益已确认；依据与验收边界见[模型策略](model-policy.md)。

### 覆盖与错误规则

| 情况 | 行为 |
|---|---|
| 阶段省略某字段 | 继承所引用 profile 的字段 |
| 阶段显式写 `model`／`channel`／`thinking` 等 | 只覆盖该阶段，不修改共享 profile 或其他阶段 |
| `thinking: ""` | 仅在后端允许省略时使用其默认 effort；Antigravity 要求显式填写并与 slug 一致 |
| Plan `resident-session: false` | 明确关闭 Plan 历史复用；加载、profile 展开和配置导出均保留其显式存在 |
| `judge-panel: null` | 关闭第二意见，保持默认行为 |
| profile 定义缺 `harness/model/channel`，或这些字段为空／null／纯空白 | 加载失败 |
| profile 不存在、未知字段、错拼阶段名、不支持的 harness | 加载失败，不静默回落 |
| profile 再引用另一个 profile | 不支持继承链；加载失败 |
| `_note`、`_note_*` | 允许配置对象中的说明文字，不进入执行绑定 |

`resident-session` 是既有阶段选项，不能放进 profile。它继续控制旧 Kimi Plan 行为，
其显式 false 在 scoped 下也保持 Plan 退出；其他角色的历史复用由顶层 `sessions`
控制。恢复的是历史，不是要求一个模型进程永久存活。

旧内联绑定保留原有缺省行为，不强制迁移。`env-key: null`、`retry.jitter: 0`、`worktree.enabled: false` 等已有合法零值继续支持。显式切换 harness 或 channel 时，要一起核对继承的 provider；必要时用 `provider: ""` 清除显式值，再按新 channel 解析，不能保留另一个 adapter 的 ID。

## 将已有配置迁移到订阅预设

当前 Gemini、Claude、Codex 和 Kimi 分工使用 `subscription-cli` 预设。普通安装／
升级仍保留已有文件。旧名称 `subscriptions` 保留其原来的 DeepSeek 账号／DSH
映射，用于兼容已有配置，不会启用新的原生订阅策略。

先用已安装的二进制执行 `flywheel template flywheel.config.example.json`，确认它实际包含所需预设；旧二进制可能仍包含旧默认值。版本变化见[变更日志](../../CHANGELOG.md)。

在实际项目目录运行以下命令；若任务目录有单独配置，再加 `--task-dir /repo/docs/my-task`：

```sh
flywheel config migrate --preset subscription-cli --cwd /repo --json
flywheel config migrate --preset subscription-cli --cwd /repo --apply --expect-sha256 PREVIEW_SHA256 --json
flywheel config inspect --cwd /repo --json
```

第一条只预览。把 `PREVIEW_SHA256` 换成返回的摘要；`--expect-sha256` 可选，用于
将应用绑定到这份精确源文件。`--apply` 更新实际命中的任务／项目／全局文件，并先
备份原始字节。没有文件时写全局配置，不创建意外的项目覆盖。源文件变化、配置损坏、
额外自定义路由／阶段或不支持的文件身份会明确阻断。预算、worktree、验证、重试、
价格、说明及已有权限／会话策略保留；已有文件没有 `sessions` 时，这次显式迁移会
补入当前 scoped 默认值。

预设将其命名 loops、routes、profiles、channels、native-profiles 替换为上表的完整
绑定，包含 Claude 多模态路由；保留说明文字，清除被替换绑定的旧覆盖及所选渠道的
fallback，并写入 `subscription` 与 `planning.compact`。未引用的自定义定义保留。
已有第二意见、execution-policy 和其他可达路径必须通过订阅图检查；冲突的付费绑定
会阻断候选，不会被悄悄删除。

已关闭的旧 HTTP JEV 转为
`{"enabled":false,"backend":"subscription","profile":"hydrogen-local"}`，
保留 timeout、调用预算、阈值和允许数据类别。报告逐字段列出 HTTP 专用的
`endpoint`、`key-env`、`model` 删除项。已开启的 HTTP JEV 在任何写入前阻断迁移；
先明确关闭，再迁移、核对新目标，需要时重新开启。已有 subscription JEV 保留；
缺少 JEV 时补入新的关闭状态默认值。对未变化文件重复迁移不会再次写入。

迁移不会扩大已有 `role-based` 权限。完成后运行 doctor；普通 Antigravity 执行和
Claude 需要明确授权的完全权限绑定。若符合使用意图，应另外审阅并显式修改权限。
权限冲突不会通过偷偷换 provider、移除订阅策略或提升 worker 权限来解决。
只有缺少 `loops.investigate` 权限覆盖时，迁移才为新分析角色补入 `read-only`，
并逐字段报告这一新增限制。原全局 mode 和其他角色不变；已有调查角色覆盖也保留，
若它不是 read-only，报告会要求在运行分析前审阅并手工调整。

迁移不登录账号，也不发模型请求。当前运行中的任务继续使用启动时已加载的配置；新配置在下一次启动生效。检查迁移后的 source／scope／bindings，再按需要停止和恢复任务。

迁移期间不要同时编辑目标文件。迁移锁协调 Flywheel 迁移进程；写入前再次检查文件，再原子替换完整文件，不提供对任意外部编辑器的原子 CAS。报告以 `configurationStatus` 区分拟议配置 `proposed`、重读确认的实际配置 `effective` 和写入后尚未确认的 `unconfirmed`；备份／写入失败不能把新模型显示成已生效。

## 完全权限与无需逐步确认

```json
{"permissions":{"mode":"full-access"}}
```

新安装模板使用这一全局设置，并显式将 `loops.investigate` 绑定为 `read-only`。
`subscription-cli` 迁移保留已有权限策略，不自动升权。已有逐绑定覆盖仍优先。
Codex Pro 通过公开 machine API 映射完全权限；Claude 使用已授权的 bypass 模式；
Antigravity 使用无沙箱的原生 `accept-edits`／`always-proceed`；Kimi 原生 Agent
使用 `yolo`。旧 DSH／Codex 和 Kimi print 保留各自映射。完全权限表示 worker
可以访问当前操作系统用户本来有权访问的文件和命令，不继承 DSH 对话框的权限菜单。

旧文件没有 `permissions` 时保持 `role-based`：Plan／Execute 为项目内写入，Review／Judge 为只读。两种策略都不等待交互审批；只读／项目限制下的 `never` 可能直接拒绝动作，不能把它等同于完全权限。`permissions.mode` 的拼写、类型和枚举都会在启动前校验。

完全权限不会关闭 Flywheel 的任务边界、冻结验收、验证和候选身份检查。评审职责仍是评价已有候选，修改候选或验收条件不能取得有效通过。需要受限权限时显式使用 `role-based`；工作区隔离本身不是 OS 沙箱。

### 逐绑定权限覆盖

`permissions.bindings` 按 `loops.<role>`、`routes.<route>` 或 `judge-panel` 覆盖
全局策略，值为 `read-only`、`workspace-write` 或 `danger-full-access`；未覆盖绑定
沿用全局设置。例如：

```json
{
  "permissions": {
    "mode": "full-access",
    "bindings": {"loops.review": "read-only"}
  }
}
```

字段以[权限 schema](https://github.com/ckanner/flywheel/blob/main/templates/permission-policy.schema.json)为准。各后端必须
支持选定能力；Claude Code Pro 要求完全权限，原生审批设置不提供 OS 只读沙箱。
实际权限变化也会改变会话复用资格。调查分析和订阅 JEV 使用另行验证的空工具
Antigravity agent；这个仅分析证据的契约不宣称提供 OS 沙箱。

## 调度与 Worktree

新安装默认配置选择并行调度，最大并发为 2。已有完整配置没有 `scheduler` 时归一为
**serial**，显式 serial 也保持不变。安装新版不会替换这些配置。
[调度 schema](https://github.com/ckanner/flywheel/blob/main/templates/scheduler-policy.schema.json)将总并发与各渠道并发限制在 1–8。
新旧配置的 `dispatch` 均缺省为 `batch`，`prewarm` 缺省为 false；这两个设置不改变
Quick／Full 流程，也不启用 JEV。

```json
{
  "scheduler": {
    "mode": "parallel",
    "max-parallel": 2,
    "providers": {"google-subscription": 1},
    "dispatch": "batch",
    "prewarm": false
  },
  "worktree": {"enabled": true}
}
```

虽然字段名为 `providers`，其键仍是**实际配置渠道名**，并非 DSH provider ID。
原生 Gemini 绑定使用 `google-subscription`；自定义 channel 别名应填写其实际名称。

并行资格来自 packet 的冻结声明：

```yaml
schedule:
  after: [M01-T01]
  writes: [src/search/, tests/search/]
```

依赖须指向较早的已知 packet。写入范围是规范的相对路径或目录前缀，不支持通配符
或越界。只有就绪且写入范围互不重叠的 packet 有并行资格；缺失声明、范围冲突或使用
live 凭证的单元仍串行。hard 预算在前一调用持久化计量前只接纳一个模型调用。
Review、集成与终验保持串行。

`worktree.enabled` 默认 true，只控制 packet 目录的自动管理，与调度独立。并行且
自动管理时要求独立 worktree，创建失败就停止；旧串行路径可记录原地执行回退。
关闭自动管理后，执行者可共享任务 checkout，或在实现前登记合格的外部 worktree。
共享批次等待 worker 收尾后验证完整共同候选，全部成员通过才可交付；外部 worktree
会被保留。

项目工作区归属与实际执行位置分别记录。原生后端无法独立设置 `workspace-write`
沙箱根时，会在派发前拒绝不支持的目录分离组合，不扩大权限。登记与批次规则见
[控制台指南](flow-console.md#串行与并行)。

### 就绪队列与运行时准备

`scheduler.dispatch` 接受 `batch` 或 `ready-queue`。batch 等待当前有界批次结束后
选择更多任务；ready queue 在一个 Execute／L1 槽位释放后接纳下一项已就绪的独立
packet。适用条件是 Full、并行调度、最大并发大于 1、自动管理 Worktree。Quick、
串行、最大并发为 1 和关闭自动管理的任务保持原路径。共享 checkout 沿用批次规则，
包括执行者之后可能登记外部 Worktree 的情况。

ready queue 的首个窗口必须全部准备并保存目录后才派发；随后补位先准备该项目录，
再占用空闲槽位。准备失败停止新接纳、取消并等待已启动工作；尚未接纳的排队任务
不创建目录或模型调用。依赖仅在交付后满足。Review、重规划和集成等待全部已接纳
worker 收尾，再串行推进；合成候选始终复验。已有总并发／渠道限制、hard 预算、
尝试上限和未知结果恢复约束继续生效。

新补入的 packet 不会恢复或取消兄弟任务仍在运行的模型调用；这些调用及其共同
计费继续由协调器持有。真实未知结果或终态回执保存失败均立即停止新接纳，恢复前
必须核对原调用。

`--pause-before review` 同样适用于独立 Worktree 的并行任务。本轮 Execute／L1
全部通过、worker 停稳后，任务在第一个 Review 和集成前暂停，保留已验证候选。
batch 完成已接纳批次；ready queue 可先按原规则补完固定独立候选队列，再到达这个
暂停点。恢复同一任务时复用仍有效的执行证据，不重复 Execute；后续集成改变候选
时仍需重新验证。

`scheduler.prewarm:true` 允许串行 packet 在 Execute 后运行确定性检查的同时，
初始化一个 DSH Review runtime。初始化不发送模型 prompt，也不创建或恢复 Session；
真正 Review 推理仍等待检查完成与正常准入。并行 Execute／L1 lane 不准备 runtime，
必须越过屏障进入串行 Review 后才可开始准备；共享候选批次不预热。因此主要重叠
机会是串行 Execute 后的 L1 和适用 Quick 检查，不能把每个并行 lane 的验证时间
都算作被隐藏的启动时间。

未使用或身份不匹配的 runtime 必须先完成有界清理，才可继续模型工作。待用实例
在尚未尝试正式派发前达到 60 秒存活上限时，只有精确、持久的未派发回执、SDK 和
整个进程组的清理均已确认，且未取消、正式调用未超时，才可使用同一已持久化请求
身份及剩余时限冷启动一次；这不增加尝试额度或重置预算。未知清理仍阻断。其他
harness 保持普通启动。会话历史续用与运行时准备相互独立，角色历史仍隔离。

已确认的初始化失败保留原错误分类，并遵守正常重试和渠道后备规则。认证或请求
参数错误不会变成传输重试，也不视为允许冷启动回退的正常预热过期。

在“设置 → 运行方式”中，dispatch 使用整批／就绪队列选择框，prewarm 使用独立
开关。表单说明就绪队列尚未满足的配置条件，保留所选值；任务适用性仍由引擎核对。
高级 JSON 也可设置这两个选项。保存后在下次启动生效，当前进程保持
已载入策略；恢复时仍须核对已有调用和共享批次，再派发新工作。待收尾预热仍需原始
未派发 prompt 的清理回执、文件和目录同步，以及 worker 与原进程组退出证明，
关闭 `prewarm` 不能绕过，详见
[预热恢复](session-recovery.md#dsh-预热运行时的重启核对)。两项优化保持可选，
真实延迟、资源消耗与默认采用策略另行验收。离线检查证明行为和恢复正确，不承诺
速度或 token 节省，详见 [P5 实施计划](../plans/2026-11-13-p5-ready-queue-prewarm.md)。

## 会话策略、历史复用与调用安全

```json
{"sessions":{"mode":"scoped","policy-version":1}}
```

新安装的两份配置模板显式选择 `scoped`。已有配置没有 `sessions` 时归一为
`fresh`／版本 1，升级不会改写原文件。显式声明时两个字段均必填；未知模式、未来版本、
null、错拼字段和不完整对象在模型启动前拒绝。字段信源为
[会话策略 schema](https://github.com/ckanner/flywheel/blob/main/templates/session-policy.schema.json)。

`sessions.mode` 控制历史复用，不关闭调用安全记录。Antigravity、Codex Pro、Claude Code Pro、DSH 和 Kimi 的主任务
Plan／Execute／Review／Judge 调用在两种模式下都先记录调用意图，再保存可信终态回执。
`fresh` 不复用通用的角色历史，保持后端原有的新调用方式；仅未配置 `native-profile` 的
旧 Kimi CLI 路径保留 Plan `resident-session` 行为。原生 Agent/Swarm 历史使用 scoped
角色账本，Tower 始终 fresh；`scoped` 在身份兼容且终态满足条件时延续同角色历史。
其他适配器及 mock 仍使用原有调用路径；Intake／Garden 不在该角色会话策略内。
调查分析使用独立的轮次账本，JEV 使用独立决策 journal，二者都不借用执行者历史。

| 角色 | 历史作用域与责任 |
|---|---|
| Plan | 同任务、同里程碑；只修订获准的任务契约 |
| Execute | 同任务、同 packet；普通返修可以延续执行者历史 |
| Review | 同任务、同 packet 和评审职责；与执行者隔离，复审仍检查完整当前候选 |
| Judge／第二意见 | 同任务、同里程碑，各自独立；不能共享主判和第二意见历史 |

Quick 最终 Review 与 Full 单元 Review 的完成要求不同，不能互用旧历史或裁决。
方案审查和审批之后，Quick 执行段的正常首次成功路径只有 Execute 和最终 Review
两次阶段调用。Go 负责所有
会话选择、恢复、重建和计量；技能、模型与桌面控制面不扫描后端历史来猜测会话。

Plan 显式 `resident-session:false` 在 scoped 下仍关闭该角色的历史复用；省略该字段
不等于明确关闭。未选择 `native-profile` 时，`resident-session:true` 保留旧 Kimi CLI
Plan 行为。原生 Agent/Swarm 在全局 `fresh` 下将这个显式 resident 意图映射为 scoped
角色账本；显式 `sessions.roles.plan:"fresh"` 仍优先关闭复用。旧 CLI Session 不会导入，
Tower 始终 fresh。该选项不启用跨 harness 历史复用；Codex Pro 仍拒绝它，为 Codex Pro
选择 scoped 应使用 `sessions`。

复用身份包含实际任务／工作目录、角色／作用域、harness、provider、channel、model、
thinking、权限及后端运行配置引用。home/profile、worker 和命令引用取实际子进程所用
配置，注入凭据只记录名称集合，不保存值。profile 改名而实际绑定不变不使历史失效；
后端、账号渠道、模型、权限或工作目录变化则重新建立合适的历史。Spec 目标、非目标、
需求身份及 packet 的目标、正文、边界或需求映射发生变化也不复用旧任务语义。
里程碑级 Plan、Judge 和第二意见还核对里程碑标题以及所属 packet 的身份和这些目标字段。
兼容的验收修订可以保留已消费的历史，但每次重新提交当前契约和证据。

这些非秘密引用不证明账号主体或所指文件的内容。配置、账号授权或可执行文件在相同
路径被原位替换时，当前策略不保证发现变化；不会读取凭据值来比对账号。实际配置
接受和权限事实仍以本次适配器回执能够证明的内容为限。

已完成但尚未消费的回执采用更严格的规则：Review／Judge 必须匹配原契约、候选与验证
证据，不能用重新验证的记录替换原证据。Plan 获准修改契约时，仍绑定完整调用后候选及
持久化的原始前后保护证据。评审或规划越界改动不能在重启后变成有效结果。

`state.sessions` 保存每个作用域的代次、动作／原因和最新调用事实；每个作用域只保留
一个原子替换的 `.sessions` 回执槽，原始回执不进入 Git。恢复先核对已保存的调用意图、
回执摘要和身份，再消费同一结果，不重复发模型请求或重复计费。预算限制新的派发，
不会把消费已有可信回执算作新调用。历史审计事实通过指标保留。

单槽上限只适用于 Flywheel 恢复回执。后端原生 thread／Session 历史依其自身规则
保留；Flywheel 不会因重建角色历史或结束 worker 而自动删除旧历史。

派发、结果或清理有任何一项未知时，原调用阻止继续执行；换成 fresh、改变适配器或
重启不能绕过。子进程退出只解决进程事实，不证明模型结果已知。`resume-candidate`
只是允许适配器尝试正常续接，不是历史已持久化或任意崩溃后都可恢复的证明。
Codex Pro 0.10.2、Claude Code Pro 0.7.0 和 Antigravity 0.3.0 的主任务调用使用外层 worker v3，
接通公开 typed lifecycle、等待 Go 持久化后返回的 ACK，以及不发送新 turn 的只读
recovery。Codex 可以在配置核对后的 `thread-ready` 阻止新 turn；Claude 在任务前
检查 SDK initialize／版本和要求的订阅证据，但原生 `system/init` 配置消息仍发生在
prompt 之后，不能提供同等的派发前权限检查点。Antigravity 的 evidence-only 和显式
subagents／Swarm 会等待原生 init 再输入任务；普通 native 路径（含 Orchestrator）在
spawn 后输入任务，之后仍核对 init，但不能宣称统一的派发前 thread-ready 闸门。
各后端对缺少证据的事实都保留未知窗口，
不会从 PID 消失或报告存在推断任务完成。详见[中断核对](session-recovery.md)。

`status` 及 `control status` 展示角色、作用域、代次、动作／原因、最新调用状态、
派发／结果／清理、是否复用历史与用量状态。紧凑投影不读取回执内容或历史正文，也不
修改账本；无会话记录的旧任务不被推断为已具备恢复资格。

### 按角色选择与上下文质量

`sessions.roles` 可对 `design-plan`、`design-review`、`plan-review`、`plan`、`execute`、`review`、`judge`、`judge-panel` 分别
选择 `fresh` 或 `scoped`；省略的角色继承 `mode`。显式 `roles.plan:"fresh"`
也关闭旧 Kimi Plan 的历史捷径；显式 `resident-session:false` 继续阻止 Plan 复用。
Intake／Garden 不接受角色覆盖。示例只改变历史策略，不改变模型与渠道：

```json
{
  "sessions": {
    "mode": "scoped",
    "policy-version": 1,
    "roles": {"review": "fresh"},
    "quality": {
      "max-confirmed-turns": 0,
      "max-unproductive-turns": 0,
      "max-run-input-tokens": 0
    }
  }
}
```

质量阈值均默认为 `0`（关闭），不会凭未测得的收益改变旧行为。设为正整数后：

| 字段 | 机械可核对的重建依据 |
|---|---|
| `max-confirmed-turns` | 当前角色历史代次内已确认派发并安全结束的调用次数达到上限 |
| `max-unproductive-turns` | Execute 连续调用前后的权威候选 fingerprint 未变达到上限；这只证明候选没变，不替代语义质量判断 |
| `max-run-input-tokens` | 最近一次 complete 用量回执的单次 run input 达到上限；它是工作量观测，不是原生 context window 的测量 |

只有上一次结果已消费、已入账、派发／终态／清理均确认且历史原本可续用，质量策略
才允许重建。未派发失败不增加已确认 turn 数；回执重放按 invocation ID 去重。
partial／unknown 用量不补零，也不能单凭缺失用量触发输入阈值。原预算、失败次数、
停滞限制、工作区成果和独立验收继续保留。重建只交接本角色的代次／调用引用、原因
与当前权威材料，不复制另一角色的私有历史，不新增模型压缩调用。

`state.sessions.records[*].quality` 与对应指标记录已确认 turn 数、候选未变化次数、
最近输入观测及完整度、代次重建次数和原因。阈值范围由同一 schema 限制；无依据的
未来字段或不合法值在启动前失败。如何对真实任务进行三策略配对评估、选择阈值以及
保留未确认结果，见[会话效果评估](session-evaluation.md)。

### 可信的执行者增量上下文

可选 `sessions.context` 省略或为 `"full"` 时保留完整提示词；显式
`"verified-incremental"` 开启仅针对 Execute 的优化。首轮与重建完整提供权威
上下文；后续同角色续用只有在 Go 复核已消费终态回执、精确绑定、提示词版本、
契约及全部 context/cache 文件内容摘要后，才可省略稳定契约。引用必须在当前
执行目录内可读、为普通文件，每个文件最多 1 MiB。增量包含当前反馈、前后候选
身份、尝试次数和权威引用。

文件缺失或修订、契约变化、旧任务缺少检查点、上下文回执校验失败，均回到完整
输入。模型、effort、权限或显式会话模式变化会重建历史。Plan、Review、Judge
保持完整输入，Reviewer 每次仍检查完整当前候选与全部适用需求。
`state.sessions.records[*].context` 保存模式、身份和原因。这是协议能力，不是
已测得的 token 节省或模型等质量证据。

## 有界执行策略（显式启用）

`execution-policy` 版本 1 独立于 Auto／Quick／Full。省略时保持既有固定路由和
渠道后备。[schema](https://github.com/ckanner/flywheel/blob/main/templates/execution-policy.schema.json)
要求完整绑定及明确允许集合。核对账号和权限后，把以下章节加入实际命中的
**完整配置文件**；不能将其独立放为局部 override 文件：

```json
{
  "execution-policy": {
    "version": 1,
    "bindings": {
      "hydrogen-medium": {
        "harness": "antigravity", "provider": "",
        "channel": "google-subscription", "model": "gemini-4-hydrogen-medium",
        "thinking": "medium", "permission": "danger-full-access",
        "session-mode": "scoped"
      },
      "hydrogen-high": {
        "harness": "antigravity", "provider": "",
        "channel": "google-subscription", "model": "gemini-4-hydrogen-high",
        "thinking": "high", "permission": "danger-full-access",
        "session-mode": "scoped"
      }
    },
    "routes": {
      "default": {
        "initial": "hydrogen-medium", "allowed": ["hydrogen-medium", "hydrogen-high"],
        "quality-order": ["hydrogen-medium", "hydrogen-high"],
        "after-stalled-attempts": 1, "max-switches": 1
      }
    }
  }
}
```

绑定必须完整声明 harness／provider／channel／model／thinking、permission 和
session-mode，非 DSH 的 provider 写为 `""`。版本 1 支持已有持久调用记录的
Antigravity、Kimi、Codex Pro、Claude Code Pro 及旧 DSH。全局订阅策略会排除 DSH
和付费 API 路径。每个绑定权限必须等于该路由已有授权，
所以示例假设路由本来就使用 `danger-full-access`；后端仍须验证模型实际可用性
及公开配置／权限能力。

`quality-order` 是用户声明的实验顺序，不是模型能力排名。只有完整重复 L1
观测确认候选、检查集合、已记录环境及失败均未变化，Go 才可前进一项。版本 1
要求 `after-stalled-attempts:1`，在既有空转停止前考虑切换。AUTH、QUOTA、
传输／协议错误和 unknown 不触发能力升级。原 packet 的 `max-retries` 和任务
token 预算继续生效；切换不清零失败、不增加尝试、不削减最终门禁或 Review。

可选 `channel-fallbacks` 将一个允许绑定 ID 映射到另一个，目标须匹配显式配置
的渠道后备链、目标 harness 和路由模型映射。已有渠道链不能越出允许集合。
每个 packet 的 `max-switches` 同时限制两种切换。Go 在派发前冻结策略身份；
`state.packets[*].execution-policy.history` 分别保存 `quality-upgrade` 和
`channel-fallback` 的完整前后绑定、尝试与原因。并行 packet 各自持有记录。

开启 `jev.enabled` 后，首次 Execute 前和确认结束的失败都可由 JEV 选择合法绑定，
切换分别记录为 `jev-initial-selection` 与 `jev-model-selection`，共用原 `max-switches`
与冷启动规则，不改变渠道错误处理。首次选择发生在业务派发前，与调查、失败决策共享
任务决策账本和预算；每个 packet 只记录一次，恢复时不能为了换一个选择而重新发问。
派发或清理 unknown 必须先对账。
允许绑定仍是明确的资源／权限声明，JEV 开关不会把新 provider、账号或模型加入集合。

订阅默认配置包含七条 route 的候选，保留原初始模型，使用 `quality-order:[]` 和
`max-switches:2`。JEV 默认仍关闭，因此默认执行保持固定初始绑定。完整表见
[模型策略](model-policy.md#首次选择与失败调整)。Go 先筛选资格再让 JEV 选择：图片要求
实际 Claude 图片传输；Swarm 要求 Full 和合法的已审查 collaboration 声明。没有合法
替代项、关闭选择器或决策预算耗尽时保留当前基线。固定档位 Orchestrator 使用 `thinking:""`。

迁移保留既有 execution-policy；只有原来未配置策略时，才给已授权 full-access 的 route
补入默认候选，其余 route 保持固定并附说明。不会因此扩大保留的权限。

`config inspect --json` 在 `execution-policy` 下展示完整声明，普通绑定表仍是
配置基线，当前选择由 packet／session 状态记录。Doctor 既有绑定探测不证明每个
允许的实验绑定均已通过接纳；实际派发仍须通过适配器配置与权限检查。

模型编辑页显示每条 route 实际的初始候选与允许集合。上面的结构化控件可以编辑完整
候选、显式授权、质量次序和渠道后备。修改路线模型保留共享绑定，不改变权限和会话
策略；绑定不完整或授权未匹配不能当作已生效修改保存。高级 JSON 保留完整
`execution-policy` 供核对。

每次切换均冷启动 Execute，仅改变 effort 也不例外；effort 保留在会话身份中。
unknown 或未消费调用先对账。恢复时删除或改变冻结策略会被拒绝：恢复原配置，
或者用新任务运行新实验。会话／适配器回执保留请求值与可得接口接纳事实；无法
确认的 effective effort、用量和订阅成本保持 unknown。工作流、Review／Judge
及交付授权仍是独立边界。

## Watchdog 与规划调用活性

`watchdog` 对能暴露进展的传输使用输出 idle 上限，同时保留绝对的 `wall-timeout-ms` 上限和 `kill-grace-ms` 的 SIGTERM 到 SIGKILL 收尾宽限。默认 idle 300 秒、wall 3600 秒、grace 5 秒；收尾宽限不按角色缩短。Kimi print 没有可靠的输出 idle 信号，因此 driver 不启动 idle 计时器，仍使用 wall 上限和显式取消。

`watchdog.roles.<role>` 可以只覆盖指定角色的 idle/wall；在适用输出 idle 检测的传输上，规划角色默认 900 秒，执行和评审保持 300 秒。这是策略上限，不能证明静默模型已经挂起。示例：

```json
{
  "watchdog": {
    "idle-timeout-ms": 300000,
    "wall-timeout-ms": 3600000,
    "kill-grace-ms": 5000,
    "roles": {
      "design-plan": { "idle-timeout-ms": 900000 },
      "plan": { "idle-timeout-ms": 900000 }
    }
  }
}
```

Kimi print 会缓存 assistant delta、不输出 thinking delta，也不暴露子代理进展，因此前台工作也可能长时间静默。延长 idle 或修改提示词不能补上这个观测缺口。所有 print 角色使用各自生效的 wall 上限（包括角色覆盖），idle 配置不作用于 print；长调用应调整 `wall-timeout-ms`。显式停止、取消和进程组清理保持生效，wall 设为 0 不代表无限运行。

Kimi native worker 会把真实主代理／子代理的 item、step 和生命周期进展作为元信息传给父进程看门狗，不转发思考或工具正文，不用定时器或 WebSocket ping/pong 制造心跳。idle 回调会重新核对最新活动期限，已经排队的旧回调不能误杀刚有新活动的调用。非 native 规划保留前台调查约束，用于收集完整结果，不作为活性保证。

派发／结果为 unknown 的规划调用不能用换绑定、换 fresh、删账本或重放修复；只有清理已确认后才能使用[会话恢复](session-recovery.md)中的显式退役动作，清理未知仍然阻断。未知不是零，也不会被自动改写成已知。

### 缺陷注入的还原期限

缺陷注入的基线与测试命令使用 `verify.command-timeout-ms`。注入后，`git apply -R` 使用独立的取消上下文和有界还原期限：正的命令预算限制在 1–10 秒之间，没有有效预算时使用 10 秒。该期限同时传给 context 与命令 watchdog，之后仍保留原进程清理宽限。这不会延长测试本身，也不能把测试超时当成已检出缺陷；还原未确认时停止运行并保留工作区。

## 工作流策略与恢复

工作流决定运行哪些阶段；模型 profiles、loops、routes 和 `permissions.mode` 仍决定
各阶段使用什么后端与权限。**工作流写在 Spec 的 YAML frontmatter，不写进
`flywheel.config.json`。** 统一 `flywheel-task` 入口默认 Auto，准备阶段保存真实的
代码依据、资格事实与风险，再交 Go 校验。下面仅示意字段，内容须替换为当前任务事实：

```yaml
workflow:
  version: 1
  mode: auto
  reason: "缺陷已定位，沿用现有实现模式及回归验收"
  scope-clear: true
  implementation-known: true
  verification-complete: true
  review-sufficient: true
  evidence:
    - "已检查实际实现、调用方与可复现该缺陷的回归测试"
  risks: []
  not-applicable:
    L2: "没有跨模块协议变化，调用边界由 L1 回归覆盖"
    L3: "没有独立 UI 或外部服务路径，行为由本地函数契约验证"
```

`version` 与 `mode` 必填，其他字段缺省不会产生 Quick 资格。Quick 要求一个里程碑、
一个 packet、四项事实标志均为 true、非空 reason/evidence、无 risks、每条 Spec
需求被 packet 引用且有非空 verification、至少一条 L1，以及缺失 L2/L3 的不适用理由。
Go 根据可核验结构与声明选择，不能把一个模型的“低风险”标签当成语义证明；
新风险仍可在执行或最终评审时升级。存在风险或资格不足时采用 Full。

```sh
flywheel control launch --task-dir /repo/docs/task --cwd /repo --mode auto
flywheel control start --task-dir /repo/docs/task --cwd /repo --mode full
flywheel run /repo/docs/task --mode full
flywheel control status --task-dir /repo/docs/task --cwd /repo
```

`launch` 默认请求 Auto；`start`／`run` 省略 `--mode` 时沿用契约或状态的选择。
只读 status、stop 不接受模式切换。旧契约或旧状态没有工作流字段时保留 Full。
已保存的策略在恢复时继续使用，Auto 不重新分类，Full 不降回 Quick；Quick 发现
新风险、验收缺口或显式选择 Full 时，由引擎先保存升级检查点，再继续完整流程。
未知工作流版本明确报错。不要手改 state 或重新建任务来重置预算与证据。

`state.workflow` 保存 requested、effective、reason、evidence、risks、版本、选择时间、
`stages.plan/judge` 适用性及 upgrades。Plan／Judge 不适用不表示验收通过。
工作流与完成要求进入证据身份，旧 Quick 最终 Review 不能充当升级后 Full 的 Judge。
新任务可在 packet 的 `done-report-format` 声明 `workflow-risks: []`，让执行者报告
具体新风险；模型和 skill 不直接修改模式或状态。

Quick 复用现有 Execute 与 Review 绑定，不自动改变 thinking、账户、后备渠道或
全访问设置。全部必需验证仍运行，适用 L2/L3 必须在实际候选中、最终 Review 前完成。
完整最终 Review 逐项核对需求，执行段正常首次成功只调用 Execute 与最终 Review
两个阶段；方案审查和审批门发生在这一执行段之前。
字段信源为 [Spec schema](https://github.com/ckanner/flywheel/blob/main/templates/spec.schema.json) 与
[state schema](https://github.com/ckanner/flywheel/blob/main/templates/state.schema.json)，运行流程见[工作流指南](workflow.md)。

## Codex Pro 与旧配置迁移

内嵌 hard 路由使用插件路径。下面是需放入自己完整配置的绑定片段，不是可替换全部配置的文件：

```json
{
  "routes": {
    "hard": {
      "harness": "codex-pro",
      "model": "gpt-6.2-astra",
      "channel": "chatgpt-subscription",
      "thinking": "high"
    }
  }
}
```

完整 profiles 模板的 hard 路由引用 `profiles.astra`，其他 Codex 职责使用 `sol`／
`sol-plan`。插件精确消费
`dsh-subagent-codex-pro@0.10.2` 的公开 `dsh-subagent-codex-pro/machine` API。
精确依赖由插件 package 和 lockfile 锁定；生命周期开发时使用的源码包见
[vendor 来源记录](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md)。内部仍使用 Codex app-server 和用户已有登录，不需要父模型二次委派；缺包、
版本或回执不符会失败，不自动回退 `codex exec`。Claude 另为精确 `0.7.0` optional peer，
使用默认 Claude 阶段时需要安装，没有选用该后端时则不需要。接口检查不证明 npm
registry 已发布，也不代表真实账号验收已经通过。

已有配置不自动改写。若只迁移一项旧绑定，在 `config inspect` 显示的有效文件中修改
inline harness 或其 profile，保留所选 model／thinking／channel，然后重启引擎。
未启用新订阅策略的兼容配置仍可显式保留 `harness:"codex"` 旧 CLI；完整模型分工及
可达图验证使用前述 `--preset subscription-cli`。`codex-pro` 必须提供 model 和
channel，不沿用旧 driver 隐式模型；`resident-session:true` 在加载时拒绝。
thinking 省略时为 high。

主任务在 fresh 与 scoped 两种模式下均使用外层 worker 协议 3，并保留 persistent 原生历史。
fresh 每次不传旧 thread ID，创建独立的具名会话；scoped 仅在确认可续用时提供精确 ID。
原生持久化不单独授权复用；派发前需完成对应生命周期检查点
与 Go 持久化 ACK。公开 machine 请求／终态回执仍为协议 1，typed lifecycle 另有
版本化契约，不能混为一个版本。旧外层 v1/v2 与显式 ephemeral 继续服务低层兼容调用；没有自动获得
主任务的持久化恢复保证。任何 resume 必须匹配原 thread，busy 或失败文本不能证明
安全冷启动。原生客户端可见性、具体事件与只读恢复条件见[会话恢复指南](session-recovery.md#在原生客户端中找到会话)。

`FLYWHEEL_CODEX_PRO_WORKER` 可显式指定 worker；通常由 npm 启动器解析同版本依赖。`FLYWHEEL_CODEX_BIN` 指定插件内部的 Codex 程序，默认 PATH 中的 `codex`；`CODEX_HOME` 可选择既有配置与登录目录。自定义环境需对启动引擎的进程可见，安装器不会复制凭证。

Codex 主任务先通过公开 `thread-ready` 核对实际模型、cwd、approval 和完整沙箱策略，
Go 保存检查点并 ACK 后才允许新 turn。role-based 的 workspace-write 允许工作区与
Codex 标准临时目录；额外 writable roots、网络启用或无法确认的策略在派发前拒绝。
full-access 核对 `danger-full-access`／`dangerFullAccess`，不将其称为受限沙箱。
终态仍重新验证配置和清理事实。低层 v1/v2 兼容调用没有 Go 的派发前 ACK；外部管理
策略与原生拒绝在所有模式下仍然生效。

指标的 `codex-pro-receipt` 保存完整 machine receipt，包含请求、实际配置、thread/turn、进程、清理、usage 和失败事实。effort 的 `accepted: true` 不等于服务器证明了实际推理强度，`effectiveEffort` 保留 null。仅 `usage.status: "complete"` 的本次增量进入已知预算；partial/unknown 原样留在回执并计入未知调用。插件不提供实际美元成本，原始 cost 保持 unknown；Flywheel 只有在用户显式配置 pricing 且 usage 完整时才能另行估算，估算不等于订阅账单。

### Worker 环境

| 环境变量 | 用途 |
|---|---|
| `FLYWHEEL_DSH_WORKER` | 可选 DSH worker 路径；npm 启动器通常解析自身包内的 worker |
| `FLYWHEEL_DSH_BIN` | DSH 命令，默认 PATH 中的 `dsh` |
| `FLYWHEEL_DSH_PROFILE` | SDK profile，默认 `sdk`，不能为 `desktop` |
| `DSH_HOME` | DSH 自身解释的配置根目录 |
| `FLYWHEEL_KIMI_BIN` | Kimi 命令，默认 PATH 中的 `kimi` |
| `FLYWHEEL_CODEX_PRO_WORKER` | 可选 Codex Pro worker 路径，通常由 npm 解析 |
| `FLYWHEEL_CODEX_BIN` | 插件使用的 Codex 程序，默认 PATH 中的 `codex` |
| `CODEX_HOME` | Codex 自身解释的已有配置／登录目录 |
| `FLYWHEEL_CLAUDE_CODE_PRO_WORKER` | 可选的配套 Claude Code Pro worker 路径 |
| `FLYWHEEL_CLAUDE_BIN` | 原生 Claude 程序，默认 PATH 中的 `claude` |
| `CLAUDE_CONFIG_DIR` | Claude 自身解释的已有配置／登录目录 |
| `FLYWHEEL_ANTIGRAVITY_WORKER` | 可选的配套 Antigravity machine worker 路径 |
| `FLYWHEEL_AGY_BIN` | machine worker 的官方 CLI，默认 PATH 中的 `agy`；个人 ACP 使用 DSH `config.antigravity.command` |

这些设置须对启动引擎的进程可见，安装器不复制凭证。GUI 启动器记录安装时的工具
路径；终端中的临时 export 不会改变已运行的桌面进程。

### 权限与清理边界

Claude 在任务前核对 SDK initialize／版本；订阅策略下还核对同一进程的账号／usage
证据。但原生 `system/init` 配置消息发生在 prompt 之后，不提供 Codex 同等的预派发
`thread-ready` 权限保证。Claude 要求显式完全权限；Kimi 原生 Agent 同样不提供
OS 沙箱保证。Antigravity 普通执行要求完全权限，独立的证据分析路径则核验生成的
agent 身份和工具目录，并拒绝工具/子 Agent step。须分别判断实际后端能力与生命周期证据。

worker 异常退出不能证明插件的独立管理进程已停止。启动后无法确认清理时，引擎记录
证据并结束运行，不自动重试或换渠道。恢复前核对本任务的 app-server／guardian 进程，
再由 Go 核对原调用结果，见[中断恢复](session-recovery.md)。

## 可选 Claude Code Pro

默认 `opus-plan`、`opus-ui` profiles 通过 `claude-code-pro` 负责作用域 Plan 和
frontend／multimodal 执行。`fable` 供显式替换，canonical model 是
`claude-fable-5-2`。插件使用官方 Agent SDK 0.3.326 调用所选本机 Claude CLI，
沿用原生登录。订阅策略要求同一进程的 usage 查询使用 `skipBehaviors:true`，并在
释放任务 prompt 前明确确认额外用量已关闭；不支持、超时或未知证据都阻止派发，
不转 API。精确版本要求、别名、effort 冲突、optional peer 安装、完全权限及恢复
见 [Claude Code Pro 指南](claude-code-pro.md)。累计 `modelUsage` 是整轮原生运行／
历史的观测，不单独证明 root 模型回退、本 turn token 或订阅账单费用。

## Antigravity 与 Gemini

0.39.0 源码候选内置 `@ckanner/dsh-subagent-antigravity@0.3.0`，桌面独立的
**Antigravity** 视图默认使用个人交互 ACP。以下是 **DSH Flywheel 插件配置片段**，
不是 `flywheel.config.json` 片段：

```json
{
  "antigravity": {
    "transport": "acp",
    "command": "agy",
    "disposeGraceMs": 2000
  }
}
```

省略 `antigravity` 等于 `{}`，使用上述默认值。`command` 可以是已安装官方程序的
名称或绝对路径。客户端要求记录的官方 CLI 1.5.14 版本及摘要；包装程序、字节改变或
未支持的升级都会明确失败。`FLYWHEEL_AGY_BIN` 不配置这个入口。客户端不会回退到
Registry agent、headless、API 凭证或另一个账号。

需要认证时，视图可以启动已核验 CLI 自己的 `agy acp --login`，由用户在认证视图
完成流程；凭据和授权码不复制到 Flywheel 配置或 slash 命令。模型、模式与权限选项
来自原生会话，此个人入口仅准入它报告的 Gemini 模型。客户端保留原生用户规则，
不替换或追加系统提示词，不注入受管自定义 agent、Preset／PTC schema 或 DSH 工具
bridge。引擎的 `profiles`、`native-profiles`、model／effort 和自动权限设置不会转换
到此会话。

每个新 ACP 会话根据来源仓库已提交的 HEAD 创建独立 Git worktree，不复制来源中的
未提交修改，断开后保留目录与分支。桌面 Host 拒绝指向 ACP 所属工作区的 machine
变更和配置保存，物理别名也在检查范围；这不锁定其他编辑器或直接 Go CLI 调用。
连接、原生权限、图片与恢复见[交互视图](flow-console.md#交互式-antigravity-视图)。
[当前配套包记录](../../plugins/dsh/vendor/antigravity-acp.json) 分开标明源码／产物、
发行和真实登录证据。

正式 `antigravity` harness 显式导入配套包的 `./machine` API，请求／回执及 lifecycle
契约仍为 version 1。Plan、Execute、Review、Intake 和 JEV 继续使用官方 `agy`
stream-json；ACP 响应不会成为成功的 machine 回执。原生目录仍须包含选定的精确
模型，`thinking` 必须与 slug 一致；machine adapter 继续验证其订阅／credits 控制、
原生生命周期、请求配置和清理，访问或证据不足时明确失败。桌面默认值不改变现有
machine 绑定。仅 machine 使用的 profile、证据分析和本机计费证据的限制见
[Antigravity 指南](antigravity.md)。

显式 `transport:"headless"` 会禁用 Flywheel 的个人 ACP 入口。独立配套插件在该设置
下保留旧任务面板和工具；此设置不改变 Flywheel 显式 machine worker 的传输方式。

## 后备渠道与预算

默认 profiles 没有渠道后备。`channels[channel].fallback` 是人工配置的有界后备链，
只有既有错误分类／重试策略允许时才使用；每个可达后备也必须通过 subscription-only
检查。切换使用目标 channel 的显式角色／路由模型及 harness，再重新解析 provider；
缺少模型映射就停止，不猜测替代。后备省略 harness 时继承当前 harness。

普通渠道后备不会替另一后端翻译已有 thinking 或 native-profile。跨 harness 的
effort、能力或权限不兼容时，由准入拒绝。需要不同 effort／原生配置时，使用完整
显式的 `execution-policy` 绑定。任何后备都不能绕过未知派发或未确认清理。

指标分别保存 `harness`、`provider`、`channel`、`model`。provider 来自 driver 可确认的结果，未知时为 null；channel 继续描述配置渠道。usage 和美元成本未报告时保持 unknown，订阅不会被当成已知零成本。profiles 不改变计费规则，也不会绕过 hard budget。

默认 token 预算为 soft，每日上限 2,000,000。soft 报告不确定用量，hard 在未知用量使
预算无法核实时停止。关闭预算强制并不关闭计量，也不把 unknown 变成零。

Codex Pro 只有本次调用完整用量进入已知 token 计量，partial／unknown 保留在回执中。
插件不提供实际美元账单；显式 pricing 与完整 usage 可产生估算，但不等于订阅费用。

验收命令使用受限环境。项目需要 `GOCACHE`、`GOPATH` 等额外工具变量时，把变量名加入
`verify.env-allowlist` 并确保引擎环境已有该变量；不要将凭证值写入契约或提交的配置。

## 无推理诊断

```sh
flywheel doctor --cwd /repo --json
flywheel doctor --cwd /repo --task-dir /repo/docs/task --json
flywheel doctor --cwd /repo --task-dir /repo/docs/task --probe --json
```

没有任务时检查全部配置绑定。有任务时校验 Spec/packet，并使用与引擎相同的策略解析器：
新任务按 Spec 判断，已有任务按持久化选择，旧状态保留 Full。Full 要求 Plan、Review、
Judge、已启用的第二意见和实际 packet 路由；合格 Quick 只要求 Review 与实际执行路由，
Plan／Judge／第二意见显示 skipped。工作流状态损坏会阻止 SDK 探测，不重分类或改状态。
任务尚未生成 packet 时检查默认执行路由并提示规划后复查。mock driver 跳过真实工具检查，不冒充模型已验证。

doctor 不接收运行时的 `--mode` 覆盖，只反映 Spec／已保存状态；启动时另传模式后，
仍以 Go 实际选定并保存的策略为准。配置查看列出所有绑定，不表示每条绑定都必然被调用。

预检可提前运行，正式 `run`／`control launch|start` 仍需至少一个已准备、范围获授权的合法 packet；没有 packet 的 warn 不表示引擎已经能从空任务开始。绑定中的 permissionMode 表示阶段要求，实际实施能力还取决于 harness，尤其 Kimi CLI 不能据此宣称拥有 OS 沙箱。

本地检查确认命令可用、版本响应可靠，以及声明的环境变量存在且非空，不显示变量值。`--probe` 才启动 DSH worker 验证 adapter 接受配置、cwd、完整沙箱、approval policy 和 SDK 正常关闭。相同 provider、channel、model、effort、cwd 和权限的探测去重；不同账号别名或读写权限不能相互代替。整体验证预算 40 秒，另留进程清理窗口。

DSH 的 `ready.reasoningEffort` 是 SDK 接受配置之后的 worker 回显，不是模型服务独立证明的实际推理强度。没有模型请求的检查不能证明更高档位提高质量；Gemini、Claude、GPT、Kimi 也不共用同一套原生 effort 契约，详见[推理等级说明](model-policy.md)。

原生 worker 检查实际导入公开 API，并核对精确 companion 契约：Codex Pro 0.10.2、
Claude Code Pro 0.7.0、Antigravity 0.3.0，及要求的 lifecycle／recovery 导出、
Node 兼容性和原生命令。
绑定的 `durableJournal`、`readOnlyRecovery`、`threadReadyBeforeDispatch` 表示锁定
适配器的静态能力；Claude 最后一项为 false。`historyRecoveryTested` 保持 false。
`--probe` 不发送模型任务，不把导入成功解释为账号、权限或某段历史恢复已经验证。
调用前仍须实际订阅准入，未观测的额度与计费状态继续保持 unknown。

`sessions.policy` 检查只确认配置模式及版本；每个适用真实绑定的 `.session` 检查
显示所选模式、原因和适配器支持，历史恢复仍为 unknown。Doctor 不尝试恢复用户
历史，不发送测试 turn，也不把 worker 包导入成功当作持久化确认。

| 状态 | 解释 |
|---|---|
| `pass` | 本项已取得对应证据；只对该项有效 |
| `fail` | 存在阻断项，整体 `ok:false` 且退出码 1 |
| `warn` | 限制或规划前状态，需要结合实际任务理解 |
| `unknown` | 未验证，例如账号授权、额度与真实模型服务能力 |
| `skipped` | 非必需绑定、mock、重复探测或前置检查失败等 |

任何 probe 都不发送 `session/prompt`、不运行 L1/L2/L3、不写 Flywheel 状态或创建工作树；SDK 初始化可能访问 provider 目录并使用临时探测会话存储及自己的缓存文件。只读指 Flywheel 配置与任务状态，不表示整个宿主文件系统绝对无写入。

桌面面板、`/flywheel` 和 `flywheel_control` 共用 `control config|doctor`，可省略 taskDir；只有 doctor 支持 `probe:true`。诊断阻断与传输失败分开显示。桌面使用的 PATH／profile／环境可能与终端不同，安装后应在面板里再检查一次。

## JEV 决策

`jev.enabled` 是唯一 JEV 启用设置，默认 false。桌面编辑器只提供一个总开关；
开启后，调查准备自动冻结采用策略，包含动作选择、依据证据跳过冗余取证和提前
进入报告；确认结束的实施失败还可选择有界返修、已批准反馈检查、重规划或允许
执行绑定。Task／New／Run 共用引擎拥有的同一策略。关闭后停止后续请求及采用，
保留已有回执与预算。

完整默认配置为：

```json
{"jev":{"enabled":false,"backend":"subscription","profile":"hydrogen-local"}}
```

开启后使用 Antigravity 原生订阅路径调用 Gemini Hydrogen／medium，在新建且
已核验的空工具 agent 中选择动作。JEV 只接收有界的已批准证据，并从既有枚举里
选择，不能增加命令、URL、工具或模型绑定。缺省每次最多 30 秒、累计 8 次调用，
applicability 下限 0.8、choice confidence 下限 0.6。完整解析的 profile 冻结进
决策目标，之后修改 profile 会使旧决策失效。派发前持久化调用意图和生命周期检查点；
清理未知时保留 pending 调用并阻断后续模型工作，直到只读 recovery 取得清理证据。
恢复不会重发 prompt，也不会采用迟到答案。

正常任务没有独立影子／建议开关。旧 `jev-shadow` 只用于把连接信息迁移到关闭的
统一配置，编辑器保存时移除旧 key；明确的新 `jev` 优先。配置仍按任务／项目／
全局／内嵌表选择一份完整文件，不深合并。旧文件省略 backend 时保留原 HTTP 契约；
未启用 subscription-only 时仍可显式使用 `backend:"http"`。subscription 后端
不接受 HTTP 专用 endpoint／model／key 字段。两类契约、数据授权、跨调查与执行
共用预算、弃权及恢复规则见 [JEV 指南](jev.md)。`loops.investigate` 的调查分析
是独立阶段，不需要开启 JEV。

Packet 可选声明最多八项 `feedback-checks`，使用与验收命令相同的 `cmd`／`script`
和 `expect` 结构。这些是已批准的短诊断候选，不替代 `acceptance.L1/L2/L3`。
为每项填写简短 `purpose`（最多 160 字符），说明它能区分什么原因；JEV 接收这段
已批准的用途说明，不接收原命令。缺少 `purpose` 的检查仍供执行者参考，但不进入
JEV 的定向检查候选。用途不得包含凭据、原始日志或命令正文。JEV 也可选择已有
失败且可核对的 L1 检查。
执行沿用验证超时、环境白名单和进程收尾规则。检查变化属于契约身份变化，旧决策失效。

## 参考文件

- [完整默认配置](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.config.example.json)
- [完整 profiles 示例](https://github.com/ckanner/flywheel/blob/main/templates/flywheel.profiles.example.json)
- [系统设计](../design.md)
- [Worker 协议](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md)
