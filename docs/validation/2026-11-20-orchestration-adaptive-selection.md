# Antigravity 编排与 JEV 首轮选择：实施、审查与验收

日期：2026-11-20。对应 [PR #60](https://github.com/ckanner/flywheel/pull/60)，实施依据为
[订阅工作流方案 §13.2.1](../plans/2026-11-19-subscription-cli-workflows.md)。
按用户要求，先将中英文方案及 PR 正文更新到
`ba0b0b9c45dbd0d537c05b501eb493d7f6056c40`，再实现本次增补。
此前的[订阅 CLI 验收](2026-11-20-subscription-cli-workflows.md)保留原有源码和结果，
不将其通过情况当作本次新代码的证明。

## 实施范围

| 层次 | 本次行为 | 边界 |
| --- | --- | --- |
| 独立 AGY 插件 0.2.0 | 三个固定 effort Orchestrator、native/subagents/swarm、受控 `/swarm` 输入、DSH 命令/工具/PTC/面板/设置 | 官方本机 CLI，订阅登录，无 ACP；普通要求 1.5.7+，新编排要求 1.5.12+ |
| 原生观察 | 请求策略、真实子会话、观察阈值、CLI 完成和截断事实分别记录 | maxObservedAgents 不是原生并发硬限额；子成员状态、模型、整树用量和远程清理未知时保持 unknown |
| Flywheel 协作契约 | 2–8 个受审查子任务，写入范围互不重叠且属于 schedule.writes | Swarm 要求 Full、无图片和合法声明；route 名或文件数量不替代授权 |
| JEV | 单开关默认关闭；开启后首次 Execute 前选择一次，确认失败后可调整 | 限定完整候选，先筛资格，沿用预算、权限、冷启动、独立 Review 与审批 |
| 默认配置 | 七条 route 的初始绑定及候选表 | 原初始模型不变，quality-order=[]，max-switches=2；没有质量排序或实时额度推断 |
| 配置与 UI | 高级 JSON 可编辑 execution-policy；路线编辑同步独有 initial；显示首轮选择与有界协作观察 | 不覆盖其他路线共享候选，不扩大权限，不把请求状态显示为已发生协作 |
| 恢复 | 首轮 Snapshot v3、决策持久记录、共享 task 预算、跨 packet 未决屏障 | 不重复派发，不采用迟到选择，不重置计数；v1/v2 原 journal 可按原快照读取 |

原模型阶段分工、Quick/Full、Auto 深度选择、review-first/delegated 权限、方案精确版本审批、
Go 的验证/交付控制仍使用同一套流程。只读规划使用前台子代理并归集结果；调查分析与 JEV
本身走 evidence-only，不能通过提示词取得工具或派生协作权限。

### 允许的执行候选

| route | 初始与替代项 |
| --- | --- |
| default | Hydrogen high / Hydrogen medium / Orchestrator Neon |
| complex | Neon high / Orchestrator Neon / Neon high Swarm |
| frontend、multimodal | Opus medium / Opus high |
| backend | Sol high / Sol medium / Astra high |
| batch | Kimi high / Kimi max / Neon high Swarm |
| hard | Astra high / Carbon high / Orchestrator Carbon |

Orchestrator 的 native 固定档位使用空 thinking 并省略 CLI effort 参数；不能将 high/xhigh
补成其档位。Argon Orchestrator 可手选。图片候选仅保留实际提供图片传输的 Claude。
既有 execution-policy 在迁移时保留；此前没有策略的配置只在原本 full-access 的 route
加入候选，不自动扩大旧权限。完整参数见[配置参考](../zh-CN/configuration.md)、
[模型策略](../zh-CN/model-policy.md)和 [Antigravity](../zh-CN/antigravity.md)。

## 审查关注点与修正

采用分区实现、主审复核和作者之间的独立交叉审查。审查以源码、真实子进程 fixture、
实际 npm 包消费和原 journal 恢复为依据，不把 fake CLI 当作真实账号推理。

1. **原生协作的证据边界。** 显式 subagents/Swarm 必须实际观察到 child conversation；
   root SUCCESS、请求提示词或 Orchestrator 模型名不能充当协作证明。非空 finished_reason
   一律失败；JSON 解析必须保留该字段，不能被嵌入类型的自定义解析吞掉。
2. **元数据累计内存边界。** 单帧限制和成员数量限制不能替代累计字节限制。增加 1 MiB
   元数据预算，覆盖跨帧和 sparse update；越限保存可信 ID 的最小记录并失败，明确标记
   metadataTruncated。至多保留首个越界 ID，即仅失败/截断回执允许 1001，正常最多 1000。
3. **规划收束。** Antigravity native profile 不代表完整后台子树生命周期，规划提示继续
   要求本次调用内前台归集。独立 Review 不被原生 verifier 替换。
4. **DSH 续跑默认值。** 已复现原会话没有 native 配置，随后保存 Swarm 默认值会使旧会话
   无法恢复。续跑现在从验证过 owner 的历史记录恢复原始缺省；显式换绑定仍拒绝，且不派发。
5. **执行配置的真实生效值。** 只更新 routes baseline 会被 execution-policy.initial
   覆盖。编辑器现在展示真实 initial，并为图形路线修改建立独有候选；共享候选、权限和
   会话规则保持原值。非法或不完整编辑不能伪装成有效保存。
6. **任务级未决屏障。** 决策/feedback/replan 尚未收尾时，手动或并行进入另一 packet
   不能绕过原调用。原 packet 只读恢复后才能解除相应阻塞；确认失败不被永久挂起。
7. **遥测保留与展示。** 同行包含公共回执和原生摘要时，合法记录可能大于旧 2 MiB 上限；
   单行读取对齐既有 8 MiB 公共回执边界，保留 64 MiB 文件上限。UI 最多显示 20 个成员 ID，
   明确其余省略数量，不输出原工具正文，也不把 unknown 用量填成零。

## 验证范围与限制

Go 门禁包含 vet、gofmt 以及每个被测函数 100% 语句覆盖。Node 验证包括插件公开 API、
实际 DSH schema/PTC、React 面板、命令默认值、取消、父进程退出、会话续跑、只读恢复、
worker 跨进程桥接、独立 npm 包安装和完整 Flywheel 包安装。

本次使用 Linux 和离线官方协议 fixture。未在用户 Mac、真实 Google/Claude/Codex/Kimi
账号上发起推理，未测真实模型相对质量、延迟、额度或账单。没有修改账号、用户 profile、
全局配置、registry、release tag 或 main。发布和真实用户安装不能由这份源码验收代替。

构建期间遇到 `/home/agent` 缓存卷空间不足；后续验证改用工作目录内的 Go/npm 缓存。
原代码和用户配置未被清理，该次未完成的安装没有计为通过。
