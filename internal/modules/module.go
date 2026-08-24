// Package modules — 外挂模块（DESIGN.md §4.5/§4.6）：Intake Loop 与 Garden
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
// 永不关闭——任务目录有 state.json 就写入；但编排器运行中（pid 文件存在）绝不写：
// 模块 mid-run 写 state 会抬 revision，砸掉 run 下一次 CAS 保存（P0-10，
// 用户实测）。metrics.jsonl 不受影响（追加式）。
func recordModuleMetrics(m *Ctx, role, model, channel, prompt string, result drivers.Result, code errs.ErrorCode) {
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
		"role":            role,
		"packet-id":       nil,
		"milestone":       nil,
		"model":           nilIfEmpty(model),
		"harness":         nilIfEmpty(result.Harness),
		"channel":         nilIfEmpty(channel),
		"prompt-hash":     promptHash,
		"usage":           result.Usage,
		"cache-hit-rate":  cacheHitRate,
		"reasoning-chars": result.ReasoningChars,
		"cost-usd":        costUSD,
		"duration-ms":     result.DurationMS,
		"exit-code":       result.ExitCode,
		"error-code":      nilIfEmpty(string(code)),
		"attempt":         nil,
	}
	_ = metrics.Append(m.MetricsFile, entry)

	usage := result.Usage
	if usage == nil {
		return
	}
	orchestratorRunning := exists(filepath.Join(m.TaskDir, "orchestrator.pid"))
	switch {
	case !state.Exists(m.TaskDir):
		// 任务尚未建立：只有 metrics.jsonl（调用方多为 intake/garden 外挂）。
	case orchestratorRunning:
		m.Log("orchestrator is running — state.json budget write skipped (metrics.jsonl still recorded)")
	default:
		st, err := state.Load(m.TaskDir)
		if err == nil {
			total := usage.TotalTokens
			state.RecordUsage(st, state.Usage{
				Input:       usage.Input,
				Output:      usage.Output,
				Reasoning:   usage.Reasoning,
				CacheRead:   usage.CacheRead,
				CacheWrite:  usage.CacheWrite,
				TotalTokens: &total,
			}, state.UsageOpts{Provider: firstNonEmpty(result.Provider, channel, "unknown"), CostUSD: costUSD})
			_, err = state.Save(m.TaskDir, st, nil) // nil → 以 st.Revision 为期望值（Node expectedRevision: state.revision）
		}
		if err != nil {
			m.Log("budget recording skipped: " + err.Error())
		}
	}
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
	channel := o.Binding.Channel
	current := o.Binding
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
		result := dispatchRole(m.Drivers, o.Role, drivers.CallArgs{
			Prompt:    o.Prompt,
			TaskDir:   m.TaskDir,
			Cwd:       m.Cwd,
			Model:     current.Model,
			Binding:   config.LoopBinding{Harness: current.Harness, Model: current.Model, Channel: current.Channel, Thinking: current.Thinking},
			APIKey:    ChannelAPIKey(m.Config, channel, m.EnvLookup),
			APIKeys:   APIKeysFor(m.Config, channel, m.EnvLookup),
			Watchdog:  ModuleWatchdogOpts(m.Config),
			SessionID: o.SessionID,
		})

		// Node 三元链（module.js:121-125）：errorMessage → 按其分类；
		// 干净退出（0/无超时/无 spawnError）→ 无错误；其余 → 分类，空则兜底
		// TRANSPORT。空产出（exit 0 无交付物）由各模块自己的 schema 判定，
		// 不在此处。
		var code errs.ErrorCode
		switch {
		case result.ErrorMessage != nil:
			code = errs.ClassifyError(errs.SpawnResult{ExitCode: 1, Stderr: *result.ErrorMessage})
		case result.ExitCode != nil && *result.ExitCode == 0 && !result.TimedOut && result.SpawnError == "":
			code = ""
		default:
			exit := 0
			if result.ExitCode != nil {
				exit = *result.ExitCode
			}
			var spawnErr error
			if result.SpawnError != "" {
				spawnErr = errors.New(result.SpawnError)
			}
			code = errs.ClassifyError(errs.SpawnResult{
				ExitCode: exit, Signal: result.Signal, TimedOut: result.TimedOut,
				Stderr: result.Stderr, Stdout: result.Stdout, SpawnError: spawnErr,
			})
			if code == "" {
				code = errs.TRANSPORT
			}
		}
		if code != "" {
			lastCode = code
		}
		recordModuleMetrics(m, o.Role, current.Model, channel, o.Prompt, result, code)

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
					harness = o.Binding.Harness
				}
				current = fallback.Binding{Harness: harness, Model: mapped.Model, Channel: next, Thinking: o.Binding.Thinking}
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
