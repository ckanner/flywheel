package machine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/prompts"
)

var (
	mkdirUXReviewDir  = os.MkdirAll
	writeUXReviewFile = os.WriteFile
)

type uxReviewRecord struct {
	Candidate string                      `json:"candidate"`
	Profile   string                      `json:"profile"`
	Model     string                      `json:"model"`
	Verdict   string                      `json:"verdict"`
	Summary   string                      `json:"summary"`
	Findings  []prompts.FrontendUXFinding `json:"findings"`
	Refined   bool                        `json:"refined,omitempty"`
	At        string                      `json:"at"`
}

func (c *Context) uxReviewReportPath(packetID string) string {
	return filepath.Join(c.TaskDir, "reports", packetID+".ux-review.json")
}

func (c *Context) readUXReviewRecord(packetID string) *uxReviewRecord {
	raw, err := os.ReadFile(c.uxReviewReportPath(packetID))
	if err != nil {
		return nil
	}
	var rec uxReviewRecord
	if json.Unmarshal(raw, &rec) != nil {
		return nil
	}
	return &rec
}

func (c *Context) frontendUXAuditStep(packet *contract.Packet) (needsRefine bool, stopped bool, err error) {
	if packet.Route != "frontend" || !c.Config.FrontendUXReview.Enabled {
		return false, false, nil
	}
	id := packet.PacketID
	ps := c.State.Packets[id]
	identity := evidenceIdentity(ps.Evidence.L1)
	if existing := c.readUXReviewRecord(id); existing != nil && (existing.Candidate == identity || existing.Refined) {
		return false, false, nil
	}
	if c.budgetGate() == "stop" {
		return false, true, nil
	}
	profileName := c.Config.FrontendUXReview.Profile
	p, ok := c.Config.Profiles[profileName]
	if !ok {
		return false, false, fmt.Errorf("frontend-ux-review profile %q is not defined", profileName)
	}
	provider := fallback.ResolveProvider(c.Config, p.Harness, p.Provider, p.Channel)
	uCwd := c.unitCwd(packet)
	changed, _ := reviewChangedFiles(uCwd, c.baseRefOf(ps))
	executorFiles, _ := c.partitionChangedFiles(changed)
	diffSummary := ""
	if len(executorFiles) > 0 {
		diffSummary = "- " + strings.Join(executorFiles, "\n- ")
	}
	diffText, _ := reviewDiffText(uCwd, c.baseRefOf(ps), c.ledgerExcludes())
	prompt := prompts.BuildFrontendUXReviewPrompt(packet, diffSummary, diffText)
	attemptNo := ps.Attempts
	res := c.Drivers.Review.Review(drivers.CallArgs{
		TaskDir:        c.TaskDir,
		Cwd:            uCwd,
		PacketID:       id,
		PermissionMode: "read-only",
		Prompt:         prompt,
		Model:          p.Model,
		Binding: config.LoopBinding{
			NativeProfile: p.NativeProfile,
			Harness:       p.Harness,
			Provider:      provider,
			Model:         p.Model,
			Channel:       p.Channel,
			Thinking:      p.Thinking,
		},
		APIKey:   c.channelApiKey(p.Channel),
		APIKeys:  c.apiKeysFor(p.Channel),
		Watchdog: c.wdogFor("frontend-ux-review"),
	})
	c.recordSpawnMetrics(spawnMetricsArgs{
		role: "frontend-ux-review", packetID: id, model: p.Model,
		channel: p.Channel, prompt: prompt, result: &res, attempt: &attemptNo,
	})
	if err := c.stopForUnconfirmedCleanup("frontend-ux-review", id, res); err != nil {
		return false, false, err
	}
	if res.Cancelled {
		return false, true, nil
	}
	parsed, parseErr := prompts.ParseFrontendUXReview(firstNonEmpty(res.Text, res.Stdout))
	if parseErr != nil {
		c.Log(fmt.Sprintf("%s: frontend UX review unparseable (%v) — continuing to code review", id, parseErr))
		return false, false, nil
	}
	hasBlocking := false
	for _, f := range parsed.Findings {
		if f.Severity == "blocking" {
			hasBlocking = true
			break
		}
	}
	rec := uxReviewRecord{
		Candidate: identity,
		Profile:   profileName,
		Model:     p.Model,
		Verdict:   parsed.Verdict,
		Summary:   parsed.Summary,
		Findings:  parsed.Findings,
		Refined:   hasBlocking,
		At:        c.nowISO(),
	}
	if err := mkdirUXReviewDir(filepath.Dir(c.uxReviewReportPath(id)), 0o700); err != nil {
		return false, false, err
	}
	raw, _ := json.MarshalIndent(rec, "", "  ")
	if err := writeUXReviewFile(c.uxReviewReportPath(id), raw, 0o600); err != nil {
		return false, false, err
	}
	if hasBlocking {
		ps.LastFeedback = map[string]any{
			"source":   "frontend-ux-review",
			"summary":  parsed.Summary,
			"findings": parsed.Findings,
			"at":       c.nowISO(),
		}
		c.save()
		return true, false, c.err()
	}
	return false, false, nil
}
