package machine

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
)

// Queue publication and adoption have an independent filesystem/CAS failure
// boundary, including after a model receipt has been consumed.
var applyQueuedPlanning = (*Context).applyPlanningQueue

func designRequest(r planning.Request) bool {
	scope, _ := planning.CanonicalScope(r.Scope, r.ScopeID)
	return scope == "design"
}

func planningReason(text string) string {
	runes := []rune(text)
	if len(runes) > 8192 {
		return string(runes[:8180]) + "\n[truncated]"
	}
	return text
}

func (c *Context) newPlanningRequest(scope, scopeID, reason string, only bool) (int, error) {
	p := c.State.Planning
	for i, r := range p.Requests {
		if r.InputRevision == p.InputRevision && r.Scope == scope && r.ScopeID == scopeID && (r.Status == "pending" || r.Status == "running") {
			return i, nil
		}
	}
	id := fmt.Sprintf("engine-%d-%d", p.InputRevision, len(p.Requests)+1)
	if err := planning.BeginRequest(p, planning.Request{ID: id, Scope: scope, ScopeID: scopeID, InputRevision: p.InputRevision, PlanningOnly: only, Feedback: planningReason(reason)}); err != nil {
		return 0, err
	}
	p.Reason = planningReason(reason)
	c.save()
	return len(p.Requests) - 1, c.err()
}

func (c *Context) prepareReviewedExecution() (bool, error) {
	if c.State.Handoff != nil {
		return false, fmt.Errorf("task is under human session control; return it before planning")
	}
	if err := c.reconcileSessionReceipts(); err != nil {
		return false, err
	}
	if err := c.checkPlanningExports(true); err != nil {
		return false, err
	}
	if err := c.resumePlanningActivation(); err != nil {
		return false, err
	}
	if err := applyQueuedPlanning(c); err != nil {
		return false, err
	}
	if c.planningPause {
		return c.pausePlanning("反馈已采用；等待你继续 Plan 或 Run。")
	}
	p := c.State.Planning
	for {
		p = c.State.Planning
		index := -1
		for i := range p.Requests {
			r := &p.Requests[i]
			if r.InputRevision != p.InputRevision && (r.Status == "pending" || r.Status == "running") {
				r.Status = "cancelled"
			}
			if index < 0 && r.InputRevision == p.InputRevision && (r.Status == "pending" || r.Status == "running") {
				index = i
			}
		}
		if index < 0 && (p.CurrentDesign == nil || planning.FindDesign(p, *p.CurrentDesign).InputRevision != p.InputRevision) {
			// A spent request stays spent across Run/restart. New user input, not
			// another process, is what creates a new bounded design request.
			for _, r := range p.Requests {
				if r.InputRevision == p.InputRevision && designRequest(r) {
					return c.pausePlanning(p.Reason)
				}
			}
			var err error
			index, err = c.newPlanningRequest("design", "", p.Reason, c.planningOnly)
			if err != nil {
				return false, err
			}
		}
		if index >= 0 {
			if p.Requests[index].PlanningOnly {
				c.planningOnly = true
			}
			ok, err := c.processPlanningRequest(index)
			if err != nil || !ok {
				return false, err
			}
			if err := applyQueuedPlanning(c); err != nil {
				return false, err
			}
			if c.planningPause {
				return c.pausePlanning("反馈已采用；等待你继续 Plan 或 Run。")
			}
			continue
		}
		if p.Phase == "blocked" || p.Phase == "budget-stopped" || p.Phase == "awaiting-input" {
			return c.pausePlanning(p.Reason)
		}
		if p.CurrentDesign == nil || !planning.Authorized(p, *p.CurrentDesign) {
			return c.pausePlanning("设计及独立评审已保存；等待你审阅并批准当前版本。")
		}
		if len(p.CurrentPlans) > 0 {
			if err := c.activatePlanningPlans(); err != nil {
				return false, err
			}
		}
		if c.planningOnly {
			return c.pausePlanning("本次 Plan 已结束；未启动实施。")
		}
		if !planning.StartAllowed(p, *p.CurrentDesign) {
			return c.pausePlanning("方案已批准；等待你明确开始实施。")
		}
		if p.Active == nil || p.Active.Design != *p.CurrentDesign {
			d := planning.FindDesign(p, *p.CurrentDesign)
			if _, err := c.newPlanningRequest("milestone", d.Milestones[0].ID, "细化首个里程碑的实施计划", false); err != nil {
				return false, err
			}
			// All requests share the same receipt/feedback boundary. A queued
			// input can cancel this child before it is eligible for activation.
			continue
		}
		if err := c.loadPlanningContracts(); err != nil {
			return false, err
		}
		if err := c.admitReviewedExecution(); err != nil {
			return false, err
		}
		return true, nil
	}
}

