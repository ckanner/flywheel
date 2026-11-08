# 模型 profiles、阶段绑定与预检

[English](en/configuration.md) · 简体中文

Flywheel 在启动时加载一份完整配置，再把模型 profile 展开成每个阶段的实际绑定。模型选择是显式配置，不由另一个模型替你决定；修改配置后需要重新启动引擎。

## 1. 配置来源

发现顺序为任务目录、项目 Git 根目录、全局文件、内嵌默认表，选择第一个存在的完整文件。全局默认路径为 `~/.config/flywheel/flywheel.config.json`；`FLYWHEEL_CONFIG` 可覆盖全局文件路径，但不能越过已经存在的任务／项目配置。当前不通过 `XDG_CONFIG_HOME` 改变该默认路径。

高优先级文件不可读、JSON 无效或符号链接失效时直接报错，不悄悄使用其他账号或模型配置。不同文件的 profiles、loops、routes、channels 不逐字段合并。

```sh
flywheel config inspect --cwd /repo --task-dir /repo/docs/task --json
```

输出 `source`、`scope`、物理 `cwd/taskDir`、`sessions` 策略和按阶段排列的 `bindings`。每个绑定的 `session` 包含所选模式、适配器支持情况、原因以及恒为 false 的 `historyRecoveryTested`。`scope` 为 task／project／global／embedded。它只解析配置，不物化全局文件，不读取账号 token，不启动模型，也不创建任务状态。省略任务时只从项目及更低层发现配置，不会误读运行命令所在的另一个目录。

展示值包含 Flywheel 已确定的 driver 默认：DSH／Codex／Kimi 的空 thinking 为 high，pi／omp／zcode 为 max；Kimi／Codex／Zcode 的空 model 使用各自 driver 常量。pi／omp 未指定模型时仍留空，表示 Flywheel 不能从配置确定它。`residentSession` 只有 Plan＋Kimi＋明确启用时为 true，不把其他阶段忽略的请求显示为已生效。

### 将旧配置切换到已有的三种资源

当前源码的 `subscriptions` 预设只使用 ChatGPT 订阅、Kimi Code 订阅和 DSH 已登录并充值的 DeepSeek 账号（仅 Flash，`provider:deepseek-account`）。普通安装／升级仍保留已有配置；所以旧文件里的 Z.AI／GLM、Kimi K3、API provider 和旧 thinking 不会因升级 UI 自动消失。

**发行边界：**迁移命令与 `permissions.mode` 已在 0.28.2 发行，但该版内嵌预设仍选择 `deepseek-official` API Key。账号默认修复从 0.28.3 开始提供。安装前核对 registry 版本。先通过 `flywheel template flywheel.config.example.json` 核对所用二进制包含 `deepseek-account`，再用下面的迁移命令；在旧 0.28.2 上重跑迁移不会得到账号默认。

在实际项目目录运行以下命令；若任务目录有单独配置，再加 `--task-dir /repo/docs/my-task`：

```sh
flywheel config migrate --preset subscriptions --cwd /repo --json
flywheel config migrate --preset subscriptions --cwd /repo --apply --json
flywheel config inspect --cwd /repo --json
```

第一条只预览。`--apply` 明确应用到当前实际命中的任务／项目／全局文件，并先备份原始字节。没有文件时写全局配置，不创建意外的项目覆盖。可以用 `--expect-sha256` 绑定之前预览的摘要；文件已经变化时拒绝陈旧应用。预算、worktree、验证、重试、价格和用户说明等非模型设置保留，Plan 明确设置的 `resident-session:false` 以及现有 `sessions` 策略或其缺省状态也保留；模型迁移不会让旧文件自动启用 scoped。配置损坏、额外自定义路由／阶段或不支持的文件身份会明确阻断，不猜测替代。

