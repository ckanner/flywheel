// Package drivers — Codex driver（DESIGN v0.22）：OpenAI 官方 harness，驱动
// Review 与 Judge 复核面板（GPT-6-Astra，chatgpt 订阅）。
//
//	codex exec --json --skip-git-repo-check -m <model> \
//	          -c model_reasoning_effort=<effort> -C <cwd> <prompt>
//
// stdout 为 JSONL 事件流（thread.started / item.completed / turn.completed /
// error / turn.failed）——流式输出天然满足看门狗 idle 语义（对比 zcode 一次性
// 模式的全程静默）。usage 明细在 turn.completed 上暴露：input/cached/cache_write/
// output/reasoning 五桶（v0.22 探针实证）。订阅 OAuth 用户侧登录（codex login），
// 编排器零 key 注入。版本硬门槛：gpt-6-astra 需 CLI ≥0.154（npm）或 app 内置
// ≥0.153——旧版报 400 "requires a newer version of Codex"。
//
// 桌面可见性（v0.22）：exec 线程原生进 ChatGPT 桌面（com.openai.codex）的统一
// 线程库（~/.codex/state_5.sqlite 的 threads 表，source=exec）；项目需注册——
// spawn 前后各触发一次 `open -b com.openai.codex <dir>`（app 声明 Folder 文档
// 类型，与 zcode 的 --open-workspace 同款双触发模式：前触发注册项目并导入历史，
// 后触发把本会话立即入库）。
package drivers

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// CodexArgs RunCodex 入参。
type CodexArgs struct {
	Prompt          string
	Model           string // 订阅内 slug，如 "gpt-6-astra"；空 → CodexDefaultModel
	ReasoningEffort string // 统一 max 档（§7）；空 → "max"（GPT-6-Astra 默认仅 medium，必须显式）
	Cwd             string
	APIKeys         map[string]string
	Watchdog        watchdog.Opts
	OnEvent         func(line string) // stdout JSONL 行回调
}

// CodexDefaultModel Review/复核面板的法定模型（§7 矩阵）。
const CodexDefaultModel = "gpt-6-astra"

// CodexParsed codex JSONL 流的聚合结果。
type CodexParsed struct {
	SessionID    string
	Text         string
	Usage        Usage
	ErrorMessage string
	HasUsage     bool
}

