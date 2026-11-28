// steps_verdict.go — LLM 裁决角色（Review / Judge / 双面板，DESIGN §4.3）：
// 传输重试 + 渠道降级 + 末行 JSON 裁决解析的共用循环与两个特化步骤。
package machine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
)

// agentVerdictStep LLM 裁决角色的共用循环：传输重试 + 渠道降级 +
// 末行 JSON 裁决解析。事件流内的 provider 错误（pi 遇 401/403 exit 0）
// 优先于解析。ps 为空时重试记录只落 metrics。
func (c *Context) agentVerdictStep(a verdictArgs) (verdictOutcome, error) {
	channel := a.binding.Channel
	currentModel, currentHarness := a.binding.Model, a.binding.Harness
	currentProvider := fallback.ResolveProvider(c.Config, currentHarness, a.binding.Provider, channel)
	visited := map[string]bool{}
	maxTransport := c.Config.Retry.MaxTransportRetries
	lastCode := errs.TRANSPORT
	sessionRole := firstNonEmpty(a.sessionRole, a.role)
	sessionScope := firstNonEmpty(a.sessionScope, a.packetID)

	call := func() (drivers.Result, error) {
		args := drivers.CallArgs{
			TaskDir:        c.TaskDir,
			Cwd:            firstNonEmpty(a.cwd, c.Cwd),
			PermissionMode: c.Config.PermissionMode(a.role, currentHarness),
			Prompt:         a.prompt,
			Model:          currentModel,
			Binding: config.LoopBinding{
				NativeProfile: a.binding.NativeProfile,
				Harness:       currentHarness, Provider: currentProvider, Model: currentModel,
				Channel: channel, Thinking: a.binding.Thinking,
			},
			APIKey:   c.channelApiKey(channel),
			APIKeys:  c.apiKeysFor(channel),
			Watchdog: c.wdogFor(a.role),
		}
		if a.role == "review" && a.finalSpec == nil {
			args.ContextTemplateVersion = reviewContextVersion
		}
		switch a.role {
		case "review":
			return c.invokeScoped(sessionRole, sessionScope, args, c.Drivers.Review.Review)
		default: // judge（当前仅此两角色走裁决循环）
			return c.invokeScoped(sessionRole, sessionScope, args, c.Drivers.Judge.Judge)
		}
	}

	record := func(scope, code, policy string, plannedDelayMS ...int64) {
		if a.ps != nil {
			c.recordRetry(a.ps, scope, code, policy, plannedDelayMS...)
		}
	}

	for t := 0; t <= maxTransport; t++ {
		if ferr := c.err(); ferr != nil {
			return verdictOutcome{}, ferr
		}
		if !c.sessionReplay(sessionRole, sessionScope) && c.budgetGate() == "stop" {
			return verdictOutcome{Stopped: true}, nil
		}
		if missingKey := c.missingKeyForRealDriver(a.role, channel); !c.sessionReplay(sessionRole, sessionScope) && missingKey != "" {
			record("transport", string(errs.AUTH),
				fmt.Sprintf("fail-loud: channel %s requires env %s (not set)", channel, missingKey))
			if a.ps != nil {
				c.save()
			}
			c.Log(fmt.Sprintf("%s: fail-loud — channel %s requires env %s (not set)", a.role, channel, missingKey))
			return verdictOutcome{ErrorCode: errs.AUTH}, nil
		}
		visited[channel] = true
		res, sessionErr := call()
		if sessionErr != nil {
			return verdictOutcome{}, sessionErr
		}

		streamMsg := ""
		if res.ErrorMessage != nil {
			streamMsg = *res.ErrorMessage
		}
		var streamCode errs.ErrorCode
		if streamMsg != "" || res.ErrorCode != "" {
			streamCode = errs.ClassifyError(errs.SpawnResult{ExitCode: 1, Stderr: streamMsg, ErrorCode: res.ErrorCode})
		}
		var verdict *prompts.Verdict
		// 出口契约同款三重门（§6）：进程终态正常 + 无信号 + stderr 无错误模式
		// 才允许解析裁决——"超时/被 kill 被 trap 成 exit 0"不得读成绿灯。
		// 裁决文本取 Text ?? Stdout（Node 语义：harness 可能把裁决写 stdout）。
		text := res.Text
		if text == "" {
			text = res.Stdout
		}
		if streamCode == "" && res.Settled && res.ExitCode != nil && *res.ExitCode == 0 && !res.TimedOut && !res.Cancelled && !res.Truncated && res.CleanupError == "" &&
			res.Signal == "" && errs.ClassifyError(errs.SpawnResult{ExitCode: 0, Stderr: res.Stderr}) == "" {
			if a.finalSpec != nil {
				verdict, _ = prompts.ParseFinalReviewVerdict(text, a.finalSpec)
			} else {
				verdict, _ = prompts.ParseReviewVerdict(text)
			}
		}
		code := ""
		if verdict == nil {
			if res.TimedOut || res.Cancelled || res.CleanupError != "" || res.SpawnError != "" ||
				(res.ErrorCode != "" && (res.ExitCode == nil || *res.ExitCode != 1 || res.Truncated || res.Signal != "")) {
				code = string(errs.ClassifyError(toSpawnResult(res)))
			} else if streamCode != "" {
				code = string(streamCode)
			} else if cls := errs.ClassifyError(toSpawnResult(res)); cls != "" {
				code = string(cls)
			} else {
				code = string(errs.EMPTY_RESPONSE)
			}
		}
		if c.sessionGuardViolated(sessionRole, sessionScope) {
			code, verdict = string(errs.INVALID_REQUEST), nil
		}
		if code != "" {
			lastCode = errs.ErrorCode(code)
		}
		c.recordSpawnMetrics(spawnMetricsArgs{
			role: a.role, packetID: a.packetID, model: currentModel,
			channel: channel, prompt: a.prompt, result: &res,
			errorCode: errs.ErrorCode(code), attempt: a.attemptNo,
		})
		if err := c.stopForUnconfirmedCleanup(a.role, a.packetID, res); err != nil {
			return verdictOutcome{}, err
		}

		if verdict != nil {
			return verdictOutcome{OK: true, Verdict: verdict}, nil
		}
		c.consumeSession(sessionRole, sessionScope)

		who := a.role
		if a.packetID != "" {
			who = fmt.Sprintf("%s %s", a.role, a.packetID)
		}
		c.Log(fmt.Sprintf("%s transport %d: failed/unparseable (%s)", who, t, code))

		if errs.ShouldFallback(errs.ErrorCode(code)) || fallback.AuthFallbackAllowed(c.Config, channel, errs.ErrorCode(code)) {
			next := fallback.NextChannel(c.Config, channel)
			var mapped *fallback.ChannelModel
			if next != "" {
				mapped = fallback.ModelForChannel(c.Config, next, a.role, "")
			}
			if next != "" && !visited[next] && mapped != nil {
				record("fallback", code, fmt.Sprintf("channel %s -> %s", channel, next))
				channel = next
				currentModel = mapped.Model
				if mapped.Harness != "" {
					currentHarness = mapped.Harness
				}
				currentProvider = fallback.ResolveProvider(c.Config, currentHarness, "", channel)
				t--
				if a.ps != nil {
					c.save()
				}
				continue
			}
			if next != "" && mapped == nil {
				c.Log(fmt.Sprintf("%s fallback chain ends: no model mapping for %s on %s (P1-12①)", a.role, a.role, next))
			}
		}
		if !errs.IsRetryable(errs.ErrorCode(code)) {
			record("transport", code, "no-retry")
			if a.ps != nil {
				c.save()
			}
			return verdictOutcome{ErrorCode: errs.ErrorCode(code)}, nil
		}
		delay := errs.ComputeBackoff(t, errs.BackoffOptions{
			BaseDelayMs:  c.Config.Retry.BaseDelayMs,
			MaxDelayMs:   c.Config.Retry.MaxDelayMs,
			Jitter:       c.Config.Retry.Jitter,
			RetryAfterMs: float64(errs.ExtractRetryAfterMs(res.Stderr + "\n" + res.Stdout)),
		})
		record("transport", code, "backoff", delay)
		if a.ps != nil {
			c.save()
		}
		c.waitRetry(time.Duration(delay)*time.Millisecond, a.packetID)
	}
	return verdictOutcome{ErrorCode: lastCode}, nil
}

