# JEV 可选影子评估验收

## 范围与身份

基线 `5d682291e39c46c4e5e7c275f3d195ffce7e5d6c`，分支 `codex/jev-workflow-shadow`；本轮为未提交源码。尚未发布 npm，也没有改动用户已安装的 0.30.0 或本机全局配置。完整设计见 [方案](../plans/2026-11-10-jev-shadow.md)，用户说明见 [指南](../jev-shadow.md)。

- 默认关闭、缺省关闭；Config schema、模板、全局／项目编辑器及只读 inspect 共用 `jev-shadow`。
- 开启只观察首次普通 run 的新任务；先保存原工作流，再最多一个原生 System One 请求。不改变实际模式、任务预算、权限或 Session。硬预算跳过；实验用量单列。
- 可选依赖的密钥、网络、模型版本、记录故障均不能阻断任务；用户取消仍停止原任务。旧任务恢复与手动单阶段不补发。
- 独立 prepare/run/report 支持审阅摘要、显式实验、离线核对。schema 严格校验、重复键和错误 UTF-8 拒绝、私有目录、意图先落盘、不可覆盖终态、未知不重放；输入和原始响应不会被当成主引擎命令。

## 本机确定性验证

- Go 全量 `make quality-gate` 最终通过：vet、格式检查、全部包测试、每个实测函数 100% 覆盖率；门禁未调整。最终日志 `/tmp/flywheel-jev-quality-complete.log`。
- `internal/shadow` 单独覆盖率 100%，`go test -race ./internal/shadow -count=1` 通过。
- 开发二进制的真实 `control config-edit/config-save` 在隔离临时配置中完成默认关闭 → 开启 → 关闭；每步由 `config inspect --json` 核对，未改用户配置。
- 新任务真实引擎 + loopback HTTP 测试：JEV 给 Full 建议时原 Quick 仍执行至 Review 前暂停；第二次启动无新增 JEV 请求。取消发生于影子 HTTP 时，引擎返回 ErrInterrupted，Execute 未运行。
- 开关关闭与恢复任务：无凭据读取、无网络、无结果目录。开启测试覆盖缺少密钥、无效目标、401、超时、摘要拒绝、记录路径错误及硬预算跳过；原状态逐字一致。
- JEV 回执覆盖完整成功、错误版本、坏 JSON、重复键、过大响应、重定向拒绝、未知 usage、并发争抢、残缺 intent、文件写入／fsync／发布故障、缓存身份不匹配及损坏记录；错误响应正文不入回执。
- 根目录 `npm test`：141 项通过，零跳过。
- `go test -race ./internal/machine -run 'TestOptionalShadow|TestAutomaticShadow' -count=1` 通过。
- DSH `npm --prefix plugins/dsh run check`：206 项主测试和 8 项旧宿主检查通过，零跳过。新增实际渲染交互测试确认默认关闭并发送布尔配置值。
- Go/Node DSH、Codex Pro、Claude Code Pro 三组离线桥接通过。Claude 测的是本地精确公开接口候选（0.4.0，本轮复核 tarball SHA-256 为 `125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`；来源和身份见 [vendor 记录](../../plugins/dsh/vendor/README.md)），不据此声称 registry 有新版本。未进行本轮原生桌面安装／UI 验收。

首次全量 Go 测试发现既有 `TestRunClaudeCodeProPhysicalFailuresAndNonDispatch` 在关闭 stdin 的 fixture 中返回 watchdog timeout，未返回预期 request write failed。fixture 使用 shell + 额外 sleep 子进程，输出管道和 TERM 清理可拖到 3 秒 deadline。将该用例独立提取并改为 `exec sleep`，保持单进程、原错误与 unknown 断言，新增 `TimedOut=false`；没有修改驱动时限或生产清理逻辑。独立回归连续 30 次通过（8.946 秒）。首次失败保留在 `/tmp/flywheel-jev-quality.log`，不以重跑覆盖原记录。

第二轮所有 Go 行为测试通过，但逐函数覆盖检查发现 `planUntilValid` 的保存失败出口未命中（98.4%）：新任务在更早的状态检查处退出，使旧的间接场景不再覆盖该边界。补充直接 CAS 冲突测试：在上下文快照后推进磁盘 revision，确认 Plan 保存失败、零模型派发、磁盘字节不变、内存进度为 failed。该测试已独立通过；没有删除防御分支或降低覆盖率门禁。第二轮日志保留在 `/tmp/flywheel-jev-quality-final.log`。

## MeRouter 真实调用

使用现有 `MEROUTER_API_KEY`，仅调用 `https://merouter.play.one2x.ai/v1/systemone`，模型 alias `jev`，响应固定 `jev-1.14.0`。未用 admin/service key，未改 MeRouter 服务配置。

[数据集](../evaluation/jev-historical-provisional.json)是六个已完成工作项的有界回溯摘要，带 Git 提交来源。没有声称它们是历史 Auto 决策：实际基线均 unknown，标签 provisional。标签、基线和来源不发送给模型。[精简结果](../evaluation/jev-historical-result-2026-11-10.json)记录数据／问题／策略摘要及概率；原私有回执在 `/tmp/flywheel-jev-integration-20261110/`。

| 项目 | 实测结果 |
|---|---|
| HTTP 调用 | 6 次，全部有效响应，未自动重试 |
| 建议 | 3 Full、3 abstain、0 Quick |
| Full 工作项 | 角色会话、lifecycle/recovery、交互式流程控制台 |
| 弃权工作项 | 标签排序、安装指南、桌面指南；检查或具体修改依据不足 |
| 已知 usage | input 7234、output 900；实际账单扣费未取证 |
| 本次延迟 | p50 498 ms，p95 1072 ms；六条串行样本，不代表生产 SLO |
| 无密钥同目录重读 | 6 个缓存结果，new-calls=0 |
| 生产校准 | false；无已确认真实标签，误放率／Quick 覆盖率保持 null |

初次数据文件把标签字段误写为 reviewer，schema 在网络调用前拒绝；改为 author 后开始六次调用。未把该校验错误算成模型请求。结果并不支持让 JEV 接管 Auto：小任务的摘要还会触发弃权，当前交付保持默认关闭、仅旁路观察。

## 待验证边界

正式发行和桌面安装不在本轮已完成项中。后续只有在用户选择开启后才会积累决策时样本；需要独立人工确认标签、评价漏判／误放／弃权与每完成任务的实际成本，再决定是否提议生产路由。不能从合成样本或本轮六条回溯材料推断成本、性能或质量收益。
