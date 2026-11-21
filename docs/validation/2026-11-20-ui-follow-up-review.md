# Flywheel UI 后续复审：已选配置、协作观察与无效草稿

[验收索引](README.md) · [上一轮 UI 验收](2026-11-20-ui-effective-configuration.md) · [PR #60](https://github.com/ckanner/flywheel/pull/60)

## 范围与基线

本次续接用户对 PR #60 的实现与 review 请求，以
`540c3e79c583608b624a3a7b4c12873cf9afd6b0` 为审查基线。该提交已实现方案
§14.3 的有效初始模型、非 Execute Swarm 拒绝、JEV 实际配置来源、可读决策、
任务入口、已审分工及图片预览；其历史 CI 和候选身份仍只证明原提交。

本轮按引擎、设置与工作台分工检查，并由不同作者交叉复核新改动。模型默认值、
Provider 选择、权限、JEV 唯一开关、图片传输通道及伴随插件归档未改变。
源版本保持未发行的 `0.37.3`；最新提交、实际候选和新 CI 的身份另记录在 PR。

2026-11-21 事实更正：官方 npm registry 已存在另一份 `0.37.3`。这里的“未发行”
指本轮当时的源码与离线候选，不表示该版本号尚未占用。历史证据保留；最终候选使用
`0.38.0`，见[补齐验收](2026-11-21-complete-controls.md)。

## 发现与修正

| 发现 | 修正 | 可观察回归 |
| --- | --- | --- |
| 切换记录只保留模型与 native profile 名称，下一次执行缺少原生选项 | 新策略创建和真实切换时冻结 `execution-policy.currentBinding`，复制完整选项 | 确定性及 JEV 切换到 Swarm，在实际派发前即可保存、读取并展示准确绑定 |
| 旧策略缺快照时，候选 ID 相同即可套用后来运行的 initial | 有已保存策略时只使用保留的当前快照、切换记录或采用决策证据；旧记录不回填 | 后来运行复用同名候选、改变模型或协作方式时，不重写旧选择；实际执行仍被原身份校验阻断 |
| 配置卡未直接说明 native／subagents／Swarm | 从冻结 options 显示请求方式，缺失、未知和省略时保持未知 | 同名 profile 在初始、已选、实际调用中保留各自方式；不会生成虚构子成员 |
| 原生观察尚在读取或读取失败，却显示已经查询但无记录 | 只在成功返回 metrics 后标记查询完成 | pending、failed、成功为空分别验证；失败信息保留 |
| 高级 JSON 中 native profile 为 null 时，图形模型页抛出 TypeError | 非对象定义阻止保存，渲染跳过无效项，保留原草稿供修正 | JSON → 模型页 → JSON → 撤销全过程不崩溃，未发任何保存请求 |

复审还将模式标签限制为明确的三个值；`toString`、`__proto__`、未知值及非对象
options 不会被 JavaScript 对象的继承属性误当作已知模式。

## 恢复、展示预算与历史兼容

`currentBinding` 仅用于展示，不授权派发。它只在新策略创建或实际选择变化时保存；
已有状态缺少该字段时不从今天的配置补造历史。配置身份、预算、权限与未决调用
检查保持原路径。同名候选重新配置也不能绕过原策略身份。

旧版 v2 repair action 哈希排除新增显示字段，原 Snapshot v1/v2/v3 协议不变。
本轮额外执行持久化 v2 journal 与 v3 首轮选择恢复回归，验证不重发原请求；
没有把未单独重跑的 v1 场景称为本轮专项验收。

新快照与原初始／JEV 显示映射共用 64 KiB 控制响应预算。原生选项正文超过既有
1 KiB 阈值时保留摘要和省略标志；完整身份放不下时整体省略并返回
`currentBindingOmitted:true`，不截断成另一模型。state 与单独的 node.packet
也共用同一预算；node 的 packet/policy 先复制，原磁盘状态和完整 node evidence
不被改写。客户端不能把整体省略用后来 initial 或不完整历史补齐。

## 独立 review 与验证

设置作者审查引擎及工作台；工作台作者审查设置及引擎；引擎作者审查模式显示。
根审查发现并关闭旧策略的同名候选回退问题。最终交叉审查未发现新增阻断项。

新增回归曾针对基线复现设置页 TypeError、模式缺失及过早的空查询说明。
新协议字段验证的旧包失败仅证明旧包不认识新显示快照，不另算一个原有生产缺陷。

本地环境：Linux、Go 1.27.3、Node 24.22.3、npm 11.19.1，依赖沿用锁定版本。

| 检查 | 本轮实际结果 |
| --- | --- |
| `make quality-gate` | vet、格式及全部 Go 包通过，1,136 个被测函数逐个 100% statement coverage |
| 设置、控制台与工作台相关专项 | 90/90 通过，0 skipped |
| 最终工作台、决策与任务入口专项 | 37/37 通过，0 skipped |
| 另一作者独立工作台／决策复核 | 25/25 通过，0 skipped |
| Go 当前绑定、投影及 v2/v3 恢复专项 | 通过；最后一次字段名计量修正后重跑当前绑定专项 |
| 根 `npm test` | 167/167 通过，0 skipped |
| 完整 DSH 插件及最低支持宿主 | 635/635 + rc.5 宿主 10/10 通过，0 skipped |
| Go → Node 公开 Provider 桥接 | AGENTS.md 的 DSH、Codex、Claude、Kimi、Antigravity 及 activity/title 组合通过 |
| Claude SDK 原图与只读恢复桥接 | execute、recover-original、recover-changed-source 全部通过 |
| 文档与差异 | 7 份变更 Markdown 的相对文件链接、围栏及 `git diff --check` 通过 |

完整包的构建、同一归档实际安装与新提交 CI 结果在 PR 中单独记录，不能用上述
源码检查或上一轮候选替代。伴随 Claude 0.7.0 的源码仍为 `ae3af4d752e4`，归档
SHA-256 仍为 `03e8cdc04f1488a45e8ce52025f1cbd99ef9ca36e097d3868199663c5d94baf4`；
Antigravity 0.2.0 归档也保持既有身份。

### CI 总时限复核

源码提交 `7259d0d8b12c` 的 [CI #294](https://github.com/ckanner/flywheel/actions/runs/44848747129)
中，候选构建和 Linux 全部通过；macOS 的实际候选安装、Provider/图片桥接、Go
逐函数 100% 门禁及本机 CLI 检查也通过。macOS 随后的配对会话演练完成 21/36
个试验，已完成试验的独立 oracle 全部通过，随后整项任务达到 30 分钟总时限而
取消，不能把这一轮 macOS 整体验收记作通过。

为让完整检查有时间结束，CI 的 test job 总时限与已有发布验证对齐为 45 分钟。
检查命令、平台矩阵、Go 每包 15 分钟超时、独立试验保护、覆盖率门槛与实际候选
安装要求保持原样；不改产品代码。后续提交和完整新 CI 结果继续记录在 PR 中。

## 验证边界

本轮重新尝试运行实际构建客户端的 Chromium/Playwright 视觉检查；Chromium 在
启动时因 `socket() failed: Operation not permitted` 退出，尚未生成截图。未完成
实际浏览器窄屏几何、键盘焦点及用户 Mac 桌面验收，不用 DOM 测试代替这些项目。

没有调用真实账号模型，也没有验证套餐用量或模型质量。本次不合并 main、不打 tag、
不发布 npm、不改用户本机插件或配置。没有图片的普通 Gemini／合格 Swarm 任务
仍可使用；必须看原图的 packet 继续受现有已验证传输能力约束。
