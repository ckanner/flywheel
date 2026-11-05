// Package drivers — driver 工厂（docs/design.md §13 绑定表）：config.drivers 按
// 角色选实现（mock 供测试/演练，pi 跑 Execute/Review/Judge，kimi 跑 Plan）。
// 分发只看绑定表的 harness（可能已被降级链改写），绝不看角色。
package drivers

import (
	"encoding/json"
	"fmt"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// Usage 单次会话用量（pi 流聚合；kimi 不暴露 → nil）。
type Usage struct {
	Input       int64 `json:"input"`
	Output      int64 `json:"output"`
	Reasoning   int64 `json:"reasoning"`
	CacheRead   int64 `json:"cacheRead"`
	CacheWrite  int64 `json:"cacheWrite"`
	TotalTokens int64 `json:"totalTokens"`
}

// Result driver 单次调用的完整结果：watchdog 可观察量 + harness 语义字段。
type Result struct {
	// watchdog 原始量。
	ExitCode     *int   `json:"exitCode"`
	Signal       string `json:"signal"`
	TimedOut     bool   `json:"timedOut"`
	Cancelled    bool   `json:"cancelled"`
	IdleTimedOut bool   `json:"idleTimedOut"`
	WallTimedOut bool   `json:"wallTimedOut"`
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	Truncated    bool   `json:"truncated"`
	DurationMS   int64  `json:"durationMs"`
	SpawnError   string `json:"spawnError,omitempty"`
	CleanupError string `json:"cleanupError,omitempty"`

	// harness 语义量。
	Harness        string   `json:"harness"` // pi | kimi | mock
	Provider       string   `json:"provider,omitempty"`
	Model          string   `json:"model,omitempty"`
	Usage          *Usage   `json:"usage,omitempty"` // kimi: nil（流不暴露）
	ReasoningChars *int64   `json:"reasoningChars,omitempty"`
	CostUSD        *float64 `json:"costUsd,omitempty"`
	Settled        bool     `json:"settled"`
	Text           string   `json:"text,omitempty"`
	ErrorMessage   *string  `json:"errorMessage,omitempty"`
	SessionID      string   `json:"sessionId,omitempty"` // kimi 长驻会话句柄
	// Preserve the exact public machine receipt, including partial usage and
	// physical cleanup facts, independently of the normalized success fields.
	CodexProReceipt json.RawMessage `json:"codexProReceipt,omitempty"`
}

// CallArgs 六角色统一入参：prompt 由 prompts 包组装；binding 来自 §13 绑定表
// （可能被降级链改写过 harness/channel）。
type CallArgs struct {
	Prompt      string
	MilestoneID string
	PacketID    string
	TaskDir     string
	Cwd         string
	Packets     []string // plan：既有 packet id 清单

	// 真实 harness 专用。
	Model          string // 空 → binding.Model 或驱动默认
	Binding        config.LoopBinding
	APIKey         string            // 渠道 env-key（仅此来源，绝不含 live 凭证）
	APIKeys        map[string]string // 注入子进程的键集
	Watchdog       watchdog.Opts
	OnEvent        func(line string) // stdout NDJSON 行回调
	SessionID      string            // plan 长驻会话句柄（machine 层持有；kimi -r <id> 续会话，空 = 新会话）
	PermissionMode string            // read-only | workspace-write | danger-full-access；由运行配置与角色显式指定
}

// Driver 六角色统一接口。mock 与 real 都实现。
type Driver interface {
	Name() string
	Plan(a CallArgs) Result
	Execute(a CallArgs) Result
	Review(a CallArgs) Result
	Judge(a CallArgs) Result
	Intake(a CallArgs) Result
	Garden(a CallArgs) Result
}

// Drivers 按角色装配的驱动集合（Node createDrivers 返回值等价）。
type Drivers struct {
	Plan    Driver
	Execute Driver
	Review  Driver
	Judge   Driver
	Intake  Driver
	Garden  Driver
}

// InvokeHarness 通用 harness 分发（Node invokeHarness）：harness 取自绑定表，
// 绝不由角色推断。空 harness 兼容旧 pi 配置，未知名称必须报错，不能换后端。
func InvokeHarness(a CallArgs, fallbackModel string) Result {
	harness := a.Binding.Harness
	if harness == "" {
		harness = "pi"
	}
	switch harness {
	case "kimi", "zcode", "codex", "codex-pro", "omp", "pi", "dsh":
	default:
		return invalidHarnessResult(harness, a.Model, fmt.Sprintf("unknown harness %q", harness))
	}
	if harness != "dsh" && a.Binding.Provider != "" {
		return invalidHarnessResult(harness, a.Model, "provider selection is supported only by the DSH harness")
	}
	if a.PermissionMode != "" && a.PermissionMode != "read-only" && a.PermissionMode != "workspace-write" && a.PermissionMode != "danger-full-access" {
		return invalidHarnessResult(harness, a.Model, "unsupported permission mode "+a.PermissionMode)
	}
	if a.PermissionMode == "danger-full-access" && harness != "codex" && harness != "codex-pro" && harness != "dsh" && harness != "kimi" {
		return invalidHarnessResult(harness, a.Model, fmt.Sprintf("harness %q has no verified full-access permission mapping", harness))
	}
	// Legacy adapters have no verified read-only sandbox mapping. workspace-write
	// remains their historical host-isolation contract, not an OS sandbox promise.
	if a.PermissionMode == "read-only" && harness != "codex" && harness != "codex-pro" && harness != "dsh" {
		return invalidHarnessResult(harness, a.Model, fmt.Sprintf("harness %q cannot enforce read-only permissions", harness))
	}
	if harness == "dsh" {
		model := a.Model
		if model == "" {
			model = a.Binding.Model
		}
		provider := a.Binding.Provider
		if provider == "" {
			provider = a.Binding.Channel // direct callers retain legacy DSH bindings
		}
		return RunDSH(DSHArgs{
			Prompt: a.Prompt, Provider: provider, Model: model, Thinking: a.Binding.Thinking,
			PermissionMode: a.PermissionMode, SessionID: a.SessionID, Cwd: a.Cwd,
			APIKeys: a.APIKeys, Watchdog: a.Watchdog, OnEvent: a.OnEvent,
		})
	}
	if harness == "kimi" {
		model := a.Model
		if model == "" {
			model = a.Binding.Model
		}
		if model == "" {
			model = KimiDefaultModel
		}
		return RunKimi(KimiArgs{
			Prompt:    a.Prompt,
			SessionID: a.SessionID, // plan 长驻会话：machine 持有句柄并传入
			Model:     model,
			Cwd:       a.Cwd,
			APIKeys:   a.APIKeys,
			Watchdog:  a.Watchdog,
			Thinking:  a.Binding.Thinking,
			OnEvent:   a.OnEvent,
		})
	}
	if harness == "zcode" {
		// v0.22：app-server 协议驱动——模型/思考档按会话指定（zai/ 命名空间）。
		zcodeModel := a.Model
		if zcodeModel == "" {
			zcodeModel = a.Binding.Model
		}
		return RunZcode(ZcodeArgs{
			Prompt:       a.Prompt,
			Model:        zcodeModel,
			ThoughtLevel: a.Binding.Thinking, // 空 → RunZcode 兜底 max（§7 统一档）
			Cwd:          a.Cwd,
			APIKeys:      a.APIKeys,
			Watchdog:     a.Watchdog,
		})
	}
	if harness == "codex" {
		codexModel := a.Model
		if codexModel == "" {
			codexModel = a.Binding.Model
		}
		return RunCodex(CodexArgs{
			Prompt:          a.Prompt,
			Model:           codexModel,
			ReasoningEffort: a.Binding.Thinking,
			PermissionMode:  a.PermissionMode,
			Cwd:             a.Cwd,
			APIKeys:         a.APIKeys,
			Watchdog:        a.Watchdog,
			OnEvent:         a.OnEvent,
		})
	}
	if harness == "codex-pro" {
		model := a.Model
		if model == "" {
			model = a.Binding.Model
		}
		return RunCodexPro(CodexProArgs{
			Prompt: a.Prompt, Model: model, ReasoningEffort: a.Binding.Thinking,
			PermissionMode: a.PermissionMode, SessionID: a.SessionID, Cwd: a.Cwd,
			APIKeys: a.APIKeys, Watchdog: a.Watchdog, OnEvent: a.OnEvent,
		})
	}
	if harness == "omp" {
		ompModel := a.Model
		if ompModel == "" {
			ompModel = a.Binding.Model
		}
		// 与 pi 分支同款：Thinking 取绑定值，空则 max——RunOmp 无条件拼
		// --thinking，漏传会把空串发给 CLI（omp 不报错、静默按缺省档跑，
		// 违反 §7 统一 max 档 + §10-2 fail-loud，PR #4 review P1）。
		thinking := a.Binding.Thinking
		if thinking == "" {
			thinking = "max"
		}
		return RunOmp(OmpArgs{
			Prompt:   a.Prompt,
			Model:    ompModel,
			Thinking: thinking,
			Cwd:      a.Cwd,
			APIKey:   a.APIKey,
			APIKeys:  a.APIKeys,
			Watchdog: a.Watchdog,
			OnEvent:  a.OnEvent,
		})
	}
	model := a.Model
	if model == "" {
		model = a.Binding.Model
	}
	thinking := a.Binding.Thinking
	if thinking == "" {
		thinking = "max"
	}
	return RunPi(PiArgs{
		Prompt:   a.Prompt,
		Model:    model,
		Thinking: thinking,
		Cwd:      a.Cwd,
		APIKey:   a.APIKey,
		APIKeys:  a.APIKeys,
		Watchdog: a.Watchdog,
		OnEvent:  a.OnEvent,
	})
}

// invalidHarnessResult 用稳定错误词汇返回配置错误，保证错误分类器不会重试。
func invalidHarnessResult(harness, model, detail string) Result {
	message := "invalid request: " + detail
	code := 1
	return Result{Harness: harness, Model: model, ExitCode: &code, ErrorMessage: &message}
}

// fromWatchdog 统一保留物理终态，避免某个后端漏传取消/清理失败而推进工作流。
func fromWatchdog(r watchdog.Result, harness, provider, model string) Result {
	return Result{
		ExitCode: r.ExitCode, Signal: r.Signal, TimedOut: r.TimedOut, Cancelled: r.Cancelled,
		IdleTimedOut: r.IdleTimedOut, WallTimedOut: r.WallTimedOut,
		Stdout: r.Stdout, Stderr: r.Stderr, Truncated: r.Truncated, DurationMS: r.DurationMS,
		SpawnError: r.SpawnError, CleanupError: r.CleanupError,
		Harness: harness, Provider: provider, Model: model,
	}
}

// realDriver 真实驱动：六角色全部走 invokeHarness 分发。
type realDriver struct{ model string }

func (realDriver) Name() string { return "real" }

func (d realDriver) harness(a CallArgs, prompt string) Result {
	a.Prompt = prompt
	return InvokeHarness(a, d.model)
}

func (d realDriver) Plan(a CallArgs) Result    { return d.harness(a, a.Prompt) }
func (d realDriver) Execute(a CallArgs) Result { return d.harness(a, a.Prompt) }
func (d realDriver) Review(a CallArgs) Result  { return d.harness(a, a.Prompt) }
func (d realDriver) Judge(a CallArgs) Result   { return d.harness(a, a.Prompt) }
func (d realDriver) Intake(a CallArgs) Result  { return d.harness(a, a.Prompt) }
func (d realDriver) Garden(a CallArgs) Result  { return d.harness(a, a.Prompt) }

// CreateDrivers 按 config.drivers 装配六角色：`mock` 选 mock 实现，否则真实
// 分发（Node createDrivers 等价）。
func CreateDrivers(cfg *config.Config) *Drivers {
	isMock := func(role string) bool { return cfg.Drivers[role] == "mock" }
	mock := NewMockDriver(cfg.MockBehaviors)
	var real Driver = realDriver{}
	pick := func(role string) Driver {
		if isMock(role) {
			return mock
		}
		return real
	}
	return &Drivers{
		Plan:    pick("plan"),
		Execute: pick("execute"),
		Review:  pick("review"),
		Judge:   pick("judge"),
		Intake:  pick("intake"),
		Garden:  pick("garden"),
	}
}
