# P5 就绪队列与 DSH 运行时预热：工程验证

[验证索引](README.md) · [本轮实施方案](../plans/2026-11-13-p5-ready-queue-prewarm.md) · [配置指南](../zh-CN/configuration.md#就绪队列与运行时准备) · [恢复指南](../zh-CN/session-recovery.md#dsh-预热运行时的重启核对)

## 源码与本轮范围

本轮接在 PR #37 的 `c445393c4f7238677146028b89efe520253f8318` 后，main 基线为
`e035b5b5736c2d26da7ada2bf1ca97ee020fd5a2`。本记录与实现一同提交，最终源码以
承载本记录的 Git commit 为准。六包版本仍为 0.31.1，模型／Provider 默认、依赖锁、
vendor、用户实际配置没有改变。

用户要求把原计划中的条件性工程优化也补齐，真实账号、性能和 Mac 桌面验收留到
后面。因此“工程能力实现”与“原方案的净收益验收”分别记录，前次 JEV 验证中 P5
尚未实施的结论保留为当时事实。

| 本轮能力 | 实际范围 |
| --- | --- |
| 就绪队列 | Full、parallel、自动管理 Worktree、并发上限大于 1 时，Execute／L1 空位及时补位 |
| 流水化 | 独立执行槽位不等待批次尾部；串行 packet 的检查与下一次 DSH Review 初始化重叠 |
| 一次性预热 | 原公开 DSH SDK `initialize`，无 Session、无 prompt；正式调用之后才创建或恢复角色会话 |
| 配置与工作台 | `scheduler.dispatch`、`scheduler.prewarm`；缺省 batch／false，保留原并发及工作流选择 |
| 恢复 | 持久准备意图、原私有清理回执、PID 与进程组退出核对、不可降级的 sessions v3 标记 |
| 已有候选接纳 | 完成受管 Session claim／return 与通用补丁导入的能力评估；没有新增任意补丁导入 |

`jev.enabled` 仍是唯一 JEV 总开关。这两个 scheduler 字段不属于 JEV 子开关；
Task／New／Run、单阶段操作和 Auto／Quick／Full 的职责保持。

## 环境

- Linux x86_64，Go 1.27.4，Node 24.22.1，npm 11.9.0。
- DSH 依赖保持原锁定的 0.2.2-rc.2；旧宿主兼容 fixture 为 0.2.1-rc.5。
- Codex Pro 保持原 vendor 0.9.0。Claude Code Pro 使用真实 0.4.0 源码 tarball，
  SHA256 为 `125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`。
- SDK 桥接使用原公开实现与离线模型 fixture，没有调用真实 JEV 或付费模型。
- 本地原生二进制 SHA256 为
  `036c1bf957ee8943e14310db02642f3d194c5e86d85969b1c517b7d2af81f9d1`，用于下面的
  36 条场景，不是 npm 发行产物。

## 已执行检查

以下是实际执行的离线工程检查；没有以历史结果替代新行为的验证。

| 检查 | 结果 | 证据边界 |
| --- | --- | --- |
| `make quality-gate` | PASS | 全 Go 测试、vet、gofmt、逐函数 100% statement coverage；最终 machine 全包约 195.3 秒 |
| queue／parallel／shared 的 machine race | PASS | 补位、依赖、共享预算、取消、准备失败、重规划与恢复；parallel.go 全部 14 个函数 100% |
| machine `TestPrewarm*` race | PASS | Full／Quick 真实编排、意图持久化、验证顺序、绑定变化及失败阻断 |
| drivers 全包 race + 最终 TTL 定向 race | PASS | 全包 72.167 秒；最后 TTL 修复后定向 4.198 秒，新增 21 个函数全部 100% |
| config／state／contract race | PASS | 配置默认与加载、扩展 schema、v1／v2／v3 兼容及旧引擎拒绝新责任 |
| `npm test` | 142 PASS，0 skipped | npm 启动器、安装器及发行协议；最终一轮约 29.4 秒 |
| `npm --prefix plugins/dsh run check` | 274 + 9 PASS，0 skipped | 主套件及旧宿主兼容；最终 TTL 改动另过 14 项 Node 行为测试，最终 UI 文案另过构建及两项回归 |
| DSH／Codex Pro／Claude 三个 Go → Node bridge | PASS，0 skipped | 最终 DSH 使用 race，15.428 秒；另两后端约 29.9 秒；公共适配器与实际子进程 |
| `make build`、36 条 session evaluation | 36 / 36 PASS | 96 次离线 adapter 调用，36 项独立 oracle 通过，0 未观察场景、0 未解决恢复或未知 outcome |
| 六包候选及隔离安装 | 10 PASS，0 skipped | 六个同版本实际 tarball，隔离 registry／profile，包含真实 optional Claude 包；不发布 |
| 文档链接／锚点／围栏、`git diff --check` | PASS | 21 个 Markdown、371 个相对链接、82 个 anchor、103 个围栏；当前双语指南、设计、方案与本记录 |

第一轮完整 Go 门禁暴露 formal handoff 错误传播分支缺少调用链覆盖；补充真实
RunTask 回归，断言清理未知时零 Review 派发、保留原责任、不得进入下一 packet，
随后最终完整门禁逐函数达到 100%。没有放宽覆盖率规则。

### npm 候选身份

[六包候选清单](evidence/2026-11-13-p5-npm-candidate.json)保留每包字节数和完整性。
它是以 `c445393c` 为父基线的开发快照，`dirty:true`，snapshot SHA256 为
`ff80895639a09f0e6310948d9e6530c51c1bae7a2784ee16eb55ac0ac2522496`，不能用于正式
发布。构建完成后仅补充本记录的验证结果与候选身份，不把候选冒称最终 clean commit。

直接检查插件 tarball，确认 `dist/prewarm.js` 与最后源码逐字节相同，且实际 worker
导入该模块；其 SHA256 为
`59c07971b67c10496b4cd1cbb3aa9d3939abdd1f80e438ff2e99cfe7640d9636`。
安装测试使用真实六包和独立用户目录，覆盖重复 setup、用户配置／技能保护、原生
取消、公开 adapter 运行、配置编辑与部分失败恢复。

## 关键行为证据

### 就绪队列确实缩短批次等待

受控长／短 lane 测试证明：首个 packet 的 Execute 和 L1 都完成后，下一独立项可在
慢 sibling 尚未完成时启动，并发上限不增加。没有把 Execute 返回当作依赖完成，
没有让有冲突、缺少写入声明或需要 live 凭据的项目获得额外并行资格。

首窗须全部准备且 CAS 保存后才运行；后续补位只为实际将接纳的一项准备工作区。
尝试耗尽、空转、取消、预算停止和后续 worktree 准备失败不接纳更多项；未接纳项
保留原状态。测试检查真实已启动集合、已有产物、等待清理及恢复后的调用数量。
Plan 修订之后重新选择，不使用原队列执行新契约。

共享 checkout 仍走原批次和共同候选屏障。Review、Plan、集成等操作仍等已启动
worker 停稳。准备了工作区但尚未 Execute 的恢复项先吸收已交付基线，再走正常
Execute／L1；不对空候选伪造原执行证据。

### 初始化与正式推理有可观察的边界

Node 与 Go 跨进程测试观察 prepare 阶段仅调用 `initialize`，在正式请求前不存在
Session／prompt。实际消费同一 SDK 后才创建或恢复会话，正式请求保留自己的
RequestID、权限、执行目录与项目目录。public SDK 通过原 `InvokeHarness` 测试
read-only、项目目录与执行目录分离、fresh、persistent create 和 persistent restore。

machine 测试分别执行 Full 和 Quick：准备发生在 Execute 证据保存之后；实际 Review
必须等待适用 L1／Quick gate 与正式调用意图持久化。scope、账号 channel 或运行时
绑定改变会先关闭未使用实例；关闭未知时没有 warm／cold Review 派发，也不执行
下一 packet。单阶段 Execute、明确暂停在 Review 前、并行 lane 与共享候选不预热。

`worker-prewarm` metrics phase 记录准备与验证可能重叠的窗口，不是纯 SDK 启动
延迟，不计成额外模型调用，也不证明该实例在调用时已经准备完成。正式调用时长从
正式 Run 入口计算，不包含之前的验证等待。

### 过期、取消与恢复不能伪造成功

准备准入最多 60 秒。到期原子禁止新的 formal handoff，再给正常关闭一个最多三秒
的短窗口；超过后沿用 watchdog TERM／KILL 并等待进程组。正式请求已移交时，
已排队的准备计时器不能取消它。Close 与到期竞争不能重置关闭宽限。

跨进程检查曾暴露一个实际问题：Go 的 TTL 比 worker 先开始计时，直接终止整个
组会先杀掉 SDK，无法取得正常 shutdown 证明。本轮改为先退休派发资格，再有界
等待正常清理，并加入同一 RequestID 安全过期冷启动的桥接回归。只有尚未尝试
正式 start、精确未派发收据、可信完整输出、SDK 与整个进程组清理及剩余期限均
成立时，才允许一次冷调用。该调用共享剩余 wall／idle，不新增业务尝试或预算。

缺失、错配、损坏、非私有、符号链接、过大、追加 JSON、文件或目录同步失败的
回执均不提供恢复许可。重启要求 worker PID 和原 PGID 都明确退出；leader 已退出
但孙进程仍存活也阻断。没有启动进程的本地事实使用单独 no-process 回执，不凭
缺失 PID 推断。取消、SDK 关闭未知或 formal 派发未知不能被优化开关覆盖。

首次准备将 session 账本提升为 v3；后续正式调用、清理、接管保持版本。新引擎
兼容旧 v1／v2，旧引擎按原 schema 拒绝 v3，防止其忽略新的清理责任。

## 未执行或未证明的内容

真实账号和 Mac 桌面验收按用户要求后置。没有测定端到端速度提升、token／费用
下降、真实模型质量等价、预热净收益或应当默认开启的阈值；不能把 fixture 的时间
或 100% statement coverage 当成这些结论。

本轮仅实现单个一次性 DSH Review runtime，不声称原生 Codex Pro、Claude Code Pro
或 Kimi 已有相同公开预热接口，不是长期常驻 worker 池。通用外部补丁导入仍是
独立工作，受管 Session claim／return 的已有能力在实施计划中单独说明。

远端 CI 与本地门禁分别核对；本记录不代替 GitHub Checks。没有合并、tag、npm
发行或更新用户机器。本地候选只用于检查新 worker 模块随包可用。
