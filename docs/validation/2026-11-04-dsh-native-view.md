# DSH 原生 Flywheel 视图：修复与验证记录

日期：2026-11-04。环境：Linux amd64，Node 24.22.1，npm 12.2.1，Go 1.27.3。

本次针对用户安装后的截图反馈：底部 Flywheel 大弹层越过窗口右边界，遮挡对话、输入框和宿主控件，且配置、任务与诊断混在同一个临时弹层中。

**状态：已按用户提供的三个固定版本文档完成校正，源码与离线验收记录如下。本轮用户已明确授权提交分支、创建 PR，并在自行验证后合并；具体提交／合并状态以 GitHub 为准。本改动没有发布到 npm，也没有替换用户 Mac 上已安装的插件。已发行 0.28.0 仍使用旧浮层。真实 DSH 组装界面和 Mac 视觉验收尚未完成。**

## 1. 接入决策

完整控制页面注册为原生 `conversation.view` 的独立 `flywheel` 条目，使用 DSH 的顶部视图导航，与对话、轨迹并列。删除底部大浮层，不替换宿主已有条目；输入框、权限与模型选择器、右侧栏保留宿主归属。

Flywheel 页面承担已有 Go 任务的启停、状态、工作单元、人工处理项、有效模型配置和环境诊断。需求讨论仍在对话中，父会话事件仍由原生轨迹展示。完整配置放入可展开区域，长字段换行；原始 JSON 在局部区域查看。

本次不新增自绘侧栏，也不虚构一个从 dock 全局切换视图的 API。后续若需要边聊边看任务摘要，应使用宿主公开侧栏槽位，并单独验证与文件、变更等原生工具并存。

### 核对的 DSH 依据

- 声明版本为 `0.2.1-rc.5` 的源码快照 `b830ec950faba0a46dda564be3e2d854afed94f1`：[槽位规则](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/docs/subsystems/slots.md)、[Conversation 合同](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/client/ui-conversation/src/client/contract/slots.ts)。这是版本声明与源码核对，不声称核验了发行 tag 映射。
- 用户指定的同一源码 `b830ec950faba0a46dda564be3e2d854afed94f1`：[外部插件实践](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/preset/agent-preset/skills/cordis-plugin-development/references/practices.md)、[UI 规范](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/.agents/skills/dsh-client-ui-ux/SKILL.md)、[布局说明](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/client/ui-layout/README.md)。

第一轮草案误用了 DSH 内部 Client 包，虽通过当时的组件夹具测试，却违反外部 plain-JS 插件规范。最终客户端运行时只加载 React。按钮、输入框和加载指示器按固定宿主源码实现于插件自身，保留 MIT 归属；状态实现符合公开结构化 StoreHandle 契约，由宿主创建、缓存并绑定会话读取 hooks。CSS 由 React 视图的 style 元素拥有，不从组件外操作 document.head/body。颜色只引用实际存在的 `--dsw-alias-*`，使用宿主 locale，字体继承宿主。路径与展开状态属于查看状态；异步请求和显示快照位于 apply 层控制器，通过自有 observable 注入。控制器以会话绑定的 actions 身份隔离，Go 继续独占业务状态。

按 ui-layout 规则，Conversation 内的视图位于既有标题栏下方，不重复叠加独立管理页的 macOS 48px 留白，不改全局窗口变量。错误、等待和提示使用预留反馈区，完整详情可局部滚动；持续的未知结果提示不会被后续只读查询的加载状态隐藏。

## 2. 异常行为与审查中关闭的问题

| 场景 | 当前处理 |
|---|---|
| 打开、关闭或切换视图 | 不自动启动、停止、扫描或探测任务；不自动轮询 |
| 启动／停止等待中 | 同一会话只保留一个在途请求，锁定目标路径 |
| 只读请求期间修改路径 | 仍等待原请求结束，旧响应不能覆盖新选择 |
| 会话切换或绑定代际变化 | 不把旧请求的结果交给另一个会话或新的绑定 |
| 路径不存在、cwd 缺失、进程无法创建 | 仅在宿主结构化证明未派发时，显示修正指引并允许手动重试 |
| 已派发的启动／停止请求超时、断连、非法响应或进程失败 | 保留“结果未确认”，禁止重复启停，直到成功读取原目标状态 |
| 状态读取失败 | 保留上次有效快照及确认时间，明确标为待确认 |
| 诊断存在阻断项 | 展示完整失败报告；未测试的账号、额度和推理不冒充通过 |

独立审查发现并修复了两项具体恢复缺陷：

