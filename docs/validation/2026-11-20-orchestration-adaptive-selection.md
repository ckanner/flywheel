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
8. **真实跨进程活动桥接。** 完整 Go→Node 验收复现了 child activity 的公开 native 快照
   被通用严格活动协议拒绝，导致 subagents／Swarm 被中止，并把观察越限分类改为 TRANSPORT。
   AGY 专用 bridge 现在只从活动流投影中移除该已公开扩展，完整 native 仍由最终回执核验；
   其他未知字段、乱序和畸形活动继续拒绝。未改写原事件、失败回执或 unknown/cleanup 语义。
   修复后 AGY 的 14 个真实跨进程子场景和 3 个 Node 桥接测试均通过。
9. **文档与时序一致。** 普通 Antigravity（含 Orchestrator）在 spawn 后发送任务，之后
   核对 init；只有 evidence-only、显式 subagents／Swarm 等待 init 后派发。中英文指南
   已删除“所有调用均有派发前 init 闸门”的泛化表述，并明确 AGY Plan 不运行 Swarm。
10. **干净 CI 的公开包依赖。** 首次完整 CI 在 Linux/macOS 均暴露桥接测试直接导入
    相邻 AGY 源码，因 DSH 步骤尚未安装兄弟包的 ajv 而失败。测试改为导入 DSH 锁定的
    `@ckanner/dsh-subagent-antigravity/machine` 公开入口，保留全部三个测试及断言。
    仅安装 DSH 依赖、root/独立 AGY 均无 node_modules 的隔离目录中，旧导入复现
    `ERR_MODULE_NOT_FOUND`，改后 3/3 通过；没有靠调整 CI 顺序或跳过检查解决。

### 独立插件的冻结身份

