package machine

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
)

func (c *Context) effectiveClarifyMode(p *planning.State) string {
	if p.Brief.ClarifyMode != "" {
		return p.Brief.ClarifyMode
	}
	if c.Config.Planning.Clarify != nil && c.Config.Planning.Clarify.Mode != "" {
		return c.Config.Planning.Clarify.Mode
	}
	return "off"
}

func (c *Context) clarifyBinding() (fallback.Binding, error) {
	if cl := c.Config.Planning.Clarify; cl != nil && cl.Profile != "" {
		return c.dualDraftProfileBinding(cl.Profile)
	}
	selected, _, ok := c.Config.PlanningBindingForRole("design-plan", "system")
	if !ok {
		return fallback.Binding{}, fmt.Errorf("no loop binding for design-plan in planning config")
	}
	return fallback.Binding{
		Harness:       selected.Harness,
		Model:         selected.Model,
		Channel:       selected.Channel,
		Thinking:      selected.Thinking,
		NativeProfile: selected.NativeProfile,
		Provider:      fallback.ResolveProvider(c.Config, selected.Harness, selected.Provider, selected.Channel),
	}, nil
}

// jevShouldClarify uses the configured JEV decision engine (when enabled in auto
// mode) to decide whether the incoming brief is already unambiguous or benefits
// from Stage 0 interactive clarification and objective quality-gate discovery.
func (c *Context) jevShouldClarify(p *planning.State) (decided bool, skip bool, reason string) {
	j, _ := c.runtimeJEVConfig(nil)
	if j == nil || !slices.Contains(j.AllowedData, decision.TaskSummary) || j.Backend != "subscription" && c.decisionOptions(j).APIKey == "" {
		return false, false, ""
	}
	snap := decision.Snapshot{
		Version: 2,
		ID:      "clarify-" + p.Brief.ID,
		Identity: decision.Identity{
			TaskID:          firstNonEmpty(c.State.Slug, p.Brief.ID),
			RunID:           sessionHash(c.TaskDir),
			Revision:        c.revision + 1,
			ContractHash:    sessionHash(p.Brief),
			CandidateHash:   sessionHash(p.Brief.OriginalRequest),
			ObservationHash: sessionHash([]any{"stage0-clarify", p.InputRevision}),
		},
		Facts: []decision.Fact{
			{Category: decision.TaskSummary, Text: boundedDecisionText("Goal: "+p.Brief.Goal+"\nRequest: "+p.Brief.OriginalRequest, 2048)},
		},
		MissingFacts: []string{"User has not yet answered upfront clarifying questions or locked measurable quality gates."},
		Actions: []decision.Action{
			{ID: "run-clarify", Kind: decision.Diagnostic, Description: "Run Stage 0 clarification to ask 2-4 targeted questions and lock measurable quality gates before architecture design.", BindingHash: sessionHash("run-clarify")},
			{ID: "skip-clarify", Kind: decision.Replan, Description: "Brief is already detailed, unambiguous, and self-contained; skip Stage 0 and proceed directly to architecture design.", BindingHash: sessionHash("skip-clarify")},
		},
		BaselineID:          "run-clarify",
		SemanticUncertainty: true,
	}
	policy := j.DecisionPolicy()
	opts := c.decisionOptions(j)
	receipt, err := decision.Evaluate(c.decisionContext(), filepath.Join(c.TaskDir, ".investigation", "runtime-decisions"), snap, policy, opts)
	if err != nil {
		return false, false, ""
	}
	selection := decision.Select(snap, receipt, policy, opts)
	return selection.Adopted, selection.ActionID == "skip-clarify", selection.Reason
}

