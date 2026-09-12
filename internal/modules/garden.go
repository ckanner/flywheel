// Garden Loop（docs/design.md §4.6）：对 loop 自身知识资产的低频维护——从 retro.md
// 蒸馏重复失败模式成 skill/验收提案，并标记腐化的 AGENTS.md/skills 内容。
// Garden 只产出给人审的 PROPOSALS（garden-proposals.md），绝不自己改
// AGENTS.md 或 skills（改的是知识资产，需人审）。
package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/state"
)

// FailurePattern 归一化后重复（>=2 次）的失败模式。
type FailurePattern struct {
	Pattern string
	Count   int
	Lines   []string
}

var (
	failureEvidenceRe = regexp.MustCompile(`failed=[1-9]\d*\b|INBOX|escalated`)
	tsRe              = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T[\d:.]+Z?`)
	packetRe          = regexp.MustCompile(`M\d{2,}-T\d{2,}`)
	milestoneRe       = regexp.MustCompile(`M-\d{2,}`)
	numberRe          = regexp.MustCompile(`\d+`)
	spacesRe          = regexp.MustCompile(`\s+`)
)

// ExtractFailurePatterns retro.md 的确定性预处理：packet id、时间戳、数字被
// 剥掉，"同一种"失败才能计在一起。只认失败证据行——"failed=0" 成功行与
// "defects(review-failed)=0" 摘要行不是失败（P1-14，用户实证）。
func ExtractFailurePatterns(retroContent string, minCount int) []FailurePattern {
	if minCount <= 0 {
		minCount = 2
	}
	buckets := map[string]*FailurePattern{}
	for _, rawLine := range strings.Split(retroContent, "\n") {
		line := strings.TrimSpace(rawLine)
		if !failureEvidenceRe.MatchString(line) {
			continue
		}
		normalized := tsRe.ReplaceAllString(line, "")
		normalized = packetRe.ReplaceAllString(normalized, "<packet>")
		normalized = milestoneRe.ReplaceAllString(normalized, "<milestone>")
		normalized = numberRe.ReplaceAllString(normalized, "<n>")
		normalized = spacesRe.ReplaceAllString(normalized, " ")
		normalized = strings.TrimSpace(normalized)
		b := buckets[normalized]
		if b == nil {
			b = &FailurePattern{Pattern: normalized}
			buckets[normalized] = b
		}
		b.Count++
		b.Lines = append(b.Lines, line)
	}
	out := make([]FailurePattern, 0, len(buckets))
	for _, b := range buckets {
		if b.Count >= minCount {
			out = append(out, *b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

type gardenInputs struct {
	retro  string
	agents string // "" = 无
	skills []string
}

func collectInputs(taskDir, cwd string) gardenInputs {
	var in gardenInputs
	if raw, err := os.ReadFile(filepath.Join(taskDir, "retro.md")); err == nil {
		in.retro = string(raw)
	}
	if raw, err := os.ReadFile(filepath.Join(cwd, "AGENTS.md")); err == nil {
		in.agents = string(raw)
	}
	entries, err := os.ReadDir(filepath.Join(cwd, "skills"))
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				in.skills = append(in.skills, e.Name())
			}
		}
	}
	sort.Strings(in.skills)
	return in
}

func buildGardenPrompt(patterns []FailurePattern, retroTail, agents string, skills []string) string {
	patternLines := "(none repeated >= 2 times)"
	if len(patterns) > 0 {
		var ls []string
		for _, p := range patterns {
			n := len(p.Lines)
			if n > 3 {
				n = 3
			}
			quoted := make([]string, 0, n)
			for _, l := range p.Lines[:n] {
				quoted = append(quoted, `"`+l+`"`)
			}
			ls = append(ls, fmt.Sprintf("- x%d: %s\n  evidence: %s", p.Count, p.Pattern, strings.Join(quoted, "; ")))
		}
		patternLines = strings.Join(ls, "\n")
	}
	agentsSection := agents
	if agentsSection == "" {
		agentsSection = "(none in this repo)"
	}
	skillsSection := "(none)"
	if len(skills) > 0 {
		var ls []string
		for _, s := range skills {
			ls = append(ls, "- "+s)
		}
		skillsSection = strings.Join(ls, "\n")
	}
	return strings.Join([]string{
		"You are the gardener of the Flywheel system (Garden Loop). You maintain the loop's OWN knowledge assets. You produce PROPOSALS ONLY — a human reviews and applies them. You never edit AGENTS.md or skills yourself.",
		"",
		"## Recurring failure patterns from retro.md (deterministic pre-pass)",
		patternLines,
		"",
		"## retro.md (tail)",
		orEmpty(retroTail),
		"",
		"## Current AGENTS.md",
		agentsSection,
		"",
		"## Installed skills",
		skillsSection,
		"",
		"## Produce garden-proposals.md content",
		"1. For every recurring failure pattern: a proposal to prevent it — a new skill, an acceptance rule, or a contract-template improvement.",
		"2. For suspected decay (AGENTS.md/skills entries that contradict the current implementation shown above): a cleanup proposal.",
		"3. Every proposal MUST cite evidence: quote the retro line or name the file and section it refers to. No evidence, no proposal.",
		"4. Keep it to the most valuable proposals (<= 5).",
		"",
		"## Output format (strict)",
		"## GP-01 — <short title>",
		"- type: failure-pattern | decay",
		"- evidence: <quoted retro line(s) or file reference>",
		"- proposal: <what a human should do>",
		"(repeat GP-02, GP-03, ...)",
	}, "\n")
}

func orEmpty(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

// ProposalValidation validateGardenProposals 结果。
type ProposalValidation struct {
	OK        bool
	Proposals []struct {
		Title    string
		Evidence string
	}
	Errors []string
}

var (
	gpBlockHeadRe  = regexp.MustCompile(`(?m)^## GP-\d+ — `)
	gpTitleRe      = regexp.MustCompile(`(?m)^## (GP-\d+ — .+)$`)
	gpEvidenceRe   = regexp.MustCompile(`(?m)^- evidence: (.+)$`)
	gpProposalRe   = regexp.MustCompile(`(?m)^- proposal: .+`)
	gpTypeRe       = regexp.MustCompile(`(?m)^- type: (failure-pattern|decay)`)
	evidenceCiteRe = regexp.MustCompile(`retro\.md|AGENTS\.md|skills/`)
)