// reviewStep 只在 L1 绿了之后运行（铁律 4）。评审者看到 packet + diff + L1
// 结果——绝看不到 executor transcript。
var reviewChangedFiles = verify.GetChangedFiles
var reviewDiffText = verify.GetDiffText

func (c *Context) reviewStep(packet *contract.Packet, defectInjection any) (out reviewOutcome, reviewErr error) {
	defer c.measurePhase("review", packet.PacketID)()
	defer func() {
		if err := c.closeReviewPrewarm("discarded: review preparation ended"); err != nil {
			reviewErr = errors.Join(reviewErr, err)
		}
	}()
	if err := c.err(); err != nil {
		return reviewOutcome{}, err
	}
	id := packet.PacketID
	ps := c.State.Packets[id]
	if !c.currentL1Evidence(packet) {
		return reviewOutcome{ErrorCode: errs.INVALID_REQUEST}, nil
	}
	if err := c.beginReviewPrewarm(packet); err != nil {
		return reviewOutcome{}, err
	}
	final := c.quick()
	scope := id
	if final {
		scope = "final:" + id
	}
	replay := c.sessionReplay("review", scope)
	if replay {
		defectInjection = ps.Evidence.DefectInjection
	}
	if final && !replay {
		stopped, err := c.quickReviewInjection(packet)
		if stopped {
			return reviewOutcome{Stopped: true}, err
		}
		defectInjection = ps.Evidence.DefectInjection
	}
	if final && !replay && !c.quickGateStep(packet) {
		return reviewOutcome{ErrorCode: errL1Failed}, c.err()
	}
	if final && replay && !c.currentFinalGate(packet) {
		return reviewOutcome{}, c.sessionBlock(sessionKey("review", scope), "recorded final Review no longer has its original passing gate")
	}
	if err := c.err(); err != nil {
		return reviewOutcome{}, err
	}
	identity := evidenceIdentity(ps.Evidence.L1)

	uCwd := c.unitCwd(packet)
	changed, changedOK := reviewChangedFiles(uCwd, c.baseRefOf(ps))
	if final && !changedOK {
		return reviewOutcome{}, fmt.Errorf("final review requires an authoritative changed-file inventory")
	}
	executorFiles, _ := c.partitionChangedFiles(changed)
	var diffSummary string
	if executorFiles != nil {
		lines := make([]string, len(executorFiles))
		for i, f := range executorFiles {
			lines[i] = "- " + f
		}
		diffSummary = fmt.Sprintf("changed files (git diff %s, ledger excluded):\n%s",
			orHEAD(c.baseRefOf(ps)), strings.Join(lines, "\n"))
	} else {
		self := []any{}
		if ex, ok := ps.Evidence.Executor.(map[string]any); ok {
			if cf, ok := ex["changed-files"].([]any); ok {
				self = cf
			}
		}
		lines := make([]string, len(self))
		for i, f := range self {
			lines[i] = "- " + fmt.Sprintf("%v", f)
		}
		diffSummary = "changed files (executor-reported):\n" + strings.Join(lines, "\n")
	}
	// §4.3 向评审者承诺的是真实 git diff，不是文件清单（#11）。
	diffText, diffOK := reviewDiffText(uCwd, c.baseRefOf(ps), c.ledgerExcludes())
	if final && !diffOK {
		return reviewOutcome{}, fmt.Errorf("final review requires an authoritative diff")
	}
	priorFindings := priorOpenReviewFindings(latestPriorReview(ps))
	promptArgs := prompts.ReviewArgs{
		Packet:          packet,
		TaskDir:         c.TaskDir,
		L1Evidence:      ps.Evidence.L1,
		DiffSummary:     diffSummary,
		DiffText:        diffText,
		DefectInjection: defectInjection,
		PriorFindings:   priorFindings,
		HumanGuidance:   ps.HumanGuidance,
	}
	if final {
		promptArgs.FinalSpec, promptArgs.FinalGate, promptArgs.Workflow = c.Spec, ps.Evidence.FinalGate, c.State.Workflow
	}
	prompt := prompts.BuildReviewPrompt(promptArgs)

	binding, err := fallback.ResolveBinding(c.Config, "review", "")
	if err != nil {
		return reviewOutcome{}, err
	}
	attemptNo := ps.Attempts
	r, ferr := c.agentVerdictStep(verdictArgs{
		role: "review", binding: binding, prompt: prompt, cwd: uCwd,
		packetID: id, attemptNo: &attemptNo, ps: ps,
		finalSpec: promptArgs.FinalSpec, sessionScope: scope,
	})
	if ferr != nil {
		return reviewOutcome{}, ferr
	}
	if r.Stopped {
		return reviewOutcome{Stopped: true}, nil
	}
	if !r.OK {
		return reviewOutcome{ErrorCode: r.ErrorCode}, nil
	}
	if !c.currentL1Evidence(packet) || (final && !c.currentFinalGate(packet)) {
		c.retireSession("review", scope, "reviewer changed the verified candidate or evidence")
		// 即使后端违反只读约束或用户同时编辑，旧裁决也不能放行新代码。
		ps.Evidence.Review = map[string]any{
			"verdict": "return", "reasons": []string{"candidate or contract changed during review; rerun verification"},
			"at": c.nowISO(), "identity": identity,
		}
		c.save()
		return reviewOutcome{ErrorCode: errs.INVALID_REQUEST}, nil
	}

	vb, _ := json.Marshal(r.Verdict)
	m := map[string]any{}
	_ = json.Unmarshal(vb, &m)
	m["at"] = c.nowISO()
	m["identity"] = identity
	if final {
		m["scope"] = "task-final"
	}
	ps.Evidence.Review = m
	c.consumeSession("review", scope)
	at := c.nowISO()
	ps.UpdatedAt = &at
	if r.Verdict.Verdict == "pass" {
		c.touchProgress()
	}
	c.save()
	currOpen := countOpenReviewFindings(r.Verdict.Findings)
	resolvedCount := countResolvedReviewFindings(r.Verdict.Findings)
	converging := (len(priorFindings) > 0 && currOpen > 0 && currOpen < len(priorFindings)) ||
		(len(priorFindings) == 0 && resolvedCount > 0 && currOpen > 0)
	return reviewOutcome{Pass: r.Verdict.Verdict == "pass", Verdict: r.Verdict, Converging: converging, OpenFindings: currOpen}, nil
}

