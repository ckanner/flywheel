package machine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV044Pillar1ExecuteSelfWritePreservesIncrementalAndReviewIncremental(t *testing.T) {
	t.Run("execute keeps incremental when editing own schedule.writes but invalidates on base-ref or external context change", func(t *testing.T) {
		fx, c := sessionFixture(t)
		c.Config.Sessions.Context = "verified-incremental"
		p := c.Packets["M01-T01"]
		p.Schedule = &contract.PacketSchedule{Writes: []string{"src/app/bootstrap.ts", "src/ui/", " ", "."}}
		p.ContextFiles = []string{"src/app/bootstrap.ts", "src/ui/panel.ts", "src/core/types.ts"}
		p.CacheScope = []string{"src/core/types.ts"}
		writeFile(t, filepath.Join(fx.root, "src/app/bootstrap.ts"), "v1-bootstrap")
		writeFile(t, filepath.Join(fx.root, "src/ui/panel.ts"), "v1-panel")
		writeFile(t, filepath.Join(fx.root, "src/core/types.ts"), "stable-core-types")
		c.State.Packets[p.PacketID].BaseRef = new("base-sha-1")

		a := sessionArgs(c)
		a.ContextTemplateVersion = executeContextVersion
		a.Prompt = "FULL EXECUTE PROMPT"
		var seen []string
		call := func(args drivers.CallArgs) drivers.Result {
			seen = append(seen, args.Prompt)
			return sessionResult(args, drivers.MockResult(nil))
		}

		// Turn 1: full context.
		first, err := c.invokeScoped("execute", p.PacketID, a, call)
		require.NoError(t, err)
		applySessionResult(t, c, "execute", p.PacketID, first)
		require.Equal(t, "full", c.sessionRecord("execute", p.PacketID).Context.Mode)

		// Execute edits its own declared write targets (exact file + prefix dir).
		writeFile(t, filepath.Join(fx.root, "src/app/bootstrap.ts"), "v2-bootstrap-edited-by-execute")
		writeFile(t, filepath.Join(fx.root, "src/ui/panel.ts"), "v2-panel-edited-by-execute")

		// Turn 2: MUST stay incremental because only schedule.writes changed.
		second, err := c.invokeScoped("execute", p.PacketID, a, call)
		require.NoError(t, err)
		applySessionResult(t, c, "execute", p.PacketID, second)
		rec := c.sessionRecord("execute", p.PacketID)
		require.Equal(t, "incremental", rec.Context.Mode)
		require.Contains(t, seen[1], "Verified context update")
		require.NotContains(t, seen[1], "FULL EXECUTE PROMPT")

		// Turn 3: base-ref changes (e.g. rebase/merge) -> MUST invalidate back to full!
		c.State.Packets[p.PacketID].BaseRef = new("base-sha-2")
		third, err := c.invokeScoped("execute", p.PacketID, a, call)
		require.NoError(t, err)
		applySessionResult(t, c, "execute", p.PacketID, third)
		require.Equal(t, "full", c.sessionRecord("execute", p.PacketID).Context.Mode)
		require.Contains(t, seen[2], "FULL EXECUTE PROMPT")

		// Turn 4: external read-only context file changes -> MUST invalidate back to full!
		writeFile(t, filepath.Join(fx.root, "src/core/types.ts"), "external-types-changed")
		fourth, err := c.invokeScoped("execute", p.PacketID, a, call)
		require.NoError(t, err)
		applySessionResult(t, c, "execute", p.PacketID, fourth)
		require.Equal(t, "full", c.sessionRecord("execute", p.PacketID).Context.Mode)
	})

	t.Run("review verified-incremental omits Layer 1/2 on stable resume and falls back safely", func(t *testing.T) {
		fx, c := sessionFixture(t)
		c.Config.Sessions.Context = "verified-incremental"
		p := c.Packets["M01-T01"]
		writeFile(t, filepath.Join(fx.taskDir, "project-context.md"), "# Stable Project Context\n- M-01")

		a := sessionArgs(c)
		a.ContextTemplateVersion = reviewContextVersion
		a.Prompt = prompts.BuildReviewPrompt(prompts.ReviewArgs{
			Packet:      p,
			TaskDir:     fx.taskDir,
			L1Evidence:  map[string]any{"pass": true},
			DiffSummary: "- src/a.ts",
			DiffText:    "diff --git a/src/a.ts b/src/a.ts\n+round1",
		})
		var seen []string
		call := func(args drivers.CallArgs) drivers.Result {
			seen = append(seen, args.Prompt)
			return sessionResult(args, drivers.MockResult(nil))
		}

		// Review #1: full prompt including Layer 1 & Layer 2.
		first, err := c.invokeScoped("review", p.PacketID, a, call)
		require.NoError(t, err)
		applySessionResult(t, c, "review", p.PacketID, first)
		rec := c.sessionRecord("review", p.PacketID)
		require.Equal(t, "full", rec.Context.Mode)
		require.Contains(t, seen[0], "## Review rubric (pass line included)")
		require.Contains(t, seen[0], "+round1")

		// Review #2: same stable contract & memory -> incremental Layer 3 only!
		a.Prompt = prompts.BuildReviewPrompt(prompts.ReviewArgs{
			Packet:      p,
			TaskDir:     fx.taskDir,
			L1Evidence:  map[string]any{"pass": true},
			DiffSummary: "- src/a.ts",
			DiffText:    "diff --git a/src/a.ts b/src/a.ts\n+round2-fix",
			PriorFindings: []prompts.ReviewFinding{
				{ID: "F-1", Severity: "major", Target: "src/a.ts:1", Evidence: "e", Status: "open"},
			},
		})
		second, err := c.invokeScoped("review", p.PacketID, a, call)
		require.NoError(t, err)
		applySessionResult(t, c, "review", p.PacketID, second)
		rec = c.sessionRecord("review", p.PacketID)
		require.Equal(t, "incremental", rec.Context.Mode)
		require.Contains(t, seen[1], "+round2-fix")
		require.Contains(t, seen[1], "Prior review open findings")
		require.NotContains(t, seen[1], "## Review rubric (pass line included)")
		require.Less(t, len(seen[1]), len(seen[0]))

		// Fallback: missing packet, missing Layer 3 heading, changed humanGuidance, and changed project-context.md.
		cpMiss, outMiss := c.sessionContextPrompt("review", "missing-pkt", "resume", "cand", rec, a)
		require.Equal(t, "full", cpMiss.Mode)
		require.Contains(t, cpMiss.Reason, "unavailable")
		require.Equal(t, a.Prompt, outMiss)

		noMarker := a
		noMarker.Prompt = "REVIEW PROMPT WITHOUT LAYER 3 HEADING"
		cpNoMarker, outNoMarker := c.sessionContextPrompt("review", p.PacketID, "resume", "cand", rec, noMarker)
		require.Equal(t, "full", cpNoMarker.Mode)
		require.Equal(t, noMarker.Prompt, outNoMarker)

		c.State.Packets[p.PacketID].HumanGuidance = "scope decision updated"
		cpGuide, outGuide := c.sessionContextPrompt("review", p.PacketID, "resume", "cand", rec, a)
		require.Equal(t, "full", cpGuide.Mode)
		require.Equal(t, a.Prompt, outGuide)
		c.State.Packets[p.PacketID].HumanGuidance = ""

		// Missing project-context.md exercises empty-hash branch in fileContentHash.
		require.NoError(t, os.Remove(filepath.Join(fx.taskDir, "project-context.md")))
		cpNoPC, _ := c.sessionContextPrompt("review", p.PacketID, "resume", "cand", rec, a)
		require.Equal(t, "full", cpNoPC.Mode)
	})
}

