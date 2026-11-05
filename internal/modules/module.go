// Package modules — 外挂模块（docs/design.md §4.5/§4.6）：Intake Loop 与 Garden
// Loop 的共享调用骨架（module.js 移植）。轻量版 run-task 上下文：同款 watchdog
// 参数、凭证卫生、传输重试 + 渠道降级链、逐次 spawn 指标。预算用量在任务目录
// 有 state.json 时记入（记录永不关闭，铁律 9）；任务尚不存在时仅追加
// metrics.jsonl。
package modules

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// Ctx 模块共享上下文（Node createModuleContext 等价 + 可注入缝）。
type Ctx struct {
	TaskDir     string
	Cwd         string
	Config      *config.Config
	Drivers     *drivers.Drivers
	Log         func(string)
	Sleep       func(ms int64)
	MetricsFile string

	// EnvLookup 渠道 env 读取（Node 直读 process.env；注入缝供测试）。
	EnvLookup childenv.LookupEnv
	fatalErr  error // persistence/cancellation/unknown cleanup stops later model calls
}

// CtxOpts NewContext 命名参数（Node 解构默认值等价）。
type CtxOpts struct {
	TaskDir string
	Cwd     string // 空 → process.cwd()
	Config  *config.Config
	Drivers *drivers.Drivers
	Log     func(string) // nil → 丢弃
	Sleep   func(ms int64)
}

// NewContext 构造模块上下文并回填缺省。
func NewContext(o CtxOpts) *Ctx {
	cwd := o.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	logf := o.Log
	if logf == nil {
		logf = func(string) {}
	}
	sleep := o.Sleep
	if sleep == nil {
		sleep = func(ms int64) { time.Sleep(time.Duration(ms) * time.Millisecond) }
	}
	return &Ctx{
		TaskDir:     o.TaskDir,
		Cwd:         cwd,
		Config:      o.Config,
		Drivers:     o.Drivers,
		Log:         logf,
		Sleep:       sleep,
		MetricsFile: filepath.Join(o.TaskDir, "metrics.jsonl"),
		EnvLookup:   childenv.OSLookupEnv,
	}
}

// ModuleWatchdogOpts 模块调用的看门狗参数（与 run-task 同源配置）。
func ModuleWatchdogOpts(cfg *config.Config) watchdog.Opts {
	return watchdog.Opts{
		IdleTimeoutMs: cfg.Watchdog.IdleTimeoutMs,
		WallTimeoutMs: cfg.Watchdog.WallTimeoutMs,
		KillGraceMs:   cfg.Watchdog.KillGraceMs,
	}
}

// ChannelAPIKey 该渠道 env-key 的现值；未设或订阅渠道为空串（Node null 等价）。
func ChannelAPIKey(cfg *config.Config, channel string, lookup childenv.LookupEnv) string {
	envKey := fallback.EnvKeyForChannel(cfg, channel)
	if envKey == "" {
		return ""
	}
	if v, ok := lookup(envKey); ok {
		return v
	}
	return ""
}

// APIKeysFor 注入子进程的键集：恰好一个渠道键（§9 凭证卫生）。
func APIKeysFor(cfg *config.Config, channel string, lookup childenv.LookupEnv) map[string]string {
	envKey := fallback.EnvKeyForChannel(cfg, channel)
	if envKey == "" {
		return map[string]string{}
	}
	if v, ok := lookup(envKey); ok {
		return map[string]string{envKey: v}
	}
	return map[string]string{}
}

