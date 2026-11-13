# JEV 总开关与取证决策实现验收

[验收索引](README.md) · [方案](../plans/2026-11-12-unified-jev-decisions.md) · [当前使用指南](../zh-CN/jev.md)

## 源码与范围

基线为 main `e035b5b5736c2d26da7ada2bf1ca97ee020fd5a2`，源码版本 0.31.1。
本记录验证从该基线起的统一 JEV 改动；提交身份以承载本记录的 PR 为准。没有修改
六包发行版本、Provider 默认模型、依赖锁、vendored 包或用户实际配置。

本次完成：唯一 `jev.enabled` 总开关；自动冻结调查策略；真实动作选择、依据证据
跳过冗余动作和提前进入报告；运行中关闭及策略收紧复核；旧影子自动路径退役；
受同一配置约束的显式在线评估；桌面、Task／New／Run 和双语文档同步。

## 环境和依赖身份

- Linux x86_64，Go 1.27.4，Node 24.22.1，npm 11.9.0。
- DSH 测试依赖从未修改的 package-lock 安装，为 `0.2.2-rc.2`；旧宿主兼容夹具为 `0.2.1-rc.5`。
- Codex Pro 使用既有锁定的 `0.9.0` vendor 包，未变更 integrity。
- Claude Code Pro 使用既有已审阅的 `0.4.0` 源码 tarball；45 个安装文件与 tarball
  逐字节比对一致，SHA256 为
  `125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`。
  这说明测试 peer 的身份，不声称本次完成 registry 发布。

## 执行结果

| 检查 | 结果 | 证据边界 |
| --- | --- | --- |
| `make quality-gate` | PASS | vet、gofmt 与全部 Go 测试；每个被测函数 100% 语句覆盖，保留既有 cmd 单行入口例外 |
| investigation / state / decision 的 `go test -race` | PASS | 调查选择、状态、journal 和恢复的并发检查 |
| CLI 决策与后台调查取消的 targeted `go test -race` | PASS | 开关、显式评估、进行中取消与子进程退出 |
| `npm test` | 142 PASS，0 skipped | 启动器、安装器、发行门禁和协议单测 |
| `npm --prefix plugins/dsh run check` | 256 PASS + 9 旧宿主测试 PASS，0 skipped | 插件构建、渲染、配置、控制与 adapter 行为 |
| 调查状态文案最后调整后的重建及 targeted tests | 12 PASS，0 skipped | “最近观察到的状态”、取证效果和异常快照显示 |
| 三个 Go → Node offline bridge 套件 | PASS，0 skipped | DSH、Codex Pro、Claude Code Pro 的真实跨进程协议桥接 |
| 文档围栏、相对链接、锚点与 `git diff --check` | PASS | 本次改动的当前文档与 skills，不重写历史结果 |

首轮完整 Go 行为测试通过，但逐函数门禁发现 `attachPreparation` 的 skipped 回执
分支缺少覆盖。补充“跳过的 JEV 请求不计费且恢复不重复记账”的真实调查移交回归后，
重新执行完整 `make quality-gate` 并通过，没有降低覆盖门禁或删除该有效分支。

## 核心行为证据

### 一个开关与完整作用域

测试验证默认关闭、显式开启、旧 `jev-shadow.enabled=true` 不会自动升级为在线
授权、新配置优先、编辑保存移除旧 key，以及 global on → project off → task on
的整文件优先级。真实 Git 根／子目录和相对 task 路径验证项目关闭不能被全局开启
覆盖。Task 等价的调查输入省略 `decision` 时，Start 仍自动取得正确采用策略；
旧的 per-contract 模式无法绕过关闭状态。

### 实际采用及减少动作

`TestMasterAutomaticallyEnablesEvidenceFinishAndStillAcceptsAnalystReport` 使用
真实 Go 调查循环及本地 HTTP fixture。原动作集有三项，JEV 先选择有效取证，随后
选择 finish：实际保留一项观察、两次 JEV 协议请求和引用证据的 finish 记录，进入
awaiting-report。重复 Run 不再次取证，分析报告仍可提交；缺少局限的缩短调查报告
被拒绝，过程不创建实现 completed 状态。

defer 的独立测试验证未观察到的假设不提供快捷选项，随后只跳过绑定的冗余动作。
成功但截断、失败、无输出、无关联假设或候选摘要改变的观察不能授权提前结束。
候选集合遵守 16 项协议上限；依据是实际观察，不是模型自行声明已证明根因。

### 关闭、取消和恢复

- 请求进行中关闭、最终候选检查时关闭，均不采用返回的建议。
- 当前数据范围、阈值、调用次数或固定目标收紧／改变时，旧策略停止可选决策。
- 关闭后仍核对旧 pending，保留未知用量；重新开启不清空累计调用预算。
- 控制效果 CAS 失败后，恢复读取原回执，未重复发送请求。
- 冻结 deadline 在配置发现前生效；最后策略检查期间取消后不保存新动作 Pending。
- 显式评估的进行中取消返回 143、保留 unknown 回执；以后读取同一回执返回 3，
  请求计数保持一次。配置发现本身也在 signal context 内。
- v0.31.1 原始 question hash 的固定回归验证旧成功回执仍可恢复并保留已知用量。
- 调查移交实现时，已知／未知用量按原派发日只导入一次；skipped 不作为付费或
  未知费用调用导入。原修复范围与验收规则保持有效。

### 桌面与入口

渲染和控制测试验证只有一个 `jev.enabled` 控件，不存在独立 shadow/adopt 开关、
expected-model 输入或第二次启用确认。展示使用真实已采用／基线记录、取证效果与
证据引用；持久状态标为“最近观察到的 JEV 状态”。异常配置或效果响应保留最后的
有效视图。Task、New、Run 保留各自职责并共用 Go 策略，skills 不实现循环。

## 复核和边界

并行独立复核发现并修正：默认 cwd 绕过项目关闭、提前结束阻止提交报告、策略
收紧未复核、最后策略读取期间取消仍可能写 Pending、旧 question hash 兼容和
过期观察可能授权快捷分支。完整门禁验证修正后的生产代码。

这里的 HTTP 仅为 loopback fixture，桥接测试使用离线协议桩。没有真实付费 JEV／
模型调用，没有证明模型正确率、净耗时或 token 成本改善。没有运行 npm 发布或
用户 Mac 桌面验收。候选输入的全路径前后摘要检查仍保留，未声称优化了该开销。
修复策略、重新规划与模型路由的 JEV 自动采用仍不属于当前接入动作族。
