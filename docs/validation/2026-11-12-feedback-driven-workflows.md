# 反馈驱动工作流、受管调查与 JEV 有限决策：实现验收

[验收索引](README.md) · [设计提案](../plans/2026-11-12-feedback-driven-workflows.md) · [系统设计](../design.md) · [中文使用指南](../zh-CN/investigation.md) · [English guide](../en/investigation.md)

## 来源与结论边界

本记录对应 [PR #34](https://github.com/ckanner/flywheel/pull/34) 中包含本文件的实现提交。
提案审阅基线为 `main@22156861a84cb834797ba23b07d3f0a651fd7b36`，实现以同一 PR
的 `c350f169d800ab1ddbaeb06c87e714610bcf419a` 为父提交。保留原设计提案与历史验证，
不将原来的文档检查冒充为本次代码验证。

源码包版本仍为 `0.31.0`；本次没有发行、修改用户配置或变更默认模型绑定。验证环境
是 Linux、Go `1.27.4`、Node `24.22.1`、npm `12.2.1`。源码从固定 Git tree 重建并
逐 blob 校验；本地 Git index 对应原 PR HEAD，随后只修改本次实现文件。

本次提供可审阅的实现和离线行为证据。真实模型质量、JEV 采用净收益、Provider
缓存命中、生产延迟和 Mac 桌面验收均需要另外取证。`100%` statement coverage
说明所计量代码被执行，不替代断言质量、真实场景或性能实验。

## 提案到实现的对应

| 批次 | 本次实现 | 证据边界 |
| --- | --- | --- |
| P0：可观测基线 | 引擎、Plan、执行、验证、Review/Judge、受管调用、真实 backoff、持久化、交付及调查的 start/end；运行视图与会话评估识别新事件 | 累计区间与并行墙钟 union 分列；缺失结束保持 unknown。宿主准备、Provider 内部排队／推理／工具细分没有观测时不补造数据。 |
| P1：缩短闭环 | 稳定 check ID、候选／检查集／环境身份、完整输出摘要与有界头尾片段；保守进展判断；可信 Plan 检查点；执行者先跑相关检查 | Go 仍运行完整冻结验收并独立评审；文本中的“10→1”不被当成可信 oracle，候选变化但证据不足时为 unknown。 |
| P2：受管调查 | 版本化准备契约与状态、有界动作、报告、CLI 后台生命周期、桌面面板及实施交接 | 首个动作族仅 Go 读取文件、字面搜索、列目录名字。无任意 shell、外部查询或可写 Agent 调查；只调查不能转为实施。 |
| P3：执行策略与上下文 | 显式完整绑定允许集、质量升级／渠道后备分离、冻结选择记录、effort 冷启动、可信增量 Execute 上下文 | 默认仍固定绑定和既有上下文策略；离线验证协议与正确性，不证明哪种真实模型／effort 更优。 |
| P4：JEV 有限决策 | 独立默认关闭策略、Noul + Choice、固定版本、完整概率、数据允许集合、调用预算、持久回执、诊断动作采用、离线／显式 advisory CLI；旧影子改本地捕获 | 只允许从 Go 已批准的动作中选择；采用须显式开启。没有真实端到端净收益证据，不默认启用，不宣称提速或成本下降。 |
| P5：条件性调度优化 | 保留现有并行、共同候选复验、取消及交付机制 | 预热、新 ready queue、进一步流水化与已有候选接纳未新增；提案要求测量证明收益后再做，本次不以离线速度推断收益。 |

## 日常入口与交付保证

- **Task：**从目标自动组织准备、调查或实施；Auto 仍只是 Quick／Full 的选择策略。
- **New → 审阅 → Run：**保留。New 对调查只保存准备契约，不运行取证动作；复杂任务
  可以先形成方案，后续获得实施授权再继续 Full。
- **Plan / Exec / Verify / Review / Status / Stop：**保留直接使用；调查和实施共用
  受管后台生命周期，阶段成功或调查报告均不冒充整项代码交付。
- **明确修复：**可直接 Auto／Quick，依旧经过资格、完整验收、独立最终 Review 与 Git 交付。
- **排查并修复：**只有原请求已授权、结论 supported、回执全部可结算且工作区一致时，
  才能进入实现。来源报告、原范围、已知／未知 JEV 用量继续绑定，不因切模式或恢复重置。

报告的 `supported` 是分析者结论及引用的结构合法性，不是 Go 对因果关系的独立证明。
首版语义报告由宿主模型根据收集的观察提交一次；skill 不承担逐动作循环。

## 关键行为与恢复证据

### 反馈与规划

测试覆盖同一文件中的不同候选、同一候选的重复权威失败、检查集／环境变化、输出
截断与物理取消。空转判定不再仅依赖文件集合；执行失败但没有权威验收观察时，
依靠原有尝试／预算上限，不能臆造进展。

Plan 复用要求真实成功终态、重载合法契约、成功 Git 持久化及 worktree 同步。
普通返修可以保留计划；显式 Plan、verification-gap、契约／验证／调度变化、未知
外部 HEAD、旧任务没有可信证明及未解决调用都不会被缓存绕过。

### 调查、取消与交接

测试使用真实文件系统、Git 仓库、CLI 测试子进程与 localhost HTTP fixture，验证：

- CAS 竞争、不同工作区、损坏状态、符号链接、特殊文件、父目录替换与打开后 inode
  校验、UTF-8 截断、文件／总输出／动作数／deadline 上限。
- action intent 已保存但无可信 receipt 时保持 blocked；已有 immutable receipt
  可重复读取并只消费一次，不重新取证，不用后来的文件内容替换原观察。
- 后台子进程可启动、查询、停止。JEV 请求在飞时 token-bound Stop 取消请求，保留
  unknown 用量，不派发下一诊断动作，不产生实施 `state.json`。
- 可选 JEV 的 pending 与实际 action 的 pending 分别处理：前者可只读核对后取消
  或退回确定性基线；后者缺少结果时不得假取消或重放。
- 实际 L1 diff 检查原 `implementation-scope`；后续 packet 扩大范围也不能扩权。
  修复接纳后的来源变更被拒绝；跨日恢复按原派发日期记账且不重复导入。
- 调查 CLI 的 `awaiting-report` 为退出码 `0`，blocked 为 `3`，取消为 `143`；后台
  控制可以返回对应真实快照。`control stop` 本身成功仍为 `0`。

### JEV 与执行策略

- JEV 决策回执消费与唯一 action intent 在同一个 CAS 中保存；该 CAS 失败后只核对
  冻结请求，不新增请求。采用前重新验证 revision、候选、观察、契约和动作身份。
- 超时、取消、动作预算耗尽及提交报告时仍结算 pending advice；unknown 包含在调用
  上限中，缺失用量／费用不当作零。损坏配置不会阻止只读恢复，也不会授权采用。
- 合法调查内容超过决策快照边界时不派发 JEV、不伪计费，继续确定性动作；新 HTTP
  只能发生在显式允许的数据和独立预算内。硬预算无法证明单次上限时跳过可选请求。
- `decision replay` 强制 offline；`decision evaluate` 强制 advisory，使用独立
  journal 与显式预算，不执行动作、不改任务。替换 policy 不继承省略的数据授权。
- 质量升级仅由权威 L1 stalled 触发，不能把 quota/auth 等渠道错误解释为能力不足；
  切换受允许集和次数约束，保留尝试与预算。未知调用不可通过换模型或 effort 绕过。
- 增量上下文只用于引擎明确标记的 Execute 模板，要求同角色／绑定、稳定可读契约
  和已消费、明确 dispatched 且 completed 的可信回执；人工 Prompt、Review／Judge
  与不匹配上下文仍发送完整内容。新增三轮持久恢复测试：旧上下文成功，新权威内容
  对应调用被拒或失败，重新加载后保留原线程并完整重发，不能把准备过当成送达过。

### 控制台

实际构建后的 React harness 检查调查面板、原 revision、unknown 阻断、原回执
核对、Stop、双语报告、stdin 参数传递与畸形响应。调查准备态隐藏普通代码启动；
已交接实施后，调查只读 status 仍能接受 Go 返回的正常实施快照。

## 最终检查

下表区分最终检查、环境缺口与未取得的证据。

| 检查 | 结果 |
| --- | --- |
| `make quality-gate` | PASS；最后的上下文修补后重新运行，vet、格式与逐函数 100% 全部通过 |
| 核心十包 `go test -race`，以及 `internal/investigation` 独立 race | PASS；全部无 data race |
| `npm test` | PASS；142 项通过，0 跳过 |
| `npm --prefix plugins/dsh run check` | PASS；冻结依赖树上主套件 251 项、旧宿主兼容套件 9 项，均零失败、零跳过 |
| 三适配器 Go → Node 离线桥接 | PASS；冻结依赖树上 DSH、Codex Pro、Claude Code Pro 三组全部通过，无跳过 |
| `make build` | PASS |
| `FLYWHEEL_EVAL_BINARY="$PWD/flywheel" npm run test:sessions` | PASS；36 个真实引擎／公开 adapter 离线配对 trial、96 次 fixture 调用，独立 oracle 全部通过 |
| 双语导航、相对链接／锚点、嵌入技能、schema 与 diff 检查 | PASS；31 个修改 Markdown 文件，409 处相对链接、70 个锚点、125 个代码围栏；嵌入技能／schema 已纳入 Go 门禁；`git diff --check` 通过 |

核心 race 命令覆盖 `machine verify watchdog drivers cli modules diagnostic decision
metrics state`；调查包另外运行。最后的上下文送达修补另行通过相关 session 测试的
race。质量门禁包含 vet、gofmt 检查和逐函数 100% coverage，
未降低阈值，未改写有效防线以制造覆盖率。新增技能的 YAML 与嵌入内容由既有 Go
测试检查。文档路径／锚点检查为本次定向检查，不宣称仓库新增了独立文档 CI。

### 可选 Claude 适配器的来源

首次跨进程桥接中 DSH／Codex 通过，Claude 因可选 peer 未安装失败。精确
`dsh-subagent-claude-code-pro@0.4.0` 的 registry 请求返回结构化 E404，因此依据
仓库既有 CI 与 vendor 记录构建其已审阅源码，未伪造 npm 发行：

- 上游：`ckanner/dsh-subagent-claude-code-pro`。
- commit：`ec0b99176b67512f3ca1838ec2a349dcb0189214`。
- tree：`13c6a3f7e3f332eee98f4e5cde77ba8806c608c1`。
- 实际 `npm pack` 产物 SHA-256：`125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`，与既有审阅记录精确相同。

最终测试环境从未改动的 lock 重新 `npm ci`，189 个已安装锁项逐一匹配版本；再放入
独立 npm 安装的原始 Claude 包。Claude 的 45 个文件、Codex vendor 包的 79 个文件
均与相应真实 tarball 逐字节一致。源 `package.json`／lock 未改动。最终插件与桥接
检查在这棵冻结依赖树上重跑；此前临时安装造成的依赖版本漂移结果不算最终证据。

## 未运行与后续采用条件

没有调用真实 JEV 或模型服务，没有为实验新增付费渠道，也没有在用户机器上安装、
登录、修改默认配置或执行桌面账号验收。没有构建／发布六包候选、合并 PR 或部署；
本次未修改 npm 发行流程，因此六包候选安装不是本次源码变更的适用门禁。

上线采用仍应比较冻结任务集上的基线、核心改进无 JEV、核心改进加单动作族三个版本，
保留预算内成功率、独立 oracle、遗漏／返工、人工介入、完整墙钟和已知／未知用量。
离线 fixture 的正确结果不能填补这项证据。真实收益成立前，JEV、可变执行绑定和
增量上下文都保持显式 opt-in；P5 继续服从提案中的测量条件。

远端 CI 必须按本次实际提交 SHA 查询；旧文档提交的 runner 失败不是此次代码结果。
最新 PR Checks 与本记录的本地源码验证分别解读。

## 后续整合复核：当前主分支、DSH rc.2 与 Mac

本节补录初始实现之后的整合，不替换上文 `ac46b04` 的原始 Linux 验证身份。
已合入 #35 的 Skill 清理及 #36 的 DSH `0.2.2-rc.2` 兼容和六包版本一致性修复。
三个 Skill 冲突保留新的调查语义、较新 metadata 版本及只读关联／配置调度指令；
验收索引保留两项独立记录。当前源码六包统一 0.31.1，尚未据此发布 npm 或更新用户安装。

本轮额外修复三个 Mac 失败：非法 UTF-8 文件名通过目录读取注入缝验证拒绝行为；
工作目录解析失败通过窄注入缝验证且保留已有状态；插件传输测试使用物理规范路径，
避免 `/var` 与 `/private/var` 别名误判。生产路径默认仍调用 `filepath.Abs`，没有放宽
断言、跳过失败路径或降低覆盖率。

最终代码候选来源为 `828f54c9992360c356bb076e1c1b78ccb7315f51`，tree
`911f09f8362adfa3f8822159ea0708c57b7a2e45`；源码 snapshot SHA256
`5dfaa3d313f197a1df62f14360daf8e6e51b7418f75a2db9f3e242c43733f2b3`。
六包候选清单 SHA256 为 `1d79ced0e82262c1099cb87c0379618b7cd68738887bf623e7c9388191de3316`。
后续仅补录本节文档，另核对最终文档打包，不将其称为该候选原始字节。

| 本轮实际检查 | 结果 |
| --- | --- |
| macOS arm64 Go 1.27.3：`GOFLAGS=-timeout=20m make quality-gate` | 通过，逐函数 100% |
| Linux arm64 容器 Go 1.27.3：原始 `make quality-gate` | 通过，逐函数 100% |
| Linux 十包 race | machine、verify、watchdog、drivers、cli、modules、diagnostic、state、decision、investigation 全部通过 |
| rc.2 插件与旧宿主 | 251/251 + 9/9，无跳过 |
| 根 npm 测试 | 142/142，无跳过 |
| 三组 Go → Node 公开适配器桥接 | DSH、Codex Pro、Claude Code Pro 全部通过，无跳过 |
| 真实引擎离线会话评估 | 36 trials、96 次 fixture 调用，全部 oracle 通过 |
| 同一六包候选安装 | macOS arm64 10/10，Linux arm64 10/10，无跳过 |
| 当前实际 DSH rc.2 SDK 无推理探测 | cwd、完全权限与正常收尾确认，没有发送 prompt |
| 源码与候选文档检查 | 本地链接、锚点、JSON/shell 围栏及六包版本／许可证一致性通过 |

Mac 原始门禁先触发 Go 默认整包 600 秒累计超时；触发时当前子用例仅运行 1 秒，
并非那个子用例卡住 10 分钟。保留首次失败，复跑只为整个测试包提供 20 分钟运行窗口；
业务验收、单项超时和逐函数 100% 门禁保持原值。另见 rc.2 验证记录中保留的首次
Codex 启动时序失败；同一截止的单独与完整复跑通过，不将原失败抹除。

上述 Go 门禁和 race 使用相同的 319 个 Go 源／测试文件；随后合入的 rc.2 manifests
及 Markdown 不改变这些已验证文件。可选 Claude 0.4.0 使用原已审阅 tarball，
SHA256 `125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`，未改
版本冒充发行。真实模型质量、JEV 在线收益与用户桌面安装仍不在本轮离线验收结论中。
