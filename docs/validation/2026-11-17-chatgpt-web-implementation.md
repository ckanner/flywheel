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

最终兼容补丁 SHA-256 为 `9718aaf2b8087dc953507347ef018f8fe6a8bbb181c7fa60aaadf32517c978b9`，334,255 bytes；应用后 tree 为 `333e1019f2333068ba4f6157705bf2d6d4b676cf`。在独立 Git index 上完成了精确基线的 apply check 和实际应用核对。

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

## 6. 与完整方案仍有距离的地方

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
