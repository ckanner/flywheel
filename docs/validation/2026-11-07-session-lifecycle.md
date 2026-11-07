# 角色会话续接与回执恢复：实现验收记录

日期：2026-11-07。实现 PR：[flywheel #31](https://github.com/ckanner/flywheel/pull/31)。

## 来源与交付边界

本实现基于已合并的 [#29](https://github.com/ckanner/flywheel/pull/29)（Auto／Quick／Full）
和仍开放的设计 [#30](https://github.com/ckanner/flywheel/pull/30)。恢复源码时逐一核验了
#30 的 344 个 Git blob，基线提交为 `e56e8b62e48b1f938df2b54e94bfbede50ecc1e0`，
基线 tree 为 `e30631033405f6465c7650a61e0f4cafea5c2b43`。

按[方案](../plans/2026-11-07-session-lifecycle.md)的分期，本 PR 实现 P1／P2：
Codex Pro、默认 DSH 路径与 Kimi 的角色续接，以及正常终态回执落盘后的恢复。
它没有把“阻止未知结果重放”描述成已经实现 P3 的早期确认与只读恢复 API，也没有将
未进行的 P4 真实质量、缓存、性能实验标为完成。Intake／Garden 不属于本轮主任务角色。

所有测试均在本轮恢复后的源码上重新执行。此前已失效工作环境的测试结果不作为本次
候选的通过依据。提交按阶段保存到独立实现分支，未修改 main、发行 tag 或 npm latest。

## 实际行为

- 新任务模板选择 `"sessions": {"mode": "scoped", "policy-version": 1}`；旧配置省略该项时
  仍按 fresh 读取，不隐式迁移。Plan 明确的 `resident-session:false` 保留其含义。
- 会话属于任务、物理工作目录、角色及 packet／里程碑。Quick 最终 Review 与 Full 单元
  Review 分开，Judge 与第二意见分开；普通修复分别续接 Executor 和 Reviewer 自己的历史。
- Spec、packet、里程碑的目标身份采用明确的字段集合。兼容的验收／版本修订可以续接已
  消费的历史；目标、正文、边界、需求归属、模型、渠道或权限发生不兼容变化时重建。
- fresh 和 scoped 都记录支持后端的主任务调用。切换模式、换后端、换角色或重新启动，
  都不能越过未核对的派发、结果或清理事实。历史存在也不能重置预算、尝试次数和停止意图。
- Go 在派发前保存 intent；结果先写入有摘要的原子回执，再以 state CAS 应用。每个作用域
  只保留一个最新回执槽，计量按 invocation ID 与 receipt digest 去重。
- 收到的回执与是否可以继续推进分开判断：可信调用的消耗应记账；过期候选、无效保护证据
  或未知执行事实仍阻止继续运行。取消不丢掉已发生的消耗，Go 证明未调用适配器的本地取消
  则记录零消耗，避免制造未知账单。
- Plan／Review／Judge 原始前后保护证据一起持久化。重启不能用被修改后的候选重新定义
  “调用前”基线，也不能用新的 L1、集成检查或缺陷抽检替代原回执所见证据。
- 不更换既有模型或付费渠道。默认 Flash Execute、DSH GPT Review、Kimi 订阅及 Codex Pro
  路径分别按自己的适配器能力处理；没有增加默认第二意见或 Quick 模型阶段。

## 后端验证与限制

| 后端 | 本轮可验证的行为 | 尚不能据此声称的能力 |
|---|---|---|
| Codex Pro | 精确公开插件 0.8.0；outer v1 fresh／v2 persistent；显式 threadId；跨进程恢复；独立 request/run/turn；仅本次 run 增量记账 | 任意崩溃点恢复、早期持久化 ack、后端 exactly-once、真实账号授权与费用 |
| DSH | 实际公开 SDK、AgentLoop 与 JSONL persistence；新进程 `session/resume`；保留原历史且只新增一个 turn；当前调用输出和用量；私有策略回执 | 把自造事件写进原生历史、根据错误文本猜测缺失历史、未知 turn 的安全重放 |
| Kimi | 配置和 argv、显式会话 ID、当前调用 resume hint 与 cwd 校验、清理和未知用量分离 | 由 CLI 未提供的 run/turn ID 或 usage；把进程退出当成任意远端结果已知 |

DSH 策略证明使用与请求、session、nonce 绑定的私有文件，不再写入破坏公开 SDK 重放的
自定义历史事件。worker 禁用自主调度／委派及未纳入当前计量的辅助模型调用。测试中的
模型适配器与命令响应是离线桩，SDK、会话持久化、worker 和进程边界使用实际代码。

`.sessions` 是 Go 的恢复证据，不是第二份状态机；文件最大 32 MiB，读取拒绝符号链接、
FIFO 和非法身份。该有界槽不代替后端历史的保留策略。本版本不自动删除 Codex／DSH／Kimi
的原生历史，也不宣称子进程退出会删除历史。运行配置路径引用的身份不证明同路径内容或
账号从未改变；这种没有公开可核验身份的变化仍是明确限制。

## Review 中复现并修复的问题

| 问题 | 修复与回归证据 |
|---|---|
| 恢复请求在启动前失败，被错判成未知执行 | 对相关联且确认未发送／已清理的失败保留原检查点；空 ID 和精确回显 ID 均覆盖，错误非空 ID 仍拒绝 |
| 收到回执后 CAS 失败导致再调用或再扣预算 | 故障注入覆盖 Plan、Execute、Review、Judge、panel 及报告收集后窗口；恢复时不重新发模型请求 |
| 报告已从 worktree 收走，再恢复时误报缺失 | 以原回执报告为准；仅在恢复已确认结果时容许已收集的临时报告不在原位置 |
| 已完成回执恢复仍依赖当前 secret | 缺失 secret 不改变已声明的凭据名称引用；不修改实际 child env；四个角色的真实步骤恢复回归覆盖 |
| 原始保护证据被重启后的状态替代 | 原始 before／after guard 和完整候选、契约、验证输入绑定；非法 Plan／Review／Judge 写入不能变成有效通过 |
| Full 抽检或 Quick final gate 在恢复时重新生成 | 保留原始检查与时间身份；Full 两单元抽检只执行原有四次检查，原 Review 不重复派发 |
| 里程碑历史忽略其 packet 的目标变化 | Plan／Judge／panel 的 goal identity 纳入里程碑及所属 packet 的明确目标字段；15 个目标变化子用例先红后绿，验收修订对照仍可续接 |
| DSH 损坏／截断结果流保留不可信用量 | 无效 stream、关联身份和截断清除 usage／cost；完整相关回执的用量与不安全的外层退出分别记录 |
| DSH 成功回执加异常 exit 仍保持可恢复 | 使用协议结果与物理退出分别校验，异常组合阻断重放 |
| 取消、CAS 或缺失候选证明使已发生调用漏账 | 在恢复入口独立处理可信回执的可归属计量；保持未知执行阻断和原始 guard 校验；本地未派发取消不虚增未知用量 |

主要回归在 [sessions_test.go](../../internal/machine/sessions_test.go)、
[session_workflow_test.go](../../internal/machine/session_workflow_test.go)、
[session_fault_test.go](../../internal/machine/session_fault_test.go)、
[session_guard_test.go](../../internal/machine/session_guard_test.go) 与
[session_audit_test.go](../../internal/machine/session_audit_test.go)。状态、schema、配置、
诊断、CLI、各后端和 Node worker 另有各自的行为与故障测试。

## 验证状态

执行环境：Linux amd64，Go **1.27.0**，Node **24.22.1**，npm **12.2.1**。

| 检查 | 本轮结果 |
|---|---|
| Go 定向行为与故障测试 | 已通过；最终审查修复后的整仓门禁待下列记录更新 |
| `make quality-gate` | 最终门禁进行中；不会把中间轮次的覆盖率当成完成 |
| CI 相同的七个核心包 `go test -race` | 最终候选待执行 |
| DSH `npm run check` | 145 个主套件测试及 8 个隔离旧宿主测试通过 |
| Go → Node → DSH／Codex Pro | 合并桥接命令通过，含公开 SDK 的真实持久化续接 |
| 根目录 `npm test` | 126／126 通过 |
| 六包构建与同一候选真实安装 | 等待最终源码冻结后执行 |
| 文档、差异与远端 tree | 提交时核对本地暂存 tree 与 GitHub 返回 tree 一致；最终链接与差异检查待补全 |

## 远端与未验证项

中间检查点 `dbb5a455fedf7aac25790a01ef3e3895e0221fc8` 的
[GitHub Actions run 42388417156](https://github.com/ckanner/flywheel/actions/runs/42388417156)
在约三秒内失败：npm-candidate 没有 runner 名称、steps 为空，后续 test skipped；
日志读取为 404 BlobNotFound。不能据此声称代码测试失败的具体原因，也不能声称远端
Linux／macOS CI 已通过。最终 PR 的 CI 状态另行核对。

没有运行真实 ChatGPT／Kimi／DeepSeek 账号推理、Mac 桌面／Seatbelt 验收或成对缓存效果
实验；不报告虚构的节省比例。没有发布 npm、提升 latest、修改用户已有安装或登录账号。
P3 仍需要上游新增公开、可等待的 typed lifecycle、派发前持久化确认和只读 recovery
契约；现有文本 progress 和 run 回执不能替代它们。P4 需要真实任务与账号测量。
