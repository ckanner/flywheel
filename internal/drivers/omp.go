// Package drivers — omp driver（PR #3 review 后追加）：oh-my-pi CLI 是 pi 的
// 官方分叉，CLI 接口与 JSON 事件流结构与 pi 高度同源，行为差异有二：
//
//  1. 终止标记：pi 以 agent_settled 收尾；omp 以 agent_end 收尾且不产生
//     agent_settled —— ParseOmpEventStream 复用共用解析器并以 agent_end 作
//     settled 标记，Result.Settled 语义与 pi 对齐。
//  2. reasoningTokens / duration / ttft 字段在 message_end.usage 中额外暴露
//     （pi 无此三字段）——usage 解析的指针化设计天然兼容。
//
// 其余（--api-key 显式注入压过 auth store、stopReason:error 流内捕获、
// --no-session/--mode json flags、agent_start/turn_start/message_update 等
// 事件顺序）已通过 kimi 0.38.0-era 二进制 + GLM-5.3-flash 本机探针实证等价。
package drivers

import (
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// OmpArgs RunOmp 入参（与 PiArgs 同构）。
type OmpArgs struct {
	Prompt   string
	Model    string
	Thinking string // 全系统恒 max（§7）
	Cwd      string
	APIKey   string // 渠道 env-key，仅此来源
	APIKeys  map[string]string
	Watchdog watchdog.Opts
	OnEvent  func(line string) // stdout NDJSON 行回调
}

// RunOmp 驱动一次 oh-my-pi 会话。凭证注入方式与 pi 相同（--api-key 权威）。
func RunOmp(a OmpArgs) Result {
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

	result := watchdog.Run("omp", args, watchdog.Opts{
		Cwd:           a.Cwd,
		Env:           childenv.EnvSlice(childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs,
		WallTimeoutMs: a.Watchdog.WallTimeoutMs,
		KillGraceMs:   a.Watchdog.KillGraceMs,
		OnActivity:    onActivity,
	})
	parsed := ParseOmpEventStream(result.Stdout)
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
		Harness:        "omp",
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

// ParseOmpEventStream 与 pi 共用同一事件流解析逻辑（omp 是 pi 的 fork，
// JSON 输出结构相同），区别仅在终止标记是 agent_end 而非 agent_settled。
func ParseOmpEventStream(stdout string) PiParsed {
	return parseAgentEventStream(stdout, "agent_end")
}