// runClarificationStage executes Stage 0 before the first root design request.
// Returns (proceedToDesign, error).
func (c *Context) runClarificationStage() (bool, error) {
	p := c.State.Planning
	mode := c.effectiveClarifyMode(p)
	if mode == "off" || p.CurrentDesign != nil || len(p.Designs) > 0 {
		return true, nil
	}
	if p.Clarification != nil {
		if p.Clarification.Status == "awaiting-answers" {
			return c.pausePlanning("需求澄清问题与可量化质量门禁已生成；请确认后进入架构设计。")
		}
		return true, nil
	}
	if mode == "auto" {
		if decided, skip, reason := c.jevShouldClarify(p); decided && skip {
			p.Clarification = &planning.Clarification{
				Status:             "skipped",
				DecidedBy:          "jev",
				NeedsClarification: false,
				IsFrontendUI:       inferFrontendUIBrief(p.Brief),
				Summary:            firstNonEmpty(reason, "JEV 评估需求已明确，跳过需求澄清直接进入设计。"),
			}
			c.save()
			return c.err() == nil, c.err()
		}
	}
	p.Phase = "clarifying"
	p.Reason = "Stage 0 正在分析需求边界与可量化质量门禁…"
	c.save()
	if err := c.err(); err != nil {
		return false, err
	}
	b, err := c.clarifyBinding()
	if err != nil {
		return false, err
	}
	driver := c.Drivers.ForRole("design-plan")
	if driver == nil {
		return false, fmt.Errorf("no driver for design-plan")
	}
	research, err := c.planningResearchContext()
	if err != nil {
		return false, err
	}
	prompt := fmt.Sprintf("任务账本目录（只读研究）: %q\n%s\n", c.TaskDir, research) +
		prompts.BuildClarificationPrompt(p, prompts.PlanningPromptOptions{NativeAgents: b.NativeProfile != "" && b.Harness != "antigravity"})
	before, err := c.planningGuard()
	if err != nil {
		return false, err
	}
	invID := "clarify-" + sessionHash([]any{p.Brief.ID, p.InputRevision})[:24]
	args := drivers.CallArgs{
		Subscription:   c.Config.Subscription,
		Prompt:         prompt,
		TaskDir:        c.TaskDir,
		Cwd:            c.Cwd,
		WorkspaceCwd:   c.WorkspaceCwd,
		Model:          b.Model,
		SessionTitle:   c.sessionTitle("clarify", "stage0"),
		Binding:        config.LoopBinding{NativeProfile: b.NativeProfile, Harness: b.Harness, Provider: b.Provider, Model: b.Model, Channel: b.Channel, Thinking: b.Thinking},
		PermissionMode: c.Config.PermissionMode("design-plan", b.Harness),
		APIKey:         c.channelApiKey(b.Channel),
		APIKeys:        c.apiKeysFor(b.Channel),
		Watchdog:       c.wdogFor("design-plan"),
	}
	trace := openLiveTrace(c.TaskDir, "clarify/stage0", "clarify", "stage0", &state.SessionRecord{
		Action: "fresh",
		Last:   &state.SessionInvocation{ID: invID, Attempt: 1},
	}, args, c.nowISO)
	args.OnEvent = trace.event
	res := driver.Plan(args)
	trace.finish(res)
	c.recordSpawnMetrics(spawnMetricsArgs{role: "design-plan", model: b.Model, channel: b.Channel, prompt: prompt, result: &res})
	if err := c.stopForUnconfirmedCleanup("design-plan", "", res); err != nil {
		return false, err
	}
	after, err := c.planningGuard()
	if err != nil || before != after {
		return false, fmt.Errorf("clarification invocation changed workspace files: %v", err)
	}
	if planningResultCode(res) != "" {
		// Clarification is advisory preflight; on failure fall through cleanly rather than deadlocking the task.
		p.Clarification = &planning.Clarification{
			Status:             "skipped",
			DecidedBy:          "fallback",
			NeedsClarification: false,
			IsFrontendUI:       inferFrontendUIBrief(p.Brief),
			Summary:            "需求澄清调用未能返回有效结果，已自动进入架构设计阶段。",
			Invocation:         invID,
		}
		p.Phase = "drafting"
		c.save()
		return c.err() == nil, c.err()
	}
	var out prompts.ClarificationOutput
	if err := prompts.DecodePlanningOutput(res.Text, &out); err != nil {
		p.Clarification = &planning.Clarification{
			Status:             "skipped",
			DecidedBy:          "fallback",
			NeedsClarification: false,
			IsFrontendUI:       inferFrontendUIBrief(p.Brief),
			Summary:            "需求澄清输出格式异常，已自动进入架构设计阶段。",
			Invocation:         invID,
		}
		p.Phase = "drafting"
		c.save()
		return c.err() == nil, c.err()
	}
	needs := out.NeedsClarification
	if mode == "always" && len(out.Questions) > 0 {
		needs = true
	}
	decidedBy := "model"
	if mode == "always" {
		decidedBy = "always"
	} else if c.Config.JEV.Enabled {
		decidedBy = "jev"
	}
	status := "skipped"
	if needs && len(out.Questions) > 0 {
		status = "awaiting-answers"
	}
	p.Clarification = &planning.Clarification{
		Status:             status,
		DecidedBy:          decidedBy,
		NeedsClarification: needs,
		IsFrontendUI:       out.IsFrontendUI || inferFrontendUIBrief(p.Brief),
		Summary:            strings.TrimSpace(out.Summary),
		SuggestedGates:     cleanStringSlice(out.SuggestedGates, 16),
		Questions:          sanitizeClarificationQuestions(out.Questions),
		Invocation:         invID,
	}
	if status == "awaiting-answers" {
		p.Phase = "awaiting-clarification"
		p.Reason = firstNonEmpty(p.Clarification.Summary, "请确认需求澄清问题与可量化质量门禁，然后开始架构设计。")
		c.State.Status = "paused"
		c.save()
		return false, c.err()
	}
	p.Phase = "drafting"
	c.save()
	return c.err() == nil, c.err()
}

func inferFrontendUIBrief(b planning.Brief) bool {
	text := strings.ToLower(b.Goal + " " + b.OriginalRequest)
	for _, kw := range []string{"canvas", "html", "css", "react", "vue", "svelte", "web", "frontend", "ui", "ux", "vite", "tailwind", "像素", "前端", "界面", "游戏", "俄罗斯方块", "网页", "桌面端"} {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func cleanStringSlice(items []string, limit int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range items {
		s := strings.TrimSpace(raw)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func sanitizeClarificationQuestions(in []planning.ClarificationQuestion) []planning.ClarificationQuestion {
	out := []planning.ClarificationQuestion{}
	seen := map[string]bool{}
	for i, q := range in {
		if len(out) >= 8 {
			break
		}
		title := strings.TrimSpace(q.Title)
		if title == "" {
			continue
		}
		id := strings.TrimSpace(q.ID)
		if id == "" || !planningControlID.MatchString(id) || seen[id] {
			id = fmt.Sprintf("Q-%d", i+1)
		}
		seen[id] = true
		kind := q.Kind
		if kind != "choice" && kind != "quality-gate" && kind != "text" {
			kind = "choice"
		}
		out = append(out, planning.ClarificationQuestion{
			ID:          id,
			Title:       title,
			Kind:        kind,
			Options:     cleanStringSlice(q.Options, 12),
			Recommended: strings.TrimSpace(q.Recommended),
			Rationale:   strings.TrimSpace(q.Rationale),
			Required:    q.Required,
		})
	}
	return out
}