func latestPriorReview(ps *state.PacketState) any {
	if ps.Evidence.Review != nil {
		return ps.Evidence.Review
	}
	for i := len(ps.EvidenceHistory) - 1; i >= 0; i-- {
		entry, ok := ps.EvidenceHistory[i].(map[string]any)
		if !ok {
			continue
		}
		switch ev := entry["evidence"].(type) {
		case state.Evidence:
			if ev.Review != nil {
				return ev.Review
			}
		case map[string]any:
			if ev["review"] != nil {
				return ev["review"]
			}
		}
	}
	return nil
}

func priorOpenReviewFindings(evidence any) []prompts.ReviewFinding {
	m, ok := evidence.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := m["findings"]
	if !ok || raw == nil {
		return nil
	}
	b, _ := json.Marshal(raw)
	var all []prompts.ReviewFinding
	if json.Unmarshal(b, &all) != nil {
		return nil
	}
	var open []prompts.ReviewFinding
	for _, f := range all {
		if f.Status != "resolved" && f.Severity != "minor" {
			open = append(open, f)
		}
	}
	return open
}

func countOpenReviewFindings(findings []prompts.ReviewFinding) int {
	n := 0
	for _, f := range findings {
		if f.Status != "resolved" && f.Severity != "minor" {
			n++
		}
	}
	return n
}

