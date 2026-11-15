# 使用原生 Agent、Team、Swarm 与 LCU

[文档](../README.zh-CN.md) · 简体中文 · [English](../en/native-capabilities.md) · [完整设计](../plans/2026-11-13-native-harness-capabilities.md)

Flywheel 可以在正式 loop 中调用各 harness 的原生协作能力。先定义 `native-profiles`，再把某个 profile 绑定到需要它的阶段。静态配置无需开启 JEV；模型、账号 channel、权限和角色历史仍由原有绑定决定。

本页描述该版本的实现。配套包身份和已核对的发行信息见 [vendor 记录](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md)，本机安装应单独核对；[0.34.0 发行记录](../validation/2026-11-14-release-0.34.0.md) 标明正式产物身份和已完成的检查。

## 1. 选择实际后端

| harness | loop 内能力 | 依赖与权限 |
| --- | --- | --- |
| `dsh` | PTC、子 Agent、Team、Workflow、显式 Web、LCU | 本机 DSH；实际 PTC probe；LCU 另需 0.5.0 peer 和 full-access |
| `kimi` | Server Agent / Swarm / Tower | 已登录 Kimi；当前 Server API；显式 full-access |
| `codex-pro` | 原生 agents 与子线程 | 内置 Codex Pro 0.10.0；已登录 Codex CLI；实际 sandbox/approval 回执 |
| `claude-code-pro` | Agent 定义、Workflow、Advisor | 可选 Claude Code Pro 0.5.1；已登录 Claude CLI；显式 full-access |

在 DSH 中选择 ChatGPT 模型仍然使用 DSH 的工具循环，不自动获得 Codex 的 app-server 能力。桌面的 `heavy` Agent Preset 也不自动成为独立 SDK worker 的组合；worker 使用与生成器共享策略的 PTC overlay，并验证实际运行结果。

### 对齐独立 worker 的依赖

