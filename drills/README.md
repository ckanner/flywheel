# Flywheel drills（实证演练记录）

DESIGN.md §10 的实证门槛中可由编排器侧推进的部分。Go 版（v0.17 迁移后）演练由
`internal/` 下真实测试承载（真 git、真子进程，非 mock）。

| 演练 | 门槛 | 记录 | 可重复入口（Go 版，v0.18 迁移后） |
|---|---|---|---|
| 401/403 降级分类 | §10-4（可达部分） | [403-fallback.md](403-fallback.md) | 分类规则：`go test ./internal/errs -run 'TestClassifyError|TestRetryableAndFallbackSets'`；降级链：`go test ./internal/machine -run TestExecuteStepChannelFallback`；真实探测为手动命令（见记录内原文，需本机 pi + 网络） |
| 缺陷注入真实仓库 | §4.3 机制验证 | [defect-injection.md](defect-injection.md) | 注入周期（真 git）：`go test ./internal/defectinjection -run TestRunDefectInjection`；编排器集成：`go test ./internal/machine -run 'TestDefectInjection|TestResumeAtReviewDefectInjection'` |

> 历史注记：上表的 Node 入口（`tests/drill-*.test.js`、`core/*.js`）已随 v0.17 Go 迁移删除；
> v0.19 渠道收敛后**全系统无降级链**（OpenCode Zen / kimi-coding 兜底移除）——本页与
> 各记录中提及的降级链、provider 目录均为 2026-08-20 实测时的历史事实基线；
> `core/child-env.js` 对应现 `internal/childenv`（GateEnv）、`core/ledger.js` 对应 `internal/ledger`。
环境事实基线（2026-08-20 实测）：

- pi 0.84.0 provider 目录：`kimi-coding`（k3 / k3-256k）、`opencode`（Zen 按量，
  deepseek-v4-flash/pro、kimi-k3）、`opencode-go`（订阅，同模型族）。
  **无 DeepSeek 直连内置 provider**——用户裁决（2026-08-20）不需要直连降级，
  DeepSeek 链以 Zen 按量为终点。
- pi 的 `~/.pi/agent/auth.json` 优先于环境变量里的 `OPENCODE_API_KEY`；
  编排器驱动因此显式传 `--api-key` 保证注入权威（凭证卫生 §9）。
- pi 对 provider 错误**退出码仍为 0**，错误在事件流内
  （`stopReason:"error"` + `errorMessage`）——出口契约必须看流，不能只看进程。
- kimi CLI 0.37.2：`kimi -p --output-format stream-json` 输出
  meta/assistant/resume_hint 三类行，**无 usage 暴露**；`kimi -r session_<id>`
  resume 有效且回显同一 id；死会话 resume → exit 1 + stderr "Session not found"。
- reasoning_effort=max 的零污染注入方式：环境变量
  `KIMI_MODEL_THINKING_EFFORT=max`（官方，kimi provider 专用，
  优先级高于 config.toml，不写用户全局配置）。
- `NODE_TEST_CONTEXT` 会沿 env 继承：嵌套 `node --test` 在 child 模式下
  失败也退出 0。编排器跑验收命令前必须 scrub（`core/child-env.js`
  `gateEnv`；且 gate 环境整体已是白名单——任何 API key 都不会进入
  L1/L2/L3 命令环境，P0-4）。