// ParseCodexEventStream 解析 codex exec --json 的 NDJSON 事件流。容错纪律与
// pi/omp/kimi 一致：非 JSON 行跳过；流内 error 累积保留（失败归因不丢前因）。
func ParseCodexEventStream(stdout string) CodexParsed {
	var p CodexParsed
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			Thread  string `json:"thread_id"`
			Message string `json:"message"` // error 事件：顶层 message
			Error   *struct {
				Message string `json:"message"` // turn.failed：嵌套 error.message
			} `json:"error"`
			Item *struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
			Usage *struct {
				InputTokens        int64 `json:"input_tokens"`
				CachedInputTokens  int64 `json:"cached_input_tokens"`
				CacheWriteInput    int64 `json:"cache_write_input_tokens"`
				OutputTokens       int64 `json:"output_tokens"`
				ReasoningOutputTok int64 `json:"reasoning_output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(trimmed), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "thread.started":
			p.SessionID = ev.Thread
		case "item.completed":
			if ev.Item != nil && ev.Item.Type == "agent_message" && ev.Item.Text != "" {
				p.Text = ev.Item.Text
			}
		case "turn.completed":
			if u := ev.Usage; u != nil {
				p.HasUsage = true
				p.Usage = Usage{
					Input:      u.InputTokens,
					Output:     u.OutputTokens,
					Reasoning:  u.ReasoningOutputTok,
					CacheRead:  u.CachedInputTokens,
					CacheWrite: u.CacheWriteInput,
					// codex 不报 total；三桶求和（metrics 零值另有 input+output 回退）。
					TotalTokens: u.InputTokens + u.OutputTokens + u.ReasoningOutputTok,
				}
			}
		case "error", "turn.failed":
			msg := ev.Message
			if msg == "" && ev.Error != nil {
				msg = ev.Error.Message
			}
			if msg != "" {
				if p.ErrorMessage == "" {
					p.ErrorMessage = msg
				} else if !strings.Contains(p.ErrorMessage, msg) {
					p.ErrorMessage += "; " + msg
				}
			}
		}
	}
	return p
}

// codexProjectOpener 项目注册的系统出口（测试注入缝，AGENTS.md 覆盖纪律）。
var codexProjectOpener = defaultOpenCodexProject

// codexDesktopGOOS 平台闸注入缝（与 zcodeDesktopGOOS 同款）：生产恒
// runtime.GOOS，测试可强制 darwin 让 Linux CI 覆盖注册路径。
var codexDesktopGOOS = runtime.GOOS

// disableCodexRegistration 测试闸：Run 级测试不得激活用户桌面（隔离纪律）。
var disableCodexRegistration = false

// codexOpenBin macOS open 二进制路径（测试注入缝：假 open 脚本/不存在路径）。
var codexOpenBin = "/usr/bin/open"

func defaultOpenCodexProject(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, codexOpenBin, "-b", "com.openai.codex", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // fire-and-forget，只回收不等待
	return nil
}

// RegisterCodexProject 在 ChatGPT 桌面注册项目路径（macOS-only），触发该项目
// CLI 线程的桌面呈现。失败静默降级：可见性是 UX 增强，绝不阻塞执行链路。
func RegisterCodexProject(path string) {
	if disableCodexRegistration || codexDesktopGOOS != "darwin" || path == "" {
		return
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	_ = codexProjectOpener(path)
}

// RunCodex 驱动一次 codex exec 会话。凭证在用户侧（codex login OAuth），本函数
// 不注入任何凭证。spawn 前后双触发项目注册（语义见文件头注释）。
func RunCodex(a CodexArgs) Result {
	RegisterCodexProject(a.Cwd)
	model := a.Model
	if model == "" {
		model = CodexDefaultModel
	}
	effort := a.ReasoningEffort
	if effort == "" {
		effort = "max" // GPT-6-Astra 默认 medium——driver 强制显式 max（§7 统一档）
	}
	args := []string{"exec", "--json", "--skip-git-repo-check",
		"-m", model, "-c", "model_reasoning_effort=" + effort, "-C", a.Cwd, a.Prompt}

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

	result := watchdog.Run("codex", args, watchdog.Opts{
		Cwd:           a.Cwd,
		Env:           childenv.EnvSlice(childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs,
		WallTimeoutMs: a.Watchdog.WallTimeoutMs,
		KillGraceMs:   a.Watchdog.KillGraceMs,
		OnActivity:    onActivity,
	})
	RegisterCodexProject(a.Cwd)

	parsed := ParseCodexEventStream(result.Stdout)
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
		Harness:      "codex",
		Provider:     "chatgpt-subscription",
		Model:        model,
		SessionID:    parsed.SessionID,
		Text:         parsed.Text,
	}
	if parsed.HasUsage {
		u := parsed.Usage
		out.Usage = &u
	}
	ok := result.ExitCode != nil && *result.ExitCode == 0 && !result.TimedOut && result.SpawnError == ""
	out.Settled = ok && parsed.ErrorMessage == "" && parsed.Text != ""
	if !out.Settled {
		msg := parsed.ErrorMessage
		if msg == "" {
			msg = strings.TrimSpace(result.Stderr)
		}
		if msg == "" && result.SpawnError != "" {
			msg = result.SpawnError
		}
		if msg == "" && ok {
			msg = "codex: no agent_message in event stream"
		}
		if msg != "" {
			out.ErrorMessage = &msg
		}
	}
	return out
}
