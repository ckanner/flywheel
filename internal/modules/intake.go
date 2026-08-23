// Intake Loop（DESIGN.md §4.5）：扫描任务源 → 指纹去重进 triage-inbox.md →
// LLM triage（pi + V4 Flash）逐条给建议。Intake 绝不冻结需求、绝不写契约：
// 人确认候选，冻结仍走 $flywheel-new + 人审（§12）。本 loop 是判断类问题的
// 人机边界。
package modules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/fallback"
)

// Fingerprint 候选指纹（sha256 前 12 hex）。"::" 分隔符保证 (source,title)
// 拼接不会碰撞。
func Fingerprint(sourceName, title string) string {
	sum := sha256.Sum256([]byte(sourceName + "::" + title))
	return hex.EncodeToString(sum[:])[:12]
}

// Candidate 一个任务候选。
type Candidate struct {
	Title      string
	Summary    string
	URL        string
	Source     string
	SourceType string
	FP         string // runIntake 去重后回填
}

// whitespaceFold Node `.replace(/\s+/g,' ').trim()` 等价：原始换行可能把
// "- approval: approved" 走私进 inbox（P1-9，用户实证）。
func whitespaceFold(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ExecFunc 可注入的命令执行（测试用）；nil = 真实 exec。
type ExecFunc func(name string, args []string, cwd string) (stdout string, err error)

func realExec(name string, args []string, cwd string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = cwd
	cmd.Env = childenv.EnvSlice(childenv.GateEnv(childenv.OSLookupEnv, nil))
	out, err := cmd.Output()
	return string(out), err
}

// FetchSourceCandidates 从一个配置源拉取候选。exec 可注入（测试）。
func FetchSourceCandidates(source config.Source, cwd string, fn ExecFunc) ([]Candidate, error) {
	if fn == nil {
		fn = realExec
	}
	switch source.Type {
	case "github-issues":
		args := []string{"issue", "list", "--repo", source.Repo, "--state", "open",
			"--json", "number,title,body,url,labels", "--limit", fmt.Sprint(orLimit(source.Limit))}
		if len(source.Labels) > 0 {
			args = append(args, "--label", strings.Join(source.Labels, ","))
		}
		stdout, err := fn("gh", args, cwd)
		if err != nil {
			return nil, err
		}
		var issues []struct {
			Title string `json:"title"`
			Body  string `json:"body"`
			URL   string `json:"url"`
		}
		if err := json.Unmarshal([]byte(stdout), &issues); err != nil {
			return nil, err
		}
		out := make([]Candidate, 0, len(issues))
		for _, i := range issues {
			out = append(out, Candidate{
				Title:      whitespaceFold(i.Title),
				Summary:    truncate(whitespaceFold(i.Body), 300),
				URL:        i.URL,
				Source:     source.Name,
				SourceType: source.Type,
			})
		}
		return out, nil
	case "command":
		stdout, err := fn("/bin/sh", []string{"-c", source.Cmd}, cwd)
		if err != nil {
			return nil, err
		}
		var candidates []Candidate
		for _, line := range strings.Split(stdout, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "{") {
				var obj struct {
					Title   string `json:"title"`
					Summary string `json:"summary"`
					URL     string `json:"url"`
				}
				if err := json.Unmarshal([]byte(trimmed), &obj); err == nil && obj.Title != "" {
					candidates = append(candidates, Candidate{
						Title:      whitespaceFold(obj.Title),
						Summary:    whitespaceFold(obj.Summary),
						URL:        obj.URL,
						Source:     source.Name,
						SourceType: source.Type,
					})
					continue
				}
				// JSON 解析失败或缺 title → 落到纯行处理
			}
			candidates = append(candidates, Candidate{
				Title:      whitespaceFold(trimmed),
				Source:     source.Name,
				SourceType: source.Type,
			})
		}
		return candidates, nil
	default:
		return nil, fmt.Errorf("unknown intake source type: %s", source.Type)
	}
}

