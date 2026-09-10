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
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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

// zcodeWorkspaceOpener workspace 注册的系统出口（测试注入缝，AGENTS.md 覆盖
// 纪律：副作用经缝注入而非测试内 mock 进程）。
var zcodeWorkspaceOpener = defaultOpenZcodeWorkspace

// zcodeDesktopGOOS 平台闸注入缝：生产恒 runtime.GOOS，测试可强制 "darwin"
// 让 Linux CI 覆盖注册路径（否则 darwin-only 分支在 Linux 上既测不到也
// 拉不满覆盖率门禁——PR #4 review 修复）。
var zcodeDesktopGOOS = runtime.GOOS

// ZcodeDesktopBinEnv 桌面二进制覆盖环境变量名。默认 macOS 安装路径；Electron
// 必须以真实第二进程启动——`open -a` 在 Desktop 已运行时不转发 argv（实测
// 参数被丢弃），而直接 exec 会触发 Electron single-instance 的 second-instance
// 事件把 --open-workspace 交给主窗口（ZCode 主进程 index.js 已确认该 handler）。
const ZcodeDesktopBinEnv = "FLY_ZCODE_BIN"

// defaultZcodeDesktopBin 默认桌面二进制路径（const，注入缝走 env，见 zcodeDesktopBin）。
const defaultZcodeDesktopBin = "/Applications/ZCode.app/Contents/MacOS/ZCode"

// zcodeDesktopBin 解析桌面二进制路径：env 覆盖权威，缺省用官方安装位置。
func zcodeDesktopBin() string {
	if bin := os.Getenv(ZcodeDesktopBinEnv); bin != "" {
		return bin
	}
	return defaultZcodeDesktopBin
}

func defaultOpenZcodeWorkspace(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, zcodeDesktopBin(), "--open-workspace", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	// 注册是 fire-and-forget，但只 Start 不 Wait 会留僵尸进程（长跑随单元
	// 数累积）——Wait 仅负责回收，超时 kill 由 CommandContext 承担。
	go func() { _ = cmd.Wait() }()
	return nil
}

// disableDesktopRegistration 测试闸：Run 级测试不得激活用户桌面（隔离纪律）。
var disableDesktopRegistration = false

// RegisterZcodeWorkspace 在 ZCode 桌面端注册一个 workspace 路径（macOS-only），
// 并触发该路径 CLI 存储会话的索引导入。实测语义矩阵（2026-08-27）：
//   - 仅注册时刻处理 --open-workspace 才扫 ~/.zcode/cli/agents/ 导入该路径
//     会话进 tasks-index.sqlite；纯冷启动、暖状态静默均不导入；
//   - 重复调用幂等（second-instance 只切换/激活，无窗口风暴），因此每次
//     RunZcode 前触发 = 单元会话可见性随执行增量累积。
//
// 失败静默：可见性是 UX 增强，绝不阻塞执行链路；非 macOS/未装 Desktop/
// 超时都安全降级。（已知边缘：/tmp 符号链接路径可能因路径归一化不匹配
// 不导入——生产 Cwd 是真实项目/worktree 路径，不受影响。）
func RegisterZcodeWorkspace(path string) {
	if disableDesktopRegistration || zcodeDesktopGOOS != "darwin" || path == "" {
		return
	}
	// 索引按归一化路径做 workspace_key（实测 /tmp 符号链接注册成功但导入
	// 失配；EvalSymlinks 后同一路径立即导入）。解析失败退回原值。
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	_ = zcodeWorkspaceOpener(path)
}

// RunZcode 驱动一次 zcode headless 会话。凭证在用户侧 config.json 配置完成
// （OAuth 或 API Key），本函数不注入任何凭证。
//
// 桌面可见性双触发（实测语义见 DESIGN v0.20）：--open-workspace 的唯一导入
// 效果是"该路径此刻已存在的会话"——首次执行的全新路径在 spawn 前触发时存储
// 为空，什么都不导入。因此 spawn 前触发负责注册工作区并导入历史（resume/
// 重试场景），watchdog 返回后再触发一次把本会话立即导入 GUI（失败尝试的
// 部分对话同样入库，供排障）。执行进行中的会话仍不可见——导入是瞬时快照。
func RunZcode(a ZcodeArgs) Result {
	RegisterZcodeWorkspace(a.Cwd)
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
	// 后触发：本会话此刻已完整落盘（成功或失败），立即导入桌面索引。
	RegisterZcodeWorkspace(a.Cwd)

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
			// input 优先用上报值（2026-08-27 探针：inputTokens 恒在且与派生
			// 值不一致——total 的构成语义未经第二组探针复核，派生仅作缺报
			// fallback，负值 clamp 0 防 usage 倒挂）。
			input := z.Usage.InputTokens
			if input == 0 {
				input = total - reasoning - cacheRead - cacheWrite
				if input < 0 {
					input = 0
				}
			}
			out.Usage = &Usage{
				Input:       input,
				Output:      z.Usage.OutputTokens,
				Reasoning:   reasoning,
				CacheRead:   cacheRead,
				CacheWrite:  cacheWrite,
				TotalTokens: total,
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