// ValidateGardenProposals 提案结构校验：每个 GP 块必须携带引用 retro.md /
// AGENTS.md / skills/ 的 evidence 行（带证据是全部意义所在——无支持的提案是噪声）。
func ValidateGardenProposals(content string) ProposalValidation {
	v := ProposalValidation{}
	// Node 用 lookahead 分割（(?=^## GP-\d+ — )）；Go regexp 无 lookahead，
	// 改为按标题起点定位后切片——语义等价（每个块含自己的标题行）。
	locs := gpBlockHeadRe.FindAllStringIndex(content, -1)
	var gps []string
	for i, loc := range locs {
		end := len(content)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		gps = append(gps, content[loc[0]:end])
	}
	if len(gps) == 0 {
		return ProposalValidation{OK: false, Errors: []string{"no GP blocks found"}}
	}
	for _, block := range gps {
		title := "(untitled)"
		if m := gpTitleRe.FindStringSubmatch(block); m != nil {
			title = m[1]
		}
		evidence := ""
		if m := gpEvidenceRe.FindStringSubmatch(block); m != nil {
			evidence = m[1]
		}
		switch {
		case evidence == "":
			v.Errors = append(v.Errors, title+": missing evidence line")
		case !evidenceCiteRe.MatchString(evidence):
			v.Errors = append(v.Errors, title+": evidence does not cite retro.md / AGENTS.md / skills/")
		}
		if !gpProposalRe.MatchString(block) {
			v.Errors = append(v.Errors, title+": missing proposal line")
		}
		if !gpTypeRe.MatchString(block) {
			v.Errors = append(v.Errors, title+": missing/unknown type")
		}
		p := struct {
			Title    string
			Evidence string
		}{Title: title}
		if evidence != "" {
			p.Evidence = evidence
		}
		v.Proposals = append(v.Proposals, p)
	}
	v.OK = len(v.Errors) == 0
	return v
}

// GardenSummary 一轮 garden 的结果（JSON 键与 Node 版逐字一致）。
type GardenSummary struct {
	OK        bool     `json:"ok"`
	File      *string  `json:"file"` // 调用失败时 null
	Proposals int      `json:"proposals"`
	Errors    []string `json:"errors"`
}

// RunGarden 一轮 garden：用 K3 分析 retro + 知识资产，写提案。
func RunGarden(o CtxOpts) (GardenSummary, error) {
	m := NewContext(o)
	in := collectInputs(o.TaskDir, o.Cwd)
	patterns := ExtractFailurePatterns(in.retro, 2)
	m.Log(fmt.Sprintf("garden inputs: retro %d chars, %d recurring patterns, AGENTS.md %s, %d skills",
		len(in.retro), len(patterns), map[bool]string{true: "present", false: "absent"}[in.agents != ""], len(in.skills)))

	tailLines := strings.Split(in.retro, "\n")
	if len(tailLines) > 40 {
		tailLines = tailLines[len(tailLines)-40:]
	}
	prompt := buildGardenPrompt(patterns, strings.Join(tailLines, "\n"), in.agents, in.skills)
	binding, err := fallback.ResolveBinding(m.Config, "garden", "")
	if err != nil {
		return GardenSummary{}, err
	}
	r := CallAgent(m, CallOptions{Role: "garden", Binding: binding, Prompt: prompt})
	if !r.OK {
		return GardenSummary{OK: false, Errors: []string{fmt.Sprintf("garden agent call failed: %s", r.ErrorCode)}}, nil
	}

	file := filepath.Join(o.TaskDir, "garden-proposals.md")
	header := fmt.Sprintf("# Garden proposals — %s\n\n"+ // Node toISOString 等价
		"> Proposals only. A HUMAN reviews and applies them — Garden never edits AGENTS.md or skills (§4.6).\n\n", state.NowISO())
	validation := ValidateGardenProposals(r.Text)
	if err := os.WriteFile(file, []byte(header+r.Text+"\n"), 0o644); err != nil {
		return GardenSummary{}, err
	}
	if !validation.OK {
		m.Log(fmt.Sprintf("garden proposals failed validation: %s (raw output kept for the human)", strings.Join(validation.Errors, "; ")))
	}
	f := file
	return GardenSummary{OK: validation.OK, File: &f, Proposals: len(validation.Proposals), Errors: validation.Errors}, nil
}
