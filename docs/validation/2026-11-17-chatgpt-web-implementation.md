# ChatGPT Web Pro：实现候选、交叉复审与验证边界

简体中文 · [English](2026-11-17-chatgpt-web-implementation.en.md) · [方案](../plans/2026-11-17-chatgpt-web-provider.md) · [验证索引](README.md)

**状态：代码实现候选，尚未合并或发布；真实网页与 macOS 验收未完成。** 本记录对应 PR #57 的第二版方案及其插件配套实现，区分已实现机制、离线回归与仍未交付的能力。不能将本记录解释为 T01–T36 全部通过。

## 1. 实现范围和源码身份

| 组件 | 本次交付 | 基线与版本 |
| --- | --- | --- |
| ChatGPT DSH 插件 | 原生 Web 模型、浏览器传输、工具协议、会话协调、恢复、设置卡 | `ckanner/dsh-plugin-chatgpt`，基于 `40d8f885e7a386af5f441b356b9c68496a47b0e3`；候选 `0.2.0-alpha.1` |
| DSH 宿主 | 公开重放否决、压缩策略、持久 ACK、主响应/摘要恢复 | 官方源码 `ca33fd6843b205e279c7ccb6cb73a472c78f5760`，`0.2.2-rc.4` 加明确维护的兼容补丁 |
| Flywheel | DSH 精确 effort、SDK 准入、稳定调用身份、活动、预算、状态账本与配置 UI | 基于 `db6b93e6b2f74b8bcc6563c54bcafbd0813ccd0c`；实现追加在 PR #57 |
| Oracle | 行为和测试设计参考 | 固定 `0a064e7b604df933f84e64f0b5a69f1b178dee78`；没有运行时依赖、私有 HTTP 或源码快照导入 |

