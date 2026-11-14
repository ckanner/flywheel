# 原生 Harness、PTC 与 LCU：六仓实现验证记录

[验证索引](README.md) · [中文设计](../plans/2026-11-13-native-harness-capabilities.md) · [English design](../plans/2026-11-13-native-harness-capabilities.en.md) · [原生能力使用指南](../zh-CN/native-capabilities.md)

日期：2026-11-14。对应 Flywheel 0.34.0 开发候选与下表六个 PR。本记录区分源码测试、真实公开包集成、已安装候选、最终完整发行包及用户机器验收，不声明已合并、已发布 npm 或已完成真实模型／桌面验收。

**后续复核：** 下列首次完整集成的来源和结果保持原样。随后发现并修复了 Preset Generator 首次创建补丁失败的问题，更新了内嵌候选；ChatGPT 另补了一个登录通知测试时序修复。新来源、验证范围与既有结果的关系见下文“首次生成补丁的后续复核”。当前依赖身份以 [vendor 记录](../../plugins/dsh/vendor/README.md) 为准，最新完整包的 CI 以 [PR #44 checks](https://github.com/ckanner/flywheel/pull/44/checks) 的具体 head 为准。

**当前本地结论：Go 完整质量门禁通过，每个受测函数均为 100% statement coverage；最终五个配套包的四条 Go→Node 桥接及 36 个 session 场景通过。Tower 目录别名修正后，Node 主套件 395/395、legacy host 10/10、Kimi Go→Node 桥接和新四平台单包构建再次通过；这个新 tarball 的 Host/Client 10/10 与安装／迁移 15/15 均通过。** 跨平台 CI 的失败、修正及最终 head 结果分别记录，不能用本地成功代替 macOS 成功。 LCU 全套测试另有明确的 Linux socket 环境限制，见下文，不能概括为六仓全套测试全部通过。

## 1. 首次完整集成的六仓范围与配套源码

PR 链接定位本次交付，不代替精确提交的 CI 状态。Flywheel 实现内容与本地候选的精确对应见 §5；最终 PR head 的远端 CI 状态在 PR checks 与说明中记录。

| 仓库与 PR | 候选版本 | 固定源码 | 本次交付边界 |
| --- | --- | --- | --- |
| [flywheel #44](https://github.com/ckanner/flywheel/pull/44) | `0.34.0` | [实现内容 854c3580ed7e](https://github.com/ckanner/flywheel/commit/854c3580ed7ec0f740c7882f23baca11dfa10b6a)；候选身份见 §5 | 配置、Go 编排与状态、DSH/Kimi workers、Pro 桥接、独立 native jobs、资源与媒体证据、工作台、单包集成 |
| [dsh-preset-generator #1](https://github.com/ckanner/dsh-preset-generator/pull/1) | `0.4.0` | [0ec05fbeb37d59f7e3af4baa0f6d9bfb0c5573a3](https://github.com/ckanner/dsh-preset-generator/commit/0ec05fbeb37d59f7e3af4baa0f6d9bfb0c5573a3) | daily/heavy 共用的 SDK PTC composition、静态身份、行为准入契约 |
| [dsh-plugin-lcu #1](https://github.com/ckanner/dsh-plugin-lcu/pull/1) | `0.5.0` | [7986057e92314b78caf66dde43d1a8d45ab6ada7](https://github.com/ckanner/dsh-plugin-lcu/commit/7986057e92314b78caf66dde43d1a8d45ab6ada7) | 受管挂载、lease/Agent/epoch 归属、回执等待后的再次授权检查、媒体与物理收尾 |
| [dsh-subagent-codex-pro #6](https://github.com/ckanner/dsh-subagent-codex-pro/pull/6) | `0.10.0` | [c0045b06e6762b8dfc56f50b14cce7a53af2725b](https://github.com/ckanner/dsh-subagent-codex-pro/commit/c0045b06e6762b8dfc56f50b14cce7a53af2725b) | 原生控制与有效配置、早到子任务归属、独立用量增量、取消／恢复、独立 native review |
| [dsh-subagent-claude-code-pro #2](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/2) | `0.5.0` | [54d7eafa9102713597e3b96a6d70a9cbd86afc6b](https://github.com/ckanner/dsh-subagent-claude-code-pro/commit/54d7eafa9102713597e3b96a6d70a9cbd86afc6b) | Agent/Workflows/advisor、迟到 task/tool 关联、独立 ultrareview、隔离交互 Team 及 journal/control |
| [dsh-plugin-chatgpt #8](https://github.com/ckanner/dsh-plugin-chatgpt/pull/8) | `0.1.19` | [fa5cfafcc1cc6bfb1f640e646b6dc3c4329d976c](https://github.com/ckanner/dsh-plugin-chatgpt/commit/fa5cfafcc1cc6bfb1f640e646b6dc3c4329d976c) | attachment/image 投影、Responses 事件归并、取消与真实公开依赖安装；隔离 npm lifecycle 日志与 artifact JSON |

最终配套包用 Node `24.22.1`、独立安装的 npm `12.2.1` 执行正常 `npm pack`，没有改源码或绕过生命周期脚本。与此前 npm `11.9.0` 产出的相同候选相比，重打包字节摘要一致。下列 tree、SHA-256 和完整 integrity 可交叉核对 [vendor 记录](../../plugins/dsh/vendor/README.md)；所有记录的 `registryReleaseVerified` 均为 `false`。

| 配套包 | 源码 tree | tarball SHA-256 | 完整元数据 |
| --- | --- | --- | --- |
| Codex Pro `0.10.0` | `1c39704cf385e9f505f5d50e9d96ef2e0b88d5f5` | `4307f7bdc129dcffc03a8bc17d452c9bd3828801c12829b5f4514e6d23278416` | [codex-pro.json](../../plugins/dsh/vendor/codex-pro.json) |
| Claude Code Pro `0.5.0` | `bf992d3e33889857526af370bbb16d91585c0ecc` | `338675c8fbafc47e7f2c7cc5ffaa6987bda952c5c9c1010ac441230597ce420c` | [claude-code-pro.json](../../plugins/dsh/vendor/claude-code-pro.json) |
| LCU `0.5.0` | `6256c2a947efec8812ae5e4e7e83c4d098ad1ee1` | `e047b045546aafc3e394b975f66dc03f4d8202a9f923bdbfa34b3ebcb8e9d84a` | [lcu.json](../../plugins/dsh/vendor/lcu.json) |
| ChatGPT `0.1.19` | `168dc4e6284f002f09291667a926eab8a31b6687` | `838ef2f6cb6fa70f9a1a708729b1efb966bee4daeb4a4f2810ab3f69fe1a1179` | [chatgpt.json](../../plugins/dsh/vendor/chatgpt.json) |
| Preset Generator `0.4.0` | `c93bbc8453a95c2b4f06d432f1c7d0e67a56aaf0` | `bdeaf8ab4a0ef7d3eb59910072807e35361db456f1533425add835cc49f5b65d` | [preset-generator.json](../../plugins/dsh/vendor/preset-generator.json) |

这些是配套包身份，不是待构建的完整 `@ckanner/flywheel` tarball 身份。后者在 §5 单独记录，不能用配套包安装通过代替完整包验收。

## 2. 环境与证据种类

Go 验证环境为 Linux/amd64、Go `1.27.0`；Node 为 `24.22.1`。早期源码测试使用工作区 npm `11.9.0`，最终五个配套包使用上节记录的 npm `12.2.1` 重打包。测试没有进行真实模型账户推理、付费 ultrareview 云任务、真实 macOS GUI 操作或用户 profile 迁移。

| 证据类型 | 本次实际使用 | 可以证明的范围 |
| --- | --- | --- |
| 确定性 Go 与协议 fixture | 临时 Git 仓库、真实状态 Save→Load、可控驱动结果和故障注入 | 配置与角色边界、预算、回执、恢复、归属、取消及错误处理的机械规则 |
| 官方 DSH 公开 npm 包 | 实际安装的 `0.2.2-rc.3` Agent、Team、subagent spawn/fork/control、SDK JSON-RPC、PTC executor 等；真实 Cordis 服务和 dispatcher | 真实公开接口注册、调用、生命周期和回执组合；不是仅手写同名 API 的 mock |
| DSH 测试替身边界 | scripted/offline LLM adapter、可控 PTC runtime 执行边界、已知 PNG 与附件 fixture | 可重复地驱动上述真实代码；不证明用户桌面 runtime、账号模型或图片已送达真实模型 |
| Pro / Kimi 跨进程 fixture | 真正安装的公开 machine API、Go/Node workers；下游 Codex/Claude CLI 和 Kimi 本地 server 使用协议替身 | 实际 adapter 入口、进程和 journal 生命周期、消息关联、去重、异常与清理路径 |
| 本地 manifest doctor | 导入固定版本公开接口，不创建模型回合、桌面会话或 GUI lease | 依赖可用性；实际 native admission 仍显示 `unknown` |

DSH 验证代码见 [native-flow.test.js](https://github.com/ckanner/flywheel/blob/854c3580ed7ec0f740c7882f23baca11dfa10b6a/plugins/dsh/test/native-flow.test.js)、[native-policy.test.js](https://github.com/ckanner/flywheel/blob/854c3580ed7ec0f740c7882f23baca11dfa10b6a/plugins/dsh/test/native-policy.test.js) 和 [native-journal-bridge.test.js](https://github.com/ckanner/flywheel/blob/854c3580ed7ec0f740c7882f23baca11dfa10b6a/plugins/dsh/test/native-journal-bridge.test.js)。rc.3 实际 PTC executor 已在可控 runtime 边界执行 canonical-value 与多图 deferred-context 路径；这不是 master `inspect` / `Image` 分支的真实 runtime 验收。

## 3. 已完成的验证矩阵

下列结果有交集，不相加为一个总测试数。精确 source、包及平台边界与结果同时保留。

| 项目 | 实际命令／入口 | 已观察结果与边界 |
| --- | --- | --- |
| Flywheel 完整 Go 质量门禁 | `make quality-gate` | **通过**：`go vet ./...`、fmt-check、全包覆盖；总计及每个受测函数均 `100.0%`。最终日志 machine `215.202s`、drivers `24.375s`、diagnostic `0.523s` |
| 原生历史与接管专项 | `go test ./internal/machine -run 'TestNativeKimi\|TestExecutionPolicyChannelFallbackSelects' -count=1` | **通过，2.095s**；使用真实引擎流程与离线驱动，覆盖 §4 所列回归 |
| Machine 独立全套 | `go test ./internal/machine -count=1 -coverprofile=...` | **通过，203.197s**；每函数 `100.0%`，随后已纳入上行完整门禁 |
| Diagnostic 独立全套 | `go test ./internal/diagnostic -count=1 -coverprofile=...` | **通过，0.261s**；每函数 `100.0%`，包含 native manifest 与有效 session mode |
| Codex 最终源码 | 配套仓库 `npm test` | **206 项：201 通过、5 跳过、0 失败**。5 项均为显式 opt-in 的真实 Codex 账户／app-server 推理验收，未据此声明真实模型通过 |
| Claude 最终源码 | 配套仓库 `npm test` | **107/107 通过**；包含 native task/tool 迟到关联和临时目录物理路径回归；云端与交互终端由离线替身覆盖 |
| LCU 最终源码的受影响测试 | `npm run typecheck`；`node --test tests/managed.spec.ts tests/session.spec.ts tests/ptc.spec.ts` | **typecheck 通过，38/38 通过**；覆盖等待 receipt 后再次核对 lease、actor、取消和 generation |
| LCU 最终已打包接口 | 配套仓库 `npm run test:packed` | **21/21 通过**；隔离 consumer 正常 `npm install` 精确 rc.3 peers，未使用 `--force`、`--legacy-peer-deps` 或手工模块链接。不是 macOS 桌面验收 |
| Preset Generator | 配套仓库 `npm test` | **16/16 通过**；daily/heavy、幂等生成、原有用户设置保留和共享 composition |
| ChatGPT provider | 配套仓库 `npm test`；`npm run verify:install` | **277/277 通过**；最终 consumer install 通过，实际导入 Cordis `4.1.0`、Schemastery `3.19.0`、DSH `0.2.2-rc.3` peers。没有真实 ChatGPT 模型请求 |
| Flywheel 根 npm 单元／协议测试 | `npm test` | **152/152 通过**；最终完整候选安装验证另见 §5 |
| 已安装的前一组 Pro 候选 | `node --test --test-concurrency=1 test/codex-pro-worker.test.js test/claude-code-pro-worker.test.js test/machine-lifecycle-bridge.test.js test/machine-worker-protocol.test.js test/native-doctor.test.js test/worker.test.js` | **137/137 通过，0 skip**。对应 Codex tree `ffa66a3...` 与 Claude tree `6c9948e...`；不能充当 §1 新 tree 的最终桥接验收 |
| 前一组候选 Go→Node→公开 API | `FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(CodexPro\|ClaudeCodePro)NodeWorkerOfflineBridge$/^native' -count=1 -v` | **2/2 通过**：Codex root `108` + 独立 child `15` = `123`；Claude raw root `22` 留存且 child snapshots 不再相加。该行只代表前一候选，最终新包重验见 §5 |
| Kimi server 跨进程 fixture | `FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^TestKimiNativeNodeWorkerOfflineBridge$' -count=1 -v` | **通过，2.382s**；实际 Go/Node worker 与本地协议 server fixture，不是实际 Kimi 账户／Swarm 服务验收 |
| 旧 DSH host 兼容专项修复后 | `node plugins/dsh/scripts/test-legacy-host.mjs` | **10/10 通过**；真实公开旧 host 依赖的隔离 consumer。最终总 `check` 状态见 §5 |

### 保留的初次失败与未闭合项

前一轮 Flywheel `npm --prefix plugins/dsh run check` 的主套件为 **394/394 通过**，但后续旧 host consumer **9/10 通过、1 失败**：隔离目录遗漏 `protocol.js` 新依赖的 `native.js`，产生 `ERR_MODULE_NOT_FOUND`。补齐真实依赖复制后，旧 host 专项 **10/10 通过**。没有删除失败测试、放宽断言或把主套件成功当成整体 `check` 成功；最终完整重跑已通过，记录在 §5。

Claude 合并过程的一份中间日志 `claude-latest-full-tests.log` 为 **92 项、91 通过、1 失败**，原因是未完成合并时的语法错误。它不是最终源码验收。采用已整合源码的 `claude-remote-final-tests.log` 才是上表 **107/107** 的依据。

LCU 前一 tree `3ff284b47cb225f29a643c287c6b627d5cc7e113` 的全套 `npm test` 为 **79 项：64 通过、6 失败、9 环境跳过**。6 个失败位于 `control.spec.ts`，本环境 UNIX socket `listen` 返回 `EPERM`；独立 `node:net` 探针可复现相同限制。最终 tree 只重跑了受影响源码测试与 packed consumer，不宣称完整套件已经转绿。最终提交的远端 CI 已通过（见下表），与本地受限环境记录分开保存；本地这 6 项失败与 9 项跳过没有被改记为通过。

### macOS CI 累计测试预算修正

[首轮 Flywheel CI](https://github.com/ckanner/flywheel/actions/runs/43582131166) 的完整单包构建与 Linux 全套（包括 race、安装和 36 场景）通过；macOS 在 `internal/machine` 累计 `600.065s` 后触发 Go 默认 10 分钟包级超时，因提前终止只得到 `98.9%`，该轮没有通过覆盖率门禁。超时时当前 `TestQuickCompletesWithTwoAgentCallsAndCandidateIntegrationEvidence/false` 仅运行约 2 秒，堆栈位于 Git 提交；没有据此把它描述为单项用例挂起 10 分钟。

实际基线为 [main c000f6d 的 macOS job](https://github.com/ckanner/flywheel/actions/runs/43580329214/job/132403788792) `563.964s`、[PR #46 的 macOS job](https://github.com/ckanner/flywheel/actions/runs/43574758128/job/132386668622) `578.507s`，两者机器包和逐函数覆盖率均通过。本 PR 新增 8 个顶层 machine 回归；按测试文件／声明顺序静态核对，超时用例位于第 523/554 个顶层测试，后面仍有 31 个。此顺序是源码核对，不冒充逐项计时记录。

据此为 `make test`／`coverage` 明确设置有限的 `15m` 包级预算，保留全部测试、全部断言、每函数 100% coverage 和 CI job 的 20 分钟总预算。生产 watchdog 与业务超时没有变化，也没有把初次的 98.9% 计为通过。修正后的精确 head／workflow 结果由 #44 的 PR 说明与 checks 记录。

### macOS Tower 路径与目录别名回归

[第二轮 CI](https://github.com/ckanner/flywheel/actions/runs/43583479213) 对应 `0299671d4a3ec68b8d3352aa58b2f114d94fa23a`。单包构建和 Linux 全套通过；macOS 的 Go 质量门禁通过，machine 包 `340.561s`、每函数 coverage 100%。随后 Node 主套件为 **388 通过、6 失败、0 跳过**，没有运行后续安装／桥接门禁，不能记为 macOS 全套通过。

六个失败均来自直接调用 Tower helper 的 fixture：macOS `mkdtemp` 返回的逻辑临时目录与 Git 报告的物理根目录不同。正式 worker 入口已解析 `cwd`／`taskDir`，candidate 自身也已规范化；修正将 source 和 linked-unit 两处 fixture 同样先 `realpath`。仓库根目录检查、tracked symlink 祖先拒绝、source／外部 contract 指纹和 Execute-only 导回规则都保留。

通过真实 `runKimiWorker`、HTTP／WS 子进程增加 Linux 可重现的目录别名回归时，还复现了运行代码缺陷：入口规范化后丢失了声明路径的拼写，prompt 中该别名仍可能指回原工作区。修正保留已通过准入的声明别名，并在原 prompt 中一次性映射它和物理目录；候选路径不二次替换。配套回归验证原始／提交 prompt 的实际摘要、物理 source 指纹、外部契约及源 HEAD 不变、成功导回和清理。另覆盖 URL query 保留及嵌套前缀选择，防止相似路径误改或合法较短前缀漏改。

旧候选身份与历史结果仍原样保留；包含这次运行修正的完整候选须单独构建、安装和记录，不能沿用旧 tarball 的通过结果。最终 PR head 的跨平台结果由 #44 的 checks 与说明记录。

### macOS Pro 桥接测试的恢复目录

[第三轮 CI](https://github.com/ckanner/flywheel/actions/runs/43585747595) 对应 `f4ac488fa58f267f619adbee6a07eba2621cee92`：完整候选和 Linux 全套（含 race、安装与 36 场景）通过；macOS 完整 Go 门禁与 **395/395 主 Node + 10/10 legacy host** 通过。随后新增 Codex／Claude native Go→Node 各一个用例在约 40–50ms 返回 `WORKER_ERROR`、`receipt:null`，其余两者用例及 DSH、Kimi 桥接通过。后续 macOS npm／安装门禁没有执行，该轮整体仍为失败。

Linux 将 `TMPDIR` 指向真实 symlink alias 后，原两用例精确复现相同失败。公开 Pro API 的 protocol 3 journal 要求物理父目录；正式引擎的 `CreateContext` 已规范化任务与执行目录，既有 durable lifecycle fixture 也已调用 `EvalSymlinks`。遗漏只在两个新 native fixture，它们直接用原始 `t.TempDir()` 派生 `RecoveryDirectory`。修正仅在这两处派生前解析物理目录，不改运行代码、journal 所有权校验、child 归属或用量断言。同一个 Linux alias 环境中，原两个用例在 `0.204s` 失败，修正后 **2/2 通过，0.690s**。随后沿用该 alias 环境复验 **四条完整 Go→Node 桥接，全部通过，44.975s**，没有跳过；本地 `make quality-gate` 随后再次通过，**每个受测函数均 100% statement coverage**。最终跨平台结果由 PR 的精确 head 检查继续核对。

这轮 CI 的安装包已实际下载核对：`18,899,640` bytes，tarball SHA-256 `c1d5cabd85ce4d4a82c1cd2dfd69ff63c4577c3964682bb2f1c94469d527b345`；完整 integrity 见 [候选证据中的 ciThirdRun](evidence/2026-11-14-native-harness-tower-candidate.json)。GitHub PR test-merge `1e6cb302eddfd950d67ea32f24719999cb690b67` 的父提交为 main `c000f6d…` 与 PR head `f4ac488…`，tree 与该 PR head 同为 `a84f754fae398cd5f4fdb6c8ccc8df99ee645ae3`。artifact ZIP 摘要与内部 tarball 摘要分别验证，不混同；后续测试夹具／证据修正不改变运行代码。

## 4. 已闭合的关键回归

- **Native profile 与真实通道一致。** Go→Node 请求保留原始 native options；`requested`、`effective` 和 `observed` 分开处理。显式 off 通过公开控制实际关闭，配置不匹配时不能仅靠提示词继续派发。
- **Child 归属、终态与费用独立核对。** native-event 不充当生命周期 ACK；早到事件、历史 replay、opaque task run ID、迟到 task/tool 关联不把旧完成事实套到新调用。成功仍要求整树 completion confirmed、没有 pending children；局部进程退出不自动证明云端任务或完整子树已结束。
- **用量不重复记账。** Codex 只在 root 与每个已归属 child 的独立增量完整时相加；Claude snapshots 不与 root 再次求和。不完整家族用量保留已知证据，总量保持 unknown，不能写成零或完整。
- **人工接管沿用相同原生历史。** `handoffArgs` 与 `invokeScoped` 共用解析，在计算 runtime identity 前填入 native options。Agent/Swarm 的 claim→send→release→继续执行经实际状态重载通过；缺失／异类 profile、配置或 worker 变化在派发前拒绝。
- **Fallback 切换完整 binding。** 同 harness 的不同 native profile、跨 harness 显式备用 profile、切换到不带 native 的 binding 均经过测试；目标权限、渠道、模型与 profile 同步，新调用不借用旧 thread。
- **Native Kimi Plan 不继承 legacy CLI ID。** Agent/Swarm 使用角色 scoped ledger；既有 `resident-session:true` 意图可映射为 scoped；显式 `sessions.roles.plan:"fresh"` 优先。Tower 永远 fresh。10 组策略均执行两次实际 `planStep` 并在中间 Save→Load，第二次不会因非 persistent + sessionId 组合失败。普通无 native profile 的 CLI 行为保留。
- **Doctor 不虚报能力准入。** DSH native profile 跳过 legacy SDK probe；`--native-doctor` 只导入 composition/managed API。manifest 可通过，实际 admission 仍为 unknown。配置检查对 Tower 显示 fresh、不可 scoped resume；对 native Agent/Swarm resident 显示实际有效策略。

相关测试见 [native_plan_test.go](https://github.com/ckanner/flywheel/blob/854c3580ed7ec0f740c7882f23baca11dfa10b6a/internal/machine/native_plan_test.go)、[console_session_test.go](https://github.com/ckanner/flywheel/blob/854c3580ed7ec0f740c7882f23baca11dfa10b6a/internal/machine/console_session_test.go)、[execution_policy_test.go](https://github.com/ckanner/flywheel/blob/854c3580ed7ec0f740c7882f23baca11dfa10b6a/internal/machine/execution_policy_test.go)、[Pro receipt tests](https://github.com/ckanner/flywheel/blob/854c3580ed7ec0f740c7882f23baca11dfa10b6a/internal/drivers/pro_native_test.go) 和 [native diagnostics](https://github.com/ckanner/flywheel/blob/854c3580ed7ec0f740c7882f23baca11dfa10b6a/internal/diagnostic/native_test.go)。

## 5. 最终集成与完整候选

本节先保留最终五个配套包与首份完整 Flywheel tarball 的实际验收。该候选在干净本地提交构建，安装测试断言修正后仍用同一 tarball 验收。后续 Tower 目录别名修正改变了运行代码，须用新的完整候选单独验证，不能将原候选的通过结果转记给它。

| 项目 | 最终命令／前提 | 实际结果与证据 |
| --- | --- | --- |
| 新包完整 Node 检查 | 安装 §1 的最终精确候选后，`npm --prefix plugins/dsh run check` | **通过**：主套件 **394/394**，`161813.556ms`；legacy host **10/10**，`1922.511ms`；均 0 失败、0 跳过。Node `24.22.1` / npm `12.2.1`；日志 `native-node-final-check.log` |
| 最终新包 Go→Node→公开 API | `FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(DSH\|CodexPro\|ClaudeCodePro\|KimiNative)NodeWorkerOfflineBridge$' -count=1` | **四条入口及其子测试全部通过，43.574s**，无跳过；使用 §1 的最终五个配套包。日志 `native-all-bridges-final.log` |
| 单包候选构建 | `npm run build:npm` | **通过**：隔离源码快照、正常 npm 安装、构建四平台引擎并核对清单和校验；仅生成一个 `ckanner-flywheel-0.34.0.tgz`，`18,861,167` bytes。精确身份如下；日志 `native-package-build-final.log` |
| 同候选真实 npm 安装 | `FLYWHEEL_NPM_PACKAGES_DIR=dist-npm FLYWHEEL_CLAUDE_CODE_PRO_TARBALL=<已验证的0.5.0候选> npm run test:install` | **通过**：同一原始 tarball 的 Host/Client **10/10**；正常 npm 安装、独立 workers、可选 Claude、配置保留与重复安装／迁移 **15/15**；均 0 失败、0 跳过。日志 `native-install-final.log` |
| 36 个引擎 session 场景 | `make build`；`FLYWHEEL_EVAL_BINARY="$PWD/flywheel" npm run test:sessions` | **引擎构建通过；36/36 场景、96 次调用完成，全部 `oraclePass:true`**，`stopReason:null`，6 类 scenario 各 6 次且 unknown/unobserved 均为 0；日志 `native-sessions-final.log` |
| 精确远端 CI 与交付身份 | 六个 PR 的最终 head、checks；Flywheel 主分支文档更新合入后的 source 身份 | 五个配套源码 head 的远端 CI 均通过，链接如下。Flywheel 实现提交保留最新主分支 `c000f6d8a11522521d7a30203e74b735490e9b73`；新 PR head 的最终检查与交付状态见 [#44 checks](https://github.com/ckanner/flywheel/pull/44/checks)，由 PR 说明记录精确 run，避免文档提交自引用。Go 实现和五个配套包保持原验证身份；后续 Tower 运行修正另行验证。 |

36 个场景的 report 为 `/tmp/flywheel-native-eval-AbJ1L5/results/report.json`，明确标为 `execution:"offline-fixture"`。使用的引擎 SHA-256 为 `5018c0e22d220f5517d5418f5f506c7de8f8942c6b753fd595b94a28bf208ade`；manifest SHA-256 为 `8e153aba584ac485524d46c8c06cf647868a3e567cd36f9491ea76c4d710b603`。该实验比较相同任务／绑定／effort／权限／oracle 下的 Execute/Review history mode，Plan/Judge 在实验三组中均显式 fresh；不代替 §4 专门针对原生 Kimi Plan 的 10 组回归。

完整 Flywheel 候选已构建并安装验证；registry publication 与用户机器安装仍是独立状态，本轮没有发布或更改用户配置。


### 首份完整候选身份

本地构建提交为 `1ba506a2ac6996700828ce40274b2c287de1103d`，`dirty:false`，tree 为 `bc65e37eab9844064699cf402481112160fc092e`；远端 [854c3580ed7e](https://github.com/ckanner/flywheel/commit/854c3580ed7ec0f740c7882f23baca11dfa10b6a) 的 **tree 与其完全一致**。两种提交路径的 commit ID 不同，没有将本地 hash 当成可获取的远端提交。

- 源码 snapshot SHA-256：`6002492ef2fee1ac1a73d46c33219cf6d2f0af79bdabc76ca15c124926d8f9c1`。
- 完整 tarball SHA-256：`fa682d25986ea8d93514a7a6705edfbaea37aff0c9257adf73e4105a1d7b86f4`。
- npm integrity：`sha512-3vtGBDA1O1Q5JqBPi76Q3fpHjmmuZGPHJLE+mmOH/bnQLaKNo+4jlY3XnDzTSFDEOv5b4z5jRlXRvF3XIN1FEQ==`。
- 四平台均为 `0.34.0`：darwin-arm64、darwin-amd64、linux-arm64、linux-amd64；每个引擎的 bytes/SHA-256 见归档 JSON。Kimi npm launcher 的 tar mode 为 `0755`。

安装门禁首次在测试夹具的旧断言处失败：它仍要求单个 Codex bundle。修复为精确检查 **Codex Pro 0.10.0 与 Preset Generator 0.4.0 两个 bundle**，同时补齐两种 Pro doctor 的 `nativeVersion:1`，验证安装后的 Kimi launcher／exports、generator 公开 composition API，以及两包均没有 registry 获取请求。使用 **同一个上述 tarball** 重跑，Host/Client 10 项和安装 15 项全部通过；未重打包换掉失败对象，也未弱化包身份检查。这些安装测试位于不随 tarball 发布的 `npm/test/` 中。

首份候选之后的安装断言和证据链接修正只改变测试／文档。第二轮 CI 后的 Tower 修正则改变运行代码，已通过针对性 Kimi/Tower **22/22**（0 skip，约 `27.27s`）和独立只读审查；新增真实目录别名回归在修复前明确失败，修复后通过。完整 Node、Go→Node 和新候选安装结果见下一小节。最终 PR CI 从精确 head 构建并安装其自己的完整候选，各次 tarball hash 不混同。

### Tower 修正后的完整候选与复验

运行源码对应远端 [be4351fbb9fb](https://github.com/ckanner/flywheel/commit/be4351fbb9fb54b450f3963b5065026434c592b0)，与本地干净构建提交 `1512b4b690a8d2b01edc70994ba4404b92659d79` 的 tree 均为 `1083a22b5177797fcde323480bda733b9042e06f`。该候选包含目录别名、URL query 和前缀边界修复，未改变五个配套包或 Go 运行代码。

| 修正后门禁 | 实际结果 |
| --- | --- |
| 完整 Node `npm --prefix plugins/dsh run check` | **395/395** 主套件、**10/10** legacy host；均 0 fail、0 skip |
| Kimi 实际 Go→Node worker 桥接 | **通过，2.048s**；使用完整检查刚构建的 worker |
| `npm run build:npm` | **通过**；干净快照、四平台、单个完整包，`18,873,136` bytes |
| 同一新候选 `npm run test:install` | **Host/Client 10/10、安装／迁移 15/15**；可选 Claude 0.5.0 参与验证，均 0 fail、0 skip |

新 tarball SHA-256 为 `41f3db0ed3d5e50fc0b0825f8278d49b1d4653732f7f0f4758a7bf3b074e1ee8`，npm integrity 为 `sha512-1058P0psSstNlkzycB9HEeeCa/xmoGT4Th37/KTkApnmInUDUXnKIxECaWgWLQfp6NawoAoca8sJC7N1DVDH/g==`。完整 source snapshot、四平台引擎、包内 Tower 模块摘要及逐项结果见 [修正后候选证据](evidence/2026-11-14-native-harness-tower-candidate.json)。Kimi launcher 仍为 `0755`。

本验证记录和该候选证据在构建后补齐；随后没有运行代码变化。最终 PR head 的 CI 会另外构建其文档快照对应的候选，Linux/macOS 安装同一份 CI artifact。最终状态由 [#44 checks](https://github.com/ckanner/flywheel/pull/44/checks) 与 PR 说明记录精确 head／run，避免通过提交 CI 结果文档不断重启 CI。

### 已核对的配套 CI

| 仓库 | 精确 head | 远端结果 |
| --- | --- | --- |
| Preset Generator | `0ec05fbeb37d59f7e3af4baa0f6d9bfb0c5573a3` | [run 43577043240](https://github.com/ckanner/dsh-preset-generator/actions/runs/43577043240)：success |
| LCU | `7986057e92314b78caf66dde43d1a8d45ab6ada7` | [run 43579319633](https://github.com/ckanner/dsh-plugin-lcu/actions/runs/43579319633)：success |
| Codex Pro | `c0045b06e6762b8dfc56f50b14cce7a53af2725b` | [run 43580027819](https://github.com/ckanner/dsh-subagent-codex-pro/actions/runs/43580027819)：success |
| Claude Code Pro | `54d7eafa9102713597e3b96a6d70a9cbd86afc6b` | [run 43579998664](https://github.com/ckanner/dsh-subagent-claude-code-pro/actions/runs/43579998664)：success |
| ChatGPT | `fa5cfafcc1cc6bfb1f640e646b6dc3c4329d976c` | [run 43579984957](https://github.com/ckanner/dsh-plugin-chatgpt/actions/runs/43579984957)：success |

### 首次生成补丁的后续复核

对六个 PR 的独立复核重新核对了当前 DSH release `dsh-v0.2.2-rc.3` 和 master `39163dd30f703a79079e2d2446545cff3c453738`。SDK 不自动挂载 named preset、官方 PTC overlay、Team 与 Workflow 生命周期均与当前接入一致。既有 Codex 子线程归属／未完成用量、Claude 迟到 task/tool 关联和 LCU receipt 等待期间的权限变化问题，均已在源码与回归中确认修复，对应四个 review 线程已解决。

本轮实际复现 Preset Generator 在 `--out FILE` 为新文件、或 profile 尚无 `cordis.patch.yml` 时直接 `ENOENT`。修正仅将缺失文件作为空补丁，并用 `wx` 独占创建；既有内容合并、读权限错误、目录目标和缺失父目录均保持明确结果。运行期间由其他程序新建的文件不会被覆盖。

- 新 Generator 源码：[3dc21791b1efe81bea9a26aa9e92ff2172b4f645](https://github.com/ckanner/dsh-preset-generator/commit/3dc21791b1efe81bea9a26aa9e92ff2172b4f645)，tree `877ea728cad4d8ef4b21a691df2a03cfc9075efc`，版本仍为未发布候选 `0.4.0`。
- 源码测试 **21/21 通过**，新增五个真实 CLI 回归；分别使用 release/master 的实际 PTC preset 内容构造临时 ASAR，首次创建、Workflow 保留、PTC 唯一性和重复生成均通过。
- Node `24.22.1` / npm `12.2.1` 正常打包的 tarball 为 `17,668` bytes，SHA-256 `25e6a718605dfb3fc0f8869d1685dc44d2e1f35375ea8c184825bfe21564a141`。两个 lockfile 和内嵌 tarball 已同步；完整 integrity 在 [preset-generator.json](../../plugins/dsh/vendor/preset-generator.json)。上文旧 tarball 与 CI 身份仅证明当时的候选。
- ChatGPT [1cb961917bdb4a9d7ce86a997bc4f06d8807e904](https://github.com/ckanner/dsh-plugin-chatgpt/commit/1cb961917bdb4a9d7ce86a997bc4f06d8807e904) 将登录通知测试的固定 50ms 等待改为既有有界事件等待，并补失败清理；保留 signed-in 和 exactly-once 断言。最终 **277/277**、session **16/16** 及 typecheck/build/package/client/i18n 通过。只有不进入 npm 包的测试文件变化，Flywheel 的 provider 包继续使用既有已验证字节。
- 本轮原锁定依赖下，Flywheel 本地 **395/395 主测试、10/10 旧宿主、152/152 根 npm 测试**再次通过；Codex **201 通过／5 live 跳过**、Claude **107/107**、LCU 受影响 **38/38** 和 packed consumer **21/21** 通过。LCU 全套仍为 **68 通过／6 Unix socket `EPERM`／9 Mac 跳过**，没有把环境限制计为通过。

新 Generator 进入完整 Flywheel 包后，重新核对其精确依赖、四平台构建和同一候选的实际安装。新完整候选与 Linux/macOS CI 结果在 #44 的 PR 说明中按具体 head/run 记录；不沿用上文旧候选的成功状态，不为记录 CI 再次修改构建输入。真实账号、桌面和远端作业的验收边界保持 §6 所述。

## 6. 真实运行与产品能力限制

本轮未验收真实 macOS 的 DSH／ChatGPT 桌面、named Agent Preset 切换后运行、LCU 浏览器／桌面 GUI、真实模型账户／额度、真实 Kimi Swarm/Tower、Claude 付费 ultrareview 或实际用户交互式 Team 会话。fixture 不证明模型质量、缓存收益、延迟、成本节省或真实截图送达模型。

公开 rc.3 的 deferred-context 测试通过，不代表 DSH master 的 `inspect`、`Image`/`PtcImage` 行为已在真实 runtime 通过。master image 路径保留为**未测试**；运行时必须根据实际 probe 回执决定输出契约，不能只凭版本字符串假定兼容。

Claude 的部分并发、深度和模型路由缺少可验证的全树 pre-dispatch hard cap；实现中的观察与取消 guard 不改称硬限制。远端 ultrareview 没有可证明的云端查询／取消接口时，停止本地进程或恢复 journal 不等于远端已取消，也不会自动重放一次付费请求。

独立 native review / Teams / remote jobs 返回各自证据，不能直接产生正式 Flywheel JEV 裁决。图片、structured output 和 Claude `maxBudgetUsd` 的公开 machine/Node 入口已实现；正式 Go loop 不会自动将 LCU evidence 转换成后续请求的 `images`。`imageDeliveredToModel`、远端 completion 和家族总用量仍按实际回执保留 unknown。

## 7. 本轮日志定位

本地证据工作区为 `/workspace/scratch/592fa92add6f`。以下为实际读取的日志名称，属于本轮临时审计记录，不是永久下载链接：

- 完整 Go：`native-quality-final.log`，末行 `OK: coverage 100% (every function)`。
- Machine／diagnostic：`research/native-machine-recovery-focused.log`、`research/native-machine-final-tests.log`、`research/native-machine-final-coverage.out`、`research/native-diagnostic-final-tests.log`、`research/native-diagnostic-final-coverage.out`。
- 最终配套身份：`native-candidates/latest-20261114/final-artifacts.json`；永久仓库对应记录为 §1 的五个 vendor JSON。
- Codex／Claude：`native-candidates/codex-latest-npm-test.log`、`claude-remote-final-tests.log`；前一组安装桥接为 `research/flywheel-pro-final-node-tests.log`、`research/flywheel-pro-final-native-bridge.log`。
- Generator／ChatGPT：`generator-final-tests.log`、`chatgpt-final-tests.log`、`native-candidates/chatgpt-latest-verify-install.log`。
- Flywheel Node／Kimi：`native-npm-final.log`、`native-node-check.log`、`native-legacy-fixed.log`、`native-journal-bridge.log`、`managed-lcu-root-tests.log`、`kimi-go-node-test.log`；最终五包集成以 `native-node-final-check.log`、`native-all-bridges-final.log` 为准。
- 引擎与 36 场景：`native-sessions-final.log` 和上节列出的 `report.json`，保留实际引擎与 manifest 摘要。
- LCU：`/tmp/lcu-authority-race-source-tests.log`、`/tmp/lcu-authority-race-packed-tests.log`、`/tmp/lcu-merged-full-tests.log`；保留成功和失败两类证据。

Pro 桥接物理路径复验保留 `research/pro-bridge-alias-before.log` 的原失败和 `research/pro-bridge-alias-after.log` 的通过，以及 `research/pro-bridge-alias-all-four.log` 的完整四桥接结果；正式完整质量门禁结果见 `native-quality-pro-fixture-fix.log`。

Tower 修正复验日志：`research/kimi-alias-prompt-before.log`、`research/kimi-alias-prompt-after.log`、`research/kimi-tower-alias-regression.log`、`native-tower-node-check.log`、`native-tower-go-node.log`、`native-tower-package-build.log`、`native-tower-install.log`。失败与通过日志分别保留。

稳定证据归档：[修正后候选身份与门禁摘要](evidence/2026-11-14-native-harness-tower-candidate.json)、[首份完整候选身份与门禁摘要](evidence/2026-11-14-native-harness-candidate.json)、[36 场景原始 report](evidence/2026-11-14-native-harness-sessions.json)。最终单包构建／安装日志为 `native-package-build-final.log`、`native-install-final.log`；远端 CI 日志通过上表与 #44 checks 访问。原始 report 明确是离线 fixture，历史结果及当时源码身份保留。