迁移写入完整的阶段模型、thinking 和渠道，去掉被替换绑定的旧 profile 引用，清除所选渠道的旧 fallback／模型映射，使旧付费渠道不再自动可达；未引用的自定义 profiles／channels 仍保留。包括 0.28.2 留下的 API channel，也可以作为未引用配置保留，但不会成为账号路由的自动后备。默认关闭第二意见，并明确报告旧多模态路由被停用。默认分配仍是 Kimi K3.1 Pro high 负责 Plan／frontend／Garden，DeepSeek Flash high 负责普通执行、low 负责 Intake，GPT-6.1 Sol high 负责 hard／Review，GPT-6 Astra high 负责 Judge。hard 通过 Codex Pro 插件，Review／Judge 通过 DSH ChatGPT provider。Kimi 缺省模型为 `kimi-code/k3.1-pro`；显式写入的自选模型不会被仅修改 driver 缺省值覆盖。档位依据与尚未完成的对照评估见[推理等级与账号选择](model-policy.md)。

迁移不登录账号，也不发模型请求。当前运行中的任务继续使用启动时已加载的配置；新配置在下一次启动生效。检查迁移后的 source／scope／bindings，再按需要停止和恢复任务。

迁移期间不要同时编辑目标文件。迁移锁协调 Flywheel 迁移进程；写入前再次检查文件，再原子替换完整文件，不提供对任意外部编辑器的原子 CAS。报告以 `configurationStatus` 区分拟议配置 `proposed`、重读确认的实际配置 `effective` 和写入后尚未确认的 `unconfirmed`；备份／写入失败不能把新模型显示成已生效。

### 完全权限与无需逐步确认

```json
{"permissions":{"mode":"full-access"}}
```

新安装模板和显式 `subscriptions` 迁移都写入这一设置。全部阶段使用完全权限：DSH／Codex 请求 `danger-full-access` 与 `approval=never`，Codex Pro 通过其公开 machine API 映射，Kimi print 沿用原生 auto。它表示这些 worker 可以访问当前操作系统用户本来能够访问的文件和命令，并不从 DSH 对话框的权限下拉菜单继承。

旧文件没有 `permissions` 时保持 `role-based`：Plan／Execute 为项目内写入，Review／Judge 为只读。两种策略都不等待交互审批；只读／项目限制下的 `never` 可能直接拒绝动作，不能把它等同于完全权限。`permissions.mode` 的拼写、类型和枚举都会在启动前校验。

完全权限不会关闭 Flywheel 的任务边界、冻结验收、验证和候选身份检查。评审职责仍是评价已有候选，修改候选或验收条件不能取得有效通过。需要受限权限时显式使用 `role-based`；工作区隔离本身不是 OS 沙箱。

### 会话策略、历史复用与调用安全

```json
{"sessions":{"mode":"scoped","policy-version":1}}
```

新安装的两份配置模板显式选择 `scoped`。已有配置没有 `sessions` 时归一为
`fresh`／版本 1，升级不会改写原文件。显式声明时两个字段均必填；未知模式、未来版本、
null、错拼字段和不完整对象在模型启动前拒绝。字段信源为
[会话策略 schema](../templates/session-policy.schema.json)。

`sessions.mode` 控制历史复用，不关闭调用安全记录。Codex Pro、DSH 和 Kimi 的主任务
Plan／Execute／Review／Judge 调用在两种模式下都先记录调用意图，再保存可信终态回执。
`fresh` 不复用通用的角色历史，保持后端原有的新调用方式，并保留既有 Kimi Plan
`resident-session` 行为；`scoped` 在身份兼容且终态满足条件时延续同角色历史。
其他适配器及 mock 仍使用原有调用路径；Intake／Garden 不在本轮角色会话策略内。

| 角色 | 历史作用域与责任 |
|---|---|
| Plan | 同任务、同里程碑；只修订获准的任务契约 |
| Execute | 同任务、同 packet；普通返修可以延续执行者历史 |
| Review | 同任务、同 packet 和评审职责；与执行者隔离，复审仍检查完整当前候选 |
| Judge／第二意见 | 同任务、同里程碑，各自独立；不能共享主判和第二意见历史 |