// recordModuleMetrics 一次 agent 调用的指标落盘 + 预算记账。Token/cost 记录
// 永不关闭——任务目录有 state.json 就写入；但编排器持有 run lease 或 PID 时绝不写：
// 模块 mid-run 写 state 会抬 revision，砸掉 run 下一次 CAS 保存（P0-10，
// 用户实测）。metrics.jsonl 不受影响（追加式）。
func recordModuleMetrics(m *Ctx, role, model, channel, prompt string, result drivers.Result, code errs.ErrorCode) error {
	var estUsage *metrics.Usage // 成本估算输入（cacheWrite 参与计费）
	if result.Usage != nil {
		estUsage = &metrics.Usage{
			Input:      result.Usage.Input,
			Output:     result.Usage.Output,
			CacheRead:  result.Usage.CacheRead,
			CacheWrite: result.Usage.CacheWrite,
		}
	}
	costUSD := result.CostUSD
	if costUSD == nil {
		costUSD = metrics.EstimateCostUSD(estUsage, metrics.PriceForModel(m.Config.Pricing, model))
	}
	var cacheHitRate any
	if result.Usage != nil && result.Usage.Input > 0 {
		cacheHitRate = round3(float64(result.Usage.CacheRead) / float64(result.Usage.Input))
	}
	var promptHash any
	if prompt != "" {
		promptHash = metrics.PromptHash(prompt)
	}
	entry := map[string]any{
		"role":              role,
		"packet-id":         nil,
		"milestone":         nil,
		"model":             nilIfEmpty(model),
		"harness":           nilIfEmpty(result.Harness),
		"provider":          nilIfEmpty(result.Provider),
		"channel":           nilIfEmpty(channel),
		"prompt-hash":       promptHash,
		"usage":             result.Usage,
		"cache-hit-rate":    cacheHitRate,
		"reasoning-chars":   result.ReasoningChars,
		"cost-usd":          costUSD,
		"duration-ms":       result.DurationMS,
		"exit-code":         result.ExitCode,
		"settled":           result.Settled,
		"cancelled":         result.Cancelled,
		"timed-out":         result.TimedOut,
		"truncated":         result.Truncated,
		"cleanup-error":     nilIfEmpty(result.CleanupError),
		"error-code":        nilIfEmpty(string(code)),
		"attempt":           nil,
		"codex-pro-receipt": result.CodexProReceipt,
	}
	if err := metrics.Append(m.MetricsFile, entry); err != nil {
		return err
	}

	usage := result.Usage
	saved, err := updateIdleTaskState(m.TaskDir, func(st *state.State) {
		day := time.Now().UTC().Format("2006-01-02")
		if st.Budget.Daily == nil {
			st.Budget.Daily = map[string]state.DailyBucket{}
		}
		bucket := st.Budget.Daily[day]
		if usage == nil {
			st.Budget.UnknownUsageCalls++
			bucket.UnknownUsageCalls++
			warning := "Some harness calls did not report usage; token/cost totals are measured lower bounds, not a complete bill."
			if !slices.Contains(st.Budget.Warnings, warning) {
				st.Budget.Warnings = append(st.Budget.Warnings, warning)
			}
		}
		if costUSD == nil {
			st.Budget.UnknownCostCalls++
			bucket.UnknownCostCalls++
		}
		st.Budget.Daily[day] = bucket
		if usage != nil {
			total := usage.TotalTokens
			state.RecordUsage(st, state.Usage{
				Input:       usage.Input,
				Output:      usage.Output,
				Reasoning:   usage.Reasoning,
				CacheRead:   usage.CacheRead,
				CacheWrite:  usage.CacheWrite,
				TotalTokens: &total,
			}, state.UsageOpts{Provider: firstNonEmpty(result.Provider, channel, "unknown"), CostUSD: costUSD, Day: day})
		}
	})
	if err != nil {
		m.Log("budget recording skipped: " + err.Error())
	} else if !saved && state.Exists(m.TaskDir) {
		m.Log("orchestrator is running — state.json budget write skipped (metrics.jsonl still recorded)")
	}
	return nil
}

// CallResult callAgent 结论。
type CallResult struct {
	OK        bool
	Text      string
	ErrorCode errs.ErrorCode // OK 时为 ""
	Channel   string
	Model     string
}

// CallOptions callAgent 入参（verdict 解析归各模块自己的 schema）。
type CallOptions struct {
	Role      string // plan|execute|review|judge|intake|garden
	Binding   fallback.Binding
	Prompt    string
	SessionID string // kimi 长驻会话句柄；模块调用恒空
}