func (c *Context) planningRequestStop(r *planning.Request, phase, reason string) (bool, error) {
	r.Status = phase
	c.State.Planning.Phase, c.State.Planning.Reason = phase, planningReason(reason)
	c.State.Status = "paused"
	if phase == "budget-stopped" {
		c.State.Status = "budget-stopped"
	}
	c.save()
	return false, c.err()
}

func planningResultCode(r drivers.Result) errs.ErrorCode {
	if r.Settled && r.ExitCode != nil && *r.ExitCode == 0 && !r.TimedOut && !r.Cancelled && !r.Truncated && r.Signal == "" && r.SpawnError == "" && r.CleanupError == "" && r.ErrorCode == "" && (r.ErrorMessage == nil || *r.ErrorMessage == "") {
		return ""
	}
	if r.ErrorCode != "" {
		return r.ErrorCode
	}
	return errs.ClassifyError(toSpawnResult(r))
}

func (c *Context) planningBinding(role string, r *planning.Request) (fallback.Binding, error) {
	granularity := planning.DesignGranularity(c.State.Planning, *r)
	selected, source, ok := c.Config.PlanningBindingForRole(role, granularity)
	if !ok {
		return fallback.Binding{}, fmt.Errorf("no loop binding for %s in planning config", role)
	}
	r.BindingSource, r.BindingGranularity = source, granularity
	b := fallback.Binding{Harness: selected.Harness, Model: selected.Model, Channel: selected.Channel, Thinking: selected.Thinking, NativeProfile: selected.NativeProfile, Provider: fallback.ResolveProvider(c.Config, selected.Harness, selected.Provider, selected.Channel)}
	if r.Role != role {
		r.Role, r.Channel = role, b.Channel
	}
	if r.Channel == "" {
		r.Channel = b.Channel
	}
	if r.Channel != b.Channel {
		mapped := fallback.ModelForChannel(c.Config, r.Channel, role, "")
		if mapped == nil {
			return b, fmt.Errorf("no explicit %s model mapping for persisted fallback channel %s", role, r.Channel)
		}
		b.Model, b.Channel = mapped.Model, r.Channel
		if mapped.Harness != "" {
			b.Harness = mapped.Harness
		}
		b.Provider = fallback.ResolveProvider(c.Config, b.Harness, "", b.Channel)
	}
	return b, nil
}

