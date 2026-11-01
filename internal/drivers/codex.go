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
// 编排器零 key 注入；沙箱按角色选择 read-only 或 workspace-write，审批
// never，不默认 danger-full-access。版本硬门槛：gpt-6-astra 需 CLI ≥0.154（npm）
// 或 app 内置 ≥0.153——旧版报 400 "requires a newer version of Codex"。
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
	"math"
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
	ReasoningEffort string // 逐任务思考档；空 → high
	PermissionMode  string // read-only（默认）或 workspace-write；审批始终 never
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
	Completed    bool // the last started turn has a successful turn.completed marker
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
			Type    string          `json:"type"`
			Thread  string          `json:"thread_id"`
			Message json.RawMessage `json:"message"`
			Error   json.RawMessage `json:"error"`
			Item    *struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
			Usage *struct {
				InputTokens        *int64 `json:"input_tokens"`
				CachedInputTokens  int64  `json:"cached_input_tokens"`
				CacheWriteInput    int64  `json:"cache_write_input_tokens"`
				OutputTokens       *int64 `json:"output_tokens"`
				ReasoningOutputTok int64  `json:"reasoning_output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(trimmed), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "thread.started":
			p.SessionID = ev.Thread
		case "turn.started":
			p.Completed = false
			p.Text, p.HasUsage = "", false
		case "item.completed":
			if ev.Item != nil && ev.Item.Type == "agent_message" && ev.Item.Text != "" {
				p.Text = ev.Item.Text
			}
		case "turn.completed":
			p.Completed = true
			p.HasUsage = false
			if u := ev.Usage; u != nil && u.InputTokens != nil && u.OutputTokens != nil &&
				*u.InputTokens >= 0 && *u.OutputTokens >= 0 && *u.InputTokens <= math.MaxInt64-*u.OutputTokens &&
				u.CachedInputTokens >= 0 && u.CacheWriteInput >= 0 && u.ReasoningOutputTok >= 0 {
				p.HasUsage = true
				p.Usage = Usage{
					Input:      *u.InputTokens,
					Output:     *u.OutputTokens,
					Reasoning:  u.ReasoningOutputTok,
					CacheRead:  u.CachedInputTokens,
					CacheWrite: u.CacheWriteInput,
					// reasoning 是 output 子集，cached input 是 input 子集；不重复相加。
					TotalTokens: *u.InputTokens + *u.OutputTokens,
				}
			}
		case "error", "turn.failed":
			msg := jsonErrorMessage(ev.Message, ev.Error, trimmed)
			if p.ErrorMessage == "" {
				p.ErrorMessage = msg
			} else if !strings.Contains(p.ErrorMessage, msg) {
				p.ErrorMessage += "; " + msg
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
	model := a.Model
	if model == "" {
		model = CodexDefaultModel
	}
	effort := a.ReasoningEffort
	if effort == "" {
		effort = "high"
	}
	permission := a.PermissionMode
	if permission == "" {
		permission = "read-only"
	}
	if permission != "read-only" && permission != "workspace-write" {
		return invalidHarnessResult("codex", model, "unsupported permission mode "+permission)
	}
	RegisterCodexProject(a.Cwd)
	args := []string{"exec", "--json", "--skip-git-repo-check",
		"-m", model, "-c", "model_reasoning_effort=" + effort,
		"-s", permission, "-c", "approval_policy=never", "-C", a.Cwd}
	args = append(args, "--", a.Prompt)

	var onActivity func(chunk, stream string)
	events := jsonLines{onLine: a.OnEvent, limit: watchdog.MaxBuffer}
	if a.OnEvent != nil {
		onActivity = events.feed
	}

	result := watchdog.Run("codex", args, watchdog.Opts{
		Context:       a.Watchdog.Context,
		Cwd:           a.Cwd,
		Env:           childenv.EnvSlice(childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs,
		WallTimeoutMs: a.Watchdog.WallTimeoutMs,
		KillGraceMs:   a.Watchdog.KillGraceMs,
		OnActivity:    onActivity,
	})
	if a.OnEvent != nil {
		events.flush()
	}
	RegisterCodexProject(a.Cwd)

	parsed := ParseCodexEventStream(result.Stdout)
	out := fromWatchdog(result, "codex", "chatgpt-subscription", model)
	out.SessionID, out.Text = parsed.SessionID, parsed.Text
	if parsed.HasUsage {
		u := parsed.Usage
		out.Usage = &u
	}
	ok := result.ExitCode != nil && *result.ExitCode == 0 && !result.TimedOut && !result.Cancelled &&
		!result.Truncated && result.Signal == "" && result.CleanupError == "" && result.SpawnError == ""
	out.Settled = ok && parsed.Completed && parsed.ErrorMessage == "" && parsed.Text != ""
	if !out.Settled {
		msg := parsed.ErrorMessage
		if msg == "" {
			msg = strings.TrimSpace(result.Stderr)
		}
		if msg == "" && result.SpawnError != "" {
			msg = result.SpawnError
		}
		if msg == "" && result.Truncated {
			msg = "codex: event stream was truncated"
		}
		if msg == "" && ok {
			if !parsed.Completed {
				msg = "codex: event stream ended without turn.completed"
			} else {
				msg = "codex: no agent_message in event stream"
			}
		}
		if msg != "" {
			out.ErrorMessage = &msg
		}
	}
	return out
}
