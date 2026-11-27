package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
)

func testPacket() *contract.Packet {
	mf := 6
	return &contract.Packet{
		PacketID:     "M01-T01",
		Goal:         "Implement the parser.",
		Requirements: []string{"R-01", "R-02"},
		CacheScope:   []string{"docs/spec.md"},
		ContextFiles: []string{"src/a.go"},
		Boundaries:   contract.Boundaries{MustNotTouch: []string{"src/payments/"}, MaxFiles: &mf},
		Acceptance: contract.Acceptance{
			L1: []contract.GateEntry{{Cmd: "go test ./...", Expect: "exit 0"}},
			L3: []contract.GateEntry{{Script: "e2e.ts", Expect: "exit 0"}},
		},
		ReviewRubric: "Pass line: score >= 0.8.",
	}
}

func testSpec() *contract.Spec {
	return &contract.Spec{
		Goal:     "Build a parser library",
		NonGoals: nil,
		Milestones: []contract.Milestone{
			{ID: "M-01", Title: "Parser core"},
			{ID: "M-02", Title: "CLI"},
		},
	}
}

func TestReadCacheScope(t *testing.T) {
	assert.Nil(t, ReadCacheScope("", []string{"a"}))
	assert.Nil(t, ReadCacheScope(t.TempDir(), nil))

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.md"), []byte("SPEC-CONTENT"), 0o644))
	sections := ReadCacheScope(dir, []string{"spec.md", "missing.md"})
	assert.Equal(t, []string{
		"## Cache-scope files (stable prefix, §8)",
		"### spec.md\nSPEC-CONTENT",
		"### missing.md\n(not found at runtime — skipped)",
	}, sections)
}

func TestExecuteFeedbackPreservesStableContract(t *testing.T) {
	p := testPacket()
	initial := BuildExecutePrompt(p, testSpec(), "/task", "", 1)
	retry := BuildExecutePrompt(p, testSpec(), "/task", "", 2, map[string]any{"error-code": "L1_FAILED", "output": "assertion: got 2 want 3"})
	assert.Equal(t, strings.Split(initial, "## Runtime")[0], strings.Split(retry, "## Runtime")[0])
	assert.Contains(t, retry, "assertion: got 2 want 3")
	assert.Contains(t, retry, "unit/regression tests")
	assert.Contains(t, retry, "protected-test-patterns")
}

func TestReadCacheScopeTruncatesAtCap(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", CacheScopeCap+100)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.txt"), []byte(big), 0o644))
	sections := ReadCacheScope(dir, []string{"big.txt"})
	require.Len(t, sections, 3)
	assert.Contains(t, sections[1], fmt.Sprintf("... [cache-scope truncated at %d chars total]", CacheScopeCap))
	assert.Equal(t, "(further cache-scope files omitted by the 50KB cap)", sections[2])
}

func TestReadCacheScopeSecondFileOverBudget(t *testing.T) {
	dir := t.TempDir()
	// 第一个文件恰好耗尽预算（cap 字节 = budget，不触发截断）→
	// 第二个文件命中 budget<=0 分支 → truncated break。
	first := strings.Repeat("a", CacheScopeCap)
	second := "tiny"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "first.txt"), []byte(first), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "second.txt"), []byte(second), 0o644))
	sections := ReadCacheScope(dir, []string{"first.txt", "second.txt"})
	require.Len(t, sections, 3)
	assert.NotContains(t, sections[1], "truncated") // 恰好 cap 不截断（len>budget 为假）
	assert.Equal(t, "(further cache-scope files omitted by the 50KB cap)", sections[2])
}

