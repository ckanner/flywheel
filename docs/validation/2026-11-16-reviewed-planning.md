# 2026-11-16：受审查规划生命周期实现与验证

[验证目录](README.md) · [设计契约](../design.md) · [方案](../plans/2026-11-16-reviewed-planning.md) · [使用指南](../zh-CN/reviewed-planning.md)

## 来源与交付范围

- 基线：`ckanner/flywheel` main，`c5cc9930ea60f63cd28488025e84c078f712f992`。
- 原方案提交：`32438be391d0c255fd8e8ee7f7ef84da852031b5`。
- 分支：`docs/reviewed-planning-native`；[PR #52](https://github.com/ckanner/flywheel/pull/52)。
- 用户批准方案后，在同一个工作会话内继续实现。此记录对应该 PR 中的实现与离线验证，
  不改变历史版本的发行记录。包版本仍为 `0.34.6`，新行为列在 CHANGELOG 的 Unreleased。
- 环境：Linux、Go `1.27.4`、Node `24.22.1`。未修改真实 DSH profile、账号或凭据。

## 实现内容

`new` 现在登记薄 brief，由 Go 调用整体设计与独立 Design Review；默认停在人工审批。
`plan` 明确负责方案设计、讨论后的修订或范围内子计划细化。显式 Plan 一律只做规划，
即使任务已有启动授权也不会顺便执行。`task` 保留原始明确授权，delegated 与人工批准
分别记录。普通 Run 不能代替用户批准或给尚未启动的版本补一份启动授权。

| 层 | 代码与行为 |
| --- | --- |
| 纯规划模型 | `internal/planning`；设计、审查、发现、反馈、精确批准、请求轮次、激活引用与追加历史 |
| 状态与契约 | `state.planning.version:1`、session ledger 4、Spec 的 `planning-ref`；状态 CAS 与历史完整性检查 |
| Go 生命周期 | brief-only 初始化、整体设计/审查、范围内 Plan/Plan Review、滚动细化、暂停与恢复 |
| 激活事务 | 完整文件映射、原文件哈希、intent → committed → synced → active；原事务恢复后才准入实施 |
| 控制协议 | `control planning` 的 prepare / plan / feedback / approve / status / migrate，精确 revision 与稳定 request-id |
| DSH | 方案正文、审查、前一版本、获批根方案、子计划、反馈回执与独立批准/开始动作 |
| 原生角色 | design-plan、design-review、plan-review 复用现有 Provider 适配器；保留原生权限、能力、会话和生命周期 |
| 文档与技能 | 中英使用指南、配置/工作流/操作手册、协议和五个按钮技能同步更新 |

机器约束只判断可验证的边界：完整引用、需求去向、固定契约、写入范围、验收底线、
当前输入、独立调用、审查历史和激活事务。设计是否合理仍交给独立 reviewer 判断。
模型不能通过省略上一轮 blocker/major 或把它降为 minor 而获得通过；关闭必须保留
相同 finding ID 及解决证据。模型输出也不能填写自己的批准、版本或真实调用身份。

## 关键验证情形

真实临时 Git 仓库和持久状态用于验证以下行为；模型内容使用明确的离线 fixture。

1. **完整主流程**：从没有 Spec/packet 的 brief 开始，设计、独立审查、等待审批、
   只批准、再开始、独立子计划审查、真实 shell 验收、代码 Review 和 Git 交付。
2. **版本与讨论**：批准绑定精确 revision/digest；新反馈使旧根方案授权失效；旧版本的
   Start 不会让新版本的 approve-only 自动付费细化或执行。范围内 Plan 的 text 保留
   在该 request 中，多轮修改状态说明也不会丢失原问题。
3. **有界循环**：每请求最多三轮；格式/传输重试、实际费用和次数持久化；重启不刷新
   额度。新角色只有显式渠道模型映射才可跨渠道 fallback。
4. **原生生命周期**：独立角色会话、真实回执恢复、未知清理拒绝再次派发、原调用计费
   一次、收到回执后才采用反馈，保持既有 Provider 失败分类。
5. **排队反馈**：queued 与 applied/rejected 可区分；原请求 ID 可查终态；回执保存或
   队列删除中断后可恢复；拒绝过的 ID 不能被换内容重用。
6. **滚动规划**：后续里程碑可 deferred，轮到时细化；已完成且未受影响的 packet 保持
   原指纹和结果。Quick 保留其实施快路径，新增设计调用不冒充两次调用完成整个任务。
7. **文件与恢复**：完整投影、路径越界/符号链接/特殊文件、未知导出、外部变更、
   文件写入/同步失败、Git 提交后 CAS 中断、worktree 同步均有拒绝或恢复回归。
8. **调查接续**：原报告、来源、用量与限制进入设计上下文，费用只导入一次；只调查的
   report 不自动变成修复授权，后续精确人工 Start 绑定该报告和新方案。
9. **旧任务**：明确迁移才加入新协议；保存真实原契约哈希，不补造过去的规划、审查或
   批准。旧引擎不能忽略 planning-ref/session ledger 4 继续实施。
10. **人机控制**：模型工具不能 approve；DSH 核对真实、未消费、非 fork 继承的用户
    command 事件；CLI 从交互终端显示权威设计与审查并确认精确 digest。释放已知安全
    的 handoff 可以恢复控制面，而未知调用仍阻止释放或继续。
11. **大方案显示**：256 版合法历史超过 1 MiB 时仍能得到有界快照；规划分区按实际
    JSON 编码限制为 512 KiB，保留精确引用和审批/Start 依据。省略当前设计或审查
    内容会禁用 UI 批准；只读摘要不能写回权威状态，也不共享可变 map/slice。

## 检查记录

本节记录提交前最终工作树的实际检查结果。GitHub 的 Linux/macOS CI 以 PR 当前 head
的 checks 为准；本地离线通过不代替远端矩阵或真实账号验收。

| 检查 | 结果 |
| --- | --- |
| `go build ./...` | 通过 |
| 新规划模型、state、contract 包测试 | 通过；逐函数 100% 语句覆盖率 |
| 新规划引擎回归 | 通过；包含主流程、逐步细化、12 组子计划期间反馈、审批/Start、恢复和投影边界 |
| `make quality-gate` | 最终全量通过：vet、gofmt、全部 Go 测试与 998 个计量函数各 100% 语句覆盖率；machine 包 203.994 秒 |
| `npm --prefix plugins/dsh run check` | 最终完整构建与检查通过；423 个插件测试、10 个 rc5 兼容测试，0 失败/跳过 |
| Go → Node → 实际公开适配器离线桥接 | DSH、Codex Pro、Claude Code Pro、Kimi 全部通过；43.092 秒，未跳过可选 Claude 后端 |
| 根目录 `npm test` | 166 项全部通过；34.489 秒，0 失败/跳过 |
| Linux race | machine、verify、watchdog、drivers、cli、modules、diagnostic 七包全部通过（machine 287.574 秒）；最后新增的只读投影定点 race 也通过（32.731 秒） |
| 文档与最终差异 | 30 个变更 Markdown 的 551 个相对链接/锚点、围栏和 6 个新指南/技能 JSON 示例通过；`git diff --check` 通过 |
| 真实模型/桌面账号 | 未运行；离线通过不证明在线质量、性能或账号可用性 |
| npm 发布/桌面安装 | 本 PR 未发布或安装 |

最初并行运行重型检查时，Claude 桥接的两个进程 fixture 以及根目录 npm 的两个
限时场景曾超时。其他检查结束后，使用相同命令、原超时和相同依赖独立复跑，上表
两个完整检查均通过。没有删减测试、跳过后端或放宽门禁。初次全量 Go 检查也发现
一个 diagnostic fixture 仍在清空必须保留的当前计划引用；已改用真实反馈/审批转移，
并修复 doctor 按当前输入选择规划角色的行为，再纳入最终全量门禁。

适配器来自仓库锁定的公开包/已审查源码包，没有以代写版本号冒充兼容：

- Codex Pro `0.10.0`：源码 `29b388a286748740b8bbbcbcdf0e5bf02038befa`，
  tarball SHA256 `d5437a2a1a125c05f6fe61afcae4c822f2bca94bc11cc5e15c45f9137cc74a61`。
- Claude Code Pro `0.5.1`：源码 `412904d3120895c30c2d75fd47216e2d663a64c6`，
  tarball SHA256 `5e12a708078729a85449ea72094bd7c50e57604fa5035c3aa0384c42ccfc884c`。
  `vendor/claude-code-pro.json` 仍标记 `registryReleaseVerified:false`，本次不改变该事实。
- DSH/Kimi 桥接复用仓库既有公开接口与原生协议 fixture；本次未变更依赖版本。

## 复验与后续会话上下文

接手时先确认 PR 的最新 head/base 和本地修改，保留并发工作，不重做或清空账本。读取
[原方案](../plans/2026-11-16-reviewed-planning.md)、[当前设计](../design.md)、
[使用指南](../zh-CN/reviewed-planning.md) 及本文后，再按最终差异选取验证。

```bash
go build ./...
FLYWHEEL_TEST_TIMEOUT=15m make quality-gate
npm --prefix plugins/dsh run check
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(DSH|CodexPro|ClaudeCodePro|KimiNative)NodeWorkerOfflineBridge$' -count=1
npm test
go test -race ./internal/machine ./internal/verify ./internal/watchdog ./internal/drivers ./internal/cli ./internal/modules ./internal/diagnostic -count=1
```

Go 门禁仍要求每个计量函数 100% 语句覆盖率，不能用四舍五入后的总覆盖率代替。
检查应避免让多个重型编译/跨进程 fixture 同时竞争资源；保留原有测试与超时配置。

后续人工 review 的重点是：New/Plan 的停止位置是否符合预期、独立审查发现是否足以
支持讨论、方案批准与开始实施的分离、现有 Provider 配置继承是否符合用户选择。
源码 review、CI、候选包安装、真实模型冒烟与正式发行是分别记录的阶段。用户当前授权
是完成实现并更新 PR；合并、打 tag、发布和修改真实桌面配置需要对应的后续指令。