func TestV044Pillar2ConvergenceDrivenExecutionAndL1Decoupling(t *testing.T) {
	// Reproduces M05-T01:
	// Attempt 1: Execute -> L1 pass -> Review #1 returns 5 open findings (F-1..F-5)
	// Attempt 2: Execute -> L1 FAILS (consumes 1 attempt slot without reaching review)
	// Attempt 3: Execute -> L1 pass -> Review #2 returns 3 open findings (5 -> 3 monotonically converging!)
	// Under the unified convergence kernel, Attempt 3 is granted bounded convergence continuation -> Attempt 4 passes!
	fx := newGitFixture(t)
	sd := happyDrivers()
	packetText := strings.Replace(
		fixturePacket("M01-T01", "R-01"),
		`    - {cmd: "test -f build/M01-T01.txt", expect: "exit 0"}`,
		"    - {cmd: \"test -f build/M01-T01.txt\", expect: \"exit 0\"}\n"+
			"    - {cmd: \"test -f build/l1-ok\", expect: \"exit 0\"}", 1)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), packetText)

	m01Exec := 0
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		pid := "M01-T01"
		if strings.Contains(a.Prompt, "packet-id: M01-T02") {
			pid = "M01-T02"
		}
		if pid == "M01-T01" {
			m01Exec++
			writeFile(t, filepath.Join(a.Cwd, "build", "M01-T01.txt"), strings.Repeat("c", m01Exec))
			if m01Exec == 2 {
				_ = os.Remove(filepath.Join(a.Cwd, "build", "l1-ok"))
			} else {
				writeFile(t, filepath.Join(a.Cwd, "build", "l1-ok"), "ok")
			}
			writeDoneReportFile(t, a.TaskDir, pid)
			return drivers.MockResult(nil)
		}
		return sd.mock.Execute(a)
	}
	m01Rev := 0
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		if strings.Contains(a.Prompt, "M01-T01") {
			m01Rev++
			switch m01Rev {
			case 1:
				return textResult(`{"verdict":"return","score":0.64,"reasons":["5 open"],"findings":[` +
					`{"id":"F-1","severity":"blocker","evidence":"e1","status":"open"},` +
					`{"id":"F-2","severity":"major","evidence":"e2","status":"open"},` +
					`{"id":"F-3","severity":"major","evidence":"e3","status":"open"},` +
					`{"id":"F-4","severity":"major","evidence":"e4","status":"open"},` +
					`{"id":"F-5","severity":"major","evidence":"e5","status":"open"}]}`)
			case 2:
				return textResult(`{"verdict":"return","score":0.79,"reasons":["5 -> 3 converging"],"findings":[` +
					`{"id":"F-1","severity":"blocker","evidence":"e1","status":"open"},` +
					`{"id":"F-2","severity":"major","evidence":"e2","status":"resolved","resolution":"fixed"},` +
					`{"id":"F-3","severity":"major","evidence":"e3","status":"resolved","resolution":"fixed"},` +
					`{"id":"F-4","severity":"major","evidence":"e4","status":"open"},` +
					`{"id":"F-5","severity":"major","evidence":"e5","status":"open"}]}`)
			default:
				return passVerdict()
			}
		}
		return passVerdict()
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "completed", ps.Status)
	assert.Equal(t, 4, m01Exec, "converging 5 -> (L1 fail) -> 3 must continue automatically and pass on attempt 4")
	assert.Equal(t, 3, m01Rev)
	assert.Equal(t, 1, executionGraceCount(ps))
	assert.Equal(t, 1, priorNonReviewFailureCount(ps))

	// Cover helper edge cases for 100% branch coverage:
	// non-map entries, reset boundaries, non-return verdicts, invalid findings JSON, nil findings, state.Evidence vs map, tail slicing.
	edgePS := &state.PacketState{
		RetryHistory: []any{
			"non-map",
			map[string]any{"scope": "attempt", "policy": "requeue", "error-code": "L1_FAILED"},
			map[string]any{"policy": "convergence-grace"},
			map[string]any{"policy": "human-triage"},
			"non-map-after",
			map[string]any{"scope": "attempt", "policy": "requeue", "error-code": "L1_FAILED"},
			map[string]any{"scope": "attempt", "policy": "requeue", "error-code": "REVIEW_RETURNED"},
			map[string]any{"scope": "attempt", "policy": "convergence-grace", "error-code": "REVIEW_RETURNED"},
		},
		EvidenceHistory: []any{
			"non-map",
			map[string]any{"evidence": state.Evidence{Review: map[string]any{"verdict": "return", "findings": nil}}},
			map[string]any{"evidence": map[string]any{"review": map[string]any{"verdict": "return", "findings": "bad-json-shape"}}},
			map[string]any{"evidence": map[string]any{"review": map[string]any{"verdict": "pass"}}},
		},
		Evidence: state.Evidence{
			Review: map[string]any{
				"verdict": "return",
				"findings": []any{
					map[string]any{"id": "B-1", "severity": "blocker", "status": "open"},
					map[string]any{"id": "M-1", "severity": "major", "status": "open"},
					map[string]any{"id": "N-1", "severity": "minor", "status": "open"},
					map[string]any{"id": "R-1", "severity": "major", "status": "resolved"},
				},
			},
		},
	}
	assert.Equal(t, 1, executionGraceCount(edgePS))
	assert.True(t, gracePolicyUsed(edgePS, "convergence-grace"))
	assert.False(t, gracePolicyUsed(edgePS, "first-review-grace"))
	assert.Equal(t, 1, priorNonReviewFailureCount(edgePS))
	assert.Equal(t, 1, reviewReturnCount(edgePS))
	hist := executionConvergenceHistory(edgePS)
	require.Len(t, hist, 1, "tail slice bounds executionConvergenceHistory to reviewReturnCount")
	assert.Equal(t, 1, hist[0].OpenBlockers)
	assert.Equal(t, 1, hist[0].OpenMajors)
	assert.Equal(t, []string{"B-1", "M-1"}, hist[0].OpenIDs)
	assert.Equal(t, []string{"R-1"}, hist[0].ResolvedIDs)

	// Replay real Pixel Tetris convergence histories through the unified kernel:
	// - M03-T01 (8 -> 6 -> 4): strictly decreasing -> continue!
	m03t01 := EvaluateConvergence(ConvergenceInput{
		History: []ConvergenceRound{
			{OpenBlockers: 2, OpenMajors: 6, OpenIDs: []string{"F-1", "F-2", "F-3", "F-4", "F-5", "F-6", "F-7", "F-8"}},
			{OpenBlockers: 1, OpenMajors: 5, OpenIDs: []string{"F-1", "F-2", "F-5", "F-6", "F-7", "F-8"}},
			{OpenBlockers: 0, OpenMajors: 4, OpenIDs: []string{"F-2", "F-6", "F-7", "F-8"}},
		},
		MaxGrace: MaxExecutionGraceRounds,
	})
	assert.True(t, m03t01.AllowGrace, "M03-T01 8->6->4 must converge-continue")

	// - M02-T01 (6 -> 2 -> 2): stalled at 2 -> circuit-break immediately!
	m02t01 := EvaluateConvergence(ConvergenceInput{
		History: []ConvergenceRound{
			{OpenMajors: 6, OpenIDs: []string{"F-1", "F-2", "F-3", "F-4", "F-5", "F-6"}},
			{OpenMajors: 2, OpenIDs: []string{"F-7", "F-8"}},
			{OpenMajors: 2, OpenIDs: []string{"F-7", "F-8"}},
		},
		MaxGrace: MaxExecutionGraceRounds,
	})
	assert.False(t, m02t01.AllowGrace, "M02-T01 6->2->2 stall must circuit-break")
	assert.True(t, m02t01.Stalled, "M02-T01 6->2->2 must be classified stalled")

	// - M01-T03 (7 -> 4 -> [F-4] -> [F-8]): all prior IDs cleared with 1 remaining -> Case B continue!
	m01t03 := EvaluateConvergence(ConvergenceInput{
		History: []ConvergenceRound{
			{OpenMajors: 7, OpenIDs: []string{"F-1", "F-2", "F-3", "F-4", "F-5", "F-6", "F-7"}},
			{OpenMajors: 4, OpenIDs: []string{"F-1", "F-3", "F-4", "F-5"}},
			{OpenMajors: 1, OpenIDs: []string{"F-4"}},
			{OpenMajors: 1, OpenIDs: []string{"F-8"}},
		},
		MaxGrace: MaxExecutionGraceRounds,
	})
	assert.True(t, m01t03.AllowGrace, "M01-T03 [F-4]->[F-8] all prior cleared must Case-B continue")

	// - M02-T03 (2 prior L1 failures -> 1st review returns 10 open): first-review-grace continue!
	m02t03 := EvaluateConvergence(ConvergenceInput{
		History: []ConvergenceRound{
			{OpenMajors: 10, OpenIDs: []string{"F-1", "F-2", "F-3", "F-4", "F-5", "F-6", "F-7", "F-8", "F-9", "F-10"}},
		},
		PriorNonReviewFailures: 2,
		MaxGrace:               MaxExecutionGraceRounds,
	})
	assert.True(t, m02t03.AllowGrace, "M02-T03 first review after L1 failures must grant first-review-grace")
	assert.Equal(t, "first-review-grace", m02t03.Policy)
}