func orLimit(n int) int {
	if n > 0 {
		return n
	}
	return 50
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

var fpCommentRe = regexp.MustCompile(`<!-- fp:([0-9a-f]{12}) -->`)

func parseExistingFingerprints(content string) map[string]bool {
	fps := map[string]bool{}
	for _, m := range fpCommentRe.FindAllStringSubmatch(content, -1) {
		fps[m[1]] = true
	}
	return fps
}

// TriageVerdict triage 建议（recommend 接 / skip 不接 / human 需人判断）。
type TriageVerdict struct {
	Verdict string
	Reason  string
}

var validTriageVerdicts = map[string]bool{"recommend": true, "skip": true, "human": true}

// ParseTriageVerdict 解析 triage 裁决 JSON 行（自尾向前扫描，剥 markdown 围栏）。
func ParseTriageVerdict(text string) (*TriageVerdict, error) {
	if text == "" {
		return nil, fmt.Errorf("empty triage output")
	}
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "```json")
		line = strings.TrimPrefix(line, "```")
		line = strings.TrimSuffix(line, "```")
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		// Node: typeof reason === 'string' ? reason : '' —— reason 非字符串
		// 不使 verdict 失效，仅归空。
		var v struct {
			Verdict string          `json:"verdict"`
			Reason  json.RawMessage `json:"reason"`
		}
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			continue
		}
		if !validTriageVerdicts[v.Verdict] {
			continue
		}
		reason := ""
		if json.Unmarshal(v.Reason, &reason) != nil {
			reason = ""
		}
		return &TriageVerdict{Verdict: v.Verdict, Reason: reason}, nil
	}
	return nil, fmt.Errorf("no verdict JSON found in triage output")
}

func candidateSection(c Candidate, fp string) string {
	url := c.URL
	if url == "" {
		url = "—"
	}
	summary := c.Summary
	if summary == "" {
		summary = "—"
	}
	return strings.Join([]string{
		fmt.Sprintf("## %s — %s", fp, c.Title),
		fmt.Sprintf("- source: %s (%s)", c.Source, c.SourceType),
		fmt.Sprintf("- url: %s", url),
		fmt.Sprintf("- summary: %s", summary),
		"- triage: pending",
		`- approval: pending  (human: change to "approved" to pick this up for /skill:flywheel-new)`,
		fmt.Sprintf("<!-- fp:%s -->", fp),
		"",
	}, "\n")
}

// writeTriageVerdict 把裁决写回对应候选块。函数式替换语义：LLM reason 里含
// "$&"/"$1" 也绝不能被当替换模式解释（P1-9，用户实证）。
var blockReCache = map[string]*regexp.Regexp{}

func writeTriageVerdict(content, fp string, v *TriageVerdict) string {
	re := regexp.MustCompile(`(## ` + fp + ` —[\s\S]*?)- triage: pending`)
	return re.ReplaceAllStringFunc(content, func(match string) string {
		head := match[:strings.Index(match, "- triage: pending")]
		return head + fmt.Sprintf("- triage: %s — %s", v.Verdict, v.Reason)
	})
}

// ListApprovedCandidates 人标记为 approved 的候选（$flywheel-new 的输入；§4.5 人确认）。
func ListApprovedCandidates(taskDir string) []struct{ Fingerprint, Title string } {
	file := filepath.Join(taskDir, "triage-inbox.md")
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	content := string(raw)
	var approved []struct{ Fingerprint, Title string }
	// 组对齐 Node：1=整块 2=fp 3=title 行 4=body（approval 检查在 body）。
	blockRe := regexp.MustCompile(`(?s)(## ([0-9a-f]{12}) — ([^\n]+)\n(.*?))(?:\n## |\z)`)
	for _, m := range blockRe.FindAllStringSubmatch(content, -1) {
		if strings.Contains(m[4], "- approval: approved") {
			approved = append(approved, struct{ Fingerprint, Title string }{m[2], strings.TrimSpace(m[3])})
		}
	}
	return approved
}

func buildTriagePrompt(c Candidate) string {
	url := c.URL
	if url == "" {
		url = "—"
	}
	summary := c.Summary
	if summary == "" {
		summary = "—"
	}
	return strings.Join([]string{
		"You are the triage advisor of the Flywheel system (Intake Loop). You ADVISE on task candidates; you never freeze requirements and never decide — the human does.",
		"",
		"## Candidate",
		"title: " + c.Title,
		"source: " + c.Source + " (" + c.SourceType + ")",
		"url: " + url,
		"summary: " + summary,
		"",
		"## Advise",
		`- "recommend" (建议接): a well-scoped, verifiable engineering task with a clear acceptance oracle.`,
		`- "skip" (建议不接): vague, non-engineering, duplicate-looking, or not actionable.`,
		`- "human" (需人判断): judgement call — priority, product taste, unclear scope.`,
		"",
		"End your reply with a final line containing ONLY this JSON:",
		`{"verdict":"recommend"|"skip"|"human","reason":"<one sentence>"}`,
	}, "\n")
}