// CallAgent 标准 transport-retry + channel-fallback 链（§6）的一次 agent 调用。
// 成功返回助手文本；失败经完整重试链后返回稳定错误码。
func CallAgent(m *Ctx, o CallOptions) CallResult {
	if m.fatalErr != nil {
		return CallResult{ErrorCode: errs.TRANSPORT, Channel: o.Binding.Channel, Model: o.Binding.Model}
	}
	channel := o.Binding.Channel
	current := o.Binding
	current.Provider = fallback.ResolveProvider(m.Config, current.Harness, current.Provider, channel)
	visited := map[string]bool{}
	maxTransport := m.Config.Retry.MaxTransportRetries
	if maxTransport <= 0 {
		maxTransport = 5 // Normalize 后恒非零；直构 Config 的调用方按 Node ?? 缺省兜底
	}
	lastCode := errs.TRANSPORT

	for t := 0; t <= maxTransport; t++ { // 经典 for：降级 t-- 必须真实回退计数
		// P1-12②：key 渠道缺 env 时 fail-loud，绝不 keyless 裸 spawn 进 pi 的
		// auth.json 陷阱。mock 驱动豁免（演练场景无真实凭证）。
		if m.Config.Drivers[o.Role] != "mock" {
			if missingKey := fallback.MissingChannelEnv(m.Config, channel, m.EnvLookup); missingKey != "" {
				m.Log(o.Role + ": fail-loud — channel " + channel + " requires env " + missingKey + " (not set)")
				return CallResult{ErrorCode: errs.AUTH, Channel: channel, Model: current.Model}
			}
		}
		visited[channel] = true
		permission := m.Config.PermissionMode(o.Role, current.Harness)
		result := dispatchRole(m.Drivers, o.Role, drivers.CallArgs{
			Prompt:         o.Prompt,
			TaskDir:        m.TaskDir,
			Cwd:            m.Cwd,
			Model:          current.Model,
			Binding:        config.LoopBinding{Harness: current.Harness, Provider: current.Provider, Model: current.Model, Channel: current.Channel, Thinking: current.Thinking},
			APIKey:         ChannelAPIKey(m.Config, channel, m.EnvLookup),
			APIKeys:        APIKeysFor(m.Config, channel, m.EnvLookup),
			Watchdog:       ModuleWatchdogOpts(m.Config),
			SessionID:      o.SessionID,
			PermissionMode: permission,
		})

		// Physical cancellation/timeout/cleanup dominates text classification.
		// A protocol terminal marker is required even if the CLI exits zero.
		var code errs.ErrorCode
		switch {
		case result.Cancelled:
			code = errs.TRANSPORT
		case result.TimedOut:
			code = errs.TIMEOUT
		case result.SpawnError != "" || result.CleanupError != "" || result.Truncated || result.Signal != "" ||
			(result.ErrorCode != "" && (result.ExitCode == nil || *result.ExitCode != 1)):
			code = errs.TRANSPORT
		case result.ErrorCode != "":
			code = errs.ClassifyError(errs.SpawnResult{ExitCode: 1, ErrorCode: result.ErrorCode})
		case result.ErrorMessage != nil && *result.ErrorMessage != "":
			code = errs.ClassifyError(errs.SpawnResult{ExitCode: 1, Stderr: *result.ErrorMessage})
		case result.ExitCode == nil || (!result.Settled && *result.ExitCode == 0):
			code = errs.TRANSPORT
		case *result.ExitCode == 0:
			code = errs.ClassifyError(errs.SpawnResult{Stderr: result.Stderr})
		default:
			code = errs.ClassifyError(errs.SpawnResult{
				ExitCode: *result.ExitCode, Stderr: result.Stderr, Stdout: result.Stdout,
			})
		}
		if code != "" {
			lastCode = code
		}
		if err := recordModuleMetrics(m, o.Role, current.Model, channel, o.Prompt, result, code); err != nil {
			m.fatalErr = err
			m.Log("metrics recording failed; further model calls stopped: " + err.Error())
		}
		if m.stopForUnconfirmedCleanup(o.Role, result) {
			return CallResult{ErrorCode: code, Channel: channel, Model: current.Model}
		}
		if m.fatalErr != nil {
			return CallResult{ErrorCode: errs.TRANSPORT, Channel: channel, Model: current.Model}
		}
		if result.Cancelled {
			m.fatalErr = errors.New("module agent call cancelled")
			return CallResult{ErrorCode: code, Channel: channel, Model: current.Model}
		}

		if code == "" {
			text := result.Text
			if text == "" {
				text = result.Stdout // Node: result.text ?? result.stdout ??
			}
			return CallResult{OK: true, Text: text, Channel: channel, Model: current.Model}
		}

		m.Log(o.Role + " transport " + strconv.Itoa(t) + ": " + string(code))

		if errs.ShouldFallback(code) || fallback.AuthFallbackAllowed(m.Config, channel, code) {
			next := fallback.NextChannel(m.Config, channel)
			mapped := fallback.ModelForChannel(m.Config, next, o.Role, "")
			if next != "" && !visited[next] && mapped != nil {
				m.Log(o.Role + " fallback: " + channel + " -> " + next + " (" + string(code) + ")")
				channel = next
				harness := mapped.Harness
				if harness == "" {
					harness = current.Harness
				}
				current = fallback.Binding{Harness: harness, Provider: fallback.ResolveProvider(m.Config, harness, "", next), Model: mapped.Model, Channel: next, Thinking: o.Binding.Thinking}
				t-- // 降级不消耗 transport 重试名额
				continue
			}
			if next != "" && mapped == nil {
				// P1-12①：下一渠道未声明该角色映射——跨厂商组合非法，链终止。
				m.Log(o.Role + " fallback chain ends: no model mapping for " + o.Role + " on " + next)
			}
		}
		if !errs.IsRetryable(code) {
			return CallResult{ErrorCode: code, Channel: channel, Model: current.Model}
		}
		delay := errs.ComputeBackoff(t, errs.BackoffOptions{
			BaseDelayMs:  m.Config.Retry.BaseDelayMs,
			MaxDelayMs:   m.Config.Retry.MaxDelayMs,
			Jitter:       m.Config.Retry.Jitter,
			RetryAfterMs: float64(errs.ExtractRetryAfterMs(result.Stderr + "\n" + result.Stdout)),
		})
		m.Sleep(delay)
	}
	return CallResult{ErrorCode: lastCode, Channel: channel, Model: current.Model}
}

// dispatchRole 角色分派（Node mctx.drivers[role][role](args) 等价：在角色的
// Driver 字段上调同名接口方法）。
func dispatchRole(d *drivers.Drivers, role string, a drivers.CallArgs) drivers.Result {
	switch role {
	case "plan":
		return d.Plan.Plan(a)
	case "execute":
		return d.Execute.Execute(a)
	case "review":
		return d.Review.Review(a)
	case "judge":
		return d.Judge.Judge(a)
	case "intake":
		return d.Intake.Intake(a)
	case "garden":
		return d.Garden.Garden(a)
	}
	panic("modules: unknown driver role " + role)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }
