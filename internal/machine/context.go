// Package machine — 核心状态机（docs/design.md §6，刻意做薄）：
//
//	for milestone in spec.milestones:
//	  packet = plan_loop.issue(milestone)
//	  for attempt in range(packet.escalation.max_retries):
//	    result = execute_loop.run(packet)      # 新会话 + 看门狗
//	    l1 = verify.run(packet.acceptance.L1)  # 编排器直接跑
//	    verdict = review_loop.review(...) if l1.pass else fail(l1)
//	    if verdict.pass and verify.milestone_gate(milestone): break
//	  else: escalate(result)                   # 回 Plan Loop / inbox 叫人
//
// 编排器不是 agent：下面的每个决策都是机械可查的状态转移（state.json 是否
// 推进、退出码是否为 0、重试/预算是否耗尽）。任何需要判断的事都交给
// Plan/Review agent 或人（铁律 1）。
//
// 与 Node 版的语义差异（均为有意为之，其余逐函数忠实移植）：
//   - P2 修复：in-place 模式 planner 修订后 base-ref 同样重锚（Node 只在
//     worktree 分支重锚，v0.17 审计项（docs/design.md 变更记录））。
//   - P2 修复：空转检测的 changedFiles 优先取 L1 boundary 的权威 git diff，
//     executor 自报只作最后兜底（Node 直接用自报值）。
//   - P2 修复：单元提交先于 completed 标记落盘（Node 先写 completed 再提交，
//     崩溃窗口内 state.json 谎报完成）。
//   - P3 修复：resume 到 review 阶段时补跑缺陷注入采样（Node 跳过）。
//   - P1 修复：RunTask 注册 SIGTERM（Node 定义了 handler 却从未注册，
//     kill switch 失效）；Go 版 signal.Notify → 杀在飞子进程组 → quiescence
//     → exit 143。
//   - Node 的 saveState 抛异常沿调用栈炸出 runTask；Go 版把首次保存失败记入
//     ctx.fatal，在各循环边界与每次 spawn 前检查——可观察行为一致（run 以错误
//     结束），但避免在 I/O 已损坏时继续烧钱 spawn。
package machine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/circuitbreaker"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/ledger"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/state"
)

// Context 一次运行的完整上下文。Sleep/Now/Log/EnvLookup 全部可注入
// （测试注入假时钟与假 sleep；零值语义 = 真实环境）。
type Context struct {
	TaskDir    string
	Cwd        string
	Config     *config.Config
	Drivers    *drivers.Drivers
	Spec       *contract.Spec
	Packets    map[string]*contract.Packet // packet-id → packet
	PacketList []*contract.Packet          // 保持契约文件顺序（plan/judge 用）
	State      *state.State
	Fresh      bool // 本次运行新建了 state.json（$flywheel-new 之后首跑）

	Log          func(string)
	Sleep        func(time.Duration) // Node ctx.sleep(ms)；Go 直接收 Duration
	Now          func() time.Time
	EnvLookup    childenv.LookupEnv
	RunContext   context.Context
	controlToken string

	metricsFile string
	pauseBefore string
	revision    int64
	fatal       error // 首次保存失败（CAS 冲突 / I/O）；循环边界检查后终止
}

// Opts CreateContext 入参（对应 Node createContext 参数对象）。
type Opts struct {
	TaskDir string
	Cwd     string // 空 = 进程工作目录
	Config  *config.Config
	Drivers *drivers.Drivers
	Log     func(string)
	Sleep   func(time.Duration)
	Now     func() time.Time
	Env     childenv.LookupEnv
	Context context.Context
}

