// kimi driver：直接运行 Kimi Code CLI；不使用 ACP（docs/design.md 2026-11-01 裁决）。
//
// Kimi Code 2.4.1 的 print 契约：
//   - `kimi -p "<prompt>" --output-format stream-json` 自带非交互 auto 模式，
//     不能叠加 --auto / --yolo / --plan；这也不构成只读沙箱。
//   - 新会话的 session.resume_hint 只在成功收尾路径输出；没有预分配 id。
//     后续用 --session <id> 在原 cwd 续接。CLI 检查 cwd，不匹配时原样报错，
//     driver 不偷偷改 cwd 或重建会话；冷启动由 machine 按契约决定。
//   - 续接恢复的是历史，不是已退出进程；timeout / cancel 仍由 watchdog 管理。
//   - 当前 print writer 不输出 token usage；未知保持 nil，不能记成免费。
//   - thinking 每次由 KIMI_MODEL_THINKING_EFFORT 显式注入，默认 high，
//     不修改用户配置。显式模型别名由 binding 提供，省略时使用当前订阅默认。
package drivers

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// KimiDefaultModel Plan Loop 默认模型。
const KimiDefaultModel = "kimi-code/k3.1-pro"

const KimiCommandEnv = "FLYWHEEL_KIMI_BIN"

type KimiArgs struct {
	Prompt            string
	SessionID         string // 长驻会话 resume 句柄；空 = 新会话
	PersistentSession bool   // scoped lifecycle policy; CLI history is already persistent
	RequestID         string // engine invocation identity, never a backend idempotency key
	Model             string
	Thinking          string // 空 = high；Kimi provider 的逐次环境覆盖
	Command           string // 空 = PATH 中的 kimi；也可传含空格的完整可执行文件路径
	Cwd               string
	APIKeys           map[string]string
	Watchdog          watchdog.Opts
	OnEvent           func(line string) // stdout JSONL 行回调，包含无换行的最后一行
}

// KimiParsed kimi stream-json 的聚合结果。
type KimiParsed struct {
	SessionID     string
	Text          string
	ErrorMessage  string
	Usage         *Usage
	ProtocolError string // engine-tracked calls require an intact stream through the final hint
}

// ParseKimiStreamJson 解析完整的 kimi stream-json。与其他 driver 一致，忽略
// 非 JSON 噪声及未知事件；turn.step.retrying 是暂态，不能误判成终态错误。
// assistant 的工具调用/空 content 不抹掉最后一段正文。usage 若未来出现，
// 只接收规范化字段的最新有效快照，不推测 delta/cumulative 语义或补算缺项。
func ParseKimiStreamJson(stdout string) KimiParsed {
	var p KimiParsed
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if p.SessionID != "" {
			p.ProtocolError = "Kimi emitted another event after session.resume_hint"
		}
		var event struct {
			Role      string          `json:"role"`
			Type      string          `json:"type"`
			SessionID string          `json:"session_id"`
			Content   json.RawMessage `json:"content"`
			Message   json.RawMessage `json:"message"`
			Error     json.RawMessage `json:"error"`
			Usage     json.RawMessage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
			p.ProtocolError = "Kimi emitted malformed stream-json"
			continue
		}
		// Usage can accompany an assistant event; parse it before role dispatch.
		if usage := parseKimiUsage(event.Usage); usage != nil {
			p.Usage = usage
		}
		if event.Role == "meta" && event.Type == "session.resume_hint" && event.SessionID != "" {
			p.SessionID = event.SessionID
		}
		if event.Role == "assistant" {
			var s string
			if err := json.Unmarshal(event.Content, &s); err == nil {
				if s != "" {
					p.Text = s
				}
			} else {
				var parts []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}
				if err := json.Unmarshal(event.Content, &parts); err == nil {
					var b strings.Builder
					for _, part := range parts {
						if part.Type == "" || part.Type == "text" {
							b.WriteString(part.Text)
						}
					}
					if b.Len() > 0 {
						p.Text = b.String()
					}
				}
			}
		}
		if event.Type == "error" || event.Role == "error" {
			message := jsonErrorMessage(event.Message, event.Error, trimmed)
			if p.ErrorMessage == "" {
				p.ErrorMessage = message
			} else if !strings.Contains(p.ErrorMessage, message) {
				p.ErrorMessage += "; " + message
			}
		}
	}
	return p
}

// parseKimiUsage 不把 {}、未知形状、null 或坏计数误读成确定的零用量。
func parseKimiUsage(raw json.RawMessage) *Usage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	// A partial counter is not complete call usage. In particular a missing
	// total must not silently become a known zero for the budget gate.
	for _, name := range []string{"input", "output", "totalTokens"} {
		if value, exists := fields[name]; !exists || string(value) == "null" {
			return nil
		}
	}
	var usage Usage
	for _, field := range []struct {
		name  string
		value *int64
	}{
		{"input", &usage.Input}, {"output", &usage.Output},
		{"reasoning", &usage.Reasoning}, {"cacheRead", &usage.CacheRead},
		{"cacheWrite", &usage.CacheWrite}, {"totalTokens", &usage.TotalTokens},
	} {
		value, exists := fields[field.name]
		if !exists || string(value) == "null" {
			continue
		}
		if err := json.Unmarshal(value, field.value); err != nil || *field.value < 0 {
			return nil
		}
	}
	return &usage
}

