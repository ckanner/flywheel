// Package prompts — 提示词构建器与评审裁决解析（DESIGN.md §8 缓存纪律）：
// 稳定契约内容前置（跨次尝试字节一致，吃前缀缓存），易变数据——尝试号、
// 时间戳——只进最末节。
//
// 评审者信息节食（§3、§4.3）：review prompt 只含 packet、git diff、L1 结果
// 与缺陷注入结论，绝不含 executor 的 transcript 或自述。
package prompts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
)

// CacheScopeCap cache-scope 文件内容进入稳定前缀的总量上限（§8.1，约 50KB）。
const CacheScopeCap = 50_000

// ReadCacheScope 把 cache-scope 声明的文件读进稳定前缀，超限截断（§8.1）。
// 长度按字节计（Node 为 UTF-16 code unit，多字节文本下 cap 语义近似）。
func ReadCacheScope(cwd string, files []string) []string {
	if cwd == "" || len(files) == 0 {
		return nil
	}
	sections := []string{"## Cache-scope files (stable prefix, §8)"}
	budget := CacheScopeCap
	truncated := false
	for _, rel := range files {
		if budget <= 0 {
			truncated = true
			break
		}
		content, err := os.ReadFile(filepath.Join(cwd, rel))
		if err != nil {
			sections = append(sections, fmt.Sprintf("### %s\n(not found at runtime — skipped)", rel))
			continue
		}
		c := string(content)
		if len(c) > budget {
			c = c[:budget] + fmt.Sprintf("\n... [cache-scope truncated at %d chars total]", CacheScopeCap)
			truncated = true
		}
		budget -= len(c)
		sections = append(sections, fmt.Sprintf("### %s\n%s", rel, c))
		if truncated {
			break
		}
	}
	if truncated {
		sections = append(sections, "(further cache-scope files omitted by the 50KB cap)")
	}
	return sections
}

func joinNonEmpty(items []string, empty string) string {
	if len(items) == 0 {
		return empty
	}
	return strings.Join(items, ", ")
}

// BuildExecutePrompt 执行单元提示词。stable 段字节稳定；volatile 段只在末尾。
func BuildExecutePrompt(packet *contract.Packet, spec *contract.Spec, taskDir string, cwd string, attempt int) string {
	stable := []string{
		"You are the executor for exactly one task packet of the Flywheel system. Do only this one thing.",
		"",
		"## Spec (stable contract content)",
		"Goal: " + spec.Goal,
		"Non-goals: " + joinNonEmpty(spec.NonGoals, "none"),
		"",
	}
	stable = append(stable, ReadCacheScope(cwd, packet.CacheScope)...)
	maxFiles := "unset"
	if packet.Boundaries.MaxFiles != nil {
		maxFiles = fmt.Sprintf("%d", *packet.Boundaries.MaxFiles)
	}
	stable = append(stable,
		"",
		"## Task packet (contract)",
		"packet-id: "+packet.PacketID,
		"goal: "+packet.Goal,
		"requirements: "+strings.Join(packet.Requirements, ", "),
		"context-files: "+joinNonEmpty(packet.ContextFiles, "none"),
		"boundaries.must-not-touch: "+joinNonEmpty(packet.Boundaries.MustNotTouch, "none"),
		"boundaries.max-files: "+maxFiles,
		"",
		"## Work discipline",
		"1. Read the contract and every context file before writing code.",
		"2. IMPLEMENT ONLY — never write or modify tests. Acceptance tests are provided by the contract or already exist in the repo. Creating or modifying ANY test file fails this unit (git diff is checked).",
		"3. If the design is hard to test, that is a design defect: STOP and escalate so the Plan Loop can adjust — do not touch tests yourself.",
		"4. Run the acceptance commands yourself and report exactly which passed and which failed.",
		"5. NEVER declare the task done. Completion is judged externally by acceptance commands and review.",
		"6. If reality contradicts the design or the work needs a scope change: STOP and say so. Do not expand or shrink scope on your own.",
		"7. Do not touch must-not-touch paths; the git diff is checked after you finish.",
		fmt.Sprintf("8. Finally write your done-report to %s/reports/%s.done.json (absolute path) with EXACTLY these array keys: requirements-covered, changed-files, commands-run, failures. Facts only.", taskDir, packet.PacketID),
		"",
		"## Acceptance commands (L1, run by the orchestrator after you exit)",
	)
	for _, c := range packet.Acceptance.Level("L1") {
		stable = append(stable, fmt.Sprintf("- %s  (expect %s)", c.Command(), c.Expect))
	}
	volatile := []string{
		"",
		"## Runtime (volatile)",
		fmt.Sprintf("attempt: %d", attempt),
		"task directory: " + taskDir,
		"started-at: " + state.NowISO(),
	}
	return strings.Join(append(stable, volatile...), "\n")
}

