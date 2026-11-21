# DSH Antigravity 插件

[English](README.md) · [机器接口](docs/MACHINE.zh-CN.md) · [验收与发布](docs/VALIDATION.md)

通过本机**官方 `agy` CLI** 使用 Gemini，沿用用户已有的官方账号登录。Desktop 任务面板、`subagent_antigravity` 工具和公开 ESM machine API 共用同一套监督 runner。没有 ACP，也没有独立付费 API 回退。

当前是 **0.2.0 候选版**。代码和离线测试不代表已发布 npm、已在用户 Mac 安装或已完成真实账号推理。实际检查范围见验收文档。

## 安装前提

- macOS/Linux、Node.js 22.18+。本版使用 POSIX 进程组；Windows 明确拒绝执行。
- 用户自行安装官方 `agy` 1.5.7+ 并登录；Orchestrator 和显式子智能体/Swarm 要求 1.5.12+。插件不下载、更新 CLI，不复制凭据，也不实现登录代理。
- DSH 具有公开 commands、workingDirectory、会话视图/设置插槽和 Agent tools；后台委派还需要 jobs。已用发布版 `0.2.2-rc.3` 的工具 schema/PTC 契约检查，并在加载时核验所需能力。
- 原生账号认证；不启用 API/ADC/GCP/custom model 路由；官方 CLI 中 AI credits 自动回退关闭。

在仓库中构建并检查候选包：

```sh
npm ci --ignore-scripts
npm run check
```

`test:package` 输出真实 `.tgz` 及 integrity。先在临时 DSH profile 验收：

```sh
dsh plugin --profile <profile> add /absolute/path/dsh-subagent-antigravity-0.2.0.tgz
```

只有该版本正式发布后，才按 npm 包名安装。包没有安装脚本，也不在安装或打开设置时偷偷调用模型。

bundle 新增一个 `antigravity` 插件行。Agent 委派还需要 preset **显式允许 `subagent_antigravity`**；插件的 `presets` 是第二层范围，不替代 preset 工具注册。无需将它注册成传统 LLM provider，原生 CLI 已拥有完整 Agent 工具循环。

## 在 DSH 中使用

打开会话中的 **Antigravity** 视图，填写完整任务、名称和工作目录，选择模型、原生协作模式及权限后点击开始。普通模型可调整思考等级，Orchestrator 显示“固定（由模型决定）”。按钮通过公开 command 接口直接启动 CLI，**不会调用父模型**。切换视图不停止任务；刷新、查看结果、停止均在同一面板完成。关闭插件会取消它仍拥有的任务。

任务绑定 Desktop 会话、规范化工作区和 invocation ID。另一个会话或工作区不能查询/取消它。重启后能读取原有 receipt；缺少确证的调用保留 `unknown`，不会自动换会话或重发。先查看原调用，再判断下一步。

面板显示本地名称、原生 conversation ID、请求模型/effort/协作模式、真实观测、进度、结果、错误和清理状态。运行中点击“查看结果”可看到当前已观察的子成员 ID、角色及未知状态，不会改已有原生标题。“检查环境”只运行 `--version`、`--help`、`models`，不发送推理，不能证明套餐权益和余额。

插件行设置包含 command、默认目录、模型、effort、原生协作模式、执行模式、工具审批、sandbox、timeout 和允许 presets。每次启动冻结配置；保存只影响后续调用，单次明确选择优先。点击续跑会把原任务绑定带回表单；续跑时省略 native 会恢复该 owner 原任务的 native 选项（包括原本字段缺失），不会继承后来保存的协作默认值。改变绑定会被 runner 拒绝。

也可以直接输入：

```text
/antigravity {"version":1,"action":"start","label":"分析超时","prompt":"只分析以下证据……","model":"gemini-4-neon-high","effort":"high","toolPolicy":"evidence-only"}
/antigravity {"version":1,"action":"list"}
/antigravity {"version":1,"action":"result","invocationId":"<返回的精确 UUID>"}
/antigravity {"version":1,"action":"cancel","invocationId":"<返回的精确 UUID>"}
```

另外提供 `status`、`doctor`、`defaults`。手工任务保存在插件自己的 owner 绑定存储中，不伪造一个 Agent job。

## 工具、PTC 和后台任务