桌面、全局 CLI worker 和选中的 SDK profile 是不同的 Node 解析树。只更新桌面不会更新其他位置。先从受管安装记录（`~/.config/flywheel/installation.json`）确认 Flywheel 的实际 prefix；需要可选 Claude 或 LCU 后端时，将精确 peer 安装到同一 prefix。以下 registry 命令须等 Claude 0.5.1 正式发行；此前仅验证[已审查的源码候选](claude-code-pro.md#准备依赖)：

```sh
npm install --global --prefix /absolute/flywheel-node-prefix --ignore-scripts dsh-subagent-claude-code-pro@0.5.1 dsh-plugin-lcu@0.5.0
dsh plugin --profile sdk add --save-exact --ignore-scripts dsh-plugin-chatgpt@0.1.20
```

自定义了 `FLYWHEEL_DSH_PROFILE` 时，将 `sdk` 换成实际名称，并继续与 `desktop` 分离。这些命令只对齐软件依赖，不更改模型凭据、权限或原生 profile 开关。之后运行 `flywheel doctor --json`；可导入不等于已验证真实原生准入或模型视觉。

### DSH 桌面插件的默认配置与 PTC 调用

Codex Pro 和 Claude Code Pro 自身提供原生能力配置页。打开插件管理器中对应插件的配置，设置 `native` 默认值；DSH 的 PTC 工具 `subagent_codex` 和 `subagent_claude_code` 会在每次调用开始时读取这些设置。已经启动的调用保留自己的配置快照，后续保存不会改变它的执行参数。

| 普通工具的 `native` 参数 | 实际含义 |
| --- | --- |
| 省略 | 读取插件当前默认值；插件也未配置时，沿用 CLI 原有行为 |
| 对象 | 以这个完整对象替换该次调用的 native 默认值；不与默认值逐字段合并 |
| `null` | 明确关闭该次普通调用的原生协作能力，不退回 CLI 的隐式启用状态 |

Codex 的设置包括 Agent 启用状态、并发子线程数、子 Agent 默认模型和 effort，以及可选 V2 backend。Claude 的设置包括可单独指定模型与 effort 的 Agent 定义、并发与深度、Workflow 和 Advisor；启用 Advisor 时必须指定其模型。工具也支持相应的逐次覆盖。原生能力设置遵守现有权限和模型选择策略。可用档位及其与父阶段 effort 的区别见[具名 Claude Agent effort](claude-code-pro.md#为具名原生-agent-设置-effort)。

这里的 `native` 指 Codex/Claude 内部的能力，DSH 仍然可以用 PTC 编排这些工具。独立原生 Review 和真正的交互 Team 使用专门入口；保存普通 Agent 默认值不会启动这些任务。

这些桌面默认值只属于桌面插件。Flywheel 的直接 `codex-pro` / `claude-code-pro` worker 使用所选 `native-profile` 的确定快照，不读取另一份 DSH profile 的动态设置；独立 `/machine` 入口也不会为此加载桌面 UI 或 DSH 服务。正常重生成 daily/heavy preset 会保留单独保存的插件配置。

## 2. 定义与绑定

先运行 `flywheel config inspect --cwd /absolute/repo --json` 找到真正命中的完整配置。Flywheel 按任务→项目→全局→默认选择一个完整文件，文件之间不做字段合并。

把下面**片段**合入完整文件；需要的定义可单独选用：

```json
{
  "native-profiles": {
    "heavy-team": {
      "harness": "dsh",
      "options": {
        "version": 1, "preset": "heavy",
        "agents": {"enabled": true, "maxConcurrent": 4, "maxDepth": 2},
        "team": {"enabled": true},
        "workflows": {"enabled": true},
        "web": {"search": true, "fetch": true}
      }
    },
    "swarm": {
      "harness": "kimi",
      "options": {"version": 1, "mode": "swarm", "agents": {"enabled": true}}
    },
    "tower": {
      "harness": "kimi",
      "options": {"version": 1, "mode": "tower", "agents": {"enabled": true}}
    },
    "codex-team": {
      "harness": "codex-pro",
      "options": {"version": 1, "agents": {"enabled": true, "maxConcurrentThreads": 4, "backend": "v2"}}
    },
    "claude-flow": {
      "harness": "claude-code-pro",
      "options": {"version": 1, "agents": {"enabled": true}, "workflows": {"enabled": true}, "advisor": {"enabled": true, "model": "opus"}}
    }
  }
}
```

在当前阶段绑定上加入 `native-profile`，保留它原来的其他字段。例如默认 Execute 仍然是 DeepSeek account + DSH：

```json
{
  "routes": {
    "default": {
      "harness": "dsh", "provider": "deepseek-account",
      "channel": "deepseek-account-sdk", "model": "deepseek-flash", "thinking": "high",
      "native-profile": "heavy-team"
    }
  }
}
```

模型 `profiles`、`loops`、`routes`、`judge-panel` 和 `execution-policy.bindings` 都可引用。引用必须与实际 harness 匹配。绑定省略字段可继承模型 profile；显式 `"native-profile":""` 关闭继承。`native-profiles` 的 options 改变后，原有不兼容角色历史不会继续复用。

桌面工作台的“模型与会话”增加“原生能力”选择器，只列当前 harness 的配置；在高级 JSON 中编辑定义。保存仍需要当前文件 revision，配置从下次启动生效。选择 profile 只是配置意图，实际准入仍以运行回执为准。

## 3. 用在不同阶段

Plan 可以让 Team/Swarm 并行调查，root 最后输出正式 Spec 和 packets。Execute 可以使用原生 Agent、Workflow 或 Tower，结果仍要通过原有验证和独立 Review。Review 和 Judge 使用自己的角色历史与子任务，永远不继承 Executor 的历史。Intake/Garden 可以使用同样能力生成建议，不能据此修改状态文件。

开启 native profile 不新增第二个循环控制器：Go 继续决定是否重试、预算是否允许、哪个 candidate 被验收以及何时交付。原生 task board 的完成状态不是 Flywheel completed。

## 4. LCU 与截图

给 DSH profile 增加以下 options 字段，并为对应角色明确选择 full-access：

```json
{
  "lcu": {
    "enabled": true,
    "config": {"chrome": true, "audio": false}
  }
}
```

`config` 还支持实际 runtime 的 `app`、`command`、`allowedApps`、`allowedOrigins`。应使用本机真实 app/runtime 和已授权目标。LCU peer 必须能从 SDK worker 的实际 Node 依赖树解析；只在另一份桌面 profile 中安装不等于独立 worker 已可导入。

worker 先取得整台桌面的受管 lease；root 和后代的 GUI 操作顺序执行。PTC 工具名为 `lcu_cua`，程序对象仍为 `cua`。对象 handle 只属于当前 runtime epoch，重启后需重新取得。未确认清理的 lease 不被其他调用自动接管；先核对原 owner 的终态与实际 runtime，不能仅删除锁来假装完成。

图片和 manifest 保存在 `<taskDir>/reports/native/<invocationId>/`。归档 hash 对应实际 DSH AttachmentStore 的最终字节，capture hash 单独保留。图片存在不等于模型已经收到；`imageDeliveredToModel` 可以为 `unverified`。ChatGPT provider 0.1.20 修复真实图片到模型请求的投影，但尚未在用户账号上完成实机视觉验收。

公开 Pro machine API 可以显式接收媒体与结构化输出设置；正式 Go loop 不会自动把所有归档图片注入下一轮，也不会把外部 JSON schema 或 native review 文本当作自己的验收 verdict。

## 5. Kimi Tower 的候选工作目录

Tower 在独立 Git 副本中运行，包括当前 dirty/untracked/binary/symlink/mode/deletion 状态。它不是 OS 沙箱；当前权限为完全访问。worker 对提示词中的精确执行目录做有边界映射，并记录摘要。

仅 Execute 在完成与清理都确认后才尝试导回。导回前再次核对源 HEAD、文件内容与模式、外部 task 契约，并进行 binary patch check。源目录或 Spec/packet 有并发变化时保留 candidate 并返回冲突；其他角色只保留产物。导回仍要经过正常验证、Review 和 Git 交付。

native Kimi 不读取或写回旧 `plan-session` 缓存。Agent/Swarm 使用角色账本；
显式旧 `resident-session:true` 可保留续接意图，但 `sessions.roles.plan:"fresh"`
明确优先。Tower 始终 fresh，不续接旧 Session。Kimi Server 模式缺乏已证明的整树 `agents.maxConcurrent` 硬上限，因此不要为这些模式填写这个字段；明确请求时会在派发前被拒绝。完整协议见 [Kimi native](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/KIMI_NATIVE_PROTOCOL.md)。

## 6. 独立原生 Review 与交互 Team

这些操作有单独的原生任务回执，不直接进入正式 loop 的 verdict：

```sh
flywheel native codex-review --request /absolute/codex-review.json
flywheel native claude-review --request /absolute/claude-review.json
flywheel native claude-team --request /absolute/claude-team.json
flywheel native claude-recover --request /absolute/recovery.json
flywheel native claude-control --request /absolute/control.json
```

请求是对应公开包 API 加私有绝对 `recoveryDirectory`；输入必须是 regular JSON 文件，最多 1 MiB。Codex 的最小示例：

```json
{
  "protocol": 1, "requestId": "review-001", "cwd": "/absolute/repo",
  "model": "gpt-6.1-sol", "effort": "high",
  "prompt": "Review the selected changes.", "permissionMode": "read-only", "threadMode": "ephemeral",
  "native": {"version": 1, "review": {"target": {"type": "uncommittedChanges"}}},
  "recoveryDirectory": "/absolute/private/native-review-001"
}
```

远端 Claude Review 的请求需 `protocol:1`、唯一 requestId、绝对 cwd、`consent:true` 和 recoveryDirectory。它运行 `ultrareview --json --no-post`；本地超时不会证明远端 job 已结束。使用原 requestId、runId 和 recoveryDirectory 只读原本地 journal，取回已持久化的回执；不查询云端状态，也不重新提交 Review。

交互 Team 请求使用公开 `runInteractiveTeam` 的 prompt 和 permissionMode；tmux 必须可用。`claude-control` 支持 capture/send/resize/cancel/attach，使用 receipt 中的原 requestId/runId 和同一 recoveryDirectory。完整字段见 [Claude native API](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/d14ff8d8334fefd039c3d48ee2a969747d1e88c8/docs/native-capabilities.md)。

Claude `recoverNativeJob` 只读本地 durable journal，不启动进程或向云端查询。当前回执明确 `jobId:null`、`querySupported:false`、`cancelSupported:false`。本地恢复 status 为 terminal 也不等于 remote 完成：若远端 state 仍为 unknown 或 native.cleanup.confirmed 为 false，不能自动重放或声称云任务已取消。

## 7. 查看真实可用性与失败

```sh
flywheel doctor --cwd /absolute/repo --json
flywheel doctor --cwd /absolute/repo --probe --json
```

native doctor 检查本地 worker、精确 public manifest 与 capability 版本，不发模型、启动 Team、创建云 job 或操作 GUI。DSH native profile 不拿 legacy SDK probe 冒充 native 准入；其实际 PTC、模型与目标能力要在正式 worker admission 时证明。`native.admission` 保持 unknown，`inferenceTested:false` 和 `admissionTested:false` 不等于失败的登录尝试。

`native-receipt` 保存请求、实际配置、观察、children、usage、completion 与 cleanup。Pro 的 native-event 不是 durable lifecycle ACK。root 返回时仍有活跃子任务，不会因随后关进程而变成完成。清理不明、receipt 损坏或恢复不确定会继续阻断。

预算和并发也保持真实语义：Flywheel scheduler 限根调用；Claude 的 requested limits 是 observed guard，`hard:false`；Kimi 没有该整树硬上限。外部子 Agent、Advisor、compaction 或 Tower 消耗无法精确归属时 total 保留 unknown。hard budget 不把 unknown 当成零。详细测试与尚未实机验证的范围见[验证记录](../validation/2026-11-14-native-harness-implementation.md)。

桌面插件配置、PTC 参数类型和本轮真实包验证见[桌面配置跟进记录](../validation/2026-11-14-desktop-native-configuration.md)。