// nowISO 注入时钟的 ISO 序列化（Node toISOString：UTC 毫秒精度）。
func (c *Context) nowISO() string {
	return c.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

// err 返回首个致命保存错误（若有）。调用方在各循环边界检查。
func (c *Context) err() error {
	if c.fatal != nil {
		return c.fatal
	}
	if c.RunContext != nil {
		return c.RunContext.Err()
	}
	return nil
}

// save CAS 保存工作状态；一切状态变更都从这里走。失败记入 fatal（见包注释）。
func (c *Context) save() {
	if c.fatal != nil {
		return
	}
	st, err := state.Save(c.TaskDir, c.State, &c.revision)
	if err != nil {
		c.fatal = err
		return
	}
	c.State = st
	c.revision = st.Revision
}

// recordRetry 重试历史追加（审计：错误码、策略、计划延迟全落 state.json）。
func (c *Context) recordRetry(ps *state.PacketState, scope, errorCode, policy string, plannedDelayMS ...int64) {
	entry := map[string]any{
		"at":               c.nowISO(),
		"scope":            scope,
		"error-code":       errorCode,
		"policy":           policy,
		"planned-delay-ms": 0,
	}
	if len(plannedDelayMS) > 0 {
		entry["planned-delay-ms"] = plannedDelayMS[0]
	}
	ps.RetryHistory = append(ps.RetryHistory, entry)
}

func (c *Context) touchProgress() {
	at := c.nowISO()
	c.State.LastProgressAt = &at
}

// recordSpawnMetrics 每次 spawn 追加一条 metrics.jsonl 记录并把用量记账进
// state（任何档位下都不关闭，铁律 9）。
func (c *Context) recordSpawnMetrics(a spawnMetricsArgs) {
	var usage *drivers.Usage
	if a.result != nil {
		usage = a.result.Usage
	}
	var costUSD *float64
	if a.result != nil && a.result.CostUSD != nil {
		costUSD = a.result.CostUSD
	} else if usage != nil {
		costUSD = metrics.EstimateCostUSD(&metrics.Usage{
			Input:      usage.Input,
			Output:     usage.Output,
			CacheRead:  usage.CacheRead,
			CacheWrite: usage.CacheWrite, // v0.18 修复（C1）：漏传导致缓存写入按 0 计价
		}, metrics.PriceForModel(c.Config.Pricing, a.model))
	}
	var cacheHitRate *float64
	if usage != nil && usage.Input > 0 {
		v := mathRound((float64(usage.CacheRead)/float64(usage.Input))*1000) / 1000
		cacheHitRate = &v
	}

	var packetID any
	milestone := any(nil)
	if a.packetID != "" {
		packetID = a.packetID
		if m, err := contract.MilestoneOfPacket(a.packetID); err == nil {
			milestone = m
		}
	}
	var model any
	if a.model != "" {
		model = a.model
	}
	var harness any
	if a.result != nil && a.result.Harness != "" {
		harness = a.result.Harness
	}
	var effectiveProvider any
	if a.result != nil && a.result.Provider != "" {
		effectiveProvider = a.result.Provider
	}
	var channel any
	if a.channel != "" {
		channel = a.channel
	}
	var promptHash any
	if a.prompt != "" {
		promptHash = metrics.PromptHash(a.prompt)
	}
	var usageVal any
	if usage != nil {
		usageVal = *usage
	}
	var costVal any
	if costUSD != nil {
		costVal = *costUSD
	}
	var durationMS any
	if a.result != nil {
		durationMS = a.result.DurationMS
	}
	var exitCode any
	if a.result != nil && a.result.ExitCode != nil {
		exitCode = *a.result.ExitCode
	}
	var errorCode any
	if a.errorCode != "" {
		errorCode = string(a.errorCode)
	}
	var attempt any
	if a.attempt != nil {
		attempt = *a.attempt
	}
	var resultState any
	if a.result != nil {
		resultState = map[string]any{
			"settled": a.result.Settled, "cancelled": a.result.Cancelled,
			"truncated": a.result.Truncated, "signal": a.result.Signal,
			"cleanup-error": a.result.CleanupError, "session-id": a.result.SessionID,
		}
	}
	if err := metrics.Append(c.metricsFile, map[string]any{
		"role":            a.role,
		"packet-id":       packetID,
		"milestone":       milestone,
		"model":           model,
		"harness":         harness,
		"provider":        effectiveProvider,
		"channel":         channel,
		"prompt-hash":     promptHash,
		"usage":           usageVal,
		"cache-hit-rate":  cacheHitRate,
		"reasoning-chars": reasoningCharsOf(a.result),
		"cost-usd":        costVal,
		"duration-ms":     durationMS,
		"exit-code":       exitCode,
		"error-code":      errorCode,
		"attempt":         attempt,
		"result":          resultState,
	}); err != nil && c.fatal == nil {
		c.fatal = fmt.Errorf("metrics could not be persisted: %w", err)
	}
	day := c.Now().UTC().Format("2006-01-02")
	bucket := c.State.Budget.Daily[day]
	if usage == nil {
		c.State.Budget.UnknownUsageCalls++
		bucket.UnknownUsageCalls++
	}
	if costUSD == nil {
		c.State.Budget.UnknownCostCalls++
		bucket.UnknownCostCalls++
	}
	c.State.Budget.Daily[day] = bucket

	if usage == nil {
		warning := "Some harness calls did not report usage; token/cost totals are measured lower bounds, not a complete bill."
		if !containsString(c.State.Budget.Warnings, warning) {
			c.State.Budget.Warnings = append(c.State.Budget.Warnings, warning)
		}
		return
	}
	provider := ""
	if a.result != nil && a.result.Provider != "" {
		provider = a.result.Provider
	} else if a.channel != "" {
		provider = a.channel
	} else {
		provider = "unknown"
	}
	tt := usage.TotalTokens
	if tt == 0 {
		tt = usage.Input + usage.Output // 上游未报 total（零值构造）→ Node 语义 i+o
	}
	state.RecordUsage(c.State, state.Usage{
		Input: usage.Input, Output: usage.Output, Reasoning: usage.Reasoning,
		CacheRead: usage.CacheRead, CacheWrite: usage.CacheWrite, TotalTokens: &tt,
	}, state.UsageOpts{Provider: provider, CostUSD: costUSD, Day: day})
	c.warnIfCacheCold(provider, a.channel)
}

type spawnMetricsArgs struct {
	role      string
	packetID  string
	model     string
	channel   string
	prompt    string
	result    *drivers.Result
	errorCode errs.ErrorCode
	attempt   *int
}

func reasoningCharsOf(r *drivers.Result) any {
	if r == nil || r.ReasoningChars == nil {
		return nil
	}
	return *r.ReasoningChars
}

func mathRound(v float64) float64 { return float64(int64(v + 0.5)) } // 正数场景 ≈ Math.round

// warnIfCacheCold §8-7：缓存命中率低于渠道阈值 → 软警告（从不停机；
// 突降意味着提示词前缀被破坏）。
func (c *Context) warnIfCacheCold(provider, channel string) {
	bp, ok := c.State.Budget.Cache.ByProvider[provider]
	if !ok {
		return
	}
	threshold := 0.3
	if ch, ok := c.Config.Channels[channel]; ok && ch.CacheHitRateMin != nil {
		threshold = *ch.CacheHitRateMin
	}
	if bp.Input >= 1000 && bp.HitRate < threshold {
		msg := fmt.Sprintf("cache hit-rate %.1f%% below threshold %d%% for %s",
			bp.HitRate*100, int(mathRound(threshold*100)), provider)
		if !containsString(c.State.Budget.Warnings, msg) {
			c.State.Budget.Warnings = append(c.State.Budget.Warnings, msg)
			c.Log(msg)
		}
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// apiKeysFor 渠道 env-key 指向的环境变量当前值（注入子进程的最小键集）。
func (c *Context) apiKeysFor(channel string) map[string]string {
	envKey := fallback.EnvKeyForChannel(c.Config, channel)
	if envKey != "" {
		if v, ok := c.EnvLookup(envKey); ok {
			return map[string]string{envKey: v}
		}
	}
	return map[string]string{}
}

// CONTRACT_FILE_RE 边界判定针对的是 EXECUTOR 的 diff；任务目录里只有编排器
// 自己的账本文件被排除（core/ledger.js 常量单一信源）——契约文件
// （spec.md / task-packet-*.md / config）留在判定内：executor 篡改契约即违规。
var contractFileRe = regexp.MustCompile(`^(spec\.md|task-packet-.*\.md|flywheel\.config\.json)$`)

func (c *Context) taskDirPrefix() string {
	rel, err := filepath.Rel(c.Cwd, c.TaskDir)
	if err != nil {
		// Node relative() 失败抛异常；此处 TaskDir 恒在 Cwd 之下，防御分支
		// 仅在病态输入（跨盘符等）触达——退化为绝对路径前缀。
		return c.TaskDir + "/"
	}
	if rel == "." {
		return ""
	}
	return rel + "/"
}

func (c *Context) ledgerExcludes() []string {
	prefix := c.taskDirPrefix()
	out := make([]string, 0, len(ledger.LedgerFiles)+len(ledger.LedgerDirs))
	for f := range ledger.LedgerFiles {
		out = append(out, prefix+f)
	}
	for _, d := range ledger.LedgerDirs {
		out = append(out, prefix+d)
	}
	return out
}

// commitExcludes 单元提交排除的易变运行时文件（活日志/metrics/pid 绝不进 git）。
func (c *Context) commitExcludes() []string {
	prefix := c.taskDirPrefix()
	out := make([]string, 0, len(ledger.CommitVolatileFiles))
	for f := range ledger.CommitVolatileFiles {
		out = append(out, prefix+f)
	}
	return out
}

// partitionChangedFiles 把变更文件切成 executor 文件与契约违规。
// changedFiles 为 nil（diff 不可用）→ 全部跳过（skip 记录进证据）。
func (c *Context) partitionChangedFiles(changedFiles []string) (executorFiles []string, contractViolations []boundaryViolationLite) {
	if changedFiles == nil {
		return nil, nil
	}
	prefix := c.taskDirPrefix()
	executorFiles = []string{}
	for _, f := range changedFiles {
		if !strings.HasPrefix(f, prefix) {
			executorFiles = append(executorFiles, f)
			continue
		}
		inner := f[len(prefix):]
		if ledger.IsLedgerTaskPath(inner) {
			continue
		}
		if contractFileRe.MatchString(inner) {
			contractViolations = append(contractViolations, boundaryViolationLite{Type: "contract-file", Path: f})
		} else {
			executorFiles = append(executorFiles, f)
		}
	}
	return executorFiles, contractViolations
}

// boundaryViolationLite partitionChangedFiles 产出的违规（与 verify.BoundaryViolation
// 同形，type=contract-file 恒有 path 无 count）。
type boundaryViolationLite struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
}

// channelApiKey 渠道 API key 原始值（供 pi --api-key）；绝不含 live 凭证审批集。
func (c *Context) channelApiKey(channel string) string {
	envKey := fallback.EnvKeyForChannel(c.Config, channel)
	if envKey == "" {
		return ""
	}
	if v, ok := c.EnvLookup(envKey); ok {
		return v
	}
	return ""
}

// missingKeyForRealDriver fail-loud 守卫（P1-12②）：声明了 env-key 却未设的
// 渠道绝不 spawn——否则 pi 会静默回退到全局 auth.json。仅对真实驱动强制
// （mock 不需要 key）。
func (c *Context) missingKeyForRealDriver(role, channel string) string {
	if c.Config.Drivers[role] == "mock" {
		return ""
	}
	return fallback.MissingChannelEnv(c.Config, channel, c.EnvLookup)
}

// budgetGate 每次 spawn 前的预算闸门。返回 'ok' | 'stop'。
// token/成本记录本身在任何 enforce 档位下都不关闭。
func (c *Context) budgetGate() string {
	verdict := circuitbreaker.CheckBudget(c.State, c.Config, c.Now())
	switch verdict.Action {
	case circuitbreaker.ActionStop:
		c.Log(fmt.Sprintf("budget HARD stop: %s", verdict.Reason))
		c.State.Status = "budget-stopped"
		c.save()
		return "stop"
	case circuitbreaker.ActionWarn:
		// P2：按天去重——消息里放易变计数会每次 spawn 新增一条告警撑爆
		// state.json。
		msg := fmt.Sprintf("budget soft warning: daily token budget exceeded (%s)", c.Now().UTC().Format("2006-01-02"))
		if !containsString(c.State.Budget.Warnings, msg) {
			c.State.Budget.Warnings = append(c.State.Budget.Warnings, msg)
		}
		c.Log(msg)
	}
	return "ok"
}

// ---- 单实例守卫 / pid 文件 ----

// PidFile 编排器 pid 文件路径。
func PidFile(taskDir string) string { return filepath.Join(taskDir, "orchestrator.pid") }

// AssertNoRunningOrchestrator 单实例守卫：pid 文件属于存活进程 = 另一个编排器
// 在跑此任务——强行启动会踩掉 pid 文件，让 `flywheel stop` 杀错（已死的）
// 进程、孤儿化真正在跑的那个。陈旧 pid 文件（死 pid / 不可读）可安全覆写。
func AssertNoRunningOrchestrator(taskDir string) error {
	file := PidFile(taskDir)
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil // 不存在/不可读 = 陈旧，下方覆写
	}
	var parsed struct {
		PID *int `json:"pid"`
	}
	if jsonUnmarshal(raw, &parsed) != nil || parsed.PID == nil {
		return nil // 不可读 = 陈旧
	}
	pid := *parsed.PID
	if pid <= 0 {
		return nil
	}
	if processAlive(pid) != nil {
		return nil // 死进程 = 陈旧 pid 文件
	}
	return fmt.Errorf("another orchestrator is running this task (pid %d) — stop it first: flywheel stop <slug>", pid)
}

// WarnIfForeignDirty 仅限 IN-place 回退模式的 P1-1 caveat 守卫（worktree 不可用
// 或被禁用时）：scoped 单元 commit diff 的是自 base ref 以来的全部改动，
// 既有的未提交 WIP 会被卷进单元提交。警告但不阻塞。worktree 模式无需警告
// ——executor 根本看不见主 checkout。
func (c *Context) WarnIfForeignDirty() {
	foreign, ok := verifyForeignDirty(c.Cwd, c.taskDirPrefix())
	if ok && len(foreign) > 0 {
		sample := foreign
		if len(sample) > 3 {
			sample = sample[:3]
		}
		c.Log(fmt.Sprintf(
			"WARNING: %d dirty file(s) outside the task dir at run start (e.g. %s) — unit commits are scoped to base-ref diffs and WILL include these pre-existing changes",
			len(foreign), strings.Join(sample, ", ")))
	}
}

// unitCwd 本单元的工作区。陈旧引用（完成时已删除的 worktree）绝不可用——
// 对已完成单元的手工 --only 步骤回退主 checkout（合并后的代码在那里）。
func (c *Context) unitCwd(packet *contract.Packet) string {
	ps := c.State.Packets[packet.PacketID]
	if ps != nil && ps.Worktree != nil && ps.Worktree.Mode == "worktree" {
		if fileExists(ps.Worktree.Path) {
			return ps.Worktree.Path
		}
	}
	return c.Cwd
}

// ProjectRoot resolves the execution root before both configuration discovery
// and context creation. Non-Git rehearsal directories keep their original cwd.
func ProjectRoot(cwd string) string {
	cmd := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel")
	cmd.Env = childenv.EnvSlice(childenv.GateEnv(childenv.OSLookupEnv, nil))
	out, err := cmd.Output()
	if err != nil {
		return cwd // 非 Git 目录可检查/规划，最终交付仍要求持久 Git tree。
	}
	return strings.TrimSpace(string(out))
}

// absoluteDirectoryPath keeps getcwd failures injectable: a removed working
// directory does not make filepath.Abs fail consistently across operating systems.
var absoluteDirectoryPath = filepath.Abs

// canonicalDirectory gives workspace and task paths the same physical namespace.
// In particular Git resolves macOS /var to /private/var, while callers may retain
// the alias. Mixing them would turn an in-repository ledger prefix into ../ paths.
func canonicalDirectory(path string) (string, error) {
	abs, err := absoluteDirectoryPath(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// CreateContext 加载状态机需要的一切并返回运行上下文。
func CreateContext(opts Opts) (*Context, error) {
	taskDir, err := canonicalDirectory(opts.TaskDir)
	if err != nil {
		return nil, fmt.Errorf("resolve task directory: %w", err)
	}
	opts.TaskDir = taskDir
	spec, err := contract.LoadSpec(opts.TaskDir)
	if err != nil {
		return nil, err
	}
	packetList, err := contract.LoadPackets(opts.TaskDir, spec)
	if err != nil {
		return nil, err
	}
	packets := map[string]*contract.Packet{}
	for _, p := range packetList {
		packets[p.PacketID] = p
	}
	var st *state.State
	fresh := false
	if state.Exists(opts.TaskDir) {
		st, err = state.Load(opts.TaskDir)
		if err != nil {
			return nil, err
		}
	} else {
		// Init 的唯一错误路径是 EmptyPacketState → MilestoneOfPacket 的 id 派生
		// 失败；packet-id 已被 LoadPackets 的 schema 锚定 pattern 校验过——这里
		// 不可能失败（覆盖率审计 2026-08-23 删除防御分支）。
		st, _ = state.Init(spec.Slug(opts.TaskDir), spec, packetList, opts.Config)
		fresh = true
	}
	// planner 修订可能在 state 上次写入后往契约层加了 packet；每个已加载
	// packet 必须有状态条目。同上，唯一错误路径（id 派生）不可达。
	_ = state.EnsurePacketStates(st, packetList)
	cwd := opts.Cwd
	if cwd == "" {
		cwd = "."
	}
	cwd, err = canonicalDirectory(ProjectRoot(cwd))
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	logf := opts.Log
	if logf == nil {
		logf = func(string) {}
	}
	sleepf := opts.Sleep
	if sleepf == nil {
		if opts.Context == nil {
			sleepf = time.Sleep
		} else {
			sleepf = func(d time.Duration) {
				timer := time.NewTimer(d)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-opts.Context.Done():
				}
			}
		}
	}
	nowf := opts.Now
	if nowf == nil {
		nowf = time.Now
	}
	env := opts.Env
	if env == nil {
		env = childenv.OSLookupEnv
	}
	drv := opts.Drivers
	if drv == nil {
		drv = drivers.CreateDrivers(opts.Config)
	}
	ctx := &Context{
		TaskDir:     opts.TaskDir,
		Cwd:         cwd,
		Config:      opts.Config,
		Drivers:     drv,
		Spec:        spec,
		Packets:     packets,
		PacketList:  packetList,
		State:       st,
		Fresh:       fresh,
		Log:         logf,
		Sleep:       sleepf,
		Now:         nowf,
		EnvLookup:   env,
		metricsFile: filepath.Join(opts.TaskDir, "metrics.jsonl"),
		revision:    st.Revision,
		RunContext:  opts.Context,
	}
	if err := ctx.reconcileContractRevisions(); err != nil {
		return nil, err
	}
	return ctx, nil
}