func TestV044Pillar3SharedReadOnlySandboxDisciplineAcrossAllEvaluators(t *testing.T) {
	pkt := &contract.Packet{PacketID: "M01-T01", Goal: "g", Requirements: []string{"R-01"}, ReviewRubric: "r"}
	spec := &contract.Spec{Goal: "g", Requirements: []contract.Requirement{{ID: "R-01", Title: "t", Milestone: "M-01"}}}
	ms := contract.Milestone{ID: "M-01", Title: "m"}
	st := &state.State{Packets: map[string]*state.PacketState{}}

	revPrompt := prompts.BuildReviewPrompt(prompts.ReviewArgs{Packet: pkt, L1Evidence: map[string]any{"pass": true}})
	judgePrompt := prompts.BuildJudgePrompt(spec, ms, []*contract.Packet{pkt}, st, map[string]any{})
	planRevPrompt := prompts.BuildPlanningPrompt(&planning.State{
		Brief: planning.Brief{Goal: "g"},
	}, planning.Request{ID: "req-1", Scope: "design"}, true, nil)

	for name, body := range map[string]string{
		"review":      revPrompt,
		"judge":       judgePrompt,
		"plan-review": planRevPrompt,
	} {
		assert.Contains(t, body, ".vite-temp", "%s prompt must include shared readOnlySandboxDiscipline", name)
		assert.Contains(t, body, "python3 -B - <<'PY'", "%s prompt must include shared heredoc batch inspection discipline", name)
	}
}
