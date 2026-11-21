# Flywheel UI：有效配置、任务入口与恢复一致性验收

[验收索引](README.md) · [中文方案](../plans/2026-11-19-subscription-cli-workflows.md) · [English plan](../plans/2026-11-19-subscription-cli-workflows.en.md)

## 范围与来源

本记录对应 [PR #60](https://github.com/ckanner/flywheel/pull/60) 中用户批准的 UI 增补。
中英文方案 §14.3 先提交于 `d91860c3e89a44af03f406a133a85ee50d9ee958`，
主体实现与初版记录提交于 `6b897618f3dc3e25e6cfcf092ff2d216cea25ff7`，
已复审并执行下述本地检查；后续 CI 修正另记于下节。上一轮
[编排与 JEV 验收](2026-11-20-orchestration-adaptive-selection.md) 的结果保留原来源，
不能代替本轮新增代码的验证。

源码版本保持未发行的 `0.37.3`。独立 Antigravity `0.2.0` 与 Claude Code Pro
`0.7.0` 的已审归档没有因 UI 改动重打或改变身份；后者仍使用
[配套 PR #12](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/12) 的公开接口。
本次不合并、不打 tag、不发布 npm，也不改用户实际配置或账号。

2026-11-21 事实更正：官方 npm registry 已存在另一份 `0.37.3`。上段的“未发行”
仅指本记录对应的源码与离线候选，不表示该版本号可供再次发布；历史提交、候选身份和
验证结果保留。最终交付改用 `0.38.0`，见[补齐验收](2026-11-21-complete-controls.md)。

## 实现结果

### 有效配置与决策事实

- 配置诊断、执行候选编辑及新运行初始快照统一解析 `execution-policy.initial`。
  原 `routes/bindings` 继续保留声明含义；旧任务缺少新快照时显示未记录。
- 初始配置、下一次已选配置、最近实际调用分别使用对应冻结事实。
  固定 Orchestrator 的空 effort 不被替换成 high；pi/omp 合法的未指定模型也不猜值。
- JEV 保留唯一 `jev.enabled` 开关，显示实际解析的订阅绑定、思考等级和配置来源。
  编辑 global/project 与任务实际选中的 task/project/global/embedded 文件分开展示。
  保存后重读有效值；保存未知或重读失败不会把旧值称为当前值，也不会重发保存。
- 图形设置支持初始候选、允许集、切换次数及 Antigravity 原生模式和观察人数阈值。
  修改共享绑定时创建专用引用，保留其他角色、路线、权限和会话策略。
- JEV 的候选身份在决策当时冻结，显示建议、是否采用、实际选择和原因。
  新显示映射在整个响应中共用 64 KiB，过大的 nativeOptions 保留摘要及省略标志；
  完整身份放不下时整体省略并计数，磁盘冻结记录保持完整。

### 协作与图片

- 配置加载与编辑器提前阻止非 Execute 的 Antigravity Swarm；引擎和 driver
  继续核对 Full、独立审查、2–8 项互不重叠的写范围及当前 packet 的图片条件。
- 分工只有在当前契约指纹、独立审查及精确批准全部匹配时才标为已审。
  超过 32 KiB 的分工展示整体省略并标未核实，不能展示截断的批准范围。
- 子成员按同一次 invocation / engine run 的真实事件关联，未观察、未完成与未知
  分开展示；请求 Swarm 或根调用结束均不证明所有子成员完成。
- 没有图片也能使用 Gemini。当前 Antigravity 自动化输入是文本；交互式 CLI
  粘贴媒体不等于 Flywheel 的 headless 原图通道。
- 新 `control images` 只读用户明确选择的仓库图片，验证路径、无符号链接、格式、
  解码和大小后返回摘要及最长边不超过 160 像素的 PNG 预览。简报携带未审路径与摘要，
  原图仍需经过 packet 审查，并经已有 Claude SDK 图片通道派发。
  聊天附件、缩略图与文本路径均不会自动成为已审原始图片输入。

### 创建任务与恢复

- 空任务界面可直接创建工程、修复或只生成报告的调查。默认 Auto、review-first；
  delegated 需要明确选择并勾选本次实施授权。Quick/Full 位于高级选项。
- Host 对新任务只解析已有父目录，Go 负责创建账本与状态。
  调查表单只授予明确列出的最多八个本地文件读取，60 秒、每文件 256 KiB、
  累计输出 16 KiB，不运行命令、不访问服务、不授予实施范围。
  `start(run:true)` 登记后以同一保存 revision 启动，避免 UI 再发一次启动请求。
- 创建结果未知时保留草稿和原目标，禁止自动重发。只在明确核对后允许原任务重试；
  隐藏、重新打开界面不会另建目录。
- 成功响应的 canonical 路径只通过同一请求响应，或宿主保存的成对
  `requestedTaskDir/requestedCwd` 证明映射。旧未知屏障随路径映射，仍需成功的
  最新状态查询才能解除；投影可能合并 pending 事件也能恢复。
- 核对确认已有 state/preparation 后清理旧草稿和旧授权。缓存属于真实会话绑定，
  不跨会话继承；图片预览不会从缓存恢复。

## 独立 review 及修正

Go、设置页、工作台与任务入口由不同作者交叉复核，最后对恢复控制器再做只读复审。
本轮发现并关闭以下问题：

| 问题 | 修正与证据 |
| --- | --- |
| 路线旧声明覆盖实际 initial，固定 effort 被补值 | 统一解析及冻结初始身份；配置、状态和 UI 回归 |
| 非 Execute 可保存 Swarm | 加载时拒绝、编辑器禁用，保留最终执行准入 |
| 订阅 JEV 显示空模型，编辑范围冒充实际范围 | 解析 jevBinding；来源区分、保存后重读及失败状态回归 |
| 旧 pi/omp 的合法空 model 被新显示验证器拒绝 | 保留空值并显示未指定，旧任务／运行记录回归 |
| 新候选映射及分工可撑破控制响应限制 | 全响应共享显示预算、完整身份整体省略；超过 1 MiB 原始配置及 100 行运行记录回归 |
| 新建任务被 Host 的已存在目录检查拦截 | 仅 prepare/start 允许新路径，Go 负责创建；其他动作仍拒绝缺失目录 |
| 重复调查证据不符合唯一性约束；隐藏字段阻挡调查 | 去重证据，仅验证当前任务类别字段 |
| 创建成功经后续核对确认时旧授权草稿残留 | 确认 state/preparation 后退役缓存，新任务重新 review-first |
| 符号链接导致草稿与未知屏障失去目标关联 | 同请求来源证明、canonical 映射及失败状态持续阻断 |
| 新配置只读刷新使旧 fixture 断言失真 | 用真实配置响应核对请求、作用域和保存次数，保留原行为断言 |
| 新依赖未进入旧宿主隔离 fixture | 补齐两个真实传递模块，rc.5 宿主复验 |

最终恢复专项 12/12，来源投影专项 9/9；另由独立审查者复验首次自动 canonical
状态查询失败、重新挂载、后续成功核对的完整路径，1/1 通过，未重复 prepare。
这些专项属于完整回归的补充，不替代全量门禁。

## 验证环境与已完成检查

Linux，Go 1.27.3、Node 24.22.3、npm 11.19.1；公开 DSH 0.2.2-rc.3，
另检查最低支持的 0.2.1-rc.5。所有安装和协议演练使用隔离目录。

| 检查 | 本地结果 |
| --- | --- |
| `make quality-gate` | 通过：vet、格式及全部 Go 包；1,134 个被测函数各自 100.0% |
| 原生 CLI 与成对会话演练 | 36/36 trials、96 次调用；全部 completed，oracle 全通过 |
| 实际 Claude SDK 图片与只读恢复桥接 | 通过 |
| 独立 Antigravity 插件及真实归档消费 | 35/35，包外公开 API、真实 DSH 安装与编排证据检查通过 |
| 根 npm | 167/167，0 skipped |
| Go→Node Provider 桥接 | AGENTS.md 中九类主测试组合通过，包含 DSH、Codex、Claude、Kimi、Antigravity 及活性／命名协议 |
| 完整 DSH | 629/629，0 skipped，包含全部 Claude／Provider 回归；最低支持宿主另 10/10 |

完整 DSH 之后，仅澄清调查路径相对于会话工作目录的中英文标签及对应选择器，
再构建客户端并复跑任务入口专项 12/12。协议、创建与恢复行为没有在全量检查后改变。
目录路径、累计输出预算和缩略图尺寸的文档表述也与实际 Go 实现对齐。

最终提交后的完整四平台候选只构建一次，并以同一归档执行真实安装检查；
PR 最终验收区记录其源码 commit、tree、snapshot、归档字节数、SHA-256 和 integrity。
Linux/macOS CI 使用 CI 构建的一份候选；其运行链接和最终 head 一并记录在 PR，
本地归档与 CI 归档不混用。提交前源代码测试不能提前算作候选或 CI 通过。

## macOS CI 后续修正

首轮 [CI 44826561500](https://github.com/ckanner/flywheel/actions/runs/44826561500)
的候选构建通过，macOS 在 DSH 检查中得到 628/629、0 skipped；唯一失败是既有
Codex worker 的 `full display progress preserves the actual task deadline and cleanup receipt`。
该 case 要求 1.5 秒内累计显示触发 1 MiB 省略标记，但 fixture 使用 5,000 条
256 字符通知，每条都经过真实公开 adapter 与 supervisor IPC。macOS 实际只输出
约 432 KiB 就到达截止时间，尚未满足该断言的前置条件。

日志中的 resultFrame 和 error 检查已经通过；位于失败断言之后的 timedOut、
truncated 与 cleanup 断言当时没有执行，不能据此宣称这些检查已通过。
修正保持真实公开 adapter、生产展示上限和超时／清理语义，只改测试的数据生成与
调度余量：128 条不超过公开 16 KiB 单条限制的通知，总量 2 MiB；按小批等待 drain
并让出事件循环，随后挂起到真实 3 秒任务截止时间。保留原全部断言，并增加 burst
完成、实际显示距离 1 MiB 不足一帧、非 cancelled 及持续到真实 deadline 的检查。

按每条 2 ms 的相同受控通知速率复现：旧 fixture 只输出 368,012 bytes，640 条
progress，没有省略标记；新 fixture 保留 1,035,979 bytes 的展示输出，在约 404 ms
观察到省略标记，随后仍存活约 2,768 ms，直到真实截止时间后取得 timedOut、非
truncated／cancelled、cleanup confirmed 且无残留进程的回执。这是本地受控复验，
不回填为首次 macOS 失败时已经执行的断言。

修后目标 case 1/1、完整 Codex worker 文件 49/49、共享 fixture 的两个大流量 activity
场景 2/2 均通过，无跳过。不同作者独立核对真实公开接口与原断言保留情况；完整候选
及新提交的 Linux/macOS CI 终态记录在 PR 最终验收区。

## 验证边界

浏览器视觉检查尝试启动 Chromium 时被当前执行环境的 socket 限制阻断，未生成
截图，未完成窄屏几何、实际浏览器焦点与原生 Mac 桌面验收。DOM 交互、会话切换、
加载和错误恢复测试通过也不能替代这些项目。

没有运行真实账号推理、套餐额度或账单检查，也没有以离线 fixture 评价模型质量。
尚未发布的版本号和本地安装成功不构成 registry 已发行证明。
