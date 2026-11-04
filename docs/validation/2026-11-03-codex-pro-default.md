# Codex Pro 默认接入验证记录

## 1. 固定基线与实际接入

Flywheel 以 `cb7d996eb233565f31d6804b3c6963a18cdb0d93` 为基线，发行目标 0.28.0。上游核验对象为 [Codex Pro v0.8.0](https://github.com/ckanner/dsh-subagent-codex-pro/releases/tag/v0.8.0)，main `04ecdb677735d84f3a0f5dba01a9d9b28c324c0e`。

本轮于 2026-11-04 完成复核；上游 PR #1–4 已合并，0.8.0 已发布，无需重复提交同一问题。其 [main CI](https://github.com/ckanner/dsh-subagent-codex-pro/actions/runs/41647110921) 的 Ubuntu／macOS × Node 22／24 四个 lane 均为 183 total、177 pass、6 skip、0 fail，并通过 packed install 的公开 machine API／CLI／guardian 冒烟。这是上游证据，不替代 Flywheel 的 CI。

DSH 兼容性审查固定在 `deepseek-ai/deepseek-harness` 的 `b830ec950faba0a46dda564be3e2d854afed94f1`：读取了 SDK protocol、SDK server README、subagent types 与 manager。该基线的 Host subagent 依赖真实父 Agent，SDK 未提供外部 subagent start RPC；因此采用插件公开 machine API，而非构造父会话。这里的 rc.5 是兼容基线，不宣称是所有发行渠道的最新版本。

已直接读取 npm 发布包，核对包名、版本、Node >=22.18.0、公开 `./machine` 和 `./package.json` exports。npm SHA-512 integrity 为：

```text
sha512-hbWPMkjWPsY8DDhcUIzBBCNR3TZexeOSSv9yP1zzIiyP2NvHfedfNkHz2ubreUyyonst+/539t7izwzl3ZOHxA==
```

集成实际导入这个包的公开 `runMachine`，不是复制 runner、调用旧 `codex exec` 或伪造 DSH 父会话。默认模板和 profiles 示例的 hard 都是 `codex-pro`；旧配置不自动覆盖。

## 2. 验收层次

| 层次 | 验证内容 |
|---|---|
| Go 单元／协议 | 配置与 profiles、请求身份、完整 receipt、模型／目录／权限、usage、错误映射、取消与物理终态；逐函数 100% statement coverage |
| Node worker | 实际导入固定 npm 插件，使用 fake app-server 执行真实 machine／guardian 代码；输入错误、超限、EOF、信号、输出断开和清理 |
| Go→Node→插件 | 真实父子进程管道，固定插件 runner／guardian 与脚本化 app-server；同一调用请求、返回、预算和终态验证 |
| npm 候选 | 六包构建一次，在隔离 registry／HOME／DSH 协议桩中安装；取得真实 Codex Pro tarball，验证公开 API、CLI、worker、10 个 skills 和 GUI 入口 |
| CI／发行 | Linux/macOS 消费同一候选，核心 race；发行另核精确 tag／source／integrity、registry 安装和 latest |
| 用户 Mac／账号 | 本环境未提供真实用户桌面与订阅；不据离线结果声称这层已通过 |

## 3. 发现并处理的关键问题

- **旧默认未消费插件：**新增独立 harness 和实际 machine API worker，npm 精确依赖与启动器解析；没有插件时拒绝，不自动使用旧 CLI。
- **参数与计量证据：**effort 仅 accepted、effectiveEffort 为 null；实际 provider 未提供则未知；完整 receipt 留存，partial usage 不当成完整预算或零费用。
- **两层进程收尾：**模型输出不证明执行结束；正常 app-server 收尾可以有终止信号，外层 worker 仍须可靠退出。真实 EOF、取消、父进程／worker 死亡用 PID／进程组消失验证，不只看文本日志。
- **清理未知后误重试：**实际 SIGKILL worker 时，Go 可在几毫秒内返回，但独立 guardian／app-server 此后仍存活。driver 现将缺失可信回执、执行身份不符、终态后坏 JSON／重复 frame 等情况标记为清理未确认；Plan／Execute／Review／Judge／Intake／Garden 均停止本次自动执行、保存升级与审计、保留 worktree，禁止重试、fallback、Plan 冷启动及后续候选。已有可信清理回执的普通服务错误仍沿原策略重试。此修复不声称 caller 死亡时已经同步确认所有进程退出。
- **模块状态所有权窗口：**只检查 PID 会漏掉主引擎已持 `.run.lock`、尚未写 PID 的启动窗口。预算记账与清理升级共用持锁的 Load／CAS Save；真实占锁回归确认主 state 字节不变，模块独立 inbox／metrics 仍保留。
- **全局停止提前截断：**原 SIGTERM kill switch 的默认 5 秒会绕过 worker 自己的 10 秒宽限。改为登记并尊重各子进程宽限，保留快照所有权与最终强杀／等待。
- **完整策略时点：**用实际 0.8.0 包和故意回报宽 roots／network 的 app-server 复现：插件会先发 turn，完整策略到最终 receipt 才交给调用者。因此 Flywheel 的完整策略校验只是交付资格检查；文档和 doctor 明确警告，不宣传为事前隔离。
- **npm 12 安装夹具：**真实安装首次触发 `EALLOWREMOTE`。已将直接 URL `npm pack` 改为从官方 registry 获取精确的 `dsh-subagent-codex-pro@0.8.0`，仍强制校验锁文件 resolved、tarball SHA-512、manifest 名称／版本及依赖图。没有放宽 `allow-remote`；修复后完整安装重跑通过。规则依据见 [npm 12 配置](https://docs.npmjs.com/cli/v12/using-npm/config/#allow-remote)。

## 4. 本轮执行记录

### 本地门禁

环境：Linux amd64，Go 1.27.3，Node 24.22.1，npm 12.2.1。所有结果为本轮实际执行，没有调用付费模型推理。

| 命令／检查 | 实际结果 |
|---|---|
| `make quality-gate` | 通过 vet、gofmt 与全部 Go 测试；439 个受测函数逐函数 100% statement coverage，单行 cmd 入口按项目既定规则排除 |
| `go test -race ./internal/machine ./internal/verify ./internal/watchdog ./internal/drivers ./internal/cli ./internal/modules ./internal/diagnostic -count=1` | 七个核心包全部通过；最后修改 modules lease 后，又单独重跑该包 race 通过 |
| `npm --prefix plugins/dsh run check` | 61 tests，61 pass，0 fail，0 skip |
| `npm test` | 78 tests，78 pass，0 fail，0 skip |
| `FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(DSH\|CodexPro)NodeWorkerOfflineBridge$' -count=1 -v` | DSH 与 Codex Pro 两组通过，含真实插件 guardian、取消、worker SIGKILL、Go SIGKILL、策略与用量校验 |
| `npm run build:npm` | 六个同版本 0.28.0 候选构建及 integrity 检查通过，四平台二进制完成交叉构建；不是 macOS 运行验收 |
| `FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install` | npm 12 夹具修复后，7 tests，7 pass，0 fail，0 skip；包含安装后实际运行公开插件 API、进程清理、重复安装、保留自定义配置、CLI 取消和失败恢复 |
| 文档／发布包 | 新 `docs/workflow.md` 已进入 meta tarball；相关文档的 62 个相对文件链接及本地 Markdown 锚点有效；`git diff --check` 通过 |

六包使用同一次冻结源码快照构建，快照 SHA-256 为 `b51416adc3b13281f0af8299ad2540d433931f7148534dfcd9d25bc2603e7e7f`。这是含本轮修复的工作区快照，候选清单明确标注 `dirty: true`，仅用于本地验证，不具备发行资格。构建后更新的是未打入六包的安装测试夹具及本验证记录；安装测试消费原始六个 tarball，没有为通过测试重打包或替换真实插件依赖。

独立代码复核实际重现并关闭了清理未知误重试、模块状态所有权两个阻断项；额外核查了错误 run／thread／ownership、终态尾部损坏、已有 fatal 时的持久化，以及普通服务错误的正常 retry／fallback。当前没有剩余本地复核阻断项；这不是 GitHub 上的人类 approval。

### PR 与远端 CI

交付 PR 为 [Flywheel #23](https://github.com/ckanner/flywheel/pull/23)，保持 Draft、未合并、未发布。最终 head 与后续检查状态以 PR 的 Checks 为准。

首次 [PR CI run 41818659925](https://github.com/ckanner/flywheel/actions/runs/41818659925) 在 `npm-candidate` 启动前失败，没有 runner 或测试 step，依赖它的 test job 跳过。GitHub 页面明确提示账户近期付款失败或 Actions 支出上限需提高，应检查 Billing & plans；这不是代码测试失败，也不能算远端 CI 已通过。账户恢复后须对最终 head 重新运行 Linux／macOS CI，确认要求均满足后再考虑合并。没有修改账户付款、支出上限或仓库可见性。

## 5. 保留的能力边界

Codex Pro 0.8.0 没有公开的运行前完整 sandboxPolicy guard。workspace-write 允许 Codex 标准临时目录；用户／项目配置仍可能在调用前扩展权限，事后拒绝不能撤销访问。需要预防此类访问的环境须等待上游 guard 或另行采用已验证的宿主隔离，不能靠此回执检查声称具备该能力。

协议桩不证明真实模型质量、账号授权、订阅额度或美元账单。Mac 桌面 UI、Seatbelt、GUI 的 CLI／登录可见性、真实 Flash／GPT／Kimi／Codex 小任务以及本机重启恢复仍需在用户实际环境验证。缺少这些证据时不将其列为“完整实机测试通过”。
