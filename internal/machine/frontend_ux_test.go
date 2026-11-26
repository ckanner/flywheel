package machine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func frontendFixture(t *testing.T) (fixture, *scriptedDrivers, RunTaskOpts) {
	t.Helper()
	fx := newGitFixture(t)
	pktPath := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
	raw, err := os.ReadFile(pktPath)
	require.NoError(t, err)
	writeFile(t, pktPath, strings.Replace(string(raw), "route: default", "route: frontend", 1))
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-qm", "test: mark M01-T01 as frontend route")
	sd := happyDrivers()
	opts := runOpts(fx, sd.set())
	opts.Config.Profiles = map[string]config.ModelProfile{
		"neon": {Harness: "antigravity", Model: "gemini-4-neon-high", Channel: "kimi-subscription", Thinking: "high"},
	}
	opts.Config.FrontendUXReview = config.FrontendUXReviewConfig{Enabled: true, Profile: "neon"}
	return fx, sd, opts
}

func TestFrontendUXAuditRefinesBlockingOnceThenPassesToCodexReview(t *testing.T) {
	fx, sd, opts := frontendFixture(t)
	uxCalls := 0
	codeReviewCalls := 0
	execCalls := 0
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		execCalls++
		if execCalls == 2 {
			require.Contains(t, a.Prompt, "frontend-ux-review")
		}
		return sd.mock.Execute(a)
	}
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
			uxCalls++
			require.Equal(t, "read-only", a.PermissionMode)
			return drivers.MockResult(func(r *drivers.Result) {
				r.Text = `{"verdict":"revise","summary":"Button label unclear","findings":[{"severity":"blocking","location":"ui.tsx:10","issue":"Ambiguous CTA","suggestion":"Use Approve & Start"},{"severity":"advisory","issue":"Spacing"}]}`
			})
		}
		codeReviewCalls++
		return passVerdict()
	}

	opts.Drivers = sd.set()
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.Equal(t, 1, uxCalls, "UX audit runs once and caps refinement at 1 pass")
	require.Equal(t, 2, codeReviewCalls, "both M01-T01 and M01-T02 receive Codex review")
	raw, err := os.ReadFile(filepath.Join(fx.taskDir, "reports", "M01-T01.ux-review.json"))
	require.NoError(t, err)
	var rec uxReviewRecord
	require.NoError(t, json.Unmarshal(raw, &rec))
	require.True(t, rec.Refined)
	require.Equal(t, "neon", rec.Profile)
}