// RunKimi 驱动一次 print 调用。只有退出成功且收到收尾 hint 才确认完成；
// 失败调用的 hint 不作为可恢复句柄暴露。任务验收仍由 engine 的 oracle 决定。
func RunKimi(a KimiArgs) Result {
	tracked := a.PersistentSession || a.RequestID != ""
	if tracked && (strings.TrimSpace(a.RequestID) == "" || strings.ContainsAny(a.RequestID+a.SessionID, "\x00\r\n")) {
		out := invalidHarnessResult("kimi", a.Model, "tracked Kimi calls require a request identity and valid opaque session handle")
		out.Session = &SessionOutcome{Protocol: "kimi-print/v1", RequestID: a.RequestID,
			Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true, UsageStatus: "unknown"}
		return out
	}
	args := []string{}
	if a.SessionID != "" {
		args = append(args, "--session", a.SessionID)
	}
	model := a.Model
	if model == "" {
		model = KimiDefaultModel
	}
	args = append(args, "-m", model, "-p", a.Prompt, "--output-format", "stream-json")

	effort := a.Thinking
	if effort == "" {
		effort = "high"
	}
	env := childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)
	env["KIMI_MODEL_THINKING_EFFORT"] = effort
	command := a.Command
	if command == "" {
		command = os.Getenv(KimiCommandEnv)
	}
	if command == "" {
		command = "kimi"
	}
	var onActivity func(chunk, stream string)
	events := jsonLines{onLine: a.OnEvent, limit: watchdog.MaxBuffer}
	if a.OnEvent != nil {
		onActivity = events.feed
	}

	result := watchdog.Run(command, args, watchdog.Opts{
		Context:       a.Watchdog.Context,
		Cwd:           a.Cwd,
		Env:           childenv.EnvSlice(env),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs,
		WallTimeoutMs: a.Watchdog.WallTimeoutMs,
		KillGraceMs:   a.Watchdog.KillGraceMs,
		OnActivity:    onActivity,
	})
	// watchdog.Wait has drained both streams; no callback can race this tail.
	if a.OnEvent != nil {
		events.flush()
	}
	parsed := ParseKimiStreamJson(result.Stdout)

	message := parsed.ErrorMessage
	if result.TimedOut {
		message = "Kimi command timed out"
	}
	if message == "" {
		switch {
		case result.Truncated:
			message = "Kimi output was truncated; completion is unknown"
		case result.ExitCode != nil && *result.ExitCode != 0:
			message = strings.TrimSpace(result.Stderr)
		case result.ExitCode != nil && *result.ExitCode == 0 && parsed.SessionID == "":
			message = "Kimi stream ended without session.resume_hint; completion is unknown"
		}
	}
	if message == "" && a.SessionID != "" && parsed.SessionID != "" && parsed.SessionID != a.SessionID {
		message = "Kimi returned session.resume_hint for another session"
	}
	if message == "" && tracked && parsed.ProtocolError != "" {
		message = parsed.ProtocolError
	}
	settled := result.ExitCode != nil && *result.ExitCode == 0 && !result.TimedOut && !result.Cancelled &&
		result.Signal == "" && result.CleanupError == "" && !result.Truncated && message == "" && parsed.SessionID != ""
	out := fromWatchdog(result, "kimi", "kimi", model)
	out.Usage, out.Settled, out.Text = parsed.Usage, settled, parsed.Text
	if settled {
		out.SessionID = parsed.SessionID
	}
	if message != "" {
		out.ErrorMessage = &message
	}
	if tracked {
		// The checked print protocol has no attributable token delta. A future
		// usage-shaped object is not proof of per-call rather than thread totals.
		out.Usage = nil
	}
	// Legacy resident Plan also consumes these facts; a failed resume cannot
	// silently cold-start a second call merely because scoped mode is disabled.
	out.Session = &SessionOutcome{Protocol: "kimi-print/v1", RequestID: a.RequestID,
		SessionID: a.SessionID, Dispatch: "unknown", Outcome: "unknown",
		CleanupConfirmed: out.CleanupError == "" && (out.ExitCode != nil || out.SpawnError != "" || out.Signal != ""),
		UsageStatus:      "unknown"}
	if out.SpawnError != "" || (out.Cancelled && out.ExitCode == nil && out.Signal == "") {
		out.Session.Dispatch, out.Session.Outcome, out.Session.CleanupConfirmed, out.Session.UsageStatus = "not-dispatched", "failed", true, "complete"
		out.Usage, out.CostUSD = &Usage{}, new(float64)
	} else if settled && parsed.ProtocolError == "" {
		out.Session.SessionID = parsed.SessionID
		out.Session.Dispatch, out.Session.Outcome = "dispatched", "completed"
		out.Session.ResumeCandidate = true
		out.Session.ResumeAccepted = a.SessionID != ""
	}
	return out
}
