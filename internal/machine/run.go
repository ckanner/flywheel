package machine

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// L1_FAILED / REVIEW_RETURNED 是 machine 层的复合错误码（不在 errs.AllCodes
// 的 spawn 分类里），Node 版即裸字符串。
var (
	errL1Failed       = errs.ErrorCode("L1_FAILED")
	errReviewReturned = errs.ErrorCode("REVIEW_RETURNED")
)

type childenvLookup = childenv.LookupEnv

// ---- verify 包装（保存失败的 fatal 检查点与 Node 异常传播等价）----

// ---- 失败 / 升级 ----

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (c *Context) milestoneByID(id string) (contract.Milestone, bool) {
	for _, m := range c.Spec.Milestones {
		if m.ID == id {
			return m, true
		}
	}
	return contract.Milestone{}, false
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
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

// jsonNilable 空串映射为 JSON null（Node null 语义）。
func jsonNilable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func commitOutcomeMap(o verify.CommitOutcome) map[string]any {
	m := map[string]any{"at": o.At}
	if o.SHA != "" {
		m["sha"] = o.SHA
	}
	if o.Skipped != "" {
		m["skipped"] = o.Skipped
	}
	if o.Error != "" {
		m["error"] = o.Error
	}
	if o.Message != "" {
		m["message"] = o.Message
	}
	return m
}

// completeUnit 单元完成落盘。
//
// ErrMergeConflict 单元代码无法 ff-merge 进主 checkout——状态绝不说谎：
// 调用方必须把单元路由到 inbox/人工处理，而不是标 completed。
var ErrMergeConflict = errors.New("unit branch could not be merged into the main checkout")

// ErrCommitFailed 表示交付物或账本未成功持久化；绝不能据此标记完成或删除工作区。
var ErrCommitFailed = errors.New("unit changes could not be committed")

// ErrInterrupted 取消已传到所有执行步骤，清理/缺陷恢复完成；CLI 对应退出143。
var ErrInterrupted = errors.New("orchestrator interrupted")

// ---- 缺陷注入抽检（§4.3）----

func derefStringPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// jsonToMap 结构体 → 通用 map（键名经 json tag 与 Node 版逐字对齐）。
func jsonToMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// ---- packet 处理（attempts 循环）----

type processOpts struct {
	PauseBefore string // "review"：L1 绿后冻结，等下一次 run 从 review 继续
}

func orTransport(code errs.ErrorCode) errs.ErrorCode {
	if code == "" {
		return errs.TRANSPORT
	}
	return code
}

// ---- 入口 ----

func (c *Context) nextActionablePacket(milestoneID string) *contract.Packet {
	for _, p := range c.PacketList {
		if m, err := contract.MilestoneOfPacket(p.PacketID); err != nil || m != milestoneID {
			continue
		}
		if c.State.Packets[p.PacketID].Status != "completed" {
			return p
		}
	}
	return nil
}

func (c *Context) currentMilestone() *contract.Milestone {
	for i := range c.Spec.Milestones {
		ms, ok := c.State.Milestones[c.Spec.Milestones[i].ID]
		if !ok || ms.Status != "closed" {
			return &c.Spec.Milestones[i]
		}
	}
	return nil
}

var writePIDData = os.WriteFile
var createPIDTemp = os.CreateTemp

func (c *Context) writePid() error {
	runtimeDir := filepath.Join(c.TaskDir, ".control")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return err
	}
	c.controlToken = rand.Text()
	payload, _ := json.Marshal(RunControl{PID: os.Getpid(), StartedAt: c.nowISO(), ControlToken: c.controlToken})
	tmp, err := createPIDTemp(runtimeDir, "orchestrator-pid-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(name)
	if err := writePIDData(name, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(name, PidFile(c.TaskDir))
}

func clearPid(taskDir string) {
	_ = os.Remove(PidFile(taskDir)) // best effort
	_ = os.Remove(StopRequestFile(taskDir))
}

// registerSigterm P1 修复：注册 SIGTERM（Node 定义了 onSigterm 却从未
// process.once 注册，kill switch 失效）。收到信号 → 杀光在飞子进程组并等到
// quiescence → 以退出码 143 结束——绝不让孤儿进程继续写工作区。
func registerSigterm(log func(string), cancelRun ...context.CancelFunc) chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	go func() {
		if _, ok := <-ch; ok {
			log("SIGTERM received — killing in-flight children, then exiting")
			for _, cancel := range cancelRun {
				cancel()
			}
			watchdog.KillActiveChildren(time.Duration(watchdog.DefaultKillGraceMs) * time.Millisecond)
			if len(cancelRun) > 0 {
				return // 让运行栈完成缺陷恢复等清理后正常返回，再由 CLI 退出143。
			}
			os.Exit(143)
		}
	}()
	return ch
}

func unregisterSigterm(ch chan os.Signal) {
	signal.Stop(ch)
	close(ch)
}

// TaskSummary runTask/runSingleStep 的返回摘要；state.json 才是事实信源。
// JSON 键与 Node 版逐字一致（CLI 直接输出）。
type TaskSummary struct {
	Slug             string   `json:"slug"`
	Status           string   `json:"status"`
	CurrentMilestone *string  `json:"current-milestone"`
	PacketsCompleted string   `json:"packets-completed"`
	Revision         int64    `json:"revision"`
	LastProgressAt   *string  `json:"last-progress-at"`
	Tokens           int64    `json:"tokens"`
	CostUSD          float64  `json:"cost-usd"`
	Step             *StepSum `json:"step,omitempty"`
}

// StepSum --only 单步模式的步骤结果（#9：失败步骤以非零码退出）。
type StepSum struct {
	Name    string           `json:"name"`
	OK      bool             `json:"ok"`
	Verdict *prompts.Verdict `json:"verdict,omitempty"`
	Detail  *string          `json:"detail,omitempty"`
}

func strPtrAlways(s string) *string { return &s }

func (c *Context) summary() TaskSummary {
	done := 0
	total := len(c.PacketList)
	for _, p := range c.PacketList {
		if c.State.Packets[p.PacketID].Status == "completed" {
			done++
		}
	}
	return TaskSummary{
		Slug:             c.State.Slug,
		Status:           c.State.Status,
		CurrentMilestone: c.State.CurrentMilestone,
		PacketsCompleted: fmt.Sprintf("%d/%d", done, total),
		Revision:         c.State.Revision,
		LastProgressAt:   c.State.LastProgressAt,
		Tokens:           c.State.Budget.Totals.Tokens,
		CostUSD:          c.State.Budget.Totals.CostUSD,
	}
}

// RunOpts RunTask / RunSingleStep 共享的装配入参。
type RunOpts struct {
	TaskDir string
	Cwd     string
	Config  *config.Config
	Drivers *drivers.Drivers
	Log     func(string)
	Sleep   func(time.Duration)
	Now     func() time.Time
	Env     childenvLookup
	Context context.Context
}

// RunTaskOpts RunTask 入参。
type RunTaskOpts struct {
	RunOpts
	PauseBefore string
}

// RunTask 全自动运行：里程碑 → packet → attempts（execute → L1 → review）
// → 里程碑闸门。返回摘要；state.json 携带真相。
func RunTask(opts RunTaskOpts) (TaskSummary, error) {
	release, err := AcquireRunLease(opts.TaskDir)
	if err != nil {
		return TaskSummary{}, err
	}
	defer release()
	clearPid(opts.TaskDir) // the lease proves any previous PID/request is stale
	parent := opts.Context
	if parent == nil {
		parent = context.Background()
	}
	runContext, cancel := context.WithCancel(parent)
	defer cancel()
	ctx, err := CreateContext(Opts{
		TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config,
		Drivers: opts.Drivers, Log: opts.Log, Sleep: opts.Sleep, Now: opts.Now, Env: opts.Env,
		Context: runContext,
	})
	if err != nil {
		return TaskSummary{}, err
	}
	ctx.pauseBefore = opts.PauseBefore
	if err := ctx.writePid(); err != nil {
		return ctx.summary(), err
	}
	sigCh := registerSigterm(ctx.Log, cancel)
	defer unregisterSigterm(sigCh)
	defer clearPid(ctx.TaskDir)
	stopControl := watchStopRequests(runContext, ctx.TaskDir, ctx.controlToken, cancel)
	defer stopControl()

	summary, err := ctx.run()
	if runContext.Err() != nil {
		return summary, ErrInterrupted
	}
	if err != nil {
		return summary, err
	}
	return summary, ctx.err()
}

func (c *Context) reloadPackets() error {
	spec, err := contract.LoadSpec(c.TaskDir)
	if err != nil {
		return err
	}
	fresh, err := contract.LoadPackets(c.TaskDir, spec)
	if err != nil {
		return err
	}
	c.Spec = spec
	c.PacketList = fresh
	c.Packets = map[string]*contract.Packet{}
	for _, p := range fresh {
		c.Packets[p.PacketID] = p
	}
	// LoadPackets validated the packet IDs; EnsurePacketStates cannot fail here.
	_ = state.EnsurePacketStates(c.State, fresh)
	return c.reconcileContractRevisions()
}

func (c *Context) run() (TaskSummary, error) {
	// 崩溃恢复：绝不静默重置在飞单元（§5.2）。
	interrupted := c.markInterruptedUnits()
	if len(interrupted) > 0 {
		c.Log(fmt.Sprintf("recovered interrupted units: %s", strings.Join(interrupted, ", ")))
	}
	c.State.Status = "running" // paused/stopped 任务 resume 时翻回 running（#6）
	c.save()

	visits := map[string]int{}
	for {
		next := c.currentMilestone()
		if next == nil {
			break
		}
		milestone := *next
		visits[milestone.ID]++
		if visits[milestone.ID] > maxMilestoneVisits {
			c.State.Status = "escalated"
			entry := map[string]any{"at": c.nowISO(), "milestone": milestone.ID,
				"reason": fmt.Sprintf("milestone %s repeatedly reopened by contract revisions; stopped after %d visits in one run", milestone.ID, maxMilestoneVisits)}
			c.State.Inbox = append(c.State.Inbox, entry)
			c.appendInbox(entry)
			c.save()
			return c.summary(), c.err()
		}
		ms := c.State.Milestones[milestone.ID]

		c.State.CurrentMilestone = strPtrAlways(milestone.ID)
		ms.Status = "in-progress"
		c.State.Milestones[milestone.ID] = ms
		budgetStartTokens := c.State.Budget.Totals.Tokens
		budgetStartCost := c.State.Budget.Totals.CostUSD
		c.save()

		plan, err := c.planUntilValid(milestone)
		if err != nil {
			return c.summary(), err
		}
		if plan.Stopped {
			return c.summary(), c.err()
		}
		if !plan.OK {
			c.State.Status = "escalated"
			entry := map[string]any{
				"at":        c.nowISO(),
				"packet-id": nil,
				"milestone": milestone.ID,
				"reason":    fmt.Sprintf("plan step failed on every channel (last: %s). Human needed.", plan.ErrorCode),
			}
			c.State.Inbox = append(c.State.Inbox, entry)
			c.appendInbox(entry)
			c.save()
			if ferr := c.err(); ferr != nil {
				return c.summary(), ferr
			}
			return c.summary(), nil
		}
		// P1：planner 可能在这步常规运行中修订过契约文件——planUntilValid 内
		// 已重载并做 acceptance 指纹留痕（P0-2/A1 对称性）。
		// v0.16：仅契约提交（脏账本不卷入——这里做账本提交会移动 HEAD，
		// 破坏暂停单元 worktree 的 ff-merge）。无变更 = 无提交 = HEAD 不动。
		if err := c.persistPlanContracts(fmt.Sprintf("docs(%s): plan update", c.State.Slug)); err != nil {
			return c.summary(), err
		}
		current, present := c.milestoneByID(milestone.ID)
		if !present {
			return c.summary(), fmt.Errorf("active milestone %s was removed by Plan; contracts preserved for human review", milestone.ID)
		}
		milestone = current
		c.save()

		for {
			if ferr := c.err(); ferr != nil {
				return c.summary(), ferr
			}
			packet := c.nextActionablePacket(milestone.ID)
			if packet == nil {
				break
			}
			outcome, err := c.processPacket(packet, processOpts{PauseBefore: c.pauseBefore})
			if err != nil {
				return c.summary(), err
			}
			if outcome == "stop" || outcome == "pause" {
				return c.summary(), nil
			}
		}

		gate := c.milestoneGateStep(milestone)
		if !gate.Pass {
			c.State.Status = "escalated"
			entry := map[string]any{
				"at":        c.nowISO(),
				"packet-id": nil,
				"milestone": milestone.ID,
				"reason":    fmt.Sprintf("milestone %s gate (L2/L3) failed", milestone.ID),
			}
			c.State.Inbox = append(c.State.Inbox, entry)
			c.appendInbox(entry)
			c.save()
			if ferr := c.err(); ferr != nil {
				return c.summary(), ferr
			}
			return c.summary(), nil
		}

		// Judge（§7 里程碑终验）：关闭前的全新上下文终验。
		judge, err := c.judgeStep(milestone)
		if err != nil {
			return c.summary(), err
		}
		if judge.Stopped {
			return c.summary(), c.err()
		}
		if !judge.Pass {
			c.State.Status = "escalated"
			verdictText := string(judge.ErrorCode)
			reasons := ""
			if judge.Verdict != nil {
				verdictText = judge.Verdict.Verdict
				reasons = strings.Join(judge.Verdict.Reasons, "; ")
			}
			if reasons == "" {
				reasons = "see metrics"
			}
			entry := map[string]any{
				"at":        c.nowISO(),
				"packet-id": nil,
				"milestone": milestone.ID,
				"reason":    fmt.Sprintf("milestone %s judge verdict: %s — %s", milestone.ID, verdictText, reasons),
			}
			c.State.Inbox = append(c.State.Inbox, entry)
			c.appendInbox(entry)
			c.save()
			if ferr := c.err(); ferr != nil {
				return c.summary(), ferr
			}
			return c.summary(), nil
		}

		ms = c.State.Milestones[milestone.ID]
		ms.Status = "closed"
		closedAt := c.nowISO()
		ms.ClosedAt = &closedAt
		c.State.Milestones[milestone.ID] = ms
		c.touchProgress()
		judgeScore := "n/a"
		if judge.Verdict != nil && judge.Verdict.Score != nil {
			judgeScore = strconv.FormatFloat(*judge.Verdict.Score, 'g', -1, 64)
		}
		c.appendRetro(fmt.Sprintf("- %s milestone %s: closed (gate pass, judge %s)", c.nowISO(), milestone.ID, judgeScore))
		c.appendRetro(c.milestoneSummaryLine(milestone, budgetStartTokens, budgetStartCost))
		c.save()
	}

	if !c.contractsComplete() {
		return c.summary(), fmt.Errorf("current contracts contain unfinished units; refusing completed status")
	}
	if _, err := verify.PersistedHead(c.Cwd, c.ledgerExcludes()); err != nil {
		c.State.Status = "escalated"
		c.save()
		return c.summary(), fmt.Errorf("%w: final task tree is not persisted: %v", ErrCommitFailed, err)
	}
	c.State.Status = "completed"
	c.State.CurrentMilestone = nil
	c.touchProgress()
	c.save()
	return c.summary(), c.err()
}

// milestoneSummaryLine retro.md 的里程碑摘要（§5.3 字段）：只记可核对事实。
func (c *Context) milestoneSummaryLine(milestone contract.Milestone, startTokens int64, startCost float64) string {
	var states []*state.PacketState
	for _, p := range c.PacketList {
		if m, err := contract.MilestoneOfPacket(p.PacketID); err == nil && m == milestone.ID {
			if ps, ok := c.State.Packets[p.PacketID]; ok {
				states = append(states, ps)
			}
		}
	}
	attempts, reviewFailed, reopens, scopeChanges := 0, 0, 0, 0
	for _, ps := range states {
		attempts += ps.Attempts
		for _, rh := range ps.RetryHistory {
			entry, ok := rh.(map[string]any)
			if !ok {
				continue
			}
			if ec, _ := entry["error-code"].(string); ec == "REVIEW_RETURNED" {
				reviewFailed++
			}
			if pol, _ := entry["policy"].(string); pol == "return-to-planner" {
				scopeChanges++
			}
		}
		reopens += ps.PlanRevisions
	}
	tokens := c.State.Budget.Totals.Tokens - startTokens
	cost := mathRound((c.State.Budget.Totals.CostUSD-startCost)*1e8) / 1e8
	// P2：人工介入只数本里程碑的条目（entry.milestone 或 entry.packet-id 解析
	// 到本里程碑）——绝不是全任务 inbox，那会随每次后续升级膨胀。
	interventions := 0
	for _, e := range c.State.Inbox {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if m, _ := entry["milestone"].(string); m == milestone.ID {
			interventions++
			continue
		}
		if pid, _ := entry["packet-id"].(string); pid != "" {
			if m, err := contract.MilestoneOfPacket(pid); err == nil && m == milestone.ID {
				interventions++
			}
		}
	}
	return strings.Join([]string{
		fmt.Sprintf("  summary: rounds=%d, defects(review-failed)=%d, reopens=%d, "+
			"scope-changes=%d (return-to-planner revisions), budget=+%d tokens / $%s, "+
			"human-interventions=%d",
			attempts, reviewFailed, reopens, scopeChanges, tokens,
			strconv.FormatFloat(cost, 'f', -1, 64), interventions),
		"  improvements (<=3, placeholder — filled by Plan/human): -",
	}, "\n")
}

// RunSingleStepOpts 手工单步模式入参。
type RunSingleStepOpts struct {
	RunOpts
	Only     string // plan | execute | review | verify
	PacketID string
}

// RunSingleStep 手工单步模式（§12）：--only 恰好执行一次状态转移。
// 步骤结果落在摘要（step 字段），失败步骤由 CLI 以非零码退出（#9）。
func RunSingleStep(opts RunSingleStepOpts) (TaskSummary, error) {
	release, err := AcquireRunLease(opts.TaskDir)
	if err != nil {
		return TaskSummary{}, err
	}
	defer release()
	clearPid(opts.TaskDir)
	parent := opts.Context
	if parent == nil {
		parent = context.Background()
	}
	runContext, cancel := context.WithCancel(parent)
	defer cancel()
	ctx, err := CreateContext(Opts{
		TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config,
		Drivers: opts.Drivers, Log: opts.Log, Sleep: opts.Sleep, Now: opts.Now, Env: opts.Env,
		Context: runContext,
	})
	if err != nil {
		return TaskSummary{}, err
	}
	if err := ctx.writePid(); err != nil {
		return ctx.summary(), err
	}
	sigCh := registerSigterm(ctx.Log, cancel)
	defer unregisterSigterm(sigCh)
	defer clearPid(ctx.TaskDir)
	stopControl := watchStopRequests(runContext, ctx.TaskDir, ctx.controlToken, cancel)
	defer stopControl()

	summary, err := ctx.runSingle(opts.Only, opts.PacketID)
	if runContext.Err() != nil {
		return summary, ErrInterrupted
	}
	if err != nil {
		return summary, err
	}
	return summary, ctx.err()
}

func (c *Context) runSingle(only, packetID string) (TaskSummary, error) {
	interrupted := c.markInterruptedUnits()
	if len(interrupted) > 0 {
		c.Log(fmt.Sprintf("recovered interrupted units: %s", strings.Join(interrupted, ", ")))
	}
	c.save()

	pickPacket := func() (*contract.Packet, error) {
		if packetID != "" {
			p, ok := c.Packets[packetID]
			if !ok {
				return nil, fmt.Errorf("unknown packet/unit: %s", packetID)
			}
			return p, nil
		}
		m := c.currentMilestone()
		if m == nil {
			return nil, fmt.Errorf("no open milestone")
		}
		p := c.nextActionablePacket(m.ID)
		if p == nil {
			return nil, fmt.Errorf("no actionable packet in milestone %s", m.ID)
		}
		return p, nil
	}

	var step *StepSum
	switch only {
	case "plan":
		m := c.currentMilestone()
		if m == nil {
			return c.summary(), fmt.Errorf("no open milestone")
		}
		// v0.24：--only plan = plan 子环闭环（到合法契约集或界尽 inbox）。
		r, perr := c.planUntilValid(*m)
		if perr != nil {
			return c.summary(), perr
		}
		if r.OK {
			if err := c.persistPlanContracts(fmt.Sprintf("docs(%s): manual plan update", c.State.Slug)); err != nil {
				return c.summary(), err
			}
			c.save()
		}
		step = &StepSum{Name: "plan", OK: r.OK, Detail: optStr(string(r.ErrorCode))}
	case "execute":
		packet, perr := pickPacket()
		if perr != nil {
			return c.summary(), perr
		}
		// v0.24：--only execute = execute 子环闭环——转到 L1 全绿或出口。
		// 人驱动的单环模式不做自动升级（升级是跨环裁决，人就在驾驶座）：
		// Bounded/Spun 如实报告，人自行决定 --only plan 修订或重跑。
		r, err := c.runExecuteLoop(packet)
		if err != nil {
			return c.summary(), err
		}
		switch {
		case r.Green:
			step = &StepSum{Name: "execute", OK: true, Detail: optStr("L1 green (execute loop closed)")}
		case r.Stopped:
			step = &StepSum{Name: "execute", OK: false, Detail: optStr("stopped (budget/credentials)")}
		case r.Bounded:
			ps := c.State.Packets[packet.PacketID]
			step = &StepSum{Name: "execute", OK: false,
				Detail: optStr(fmt.Sprintf("bounded stop (%d/%d attempts) — rerun, or --only plan to revise", ps.FailedAttempts, packet.Escalation.MaxRetries))}
		case r.Spun:
			step = &StepSum{Name: "execute", OK: false, Detail: optStr("spin detected (identical failure signature) — human judgement needed")}
		}
	case "review":
		packet, perr := pickPacket()
		if perr != nil {
			return c.summary(), perr
		}
		ps := c.State.Packets[packet.PacketID]
		// 铁律 4：确定性验收绿了评审才有发言权。
		if !l1PassTrue(ps) {
			return c.summary(), fmt.Errorf("unit %s has no PASSING L1 evidence — run execute/verify first", packet.PacketID)
		}
		r, err := c.reviewStep(packet, nil)
		if err != nil {
			return c.summary(), err
		}
		step = &StepSum{Name: "review", OK: r.Pass, Verdict: r.Verdict, Detail: optStr(string(r.ErrorCode))}
	case "verify":
		if packetID != "" {
			packet, perr := pickPacket()
			if perr != nil {
				return c.summary(), perr
			}
			r := c.l1Step(packet)
			step = &StepSum{Name: "verify", OK: r.Pass}
		} else {
			ok := true
			for _, p := range c.PacketList {
				if c.State.Packets[p.PacketID].Status != "completed" {
					r := c.l1Step(p)
					if !r.Pass {
						ok = false
					}
				}
			}
			if m := c.currentMilestone(); m != nil {
				g := c.milestoneGateStep(*m)
				if !g.Pass {
					ok = false
				}
			}
			step = &StepSum{Name: "verify", OK: ok}
		}
	default:
		return c.summary(), fmt.Errorf("unknown --only step: %s", only)
	}
	s := c.summary()
	s.Step = step
	return s, nil
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