插件实现和使用说明位于 [feat/chatgpt-web-pro](https://github.com/ckanner/dsh-plugin-chatgpt/tree/feat/chatgpt-web-pro)。[兼容 manifest](https://github.com/ckanner/dsh-plugin-chatgpt/blob/feat/chatgpt-web-pro/compat/manifest.json) 记录补丁 SHA-256、精确上游基线、应用后 tree 和验证范围。它不是已发布的官方 SDK；安装插件不会自动给原版 SDK 或桌面端打补丁。

最初实现快照的兼容补丁 SHA-256 为 `9718aaf2b8087dc953507347ef018f8fe6a8bbb181c7fa60aaadf32517c978b9`，334,255 bytes；应用后 tree 为 `333e1019f2333068ba4f6157705bf2d6d4b676cf`。当时在独立 Git index 上完成了精确基线的 apply check 和实际应用核对。这些历史身份不包含 §8 的 `summaryRoute:2` 接续修复；当前补丁身份以配套插件的兼容 manifest 为准。

## 2. 用户获得的模型能力

现有 `chatgpt` Provider 新增精确模型 `gpt-6-pro-web`，显示名 `GPT-6 Pro · Web`，原生 effort 为 `pro`。自定义 Provider route 仍受支持。Web 模型按精确 ID 分流；原有 SIWC 模型继续使用原来的账号、转换、图像、上下文和重试实现。

Web 默认关闭。发现模型、读取设置和静态诊断不启动浏览器，不发送 prompt。用户在现有 Provider 卡片配置专用 Chrome、手动登录并绑定账号；实际发送前再次核验 GPT-6、Pro、账号、项目和聊天身份。显式 profile 配置优先，Connect 不会覆盖它。

DSH 仍负责工具权限、执行、工作目录和权威历史。插件只在整份响应通过 JSON、调用身份、工具表、schema、大小、深度和调用数校验后生成工具块。短网页 call ID 与持久 DSH tool ID 的映射随下一轮结果明确传递，不能依赖模型猜测。

同一逻辑会话按真实前缀追加增量。前缀编辑、压缩、工具/指令绑定变化和本地历史/回合阈值触发新代次；新代次重新提交权威上下文。主调用与摘要、标题等辅助调用使用各自的网页聊天。标题在本地从任务生成，完整历史和网页链接保留。

唯一的 DSH CompactionEngine 执行自动和手动压缩。公开 pressure policy 修正默认 headroom 与 Web 本地预算不相容的问题；摘要在构造调用前选择路由，并记录实际解析后的模型和 Pro。不存在 Go 内的第二套摘要器。

## 3. 恢复和并发的关键不变量

| 边界 | 实现行为 |
| --- | --- |
| 发送之前 | 完整准备记录先持久化，再发布宿主索引和 conversation pending；进入传输前另写 dispatch-intent。网页项目创建与 prompt 提交各有独立 WAL 意图。 |
| 结果交付 | 完整原文 artifact、digest 和已验证 block 持久化后结束 stream；宿主在自己的提交及 flush 后，通过独立回调 ACK。 |
| 连接容量 | 每个实际 Chrome profile 并发 1。正常兄弟调用可取消地排队；完整捕获后仍保留容量，直到宿主 ACK。stream 不等待自己的 ACK。 |
| 不确定失败 | 保留 dispatch、outcome、remote generation、local cleanup 和 usage 各自事实；未知结果不能借 fresh、换 Provider、换聊天或 always retry 重放。 |
| 活跃与未知 | 当前进程的健康请求可排队；idle 同步撤销已结束请求的活跃例外。进程重启后不能凭进程外日志猜测其仍在正常执行。 |
| 域继承 | 主/辅助/子 Agent、非 Web 子 Provider 和嵌套工具共享执行域；workspace 的物理路径持久关联外部域，路径别名与 fresh session 不能绕过。 |
| 受管身份 | Flywheel 明确创建的 invocation 不因主回答结束而换 ID、预算或 isolation；整个调用冻结连接、账号和策略。 |
| 本地清理 | 关闭 CDP 或原 owner 进程确实退出可证明本地资源释放；点击 Stop 不能证明云端已经停止。 |
| 恢复采用 | 主响应和摘要进入不同的宿主维护事务。读取真实 JSONL 历史、校验原消息/surface/header/route/工具及持久身份，重复采用不重复工具执行或摘要替换。 |
| 用量 | 网页没有可确认的 token/cost 时保持 unknown/null；不会构造零值、估算值或重置外层预算。 |

普通 main 最终回答成功、Agent 进入 idle、标题生成完成是不同事实。空 invocation 的事实是 `not-dispatched / failed / idle`，不会因为 idle 就变成业务成功。恢复也不能用一个已捕获回答掩盖同域另一个未知工具结果。

## 4. 交叉复审中已经修复的问题

本轮使用独立 reviewer 和实际生产类的最小复现，修复了以下问题；它们不是待办清单：

- 原生配置 schema 在没有 Web 设置时自动合成半个对象，破坏旧 SIWC 模型目录；用真实 `ctx.plugin` 默认组合回归验证修复。
- DSH 默认 65,536 headroom 与 Web 的保守本地窗口不匹配，导致永远不能压缩；公开模型压力策略覆盖默认配置。
- 默认 Pro 在 pre-projection 之后解析，原始调用仍没有 effort；只合入实际准入的 effort，保留原始媒体和历史检查。
- 主回答完成后辅助调用逃离原 hostInvocationId、domain、isolation 和 wall；区分宿主受管调用与原生调用轮转。
- 受管调用中途切换账号/策略、以及已绑定 profile 的 Connect 验证成功却尝试不可写保存；冻结调用绑定并分开验证与保存。
- 健康兄弟调用被当成恢复故障、捕获后过早释放容量、失败 ACK 等 idle 才阻断；补齐排队、ACK 容量屏障和立即停止域。
- 非 Web 子 Agent 没继承根域、嵌套工具 unknown 被根结果覆盖、canonical workspace 取消遗漏别名；统一继承和持久工具事实。
- 准备记录不可发现、capture 到 delivery 之间崩溃、capture 后 cleanup 前崩溃、并发锁回收 ABA 和 Chrome 重复启动窗口；增加阶段证据及真实子进程故障回归。
- 单个 JSON 存储上限与合法输入/回答预算矛盾；分层限制原请求、响应与完整可恢复记录，发送前预检可保存空间。
- 摘要恢复不能只凭相同文本认领已有 checkpoint；恢复身份必须包含原请求/响应与 opaque replay state。

## 5. 验证层次与结果

| 验证 | 结果和证据范围 |
| --- | --- |
| Flywheel `go build ./...` | 通过 |
| Flywheel `make quality-gate` | 通过；总覆盖率与每个函数均为 100% |
| Flywheel DSH Node check | 主套件 553/553，legacy 10/10，0 skipped |
| Go → Node worker → 公开发布 SDK | 原有 Provider 与 Web 消费契约回归通过；Web/browser 对端为明确标识的离线 fixture |
| ChatGPT 插件 TypeScript、原有与新增 Node tests | 319 tests：318 passed，1 skipped，0 failed；跳过项是受环境限制的真实 Chromium fixture |
| 插件真实构建与安装 | build、10 个 client Remote 方法、182 文件 pack、逐行双语检查通过；精确 tarball 与真实 Cordis 4.1.0 / Schemastery 3.19.0 / DSH rc.3 peers 安装及导入通过，证明旧 SIWC peer 可加载，不代表原版 rc.3 支持 Web |
| DSH 兼容宿主 | 首轮 19 文件 / 512 项通过；最终摘要身份补正再通过 18 项相关测试（含 8 个新增拒绝例）和 6 项跨层。相关源码/测试 TypeScript、lint、公共 API 与双语生成检查通过 |
| 真实宿主跨层 | 6 项通过：只替换浏览器页面，DSH、插件、JSONL、工具、恢复和 Flywheel 消费模块均为实际源码 |
| 完整上游宿主构建 | 本环境未完成：tsx IPC 被拒绝，完整 aggregate TypeScript 超过可用内存；相关项目编译不能代替完整发行构建 |
| 用户 macOS / 实际 ChatGPT 账号 | 未执行；不属于本轮通过项 |

真实宿主脚本为插件的 `scripts/verify-web-harness.mjs`，接收 DSH checkout 和可选 Flywheel checkout。六个场景是：默认插件组合；main→持久 ACK→工具→final；完整捕获后中断→采纳→工具仅一次；默认自动/手动压缩；always 硬否决；实际 `createWebPolicy` 准入及销毁后的持久终态回执。

## 6. 2026-11-17 原始验收快照

下表保留最初实现快照；后文按日期记录接续变更。表中的原缺口不代表当前图片上传
与格式修正仍未实现。

| 尚未验收或交付 | 当前行为 / 后续条件 |
| --- | --- |
| T01/T05/T25/T28/T36 的真实网页和 macOS 组合 | DOM fixture、选择和命名逻辑已实现；真实账号、最新网页布局、Projects 内 Pro 与可回访历史必须实测。当前 Chromium 在 socket 创建阶段返回 `EPERM`。 |
| 图片、截图和 LCU 视觉链路 | 原始附件在投影前明确拒绝；没有上传能力声明。普通授权文本工具仍可用于代码执行。 |
| T17 网页明确满窗拒绝后的自动迁移 | 本地预算与前缀迁移已实现；服务端拒绝若没有可靠“未生成/无副作用”证据，仍阻断，不自动重发。 |
| §6.1 的有界格式修正及 retire/discard | 尚未实现。非法完整响应保留 artifact 和容量；恢复不会让非法 JSON 自动变得可采用。 |
| T26 项目创建歧义的自动只读定位 | 创建意图被保留，无法确认时需人工定位；不会连续创建替代项目。 |
| T30 完整重活 Preset、真实 PTC/Team/LCU 与 hosted apps | 原生工具、子域和嵌套结果有契约回归；不等于所有实际工具/外部应用的真实验收。 |
| 安装与发行 | 已交付精确源码兼容补丁和候选包构建路径；未发布 patched SDK、桌面应用或 npm 正式包。 |

因此两个 PR 保持 Draft。后续以真实环境补齐上述证据，并逐项更新方案验收矩阵，不能仅因离线测试绿色就宣布完整能力已经发布。

## 7. 2026-11-18 接续：图片、格式修正与 Flywheel 对账

配套 ChatGPT 插件现已实现有界持久图片上传，保留原 user/tool 位置、bytes/digest、
已接受文件名和私有恢复 artifact。格式修正是已捕获回答的有界继续调用，保留不可变
parent/child 身份，不允许改变工具参数或任务语义。显式丢弃绑定已检查的拒绝回答
digest，独立记录 failed retirement，不伪造 host commit。这些源码路径已有离线
回归证据。上传接受仍标记 `modelConsumption: unverified`；实际模型读图、账号限额
和真实 LCU 视觉链路需要独立验收。

Flywheel 在 prompt 前保存 `chatgpt-web/read-only-recovery/1` 锚点，并持久记录原
SDK prompt/wait/terminal/shutdown 证据。对账在原 owner 退出后只读取插件及 worker
证据，核对原 binding/domain/configuration、candidate/role guard，不启动 SDK、
浏览器、模型或工具。[模型策略](../zh-CN/model-policy.md#对账中断的-web-调用)与
[worker 协议](../../plugins/dsh/PROTOCOL.md#原调用只读恢复协议)逐项定义合法结果。
SDK 中断后才 adopt 纯 final、采用工具/摘要，或原 candidate 检查点缺失，仍不能
凭捕获回答变为任务成功；显式 retire 仅在原 SDK 关闭与 guard 证据满足时结算失败。

本次 Flywheel 源码通过 `make quality-gate`，所有受测 Go 函数覆盖率 100%，DSH
Node check（含 legacy host 加载）和启用的跨进程 worker bridge 测试。
新增测试覆盖原完整终态、未派发/已知失败、retire、未完成 adoption、身份和 guard
变化、不可变响应及修正链，并在最终回答来自修正 tail 时仍验证原图片接受证据。
插件 helper 测试在独立已退出进程运行真实 host/transport，只替换浏览器页面。

后续审查关闭了三个实际集成缺口：原 candidate receipt 在主 receipt 被替换前，
先用独立摘要获得 state CAS 确认；retire 失败保留原 SDK 实际观察到的 turn ID；
Go 查询关闭 stdin，并透传原先非凭证 SDK 选择。测试覆盖三个 checkpoint/CAS
中断窗口，以及真实 Go parser 读取 Node completed、retired、未发送三种恢复结果。
最终 Go 门禁所有受测函数 100%。发行模块后续修改前，DSH 源码测试 560 项加
10 项 legacy-host 场景通过；之后独立 reader 与恢复定向测试 9 项、发行校验
45 项通过。

发行补充将候选公开的只读 reader 与 schema 验证器构建为 Flywheel 独立模块，
限定外部导入为 Node filesystem、crypto、path、os。精确源码/输入/输出清单和
所有原许可证同包交付，干净 consumer 不依赖另一个 SDK profile 的 provider
搜索路径。最终根目录发行门禁必须对稳定源码运行：

```sh
npm test
npm run build:npm
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm \
  FLYWHEEL_CLAUDE_CODE_PRO_TARBALL="$PWD/plugins/dsh/vendor/dsh-subagent-claude-code-pro-0.6.0.tgz" \
  npm run test:install
```

这些门禁执行后的精确候选、安装包恢复结果与 CI 证据记录在 PR 中；本文不预先
宣称尚未执行的发行门禁通过，也不为追加结果而重新生成不同字节的候选。

上述打包验证当时采用的 ChatGPT 候选源码为
[`191a2cc11ded693970e0c77a8fd351dfa3aec9cf`](https://github.com/ckanner/dsh-plugin-chatgpt/commit/191a2cc11ded693970e0c77a8fd351dfa3aec9cf)，
tree `eff36fdf3756ac95856420e518abf50328f9aefc`。候选
`dsh-plugin-chatgpt-0.2.0-alpha.1.tgz` 含 199 文件、735161 bytes，SHA-256 为
`e53a5ee4d8c6d5300f896ca680f362a453fcf1385e5eec227eeaa29b395b59d7`；
此 archive 身份保留为历史证据；当前选择的候选、完整 integrity 与构建工具链见
[vendor/chatgpt.json](../../plugins/dsh/vendor/chatgpt.json)。
[CI run 44245907991](https://github.com/ckanner/dsh-plugin-chatgpt/actions/runs/44245907991)
的 Node 22/24 与 Ubuntu/macOS 精确宿主矩阵全过。每个 OS 覆盖 391 项宿主测试、
8 个真实跨层场景和 6 个真实 Chromium fixture 场景，包含媒体。macOS profile
清理修复只改测试文件，重打包逐字节一致。

这些是源码、浏览器 fixture 和真实 peer 安装验证，不代表发布 patched SDK/npm
正式版本、把 provider 装入用户 SDK profile 或验证真实 ChatGPT 账号。真实账号/
当前页面、完整 heavy Preset 任务与有歧义的项目创建定位仍是独立门槛。

## 8. 2026-11-18 接续复审：浏览器归属与压缩契约

本轮复审检查 Flywheel 消费的配套插件及宿主契约。此前快照和测试结果保留，
不能用其证明更新后的候选已经通过验证。

| 发现的问题 | 更新后配套候选要求的行为 |
| --- | --- |
| 新建聊天完成准备后，输入页可能在上传或提交前跳转至其他聊天 | 在实际操作边界再次验证已准入的新聊天 URL；其他会话或新聊天页面不能接收原请求的图片和 prompt。 |
| 恢复可能把已停止、带错误提示的页面当作完整回答 | 恢复与正常轮询应用相同的生成错误检查；仅有部分文本且生成不再运行，不构成可采用的完整响应。 |
| Web 压缩策略可能覆盖用户显式配置的其他摘要 provider/model | 单独传递显式 `configuredTarget`、`proposedTarget` 与历史默认值，显式选择优先。仅选中的 Web 摘要使用 Web Pro 策略，其他显式路由保留自己的 effort 与计量。 |
| 手动压缩丢失真实发起 Agent；插件状态目录与 workspace 不同时，已捕获但中断的摘要缺少恢复元数据，并错误阻断状态目录，真实 workspace 内的 fresh 非 Web Agent 仍可继续 | 宿主维护操作全程保留真实 Agent 上下文，摘要恢复与执行域阻断绑定原 Session/workspace，并覆盖 fresh 和非 Web Agent。原摘要仅通过显式、幂等的宿主事务采用，插件不能按 sessionId 推测 Agent。 |

第三项修复要求兼容 DSH 宿主及插件准入使用 `summaryRoute:2`。
`replayVeto`、`durableResponseAck`、`durableSummaryAck`、`pressurePolicy` 仍为 1；
Web extension v1、`chatgptWebHost` v1、binding 的 `hostContractVersion:1`
及 `chatgpt-web/read-only-recovery/1` 不变。旧摘要路由能力在派发前拒绝；
只安装更新后的插件，不会自动更新宿主。

配套变更前，本轮已重新构建 Flywheel 提交
`0804c467c7f29f436e01bba27398ade0e15cfa56`，18 项定向 Node Web policy/recovery/bundle
测试全部通过，0 skip。`internal/drivers` 和 `internal/machine` 的 Web Go 测试在
`FLYWHEEL_TEST_NODE_BRIDGE=1` 下通过，包含实际 Node worker 及四类恢复结果。
该复审未复现新的 Flywheel 执行或恢复缺陷。

更新后的配套 archive 固定至插件源码提交
[`783c8ce53287be47840c9b28eb70041584834e74`](https://github.com/ckanner/dsh-plugin-chatgpt/commit/783c8ce53287be47840c9b28eb70041584834e74)，
tree `0e02c3aed787b585209587e7e819b924d1f43de3`。单次构建的
`dsh-plugin-chatgpt-0.2.0-alpha.1.tgz` 含 203 文件、755337 bytes，使用
Node 24.22.1 / npm 12.2.1，SHA-256 为
`3a1bf051514a4649492e9909a6a1286ed30253c32248956988f8b3f109ecfb60`。
完整 integrity 记录在 [vendor/chatgpt.json](../../plugins/dsh/vendor/chatgpt.json)
及源码 lock。内嵌兼容 DSH 补丁仍基于官方提交
`ca33fd6843b205e279c7ccb6cb73a472c78f5760`（`0.2.2-rc.4`）；补丁
354986 bytes，SHA-256 为
`d6f629762a2a7a7a3792eda1748f3942a222f9ebeeac99c3edec999ecc283fbb`，
产出 tree `4086a378e17bc09b7fbfb19999e76be180664d94`。

插件 [CI run 44293903906](https://github.com/ckanner/dsh-plugin-chatgpt/actions/runs/44293903906)
在该精确源码提交上的四项 job 均通过。Node 22/24 各报告 400 tests：399 pass、
1 skip。Linux/macOS 兼容宿主 job 各通过 395 项宿主测试、14 个真实跨层场景及
全部 7 个 Chromium fixture 场景，浏览器场景 0 skip；两项 job 均在测试前后验证
精确 patched host tree。单次构建的 archive 已逐文件核对，并使用 Cordis 4.1.0、
Schemastery 3.19.0、DSH `0.2.2-rc.3` peer 完成隔离 consumer 安装。该安装证明
包可加载；原版 peer 的版本号本身不能证明必需 Web 宿主能力存在。

更新此精确 vendor pin 后，Flywheel 源码检查在 Node 24.22.1、npm 12.2.1、
Go 1.27.4 下通过：

| 检查 | 结果 |
| --- | --- |
| 根目录及私有 DSH 的全新 `npm ci`；精确安装插件内容 | 通过；安装后的全部 203 个插件文件与固定 archive 一致。重新构建的独立恢复 manifest 记录同一插件 commit、tree 及 archive SHA-256。 |
| 根目录 `npm test` | 167 pass，0 fail，0 skip。 |
| `npm --prefix plugins/dsh run check` | 构建、全部 563 项主测试及 10 项旧宿主检查通过，0 skip。 |
| 下方 Go 到 Node 桥接矩阵 | 11 项顶层测试通过，含嵌套场景共 118 条 PASS 记录，0 fail，0 skip。 |

Go 结果由文档规定的 10 项选择及 CI 额外选择的 Kimi 身份测试合并；后者单独
运行，没有重复第一组选中的测试：

```sh
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test((DSH|CodexPro|ClaudeCodePro|KimiNative)NodeWorkerOfflineBridge|(DSH|CodexPro|ClaudeCodePro)ProviderActivityOfflineBridge|KimiPrintSessionTitleOfflineBridge|DSHWebNodeWorkerOfflineBridge|DSHWebRecoveryNodeBridge)$' -count=1 -v
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^TestKimiPrintIdentityFailureIsNotRetryableOfflineBridge$' -count=1 -v
```

`TestDSHWebRecoveryNodeBridge` 的 `completed`、`retired`、`unsent`、
`browser-unsent` 均通过，分别保留 SDK 派发与浏览器派发事实，并证明恢复没有
新增模型请求。`TestDSHWebNodeWorkerOfflineBridge` 实际执行 cold 与 prewarmed
两类 Node worker 调用，保留 invocation、execution-domain 身份及 unknown
usage/cost。

最终 Flywheel 候选及精确安装结果在构建已推送的干净源码后记录到 PR，不为追加
证据而重打不同字节的包。用户真实 macOS Desktop、当前 ChatGPT 网页及已登录
账号仍须独立验收。

## 9. 2026-11-18 接续复审：发送前身份、当前工作区与恢复目录

此前各节保留当时的源码身份和验证结果。本轮复现并修复三类问题：

| 问题 | 复现和修复 |
| --- | --- |
| P1：同 URL 下的发送身份变化 | `prepare` 后账号、模型、Pro 档位、活动生成或 hosted controls 改变，旧纯文本路径仍输入并发送；图片路径可能先上传再发现账号变化。插件在任何上传或输入前统一复核，图片上传后再次复核，不静默切回选择或重发。生产 page 的 DOM 负对照捕获旧缺陷。 |
| P1：执行域遗漏当前 worktree | 旧集成读取初始 root header，根或已继承 child 的后续工具也跳过新目录登记。真实 DSH 工作目录切换、子 Agent 独立目录及非 Web 工具均复现缺陷。现在使用当前 Agent 的公开目录服务，保持父域及所有旧目录关联，重启后的 fresh 非 Web 调用继续被拦截。 |
| P2：恢复 worker 启动目录不一致 | Web、Codex Pro、Claude Code Pro 正常派发使用原 `cwd`，恢复却使用引擎启动目录。相对 worker 和依赖 cwd 的绝对 wrapper 共六个真实子进程场景在旧代码失败。两个只读恢复入口现在均沿用原物理目录，六场景通过。 |

工作区修复保留稳定绑定的早返回，让集成层先对账已持久的工具结果，再做正常
guard。独立正负对照确认错误顺序的 flush/后续派发均为 0，修正后均为 1；新增
回归也验证未对账前 guard 仍拒绝。无目录服务的旧宿主保留相对路径 fallback；
服务存在但失败时不退回旧 header。协议、宿主能力版本、环境过滤、原候选与角色
校验及 unknown 禁重放规则没有变化。

### 精确插件候选与本地验证

插件提交 `5d2325abf0da9d7897cd273ed30d463dba0cc204`，tree 为
`b780ceb8cf4501e0aed47270cba140b6e29624cb`。单次打包的 `0.2.0-alpha.1`
候选为 **203 files / 756836 bytes**，SHA-256 为
`f49fb116f45d3be2c4e8749b53fa246a4bb94c25f6bfbdba0006e5e28b556f03`。
vendor、archive、lock 和独立 reader 来源已同步，全部 203 个安装文件逐字节一致。
DSH 兼容补丁保持 §8 的身份，没有重新生成。

使用 Node 24.22.1、npm 12.2.1、Go 1.27.4：

| 检查 | 本轮证据 |
| --- | --- |
| 插件完整测试 | 410 项：409 pass、1 skip、0 fail；skip 为独立 Chrome fixture。 |
| 插件构建和安装 | TypeScript、build、11 Remote 方法、双语 README 通过；同一 tarball 内容校验及真实 peers 隔离安装通过。 |
| 真实 patched DSH + Flywheel policy | 17 个跨层场景通过，包含 root/child 当前目录；仅网页是 fixture。 |
| Flywheel DSH | 更新精确 vendor 后的 build、563/563 主测试及 10/10 旧宿主测试通过，0 skip。 |
| Flywheel Go | 最终 `make quality-gate` 全通过：vet、fmt、全部测试及每函数 100% statement coverage；六个新增恢复目录场景通过。 |
| 根 npm | 最终串行完整运行 167/167，0 skip。首次并行运行一项 timeout cleanup 日志断言失败，未计为通过；原用例单独运行通过。受控延迟 doctor 启动可复现缺日志，同时进程组已退出，不能仅凭该断言推断清理泄漏。 |

本地真实 Chromium 在用例开始前因 runner 的 Unix socket/ptrace 权限退出；不把
这次启动失败记为通过，也不绕过限制。当前提交的双 OS CI、Flywheel DSH 门禁、
完整候选和同一包安装结果记录在 PR。兼容 SDK/Desktop 发行物及真实账号验收仍
未完成；开发候选没有发布。