func TestBuildExecutePromptStableThenVolatile(t *testing.T) {
	pkt := testPacket()
	spec := testSpec()
	out := BuildExecutePrompt(pkt, spec, "/abs/taskdir", "", 2)

	stableIdx := strings.Index(out, "## Task packet (contract)")
	volatileIdx := strings.Index(out, "## Runtime (volatile)")
	require.Greater(t, stableIdx, 0)
	require.Greater(t, volatileIdx, stableIdx, "缓存纪律：易变段必须在稳定契约段之后")

	// 稳定内容逐项在位。
	assert.Contains(t, out, "Goal: Build a parser library")
	assert.Contains(t, out, "Non-goals: none")
	assert.Contains(t, out, "packet-id: M01-T01")
	assert.Contains(t, out, "requirements: R-01, R-02")
	assert.Contains(t, out, "boundaries.max-files: 6")
	assert.Contains(t, out, "- go test ./...  (expect exit 0)")

	// 易变段只在末尾且含 attempt；started-at 在最后。
	lines := strings.Split(out, "\n")
	last := lines[len(lines)-1]
	assert.True(t, strings.HasPrefix(last, "started-at: "))
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`, strings.TrimPrefix(last, "started-at: "))
	assert.Contains(t, out, "attempt: 2")
	assert.Contains(t, out, "task directory: /abs/taskdir")

	// 同一 packet 两次构建：volatile 之前字节一致（前缀缓存可命中）。
	out2 := BuildExecutePrompt(pkt, spec, "/abs/taskdir", "", 3)
	assert.Equal(t, out[:stableIdx], out2[:stableIdx])

	// script 型 L1 条目用 Command() 回退。
	scriptOnly := &contract.Acceptance{L1: []contract.GateEntry{{Script: "e2e/auth.ts", Expect: "exit 0"}}}
	pkt.Acceptance = *scriptOnly
	assert.Contains(t, BuildExecutePrompt(pkt, spec, "/t", "", 1), "- e2e/auth.ts  (expect exit 0)")
}

func TestBuildExecutePromptMaxFilesUnset(t *testing.T) {
	pkt := testPacket()
	pkt.Boundaries.MaxFiles = nil
	assert.Contains(t, BuildExecutePrompt(pkt, testSpec(), "/t", "", 1), "boundaries.max-files: unset")
}

func TestExecutePromptCarriesReviewedCollaboration(t *testing.T) {
	pkt := testPacket()
	ordinary := BuildExecutePrompt(pkt, testSpec(), "/task", "", 1)
	assert.NotContains(t, ordinary, "## Reviewed collaboration tasks")
	pkt.Collaboration = &contract.PacketCollaboration{Version: 1, Tasks: []contract.CollaborationTask{
		{Goal: "Repair the parser", Writes: []string{"src/parser/"}},
		{Goal: "Update independent CLI help", Writes: []string{"src/help/"}},
	}}
	initial := BuildExecutePrompt(pkt, testSpec(), "/task", "", 1)
	retry := BuildExecutePrompt(pkt, testSpec(), "/task", "", 2, "repair the failed parser check")
	for _, fact := range []string{"Repair the parser", "src/parser/", "Update independent CLI help", "src/help/", "every delegated task", "independent Review"} {
		assert.Contains(t, initial, fact)
	}
	assert.Equal(t, strings.Split(initial, "## Runtime")[0], strings.Split(retry, "## Runtime")[0])
	pkt.Collaboration.Tasks[0].Writes = []string{"src/revised-parser/"}
	revised := BuildExecutePrompt(pkt, testSpec(), "/task", "", 2)
	assert.NotEqual(t, strings.Split(initial, "## Runtime")[0], strings.Split(revised, "## Runtime")[0], "a revised scope must change the stable authority supplied to the executor")
}

func TestBuildReviewPromptSections(t *testing.T) {
	pkt := testPacket()
	l1 := map[string]any{"pass": true, "at": "2026-08-22T00:00:00.000Z"}

	base := BuildReviewPrompt(ReviewArgs{Packet: pkt, L1Evidence: l1, DiffSummary: "- src/a.go"})
	assert.Contains(t, base, "## Review rubric (pass line included)\nPass line: score >= 0.8.")
	assert.Contains(t, base, "Monotone convergence & anti-fuzzing (do not move goalposts)")
	assert.Contains(t, base, "~/.codex/memories")
	assert.Contains(t, base, "Complete enumeration (no drip-feeding)")
	assert.Contains(t, base, "Never block on orchestrator metadata")
	assert.Contains(t, base, `Use "verification-gap" ONLY when`)
	assert.Contains(t, base, `"pass": true`)

	withPrior := BuildReviewPrompt(ReviewArgs{
		Packet: pkt, L1Evidence: l1, DiffSummary: "- src/a.go",
		PriorFindings: []ReviewFinding{{ID: "F-1", Severity: "blocker", Target: "a.go:10", Evidence: "missing export", Fix: "export Foo", Status: "open"}},
	})
	assert.Contains(t, withPrior, "## Prior review open findings (verify closure monotonically)")
	assert.Contains(t, withPrior, `"id": "F-1"`)

	execStructured := BuildExecutePrompt(pkt, testSpec(), "/task", "", 2, map[string]any{
		"error-code": "REVIEW_RETURNED",
		"review": map[string]any{
			"findings": []any{
				"skip-non-map",
				map[string]any{"id": "F-0", "severity": "blocker", "status": "resolved", "evidence": "closed"},
				map[string]any{"id": "F-1", "severity": "blocker", "target": "a.go:10", "evidence": "missing Foo", "fix": "export Foo", "status": "open"},
			},
		},
	})
	assert.Contains(t, execStructured, "## Open review findings to close")
	assert.Contains(t, execStructured, "Required fix: export Foo")

	execReasons := BuildExecutePrompt(pkt, testSpec(), "/task", "", 2, map[string]any{
		"error-code": "REVIEW_RETURNED",
		"review":     map[string]any{"reasons": []any{"missing export Bar"}},
	})
	assert.Contains(t, execReasons, "- missing export Bar")
	assert.Equal(t, "", extractReviewFindingsSummary("not-a-map"))
	assert.Equal(t, "", extractReviewFindingsSummary(map[string]any{"error-code": "REVIEW_RETURNED"}))
	assert.Contains(t, base, "## Changed files\n- src/a.go")
	// 无 diffText → 降级说明分支。
	assert.Contains(t, base, "(diff unavailable — judge from the changed-files list only")
	// 无缺陷注入 → 无该节。
	assert.NotContains(t, base, "Defect-injection spot check")

	withDiff := BuildReviewPrompt(ReviewArgs{Packet: pkt, L1Evidence: l1, DiffSummary: "", DiffText: "diff --git a/x b/x"})
	assert.Contains(t, withDiff, "## git diff (full, against the unit base ref)\ndiff --git a/x b/x")
	assert.Contains(t, withDiff, "## Changed files\n(unavailable)")

	withDI := BuildReviewPrompt(ReviewArgs{Packet: pkt, L1Evidence: l1, DiffSummary: "s", DefectInjection: map[string]any{"outcome": "caught"}})
	assert.Contains(t, withDI, "## Defect-injection spot check (orchestrator-run, read-only for you)")
	assert.Contains(t, withDI, `"outcome": "caught"`)
}

func TestBuildPlanPrompt(t *testing.T) {
	st := &state.State{Status: "running", CurrentMilestone: new("M-01")}
	a := PlanArgs{
		Spec:      testSpec(),
		State:     st,
		Milestone: contract.Milestone{ID: "M-01", Title: "Parser core"},
		Packets:   []*contract.Packet{testPacket()},
	}
	out := BuildPlanPrompt(a)
	assert.Contains(t, out, "Goal: Build a parser library")
	assert.Contains(t, out, "Milestones: M-01 Parser core; M-02 CLI")
	assert.Contains(t, out, "status: running, current milestone: M-01")
	assert.Contains(t, out, "Existing packets: M01-T01")
	assert.NotContains(t, out, "Cold start note")

	cold := BuildPlanPrompt(func() PlanArgs { a.ColdStart = true; a.RetroTail = "- retro line"; return a }())
	assert.Contains(t, cold, "## Cold start note")
	assert.Contains(t, cold, "## retro.md (tail)\n- retro line")

	// 冷启动但无 retro → 无 retro 节。
	coldNoRetro := BuildPlanPrompt(func() PlanArgs { a.RetroTail = ""; return a }())
	assert.NotContains(t, coldNoRetro, "retro.md (tail)")

	// current-milestone 为 nil 的渲染。
	st.CurrentMilestone = nil
	assert.Contains(t, BuildPlanPrompt(a), "current milestone: <nil>")
}

func TestBuildJudgePromptCoverageMatrix(t *testing.T) {
	spec := testSpec()
	spec.Requirements = []contract.Requirement{
		{ID: "R-01", Title: "Tokenize", Milestone: "M-01"},
		{ID: "R-02", Title: "Parse", Milestone: "M-01"},
		{ID: "R-03", Title: "Other milestone", Milestone: "M-02"}, // 不进矩阵
	}
	pkt := testPacket() // requirements R-01,R-02
	st := &state.State{Packets: map[string]*state.PacketState{
		"M01-T01": {Evidence: state.Evidence{
			L1:     map[string]any{"pass": true},
			Review: map[string]any{"verdict": "return"}, // 非 pass → Review—
		}},
	}}
	out := BuildJudgePrompt(spec, contract.Milestone{ID: "M-01", Title: "Parser core"}, []*contract.Packet{pkt}, st,
		map[string]any{"L2": []any{map[string]any{"cmd": "gate"}}})
	assert.Contains(t, out, "Milestone: M-01 Parser core")
	assert.NotContains(t, out, "R-03")
	assert.Contains(t, out, "- R-01 Tokenize -> M01-T01(L1✓/Review—)")
	assert.Contains(t, out, "- R-02 Parse -> M01-T01(L1✓/Review—)")
	assert.Contains(t, out, `"cmd": "gate"`)
	assert.Contains(t, out, "~/.codex/memories")
	assert.Contains(t, out, "Closed-world DoD")
	assert.Contains(t, out, "No synthetic adversarial fuzzing")
	assert.Contains(t, out, `"advisory"`)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "project-context.md"), []byte("# Delegated\nKeep helper scripts minimal.\n"), 0o644))
	withOpts := BuildJudgePrompt(spec, contract.Milestone{ID: "M-01", Title: "Parser core"}, []*contract.Packet{pkt}, st, nil, JudgeOptions{
		TaskDir:       dir,
		HumanGuidance: "Do not reject on synthetic preload imagesrcset edge cases.",
	})
	assert.Contains(t, withOpts, "## Project context memory (stable)")
	assert.Contains(t, withOpts, "Keep helper scripts minimal.")
	assert.Contains(t, withOpts, "## Human triage guidance (authoritative scope decision)")
	assert.Contains(t, withOpts, "Do not reject on synthetic preload imagesrcset edge cases.")

	reviewWithMem := BuildReviewPrompt(ReviewArgs{Packet: pkt, TaskDir: dir, L1Evidence: map[string]any{"pass": true}, DiffSummary: "- a.go"})
	assert.Contains(t, reviewWithMem, "## Project context memory (stable)")
	assert.Contains(t, reviewWithMem, "Keep helper scripts minimal.")

	// 无证据对象 → 双破折号。
	st.Packets["M01-T01"].Evidence = state.Evidence{}
	out = BuildJudgePrompt(spec, contract.Milestone{ID: "M-01", Title: "X"}, []*contract.Packet{pkt}, st, nil)
	assert.Contains(t, out, "M01-T01(L1—/Review—)")
	assert.Contains(t, out, "{}") // gateLevels nil → {}

	// 完全无覆盖 → NO COVERAGE。
	out = BuildJudgePrompt(spec, contract.Milestone{ID: "M-01", Title: "X"}, nil, st, nil)
	assert.Contains(t, out, "-> NO COVERAGE")
}

func TestParseReviewVerdict(t *testing.T) {
	v, err := ParseReviewVerdict("some prose\n{\"verdict\":\"pass\",\"score\":0.9,\"reasons\":[\"ok\"]}")
	require.NoError(t, err)
	assert.Equal(t, "pass", v.Verdict)
	require.NotNil(t, v.Score)
	assert.InDelta(t, 0.9, *v.Score, 1e-9)
	assert.Equal(t, []string{"ok"}, v.Reasons)

	// 围栏包裹 + 前置噪声，从尾向前扫描命中最后一行。
	v, err = ParseReviewVerdict("```json\n{\"verdict\":\"verification-gap\",\"score\":0.5,\"reasons\":[\"acceptance too weak\"]}\n```\nnoise above")
	require.NoError(t, err)
	assert.Equal(t, "verification-gap", v.Verdict)
	assert.InDelta(t, 0.5, *v.Score, 1e-9)
	assert.Equal(t, []string{"acceptance too weak"}, v.Reasons)

	// 非法 verdict 行被跳过，继续向上找到合法行。
	v, err = ParseReviewVerdict("{\"verdict\":\"bogus\",\"score\":1,\"reasons\":[\"x\"]}\n{\"verdict\":\"pass\",\"score\":0.9,\"reasons\":[\"cmd exit 0\"]}")
	require.NoError(t, err)
	assert.Equal(t, "pass", v.Verdict)

	// A3 出口契约强化：缺 score 或空 reasons = 无证据支撑 → 跳过该行继续扫，
	// 全部不合法则报 no verdict JSON found。
	_, err = ParseReviewVerdict(`{"verdict":"return"}`)
	assert.EqualError(t, err, "no verdict JSON found in review output")
	_, err = ParseReviewVerdict(`{"verdict":"pass","score":1,"reasons":[]}`)
	assert.EqualError(t, err, "no verdict JSON found in review output")
	// 无证据行之下还有合法行 → 命中合法行（自尾向前扫描语义保持）。
	v, err = ParseReviewVerdict("{\"verdict\":\"pass\",\"score\":0.8,\"reasons\":[\"cmd pnpm test exit 0\"]}\n{\"verdict\":\"pass\",\"score\":1}")
	require.NoError(t, err)
	assert.InDelta(t, 0.8, *v.Score, 1e-9)

	// score 越界（>1）= 契约违约 → 跳过。
	_, err = ParseReviewVerdict(`{"verdict":"pass","score":1.5,"reasons":["x"]}`)
	assert.EqualError(t, err, "no verdict JSON found in review output")

	// 空 / 无 JSON / 全非法。
	_, err = ParseReviewVerdict("")
	assert.EqualError(t, err, "empty review output")
	_, err = ParseReviewVerdict("no json at all")
	assert.EqualError(t, err, "no verdict JSON found in review output")
	_, err = ParseReviewVerdict("{broken json\nalso no verdict}")
	assert.EqualError(t, err, "no verdict JSON found in review output")
}