1. 浏览器不能把输入的路径与 Go 的真实路径做字面比较。`/var`、符号链接、尾部斜杠或 `..` 经规范化后可能不同；任务身份仍由拥有文件系统权限的 host 校验。
2. 将派发前错误一律标记为未知，会让不存在的原路径永久阻塞界面。新增结构化派发证明后，可以区分已知未执行和执行结果未知。每个 host 命令失败都使用外层包装，子进程错误文字不能冒充宿主证明；旧格式和异常格式继续按未知处理。协议见 [PROTOCOL.md](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md#dsh-命令失败与是否派发)。

独立复核未发现剩余源码或协议阻断问题。该结论不包含真实桌面几何布局验证。

## 3. 已执行验证

| 检查 | 结果与边界 |
|---|---|
| Go 完整质量门禁 | `make quality-gate` 通过，vet、格式与逐函数 100% statement coverage 保持；[日志](evidence/dsh-native-view-quality-gate.txt) |
| 插件干净依赖安装 | `npm ci --ignore-scripts` 通过，53 个包 |
| 完整插件构建与测试 | **75/75 通过**，其中 UI 19 项、真实发布版 renderer 集成 1 项、控制面 9 项；[完整日志](evidence/dsh-native-view-plugin-tests.txt) |
| 根目录 npm 契约测试 | **125/125 通过**；[完整日志](evidence/dsh-native-view-npm-tests.txt) |
| Go → Node → DSH / Codex Pro 离线桥接 | 两个指定桥接测试通过；[日志](evidence/dsh-native-view-bridge-tests.txt) |
| 隔离源码六包构建 | 四个平台二进制、插件、meta 共六包通过；[日志](evidence/dsh-native-view-build.txt) |
| 同一候选的真实 npm 安装流程 | **7/7 通过**，使用隔离 registry、用户目录和 DSH 协议桩；[完整日志](evidence/dsh-native-view-install-tests.txt) |
| 浏览器产物与 host 语法 | `node --check` 通过 |
| 差异空白检查 | `git diff --check` 通过 |

UI 测试运行构建后的插件、自有控件与状态实现、React 和 jsdom；只允许客户端加载 React。普通行为测试对槽位桥接和 Remote 使用适配器。新增的集成测试真实加载 `@deepseek-ai/cordis@4.0.5`、`dsh-client-ui-renderer@0.2.1-rc.5` 和 `dsh-client-ui-slots@0.2.1-rc.5`：验证等待原生声明、与对话／轨迹并存、隐藏与重开、会话及绑定代际隔离、声明收起／重建和插件卸载。没有槽位崩溃，组件外 DOM 保持不变，样式随 React 视图卸载。另有真实 handler / runControl / fixture CLI 的往返测试，覆盖可修复的派发前错误、进程已执行后的未知结果、子进程伪造错误标记及路径规范化。

响应式和主题规则已做源码核对。独立审查按固定默认色板计算亮／暗文字对比度，说明、占位符、状态文字改用可读的 primary/secondary；状态色作为边线等辅助标记。默认 secondary 在基础背景的对比度约为 5.80／12.11，在暗色卡片背景约为 9.25。这是色板计算，不是实际显示器或所有自定义主题的验证，也不作整个界面的 WCAG 认证声明。

另外，在隔离 Linux Context 中安装官方 rc.5 的 CordisInspectRegistry、SystemPrompt、ToolRuntime 与 tool-cordis，实际调用 `cordis_inspect_list` 和 `cordis_inspect_query`：先读取 Service／Event 目录，再读取已列出的 commands／tools 具体契约。目录包含 98 个 Host Service 和 76 个 Event；[检查摘要](evidence/dsh-native-view-inspect.txt)。没有连接用户桌面的 Client，因此不宣称完成目标 Mac 的 live Client inspection。客户端契约由固定源码与真实已发布 renderer 集成验证。

上一轮对话中的交互示意展示的是当时草案，不能作为本次最终控件实现或原生宿主视觉验收的证据。jsdom 不计算实际页面几何，也不代替浏览器截图。

六包构建从没有 `node_modules` 或 `dist` 的源码快照开始，在快照内按插件锁文件安装构建依赖，明确禁用生命周期脚本，再编译 CSS Modules；安装包的运行时依赖不包含这套开发依赖。

### 开发候选身份

本次基线是 `3cb79b9b7cffaacb799ecd804a0d7cdd885635e1`，工作分支为 `fix/dsh-native-task-view`。测试候选保持仓库当前版本 `0.28.0`，仅用于隔离验证，`source.dirty: true`，**不可发布，也不能覆盖 npm 已存在的 0.28.0**。

构建与安装测试使用的源码快照 SHA-256 和六包完整性见 [candidate 记录](evidence/dsh-native-view-candidate.json)。验证日志和 candidate 元数据在构建之后补入；测试后的生产代码、依赖锁文件和构建脚本未再修改。CSS 导出映射排序后，已逐字节确认六包候选中的 client.js、control.js、index.js 与插件测试使用的产物一致；第三方许可证声明也已包含且内容一致。后续发布仍须使用新版本、已审阅提交、准确 tag 和新的完整发行候选。

## 4. 仍需实机验收

连接的浏览器无法访问执行环境的本地测试服务；直接打开内嵌测试页也被浏览器安全策略拒绝。本次没有绕过该限制，也没有真实 DSH/Mac 截图。因此以下项目尚未通过：

- 目标 Mac 和用户实际 DSH 版本的插件加载、原生导航与完整页面组装。
- 390 CSS px 的验收最小宽度、常见桌面宽度、缩放、左右侧栏组合；检查对话、输入框、按钮和长路径没有相互遮挡。
- 亮／暗主题、键盘焦点、输入中调整窗口、长模型名和错误详情。
- 真实桌面断连、插件重载、切换任务与后台运行的可见结果。

Go、DSH SDK、Kimi 和 Codex Pro 的实际模型调用、账号、Seatbelt 等实机验证也不由这次 UI 离线检查证明。本次未修改 worker 权限或辅助能力策略，完整 worker 历史仍不会自动并入父会话轨迹，Go 任务也未冒充原生团队或后台任务条目。
