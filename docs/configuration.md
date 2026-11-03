# 模型 profiles、阶段绑定与预检

Flywheel 在启动时加载一份完整配置，再把模型 profile 展开成每个阶段的实际绑定。模型选择是显式配置，不由另一个模型替你决定；修改配置后需要重新启动引擎。

## 1. 配置来源

发现顺序为任务目录、项目 Git 根目录、全局文件、内嵌默认表，选择第一个存在的完整文件。全局默认路径为 `~/.config/flywheel/flywheel.config.json`；`FLYWHEEL_CONFIG` 可覆盖全局文件路径，但不能越过已经存在的任务／项目配置。当前不通过 `XDG_CONFIG_HOME` 改变该默认路径。

高优先级文件不可读、JSON 无效或符号链接失效时直接报错，不悄悄使用其他账号或模型配置。不同文件的 profiles、loops、routes、channels 不逐字段合并。

```sh
flywheel config inspect --cwd /repo --task-dir /repo/docs/task --json
```

输出 `source`、`scope`、物理 `cwd/taskDir` 和按阶段排列的 `bindings`。`scope` 为 task／project／global／embedded。它只解析配置，不物化全局文件，不读取账号 token，不启动模型，也不创建任务状态。省略任务时只从项目及更低层发现配置，不会误读运行命令所在的另一个目录。

展示值包含 Flywheel 已确定的 driver 默认：DSH／Codex／Kimi 的空 thinking 为 high，pi／omp／zcode 为 max；Kimi／Codex／Zcode 的空 model 使用各自 driver 常量。pi／omp 未指定模型时仍留空，表示 Flywheel 不能从配置确定它。`residentSession` 只有 Plan＋Kimi＋明确启用时为 true，不把其他阶段忽略的请求显示为已生效。

## 2. 四个概念各负责什么

| 字段 | 含义 | 示例 |
|---|---|---|
| `harness` | 实际工具循环与进程后端 | `dsh`、`kimi`、`codex-pro`；`codex` 为显式旧 CLI 路径 |
| `provider` | DSH SDK 已注册的 LLM adapter ID；仅 DSH 使用 | `deepseek-official`、`chatgpt` |
| `channel` | 配置中的凭证／后备／计量渠道名称 | `deepseek-sdk`、`chatgpt-sdk`、`kimi-subscription` |
| `profile` | 当前 Flywheel 配置内的一组模型字段引用 | `flash`、`gpt-review`、`kimi` |

Flywheel 的 model profile 与 DSH 的 `sdk`／`desktop` profile 是两件事。前者复用模型定义，后者决定宿主实际加载的插件和授权配置。channel 别名本身不会切换 DSH 账号；给同一个 provider 起两个 channel 名称不等于拥有两份独立订阅或额度。

DSH 的实际 provider 按 `binding.provider` → `channels[channel].provider` → 旧 `channel` 名解析。前两者都明确配置而不相等时拒绝加载。兼容旧内联配置的最后一项只适用于 DSH；Codex Pro／旧 Codex CLI 和 Kimi 使用自己的登录状态，不接受一个会被静默忽略的 provider。

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

已有配置不自动改写。要从旧路径迁移，在 `config inspect` 显示的有效文件中修改 inline harness 或其 profile，保留所选 model／thinking／channel，然后重启引擎。保留 `harness: "codex"` 则明确继续使用旧 CLI。`codex-pro` 必须提供 model 和 channel，不沿用旧 Codex driver 的隐式模型；`resident-session: true` 会在加载时拒绝。执行／评审每次创建 ephemeral 新会话，thinking 省略时为 high。

`FLYWHEEL_CODEX_PRO_WORKER` 可显式指定 worker；通常由 npm 启动器解析同版本依赖。`FLYWHEEL_CODEX_BIN` 指定插件内部的 Codex 程序，默认 PATH 中的 `codex`；`CODEX_HOME` 可选择既有配置与登录目录。自定义环境需对启动引擎的进程可见，安装器不会复制凭证。

