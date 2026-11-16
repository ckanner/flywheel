# 2026-11-16：会话活性与未知规划调用退役实现验证

[验证目录](README.md) · [设计契约](../design.md) · [会话恢复](../zh-CN/session-recovery.md) · [工作台](../zh-CN/flow-console.md)

## 来源与交付范围

- 基线：`ckanner/flywheel` main，`85c1b2dcad6e28c44438c5f5e3cf71a890d17c2c`。
- 分支：`fix-session-liveness-recovery`；PR 链接在创建后补充。
- 环境：macOS arm64、Go `1.27.0`、Node `26.12.1`。未修改真实 DSH profile、账号、凭据或用户配置。
- 本记录对应代码与离线验证，不声称 npm 发布、真实桌面 UI 或真实模型调用已验收。

## 实现内容

| 层 | 代码与行为 |
| --- | --- |
| 看门狗 | `watchdog.roles.<role>` 覆盖 idle/wall；规划角色默认 900 秒，执行/评审保持 300 秒；收尾宽限不变 |
| 规划提示词 | 非 native harness 明确禁止后台/异步子代理，调查必须前台完成；Kimi native/profile 仍可观察子任务 |
| 状态 | session ledger 增加 `abandoned` 状态和 `abandon` 对象；保留 unknown dispatch/outcome、原因、来源和旧 thread 证据 |
| 引擎 | 仅规划角色的 blocked、已计量、清理确认调用可退役；不应用结果、不重放、不复用旧 thread；关联 request 标为 cancelled |
| 人工边界 | 模型工具不暴露退役；DSH 用户命令证明或控制终端确认精确 invocation；CLI 提供 `session --abandon` |
| 控制回执 | `session-control.applied/status/message/invocation` 明确异步 session 命令的语义结果；信封 `ok` 不再被误读为退役成功 |
| 视图 | 规划面板显示未解开调用、准确 invocation 与原因，提供原因和确认框；退役后显示会话结果并允许显式 Plan |
| 文档与技能 | 设计、恢复、配置、工作台、使用手册、原生能力、协议和中英技能同步说明 |

## 关键验证情形

1. **误杀防护**：配置解析为规划角色选择 900 秒 idle，执行角色保持全局值；部分覆盖只声明 wall 时仍继承角色默认 idle；未知角色/非法值在派发前拒绝。
2. **提示词边界**：非 native 规划提示词包含前台调查约束；native profile 选项不注入该约束；现有规划/审查 JSON 协议不变。
3. **状态与 schema**：合法 abandoned 记录可原子保存/重载；缺 proof、非 human、空原因、未计量、清理未知、已知 dispatch+outcome、可恢复 thread、未知字段均在派发前拒绝。
4. **引擎退役**：成功路径保留 unknown 事实、清空可恢复 thread、取消关联 request、任务回到 paused、写 inbox/orchestrator log；无 proof、错误状态、未计量、清理未知、非规划角色、人工接管、缺失 session、错误 key 均拒绝。
5. **CLI/DSH 人工证明**：控制路径只接受匹配 key/revision/action/reason 的 host 证明；缺失、伪造、过期或 env 继承均失败；控制终端要求准确 invocation 确认；模型工具不可用。
6. **控制回执**：abandon/reconcile 返回 applied/status/message；`ok:true` 但记录仍 blocked 时 applied=false；快速 session 命令等待子进程退出后才生成回执。
7. **视图**：blocked 规划调用显示 invocation 和原因；原因/确认缺一不可；真实用户命令提交后响应被采用，退役记录从阻断面板消失。
8. **回归**：原有 planning、session、状态、CLI、插件客户端和 legacy host 测试保持通过；新增测试覆盖所有新分支。

## 检查记录

| 检查 | 结果 |
| --- | --- |
| `go build ./...` | 通过 |
| `go test ./internal/config ./internal/state ./internal/contract ./internal/prompts ./internal/cli` | 通过 |
| `go vet ./...` / `gofmt -l` | 通过；无诊断或未格式化文件 |
| `make coverage` | 待最终运行记录 |
| `npm --prefix plugins/dsh test` | 428 个插件测试与 10 个 rc5/legacy host 测试全部通过，0 失败/跳过（最终 UI 守卫后又跑了 43 个直接相关测试，0 失败） |
| 真实模型/桌面/发布验收 | 未运行；离线 fixture 不等于真实账号或桌面验证 |

## 限制

- 本次没有真实 Kimi/DSH 设计调用、真实桌面按钮或 npm 安装包验收；真实模型仍可能长时间无输出，role idle 只是策略上限，不是活性证明。
- 快速 session 命令的等待测试使用本地 fake orchestrator；真实引擎启动延迟和状态保存仍需桌面场景复核。
- 退役只适用于规划角色；packet 角色继续使用人工接管/原调用对账路径，不因本次变更获得新的通用放弃动作。
