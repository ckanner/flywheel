package machine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
)

var (
	mkdirDualDraftDir   = os.MkdirAll
	writeDualDraftFile  = os.WriteFile
	errDualDraftPartial = errors.New("dual-draft partial completion requires user decision")
)

type dualDraftModelMeta struct {
	Profile string `json:"profile"`
	Harness string `json:"harness"`
	Model   string `json:"model"`
}

type dualDraftMeta struct {
	Revision      int                `json:"revision"`
	InputRevision int                `json:"inputRevision"`
	Primary       dualDraftModelMeta `json:"primary"`
	Secondary     dualDraftModelMeta `json:"secondary"`
	Synthesizer   dualDraftModelMeta `json:"synthesizer"`
	CreatedAt     string             `json:"createdAt"`
	Partial       bool               `json:"partial,omitempty"`
	CompletedSide string             `json:"completedSide,omitempty"`
	FailedSide    string             `json:"failedSide,omitempty"`
	FailureCode   string             `json:"failureCode,omitempty"`
	FailureError  string             `json:"failureError,omitempty"`
}

type dualDraftPartialPayload struct {
	Revision      int    `json:"revision"`
	InputRevision int    `json:"inputRevision"`
	CompletedSide string `json:"completedSide"`
	Invocation    string `json:"invocation"`
	Text          string `json:"text"`
}

func (c *Context) shouldDualDraftDesign(r *planning.Request, reviewer bool) bool {
	d := c.Config.Planning.DualDraft
	return !reviewer && designRequest(*r) &&
		r.DraftMode != "single" &&
		planning.DesignGranularity(c.State.Planning, *r) != "compact" &&
		r.Rounds == 1 && d != nil && d.Enabled &&
		!c.sessionReplay("design-plan", planningSessionScope(r))
}

func (c *Context) dualDraftProfileBinding(profileName string) (fallback.Binding, error) {
	p, ok := c.Config.Profiles[profileName]
	if !ok {
		return fallback.Binding{}, fmt.Errorf("dual-draft profile %q is not defined", profileName)
	}
	b := fallback.Binding{
		Harness:       p.Harness,
		Model:         p.Model,
		Channel:       p.Channel,
		Thinking:      p.Thinking,
		NativeProfile: p.NativeProfile,
		Provider:      fallback.ResolveProvider(c.Config, p.Harness, p.Provider, p.Channel),
	}
	if c.Config.DriverKind("design-plan") != "mock" {
		if key := fallback.MissingChannelEnv(c.Config, b.Channel, c.EnvLookup); key != "" {
			return fallback.Binding{}, fmt.Errorf("dual-draft channel %s requires %s", b.Channel, key)
		}
	}
	return b, nil
}

func extractDesignDraftMarkdown(text string) string {
	var out prompts.DesignOutput
	if prompts.DecodePlanningOutput(text, &out) == nil && out.Document != "" {
		return out.Document
	}
	return text
}