func (c *Context) invokePlanning(r *planning.Request, reviewer bool) (drivers.Result, string, string, error) {
	role := "plan"
	if designRequest(*r) {
		role = "design-plan"
	}
	if reviewer {
		role = "plan-review"
		if designRequest(*r) {
			role = "design-review"
		}
	}
	if err := c.reconcileSessionReceipts(); err != nil {
		return drivers.Result{}, role, "", err
	}
	if err := applyQueuedPlanning(c); err != nil {
		return drivers.Result{}, role, "", err
	}
	if c.planningPause || r.InputRevision != c.State.Planning.InputRevision {
		return drivers.Result{}, role, "", errPlanningQueued
	}
	research, err := c.planningResearchContext()
	if err != nil {
		return drivers.Result{}, role, "", err
	}
	b, err := c.planningBinding(role, r)
	if err != nil {
		return drivers.Result{}, role, "", err
	}
	scope := "planning:" + r.ID
	if !c.sessionReplay(role, scope) && c.budgetGate() == "stop" {
		return drivers.Result{}, role, "", fmt.Errorf("planning budget exhausted")
	}
	if c.Config.DriverKind(role) != "mock" && !c.sessionReplay(role, scope) {
		if key := fallback.MissingChannelEnv(c.Config, b.Channel, c.EnvLookup); key != "" {
			return drivers.Result{}, role, "", fmt.Errorf("planning channel %s requires %s", b.Channel, key)
		}
	}
	c.save() // stage, chosen channel and spent rounds survive dispatch/crash.
	if err := c.err(); err != nil {
		return drivers.Result{}, role, "", err
	}
	driver := c.Drivers.ForRole(role)
	if driver == nil {
		return drivers.Result{}, role, "", fmt.Errorf("no driver for %s", role)
	}
	call := driver.Plan
	if reviewer {
		call = driver.Review
	}
	files := map[string]string{}
	if c.State.Planning.Active != nil {
		files = c.planFiles(c.State.Planning.Active.Plans)
	}
	prompt := fmt.Sprintf("任务账本目录（只读研究）: %q\n%s\n", c.TaskDir, research) + prompts.BuildPlanningPrompt(c.State.Planning, *r, reviewer, files, prompts.PlanningPromptOptions{NativeAgents: b.NativeProfile != ""})
	before, err := c.planningGuard()
	if err != nil {
		return drivers.Result{}, role, "", err
	}
	res, err := c.invokeScoped(role, scope, drivers.CallArgs{Prompt: prompt, TaskDir: c.TaskDir, Cwd: c.Cwd,
		MilestoneID: r.ScopeID, Model: b.Model, Binding: config.LoopBinding{NativeProfile: b.NativeProfile, Harness: b.Harness, Provider: b.Provider, Model: b.Model, Channel: b.Channel, Thinking: b.Thinking},
		PermissionMode: c.Config.PermissionMode(role, b.Harness), APIKey: c.channelApiKey(b.Channel), APIKeys: c.apiKeysFor(b.Channel), Watchdog: c.wdogFor(role)}, call)
	if err != nil {
		return res, role, "", err
	}
	c.recordSpawnMetrics(spawnMetricsArgs{role: role, model: b.Model, channel: b.Channel, prompt: prompt, result: &res})
	if err := c.stopForUnconfirmedCleanup(role, "", res); err != nil {
		return res, role, "", err
	}
	if err := c.err(); err != nil {
		return res, role, "", err
	}
	after, err := c.planningGuard()
	if err != nil || before != after {
		return res, role, "", fmt.Errorf("planning invocation changed files outside its proposal; preserved for review: %v", err)
	}
	invocation := "planning-" + sessionHash([]any{r.ID, r.Rounds, r.Retries, role})[:32]
	if res.Session != nil {
		invocation = res.Session.RequestID
	}
	return res, role, invocation, nil
}

func (c *Context) retryPlanning(r *planning.Request, role string, code errs.ErrorCode, reason string) (bool, error) {
	c.consumeSession(role, "planning:"+r.ID)
	r.Retries++
	c.State.Planning.Reason = planningReason(reason)
	if r.Retries > c.Config.Retry.MaxTransportRetries+planValidationRetries {
		return c.planningRequestStop(r, "blocked", "规划调用/输出重试已耗尽："+reason)
	}
	if errs.ShouldFallback(code) || fallback.AuthFallbackAllowed(c.Config, r.Channel, code) {
		next := fallback.NextChannel(c.Config, r.Channel)
		if next != "" && fallback.ModelForChannel(c.Config, next, role, "") != nil {
			r.Channel = next
		}
	}
	c.save()
	if err := c.err(); err != nil {
		return false, err
	}
	if err := applyQueuedPlanning(c); err != nil {
		return false, err
	}
	if c.planningPause {
		return false, nil
	}
	if r.InputRevision != c.State.Planning.InputRevision {
		return false, nil
	}
	if errs.IsRetryable(code) {
		delay := errs.ComputeBackoff(r.Retries-1, errs.BackoffOptions{BaseDelayMs: c.Config.Retry.BaseDelayMs, MaxDelayMs: c.Config.Retry.MaxDelayMs, Jitter: c.Config.Retry.Jitter})
		c.waitRetry(time.Duration(delay)*time.Millisecond, "")
	}
	return true, c.err()
}

