# 四家订阅、Antigravity CLI、紧凑准备与连续调查：实施及复审

日期：2026-11-20。实施依据为[已批准方案](../plans/2026-11-19-subscription-cli-workflows.md)，
在主分支 `f016015a884351d081e066a4031a6814299f25e5` 与原方案提交
`5f6d38a4792b4af360e272477f1cd2e325165a7d` 之上继续完成
[Flywheel PR #60](https://github.com/ckanner/flywheel/pull/60)。Claude 配套改动独立提交至
[PR #12](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/12)。

本文记录源码行为、独立审查、已执行检查及其边界。统一 Flywheel 候选的确切 source
commit、archive integrity 与安装结果另随 PR 记录；源码版本仍为 0.37.3 的未发行变更，
不表示修改了 registry 中既有版本。没有合并、打 tag、发布 npm、调整账号设置或迁移真实用户配置。

## 实施范围

| 方案阶段 | 已实现的源码边界 | 依据 |
|---|---|---|
| P1 / P2 | 独立 `@ckanner/dsh-subagent-antigravity` npm 包、官方 `agy` runner、machine/lifecycle/recovery、DSH 直接任务、工具/PTC/jobs 与 client 设置 | [Antigravity 指南](../zh-CN/antigravity.md)、[独立插件](https://github.com/ckanner/dsh-subagent-antigravity/blob/main/README.zh-CN.md) |
| P3 | 官方 Claude Agent SDK → 指定的官方本机 CLI；同进程订阅预检、Fable/Opus 模型绑定与无自动付费后备 | [Claude 指南](../zh-CN/claude-code-pro.md)、Claude PR #12 |
| P4 | 四家订阅默认表、AGY driver/worker、严格可达配置、compact 联合准备与前审、精确批准、路由、迁移、UI、真实图片传输 | [模型策略](../zh-CN/model-policy.md)、[审查规划](../zh-CN/reviewed-planning.md)、[图片输入](../zh-CN/image-inputs.md) |
| P5 | 可配置调查分析、同任务多轮补证、原范围与累计预算、默认关闭的订阅 JEV | [调查](../zh-CN/investigation.md)、[JEV](../zh-CN/jev.md) |
| P6 | 源码/候选/CI 分层验证流程已接入；真实账号、模型比较、正式发布与用户显式迁移单独验收 | [发行流程](../zh-CN/releasing.md) |

新分发模板的 13 个 profiles、18 个可达 loop/route/compact 绑定来自同一张默认表。
Gemini Hydrogen/Neon/Carbon、Claude Opus、Codex Sol/Astra、Kimi 均已进入实际配置。
Fable 是显式可选 profile。`subscription-only` 只允许四个已实现原生准入的 harness，
不能把旧 DSH ChatGPT provider、DeepSeek 或未实现原生验证的路径当作隐式后备。
未知账号资格、余额或模型使用费保持 unknown；本地配置通过不保证厂商账单。

流程保留 Auto 作为 Quick/Full 选择策略；compact/system 决定准备粒度，
review-first/delegated 决定是否需用户精确批准。这三个维度独立。Task 的
review-first 在受审查方案后等待批准；已明确授权的 delegated 任务按批准范围继续。
调查报告、纯规划及修复的交付停止点保持各自语义，Continue 不能将旧调查改成修复。

## 相对提案的明确调整

1. **JEV 使用 Hydrogen medium，仍默认关闭。** 提案的初始选择为 Sol medium。
   本轮只有 Antigravity 路径完成了由原生初始化证明工具集为空的 evidence-only 准入，
   因此默认 subscription profile 使用 `hydrogen-local`。这是可验证执行能力的选择，
   不表示已证明 Hydrogen 的决策质量优于 Sol；非受支持的 JEV harness 会明确拒绝。
2. **独立 AGY 包的源码维护在 `dsh-subagent-antigravity`。** 包名、machine、CLI、DSH
   client 与独立安装入口保持独立 npm 插件结构。候选内的 repository 元数据指向真实
   Flywheel 仓库及该子目录；没有依赖一个尚未创建的单独 GitHub 仓库来完成安装。
3. **图片以冻结 packet 契约接入。** `input-images` 声明真实仓库文件、SHA-256 与
   MIME；最多四张、每张 4 MiB、合计 8 MiB。完整 Claude 请求帧最多 16 MiB，prompt
   独立保持 1 MiB。图片实际发送给 Execute；默认独立 Review 收到冻结引用和验收证据，
   不声称看过原图，必要视觉证据不足时返回 verification-gap。

## 独立审查发现及修复

审查分工覆盖配置/订阅、CLI/SDK/生命周期、工作流/调查/状态、图片/恢复及安装包。
实现者的单测之外，其他审查者对关键路径重新阅读源码、协议、实际安装库与行为断言。

| 发现 | 修复与回归证据 |
|---|---|
| 图片最初只有 route 与底层 driver 参数，缺少任务端生产链 | packet/schema → Go Execute → worker → 公开 companion → 真实 Agent SDK 已贯通；只有收到原生 PNG block 的 fake CLI 才生成产物与 done-report |
| 4 MiB Base64 触发 companion 嵌套正则栈溢出 | 改为平坦字符校验及规范化重编码核对；4 MiB 输入与 8 MiB 合计跨 SDK 回归通过 |
| 图片恢复身份可能脱离实际发送字节 | decoded-byte manifest 同时进入派发前 journal、回执及冻结调用；缺失/错序/摘要变化拒绝，恢复不接受新图片或 prompt |
| AGY 未完成本地生命周期时可能留下不完整 cleanup 证据 | 保持 dispatch/terminal/cleanup 独立事实，真实 unknown 不能被当作成功；cancel、父退出、原生 pending 与恢复回归覆盖 |
| npm launcher 没有给 Go 传入安装后的 AGY worker | 补齐运行时路径、环境 override 与空值语义；源码及最终安装入口检查实际 worker |
| 严格配置编辑器仍显示未接通原生订阅准入的 provider | 严格资源集合限定为四个实际 harness，旧显式兼容配置仍保留其入口 |
| 新调查轮可扩大目标或改变只调查意图 | Continue 及持久化历史校验同时固定 goal、intent 和 implementation-scope；拒绝时旧状态不变 |
| 跨轮来源字段位置与实际 state schema 不一致 | 修正 preparation-source.round；真实两轮分析 → planning prepare → Save/Load 回归通过 |
| 原生 JSON 缩进变化会使合法归档看似被改写 | 使用规范化 PreparationHash 判断内容身份，保留旧轮不可变约束 |
| 原调查和 JEV 的未知清理可能被后续流程绕过 | 先恢复原 invocation；Run、Finish、Cancel、Continue 与历史归档均保留 unknown 阻断及累计预算 |
| 归档调查缺失 report 时 UI 可能崩溃 | 结构校验拒绝畸形新快照、保留旧视图并禁用不安全变更操作 |
| 新默认迁移会遗漏只读分析权限或旧 HTTP JEV | 只在缺失时加 investigate 只读绑定，保留显式权限；关闭的旧 HTTP JEV 可迁移，已开启的路径先给 blocker |
| 安装测试仍假定只有两个、且都无依赖的 bundle | 严格核验三个顶层 bundle 及 AGY/AJV 完整八包依赖图、实际 tar 内容和 source lock；隔离测试宿主的凭据/路由覆盖 |
| 16 MiB 请求沿用 3 秒 fixture 时限，在 race 检测下超时 | 仅该测试改用 30 秒有界预算，保留帧边界、拒绝、清理和结果断言；普通测试、专项 race 与 drivers 整包 race 复验通过 |
| 真实候选安装中的 Codex 完成后收尾被代理报告为异常退出 | stop 幂等、收尾后停止派发、只忽略已开始收尾时预期的管道关闭错误；普通终止信号在输出排空后交给公开 adapter 判断，真实非零与异常信号仍失败；55 项专项及实际 Go/Node 桥接通过 |
| Codex 流在完整 turn 后追加半截 JSON 或 UTF-8 时可能遗漏 EOF 校验 | 两个输入流均使用 fatal decoder 完成 EOF 刷新；原生和父输入的坏尾明确失败，保留完成前提前关闭输入的失败断言 |
| npm 12 的 pack JSON 改为以包名为键的对象，AGY 验收仍只接受数组 | 严格兼容 npm 11 单元素数组和 npm 12 单包对象，绑定准确 name/version/filename，拒绝多包和路径逃逸；实际 npm 11/12 pack/install 均通过且归档字节不变 |
| macOS 临时目录别名使三个新测试混用 `/var` 与 `/private/var` | 仅对测试原始任务根和调用 cwd 做物理路径规范化；Linux 的 symlink TMPDIR 原样复现并关闭三个失败，错误目录/坏 checkpoint 拒绝断言及生产身份规则不变 |
| 主动复查发现 AGY 跨进程、独立插件和完整安装也沿用未规范化的有效临时根 | 同一 symlink TMPDIR 复现 Go bridge、独立包 journal 和实际安装失败；在测试创建根目录后、派生 HOME/恢复目录与冻结调用前规范化，三层完整复验通过，生产 physical-directory 防御保持原规则 |
| macOS 全部 Go 测试通过，但删当前目录的测试没有命中 `filepath.Abs` 错误分支，`canonical` 覆盖率为 75% | 按包内既有 I/O seam 风格注入路径解析失败，明确验证原错误及空根、已有合法 checkpoint 时仍不派发/不查询；正常和 symlink TMPDIR 下六项测试通过，五个函数全部 100%，保留原物理路径与错误处理规则 |
| macOS DSH 的 EPIPE 夹具在 payload 之前提前结束，旧夹具混用 Node stdin 与底层 fd 关闭 | 夹具先握手，由单一 raw read/close 所有者在首块读取结束后物理关闭 fd 0；实际复用 0 并验证清理后仍可写，原生 exit 0 仍不能掩盖代理 exit 1；55 项专项通过，生产代理不变，握手诊断补充退出码与原生记录 |

上述确定问题已修复。审查没有以降低覆盖率、删除有效防御、跳过 Claude optional peer
或把错误变成默认通过来解除门禁。

## 实测配套产物身份

| 项目 | Claude Code Pro | Antigravity |
|---|---|---|
| 包版本 | `0.7.0` 源码候选 | `0.1.0` 源码候选 |
| Source commit | `ae3af4d752e4a974d41c848301472f4bb5d46d0f` | 本地独立源码 `aa6f396058d0c83eb8bb45ca9fd4c2b7d92a9717`，完整源码纳入本 PR |
| Source tree | `501e99df8e626ef71568faefbafc974803e401bf` | `4c2f84f12df6b4fc8bf8ac9e45aad23c09fcdc32` |
| Archive SHA-256 | `03e8cdc04f1488a45e8ce52025f1cbd99ef9ca36e097d3868199663c5d94baf4` | `f798a4e13c32d885e45be46f27c05c800f7a86256d5055c42a35aaedd7d8cb0d` |
| Archive bytes | 179625 | 42092 |
| 关键 runtime 依赖 | Agent SDK `0.3.326` | AJV `8.17.1` |

SHA-512 与构建工具链见 [Claude 记录](../../plugins/dsh/vendor/claude-code-pro.json)及
[AGY 记录](../../plugins/dsh/vendor/antigravity.json)。两个源码 lock 绑定同一归档实际
integrity。AGY 的 npm 11/12 与临时目录别名验收补丁另记为 verification source
`4b7fda3cf2fecf2a413c2e303518c4f34f7eb749` / tree
`2399a6daa71f88a461aab279396d65ef29fc2440`；两个实际 pack/install 检查重现上述
归档字节，不改写原归档源码身份。独立 AGY checkout 的 36 个 tracked 文件与主仓内副本逐字节一致；Claude
安装后的五个新增/变动 machine library 文件也已与审查源码核对。

## 已执行验证

本地环境：Linux、Go 1.27.3、Node 24.22.3、npm 11.19.1。fixture 使用临时仓库、用户
目录和 registry；实际模型推理为零。独立 CLI 进程、真实 SDK、真实 Git 与真实 npm
安装仍会运行。

| 检查 | 结果与实测范围 |
|---|---|
| `make quality-gate` | vet、格式、全部 Go 测试通过；每个测量函数 statement coverage 均为 100% |
| Race 检查 | 核心生命周期七包检查通过；图片增量后 machine/drivers/state/contract/imageinput/prompts 再次通过，无数据竞争报告 |
| Flywheel `npm test` | 167 通过；运行时、安装器、发行与协议单测 |
| DSH `npm run check` | 收尾修复后 583 项测试及 10 项发布宿主兼容检查通过，0 失败、0 跳过；包含实际插件构建 |
| Claude companion `npm test` | 222 通过 |
| Claude 真正 pack/install consumer | 通过；公开 machine、官方 SDK、真实 PNG 内容、durable identity 和只读恢复 |
| Claude worker 专项 | 39 通过；16 MiB 帧边界、8 MiB 图片合计、同进程订阅预检及原有生命周期 |
| AGY 独立功能与候选消费 | 20 项回归通过；npm 11.19.1 与 npm 12.2.1 的真实 pack/install、ESM/types/wrapper/supervisor/journal/evidence-only 均通过，归档 SHA-256/SHA-512 不变 |
| AGY 真实 DSH 安装 | `0.2.2-rc.3` 的隔离 profile 安装通过；没有修改用户桌面配置 |
| `TestPacketImagesClaudeSDKOfflineBridge` | 正式 Execute + L1、真实 SDK 图片、原终态恢复、源图变化、无重派及 22 tokens 只归账一次均通过 |
| 全组 Go → Node 跨进程桥接 | DSH、Codex、Claude、Kimi、AGY 及 Provider 活性、Kimi print 标题/身份失败检查均通过 |
| Codex 收尾专项 | 55/55 通过，包含实际打包代理与公开 worker；7 MiB 背压/EOF/进程组 TERM 回归能使旧代理错误退出、新代理正确结束；Go → Node Codex 协议/活性/订阅策略再次通过 |
| Codex 物理输入关闭的可移植夹具 | 55/55 专项再次通过，无跳过；明确证明在途写、单次物理关闭、fd 0 复用仍可写、原生正常退出、代理失败及进程组清理，不用 stream close 事件推断物理关闭 |
| macOS 路径别名回归 | symlink TMPDIR 下三个原失败及两项负向检查通过；受影响 decisionexec/drivers 两包完整测试通过，逐函数覆盖率均为 100% |
| AGY 跨层路径别名回归 | 同一 symlink TMPDIR 下 Go bridge 8 子例及 strict subscription 2 子例、独立插件 20 项和实际 pack/install、真实 DSH 隔离安装、主候选宿主 10/10 与安装 16/16 均通过；修改只涉及测试/验收脚本的有效目录 setup |
| 路径解析失败的可移植覆盖 | `decisionexec` 六项测试在正常与别名环境通过；准确验证路径失败阻止派发和恢复查询，全部函数含 `canonical` 均为 100% |
| 嵌入技能 | `internal/skillsembed` 通过，frontmatter 与实际技能契约保持一致 |
| 文档静态复核 | 61 个变动/新增 Markdown、823 个相对链接及适用锚点、91 个完整 JSON 围栏通过；中英图片/配置/工作流指南同步 |
| 36 条真实引擎会话演练 | 36/36 完成，96 次离线调用；首轮、L1 返修、Review 返修、长上下文、停止恢复和配置变化全覆盖 |
| Claude PR CI | [run 44733802702](https://github.com/ckanner/dsh-subagent-claude-code-pro/actions/runs/44733802702) 四个 Linux/macOS × Node 22/24 job 全通过，包含公开包消费和真实 DSH schemas/PTC bindings |

CI 将 DSH、独立插件、原生桥接与实际候选安装放在较慢的完整 Go 门禁之前，以便先
反馈已发现的平台兼容问题。所有原命令、job、超时、逐函数覆盖率门禁、Linux race
检查和真实引擎会话演练保持完整；只移动 `make quality-gate` 的执行顺序。

## 仍需单独认定的证据

- 本地与 CI 的 fake CLI 不证明真实 Mac 桌面交互、账号登录、具体模型目录、Fable
  计费资格、套餐降档后权益或长期额度。CLI 的订阅预检是可观察事实的快照，不是账号设置锁。
- 图片传输与验收证据不证明模型视觉准确性。完整模型分工没有做真实任务 A/B，
  36 条会话演练也不是 Gemini、Claude、Codex、Kimi 的能力排名或省额度实验。
- 主包与配套候选不是 npm 正式发行。Claude optional peer 正式发布前仍须按发行门禁
  验证 registry 的精确归档；独立 AGY 已随统一候选内置，并保留独立 DSH 安装入口。
- 真实配置只通过用户明确选择的 `config migrate --preset subscription-cli` 迁移。
  本次实现及测试没有运行针对用户配置的迁移，也没有修改原生客户端或账号计费设置。