func TestBuildJudgePromptPartialCoverage(t *testing.T) {
	// packet 不覆盖该需求 → skip 分支；唯一覆盖者为无状态条目。
	spec := testSpec()
	spec.Requirements = []contract.Requirement{{ID: "R-09", Title: "Unrelated", Milestone: "M-01"}}
	other := testPacket() // requirements R-01,R-02，不含 R-09
	st := &state.State{Packets: map[string]*state.PacketState{}}
	out := BuildJudgePrompt(spec, contract.Milestone{ID: "M-01", Title: "X"}, []*contract.Packet{other}, st, nil)
	assert.Contains(t, out, "-> NO COVERAGE")
}

func TestParseReviewVerdictSkipsInvalidVerdictValue(t *testing.T) {
	_, err := ParseReviewVerdict(`{"verdict":"bogus","score":1,"reasons":["x"]}`)
	assert.EqualError(t, err, "no verdict JSON found in review output")
}

func TestParseReviewVerdictSkipsBlankLines(t *testing.T) {
	// 尾部空行 + 中间空行：空行 continue 分支。
	v, err := ParseReviewVerdict("\n\n{\"verdict\":\"pass\",\"score\":1,\"reasons\":[\"e\"]}\n\n")
	require.NoError(t, err)
	assert.Equal(t, "pass", v.Verdict)
}