func TestFrontendUXAuditAdvisoryOnlyAndEdgeBranches(t *testing.T) {
	t.Run("advisory only proceeds directly to code review", func(t *testing.T) {
		fx, sd, opts := frontendFixture(t)
		execCalls := 0
		sd.execFn = func(a drivers.CallArgs) drivers.Result {
			if a.PacketID == "M01-T01" {
				execCalls++
			}
			return sd.mock.Execute(a)
		}
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) {
					r.Text = `{"verdict":"pass","summary":"Clean UI","findings":[{"severity":"advisory","issue":"Minor spacing"}]}`
				})
			}
			return passVerdict()
		}
		opts.Drivers = sd.set()
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "completed", summary.Status)
		require.Equal(t, 1, execCalls)
		require.FileExists(t, filepath.Join(fx.taskDir, "reports", "M01-T01.ux-review.json"))
	})

	t.Run("unparseable UX output logs and continues", func(t *testing.T) {
		_, sd, opts := frontendFixture(t)
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) { r.Text = "not json" })
			}
			return passVerdict()
		}
		opts.Drivers = sd.set()
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "completed", summary.Status)
	})

	t.Run("cancelled, budget stop, missing profile, cleanup error, and write failures", func(t *testing.T) {
		fx, sd, opts := frontendFixture(t)
		writeFile(t, filepath.Join(fx.taskDir, "reports", "M01-T01.ux-review.json"), "{bad json")
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) { r.Cancelled = true })
			}
			return passVerdict()
		}
		opts.Drivers = sd.set()
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "running", summary.Status)

		_, sd, opts = frontendFixture(t)
		opts.Config.Guardrails.MaxTaskCalls = new(1)
		cBudget, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Drivers: sd.set(), Log: func(string) {}})
		require.NoError(t, err)
		cBudget.State.Budget.Totals.Calls = 1
		_, uxStopped, err := cBudget.frontendUXAuditStep(cBudget.Packets["M01-T01"])
		require.NoError(t, err)
		require.True(t, uxStopped)

		_, sd, opts = frontendFixture(t)
		opts.Config.Guardrails.MaxTaskCalls = new(1)
		opts.Drivers = sd.set()
		summary, err = RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "budget-stopped", summary.Status)

		_, sd, opts = frontendFixture(t)
		opts.Config.FrontendUXReview.Profile = "missing"
		c, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Drivers: sd.set(), Log: func(string) {}})
		require.NoError(t, err)
		_, _, err = c.frontendUXAuditStep(c.Packets["M01-T01"])
		require.ErrorContains(t, err, "not defined")

		_, sd, opts = frontendFixture(t)
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) { r.CleanupError = "ux orphan" })
			}
			return passVerdict()
		}
		opts.Drivers = sd.set()
		_, err = RunTask(opts)
		require.ErrorContains(t, err, "ux orphan")

		_, sd, opts = frontendFixture(t)
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) {
					r.Text = `{"verdict":"pass","summary":"Clean","findings":[]}`
				})
			}
			return passVerdict()
		}
		origMkdir := mkdirUXReviewDir
		mkdirUXReviewDir = func(string, os.FileMode) error { return errors.New("ux mkdir boom") }
		t.Cleanup(func() { mkdirUXReviewDir = origMkdir })
		opts.Drivers = sd.set()
		_, err = RunTask(opts)
		require.ErrorContains(t, err, "ux mkdir boom")
		mkdirUXReviewDir = origMkdir

		_, sd, opts = frontendFixture(t)
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) {
					r.Text = `{"verdict":"pass","summary":"Clean","findings":[]}`
				})
			}
			return passVerdict()
		}
		origWrite := writeUXReviewFile
		writeUXReviewFile = func(string, []byte, os.FileMode) error { return errors.New("ux write boom") }
		t.Cleanup(func() { writeUXReviewFile = origWrite })
		opts.Drivers = sd.set()
		_, err = RunTask(opts)
		require.ErrorContains(t, err, "ux write boom")
		writeUXReviewFile = origWrite
	})
}

