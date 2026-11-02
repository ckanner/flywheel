# 模型 profiles 与安装预检：验证记录

日期：2026-11-01。基线：已合并的 `1c515ca0c694047d1f6c7d7c3d52853cc9bf9c2d`，实施分支 `feat/model-profiles-and-preflight`。本文保存本轮提交前的本地验收证据；远端双平台 CI 和合并结果以该 PR 的 Checks／合并记录为准，不由本地结果推断。源码合并不等于 npm 发布。

设计见 [v0.26](../design.md)，范围见 [P01–P05 计划](../plans/2026-11-01-model-profiles-preflight.md)，使用见[配置指南](../configuration.md)与[Mac 安装教程](../getting-started.md)。前轮 DSH 集成与持久化行为的历史记录保留在[原验证记录](2026-11-01-results.md)。

## 1. 最终本地门禁

运行环境为 Linux amd64、Go 1.27.3、Node.js 24.22.1。所有检查使用本地 Git、真实进程、DOM 或模拟协议，不调用付费模型；没有修改外部插件仓库或用户 Mac。

| 检查 | 结果 | 证据 |
|---|---|---|
| `make quality-gate` | PASS：vet、gofmt、全部 Go 测试 | [完整输出](evidence/profiles-quality-gate.txt) |
| 逐函数覆盖门禁 | **410 个函数全部 100%；6,036 个 statement，未覆盖 0** | [逐函数输出](evidence/profiles-function-coverage.txt) |
| 核心七包 `go test -race` | PASS：machine、verify、watchdog、drivers、cli、modules、diagnostic | [输出](evidence/profiles-race.txt) |
| DSH 插件 `npm run check` | PASS：**35/35**，含真实 React DOM 与进程取消回归 | [输出](evidence/profiles-dsh-plugin-tests.txt) |
| Go → Node worker → fake SDK | PASS：provider 独立、doctor 权限／目录／坏帧／关闭边界 | [输出](evidence/profiles-dsh-bridge.txt) |
| 真实 Node control → 新 Go 二进制 | PASS：4 场景，符号链接／空格／特殊字面字符路径与诊断失败 | [机器记录](evidence/profiles-control-smoke.json) |
| skills | 内嵌 YAML／字段约束、模板输出和 CLI 集成测试通过 | 已纳入 Go 门禁 |

CLI 新增真实子进程信号测试：config inspect、doctor、control config、control doctor 收到 SIGTERM 后，等待受管命令执行 TERM cleanup 并退出，再返回失败；不是信号直接杀死 CLI，也没有创建 state。Node 控制测试另启动独立 worker 进程组，用超过六秒的实际清理过程证明 host 不会沿用旧 500ms／不足六秒的宽限过早杀死 Go 所有者；忽略 TERM 的诊断进程仍在 20 秒后强制终止，迟到成功不覆盖取消。

测试环境的执行身份为 root，已有 chmod 故障注入需要去掉 DAC 绕过能力。因此本地使用 `setpriv --bounding-set=-dac_override,-dac_read_search --inh-caps=-all --ambient-caps=-all` 启动 Go 测试。没有删除原失败断言或放松覆盖率，普通 CI 用户不需要这层环境适配。

100% 是本项目的逐函数 Go **statement coverage**，不是分支覆盖、JavaScript 覆盖率或真实模型质量的证明。新增断言检查可见行为、错误传播、隔离和收尾，不只检查配置拼接。

## 2. 本轮确认的行为

### 配置与实际调用

- profile 在加载时展开，引用只在同一完整文件内解析；阶段显式覆盖不污染其他阶段。
- harness/model/channel 的必需值、未知引用、错拼字段、非法 role、错误类型与 provider 冲突在调用前拒绝；已有 inline 缺省及合法零值保留。
- 配置优先级不因坏 JSON、不可读或失效链接而回落，空 taskDir 不会误读进程所在目录的任务配置。
- Provider 在 Plan、Execute、Review、Judge、第二意见、Intake、Garden 全部贯通；每次后备渠道切换重新解析，跨到非 DSH 后端不残留 DSH Provider。
- metrics 保留账号 channel 与实际 provider 两个维度，unknown usage/cost 不变成零；渠道别名不被当作账号切换证明。

