// pi driver（DESIGN.md §4.2/§4.3）：经 pi CLI 驱动 Execute/Review/Judge。
//
//	pi -p "<prompt>" --model <provider/model> --thinking max --no-session --mode json
//
// stdout 是 NDJSON 事件流；usage（input/output/reasoning/cacheRead/cacheWrite
// + cost）从 assistant message_end 事件聚合，agent_settled 表示干净收尾。
// 附录 D 警告：thinking=max 时 usage 可能缺 reasoning 明细——绝不假设字段存在。
//
// 凭证卫生（§9）：子进程拿最小环境——低价值 executor key（OPENCODE_API_KEY）
// 加 CLI 运行所需，别的什么都没有。
package drivers

import (
	"encoding/json"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// PiArgs RunPi 入参。
type PiArgs struct {
	Prompt   string
	Model    string
	Thinking string // 全系统恒 max（§7）
	Cwd      string
	APIKey   string // 渠道 env-key，仅此来源
	APIKeys  map[string]string
	Watchdog watchdog.Opts
	OnEvent  func(line string) // stdout NDJSON 行回调
}

// PiParsed pi NDJSON 流的聚合结果。
type PiParsed struct {
	Usage          Usage
	ReasoningChars int64
	CostUSD        *float64 // 流未报成本 → nil
	Settled        bool
	Text           string
	Provider       string
	Model          string
	ErrorMessage   string
}

// ParsePiEventStream 解析 pi 的 NDJSON 事件流：聚合 usage、最终文本、settled
// 标记，以及——关键的——流内错误。provider 调用失败时 pi 照样 exit 0
// （2026-08-20 实证：坏 key 在流内给出 stopReason:"error" + "401: ..."），
// 所以错误必须在这里读出来。stdout 上的非 JSON 噪声容忍，绝不致命。
func ParsePiEventStream(stdout string) PiParsed {
	var p PiParsed
	usage := Usage{}
	costUsd := 0.0
	sawCost := false
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var event struct {
			Type    string `json:"type"`
			Message *struct {
				Role         string          `json:"role"`
				Provider     string          `json:"provider"`
				Model        string          `json:"model"`
				StopReason   string          `json:"stopReason"`
				ErrorMessage string          `json:"errorMessage"`
				Usage        json.RawMessage `json:"usage"`
				Content      []struct {
					Type     string `json:"type"`
					Thinking string `json:"thinking"`
					Text     string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
			continue
		}
		if event.Type == "agent_settled" {
			p.Settled = true
			continue
		}
		if event.Type != "message_end" || event.Message == nil || event.Message.Role != "assistant" {
			continue
		}
		msg := event.Message
		if p.Provider == "" {
			p.Provider = msg.Provider
		}
		if p.Model == "" {
			p.Model = msg.Model
		}
		if msg.StopReason == "error" && msg.ErrorMessage != "" {
			p.ErrorMessage = msg.ErrorMessage
		}
		if len(msg.Usage) > 0 && string(msg.Usage) != "null" {
			var u struct {
				Input       *int64 `json:"input"`
				Output      *int64 `json:"output"`
				Reasoning   *int64 `json:"reasoning"`
				CacheRead   *int64 `json:"cacheRead"`
				CacheWrite  *int64 `json:"cacheWrite"`
				TotalTokens *int64 `json:"totalTokens"`
				Cost        *struct {
					Total *float64 `json:"total"`
				} `json:"cost"`
			}
			if err := json.Unmarshal(msg.Usage, &u); err == nil {
				val := func(p *int64) int64 {
					if p != nil {
						return *p
					}
					return 0
				}
				usage.Input += val(u.Input)
				usage.Output += val(u.Output)
				usage.Reasoning += val(u.Reasoning) // thinking=max 时可能缺（附录 D）
				usage.CacheRead += val(u.CacheRead)
				usage.CacheWrite += val(u.CacheWrite)
				tt := val(u.TotalTokens)
				if u.TotalTokens == nil {
					tt = val(u.Input) + val(u.Output)
				}
				usage.TotalTokens += tt
				if u.Cost != nil && u.Cost.Total != nil {
					costUsd += *u.Cost.Total
					sawCost = true
				}
			}
		}
		for _, part := range msg.Content {
			if part.Type == "thinking" {
				p.ReasoningChars += int64(len(part.Thinking))
			}
			if part.Type == "text" && part.Text != "" {
				p.Text = part.Text
			}
		}
	}
	p.Usage = usage
	if sawCost {
		p.CostUSD = &costUsd
	}
	return p
}

// RunPi 驱动一次 pi 会话。注入的渠道 key 是权威的：pi 自己的 auth store
// （~/.pi/agent/auth.json）否则会静默压过环境变量（2026-08-20 实证），那会
// 架空凭证卫生（§9）。apiKey 只来自渠道 env-key，绝不含 live 凭证审批集。
func RunPi(a PiArgs) Result {
	args := []string{"-p", a.Prompt, "--model", a.Model, "--thinking", a.Thinking, "--no-session", "--mode", "json"}
	if a.APIKey != "" {
		args = append(args, "--api-key", a.APIKey)
	}

	var onActivity func(chunk, stream string)
	if a.OnEvent != nil {
		buf := ""
		onActivity = func(chunk, stream string) {
			if stream != "stdout" {
				return
			}
			buf += chunk
			lines := strings.Split(buf, "\n")
			for i := 0; i < len(lines)-1; i++ {
				a.OnEvent(lines[i])
			}
			buf = lines[len(lines)-1]
		}
	}

	result := watchdog.Run("pi", args, watchdog.Opts{
		Cwd:           a.Cwd,
		Env:           childenv.EnvSlice(childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs,
		WallTimeoutMs: a.Watchdog.WallTimeoutMs,
		KillGraceMs:   a.Watchdog.KillGraceMs,
		OnActivity:    onActivity,
	})
	parsed := ParsePiEventStream(result.Stdout)
	out := Result{
		ExitCode:       result.ExitCode,
		Signal:         result.Signal,
		TimedOut:       result.TimedOut,
		IdleTimedOut:   result.IdleTimedOut,
		WallTimedOut:   result.WallTimedOut,
		Stdout:         result.Stdout,
		Stderr:         result.Stderr,
		Truncated:      result.Truncated,
		DurationMS:     result.DurationMS,
		SpawnError:     result.SpawnError,
		Harness:        "pi",
		Provider:       parsed.Provider,
		Model:          parsed.Model,
		Usage:          &parsed.Usage,
		Settled:        parsed.Settled,
		Text:           parsed.Text,
		ReasoningChars: &parsed.ReasoningChars,
		CostUSD:        parsed.CostUSD,
	}
	if out.Provider == "" {
		if i := strings.IndexByte(a.Model, '/'); i > 0 {
			out.Provider = a.Model[:i]
		}
	}
	if out.Model == "" {
		out.Model = a.Model
	}
	if parsed.ErrorMessage != "" {
		msg := parsed.ErrorMessage
		out.ErrorMessage = &msg
	}
	return out
}
