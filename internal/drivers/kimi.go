// kimi driver：kimi code CLI 驱动 Plan Loop（DESIGN.md §4.1：kimi code +
// K3 max，长驻会话）。
//
// 对 kimi CLI 0.37.2 实证（2026-08-20）：
//   - `kimi -p "<prompt>" --output-format stream-json` 打印 NDJSON：
//     {"role":"meta","type":"system.version"} / {"role":"assistant","content":...}
//     / {"role":"meta","type":"session.resume_hint","session_id":"session_..."}。
//     无 usage/token 事件 → usage 记为 null。
//   - 长驻会话：首次调用从 resume hint 捕获 session_id；后续 `kimi -r session_<id>` 续。
//   - 会话死亡（挂起/过期）：`kimi -r <死id>` exit 1 + stderr
//     "Session ... not found" → machine 从契约层冷启动新会话（§4.1 重建协议）。
//   - reasoning_effort=max 每次调用经环境变量 KIMI_MODEL_THINKING_EFFORT=max
//     传入（官方支持，仅 kimi provider），绝不碰用户全局配置 ~/.kimi-code。
package drivers

import (
	"encoding/json"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// KimiDefaultModel Plan Loop 默认模型。
const KimiDefaultModel = "kimi-code/k3"

type KimiArgs struct {
	Prompt    string
	SessionID string // 长驻会话 resume 句柄；空 = 新会话
	Model     string
	Cwd       string
	APIKeys   map[string]string
	Watchdog  watchdog.Opts
}

// KimiParsed kimi stream-json 的聚合结果。
type KimiParsed struct {
	SessionID    string
	Text         string
	ErrorMessage string
	Usage        *Usage
}

// ParseKimiStreamJson 解析 kimi stream-json：会话 id、assistant 文本、错误、usage。
func ParseKimiStreamJson(stdout string) KimiParsed {
	var p KimiParsed
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var event struct {
			Role      string          `json:"role"`
			Type      string          `json:"type"`
			SessionID string          `json:"session_id"`
			Content   json.RawMessage `json:"content"`
			Message   string          `json:"message"`
			Error     string          `json:"error"`
			Usage     json.RawMessage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
			continue
		}
		if event.Role == "meta" && event.Type == "session.resume_hint" && event.SessionID != "" {
			p.SessionID = event.SessionID
			continue
		}
		if event.Role == "assistant" {
			var s string
			if err := json.Unmarshal(event.Content, &s); err == nil {
				p.Text = s
			} else {
				var parts []struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(event.Content, &parts); err == nil {
					var b strings.Builder
					for _, part := range parts {
						b.WriteString(part.Text)
					}
					p.Text = b.String()
				}
			}
			continue
		}
		if event.Type == "error" || event.Role == "error" {
			switch {
			case event.Message != "":
				p.ErrorMessage = event.Message
			case event.Error != "":
				p.ErrorMessage = event.Error
			default:
				p.ErrorMessage = trimmed
			}
			continue
		}
		if len(event.Usage) > 0 && string(event.Usage) != "null" {
			var u Usage
			if err := json.Unmarshal(event.Usage, &u); err == nil {
				copied := u // not emitted by 0.37.2; captured if it ever appears
				p.Usage = &copied
			}
		}
	}
	return p
}

// RunKimi 驱动一次 kimi code 调用（Plan Loop）。KIMI_MODEL_THINKING_EFFORT=max
// 按次注入，绝不写用户全局配置（§7）。
func RunKimi(a KimiArgs) Result {
	args := []string{}
	if a.SessionID != "" {
		args = append(args, "-r", a.SessionID)
	}
	model := a.Model
	if model == "" {
		model = KimiDefaultModel
	}
	args = append(args, "-m", model, "-p", a.Prompt, "--output-format", "stream-json")

	env := childenv.EnvSlice(childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys))
	env = append(env, "KIMI_MODEL_THINKING_EFFORT=max")

	result := watchdog.Run("kimi", args, watchdog.Opts{
		Cwd:           a.Cwd,
		Env:           env,
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs,
		WallTimeoutMs: a.Watchdog.WallTimeoutMs,
		KillGraceMs:   a.Watchdog.KillGraceMs,
	})
	parsed := ParseKimiStreamJson(result.Stdout)

	settled := result.ExitCode != nil && *result.ExitCode == 0 && !result.TimedOut
	out := Result{
		ExitCode:     result.ExitCode,
		Signal:       result.Signal,
		TimedOut:     result.TimedOut,
		IdleTimedOut: result.IdleTimedOut,
		WallTimedOut: result.WallTimedOut,
		Stdout:       result.Stdout,
		Stderr:       result.Stderr,
		Truncated:    result.Truncated,
		DurationMS:   result.DurationMS,
		SpawnError:   result.SpawnError,
		Harness:      "kimi",
		Provider:     "kimi",
		Model:        model,
		Settled:      settled,
		Text:         parsed.Text,
		SessionID:    parsed.SessionID,
	}
	if parsed.ErrorMessage != "" {
		out.ErrorMessage = &parsed.ErrorMessage
	} else if result.ExitCode != nil && *result.ExitCode != 0 {
		msg := strings.TrimSpace(result.Stderr)
		if msg != "" {
			out.ErrorMessage = &msg
		}
	}
	return out
}