func countResolvedReviewFindings(findings []prompts.ReviewFinding) int {
	n := 0
	for _, f := range findings {
		if f.Status == "resolved" {
			n++
		}
	}
	return n
}

// loopToBinding config.LoopBinding → fallback.Binding（judge-panel 复用
// agentVerdictStep 的传输重试/降级骨架）。
func loopToBinding(b config.LoopBinding) fallback.Binding {
	return fallback.Binding{NativeProfile: b.NativeProfile, Harness: b.Harness, Provider: b.Provider, Model: b.Model, Channel: b.Channel, Thinking: b.Thinking}
}

// judgeStep 在 L2/L3 闸门通过后、里程碑关闭前，由独立裁决角色对照 spec 的需求
// 矩阵复核整个里程碑。失败 → 升级 inbox。
// 可选 judge-panel 增加第二意见，默认不启用。语义：一致按共识；
// 分歧 → split（Pass=false，inbox 叫人）；复核通道失败也升级，
// 不能静默降为单 judge。两次调用都不得改变绑定的主候选。
func (c *Context) judgeStep(milestone contract.Milestone) (out judgeOutcome, err error) {
	defer c.measurePhase("judge", milestone.ID)()
	if err := c.err(); err != nil {
		return judgeOutcome{}, err
	}
	ms := c.State.Milestones[milestone.ID]
	gateIdentity := evidenceIdentity(ms.Gate)
	// run 总是先执行 milestoneGateStep。无身份仅保留内部独立 judge 的
	// 兼容调用；正式闭环必须绑定主 checkout 上刚通过的 L2/L3 候选。
	checkIdentity := func(phase string) bool {
		if gateIdentity == "" {
			return true
		}
		identity, snapshotErr := c.candidateFingerprint(c.Cwd)
		gmap := ms.Gate.(map[string]any) // 非空 evidenceIdentity 只来自这个对象形状。
		if snapshotErr == nil && identity == gateIdentity && gmap["pass"] == true {
			return true
		}
		message := phase + ": milestone candidate changed or its gate is not passing"
		if snapshotErr != nil {
			message = fmt.Sprintf("%s: candidate snapshot failed: %v", phase, snapshotErr)
		}
		gmap["identity-error"] = message
		gmap["pass"] = false
		gmap["judge"] = map[string]any{
			"verdict": "error", "error": string(errs.INVALID_REQUEST), "at": c.nowISO(),
			"identity": gateIdentity, "identity-error": message, "rejected-judge": gmap["judge"],
		}
		c.State.Milestones[milestone.ID] = ms
		c.retireSession("judge", milestone.ID, message)
		c.retireSession("judge-panel", milestone.ID, message)
		c.save()
		return false
	}
	if !checkIdentity("before judge") {
		return judgeOutcome{ErrorCode: errs.INVALID_REQUEST}, nil
	}
	defer func() {
		if !checkIdentity("after judge") {
			out.Pass = false
			out.Verdict = nil
			out.ErrorCode = errs.INVALID_REQUEST
		}
	}()
	packets := []*contract.Packet{}
	for _, p := range c.PacketList {
		if m, err := contract.MilestoneOfPacket(p.PacketID); err == nil && m == milestone.ID {
			packets = append(packets, p)
		}
	}
	var gateLevels map[string]any
	if g, ok := ms.Gate.(map[string]any); ok {
		gateLevels, _ = g["levels"].(map[string]any)
	}
	prompt := prompts.BuildJudgePrompt(c.Spec, milestone, packets, c.State, gateLevels, prompts.JudgeOptions{
		TaskDir:       c.TaskDir,
		HumanGuidance: ms.HumanGuidance,
	})

	binding, err := fallback.ResolveBinding(c.Config, "judge", "")
	if err != nil {
		return judgeOutcome{}, err
	}
	r, ferr := c.agentVerdictStep(verdictArgs{role: "judge", binding: binding, prompt: prompt, sessionScope: milestone.ID})
	if ferr != nil {
		return judgeOutcome{}, ferr
	}

	var r2 *verdictOutcome
	if c.Config.JudgePanel != nil && r.OK && !r.Stopped {
		res, ferr := c.agentVerdictStep(verdictArgs{role: "judge", sessionRole: "judge-panel", binding: loopToBinding(*c.Config.JudgePanel), prompt: prompt, sessionScope: milestone.ID})
		if ferr != nil {
			return judgeOutcome{}, ferr
		}
		r2 = &res
	}

	if ms.Gate == nil {
		ms.Gate = map[string]any{"pass": true, "levels": map[string]any{}, "at": c.nowISO()}
		c.State.Milestones[milestone.ID] = ms
	}
	gmap, ok := ms.Gate.(map[string]any)
	if !ok {
		// C11（v0.18）：手工把 gate 写成非对象时，替换为对象并写回 state——
		// 原实现只改内存拷贝，judge 结论落进孤立 map 不落盘。
		gmap = map[string]any{}
		ms.Gate = gmap
		c.State.Milestones[milestone.ID] = ms
	}
	if r.Stopped {
		return judgeOutcome{Stopped: true}, nil
	}
	if !r.OK {
		gmap["judge"] = map[string]any{"verdict": "error", "error": string(r.ErrorCode), "identity": gateIdentity, "at": c.nowISO()}
		c.save()
		return judgeOutcome{ErrorCode: r.ErrorCode}, nil
	}

	verdictRec := func(vo verdictOutcome) map[string]any {
		vb, _ := json.Marshal(vo.Verdict)
		m := map[string]any{}
		_ = json.Unmarshal(vb, &m)
		m["at"] = c.nowISO()
		m["identity"] = gateIdentity
		return m
	}

	judge := verdictRec(r)
	pass := r.Verdict.Verdict == "pass"
	out = judgeOutcome{Pass: pass, Verdict: r.Verdict}
	if r2 != nil {
		switch {
		case r2.Stopped:
			// 面板复核撞上预算硬停：原样透传 Stopped——吞成 secondary-error
			// 会把 budget-stopped 谎报成 judge 失败升级（PR#3 review 中项）。
			return judgeOutcome{Stopped: true}, nil
		case !r2.OK:
			// 复核通道失败：fail-loud，与主判错误同路径升级。
			judge["panel"] = map[string]any{"agreement": "secondary-error",
				"error": string(r2.ErrorCode), "at": c.nowISO()}
			gmap["judge"] = judge
			c.consumeSession("judge", milestone.ID)
			c.consumeSession("judge-panel", milestone.ID)
			c.save()
			return judgeOutcome{ErrorCode: r2.ErrorCode}, nil
		default:
			secPass := r2.Verdict.Verdict == "pass"
			split := secPass != pass
			rec := map[string]any{
				"agreement":     "agree",
				"binding-model": c.Config.JudgePanel.Model,
				"secondary":     verdictRec(*r2),
				"at":            c.nowISO(),
			}
			if split {
				rec["agreement"] = "split"
				merged := make([]string, 0, len(r.Verdict.Reasons)+len(r2.Verdict.Reasons))
				for _, rr := range r.Verdict.Reasons {
					merged = append(merged, "[primary] "+rr)
				}
				for _, rr := range r2.Verdict.Reasons {
					merged = append(merged, "[secondary] "+rr)
				}
				judge["verdict"] = "split"
				judge["score"] = nil
				judge["reasons"] = merged
				out.Split = true
				out.Pass = false
				out.Verdict = &prompts.Verdict{Verdict: "split", Reasons: merged}
			}
			judge["panel"] = rec
		}
	}
	gmap["judge"] = judge
	c.consumeSession("judge", milestone.ID)
	c.consumeSession("judge-panel", milestone.ID)
	c.save()
	return out, nil
}