Quick 最终 Review 与 Full 单元 Review 的完成要求不同，不能互用旧历史或裁决。
Quick 的正常首次成功路径仍只有 Execute 和最终 Review 两次阶段调用。Go 负责所有
会话选择、恢复、重建和计量；技能、模型与桌面控制面不扫描后端历史来猜测会话。

Plan 显式 `resident-session:false` 在 scoped 下仍关闭该角色的历史复用；省略该字段
不等于明确关闭。`resident-session:true` 是既有 Kimi Plan 选项，不是跨 harness 开关，
Codex Pro 仍拒绝它；为 Codex Pro 选择 scoped 应使用 `sessions`。

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
本轮只实现正常终态续接和已保存回执恢复（P1/P2）；实时恢复与只读核对（P3）仍待
上游公开协议，真实质量、缓存和性能实验（P4）尚未完成。

`status` 及 `control status` 展示角色、作用域、代次、动作／原因、最新调用状态、
派发／结果／清理、是否复用历史与用量状态。紧凑投影不读取回执内容或历史正文，也不
修改账本；无会话记录的旧任务不被推断为已具备恢复资格。

### 工作流策略与恢复

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
完整最终 Review 逐项核对需求，正常首次成功只调用 Execute 与最终 Review 两个阶段。
字段信源为 [Spec schema](../templates/spec.schema.json) 与
[state schema](../templates/state.schema.json)，运行流程见[工作流指南](workflow.md)。

## 2. 四个概念各负责什么

| 字段 | 含义 | 示例 |
|---|---|---|
| `harness` | 实际工具循环与进程后端 | `dsh`、`kimi`、`codex-pro`；`codex` 为显式旧 CLI 路径 |
| `provider` | DSH SDK 已注册的 LLM adapter ID；仅 DSH 使用 | `deepseek-account`、`chatgpt` |
| `channel` | 配置中的凭证／后备／计量渠道名称 | `deepseek-account-sdk`、`chatgpt-sdk`、`kimi-subscription` |
| `profile` | 当前 Flywheel 配置内的一组模型字段引用 | `flash`、`gpt-review`、`kimi` |

Flywheel 的 model profile 与 DSH 的 `sdk`／`desktop` profile 是两件事。前者复用模型定义，后者决定宿主实际加载的插件和授权配置。channel 别名本身不会切换 DSH 账号；给同一个 provider 起两个 channel 名称不等于拥有两份独立订阅或额度。

DSH 的实际 provider 按 `binding.provider` → `channels[channel].provider` → 旧 `channel` 名解析。前两者都明确配置而不相等时拒绝加载。兼容旧内联配置的最后一项只适用于 DSH；Codex Pro／旧 Codex CLI 和 Kimi 使用自己的登录状态，不接受一个会被静默忽略的 provider。

### DeepSeek 账号与 API Key 是两个 provider

| 方式 | Provider | 凭证来源 | 选择方式 |
|---|---|---|---|
| DSH 已登录、已充值的 DeepSeek 账号 | `deepseek-account` | DSH 的 `deepseekAccount` 服务 | 当前源码默认；不要求 `DEEPSEEK_API_KEY` |
| DeepSeek API Key | `deepseek-official` | DSH credentials／启动环境中的 API key，默认引用 `DEEPSEEK_API_KEY` | 仅在用户明确选择 API 路径时配置 |

标准 SDK profile 叠加的 [base](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/bundle/base/cordis.patch.yml) 包含账号服务和这两种 adapter。标准桌面与 SDK 使用同一 `DSH_HOME` 时，默认共享该 home 下的[账号授权存储](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/credentials/credentials-local/src/index.ts)；profile 隔离插件配置，不要求把 token 复制到 `sdk` 或改用 `desktop` profile。自定义 home、凭证存储或 SDK 插件集时，须核对它们指向同一授权来源。这里不读取或展示凭证值。