// ReviewArgs BuildReviewPrompt 输入。l1Evidence/defectInjection 为任意 JSON
// 值（machine 层的证据对象）；diffText 为空 → 提示评审者仅凭 changed-files；
// defectInjection 为 nil → 无该节。
type ReviewArgs struct {
	Packet          *contract.Packet
	L1Evidence      any
	DiffSummary     string
	DiffText        string
	DefectInjection any
}

// BuildReviewPrompt 评审提示词（信息节食：契约 + diff + L1 结果，无 executor 自述）。
func BuildReviewPrompt(a ReviewArgs) string {
	l1JSON, _ := json.MarshalIndent(a.L1Evidence, "", "  ")
	p := a.Packet
	sections := []string{
		"You are the reviewer for one executed unit of the Flywheel system. Judge only what commands cannot.",
		"",
		"## Task packet (contract)",
		"packet-id: " + p.PacketID,
		"goal: " + p.Goal,
		"requirements: " + strings.Join(p.Requirements, ", "),
		"boundaries.must-not-touch: " + joinNonEmpty(p.Boundaries.MustNotTouch, "none"),
		"",
		"## Review rubric (pass line included)",
		p.ReviewRubric,
		"",
		"## L1 deterministic test results (ground truth, already green)",
		string(l1JSON),
	}
	if a.DefectInjection != nil {
		diJSON, _ := json.MarshalIndent(a.DefectInjection, "", "  ")
		sections = append(sections,
			"",
			"## Defect-injection spot check (orchestrator-run, read-only for you)",
			string(diJSON),
			"Judge what this says about test assertion quality: an injected defect the tests did NOT catch proves the tests are fake/weak.",
		)
	}
	diffSummary := a.DiffSummary
	if diffSummary == "" {
		diffSummary = "(unavailable)"
	}
	sections = append(sections, "", "## Changed files", diffSummary)
	if a.DiffText != "" {
		sections = append(sections, "", "## git diff (full, against the unit base ref)", a.DiffText)
	} else {
		sections = append(sections, "", "## git diff", "(diff unavailable — judge from the changed-files list only, and say so in your reasons)")
	}
	sections = append(sections,
		"",
		"## Your checks",
		"- Code quality, project conventions, design intent, security hazards.",
		"- Test assertion quality: coverage proves execution, not assertion validity. Remember the executor never writes tests — any test file in the diff is already a boundary violation.",
		"- Coverage matrix: every numbered requirement above must have at least one verification + evidence; a missing one means return.",
		"- Scope: the diff must stay within the packet boundaries; any change to contract files (spec.md / task-packet-*.md) means return.",
		"",
		"## Output",
		"End your reply with a final line containing ONLY this JSON:",
		`{"verdict":"pass"|"return"|"verification-gap","score":0.0-1.0,"reasons":["..."]}`,
		`Every entry in reasons MUST cite verifiable evidence: an acceptance command with its exit code, or file:line in the diff. A verdict without evidence is invalid.`,
		`Use "verification-gap" when the code looks right but the acceptance is too weak to prove it. The gap goes back to the Plan Loop to strengthen acceptance — the unit is NEVER completed on a gap.`,
	)
	return strings.Join(sections, "\n")
}

// PlanArgs BuildPlanPrompt 输入。
type PlanArgs struct {
	Spec      *contract.Spec
	State     *state.State
	Milestone contract.Milestone
	Packets   []*contract.Packet
	ColdStart bool
	RetroTail string // 冷启动时附 retro 尾部；非冷启动忽略
}

// BuildPlanPrompt 规划提示词（低频 K3；冷启动 = 会话重建协议 §4.1：
// 契约层是唯一记忆）。
func BuildPlanPrompt(a PlanArgs) string {
	titles := make([]string, 0, len(a.Spec.Milestones))
	for _, m := range a.Spec.Milestones {
		titles = append(titles, m.ID+" "+m.Title)
	}
	currentMilestone := "<nil>"
	if a.State.CurrentMilestone != nil {
		currentMilestone = *a.State.CurrentMilestone
	}
	packetIDs := make([]string, 0, len(a.Packets))
	for _, p := range a.Packets {
		packetIDs = append(packetIDs, p.PacketID)
	}
	lines := []string{
		"You are the planner of the Flywheel system. You only produce contract files; you never implement code and never expand requirement scope.",
		"",
		"## Spec (frozen)",
		"Goal: " + a.Spec.Goal,
		"Milestones: " + strings.Join(titles, "; "),
		"",
		"## Current state",
		fmt.Sprintf("status: %s, current milestone: %s", a.State.Status, currentMilestone),
		"",
		fmt.Sprintf("## Task: issue/confirm packets for milestone %s", a.Milestone.ID),
		fmt.Sprintf("Existing packets: %s", joinNonEmpty(packetIDs, "none")),
		"Confirm the packet sequence for this milestone, revise returned packets, or strengthen acceptance after a verification-gap. Scope changes are yours alone to make. Acceptance tests are YOURS to provide or strengthen — the executor never writes tests.",
	}
	if a.ColdStart {
		lines = append(lines, "", "## Cold start note", "This is a fresh planning session rebuilt from the contract layer (previous resident session died). All authoritative context is above/in the contract files on disk.")
		if a.RetroTail != "" {
			lines = append(lines, "", "## retro.md (tail)", a.RetroTail)
		}
	}
	return strings.Join(lines, "\n")
}