### 预检与桌面

- config inspect 与 doctor 使用只读配置加载；普通 status 的停滞判断也改用只读加载，避免查看进度时物化全局配置。
- Git 根发现、命令版本、SDK 探测共享有界上下文；预先取消不启动任何进程，中途取消不继续后续 SDK。
- task scope 只包含必需阶段与实际路由；取消仍保留完整正确的范围标记。无 packet 可做提前预检，但正式启动仍需合法 packet。
- probe 只确认 adapter 接受配置、目录、完整沙箱、审批策略和正常 SDK 收尾；明确断言零 `session/prompt`、零 `session/wait`，usage/cost 为 null。
- 配置错误、版本和 SDK 错误不回显原始字段值或凭据；账号授权、额度、实际模型目录与推理能力保持 unknown。
- 面板复用 Go 控制协议；合法 doctor 失败可逐项查看，错误身份／协议／超时继续拒绝。路径切换后迟到成功、失败和 finally 不污染新视图。
- 所有 packet 可见，完成／停止／暂停／升级与未知成本分别呈现，原始 JSON 默认折叠。

## 3. 自审与文档

本轮 review 修复并保留回归的具体问题：

1. 新预检的 Git 根发现最初未受超时约束，现统一使用 watchdog/context。
2. 诊断被取消时，Go 默认信号退出与 Node 500ms 强杀可能截断清理；现由 CLI signal context 和 host 20 秒收尾窗口共同处理。
3. 取消后未遍历的绑定可能误保留 `inScope:true`，现先计算全体范围。
4. 旧 status 读取可能创建全局配置，现使用 LoadReadOnly。
5. 严格配置解析保留旧 `invalid JSON` 错误前缀；原测试中把 harness 名写进 drivers 的 fixture 改为明确 real，仍验证缺环境变量时 fail-loud AUTH。

[PR #14 的首轮 CI](https://github.com/ckanner/flywheel/actions/runs/41369336819) 在 Ubuntu 完整通过；macOS 暴露了新 CLI fixture 的路径假设：测试期望 `/var/...`，而控制协议正确返回解析后的 `/private/var/...`。后续修复只将 fixture 的期望路径规范化，并新增 `TestDiagnosticCLIUsesPhysicalTaskIdentity`：通过真实仓库符号链接调用 config／doctor，检查响应中的 taskDir、cwd 和嵌套诊断身份一致且不创建 state。生产路径语义、失败断言和覆盖率门禁保持不变。修复后的定向回归和完整 `make quality-gate` 再次通过，独立只读复审未发现阻断问题。最终双平台结果以 PR 最新提交的 Checks 为准。

README、设计、Mac 教程、配置指南、插件 README／PROTOCOL 和 new/run/status 三项 skills 已同步。skills 仍是校验、CLI 调用和结果解释，不实现第二套调度、重试或状态写入。

外部 Codex Pro 交接文档重新固定到 `21fd7ac...`／0.7.0，区分已经修复的启动 deadline、内部 usage／receipt，以及仍阻断公开消费的有效配置、权限、完整收据和进程所有权问题。该外仓只读审计，没有在 Flywheel 内通过非公开 import 或嵌套父模型冒充接入；独立 Codex CLI 路径继续可用。

## 4. 仍需实机证据

当前实现可以离线构建和测试；以下事实未在本轮用户 Mac 上确认：DSH 实际桌面安装／渲染、Seatbelt 成功探针、GUI PATH 与账号可见性、真实 Flash/GPT/Kimi/Codex 响应、订阅限额和中断恢复。React DOM、交叉编译和 macOS CI 不能替代这些证据。

没有本轮真实模型性能／价格测量，所以不声称节省百分比或模型质量排名。SDK 初始化可能访问模型目录并写入 SDK 自己的会话／缓存；“只读预检”指不修改 Flywheel 配置和任务状态，不表示整个文件系统绝对无写入。