DSH 桌面在登录后且没有任何 provider API key 时，可以自动选择账号目录里的第一个模型。该桌面默认不决定独立 SDK 会话的 provider/model，且“第一个模型”不保证是 Flash。Flywheel 因此明确传入 `deepseek-account`／`deepseek-flash`；账号未登录、授权失效或额度不足时保留失败。默认预设没有 API Key 后备；用户显式配置的后备链仍按既有规则处理。已核对的[官方账号 adapter](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek-account/src/index.ts)、[API adapter](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/llm/llm-deepseek-api-key/src/index.ts)及[默认选择逻辑](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/api/session-controller/src/index.ts)是这一边界的依据。

需要继续用 API Key 的自定义配置仍受支持：在完整配置的相关绑定及其 channel 中一致选择 `deepseek-official`，按 DSH API provider 的正常方式配置凭证。不要仅改 channel 名却保留冲突的显式 provider；也不必为账号路径额外添加 API key。

### Codex Pro 默认与旧配置迁移

0.28.0 的内嵌 hard 路由使用插件路径。下面是需放入自己完整配置的绑定片段，不是可替换全部配置的文件：

```json
{
  "routes": {
    "hard": {
      "harness": "codex-pro",
      "model": "gpt-6.1-sol",
      "channel": "chatgpt-subscription",
      "thinking": "high"
    }
  }
}
```

完整 profiles 模板使用 `profiles.codex-pro`，hard 路由引用它。插件依赖 `dsh-subagent-codex-pro@0.8.0` 由 npm 安装；worker 直接导入其公开 `dsh-subagent-codex-pro/machine` API，内部仍使用 Codex app-server 和用户已有登录。它不是 DSH LLM provider，也不需要父模型调用 `subagent_codex_pro`。缺少插件或回执不符会失败，不自动回退 `codex exec`。

已有配置不自动改写。要从旧路径迁移，在 `config inspect` 显示的有效文件中修改 inline harness 或其 profile，保留所选 model／thinking／channel，然后重启引擎。保留 `harness: "codex"` 则明确继续使用旧 CLI。`codex-pro` 必须提供 model 和 channel，不沿用旧 Codex driver 的隐式模型；`resident-session: true` 会在加载时拒绝。thinking 省略时为 high。

fresh 使用外层 worker 协议 1 与 ephemeral 历史；scoped 使用外层协议 2、
`threadMode:persistent` 和可选的精确 `threadId`。两者内部仍消费插件 0.8.0 的
machine API／原始 receipt 协议 1，不能把外层 v2 解释成上游协议升级。恢复必须匹配
原 thread；busy、缺失历史或通用恢复失败不能靠错误正文猜测为安全冷启动。
能力边界见[会话生命周期方案](plans/2026-11-07-session-lifecycle.md)。

`FLYWHEEL_CODEX_PRO_WORKER` 可显式指定 worker；通常由 npm 启动器解析同版本依赖。`FLYWHEEL_CODEX_BIN` 指定插件内部的 Codex 程序，默认 PATH 中的 `codex`；`CODEX_HOME` 可选择既有配置与登录目录。自定义环境需对启动引擎的进程可见，安装器不会复制凭证。

插件在运行前确认模型、cwd、approval 和沙箱模式；Flywheel 在完整结果返回后检查实际策略及进程终态。在 role-based 的 workspace-write 模式，允许工作区及 Codex 标准临时目录；额外的工作区外 writable roots、网络启用或不能确认的策略不能进入成功验收。显式 full-access 则核对 `danger-full-access`／`dangerFullAccess` 完全权限回执，不将其误报为受限沙箱。**该完整策略检查发生在运行后，不能阻止自定义 Codex 配置已允许的访问。** 要求完整策略在任务前被拒绝的环境，应等待上游公开 pre-turn guard；doctor 会持续提示这一边界。

指标的 `codex-pro-receipt` 保存完整 machine receipt，包含请求、实际配置、thread/turn、进程、清理、usage 和失败事实。effort 的 `accepted: true` 不等于服务器证明了实际推理强度，`effectiveEffort` 保留 null。仅 `usage.status: "complete"` 的本次增量进入已知预算；partial/unknown 原样留在回执并计入未知调用。插件不提供实际美元成本，原始 cost 保持 unknown；Flywheel 只有在用户显式配置 pricing 且 usage 完整时才能另行估算，估算不等于订阅账单。

