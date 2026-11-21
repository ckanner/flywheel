# 0.38.0：模型选择、分工与图片编辑及执行状态补齐

[验证索引](README.md) · [PR #60](https://github.com/ckanner/flywheel/pull/60)

本记录对应 2026-11-21 对“继续完成”的实施和独立交叉 review。基线为
`dd8191105248b2447e473693f019f3eb1308c04a`，新增代码、协议、schema、双语使用文档及
本记录随同一 PR 提交。最终源码提交、完整候选归档身份、实际安装结果及 CI run 由
PR 中的交付记录和候选 `candidate.json` 绑定；本文记录源码行为及已执行的本地验证，
不把源码测试当作 registry 发布或用户 Mac 桌面验收。

## 本轮交付

### Provider、模型与执行候选

设置页保留 Provider/backend、channel、精确 model ID、thinking 和 native-profile，
增加显式读取原生客户端模型目录的选择器。Antigravity 使用 `agy models`，Codex Pro
使用 app-server `model/list`，Claude 使用 SDK 的初始化模型元数据；每份结果绑定
harness/provider/channel，显示来源、范围、截断及账号权益未知。

读取目录不发送用户消息、不创建 Codex thread/turn、不登录、不改变 Flywheel 配置。
Claude 元数据查询不持久化会话，禁用 settings 来源、hooks、plugins、工具和 MCP。
最多 256 项、Codex 八页，累计输出 1 MiB，原生进程 10 秒、Go watchdog 15 秒；
异常文本不暴露给界面。Kimi、DSH 尚无已验证的无会话目录通道，明确返回 unsupported，
保留手动输入精确模型 ID。原生目录中的 effort 还要满足 Flywheel 绑定契约。

执行候选表支持创建、复制、编辑、删除、引用路线查看、每候选 session-mode 和 permission。
路线权限调整会复制被其他路线共享的候选，再同步该路线的 initial、allowed、质量顺序及
后备引用。质量顺序支持增删、排序；渠道后备展示渠道链与模型映射变更，经明确采用后
写入同一草稿。冲突和循环阻止保存，不扩大其他路线的权限，不修改冻结运行。

### 每 packet 的实际输入资格

`control node` 的资格视图复用引擎实际输入判断：图片传输、native-profile、Full、
分工声明以及独立写入范围。Go 同时检查当前冻结 packet 指纹、有效配置、execution-policy
身份，以及 reviewed task 的确切批准、计划评审、活动投影和不可变规划导出。

任何相关证据变化都显示 unknown；其他活动 packet 或规划导出损坏也不能显示肯定资格。
无图任务可使用 Gemini；Swarm 仍只用于符合 Full、已审独立分工和当前输入能力的 Execute。
这项预览不证明账号可用、额度、客户端在线或模型实际执行。48 KiB 候选和 32 KiB 编辑
快照超限时完整省略并说明原因，不缩短具有授权意义的路径。

### 分工编辑、独立评审和恢复

结构化编辑使用真实 schema：`{version:1,tasks:[{goal,writes}]}`，支持 2–8 项。
保存绑定 state revision、request-id、packet 指纹及当前计划 revision/digest。
同一 run lease 内重新核对，仅修改选中 packet 的 collaboration，不扩大 schedule.writes，
不改写已完成 packet、当前活动文件、旧批准或历史评审。

新完整 plan 与 review-only request 原子采用；独立 `plan-review` 通过后才可激活新版本。
保存并审查只运行 planning-only，不授予 Start，也不启动 Execute。经批准但尚未授权
Start 的活动设计可以准备分工修订，实施仍经过原有 Start 检查。

请求带不可变 `origin:collaboration-edit`；producer 使用 request-id 的摘要并明确标记
控制修订，不能伪装模型调用。普通 plan 的自由反馈不能冒用此身份。新增请求也不能预填
planner stage 绕过规划轮次计量；原子采用的任一步失败都不泄漏部分计划或请求。

lease 内确认未采用的无效输入会记录带命令摘要的拒绝审计，并返回 rejected。
这不改变计划批准，start:true 也不会启动进程。CAS 或传输不确定仍保留 unknown；
同 ID 不同命令内容拒绝复用。实际状态查询能恢复正确的 collaboration action。

界面把草稿和未决请求放在当前 host Session 的任务/packet 缓存中，关闭检查器、切页、
切任务和重新挂载都不会丢失。回执同时匹配 action 和请求 ID；已确认未派发的失败即使
发生在卸载期间也能恢复可编辑草稿。完整 UTF-8 请求超过 32 KiB 时不能提交。
拒绝审计增加 revision 后，可以明确刷新版本并保留草稿，但必须重新确认计划、契约、
父范围和原分工都未改变。

### 交付前范围复核：图片、新建任务与决策状态

首轮源码 `ac53acff3c293c20ece22ac842d13f8aca23aac5` 已完成上述模型与分工控制。
对照最初需求再次复核后，进一步补齐以下三个操作闭环；这批结果不借用首轮候选的
安装或 CI 证明，最终交付重新构建并验证完整归档。

**图片输入。** Execute 节点提供仓库图片路径选择、真实解码预览和摘要核对，以及新增、
替换、明确清空。`planning action:input-images` 携带精确 packet、契约指纹、计划引用及
显式 value；只有 `value:null` 表示申请清空。缺失字段、空数组、未知字段和过大请求拒绝。
保存时重读源 checkout 原始字节；预览后图片改变不能沿用旧摘要。路径统一按实际项目根
解析，子目录会话预览、配置发现与保存采用同一根目录。

图片修订复用规划 CAS、run lease、完整投影与不可变导出校验，新增不可变
`origin:input-images-edit`。新增、替换和清空都产生待独立评审的新完整 plan，原活动输入
保持到评审与激活完成；保存并审查不授予实施 Start。测试经过真实独立 reviewer 到实际
计划激活。节点只读声明不打开原图；`status:unknown` 不能显示成“没有图片”，也不能把
旧活动声明挂到缺失或损坏的新 plan 上。预览 data URL 仅在当前组件内存中，不进入持久
会话草稿或控制器缓存；重挂载后必须重新预览才能保存替换。

**已有任务中新建任务。** 工作台提供明确的新建入口，支持工程、修复和只报告的调查。
Go 确认新任务及规范位置前保留原任务选择与画布；创建结果未知时绑定原请求核对，
不能自动重试或绕过旧任务的不确定写入。取消新建清除该草稿授权，成功后下一次仍默认
Auto、review-first；正常收起再打开可保留当前草稿，不借用上次任务的实施授权。

**决策与实际调用。** 模型决策冻结契约指纹；在实际 Execute intent 落盘前，以 packet、
attempt、plan、契约、候选 ID 和完整有效绑定匹配，写入确切 decision-id。owner 在 CAS
保存时同步有界调用摘要，parallel worker 先合并所属 session，不能覆写整本决策账本。
决策卡直接区分已选择、尚未启动、已启动、已完成、失败、取消及未知，并展示确切调用。
没有此关联的旧历史保持 unknown；不按模型名字或时间猜测归属。

终态引擎字段为 `dispatch:dispatched`；原生 lifecycle 的 `accepted` 只在相应事件持久化
后成为正向观察位，不能代替终态或重放准入。完成必须同时有收到/消费回执、已确认清理
和已知 outcome；一次调用完成不等于任务验收通过或每个 Swarm worker 成功。

## 独立 review 及修复

实现按模型目录后端、设置 UI、packet UI 和 Go 控制面分工，作者之间交叉检查。
下列发现均已修复并加入针对性回归：

| 发现 | 修正与证据 |
| --- | --- |
| 只有 review 的分工修订被实施 Start 阻挡 | `AdmitPlanRevision` 保留精确批准/评审/激活，单独保留实施 Start 门禁 |
| 预览未核对兄弟 packet 和不可变导出 | 检查完整活动证据，损坏时 unknown；只读检查不重建缺失目录 |
| 状态查询把 collaboration 回执误报为 plan | 不可变 Origin；真实 engine status 回归 |
| 明确拒绝被永久当作未知写入 | CAS 持久拒绝审计、同 ID 查询、修正后新 ID 可继续 |
| rejected 且 start:true 可能启动旧流程 | CLI 在 rejected 回执后直接返回，测试断言不调用启动入口 |
| 相同 ID 的不同命令可能套用旧拒绝 | 完整 command digest 核对，拒绝 ID 复用 |
| 面板关闭丢草稿和未决 ID | 会话绑定的持久草稿缓存，覆盖真实卸载、切任务和重挂载 |
| 错误 action 回执解除全局未知 | action + request ID + 确定终态共同核对 |
| 未挂载时丢失已知未派发证据 | 可信本地 controlFailure 仅返回原 collaboration 缓存，隔离其他选择 |
| 浏览器未按完整 UTF-8 请求计大小 | 提交前 32 KiB 限制和回归 |
| 新 planning request 可预填 planner stage | 拒绝新请求预填 stage，保留已计数请求的正常恢复 |
| 新 metadata worker 漏出最低宿主和完整包门禁 | 补齐依赖复制、bin/export 完整性拒绝测试与安装消费回归 |
| 发布版本号占用 | npm 0.37.3 已存在，新的候选统一改用 0.38.0 |
| 已有任务缺少新建入口 | 保留旧选择的 provisional intake；精确创建回执或状态确认后才切换 |
| 图片只有简报预览，没有 packet 编辑闭环 | 新增、替换、清空均创建不可变 plan 并独立审查；真实激活回归 |
| 子目录图片预览和保存使用不同根 | 控制上下文统一 canonical 项目根；实际子目录预览到冻结回归 |
| 声明读取失败可能被显示成无图或错误 plan 的引用 | known/unknown 协议及 host/browser/DOM 校验；损坏或错位 packet 保持 unknown |
| UI 把 native accepted 当作引擎 terminal dispatch | 使用实际 dispatched wire；实时正向观察与未知/矛盾终态分别处理 |
| 决策卡缺少调用状态且可能由同名模型推测 | 精确持久 decision-id 与有界调用摘要，旧历史 unknown，独立交叉 review |
| 延迟任务发现可改变目录并清空新任务草稿 | 确认会话项目目录后开放入口，并固定本次草稿目录；规范目标仍由 Go 确认 |

最终跨作者 review 没有剩余阻断 finding。没有通过删除字段、降低门禁、跳过失败用例
或把未知观察填成成功来完成验证。

## 已执行的本地检查

环境：Linux，Go 1.27.3、Node 24.22.3、npm 11.19.1。

### 首轮实现验证（ac53acff）

以下为首轮实现实际结果，保留其身份；交付前补齐后的最终检查另列。

| 检查 | 结果 |
| --- | --- |
| `make quality-gate` | 通过：vet、gofmt、全部 Go 包，1,156 个被测函数各 100% |
| 完整 DSH 检查 | 主套件 688/688，最低支持宿主 10/10，均 0 skipped |
| 独立 Antigravity 检查 | 35/35，以及实际打包消费、Orchestrator/Swarm smoke 通过 |
| 模型目录 | Go 每函数 100%，Node 21/21；真实固定版本 Claude SDK 对接隔离假 CLI，仅初始化无用户消息 |
| 分工编辑 DOM 回归 | 16/16，含真实卸载、未知回执、拒绝修正和保存期间晚到结果 |
| Provider 跨进程桥接 | AGENTS.md 指定的 DSH/Codex/Claude/Kimi/AGY、活性和标题组合通过 |
| Claude 原图桥接 | Execute、原始输入恢复、来源改变后的只读恢复通过 |
| 会话成对演练 | 36 trials、96 calls，complete:true；六类场景各 observed 6，unknown/unobserved 0 |
| Root npm | 运行时、安装器和发行协议完整回归 168/168，0 skipped |
| 补充发行/宿主检查 | release gate 44/44；补齐共享模块后的 legacy-host 10/10 |
| 差异 | `git diff --check` 通过 |

跨进程桥接及会话演练运行真实引擎和 public adapter 的离线 fixture，不代表真实套餐账号
完成推理。候选安装新增模型目录的完整路径测试：已安装 `control models` → 包内 worker
→ 隔离 `agy models` fixture；必须对最终生成的完整归档执行，不能消费旧 tarball 冒充。
实际归档的 source commit/tree、bytes、SHA-256、integrity 和安装/CI 结果另记于 PR。

### 交付前补齐后的最终验证

| 检查 | 结果 |
| --- | --- |
| `make quality-gate` | 通过：vet、gofmt、全部 Go 包，1,168 个被测函数各 100% |
| 最终完整 DSH 检查 | 主套件 716/716，最低支持宿主 10/10，均 0 skipped；包括最后的草稿目录竞态修复 |
| 独立 Antigravity 检查 | 35/35，0 skipped；实际打包后的 public API、宿主、Orchestrator/Swarm smoke 通过 |
| Provider 跨进程桥接 | AGENTS.md 指定组合全部通过 |
| Claude 原图桥接 | 真实固定 SDK 下的执行、原始和改变来源后的只读恢复通过 |
| Root npm | 168/168，0 skipped |
| 原生构建与会话演练 | 0.38.0 构建通过；36 trials、96 calls，complete:true，六类场景全部观察且 oracle 通过 |
| 新建入口及跨作者检查 | 新建、原 intake 和发现/导航 44/44（含延迟目录发现及失败重试）；图片 known/unknown 12/12；生命周期及节点观察 17/17，均 0 skipped |

完整 Go 门禁第一次运行的行为测试全部通过，但新状态校验入口的一条拒绝分支未覆盖，
因此门禁正确失败。补充从完整加载校验入口拒绝损坏 decision/invocation 关联的断言后，
重新运行完整门禁通过；未删除防御或降低覆盖要求。最终安装和双平台 CI 使用补齐后的
新候选，其身份和结果记于 PR，不沿用 `ac53acff` 候选的通过结果。

## 发布与现场边界

2026-11-21 显式查询官方 npm registry：Flywheel latest 为已发行的 0.37.3；精确
0.38.0、Antigravity 0.2.0、Claude 0.7.0 均返回结构化 E404。本次候选不会覆盖旧 0.37.3。

AGY 0.2.0 的已审归档及依赖随 Flywheel bundled，因此 Flywheel 安装不要求先独立发布
AGY。独立插件按包名安装仍需要其自己的授权发行。Claude 0.7.0 是外部 optional peer；
缺失时基础安装可成功，但默认 Claude 路径不就绪。正式发布门禁要求 registry 中该版本
与已审 vendor 字节完全一致，当前不能绕过此条件。

用户 Mac 桌面、真实账号模型推理、套餐额度、原生真实 Swarm 行为和本机插件升级没有由
上述离线结果替代。此前 Chromium 视觉检查被环境 socket 限制阻断；本轮完成 DOM/宿主
行为和安装协议验证，没有生成新的真实浏览器或 Mac 现场验收证明。

本轮授权范围内未合并 PR、打发布标签、发布 registry 或改写用户真实 profile。
正式发行需要遵守仓库 AGENTS 的独立授权和 Claude 前置发行门禁。
