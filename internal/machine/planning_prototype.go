package machine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
)

var (
	mkdirPrototypeDir  = os.MkdirAll
	writePrototypeFile = os.WriteFile
)

func (c *Context) effectiveUIPrototypeMode(p *planning.State) string {
	if p.Clarification != nil && p.Clarification.UIPrototypeMode != "" {
		return p.Clarification.UIPrototypeMode
	}
	if p.Brief.UIPrototypeMode != "" {
		return p.Brief.UIPrototypeMode
	}
	if c.Config.Planning.UIPrototype != nil && c.Config.Planning.UIPrototype.Mode != "" {
		return c.Config.Planning.UIPrototype.Mode
	}
	return "off"
}

func (c *Context) shouldRunUIPrototype(p *planning.State, design *planning.Design) bool {
	mode := c.effectiveUIPrototypeMode(p)
	if mode == "off" {
		return false
	}
	if mode == "always" {
		return true
	}
	if p.Clarification != nil && p.Clarification.IsFrontendUI {
		return true
	}
	if inferFrontendUIBrief(p.Brief) {
		return true
	}
	text := strings.ToLower(design.Document)
	for _, kw := range []string{"canvas", "index.html", "react", "css", "tailwind", "ui/ux", "组件", "界面渲染"} {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func (c *Context) uiPrototypeBinding(review bool) (fallback.Binding, error) {
	if ui := c.Config.Planning.UIPrototype; ui != nil {
		profile := ui.Profile
		if review && ui.ReviewProfile != "" {
			profile = ui.ReviewProfile
		}
		if profile != "" {
			return c.dualDraftProfileBinding(profile)
		}
	}
	if c.Config.FrontendUXReview.Enabled && c.Config.FrontendUXReview.Profile != "" {
		return c.dualDraftProfileBinding(c.Config.FrontendUXReview.Profile)
	}
	role := "design-plan"
	if review {
		role = "design-review"
	}
	selected, _, ok := c.Config.PlanningBindingForRole(role, "system")
	if !ok {
		return fallback.Binding{}, fmt.Errorf("no loop binding for %s in planning config", role)
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

// runUIPrototypeStage executes conditional Stage 1.5 after architecture approval
// and before Milestone M-01 execution planning.
func (c *Context) runUIPrototypeStage() (bool, error) {
	p := c.State.Planning
	if p.CurrentDesign == nil {
		return true, nil
	}
	design := planning.FindDesign(p, *p.CurrentDesign)
	if design == nil || !c.shouldRunUIPrototype(p, design) {
		return true, nil
	}
	if p.UIPrototype != nil && p.UIPrototype.Design == *p.CurrentDesign {
		switch p.UIPrototype.Status {
		case "approved", "skipped":
			return true, nil
		case "awaiting-approval":
			return c.pausePlanning("UI/UX 交互原型已生成；请预览并批准后开始编码，或提交视觉调整意见。")
		}
	}
	rev := 1
	var feedback []string
	if p.UIPrototype != nil && p.UIPrototype.Design == *p.CurrentDesign {
		rev = p.UIPrototype.Revision + 1
		feedback = p.UIPrototype.Feedback
	}
	p.Phase = "prototyping"
	p.Reason = fmt.Sprintf("正在生成第 %d 版 UI/UX 交互视觉原型…", rev)
	c.save()
	if err := c.err(); err != nil {
		return false, err
	}

	b, err := c.uiPrototypeBinding(false)
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
		prompts.BuildUIPrototypePrompt(p, design, prompts.PlanningPromptOptions{NativeAgents: b.NativeProfile != "" && b.Harness != "antigravity"})
	before, err := c.planningGuard()
	if err != nil {
		return false, err
	}
	invID := fmt.Sprintf("uiproto-%d-%s", rev, sessionHash([]any{design.Ref, rev, len(feedback)})[:20])
	args := drivers.CallArgs{
		Subscription:   c.Config.Subscription,
		Prompt:         prompt,
		TaskDir:        c.TaskDir,
		Cwd:            c.Cwd,
		WorkspaceCwd:   c.WorkspaceCwd,
		Model:          b.Model,
		SessionTitle:   c.sessionTitle("ui-prototype", fmt.Sprintf("rev-%d", rev)),
		Binding:        config.LoopBinding{NativeProfile: b.NativeProfile, Harness: b.Harness, Provider: b.Provider, Model: b.Model, Channel: b.Channel, Thinking: b.Thinking},
		PermissionMode: c.Config.PermissionMode("design-plan", b.Harness),
		APIKey:         c.channelApiKey(b.Channel),
		APIKeys:        c.apiKeysFor(b.Channel),
		Watchdog:       c.wdogFor("design-plan"),
	}
	trace := openLiveTrace(c.TaskDir, fmt.Sprintf("ui-prototype/rev-%d", rev), "ui-prototype", fmt.Sprintf("rev-%d", rev), &state.SessionRecord{
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
		return false, fmt.Errorf("ui-prototype invocation changed workspace files: %v", err)
	}
	if planningResultCode(res) != "" {
		return c.pausePlanning("UI/UX 视觉原型生成失败，请重试或选择跳过原型直接编码。")
	}
	out, err := prompts.DecodeUIPrototypeOutput(res.Text)
	if err != nil || strings.TrimSpace(out.HTML) == "" || len(out.HTML) > 524288 {
		return c.pausePlanning("UI/UX 视觉原型输出无效或超过 512KB 上限，请重试或跳过原型直接编码。")
	}
	relPath := fmt.Sprintf("ui-prototypes/ui-prototype-%06d.html", rev)
	absPath := filepath.Join(c.TaskDir, relPath)
	if err := mkdirPrototypeDir(filepath.Dir(absPath), 0o700); err != nil {
		return false, err
	}
	if err := writePrototypeFile(absPath, []byte(out.HTML), 0o600); err != nil {
		return false, err
	}
	proto := &planning.UIPrototype{
		Revision:     rev,
		Design:       *p.CurrentDesign,
		Status:       "reviewing",
		HTMLFile:     relPath,
		Summary:      strings.TrimSpace(out.Summary),
		DesignTokens: out.DesignTokens,
		States:       cleanStringSlice(out.States, 32),
		Feedback:     feedback,
		Model:        b.Model,
		Invocation:   invID,
	}
	// Run 1 lightweight independent UX review of the generated prototype (advisory; never blocks human preview).
	proto.Review = c.reviewUIPrototype(p, design, proto, out.HTML)
	proto.Status = "awaiting-approval"
	p.UIPrototype = proto
	p.Phase = "awaiting-prototype-approval"
	p.Reason = fmt.Sprintf("第 %d 版 UI/UX 交互原型已就绪；请在线预览并批准，或提交视觉调整意见。", rev)
	c.State.Status = "paused"
	c.save()
	return false, c.err()
}

func (c *Context) reviewUIPrototype(p *planning.State, design *planning.Design, proto *planning.UIPrototype, html string) *planning.UIPrototypeReview {
	b, err := c.uiPrototypeBinding(true)
	if err != nil {
		return nil
	}
	driver := c.Drivers.ForRole("design-review")
	if driver == nil {
		return nil
	}
	prompt := prompts.BuildUIPrototypeReviewPrompt(p, design, proto, html, prompts.PlanningPromptOptions{NativeAgents: false})
	invID := fmt.Sprintf("uiproto-rev-%d-%s", proto.Revision, sessionHash([]any{design.Ref, proto.Revision})[:16])
	perm := c.Config.PermissionMode("design-review", b.Harness)
	args := drivers.CallArgs{
		Subscription:   c.Config.Subscription,
		EvidenceOnly:   b.Harness == "antigravity" && perm == "read-only",
		Prompt:         prompt,
		TaskDir:        c.TaskDir,
		Cwd:            c.Cwd,
		WorkspaceCwd:   c.WorkspaceCwd,
		Model:          b.Model,
		SessionTitle:   c.sessionTitle("ui-prototype-review", fmt.Sprintf("rev-%d", proto.Revision)),
		Binding:        config.LoopBinding{NativeProfile: b.NativeProfile, Harness: b.Harness, Provider: b.Provider, Model: b.Model, Channel: b.Channel, Thinking: b.Thinking},
		PermissionMode: perm,
		APIKey:         c.channelApiKey(b.Channel),
		APIKeys:        c.apiKeysFor(b.Channel),
		Watchdog:       c.wdogFor("design-review"),
	}
	trace := openLiveTrace(c.TaskDir, fmt.Sprintf("ui-prototype-review/rev-%d", proto.Revision), "ui-prototype-review", fmt.Sprintf("rev-%d", proto.Revision), &state.SessionRecord{
		Action: "fresh",
		Last:   &state.SessionInvocation{ID: invID, Attempt: 1},
	}, args, c.nowISO)
	args.OnEvent = trace.event
	res := driver.Review(args)
	trace.finish(res)
	c.recordSpawnMetrics(spawnMetricsArgs{role: "design-review", model: b.Model, channel: b.Channel, prompt: prompt, result: &res})
	if planningResultCode(res) != "" {
		return nil
	}
	var out prompts.UIPrototypeReviewOutput
	if err := prompts.DecodePlanningOutput(res.Text, &out); err != nil {
		return nil
	}
	if out.Findings == nil {
		out.Findings = []planning.Finding{}
	}
	verdict := out.Verdict
	if verdict != "pass" && verdict != "revise" {
		verdict = "pass"
	}
	return &planning.UIPrototypeReview{
		Verdict:  verdict,
		Summary:  strings.TrimSpace(out.Summary),
		Findings: out.Findings,
		Model:    b.Model,
		At:       c.nowISO(),
	}
}