func (c *Context) processPlanningRequest(index int) (bool, error) {
	p := c.State.Planning
	r := &p.Requests[index]
	if !designRequest(*r) && (p.CurrentDesign == nil || !planning.Authorized(p, *p.CurrentDesign)) {
		return c.planningRequestStop(r, "awaiting-input", "请先批准当前整体方案，再细化实施计划。")
	}
	for {
		p = c.State.Planning
		r = &p.Requests[index]
		if c.planningPause {
			return c.pausePlanning("反馈已采用；等待你继续 Plan 或 Run。")
		}
		if err := c.err(); err != nil {
			return false, err
		}
		if r.InputRevision != p.InputRevision {
			r.Status = "cancelled"
			c.save()
			return true, c.err()
		}
		if r.Stage == "" || r.Stage == "planner" {
			if err := planning.BeginRound(p, r.ID); err != nil {
				return c.planningRequestStop(r, "budget-stopped", err.Error())
			}
		}
		reviewer := r.Stage == "reviewer"
		res, role, invocation, err := c.invokePlanning(r, reviewer)
		if err != nil {
			if errors.Is(err, errPlanningQueued) {
				if err := applyQueuedPlanning(c); err != nil {
					return false, err
				}
				return c.pausePlanning("规划输入已变化；先检查反馈和当前设计，再继续 Plan。")
			}
			if c.err() != nil {
				return false, err
			}
			phase := "blocked"
			if c.State.Status == "budget-stopped" {
				phase = "budget-stopped"
			}
			return c.planningRequestStop(r, phase, err.Error())
		}
		code := planningResultCode(res)
		if res.Cancelled {
			c.consumeSession(role, "planning:"+r.ID)
			return c.planningRequestStop(r, "cancelled", "规划调用已取消；已保留用量与终态，不自动重试。")
		}
		if code == "" {
			if reviewer {
				err = c.adoptPlanningReview(r, res.Text, role, invocation)
			} else {
				err = c.adoptPlanningProposal(r, res.Text, invocation)
			}
			if err != nil {
				code = errs.INVALID_REQUEST
			}
		}
		if code != "" {
			reason := string(code)
			if err != nil {
				reason = err.Error()
			}
			if res.ErrorMessage != nil {
				reason += " " + *res.ErrorMessage
			}
			more, err := c.retryPlanning(r, role, code, reason)
			if err != nil || !more {
				return false, err
			}
			continue
		}
		c.consumeSession(role, "planning:"+r.ID)
		c.save() // proposal/review and consumed receipt are one atomic adoption.
		if err := c.err(); err != nil {
			return false, err
		}
		if err := c.checkPlanningExports(true); err != nil {
			return false, err
		}
		if err := applyQueuedPlanning(c); err != nil {
			return false, err
		}
		p = c.State.Planning
		r = &p.Requests[index]
		if r.InputRevision != p.InputRevision {
			r.Status = "cancelled"
			c.save()
			return true, c.err()
		}
		if !reviewer {
			continue
		}
		if r.Status == "completed" {
			return true, nil
		}
		if r.Status != "running" {
			return false, nil
		}
	}
}