// evidenceFlag 从证据对象读取 pass/verdict 标记（JS optional-chaining 等价）。
func evidenceFlag(ev any, key string, want any, yes, no string) string {
	if m, ok := ev.(map[string]any); ok {
		if m[key] == want {
			return yes
		}
	}
	return no
}

// BuildJudgePrompt 里程碑终验提示词（Judge=K3 fresh context）：需求覆盖矩阵
// （防漏做）+ 门禁结果 + 一致性检查。
func BuildJudgePrompt(spec *contract.Spec, milestone contract.Milestone, packets []*contract.Packet, st *state.State, gateLevels map[string]any) string {
	matrix := []string{}
	for _, r := range spec.Requirements {
		if r.Milestone != milestone.ID {
			continue
		}
		var evidence []string
		for _, p := range packets {
			covered := false
			for _, req := range p.Requirements {
				if req == r.ID {
					covered = true
					break
				}
			}
			if !covered {
				continue
			}
			l1Flag, reviewFlag := "L1—", "Review—"
			if ps, ok := st.Packets[p.PacketID]; ok && ps != nil {
				l1Flag = evidenceFlag(ps.Evidence.L1, "pass", true, "L1✓", "L1—")
				reviewFlag = evidenceFlag(ps.Evidence.Review, "verdict", "pass", "Review✓", "Review—")
			}
			evidence = append(evidence, fmt.Sprintf("%s(%s/%s)", p.PacketID, l1Flag, reviewFlag))
		}
		evText := strings.Join(evidence, ", ")
		if evText == "" {
			evText = "NO COVERAGE"
		}
		matrix = append(matrix, fmt.Sprintf("- %s %s -> %s", r.ID, r.Title, evText))
	}
	// Node: JSON.stringify(gate?.levels ?? {}) —— 缺省渲染为 {}。
	gateJSON, _ := json.MarshalIndent(gateLevels, "", "  ")
	if gateLevels == nil {
		gateJSON = []byte("{}")
	}
	return strings.Join([]string{
		"You are the judge for a milestone final verification of the Flywheel system. Fresh context, no prior involvement: re-verify what the execution/review loops produced.",
		"",
		"## Spec",
		"Goal: " + spec.Goal,
		"Milestone: " + milestone.ID + " " + milestone.Title,
		"",
		"## Requirement coverage matrix (anti-omission)",
		strings.Join(matrix, "\n"),
		"",
		"## Milestone gate results (L2/L3, deterministic)",
		string(gateJSON),
		"",
		"## Your checks",
		"- Every numbered requirement of this milestone has at least one verification with evidence; any gap fails.",
		"- The delivered work matches the spec intent; nothing outside the milestone scope was built.",
		"- The evidence is coherent (L1 commands, review verdicts, gate results tell one consistent story).",
		"",
		"## Output",
		"End your reply with a final line containing ONLY this JSON:",
		`{"verdict":"pass"|"return"|"verification-gap","score":0.0-1.0,"reasons":["..."]}`,
		`Every entry in reasons MUST cite verifiable evidence (command + exit code, or file:line). A verdict without evidence is invalid.`,
	}, "\n")
}

// Verdict 评审/Judge 的结构化裁决（输出契约的最后一行 JSON）。
type Verdict struct {
	Verdict string   `json:"verdict"` // pass | return | verification-gap
	Score   *float64 `json:"score"`
	Reasons []string `json:"reasons"`
}

var validVerdicts = map[string]bool{"pass": true, "return": true, "verification-gap": true}

// ParseReviewVerdict 从回复中解析最后一行裁决 JSON。自尾向前扫描；剥掉
// markdown 围栏；非法/不完整行跳过继续向前。找不到则报错。
// v0.18（A3 出口契约强化）：score 必须存在且 reasons ≥1 条——无证据支撑的
// 裁决按 unparseable 处理，走既有重试/升级路径（LLM judge 偏差实证的最低
// 防线：结论必须挂可复核证据，见 DESIGN §4.3）。
func ParseReviewVerdict(text string) (*Verdict, error) {
	if text == "" {
		return nil, fmt.Errorf("empty review output")
	}
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// Node: replace(/^```(?:json)?|```$/g, '') —— 剥首部 ```/```json 与尾部 ```。
		line = strings.TrimPrefix(line, "```json")
		line = strings.TrimPrefix(line, "```")
		line = strings.TrimSuffix(line, "```")
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var v Verdict
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			continue // 向上继续扫
		}
		if !validVerdicts[v.Verdict] {
			continue
		}
		if v.Score == nil || len(v.Reasons) == 0 {
			continue // 无 score / 空理由 = 无证据支撑 → 视为不完整裁决
		}
		return &v, nil
	}
	return nil, fmt.Errorf("no verdict JSON found in review output")
}