## 3. 从完整示例开始

查看默认内联表或可复用 profiles 表：

```sh
flywheel template flywheel.config.example.json
flywheel template flywheel.profiles.example.json
```

确认目标没有需要保留的配置后，选择其中一份导出到项目根。已有配置先备份并审阅差异，避免 shell 重定向覆盖它。下面只是解释结构的片段；完整可用起点在第二份模板中：

```json
{
  "profiles": {
    "flash": {
      "harness": "dsh",
      "provider": "deepseek-account",
      "channel": "deepseek-account-sdk",
      "model": "deepseek-flash",
      "thinking": "high"
    },
    "gpt": {
      "harness": "dsh",
      "provider": "chatgpt",
      "channel": "chatgpt-sdk",
      "model": "gpt-6.1-sol",
      "thinking": "high"
    }
  },
  "loops": {
    "review": { "profile": "gpt" },
    "judge": { "profile": "gpt", "model": "gpt-6-astra" },
    "intake": { "profile": "flash", "thinking": "low" }
  },
  "routes": {
    "default": { "profile": "flash" }
  },
  "channels": {
    "deepseek-account-sdk": { "harness": "dsh", "provider": "deepseek-account", "env-key": null },
    "chatgpt-sdk": { "harness": "dsh", "provider": "chatgpt", "env-key": null }
  }
}
```

这些模型名沿用本仓库的初始绑定，能否使用仍由实际账号和 adapter 决定。读取配置、看到模型名称、adapter 接受参数都不等于账号已获授权。DeepSeek 默认仅 Flash；profiles 不引入自动跨渠道付费回退。

### 覆盖与错误规则

| 情况 | 行为 |
|---|---|
| 阶段省略某字段 | 继承所引用 profile 的字段 |
| 阶段显式写 `model`／`channel`／`thinking` 等 | 只覆盖该阶段，不修改共享 profile 或其他阶段 |
| `thinking: ""` | 使用对应 harness 的现有默认 effort，不重新继承 profile |
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

## 4. 后备渠道与记录

`channels[channel].fallback` 仍是人工配置的有界后备链，只有既有错误分类／重试策略允许时才使用。切换后依据新渠道重新选择 provider 和模型；不会继续沿用原渠道的 provider。后备渠道省略 harness 时继承当前 harness，不回到链起点的后端。

指标分别保存 `harness`、`provider`、`channel`、`model`。provider 来自 driver 可确认的结果，未知时为 null；channel 继续描述配置渠道。usage 和美元成本未报告时保持 unknown，订阅不会被当成已知零成本。profiles 不改变计费规则，也不会绕过 hard budget。

## 5. 预检范围和结果

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

DSH 的 `ready.reasoningEffort` 是 SDK 接受配置之后的 worker 回显，不是模型服务独立证明的实际推理强度。没有模型请求的检查不能证明更高档位提高质量；GPT、Kimi 和 Flash 也不共用同一套档位枚举，详见[推理等级说明](model-policy.md)。

Codex Pro 本地检查额外实际导入公开 machine API，核对精确 0.8.0／协议 1、Node >=22.18.0 以及 Codex 命令版本；同一 worker 只查一次。`--probe` 不为此路径发送任务，实际模型、effort 与完整权限保持 unknown；它不会把成功导入插件当成账号、沙箱或推理能力已经验证。

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

任何 probe 都不发送 `session/prompt`、不运行 L1/L2/L3、不写 Flywheel 状态或创建工作树；SDK 初始化可能访问 provider 目录并产生自己的会话／缓存文件。只读指 Flywheel 配置与任务状态，不表示整个宿主文件系统绝对无写入。

桌面面板、`/flywheel` 和 `flywheel_control` 共用 `control config|doctor`，可省略 taskDir；只有 doctor 支持 `probe:true`。诊断阻断与传输失败分开显示。桌面使用的 PATH／profile／环境可能与终端不同，安装后应在面板里再检查一次。