func (c *Context) adoptPlanningProposal(r *planning.Request, text, invocation string) error {
	p := c.State.Planning
	if designRequest(*r) {
		var output prompts.DesignOutput
		if err := prompts.DecodePlanningOutput(text, &output); err != nil {
			return err
		}
		if planning.DesignGranularity(p, *r) == "compact" && output.Compact == nil {
			escalateCompact(p, r)
			return nil
		}
		baseline, ok := verify.GetBaseRef(c.Cwd)
		if !ok {
			return fmt.Errorf("design research requires an inspectable Git baseline")
		}
		d, err := planning.SealDesign(planning.Design{Ref: planning.Ref{Revision: len(p.Designs) + 1}, InputRevision: p.InputRevision,
			Document: output.Document, Requirements: output.Requirements, Decisions: output.Decisions, Milestones: output.Milestones,
			ImplementationScope: output.ImplementationScope, Baseline: baseline, PlannerInvocation: invocation})
		if err != nil {
			return err
		}
		compact, err := c.prepareCompactProjection(*r, d, output.Compact)
		if errors.Is(err, errCompactIneligible) {
			escalateCompact(p, r)
			return nil
		}
		if err != nil {
			return err
		}
		if compact != nil {
			err = planning.RecordCompact(p, d, *compact)
		} else {
			err = planning.RecordDesign(p, d)
		}
		if err != nil {
			return err
		}
	} else {
		var output prompts.PlanOutput
		if err := prompts.DecodePlanningOutput(text, &output); err != nil {
			return err
		}
		if err := c.validatePlanningFiles(output.Files, *r); err != nil {
			return err
		}
		dependencies, err := c.planScopeIdentity()
		if err != nil {
			return err
		}
		plan, err := planning.SealPlan(planning.ExecutionPlan{Ref: planning.Ref{Revision: len(p.Plans) + 1}, Design: *p.CurrentDesign, InputRevision: p.InputRevision,
			Scope: r.Scope, ScopeID: r.ScopeID, Files: output.Files, Dependencies: dependencies, ScopeVerdict: "within-envelope", PlannerInvocation: invocation})
		if err != nil {
			return err
		}
		if err := planning.RecordPlan(p, plan); err != nil {
			return err
		}
	}
	r.Stage = "reviewer"
	return nil
}

func (c *Context) adoptPlanningReview(r *planning.Request, text, role, invocation string) error {
	p := c.State.Planning
	var output prompts.DesignReviewOutput
	if err := prompts.DecodePlanningOutput(text, &output); err != nil {
		return err
	}
	if output.Findings == nil {
		output.Findings = []planning.Finding{}
	}
	b, err := c.planningBinding(role, r)
	if err != nil {
		return err
	}
	review := planning.Review{ID: "review-" + sessionHash([]any{r.ID, r.Rounds, r.Retries, invocation})[:32], Kind: "design", Target: *p.CurrentDesign,
		InputRevision: p.InputRevision, Invocation: invocation, Role: role, Binding: sessionHash(b), Verdict: output.Verdict,
		ScopeVerdict: output.ScopeVerdict, Summary: output.Summary, Findings: output.Findings}
	if designRequest(*r) && planning.DesignGranularity(p, *r) == "compact" {
		review.Kind, review.Plan = "compact-v1", p.CurrentPlan
		review.DesignVerdict, review.CoverageVerdict = output.DesignVerdict, output.CoverageVerdict
	}
	if record := c.sessionRecord(role, "planning:"+r.ID); record != nil {
		review.Binding = record.Binding
	}
	if !designRequest(*r) {
		review.Kind = "plan"
		review.Target = p.CurrentPlans[planning.PlanKey(r.Scope, r.ScopeID)]
		design := *p.CurrentDesign
		review.Design = &design
	}
	if err := planning.RecordReview(p, review); err != nil {
		return err
	}
	switch review.Verdict {
	case "pass":
		r.Status, r.Stage = "completed", "finished"
		if designRequest(*r) && p.Policy == "delegated" {
			if err := planning.Approve(p, planning.Approval{Kind: "delegated", Design: review.Target, ReviewID: review.ID,
				RequestID: "delegated-" + review.ID, Source: p.Brief.Authorization, At: c.nowISO(), Start: p.Brief.ImplementationAuthorized}); err != nil {
				return err
			}
		}
	case "revise":
		r.Stage = ""
		if review.Kind == "compact-v1" && review.ScopeVerdict != "within-envelope" {
			r.DesignGranularity, r.Role, r.Channel = "system", "", ""
		}
	case "needs-user-decision":
		r.Status = "awaiting-input"
		c.State.Status = "paused"
	case "blocked":
		r.Status = "blocked"
		c.State.Status = "paused"
	}
	return nil
}

