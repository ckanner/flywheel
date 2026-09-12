# Drill: 401/403 降级分类（门槛 §10-4 可达部分，2026-08-20）

## 目的

docs/DESIGN.md §6 要求"检测到 403/限流自动切渠道"。演练回答：OpenCode Go 的
鉴权类失败经 pi 暴露成什么形态？编排器 `classifyError` 归为什么码？
401 与 403 应不应走同一条路？

## 方法（可重复）

真实探测（原 Node drill 首条，现为手动命令——需本机 pi + 网络；
用**无效 key** 经 `--api-key` 强制注入，不依赖任何有效凭证）：

```sh
pi -p "Reply with exactly: ok" --model opencode-go/deepseek-v4-flash \
  --thinking max --no-session --mode json --api-key "sk-invalid-drill-000..."
```

分类规则单测（离线，Go 版）：合成 401/403 文本对照 ——
`go test ./internal/errs -run 'TestClassifyError|TestRetryableAndFallbackSets'`。

## 实测发现

1. **pi 对 provider 错误退出码为 0**。无效 key 下 stdout 事件流里
   `message_start/message_end` 带 `stopReason:"error"` +
   `errorMessage:"401: {\"type\":\"AuthError\",\"message\":\"Invalid API key.\"}"`，
   `agent_settled` 照常到达，stderr 为空。
   → 只看进程终态会把 401 误读为成功。驱动必须解析流内 errorMessage
   （`parsePiEventStream`），且流内错误优先于出口契约。
2. **`OPENCODE_API_KEY` 环境变量被 `~/.pi/agent/auth.json` 抢先**：
   仅改 env 时错误 key 不生效（请求照常成功）。编排器驱动改为显式
   `--api-key <injected>`，保证"注入什么用什么"（凭证卫生 §9）。
3. **401 ≠ 403，处置不同**：
   - `401 Invalid API key`（AuthError）= 配置错误 → `AUTH`，**不降级、
     不重试**（拿着同一把坏 key 换渠道无意义），直接走升级路径。
   - `403 forbidden / fair-use / quota` = 订阅超额或公平使用封禁（§7 描述的
     主力通路风险）→ `QUOTA`，**降级链切换**（opencode-go → opencode Zen
     按量，OpenCode 体系内兜底为终点——用户裁决 2026-08-20 移除直连降级），
     同渠道不重试。
   - 429 → `RATE_LIMIT`：降级 + 可重试（尊重 Retry-After）。

## 规则落点

- `internal/errs`（Go 版单一落点）：`FallbackCodes = {QUOTA, RATE_LIMIT}`（AUTH 移出）；
  模式表 401→AUTH、403/forbidden→QUOTA。
- 编排器层回归：`go test ./internal/machine -run 'TestExecuteStepChannelFallback|TestAgentVerdictStep'`
  （403 切换下一渠道 / 401 只试主渠道后升级）。

## 残余风险

- **真实 403 未能注入**：手头无超额/被封的 Go 账号，403 文本形态来自
  §7 描述与通用 HTTP 形态合成，未亲见 Go 的 403 报文。若实际报文不含
  "403/forbidden/quota" 字样（如 Zen 返回 402/429 变体），需按真实报文
  再调模式表——分类规则集中在 internal/errs 一处，改动成本低。
- 降级链终点为 Zen 按量；Zen 自身限流时的处置（排队重试 vs 停机叫人）
  未演练。