func TestRunGuardrailsEnforceMaxPacketAttemptsMaxReviewRoundsAndMaxTaskCalls(t *testing.T) {
	t.Run("max-packet-attempts caps higher packet max-retries", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		execCalls := 0
		sd.execFn = func(a drivers.CallArgs) drivers.Result {
			execCalls++
			return drivers.MockResult(func(r *drivers.Result) {
				code := 1
				r.ExitCode = &code
				msg := "error-" + string(rune('a'+execCalls))
				r.ErrorMessage = &msg
				r.Settled = false
			})
		}
		opts := runOpts(fx, sd.set())
		opts.Config.Guardrails.MaxPacketAttempts = new(1)
		opts.Drivers = sd.set()
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "escalated", summary.Status)
		st := loadFixtureState(t, fx)
		require.Equal(t, 1, st.Packets["M01-T01"].FailedAttempts)
	})

	t.Run("max-review-rounds escalates stagnating review returns straight to inbox", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		reviewCalls := 0
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			reviewCalls++
			return drivers.MockResult(func(r *drivers.Result) {
				r.Text = `{"verdict":"return","score":0.4,"reasons":["defect round ` + string(rune('0'+reviewCalls)) + `"]}`
			})
		}
		opts := runOpts(fx, sd.set())
		opts.Config.Guardrails.MaxPacketAttempts = new(5)
		opts.Config.Guardrails.MaxReviewRounds = new(2)
		opts.Drivers = sd.set()
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "escalated", summary.Status)
		require.Equal(t, 2, reviewCalls, "stagnating REVIEW_RETURNED escalates directly to inbox without rewriting the milestone plan")
		st := loadFixtureState(t, fx)
		require.Equal(t, 0, st.Packets["M01-T01"].PlanRevisions)
		require.Equal(t, "inbox", st.Packets["M01-T01"].Status)
	})

	t.Run("converging review findings continue past max-review-rounds and pass", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		reviewCalls := 0
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			reviewCalls++
			switch reviewCalls {
			case 1:
				return drivers.MockResult(func(r *drivers.Result) {
					r.Text = `{"verdict":"return","score":0.6,"reasons":["missing A and B"],"findings":[{"id":"F-1","severity":"blocker","target":"a.ts:1","evidence":"missing A","fix":"export A","status":"open"},{"id":"F-2","severity":"major","target":"b.ts:1","evidence":"missing B","fix":"export B","status":"open"},{"id":"F-3","severity":"minor","target":"c.ts:1","evidence":"style","status":"open"}]}`
				})
			case 2:
				require.Contains(t, a.Prompt, "Prior review open findings")
				require.Contains(t, a.Prompt, "F-1")
				require.Contains(t, a.Prompt, "F-2")
				return drivers.MockResult(func(r *drivers.Result) {
					r.Text = `{"verdict":"return","score":0.85,"reasons":["F-1 closed, F-2 open"],"findings":[{"id":"F-1","severity":"blocker","target":"a.ts:1","evidence":"ok","status":"resolved","resolution":"exported A"},{"id":"F-2","severity":"major","target":"b.ts:1","evidence":"still missing B","fix":"export B","status":"open"}]}`
				})
			case 3:
				require.Contains(t, a.Prompt, "F-2")
				return drivers.MockResult(func(r *drivers.Result) {
					r.Text = `{"verdict":"pass","score":0.97,"reasons":["all findings resolved"],"findings":[{"id":"F-2","severity":"major","target":"b.ts:1","evidence":"ok","status":"resolved","resolution":"exported B"}]}`
				})
			default:
				return drivers.MockResult(func(r *drivers.Result) {
					r.Text = `{"verdict":"pass","score":0.99,"reasons":["clean packet"]}`
				})
			}
		}
		opts := runOpts(fx, sd.set())
		opts.Config.Guardrails.MaxPacketAttempts = new(5)
		opts.Config.Guardrails.MaxReviewRounds = new(2)
		opts.Drivers = sd.set()
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "completed", summary.Status)
		require.Equal(t, 4, reviewCalls, "M01-T01 converges across 3 review rounds (2 -> 1 -> 0 open blockers) and M01-T02 passes on 1")
	})

	t.Run("priorOpenReviewFindings and latestPriorReview handle all evidence shapes", func(t *testing.T) {
		require.Nil(t, priorOpenReviewFindings(nil))
		require.Nil(t, priorOpenReviewFindings(map[string]any{"verdict": "return"}))
		require.Nil(t, priorOpenReviewFindings(map[string]any{"findings": "bad-shape"}))

		direct := &state.PacketState{Evidence: state.Evidence{Review: map[string]any{"verdict": "return"}}}
		require.Equal(t, direct.Evidence.Review, latestPriorReview(direct))

		fromMapHistory := &state.PacketState{EvidenceHistory: []any{
			map[string]any{"evidence": map[string]any{"review": map[string]any{"verdict": "return"}}},
			"not-a-map",
			map[string]any{"evidence": state.Evidence{}},
		}}
		require.Equal(t, map[string]any{"verdict": "return"}, latestPriorReview(fromMapHistory))

		require.Nil(t, latestPriorReview(&state.PacketState{}))
	})

	t.Run("reviewReturnCount skips non-map history entries", func(t *testing.T) {
		ps := &state.PacketState{RetryHistory: []any{
			map[string]any{"scope": "attempt", "error-code": "REVIEW_RETURNED"},
			map[string]any{"policy": "return-to-planner"},
			"not-a-map",
			map[string]any{"scope": "attempt", "error-code": "REVIEW_RETURNED"},
		}}
		require.Equal(t, 1, reviewReturnCount(ps))
	})
}
