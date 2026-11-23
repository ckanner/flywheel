# 官方 ACP 默认交互入口：实现、复核与候选安装

[文档索引](../README.zh-CN.md) · [Validation index](README.md) · [交互方案](../plans/2026-11-22-antigravity-native-acp.md) · [English plan](../plans/2026-11-22-antigravity-native-acp.en.md)

日期采用 America/New_York 的 2026-11-22。官方发行物与未认证探测记录保留实际 2026-11-23 UTC 时间。Flywheel 0.39.0、独立 companion 0.3.0 均为本次未发布候选；不把既有 0.38.0／0.2.1 发布或旧 headless 账号记录当作 ACP 验收。

## 目标与实现范围

用户要求在现有方案和 PR #63 上完成实现，并将 ACP 设为默认方式。当前默认指 **DSH/Flywheel 的人类交互入口**：由未经修改的官方 CLI 执行 `agy acp`，客户端提供会话界面、连接、原生审批和进程生命周期管理。官方 agent 负责系统指令、原生项目规则、模型、工具与内部循环。

实现不发送宿主 system prompt 或 append，不添加隐藏控制前缀、DSH Preset/PTC、受管 agent 或宿主 MCP/tool bridge，不运行外层自动循环。客户端真实标识为 `dsh-antigravity`，没有伪装为 Zed。个人登录入口只在官方 session 暴露并确认 Gemini 模型时允许发送 prompt。

Flywheel 的 `harness:"antigravity"` 仍是显式 headless machine 接口，machine/lifecycle/activity 协议均保留版本 1；本次只同步 companion 的精确 0.3.0 身份，不更改既有自动化任务的模型、角色或订阅配置。ACP 会话没有被接入 Plan/Execute/Review、JEV 或后台任务自动委派，也不代表 Go task 已完成。旧独立插件任务面板和 `subagent_antigravity` 工具要求显式 `transport:"headless"`。这个兼容接口的用途与认证依据不由本次 ACP 实现背书。

## 源码与独立候选身份