// TestBuildPlanPromptFeedback v0.24 plan 子环的校验反馈通道：上一轮坏契约的
// 错误必须出现在提示词（volatile 段），驱动 planner 修正自己的产物。
func TestBuildPlanPromptFeedback(t *testing.T) {
	a := PlanArgs{
		Spec:      &contract.Spec{Title: "t", Goal: "g"},
		State:     &state.State{Status: "running"},
		Milestone: contract.Milestone{ID: "M-01", Title: "one"},
		Feedback:  "packet M01-T03: frontmatter parse error at line 1",
	}
	out := BuildPlanPrompt(a)
	assert.Contains(t, out, "Contract validation feedback")
	assert.Contains(t, out, "frontmatter parse error")
	assert.Contains(t, out, "REJECTED")
}

func TestExecutePromptMandatesRegressionTestOnReviewReturnAndSupportsTieredVerdict(t *testing.T) {
	pkt := testPacket()
	l1Retry := BuildExecutePrompt(pkt, testSpec(), "/task", "", 2, map[string]any{"error-code": "L1_FAILED"})
	assert.NotContains(t, l1Retry, "## Regression / guard test requirement")

	revRetry := BuildExecutePrompt(pkt, testSpec(), "/task", "", 2, map[string]any{"error-code": "REVIEW_RETURNED", "review": map[string]any{"verdict": "return"}})
	assert.Contains(t, revRetry, "## Regression / guard test requirement")
	assert.Contains(t, revRetry, "targeted regression or guard test")

	reviewPrompt := BuildReviewPrompt(ReviewArgs{Packet: pkt, L1Evidence: map[string]any{"pass": true}})
	assert.Contains(t, reviewPrompt, "Severity tiering:")
	v, err := ParseReviewVerdict("```json\n{\"verdict\":\"pass\",\"score\":0.95,\"reasons\":[\"cmd exit 0\"],\"advisory\":[\"rename helper\"]}\n```")
	require.NoError(t, err)
	assert.Equal(t, []string{"rename helper"}, v.Advisory)
}

