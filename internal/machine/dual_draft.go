package machine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
)

var (
	mkdirDualDraftDir  = os.MkdirAll
	writeDualDraftFile = os.WriteFile
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
}

func (c *Context) shouldDualDraftDesign(r *planning.Request, reviewer bool) bool {
	d := c.Config.Planning.DualDraft
	return !reviewer && designRequest(*r) &&
		planning.DesignGranularity(c.State.Planning, *r) != "compact" &&
		r.Rounds == 1 && d != nil && d.Enabled &&
		!c.sessionReplay("design-plan", "planning:"+r.ID)
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

func (c *Context) planningCallArgs(r *planning.Request, role string, b fallback.Binding, prompt string) drivers.CallArgs {
	return drivers.CallArgs{
		Prompt:      prompt,
		TaskDir:     c.TaskDir,
		Cwd:         c.Cwd,
		MilestoneID: r.ScopeID,
		Model:       b.Model,
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
		})

	var primaryRes, secondaryRes drivers.Result
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		primaryRes = driver.Plan(c.planningCallArgs(r, role, primaryBinding, primaryPrompt))
	}()
	go func() {
		defer wg.Done()
		secondaryRes = driver.Plan(c.planningCallArgs(r, role, secondaryBinding, secondaryPrompt))
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
	if planningResultCode(primaryRes) != "" || primaryRes.Cancelled {
		return primaryRes, role, "", nil
	}
	if planningResultCode(secondaryRes) != "" || secondaryRes.Cancelled {
		return secondaryRes, role, "", nil
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
	if err := c.persistDualDraftCandidates(meta, primaryRes.Text, secondaryRes.Text); err != nil {
		return drivers.Result{}, role, "", fmt.Errorf("persist design candidates: %w", err)
	}

	synthPrompt := fmt.Sprintf("任务账本目录（只读研究）: %q\n%s\n", c.TaskDir, research) +
		prompts.BuildDesignSynthesisPrompt(
			c.State.Planning, *r,
			prompts.DesignDraftInput{Profile: primaryProfile, Harness: primaryBinding.Harness, Model: primaryBinding.Model, Text: primaryRes.Text},
			prompts.DesignDraftInput{Profile: secondaryName, Harness: secondaryBinding.Harness, Model: secondaryBinding.Model, Text: secondaryRes.Text},
			prompts.PlanningPromptOptions{NativeAgents: synthBinding.NativeProfile != "" && synthBinding.Harness != "antigravity"},
		)
	synthRes, err := c.invokeScoped(role, scope, c.planningCallArgs(r, role, synthBinding, synthPrompt), driver.Plan)
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