| 项目 | 已核实身份 |
| --- | --- |
| Flywheel 集成 PR | [ckanner/flywheel #63](https://github.com/ckanner/flywheel/pull/63) |
| Companion PR | [ckanner/dsh-subagent-antigravity #1](https://github.com/ckanner/dsh-subagent-antigravity/pull/1) |
| Companion 源码提交 | `7d5056c81688cc9408f3be32fc38ac7388b97377` |
| Companion 源码 tree | `1ed2fdc5cae8ee4b507b2bf461ec68a7f70c97f3` |
| 独立候选包 | `@ckanner/dsh-subagent-antigravity@0.3.0` |
| Tarball | `ckanner-dsh-subagent-antigravity-0.3.0.tgz` |
| SHA-256 | `7bb3c7277658c1932c26c1552d5515e1ed8e13fcdd4c7380b4c30e4441328447` |
| SHA-512 integrity | `sha512-RmpcFTcrTUqZKWpqvMSz8y858ZQq+L8fXfpTi6QziQjeYKfonx1ZdtGBzUo50foJ8iAxYsAEWqODk5rengwz/Q==` |
| 协议和 SDK | 稳定 ACP v1；`@agentclientprotocol/sdk@2.9.1` |
| 实际工具链 | Linux x64，Node 24.22.3，npm 11.19.1，Go 1.27.4 |

完整 [vendor 记录](../../plugins/dsh/vendor/antigravity-acp.json) 同时固定源码、tree、tarball、摘要和未发布状态。根目录与插件 lockfile 使用相同实际 integrity；安装后也核对了共享 client 的实际字节。最终 tarball 的 40 个打包文件逐一与上面的源码提交比对一致。旧 [0.2.1 来源记录](../../plugins/dsh/vendor/antigravity.json) 和 [2026-11-22 registry 检查](../../plugins/dsh/vendor/registry-verification-2026-11-22.json) 原样保留。

## 人类入口与所有权

新建一个选择了 Git 项目工作目录的 DSH 会话，执行空 `/flywheel-antigravity`，再点击 **Antigravity** 标签。独立插件对应 `/antigravity`。命令本身只激活入口和显示指引，不启动 agent、登录或推理。

- ACP 控制通过公开 Typert Remote，使用真实 agent/session lookup 确定 owner 和物理工作目录。浏览器请求不能指定另一个 owner、cwd、env 或系统提示词。
- 文本、图片、登录响应和权限选择通过鉴权 Remote；slash command 只接受有限控制动作，并设置 `recordInput:false`。command 结果也不复制 transcript 或登录 URL。
- 新连接在源仓库已提交 HEAD 上建立独立 worktree，不复制未提交修改；运行中的 ACP worktree 拒绝 Flywheel 宿主可变 machine 操作及路径别名。关闭后保留工作树和分支供人类审阅，不自动合并、删除。这不是阻止用户手动运行任意外部命令的全局锁。
- SDK 使用真实权限 handler，返回用户选择的官方 option ID。没有宿主 fs/terminal 工具桥；MCP server 输入为空。可选能力按真实协商结果处理，未知事实保留 unknown。
- 登录运行同一核验程序的 `acp --login`，校验 Google URL、回调和 state，登录码只写 helper stdin。不会提取 OAuth token、读 keyring 或认证文件。helper 退出成功只表示流程结束，用户仍需显式重新连接来验证原生认证。

## 生命周期与独立复核修复

| 发现的问题 | 实现与可观测验证 |
| --- | --- |
| rc.3 空白会话隐藏所有标签，普通 command 记录不会变成父模型 turn | 使用已发布 `uiConversation.views/events` 与 `uiSession.provide` 为真实人类入口贡献视图活动；真实 rc.3 assembler/header/view renderer 验证显示标签。日志仍严格只有 `command/run`、`command/done`，不伪造消息或 turn，不自动调用模型。 |
| 异步预检期间可变请求对象可能改变 operation ID 与记录对应关系 | 在入队和 await 前复制请求，固定 operation ID。回归在入队后、预检中和完成前修改原 action/id/text/image，证明实际发送、文件名、最终回执和原 ID 幂等均不漂移。 |
| 宿主 stop/dispose 可能早于预检或 worktree 准备结束 | owner generation 和 active-actions 等待覆盖全部准备过程；停止后不能迟到启动。已经创建的 worktree 以 not-dispatched 记录保留。 |
| 根进程先退出时，同 PGID 后代仍可能运行 | 真实 detached 子进程回归要求继续检查并终止整个自有进程组；无法确认消失时不能标为 clean 或释放 lease。 |
| root prompt 返回后仍可能有工具、审批或 child activity | 常驻 ACP handler 保持接收更新；root stop reason、已知活动、会话关闭和本地清理分别记录。配置修改与下一 prompt 在已知活动未结束时被阻止。 |
| 原生登录与连接同时修改共享原生 profile | 私有 durable session/login locks 和短 profile gate 互斥；冷启动也检查 unresolved owner locks。未知锁不自动清除，不宣称能锁住插件之外另行启动的 CLI。 |
| 连接丢失后自动补发可能重复执行用户操作 | 精确 typed input 先持久化，再发送一次。原 operation ID 可查询既有结果，内容变化被拒绝。结果未知时不重放、不换账号/runtime/headless，不把 reconnect 当作未执行证明。 |

Session 元数据、操作记录和锁保存在私有目录，目录 0700、文件 0600；本地展示历史有大小限制，完整出站操作单独保存。断开、原生 close、自有进程组停止和未知远程／detached 工作是不同事实。退出桌面标签页不等于取消已派发操作，断开也不等于 logout。

## 已执行的源码与独立包检查

| 检查 | 结果与范围 |
| --- | --- |
| Companion `npm test` | **89/89 通过，0 skipped**。涵盖真实 stdio framing、权限重入、root 后事件、operation 幂等／未知结果、worktree、模型选择、原生登录 fixture、owner 约束和 React 图片行为。 |
| Companion `npm run test:package` | 通过真实 `npm pack`、源树外干净安装、公开 ACP/machine ESM、严格 TypeScript、包装命令、supervisor/journal 和旧接口消费。没有调用真实模型。 |
| Companion `npm run test:host-install` | 上述同一 tarball 通过真实 DSH 0.2.2-rc.3 CLI 的 disposable profile 安装；没有修改用户 profile。 |
| Flywheel 根目录 `npm test` | **169/169 通过，0 skipped**；包含 npm 安装器、发行门禁及单包依赖契约。 |
| Flywheel `make quality-gate` | vet、fmt-check 与完整 Go coverage 通过；所有受测函数保持 **100.0%**。Go 生产修改仅为精确 companion 版本同步。 |
| Go → Node → public companion 桥 | 最终依赖下 9 个顶层 driver/activity/title 桥通过，0 skipped；覆盖 DSH、Codex、Claude、Kimi、Antigravity。Claude 使用已核实的 optional 0.7.0 tarball，并非缺依赖跳过。 |
| Go machine 跨进程检查 | `TestAntigravityEvidenceJEVOfflineBridge`、`TestAntigravityEvidenceInvestigationOfflineBridge`、`TestPacketImagesClaudeSDKOfflineBridge` 均通过，0 skipped；最后一项包含 execute、recover-original、recover-changed-source 三种情形。它们验证既有 machine 接口，不能作为 ACP 真实模型或图片理解证据。 |

Flywheel 完整插件检查和完整候选包安装的最终结果记录在本记录后续验收段及 PR #63。候选由隔离快照构建，源码身份、打包身份、安装证据分别记录；源码测试不能代替候选消费。

### 完整回归发现的测试夹具问题

首轮 Flywheel 完整插件检查执行 722 项，721 通过、1 失败、0 skipped；由于 `&&`，随后 legacy Host 检查当次没有执行。唯一失败为既有 Codex `subscription-stop-exit-nonzero`：预期进程在 SIGTERM handler 中退出 23，但 fixture 在 stdin EOF 后可能先自然退出 0，导致根本没有执行目标信号场景。窄复现记录了 `exitCode:0`、`signal:null`、`sigtermObserved:false`。

修复仅让 fixture 的三种 `subscription-stop-*` 信号场景保持 event loop，直至真正收到预期信号；生产 proxy 与原断言均未改变。相关五种信号／malformed-tail 场景全部通过，随后对冻结源代码重新执行完整插件门禁，最终结果单独记录。

Companion 首轮 CI 的 Linux Node 22/24 两组均通过；macOS 两组在新增 ACP fixtures 中遇到 `/var` 与 `/private/var` 临时目录别名，触发预期的物理路径检查。三份测试改为先 `realpathSync` 临时根，没有放宽生产目录／权限检查。修前在符号链接 TMPDIR 下实际复现，修后相关 19 项及全套 89 项均通过。仅测试文件变化，不重标原始 tarball 的源码或摘要；验证源码提交为 [`dd11abd89e2d1693a89a815d4b0ae4b04fafa7fd`](https://github.com/ckanner/dsh-subagent-antigravity/commit/dd11abd89e2d1693a89a815d4b0ae4b04fafa7fd)，tree `05ecd07c88b81ebe1818312f69cd82c722c3799a`。真实 macOS CI 结果以该后续提交为准。

## 官方 CLI 实际证据与未测范围

生产只准入来源 manifest 中固定的 **1.5.14** 官方字节。四个平台（Linux/macOS × x64/arm64）归档与解包后的程序均实际下载并校验；插件本身不下载或替换用户 CLI，持有的进程禁用自动更新，未知版本／wrapper 被拒绝。1.5.13 是历史最低版本依据，不是当前准入范围。

只有 Linux x64 实际运行 `--version`、`acp --help` 和未认证 ACP initialize。精确二进制 SHA-256 为 `37873b8062391b6202a3d3199ec012abc9e53aab720d26cd65699441497d8c94`。初始化协商协议 1，观察到 image/embedded context、session load/resume/list/close、terminal login、logout 和 system-prompt append 的能力声明；客户端明确没有使用 append。详见配套源码中的 [provenance manifest](https://github.com/ckanner/dsh-subagent-antigravity/blob/7d5056c81688cc9408f3be32fc38ac7388b97377/lib/acp/cli-1.5.14.json) 与 [原始无凭证 probe](https://github.com/ckanner/dsh-subagent-antigravity/blob/7d5056c81688cc9408f3be32fc38ac7388b97377/test/fixtures/acp/official-cli-1.5.14-linux-x64.json)。

没有发送真实 `session/new`、Google 账号登录、prompt、模型切换或图片。以下仍需真实目标机器和明确选定账号的验收：原生认证结果、Gemini 交互、实际图片理解、权限和多轮恢复、root 后原生后台工作的停止效果、macOS/arm64 执行。离线 fixture、能力声明和进程组回归都不等于这些事实。

没有作出 Google 对本项目的政策批准、账号权益、额度、费用、质量、性能或整个 native/remote 任务树终止保证。没有合并 PR、打 tag、发布 npm、修改真实用户账号或安装到真实用户 profile。