插件在运行前确认模型、cwd、approval 和沙箱模式；Flywheel 在完整结果返回后检查实际策略及进程终态。workspace-write 允许工作区及 Codex 标准临时目录；额外的工作区外 writable roots、网络启用或不能确认的策略不能进入成功验收。**该完整策略检查发生在运行后，不能阻止自定义 Codex 配置已允许的访问。** 要求完整策略在任务前被拒绝的环境，应等待上游公开 pre-turn guard；doctor 会持续提示这一边界。

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
      "provider": "deepseek-official",
      "channel": "deepseek-sdk",
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
    "deepseek-sdk": { "harness": "dsh", "provider": "deepseek-official", "env-key": null },
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
| `resident-session: false` | 明确关闭该阶段的恢复请求；不会丢失为缺省值 |
| `judge-panel: null` | 关闭第二意见，保持默认行为 |
| profile 定义缺 `harness/model/channel`，或这些字段为空／null／纯空白 | 加载失败 |
| profile 不存在、未知字段、错拼阶段名、不支持的 harness | 加载失败，不静默回落 |
| profile 再引用另一个 profile | 不支持继承链；加载失败 |
| `_note`、`_note_*` | 允许配置对象中的说明文字，不进入执行绑定 |

`resident-session` 是阶段策略，不能放进 profile。当前只有 Kimi 的 Plan 可恢复历史，且必须匹配实际 harness/model/channel/cwd/thinking；profile 名改了而实际值没变，不影响身份。其他执行、评审和 SDK 调用仍是新会话。

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

没有任务时检查全部配置绑定。有任务时校验 Spec/packet，仅把 Plan、Review、Judge、已启用的第二意见和实际 packet 路由列为必需；其他绑定显示 skipped。任务尚未生成 packet 时检查默认执行路由并提示规划后复查。mock driver 跳过真实工具检查，不冒充模型已验证。

预检可提前运行，正式 `run`／`control start` 仍需至少一个已准备并审阅的合法 packet；没有 packet 的 warn 不表示引擎已经能从空任务开始。绑定中的 permissionMode 表示阶段要求，实际实施能力还取决于 harness，尤其 Kimi CLI 不能据此宣称拥有 OS 沙箱。

本地检查确认命令可用、版本响应可靠，以及声明的环境变量存在且非空，不显示变量值。`--probe` 才启动 DSH worker 验证 adapter 接受配置、cwd、完整沙箱、approval policy 和 SDK 正常关闭。相同 provider、channel、model、effort、cwd 和权限的探测去重；不同账号别名或读写权限不能相互代替。整体验证预算 40 秒，另留进程清理窗口。

Codex Pro 本地检查额外实际导入公开 machine API，核对精确 0.8.0／协议 1、Node >=22.18.0 以及 Codex 命令版本；同一 worker 只查一次。`--probe` 不为此路径发送任务，实际模型、effort 与完整权限保持 unknown；它不会把成功导入插件当成账号、沙箱或推理能力已经验证。

| 状态 | 解释 |
|---|---|
| `pass` | 本项已取得对应证据；只对该项有效 |
| `fail` | 存在阻断项，整体 `ok:false` 且退出码 1 |
| `warn` | 限制或规划前状态，需要结合实际任务理解 |
| `unknown` | 未验证，例如账号授权、额度与真实模型服务能力 |
| `skipped` | 非必需绑定、mock、重复探测或前置检查失败等 |

任何 probe 都不发送 `session/prompt`、不运行 L1/L2/L3、不写 Flywheel 状态或创建工作树；SDK 初始化可能访问 provider 目录并产生自己的会话／缓存文件。只读指 Flywheel 配置与任务状态，不表示整个宿主文件系统绝对无写入。

桌面面板、`/flywheel` 和 `flywheel_control` 共用 `control config|doctor`，可省略 taskDir；只有 doctor 支持 `probe:true`。诊断阻断与传输失败分开显示。桌面使用的 PATH／profile／环境可能与终端不同，安装后应在面板里再检查一次。
