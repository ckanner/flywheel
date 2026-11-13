# DSH 预热回执大小与 TTL 测试同步修复

## 范围与原始问题

基线为 `9ff7d704356bb8302a097e94dff924b95e7c2616`。本轮处理
[Issue #41](https://github.com/ckanner/flywheel/issues/41) 与
[Issue #42](https://github.com/ckanner/flywheel/issues/42)，用户授权修复并提交 PR。

Issue #41 是生产读写边界不一致：Node 将完整初始化错误写入私有终态回执，
Go 只接受不超过 8192 字节的文件。新增回归先在未修改的写入实现上执行：
真实 Go → Node worker → 离线 SDK 的 ASCII、多字节、JSON 转义场景分别生成
9221、40221、28221 字节回执，均被 Go 拒绝；直接写入测试还复现了恰好超出
上限一字节的 8193 字节文件。上述失败说明 SDK 已清理也不足以弥补格式超限。

Issue #42 是测试同步问题：原成功用例在启动子进程前设置 75ms 准备期限和
25ms 清理宽限，未确认夹具已安装可生成回执的关闭处理。Issue 报告 macOS ARM64、
Go 1.27.3 下重复 20 次全部失败。本轮 Linux 原用例重复 20 次通过（6.299 秒）；
该结果不否定 macOS 报告，也不能证明原测试没有启动时序依赖。

随后在独立的原基线 checkout 中，仅给启动测试二进制的 shell 增加 200ms 延迟，
保留原 75ms／25ms 设置和成功断言。该受控复现于 0.124 秒内失败，报告
`Signal:terminated`、`Settled:false`、`TRANSPORT` 和没有可信终态回执，与 Issue
描述一致。这个故意失败的复现只用于记录原因，没有进入修复分支，也不冒充 macOS 验证。

## 修复行为

- Node 在排他发布前按完整 JSON 的 UTF-8 字节数限定终态文件，包括转义和末尾换行。
  仅过长的 `error` 诊断保留前缀并加 ` [truncated]`；短错误和恰好符合上限的错误完整保留。
  算法只扫描可容纳的前缀，按 Unicode 码点计算实际 JSON 字节数，不拆开有效代理对。
- protocol、warmId、binding、workerPid、dispatched、cleanupConfirmed 保持完整。
  证明字段本身无法满足大小限制时拒绝发布；私有权限、排他发布和文件／目录同步保留。
  Go 生产代码、8192 字节读取上限、身份和进程组退出核对均未修改。
- 成功过期测试使用夹具自身发布的接纳标记，确认 prepare 已读入、关闭处理已就绪。
  该标记不是 `warm-ready`；测试明确断言 SDK 未就绪，再重新设置现有真实定时器。
  正式调用在过期前开始，验证同一 Request ID 只冷调用一次，并只获得剩余时限。
- 独立负例在子进程尚未读取 prepare 时触发实际准备期限：进程有界退出，但没有
  可信清理回执，仍保持未知结果、拒绝恢复和冷调用，不能伪造 no-process 证明。
- 跨语言回归使用真实 worker 与离线 JSON-RPC SDK；初始化错误仍是未派发失败，
  SDK 方法只有 initialize 和 shutdown。删除、损坏或将清理证明改为 false 后，
  Go 仍拒绝恢复。

## 验证结果

环境：Linux x86_64、Go 1.27.0、Node 24.22.1。全部模型与 SDK 响应使用离线夹具。

| 检查 | 结果 |
| --- | --- |
| 新增回执大小用例，修复前 | 预期失败，原始日志保留 |
| Go → Node → SDK 长错误回归，修复前 | 三类场景预期失败，Go 拒绝超限回执 |
| 直接回执边界用例，修复后 | 6/6 通过 |
| Node 预热专项完整测试 | 17/17 通过，无跳过 |
| Go → Node → SDK 长错误回归，修复后 | ASCII、多字节、JSON 转义场景通过 |
| TTL 同步与未接纳边界重复检查 | 两个用例各 20 次通过，共 40 次，10.916 秒 |
| 同上，启用 race | 两个用例各 20 次通过，共 40 次，27.789 秒，无 race 报告 |
| 完整 Go 质量门禁 | `make quality-gate` 通过，所有被测函数 100% 语句覆盖率 |
| 插件完整检查与旧宿主兼容 | 最终 `npm --prefix plugins/dsh run check` 退出 0；302/302 与 rc.5 10/10 通过，均无跳过 |
| Go → Node 公开桥接与 race | 最终 DSH、Codex Pro、Claude Code Pro 三后端均通过，57.232 秒，无跳过或 race 报告 |
| 独立代码复核与文档检查 | 独立复核未发现阻断项；文档相对链接、围栏和 `git diff --check` 通过 |

插件完整检查首轮为 301/302 通过。未修改的 Claude 后端用例
`stubborn: SIGTERM waits for the owned Claude group to stop` 在 6.254 秒后发现
`receipt.cleanup.confirmed:false`；此前的退出码 143、取消和失败结果断言均通过。
这不是该用例 15 秒测试期限到达，也没有证据将原因归为 CPU 竞争。该路径及其依赖、
夹具均未修改，且不导入本轮修改的 DSH 预热模块；原失败日志保留。在不改动代码、
超时或断言的情况下，该用例定向复测 1/1 通过（3.647 秒），随后完整复测 302/302
通过（213.621 秒），旧宿主 rc.5 兼容测试 10/10 通过（4.236 秒），均无跳过。
复测通过不替代首次失败记录；该次清理确认失败的具体原因尚未确定。

三后端 Go → Node 桥接的首轮 race 检查于 70.438 秒退出 1。Codex Pro、Claude
Code Pro 和本轮新增的三类长错误回归均通过；DSH 的既有公开 SDK 预热用例等待
ready 超过 3 秒，持久化历史用例触发 3 秒 idle watchdog，
`account-token-invalid-incomplete-tail` 则出现 `Cancelled:true`、缺失错误消息，
原日志未输出该项的具体取消原因。三项保持原代码、超时和断言定向复测全部通过
（13.334 秒），没有 race 报告。最终三后端完整复测结果见上表；本轮没有修改这些
失败路径，首次失败的具体原因仍未确定，不能仅以复测通过归因于环境。

## 验证边界

此环境不能执行 macOS 测试；Issue 中的 macOS 结果是问题报告证据，不是本轮执行结果。
后续 macOS 应对两个修复后的 TTL 用例重复运行，并执行完整质量门禁：

```sh
go test ./internal/drivers \
  -run '^TestDSHPrewarm(LocalTTLAndSafeColdFallbackBoundaries|DeadlineBeforeAdmissionDoesNotAuthorizeColdFallback)$' \
  -count=20 -timeout=2m
make quality-gate
```

修复只约束新发布的回执，不改写已有超限或损坏的历史文件。已有未知清理责任仍按
原规则阻断；诊断截断不能把初始化失败当作安全过期，也不能证明模型调用成功。
生产准备期限仍为 60 秒，默认预热关闭；没有延长生产时限、删除失败断言或放宽恢复。
本轮不涉及真实模型性能、桌面安装、npm 发行或用户配置修改。