```js
subagent_antigravity({
  label: "修复超时回归",
  prompt: "复现并修复已批准范围内的超时问题，然后运行回归验证……",
  model: "gemini-4-hydrogen-high",
  effort: "high",
  run_in_background: true
})
```

后台返回 `{kind:"background", jobId}`，沿用 `job_output` / `job_kill`；前台返回文本内容中的任务结果、原生观察和证据身份。完整机器 receipt 单独保存，进度有界，不将无限事件流塞进父对话。工具不能扩大已配置的权限或改变 owner 工作目录。可选择模型/effort、`native` 模式、精确历史 sessionId、deadline、JSON schema，以及工具策略。

只在允许的 committed preset 中注册，执行时重新检查，包括 preset 切换。输入/输出 schema 经过真实 DSH 发布包的校验器与 PTC 类型生成检查。

## Orchestrator、原生子智能体与 Swarm

模型选择与协作模式是独立配置。[官方模型指南](https://antigravity.google/docs/models)将 Orchestrator 定义为同一智能体内部的规划/实施模型交接；[Swarm](https://antigravity.google/docs/swarm/)才显式请求多成员并行协作。只选 Orchestrator 不代表已经启动蜂群。

| 模型 | effort |
| --- | --- |
| `gemini-4-hydrogen-*`、`gemini-4-neon-*` | 与 slug 后缀匹配的 `low`、`medium`、`high` 或 `xhigh` |
| `gemini-4-carbon-*`、`gemini-4-argon-*` | 与 slug 后缀匹配的 `low`、`medium` 或 `high` |
| `gemini-pro-orchestrator-neon` | `""`，固定配置，不传 `--effort` |
| `gemini-pro-orchestrator`（Carbon） | `""`，固定配置，不传 `--effort` |
| `gemini-pro-orchestrator-argon` | `""`，固定配置，不传 `--effort` |

不写 `native` 就保留普通 CLI 行为。DSH 默认配置、直接任务、Agent/PTC 工具和 machine API 使用相同结构：

```js
subagent_antigravity({
  prompt: "按照已批准范围，分别实施 parser 和 UI 修改；汇总成员结果并完成集成检查。",
  model: "gemini-pro-orchestrator",
  effort: "",
  native: { version: 1, mode: "swarm", maxObservedAgents: 4 }
})
```

| `native.mode` | 请求行为与准入 |
| --- | --- |
| `native` | 保留原生自主行为，仍可能使用子智能体；禁用 slash 展开。 |
| `subagents` | 要求协调者委派并汇总子任务；发 prompt 前，真实 `init.tools` 必须包含 `invoke_subagent`；继续禁用 slash 展开。 |
| `swarm` | 仅由适配器合成 `/swarm` 前缀，用户任务作为 JSON 数据。原生 `disableSlashCommands:true` 会阻止调用，插件不会修改设置。 |

显式协作必须观察到子成员元数据；只有根 SUCCESS、没有子成员证据时，以 `collaboration-not-observed` 失败。receipt 区分请求的命令与已观察成员，不伪造 Swarm board、子模型或子成员终态。根据[官方 headless/changelog 契约](https://antigravity.google/docs/changelog?tab=cli)，非空 `finished_reason`（含 `print_timeout`）一律失败，即便同时给出 SUCCESS 和 exit 0。

`maxObservedAgents` 可选，整数 1–8，统计**全部已观察的唯一子成员 ID**，包含 verifier、monitor。超出后停止本调用；回执固定 `hard:false`。这不是原生并发硬上限、完整远端树上限或额度保证。整树用量、未报告的子模型/终态保留 unknown。`log_uri` / `workspace_uris` 仅作为元数据保存，不自动打开。

完成范围为**当前 CLI invocation**。取消只监督本调用拥有的进程组；恢复只读原 journal，不进行全账号子任务查询或取消。适合已能划分独立子任务的工作；外层流程要求独立正式 Review 时仍要另起 Reviewer，本轮原生 verifier 仅作为同一根调用内的证据。

## 机器接口和会话

`dsh-subagent-antigravity/machine` 导出协议/能力常量、`parseMachineRequest`、`runMachine`、`recoverMachine`、`doctor` 与 TypeScript 类型。完整示例和字段见 [机器契约](docs/MACHINE.zh-CN.md)。Flywheel 不需要创建 DSH 父 Agent。

每次调用使用一个 CLI 进程、一条文本 user message。请求必须给出 cwd、精确官方模型 slug、匹配的 effort（Orchestrator 为固定空字符串）、执行/审批模式及订阅策略。传入 `runtime.env` 时，它是完整环境，不与全局环境合并；提供同一官方 CLI 账号作用域的 HOME/PATH。敏感及替代付费路由变量会过滤。

新会话会保存原生 conversation ID，不承诺原生历史不持久化。只有 adapter 拥有、已终态且清理确认的会话才能续跑；cwd、模型、effort、权限、toolPolicy、native 选项或原生配置变化要求新绑定。同一会话跨进程并发续跑会被拒绝；未知调用不能借重新启动绕过。

`antigravity-machine --capabilities / --doctor / --json / --recover` 是本插件包装命令，**不是原生 `agy` 参数**。除 capabilities 外从 stdin 读取 JSON，stdout 只输出一份 JSON envelope。

## 只分析给定证据

`toolPolicy:"evidence-only"` 在工作区创建唯一、归本调用所有的 custom agent，设置 `tools: []`、`mcpServers: []`、`subagent:false`、`commandExecutionPolicy:off`，通过官方 `--agent` 选择。**发送 prompt 前**，必须从真实 init 看到同一个 agent 和空的 available tools。无 init、agent 缺失/不匹配、工具非空均拒绝；后续若出现工具/子 Agent 事件也失败并取消。

该策略拒绝 Orchestrator 以及 `subagents` / `swarm`，始终禁用 slash 展开和 spawn，包括供外层 JEV 做证据决策时。prompt 要求使用工具不能绕过已核验的空工具列表。

这样不假定所有版本都把 YAML 空列表解释为禁用工具，而以真实启动观测作准入。[官方 headless 文档](https://antigravity.google/docs/cli/headless/)明确 init.tools 是全部可用工具，指定 `--model`/`--agent` 时有相应字段；[custom agent 文档](https://antigravity.google/docs/subagents/)规定工具允许列表和工作区发现路径。

临时文件用独占创建、唯一名称；取消/父死亡也只在 inode 与内容仍匹配时删除本调用文件。不会覆盖用户 agent，也不改 global settings。这是原生工具能力约束，**不是 OS sandbox**；CLI 自身初始化、定制发现和操作系统权限不因此隔离。要求完整宿主隔离时应使用独立受管执行环境。单靠 `plan` 模式不能作为只读取证约束。

## 订阅策略与未知状态

准入证据是**本地计费路由控制**：拒绝 API provider、ADC、GCP、自定义 API model，过滤 API key/endpoint 环境，要求官方 `useG1Credits:false`（文档默认值），并检查派发前后配置漂移。认证或额度失败不切换到付费 API/credits。

依据：[官方认证](https://antigravity.google/docs/install/)、[CLI 参数与配置](https://antigravity.google/docs/cli/reference/)、[AI credits](https://antigravity.google/docs/cli/commands/credits)。官方把 useG1Credits 定义为套餐额度耗尽后改用 credits；插件不会启用或修改它。

通过后 receipt 写 `verified-local-controls`，不会冒充已核验商业权益。原生账号权益、套餐余额、第三方工具计费和实际额外账单没有可靠原生机器证据时仍为 unknown。模型目录不等于余额；token 统计不等于套餐剩余量。金额缺失写 `null`，不写成零元。

成功还必须有无非空 `finished_reason` 的正常原生 SUCCESS 终态、可观测身份/模型/目录一致、schema 有效及本地清理确认；显式协作还要求已观察成员。exit 0、WAITING/RUNNING、soft-denied、半截输出不能成功。effort、executionMode、sandbox 没有原生回显时保持 unknown。`--dangerously-skip-permissions` 不等于沙箱关闭或任意路径可写；`--sandbox` 也可能在不支持机器上无效。

本地进程组退出不证明远端或所有子工作已完成；无终态保留 unknown，恢复只读原 journal，绝不重放。观察上限不构成整树硬预算；不承诺原生图片/PDF 输入或已验证的截图产物回传。真实 Mac、账号、模型权益和完整 Desktop 验收见 [验收文档](docs/VALIDATION.md)。