func (c *Context) persistDualDraftCandidates(meta dualDraftMeta, primaryText, secondaryText string) error {
	dir := filepath.Join(c.TaskDir, "design-candidates")
	if err := mkdirDualDraftDir(dir, 0o700); err != nil {
		return err
	}
	prefix := fmt.Sprintf("rev-%06d", meta.Revision)
	metaRaw, _ := json.MarshalIndent(meta, "", "  ")
	for _, item := range []struct {
		name string
		data []byte
	}{
		{prefix + "-primary.md", []byte(extractDesignDraftMarkdown(primaryText))},
		{prefix + "-secondary.md", []byte(extractDesignDraftMarkdown(secondaryText))},
		{prefix + "-meta.json", metaRaw},
	} {
		if err := writeDualDraftFile(filepath.Join(dir, item.name), item.data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) persistDualDraftPartial(payload dualDraftPartialPayload) error {
	dir := filepath.Join(c.TaskDir, "design-candidates")
	raw, _ := json.MarshalIndent(payload, "", "  ")
	return writeDualDraftFile(filepath.Join(dir, fmt.Sprintf("rev-%06d-partial.json", payload.Revision)), raw, 0o600)
}

func (c *Context) loadMatchingDualDraftPartial(revision, inputRevision int) (dualDraftPartialPayload, bool) {
	path := filepath.Join(c.TaskDir, "design-candidates", fmt.Sprintf("rev-%06d-partial.json", revision))
	raw, err := os.ReadFile(path)
	if err != nil {
		return dualDraftPartialPayload{}, false
	}
	var payload dualDraftPartialPayload
	if json.Unmarshal(raw, &payload) != nil || payload.Revision != revision || payload.InputRevision != inputRevision || strings.TrimSpace(payload.Text) == "" || payload.Invocation == "" {
		return dualDraftPartialPayload{}, false
	}
	return payload, true
}

func dualDraftFailureDetail(code errs.ErrorCode, res drivers.Result) string {
	if res.ErrorMessage != nil && strings.TrimSpace(*res.ErrorMessage) != "" {
		return fmt.Sprintf("%s: %s", code, strings.TrimSpace(*res.ErrorMessage))
	}
	return string(code)
}

func (c *Context) planningCallArgs(r *planning.Request, role, scope string, b fallback.Binding, prompt string) drivers.CallArgs {
	return drivers.CallArgs{
		Subscription: c.Config.Subscription,
		Prompt:       prompt,
		TaskDir:      c.TaskDir,
		Cwd:          c.Cwd,
		WorkspaceCwd: c.WorkspaceCwd,
		MilestoneID:  r.ScopeID,
		Model:        b.Model,
		SessionTitle: c.sessionTitle(role, scope),
		Binding: config.LoopBinding{
			NativeProfile: b.NativeProfile,
			Harness:       b.Harness,
			Provider:      b.Provider,
			Model:         b.Model,
			Channel:       b.Channel,
			Thinking:      b.Thinking,
		},
		PermissionMode: c.Config.PermissionMode(role, b.Harness),
		APIKey:         c.channelApiKey(b.Channel),
		APIKeys:        c.apiKeysFor(b.Channel),
		Watchdog:       c.wdogFor(role),
	}
}

func (c *Context) invokeDualDraftDesign(r *planning.Request, role, scope, research string, primaryBinding fallback.Binding, driver drivers.Driver, primaryPrompt, beforeGuard string) (drivers.Result, string, string, error) {
	cfg := c.Config.Planning.DualDraft
	secondaryName := cfg.SecondaryProfile
	synthName := firstNonEmpty(cfg.SynthesizerProfile, secondaryName)
	secondaryBinding, err := c.dualDraftProfileBinding(secondaryName)
	if err != nil {
		return drivers.Result{}, role, "", err
	}
	synthBinding, err := c.dualDraftProfileBinding(synthName)
	if err != nil {
		return drivers.Result{}, role, "", err
	}
	secondaryPrompt := fmt.Sprintf("任务账本目录（只读研究）: %q\n%s\n", c.TaskDir, research) +
		prompts.BuildPlanningPrompt(c.State.Planning, *r, false, map[string]string{}, prompts.PlanningPromptOptions{
			NativeAgents: secondaryBinding.NativeProfile != "" && secondaryBinding.Harness != "antigravity",
			TaskDir:      c.TaskDir,
		})

	var clockMu sync.Mutex
	safeNowISO := func() string {
		clockMu.Lock()
		defer clockMu.Unlock()
		return c.nowISO()
	}
	primaryScope := scope + ":primary"
	secondaryScope := scope + ":secondary"
	primaryArgs := c.planningCallArgs(r, role, primaryScope, primaryBinding, primaryPrompt)
	secondaryArgs := c.planningCallArgs(r, role, secondaryScope, secondaryBinding, secondaryPrompt)
	primaryInvID := "planning-" + sessionHash([]any{r.ID, r.Rounds, r.Retries, role, primaryScope})[:24]
	secondaryInvID := "planning-" + sessionHash([]any{r.ID, r.Rounds, r.Retries, role, secondaryScope})[:24]
	primaryTrace := openLiveTrace(c.TaskDir, sessionKey(role, primaryScope), role, primaryScope, &state.SessionRecord{
		Action: "fresh",
		Last:   &state.SessionInvocation{ID: primaryInvID, Attempt: r.Retries + 1},
	}, primaryArgs, safeNowISO)
	secondaryTrace := openLiveTrace(c.TaskDir, sessionKey(role, secondaryScope), role, secondaryScope, &state.SessionRecord{
		Action: "fresh",
		Last:   &state.SessionInvocation{ID: secondaryInvID, Attempt: r.Retries + 1},
	}, secondaryArgs, safeNowISO)
	primaryArgs.OnEvent = primaryTrace.event
	secondaryArgs.OnEvent = secondaryTrace.event

	var primaryRes, secondaryRes drivers.Result
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		primaryRes = driver.Plan(primaryArgs)
		primaryTrace.finish(primaryRes)
	}()
	go func() {
		defer wg.Done()
		secondaryRes = driver.Plan(secondaryArgs)
		secondaryTrace.finish(secondaryRes)
	}()
	wg.Wait()

	c.recordSpawnMetrics(spawnMetricsArgs{role: role, model: primaryBinding.Model, channel: primaryBinding.Channel, prompt: primaryPrompt, result: &primaryRes})
	c.recordSpawnMetrics(spawnMetricsArgs{role: role, model: secondaryBinding.Model, channel: secondaryBinding.Channel, prompt: secondaryPrompt, result: &secondaryRes})
	if err := c.stopForUnconfirmedCleanup(role, "", primaryRes); err != nil {
		return primaryRes, role, "", err
	}
	if err := c.stopForUnconfirmedCleanup(role, "", secondaryRes); err != nil {
		return secondaryRes, role, "", err
	}
	if err := c.err(); err != nil {
		return drivers.Result{}, role, "", err
	}
	afterDrafts, err := c.planningGuard()
	if err != nil || beforeGuard != afterDrafts {
		return drivers.Result{}, role, "", fmt.Errorf("planning invocation changed files outside its proposal; preserved for review: %v", err)
	}
	if primaryRes.Cancelled {
		return primaryRes, role, "", nil
	}
	if secondaryRes.Cancelled {
		return secondaryRes, role, "", nil
	}
	primaryCode := planningResultCode(primaryRes)
	secondaryCode := planningResultCode(secondaryRes)
	primaryOK := primaryCode == ""
	secondaryOK := secondaryCode == ""
	if !primaryOK && !secondaryOK {
		return primaryRes, role, "", nil
	}

	primaryProfile := firstNonEmpty(c.Config.ProfileRefs["loops.design-plan"], "design-plan")
	meta := dualDraftMeta{
		Revision:      len(c.State.Planning.Designs) + 1,
		InputRevision: c.State.Planning.InputRevision,
		Primary:       dualDraftModelMeta{Profile: primaryProfile, Harness: primaryBinding.Harness, Model: primaryBinding.Model},
		Secondary:     dualDraftModelMeta{Profile: secondaryName, Harness: secondaryBinding.Harness, Model: secondaryBinding.Model},
		Synthesizer:   dualDraftModelMeta{Profile: synthName, Harness: synthBinding.Harness, Model: synthBinding.Model},
		CreatedAt:     c.nowISO(),
	}
	if primaryOK != secondaryOK {
		completedSide, failedSide, completedRes, failCode, failRes := "primary", "secondary", primaryRes, secondaryCode, secondaryRes
		primaryMD, secondaryMD := primaryRes.Text, ""
		if secondaryOK {
			completedSide, failedSide, completedRes, failCode, failRes = "secondary", "primary", secondaryRes, primaryCode, primaryRes
			primaryMD, secondaryMD = "", secondaryRes.Text
		}
		meta.Partial = true
		meta.CompletedSide = completedSide
		meta.FailedSide = failedSide
		meta.FailureCode = string(failCode)
		meta.FailureError = dualDraftFailureDetail(failCode, failRes)
		if err := c.persistDualDraftCandidates(meta, primaryMD, secondaryMD); err != nil {
			return drivers.Result{}, role, "", fmt.Errorf("persist partial design candidate: %w", err)
		}
		partialInv := "planning-partial-" + sessionHash([]any{r.ID, r.Rounds, r.Retries, completedSide})[:24]
		if err := c.persistDualDraftPartial(dualDraftPartialPayload{
			Revision:      meta.Revision,
			InputRevision: meta.InputRevision,
			CompletedSide: completedSide,
			Invocation:    partialInv,
			Text:          completedRes.Text,
		}); err != nil {
			return drivers.Result{}, role, "", fmt.Errorf("persist partial design payload: %w", err)
		}
		return drivers.Result{}, role, "", fmt.Errorf("%w: 已完成 %s 稿，%s 稿失败（%s）；请选择「重试完整双稿」或「仅用已完成单稿继续审查」。", errDualDraftPartial, completedSide, failedSide, meta.FailureError)
	}
	if err := c.persistDualDraftCandidates(meta, primaryRes.Text, secondaryRes.Text); err != nil {
		return drivers.Result{}, role, "", fmt.Errorf("persist design candidates: %w", err)
	}

	synthPrompt := fmt.Sprintf("任务账本目录（只读研究）: %q\n%s\n", c.TaskDir, research) +
		prompts.BuildDesignSynthesisPrompt(
			c.State.Planning, *r,
			prompts.DesignDraftInput{Profile: primaryProfile, Harness: primaryBinding.Harness, Model: primaryBinding.Model, Text: primaryRes.Text},
			prompts.DesignDraftInput{Profile: secondaryName, Harness: secondaryBinding.Harness, Model: secondaryBinding.Model, Text: secondaryRes.Text},
			prompts.PlanningPromptOptions{NativeAgents: synthBinding.NativeProfile != "" && synthBinding.Harness != "antigravity", TaskDir: c.TaskDir},
		)
	synthArgs := c.planningCallArgs(r, role, scope+":synthesize", synthBinding, synthPrompt)
	synthRes, err := c.invokeScoped(role, scope, synthArgs, driver.Plan)
	if err != nil {
		return synthRes, role, "", err
	}
	c.recordSpawnMetrics(spawnMetricsArgs{role: role, model: synthBinding.Model, channel: synthBinding.Channel, prompt: synthPrompt, result: &synthRes})
	if err := c.stopForUnconfirmedCleanup(role, "", synthRes); err != nil {
		return synthRes, role, "", err
	}
	if err := c.err(); err != nil {
		return synthRes, role, "", err
	}
	afterSynth, err := c.planningGuard()
	if err != nil || beforeGuard != afterSynth {
		return synthRes, role, "", fmt.Errorf("planning invocation changed files outside its proposal; preserved for review: %v", err)
	}
	invocation := "planning-" + sessionHash([]any{r.ID, r.Rounds, r.Retries, role})[:32]
	if synthRes.Session != nil {
		invocation = synthRes.Session.RequestID
	}
	return synthRes, role, invocation, nil
}