func TestFrontendUXReviewPromptAndParser(t *testing.T) {
	pkt := testPacket()
	promptEmpty := BuildFrontendUXReviewPrompt(pkt, "", "")
	assert.Contains(t, promptEmpty, "(unavailable)")
	assert.Contains(t, promptEmpty, "(diff unavailable)")
	promptFull := BuildFrontendUXReviewPrompt(pkt, "- src/view.tsx", "+ <button>Save</button>")
	assert.Contains(t, promptFull, "+ <button>Save</button>")

	_, err := ParseFrontendUXReview("")
	assert.EqualError(t, err, "empty frontend UX review output")

	for _, bad := range []string{
		"no json",
		`{"verdict":"unknown","summary":"bad","findings":[]}`,
		`{"verdict":"pass","summary":" ","findings":[]}`,
		`{"verdict":"pass","summary":"ok"}`,
		`{"verdict":"revise","summary":"bad","findings":[{"severity":"minor","issue":"x"}]}`,
		`{"verdict":"revise","summary":"bad","findings":[{"severity":"blocking","issue":""}]}`,
	} {
		_, err = ParseFrontendUXReview(bad)
		require.Error(t, err, bad)
	}

	ux, err := ParseFrontendUXReview("Audit complete\n```json\n{\"verdict\":\"revise\",\"summary\":\"Disabled button hides error\",\"findings\":[{\"severity\":\"blocking\",\"location\":\"src/view.tsx:12\",\"issue\":\"No error state\",\"suggestion\":\"Render inline alert\"},{\"severity\":\"advisory\",\"issue\":\"Tighten spacing\"}]}\n```")
	require.NoError(t, err)
	assert.Equal(t, "revise", ux.Verdict)
	require.Len(t, ux.Findings, 2)
	assert.Equal(t, "blocking", ux.Findings[0].Severity)
	assert.Equal(t, "advisory", ux.Findings[1].Severity)

	hugeDiff := strings.Repeat("+line\n", 5000)
	boundedUX := BuildFrontendUXReviewPrompt(pkt, "- src/view.tsx", hugeDiff)
	assert.Contains(t, boundedUX, "diff truncated at 24000 bytes")
	assert.Less(t, strings.Index(boundedUX, "## Output"), strings.Index(boundedUX, "## git diff"))
}

