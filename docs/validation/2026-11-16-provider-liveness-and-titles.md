# Provider 活性、终止原因与可读会话名修复验证（2026-11-16）

[验证索引](README.md) · [原始审查 F1–F8](../reviews/2026-11-16-provider-liveness.md) · [会话恢复指南](../zh-CN/session-recovery.md) · [PR #53](https://github.com/ckanner/flywheel/pull/53)

## 范围与证据身份

这是原始跨 Provider 审查之后的实现验证，不替换原审查的历史结论。修改范围是
Flywheel、Codex Pro 与 Claude Code Pro。ChatGPT 0.1.20、LCU 0.5.0 和 preset
generator 0.4.0 没有发现必须修改源码的同类缺陷；DSH 公共流桥接修复适用于实际
由该 AgentLoop 产生的 DeepSeek、ChatGPT 和本地子代理活动，不能据此宣称对每个
模型或旧兼容 CLI 都做过真实账号验收。

- Flywheel 实现基于 PR #53 的 `9afb14bf2a554a55a67d14648962716524949c45`，并保留
  并发远端提交 `ae9b256324641c2a8e306910c7ae2a2a0e5942a9` 的 macOS CI 时限调整。
- Codex Pro：[PR #8](https://github.com/ckanner/dsh-subagent-codex-pro/pull/8)，
  候选源码 `911868c0b3836c79c9b61ddb51cd8c4aed0043f9`，tree
  `1ea5a546b386c678da947a6960b7950dfa6ff005`。随后测试专用提交
  `a15a2950e264a1332eed9f86b88aaf2e298cca30` 规范化 macOS 临时目录；真实符号链接
  TMPDIR 下 11/11 activity 测试通过，重新 pack 与原候选逐字节相同。
- Claude Code Pro：[PR #8](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/8)，
  候选源码 `ab9d52b322bd53cc10f45bb3cb03735ed170ef91`，tree
  `a3a7f97d7faf2c8695838eecb89b788c5d00c982`。
- 验证环境：Linux、Go 1.27.4、Node 24.22.1、npm 11.9.0。使用真实 Go driver、
  构建后的 Node worker、实际安装的公开插件 API、supervisor/guardian 及公开
  DSH SDK；只有原生模型／CLI／HTTP/WS 对端由离线 fixture 提供。

### 精确 companion 候选

| 包 | SHA-256 | 发布状态 |
| --- | --- | --- |
| Codex Pro 0.10.1 | `d573280d359271b38c8233ee7f8bd420df68e440caa8ac44ce7a6ad615083d8c` | 源码候选，未在本次操作发布 |
| Claude Code Pro 0.5.2 | `27682aff67ad8ac600a9d8fea85f4645930b623944d781b81873cc1b7b385cd6` | 源码候选，未在本次操作发布 |

实际 npm integrity、源码来源和打包工具链见
[Codex 记录](../../plugins/dsh/vendor/codex-pro.json)、
[Claude 记录](../../plugins/dsh/vendor/claude-code-pro.json) 与
[vendor 说明](../../plugins/dsh/vendor/README.md)。没有只改 package version 冒充
兼容；实际 tarball 已重新安装验证。Codex 随 Flywheel 完整包内置；Claude 仍为
optional peer，Flywheel 正式发行前的既有 gate 仍要求 registry 中的精确 Claude
版本与已审查字节一致。源码验证不构成 npm 发行证明。

## 发现与修复的对应关系

| 原发现 | 实现 | 关键验证 |
| --- | --- | --- |
| F1：DSH transient stream 没有到达 Go | 公开 `agent/assistant-stream` + 可选 `flywheelProviderActivity` host 服务；核对真实 Agent registry、根／后代归属、每次 invocation；冷启动与预热同协议 | 真实 SDK root streaming、Agent Team/PTC child、foreign/stale 拒绝；冷／热 Go 桥接 |
| F2：Codex root/child delta 丢失 | 独立公开 activity v1；真实事件触发、全树统一合并；默认原生 children 不依赖显式 native profile | root reasoning、child-only reasoning、默认 native tools，以及真实 Go idle 对照 |
| F3：Claude 只有最终主文本可见 | 同一 wall 内预检 CLI partial/subagent-forwarding 能力；观察 thinking、tool、partial、retry、child | root tool、root thinking、无显式 native 的 child thinking；能力缺失时 prompt 前拒绝 |
| F4：同一 native running 状态永久去重 | activity 与 durable native ledger 分离 | 连续 task progress 在状态不变时仍可观察；ledger 不产生重复完成证据 |
| F5：累计协议流量误当结果上限 | 插件区分单帧、保留答案／图片、展示尾部与累计流量；worker 展示满额后继续；Go 严格压缩 advisory 帧 | Codex >32 MiB delta、Claude 约69 MiB thinking、两 Pro >17 MiB外层activity、DSH冷／热>16 MiB活动、Kimi 100000事件；最终答案和终态保留 |
| F6：DSH timeout 擦除身份/清理 | 保留独立可信的 sessionId、dispatch、cleanup；outcome 继续 unknown | fresh persistent 和 warm path 真实 TERM 回执；损坏流/未知物理退出不能伪造清理 |
| F7：清理 cancel 遮蔽原始原因 | idle/wall/provider timeout 与外层 context cancel 独立；protocol/ACK 错误不继承不可信派生 cancellation | 四后端 timeout/取消、Pro worker/ACK错误、DSH/Kimi坏尾/写失败；原始 receipt 保留 |
| F8：旧 Codex exec 也不持续暴露进度 | 与 Kimi print 一样仅使用有限 wall + 明确取消，不按 stdout 静默杀死 | 静默超过 idle 后正常完成；wall 和取消仍生效 |

### 本轮实现中继续发现并修正的边界

1. Claude 的累计 64 MiB 查询流上限也会杀掉健康运行，已改成单帧和实际保留数据
   的上限。约 69 MiB 的合法 thinking 流完成；17 MiB 单帧仍正确失败并清理。
2. Claude foreign root ID 的 partial/tool 帧必须在转发活动之前拒绝，不能先刷新
   活性或把它计入当前会话。合法子流仍按 native 归属准入。
3. Codex 未显式启用 native profile 时依然可能产生原生子线程，活动跟踪不能被
   配置开关短路；也不能为其虚构已请求的 native policy 回执。
4. 补了 worker activity 后，Go 原累计 16 MiB stdout 仍可能装满。新的 capture
   只省略逐帧严格验证的 advisory 活动，保留最新摘要及所有非 advisory 帧。
   原始 `OnEvent` 继续实时接收全部事件，终态、native evidence 和 durable ACK
   不被压缩。坏帧、重复序号、foreign、post-terminal 和 EOF 半帧会保留失败标记，
   不能被过滤过程洗成合法结果。
5. 新增 Kimi print 命名准备独立允许最长4096字符的绝对工作目录，真实大于512字符
   路径已通过；不把路径复用成512字符的模型/ID校验。内部1ms deadline即使叠加
   清理未知或抛错，仍为 TIMEOUT/error，独立保留 cleanupConfirmed:false；外部取消
   继续保持 cancelled。三个失效条件均先复现失败再验证修复。
6. cleanup 为响应上游 timeout 或协议故障而执行 cancellation，不代表用户点击
   取消。原始失败原因、取消机制、业务 outcome 和物理 cleanup 分开记录。

## Session 名称

### 根因与实际路径

Kimi 的交互式命名能力不意味着 print 会执行相同路径。复核上游
[`fec742878d73678082b304c887e4fb1564088c54`](https://github.com/MoonshotAI/kimi-code/tree/fec742878d73678082b304c887e4fb1564088c54)：

- [`run-v2-print.ts`](https://github.com/MoonshotAI/kimi-code/blob/fec742878d73678082b304c887e4fb1564088c54/apps/kimi-code/src/cli/v2/run-v2-print.ts)
  直接提交 loop turn，绕过交互入口的首条 prompt metadata／自动标题流程；CLI 没有对应 title flag。
- [公开 session profile route](https://github.com/MoonshotAI/kimi-code/blob/fec742878d73678082b304c887e4fb1564088c54/packages/kap-server/src/routes/sessionProfile.ts)
  改标题也会先 resume；[SDK renameSession](https://github.com/MoonshotAI/kimi-code/blob/fec742878d73678082b304c887e4fb1564088c54/packages/node-sdk/src/sdk-rpc-client-v2.ts)
  对 cold history 同样打开／改名／关闭。因此不存在已经验证的无生命周期公共 rename 捷径。

Flywheel 从 spec.title、规划 brief.goal 或 slug 取得名称来源，结合 role 和适用
scope，生成如 `Flywheel · design-plan · 排查告警重复发送` 的标题。移除控制字符、
折叠空白、按 Unicode 字符截断，并保证总计不超过 512 UTF-8 字节；不把内部随机
invocation ID 或整份 prompt 放进会话列表，不额外调用命名模型。

| 路径 | 新建持久会话 | 已有会话 resume |
| --- | --- | --- |
| DSH | 公开 title 服务设置并回读验证，按宿主字节上限规范化 | 保留既有／用户修改名称 |
| Codex Pro | 公共 thread 命名接口；独立插件无显式名时取 workspace 可读回退 | 不覆盖名称 |
| Claude Code Pro | 受支持 CLI naming；query/interactive team 一致；回退取 workspace，避免泄露 prompt 首行 | 不覆盖名称 |
| Kimi Agent/Swarm | 在已有原生创建运行时传 title 并验证 | 不重新命名 |
| Kimi print | 公开本地 API 创建具名空历史、确认关闭后以同 ID 运行原 print | 跳过准备、不重新命名 |

Kimi print 准备严格拒绝 prompt，核对 API schema、requestId、title、cwd、owner
metadata 和 idle 状态。确认拥有的准备运行时物理退出后才派发业务 prompt；两步
共享一个 wall 预算和相同 KIMI_CODE_HOME。只设置原 fresh print 的 model、auto
权限和 plan_mode:false，不显式注入 thinking/profile/environment；原 effort 环境
在两端一致。缺能力、错配或清理未知时失败，不静默转成匿名执行。

**明确成本：**公开 create/close 多产生一轮原生启动／关闭，配置的
`SessionStart` / `SessionEnd` hooks 也会多执行一轮。依据是上游
[session lifecycle](https://github.com/MoonshotAI/kimi-code/blob/fec742878d73678082b304c887e4fb1564088c54/packages/agent-core-v2/src/workspace/sessionLifecycle/sessionLifecycleService.ts)
与 [external hooks](https://github.com/MoonshotAI/kimi-code/blob/fec742878d73678082b304c887e4fb1564088c54/packages/agent-core-v2/src/features/externalHooks/session/sessionExternalHooksService.ts)。
业务 prompt 仍只发送一次；没有额外标题推理。Agent/Swarm 不增加这轮准备。

旧 `codex exec`、pi、omp、ZCode 未被添加未经验证的原生命名接口；已经存在的无意义
历史名称也没有被批量覆盖。opaque session ID 仍是恢复和身份标识，可读名称只是
展示元数据。

## 已执行验证

| 层 | 结果 |
| --- | --- |
| Codex standalone | 243 tests：238 pass、5 live/account skip；typecheck、实际 tarball 安装后的 API/CLI/supervisor/recovery smoke 通过 |
| Claude standalone | 153/153 tests；实际 npm pack/install 与 8 个 schema/PTC host 组合通过 |
| Flywheel 插件完整检查 | 469/469 Node 行为测试 + 10/10 rc5/legacy host 兼容测试通过；后续 Kimi 边界修改单独复验 27/27（含9个标题测试） |
| Flywheel 根 npm 检查 | 166/166 tests 通过 |
| Go 完整门禁 | 最终代码 `make quality-gate` 通过：vet、fmt、全包测试及每函数100%（含 Kimi 结构化 TIMEOUT 最后回归） |
| 36 trial 真实引擎评估 | 36/36 质量通过，六类场景各6条、96次 adapter 调用，未提前停止；40个新持久历史在prompt前命名，10次resume不改名 |
| 发行/evaluator补充 | 23/23相关npm单测、fixture拒绝路径和workflow YAML解析通过；CI保留远端30分钟时限，publish仍核验官方Claude归档 |
| 最终真实 Go → Node 桥接 | 扩展后的8个顶层 bridge 测试全部通过（105.336s），覆盖四后端原协议、三组新增activity及Kimi print命名 |
| 完整候选安装 | 实现提交之后以干净源码隔离构建、安装；实际身份与安装结果在后续验收补记，不将尚未执行的步骤计为通过 |

各修复均有针对失效模式的回归，不只是断言 mock 被调用。跨层测试以短 idle/wall
放大同一竞态边界；测试时长不能当作模型性能或最佳 timeout 测量。临时环境中的
真实 account/live tests 明确跳过，不记作成功。

## 仍然成立的限制

- activity 说明真实观测到进展，不保证模型最终能完成或答案正确。队列等待、首个
  delta 前的思考、静默远程工具等仍可能耗尽可观察传输的 idle 策略预算。
- Kimi print 与旧 Codex exec 保留有限 wall、明确取消和进程组清理；没有以假心跳
  冒充工作，也没有整体关闭 timeout。
- unknown dispatch/outcome 或 cleanup 未确认仍禁止自动 fresh/retry/replay。只有
  已有的明确恢复／人工退役规则可以解除阻塞，改变超时或名称不会修复旧未知调用。
- 所有离线结果与 registry publication、真实模型/账号、桌面 UI、用户机器及原始
  告警仓库的验收分开。本次没有合并 PR、发布版本或修改用户 profile。