- 源码提交：[`56b34df60ba5`](https://github.com/ckanner/dsh-subagent-antigravity/commit/56b34df60ba566b97a6d81bfe7b5a056f3fc2faa)。
- 源码树：`cfe0b7fd9863daa63ac8ac5b112419b31466e6f9`。
- 归档：`ckanner-dsh-subagent-antigravity-0.2.0.tgz`，59,868 bytes。
- SHA-256：`ac4309f38208fa7e60502a5c8581e36f17ec46dd1dd8852da3ca6870eae8681d`。
- npm integrity：`sha512-3w51MLSraMgMwxfW45+93ZFGhFuLQ7Etw8SMSldE40Ac1IAmxT0q/2DA1KaMpoNuUF7UWrXf/AiBoTgEiDnZMw==`。

冻结源码的插件测试 35/35 通过。该源码生成的上述归档随后用于源码目录外的
ESM/types/bin/guardian/journal/evidence-only/Orchestrator/Swarm 消费，
以及真实 DSH 0.2.2-rc.3 隔离 profile 安装；源码测试与归档消费分别核验。
Flywheel 桥接修复没有改动该插件或重打此包。两个消费树的 lockfile 均记录相同 integrity，
实际 `npm ci` 后确认安装的是 0.2.0；仅看 lockfile 或 npm 的“up to date”不作为版本证明。

## 本次实际验证结果

以下本地结果对应完整生产实现提交
[`48086f91d14f`](https://github.com/ckanner/flywheel/commit/48086f91d14fbc74d406832d0a1febe987f7632f)。
最后的活动桥接修复仅影响 Node 桥接；Go 代码与已通过完整 Go 门禁的实现相同，
桥接修复后重新运行完整跨进程组合，并从该干净提交构建和安装完整候选。
本地工具链为 Go 1.27.3、Node 24.22.3、npm 11.19.1。

| 验证 | 命令或入口 | 实际结果 |
| --- | --- | --- |
| Go 完整门禁 | `make quality-gate` | vet、格式、全部包测试通过；1,123 个被测函数各自 100.0% 语句覆盖 |
| 独立 AGY 插件 | 独立仓库 `ckanner/dsh-subagent-antigravity` 的 `npm run check` | 35/35；随后真实归档消费通过 |
| 独立 DSH 安装 | 插件的真实 DSH 0.2.2-rc.3 隔离 profile 安装 | 通过，使用上述相同 0.2.0 归档 |
| 完整 DSH | `npm --prefix plugins/dsh run check` | 主测试 590/590；发布宿主/客户端 10/10；0 skipped |
| 根 Node | `npm test` | 167/167；0 skipped |
| Go→Node Provider 桥接 | `FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers` 的 AGENTS.md 指定组合 | 9 项主测试组合全部通过，含 AGY 的 14 个子场景；101.750 秒、exit 0 |
| 实际 Claude SDK 图片与恢复 | `TestPacketImagesClaudeSDKOfflineBridge` | 通过，使用公开 SDK 和离线 CLI fixture |
| 原生构建与会话演练 | `make build`；`FLYWHEEL_EVAL_BINARY="$PWD/flywheel" npm run test:sessions` | 36/36 trials、96 次调用；六类场景的 oracle 全通过，0 unknown / unobserved |
| 四平台完整候选 | `npm run build:npm` | 干净源码隔离构建成功，四个平台引擎包含在同一包中 |
| 同一完整候选的真实安装 | `FLYWHEEL_NPM_PACKAGES_DIR=dist-npm FLYWHEEL_CLAUDE_CODE_PRO_TARBALL=… npm run test:install` | 发布宿主 10/10；安装 16/16；全部 0 skipped，含已审 Claude 0.7.0 归档 |

安装检查第一次未提供 Claude 可选归档时为 15 passed / 1 skipped；之后针对同一完整候选，
显式提供已审归档并取得 16 passed / 0 skipped。表中使用后一次完整结果。
CI 的 Provider 桥接组合另包含 Kimi print identity failure 测试，不与本地 9 项计数混用。

### 完整 Flywheel 本地候选身份

| 项目 | 值 |
| --- | --- |
| Source commit | `48086f91d14fbc74d406832d0a1febe987f7632f` |
| Source tree | `27d780aa5ace521ab8405f56bd04e54131fff7b6`，clean |
| Source snapshot SHA-256 | `638fdef309bb3f5b0369db91faee7ac338a08cca46a43128fb6017fea1e504a0` |
| Archive | `ckanner-flywheel-0.37.3.tgz`，21,687,212 bytes |
| Archive SHA-256 | `26d9b949a682b06ef9da4a3ef73c46a2813841dd6dc8eb3e2bbcc8c4fd3cdb3c` |
| Archive integrity | `sha512-TnWbgQnAFBeqIeA+YwqYm7flDeBdWj7/5zl0udRfUsadgZ/WHQgQ4ZkPyr3mYGXm6GI1zm64Bq3LE+AqWzXgog==` |

0.37.3 是此次源码版本的未发行候选，不表示已经更新 registry 中的同名版本。
CI 使用固定 Node 24.22.1 / npm 12.2.1 构建另一份候选，Linux 和 macOS 消费该份
相同归档；其身份及通过状态以对应提交的 workflow 为准，不与本地归档混用。

### CI 与后续审查证据

提交 `48086f91d14f` 的 [run 44778917310](https://github.com/ckanner/flywheel/actions/runs/44778917310)
完整候选构建成功，两 OS 的 DSH 测试因上述公开包导入问题失败，该 run 不计为通过。
随后的修正只涉及这项测试和本验收文档；生产代码、Go 代码、插件归档均未改变。
最新提交的完整 CI 结果与 run 链接统一记录在 [PR #60](https://github.com/ckanner/flywheel/pull/60)，
必须按 PR 的 head SHA 核对，不引用本次增补前的旧绿灯。

## 验证范围与限制

Go 门禁包含 vet、gofmt 以及每个被测函数 100% 语句覆盖。Node 验证包括插件公开 API、
实际 DSH schema/PTC、React 面板、命令默认值、取消、父进程退出、会话续跑、只读恢复、
worker 跨进程桥接、独立 npm 包安装和完整 Flywheel 包安装。

本次使用 Linux 和离线官方协议 fixture。未在用户 Mac、真实 Google/Claude/Codex/Kimi
账号上发起推理，未测真实模型相对质量、延迟、额度或账单。没有修改账号、用户 profile、
全局配置、registry、release tag 或 main。发布和真实用户安装不能由这份源码验收代替。

构建期间遇到 `/home/agent` 缓存卷空间不足；后续验证改用工作目录内的 Go/npm 缓存。
原代码和用户配置未被清理，该次未完成的安装没有计为通过。
