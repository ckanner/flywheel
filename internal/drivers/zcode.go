// Package drivers — zcode driver（PR #4 review 后新增）：ZCode headless CLI
// 是 Z.ai 官方 GLM-5.3 harness 的命令行出口（unofficial wrapper zcode-app-cli
// 封装官方 runtime 0.16.5），单 prompt 模式输出单个 JSON 对象：
//
//	{
//	  "sessionId": "sess_...", "traceId": "...", "turnId": "...",
//	  "response": "<final text>",
//	  "usage": {"inputTokens":N,"outputTokens":N,"reasoningTokens":N,
//	            "cacheReadTokens":N,"cacheWriteTokens":N,"totalTokens":N,...},
//	  "projection": {"status":"idle","turnCount":1,"contextWindow":1000000}
//	}
//
// 认证：provider.options.apiKey（写 ~/.zcode/cli/config.json）或 Z.ai OAuth，
// 均在用户侧配置完成——driver 不注入 key（与 kimi 同策略：凭证留用户侧）。
// --mode 默认 yolo（官方 --prompt 缺省值）：无人值守必须跳过审批。
package drivers

import (
	"encoding/json"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// ZcodeArgs RunZcode 入参。
type ZcodeArgs struct {
	Prompt    string
	Model     string // provider 内 slug，如 "zai/glm-5.3-flash"
	Cwd       string
	SessionID string // sess_... 续会话；空 = 新会话
	Mode      string // permission mode: build|edit|plan|yolo；空 = 官方默认 yolo
	APIKeys   map[string]string
	Watchdog  watchdog.Opts
}

// ZcodeDefaultModel Coding Plan 下 GLM-5.3-Flash 的 provider 内 slug。
const ZcodeDefaultModel = "zai/glm-5.3-flash"

// ZcodeJSON zcode --json 单对象输出的字段映射。
type ZcodeJSON struct {
	SessionID string `json:"sessionId"`
	TraceID   string `json:"traceId"`
	TurnID    string `json:"turnId"`
	Response  string `json:"response"`
	Usage     struct {
		InputTokens      int64 `json:"inputTokens"`
		OutputTokens     int64 `json:"outputTokens"`
		TotalTokens      int64 `json:"totalTokens"`
		CacheReadTokens  int64 `json:"cacheReadTokens"`
		CacheWriteTokens int64 `json:"cacheWriteTokens"`
		ReasoningTokens  int64 `json:"reasoningTokens"`
	} `json:"usage"`
	EventCount int64 `json:"eventCount"`
	Projection struct {
		Status          string `json:"status"`
		ContextWindow   int64  `json:"contextWindow"`
		TotalTokenCount int64  `json:"totalTokenCount"`
	} `json:"projection"`
}

// ParseZcodeJSON 解析 zcode --json 输出。非 JSON（TUI 泄漏/错误文本）返回零值，
// 由调用方按 EMPTY_RESPONSE 处理——与 pi/kimi 解析器的容错纪律一致。
func ParseZcodeJSON(stdout string) (ZcodeJSON, bool) {
	var z ZcodeJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &z); err != nil {
		return ZcodeJSON{}, false
	}
	return z, true
}

// RunZcode 驱动一次 zcode headless 会话。凭证在用户侧 config.json 配置完成
// （OAuth 或 API Key），本函数不注入任何凭证。
func RunZcode(a ZcodeArgs) Result {
	args := []string{"--prompt", a.Prompt, "--json"}
	model := a.Model
	if model == "" {
		model = ZcodeDefaultModel
	}
	args = append(args, "--model", model)
	if a.SessionID != "" {
		args = append(args, "--resume", a.SessionID)
	}
	mode := a.Mode
	if mode == "" {
		mode = "yolo" // 官方 --prompt 缺省值，显式声明防上游变更
	}
	args = append(args, "--mode", mode)

	result := watchdog.Run("zcode", args, watchdog.Opts{
		Cwd:           a.Cwd,
		Env:           childenv.EnvSlice(childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs,
		WallTimeoutMs: a.Watchdog.WallTimeoutMs,
		KillGraceMs:   a.Watchdog.KillGraceMs,
	})

	out := Result{
		ExitCode:   result.ExitCode,
		Signal:     result.Signal,
		TimedOut:   result.TimedOut,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		Truncated:  result.Truncated,
		DurationMS: result.DurationMS,
		SpawnError: result.SpawnError,
		Harness:    "zcode",
		Provider:   "zai-coding-plan",
		Model:      model,
	}
	settled := result.ExitCode != nil && *result.ExitCode == 0 && !result.TimedOut && result.SpawnError == ""
	if settled {
		if z, ok := ParseZcodeJSON(result.Stdout); ok {
			out.Settled = true
			out.Text = z.Response
			out.SessionID = z.SessionID
			total := z.Usage.TotalTokens
			reasoning := z.Usage.ReasoningTokens
			cacheRead := z.Usage.CacheReadTokens
			cacheWrite := z.Usage.CacheWriteTokens
			out.Usage = &Usage{
				Input:      total - reasoning - cacheRead - cacheWrite,
				Output:     z.Usage.OutputTokens,
				Reasoning:  reasoning,
				CacheRead:  cacheRead,
				CacheWrite: cacheWrite,
			}
		} else {
			out.Settled = false
			msg := strings.TrimSpace(result.Stdout)
			out.ErrorMessage = &msg // 非 JSON 输出留给分类器定码
		}
	} else if msg := strings.TrimSpace(result.Stderr); msg != "" {
		out.ErrorMessage = &msg
	}
	return out
}