func TestBoundTextProjectContextAndReviewThreeLayerOrdering(t *testing.T) {
	assert.Equal(t, "short", boundText("short", 10, "..."))
	assert.Equal(t, "line1...", boundText("line1\nline2\nline3", 10, "..."))
	assert.Equal(t, "abcdefghij...", boundText("abcdefghijklmnop", 10, "..."))

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "planning"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "planning", "project-context.md"), []byte("# Stack\nGo 1.27\n"), 0o644))
	pkt := testPacket()
	execPrompt := BuildExecutePrompt(pkt, testSpec(), dir, "", 1, map[string]any{"output": strings.Repeat("err\n", 5000)})
	assert.Contains(t, execPrompt, "## Project context memory (stable)\n# Stack\nGo 1.27")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "project-context.md"), []byte("# Stack\nGo Primary\n"), 0o644))
	assert.Contains(t, BuildExecutePrompt(pkt, testSpec(), dir, "", 1), "# Stack\nGo Primary")
	assert.Contains(t, execPrompt, "feedback truncated at 16000 bytes")
	assert.Less(t, strings.Index(execPrompt, "## Project context memory"), strings.Index(execPrompt, "## Runtime (volatile)"))

	revPrompt := BuildReviewPrompt(ReviewArgs{
		Packet:      pkt,
		L1Evidence:  map[string]any{"pass": true},
		DiffSummary: "- src/a.go",
		DiffText:    strings.Repeat("+code line\n", 4000),
	})
	assert.Contains(t, revPrompt, "diff truncated at 24000 bytes")
	assert.Less(t, strings.Index(revPrompt, "## Output"), strings.Index(revPrompt, "## Task packet (contract)"))
	assert.Less(t, strings.Index(revPrompt, "## Task packet (contract)"), strings.Index(revPrompt, "## L1 deterministic test results"))
}