func (c *Context) reviewedMilestonePlan(milestone contract.Milestone, reason string, feedback []string) (planOutcome, error) {
	if err := applyQueuedPlanning(c); err != nil {
		return planOutcome{}, err
	}
	if !planning.Authorized(c.State.Planning, *c.State.Planning.CurrentDesign) || planning.FindDesign(c.State.Planning, *c.State.Planning.CurrentDesign).InputRevision != c.State.Planning.InputRevision {
		_, err := c.pausePlanning("输入已变化，请先重新规划和批准整体方案。")
		return planOutcome{Stopped: true}, err
	}
	index, err := c.newPlanningRequest("milestone", milestone.ID, strings.Join(append([]string{reason}, feedback...), "\n"), c.planningOnly)
	if err != nil {
		return planOutcome{}, err
	}
	ok, err := c.processPlanningRequest(index)
	if err != nil || !ok {
		return planOutcome{Stopped: !ok}, err
	}
	request := c.State.Planning.Requests[index]
	if c.planningPause || request.InputRevision != c.State.Planning.InputRevision || request.Status != "completed" {
		_, err := c.pausePlanning("反馈已采用；本次细化已停止，请重新规划和批准当前整体方案。")
		return planOutcome{Stopped: true}, err
	}
	if err := c.activatePlanningPlans(); err != nil {
		return planOutcome{}, err
	}
	if err := c.loadPlanningContracts(); err != nil {
		return planOutcome{}, err
	}
	return planOutcome{OK: true, Packets: c.PacketList}, nil
}

func (c *Context) runReviewedSinglePlan(packetID string) (TaskSummary, error) {
	c.planningOnly = true
	if err := c.reconcileSessionReceipts(); err != nil {
		return c.summary(), err
	}
	if err := c.checkPlanningExports(true); err != nil {
		return c.summary(), err
	}
	if err := c.resumePlanningActivation(); err != nil {
		return c.summary(), err
	}
	if c.State.Planning.CurrentDesign == nil {
		return c.summary(), fmt.Errorf("run --only plan refines a milestone; use control planning action plan for root design")
	}
	d := planning.FindDesign(c.State.Planning, *c.State.Planning.CurrentDesign)
	id := packetID
	if strings.Contains(id, "-T") {
		var err error
		id, err = contract.MilestoneOfPacket(id)
		if err != nil {
			return c.summary(), err
		}
	}
	if id == "" {
		id = d.Milestones[0].ID
	}
	index, err := c.newPlanningRequest("milestone", id, "显式单步实施计划细化", true)
	if err != nil {
		return c.summary(), err
	}
	ok, err := c.processPlanningRequest(index)
	request := c.State.Planning.Requests[index]
	current := !c.planningPause && request.InputRevision == c.State.Planning.InputRevision && request.Status == "completed"
	if err == nil && ok && current {
		err = c.activatePlanningPlans()
	}
	if err == nil {
		message := "Plan 单步已停止；未启动实施。"
		if request.InputRevision != c.State.Planning.InputRevision {
			message = "反馈已采用；本次细化已停止，请先重新规划当前整体方案。"
		}
		_, err = c.pausePlanning(message)
	}
	return c.summary(), err
}

// A completed independent child review replaces the legacy Plan receipt as the
// trusted checkpoint. It is recomputed after projection and reconciliation.
func (c *Context) reviewedPlanCheckpoint(milestone string) *state.PlanCheckpoint {
	p := c.State.Planning
	if p.Active == nil {
		return nil
	}
	for i := len(p.Active.Plans) - 1; i >= 0; i-- {
		ref := p.Active.Plans[i]
		plan := planning.FindPlan(p, ref)
		scopeMilestone, _ := contract.MilestoneOfPacket(plan.ScopeID)
		if plan.ScopeID != milestone && scopeMilestone != milestone {
			continue
		}
		r := planning.LatestReview(p, "plan", ref)
		if r != nil && r.Verdict == "pass" {
			return &state.PlanCheckpoint{Version: planCheckpointVersion, Milestone: milestone, Inputs: c.planInputIdentity(milestone), Receipt: sessionHash(r), Head: p.Active.Head}
		}
	}
	return nil
}