type IntakeSummary struct {
	Scanned    int            `json:"scanned"`
	New        int            `json:"new"`
	Duplicates int            `json:"duplicates"`
	Triaged    map[string]int `json:"triaged"` // recommend/skip/human/failed
	InboxFile  string         `json:"inbox"`
	OK         bool           `json:"ok"`
}

// RunIntake 一轮 intake：扫描源（或单个），追加新候选并 triage。源错误只记日志
// 绝不阻断 intake。
func RunIntake(o CtxOpts, sourceName string, execFn ExecFunc) (IntakeSummary, error) {
	m := NewContext(o)
	cfg := m.Config
	sources := cfg.Intake.Sources
	if len(sources) == 0 {
		return IntakeSummary{}, fmt.Errorf("config intake.sources is empty — nothing to scan")
	}
	selected := sources
	if sourceName != "" {
		selected = nil
		for _, s := range sources {
			if s.Name == sourceName {
				selected = append(selected, s)
			}
		}
		if len(selected) == 0 {
			return IntakeSummary{}, fmt.Errorf("unknown intake source: %s", sourceName)
		}
	}

	// 1. 扫描。
	var all []Candidate
	for _, s := range selected {
		candidates, err := FetchSourceCandidates(s, o.Cwd, execFn)
		if err != nil {
			m.Log(fmt.Sprintf("source %s failed: %v — skipped (source errors never block intake)", s.Name, err))
			continue
		}
		m.Log(fmt.Sprintf("source %s: %d candidates", s.Name, len(candidates)))
		all = append(all, candidates...)
	}

	// 2. 对照既有 inbox 去重。
	inboxFile := filepath.Join(o.TaskDir, "triage-inbox.md")
	existing := "# Triage inbox (flywheel intake)\n\n"
	if raw, err := os.ReadFile(inboxFile); err == nil {
		existing = string(raw)
	}
	knownFps := parseExistingFingerprints(existing)
	var fresh []Candidate
	duplicates := 0
	for _, c := range all {
		fp := Fingerprint(c.Source, c.Title)
		if knownFps[fp] {
			duplicates += 1 // 同时覆盖 inbox 已知与批内重复（P1-9）
		} else {
			knownFps[fp] = true
			c.FP = fp
			fresh = append(fresh, c)
		}
	}

	// 3. 追加新候选。
	content := existing
	for _, c := range fresh {
		content += "\n" + candidateSection(c, c.FP) + "\n"
	}
	if len(fresh) > 0 {
		if err := os.WriteFile(inboxFile, []byte(content), 0o644); err != nil {
			return IntakeSummary{}, err
		}
	}

	// 4. 新候选的 LLM triage（只是建议，绝非冻结决定）。
	binding, err := fallback.ResolveBinding(cfg, "intake", "")
	if err != nil {
		return IntakeSummary{}, err
	}
	triaged := map[string]int{"recommend": 0, "skip": 0, "human": 0, "failed": 0}
	for _, c := range fresh {
		r := CallAgent(m, CallOptions{Role: "intake", Binding: binding, Prompt: buildTriagePrompt(c)})
		var verdict *TriageVerdict
		if r.OK {
			verdict, _ = ParseTriageVerdict(r.Text)
		}
		if verdict != nil {
			triaged[verdict.Verdict]++
			content = writeTriageVerdict(content, c.FP, verdict)
		} else {
			triaged["failed"]++
			triaged["human"]++
			code := r.ErrorCode
			reason := "triage failed"
			if !r.OK {
				reason += fmt.Sprintf(" (%s)", code)
			} else {
				reason += " (unparseable output)"
			}
			content = writeTriageVerdict(content, c.FP, &TriageVerdict{
				Verdict: "human", Reason: reason + " — human must look",
			})
		}
		if err := os.WriteFile(inboxFile, []byte(content), 0o644); err != nil {
			return IntakeSummary{}, err
		}
	}

	return IntakeSummary{
		Scanned:    len(all),
		New:        len(fresh),
		Duplicates: duplicates,
		Triaged:    triaged,
		InboxFile:  inboxFile,
		OK:         true,
	}, nil
}
